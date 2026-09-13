package mcp

import (
	"context"
	"github.com/SampsonFox/assetloop/internal/application"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"net/http"
	"net/http/httptest"
	"testing"
)

func testWorkerTools(t *testing.T, s Services, admin application.Principal, categoryID string) {
	ctx := context.Background()
	model, err := s.Catalog.CreateModel(ctx, admin, application.CreateModel{CategoryID: categoryID, Name: "Worker model"})
	if err != nil {
		t.Fatal(err)
	}
	actor := admin
	actor.Role = application.RoleEditor
	identity := Identity{Principal: actor, Scopes: []string{ScopeRead, ScopeCatalog, ScopeLifecycle}}
	host := httptest.NewServer(NewHandler(s, func(context.Context, *http.Request) (Identity, error) { return identity, nil }))
	defer host.Close()
	client, err := sdk.NewClient(&sdk.Implementation{Name: "permissions-test", Version: "1"}, nil).Connect(ctx, &sdk.StreamableClientTransport{Endpoint: host.URL}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	call := func(name string, args any, denied bool) {
		t.Helper()
		result, err := client.CallTool(ctx, &sdk.CallToolParams{Name: name, Arguments: args})
		if err != nil {
			t.Fatal(err)
		}
		if result.IsError != denied {
			t.Fatalf("%s denied=%v: %v", name, result.IsError, result.Content)
		}
	}
	args := map[string]any{"request_key": "worker-save", "model_id": model.ID, "display_name": "Worker item", "serial_number": "", "purchase_channel": "", "notes": "", "tag_ids": []string{}}
	call("save_asset", args, false)
	for _, test := range []struct {
		name string
		args map[string]any
	}{
		{"create_category", map[string]any{"request_key": "worker-category", "name": "Denied", "icon_key": "camera"}},
		{"create_event_type", map[string]any{"request_key": "worker-type", "name": "Denied", "cashflow": "expense"}},
		{"save_model_configuration", map[string]any{"request_key": "worker-model", "model_id": model.ID, "tag_ids": []string{}, "appearance_overrides": map[string]bool{}}},
	} {
		call(test.name, test.args, true)
	}
	identity.Principal.Role = application.RoleViewer
	call("save_asset", args, true) // Authorization also precedes durable receipt replay.
}
