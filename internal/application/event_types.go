package application

import (
	"context"
	"github.com/SampsonFox/assetloop/internal/domain"
	"strings"
)

type EventTypeListOptions struct {
	Query, Status  string
	Page, PageSize int
}
type EventTypeListResult struct {
	Types []domain.AssetEventTypeDefinition
	Total int
}
type UpdateEventType struct {
	Name     string
	Cashflow domain.AssetEventCashflow
}

func (s *LifecycleService) EventTypePage(ctx context.Context, actor Principal, opts EventTypeListOptions) (EventTypeListResult, error) {
	if err := actor.Require(CapabilityView); err != nil {
		return EventTypeListResult{}, err
	}
	opts.Page, opts.PageSize = normalizePage(opts.Page, opts.PageSize)
	opts.Query = strings.TrimSpace(opts.Query)
	if opts.Status != "" && opts.Status != "enabled" && opts.Status != "disabled" {
		return EventTypeListResult{}, NewInputError("validation.filter_invalid")
	}
	items, total, err := s.store.ListAssetEventTypesPage(ctx, actor.TenantID, opts)
	return EventTypeListResult{Types: items, Total: total}, err
}

func (s *LifecycleService) UpdateEventType(ctx context.Context, actor Principal, id string, cmd UpdateEventType) (domain.AssetEventTypeDefinition, error) {
	return s.mutateEventType(ctx, actor, id, func(item *domain.AssetEventTypeDefinition, store LifecycleStore) error {
		name, err := catalogText("event type", cmd.Name, 80, true)
		if err != nil {
			return err
		}
		if cmd.Cashflow != domain.AssetEventExpense && cmd.Cashflow != domain.AssetEventIncome && cmd.Cashflow != domain.AssetEventNeutral {
			return NewInputError("validation.event_cashflow")
		}
		if item.ReferenceCount > 0 && cmd.Cashflow != item.Cashflow {
			return NewInputError("validation.event_type_in_use")
		}
		types, err := store.ListAssetEventTypes(ctx, actor.TenantID)
		if err != nil {
			return err
		}
		for _, other := range types {
			if other.ID != id && other.NormalizedName == strings.ToLower(name) {
				return NewInputError("validation.event_type_exists")
			}
		}
		item.Name = name
		item.NormalizedName = strings.ToLower(name)
		item.Cashflow = cmd.Cashflow
		return nil
	})
}

func (s *LifecycleService) SetEventTypeEnabled(ctx context.Context, actor Principal, id string, enabled bool) (domain.AssetEventTypeDefinition, error) {
	return s.mutateEventType(ctx, actor, id, func(item *domain.AssetEventTypeDefinition, _ LifecycleStore) error {
		item.Enabled = enabled
		return nil
	})
}

func (s *LifecycleService) mutateEventType(ctx context.Context, actor Principal, id string, change func(*domain.AssetEventTypeDefinition, LifecycleStore) error) (domain.AssetEventTypeDefinition, error) {
	if err := actor.Require(CapabilityManageCatalog); err != nil {
		return domain.AssetEventTypeDefinition{}, err
	}
	if err := validID("event type ID", id); err != nil {
		return domain.AssetEventTypeDefinition{}, err
	}
	var result domain.AssetEventTypeDefinition
	_, err := s.store.WithLifecycleWrite(ctx, actor.TenantID, func(store LifecycleStore) (domain.AssetEvent, error) {
		item, err := scopedEventType(ctx, store, actor.TenantID, id)
		if err != nil {
			return domain.AssetEvent{}, err
		}
		if item.BuiltIn {
			return domain.AssetEvent{}, NewInputError("validation.event_type_builtin")
		}
		if err := change(&item, store); err != nil {
			return domain.AssetEvent{}, err
		}
		item.UpdatedAt = s.now().UTC()
		if err := store.UpdateAssetEventType(ctx, item); err != nil {
			return domain.AssetEvent{}, err
		}
		result = item
		return domain.AssetEvent{}, nil
	})
	return result, err
}
