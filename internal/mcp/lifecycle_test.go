package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/SampsonFox/assetloop/internal/domain"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func testLifecycleTools(t *testing.T, s Services, owner application.Principal, category string) {
	ctx := context.Background()
	model, err := s.Catalog.CreateModel(ctx, owner, application.CreateModel{CategoryID: category, Name: "MCP camera"})
	if err != nil {
		t.Fatal(err)
	}
	asset, err := s.Specifications.SaveAsset(ctx, owner, application.SaveSpecificationAsset{ModelID: model.ID, DisplayName: "MCP test item"})
	if err != nil {
		t.Fatal(err)
	}
	types, err := s.Lifecycle.EventTypePage(ctx, owner, application.EventTypeListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var purchase string
	for _, kind := range types.Types {
		if kind.SystemCode == "purchase" {
			purchase = kind.ID
		}
	}
	if purchase == "" {
		t.Fatal("missing purchase type")
	}
	identity := Identity{Principal: owner, Scopes: []string{ScopeRead, ScopeLifecycle}}
	host := httptest.NewServer(NewHandler(s, func(context.Context, *http.Request) (Identity, error) { return identity, nil }))
	defer host.Close()
	client, err := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "1"}, nil).Connect(ctx, &sdk.StreamableClientTransport{Endpoint: host.URL}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	call := func(name string, args any, wantError bool) domain.AssetEvent {
		t.Helper()
		result, err := client.CallTool(ctx, &sdk.CallToolParams{Name: name, Arguments: args})
		if err != nil {
			t.Fatal(err)
		}
		if result.IsError != wantError {
			t.Fatalf("%s error=%v want=%v: %v", name, result.IsError, wantError, result.Content)
		}
		var decoded struct{ Data domain.AssetEvent }
		encoded, _ := json.Marshal(result.StructuredContent)
		if err := json.Unmarshal(encoded, &decoded); err != nil {
			t.Fatal(err)
		}
		return decoded.Data
	}
	input := EventInput{AssetID: asset.ID, TypeID: purchase, EventFields: EventFields{RequestKey: "mcp-purchase", AmountMinor: 10000, Currency: "CNY", OccurredAt: "2026-01-01T12:00:00Z"}}
	first := call("record_event", input, false)
	retry := call("record_event", input, false)
	if first.ID == "" || retry.ID != first.ID {
		t.Fatal("retry duplicated purchase")
	}
	input.AmountMinor = 11000
	call("record_event", input, true)
	input.RequestKey = ""
	call("record_event", input, true)
	input.RequestKey = "mcp-correction"
	replacement := CorrectEventInput{EventID: first.ID, Replacement: input.EventFields}
	corrected := call("correct_event", replacement, false)
	if again := call("correct_event", replacement, false); again.ID != corrected.ID {
		t.Fatal("retry duplicated correction")
	}
	events, _, err := s.Lifecycle.Timeline(ctx, owner, asset.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 {
		t.Fatalf("append-only history has %d rows", len(events))
	}
	original, err := s.Lifecycle.GetEvent(ctx, owner, first.ID)
	if err != nil || !original.IsVoided {
		t.Fatal("original not preserved as voided")
	}
	identity.Scopes = []string{ScopeRead}
	input.RequestKey = "denied-write"
	call("record_event", input, true)
	identity.Scopes = []string{ScopeLifecycle}
	identity.Principal.Role = application.RoleViewer
	call("record_event", input, true)
	identity = Identity{Principal: owner, Scopes: []string{ScopeCatalog}}
	categoryInput := CategoryInput{RequestKey: "tool-category", Name: "Tool category", IconKey: "camera"}
	created := call("create_category", categoryInput, false)
	if created.ID == "" || call("create_category", categoryInput, false).ID != created.ID {
		t.Fatal("category tool replay failed")
	}
	categoryInput.RequestKey, categoryInput.Name = "tool-category-update", "Updated tool category"
	call("update_category", UpdateCategoryInput{CategoryInput: categoryInput, ID: created.ID}, false)
	modelInput := CreateModelInput{RequestKey: "tool-model", CategoryID: created.ID, Name: "Tool model"}
	createdModel := call("create_product_model", modelInput, false)
	if createdModel.ID == "" || call("create_product_model", modelInput, false).ID != createdModel.ID {
		t.Fatal("model tool replay failed")
	}
	kind := call("save_tag_type", SaveTagTypeInput{RequestKey: "http-tag-type", Name: "Capacity", Enabled: true}, false)
	tagInput := SaveTagInput{RequestKey: "http-tag", TypeID: kind.ID, Name: "256GB", Enabled: true}
	tag := call("save_specification_tag", tagInput, false)
	if tag.ID == "" || call("save_specification_tag", tagInput, false).ID != tag.ID {
		t.Fatal("tag tool replay failed")
	}
	call("save_model_configuration", SaveModelInput{RequestKey: "http-model-config", ModelID: createdModel.ID, TagIDs: []string{tag.ID}, AppearanceOverrides: map[string]bool{}}, false)
	assetInput := SaveAssetInput{RequestKey: "http-asset", ModelID: createdModel.ID, DisplayName: "From MCP", TagIDs: []string{tag.ID}}
	createdAsset := call("save_asset", assetInput, false)
	if createdAsset.ID == "" || call("save_asset", assetInput, false).ID != createdAsset.ID {
		t.Fatal("asset tool replay failed")
	}
	persisted, err := s.Specifications.Asset(ctx, owner, createdAsset.ID)
	if err != nil || persisted.DisplayName != "From MCP" {
		t.Fatal("asset not visible through shared service")
	}
	assetInput.ID = createdAsset.ID
	assetInput.RequestKey = "http-asset-update"
	assetInput.DisplayName = "Updated through MCP"
	call("save_asset", assetInput, false)
	identity.Scopes = []string{ScopeLifecycle}
	typeInput := CreateEventTypeInput{RequestKey: "http-event-type", Name: "Cleaning", Cashflow: "expense"}
	custom := call("create_event_type", typeInput, false)
	if custom.ID == "" || call("create_event_type", typeInput, false).ID != custom.ID {
		t.Fatal("event type replay failed")
	}
	call("update_event_type", UpdateEventTypeInput{ID: custom.ID, CreateEventTypeInput: CreateEventTypeInput{RequestKey: "http-type-rename", Name: "Cleaning fee", Cashflow: "expense"}}, false)
	call("record_event", EventInput{AssetID: asset.ID, TypeID: custom.ID, EventFields: EventFields{RequestKey: "http-custom-expense", AmountMinor: 500, Currency: "CNY", OccurredAt: "2026-01-02T12:00:00Z"}}, false)
	call("update_event_type", UpdateEventTypeInput{ID: custom.ID, CreateEventTypeInput: CreateEventTypeInput{RequestKey: "http-type-direction", Name: "Cleaning fee", Cashflow: "income"}}, true)
	call("set_event_type_enabled", EnableEventTypeInput{RequestKey: "http-type-disable", ID: custom.ID, Enabled: false}, false)
	call("set_event_type_enabled", EnableEventTypeInput{RequestKey: "http-type-disable", ID: custom.ID, Enabled: false}, false)
	call("set_event_type_enabled", EnableEventTypeInput{RequestKey: "http-builtin-disable", ID: purchase, Enabled: false}, true)
}
