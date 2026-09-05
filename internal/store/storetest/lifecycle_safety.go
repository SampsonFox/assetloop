package storetest

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/SampsonFox/assetloop/internal/domain"
)

// Force two callers to observe the same pre-write state in the defective path.
// A transaction-bound service bypasses this outer adapter entirely.
type lifecycleReadBarrier struct {
	application.LifecycleStore
	mu       sync.Mutex
	arrivals int
	ready    chan struct{}
}

// RunLifecycleRetries uses two independently opened connections to the same database.
func RunLifecycleRetries(t *testing.T, first, second Store) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	owner, err := first.FirstPrincipal(ctx)
	if err != nil {
		t.Fatal(err)
	}
	catalog := application.NewCatalogService(first)
	snapshot, err := catalog.Snapshot(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	asset, err := catalog.CreateAsset(ctx, owner, application.CreateCatalogAsset{VariantID: snapshot.Variants[0].ID, DisplayName: "Idempotency regression"})
	if err != nil {
		t.Fatal(err)
	}
	services := []*application.LifecycleService{application.NewLifecycleService(first), application.NewLifecycleService(second)}
	cmd := application.RecordEvent{RequestKey: "purchase-retry", AssetID: asset.ID, Type: domain.AssetEventPurchase, AmountMinor: 10000, Currency: "CNY", OccurredAt: time.Date(2026, 8, 26, 0, 0, 0, 0, time.UTC)}
	type result struct {
		event domain.AssetEvent
		err   error
	}
	results := make(chan result, 6)
	start := make(chan struct{})
	for i := range 6 {
		go func() { <-start; event, err := services[i%2].Record(ctx, owner, cmd); results <- result{event, err} }()
	}
	close(start)
	var purchase domain.AssetEvent
	for range 6 {
		got := <-results
		if got.err != nil {
			t.Fatal(got.err)
		}
		if purchase.ID != "" && got.event.ID != purchase.ID {
			t.Fatal("retry created another event")
		}
		purchase = got.event
	}
	cmd.AmountMinor++
	if _, err := services[0].Record(ctx, owner, cmd); err == nil {
		t.Fatal("key reused for different amount")
	}
	cmd.AmountMinor--
	viewer := owner
	viewer.Role = application.RoleViewer
	if _, err := services[0].Record(ctx, viewer, cmd); !errors.Is(err, application.ErrForbidden) {
		t.Fatalf("retry bypassed authorization: %v", err)
	}
	// A failed command does not reserve its key or commit an economic transaction.
	repair := cmd
	repair.RequestKey = "repair-retry"
	repair.Type = domain.AssetEventRepair
	repair.AmountMinor = -1
	if _, err := services[0].Record(ctx, owner, repair); err == nil {
		t.Fatal("invalid amount accepted")
	}
	repair.AmountMinor = 200
	maintenance, err := services[0].Record(ctx, owner, repair)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := services[1].Record(ctx, owner, repair)
	if err != nil || replayed.ID != maintenance.ID {
		t.Fatalf("repair retry: %+v %v", replayed, err)
	}
	correction := repair
	correction.RequestKey = "correction-retry"
	correction.AmountMinor = 150
	replacement, err := services[0].Correct(ctx, owner, maintenance.ID, correction)
	if err != nil {
		t.Fatal(err)
	}
	for i := range 6 {
		go func() {
			event, err := services[i%2].Correct(ctx, owner, maintenance.ID, correction)
			results <- result{event, err}
		}()
	}
	for range 6 {
		got := <-results
		if got.err != nil || got.event.ID != replacement.ID {
			t.Fatalf("concurrent correction retry: %+v %v", got.event, got.err)
		}
	}
	replayed, err = services[1].Correct(ctx, owner, maintenance.ID, correction)
	if err != nil || replayed.ID != replacement.ID {
		t.Fatalf("correction retry after void: %+v %v", replayed, err)
	}
	if _, err := services[0].Record(ctx, owner, correction); err == nil {
		t.Fatal("same key accepted for different operation")
	}
	// Different keys correcting the same original must still have only one winner.
	correction.RequestKey = "another-correction"
	if _, err := services[1].Correct(ctx, owner, maintenance.ID, correction); !errors.Is(err, application.ErrAlreadyVoided) {
		t.Fatalf("original corrected twice: %v", err)
	}
	events, summary, err := services[0].Timeline(ctx, owner, asset.ID)
	if err != nil || len(events) != 4 || summary.ExpenseMinor != 10150 {
		t.Fatalf("retry changed economic history: %d %+v %v", len(events), summary, err)
	}
	// A distinct user may use the same key without receiving someone else's receipt.
	_, err = application.NewAuthService(first).AddMember(ctx, owner, application.AddMember{Username: "retry-editor", Password: "retry editor password", Role: application.RoleEditor})
	if err != nil {
		t.Fatal(err)
	}
	login, err := application.NewAuthService(first).Login(ctx, application.Login{Username: "retry-editor", Password: "retry editor password"})
	if err != nil {
		t.Fatal(err)
	}
	repair.RequestKey = "repair-retry"
	other, err := services[1].Record(ctx, login.Principal, repair)
	if err != nil || other.ID == maintenance.ID {
		t.Fatalf("request keys not user-scoped: %+v %v", other, err)
	}
	if _, found, err := first.FindLifecycleRequest(ctx, "99999999-9999-4999-8999-999999999999", owner.UserID, "purchase-retry"); err != nil || found {
		t.Fatalf("cross-tenant receipt visible: %v %v", found, err)
	}
	before, _, err := services[0].Timeline(ctx, owner, asset.ID)
	if err != nil {
		t.Fatal(err)
	}
	repair.RequestKey = "failed-receipt"
	failing := application.NewLifecycleService(failedReceiptStore{LifecycleStore: first})
	if _, err := failing.Record(ctx, owner, repair); err == nil {
		t.Fatal("receipt failure reported success")
	}
	after, _, err := services[0].Timeline(ctx, owner, asset.ID)
	if err != nil || len(after) != len(before) {
		t.Fatalf("receipt failure committed event: %d -> %d %v", len(before), len(after), err)
	}
	if _, found, err := first.FindLifecycleRequest(ctx, owner.TenantID, owner.UserID, repair.RequestKey); err != nil || found {
		t.Fatalf("failed receipt persisted: %v %v", found, err)
	}
	if _, err := services[1].Record(ctx, owner, repair); err != nil {
		t.Fatalf("rolled-back request cannot retry: %v", err)
	}
}

