package blob

import (
	"github.com/google/uuid"
	"strings"
	"testing"
)

func TestProductModel3DKey(t *testing.T) {
	tenant, model := uuid.NewString(), uuid.NewString()
	sha := strings.Repeat("a", 64)
	got, err := (ObjectKeyMapper{}).ProductModel3D(tenant, model, sha)
	if err != nil {
		t.Fatal(err)
	}
	want := "tenants/" + tenant + "/models/" + model + "/" + sha + ".glb"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if _, err := (ObjectKeyMapper{}).ProductModel3D("../x", model, sha); err == nil {
		t.Fatal("accepted invalid tenant")
	}
}
