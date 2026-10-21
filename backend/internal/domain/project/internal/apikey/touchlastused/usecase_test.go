package touchlastused

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"grimoire/lib/contracts/project/apikey/touch"
)

type fakeRepo struct {
	calls int
	id    string
	now   time.Time
	n     int64
	err   error
}

func (f *fakeRepo) Touch(_ context.Context, id string, now time.Time) (int64, error) {
	f.calls++
	f.id, f.now = id, now
	return f.n, f.err
}

const validID = "0192f5a0-2222-7aaa-8bbb-000000000001"

var t0 = time.Date(2026, 10, 23, 11, 42, 0, 0, time.UTC)

func newUC(r *fakeRepo) (*Usecase, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	log := slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	return NewUsecase(r, func() time.Time { return t0 }, log), buf
}

func TestTouch_Written(t *testing.T) {
	r := &fakeRepo{n: 1}
	uc, buf := newUC(r)
	uc.Touch(context.Background(), touch.Request{KeyID: validID})
	if r.calls != 1 || r.id != validID || !r.now.Equal(t0) {
		t.Fatalf("%+v", r)
	}
	if strings.Contains(buf.String(), "level=WARN") {
		t.Fatal(buf.String())
	}
}

func TestTouch_Throttled(t *testing.T) {
	r := &fakeRepo{n: 0}
	uc, buf := newUC(r)
	uc.Touch(context.Background(), touch.Request{KeyID: validID})
	if r.calls != 1 || strings.Contains(buf.String(), "level=WARN") {
		t.Fatal(buf.String())
	}
}

func TestTouch_InvalidUUIDSkipsRepo(t *testing.T) {
	for _, id := range []string{"", "bukan-uuid", "123", "' OR 1=1 --"} {
		r := &fakeRepo{}
		uc, buf := newUC(r)
		uc.Touch(context.Background(), touch.Request{KeyID: id})
		if r.calls != 0 {
			t.Fatalf("%q: repo called", id)
		}
		if strings.Contains(buf.String(), "level=WARN") || !strings.Contains(buf.String(), "level=DEBUG") {
			t.Fatalf("%q: %s", id, buf.String())
		}
	}
}

func TestTouch_RepoErrorLogsWarnOnce(t *testing.T) {
	r := &fakeRepo{err: errors.New("boom")}
	uc, buf := newUC(r)
	uc.Touch(context.Background(), touch.Request{KeyID: validID})
	out := buf.String()
	if strings.Count(out, "level=WARN") != 1 || !strings.Contains(out, "key_id="+validID) || !strings.Contains(out, "error=boom") {
		t.Fatal(out)
	}
}
