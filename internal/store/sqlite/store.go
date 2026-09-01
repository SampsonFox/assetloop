package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/SampsonFox/assetloop/internal/domain"
	"github.com/SampsonFox/assetloop/internal/store/sqlite/sqlitedb"
)

type Store struct {
	db *sql.DB
}

func New(db *sql.DB) *Store { return &Store{db: db} }

func (s *Store) CreateAsset(ctx context.Context, asset domain.Asset) (domain.Asset, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Asset{}, err
	}
	defer tx.Rollback()
	q := sqlitedb.New(tx)
	createdAt := asset.CreatedAt.UTC().Format(time.RFC3339Nano)
	if err := q.EnsureTenant(ctx, sqlitedb.EnsureTenantParams{ID: asset.TenantID, Name: "Default", BaseCurrency: "CNY", CreatedAt: createdAt}); err != nil {
		return domain.Asset{}, err
	}
	asset.CategoryID, err = q.EnsureCategory(ctx, sqlitedb.EnsureCategoryParams{ID: asset.CategoryID, TenantID: asset.TenantID, Name: asset.Category, CreatedAt: createdAt})
	if err != nil {
		return domain.Asset{}, err
	}
	asset.ModelID, err = q.EnsureModel(ctx, sqlitedb.EnsureModelParams{ID: asset.ModelID, TenantID: asset.TenantID, CategoryID: asset.CategoryID, Name: asset.Model, CreatedAt: createdAt})
	if err != nil {
		return domain.Asset{}, err
	}
	asset.VariantID, err = q.EnsureVariant(ctx, sqlitedb.EnsureVariantParams{ID: asset.VariantID, TenantID: asset.TenantID, ModelID: asset.ModelID, Name: asset.Variant, CreatedAt: createdAt})
	if err != nil {
		return domain.Asset{}, err
	}
	if err := q.CreateAsset(ctx, sqlitedb.CreateAssetParams{ID: asset.ID, TenantID: asset.TenantID, VariantID: asset.VariantID, DisplayName: asset.DisplayName, CreatedAt: createdAt}); err != nil {
		return domain.Asset{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.Asset{}, err
	}
	return asset, nil
}

func (s *Store) GetAsset(ctx context.Context, tenantID, assetID string) (domain.Asset, error) {
	row, err := sqlitedb.New(s.db).GetAsset(ctx, sqlitedb.GetAssetParams{TenantID: tenantID, ID: assetID})
	if err != nil {
		return domain.Asset{}, err
	}
	createdAt, err := time.Parse(time.RFC3339Nano, row.CreatedAt)
	if err != nil {
		return domain.Asset{}, fmt.Errorf("parse created_at: %w", err)
	}
	return domain.Asset{
		ID: row.ID, TenantID: row.TenantID,
		CategoryID: row.CategoryID, Category: row.CategoryName,
		ModelID: row.ModelID, Model: row.ModelName,
		VariantID: row.VariantID, Variant: row.VariantName,
		DisplayName: row.DisplayName, CreatedAt: createdAt,
	}, nil
}
