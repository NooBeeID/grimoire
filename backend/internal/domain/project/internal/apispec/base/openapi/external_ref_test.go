package openapi

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
)

// TestT06_AC2_ExternalRefNoRequest membuktikan $ref ke URL ditolak tanpa satu pun request keluar.
func TestT06_AC2_ExternalRefNoRequest(t *testing.T) {
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	raw, err := os.ReadFile("testdata/ref-http.yaml")
	if err != nil {
		t.Fatal(err)
	}
	host := strings.TrimPrefix(srv.URL, "http://")
	content := strings.ReplaceAll(string(raw), "127.0.0.1:__PORT__", host)
	if !strings.Contains(content, srv.URL) {
		t.Fatalf("placeholder tidak terganti: %s", content)
	}

	_, issues := Parse([]byte(content), FormatYAML, DefaultLimits)
	found := false
	for _, is := range issues {
		if is.Code == CodeExternalRef {
			found = true
		}
	}
	if !found {
		t.Fatalf("want issue %s, got %+v", CodeExternalRef, issues)
	}
	if n := hits.Load(); n != 0 {
		t.Fatalf("server menerima %d request, want 0", n)
	}
}

func TestRefs_Swagger2External(t *testing.T) {
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
	}))
	defer srv.Close()
	host := strings.TrimPrefix(srv.URL, "http://")
	for _, f := range []string{"swagger-2.0-ref-http.yaml", "swagger-2.0-ref-file.yaml"} {
		content := strings.ReplaceAll(string(load(t, f)), "127.0.0.1:__PORT__", host)
		_, is := Parse([]byte(content), FormatYAML, DefaultLimits)
		if len(is) != 1 || is[0].Code != CodeExternalRef || is[0].Line <= 0 ||
			is[0].Pointer != "#/paths/~1pets/get/responses/200/schema" {
			t.Fatalf("%s: %+v", f, is)
		}
	}
	if n := hits.Load(); n != 0 {
		t.Fatalf("server menerima %d request, want 0", n)
	}
}
