package application

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"net"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	OAuthRead      = "assets:read"
	OAuthCatalog   = "assets:catalog"
	OAuthLifecycle = "assets:lifecycle"
)

var ErrOAuthRequest = errors.New("invalid OAuth request")
var ErrOAuthGrant = errors.New("invalid or expired OAuth grant")
var ErrOAuthScope = errors.New("invalid OAuth scope")

// OAuthStore implementations serialize credential exchanges in a transaction.
// Consumed credentials are retained until grant expiry for replay detection.
// Missing grants/credentials return ErrOAuthGrant, not an infrastructure error.
type OAuthStore interface {
	WithOAuthWrite(context.Context, func(OAuthStore) error) error
	OAuthPrincipal(context.Context, string, string) (Principal, error)
	PutOAuthGrant(context.Context, OAuthGrant) error
	GetOAuthGrant(context.Context, string) (OAuthGrant, error)
	ListOAuthGrants(context.Context, string, string) ([]OAuthGrant, error)
	RevokeOAuthGrant(context.Context, string) error
	PutOAuthCredential(context.Context, OAuthCredential) error
	GetOAuthCredential(context.Context, string) (OAuthCredential, error)
	ConsumeOAuthCredential(context.Context, string) (bool, error)
}

type OAuthGrant struct {
	ID, TenantID, UserID, ClientID, Scope, Resource string
	CreatedAt, ExpiresAt                            time.Time
	Revoked                                         bool
}

// Only hashes cross the persistence boundary; never persist raw codes/tokens.
type OAuthCredential struct {
	Hash, TenantID, GrantID, Kind, RedirectURI, Challenge string
	ExpiresAt                                             time.Time
	Consumed                                              bool
}

type OAuthClient struct {
	ID, Name     string
	RedirectURIs []string
}
type OAuthAuthorization struct{ ClientID, RedirectURI, Resource, Scope, Challenge, ChallengeMethod string }
type OAuthTokenRequest struct{ ClientID, Resource, Code, RedirectURI, Verifier, RefreshToken, Scope string }
type OAuthTokens struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
}
type OAuthIdentity struct {
	Principal Principal
	Scopes    []string
}

type OAuthService struct {
	store    OAuthStore
	resource string
	clients  map[string]OAuthClient
	now      func() time.Time
}

func (s *OAuthService) Resource() string { return s.resource }

func NewOAuthService(store OAuthStore, resource string, clients []OAuthClient) (*OAuthService, error) {
	u, err := url.Parse(resource)
	if err != nil || !validOAuthRedirect(resource) || u.RawQuery != "" || u.ForceQuery {
		return nil, ErrOAuthRequest
	}
	s := &OAuthService{store: store, resource: resource, clients: map[string]OAuthClient{}, now: time.Now}
	for _, client := range clients {
		if client.ID == "" || client.Name == "" || len(client.RedirectURIs) == 0 {
			return nil, ErrOAuthRequest
		}
		if _, exists := s.clients[client.ID]; exists {
			return nil, ErrOAuthRequest
		}
		for _, redirect := range client.RedirectURIs {
			if !validOAuthRedirect(redirect) {
				return nil, ErrOAuthRequest
			}
		}
		client.RedirectURIs = slices.Clone(client.RedirectURIs)
		s.clients[client.ID] = client
	}
	return s, nil
}

// ValidateAuthorization is safe before login; it never grants authority.
func (s *OAuthService) ValidateAuthorization(cmd OAuthAuthorization) (OAuthClient, error) {
	client, ok := s.clients[cmd.ClientID]
	if !ok || cmd.Resource != s.resource || cmd.ChallengeMethod != "S256" {
		return OAuthClient{}, ErrOAuthRequest
	}
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(cmd.Challenge)
	if err != nil || len(decoded) != sha256.Size {
		return OAuthClient{}, ErrOAuthRequest
	}
	match := false
	for _, registered := range client.RedirectURIs {
		match = match || matchingOAuthRedirect(registered, cmd.RedirectURI)
	}
	if !match {
		return OAuthClient{}, ErrOAuthRequest
	}
	if _, err := oauthScopes(cmd.Scope); err != nil {
		return OAuthClient{}, err
	}
	client.RedirectURIs = slices.Clone(client.RedirectURIs)
	return client, nil
}

func (s *OAuthService) Authorize(ctx context.Context, actor Principal, cmd OAuthAuthorization) (string, error) {
	if err := actor.Require(CapabilityView); err != nil {
		return "", err
	}
	if _, err := s.ValidateAuthorization(cmd); err != nil {
		return "", err
	}
	scopes, _ := oauthScopes(cmd.Scope)
	code := oauthSecret()
	now := s.now().UTC()
	err := s.store.WithOAuthWrite(ctx, func(store OAuthStore) error {
		current, err := store.OAuthPrincipal(ctx, actor.TenantID, actor.UserID)
		if err != nil || !oauthAllows(current, scopes) {
			return ErrForbidden
		}
		grant := OAuthGrant{ID: newID(), TenantID: current.TenantID, UserID: current.UserID, ClientID: cmd.ClientID, Scope: strings.Join(scopes, " "), Resource: s.resource, CreatedAt: now, ExpiresAt: now.Add(30 * 24 * time.Hour)}
		if err := store.PutOAuthGrant(ctx, grant); err != nil {
			return err
		}
		return store.PutOAuthCredential(ctx, OAuthCredential{Hash: tokenHash(code), TenantID: grant.TenantID, GrantID: grant.ID, Kind: "code", RedirectURI: cmd.RedirectURI, Challenge: cmd.Challenge, ExpiresAt: now.Add(5 * time.Minute)})
	})
	if err != nil {
		return "", err
	}
	return code, nil
}

