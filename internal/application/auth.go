package application

import (
	"context"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

type Role string

const (
	RoleOwner  Role = "owner"
	RoleEditor Role = "editor"
	RoleViewer Role = "viewer"
)

type Locale string

const (
	LocaleZhCN Locale = "zh-CN"
	LocaleEn   Locale = "en"
)

type Theme string

const (
	ThemeSystem Theme = "system"
	ThemeLight  Theme = "light"
	ThemeDark   Theme = "dark"
)

type Capability string

const (
	CapabilityView            Capability = "view"
	CapabilityManageCatalog   Capability = "manage_catalog"
	CapabilityManageLifecycle Capability = "manage_lifecycle"
	CapabilityManageMembers   Capability = "manage_members"
	CapabilityManageSettings  Capability = "manage_settings"
)

var (
	ErrUnauthorized  = errors.New("authentication required")
	ErrForbidden     = errors.New("permission denied")
	ErrSetupComplete = errors.New("initial setup is already complete")
	ErrInvalidLogin  = errors.New("invalid username or password")
)

type Principal struct {
	TenantID   string
	TenantName string
	UserID     string
	Username   string
	Role       Role
	Locale     Locale
	Theme      Theme
}

func (p Principal) Can(capability Capability) bool {
	switch p.Role {
	case RoleOwner:
		return true
	case RoleEditor:
		return capability == CapabilityView || capability == CapabilityManageCatalog || capability == CapabilityManageLifecycle
	case RoleViewer:
		return capability == CapabilityView
	default:
		return false
	}
}

func (p Principal) Require(capability Capability) error {
	if strings.TrimSpace(p.UserID) == "" || strings.TrimSpace(p.TenantID) == "" {
		return ErrUnauthorized
	}
	if !p.Can(capability) {
		return ErrForbidden
	}
	return nil
}

type Tenant struct {
	ID           string
	Name         string
	BaseCurrency string
	CreatedAt    time.Time
}

type User struct {
	ID                 string
	Username           string
	UsernameNormalized string
	PasswordHash       string
	Locale             Locale
	Theme              Theme
	CreatedAt          time.Time
}

type Membership struct {
	TenantID  string
	UserID    string
	Role      Role
	CreatedAt time.Time
}

type Account struct {
	Principal
	PasswordHash string
}

type Session struct {
	TokenHash string
	TenantID  string
	UserID    string
	ExpiresAt time.Time
	CreatedAt time.Time
}

type Member struct {
	UserID    string
	Username  string
	Role      Role
	CreatedAt time.Time
}

type SecurityEvent struct {
	ID           string
	TenantID     string
	ActorUserID  string
	Action       string
	TargetUserID string
	Detail       string
	OccurredAt   time.Time
}

type SessionCredential struct {
	Token     string
	Principal Principal
	ExpiresAt time.Time
}

type SetupAuth struct {
	TenantName   string
	BaseCurrency string
	Username     string
	Password     string
}

type Login struct {
	Username string
	Password string
}

type AddMember struct {
	Username string
	Password string
	Role     Role
}

type UpdatePreferences struct {
	Locale Locale
	Theme  Theme
}

type AuthService struct {
	store      AuthStore
	now        func() time.Time
	sessionTTL time.Duration
}

func NewAuthService(store AuthStore) *AuthService {
	return &AuthService{store: store, now: time.Now, sessionTTL: 7 * 24 * time.Hour}
}

func (s *AuthService) NeedsSetup(ctx context.Context) (bool, error) {
	return s.store.AuthNeedsSetup(ctx)
}

func (s *AuthService) Setup(ctx context.Context, cmd SetupAuth) (SessionCredential, error) {
	needsSetup, err := s.store.AuthNeedsSetup(ctx)
	if err != nil {
		return SessionCredential{}, fmt.Errorf("check auth setup: %w", err)
	}
	if !needsSetup {
		return SessionCredential{}, ErrSetupComplete
	}
	tenantName := strings.TrimSpace(cmd.TenantName)
	if tenantName == "" {
		return SessionCredential{}, errors.New("tenant name is required")
	}
	baseCurrency, err := normalizeCurrency(cmd.BaseCurrency)
	if err != nil {
		return SessionCredential{}, err
	}
	user, err := newUser(cmd.Username, cmd.Password, s.now().UTC())
	if err != nil {
		return SessionCredential{}, err
	}
	tenant := Tenant{ID: newID(), Name: tenantName, BaseCurrency: baseCurrency, CreatedAt: user.CreatedAt}
	membership := Membership{TenantID: tenant.ID, UserID: user.ID, Role: RoleOwner, CreatedAt: user.CreatedAt}
	event := SecurityEvent{ID: newID(), TenantID: tenant.ID, ActorUserID: user.ID, Action: "auth.setup", TargetUserID: user.ID, OccurredAt: user.CreatedAt}
	if err := s.store.BootstrapAuth(ctx, tenant, user, membership, event); err != nil {
		return SessionCredential{}, fmt.Errorf("bootstrap authentication: %w", err)
	}
	return s.issueSession(ctx, Principal{TenantID: tenant.ID, TenantName: tenant.Name, UserID: user.ID, Username: user.Username, Role: RoleOwner, Locale: user.Locale, Theme: user.Theme})
}

func (s *AuthService) EnsureDisabledPrincipal(ctx context.Context) (Principal, error) {
	needsSetup, err := s.store.AuthNeedsSetup(ctx)
	if err != nil {
		return Principal{}, err
	}
	if needsSetup {
		now := s.now().UTC()
		tenant := Tenant{ID: "00000000-0000-4000-8000-000000000001", Name: "Local", BaseCurrency: "CNY", CreatedAt: now}
		user := User{ID: "00000000-0000-4000-8000-000000000002", Username: "local", UsernameNormalized: "local", PasswordHash: "disabled", Locale: LocaleZhCN, Theme: ThemeSystem, CreatedAt: now}
		membership := Membership{TenantID: tenant.ID, UserID: user.ID, Role: RoleOwner, CreatedAt: now}
		event := SecurityEvent{ID: newID(), TenantID: tenant.ID, ActorUserID: user.ID, Action: "auth.disabled_bootstrap", TargetUserID: user.ID, OccurredAt: now}
		if err := s.store.BootstrapAuth(ctx, tenant, user, membership, event); err != nil {
			return Principal{}, err
		}
	}
	return s.store.FirstPrincipal(ctx)
}

func (s *AuthService) LocalPrincipal(ctx context.Context) (Principal, error) {
	return s.store.FirstPrincipal(ctx)
}

func (s *AuthService) Login(ctx context.Context, cmd Login) (SessionCredential, error) {
	normalized, err := normalizeUsername(cmd.Username)
	if err != nil {
		return SessionCredential{}, ErrInvalidLogin
	}
	account, err := s.store.FindAccount(ctx, normalized)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			_, _ = hashPassword(cmd.Password)
			return SessionCredential{}, ErrInvalidLogin
		}
		return SessionCredential{}, fmt.Errorf("find account: %w", err)
	}
	if !verifyPassword(account.PasswordHash, cmd.Password) {
		_ = s.store.RecordSecurityEvent(ctx, SecurityEvent{ID: newID(), TenantID: account.TenantID, ActorUserID: account.UserID, Action: "auth.login_failed", TargetUserID: account.UserID, OccurredAt: s.now().UTC()})
		return SessionCredential{}, ErrInvalidLogin
	}
	credential, err := s.issueSession(ctx, account.Principal)
	if err != nil {
		return SessionCredential{}, err
	}
	_ = s.store.RecordSecurityEvent(ctx, SecurityEvent{ID: newID(), TenantID: account.TenantID, ActorUserID: account.UserID, Action: "auth.login_succeeded", TargetUserID: account.UserID, OccurredAt: s.now().UTC()})
	return credential, nil
}

