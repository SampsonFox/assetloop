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
		Model3DResourceID: optionalUUID(row.Model3dResourceID),
		DisplayName:       row.DisplayName, SerialNumber: row.SerialNumber,
		PurchaseChannel: row.PurchaseChannel, Notes: row.Notes, CreatedAt: row.CreatedAt,
	}, nil
}
