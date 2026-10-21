# T12 management-resendinvitation — Plan Usecase

Induk: `docs/superpowers/plans/core-invitation-email/overview.md` (foundation; menang bila bertentangan). Pola acuan kode: `backend/internal/domain/project/internal/management/removemember` (`init.go`, `handler.go`, `usecase.go`, `repository.go`) dan `addmember` (respons JSON). Pola "kredensial sekali tampil + no-store": `backend/internal/domain/core/internal/auth/resetpassword/handler.go`.

## 1. Metadata

| Atribut | Nilai |
|---|---|
| Ticket | T12 (kriteria 3, 5, 7, 9) |
| Usecase slug | `management-resendinvitation` |
| Scope | backend (UI tombol "Kirim ulang" ada di `frontend-plan.md`) |
| Domain / Module | project / management |
| Folder | `backend/internal/domain/project/internal/management/resendinvitation` |
| Endpoint | `POST /api/projects/{projectID}/invitations/{userID}/resend` |
| Method | POST, tanpa body |
| Version | tanpa prefix versi; naikkan `info.version` minor `api/modules/project/management.yml` (sekali, bersama usecase lain) |
| Auth | `RequireAuth`; owner project atau super_admin lewat `PermManageMembers` |
| Bus dikonsumsi | `core.auth.get-user` (Call, aditif `Invitation`), `core.auth.resend-invitation` (Call) |
| Bus disediakan | tidak ada |
| Audit | tidak ada di management. Aksi `user.invitation.resent` dan `user.invitation.email_failed` dicatat auth (actor dari ctx). Tidak ada `project.*` baru |
| Bergantung pada | foundation (kontrak `resendinvitation`, aditif `getuser.Response.Invitation`, `errs.CodeAlreadyRegistered`, konstanta bus); `auth-resendinvitation` (provider bus); `auth-invitation-markers` (`getuserbus` mengisi `Invitation`); `management-listinvitations` (menulis `ErrInvitationNotPending` di `base/errors.go`); `management-inviteuser` (menulis `base/users` method `ResendInvitation`) |

## 2. Summary Plan

Owner (atau super_admin) mengirim ulang undangan ke anggota project yang belum menerima undangannya. Management hanya mengorkestrasi: cek izin, pastikan target anggota project, pastikan undangan masih menunggu, lalu panggil bus `core.auth.resend-invitation`. Auth yang menerbitkan password sementara baru, mencabut sesi, memperpanjang `expires_at`, menghitung kuota kirim ulang, mengirim email, dan mengaudit.

Keputusan:

- Tidak ada transaksi di management. Fitur ini tidak menulis tabel project; hanya membaca (`FindAccess`, daftar anggota). Tidak ada `InTx`, tidak ada lock. Bus tidak pernah dipanggil di dalam tx (tidak ada tx).
- Cek "anggota project" memakai `ListMembers` + `base.FindMember` (rule yang sudah ada), bukan SQL baru. Id bukan uuid -> `ErrMemberNotFound` tanpa menyentuh DB (pola `removemember`).
- Hak akses: `ResolveLevel` + `Require(PermManageMembers)`. Non-anggota (level none) -> 404 `ErrProjectNotFound`; member biasa -> 403 `ErrForbidden`. Cek dilakukan SEBELUM apa pun soal target, supaya member/non-anggota tidak bisa mengetahui keberadaan user atau status undangan.
- Target bukan anggota project (termasuk user yang ada di tempat lain) -> 404 `ErrMemberNotFound`. Kirim ulang hanya sah untuk anggota project itu, mencegah owner memicu email/reset password ke akun sembarang.
- Status undangan dibaca lewat `get-user` (`Response.Invitation`). `Found=false` (anggota yatim, user terhapus) -> 404 `ErrMemberNotFound`. `Invitation == nil` (sudah diterima, atau akun tanpa undangan seperti `createuser`) -> 409 `INVITATION_NOT_PENDING`. Undangan kedaluwarsa tetap `Invitation != nil` (field `Expired=true`) sehingga kirim ulang memulihkannya (kriteria 3).
- Cek `get-user` di management bersifat optimistik (cepat gagal, pesan tepat). Sumber kebenaran tetap auth: bila antara get-user dan resend undangan diterima, auth membalas `CONFLICT` reason `INVITATION_NOT_PENDING` dan diteruskan apa adanya (bentuk sama dengan error lokal, jadi klien tidak membedakan).
- Error auth diteruskan utuh: `RATE_LIMITED` (details `retry_after_seconds`, `scope="resend"`) -> 429 (kriteria 7); `CONFLICT`/`INVITATION_NOT_PENDING` -> 409; `ALREADY_REGISTERED` reason `inactive` -> 409. `NOT_FOUND` dari auth dipetakan ke `ErrMemberNotFound`. Error lain (bus tidak terdaftar, DB) dibungkus `%w` -> 500. Pemetaan memakai `errors.As(err, *errs.Error)`; `base/users` membungkus dengan `%w` sehingga `errors.As` tetap jalan.
- Request ke auth: `{UserID, InvitedBy: actor.ID, InvitedByLabel: actor.Label (email actor), ProjectName: project.Name, Initial: false}`. `Initial=false` berarti kuota kirim ulang dihitung dan audit `user.invitation.resent` ditulis auth.
- Respons 200 `{email_sent, email_status, expires_at, credentials?}`. `credentials` hanya ada bila `EmailSent=false` dan auth mengembalikan `Credentials != nil`. Bila ada, handler memasang `Cache-Control: no-store` SEBELUM menulis body. Kredensial tidak di-log, tidak diaudit, tidak disimpan; usecase hanya meneruskan nilai.
- Kegagalan SMTP bukan error HTTP: auth membalas sukses `EmailSent=false` + `Credentials` (kriteria 9), management meneruskannya sebagai 200.
- Tidak ada audit tambahan di management (keputusan tiket/overview). Konsekuensinya: aksi tidak muncul sebagai `project.*`; jejak ada di `user.invitation.resent` dengan args `via="project"`, `project_name`.
- Rule bisnis yang ada sudah di entity/base (`ResolveLevel`, `Require`, `FindMember`); tidak ada entity baru. Usecase murni orkestrasi sehingga tidak dipecah.

