package resendinvitation

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/internal/domain/core/internal/auth/base/audit"
	"grimoire/internal/infra/mailer/mailertest"
	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
	"grimoire/lib/contracts/core/audit/record"
)

const uid = "7c1f3c2e-5b7a-4d7e-9a51-1f0b8a3e6d21"

var now = time.Date(2026, 10, 21, 10, 0, 0, 0, time.UTC)

var policy = base.InvitationPolicy{TTL: 72 * time.Hour, ResendMaxPerHour: 3, MaxPerInviterPerHour: 20}

type fakeRepo struct {
	user         base.User
	userExists   bool
	inv          *base.Invitation
	hash         string
	mustChange   bool
	revokeCalls  int
	issueErr     error
	markErr      error
	marks        []string
	marked       *base.Invitation
	issueCalls   int
	lastIssueNow time.Time
}

func (f *fakeRepo) Issue(_ context.Context, in IssueInput) (Issued, error) {
	f.issueCalls++
	if f.issueErr != nil {
		return Issued{}, f.issueErr
	}
	if !f.userExists {
		return Issued{}, base.ErrUserNotFound
	}
	var cp *base.Invitation
	if f.inv != nil {
		c := *f.inv
		cp = &c
	}
	if err := in.Apply(f.user, cp); err != nil {
		return Issued{}, err
	}
	f.inv = cp // commit
	f.hash, f.mustChange = in.Hash, true
	f.revokeCalls++
	f.lastIssueNow = in.Now
	u := f.user
	u.PasswordHash, u.MustChangePassword = in.Hash, true
	return Issued{User: u, Invitation: *cp, Revoked: 2}, nil
}

func (f *fakeRepo) MarkDelivery(_ context.Context, _ string, status string, n time.Time) error {
	f.marks = append(f.marks, status)
	if f.markErr != nil {
		return f.markErr
	}
	f.inv.MarkDelivery(status, n)
	return nil
}

type fakeRec struct{ evs []audit.Event }

func (f *fakeRec) Record(_ context.Context, ev audit.Event) { f.evs = append(f.evs, ev) }

func (f *fakeRec) actions() []string {
	var a []string
	for _, e := range f.evs {
		a = append(a, e.Action)
	}
	return a
}

type env struct {
	uc   *Usecase
	rp   *fakeRepo
	rec  *fakeRec
	mail *mailertest.Fake
	log  *bytes.Buffer
	ctx  context.Context
	clk  *time.Time
	h    base.Hasher
}

func setup() *env {
	t0 := now
	rp := &fakeRepo{
		user:       base.User{ID: uid, Email: "baru@contoh.id", Name: "Baru", Role: base.RoleDeveloper, IsActive: true, PasswordHash: "old"},
		userExists: true,
		inv: &base.Invitation{UserID: uid, InvitedBy: "inviter-1", IssuedAt: now.Add(-time.Hour), ExpiresAt: now.Add(71 * time.Hour),
			DeliveryStatus: base.DeliveryFailed},
	}
	rec := &fakeRec{}
	ml := &mailertest.Fake{}
	buf := &bytes.Buffer{}
	h := base.Argon2Hasher{Params: base.Argon2Params{MemoryKiB: 64, Iterations: 1, Parallelism: 1}}
	e := &env{rp: rp, rec: rec, mail: ml, log: buf, clk: &t0, h: h}
	e.uc = NewUsecase(rp, h, ml, rec, func() time.Time { return *e.clk },
		bytes.NewReader(bytes.Repeat([]byte{1, 2, 3, 4, 5, 6, 7, 8, 9}, 128)), policy,
		"https://grimoire.contoh.id/login", time.Second, slog.New(slog.NewTextHandler(buf, nil)))
	e.ctx = reqctx.WithIdentity(context.Background(), reqctx.Identity{UserID: "admin-1", Email: "admin@contoh.id", Name: "Admin Satu", Role: base.RoleSuperAdmin})
	return e
}

