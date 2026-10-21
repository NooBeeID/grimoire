package createproject

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"grimoire/internal/domain/project/internal/management/base"
	"grimoire/internal/domain/project/internal/management/base/audit"
	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
)

var fixedNow = time.Date(2026, 10, 22, 9, 0, 0, 0, time.UTC)

type fakeRepo struct {
	log      *[]string
	projects []base.Project
	members  []base.Member
	inTx     int
	failProj error
	failMem  error
}

func (f *fakeRepo) InTx(ctx context.Context, fn func(Tx) error) error {
	f.inTx++
	*f.log = append(*f.log, "tx-begin")
	err := fn(f)
	*f.log = append(*f.log, "tx-end")
	return err
}

func (f *fakeRepo) InsertProject(_ context.Context, p base.Project) (base.Project, error) {
	*f.log = append(*f.log, "insert-project")
	if f.failProj != nil {
		return base.Project{}, f.failProj
	}
	p.ID = "pid-1"
	f.projects = append(f.projects, p)
	return p, nil
}

func (f *fakeRepo) InsertMember(_ context.Context, m base.Member) error {
	*f.log = append(*f.log, "insert-member")
	if f.failMem != nil {
		return f.failMem
	}
	f.members = append(f.members, m)
	return nil
}

type fakeAudit struct {
	log    *[]string
	events []audit.Event
}

func (a *fakeAudit) Record(_ context.Context, ev audit.Event) {
	*a.log = append(*a.log, "audit")
	a.events = append(a.events, ev)
}

type env struct {
	uc   *Usecase
	repo *fakeRepo
	aud  *fakeAudit
	log  *[]string
}

func newEnv() env {
	log := &[]string{}
	r := &fakeRepo{log: log}
	a := &fakeAudit{log: log}
	return env{uc: NewUsecase(r, a, func() time.Time { return fixedNow }), repo: r, aud: a, log: log}
}

func ctxFor(role string) context.Context {
	return reqctx.WithIdentity(context.Background(), reqctx.Identity{UserID: "u-1", Email: "a@tim.id", Role: role})
}

func TestExecute_DeveloperBecomesOwner(t *testing.T) {
	e := newEnv()
	meta := base.RequestMeta{IP: "10.0.0.5", UserAgent: "ua"}
	s, err := e.uc.Execute(ctxFor("developer"), CreateRequest{Name: "  Payments  ", Description: "d"}, meta)
	if err != nil {
		t.Fatal(err)
	}
	if e.repo.inTx != 1 || len(e.repo.projects) != 1 || len(e.repo.members) != 1 {
		t.Fatalf("calls: %v", *e.log)
	}
	p, m := e.repo.projects[0], e.repo.members[0]
	if p.Name != "Payments" || *p.CreatedBy != "u-1" || *p.CreatedByLabel != "a@tim.id" {
		t.Fatalf("project: %+v", p)
	}
	if m.Role != base.RoleOwner || m.UserID != "u-1" || *m.AddedBy != "u-1" || *m.AddedByLabel != "a@tim.id" || m.ProjectID != "pid-1" {
		t.Fatalf("member: %+v", m)
	}
	if s.MyRole == nil || *s.MyRole != "owner" || s.AccessLevel != base.LevelOwner || !s.IsMember || s.MemberCount != 1 {
		t.Fatalf("summary: %+v", s)
	}
	if s.Counts.Files == nil || s.Counts.Specs == nil || s.Counts.Keys == nil ||
		*s.Counts.Files != 0 || *s.Counts.Specs != 0 || *s.Counts.Keys != 0 {
		t.Fatalf("counts: %+v", s.Counts)
	}
	if want := []string{"tx-begin", "insert-project", "insert-member", "tx-end", "audit"}; !reflect.DeepEqual(*e.log, want) {
		t.Fatalf("order: %v", *e.log)
	}
}

