package application

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/SampsonFox/assetloop/internal/domain"
)

type SpecificationService struct {
	store SpecificationStore
	now   func() time.Time
}

func NewSpecificationService(store SpecificationStore) *SpecificationService {
	return &SpecificationService{store: store, now: time.Now}
}

type SaveSpecificationType struct {
	ID, Name                                                  string
	Multiple, AffectsAppearance, Enabled, ConfirmSharedRename bool
}
type SaveSpecificationTag struct {
	ID, TypeID, Name             string
	Enabled, ConfirmSharedRename bool
}
type SaveModelSpecification struct {
	ModelID             string
	TagIDs              []string
	AppearanceOverrides map[string]bool
}
type SaveSpecificationAsset struct {
	ID, ModelID, DisplayName, SerialNumber, PurchaseChannel, Notes string
	TagIDs                                                         []string
}
type SaveAppearanceDefault struct {
	ID, ModelID, ResourceID string
	TagIDs                  []string
}
type SaveResourceSpecification struct {
	ResourceID          string
	TagIDs, CategoryIDs []string
}
type SpecificationInUseError struct{ References []SpecificationLink }

func (e SpecificationInUseError) Error() string { return "validation.specification_referenced" }
func (e SpecificationInUseError) Unwrap() error {
	return NewInputError("validation.specification_referenced")
}

func (s *SpecificationService) Snapshot(ctx context.Context, actor Principal) (SpecificationSnapshot, error) {
	if err := actor.Require(CapabilityView); err != nil {
		return SpecificationSnapshot{}, err
	}
	return s.store.SpecificationSnapshot(ctx, actor.TenantID)
}

// InitialColorTagType is part of tenant bootstrap policy. The auth Store persists
// it in the same transaction as the tenant; no tag is selected on any item.
func InitialColorTagType(tenant Tenant, locale Locale) domain.SpecificationTagType {
	name := "颜色"
	if locale == LocaleEn {
		name = "Color"
	}
	return domain.SpecificationTagType{ID: newID(), TenantID: tenant.ID, SystemCode: "color", Name: name, NormalizedName: domain.NormalizeSpecificationName(name), Enabled: true, AffectsAppearance: true, CreatedAt: tenant.CreatedAt, UpdatedAt: tenant.CreatedAt}
}
func (s *SpecificationService) write(ctx context.Context, actor Principal, fn func(SpecificationStore, SpecificationSnapshot) error) error {
	if err := actor.Require(CapabilityManageCatalog); err != nil {
		return err
	}
	return s.store.WithSpecificationWrite(ctx, actor.TenantID, func(store SpecificationStore) error {
		state, err := store.SpecificationSnapshot(ctx, actor.TenantID)
		if err != nil {
			return err
		}
		return fn(store, state)
	})
}

func (s *SpecificationService) SaveType(ctx context.Context, actor Principal, cmd SaveSpecificationType) (domain.SpecificationTagType, error) {
	var result domain.SpecificationTagType
	err := s.write(ctx, actor, func(store SpecificationStore, state SpecificationSnapshot) error {
		name, err := catalogText("tag type name", cmd.Name, 120, true)
		if err != nil {
			return err
		}
		result = domain.SpecificationTagType{ID: cmd.ID, TenantID: actor.TenantID, Name: name, NormalizedName: domain.NormalizeSpecificationName(name), Multiple: cmd.Multiple, AffectsAppearance: cmd.AffectsAppearance, Enabled: cmd.Enabled, CreatedAt: s.now().UTC(), UpdatedAt: s.now().UTC()}
		if cmd.ID != "" {
			old, ok := state.Type(cmd.ID)
			if !ok {
				return NewInputError("validation.specification_missing")
			}
			result.CreatedAt, result.SystemCode = old.CreatedAt, old.SystemCode
			if old.Name != name && !cmd.ConfirmSharedRename && len(state.TypeReferences(cmd.ID)) > 0 {
				return NewInputError("validation.specification_rename_confirmation")
			}
		} else {
			result.ID = newID()
		}
		for i, kind := range state.Types {
			if kind.ID != result.ID && kind.NormalizedName == result.NormalizedName {
				return NewInputError("validation.specification_duplicate")
			}
			if kind.ID == result.ID {
				state.Types[i] = result
			}
		}
		if cmd.ID != "" {
			if err := state.ValidateExisting(); err != nil {
				return err
			}
		}
		return store.PutSpecificationType(ctx, result)
	})
	return result, err
}

