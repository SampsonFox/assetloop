package web

import (
	"bytes"
	"strings"
	"testing"

	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/SampsonFox/assetloop/internal/domain"
)

func TestAssetListIconControlsAndCardContent(t *testing.T) {
	server, err := New(nil, nil, nil, nil, Options{Specifications: application.NewSpecificationService(nil)})
	if err != nil {
		t.Fatal(err)
	}
	for _, locale := range []application.Locale{application.LocaleZhCN, application.LocaleEn} {
		for _, editable := range []bool{false, true} {
			data := pageData{
				Strings: messages[locale], Principal: &application.Principal{TenantName: "Test"},
				AssetView: "grid", CanManageCatalog: editable, CanManageLifecycle: editable,
				AssetAdvanced: true, AssetHasFilters: true, AssetQuery: "phone",
				Assets:         []domain.Asset{{ID: "sample", DisplayName: "Personal phone", Category: "Phone", Model: "Model", TagSummary: "256GB", SerialNumber: "SERIAL-001"}},
				AssetSummaries: map[string]domain.AssetSummary{"sample": {BaseCurrency: "CNY", ExpenseMinor: 1010000, NetCashflowMinor: 40000}},
			}
			var out bytes.Buffer
			if err := server.templates["assets"].ExecuteTemplate(&out, "content", data); err != nil {
				t.Fatal(err)
			}
			body := out.String()
			formStart := strings.Index(body, `<form class="asset-filters asset-search-shell"`)
			viewStart := strings.Index(body, `class="asset-view-actions"`)
			searchStart := strings.Index(body, `class="asset-search"`)
			filterStart := strings.Index(body, `class="asset-filter-actions"`)
			formEnd := strings.Index(body, "</form>")
			if !(formStart < viewStart && viewStart < searchStart && searchStart < filterStart && filterStart < formEnd) {
				t.Fatal("view controls and search/filter controls must share one toolbar in reading order")
			}
			for _, want := range []string{`class="asset-card-footer"`, "Personal phone", "Phone · Model · 256GB", "SERIAL-001", "10100.00 CNY", "400.00 CNY", `name="q" value="phone"`, `name="status"`, `name="sort"`, `name="direction"`, `class="filter-active-dot"`} {
				if !strings.Contains(body, want) {
					t.Errorf("missing %q", want)
				}
			}
			for _, key := range []string{"assets.search", "assets.filter", "assets.clear_filters", "assets.list", "assets.grid"} {
				if !strings.Contains(body, `aria-label="`+messages[locale][key]+`"`) {
					t.Errorf("missing localized icon name %s", key)
				}
			}
			if strings.Contains(body, `href="/assets/sample#add-event"`) != editable {
				t.Fatal("event action does not match permission")
			}
			if strings.Contains(body, `href="/assets/new"`) != editable {
				t.Fatal("create action does not match permission")
			}
			if editable && !strings.Contains(body, `aria-label="`+messages[locale]["assets.add_event"]+` · Personal phone"`) {
				t.Fatal("event icon must identify its asset")
			}
		}
	}
}
