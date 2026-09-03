package postgres

import (
	"context"
	"database/sql"
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
		ID: id, TenantID: tenantID, Name: category.Name, IconKey: category.IconKey, CreatedAt: category.CreatedAt,
	})
}

func (s *Store) UpdateCategory(ctx context.Context, category domain.ItemCategory) error {
	id, tenantID, err := catalogIDs(category.ID, category.TenantID)
	if err != nil {
		return err
	}
	count, err := postgresdb.New(s.db).UpdateCategory(ctx, postgresdb.UpdateCategoryParams{Name: category.Name, IconKey: category.IconKey, TenantID: tenantID, ID: id})
	return updatedRow(count, err)
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

func (s *Store) UpdateModel(ctx context.Context, model domain.ProductModel) error {
	id, tenantID, err := catalogIDs(model.ID, model.TenantID)
	if err != nil {
		return err
	}
	categoryID, err := uuid.Parse(model.CategoryID)
	if err != nil {
		return fmt.Errorf("parse category ID: %w", err)
	}
	count, err := postgresdb.New(s.db).UpdateModel(ctx, postgresdb.UpdateModelParams{CategoryID: categoryID, Name: model.Name, TenantID: tenantID, ID: id})
	return updatedRow(count, err)
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

func (s *Store) UpdateVariant(ctx context.Context, variant domain.ProductVariant) error {
	id, tenantID, err := catalogIDs(variant.ID, variant.TenantID)
	if err != nil {
		return err
	}
	modelID, err := uuid.Parse(variant.ModelID)
	if err != nil {
		return fmt.Errorf("parse model ID: %w", err)
	}
	count, err := postgresdb.New(s.db).UpdateVariant(ctx, postgresdb.UpdateVariantParams{ModelID: modelID, Name: variant.Name, TenantID: tenantID, ID: id})
	return updatedRow(count, err)
}

func (s *Store) DeleteVariant(ctx context.Context, tenantID, variantID string) (bool, error) {
	id, parsedTenantID, err := catalogIDs(variantID, tenantID)
	if err != nil {
		return false, err
	}
	count, err := postgresdb.New(s.db).DeleteVariant(ctx, postgresdb.DeleteVariantParams{TenantID: parsedTenantID, ID: id})
	return count > 0, err
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

func (s *Store) UpdateCatalogAsset(ctx context.Context, asset domain.Asset) error {
	id, tenantID, err := catalogIDs(asset.ID, asset.TenantID)
	if err != nil {
		return err
	}
	variantID, err := uuid.Parse(asset.VariantID)
	if err != nil {
		return fmt.Errorf("parse variant ID: %w", err)
	}
	count, err := postgresdb.New(s.db).UpdateCatalogAsset(ctx, postgresdb.UpdateCatalogAssetParams{
		VariantID: variantID, DisplayName: asset.DisplayName, SerialNumber: asset.SerialNumber,
		Color: asset.Color, PurchaseChannel: asset.PurchaseChannel, Notes: asset.Notes,
		TenantID: tenantID, ID: id,
	})
	return updatedRow(count, err)
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
		result = append(result, domain.ItemCategory{ID: row.ID.String(), TenantID: row.TenantID.String(), Name: row.Name, IconKey: row.IconKey, CreatedAt: row.CreatedAt})
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
			CategoryName: row.CategoryName, CategoryIcon: row.CategoryIcon, Name: row.Name, CreatedAt: row.CreatedAt,
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
			CategoryName: row.CategoryName, CategoryIcon: row.CategoryIcon, ModelID: row.ModelID.String(), ModelName: row.ModelName,
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
			ID: row.ID.String(), TenantID: row.TenantID.String(), CategoryID: row.CategoryID.String(), Category: row.CategoryName, CategoryIcon: row.CategoryIcon,
			ModelID: row.ModelID.String(), Model: row.ModelName, VariantID: row.VariantID.String(), Variant: row.VariantName,
			DisplayName: row.DisplayName, SerialNumber: row.SerialNumber, Color: row.Color,
			PurchaseChannel: row.PurchaseChannel, Notes: row.Notes, CreatedAt: row.CreatedAt,
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
