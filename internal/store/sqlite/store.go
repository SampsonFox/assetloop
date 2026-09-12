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
	db               *sql.DB
	tx               *sql.Tx
	managementTenant string
}

func (s *Store) queries() *sqlitedb.Queries {
	if s.tx != nil {
		return sqlitedb.New(s.tx)
	}
	return sqlitedb.New(s.db)
}

func New(db *sql.DB) *Store { return &Store{db: db} }

func (s *Store) GetAsset(ctx context.Context, tenantID, assetID string) (domain.Asset, error) {
	row, err := s.queries().GetAsset(ctx, sqlitedb.GetAssetParams{TenantID: tenantID, ID: assetID})
	if err != nil {
		return domain.Asset{}, err
	}
	createdAt, err := time.Parse(time.RFC3339Nano, row.CreatedAt)
	if err != nil {
		return domain.Asset{}, fmt.Errorf("parse created_at: %w", err)
	}
	return domain.Asset{
		MarketItemID: row.MarketItemID,
		ID:           row.ID, TenantID: row.TenantID,
		CategoryID: row.CategoryID, Category: row.CategoryName, CategoryIcon: row.CategoryIcon,
		ModelID: row.ModelID, Model: row.ModelName,
		Model3DResourceID: row.Model3dResourceID.String,
		DisplayName:       row.DisplayName, SerialNumber: row.SerialNumber,
		PurchaseChannel: row.PurchaseChannel, Notes: row.Notes, CreatedAt: createdAt,
	}, nil
}
