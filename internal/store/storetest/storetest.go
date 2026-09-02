package storetest

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/SampsonFox/assetloop/internal/domain"
)

type Store interface {
	application.Store
	application.AuthStore
	application.CatalogStore
	application.LifecycleStore
}

func Run(t *testing.T, store Store) {
	t.Helper()
	t.Run("asset", func(t *testing.T) { runAsset(t, store) })
	t.Run("auth", func(t *testing.T) { runAuth(t, store) })
	t.Run("catalog", func(t *testing.T) { runCatalog(t, store) })
	t.Run("lifecycle", func(t *testing.T) { runLifecycle(t, store) })
}

func runLifecycle(t *testing.T, store Store) {
	t.Helper()
	ctx := context.Background()
	owner, err := store.FirstPrincipal(ctx)
	if err != nil {
		t.Fatal(err)
	}
	catalog := application.NewCatalogService(store)
	snapshot, err := catalog.Snapshot(ctx, owner)
	if err != nil || len(snapshot.Assets) != 1 {
		t.Fatalf("get lifecycle asset: assets=%d err=%v", len(snapshot.Assets), err)
	}
	asset := snapshot.Assets[0]
	service := application.NewLifecycleService(store)
	purchase, err := service.Record(ctx, owner, application.RecordEvent{
		AssetID: asset.ID, Type: domain.AssetEventPurchase, AmountMinor: 10_000, Currency: "USD",
		FXRateScaled: 710_000_000, FXRateDate: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		FXRateSource: "store-test", FXConfirmed: true, OccurredAt: time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC),
		Source: "manual", ExternalReference: "ORDER-001", Notes: "purchase",
	})
	if err != nil {
		t.Fatalf("record foreign-currency purchase: %v", err)
	}
	if purchase.BaseAmountMinor != -71_000 || purchase.FX == nil || purchase.FX.OriginalCurrency != "USD" {
		t.Fatalf("purchase conversion evidence mismatch: %+v", purchase)
	}
	_, locked, err := service.BaseCurrency(ctx, owner)
	if err != nil || !locked {
		t.Fatalf("base currency should lock after first money event: locked=%v err=%v", locked, err)
	}
	repair, err := service.Record(ctx, owner, application.RecordEvent{
		AssetID: asset.ID, Type: domain.AssetEventRepair, AmountMinor: 20_000, Currency: "CNY",
		OccurredAt: time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC), Notes: "screen repair",
	})
	if err != nil {
		t.Fatalf("record repair: %v", err)
	}
	replacement, err := service.Correct(ctx, owner, repair.ID, application.RecordEvent{
		AmountMinor: 15_000, Currency: "CNY", OccurredAt: time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC), Notes: "corrected repair",
	})
	if err != nil {
		t.Fatalf("correct repair: %v", err)
	}
	if replacement.ReplacesEventID != repair.ID || replacement.BaseAmountMinor != -15_000 {
		t.Fatalf("replacement mismatch: %+v", replacement)
	}
	draft, err := service.CreateDraft(ctx, owner, application.CreateImportDraft{
		AssetID: asset.ID, Type: domain.AssetEventSale, AmountMinor: 800_000, Currency: "CNY",
		OccurredAt: time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC), Source: "ai-import",
		ExternalReference: "SALE-001", Notes: "sale", RawText: "recognized sale receipt",
	})
	if err != nil {
		t.Fatalf("create import draft: %v", err)
	}
	if _, err := service.ConfirmDraft(ctx, owner, draft.ID, application.ConfirmImport{}); err != nil {
		t.Fatalf("confirm base-currency import draft: %v", err)
	}
	if _, err := service.ConfirmDraft(ctx, owner, draft.ID, application.ConfirmImport{}); !errors.Is(err, application.ErrDraftNotPending) {
		t.Fatalf("confirmed draft must not be confirmed twice, got %v", err)
	}
	if drafts, err := service.PendingDrafts(ctx, owner); err != nil || len(drafts) != 0 {
		t.Fatalf("confirmed draft should leave pending list: drafts=%d err=%v", len(drafts), err)
	}
	events, summary, err := service.Timeline(ctx, owner, asset.ID)
	if err != nil {
		t.Fatalf("timeline: %v", err)
	}
	if len(events) != 5 || summary.ExpenseMinor != 86_000 || summary.IncomeMinor != 800_000 || summary.NetCashflowMinor != 714_000 || summary.Status != "已卖出" {
		t.Fatalf("unexpected lifecycle result: events=%d summary=%+v", len(events), summary)
	}
	if events[len(events)-1].FX != nil {
		t.Fatalf("base-currency sale should keep original-currency evidence nullable: %+v", events[len(events)-1])
	}
	originalRepair, err := store.GetAssetEvent(ctx, owner.TenantID, repair.ID)
	if err != nil || !originalRepair.IsVoided || originalRepair.Notes != "screen repair" {
		t.Fatalf("original repair should remain unchanged and voided: %+v err=%v", originalRepair, err)
	}
	if _, err := service.Correct(ctx, owner, repair.ID, application.RecordEvent{AmountMinor: 1, Currency: "CNY", OccurredAt: time.Now()}); !errors.Is(err, application.ErrAlreadyVoided) {
		t.Fatalf("second correction should fail, got %v", err)
	}
	viewer := owner
	viewer.Role = application.RoleViewer
	if _, _, err := service.Timeline(ctx, viewer, asset.ID); err != nil {
		t.Fatalf("viewer should read lifecycle: %v", err)
	}
	if _, err := service.Record(ctx, viewer, application.RecordEvent{}); !errors.Is(err, application.ErrForbidden) {
		t.Fatalf("viewer lifecycle write should be forbidden, got %v", err)
	}
}

