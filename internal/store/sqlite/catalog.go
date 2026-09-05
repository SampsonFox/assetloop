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
		model := domain.ProductModel{
			ID: row.ID, TenantID: row.TenantID, CategoryID: row.CategoryID,
			CategoryName: row.CategoryName, CategoryIcon: row.CategoryIcon, Name: row.Name, CreatedAt: createdAt,
		}
		model.Model3D, err = sqliteModel3D(row.Model3dStoreID, row.Model3dObjectKey, row.Model3dSha256, row.Model3dSizeBytes, row.Model3dSourceUrl, row.Model3dAuthor, row.Model3dLicense, row.Model3dUpdatedAt)
		if err != nil {
			return nil, err
		}
		result = append(result, model)
	}
	return result, nil
}

func (s *Store) GetProductModel(ctx context.Context, tenantID, modelID string) (domain.ProductModel, error) {
	row, err := sqlitedb.New(s.db).GetProductModel(ctx, sqlitedb.GetProductModelParams{TenantID: tenantID, ID: modelID})
	if err != nil {
		return domain.ProductModel{}, err
	}
	createdAt, err := parseCatalogTime(row.CreatedAt)
	if err != nil {
		return domain.ProductModel{}, err
	}
	media, err := sqliteModel3D(row.Model3dStoreID, row.Model3dObjectKey, row.Model3dSha256, row.Model3dSizeBytes, row.Model3dSourceUrl, row.Model3dAuthor, row.Model3dLicense, row.Model3dUpdatedAt)
	if err != nil {
		return domain.ProductModel{}, err
	}
	return domain.ProductModel{ID: row.ID, TenantID: row.TenantID, CategoryID: row.CategoryID, CategoryName: row.CategoryName, CategoryIcon: row.CategoryIcon, Name: row.Name, CreatedAt: createdAt, Model3D: media}, nil
}

func (s *Store) UpdateProductModel3D(ctx context.Context, tenantID, modelID string, media domain.ProductModel3D) error {
	count, err := sqlitedb.New(s.db).UpdateProductModel3D(ctx, sqlitedb.UpdateProductModel3DParams{
		Model3dStoreID: nullString(media.StoreID), Model3dObjectKey: nullString(media.ObjectKey), Model3dSha256: nullString(media.SHA256),
		Model3dSizeBytes: sql.NullInt64{Int64: media.SizeBytes, Valid: media.SizeBytes > 0}, Model3dSourceUrl: nullString(media.SourceURL),
		Model3dAuthor: nullString(media.Author), Model3dLicense: nullString(media.License), Model3dUpdatedAt: nullString(sqliteTime(media.UpdatedAt)), TenantID: tenantID, ID: modelID,
	})
	return updatedRow(count, err)
}

func sqliteModel3D(storeID, objectKey, sha sql.NullString, size sql.NullInt64, source, author, license, updated sql.NullString) (*domain.ProductModel3D, error) {
	if !storeID.Valid || !objectKey.Valid || !sha.Valid || !size.Valid {
		return nil, nil
	}
	updatedAt := time.Time{}
	var err error
	if updated.Valid {
		updatedAt, err = parseCatalogTime(updated.String)
		if err != nil {
			return nil, err
		}
	}
	return &domain.ProductModel3D{StoreID: storeID.String, ObjectKey: objectKey.String, SHA256: sha.String, SizeBytes: size.Int64, SourceURL: source.String, Author: author.String, License: license.String, UpdatedAt: updatedAt}, nil
}

func nullString(value string) sql.NullString {
	return sql.NullString{String: value, Valid: value != ""}
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
