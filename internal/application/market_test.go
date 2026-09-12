package application

import (
	"context"
	"github.com/SampsonFox/assetloop/internal/domain"
	"testing"
	"time"
)

type rateStub struct{ rate FXRate }

func (f rateStub) Rate(context.Context, string, string, string) (FXRate, error) { return f.rate, nil }
func TestMarketConversionMinorUnitsAndEvidence(t *testing.T) {
	for _, tt := range []struct {
		currency string
		amount   int64
		rate     int64
		want     int64
	}{{"CNY", 12345, 0, 12345}, {"JPY", 12345, 2000000000, 2469}, {"KWD", 100, 4300000, 43}, {"USD", 645800, 14000000, 90412}} {
		t.Run(tt.currency, func(t *testing.T) {
			s := NewMarketService(nil, nil, rateStub{FXRate{Scaled: tt.rate, Date: "2026-09-04", Source: "fixture"}}, MarketOptions{})
			p := domain.MarketPrice{MaxMinor: tt.amount, Currency: "CNY", BaseCurrency: tt.currency, ObservationDate: "2026-09-06"}
			if !s.convert(context.Background(), &p) || p.BaseMinor == nil || *p.BaseMinor != tt.want || p.FX == nil {
				t.Fatalf("%+v", p)
			}
			if tt.currency != "CNY" && p.FX.RateDate.Format("2006-01-02") != "2026-09-04" {
				t.Fatal(p.FX)
			}
		})
	}
	s := NewMarketService(nil, nil, rateStub{FXRate{Scaled: 100000000, Date: "2026-09-07", Source: "future"}}, MarketOptions{})
	p := domain.MarketPrice{MaxMinor: 100, Currency: "CNY", BaseCurrency: "USD", ObservationDate: "2026-09-06"}
	if s.convert(context.Background(), &p) || p.BaseMinor != nil {
		t.Fatal("future rate accepted")
	}
}
func TestMarketDateShanghai(t *testing.T) {
	if MarketDate(time.Date(2026, 9, 8, 16, 0, 0, 0, time.UTC)) != "2026-09-09" {
		t.Fatal("wrong snapshot date")
	}
}

type normalizedQuoteStub struct{ quote MarketQuote }

func (p normalizedQuoteStub) FetchQuote(context.Context, MarketQuery) (MarketQuote, error) {
	return p.quote, nil
}
func TestMarketAcceptsAdapterCurrencyWithoutPlatformDefaults(t *testing.T) {
	for _, currency := range []string{"CNY", "USD", "JPY", "KWD", "ZZZ", ""} {
		t.Run(currency, func(t *testing.T) {
			quote := MarketQuote{Provider: "fixture-platform", Currency: currency, ModelDesc: "Fixture device", MaxMinor: 12345, ObservedAt: time.Now()}
			svc := NewMarketService(nil, normalizedQuoteStub{quote}, nil, MarketOptions{})
			got, err := svc.fetch(context.Background(), MarketQuery{Keyword: "device"})
			if currency == "" || currency == "ZZZ" {
				if err == nil {
					t.Fatal("invalid currency accepted")
				}
				return
			}
			if err != nil || got.Currency != currency || got.MaxMinor != 12345 || got.Provider != "fixture-platform" {
				t.Fatalf("adapter contract changed: %+v %v", got, err)
			}
		})
	}
}
