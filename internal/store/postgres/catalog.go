package postgres

import (
	"context"
	"fmt"

	"github.com/SampsonFox/assetloop/internal/domain"
	"github.com/SampsonFox/assetloop/internal/store/postgres/postgresdb"
	"github.com/google/uuid"
)

func (s *Store) CreateCategory(ctx context.Context, category domain.ItemCategory) error {
	id, tenantID, err := catalogIDs(category.ID, category.TenantID)
	if err != nil {
		return err
	}
	return postgresdb.New(s.db).CreateCategory(ctx, postgresdb.CreateCategoryParams{
		ID: id, TenantID: tenantID, Name: category.Name, CreatedAt: category.CreatedAt,
	})
}

func (s *Store) CreateModel(ctx context.Context, model domain.ProductModel) error {
	id, tenantID, err := catalogIDs(model.ID, model.TenantID)
	if err != nil {
		return err
	}
	categoryID, err := uuid.Parse(model.CategoryID)
	if err != nil {
		return fmt.Errorf("parse category ID: %w", err)
	}
	return postgresdb.New(s.db).CreateModel(ctx, postgresdb.CreateModelParams{
		ID: id, TenantID: tenantID, CategoryID: categoryID, Name: model.Name, CreatedAt: model.CreatedAt,
	})
}

func (s *Store) CreateVariant(ctx context.Context, variant domain.ProductVariant) error {
	id, tenantID, err := catalogIDs(variant.ID, variant.TenantID)
	if err != nil {
		return err
	}
	modelID, err := uuid.Parse(variant.ModelID)
	if err != nil {
		return fmt.Errorf("parse model ID: %w", err)
	}
	return postgresdb.New(s.db).CreateVariant(ctx, postgresdb.CreateVariantParams{
		ID: id, TenantID: tenantID, ModelID: modelID, Name: variant.Name, CreatedAt: variant.CreatedAt,
	})
}

func (s *Store) CreateCatalogAsset(ctx context.Context, asset domain.Asset) error {
	id, tenantID, err := catalogIDs(asset.ID, asset.TenantID)
	if err != nil {
		return err
	}
	variantID, err := uuid.Parse(asset.VariantID)
	if err != nil {
		return fmt.Errorf("parse variant ID: %w", err)
	}
	return postgresdb.New(s.db).CreateCatalogAsset(ctx, postgresdb.CreateCatalogAssetParams{
		ID: id, TenantID: tenantID, VariantID: variantID, DisplayName: asset.DisplayName,
		SerialNumber: asset.SerialNumber, Color: asset.Color, PurchaseChannel: asset.PurchaseChannel,
		Notes: asset.Notes, CreatedAt: asset.CreatedAt,
	})
}

func (s *Store) ListCategories(ctx context.Context, tenantID string) ([]domain.ItemCategory, error) {
	tenantUUID, err := uuid.Parse(tenantID)
	if err != nil {
		return nil, fmt.Errorf("parse tenant ID: %w", err)
	}
	rows, err := postgresdb.New(s.db).ListCategories(ctx, tenantUUID)
	if err != nil {
		return nil, err
	}
	result := make([]domain.ItemCategory, 0, len(rows))
	for _, row := range rows {
		result = append(result, domain.ItemCategory{ID: row.ID.String(), TenantID: row.TenantID.String(), Name: row.Name, CreatedAt: row.CreatedAt})
	}
	return result, nil
}

func (s *Store) ListModels(ctx context.Context, tenantID string) ([]domain.ProductModel, error) {
	tenantUUID, err := uuid.Parse(tenantID)
	if err != nil {
		return nil, fmt.Errorf("parse tenant ID: %w", err)
	}
	rows, err := postgresdb.New(s.db).ListModels(ctx, tenantUUID)
	if err != nil {
		return nil, err
	}
	result := make([]domain.ProductModel, 0, len(rows))
	for _, row := range rows {
		result = append(result, domain.ProductModel{
			ID: row.ID.String(), TenantID: row.TenantID.String(), CategoryID: row.CategoryID.String(),
			CategoryName: row.CategoryName, Name: row.Name, CreatedAt: row.CreatedAt,
		})
	}
	return result, nil
}

func (s *Store) ListVariants(ctx context.Context, tenantID string) ([]domain.ProductVariant, error) {
	tenantUUID, err := uuid.Parse(tenantID)
	if err != nil {
		return nil, fmt.Errorf("parse tenant ID: %w", err)
	}
	rows, err := postgresdb.New(s.db).ListVariants(ctx, tenantUUID)
	if err != nil {
		return nil, err
	}
	result := make([]domain.ProductVariant, 0, len(rows))
	for _, row := range rows {
		result = append(result, domain.ProductVariant{
			ID: row.ID.String(), TenantID: row.TenantID.String(), CategoryID: row.CategoryID.String(),
			CategoryName: row.CategoryName, ModelID: row.ModelID.String(), ModelName: row.ModelName,
			Name: row.Name, CreatedAt: row.CreatedAt,
		})
	}
	return result, nil
}

func (s *Store) ListAssets(ctx context.Context, tenantID string) ([]domain.Asset, error) {
	tenantUUID, err := uuid.Parse(tenantID)
	if err != nil {
		return nil, fmt.Errorf("parse tenant ID: %w", err)
	}
	rows, err := postgresdb.New(s.db).ListAssets(ctx, tenantUUID)
	if err != nil {
		return nil, err
	}
	result := make([]domain.Asset, 0, len(rows))
	for _, row := range rows {
		result = append(result, domain.Asset{
			ID: row.ID.String(), TenantID: row.TenantID.String(), CategoryID: row.CategoryID.String(), Category: row.CategoryName,
			ModelID: row.ModelID.String(), Model: row.ModelName, VariantID: row.VariantID.String(), Variant: row.VariantName,
			DisplayName: row.DisplayName, SerialNumber: row.SerialNumber, Color: row.Color,
			PurchaseChannel: row.PurchaseChannel, Notes: row.Notes, CreatedAt: row.CreatedAt,
		})
	}
	return result, nil
}

func catalogIDs(id, tenantID string) (uuid.UUID, uuid.UUID, error) {
	parsedID, err := uuid.Parse(id)
	if err != nil {
		return uuid.Nil, uuid.Nil, fmt.Errorf("parse ID: %w", err)
	}
	parsedTenantID, err := uuid.Parse(tenantID)
	if err != nil {
		return uuid.Nil, uuid.Nil, fmt.Errorf("parse tenant ID: %w", err)
	}
	return parsedID, parsedTenantID, nil
}
