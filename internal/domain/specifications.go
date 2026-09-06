package domain

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Specification labels describe products; they never replace product or asset IDs.
type SpecificationTagType struct {
	ID, TenantID, Name, NormalizedName   string
	Multiple, AffectsAppearance, Enabled bool
	CreatedAt, UpdatedAt                 time.Time
}

type SpecificationTag struct {
	ID, TenantID, TypeID, Name, NormalizedName string
	Enabled                                    bool
	CreatedAt, UpdatedAt                       time.Time
}

type ModelSpecification struct {
	TenantID, ModelID string
	AllowedTagIDs     []string
	// An absent entry uses the tag type's default. False is an explicit override.
	AppearanceOverrides map[string]bool
}

type AppearanceDefault struct {
	ID, TenantID, ModelID, ResourceID string
	TagIDs                            []string
}

type AppearanceMatch struct {
	ResourceID string
	RuleIDs    []string
	Conflict   bool
}

var (
	ErrSpecificationTagUnavailable = errors.New("specification tag unavailable")
	ErrSpecificationTagNotAllowed  = errors.New("specification tag not allowed by model")
	ErrSpecificationSingleChoice   = errors.New("multiple tags selected for a single-choice type")
	ErrAppearanceCondition         = errors.New("appearance condition must contain allowed appearance tags")
)

func NormalizeSpecificationName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

// ValidateSpecificationSelection validates only data rules, not manufacturer SKUs.
// Retained IDs must come from the existing asset loaded inside the write transaction,
// never from a client-supplied list. They allow unchanged disabled choices to survive.
func ValidateSpecificationSelection(model ModelSpecification, types []SpecificationTagType, tags []SpecificationTag, selected, retained []string) ([]string, error) {
	typeByID := make(map[string]SpecificationTagType, len(types))
	for _, kind := range types {
		if kind.TenantID == model.TenantID {
			typeByID[kind.ID] = kind
		}
	}
	tagByID := make(map[string]SpecificationTag, len(tags))
	for _, tag := range tags {
		if tag.TenantID == model.TenantID {
			tagByID[tag.ID] = tag
		}
	}
	allowed, previous := specificationSet(model.AllowedTagIDs), specificationSet(retained)
	result := make([]string, 0, len(selected))
	seen, chosenTypes := map[string]bool{}, map[string]bool{}
	for _, id := range selected {
		if seen[id] {
			continue
		}
		seen[id] = true
		tag, found := tagByID[id]
		kind, knownType := typeByID[tag.TypeID]
		if !found || !knownType || ((!tag.Enabled || !kind.Enabled) && !previous[id]) {
			return nil, fmt.Errorf("%w: %s", ErrSpecificationTagUnavailable, id)
		}
		if !allowed[id] {
			return nil, fmt.Errorf("%w: %s", ErrSpecificationTagNotAllowed, id)
		}
		if !kind.Multiple && chosenTypes[kind.ID] {
			return nil, fmt.Errorf("%w: %s", ErrSpecificationSingleChoice, kind.ID)
		}
		chosenTypes[kind.ID] = true
		result = append(result, id)
	}
	sort.Strings(result)
	return result, nil
}

func ValidateAppearanceCondition(model ModelSpecification, types []SpecificationTagType, tags []SpecificationTag, selected, retained []string) ([]string, error) {
	ids, err := ValidateSpecificationSelection(model, types, tags, selected, retained)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, ErrAppearanceCondition
	}
	appearanceTypes := map[string]bool{}
	for _, kind := range types {
		if kind.TenantID != model.TenantID {
			continue
		}
		affects := kind.AffectsAppearance
		if override, exists := model.AppearanceOverrides[kind.ID]; exists {
			affects = override
		}
		appearanceTypes[kind.ID] = affects
	}
	appearanceTags := map[string]bool{}
	for _, tag := range tags {
		if tag.TenantID == model.TenantID {
			appearanceTags[tag.ID] = appearanceTypes[tag.TypeID]
		}
	}
	for _, id := range ids {
		if !appearanceTags[id] {
			return nil, fmt.Errorf("%w: %s", ErrAppearanceCondition, id)
		}
	}
	return ids, nil
}

// MatchAppearanceDefaults resolves confirmed rules only, never resource search
// results. The caller handles asset overrides, model fallback, and unavailable blobs.
func MatchAppearanceDefaults(tenantID, modelID string, selected []string, rules []AppearanceDefault) AppearanceMatch {
	chosen := specificationSet(selected)
	best := 0
	resources := map[string]bool{}
	result := AppearanceMatch{}
	for _, rule := range rules {
		if rule.TenantID != tenantID || rule.ModelID != modelID || rule.ResourceID == "" {
			continue
		}
		conditions := specificationSet(rule.TagIDs)
		if len(conditions) == 0 || len(conditions) < best {
			continue
		}
		matches := true
		for id := range conditions {
			if !chosen[id] {
				matches = false
				break
			}
		}
		if !matches {
			continue
		}
		if len(conditions) > best {
			best = len(conditions)
			resources = map[string]bool{}
			result.RuleIDs = nil
		}
		resources[rule.ResourceID] = true
		result.RuleIDs = append(result.RuleIDs, rule.ID)
	}
	sort.Strings(result.RuleIDs)
	result.Conflict = len(resources) > 1
	if len(resources) == 1 {
		for id := range resources {
			result.ResourceID = id
		}
	}
	return result
}

func specificationSet(ids []string) map[string]bool {
	set := make(map[string]bool, len(ids))
	for _, id := range ids {
		set[id] = true
	}
	return set
}
