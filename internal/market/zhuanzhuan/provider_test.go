package zhuanzhuan

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/SampsonFox/assetloop/internal/application"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestRecordedQuoteAndFormats(t *testing.T) {
	b, e := os.ReadFile("testdata/market_price.json")
	if e != nil {
		t.Fatal(e)
	}
	p := New("fixture-credential")
	q, e := p.parseQuote(b, "1.0.0")
	if e != nil || q.MaxMinor != 645800 || q.MinMinor == nil || *q.MinMinor != 345200 || q.ModelDesc != "iPhone 15 Pro 256G" || q.Currency != "CNY" || q.SourceDate != nil || q.SampleCount != nil {
		t.Fatalf("%+v %v", q, e)
	}
	for _, body := range []string{`{"structuredContent":{"data":{"modelDesc":"phone","dealMaxPrice":12.34}}}`, `{"content":[{"type":"text","text":"{\"modelDesc\":\"phone\",\"dealMaxPrice\":\"12.34\",\"jumpUrl\":\"fixture-credential\"}"}]}`} {
		q, e = p.parseQuote([]byte(body), "1")
		if e != nil || q.MaxMinor != 1234 || strings.Contains(q.Evidence, "fixture-credential") {
			t.Fatalf("%+v %v", q, e)
		}
	}
	for _, body := range []string{`{}`, `{"isError":true}`, `{"structuredContent":{"modelDesc":"phone","dealMaxPrice":"bad"}}`, `{"structuredContent":{"modelDesc":"phone","dealMaxPrice":-1}}`, `{"structuredContent":{"modelDesc":"phone","dealMaxPrice":1,"dealMinPrice":2}}`, `{"structuredContent":{"modelDesc":"fixture-credential","dealMaxPrice":1}}`, `{"structuredContent":{"modelDesc":"phone","dealMaxPrice":1.234}}`} {
		if _, e = p.parseQuote([]byte(body), "1"); !errors.Is(e, application.ErrMarketInvalid) {
			t.Fatalf("accepted %s: %v", body, e)
		}
	}
}
func TestMCPHandshakeAndSSE(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer fixture-credential" {
			t.Error("missing authorization")
		}
		var req struct {
			Method string
			ID     int
			Params json.RawMessage
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		switch req.Method {
		case "initialize":
			w.Header().Set("Mcp-Session-Id", "fixture-session")
			w.Write([]byte(`{"id":1,"result":{"protocolVersion":"2025-03-26","serverInfo":{"version":"1.0.0"}}}`))
		case "notifications/initialized":
			if r.Header.Get("Mcp-Session-Id") != "fixture-session" {
				t.Error("missing session")
			}
			w.WriteHeader(202)
		case "tools/call":
			if !strings.Contains(string(req.Params), "market_price") || !strings.Contains(string(req.Params), "256GB") {
				t.Error(string(req.Params))
			}
			w.Header().Set("Content-Type", "text/event-stream")
			w.Write([]byte("event: message\ndata: {\"id\":2,\ndata: \"result\":{\"structuredContent\":{\"modelDesc\":\"phone\",\"dealMaxPrice\":6458}}}\n\n"))
		default:
			t.Error(req.Method)
		}
	}))
	defer server.Close()
	p := New("fixture-credential")
	p.endpoint = server.URL
	q, e := p.FetchQuote(context.Background(), application.MarketQuery{Keyword: "phone", FilterCriteria: "256GB"})
	if e != nil || q.MaxMinor != 645800 || calls != 3 {
		t.Fatalf("%+v %d %v", q, calls, e)
	}
}
func TestTransportFailures(t *testing.T) {
	for status, want := range map[int]error{401: application.ErrMarketAuth, 403: application.ErrMarketAuth, 429: application.ErrMarketRateLimit, 503: application.ErrMarketTemporary, 400: application.ErrMarketInvalid} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				w.Write([]byte("do not expose fixture-credential"))
			}))
			defer server.Close()
			p := New("fixture-credential")
			p.endpoint = server.URL
			_, e := p.FetchQuote(context.Background(), application.MarketQuery{})
			if !errors.Is(e, want) {
				t.Fatal(e)
			}
		})
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(100 * time.Millisecond):
		}
	}))
	defer server.Close()
	p := New("fixture")
	p.endpoint = server.URL
	p.client.Timeout = 10 * time.Millisecond
	if _, e := p.FetchQuote(context.Background(), application.MarketQuery{}); !errors.Is(e, application.ErrMarketTemporary) {
		t.Fatal(e)
	}
}
