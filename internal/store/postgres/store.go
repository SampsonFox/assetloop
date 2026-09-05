package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/SampsonFox/assetloop/internal/domain"
	"github.com/SampsonFox/assetloop/internal/store/postgres/postgresdb"
	"github.com/google/uuid"
)

type Store struct {
	db *sql.DB
	tx *sql.Tx
}

func (s *Store) queries() *postgresdb.Queries {
	if s.tx != nil {
		return postgresdb.New(s.tx)
	}
	return postgresdb.New(s.db)
}

func New(db *sql.DB) *Store { return &Store{db: db} }

func (s *Store) CreateAsset(ctx context.Context, asset domain.Asset) (domain.Asset, error) {
	ids, err := parseIDs(asset)
	if err != nil {
		return domain.Asset{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Asset{}, err
	}
	defer tx.Rollback()
	q := postgresdb.New(tx)
	if err := q.EnsureTenant(ctx, postgresdb.EnsureTenantParams{ID: ids.tenant, Name: "Default", BaseCurrency: "CNY", CreatedAt: asset.CreatedAt}); err != nil {
		return domain.Asset{}, err
	}
	ids.category, err = q.EnsureCategory(ctx, postgresdb.EnsureCategoryParams{ID: ids.category, TenantID: ids.tenant, Name: asset.Category, CreatedAt: asset.CreatedAt})
	if err != nil {
		return domain.Asset{}, err
	}
	ids.model, err = q.EnsureModel(ctx, postgresdb.EnsureModelParams{ID: ids.model, TenantID: ids.tenant, CategoryID: ids.category, Name: asset.Model, CreatedAt: asset.CreatedAt})
	if err != nil {
		return domain.Asset{}, err
	}
	ids.variant, err = q.EnsureVariant(ctx, postgresdb.EnsureVariantParams{ID: ids.variant, TenantID: ids.tenant, ModelID: ids.model, Name: asset.Variant, CreatedAt: asset.CreatedAt})
	if err != nil {
		return domain.Asset{}, err
	}
	if err := q.CreateAsset(ctx, postgresdb.CreateAssetParams{ID: ids.asset, TenantID: ids.tenant, VariantID: ids.variant, DisplayName: asset.DisplayName, CreatedAt: asset.CreatedAt}); err != nil {
		return domain.Asset{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.Asset{}, err
	}
	asset.CategoryID = ids.category.String()
	asset.ModelID = ids.model.String()
	asset.VariantID = ids.variant.String()
	return asset, nil
}

func (s *Store) GetAsset(ctx context.Context, tenantID, assetID string) (domain.Asset, error) {
	tenantUUID, err := uuid.Parse(tenantID)
	if err != nil {
		return domain.Asset{}, fmt.Errorf("parse tenant ID: %w", err)
	}
	assetUUID, err := uuid.Parse(assetID)
	if err != nil {
		return domain.Asset{}, fmt.Errorf("parse asset ID: %w", err)
	}
	row, err := s.queries().GetAsset(ctx, postgresdb.GetAssetParams{TenantID: tenantUUID, ID: assetUUID})
	if err != nil {
		return domain.Asset{}, err
	}
	return domain.Asset{
		ID: row.ID.String(), TenantID: row.TenantID.String(),
		CategoryID: row.CategoryID.String(), Category: row.CategoryName, CategoryIcon: row.CategoryIcon,
		ModelID: row.ModelID.String(), Model: row.ModelName,
		VariantID: row.VariantID.String(), Variant: row.VariantName,
		DisplayName: row.DisplayName, SerialNumber: row.SerialNumber, Color: row.Color,
		PurchaseChannel: row.PurchaseChannel, Notes: row.Notes, CreatedAt: row.CreatedAt,
	}, nil
}

type assetIDs struct {
	asset, tenant, category, model, variant uuid.UUID
}

func parseIDs(asset domain.Asset) (assetIDs, error) {
	values := []string{asset.ID, asset.TenantID, asset.CategoryID, asset.ModelID, asset.VariantID}
	parsed := make([]uuid.UUID, len(values))
	for i, value := range values {
		id, err := uuid.Parse(value)
		if err != nil {
			return assetIDs{}, fmt.Errorf("parse asset identifiers: %w", err)
		}
		parsed[i] = id
	}
	return assetIDs{asset: parsed[0], tenant: parsed[1], category: parsed[2], model: parsed[3], variant: parsed[4]}, nil
}
