package storetest

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/SampsonFox/assetloop/internal/domain"
)

// RunTradeIn proves the trade-in relation on one tenant: explicit selected pairs,
// reused or newly recorded money once per distinct asset, neutral zero-amount
// paired events, idempotent replay and conflict, append-only correction and
// cancellation under a stable link ID, and survival of one endpoint's purge.
// Fixture values and IDs are synthetic.
//
// Direction contract under test: with TradeInDirectionSource the CURRENT asset is
// the NEW asset carrying trade_in_source and the purchase while every counterpart
// is an OLD asset carrying the sale; with TradeInDirectionDestination the CURRENT
// asset is the OLD asset carrying trade_in_destination and the sale while every
// counterpart is a NEW asset carrying the purchase.
func RunTradeIn(t *testing.T, store ManagementStore) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	f := newTradeInFixture(t, ctx, store)
	manager := application.NewManagementService(store, nil)
	// fail is always called with the currently active *testing.T. Reporting from
	// the parent test while a subtest is running would attribute the failure to
	// the wrong test.
	fail := func(t *testing.T, label string, err error, code string) {
		t.Helper()
		var inputErr application.InputError
		if !errors.As(err, &inputErr) || inputErr.Code != code {
			t.Fatalf("%s: want %s, got %v", label, code, err)
		}
	}

	// The two pairing types are selectable user options with direction metadata,
	// while the ordinary record path keeps refusing them.
	t.Run("pairing types are selectable but routed", func(t *testing.T) {
		types, err := f.lifecycle.EventTypes(ctx, f.owner)
		if err != nil {
			t.Fatal(err)
		}
		offered := map[domain.AssetEventType]bool{}
		for _, item := range types {
			if item.SystemCode == domain.AssetEventVoid {
				t.Fatal("the technical void type must stay hidden")
			}
			if !domain.IsTradeInSystemType(item.SystemCode) {
				continue
			}
			offered[item.SystemCode] = true
			if item.Name != string(item.SystemCode) || !item.BuiltIn || item.Cashflow != domain.AssetEventNeutral {
				t.Fatalf("pairing type %s must stay a canonical built-in neutral type: %+v", item.SystemCode, item)
			}
			if _, ok := item.TradeInRole(); !ok {
				t.Fatalf("pairing type %s must expose a direction", item.SystemCode)
			}
		}
		if !offered[domain.AssetEventTradeInSource] || !offered[domain.AssetEventTradeInDestination] {
			t.Fatalf("both pairing types must be discoverable: %+v", offered)
		}
		page, err := f.lifecycle.EventTypePage(ctx, f.owner, application.EventTypeListOptions{Page: 1, PageSize: 50})
		if err != nil {
			t.Fatal(err)
		}
		listed := map[domain.AssetEventType]bool{}
		for _, item := range page.Types {
			listed[item.SystemCode] = true
		}
		if !listed[domain.AssetEventTradeInSource] || !listed[domain.AssetEventTradeInDestination] || listed[domain.AssetEventVoid] {
			t.Fatalf("paged type list mismatch: %+v", listed)
		}
		var pairingID string
		all, err := store.ListAssetEventTypes(ctx, f.owner.TenantID)
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range all {
			if item.SystemCode == domain.AssetEventTradeInSource {
				pairingID = item.ID
			}
		}
		if pairingID == "" {
			t.Fatal("the trade_in_source system type is missing")
		}
		if _, err := f.lifecycle.Record(ctx, f.owner, application.RecordEvent{
			RequestKey: "trade-in-direct", AssetID: f.asset(t, "new2").ID, TypeID: pairingID,
			AmountMinor: 0, Currency: "CNY", OccurredAt: f.at(9),
		}); err == nil {
			t.Fatal("a pairing type was recordable through the ordinary path")
		} else {
			fail(t, "direct pairing type record", err, "validation.trade_in_route")
		}
		if _, err := f.lifecycle.UpdateEventType(ctx, f.owner, pairingID, application.UpdateEventType{Name: "renamed", Cashflow: domain.AssetEventNeutral}); err == nil {
			t.Fatal("a built-in pairing type was renamed")
		}
		if _, err := f.lifecycle.Record(ctx, f.owner, application.RecordEvent{
			RequestKey: "trade-in-name-collision", AssetID: f.asset(t, "new2").ID,
			Type: domain.AssetEventType("trade_in_source"), AmountMinor: 0, Currency: "CNY", OccurredAt: f.at(9),
		}); err == nil {
			t.Fatal("a pairing code was recordable by name")
		}
	})

	// One old asset against one new asset, reusing money that already exists on
	// both sides. No amount may change and no extra economic event may appear.
	// The current asset is the OLD destination here.
	t.Run("one to one reuses existing money", func(t *testing.T) {
		old1, new1 := f.asset(t, "old1"), f.asset(t, "new1")
		before := f.totals(t, old1.ID, new1.ID)
		result, err := manager.RecordTradeIn(ctx, f.owner, "trade-in-one-to-one", application.TradeInCommand{
			CurrentAssetID: old1.ID,
			Direction:      domain.TradeInDirectionDestination,
			Current:        application.EconomicSelection{ExistingEventID: f.old1Sale},
			Counterparts:   []application.EconomicSelection{{AssetID: new1.ID, ExistingEventID: f.new1Purchase}},
			OccurredAt:     f.at(10), ExternalReference: "TRADE-1", Notes: "one to one",
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Pairs) != 1 || result.Pairs[0].LinkStatus != application.TradeInStatusCreated ||
			result.Pairs[0].NewEconomicStatus != application.TradeInStatusReused || result.Pairs[0].OldEconomicStatus != application.TradeInStatusReused {
			t.Fatalf("unexpected pair result: %+v", result.Pairs)
		}
		pair := result.Pairs[0]
		source, err := store.GetAssetEvent(ctx, f.owner.TenantID, pair.SourceEventID)
		if err != nil {
			t.Fatal(err)
		}
		destination, err := store.GetAssetEvent(ctx, f.owner.TenantID, pair.DestinationEventID)
		if err != nil {
			t.Fatal(err)
		}
		if source.AssetID != new1.ID || destination.AssetID != old1.ID {
			t.Fatalf("pair direction mismatch: %s %s", source.AssetID, destination.AssetID)
		}
		if source.Kind() != domain.AssetEventTradeInSource || destination.Kind() != domain.AssetEventTradeInDestination {
			t.Fatalf("pair system types mismatch: %s %s", source.Kind(), destination.Kind())
		}
		if role, ok := source.TradeInRole(); !ok || role != domain.TradeInDirectionSource {
			t.Fatalf("source event must carry the source direction: %s %v", role, ok)
		}
		if role, ok := destination.TradeInRole(); !ok || role != domain.TradeInDirectionDestination {
			t.Fatalf("destination event must carry the destination direction: %s %v", role, ok)
		}
		if source.BaseAmountMinor != 0 || destination.BaseAmountMinor != 0 || source.FX != nil || destination.FX != nil {
			t.Fatalf("neutral pair carried money evidence: %+v %+v", source, destination)
		}
		if source.TradeInLinkID == "" || source.TradeInLinkID != destination.TradeInLinkID || pair.LinkID != source.TradeInLinkID {
			t.Fatalf("stable link ID mismatch: %+v", pair)
		}
		if source.TradeInState != domain.TradeInStateActive || destination.TradeInState != domain.TradeInStateActive {
			t.Fatalf("pair state mismatch: %q %q", source.TradeInState, destination.TradeInState)
		}
		if source.TransactionID != destination.TransactionID {
			t.Fatal("paired events must share one grouping transaction")
		}
		if source.RelatedAssetID != old1.ID || source.RelatedAssetName != "Old one" || destination.RelatedAssetID != new1.ID {
			t.Fatalf("relation snapshot mismatch: %+v %+v", source, destination)
		}
		// The association owns its own order reference independently of money, so
		// both paired events must project exactly what was persisted.
		if source.ExternalReference != "TRADE-1" || destination.ExternalReference != "TRADE-1" || source.Source != "manual" {
			t.Fatalf("paired events must project the persisted reference: %+v %+v", source, destination)
		}
		if source.RelatedAssetDeleted || destination.RelatedAssetDeleted {
			t.Fatal("a live relation was projected as deleted")
		}
		if label, deleted := source.RelatedAssetLabel(); label != "Old one" || deleted {
			t.Fatalf("related label projection mismatch: %q %v", label, deleted)
		}
		if source.RelatedAssetSpecLabel() != fixtureTagSummary {
			t.Fatalf("related spec projection mismatch: %q", source.RelatedAssetSpecLabel())
		}
		if f.effectiveKind(t, new1.ID, domain.AssetEventPurchase) != 1 || f.effectiveKind(t, old1.ID, domain.AssetEventSale) != 1 {
			t.Fatal("reuse recorded duplicate money")
		}
		if source.ReplacesEventID != "" || destination.ReplacesEventID != "" {
			t.Fatal("an initial pair must not claim a replacement lineage")
		}
		f.assertTotalsUnchanged(t, before, old1.ID, new1.ID)
		links, err := store.TradeInLinks(ctx, f.owner.TenantID, new1.ID, false)
		if err != nil || len(links) != 1 {
			t.Fatalf("active link projection: %+v %v", links, err)
		}
		if links[0].OldAssetID != old1.ID || links[0].NewAssetID != new1.ID || links[0].SourceEventID != source.ID || links[0].DestinationEventID != destination.ID {
			t.Fatalf("link projection mismatch: %+v", links[0])
		}
		if links[0].State != string(domain.TradeInStateActive) || links[0].OldAssetName != "Old one" || links[0].NewAssetName != "New one" {
			t.Fatalf("link projection labels mismatch: %+v", links[0])
		}
		if links[0].OldAssetSpec != fixtureTagSummary || links[0].NewAssetSpec != fixtureTagSummary {
			t.Fatalf("link projection specification mismatch: %+v", links[0])
		}
		if links, err := store.TradeInLinks(ctx, f.owner.TenantID, old1.ID, false); err != nil || len(links) != 1 {
			t.Fatalf("link projection from the old asset: %+v %v", links, err)
		}
		f.old1Link = source
	})

	// The same request key replays the original result, a changed payload is a
	// conflict, and another key with the same effective pair reuses the relation.
	// A stale explicit ID is still refused inside the existing-pair branch.
	t.Run("idempotent replay and relation reuse", func(t *testing.T) {
		old1, new1 := f.asset(t, "old1"), f.asset(t, "new1")
		cmd := application.TradeInCommand{
			CurrentAssetID: old1.ID, Direction: domain.TradeInDirectionDestination,
			Current:      application.EconomicSelection{ExistingEventID: f.old1Sale},
			Counterparts: []application.EconomicSelection{{AssetID: new1.ID, ExistingEventID: f.new1Purchase}},
			OccurredAt:   f.at(10), ExternalReference: "TRADE-1", Notes: "one to one",
		}
		replayed, err := manager.RecordTradeIn(ctx, f.owner, "trade-in-one-to-one", cmd)
		if err != nil || replayed.Pairs[0].LinkID != f.old1Link.TradeInLinkID || replayed.Pairs[0].SourceEventID != f.old1Link.ID {
			t.Fatalf("replay changed the result: %+v %v", replayed, err)
		}
		conflict := cmd
		conflict.Notes = "different payload"
		fail(t, "same key different payload", mustErr(manager.RecordTradeIn(ctx, f.owner, "trade-in-one-to-one", conflict)), "validation.request_conflict")
		reused, err := manager.RecordTradeIn(ctx, f.owner, "trade-in-one-to-one-again", cmd)
		if err != nil {
			t.Fatal(err)
		}
		if len(reused.Pairs) != 1 || reused.Pairs[0].LinkStatus != application.TradeInStatusReused || reused.Pairs[0].LinkID != f.old1Link.TradeInLinkID {
			t.Fatalf("a same-pair retry created another relation: %+v", reused.Pairs)
		}
		if f.effectiveKind(t, new1.ID, domain.AssetEventPurchase) != 1 || f.effectiveKind(t, old1.ID, domain.AssetEventSale) != 1 {
			t.Fatal("reusing the relation duplicated money")
		}
		// An approved repeated new selection returns the existing pair without
		// recording the (different) submitted amount.
		repeated, err := manager.RecordTradeIn(ctx, f.owner, "trade-in-one-to-one-new-selection", application.TradeInCommand{
			CurrentAssetID: old1.ID, Direction: domain.TradeInDirectionDestination,
			Current:      application.EconomicSelection{ExistingEventID: f.old1Sale},
			Counterparts: []application.EconomicSelection{{AssetID: new1.ID, NewEvent: &application.RecordEvent{AmountMinor: 999_000, Currency: "CNY", OccurredAt: f.at(10)}}},
			OccurredAt:   f.at(10),
		})
		if err != nil || repeated.Pairs[0].LinkStatus != application.TradeInStatusReused || repeated.Pairs[0].NewEconomicEventID != f.new1Purchase {
			t.Fatalf("a repeated new selection must reuse the relation: %+v %v", repeated.Pairs, err)
		}
		if f.effectiveKind(t, new1.ID, domain.AssetEventPurchase) != 1 {
			t.Fatal("a repeated new selection recorded a duplicate purchase")
		}
		// A stale explicit ID is refused even though the pair already exists, so
		// the recorded amount can never be silently substituted.
		fail(t, "stale explicit ID on an existing pair", mustErr(manager.RecordTradeIn(ctx, f.owner, "trade-in-one-to-one-stale", application.TradeInCommand{
			CurrentAssetID: old1.ID, Direction: domain.TradeInDirectionDestination,
			Current:      application.EconomicSelection{ExistingEventID: f.voidedSale},
			Counterparts: []application.EconomicSelection{{AssetID: new1.ID, ExistingEventID: f.new1Purchase}},
			OccurredAt:   f.at(10),
		})), "validation.trade_in_event_stale")
	})

	// Counterpart order is normalized before fingerprinting, so the same content
	// in a different order replays the original receipt instead of conflicting.
	t.Run("reordered counterparts replay", func(t *testing.T) {
		old13, new13, new14 := f.asset(t, "old13"), f.asset(t, "new13"), f.asset(t, "new14")
		cmd := application.TradeInCommand{
			CurrentAssetID: old13.ID, Direction: domain.TradeInDirectionDestination,
			Current: application.EconomicSelection{ExistingEventID: f.old13Sale},
			Counterparts: []application.EconomicSelection{
				{AssetID: new14.ID, ExistingEventID: f.new14Purchase},
				{AssetID: new13.ID, ExistingEventID: f.new13Purchase},
			},
			OccurredAt: f.at(35), Notes: "reordered",
		}
		first, err := manager.RecordTradeIn(ctx, f.owner, "trade-in-reordered", cmd)
		if err != nil || len(first.Pairs) != 2 {
			t.Fatalf("reordered setup failed: %+v %v", first, err)
		}
		reordered := cmd
		reordered.Counterparts = []application.EconomicSelection{cmd.Counterparts[1], cmd.Counterparts[0]}
		replayed, err := manager.RecordTradeIn(ctx, f.owner, "trade-in-reordered", reordered)
		if err != nil {
			t.Fatalf("reordered replay failed: %v", err)
		}
		if len(replayed.Pairs) != len(first.Pairs) || replayed.Pairs[0].LinkID != first.Pairs[0].LinkID ||
			replayed.Pairs[1].LinkID != first.Pairs[1].LinkID || replayed.Pairs[0].NewAssetID != first.Pairs[0].NewAssetID {
			t.Fatalf("reordered replay changed the original result: %+v vs %+v", replayed.Pairs, first.Pairs)
		}
	})

	// One old asset against many new assets: the single sale is reused while each
	// new asset records exactly one purchase, reused or newly created.
	t.Run("many new assets for one old asset", func(t *testing.T) {
		old2, new2, new3 := f.asset(t, "old2"), f.asset(t, "new2"), f.asset(t, "new3")
		cmd := application.TradeInCommand{
			CurrentAssetID: old2.ID, Direction: domain.TradeInDirectionDestination,
			Current: application.EconomicSelection{ExistingEventID: f.old2Sale},
			Counterparts: []application.EconomicSelection{
				{AssetID: new3.ID, ExistingEventID: f.new3Purchase},
				{AssetID: new2.ID, NewEvent: &application.RecordEvent{AmountMinor: 600_000, Currency: "CNY", OccurredAt: f.at(11), Notes: "new phone"}},
			},
			OccurredAt: f.at(11), Notes: "one source two destinations",
		}
		result, err := manager.RecordTradeIn(ctx, f.owner, "trade-in-many-new", cmd)
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Pairs) != 2 {
			t.Fatalf("expected two pairs: %+v", result.Pairs)
		}
		var created, reused string
		for _, pair := range result.Pairs {
			if pair.OldAssetID != old2.ID {
				t.Fatalf("pair direction mismatch: %+v", pair)
			}
			switch pair.NewAssetID {
			case new2.ID:
				created = pair.LinkID
				if pair.NewEconomicStatus != application.TradeInStatusCreated {
					t.Fatalf("new purchase was not created: %+v", pair)
				}
			case new3.ID:
				reused = pair.LinkID
				if pair.NewEconomicStatus != application.TradeInStatusReused {
					t.Fatalf("existing purchase was not reused: %+v", pair)
				}
			default:
				t.Fatalf("unexpected counterpart: %+v", pair)
			}
		}
		if created == "" || reused == "" || created == reused {
			t.Fatalf("distinct stable link IDs are required: %q %q", created, reused)
		}
		if f.effectiveKind(t, new2.ID, domain.AssetEventPurchase) != 1 || f.effectiveKind(t, new3.ID, domain.AssetEventPurchase) != 1 {
			t.Fatal("multi-pair trade-in repeated money")
		}
		if f.effectiveKind(t, old2.ID, domain.AssetEventSale) != 1 || f.effectiveKind(t, old2.ID, domain.AssetEventPurchase) != 1 {
			t.Fatal("the shared source changed money")
		}
		// Reusing the same relations under a different key must not create money
		// or move any aggregate or holding-day figure.
		after := f.totals(t, old2.ID, new2.ID, new3.ID)
		if _, err := manager.RecordTradeIn(ctx, f.owner, "trade-in-many-new-again", cmd); err != nil {
			t.Fatalf("relation reuse failed: %v", err)
		}
		f.assertTotalsUnchanged(t, after, old2.ID, new2.ID, new3.ID)
		if links, err := store.TradeInLinks(ctx, f.owner.TenantID, old2.ID, false); err != nil || len(links) != 2 {
			t.Fatalf("source links: %+v %v", links, err)
		}
	})

	// Many old assets against one new asset: the single purchase is reused while
	// each old asset records exactly one sale. The current asset is the NEW source.
	t.Run("many old assets for one new asset", func(t *testing.T) {
		old3, old4, new4 := f.asset(t, "old3"), f.asset(t, "old4"), f.asset(t, "new4")
		cmd := application.TradeInCommand{
			CurrentAssetID: new4.ID, Direction: domain.TradeInDirectionSource,
			Current: application.EconomicSelection{ExistingEventID: f.new4Purchase},
			Counterparts: []application.EconomicSelection{
				{AssetID: old4.ID, NewEvent: &application.RecordEvent{AmountMinor: 95_000, Currency: "CNY", OccurredAt: f.at(12)}},
				{AssetID: old3.ID, NewEvent: &application.RecordEvent{AmountMinor: 90_000, Currency: "CNY", OccurredAt: f.at(12)}},
			},
			OccurredAt: f.at(12), Notes: "two sources one destination",
		}
		result, err := manager.RecordTradeIn(ctx, f.owner, "trade-in-many-old", cmd)
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Pairs) != 2 || result.Pairs[0].OldAssetID >= result.Pairs[1].OldAssetID {
			t.Fatalf("counterpart order must be normalized: %+v", result.Pairs)
		}
		pairOlder := map[string]bool{result.Pairs[0].OldAssetID: true, result.Pairs[1].OldAssetID: true}
		if !pairOlder[old3.ID] || !pairOlder[old4.ID] {
			t.Fatalf("both old counterparts must be paired: %+v", result.Pairs)
		}
		for _, pair := range result.Pairs {
			if pair.NewAssetID != new4.ID || pair.OldEconomicStatus != application.TradeInStatusCreated || pair.NewEconomicStatus != application.TradeInStatusReused {
				t.Fatalf("unexpected pair: %+v", pair)
			}
		}
		if f.effectiveKind(t, new4.ID, domain.AssetEventPurchase) != 1 {
			t.Fatal("the destination purchase was repeated")
		}
		for _, asset := range []string{old3.ID, old4.ID} {
			if f.effectiveKind(t, asset, domain.AssetEventSale) != 1 {
				t.Fatalf("asset %s must hold exactly one sale", asset)
			}
			if _, summary, err := f.lifecycle.Timeline(ctx, f.owner, asset); err != nil || summary.Status != "sold" {
				t.Fatalf("sale did not reach the lifecycle: %+v %v", summary, err)
			}
		}
		if _, summary, err := f.lifecycle.Timeline(ctx, f.owner, new4.ID); err != nil || summary.Status != "active" || summary.IncomeMinor != 0 {
			t.Fatalf("neutral pair events changed the destination status: %+v %v", summary, err)
		}
		// Reusing the same relations under a different key must not create money
		// or move any aggregate or holding-day figure.
		after := f.totals(t, old3.ID, old4.ID, new4.ID)
		if _, err := manager.RecordTradeIn(ctx, f.owner, "trade-in-many-old-again", cmd); err != nil {
			t.Fatalf("relation reuse failed: %v", err)
		}
		f.assertTotalsUnchanged(t, after, old3.ID, old4.ID, new4.ID)
	})

	// Rejected commands never write, including a failure that appears after an
	// earlier valid pair.
	t.Run("selection guards", func(t *testing.T) {
		old1, new1, new5 := f.asset(t, "old1"), f.asset(t, "new1"), f.asset(t, "new5")
		standalone := f.asset(t, "standalone")
		// The human-readable label is not a valid request key: key validation runs
		// before the invariant under test, so a spaced label would fail early with
		// validation.request_key instead of the intended error code.
		reject := func(label, code string, cmd application.TradeInCommand) {
			t.Helper()
			if _, err := manager.RecordTradeIn(ctx, f.owner, tradeInRequestKey(label), cmd); err != nil {
				fail(t, label, err, code)
			} else {
				t.Fatalf("%s was accepted", label)
			}
		}
		reject("sale for an asset without acquisition", "validation.event_purchase_first", application.TradeInCommand{
			CurrentAssetID: standalone.ID, Direction: domain.TradeInDirectionDestination,
			Current:      application.EconomicSelection{NewEvent: &application.RecordEvent{AmountMinor: 10_000, Currency: "CNY", OccurredAt: f.at(13)}},
			Counterparts: []application.EconomicSelection{{AssetID: new5.ID, ExistingEventID: f.new5Purchase}},
			OccurredAt:   f.at(13),
		})
		reject("missing selection", "validation.trade_in_selection_required", application.TradeInCommand{
			CurrentAssetID: old1.ID, Direction: domain.TradeInDirectionDestination,
			Current:      application.EconomicSelection{ExistingEventID: f.old1Sale},
			Counterparts: []application.EconomicSelection{{AssetID: new5.ID}},
			OccurredAt:   f.at(13),
		})
		reject("new money when a purchase exists", "validation.trade_in_event_exists", application.TradeInCommand{
			CurrentAssetID: new5.ID, Direction: domain.TradeInDirectionSource,
			Current:      application.EconomicSelection{NewEvent: &application.RecordEvent{AmountMinor: 10_000, Currency: "CNY", OccurredAt: f.at(13)}},
			Counterparts: []application.EconomicSelection{{AssetID: old1.ID, ExistingEventID: f.old1Sale}},
			OccurredAt:   f.at(13),
		})
		reject("voided reused sale", "validation.trade_in_event_stale", application.TradeInCommand{
			CurrentAssetID: f.asset(t, "old8").ID, Direction: domain.TradeInDirectionDestination,
			Current:      application.EconomicSelection{ExistingEventID: f.voidedSale},
			Counterparts: []application.EconomicSelection{{AssetID: f.asset(t, "new10").ID, ExistingEventID: f.new10Purchase}},
			OccurredAt:   f.at(13),
		})
		reject("self counterpart", "validation.trade_in_counterpart_duplicate", application.TradeInCommand{
			CurrentAssetID: old1.ID, Direction: domain.TradeInDirectionDestination,
			Current:      application.EconomicSelection{ExistingEventID: f.old1Sale},
			Counterparts: []application.EconomicSelection{{AssetID: old1.ID, ExistingEventID: f.new1Purchase}},
			OccurredAt:   f.at(13),
		})
		reject("unknown counterpart", "validation.trade_in_asset_unavailable", application.TradeInCommand{
			CurrentAssetID: old1.ID, Direction: domain.TradeInDirectionDestination,
			Current:      application.EconomicSelection{ExistingEventID: f.old1Sale},
			Counterparts: []application.EconomicSelection{{AssetID: "99999999-9999-4999-8999-999999999999", ExistingEventID: f.new1Purchase}},
			OccurredAt:   f.at(13),
		})
		reject("unknown direction", "validation.trade_in_direction", application.TradeInCommand{
			CurrentAssetID: old1.ID, Direction: "sideways",
			Current:      application.EconomicSelection{ExistingEventID: f.old1Sale},
			Counterparts: []application.EconomicSelection{{AssetID: new1.ID, ExistingEventID: f.new1Purchase}},
			OccurredAt:   f.at(13),
		})
		counterparts := make([]application.EconomicSelection, 0, 51)
		for index := range 51 {
			counterparts = append(counterparts, application.EconomicSelection{AssetID: fmt.Sprintf("0f0f0f0f-0f0f-4f0f-8f0f-0f0f0f0f%04d", index), ExistingEventID: f.old1Sale})
		}
		reject("too many counterparts", "validation.trade_in_counterparts", application.TradeInCommand{
			CurrentAssetID: old1.ID, Direction: domain.TradeInDirectionDestination,
			Current: application.EconomicSelection{ExistingEventID: f.old1Sale}, Counterparts: counterparts, OccurredAt: f.at(13),
		})
		if f.eventCount(t, standalone.ID) != 0 || f.eventCount(t, new5.ID) != 1 {
			t.Fatal("a rejected command wrote events")
		}
		// A cross-tenant caller sees nothing, including for an existing pair.
		foreign := f.owner
		foreign.TenantID = "99999999-9999-4999-8999-999999999999"
		if _, err := manager.RecordTradeIn(ctx, foreign, "trade-in-cross-tenant", application.TradeInCommand{
			CurrentAssetID: old1.ID, Direction: domain.TradeInDirectionDestination,
			Current:      application.EconomicSelection{ExistingEventID: f.old1Sale},
			Counterparts: []application.EconomicSelection{{AssetID: new1.ID, ExistingEventID: f.new1Purchase}},
			OccurredAt:   f.at(13),
		}); err == nil {
			t.Fatal("a cross-tenant trade-in was accepted")
		}
		if links, err := store.TradeInLinks(ctx, foreign.TenantID, new1.ID, false); err != nil || len(links) != 0 {
			t.Fatalf("cross-tenant link projection leaked: %+v %v", links, err)
		}
	})

	// An invalid pair atomically rolls the whole command back, including its
	// receipt, and the retained key then succeeds.
	t.Run("atomic rollback and receipt", func(t *testing.T) {
		old5, new5, new6 := f.asset(t, "old5"), f.asset(t, "new5"), f.asset(t, "new6")
		cmd := application.TradeInCommand{
			CurrentAssetID: old5.ID, Direction: domain.TradeInDirectionDestination,
			Current:      application.EconomicSelection{ExistingEventID: f.old5Sale},
			Counterparts: []application.EconomicSelection{{AssetID: new5.ID, ExistingEventID: f.new5Purchase}, {AssetID: new6.ID}},
			OccurredAt:   f.at(14), Notes: "late failure",
		}
		beforeEvents := f.eventCount(t, new5.ID) + f.eventCount(t, new6.ID)
		fail(t, "late invalid pair", mustErr(manager.RecordTradeIn(ctx, f.owner, "trade-in-late-failure", cmd)), "validation.trade_in_selection_required")
		if after := f.eventCount(t, new5.ID) + f.eventCount(t, new6.ID); after != beforeEvents {
			t.Fatalf("a rejected command wrote events: %d -> %d", beforeEvents, after)
		}
		if links, err := store.TradeInLinks(ctx, f.owner.TenantID, new5.ID, true); err != nil || len(links) != 0 {
			t.Fatalf("a rejected command wrote a link: %+v %v", links, err)
		}
		retry := cmd
		retry.Counterparts = []application.EconomicSelection{{AssetID: new5.ID, ExistingEventID: f.new5Purchase}, {AssetID: new6.ID, ExistingEventID: f.new6Purchase}}
		beforeReceipt := f.eventCount(t, new6.ID)
		failing := application.NewManagementService(failingTradeInStore{store}, nil)
		if _, err := failing.RecordTradeIn(ctx, f.owner, "trade-in-failed-receipt", retry); err == nil {
			t.Fatal("an injected receipt failure reported success")
		}
		if f.eventCount(t, new6.ID) != beforeReceipt {
			t.Fatal("a failed receipt committed the pair")
		}
		if links, err := store.TradeInLinks(ctx, f.owner.TenantID, new6.ID, true); err != nil || len(links) != 0 {
			t.Fatalf("a failed receipt committed a link: %+v %v", links, err)
		}
		if _, found, err := store.FindManagementRequest(ctx, f.owner.TenantID, f.owner.UserID, "trade-in-failed-receipt"); err != nil || found {
			t.Fatalf("a failed receipt persisted: %v %v", found, err)
		}
		if _, err := manager.RecordTradeIn(ctx, f.owner, "trade-in-failed-receipt", retry); err != nil {
			t.Fatalf("retry after a rolled-back receipt: %v", err)
		}
		if f.effectiveKind(t, new6.ID, domain.AssetEventPurchase) != 1 {
			t.Fatal("the retry did not record exactly one purchase")
		}
	})

	// A normal monetary correction appends a replacement and keeps the relation;
	// the paired evidence stays out of the ordinary correction path.
	t.Run("monetary correction keeps the relation", func(t *testing.T) {
		new1 := f.asset(t, "new1")
		replacement, err := f.lifecycle.Correct(ctx, f.owner, f.new1Purchase, application.RecordEvent{
			AmountMinor: 520_000, Currency: "CNY", OccurredAt: f.at(10), Notes: "price corrected",
		})
		if err != nil {
			t.Fatal(err)
		}
		if replacement.Kind() != domain.AssetEventPurchase || replacement.ReplacesEventID != f.new1Purchase || replacement.RelatedAssetID != "" {
			t.Fatalf("unexpected replacement: %+v", replacement)
		}
		links, err := store.TradeInLinks(ctx, f.owner.TenantID, new1.ID, false)
		if err != nil || len(links) != 1 {
			t.Fatalf("the relation was lost: %+v %v", links, err)
		}
		if links[0].SourceEventID != f.old1Link.ID || links[0].LinkID != f.old1Link.TradeInLinkID {
			t.Fatalf("the relation changed: %+v", links[0])
		}
		if _, err := f.lifecycle.Correct(ctx, f.owner, f.old1Link.ID, application.RecordEvent{AmountMinor: 0, Currency: "CNY", OccurredAt: f.at(10)}); err == nil {
			t.Fatal("a paired event was correctable through the ordinary path")
		} else {
			fail(t, "paired event correction", err, "validation.trade_in_route")
		}
		// A trade-in system type ID is never a valid ordinary reclassification.
		pairingTypes, err := store.ListAssetEventTypes(ctx, f.owner.TenantID)
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range pairingTypes {
			if item.SystemCode != domain.AssetEventTradeInSource {
				continue
			}
			if _, err := f.lifecycle.Correct(ctx, f.owner, replacement.ID, application.RecordEvent{
				TypeID: item.ID, AmountMinor: 520_000, Currency: "CNY", OccurredAt: f.at(15),
			}); err == nil {
				t.Fatal("a trade-in system type was accepted as an ordinary correction target")
			} else {
				fail(t, "trade-in correction target", err, "validation.event_type")
			}
		}
		if _, err := f.lifecycle.Record(ctx, f.owner, application.RecordEvent{
			RequestKey: "trade-in-normal-relation", AssetID: new1.ID, Type: domain.AssetEventRepair,
			RelatedAssetID: pointer(f.asset(t, "standalone").ID), AmountMinor: 1_000, Currency: "CNY", OccurredAt: f.at(15),
		}); err != nil {
			t.Fatalf("an ordinary event with a related asset was rejected: %v", err)
		}
	})

	// An optional related asset on an ordinary record: preserved when omitted,
	// cleared explicitly, and readable through its snapshot after a purge.
	t.Run("optional related asset", func(t *testing.T) {
		standalone, repairAsset := f.asset(t, "standalone"), f.asset(t, "repairTarget")
		related, err := f.lifecycle.Record(ctx, f.owner, application.RecordEvent{
			RequestKey: "related-linked", AssetID: repairAsset.ID, Type: domain.AssetEventRepair,
			RelatedAssetID: pointer(standalone.ID), AmountMinor: 2_000, Currency: "CNY", OccurredAt: f.at(16),
		})
		if err != nil {
			t.Fatal(err)
		}
		if related.RelatedAssetID != standalone.ID || related.RelatedAssetName != "Standalone" || related.RelatedAssetSpec != fixtureTagSummary || related.RelatedAssetDeleted {
			t.Fatalf("related projection mismatch: %+v", related)
		}
		if _, err := f.lifecycle.Record(ctx, f.owner, application.RecordEvent{
			RequestKey: "related-self", AssetID: repairAsset.ID, Type: domain.AssetEventRepair,
			RelatedAssetID: pointer(repairAsset.ID), AmountMinor: 2_000, Currency: "CNY", OccurredAt: f.at(16),
		}); err == nil {
			t.Fatal("a self relation was accepted")
		} else {
			fail(t, "self relation", err, "validation.related_asset_self")
		}
		preserved, err := f.lifecycle.Correct(ctx, f.owner, related.ID, application.RecordEvent{
			AmountMinor: 2_500, Currency: "CNY", OccurredAt: f.at(16), Notes: "same relation",
		})
		if err != nil || preserved.RelatedAssetID != standalone.ID || preserved.RelatedAssetName != "Standalone" {
			t.Fatalf("an omitted relation was not preserved: %+v %v", preserved, err)
		}
		cleared, err := f.lifecycle.Correct(ctx, f.owner, preserved.ID, application.RecordEvent{
			AmountMinor: 2_500, Currency: "CNY", OccurredAt: f.at(16), RelatedAssetID: pointer(""),
		})
		if err != nil || cleared.RelatedAssetID != "" || cleared.RelatedAssetName != "" {
			t.Fatalf("an explicit empty relation did not clear: %+v %v", cleared, err)
		}
		linkedAgain, err := f.lifecycle.Correct(ctx, f.owner, cleared.ID, application.RecordEvent{
			AmountMinor: 2_500, Currency: "CNY", OccurredAt: f.at(16), RelatedAssetID: pointer(standalone.ID),
		})
		if err != nil {
			t.Fatal(err)
		}
		// Purging the target keeps the surviving snapshot readable and correctable.
		if err := application.NewSpecificationService(store).DeleteAsset(ctx, f.owner, standalone.ID); err != nil {
			t.Fatalf("purge related target: %v", err)
		}
		after, err := store.GetAssetEvent(ctx, f.owner.TenantID, linkedAgain.ID)
		if err != nil {
			t.Fatal(err)
		}
		label, deleted := after.RelatedAssetLabel()
		if label != "Standalone" || !deleted {
			t.Fatalf("purged target label mismatch: %q %v", label, deleted)
		}
		if after.RelatedAssetSpecLabel() != fixtureTagSummary {
			t.Fatalf("purged target spec snapshot mismatch: %q", after.RelatedAssetSpecLabel())
		}
		kept, err := f.lifecycle.Correct(ctx, f.owner, linkedAgain.ID, application.RecordEvent{
			AmountMinor: 2_600, Currency: "CNY", OccurredAt: f.at(16),
		})
		if err != nil || kept.RelatedAssetID != standalone.ID || !kept.RelatedAssetDeleted {
			t.Fatalf("a preserved reference to a deleted target was not correctable: %+v %v", kept, err)
		}
		if _, err := f.lifecycle.Correct(ctx, f.owner, kept.ID, application.RecordEvent{
			AmountMinor: 2_600, Currency: "CNY", OccurredAt: f.at(16), RelatedAssetID: pointer(standalone.ID),
		}); err == nil {
			t.Fatal("a deleted target was accepted as a new relation")
		} else {
			fail(t, "deleted target as new relation", err, "validation.related_asset_unavailable")
		}
	})

	// Cancellation keeps both rows, hides them from the effective and the default
	// paged view, exposes them again with history, and permits re-linking.
	t.Run("cancel and re-link", func(t *testing.T) {
		old6, new7 := f.asset(t, "old6"), f.asset(t, "new7")
		created, err := manager.RecordTradeIn(ctx, f.owner, "trade-in-cancel", application.TradeInCommand{
			CurrentAssetID: old6.ID, Direction: domain.TradeInDirectionDestination,
			Current:      application.EconomicSelection{ExistingEventID: f.old6Sale},
			Counterparts: []application.EconomicSelection{{AssetID: new7.ID, ExistingEventID: f.new7Purchase}},
			OccurredAt:   f.at(17), Notes: "cancel me",
		})
		if err != nil {
			t.Fatal(err)
		}
		pair := created.Pairs[0]
		before := f.totals(t, old6.ID, new7.ID)
		cancelled, err := manager.CancelTradeInLink(ctx, f.owner, "trade-in-cancel-link", application.CancelTradeInLinkCommand{
			LinkID: pair.LinkID, ExpectedSourceEventID: pair.SourceEventID, ExpectedDestinationEventID: pair.DestinationEventID,
			OccurredAt: f.at(18), Notes: "no longer traded",
		})
		if err != nil {
			t.Fatal(err)
		}
		if cancelled.Pairs[0].LinkStatus != application.TradeInLinkStatusCancelled || cancelled.Pairs[0].LinkID != pair.LinkID {
			t.Fatalf("cancel result mismatch: %+v", cancelled.Pairs)
		}
		if cancelled.Pairs[0].NewEconomicEventID != f.new7Purchase || cancelled.Pairs[0].OldEconomicEventID != f.old6Sale {
			t.Fatalf("cancel must report the involved economic events: %+v", cancelled.Pairs[0])
		}
		replacement, err := store.GetAssetEvent(ctx, f.owner.TenantID, cancelled.Pairs[0].SourceEventID)
		if err != nil {
			t.Fatal(err)
		}
		if replacement.TradeInState != domain.TradeInStateCancelled || replacement.IsVoided || replacement.BaseAmountMinor != 0 {
			t.Fatalf("cancelled replacement mismatch: %+v", replacement)
		}
		if replacement.ReplacesEventID != pair.SourceEventID {
			t.Fatalf("a same-asset cancel replacement must keep its predecessor: %+v", replacement)
		}
		original, err := store.GetAssetEvent(ctx, f.owner.TenantID, pair.SourceEventID)
		if err != nil || !original.IsVoided {
			t.Fatalf("the cancelled original must stay visible as voided history: %+v %v", original, err)
		}
		if links, err := store.TradeInLinks(ctx, f.owner.TenantID, new7.ID, false); err != nil || len(links) != 0 {
			t.Fatalf("a cancelled pair stayed effective: %+v %v", links, err)
		}
		if links, err := store.TradeInLinks(ctx, f.owner.TenantID, new7.ID, true); err != nil || len(links) != 1 {
			t.Fatalf("a cancelled pair must stay visible with history: %+v %v", links, err)
		}
		// The default paged view hides the cancelled replacement, while history
		// shows it together with the voided original. The two pairing reads are
		// scoped to the trade-in source system type so they measure the relation
		// itself; an extra unfiltered read below proves the ordinary purchase is
		// still visible, because hiding real money would be a silent loss rather
		// than a clean default view.
		pairingOnly := application.EventListOptions{Page: 1, PageSize: 25, Type: string(domain.AssetEventTradeInSource)}
		effective, err := f.lifecycle.TimelinePage(ctx, f.owner, new7.ID, pairingOnly)
		if err != nil || effective.Total != 0 || len(effective.Events) != 0 {
			t.Fatalf("default paged view exposed a cancelled pairing event: %+v %v", effective, err)
		}
		history, err := f.lifecycle.TimelinePage(ctx, f.owner, new7.ID, application.EventListOptions{
			ShowVoided: true, Page: 1, PageSize: 25, Type: string(domain.AssetEventTradeInSource),
		})
		if err != nil || history.Total != 2 || len(history.Events) != 2 {
			t.Fatalf("cancellation history mismatch: %+v %v", history, err)
		}
		// History keeps the exact rows and states: the voided original with its
		// recorded active state, and the unvoided cancelled replacement.
		voidedOriginal, cancelledReplacement := false, false
		for _, event := range history.Events {
			switch event.ID {
			case pair.SourceEventID:
				if !event.IsVoided || event.TradeInState != domain.TradeInStateActive {
					t.Fatalf("the voided original lost its recorded state: %+v", event)
				}
				voidedOriginal = true
			case replacement.ID:
				if event.IsVoided || event.TradeInState != domain.TradeInStateCancelled {
					t.Fatalf("the cancelled replacement must stay visible and unvoided: %+v", event)
				}
				cancelledReplacement = true
			default:
				t.Fatalf("unexpected cancellation history event: %+v", event)
			}
		}
		if !voidedOriginal || !cancelledReplacement {
			t.Fatalf("cancellation history is incomplete: %+v", history.Events)
		}
		// The unfiltered default view still shows the ordinary purchase that the
		// cancelled pair never owned.
		visible, err := f.lifecycle.TimelinePage(ctx, f.owner, new7.ID, application.EventListOptions{Page: 1, PageSize: 25})
		if err != nil || visible.Total != 1 || len(visible.Events) != 1 || visible.Events[0].ID != f.new7Purchase {
			t.Fatalf("the cancelled pair must not hide the ordinary purchase: %+v %v", visible, err)
		}
		if _, err := manager.CancelTradeInLink(ctx, f.owner, "trade-in-cancel-stale", application.CancelTradeInLinkCommand{
			LinkID: pair.LinkID, ExpectedSourceEventID: pair.SourceEventID, ExpectedDestinationEventID: pair.DestinationEventID, OccurredAt: f.at(18),
		}); err == nil {
			t.Fatal("a stale cancellation was accepted")
		} else {
			fail(t, "stale cancellation", err, "validation.trade_in_link_stale")
		}
		f.assertTotalsUnchanged(t, before, old6.ID, new7.ID)
		relinked, err := manager.RecordTradeIn(ctx, f.owner, "trade-in-relink", application.TradeInCommand{
			CurrentAssetID: old6.ID, Direction: domain.TradeInDirectionDestination,
			Current:      application.EconomicSelection{ExistingEventID: f.old6Sale},
			Counterparts: []application.EconomicSelection{{AssetID: new7.ID, ExistingEventID: f.new7Purchase}},
			OccurredAt:   f.at(19),
		})
		if err != nil {
			t.Fatal(err)
		}
		if relinked.Pairs[0].LinkStatus != application.TradeInStatusCreated || relinked.Pairs[0].LinkID == pair.LinkID {
			t.Fatalf("re-linking reused the cancelled link: %+v", relinked.Pairs)
		}
		if f.effectiveKind(t, new7.ID, domain.AssetEventPurchase) != 1 || f.effectiveKind(t, old6.ID, domain.AssetEventSale) != 1 {
			t.Fatal("re-linking repeated money")
		}
		reactivated, err := f.lifecycle.TimelinePage(ctx, f.owner, new7.ID, application.EventListOptions{
			Page: 1, PageSize: 25, Type: string(domain.AssetEventTradeInSource),
		})
		if err != nil || reactivated.Total != 1 || len(reactivated.Events) != 1 || reactivated.Events[0].ID != relinked.Pairs[0].SourceEventID {
			t.Fatalf("the effective view must show only the re-linked pair: %+v %v", reactivated, err)
		}
		f.assertTotalsUnchanged(t, before, old6.ID, new7.ID)
	})

	// Correction voids and replaces the pair under the same stable link ID, never
	// touching money, keeps the lineage on the same asset, and rejects a colliding
	// pair.
	t.Run("correct link", func(t *testing.T) {
		old7, new8 := f.asset(t, "old7"), f.asset(t, "new8")
		repairTarget, old13 := f.asset(t, "repairTarget"), f.asset(t, "old13")
		created, err := manager.RecordTradeIn(ctx, f.owner, "trade-in-correct-source", application.TradeInCommand{
			CurrentAssetID: old7.ID, Direction: domain.TradeInDirectionDestination,
			Current:      application.EconomicSelection{ExistingEventID: f.old7Sale},
			Counterparts: []application.EconomicSelection{{AssetID: new8.ID, ExistingEventID: f.new8Purchase}},
			OccurredAt:   f.at(20),
		})
		if err != nil {
			t.Fatal(err)
		}
		pair := created.Pairs[0]
		before := f.totals(t, old7.ID, new8.ID, repairTarget.ID)
		// A target without a valid sale is refused: a correction never creates money.
		unacquiredEvents := f.eventCount(t, repairTarget.ID)
		if _, err := manager.CorrectTradeInLink(ctx, f.owner, "trade-in-correct-unacquired", application.CorrectTradeInLinkCommand{
			LinkID: pair.LinkID, ExpectedSourceEventID: pair.SourceEventID, ExpectedDestinationEventID: pair.DestinationEventID,
			NewAssetID: new8.ID, OldAssetID: repairTarget.ID, OccurredAt: f.at(21),
		}); err == nil {
			t.Fatal("a target without a valid sale was accepted")
		} else {
			fail(t, "unacquired correction target", err, "validation.trade_in_target_sale")
		}
		if f.eventCount(t, repairTarget.ID) != unacquiredEvents {
			t.Fatal("a rejected correction wrote money")
		}
		corrected, err := manager.CorrectTradeInLink(ctx, f.owner, "trade-in-correct", application.CorrectTradeInLinkCommand{
			LinkID: pair.LinkID, ExpectedSourceEventID: pair.SourceEventID, ExpectedDestinationEventID: pair.DestinationEventID,
			NewAssetID: new8.ID, OldAssetID: old7.ID, OccurredAt: f.at(21), ExternalReference: "TRADE-R", Notes: "date corrected",
		})
		if err != nil {
			t.Fatal(err)
		}
		if corrected.Pairs[0].LinkID != pair.LinkID || corrected.Pairs[0].LinkStatus != application.TradeInLinkStatusCorrected {
			t.Fatalf("correction changed the stable link ID: %+v", corrected.Pairs)
		}
		newPair := corrected.Pairs[0]
		if newPair.SourceEventID == pair.SourceEventID || newPair.DestinationEventID == pair.DestinationEventID {
			t.Fatal("correction replaced no paired event")
		}
		// Both endpoints kept their asset, so each replacement names its own
		// predecessor and never a cross-asset event.
		for _, expectation := range []struct{ got, want string }{
			{newPair.SourceEventID, pair.SourceEventID},
			{newPair.DestinationEventID, pair.DestinationEventID},
		} {
			replacement, err := store.GetAssetEvent(ctx, f.owner.TenantID, expectation.got)
			if err != nil {
				t.Fatal(err)
			}
			if replacement.ReplacesEventID != expectation.want {
				t.Fatalf("same-asset replacement lineage mismatch: %+v", replacement)
			}
		}
		voided, err := store.GetAssetEvent(ctx, f.owner.TenantID, pair.SourceEventID)
		if err != nil || !voided.IsVoided {
			t.Fatalf("correction did not void the original: %+v %v", voided, err)
		}
		links, err := store.TradeInLinks(ctx, f.owner.TenantID, new8.ID, false)
		if err != nil || len(links) != 1 || links[0].SourceEventID != newPair.SourceEventID || links[0].DestinationEventID != newPair.DestinationEventID {
			t.Fatalf("corrected link projection mismatch: %+v %v", links, err)
		}
		// The replacement pair keeps the association's own order reference, so an
		// edit form can prefill it instead of dropping it.
		correctedSource, err := store.GetAssetEvent(ctx, f.owner.TenantID, newPair.SourceEventID)
		if err != nil {
			t.Fatal(err)
		}
		if correctedSource.ExternalReference != "TRADE-R" || correctedSource.Source != "manual" {
			t.Fatalf("a correction must project its association reference: %+v", correctedSource)
		}
		// Changing only the old endpoint keeps the source lineage and leaves the
		// new destination without a cross-asset replacement ID.
		moved, err := manager.CorrectTradeInLink(ctx, f.owner, "trade-in-correct-move", application.CorrectTradeInLinkCommand{
			LinkID: newPair.LinkID, ExpectedSourceEventID: newPair.SourceEventID, ExpectedDestinationEventID: newPair.DestinationEventID,
			NewAssetID: new8.ID, OldAssetID: old13.ID, OccurredAt: f.at(22),
		})
		if err != nil {
			t.Fatalf("moving the old endpoint failed: %v", err)
		}
		movedPair := moved.Pairs[0]
		movedSource, err := store.GetAssetEvent(ctx, f.owner.TenantID, movedPair.SourceEventID)
		if err != nil {
			t.Fatal(err)
		}
		if movedSource.ReplacesEventID != newPair.SourceEventID {
			t.Fatalf("a same-asset source must keep its lineage: %+v", movedSource)
		}
		movedDestination, err := store.GetAssetEvent(ctx, f.owner.TenantID, movedPair.DestinationEventID)
		if err != nil {
			t.Fatal(err)
		}
		if movedDestination.ReplacesEventID != "" || movedDestination.AssetID != old13.ID {
			t.Fatalf("a changed endpoint must not claim a cross-asset replacement: %+v", movedDestination)
		}
		if _, err := manager.CorrectTradeInLink(ctx, f.owner, "trade-in-correct-stale", application.CorrectTradeInLinkCommand{
			LinkID: pair.LinkID, ExpectedSourceEventID: pair.SourceEventID, ExpectedDestinationEventID: pair.DestinationEventID,
			NewAssetID: new8.ID, OldAssetID: old7.ID, OccurredAt: f.at(21),
		}); err == nil {
			t.Fatal("a stale correction was accepted")
		} else {
			fail(t, "stale correction", err, "validation.trade_in_link_stale")
		}
		// A pair that already has an active link cannot be corrected onto it.
		if _, err := manager.CorrectTradeInLink(ctx, f.owner, "trade-in-correct-collision", application.CorrectTradeInLinkCommand{
			LinkID: movedPair.LinkID, ExpectedSourceEventID: movedPair.SourceEventID, ExpectedDestinationEventID: movedPair.DestinationEventID,
			NewAssetID: f.asset(t, "new1").ID, OldAssetID: f.asset(t, "old1").ID, OccurredAt: f.at(22),
		}); err == nil {
			t.Fatal("a colliding correction was accepted")
		} else {
			fail(t, "colliding correction", err, "validation.trade_in_link_exists")
		}
		f.assertTotalsUnchanged(t, before, old7.ID, new8.ID, repairTarget.ID)
	})

	// One endpoint purge keeps the surviving history with its snapshot and no live
	// link, in both directions, and editing the broken pair is refused.
	t.Run("purge keeps surviving history", func(t *testing.T) {
		old15, new15, old16, new16 := f.asset(t, "old15"), f.asset(t, "new15"), f.asset(t, "old16"), f.asset(t, "new16")
		first, err := manager.RecordTradeIn(ctx, f.owner, "trade-in-purge-old", application.TradeInCommand{
			CurrentAssetID: old15.ID, Direction: domain.TradeInDirectionDestination,
			Current:      application.EconomicSelection{ExistingEventID: f.old15Sale},
			Counterparts: []application.EconomicSelection{{AssetID: new15.ID, ExistingEventID: f.new15Purchase}},
			OccurredAt:   f.at(22),
		})
		if err != nil {
			t.Fatal(err)
		}
		second, err := manager.RecordTradeIn(ctx, f.owner, "trade-in-purge-new", application.TradeInCommand{
			CurrentAssetID: old16.ID, Direction: domain.TradeInDirectionDestination,
			Current:      application.EconomicSelection{ExistingEventID: f.old16Sale},
			Counterparts: []application.EconomicSelection{{AssetID: new16.ID, ExistingEventID: f.new16Purchase}},
			OccurredAt:   f.at(22),
		})
		if err != nil {
			t.Fatal(err)
		}
		// Purge the OLD endpoint: the source event on the new asset survives.
		if err := application.NewSpecificationService(store).DeleteAsset(ctx, f.owner, old15.ID); err != nil {
			t.Fatalf("purge trade-in old endpoint: %v", err)
		}
		surviving, err := store.GetAssetEvent(ctx, f.owner.TenantID, first.Pairs[0].SourceEventID)
		if err != nil {
			t.Fatalf("the surviving endpoint lost its history: %v", err)
		}
		if label, deleted := surviving.RelatedAssetLabel(); label != "Old fifteen" || !deleted {
			t.Fatalf("surviving snapshot mismatch: %q %v", label, deleted)
		}
		if surviving.TradeInState != domain.TradeInStateActive || surviving.IsVoided {
			t.Fatalf("surviving state mismatch: %+v", surviving)
		}
		broken, err := store.TradeInLinks(ctx, f.owner.TenantID, new15.ID, true)
		if err != nil || len(broken) != 1 {
			t.Fatalf("the surviving relation must stay visible: %+v %v", broken, err)
		}
		if !broken[0].OldAssetDeleted || broken[0].OldAssetID != old15.ID || broken[0].OldAssetName != "Old fifteen" ||
			broken[0].OldAssetSpec != fixtureTagSummary || broken[0].NewAssetDeleted {
			t.Fatalf("surviving relation projection mismatch: %+v", broken[0])
		}
		if broken[0].SourceEventID != first.Pairs[0].SourceEventID {
			t.Fatalf("surviving relation lost its event: %+v", broken[0])
		}
		if _, err := manager.CancelTradeInLink(ctx, f.owner, "trade-in-purge-cancel", application.CancelTradeInLinkCommand{
			LinkID: first.Pairs[0].LinkID, ExpectedSourceEventID: first.Pairs[0].SourceEventID, ExpectedDestinationEventID: first.Pairs[0].DestinationEventID, OccurredAt: f.at(23),
		}); err == nil {
			t.Fatal("cancelling a broken pair was accepted")
		} else {
			fail(t, "cancelling a broken pair", err, "validation.trade_in_asset_deleted")
		}
		if f.eventCount(t, old15.ID) != 0 {
			t.Fatal("the purged endpoint was recreated")
		}
		// Purge the NEW endpoint: the destination event on the old asset survives.
		if err := application.NewSpecificationService(store).DeleteAsset(ctx, f.owner, new16.ID); err != nil {
			t.Fatalf("purge trade-in new endpoint: %v", err)
		}
		surviving, err = store.GetAssetEvent(ctx, f.owner.TenantID, second.Pairs[0].DestinationEventID)
		if err != nil {
			t.Fatalf("the surviving destination lost its history: %v", err)
		}
		if label, deleted := surviving.RelatedAssetLabel(); label != "New sixteen" || !deleted {
			t.Fatalf("surviving destination snapshot mismatch: %q %v", label, deleted)
		}
		broken, err = store.TradeInLinks(ctx, f.owner.TenantID, old16.ID, true)
		if err != nil || len(broken) != 1 {
			t.Fatalf("the surviving new-endpoint relation must stay visible: %+v %v", broken, err)
		}
		if !broken[0].NewAssetDeleted || broken[0].NewAssetID != new16.ID || broken[0].NewAssetName != "New sixteen" ||
			broken[0].NewAssetSpec != fixtureTagSummary || broken[0].OldAssetDeleted {
			t.Fatalf("surviving new-endpoint projection mismatch: %+v", broken[0])
		}
		if f.eventCount(t, new16.ID) != 0 {
			t.Fatal("the purged new endpoint was recreated")
		}
	})

	// A viewer may preview but not record; an editor may record without catalog
	// capability. Preview never writes.
	t.Run("authorization", func(t *testing.T) {
		old1, new1 := f.asset(t, "old1"), f.asset(t, "new1")
		cmd := application.TradeInCommand{
			CurrentAssetID: old1.ID, Direction: domain.TradeInDirectionDestination,
			Current:      application.EconomicSelection{ExistingEventID: f.old1Sale},
			Counterparts: []application.EconomicSelection{{AssetID: new1.ID, ExistingEventID: f.new1Purchase}},
			OccurredAt:   f.at(24),
		}
		viewer := f.owner
		viewer.Role = application.RoleViewer
		preview, err := manager.PreviewTradeIn(ctx, viewer, cmd)
		if err != nil || !preview.Ready || len(preview.Pairs) != 1 || preview.Pairs[0].LinkID == "" {
			t.Fatalf("viewer preview mismatch: %+v %v", preview, err)
		}
		if preview.Pairs[0].NewSelection != application.TradeInSelectionLinked || preview.Pairs[0].OldSelection != application.TradeInSelectionLinked {
			t.Fatalf("an existing relation must preview as linked: %+v", preview.Pairs[0])
		}
		before := f.eventCount(t, new1.ID)
		if _, err := manager.RecordTradeIn(ctx, viewer, "trade-in-viewer", cmd); !errors.Is(err, application.ErrForbidden) {
			t.Fatalf("viewer trade-in was not denied: %v", err)
		}
		if f.eventCount(t, new1.ID) != before {
			t.Fatal("a denied write changed history")
		}
		editor := f.owner
		editor.Role = application.RoleEditor
		old10, new10 := f.asset(t, "old10"), f.asset(t, "new10")
		f.purchase(t, "editor-purchase", old10.ID, 120_000, 25)
		f.sale(t, "editor-sale", old10.ID, 80_000, 26)
		if _, err := manager.RecordTradeIn(ctx, editor, "trade-in-editor", application.TradeInCommand{
			CurrentAssetID: old10.ID, Direction: domain.TradeInDirectionDestination,
			Current:      application.EconomicSelection{ExistingEventID: f.effectiveEvent(t, old10.ID, domain.AssetEventSale)},
			Counterparts: []application.EconomicSelection{{AssetID: new10.ID, ExistingEventID: f.new10Purchase}},
			OccurredAt:   f.at(26),
		}); err != nil {
			t.Fatalf("an editor could not record a trade-in: %v", err)
		}
	})

	// A preview of an unlinked pair reports exactly what a write still needs and
	// surfaces dates, rates and lifecycle blockers that would make a write fail.
	t.Run("preview resolves missing fields and blockers", func(t *testing.T) {
		old11, new12 := f.asset(t, "old11"), f.asset(t, "new12")
		beforeOld, beforeNew := f.eventCount(t, old11.ID), f.eventCount(t, new12.ID)
		preview, err := manager.PreviewTradeIn(ctx, f.owner, application.TradeInCommand{
			CurrentAssetID: old11.ID, Direction: domain.TradeInDirectionDestination,
			Current:      application.EconomicSelection{},
			Counterparts: []application.EconomicSelection{{AssetID: new12.ID, NewEvent: &application.RecordEvent{AmountMinor: 0, Currency: "CNY"}}},
			OccurredAt:   f.at(27),
		})
		if err != nil {
			t.Fatal(err)
		}
		if preview.Ready || len(preview.Pairs) != 1 {
			t.Fatalf("expected an incomplete preview: %+v", preview)
		}
		pair := preview.Pairs[0]
		if pair.NewSelection != application.TradeInSelectionCreate || len(pair.NewEventID) != 0 ||
			len(pair.MissingFields) != 3 || pair.MissingFields[0] != "occurred_at" || pair.MissingFields[1] != "amount" ||
			pair.MissingFields[2] != string(domain.AssetEventSale) {
			t.Fatalf("new selection mismatch: %+v", pair)
		}
		if pair.OldSelection != application.TradeInSelectionMissing {
			t.Fatalf("missing sale selection mismatch: %+v", pair)
		}
		if f.eventCount(t, old11.ID) != beforeOld || f.eventCount(t, new12.ID) != beforeNew {
			t.Fatal("preview wrote history")
		}
		// A zero date and an incomplete rate are reported instead of promising a
		// ready write.
		blocked, err := manager.PreviewTradeIn(ctx, f.owner, application.TradeInCommand{
			CurrentAssetID: old11.ID, Direction: domain.TradeInDirectionDestination,
			Current:      application.EconomicSelection{},
			Counterparts: []application.EconomicSelection{{AssetID: new12.ID, NewEvent: &application.RecordEvent{AmountMinor: 5_000, Currency: "USD"}}},
			OccurredAt:   f.at(27),
		})
		if err != nil {
			t.Fatal(err)
		}
		fields := map[string]bool{}
		for _, field := range blocked.Pairs[0].MissingFields {
			fields[field] = true
		}
		if blocked.Ready || !fields["occurred_at"] || !fields["fx"] {
			t.Fatalf("date and rate gaps must be reported: %+v", blocked.Pairs[0])
		}
		// A fully specified preview is decided by the same lifecycle policy that
		// writes it: a valid command is ready, while an unconvertible amount, a
		// negative rate or a future occurrence is refused instead of being
		// reported as incomplete and ready.
		old12 := f.asset(t, "old12")
		ready, err := manager.PreviewTradeIn(ctx, f.owner, application.TradeInCommand{
			CurrentAssetID: old12.ID, Direction: domain.TradeInDirectionDestination,
			Current:      application.EconomicSelection{ExistingEventID: f.old12Sale},
			Counterparts: []application.EconomicSelection{{AssetID: new12.ID, NewEvent: &application.RecordEvent{AmountMinor: 5_000, Currency: "CNY", OccurredAt: f.at(27)}}},
			OccurredAt:   f.at(27),
		})
		if err != nil || !ready.Ready || len(ready.Pairs) != 1 || ready.Pairs[0].NewSelection != application.TradeInSelectionCreate {
			t.Fatalf("a fully specified preview must be ready: %+v %v", ready, err)
		}
		if _, err := manager.PreviewTradeIn(ctx, f.owner, application.TradeInCommand{
			CurrentAssetID: old12.ID, Direction: domain.TradeInDirectionDestination,
			Current: application.EconomicSelection{ExistingEventID: f.old12Sale},
			Counterparts: []application.EconomicSelection{{AssetID: new12.ID, NewEvent: &application.RecordEvent{
				AmountMinor: 5_000, Currency: "USD", OccurredAt: f.at(27),
				FXConfirmed: true, FXRateScaled: -1, FXRateDate: f.at(27), FXRateSource: "test",
			}}},
			OccurredAt: f.at(27),
		}); err == nil {
			t.Fatal("a preview promised a record with a negative FX rate")
		}
		if _, err := manager.PreviewTradeIn(ctx, f.owner, application.TradeInCommand{
			CurrentAssetID: old12.ID, Direction: domain.TradeInDirectionDestination,
			Current: application.EconomicSelection{ExistingEventID: f.old12Sale},
			Counterparts: []application.EconomicSelection{{AssetID: new12.ID, NewEvent: &application.RecordEvent{
				AmountMinor: 5_000, Currency: "CNY", OccurredAt: time.Now().UTC().Add(48 * time.Hour),
			}}},
			OccurredAt: f.at(27),
		}); err == nil {
			t.Fatal("a preview promised a future occurrence")
		}
		if _, err := manager.PreviewTradeIn(ctx, f.owner, application.TradeInCommand{
			CurrentAssetID: old12.ID, Direction: domain.TradeInDirectionDestination,
			Current:      application.EconomicSelection{ExistingEventID: f.old12Sale},
			Counterparts: []application.EconomicSelection{{AssetID: new12.ID, NewEvent: &application.RecordEvent{AmountMinor: 5_000, Currency: "CNY", OccurredAt: f.at(27)}}},
			OccurredAt:   time.Now().UTC().Add(48 * time.Hour),
		}); err == nil {
			t.Fatal("a preview promised a future trade-in date")
		}
		// A sale with no acquisition can never be written, so the shared lifecycle
		// policy reports it as an actionable blocker, not a missing field. The
		// asset must still exist: a purged item is an unavailable asset instead.
		unacquired := f.asset(t, "old14")
		if _, err := manager.PreviewTradeIn(ctx, f.owner, application.TradeInCommand{
			CurrentAssetID: unacquired.ID, Direction: domain.TradeInDirectionDestination,
			Current:      application.EconomicSelection{NewEvent: &application.RecordEvent{AmountMinor: 1_000, Currency: "CNY", OccurredAt: f.at(27)}},
			Counterparts: []application.EconomicSelection{{AssetID: f.asset(t, "new5").ID, ExistingEventID: f.new5Purchase}},
			OccurredAt:   f.at(27),
		}); err == nil {
			t.Fatal("a preview promised an impossible sale")
		} else {
			fail(t, "preview lifecycle blocker", err, "validation.event_purchase_first")
		}
	})

	// Two concurrent commands for the same pair never duplicate money or links.
	t.Run("concurrent record", func(t *testing.T) {
		old12, new12 := f.asset(t, "old12"), f.asset(t, "new12")
		start := make(chan struct{})
		var wg sync.WaitGroup
		results := make([]application.TradeInResult, 2)
		errs := make([]error, 2)
		for index := range 2 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				results[index], errs[index] = application.NewManagementService(store, nil).RecordTradeIn(ctx, f.owner,
					fmt.Sprintf("trade-in-concurrent-%d", index), application.TradeInCommand{
						CurrentAssetID: old12.ID, Direction: domain.TradeInDirectionDestination,
						Current:      application.EconomicSelection{ExistingEventID: f.old12Sale},
						Counterparts: []application.EconomicSelection{{AssetID: new12.ID, NewEvent: &application.RecordEvent{AmountMinor: 300_000, Currency: "CNY", OccurredAt: f.at(28)}}},
						OccurredAt:   f.at(28),
					})
			}()
		}
		close(start)
		wg.Wait()
		succeeded, linkIDs := 0, map[string]bool{}
		for index := range 2 {
			if errs[index] != nil {
				var inputErr application.InputError
				if !errors.As(errs[index], &inputErr) || inputErr.Code != "validation.trade_in_event_exists" {
					t.Fatalf("unexpected concurrent failure: %v", errs[index])
				}
				continue
			}
			succeeded++
			linkIDs[results[index].Pairs[0].LinkID] = true
		}
		if succeeded == 0 {
			t.Fatal("both concurrent trade-ins failed")
		}
		if len(linkIDs) != 1 {
			t.Fatalf("concurrent trade-ins produced diverging links: %+v", linkIDs)
		}
		if f.effectiveKind(t, new12.ID, domain.AssetEventPurchase) != 1 {
			t.Fatal("concurrent trade-ins recorded duplicate money")
		}
		if links, err := store.TradeInLinks(ctx, f.owner.TenantID, new12.ID, false); err != nil || len(links) != 1 {
			t.Fatalf("concurrent trade-ins produced %d links: %v", len(links), err)
		}
	})
}

