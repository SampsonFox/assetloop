package postgres

import (
	"context"
	"database/sql"
	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/SampsonFox/assetloop/internal/domain"
	"github.com/SampsonFox/assetloop/internal/store/postgres/postgresdb"
	"github.com/google/uuid"
	"time"
)

func (s *Store) WithSpecificationWrite(ctx context.Context, tenant string, fn func(application.SpecificationStore) error) error {
	if s.tx != nil && s.managementTenant != "" {
		if tenant != s.managementTenant {
			return application.ErrForbidden
		}
		return fn(s)
	}
	tenantID, err := uuid.Parse(tenant)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	scoped := &Store{db: s.db, tx: tx}
	n, err := scoped.queries().LockLifecycleTenant(ctx, tenantID)
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
	tenantID, err := uuid.Parse(tenant)
	if err != nil {
		return result, err
	}
	q := s.queries()
	types, err := q.SpecificationTypes(ctx, tenantID)
	if err != nil {
		return result, err
	}
	for _, r := range types {
		created, updated := r.CreatedAt, r.UpdatedAt
		result.Types = append(result.Types, domain.SpecificationTagType{ID: r.ID, TenantID: tenant, Name: r.Name, NormalizedName: r.NormalizedName, SystemCode: r.SystemCode, Multiple: r.Multiple, AffectsAppearance: r.AffectsAppearance, Enabled: r.Enabled, CreatedAt: created, UpdatedAt: updated})
	}
	tags, err := q.SpecificationTags(ctx, tenantID)
	if err != nil {
		return result, err
	}
	for _, r := range tags {
		created, updated := r.CreatedAt, r.UpdatedAt
		result.Tags = append(result.Tags, domain.SpecificationTag{ID: r.ID, TenantID: tenant, TypeID: r.TypeID, Name: r.Name, NormalizedName: r.NormalizedName, Enabled: r.Enabled, CreatedAt: created, UpdatedAt: updated})
	}
	links, err := q.SpecificationLinks(ctx, tenantID)
	if err != nil {
		return result, err
	}
	for _, r := range links {
		result.Links = append(result.Links, application.SpecificationLink{Kind: r.Kind, TargetID: r.TargetID, ModelID: r.ModelID, TagID: r.TagID})
	}
	dims, err := q.SpecificationDimensions(ctx, tenantID)
	if err != nil {
		return result, err
	}
	for _, r := range dims {
		result.Dimensions = append(result.Dimensions, application.AppearanceDimension{ModelID: r.ModelID, TypeID: r.TypeID, AffectsAppearance: r.AffectsAppearance})
	}
	rules, err := q.SpecificationDefaults(ctx, tenantID)
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
	result.CategoryIDs, err = q.SpecificationCategories(ctx, tenantID)
	return result, err
}