## 3. Plan (langkah berurutan)

Prasyarat: foundation selesai; `auth-resendinvitation` dan `auth-invitation-markers` sudah merge (atau handler bus-nya terdaftar di tes integrasi); `management-listinvitations` dan `management-inviteuser` sudah menulis `ErrInvitationNotPending` dan `users.ResendInvitation`. Bila dikerjakan paralel, pastikan commit `base/users` dari `management-inviteuser` masuk lebih dulu (lihat overview bagian 9). Bila `ErrInvitationNotPending` belum ada saat mulai, JANGAN mendefinisikan duplikat di feature ini; tunggu atau cherry-pick commit dari `listinvitations`.

1. Tes dulu (TDD): `usecase_test.go`, `handler_test.go` (bagian 6).
2. `params.go`: DTO respons.
   ```go
   // ResendResponse adalah 200 untuk kirim ulang undangan.
   type ResendResponse struct {
       EmailSent   bool                 `json:"email_sent"`
       EmailStatus string               `json:"email_status"`
       ExpiresAt   time.Time            `json:"expires_at"`
       Credentials *CredentialsResponse `json:"credentials,omitempty"`
   }
   // CredentialsResponse memakai string biasa (BUKAN base.Secret) agar ter-marshal apa adanya.
   type CredentialsResponse struct {
       Email             string `json:"email"`
       TemporaryPassword string `json:"temporary_password"`
       LoginURL          string `json:"login_url"`
   }
   ```
   `ExpiresAt` di-UTC-kan di usecase.