func AssertAssetEventsAppendOnly(t *testing.T, db *sql.DB, driver string) {
	t.Helper()
	var eventID string
	if err := db.QueryRow("SELECT id FROM asset_events LIMIT 1").Scan(&eventID); err != nil {
		t.Fatalf("find event for append-only test: %v", err)
	}
	placeholder := "?"
	if driver == "postgres" {
		placeholder = "$1"
	}
	if _, err := db.Exec("UPDATE asset_events SET notes = 'mutated' WHERE id = "+placeholder, eventID); err == nil {
		t.Fatal("direct asset event update should be rejected")
	}
	if _, err := db.Exec("DELETE FROM asset_events WHERE id = "+placeholder, eventID); err == nil {
		t.Fatal("direct asset event delete should be rejected")
	}
}

func AssertBaseCurrencyLocked(t *testing.T, db *sql.DB, driver string) {
	t.Helper()
	lockedPredicate := "base_currency_locked = 1"
	if driver == "postgres" {
		lockedPredicate = "base_currency_locked = TRUE"
	}
	if _, err := db.Exec("UPDATE tenants SET base_currency = 'USD' WHERE " + lockedPredicate); err == nil {
		t.Fatal("locked base currency update should be rejected")
	}
	var currency string
	if err := db.QueryRow("SELECT base_currency FROM tenants WHERE " + lockedPredicate + " LIMIT 1").Scan(&currency); err != nil {
		t.Fatalf("read locked base currency: %v", err)
	}
	if currency != "CNY" {
		t.Fatalf("locked base currency changed: %q", currency)
	}
}

