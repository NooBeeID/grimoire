package createkey

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"io"
	"log/slog"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"grimoire/internal/domain/project/internal/apikey/base"
	"grimoire/internal/domain/project/internal/apikey/base/audit"
	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
	"grimoire/lib/contracts/core/audit/record"
)

const pid = "0192f5a0-1111-7aaa-8bbb-000000000001"

type journal struct{ events []string }

func (j *journal) add(s string) { j.events = append(j.events, s) }

type fakeTx struct {
	r *fakeRepo
}

func (t fakeTx) LockOwnerKeys(_ context.Context, projectID, ownerID string) error {
	t.r.j.add("lock")
	t.r.lockArgs = append(t.r.lockArgs, [2]string{projectID, ownerID})
	return t.r.lockErr
}
func (t fakeTx) CountActive(_ context.Context, projectID, ownerID string) (int, error) {
	t.r.j.add("count")
	t.r.countArgs = append(t.r.countArgs, [2]string{projectID, ownerID})
	return t.r.count, t.r.countErr
}
func (t fakeTx) Insert(_ context.Context, k base.Key) (base.Key, error) {
	t.r.j.add("insert")
	t.r.inserted = append(t.r.inserted, k)
	t.r.seen = append(t.r.seen, fmt.Sprintf("%+v", k))
	if n := len(t.r.inserted); n <= len(t.r.insertErrs) && t.r.insertErrs[n-1] != nil {
		return base.Key{}, t.r.insertErrs[n-1]
	}
	k.ID = fmt.Sprintf("key-%d", len(t.r.inserted))
	return k, nil
}

type fakeRepo struct {
	j          *journal
	count      int
	lockErr    error
	countErr   error
	insertErrs []error
	inserted   []base.Key
	seen       []string
	txCalls    int
	lockArgs   [][2]string
	countArgs  [][2]string
}

func (f *fakeRepo) InTx(_ context.Context, fn func(Tx) error) error {
	f.txCalls++
	f.j.add("tx")
	err := fn(fakeTx{r: f})
	f.j.add("tx-end")
	return err
}

type fakeAccess struct {
	j     *journal
	err   error
	calls int
}

func (a *fakeAccess) Require(context.Context, string, base.Actor) error {
	a.calls++
	a.j.add("access")
	return a.err
}

type fakeAudit struct {
	j      *journal
	events []audit.Event
}

func (a *fakeAudit) Record(_ context.Context, ev audit.Event) {
	a.j.add("audit")
	a.events = append(a.events, ev)
}

type rig struct {
	uc  *Usecase
	j   *journal
	rp  *fakeRepo
	ac  *fakeAccess
	au  *fakeAudit
	rnd *bytes.Reader
	log *bytes.Buffer
}

var fixedNow = time.Date(2026, 10, 23, 9, 0, 0, 0, time.UTC)

func newRig(publicURL string, limit int) *rig {
	j := &journal{}
	data := make([]byte, 0, 400)
	for i := 0; i < 400; i++ {
		data = append(data, byte(i*7+3)) // deterministik, berbeda tiap blok
	}
	r := &rig{j: j, rp: &fakeRepo{j: j}, ac: &fakeAccess{j: j}, au: &fakeAudit{j: j}, rnd: bytes.NewReader(data), log: &bytes.Buffer{}}
	r.uc = NewUsecase(r.rp, r.ac, r.au, func() time.Time { return fixedNow }, r.rnd, limit, publicURL,
		slog.New(slog.NewTextHandler(r.log, nil)))
	return r
}

func ctxWith() context.Context {
	return reqctx.WithIdentity(context.Background(), reqctx.Identity{UserID: "user-b", Email: "b@tim.id", Role: "developer"})
}

func asErr(t *testing.T, err error) *errs.Error {
	t.Helper()
	var e *errs.Error
	if !errors.As(err, &e) {
		t.Fatalf("bukan *errs.Error: %v", err)
	}
	return e
}

