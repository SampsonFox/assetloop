package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/google/uuid"
	"time"

	"github.com/SampsonFox/assetloop/internal/application"
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
		model := domain.ProductModel{Model3DResourceID: row.Model3dResourceID.String,
			ID: row.ID, TenantID: row.TenantID, CategoryID: row.CategoryID,
			CategoryName: row.CategoryName, CategoryIcon: row.CategoryIcon, Name: row.Name, CreatedAt: createdAt,
		}
		model.Model3D, err = sqliteModel3D(row.Model3dResourceID, row.Model3dStoreID, row.Model3dObjectKey, row.Model3dSha256, row.Model3dSizeBytes, row.Model3dSourceUrl, row.Model3dAuthor, row.Model3dLicense, row.Model3dUpdatedAt)
		if err != nil {
			return nil, err
		}
		result = append(result, model)
	}
	return result, nil
}

func (s *Store) GetProductModel(ctx context.Context, tenantID, modelID string) (domain.ProductModel, error) {
	row, err := s.queries().GetProductModel(ctx, sqlitedb.GetProductModelParams{TenantID: tenantID, ID: modelID})
	if err != nil {
		return domain.ProductModel{}, err
	}
	createdAt, err := parseCatalogTime(row.CreatedAt)
	if err != nil {
		return domain.ProductModel{}, err
	}
	media, err := sqliteModel3D(row.Model3dResourceID, row.Model3dStoreID, row.Model3dObjectKey, row.Model3dSha256, row.Model3dSizeBytes, row.Model3dSourceUrl, row.Model3dAuthor, row.Model3dLicense, row.Model3dUpdatedAt)
	if err != nil {
		return domain.ProductModel{}, err
	}
	return domain.ProductModel{Model3DResourceID: row.Model3dResourceID.String, ID: row.ID, TenantID: row.TenantID, CategoryID: row.CategoryID, CategoryName: row.CategoryName, CategoryIcon: row.CategoryIcon, Name: row.Name, CreatedAt: createdAt, Model3D: media}, nil
}

func (s *Store) UpdateProductModel3D(ctx context.Context, tenantID, modelID string, media domain.ProductModel3D) error {
	if _, err := s.GetProductModel(ctx, tenantID, modelID); err != nil {
		return err
	}
	if media.ResourceID == "" {
		media.ResourceID = uuid.NewString()
		return s.CreateAndBindModel3DResource(ctx, domain.Model3DResource{ID: media.ResourceID, TenantID: tenantID, Name: "Model 3D", Status: "ready", ProductModel3D: media, CreatedAt: media.UpdatedAt}, application.BindModel3DResource{Kind: "model", TargetID: modelID, ResourceID: media.ResourceID})
	}
	return s.BindModel3DResource(ctx, tenantID, application.BindModel3DResource{Kind: "model", TargetID: modelID, ResourceID: media.ResourceID})
}

func sqliteModel3D(resourceID sql.NullString, storeID, objectKey, sha string, size int64, source, author, license, updated string) (*domain.ProductModel3D, error) {
	if !resourceID.Valid || storeID == "" || objectKey == "" || size == 0 {
		return nil, nil
	}
	updatedAt, err := parseCatalogTime(updated)
	if err != nil {
		return nil, err
	}
	return &domain.ProductModel3D{ResourceID: resourceID.String, StoreID: storeID, ObjectKey: objectKey, SHA256: sha, SizeBytes: size, SourceURL: source, Author: author, License: license, UpdatedAt: updatedAt}, nil
}

func nullString(value string) sql.NullString {
	return sql.NullString{String: value, Valid: value != ""}
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
		result = append(result, domain.Asset{Model3DResourceID: row.Model3dResourceID.String,
			ID: row.ID, TenantID: row.TenantID, CategoryID: row.CategoryID, Category: row.CategoryName, CategoryIcon: row.CategoryIcon,
			ModelID: row.ModelID, Model: row.ModelName,
			DisplayName: row.DisplayName, SerialNumber: row.SerialNumber,
			PurchaseChannel: row.PurchaseChannel, Notes: row.Notes, CreatedAt: createdAt,
		})
	}
	return result, nil
}

