package modeldownload

import (
	"context"
	"errors"
	"github.com/SampsonFox/assetloop/internal/application"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"testing"
	"time"
)

type roundTrip func(*http.Request) (*http.Response, error)

func TestDNSPinnedAndMixedAnswersDenied(t *testing.T) {
	calls := 0
	dials := 0
	lookup := func(context.Context, string, string) ([]netip.Addr, error) {
		calls++
		return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
	}
	dial := func(_ context.Context, _ string, addr string) (net.Conn, error) {
		dials++
		if addr != "8.8.8.8:443" {
			t.Fatal("hostname re-resolved")
		}
		return nil, errors.New("test dial")
	}
	_, _ = dialResolved(context.Background(), "tcp", "models.example:443", lookup, dial)
	if calls != 1 || dials != 1 {
		t.Fatal("unexpected DNS/dial count")
	}
	lookup = func(context.Context, string, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("8.8.8.8"), netip.MustParseAddr("127.0.0.1")}, nil
	}
	dials = 0
	_, err := dialResolved(context.Background(), "tcp", "models.example:443", lookup, dial)
	if err == nil || dials != 0 {
		t.Fatal("mixed public/private DNS dialed")
	}
}

func TestCancellationAndTransportPolicy(t *testing.T) {
	d := New()
	tr := d.client.Transport.(*http.Transport)
	if tr.Proxy != nil || !tr.DisableCompression || !tr.DisableKeepAlives || d.client.Timeout != 30*time.Second || tr.TLSClientConfig != nil {
		t.Fatal("unsafe transport defaults")
	}
	d.client.Transport = roundTrip(func(r *http.Request) (*http.Response, error) { <-r.Context().Done(); return nil, r.Context().Err() })
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if _, err := d.Download(ctx, "https://example.com/a.glb"); err == nil {
		t.Fatal("cancellation ignored")
	}
}

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestPublicAddressPolicy(t *testing.T) {
	for _, s := range []string{"127.0.0.1", "10.0.0.1", "172.16.1.1", "192.168.1.1", "169.254.169.254", "100.100.100.200", "0.0.0.0", "192.0.2.1", "198.18.0.1", "224.0.0.1", "240.1.1.1", "::1", "::ffff:8.8.8.8", "fc00::1", "fe80::1", "64:ff9b::808:808", "2002:808:808::1", "2001:db8::1", "3fff::1"} {
		if publicIP(netip.MustParseAddr(s)) {
			t.Errorf("allowed %s", s)
		}
	}
	for _, s := range []string{"8.8.8.8", "1.1.1.1", "2606:4700:4700::1111"} {
		if !publicIP(netip.MustParseAddr(s)) {
			t.Errorf("rejected %s", s)
		}
	}
	for _, raw := range []string{"http://example.com/a.glb", "https://user:pass@example.com/a", "https://127.0.0.1/a", "https://localhost/a", "https://x.local/a", "https://example.com:8443/a", "file:///a", "https://example.com/a#fragment"} {
		u, _ := url.Parse(raw)
		if validURL(u) == nil {
			t.Errorf("allowed %s", raw)
		}
	}
}
func TestDownloadLimitsAndRedirects(t *testing.T) {
	for _, tc := range []struct {
		name                     string
		code                     int
		length                   int64
		encoding, location, body string
		ok                       bool
	}{
		{name: "body", code: 200, length: 3, body: "glb", ok: true},
		{name: "status", code: 404},
		{name: "declared size", code: 200, length: application.MaxProductModel3DBytes + 1},
		{name: "actual size", code: 200, length: -1, body: strings.Repeat("x", int(application.MaxProductModel3DBytes)+1)},
		{name: "compression", code: 200, encoding: "gzip"},
		{name: "private redirect", code: 302, location: "https://127.0.0.1/model"},
		{name: "downgrade", code: 302, location: "http://example.com/model"},
		{name: "redirect loop", code: 302, location: "https://example.com/model"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := New()
			calls := 0
			d.client.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
				calls++
				if calls > 4 {
					t.Fatal("redirect bound exceeded")
				}
				if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" || r.Header.Get("Referer") != "" {
					t.Fatal("credentials/referrer leaked")
				}
				h := make(http.Header)
				h.Set("Content-Encoding", tc.encoding)
				h.Set("Location", tc.location)
				return &http.Response{StatusCode: tc.code, Header: h, ContentLength: tc.length, Body: io.NopCloser(strings.NewReader(tc.body)), Request: r}, nil
			})
			b, err := d.Download(context.Background(), "https://example.com/model.glb")
			if (err == nil) != tc.ok {
				t.Fatalf("success=%v", err == nil)
			}
			if tc.ok && string(b) != "glb" {
				t.Fatal("body changed")
			}
		})
	}
}
