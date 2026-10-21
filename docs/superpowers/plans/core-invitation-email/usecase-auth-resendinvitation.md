# T12 Usecase Plan: auth-resendinvitation

Tiket: `docs/tickets/T12-core-invitation-email.md`. Foundation: `docs/superpowers/plans/core-invitation-email/overview.md` (menang bila bertentangan; bagian yang dirujuk ditulis "overview 4.6" dst).

Prasyarat: foundation overview 4.1 sampai 4.10 selesai dan hijau (`user_invitations`, `base/invitation.go`, `base/invitemail.go`, `base/delivery`, `base/store` undangan, kontrak bus, aksi audit, `app.Deps.Mailer`).

---

## 1. Metadata

| Atribut | Nilai |
|---|---|
| Ticket | T12 |
| Feature | core-invitation-email |
| Usecase slug | `auth-resendinvitation` |
| Scope | backend |
| Domain / Module | core / auth |
| Folder | `backend/internal/domain/core/internal/auth/resendinvitation` |
| Endpoint HTTP | `/api/users/{id}/resend-invitation` |
| Method | POST (tanpa body) |
| Version | Tanpa prefix versi di URL. `info.version` `api/modules/core/auth.yml` naik minor (satu kali untuk seluruh T12, dikerjakan foundation/plan lain; plan ini hanya menambah path) |
| Auth HTTP | `RequireAuth` + `RequireRole(super_admin)`; param `{id}` mengikuti route user T01 |
| Endpoint bus | `core.auth.resend-invitation` (Call), kontrak `lib/contracts/core/auth/resendinvitation` |
| Consumer bus | project/management `inviteuser` (Initial=true) dan `resendinvitation` (Initial=false) |
| Konfigurasi dipakai | `INVITATION_TTL`, `INVITATION_RESEND_MAX_PER_HOUR`, `MAIL_TIMEOUT`, `PUBLIC_URL` (lewat `Shared.LoginURL`), `Deps.Mailer` |

Dua titik masuk, satu `Usecase`. Handler HTTP dan handler bus hanya menerjemahkan masukan ke `Input`; seluruh logika ada di `Usecase.Execute`.

---

## 2. Summary Plan

Usecase menerbitkan password sementara baru untuk akun yang undangannya masih menunggu, memperpanjang masa berlaku, mencabut semua sesi akun itu, lalu mengirim email undangan setelah transaksi commit. Dipakai dengan dua mode:

- `Initial=false` (kirim ulang): menghitung kuota kirim ulang (default 3 per jam per undangan, 429 `RATE_LIMITED` scope `resend`), mencatat audit `user.invitation.resent`.
- `Initial=true` (penerbitan pertama dari management setelah `inviteuser` Deliver=false dan keanggotaan tercatat): tanpa kuota, tanpa audit `resent`.

Urutan logika:

1. User ada (404 `NOT_FOUND`).
2. Undangan menunggu (409 `CONFLICT` reason `INVITATION_NOT_PENDING`): baris ada dan `accepted_at IS NULL`. Berlaku juga untuk undangan yang sudah kedaluwarsa (inilah jalur pemulihan).
3. User aktif (409 `ALREADY_REGISTERED` reason `inactive`).
4. Tx: `LockInvitation` -> terapkan rule entity (cek target, `RegisterResend` bila `!Initial`, `Reissue`) -> `UpdatePassword(must_change=true)` -> `DeleteSessionsByUser(semua)` -> `UpdateInvitation`.
5. Setelah commit: render email, kirim lewat `base/delivery`, update `delivery_status` dan `sent_at` (tx kecil), audit, bentuk respons. `credentials` HANYA bila tidak terkirim.

Keputusan desain:

