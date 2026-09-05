package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/SampsonFox/assetloop/internal/domain"
)

var ErrAlreadyVoided = errors.New("asset event is already voided")

type LifecycleService struct {
	store LifecycleStore
	now   func() time.Time
}

type RecordEvent struct {
	RequestKey        string
	AssetID           string
	Type              domain.AssetEventType
	AmountMinor       int64
	Currency          string
	FXRateScaled      int64
	FXRateDate        time.Time
	FXRateSource      string
	FXConfirmed       bool
	OccurredAt        time.Time
	Source            string
	ExternalReference string
	Notes             string
}

type CreateAssetEventType struct {
	Name     string
	Cashflow domain.AssetEventCashflow
}

func NewLifecycleService(store LifecycleStore) *LifecycleService {
	return &LifecycleService{store: store, now: time.Now}
}

func (s *LifecycleService) BaseCurrency(ctx context.Context, actor Principal) (string, bool, error) {
	if err := actor.Require(CapabilityView); err != nil {
		return "", false, err
	}
	currency, locked, err := s.store.TenantBaseCurrency(ctx, actor.TenantID)
	if err != nil {
		return "", false, fmt.Errorf("get base currency: %w", err)
	}
	return currency, locked, nil
}

func (s *LifecycleService) PortfolioSummary(ctx context.Context, actor Principal) (PortfolioSummary, error) {
	if err := actor.Require(CapabilityView); err != nil {
		return PortfolioSummary{}, err
	}
	summary, err := s.store.GetPortfolioSummary(ctx, actor.TenantID)
	if err != nil {
		return PortfolioSummary{}, fmt.Errorf("get portfolio summary: %w", err)
	}
	return summary, nil
}

func (s *LifecycleService) Record(ctx context.Context, actor Principal, cmd RecordEvent) (domain.AssetEvent, error) {
	if err := actor.Require(CapabilityManageLifecycle); err != nil {
		return domain.AssetEvent{}, err
	}
	return s.write(ctx, actor, "record", "", cmd, func(scoped *LifecycleService) (domain.AssetEvent, error) {
		return scoped.record(ctx, actor, cmd)
	})
}

func (s *LifecycleService) record(ctx context.Context, actor Principal, cmd RecordEvent) (domain.AssetEvent, error) {
	eventType, err := s.resolveEventType(ctx, actor.TenantID, cmd.Type)
	if err != nil {
		return domain.AssetEvent{}, err
	}
	if err := s.validateLifecycle(ctx, actor, cmd.AssetID, domain.AssetEventType(eventType.Name)); err != nil {
		return domain.AssetEvent{}, err
	}
	transaction, event, err := s.prepareEvent(ctx, actor, cmd, eventType, "")
	if err != nil {
		return domain.AssetEvent{}, err
	}
	if err := s.store.AppendAssetEvent(ctx, transaction, event); err != nil {
		return domain.AssetEvent{}, fmt.Errorf("append asset event: %w", err)
	}
	return event, nil
}

func (s *LifecycleService) Correct(ctx context.Context, actor Principal, eventID string, cmd RecordEvent) (domain.AssetEvent, error) {
	if err := actor.Require(CapabilityManageLifecycle); err != nil {
		return domain.AssetEvent{}, err
	}
	return s.write(ctx, actor, "correct", eventID, cmd, func(scoped *LifecycleService) (domain.AssetEvent, error) {
		return scoped.correct(ctx, actor, eventID, cmd)
	})
}

func (s *LifecycleService) correct(ctx context.Context, actor Principal, eventID string, cmd RecordEvent) (domain.AssetEvent, error) {
	if err := validID("event ID", eventID); err != nil {
		return domain.AssetEvent{}, err
	}
	original, err := s.store.GetAssetEvent(ctx, actor.TenantID, eventID)
	if err != nil {
		return domain.AssetEvent{}, fmt.Errorf("get original event: %w", err)
	}
	if original.IsVoided || original.Type == domain.AssetEventVoid {
		return domain.AssetEvent{}, ErrAlreadyVoided
	}
	cmd.AssetID = original.AssetID
	cmd.Type = original.Type
	eventType, err := s.resolveEventType(ctx, actor.TenantID, original.Type)
	if err != nil {
		return domain.AssetEvent{}, err
	}
	transaction, replacement, err := s.prepareEvent(ctx, actor, cmd, eventType, original.ID)
	if err != nil {
		return domain.AssetEvent{}, err
	}
	voidEvent := domain.AssetEvent{
		ID: newID(), TenantID: actor.TenantID, AssetID: original.AssetID,
		TransactionID: transaction.ID, Type: domain.AssetEventVoid,
		BaseCurrency: replacement.BaseCurrency, Notes: "作废并由更正记录替代",
		VoidsEventID: original.ID, OccurredAt: transaction.OccurredAt,
		CreatedByUserID: actor.UserID, CreatedAt: transaction.CreatedAt,
	}
	if err := s.store.CorrectAssetEvent(ctx, transaction, voidEvent, replacement); err != nil {
		return domain.AssetEvent{}, fmt.Errorf("correct asset event: %w", err)
	}
	return replacement, nil
}

