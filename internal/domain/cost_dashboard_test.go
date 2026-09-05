package domain

import (
	"math"
	"testing"
	"time"
)

func TestCostDashboardOverflowAndFX(t *testing.T) {
	now := time.Now()
	e := AssetEvent{Type: AssetEventPurchase, BaseAmountMinor: -100, BaseCurrency: "CNY", OccurredAt: now, FX: &FXEvidence{OriginalAmountMinor: 999999, OriginalCurrency: "USD"}}
	d, err := CalculateCostDashboard([]AssetEvent{e}, "CNY", now, time.UTC)
	if err != nil || d.NetMinor != 100 || d.DailyMinor != 100 {
		t.Fatal("must use persisted base amount")
	}
	e.BaseAmountMinor = math.MinInt64
	if _, err := CalculateCostDashboard([]AssetEvent{e}, "CNY", now, time.UTC); err == nil {
		t.Fatal("overflow must fail")
	}
	e.BaseAmountMinor = -1
	e.BaseCurrency = "USD"
	if _, err := CalculateCostDashboard([]AssetEvent{e}, "CNY", now, time.UTC); err == nil {
		t.Fatal("mixed base currency must fail")
	}
}

func TestCostDashboard(t *testing.T) {
	start := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	event := func(kind AssetEventType, amount int64, days int) AssetEvent {
		return AssetEvent{Type: kind, BaseAmountMinor: amount, BaseCurrency: "CNY", OccurredAt: start.AddDate(0, 0, days)}
	}
	purchase, repair, sale := event(AssetEventPurchase, -1000000, 0), event(AssetEventRepair, -10000, 20), event(AssetEventSale, 800000, 99)
	for _, tc := range []struct {
		name             string
		events           []AssetEvent
		days, net, daily int64
		valid            bool
	}{
		{"sold", []AssetEvent{sale, purchase, repair}, 100, 210000, 2100, true},
		{"holding", []AssetEvent{purchase, repair}, 201, 1010000, 5025, true},
		{"refund after sale", []AssetEvent{purchase, repair, sale, event("refund", 10000, 110)}, 100, 200000, 2000, true},
		{"negative", []AssetEvent{purchase, event(AssetEventSale, 1100000, 99)}, 100, -100000, -1000, true},
		{"same day", []AssetEvent{purchase, event(AssetEventSale, 999999, 0)}, 1, 1, 1, true},
		{"no purchase", []AssetEvent{repair}, 0, 10000, 0, false},
		{"future", []AssetEvent{event(AssetEventPurchase, -100, 300)}, 0, 100, 0, false},
		{"sale before purchase", []AssetEvent{purchase, event(AssetEventSale, 100, -1)}, 0, 999900, 0, false},
		{"empty", nil, 0, 0, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, err := CalculateCostDashboard(tc.events, "CNY", start.AddDate(0, 0, 200), time.UTC)
			if err != nil || d.NetMinor != tc.net || d.Days != tc.days || d.DailyMinor != tc.daily || d.HasDuration != tc.valid {
				t.Fatalf("got %+v err %v", d, err)
			}
		})
	}
	repair.IsVoided = true
	d, err := CalculateCostDashboard([]AssetEvent{purchase, repair, event(AssetEventVoid, 0, 21), event(AssetEventRepair, -20000, 20), sale}, "CNY", start.AddDate(0, 0, 200), time.UTC)
	if err != nil || d.NetMinor != 220000 || len(d.Points) != 3 || len(d.Categories) != 2 {
		t.Fatalf("correction: %+v %v", d, err)
	}
}

func TestCostDashboardCalendarAndRounding(t *testing.T) {
	zone := time.FixedZone("local", 8*3600)
	start := time.Date(2026, 1, 1, 23, 59, 0, 0, zone)
	for _, amount := range []int64{-1, 1} {
		d, err := CalculateCostDashboard([]AssetEvent{{Type: AssetEventPurchase, BaseAmountMinor: amount, BaseCurrency: "JPY", OccurredAt: start}}, "JPY", start.Add(2*time.Minute), zone)
		if err != nil || d.Days != 2 || d.DailyMinor != -amount {
			t.Fatalf("calendar rounding: %+v %v", d, err)
		}
	}
}
