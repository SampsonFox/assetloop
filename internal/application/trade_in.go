package application

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/SampsonFox/assetloop/internal/domain"
)

// maxTradeInCounterparts bounds one command so a single confirmed trade-in stays
// reviewable. Distinct counterparts are required; the order is normalized before
// fingerprinting so an idempotent retry is independent of the submitted order.
const maxTradeInCounterparts = 50

// EconomicSelection is one side of a trade-in pair. Exactly one of
// ExistingEventID (reuse an already recorded effective purchase/sale) or
// NewEvent (append fully specified fields) may be present. A selection with
// neither is a preview-only gap and is refused by the write path.
type EconomicSelection struct {
	AssetID         string
	ExistingEventID string
	NewEvent        *RecordEvent
}

// TradeInCommand records one asset against many counterparts, or many assets
// against one counterpart, as explicit selected pairs only.
//
// Direction is the public contract that names which side the CURRENT asset
// plays; it is never inferred from event order or display names:
//   - TradeInDirectionSource: the CURRENT asset is the NEW asset. It carries the
//     trade_in_source pairing event ("换新来源") and its economic side is the
//     purchase. Every counterpart is an OLD asset carrying the sale.
//   - TradeInDirectionDestination: the CURRENT asset is the OLD asset. It carries
//     the trade_in_destination pairing event ("换新去向") and its economic side is
//     the sale. Every counterpart is a NEW asset carrying the purchase.
//
// Current and every counterpart selection always describe their own economic
// side, so a source command's Current selection is a purchase-side selection and
// a destination command's Current selection is a sale-side selection.
type TradeInCommand struct {
	RequestKey        string
	CurrentAssetID    string
	Direction         domain.TradeInDirection
	Current           EconomicSelection
	Counterparts      []EconomicSelection
	OccurredAt        time.Time
	Source            string
	ExternalReference string
	Notes             string
}

// CorrectTradeInLinkCommand voids the current pair under its stable link ID and
// appends a replacement pair. Both target assets must already hold an effective
// purchase (new side) and sale (old side); a correction never creates money.
type CorrectTradeInLinkCommand struct {
	RequestKey                 string
	LinkID                     string
	ExpectedSourceEventID      string
	ExpectedDestinationEventID string
	NewAssetID                 string
	OldAssetID                 string
	OccurredAt                 time.Time
	ExternalReference          string
	Notes                      string
}

// CancelTradeInLinkCommand voids the current pair and appends cancelled
// replacements: hidden from the default effective view, visible with history.
type CancelTradeInLinkCommand struct {
	RequestKey                 string
	LinkID                     string
	ExpectedSourceEventID      string
	ExpectedDestinationEventID string
	OccurredAt                 time.Time
	Notes                      string
}

// Selection and status projections returned to transports. They are stable
// string codes, never localized text.
const (
	TradeInSelectionLinked  = "linked"
	TradeInSelectionReuse   = "reuse"
	TradeInSelectionCreate  = "create"
	TradeInSelectionMissing = "missing"

	TradeInStatusCreated   = "created"
	TradeInStatusReused    = "reused"
	TradeInStatusUnchanged = "unchanged"

	TradeInLinkStatusCorrected = "corrected"
	TradeInLinkStatusCancelled = "cancelled"
)

type TradeInPreviewPair struct {
	NewAssetID    string
	NewAssetName  string
	NewAssetSpec  string
	OldAssetID    string
	OldAssetName  string
	OldAssetSpec  string
	LinkID        string
	NewEventID    string
	OldEventID    string
	NewSelection  string
	OldSelection  string
	MissingFields []string
}

type TradeInPreview struct {
	Direction      domain.TradeInDirection
	CurrentAssetID string
	Ready          bool
	Pairs          []TradeInPreviewPair
	MissingFields  []string
}

type TradeInPairResult struct {
	LinkID             string
	LinkStatus         string
	SourceEventID      string
	DestinationEventID string
	NewAssetID         string
	OldAssetID         string
	NewAssetName       string
	OldAssetName       string
	NewEconomicEventID string
	NewEconomicStatus  string
	OldEconomicEventID string
	OldEconomicStatus  string
}

type TradeInResult struct {
	Direction      domain.TradeInDirection
	CurrentAssetID string
	Pairs          []TradeInPairResult
}

