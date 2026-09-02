package application

import (
	"context"
	"errors"
	"testing"

	"github.com/SampsonFox/assetloop/internal/domain"
)

func TestCatalogServiceValidatesRoleTenantAndNames(t *testing.T) {
	store := &catalogSpy{}
	service := NewCatalogService(store)
	owner := Principal{
		TenantID: "11111111-1111-4111-8111-111111111111", TenantName: "Catalog",
		UserID: "22222222-2222-4222-8222-222222222222", Username: "owner", Role: RoleOwner,
	}
	viewer := owner
	viewer.Role = RoleViewer

	if _, err := service.CreateCategory(context.Background(), viewer, CreateCategory{Name: "Phone"}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("viewer write should be forbidden, got %v", err)
	}
	if _, err := service.CreateCategory(context.Background(), owner, CreateCategory{Name: "   "}); err == nil {
		t.Fatal("blank category should fail")
	}
	category, err := service.CreateCategory(context.Background(), owner, CreateCategory{Name: "  Phone  ", IconKey: "smartphone"})
	if err != nil {
		t.Fatal(err)
	}
	if category.Name != "Phone" || category.IconKey != "smartphone" || store.createdCategory.TenantID != owner.TenantID {
		t.Fatalf("category was not normalized and tenant scoped: %+v", store.createdCategory)
	}
	if _, err := service.CreateModel(context.Background(), owner, CreateModel{CategoryID: "not-a-uuid", Name: "Model"}); err == nil {
		t.Fatal("invalid parent identifier should fail")
	}
	if _, err := service.CreateCategory(context.Background(), owner, CreateCategory{Name: "Bad", IconKey: "script"}); err == nil {
		t.Fatal("unknown category icon should fail")
	}
}

type catalogSpy struct {
	createdCategory domain.ItemCategory
}

func (s *catalogSpy) CreateCategory(_ context.Context, value domain.ItemCategory) error {
	s.createdCategory = value
	return nil
}
func (*catalogSpy) UpdateCategory(context.Context, domain.ItemCategory) error  { return nil }
func (*catalogSpy) CreateModel(context.Context, domain.ProductModel) error     { return nil }
func (*catalogSpy) UpdateModel(context.Context, domain.ProductModel) error     { return nil }
func (*catalogSpy) CreateVariant(context.Context, domain.ProductVariant) error { return nil }
func (*catalogSpy) UpdateVariant(context.Context, domain.ProductVariant) error { return nil }
func (*catalogSpy) CreateCatalogAsset(context.Context, domain.Asset) error     { return nil }
func (*catalogSpy) UpdateCatalogAsset(context.Context, domain.Asset) error     { return nil }
func (*catalogSpy) ListCategories(context.Context, string) ([]domain.ItemCategory, error) {
	return nil, nil
}
func (*catalogSpy) ListModels(context.Context, string) ([]domain.ProductModel, error) {
	return nil, nil
}
func (*catalogSpy) ListVariants(context.Context, string) ([]domain.ProductVariant, error) {
	return nil, nil
}
func (*catalogSpy) ListAssets(context.Context, string) ([]domain.Asset, error) { return nil, nil }
func (*catalogSpy) GetAsset(context.Context, string, string) (domain.Asset, error) {
	return domain.Asset{}, nil
}
