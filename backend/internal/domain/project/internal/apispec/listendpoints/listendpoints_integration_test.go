//go:build integration

package listendpoints

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"testing"
	"time"

	"grimoire/internal/infra/app"
	"grimoire/internal/infra/bus"
	"grimoire/internal/infra/db"
	"grimoire/lib/common/errs"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/project/apispec/listendpoints"
)

const actorID = "0b6c0000-0000-4000-8000-000000000001"

type env struct {
	d   *db.DB
	reg *bus.Registry
}

func setup(t *testing.T) *env {
	t.Helper()
	d := db.TestDB(t)
	if _, err := db.Up(context.Background(), d, nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	reg := bus.NewRegistry(slog.New(slog.NewTextHandler(io.Discard, nil)))
	Init(app.Deps{DB: d, Bus: reg})
	return &env{d: d, reg: reg}
}

func (e *env) project(t *testing.T, name string) string {
	t.Helper()
	var id string
	if err := e.d.GetContext(context.Background(), &id, `INSERT INTO projects (name) VALUES ($1) RETURNING id::text`, "Proyek "+name); err != nil {
		t.Fatal(err)
	}
	return id
}

func (e *env) spec(t *testing.T, pid, name string) string {
	t.Helper()
	var id string
	err := e.d.GetContext(context.Background(), &id, `INSERT INTO api_specs (project_id, name, title, api_version, openapi_version,
content, content_format, content_hash, size_bytes, created_by, created_by_label, updated_by, updated_by_label)
VALUES ($1,$2,'T','1','3.0.3','x','yaml',decode(repeat('00',32),'hex'),1,$3,'b',$3,'b') RETURNING id::text`, pid, name, actorID)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func (e *env) ep(t *testing.T, specID, method, path, op, summary string, tags ...string) {
	t.Helper()
	var opv *string
	if op != "" {
		opv = &op
	}
	if tags == nil {
		tags = []string{}
	}
	_, err := e.d.ExecContext(context.Background(), `INSERT INTO api_spec_endpoints (spec_id, method, path, operation_id, summary, tags)
VALUES ($1,$2,$3,$4,$5,$6)`, specID, method, path, opv, summary, tags)
	if err != nil {
		t.Fatal(err)
	}
}

func (e *env) call(t *testing.T, req listendpoints.Request) listendpoints.Response {
	t.Helper()
	res, err := bus.CallTyped[listendpoints.Response](context.Background(), e.reg, constants.BUS_PROJECT_APISPEC_LIST_ENDPOINTS, req)
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	return res
}

func paths(r listendpoints.Response) []string {
	out := []string{}
	for _, it := range r.Items {
		out = append(out, it.Path)
	}
	return out
}

func eq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func seedBig(t *testing.T, e *env, pid, name string, n int) {
	t.Helper()
	sid := e.spec(t, pid, name)
	_, err := e.d.ExecContext(context.Background(), `INSERT INTO api_spec_endpoints (spec_id, method, path, operation_id, summary, tags)
SELECT $1::uuid, (ARRAY['GET','PUT','POST','DELETE'])[1 + g % 4], '/r/' || g || '/' || ($2::text), 'op' || g || $2::text,
       CASE WHEN g % 50 = 0 THEN 'refund item ' || g ELSE 'item ' || g END,
       CASE WHEN g % 3 = 0 THEN ARRAY['payments'] ELSE ARRAY['other','misc'] END
FROM generate_series(1, $3::int) g`, sid, name, n)
	if err != nil {
		t.Fatal(err)
	}
}

func TestT06_AC4_ListEndpoints500Fast(t *testing.T) {
	e := setup(t)
	pid := e.project(t, "P")
	seedBig(t, e, pid, "big", 500)
	req := listendpoints.Request{ProjectID: pid, Spec: "big", Limit: 300}
	e.call(t, req)
	var ds []time.Duration
	for i := 0; i < 5; i++ {
		st := time.Now()
		res := e.call(t, req)
		ds = append(ds, time.Since(st))
		if res.Total != 500 || len(res.Items) != 300 || !res.Truncated || res.NextOffset != 300 {
			t.Fatalf("%+v", res.Total)
		}
	}
	sort.Slice(ds, func(i, j int) bool { return ds[i] < ds[j] })
	if ds[2] > 100*time.Millisecond {
		t.Fatalf("median %v", ds[2])
	}
}

func TestT06_AC4_HeavyProjectAndExplain(t *testing.T) {
	e := setup(t)
	pid := e.project(t, "P")
	for i := 0; i < 100; i++ {
		seedBig(t, e, pid, fmt.Sprintf("s%03d", i), 500)
	}
	req := listendpoints.Request{ProjectID: pid, Query: "refund"}
	e.call(t, req)
	st := time.Now()
	res := e.call(t, req)
	if d := time.Since(st); d > 100*time.Millisecond {
		t.Logf("PERLU PERHATIAN: 100x500 query teks %v (> 100ms)", d)
	}
	if res.Total != 100*10 {
		t.Fatalf("total=%d", res.Total)
	}
	var plan []string
	err := e.d.SelectContext(context.Background(), &plan, "EXPLAIN (ANALYZE, BUFFERS) "+mainQuery, pid, "", "", "%refund%", 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range plan {
		t.Log(l)
	}
}

func TestT06_AC4_FilterByTagAndText(t *testing.T) {
	e := setup(t)
	pid := e.project(t, "P")
	s := e.spec(t, pid, "pay")
	e.ep(t, s, "GET", "/a", "", "x", "payments")
	e.ep(t, s, "GET", "/b", "", "x", "Payments")
	e.ep(t, s, "GET", "/c", "", "x", "pay")
	e.ep(t, s, "GET", "/d", "", "x", "other", "payments")
	e.ep(t, s, "GET", "/e", "", "x")
	if got := paths(e.call(t, listendpoints.Request{ProjectID: pid, Tag: "payments"})); !eq(got, []string{"/a", "/d"}) {
		t.Fatalf("tag: %v", got)
	}
	e.ep(t, s, "POST", "/f/Refund", "", "none")
	e.ep(t, s, "POST", "/g", "refundIt", "none")
	e.ep(t, s, "POST", "/h", "", "Please REFUND")
	if got := paths(e.call(t, listendpoints.Request{ProjectID: pid, Query: "refund"})); !eq(got, []string{"/f/Refund", "/g", "/h"}) {
		t.Fatalf("text: %v", got)
	}
	if got := paths(e.call(t, listendpoints.Request{ProjectID: pid, Query: "refund", Tag: "payments"})); len(got) != 0 {
		t.Fatalf("and: %v", got)
	}
}

func TestT06_AC4_TextEscape(t *testing.T) {
	e := setup(t)
	pid := e.project(t, "P")
	s := e.spec(t, pid, "x")
	e.ep(t, s, "GET", "/discount/100%", "", "")
	e.ep(t, s, "GET", "/discount/1000", "", "")
	e.ep(t, s, "GET", "/a_b", "", "")
	e.ep(t, s, "GET", "/axb", "", "")
	e.ep(t, s, "GET", "/win", "", `C:\temp`)
	cases := []struct {
		q    string
		want []string
	}{
		{"%", []string{"/discount/100%"}},
		{"100%", []string{"/discount/100%"}},
		{"a_b", []string{"/a_b"}},
		{"_", []string{"/a_b"}},
		{`\`, []string{"/win"}},
		{`c:\t`, []string{"/win"}},
	}
	for _, c := range cases {
		if got := paths(e.call(t, listendpoints.Request{ProjectID: pid, Query: c.q})); !eq(got, c.want) {
			t.Errorf("q=%q got %v want %v", c.q, got, c.want)
		}
	}
}

func TestT06_AC4_SQLInjectionStringsAreLiteral(t *testing.T) {
	e := setup(t)
	pid := e.project(t, "P")
	s := e.spec(t, pid, "x")
	e.ep(t, s, "GET", "/a", "", "")
	for _, inj := range []string{"' OR 1=1 --", "\"; DROP TABLE api_specs;--"} {
		if r := e.call(t, listendpoints.Request{ProjectID: pid, Query: inj}); len(r.Items) != 0 {
			t.Fatalf("query: %+v", r)
		}
		if r := e.call(t, listendpoints.Request{ProjectID: pid, Tag: inj}); len(r.Items) != 0 {
			t.Fatalf("tag: %+v", r)
		}
		if r := e.call(t, listendpoints.Request{ProjectID: pid, Spec: inj}); r.SpecFound {
			t.Fatalf("spec: %+v", r)
		}
	}
	if r := e.call(t, listendpoints.Request{ProjectID: pid}); r.Total != 1 {
		t.Fatal("tabel rusak")
	}
}

func TestT06_AC4_PaginationOrderAndOffsetBeyond(t *testing.T) {
	e := setup(t)
	pid := e.project(t, "P")
	a, b := e.spec(t, pid, "a"), e.spec(t, pid, "b")
	for i := 0; i < 7; i++ {
		e.ep(t, a, "GET", fmt.Sprintf("/a%d", i), "", "")
	}
	for i := 0; i < 6; i++ {
		e.ep(t, b, "GET", fmt.Sprintf("/b%d", i), "", "")
	}
	all := e.call(t, listendpoints.Request{ProjectID: pid, Limit: 300})
	var got []string
	off := 0
	for {
		r := e.call(t, listendpoints.Request{ProjectID: pid, Limit: 5, Offset: off})
		if r.Total != 13 {
			t.Fatalf("total=%d", r.Total)
		}
		got = append(got, paths(r)...)
		if !r.Truncated {
			if r.NextOffset != 0 {
				t.Fatal("next")
			}
			break
		}
		off = r.NextOffset
	}
	if !eq(got, paths(all)) || len(got) != 13 {
		t.Fatalf("%v", got)
	}
	r := e.call(t, listendpoints.Request{ProjectID: pid, Offset: 1000})
	if r.Items == nil || len(r.Items) != 0 || r.Total != 13 || r.Truncated || r.NextOffset != 0 {
		t.Fatalf("%+v", r)
	}
}

func TestT06_AC4_CanonicalOrder(t *testing.T) {
	e := setup(t)
	pid := e.project(t, "P")
	s := e.spec(t, pid, "x")
	for _, m := range []string{"TRACE", "PATCH", "HEAD", "OPTIONS", "DELETE", "POST", "PUT", "GET"} {
		e.ep(t, s, m, "/b", "", "")
		e.ep(t, s, m, "/a", "", "")
	}
	e.ep(t, s, "GET", "/Z", "", "")
	r := e.call(t, listendpoints.Request{ProjectID: pid})
	if r.Items[0].Path != "/Z" || r.Items[1].Path != "/a" || r.Items[1].Method != "GET" || r.Items[2].Method != "PUT" || r.Items[8].Path != "/a" || r.Items[9].Path != "/b" {
		t.Fatalf("%+v", r.Items)
	}
}

func TestT06_AC4_ClampOverBus(t *testing.T) {
	e := setup(t)
	pid := e.project(t, "P")
	seedBig(t, e, pid, "big", 500)
	if r := e.call(t, listendpoints.Request{ProjectID: pid, Limit: 100000}); len(r.Items) != 300 {
		t.Fatal(len(r.Items))
	}
	if r := e.call(t, listendpoints.Request{ProjectID: pid, Limit: 0, Offset: -10}); len(r.Items) != 100 {
		t.Fatal(len(r.Items))
	}
}

func TestT06_AC4_UnknownSpecEmptyAndInvalid(t *testing.T) {
	e := setup(t)
	pid := e.project(t, "P")
	empty := e.project(t, "E")
	e.spec(t, pid, "auth")
	e.spec(t, pid, "payment")
	r := e.call(t, listendpoints.Request{ProjectID: pid, Spec: "nope"})
	if r.SpecFound || !eq(r.Specs, []string{"auth", "payment"}) || r.Items == nil || len(r.Items) != 0 {
		t.Fatalf("%+v", r)
	}
	if r := e.call(t, listendpoints.Request{ProjectID: empty}); !r.SpecFound || r.Total != 0 || r.Items == nil {
		t.Fatalf("%+v", r)
	}
	_, err := bus.CallTyped[listendpoints.Response](context.Background(), e.reg, constants.BUS_PROJECT_APISPEC_LIST_ENDPOINTS, listendpoints.Request{ProjectID: "bukan-uuid"})
	if !errors.Is(err, errs.ErrInvalid) {
		t.Fatalf("err=%v", err)
	}
}

func TestT06_AC7_ProjectIsolation(t *testing.T) {
	e := setup(t)
	a, b := e.project(t, "A"), e.project(t, "B")
	pa, pb, bb := e.spec(t, a, "payment"), e.spec(t, b, "payment"), e.spec(t, b, "billing")
	e.ep(t, pa, "GET", "/only-a", "", "")
	e.ep(t, pb, "GET", "/only-b", "", "")
	e.ep(t, bb, "GET", "/bill", "", "")
	for _, sp := range []string{"payment", ""} {
		r := e.call(t, listendpoints.Request{ProjectID: a, Spec: sp})
		if !eq(paths(r), []string{"/only-a"}) || r.Total != 1 {
			t.Fatalf("spec=%q %+v", sp, r)
		}
	}
	r := e.call(t, listendpoints.Request{ProjectID: a, Spec: "billing"})
	if r.SpecFound || !eq(r.Specs, []string{"payment"}) {
		t.Fatalf("%+v", r)
	}
	if r := e.call(t, listendpoints.Request{ProjectID: "0b6c2e1a-7d4f-4c1e-9a55-3f2b8c9d1e10"}); len(r.Items) != 0 || r.Total != 0 {
		t.Fatalf("%+v", r)
	}
}

func TestListEndpoints_DeleteSpecRemovesFromIndex(t *testing.T) {
	e := setup(t)
	pid := e.project(t, "P")
	s := e.spec(t, pid, "x")
	e.ep(t, s, "GET", "/a", "", "")
	if _, err := e.d.ExecContext(context.Background(), `DELETE FROM api_specs WHERE id=$1`, s); err != nil {
		t.Fatal(err)
	}
	if r := e.call(t, listendpoints.Request{ProjectID: pid}); r.Total != 0 {
		t.Fatalf("%+v", r)
	}
}