- **Hash dihitung sebelum tx.** Overview menulis urutan "generate + hash" di dalam tx. Argon2id memakan puluhan sampai ratusan ms, dan menahan row-lock selama itu tidak perlu. Efek semantik identik. Konsekuensi: satu hash terbuang bila 404/409/429; diterima karena endpoint hanya super_admin atau internal.
- **Rule di entity, SQL di repository.** Repository mengeksekusi tx dan menerima closure `Apply` dari usecase yang memanggil method entity (`CheckResendTarget`, `RegisterResend`, `Reissue`). Usecase tidak memuat if-rule sendiri.
- **Urutan lock.** `LockInvitation` selalu sebelum `UpdatePassword` (yang meng-UPDATE `users`). `auth-login-changepassword` WAJIB memakai urutan yang sama (`LockInvitation` lalu `UpdatePassword` lalu `AcceptInvitation`) agar tidak deadlock dengan usecase ini. Dicatat sebagai persyaratan lintas-plan.
- **Pengirim email.** Nama pengundang = `Input.InvitedByLabel`; kosong -> label dari identitas di ctx (`Name`, lalu `Email`); kosong -> `Invitation.InvitedByLabel` yang tersimpan; kosong -> `RenderInvitationEmail` memakai fallback "Administrator Grimoire". `invited_by` di tabel TIDAK diubah oleh kirim ulang (tetap pengundang pertama).
- **Via audit.** Entry HTTP -> `via="users"`; entry bus -> `via="project"`. Ditentukan handler, dikirim di `Input.Via`.
- **Kegagalan setelah commit.** Gagal render template atau gagal update `delivery_status` tidak membatalkan request: password sudah terbit, sehingga respons tetap dibentuk dari hasil kirim. Render gagal diperlakukan sebagai `failed` (log error) dan credentials dikembalikan. Gagal `UpdateInvitation` kecil hanya di-log.
- **`error_class` audit.** Overview 4.8 menyebut `error_class` untuk `email_failed`, tetapi `delivery.Deliver` hanya mengembalikan status string. Plan ini hanya mengisi `email` dan `delivery_status` (kelas error sudah ada di log `delivery`). Bila foundation kemudian memperluas `Deliver`, tambahkan `error_class` di satu tempat.
- **Rahasia.** Password di dalam usecase bertipe `base.Secret`; hanya `Reveal()` saat render email dan saat mengisi DTO/bus Response. Tidak pernah masuk `audit.Event`, log, atau `Publish`.
- **Bus tidak mengecek peran.** Handler bus dipercaya (internal). Otorisasi project dilakukan management sebelum memanggil.

Tambahan kecil pada foundation (perlu masuk `base/invitation.go`, ditulis sebagai langkah 1 plan ini bila foundation belum memuatnya):

```go
// CheckResendTarget memastikan user dan undangan boleh diterbitkan ulang.
// inv == nil atau sudah diterima -> ErrInvitationNotPending; user nonaktif -> ErrAlreadyRegistered(AlreadyRegisteredInactive).
func CheckResendTarget(u User, inv *Invitation) error
```

Urutan cek: `inv == nil || !inv.IsPending()` dulu, baru `!u.IsActive`. Tidak dicampur dengan `CheckInviteTarget` karena pemetaan error berbeda (tanpa undangan di sini = `INVITATION_NOT_PENDING`, bukan `registered`).

---

## 3. Plan (langkah berurutan)

Semua langkah TDD: tes dulu, lalu implementasi.

### Langkah 1. Entity: `CheckResendTarget`

File: `backend/internal/domain/core/internal/auth/base/invitation.go` (+ `invitation_test.go`). Tambah fungsi di atas. Tes tabel: user aktif + undangan menunggu -> nil; undangan kedaluwarsa tetap nil; inv nil -> `ErrInvitationNotPending`; inv diterima -> `ErrInvitationNotPending`; user nonaktif + menunggu -> `ALREADY_REGISTERED` reason `inactive`; user nonaktif + inv nil -> `INVITATION_NOT_PENDING` (urutan).

### Langkah 2. `params.go`

`backend/internal/domain/core/internal/auth/resendinvitation/params.go`:

```go
type Input struct {
    UserID         string
    InvitedBy      string // id pengundang; dipakai sebagai aktor audit bila ctx tanpa identitas
    InvitedByLabel string
    ProjectName    string
    Initial        bool
    Via            string // "users" | "project"
    Meta           base.RequestMeta
}

type Credentials struct {
    Email             string `json:"email"`
    TemporaryPassword string `json:"temporary_password"` // string biasa, bukan base.Secret (Secret ter-marshal "***")
    LoginURL          string `json:"login_url"`
}

// Result keluar lewat WriteJSON (HTTP) atau Response bus. TIDAK dilog, TIDAK masuk audit.Event.
type Result struct {
    EmailSent   bool         `json:"email_sent"`
    EmailStatus string       `json:"email_status"` // sent | failed | not_configured
    ExpiresAt   time.Time    `json:"expires_at"`
    Credentials *Credentials `json:"credentials,omitempty"`
}
```