func (s *AuthService) Authenticate(ctx context.Context, token string) (Principal, error) {
	if strings.TrimSpace(token) == "" {
		return Principal{}, ErrUnauthorized
	}
	principal, err := s.store.GetSessionPrincipal(ctx, tokenHash(token), s.now().UTC())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Principal{}, ErrUnauthorized
		}
		return Principal{}, fmt.Errorf("authenticate session: %w", err)
	}
	return principal, nil
}

func (s *AuthService) Logout(ctx context.Context, token string) error {
	if strings.TrimSpace(token) == "" {
		return nil
	}
	return s.store.DeleteSession(ctx, tokenHash(token))
}

func (s *AuthService) UpdatePreferences(ctx context.Context, actor Principal, cmd UpdatePreferences) (Principal, error) {
	if err := actor.Require(CapabilityView); err != nil {
		return Principal{}, err
	}
	if !validLocale(cmd.Locale) {
		return Principal{}, errors.New("unsupported locale")
	}
	if !validTheme(cmd.Theme) {
		return Principal{}, errors.New("unsupported theme")
	}
	if err := s.store.UpdateUserPreferences(ctx, actor.UserID, cmd.Locale, cmd.Theme); err != nil {
		return Principal{}, fmt.Errorf("update user preferences: %w", err)
	}
	actor.Locale, actor.Theme = cmd.Locale, cmd.Theme
	return actor, nil
}