func TestExecute_SuccessOrderAndResult(t *testing.T) {
	r := newRig("https://g.tim.id", 20)
	meta := base.NewRequestMeta("10.0.0.5", "UA")
	res, err := r.uc.Execute(ctxWith(), pid, CreateRequest{Name: "  claude  "}, meta)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"access", "tx", "lock", "count", "insert", "tx-end", "audit"}
	if strings.Join(r.j.events, ",") != strings.Join(want, ",") {
		t.Fatalf("urutan: %v", r.j.events)
	}
	k := res.Key
	if k.Name != "claude" || k.Status != "active" || k.CreatedBy == nil || *k.CreatedBy != "user-b" ||
		k.CreatedByLabel == nil || *k.CreatedByLabel != "b@tim.id" || !k.CreatedAt.Equal(fixedNow) {
		t.Fatalf("%+v", k)
	}
	if len(res.RawKey) != 55 || res.RawKey != "gr_"+k.Prefix+"_"+strings.TrimPrefix(res.RawKey, "gr_"+k.Prefix+"_") {
		t.Fatalf("raw key: %q", res.RawKey)
	}
	if res.MCPURL == nil || *res.MCPURL != "https://g.tim.id/mcp" {
		t.Fatalf("mcp_url: %v", res.MCPURL)
	}
	if len(r.au.events) != 1 {
		t.Fatalf("audit: %d", len(r.au.events))
	}
	ev := r.au.events[0]
	if ev.Action != record.ActionAPIKeyCreated || ev.ProjectID != pid || ev.KeyID != "key-1" ||
		ev.Actor.Label != "b@tim.id" || ev.Meta != meta ||
		len(ev.Args) != 2 || ev.Args["name"] != "claude" || ev.Args["prefix"] != k.Prefix {
		t.Fatalf("event: %+v", ev)
	}
	secret := strings.TrimPrefix(res.RawKey, "gr_"+k.Prefix+"_")
	for _, s := range append(r.rp.seen, r.log.String(), fmt.Sprint(ev.Args)) {
		if strings.Contains(s, secret) || strings.Contains(s, res.RawKey) {
			t.Fatalf("secret bocor: %s", s)
		}
	}
}

func TestExecute_NoPublicURL_NilMCPURL(t *testing.T) {
	r := newRig("", 20)
	res, err := r.uc.Execute(ctxWith(), pid, CreateRequest{Name: "ci"}, base.RequestMeta{})
	if err != nil || res.MCPURL != nil {
		t.Fatalf("%v %v", res.MCPURL, err)
	}
}

func TestExecute_AccessDenied_NothingElse(t *testing.T) {
	r := newRig("", 20)
	r.ac.err = base.ErrProjectNotFound
	_, err := r.uc.Execute(ctxWith(), pid, CreateRequest{Name: "A"}, base.RequestMeta{})
	if !errors.Is(err, base.ErrProjectNotFound) {
		t.Fatalf("err=%v", err)
	}
	if r.rp.txCalls != 0 || len(r.au.events) != 0 || r.rnd.Len() != 400 {
		t.Fatalf("tx=%d audit=%d rnd=%d", r.rp.txCalls, len(r.au.events), r.rnd.Len())
	}
}

func TestExecute_AccessInfraError(t *testing.T) {
	r := newRig("", 20)
	r.ac.err = errors.New("bus down")
	_, err := r.uc.Execute(ctxWith(), pid, CreateRequest{Name: "ci"}, base.RequestMeta{})
	var e *errs.Error
	if err == nil || errors.As(err, &e) || r.rp.txCalls != 0 || len(r.au.events) != 0 {
		t.Fatalf("err=%v", err)
	}
}

func TestExecute_NoIdentity_Unauthorized(t *testing.T) {
	r := newRig("", 20)
	_, err := r.uc.Execute(context.Background(), pid, CreateRequest{Name: "ci"}, base.RequestMeta{})
	if asErr(t, err).Code != errs.CodeUnauthorized || r.ac.calls != 0 || r.rp.txCalls != 0 {
		t.Fatalf("err=%v", err)
	}
}

func TestExecute_InvalidName(t *testing.T) {
	for _, n := range []string{"A", "    ", "a\tb", strings.Repeat("x", 61)} {
		r := newRig("", 20)
		_, err := r.uc.Execute(ctxWith(), pid, CreateRequest{Name: n}, base.RequestMeta{})
		e := asErr(t, err)
		d, _ := e.Details.(map[string]any)
		if e.Code != errs.CodeInvalid || d["fields"] == nil {
			t.Fatalf("%q: %v", n, err)
		}
		if r.rp.txCalls != 0 || len(r.au.events) != 0 || r.rnd.Len() != 400 {
			t.Fatalf("%q: efek samping", n)
		}
	}
	r := newRig("", 20)
	if _, err := r.uc.Execute(ctxWith(), pid, CreateRequest{Name: strings.Repeat("é", 60)}, base.RequestMeta{}); err != nil {
		t.Fatalf("60 rune multibyte harus lulus: %v", err)
	}
}

