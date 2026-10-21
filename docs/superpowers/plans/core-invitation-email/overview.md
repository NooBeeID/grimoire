# T12 core-invitation-email — Overview Plan (FOUNDATION)

Tiket: `docs/tickets/T12-core-invitation-email.md`
Sumber aturan: `rules.txt` (menang untuk arsitektur), `docs/tickets/README.md` (konvensi, katalog bus, katalog audit), `.claude/rules/migrations.md`, `docs/superpowers/specs/2026-10-20-grimoire-design.md`.
Plan pendahulu yang dirujuk: `docs/superpowers/plans/core-auth/overview.md` (T01), `docs/superpowers/plans/project-management/overview.md` (T03).
Konteks yang dicek: tidak ada `CLAUDE.md`. `.claude/rules/` hanya berisi `migrations.md`. Tidak ada knowledge doc khusus module auth/management selain plan T01/T03.

Dokumen ini adalah plan FOUNDATION T12: keputusan bersama, isi `base/`, paket `infra/mailer`, konfigurasi, migrasi, kontrak bus, aksi audit, urutan implementasi, dan daftar usecase. Plan rinci per usecase ditulis terpisah di `docs/superpowers/plans/core-invitation-email/usecase-<slug>.md`, plan frontend di `frontend-plan.md`. Semua plan usecase WAJIB mengikuti dokumen ini. Bila bertentangan, dokumen ini yang menang sampai diperbarui.

---

## 0. Fakta kode yang menentukan desain

Diverifikasi dari kode `main` (`429d33b`), bukan asumsi:

1. `project_members.user_id` TIDAK punya FK ke `users` (`backend/migrations/project/20261022090000000_create_projects.sql`). Menghapus user tidak menghapus keanggotaan, dan auth tidak boleh membaca `project_members`. Karena itu job purge butuh endpoint bus baru milik management: `project.management.count-memberships`.
2. Bus in-process: `Registry.Call` meneruskan `ctx` (identitas `reqctx` ikut) dan mengembalikan error handler apa adanya. `*errs.Error` dari auth sampai utuh ke management (`base/users` membungkus dengan `%w`, `errors.As` tetap jalan). Bus tidak me-log payload.
3. Pola T03 (`addmember`): tidak ada panggilan bus di dalam tx. Audit dicatat setelah commit.
4. Route user T01 memakai param `{id}` (`/users/{id}/reset-password`). Route baru di auth WAJIB `{id}` juga (tiket menulis `{userID}`; itu akan bentrok dengan pohon chi `/users/{id}`). Route di `/projects` memakai `{projectID}` dan `{userID}` (aturan README).
5. Respons JSON repo memakai snake_case (`UserDTO`, `MemberDTO`). Field tiket `emailSent`/`credentials{temporaryPassword, loginUrl}` ditulis `email_sent`, `email_status`, `credentials{email, temporary_password, login_url}`.
6. Sudah ada dan dipakai ulang: `base.GeneratePassword` (16 karakter, alfabet tanpa `0/O/1/l/I`, `crypto/rand`; memenuhi K1), `base.Secret`, `base.Argon2Hasher`, pola "kredensial sekali tampil" + `Cache-Control: no-store` di `resetpassword`, `errs.CodeRateLimited` (429), `config.AppEnv` (`production` | `development`), `config.PublicURL` (sudah di-trim `/` akhir).
7. `httpx.RegisterCodeStatus` panic bila kode didaftarkan dua kali. Kode error yang dihasilkan auth lalu diteruskan management harus ada di peta status bawaan, bukan didaftarkan per module (lihat 0.1 dan 4.1).
8. `app.Deps` dirakit di DUA tempat: `backend/cmd/internal/server/run.go` (baris `deps := app.Deps{...}`) dan `backend/cmd/internal/server/domains.go`. Field baru harus diisi di keduanya.

## 0.1 Penyimpangan dari tiket (keputusan pembangun, dicatat untuk pemilik)

| Tiket | Plan | Alasan |
|---|---|---|
| Env `APP_BASE_URL` | Pakai `PUBLIC_URL` yang sudah ada. Login URL = `PUBLIC_URL + "/login"`. `PUBLIC_URL` wajib bila `MAIL_DRIVER=smtp`. Bila kosong (hanya mungkin di driver `log`/kosong), `login_url = "/login"` dan FE meresolusinya dengan `window.location.origin`. | Satu sumber URL publik; tidak menduplikasi config. |
| Template email di `infra/mailer` | Template di `core/auth/base/invitemail.go` (murni). | `infra/mailer` tetap generik (tujuan 6) dan infra tidak tahu bisnis (rule 4.6). |
| `POST /api/users/{userID}/resend-invitation` | `POST /api/users/{id}/resend-invitation` | Konsisten dengan route T01, cegah konflik param chi. |
| Satu tx: buat akun + tambah member + commit + kirim | Tiga langkah: siapkan (auth) -> tambah member (management) -> terbitkan + kirim (auth). Lihat bagian 3. | Lintas module tidak bisa satu tx; bus dilarang di dalam tx. |
| Kontrak bus resend `{UserID, InvitedBy, ProjectName}` | Tambah `InvitedByLabel` dan `Initial`. Respons tambah `EmailStatus`, `ExpiresAt`. | Dibutuhkan alur tiga langkah dan UI. |
| Model data tiket | Tambah kolom `issued_at`, `delivery_status`, `resend_window_started_at`, `resend_window_count`. | Status kirim untuk UI ("email gagal"), rate limit kirim ulang persisten. |
| `ALREADY_REGISTERED` sebagai kode module auth | Kode bersama `errs.CodeAlreadyRegistered` di `lib/common/errs` + peta status bawaan (409), mengikuti preseden `CodePasswordChangeRequired`. | Dihasilkan auth dan diteruskan management; `RegisterCodeStatus` ganda = panic. |

---

## 1. Metadata

| Atribut | Nilai |
|---|---|
| Ticket | T12 |
| Feature | core-invitation-email |
| Scope | fullstack (backend di plan usecase; frontend di `frontend-plan.md`) |
| Domain | infra (mailer, config) + core (auth) + project (management) |
| Module jangkar | core/auth (`backend/internal/domain/core/internal/auth`) |
| Module lain | project/management (`backend/internal/domain/project/internal/management`), paket baru `backend/internal/infra/mailer` |
| Versi kontrak REST | Tanpa prefix versi di URL. Naikkan `info.version` minor di `api/modules/core/auth.yml` dan `api/modules/project/management.yml` |
| Bergantung pada | T01, T02, T03 (semua merged) |

Endpoint HTTP:

| Endpoint | Method | Version | Auth | Module / usecase slug |
|---|---|---|---|---|
| `/api/users/invite` | POST | - | `RequireAuth` + `RequireRole(super_admin)` | core/auth `auth-inviteuser` |
| `/api/users/{id}/resend-invitation` | POST | - | `RequireAuth` + `RequireRole(super_admin)` | core/auth `auth-resendinvitation` |
| `/api/projects/{projectID}/invitations` | POST | - | `RequireAuth` (owner/super_admin lewat `PermManageMembers`) | management `management-inviteuser` |
| `/api/projects/{projectID}/invitations` | GET | - | `RequireAuth` (owner/super_admin) | management `management-listinvitations` |
| `/api/projects/{projectID}/invitations/{userID}/resend` | POST | - | `RequireAuth` (owner/super_admin) | management `management-resendinvitation` |
| `/api/projects/{projectID}/invitations/{userID}` | DELETE | - | `RequireAuth` (owner/super_admin) | management `management-revokeinvitation` |
| `/api/auth/login` | POST | - (aditif perilaku) | publik | `auth-login-changepassword` |
| `/api/auth/change-password` | POST | - (aditif perilaku) | `RequireAuth` | `auth-login-changepassword` |
| `/api/users`, `/api/users/{id}` | GET | - (aditif `invitation`) | super_admin | `auth-invitation-markers` |
| `/api/auth/me` | GET | - (aditif `email_enabled`) | `RequireAuth` | `auth-invitation-markers` |
| `/api/projects/{projectID}/members` | GET | - (aditif `invitation_pending`, `invitation_expires_at`) | anggota | `management-listinvitations` (perubahan base) |
| `/api/projects/{projectID}/member-candidates` (route T03 yang ada) | GET | - (aditif `invitation_pending`) | owner/super_admin | `management-listinvitations` (perubahan base) |

