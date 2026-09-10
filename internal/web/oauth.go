package web

import (
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/SampsonFox/assetloop/internal/application"
)

type oauthPageData struct {
	Client     string
	Command    application.OAuthAuthorization
	State      string
	Scopes     []string
	Grants     []application.OAuthGrant
	Management bool
}

func (s *Server) oauthRequest(w http.ResponseWriter, r *http.Request) bool {
	w.Header().Set("Cache-Control", "no-store")
	// Native POSTs need a non-opaque Origin; never send referrers to callbacks.
	w.Header().Set("Referrer-Policy", "same-origin")
	u, err := url.Parse(s.options.OAuthIssuer)
	if err != nil || u.Host == "" || r.Host != u.Host || len(r.Header.Values("Origin")) > 1 || r.Header.Get("Origin") != "" && r.Header.Get("Origin") != s.options.OAuthIssuer {
		http.Error(w, "Invalid origin", 403)
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid request", 400)
		return false
	}
	for key, values := range r.Form {
		// RFC 8707 permits repeated resource indicators; retain one audience.
		if key == "resource" && len(values) > 0 && !slices.ContainsFunc(values, func(value string) bool { return value != values[0] }) {
			continue
		}
		if len(values) != 1 {
			http.Error(w, "Duplicate parameter", 400)
			return false
		}
	}
	return true
}

func (s *Server) oauthAuthorize(w http.ResponseWriter, r *http.Request) {
	if !s.oauthRequest(w, r) {
		return
	}
	cmd := application.OAuthAuthorization{ClientID: r.Form.Get("client_id"), RedirectURI: r.Form.Get("redirect_uri"), Resource: r.Form.Get("resource"), Scope: r.Form.Get("scope"), Challenge: r.Form.Get("code_challenge"), ChallengeMethod: r.Form.Get("code_challenge_method")}
	client, err := s.options.OAuth.ValidateAuthorization(cmd)
	state := r.Form.Get("state")
	if err != nil || r.Form.Get("response_type") != "code" || len(state) > 2048 {
		http.Error(w, "Invalid authorization request", 400)
		return
	}
	// Browsers enforce form-action on the POST's OAuth redirect too. Allow only
	// this validated callback origin, never all loopback ports or arbitrary URLs.
	redirect, _ := url.Parse(cmd.RedirectURI)
	callbackOrigin := redirect.Scheme + "://" + redirect.Host
	if strings.ContainsAny(callbackOrigin, "; \t\r\n") {
		http.Error(w, "Invalid callback origin", 400)
		return
	}
	w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; form-action 'self' "+callbackOrigin+"; frame-ancestors 'none'")
	actor, err := s.principal(r)
	if err != nil {
		if r.Method != "GET" {
			http.Error(w, "Sign in again", 401)
			return
		}
		http.Redirect(w, r, "/login?return_to="+url.QueryEscape(r.URL.RequestURI()), http.StatusSeeOther)
		return
	}
	if r.Method == "POST" {
		if !s.verifyCSRF(w, r) {
			return
		}
		callback, _ := url.Parse(cmd.RedirectURI)
		query := callback.Query()
		query.Set("iss", s.options.OAuthIssuer)
		if state != "" {
			query.Set("state", state)
		}
		switch r.PostForm.Get("decision") {
		case "allow":
			code, err := s.options.OAuth.Authorize(r.Context(), actor, cmd)
			if err != nil {
				http.Error(w, "Authorization failed; check account permissions and retry.", 403)
				return
			}
			query.Set("code", code)
		case "deny":
			query.Set("error", "access_denied")
		default:
			http.Error(w, "Choose allow or deny", 400)
			return
		}
		callback.RawQuery = query.Encode()
		http.Redirect(w, r, callback.String(), http.StatusSeeOther)
		return
	}
	scopes := strings.Fields(cmd.Scope)
	if len(scopes) == 0 {
		scopes = []string{application.OAuthRead}
	}
	s.render(w, 200, "oauth", pageData{Title: "MCP", Principal: &actor, CSRFToken: s.ensureCSRF(w, r), ReturnTo: r.URL.RequestURI(), OAuth: oauthPageData{Client: client.Name, Command: cmd, State: state, Scopes: scopes}})
}

func (s *Server) oauthClients(w http.ResponseWriter, r *http.Request) {
	if !s.oauthRequest(w, r) {
		return
	}
	actor, ok := s.requirePrincipal(w, r)
	if !ok {
		return
	}
	grants, err := s.options.OAuth.Grants(r.Context(), actor)
	if err != nil {
		s.renderError(w, r, 500, err)
		return
	}
	s.render(w, 200, "oauth", pageData{Title: "MCP", Principal: &actor, CSRFToken: s.ensureCSRF(w, r), ReturnTo: r.URL.RequestURI(), OAuth: oauthPageData{Management: true, Grants: grants}})
}

func (s *Server) oauthRevoke(w http.ResponseWriter, r *http.Request) {
	if !s.oauthRequest(w, r) || !s.verifyCSRF(w, r) {
		return
	}
	actor, ok := s.requirePrincipal(w, r)
	if !ok {
		return
	}
	if err := s.options.OAuth.Revoke(r.Context(), actor, r.PathValue("id")); err != nil {
		http.Error(w, "Cannot revoke this authorization", 403)
		return
	}
	http.Redirect(w, r, "/account/clients", http.StatusSeeOther)
}
