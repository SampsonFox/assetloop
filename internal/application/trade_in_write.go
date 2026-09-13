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

// pendingEconomic is one economic event prepared but not yet appended. Economic
// events of a trade-in command are appended once per distinct asset.
type pendingEconomic struct {
	transaction domain.AssetTransaction
	event       domain.AssetEvent
	command     RecordEvent
}

// pendingPair is one neutral pair prepared but not yet appended.
type pendingPair struct {
	linkID      string
	transaction domain.AssetTransaction
	source      domain.AssetEvent
	destination domain.AssetEvent
}

// recordInTransaction records one confirmed trade-in inside the caller's tenant
// transaction. Every pair is validated before the first write; the economic
// events, the two neutral events of each pair and the idempotency receipt then
// commit together, so an invalid late pair leaves nothing behind. It is private:
// callers must go through ManagementService so the outer tenant transaction and
// receipt are never bypassed.
func (s *TradeInService) recordInTransaction(ctx context.Context, actor Principal, cmd TradeInCommand) (TradeInResult, error) {
	if err := actor.Require(CapabilityManageLifecycle); err != nil {
		return TradeInResult{}, err
	}
	cmd = normalizeTradeInCommand(cmd)
	pairs, err := s.planPairs(ctx, actor, cmd)
	if err != nil {
		return TradeInResult{}, err
	}
	occurredAt, err := s.lifecycle().validOccurredAt(cmd.OccurredAt)
	if err != nil {
		return TradeInResult{}, err
	}
	baseCurrency, err := s.baseCurrency(ctx, actor.TenantID)
	if err != nil {
		return TradeInResult{}, err
	}
	types, err := s.systemTypes(ctx, actor.TenantID)
	if err != nil {
		return TradeInResult{}, err
	}
	notes := strings.TrimSpace(cmd.Notes)
	snapshotIDs := make([]string, 0, len(pairs)*2)
	for _, pair := range pairs {
		snapshotIDs = append(snapshotIDs, pair.newAsset.ID, pair.oldAsset.ID)
	}
	summaries, err := assetTagSummaries(ctx, s.store, actor.TenantID, snapshotIDs)
	if err != nil {
		return TradeInResult{}, fmt.Errorf("read trade-in asset specifications: %w", err)
	}
	pendingMoney := map[string]pendingEconomic{}
	reusedMoney := map[string]string{}
	pending := make([]pendingPair, 0, len(pairs))
	results := make([]TradeInPairResult, len(pairs))
	for index, pair := range pairs {
		link, found, err := s.activeLink(ctx, actor.TenantID, pair.newAsset.ID, pair.oldAsset.ID)
		if err != nil {
			return TradeInResult{}, err
		}
		if found {
			// Different request key, same effective pair: the existing relation is
			// returned instead of a duplicate, and no money is recorded again. An
			// explicit reused ID is still checked here so a stale ID can never be
			// silently replaced by the already-recorded amount.
			if err := s.requireExplicitSelection(ctx, actor, pair.newAsset, pair.newSel, domain.AssetEventPurchase); err != nil {
				return TradeInResult{}, err
			}
			if err := s.requireExplicitSelection(ctx, actor, pair.oldAsset, pair.oldSel, domain.AssetEventSale); err != nil {
				return TradeInResult{}, err
			}
			results[index] = TradeInPairResult{
				LinkID: link.LinkID, LinkStatus: TradeInStatusReused,
				SourceEventID: link.SourceEventID, DestinationEventID: link.DestinationEventID,
				NewAssetID: pair.newAsset.ID, OldAssetID: pair.oldAsset.ID,
				NewAssetName: relatedAssetName(pair.newAsset), OldAssetName: relatedAssetName(pair.oldAsset),
				NewEconomicStatus: TradeInStatusReused, OldEconomicStatus: TradeInStatusReused,
			}
			if event, found, err := s.effectiveEvent(ctx, actor.TenantID, pair.newAsset.ID, domain.AssetEventPurchase); err != nil {
				return TradeInResult{}, err
			} else if found {
				results[index].NewEconomicEventID = event.ID
			}
			if event, found, err := s.effectiveEvent(ctx, actor.TenantID, pair.oldAsset.ID, domain.AssetEventSale); err != nil {
				return TradeInResult{}, err
			} else if found {
				results[index].OldEconomicEventID = event.ID
			}
			continue
		}
		newEventID, newStatus, err := s.resolveEconomic(ctx, actor, pair.newAsset, pair.newSel, domain.AssetEventPurchase, types.purchase, occurredAt, pendingMoney, reusedMoney)
		if err != nil {
			return TradeInResult{}, err
		}
		oldEventID, oldStatus, err := s.resolveEconomic(ctx, actor, pair.oldAsset, pair.oldSel, domain.AssetEventSale, types.sale, occurredAt, pendingMoney, reusedMoney)
		if err != nil {
			return TradeInResult{}, err
		}
		createdAt := s.now().UTC()
		transaction := domain.AssetTransaction{
			ID: newID(), TenantID: actor.TenantID, OccurredAt: occurredAt, Source: tradeInSource(cmd.Source),
			ExternalReference: strings.TrimSpace(cmd.ExternalReference), Notes: notes,
			CreatedByUserID: actor.UserID, CreatedAt: createdAt,
		}
		// The two neutral events of one pair share one grouping transaction.
		linkID := newID()
		results[index] = TradeInPairResult{
			LinkID: linkID, LinkStatus: TradeInStatusCreated,
			NewAssetID: pair.newAsset.ID, OldAssetID: pair.oldAsset.ID,
			NewAssetName: relatedAssetName(pair.newAsset), OldAssetName: relatedAssetName(pair.oldAsset),
			NewEconomicEventID: newEventID, NewEconomicStatus: newStatus,
			OldEconomicEventID: oldEventID, OldEconomicStatus: oldStatus,
		}
		pending = append(pending, pendingPair{
			linkID: linkID, transaction: transaction,
			source: pairedEvent(actor, transaction, types.source, pair.newAsset, pair.oldAsset,
				relatedAssetName(pair.oldAsset), relatedAssetSpecification(pair.oldAsset, summaries), linkID, domain.TradeInStateActive, baseCurrency, occurredAt, notes),
			destination: pairedEvent(actor, transaction, types.destination, pair.oldAsset, pair.newAsset,
				relatedAssetName(pair.newAsset), relatedAssetSpecification(pair.newAsset, summaries), linkID, domain.TradeInStateActive, baseCurrency, occurredAt, notes),
		})
	}
	// Persist money first, once per distinct asset and in a deterministic order.
	assetIDs := make([]string, 0, len(pendingMoney))
	for assetID := range pendingMoney {
		assetIDs = append(assetIDs, assetID)
	}
	sort.Strings(assetIDs)
	for _, assetID := range assetIDs {
		prepared := pendingMoney[assetID]
		if err := s.store.AppendAssetEvent(ctx, prepared.transaction, prepared.event); err != nil {
			return TradeInResult{}, fmt.Errorf("append trade-in economic event: %w", err)
		}
	}
	for _, item := range pending {
		if err := s.store.CreateAssetEvents(ctx, item.transaction, []domain.AssetEvent{item.source, item.destination}); err != nil {
			return TradeInResult{}, fmt.Errorf("append trade-in pair: %w", err)
		}
		for index := range results {
			if results[index].LinkID != item.linkID {
				continue
			}
			results[index].SourceEventID = item.source.ID
			results[index].DestinationEventID = item.destination.ID
		}
	}
	return TradeInResult{Direction: cmd.Direction, CurrentAssetID: strings.TrimSpace(cmd.CurrentAssetID), Pairs: results}, nil
}

