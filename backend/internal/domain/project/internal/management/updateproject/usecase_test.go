package updateproject

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"grimoire/internal/domain/project/internal/management/base"
	"grimoire/internal/domain/project/internal/management/base/audit"
	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
	"grimoire/lib/contracts/core/audit/record"
)

var (
	t0   = time.Date(2026, 10, 22, 9, 0, 0, 0, time.UTC)
	tNow = time.Date(2026, 10, 22, 10, 0, 0, 0, time.UTC)
)

type fakeRepo struct {
	log        *[]string
	project    base.Project
	memberRole string
	found      bool
	lockOK     bool
	members    []base.Member
	updated    []base.Project
	repoAt     *time.Time
	failUpdate error
	failList   error
	inTx       int
}

func (f *fakeRepo) FindAccess(context.Context, string, string) (base.Project, string, bool, error) {
	*f.log = append(*f.log, "FindAccess")
	return f.project, f.memberRole, f.found, nil
}

func (f *fakeRepo) ListMembers(context.Context, string) ([]base.Member, error) {
	*f.log = append(*f.log, "ListMembers")
	return f.members, f.failList
}

func (f *fakeRepo) InTx(_ context.Context, fn func(TxRepository) error) error {
	f.inTx++
	*f.log = append(*f.log, "tx-begin")
	err := fn(f)
	if err == nil {
		*f.log = append(*f.log, "tx-commit")
	}
	return err
}

func (f *fakeRepo) LockProject(context.Context, string) (base.Project, bool, error) {
	*f.log = append(*f.log, "LockProject")
	return f.project, f.lockOK, nil
}

func (f *fakeRepo) UpdateProject(_ context.Context, p base.Project) (time.Time, error) {
	*f.log = append(*f.log, "UpdateProject")
	if f.failUpdate != nil {
		return time.Time{}, f.failUpdate
	}
	f.updated = append(f.updated, p)
	if f.repoAt != nil {
		return *f.repoAt, nil
	}
	return p.UpdatedAt, nil
}

type fakeAudit struct {
	log    *[]string
	events []audit.Event
}

func (a *fakeAudit) Record(_ context.Context, ev audit.Event) {
	*a.log = append(*a.log, "audit")
	a.events = append(a.events, ev)
}

type fakeCounts struct {
	log *[]string
	c   base.ContentCounts
}

func (c *fakeCounts) ForProjects(_ context.Context, ids []string) map[string]base.ContentCounts {
	*c.log = append(*c.log, "counts")
	return map[string]base.ContentCounts{ids[0]: c.c}
}

type env struct {
	uc   *Usecase
	repo *fakeRepo
	aud  *fakeAudit
	cnt  *fakeCounts
	log  *[]string
}

func newEnv(memberRole string) env {
	log := &[]string{}
	r := &fakeRepo{
		log: log, memberRole: memberRole, found: true, lockOK: true,
		project: base.Project{ID: "p1", Name: "Lama", Description: "desk", CreatedAt: t0, UpdatedAt: t0},
		members: []base.Member{{UserID: "u-1", Role: "owner"}, {UserID: "u-2", Role: "member"}},
	}
	a := &fakeAudit{log: log}
	c := &fakeCounts{log: log}
	return env{uc: NewUsecase(r, a, c, func() time.Time { return tNow }), repo: r, aud: a, cnt: c, log: log}
}

func ctxFor(role string) context.Context {
	return reqctx.WithIdentity(context.Background(), reqctx.Identity{UserID: "u-1", Email: "a@tim.id", Role: role})
}

func sp(s string) *string { return &s }

func in(p base.ProjectPatch) Input {
	return Input{ProjectID: "p1", Patch: p, Meta: base.RequestMeta{IP: "1.2.3.4", UserAgent: "ua"}}
}

