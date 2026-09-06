package sqlite

import (
	"context"
	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/SampsonFox/assetloop/internal/domain"
	"github.com/SampsonFox/assetloop/internal/store/sqlite/sqlitedb"
)

func (s *Store) ListSpecificationTypes(ctx context.Context, tenant string, opts application.SpecificationListOptions) (application.SpecificationTypeList, error) {
	result := application.SpecificationTypeList{}
	tenantID := tenant
	q := s.queries()
	total, err := q.CountSpecificationTypes(ctx, sqlitedb.CountSpecificationTypesParams{TenantID: tenantID, SearchQuery: opts.Query, StatusFilter: opts.Status})
	if err != nil {
		return result, err
	}
	result.Total = int(total)
	rows, err := q.ListSpecificationTypes(ctx, sqlitedb.ListSpecificationTypesParams{TenantID: tenantID, SearchQuery: opts.Query, StatusFilter: opts.Status, PageSize: int64(opts.PageSize), PageOffset: int64((opts.Page - 1) * opts.PageSize)})
	if err != nil {
		return result, err
	}
	for _, row := range rows {
		created, err := parseCatalogTime(row.CreatedAt)
		if err != nil {
			return result, err
		}
		updated, err := parseCatalogTime(row.UpdatedAt)
		if err != nil {
			return result, err
		}
		result.Types = append(result.Types, application.SpecificationTypeSummary{
			SpecificationTagType: domain.SpecificationTagType{
				ID: row.ID, TenantID: tenant, Name: row.Name, NormalizedName: row.NormalizedName,
				Enabled: row.Enabled != 0, CreatedAt: created, UpdatedAt: updated,
				SystemCode: row.SystemCode, Multiple: row.Multiple != 0, AffectsAppearance: row.AffectsAppearance != 0,
			},
			ReferenceCount: int(row.ReferenceCount),
		})
	}
	return result, nil
}

func (s *Store) ListSpecificationTags(ctx context.Context, tenant string, opts application.SpecificationListOptions) (application.SpecificationTagList, error) {
	result := application.SpecificationTagList{}
	tenantID := tenant
	q := s.queries()
	total, err := q.CountSpecificationTags(ctx, sqlitedb.CountSpecificationTagsParams{TenantID: tenantID, SearchQuery: opts.Query, StatusFilter: opts.Status, TypeFilter: opts.TypeID})
	if err != nil {
		return result, err
	}
	result.Total = int(total)
	rows, err := q.ListSpecificationTags(ctx, sqlitedb.ListSpecificationTagsParams{TenantID: tenantID, SearchQuery: opts.Query, StatusFilter: opts.Status, TypeFilter: opts.TypeID, PageSize: int64(opts.PageSize), PageOffset: int64((opts.Page - 1) * opts.PageSize)})
	if err != nil {
		return result, err
	}
	for _, row := range rows {
		created, err := parseCatalogTime(row.CreatedAt)
		if err != nil {
			return result, err
		}
		updated, err := parseCatalogTime(row.UpdatedAt)
		if err != nil {
			return result, err
		}
		result.Tags = append(result.Tags, application.SpecificationTagSummary{
			SpecificationTag: domain.SpecificationTag{
				ID: row.ID, TenantID: tenant, Name: row.Name, NormalizedName: row.NormalizedName,
				Enabled: row.Enabled != 0, CreatedAt: created, UpdatedAt: updated,
				TypeID: row.TypeID,
			},
			ReferenceCount: int(row.ReferenceCount), TypeName: row.TypeName,
		})
	}
	return result, nil
}