func (s *LifecycleService) Timeline(ctx context.Context, actor Principal, assetID string) ([]domain.AssetEvent, domain.AssetSummary, error) {
	if err := actor.Require(CapabilityView); err != nil {
		return nil, domain.AssetSummary{}, err
	}
	if err := validID("asset ID", assetID); err != nil {
		return nil, domain.AssetSummary{}, err
	}
	if _, err := s.store.GetAsset(ctx, actor.TenantID, assetID); err != nil {
		return nil, domain.AssetSummary{}, fmt.Errorf("get asset: %w", err)
	}
	events, err := s.store.ListAssetEvents(ctx, actor.TenantID, assetID)
	if err != nil {
		return nil, domain.AssetSummary{}, fmt.Errorf("list asset events: %w", err)
	}
	baseCurrency, _, err := s.store.TenantBaseCurrency(ctx, actor.TenantID)
	if err != nil {
		return nil, domain.AssetSummary{}, fmt.Errorf("get base currency: %w", err)
	}
	summary := summarizeEvents(baseCurrency, events)
	return events, summary, nil
}

func (s *LifecycleService) TimelinePage(ctx context.Context, actor Principal, assetID string, opts EventListOptions) (EventListResult, error) {
	if err := actor.Require(CapabilityView); err != nil {
		return EventListResult{}, err
	}
	if err := validID("asset ID", assetID); err != nil {
		return EventListResult{}, err
	}
	opts.Page, opts.PageSize = normalizePage(opts.Page, opts.PageSize)
	opts.Query = strings.TrimSpace(opts.Query)
	opts.Type = strings.TrimSpace(opts.Type)
	if opts.Type == string(domain.AssetEventVoid) {
		return EventListResult{}, NewInputError("validation.filter_invalid")
	}
	if opts.Type != "" {
		eventType, err := s.resolveEventType(ctx, actor.TenantID, domain.AssetEventType(opts.Type))
		if err != nil {
			return EventListResult{}, NewInputError("validation.filter_invalid")
		}
		opts.Type = eventType.Name
	}
	var err error
	opts.Sort, opts.Direction, err = normalizeSort(opts.Sort, opts.Direction, "occurred", "asc", map[string]struct{}{"occurred": {}, "amount": {}, "type": {}})
	if err != nil {
		return EventListResult{}, err
	}
	events, total, err := s.store.ListAssetEventsPage(ctx, actor.TenantID, assetID, opts)
	if err != nil {
		return EventListResult{}, fmt.Errorf("list asset events page: %w", err)
	}
	summary, err := s.store.GetAssetSummary(ctx, actor.TenantID, assetID)
	if err != nil {
		return EventListResult{}, fmt.Errorf("get asset summary: %w", err)
	}
	return EventListResult{Events: events, Summary: summary, Total: total}, nil
}

func (s *LifecycleService) GetEvent(ctx context.Context, actor Principal, eventID string) (domain.AssetEvent, error) {
	if err := actor.Require(CapabilityView); err != nil {
		return domain.AssetEvent{}, err
	}
	if err := validID("event ID", eventID); err != nil {
		return domain.AssetEvent{}, err
	}
	event, err := s.store.GetAssetEvent(ctx, actor.TenantID, eventID)
	if err != nil {
		return domain.AssetEvent{}, fmt.Errorf("get asset event: %w", err)
	}
	return event, nil
}

func (s *LifecycleService) EventTypes(ctx context.Context, actor Principal) ([]domain.AssetEventTypeDefinition, error) {
	if err := actor.Require(CapabilityView); err != nil {
		return nil, err
	}
	custom, err := s.store.ListAssetEventTypes(ctx, actor.TenantID)
	if err != nil {
		return nil, fmt.Errorf("list asset event types: %w", err)
	}
	return append(builtInEventTypes(), custom...), nil
}

