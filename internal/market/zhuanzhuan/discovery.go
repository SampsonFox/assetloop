package zhuanzhuan

import (
	"context"
	"encoding/json"
	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/SampsonFox/assetloop/internal/domain"
	"strconv"
	"strings"
)

// A tool has one authoritative payload. Never combine fields from different products.
func toolData(b []byte) (map[string]json.RawMessage, string, error) {
	var r struct {
		IsError           bool
		StructuredContent json.RawMessage
		Content           []struct{ Type, Text string }
	}
	bad := application.ErrMarketInvalid
	if json.Unmarshal(b, &r) != nil || r.IsError {
		return nil, "", bad
	}
	raw := r.StructuredContent
	if len(raw) == 0 || string(raw) == "null" {
		var texts []string
		for _, c := range r.Content {
			if c.Type == "text" && strings.TrimSpace(c.Text) != "" {
				texts = append(texts, c.Text)
			}
		}
		if len(texts) != 1 {
			return nil, "", bad
		}
		raw = []byte(strings.TrimSpace(texts[0]))
		if !strings.HasPrefix(string(raw), "{") {
			if strings.HasPrefix(string(raw), "[") {
				return nil, "", bad
			}
			return nil, string(raw), nil
		}
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) != nil || obj == nil {
		return nil, "", bad
	}
	if code, ok := obj["code"]; ok && string(code) != "0" {
		return nil, "", bad
	}
	if data, ok := obj["data"]; ok {
		if string(data) == "null" {
			return map[string]json.RawMessage{}, "", nil
		}
		if json.Unmarshal(data, &obj) != nil || obj == nil {
			return nil, "", bad
		}
	}
	return obj, "", nil
}
func stringField(obj map[string]json.RawMessage, key string) string {
	b := obj[key]
	if len(b) == 0 || string(b) == "null" {
		return ""
	}
	var s string
	if json.Unmarshal(b, &s) == nil {
		return s
	}
	return string(b)
}
func optionalPrice(v string) (*int64, error) {
	if v == "" || v == "null" {
		return nil, nil
	}
	n, e := domain.ParseMajorAmount(v, DefaultCurrency)
	if e != nil || n <= 0 {
		return nil, application.ErrMarketInvalid
	}
	return &n, nil
}
func (p *Provider) SearchProducts(ctx context.Context, q application.ProductSearchQuery) (application.ProductSearchResult, error) {
	b, _, err := p.callTool(ctx, "search", map[string]any{"keyword": q.Keyword, "filterCriteria": q.FilterCriteria, "pageToken": q.PageToken})
	if err != nil {
		return application.ProductSearchResult{}, err
	}
	return p.parseProducts(b)
}
func (p *Provider) parseProducts(b []byte) (application.ProductSearchResult, error) {
	var out application.ProductSearchResult
	obj, _, err := toolData(b)
	if err != nil || obj == nil {
		return out, application.ErrMarketInvalid
	}
	if len(obj) == 0 {
		return out, nil
	}
	var rows []map[string]json.RawMessage
	if json.Unmarshal(obj["items"], &rows) != nil {
		return out, application.ErrMarketInvalid
	}
	if len(rows) > 100 {
		return out, application.ErrMarketInvalid
	}
	for _, r := range rows {
		id := stringField(r, "infoId")
		if n, e := strconv.ParseInt(id, 10, 64); e != nil || n <= 0 {
			return out, application.ErrMarketInvalid
		}
		price, e := optionalPrice(stringField(r, "price"))
		if e != nil {
			return out, e
		}
		item := application.MarketProduct{Reference: application.ProductReference{ID: id, Metric: stringField(r, "metric"), BusinessType: stringField(r, "businessType")}, Provider: "zhuanzhuan", Title: stringField(r, "title"), Currency: DefaultCurrency, PriceMinor: price}
		if strings.TrimSpace(item.Title) == "" {
			return out, application.ErrMarketInvalid
		}
		out.Items = append(out.Items, item)
	}
	if v := obj["pageNo"]; len(v) > 0 && json.Unmarshal(v, &out.PageNo) != nil {
		return out, application.ErrMarketInvalid
	}
	if v := obj["hasNext"]; len(v) > 0 && json.Unmarshal(v, &out.HasNext) != nil {
		return out, application.ErrMarketInvalid
	}
	out.NextPageToken = stringField(obj, "nextPageToken")
	if out.HasNext && (out.NextPageToken == "" || out.PageNo >= 10) {
		out.HasNext = false
	}
	if p.containsToken(out) {
		return application.ProductSearchResult{}, application.ErrMarketInvalid
	}
	return out, nil
}
func (p *Provider) GetProductDetail(ctx context.Context, ref application.ProductReference) (application.MarketProductDetail, error) {
	id, err := strconv.ParseInt(ref.ID, 10, 64)
	if err != nil || id <= 0 {
		return application.MarketProductDetail{}, application.ErrMarketInvalid
	}
	b, _, err := p.callTool(ctx, "product_detail", map[string]any{"infoId": id, "metric": ref.Metric, "businessType": ref.BusinessType})
	if err != nil {
		return application.MarketProductDetail{}, err
	}
	return p.parseDetail(b, ref)
}
func (p *Provider) parseDetail(b []byte, ref application.ProductReference) (application.MarketProductDetail, error) {
	out := application.MarketProductDetail{Currency: DefaultCurrency, Selection: domain.MarketSelection{Provider: "zhuanzhuan", ProductID: ref.ID, BusinessType: ref.BusinessType, ObservedAt: p.now().UTC()}}
	obj, txt, err := toolData(b)
	if err != nil {
		return out, err
	}
	if obj == nil {
		obj = map[string]json.RawMessage{}
		var current *domain.MarketSpecification
		for _, line := range strings.Split(txt, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			if strings.HasPrefix(line, "spec=") {
				name, value, ok := strings.Cut(strings.TrimPrefix(line, "spec="), "|")
				if !ok {
					return out, application.ErrMarketInvalid
				}
				out.Selection.Specifications = append(out.Selection.Specifications, domain.MarketSpecification{Name: strings.TrimSpace(name), Value: strings.TrimSpace(value)})
				current = &out.Selection.Specifications[len(out.Selection.Specifications)-1]
				continue
			}
			if strings.HasPrefix(line, "|") && current != nil {
				current.Explanations = append(current.Explanations, strings.TrimSpace(strings.TrimPrefix(line, "|")))
				continue
			}
			if k, v, ok := strings.Cut(line, "="); ok {
				current = nil
				if k == "title" || k == "price" || k == "chengSe" {
					if _, exists := obj[k]; exists {
						return out, application.ErrMarketInvalid
					}
					obj[k], _ = json.Marshal(v)
				}
			}
		}
	} else if v := obj["specifications"]; len(v) > 0 && string(v) != "null" {
		if json.Unmarshal(v, &out.Selection.Specifications) != nil {
			return out, application.ErrMarketInvalid
		}
	}
	out.Selection.Title = stringField(obj, "title")
	out.Selection.Condition = stringField(obj, "chengSe")
	if strings.TrimSpace(out.Selection.Title) == "" {
		return out, application.ErrMarketProductGone
	}
	out.PriceMinor, err = optionalPrice(stringField(obj, "price"))
	if p.containsToken(out) {
		return application.MarketProductDetail{}, application.ErrMarketInvalid
	}
	return out, err
}
func (p *Provider) containsToken(v any) bool {
	if p.token == "" {
		return false
	}
	b, _ := json.Marshal(v)
	return strings.Contains(string(b), p.token)
}