// resolveEconomic selects or appends the single purchase/sale of one pair side.
// A reused event must still be effective and belong to the same asset and type;
// a new event is refused when a valid event already exists, so a concurrent
// winner can never be replaced by a silently substituted amount.
func (s *TradeInService) resolveEconomic(ctx context.Context, actor Principal, asset domain.Asset, selection EconomicSelection,
	kind domain.AssetEventType, eventType domain.AssetEventTypeDefinition, occurredAt time.Time,
	pending map[string]pendingEconomic, reused map[string]string) (string, string, error) {
	if selection.ExistingEventID != "" {
		event, err := s.store.GetAssetEvent(ctx, actor.TenantID, strings.TrimSpace(selection.ExistingEventID))
		if errors.Is(err, sql.ErrNoRows) {
			return "", "", NewInputError("validation.trade_in_event_stale")
		}
		if err != nil {
			return "", "", fmt.Errorf("get reused economic event: %w", err)
		}
		if event.IsVoided || event.Kind() != kind || event.AssetID != asset.ID {
			return "", "", NewInputError("validation.trade_in_event_stale")
		}
		if prepared, ok := pending[asset.ID]; ok && prepared.event.ID != event.ID {
			return "", "", NewInputError("validation.trade_in_asset_conflict")
		}
		reused[asset.ID] = event.ID
		return event.ID, TradeInStatusReused, nil
	}
	if selection.NewEvent == nil {
		return "", "", NewInputError("validation.trade_in_selection_required")
	}
	if _, ok := reused[asset.ID]; ok {
		return "", "", NewInputError("validation.trade_in_asset_conflict")
	}
	command, err := normalizeRecordAmount(*selection.NewEvent)
	if err != nil {
		return "", "", err
	}
	command.AssetID = asset.ID
	command.RequestKey = ""
	if requested := requestedEventType(command); requested != "" {
		resolved, err := scopedEventType(ctx, s.store, actor.TenantID, requested)
		if err != nil {
			return "", "", err
		}
		if resolved.SystemCode != kind {
			return "", "", NewInputError("validation.trade_in_event_type")
		}
	}
	command.Type = kind
	command.TypeID = eventType.ID
	if prepared, ok := pending[asset.ID]; ok {
		if !sameEconomicCommand(prepared.command, command) {
			return "", "", NewInputError("validation.trade_in_asset_conflict")
		}
		return prepared.event.ID, TradeInStatusCreated, nil
	}
	if event, found, err := s.effectiveEvent(ctx, actor.TenantID, asset.ID, kind); err != nil {
		return "", "", err
	} else if found {
		return "", "", NewInputError("validation.trade_in_event_exists", event.ID)
	}
	if err := s.lifecycle().validateLifecycle(ctx, actor, asset.ID, kind); err != nil {
		return "", "", err
	}
	transaction, event, err := s.lifecycle().prepareEvent(ctx, actor, command, eventType, "")
	if err != nil {
		return "", "", err
	}
	// A supplied RelatedAssetID on a trade-in economic entry is validated through
	// the ordinary relation policy and never silently discarded.
	if err := s.lifecycle().attachRelatedAsset(ctx, actor, command.RelatedAssetID, event.AssetID, &event); err != nil {
		return "", "", err
	}
	pending[asset.ID] = pendingEconomic{transaction: transaction, event: event, command: command}
	return event.ID, TradeInStatusCreated, nil
}

