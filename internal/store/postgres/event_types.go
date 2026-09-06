package postgres

import (
	"context"
	"database/sql"
	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/SampsonFox/assetloop/internal/domain"
	"github.com/SampsonFox/assetloop/internal/store/postgres/postgresdb"
	"github.com/google/uuid"
)

func (s *Store) UpdateAssetEventType(ctx context.Context, item domain.AssetEventTypeDefinition) error {
	id, tenantID, err := catalogIDs(item.ID, item.TenantID)
	if err != nil {
		return err
	}
	n, err := s.queries().UpdateAssetEventType(ctx, postgresdb.UpdateAssetEventTypeParams{ID: id, TenantID: tenantID, Name: item.Name, NormalizedName: item.NormalizedName, CashflowDirection: string(item.Cashflow), Enabled: item.Enabled, UpdatedAt: item.UpdatedAt})
	if err != nil {
		return err
	}
	if n != 1 {
		return sql.ErrNoRows
	}
	return nil
}
func (s *Store) ListAssetEventTypesPage(ctx context.Context, tenant string, opts application.EventTypeListOptions) ([]domain.AssetEventTypeDefinition, int, error) {
	tenantID, err := uuid.Parse(tenant)
	if err != nil {
		return nil, 0, err
	}
	rows, err := s.queries().ListAssetEventTypesPage(ctx, postgresdb.ListAssetEventTypesPageParams{TenantID: tenantID, SearchQuery: opts.Query, StatusFilter: opts.Status, PageSize: int32(opts.PageSize), PageOffset: int32((opts.Page - 1) * opts.PageSize)})
	if err != nil {
		return nil, 0, err
	}
	items := make([]domain.AssetEventTypeDefinition, 0, len(rows))
	total := 0
	for _, row := range rows {
		total = int(row.TotalCount)
		items = append(items, domain.AssetEventTypeDefinition{ID: row.ID.String(), TenantID: tenant, Name: row.Name, NormalizedName: row.NormalizedName, Cashflow: domain.AssetEventCashflow(row.CashflowDirection), SystemCode: domain.AssetEventType(row.SystemCode), BuiltIn: row.SystemCode != "", ReferenceCount: row.ReferenceCount, Enabled: row.Enabled})
	}
	return items, total, nil
}
