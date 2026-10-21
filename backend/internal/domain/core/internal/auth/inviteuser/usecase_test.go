package inviteuser

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/internal/domain/core/internal/auth/base/audit"
	"grimoire/lib/common/errs"
	"grimoire/lib/contracts/core/audit/record"
	"grimoire/lib/contracts/core/auth/invitation"
)

var t0 = time.Date(2026, 10, 21, 8, 0, 0, 0, time.UTC)

var testPolicy = base.InvitationPolicy{TTL: 168 * time.Hour, MaxPerInviterPerHour: 20, ResendMaxPerHour: 3, PurgeAfter: 720 * time.Hour}

type fakeHasher struct {
	inputs []string
	err    error
}

func (h *fakeHasher) Hash(pw string) (string, error) {
	h.inputs = append(h.inputs, pw)
	if h.err != nil {
		return "", h.err
	}
	return "HASH(" + fmt.Sprint(len(h.inputs)) + ")", nil
}
func (h *fakeHasher) Verify(string, string) (bool, error) { return false, nil }

type fakeAudit struct{ events []audit.Event }

func (a *fakeAudit) Record(_ context.Context, ev audit.Event) { a.events = append(a.events, ev) }

func (a *fakeAudit) actions() []string {
	var out []string
	for _, e := range a.events {
		out = append(out, e.Action)
	}
	return out
}

// fakeRepo adalah repo + tx in-memory.
type fakeRepo struct {
	user    *base.User
	inv     *base.Invitation
	count   int
	oldest  time.Time
	findN   int
	txN     int
	insertN int
	// simulasi
	insertErr     error // dikembalikan InsertUser
	afterTaken    *Target
	lockInv       *base.Invitation // override LockInvitation pada tx reissue
	updateErr     error
	updateCalls   int
	passwordCalls int
	passwordHash  string
	mustChange    bool
	sessionsCalls int
	findErr       error
	txCtxErrs     []error
}

func (r *fakeRepo) FindTarget(_ context.Context, _ string) (Target, error) {
	r.findN++
	if r.findErr != nil {
		return Target{}, r.findErr
	}
	if r.findN > 1 && r.afterTaken != nil {
		return *r.afterTaken, nil
	}
	return Target{User: r.user, Invitation: r.inv}, nil
}

func (r *fakeRepo) CountInvitedSince(context.Context, string, time.Time) (int, time.Time, error) {
	return r.count, r.oldest, nil
}

func (r *fakeRepo) InTx(ctx context.Context, fn func(Tx) error) error {
	r.txN++
	r.txCtxErrs = append(r.txCtxErrs, ctx.Err())
	return fn(r)
}

func (r *fakeRepo) InsertUser(_ context.Context, u base.User) (base.User, error) {
	r.insertN++
	if r.insertErr != nil {
		return base.User{}, r.insertErr
	}
	u.ID = "new-id"
	r.user = &u
	return u, nil
}

func (r *fakeRepo) InsertInvitation(_ context.Context, inv base.Invitation) error {
	r.inv = &inv
	return nil
}

func (r *fakeRepo) LockUser(context.Context, string) (base.User, bool, error) {
	if r.user == nil {
		return base.User{}, false, nil
	}
	return *r.user, true, nil
}

func (r *fakeRepo) LockInvitation(context.Context, string) (base.Invitation, bool, error) {
	if r.lockInv != nil {
		return *r.lockInv, true, nil
	}
	if r.inv == nil {
		return base.Invitation{}, false, nil
	}
	return *r.inv, true, nil
}

func (r *fakeRepo) UpdateInvitation(_ context.Context, inv base.Invitation) error {
	r.updateCalls++
	if r.updateErr != nil {
		return r.updateErr
	}
	r.inv = &inv
	return nil
}

func (r *fakeRepo) UpdatePassword(_ context.Context, _ string, hash string, mc bool, _ time.Time) error {
	r.passwordCalls++
	r.passwordHash, r.mustChange = hash, mc
	return nil
}