// requireExplicitSelection refuses a stale explicit economic ID even when the
// pair already exists, so an idempotent retry can never silently substitute the
// already-recorded amount for the one the caller named.
func (s *TradeInService) requireExplicitSelection(ctx context.Context, actor Principal, asset domain.Asset, selection EconomicSelection, kind domain.AssetEventType) error {
	requested := strings.TrimSpace(selection.ExistingEventID)
	if requested == "" {
		return nil
	}
	event, err := s.store.GetAssetEvent(ctx, actor.TenantID, requested)
	if errors.Is(err, sql.ErrNoRows) {
		return NewInputError("validation.trade_in_event_stale")
	}
	if err != nil {
		return fmt.Errorf("get reused economic event: %w", err)
	}
	if event.IsVoided || event.Kind() != kind || event.AssetID != asset.ID {
		return NewInputError("validation.trade_in_event_stale")
	}
	return nil
}

func requestedEventType(cmd RecordEvent) string {
	if value := strings.TrimSpace(cmd.TypeID); value != "" {
		return value
	}
	return strings.TrimSpace(string(cmd.Type))
}

func sameEconomicCommand(a, b RecordEvent) bool {
	return a.AssetID == b.AssetID && a.Type == b.Type && a.TypeID == b.TypeID && a.AmountMinor == b.AmountMinor &&
		a.Currency == b.Currency && a.FXRateScaled == b.FXRateScaled && a.FXRateDate.Equal(b.FXRateDate) &&
		a.FXRateSource == b.FXRateSource && a.FXConfirmed == b.FXConfirmed && a.OccurredAt.Equal(b.OccurredAt) &&
		a.Source == b.Source && a.ExternalReference == b.ExternalReference && a.Notes == b.Notes
}

