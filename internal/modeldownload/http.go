// Package modeldownload implements public HTTPS-only, bounded model retrieval.
package modeldownload

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"github.com/SampsonFox/assetloop/internal/application"
)

type Downloader struct{ client *http.Client }

func New() *Downloader {
	tr := &http.Transport{
		// No environment proxy or shared cookies/credentials. TLS still verifies
		// the original hostname, while DialContext connects to the validated IP.
		DialContext: publicDial, DisableCompression: true, DisableKeepAlives: true,
		TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: 10 * time.Second,
		MaxResponseHeaderBytes: 32 << 10,
	}
	return &Downloader{client: &http.Client{Transport: tr, Timeout: 30 * time.Second,
		CheckRedirect: func(r *http.Request, via []*http.Request) error {
			if len(via) > 3 {
				return application.NewInputError("validation.model_import_redirect")
			}
			if err := validURL(r.URL); err != nil {
				return err
			}
			r.Header.Del("Referer")
			return nil
		},
	}}
}

func validURL(u *url.URL) error {
	if u == nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Opaque != "" || u.Fragment != "" || (u.Port() != "" && u.Port() != "443") || len(u.String()) > 4096 {
		return application.NewInputError("validation.model_import_url")
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if !strings.Contains(host, ".") && !strings.Contains(host, ":") || host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".internal") {
		return application.NewInputError("validation.model_import_url")
	}
	if ip, err := netip.ParseAddr(host); err == nil && !publicIP(ip) {
		return application.NewInputError("validation.model_import_url")
	}
	return nil
}

// Conservatively exclude special-use ranges in addition to Go's private/local
// checks (which intentionally do not cover CGNAT, documentation or transition IPs).
var excluded = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"), netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"), netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.88.99.0/24"), netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"), netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"), netip.MustParsePrefix("2001::/23"),
	netip.MustParsePrefix("2001:db8::/32"), netip.MustParsePrefix("2002::/16"),
	netip.MustParsePrefix("3fff::/20"),
}

func publicIP(ip netip.Addr) bool {
	if ip.Zone() != "" || ip.Is4In6() {
		return false
	}
	if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}
	if ip.Is6() && !netip.MustParsePrefix("2000::/3").Contains(ip) {
		return false
	}
	for _, p := range excluded {
		if p.Contains(ip) {
			return false
		}
	}
	return true
}
func publicDial(ctx context.Context, network, address string) (net.Conn, error) {
	return dialResolved(ctx, network, address, net.DefaultResolver.LookupNetIP, (&net.Dialer{Timeout: 5 * time.Second}).DialContext)
}

func dialResolved(ctx context.Context, network, address string, lookup func(context.Context, string, string) ([]netip.Addr, error), dial func(context.Context, string, string) (net.Conn, error)) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, application.ErrModel3DUnavailable
	}
	ips, err := lookup(ctx, "ip", host)
	if err != nil || len(ips) == 0 {
		return nil, application.ErrModel3DUnavailable
	}
	for _, ip := range ips {
		if !publicIP(ip) {
			return nil, application.NewInputError("validation.model_import_url")
		}
	}
	// Pin the validated answer: no second DNS resolution, including redirects.
	for _, ip := range ips {
		c, e := dial(ctx, network, net.JoinHostPort(ip.String(), port))
		if e == nil {
			return c, nil
		}
	}
	return nil, application.ErrModel3DUnavailable
}
func (d *Downloader) Download(ctx context.Context, raw string) ([]byte, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, application.NewInputError("validation.model_import_url")
	}
	if err = validURL(u); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, application.NewInputError("validation.model_import_url")
	}
	req.Header.Set("Accept", "model/gltf-binary, application/octet-stream")
	req.Header.Set("Accept-Encoding", "identity")
	resp, err := d.client.Do(req)
	if err != nil {
		return nil, application.ErrModel3DUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, application.ErrModel3DUnavailable
	}
	if resp.Header.Get("Content-Encoding") != "" && resp.Header.Get("Content-Encoding") != "identity" {
		return nil, application.NewInputError("validation.model_import_encoding")
	}
	if resp.ContentLength > application.MaxProductModel3DBytes {
		return nil, application.NewInputError("validation.model_import_size")
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, application.MaxProductModel3DBytes+1))
	if err != nil {
		return nil, application.ErrModel3DUnavailable
	}
	if int64(len(b)) > application.MaxProductModel3DBytes {
		return nil, application.NewInputError("validation.model_import_size")
	}
	return b, nil
}
