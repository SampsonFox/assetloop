package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/SampsonFox/assetloop/internal/application"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestTradeInToolContract proves the advertised trade-in tools over a real SDK
// HTTP session: the discovered names, field optionality, integer money, the
// required outer request key and expected pair event IDs, and the scope and role
// denials. Durable replay and conflict behavior needs a real store and is proven
// by the integration walkthrough, which calls these tools over the same transport.
func TestTradeInToolContract(t *testing.T) {
	ctx := context.Background()
	identity := Identity{
		Principal: application.Principal{TenantID: "11111111-1111-4111-8111-111111111111", UserID: "11111111-1111-4111-8111-111111111112", Role: application.RoleOwner},
		Scopes:    []string{ScopeRead, ScopeLifecycle},
	}
	server := sdk.NewServer(&sdk.Implementation{Name: "trade-in-contract", Version: "1"}, nil)
	// No application service is supplied: every assertion below is decided by the
	// advertised schema, the scope/role gate or the input mapping, never by a store.
	registerTradeIn(server, Services{})
	transport := sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return server }, &sdk.StreamableHTTPOptions{Stateless: true})
	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		transport.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), identityKey{}, identity)))
	}))
	defer host.Close()
	client, err := sdk.NewClient(&sdk.Implementation{Name: "trade-in-contract", Version: "1"}, nil).Connect(ctx, &sdk.StreamableClientTransport{Endpoint: host.URL}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	list, err := client.ListTools(ctx, &sdk.ListToolsParams{})
	if err != nil {
		t.Fatal(err)
	}
	advertised := map[string]*sdk.Tool{}
	for _, tool := range list.Tools {
		advertised[tool.Name] = tool
	}
	for _, name := range []string{"preview_trade_in", "record_trade_in", "correct_trade_in_link", "cancel_trade_in_link"} {
		if advertised[name] == nil {
			t.Fatalf("advertised trade-in tool %s is missing", name)
		}
	}
	decode := func(value any) schemaContract {
		t.Helper()
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		var schema schemaContract
		if err := json.Unmarshal(data, &schema); err != nil {
			t.Fatal(err)
		}
		return schema
	}
	inputOf := func(tool *sdk.Tool) schemaContract {
		t.Helper()
		input := decode(tool.InputSchema)
		if !input.allows("object") {
			t.Fatalf("%s input is not an object schema: %+v", tool.Name, input.Types)
		}
		return input
	}
	// The advertised required sets are exact: a new top-level field must be a
	// deliberate contract change, and a dropped one cannot silently become
	// optional.
	assertExactRequired := func(tool *sdk.Tool, fields ...string) schemaContract {
		t.Helper()
		input := inputOf(tool)
		got, want := append([]string(nil), input.Required...), append([]string(nil), fields...)
		slices.Sort(got)
		slices.Sort(want)
		if !slices.Equal(got, want) {
			t.Errorf("%s required = %v, want %v", tool.Name, got, want)
		}
		return input
	}
	assertOptional := func(tool *sdk.Tool, fields ...string) {
		t.Helper()
		input := inputOf(tool)
		for _, field := range fields {
			if slices.Contains(input.Required, field) {
				t.Errorf("%s must keep %s optional", tool.Name, field)
			}
		}
	}

	preview := advertised["preview_trade_in"]
	if preview.Annotations == nil || !preview.Annotations.ReadOnlyHint {
		t.Error("preview_trade_in must be advertised as read-only")
	}
	previewInput := assertExactRequired(preview, "current_asset_id", "direction", "counterparts")
	assertOptional(preview, "current", "occurred_at", "external_reference", "notes", "request_key")
	counterparts := decode(previewInput.Properties["counterparts"])
	if !counterparts.isExact("array") {
		t.Fatalf("counterparts must be an array: %+v", counterparts.Types)
	}
	counterpart := decode(counterparts.Items)
	if !counterpart.isExact("object") {
		t.Fatal("a counterpart must be an object")
	}
	newEvent := decode(counterpart.Properties["new_event"])
	// An optional pointer field is advertised as a nullable object union, so the
	// assertion tolerates "null" without accepting any other type.
	if !newEvent.allows("object") {
		t.Fatalf("a counterpart's new_event must be an object: %+v", newEvent.Types)
	}
	// Preview accepts an incomplete new_event so the shared preview can report the
	// fields a write would still reject.
	if slices.Contains(newEvent.Required, "amount_minor") || slices.Contains(newEvent.Required, "currency") || slices.Contains(newEvent.Required, "occurred_at") {
		t.Error("preview_trade_in must allow an incomplete new_event")
	}
	if !decode(newEvent.Properties["amount_minor"]).isExact("integer") || !decode(newEvent.Properties["fx_rate_scaled"]).isExact("integer") {
		t.Error("new_event money must be integer minor units")
	}
	if _, found := newEvent.Properties["request_key"]; found {
		t.Error("a nested monetary record must not own a request key")
	}

	record := advertised["record_trade_in"]
	recordInput := assertExactRequired(record, "request_key", "current_asset_id", "direction", "counterparts")
	assertOptional(record, "current", "occurred_at", "external_reference", "notes")
	if !decode(recordInput.Properties["request_key"]).isExact("string") {
		t.Error("record_trade_in must require a string outer request_key")
	}
	recordCounterpart := decode(decode(recordInput.Properties["counterparts"]).Items)
	recordNewEvent := decode(recordCounterpart.Properties["new_event"])
	if !decode(recordNewEvent.Properties["amount_minor"]).isExact("integer") || !decode(recordNewEvent.Properties["fx_rate_scaled"]).isExact("integer") {
		t.Error("record_trade_in new_event money must be integer minor units")
	}
	if _, found := recordNewEvent.Properties["request_key"]; found {
		t.Error("a nested monetary record must not own a request key")
	}

	correct := advertised["correct_trade_in_link"]
	assertExactRequired(correct, "request_key", "link_id", "expected_source_event_id", "expected_destination_event_id", "new_asset_id", "old_asset_id")
	assertOptional(correct, "occurred_at", "external_reference", "notes")
	cancel := advertised["cancel_trade_in_link"]
	assertExactRequired(cancel, "request_key", "link_id", "expected_source_event_id", "expected_destination_event_id")
	assertOptional(cancel, "occurred_at", "notes")

	// Every refusal returns the shared safe error envelope without infrastructure
	// details, so a caller can act on the code and the localized key.
	refuse := func(name string, args any) *ToolError {
		t.Helper()
		result, err := client.CallTool(ctx, &sdk.CallToolParams{Name: name, Arguments: args})
		if err != nil {
			t.Fatalf("%s call failed: %v", name, err)
		}
		if !result.IsError {
			t.Fatalf("%s should have been refused", name)
		}
		data, err := json.Marshal(result.StructuredContent)
		if err != nil {
			t.Fatal(err)
		}
		var decoded Result
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatal(err)
		}
		if decoded.Error == nil || decoded.Data != nil {
			t.Fatalf("%s lost its error contract: %s", name, data)
		}
		for _, hidden := range []string{"select ", "sql", "store_id", "object_key", "tenant_id"} {
			if strings.Contains(strings.ToLower(decoded.Error.Message), hidden) {
				t.Fatalf("%s leaked internal detail: %s", name, decoded.Error.Message)
			}
		}
		return decoded.Error
	}
	pairArgs := map[string]any{
		"request_key": "trade-in-denied", "link_id": "11111111-1111-4111-8111-111111111113",
		"expected_source_event_id": "11111111-1111-4111-8111-111111111114", "expected_destination_event_id": "11111111-1111-4111-8111-111111111115",
		"new_asset_id": "11111111-1111-4111-8111-111111111116", "old_asset_id": "11111111-1111-4111-8111-111111111117",
	}
	recordArgs := map[string]any{
		"request_key": "trade-in-denied", "current_asset_id": "11111111-1111-4111-8111-111111111116", "direction": "source",
		"counterparts": []any{map[string]any{"asset_id": "11111111-1111-4111-8111-111111111117", "existing_event_id": "11111111-1111-4111-8111-111111111114"}},
	}
	// Cancellation advertises its own schema without the pair-asset fields that
	// only correction accepts, so its denied probe must not carry them; an
	// unknown property would be rejected by the transport before the shared
	// authorization gate could answer with the forbidden envelope.
	cancelArgs := map[string]any{
		"request_key": "trade-in-denied", "link_id": "11111111-1111-4111-8111-111111111113",
		"expected_source_event_id": "11111111-1111-4111-8111-111111111114", "expected_destination_event_id": "11111111-1111-4111-8111-111111111115",
	}
	identity.Principal.Role = application.RoleViewer
	for _, name := range []string{"record_trade_in", "correct_trade_in_link", "cancel_trade_in_link"} {
		args := any(pairArgs)
		switch name {
		case "record_trade_in":
			args = recordArgs
		case "cancel_trade_in_link":
			args = cancelArgs
		}
		if code := refuse(name, args); code.Code != "forbidden" {
			t.Errorf("%s must be refused for a viewer: %+v", name, code)
		}
	}
	identity.Principal.Role = application.RoleOwner
	identity.Scopes = []string{ScopeRead}
	if code := refuse("record_trade_in", recordArgs); code.Code != "forbidden" {
		t.Errorf("a client without the lifecycle scope must not record: %+v", code)
	}
	identity.Scopes = []string{ScopeLifecycle}
	if code := refuse("preview_trade_in", map[string]any{"current_asset_id": recordArgs["current_asset_id"], "direction": "source", "counterparts": []any{}}); code.Code != "forbidden" {
		t.Errorf("a client without the read scope must not preview: %+v", code)
	}

	// The documented command mapping is validated before any service call.
	identity.Scopes = []string{ScopeRead, ScopeLifecycle}
	if code := refuse("preview_trade_in", map[string]any{"current_asset_id": "11111111-1111-4111-8111-111111111116", "direction": "sideways", "counterparts": []any{}}); code.Code != "invalid_input" || !strings.Contains(code.Message, "validation.trade_in_direction") {
		t.Errorf("an unknown direction must be refused as input: %+v", code)
	}
	if code := refuse("preview_trade_in", map[string]any{"current_asset_id": "11111111-1111-4111-8111-111111111116", "direction": "source", "counterparts": []any{}, "occurred_at": "not-a-time"}); code.Code != "invalid_input" {
		t.Errorf("an invalid trade-in timestamp must be refused as input: %+v", code)
	}
	if code := refuse("record_trade_in", map[string]any{"request_key": " ", "current_asset_id": "11111111-1111-4111-8111-111111111116", "direction": "source", "counterparts": []any{}}); code.Code != "invalid_input" || !strings.Contains(code.Message, "validation.request_key") {
		t.Errorf("record_trade_in must require an outer request key: %+v", code)
	}
	conflicting := map[string]any{
		"current_asset_id": "11111111-1111-4111-8111-111111111116", "direction": "source",
		"counterparts": []any{map[string]any{
			"asset_id": "11111111-1111-4111-8111-111111111117", "existing_event_id": "11111111-1111-4111-8111-111111111114",
			"new_event": map[string]any{"amount_minor": 1000, "currency": "CNY", "occurred_at": "2026-08-01T00:00:00Z"},
		}},
	}
	if code := refuse("preview_trade_in", conflicting); code.Code != "invalid_input" || !strings.Contains(code.Message, "validation.trade_in_selection_conflict") {
		t.Errorf("a reuse plus new record must be refused as input: %+v", code)
	}
}