type resolvedLink struct {
	linkID      string
	source      domain.AssetEvent
	destination domain.AssetEvent
	newAsset    domain.Asset
	oldAsset    domain.Asset
}

// requireActiveLink loads the pair addressed by its stable link ID and the caller's
// expected event IDs. Stale expectations and a purged endpoint are refused, and a
// missing asset is never recreated.
func (s *TradeInService) requireActiveLink(ctx context.Context, actor Principal, linkID, expectedSource, expectedDestination string) (resolvedLink, error) {
	if err := validID("link ID", linkID); err != nil {
		return resolvedLink{}, err
	}
	if err := validID("trade-in source event ID", expectedSource); err != nil {
		return resolvedLink{}, err
	}
	if err := validID("trade-in destination event ID", expectedDestination); err != nil {
		return resolvedLink{}, err
	}
	source, sourceErr := s.store.GetAssetEvent(ctx, actor.TenantID, expectedSource)
	if sourceErr != nil && !errors.Is(sourceErr, sql.ErrNoRows) {
		return resolvedLink{}, fmt.Errorf("get trade-in source event: %w", sourceErr)
	}
	destination, destinationErr := s.store.GetAssetEvent(ctx, actor.TenantID, expectedDestination)
	if destinationErr != nil && !errors.Is(destinationErr, sql.ErrNoRows) {
		return resolvedLink{}, fmt.Errorf("get trade-in destination event: %w", destinationErr)
	}
	if errors.Is(sourceErr, sql.ErrNoRows) || errors.Is(destinationErr, sql.ErrNoRows) {
		// A whole-item purge removes that item's events, including one side of this
		// pair. The surviving event still names the purged counterpart, so the
		// caller receives the deleted-object error instead of a generic conflict.
		if sourceErr == nil && source.RelatedAssetID != "" {
			if _, err := s.store.GetAsset(ctx, actor.TenantID, source.RelatedAssetID); errors.Is(err, sql.ErrNoRows) {
				return resolvedLink{}, NewInputError("validation.trade_in_asset_deleted")
			}
		}
		if destinationErr == nil && destination.RelatedAssetID != "" {
			if _, err := s.store.GetAsset(ctx, actor.TenantID, destination.RelatedAssetID); errors.Is(err, sql.ErrNoRows) {
				return resolvedLink{}, NewInputError("validation.trade_in_asset_deleted")
			}
		}
		return resolvedLink{}, NewInputError("validation.trade_in_link_stale")
	}
	stale := source.TradeInLinkID != linkID || destination.TradeInLinkID != linkID ||
		source.IsVoided || destination.IsVoided ||
		source.Kind() != domain.AssetEventTradeInSource || destination.Kind() != domain.AssetEventTradeInDestination ||
		source.TradeInState != domain.TradeInStateActive || destination.TradeInState != domain.TradeInStateActive ||
		source.RelatedAssetID != destination.AssetID || destination.RelatedAssetID != source.AssetID
	if stale {
		return resolvedLink{}, NewInputError("validation.trade_in_link_stale")
	}
	newAsset, err := s.store.GetAsset(ctx, actor.TenantID, source.AssetID)
	if errors.Is(err, sql.ErrNoRows) {
		return resolvedLink{}, NewInputError("validation.trade_in_asset_deleted")
	}
	if err != nil {
		return resolvedLink{}, fmt.Errorf("get trade-in asset: %w", err)
	}
	oldAsset, err := s.store.GetAsset(ctx, actor.TenantID, destination.AssetID)
	if errors.Is(err, sql.ErrNoRows) {
		return resolvedLink{}, NewInputError("validation.trade_in_asset_deleted")
	}
	if err != nil {
		return resolvedLink{}, fmt.Errorf("get trade-in asset: %w", err)
	}
	return resolvedLink{linkID: linkID, source: source, destination: destination, newAsset: newAsset, oldAsset: oldAsset}, nil
}

