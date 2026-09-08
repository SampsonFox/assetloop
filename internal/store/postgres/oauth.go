package postgres

import (
	"context"
	"database/sql"
	"errors"
	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/SampsonFox/assetloop/internal/store/postgres/postgresdb"
	"github.com/google/uuid"
)

var _ application.OAuthStore = (*Store)(nil)

func (s *Store) WithOAuthWrite(ctx context.Context, fn func(application.OAuthStore) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	scoped := &Store{db: s.db, tx: tx}
	// ponytail: serialize short OAuth exchanges; use grant-specific locking if measured contention warrants it.
	if err := scoped.queries().LockOAuthWrites(ctx); err != nil {
		return err
	}
	if err := fn(scoped); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) OAuthPrincipal(ctx context.Context, tenant, user string) (application.Principal, error) {
	tenantID, userID, err := catalogIDs(tenant, user)
	if err != nil {
		return application.Principal{}, err
	}
	r, err := s.queries().OAuthPrincipal(ctx, postgresdb.OAuthPrincipalParams{TenantID: tenantID, UserID: userID})
	if err != nil {
		return application.Principal{}, err
	}
	return application.Principal{TenantID: r.TenantID.String(), UserID: r.UserID.String(), Username: r.Username, TenantName: r.TenantName, Role: application.Role(r.Role), Locale: application.Locale(r.Locale), Theme: application.Theme(r.Theme), Accent: application.Accent(r.Accent)}, nil
}
func (s *Store) PutOAuthGrant(ctx context.Context, g application.OAuthGrant) error {
	tenant, user, err := catalogIDs(g.TenantID, g.UserID)
	if err != nil {
		return err
	}
	grant, err := uuid.Parse(g.ID)
	if err != nil {
		return err
	}
	return s.queries().PutOAuthGrant(ctx, postgresdb.PutOAuthGrantParams{ID: grant, TenantID: tenant, UserID: user, ClientID: g.ClientID, Scope: g.Scope, Resource: g.Resource, CreatedAt: g.CreatedAt, ExpiresAt: g.ExpiresAt, Revoked: g.Revoked})
}
func oauthGrant(r postgresdb.OauthGrant) (application.OAuthGrant, error) {

	return application.OAuthGrant{ID: r.ID.String(), TenantID: r.TenantID.String(), UserID: r.UserID.String(), ClientID: r.ClientID, Scope: r.Scope, Resource: r.Resource, CreatedAt: r.CreatedAt, ExpiresAt: r.ExpiresAt, Revoked: r.Revoked}, nil
}
func (s *Store) GetOAuthGrant(ctx context.Context, id string) (application.OAuthGrant, error) {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return application.OAuthGrant{}, err
	}
	r, err := s.queries().GetOAuthGrant(ctx, parsed)
	if errors.Is(err, sql.ErrNoRows) {
		return application.OAuthGrant{}, application.ErrOAuthGrant
	}
	if err != nil {
		return application.OAuthGrant{}, err
	}
	return oauthGrant(r)
}
func (s *Store) ListOAuthGrants(ctx context.Context, tenant, user string) ([]application.OAuthGrant, error) {
	tenantID, userID, err := catalogIDs(tenant, user)
	if err != nil {
		return nil, err
	}
	rows, err := s.queries().ListOAuthGrants(ctx, postgresdb.ListOAuthGrantsParams{TenantID: tenantID, UserID: userID})
	if err != nil {
		return nil, err
	}
	result := make([]application.OAuthGrant, 0, len(rows))
	for _, row := range rows {
		g, err := oauthGrant(row)
		if err != nil {
			return nil, err
		}
		result = append(result, g)
	}
	return result, nil
}
func (s *Store) RevokeOAuthGrant(ctx context.Context, id string) error {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return err
	}
	return s.queries().RevokeOAuthGrant(ctx, parsed)
}
func (s *Store) PutOAuthCredential(ctx context.Context, c application.OAuthCredential) error {
	tenant, grant, err := catalogIDs(c.TenantID, c.GrantID)
	if err != nil {
		return err
	}
	return s.queries().PutOAuthCredential(ctx, postgresdb.PutOAuthCredentialParams{Hash: c.Hash, TenantID: tenant, GrantID: grant, Kind: c.Kind, RedirectUri: c.RedirectURI, Challenge: c.Challenge, ExpiresAt: c.ExpiresAt, Consumed: c.Consumed})
}
func (s *Store) GetOAuthCredential(ctx context.Context, hash string) (application.OAuthCredential, error) {
	r, err := s.queries().GetOAuthCredential(ctx, hash)
	if errors.Is(err, sql.ErrNoRows) {
		return application.OAuthCredential{}, application.ErrOAuthGrant
	}
	if err != nil {
		return application.OAuthCredential{}, err
	}

	return application.OAuthCredential{Hash: r.Hash, TenantID: r.TenantID.String(), GrantID: r.GrantID.String(), Kind: r.Kind, RedirectURI: r.RedirectUri, Challenge: r.Challenge, ExpiresAt: r.ExpiresAt, Consumed: r.Consumed}, nil
}
func (s *Store) ConsumeOAuthCredential(ctx context.Context, hash string) (bool, error) {
	n, err := s.queries().ConsumeOAuthCredential(ctx, hash)
	return n == 1, err
}
