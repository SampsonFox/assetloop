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
}

func Run(t *testing.T, store Store) {
	t.Helper()
	t.Run("asset", func(t *testing.T) { runAsset(t, store) })
	t.Run("auth", func(t *testing.T) { runAuth(t, store) })
	t.Run("catalog", func(t *testing.T) { runCatalog(t, store) })
}

func runCatalog(t *testing.T, store Store) {
	t.Helper()
	ctx := context.Background()
	owner, err := store.FirstPrincipal(ctx)
	if err != nil {
		t.Fatalf("get catalog owner: %v", err)
	}
	service := application.NewCatalogService(store)
	category, err := service.CreateCategory(ctx, owner, "Phone")
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
	snapshot, err := service.Snapshot(ctx, owner)
	if err != nil {
		t.Fatalf("catalog snapshot: %v", err)
	}
	if len(snapshot.Categories) != 1 || len(snapshot.Models) != 1 || len(snapshot.Variants) != 2 || len(snapshot.Assets) != 1 {
		t.Fatalf("unexpected catalog counts: categories=%d models=%d variants=%d assets=%d", len(snapshot.Categories), len(snapshot.Models), len(snapshot.Variants), len(snapshot.Assets))
	}
	viewer := owner
	viewer.Role = application.RoleViewer
	if _, err := service.CreateCategory(ctx, viewer, "Forbidden"); !errors.Is(err, application.ErrForbidden) {
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
		ModelID: "44444444-4444-4444-8444-444444444444", Model: "Example Phone",
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