func (s *Store) PutSpecificationType(ctx context.Context, r domain.SpecificationTagType) error {
	tenant, id := r.TenantID, r.ID
	tenantID, err := uuid.Parse(tenant)
	if err != nil {
		return err
	}
	idID, err := uuid.Parse(id)
	if err != nil {
		return err
	}
	return s.queries().PutSpecificationType(ctx, postgresdb.PutSpecificationTypeParams{ID: idID, TenantID: tenantID, Name: r.Name, NormalizedName: r.NormalizedName, SystemCode: r.SystemCode, Multiple: r.Multiple, AffectsAppearance: r.AffectsAppearance, Enabled: r.Enabled, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt})
}
func (s *Store) PutSpecificationTag(ctx context.Context, r domain.SpecificationTag) error {
	tenant, id, kind := r.TenantID, r.ID, r.TypeID
	tenantID, err := uuid.Parse(tenant)
	if err != nil {
		return err
	}
	idID, err := uuid.Parse(id)
	if err != nil {
		return err
	}
	kindID, err := uuid.Parse(kind)
	if err != nil {
		return err
	}
	return s.queries().PutSpecificationTag(ctx, postgresdb.PutSpecificationTagParams{ID: idID, TenantID: tenantID, TypeID: kindID, Name: r.Name, NormalizedName: r.NormalizedName, Enabled: r.Enabled, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt})
}
func (s *Store) PutAppearanceDimension(ctx context.Context, tenant string, r application.AppearanceDimension) error {
	model, kind := r.ModelID, r.TypeID
	tenantID, err := uuid.Parse(tenant)
	if err != nil {
		return err
	}
	modelID, err := uuid.Parse(model)
	if err != nil {
		return err
	}
	kindID, err := uuid.Parse(kind)
	if err != nil {
		return err
	}
	return s.queries().PutAppearanceDimension(ctx, postgresdb.PutAppearanceDimensionParams{TenantID: tenantID, ModelID: modelID, TypeID: kindID, AffectsAppearance: r.AffectsAppearance})
}
func (s *Store) PutAppearanceDefault(ctx context.Context, r domain.AppearanceDefault) error {
	tenant, id, model, resource := r.TenantID, r.ID, r.ModelID, r.ResourceID
	tenantID, err := uuid.Parse(tenant)
	if err != nil {
		return err
	}
	idID, err := uuid.Parse(id)
	if err != nil {
		return err
	}
	modelID, err := uuid.Parse(model)
	if err != nil {
		return err
	}
	resourceID, err := uuid.Parse(resource)
	if err != nil {
		return err
	}
	stamp := time.Now().UTC()
	return s.queries().PutAppearanceDefault(ctx, postgresdb.PutAppearanceDefaultParams{ID: idID, TenantID: tenantID, ModelID: modelID, ResourceID: resourceID, CreatedAt: stamp, UpdatedAt: stamp})
}
func (s *Store) AddModelAllowedTag(ctx context.Context, tenant, model, tag string) error {
	tenantID, err := uuid.Parse(tenant)
	if err != nil {
		return err
	}
	modelID, err := uuid.Parse(model)
	if err != nil {
		return err
	}
	tagID, err := uuid.Parse(tag)
	if err != nil {
		return err
	}
	return s.queries().AddModelAllowedTag(ctx, postgresdb.AddModelAllowedTagParams{TenantID: tenantID, ModelID: modelID, TagID: tagID})

}
func (s *Store) RemoveModelAllowedTag(ctx context.Context, tenant, model, tag string) error {
	tenantID, err := uuid.Parse(tenant)
	if err != nil {
		return err
	}
	modelID, err := uuid.Parse(model)
	if err != nil {
		return err
	}
	tagID, err := uuid.Parse(tag)
	if err != nil {
		return err
	}
	return s.queries().RemoveModelAllowedTag(ctx, postgresdb.RemoveModelAllowedTagParams{TenantID: tenantID, ModelID: modelID, TagID: tagID})

}
func (s *Store) ClearAppearanceDimensions(ctx context.Context, tenant, model string) error {
	tenantID, err := uuid.Parse(tenant)
	if err != nil {
		return err
	}
	modelID, err := uuid.Parse(model)
	if err != nil {
		return err
	}
	return s.queries().ClearAppearanceDimensions(ctx, postgresdb.ClearAppearanceDimensionsParams{TenantID: tenantID, ModelID: modelID})

}
func (s *Store) DeleteAppearanceDefault(ctx context.Context, tenant, id string) error {
	tenantID, err := uuid.Parse(tenant)
	if err != nil {
		return err
	}
	idID, err := uuid.Parse(id)
	if err != nil {
		return err
	}
	return s.queries().DeleteAppearanceDefault(ctx, postgresdb.DeleteAppearanceDefaultParams{TenantID: tenantID, ID: idID})

}
func (s *Store) ClearAppearanceConditions(ctx context.Context, tenant, rule string) error {
	tenantID, err := uuid.Parse(tenant)
	if err != nil {
		return err
	}
	ruleID, err := uuid.Parse(rule)
	if err != nil {
		return err
	}
	return s.queries().ClearAppearanceConditions(ctx, postgresdb.ClearAppearanceConditionsParams{TenantID: tenantID, RuleID: ruleID})

}
func (s *Store) AddAppearanceCondition(ctx context.Context, tenant, rule, model, tag string) error {
	tenantID, err := uuid.Parse(tenant)
	if err != nil {
		return err
	}
	ruleID, err := uuid.Parse(rule)
	if err != nil {
		return err
	}
	modelID, err := uuid.Parse(model)
	if err != nil {
		return err
	}
	tagID, err := uuid.Parse(tag)
	if err != nil {
		return err
	}
	return s.queries().AddAppearanceCondition(ctx, postgresdb.AddAppearanceConditionParams{TenantID: tenantID, RuleID: ruleID, ModelID: modelID, TagID: tagID})

}
func (s *Store) ClearAssetSpecificationTags(ctx context.Context, tenant, asset string) error {
	tenantID, err := uuid.Parse(tenant)
	if err != nil {
		return err
	}
	assetID, err := uuid.Parse(asset)
	if err != nil {
		return err
	}
	return s.queries().ClearAssetSpecificationTags(ctx, postgresdb.ClearAssetSpecificationTagsParams{TenantID: tenantID, AssetID: assetID})

}
func (s *Store) AddAssetSpecificationTag(ctx context.Context, tenant, asset, model, tag string) error {
	tenantID, err := uuid.Parse(tenant)
	if err != nil {
		return err
	}
	assetID, err := uuid.Parse(asset)
	if err != nil {
		return err
	}
	modelID, err := uuid.Parse(model)
	if err != nil {
		return err
	}
	tagID, err := uuid.Parse(tag)
	if err != nil {
		return err
	}
	return s.queries().AddAssetSpecificationTag(ctx, postgresdb.AddAssetSpecificationTagParams{TenantID: tenantID, AssetID: assetID, ModelID: modelID, TagID: tagID})

}
func (s *Store) ClearResourceSpecificationTags(ctx context.Context, tenant, resource string) error {
	tenantID, err := uuid.Parse(tenant)
	if err != nil {
		return err
	}
	resourceID, err := uuid.Parse(resource)
	if err != nil {
		return err
	}
	return s.queries().ClearResourceSpecificationTags(ctx, postgresdb.ClearResourceSpecificationTagsParams{TenantID: tenantID, ResourceID: resourceID})

}
func (s *Store) AddResourceSpecificationTag(ctx context.Context, tenant, resource, tag string) error {
	tenantID, err := uuid.Parse(tenant)
	if err != nil {
		return err
	}
	resourceID, err := uuid.Parse(resource)
	if err != nil {
		return err
	}
	tagID, err := uuid.Parse(tag)
	if err != nil {
		return err
	}
	return s.queries().AddResourceSpecificationTag(ctx, postgresdb.AddResourceSpecificationTagParams{TenantID: tenantID, ResourceID: resourceID, TagID: tagID})

}
func (s *Store) ClearResourceCategories(ctx context.Context, tenant, resource string) error {
	tenantID, err := uuid.Parse(tenant)
	if err != nil {
		return err
	}
	resourceID, err := uuid.Parse(resource)
	if err != nil {
		return err
	}
	return s.queries().ClearResourceCategories(ctx, postgresdb.ClearResourceCategoriesParams{TenantID: tenantID, ResourceID: resourceID})

}
func (s *Store) AddResourceCategory(ctx context.Context, tenant, resource, category string) error {
	tenantID, err := uuid.Parse(tenant)
	if err != nil {
		return err
	}
	resourceID, err := uuid.Parse(resource)
	if err != nil {
		return err
	}
	categoryID, err := uuid.Parse(category)
	if err != nil {
		return err
	}
	return s.queries().AddResourceCategory(ctx, postgresdb.AddResourceCategoryParams{TenantID: tenantID, ResourceID: resourceID, CategoryID: categoryID})

}
func (s *Store) WriteSelectedAsset(ctx context.Context, r domain.Asset, create bool) error {
	tenant, id, model := r.TenantID, r.ID, r.ModelID
	tenantID, err := uuid.Parse(tenant)
	if err != nil {
		return err
	}
	idID, err := uuid.Parse(id)
	if err != nil {
		return err
	}
	modelID, err := uuid.Parse(model)
	if err != nil {
		return err
	}
	if create {
		return s.queries().CreateSelectedAsset(ctx, postgresdb.CreateSelectedAssetParams{ID: idID, TenantID: tenantID, ModelID: modelID, DisplayName: r.DisplayName, SerialNumber: r.SerialNumber, PurchaseChannel: r.PurchaseChannel, Notes: r.Notes, CreatedAt: r.CreatedAt})
	}
	n, err := s.queries().UpdateSelectedAsset(ctx, postgresdb.UpdateSelectedAssetParams{ID: idID, TenantID: tenantID, ModelID: modelID, DisplayName: r.DisplayName, SerialNumber: r.SerialNumber, PurchaseChannel: r.PurchaseChannel, Notes: r.Notes})
	return updatedRow(n, err)
}

var _ application.SpecificationStore = (*Store)(nil)