// correctInTransaction voids both current paired events and appends the
// replacement pair under the same stable link ID. Money is never created or
// modified: both target assets must already hold the required economic event.
func (s *TradeInService) correctInTransaction(ctx context.Context, actor Principal, cmd CorrectTradeInLinkCommand) (TradeInResult, error) {
	if err := actor.Require(CapabilityManageLifecycle); err != nil {
		return TradeInResult{}, err
	}
	link, err := s.requireActiveLink(ctx, actor, cmd.LinkID, cmd.ExpectedSourceEventID, cmd.ExpectedDestinationEventID)
	if err != nil {
		return TradeInResult{}, err
	}
	if err := validID("asset ID", cmd.NewAssetID); err != nil {
		return TradeInResult{}, err
	}
	if err := validID("asset ID", cmd.OldAssetID); err != nil {
		return TradeInResult{}, err
	}
	newTarget, err := s.requireAsset(ctx, actor, strings.TrimSpace(cmd.NewAssetID))
	if err != nil {
		return TradeInResult{}, err
	}
	oldTarget, err := s.requireAsset(ctx, actor, strings.TrimSpace(cmd.OldAssetID))
	if err != nil {
		return TradeInResult{}, err
	}
	if newTarget.ID == oldTarget.ID {
		return TradeInResult{}, NewInputError("validation.trade_in_selection_conflict")
	}
	if _, found, err := s.effectiveEvent(ctx, actor.TenantID, newTarget.ID, domain.AssetEventPurchase); err != nil {
		return TradeInResult{}, err
	} else if !found {
		return TradeInResult{}, NewInputError("validation.trade_in_target_purchase")
	}
	if _, found, err := s.effectiveEvent(ctx, actor.TenantID, oldTarget.ID, domain.AssetEventSale); err != nil {
		return TradeInResult{}, err
	} else if !found {
		return TradeInResult{}, NewInputError("validation.trade_in_target_sale")
	}
	if conflict, err := s.conflictingLink(ctx, actor.TenantID, newTarget.ID, oldTarget.ID, cmd.LinkID); err != nil {
		return TradeInResult{}, err
	} else if conflict != "" {
		return TradeInResult{}, NewInputError("validation.trade_in_link_exists")
	}
	occurredAt, err := s.lifecycle().validOccurredAt(cmd.OccurredAt)
	if err != nil {
		return TradeInResult{}, err
	}
	baseCurrency, err := s.baseCurrency(ctx, actor.TenantID)
	if err != nil {
		return TradeInResult{}, err
	}
	types, err := s.systemTypes(ctx, actor.TenantID)
	if err != nil {
		return TradeInResult{}, err
	}
	notes := strings.TrimSpace(cmd.Notes)
	summaries, err := assetTagSummaries(ctx, s.store, actor.TenantID, []string{newTarget.ID, oldTarget.ID})
	if err != nil {
		return TradeInResult{}, fmt.Errorf("read trade-in asset specifications: %w", err)
	}
	transaction := domain.AssetTransaction{
		ID: newID(), TenantID: actor.TenantID, OccurredAt: occurredAt, Source: tradeInSource(""),
		ExternalReference: strings.TrimSpace(cmd.ExternalReference), Notes: notes,
		CreatedByUserID: actor.UserID, CreatedAt: s.now().UTC(),
	}
	source := pairedEvent(actor, transaction, types.source, newTarget, oldTarget,
		relatedAssetName(oldTarget), relatedAssetSpecification(oldTarget, summaries), link.linkID, domain.TradeInStateActive, baseCurrency, occurredAt, notes)
	destination := pairedEvent(actor, transaction, types.destination, oldTarget, newTarget,
		relatedAssetName(newTarget), relatedAssetSpecification(newTarget, summaries), link.linkID, domain.TradeInStateActive, baseCurrency, occurredAt, notes)
	// A replacement on the same owned asset keeps its predecessor in the event
	// lineage; when the endpoint changes, only the voided original and the shared
	// link ID carry the history, so no cross-asset replacement ID is written.
	if newTarget.ID == link.newAsset.ID {
		source.ReplacesEventID = link.source.ID
	}
	if oldTarget.ID == link.oldAsset.ID {
		destination.ReplacesEventID = link.destination.ID
	}
	events := []domain.AssetEvent{
		voidPairedEvent(actor, transaction, types.void, link.source, "作废并由交易链路更正替代"),
		voidPairedEvent(actor, transaction, types.void, link.destination, "作废并由交易链路更正替代"),
		source,
		destination,
	}
	if err := s.store.CreateAssetEvents(ctx, transaction, events); err != nil {
		return TradeInResult{}, fmt.Errorf("correct trade-in link: %w", err)
	}
	entry := TradeInPairResult{
		LinkID: link.linkID, LinkStatus: TradeInLinkStatusCorrected,
		SourceEventID: source.ID, DestinationEventID: destination.ID,
		NewAssetID: newTarget.ID, OldAssetID: oldTarget.ID,
		NewAssetName: relatedAssetName(newTarget), OldAssetName: relatedAssetName(oldTarget),
		NewEconomicStatus: TradeInStatusUnchanged, OldEconomicStatus: TradeInStatusUnchanged,
	}
	if event, found, err := s.effectiveEvent(ctx, actor.TenantID, newTarget.ID, domain.AssetEventPurchase); err != nil {
		return TradeInResult{}, err
	} else if found {
		entry.NewEconomicEventID = event.ID
	}
	if event, found, err := s.effectiveEvent(ctx, actor.TenantID, oldTarget.ID, domain.AssetEventSale); err != nil {
		return TradeInResult{}, err
	} else if found {
		entry.OldEconomicEventID = event.ID
	}
	return TradeInResult{Pairs: []TradeInPairResult{entry}}, nil
}

