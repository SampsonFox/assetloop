package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/SampsonFox/assetloop/internal/config"
	basestore "github.com/SampsonFox/assetloop/internal/store"
	"github.com/SampsonFox/assetloop/internal/store/sqlite"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestHTTPQueries(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cfg := config.Database{Driver: "sqlite", DSN: filepath.Join(t.TempDir(), "mcp.db")}
	db, err := basestore.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := basestore.Migrate(ctx, db, cfg); err != nil {
		t.Fatal(err)
	}
	store := sqlite.New(db)
	credential, err := application.NewAuthService(store).Setup(ctx, application.SetupAuth{
		TenantName: "MCP test", BaseCurrency: "CNY", Username: "owner", Password: "test-only owner password",
	})
	if err != nil {
		t.Fatal(err)
	}
	owner := credential.Principal
	services := Services{Catalog: application.NewCatalogService(store), Specifications: application.NewSpecificationService(store), Lifecycle: application.NewLifecycleService(store), Management: application.NewManagementService(store)}
	category, err := services.Catalog.CreateCategory(ctx, owner, application.CreateCategory{Name: "Private camera", IconKey: "camera"})
	if err != nil {
		t.Fatal(err)
	}
	t.Run("lifecycle writes", func(t *testing.T) { testLifecycleTools(t, services, owner, category.ID) })
	for _, tc := range []struct {
		name               string
		actor              application.Principal
		scopes             []string
		forbidden, private bool
	}{
		{"owner", owner, []string{ScopeRead}, false, true},
		{"missing scope", owner, []string{ScopeCatalog}, true, false},
		{"other tenant", application.Principal{UserID: "other-user", TenantID: "other-tenant", Role: application.RoleViewer}, []string{ScopeRead}, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			host := httptest.NewServer(NewHandler(services, func(context.Context, *http.Request) (Identity, error) {
				return Identity{Principal: tc.actor, Scopes: tc.scopes}, nil
			}))
			defer host.Close()
			session, err := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "1"}, nil).Connect(ctx, &sdk.StreamableClientTransport{Endpoint: host.URL}, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer session.Close()
			list, err := session.ListTools(ctx, &sdk.ListToolsParams{})
			if err != nil {
				t.Fatal(err)
			}
			if len(list.Tools) != 29 {
				t.Fatalf("tools = %d, want 29", len(list.Tools))
			}
			for _, tool := range list.Tools {
				write := strings.HasPrefix(tool.Name, "save_") || strings.HasPrefix(tool.Name, "create_") || strings.HasPrefix(tool.Name, "update_") || strings.HasPrefix(tool.Name, "delete_") || tool.Name == "record_event" || tool.Name == "correct_event"
				if tool.Annotations == nil || tool.Annotations.ReadOnlyHint == write {
					t.Errorf("%s is not annotated read-only", tool.Name)
				}
				schema, _ := json.Marshal(tool.InputSchema)
				if strings.Contains(string(schema), "tenant_id") || strings.Contains(string(schema), "user_id") {
					t.Errorf("%s accepts identity", tool.Name)
				}
			}
			result, err := session.CallTool(ctx, &sdk.CallToolParams{Name: "list_categories", Arguments: map[string]any{}})
			if err != nil {
				t.Fatal(err)
			}
			if result.IsError != tc.forbidden {
				t.Fatalf("IsError = %v, want %v", result.IsError, tc.forbidden)
			}
			encoded, err := json.Marshal(result.StructuredContent)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(encoded), category.ID) != tc.private {
				t.Fatalf("unexpected tenant result: %s", encoded)
			}
			if tc.forbidden && !strings.Contains(string(encoded), `"code":"forbidden"`) {
				t.Fatalf("missing stable error: %s", encoded)
			}
			if tc.private {
				result, err = session.CallTool(ctx, &sdk.CallToolParams{Name: "get_context", Arguments: map[string]any{}})
				if err != nil || result.IsError {
					t.Fatalf("context: %v, %v", result, err)
				}
				encoded, _ = json.Marshal(result.StructuredContent)
				if !strings.Contains(string(encoded), owner.UserID) || !strings.Contains(string(encoded), `"base_currency":"CNY"`) {
					t.Fatalf("context: %s", encoded)
				}
				result, err = session.CallTool(ctx, &sdk.CallToolParams{Name: "get_asset", Arguments: map[string]any{}})
				if err == nil && !result.IsError {
					t.Fatal("missing required ID accepted")
				}
			}
		})
	}
}

func TestHTTPAuthorizationFailsClosed(t *testing.T) {
	for _, verifier := range []Authenticate{nil, func(context.Context, *http.Request) (Identity, error) { return Identity{}, nil }} {
		for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodDelete} {
			w := httptest.NewRecorder()
			NewHandler(Services{}, verifier).ServeHTTP(w, httptest.NewRequest(method, "/mcp", nil))
			if w.Code != http.StatusUnauthorized {
				t.Fatalf("%s: %d", method, w.Code)
			}
			if w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("authorization response can be cached")
			}
		}
	}
}
