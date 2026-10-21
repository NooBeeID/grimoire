package base

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateName(t *testing.T) {
	ok := []string{"payment", "a", "v1.2_x-y", strings.Repeat("a", 80)}
	for _, s := range ok {
		if err := ValidateName(s); err != nil {
			t.Errorf("%q: %v", s, err)
		}
	}
	bad := []string{"", ".", "..", "Payment", "a b", "a/b", strings.Repeat("a", 81), "é"}
	for _, s := range bad {
		if err := ValidateName(s); !errors.Is(err, ErrInvalidName) {
			t.Errorf("%q: want ErrInvalidName, got %v", s, err)
		}
	}
}

func TestDeriveName(t *testing.T) {
	cases := map[string]string{
		"auth.yaml":                 "auth",
		"specs/Payment API.v2.yaml": "payment-api.v2",
		`C:\x\Notif.YML`:            "notif",
		"Ünïcode.yaml":              "n-code",
	}
	for in, want := range cases {
		got, err := DeriveName(in)
		if err != nil || got != want {
			t.Errorf("%q: got %q %v want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"---.json", ".yaml", "", "..yaml"} {
		if _, err := DeriveName(in); !errors.Is(err, ErrInvalidName) {
			t.Errorf("%q: want error, got %v", in, err)
		}
	}
	long := strings.Repeat("a", 79) + "-bbbb.yaml"
	got, err := DeriveName(long)
	if err != nil || len(got) > NameMaxLen || strings.HasSuffix(got, "-") || strings.HasSuffix(got, ".") {
		t.Errorf("long: %q %v", got, err)
	}
}

func TestNormalizeSourcePath(t *testing.T) {
	ok := map[string]string{
		`a\b\c.yaml`:  "a/b/c.yaml",
		"./x/y.yaml":  "x/y.yaml",
		"a//b.yaml":   "a/b.yaml",
		"plain.yaml":  "plain.yaml",
		"././a.yaml":  "a.yaml",
		"dir/日本.yaml": "dir/日本.yaml",
	}
	for in, want := range ok {
		got, err := NormalizeSourcePath(in)
		if err != nil || got != want {
			t.Errorf("%q: got %q %v", in, got, err)
		}
	}
	bad := []string{"", "/etc/x.yaml", "C:/x.yaml", `c:\x.yaml`, "a/../b.yaml", "a\x00b", "a\nb", strings.Repeat("a", 513), "\xff.yaml", ".//"}
	for _, in := range bad {
		if _, err := NormalizeSourcePath(in); !errors.Is(err, ErrInvalidPath) {
			t.Errorf("%q: want ErrInvalidPath, got %v", in, err)
		}
	}
}

func TestFormatFromExt(t *testing.T) {
	for in, want := range map[string]string{"a.YAML": FormatYAML, "a.yml": FormatYAML, "a.json": FormatJSON, "d/a.Json": FormatJSON} {
		if got, ok := FormatFromExt(in); !ok || got != want {
			t.Errorf("%q: %q %v", in, got, ok)
		}
	}
	for _, in := range []string{"a.txt", "noext", "a.yaml.bak", ""} {
		if _, ok := FormatFromExt(in); ok {
			t.Errorf("%q should be skipped", in)
		}
	}
}
