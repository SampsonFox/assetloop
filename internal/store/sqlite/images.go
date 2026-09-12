package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/SampsonFox/assetloop/internal/store/sqlite/sqlitedb"
)

func (s *Store) WithImageWrite(ctx context.Context, t string, fn func(application.ModelImageStore) error) error {
	return s.WithSpecificationWrite(ctx, t, func(tx application.SpecificationStore) error { return fn(tx.(application.ModelImageStore)) })
}
func imageResult(r sqlitedb.ModelImage, err error) (application.ModelImage, error) {
	if errors.Is(err, sql.ErrNoRows) {
		return application.ModelImage{}, application.ErrImageNotFound
	}
	return application.ModelImage{ID: r.ID, TenantID: r.TenantID, ModelID: r.ModelID, StoreID: r.StoreID, ObjectKey: r.ObjectKey, SHA256: r.Sha256, ContentType: r.ContentType, SizeBytes: r.SizeBytes, SourceURL: r.SourceUrl}, err
}
func (s *Store) GetModelImage(ctx context.Context, t, m string) (application.ModelImage, error) {

	r, e := s.queries().GetModelImage(ctx, sqlitedb.GetModelImageParams{TenantID: t, ModelID: m})
	return imageResult(r, e)
}
func (s *Store) GetImageRevision(ctx context.Context, t, id string) (application.ModelImage, error) {

	r, e := s.queries().GetImageRevision(ctx, sqlitedb.GetImageRevisionParams{TenantID: t, ID: id})
	return imageResult(r, e)
}
func (s *Store) ClearModelImage(ctx context.Context, t, m string) error {

	return s.queries().ClearModelImage(ctx, sqlitedb.ClearModelImageParams{TenantID: t, ModelID: m})
}
func (s *Store) InsertModelImage(ctx context.Context, m application.ModelImage) error {

	return s.queries().InsertModelImage(ctx, sqlitedb.InsertModelImageParams{ID: m.ID, TenantID: m.TenantID, ModelID: m.ModelID, StoreID: m.StoreID, ObjectKey: m.ObjectKey, Sha256: m.SHA256, ContentType: m.ContentType, SizeBytes: m.SizeBytes, SourceUrl: m.SourceURL})
}