// RunTradeInRetries uses two independently opened connections to the same
// database, which is how a retry after a dropped response reaches the service.
func RunTradeInRetries(t *testing.T, first, second ManagementStore) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	f := newTradeInFixture(t, ctx, first)
	manager := application.NewManagementService(first, nil)
	old1, new1 := f.asset(t, "old1"), f.asset(t, "new1")
	cmd := application.TradeInCommand{
		CurrentAssetID: old1.ID, Direction: domain.TradeInDirectionDestination,
		Current:      application.EconomicSelection{ExistingEventID: f.old1Sale},
		Counterparts: []application.EconomicSelection{{AssetID: new1.ID, ExistingEventID: f.new1Purchase}},
		OccurredAt:   f.at(30), Notes: "retry",
	}
	created, err := manager.RecordTradeIn(ctx, f.owner, "trade-in-retry", cmd)
	if err != nil {
		t.Fatal(err)
	}
	other := application.NewManagementService(second, nil)
	replayed, err := other.RecordTradeIn(ctx, f.owner, "trade-in-retry", cmd)
	if err != nil || replayed.Pairs[0].LinkID != created.Pairs[0].LinkID || replayed.Pairs[0].SourceEventID != created.Pairs[0].SourceEventID {
		t.Fatalf("cross-connection replay changed the result: %+v %v", replayed, err)
	}
	reused, err := other.RecordTradeIn(ctx, f.owner, "trade-in-retry-other-key", cmd)
	if err != nil || reused.Pairs[0].LinkStatus != application.TradeInStatusReused || reused.Pairs[0].LinkID != created.Pairs[0].LinkID {
		t.Fatalf("cross-connection pair reuse mismatch: %+v %v", reused, err)
	}
	if f.effectiveKind(t, new1.ID, domain.AssetEventPurchase) != 1 {
		t.Fatal("cross-connection retry duplicated money")
	}
	// Concurrent same-key commands from two connections must converge on one pair.
	old2, new2 := f.asset(t, "old2"), f.asset(t, "new2")
	concurrent := application.TradeInCommand{
		CurrentAssetID: old2.ID, Direction: domain.TradeInDirectionDestination,
		Current:      application.EconomicSelection{ExistingEventID: f.old2Sale},
		Counterparts: []application.EconomicSelection{{AssetID: new2.ID, NewEvent: &application.RecordEvent{AmountMinor: 400_000, Currency: "CNY", OccurredAt: f.at(31)}}},
		OccurredAt:   f.at(31),
	}
	start := make(chan struct{})
	var wg sync.WaitGroup
	outcomes := make([]application.TradeInResult, 2)
	errs := make([]error, 2)
	for index, service := range []*application.ManagementService{manager, other} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			outcomes[index], errs[index] = service.RecordTradeIn(ctx, f.owner, "trade-in-retry-concurrent", concurrent)
		}()
	}
	close(start)
	wg.Wait()
	seen := map[string]bool{}
	for index := range 2 {
		if errs[index] != nil {
			t.Fatalf("concurrent cross-connection retry failed: %v", errs[index])
		}
		seen[outcomes[index].Pairs[0].LinkID] = true
		seen[outcomes[index].Pairs[0].SourceEventID] = true
	}
	if len(seen) != 2 {
		t.Fatalf("concurrent retry produced diverging results: %+v", outcomes)
	}
	if f.effectiveKind(t, new2.ID, domain.AssetEventPurchase) != 1 {
		t.Fatal("concurrent retry duplicated money")
	}
	// Concurrent cancellation from two connections has exactly one winner. The
	// fixture only acquired old3, so its sale is recorded through the ordinary
	// path first and reused by its effective ID, exactly as a caller would.
	old3, new3 := f.asset(t, "old3"), f.asset(t, "new3")
	f.sale(t, "retry-old3-sale", old3.ID, 90_000, 2)
	link, err := manager.RecordTradeIn(ctx, f.owner, "trade-in-retry-cancel", application.TradeInCommand{
		CurrentAssetID: old3.ID, Direction: domain.TradeInDirectionDestination,
		Current:      application.EconomicSelection{ExistingEventID: f.effectiveEvent(t, old3.ID, domain.AssetEventSale)},
		Counterparts: []application.EconomicSelection{{AssetID: new3.ID, ExistingEventID: f.new3Purchase}},
		OccurredAt:   f.at(32),
	})
	if err != nil {
		t.Fatal(err)
	}
	pair := link.Pairs[0]
	cancelCmd := application.CancelTradeInLinkCommand{
		LinkID: pair.LinkID, ExpectedSourceEventID: pair.SourceEventID, ExpectedDestinationEventID: pair.DestinationEventID, OccurredAt: f.at(33),
	}
	startCancel := make(chan struct{})
	outcomes = make([]application.TradeInResult, 2)
	errs = make([]error, 2)
	for index, service := range []*application.ManagementService{manager, other} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-startCancel
			outcomes[index], errs[index] = service.CancelTradeInLink(ctx, f.owner, fmt.Sprintf("trade-in-cancel-race-%d", index), cancelCmd)
		}()
	}
	close(startCancel)
	wg.Wait()
	succeeded, stale := 0, 0
	for index := range 2 {
		if errs[index] == nil {
			succeeded++
			continue
		}
		var inputErr application.InputError
		if !errors.As(errs[index], &inputErr) || inputErr.Code != "validation.trade_in_link_stale" {
			t.Fatalf("unexpected concurrent cancellation failure: %v", errs[index])
		}
		stale++
	}
	if succeeded != 1 || stale != 1 {
		t.Fatalf("concurrent cancellation: %d successes, %d stale", succeeded, stale)
	}
	cancelled := 0
	events, err := first.ListAssetEvents(ctx, f.owner.TenantID, new3.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if event.TradeInLinkID == pair.LinkID && !event.IsVoided && event.TradeInState == domain.TradeInStateCancelled {
			cancelled++
		}
	}
	if cancelled != 1 {
		t.Fatalf("concurrent cancellation wrote %d cancelled pairs", cancelled)
	}
	if links, err := first.TradeInLinks(ctx, f.owner.TenantID, new3.ID, false); err != nil || len(links) != 0 {
		t.Fatalf("cancelled pair stayed effective: %+v %v", links, err)
	}
}

