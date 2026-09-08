package storetest

import (
	"context"
	"database/sql"
	"errors"
	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/SampsonFox/assetloop/internal/domain"
	"github.com/google/uuid"
	"reflect"
	"testing"
	"time"
)

type quoteFixture struct {
	now     *time.Time
	calls   int
	err     error
	model   string
	max     int64
	entered chan struct{}
	release chan struct{}
}

func (p *quoteFixture) FetchQuote(ctx context.Context, q application.MarketQuery) (application.MarketQuote, error) {
	p.calls++
	if p.entered != nil {
		close(p.entered)
		select {
		case <-ctx.Done():
			return application.MarketQuote{}, ctx.Err()
		case <-p.release:
		}
	}
	return application.MarketQuote{Provider: "zhuanzhuan", ProviderVersion: "fixture-1", Currency: "CNY", ModelDesc: p.model, MaxMinor: p.max, ObservedAt: *p.now, Evidence: "{}"}, p.err
}

type fxFixture struct {
	err   error
	dates []string
}

func (f *fxFixture) Rate(ctx context.Context, base, quote, date string) (application.FXRate, error) {
	f.dates = append(f.dates, date)
	return application.FXRate{Scaled: 14000000, Date: date, Source: "fixture:daily"}, f.err
}

// RunMarket exercises public use cases and independent connections on each adapter.
func RunMarket(t *testing.T, first, second Store, db *sql.DB, driver string) {
	t.Helper()
	ctx := context.Background()
	actor, e := first.FirstPrincipal(ctx)
	if e != nil {
		t.Fatal(e)
	}
	now := time.Date(2026, 9, 8, 2, 0, 0, 0, time.UTC)
	p := &quoteFixture{now: &now, model: "phone 256GB", max: 645800}
	svc := application.NewMarketService(first, p, nil, application.MarketOptions{UnitsConfirmed: true, Now: func() time.Time { return now }})
	catalog := application.NewCatalogService(first)
	cat, e := catalog.CreateCategory(ctx, actor, application.CreateCategory{Name: "Market fixture"})
	if e != nil {
		t.Fatal(e)
	}
	model, e := catalog.CreateModel(ctx, actor, application.CreateModel{CategoryID: cat.ID, Name: "Market phone"})
	if e != nil {
		t.Fatal(e)
	}
	spec := application.NewSpecificationService(first)
	asset := func(name string) domain.Asset {
		t.Helper()
		v, e := spec.SaveAsset(ctx, actor, application.SaveSpecificationAsset{ModelID: model.ID, DisplayName: name})
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	one, two := asset("First market device"), asset("Second market device")
	cmd := application.CreateMarketItem{Name: "Shared phone", Query: application.MarketQuery{Keyword: "phone", FilterCriteria: "256GB"}, ConfirmedModel: p.model, AssetID: one.ID}
	gated := application.NewMarketService(first, p, nil, application.MarketOptions{})
	if _, e := gated.Create(ctx, actor, cmd); !errors.Is(e, application.ErrMarketUnit) {
		t.Fatal(e)
	}
	item, e := svc.Create(ctx, actor, cmd)
	if e != nil {
		t.Fatal(e)
	}
	if e := svc.Bind(ctx, actor, two.ID, item.ID); e != nil {
		t.Fatal(e)
	}
	latest, e := svc.LatestForAsset(ctx, actor, two.ID)
	if e != nil || latest == nil || latest.BaseMinor == nil || *latest.BaseMinor != 645800 || latest.FX.RateSource != "identity" {
		t.Fatalf("shared quote: %+v %v", latest, e)
	}
	got, e := second.GetMarketItem(ctx, actor.TenantID, item.ID)
	if e != nil || !got.LastSuccess.Equal(now) {
		t.Fatalf("initial status: %+v %v", got, e)
	}
	other, e := svc.Create(ctx, actor, cmd)
	if e != nil || other.ID != item.ID {
		t.Fatalf("dedup: %+v %v", other, e)
	}
	cmd.Query.FilterCriteria = "512GB"
	other, e = svc.Create(ctx, actor, cmd)
	if e != nil || other.ID == item.ID {
		t.Fatalf("configuration isolation: %v", e)
	}
	// Bind back after the explicit different-configuration creation.
	if e = svc.Bind(ctx, actor, one.ID, item.ID); e != nil {
		t.Fatal(e)
	}
	before, e := application.NewLifecycleService(first).CostDashboard(ctx, actor, one.ID)
	if e != nil {
		t.Fatal(e)
	}
	now = now.Add(time.Hour)
	p.max = 650000
	if e = svc.Refresh(ctx, actor, item.ID); e != nil {
		t.Fatal(e)
	}
	prices, e := second.ListMarketPrices(ctx, actor.TenantID, item.ID)
	if e != nil || len(prices) != 1 || prices[0].MaxMinor != 650000 || !prices[0].ObservedAt.Equal(now) {
		t.Fatalf("same day: %+v %v", prices, e)
	}
	p.err = application.ErrMarketInvalid
	now = now.Add(time.Hour)
	if e = svc.Refresh(ctx, actor, item.ID); !errors.Is(e, p.err) {
		t.Fatal(e)
	}
	retained, _ := svc.LatestForAsset(ctx, actor, one.ID)
	if !reflect.DeepEqual(retained, &prices[0]) {
		t.Fatal("failure changed last successful price")
	}
	got, _ = second.GetMarketItem(ctx, actor.TenantID, item.ID)
	if got.LastError != application.ErrMarketInvalid.Error() || got.LeaseToken != "" {
		t.Fatalf("failure status: %+v", got)
	}
	p.err = nil
	p.model = "unexpected model"
	if e = svc.Refresh(ctx, actor, item.ID); !errors.Is(e, application.ErrMarketMismatch) {
		t.Fatal(e)
	}
	p.model = "phone 256GB"
	now = now.AddDate(0, 0, 1)
	calls := p.calls
	n, e := svc.RefreshDue(ctx, actor.TenantID, "")
	if e != nil || n != 1 || p.calls != calls+1 {
		t.Fatalf("shared daily refresh: %d %v %d", n, e, p.calls-calls)
	}
	n, e = svc.RefreshDue(ctx, actor.TenantID, "")
	if e != nil || n != 0 {
		t.Fatalf("same day skip: %d %v", n, e)
	}
	after, e := application.NewLifecycleService(first).CostDashboard(ctx, actor, one.ID)
	if e != nil || (before.NetMinor != after.NetMinor || before.ExpenseMinor != after.ExpenseMinor || before.IncomeMinor != after.IncomeMinor) {
		t.Fatalf("quotes changed cashflow: %v", e)
	}
	if e = svc.Update(ctx, actor, item.ID, "Renamed", false); e != nil {
		t.Fatal(e)
	}
	now = now.AddDate(0, 0, 1)
	n, e = svc.RefreshDue(ctx, actor.TenantID, "")
	if e != nil || n != 0 {
		t.Fatal(n, e)
	}
	history, _ := first.ListMarketPrices(ctx, actor.TenantID, item.ID)
	if len(history) != 2 {
		t.Fatal("rename/disable lost history")
	}
	if e = svc.Update(ctx, actor, item.ID, "Renamed", true); e != nil {
		t.Fatal(e)
	}
	viewer := actor
	viewer.Role = application.RoleViewer
	if e = svc.Bind(ctx, viewer, one.ID, item.ID); !errors.Is(e, application.ErrForbidden) {
		t.Fatal(e)
	}
	if _, e = svc.Preview(ctx, viewer, cmd.Query); !errors.Is(e, application.ErrForbidden) {
		t.Fatal(e)
	}
	outsider := actor
	outsider.TenantID = uuid.NewString()
	if e = svc.Bind(ctx, outsider, one.ID, item.ID); e == nil {
		t.Fatal("cross-tenant bind accepted")
	}
	if _, e = svc.LatestForAsset(ctx, outsider, one.ID); e == nil {
		t.Fatal("cross-tenant read accepted")
	}
	// One in-flight external call holds no write transaction; a second process cannot fetch.
	p.entered = make(chan struct{})
	p.release = make(chan struct{})
	done := make(chan error, 1)
	go func() { done <- svc.Refresh(ctx, actor, item.ID) }()
	select {
	case <-p.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("refresh did not reach provider")
	}
	peer := application.NewMarketService(second, p, nil, application.MarketOptions{UnitsConfirmed: true, Now: func() time.Time { return now }})
	if e = peer.Refresh(ctx, actor, item.ID); !errors.Is(e, application.ErrMarketBusy) {
		t.Fatal(e)
	}
	close(p.release)
	if e = <-done; e != nil {
		t.Fatal(e)
	}
	p.entered = nil
	p.release = nil
	// Lifecycle-derived stop and resume, retaining the 90-day tail.
	life := application.NewLifecycleService(first)
	saleDate := now.AddDate(0, 0, -89)
	for _, a := range []domain.Asset{one, two} {
		if _, e = life.Record(ctx, actor, application.RecordEvent{AssetID: a.ID, Type: domain.AssetEventPurchase, AmountMinor: -100000, Currency: "CNY", OccurredAt: saleDate.AddDate(0, 0, -1)}); e != nil {
			t.Fatal(e)
		}
		if _, e = life.Record(ctx, actor, application.RecordEvent{AssetID: a.ID, Type: domain.AssetEventSale, AmountMinor: 50000, Currency: "CNY", OccurredAt: saleDate}); e != nil {
			t.Fatal(e)
		}
	}
	now = now.AddDate(0, 0, 1)
	n, e = svc.RefreshDue(ctx, actor.TenantID, "")
	if e != nil || n != 1 {
		t.Fatalf("day90: %d %v", n, e)
	}
	now = now.AddDate(0, 0, 1)
	n, e = svc.RefreshDue(ctx, actor.TenantID, "")
	if e != nil || n != 0 {
		t.Fatalf("after90: %d %v", n, e)
	}
	three := asset("New held device")
	if e = svc.Bind(ctx, actor, three.ID, item.ID); e != nil {
		t.Fatal(e)
	}
	n, e = svc.RefreshDue(ctx, actor.TenantID, "")
	if e != nil || n != 1 {
		t.Fatalf("resume: %d %v", n, e)
	}
	// A real second tenant starts with no monetary data. Failed FX still locks currency.
	foreign := actor
	foreign.TenantID = uuid.NewString()
	created := any("2026-09-01T00:00:00Z")
	if driver == "postgres" {
		created = now
	}
	query := "INSERT INTO tenants(id,name,base_currency,created_at) VALUES(?,?,?,?)"
	if driver == "postgres" {
		query = "INSERT INTO tenants(id,name,base_currency,created_at) VALUES($1,$2,$3,$4)"
	}
	if _, e = db.Exec(query, foreign.TenantID, "Market USD", "USD", created); e != nil {
		t.Fatal(e)
	}
	fx := &fxFixture{err: application.ErrMarketFX}
	foreignSvc := application.NewMarketService(first, p, fx, application.MarketOptions{UnitsConfirmed: true, Now: func() time.Time { return now }})
	cmd.AssetID = ""
	cmd.Name = "USD phone"
	foreignItem, e := foreignSvc.Create(ctx, foreign, cmd)
	if e != nil {
		t.Fatal(e)
	}
	pending, _ := first.ListMarketPrices(ctx, foreign.TenantID, foreignItem.ID)
	if len(pending) != 1 || pending[0].BaseMinor != nil || pending[0].MaxMinor != 650000 {
		t.Fatalf("pending: %+v", pending)
	}
	_, locked, e := first.TenantBaseCurrency(ctx, foreign.TenantID)
	if e != nil || !locked {
		t.Fatal("market quote did not lock base currency", e)
	}
	if e = svc.Bind(ctx, actor, three.ID, foreignItem.ID); e == nil {
		t.Fatal("foreign market item binding accepted")
	}
	// The FK protects direct invalid store writes too.
	if e = first.BindAssetMarket(ctx, actor.TenantID, three.ID, foreignItem.ID); e == nil {
		t.Fatal("cross-tenant FK missing")
	}
	fx.err = nil
	oldDate := pending[0].ObservationDate
	now = now.AddDate(0, 0, 2)
	if e = foreignSvc.Refresh(ctx, foreign, foreignItem.ID); e != nil {
		t.Fatal(e)
	}
	repaired, _ := second.ListMarketPrices(ctx, foreign.TenantID, foreignItem.ID)
	if len(repaired) != 2 || repaired[1].BaseMinor == nil || *repaired[1].BaseMinor != 91000 || repaired[1].FX.RateDate.Format("2006-01-02") != oldDate {
		t.Fatalf("historical FX repair: %+v", repaired)
	}
}