Endpoint bus:

| Endpoint | Mode | Version | Provider | Consumer |
|---|---|---|---|---|
| `core.auth.invite-user` | Call | baru | core/auth `inviteuser` | management `inviteuser` |
| `core.auth.resend-invitation` | Call | baru | core/auth `resendinvitation` | management `inviteuser`, `resendinvitation` |
| `project.management.count-memberships` | Call | baru | management `countmemberships` | core/auth `purgeinvitations` |
| `core.auth.get-user` | Call | aditif (`Request.Email`, `Response.Invitation`) | core/auth `getuserbus` | T03, T06, management baru |
| `core.auth.find-developers` | Call | aditif (`Developer.InvitationPending`) | core/auth `finddevelopers` | management `searchmembercandidates` |
| `core.audit.record` | Call | tetap | T02 | semua usecase T12 yang mengaudit |

Job: `core.auth.purge-invitations` (DailyAt `03:30` UTC), usecase `auth-purgeinvitations`.

---

## 2. Summary Plan

T12 menambah jalur undangan lewat email. Owner (atau Super Admin) bisa mengundang developer yang belum punya akun dari dialog tambah anggota project; Super Admin juga bisa mengundang dari halaman Users. Sistem membuat akun developer dengan password sementara (hash argon2id, `must_change_password=true`), mengirimnya lewat SMTP setelah commit, dan bila pengiriman gagal atau email belum diatur, mengembalikan kredensial satu kali di respons (`no-store`).

Isi foundation:

- Paket generik baru `infra/mailer` (driver `smtp`, `log`, no-op) masuk `app.Deps`.
- Field config baru untuk mail dan kebijakan undangan, dengan validasi (driver `log` ditolak di produksi).
- Satu tabel baru milik core/auth: `user_invitations` (migrasi core). Tidak ada perubahan `users`, tidak ada migrasi project.
- Entity murni `base/invitation.go`, template email murni `base/invitemail.go`, SQL bersama di `base/store`, helper pengiriman `base/delivery`.
- Tiga endpoint bus baru, dua kontrak aditif, enam aksi audit baru.
- Satu kode error bersama baru `ALREADY_REGISTERED` (409).
- Mailpit di docker-compose untuk dev lokal.

Usecase (ditulis agent lain): 5 plan di core/auth, 5 plan di management, plus 1 plan frontend.

---

## 3. Alur lintas module (keputusan utama)

### 3.1 Jalur project (management)

Satu transaksi seperti di diagram tiket tidak mungkin: auth dan management module berbeda, dan bus dilarang di dalam tx. Supaya email tidak pernah terkirim sebelum keanggotaan tercatat, dan kegagalan SMTP tidak meninggalkan keadaan setengah jadi, alurnya tiga langkah:

```
management/inviteuser (HTTP)
 0. actor, FindAccess + Require(PermManageMembers)        (member 403, non-anggota 404)
 0b. bus core.auth.get-user {Email}                        (Found & sudah anggota -> 409 ALREADY_MEMBER)
 1. bus core.auth.invite-user {Deliver:false}  ---------->  auth/inviteuser
       ALREADY_REGISTERED / RATE_LIMITED / INVALID diteruskan apa adanya
       akun baru: hash dari password acak yang DIBUANG, must_change=true, aktif,
                  baris user_invitations delivery_status='pending'
       akun menunggu (K6): tidak diubah; kuota resend DICEK tanpa increment
       audit user.invited (via="project")
 2. tx management: LockProject, LockMembers, cek ulang izin, ALREADY_MEMBER, InsertMember
 3. bus core.auth.resend-invitation {Initial: Created} -->  auth/resendinvitation
       tx auth: password sementara baru, hash, must_change=true, cabut semua sesi,
                Reissue (expires=now+TTL), kuota resend++ hanya bila !Initial
       setelah commit: kirim email -> UpdateInvitation(delivery_status, sent_at) tx kecil
       gagal kirim: audit user.invitation.email_failed + kembalikan Credentials
       audit user.invitation.resent hanya bila !Initial
 4. audit project.member.invited
 5. 201 {member, created, email_sent, email_status, expires_at, credentials?}
```

- Langkah 3 gagal total (error bus/DB): keanggotaan sudah tercatat. Management membalas 201 `email_sent=false`, `email_status="failed"`, TANPA `credentials`, dan me-log error. Owner bisa "Kirim ulang" dari daftar undangan (anti-tujuan 4 terpenuhi).
- Langkah 2 gagal (race/ALREADY_MEMBER): akun menunggu tanpa password yang diketahui siapa pun tetap ada; dipakai ulang lewat K6 atau dihapus purge.
- Biaya: dua hash argon2 di jalur project. Diterima demi konsistensi.

### 3.2 Jalur Users (auth HTTP)

`POST /api/users/invite` memanggil usecase `inviteuser` langsung dengan `Deliver=true`: siapkan, terbitkan password, kirim email dalam satu usecase tanpa bus. Untuk akun menunggu (K6) jalur ini menerbitkan ulang (sama dengan resend, termasuk kuota resend dan audit `user.invitation.resent`). Logika terbit+kirim memakai helper bersama di `base/` (bagian 4.6), bukan import feature `resendinvitation` (rule 4.3).

### 3.3 Siklus hidup undangan

| Keadaan | Definisi | Login | Change password |
|---|---|---|---|
| Tidak ada undangan | tidak ada baris `user_invitations` (akun `createuser`/admin) | perilaku T01 | perilaku T01 |
| Menunggu | baris ada, `accepted_at IS NULL`, `now < expires_at` | boleh (kena `PasswordChangeGate`) | boleh, menandai `accepted_at` |
| Kedaluwarsa | baris ada, `accepted_at IS NULL`, `now >= expires_at` | ditolak identik kredensial salah | ditolak (field `current_password`) |
| Diterima | `accepted_at IS NOT NULL` | perilaku T01 | perilaku T01 |

`resolvesession` tidak berubah. Sesi yang dibuat sebelum kedaluwarsa tetap terjebak `PasswordChangeGate`, dan `changepassword` menolak bila sudah kedaluwarsa, jadi tidak ada celah.

---

## 4. Plan (foundation, langkah berurutan)

Foundation dikerjakan sebelum semua usecase. Langkah 4.1 sampai 4.10 harus lolos `go build ./...` dan `go test ./...` sebelum usecase dimulai.

### 4.1 Kode error bersama

1. `backend/lib/common/errs/errs.go`: tambah `CodeAlreadyRegistered Code = "ALREADY_REGISTERED" // 409` di blok yang sama dengan `CodePasswordChangeRequired`.
2. `backend/internal/infra/http/respond/status.go`: tambah `errs.CodeAlreadyRegistered: http.StatusConflict` di peta bawaan.
3. Tes peta status di package `respond` (tes yang ada) ditambah satu baris.

### 4.2 Konfigurasi (`backend/internal/infra/config/config.go`)

Field baru (semua ber-tag `env`; `docs_test.go` dan `envexample_test.go` memaksa baris di `docs/configuration.md` dan `.env.example`):