3. `usecase.go`:
   ```go
   type Repository interface {
       FindAccess(ctx context.Context, projectID, userID string) (p base.Project, memberRole string, found bool, err error)
       ListMembers(ctx context.Context, projectID string) ([]base.Member, error)
   }
   // Sisi consumer; users.BusLookup memenuhinya (Get sudah ada, ResendInvitation ditulis oleh management-inviteuser).
   type Users interface {
       Get(ctx context.Context, userID string) (getuser.Response, error)
       ResendInvitation(ctx context.Context, req resendinvitation.Request) (resendinvitation.Response, error)
   }
   type Input struct{ ProjectID, UserID string }
   type Usecase struct { repo Repository; users Users; log *slog.Logger }
   func NewUsecase(repo Repository, u Users, log *slog.Logger) *Usecase
   func (u *Usecase) Execute(ctx context.Context, in Input) (ResendResponse, error)
   ```
   Urutan `Execute`:
   1. `actor, ok := base.ActorFrom(reqctx.IdentityFrom(ctx))`; `!ok` -> `errs.New(errs.CodeUnauthorized, "perlu login")`.
   2. `p, memberRole, found, err := repo.FindAccess(ctx, in.ProjectID, actor.ID)`; error dibungkus `resendinvitation: cari akses: %w`; `!found` -> `base.ErrProjectNotFound`.
   3. `level := base.ResolveLevel(actor.Role, memberRole, true)`; `base.Require(level, base.PermManageMembers)` (error dikembalikan apa adanya: 404 atau 403).
   4. `uuid.Parse(in.UserID)` gagal -> `base.ErrMemberNotFound`; gunakan bentuk kanonik `.String()`.
   5. `members, err := repo.ListMembers(ctx, p.ID)`; `base.FindMember(members, target)`; tidak ada -> `base.ErrMemberNotFound`.
   6. `res, err := users.Get(ctx, target)`; error -> `resendinvitation: get-user: %w`. `!res.Found` -> `base.ErrMemberNotFound`. `res.Invitation == nil` -> `base.ErrInvitationNotPending`.
   7. `out, err := users.ResendInvitation(ctx, resendinvitation.Request{UserID: target, InvitedBy: actor.ID, InvitedByLabel: actor.Label, ProjectName: p.Name, Initial: false})`. Error: bila `errors.As` ke `*errs.Error` dengan `Code == errs.CodeNotFound` -> `base.ErrMemberNotFound`; `*errs.Error` lain dikembalikan APA ADANYA (jangan dibungkus ulang supaya code/details sampai ke `respond.WriteError`); non-errs dibungkus `%w`.
   8. Bangun `ResendResponse`; `out.Credentials != nil && !out.EmailSent` -> isi `Credentials`. (Bila `EmailSent=true` tetapi `Credentials != nil`, abaikan kredensial: pertahanan berlapis agar tidak bocor, dan `log.WarnContext` tanpa nilai kredensial.)
4. `handler.go`:
   ```go
   type executor interface {
       Execute(ctx context.Context, in Input) (ResendResponse, error)
   }
   type handler struct{ uc executor }
   ```
   `ServeHTTP`: panggil `Execute` dengan `chi.URLParam(r, "projectID")` dan `chi.URLParam(r, "userID")`; error -> `respond.WriteError`; bila `res.Credentials != nil` -> `w.Header().Set("Cache-Control", "no-store")`; lalu `respond.WriteJSON(w, http.StatusOK, res)`. Tidak membaca body (body diabaikan, tidak di-decode). Tidak butuh `RequestMeta` karena management tidak mengaudit.
5. `repository.go`: `Repo{s *store.Store}` dengan `newRepo(s)`; `FindAccess` dan `ListMembers` hanya mendelegasikan ke `store.Store` (SQL bersama sudah ada di `base/store`, tidak ada SQL baru di feature ini). `var _ Repository = (*Repo)(nil)`.
6. `init.go`:
   ```go
   // Init memasang POST /projects/{projectID}/invitations/{userID}/resend pada d.API.
   func Init(d app.Deps) {
       s := store.New(d.DB)
       uc := NewUsecase(newRepo(s), users.New(d.Bus), d.Log)
       d.API.With(middleware.RequireAuth()).Post("/projects/{projectID}/invitations/{userID}/resend", handler{uc: uc}.ServeHTTP)
   }
   ```
   Pola penuh langsung di `d.API` (aturan README); param `{projectID}` dan `{userID}`.
7. `management/base.go`: tambah `resendinvitation.Init(d)` di blok route REST setelah `removemember.Init(d)` (atau setelah `inviteuser`/`listinvitations` bila sudah ada) dan import-nya. Karena path `/invitations/{userID}/resend` berbeda dari `/invitations/{userID}` (DELETE) dan `/invitations` (GET/POST), tidak ada konflik pohon chi selama semuanya didaftarkan pola penuh.
8. OpenAPI `api/modules/project/management.yml`: tambah path `/projects/{projectID}/invitations/{userID}/resend` (POST, `operationId: resendProjectInvitation`), skema `ResendInvitationResponse` yang merujuk `InvitationCredentials` dan `InvitationStatus` dari foundation (`auth.yml`), respons 200/401/403/404/409/429/500 memakai `Error` dari `foundation.yml`; header `Cache-Control` didokumentasikan pada 200. Daftarkan `$ref` path di `api/root.yml`, lalu `make api-bundle`, `make api-check`, commit `api/openapi.yml`.
9. Tes arsitektur: `go test ./internal/archtest/...` harus hijau (feature hanya meng-import `base`, `base/store`, `base/users`, `lib/contracts/...`, `infra/{app,middleware,http/respond}`).