func TestExecute_Limit(t *testing.T) {
	r := newRig("", 20)
	r.rp.count = 20
	_, err := r.uc.Execute(ctxWith(), pid, CreateRequest{Name: "ci"}, base.RequestMeta{})
	e := asErr(t, err)
	d, _ := e.Details.(map[string]any)
	if e.Code != errs.CodeConflict || d["reason"] != base.ReasonKeyLimitReached || d["limit"] != 20 {
		t.Fatalf("%v", err)
	}
	if len(r.rp.inserted) != 0 || len(r.au.events) != 0 {
		t.Fatal("insert/audit tidak boleh terjadi")
	}
	r2 := newRig("", 20)
	r2.rp.count = 19
	if _, err := r2.uc.Execute(ctxWith(), pid, CreateRequest{Name: "ci"}, base.RequestMeta{}); err != nil {
		t.Fatal(err)
	}
}

func TestExecute_InsertBusinessErrors(t *testing.T) {
	for _, bizErr := range []error{base.ErrNameTaken, base.ErrProjectNotFound} {
		r := newRig("", 20)
		r.rp.insertErrs = []error{bizErr}
		_, err := r.uc.Execute(ctxWith(), pid, CreateRequest{Name: "ci"}, base.RequestMeta{})
		if !errors.Is(err, bizErr) || len(r.au.events) != 0 {
			t.Fatalf("err=%v audit=%d", err, len(r.au.events))
		}
	}
}

func TestExecute_PrefixCollisionRetry(t *testing.T) {
	r := newRig("", 20)
	r.rp.insertErrs = []error{base.ErrPrefixCollision, base.ErrPrefixCollision}
	res, err := r.uc.Execute(ctxWith(), pid, CreateRequest{Name: "ci"}, base.RequestMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if r.rp.txCalls != 3 || len(r.rp.inserted) != 3 {
		t.Fatalf("tx=%d", r.rp.txCalls)
	}
	seenP, seenH := map[string]bool{}, map[string]bool{}
	for _, k := range r.rp.inserted {
		seenP[k.Prefix] = true
		seenH[string(k.SecretHash)] = true
	}
	if len(seenP) != 3 || len(seenH) != 3 {
		t.Fatalf("prefix/hash harus berbeda: %d %d", len(seenP), len(seenH))
	}
	last := r.rp.inserted[2]
	if res.Key.Prefix != last.Prefix || !strings.HasPrefix(res.RawKey, "gr_"+last.Prefix+"_") {
		t.Fatalf("hasil harus dari percobaan ke-3")
	}
	if len(r.au.events) != 1 || r.au.events[0].Args["prefix"] != last.Prefix {
		t.Fatalf("audit: %+v", r.au.events)
	}
	if strings.Contains(r.log.String(), res.RawKey) {
		t.Fatal("log memuat raw key")
	}
}

func TestExecute_PrefixCollisionExhausted(t *testing.T) {
	r := newRig("", 20)
	r.rp.insertErrs = []error{base.ErrPrefixCollision, base.ErrPrefixCollision, base.ErrPrefixCollision}
	_, err := r.uc.Execute(ctxWith(), pid, CreateRequest{Name: "ci"}, base.RequestMeta{})
	var e *errs.Error
	if !errors.Is(err, base.ErrPrefixCollision) || errors.As(err, &e) || r.rp.txCalls != 3 || len(r.au.events) != 0 {
		t.Fatalf("err=%v tx=%d", err, r.rp.txCalls)
	}
}

func TestExecute_InfraErrors(t *testing.T) {
	cases := map[string]func(*fakeRepo){
		"lock":   func(f *fakeRepo) { f.lockErr = errors.New("x") },
		"count":  func(f *fakeRepo) { f.countErr = errors.New("x") },
		"insert": func(f *fakeRepo) { f.insertErrs = []error{errors.New("x")} },
	}
	for name, set := range cases {
		r := newRig("", 20)
		set(r.rp)
		_, err := r.uc.Execute(ctxWith(), pid, CreateRequest{Name: "ci"}, base.RequestMeta{})
		var e *errs.Error
		if err == nil || errors.As(err, &e) || !strings.Contains(err.Error(), "createkey: "+name) || len(r.au.events) != 0 {
			t.Fatalf("%s: %v", name, err)
		}
	}
}

func TestExecute_RandomFailure(t *testing.T) {
	r := newRig("", 20)
	r.uc.rnd = iotest.ErrReader(io.ErrUnexpectedEOF)
	_, err := r.uc.Execute(ctxWith(), pid, CreateRequest{Name: "ci"}, base.RequestMeta{})
	if err == nil || !strings.Contains(err.Error(), "generate key") || r.rp.txCalls != 0 || len(r.au.events) != 0 {
		t.Fatalf("err=%v", err)
	}
}

func TestNoMathRandImport(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", nil, parser.ImportsOnly)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range pkgs {
		for fn, f := range p.Files {
			for _, im := range f.Imports {
				if strings.Contains(im.Path.Value, "math/rand") {
					t.Fatalf("%s mengimpor %s", fn, im.Path.Value)
				}
			}
		}
	}
}