// cancelInTransaction voids both current paired events and appends cancelled
// replacements under the same stable link ID. The pair disappears from the
// default effective view but stays visible with history, and the same pair can be
// linked again afterwards with a new stable link ID.
func (s *TradeInService) cancelInTransaction(ctx context.Context, actor Principal, cmd CancelTradeInLinkCommand) (TradeInResult, error) {
	if err := actor.Require(CapabilityManageLifecycle); err != nil {
		return TradeInResult{}, err
	}
	link, err := s.requireActiveLink(ctx, actor, cmd.LinkID, cmd.ExpectedSourceEventID, cmd.ExpectedDestinationEventID)
	if err != nil {
		return TradeInResult{}, err
	}
	occurredAt := cmd.OccurredAt
	if occurredAt.IsZero() {
		occurredAt = s.now().UTC()
	}
	occurredAt, err = s.lifecycle().validOccurredAt(occurredAt)
	if err != nil {
		return TradeInResult{}, err
	}
	baseCurrency, err := s.baseCurrency(ctx, actor.TenantID)
	if err != nil {
		return TradeInResult{}, err
	}
	types, err := s.systemTypes(ctx, actor.TenantID)
	if err != nil {
		return TradeInResult{}, err
	}
	notes := strings.TrimSpace(cmd.Notes)
	summaries, err := assetTagSummaries(ctx, s.store, actor.TenantID, []string{link.newAsset.ID, link.oldAsset.ID})
	if err != nil {
		return TradeInResult{}, fmt.Errorf("read trade-in asset specifications: %w", err)
	}
	transaction := domain.AssetTransaction{
		ID: newID(), TenantID: actor.TenantID, OccurredAt: occurredAt, Source: tradeInSource(""),
		Notes: notes, CreatedByUserID: actor.UserID, CreatedAt: s.now().UTC(),
	}
	source := pairedEvent(actor, transaction, types.source, link.newAsset, link.oldAsset,
		relatedAssetName(link.oldAsset), relatedAssetSpecification(link.oldAsset, summaries), link.linkID, domain.TradeInStateCancelled, baseCurrency, occurredAt, notes)
	destination := pairedEvent(actor, transaction, types.destination, link.oldAsset, link.newAsset,
		relatedAssetName(link.newAsset), relatedAssetSpecification(link.newAsset, summaries), link.linkID, domain.TradeInStateCancelled, baseCurrency, occurredAt, notes)
	// Cancellation replaces the same two owned assets, so each cancelled event
	// keeps its own predecessor in the lineage.
	source.ReplacesEventID = link.source.ID
	destination.ReplacesEventID = link.destination.ID
	events := []domain.AssetEvent{
		voidPairedEvent(actor, transaction, types.void, link.source, "作废并由交易链路取消替代"),
		voidPairedEvent(actor, transaction, types.void, link.destination, "作废并由交易链路取消替代"),
		source,
		destination,
	}
	if err := s.store.CreateAssetEvents(ctx, transaction, events); err != nil {
		return TradeInResult{}, fmt.Errorf("cancel trade-in link: %w", err)
	}
	entry := TradeInPairResult{
		LinkID: link.linkID, LinkStatus: TradeInLinkStatusCancelled,
		SourceEventID: source.ID, DestinationEventID: destination.ID,
		NewAssetID: link.newAsset.ID, OldAssetID: link.oldAsset.ID,
		NewAssetName: relatedAssetName(link.newAsset), OldAssetName: relatedAssetName(link.oldAsset),
		NewEconomicStatus: TradeInStatusUnchanged, OldEconomicStatus: TradeInStatusUnchanged,
	}
	if event, found, err := s.effectiveEvent(ctx, actor.TenantID, link.newAsset.ID, domain.AssetEventPurchase); err != nil {
		return TradeInResult{}, err
	} else if found {
		entry.NewEconomicEventID = event.ID
	}
	if event, found, err := s.effectiveEvent(ctx, actor.TenantID, link.oldAsset.ID, domain.AssetEventSale); err != nil {
		return TradeInResult{}, err
	} else if found {
		entry.OldEconomicEventID = event.ID
	}
	return TradeInResult{Pairs: []TradeInPairResult{entry}}, nil
}

