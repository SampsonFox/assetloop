package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/SampsonFox/assetloop/internal/domain"
	"github.com/SampsonFox/assetloop/internal/store/sqlite/sqlitedb"
)

func (s *Store) WithLifecycleWrite(ctx context.Context, tenantID string, fn func(application.LifecycleStore) (domain.AssetEvent, error)) (domain.AssetEvent, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.AssetEvent{}, err
	}
	defer tx.Rollback()
	scoped := &Store{db: s.db, tx: tx}
	// Acquire the SQLite write reservation before reading business state.
	n, err := scoped.queries().LockLifecycleTenant(ctx, tenantID)
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
	row, err := s.queries().FindLifecycleRequest(ctx, sqlitedb.FindLifecycleRequestParams{TenantID: tenantID, UserID: userID, RequestKey: key})
	if errors.Is(err, sql.ErrNoRows) {
		return application.LifecycleRequest{}, false, nil
	}
	if err != nil {
		return application.LifecycleRequest{}, false, err
	}
	return application.LifecycleRequest{TenantID: tenantID, UserID: userID, Key: key, Hash: row.RequestHash, EventID: row.EventID}, true, nil
}

func (s *Store) SaveLifecycleRequest(ctx context.Context, req application.LifecycleRequest) error {
	return s.queries().SaveLifecycleRequest(ctx, sqlitedb.SaveLifecycleRequestParams{TenantID: req.TenantID, UserID: req.UserID, RequestKey: req.Key, RequestHash: req.Hash, EventID: req.EventID})
}