func (r *fakeRepo) DeleteSessions(context.Context, string) (int64, error) {
	r.sessionsCalls++
	return 2, nil
}

type fakeSend struct {
	status string
	calls  []sendCall
}
type sendCall struct {
	to      string
	c       base.EmailContent
	ctxErr  error
	ctxDone bool
}

func (s *fakeSend) fn(ctx context.Context, to string, c base.EmailContent) string {
	s.calls = append(s.calls, sendCall{to: to, c: c, ctxErr: ctx.Err()})
	if s.status == "" {
		return base.DeliverySent
	}
	return s.status
}

type zeroRand struct{}

func (zeroRand) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = byte(i * 7)
	}
	return len(p), nil
}

type errRand struct{}

func (errRand) Read([]byte) (int, error) { return 0, errors.New("rand rusak") }

type env struct {
	uc   *Usecase
	r    *fakeRepo
	h    *fakeHasher
	a    *fakeAudit
	s    *fakeSend
	logs *bytes.Buffer
}

func setup() env {
	e := env{r: &fakeRepo{}, h: &fakeHasher{}, a: &fakeAudit{}, s: &fakeSend{}, logs: &bytes.Buffer{}}
	e.uc = NewUsecase(e.r, e.h, e.a, func() time.Time { return t0 }, zeroRand{}, testPolicy, "https://g.id/login", e.s.fn).
		WithLogger(slog.New(slog.NewJSONHandler(e.logs, nil)))
	return e
}

func in(email string) Input {
	return Input{Email: email, InvitedBy: "inviter-1", InvitedByLabel: "Owner Satu", Deliver: true, Via: ViaUsers,
		Meta: base.RequestMeta{IP: "1.2.3.4", UserAgent: "ua"}}
}

func pendingTarget(e env) {
	u := base.User{ID: "u-1", Email: "baru@x.id", Name: "baru", Role: base.RoleDeveloper, IsActive: true, MustChangePassword: true, PasswordHash: "old"}
	inv := base.NewInvitation("u-1", "inviter-0", "Lama", t0.Add(-200*time.Hour), testPolicy) // sudah kedaluwarsa
	e.r.user, e.r.inv = &u, &inv
}

func errCode(t *testing.T, err error) (errs.Code, map[string]any) {
	t.Helper()
	var ee *errs.Error
	if !errors.As(err, &ee) {
		t.Fatalf("bukan *errs.Error: %v", err)
	}
	d, _ := ee.Details.(map[string]any)
	return ee.Code, d
}

func TestExecute_NewAccount_Deliver_Sent(t *testing.T) {
	e := setup()
	res, err := e.uc.Execute(context.Background(), in(" Baru@X.ID "))
	if err != nil {
		t.Fatal(err)
	}
	u := e.r.user
	if u == nil || u.Role != base.RoleDeveloper || !u.IsActive || !u.MustChangePassword || u.CreatedBy == nil || *u.CreatedBy != "inviter-1" {
		t.Fatalf("user: %+v", u)
	}
	if u.Email != "baru@x.id" {
		t.Fatalf("email: %s", u.Email)
	}
	if len(e.s.calls) != 1 || e.s.calls[0].to != "baru@x.id" {
		t.Fatalf("send: %+v", e.s.calls)
	}
	if !res.EmailSent || res.EmailStatus != "sent" || res.Credentials != nil || !res.Created {
		t.Fatalf("res: %+v", res)
	}
	if e.r.inv.DeliveryStatus != base.DeliverySent || e.r.inv.SentAt == nil || !e.r.inv.SentAt.Equal(t0) {
		t.Fatalf("inv: %+v", e.r.inv)
	}
	if !e.r.inv.ExpiresAt.Equal(t0.Add(testPolicy.TTL)) {
		t.Fatalf("expires: %v", e.r.inv.ExpiresAt)
	}
}

