package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/SampsonFox/assetloop/internal/application"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type schemaContract struct {
	Type        string
	Description string
	Required    []string
	Properties  map[string]json.RawMessage
}

func assertToolSchema(t *testing.T, tool *sdk.Tool, write bool) {
	t.Helper()
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
	input, output := decode(tool.InputSchema), decode(tool.OutputSchema)
	// These are client-visible instructions from tools/list, not source comments.
	assertMeaning := func(description string, concepts ...string) {
		t.Helper()
		for _, concept := range concepts {
			if !strings.Contains(description, concept) {
				t.Errorf("%s discovery description missing %q: %s", tool.Name, concept, description)
			}
		}
	}
	switch tool.Name {
	case "save_asset":
		if slices.Contains(input.Required, "display_name") {
			t.Fatal("custom item name must be optional")
		}
		assertMeaning(decode(input.Properties["display_name"]).Description, "Optional custom", "model name", "omission clears")
	case "record_event":
		assertMeaning(tool.Description, "list_event_types", "notes", "purchase")
		assertMeaning(decode(input.Properties["type_id"]).Description, "reusable", "not a product")
		assertMeaning(decode(input.Properties["notes"]).Description, "product or service", "purchase")
	case "correct_event":
		assertMeaning(tool.Description, "does not change", "event type")
		replacement := decode(input.Properties["replacement"])
		assertMeaning(decode(replacement.Properties["notes"]).Description, "product or service")
	case "create_event_type":
		assertMeaning(tool.Description, "list_event_types", "explicitly", "notes")
		assertMeaning(decode(input.Properties["name"]).Description, "reusable", "not a product")
	case "update_event_type":
		assertMeaning(decode(input.Properties["name"]).Description, "reusable", "not a product")
	case "list_event_types":
		assertMeaning(tool.Description, "reusable", "notes")
	}
	if input.Type != "object" || output.Type != "object" || output.Properties["data"] == nil || output.Properties["error"] == nil {
		t.Fatalf("%s has an invalid input/result envelope", tool.Name)
	}
	if !write {
		return
	}
	if tool.Name == "correct_event" {
		if !slices.Contains(input.Required, "replacement") {
			t.Fatal("correction replacement is optional")
		}
		input = decode(input.Properties["replacement"])
	}
	if !slices.Contains(input.Required, "request_key") || decode(input.Properties["request_key"]).Type != "string" {
		t.Fatalf("%s does not require a string request_key", tool.Name)
	}
	if tool.Name == "record_event" || tool.Name == "correct_event" {
		for _, field := range []string{"amount_minor", "fx_rate_scaled"} {
			if decode(input.Properties[field]).Type != "integer" {
				t.Fatalf("%s %s is not an integer", tool.Name, field)
			}
		}
	}
}

func TestSafeToolErrorContract(t *testing.T) {
	for _, tc := range []struct {
		name      string
		err       error
		code      string
		retryable bool
	}{
		{"input", application.NewInputError("validation.request_conflict"), "invalid_input", false},
		{"forbidden", application.ErrForbidden, "forbidden", false},
		{"unauthorized", application.ErrUnauthorized, "forbidden", false},
		{"missing", application.ErrModel3DNotFound, "not_found", false},
		{"references", application.ErrModel3DReferenced, "referenced", false},
		{"voided", application.ErrAlreadyVoided, "conflict", false},
		{"internal", errors.New("private-driver-detail /private/storage/secret-marker"), "unavailable", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			server := sdk.NewServer(&sdk.Implementation{Name: "error-contract", Version: "1"}, nil)
			register(server, "error_test", "Contract test", ScopeRead, application.CapabilityView, func(context.Context, application.Principal, Empty) (any, error) { return nil, tc.err })
			transport := sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return server }, &sdk.StreamableHTTPOptions{Stateless: true})
			host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				identity := Identity{Principal: application.Principal{TenantID: "test-tenant", UserID: "test-user", Role: application.RoleViewer}, Scopes: []string{ScopeRead}}
				transport.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), identityKey{}, identity)))
			}))
			defer host.Close()
			client, err := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "1"}, nil).Connect(ctx, &sdk.StreamableClientTransport{Endpoint: host.URL}, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			result, err := client.CallTool(ctx, &sdk.CallToolParams{Name: "error_test", Arguments: Empty{}})
			if err != nil || !result.IsError {
				t.Fatal("business error lost")
			}
			data, err := json.Marshal(result.StructuredContent)
			if err != nil {
				t.Fatal(err)
			}
			var decoded Result
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatal(err)
			}
			if decoded.Error == nil || decoded.Error.Code != tc.code || decoded.Error.Retryable != tc.retryable || decoded.Data != nil {
				t.Fatalf("wrong error contract: %s", data)
			}
			all, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(all), "secret-marker") || strings.Contains(string(all), "private-driver-detail") {
				t.Fatal("internal error exposed")
			}
		})
	}
}