func (s *LifecycleService) CreateEventType(ctx context.Context, actor Principal, cmd CreateAssetEventType) (domain.AssetEventTypeDefinition, error) {
	if err := actor.Require(CapabilityManageLifecycle); err != nil {
		return domain.AssetEventTypeDefinition{}, err
	}
	name, err := catalogText("event type", cmd.Name, 80, true)
	if err != nil {
		return domain.AssetEventTypeDefinition{}, err
	}
	normalized := strings.ToLower(name)
	for _, builtIn := range builtInEventTypes() {
		if normalized == builtIn.NormalizedName {
			return domain.AssetEventTypeDefinition{}, NewInputError("validation.event_type_exists")
		}
	}
	if normalized == string(domain.AssetEventVoid) {
		return domain.AssetEventTypeDefinition{}, NewInputError("validation.event_type_exists")
	}
	existing, err := s.store.ListAssetEventTypes(ctx, actor.TenantID)
	if err != nil {
		return domain.AssetEventTypeDefinition{}, fmt.Errorf("list asset event types: %w", err)
	}
	for _, eventType := range existing {
		if normalized == eventType.NormalizedName {
			return domain.AssetEventTypeDefinition{}, NewInputError("validation.event_type_exists")
		}
	}
	if cmd.Cashflow != domain.AssetEventExpense && cmd.Cashflow != domain.AssetEventIncome && cmd.Cashflow != domain.AssetEventNeutral {
		return domain.AssetEventTypeDefinition{}, NewInputError("validation.event_cashflow")
	}
	eventType := domain.AssetEventTypeDefinition{
		ID: newID(), TenantID: actor.TenantID, Name: name, NormalizedName: normalized,
		Cashflow: cmd.Cashflow, CreatedByUserID: actor.UserID, CreatedAt: s.now().UTC(),
	}
	if err := s.store.CreateAssetEventType(ctx, eventType); err != nil {
		return domain.AssetEventTypeDefinition{}, fmt.Errorf("create asset event type: %w", err)
	}
	return eventType, nil
}

func (s *LifecycleService) prepareEvent(ctx context.Context, actor Principal, cmd RecordEvent, eventType domain.AssetEventTypeDefinition, replacesID string) (domain.AssetTransaction, domain.AssetEvent, error) {
	if err := validID("asset ID", cmd.AssetID); err != nil {
		return domain.AssetTransaction{}, domain.AssetEvent{}, err
	}
	if _, err := s.store.GetAsset(ctx, actor.TenantID, cmd.AssetID); err != nil {
		return domain.AssetTransaction{}, domain.AssetEvent{}, fmt.Errorf("get asset: %w", err)
	}
	if eventType.Cashflow != domain.AssetEventNeutral && cmd.AmountMinor <= 0 {
		return domain.AssetTransaction{}, domain.AssetEvent{}, NewInputError("validation.amount_positive")
	}
	if eventType.Cashflow == domain.AssetEventNeutral && cmd.AmountMinor != 0 {
		return domain.AssetTransaction{}, domain.AssetEvent{}, NewInputError("validation.amount_zero")
	}
	currency, err := domain.NormalizeSelectableCurrency(cmd.Currency)
	if err != nil {
		return domain.AssetTransaction{}, domain.AssetEvent{}, err
	}
	baseCurrency, _, err := s.store.TenantBaseCurrency(ctx, actor.TenantID)
	if err != nil {
		return domain.AssetTransaction{}, domain.AssetEvent{}, fmt.Errorf("get base currency: %w", err)
	}
	baseCurrency, err = domain.NormalizeCurrency(baseCurrency)
	if err != nil {
		return domain.AssetTransaction{}, domain.AssetEvent{}, err
	}
	baseAmount := cmd.AmountMinor
	var fx *domain.FXEvidence
	if eventType.Cashflow != domain.AssetEventNeutral && currency != baseCurrency {
		if !cmd.FXConfirmed {
			return domain.AssetTransaction{}, domain.AssetEvent{}, NewInputError("validation.fx_confirm")
		}
		if cmd.FXRateDate.IsZero() || strings.TrimSpace(cmd.FXRateSource) == "" {
			return domain.AssetTransaction{}, domain.AssetEvent{}, NewInputError("validation.fx_evidence")
		}
		baseAmount, err = domain.ConvertMinor(cmd.AmountMinor, currency, baseCurrency, cmd.FXRateScaled)
		if err != nil {
			return domain.AssetTransaction{}, domain.AssetEvent{}, err
		}
		fx = &domain.FXEvidence{
			OriginalAmountMinor: cmd.AmountMinor, OriginalCurrency: currency,
			RateScaled: cmd.FXRateScaled, RateDate: cmd.FXRateDate.UTC(), RateSource: strings.TrimSpace(cmd.FXRateSource),
		}
	}
	if eventType.Cashflow == domain.AssetEventExpense {
		baseAmount = -baseAmount
	}
	occurredAt, err := s.validOccurredAt(cmd.OccurredAt)
	if err != nil {
		return domain.AssetTransaction{}, domain.AssetEvent{}, err
	}
	createdAt := s.now().UTC()
	source := strings.TrimSpace(cmd.Source)
	if source == "" {
		source = "manual"
	}
	transaction := domain.AssetTransaction{
		ID: newID(), TenantID: actor.TenantID, OccurredAt: occurredAt, Source: source,
		ExternalReference: strings.TrimSpace(cmd.ExternalReference), Notes: strings.TrimSpace(cmd.Notes),
		CreatedByUserID: actor.UserID, CreatedAt: createdAt,
	}
	event := domain.AssetEvent{
		ID: newID(), TenantID: actor.TenantID, AssetID: cmd.AssetID, TransactionID: transaction.ID,
		Type: domain.AssetEventType(eventType.Name), BaseAmountMinor: baseAmount, BaseCurrency: baseCurrency, FX: fx,
		Notes: strings.TrimSpace(cmd.Notes), ReplacesEventID: replacesID, OccurredAt: occurredAt,
		CreatedByUserID: actor.UserID, CreatedAt: createdAt,
	}
	return transaction, event, nil
}

