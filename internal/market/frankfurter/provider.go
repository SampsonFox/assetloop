package frankfurter

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/SampsonFox/assetloop/internal/domain"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
)

const Endpoint = "https://api.frankfurter.dev/v2/rates"

type Provider struct {
	endpoint string
	client   *http.Client
	mu       sync.Mutex
	cache    map[string]application.FXRate
}

func New() *Provider {
	return &Provider{endpoint: Endpoint, client: &http.Client{Timeout: 20 * time.Second}, cache: map[string]application.FXRate{}}
}
func (p *Provider) Rate(ctx context.Context, base, quote, date string) (application.FXRate, error) {
	var zero application.FXRate
	if _, e := domain.NormalizeCurrency(base); e != nil {
		return zero, e
	}
	if _, e := domain.NormalizeCurrency(quote); e != nil {
		return zero, e
	}
	day, e := time.Parse("2006-01-02", date)
	if e != nil {
		return zero, e
	}
	key := base + "/" + quote + "/" + date
	p.mu.Lock()
	cached, ok := p.cache[key]
	p.mu.Unlock()
	if ok {
		return cached, nil
	}
	// Include a bounded preceding window for weekends and central-bank holidays.
	params := url.Values{"base": {base}, "quotes": {quote}, "from": {day.AddDate(0, 0, -14).Format("2006-01-02")}, "to": {date}, "expand": {"providers"}}
	req, e := http.NewRequestWithContext(ctx, http.MethodGet, p.endpoint+"?"+params.Encode(), nil)
	if e != nil {
		return zero, application.ErrMarketFX
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "AssetLoop/1")
	resp, e := p.client.Do(req)
	if e != nil {
		return zero, application.ErrMarketFX
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return zero, application.ErrMarketFX
	}
	var rows []struct {
		Date, Base, Quote string
		Rate              json.Number
		Providers         []struct {
			Key, Date string
			Rate      json.Number
		}
	}
	if e = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&rows); e != nil {
		return zero, application.ErrMarketFX
	}
	var best application.FXRate
	for _, r := range rows {
		if r.Base != base || r.Quote != quote || r.Date > date || r.Date < params.Get("from") {
			continue
		}
		if _, e = time.Parse("2006-01-02", r.Date); e != nil {
			continue
		}
		n, e := fixedRate(r.Rate.String())
		if e != nil {
			continue
		}
		sources := make([]string, 0, len(r.Providers))
		for _, provider := range r.Providers {
			if provider.Key != "" {
				sources = append(sources, provider.Key)
			}
		}
		sort.Strings(sources)
		source := "frankfurter-v2:blended"
		if len(sources) > 0 {
			source += ":" + strings.Join(sources, ",")
		}
		if r.Date > best.Date {
			best = application.FXRate{Scaled: n, Date: r.Date, Source: source}
		}
	}
	if best.Scaled <= 0 {
		return zero, application.ErrMarketFX
	}
	p.mu.Lock()
	if len(p.cache) >= 512 {
		p.cache = map[string]application.FXRate{}
	}
	p.cache[key] = best
	p.mu.Unlock()
	return best, nil
}

// Round upstream decimal JSON to the domain's eight-place fixed-point rate.
func fixedRate(s string) (int64, error) {
	r, ok := new(big.Rat).SetString(s)
	if !ok || r.Sign() <= 0 {
		return 0, fmt.Errorf("invalid rate")
	}
	r.Mul(r, new(big.Rat).SetInt64(domain.FXRateScale))
	q, rem := new(big.Int), new(big.Int)
	q.QuoRem(r.Num(), r.Denom(), rem)
	if new(big.Int).Lsh(rem, 1).Cmp(r.Denom()) >= 0 {
		q.Add(q, big.NewInt(1))
	}
	if !q.IsInt64() || q.Sign() <= 0 {
		return 0, fmt.Errorf("rate out of range")
	}
	return q.Int64(), nil
}