func in() Input { return Input{UserID: uid, Via: ViaUsers} }

func errCode(t *testing.T, err error) *errs.Error {
	t.Helper()
	var ee *errs.Error
	if !errors.As(err, &ee) {
		t.Fatalf("bukan *errs.Error: %v", err)
	}
	return ee
}

func TestU1_ResendSuccess(t *testing.T) {
	e := setup()
	res, err := e.uc.Execute(e.ctx, in())
	if err != nil {
		t.Fatal(err)
	}
	if !res.EmailSent || res.EmailStatus != "sent" || res.Credentials != nil {
		t.Fatalf("%+v", res)
	}
	if e.rp.inv.ResendCount != 1 || !res.ExpiresAt.Equal(now.Add(policy.TTL)) {
		t.Fatalf("%+v", e.rp.inv)
	}
	m, ok := e.mail.Last()
	if !ok || !strings.Contains(m.To, "baru@contoh.id") {
		t.Fatal("email tidak terkirim")
	}
	// password di email cocok dengan hash yang tersimpan (U11)
	var pw string
	for _, line := range strings.Split(m.Text, "\n") {
		for _, f := range strings.Fields(line) {
			if len(f) == base.GeneratedPasswordLen {
				if ok, _ := e.h.Verify(e.rp.hash, f); ok {
					pw = f
				}
			}
		}
	}
	if pw == "" {
		t.Fatalf("password di email tidak cocok dengan hash: %s", m.Text)
	}
	if ok, _ := e.h.Verify(e.rp.hash, "old"); ok || !e.rp.mustChange {
		t.Fatal("hash lama harus tak berlaku dan must_change=true")
	}
	if got := e.rec.actions(); len(got) != 1 || got[0] != record.ActionUserInvitationResent {
		t.Fatalf("audit %v", got)
	}
	if e.rec.evs[0].Args["via"] != "users" || e.rec.evs[0].Args["resend_count"] != 1 {
		t.Fatalf("%v", e.rec.evs[0].Args)
	}
	if e.rec.evs[0].Actor != nil {
		t.Fatal("aktor dari ctx")
	}
	if e.rp.revokeCalls != 1 {
		t.Fatal("sesi harus dicabut")
	}
	// U17
	if len(e.rp.marks) != 1 || e.rp.marks[0] != base.DeliverySent || e.rp.inv.SentAt == nil {
		t.Fatalf("%v %+v", e.rp.marks, e.rp.inv)
	}
}

func TestU2_InitialSkipsQuotaAndAudit(t *testing.T) {
	e := setup()
	i := in()
	i.Initial = true
	res, err := e.uc.Execute(e.ctx, i)
	if err != nil || !res.EmailSent {
		t.Fatalf("%v %+v", err, res)
	}
	if e.rp.inv.ResendCount != 0 || e.rp.inv.ResendWindowStartedAt != nil {
		t.Fatalf("%+v", e.rp.inv)
	}
	if len(e.rec.evs) != 0 {
		t.Fatalf("audit %v", e.rec.actions())
	}
	if !res.ExpiresAt.Equal(now.Add(policy.TTL)) {
		t.Fatal("expires")
	}
}

func TestU3_U4_Quota(t *testing.T) {
	e := setup()
	for n := 1; n <= 3; n++ {
		if _, err := e.uc.Execute(e.ctx, in()); err != nil {
			t.Fatalf("resend %d: %v", n, err)
		}
	}
	hash, mails, evs := e.rp.hash, len(e.mail.Sent), len(e.rec.evs)
	_, err := e.uc.Execute(e.ctx, in())
	ee := errCode(t, err)
	if ee.Code != errs.CodeRateLimited {
		t.Fatalf("%+v", ee)
	}
	d, _ := ee.Details.(map[string]any)
	if d["scope"] != "resend" || d["retry_after_seconds"] == nil {
		t.Fatalf("details %v", ee.Details)
	}
	if e.rp.hash != hash || len(e.mail.Sent) != mails || len(e.rec.evs) != evs {
		t.Fatal("tidak boleh ada efek samping saat 429")
	}
	// U4: window bergeser
	*e.clk = now.Add(time.Hour)
	if _, err := e.uc.Execute(e.ctx, in()); err != nil {
		t.Fatalf("window baru: %v", err)
	}
	if e.rp.inv.ResendWindowCount != 1 {
		t.Fatalf("%+v", e.rp.inv)
	}
}

