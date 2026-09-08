package mcp

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/SampsonFox/assetloop/internal/config"
	basestore "github.com/SampsonFox/assetloop/internal/store"
	"github.com/SampsonFox/assetloop/internal/store/sqlite"
)

func TestOAuthHTTPExchangeAndRevoke(t *testing.T) {
	ctx := context.Background()
	cfg := config.Database{Driver: "sqlite", DSN: filepath.Join(t.TempDir(), "oauth-http.db")}
	db, err := basestore.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := basestore.Migrate(ctx, db, cfg); err != nil {
		t.Fatal(err)
	}
	store := sqlite.New(db)
	account, err := application.NewAuthService(store).Setup(ctx, application.SetupAuth{TenantName: "HTTP test", BaseCurrency: "CNY", Username: "owner", Password: "test only owner password"})
	if err != nil {
		t.Fatal(err)
	}
	const issuer = "http://127.0.0.1:8081"
	service, err := application.NewOAuthService(store, issuer+"/mcp", []application.OAuthClient{{ID: "test", Name: "Test", RedirectURIs: []string{"http://127.0.0.1/callback"}}})
	if err != nil {
		t.Fatal(err)
	}
	o, err := NewOAuthHTTP(service, issuer)
	if err != nil {
		t.Fatal(err)
	}
	verifier := strings.Repeat("a", 43)
	sum := sha256.Sum256([]byte(verifier))
	cmd := application.OAuthAuthorization{ClientID: "test", RedirectURI: "http://127.0.0.1:49152/callback", Resource: issuer + "/mcp", ChallengeMethod: "S256", Challenge: base64.RawURLEncoding.EncodeToString(sum[:]), Scope: ScopeRead}
	code, err := service.Authorize(ctx, account.Principal, cmd)
	if err != nil {
		t.Fatal(err)
	}
	form := url.Values{"grant_type": {"authorization_code"}, "client_id": {"test"}, "resource": {cmd.Resource}, "redirect_uri": {cmd.RedirectURI}, "code_verifier": {verifier}, "code": {code}}
	form.Add("resource", cmd.Resource) // Codex can repeat the discovered resource indicator.
	post := func(path string, form url.Values, handler http.HandlerFunc) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", issuer+path, strings.NewReader(form.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		o.Guard(handler).ServeHTTP(w, r)
		return w
	}
	w := post("/oauth/token", form, o.Token)
	if w.Code != 200 {
		t.Fatalf("exchange: %d %s", w.Code, w.Body.String())
	}
	var tokens application.OAuthTokens
	if err := json.Unmarshal(w.Body.Bytes(), &tokens); err != nil {
		t.Fatal(err)
	}
	if w.Header().Get("Cache-Control") != "no-store" || tokens.AccessToken == "" {
		t.Fatal("missing token or cache protection")
	}
	r := httptest.NewRequest("POST", issuer+"/mcp", nil)
	r.Header.Set("Authorization", "Bearer "+tokens.AccessToken)
	identity, err := o.Authenticate(ctx, r)
	if err != nil || identity.Principal.UserID != account.Principal.UserID {
		t.Fatalf("bearer: %v", err)
	}
	r.URL.RawQuery = "access_token=forbidden"
	if _, err := o.Authenticate(ctx, r); err == nil {
		t.Fatal("query credential accepted")
	}
	r.URL.RawQuery = ""
	w = post("/oauth/revoke", url.Values{"client_id": {"test"}, "token": {tokens.RefreshToken}}, o.Revoke)
	if w.Code != 200 {
		t.Fatalf("revoke: %d", w.Code)
	}
	if _, err := o.Authenticate(ctx, r); err == nil {
		t.Fatal("revoked token accepted")
	}
	for _, path := range []string{"/.well-known/oauth-authorization-server", "/.well-known/oauth-protected-resource/mcp"} {
		w = httptest.NewRecorder()
		o.Guard(http.HandlerFunc(o.Metadata)).ServeHTTP(w, httptest.NewRequest("GET", issuer+path, nil))
		if w.Code != 200 || !strings.Contains(w.Body.String(), issuer) {
			t.Fatal("discovery missing")
		}
	}
	for _, tc := range []struct{ host, origin string }{{"evil.invalid", ""}, {"127.0.0.1:8081", "null"}, {"127.0.0.1:8081", "https://evil.invalid"}} {
		r := httptest.NewRequest("GET", issuer+"/.well-known/oauth-authorization-server", nil)
		r.Host = tc.host
		if tc.origin != "" {
			r.Header.Set("Origin", tc.origin)
		}
		w = httptest.NewRecorder()
		o.Guard(http.HandlerFunc(o.Metadata)).ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatal("host/origin accepted")
		}
	}
}

func TestOAuthFormBoundary(t *testing.T) {
	for _, tc := range []struct{ name, method, query, body, media string }{
		{"GET", "GET", "", "", "application/x-www-form-urlencoded"},
		{"query", "POST", "?code=secret", "client_id=test", "application/x-www-form-urlencoded"},
		{"JSON", "POST", "", `{}`, "application/json"},
		{"duplicate", "POST", "", "client_id=a&client_id=b", "application/x-www-form-urlencoded"},
		{"same client", "POST", "", "client_id=a&client_id=a", "application/x-www-form-urlencoded"},
		{"different resources", "POST", "", "resource=a&resource=b", "application/x-www-form-urlencoded"},
		{"large", "POST", "", strings.Repeat("x", 17<<10), "application/x-www-form-urlencoded"},
		{"secret", "POST", "", "client_secret=not-supported", "application/x-www-form-urlencoded"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, "http://127.0.0.1/oauth/token"+tc.query, strings.NewReader(tc.body))
			r.Header.Set("Content-Type", tc.media)
			w := httptest.NewRecorder()
			if _, ok := oauthForm(w, r); ok {
				t.Fatal("invalid input accepted")
			}
			if w.Code < 400 {
				t.Fatal("missing error response")
			}
		})
	}
}