func TestExecute_SuperAdminNotMember(t *testing.T) {
	e := newEnv()
	s, err := e.uc.Execute(ctxFor("super_admin"), CreateRequest{Name: "Payments"}, base.RequestMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if len(e.repo.members) != 0 {
		t.Fatal("member tidak boleh dibuat")
	}
	p := e.repo.projects[0]
	if *p.CreatedBy != "u-1" || *p.CreatedByLabel != "a@tim.id" {
		t.Fatalf("project: %+v", p)
	}
	if s.MyRole != nil || s.AccessLevel != base.LevelAdmin || s.IsMember || s.MemberCount != 0 {
		t.Fatalf("summary: %+v", s)
	}
}

func TestExecute_InvalidInput(t *testing.T) {
	long := strings.Repeat("x", 101)
	for name, n := range map[string]string{"satu": "A", "spasi": "   ", "kontrol": "a\nb", "101": long} {
		t.Run(name, func(t *testing.T) {
			e := newEnv()
			_, err := e.uc.Execute(ctxFor("developer"), CreateRequest{Name: n}, base.RequestMeta{})
			var ee *errs.Error
			if !errors.As(err, &ee) || ee.Code != errs.CodeInvalid {
				t.Fatalf("err: %v", err)
			}
			fe := ee.Details.(map[string]any)["fields"].(base.FieldErrors)
			if _, ok := fe["name"]; !ok {
				t.Fatalf("fields: %v", fe)
			}
			if e.repo.inTx != 0 || len(e.aud.events) != 0 {
				t.Fatal("repo/audit tidak boleh dipanggil")
			}
		})
	}
}

func TestExecute_BothFieldErrors(t *testing.T) {
	e := newEnv()
	_, err := e.uc.Execute(ctxFor("developer"), CreateRequest{Name: "A", Description: strings.Repeat("d", 1001)}, base.RequestMeta{})
	var ee *errs.Error
	if !errors.As(err, &ee) {
		t.Fatal(err)
	}
	fe := ee.Details.(map[string]any)["fields"].(base.FieldErrors)
	if _, ok := fe["name"]; !ok {
		t.Fatal("name")
	}
	if _, ok := fe["description"]; !ok {
		t.Fatal("description")
	}
}

func TestExecute_ValidBoundaries(t *testing.T) {
	e := newEnv()
	name := strings.Repeat("é", 100)
	s, err := e.uc.Execute(ctxFor("developer"), CreateRequest{Name: name}, base.RequestMeta{})
	if err != nil || s.Name != name || s.Description != "" {
		t.Fatalf("%v %+v", err, s)
	}
}

func TestExecute_AuthErrors(t *testing.T) {
	e := newEnv()
	_, err := e.uc.Execute(context.Background(), CreateRequest{Name: "Payments"}, base.RequestMeta{})
	var ee *errs.Error
	if !errors.As(err, &ee) || ee.Code != errs.CodeUnauthorized {
		t.Fatalf("err: %v", err)
	}
	_, err = e.uc.Execute(ctxFor("agent"), CreateRequest{Name: "Payments"}, base.RequestMeta{})
	if !errors.Is(err, base.ErrForbidden) {
		t.Fatalf("err: %v", err)
	}
	if e.repo.inTx != 0 || len(e.aud.events) != 0 {
		t.Fatal("repo/audit tidak boleh dipanggil")
	}
}

func TestExecute_InfraErrors(t *testing.T) {
	for name, set := range map[string]func(*fakeRepo, error){
		"project": func(r *fakeRepo, err error) { r.failProj = err },
		"member":  func(r *fakeRepo, err error) { r.failMem = err },
	} {
		t.Run(name, func(t *testing.T) {
			e := newEnv()
			boom := errors.New("boom")
			set(e.repo, boom)
			_, err := e.uc.Execute(ctxFor("developer"), CreateRequest{Name: "Payments"}, base.RequestMeta{})
			var ee *errs.Error
			if err == nil || errors.As(err, &ee) || !errors.Is(err, boom) || !strings.HasPrefix(err.Error(), "createproject: ") {
				t.Fatalf("err: %v", err)
			}
			if len(e.aud.events) != 0 {
				t.Fatal("audit harus 0")
			}
		})
	}
}

func TestExecute_AuditEvent(t *testing.T) {
	e := newEnv()
	meta := base.RequestMeta{IP: "10.0.0.5", UserAgent: "ua"}
	if _, err := e.uc.Execute(ctxFor("developer"), CreateRequest{Name: "Payments", Description: "rahasia"}, meta); err != nil {
		t.Fatal(err)
	}
	if len(e.aud.events) != 1 {
		t.Fatalf("events: %d", len(e.aud.events))
	}
	ev := e.aud.events[0]
	want := audit.Event{
		Action: "project.created", ProjectID: "pid-1", ProjectLabel: "Payments",
		TargetType: "project", Target: "pid-1", Args: map[string]any{"name": "Payments"}, Meta: meta,
	}
	if !reflect.DeepEqual(ev, want) {
		t.Fatalf("event: %+v", ev)
	}
}