// TradeInService owns the trade-in policy. Persistence is the append-only event
// stream: the stable link ID and the state live on the two paired events, so
// there is no second mutable workflow store.
type TradeInService struct {
	store TradeInStore
	now   func() time.Time
}

func NewTradeInService(store TradeInStore) *TradeInService {
	return &TradeInService{store: store, now: time.Now}
}

func (s *TradeInService) lifecycle() *LifecycleService {
	return &LifecycleService{store: s.store, now: s.now}
}

type tradeInPair struct {
	newAsset domain.Asset
	oldAsset domain.Asset
	newSel   EconomicSelection
	oldSel   EconomicSelection
}

// Preview resolves the effective purchase/sale of every pair and reports the
// fields a write still needs. It performs reads only and requires no catalog
// capability, so a viewer can review a trade-in before an editor records it.
func (s *TradeInService) Preview(ctx context.Context, actor Principal, cmd TradeInCommand) (TradeInPreview, error) {
	if err := actor.Require(CapabilityView); err != nil {
		return TradeInPreview{}, err
	}
	cmd = normalizeTradeInCommand(cmd)
	pairs, err := s.planPairs(ctx, actor, cmd)
	if err != nil {
		return TradeInPreview{}, err
	}
	baseCurrency, err := s.baseCurrency(ctx, actor.TenantID)
	if err != nil {
		return TradeInPreview{}, err
	}
	// A fully specified new side is validated by the same lifecycle policy that
	// will write it, so an invalid rate, conversion or date can never preview as
	// ready. The pairing types are resolved once for the whole preview.
	types, err := s.systemTypes(ctx, actor.TenantID)
	if err != nil {
		return TradeInPreview{}, err
	}
	preview := TradeInPreview{Direction: cmd.Direction, CurrentAssetID: strings.TrimSpace(cmd.CurrentAssetID)}
	if cmd.OccurredAt.IsZero() {
		preview.MissingFields = append(preview.MissingFields, "occurred_at")
	} else if _, err := s.lifecycle().validOccurredAt(cmd.OccurredAt); err != nil {
		return TradeInPreview{}, err
	}
	for _, pair := range pairs {
		projection := TradeInPreviewPair{
			NewAssetID: pair.newAsset.ID, NewAssetName: pair.newAsset.DisplayName, NewAssetSpec: pair.newAsset.Model,
			OldAssetID: pair.oldAsset.ID, OldAssetName: pair.oldAsset.DisplayName, OldAssetSpec: pair.oldAsset.Model,
		}
		link, found, err := s.activeLink(ctx, actor.TenantID, pair.newAsset.ID, pair.oldAsset.ID)
		if err != nil {
			return TradeInPreview{}, err
		}
		if found {
			projection.LinkID = link.LinkID
			projection.NewSelection, projection.OldSelection = TradeInSelectionLinked, TradeInSelectionLinked
			if event, found, err := s.effectiveEvent(ctx, actor.TenantID, pair.newAsset.ID, domain.AssetEventPurchase); err != nil {
				return TradeInPreview{}, err
			} else if found {
				projection.NewEventID = event.ID
			}
			if event, found, err := s.effectiveEvent(ctx, actor.TenantID, pair.oldAsset.ID, domain.AssetEventSale); err != nil {
				return TradeInPreview{}, err
			} else if found {
				projection.OldEventID = event.ID
			}
			preview.Pairs = append(preview.Pairs, projection)
			continue
		}
		if projection.NewSelection, projection.NewEventID, projection.MissingFields, err =
			s.previewSide(ctx, actor, pair.newAsset.ID, pair.newSel, domain.AssetEventPurchase, types.purchase, baseCurrency); err != nil {
			return TradeInPreview{}, err
		}
		oldSelection, oldEventID, oldMissing, err := s.previewSide(ctx, actor, pair.oldAsset.ID, pair.oldSel, domain.AssetEventSale, types.sale, baseCurrency)
		if err != nil {
			return TradeInPreview{}, err
		}
		projection.OldSelection, projection.OldEventID = oldSelection, oldEventID
		projection.MissingFields = append(projection.MissingFields, oldMissing...)
		preview.Pairs = append(preview.Pairs, projection)
	}
	preview.Ready = len(preview.Pairs) > 0 && len(preview.MissingFields) == 0
	for _, pair := range preview.Pairs {
		if len(pair.MissingFields) != 0 {
			preview.Ready = false
		}
	}
	return preview, nil
}