func TestExecute_Deliver_FallbackStatuses(t *testing.T) {
	for _, st := range []string{base.DeliveryNotConfigured, base.DeliveryFailed} {
		t.Run(st, func(t *testing.T) {
			e := setup()
			e.s.status = st
			res, err := e.uc.Execute(context.Background(), in("a@x.id"))
			if err != nil {
				t.Fatal(err)
			}
			c := res.Credentials
			if res.EmailSent || res.EmailStatus != st || c == nil || c.Email != "a@x.id" || len(c.TemporaryPassword) != 16 || c.LoginURL != "https://g.id/login" {
				t.Fatalf("res: %+v", res)
			}
			if e.r.inv == nil || e.r.inv.DeliveryStatus != st || e.r.inv.SentAt != nil || e.r.user == nil {
				t.Fatalf("akun/undangan harus tetap tersimpan: %+v", e.r.inv)
			}
			got := e.a.actions()
			if len(got) != 2 || got[0] != record.ActionUserInvited || got[1] != record.ActionUserInvitationEmailFailed {
				t.Fatalf("audit: %v", got)
			}
			ev := e.a.events[1]
			class := "send_failed"
			if st == base.DeliveryNotConfigured {
				class = "not_configured"
			}
			if ev.Status != record.StatusError || ev.Args["delivery_status"] != st || ev.Args["error_class"] != class || len(ev.Args) != 3 {
				t.Fatalf("event: %+v", ev)
			}
		})
	}
}

func TestExecute_Deliver_False_OnlyPrepares(t *testing.T) {
	e := setup()
	i := in("a@x.id")
	i.Deliver = false
	i.Via = ViaProject
	res, err := e.uc.Execute(context.Background(), i)
	if err != nil {
		t.Fatal(err)
	}
	if len(e.s.calls) != 0 || res.EmailStatus != "pending" || res.Credentials != nil || res.EmailSent {
		t.Fatalf("res: %+v calls=%d", res, len(e.s.calls))
	}
	if e.r.inv.DeliveryStatus != base.DeliveryPending || e.r.user.PasswordHash != "HASH(1)" {
		t.Fatalf("inv=%+v hash=%s", e.r.inv, e.r.user.PasswordHash)
	}
	b, _ := json.Marshal(res)
	if strings.Contains(string(b), e.h.inputs[0]) {
		t.Fatal("plaintext ada di Result")
	}
}

func TestExecute_PasswordMatchesCredentialsAndHash(t *testing.T) {
	e := setup()
	e.s.status = base.DeliveryFailed
	res, err := e.uc.Execute(context.Background(), in("a@x.id"))
	if err != nil {
		t.Fatal(err)
	}
	if len(e.h.inputs) != 1 || e.h.inputs[0] != res.Credentials.TemporaryPassword {
		t.Fatalf("hasher=%v cred=%v", e.h.inputs, res.Credentials)
	}
	if e.r.user.PasswordHash != "HASH(1)" || e.r.user.PasswordHash == res.Credentials.TemporaryPassword {
		t.Fatalf("hash: %s", e.r.user.PasswordHash)
	}
}

func TestExecute_Names(t *testing.T) {
	for name, tc := range map[string]struct{ email, name, want string }{
		"default":  {"budi.santoso@x.id", "", "budi santoso"},
		"spaces":   {"budi.santoso@x.id", "   ", "budi santoso"},
		"provided": {"a@x.id", "  Budi S ", "Budi S"},
	} {
		t.Run(name, func(t *testing.T) {
			e := setup()
			if _, err := e.uc.Execute(context.Background(), Input{Email: tc.email, Name: tc.name, InvitedBy: "i", Deliver: false, Via: ViaUsers}); err != nil {
				t.Fatal(err)
			}
			if e.r.user.Name != tc.want {
				t.Fatalf("name=%q", e.r.user.Name)
			}
		})
	}
}

