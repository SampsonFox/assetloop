package web

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/SampsonFox/assetloop/internal/config"
	basestore "github.com/SampsonFox/assetloop/internal/store"
	"github.com/SampsonFox/assetloop/internal/store/sqlite"
)

func TestOAuthConsentAndRevocation(t *testing.T) {
	ctx := context.Background()
	cfg := config.Database{Driver: "sqlite", DSN: filepath.Join(t.TempDir(), "consent.db")}
	db, err := basestore.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := basestore.Migrate(ctx, db, cfg); err != nil {
		t.Fatal(err)
	}
	store := sqlite.New(db)
	auth := application.NewAuthService(store)
	account, err := auth.Setup(ctx, application.SetupAuth{TenantName: "Consent space", BaseCurrency: "CNY", Username: "owner", Password: "owner test password"})
	if err != nil {
		t.Fatal(err)
	}
	const issuer = "http://127.0.0.1:8081"
	oauth, err := application.NewOAuthService(store, issuer+"/mcp", []application.OAuthClient{{ID: "test-client", Name: "Test client", RedirectURIs: []string{"http://127.0.0.1/callback"}}})
	if err != nil {
		t.Fatal(err)
	}
	s, err := New(auth, application.NewCatalogService(store), application.NewLifecycleService(store), db, Options{AuthMode: "local", OAuth: oauth, OAuthIssuer: issuer, Specifications: application.NewSpecificationService(store)})
	if err != nil {
		t.Fatal(err)
	}
	h := s.Handler()
	verifier := strings.Repeat("a", 43)
	sum := sha256.Sum256([]byte(verifier))
	form := url.Values{"client_id": {"test-client"}, "response_type": {"code"}, "redirect_uri": {"http://127.0.0.1:49152/callback"}, "resource": {issuer + "/mcp"}, "code_challenge": {base64.RawURLEncoding.EncodeToString(sum[:])}, "code_challenge_method": {"S256"}, "scope": {"assets:read"}, "state": {"round-trip-state"}}
	target := issuer + "/oauth/authorize?" + form.Encode()
	anonymous := request(t, h, "GET", target, nil, nil)
	if anonymous.Code != 303 || !strings.HasPrefix(anonymous.Header().Get("Location"), "/login?return_to=") {
		t.Fatal("missing login continuation")
	}
	cookie := &http.Cookie{Name: sessionCookie, Value: account.Token}
	page := request(t, h, "GET", target, nil, []*http.Cookie{cookie})
	if page.Code != 200 || !strings.Contains(page.Body.String(), "Test client") || !strings.Contains(page.Body.String(), "读取物品") {
		t.Fatalf("consent: %d %s", page.Code, page.Body.String())
	}
	csrf := responseCookie(t, page, csrfCookie)
	form.Set("decision", "allow")
	denied := request(t, h, "POST", issuer+"/oauth/authorize", form, []*http.Cookie{cookie})
	if denied.Code != 403 {
		t.Fatal("consent accepted without CSRF")
	}
	form.Set("csrf_token", csrf.Value)
	allowed := request(t, h, "POST", issuer+"/oauth/authorize", form, []*http.Cookie{cookie, csrf})
	callback, err := url.Parse(allowed.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	if allowed.Code != 303 || callback.Query().Get("state") != "round-trip-state" || callback.Query().Get("iss") != issuer || callback.Query().Get("code") == "" {
		t.Fatal("invalid consent callback")
	}
	tokens, err := oauth.Exchange(ctx, application.OAuthTokenRequest{ClientID: "test-client", Resource: issuer + "/mcp", Code: callback.Query().Get("code"), RedirectURI: form.Get("redirect_uri"), Verifier: verifier})
	if err != nil {
		t.Fatal(err)
	}
	grants, err := oauth.Grants(ctx, account.Principal)
	if err != nil || len(grants) != 1 {
		t.Fatal("missing grant")
	}
	listing := request(t, h, "GET", issuer+"/account/clients", nil, []*http.Cookie{cookie, csrf})
	if listing.Code != 200 || !strings.Contains(listing.Body.String(), "test-client") {
		t.Fatal("missing management entry")
	}
	revoked := request(t, h, "POST", issuer+"/account/clients/"+grants[0].ID+"/revoke", url.Values{"csrf_token": {csrf.Value}}, []*http.Cookie{cookie, csrf})
	if revoked.Code != 303 {
		t.Fatal("revoke failed")
	}
	if _, err := oauth.Authenticate(ctx, tokens.AccessToken); err == nil {
		t.Fatal("revoked token still valid")
	}
	form.Set("decision", "deny")
	refused := request(t, h, "POST", issuer+"/oauth/authorize", form, []*http.Cookie{cookie, csrf})
	u, _ := url.Parse(refused.Header().Get("Location"))
	if refused.Code != 303 || u.Query().Get("error") != "access_denied" {
		t.Fatal("deny failed")
	}
}