// previewSide resolves one side of a pair without writing. An explicit stale
// event ID is refused immediately; a fully specified new event reports the
// fields the write path would still reject and surfaces a lifecycle blocker (for
// example a sale with no acquisition) as an actionable error instead of a ready
// preview. Once every field is present the shared lifecycle validation decides
// whether the record is actually valid, so an unconvertible amount, a negative
// FX rate or a future occurrence is refused rather than reported as ready.
func (s *TradeInService) previewSide(ctx context.Context, actor Principal, assetID string, selection EconomicSelection,
	kind domain.AssetEventType, eventType domain.AssetEventTypeDefinition, baseCurrency string) (string, string, []string, error) {
	if selection.ExistingEventID != "" {
		event, err := s.store.GetAssetEvent(ctx, actor.TenantID, strings.TrimSpace(selection.ExistingEventID))
		if errors.Is(err, sql.ErrNoRows) {
			return "", "", nil, NewInputError("validation.trade_in_event_stale")
		}
		if err != nil {
			return "", "", nil, fmt.Errorf("get existing economic event: %w", err)
		}
		if event.IsVoided || event.Kind() != kind || event.AssetID != assetID {
			return "", "", nil, NewInputError("validation.trade_in_event_stale")
		}
		return TradeInSelectionReuse, event.ID, nil, nil
	}
	if selection.NewEvent != nil {
		// The write path refuses a new selection while valid money exists, so the
		// preview must refuse it too instead of promising a second record.
		if event, found, err := s.effectiveEvent(ctx, actor.TenantID, assetID, kind); err != nil {
			return "", "", nil, err
		} else if found {
			return "", "", nil, NewInputError("validation.trade_in_event_exists", event.ID)
		}
		// A missing acquisition or an already sold asset can never accept this
		// side through the shared lifecycle policy, so report the real blocker.
		if err := s.lifecycle().validateLifecycle(ctx, actor, assetID, kind); err != nil {
			return "", "", nil, err
		}
		command := *selection.NewEvent
		missing := s.missingNewEventFields(command, baseCurrency)
		if len(missing) == 0 {
			command.AssetID = assetID
			if _, _, err := s.lifecycle().prepareEvent(ctx, actor, command, eventType, ""); err != nil {
				return "", "", nil, err
			}
		}
		return TradeInSelectionCreate, "", missing, nil
	}
	event, found, err := s.effectiveEvent(ctx, actor.TenantID, assetID, kind)
	if err != nil {
		return "", "", nil, err
	}
	if found {
		return TradeInSelectionReuse, event.ID, nil, nil
	}
	return TradeInSelectionMissing, "", []string{string(kind)}, nil
}

// missingNewEventFields mirrors the amount, date, currency and FX requirements of
// the shared lifecycle policy without converting or persisting anything.
func (s *TradeInService) missingNewEventFields(cmd RecordEvent, baseCurrency string) []string {
	missing := []string{}
	if _, err := normalizeRecordAmount(cmd); err != nil {
		return []string{"amount"}
	}
	if cmd.OccurredAt.IsZero() {
		missing = append(missing, "occurred_at")
	}
	if cmd.AmountMinor == 0 {
		missing = append(missing, "amount")
	}
	currency, err := domain.NormalizeSelectableCurrency(cmd.Currency)
	if err != nil {
		return append(missing, "currency")
	}
	if currency == baseCurrency {
		return missing
	}
	if !cmd.FXConfirmed || cmd.FXRateScaled == 0 || cmd.FXRateDate.IsZero() || strings.TrimSpace(cmd.FXRateSource) == "" {
		missing = append(missing, "fx")
	}
	return missing
}

