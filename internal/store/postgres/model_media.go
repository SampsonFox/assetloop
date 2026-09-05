package postgres

import (
	"context"
	"database/sql"
	"errors"
	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/SampsonFox/assetloop/internal/domain"
	"github.com/SampsonFox/assetloop/internal/store/postgres/postgresdb"
	"github.com/google/uuid"
)

func (s *Store) CreateAndBindModel3DResource(ctx context.Context, r domain.Model3DResource, b application.BindModel3DResource) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	bound := &Store{db: s.db, tx: tx}
	if err := bound.CreateModel3DResource(ctx, r); err != nil {
		return err
	}
	b.ResourceID = r.ID
	if err := bound.BindModel3DResource(ctx, r.TenantID, b); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) GetModel3DBinding(ctx context.Context, tenantID, kind, targetID string) (application.Model3DBinding, error) {
	id, tenant, err := catalogIDs(targetID, tenantID)
	if err != nil {
		return application.Model3DBinding{}, err
	}
	r, err := s.queries().GetModel3DBinding(ctx, postgresdb.GetModel3DBindingParams{ID: id, TenantID: tenant, Kind: kind})
	if err != nil {
		return application.Model3DBinding{}, err
	}
	return application.Model3DBinding{Name: r.Name, ResourceID: r.ResourceID.(string), EffectiveResourceID: r.EffectiveResourceID.(string), Source: r.Source}, nil
}
func resourceValue(r postgresdb.Model3dResource) (domain.Model3DResource, error) {
	created, updated := r.CreatedAt, r.UpdatedAt
	return domain.Model3DResource{
		ID: r.ID.String(), TenantID: r.TenantID.String(), Name: r.Name, Status: r.Status, CreatedAt: created,
		ProductModel3D: domain.ProductModel3D{ResourceID: r.ID.String(), StoreID: r.StoreID, ObjectKey: r.ObjectKey, SHA256: r.Sha256, SizeBytes: r.SizeBytes, SourceURL: r.SourceUrl, Author: r.Author, License: r.License, UpdatedAt: updated},
	}, nil
}
func (s *Store) CreateModel3DResource(ctx context.Context, r domain.Model3DResource) error {
	id, tenant, err := catalogIDs(r.ID, r.TenantID)
	if err != nil {
		return err
	}
	return s.queries().CreateModel3DResource(ctx, postgresdb.CreateModel3DResourceParams{ID: id, TenantID: tenant, Name: r.Name, Status: r.Status, StoreID: r.StoreID, ObjectKey: r.ObjectKey, Sha256: r.SHA256, SizeBytes: r.SizeBytes, SourceUrl: r.SourceURL, Author: r.Author, License: r.License, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt})
}
func (s *Store) GetModel3DResource(ctx context.Context, tenantID, resourceID string) (domain.Model3DResource, error) {
	id, tenant, err := catalogIDs(resourceID, tenantID)
	if err != nil {
		return domain.Model3DResource{}, err
	}
	r, err := s.queries().GetModel3DResource(ctx, postgresdb.GetModel3DResourceParams{ID: id, TenantID: tenant})
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Model3DResource{}, application.ErrModel3DNotFound
	}
	if err != nil {
		return domain.Model3DResource{}, err
	}
	return resourceValue(r)
}
func (s *Store) ResolveAssetModel3D(ctx context.Context, tenantID, resourceID string) (domain.Model3DResource, error) {
	id, tenant, err := catalogIDs(resourceID, tenantID)
	if err != nil {
		return domain.Model3DResource{}, err
	}
	r, err := s.queries().ResolveAssetModel3D(ctx, postgresdb.ResolveAssetModel3DParams{ID: id, TenantID: tenant})
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Model3DResource{}, application.ErrModel3DNotFound
	}
	if err != nil {
		return domain.Model3DResource{}, err
	}
	return resourceValue(r)
}
func (s *Store) ListModel3DResources(ctx context.Context, tenantID string, o application.Model3DResourceListOptions) (application.Model3DResourceListResult, error) {
	tenant, err := uuid.Parse(tenantID)
	if err != nil {
		return application.Model3DResourceListResult{}, err
	}
	total, err := s.queries().CountModel3DResources(ctx, postgresdb.CountModel3DResourcesParams{TenantID: tenant, SearchQuery: o.Query})
	if err != nil {
		return application.Model3DResourceListResult{}, err
	}
	rows, err := s.queries().ListModel3DResources(ctx, postgresdb.ListModel3DResourcesParams{TenantID: tenant, SearchQuery: o.Query, PageSize: int32(o.PageSize), PageOffset: int32((o.Page - 1) * o.PageSize)})
	if err != nil {
		return application.Model3DResourceListResult{}, err
	}
	result := application.Model3DResourceListResult{Total: int(total), Resources: []domain.Model3DResource{}}
	for _, row := range rows {
		r, err := resourceValue(postgresdb.Model3dResource{ID: row.ID, TenantID: row.TenantID, Name: row.Name, Status: row.Status, StoreID: row.StoreID, ObjectKey: row.ObjectKey, Sha256: row.Sha256, SizeBytes: row.SizeBytes, SourceUrl: row.SourceUrl, Author: row.Author, License: row.License, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt})
		if err != nil {
			return application.Model3DResourceListResult{}, err
		}
		r.ReferenceCount = int(row.ReferenceCount)
		result.Resources = append(result.Resources, r)
	}
	return result, nil
}
func (s *Store) UpdateModel3DResource(ctx context.Context, r domain.Model3DResource) error {
	id, tenant, err := catalogIDs(r.ID, r.TenantID)
	if err != nil {
		return err
	}
	n, err := s.queries().UpdateModel3DResource(ctx, postgresdb.UpdateModel3DResourceParams{ID: id, TenantID: tenant, Name: r.Name, SourceUrl: r.SourceURL, Author: r.Author, License: r.License, UpdatedAt: r.UpdatedAt})
	return updatedRow(n, err)
}
func (s *Store) Model3DReferences(ctx context.Context, tenantID, resourceID string) ([]application.Model3DReference, error) {
	id, tenant, err := catalogIDs(resourceID, tenantID)
	if err != nil {
		return nil, err
	}
	rows, err := s.queries().Model3DReferences(ctx, postgresdb.Model3DReferencesParams{ID: uuid.NullUUID{UUID: id, Valid: true}, TenantID: tenant})
	if err != nil {
		return nil, err
	}
	result := []application.Model3DReference{}
	for _, row := range rows {
		result = append(result, application.Model3DReference{Kind: row.Kind, ID: row.ID.String(), Name: row.Name})
	}
	return result, nil
}
func (s *Store) MarkModel3DResourcePendingDelete(ctx context.Context, tenantID, resourceID string) error {
	id, tenant, err := catalogIDs(resourceID, tenantID)
	if err != nil {
		return err
	}
	n, err := s.queries().MarkModel3DResourcePendingDelete(ctx, postgresdb.MarkModel3DResourcePendingDeleteParams{ID: id, TenantID: tenant})
	if err != nil {
		return err
	}
	if n == 0 {
		return application.ErrModel3DReferenced
	}
	return nil
}
func (s *Store) FinishModel3DResourceDelete(ctx context.Context, tenantID, resourceID string) error {
	id, tenant, err := catalogIDs(resourceID, tenantID)
	if err != nil {
		return err
	}
	_, err = s.queries().FinishModel3DResourceDelete(ctx, postgresdb.FinishModel3DResourceDeleteParams{ID: id, TenantID: tenant})
	return err
}
func (s *Store) BindModel3DResource(ctx context.Context, tenantID string, c application.BindModel3DResource) error {
	id, tenant, err := catalogIDs(c.TargetID, tenantID)
	if err != nil {
		return err
	}
	resource := uuid.NullUUID{}
	if c.ResourceID != "" {
		v, e := uuid.Parse(c.ResourceID)
		if e != nil {
			return e
		}
		resource = uuid.NullUUID{UUID: v, Valid: true}
	}
	var n int64
	switch c.Kind {
	case "model":
		n, err = s.queries().BindModel3D(ctx, postgresdb.BindModel3DParams{ID: id, TenantID: tenant, ResourceID: resource})
	case "variant":
		n, err = s.queries().BindVariant3D(ctx, postgresdb.BindVariant3DParams{ID: id, TenantID: tenant, ResourceID: resource})
	case "asset":
		n, err = s.queries().BindAsset3D(ctx, postgresdb.BindAsset3DParams{ID: id, TenantID: tenant, ResourceID: resource})
	default:
		return application.ErrModel3DUnavailable
	}
	return updatedRow(n, err)
}