### Langkah 3. `repository.go`

`Repo` membungkus `*db.DB` dan `store.Store` (pola `resetpassword`).

```go
type Mutator func(u base.User, inv *base.Invitation) error

type IssueInput struct {
    UserID string
    Hash   string
    Now    time.Time
    Apply  Mutator // rule entity; error dikembalikan apa adanya
}

type Issued struct {
    User       base.User
    Invitation base.Invitation // setelah mutasi
    Revoked    int64           // jumlah sesi dicabut
}

func (r *Repo) Issue(ctx context.Context, in IssueInput) (Issued, error)
func (r *Repo) MarkDelivery(ctx context.Context, userID, status string, now time.Time) error
```

`Issue` dalam `db.BeginTx`:

1. `FindUserByID` (id bukan uuid atau tidak ada -> `base.ErrUserNotFound`).
2. `LockInvitation(userID)`; `found=false` -> `inv=nil`.
3. `in.Apply(user, invPtr)`; error -> rollback dan kembalikan error entity (bukan dibungkus ulang agar `*errs.Error` tetap terdeteksi; gunakan `%w` hanya untuk error non-errs).
4. `UpdatePassword(ctx, id, hash, true, now)`.
5. `DeleteSessionsByUser(ctx, id, nil)` (semua sesi).
6. `UpdateInvitation(ctx, inv)` (menyimpan `issued_at`, `expires_at`, `delivery_status=pending`, `resend_count`, window).

Catatan: `UpdateInvitation` ada di langkah terakhir; `RegisterResend` yang gagal (429) sudah menghentikan tx sebelum ada tulis apa pun. Counter kuota hanya naik bila seluruh tx commit.

`MarkDelivery`: tx kecil `LockInvitation` -> `inv.MarkDelivery(status, now)` -> `UpdateInvitation`. Bila baris sudah tak ada atau sudah diterima (race: penerima sempat login) -> no-op tanpa error.

### Langkah 4. `usecase.go`

```go
type Repository interface {
    Issue(ctx context.Context, in IssueInput) (Issued, error)
    MarkDelivery(ctx context.Context, userID, status string, now time.Time) error
}

type Usecase struct {
    repo     Repository
    hasher   base.Hasher
    mailer   mailer.Mailer
    audit    audit.Recorder
    clock    base.Clock
    rand     io.Reader
    policy   base.InvitationPolicy
    loginURL string
    mailTO   time.Duration
    log      *slog.Logger
}
```

`Execute(ctx, in) (Result, error)`:

1. `in.UserID` kosong atau bukan uuid -> `base.ErrUserNotFound` (repo juga menutup ini).
2. `GeneratePassword(u.rand)` -> `base.Secret`; `hasher.Hash` -> hash.
3. `now := u.clock()`.
4. `repo.Issue(ctx, IssueInput{..., Apply: func(user, inv) error { if err := base.CheckResendTarget(user, inv); err != nil { return err }; if !in.Initial { if err := inv.RegisterResend(now, u.policy); err != nil { return err } }; inv.Reissue(now, u.policy); return nil }})`.
   Error `*errs.Error` diteruskan; error lain dibungkus `resendinvitation: terbitkan: %w`.
5. Setelah commit (bukan lagi di dalam tx):
   - `label := inviterLabel(ctx, in, issued.Invitation)` (aturan di bagian Summary).
   - `content, rerr := base.RenderInvitationEmail(base.InvitationEmailData{InviteeEmail, InviterName: label, ProjectName: in.ProjectName, LoginURL: u.loginURL, TemporaryPassword: pw.Reveal(), ExpiresAt: issued.Invitation.ExpiresAt})`.
   - `rerr != nil` -> `status = base.DeliveryFailed`, log error (tanpa data). Selain itu `sendCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), u.mailTO)`; `status = delivery.Deliver(sendCtx, u.mailer, delivery.Message(user.Email, content), u.log)`.
   - `repo.MarkDelivery(ctx2, user.ID, status, u.clock())` dengan `context.WithoutCancel(ctx)`; error hanya di-log.