// planPairs validates the command shape and loads every referenced asset under
// the caller's tenant, then normalizes the pair order deterministically.
func (s *TradeInService) planPairs(ctx context.Context, actor Principal, cmd TradeInCommand) ([]tradeInPair, error) {
	if err := validID("asset ID", cmd.CurrentAssetID); err != nil {
		return nil, err
	}
	switch cmd.Direction {
	case domain.TradeInDirectionSource, domain.TradeInDirectionDestination:
	default:
		return nil, NewInputError("validation.trade_in_direction")
	}
	if len(cmd.Counterparts) == 0 {
		return nil, NewInputError("validation.trade_in_counterparts")
	}
	if len(cmd.Counterparts) > maxTradeInCounterparts {
		return nil, NewInputError("validation.trade_in_counterparts", maxTradeInCounterparts)
	}
	current := cmd.Current
	if current.AssetID != "" && strings.TrimSpace(current.AssetID) != strings.TrimSpace(cmd.CurrentAssetID) {
		return nil, NewInputError("validation.trade_in_current_asset")
	}
	current.AssetID = strings.TrimSpace(cmd.CurrentAssetID)
	if err := validateEconomicSelection(current, true); err != nil {
		return nil, err
	}
	counterparts := make([]EconomicSelection, 0, len(cmd.Counterparts))
	seen := map[string]struct{}{current.AssetID: {}}
	for _, counterpart := range cmd.Counterparts {
		selection := counterpart
		selection.AssetID = strings.TrimSpace(selection.AssetID)
		if selection.AssetID == "" {
			return nil, NewInputError("validation.trade_in_counterpart_asset")
		}
		if err := validID("asset ID", selection.AssetID); err != nil {
			return nil, err
		}
		if _, duplicate := seen[selection.AssetID]; duplicate {
			return nil, NewInputError("validation.trade_in_counterpart_duplicate")
		}
		seen[selection.AssetID] = struct{}{}
		if err := validateEconomicSelection(selection, true); err != nil {
			return nil, err
		}
		counterparts = append(counterparts, selection)
	}
	currentAsset, err := s.requireAsset(ctx, actor, current.AssetID)
	if err != nil {
		return nil, err
	}
	pairs := make([]tradeInPair, 0, len(counterparts))
	for _, selection := range counterparts {
		asset, err := s.requireAsset(ctx, actor, selection.AssetID)
		if err != nil {
			return nil, err
		}
		if cmd.Direction == domain.TradeInDirectionSource {
			// The current asset is the trade-in source (换新来源): it is the NEW
			// asset carrying the purchase, and every counterpart is an OLD asset.
			pairs = append(pairs, tradeInPair{newAsset: currentAsset, oldAsset: asset, newSel: current, oldSel: selection})
			continue
		}
		// The current asset is the trade-in destination (换新去向): it is the OLD
		// asset carrying the sale, and every counterpart is a NEW asset.
		pairs = append(pairs, tradeInPair{newAsset: asset, oldAsset: currentAsset, newSel: selection, oldSel: current})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].oldAsset.ID != pairs[j].oldAsset.ID {
			return pairs[i].oldAsset.ID < pairs[j].oldAsset.ID
		}
		return pairs[i].newAsset.ID < pairs[j].newAsset.ID
	})
	return pairs, nil
}

func validateEconomicSelection(selection EconomicSelection, allowMissing bool) error {
	if selection.ExistingEventID != "" && selection.NewEvent != nil {
		return NewInputError("validation.trade_in_selection_conflict")
	}
	if selection.ExistingEventID == "" && selection.NewEvent == nil {
		if allowMissing {
			return nil
		}
		return NewInputError("validation.trade_in_selection_required")
	}
	if selection.NewEvent != nil {
		assetID := strings.TrimSpace(selection.NewEvent.AssetID)
		if assetID != "" && assetID != selection.AssetID {
			return NewInputError("validation.trade_in_selection_conflict")
		}
	}
	return nil
}

