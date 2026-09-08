package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"mime"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/SampsonFox/assetloop/internal/application"
)

// OAuthHTTP contains protocol formatting only; credentials are managed by the
// shared application service. Issuer comes from configuration, never Host headers.
type OAuthHTTP struct {
	service *application.OAuthService
	issuer  string
	host    string
}

func NewOAuthHTTP(service *application.OAuthService, issuer string) (*OAuthHTTP, error) {
	u, err := url.Parse(issuer)
	if err != nil || service == nil || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.ForceQuery || strings.Contains(issuer, "#") {
		return nil, application.ErrOAuthRequest
	}
	ip := net.ParseIP(u.Hostname())
	if u.Scheme != "https" && !(u.Scheme == "http" && ip != nil && ip.IsLoopback()) {
		return nil, application.ErrOAuthRequest
	}
	if service.Resource() != issuer+"/mcp" {
		return nil, application.ErrOAuthRequest
	}
	return &OAuthHTTP{service: service, issuer: issuer, host: u.Host}, nil
}

// Guard applies to metadata, OAuth and MCP routes. Browser cross-origin calls
// and DNS-rebinding Host values are rejected; native clients need no Origin.
func (o *OAuthHTTP) Guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		origins := r.Header.Values("Origin")
		if r.Host != o.host || len(origins) > 1 || len(origins) == 1 && origins[0] != o.issuer {
			http.Error(w, "Origin not allowed", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (o *OAuthHTTP) Metadata(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		oauthJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "invalid_request"})
		return
	}
	scopes := []string{ScopeRead, ScopeCatalog, ScopeLifecycle}
	switch r.URL.Path {
	case "/.well-known/oauth-authorization-server":
		oauthJSON(w, http.StatusOK, map[string]any{
			"issuer": o.issuer, "authorization_endpoint": o.issuer + "/oauth/authorize",
			"token_endpoint": o.issuer + "/oauth/token", "revocation_endpoint": o.issuer + "/oauth/revoke",
			"response_types_supported": []string{"code"}, "grant_types_supported": []string{"authorization_code", "refresh_token"},
			"code_challenge_methods_supported": []string{"S256"}, "token_endpoint_auth_methods_supported": []string{"none"},
			"revocation_endpoint_auth_methods_supported": []string{"none"}, "scopes_supported": scopes,
			"authorization_response_iss_parameter_supported": true,
		})
	case "/.well-known/oauth-protected-resource", "/.well-known/oauth-protected-resource/mcp":
		oauthJSON(w, http.StatusOK, map[string]any{"resource": o.service.Resource(), "authorization_servers": []string{o.issuer}, "scopes_supported": scopes, "bearer_methods_supported": []string{"header"}})
	default:
		http.NotFound(w, r)
	}
}

func (o *OAuthHTTP) Token(w http.ResponseWriter, r *http.Request) {
	form, ok := oauthForm(w, r)
	if !ok {
		return
	}
	request := application.OAuthTokenRequest{ClientID: form.Get("client_id"), Resource: form.Get("resource"), Code: form.Get("code"), RedirectURI: form.Get("redirect_uri"), Verifier: form.Get("code_verifier"), RefreshToken: form.Get("refresh_token")}
	var tokens application.OAuthTokens
	var err error
	switch form.Get("grant_type") {
	case "authorization_code":
		tokens, err = o.service.Exchange(r.Context(), request)
	case "refresh_token":
		// First phase retains the consented scope; do not silently ignore a
		// client's requested scope change during refresh.
		if form.Get("scope") != "" {
			oauthJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_scope"})
			return
		}
		tokens, err = o.service.Refresh(r.Context(), request)
	default:
		oauthJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported_grant_type"})
		return
	}
	if err != nil {
		oauthError(w, err)
		return
	}
	oauthJSON(w, http.StatusOK, tokens)
}

func (o *OAuthHTTP) Revoke(w http.ResponseWriter, r *http.Request) {
	form, ok := oauthForm(w, r)
	if !ok {
		return
	}
	if form.Get("token") == "" {
		oauthJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return
	}
	if err := o.service.RevokeToken(r.Context(), form.Get("client_id"), form.Get("token")); err != nil {
		oauthError(w, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (o *OAuthHTTP) Authenticate(ctx context.Context, r *http.Request) (Identity, error) {
	if len(r.Header.Values("Authorization")) != 1 || r.URL.Query().Has("access_token") {
		return Identity{}, application.ErrUnauthorized
	}
	parts := strings.Fields(r.Header.Get("Authorization"))
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return Identity{}, application.ErrUnauthorized
	}
	identity, err := o.service.Authenticate(ctx, parts[1])
	if err != nil {
		return Identity{}, err
	}
	return Identity{Principal: identity.Principal, Scopes: identity.Scopes}, nil
}

func (o *OAuthHTTP) Protected(next http.Handler) http.Handler {
	return o.Guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Set on the eventual 401 generated by the fail-closed MCP verifier.
		w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="`+o.issuer+`/.well-known/oauth-protected-resource/mcp", scope="`+ScopeRead+`"`)
		next.ServeHTTP(w, r)
	}))
}

func oauthForm(w http.ResponseWriter, r *http.Request) (url.Values, bool) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		oauthJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "invalid_request"})
		return nil, false
	}
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/x-www-form-urlencoded" || r.URL.RawQuery != "" || r.Header.Get("Authorization") != "" {
		oauthJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return nil, false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	if err := r.ParseForm(); err != nil {
		oauthJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return nil, false
	}
	for key, values := range r.PostForm {
		// RFC 8707 permits repeated resource indicators; retain one audience.
		if key == "resource" && len(values) > 0 && !slices.ContainsFunc(values, func(value string) bool { return value != values[0] }) {
			continue
		}
		if len(values) != 1 {
			oauthJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
			return nil, false
		}
	}
	if r.PostForm.Get("client_secret") != "" {
		oauthJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_client"})
		return nil, false
	}
	return r.PostForm, true
}
func oauthError(w http.ResponseWriter, err error) {
	code, status := "server_error", http.StatusInternalServerError
	switch {
	case errors.Is(err, application.ErrOAuthGrant):
		code, status = "invalid_grant", http.StatusBadRequest
	case errors.Is(err, application.ErrOAuthRequest):
		code, status = "invalid_request", http.StatusBadRequest
	case errors.Is(err, application.ErrForbidden):
		code, status = "access_denied", http.StatusForbidden
	}
	oauthJSON(w, status, map[string]string{"error": code})
}
func oauthJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
