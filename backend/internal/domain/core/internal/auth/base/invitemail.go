package base

import (
	"bytes"
	"fmt"
	htmltemplate "html/template"
	"strings"
	texttemplate "text/template"
	"time"
)

// InvitationEmailData adalah data untuk email undangan.
type InvitationEmailData struct {
	InviteeEmail, InviterName, ProjectName, LoginURL, TemporaryPassword string
	ExpiresAt                                                           time.Time
}

// EmailContent adalah isi email yang sudah dirender.
type EmailContent struct{ Subject, Text, HTML string }

const fallbackInviter = "Administrator Grimoire"

var bulanIndonesia = [...]string{"Januari", "Februari", "Maret", "April", "Mei", "Juni", "Juli",
	"Agustus", "September", "Oktober", "November", "Desember"}

// formatExpiry memformat "2 Januari 2006 15:04 UTC" dengan nama bulan Indonesia (tabel lokal).
func formatExpiry(t time.Time) string {
	t = t.UTC()
	return fmt.Sprintf("%d %s %d %02d:%02d UTC", t.Day(), bulanIndonesia[t.Month()-1], t.Year(), t.Hour(), t.Minute())
}

// oneLine mengganti semua whitespace (termasuk CR/LF) menjadi satu spasi.
func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

type emailView struct {
	InviteeEmail, Inviter, Project, LoginURL, Password, Expires string
}

const textTmpl = `Halo,

{{.Inviter}} mengundang Anda untuk bergabung ke Grimoire{{if .Project}} pada project "{{.Project}}"{{end}}.

Masuk di: {{.LoginURL}}
Email: {{.InviteeEmail}}
Password sementara: {{.Password}}

Segera ganti password setelah login. Password ini berlaku sampai {{.Expires}}.

Jika Anda tidak merasa diundang, abaikan email ini.
`

const htmlTmpl = `<!doctype html>
<html lang="id"><body style="font-family:sans-serif;line-height:1.5">
<p>Halo,</p>
<p><strong>{{.Inviter}}</strong> mengundang Anda untuk bergabung ke Grimoire{{if .Project}} pada project <strong>{{.Project}}</strong>{{end}}.</p>
<p>Masuk di: <a href="{{.LoginURL}}">{{.LoginURL}}</a><br>
Email: {{.InviteeEmail}}<br>
Password sementara: <code>{{.Password}}</code></p>
<p>Segera ganti password setelah login. Password ini berlaku sampai {{.Expires}}.</p>
<p>Jika Anda tidak merasa diundang, abaikan email ini.</p>
</body></html>
`

var (
	invitationText = texttemplate.Must(texttemplate.New("invitation.txt").Parse(textTmpl))
	invitationHTML = htmltemplate.Must(htmltemplate.New("invitation.html").Parse(htmlTmpl))
)

// RenderInvitationEmail merender email undangan (Bahasa Indonesia). Murni: tanpa I/O.
// Nama pengundang dan project dibersihkan dari newline; HTML di-escape otomatis.
func RenderInvitationEmail(d InvitationEmailData) (EmailContent, error) {
	v := emailView{
		InviteeEmail: d.InviteeEmail,
		Inviter:      oneLine(d.InviterName),
		Project:      oneLine(d.ProjectName),
		LoginURL:     d.LoginURL,
		Password:     d.TemporaryPassword,
		Expires:      formatExpiry(d.ExpiresAt),
	}
	if v.Inviter == "" {
		v.Inviter = fallbackInviter
	}
	subject := "Undangan bergabung ke Grimoire"
	if v.Project != "" {
		subject = "Undangan bergabung ke project " + v.Project + " di Grimoire"
	}
	var tb, hb bytes.Buffer
	if err := invitationText.Execute(&tb, v); err != nil {
		return EmailContent{}, fmt.Errorf("base: render email teks: %w", err)
	}
	if err := invitationHTML.Execute(&hb, v); err != nil {
		return EmailContent{}, fmt.Errorf("base: render email html: %w", err)
	}
	return EmailContent{Subject: subject, Text: tb.String(), HTML: hb.String()}, nil
}