func (s *TradeInService) conflictingLink(ctx context.Context, tenantID, newAssetID, oldAssetID, exceptLinkID string) (string, error) {
	links, err := s.store.TradeInLinks(ctx, tenantID, newAssetID, false)
	if err != nil {
		return "", fmt.Errorf("list trade-in links: %w", err)
	}
	for _, link := range links {
		if link.OldAssetID == oldAssetID && link.LinkID != exceptLinkID {
			return link.LinkID, nil
		}
	}
	return "", nil
}

func tradeInSource(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "manual"
	}
	return value
}

// RecordTradeIn persists one confirmed trade-in with a durable tenant/user/key
// receipt. Replays return the original immutable result; a different payload with
// the same key is refused. The command is normalized before the fingerprint, so
// the same content in a different counterpart order replays the original result.
// Lifecycle capability is required, not catalog.
func (s *ManagementService) RecordTradeIn(ctx context.Context, actor Principal, key string, cmd TradeInCommand) (TradeInResult, error) {
	cmd = normalizeTradeInCommand(cmd)
	cmd.RequestKey = ""
	return managementWrite(ctx, s, actor, key, "trade_in_record", CapabilityManageLifecycle, cmd, func(store ManagementStore) (TradeInResult, error) {
		return NewTradeInService(store).recordInTransaction(ctx, actor, cmd)
	})
}

// CorrectTradeInLink voids and replaces both events of one stable link ID.
func (s *ManagementService) CorrectTradeInLink(ctx context.Context, actor Principal, key string, cmd CorrectTradeInLinkCommand) (TradeInResult, error) {
	cmd.RequestKey = ""
	return managementWrite(ctx, s, actor, key, "trade_in_correct", CapabilityManageLifecycle, cmd, func(store ManagementStore) (TradeInResult, error) {
		return NewTradeInService(store).correctInTransaction(ctx, actor, cmd)
	})
}

// CancelTradeInLink voids one link and appends cancelled replacements.
func (s *ManagementService) CancelTradeInLink(ctx context.Context, actor Principal, key string, cmd CancelTradeInLinkCommand) (TradeInResult, error) {
	cmd.RequestKey = ""
	return managementWrite(ctx, s, actor, key, "trade_in_cancel", CapabilityManageLifecycle, cmd, func(store ManagementStore) (TradeInResult, error) {
		return NewTradeInService(store).cancelInTransaction(ctx, actor, cmd)
	})
}

// PreviewTradeIn resolves effective money and missing fields without writing and
// without a receipt, so a read-only caller can review a command.
func (s *ManagementService) PreviewTradeIn(ctx context.Context, actor Principal, cmd TradeInCommand) (TradeInPreview, error) {
	return NewTradeInService(s.store).Preview(ctx, actor, cmd)
}