6. Audit (semua via `u.audit.Record`, fail-open, `Meta: in.Meta`, `TargetType:"user"`, `Target: user.ID`):
   - `!in.Initial`: `ActionUserInvitationResent`, args `{email, via, project_name (bila ada), resend_count: issued.Invitation.ResendCount}`.
   - `status != sent`: `ActionUserInvitationEmailFailed`, `Status:"error"`, args `{email, delivery_status: status}`.
   - Aktor: `Event.Actor = nil` bila ctx punya identitas; bila tidak dan `in.InvitedBy != ""`, `Actor = &audit.Actor{Type:"user", ID:in.InvitedBy, Label:in.InvitedByLabel}`.
   - Args TIDAK memuat password, hash, credentials, atau kunci `temporary_password`.
7. Bentuk `Result`: `EmailSent = status==sent`, `EmailStatus = status`, `ExpiresAt`, dan `Credentials` HANYA bila `!EmailSent` (`{Email: user.Email, TemporaryPassword: pw.Reveal(), LoginURL: u.loginURL}`).

Konstanta status memakai `base.Delivery*`; terjemahan ke string kontrak (`invitation.EmailStatus*`) satu-ke-satu (`sent`, `failed`, `not_configured`) di handler bus.

### Langkah 5. `handler.go` (HTTP)

`Handler{uc}` dengan method `Resend(w, r)`:

- Tanpa body; body yang dikirim diabaikan (tidak didekode, tidak error).
- `identity` dari `reqctx.IdentityFrom` -> `InvitedBy = id.UserID`, `InvitedByLabel = id.Name` (fallback `id.Email`).
- `Input{UserID: chi.URLParam(r,"id"), Via:"users", Meta: base.RequestMeta{IP: middleware.KeyByIP(r), UserAgent: base.TruncateUA(r.UserAgent())}}` (`Initial=false`, `ProjectName=""`).
- Error -> `httpx.WriteError`. Sukses: bila `res.Credentials != nil` set `Cache-Control: no-store`; `httpx.WriteJSON(w, 200, res)`.

### Langkah 6. `bushandler.go`

`BusHandler{uc}` dengan `Handle(ctx, req any) (any, error)`:

- `bus.Expect[resendinvitation.Request](req)`.
- Map ke `Input{UserID, InvitedBy, InvitedByLabel, ProjectName, Initial, Via:"project"}` (Meta kosong; IP/UA tidak tersedia lewat bus).
- Kembalikan `resendinvitation.Response{EmailSent, EmailStatus, ExpiresAt, Credentials: &invitation.Credentials{...}|nil}`.
- Error `*errs.Error` dikembalikan apa adanya (bus meneruskan utuh ke management).
- Kontrak `Response` di `lib/contracts/core/auth/resendinvitation` sudah ditetapkan overview 4.7; plan ini tidak mengubahnya.

### Langkah 7. `init.go`

```go
func Init(d app.Deps, s base.Shared) {
    uc := NewUsecase(NewRepo(d.DB), s.Hasher, d.Mailer, audit.New(d.Bus, d.Log), s.Clock, s.Rand,
        s.Invitation, s.LoginURL, d.Config.MailTimeout, d.Log)
    d.API.With(middleware.RequireAuth(), middleware.RequireRole(base.RoleSuperAdmin)).
        Post("/users/{id}/resend-invitation", NewHandler(uc).Resend)
    d.Bus.Register(constants.BUS_CORE_AUTH_RESEND_INVITATION, NewBusHandler(uc).Handle)
}
```

### Langkah 8. Wiring

`backend/internal/domain/core/internal/auth/base.go`: import `resendinvitation`, panggil `resendinvitation.Init(d, shared)` di `Register` (setelah `resetpassword`). Perbarui `base_test.go` bila ada tes yang menghitung route atau handler terdaftar.

Route `/users/{id}/resend-invitation` tidak bentrok dengan `/users/invite` (route literal menang atas param di chi; pastikan `inviteuser` mendaftar `Post("/users/invite")` dan tes route di `base_test.go` memeriksa keduanya).

### Langkah 9. OpenAPI

`api/modules/core/auth.yml`: path `/users/{id}/resend-invitation` (POST), tag `Users`, security session cookie, parameter path `id` (uuid), respons 200 skema `ResendInvitationResult` (`email_sent`, `email_status` enum `sent|failed|not_configured`, `expires_at`, `credentials` opsional rujuk `InvitationCredentials` dari foundation), header respons `Cache-Control: no-store` bila `credentials`. Error 401, 403, 404, 409 (dua contoh reason: `INVITATION_NOT_PENDING` dan `ALREADY_REGISTERED` `inactive`), 429 (`details.retry_after_seconds`, `scope`). Rujuk `Error` dari `api/modules/infra/foundation.yml`. Tambah path ke `api/root.yml`; jalankan `make api-bundle` dan `make api-check`; commit `api/openapi.yml`.

