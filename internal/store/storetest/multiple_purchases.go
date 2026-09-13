package storetest

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/SampsonFox/assetloop/internal/domain"
)

// RunCustomLifecycleCosts proves the supported cost model on one item: exactly
// one built-in purchase (the item acquisition), reusable custom expense types
// for purchased services and accessories, a custom neutral type for a gift,
// append-only reclassification of already recorded purchases, and the
// acquisition/sale guards. Fixture values and IDs are synthetic.
func RunCustomLifecycleCosts(t *testing.T, store Store) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	owner, err := store.FirstPrincipal(ctx)
	if err != nil {
		t.Fatal(err)
	}
	catalog := application.NewCatalogService(store)
	snapshot, err := catalog.Snapshot(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	modelID := snapshot.Models[0].ID
	spec := application.NewSpecificationService(store)
	service := application.NewLifecycleService(store)
	at := func(day int) time.Time { return time.Date(2026, 3, day, 12, 0, 0, 0, time.UTC) }
	var inputErr application.InputError
	fail := func(label string, err error, code string) {
		t.Helper()
		if !errors.As(err, &inputErr) || inputErr.Code != code {
			t.Fatalf("%s: want %s, got %v", label, code, err)
		}
	}
	typeID := func(systemCode string) string {
		t.Helper()
		types, err := service.EventTypes(ctx, owner)
		if err != nil {
			t.Fatal(err)
		}
		for _, kind := range types {
			if string(kind.SystemCode) == systemCode {
				return kind.ID
			}
		}
		t.Fatalf("event type %q is missing", systemCode)
		return ""
	}
	createType := func(name string, cashflow domain.AssetEventCashflow) string {
		t.Helper()
		kind, err := service.CreateEventType(ctx, owner, application.CreateAssetEventType{Name: name, Cashflow: cashflow})
		if err != nil {
			t.Fatalf("create custom type %q: %v", name, err)
		}
		return kind.ID
	}
	services, accessories, gift := createType("Synthetic service", domain.AssetEventExpense), createType("Synthetic accessory", domain.AssetEventExpense), createType("Synthetic gift", domain.AssetEventNeutral)
	purchaseType := typeID("purchase")

	// One phone purchase plus reusable custom costs, recorded in time order.
	asset, err := spec.SaveAsset(ctx, owner, application.SaveSpecificationAsset{ModelID: modelID, DisplayName: "Custom cost phone"})
	if err != nil {
		t.Fatal(err)
	}
	record := func(key string, kind string, amount int64, day int, notes string) domain.AssetEvent {
		t.Helper()
		event, err := service.Record(ctx, owner, application.RecordEvent{
			RequestKey: key, AssetID: asset.ID, TypeID: kind, AmountMinor: amount, Currency: "CNY",
			OccurredAt: at(day), Notes: notes,
		})
		if err != nil {
			t.Fatalf("record %s: %v", key, err)
		}
		return event
	}
	purchase := record("custom-cost-purchase", purchaseType, 499_900, 1, "device")
	record("custom-cost-service-1", services, 19_900, 2, "extended warranty service")
	service2 := record("custom-cost-service-2", services, 9_900, 2, "screen protection service")
	record("custom-cost-service-3", services, 5_900, 3, "setup service")
	record("custom-cost-accessory-1", accessories, 8_900, 3, "protective case")
	record("custom-cost-accessory-2", accessories, 14_900, 4, "charger")
	giftEvent := record("custom-cost-gift", gift, 0, 5, "free gift")
	if purchase.BaseAmountMinor != -499_900 || giftEvent.BaseAmountMinor != 0 {
		t.Fatalf("purchase or gift direction mismatch: %+v %+v", purchase, giftEvent)
	}
	// The built-in purchase acquires the item, so a second one is refused.
	if _, err := service.Record(ctx, owner, application.RecordEvent{
		RequestKey: "custom-cost-second-purchase", AssetID: asset.ID, TypeID: purchaseType, AmountMinor: 1_000, Currency: "CNY", OccurredAt: at(6),
	}); err == nil {
		t.Fatal("second built-in purchase was accepted")
	} else {
		fail("second purchase", err, "validation.event_purchase_exists")
	}
	// Reusing a request key with the same payload replays the event; different content conflicts.
	if retry, err := service.Record(ctx, owner, application.RecordEvent{
		RequestKey: "custom-cost-service-2", AssetID: asset.ID, TypeID: services, AmountMinor: 9_900, Currency: "CNY", OccurredAt: at(2), Notes: "screen protection service",
	}); err != nil || retry.ID != service2.ID {
		t.Fatalf("custom cost retry duplicated the record: %+v %v", retry, err)
	}
	if _, err := service.Record(ctx, owner, application.RecordEvent{
		RequestKey: "custom-cost-service-2", AssetID: asset.ID, TypeID: services, AmountMinor: 10_900, Currency: "CNY", OccurredAt: at(2), Notes: "screen protection service",
	}); err == nil {
		t.Fatal("changed payload with the same key was accepted")
	} else {
		fail("service retry conflict", err, "validation.request_conflict")
	}
	events, summary, err := service.Timeline(ctx, owner, asset.ID)
	if err != nil || len(events) != 7 || summary.ExpenseMinor != 559_400 || summary.IncomeMinor != 0 || summary.NetCashflowMinor != -559_400 || summary.Status != "active" {
		t.Fatalf("custom cost totals mismatch: events=%d summary=%+v err=%v", len(events), summary, err)
	}
	// The dashboard reports accumulated net cost as expense minus income, so its
	// NetMinor is positive here while the timeline's signed cash flow stays negative.
	cost, err := service.CostDashboard(ctx, owner, asset.ID)
	if err != nil || cost.ExpenseMinor != 559_400 || cost.NetMinor != 559_400 || len(cost.Categories) != 3 || len(cost.Points) != 6 || !cost.HasDuration {
		t.Fatalf("custom cost dashboard mismatch: %+v %v", cost, err)
	}
	grouped := map[string]int64{}
	for _, category := range cost.Categories {
		grouped[category.TypeID] = category.AmountMinor
	}
	if grouped[purchaseType] != 499_900 || grouped[services] != 35_700 || grouped[accessories] != 23_800 {
		t.Fatalf("cost grouping mismatch: %+v", grouped)
	}
	// The holding period starts at the acquisition, not at the later custom costs.
	if !cost.Start.Equal(at(1)) {
		t.Fatalf("duration did not start at the acquisition: %+v", cost.Start)
	}

	// Repair and sale keep their existing guards; custom post-sale costs stay available.
	saleAsset, err := spec.SaveAsset(ctx, owner, application.SaveSpecificationAsset{ModelID: modelID, DisplayName: "Custom cost sold phone"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Record(ctx, owner, application.RecordEvent{RequestKey: "sold-purchase", AssetID: saleAsset.ID, TypeID: purchaseType, AmountMinor: 300_000, Currency: "CNY", OccurredAt: at(1)}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Record(ctx, owner, application.RecordEvent{RequestKey: "sold-service", AssetID: saleAsset.ID, TypeID: services, AmountMinor: 3_000, Currency: "CNY", OccurredAt: at(2)}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Record(ctx, owner, application.RecordEvent{RequestKey: "sold-repair", AssetID: saleAsset.ID, TypeID: typeID("repair"), AmountMinor: 5_000, Currency: "CNY", OccurredAt: at(3)}); err != nil {
		t.Fatal(err)
	}
	sale, err := service.Record(ctx, owner, application.RecordEvent{RequestKey: "sold-sale", AssetID: saleAsset.ID, TypeID: typeID("sale"), AmountMinor: 250_000, Currency: "CNY", OccurredAt: at(4)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Record(ctx, owner, application.RecordEvent{RequestKey: "sold-sale-again", AssetID: saleAsset.ID, TypeID: typeID("sale"), AmountMinor: 1_000, Currency: "CNY", OccurredAt: at(5)}); err == nil {
		t.Fatal("second sale was accepted")
	} else {
		fail("second sale", err, "validation.event_after_sale")
	}
	if _, err := service.Record(ctx, owner, application.RecordEvent{RequestKey: "sold-purchase-again", AssetID: saleAsset.ID, TypeID: purchaseType, AmountMinor: 1_000, Currency: "CNY", OccurredAt: at(5)}); err == nil {
		t.Fatal("purchase after sale was accepted")
	}
	if _, err := service.Record(ctx, owner, application.RecordEvent{RequestKey: "sold-post-cost", AssetID: saleAsset.ID, TypeID: services, AmountMinor: 1_500, Currency: "CNY", OccurredAt: at(5)}); err != nil {
		t.Fatalf("custom post-sale cost was rejected: %v", err)
	}
	soldCost, err := service.CostDashboard(ctx, owner, saleAsset.ID)
	if err != nil || !soldCost.Sold || !soldCost.HasDuration || !soldCost.Start.Equal(at(1)) || !soldCost.End.Equal(sale.OccurredAt) {
		t.Fatalf("custom costs changed the sold holding period: %+v %v", soldCost, err)
	}

	// The previous modeling mistake recorded service and accessory purchases as
	// built-in purchases, so legacy duplicates are seeded directly through the
	// Store: the corrected application rules no longer accept a second purchase.
	seed := func(assetID, suffix string, amount int64, day int, notes string) domain.AssetEvent {
		t.Helper()
		occurred := at(day)
		event := domain.AssetEvent{
			ID: "5eed0000-0000-4000-8000-0000000000" + suffix, TenantID: owner.TenantID, AssetID: assetID,
			TransactionID: "5eed0000-0000-4000-8000-0000000100" + suffix, Type: domain.AssetEventPurchase,
			TypeID: purchaseType, SystemType: domain.AssetEventPurchase, BaseAmountMinor: amount,
			BaseCurrency: "CNY", Notes: notes, OccurredAt: occurred, CreatedByUserID: owner.UserID, CreatedAt: occurred,
		}
		transaction := domain.AssetTransaction{
			ID: event.TransactionID, TenantID: owner.TenantID, OccurredAt: occurred, Source: "legacy-fixture",
			Notes: notes, CreatedByUserID: owner.UserID, CreatedAt: occurred,
		}
		if err := store.AppendAssetEvent(ctx, transaction, event); err != nil {
			t.Fatalf("seed legacy purchase %s: %v", event.ID, err)
		}
		return event
	}
	legacy, err := spec.SaveAsset(ctx, owner, application.SaveSpecificationAsset{ModelID: modelID, DisplayName: "Legacy duplicate phone"})
	if err != nil {
		t.Fatal(err)
	}
	device, duplicate := seed(legacy.ID, "01", -499_900, 1, "legacy device purchase"), seed(legacy.ID, "02", -19_900, 2, "legacy service purchase")
	reclassified, err := service.Correct(ctx, owner, duplicate.ID, application.RecordEvent{
		TypeID: services, AmountMinor: 19_900, Currency: "CNY", OccurredAt: duplicate.OccurredAt, Notes: "extended warranty service",
	})
	if err != nil || reclassified.TypeID != services || reclassified.BaseAmountMinor != -19_900 || reclassified.ReplacesEventID != duplicate.ID {
		t.Fatalf("duplicate purchase was not reclassified: %+v %v", reclassified, err)
	}
	if kept, err := store.GetAssetEvent(ctx, owner.TenantID, duplicate.ID); err != nil || !kept.IsVoided || kept.BaseAmountMinor != -19_900 || kept.Notes != "legacy service purchase" || !kept.OccurredAt.Equal(duplicate.OccurredAt) {
		t.Fatalf("reclassification overwrote the original: %+v %v", kept, err)
	}
	// The remaining purchase can be repaired one at a time once no other purchase exists.
	if _, err := service.Correct(ctx, owner, device.ID, application.RecordEvent{
		TypeID: services, AmountMinor: 499_900, Currency: "CNY", OccurredAt: device.OccurredAt, Notes: "device cost",
	}); err != nil {
		t.Fatalf("last duplicate purchase was not reclassifiable: %v", err)
	}
	remaining, summary, err := service.Timeline(ctx, owner, legacy.ID)
	if err != nil || len(remaining) != 6 || summary.ExpenseMinor != 519_800 || summary.Status != "unacquired" {
		t.Fatalf("reclassified history mismatch: events=%d summary=%+v err=%v", len(remaining), summary, err)
	}

	// A legacy zero purchase becomes a neutral custom gift while keeping its evidence.
	gifted, err := spec.SaveAsset(ctx, owner, application.SaveSpecificationAsset{ModelID: modelID, DisplayName: "Legacy zero gift phone"})
	if err != nil {
		t.Fatal(err)
	}
	zeroGift := seed(gifted.ID, "03", 0, 1, "legacy gift purchase")
	neutral, err := service.Correct(ctx, owner, zeroGift.ID, application.RecordEvent{
		TypeID: gift, AmountMinor: 0, Currency: "CNY", OccurredAt: zeroGift.OccurredAt, Notes: "gift from family",
	})
	if err != nil || neutral.TypeID != gift || neutral.BaseAmountMinor != 0 || neutral.ReplacesEventID != zeroGift.ID {
		t.Fatalf("zero legacy purchase was not reclassified to neutral: %+v %v", neutral, err)
	}
	if kept, err := store.GetAssetEvent(ctx, owner.TenantID, zeroGift.ID); err != nil || !kept.IsVoided || kept.BaseAmountMinor != 0 || kept.Notes != "legacy gift purchase" {
		t.Fatalf("zero reclassification overwrote the original: %+v %v", kept, err)
	}

	// Scoped reclassification refuses built-in and unavailable targets, disabled
	// targets, read-only members and cash-flow direction changes.
	denied, err := spec.SaveAsset(ctx, owner, application.SaveSpecificationAsset{ModelID: modelID, DisplayName: "Reclassification denial phone"})
	if err != nil {
		t.Fatal(err)
	}
	serviceEvent, err := service.Record(ctx, owner, application.RecordEvent{
		RequestKey: "denial-service", AssetID: denied.ID, TypeID: services, AmountMinor: 1_500, Currency: "CNY", OccurredAt: at(1),
	})
	if err != nil {
		t.Fatal(err)
	}
	replace := func(key string, target string, amount int64) error {
		t.Helper()
		_, err := service.Correct(ctx, owner, serviceEvent.ID, application.RecordEvent{
			RequestKey: key, TypeID: target, AmountMinor: amount, Currency: "CNY", OccurredAt: at(1),
		})
		return err
	}
	fail("built-in target", replace("denial-builtin", purchaseType, 1_500), "validation.event_type")
	fail("unavailable custom target", replace("denial-unknown", "0f0f0f0f-0f0f-4f0f-8f0f-0f0f0f0f0f0f", 1_500), "validation.event_type")
	fail("cross direction", replace("denial-direction", createType("Synthetic refund", domain.AssetEventIncome), 1_500), "validation.event_cashflow")
	disabled := createType("Synthetic disabled fee", domain.AssetEventExpense)
	if _, err := service.SetEventTypeEnabled(ctx, owner, disabled, false); err != nil {
		t.Fatal(err)
	}
	fail("disabled target", replace("denial-disabled", disabled, 1_500), "validation.event_type_disabled")
	// A read-only member cannot reclassify, with the same no-write guarantee.
	viewer := owner
	viewer.Role = application.RoleViewer
	if _, err := service.Correct(ctx, viewer, serviceEvent.ID, application.RecordEvent{
		RequestKey: "denial-viewer", TypeID: accessories, AmountMinor: 1_500, Currency: "CNY", OccurredAt: at(1),
	}); !errors.Is(err, application.ErrForbidden) {
		t.Fatalf("viewer reclassification was not denied: %v", err)
	}
	if kept, err := store.GetAssetEvent(ctx, owner.TenantID, serviceEvent.ID); err != nil || kept.IsVoided || kept.TypeID != services {
		t.Fatalf("denied reclassification changed the original: %+v %v", kept, err)
	}
	if after, _, err := service.Timeline(ctx, owner, denied.ID); err != nil || len(after) != 1 {
		t.Fatalf("denied reclassification wrote history: %d %v", len(after), err)
	}
	// A failed receipt rolls the correction back with its receipt; the retained key
	// then succeeds on retry.
	receiptRetry := application.RecordEvent{RequestKey: "denial-receipt", TypeID: accessories, AmountMinor: 1_500, Currency: "CNY", OccurredAt: at(1), Notes: "protective case"}
	failing := application.NewLifecycleService(failedReceiptStore{LifecycleStore: store})
	if _, err := failing.Correct(ctx, owner, serviceEvent.ID, receiptRetry); err == nil {
		t.Fatal("correct receipt failure reported success")
	}
	if kept, err := store.GetAssetEvent(ctx, owner.TenantID, serviceEvent.ID); err != nil || kept.IsVoided || kept.TypeID != services {
		t.Fatalf("failed correction receipt voided the original: %+v %v", kept, err)
	}
	if _, found, err := store.FindLifecycleRequest(ctx, owner.TenantID, owner.UserID, receiptRetry.RequestKey); err != nil || found {
		t.Fatalf("failed correction receipt persisted: %v %v", found, err)
	}
	if after, _, err := service.Timeline(ctx, owner, denied.ID); err != nil || len(after) != 1 {
		t.Fatalf("failed correction receipt wrote history: %d %v", len(after), err)
	}
	if retried, err := service.Correct(ctx, owner, serviceEvent.ID, receiptRetry); err != nil || retried.TypeID != accessories || retried.BaseAmountMinor != -1_500 || retried.ReplacesEventID != serviceEvent.ID {
		t.Fatalf("reclassification retry after receipt failure: %+v %v", retried, err)
	}
	// The last acquisition cannot be reclassified away while a repair depends on it.
	guarded, err := spec.SaveAsset(ctx, owner, application.SaveSpecificationAsset{ModelID: modelID, DisplayName: "Last acquisition phone"})
	if err != nil {
		t.Fatal(err)
	}
	only := seed(guarded.ID, "04", -300_000, 1, "legacy device purchase")
	if _, err := service.Record(ctx, owner, application.RecordEvent{RequestKey: "guarded-repair", AssetID: guarded.ID, TypeID: typeID("repair"), AmountMinor: 2_000, Currency: "CNY", OccurredAt: at(2)}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Correct(ctx, owner, only.ID, application.RecordEvent{
		TypeID: services, AmountMinor: 300_000, Currency: "CNY", OccurredAt: only.OccurredAt, Notes: "device cost",
	}); err == nil {
		t.Fatal("last acquisition was reclassified away while a repair depended on it")
	} else {
		fail("last acquisition guard", err, "validation.event_purchase_first")
	}
	if kept, err := store.GetAssetEvent(ctx, owner.TenantID, only.ID); err != nil || kept.IsVoided {
		t.Fatalf("guarded reclassification voided the acquisition: %+v %v", kept, err)
	}
	if after, _, err := service.Timeline(ctx, owner, guarded.ID); err != nil || len(after) != 2 {
		t.Fatalf("guarded reclassification wrote history: %d %v", len(after), err)
	}

	// Same-key replay, conflict and concurrency on the reclassification path.
	replayAsset, err := spec.SaveAsset(ctx, owner, application.SaveSpecificationAsset{ModelID: modelID, DisplayName: "Reclassification replay phone"})
	if err != nil {
		t.Fatal(err)
	}
	first, second := seed(replayAsset.ID, "05", -100_000, 1, "legacy device purchase"), seed(replayAsset.ID, "06", -10_000, 2, "legacy service purchase")
	reclassify := application.RecordEvent{RequestKey: "reclassify-replay", TypeID: accessories, AmountMinor: 10_000, Currency: "CNY", OccurredAt: second.OccurredAt, Notes: "protective case"}
	firstReplacement, err := service.Correct(ctx, owner, second.ID, reclassify)
	if err != nil {
		t.Fatalf("reclassify duplicate: %v", err)
	}
	if replayed, err := application.NewLifecycleService(store).Correct(ctx, owner, second.ID, reclassify); err != nil || replayed.ID != firstReplacement.ID {
		t.Fatalf("reclassification retry changed the replacement: %+v %v", replayed, err)
	}
	conflict := reclassify
	conflict.TypeID = services
	if _, err := service.Correct(ctx, owner, second.ID, conflict); err == nil {
		t.Fatal("same key with a different target was accepted")
	} else {
		fail("reclassification conflict", err, "validation.request_conflict")
	}
	concurrent := application.RecordEvent{RequestKey: "reclassify-concurrent", TypeID: services, AmountMinor: 100_000, Currency: "CNY", OccurredAt: first.OccurredAt, Notes: "device cost"}
	type outcome struct {
		event domain.AssetEvent
		err   error
	}
	outcomes := make(chan outcome, 2)
	start := make(chan struct{})
	for range 2 {
		go func() {
			<-start
			event, err := application.NewLifecycleService(store).Correct(ctx, owner, first.ID, concurrent)
			outcomes <- outcome{event, err}
		}()
	}
	close(start)
	seen := map[string]bool{}
	succeeded := 0
	for range 2 {
		got := <-outcomes
		if got.err != nil {
			t.Fatalf("concurrent reclassification failed: %+v %v", got.event, got.err)
		}
		if got.event.ID == "" {
			t.Fatalf("concurrent reclassification returned no replacement: %+v", got.event)
		}
		succeeded++
		seen[got.event.ID] = true
	}
	if succeeded != 2 || len(seen) != 1 {
		t.Fatalf("concurrent reclassification: %d successes, %d replacements", succeeded, len(seen))
	}
	replaced := 0
	history, _, err := service.Timeline(ctx, owner, replayAsset.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range history {
		if event.ReplacesEventID == first.ID {
			replaced++
		}
	}
	if replaced != 1 {
		t.Fatalf("concurrent reclassification appended %d replacements", replaced)
	}
}
