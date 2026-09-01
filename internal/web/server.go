package web

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/base64"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/SampsonFox/assetloop/internal/application"
)

const (
	sessionCookie = "assetloop_session"
	csrfCookie    = "assetloop_csrf"
)

//go:embed templates/*.html static/*
var assets embed.FS

type Pinger interface {
	PingContext(context.Context) error
}

type Options struct {
	AuthMode          string
	SecureCookies     bool
	DisabledPrincipal application.Principal
}

type Server struct {
	auth      *application.AuthService
	db        Pinger
	options   Options
	templates map[string]*template.Template
	limiter   *loginLimiter
}

type pageData struct {
	Title     string
	CSRFToken string
	Error     string
	Principal *application.Principal
	Members   []application.Member
}

func New(auth *application.AuthService, db Pinger, options Options) (*Server, error) {
	templates := map[string]*template.Template{}
	for _, page := range []string{"setup", "login", "dashboard", "members", "error"} {
		parsed, err := template.ParseFS(assets, "templates/base.html", "templates/"+page+".html")
		if err != nil {
			return nil, fmt.Errorf("parse %s template: %w", page, err)
		}
		templates[page] = parsed
	}
	return &Server{auth: auth, db: db, options: options, templates: templates, limiter: newLoginLimiter(5, 5*time.Minute)}, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("GET /", s.dashboard)
	mux.HandleFunc("GET /setup", s.setupForm)
	mux.HandleFunc("POST /setup", s.setup)
	mux.HandleFunc("GET /login", s.loginForm)
	mux.HandleFunc("POST /login", s.login)
	mux.HandleFunc("POST /logout", s.logout)
	mux.HandleFunc("GET /admin/members", s.members)
	mux.HandleFunc("POST /admin/members", s.addMember)
	staticFS, _ := fs.Sub(assets, "static")
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))
	return securityHeaders(mux)
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	if err := s.db.PingContext(r.Context()); err != nil {
		http.Error(w, "database unavailable", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("ok\n"))
}

func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.requirePrincipal(w, r)
	if !ok {
		return
	}
	s.render(w, http.StatusOK, "dashboard", pageData{Title: "概览", CSRFToken: s.ensureCSRF(w, r), Principal: &principal})
}