func TestU5_InitialNeverRateLimited(t *testing.T) {
	e := setup()
	i := in()
	i.Initial = true
	for n := 0; n < 6; n++ {
		if _, err := e.uc.Execute(e.ctx, i); err != nil {
			t.Fatalf("%d: %v", n, err)
		}
	}
}

func TestU6_UserNotFound(t *testing.T) {
	e := setup()
	for _, id := range []string{"", "bukan-uuid", "7c1f3c2e-5b7a-4d7e-9a51-1f0b8a3e6dzz"} {
		i := in()
		i.UserID = id
		if _, err := e.uc.Execute(e.ctx, i); errCode(t, err).Code != errs.CodeNotFound {
			t.Fatalf("%q: %v", id, err)
		}
	}
	e.rp.userExists = false
	if _, err := e.uc.Execute(e.ctx, in()); errCode(t, err).Code != errs.CodeNotFound {
		t.Fatal(err)
	}
	if len(e.mail.Sent) != 0 {
		t.Fatal("tanpa email")
	}
}

func reason(ee *errs.Error) any {
	d, _ := ee.Details.(map[string]any)
	return d["reason"]
}

func TestU7_NoInvitationRow(t *testing.T) {
	e := setup()
	e.rp.inv = nil
	_, err := e.uc.Execute(e.ctx, in())
	ee := errCode(t, err)
	if ee.Code != errs.CodeConflict || reason(ee) != "INVITATION_NOT_PENDING" {
		t.Fatalf("%+v", ee)
	}
}

func TestU8_AlreadyAccepted(t *testing.T) {
	e := setup()
	at := now.Add(-time.Minute)
	e.rp.inv.AcceptedAt = &at
	_, err := e.uc.Execute(e.ctx, in())
	if ee := errCode(t, err); reason(ee) != "INVITATION_NOT_PENDING" {
		t.Fatalf("%+v", ee)
	}
}

func TestU9_InactiveUser(t *testing.T) {
	e := setup()
	e.rp.user.IsActive = false
	_, err := e.uc.Execute(e.ctx, in())
	ee := errCode(t, err)
	if reason(ee) != base.AlreadyRegisteredInactive || ee.Code != base.ErrAlreadyRegistered(base.AlreadyRegisteredInactive).Code {
		t.Fatalf("%+v", ee)
	}
	if len(e.mail.Sent) != 0 || len(e.rec.evs) != 0 {
		t.Fatal("tanpa email/audit")
	}
	// urutan cek: nonaktif tanpa undangan -> INVITATION_NOT_PENDING
	e.rp.inv = nil
	_, err = e.uc.Execute(e.ctx, in())
	if reason(errCode(t, err)) != "INVITATION_NOT_PENDING" {
		t.Fatal(err)
	}
}

func TestCheckResendTarget(t *testing.T) {
	active := base.User{IsActive: true}
	pending := &base.Invitation{ExpiresAt: now.Add(-time.Hour)} // kedaluwarsa tetap pending
	if err := checkResendTarget(active, pending); err != nil {
		t.Fatal(err)
	}
}

func TestU10_ExpiredRecovers(t *testing.T) {
	e := setup()
	e.rp.inv.ExpiresAt = now.Add(-24 * time.Hour)
	res, err := e.uc.Execute(e.ctx, in())
	if err != nil || !res.ExpiresAt.After(now) {
		t.Fatalf("%v %+v", err, res)
	}
}

