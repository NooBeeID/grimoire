package middleware

import (
	"grimoire/internal/infra/http/respond"
	"net/http"
	"testing"
	"time"
)

func TestTimeout(t *testing.T) {
	useLogger(t)
	h := Timeout(20 * time.Millisecond)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
		respond.WriteError(w, r, r.Context().Err())
	}))
	w := do(h, "GET", "/", nil, "")
	if w.Code != 504 || decodeErr(t, w).Error.Code != "TIMEOUT" {
		t.Fatal(w.Code)
	}
	var dl time.Time
	override := Timeout(time.Hour)(Timeout(time.Minute)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dl, _ = r.Context().Deadline()
	})))
	do(override, "GET", "/", nil, "")
	if time.Until(dl) > time.Minute {
		t.Fatal("deadline terdalam harus menang")
	}
	mustPanic(t, func() { Timeout(0) })
}
