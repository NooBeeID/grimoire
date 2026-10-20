package updateuser

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/internal/domain/core/internal/auth/base/audit"
	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
)

const (
	actorID  = "11111111-1111-4111-8111-111111111111"
	targetID = "22222222-2222-4222-8222-222222222222"
)

var fixedNow = time.Date(2026, 10, 21, 10, 0, 0, 0, time.UTC)

type fakeRepo struct {
	user      base.User
	found     bool
	ids       []string
	lockErr   error
	updateErr error
	calls     []string
	updates   []base.User
	txCalls   int
}

func (f *fakeRepo) InTx(ctx context.Context, fn func(context.Context, TxRepository) error) error {
	f.txCalls++
	return fn(ctx, f)
}
func (f *fakeRepo) LockActiveSuperAdminIDs(context.Context) ([]string, error) {
	f.calls = append(f.calls, "admins")
	return f.ids, f.lockErr
}
func (f *fakeRepo) LockUserByID(context.Context, string) (base.User, bool, error) {
	f.calls = append(f.calls, "user")
	return f.user, f.found, f.lockErr
}
func (f *fakeRepo) UpdateUserFields(_ context.Context, u base.User) error {
	f.updates = append(f.updates, u)
	return f.updateErr
}

type fakeRec struct{ evs []audit.Event }

func (f *fakeRec) Record(_ context.Context, ev audit.Event) { f.evs = append(f.evs, ev) }

type env struct {
	uc  *Usecase
	rp  *fakeRepo
	rec *fakeRec
	ctx context.Context
}

func setup() env {
	rp := &fakeRepo{
		user: base.User{ID: targetID, Email: "sari@tim.id", Name: "Sari", Role: base.RoleDeveloper, IsActive: true,
			UpdatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
		found: true,
		ids:   []string{actorID, "33333333-3333-4333-8333-333333333333"},
	}
	rec := &fakeRec{}
	ctx := reqctx.WithIdentity(context.Background(), reqctx.Identity{UserID: actorID, Role: base.RoleSuperAdmin, Email: "admin@tim.id"})
	return env{uc: NewUsecase(rp, rec, func() time.Time { return fixedNow }), rp: rp, rec: rec, ctx: ctx}
}

func sp(s string) *string { return &s }

func p(name, email, role *string) Params {
	return Params{ID: targetID, Name: name, Email: email, Role: role}
}

func wantErr(t *testing.T, err error, code errs.Code, reason string) {
	t.Helper()
	var ee *errs.Error
	if !errors.As(err, &ee) || ee.Code != code {
		t.Fatalf("want %s, got %v", code, err)
	}
	if reason != "" {
		if got, _ := detailsOf(ee)["reason"].(string); got != reason {
			t.Fatalf("reason=%q want %q", got, reason)
		}
	}
}

func TestUpdateUser_ChangesNameOnly(t *testing.T) {
	e := setup()
	res, err := e.uc.UpdateUser(e.ctx, p(sp("Sari W."), nil, nil))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(res.Changed, []string{"name"}) || res.User.Name != "Sari W." {
		t.Fatalf("%+v", res)
	}
	if len(e.rp.updates) != 1 || !e.rp.updates[0].UpdatedAt.Equal(fixedNow) {
		t.Fatalf("%+v", e.rp.updates)
	}
	if len(e.rec.evs) != 1 {
		t.Fatalf("audit=%d", len(e.rec.evs))
	}
	ev := e.rec.evs[0]
	if ev.Action != "user.updated" || ev.Status != "ok" || ev.Target != targetID || ev.TargetType != "user" ||
		!reflect.DeepEqual(ev.Args["changed_fields"], []string{"name"}) {
		t.Fatalf("%+v", ev)
	}
	if _, ok := ev.Args["role_from"]; ok {
		t.Fatal("role_from tidak boleh ada")
	}
}

func TestUpdateUser_ChangesAllFields(t *testing.T) {
	e := setup()
	res, err := e.uc.UpdateUser(e.ctx, p(sp("Sari W."), sp("Sari.W@Tim.ID"), sp(base.RoleSuperAdmin)))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(res.Changed, []string{"name", "email", "role"}) || res.User.Email != "sari.w@tim.id" {
		t.Fatalf("%+v", res)
	}
	a := e.rec.evs[0].Args
	if a["role_from"] != base.RoleDeveloper || a["role_to"] != base.RoleSuperAdmin {
		t.Fatalf("%v", a)
	}
}

func TestUpdateUser_NoChange_NoWriteNoAudit(t *testing.T) {
	e := setup()
	res, err := e.uc.UpdateUser(e.ctx, p(sp("  Sari "), sp("SARI@tim.id "), sp(base.RoleDeveloper)))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Changed) != 0 || len(e.rp.updates) != 0 || len(e.rec.evs) != 0 {
		t.Fatalf("%+v updates=%d audit=%d", res, len(e.rp.updates), len(e.rec.evs))
	}
	if !res.User.UpdatedAt.Equal(e.rp.user.UpdatedAt) {
		t.Fatal("updated_at berubah")
	}
}