func TestT03_AC2_MemberCanEditName(t *testing.T) {
	e := newEnv("member")
	out, err := e.uc.Execute(ctxFor("developer"), in(base.ProjectPatch{Name: sp("Baru")}))
	if err != nil {
		t.Fatal(err)
	}
	if len(e.repo.updated) != 1 || e.repo.updated[0].Name != "Baru" {
		t.Fatalf("updated: %+v", e.repo.updated)
	}
	if out.Name != "Baru" || out.AccessLevel != base.LevelMember || out.MyRole == nil || *out.MyRole != "member" {
		t.Fatalf("out: %+v", out)
	}
}

func TestUpdate_OwnerEdits(t *testing.T) {
	e := newEnv("owner")
	out, err := e.uc.Execute(ctxFor("developer"), in(base.ProjectPatch{Name: sp("Baru")}))
	if err != nil || out.AccessLevel != base.LevelOwner {
		t.Fatalf("%v %+v", err, out)
	}
}

func TestUpdate_SuperAdminNotMember(t *testing.T) {
	e := newEnv("")
	out, err := e.uc.Execute(ctxFor("super_admin"), in(base.ProjectPatch{Name: sp("Baru")}))
	if err != nil {
		t.Fatal(err)
	}
	if out.AccessLevel != base.LevelAdmin || out.MyRole != nil || out.IsMember {
		t.Fatalf("out: %+v", out)
	}
}

func TestUpdate_NonMemberGets404(t *testing.T) {
	e := newEnv("")
	_, err := e.uc.Execute(ctxFor("developer"), in(base.ProjectPatch{Name: sp("Baru")}))
	if !errors.Is(err, base.ErrProjectNotFound) {
		t.Fatalf("err: %v", err)
	}
	if e.repo.inTx != 0 || len(e.aud.events) != 0 {
		t.Fatal("tx/audit tidak boleh dipanggil")
	}
	for _, s := range *e.log {
		if s == "counts" {
			t.Fatal("counts dipanggil")
		}
	}
	// patch kosong oleh non-anggota tetap 404
	_, err = e.uc.Execute(ctxFor("developer"), in(base.ProjectPatch{}))
	if !errors.Is(err, base.ErrProjectNotFound) {
		t.Fatalf("empty patch non-member: %v", err)
	}
}

func TestUpdate_ProjectNotFound(t *testing.T) {
	e := newEnv("owner")
	e.repo.found = false
	_, err := e.uc.Execute(ctxFor("developer"), in(base.ProjectPatch{Name: sp("Baru")}))
	if !errors.Is(err, base.ErrProjectNotFound) || e.repo.inTx != 0 {
		t.Fatalf("err %v tx %d", err, e.repo.inTx)
	}
}

func TestUpdate_DeletedBetweenCheckAndLock(t *testing.T) {
	e := newEnv("owner")
	e.repo.lockOK = false
	_, err := e.uc.Execute(ctxFor("developer"), in(base.ProjectPatch{Name: sp("Baru")}))
	if !errors.Is(err, base.ErrProjectNotFound) || len(e.repo.updated) != 0 || len(e.aud.events) != 0 {
		t.Fatalf("err %v", err)
	}
}

func fieldsOf(t *testing.T, err error) map[string]string {
	t.Helper()
	var ee *errs.Error
	if !errors.As(err, &ee) || ee.Code != errs.CodeInvalid {
		t.Fatalf("bukan 400: %v", err)
	}
	m, _ := ee.Details.(map[string]any)
	fe, _ := m["fields"].(base.FieldErrors)
	return fe
}

func TestUpdate_EmptyPatch400(t *testing.T) {
	e := newEnv("owner")
	_, err := e.uc.Execute(ctxFor("developer"), in(base.ProjectPatch{}))
	fe := fieldsOf(t, err)
	if fe["body"] != "minimal satu field (name, description) wajib diisi" {
		t.Fatalf("fields: %v", fe)
	}
	if len(e.repo.updated) != 0 || len(e.aud.events) != 0 {
		t.Fatal("tidak boleh menulis")
	}
}

