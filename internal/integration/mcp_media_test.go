package integration_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/SampsonFox/assetloop/internal/blob"
	localblob "github.com/SampsonFox/assetloop/internal/blob/local"
	transport "github.com/SampsonFox/assetloop/internal/mcp"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func testManagementBinding(t *testing.T, store application.ManagementStore, owner application.Principal, category string) {
	t.Helper()
	ctx := context.Background()
	local, err := localblob.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	media := application.NewModelMediaService(store, blob.Registry{"local": local}, blob.ObjectKeyMapper{}, "local")
	manager := application.NewManagementService(store)
	model, err := manager.CreateModel(ctx, owner, "binding-model", application.CreateModel{CategoryID: category, Name: "Binding model"})
	if err != nil {
		t.Fatal(err)
	}
	asset, err := manager.SaveAsset(ctx, owner, "binding-asset", application.SaveSpecificationAsset{ModelID: model.ID, DisplayName: "Binding item"})
	if err != nil {
		t.Fatal(err)
	}
	r, err := media.Upload(ctx, owner, application.UploadModel3DResource{Name: "Binding GLB", File: fullElementGLB()})
	if err != nil {
		t.Fatal(err)
	}
	cmd := application.BindModel3DResource{Kind: "model", TargetID: model.ID, ResourceID: r.ID}
	if err := application.NewManagementService(failedReceiptStore{store}).BindResource(ctx, owner, "binding-rollback", cmd); err == nil {
		t.Fatal("receipt failure ignored")
	}
	b, err := media.Binding(ctx, owner, "model", model.ID)
	if err != nil || b.ResourceID != "" {
		t.Fatalf("binding escaped rollback: %+v / %v", b, err)
	}
	if err := manager.BindResource(ctx, owner, "binding-key", cmd); err != nil {
		t.Fatal(err)
	}
	if err := media.DeleteResource(ctx, owner, r.ID); !errors.Is(err, application.ErrModel3DReferenced) {
		t.Fatalf("referenced deletion accepted: %v", err)
	}
	opened, err := media.OpenResource(ctx, owner, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := opened.Reader.Close(); err != nil {
		t.Fatal(err)
	}
	// A replay acknowledges the original command; it must not overwrite a later edit.
	clear := cmd
	clear.ResourceID = ""
	if err := manager.BindResource(ctx, owner, "binding-clear", clear); err != nil {
		t.Fatal(err)
	}
	if err := manager.BindResource(ctx, owner, "binding-key", cmd); err != nil {
		t.Fatal(err)
	}
	b, err = media.Binding(ctx, owner, "model", model.ID)
	if err != nil || b.ResourceID != "" {
		t.Fatal("replay overwrote newer binding")
	}
	if err := manager.BindResource(ctx, owner, "binding-key", clear); err == nil {
		t.Fatal("conflicting binding replay accepted")
	}
	if err := manager.BindResource(ctx, owner, "", cmd); err == nil {
		t.Fatal("empty key accepted")
	}
	if err := manager.BindResource(ctx, owner, "binding-restore", cmd); err != nil {
		t.Fatal(err)
	}

	identity := transport.Identity{Principal: owner, Scopes: []string{transport.ScopeRead, transport.ScopeCatalog}}
	host := httptest.NewServer(transport.NewHandler(transport.Services{Media: media, Management: manager}, func(context.Context, *http.Request) (transport.Identity, error) { return identity, nil }))
	defer host.Close()
	client, err := sdk.NewClient(&sdk.Implementation{Name: "media-test", Version: "1"}, nil).Connect(ctx, &sdk.StreamableClientTransport{Endpoint: host.URL}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	call := func(name string, args any, wantError bool) []byte {
		t.Helper()
		result, err := client.CallTool(ctx, &sdk.CallToolParams{Name: name, Arguments: args})
		if err != nil {
			t.Fatal(err)
		}
		if result.IsError != wantError {
			t.Fatalf("%s: error=%v want=%v: %v", name, result.IsError, wantError, result.Content)
		}
		data, err := json.Marshal(result.StructuredContent)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	query := transport.BindingInput{Kind: "asset", TargetID: asset.ID}
	read := func(explicit string) {
		t.Helper()
		data := call("get_3d_binding", query, false)
		var result struct{ Data transport.BindingResult }
		if err := json.Unmarshal(data, &result); err != nil {
			t.Fatal(err)
		}
		if result.Data.ResourceID != explicit || result.Data.EffectiveResourceID != r.ID {
			t.Fatalf("wrong inheritance: %s", data)
		}
		if strings.Contains(string(data), r.ObjectKey) || strings.Contains(string(data), "store_id") {
			t.Fatal("binding exposes storage location")
		}
	}
	read("")
	input := transport.BindResourceInput{BindingInput: query, RequestKey: "http-binding", ResourceID: r.ID}
	call("bind_3d_resource", input, false)
	call("bind_3d_resource", input, false)
	read(r.ID)
	input.ResourceID, input.RequestKey = "", "http-clear"
	call("bind_3d_resource", input, false)
	read("")
	identity.Scopes = []string{transport.ScopeRead}
	call("bind_3d_resource", input, true)
	identity.Scopes = []string{transport.ScopeRead, transport.ScopeCatalog}
	identity.Principal.Role = application.RoleViewer
	call("bind_3d_resource", input, true)
	identity.Principal = owner
	identity.Principal.TenantID = "00000000-0000-4000-8000-000000000001"
	call("get_3d_binding", query, true)
	input.RequestKey = "other-tenant"
	call("bind_3d_resource", input, true)
}