// failingTradeInStore injects a receipt failure inside the management transaction
// so the whole trade-in must roll back.
type failingTradeInStore struct{ ManagementStore }

func (s failingTradeInStore) WithManagementWrite(ctx context.Context, tenant string, fn func(application.ManagementStore) error) error {
	return s.ManagementStore.WithManagementWrite(ctx, tenant, func(scoped application.ManagementStore) error {
		return fn(failingManagementStore{scoped})
	})
}

type failingManagementStore struct{ application.ManagementStore }

func (s failingManagementStore) SaveManagementRequest(context.Context, application.ManagementRequest) error {
	return errors.New("injected trade-in receipt failure")
}

func mustErr(_ application.TradeInResult, err error) error { return err }

func pointer(value string) *string { return &value }

// tradeInRequestKey turns a human-readable guard label into a stable ASCII
// request key so a guard test exercises its intended invariant instead of the
// production key validator. Production key validation is never weakened.
func tradeInRequestKey(label string) string {
	mapped := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			return r
		case r >= 'A' && r <= 'Z':
			return r + ('a' - 'A')
		default:
			return '-'
		}
	}, label)
	return "trade-in-guard-" + strings.Trim(mapped, "-")
}

// fixtureTagSummary is the specification label every trade-in fixture asset
// carries, so relation snapshots assert a real tag summary rather than a model
// name with a random fixture suffix.
const fixtureTagSummary = "Like new"

