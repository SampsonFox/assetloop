package application

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"
)

type memoryAuthStore struct {
	users      map[string]User
	accounts   map[string]Account
	members    map[string][]Member
	sessions   map[string]Session
	principals map[string]Principal
	events     []SecurityEvent
}

func newMemoryAuthStore() *memoryAuthStore {
	return &memoryAuthStore{
		users: map[string]User{}, accounts: map[string]Account{}, members: map[string][]Member{},
		sessions: map[string]Session{}, principals: map[string]Principal{},
	}
}

func (s *memoryAuthStore) AuthNeedsSetup(context.Context) (bool, error) {
	return len(s.users) == 0, nil
}

func (s *memoryAuthStore) BootstrapAuth(_ context.Context, tenant Tenant, user User, membership Membership, event SecurityEvent) error {
	if len(s.users) != 0 {
		return ErrSetupComplete
	}
	s.users[user.ID] = user
	p := Principal{TenantID: tenant.ID, TenantName: tenant.Name, UserID: user.ID, Username: user.Username, Role: membership.Role, Locale: user.Locale, Theme: user.Theme, Accent: user.Accent}
	s.accounts[user.UsernameNormalized] = Account{Principal: p, PasswordHash: user.PasswordHash}
	s.principals[user.ID] = p
	s.members[tenant.ID] = []Member{{UserID: user.ID, Username: user.Username, Role: membership.Role, CreatedAt: membership.CreatedAt}}
	s.events = append(s.events, event)
	return nil
}

func (s *memoryAuthStore) FindAccount(_ context.Context, username string) (Account, error) {
	account, ok := s.accounts[username]
	if !ok {
		return Account{}, sql.ErrNoRows
	}
	return account, nil
}

func (s *memoryAuthStore) FirstPrincipal(context.Context) (Principal, error) {
	for _, principal := range s.principals {
		return principal, nil
	}
	return Principal{}, sql.ErrNoRows
}

func (s *memoryAuthStore) CreateSession(_ context.Context, session Session) error {
	s.sessions[session.TokenHash] = session
	return nil
}

func (s *memoryAuthStore) GetSessionPrincipal(_ context.Context, tokenHash string, now time.Time) (Principal, error) {
	session, ok := s.sessions[tokenHash]
	if !ok || !session.ExpiresAt.After(now) {
		return Principal{}, sql.ErrNoRows
	}
	return s.principals[session.UserID], nil
}

func (s *memoryAuthStore) DeleteSession(_ context.Context, tokenHash string) error {
	delete(s.sessions, tokenHash)
	return nil
}

func (s *memoryAuthStore) UpdateUserPreferences(_ context.Context, userID string, locale Locale, theme Theme, accent Accent) error {
	principal, ok := s.principals[userID]
	if !ok {
		return sql.ErrNoRows
	}
	principal.Locale, principal.Theme, principal.Accent = locale, theme, accent
	s.principals[userID] = principal
	for username, account := range s.accounts {
		if account.UserID == userID {
			account.Principal = principal
			s.accounts[username] = account
		}
	}
	return nil
}

func (s *memoryAuthStore) CreateMember(_ context.Context, user User, membership Membership, event SecurityEvent) error {
	if _, exists := s.accounts[user.UsernameNormalized]; exists {
		return errors.New("duplicate username")
	}
	s.users[user.ID] = user
	owner := s.members[membership.TenantID][0]
	principal := s.principals[owner.UserID]
	p := Principal{TenantID: membership.TenantID, TenantName: principal.TenantName, UserID: user.ID, Username: user.Username, Role: membership.Role, Locale: user.Locale, Theme: user.Theme, Accent: user.Accent}
	s.accounts[user.UsernameNormalized] = Account{Principal: p, PasswordHash: user.PasswordHash}
	s.principals[user.ID] = p
	s.members[membership.TenantID] = append(s.members[membership.TenantID], Member{UserID: user.ID, Username: user.Username, Role: membership.Role, CreatedAt: membership.CreatedAt})
	s.events = append(s.events, event)
	return nil
}

func (s *memoryAuthStore) ListMembers(_ context.Context, tenantID string, opts MemberListOptions) (MemberListResult, error) {
	members := append([]Member(nil), s.members[tenantID]...)
	return MemberListResult{Members: members, Total: len(members)}, nil
}

func (s *memoryAuthStore) RecordSecurityEvent(_ context.Context, event SecurityEvent) error {
	s.events = append(s.events, event)
	return nil
}

func TestRoleCapabilities(t *testing.T) {
	tests := []struct {
		role       Role
		capability Capability
		allowed    bool
	}{
		{RoleOwner, CapabilityManageMembers, true},
		{RoleEditor, CapabilityManageCatalog, false},
		{RoleEditor, CapabilityManageMembers, false},
		{RoleViewer, CapabilityView, true},
		{RoleViewer, CapabilityManageLifecycle, false},
	}
	for _, test := range tests {
		principal := Principal{TenantID: "tenant", UserID: "user", Role: test.role}
		if got := principal.Can(test.capability); got != test.allowed {
			t.Fatalf("role %q capability %q: got %v, want %v", test.role, test.capability, got, test.allowed)
		}
	}
}

