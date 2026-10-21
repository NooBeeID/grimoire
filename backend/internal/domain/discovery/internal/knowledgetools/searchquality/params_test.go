package searchquality

import (
	"errors"
	"strings"
	"testing"
	"time"

	"grimoire/lib/common/errs"
	"grimoire/lib/contracts/discovery/knowledgetools/searchquality"
)

const pid = "3f2b8c1e-5a4d-4e6f-9b7a-1c2d3e4f5a6b"

var (
	tSince = time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)
	tNow   = time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
)

func okReq() searchquality.Request {
	return searchquality.Request{ProjectID: pid, Since: tSince, Until: tNow}
}

func wantInvalid(t *testing.T, err error, msg string) {
	t.Helper()
	var e *errs.Error
	if !errors.As(err, &e) || e.Code != errs.CodeInvalid || !errors.Is(err, errs.ErrInvalid) {
		t.Fatalf("bukan invalid: %v", err)
	}
	if e.Message != msg {
		t.Fatalf("pesan = %q, mau %q", e.Message, msg)
	}
}

func TestParseParams_ProjectID(t *testing.T) {
	for _, in := range []string{"", "x", pid[:20]} {
		r := okReq()
		r.ProjectID = in
		_, err := ParseParams(r, tNow)
		wantInvalid(t, err, "project_id tidak valid")
		if in != "" && strings.Contains(err.Error(), in) {
			t.Errorf("pesan memuat input %q", in)
		}
	}
	r := okReq()
	r.ProjectID = strings.ToUpper(pid)
	p, err := ParseParams(r, tNow)
	if err != nil || p.ProjectID != pid {
		t.Fatalf("p=%+v err=%v", p, err)
	}
}

func TestParseParams_SinceUntil(t *testing.T) {
	r := okReq()
	r.Since = time.Time{}
	_, err := ParseParams(r, tNow)
	wantInvalid(t, err, "since wajib diisi")

	r = okReq()
	r.Until = time.Time{}
	p, err := ParseParams(r, tNow)
	if err != nil || !p.Until.Equal(tNow) {
		t.Fatalf("p=%+v err=%v", p, err)
	}
	r.Until = tSince
	_, err = ParseParams(r, tNow)
	wantInvalid(t, err, "until harus setelah since")
	r.Until = tSince.Add(-time.Second)
	_, err = ParseParams(r, tNow)
	wantInvalid(t, err, "until harus setelah since")
	r.Until = time.Time{}
	_, err = ParseParams(r, tSince)
	wantInvalid(t, err, "until harus setelah since")
}

func TestParseParams_Window(t *testing.T) {
	r := okReq()
	p, err := ParseParams(r, tNow)
	if err != nil || p.Window != 120*time.Second {
		t.Fatalf("default: %+v %v", p, err)
	}
	for _, w := range []time.Duration{time.Second, time.Hour} {
		r.Window = w
		if p, err := ParseParams(r, tNow); err != nil || p.Window != w {
			t.Errorf("w=%v: %+v %v", w, p, err)
		}
	}
	for _, w := range []time.Duration{999 * time.Millisecond, time.Hour + 1, -time.Second} {
		r.Window = w
		_, err := ParseParams(r, tNow)
		wantInvalid(t, err, "window harus 1s..1h")
	}
}