// tradeInFixture owns the tenant-scoped synthetic assets and recorded money used
// by the trade-in suite. Every endpoint keeps its own economic history because a
// trade-in never creates money that is already there.
type tradeInFixture struct {
	owner         application.Principal
	store         ManagementStore
	lifecycle     *application.LifecycleService
	spec          *application.SpecificationService
	token         string
	assets        map[string]domain.Asset
	old1Sale      string
	old2Sale      string
	old5Sale      string
	old6Sale      string
	old7Sale      string
	old9Sale      string
	old12Sale     string
	old13Sale     string
	old15Sale     string
	old16Sale     string
	new1Purchase  string
	new3Purchase  string
	new4Purchase  string
	new5Purchase  string
	new6Purchase  string
	new7Purchase  string
	new8Purchase  string
	new9Purchase  string
	new10Purchase string
	new13Purchase string
	new14Purchase string
	new15Purchase string
	new16Purchase string
	voidedSale    string
	old1Link      domain.AssetEvent
}

func newTradeInFixture(t *testing.T, ctx context.Context, store ManagementStore) *tradeInFixture {
	t.Helper()
	owner, err := store.FirstPrincipal(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// Every fixture needs its own category and lifecycle request keys, because the
	// shared conformance tenant is reused by the retry suite.
	token := fmt.Sprintf("fx%d", time.Now().UnixNano())
	catalog := application.NewCatalogService(store)
	category, err := catalog.CreateCategory(ctx, owner, application.CreateCategory{Name: "Trade-in fixture " + token})
	if err != nil {
		t.Fatal(err)
	}
	model, err := catalog.CreateModel(ctx, owner, application.CreateModel{CategoryID: category.ID, Name: "Trade-in phone " + token})
	if err != nil {
		t.Fatal(err)
	}
	spec := application.NewSpecificationService(store)
	kind, err := spec.SaveType(ctx, owner, application.SaveSpecificationType{Name: "Trade-in spec " + token, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	condition, err := spec.SaveTag(ctx, owner, application.SaveSpecificationTag{TypeID: kind.ID, Name: fixtureTagSummary, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := spec.SaveModel(ctx, owner, application.SaveModelSpecification{ModelID: model.ID, TagIDs: []string{condition.ID}}); err != nil {
		t.Fatal(err)
	}
	f := &tradeInFixture{
		owner: owner, store: store, lifecycle: application.NewLifecycleService(store),
		spec: spec, token: token, assets: map[string]domain.Asset{},
	}
	for _, name := range []string{
		"old1", "old2", "old3", "old4", "old5", "old6", "old7", "old8", "old9", "old10", "old11", "old12",
		"old13", "old14", "old15", "old16",
		"new1", "new2", "new3", "new4", "new5", "new6", "new7", "new8", "new9", "new10", "new11", "new12",
		"new13", "new14", "new15", "new16",
		"standalone", "repairTarget",
	} {
		asset, err := spec.SaveAsset(ctx, owner, application.SaveSpecificationAsset{
			ModelID: model.ID, DisplayName: tradeInLabel(name), TagIDs: []string{condition.ID},
		})
		if err != nil {
			t.Fatalf("create asset %s: %v", name, err)
		}
		f.assets[name] = asset
	}
	f.purchase(t, "fixture-old1-purchase", f.asset(t, "old1").ID, 200_000, 1)
	f.old1Sale = f.sale(t, "fixture-old1-sale", f.asset(t, "old1").ID, 150_000, 2).ID
	f.new1Purchase = f.purchase(t, "fixture-new1-purchase", f.asset(t, "new1").ID, 500_000, 1).ID

	f.purchase(t, "fixture-old2-purchase", f.asset(t, "old2").ID, 300_000, 1)
	f.old2Sale = f.sale(t, "fixture-old2-sale", f.asset(t, "old2").ID, 250_000, 2).ID
	f.new3Purchase = f.purchase(t, "fixture-new3-purchase", f.asset(t, "new3").ID, 400_000, 1).ID

	f.purchase(t, "fixture-old3-purchase", f.asset(t, "old3").ID, 100_000, 1)
	f.purchase(t, "fixture-old4-purchase", f.asset(t, "old4").ID, 110_000, 1)
	f.new4Purchase = f.purchase(t, "fixture-new4-purchase", f.asset(t, "new4").ID, 700_000, 1).ID

	f.purchase(t, "fixture-old5-purchase", f.asset(t, "old5").ID, 140_000, 1)
	f.old5Sale = f.sale(t, "fixture-old5-sale", f.asset(t, "old5").ID, 100_000, 2).ID
	f.new5Purchase = f.purchase(t, "fixture-new5-purchase", f.asset(t, "new5").ID, 450_000, 1).ID
	f.new6Purchase = f.purchase(t, "fixture-new6-purchase", f.asset(t, "new6").ID, 460_000, 1).ID

	f.purchase(t, "fixture-old6-purchase", f.asset(t, "old6").ID, 150_000, 1)
	f.old6Sale = f.sale(t, "fixture-old6-sale", f.asset(t, "old6").ID, 110_000, 2).ID
	f.new7Purchase = f.purchase(t, "fixture-new7-purchase", f.asset(t, "new7").ID, 470_000, 1).ID

	f.purchase(t, "fixture-old7-purchase", f.asset(t, "old7").ID, 160_000, 1)
	f.old7Sale = f.sale(t, "fixture-old7-sale", f.asset(t, "old7").ID, 120_000, 2).ID
	f.new8Purchase = f.purchase(t, "fixture-new8-purchase", f.asset(t, "new8").ID, 480_000, 1).ID

	f.purchase(t, "fixture-old9-purchase", f.asset(t, "old9").ID, 170_000, 1)
	f.old9Sale = f.sale(t, "fixture-old9-sale", f.asset(t, "old9").ID, 130_000, 2).ID
	f.new9Purchase = f.purchase(t, "fixture-new9-purchase", f.asset(t, "new9").ID, 490_000, 1).ID

	f.new10Purchase = f.purchase(t, "fixture-new10-purchase", f.asset(t, "new10").ID, 510_000, 1).ID
	f.purchase(t, "fixture-new11-purchase", f.asset(t, "new11").ID, 520_000, 1)

	f.purchase(t, "fixture-old12-purchase", f.asset(t, "old12").ID, 180_000, 1)
	f.old12Sale = f.sale(t, "fixture-old12-sale", f.asset(t, "old12").ID, 140_000, 2).ID

	f.purchase(t, "fixture-old13-purchase", f.asset(t, "old13").ID, 210_000, 1)
	f.old13Sale = f.sale(t, "fixture-old13-sale", f.asset(t, "old13").ID, 160_000, 2).ID
	f.new13Purchase = f.purchase(t, "fixture-new13-purchase", f.asset(t, "new13").ID, 540_000, 1).ID
	f.new14Purchase = f.purchase(t, "fixture-new14-purchase", f.asset(t, "new14").ID, 550_000, 1).ID

	f.purchase(t, "fixture-old15-purchase", f.asset(t, "old15").ID, 220_000, 1)
	f.old15Sale = f.sale(t, "fixture-old15-sale", f.asset(t, "old15").ID, 170_000, 2).ID
	f.new15Purchase = f.purchase(t, "fixture-new15-purchase", f.asset(t, "new15").ID, 560_000, 1).ID

	f.purchase(t, "fixture-old16-purchase", f.asset(t, "old16").ID, 230_000, 1)
	f.old16Sale = f.sale(t, "fixture-old16-sale", f.asset(t, "old16").ID, 180_000, 2).ID
	f.new16Purchase = f.purchase(t, "fixture-new16-purchase", f.asset(t, "new16").ID, 570_000, 1).ID

	f.purchase(t, "fixture-repair-purchase", f.asset(t, "repairTarget").ID, 90_000, 1)

	// A corrected sale leaves one voided original that must never be reused.
	f.purchase(t, "fixture-old8-purchase", f.asset(t, "old8").ID, 190_000, 1)
	voided := f.sale(t, "fixture-old8-sale", f.asset(t, "old8").ID, 150_000, 2)
	f.voidedSale = voided.ID
	if _, err := f.lifecycle.Correct(ctx, f.owner, voided.ID, application.RecordEvent{
		AmountMinor: 160_000, Currency: "CNY", OccurredAt: f.at(2), Notes: "corrected sale",
	}); err != nil {
		t.Fatalf("fixture correction: %v", err)
	}
	return f
}

func tradeInLabel(name string) string {
	switch {
	case strings.HasPrefix(name, "old"):
		return "Old " + tradeInNumberWord(name)
	case strings.HasPrefix(name, "new"):
		return "New " + tradeInNumberWord(name)
	case name == "standalone":
		return "Standalone"
	default:
		return "Repair target"
	}
}

func tradeInNumberWord(name string) string {
	words := map[string]string{
		"1": "one", "2": "two", "3": "three", "4": "four", "5": "five", "6": "six",
		"7": "seven", "8": "eight", "9": "nine", "10": "ten", "11": "eleven", "12": "twelve",
		"13": "thirteen", "14": "fourteen", "15": "fifteen", "16": "sixteen",
	}
	digits := strings.TrimLeft(name, "oldnew")
	if word, ok := words[digits]; ok {
		return word
	}
	return digits
}

func (f *tradeInFixture) at(day int) time.Time {
	return time.Date(2026, 4, day, 9, 0, 0, 0, time.UTC)
}

func (f *tradeInFixture) asset(t *testing.T, name string) domain.Asset {
	t.Helper()
	asset, ok := f.assets[name]
	if !ok {
		t.Fatalf("unknown fixture asset %q", name)
	}
	return asset
}

func (f *tradeInFixture) purchase(t *testing.T, key, assetID string, amount int64, day int) domain.AssetEvent {
	t.Helper()
	return f.record(t, key, assetID, domain.AssetEventPurchase, amount, day)
}

func (f *tradeInFixture) sale(t *testing.T, key, assetID string, amount int64, day int) domain.AssetEvent {
	t.Helper()
	return f.record(t, key, assetID, domain.AssetEventSale, amount, day)
}

func (f *tradeInFixture) record(t *testing.T, key, assetID string, kind domain.AssetEventType, amount int64, day int) domain.AssetEvent {
	t.Helper()
	// The fixture is created in tests that use background contexts with their own
	// timeout; the request key keeps retries idempotent across the shared tenant.
	event, err := f.lifecycle.Record(context.Background(), f.owner, application.RecordEvent{
		RequestKey: f.token + "-" + key, AssetID: assetID, Type: kind, AmountMinor: amount, Currency: "CNY", OccurredAt: f.at(day),
	})
	if err != nil {
		t.Fatalf("record %s %s: %v", kind, key, err)
	}
	return event
}

func (f *tradeInFixture) effectiveEvent(t *testing.T, assetID string, kind domain.AssetEventType) string {
	t.Helper()
	events, err := f.store.ListAssetEvents(context.Background(), f.owner.TenantID, assetID)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if !event.IsVoided && event.Kind() == kind {
			return event.ID
		}
	}
	t.Fatalf("asset %s has no effective %s", assetID, kind)
	return ""
}

func (f *tradeInFixture) effectiveKind(t *testing.T, assetID string, kind domain.AssetEventType) int {
	t.Helper()
	events, err := f.store.ListAssetEvents(context.Background(), f.owner.TenantID, assetID)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, event := range events {
		if !event.IsVoided && event.Kind() == kind {
			count++
		}
	}
	return count
}

func (f *tradeInFixture) eventCount(t *testing.T, assetID string) int {
	t.Helper()
	events, err := f.store.ListAssetEvents(context.Background(), f.owner.TenantID, assetID)
	if err != nil {
		t.Fatal(err)
	}
	return len(events)
}

// tradeInTotals captures the cash flow, holding days and status a neutral
// relation must never change, plus the tenant portfolio it must leave untouched.
type tradeInTotals struct {
	Net      map[string]int64
	Status   map[string]string
	Expenses map[string]int64
	Days     map[string]int64
}

func (f *tradeInFixture) totals(t *testing.T, assetIDs ...string) tradeInTotals {
	t.Helper()
	totals := tradeInTotals{Net: map[string]int64{}, Status: map[string]string{}, Expenses: map[string]int64{}, Days: map[string]int64{}}
	for _, assetID := range assetIDs {
		_, summary, err := f.lifecycle.Timeline(context.Background(), f.owner, assetID)
		if err != nil {
			t.Fatal(err)
		}
		cost, err := f.lifecycle.CostDashboard(context.Background(), f.owner, assetID)
		if err != nil {
			t.Fatal(err)
		}
		totals.Net[assetID] = summary.NetCashflowMinor
		totals.Status[assetID] = summary.Status
		totals.Expenses[assetID] = summary.ExpenseMinor
		totals.Days[assetID] = cost.Days
	}
	portfolio, err := f.lifecycle.PortfolioSummary(context.Background(), f.owner)
	if err != nil {
		t.Fatal(err)
	}
	totals.Net["portfolio"] = portfolio.NetMinor
	totals.Expenses["portfolio"] = portfolio.ExpenseMinor
	return totals
}

func (f *tradeInFixture) assertTotalsUnchanged(t *testing.T, before tradeInTotals, assetIDs ...string) {
	t.Helper()
	for _, assetID := range assetIDs {
		_, summary, err := f.lifecycle.Timeline(context.Background(), f.owner, assetID)
		if err != nil {
			t.Fatal(err)
		}
		cost, err := f.lifecycle.CostDashboard(context.Background(), f.owner, assetID)
		if err != nil {
			t.Fatal(err)
		}
		if summary.NetCashflowMinor != before.Net[assetID] || summary.ExpenseMinor != before.Expenses[assetID] || summary.Status != before.Status[assetID] || cost.Days != before.Days[assetID] {
			t.Fatalf("asset %s changed: net %d -> %d, expense %d -> %d, status %q -> %q, days %d -> %d",
				assetID, before.Net[assetID], summary.NetCashflowMinor, before.Expenses[assetID], summary.ExpenseMinor, before.Status[assetID], summary.Status, before.Days[assetID], cost.Days)
		}
	}
	portfolio, err := f.lifecycle.PortfolioSummary(context.Background(), f.owner)
	if err != nil {
		t.Fatal(err)
	}
	if portfolio.NetMinor != before.Net["portfolio"] || portfolio.ExpenseMinor != before.Expenses["portfolio"] {
		t.Fatalf("portfolio changed: net %d -> %d, expense %d -> %d",
			before.Net["portfolio"], portfolio.NetMinor, before.Expenses["portfolio"], portfolio.ExpenseMinor)
	}
}