func (s *LifecycleService) validateLifecycle(ctx context.Context, actor Principal, assetID string, eventType domain.AssetEventType) error {
	if err := validID("asset ID", assetID); err != nil {
		return err
	}
	events, err := s.store.ListAssetEvents(ctx, actor.TenantID, assetID)
	if err != nil {
		return fmt.Errorf("list asset events: %w", err)
	}
	hasPurchase, sold := false, false
	for _, event := range events {
		if event.IsVoided || event.Type == domain.AssetEventVoid {
			continue
		}
		if event.Type == domain.AssetEventPurchase {
			hasPurchase = true
		}
		if event.Type == domain.AssetEventSale {
			sold = true
		}
	}
	switch eventType {
	case domain.AssetEventPurchase:
		if hasPurchase {
			return NewInputError("validation.event_purchase_exists")
		}
	case domain.AssetEventRepair, domain.AssetEventSale:
		if !hasPurchase {
			return NewInputError("validation.event_purchase_first")
		}
		if sold {
			return NewInputError("validation.event_after_sale")
		}
	}
	return nil
}

func (s *LifecycleService) validOccurredAt(value time.Time) (time.Time, error) {
	if value.IsZero() {
		return time.Time{}, NewInputError("validation.occurred_required")
	}
	value = value.UTC()
	if value.After(s.now().UTC().Add(5 * time.Minute)) {
		return time.Time{}, NewInputError("validation.occurred_future")
	}
	return value, nil
}

func (s *LifecycleService) resolveEventType(ctx context.Context, tenantID string, value domain.AssetEventType) (domain.AssetEventTypeDefinition, error) {
	normalized := strings.ToLower(strings.TrimSpace(string(value)))
	for _, eventType := range builtInEventTypes() {
		if normalized == eventType.NormalizedName {
			return eventType, nil
		}
	}
	if normalized == "" || normalized == string(domain.AssetEventVoid) {
		return domain.AssetEventTypeDefinition{}, NewInputError("validation.event_type")
	}
	eventTypes, err := s.store.ListAssetEventTypes(ctx, tenantID)
	if err != nil {
		return domain.AssetEventTypeDefinition{}, fmt.Errorf("list asset event types: %w", err)
	}
	for _, eventType := range eventTypes {
		if normalized == eventType.NormalizedName {
			return eventType, nil
		}
	}
	return domain.AssetEventTypeDefinition{}, NewInputError("validation.event_type")
}

func builtInEventTypes() []domain.AssetEventTypeDefinition {
	return []domain.AssetEventTypeDefinition{
		{Name: string(domain.AssetEventPurchase), NormalizedName: string(domain.AssetEventPurchase), Cashflow: domain.AssetEventExpense, BuiltIn: true},
		{Name: string(domain.AssetEventRepair), NormalizedName: string(domain.AssetEventRepair), Cashflow: domain.AssetEventExpense, BuiltIn: true},
		{Name: string(domain.AssetEventSale), NormalizedName: string(domain.AssetEventSale), Cashflow: domain.AssetEventIncome, BuiltIn: true},
	}
}

func summarizeEvents(baseCurrency string, events []domain.AssetEvent) domain.AssetSummary {
	summary := domain.AssetSummary{BaseCurrency: baseCurrency, Status: "unacquired"}
	activePurchase, sold := false, false
	for _, event := range events {
		if event.IsVoided || event.Type == domain.AssetEventVoid {
			continue
		}
		if event.BaseAmountMinor < 0 {
			summary.ExpenseMinor += -event.BaseAmountMinor
		} else {
			summary.IncomeMinor += event.BaseAmountMinor
		}
		summary.NetCashflowMinor += event.BaseAmountMinor
		if event.Type == domain.AssetEventPurchase {
			activePurchase = true
		}
		if event.Type == domain.AssetEventSale {
			sold = true
		}
	}
	if activePurchase {
		summary.Status = "active"
	}
	if sold {
		summary.Status = "sold"
	}
	return summary
}