func runCatalog(t *testing.T, store Store) {
	t.Helper()
	ctx := context.Background()
	owner, err := store.FirstPrincipal(ctx)
	if err != nil {
		t.Fatalf("get catalog owner: %v", err)
	}
	service := application.NewCatalogService(store)
	category, err := service.CreateCategory(ctx, owner, application.CreateCategory{Name: "Phone", IconKey: "smartphone"})
	if err != nil {
		t.Fatalf("create category: %v", err)
	}
	model, err := service.CreateModel(ctx, owner, application.CreateModel{CategoryID: category.ID, Name: "Example Pro"})
	if err != nil {
		t.Fatalf("create model: %v", err)
	}
	variant256, err := service.CreateVariant(ctx, owner, application.CreateVariant{ModelID: model.ID, Name: "256GB"})
	if err != nil {
		t.Fatalf("create 256GB variant: %v", err)
	}
	if _, err := service.CreateVariant(ctx, owner, application.CreateVariant{ModelID: model.ID, Name: "512GB"}); err != nil {
		t.Fatalf("create 512GB variant: %v", err)
	}
	asset, err := service.CreateAsset(ctx, owner, application.CreateCatalogAsset{
		VariantID: variant256.ID, DisplayName: "Daily Phone", SerialNumber: "SERIAL-001",
		Color: "Black", PurchaseChannel: "Official Store", Notes: "Complete catalog record",
	})
	if err != nil {
		t.Fatalf("create catalog asset: %v", err)
	}
	if asset.Category != "Phone" || asset.Model != "Example Pro" || asset.Variant != "256GB" || asset.SerialNumber != "SERIAL-001" {
		t.Fatalf("catalog asset was not hydrated: %+v", asset)
	}
	if _, err := service.UpdateCategory(ctx, owner, application.UpdateCategory{ID: category.ID, Name: "Phones", IconKey: "tablet"}); err != nil {
		t.Fatalf("update category: %v", err)
	}
	if _, err := service.UpdateModel(ctx, owner, application.UpdateModel{ID: model.ID, CategoryID: category.ID, Name: "Example Ultra"}); err != nil {
		t.Fatalf("update model: %v", err)
	}
	if _, err := service.UpdateVariant(ctx, owner, application.UpdateVariant{ID: variant256.ID, ModelID: model.ID, Name: "256 GB"}); err != nil {
		t.Fatalf("update variant: %v", err)
	}
	updatedAsset, err := service.UpdateAsset(ctx, owner, application.UpdateCatalogAsset{ID: asset.ID, VariantID: variant256.ID, DisplayName: "Updated Phone", SerialNumber: "SERIAL-001", Color: "Blue", PurchaseChannel: "Retail", Notes: "Updated"})
	if err != nil || updatedAsset.DisplayName != "Updated Phone" || updatedAsset.CategoryIcon != "tablet" || updatedAsset.Category != "Phones" || updatedAsset.Model != "Example Ultra" || updatedAsset.Variant != "256 GB" {
		t.Fatalf("updated catalog hierarchy mismatch: asset=%+v err=%v", updatedAsset, err)
	}
	snapshot, err := service.Snapshot(ctx, owner)
	if err != nil {
		t.Fatalf("catalog snapshot: %v", err)
	}
	if len(snapshot.Categories) != 1 || len(snapshot.Models) != 1 || len(snapshot.Variants) != 2 || len(snapshot.Assets) != 1 {
		t.Fatalf("unexpected catalog counts: categories=%d models=%d variants=%d assets=%d", len(snapshot.Categories), len(snapshot.Models), len(snapshot.Variants), len(snapshot.Assets))
	}
	viewer := owner
	viewer.Role = application.RoleViewer
	if _, err := service.CreateCategory(ctx, viewer, application.CreateCategory{Name: "Forbidden"}); !errors.Is(err, application.ErrForbidden) {
		t.Fatalf("viewer catalog write should be forbidden, got %v", err)
	}
	if _, err := service.CreateModel(ctx, owner, application.CreateModel{CategoryID: "99999999-9999-4999-8999-999999999999", Name: "Cross tenant"}); err == nil {
		t.Fatal("model with unavailable category should fail")
	}
	if _, err := service.CreateAsset(ctx, owner, application.CreateCatalogAsset{VariantID: variant256.ID, DisplayName: "Duplicate serial", SerialNumber: "SERIAL-001"}); err == nil {
		t.Fatal("duplicate non-empty serial should fail")
	}
	foreign := owner
	foreign.TenantID = "99999999-9999-4999-8999-999999999999"
	if _, err := service.GetAsset(ctx, foreign, asset.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("cross-tenant catalog read should be hidden, got %v", err)
	}
}