func (s *OAuthService) Exchange(ctx context.Context, cmd OAuthTokenRequest) (OAuthTokens, error) {
	return s.exchange(ctx, cmd, false)
}
func (s *OAuthService) Refresh(ctx context.Context, cmd OAuthTokenRequest) (OAuthTokens, error) {
	return s.exchange(ctx, cmd, true)
}

func (s *OAuthService) exchange(ctx context.Context, cmd OAuthTokenRequest, refresh bool) (OAuthTokens, error) {
	if _, ok := s.clients[cmd.ClientID]; !ok || cmd.Resource != s.resource {
		return OAuthTokens{}, ErrOAuthGrant
	}
	secret, kind := cmd.Code, "code"
	if refresh {
		secret, kind = cmd.RefreshToken, "refresh"
	}
	if len(secret) != 43 {
		return OAuthTokens{}, ErrOAuthGrant
	}
	var result OAuthTokens
	var rejected bool
	now := s.now().UTC()
	err := s.store.WithOAuthWrite(ctx, func(store OAuthStore) error {
		credential, err := store.GetOAuthCredential(ctx, tokenHash(secret))
		if err != nil || credential.Kind != kind || !now.Before(credential.ExpiresAt) {
			return ErrOAuthGrant
		}
		grant, err := store.GetOAuthGrant(ctx, credential.GrantID)
		if err != nil || grant.Revoked || !now.Before(grant.ExpiresAt) || grant.ClientID != cmd.ClientID || grant.Resource != s.resource {
			return ErrOAuthGrant
		}
		if !refresh && (credential.RedirectURI != cmd.RedirectURI || !verifyOAuthPKCE(cmd.Verifier, credential.Challenge)) {
			return ErrOAuthGrant
		}
		if refresh && cmd.Scope != "" {
			requested, err := oauthScopes(cmd.Scope)
			// Refresh retains the consented scope. Explicitly repeating it is
			// valid; a different scope must not be silently ignored or granted.
			if err != nil || len(strings.Fields(cmd.Scope)) == 0 || !slices.Equal(requested, strings.Fields(grant.Scope)) {
				return ErrOAuthScope
			}
		}
		if credential.Consumed {
			// Commit revocation before reporting replay; returning an error here
			// would roll back the very revocation intended to stop the attacker.
			rejected = true
			return store.RevokeOAuthGrant(ctx, grant.ID)
		}
		actor, err := store.OAuthPrincipal(ctx, grant.TenantID, grant.UserID)
		if err != nil || !oauthAllows(actor, strings.Fields(grant.Scope)) {
			return ErrOAuthGrant
		}
		consumed, err := store.ConsumeOAuthCredential(ctx, credential.Hash)
		if err != nil {
			return err
		}
		if !consumed {
			return ErrOAuthGrant
		}
		result = OAuthTokens{AccessToken: oauthSecret(), TokenType: "Bearer", ExpiresIn: 600, RefreshToken: oauthSecret(), Scope: grant.Scope}
		accessExpiry := now.Add(10 * time.Minute)
		if grant.ExpiresAt.Before(accessExpiry) {
			accessExpiry = grant.ExpiresAt
			result.ExpiresIn = int(accessExpiry.Sub(now).Seconds())
		}
		if err := store.PutOAuthCredential(ctx, OAuthCredential{Hash: tokenHash(result.AccessToken), TenantID: grant.TenantID, GrantID: grant.ID, Kind: "access", ExpiresAt: accessExpiry}); err != nil {
			return err
		}
		return store.PutOAuthCredential(ctx, OAuthCredential{Hash: tokenHash(result.RefreshToken), TenantID: grant.TenantID, GrantID: grant.ID, Kind: "refresh", ExpiresAt: grant.ExpiresAt})
	})
	if err != nil {
		return OAuthTokens{}, err
	}
	if rejected {
		return OAuthTokens{}, ErrOAuthGrant
	}
	return result, nil
}