### Langkah 10. Tes dan verifikasi

Tulis tes sesuai bagian 6; jalankan `go build ./...`, `go vet ./...`, `go test ./...`, `go test -tags integration ./internal/domain/core/...`, `internal/archtest`, lint.

---

## 4. Migration

Tidak ada. Memakai `user_invitations` (foundation, `backend/migrations/core/<VERSION>_create_user_invitations.sql`) dan tabel `users`/`sessions` yang sudah ada.

---

## 5. Changes (per file/folder)

| File / folder | Aksi | Isi |
|---|---|---|
| `backend/internal/domain/core/internal/auth/base/invitation.go` + `invitation_test.go` | ubah | `CheckResendTarget` (bila belum ada dari foundation) |
| `.../auth/resendinvitation/params.go` | baru | `Input`, `Credentials`, `Result` |
| `.../auth/resendinvitation/repository.go` | baru | `Repo.Issue` (tx), `Repo.MarkDelivery` |
| `.../auth/resendinvitation/usecase.go` | baru | `Usecase.Execute`, helper `inviterLabel` |
| `.../auth/resendinvitation/handler.go` | baru | HTTP `Resend`, header `no-store` bersyarat |
| `.../auth/resendinvitation/bushandler.go` | baru | `bus.Expect` + map ke/dari kontrak |
| `.../auth/resendinvitation/init.go` | baru | route + registrasi bus |
| `.../auth/resendinvitation/usecase_test.go`, `handler_test.go`, `bushandler_test.go` | baru | tes unit |
| `.../auth/resendinvitation/repository_integration_test.go` | baru | tes integrasi (tag `integration`) |
| `.../auth/resendinvitation/e2e_test.go` | baru | tes e2e kriteria 3 dan 7 (ikuti pola e2e/integrasi auth yang sudah ada, mis. `auth_integration_test.go`) |
| `.../auth/base.go` (+ `base_test.go`) | ubah | panggil `resendinvitation.Init` |
| `api/modules/core/auth.yml`, `api/root.yml`, `api/openapi.yml` | ubah | path baru + bundle |

Tidak menyentuh: `lib/contracts/...` (sudah di foundation), migrasi, module lain.

---

## 6. Test Scenario

### 6.1 Unit usecase (`usecase_test.go`, fake `Repository`, `mailertest.Fake`, `fakeAudit`, jam tetap, `rand` deterministik)