| Env | Field Go | Tipe | Default | Validasi |
|---|---|---|---|---|
| `MAIL_DRIVER` | `MailDriver` | string | `""` | enum `""`, `smtp`, `log`. `log` + `APP_ENV=production` -> gagal |
| `SMTP_HOST` | `SMTPHost` | string | `""` | wajib bila `smtp` |
| `SMTP_PORT` | `SMTPPort` | int | `587` | 1..65535 |
| `SMTP_USERNAME` | `SMTPUsername` | string | `""` | opsional |
| `SMTP_PASSWORD` | `SMTPPassword` | string | `""` | disamarkan `***` di `masked()`/`String`/`LogValue`; tidak pernah masuk pesan error |
| `SMTP_TLS` | `SMTPTLS` | string | `starttls` | enum `starttls`, `tls`, `none`. `none` + produksi -> log peringatan saat start (bukan gagal) |
| `MAIL_FROM` | `MailFrom` | string | `""` | wajib bila `smtp`; `net/mail.ParseAddress` (boleh `Nama <a@b>`) |
| `MAIL_TIMEOUT` | `MailTimeout` | duration | `10s` | > 0, maks `60s` |
| `INVITATION_TTL` | `InvitationTTL` | duration | `168h` | `1h`..`720h` |
| `INVITATION_MAX_PER_HOUR` | `InvitationMaxPerHour` | int | `20` | 1..1000 |
| `INVITATION_RESEND_MAX_PER_HOUR` | `InvitationResendMaxPerHour` | int | `3` | 1..100 |
| `INVITATION_PURGE_AFTER` | `InvitationPurgeAfter` | duration | `720h` | `24h`..`8760h` |
| `PUBLIC_URL` (ada) | `PublicURL` | string | `""` | sekarang wajib bila `MAIL_DRIVER=smtp` |

- Ikuti pola parsing yang ada di `Load` (helper durasi/int/enum yang sudah dipakai field lain); jangan tambah library.
- Pesan error validasi menyebut nama env, bukan nilainya (untuk `SMTP_PASSWORD` sama sekali tidak disebut).
- `LogValue` menambah `mail_driver`, `smtp_host`, `smtp_port`, `smtp_tls`, `mail_from`, `smtp_password` (`***` bila terisi).
- Tes di `config_test.go`: default, tiap batas min/maks, `log`+production gagal, `smtp` tanpa host/from/public_url gagal, `MAIL_FROM` invalid gagal, password tersamar di `String()` dan `LogValue`.

### 4.3 Paket `infra/mailer` (baru, `backend/internal/infra/mailer`)

File:

- `mailer.go`
  ```go
  type Message struct{ To, Subject, Text, HTML string } // satu penerima, tanpa CC/BCC by design
  type Mailer interface {
      Send(ctx context.Context, m Message) error
      Configured() bool
  }
  var ErrNotConfigured = errors.New("mailer: tidak terkonfigurasi")
  func New(cfg config.Config, log *slog.Logger) (Mailer, error)
  ```
  `New` memilih driver dari `cfg.MailDriver`. `validate(m Message) error`: `To` lolos `mail.ParseAddress` dan tanpa CR/LF; `Subject` tanpa CR/LF (cegah header injection); minimal satu dari `Text`/`HTML`.
- `smtp.go`: stdlib `net/smtp` + `crypto/tls`.
  - `SMTP_TLS=starttls`: dial plain, `Client.StartTLS` WAJIB (gagal bila server tidak mendukung). `tls`: `tls.DialWithDialer` (TLS implisit, port 465). `none`: plain (Mailpit).
  - `ServerName` TLS = `SMTPHost`. `MinVersion: tls.VersionTLS12`.
  - `AUTH PLAIN` hanya bila `SMTPUsername` diisi (`smtp.PlainAuth`; stdlib menolak PLAIN di koneksi tanpa TLS kecuali localhost; untuk `none` dengan username di host non-localhost kembalikan error konfigurasi yang jelas).
  - Dial lewat `net.Dialer{Timeout: MailTimeout}`; `conn.SetDeadline(min(ctx deadline, now+MailTimeout))`. Satu percobaan, tanpa retry.
  - Body MIME `multipart/alternative` (`text/plain` + `text/html`, `charset=UTF-8`, `Content-Transfer-Encoding: quoted-printable` via `mime/quotedprintable`). Header: `From` (dari `MAIL_FROM`), `To`, `Subject` (dienkode `mime.QEncoding.Encode("utf-8", ...)`), `Date`, `Message-ID` (`<random@domain-from>`), `MIME-Version: 1.0`. Boundary acak dari `crypto/rand`.
  - Error dibungkus `fmt.Errorf("mailer: smtp %s: %w", tahap, err)` dengan tahap `dial|starttls|auth|mail|rcpt|data|quit`. TIDAK pernah memuat body, subject, atau password.
- `log.go`: driver `log` mencetak email utuh ke `slog` level info (`to`, `subject`, `text`). `Configured()=true`. Hanya dev/tes (dicegah di produksi oleh config).
- `noop.go`: `Send` -> `ErrNotConfigured`, `Configured()=false`.
- `mailertest/fake.go`: driver fake in-memory untuk tes domain.
  ```go
  type Fake struct{ Sent []mailer.Message; Err error; NotConfigured bool; mu sync.Mutex }
  func (f *Fake) Send(ctx context.Context, m mailer.Message) error // simpan bila Err==nil; NotConfigured -> ErrNotConfigured
  func (f *Fake) Configured() bool
  func (f *Fake) Last() (mailer.Message, bool)
  ```
- Tes: `mailer_test.go` (pemilihan driver, validasi header injection, noop), `smtp_test.go` (server SMTP palsu `net.Listen` di goroutine: sukses `none`, `AUTH` dikirim hanya bila username ada, error server 550 dibungkus tanpa body, timeout ctx, STARTTLS wajib gagal bila server tidak menawarkan), `log_test.go`.
- Aturan import: `infra/mailer` boleh meng-import `infra/config`; DILARANG `domain/**` (4.6). `mailertest` hanya meng-import `infra/mailer`.

### 4.4 `app.Deps` dan perakitan

1. `backend/internal/infra/app/deps.go` (file `Deps`): tambah field `Mailer mailer.Mailer` dengan komentar "Mailer generik; nil tidak pernah terjadi (driver noop bila tidak dikonfigurasi)".
2. `backend/cmd/internal/server/run.go` dan `domains.go`: panggil `mailer.New(cfg, log)` sebelum `app.Deps{...}`; kegagalan = start gagal (kembalikan error). Isi `Mailer:` di kedua literal. Bila `SMTP_TLS=none` dan produksi, `New` atau `run.go` me-log peringatan sekali.
3. Tes yang membangun `app.Deps` sendiri (helper tes module) diisi `&mailertest.Fake{}` atau `mailer.New` dengan config kosong (noop). Cari literal `app.Deps{` di repo dan perbarui yang perlu Mailer.
4. `rules.txt` baris 47 dan `docs/tickets/README.md` bagian "Konvensi arsitektur": `infra/{bus,db,http,middleware,config,static,logger,jobs,app,mailer}/`. Cek `internal/archtest` tetap hijau (tanpa allowlist).

### 4.5 Migrasi core

Lihat bagian 5. File dibuat saat implement dengan timestamp hasil generate.

### 4.6 `base/` core/auth

Folder `backend/internal/domain/core/internal/auth/base/`:

**a) `invitation.go` (murni, tanpa I/O)**

