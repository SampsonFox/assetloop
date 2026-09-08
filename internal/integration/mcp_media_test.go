package integration_test

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
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
	manager := application.NewManagementService(store, blob.Registry{"local": local})
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
	if err := application.NewManagementService(failedReceiptStore{store}, nil).BindResource(ctx, owner, "binding-rollback", cmd); err == nil {
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
	host := httptest.NewServer(transport.NewHandler(transport.Services{Media: media, Management: manager, Catalog: application.NewCatalogService(store), Specifications: application.NewSpecificationService(store)}, func(context.Context, *http.Request) (transport.Identity, error) { return identity, nil }))
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
	for _, read := range []struct {
		name string
		args any
	}{
		{"get_product_model", transport.IDInput{ID: model.ID}},
		{"search_product_models", transport.ModelQuery{Query: model.Name}},
		{"get_3d_resource", transport.IDInput{ID: r.ID}},
		{"list_3d_resources", transport.ResourceQuery{Query: r.Name}},
		{"get_asset_appearance", transport.IDInput{ID: asset.ID}},
	} {
		data := call(read.name, read.args, false)
		for _, private := range []string{r.ObjectKey, "ObjectKey", "object_key", "StoreID", "store_id", "SHA256"} {
			if strings.Contains(string(data), private) {
				t.Fatalf("%s exposed private storage metadata", read.name)
			}
		}
		if !strings.Contains(string(data), r.ID) {
			t.Fatalf("%s lost resource identity: %s", read.name, data)
		}
	}
	var resourceMetadata struct{ Data transport.ResourceResult }
	if err := json.Unmarshal(call("get_3d_resource", transport.IDInput{ID: r.ID}, false), &resourceMetadata); err != nil {
		t.Fatal(err)
	}
	if resourceMetadata.Data.Name != r.Name || resourceMetadata.Data.SizeBytes != r.SizeBytes || resourceMetadata.Data.Status != "ready" {
		t.Fatal("public resource metadata lost")
	}
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
	identity.Principal = owner
	deletable, err := media.Upload(ctx, owner, application.UploadModel3DResource{Name: "HTTP delete", File: fullElementGLB()})
	if err != nil {
		t.Fatal(err)
	}
	deletion := transport.DeleteResourceInput{RequestKey: "http-delete", ResourceID: deletable.ID}
	identity.Scopes = []string{transport.ScopeRead}
	call("delete_3d_resource", deletion, true)
	identity.Scopes = []string{transport.ScopeCatalog}
	call("delete_3d_resource", deletion, false)
	call("delete_3d_resource", deletion, false)
	if _, err := media.GetResource(ctx, owner, deletable.ID); !errors.Is(err, application.ErrModel3DNotFound) {
		t.Fatal("HTTP deletion did not remove resource")
	}
	testManagementDeletion(t, store, owner, media, local, model.ID)
	testConfigurationQueries(t, store, owner, manager, model.ID, r.ID, category)
}

type retryDeleteBlob struct {
	application.BlobStore
	fail  bool
	calls int
}

func (b *retryDeleteBlob) Delete(ctx context.Context, key string) error {
	b.calls++
	if b.fail {
		b.fail = false
		return errors.New("injected blob deletion failure")
	}
	return b.BlobStore.Delete(ctx, key)
}

type retryFinishStore struct {
	application.ManagementStore
	fail bool
}

func (s *retryFinishStore) FinishModel3DResourceDelete(ctx context.Context, tenant, id string) error {
	if s.fail {
		s.fail = false
		return errors.New("injected metadata deletion failure")
	}
	return s.ManagementStore.FinishModel3DResourceDelete(ctx, tenant, id)
}

func testManagementDeletion(t *testing.T, store application.ManagementStore, owner application.Principal, media *application.ModelMediaService, local application.BlobStore, modelID string) {
	t.Helper()
	ctx := context.Background()
	r, err := media.Upload(ctx, owner, application.UploadModel3DResource{Name: "Retry delete", File: fullElementGLB()})
	if err != nil {
		t.Fatal(err)
	}
	blobs := &retryDeleteBlob{BlobStore: local, fail: true}
	registry := blob.Registry{"local": blobs}
	failedReceipt := application.NewManagementService(failedReceiptStore{store}, registry)
	if err := failedReceipt.DeleteResource(ctx, owner, "delete-rollback", r.ID); err == nil {
		t.Fatal("receipt failure ignored")
	}
	current, err := media.GetResource(ctx, owner, r.ID)
	if err != nil || current.Status != "ready" || blobs.calls != 0 {
		t.Fatal("failed receipt changed resource or blob")
	}
	manager := application.NewManagementService(store, registry)
	if err := manager.DeleteResource(ctx, owner, "delete-retry", r.ID); err == nil {
		t.Fatal("blob failure reported success")
	}
	current, err = media.GetResource(ctx, owner, r.ID)
	if err != nil || current.Status != "pending-delete" {
		t.Fatalf("missing recoverable state: %+v / %v", current, err)
	}
	if err := manager.BindResource(ctx, owner, "pending-bind", application.BindModel3DResource{Kind: "model", TargetID: modelID, ResourceID: r.ID}); err == nil {
		t.Fatal("pending resource was rebound")
	}
	for i := 0; i < 2; i++ {
		if err := manager.DeleteResource(ctx, owner, "delete-retry", r.ID); err != nil {
			t.Fatalf("deletion replay: %v", err)
		}
	}
	if _, err := local.Stat(ctx, r.ObjectKey); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("deleted blob remains: %v", err)
	}
	if _, err := media.GetResource(ctx, owner, r.ID); !errors.Is(err, application.ErrModel3DNotFound) {
		t.Fatal("deleted metadata remains")
	}
	if err := manager.DeleteResource(ctx, owner, "delete-retry", modelID); err == nil {
		t.Fatal("conflicting delete key accepted")
	}
	viewer := owner
	viewer.Role = application.RoleViewer
	if err := manager.DeleteResource(ctx, viewer, "delete-retry", r.ID); !errors.Is(err, application.ErrForbidden) {
		t.Fatal("replay bypassed current role")
	}
	if err := manager.DeleteResource(ctx, owner, "unknown-delete", r.ID); err == nil {
		t.Fatal("missing resource without matching receipt reported success")
	}

	r, err = media.Upload(ctx, owner, application.UploadModel3DResource{Name: "Metadata retry", File: fullElementGLB()})
	if err != nil {
		t.Fatal(err)
	}
	finish := &retryFinishStore{ManagementStore: store, fail: true}
	manager = application.NewManagementService(finish, registry)
	if err := manager.DeleteResource(ctx, owner, "finish-retry", r.ID); err == nil {
		t.Fatal("metadata failure reported success")
	}
	if _, err := local.Stat(ctx, r.ObjectKey); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("failure injection did not occur after blob deletion")
	}
	current, err = media.GetResource(ctx, owner, r.ID)
	if err != nil || current.Status != "pending-delete" {
		t.Fatal("metadata failure lost retry state")
	}
	if err := manager.DeleteResource(ctx, owner, "finish-retry", r.ID); err != nil {
		t.Fatalf("metadata recovery: %v", err)
	}
	if err := manager.DeleteResource(ctx, owner, "finish-retry", r.ID); err != nil {
		t.Fatalf("completed recovery replay: %v", err)
	}
}
