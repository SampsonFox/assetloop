package sqlite

import (
	"context"
	"fmt"
	"time"

	"github.com/SampsonFox/assetloop/internal/domain"
	"github.com/SampsonFox/assetloop/internal/store/sqlite/sqlitedb"
)

func (s *Store) CreateCategory(ctx context.Context, category domain.ItemCategory) error {
	return sqlitedb.New(s.db).CreateCategory(ctx, sqlitedb.CreateCategoryParams{
		ID: category.ID, TenantID: category.TenantID, Name: category.Name, CreatedAt: sqliteTime(category.CreatedAt),
	})
}

func (s *Store) CreateModel(ctx context.Context, model domain.ProductModel) error {
	return sqlitedb.New(s.db).CreateModel(ctx, sqlitedb.CreateModelParams{
		ID: model.ID, TenantID: model.TenantID, CategoryID: model.CategoryID, Name: model.Name, CreatedAt: sqliteTime(model.CreatedAt),
	})
}

func (s *Store) CreateVariant(ctx context.Context, variant domain.ProductVariant) error {
	return sqlitedb.New(s.db).CreateVariant(ctx, sqlitedb.CreateVariantParams{
		ID: variant.ID, TenantID: variant.TenantID, ModelID: variant.ModelID, Name: variant.Name, CreatedAt: sqliteTime(variant.CreatedAt),
	})
}

func (s *Store) CreateCatalogAsset(ctx context.Context, asset domain.Asset) error {
	return sqlitedb.New(s.db).CreateCatalogAsset(ctx, sqlitedb.CreateCatalogAssetParams{
		ID: asset.ID, TenantID: asset.TenantID, VariantID: asset.VariantID,
		DisplayName: asset.DisplayName, SerialNumber: asset.SerialNumber, Color: asset.Color,
		PurchaseChannel: asset.PurchaseChannel, Notes: asset.Notes, CreatedAt: sqliteTime(asset.CreatedAt),
	})
}

func (s *Store) ListCategories(ctx context.Context, tenantID string) ([]domain.ItemCategory, error) {
	rows, err := sqlitedb.New(s.db).ListCategories(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	result := make([]domain.ItemCategory, 0, len(rows))
	for _, row := range rows {
		createdAt, err := parseCatalogTime(row.CreatedAt)
		if err != nil {
			return nil, err
		}
		result = append(result, domain.ItemCategory{ID: row.ID, TenantID: row.TenantID, Name: row.Name, CreatedAt: createdAt})
	}
	return result, nil
}

func (s *Store) ListModels(ctx context.Context, tenantID string) ([]domain.ProductModel, error) {
	rows, err := sqlitedb.New(s.db).ListModels(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	result := make([]domain.ProductModel, 0, len(rows))
	for _, row := range rows {
		createdAt, err := parseCatalogTime(row.CreatedAt)
		if err != nil {
			return nil, err
		}
		result = append(result, domain.ProductModel{
			ID: row.ID, TenantID: row.TenantID, CategoryID: row.CategoryID,
			CategoryName: row.CategoryName, Name: row.Name, CreatedAt: createdAt,
		})
	}
	return result, nil
}

func (s *Store) ListVariants(ctx context.Context, tenantID string) ([]domain.ProductVariant, error) {
	rows, err := sqlitedb.New(s.db).ListVariants(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	result := make([]domain.ProductVariant, 0, len(rows))
	for _, row := range rows {
		createdAt, err := parseCatalogTime(row.CreatedAt)
		if err != nil {
			return nil, err
		}
		result = append(result, domain.ProductVariant{
			ID: row.ID, TenantID: row.TenantID, CategoryID: row.CategoryID, CategoryName: row.CategoryName,
			ModelID: row.ModelID, ModelName: row.ModelName, Name: row.Name, CreatedAt: createdAt,
		})
	}
	return result, nil
}

func (s *Store) ListAssets(ctx context.Context, tenantID string) ([]domain.Asset, error) {
	rows, err := sqlitedb.New(s.db).ListAssets(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	result := make([]domain.Asset, 0, len(rows))
	for _, row := range rows {
		createdAt, err := parseCatalogTime(row.CreatedAt)
		if err != nil {
			return nil, err
		}
		result = append(result, domain.Asset{
			ID: row.ID, TenantID: row.TenantID, CategoryID: row.CategoryID, Category: row.CategoryName,
			ModelID: row.ModelID, Model: row.ModelName, VariantID: row.VariantID, Variant: row.VariantName,
			DisplayName: row.DisplayName, SerialNumber: row.SerialNumber, Color: row.Color,
			PurchaseChannel: row.PurchaseChannel, Notes: row.Notes, CreatedAt: createdAt,
		})
	}
	return result, nil
}

func parseCatalogTime(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse catalog time: %w", err)
	}
	return parsed, nil
}