func TestExecute_InvalidInput_NoSideEffects(t *testing.T) {
	long := strings.Repeat("a", 250) + "@x.id"
	cases := []struct {
		name, email, nm, field string
	}{
		{"empty", "", "", "email"}, {"no at", "a", "", "email"}, {"display", "Budi <b@x.id>", "", "email"},
		{"long", long, "", "email"},
		{"name long", "a@x.id", strings.Repeat("x", 101), "name"}, {"name ctl", "a@x.id", "a\x01b", "name"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := setup()
			i := in(c.email)
			i.Name = c.nm
			_, err := e.uc.Execute(context.Background(), i)
			code, d := errCode(t, err)
			fields, _ := d["fields"].(base.FieldErrors)
			if fields == nil {
				if m, ok := d["fields"].(map[string]string); ok {
					fields = m
				}
			}
			if code != errs.CodeInvalid || fields[c.field] == "" {
				t.Fatalf("code=%s d=%v", code, d)
			}
			if e.r.findN+e.r.txN != 0 || len(e.h.inputs) != 0 || len(e.s.calls) != 0 || len(e.a.events) != 0 {
				t.Fatal("ada efek samping")
			}
		})
	}
}

func TestExecute_AlreadyRegistered(t *testing.T) {
	now := t0
	cases := map[string]struct {
		user   base.User
		inv    *base.Invitation
		reason string
	}{
		"registered": {base.User{ID: "u", Role: base.RoleDeveloper, IsActive: true}, nil, "registered"},
		"accepted": {base.User{ID: "u", Role: base.RoleDeveloper, IsActive: true},
			&base.Invitation{UserID: "u", AcceptedAt: &now}, "registered"},
		"inactive":      {base.User{ID: "u", Role: base.RoleDeveloper, IsActive: false}, nil, "inactive"},
		"not developer": {base.User{ID: "u", Role: base.RoleSuperAdmin, IsActive: true}, nil, "not_developer"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			e := setup()
			u := c.user
			e.r.user, e.r.inv = &u, c.inv
			_, err := e.uc.Execute(context.Background(), in("a@x.id"))
			code, d := errCode(t, err)
			if code != errs.CodeAlreadyRegistered || d["reason"] != c.reason {
				t.Fatalf("code=%s d=%v", code, d)
			}
			if len(e.h.inputs) != 0 || e.r.txN != 0 || len(e.a.events) != 0 || len(e.s.calls) != 0 {
				t.Fatal("ada efek samping")
			}
		})
	}
}

func TestExecute_InviterQuota(t *testing.T) {
	e := setup()
	e.r.count, e.r.oldest = 20, t0.Add(-10*time.Minute)
	_, err := e.uc.Execute(context.Background(), in("a@x.id"))
	code, d := errCode(t, err)
	if code != errs.CodeRateLimited || d["scope"] != "invite" || d["retry_after_seconds"] != 3000 {
		t.Fatalf("code=%s d=%v", code, d)
	}
	if len(e.h.inputs) != 0 || e.r.insertN != 0 || len(e.a.events) != 0 {
		t.Fatal("ada efek samping")
	}
	e = setup()
	e.r.count = 19
	if _, err := e.uc.Execute(context.Background(), in("a@x.id")); err != nil {
		t.Fatalf("19 harus lolos: %v", err)
	}
}

func TestExecute_QuotaNotAppliedToReissue(t *testing.T) {
	e := setup()
	pendingTarget(e)
	e.r.count, e.r.oldest = 20, t0
	if _, err := e.uc.Execute(context.Background(), in("baru@x.id")); err != nil {
		t.Fatalf("kuota invite tidak berlaku: %v", err)
	}
}

func TestExecute_K6_DeliverFalse_NoIncrement(t *testing.T) {
	e := setup()
	pendingTarget(e)
	i := in("baru@x.id")
	i.Deliver, i.Via = false, ViaProject
	res, err := e.uc.Execute(context.Background(), i)
	if err != nil {
		t.Fatal(err)
	}
	if e.r.txN != 0 || e.r.updateCalls != 0 || e.r.passwordCalls != 0 || e.r.sessionsCalls != 0 || len(e.h.inputs) != 0 {
		t.Fatalf("tidak boleh mengubah apa pun: %+v", e.r)
	}
	if res.Created || res.EmailStatus != "pending" || res.Invitation.ResendWindowCount != 0 {
		t.Fatalf("res: %+v", res)
	}
}