// normalizeTradeInCommand applies the ordinary lifecycle normalization to every
// selection before the command is fingerprinted, so an idempotent retry is
// independent of the submitted counterpart order, whitespace and timestamp zone.
// It never mutates the caller's slices or RecordEvent pointers: each selection
// and nested event is copied before it is adjusted.
func normalizeTradeInCommand(cmd TradeInCommand) TradeInCommand {
	cmd.CurrentAssetID = strings.TrimSpace(cmd.CurrentAssetID)
	cmd.Source = strings.TrimSpace(cmd.Source)
	cmd.ExternalReference = strings.TrimSpace(cmd.ExternalReference)
	cmd.Notes = strings.TrimSpace(cmd.Notes)
	if !cmd.OccurredAt.IsZero() {
		cmd.OccurredAt = cmd.OccurredAt.UTC()
	}
	cmd.Current = normalizeEconomicSelection(cmd.Current)
	counterparts := make([]EconomicSelection, len(cmd.Counterparts))
	for index, selection := range cmd.Counterparts {
		counterparts[index] = normalizeEconomicSelection(selection)
	}
	sort.SliceStable(counterparts, func(i, j int) bool { return counterparts[i].AssetID < counterparts[j].AssetID })
	cmd.Counterparts = counterparts
	return cmd
}

func normalizeEconomicSelection(selection EconomicSelection) EconomicSelection {
	selection.AssetID = strings.TrimSpace(selection.AssetID)
	selection.ExistingEventID = strings.TrimSpace(selection.ExistingEventID)
	if selection.NewEvent == nil {
		return selection
	}
	command := *selection.NewEvent
	if normalized, err := normalizeRecordAmount(command); err == nil {
		command = normalized
	}
	command.AssetID = strings.TrimSpace(command.AssetID)
	command.TypeID = strings.TrimSpace(command.TypeID)
	command.Type = domain.AssetEventType(strings.TrimSpace(string(command.Type)))
	command.Source = strings.TrimSpace(command.Source)
	command.ExternalReference = strings.TrimSpace(command.ExternalReference)
	command.Notes = strings.TrimSpace(command.Notes)
	command.FXRateSource = strings.TrimSpace(command.FXRateSource)
	if !command.OccurredAt.IsZero() {
		command.OccurredAt = command.OccurredAt.UTC()
	}
	if !command.FXRateDate.IsZero() {
		command.FXRateDate = command.FXRateDate.UTC()
	}
	if currency, err := domain.NormalizeSelectableCurrency(command.Currency); err == nil {
		command.Currency = currency
	} else {
		command.Currency = strings.TrimSpace(command.Currency)
	}
	if command.RelatedAssetID != nil {
		trimmed := strings.TrimSpace(*command.RelatedAssetID)
		command.RelatedAssetID = &trimmed
	}
	selection.NewEvent = &command
	return selection
}

func (s *TradeInService) requireAsset(ctx context.Context, actor Principal, assetID string) (domain.Asset, error) {
	asset, err := s.store.GetAsset(ctx, actor.TenantID, assetID)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Asset{}, NewInputError("validation.trade_in_asset_unavailable")
	}
	if err != nil {
		return domain.Asset{}, fmt.Errorf("get trade-in asset: %w", err)
	}
	return asset, nil
}

func (s *TradeInService) baseCurrency(ctx context.Context, tenantID string) (string, error) {
	baseCurrency, _, err := s.store.TenantBaseCurrency(ctx, tenantID)
	if err != nil {
		return "", fmt.Errorf("get base currency: %w", err)
	}
	baseCurrency, err = domain.NormalizeCurrency(baseCurrency)
	if err != nil {
		return "", err
	}
	return baseCurrency, nil
}

// effectiveEvent finds the one current purchase or sale of an asset. Voided
// originals and unrelated custom types are never part of a trade-in pair.
func (s *TradeInService) effectiveEvent(ctx context.Context, tenantID, assetID string, kind domain.AssetEventType) (domain.AssetEvent, bool, error) {
	events, err := s.store.ListAssetEvents(ctx, tenantID, assetID)
	if err != nil {
		return domain.AssetEvent{}, false, fmt.Errorf("list asset events: %w", err)
	}
	for _, event := range events {
		if event.IsVoided || event.Kind() != kind {
			continue
		}
		return event, true, nil
	}
	return domain.AssetEvent{}, false, nil
}

// activeLink resolves the effective relation between one new and one old asset.
func (s *TradeInService) activeLink(ctx context.Context, tenantID, newAssetID, oldAssetID string) (TradeInLinkView, bool, error) {
	links, err := s.store.TradeInLinks(ctx, tenantID, newAssetID, false)
	if err != nil {
		return TradeInLinkView{}, false, fmt.Errorf("list trade-in links: %w", err)
	}
	for _, link := range links {
		if link.OldAssetID == oldAssetID && link.NewAssetID == newAssetID {
			return link, true, nil
		}
	}
	return TradeInLinkView{}, false, nil
}

