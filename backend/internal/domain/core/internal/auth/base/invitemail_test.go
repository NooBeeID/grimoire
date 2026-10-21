package base

import (
	"strings"
	"testing"
	"time"
)

func sampleEmail() InvitationEmailData {
	return InvitationEmailData{
		InviteeEmail: "baru@contoh.id", InviterName: "Owner Satu", ProjectName: "Proyek Alfa",
		LoginURL: "https://g.x/login", TemporaryPassword: "q7XkR2mVtz9HbNcP",
		ExpiresAt: time.Date(2026, 1, 2, 15, 4, 0, 0, time.UTC),
	}
}

func TestRenderInvitationEmail_AllFields(t *testing.T) {
	c, err := RenderInvitationEmail(sampleEmail())
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range []string{"Owner Satu", "Proyek Alfa", "https://g.x/login", "baru@contoh.id", "q7XkR2mVtz9HbNcP", "2 Januari 2026 15:04 UTC", "Segera ganti password"} {
		if !strings.Contains(c.Text, w) {
			t.Errorf("Text tanpa %q", w)
		}
		if !strings.Contains(c.HTML, w) {
			t.Errorf("HTML tanpa %q", w)
		}
	}
	if c.Subject != "Undangan bergabung ke project Proyek Alfa di Grimoire" {
		t.Fatalf("subject %q", c.Subject)
	}
}

func TestRenderInvitationEmail_ProjectOptionalAndInviterFallback(t *testing.T) {
	d := sampleEmail()
	d.ProjectName, d.InviterName = "", ""
	c, err := RenderInvitationEmail(d)
	if err != nil {
		t.Fatal(err)
	}
	if c.Subject != "Undangan bergabung ke Grimoire" {
		t.Fatalf("subject %q", c.Subject)
	}
	if !strings.Contains(c.Text, "Administrator Grimoire") || strings.Contains(c.Text, "pada project") {
		t.Fatalf("text: %s", c.Text)
	}
}

func TestRenderInvitationEmail_EscapesHTML(t *testing.T) {
	d := sampleEmail()
	d.InviterName = "<script>alert(1)</script>"
	d.ProjectName = "<img src=x onerror=1>"
	c, err := RenderInvitationEmail(d)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(c.HTML, "<script>") || strings.Contains(c.HTML, "<img src=x") {
		t.Fatalf("HTML tidak di-escape: %s", c.HTML)
	}
	if !strings.Contains(c.HTML, "&lt;script&gt;") {
		t.Fatal("harus ter-escape")
	}
}

func TestRenderInvitationEmail_SubjectNoNewline(t *testing.T) {
	d := sampleEmail()
	d.ProjectName = "Alfa\r\nBcc: x@y.id"
	c, err := RenderInvitationEmail(d)
	if err != nil {
		t.Fatal(err)
	}
	if strings.ContainsAny(c.Subject, "\r\n") {
		t.Fatalf("subject: %q", c.Subject)
	}
}

func TestFormatExpiry_AllMonths(t *testing.T) {
	want := []string{"Januari", "Februari", "Maret", "April", "Mei", "Juni", "Juli", "Agustus", "September", "Oktober", "November", "Desember"}
	for m := 1; m <= 12; m++ {
		got := formatExpiry(time.Date(2026, time.Month(m), 5, 9, 7, 0, 0, time.FixedZone("x", 7*3600)))
		if !strings.Contains(got, want[m-1]) {
			t.Errorf("bulan %d: %s", m, got)
		}
	}
	if got := formatExpiry(time.Date(2026, 3, 5, 9, 7, 0, 0, time.UTC)); got != "5 Maret 2026 09:07 UTC" {
		t.Fatal(got)
	}
}
