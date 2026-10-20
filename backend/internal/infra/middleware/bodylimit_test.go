package middleware

import (
	"grimoire/internal/infra/http/respond"
	"net/http"
	"strings"
	"testing"
)

func readBody(w http.ResponseWriter, r *http.Request) {
	var v map[string]any
	if err := respond.DecodeJSON(r, &v); err != nil {
		respond.WriteError(w, r, err)
		return
	}
	respond.WriteJSON(w, 200, v)
}

func TestBodyLimit(t *testing.T) {
	useLogger(t)
	small := `{"a":"` + strings.Repeat("x", 100) + `"}`
	h := BodyLimit(50)(http.HandlerFunc(readBody))
	w := do(h, "POST", "/", nil, small)
	e := decodeErr(t, w)
	d, _ := e.Error.Details.(map[string]any)
	if w.Code != 413 || e.Error.Code != "PAYLOAD_TOO_LARGE" || d["limit_bytes"] != float64(50) {
		t.Fatalf("%d %+v", w.Code, e)
	}
	if w := do(h, "POST", "/", nil, `{"a":1}`); w.Code != 200 {
		t.Fatal("di bawah batas harus lolos")
	}
	// batas terdalam menang: naik
	raise := BodyLimit(50)(BodyLimit(1000)(http.HandlerFunc(readBody)))
	if w := do(raise, "POST", "/", nil, small); w.Code != 200 {
		t.Fatalf("raise: %d", w.Code)
	}
	// turun
	lower := BodyLimit(1000)(BodyLimit(20)(http.HandlerFunc(readBody)))
	if w := do(lower, "POST", "/", nil, small); w.Code != 413 {
		t.Fatalf("lower: %d", w.Code)
	}
	mustPanic(t, func() { BodyLimit(0) })
}
