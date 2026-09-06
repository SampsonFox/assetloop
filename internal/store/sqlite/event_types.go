package sqlite

import (
	"context"
	"database/sql"
	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/SampsonFox/assetloop/internal/domain"
	"github.com/SampsonFox/assetloop/internal/store/sqlite/sqlitedb"
)

func (s *Store) UpdateAssetEventType(ctx context.Context, item domain.AssetEventTypeDefinition) error {
	id, tenantID := item.ID, item.TenantID
	enabled := int64(0)
	if item.Enabled {
		enabled = 1
	}
	n, err := s.queries().UpdateAssetEventType(ctx, sqlitedb.UpdateAssetEventTypeParams{ID: id, TenantID: tenantID, Name: item.Name, NormalizedName: item.NormalizedName, CashflowDirection: string(item.Cashflow), Enabled: enabled, UpdatedAt: sqliteTime(item.UpdatedAt)})
	if err != nil {
		return err
	}
	if n != 1 {
		return sql.ErrNoRows
	}
	return nil
}
func (s *Store) ListAssetEventTypesPage(ctx context.Context, tenant string, opts application.EventTypeListOptions) ([]domain.AssetEventTypeDefinition, int, error) {
	tenantID := tenant
	rows, err := s.queries().ListAssetEventTypesPage(ctx, sqlitedb.ListAssetEventTypesPageParams{TenantID: tenantID, SearchQuery: opts.Query, StatusFilter: opts.Status, PageSize: int64(opts.PageSize), PageOffset: int64((opts.Page - 1) * opts.PageSize)})
	if err != nil {
		return nil, 0, err
	}
	items := make([]domain.AssetEventTypeDefinition, 0, len(rows))
	total := 0
	for _, row := range rows {
		total = int(row.TotalCount)
		items = append(items, domain.AssetEventTypeDefinition{ID: row.ID, TenantID: tenant, Name: row.Name, NormalizedName: row.NormalizedName, Cashflow: domain.AssetEventCashflow(row.CashflowDirection), SystemCode: domain.AssetEventType(row.SystemCode), BuiltIn: row.SystemCode != "", ReferenceCount: row.ReferenceCount, Enabled: row.Enabled != 0})
	}
	return items, total, nil
}
