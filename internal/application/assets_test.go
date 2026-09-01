package application

import (
	"context"
	"testing"

	"github.com/SampsonFox/assetloop/internal/domain"
)

type memoryStore struct{ asset domain.Asset }

func (s *memoryStore) CreateAsset(_ context.Context, asset domain.Asset) (domain.Asset, error) {
	s.asset = asset
	return asset, nil
}

func (s *memoryStore) GetAsset(_ context.Context, _, _ string) (domain.Asset, error) {
	return s.asset, nil
}

func TestAssetServiceValidatesAndCreates(t *testing.T) {
	store := &memoryStore{}
	service := NewAssetService(store)
	if _, err := service.Create(context.Background(), CreateAsset{}); err == nil {
		t.Fatal("expected missing tenant validation error")
	}
	asset, err := service.Create(context.Background(), CreateAsset{
		TenantID: "22222222-2222-4222-8222-222222222222",
		Category: " Phone ", Model: " Example Phone ", Variant: " 256GB ",
	})
	if err != nil {
		t.Fatal(err)
	}
	if asset.DisplayName != "Example Phone 256GB" || asset.Category != "Phone" {
		t.Fatalf("unexpected normalized asset: %+v", asset)
	}
}