func TestUpdateUser_SameRoleSelf_IsNoOp(t *testing.T) {
	e := setup()
	e.rp.user.ID, e.rp.user.Role = actorID, base.RoleSuperAdmin
	_, err := e.uc.UpdateUser(e.ctx, Params{ID: actorID, Role: sp(base.RoleSuperAdmin)})
	if err != nil || len(e.rec.evs) != 0 || len(e.rp.updates) != 0 {
		t.Fatalf("err=%v", err)
	}
}

func TestUpdateUser_InvalidID_NotFoundWithoutQuery(t *testing.T) {
	e := setup()
	_, err := e.uc.UpdateUser(e.ctx, Params{ID: "bukan-uuid", Name: sp("x")})
	if !errors.Is(err, base.ErrUserNotFound) || e.rp.txCalls != 0 {
		t.Fatalf("err=%v tx=%d", err, e.rp.txCalls)
	}
}

func TestUpdateUser_NoIdentity_Unauthorized(t *testing.T) {
	e := setup()
	_, err := e.uc.UpdateUser(context.Background(), p(sp("x"), nil, nil))
	wantErr(t, err, errs.CodeUnauthorized, "")
}

func TestUpdateUser_TargetMissing_NotFound(t *testing.T) {
	e := setup()
	e.rp.found = false
	_, err := e.uc.UpdateUser(e.ctx, p(sp("x"), nil, nil))
	if !errors.Is(err, base.ErrUserNotFound) || len(e.rec.evs) != 0 {
		t.Fatalf("%v", err)
	}
}

func TestUpdateUser_ValidationErrorsAggregated(t *testing.T) {
	e := setup()
	_, err := e.uc.UpdateUser(e.ctx, p(sp(" "), sp("a@"), sp("owner")))
	wantErr(t, err, errs.CodeInvalid, "")
	var ee *errs.Error
	_ = errors.As(err, &ee)
	fe, _ := detailsOf(ee)["fields"].(base.FieldErrors)
	for _, k := range []string{"name", "email", "role"} {
		if fe[k] == "" {
			t.Fatalf("field %s hilang: %v", k, fe)
		}
	}
	if len(e.rp.updates) != 0 || len(e.rec.evs) != 0 {
		t.Fatal("tidak boleh update/audit")
	}
}

func TestUpdateUser_SelfDemote_Forbidden(t *testing.T) {
	e := setup()
	e.rp.user.ID, e.rp.user.Role = actorID, base.RoleSuperAdmin
	_, err := e.uc.UpdateUser(e.ctx, Params{ID: actorID, Role: sp(base.RoleDeveloper)})
	wantErr(t, err, errs.CodeForbidden, base.ReasonSelfAction)
	if len(e.rp.updates) != 0 || len(e.rec.evs) != 0 {
		t.Fatal("tidak boleh update/audit")
	}
}

