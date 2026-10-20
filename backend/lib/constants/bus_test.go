package constants

import (
	"regexp"
	"testing"
)

func TestBusConstants_NamingRule64(t *testing.T) {
	re := regexp.MustCompile(`^[a-z]+\.[a-z]+\.[a-z]+(-[a-z]+)*$`)
	if !re.MatchString(BUS_CORE_AUTH_RESOLVE_SESSION) || BUS_CORE_AUTH_RESOLVE_SESSION != "core.auth.resolve-session" {
		t.Fatalf("got %q", BUS_CORE_AUTH_RESOLVE_SESSION)
	}
}