// systemEventType resolves an immutable system code. Custom types are never
// recognized by display name.
func systemEventType(ctx context.Context, store LifecycleStore, tenantID string, code domain.AssetEventType) (domain.AssetEventTypeDefinition, error) {
	items, err := store.ListAssetEventTypes(ctx, tenantID)
	if err != nil {
		return domain.AssetEventTypeDefinition{}, fmt.Errorf("list asset event types: %w", err)
	}
	for _, item := range items {
		if item.SystemCode == code {
			return item, nil
		}
	}
	return domain.AssetEventTypeDefinition{}, NewInputError("validation.event_type")
}

// tradeInSystemTypes resolves the four built-in economic types and both neutral
// pairing types once per command.
type tradeInSystemTypes struct {
	purchase    domain.AssetEventTypeDefinition
	sale        domain.AssetEventTypeDefinition
	void        domain.AssetEventTypeDefinition
	source      domain.AssetEventTypeDefinition
	destination domain.AssetEventTypeDefinition
}

func (s *TradeInService) systemTypes(ctx context.Context, tenantID string) (tradeInSystemTypes, error) {
	codes := tradeInSystemTypes{}
	targets := []struct {
		code domain.AssetEventType
		into *domain.AssetEventTypeDefinition
	}{
		{domain.AssetEventPurchase, &codes.purchase},
		{domain.AssetEventSale, &codes.sale},
		{domain.AssetEventVoid, &codes.void},
		{domain.AssetEventTradeInSource, &codes.source},
		{domain.AssetEventTradeInDestination, &codes.destination},
	}
	for _, target := range targets {
		item, err := systemEventType(ctx, s.store, tenantID, target.code)
		if err != nil {
			return tradeInSystemTypes{}, err
		}
		*target.into = item
	}
	return codes, nil
}

// pairedEvent builds one side of a neutral relation. Amounts stay exactly zero
// and the base currency is only carried so existing cost projections can read it.
// counterpartName and counterpartSpec are the resolved write-time snapshot of the
// other endpoint, so the relation stays readable after that asset is purged.
func pairedEvent(actor Principal, transaction domain.AssetTransaction, eventType domain.AssetEventTypeDefinition, asset, counterpart domain.Asset,
	counterpartName, counterpartSpec, linkID string, state domain.TradeInState, baseCurrency string, occurredAt time.Time, notes string) domain.AssetEvent {
	return domain.AssetEvent{
		ID: newID(), TenantID: actor.TenantID, AssetID: asset.ID, TransactionID: transaction.ID,
		Type: domain.AssetEventType(eventType.Name), TypeID: eventType.ID, SystemType: eventType.SystemCode,
		BaseAmountMinor: 0, BaseCurrency: baseCurrency, Notes: notes,
		RelatedAssetID: counterpart.ID, RelatedAssetName: counterpartName, RelatedAssetSpec: counterpartSpec,
		TradeInLinkID: linkID, TradeInState: state,
		OccurredAt: occurredAt, CreatedByUserID: actor.UserID, CreatedAt: transaction.CreatedAt,
		// Projected from the grouping transaction on reads; set here so an
		// immediate result matches a re-read.
		Source: transaction.Source, ExternalReference: transaction.ExternalReference,
	}
}

func voidPairedEvent(actor Principal, transaction domain.AssetTransaction, eventType domain.AssetEventTypeDefinition, original domain.AssetEvent, notes string) domain.AssetEvent {
	return domain.AssetEvent{
		ID: newID(), TenantID: actor.TenantID, AssetID: original.AssetID, TransactionID: transaction.ID,
		Type: domain.AssetEventVoid, TypeID: eventType.ID, SystemType: domain.AssetEventVoid,
		BaseAmountMinor: 0, BaseCurrency: original.BaseCurrency, Notes: notes,
		TradeInLinkID: original.TradeInLinkID, VoidsEventID: original.ID,
		OccurredAt: transaction.OccurredAt, CreatedByUserID: actor.UserID, CreatedAt: transaction.CreatedAt,
		Source: transaction.Source, ExternalReference: transaction.ExternalReference,
	}
}
