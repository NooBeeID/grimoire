package base

import (
	"strconv"
	"strings"
)

const (
	hostGitHub = "github.com"
	hostGitLab = "gitlab.com"
)

// NormalizeHost menormalkan host sesuai provider. Policy (daftar host yang diizinkan)
// diperiksa usecase setelah ini.
//   - github: selalu "github.com"; input kosong, "github.com", atau "api.github.com" diterima, selain itu ErrHostNotAllowed.
//   - gitlab: trim, lowercase, buang "https://" dan satu "/" penutup; tolak userinfo/path/query; port 1-65535
//     boleh; kosong -> "gitlab.com"; panjang 1-253.
func NormalizeHost(p Provider, raw string) (string, error) {
	h := strings.ToLower(strings.TrimSpace(raw))
	h = strings.TrimPrefix(h, "https://")
	h = strings.TrimSuffix(h, "/")
	switch p {
	case ProviderGitHub:
		switch h {
		case "", hostGitHub, "api.github.com":
			return hostGitHub, nil
		}
		return "", HostNotAllowed(h)
	case ProviderGitLab:
		if h == "" {
			return hostGitLab, nil
		}
		if !validHost(h) {
			return "", ErrInvalidHost
		}
		return h, nil
	}
	return "", ErrInvalidProvider
}

func validHost(h string) bool {
	if len(h) == 0 || len(h) > 253 {
		return false
	}
	name := h
	if i := strings.LastIndexByte(h, ':'); i >= 0 {
		port, err := strconv.Atoi(h[i+1:])
		if err != nil || port < 1 || port > 65535 || h[i+1:] != strconv.Itoa(port) {
			return false
		}
		name = h[:i]
	}
	if name == "" {
		return false
	}
	for _, label := range strings.Split(name, ".") {
		if label == "" || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return false
		}
		for _, r := range label {
			switch {
			case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
			default:
				return false
			}
		}
	}
	return true
}