func (s *SpecificationService) SaveTag(ctx context.Context, actor Principal, cmd SaveSpecificationTag) (domain.SpecificationTag, error) {
	var result domain.SpecificationTag
	err := s.write(ctx, actor, func(store SpecificationStore, state SpecificationSnapshot) error {
		name, err := catalogText("tag name", cmd.Name, 160, true)
		if err != nil {
			return err
		}
		kind, ok := state.Type(cmd.TypeID)
		if !ok {
			return NewInputError("validation.specification_missing")
		}
		result = domain.SpecificationTag{ID: cmd.ID, TenantID: actor.TenantID, TypeID: cmd.TypeID, Name: name, NormalizedName: domain.NormalizeSpecificationName(name), Enabled: cmd.Enabled, CreatedAt: s.now().UTC(), UpdatedAt: s.now().UTC()}
		if cmd.ID != "" {
			old, ok := state.Tag(cmd.ID)
			if !ok {
				return NewInputError("validation.specification_missing")
			}
			if old.TypeID != cmd.TypeID {
				return NewInputError("validation.specification_type_immutable")
			}
			result.CreatedAt = old.CreatedAt
			if old.Name != name && !cmd.ConfirmSharedRename && len(state.TagReferences(cmd.ID)) > 0 {
				return NewInputError("validation.specification_rename_confirmation")
			}
		} else {
			if !kind.Enabled {
				return NewInputError("validation.specification_disabled")
			}
			result.ID = newID()
		}
		for _, tag := range state.Tags {
			if tag.ID != result.ID && tag.TypeID == result.TypeID && tag.NormalizedName == result.NormalizedName {
				return NewInputError("validation.specification_duplicate")
			}
		}
		return store.PutSpecificationTag(ctx, result)
	})
	return result, err
}