```go
const (
    DeliveryPending       = "pending"
    DeliverySent          = "sent"
    DeliveryFailed        = "failed"
    DeliveryNotConfigured = "not_configured"
)

type Invitation struct {
    UserID                string
    InvitedBy             string
    InvitedByLabel        *string
    IssuedAt, ExpiresAt   time.Time
    CreatedAt             time.Time
    SentAt, AcceptedAt    *time.Time
    ResendWindowStartedAt *time.Time
    DeliveryStatus        string
    ResendCount           int
    ResendWindowCount     int
}

type InvitationPolicy struct {
    TTL                  time.Duration
    MaxPerInviterPerHour int
    ResendMaxPerHour     int
    PurgeAfter           time.Duration
}

type InviteMode int
const (
    InviteCreateNew InviteMode = iota + 1
    InviteReissuePending
)

func NewInvitation(userID, invitedBy, label string, now time.Time, p InvitationPolicy) Invitation
func (i Invitation) IsPending() bool                 // AcceptedAt == nil
func (i Invitation) IsExpired(now time.Time) bool     // IsPending() && !now.Before(ExpiresAt)
func (i Invitation) BlocksLogin(now time.Time) bool   // IsExpired(now)
func (i *Invitation) Reissue(now time.Time, p InvitationPolicy) // IssuedAt=now, ExpiresAt=now+TTL, DeliveryStatus=pending
func (i Invitation) CheckResend(now time.Time, p InvitationPolicy) error // cek tanpa mengubah (dipakai K6 Deliver=false)
func (i *Invitation) RegisterResend(now time.Time, p InvitationPolicy) error // window 1 jam; lolos -> ResendCount++, ResendWindowCount++
func (i *Invitation) MarkDelivery(status string, now time.Time) // sent -> SentAt=now
func (i *Invitation) Accept(now time.Time)
func CheckInviteTarget(u *User, inv *Invitation) (InviteMode, error)
func CheckInviterQuota(createdLastHour int, oldestInWindow time.Time, now time.Time, p InvitationPolicy) error
func DefaultNameFromEmail(email string) string
```

Rule:
- `NewInvitation`: `IssuedAt=now`, `ExpiresAt=now+TTL`, `CreatedAt=now`, `DeliveryStatus=pending`, label kosong -> nil.
- Window resend (pola `LoginThrottle`): bila `ResendWindowStartedAt == nil` atau `now >= start+1h`, mulai window baru (`start=now`, `count=0`). Bila `count >= ResendMaxPerHour` -> `ErrInviteRateLimited(start+1h-now, "resend")`. Lolos -> `count++`, `ResendCount++`.
- `CheckInviteTarget`: `u == nil` -> `InviteCreateNew`. `inv != nil && inv.IsPending() && u.IsActive && u.Role == RoleDeveloper` -> `InviteReissuePending` (berlaku juga bila sudah kedaluwarsa, sehingga undang ulang memulihkan). Selain itu `ErrAlreadyRegistered(reason)` dengan urutan reason: `inactive` (u tidak aktif), `not_developer` (role super_admin), `registered` (tanpa undangan atau sudah diterima).
- `CheckInviterQuota`: `createdLastHour >= MaxPerInviterPerHour` -> `ErrInviteRateLimited(oldestInWindow+1h-now, "invite")` (retry minimal 1 detik).
- `DefaultNameFromEmail`: bagian lokal sebelum `@`, ganti `.`, `_`, `-`, `+` dengan spasi, trim, potong 100 rune; kosong -> `"Developer"`. Hasil selalu lolos `ValidateName`.

**b) `errors.go` (tambah)**

```go
const (
    ReasonInvitationNotPending = "INVITATION_NOT_PENDING"
    AlreadyRegisteredRegistered   = "registered"
    AlreadyRegisteredInactive     = "inactive"
    AlreadyRegisteredNotDeveloper = "not_developer"
)
var ErrInvitationNotPending = errs.WithDetails(errs.CodeConflict, "undangan sudah diterima atau tidak ada", map[string]any{"reason": ReasonInvitationNotPending})
func ErrAlreadyRegistered(reason string) *errs.Error // errs.CodeAlreadyRegistered, "email sudah terdaftar", details{"reason": reason}
func ErrInviteRateLimited(retry time.Duration, scope string) *errs.Error // errs.CodeRateLimited, details{"retry_after_seconds": ceil(retry), "scope": scope}
```

**c) `invitemail.go` (murni)**

```go
type InvitationEmailData struct {
    InviteeEmail, InviterName, ProjectName, LoginURL, TemporaryPassword string
    ExpiresAt time.Time
}
type EmailContent struct{ Subject, Text, HTML string }
func RenderInvitationEmail(d InvitationEmailData) (EmailContent, error)
```

- Bahasa Indonesia. Subjek: `Undangan bergabung ke Grimoire` atau `Undangan bergabung ke project <ProjectName> di Grimoire`.
- Isi: nama pengundang (fallback "Administrator Grimoire"), nama project bila ada, URL login, email, password sementara, tanggal kedaluwarsa format `2 Januari 2006 15:04 UTC` (nama bulan Indonesia lewat tabel lokal, bukan locale OS), peringatan "Segera ganti password setelah login. Password ini berlaku sampai ...".
- Text via `text/template`, HTML via `html/template` (escape otomatis). Template di-parse sekali (`template.Must` di var package).
- Tes: semua field muncul di Text dan HTML, project opsional, escape `<script>` di nama pengundang/project, format tanggal tepat, subjek tanpa CR/LF walau nama project mengandung newline (dibersihkan).

**d) `deps.go` (ubah `Shared`)**

Tambah `Invitation InvitationPolicy` dan `LoginURL string`. `auth/base.go` `buildShared`: `LoginURL = cfg.PublicURL + "/login"` bila `PublicURL != ""`, selain itu `"/login"`.

**e) `store/store.go` (tambah, SQL dipakai >1 feature, rule 5.3)**

```go
func (s *Store) FindInvitation(ctx context.Context, userID string) (base.Invitation, bool, error)
func (s *Store) LockInvitation(ctx context.Context, userID string) (base.Invitation, bool, error) // FOR UPDATE, dipakai di tx
func (s *Store) InsertInvitation(ctx context.Context, inv base.Invitation) error
func (s *Store) UpdateInvitation(ctx context.Context, inv base.Invitation) error // simpan semua kolom mutable
func (s *Store) CountInvitationsCreatedBy(ctx context.Context, inviterID string, since time.Time) (count int, oldest time.Time, err error)
func (s *Store) AcceptInvitation(ctx context.Context, userID string, now time.Time) (bool, error) // UPDATE ... SET accepted_at=$2 WHERE user_id=$1 AND accepted_at IS NULL
```

`invitationRow` dengan tag `db`, `toInvitation()`. Id bukan uuid -> `found=false` (pakai `canonicalUUID` yang ada). Tes integrasi di `store_integration_test.go` (tag `integration`).

**f) `delivery/delivery.go` (paket kecil ber-I/O, pola `base/audit`)**

```go
// Deliver mengirim m lewat ml dan mengembalikan status pengiriman.
// Tidak pernah me-log body, subject, atau password; hanya domain penerima dan kelas error.
func Deliver(ctx context.Context, ml mailer.Mailer, m mailer.Message, log *slog.Logger) string
```

- `nil` -> `DeliverySent`. `errors.Is(err, mailer.ErrNotConfigured)` -> `DeliveryNotConfigured` (log info). Lainnya -> `DeliveryFailed` (log error `"invitation email failed"`, `to_domain`, `err` yang sudah aman dari mailer).
- Timeout diserahkan ke driver; `Deliver` tidak membuat ctx baru, tetapi pemanggil memakai `context.WithoutCancel(ctx)` + timeout `MailTimeout` agar klien yang memutus koneksi setelah commit tidak membatalkan kirim.
- Tes dengan `mailertest.Fake`.

**g) Pemetaan `EmailContent -> mailer.Message`**: diletakkan di `base/delivery` sebagai `delivery.Message(to string, c base.EmailContent) mailer.Message`, bukan di `base/*.go`. Dengan begitu file `base/*.go` tetap murni dan tidak meng-import `infra/mailer`; hanya subpaket `base/delivery` yang menyentuh infra (pola `base/audit` -> `infra/bus`).

**h) Kredensial di respons HTTP**: handler memakai DTO `string` biasa untuk `temporary_password`. `base.Secret` ter-marshal `"***"`, jadi JANGAN dipakai di DTO. Di dalam usecase password disimpan sebagai `base.Secret` sampai dimasukkan ke DTO/bus response.

