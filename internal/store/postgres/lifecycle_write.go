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

func (s *Store) WithLifecycleWrite(ctx context.Context, tenantID string, fn func(application.LifecycleStore) (domain.AssetEvent, error)) (domain.AssetEvent, error) {
	tenant, err := uuid.Parse(tenantID)
	if err != nil {
		return domain.AssetEvent{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.AssetEvent{}, err
	}
	defer tx.Rollback()
	scoped := &Store{db: s.db, tx: tx}
	// ponytail: serialize lifecycle writes per tenant to protect currency and request keys;
	// split into narrower locks only when measured write contention requires it.
	n, err := scoped.queries().LockLifecycleTenant(ctx, tenant)
	if err != nil {
		return domain.AssetEvent{}, err
	}
	if n != 1 {
		return domain.AssetEvent{}, sql.ErrNoRows
	}
	event, err := fn(scoped)
	if err != nil {
		return domain.AssetEvent{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.AssetEvent{}, err
	}
	return event, nil
}

func (s *Store) FindLifecycleRequest(ctx context.Context, tenantID, userID, key string) (application.LifecycleRequest, bool, error) {
	tenant, user, err := catalogIDs(tenantID, userID)
	if err != nil {
		return application.LifecycleRequest{}, false, err
	}
	row, err := s.queries().FindLifecycleRequest(ctx, postgresdb.FindLifecycleRequestParams{TenantID: tenant, UserID: user, RequestKey: key})
	if errors.Is(err, sql.ErrNoRows) {
		return application.LifecycleRequest{}, false, nil
	}
	if err != nil {
		return application.LifecycleRequest{}, false, err
	}
	return application.LifecycleRequest{TenantID: tenantID, UserID: userID, Key: key, Hash: row.RequestHash, EventID: row.EventID.String()}, true, nil
}

func (s *Store) SaveLifecycleRequest(ctx context.Context, req application.LifecycleRequest) error {
	tenant, user, err := catalogIDs(req.TenantID, req.UserID)
	if err != nil {
		return err
	}
	event, err := uuid.Parse(req.EventID)
	if err != nil {
		return err
	}
	return s.queries().SaveLifecycleRequest(ctx, postgresdb.SaveLifecycleRequestParams{TenantID: tenant, UserID: user, RequestKey: req.Key, RequestHash: req.Hash, EventID: event})
}
