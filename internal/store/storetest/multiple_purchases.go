package storetest

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/SampsonFox/assetloop/internal/domain"
)

// RunMultiplePurchases proves one physical item may carry several distinct
// built-in purchase records (device, services, accessories, a free gift) while
// repair/sale guards, append-only correction and sale uniqueness stay intact.
// It uses dedicated items so existing scenario totals are unchanged.
func RunMultiplePurchases(t *testing.T, store Store) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
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
	modelID := snapshot.Models[0].ID
	spec := application.NewSpecificationService(store)
	asset, err := spec.SaveAsset(ctx, owner, application.SaveSpecificationAsset{ModelID: modelID, DisplayName: "Multiple purchase phone"})
	if err != nil {
		t.Fatal(err)
	}
	service := application.NewLifecycleService(store)
	record := func(key, notes string, amount int64, day int) domain.AssetEvent {
		t.Helper()
		event, err := service.Record(ctx, owner, application.RecordEvent{
			RequestKey: key, AssetID: asset.ID, Type: domain.AssetEventPurchase, AmountMinor: amount, Currency: "CNY",
			OccurredAt: time.Date(2026, 3, 1+day, 12, 0, 0, 0, time.UTC), Notes: notes,
		})
		if err != nil {
			t.Fatalf("record %s: %v", key, err)
		}
		return event
	}
	device := record("multi-device", "device", 499_900, 0)
	record("multi-warranty", "extended warranty service", 19_900, 1)
	record("multi-protection", "screen protection service", 9_900, 1)
	record("multi-case", "protective case", 8_900, 2)
	record("multi-charger", "charger", 14_900, 3)
	if _, locked, err := service.BaseCurrency(ctx, owner); err != nil || !locked {
		t.Fatalf("nonzero purchase did not lock the base currency: locked=%v err=%v", locked, err)
	}
	// A free gift is the same built-in purchase type with zero magnitude.
	gift := record("multi-gift", "free gift", 0, 4)
	if gift.BaseAmountMinor != 0 {
		t.Fatalf("gift amount was not preserved as zero: %+v", gift)
	}
	// A zero purchase must not change the base-currency lock state.
	if _, locked, err := service.BaseCurrency(ctx, owner); err != nil || !locked {
		t.Fatalf("zero purchase changed base-currency locking: locked=%v err=%v", locked, err)
	}
	// Same key and payload replays the original event; different content conflicts.
	retry, err := service.Record(ctx, owner, application.RecordEvent{
		RequestKey: "multi-gift", AssetID: asset.ID, Type: domain.AssetEventPurchase, AmountMinor: 0, Currency: "CNY",
		OccurredAt: time.Date(2026, 3, 5, 12, 0, 0, 0, time.UTC), Notes: "free gift",
	})
	if err != nil || retry.ID != gift.ID {
		t.Fatalf("purchase retry duplicated the event: %+v %v", retry, err)
	}
	conflict := application.RecordEvent{
		RequestKey: "multi-gift", AssetID: asset.ID, Type: domain.AssetEventPurchase, AmountMinor: 1, Currency: "CNY",
		OccurredAt: time.Date(2026, 3, 5, 12, 0, 0, 0, time.UTC), Notes: "free gift",
	}
	var inputErr application.InputError
	if _, err := service.Record(ctx, owner, conflict); !errors.As(err, &inputErr) || inputErr.Code != "validation.request_conflict" {
		t.Fatalf("changed payload with the same key: %v", err)
	}
	// Append-only correction keeps the original purchase and allows later purchases.
	replacement, err := service.Correct(ctx, owner, device.ID, application.RecordEvent{
		AmountMinor: 450_000, Currency: "CNY", OccurredAt: time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC), Notes: "corrected device price",
	})
	if err != nil || replacement.ReplacesEventID != device.ID || replacement.BaseAmountMinor != -450_000 {
		t.Fatalf("correction did not append a replacement: %+v %v", replacement, err)
	}
	original, err := store.GetAssetEvent(ctx, owner.TenantID, device.ID)
	if err != nil || !original.IsVoided || original.BaseAmountMinor != -499_900 || original.Notes != "device" {
		t.Fatalf("correction overwrote the original purchase: %+v %v", original, err)
	}
	record("multi-after-correction", "post-correction purchase", 1_000, 5)
	// Two concurrent distinct purchases on one item both commit.
	keys := []string{"multi-concurrent-a", "multi-concurrent-b"}
	results := make(chan domain.AssetEvent, len(keys))
	start := make(chan struct{})
	for _, key := range keys {
		go func(key string) {
			<-start
			event, err := application.NewLifecycleService(store).Record(ctx, owner, application.RecordEvent{
				RequestKey: key, AssetID: asset.ID, Type: domain.AssetEventPurchase, AmountMinor: 2_000, Currency: "CNY",
				OccurredAt: time.Date(2026, 3, 6, 12, 0, 0, 0, time.UTC), Notes: key,
			})
			if err != nil {
				t.Errorf("concurrent purchase %s: %v", key, err)
			}
			results <- event
		}(key)
	}
	close(start)
	seen := map[string]bool{}
	for range keys {
		event := <-results
		if event.ID == "" || seen[event.ID] {
			t.Fatalf("concurrent purchase did not persist a distinct event: %+v", event)
		}
		seen[event.ID] = true
	}
	// Repair still needs an acquisition, sale stays unique, and nothing follows a sale.
	if _, err := service.Record(ctx, owner, application.RecordEvent{
		AssetID: asset.ID, Type: domain.AssetEventRepair, AmountMinor: 3_000, Currency: "CNY",
		OccurredAt: time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("repair after multiple purchases: %v", err)
	}
	sale, err := service.Record(ctx, owner, application.RecordEvent{
		AssetID: asset.ID, Type: domain.AssetEventSale, AmountMinor: 300_000, Currency: "CNY",
		OccurredAt: time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("record sale: %v", err)
	}
	if _, err := service.Record(ctx, owner, application.RecordEvent{
		AssetID: asset.ID, Type: domain.AssetEventSale, AmountMinor: 1, Currency: "CNY",
		OccurredAt: time.Date(2026, 3, 21, 12, 0, 0, 0, time.UTC),
	}); !errors.As(err, &inputErr) || inputErr.Code != "validation.event_after_sale" {
		t.Fatalf("second sale was accepted: %v", err)
	}
	if _, err := service.Record(ctx, owner, application.RecordEvent{
		RequestKey: "multi-after-sale", AssetID: asset.ID, Type: domain.AssetEventPurchase, AmountMinor: 1_000, Currency: "CNY",
		OccurredAt: time.Date(2026, 3, 21, 12, 0, 0, 0, time.UTC),
	}); !errors.As(err, &inputErr) || inputErr.Code != "validation.event_after_sale" {
		t.Fatalf("purchase after sale was accepted: %v", err)
	}
	// Duration derives from the earliest purchase, not the latest one.
	cost, err := service.CostDashboard(ctx, owner, asset.ID)
	if err != nil || !cost.Sold || !cost.HasDuration || !cost.Start.Equal(time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)) || !cost.End.Equal(sale.OccurredAt) {
		t.Fatalf("purchase-based duration mismatch: %+v %v", cost, err)
	}
	events, summary, err := service.Timeline(ctx, owner, asset.ID)
	if err != nil || summary.Status != "sold" || summary.ExpenseMinor != 511_600 || summary.IncomeMinor != 300_000 {
		t.Fatalf("multiple purchases changed the summary: events=%d summary=%+v err=%v", len(events), summary, err)
	}

	// A zero first purchase still acquires the item and enables repair without
	// changing the existing base-currency lock state.
	giftAsset, err := spec.SaveAsset(ctx, owner, application.SaveSpecificationAsset{ModelID: modelID, DisplayName: "Free gift phone"})
	if err != nil {
		t.Fatal(err)
	}
	_, lockBeforeGift, err := service.BaseCurrency(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Record(ctx, owner, application.RecordEvent{
		RequestKey: "gift-first", AssetID: giftAsset.ID, Type: domain.AssetEventPurchase, AmountMinor: 0, Currency: "CNY",
		OccurredAt: time.Date(2026, 3, 2, 12, 0, 0, 0, time.UTC), Notes: "gifted device",
	}); err != nil {
		t.Fatalf("zero first purchase: %v", err)
	}
	if _, err := service.Record(ctx, owner, application.RecordEvent{
		AssetID: giftAsset.ID, Type: domain.AssetEventRepair, AmountMinor: 500, Currency: "CNY",
		OccurredAt: time.Date(2026, 3, 3, 12, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("repair after zero purchase: %v", err)
	}
	// LifecycleService.Timeline derives status from the appended events, so an
	// unsold item with a repair is "active" (never "repairing"); the repair only
	// proves the zero purchase acquired the item and enabled the guard.
	_, summary, err = service.Timeline(ctx, owner, giftAsset.ID)
	if err != nil || summary.Status != "active" || summary.IncomeMinor != 0 || summary.ExpenseMinor != 500 {
		t.Fatalf("zero purchase did not acquire the item: %+v %v", summary, err)
	}
	if _, lockedAfterGift, err := service.BaseCurrency(ctx, owner); err != nil || lockedAfterGift != lockBeforeGift {
		t.Fatalf("zero-first item changed base-currency locking: before=%v after=%v err=%v", lockBeforeGift, lockedAfterGift, err)
	}
	// Zero remains a gift only: other expense or income types still reject it.
	if _, err := service.Record(ctx, owner, application.RecordEvent{
		AssetID: giftAsset.ID, Type: domain.AssetEventRepair, AmountMinor: 0, Currency: "CNY",
		OccurredAt: time.Date(2026, 3, 21, 12, 0, 0, 0, time.UTC),
	}); !errors.As(err, &inputErr) || inputErr.Code != "validation.amount_positive" {
		t.Fatalf("zero repair was accepted: %v", err)
	}
	if _, err := service.Record(ctx, owner, application.RecordEvent{
		AssetID: giftAsset.ID, Type: domain.AssetEventSale, AmountMinor: 0, Currency: "CNY",
		OccurredAt: time.Date(2026, 3, 21, 12, 0, 0, 0, time.UTC),
	}); !errors.As(err, &inputErr) || inputErr.Code != "validation.amount_positive" {
		t.Fatalf("zero sale was accepted: %v", err)
	}
	// Persisted FX evidence requires a positive original amount, so a zero gift is
	// only representable in the base currency and must be rejected before storage.
	if _, err := service.Record(ctx, owner, application.RecordEvent{
		RequestKey: "gift-foreign", AssetID: giftAsset.ID, Type: domain.AssetEventPurchase, AmountMinor: 0, Currency: "USD",
		OccurredAt: time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC), FXRateScaled: 712_000_000,
		FXRateDate: time.Date(2026, 3, 4, 0, 0, 0, 0, time.UTC), FXRateSource: "scenario-fixture", FXConfirmed: true,
	}); !errors.As(err, &inputErr) || inputErr.Code != "validation.amount_positive" {
		t.Fatalf("zero foreign purchase was accepted: %v", err)
	}
	remaining, _, err := service.Timeline(ctx, owner, giftAsset.ID)
	if err != nil || len(remaining) != 2 {
		t.Fatalf("rejected zero foreign purchase changed history: %d %v", len(remaining), err)
	}
}
