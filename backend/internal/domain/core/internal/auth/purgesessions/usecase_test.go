package purgesessions

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"
)

type fakeRepo struct {
	sCalls, tCalls         int
	sNow, sCut, tNow, tCut time.Time
	sN, tN                 int64
	sErr, tErr             error
}

func (f *fakeRepo) DeleteExpiredSessions(_ context.Context, now, cut time.Time) (int64, error) {
	f.sCalls++
	f.sNow, f.sCut = now, cut
	return f.sN, f.sErr
}

func (f *fakeRepo) DeleteExpiredThrottles(_ context.Context, now, cut time.Time) (int64, error) {
	f.tCalls++
	f.tNow, f.tCut = now, cut
	return f.tN, f.tErr
}

var t0 = time.Date(2026, 10, 20, 3, 0, 0, 0, time.UTC)

func newUC(f *fakeRepo, clock func() time.Time) (*Usecase, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	log := slog.New(slog.NewTextHandler(buf, nil))
	return New(f, clock, 12*time.Hour, 15*time.Minute, log), buf
}

func TestRun_ComputesCutoffsOnce(t *testing.T) {
	f := &fakeRepo{}
	calls := 0
	uc, _ := newUC(f, func() time.Time { calls++; return t0.Add(time.Duration(calls) * time.Hour) })
	if err := uc.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("clock dipanggil %d kali", calls)
	}
	if !f.sNow.Equal(f.tNow) {
		t.Fatal("now berbeda antar repo")
	}
}

func TestRun_CutoffValues(t *testing.T) {
	f := &fakeRepo{}
	uc, _ := newUC(f, func() time.Time { return t0 })
	if err := uc.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !f.sNow.Equal(t0) || !f.sCut.Equal(time.Date(2026, 10, 19, 15, 0, 0, 0, time.UTC)) {
		t.Fatalf("sessions: %v %v", f.sNow, f.sCut)
	}
	if !f.tCut.Equal(time.Date(2026, 10, 20, 2, 45, 0, 0, time.UTC)) {
		t.Fatalf("throttles cutoff: %v", f.tCut)
	}
}

func TestRun_LogsCounts(t *testing.T) {
	f := &fakeRepo{sN: 7, tN: 3}
	uc, buf := newUC(f, func() time.Time { return t0 })
	if err := uc.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, w := range []string{"auth purge sessions selesai", "deleted_sessions=7", "deleted_throttles=3", "duration_ms="} {
		if !strings.Contains(out, w) {
			t.Errorf("log tanpa %q: %s", w, out)
		}
	}
	if strings.Contains(out, "level=ERROR") {
		t.Error("tidak boleh ada log error")
	}
}

func TestRun_ZeroDeletedIsSuccess(t *testing.T) {
	uc, buf := newUC(&fakeRepo{}, func() time.Time { return t0 })
	if err := uc.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "deleted_sessions=0 deleted_throttles=0") {
		t.Fatal(buf.String())
	}
}

func TestRun_SessionsErrorStopsAndReturns(t *testing.T) {
	sentinel := errors.New("boom")
	f := &fakeRepo{sErr: sentinel}
	uc, _ := newUC(f, func() time.Time { return t0 })
	err := uc.Run(context.Background())
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v", err)
	}
	if f.tCalls != 0 {
		t.Fatal("throttles tidak boleh dipanggil")
	}
}

func TestRun_ThrottlesErrorReturnsAfterSessions(t *testing.T) {
	sentinel := errors.New("boom")
	f := &fakeRepo{sN: 5, tErr: sentinel}
	uc, buf := newUC(f, func() time.Time { return t0 })
	err := uc.Run(context.Background())
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(buf.String(), "deleted_sessions=5") {
		t.Fatal(buf.String())
	}
}

func TestRun_ContextCanceledBeforeStart(t *testing.T) {
	f := &fakeRepo{}
	uc, _ := newUC(f, func() time.Time { return t0 })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := uc.Run(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
	if f.sCalls != 0 || f.tCalls != 0 {
		t.Fatal("repo tidak boleh dipanggil")
	}
}

func TestNew_RejectsNonPositiveDurations(t *testing.T) {
	cases := [][2]time.Duration{{0, time.Minute}, {time.Minute, 0}, {-1, time.Minute}, {time.Minute, -1}}
	for _, c := range cases {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("tidak panic untuk %v", c)
				}
			}()
			New(&fakeRepo{}, func() time.Time { return t0 }, c[0], c[1], slog.Default())
		}()
	}
}

func TestRun_NoSecretsInLog(t *testing.T) {
	uc, buf := newUC(&fakeRepo{sN: 1, tN: 1}, func() time.Time { return t0 })
	_ = uc.Run(context.Background())
	for _, bad := range []string{"email", "token", "user_id"} {
		if strings.Contains(buf.String(), bad) {
			t.Errorf("log memuat %q", bad)
		}
	}
}

func TestNewJob_NameAndSchedule(t *testing.T) {
	j := newJob(nil)
	if j.Name != "core.auth.purge-sessions" || j.DailyAt != "03:00" || j.Every != 0 {
		t.Fatalf("job = %+v", j)
	}
}
