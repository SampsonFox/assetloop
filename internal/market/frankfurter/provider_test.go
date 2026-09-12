package frankfurter

import (
	"context"
	"errors"
	"github.com/SampsonFox/assetloop/internal/application"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestHolidayRateNeverUsesFutureAndCaches(t *testing.T) {
	calls := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		q := r.URL.Query()
		if q.Get("base") != "CNY" || q.Get("quotes") != "USD" || q.Get("to") != "2026-09-06" || q.Get("expand") != "providers" {
			t.Error(r.URL)
		}
		w.Write([]byte(`[{"date":"2026-09-07","base":"CNY","quote":"USD","rate":0.9},{"date":"2026-09-04","base":"CNY","quote":"USD","rate":0.140123456,"providers":[{"key":"ECB","date":"2026-09-04","rate":0.14},{"key":"BOC","date":"2026-09-04","rate":0.14}]},{"date":"2026-09-03","base":"CNY","quote":"USD","rate":0.13}]`))
	}))
	defer s.Close()
	p := New()
	p.endpoint = s.URL
	for i := 0; i < 2; i++ {
		r, e := p.Rate(context.Background(), "CNY", "USD", "2026-09-06")
		if e != nil || r.Scaled != 14012346 || r.Date != "2026-09-04" || r.Source != "frankfurter-v2:blended:BOC,ECB" {
			t.Fatalf("%+v %v", r, e)
		}
	}
	if calls != 1 {
		t.Fatal(calls)
	}
}
func TestUnavailableRates(t *testing.T) {
	for _, body := range []string{`[]`, `[{"date":"2026-09-09","base":"CNY","quote":"USD","rate":1}]`, `[{"date":"2026-09-08","base":"USD","quote":"CNY","rate":7}]`, `[{"date":"2026-09-08","base":"CNY","quote":"USD","rate":0}]`, `bad`} {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(body)) }))
		p := New()
		p.endpoint = s.URL
		_, e := p.Rate(context.Background(), "CNY", "USD", "2026-09-08")
		s.Close()
		if !errors.Is(e, application.ErrMarketFX) {
			t.Fatal(body, e)
		}
	}
}
func TestFixedRate(t *testing.T) {
	for input, want := range map[string]int64{"1": 100000000, "0.140123455": 14012346, "0.140123454": 14012345, "1e-2": 1000000} {
		got, e := fixedRate(input)
		if e != nil || got != want {
			t.Fatal(input, got, e)
		}
	}
	for _, s := range []string{"0", "-1", "bad", "1e30"} {
		if _, e := fixedRate(s); e == nil {
			t.Fatal(s)
		}
	}
}

func TestRecordedV2ExpandedProviders(t *testing.T) {
	b, e := os.ReadFile("testdata/rates.json")
	if e != nil {
		t.Fatal(e)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(b) }))
	defer server.Close()
	p := New()
	p.endpoint = server.URL
	rate, e := p.Rate(context.Background(), "CNY", "USD", "2026-09-08")
	if e != nil || rate.Date != "2026-09-08" || rate.Scaled != 14909000 || !strings.Contains(rate.Source, "ECB") {
		t.Fatalf("%+v %v", rate, e)
	}
}
