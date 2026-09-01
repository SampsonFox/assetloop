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

func Run(t *testing.T, store application.Store) {
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
	if got != asset {
		t.Fatalf("asset mismatch:\n got: %+v\nwant: %+v", got, asset)
	}
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
