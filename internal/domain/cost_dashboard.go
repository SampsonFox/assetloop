package domain

import (
	"errors"
	"math/big"
	"sort"
	"time"
)

type CostPoint struct {
	EventID  string
	Type     AssetEventType
	At       time.Time
	NetMinor int64
}

type CostCategory struct {
	Type          AssetEventType
	AmountMinor   int64
	PercentTenths int64
}

type CostDashboard struct {
	Currency                                        string
	ExpenseMinor, IncomeMinor, NetMinor, DailyMinor int64
	Days                                            int64
	HasDuration, Sold                               bool
	Start, End                                      time.Time
	Points                                          []CostPoint
	Categories                                      []CostCategory
}

// CalculateCostDashboard uses recorded base-currency cash flows, never market estimates.
func CalculateCostDashboard(events []AssetEvent, currency string, now time.Time, location *time.Location) (CostDashboard, error) {
	d := CostDashboard{Currency: currency}
	active := make([]AssetEvent, 0, len(events))
	for _, e := range events {
		if !e.IsVoided && e.Type != AssetEventVoid {
			active = append(active, e)
		}
	}
	sort.Slice(active, func(i, j int) bool {
		if active[i].OccurredAt.Equal(active[j].OccurredAt) {
			if active[i].CreatedAt.Equal(active[j].CreatedAt) {
				return active[i].ID < active[j].ID
			}
			return active[i].CreatedAt.Before(active[j].CreatedAt)
		}
		return active[i].OccurredAt.Before(active[j].OccurredAt)
	})
	expense, income, net := new(big.Int), new(big.Int), new(big.Int)
	groups := map[AssetEventType]*big.Int{}
	badDates := false
	for _, e := range active {
		if e.BaseCurrency != currency {
			return d, errors.New("cost currency mismatch")
		}
		amount := big.NewInt(e.BaseAmountMinor)
		if amount.Sign() < 0 {
			amount.Neg(amount)
			expense.Add(expense, amount)
			if groups[e.Type] == nil {
				groups[e.Type] = new(big.Int)
			}
			groups[e.Type].Add(groups[e.Type], amount)
		} else {
			income.Add(income, amount)
		}
		net.Sub(expense, income)
		if !expense.IsInt64() || !income.IsInt64() || !net.IsInt64() {
			return d, errors.New("cost amount overflow")
		}
		if e.OccurredAt.IsZero() || e.OccurredAt.After(now) {
			badDates = true
		}
		if e.Type == AssetEventPurchase && d.Start.IsZero() {
			d.Start = e.OccurredAt
		}
		if e.Type == AssetEventSale {
			d.Sold = true
			if d.End.IsZero() {
				d.End = e.OccurredAt
			}
		}
		if e.BaseAmountMinor != 0 {
			d.Points = append(d.Points, CostPoint{e.ID, e.Type, e.OccurredAt, net.Int64()})
		}
	}
	d.ExpenseMinor, d.IncomeMinor, d.NetMinor = expense.Int64(), income.Int64(), net.Int64()
	for kind, amount := range groups {
		share := new(big.Int).Mul(amount, big.NewInt(1000))
		share.Add(share, new(big.Int).Quo(new(big.Int).Set(expense), big.NewInt(2)))
		share.Quo(share, expense)
		d.Categories = append(d.Categories, CostCategory{kind, amount.Int64(), share.Int64()})
	}
	sort.Slice(d.Categories, func(i, j int) bool {
		if d.Categories[i].AmountMinor == d.Categories[j].AmountMinor {
			return d.Categories[i].Type < d.Categories[j].Type
		}
		return d.Categories[i].AmountMinor > d.Categories[j].AmountMinor
	})
	if !d.Sold {
		d.End = now
	}
	if d.Start.IsZero() || badDates || d.End.Before(d.Start) {
		return d, nil
	}
	if location == nil {
		location = time.Local
	}
	// Calendar dates mapped to UTC avoid 23/25-hour DST days changing the divisor.
	day := func(t time.Time) time.Time {
		y, m, d := t.In(location).Date()
		return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	}
	d.Days = (day(d.End).Unix()-day(d.Start).Unix())/86400 + 1
	if d.Days < 1 {
		return d, nil
	}
	d.HasDuration = true
	abs := new(big.Int).Abs(net)
	q, r := new(big.Int), new(big.Int)
	q.QuoRem(abs, big.NewInt(d.Days), r)
	if r.Mul(r, big.NewInt(2)).Cmp(big.NewInt(d.Days)) >= 0 {
		q.Add(q, big.NewInt(1))
	}
	if net.Sign() < 0 {
		q.Neg(q)
	}
	d.DailyMinor = q.Int64()
	return d, nil
}
