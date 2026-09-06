package application

import (
	"reflect"
	"testing"
)

func TestLegacySelectionUsesOneInputTranslation(t *testing.T) {
	state := SpecificationSnapshot{Links: []SpecificationLink{
		{Kind: "legacy", TargetID: "old-spec", ModelID: "phone", TagID: "orange"},
		{Kind: "legacy", TargetID: "old-spec", ModelID: "phone", TagID: "old-description"},
		{Kind: "legacy", TargetID: "other-spec", ModelID: "other-phone", TagID: "black"},
	}}
	got, err := state.resolveAssetSelection(SaveSpecificationAsset{VariantID: "old-spec", DisplayName: "Keep alias"})
	if err != nil || got.ModelID != "phone" || got.VariantID != "" || got.DisplayName != "Keep alias" || !reflect.DeepEqual(got.TagIDs, []string{"orange", "old-description"}) {
		t.Fatalf("translated selection: %+v, %v", got, err)
	}
	for _, cmd := range []SaveSpecificationAsset{
		{VariantID: "old-spec", ModelID: "phone"},
		{VariantID: "old-spec", TagIDs: []string{}},
		{VariantID: "missing"},
	} {
		if _, err := state.resolveAssetSelection(cmd); err == nil {
			t.Fatalf("ambiguous or missing legacy selection accepted: %+v", cmd)
		}
	}
	direct := SaveSpecificationAsset{ModelID: "phone", TagIDs: []string{}, DisplayName: "Optional tags"}
	got, err = state.resolveAssetSelection(direct)
	if err != nil || !reflect.DeepEqual(got, direct) {
		t.Fatalf("direct selection changed: %+v, %v", got, err)
	}
}
