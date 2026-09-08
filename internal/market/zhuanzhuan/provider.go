// Package zhuanzhuan translates the official MCP quote into application DTOs.
package zhuanzhuan

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/SampsonFox/assetloop/internal/domain"
	"io"
	"net/http"
	"strings"
	"time"
)

const Endpoint = "https://mcp.zhuanzhuan.com/zai/zai-transfer"

type Provider struct {
	token, endpoint string
	client          *http.Client
	now             func() time.Time
}

func New(token string) *Provider {
	return &Provider{token: token, endpoint: Endpoint, client: &http.Client{Timeout: 45 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, now: time.Now}
}

type envelope struct {
	ID     int
	Result json.RawMessage
	Error  json.RawMessage
}

func (p *Provider) rpc(ctx context.Context, method string, params any, id int, session *string, protocol string) (json.RawMessage, error) {
	payload := map[string]any{"jsonrpc": "2.0", "method": method}
	if params != nil {
		payload["params"] = params
	}
	if id != 0 {
		payload["id"] = id
	}
	body, e := json.Marshal(payload)
	if e != nil {
		return nil, application.ErrMarketInvalid
	}
	req, e := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(body))
	if e != nil {
		return nil, application.ErrMarketInvalid
	}
	req.Header.Set("Authorization", "Bearer "+p.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if *session != "" {
		req.Header.Set("Mcp-Session-Id", *session)
	}
	if protocol != "" {
		req.Header.Set("MCP-Protocol-Version", protocol)
	}
	resp, e := p.client.Do(req)
	if e != nil {
		return nil, application.ErrMarketTemporary
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == 401 || resp.StatusCode == 403:
		return nil, application.ErrMarketAuth
	case resp.StatusCode == 429:
		return nil, application.ErrMarketRateLimit
	case resp.StatusCode >= 500:
		return nil, application.ErrMarketTemporary
	case resp.StatusCode < 200 || resp.StatusCode >= 300:
		return nil, application.ErrMarketInvalid
	}
	if v := resp.Header.Get("Mcp-Session-Id"); v != "" {
		*session = v
	}
	if id == 0 {
		return nil, nil
	}
	decode := func(b []byte) (json.RawMessage, bool, error) {
		var v envelope
		if json.Unmarshal(b, &v) != nil {
			return nil, false, application.ErrMarketInvalid
		}
		if v.ID != id {
			return nil, false, nil
		}
		if len(v.Error) > 0 && string(v.Error) != "null" {
			return nil, true, application.ErrMarketInvalid
		}
		if len(v.Result) == 0 {
			return nil, true, application.ErrMarketInvalid
		}
		return v.Result, true, nil
	}
	reader := io.LimitReader(resp.Body, 1<<20)
	if strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		scanner := bufio.NewScanner(reader)
		scanner.Buffer(make([]byte, 4096), 1<<20)
		var data []string
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "data:") {
				data = append(data, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
			}
			if line == "" && len(data) > 0 {
				b, match, e := decode([]byte(strings.Join(data, "\n")))
				data = nil
				if e != nil || match {
					return b, e
				}
			}
		}
		if len(data) > 0 {
			b, match, e := decode([]byte(strings.Join(data, "\n")))
			if e != nil || match {
				return b, e
			}
		}
		return nil, application.ErrMarketInvalid
	}
	b, e := io.ReadAll(reader)
	if e != nil {
		return nil, application.ErrMarketTemporary
	}
	out, match, e := decode(b)
	if e != nil {
		return nil, e
	}
	if !match {
		return nil, application.ErrMarketInvalid
	}
	return out, nil
}
func (p *Provider) FetchQuote(ctx context.Context, q application.MarketQuery) (application.MarketQuote, error) {
	var zero application.MarketQuote
	session := ""
	init, e := p.rpc(ctx, "initialize", map[string]any{"protocolVersion": "2025-03-26", "capabilities": map[string]any{}, "clientInfo": map[string]string{"name": "assetloop", "version": "1"}}, 1, &session, "")
	if e != nil {
		return zero, e
	}
	var hello struct {
		ProtocolVersion string
		ServerInfo      struct{ Version string }
	}
	if json.Unmarshal(init, &hello) != nil || hello.ProtocolVersion == "" {
		return zero, application.ErrMarketInvalid
	}
	if _, e = p.rpc(ctx, "notifications/initialized", nil, 0, &session, hello.ProtocolVersion); e != nil {
		return zero, e
	}
	result, e := p.rpc(ctx, "tools/call", map[string]any{"name": "market_price", "arguments": map[string]string{"keyword": q.Keyword, "filterCriteria": q.FilterCriteria}}, 2, &session, hello.ProtocolVersion)
	if e != nil {
		return zero, e
	}
	return p.parseQuote(result, hello.ServerInfo.Version)
}
func (p *Provider) parseQuote(b []byte, version string) (application.MarketQuote, error) {
	var result struct {
		IsError           bool
		StructuredContent json.RawMessage
		Content           []struct{ Type, Text string }
	}
	invalid := application.ErrMarketInvalid
	if json.Unmarshal(b, &result) != nil || result.IsError {
		return application.MarketQuote{}, invalid
	}
	values := map[string]json.RawMessage{}
	ingest := func(raw []byte) bool {
		var obj map[string]json.RawMessage
		if json.Unmarshal(raw, &obj) != nil {
			return false
		}
		if code, ok := obj["code"]; ok && string(code) != "0" {
			return false
		}
		if data, ok := obj["data"]; ok {
			if json.Unmarshal(data, &obj) != nil {
				return false
			}
		}
		for k, v := range obj {
			values[k] = v
		}
		return true
	}
	if len(result.StructuredContent) > 0 {
		ingest(result.StructuredContent)
	}
	for _, c := range result.Content {
		if c.Type != "text" {
			continue
		}
		if ingest([]byte(c.Text)) {
			continue
		}
		for _, line := range strings.Split(c.Text, "\n") {
			k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
			if ok {
				values[k], _ = json.Marshal(strings.TrimSpace(v))
			}
		}
	}
	field := func(k string) string {
		b := values[k]
		var s string
		if json.Unmarshal(b, &s) == nil {
			return s
		}
		return string(b)
	}
	model := strings.TrimSpace(field("modelDesc"))
	max, e := domain.ParseMajorAmount(field("dealMaxPrice"), "CNY")
	if e != nil || max <= 0 || model == "" || len(model) > 1000 {
		return application.MarketQuote{}, invalid
	}
	quote := application.MarketQuote{ModelDesc: model, Currency: "CNY", Provider: "zhuanzhuan", ProviderVersion: version, MaxMinor: max, ObservedAt: p.now().UTC()}
	if v := field("dealMinPrice"); v != "" && v != "null" {
		n, e := domain.ParseMajorAmount(v, "CNY")
		if e != nil || n <= 0 || n > max {
			return application.MarketQuote{}, invalid
		}
		quote.MinMinor = &n
	}
	// Whitelist evidence; provider text, links and error bodies may contain credentials.
	evidence := map[string]string{"modelDesc": model, "dealMaxPrice": field("dealMaxPrice"), "dealMinPrice": field("dealMinPrice"), "unitContract": "CNY major units"}
	encoded, _ := json.Marshal(evidence)
	quote.Evidence = string(encoded)
	if p.token != "" && (strings.Contains(quote.ModelDesc, p.token) || strings.Contains(quote.Evidence, p.token)) {
		return application.MarketQuote{}, invalid
	}
	return quote, nil
}
