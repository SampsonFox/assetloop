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

// Currency and upstream major-unit interpretation belong to this adapter.
const DefaultCurrency = "CNY"

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
func (p *Provider) callTool(ctx context.Context, name string, args any) (json.RawMessage, string, error) {
	session := ""
	init, e := p.rpc(ctx, "initialize", map[string]any{"protocolVersion": "2025-03-26", "capabilities": map[string]any{}, "clientInfo": map[string]string{"name": "assetloop", "version": "1"}}, 1, &session, "")
	if e != nil {
		return nil, "", e
	}
	var hello struct {
		ProtocolVersion string
		ServerInfo      struct{ Version string }
	}
	if json.Unmarshal(init, &hello) != nil || hello.ProtocolVersion == "" {
		return nil, "", application.ErrMarketInvalid
	}
	if _, e = p.rpc(ctx, "notifications/initialized", nil, 0, &session, hello.ProtocolVersion); e != nil {
		return nil, "", e
	}
	result, e := p.rpc(ctx, "tools/call", map[string]any{"name": name, "arguments": args}, 2, &session, hello.ProtocolVersion)
	if e != nil {
		return nil, "", e
	}
	return result, hello.ServerInfo.Version, nil
}
func (p *Provider) FetchQuote(ctx context.Context, q application.MarketQuery) (application.MarketQuote, error) {
	result, version, err := p.callTool(ctx, "market_price", map[string]string{"keyword": q.Keyword, "filterCriteria": q.FilterCriteria})
	if err != nil {
		return application.MarketQuote{}, err
	}
	return p.parseQuote(result, version)
}
func (p *Provider) parseQuote(b []byte, version string) (application.MarketQuote, error) {
	invalid := application.ErrMarketInvalid
	values, text, err := toolData(b)
	if err != nil {
		return application.MarketQuote{}, err
	}
	if values == nil {
		values = map[string]json.RawMessage{}
		for _, line := range strings.Split(text, "\n") {
			k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
			if !ok {
				continue
			}
			if _, exists := values[k]; exists {
				return application.MarketQuote{}, invalid
			}
			values[k], _ = json.Marshal(strings.TrimSpace(v))
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
	max, e := domain.ParseMajorAmount(field("dealMaxPrice"), DefaultCurrency)
	if e != nil || max <= 0 || model == "" || len(model) > 1000 {
		return application.MarketQuote{}, invalid
	}
	quote := application.MarketQuote{ModelDesc: model, Currency: DefaultCurrency, Provider: "zhuanzhuan", ProviderVersion: version, MaxMinor: max, ObservedAt: p.now().UTC()}
	if v := field("dealMinPrice"); v != "" && v != "null" {
		n, e := domain.ParseMajorAmount(v, DefaultCurrency)
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