func TestU13_NotConfigured(t *testing.T) {
	e := setup()
	e.mail.NotConfigured = true
	res, err := e.uc.Execute(e.ctx, in())
	if err != nil {
		t.Fatal(err)
	}
	if res.EmailSent || res.EmailStatus != "not_configured" || res.Credentials == nil {
		t.Fatalf("%+v", res)
	}
	c := res.Credentials
	if c.Email != "baru@contoh.id" || len(c.TemporaryPassword) != base.GeneratedPasswordLen || c.LoginURL != "https://grimoire.contoh.id/login" {
		t.Fatalf("%+v", c)
	}
	if ok, _ := e.h.Verify(e.rp.hash, c.TemporaryPassword); !ok {
		t.Fatal("kredensial harus cocok hash")
	}
	var failed *audit.Event
	for i := range e.rec.evs {
		if e.rec.evs[i].Action == record.ActionUserInvitationEmailFailed {
			failed = &e.rec.evs[i]
		}
	}
	if failed == nil || failed.Status != record.StatusError || failed.Args["delivery_status"] != "not_configured" {
		t.Fatalf("%+v", e.rec.evs)
	}
	if e.rp.marks[0] != "not_configured" {
		t.Fatal(e.rp.marks)
	}
}

func TestU14_MailerError(t *testing.T) {
	e := setup()
	e.mail.Err = errors.New("mailer: smtp rcpt: 550 no such user")
	res, err := e.uc.Execute(e.ctx, in())
	if err != nil || res.EmailStatus != "failed" || res.Credentials == nil {
		t.Fatalf("%v %+v", err, res)
	}
	if e.rp.marks[0] != "failed" || len(e.rec.evs) != 2 {
		t.Fatalf("%v %v", e.rp.marks, e.rec.actions())
	}
	// SentAt tidak berubah oleh status failed
	if e.rp.inv.SentAt != nil {
		t.Fatal("sent_at tidak boleh terisi")
	}
}

func TestU15_OddInviterLabelStillDelivers(t *testing.T) {
	e := setup()
	// RenderInvitationEmail tidak punya jalur gagal yang bisa dipicu dari luar; kita pastikan
	// data pengundang yang aneh tidak membuat request error.
	e.rp.inv.InvitedByLabel = ptr("<b>Evil\nName</b>")
	res, err := e.uc.Execute(e.ctx, in())
	if err != nil || (res.EmailStatus != "sent" && res.EmailStatus != "failed") {
		t.Fatalf("%v %+v", err, res)
	}
}

func ptr(s string) *string { return &s }

func TestU16_MarkDeliveryErrorLoggedOnly(t *testing.T) {
	e := setup()
	e.rp.markErr = errors.New("db mati")
	res, err := e.uc.Execute(e.ctx, in())
	if err != nil || !res.EmailSent {
		t.Fatalf("%v %+v", err, res)
	}
	if !strings.Contains(e.log.String(), "catat status kirim gagal") {
		t.Fatalf("log: %s", e.log.String())
	}
}

func TestU18_InviterLabelPriority(t *testing.T) {
	cases := []struct {
		name  string
		ctx   context.Context
		label string
		stor  *string
		want  string
	}{
		{"explicit", reqctx.WithIdentity(context.Background(), reqctx.Identity{UserID: "a", Name: "N", Email: "e@x.id"}), "Label Eksplisit", ptr("Tersimpan"), "Label Eksplisit"},
		{"ctx name", reqctx.WithIdentity(context.Background(), reqctx.Identity{UserID: "a", Name: "Nama Ctx", Email: "e@x.id"}), "", ptr("Tersimpan"), "Nama Ctx"},
		{"ctx email", reqctx.WithIdentity(context.Background(), reqctx.Identity{UserID: "a", Email: "e@x.id"}), "", ptr("Tersimpan"), "e@x.id"},
		{"stored", context.Background(), "", ptr("Tersimpan"), "Tersimpan"},
		{"none", context.Background(), "", nil, ""},
	}
	for _, c := range cases {
		got := inviterLabel(c.ctx, Input{InvitedByLabel: c.label}, base.Invitation{InvitedByLabel: c.stor})
		if got != c.want {
			t.Errorf("%s: %q != %q", c.name, got, c.want)
		}
	}
	// fallback template + ProjectName di Subject/Text
	e := setup()
	i := in()
	i.ProjectName = "Proyek Atlas"
	if _, err := e.uc.Execute(context.Background(), i); err != nil {
		t.Fatal(err)
	}
	m, _ := e.mail.Last()
	if !strings.Contains(m.Subject+m.Text, "Proyek Atlas") {
		t.Fatalf("%s %s", m.Subject, m.Text)
	}
	if !strings.Contains(m.Text, "Administrator Grimoire") {
		t.Fatalf("fallback pengundang: %s", m.Text)
	}
}