## 4. Migration

Tidak ada. Tabel `user_invitations` milik core/auth dibuat oleh foundation. Management tidak menyentuh tabel itu dan tidak ada migrasi project.

## 5. Changes (per file/folder)

| File / folder | Aksi | Isi |
|---|---|---|
| `backend/internal/domain/project/internal/management/resendinvitation/init.go` | baru | rute POST + wiring |
| `.../resendinvitation/handler.go`, `handler_test.go` | baru | HTTP, no-store, pemetaan error |
| `.../resendinvitation/params.go` | baru | `ResendResponse`, `CredentialsResponse` |
| `.../resendinvitation/usecase.go`, `usecase_test.go` | baru | orkestrasi |
| `.../resendinvitation/repository.go`, `repository_integration_test.go` | baru | delegasi ke `store` (tag `integration`) |
| `.../resendinvitation/resendinvitation_integration_test.go` | baru | lintas bus (auth asli) dengan `mailertest.Fake` |
| `backend/internal/domain/project/internal/management/base.go` | ubah | `resendinvitation.Init(d)` |
| `backend/internal/domain/project/internal/management/base/users/users.go` | TIDAK diubah di sini | `ResendInvitation` ditulis oleh plan `management-inviteuser`; bila belum ada saat implement, tambahkan persis sesuai kontrak di overview 4.9 (wrapper `bus.CallTyped[resendinvitation.Response]`, error `fmt.Errorf("users: resend-invitation: %w", err)`) dan koordinasikan agar tidak duplikat |
| `backend/internal/domain/project/internal/management/base/errors.go` | TIDAK diubah di sini | `ErrInvitationNotPending` dari `management-listinvitations` |
| `api/modules/project/management.yml`, `api/root.yml`, `api/openapi.yml` | ubah | path baru + bundle |

Tidak ada perubahan pada auth, mailer, config, `lib/contracts`, `lib/constants`, atau audit (semuanya foundation / plan usecase auth).

## 6. Test Scenario

Unit (`usecase_test.go`, fake `Repository` dan fake `Users` yang mencatat request; actor lewat `reqctx.WithIdentity`):

| # | Skenario | Harapan |
|---|---|---|
| 1 | Tanpa identitas | 401 `UNAUTHORIZED`, tidak ada panggilan repo/bus |
| 2 | Non-anggota (FindAccess `found=false`, atau level none) | `ErrProjectNotFound` (404); `Users` tidak dipanggil |
| 3 | Actor member biasa | `ErrForbidden` (403); `Users` tidak dipanggil |
| 4 | Owner project -> sukses | `ResendInvitation` dipanggil sekali dengan `UserID` kanonik, `InvitedBy=actor.ID`, `InvitedByLabel=actor.Label`, `ProjectName=project.Name`, `Initial=false` |
| 5 | super_admin non-anggota | diizinkan (level admin) |
| 6 | `userID` bukan uuid | `ErrMemberNotFound`; `ListMembers` tidak dipanggil |
| 7 | Target bukan anggota project | `ErrMemberNotFound`; `Get`/`ResendInvitation` tidak dipanggil |
| 8 | `Get` -> `Found=false` | `ErrMemberNotFound` |
| 9 | `Get` -> `Invitation=nil` (sudah diterima / `createuser`) | `ErrInvitationNotPending` (409, reason `INVITATION_NOT_PENDING`); `ResendInvitation` tidak dipanggil |
| 10 | `Get` -> `Invitation{Expired:true}` | lanjut ke `ResendInvitation` (kriteria 3) |
| 11 | `Get` error bus | error dibungkus, `errors.Is` tetap jalan, bukan `*errs.Error` |
| 12 | Auth `RATE_LIMITED` (details `retry_after_seconds`, `scope=resend`) | error sama persis diteruskan (`errors.As` code + details utuh) (kriteria 7) |
| 13 | Auth `CONFLICT` `INVITATION_NOT_PENDING` (race) | diteruskan apa adanya |
| 14 | Auth `NOT_FOUND` | dipetakan ke `ErrMemberNotFound` |
| 15 | Auth `ALREADY_REGISTERED` reason `inactive` | diteruskan apa adanya |
| 16 | Auth error non-errs | dibungkus, 500 di handler |
| 17 | Auth sukses `EmailSent=true`, `Credentials=nil` | respons tanpa `Credentials`; `ExpiresAt` UTC |
| 18 | Auth `EmailSent=false` + `Credentials` (kriteria 9) | respons memuat `Credentials` dengan tiga field persis; `EmailStatus` diteruskan (`failed`/`not_configured`) |
| 19 | Auth `EmailSent=true` tapi `Credentials != nil` (kontrak dilanggar) | `Credentials` dibuang dari respons |
| 20 | Log (buffer slog) pada semua jalur | tidak memuat password sementara |