func TestUpdate_ValidationError(t *testing.T) {
	long := make([]rune, 1001)
	for i := range long {
		long[i] = 'x'
	}
	cases := []struct {
		name  string
		patch base.ProjectPatch
		keys  []string
	}{
		{"nama pendek", base.ProjectPatch{Name: sp("A")}, []string{"name"}},
		{"deskripsi panjang", base.ProjectPatch{Description: sp(string(long))}, []string{"description"}},
		{"keduanya", base.ProjectPatch{Name: sp("A"), Description: sp(string(long))}, []string{"name", "description"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newEnv("owner")
			_, err := e.uc.Execute(ctxFor("developer"), in(c.patch))
			fe := fieldsOf(t, err)
			for _, k := range c.keys {
				if fe[k] == "" {
					t.Fatalf("kunci %s hilang: %v", k, fe)
				}
			}
			if len(fe) != len(c.keys) || len(e.repo.updated) != 0 || len(e.aud.events) != 0 {
				t.Fatalf("fe %v", fe)
			}
		})
	}
}

func TestUpdate_NoopSkipsWriteAndAudit(t *testing.T) {
	for _, name := range []string{"Lama", "  Lama  "} {
		e := newEnv("owner")
		out, err := e.uc.Execute(ctxFor("developer"), in(base.ProjectPatch{Name: sp(name)}))
		if err != nil {
			t.Fatal(err)
		}
		if len(e.repo.updated) != 0 || len(e.aud.events) != 0 || !out.UpdatedAt.Equal(t0) {
			t.Fatalf("noop: upd %d aud %d at %v", len(e.repo.updated), len(e.aud.events), out.UpdatedAt)
		}
	}
}

func TestUpdate_AuditArgsNameChanged(t *testing.T) {
	e := newEnv("owner")
	_, err := e.uc.Execute(ctxFor("developer"), in(base.ProjectPatch{Name: sp("Baru")}))
	if err != nil || len(e.aud.events) != 1 {
		t.Fatalf("%v %d", err, len(e.aud.events))
	}
	ev := e.aud.events[0]
	if ev.Action != record.ActionProjectUpdated || ev.ProjectLabel != "Baru" || ev.TargetType != "project" ||
		ev.Target != "p1" || ev.ProjectID != "p1" || ev.Meta != (base.RequestMeta{IP: "1.2.3.4", UserAgent: "ua"}) {
		t.Fatalf("event: %+v", ev)
	}
	want := map[string]any{"changed_fields": []string{"name"}, "name_from": "Lama", "name_to": "Baru"}
	if !reflect.DeepEqual(ev.Args, want) {
		t.Fatalf("args: %v", ev.Args)
	}
}

func TestUpdate_AuditArgsDescriptionOnly(t *testing.T) {
	e := newEnv("owner")
	if _, err := e.uc.Execute(ctxFor("developer"), in(base.ProjectPatch{Description: sp("rahasia baru")})); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"changed_fields": []string{"description"}}
	if !reflect.DeepEqual(e.aud.events[0].Args, want) {
		t.Fatalf("args: %v", e.aud.events[0].Args)
	}
	if e.aud.events[0].ProjectLabel != "Lama" {
		t.Fatal("label harus nama tak berubah")
	}
}

func TestUpdate_AuditArgsBoth(t *testing.T) {
	e := newEnv("owner")
	if _, err := e.uc.Execute(ctxFor("developer"), in(base.ProjectPatch{Name: sp("Baru"), Description: sp("d2")})); err != nil {
		t.Fatal(err)
	}
	args := e.aud.events[0].Args
	if !reflect.DeepEqual(args["changed_fields"], []string{"name", "description"}) || args["name_from"] != "Lama" || args["name_to"] != "Baru" {
		t.Fatalf("args: %v", args)
	}
}