func (s *SpecificationService) SaveModel(ctx context.Context, actor Principal, cmd SaveModelSpecification) error {
	return s.write(ctx, actor, func(store SpecificationStore, state SpecificationSnapshot) error {
		if err := validID("model ID", cmd.ModelID); err != nil {
			return err
		}
		if _, err := store.GetProductModel(ctx, actor.TenantID, cmd.ModelID); err != nil {
			return err
		}
		previous := state.Model(actor.TenantID, cmd.ModelID)
		old, selected := specIDSet(previous.AllowedTagIDs), specIDSet(cmd.TagIDs)
		for id := range selected {
			tag, ok := state.Tag(id)
			kind, known := state.Type(tag.TypeID)
			if !ok || !known {
				return NewInputError("validation.specification_missing")
			}
			if (!tag.Enabled || !kind.Enabled) && !old[id] {
				return NewInputError("validation.specification_disabled")
			}
		}
		var refs []SpecificationLink
		for _, link := range state.Links {
			if link.ModelID == cmd.ModelID && (link.Kind == "asset" || link.Kind == "appearance") && !selected[link.TagID] {
				refs = append(refs, link)
			}
		}
		if len(refs) > 0 {
			return SpecificationInUseError{References: refs}
		}
		for id := range cmd.AppearanceOverrides {
			found := false
			for tagID := range selected {
				tag, _ := state.Tag(tagID)
				if tag.TypeID == id {
					found = true
				}
			}
			if !found {
				return NewInputError("validation.specification_missing")
			}
		}
		next := domain.ModelSpecification{TenantID: actor.TenantID, ModelID: cmd.ModelID, AllowedTagIDs: uniqueSpecIDs(cmd.TagIDs), AppearanceOverrides: cmd.AppearanceOverrides}
		for _, rule := range state.Defaults {
			if rule.ModelID == cmd.ModelID {
				if _, err := domain.ValidateAppearanceCondition(next, state.Types, state.Tags, rule.TagIDs, rule.TagIDs); err != nil {
					return specificationInputError(err)
				}
			}
		}
		for id := range old {
			if !selected[id] {
				if err := store.RemoveModelAllowedTag(ctx, actor.TenantID, cmd.ModelID, id); err != nil {
					return err
				}
			}
		}
		for id := range selected {
			if !old[id] {
				if err := store.AddModelAllowedTag(ctx, actor.TenantID, cmd.ModelID, id); err != nil {
					return err
				}
			}
		}
		if err := store.ClearAppearanceDimensions(ctx, actor.TenantID, cmd.ModelID); err != nil {
			return err
		}
		for id, affects := range cmd.AppearanceOverrides {
			if err := store.PutAppearanceDimension(ctx, actor.TenantID, AppearanceDimension{ModelID: cmd.ModelID, TypeID: id, AffectsAppearance: affects}); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *SpecificationService) SaveAsset(ctx context.Context, actor Principal, cmd SaveSpecificationAsset) (domain.Asset, error) {
	var result domain.Asset
	err := s.write(ctx, actor, func(store SpecificationStore, state SpecificationSnapshot) error {
		if err := validID("model ID", cmd.ModelID); err != nil {
			return err
		}
		model, err := store.GetProductModel(ctx, actor.TenantID, cmd.ModelID)
		if err != nil {
			return err
		}
		var retained []string
		if cmd.ID != "" {
			if err := validID("asset ID", cmd.ID); err != nil {
				return err
			}
			result, err = store.GetAsset(ctx, actor.TenantID, cmd.ID)
			if err != nil {
				return err
			}
			retained = state.Selected("asset", cmd.ID)
		} else {
			result = domain.Asset{ID: newID(), TenantID: actor.TenantID, CreatedAt: s.now().UTC()}
		}
		ids, err := domain.ValidateSpecificationSelection(state.Model(actor.TenantID, cmd.ModelID), state.Types, state.Tags, cmd.TagIDs, retained)
		if err != nil {
			return specificationInputError(err)
		}
		result.ModelID, result.Model = cmd.ModelID, model.Name
		result.CategoryID, result.Category, result.CategoryIcon = model.CategoryID, model.CategoryName, model.CategoryIcon
		for _, field := range []struct {
			name, value string
			max         int
			required    bool
			target      *string
		}{
			{"display name", cmd.DisplayName, 200, true, &result.DisplayName}, {"serial number", cmd.SerialNumber, 200, false, &result.SerialNumber},
			{"purchase channel", cmd.PurchaseChannel, 160, false, &result.PurchaseChannel}, {"notes", cmd.Notes, 2000, false, &result.Notes},
		} {
			value, err := catalogText(field.name, field.value, field.max, field.required)
			if err != nil {
				return err
			}
			*field.target = value
		}
		if err := store.ClearAssetSpecificationTags(ctx, actor.TenantID, result.ID); err != nil {
			return err
		}
		if err := store.WriteSelectedAsset(ctx, result, cmd.ID == ""); err != nil {
			return err
		}
		for _, id := range ids {
			if err := store.AddAssetSpecificationTag(ctx, actor.TenantID, result.ID, result.ModelID, id); err != nil {
				return err
			}
		}
		state.HydrateSelection(&result, ids)
		return nil
	})
	return result, err
}

func (s *SpecificationService) SaveAppearance(ctx context.Context, actor Principal, cmd SaveAppearanceDefault) (domain.AppearanceDefault, error) {
	var result domain.AppearanceDefault
	err := s.write(ctx, actor, func(store SpecificationStore, state SpecificationSnapshot) error {
		var err error
		result, err = saveAppearanceInTransaction(ctx, store, state, actor, cmd)
		return err
	})
	return result, err
}

func saveAppearanceInTransaction(ctx context.Context, store SpecificationStore, state SpecificationSnapshot, actor Principal, cmd SaveAppearanceDefault) (domain.AppearanceDefault, error) {
	var result domain.AppearanceDefault
	err := func() error {
		if err := validID("model ID", cmd.ModelID); err != nil {
			return err
		}
		if _, err := store.GetProductModel(ctx, actor.TenantID, cmd.ModelID); err != nil {
			return err
		}
		if err := validID("resource ID", cmd.ResourceID); err != nil {
			return err
		}
		resource, err := store.GetModel3DResource(ctx, actor.TenantID, cmd.ResourceID)
		if err != nil {
			return err
		}
		if resource.Status != "ready" {
			return ErrModel3DUnavailable
		}
		var retained []string
		if cmd.ID != "" {
			found := false
			for _, rule := range state.Defaults {
				if rule.ID == cmd.ID && rule.ModelID == cmd.ModelID {
					found = true
					retained = rule.TagIDs
				}
			}
			if !found {
				return NewInputError("validation.specification_missing")
			}
		}
		ids, err := domain.ValidateAppearanceCondition(state.Model(actor.TenantID, cmd.ModelID), state.Types, state.Tags, cmd.TagIDs, retained)
		if err != nil {
			return specificationInputError(err)
		}
		for _, rule := range state.Defaults {
			if rule.ModelID == cmd.ModelID && rule.ID != cmd.ID && strings.Join(uniqueSpecIDs(rule.TagIDs), "/") == strings.Join(ids, "/") {
				return NewInputError("validation.specification_duplicate")
			}
		}
		result = domain.AppearanceDefault{ID: cmd.ID, TenantID: actor.TenantID, ModelID: cmd.ModelID, ResourceID: cmd.ResourceID, TagIDs: ids}
		if result.ID == "" {
			result.ID = newID()
		}
		if err := store.PutAppearanceDefault(ctx, result); err != nil {
			return err
		}
		if err := store.ClearAppearanceConditions(ctx, actor.TenantID, result.ID); err != nil {
			return err
		}
		for _, id := range ids {
			if err := store.AddAppearanceCondition(ctx, actor.TenantID, result.ID, result.ModelID, id); err != nil {
				return err
			}
		}
		return nil
	}()
	return result, err
}

func (s *SpecificationService) DeleteAppearance(ctx context.Context, actor Principal, id string) error {
	return s.write(ctx, actor, func(store SpecificationStore, state SpecificationSnapshot) error {
		for _, rule := range state.Defaults {
			if rule.ID == id {
				return store.DeleteAppearanceDefault(ctx, actor.TenantID, id)
			}
		}
		return NewInputError("validation.specification_missing")
	})
}

func (s *SpecificationService) ResolveLegacyMapping(ctx context.Context, actor Principal, modelID, variantID string) error {
	return s.write(ctx, actor, func(store SpecificationStore, state SpecificationSnapshot) error {
		for _, mapping := range state.LegacyMedia {
			if mapping.VariantID == variantID && mapping.ModelID == modelID {
				return store.ResolveLegacyMedia(ctx, actor.TenantID, variantID)
			}
		}
		return NewInputError("validation.specification_missing")
	})
}

func (s *SpecificationService) SaveResource(ctx context.Context, actor Principal, cmd SaveResourceSpecification) error {
	return s.write(ctx, actor, func(store SpecificationStore, state SpecificationSnapshot) error {
		if err := validID("resource ID", cmd.ResourceID); err != nil {
			return err
		}
		resource, err := store.GetModel3DResource(ctx, actor.TenantID, cmd.ResourceID)
		if err != nil {
			return err
		}
		if resource.Status != "ready" {
			return ErrModel3DUnavailable
		}
		all := []string{}
		for _, tag := range state.Tags {
			all = append(all, tag.ID)
		}
		ids, err := domain.ValidateSpecificationSelection(domain.ModelSpecification{TenantID: actor.TenantID, AllowedTagIDs: all}, state.Types, state.Tags, cmd.TagIDs, state.Selected("resource", cmd.ResourceID))
		if err != nil {
			return specificationInputError(err)
		}
		categories := specIDSet(state.CategoryIDs)
		for _, id := range cmd.CategoryIDs {
			if !categories[id] {
				return NewInputError("validation.specification_missing")
			}
		}
		if err := store.ClearResourceSpecificationTags(ctx, actor.TenantID, cmd.ResourceID); err != nil {
			return err
		}
		for _, id := range ids {
			if err := store.AddResourceSpecificationTag(ctx, actor.TenantID, cmd.ResourceID, id); err != nil {
				return err
			}
		}
		if err := store.ClearResourceCategories(ctx, actor.TenantID, cmd.ResourceID); err != nil {
			return err
		}
		for _, id := range uniqueSpecIDs(cmd.CategoryIDs) {
			if err := store.AddResourceCategory(ctx, actor.TenantID, cmd.ResourceID, id); err != nil {
				return err
			}
		}
		return nil
	})
}

func (state SpecificationSnapshot) Type(id string) (domain.SpecificationTagType, bool) {
	for _, value := range state.Types {
		if value.ID == id {
			return value, true
		}
	}
	return domain.SpecificationTagType{}, false
}
func (state SpecificationSnapshot) Tag(id string) (domain.SpecificationTag, bool) {
	for _, value := range state.Tags {
		if value.ID == id {
			return value, true
		}
	}
	return domain.SpecificationTag{}, false
}
func (state SpecificationSnapshot) Selected(kind, id string) []string {
	var ids []string
	for _, link := range state.Links {
		if link.Kind == kind && link.TargetID == id {
			ids = append(ids, link.TagID)
		}
	}
	return uniqueSpecIDs(ids)
}
func (state SpecificationSnapshot) Model(tenant, id string) domain.ModelSpecification {
	model := domain.ModelSpecification{TenantID: tenant, ModelID: id, AllowedTagIDs: state.Selected("model", id), AppearanceOverrides: map[string]bool{}}
	for _, dimension := range state.Dimensions {
		if dimension.ModelID == id {
			model.AppearanceOverrides[dimension.TypeID] = dimension.AffectsAppearance
		}
	}
	return model
}
func (state SpecificationSnapshot) TagReferences(id string) []SpecificationLink {
	var refs []SpecificationLink
	for _, link := range state.Links {
		if link.Kind != "resource-category" && link.TagID == id {
			refs = append(refs, link)
		}
	}
	return refs
}
func (state SpecificationSnapshot) TypeReferences(id string) []SpecificationLink {
	var refs []SpecificationLink
	for _, tag := range state.Tags {
		if tag.TypeID == id {
			refs = append(refs, state.TagReferences(tag.ID)...)
		}
	}
	return refs
}
func (state SpecificationSnapshot) ValidateExisting() error {
	groups := map[string][]string{}
	models := map[string]string{}
	for _, link := range state.Links {
		if link.Kind == "asset" || link.Kind == "resource" {
			key := link.Kind + "/" + link.TargetID
			groups[key] = append(groups[key], link.TagID)
			models[key] = link.ModelID
		}
	}
	all := []string{}
	tenant := ""
	for _, tag := range state.Tags {
		all = append(all, tag.ID)
		tenant = tag.TenantID
	}
	for key, ids := range groups {
		model := state.Model(tenant, models[key])
		if strings.HasPrefix(key, "resource/") {
			model.AllowedTagIDs = all
		}
		if _, err := domain.ValidateSpecificationSelection(model, state.Types, state.Tags, ids, ids); err != nil {
			return specificationInputError(err)
		}
	}
	for _, rule := range state.Defaults {
		if _, err := domain.ValidateAppearanceCondition(state.Model(rule.TenantID, rule.ModelID), state.Types, state.Tags, rule.TagIDs, rule.TagIDs); err != nil {
			return specificationInputError(err)
		}
	}
	return nil
}
func (state SpecificationSnapshot) HydrateSelection(asset *domain.Asset, ids []string) {
	asset.Tags = nil
	asset.Color = ""
	var labels []string
	selected := specIDSet(ids)
	for _, tag := range state.Tags {
		if selected[tag.ID] {
			asset.Tags = append(asset.Tags, tag)
			labels = append(labels, tag.Name)
			kind, _ := state.Type(tag.TypeID)
			if kind.SystemCode == "color" {
				if asset.Color != "" {
					asset.Color += " · "
				}
				asset.Color += tag.Name
			}
		}
	}
	asset.Variant = strings.Join(labels, " · ")
}
func specificationInputError(err error) error {
	switch {
	case errors.Is(err, domain.ErrSpecificationSingleChoice):
		return NewInputError("validation.specification_single")
	case errors.Is(err, domain.ErrSpecificationTagNotAllowed):
		return NewInputError("validation.specification_not_allowed")
	case errors.Is(err, domain.ErrSpecificationTagUnavailable):
		return NewInputError("validation.specification_disabled")
	default:
		return NewInputError("validation.specification_appearance")
	}
}
func specIDSet(ids []string) map[string]bool {
	result := map[string]bool{}
	for _, id := range ids {
		result[id] = true
	}
	return result
}
func uniqueSpecIDs(ids []string) []string {
	result := []string{}
	for id := range specIDSet(ids) {
		result = append(result, id)
	}
	sort.Strings(result)
	return result
}
