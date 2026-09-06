package domain

import (
	"errors"
	"reflect"
	"testing"
)

func specificationFixture() (ModelSpecification, []SpecificationTagType, []SpecificationTag) {
	model := ModelSpecification{TenantID: "space", ModelID: "phone", AllowedTagIDs: []string{"orange", "silver", "256", "512", "retired", "sim", "esim"}}
	types := []SpecificationTagType{
		{ID: "color", TenantID: "space", Enabled: true, AffectsAppearance: true},
		{ID: "storage", TenantID: "space", Enabled: true},
		{ID: "connectivity", TenantID: "space", Enabled: true, Multiple: true},
	}
	tags := []SpecificationTag{
		{ID: "orange", TenantID: "space", TypeID: "color", Enabled: true},
		{ID: "silver", TenantID: "space", TypeID: "color", Enabled: true},
		{ID: "256", TenantID: "space", TypeID: "storage", Enabled: true},
		{ID: "512", TenantID: "space", TypeID: "storage", Enabled: true},
		{ID: "retired", TenantID: "space", TypeID: "color"},
		{ID: "sim", TenantID: "space", TypeID: "connectivity", Enabled: true},
		{ID: "esim", TenantID: "space", TypeID: "connectivity", Enabled: true},
		{ID: "foreign", TenantID: "other", TypeID: "color", Enabled: true},
		{ID: "unlisted", TenantID: "space", TypeID: "color", Enabled: true},
	}
	return model, types, tags
}

func TestSpecificationSelection(t *testing.T) {
	for _, tc := range []struct {
		name               string
		selected, retained []string
		want               error
	}{
		{"all optional", nil, nil, nil},
		{"independent choices", []string{"orange", "512"}, nil, nil},
		{"multiple dimension", []string{"sim", "esim"}, nil, nil},
		{"deduplicated", []string{"orange", "orange"}, nil, nil},
		{"single dimension", []string{"orange", "silver"}, nil, ErrSpecificationSingleChoice},
		{"unknown", []string{"missing"}, nil, ErrSpecificationTagUnavailable},
		{"foreign", []string{"foreign"}, nil, ErrSpecificationTagUnavailable},
		{"outside model", []string{"unlisted"}, nil, ErrSpecificationTagNotAllowed},
		{"disabled new", []string{"retired"}, nil, ErrSpecificationTagUnavailable},
		{"disabled retained", []string{"retired"}, []string{"retired"}, nil},
		{"retained must remain allowed", []string{"unlisted"}, []string{"unlisted"}, ErrSpecificationTagNotAllowed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			model, types, tags := specificationFixture()
			_, err := ValidateSpecificationSelection(model, types, tags, tc.selected, tc.retained)
			if !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
		})
	}
}

func TestSpecificationDisabledTypeAndCanonicalIDs(t *testing.T) {
	model, types, tags := specificationFixture()
	ids, err := ValidateSpecificationSelection(model, types, tags, []string{"orange", "256", "orange"}, nil)
	if err != nil || !reflect.DeepEqual(ids, []string{"256", "orange"}) {
		t.Fatalf("unstable IDs: %v %v", ids, err)
	}
	types[0].Enabled = false
	if _, err := ValidateSpecificationSelection(model, types, tags, []string{"orange"}, nil); !errors.Is(err, ErrSpecificationTagUnavailable) {
		t.Fatalf("disabled type allowed new selection: %v", err)
	}
	if _, err := ValidateSpecificationSelection(model, types, tags, []string{"orange"}, []string{"orange"}); err != nil {
		t.Fatalf("disabled type lost retained selection: %v", err)
	}
	if NormalizeSpecificationName("  Δ Display  ") != "δ display" {
		t.Fatal("name normalization must include Unicode case folding")
	}
}

func TestAppearanceConditionUsesModelOverride(t *testing.T) {
	model, types, tags := specificationFixture()
	if _, err := ValidateAppearanceCondition(model, types, tags, nil, nil); !errors.Is(err, ErrAppearanceCondition) {
		t.Fatal("empty condition is not a generic model default")
	}
	if _, err := ValidateAppearanceCondition(model, types, tags, []string{"orange"}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateAppearanceCondition(model, types, tags, []string{"256"}, nil); !errors.Is(err, ErrAppearanceCondition) {
		t.Fatal("non-appearance dimension must not become a condition")
	}
	model.AppearanceOverrides = map[string]bool{"color": false, "storage": true}
	if _, err := ValidateAppearanceCondition(model, types, tags, []string{"orange"}, nil); !errors.Is(err, ErrAppearanceCondition) {
		t.Fatal("explicit false must override the default")
	}
	if _, err := ValidateAppearanceCondition(model, types, tags, []string{"256"}, nil); err != nil {
		t.Fatal(err)
	}
}

func TestAppearanceDefaultsSpecificityAndConflict(t *testing.T) {
	rules := []AppearanceDefault{
		{ID: "orange", TenantID: "space", ModelID: "phone", ResourceID: "body", TagIDs: []string{"orange"}},
		{ID: "engraved", TenantID: "space", ModelID: "phone", ResourceID: "engraved", TagIDs: []string{"orange", "engraving"}},
		{ID: "other product", TenantID: "space", ModelID: "laptop", ResourceID: "wrong", TagIDs: []string{"orange"}},
		{ID: "other space", TenantID: "other", ModelID: "phone", ResourceID: "wrong", TagIDs: []string{"orange"}},
		{ID: "empty", TenantID: "space", ModelID: "phone", ResourceID: "wrong"},
	}
	for _, selected := range [][]string{{"orange", "256"}, {"orange", "512"}} {
		got := MatchAppearanceDefaults("space", "phone", selected, rules)
		if got.ResourceID != "body" || got.Conflict {
			t.Fatalf("capacity changed appearance: %+v", got)
		}
	}
	got := MatchAppearanceDefaults("space", "phone", []string{"256"}, rules)
	if got.ResourceID != "" || got.Conflict {
		t.Fatalf("missing color selected a body: %+v", got)
	}
	got = MatchAppearanceDefaults("space", "phone", []string{"orange", "engraving"}, rules)
	if got.ResourceID != "engraved" || got.Conflict {
		t.Fatalf("specific rule failed: %+v", got)
	}
	rules = append(rules, AppearanceDefault{ID: "duplicate same resource", TenantID: "space", ModelID: "phone", ResourceID: "body", TagIDs: []string{"orange", "orange"}})
	got = MatchAppearanceDefaults("space", "phone", []string{"orange"}, rules)
	if got.ResourceID != "body" || got.Conflict {
		t.Fatalf("same resource incorrectly conflicted: %+v", got)
	}
	rules = append(rules, AppearanceDefault{ID: "conflicting", TenantID: "space", ModelID: "phone", ResourceID: "different", TagIDs: []string{"orange"}})
	got = MatchAppearanceDefaults("space", "phone", []string{"orange"}, rules)
	if got.ResourceID != "" || !got.Conflict || len(got.RuleIDs) != 3 {
		t.Fatalf("ambiguous rules picked an arbitrary resource: %+v", got)
	}
	got = MatchAppearanceDefaults("space", "phone", []string{"orange", "engraving"}, rules)
	if got.ResourceID != "engraved" || got.Conflict {
		t.Fatalf("less-specific conflicts blocked unique specific rule: %+v", got)
	}
}