| # | Skenario | Harapan |
|---|---|---|
| U1 | `Initial=false`, undangan menunggu, mailer sukses | `EmailSent=true`, `EmailStatus=sent`, `Credentials=nil`; `Apply` memanggil `RegisterResend` (`ResendCount` naik) dan `Reissue` (`ExpiresAt=now+TTL`); email terkirim ke email user dengan password yang sama dengan yang di-hash; audit `user.invitation.resent` satu kali |
| U2 | `Initial=true` | `RegisterResend` TIDAK dipanggil (`ResendCount` dan window tetap); tidak ada audit `resent`; email tetap terkirim; `ExpiresAt` diperpanjang |
| U3 | Kuota: resend ke-1..3 lolos, ke-4 dalam jam sama | 429 `RATE_LIMITED`, `details.scope="resend"`, `retry_after_seconds` benar; tidak ada hash tersimpan, tidak ada email, tidak ada audit |
| U4 | Window bergeser | resend ke-4 pada `start+1h` lolos (window baru, count=1) |
| U5 | `Initial=true` berulang >3 kali | tidak pernah 429 |
| U6 | User tidak ada (repo -> `ErrUserNotFound`), id bukan uuid, id kosong | 404 `NOT_FOUND`, tanpa email |
| U7 | Tanpa baris undangan (akun `createuser`) | 409 `CONFLICT` reason `INVITATION_NOT_PENDING` |
| U8 | Undangan sudah diterima | 409 `INVITATION_NOT_PENDING` |
| U9 | User nonaktif dengan undangan menunggu | 409 `ALREADY_REGISTERED` reason `inactive`; tidak ada email |
| U10 | Undangan kedaluwarsa tetapi pending | berhasil (memulihkan); `ExpiresAt` baru |
| U11 | Password lama tidak berlaku | hash yang dikirim ke `UpdatePassword` berbeda dari hash lama dan `Verify(passwordBaru)=true`, `Verify(passwordLama)=false`; `must_change=true` |
| U12 | Sesi dicabut | `DeleteSessionsByUser` dipanggil dengan `exceptTokenHash=nil` (semua sesi); `Revoked` mencerminkan jumlah |
| U13 | Fallback `ErrNotConfigured` (mailer noop) | `EmailSent=false`, `EmailStatus=not_configured`, `Credentials` terisi (email, password, login URL); audit `email_failed` status `error` args `delivery_status=not_configured`; `MarkDelivery("not_configured")` |
| U14 | Mailer error 550 | `EmailStatus=failed`, `Credentials` terisi; audit `email_failed`; `MarkDelivery("failed")` |
| U15 | Render template gagal (stub renderer atau data tak valid) | diperlakukan `failed`, credentials dikembalikan, request tidak error |
| U16 | `MarkDelivery` mengembalikan error | request tetap sukses, error hanya di-log |
| U17 | Kirim sukses | `MarkDelivery("sent")` dipanggil dan `SentAt` terisi (cek di fake repo) |
| U18 | Label pengundang | prioritas `InvitedByLabel` > nama identitas ctx > email identitas > label tersimpan > kosong (fallback template); `ProjectName` muncul di Subject dan Text bila diisi |
| U19 | Aktor audit | ctx dengan identitas -> `Actor=nil`; ctx tanpa identitas dan `InvitedBy` diisi -> `Actor` eksplisit |
| U20 | Rahasia tidak bocor | seluruh `audit.Event.Args/Result` dan buffer log slog tidak mengandung password, hash, atau `temporary_password` (cek string) |
| U21 | `Via` | `users` untuk HTTP, `project` untuk bus tercermin di args audit |
| U22 | Klien memutus ctx setelah commit | pengiriman tetap berjalan (ctx kirim memakai `WithoutCancel` + timeout `MailTimeout`) |
| U23 | `Repo.Issue` error DB | dibungkus, tidak ada email dan tidak ada audit |

### 6.2 Unit entity (`base/invitation_test.go`)

`CheckResendTarget` seperti Langkah 1. `RegisterResend` dan `Reissue` sudah diuji foundation.

### 6.3 Handler (`handler_test.go`, `bushandler_test.go`)

| # | Skenario | Harapan |
|---|---|---|
| H1 | POST sukses terkirim | 200, JSON `{email_sent:true,email_status:"sent",expires_at}`; tidak ada kunci `credentials`; tidak ada header `no-store` |
| H2 | POST fallback | 200 dengan `credentials{email,temporary_password,login_url}` dan `Cache-Control: no-store`; `temporary_password` bukan `"***"` |
| H3 | Body tidak diperlukan | request dengan body sembarang atau kosong menghasilkan hasil sama |
| H4 | Param `{id}` diteruskan | `chi.URLParam` terbaca; `Via="users"`, `Initial=false` |
| H5 | Peta error | 404, 409 (kedua reason), 429 dengan `details` dan `Retry-After` bila sudah disediakan `httpx.WriteError`; 500 tidak membocorkan detail |
| H6 | Routing | tanpa sesi -> 401; developer -> 403; super_admin -> 200 |
| B1 | Bus Initial=true | Request dipetakan, `Via="project"`, Response berisi `EmailStatus`, `ExpiresAt`, `Credentials` hanya saat gagal kirim |
| B2 | Bus tipe request salah | error `bus.Expect` |
| B3 | Error `*errs.Error` | diteruskan identik; `errors.As` di consumer menemukan kode dan reason |

### 6.4 Integrasi repo (`repository_integration_test.go`, `-tags integration`, `db.TestDB`)

