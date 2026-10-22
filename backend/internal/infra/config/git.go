package config

import (
	"encoding/base64"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

func gitKeyState(c Config) string {
	if len(c.GitCredentialKey) > 0 {
		return "set"
	}
	return "unset"
}

// hostPort memvalidasi "host[:port]" tanpa skema/path/userinfo; hasil lowercase.
func hostPort(e string) (string, bool) {
	if e == "" || strings.ContainsAny(e, "/@?#\\ ") || strings.Contains(e, "://") {
		return "", false
	}
	e = strings.ToLower(e)
	host := e
	if h, p, err := net.SplitHostPort(e); err == nil {
		n, err := strconv.Atoi(p)
		if err != nil || n < 1 || n > 65535 {
			return "", false
		}
		host = h
	} else if strings.Contains(e, ":") && !strings.HasPrefix(e, "[") {
		return "", false
	}
	if host == "" || len(host) > 253 {
		return "", false
	}
	return e, true
}

func (l *loader) hostList(env string, needPort bool) []string {
	v, ok := l.raw(env)
	if !ok {
		return nil
	}
	var out []string
	for _, e := range strings.Split(v, ",") {
		e = strings.TrimSpace(e)
		if e == "" {
			continue
		}
		hp, ok := hostPort(e)
		if ok && needPort {
			_, _, err := net.SplitHostPort(hp)
			ok = err == nil
		}
		if !ok {
			if needPort {
				l.fail(env, "entri %q harus berbentuk host:port", e)
			} else {
				l.fail(env, "entri %q harus berbentuk host[:port] tanpa skema/path/userinfo", e)
			}
			continue
		}
		out = append(out, hp)
	}
	return out
}

func (l *loader) loadGit(c *Config, appEnv string) {
	// Nilai kunci tidak pernah disertakan di pesan error.
	if v, ok := l.raw("GIT_CREDENTIAL_KEY"); ok {
		k, err := base64.StdEncoding.DecodeString(v)
		if err != nil || len(k) != 32 {
			l.fail("GIT_CREDENTIAL_KEY", "harus base64 dari tepat 32 byte")
		} else {
			c.GitCredentialKey = k
		}
	}
	for _, h := range l.hostList("GIT_SYNC_ALLOWED_HOSTS", false) {
		if h == "github.com" || h == "gitlab.com" {
			continue // sudah implisit
		}
		c.GitSyncAllowedHosts = append(c.GitSyncAllowedHosts, h)
	}
	c.GitSyncMaxTotalWait = l.dur("GIT_SYNC_MAX_TOTAL_WAIT", 5*time.Minute)
	c.GitSyncMaxWait = l.durBetween("GIT_SYNC_MAX_WAIT", 120*time.Second, time.Second, c.GitSyncMaxTotalWait)
	c.GitSyncRequestInterval = l.durBetween("GIT_SYNC_REQUEST_INTERVAL", 500*time.Millisecond, 0, 10*time.Second)
	c.GitSyncTimeout = l.dur("GIT_SYNC_TIMEOUT", 15*time.Minute)
	if c.GitSyncTimeout <= c.HTTPDefaultTimeout {
		l.fail("GIT_SYNC_TIMEOUT", "harus lebih besar dari HTTP_DEFAULT_TIMEOUT (%s)", c.HTTPDefaultTimeout)
	} else if c.GitSyncTimeout <= c.GitSyncMaxTotalWait {
		l.fail("GIT_SYNC_TIMEOUT", "harus lebih besar dari GIT_SYNC_MAX_TOTAL_WAIT (%s)", c.GitSyncMaxTotalWait)
	}
	c.GitSyncInsecureTestHosts = l.hostList("GIT_SYNC_INSECURE_TEST_HOSTS", true)
	if len(c.GitSyncInsecureTestHosts) > 0 && appEnv == "production" {
		l.fail("GIT_SYNC_INSECURE_TEST_HOSTS", "tidak boleh diisi saat APP_ENV=production")
	}
	if v, ok := l.raw("GIT_SYNC_GITHUB_API_BASE"); ok {
		u, err := url.Parse(v)
		switch {
		case appEnv == "production":
			l.fail("GIT_SYNC_GITHUB_API_BASE", "tidak boleh diisi saat APP_ENV=production")
		case err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "":
			l.fail("GIT_SYNC_GITHUB_API_BASE", "harus URL http(s)://host:port")
		case !contains(c.GitSyncInsecureTestHosts, strings.ToLower(u.Host)):
			l.fail("GIT_SYNC_GITHUB_API_BASE", "host:port harus ada di GIT_SYNC_INSECURE_TEST_HOSTS")
		default:
			c.GitSyncGitHubAPIBase = strings.TrimRight(v, "/")
		}
	}
}

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}
