// Package safehttp menyediakan http.Client yang aman terhadap SSRF: IP tujuan diperiksa
// saat dial (menutup DNS rebinding), hanya https, redirect hanya ke host yang sama,
// proxy lingkungan dimatikan.
package safehttp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

var (
	ErrBlockedAddress    = errors.New("safehttp: destination address not allowed")
	ErrSchemeNotAllowed  = errors.New("safehttp: only https allowed")
	ErrCrossHostRedirect = errors.New("safehttp: redirect to another host not allowed")
	ErrTooManyRedirects  = errors.New("safehttp: too many redirects")
)

// Options mengonfigurasi client.
type Options struct {
	Timeout      time.Duration // total per request (http.Client.Timeout)
	MaxRedirects int           // default 3, hanya host sama
	// InsecureAllowHosts: "host:port" yang boleh http + loopback. HANYA test/e2e.
	InsecureAllowHosts []string
	// Resolver nil -> net.DefaultResolver. Seam tes DNS rebinding.
	Resolver Resolver
}

// Resolver adalah subset net.Resolver.
type Resolver interface {
	LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error)
}

// New membuat client aman.
func New(o Options) *http.Client {
	if o.MaxRedirects <= 0 {
		o.MaxRedirects = 3
	}
	var res Resolver = net.DefaultResolver
	if o.Resolver != nil {
		res = o.Resolver
	}
	insecure := make(map[string]struct{}, len(o.InsecureAllowHosts))
	for _, h := range o.InsecureAllowHosts {
		insecure[h] = struct{}{}
	}
	tr := &http.Transport{
		Proxy:                 nil,
		DialContext:           newDialer(res, insecure),
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          10,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 0,
	}
	return &http.Client{
		Timeout:       o.Timeout,
		Transport:     &schemeGuard{next: tr, insecure: insecure},
		CheckRedirect: checkRedirect(o.MaxRedirects),
	}
}

type schemeGuard struct {
	next     http.RoundTripper
	insecure map[string]struct{}
}

func (g *schemeGuard) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Scheme != "https" {
		_, ok := g.insecure[r.URL.Host]
		if !(r.URL.Scheme == "http" && ok) {
			return nil, ErrSchemeNotAllowed
		}
	}
	return g.next.RoundTrip(r)
}

func checkRedirect(max int) func(*http.Request, []*http.Request) error {
	return func(req *http.Request, via []*http.Request) error {
		if len(via) > max {
			return ErrTooManyRedirects
		}
		if req.URL.Host != via[0].URL.Host {
			return ErrCrossHostRedirect
		}
		return nil
	}
}

// ReadLimited membaca paling banyak max byte. tooLarge true bila body lebih besar dari max
// (body dikembalikan nil).
func ReadLimited(resp *http.Response, max int64) (body []byte, tooLarge bool, err error) {
	if max < 0 {
		return nil, false, fmt.Errorf("safehttp: negative limit")
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, max+1))
	if err != nil {
		return nil, false, err
	}
	if int64(len(b)) > max {
		return nil, true, nil
	}
	return b, false, nil
}
