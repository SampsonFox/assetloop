package application

import (
	"context"
	"sort"
	"strings"

	"github.com/SampsonFox/assetloop/internal/domain"
)

type SpecificationListOptions struct {
	Query, Status, TypeID string
	Page, PageSize        int
}
type SpecificationTypeSummary struct {
	domain.SpecificationTagType
	ReferenceCount int
}
type SpecificationTagSummary struct {
	domain.SpecificationTag
	TypeName       string
	ReferenceCount int
}
type SpecificationTypeList struct {
	Types []SpecificationTypeSummary
	Total int
}
type SpecificationTagList struct {
	Tags  []SpecificationTagSummary
	Total int
}
type EffectiveAppearance struct {
	Resource *domain.Model3DResource
	Source   string
	Conflict bool
	RuleIDs  []string
}
type AppearanceCandidate struct {
	Resource            domain.Model3DResource
	MatchingTags        int
	DescriptionComplete bool
}
type AppearanceCandidateList struct {
	Candidates []AppearanceCandidate
	Total      int
}

func (s *SpecificationService) Model(ctx context.Context, actor Principal, id string) (domain.ProductModel, error) {
	if err := actor.Require(CapabilityView); err != nil {
		return domain.ProductModel{}, err
	}
	if err := validID("model ID", id); err != nil {
		return domain.ProductModel{}, err
	}
	return s.store.GetProductModel(ctx, actor.TenantID, id)
}

func (s *SpecificationService) Asset(ctx context.Context, actor Principal, id string) (domain.Asset, error) {
	if err := actor.Require(CapabilityView); err != nil {
		return domain.Asset{}, err
	}
	if err := validID("asset ID", id); err != nil {
		return domain.Asset{}, err
	}
	asset, err := s.store.GetAsset(ctx, actor.TenantID, id)
	if err != nil {
		return domain.Asset{}, err
	}
	items, err := s.DescribeAssets(ctx, actor, []domain.Asset{asset})
	if err != nil {
		return domain.Asset{}, err
	}
	return items[0], nil
}

// DescribeAssets decorates an already authorized result page in one batch, never
// deriving current descriptions from retained legacy specification columns.
func (s *SpecificationService) DescribeAssets(ctx context.Context, actor Principal, assets []domain.Asset) ([]domain.Asset, error) {
	state, err := s.Snapshot(ctx, actor)
	if err != nil {
		return nil, err
	}
	result := append([]domain.Asset(nil), assets...)
	for i := range result {
		if result[i].TenantID != actor.TenantID {
			return nil, ErrForbidden
		}
		state.HydrateSelection(&result[i], state.Selected("asset", result[i].ID))
	}
	return result, nil
}

func (s *SpecificationService) ListTypes(ctx context.Context, actor Principal, opts SpecificationListOptions) (SpecificationTypeList, error) {
	state, err := s.Snapshot(ctx, actor)
	if err != nil {
		return SpecificationTypeList{}, err
	}
	if !validSpecificationStatus(opts.Status) {
		return SpecificationTypeList{}, NewInputError("validation.filter_invalid")
	}
	query := domain.NormalizeSpecificationName(opts.Query)
	var items []SpecificationTypeSummary
	for _, kind := range state.Types {
		if specificationStatusMatches(opts.Status, kind.Enabled) && strings.Contains(kind.NormalizedName, query) {
			items = append(items, SpecificationTypeSummary{SpecificationTagType: kind, ReferenceCount: len(state.TypeReferences(kind.ID))})
		}
	}
	return SpecificationTypeList{Types: specificationPage(items, opts.Page, opts.PageSize), Total: len(items)}, nil
}
func (s *SpecificationService) ListTags(ctx context.Context, actor Principal, opts SpecificationListOptions) (SpecificationTagList, error) {
	state, err := s.Snapshot(ctx, actor)
	if err != nil {
		return SpecificationTagList{}, err
	}
	if !validSpecificationStatus(opts.Status) {
		return SpecificationTagList{}, NewInputError("validation.filter_invalid")
	}
	query := domain.NormalizeSpecificationName(opts.Query)
	var items []SpecificationTagSummary
	for _, tag := range state.Tags {
		kind, _ := state.Type(tag.TypeID)
		if (opts.TypeID == "" || tag.TypeID == opts.TypeID) && specificationStatusMatches(opts.Status, tag.Enabled) && strings.Contains(tag.NormalizedName+" "+kind.NormalizedName, query) {
			items = append(items, SpecificationTagSummary{SpecificationTag: tag, TypeName: kind.Name, ReferenceCount: len(state.TagReferences(tag.ID))})
		}
	}
	return SpecificationTagList{Tags: specificationPage(items, opts.Page, opts.PageSize), Total: len(items)}, nil
}

func (s *SpecificationService) EffectiveForAsset(ctx context.Context, actor Principal, id string) (EffectiveAppearance, error) {
	return effectiveAppearance(ctx, s.store, actor, id)
}

