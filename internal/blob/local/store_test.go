package local

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/SampsonFox/assetloop/internal/application"
)

func TestStoreRoundTripAndTraversal(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	key := "tenants/t/models/m/a.glb"
	if err := store.Put(ctx, key, bytes.NewReader([]byte("model")), application.BlobMetadata{}); err != nil {
		t.Fatal(err)
	}
	info, err := store.Stat(ctx, key)
	if err != nil || info.Size != 5 {
		t.Fatalf("stat=%#v err=%v", info, err)
	}
	r, _, err := store.Open(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(r)
	r.Close()
	if string(got) != "model" {
		t.Fatalf("got %q", got)
	}
	if err := store.Delete(ctx, key); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"../escape", "/absolute", "a\\b"} {
		if err := store.Put(ctx, bad, bytes.NewReader(nil), application.BlobMetadata{}); err == nil {
			t.Fatalf("accepted %q", bad)
		}
	}
}
