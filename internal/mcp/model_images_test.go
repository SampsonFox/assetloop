package mcp

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/SampsonFox/assetloop/internal/blob"
	localblob "github.com/SampsonFox/assetloop/internal/blob/local"
	"github.com/SampsonFox/assetloop/internal/config"
	basestore "github.com/SampsonFox/assetloop/internal/store"
	"github.com/SampsonFox/assetloop/internal/store/sqlite"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type imageDownloadFixture struct {
	calls atomic.Int32
	err   error
}

func (f *imageDownloadFixture) DownloadImage(context.Context, string) ([]byte, error) {
	f.calls.Add(1)
	if f.err != nil {
		return nil, f.err
	}
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// TestModelImageTools covers the read-only contract, the receipt-backed import,
// replay without re-downloading, authorization and the public result shape.
func TestModelImageTools(t *testing.T) {
	ctx := context.Background()
	cfg := config.Database{Driver: "sqlite", DSN: filepath.Join(t.TempDir(), "mcp-image.db")}
	db, err := basestore.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := basestore.Migrate(ctx, db, cfg); err != nil {
		t.Fatal(err)
	}
	store := sqlite.New(db)
	credential, err := application.NewAuthService(store).Setup(ctx, application.SetupAuth{TenantName: "MCP images", BaseCurrency: "CNY", Username: "owner", Password: "test-only owner password"})
	if err != nil {
		t.Fatal(err)
	}
	owner := credential.Principal
	catalog := application.NewCatalogService(store)
	category, err := catalog.CreateCategory(ctx, owner, application.CreateCategory{Name: "Phone", IconKey: "smartphone"})
	if err != nil {
		t.Fatal(err)
	}
	model, err := catalog.CreateModel(ctx, owner, application.CreateModel{CategoryID: category.ID, Name: "Imaged model"})
	if err != nil {
		t.Fatal(err)
	}
	local, err := localblob.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	registry := blob.Registry{"local": local}
	images := application.NewModelImageService(store, registry, blob.ObjectKeyMapper{}, "local")
	management := application.NewManagementService(store, registry)
	fixture := &imageDownloadFixture{}
	services := Services{
		Catalog: catalog, Specifications: application.NewSpecificationService(store), Lifecycle: application.NewLifecycleService(store),
		Management: management, Images: images, ImageImport: application.NewModelImageImportService(management, images, fixture),
	}
	identity := Identity{Principal: owner, Scopes: []string{ScopeRead, ScopeCatalog}}
	host := httptest.NewServer(NewHandler(services, func(context.Context, *http.Request) (Identity, error) { return identity, nil }))
	defer host.Close()
	client, err := sdk.NewClient(&sdk.Implementation{Name: "image-test", Version: "1"}, nil).Connect(ctx, &sdk.StreamableClientTransport{Endpoint: host.URL}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	call := func(name string, args any) (Result, string) {
		t.Helper()
		result, err := client.CallTool(ctx, &sdk.CallToolParams{Name: name, Arguments: args})
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := json.Marshal(result.StructuredContent)
		if err != nil {
			t.Fatal(err)
		}
		var decoded Result
		if err := json.Unmarshal(encoded, &decoded); err != nil {
			t.Fatal(err)
		}
		if result.IsError != (decoded.Error != nil) {
			t.Fatalf("%s envelope mismatch: %s", name, encoded)
		}
		return decoded, string(encoded)
	}
	decode := func(result Result, target any) {
		t.Helper()
		encoded, err := json.Marshal(result.Data)
		if err != nil || json.Unmarshal(encoded, target) != nil {
			t.Fatalf("decode result: %s", encoded)
		}
	}
	tools, err := client.ListTools(ctx, &sdk.ListToolsParams{})
	if err != nil {
		t.Fatal(err)
	}
	// A model without an image returns null data rather than a business failure.
	assertNullData := func(label, encoded string, result Result) {
		t.Helper()
		if result.Error != nil || result.Data != nil {
			t.Fatalf("%s: %s", label, encoded)
		}
		if !strings.Contains(encoded, `"data":null`) && strings.TrimSpace(encoded) != "{}" {
			t.Fatalf("%s data was not null: %s", label, encoded)
		}
		for _, leaked := range []string{`"id"`, `"sha256"`, "object_key", "store_id"} {
			if strings.Contains(encoded, leaked) {
				t.Fatalf("%s leaked image metadata: %s", label, encoded)
			}
		}
	}
	found := map[string]bool{}
	for _, tool := range tools.Tools {
		if tool.Name != "get_model_image" && tool.Name != "import_model_image_from_url" && tool.Name != "upload_model_image" {
			continue
		}
		found[tool.Name] = true
		wantReadOnly := tool.Name == "get_model_image"
		if tool.Annotations == nil || tool.Annotations.ReadOnlyHint != wantReadOnly {
			t.Fatalf("%s read-only annotation", tool.Name)
		}
		schema, _ := json.Marshal(tool.InputSchema)
		if strings.Contains(string(schema), "tenant_id") || strings.Contains(string(schema), "user_id") || strings.Contains(string(schema), "object_key") || strings.Contains(string(schema), "store_id") {
			t.Fatalf("%s accepts private identity or storage input: %s", tool.Name, schema)
		}
		if tool.Name == "upload_model_image" {
			for _, private := range []string{`"url"`, `"path"`, `"file"`, `"credential"`} {
				if strings.Contains(string(schema), private) {
					t.Fatalf("upload accepts non-content input %s: %s", private, schema)
				}
			}
			for _, required := range []string{"content_base64", "model_id", "request_key"} {
				if !strings.Contains(string(schema), required) {
					t.Fatalf("upload schema is missing %s: %s", required, schema)
				}
			}
		}
	}
	if !found["get_model_image"] || !found["import_model_image_from_url"] || !found["upload_model_image"] {
		t.Fatal("image tools are not discoverable")
	}
	// An unset image is an explicit null, not a business failure.
	absent, encoded := call("get_model_image", ModelImageInput{ModelID: model.ID})
	assertNullData("absent image", encoded, absent)
	for _, private := range []string{owner.TenantID, "object_key", "store_id", "ObjectKey", "StoreID", "tenant_id"} {
		if strings.Contains(encoded, private) {
			t.Fatalf("absent image leaked private metadata: %s", encoded)
		}
	}
	input := ImportModelImageInput{ModelID: model.ID, URL: "https://images.example/model.png", SourceURL: "https://example.com/product", RequestKey: "mcp-image-import"}
	result, encoded := call("import_model_image_from_url", input)
	if result.Error != nil {
		t.Fatalf("import failed: %s", encoded)
	}
	var imported ModelImageResult
	decode(result, &imported)
	if imported.ID == "" || imported.ModelID != model.ID || imported.ContentType != "image/png" || imported.SizeBytes == 0 || imported.SHA256 == "" || imported.SourceURL != "https://example.com/product" {
		t.Fatalf("import result: %+v", imported)
	}
	for _, private := range []string{owner.TenantID, "object_key", "store_id", "ObjectKey", "StoreID", "tenant_id"} {
		if strings.Contains(encoded, private) {
			t.Fatalf("import leaked private metadata: %s", encoded)
		}
	}
	downloads := fixture.calls.Load()
	if replay, replayEncoded := call("import_model_image_from_url", input); replay.Error != nil || fixture.calls.Load() != downloads {
		t.Fatalf("replay re-downloaded or failed: %s", replayEncoded)
	}
	current, encoded := call("get_model_image", ModelImageInput{ModelID: model.ID})
	var active ModelImageResult
	decode(current, &active)
	if current.Error != nil || active.ID != imported.ID || active.SHA256 != imported.SHA256 {
		t.Fatalf("active image: %+v %s", active, encoded)
	}
	changed := input
	changed.URL = "https://images.example/changed.png"
	if conflict, _ := call("import_model_image_from_url", changed); conflict.Error == nil || conflict.Error.Code != "invalid_input" || fixture.calls.Load() != downloads {
		t.Fatalf("conflicting replay: %+v", conflict)
	}
	// Bounded content upload shares the durable receipt path but never the network.
	encodePNG := func(size int) string {
		var b bytes.Buffer
		if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, size, size))); err != nil {
			t.Fatal(err)
		}
		return base64.StdEncoding.EncodeToString(b.Bytes())
	}
	content := encodePNG(4)
	uploadInput := UploadModelImageInput{ModelID: model.ID, ContentBase64: content, SourceURL: "https://example.com/product", RequestKey: "mcp-image-upload"}
	uploads := fixture.calls.Load()
	uploaded, uploadEncoded := call("upload_model_image", uploadInput)
	if uploaded.Error != nil {
		t.Fatalf("content upload failed: %s", uploadEncoded)
	}
	var activeUpload ModelImageResult
	decode(uploaded, &activeUpload)
	if activeUpload.ID == "" || activeUpload.ModelID != model.ID || activeUpload.ContentType != "image/png" || activeUpload.SizeBytes == 0 || activeUpload.SHA256 == "" || activeUpload.SourceURL != "https://example.com/product" {
		t.Fatalf("upload result: %+v", activeUpload)
	}
	if fixture.calls.Load() != uploads {
		t.Fatal("content upload consulted the downloader")
	}
	for _, private := range []string{owner.TenantID, "object_key", "store_id", "ObjectKey", "StoreID", "tenant_id", content, `C:\\`, "content_base64"} {
		if strings.Contains(uploadEncoded, private) {
			t.Fatalf("upload echoed content, a path or storage metadata: %s", uploadEncoded)
		}
	}
	if replay, replayEncoded := call("upload_model_image", uploadInput); replay.Error != nil || fixture.calls.Load() != uploads {
		t.Fatalf("upload replay failed or used the downloader: %s", replayEncoded)
	}
	laterContent := encodePNG(5)
	for _, invalid := range []UploadModelImageInput{
		{ModelID: model.ID, ContentBase64: "not base64!!", RequestKey: "mcp-image-bad-base64"},
		{ModelID: model.ID, ContentBase64: base64.StdEncoding.EncodeToString([]byte("<svg/>")), RequestKey: "mcp-image-not-image"},
		{ModelID: model.ID, ContentBase64: "", RequestKey: "mcp-image-empty"},
		{ModelID: model.ID, ContentBase64: strings.Repeat("A", base64.StdEncoding.EncodedLen(application.MaxModelImageBytes)+1), RequestKey: "mcp-image-oversize"},
	} {
		if rejected, _ := call("upload_model_image", invalid); rejected.Error == nil || rejected.Error.Code != "invalid_input" {
			t.Fatalf("invalid content was accepted: %+v", rejected)
		}
	}
	if fixture.calls.Load() != uploads {
		t.Fatal("invalid content upload reached the downloader")
	}
	conflictingUpload := uploadInput
	conflictingUpload.ContentBase64 = laterContent
	if rejected, _ := call("upload_model_image", conflictingUpload); rejected.Error == nil || rejected.Error.Code != "invalid_input" || fixture.calls.Load() != uploads {
		t.Fatalf("changed content under one key was accepted: %+v", rejected)
	}
	// Authorization is checked before the receipt or the network.
	identity.Scopes = []string{ScopeRead}
	if denied, _ := call("import_model_image_from_url", input); denied.Error == nil || denied.Error.Code != "forbidden" {
		t.Fatalf("read-only scope import: %+v", denied)
	}
	if denied, _ := call("upload_model_image", uploadInput); denied.Error == nil || denied.Error.Code != "forbidden" {
		t.Fatalf("read-only scope upload: %+v", denied)
	}
	identity.Scopes = []string{ScopeRead, ScopeCatalog}
	identity.Principal.Role = application.RoleViewer
	if denied, _ := call("import_model_image_from_url", input); denied.Error == nil || denied.Error.Code != "forbidden" {
		t.Fatalf("viewer import: %+v", denied)
	}
	if denied, _ := call("upload_model_image", uploadInput); denied.Error == nil || denied.Error.Code != "forbidden" {
		t.Fatalf("viewer upload: %+v", denied)
	}
	if fixture.calls.Load() != uploads {
		t.Fatal("denied image writes reached the downloader")
	}
	if visible, _ := call("get_model_image", ModelImageInput{ModelID: model.ID}); visible.Error != nil {
		t.Fatalf("viewer read: %+v", visible)
	}
	identity.Principal = owner
	identity.Scopes = []string{ScopeCatalog}
	if denied, _ := call("get_model_image", ModelImageInput{ModelID: model.ID}); denied.Error == nil || denied.Error.Code != "forbidden" {
		t.Fatal("missing read scope was accepted")
	}
	// A replayed key returns its original revision but must never overwrite a later
	// active upload, and an unsafe attribution source never reaches the downloader.
	identity.Scopes = []string{ScopeRead, ScopeCatalog}
	later, laterEncoded := call("upload_model_image", UploadModelImageInput{ModelID: model.ID, ContentBase64: laterContent, SourceURL: "https://example.com/product", RequestKey: "mcp-image-upload-later"})
	var laterImage ModelImageResult
	decode(later, &laterImage)
	if later.Error != nil || laterImage.ID == "" || laterImage.ID == activeUpload.ID {
		t.Fatalf("later upload: %+v %s", later, laterEncoded)
	}
	if replay, _ := call("upload_model_image", uploadInput); replay.Error != nil {
		t.Fatalf("original key replay after a later upload: %+v", replay)
	}
	activeResult, activeEncoded := call("get_model_image", ModelImageInput{ModelID: model.ID})
	decode(activeResult, &active)
	if active.ID != laterImage.ID || active.SHA256 != laterImage.SHA256 {
		t.Fatalf("key replay replaced a later active image: %+v %s", active, activeEncoded)
	}
	if rejected, _ := call("import_model_image_from_url", ImportModelImageInput{ModelID: model.ID, URL: input.URL, SourceURL: "https://user:secret@example.com/", RequestKey: "mcp-image-bad-source"}); rejected.Error == nil || rejected.Error.Code != "invalid_input" || fixture.calls.Load() != uploads {
		t.Fatalf("credential-bearing source reached the downloader: %+v", rejected)
	}
	// A failed download leaves no active image or receipt behind.
	other, err := catalog.CreateModel(ctx, owner, application.CreateModel{CategoryID: category.ID, Name: "No image model"})
	if err != nil {
		t.Fatal(err)
	}
	fixture.err = application.ErrModel3DUnavailable
	if failed, _ := call("import_model_image_from_url", ImportModelImageInput{ModelID: other.ID, URL: input.URL, SourceURL: input.SourceURL, RequestKey: "mcp-image-failed"}); failed.Error == nil {
		t.Fatal("failed download reported success")
	}
	missing, encoded := call("get_model_image", ModelImageInput{ModelID: other.ID})
	assertNullData("failed import", encoded, missing)
}

