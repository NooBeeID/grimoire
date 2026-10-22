package safehttp

import (
	"context"
	"net"
	"net/netip"
	"syscall"
	"time"
)

type dialFunc func(ctx context.Context, network, addr string) (net.Conn, error)

var blockedPrefixes = mustPrefixes(
	"0.0.0.0/8", "10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8", "169.254.0.0/16",
	"172.16.0.0/12", "192.168.0.0/16", "224.0.0.0/4", "240.0.0.0/4",
	"::/128", "::1/128", "fc00::/7", "fe80::/10", "ff00::/8",
)

func mustPrefixes(ss ...string) []netip.Prefix {
	out := make([]netip.Prefix, len(ss))
	for i, s := range ss {
		out[i] = netip.MustParsePrefix(s)
	}
	return out
}

// blocked true bila ip tidak boleh dituju.
func blocked(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsValid() || ip.IsUnspecified() || ip.IsLoopback() || ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() {
		return true
	}
	for _, p := range blockedPrefixes {
		if p.Contains(ip) {
			return true
		}
	}
	return false
}

func newDialer(res Resolver, insecure map[string]struct{}) dialFunc {
	plain := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	safe := &net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
		Control: func(network, address string, c syscall.RawConn) error {
			h, _, err := net.SplitHostPort(address)
			if err != nil {
				return ErrBlockedAddress
			}
			ip, err := netip.ParseAddr(h)
			if err != nil || blocked(ip) {
				return ErrBlockedAddress
			}
			return nil
		},
	}
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		if _, ok := insecure[addr]; ok {
			return plain.DialContext(ctx, network, addr)
		}
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		var ips []netip.Addr
		if ip, perr := netip.ParseAddr(host); perr == nil {
			ips = []netip.Addr{ip}
		} else {
			addrs, lerr := res.LookupIPAddr(ctx, host)
			if lerr != nil {
				return nil, lerr
			}
			for _, a := range addrs {
				if ip, ok := netip.AddrFromSlice(a.IP); ok {
					ips = append(ips, ip)
				}
			}
		}
		if len(ips) == 0 {
			return nil, ErrBlockedAddress
		}
		// Semua jawaban DNS harus publik; satu IP privat menolak seluruhnya.
		for _, ip := range ips {
			if blocked(ip) {
				return nil, ErrBlockedAddress
			}
		}
		var lastErr error
		for _, ip := range ips {
			conn, err := safe.DialContext(ctx, network, net.JoinHostPort(ip.Unmap().String(), port))
			if err == nil {
				return conn, nil
			}
			lastErr = err
		}
		return nil, lastErr
	}
}
