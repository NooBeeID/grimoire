package base

import (
	"strings"
	"testing"
)

func TestNegotiate(t *testing.T) {
	for _, v := range SupportedVersions {
		if got, ok := Negotiate(v); !ok || got != v {
			t.Errorf("%s", v)
		}
	}
	if _, ok := Negotiate("2024-11-05"); ok {
		t.Fatal("versi lama harus ditolak")
	}
	if v, ok := KnownHeaderVersion(""); !ok || v != DefaultHeaderVersion {
		t.Fatal("default")
	}
	if _, ok := KnownHeaderVersion("x"); ok {
		t.Fatal("header tak dikenal")
	}
}

func TestInstructions(t *testing.T) {
	got := Instructions("Payments", []string{"list_files", "read_file", "search_knowledge"})
	for _, w := range []string{"Payments", "list_files", "read_file", "search_knowledge"} {
		if !strings.Contains(got, w) {
			t.Errorf("tidak memuat %q: %s", w, got)
		}
	}
	if e := Instructions("", nil); !strings.Contains(e, "project ini") || !strings.Contains(e, "Belum ada tool") {
		t.Fatal(e)
	}
}
