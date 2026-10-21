package base

import (
	"encoding/json"
	"testing"
	"time"

	"grimoire/lib/contracts/core/auth/getuser"
)

func pendingUser(deliv string, expired bool) getuser.Response {
	sent := t0.Add(time.Minute)
	return getuser.Response{
		Found: true, ID: "u1", Name: "Budi", Email: "budi@x.id", Role: "developer", IsActive: true,
		Invitation: &getuser.Invitation{
			InvitedAt: t0, IssuedAt: t0, ExpiresAt: t0.Add(168 * time.Hour), SentAt: &sent,
			Expired: expired, DeliveryStatus: deliv, ResendCount: 2,
		},
	}
}

func TestEmailFailed(t *testing.T) {
	for st, want := range map[string]bool{"sent": false, "pending": false, "failed": true, "not_configured": true, "": false} {
		if got := EmailFailed(st); got != want {
			t.Errorf("%q: got %v want %v", st, got, want)
		}
	}
}

func TestMember_ToInvitationDTO(t *testing.T) {
	label := "Owner Satu"
	m := Member{UserID: "u1", Role: RoleMember, AddedByLabel: &label, AddedAt: t0}
	d, ok := m.ToInvitationDTO(pendingUser("failed", true))
	if !ok || d.UserID != "u1" || d.Email != "budi@x.id" || d.Name != "Budi" || d.Role != RoleMember ||
		d.InvitedByLabel == nil || *d.InvitedByLabel != label || !d.AddedAt.Equal(t0) || d.SentAt == nil ||
		!d.ExpiresAt.Equal(t0.Add(168*time.Hour)) || !d.Expired || d.DeliveryStatus != "failed" || !d.EmailFailed || d.ResendCount != 2 {
		t.Fatalf("%+v", d)
	}
	if d, _ := m.ToInvitationDTO(pendingUser("sent", false)); d.EmailFailed || d.Expired {
		t.Fatalf("%+v", d)
	}
	if _, ok := m.ToInvitationDTO(getuser.Response{}); ok {
		t.Fatal("Found=false -> ok=false")
	}
	nopending := pendingUser("sent", false)
	nopending.Invitation = nil
	if _, ok := m.ToInvitationDTO(nopending); ok {
		t.Fatal("tanpa undangan -> ok=false")
	}
}

func TestMember_ToInvitationDTO_CopiesSentAt(t *testing.T) {
	u := pendingUser("sent", false)
	d, _ := Member{UserID: "u1", Role: RoleMember, AddedAt: t0}.ToInvitationDTO(u)
	*d.SentAt = t0.Add(99 * time.Hour)
	if u.Invitation.SentAt.Equal(*d.SentAt) {
		t.Fatal("SentAt tidak boleh berbagi memori dengan respons bus")
	}
}

func TestSortInvitations(t *testing.T) {
	items := []InvitationDTO{
		{UserID: "b", AddedAt: t0},
		{UserID: "c", AddedAt: t0.Add(time.Hour)},
		{UserID: "a", AddedAt: t0},
	}
	SortInvitations(items)
	if items[0].UserID != "c" || items[1].UserID != "a" || items[2].UserID != "b" {
		t.Fatalf("%+v", items)
	}
}

func TestInvitationDTO_JSONKeys(t *testing.T) {
	d, _ := Member{UserID: "u1", Role: RoleMember, AddedAt: t0}.ToInvitationDTO(pendingUser("sent", false))
	b, _ := json.Marshal(d)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	for _, k := range []string{"user_id", "email", "name", "role", "invited_by_label", "added_at", "sent_at", "expires_at", "expired", "delivery_status", "email_failed", "resend_count"} {
		if _, ok := m[k]; !ok {
			t.Errorf("kunci %s hilang: %s", k, b)
		}
	}
}

func TestMemberToDTO_InvitationMarkers(t *testing.T) {
	m := Member{UserID: "u1", Role: RoleMember, AddedAt: t0}
	d := m.ToDTO(pendingUser("sent", true))
	if !d.InvitationPending || d.InvitationExpiresAt == nil || !d.InvitationExpiresAt.Equal(t0.Add(168*time.Hour)) {
		t.Fatalf("%+v", d)
	}
	u := pendingUser("sent", false)
	u.Invitation = nil
	b, _ := json.Marshal(m.ToDTO(u))
	var mm map[string]any
	_ = json.Unmarshal(b, &mm)
	if mm["invitation_pending"] != false || mm["invitation_expires_at"] != nil {
		t.Fatalf("%s", b)
	}
	if _, ok := mm["invitation_expires_at"]; !ok {
		t.Fatal("kunci harus selalu hadir")
	}
	if d := m.ToDTO(getuser.Response{}); d.InvitationPending {
		t.Fatal("Found=false tidak pending")
	}
}

func TestCandidateDTO_HasInvitationPending(t *testing.T) {
	b, _ := json.Marshal(CandidateDTO{ID: "1"})
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	if v, ok := m["invitation_pending"]; !ok || v != false {
		t.Fatalf("%s", b)
	}
}
