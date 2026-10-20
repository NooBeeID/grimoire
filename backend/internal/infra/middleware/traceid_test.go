package middleware

import (
	"grimoire/internal/infra/http/respond"
	"net/http"
	"strings"
	"testing"

	"grimoire/lib/common/reqctx"
)

func TestTraceID(t *testing.T) {
	var seen string
	h := TraceID()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { seen = reqctx.TraceID(r.Context()) }))

	w := do(h, "GET", "/", nil, "")
	if id := w.Header().Get(respond.TraceHeader); len(id) != 36 || id != seen {
		t.Fatalf("generated %q ctx %q", id, seen)
	}
	w = do(h, "GET", "/", map[string]string{respond.TraceHeader: "abc-123"}, "")
	if w.Header().Get(respond.TraceHeader) != "abc-123" || seen != "abc-123" {
		t.Fatal("valid harus dipakai ulang")
	}
	for _, bad := range []string{"has space", strings.Repeat("a", 65), "<script>", "a_b"} {
		w = do(h, "GET", "/", map[string]string{respond.TraceHeader: bad}, "")
		if got := w.Header().Get(respond.TraceHeader); got == bad || len(got) != 36 {
			t.Fatalf("%q tidak diganti: %q", bad, got)
		}
	}
}
