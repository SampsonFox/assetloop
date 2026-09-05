package aliyun

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/SampsonFox/assetloop/internal/application"
)

func TestUploadFailureAndDeleteRetryPreserveLegacy(t *testing.T) {
	var mu sync.Mutex
	failPut, failDelete := true, true
	legacy := "/bucket/tenants/t/models/m/old.glb"
	objects := map[string][]byte{legacy: []byte("legacy")}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if (r.Method == http.MethodPut && failPut) || (r.Method == http.MethodDelete && failDelete) {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(w, `<Error><Code>AccessDenied</Code><Message>test failure</Message></Error>`)
			return
		}
		switch r.Method {
		case http.MethodPut:
			objects[r.URL.Path], _ = io.ReadAll(r.Body)
			w.Header().Set("ETag", `"test"`)
		case http.MethodGet:
			value, ok := objects[r.URL.Path]
			if !ok {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write(value)
		case http.MethodDelete:
			delete(objects, r.URL.Path)
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer server.Close()
	s, err := New(Config{Endpoint: server.URL, Region: "cn-test", Bucket: "bucket", AccessKeyID: "test-key", AccessKeySecret: "test-secret", UsePathStyle: true})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	key := "tenants/t/model-3d-resources/r/new.glb"
	if err := s.Put(ctx, key, bytes.NewBufferString("new"), application.BlobMetadata{}); err == nil {
		t.Fatal("upload failure lost")
	}
	mu.Lock()
	_, partial := objects["/bucket/"+key]
	failPut = false
	mu.Unlock()
	if partial {
		t.Fatal("failed upload retained object")
	}
	if err := s.Put(ctx, key, bytes.NewBufferString("new"), application.BlobMetadata{}); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(ctx, key); err == nil {
		t.Fatal("delete failure lost")
	}
	mu.Lock()
	failDelete = false
	mu.Unlock()
	if err := s.Delete(ctx, key); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(ctx, key); err != nil {
		t.Fatalf("repeat delete: %v", err)
	}
	r, _, err := s.Open(ctx, strings.TrimPrefix(legacy, "/bucket/"))
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(r)
	_ = r.Close()
	if err != nil || string(data) != "legacy" {
		t.Fatalf("legacy object changed: %q %v", data, err)
	}
}
