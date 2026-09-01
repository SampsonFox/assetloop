package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/SampsonFox/assetloop/internal/store/sqlite/sqlitedb"
)

func (s *Store) AuthNeedsSetup(ctx context.Context) (bool, error) {
	count, err := sqlitedb.New(s.db).CountUsers(ctx)
	return count == 0, err
}

func (s *Store) BootstrapAuth(ctx context.Context, tenant application.Tenant, user application.User, membership application.Membership, event application.SecurityEvent) error {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := sqlitedb.New(tx)
	count, err := q.CountUsers(ctx)
	if err != nil {
		return err
	}
	if count != 0 {
		return application.ErrSetupComplete
	}
	if err := q.CreateTenant(ctx, sqlitedb.CreateTenantParams{ID: tenant.ID, Name: tenant.Name, BaseCurrency: tenant.BaseCurrency, CreatedAt: sqliteTime(tenant.CreatedAt)}); err != nil {
		return err
	}
	if err := createUserAndMembershipSQLite(ctx, q, user, membership); err != nil {
		return err
	}
	if err := q.CreateSecurityAuditEvent(ctx, sqliteSecurityEvent(event)); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) FindAccount(ctx context.Context, usernameNormalized string) (application.Account, error) {
	row, err := sqlitedb.New(s.db).FindAccountByUsername(ctx, usernameNormalized)
	if err != nil {
		return application.Account{}, err
	}
	return application.Account{Principal: application.Principal{TenantID: row.TenantID, TenantName: row.TenantName, UserID: row.UserID, Username: row.Username, Role: application.Role(row.Role)}, PasswordHash: row.PasswordHash}, nil
}

func (s *Store) FirstPrincipal(ctx context.Context) (application.Principal, error) {
	row, err := sqlitedb.New(s.db).FirstPrincipal(ctx)
	if err != nil {
		return application.Principal{}, err
	}
	return application.Principal{TenantID: row.TenantID, TenantName: row.TenantName, UserID: row.UserID, Username: row.Username, Role: application.Role(row.Role)}, nil
}

func (s *Store) CreateSession(ctx context.Context, session application.Session) error {
	return sqlitedb.New(s.db).CreateSession(ctx, sqlitedb.CreateSessionParams{TokenHash: session.TokenHash, TenantID: session.TenantID, UserID: session.UserID, ExpiresAt: sqliteTime(session.ExpiresAt), CreatedAt: sqliteTime(session.CreatedAt)})
}

func (s *Store) GetSessionPrincipal(ctx context.Context, tokenHash string, now time.Time) (application.Principal, error) {
	row, err := sqlitedb.New(s.db).GetSessionPrincipal(ctx, sqlitedb.GetSessionPrincipalParams{TokenHash: tokenHash, ExpiresAt: sqliteTime(now)})
	if err != nil {
		return application.Principal{}, err
	}
	return application.Principal{TenantID: row.TenantID, TenantName: row.TenantName, UserID: row.UserID, Username: row.Username, Role: application.Role(row.Role)}, nil
}

func (s *Store) DeleteSession(ctx context.Context, tokenHash string) error {
	return sqlitedb.New(s.db).DeleteSession(ctx, tokenHash)
}

func (s *Store) CreateMember(ctx context.Context, user application.User, membership application.Membership, event application.SecurityEvent) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := sqlitedb.New(tx)
	if err := createUserAndMembershipSQLite(ctx, q, user, membership); err != nil {
		return err
	}
	if err := q.CreateSecurityAuditEvent(ctx, sqliteSecurityEvent(event)); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) ListMembers(ctx context.Context, tenantID string) ([]application.Member, error) {
	rows, err := sqlitedb.New(s.db).ListMembers(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	members := make([]application.Member, 0, len(rows))
	for _, row := range rows {
		createdAt, err := time.Parse(time.RFC3339Nano, row.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("parse membership created_at: %w", err)
		}
		members = append(members, application.Member{UserID: row.UserID, Username: row.Username, Role: application.Role(row.Role), CreatedAt: createdAt})
	}
	return members, nil
}

func (s *Store) RecordSecurityEvent(ctx context.Context, event application.SecurityEvent) error {
	return sqlitedb.New(s.db).CreateSecurityAuditEvent(ctx, sqliteSecurityEvent(event))
}

func createUserAndMembershipSQLite(ctx context.Context, q *sqlitedb.Queries, user application.User, membership application.Membership) error {
	if err := q.CreateUser(ctx, sqlitedb.CreateUserParams{ID: user.ID, Username: user.Username, UsernameNormalized: user.UsernameNormalized, PasswordHash: user.PasswordHash, CreatedAt: sqliteTime(user.CreatedAt)}); err != nil {
		return err
	}
	return q.CreateMembership(ctx, sqlitedb.CreateMembershipParams{TenantID: membership.TenantID, UserID: membership.UserID, Role: string(membership.Role), CreatedAt: sqliteTime(membership.CreatedAt)})
}

func sqliteSecurityEvent(event application.SecurityEvent) sqlitedb.CreateSecurityAuditEventParams {
	return sqlitedb.CreateSecurityAuditEventParams{
		ID: event.ID, TenantID: event.TenantID, ActorUserID: nullableString(event.ActorUserID),
		Action: event.Action, TargetUserID: nullableString(event.TargetUserID), Detail: event.Detail, OccurredAt: sqliteTime(event.OccurredAt),
	}
}

func sqliteTime(value time.Time) string {
	return value.UTC().Format("2006-01-02T15:04:05.000000000Z07:00")
}

func nullableString(value string) sql.NullString {
	return sql.NullString{String: value, Valid: value != ""}
}
