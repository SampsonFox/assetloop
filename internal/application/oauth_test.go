package application

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"maps"
	"strings"
	"sync"
	"testing"
	"time"
)

// Transactional test double; persistence compatibility is tested separately.
type oauthMemory struct {
	mu          sync.Mutex
	actor       Principal
	grants      map[string]OAuthGrant
	credentials map[string]OAuthCredential
	failToken   bool
}

func (m *oauthMemory) WithOAuthWrite(ctx context.Context, fn func(OAuthStore) error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	grants, credentials := maps.Clone(m.grants), maps.Clone(m.credentials)
	if err := fn(m); err != nil {
		m.grants, m.credentials = grants, credentials
		return err
	}
	return nil
}
func (m *oauthMemory) OAuthPrincipal(_ context.Context, tenant, user string) (Principal, error) {
	if m.actor.TenantID != tenant || m.actor.UserID != user {
		return Principal{}, ErrUnauthorized
	}
	return m.actor, nil
}
func (m *oauthMemory) PutOAuthGrant(_ context.Context, g OAuthGrant) error {
	m.grants[g.ID] = g
	return nil
}
func (m *oauthMemory) GetOAuthGrant(_ context.Context, id string) (OAuthGrant, error) {
	g, ok := m.grants[id]
	if !ok {
		return g, ErrOAuthGrant
	}
	return g, nil
}
func (m *oauthMemory) ListOAuthGrants(_ context.Context, tenant, user string) ([]OAuthGrant, error) {
	var all []OAuthGrant
	for _, g := range m.grants {
		if g.TenantID == tenant && g.UserID == user {
			all = append(all, g)
		}
	}
	return all, nil
}
func (m *oauthMemory) RevokeOAuthGrant(_ context.Context, id string) error {
	g := m.grants[id]
	g.Revoked = true
	m.grants[id] = g
	return nil
}
func (m *oauthMemory) PutOAuthCredential(_ context.Context, c OAuthCredential) error {
	if m.failToken && c.Kind == "refresh" {
		return errors.New("test write failure")
	}
	m.credentials[c.Hash] = c
	return nil
}
func (m *oauthMemory) GetOAuthCredential(_ context.Context, hash string) (OAuthCredential, error) {
	c, ok := m.credentials[hash]
	if !ok {
		return c, ErrOAuthGrant
	}
	return c, nil
}
func (m *oauthMemory) ConsumeOAuthCredential(_ context.Context, hash string) (bool, error) {
	c, ok := m.credentials[hash]
	if !ok || c.Consumed {
		return false, nil
	}
	c.Consumed = true
	m.credentials[hash] = c
	return true, nil
}

func oauthFixture(t *testing.T) (*OAuthService, *oauthMemory, OAuthAuthorization, OAuthTokenRequest) {
	t.Helper()
	m := &oauthMemory{actor: Principal{TenantID: "space", UserID: "owner", Role: RoleOwner}, grants: map[string]OAuthGrant{}, credentials: map[string]OAuthCredential{}}
	s, err := NewOAuthService(m, "http://127.0.0.1:8081/mcp", []OAuthClient{{ID: "codex-local", Name: "Codex local", RedirectURIs: []string{"http://127.0.0.1/callback"}}})
	if err != nil {
		t.Fatal(err)
	}
	verifier := strings.Repeat("a", 43)
	sum := sha256.Sum256([]byte(verifier))
	cmd := OAuthAuthorization{ClientID: "codex-local", RedirectURI: "http://127.0.0.1:54321/callback", Resource: s.resource, Scope: OAuthRead + " " + OAuthCatalog, ChallengeMethod: "S256", Challenge: base64.RawURLEncoding.EncodeToString(sum[:])}
	return s, m, cmd, OAuthTokenRequest{ClientID: cmd.ClientID, RedirectURI: cmd.RedirectURI, Resource: cmd.Resource, Verifier: verifier}
}