func (s *AuthService) AddMember(ctx context.Context, actor Principal, cmd AddMember) (Member, error) {
	if err := actor.Require(CapabilityManageMembers); err != nil {
		return Member{}, err
	}
	if !validRole(cmd.Role) {
		return Member{}, errors.New("role must be owner, editor, or viewer")
	}
	now := s.now().UTC()
	user, err := newUser(cmd.Username, cmd.Password, now)
	if err != nil {
		return Member{}, err
	}
	membership := Membership{TenantID: actor.TenantID, UserID: user.ID, Role: cmd.Role, CreatedAt: now}
	event := SecurityEvent{ID: newID(), TenantID: actor.TenantID, ActorUserID: actor.UserID, Action: "membership.created", TargetUserID: user.ID, Detail: string(cmd.Role), OccurredAt: now}
	if err := s.store.CreateMember(ctx, user, membership, event); err != nil {
		return Member{}, fmt.Errorf("create member: %w", err)
	}
	return Member{UserID: user.ID, Username: user.Username, Role: cmd.Role, CreatedAt: now}, nil
}

func (s *AuthService) ListMembers(ctx context.Context, actor Principal) ([]Member, error) {
	if err := actor.Require(CapabilityManageMembers); err != nil {
		return nil, err
	}
	return s.store.ListMembers(ctx, actor.TenantID)
}

func (s *AuthService) issueSession(ctx context.Context, principal Principal) (SessionCredential, error) {
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return SessionCredential{}, fmt.Errorf("generate session token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(tokenBytes)
	now := s.now().UTC()
	expiresAt := now.Add(s.sessionTTL)
	if err := s.store.CreateSession(ctx, Session{TokenHash: tokenHash(token), TenantID: principal.TenantID, UserID: principal.UserID, ExpiresAt: expiresAt, CreatedAt: now}); err != nil {
		return SessionCredential{}, fmt.Errorf("create session: %w", err)
	}
	return SessionCredential{Token: token, Principal: principal, ExpiresAt: expiresAt}, nil
}

const passwordIterations = 600_000

func newUser(username, password string, now time.Time) (User, error) {
	normalized, err := normalizeUsername(username)
	if err != nil {
		return User{}, err
	}
	if utf8.RuneCountInString(password) < 12 {
		return User{}, errors.New("password must contain at least 12 characters")
	}
	hash, err := hashPassword(password)
	if err != nil {
		return User{}, err
	}
	return User{ID: newID(), Username: strings.TrimSpace(username), UsernameNormalized: normalized, PasswordHash: hash, Locale: LocaleZhCN, Theme: ThemeSystem, CreatedAt: now}, nil
}

func validLocale(locale Locale) bool {
	return locale == LocaleZhCN || locale == LocaleEn
}

func validTheme(theme Theme) bool {
	return theme == ThemeSystem || theme == ThemeLight || theme == ThemeDark
}

func normalizeUsername(value string) (string, error) {
	value = strings.TrimSpace(value)
	count := utf8.RuneCountInString(value)
	if count < 3 || count > 64 {
		return "", errors.New("username must contain 3 to 64 characters")
	}
	for _, r := range value {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return "", errors.New("username must not contain whitespace or control characters")
		}
	}
	return strings.ToLower(value), nil
}

func normalizeCurrency(value string) (string, error) {
	value = strings.ToUpper(strings.TrimSpace(value))
	if len(value) != 3 {
		return "", errors.New("base currency must be a three-letter ISO code")
	}
	for _, r := range value {
		if r < 'A' || r > 'Z' {
			return "", errors.New("base currency must be a three-letter ISO code")
		}
	}
	return value, nil
}

func validRole(role Role) bool {
	return role == RoleOwner || role == RoleEditor || role == RoleViewer
}

func hashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate password salt: %w", err)
	}
	key, err := pbkdf2.Key(sha256.New, password, salt, passwordIterations, 32)
	if err != nil {
		return "", fmt.Errorf("derive password hash: %w", err)
	}
	return fmt.Sprintf("pbkdf2_sha256$%d$%s$%s", passwordIterations, base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}

func verifyPassword(encoded, password string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2_sha256" {
		return false
	}
	iterations, err := strconv.Atoi(parts[1])
	if err != nil || iterations < 1 || iterations > 2_000_000 {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[2])
	if err != nil || len(salt) != 16 {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil || len(want) != 32 {
		return false
	}
	got, err := pbkdf2.Key(sha256.New, password, salt, iterations, len(want))
	return err == nil && subtle.ConstantTimeCompare(got, want) == 1
}

func tokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