func runAsset(t *testing.T, store application.Store) {
	t.Helper()
	ctx := context.Background()
	asset := domain.Asset{
		ID: "11111111-1111-4111-8111-111111111111", TenantID: "22222222-2222-4222-8222-222222222222",
		CategoryID: "33333333-3333-4333-8333-333333333333", Category: "Phone",
		CategoryIcon: "package",
		ModelID:      "44444444-4444-4444-8444-444444444444", Model: "Example Phone",
		VariantID: "55555555-5555-4555-8555-555555555555", Variant: "256GB",
		DisplayName: "My Example Phone", CreatedAt: time.Date(2026, 9, 1, 1, 2, 3, 0, time.UTC),
	}
	created, err := store.CreateAsset(ctx, asset)
	if err != nil {
		t.Fatalf("create asset: %v", err)
	}
	asset = created
	got, err := store.GetAsset(ctx, asset.TenantID, asset.ID)
	if err != nil {
		t.Fatalf("get asset: %v", err)
	}
	gotCreatedAt, wantCreatedAt := got.CreatedAt, asset.CreatedAt
	got.CreatedAt, asset.CreatedAt = time.Time{}, time.Time{}
	if got != asset || !gotCreatedAt.Equal(wantCreatedAt) {
		t.Fatalf("asset mismatch:\n got: %+v\nwant: %+v", got, asset)
	}
	got.CreatedAt, asset.CreatedAt = gotCreatedAt, wantCreatedAt
	second := asset
	second.ID = "66666666-6666-4666-8666-666666666666"
	second.CategoryID = "77777777-7777-4777-8777-777777777777"
	second.ModelID = "88888888-8888-4888-8888-888888888888"
	second.VariantID = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	second.DisplayName = "Second Example Phone"
	second, err = store.CreateAsset(ctx, second)
	if err != nil {
		t.Fatalf("create second asset: %v", err)
	}
	if second.CategoryID != asset.CategoryID || second.ModelID != asset.ModelID || second.VariantID != asset.VariantID {
		t.Fatalf("existing category/model/variant were not reused: %+v", second)
	}
	_, err = store.GetAsset(ctx, "99999999-9999-4999-8999-999999999999", asset.ID)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("cross-tenant read should return sql.ErrNoRows, got %v", err)
	}
}

func runAuth(t *testing.T, store Store) {
	t.Helper()
	ctx := context.Background()
	needsSetup, err := store.AuthNeedsSetup(ctx)
	if err != nil || !needsSetup {
		t.Fatalf("fresh store should need auth setup: needs=%v err=%v", needsSetup, err)
	}
	service := application.NewAuthService(store)
	credential, err := service.Setup(ctx, application.SetupAuth{
		TenantName: "Store Test", BaseCurrency: "CNY", Username: "store-owner", Password: "store owner password",
	})
	if err != nil {
		t.Fatalf("setup auth: %v", err)
	}
	got, err := service.Authenticate(ctx, credential.Token)
	if err != nil || got != credential.Principal {
		t.Fatalf("session principal mismatch: got=%+v want=%+v err=%v", got, credential.Principal, err)
	}
	got, err = service.UpdatePreferences(ctx, got, application.UpdatePreferences{Locale: application.LocaleEn, Theme: application.ThemeDark})
	if err != nil {
		t.Fatalf("update preferences: %v", err)
	}
	got, err = service.Authenticate(ctx, credential.Token)
	if err != nil || got.Locale != application.LocaleEn || got.Theme != application.ThemeDark {
		t.Fatalf("stored preferences mismatch: principal=%+v err=%v", got, err)
	}
	if _, err := service.AddMember(ctx, got, application.AddMember{Username: "store-viewer", Password: "store viewer password", Role: application.RoleViewer}); err != nil {
		t.Fatalf("create member: %v", err)
	}
	members, err := service.ListMembers(ctx, got)
	if err != nil || len(members) != 2 {
		t.Fatalf("list members: count=%d err=%v", len(members), err)
	}
	err = store.CreateSession(ctx, application.Session{
		TokenHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		TenantID:  "99999999-9999-4999-8999-999999999999", UserID: got.UserID,
		CreatedAt: time.Now().UTC(), ExpiresAt: time.Now().UTC().Add(time.Hour),
	})
	if err == nil {
		t.Fatal("cross-tenant session should violate membership foreign key")
	}
	if err := service.Logout(ctx, credential.Token); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if _, err := service.Authenticate(ctx, credential.Token); !errors.Is(err, application.ErrUnauthorized) {
		t.Fatalf("deleted session should be unauthorized, got %v", err)
	}
}