func TestOAuthAuthorizationValidation(t *testing.T) {
	s, m, cmd, _ := oauthFixture(t)
	if _, err := s.ValidateAuthorization(cmd); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*OAuthAuthorization){
		func(c *OAuthAuthorization) { c.ClientID = "unknown" },
		func(c *OAuthAuthorization) { c.Resource += "/other" },
		func(c *OAuthAuthorization) { c.ChallengeMethod = "plain" },
		func(c *OAuthAuthorization) { c.Challenge = "invalid" },
		func(c *OAuthAuthorization) { c.Scope = "members:write" },
		func(c *OAuthAuthorization) { c.RedirectURI = "http://127.0.0.1:54321/other" },
		func(c *OAuthAuthorization) { c.RedirectURI = "http://localhost:54321/callback" },
		func(c *OAuthAuthorization) { c.RedirectURI = "http://127.0.0.1.evil.invalid/callback" },
		func(c *OAuthAuthorization) { c.RedirectURI = "http://127.0.0.1:54321/callback?forward=evil" },
		func(c *OAuthAuthorization) { c.RedirectURI = "http://127.0.0.1:54321/callback#" },
		func(c *OAuthAuthorization) { c.RedirectURI = "http://user@127.0.0.1:54321/callback" },
		func(c *OAuthAuthorization) { c.RedirectURI = "http://127.0.0.1:99999/callback" },
	} {
		bad := cmd
		mutate(&bad)
		if _, err := s.ValidateAuthorization(bad); !errors.Is(err, ErrOAuthRequest) {
			t.Errorf("accepted invalid request: %+v", bad)
		}
	}
	m.actor.Role = RoleViewer
	if _, err := s.Authorize(context.Background(), m.actor, cmd); !errors.Is(err, ErrForbidden) {
		t.Fatalf("viewer received catalog grant: %v", err)
	}
}

func TestOAuthExchangeRotationAndRevocation(t *testing.T) {
	s, m, cmd, request := oauthFixture(t)
	ctx := context.Background()
	code, err := s.Authorize(ctx, m.actor, cmd)
	if err != nil {
		t.Fatal(err)
	}
	request.Code = code
	for _, mutate := range []func(*OAuthTokenRequest){
		func(r *OAuthTokenRequest) { r.Verifier = strings.Repeat("b", 43) },
		func(r *OAuthTokenRequest) { r.RedirectURI = "http://127.0.0.1:54322/callback" },
		func(r *OAuthTokenRequest) { r.Resource += "/other" },
		func(r *OAuthTokenRequest) { r.ClientID = "unknown" },
	} {
		bad := request
		mutate(&bad)
		if _, err := s.Exchange(ctx, bad); !errors.Is(err, ErrOAuthGrant) {
			t.Fatal("invalid exchange accepted")
		}
	}
	tokens, err := s.Exchange(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Authenticate(ctx, tokens.RefreshToken); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("refresh token accepted as bearer")
	}
	if _, err := s.Authenticate(ctx, code); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("code accepted as bearer")
	}
	identity, err := s.Authenticate(ctx, tokens.AccessToken)
	if err != nil || identity.Principal.UserID != m.actor.UserID {
		t.Fatalf("identity: %v", err)
	}
	for hash, c := range m.credentials {
		if hash == code || hash == tokens.AccessToken || hash == tokens.RefreshToken || hash != c.Hash {
			t.Fatal("raw credential persisted")
		}
	}
	request.RefreshToken = tokens.RefreshToken
	rotated, err := s.Refresh(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if rotated.RefreshToken == tokens.RefreshToken {
		t.Fatal("refresh did not rotate")
	}
	if _, err := s.Refresh(ctx, request); !errors.Is(err, ErrOAuthGrant) {
		t.Fatal("refresh replay accepted")
	}
	if _, err := s.Authenticate(ctx, rotated.AccessToken); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("replay revocation rolled back")
	}
}

