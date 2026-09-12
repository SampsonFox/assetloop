package integration_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/SampsonFox/assetloop/internal/application"
	transport "github.com/SampsonFox/assetloop/internal/mcp"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func testConfigurationQueries(t *testing.T, store application.ManagementStore, owner application.Principal, manager *application.ManagementService, modelID, resourceID, categoryID string) {
	t.Helper()
	ctx := context.Background()
	color, err := manager.SaveType(ctx, owner, "config-color-type", application.SaveSpecificationType{Name: "Finish", Enabled: true, AffectsAppearance: true})
	if err != nil {
		t.Fatal(err)
	}
	capacity, err := manager.SaveType(ctx, owner, "config-capacity-type", application.SaveSpecificationType{Name: "Size", Enabled: true, AffectsAppearance: true})
	if err != nil {
		t.Fatal(err)
	}
	black, err := manager.SaveTag(ctx, owner, "config-color-tag", application.SaveSpecificationTag{TypeID: color.ID, Name: "Black", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	large, err := manager.SaveTag(ctx, owner, "config-capacity-tag", application.SaveSpecificationTag{TypeID: capacity.ID, Name: "Large", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.SaveModel(ctx, owner, "config-model", application.SaveModelSpecification{ModelID: modelID, TagIDs: []string{black.ID, large.ID}, AppearanceOverrides: map[string]bool{capacity.ID: false}}); err != nil {
		t.Fatal(err)
	}
	rule, err := manager.SaveAppearance(ctx, owner, "config-rule", application.SaveAppearanceDefault{ModelID: modelID, ResourceID: resourceID, TagIDs: []string{black.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.SaveResource(ctx, owner, "config-resource", application.SaveResourceSpecification{ResourceID: resourceID, TagIDs: []string{black.ID}, CategoryIDs: []string{categoryID}}); err != nil {
		t.Fatal(err)
	}
	identity := transport.Identity{Principal: owner, Scopes: []string{transport.ScopeRead}}
	host := httptest.NewServer(transport.NewHandler(transport.Services{Specifications: application.NewSpecificationService(store)}, func(context.Context, *http.Request) (transport.Identity, error) { return identity, nil }))
	defer host.Close()
	client, err := sdk.NewClient(&sdk.Implementation{Name: "configuration-test", Version: "1"}, nil).Connect(ctx, &sdk.StreamableClientTransport{Endpoint: host.URL}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	call := func(name string, args any, output any, wantError bool) {
		t.Helper()
		result, err := client.CallTool(ctx, &sdk.CallToolParams{Name: name, Arguments: args})
		if err != nil {
			t.Fatal(err)
		}
		if result.IsError != wantError {
			t.Fatalf("%s: unexpected result: %v", name, result.Content)
		}
		if output != nil {
			data, err := json.Marshal(result.StructuredContent)
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(data, output); err != nil {
				t.Fatal(err)
			}
		}
	}
	var model struct {
		Data transport.ModelConfigurationResult
	}
	call("get_model_configuration", transport.IDInput{ID: modelID}, &model, false)
	if model.Data.ModelID != modelID || len(model.Data.TagIDs) != 2 || len(model.Data.Defaults) != 1 || model.Data.Defaults[0].ID != rule.ID || model.Data.Defaults[0].ResourceID != resourceID || !reflect.DeepEqual(model.Data.Defaults[0].TagIDs, []string{black.ID}) {
		t.Fatalf("incomplete model configuration: %+v", model.Data)
	}
	if !reflect.DeepEqual(model.Data.AppearanceOverrides, map[string]bool{capacity.ID: false}) {
		t.Fatal("explicit false override or inherited dimension changed")
	}
	var resource struct {
		Data transport.ResourceConfigurationResult
	}
	call("get_resource_configuration", transport.IDInput{ID: resourceID}, &resource, false)
	if resource.Data.ResourceID != resourceID || !reflect.DeepEqual(resource.Data.TagIDs, []string{black.ID}) || !reflect.DeepEqual(resource.Data.CategoryIDs, []string{categoryID}) {
		t.Fatal("incomplete resource associations")
	}
	var refs struct {
		Data struct {
			References []transport.SpecificationReferenceResult
			Total      int
		}
	}
	query := transport.SpecificationReferencesInput{ID: black.ID, Kind: "tag", PageSize: 1}
	call("get_specification_references", query, &refs, false)
	if refs.Data.Total != 3 || len(refs.Data.References) != 1 {
		t.Fatalf("references not paged: %+v", refs.Data)
	}
	if refs.Data.References[0].Kind != "appearance" {
		t.Fatal("reference paging is not stably ordered")
	}
	query.Page = 2
	call("get_specification_references", query, &refs, false)
	if len(refs.Data.References) != 1 || refs.Data.References[0].Kind != "model" {
		t.Fatal("second reference page is incorrect")
	}
	query.Page = 10
	call("get_specification_references", query, &refs, false)
	if refs.Data.Total != 3 || len(refs.Data.References) != 0 {
		t.Fatal("out-of-range page changed total")
	}
	query.ID, query.Kind, query.Page, query.PageSize = color.ID, "type", 1, 20
	call("get_specification_references", query, &refs, false)
	if refs.Data.Total != 3 || len(refs.Data.References) != 3 {
		t.Fatal("type references incomplete")
	}
	var candidates struct {
		Data struct {
			Candidates []transport.AppearanceCandidateResult
			Total      int
		}
	}
	candidateInput := transport.AppearanceCandidateInput{ModelID: modelID, TagIDs: []string{black.ID, large.ID}, PageSize: 1}
	call("search_appearance_candidates", candidateInput, &candidates, false)
	if candidates.Data.Total != 1 || len(candidates.Data.Candidates) != 1 || candidates.Data.Candidates[0].Resource.ID != resourceID || candidates.Data.Candidates[0].MatchingTags != 1 || !candidates.Data.Candidates[0].DescriptionComplete {
		t.Fatalf("candidate query lost appearance policy: %+v", candidates.Data)
	}
	candidateInput.Page = 2
	call("search_appearance_candidates", candidateInput, &candidates, false)
	if candidates.Data.Total != 1 || len(candidates.Data.Candidates) != 0 {
		t.Fatal("candidate paging changed total")
	}
	query.Kind = "unknown"
	call("get_specification_references", query, nil, true)
	for _, denied := range []string{"scope", "tenant"} {
		identity.Scopes = nil
		if denied == "tenant" {
			identity.Scopes = []string{transport.ScopeRead}
			identity.Principal.TenantID = "00000000-0000-4000-8000-000000000001"
		}
		call("get_model_configuration", transport.IDInput{ID: modelID}, nil, true)
		call("get_resource_configuration", transport.IDInput{ID: resourceID}, nil, true)
		call("get_specification_references", transport.SpecificationReferencesInput{ID: black.ID, Kind: "tag"}, nil, true)
		call("search_appearance_candidates", candidateInput, nil, true)
	}
}