// requestLimitBody streams a declared number of bytes and records how many were
// read, so a rejected request can be proven never to have consumed its body.
type requestLimitBody struct {
	size int
	read int
}

func (b *requestLimitBody) Read(p []byte) (int, error) {
	if b.read >= b.size {
		return 0, io.EOF
	}
	n := len(p)
	if remaining := b.size - b.read; n > remaining {
		n = remaining
	}
	for i := range p[:n] {
		p[i] = 'a'
	}
	b.read += n
	return n, nil
}

// requestLimitWriter forwards the HTTP response while remembering the status
// the transport wrote, including for streamed responses.
type requestLimitWriter struct {
	http.ResponseWriter
	status int
}

func (w *requestLimitWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *requestLimitWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(p)
}

func (w *requestLimitWriter) Flush() {
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		if w.status == 0 {
			w.status = http.StatusOK
		}
		flusher.Flush()
	}
}

// TestHTTPRequestBodyLimit proves the shared 12 MiB cap is enforced at the HTTP
// boundary. Authorization still runs first, so an unauthenticated large body is
// answered with 401 without being read, while an authenticated request above
// the cap is rejected with 413 instead of reaching the image tool.
func TestHTTPRequestBodyLimit(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	for _, tc := range []struct {
		name     string
		verifier Authenticate
	}{
		{"missing verifier", nil},
		{"rejected identity", func(context.Context, *http.Request) (Identity, error) { return Identity{}, nil }},
	} {
		body := &requestLimitBody{size: maxRequestBodyBytes + 1}
		request := httptest.NewRequest(http.MethodPost, "/mcp", body)
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Accept", "application/json, text/event-stream")
		response := httptest.NewRecorder()
		NewHandler(Services{}, tc.verifier).ServeHTTP(response, request)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("%s: status = %d, want 401", tc.name, response.Code)
		}
		if body.read != 0 {
			t.Fatalf("%s: unauthorized body was read: %d bytes", tc.name, body.read)
		}
	}
	identity := Identity{Principal: application.Principal{UserID: "limit-user", TenantID: "limit-tenant", Role: application.RoleViewer}, Scopes: []string{ScopeRead}}
	var sawTooLarge atomic.Bool
	handler := NewHandler(Services{}, func(context.Context, *http.Request) (Identity, error) { return identity, nil })
	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorder := &requestLimitWriter{ResponseWriter: w}
		handler.ServeHTTP(recorder, r)
		if recorder.status == http.StatusRequestEntityTooLarge {
			sawTooLarge.Store(true)
		}
	}))
	defer host.Close()
	client, err := sdk.NewClient(&sdk.Implementation{Name: "limit-test", Version: "1"}, nil).Connect(ctx, &sdk.StreamableClientTransport{Endpoint: host.URL}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	// The base64 argument alone reaches the cap, so the JSON-RPC envelope pushes
	// the request beyond it.
	oversize := UploadModelImageInput{ModelID: "model", ContentBase64: strings.Repeat("A", maxRequestBodyBytes), RequestKey: "mcp-image-over-http-cap"}
	if _, err := client.CallTool(ctx, &sdk.CallToolParams{Name: "upload_model_image", Arguments: oversize}); err == nil {
		t.Fatal("request beyond the cap was processed")
	}
	if !sawTooLarge.Load() {
		t.Fatal("request beyond the cap was not answered with 413")
	}
}
