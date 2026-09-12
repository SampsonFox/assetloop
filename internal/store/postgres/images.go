package postgres

import (
	"context"
	"database/sql"
	"errors"
	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/SampsonFox/assetloop/internal/store/postgres/postgresdb"
	"github.com/google/uuid"
)

func (s *Store) WithImageWrite(ctx context.Context, t string, fn func(application.ModelImageStore) error) error {
	return s.WithSpecificationWrite(ctx, t, func(tx application.SpecificationStore) error { return fn(tx.(application.ModelImageStore)) })
}
func imageResult(r postgresdb.ModelImage, err error) (application.ModelImage, error) {
	if errors.Is(err, sql.ErrNoRows) {
		return application.ModelImage{}, application.ErrImageNotFound
	}
	return application.ModelImage{ID: r.ID.String(), TenantID: r.TenantID.String(), ModelID: r.ModelID.String(), StoreID: r.StoreID, ObjectKey: r.ObjectKey, SHA256: r.Sha256, ContentType: r.ContentType, SizeBytes: r.SizeBytes, SourceURL: r.SourceUrl}, err
}
func (s *Store) GetModelImage(ctx context.Context, t, m string) (application.ModelImage, error) {
	tenant, e := uuid.Parse(t)
	if e != nil {
		return application.ModelImage{}, e
	}
	model, e := uuid.Parse(m)
	if e != nil {
		return application.ModelImage{}, e
	}
	r, e := s.queries().GetModelImage(ctx, postgresdb.GetModelImageParams{TenantID: tenant, ModelID: model})
	return imageResult(r, e)
}
func (s *Store) GetImageRevision(ctx context.Context, t, id string) (application.ModelImage, error) {
	tenant, e := uuid.Parse(t)
	if e != nil {
		return application.ModelImage{}, e
	}
	imageID, e := uuid.Parse(id)
	if e != nil {
		return application.ModelImage{}, e
	}
	r, e := s.queries().GetImageRevision(ctx, postgresdb.GetImageRevisionParams{TenantID: tenant, ID: imageID})
	return imageResult(r, e)
}
func (s *Store) ClearModelImage(ctx context.Context, t, m string) error {
	tenant, e := uuid.Parse(t)
	if e != nil {
		return e
	}
	model, e := uuid.Parse(m)
	if e != nil {
		return e
	}
	return s.queries().ClearModelImage(ctx, postgresdb.ClearModelImageParams{TenantID: tenant, ModelID: model})
}
func (s *Store) InsertModelImage(ctx context.Context, m application.ModelImage) error {
	id, e := uuid.Parse(m.ID)
	if e != nil {
		return e
	}
	tenant, e := uuid.Parse(m.TenantID)
	if e != nil {
		return e
	}
	model, e := uuid.Parse(m.ModelID)
	if e != nil {
		return e
	}
	return s.queries().InsertModelImage(ctx, postgresdb.InsertModelImageParams{ID: id, TenantID: tenant, ModelID: model, StoreID: m.StoreID, ObjectKey: m.ObjectKey, Sha256: m.SHA256, ContentType: m.ContentType, SizeBytes: m.SizeBytes, SourceUrl: m.SourceURL})
}