func TestUpdateUser_DemoteLastSuperAdmin_Conflict(t *testing.T) {
	e := setup()
	e.rp.user.Role = base.RoleSuperAdmin
	e.rp.ids = []string{targetID}
	_, err := e.uc.UpdateUser(e.ctx, p(nil, nil, sp(base.RoleDeveloper)))
	wantErr(t, err, errs.CodeConflict, base.ReasonLastSuperAdmin)
	if len(e.rec.evs) != 0 {
		t.Fatal("audit")
	}
}

func TestUpdateUser_DemoteOtherAdminWhenTwo_OK(t *testing.T) {
	e := setup()
	e.rp.user.Role = base.RoleSuperAdmin
	e.rp.ids = []string{actorID, targetID}
	if _, err := e.uc.UpdateUser(e.ctx, p(nil, nil, sp(base.RoleDeveloper))); err != nil {
		t.Fatal(err)
	}
}

func TestUpdateUser_DemoteInactiveSuperAdmin_OK(t *testing.T) {
	e := setup()
	e.rp.user.Role, e.rp.user.IsActive = base.RoleSuperAdmin, false
	e.rp.ids = []string{actorID}
	if _, err := e.uc.UpdateUser(e.ctx, p(nil, nil, sp(base.RoleDeveloper))); err != nil {
		t.Fatal(err)
	}
}

func TestUpdateUser_PromoteDeveloper_OK(t *testing.T) {
	e := setup()
	e.rp.ids = []string{actorID}
	res, err := e.uc.UpdateUser(e.ctx, p(nil, nil, sp(base.RoleSuperAdmin)))
	if err != nil || res.User.Role != base.RoleSuperAdmin {
		t.Fatalf("%v", err)
	}
}

func TestUpdateUser_EmailConflict(t *testing.T) {
	e := setup()
	e.rp.updateErr = base.ErrEmailTaken
	_, err := e.uc.UpdateUser(e.ctx, p(nil, sp("dup@tim.id"), nil))
	wantErr(t, err, errs.CodeConflict, "")
	if len(e.rec.evs) != 0 {
		t.Fatal("audit")
	}
}

func TestUpdateUser_LocksSuperAdminSetOnlyWhenRolePresent(t *testing.T) {
	e := setup()
	_, _ = e.uc.UpdateUser(e.ctx, p(sp("Baru"), nil, nil))
	if !reflect.DeepEqual(e.rp.calls, []string{"user"}) {
		t.Fatalf("%v", e.rp.calls)
	}
	e = setup()
	_, _ = e.uc.UpdateUser(e.ctx, p(nil, nil, sp(base.RoleSuperAdmin)))
	if !reflect.DeepEqual(e.rp.calls, []string{"admins", "user"}) {
		t.Fatalf("%v", e.rp.calls)
	}
}

func TestUpdateUser_AuditArgsHaveNoEmailOrName(t *testing.T) {
	e := setup()
	_, err := e.uc.UpdateUser(e.ctx, p(sp("Nama Rahasia"), sp("rahasia@tim.id"), nil))
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range e.rec.evs[0].Args {
		if s, ok := v.(string); ok && (s == "Nama Rahasia" || s == "rahasia@tim.id") {
			t.Fatalf("%s memuat PII", k)
		}
	}
	if len(e.rec.evs[0].Args) != 1 {
		t.Fatalf("%v", e.rec.evs[0].Args)
	}
}

func TestUpdateUser_InfraErrorWrapped(t *testing.T) {
	e := setup()
	e.rp.lockErr = errors.New("db mati")
	_, err := e.uc.UpdateUser(e.ctx, p(sp("x"), nil, nil))
	var ee *errs.Error
	if err == nil || errors.As(err, &ee) || len(e.rec.evs) != 0 {
		t.Fatalf("%v", err)
	}
}

func detailsOf(ee *errs.Error) map[string]any {
	m, _ := ee.Details.(map[string]any)
	return m
}