func (s *Server) setupForm(w http.ResponseWriter, r *http.Request) {
	needsSetup, err := s.auth.NeedsSetup(r.Context())
	if err != nil {
		s.renderError(w, http.StatusInternalServerError, err)
		return
	}
	if !needsSetup {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	s.render(w, http.StatusOK, "setup", pageData{Title: "初始化", CSRFToken: s.ensureCSRF(w, r)})
}

func (s *Server) setup(w http.ResponseWriter, r *http.Request) {
	if !s.verifyCSRF(w, r) {
		return
	}
	credential, err := s.auth.Setup(r.Context(), application.SetupAuth{
		TenantName: r.FormValue("tenant_name"), BaseCurrency: r.FormValue("base_currency"),
		Username: r.FormValue("username"), Password: r.FormValue("password"),
	})
	if err != nil {
		s.render(w, http.StatusUnprocessableEntity, "setup", pageData{Title: "初始化", CSRFToken: s.ensureCSRF(w, r), Error: err.Error()})
		return
	}
	s.setSessionCookie(w, credential)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) loginForm(w http.ResponseWriter, r *http.Request) {
	needsSetup, err := s.auth.NeedsSetup(r.Context())
	if err != nil {
		s.renderError(w, http.StatusInternalServerError, err)
		return
	}
	if needsSetup {
		http.Redirect(w, r, "/setup", http.StatusSeeOther)
		return
	}
	if _, err := s.principal(r); err == nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	s.render(w, http.StatusOK, "login", pageData{Title: "登录", CSRFToken: s.ensureCSRF(w, r)})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if !s.verifyCSRF(w, r) {
		return
	}
	key := clientIP(r)
	if !s.limiter.Allow(key, time.Now()) {
		http.Error(w, "登录尝试过多，请稍后再试", http.StatusTooManyRequests)
		return
	}
	credential, err := s.auth.Login(r.Context(), application.Login{Username: r.FormValue("username"), Password: r.FormValue("password")})
	if err != nil {
		s.render(w, http.StatusUnauthorized, "login", pageData{Title: "登录", CSRFToken: s.ensureCSRF(w, r), Error: "用户名或密码错误"})
		return
	}
	s.limiter.Reset(key)
	s.setSessionCookie(w, credential)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if !s.verifyCSRF(w, r) {
		return
	}
	if cookie, err := r.Cookie(sessionCookie); err == nil {
		_ = s.auth.Logout(r.Context(), cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Path: "/", MaxAge: -1, HttpOnly: true, Secure: s.options.SecureCookies, SameSite: http.SameSiteLaxMode})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (s *Server) members(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.requirePrincipal(w, r)
	if !ok {
		return
	}
	members, err := s.auth.ListMembers(r.Context(), principal)
	if errors.Is(err, application.ErrForbidden) {
		s.render(w, http.StatusForbidden, "error", pageData{Title: "无权限", CSRFToken: s.ensureCSRF(w, r), Principal: &principal, Error: "只有 Owner 可以管理成员"})
		return
	}
	if err != nil {
		s.renderError(w, http.StatusInternalServerError, err)
		return
	}
	s.render(w, http.StatusOK, "members", pageData{Title: "成员", CSRFToken: s.ensureCSRF(w, r), Principal: &principal, Members: members})
}

func (s *Server) addMember(w http.ResponseWriter, r *http.Request) {
	if !s.verifyCSRF(w, r) {
		return
	}
	principal, ok := s.requirePrincipal(w, r)
	if !ok {
		return
	}
	_, err := s.auth.AddMember(r.Context(), principal, application.AddMember{Username: r.FormValue("username"), Password: r.FormValue("password"), Role: application.Role(r.FormValue("role"))})
	if errors.Is(err, application.ErrForbidden) {
		s.render(w, http.StatusForbidden, "error", pageData{Title: "无权限", CSRFToken: s.ensureCSRF(w, r), Principal: &principal, Error: "只有 Owner 可以管理成员"})
		return
	}
	if err != nil {
		members, _ := s.auth.ListMembers(r.Context(), principal)
		s.render(w, http.StatusUnprocessableEntity, "members", pageData{Title: "成员", CSRFToken: s.ensureCSRF(w, r), Principal: &principal, Members: members, Error: err.Error()})
		return
	}
	http.Redirect(w, r, "/admin/members", http.StatusSeeOther)
}

func (s *Server) principal(r *http.Request) (application.Principal, error) {
	if s.options.AuthMode == "disabled" {
		return s.options.DisabledPrincipal, nil
	}
	cookie, err := r.Cookie(sessionCookie)
	if err != nil {
		return application.Principal{}, application.ErrUnauthorized
	}
	return s.auth.Authenticate(r.Context(), cookie.Value)
}

func (s *Server) requirePrincipal(w http.ResponseWriter, r *http.Request) (application.Principal, bool) {
	principal, err := s.principal(r)
	if err == nil {
		return principal, true
	}
	http.Redirect(w, r, "/login", http.StatusSeeOther)
	return application.Principal{}, false
}

func (s *Server) ensureCSRF(w http.ResponseWriter, r *http.Request) string {
	if cookie, err := r.Cookie(csrfCookie); err == nil && cookie.Value != "" {
		return cookie.Value
	}
	value := randomToken()
	http.SetCookie(w, &http.Cookie{Name: csrfCookie, Value: value, Path: "/", HttpOnly: true, Secure: s.options.SecureCookies, SameSite: http.SameSiteLaxMode})
	return value
}

func (s *Server) verifyCSRF(w http.ResponseWriter, r *http.Request) bool {
	cookie, err := r.Cookie(csrfCookie)
	if err != nil || cookie.Value == "" {
		http.Error(w, "invalid CSRF token", http.StatusForbidden)
		return false
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return false
	}
	want, got := []byte(cookie.Value), []byte(r.FormValue("csrf_token"))
	if len(want) != len(got) || subtle.ConstantTimeCompare(want, got) != 1 {
		http.Error(w, "invalid CSRF token", http.StatusForbidden)
		return false
	}
	return true
}

func (s *Server) setSessionCookie(w http.ResponseWriter, credential application.SessionCredential) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: credential.Token, Path: "/", Expires: credential.ExpiresAt, HttpOnly: true, Secure: s.options.SecureCookies, SameSite: http.SameSiteLaxMode})
}

func (s *Server) render(w http.ResponseWriter, status int, name string, data pageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := s.templates[name].ExecuteTemplate(w, "base", data); err != nil {
		panic(err)
	}
}

func (s *Server) renderError(w http.ResponseWriter, status int, err error) {
	s.render(w, status, "error", pageData{Title: "错误", Error: err.Error()})
}

func randomToken() string {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		panic("crypto/rand unavailable: " + err.Error())
	}
	return base64.RawURLEncoding.EncodeToString(value)
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return strings.TrimSpace(r.RemoteAddr)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; form-action 'self'; frame-ancestors 'none'")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		next.ServeHTTP(w, r)
	})
}

type loginAttempt struct {
	count int
	until time.Time
}

type loginLimiter struct {
	mu     sync.Mutex
	items  map[string]loginAttempt
	limit  int
	window time.Duration
}

func newLoginLimiter(limit int, window time.Duration) *loginLimiter {
	return &loginLimiter{items: map[string]loginAttempt{}, limit: limit, window: window}
}

func (l *loginLimiter) Allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	attempt := l.items[key]
	if !attempt.until.After(now) {
		attempt = loginAttempt{until: now.Add(l.window)}
	}
	if attempt.count >= l.limit {
		l.items[key] = attempt
		return false
	}
	attempt.count++
	l.items[key] = attempt
	return true
}

func (l *loginLimiter) Reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.items, key)
}