func (s *OAuthService) Authenticate(ctx context.Context, token string) (OAuthIdentity, error) {
	if len(token) != 43 {
		return OAuthIdentity{}, ErrUnauthorized
	}
	credential, err := s.store.GetOAuthCredential(ctx, tokenHash(token))
	now := s.now().UTC()
	if err != nil || credential.Kind != "access" || credential.Consumed || !now.Before(credential.ExpiresAt) {
		return OAuthIdentity{}, ErrUnauthorized
	}
	grant, err := s.store.GetOAuthGrant(ctx, credential.GrantID)
	if err != nil || grant.Revoked || grant.Resource != s.resource || !now.Before(grant.ExpiresAt) {
		return OAuthIdentity{}, ErrUnauthorized
	}
	if _, registered := s.clients[grant.ClientID]; !registered {
		return OAuthIdentity{}, ErrUnauthorized
	}
	actor, err := s.store.OAuthPrincipal(ctx, grant.TenantID, grant.UserID)
	scopes := strings.Fields(grant.Scope)
	if err != nil || !oauthAllows(actor, scopes) {
		return OAuthIdentity{}, ErrUnauthorized
	}
	return OAuthIdentity{Principal: actor, Scopes: scopes}, nil
}

func (s *OAuthService) Grants(ctx context.Context, actor Principal) ([]OAuthGrant, error) {
	if err := actor.Require(CapabilityView); err != nil {
		return nil, err
	}
	return s.store.ListOAuthGrants(ctx, actor.TenantID, actor.UserID)
}

func (s *OAuthService) Revoke(ctx context.Context, actor Principal, id string) error {
	if err := actor.Require(CapabilityView); err != nil {
		return err
	}
	return s.store.WithOAuthWrite(ctx, func(store OAuthStore) error {
		grant, err := store.GetOAuthGrant(ctx, id)
		if err != nil || grant.TenantID != actor.TenantID || grant.UserID != actor.UserID {
			return ErrForbidden
		}
		return store.RevokeOAuthGrant(ctx, id)
	})
}

// RevokeToken follows RFC 7009: unknown tokens succeed without revealing whether
// another client's credential exists. A valid client token revokes its grant.
func (s *OAuthService) RevokeToken(ctx context.Context, clientID, token string) error {
	if _, ok := s.clients[clientID]; !ok {
		return ErrOAuthRequest
	}
	if len(token) != 43 {
		return nil
	}
	return s.store.WithOAuthWrite(ctx, func(store OAuthStore) error {
		credential, err := store.GetOAuthCredential(ctx, tokenHash(token))
		if errors.Is(err, ErrOAuthGrant) {
			return nil
		}
		if err != nil {
			return err
		}
		if credential.Kind != "access" && credential.Kind != "refresh" {
			return nil
		}
		grant, err := store.GetOAuthGrant(ctx, credential.GrantID)
		if errors.Is(err, ErrOAuthGrant) {
			return nil
		}
		if err != nil {
			return err
		}
		if grant.ClientID != clientID || grant.Resource != s.resource {
			return nil
		}
		return store.RevokeOAuthGrant(ctx, grant.ID)
	})
}

func oauthSecret() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand unavailable")
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
func isLoopbackIP(host string) bool { ip := net.ParseIP(host); return ip != nil && ip.IsLoopback() }
func validOAuthRedirect(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil || strings.Contains(raw, "#") || u.Opaque != "" {
		return false
	}
	if u.Port() != "" {
		port, err := strconv.Atoi(u.Port())
		if err != nil || port < 1 || port > 65535 {
			return false
		}
	}
	return u.Scheme == "https" || (u.Scheme == "http" && isLoopbackIP(u.Hostname()))
}
func matchingOAuthRedirect(registered, requested string) bool {
	if !validOAuthRedirect(requested) {
		return false
	}
	if registered == requested {
		return true
	}
	a, _ := url.Parse(registered)
	b, _ := url.Parse(requested)
	if a.Scheme != "http" || b.Scheme != "http" || !isLoopbackIP(a.Hostname()) || a.Hostname() != b.Hostname() {
		return false
	}
	// RFC 8252 permits only the port to vary, not path/query/host/escaping.
	a.Host = b.Host
	return a.String() == b.String()
}
func verifyOAuthPKCE(verifier, challenge string) bool {
	if len(verifier) < 43 || len(verifier) > 128 {
		return false
	}
	for _, ch := range verifier {
		if !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || strings.ContainsRune("-._~", ch)) {
			return false
		}
	}
	sum := sha256.Sum256([]byte(verifier))
	return subtle.ConstantTimeCompare([]byte(base64.RawURLEncoding.EncodeToString(sum[:])), []byte(challenge)) == 1
}
func oauthScopes(scope string) ([]string, error) {
	scopes := strings.Fields(scope)
	if len(scopes) == 0 {
		scopes = []string{OAuthRead}
	}
	for _, item := range scopes {
		if item != OAuthRead && item != OAuthCatalog && item != OAuthLifecycle {
			return nil, ErrOAuthRequest
		}
	}
	slices.Sort(scopes)
	return slices.Compact(scopes), nil
}
func oauthAllows(actor Principal, scopes []string) bool {
	if actor.Require(CapabilityView) != nil {
		return false
	}
	for _, scope := range scopes {
		capability := CapabilityView
		switch scope {
		case OAuthRead:
		case OAuthCatalog:
			capability = CapabilityManageAssets
		case OAuthLifecycle:
			capability = CapabilityManageLifecycle
		default:
			return false
		}
		if actor.Require(capability) != nil {
			return false
		}
	}
	return true
}
