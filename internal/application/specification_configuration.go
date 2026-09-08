package application

import (
	"context"

	"github.com/SampsonFox/assetloop/internal/domain"
)

type ModelSpecificationConfiguration struct {
	Specification domain.ModelSpecification
	Defaults      []domain.AppearanceDefault
}
type ResourceSpecificationConfiguration struct {
	TagIDs, CategoryIDs []string
}
type SpecificationReferenceList struct {
	References []SpecificationLink
	Total      int
}

func (s *SpecificationService) ModelConfiguration(ctx context.Context, actor Principal, id string) (ModelSpecificationConfiguration, error) {
	if _, err := s.Model(ctx, actor, id); err != nil {
		return ModelSpecificationConfiguration{}, err
	}
	state, err := s.Snapshot(ctx, actor)
	if err != nil {
		return ModelSpecificationConfiguration{}, err
	}
	result := ModelSpecificationConfiguration{Specification: state.Model(actor.TenantID, id), Defaults: []domain.AppearanceDefault{}}
	for _, rule := range state.Defaults {
		if rule.ModelID == id {
			result.Defaults = append(result.Defaults, rule)
		}
	}
	return result, nil
}

func (s *SpecificationService) ResourceConfiguration(ctx context.Context, actor Principal, id string) (ResourceSpecificationConfiguration, error) {
	if err := actor.Require(CapabilityView); err != nil {
		return ResourceSpecificationConfiguration{}, err
	}
	if err := validID("resource ID", id); err != nil {
		return ResourceSpecificationConfiguration{}, err
	}
	if _, err := s.store.GetModel3DResource(ctx, actor.TenantID, id); err != nil {
		return ResourceSpecificationConfiguration{}, err
	}
	state, err := s.Snapshot(ctx, actor)
	if err != nil {
		return ResourceSpecificationConfiguration{}, err
	}
	return ResourceSpecificationConfiguration{TagIDs: state.Selected("resource", id), CategoryIDs: state.Selected("resource-category", id)}, nil
}

func (s *SpecificationService) References(ctx context.Context, actor Principal, kind, id string, opts SpecificationListOptions) (SpecificationReferenceList, error) {
	state, err := s.Snapshot(ctx, actor)
	if err != nil {
		return SpecificationReferenceList{}, err
	}
	if err := validID("specification ID", id); err != nil {
		return SpecificationReferenceList{}, err
	}
	var refs []SpecificationLink
	var found bool
	switch kind {
	case "tag":
		_, found = state.Tag(id)
		refs = state.TagReferences(id)
	case "type":
		_, found = state.Type(id)
		refs = state.TypeReferences(id)
	default:
		return SpecificationReferenceList{}, NewInputError("validation.filter_invalid")
	}
	if !found {
		return SpecificationReferenceList{}, NewInputError("validation.specification_missing")
	}
	return SpecificationReferenceList{References: specificationPage(refs, opts.Page, opts.PageSize), Total: len(refs)}, nil
}
