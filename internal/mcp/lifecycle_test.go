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
}
