package base

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"grimoire/lib/common/errs"
)

var t0 = time.Date(2026, 10, 21, 8, 0, 0, 0, time.UTC)

var pol = InvitationPolicy{TTL: 168 * time.Hour, MaxPerInviterPerHour: 20, ResendMaxPerHour: 3, PurgeAfter: 720 * time.Hour}

func details(t *testing.T, err error) map[string]any {
	t.Helper()
	var e *errs.Error
	if !asErr(err, &e) {
		t.Fatalf("bukan *errs.Error: %v", err)
	}
	return e.Details.(map[string]any)
}

func asErr(err error, target **errs.Error) bool {
	e, ok := err.(*errs.Error)
	if ok {
		*target = e
	}
	return ok
}

func TestNewInvitation(t *testing.T) {
	inv := NewInvitation("u1", "adm", "  ", t0, pol)
	if inv.InvitedByLabel != nil || inv.DeliveryStatus != DeliveryPending || !inv.IssuedAt.Equal(t0) ||
		!inv.ExpiresAt.Equal(t0.Add(pol.TTL)) || !inv.CreatedAt.Equal(t0) || !inv.IsPending() {
		t.Fatalf("%+v", inv)
	}
	if l := NewInvitation("u1", "adm", " Owner ", t0, pol).InvitedByLabel; l == nil || *l != "Owner" {
		t.Fatal("label")
	}
}

func TestInvitation_Expiry(t *testing.T) {
	inv := NewInvitation("u", "a", "", t0, pol)
	if inv.IsExpired(inv.ExpiresAt.Add(-time.Nanosecond)) {
		t.Fatal("1ns sebelum: belum kedaluwarsa")
	}
	if !inv.IsExpired(inv.ExpiresAt) || !inv.BlocksLogin(inv.ExpiresAt) {
		t.Fatal("tepat di expires_at: kedaluwarsa")
	}
	inv.Accept(t0.Add(time.Hour))
	if inv.IsPending() || inv.IsExpired(t0.Add(1000*time.Hour)) || inv.BlocksLogin(t0.Add(1000*time.Hour)) {
		t.Fatal("diterima: tidak pending/expired/blok")
	}
	if inv.AcceptedAt == nil {
		t.Fatal("AcceptedAt")
	}
}

func TestInvitation_Reissue(t *testing.T) {
	inv := NewInvitation("u", "a", "", t0, pol)
	inv.MarkDelivery(DeliveryFailed, t0)
	later := t0.Add(200 * time.Hour)
	inv.Reissue(later, pol)
	if !inv.IssuedAt.Equal(later) || !inv.ExpiresAt.Equal(later.Add(pol.TTL)) || inv.DeliveryStatus != DeliveryPending {
		t.Fatalf("%+v", inv)
	}
	if inv.IsExpired(later) {
		t.Fatal("reissue memulihkan")
	}
}

func TestInvitation_MarkDelivery(t *testing.T) {
	inv := NewInvitation("u", "a", "", t0, pol)
	inv.MarkDelivery(DeliveryFailed, t0)
	if inv.SentAt != nil || inv.DeliveryStatus != DeliveryFailed {
		t.Fatal("failed tidak mengisi SentAt")
	}
	inv.MarkDelivery(DeliverySent, t0.Add(time.Second))
	if inv.SentAt == nil || !inv.SentAt.Equal(t0.Add(time.Second)) {
		t.Fatal("sent mengisi SentAt")
	}
}

func TestInvitation_RegisterResendWindow(t *testing.T) {
	inv := NewInvitation("u", "a", "", t0, pol)
	for i := 1; i <= 3; i++ {
		if err := inv.RegisterResend(t0.Add(time.Duration(i)*time.Minute), pol); err != nil {
			t.Fatalf("ke-%d: %v", i, err)
		}
	}
	if inv.ResendCount != 3 || inv.ResendWindowCount != 3 {
		t.Fatalf("%+v", inv)
	}
	now := t0.Add(10 * time.Minute)
	err := inv.RegisterResend(now, pol)
	if err == nil {
		t.Fatal("ke-4 harus 429")
	}
	d := details(t, err)
	// window mulai t0+1m, berakhir t0+61m; sisa dari t0+10m = 51m.
	if d["scope"] != "resend" || d["retry_after_seconds"] != 3060 {
		t.Fatalf("%v", d)
	}
	if inv.ResendCount != 3 || inv.ResendWindowCount != 3 {
		t.Fatal("gagal tidak boleh mengubah state")
	}
	// window bergeser setelah 1 jam
	if err := inv.RegisterResend(t0.Add(62*time.Minute), pol); err != nil {
		t.Fatal(err)
	}
	if inv.ResendWindowCount != 1 || inv.ResendCount != 4 {
		t.Fatalf("%+v", inv)
	}
}

func TestInvitation_CheckResendDoesNotMutate(t *testing.T) {
	inv := NewInvitation("u", "a", "", t0, pol)
	if err := inv.CheckResend(t0, pol); err != nil {
		t.Fatal(err)
	}
	if inv.ResendWindowStartedAt != nil || inv.ResendCount != 0 {
		t.Fatal("CheckResend mengubah state")
	}
	for i := 0; i < 3; i++ {
		_ = inv.RegisterResend(t0, pol)
	}
	snap := inv
	if err := inv.CheckResend(t0.Add(time.Minute), pol); err == nil {
		t.Fatal("harus 429")
	}
	if inv.ResendWindowCount != snap.ResendWindowCount {
		t.Fatal("mutasi")
	}
	if err := inv.CheckResend(t0.Add(time.Hour), pol); err != nil {
		t.Fatalf("window sudah lewat: %v", err)
	}
}