| # | Skenario | Harapan |
|---|---|---|
| I1 | `Issue` sukses | satu tx: `users.password_hash` berubah, `must_change_password=true`, semua baris `sessions` user terhapus, `user_invitations` (`issued_at`, `expires_at`, `delivery_status='pending'`, `resend_count`, `resend_window_*`) terbarui; sesi user lain tidak tersentuh |
| I2 | Rollback saat `Apply` error (429 atau 409) | hash dan sesi tidak berubah, counter tidak naik |
| I3 | Rollback saat langkah akhir gagal (mis. paksa error di `UpdateInvitation` lewat user dihapus di tx lain) | tidak ada perubahan parsial |
| I4 | Lock | dua `Issue` bersamaan untuk user yang sama: tx kedua menunggu `LockInvitation`; hasil akhir `resend_window_count` = 2 (tidak ada lost update) dan hash terakhir milik tx kedua |
| I5 | Lock vs `AcceptInvitation` bersamaan | serial: bila accept lebih dulu, `Issue` -> `INVITATION_NOT_PENDING`; bila `Issue` lebih dulu, accept tetap menandai (password lama sudah tak berlaku sehingga changepassword memakai password baru) |
| I6 | Undangan kedaluwarsa dan diterbitkan ulang | `expires_at > now` setelah `Issue` |
| I7 | `MarkDelivery` | `sent` mengisi `sent_at`; `failed` dan `not_configured` tidak mengubah `sent_at` sebelumnya; undangan diterima -> no-op |
| I8 | User tidak aktif / tidak ada / id bukan uuid | error sesuai (`ErrUserNotFound` atau error entity) |

### 6.5 Regresi

- Tes T01 `resetpassword`, `login`, `changepassword` tetap hijau.
- `archtest` hijau (feature tidak meng-import feature lain; hanya `base`, `base/store`, `base/audit`, `base/delivery`, `infra/*`, `lib/*`).

---

## 7. E2E Scenarios

Memakai server uji penuh (`db.TestDB`, `mailertest.Fake` atau SMTP Mailpit palsu, jam yang dapat dimajukan).

Positif:

1. **Kriteria 3: kirim ulang memulihkan akun kedaluwarsa.** Super_admin mengundang `baru@contoh.id` (via `POST /api/users/invite`). Set `expires_at` ke masa lalu. `POST /api/auth/login` dengan password lama -> 401 `INVALID_CREDENTIALS` identik kredensial salah. `POST /api/users/{id}/resend-invitation` -> 200 `email_sent=true`; email baru diterima, mengandung password baru. Login dengan password lama -> 401. Login dengan password baru -> 200 (dengan `must_change_password=true`).
2. **Sesi dicabut.** Penerima sudah login (sesi aktif, terjebak gate ganti password); setelah kirim ulang, request dengan cookie lama -> 401.
3. **Penerbitan pertama jalur project** (lewat bus `Initial=true`, diuji di tes modul auth dengan memanggil bus langsung): tanpa audit `user.invitation.resent`, kuota resend tidak berkurang; email terkirim.
4. **Kirim ulang pending belum kedaluwarsa.** Password lama berhenti berlaku, `expires_at` diperpanjang, audit `user.invitation.resent` tercatat dengan `resend_count` dan tanpa password.
5. **Fallback.** `MAIL_DRIVER` kosong atau Mailpit mati: 200 `email_sent=false`, `credentials` ada, `Cache-Control: no-store`; audit `user.invitation.email_failed` (status `error`) tanpa password; `GET /api/users/{id}` menampilkan `invitation.delivery_status` `failed` atau `not_configured` dan tidak memuat password; tidak ada endpoint yang mengembalikan kredensial itu lagi.

Negatif:

1. **Kriteria 7: rate limit.** Kirim ulang ke-4 dalam satu jam -> 429 `RATE_LIMITED`, `details.scope="resend"`, `retry_after_seconds` > 0; password dan sesi tidak berubah; setelah jam dimajukan 1 jam -> 200.
2. Undangan sudah diterima (penerima sudah ganti password) -> 409 `CONFLICT` reason `INVITATION_NOT_PENDING`.
3. Akun biasa tanpa undangan (dibuat lewat `POST /api/users`) -> 409 `INVITATION_NOT_PENDING`.
4. Akun menunggu tetapi dinonaktifkan -> 409 `ALREADY_REGISTERED` reason `inactive`.
5. User tidak ada atau id bukan uuid -> 404.
6. Developer -> 403; tanpa sesi -> 401.
7. Pengirim memutus koneksi tepat setelah commit: pengiriman email tetap selesai dan `delivery_status` terbarui.

---

## 8. Request/Response body

