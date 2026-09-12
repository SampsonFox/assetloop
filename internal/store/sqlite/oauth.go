package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/SampsonFox/assetloop/internal/store/sqlite/sqlitedb"
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

	r, err := s.queries().OAuthPrincipal(ctx, sqlitedb.OAuthPrincipalParams{TenantID: tenant, UserID: user})
	if err != nil {
		return application.Principal{}, err
	}
	return application.Principal{TenantID: r.TenantID, UserID: r.UserID, Username: r.Username, TenantName: r.TenantName, Role: application.Role(r.Role), Locale: application.Locale(r.Locale), Theme: application.Theme(r.Theme), Accent: application.Accent(r.Accent)}, nil
}
func (s *Store) PutOAuthGrant(ctx context.Context, g application.OAuthGrant) error {

	return s.queries().PutOAuthGrant(ctx, sqlitedb.PutOAuthGrantParams{ID: g.ID, TenantID: g.TenantID, UserID: g.UserID, ClientID: g.ClientID, Scope: g.Scope, Resource: g.Resource, CreatedAt: sqliteTime(g.CreatedAt), ExpiresAt: sqliteTime(g.ExpiresAt), Revoked: g.Revoked})
}
func oauthGrant(r sqlitedb.OauthGrant) (application.OAuthGrant, error) {
	created, err := parseCatalogTime(r.CreatedAt)
	if err != nil {
		return application.OAuthGrant{}, err
	}
	expires, err := parseCatalogTime(r.ExpiresAt)
	if err != nil {
		return application.OAuthGrant{}, err
	}
	return application.OAuthGrant{ID: r.ID, TenantID: r.TenantID, UserID: r.UserID, ClientID: r.ClientID, Scope: r.Scope, Resource: r.Resource, CreatedAt: created, ExpiresAt: expires, Revoked: r.Revoked}, nil
}
func (s *Store) GetOAuthGrant(ctx context.Context, id string) (application.OAuthGrant, error) {

	r, err := s.queries().GetOAuthGrant(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return application.OAuthGrant{}, application.ErrOAuthGrant
	}
	if err != nil {
		return application.OAuthGrant{}, err
	}
	return oauthGrant(r)
}
func (s *Store) ListOAuthGrants(ctx context.Context, tenant, user string) ([]application.OAuthGrant, error) {

	rows, err := s.queries().ListOAuthGrants(ctx, sqlitedb.ListOAuthGrantsParams{TenantID: tenant, UserID: user})
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
	return s.queries().RevokeOAuthGrant(ctx, id)
}
func (s *Store) PutOAuthCredential(ctx context.Context, c application.OAuthCredential) error {

	return s.queries().PutOAuthCredential(ctx, sqlitedb.PutOAuthCredentialParams{Hash: c.Hash, TenantID: c.TenantID, GrantID: c.GrantID, Kind: c.Kind, RedirectUri: c.RedirectURI, Challenge: c.Challenge, ExpiresAt: sqliteTime(c.ExpiresAt), Consumed: c.Consumed})
}
func (s *Store) GetOAuthCredential(ctx context.Context, hash string) (application.OAuthCredential, error) {
	r, err := s.queries().GetOAuthCredential(ctx, hash)
	if errors.Is(err, sql.ErrNoRows) {
		return application.OAuthCredential{}, application.ErrOAuthGrant
	}
	if err != nil {
		return application.OAuthCredential{}, err
	}
	expires, err := parseCatalogTime(r.ExpiresAt)
	if err != nil {
		return application.OAuthCredential{}, err
	}
	return application.OAuthCredential{Hash: r.Hash, TenantID: r.TenantID, GrantID: r.GrantID, Kind: r.Kind, RedirectURI: r.RedirectUri, Challenge: r.Challenge, ExpiresAt: expires, Consumed: r.Consumed}, nil
}
func (s *Store) ConsumeOAuthCredential(ctx context.Context, hash string) (bool, error) {
	n, err := s.queries().ConsumeOAuthCredential(ctx, hash)
	return n == 1, err
}
