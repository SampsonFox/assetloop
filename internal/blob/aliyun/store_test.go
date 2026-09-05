package aliyun

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/SampsonFox/assetloop/internal/application"
)

func TestStoreAgainstHTTPDouble(t *testing.T) {
	var mu sync.Mutex
	objects := map[string][]byte{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
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
			w.Header().Set("Content-Length", "5")
			_, _ = w.Write(value)
		case http.MethodHead:
			if _, ok := objects[r.URL.Path]; !ok {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Length", "5")
		case http.MethodDelete:
			delete(objects, r.URL.Path)
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer server.Close()
	store, err := New(Config{Endpoint: server.URL, Region: "cn-test", Bucket: "bucket", AccessKeyID: "ak", AccessKeySecret: "sk", PathPrefix: "assetloop", UsePathStyle: true})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	key := "tenants/t/models/m/model.glb"
	if err := store.Put(ctx, key, bytes.NewReader([]byte("model")), application.BlobMetadata{ContentType: "model/gltf-binary"}); err != nil {
		t.Fatal(err)
	}
	info, err := store.Stat(ctx, key)
	if err != nil || info.Size != 5 {
		t.Fatalf("stat=%+v err=%v", info, err)
	}
	reader, info, err := store.Open(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(reader)
	_ = reader.Close()
	if string(got) != "model" || info.Size != 5 {
		t.Fatalf("got=%q info=%+v", got, info)
	}
	if err := store.Delete(ctx, key); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Stat(ctx, key); err == nil {
		t.Fatal("deleted object still exists")
	}
	if err := store.Put(ctx, "../escape", bytes.NewReader(nil), application.BlobMetadata{}); err == nil {
		t.Fatal("accepted traversal")
	}
}
