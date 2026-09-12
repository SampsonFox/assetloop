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
			for _, want := range []string{"Personal phone", "Phone · Model · 256GB", "10100.00 CNY", "400.00 CNY", `name="q" value="phone"`, `name="status"`, `name="sort"`, `name="direction"`, `class="filter-active-dot"`} {
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

func TestAssetListIdentity(t *testing.T) {
	server, err := New(nil, nil, nil, nil, Options{Specifications: application.NewSpecificationService(nil)})
	if err != nil {
		t.Fatal(err)
	}
	for _, view := range []string{"list", "grid"} {
		for _, name := range []string{"", "  ", "My phone", strings.Repeat("Long custom name ", 12)} {
			t.Run(view+"/"+name, func(t *testing.T) {
				data := pageData{Strings: messages[application.LocaleEn], Principal: &application.Principal{}, AssetView: view, CanManageLifecycle: true,
					Assets:         []domain.Asset{{ID: "item", DisplayName: name, Model: "Xiaomi 15", Category: "Phone", TagSummary: "512GB", SerialNumber: "PRIVATE-SERIAL-123"}},
					AssetSummaries: map[string]domain.AssetSummary{"item": {BaseCurrency: "CNY"}}}
				var out bytes.Buffer
				if err := server.templates["assets"].ExecuteTemplate(&out, "content", data); err != nil {
					t.Fatal(err)
				}
				body := out.String()
				want := strings.TrimSpace(name)
				if want == "" {
					want = "Xiaomi 15"
				}
				if !strings.Contains(body, `href="/assets/item">`+want+`</a>`) {
					t.Errorf("item title should be custom name or model: %q", want)
				}
				for _, forbidden := range []string{"PRIVATE-SERIAL-123", messages[application.LocaleEn]["assets.serial_missing"], `data-label="` + messages[application.LocaleEn]["assets.serial_number"] + `"`} {
					if strings.Contains(body, forbidden) {
						t.Errorf("list exposes serial detail %q", forbidden)
					}
				}
				if !strings.Contains(body, `href="/assets/item#add-event"`) {
					t.Error("event action lost")
				}
			})
		}
	}
}
