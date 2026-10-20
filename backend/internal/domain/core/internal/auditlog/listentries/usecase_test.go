package listentries

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"grimoire/internal/domain/core/internal/auditlog/base"
)

type fakeRepo struct {
	rows  []base.Entry
	err   error
	limit int
}

func (f *fakeRepo) List(_ context.Context, _ base.Filter, limit int) ([]base.Entry, error) {
	f.limit = limit
	if f.err != nil {
		return nil, f.err
	}
	if limit < len(f.rows) {
		return f.rows[:limit], nil
	}
	return f.rows, nil
}

func entries(n int) []base.Entry {
	base0 := time.Date(2026, 10, 19, 8, 0, 0, 123456000, time.UTC)
	out := make([]base.Entry, n)
	for i := range out {
		out[i] = base.Entry{
			ID:         fmt.Sprintf("00000000-0000-7000-8000-%012d", n-i),
			OccurredAt: base0.Add(-time.Duration(i) * time.Second),
			ActorType:  "system", ActorLabel: "system", Action: "a.b", Status: "ok",
		}
	}
	return out
}

func TestUsecase_NextCursorFromLastKeptRow(t *testing.T) {
	rows := entries(4)
	repo := &fakeRepo{rows: rows}
	res, err := NewUsecase(repo).List(context.Background(), base.Filter{Limit: 3})
	if err != nil {
		t.Fatal(err)
	}
	if repo.limit != 4 || len(res.Items) != 3 || res.NextCursor == nil {
		t.Fatalf("limit=%d items=%d next=%v", repo.limit, len(res.Items), res.NextCursor)
	}
	c, err := base.DecodeCursor(*res.NextCursor)
	if err != nil {
		t.Fatal(err)
	}
	if c.ID != rows[2].ID || !c.OccurredAt.Equal(rows[2].OccurredAt) {
		t.Fatalf("cursor %+v bukan baris ke-3", c)
	}
}

func TestUsecase_NoNextCursor(t *testing.T) {
	for _, n := range []int{0, 2, 3} {
		res, err := NewUsecase(&fakeRepo{rows: entries(n)}).List(context.Background(), base.Filter{Limit: 3})
		if err != nil || res.NextCursor != nil || len(res.Items) != n {
			t.Fatalf("n=%d: %v %+v", n, err, res)
		}
	}
}

func TestUsecase_EmptyJSON(t *testing.T) {
	res, _ := NewUsecase(&fakeRepo{}).List(context.Background(), base.Filter{Limit: 5})
	b, _ := json.Marshal(res)
	if string(b) != `{"items":[],"next_cursor":null}` {
		t.Fatal(string(b))
	}
}

func TestUsecase_RequestsLimitPlusOne(t *testing.T) {
	for _, c := range []struct {
		raw  string
		want int
	}{{"", 51}, {"limit=200", 201}} {
		f, err := parseValidate(c.raw)
		if err != nil {
			t.Fatal(err)
		}
		repo := &fakeRepo{}
		if _, err := NewUsecase(repo).List(context.Background(), f); err != nil {
			t.Fatal(err)
		}
		if repo.limit != c.want {
			t.Fatalf("%q: limit %d, want %d", c.raw, repo.limit, c.want)
		}
	}
}

func TestUsecase_SummaryOmitsSensitiveKeys(t *testing.T) {
	e := entries(1)
	e[0].Args = map[string]any{"secret": 1}
	e[0].IP = "1.2.3.4"
	e[0].UserAgent = "ua"
	e[0].TraceID = "tr"
	res, _ := NewUsecase(&fakeRepo{rows: e}).List(context.Background(), base.Filter{Limit: 5})
	b, _ := json.Marshal(res.Items[0])
	m := map[string]any{}
	_ = json.Unmarshal(b, &m)
	for _, k := range []string{"args", "result", "ip", "user_agent", "trace_id"} {
		if _, ok := m[k]; ok {
			t.Fatalf("kunci %s bocor", k)
		}
	}
	for _, k := range []string{"actor_id", "project_id", "project_label", "target_type", "target"} {
		if v, ok := m[k]; !ok || v != nil {
			t.Fatalf("%s harus null: %v", k, v)
		}
	}
}

func TestUsecase_RepoErrorWrapped(t *testing.T) {
	boom := errors.New("boom")
	_, err := NewUsecase(&fakeRepo{err: boom}).List(context.Background(), base.Filter{Limit: 5})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
}
