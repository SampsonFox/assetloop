package integration_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/SampsonFox/assetloop/internal/config"
	basestore "github.com/SampsonFox/assetloop/internal/store"
	"github.com/SampsonFox/assetloop/internal/store/postgres"
	"github.com/SampsonFox/assetloop/internal/store/sqlite"
)

type oauthStore interface {
	application.OAuthStore
	application.AuthStore
}

func TestOAuthPersistence(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			cfg := config.Database{Driver: "sqlite", DSN: filepath.Join(t.TempDir(), "oauth.db")}
			if driver == "postgres" {
				dsn := os.Getenv("TEST_POSTGRES_DSN")
				if dsn == "" {
					if os.Getenv("REQUIRE_POSTGRES_TEST") == "true" {
						t.Fatal("PostgreSQL required")
					}
					t.Skip("TEST_POSTGRES_DSN is not set")
				}
				var cleanup func()
				cfg, cleanup = isolatedPostgres(t, dsn)
				defer cleanup()
			}
			db := openAndMigrate(t, cfg)
			second, err := basestore.Open(cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer second.Close()
			var firstStore, secondStore oauthStore = sqlite.New(db), sqlite.New(second)
			if driver == "postgres" {
				firstStore, secondStore = postgres.New(db), postgres.New(second)
			}
			ctx := context.Background()
			account, err := application.NewAuthService(firstStore).Setup(ctx, application.SetupAuth{TenantName: "OAuth persistence", BaseCurrency: "CNY", Username: "owner", Password: "test only owner password"})
			if err != nil {
				t.Fatal(err)
			}
			clients := []application.OAuthClient{{ID: "test-client", Name: "Test client", RedirectURIs: []string{"http://127.0.0.1/callback"}}}
			service, err := application.NewOAuthService(firstStore, "http://127.0.0.1:8081/mcp", clients)
			if err != nil {
				t.Fatal(err)
			}
			other, err := application.NewOAuthService(secondStore, "http://127.0.0.1:8081/mcp", clients)
			if err != nil {
				t.Fatal(err)
			}
			verifier := strings.Repeat("a", 43)
			sum := sha256.Sum256([]byte(verifier))
			cmd := application.OAuthAuthorization{ClientID: "test-client", RedirectURI: "http://127.0.0.1:49152/callback", Resource: "http://127.0.0.1:8081/mcp", Scope: application.OAuthRead, ChallengeMethod: "S256", Challenge: base64.RawURLEncoding.EncodeToString(sum[:])}
			code, err := service.Authorize(ctx, account.Principal, cmd)
			if err != nil {
				t.Fatal(err)
			}
			request := application.OAuthTokenRequest{ClientID: cmd.ClientID, Resource: cmd.Resource, Code: code, RedirectURI: cmd.RedirectURI, Verifier: verifier}
			tokens, err := other.Exchange(ctx, request)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := service.Authenticate(ctx, tokens.AccessToken); err != nil {
				t.Fatal(err)
			}
			request.RefreshToken = tokens.RefreshToken
			rotated, err := other.Refresh(ctx, request)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := service.Refresh(ctx, request); !errors.Is(err, application.ErrOAuthGrant) {
				t.Fatalf("replay: %v", err)
			}
			if _, err := other.Authenticate(ctx, rotated.AccessToken); !errors.Is(err, application.ErrUnauthorized) {
				t.Fatalf("revocation not persisted: %v", err)
			}
			grants, err := service.Grants(ctx, account.Principal)
			if err != nil || len(grants) != 1 || !grants[0].Revoked {
				t.Fatalf("grant state: %v / %v", grants, err)
			}
			code, err = service.Authorize(ctx, account.Principal, cmd)
			if err != nil {
				t.Fatal(err)
			}
			request.Code = code
			results := make(chan error, 2)
			for _, s := range []*application.OAuthService{service, other} {
				go func() { _, err := s.Exchange(ctx, request); results <- err }()
			}
			a, b := <-results, <-results
			if (a == nil) == (b == nil) {
				t.Fatalf("expected one successful exchange: %v / %v", a, b)
			}
			if a != nil && !errors.Is(a, application.ErrOAuthGrant) || b != nil && !errors.Is(b, application.ErrOAuthGrant) {
				t.Fatalf("unexpected exchange error: %v / %v", a, b)
			}
		})
	}
}