func TestCheckInviteTarget(t *testing.T) {
	pending := NewInvitation("u", "a", "", t0, pol)
	accepted := pending
	accepted.Accept(t0)
	expired := pending
	expired.ExpiresAt = t0.Add(-time.Hour)
	dev := User{IsActive: true, Role: RoleDeveloper}
	inactive := User{IsActive: false, Role: RoleDeveloper}
	admin := User{IsActive: true, Role: RoleSuperAdmin}
	cases := []struct {
		name   string
		u      *User
		inv    *Invitation
		mode   InviteMode
		reason string
	}{
		{"nil user", nil, nil, InviteCreateNew, ""},
		{"pending aktif developer", &dev, &pending, InviteReissuePending, ""},
		{"pending expired", &dev, &expired, InviteReissuePending, ""},
		{"diterima", &dev, &accepted, 0, AlreadyRegisteredRegistered},
		{"tanpa undangan", &dev, nil, 0, AlreadyRegisteredRegistered},
		{"nonaktif pending", &inactive, &pending, 0, AlreadyRegisteredInactive},
		{"nonaktif tanpa undangan", &inactive, nil, 0, AlreadyRegisteredInactive},
		{"super_admin", &admin, nil, 0, AlreadyRegisteredNotDeveloper},
		{"super_admin pending", &admin, &pending, 0, AlreadyRegisteredNotDeveloper},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			mode, err := CheckInviteTarget(c.u, c.inv)
			if c.reason == "" {
				if err != nil || mode != c.mode {
					t.Fatalf("mode %v err %v", mode, err)
				}
				return
			}
			var e *errs.Error
			if !asErr(err, &e) || e.Code != errs.CodeAlreadyRegistered || details(t, err)["reason"] != c.reason {
				t.Fatalf("err %v", err)
			}
		})
	}
}

func TestCheckInviterQuota(t *testing.T) {
	if err := CheckInviterQuota(19, t0, t0.Add(time.Minute), pol); err != nil {
		t.Fatal(err)
	}
	err := CheckInviterQuota(20, t0, t0.Add(30*time.Minute), pol)
	if err == nil {
		t.Fatal("harus 429")
	}
	d := details(t, err)
	if d["scope"] != "invite" || d["retry_after_seconds"] != 1800 {
		t.Fatalf("%v", d)
	}
	d = details(t, CheckInviterQuota(20, t0, t0.Add(2*time.Hour), pol))
	if d["retry_after_seconds"] != 1 {
		t.Fatalf("minimal 1 detik: %v", d)
	}
}

func TestDefaultNameFromEmail(t *testing.T) {
	cases := map[string]string{
		"budi.santoso@x.id": "budi santoso",
		"a_b-c+d@x.id":      "a b c d",
		"@x.id":             "Developer",
		"...@x.id":          "Developer",
		"plain@x.id":        "plain",
	}
	for in, want := range cases {
		if got := DefaultNameFromEmail(in); got != want {
			t.Errorf("%q: got %q want %q", in, got, want)
		}
	}
	long := DefaultNameFromEmail(strings.Repeat("é", 300) + "@x.id")
	if utf8.RuneCountInString(long) != 100 {
		t.Fatalf("rune = %d", utf8.RuneCountInString(long))
	}
	for _, in := range []string{"a@b.id", "@x", "x.y@z", strings.Repeat("a", 300) + "@x"} {
		if _, err := ValidateName(DefaultNameFromEmail(in)); err != nil {
			t.Errorf("%q: %v", in, err)
		}
	}
}

func TestInvitation_ToResponse(t *testing.T) {
	inv := NewInvitation("u", "a", "", t0, pol)
	inv.ResendCount = 2
	dto := inv.ToResponse(t0.Add(time.Hour))
	if dto == nil || !dto.Pending || dto.Expired || !dto.InvitedAt.Equal(t0) || dto.ResendCount != 2 || dto.DeliveryStatus != DeliveryPending {
		t.Fatalf("%+v", dto)
	}
	if !inv.ToResponse(inv.ExpiresAt).Expired {
		t.Fatal("now == expires_at -> expired")
	}
	inv.Accept(t0)
	if inv.ToResponse(t0) != nil {
		t.Fatal("diterima -> nil")
	}
}

func TestUser_ToResponseWithInvitation(t *testing.T) {
	u := User{ID: "1", Email: "a@b.id", Name: "A", Role: RoleDeveloper, IsActive: true, PasswordHash: "SECRET-HASH"}
	b, _ := json.Marshal(u.ToResponseWithInvitation(nil, t0))
	if !strings.Contains(string(b), `"invitation":null`) {
		t.Fatalf("harus memuat invitation:null: %s", b)
	}
	b, _ = json.Marshal(u.ToResponse())
	if !strings.Contains(string(b), `"invitation":null`) {
		t.Fatalf("ToResponse lama juga null: %s", b)
	}
	inv := NewInvitation("1", "adm", "", t0, pol)
	b, _ = json.Marshal(u.ToResponseWithInvitation(&inv, t0))
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	i := m["invitation"].(map[string]any)
	for _, k := range []string{"pending", "invited_at", "sent_at", "expires_at", "expired", "delivery_status", "resend_count"} {
		if _, ok := i[k]; !ok {
			t.Errorf("kunci %s hilang", k)
		}
	}
	if _, ok := i["issued_at"]; ok || strings.Contains(string(b), "SECRET-HASH") {
		t.Fatal("tidak boleh bocor")
	}
}