func TestU19_AuditActor(t *testing.T) {
	e := setup()
	i := in()
	i.InvitedBy, i.InvitedByLabel, i.Via = "owner-1", "Owner Satu", ViaProject
	if _, err := e.uc.Execute(context.Background(), i); err != nil {
		t.Fatal(err)
	}
	ev := e.rec.evs[0]
	if ev.Actor == nil || ev.Actor.ID != "owner-1" || ev.Actor.Label != "Owner Satu" || ev.Actor.Type != record.ActorUser {
		t.Fatalf("%+v", ev.Actor)
	}
	if ev.Args["via"] != "project" {
		t.Fatalf("%v", ev.Args)
	}
	// ctx dengan identitas -> Actor nil
	e2 := setup()
	if _, err := e2.uc.Execute(e2.ctx, i); err != nil {
		t.Fatal(err)
	}
	if e2.rec.evs[0].Actor != nil {
		t.Fatal("Actor harus nil")
	}
}

func TestU20_NoSecretLeak(t *testing.T) {
	for _, mode := range []string{"fail", "ok"} {
		e := setup()
		if mode == "fail" {
			e.mail.Err = errors.New("550")
		}
		res, err := e.uc.Execute(e.ctx, in())
		if err != nil {
			t.Fatal(err)
		}
		pw := ""
		if res.Credentials != nil {
			pw = res.Credentials.TemporaryPassword
		} else if m, ok := e.mail.Last(); ok {
			for _, f := range strings.Fields(m.Text) {
				if len(f) == base.GeneratedPasswordLen {
					if ok, _ := e.h.Verify(e.rp.hash, f); ok {
						pw = f
					}
				}
			}
		}
		if pw == "" {
			t.Fatal("password tak ditemukan")
		}
		dump := fmt.Sprintf("%v", e.rec.evs) + e.log.String()
		for _, s := range []string{pw, e.rp.hash, "temporary_password"} {
			if strings.Contains(dump, s) {
				t.Fatalf("%s: bocor %q", mode, s)
			}
		}
	}
}

func TestU22_CtxCancelledStillSends(t *testing.T) {
	e := setup()
	ctx, cancel := context.WithCancel(e.ctx)
	cancel()
	res, err := e.uc.Execute(ctx, in())
	if err != nil || !res.EmailSent || len(e.mail.Sent) != 1 {
		t.Fatalf("%v %+v", err, res)
	}
}

func TestU23_RepoErrorWrapped(t *testing.T) {
	e := setup()
	e.rp.issueErr = errors.New("koneksi putus")
	_, err := e.uc.Execute(e.ctx, in())
	if err == nil || !strings.Contains(err.Error(), "resendinvitation: terbitkan") || errors.Is(err, errs.New(errs.CodeNotFound, "")) {
		t.Fatal(err)
	}
	var ee *errs.Error
	if errors.As(err, &ee) {
		t.Fatal("bukan errs")
	}
	if len(e.mail.Sent) != 0 || len(e.rec.evs) != 0 {
		t.Fatal("tanpa email/audit")
	}
}
