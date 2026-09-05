package local

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/SampsonFox/assetloop/internal/application"
)

type brokenReader struct{}

func (brokenReader) Read([]byte) (int, error) { return 0, errors.New("upload interrupted") }

func TestFailedUploadPreservesLegacyObject(t *testing.T) {
	root := t.TempDir()
	s, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	legacy := "tenants/t/models/m/old.glb"
	key := "tenants/t/model-3d-resources/r/new.glb"
	if err := s.Put(ctx, legacy, bytes.NewBufferString("legacy"), application.BlobMetadata{}); err != nil {
		t.Fatal(err)
	}
	if err := s.Put(ctx, key, io.MultiReader(bytes.NewBufferString("partial"), brokenReader{}), application.BlobMetadata{}); err == nil {
		t.Fatal("accepted interrupted upload")
	}
	if _, err := s.Stat(ctx, key); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("partial object remains: %v", err)
	}
	partials, err := filepath.Glob(filepath.Join(root, "tenants/t/model-3d-resources/r/.blob-*"))
	if err != nil || len(partials) != 0 {
		t.Fatalf("temporary files remain: %v %v", partials, err)
	}
	r, _, err := s.Open(ctx, legacy)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(r)
	_ = r.Close()
	if err != nil || string(data) != "legacy" {
		t.Fatalf("legacy bytes changed: %q %v", data, err)
	}
}

func TestDeleteFailureCanBeRetried(t *testing.T) {
	root := t.TempDir()
	s, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	key := "tenants/t/model-3d-resources/r/model.glb"
	// A nonempty directory gives a deterministic failure on Windows and Unix,
	// independent of elevated test-user permissions.
	path := filepath.Join(root, filepath.FromSlash(key))
	if err := os.MkdirAll(path, 0700); err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(path, "blocker")
	if err := os.WriteFile(child, []byte("test"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(context.Background(), key); err == nil {
		t.Fatal("expected deletion failure")
	}
	if err := os.Remove(child); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(context.Background(), key); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(context.Background(), key); err != nil {
		t.Fatalf("retry missing object: %v", err)
	}
}