func (s *Store) ListAssetsWithSummary(ctx context.Context, tenantID string, opts application.AssetListOptions) (application.AssetListResult, error) {
	queries := sqlitedb.New(s.db)
	filter := sqlitedb.CountAssetsWithSummaryParams{TenantID: tenantID, SearchQuery: opts.Query, StatusFilter: opts.Status}
	total, err := queries.CountAssetsWithSummary(ctx, filter)
	if err != nil || total == 0 {
		return application.AssetListResult{Total: int(total)}, err
	}
	rows, err := queries.ListAssetsWithSummary(ctx, sqlitedb.ListAssetsWithSummaryParams{
		TenantID: tenantID, SearchQuery: opts.Query, StatusFilter: opts.Status,
		SortKey: opts.Sort, SortDirection: opts.Direction,
		PageSize: int64(opts.PageSize), PageOffset: int64((opts.Page - 1) * opts.PageSize),
	})
	if err != nil {
		return application.AssetListResult{}, err
	}
	result := application.AssetListResult{Assets: make([]application.AssetWithSummary, 0, len(rows)), Total: int(total)}
	for _, row := range rows {
		createdAt, err := parseCatalogTime(row.CreatedAt)
		if err != nil {
			return application.AssetListResult{}, err
		}
		result.Assets = append(result.Assets, application.AssetWithSummary{
			Asset: domain.Asset{Model3DResourceID: row.Model3dResourceID.String,
				ID: row.ID, TenantID: row.TenantID, CategoryID: row.CategoryID, Category: row.CategoryName, CategoryIcon: row.CategoryIcon,
				ModelID: row.ModelID, Model: row.ModelName,
				DisplayName: row.DisplayName, SerialNumber: row.SerialNumber,
				PurchaseChannel: row.PurchaseChannel, Notes: row.Notes, CreatedAt: createdAt,
			},
			Summary: domain.AssetSummary{
				BaseCurrency: row.BaseCurrency, ExpenseMinor: row.ExpenseMinor, IncomeMinor: row.IncomeMinor,
				NetCashflowMinor: row.NetMinor, Status: row.Status,
			},
		})
	}
	return result, nil
}

func (s *Store) ListModelsPage(ctx context.Context, tenantID string, opts application.ModelListOptions) (application.ModelListResult, error) {
	rows, err := sqlitedb.New(s.db).ListModelsPage(ctx, sqlitedb.ListModelsPageParams{
		TenantID: tenantID, SearchQuery: opts.Query, CategoryFilter: opts.CategoryID,
		SortKey: opts.Sort, SortDirection: opts.Direction,
		PageSize: int64(opts.PageSize), PageOffset: int64((opts.Page - 1) * opts.PageSize),
	})
	if err != nil {
		return application.ModelListResult{}, err
	}
	result := application.ModelListResult{Models: []domain.ProductModel{}}
	for _, row := range rows {
		if result.Total == 0 {
			result.Total = int(row.TotalCount)
		}
		createdAt, err := parseCatalogTime(row.CreatedAt)
		if err != nil {
			return application.ModelListResult{}, err
		}
		model := domain.ProductModel{Model3DResourceID: row.Model3dResourceID.String,
			ID: row.ID, TenantID: row.TenantID, CategoryID: row.CategoryID,
			CategoryName: row.CategoryName, CategoryIcon: row.CategoryIcon, Name: row.Name, CreatedAt: createdAt,
		}
		model.Model3D, err = sqliteModel3D(row.Model3dResourceID, row.Model3dStoreID, row.Model3dObjectKey, row.Model3dSha256, row.Model3dSizeBytes, row.Model3dSourceUrl, row.Model3dAuthor, row.Model3dLicense, row.Model3dUpdatedAt)
		if err != nil {
			return application.ModelListResult{}, err
		}
		result.Models = append(result.Models, model)
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