func TestExecute_K6_ResendQuotaExceeded(t *testing.T) {
	for _, deliver := range []bool{false, true} {
		t.Run(fmt.Sprint("deliver=", deliver), func(t *testing.T) {
			e := setup()
			pendingTarget(e)
			start := t0.Add(-10 * time.Minute)
			e.r.inv.ResendWindowStartedAt, e.r.inv.ResendWindowCount = &start, 3
			i := in("baru@x.id")
			i.Deliver = deliver
			_, err := e.uc.Execute(context.Background(), i)
			code, d := errCode(t, err)
			if code != errs.CodeRateLimited || d["scope"] != "resend" {
				t.Fatalf("code=%s d=%v", code, d)
			}
			if e.r.passwordCalls != 0 || e.r.sessionsCalls != 0 || e.r.updateCalls != 0 || len(e.s.calls) != 0 || len(e.a.events) != 0 {
				t.Fatal("ada efek samping")
			}
		})
	}
}

func TestExecute_K6_DeliverTrue_Reissues(t *testing.T) {
	e := setup()
	pendingTarget(e)
	res, err := e.uc.Execute(context.Background(), in("baru@x.id"))
	if err != nil {
		t.Fatal(err)
	}
	if res.Created || !res.EmailSent {
		t.Fatalf("res: %+v", res)
	}
	if e.r.passwordCalls != 1 || e.r.passwordHash != "HASH(1)" || !e.r.mustChange || e.r.sessionsCalls != 1 {
		t.Fatalf("repo: %+v", e.r)
	}
	inv := e.r.inv
	if !inv.ExpiresAt.Equal(t0.Add(testPolicy.TTL)) || inv.ResendCount != 1 || inv.ResendWindowCount != 1 || inv.DeliveryStatus != base.DeliverySent {
		t.Fatalf("inv: %+v", inv)
	}
	if len(e.s.calls) != 1 {
		t.Fatalf("send=%d", len(e.s.calls))
	}
	got := e.a.actions()
	if len(got) != 2 || got[0] != record.ActionUserInvited || got[1] != record.ActionUserInvitationResent || e.a.events[0].Args["created"] != false {
		t.Fatalf("audit: %v %+v", got, e.a.events)
	}
	if e.a.events[1].Args["resend_count"] != 1 {
		t.Fatalf("args: %+v", e.a.events[1].Args)
	}
}

func TestExecute_K6_AcceptedDuringTx(t *testing.T) {
	e := setup()
	pendingTarget(e)
	acc := *e.r.inv
	acc.Accept(t0)
	e.r.lockInv = &acc
	_, err := e.uc.Execute(context.Background(), in("baru@x.id"))
	code, _ := errCode(t, err)
	if code != errs.CodeAlreadyRegistered {
		t.Fatalf("code=%s", code)
	}
	if e.r.passwordCalls != 0 || e.r.updateCalls != 0 || e.r.sessionsCalls != 0 || len(e.s.calls) != 0 {
		t.Fatal("ada efek samping")
	}
}

func TestExecute_RaceEmailTaken_BecomesReissue(t *testing.T) {
	e := setup()
	e.r.insertErr = base.ErrEmailTaken
	u := base.User{ID: "u-1", Email: "a@x.id", Role: base.RoleDeveloper, IsActive: true}
	inv := base.NewInvitation("u-1", "x", "", t0.Add(-time.Hour), testPolicy)
	e.r.afterTaken = &Target{User: &u, Invitation: &inv}
	e.r.user, e.r.inv = &u, &inv
	res, err := e.uc.Execute(context.Background(), in("a@x.id"))
	if err != nil {
		t.Fatal(err)
	}
	if res.Created || len(e.h.inputs) != 1 || e.r.passwordHash != "HASH(1)" {
		t.Fatalf("res=%+v hasher=%v", res, e.h.inputs)
	}
}