func ctxAs(id, email, role string) context.Context {
	return reqctx.WithIdentity(context.Background(), reqctx.Identity{UserID: id, Email: email, Role: role})
}

func TestExecute_LockAndCountUseProjectAndActor(t *testing.T) {
	r := newRig("", 20)
	if _, err := r.uc.Execute(ctxWith(), pid, CreateRequest{Name: "ci"}, base.RequestMeta{}); err != nil {
		t.Fatal(err)
	}
	want := [][2]string{{pid, "user-b"}}
	if fmt.Sprint(r.rp.lockArgs) != fmt.Sprint(want) || fmt.Sprint(r.rp.countArgs) != fmt.Sprint(want) {
		t.Fatalf("lock=%v count=%v", r.rp.lockArgs, r.rp.countArgs)
	}
	if r.rp.inserted[0].CreatedBy == nil || *r.rp.inserted[0].CreatedBy != "user-b" {
		t.Fatalf("created_by: %v", r.rp.inserted[0].CreatedBy)
	}
}

func TestExecute_SuperAdminUsesOwnID(t *testing.T) {
	r := newRig("", 20)
	if _, err := r.uc.Execute(ctxAs("user-s", "s@tim.id", "super_admin"), pid, CreateRequest{Name: "ci"}, base.RequestMeta{}); err != nil {
		t.Fatal(err)
	}
	want := [][2]string{{pid, "user-s"}}
	if fmt.Sprint(r.rp.lockArgs) != fmt.Sprint(want) || fmt.Sprint(r.rp.countArgs) != fmt.Sprint(want) {
		t.Fatalf("lock=%v count=%v", r.rp.lockArgs, r.rp.countArgs)
	}
	if k := r.rp.inserted[0]; k.CreatedBy == nil || *k.CreatedBy != "user-s" {
		t.Fatalf("created_by: %v", k.CreatedBy)
	}
	if len(r.au.events) != 1 || r.au.events[0].Action != record.ActionAPIKeyCreated || r.au.events[0].Actor.ID != "user-s" {
		t.Fatalf("audit: %+v", r.au.events)
	}
}

func TestExecute_LimitReached_DetailsLimit(t *testing.T) {
	r := newRig("", 20)
	r.rp.count = 20
	_, err := r.uc.Execute(ctxWith(), pid, CreateRequest{Name: "ci"}, base.RequestMeta{})
	e := asErr(t, err)
	d, _ := e.Details.(map[string]any)
	if e.Code != errs.CodeConflict || d["reason"] != base.ReasonKeyLimitReached || d["limit"] != 20 ||
		!strings.Contains(e.Message, "per user per project") {
		t.Fatalf("%v", err)
	}
	if len(r.rp.inserted) != 0 || len(r.au.events) != 0 {
		t.Fatal("insert/audit tidak boleh terjadi")
	}
}

func TestExecute_NameTakenPassedThrough(t *testing.T) {
	r := newRig("", 20)
	r.rp.insertErrs = []error{base.ErrNameTaken}
	_, err := r.uc.Execute(ctxWith(), pid, CreateRequest{Name: "ci"}, base.RequestMeta{})
	d, _ := asErr(t, err).Details.(map[string]any)
	if !errors.Is(err, base.ErrNameTaken) || d["reason"] != base.ReasonNameTaken || r.rp.txCalls != 1 || len(r.au.events) != 0 {
		t.Fatalf("err=%v tx=%d", err, r.rp.txCalls)
	}
}

func TestExecute_PrefixCollisionRetry_RelocksSameOwner(t *testing.T) {
	r := newRig("", 20)
	r.rp.insertErrs = []error{base.ErrPrefixCollision}
	if _, err := r.uc.Execute(ctxWith(), pid, CreateRequest{Name: "ci"}, base.RequestMeta{}); err != nil {
		t.Fatal(err)
	}
	want := [][2]string{{pid, "user-b"}, {pid, "user-b"}}
	if r.rp.txCalls != 2 || fmt.Sprint(r.rp.lockArgs) != fmt.Sprint(want) || fmt.Sprint(r.rp.countArgs) != fmt.Sprint(want) || len(r.au.events) != 1 {
		t.Fatalf("tx=%d lock=%v count=%v", r.rp.txCalls, r.rp.lockArgs, r.rp.countArgs)
	}
}