func TestOAuthFailureRollsBackAndCurrentRoleApplies(t *testing.T) {
	s, m, cmd, request := oauthFixture(t)
	ctx := context.Background()
	code, err := s.Authorize(ctx, m.actor, cmd)
	if err != nil {
		t.Fatal(err)
	}
	request.Code = code
	m.failToken = true
	if _, err := s.Exchange(ctx, request); err == nil {
		t.Fatal("missing write failure")
	}
	if m.credentials[tokenHash(code)].Consumed || len(m.credentials) != 1 {
		t.Fatal("failed exchange partly committed")
	}
	m.failToken = false
	tokens, err := s.Exchange(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	m.actor.Role = RoleViewer
	if _, err := s.Authenticate(ctx, tokens.AccessToken); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("stale owner capability accepted")
	}
	m.actor.Role = RoleOwner
	grants, _ := s.Grants(ctx, m.actor)
	other := m.actor
	other.UserID = "someone-else"
	if err := s.Revoke(ctx, other, grants[0].ID); !errors.Is(err, ErrForbidden) {
		t.Fatal("other user revoked grant")
	}
	if err := s.Revoke(ctx, m.actor, grants[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Authenticate(ctx, tokens.AccessToken); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("revoked bearer accepted")
	}
}

func TestOAuthCodeReplayAndExpiry(t *testing.T) {
	for _, mode := range []string{"code expired", "access expired", "replay"} {
		t.Run(mode, func(t *testing.T) {
			s, m, cmd, request := oauthFixture(t)
			ctx := context.Background()
			now := time.Now().UTC()
			s.now = func() time.Time { return now }
			code, err := s.Authorize(ctx, m.actor, cmd)
			if err != nil {
				t.Fatal(err)
			}
			request.Code = code
			if mode == "code expired" {
				now = now.Add(5 * time.Minute)
				if _, err := s.Exchange(ctx, request); !errors.Is(err, ErrOAuthGrant) {
					t.Fatal("expired code accepted")
				}
				return
			}
			tokens, err := s.Exchange(ctx, request)
			if err != nil {
				t.Fatal(err)
			}
			if mode == "access expired" {
				now = now.Add(10 * time.Minute)
			} else {
				if _, err := s.Exchange(ctx, request); !errors.Is(err, ErrOAuthGrant) {
					t.Fatal("code replay accepted")
				}
			}
			if _, err := s.Authenticate(ctx, tokens.AccessToken); !errors.Is(err, ErrUnauthorized) {
				t.Fatal("expired/revoked access accepted")
			}
		})
	}
}

func TestOAuthTokenRevocationIsClientScoped(t *testing.T) {
	s, m, cmd, request := oauthFixture(t)
	ctx := context.Background()
	s.clients["second-client"] = OAuthClient{ID: "second-client", Name: "Second client"}
	code, err := s.Authorize(ctx, m.actor, cmd)
	if err != nil {
		t.Fatal(err)
	}
	request.Code = code
	tokens, err := s.Exchange(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RevokeToken(ctx, "second-client", tokens.RefreshToken); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Authenticate(ctx, tokens.AccessToken); err != nil {
		t.Fatal("different client revoked grant")
	}
	if err := s.RevokeToken(ctx, cmd.ClientID, "unknown"); err != nil {
		t.Fatal(err)
	}
	if err := s.RevokeToken(ctx, cmd.ClientID, tokens.RefreshToken); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Authenticate(ctx, tokens.AccessToken); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("revoked access accepted")
	}
}

func TestOAuthConcurrentCodeExchange(t *testing.T) {
	s, m, cmd, request := oauthFixture(t)
	ctx := context.Background()
	code, err := s.Authorize(ctx, m.actor, cmd)
	if err != nil {
		t.Fatal(err)
	}
	request.Code = code
	results := make(chan error, 2)
	for range 2 {
		go func() { _, err := s.Exchange(ctx, request); results <- err }()
	}
	first, second := <-results, <-results
	if (first == nil) == (second == nil) {
		t.Fatalf("want exactly one successful exchange: %v / %v", first, second)
	}
	for _, grant := range m.grants {
		if !grant.Revoked {
			t.Fatal("concurrent replay did not revoke grant")
		}
	}
}