### 8.1 `POST /api/users/{id}/resend-invitation`

Request: tanpa body. Contoh:

```
POST /api/users/7c1f3c2e-5b7a-4d7e-9a51-1f0b8a3e6d21/resend-invitation
Cookie: <sesi super_admin>
```

200, email terkirim (tanpa kunci `credentials`):

```json
{
  "email_sent": true,
  "email_status": "sent",
  "expires_at": "2026-10-28T09:00:00Z"
}
```

200, fallback (header `Cache-Control: no-store`):

```json
{
  "email_sent": false,
  "email_status": "failed",
  "expires_at": "2026-10-28T09:00:00Z",
  "credentials": {
    "email": "baru@contoh.id",
    "temporary_password": "Zp4wNe8TqYv3KdRh",
    "login_url": "https://grimoire.contoh.id/login"
  }
}
```

`email_status` fallback bernilai `failed` (SMTP error) atau `not_configured` (mailer noop).

### 8.2 Bus `core.auth.resend-invitation`

Request (`resendinvitation.Request`):

```json
{
  "UserID": "7c1f3c2e-5b7a-4d7e-9a51-1f0b8a3e6d21",
  "InvitedBy": "2b6a0f5e-8f3a-4c0a-9d2e-5c1f7a9b3e10",
  "InvitedByLabel": "Owner Satu",
  "ProjectName": "Proyek Alfa",
  "Initial": true
}
```

Response (`resendinvitation.Response`), terkirim:

```json
{ "EmailSent": true, "EmailStatus": "sent", "ExpiresAt": "2026-10-28T09:00:00Z", "Credentials": null }
```

Response, tidak terkirim (`Credentials` hanya diteruskan ke respons HTTP pengundang; jangan di-log):

```json
{
  "EmailSent": false,
  "EmailStatus": "not_configured",
  "ExpiresAt": "2026-10-28T09:00:00Z",
  "Credentials": { "Email": "baru@contoh.id", "TemporaryPassword": "Zp4wNe8TqYv3KdRh", "LoginURL": "/login" }
}
```

### 8.3 Error

Envelope mengikuti `httpx.WriteError`.

404 user tidak ada:

```json
{ "error": { "code": "NOT_FOUND", "message": "user tidak ditemukan" } }
```

409 undangan tidak menunggu (diterima atau tidak ada):

```json
{ "error": { "code": "CONFLICT", "message": "undangan sudah diterima atau tidak ada", "details": { "reason": "INVITATION_NOT_PENDING" } } }
```

409 akun nonaktif:

```json
{ "error": { "code": "ALREADY_REGISTERED", "message": "email sudah terdaftar", "details": { "reason": "inactive" } } }
```

429 kuota kirim ulang:

```json
{ "error": { "code": "RATE_LIMITED", "message": "terlalu banyak permintaan undangan", "details": { "retry_after_seconds": 1840, "scope": "resend" } } }
```

403 bukan super_admin:

```json
{ "error": { "code": "FORBIDDEN", "message": "akses ditolak" } }
```

401 tanpa sesi:

```json
{ "error": { "code": "UNAUTHORIZED", "message": "perlu login" } }
```

---

## 9. Catatan lintas plan dan risiko

- `auth-login-changepassword` harus menjaga urutan lock `LockInvitation` sebelum `UpdatePassword` (lihat bagian 2) agar tidak deadlock dengan usecase ini.
- `auth-inviteuser` untuk akun menunggu (K6, jalur Users) menerbitkan ulang dengan logika setara; logika bersama yang sama harus berasal dari `base/` (`CheckResendTarget`, `RegisterResend`, `Reissue`, `delivery`, `invitemail`), bukan import feature ini.
- `management-inviteuser` dan `management-resendinvitation` hanya bergantung pada kontrak bus di atas; mereka meneruskan `RATE_LIMITED`, `INVITATION_NOT_PENDING`, `ALREADY_REGISTERED` apa adanya.
- Risiko: email terkirim tetapi `MarkDelivery` gagal -> UI menampilkan `pending` sementara sebenarnya terkirim; diperbaiki oleh kirim ulang berikutnya. Diterima.
- Risiko: proses mati antara commit dan kirim -> undangan `pending` tanpa email dan tanpa kredensial di respons; dipulihkan lewat Kirim ulang. Diterima (sejalan overview 12).
