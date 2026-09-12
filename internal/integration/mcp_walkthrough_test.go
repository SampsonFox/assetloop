package integration_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// Reuse the full-element scenario's real OAuth bearer connection. Do not replace
// tool calls with application-service calls or count discovery as execution.
func runMCPToolWalkthrough(t *testing.T, call func(string, any, any), resourceID, economicAssetID, originalEventID string) {
	t.Helper()
	type args = map[string]any
	invoke := func(name string, input args) json.RawMessage {
		t.Helper()
		var result struct{ Data json.RawMessage }
		call(name, input, &result)
		if len(result.Data) == 0 || string(result.Data) == "null" {
			t.Fatalf("%s returned no data", name)
		}
		if _, write := input["request_key"]; write {
			var replay struct{ Data json.RawMessage }
			call(name, input, &replay)
			if !reflect.DeepEqual(result.Data, replay.Data) {
				t.Fatalf("%s replay changed result", name)
			}
		}
		return result.Data
	}
	id := func(data json.RawMessage) string {
		t.Helper()
		var value struct{ ID string }
		if json.Unmarshal(data, &value) != nil || value.ID == "" {
			t.Fatal("missing returned stable ID")
		}
		return value.ID
	}
	check := func(name string, input args, expected ...string) {
		t.Helper()
		result := string(invoke(name, input))
		for _, value := range expected {
			if !strings.Contains(result, value) {
				t.Fatalf("%s did not return expected persisted value %q: %s", name, value, result)
			}
		}
	}
	check("get_context", args{}, `"base_currency":"CNY"`)
	categoryID := id(invoke("create_category", args{"request_key": "walk-category", "name": "Walkthrough category", "icon_key": "camera"}))
	check("update_category", args{"request_key": "walk-category-edit", "id": categoryID, "name": "Walkthrough renamed", "icon_key": "camera"}, categoryID, "Walkthrough renamed")
	check("list_categories", args{}, categoryID, "Walkthrough renamed")
	modelID := id(invoke("create_product_model", args{"request_key": "walk-model", "category_id": categoryID, "name": "Walkthrough model"}))
	check("search_product_models", args{"query": "Walkthrough model", "category_id": categoryID}, modelID)
	check("get_product_model", args{"id": modelID}, modelID, "Walkthrough model")
	typeID := id(invoke("save_tag_type", args{"request_key": "walk-type", "name": "Walkthrough finish", "multiple": false, "affects_appearance": true, "enabled": true}))
	tagID := id(invoke("save_specification_tag", args{"request_key": "walk-tag", "type_id": typeID, "name": "Walkthrough black", "enabled": true}))
	check("list_tag_types", args{"query": "Walkthrough finish"}, typeID)
	check("search_specification_tags", args{"type_id": typeID}, tagID)
	check("save_model_configuration", args{"request_key": "walk-model-config", "model_id": modelID, "tag_ids": []string{tagID}, "appearance_overrides": args{}}, modelID)
	check("get_model_configuration", args{"id": modelID}, modelID, tagID)
	itemID := id(invoke("save_asset", args{"request_key": "walk-item", "model_id": modelID, "display_name": "Walkthrough item", "serial_number": "WALK-001", "purchase_channel": "test", "notes": "Disposable walkthrough", "tag_ids": []string{tagID}}))
	check("get_asset", args{"id": itemID}, itemID, "Walkthrough item", "WALK-001", tagID)
	check("save_asset", args{"request_key": "walk-clear-name", "id": itemID, "model_id": modelID, "serial_number": "WALK-001", "purchase_channel": "test", "notes": "Disposable walkthrough", "tag_ids": []string{tagID}}, itemID)
	check("get_asset", args{"id": itemID}, itemID, "WALK-001", tagID)
	check("list_assets", args{"query": "WALK-001"}, itemID)
	check("save_3d_resource_metadata", args{"request_key": "walk-resource", "resource_id": resourceID, "tag_ids": []string{tagID}, "category_ids": []string{categoryID}, "details": args{"name": "Walkthrough GLB", "source_url": "", "author": "Fixture", "license": "CC0"}}, resourceID)
	check("get_resource_configuration", args{"id": resourceID}, resourceID, tagID, categoryID)
	check("list_3d_resources", args{"query": "Walkthrough GLB"}, resourceID)
	check("get_3d_resource", args{"id": resourceID}, resourceID, "Fixture", "CC0")
	check("search_appearance_candidates", args{"model_id": modelID, "tag_ids": []string{tagID}}, resourceID)
	ruleID := id(invoke("save_appearance_default", args{"request_key": "walk-rule", "model_id": modelID, "resource_id": resourceID, "tag_ids": []string{tagID}}))
	check("get_model_configuration", args{"id": modelID}, ruleID)
	check("get_asset_appearance", args{"id": itemID}, resourceID)
	check("get_specification_references", args{"kind": "tag", "id": tagID}, modelID)
	check("bind_3d_resource", args{"request_key": "walk-bind", "kind": "asset", "target_id": itemID, "resource_id": resourceID}, "true")
	check("get_3d_binding", args{"kind": "asset", "target_id": itemID}, resourceID)
	check("get_3d_references", args{"id": resourceID}, itemID)
	eventTypeID := id(invoke("create_event_type", args{"request_key": "walk-event-type", "name": "Walkthrough inspection", "cashflow": "neutral"}))
	check("update_event_type", args{"request_key": "walk-event-type-edit", "id": eventTypeID, "name": "Walkthrough inspected", "cashflow": "neutral"}, eventTypeID, "Walkthrough inspected")
	check("set_event_type_enabled", args{"request_key": "walk-event-type-disable", "id": eventTypeID, "enabled": false}, eventTypeID, `"Enabled":false`)
	check("list_event_types", args{"query": "Walkthrough inspected", "status": "disabled"}, eventTypeID)
	check("get_event", args{"id": originalEventID}, originalEventID, `"IsVoided":true`, `"BaseAmountMinor":-712`)
	check("list_events", args{"asset_id": economicAssetID, "show_voided": true}, originalEventID)
	check("get_asset_cost", args{"id": economicAssetID}, "1424")
	check("get_portfolio_summary", args{}, "CNY")
	check("delete_appearance_default", args{"request_key": "walk-rule-delete", "id": ruleID}, ruleID)
	check("bind_3d_resource", args{"request_key": "walk-unbind", "kind": "asset", "target_id": itemID, "resource_id": ""}, "true")
	var binding struct {
		ResourceID  string `json:"resource_id"`
		EffectiveID string `json:"effective_resource_id"`
	}
	if json.Unmarshal(invoke("get_3d_binding", args{"kind": "asset", "target_id": itemID}), &binding) != nil || binding.ResourceID != "" || binding.EffectiveID != "" {
		t.Fatal("deleted rule/binding still effective")
	}
	check("delete_3d_resource", args{"request_key": "walk-resource-delete", "resource_id": resourceID}, "true")
	var resources struct {
		Total int `json:"total"`
	}
	if json.Unmarshal(invoke("list_3d_resources", args{"query": "Walkthrough GLB"}), &resources) != nil || resources.Total != 0 {
		t.Fatal("deleted resource still listed")
	}
}
