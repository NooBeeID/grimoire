package constants

import (
	"regexp"
	"testing"
)

func TestBusConstants_AuthAndAudit(t *testing.T) {
	re := regexp.MustCompile(`^[a-z]+\.[a-z]+\.[a-z]+(-[a-z]+)*$`)
	want := map[string]string{
		BUS_CORE_AUTH_GET_USER:        "core.auth.get-user",
		BUS_CORE_AUTH_FIND_DEVELOPERS: "core.auth.find-developers",
		BUS_CORE_AUTH_CREATE_ADMIN:    "core.auth.create-admin",
		BUS_CORE_AUDIT_RECORD:         "core.audit.record",
	}
	seen := map[string]bool{BUS_CORE_AUTH_RESOLVE_SESSION: true}
	for got, w := range want {
		if got != w || !re.MatchString(got) || seen[got] {
			t.Errorf("konstanta bus salah/duplikat: %q", got)
		}
		seen[got] = true
	}
}

func TestBusConstants_NamingRule64(t *testing.T) {
	re := regexp.MustCompile(`^[a-z]+\.[a-z]+\.[a-z]+(-[a-z]+)*$`)
	if !re.MatchString(BUS_CORE_AUTH_RESOLVE_SESSION) || BUS_CORE_AUTH_RESOLVE_SESSION != "core.auth.resolve-session" {
		t.Fatalf("got %q", BUS_CORE_AUTH_RESOLVE_SESSION)
	}
}
