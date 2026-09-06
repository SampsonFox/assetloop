package web

import (
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestAssetFormDoesNotEchoObsoleteColor(t *testing.T) {
	values := url.Values{"model_id": {"model"}, "tag_ids": {"tag"}, "variant_id": {"obsolete"}, "display_name": {"Phone"}, "color": {"obsolete"}, "notes": {"Keep notes"}}
	r := httptest.NewRequest("POST", "/assets", strings.NewReader(values.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	asset := assetFromForm(r, "asset")
	if asset.ModelID != "model" || asset.DisplayName != "Phone" || asset.Notes != "Keep notes" || len(asset.Tags) != 1 || asset.Tags[0].ID != "tag" {
		t.Fatalf("unexpected form values: %+v", asset)
	}
}
