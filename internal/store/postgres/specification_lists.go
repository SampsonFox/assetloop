package postgres

import (
	"context"
	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/SampsonFox/assetloop/internal/domain"
	"github.com/SampsonFox/assetloop/internal/store/postgres/postgresdb"
	"github.com/google/uuid"
)

func (s *Store) ListSpecificationTypes(ctx context.Context, tenant string, opts application.SpecificationListOptions) (application.SpecificationTypeList, error) {
	result := application.SpecificationTypeList{}
	tenantID, err := uuid.Parse(tenant)
	if err != nil {
		return result, err
	}
	q := s.queries()
	total, err := q.CountSpecificationTypes(ctx, postgresdb.CountSpecificationTypesParams{TenantID: tenantID, SearchQuery: opts.Query, StatusFilter: opts.Status})
	if err != nil {
		return result, err
	}
	result.Total = int(total)
	rows, err := q.ListSpecificationTypes(ctx, postgresdb.ListSpecificationTypesParams{TenantID: tenantID, SearchQuery: opts.Query, StatusFilter: opts.Status, PageSize: int32(opts.PageSize), PageOffset: int32((opts.Page - 1) * opts.PageSize)})
	if err != nil {
		return result, err
	}
	for _, row := range rows {
		created, updated := row.CreatedAt, row.UpdatedAt
		result.Types = append(result.Types, application.SpecificationTypeSummary{
			SpecificationTagType: domain.SpecificationTagType{
				ID: row.ID, TenantID: tenant, Name: row.Name, NormalizedName: row.NormalizedName,
				Enabled: row.Enabled, CreatedAt: created, UpdatedAt: updated,
				SystemCode: row.SystemCode, Multiple: row.Multiple, AffectsAppearance: row.AffectsAppearance,
			},
			ReferenceCount: int(row.ReferenceCount),
		})
	}
	return result, nil
}

func (s *Store) ListSpecificationTags(ctx context.Context, tenant string, opts application.SpecificationListOptions) (application.SpecificationTagList, error) {
	result := application.SpecificationTagList{}
	tenantID, err := uuid.Parse(tenant)
	if err != nil {
		return result, err
	}
	q := s.queries()
	total, err := q.CountSpecificationTags(ctx, postgresdb.CountSpecificationTagsParams{TenantID: tenantID, SearchQuery: opts.Query, StatusFilter: opts.Status, TypeFilter: opts.TypeID})
	if err != nil {
		return result, err
	}
	result.Total = int(total)
	rows, err := q.ListSpecificationTags(ctx, postgresdb.ListSpecificationTagsParams{TenantID: tenantID, SearchQuery: opts.Query, StatusFilter: opts.Status, TypeFilter: opts.TypeID, PageSize: int32(opts.PageSize), PageOffset: int32((opts.Page - 1) * opts.PageSize)})
	if err != nil {
		return result, err
	}
	for _, row := range rows {
		created, updated := row.CreatedAt, row.UpdatedAt
		result.Tags = append(result.Tags, application.SpecificationTagSummary{
			SpecificationTag: domain.SpecificationTag{
				ID: row.ID, TenantID: tenant, Name: row.Name, NormalizedName: row.NormalizedName,
				Enabled: row.Enabled, CreatedAt: created, UpdatedAt: updated,
				TypeID: row.TypeID,
			},
			ReferenceCount: int(row.ReferenceCount), TypeName: row.TypeName,
		})
	}
	return result, nil
}