### 4.7 Kontrak bus (`backend/lib/constants/bus.go` dan `backend/lib/contracts`)

`lib/constants/bus.go` (satu komentar Provider/Consumer per baris, pola yang ada):

```go
// Provider: core/auth inviteuser. Consumer: project/management inviteuser.
const BUS_CORE_AUTH_INVITE_USER = "core.auth.invite-user"
// Provider: core/auth resendinvitation. Consumer: project/management inviteuser, resendinvitation.
const BUS_CORE_AUTH_RESEND_INVITATION = "core.auth.resend-invitation"
// Provider: project/management countmemberships. Consumer: core/auth purgeinvitations.
const BUS_PROJECT_MANAGEMENT_COUNT_MEMBERSHIPS = "project.management.count-memberships"
```

`lib/contracts/core/auth/invitation/invitation.go` (baru):

```go
// Credentials adalah kredensial sementara fallback. JANGAN di-log, diaudit, atau
// diterbitkan lewat Publish; hanya diteruskan ke respons HTTP pengundang.
type Credentials struct{ Email, TemporaryPassword, LoginURL string }

const (
    EmailStatusSent          = "sent"
    EmailStatusFailed        = "failed"
    EmailStatusNotConfigured = "not_configured"
    EmailStatusPending       = "pending" // hanya Deliver=false
)
```

`lib/contracts/core/auth/inviteuser/inviteuser.go` (baru):

```go
type Request struct {
    Email, Name               string
    InvitedBy, InvitedByLabel string
    ProjectName               string
    Deliver                   bool // false: hanya siapkan (dipakai management)
}
type Response struct {
    UserID      string
    Created     bool
    ExpiresAt   time.Time
    EmailSent   bool
    EmailStatus string                  // invitation.EmailStatus*; "pending" bila Deliver=false
    Credentials *invitation.Credentials // hanya bila Deliver && !EmailSent
}
```
Error: `INVALID_ARGUMENT` (details.fields), `ALREADY_REGISTERED` (details.reason), `RATE_LIMITED` (details.retry_after_seconds, scope).

`lib/contracts/core/auth/resendinvitation/resendinvitation.go` (baru):

```go
type Request struct {
    UserID                    string
    InvitedBy, InvitedByLabel string
    ProjectName               string
    Initial                   bool // true: penerbitan pertama dari management; tanpa kuota & audit resent
}
type Response struct {
    EmailSent   bool
    EmailStatus string
    ExpiresAt   time.Time
    Credentials *invitation.Credentials
}
```
Error: `NOT_FOUND`, `CONFLICT` reason `INVITATION_NOT_PENDING`, `ALREADY_REGISTERED` reason `inactive`, `RATE_LIMITED` scope `resend`.

`lib/contracts/core/auth/getuser` (ADITIF):

```go
type Request struct {
    UserID string
    Email  string // dipakai bila UserID kosong; dinormalisasi lowercase+trim oleh provider
}
type Response struct {
    // ... field lama tetap
    Invitation *Invitation // nil bila tidak ada undangan menunggu
}
type Invitation struct {
    InvitedAt, IssuedAt, ExpiresAt time.Time
    SentAt                         *time.Time
    Expired                        bool
    DeliveryStatus                 string
    ResendCount                    int
}
```

`lib/contracts/core/auth/finddevelopers` (ADITIF): `Developer.InvitationPending bool`.

`lib/contracts/project/management/countmemberships/countmemberships.go` (baru):

```go
const MaxUserIDs = 500
type Request struct{ UserIDs []string }
// Counts: user tanpa keanggotaan absen (dibaca sebagai 0). Id bukan uuid diabaikan.
type Response struct{ Counts map[string]int }
```

Doc comment tiap paket kontrak menyebut pola consumer (`bus.CallTyped[...]`) seperti `checkaccess`.

### 4.8 Aksi audit

`backend/lib/contracts/core/audit/record/actions.go` + `Actions()` + `actions_test.go`:

| Konstanta | Nilai | Dicatat oleh | Args (tanpa rahasia) |
|---|---|---|---|
| `ActionUserInvited` | `user.invited` | auth `inviteuser` | `email`, `created`, `via` (`users`/`project`), `project_name` (bila ada) |
| `ActionUserInvitationResent` | `user.invitation.resent` | auth `resendinvitation`, `inviteuser` (K6 jalur Users) | `email`, `via`, `project_name`, `resend_count` |
| `ActionUserInvitationAccepted` | `user.invitation.accepted` | auth `changepassword` | `email` |
| `ActionUserInvitationEmailFailed` | `user.invitation.email_failed` | auth `inviteuser`, `resendinvitation` | `email`, `delivery_status` (`failed`/`not_configured`), `error_class` (tahap smtp) ; status audit `error` |
| `ActionProjectMemberInvited` | `project.member.invited` | management `inviteuser` | `user_id`, `user_label`, `role`, `created` |
| `ActionProjectInvitationRevoked` | `project.invitation.revoked` | management `revokeinvitation` | `user_id`, `user_label`, `role` |

Aturan: Args TIDAK PERNAH memuat password, hash, atau credentials. Jangan pakai kunci `temporary_password` sama sekali (walau `sanitize` membuangnya). Target audit = user yang diundang (`target_type=user`) untuk aksi `user.*`, project untuk aksi `project.*`. Katalog di `docs/tickets/README.md` diperbarui.

### 4.9 Wiring module

- `core/internal/auth/base.go`: `buildShared` mengisi `Invitation` dan `LoginURL`; `Register` memanggil `inviteuser.Init`, `resendinvitation.Init`, `purgeinvitations.Init` (ditambah oleh plan usecase masing-masing; foundation hanya menyiapkan `Shared`).
- `project/internal/management/base.go`: `countmemberships.Init`, `inviteuser.Init`, `listinvitations.Init`, `resendinvitation.Init`, `revokeinvitation.Init` (oleh plan usecase).
- `project/internal/management/base/users/users.go`: method baru `GetByEmail`, `Invite`, `ResendInvitation` (wrapper bus, bungkus error dengan `%w`). Ditulis di plan `management-inviteuser`; `management-resendinvitation` dan `management-revokeinvitation` memakainya.
- `project/internal/management/base/errors.go`: `ReasonInvitationNotPending`, `ErrInvitationNotPending` (409 CONFLICT). Ditulis di plan `management-listinvitations` (yang pertama dikerjakan di management, lihat bagian 9).

### 4.10 Lain-lain

- `docker-compose.yml`: service `mailpit` (`image: axllent/mailpit:v1.21.8`, port `1025:1025` SMTP dan `8025:8025` UI, `restart: unless-stopped`). Service `grimoire`: `MAIL_DRIVER=smtp`, `SMTP_HOST=mailpit`, `SMTP_PORT=1025`, `SMTP_TLS=none`, `MAIL_FROM="Grimoire <no-reply@grimoire.local>"`, `PUBLIC_URL=http://localhost:8080`, `depends_on: mailpit`.
- `docker-compose.dev.yml`: service `mailpit` yang sama (backend dev jalan di host, memakai `localhost:1025`).
- `.env.example`: `MAIL_DRIVER=smtp`, `SMTP_HOST=localhost`, `SMTP_PORT=1025`, `SMTP_TLS=none`, `MAIL_FROM=Grimoire <no-reply@grimoire.local>`, `PUBLIC_URL=http://localhost:5173` (atau port dev yang dipakai repo), plus semua env baru dengan default dan komentar.
- `docs/configuration.md`: baris tiap env baru; catatan `PUBLIC_URL` wajib untuk SMTP; `log` dilarang di produksi; contoh SMTP penyedia umum (STARTTLS 587, TLS 465).
- `docs/infra/running.md`: cara membuka Mailpit (`http://localhost:8025`), cara menjalankan tanpa SMTP (fallback kredensial).
- `docs/tickets/T01-core-auth.md` dan `T03-project-management.md`: tambah bagian "Catatan revisi (T12)" di akhir file, tanpa menulis ulang isi.
- `docs/tickets/README.md`: katalog bus (3 baris baru + catatan aditif get-user/find-developers), katalog audit, status T12.
- `api/modules/core/auth.yml` + `api/modules/project/management.yml`: path baru, skema `Credentials`, `InvitationStatus`, perubahan aditif `User`, `Member`, `Candidate`, `Me`. Komponen `Error` dirujuk dari `api/modules/infra/foundation.yml`. Path baru ditambahkan ke `api/root.yml`. Lalu `make api-bundle` dan commit `api/openapi.yml`. Bagian OpenAPI ditulis oleh plan usecase masing-masing; foundation hanya menambah skema bersama `InvitationCredentials` dan `InvitationStatus` di `auth.yml`.