func TestAuthServiceSetupLoginAndMemberPermissions(t *testing.T) {
	ctx := context.Background()
	store := newMemoryAuthStore()
	service := NewAuthService(store)
	now := time.Date(2026, 9, 1, 3, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }

	credential, err := service.Setup(ctx, SetupAuth{TenantName: "Home", BaseCurrency: "cny", Username: "Owner", Password: "correct horse battery"})
	if err != nil {
		t.Fatal(err)
	}
	if credential.Principal.Role != RoleOwner || credential.Principal.TenantName != "Home" || credential.Token == "" {
		t.Fatalf("unexpected setup credential: %+v", credential)
	}
	if credential.Principal.Locale != LocaleZhCN || credential.Principal.Theme != ThemeSystem || credential.Principal.Accent != AccentEmerald {
		t.Fatalf("unexpected default preferences: %+v", credential.Principal)
	}
	if _, err := service.Setup(ctx, SetupAuth{TenantName: "Other", BaseCurrency: "CNY", Username: "other", Password: "another long password"}); !errors.Is(err, ErrSetupComplete) {
		t.Fatalf("second setup should fail with ErrSetupComplete, got %v", err)
	}

	principal, err := service.Authenticate(ctx, credential.Token)
	if err != nil || principal.UserID != credential.Principal.UserID {
		t.Fatalf("authenticate setup session: principal=%+v err=%v", principal, err)
	}
	member, err := service.AddMember(ctx, principal, AddMember{Username: "editor", Password: "editor secure password", Role: RoleEditor})
	if err != nil || member.Role != RoleEditor {
		t.Fatalf("add editor: member=%+v err=%v", member, err)
	}

	editorCredential, err := service.Login(ctx, Login{Username: "EDITOR", Password: "editor secure password"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AddMember(ctx, editorCredential.Principal, AddMember{Username: "viewer", Password: "viewer secure password", Role: RoleViewer}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("editor member creation should be forbidden, got %v", err)
	}
	if _, err := service.Login(ctx, Login{Username: "editor", Password: "wrong password"}); !errors.Is(err, ErrInvalidLogin) {
		t.Fatalf("wrong password should return ErrInvalidLogin, got %v", err)
	}
	if len(store.events) != 4 {
		t.Fatalf("expected setup, membership, login success and login failure audit events, got %d", len(store.events))
	}
}

func TestAuthServiceUpdatesPreferences(t *testing.T) {
	ctx := context.Background()
	store := newMemoryAuthStore()
	service := NewAuthService(store)
	credential, err := service.Setup(ctx, SetupAuth{TenantName: "Home", BaseCurrency: "CNY", Username: "owner", Password: "correct horse battery"})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := service.UpdatePreferences(ctx, credential.Principal, UpdatePreferences{Locale: LocaleEn, Theme: ThemeDark, Accent: AccentViolet})
	if err != nil || updated.Locale != LocaleEn || updated.Theme != ThemeDark || updated.Accent != AccentViolet {
		t.Fatalf("update preferences: principal=%+v err=%v", updated, err)
	}
	authenticated, err := service.Authenticate(ctx, credential.Token)
	if err != nil || authenticated.Locale != LocaleEn || authenticated.Theme != ThemeDark || authenticated.Accent != AccentViolet {
		t.Fatalf("preferences were not visible through session: principal=%+v err=%v", authenticated, err)
	}
	if _, err := service.UpdatePreferences(ctx, updated, UpdatePreferences{Locale: "fr", Theme: ThemeDark, Accent: AccentViolet}); err == nil {
		t.Fatal("unsupported locale should fail")
	}
	if _, err := service.UpdatePreferences(ctx, updated, UpdatePreferences{Locale: LocaleEn, Theme: "sepia", Accent: AccentViolet}); err == nil {
		t.Fatal("unsupported theme should fail")
	}
	if _, err := service.UpdatePreferences(ctx, updated, UpdatePreferences{Locale: LocaleEn, Theme: ThemeDark, Accent: "neon"}); err == nil {
		t.Fatal("unsupported accent should fail")
	}
}

func TestPasswordHashRejectsMalformedAndWrongPassword(t *testing.T) {
	hash, err := hashPassword("a long valid password")
	if err != nil {
		t.Fatal(err)
	}
	if !verifyPassword(hash, "a long valid password") || verifyPassword(hash, "wrong") || verifyPassword("bad", "anything") {
		t.Fatal("password verification result was incorrect")
	}
}

// TestNewUserPasswordMinimumLength pins the shared validator to an eight-rune
// minimum so both setup and member creation reject shorter passwords while the
// Unicode case proves the limit counts runes rather than bytes.
func TestNewUserPasswordMinimumLength(t *testing.T) {
	now := time.Date(2026, 9, 1, 3, 0, 0, 0, time.UTC)
	tests := []struct {
		name     string
		password string
		accepted bool
	}{
		{name: "seven ASCII characters", password: "abcdefg", accepted: false},
		{name: "eight ASCII characters", password: "abcdefgh", accepted: true},
		{name: "seven Unicode runes", password: strings.Repeat("密", 7), accepted: false},
		{name: "eight Unicode runes", password: strings.Repeat("密", 8), accepted: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			user, err := newUser("member", test.password, now)
			if test.accepted {
				if err != nil {
					t.Fatalf("password %q should be accepted: %v", test.password, err)
				}
				if user.PasswordHash == "" {
					t.Fatalf("accepted password %q was not hashed", test.password)
				}
				return
			}
			var input InputError
			if !errors.As(err, &input) || input.Code != "validation.password_length" {
				t.Fatalf("password %q should fail with validation.password_length: %v", test.password, err)
			}
			if len(input.Args) != 1 || input.Args[0] != 8 {
				t.Fatalf("validation.password_length must report the 8-rune minimum, got %v", input.Args)
			}
		})
	}
}

func (s *memoryAuthStore) ChangeMemberRole(_ context.Context, actor Principal, id string, role Role, event SecurityEvent) error {
	p, ok := s.principals[id]
	if !ok || p.TenantID != actor.TenantID {
		return sql.ErrNoRows
	}
	p.Role = role
	s.principals[id] = p
	s.events = append(s.events, event)
	return nil
}
