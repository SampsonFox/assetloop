package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/SampsonFox/assetloop/internal/store/sqlite/sqlitedb"
)

var _ application.ManagementStore = (*Store)(nil)

func (s *Store) WithManagementWrite(ctx context.Context, tenant string, fn func(application.ManagementStore) error) error {

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	scoped := &Store{db: s.db, tx: tx, managementTenant: tenant}
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
func (s *Store) FindManagementRequest(ctx context.Context, tenant, user, key string) (application.ManagementRequest, bool, error) {

	row, err := s.queries().FindManagementRequest(ctx, sqlitedb.FindManagementRequestParams{TenantID: tenant, UserID: user, RequestKey: key})
	if errors.Is(err, sql.ErrNoRows) {
		return application.ManagementRequest{}, false, nil
	}
	if err != nil {
		return application.ManagementRequest{}, false, err
	}
	return application.ManagementRequest{TenantID: tenant, UserID: user, Key: key, Hash: row.RequestHash, ResultJSON: row.ResultJson}, true, nil
}
func (s *Store) SaveManagementRequest(ctx context.Context, r application.ManagementRequest) error {

	return s.queries().SaveManagementRequest(ctx, sqlitedb.SaveManagementRequestParams{TenantID: r.TenantID, UserID: r.UserID, RequestKey: r.Key, RequestHash: r.Hash, ResultJson: r.ResultJSON})
}