Handler (`handler_test.go`, fake executor, router chi):

| # | Skenario | Harapan |
|---|---|---|
| 1 | Sukses tanpa kredensial | 200, JSON snake_case `{email_sent, email_status, expires_at}`, tidak ada kunci `credentials`, tidak ada header `Cache-Control: no-store` |
| 2 | Sukses dengan kredensial | 200, `Cache-Control: no-store`, `credentials{email, temporary_password, login_url}` berisi nilai asli (bukan `"***"`) |
| 3 | Param diteruskan | `projectID` dan `userID` dari URL apa adanya (`bukan-uuid` tetap sampai ke usecase) |
| 4 | Body diabaikan | body acak tidak memengaruhi hasil, tidak ada 400 |
| 5 | Pemetaan error | `ErrProjectNotFound`/`ErrMemberNotFound` 404, `ErrForbidden` 403, `ErrInvitationNotPending` 409 + `details.reason`, `RATE_LIMITED` 429 + `details.retry_after_seconds` dan `scope`, `ALREADY_REGISTERED` 409, error acak 500 tanpa bocor pesan |
| 6 | Error tidak menambah `no-store` palsu tetapi body error tidak memuat kredensial | cek header/body |

Integrasi (`-tags integration`, `db.TestDB`, bus nyata dengan module auth + management terdaftar, `mailertest.Fake`):

| # | Skenario | Harapan |
|---|---|---|
| 1 | `repository_integration_test`: `FindAccess` dan `ListMembers` lewat `store` | anggota dan role benar; project lain tidak bocor |
| 2 | Owner mengirim ulang ke anggota undangan menunggu | 200 `email_sent=true`; Fake menerima satu email berisi password baru; hash di DB berubah; sesi target dicabut; `expires_at` maju; `resend_count` naik |
| 3 | Kirim ulang ke-4 dalam satu jam (`INVITATION_RESEND_MAX_PER_HOUR=3`) | 4 kali pertama (kuota 3) lolos, ke-4 429 `RATE_LIMITED` scope `resend` |
| 4 | `Fake.Err` di-set / `NotConfigured=true` | 200 `email_sent=false`, `credentials` ada, `Cache-Control: no-store`; baris audit `user.invitation.email_failed` ada dan tanpa password (query `audit_logs` args) |
| 5 | Audit sukses | tepat satu `user.invitation.resent` dengan actor = owner, args `via="project"`, `project_name`; tidak ada aksi `project.*` baru dari endpoint ini |
| 6 | Target sudah menerima undangan (`accepted_at` terisi) | 409 `INVITATION_NOT_PENDING`; tidak ada email, hash tidak berubah, kuota tidak naik |
| 7 | Target anggota akun `createuser` (tanpa baris undangan) | 409 `INVITATION_NOT_PENDING` |
| 8 | Target bukan anggota project ini tapi punya undangan di project lain | 404 `NOT_FOUND`; tidak ada email |

## 7. E2E Scenarios

Memakai server penuh (`cmd/internal/server`), cookie sesi + CSRF seperti `management_scenarios_integration_test.go` (`env.rawDo`), `mailertest.Fake` atau Mailpit bila tersedia.

Positif:

