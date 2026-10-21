package scanentries

import (
	"context"
	"errors"
	"testing"
	"time"

	"grimoire/lib/common/errs"
	"grimoire/lib/contracts/core/audit/scanentries"
)

type fakeRepo struct {
	data  []scanentries.Entry
	calls int
	limit int
	last  params
	err   error
}

func (f *fakeRepo) Scan(_ context.Context, p params, limit int) ([]scanentries.Entry, error) {
	f.calls++
	f.limit = limit
	f.last = p
	if f.err != nil {
		return nil, f.err
	}
	var out []scanentries.Entry
	for _, e := range f.data {
		if p.After != nil {
			a := p.After
			if e.OccurredAt.Before(a.OccurredAt) || (e.OccurredAt.Equal(a.OccurredAt) && e.ID <= a.ID) {
				continue
			}
		}
		out = append(out, e)
		if len(out) == limit {
			break
		}
	}
	return out, nil
}

func newUC(r Repository) *Usecase { return NewUsecase(r, func() time.Time { return now }) }

func seven() []scanentries.Entry {
	var es []scanentries.Entry
	for i := 0; i < 7; i++ {
		es = append(es, scanentries.Entry{ID: "0192f0a4-6c1e-7b3a-9c55-2f1e8d7a400" + string(rune('0'+i)), OccurredAt: t0.Add(time.Duration(i/2) * time.Second)})
	}
	return es
}

func TestUsecase_InvalidSkipsRepo(t *testing.T) {
	r := &fakeRepo{}
	in := validReq()
	in.ProjectID = "x"
	if _, err := newUC(r).Execute(context.Background(), in); !errors.Is(err, errs.ErrInvalid) || r.calls != 0 {
		t.Fatalf("err=%v calls=%d", err, r.calls)
	}
}

func TestUsecase_LimitPlusOne(t *testing.T) {
	r := &fakeRepo{}
	uc := newUC(r)
	if _, err := uc.Execute(context.Background(), validReq()); err != nil || r.limit != 501 {
		t.Fatalf("limit=%d err=%v", r.limit, err)
	}
	in := validReq()
	in.Limit = 3
	_, _ = uc.Execute(context.Background(), in)
	if r.limit != 4 {
		t.Fatalf("limit=%d", r.limit)
	}
}

func TestUsecase_Paging(t *testing.T) {
	r := &fakeRepo{data: seven()}
	uc := newUC(r)
	in := validReq()
	in.Limit = 3
	var got []string
	sizes := []int{}
	for i := 0; i < 5; i++ {
		resp, err := uc.Execute(context.Background(), in)
		if err != nil {
			t.Fatal(err)
		}
		sizes = append(sizes, len(resp.Entries))
		for _, e := range resp.Entries {
			got = append(got, e.ID)
		}
		if resp.Next == nil {
			break
		}
		last := resp.Entries[len(resp.Entries)-1]
		if resp.Next.ID != last.ID || !resp.Next.OccurredAt.Equal(last.OccurredAt) {
			t.Fatalf("next=%+v last=%+v", resp.Next, last)
		}
		in.After = resp.Next
	}
	if len(sizes) != 3 || sizes[0] != 3 || sizes[1] != 3 || sizes[2] != 1 {
		t.Fatalf("sizes=%v", sizes)
	}
	seen := map[string]bool{}
	for _, id := range got {
		if seen[id] {
			t.Fatalf("dup %s", id)
		}
		seen[id] = true
	}
	if len(got) != 7 {
		t.Fatalf("got=%d", len(got))
	}
}

func TestUsecase_ExactLimitNoNext(t *testing.T) {
	r := &fakeRepo{data: seven()[:3]}
	in := validReq()
	in.Limit = 3
	resp, err := newUC(r).Execute(context.Background(), in)
	if err != nil || len(resp.Entries) != 3 || resp.Next != nil {
		t.Fatalf("resp=%+v err=%v", resp, err)
	}
}

func TestUsecase_EmptyNonNil(t *testing.T) {
	resp, err := newUC(&fakeRepo{}).Execute(context.Background(), validReq())
	if err != nil || resp.Entries == nil || len(resp.Entries) != 0 || resp.Next != nil {
		t.Fatalf("resp=%+v err=%v", resp, err)
	}
}

func TestUsecase_RepoError(t *testing.T) {
	boom := errors.New("boom")
	resp, err := newUC(&fakeRepo{err: boom}).Execute(context.Background(), validReq())
	if !errors.Is(err, boom) || resp.Entries != nil || resp.Next != nil {
		t.Fatalf("resp=%+v err=%v", resp, err)
	}
}

func TestUsecase_NowUsedForZeroTo(t *testing.T) {
	r := &fakeRepo{}
	_, _ = newUC(r).Execute(context.Background(), validReq())
	if !r.last.To.Equal(now) {
		t.Fatalf("to=%v", r.last.To)
	}
}