---

## 5. Migration

Satu file baru di `backend/migrations/core/`: `<VERSION>_create_user_invitations.sql`.
`<VERSION>` di-generate saat implement (lihat `.claude/rules/migrations.md`):

```
python3 -c "import datetime;print(datetime.datetime.now().strftime('%Y%m%d%H%M%S%f')[:17])"
```

Versi harus lebih besar dari `20261021100000000_create_auth_tables.sql` (FK ke `users`); otomatis terpenuhi. Cek duplikat dengan `ls backend/migrations/*/ | cut -c1-17 | sort | uniq -d`.

```sql
CREATE TABLE IF NOT EXISTS user_invitations (
    user_id                  uuid        PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    invited_by               uuid        NOT NULL,          -- tanpa FK (pola created_by T01)
    invited_by_label         text        NULL,
    issued_at                timestamptz NOT NULL,          -- terakhir password sementara diterbitkan
    expires_at               timestamptz NOT NULL,
    sent_at                  timestamptz NULL,              -- terakhir BERHASIL terkirim
    delivery_status          text        NOT NULL DEFAULT 'pending'
        CHECK (delivery_status IN ('pending', 'sent', 'failed', 'not_configured')),
    accepted_at              timestamptz NULL,
    resend_count             int         NOT NULL DEFAULT 0 CHECK (resend_count >= 0),
    resend_window_started_at timestamptz NULL,
    resend_window_count      int         NOT NULL DEFAULT 0 CHECK (resend_window_count >= 0),
    created_at               timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS user_invitations_pending_expires_idx
    ON user_invitations (expires_at) WHERE accepted_at IS NULL;

CREATE INDEX IF NOT EXISTS user_invitations_inviter_created_idx
    ON user_invitations (invited_by, created_at);
```

- Tidak ada kolom token. Tidak ada perubahan pada `users`. Tidak ada migrasi project.
- "Menunggu" = baris ada dan `accepted_at IS NULL`. Akun `createuser`/`create-admin` tidak punya baris.
- `FK user_id -> users ON DELETE CASCADE` sah karena tabel satu module.
- Ikuti format file migrasi yang ada (cek apakah runner memakai penanda `-- +up`/`-- +down` atau satu blok; samakan dengan `20261021100000000_create_auth_tables.sql`).

---

## 6. Changes (daftar perubahan foundation per file/folder)

| File / folder | Aksi | Isi |
|---|---|---|
| `backend/lib/common/errs/errs.go` | ubah | `CodeAlreadyRegistered` |
| `backend/internal/infra/http/respond/status.go` (+ tes) | ubah | peta 409 untuk `ALREADY_REGISTERED` |
| `backend/internal/infra/config/config.go`, `config_test.go` | ubah | field mail/invitation + validasi + masking |
| `backend/internal/infra/mailer/{mailer,smtp,log,noop}.go` + tes | baru | paket mailer |
| `backend/internal/infra/mailer/mailertest/fake.go` | baru | fake untuk tes |
| `backend/internal/infra/app/<file Deps>.go` | ubah | field `Mailer` |
| `backend/cmd/internal/server/run.go`, `domains.go` (+ `run_test.go`/`domains_test.go` bila membangun Deps) | ubah | rakit `mailer.New` |
| `backend/migrations/core/<VERSION>_create_user_invitations.sql` | baru | tabel `user_invitations` |
| `backend/internal/domain/core/internal/auth/base/invitation.go`, `invitation_test.go` | baru | entity + rule |
| `backend/internal/domain/core/internal/auth/base/invitemail.go`, `invitemail_test.go` | baru | template email |
| `backend/internal/domain/core/internal/auth/base/errors.go`, `errors_test.go` | ubah | error undangan |
| `backend/internal/domain/core/internal/auth/base/deps.go` | ubah | `Shared.Invitation`, `Shared.LoginURL` |
| `backend/internal/domain/core/internal/auth/base.go`, `base_test.go` | ubah | `buildShared` mengisi kebijakan + login URL |
| `backend/internal/domain/core/internal/auth/base/store/store.go`, `store_integration_test.go` | ubah | SQL undangan bersama |
| `backend/internal/domain/core/internal/auth/base/delivery/delivery.go`, `delivery_test.go` | baru | helper kirim |
| `backend/lib/constants/bus.go` | ubah | 3 konstanta |
| `backend/lib/contracts/core/auth/invitation/` | baru | `Credentials`, status |
| `backend/lib/contracts/core/auth/inviteuser/` | baru | kontrak |
| `backend/lib/contracts/core/auth/resendinvitation/` | baru | kontrak |
| `backend/lib/contracts/core/auth/getuser/` | ubah (aditif) | `Email`, `Invitation` |
| `backend/lib/contracts/core/auth/finddevelopers/` | ubah (aditif) | `InvitationPending` |
| `backend/lib/contracts/project/management/countmemberships/` | baru | kontrak |
| `backend/lib/contracts/core/audit/record/actions.go`, `actions_test.go` | ubah | 6 aksi |
| `rules.txt` | ubah | `mailer` di daftar infra |
| `docs/tickets/README.md` | ubah | konvensi infra, katalog bus, katalog audit, status T12 |
| `docs/tickets/T01-core-auth.md`, `T03-project-management.md` | ubah | catatan revisi |
| `docker-compose.yml`, `docker-compose.dev.yml` | ubah | Mailpit + env grimoire |
| `.env.example`, `docs/configuration.md`, `docs/infra/running.md` | ubah | dokumentasi env |
| `api/modules/core/auth.yml`, `api/openapi.yml` | ubah | skema bersama undangan (bundle) |

Perubahan per usecase (folder feature, handler, route, OpenAPI path) ada di plan usecase masing-masing.

---

## 7. Test Scenario (foundation)

Unit:

| Area | Skenario |
|---|---|
| config | default semua field; `MAIL_DRIVER=log` + `APP_ENV=production` gagal; `log` + development lolos; `smtp` tanpa `SMTP_HOST`/`MAIL_FROM`/`PUBLIC_URL` gagal (pesan menyebut nama env); `MAIL_FROM=bukan-email` gagal; `SMTP_PORT=0`/`70000` gagal; `SMTP_TLS=foo` gagal; `MAIL_TIMEOUT=61s` gagal; `INVITATION_TTL=30m` dan `721h` gagal; `SMTP_PASSWORD=rahasia` tidak muncul di `String()`, `LogValue`, dan pesan error manapun |
| mailer | `New` memilih `noop` (driver kosong), `log`, `smtp`; noop `Configured()=false` dan `Send` = `ErrNotConfigured`; header injection (`To` atau `Subject` berisi `\r\n`) ditolak sebelum dial; SMTP palsu `none` menerima satu pesan multipart dengan dua bagian UTF-8 QP; `AUTH` tidak dikirim bila username kosong; balasan 550 di RCPT -> error ber-tahap `rcpt` tanpa body/subject; ctx deadline lewat -> error dalam batas waktu; `starttls` ke server tanpa STARTTLS -> error |
| errs/respond | `StatusFor(CodeAlreadyRegistered)=409` |
| base/invitation | `IsExpired` tepat di `expires_at` (true) dan 1ns sebelum (false); `Accept` -> tidak pending, tidak expired, tidak blok login; `Reissue` memperpanjang dan reset status; `RegisterResend` ke-1..3 lolos, ke-4 dalam jam yang sama 429 dengan `retry_after_seconds` benar dan `scope=resend`; window bergeser setelah 1 jam; `CheckResend` tidak mengubah state; `CheckInviteTarget` tabel (nil user, pending aktif developer, pending expired, diterima, tanpa undangan, nonaktif, super_admin); `CheckInviterQuota` batas 19/20; `DefaultNameFromEmail` (`budi.santoso@x` -> `budi santoso`, lokal kosong -> `Developer`, 300 rune -> 100 rune, hasil lolos `ValidateName`) |
| base/invitemail | semua field ada; project opsional; escape HTML; tanggal Indonesia; subjek tanpa newline |
| base/delivery | fake sukses -> `sent`; `ErrNotConfigured` -> `not_configured`; error lain -> `failed`; log tidak memuat password/subject (cek buffer handler slog) |
| audit actions | `Actions()` memuat 6 aksi baru, unik, format `<area>.<objek>.<aksi>` |
| buildShared | `PublicURL="https://g.x"` -> `LoginURL="https://g.x/login"`; kosong -> `/login`; kebijakan dari config |

Integrasi (`-tags integration`, `db.TestDB`):

| Area | Skenario |
|---|---|
| migrasi | tabel dan dua index ada; `CHECK delivery_status` menolak nilai lain; hapus user meng-cascade baris undangan |
| store | `InsertInvitation` lalu `FindInvitation` round-trip semua kolom; `LockInvitation` di dua tx bersamaan menahan tx kedua; `UpdateInvitation` menyimpan kolom mutable; `CountInvitationsCreatedBy` hanya menghitung milik pengundang sejak `since`; `AcceptInvitation` true sekali lalu false; id bukan uuid -> not found |
| server | `run` start dengan `MAIL_DRIVER` kosong (noop) sukses; config `log` + production gagal start |
| archtest | `internal/archtest` hijau dengan paket `infra/mailer` baru |

---

## 8. E2E Scenarios (lintas usecase, dirinci di plan usecase)

Positif:
1. (K1) Owner mengundang `baru@contoh.id` -> 201, `email_sent=true`, tanpa field `credentials`; Mailpit menerima satu email berisi URL login dan password sementara; DB hanya berisi hash; log dan audit tanpa password. [`management-inviteuser`]
2. Penerima login dengan password sementara -> dipaksa ganti password -> `getme` normal, project muncul dengan role undangan; `user.invitation.accepted` tercatat. [`auth-login-changepassword`]
3. Undangan kedaluwarsa (jam dimajukan / `expires_at` di-set lampau) -> login ditolak identik kredensial salah; kirim ulang -> password baru berlaku, password lama ditolak. [`auth-resendinvitation`, `management-resendinvitation`]
4. Mengundang email yang masih menunggu -> tidak ada akun ganda, password baru, email kedua di Mailpit. [`management-inviteuser`]
5. Super Admin mengundang dari halaman Users -> akun tanpa project, email terkirim. [`auth-inviteuser`]
6. Owner melihat daftar undangan, membatalkan -> keanggotaan hilang, akun tetap; purge menghapus akun kedaluwarsa > 30 hari tanpa keanggotaan. [`management-listinvitations`, `management-revokeinvitation`, `auth-purgeinvitations`]
7. Dev lokal `docker compose up` -> Mailpit di `:8025` menampilkan email undangan.

Negatif:
1. Email sudah terdaftar (diterima / `createuser`) -> 409 `ALREADY_REGISTERED`; email sudah anggota project -> 409 `CONFLICT` reason `ALREADY_MEMBER`.
2. Member (bukan owner) -> 403 di keempat endpoint undangan project; non-anggota -> 404.
3. Developer memanggil `POST /api/users/invite` -> 403.
4. Kirim ulang ke-4 dalam satu jam -> 429 `RATE_LIMITED` scope `resend`; undangan ke-21 dalam satu jam oleh pengundang yang sama -> 429 scope `invite`.
5. SMTP mati (Mailpit dihentikan) atau `MAIL_DRIVER` kosong -> 201 `email_sent=false`, `credentials` ada, `Cache-Control: no-store`; tidak ada endpoint yang mengembalikan kredensial itu lagi; audit `user.invitation.email_failed` tanpa password.
6. Aplikasi start tanpa konfigurasi email; `MAIL_DRIVER=log` + `APP_ENV=production` gagal start.
7. Batalkan undangan yang sudah diterima -> 409 `INVITATION_NOT_PENDING`.

---

## 9. Urutan implementasi

1. Foundation 4.1 (kode error) -> 4.2 (config) -> 4.3 (mailer) -> 4.4 (Deps + perakitan + rules.txt) -> 4.5 (migrasi) -> 4.6 (base auth) -> 4.7 (kontrak) -> 4.8 (audit) -> 4.10 (compose, docs, OpenAPI skema bersama). Build + test hijau.
2. Gelombang A (paralel, saling bebas):
   - `auth-invitation-markers` (getuserbus Email + Invitation, finddevelopers, listusers/getuser, getme `email_enabled`)
   - `management-countmemberships`
   - `auth-login-changepassword`
3. Gelombang B (paralel):
   - `auth-resendinvitation`
   - `auth-inviteuser` (memakai helper `base/delivery` dan `base/invitemail`, tidak bergantung pada feature resend)
   - `auth-purgeinvitations` (butuh `countmemberships` terdaftar untuk tes integrasi lintas module; unit memakai fake bus)
   - `management-listinvitations` (juga perubahan base management: `MemberDTO`, `CandidateDTO`, `ErrInvitationNotPending`; butuh `getuser` aditif)
4. Gelombang C (paralel, butuh B):
   - `management-inviteuser` (menulis `base/users` `GetByEmail`/`Invite`/`ResendInvitation`; butuh kedua provider auth)
   - `management-resendinvitation` dan `management-revokeinvitation` (memakai `base/users` dari `management-inviteuser`; bila dikerjakan paralel, `management-inviteuser` menulis `base/users` lebih dulu sebagai commit terpisah)
5. Frontend (`frontend-plan.md`): bisa mulai setelah kontrak OpenAPI final (akhir gelombang B untuk auth, C untuk management).
6. Penutup: `make api-bundle`, `make api-check`, lint, arch test, unit, integrasi, e2e; perbarui status T12 di README.

---

## 10. Daftar usecase

| Slug | Folder | Titik masuk | Plan |
|---|---|---|---|
| `auth-inviteuser` | `backend/internal/domain/core/internal/auth/inviteuser` | `POST /api/users/invite`, bus `core.auth.invite-user` | `usecase-auth-inviteuser.md` |
| `auth-resendinvitation` | `backend/internal/domain/core/internal/auth/resendinvitation` | `POST /api/users/{id}/resend-invitation`, bus `core.auth.resend-invitation` | `usecase-auth-resendinvitation.md` |
| `auth-purgeinvitations` | `backend/internal/domain/core/internal/auth/purgeinvitations` | job `core.auth.purge-invitations` 03:30 UTC | `usecase-auth-purgeinvitations.md` |
| `auth-login-changepassword` | `.../auth/login`, `.../auth/changepassword` | perilaku login/ganti password | `usecase-auth-login-changepassword.md` |
| `auth-invitation-markers` | `.../auth/{getuserbus,finddevelopers,listusers,getuser,getme}` | penanda aditif | `usecase-auth-invitation-markers.md` |
| `management-inviteuser` | `backend/internal/domain/project/internal/management/inviteuser` | `POST /api/projects/{projectID}/invitations` | `usecase-management-inviteuser.md` |
| `management-listinvitations` | `.../management/listinvitations` (+ base `MemberDTO`/`CandidateDTO`) | `GET /api/projects/{projectID}/invitations` | `usecase-management-listinvitations.md` |
| `management-resendinvitation` | `.../management/resendinvitation` | `POST /api/projects/{projectID}/invitations/{userID}/resend` | `usecase-management-resendinvitation.md` |
| `management-revokeinvitation` | `.../management/revokeinvitation` | `DELETE /api/projects/{projectID}/invitations/{userID}` | `usecase-management-revokeinvitation.md` |
| `management-countmemberships` | `.../management/countmemberships` | bus `project.management.count-memberships` | `usecase-management-countmemberships.md` |
| (frontend) | `frontend/src/domain/core/auth/users`, `frontend/src/domain/project/management/members`, `frontend/src/shared/components/TemporaryCredentialsDialog.svelte`, login/changepassword, `shared/i18n` | UI | `frontend-plan.md` (wajib bagian "Visual" sesuai `DESIGN.md`) |

