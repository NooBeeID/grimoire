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

Detail masuk
------------
Masuk di: {{.LoginURL}}
Email: {{.InviteeEmail}}
Password sementara: {{.Password}}

Segera ganti password setelah login. Password ini berlaku sampai {{.Expires}}.

Jika Anda tidak merasa diundang, abaikan email ini.

--
Grimoire, knowledge base untuk tim dan agent AI.
`

// htmlTmpl memakai tabel dan inline style supaya aman di klien email (Gmail, Outlook, Apple
// Mail). Warna mengikuti DESIGN.md: kerangka biru-abu, sheet putih, aksen kuning, aksi near-black.
// Blok <style> hanya untuk mode gelap dan lebar ponsel; tanpa itu tampilan terang tetap utuh.
const htmlTmpl = `<!doctype html>
<html lang="id" xmlns="http://www.w3.org/1999/xhtml">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<meta name="color-scheme" content="light dark">
<meta name="supported-color-schemes" content="light dark">
<title>{{if .Project}}Undangan ke project {{.Project}}{{else}}Undangan bergabung ke Grimoire{{end}}</title>
<style>
  @media (max-width:620px){
    .wrap{padding:16px 12px !important}
    .pad{padding:24px 20px !important}
    .btn a{display:block !important}
  }
  @media (prefers-color-scheme:dark){
    .bg-ground{background:#0f1214 !important}
    .sheet{background:#17191c !important;border-color:#2a2d31 !important}
    .ink{color:#f1f2f3 !important}
    .muted{color:#a4a7ab !important}
    .panel{background:#1f2226 !important;border-color:#2a2d31 !important}
    .code{background:#17191c !important;border-color:#33373c !important;color:#f5c518 !important}
    .btn-a{background:#f1f2f3 !important;color:#131416 !important}
    .chip{background:#2a2d31 !important;color:#d6d8da !important}
    .rule{border-color:#2a2d31 !important}
    .link{color:#f5c518 !important}
  }
</style>
</head>
<body class="bg-ground" style="margin:0;padding:0;background:#e8eff2;-webkit-text-size-adjust:100%">
<div style="display:none;max-height:0;overflow:hidden;opacity:0;color:transparent;mso-hide:all">{{.Inviter}} mengundang Anda{{if .Project}} ke project {{.Project}}{{end}}. Password sementara berlaku sampai {{.Expires}}.&#847;&zwnj;&nbsp;&#847;&zwnj;&nbsp;&#847;&zwnj;&nbsp;</div>
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" class="bg-ground" style="background:#e8eff2">
<tr><td align="center" class="wrap" style="padding:40px 16px">
  <table role="presentation" width="560" cellpadding="0" cellspacing="0" border="0" style="width:100%;max-width:560px">
    <tr><td style="padding:0 4px 20px">
      <table role="presentation" cellpadding="0" cellspacing="0" border="0"><tr>
        <td width="32" height="32" align="center" valign="middle" style="width:32px;height:32px;background:#f2b705;border-radius:8px;font:700 17px/32px Arial,Helvetica,sans-serif;color:#1d1600">G</td>
        <td class="ink" style="padding-left:10px;font:600 17px/32px -apple-system,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;color:#131416;letter-spacing:-.1px">Grimoire</td>
      </tr></table>
    </td></tr>
    <tr><td class="sheet" style="background:#ffffff;border:1px solid #e8eaec;border-radius:16px">
      <table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0">
        <tr><td height="4" style="height:4px;line-height:4px;font-size:0;background:#f2b705;border-radius:16px 16px 0 0">&nbsp;</td></tr>
        <tr><td class="pad" style="padding:32px 36px 28px;font-family:-apple-system,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;color:#131416">
          <h1 class="ink" style="margin:0 0 12px;font:600 22px/1.3 -apple-system,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;color:#131416;letter-spacing:-.2px">{{if .Project}}Bergabung ke project {{.Project}}{{else}}Anda diundang ke Grimoire{{end}}</h1>
          <p class="muted" style="margin:0 0 24px;font-size:15px;line-height:1.6;color:#5d5f60"><strong class="ink" style="color:#131416;font-weight:600">{{.Inviter}}</strong> mengundang Anda untuk bergabung ke Grimoire{{if .Project}} pada project <strong class="ink" style="color:#131416;font-weight:600">{{.Project}}</strong>{{end}}. Masuk dengan detail di bawah, lalu pilih password Anda sendiri.</p>

          <table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" class="panel" style="background:#f4f6f8;border:1px solid #e8eaec;border-radius:8px">
            <tr><td style="padding:16px 18px 4px">
              <div class="muted" style="font-size:12px;line-height:1.4;color:#5d5f60">Email</div>
              <div class="ink" style="margin-top:2px;font-size:14px;line-height:1.5;color:#131416;word-break:break-all">{{.InviteeEmail}}</div>
            </td></tr>
            <tr><td style="padding:12px 18px 18px">
              <div class="muted" style="font-size:12px;line-height:1.4;color:#5d5f60;margin-bottom:6px">Password sementara</div>
              <div class="code" style="display:inline-block;padding:10px 14px;background:#ffffff;border:1px solid #cdced1;border-radius:6px;font:500 18px/1.3 'IBM Plex Mono',ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;letter-spacing:.5px;color:#131416;word-break:break-all">{{.Password}}</div>
            </td></tr>
          </table>

          <table role="presentation" cellpadding="0" cellspacing="0" border="0" class="btn" style="margin:28px 0 0;width:100%"><tr>
            <td align="left"><a class="btn-a" href="{{.LoginURL}}" style="display:inline-block;padding:12px 24px;background:#131416;color:#ffffff;border-radius:6px;font:600 15px/1.2 -apple-system,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;text-decoration:none;text-align:center">Masuk ke Grimoire</a></td>
          </tr></table>
          <p class="muted" style="margin:14px 0 0;font-size:12px;line-height:1.6;color:#5d5f60">Atau buka tautan ini: <a class="link" href="{{.LoginURL}}" style="color:#7a5600;word-break:break-all">{{.LoginURL}}</a></p>

          <table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" class="rule" style="margin-top:28px;border-top:1px solid #e8eaec"><tr>
            <td style="padding-top:20px;font-size:13px;line-height:1.6;color:#5d5f60" class="muted">
              <strong class="ink" style="color:#131416;font-weight:600">Segera ganti password setelah login.</strong> Password sementara ini berlaku sampai {{.Expires}}; setelah itu undangan perlu dikirim ulang.
            </td>
          </tr></table>
        </td></tr>
      </table>
    </td></tr>
    <tr><td style="padding:20px 8px 0;font-family:-apple-system,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;font-size:12px;line-height:1.6;color:#5d5f60" class="muted" align="center">
      Jika Anda tidak merasa diundang, abaikan email ini.<br>
      Grimoire, knowledge base untuk tim dan agent AI.
    </td></tr>
  </table>
</td></tr>
</table>
</body>
</html>
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
