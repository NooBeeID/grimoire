package base

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
)

func codeOf(t *testing.T, err error) errs.Code {
	t.Helper()
	e, ok := err.(*errs.Error)
	if !ok {
		t.Fatalf("bukan *errs.Error: %v", err)
	}
	return e.Code
}

func TestParseProvider(t *testing.T) {
	for in, want := range map[string]Provider{"github": ProviderGitHub, "GitHub": ProviderGitHub, " gitlab ": ProviderGitLab} {
		if got, err := ParseProvider(in); err != nil || got != want {
			t.Errorf("%q: %v %v", in, got, err)
		}
	}
	for _, in := range []string{"bitbucket", ""} {
		if _, err := ParseProvider(in); err != ErrInvalidProvider {
			t.Errorf("%q: %v", in, err)
		}
	}
}

func TestNormalizeHost(t *testing.T) {
	ok := []struct {
		p       Provider
		in, out string
	}{
		{ProviderGitHub, "", "github.com"},
		{ProviderGitHub, "github.com", "github.com"},
		{ProviderGitHub, "api.github.com", "github.com"},
		{ProviderGitLab, "", "gitlab.com"},
		{ProviderGitLab, "HTTPS://Git.Noobee.ID/", "git.noobee.id"},
		{ProviderGitLab, "git.noobee.id:8443", "git.noobee.id:8443"},
	}
	for _, c := range ok {
		got, err := NormalizeHost(c.p, c.in)
		if err != nil || got != c.out {
			t.Errorf("%s %q: %q %v", c.p, c.in, got, err)
		}
	}
	if _, err := NormalizeHost(ProviderGitHub, "evil.com"); err == nil || codeOf(t, err) != errs.CodeHostNotAllowed {
		t.Errorf("github evil: %v", err)
	}
	for _, in := range []string{"user@host.com", "host.com/path", "host.com?x=1", "host:0", "host:99999", "host:abc", "a b.com", "-", ".x.com", "x..com"} {
		if _, err := NormalizeHost(ProviderGitLab, in); err != ErrInvalidHost {
			t.Errorf("%q: %v", in, err)
		}
	}
	if _, err := NormalizeHost(ProviderGitLab, strings.Repeat("a", 254)); err != ErrInvalidHost {
		t.Error("terlalu panjang")
	}
	if _, err := NormalizeHost("bitbucket", "x"); err != ErrInvalidProvider {
		t.Error("provider")
	}
}

func TestValidateToken(t *testing.T) {
	tok, err := ValidateToken("  ghp_abcdefgh  ")
	if err != nil || tok.Reveal() != "ghp_abcdefgh" {
		t.Fatal(tok, err)
	}
	for name, in := range map[string]string{
		"7 char": "1234567", "513 char": strings.Repeat("a", 513), "spasi": "abcd efgh1", "non-ascii": "abcdefgé12", "kosong": "", "tab": "abcd\tefgh",
	} {
		_, err := ValidateToken(in)
		if err == nil {
			t.Errorf("%s: want error", name)
			continue
		}
		if strings.Contains(err.Error(), in) && in != "" {
			t.Errorf("%s: pesan memuat token", name)
		}
	}
	if _, err := ValidateToken(strings.Repeat("a", 512)); err != nil {
		t.Error("512 harus lolos")
	}
	if _, err := ValidateToken("12345678"); err != nil {
		t.Error("8 harus lolos")
	}
}

func TestAAD(t *testing.T) {
	a := AAD("u1", ProviderGitLab, "git.noobee.id")
	if string(a) != "grimoire.git_credential.v1|u1|gitlab|git.noobee.id" {
		t.Fatal(string(a))
	}
	if string(AAD("u1", ProviderGitLab, "x")) == string(AAD("u2", ProviderGitLab, "x")) {
		t.Fatal("aad harus beda")
	}
}

func TestToDTONoToken(t *testing.T) {
	c := Credential{UserID: "u", Provider: ProviderGitHub, Host: "github.com", TokenEnc: []byte("ENCRYPTEDBLOB"),
		Last4: "a9F3", CreatedAt: time.Unix(0, 0), UpdatedAt: time.Unix(10, 0)}
	b, _ := json.Marshal(c.ToDTO())
	s := string(b)
	if strings.Contains(strings.ToLower(s), "token") || strings.Contains(s, "ENCRYPTED") || !strings.Contains(s, `"last4":"a9F3"`) {
		t.Fatal(s)
	}
}

func TestActorAndMeta(t *testing.T) {
	if _, ok := ActorFrom(reqctx.Identity{}, true); ok {
		t.Error("UserID kosong")
	}
	if _, ok := ActorFrom(reqctx.Identity{UserID: "u"}, false); ok {
		t.Error("ok false")
	}
	a, _ := ActorFrom(reqctx.Identity{UserID: "u", Email: "e@x"}, true)
	if a.Label != "e@x" {
		t.Error(a)
	}
	a, _ = ActorFrom(reqctx.Identity{UserID: "u", Email: "e@x", Name: "Rina"}, true)
	if a.Label != "Rina" {
		t.Error(a)
	}
	m := NewRequestMeta("1.2.3.4", strings.Repeat("é", 400))
	if len(m.UserAgent) > 512 || !strings.HasSuffix(m.UserAgent, "é") {
		t.Error(len(m.UserAgent))
	}
}

func TestErrorsHideToken(t *testing.T) {
	e := TokenInvalid(ProviderGitLab, "git.noobee.id")
	if e.Code != CodeTokenInvalid || e.Details.(map[string]any)["host"] != "git.noobee.id" {
		t.Fatal(e)
	}
	if RateLimited(0).Details != nil || RateLimited(60).Details.(map[string]any)["retry_after_seconds"] != 60 {
		t.Fatal("rate limited")
	}
	if !ValidUUID("0192f5a0-aaaa-7aaa-8bbb-00000000000a") || ValidUUID("x") {
		t.Fatal("uuid")
	}
}