---

## 11. Request/Response body (bentuk bersama)

Semua contoh memakai snake_case. Error memakai envelope `Error` dari `api/modules/infra/foundation.yml`.

### 11.1 `POST /api/users/invite`

Request:
```json
{ "email": "Baru@Contoh.id ", "name": "" }
```

201, email terkirim (tanpa `credentials` sama sekali):
```json
{
  "user": {
    "id": "7c1f3c2e-5b7a-4d7e-9a51-1f0b8a3e6d21",
    "email": "baru@contoh.id",
    "name": "baru",
    "role": "developer",
    "is_active": true,
    "must_change_password": true,
    "created_at": "2026-10-21T08:15:00Z",
    "invitation": {
      "pending": true,
      "invited_at": "2026-10-21T08:15:00Z",
      "sent_at": "2026-10-21T08:15:01Z",
      "expires_at": "2026-10-28T08:15:00Z",
      "expired": false,
      "delivery_status": "sent",
      "resend_count": 0
    }
  },
  "created": true,
  "email_sent": true,
  "email_status": "sent",
  "expires_at": "2026-10-28T08:15:00Z"
}
```

201, fallback (header `Cache-Control: no-store`):
```json
{
  "user": { "id": "7c1f3c2e-5b7a-4d7e-9a51-1f0b8a3e6d21", "email": "baru@contoh.id", "name": "baru", "role": "developer", "is_active": true, "must_change_password": true, "created_at": "2026-10-21T08:15:00Z",
    "invitation": { "pending": true, "invited_at": "2026-10-21T08:15:00Z", "sent_at": null, "expires_at": "2026-10-28T08:15:00Z", "expired": false, "delivery_status": "not_configured", "resend_count": 0 } },
  "created": true,
  "email_sent": false,
  "email_status": "not_configured",
  "expires_at": "2026-10-28T08:15:00Z",
  "credentials": {
    "email": "baru@contoh.id",
    "temporary_password": "q7XkR2mVtz9HbNcP",
    "login_url": "https://grimoire.contoh.id/login"
  }
}
```

### 11.2 `POST /api/projects/{projectID}/invitations`

Request:
```json
{ "email": "baru@contoh.id", "role": "member", "name": "Budi" }
```

201:
```json
{
  "member": {
    "user_id": "7c1f3c2e-5b7a-4d7e-9a51-1f0b8a3e6d21",
    "name": "Budi",
    "email": "baru@contoh.id",
    "role": "member",
    "is_active": true,
    "added_at": "2026-10-21T08:15:00Z",
    "added_by_label": "Owner Satu",
    "invitation_pending": true,
    "invitation_expires_at": "2026-10-28T08:15:00Z"
  },
  "created": true,
  "email_sent": true,
  "email_status": "sent",
  "expires_at": "2026-10-28T08:15:00Z"
}
```
Langkah terbit gagal total: sama tetapi `"email_sent": false, "email_status": "failed"` dan tanpa `credentials`.
Bentuk persis `member` mengikuti `MemberDTO` T03 yang ada; field baru hanya `invitation_pending` dan `invitation_expires_at`.

### 11.3 `GET /api/projects/{projectID}/invitations`

200:
```json
{
  "items": [
    {
      "user_id": "7c1f3c2e-5b7a-4d7e-9a51-1f0b8a3e6d21",
      "email": "baru@contoh.id",
      "name": "Budi",
      "role": "member",
      "invited_by_label": "Owner Satu",
      "added_at": "2026-10-21T08:15:00Z",
      "sent_at": null,
      "expires_at": "2026-10-28T08:15:00Z",
      "expired": false,
      "delivery_status": "failed",
      "email_failed": true,
      "resend_count": 1
    }
  ]
}
```

### 11.4 Resend (`POST /api/users/{id}/resend-invitation`, `POST /api/projects/{projectID}/invitations/{userID}/resend`)

Tanpa body. 200 sukses:
```json
{ "email_sent": true, "email_status": "sent", "expires_at": "2026-10-28T09:00:00Z" }
```
200 fallback (`Cache-Control: no-store`):
```json
{
  "email_sent": false,
  "email_status": "failed",
  "expires_at": "2026-10-28T09:00:00Z",
  "credentials": { "email": "baru@contoh.id", "temporary_password": "Zp4wNe8TqYv3KdRh", "login_url": "https://grimoire.contoh.id/login" }
}
```

### 11.5 `DELETE /api/projects/{projectID}/invitations/{userID}`

204 tanpa body.

### 11.6 `GET /api/auth/me` (aditif)

```json
{ "id": "...", "email": "owner@contoh.id", "name": "Owner Satu", "role": "developer", "must_change_password": false, "email_enabled": false }
```

### 11.7 Error

400:
```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "permintaan tidak valid", "details": { "fields": { "email": "format email tidak valid" } } } }
```
409 sudah terdaftar:
```json
{ "error": { "code": "ALREADY_REGISTERED", "message": "email sudah terdaftar", "details": { "reason": "registered" } } }
```
409 sudah anggota:
```json
{ "error": { "code": "CONFLICT", "message": "user sudah menjadi anggota project", "details": { "reason": "ALREADY_MEMBER" } } }
```
409 undangan tidak menunggu:
```json
{ "error": { "code": "CONFLICT", "message": "undangan sudah diterima atau tidak ada", "details": { "reason": "INVITATION_NOT_PENDING" } } }
```
429:
```json
{ "error": { "code": "RATE_LIMITED", "message": "terlalu banyak permintaan undangan", "details": { "retry_after_seconds": 1840, "scope": "resend" } } }
```
403:
```json
{ "error": { "code": "FORBIDDEN", "message": "akses ditolak" } }
```
404:
```json
{ "error": { "code": "NOT_FOUND", "message": "project tidak ditemukan" } }
```
Bentuk envelope (`error.code`, `error.message`, `error.details`, dan `trace_id` bila ada) mengikuti `httpx.WriteError` yang ada; contoh di atas menunjukkan isi, bukan menambah field baru.

---

## 12. Risiko dan catatan

- Race purge: member ditambahkan di antara cek `count-memberships` dan `DELETE` (tanpa FK). Peluang sangat kecil (akun harus menunggu, kedaluwarsa > 30 hari, lalu diundang ke project tepat saat job jalan). Diterima dan didokumentasikan di plan purge. `DELETE` mengecek ulang kondisi menunggu + batas waktu di SQL.
- Dua hash argon2 di jalur project (langkah 1 dan 3). Diterima.
- Langkah 3 gagal total meninggalkan undangan `pending` tanpa password yang diketahui; dipulihkan lewat Kirim ulang.
- Perubahan `login`/`changepassword` menyentuh jalur kritis; tes regresi T01 wajib tetap hijau.
- Semua K-pembangun di tiket (K3, K5/K6, K13) masih menunggu persetujuan pemilik; plan ini mengikutinya apa adanya.