type failedReceiptStore struct{ application.LifecycleStore }

func (s failedReceiptStore) WithLifecycleWrite(ctx context.Context, tenantID string, fn func(application.LifecycleStore) (domain.AssetEvent, error)) (domain.AssetEvent, error) {
	return s.LifecycleStore.WithLifecycleWrite(ctx, tenantID, func(scoped application.LifecycleStore) (domain.AssetEvent, error) {
		return fn(failedReceiptStore{scoped})
	})
}
func (s failedReceiptStore) SaveLifecycleRequest(context.Context, application.LifecycleRequest) error {
	return errors.New("injected receipt failure")
}

func (s *lifecycleReadBarrier) ListAssetEvents(ctx context.Context, tenantID, assetID string) ([]domain.AssetEvent, error) {
	events, err := s.LifecycleStore.ListAssetEvents(ctx, tenantID, assetID)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.arrivals++
	if s.arrivals == 2 {
		close(s.ready)
	}
	s.mu.Unlock()
	select {
	case <-s.ready:
		return events, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func runConcurrentLifecycle(t *testing.T, store Store) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	owner, err := store.FirstPrincipal(ctx)
	if err != nil {
		t.Fatal(err)
	}
	catalog := application.NewCatalogService(store)
	snapshot, err := catalog.Snapshot(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	asset, err := catalog.CreateAsset(ctx, owner, application.CreateCatalogAsset{VariantID: snapshot.Variants[0].ID, DisplayName: "Concurrency regression"})
	if err != nil {
		t.Fatal(err)
	}
	for _, eventType := range []domain.AssetEventType{domain.AssetEventPurchase, domain.AssetEventSale} {
		barrier := &lifecycleReadBarrier{LifecycleStore: store, ready: make(chan struct{})}
		service := application.NewLifecycleService(barrier)
		errs := make(chan error, 2)
		for range 2 {
			go func() {
				_, err := service.Record(ctx, owner, application.RecordEvent{AssetID: asset.ID, Type: eventType, AmountMinor: 100, Currency: "CNY", OccurredAt: time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC)})
				errs <- err
			}()
		}
		succeeded := 0
		for range 2 {
			if <-errs == nil {
				succeeded++
			}
		}
		if succeeded != 1 {
			t.Fatalf("concurrent %s: want one success, got %d", eventType, succeeded)
		}
	}
}
