package web

import (
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestAssetFormDoesNotEchoObsoleteColor(t *testing.T) {
	values := url.Values{"variant_id": {"variant"}, "display_name": {"Phone"}, "color": {"obsolete"}, "notes": {"Keep notes"}}
	r := httptest.NewRequest("POST", "/assets", strings.NewReader(values.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	asset := assetFromForm(r, "asset")
	if asset.Color != "" || asset.VariantID != "variant" || asset.DisplayName != "Phone" || asset.Notes != "Keep notes" {
		t.Fatalf("unexpected form values: %+v", asset)
	}
}
