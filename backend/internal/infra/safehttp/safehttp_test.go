package safehttp

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

type fakeResolver struct {
	answers map[string][]string
}

func (f fakeResolver) LookupIPAddr(_ context.Context, host string) ([]net.IPAddr, error) {
	var out []net.IPAddr
	for _, s := range f.answers[host] {
		out = append(out, net.IPAddr{IP: net.ParseIP(s)})
	}
	if out == nil {
		return nil, errors.New("no such host")
	}
	return out, nil
}

func get(t *testing.T, c *http.Client, u string) error {
	t.Helper()
	resp, err := c.Get(u)
	if err == nil {
		resp.Body.Close()
	}
	return err
}

func TestBlockedAddresses(t *testing.T) {
	c := New(Options{Timeout: 2 * time.Second})
	for _, u := range []string{
		"https://127.0.0.1/", "https://10.0.0.5/", "https://169.254.169.254/latest",
		"https://[fc00::1]/", "https://[::ffff:127.0.0.1]/", "https://100.64.0.1/",
		"https://0.0.0.0/", "https://192.168.1.1/", "https://172.16.0.1/", "https://[::1]/", "https://[fe80::1]/",
	} {
		if err := get(t, c, u); !errors.Is(err, ErrBlockedAddress) {
			t.Errorf("%s: want ErrBlockedAddress, got %v", u, err)
		}
	}
}

func TestDNSRebindingMixedAnswer(t *testing.T) {
	r := fakeResolver{answers: map[string][]string{
		"evil.test":  {"93.184.216.34", "10.0.0.5"},
		"local.test": {"127.0.0.1"},
	}}
	c := New(Options{Timeout: time.Second, Resolver: r})
	for _, h := range []string{"evil.test", "local.test"} {
		if err := get(t, c, "https://"+h+"/"); !errors.Is(err, ErrBlockedAddress) {
			t.Errorf("%s: %v", h, err)
		}
	}
}

func TestSchemeRejected(t *testing.T) {
	c := New(Options{Timeout: time.Second})
	if err := get(t, c, "http://example.com/"); !errors.Is(err, ErrSchemeNotAllowed) {
		t.Fatal(err)
	}
}

func TestInsecureAllowHostsOnlyListedPort(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) }))
	defer srv.Close()
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer other.Close()
	u, _ := url.Parse(srv.URL)
	c := New(Options{Timeout: 2 * time.Second, InsecureAllowHosts: []string{u.Host}})
	if err := get(t, c, srv.URL); err != nil {
		t.Fatal(err)
	}
	if err := get(t, c, other.URL); err == nil {
		t.Fatal("port lain harus ditolak")
	}
}

func TestRedirects(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("x")) }))
	defer target.Close()
	var self *httptest.Server
	self = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cross":
			http.Redirect(w, r, target.URL, http.StatusFound)
		case "/loop":
			http.Redirect(w, r, "/loop", http.StatusFound)
		case "/same":
			http.Redirect(w, r, "/ok", http.StatusFound)
		default:
			if r.Header.Get("Authorization") != "tok" {
				w.WriteHeader(401)
			}
		}
	}))
	defer self.Close()
	su, _ := url.Parse(self.URL)
	tu, _ := url.Parse(target.URL)
	c := New(Options{Timeout: 2 * time.Second, InsecureAllowHosts: []string{su.Host, tu.Host}})
	req := func(p string) error {
		r, _ := http.NewRequest("GET", self.URL+p, nil)
		r.Header.Set("Authorization", "tok")
		resp, err := c.Do(r)
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode != 200 {
				return errors.New("status " + resp.Status)
			}
		}
		return err
	}
	if err := req("/cross"); !errors.Is(err, ErrCrossHostRedirect) {
		t.Errorf("cross: %v", err)
	}
	if err := req("/loop"); !errors.Is(err, ErrTooManyRedirects) {
		t.Errorf("loop: %v", err)
	}
	if err := req("/same"); err != nil {
		t.Errorf("same: %v", err)
	}
}

func TestProxyEnvIgnored(t *testing.T) {
	hit := false
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hit = true }))
	defer proxy.Close()
	t.Setenv("HTTPS_PROXY", proxy.URL)
	t.Setenv("HTTP_PROXY", proxy.URL)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	c := New(Options{Timeout: 2 * time.Second, InsecureAllowHosts: []string{u.Host}})
	if err := get(t, c, srv.URL); err != nil {
		t.Fatal(err)
	}
	if hit {
		t.Fatal("proxy dipakai")
	}
}

func TestReadLimited(t *testing.T) {
	mk := func(n int) *http.Response {
		return &http.Response{Body: nopCloser{strings.NewReader(strings.Repeat("a", n))}}
	}
	b, tl, err := ReadLimited(mk(10), 10)
	if err != nil || tl || len(b) != 10 {
		t.Fatal(len(b), tl, err)
	}
	b, tl, err = ReadLimited(mk(11), 10)
	if err != nil || !tl || b != nil {
		t.Fatal(len(b), tl, err)
	}
}

type nopCloser struct{ *strings.Reader }

func (nopCloser) Close() error { return nil }
