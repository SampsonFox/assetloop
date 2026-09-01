package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/SampsonFox/assetloop/internal/store/postgres/postgresdb"
	"github.com/google/uuid"
)

func (s *Store) AuthNeedsSetup(ctx context.Context) (bool, error) {
	count, err := postgresdb.New(s.db).CountUsers(ctx)
	return count == 0, err
}

func (s *Store) BootstrapAuth(ctx context.Context, tenant application.Tenant, user application.User, membership application.Membership, event application.SecurityEvent) error {
	tenantID, err := uuid.Parse(tenant.ID)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := postgresdb.New(tx)
	count, err := q.CountUsers(ctx)
	if err != nil {
		return err
	}
	if count != 0 {
		return application.ErrSetupComplete
	}
	if err := q.CreateTenant(ctx, postgresdb.CreateTenantParams{ID: tenantID, Name: tenant.Name, BaseCurrency: tenant.BaseCurrency, CreatedAt: tenant.CreatedAt}); err != nil {
		return err
	}
	if err := createUserAndMembershipPostgres(ctx, q, user, membership); err != nil {
		return err
	}
	eventParams, err := postgresSecurityEvent(event)
	if err != nil {
		return err
	}
	if err := q.CreateSecurityAuditEvent(ctx, eventParams); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) FindAccount(ctx context.Context, usernameNormalized string) (application.Account, error) {
	row, err := postgresdb.New(s.db).FindAccountByUsername(ctx, usernameNormalized)
	if err != nil {
		return application.Account{}, err
	}
	return application.Account{Principal: application.Principal{TenantID: row.TenantID.String(), TenantName: row.TenantName, UserID: row.UserID.String(), Username: row.Username, Role: application.Role(row.Role)}, PasswordHash: row.PasswordHash}, nil
}

func (s *Store) FirstPrincipal(ctx context.Context) (application.Principal, error) {
	row, err := postgresdb.New(s.db).FirstPrincipal(ctx)
	if err != nil {
		return application.Principal{}, err
	}
	return application.Principal{TenantID: row.TenantID.String(), TenantName: row.TenantName, UserID: row.UserID.String(), Username: row.Username, Role: application.Role(row.Role)}, nil
}

func (s *Store) CreateSession(ctx context.Context, session application.Session) error {
	tenantID, userID, err := twoUUIDs(session.TenantID, session.UserID)
	if err != nil {
		return err
	}
	return postgresdb.New(s.db).CreateSession(ctx, postgresdb.CreateSessionParams{TokenHash: session.TokenHash, TenantID: tenantID, UserID: userID, ExpiresAt: session.ExpiresAt, CreatedAt: session.CreatedAt})
}

func (s *Store) GetSessionPrincipal(ctx context.Context, tokenHash string, now time.Time) (application.Principal, error) {
	row, err := postgresdb.New(s.db).GetSessionPrincipal(ctx, postgresdb.GetSessionPrincipalParams{TokenHash: tokenHash, ExpiresAt: now})
	if err != nil {
		return application.Principal{}, err
	}
	return application.Principal{TenantID: row.TenantID.String(), TenantName: row.TenantName, UserID: row.UserID.String(), Username: row.Username, Role: application.Role(row.Role)}, nil
}

func (s *Store) DeleteSession(ctx context.Context, tokenHash string) error {
	return postgresdb.New(s.db).DeleteSession(ctx, tokenHash)
}

func (s *Store) CreateMember(ctx context.Context, user application.User, membership application.Membership, event application.SecurityEvent) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := postgresdb.New(tx)
	if err := createUserAndMembershipPostgres(ctx, q, user, membership); err != nil {
		return err
	}
	eventParams, err := postgresSecurityEvent(event)
	if err != nil {
		return err
	}
	if err := q.CreateSecurityAuditEvent(ctx, eventParams); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) ListMembers(ctx context.Context, tenantID string) ([]application.Member, error) {
	id, err := uuid.Parse(tenantID)
	if err != nil {
		return nil, fmt.Errorf("parse tenant ID: %w", err)
	}
	rows, err := postgresdb.New(s.db).ListMembers(ctx, id)
	if err != nil {
		return nil, err
	}
	members := make([]application.Member, 0, len(rows))
	for _, row := range rows {
		members = append(members, application.Member{UserID: row.UserID.String(), Username: row.Username, Role: application.Role(row.Role), CreatedAt: row.CreatedAt})
	}
	return members, nil
}

func (s *Store) RecordSecurityEvent(ctx context.Context, event application.SecurityEvent) error {
	params, err := postgresSecurityEvent(event)
	if err != nil {
		return err
	}
	return postgresdb.New(s.db).CreateSecurityAuditEvent(ctx, params)
}

func createUserAndMembershipPostgres(ctx context.Context, q *postgresdb.Queries, user application.User, membership application.Membership) error {
	userID, tenantID, err := twoUUIDs(user.ID, membership.TenantID)
	if err != nil {
		return err
	}
	if err := q.CreateUser(ctx, postgresdb.CreateUserParams{ID: userID, Username: user.Username, UsernameNormalized: user.UsernameNormalized, PasswordHash: user.PasswordHash, CreatedAt: user.CreatedAt}); err != nil {
		return err
	}
	return q.CreateMembership(ctx, postgresdb.CreateMembershipParams{TenantID: tenantID, UserID: userID, Role: string(membership.Role), CreatedAt: membership.CreatedAt})
}

func postgresSecurityEvent(event application.SecurityEvent) (postgresdb.CreateSecurityAuditEventParams, error) {
	ids, err := authIDs(event.ID, event.TenantID)
	if err != nil {
		return postgresdb.CreateSecurityAuditEventParams{}, err
	}
	actor, err := nullableUUID(event.ActorUserID)
	if err != nil {
		return postgresdb.CreateSecurityAuditEventParams{}, err
	}
	target, err := nullableUUID(event.TargetUserID)
	if err != nil {
		return postgresdb.CreateSecurityAuditEventParams{}, err
	}
	return postgresdb.CreateSecurityAuditEventParams{
		ID: ids[0], TenantID: ids[1], ActorUserID: actor,
		Action: event.Action, TargetUserID: target, Detail: event.Detail, OccurredAt: event.OccurredAt,
	}, nil
}

func authIDs(values ...string) ([]uuid.UUID, error) {
	result := make([]uuid.UUID, len(values))
	for i, value := range values {
		id, err := uuid.Parse(value)
		if err != nil {
			return nil, fmt.Errorf("parse auth identifier: %w", err)
		}
		result[i] = id
	}
	return result, nil
}

func twoUUIDs(first, second string) (uuid.UUID, uuid.UUID, error) {
	ids, err := authIDs(first, second)
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	return ids[0], ids[1], nil
}

func nullableUUID(value string) (uuid.NullUUID, error) {
	if value == "" {
		return uuid.NullUUID{}, nil
	}
	id, err := uuid.Parse(value)
	if err != nil {
		return uuid.NullUUID{}, fmt.Errorf("parse nullable auth identifier: %w", err)
	}
	return uuid.NullUUID{UUID: id, Valid: true}, nil
}