func TestUpdate_ClearDescription(t *testing.T) {
	e := newEnv("owner")
	out, err := e.uc.Execute(ctxFor("developer"), in(base.ProjectPatch{Description: sp("")}))
	if err != nil {
		t.Fatal(err)
	}
	if len(e.repo.updated) != 1 || e.repo.updated[0].Description != "" || out.Description != "" {
		t.Fatalf("updated %+v", e.repo.updated)
	}
	if !reflect.DeepEqual(e.aud.events[0].Args["changed_fields"], []string{"description"}) {
		t.Fatalf("args %v", e.aud.events[0].Args)
	}
}

func TestUpdate_UsesRepoUpdatedAt(t *testing.T) {
	e := newEnv("owner")
	at := tNow.Add(123 * time.Microsecond)
	e.repo.repoAt = &at
	out, err := e.uc.Execute(ctxFor("developer"), in(base.ProjectPatch{Name: sp("Baru")}))
	if err != nil || !out.UpdatedAt.Equal(at) {
		t.Fatalf("%v %v", err, out.UpdatedAt)
	}
}

func TestUpdate_AuditAfterTx(t *testing.T) {
	e := newEnv("owner")
	if _, err := e.uc.Execute(ctxFor("developer"), in(base.ProjectPatch{Name: sp("Baru")})); err != nil {
		t.Fatal(err)
	}
	want := []string{"FindAccess", "tx-begin", "LockProject", "UpdateProject", "tx-commit", "audit", "ListMembers", "counts"}
	if !reflect.DeepEqual(*e.log, want) {
		t.Fatalf("urutan: %v", *e.log)
	}
}

func TestUpdate_TxErrorNoAudit(t *testing.T) {
	e := newEnv("owner")
	boom := errors.New("boom")
	e.repo.failUpdate = boom
	_, err := e.uc.Execute(ctxFor("developer"), in(base.ProjectPatch{Name: sp("Baru")}))
	if !errors.Is(err, boom) || len(e.aud.events) != 0 {
		t.Fatalf("err %v", err)
	}
}

func TestUpdate_ResponseCounts(t *testing.T) {
	e := newEnv("owner")
	e.repo.members = []base.Member{{Role: "owner"}, {Role: "owner"}, {Role: "member"}}
	three := 3
	e.cnt.c = base.ContentCounts{Files: &three}
	out, err := e.uc.Execute(ctxFor("developer"), in(base.ProjectPatch{Name: sp("Baru")}))
	if err != nil {
		t.Fatal(err)
	}
	if out.MemberCount != 3 || out.OwnerCount != 2 || out.Counts.Files == nil || *out.Counts.Files != 3 ||
		out.Counts.Specs != nil || out.Counts.Keys != nil {
		t.Fatalf("out: %+v", out)
	}
	e.cnt.c = base.ContentCounts{}
	out, _ = e.uc.Execute(ctxFor("developer"), in(base.ProjectPatch{Name: sp("Baru2")}))
	if out.Counts.Files != nil || out.Counts.Specs != nil || out.Counts.Keys != nil {
		t.Fatalf("counts: %+v", out.Counts)
	}
}

// Perubahan sudah commit saat ListMembers gagal, jadi audit tetap terkirim sekali.
func TestUpdate_ListMembersErrorAfterCommit(t *testing.T) {
	e := newEnv("owner")
	boom := errors.New("boom")
	e.repo.failList = boom
	_, err := e.uc.Execute(ctxFor("developer"), in(base.ProjectPatch{Name: sp("Baru")}))
	if !errors.Is(err, boom) || len(e.aud.events) != 1 {
		t.Fatalf("err %v events %d", err, len(e.aud.events))
	}
}

func TestUpdate_NoIdentity(t *testing.T) {
	e := newEnv("owner")
	_, err := e.uc.Execute(context.Background(), in(base.ProjectPatch{Name: sp("Baru")}))
	var ee *errs.Error
	if !errors.As(err, &ee) || ee.Code != errs.CodeUnauthorized || len(*e.log) != 0 {
		t.Fatalf("err %v log %v", err, *e.log)
	}
}