1. (Kriteria 3) Undangan dibuat lalu `expires_at` di-set ke masa lalu lewat SQL; login penerima dengan password lama -> ditolak identik kredensial salah. Owner `POST .../resend` -> 200 `email_sent=true`, `expires_at` baru > now. Penerima login dengan password baru dari email -> sukses (kena `PasswordChangeGate`); login dengan password lama -> ditolak.
2. (Kriteria 9) `MAIL_DRIVER` kosong (noop): owner kirim ulang -> 200 `email_sent=false`, `email_status="not_configured"`, `credentials` lengkap, header `Cache-Control: no-store`. Login dengan `credentials.temporary_password` berhasil. Panggil ulang endpoint apa pun (`GET invitations`, `GET members`, `GET /api/users/{id}`) -> tidak ada yang mengembalikan password itu lagi.
3. (Kriteria 9) SMTP mati (Fake.Err atau Mailpit dihentikan): sama dengan skenario 2 dengan `email_status="failed"`; audit `user.invitation.email_failed` tercatat; log tidak berisi password.
4. super_admin non-anggota project boleh mengirim ulang (200).
5. (Kriteria 3) Kirim ulang ke undangan yang masih berlaku (belum kedaluwarsa) -> 200 dan password lama tidak berlaku lagi.

Negatif:

1. (Kriteria 5) Member (bukan owner) -> 403 `FORBIDDEN`, tidak ada email, tidak ada perubahan DB.
2. (Kriteria 5) Non-anggota project (developer lain) -> 404 `NOT_FOUND` "project tidak ditemukan"; sama persis untuk project yang tidak ada dan `projectID` bukan uuid.
3. Tanpa login -> 401; tanpa CSRF di metode POST -> sesuai middleware yang ada (ditolak).
4. (Kriteria 7) Kirim ulang ke-4 dalam satu jam -> 429 `RATE_LIMITED`, `details.scope="resend"`, `details.retry_after_seconds` > 0 dan <= 3600; setelah jam dimajukan 1 jam -> 200 lagi. Tidak ada email ke-4.
5. Anggota yang sudah menerima undangan (sudah ganti password) -> 409 `CONFLICT` reason `INVITATION_NOT_PENDING`.
6. `userID` bukan anggota project atau bukan uuid -> 404 `NOT_FOUND` "anggota tidak ditemukan".
7. Anggota yatim (user dihapus, `project_members` tersisa) -> 404 `NOT_FOUND`.
8. Anggota yang dinonaktifkan (`is_active=false`) -> 409 `ALREADY_REGISTERED` reason `inactive` diteruskan dari auth, tanpa email.

## 8. Request/Response body

Request: `POST /api/projects/6f2d8d3a-1c1e-4a55-8f40-3d2b6a9e0c11/invitations/7c1f3c2e-5b7a-4d7e-9a51-1f0b8a3e6d21/resend`, tanpa body, cookie sesi owner + header CSRF seperti route mutasi lain.

200, email terkirim (tanpa `credentials` sama sekali, tanpa `no-store`):
```json
{ "email_sent": true, "email_status": "sent", "expires_at": "2026-10-28T09:00:00Z" }
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
`email_status` bernilai `sent`, `failed`, atau `not_configured` (nilai `pending` tidak pernah muncul di endpoint ini). Pada `not_configured` bentuk body sama dengan contoh fallback.

Error (envelope `Error` dari `foundation.yml`):

401:
```json
{ "error": { "code": "UNAUTHORIZED", "message": "perlu login" } }
```
403 (member biasa):
```json
{ "error": { "code": "FORBIDDEN", "message": "akses ditolak" } }
```
404 (non-anggota / project tidak ada):
```json
{ "error": { "code": "NOT_FOUND", "message": "project tidak ditemukan" } }
```
404 (target bukan anggota / bukan uuid / yatim):
```json
{ "error": { "code": "NOT_FOUND", "message": "anggota tidak ditemukan" } }
```
409 (undangan sudah diterima atau tidak ada):
```json
{ "error": { "code": "CONFLICT", "message": "undangan sudah diterima atau tidak ada", "details": { "reason": "INVITATION_NOT_PENDING" } } }
```
409 (akun nonaktif, dari auth):
```json
{ "error": { "code": "ALREADY_REGISTERED", "message": "email sudah terdaftar", "details": { "reason": "inactive" } } }
```
429 (kuota kirim ulang, dari auth):
```json
{ "error": { "code": "RATE_LIMITED", "message": "terlalu banyak permintaan undangan", "details": { "retry_after_seconds": 1840, "scope": "resend" } } }
```
500:
```json
{ "error": { "code": "INTERNAL", "message": "terjadi kesalahan internal" } }
```
Bentuk envelope (termasuk `trace_id` bila ada dan teks pesan 401/500 persis) mengikuti `respond.WriteError` yang ada; contoh menunjukkan isi, bukan menambah field. Pada pesan 409 `INVITATION_NOT_PENDING` bentuk dari management dan dari auth identik karena memakai pesan dan reason yang sama.
