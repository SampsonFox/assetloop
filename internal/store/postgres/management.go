package postgres

import (
	"context"
	"database/sql"
	"errors"
	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/SampsonFox/assetloop/internal/store/postgres/postgresdb"
	"github.com/google/uuid"
)

var _ application.ManagementStore = (*Store)(nil)

func (s *Store) WithManagementWrite(ctx context.Context, tenant string, fn func(application.ManagementStore) error) error {
	tenantID, err := uuid.Parse(tenant)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	scoped := &Store{db: s.db, tx: tx, managementTenant: tenant}
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
func (s *Store) FindManagementRequest(ctx context.Context, tenant, user, key string) (application.ManagementRequest, bool, error) {
	tenantID, userID, err := catalogIDs(tenant, user)
	if err != nil {
		return application.ManagementRequest{}, false, err
	}
	row, err := s.queries().FindManagementRequest(ctx, postgresdb.FindManagementRequestParams{TenantID: tenantID, UserID: userID, RequestKey: key})
	if errors.Is(err, sql.ErrNoRows) {
		return application.ManagementRequest{}, false, nil
	}
	if err != nil {
		return application.ManagementRequest{}, false, err
	}
	return application.ManagementRequest{TenantID: tenant, UserID: user, Key: key, Hash: row.RequestHash, ResultJSON: row.ResultJson}, true, nil
}
func (s *Store) SaveManagementRequest(ctx context.Context, r application.ManagementRequest) error {
	tenant, user, err := catalogIDs(r.TenantID, r.UserID)
	if err != nil {
		return err
	}
	return s.queries().SaveManagementRequest(ctx, postgresdb.SaveManagementRequestParams{TenantID: tenant, UserID: user, RequestKey: r.Key, RequestHash: r.Hash, ResultJson: r.ResultJSON})
}