func TestExecute_RaceEmailTaken_BecomesAlreadyRegistered(t *testing.T) {
	e := setup()
	e.r.insertErr = base.ErrEmailTaken
	u := base.User{ID: "u-1", Email: "a@x.id", Role: base.RoleDeveloper, IsActive: true}
	e.r.afterTaken = &Target{User: &u}
	_, err := e.uc.Execute(context.Background(), in("a@x.id"))
	code, d := errCode(t, err)
	if code != errs.CodeAlreadyRegistered || d["reason"] != "registered" {
		t.Fatalf("code=%s d=%v", code, d)
	}
	if len(e.a.events) != 0 || len(e.s.calls) != 0 {
		t.Fatal("ada efek samping")
	}
}

func TestExecute_InsertError_Wrapped(t *testing.T) {
	e := setup()
	e.r.insertErr = errors.New("db mati")
	_, err := e.uc.Execute(context.Background(), in("a@x.id"))
	var ee *errs.Error
	if err == nil || errors.As(err, &ee) || !strings.HasPrefix(err.Error(), "inviteuser:") {
		t.Fatalf("err=%v", err)
	}
	if strings.Contains(err.Error(), e.h.inputs[0]) || len(e.s.calls) != 0 || len(e.a.events) != 0 {
		t.Fatal("bocor / efek samping")
	}
}

func TestExecute_HasherAndRandError(t *testing.T) {
	e := setup()
	e.h.err = errors.New("hasher rusak")
	_, err := e.uc.Execute(context.Background(), in("a@x.id"))
	if err == nil || !strings.HasPrefix(err.Error(), "inviteuser: hash") || e.r.insertN != 0 {
		t.Fatalf("err=%v", err)
	}
	e = setup()
	e.uc.rand = errRand{}
	_, err = e.uc.Execute(context.Background(), in("a@x.id"))
	if err == nil || !strings.HasPrefix(err.Error(), "inviteuser: generate password") || e.r.insertN != 0 || len(e.h.inputs) != 0 {
		t.Fatalf("err=%v", err)
	}
}

func TestExecute_FindError(t *testing.T) {
	e := setup()
	e.r.findErr = errors.New("boom")
	if _, err := e.uc.Execute(context.Background(), in("a@x.id")); err == nil || !strings.HasPrefix(err.Error(), "inviteuser: cari target") {
		t.Fatalf("err=%v", err)
	}
}

