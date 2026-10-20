package getentry

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"grimoire/lib/common/errs"
)

const validID = "0192f0c4-7b00-7d11-8a00-1a2b3c4d5e6f"

func reqWithID(id string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	rc := chi.NewRouteContext()
	rc.URLParams.Add("id", id)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rc))
}

func TestParseParams_Valid(t *testing.T) {
	p, err := parseParams(reqWithID(validID))
	if err != nil || p.ID != validID {
		t.Fatalf("got %+v err %v", p, err)
	}
}

func TestParseParams_UppercaseNormalized(t *testing.T) {
	p, err := parseParams(reqWithID(strings.ToUpper(validID)))
	if err != nil || p.ID != validID {
		t.Fatalf("got %+v err %v", p, err)
	}
}

func TestParseParams_Invalid(t *testing.T) {
	cases := []string{
		"", "abc", "not-a-uuid", strings.ReplaceAll(validID, "-", ""),
		"urn:uuid:" + validID, "{" + validID + "}", validID + " ", validID + "a",
		"../etc/passwd", "' OR 1=1 --",
	}
	for _, c := range cases {
		_, err := parseParams(reqWithID(c))
		if err == nil || !errors.Is(err, errs.ErrNotFound) {
			t.Fatalf("%q: want not found, got %v", c, err)
		}
		var e *errs.Error
		if !errors.As(err, &e) || e.Code != errs.CodeNotFound {
			t.Fatalf("%q: code salah: %v", c, err)
		}
		if c != "" && strings.Contains(err.Error(), c) {
			t.Fatalf("%q: pesan memuat input", c)
		}
	}
}
