package sqlite

import (
	"context"
	"database/sql"
	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/SampsonFox/assetloop/internal/domain"
	"github.com/SampsonFox/assetloop/internal/store/sqlite/sqlitedb"
	"time"
)

func (s *Store) WithSpecificationWrite(ctx context.Context, tenant string, fn func(application.SpecificationStore) error) error {

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	scoped := &Store{db: s.db, tx: tx}
	n, err := scoped.queries().LockLifecycleTenant(ctx, tenant)
	if err != nil {
		return err
	}
	if n != 1 {
		return sql.ErrNoRows
	}
	if err := fn(scoped); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) SpecificationSnapshot(ctx context.Context, tenant string) (application.SpecificationSnapshot, error) {
	result := application.SpecificationSnapshot{}

	q := s.queries()
	types, err := q.SpecificationTypes(ctx, tenant)
	if err != nil {
		return result, err
	}
	for _, r := range types {
		created, err := parseCatalogTime(r.CreatedAt)
		if err != nil {
			return result, err
		}
		updated, err := parseCatalogTime(r.UpdatedAt)
		if err != nil {
			return result, err
		}
		result.Types = append(result.Types, domain.SpecificationTagType{ID: r.ID, TenantID: tenant, Name: r.Name, NormalizedName: r.NormalizedName, SystemCode: r.SystemCode, Multiple: r.Multiple != 0, AffectsAppearance: r.AffectsAppearance != 0, Enabled: r.Enabled != 0, CreatedAt: created, UpdatedAt: updated})
	}
	tags, err := q.SpecificationTags(ctx, tenant)
	if err != nil {
		return result, err
	}
	for _, r := range tags {
		created, err := parseCatalogTime(r.CreatedAt)
		if err != nil {
			return result, err
		}
		updated, err := parseCatalogTime(r.UpdatedAt)
		if err != nil {
			return result, err
		}
		result.Tags = append(result.Tags, domain.SpecificationTag{ID: r.ID, TenantID: tenant, TypeID: r.TypeID, Name: r.Name, NormalizedName: r.NormalizedName, Enabled: r.Enabled != 0, CreatedAt: created, UpdatedAt: updated})
	}
	links, err := q.SpecificationLinks(ctx, tenant)
	if err != nil {
		return result, err
	}
	for _, r := range links {
		result.Links = append(result.Links, application.SpecificationLink{Kind: r.Kind, TargetID: r.TargetID, ModelID: r.ModelID, TagID: r.TagID})
	}
	dims, err := q.SpecificationDimensions(ctx, tenant)
	if err != nil {
		return result, err
	}
	for _, r := range dims {
		result.Dimensions = append(result.Dimensions, application.AppearanceDimension{ModelID: r.ModelID, TypeID: r.TypeID, AffectsAppearance: r.AffectsAppearance != 0})
	}
	rules, err := q.SpecificationDefaults(ctx, tenant)
	if err != nil {
		return result, err
	}
	for _, r := range rules {
		rule := domain.AppearanceDefault{ID: r.ID, TenantID: tenant, ModelID: r.ModelID, ResourceID: r.ResourceID}
		for _, link := range result.Links {
			if link.Kind == "appearance" && link.TargetID == r.ID {
				rule.TagIDs = append(rule.TagIDs, link.TagID)
			}
		}
		result.Defaults = append(result.Defaults, rule)
	}
	result.CategoryIDs, err = q.SpecificationCategories(ctx, tenant)
	return result, err
}