func TestExecute_DeliveryRecordFails_StillSuccess(t *testing.T) {
	e := setup()
	// InsertInvitation/insert lewat; hanya UpdateInvitation (catat status) gagal.
	e.r.updateErr = errors.New("update gagal")
	res, err := e.uc.Execute(context.Background(), in("a@x.id"))
	if err != nil || !res.EmailSent {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	logs := e.logs.String()
	if !strings.Contains(logs, "inviteuser: catat status kirim gagal") || strings.Contains(logs, e.h.inputs[0]) {
		t.Fatalf("log: %s", logs)
	}
}

func TestExecute_DeliveryRecordSkippedWhenReissuedConcurrently(t *testing.T) {
	e := setup()
	pendingTarget(e)
	calls := 0
	e.uc.send = func(ctx context.Context, to string, c base.EmailContent) string {
		calls++
		n := *e.r.inv
		n.IssuedAt = t0.Add(time.Minute) // request lain menerbitkan ulang saat kirim
		e.r.lockInv = &n
		return base.DeliverySent
	}
	if _, err := e.uc.Execute(context.Background(), in("baru@x.id")); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || e.r.inv.DeliveryStatus == base.DeliverySent {
		t.Fatalf("status tidak boleh ditimpa: %+v", e.r.inv)
	}
}

func TestExecute_SendUsesDetachedContext(t *testing.T) {
	e := setup()
	ctx, cancel := context.WithCancel(context.Background())
	e.uc.send = func(c context.Context, to string, _ base.EmailContent) string {
		cancel() // klien memutus sebelum/saat kirim
		if c.Err() != nil {
			t.Error("ctx kirim ikut dibatalkan")
		}
		return base.DeliverySent
	}
	cancel()
	if _, err := e.uc.Execute(ctx, in("a@x.id")); err != nil {
		t.Fatal(err)
	}
	for _, ce := range e.r.txCtxErrs[1:] {
		if ce != nil {
			t.Fatal("tx catat status memakai ctx yang dibatalkan")
		}
	}
}

func TestExecute_EmailContent(t *testing.T) {
	e := setup()
	i := in("a@x.id")
	i.ProjectName = "Alpha"
	res, err := e.uc.Execute(context.Background(), i)
	if err != nil || !res.EmailSent {
		t.Fatal(err)
	}
	c := e.s.calls[0].c
	if !strings.Contains(c.Text, "Owner Satu") || !strings.Contains(c.Text, "Alpha") || !strings.Contains(c.Text, "https://g.id/login") ||
		!strings.Contains(c.Text, e.h.inputs[0]) || strings.ContainsAny(c.Subject, "\r\n") {
		t.Fatalf("content: %+v", c)
	}
}

func TestExecute_AuditShapes(t *testing.T) {
	e := setup()
	if _, err := e.uc.Execute(context.Background(), in("a@x.id")); err != nil {
		t.Fatal(err)
	}
	if len(e.a.events) != 1 {
		t.Fatalf("events: %+v", e.a.events)
	}
	ev := e.a.events[0]
	if ev.Action != "user.invited" || ev.Target != "new-id" || ev.TargetType != "user" || ev.Status != record.StatusOK ||
		ev.Actor != nil || ev.Meta.IP != "1.2.3.4" || len(ev.Args) != 3 ||
		ev.Args["email"] != "a@x.id" || ev.Args["created"] != true || ev.Args["via"] != "users" {
		t.Fatalf("event: %+v", ev)
	}

	e = setup()
	i := in("a@x.id")
	i.Via, i.ProjectName = ViaProject, "Alpha"
	if _, err := e.uc.Execute(context.Background(), i); err != nil {
		t.Fatal(err)
	}
	if a := e.a.events[0].Args; a["via"] != "project" || a["project_name"] != "Alpha" {
		t.Fatalf("args: %+v", a)
	}
}

func TestExecute_NoSecretsInAuditOrLogs(t *testing.T) {
	for _, st := range []string{base.DeliverySent, base.DeliveryFailed, base.DeliveryNotConfigured} {
		for _, deliver := range []bool{true, false} {
			e := setup()
			e.s.status = st
			i := in("a@x.id")
			i.Deliver = deliver
			if _, err := e.uc.Execute(context.Background(), i); err != nil {
				t.Fatal(err)
			}
			b, _ := json.Marshal(e.a.events)
			blob := string(b) + e.logs.String()
			if strings.Contains(blob, e.h.inputs[0]) || strings.Contains(blob, "HASH(") || strings.Contains(strings.ToLower(blob), "temporary_password") {
				t.Fatalf("bocor (%s, deliver=%v): %s", st, deliver, blob)
			}
		}
	}
}

func TestExecute_NoAuditOnErrors(t *testing.T) {
	e := setup()
	e.r.count, e.r.oldest = 20, t0
	_, _ = e.uc.Execute(context.Background(), in("a@x.id"))
	_, _ = e.uc.Execute(context.Background(), in(""))
	if len(e.a.events) != 0 {
		t.Fatal("audit pada error")
	}
}

func TestExecute_EmptyInvitedBy(t *testing.T) {
	e := setup()
	i := in("a@x.id")
	i.InvitedBy = ""
	_, err := e.uc.Execute(context.Background(), i)
	var ee *errs.Error
	if err == nil || errors.As(err, &ee) || e.r.findN != 0 {
		t.Fatalf("err=%v", err)
	}
}

func TestExecute_RoleAlwaysDeveloper(t *testing.T) {
	e := setup()
	if _, err := e.uc.Execute(context.Background(), in("a@x.id")); err != nil {
		t.Fatal(err)
	}
	if e.r.user.Role != base.RoleDeveloper {
		t.Fatal(e.r.user.Role)
	}
}

var _ = invitation.EmailStatusSent
