package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/SampsonFox/assetloop/internal/domain"
	"github.com/SampsonFox/assetloop/internal/store/sqlite/sqlitedb"
)

func (s *Store) CreateCategory(ctx context.Context, category domain.ItemCategory) error {
	return sqlitedb.New(s.db).CreateCategory(ctx, sqlitedb.CreateCategoryParams{
		ID: category.ID, TenantID: category.TenantID, Name: category.Name, IconKey: category.IconKey, CreatedAt: sqliteTime(category.CreatedAt),
	})
}

func (s *Store) UpdateCategory(ctx context.Context, category domain.ItemCategory) error {
	count, err := sqlitedb.New(s.db).UpdateCategory(ctx, sqlitedb.UpdateCategoryParams{Name: category.Name, IconKey: category.IconKey, TenantID: category.TenantID, ID: category.ID})
	return updatedRow(count, err)
}

func (s *Store) CreateModel(ctx context.Context, model domain.ProductModel) error {
	return sqlitedb.New(s.db).CreateModel(ctx, sqlitedb.CreateModelParams{
		ID: model.ID, TenantID: model.TenantID, CategoryID: model.CategoryID, Name: model.Name, CreatedAt: sqliteTime(model.CreatedAt),
	})
}

func (s *Store) UpdateModel(ctx context.Context, model domain.ProductModel) error {
	count, err := sqlitedb.New(s.db).UpdateModel(ctx, sqlitedb.UpdateModelParams{CategoryID: model.CategoryID, Name: model.Name, TenantID: model.TenantID, ID: model.ID})
	return updatedRow(count, err)
}

func (s *Store) CreateVariant(ctx context.Context, variant domain.ProductVariant) error {
	return sqlitedb.New(s.db).CreateVariant(ctx, sqlitedb.CreateVariantParams{
		ID: variant.ID, TenantID: variant.TenantID, ModelID: variant.ModelID, Name: variant.Name, CreatedAt: sqliteTime(variant.CreatedAt),
	})
}

func (s *Store) UpdateVariant(ctx context.Context, variant domain.ProductVariant) error {
	count, err := sqlitedb.New(s.db).UpdateVariant(ctx, sqlitedb.UpdateVariantParams{ModelID: variant.ModelID, Name: variant.Name, TenantID: variant.TenantID, ID: variant.ID})
	return updatedRow(count, err)
}

func (s *Store) DeleteVariant(ctx context.Context, tenantID, variantID string) (bool, error) {
	count, err := sqlitedb.New(s.db).DeleteVariant(ctx, sqlitedb.DeleteVariantParams{TenantID: tenantID, ID: variantID})
	return count > 0, err
}

func (s *Store) CreateCatalogAsset(ctx context.Context, asset domain.Asset) error {
	return sqlitedb.New(s.db).CreateCatalogAsset(ctx, sqlitedb.CreateCatalogAssetParams{
		ID: asset.ID, TenantID: asset.TenantID, VariantID: asset.VariantID,
		DisplayName: asset.DisplayName, SerialNumber: asset.SerialNumber, Color: asset.Color,
		PurchaseChannel: asset.PurchaseChannel, Notes: asset.Notes, CreatedAt: sqliteTime(asset.CreatedAt),
	})
}

func (s *Store) UpdateCatalogAsset(ctx context.Context, asset domain.Asset) error {
	count, err := sqlitedb.New(s.db).UpdateCatalogAsset(ctx, sqlitedb.UpdateCatalogAssetParams{
		VariantID: asset.VariantID, DisplayName: asset.DisplayName, SerialNumber: asset.SerialNumber,
		Color: asset.Color, PurchaseChannel: asset.PurchaseChannel, Notes: asset.Notes,
		TenantID: asset.TenantID, ID: asset.ID,
	})
	return updatedRow(count, err)
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
		result = append(result, domain.ItemCategory{ID: row.ID, TenantID: row.TenantID, Name: row.Name, IconKey: row.IconKey, CreatedAt: createdAt})
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
			CategoryName: row.CategoryName, CategoryIcon: row.CategoryIcon, Name: row.Name, CreatedAt: createdAt,
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
			ID: row.ID, TenantID: row.TenantID, CategoryID: row.CategoryID, CategoryName: row.CategoryName, CategoryIcon: row.CategoryIcon,
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
			ID: row.ID, TenantID: row.TenantID, CategoryID: row.CategoryID, Category: row.CategoryName, CategoryIcon: row.CategoryIcon,
			ModelID: row.ModelID, Model: row.ModelName, VariantID: row.VariantID, Variant: row.VariantName,
			DisplayName: row.DisplayName, SerialNumber: row.SerialNumber, Color: row.Color,
			PurchaseChannel: row.PurchaseChannel, Notes: row.Notes, CreatedAt: createdAt,
		})
	}
	return result, nil
}

func updatedRow(count int64, err error) error {
	if err != nil {
		return err
	}
	if count == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func parseCatalogTime(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse catalog time: %w", err)
	}
	return parsed, nil
}