func (s *Store) PutSpecificationType(ctx context.Context, r domain.SpecificationTagType) error {
	tenant, id := r.TenantID, r.ID

	return s.queries().PutSpecificationType(ctx, sqlitedb.PutSpecificationTypeParams{ID: id, TenantID: tenant, Name: r.Name, NormalizedName: r.NormalizedName, SystemCode: r.SystemCode, Multiple: specificationBool(r.Multiple), AffectsAppearance: specificationBool(r.AffectsAppearance), Enabled: specificationBool(r.Enabled), CreatedAt: sqliteTime(r.CreatedAt), UpdatedAt: sqliteTime(r.UpdatedAt)})
}
func (s *Store) PutSpecificationTag(ctx context.Context, r domain.SpecificationTag) error {
	tenant, id, kind := r.TenantID, r.ID, r.TypeID

	return s.queries().PutSpecificationTag(ctx, sqlitedb.PutSpecificationTagParams{ID: id, TenantID: tenant, TypeID: kind, Name: r.Name, NormalizedName: r.NormalizedName, Enabled: specificationBool(r.Enabled), CreatedAt: sqliteTime(r.CreatedAt), UpdatedAt: sqliteTime(r.UpdatedAt)})
}
func (s *Store) PutAppearanceDimension(ctx context.Context, tenant string, r application.AppearanceDimension) error {
	model, kind := r.ModelID, r.TypeID

	return s.queries().PutAppearanceDimension(ctx, sqlitedb.PutAppearanceDimensionParams{TenantID: tenant, ModelID: model, TypeID: kind, AffectsAppearance: specificationBool(r.AffectsAppearance)})
}
func (s *Store) PutAppearanceDefault(ctx context.Context, r domain.AppearanceDefault) error {
	tenant, id, model, resource := r.TenantID, r.ID, r.ModelID, r.ResourceID

	stamp := time.Now().UTC()
	return s.queries().PutAppearanceDefault(ctx, sqlitedb.PutAppearanceDefaultParams{ID: id, TenantID: tenant, ModelID: model, ResourceID: resource, CreatedAt: sqliteTime(stamp), UpdatedAt: sqliteTime(stamp)})
}
func (s *Store) AddModelAllowedTag(ctx context.Context, tenant, model, tag string) error {

	return s.queries().AddModelAllowedTag(ctx, sqlitedb.AddModelAllowedTagParams{TenantID: tenant, ModelID: model, TagID: tag})

}
func (s *Store) RemoveModelAllowedTag(ctx context.Context, tenant, model, tag string) error {

	return s.queries().RemoveModelAllowedTag(ctx, sqlitedb.RemoveModelAllowedTagParams{TenantID: tenant, ModelID: model, TagID: tag})

}
func (s *Store) ClearAppearanceDimensions(ctx context.Context, tenant, model string) error {

	return s.queries().ClearAppearanceDimensions(ctx, sqlitedb.ClearAppearanceDimensionsParams{TenantID: tenant, ModelID: model})

}
func (s *Store) DeleteAppearanceDefault(ctx context.Context, tenant, id string) error {

	return s.queries().DeleteAppearanceDefault(ctx, sqlitedb.DeleteAppearanceDefaultParams{TenantID: tenant, ID: id})

}
func (s *Store) ClearAppearanceConditions(ctx context.Context, tenant, rule string) error {

	return s.queries().ClearAppearanceConditions(ctx, sqlitedb.ClearAppearanceConditionsParams{TenantID: tenant, RuleID: rule})

}
func (s *Store) AddAppearanceCondition(ctx context.Context, tenant, rule, model, tag string) error {

	return s.queries().AddAppearanceCondition(ctx, sqlitedb.AddAppearanceConditionParams{TenantID: tenant, RuleID: rule, ModelID: model, TagID: tag})

}
func (s *Store) ClearAssetSpecificationTags(ctx context.Context, tenant, asset string) error {

	return s.queries().ClearAssetSpecificationTags(ctx, sqlitedb.ClearAssetSpecificationTagsParams{TenantID: tenant, AssetID: asset})

}
func (s *Store) AddAssetSpecificationTag(ctx context.Context, tenant, asset, model, tag string) error {

	return s.queries().AddAssetSpecificationTag(ctx, sqlitedb.AddAssetSpecificationTagParams{TenantID: tenant, AssetID: asset, ModelID: model, TagID: tag})

}
func (s *Store) ClearResourceSpecificationTags(ctx context.Context, tenant, resource string) error {

	return s.queries().ClearResourceSpecificationTags(ctx, sqlitedb.ClearResourceSpecificationTagsParams{TenantID: tenant, ResourceID: resource})

}
func (s *Store) AddResourceSpecificationTag(ctx context.Context, tenant, resource, tag string) error {

	return s.queries().AddResourceSpecificationTag(ctx, sqlitedb.AddResourceSpecificationTagParams{TenantID: tenant, ResourceID: resource, TagID: tag})

}
func (s *Store) ClearResourceCategories(ctx context.Context, tenant, resource string) error {

	return s.queries().ClearResourceCategories(ctx, sqlitedb.ClearResourceCategoriesParams{TenantID: tenant, ResourceID: resource})

}
func (s *Store) AddResourceCategory(ctx context.Context, tenant, resource, category string) error {

	return s.queries().AddResourceCategory(ctx, sqlitedb.AddResourceCategoryParams{TenantID: tenant, ResourceID: resource, CategoryID: category})

}
func (s *Store) WriteSelectedAsset(ctx context.Context, r domain.Asset, create bool) error {
	tenant, id, model := r.TenantID, r.ID, r.ModelID

	if create {
		return s.queries().CreateSelectedAsset(ctx, sqlitedb.CreateSelectedAssetParams{ID: id, TenantID: tenant, ModelID: model, DisplayName: r.DisplayName, SerialNumber: r.SerialNumber, PurchaseChannel: r.PurchaseChannel, Notes: r.Notes, CreatedAt: sqliteTime(r.CreatedAt)})
	}
	n, err := s.queries().UpdateSelectedAsset(ctx, sqlitedb.UpdateSelectedAssetParams{ID: id, TenantID: tenant, ModelID: model, DisplayName: r.DisplayName, SerialNumber: r.SerialNumber, PurchaseChannel: r.PurchaseChannel, Notes: r.Notes})
	return updatedRow(n, err)
}
func specificationBool(value bool) int64 {
	if value {
		return 1
	}
	return 0
}

var _ application.SpecificationStore = (*Store)(nil)