func effectiveAppearance(ctx context.Context, store AppearanceStore, actor Principal, id string) (EffectiveAppearance, error) {
	result := EffectiveAppearance{}
	if err := actor.Require(CapabilityView); err != nil {
		return result, err
	}
	if err := validID("asset ID", id); err != nil {
		return result, err
	}
	asset, err := store.GetAsset(ctx, actor.TenantID, id)
	if err != nil {
		return result, err
	}
	resourceID := asset.Model3DResourceID
	if resourceID != "" {
		result.Source = "asset"
	} else {
		state, err := store.SpecificationSnapshot(ctx, actor.TenantID)
		if err != nil {
			return result, err
		}
		match := domain.MatchAppearanceDefaults(actor.TenantID, asset.ModelID, state.Selected("asset", asset.ID), state.Defaults)
		result.Conflict, result.RuleIDs = match.Conflict, match.RuleIDs
		resourceID = match.ResourceID
		if resourceID != "" {
			result.Source = "appearance"
		} else {
			model, err := store.GetProductModel(ctx, actor.TenantID, asset.ModelID)
			if err != nil {
				return result, err
			}
			resourceID = model.Model3DResourceID
			if resourceID != "" {
				result.Source = "model"
			}
		}
	}
	if resourceID == "" {
		return result, nil
	}
	resource, err := store.GetModel3DResource(ctx, actor.TenantID, resourceID)
	if err != nil {
		return result, err
	}
	if resource.Status != "ready" {
		return result, ErrModel3DUnavailable
	}
	result.Resource = &resource
	return result, nil
}

// Candidates are descriptions for user confirmation, never inferred bindings.
func (s *SpecificationService) Candidates(ctx context.Context, actor Principal, modelID string, selected []string, opts SpecificationListOptions) (AppearanceCandidateList, error) {
	state, err := s.Snapshot(ctx, actor)
	if err != nil {
		return AppearanceCandidateList{}, err
	}
	if err := validID("model ID", modelID); err != nil {
		return AppearanceCandidateList{}, err
	}
	model, err := s.store.GetProductModel(ctx, actor.TenantID, modelID)
	if err != nil {
		return AppearanceCandidateList{}, err
	}
	definition := state.Model(actor.TenantID, modelID)
	ids, err := domain.ValidateSpecificationSelection(definition, state.Types, state.Tags, selected, selected)
	if err != nil {
		return AppearanceCandidateList{}, specificationInputError(err)
	}
	wanted := map[string]map[string]bool{}
	for _, id := range ids {
		tag, _ := state.Tag(id)
		kind, _ := state.Type(tag.TypeID)
		affects := kind.AffectsAppearance
		if override, ok := definition.AppearanceOverrides[kind.ID]; ok {
			affects = override
		}
		if affects {
			if wanted[kind.ID] == nil {
				wanted[kind.ID] = map[string]bool{}
			}
			wanted[kind.ID][id] = true
		}
	}
	var candidates []AppearanceCandidate
	for page := 1; ; page++ {
		resources, err := s.store.ListModel3DResources(ctx, actor.TenantID, Model3DResourceListOptions{Query: strings.TrimSpace(opts.Query), Page: page, PageSize: 200})
		if err != nil {
			return AppearanceCandidateList{}, err
		}
		for _, resource := range resources.Resources {
			if resource.Status != "ready" || !specIDSet(state.Selected("resource-category", resource.ID))[model.CategoryID] {
				continue
			}
			described := map[string]map[string]bool{}
			for _, id := range state.Selected("resource", resource.ID) {
				tag, _ := state.Tag(id)
				if described[tag.TypeID] == nil {
					described[tag.TypeID] = map[string]bool{}
				}
				described[tag.TypeID][id] = true
			}
			matched, total, conflict := 0, 0, false
			for kind, values := range wanted {
				hits := 0
				for id := range values {
					total++
					if described[kind][id] {
						hits++
						matched++
					}
				}
				if len(described[kind]) > 0 && hits == 0 {
					conflict = true
				}
			}
			if !conflict {
				candidates = append(candidates, AppearanceCandidate{Resource: resource, MatchingTags: matched, DescriptionComplete: total > 0 && matched == total})
			}
		}
		if len(resources.Resources) == 0 || page*200 >= resources.Total {
			break
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		a, b := candidates[i], candidates[j]
		if a.MatchingTags != b.MatchingTags {
			return a.MatchingTags > b.MatchingTags
		}
		if a.Resource.Name != b.Resource.Name {
			return a.Resource.Name < b.Resource.Name
		}
		return a.Resource.ID < b.Resource.ID
	})
	return AppearanceCandidateList{Candidates: specificationPage(candidates, opts.Page, opts.PageSize), Total: len(candidates)}, nil
}

func validSpecificationStatus(value string) bool {
	return value == "" || value == "all" || value == "enabled" || value == "disabled"
}
func specificationStatusMatches(status string, enabled bool) bool {
	return status == "" || status == "all" || (status == "enabled" && enabled) || (status == "disabled" && !enabled)
}
func specificationPage[T any](items []T, page, size int) []T {
	page, size = normalizePage(page, size)
	if page > len(items)/size+1 {
		return nil
	}
	start := (page - 1) * size
	if start >= len(items) {
		return nil
	}
	end := start + size
	if end > len(items) {
		end = len(items)
	}
	return items[start:end]
}
