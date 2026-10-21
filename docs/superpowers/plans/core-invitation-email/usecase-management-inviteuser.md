# T12 project/management — Usecase `management-inviteuser` (gelombang C)

Melengkapi `docs/superpowers/plans/core-invitation-email/overview.md` (selanjutnya "overview"). Bila dokumen ini bertentangan dengan overview, overview menang. Hanya satu usecase: `inviteuser` di management. Plan usecase lain tidak disentuh. Pola dasar mengikuti `docs/superpowers/plans/project-management/usecase-addmember.md` (T03) dan kode `addmember/` yang ada.

## Metadata

| Field | Nilai |
|---|---|
| Ticket | T12 (kriteria 1, 4, 5, 9, 10; menyumbang ke 13 lewat audit) |
| Scope | fullstack pada tiket; plan ini backend + kontrak OpenAPI. Dialog UI ada di `frontend-plan.md` |
| Domain | project |
| Module | management |
| Feature | `inviteuser` |
| Endpoint | `/api/projects/{projectID}/invitations` |
| Method | POST |
| Version | tanpa prefix versi di URL (`/api`); naikkan `info.version` minor di `api/modules/project/management.yml` (sekali, bersama plan management lain; jangan naik dua kali) |
| Auth | `RequireAuth()`; izin `PermManageMembers` dicek di usecase (owner project, super_admin). Member 403, non-anggota 404 |
| Header wajib | `X-Requested-With: grimoire` (CSRF T00), `Content-Type: application/json` |
| Respons sukses | 201 `InviteResponse`; `Cache-Control: no-store` hanya bila ada `credentials` |
| Audit | `project.member.invited` (fail-open, setelah commit dan setelah langkah kirim) |
| Bus dikonsumsi | `core.auth.get-user` (Request.Email, lalu Request.UserID), `core.auth.invite-user`, `core.auth.resend-invitation`, `core.audit.record` |
| Tabel / migrasi | menulis `project_members`; mengunci `projects`; tidak ada migrasi |
| Resource root | `backend/internal/domain/project/internal/management` |
| allowedPaths | `.../management/inviteuser/`, `.../management/base/users/users.go` (+ `users_test.go`), `.../management/base.go` (satu baris `inviteuser.Init(d)`), `.../management/management_scenarios_integration_test.go` (tambah skenario), `api/modules/project/management.yml`, `api/openapi.yml` (bundle) |

Prasyarat (bila belum ada, hentikan dan laporkan; jangan dibuat di luar allowedPaths):
- Foundation overview 4.1 sampai 4.10: `errs.CodeAlreadyRegistered` + peta 409, konstanta `BUS_CORE_AUTH_INVITE_USER`, `BUS_CORE_AUTH_RESEND_INVITATION`, kontrak `inviteuser`, `resendinvitation`, `invitation` (konstanta `EmailStatus*`, `Credentials`), kontrak `getuser` aditif (`Request.Email`), aksi `record.ActionProjectMemberInvited`.
- Provider bus `core.auth.invite-user` dan `core.auth.resend-invitation` (plan `auth-inviteuser`, `auth-resendinvitation`, gelombang B) dan provider `getuser` dengan lookup Email (plan `auth-invitation-markers`, gelombang A). Untuk tes unit dan scenario cukup fake bus; provider asli hanya dibutuhkan e2e.
- `base.MemberDTO` dengan field `InvitationPending bool` (`json:"invitation_pending"`) dan `InvitationExpiresAt *time.Time` (`json:"invitation_expires_at"`) dari plan `management-listinvitations` (perubahan base). Bila belum merge saat plan ini dikerjakan, tambahkan kedua field itu persis demikian di `base/dto.go` sebagai commit terpisah dan beri tahu pemilik plan listinvitations (hanya dua field itu; tipe tidak boleh beda).
- Yang sudah ada dari T03 dan dipakai apa adanya: `base.ActorFrom`, `base.NewMember`, `base.FindMember`, `base.Require`, `base.ResolveLevel`, `base.PermManageMembers`, `base.ErrProjectNotFound`, `base.ErrAlreadyMember`, `base.InvalidFields`, `base.FieldErrors`, `base.SystemClock`, `base/store` (`FindAccess`, `LockProject`, `LockMembers`), `base/audit`.

## Summary Plan

`inviteuser` mengundang orang yang belum punya akun (atau yang akunnya masih menunggu undangan) ke project lewat email. Satu usecase, satu `Execute`. Tiga bagian kerja ada di dua module yang berbeda, jadi usecase ini hanya mengurutkan panggilan; rule bisnis ada di `base` (management) dan di auth (lewat bus).

Urutan (overview 3.1):

1. Aktor: `base.ActorFrom`; tanpa identitas -> 401.
2. Pre-check akses: `FindAccess(projectID, actor.ID)`; tidak ketemu -> `ErrProjectNotFound` (404); `Require(ResolveLevel(...), PermManageMembers)` (member 403, non-anggota 404). Semua panggilan bus ke auth terjadi SETELAH ini, sehingga non-anggota tidak bisa memakai endpoint sebagai oracle keberadaan email (anti-tujuan 3).
3. Validasi role lewat entity: `base.NewMember(projectID, "", in.Role, actor, now)` (role `""` -> member; selain `owner`/`member` -> 400 `fields.role`). Sebelum bus supaya request cacat tidak memicu panggilan bus. Role `owner` boleh karena yang sampai sini pasti owner/admin.
4. `users.GetByEmail(email)` (bus `get-user` dengan `Request{Email}`). Bila `Found`: cek keanggotaan pra-tx `FindAccess(projectID, found.ID)`; `memberRole != ""` -> `base.ErrAlreadyMember` (409) sebelum akun apa pun disentuh (tidak ada efek samping). `Found` tapi bukan anggota: lanjut (akun menunggu K6, atau akun terdaftar yang akan ditolak auth dengan `ALREADY_REGISTERED`). Error bus -> dibungkus, 500 (fail-closed).
5. `users.Invite(...)` (bus `core.auth.invite-user`) dengan `Deliver:false`, `InvitedBy=actor.ID`, `InvitedByLabel=actor.Label`, `ProjectName=project.Name`, `Email`, `Name`. `ALREADY_REGISTERED`, `RATE_LIMITED`, `INVALID_ARGUMENT` diteruskan apa adanya (`*errs.Error` tetap terbaca lewat `errors.As` karena wrapper memakai `%w`). Hasil: `UserID`, `Created`, `ExpiresAt`.
6. Transaksi management (bus DILARANG di dalam): `LockProject` (hilang -> 404) -> `LockMembers` -> hitung ulang level aktor dari daftar terkunci lalu `Require(PermManageMembers)` lagi -> target sudah di daftar terkunci -> `ErrAlreadyMember` -> `InsertMember` (`ON CONFLICT DO NOTHING`; 0 baris -> `ErrAlreadyMember`) dengan `added_by=actor.ID`, `added_by_label=actor.Label`.
7. Setelah commit: `users.ResendInvitation({UserID, InvitedBy, InvitedByLabel, ProjectName, Initial: Created})`. Ini menerbitkan password sementara, mengirim email, dan (di sisi auth) mengaudit kegagalan kirim. `Initial=Created` berarti penerbitan pertama untuk akun baru: tanpa kuota resend dan tanpa audit `user.invitation.resent`. Untuk akun menunggu (K6, `Created=false`) langkah ini dihitung sebagai kirim ulang (kuota 3/jam berlaku; 429 `scope=resend` dari langkah ini ditangani seperti kegagalan langkah 7, lihat di bawah).
8. Bila langkah 7 mengembalikan error (bus/DB/rate limit/undangan sudah diterima): keanggotaan SUDAH tercatat, jadi usecase TIDAK gagal. Log `ErrorContext` ("invite: terbit/kirim gagal setelah anggota tercatat", `project_id`, `user_id`, `err`; tanpa email dan tanpa kredensial) lalu hasilkan `email_sent=false`, `email_status="failed"`, tanpa `credentials`, `expires_at` dari langkah 5. Owner memakai "Kirim ulang" dari daftar undangan (anti-tujuan 4).
9. Ambil data tampilan anggota: `users.Get(UserID)` (bus `get-user` by id) setelah commit. Error di sini TIDAK menggagalkan request; fallback ke `Found=true, ID, Email=<email ternormalisasi>, Name=in.Name (kosong -> pointer nil), Role=developer, IsActive=true` dan log warning. Alasan: `inviteuser.Response` tidak membawa nama, dan untuk akun menunggu (K6) nama yang tampil adalah nama akun yang sudah ada, bukan `in.Name`.
10. Audit `project.member.invited` lewat `audit.Recorder` (fail-open): `Args = {user_id, user_label (email), role, created}`. Tanpa `credentials`, tanpa password, tanpa `email_status`. Tepat satu entri per undangan sukses (termasuk kasus email gagal); 400/403/404/409/429/500 -> 0 entri.
11. Respons 201 `InviteResponse` `{member, created, email_sent, email_status, expires_at, credentials?}`. `member` = `MemberDTO` dari `draft.ToDTO(target)` lalu `InvitationPending=true`, `InvitationExpiresAt=&expires_at`. `credentials` hanya diisi bila langkah 7 sukses dan `Credentials != nil`. Handler menambah `Cache-Control: no-store` hanya bila `credentials != nil`.

Keputusan penting:
- Tidak ada bus di dalam tx, dan audit/kirim setelah commit (overview 3.1, fakta 3). Diuji unit dengan flag `inTx`.
- Langkah 5 sebelum 6 (bukan sesudah): memastikan `ALREADY_REGISTERED`/`RATE_LIMITED`/`INVALID_ARGUMENT` ditolak sebelum keanggotaan tercatat. Konsekuensi diterima (overview 3.1): bila tx langkah 6 gagal (race `ALREADY_MEMBER`), akun menunggu tanpa password yang diketahui siapa pun tetap ada; dipakai ulang lewat K6 atau dihapus purge.
- Pre-cek keanggotaan di langkah 4 hanya optimasi dan penjaga efek samping; kebenaran akhir ada di tx langkah 6.
- Normalisasi email di management: `strings.ToLower(strings.TrimSpace(...))` hanya untuk `get-user` dan label audit; auth melakukan normalisasi dan validasi format yang otoritatif (nilai yang dikirim ke `invite-user` adalah email ternormalisasi yang sama).
- Pengundang non-aktif tidak mungkin sampai sini (sesi tidak resolve). Tidak ada cek tambahan.
- Race kecil yang diterima: undangan K6 diterima (penerima ganti password) di antara langkah 5 dan 7 -> `resend` mengembalikan `CONFLICT INVITATION_NOT_PENDING`; diperlakukan sebagai kegagalan langkah 7 (201 `failed`, tanpa kredensial). Keanggotaan tetap benar, dan karena akunnya sudah aktif dengan password sendiri, tidak ada yang perlu dikirim; di daftar undangan ia tidak muncul (sudah diterima).
- Dua hash argon2 di jalur ini diterima (overview risiko).
- Tidak idempotent: pengulangan untuk email yang sudah jadi anggota -> 409.

## Plan

Urutan wajib. Tes dulu (TDD), lalu implementasi.

**M0. Verifikasi (tanpa edit).** Baca `app.Deps`, `db.DB.BeginTx`, `respond.DecodeJSON/WriteJSON/WriteError`, `middleware.RequireAuth/KeyByIP`, `bus.CallTyped`, dan seluruh `addmember/` sebagai templat. Pastikan simbol prasyarat ada. Pastikan `respond.WriteJSON` mempertahankan header yang di-set sebelumnya (pola `resetpassword/handler.go` baris 65: `w.Header().Set("Cache-Control", "no-store")` sebelum tulis).

**M1. `base/users/users.go` (tambah tiga method; `users_test.go` diperluas).**

```go
type Lookup interface {
    Get(ctx context.Context, userID string) (getuser.Response, error)                 // ada
    FindDevelopers(ctx context.Context, q string, limit int) ([]finddevelopers.Developer, error) // ada
    GetByEmail(ctx context.Context, email string) (getuser.Response, error)
    Invite(ctx context.Context, in inviteuser.Request) (inviteuser.Response, error)
    ResendInvitation(ctx context.Context, in resendinvitation.Request) (resendinvitation.Response, error)
}
```

Implementasi di `BusLookup` (pola sama dengan `Get`):
- `GetByEmail`: `bus.CallTyped[getuser.Response](ctx, b, constants.BUS_CORE_AUTH_GET_USER, getuser.Request{Email: email})`; error `fmt.Errorf("users: get-user by email: %w", err)`.
- `Invite`: `constants.BUS_CORE_AUTH_INVITE_USER`; error `fmt.Errorf("users: invite-user: %w", err)`.
- `ResendInvitation`: `constants.BUS_CORE_AUTH_RESEND_INVITATION`; error `fmt.Errorf("users: resend-invitation: %w", err)`.
- Pembungkus selalu `%w` agar `errors.As(err, &*errs.Error)` dan `errors.Is(err, bus.ErrNotRegistered)` tetap jalan; `respond.WriteError` memakai `errors.As` sehingga `ALREADY_REGISTERED`/`RATE_LIMITED` sampai ke klien dengan kode dan details aslinya.
- Method ini juga dipakai plan `management-resendinvitation` dan `management-revokeinvitation`; tulis dan commit lebih dulu secara terpisah (overview 9, gelombang C). Fake `Lookup` di test feature lain (`addmember`, `searchmembercandidates`, dst.) yang mengimplementasikan interface harus ditambah tiga method stub (return zero value). Cari dengan `grep -rn "FindDevelopers(context.Context" backend/internal/domain/project` dan perbarui hanya fake di tes (jangan menyentuh logika feature lain).

**M2. `inviteuser/params.go`**

```go
const maxEmailLen = 254

type InviteRequest struct {
    Email string `json:"email"`
    Role  string `json:"role"` // opsional; "" -> member
    Name  string `json:"name"` // opsional; default dari auth (bagian lokal email)
}

// Validate hanya bentuk. Format email penuh dan panjang nama divalidasi auth.
func (r *InviteRequest) Validate() error {
    r.Email = strings.ToLower(strings.TrimSpace(r.Email))
    r.Name = strings.TrimSpace(r.Name)
    fe := base.FieldErrors{}
    switch {
    case r.Email == "":
        fe["email"] = "wajib diisi"
    case utf8.RuneCountInString(r.Email) > maxEmailLen:
        fe["email"] = "maksimal 254 karakter"
    case strings.ContainsAny(r.Email, " \t\r\n"):
        fe["email"] = "format email tidak valid"
    }
    if len(fe) > 0 { return base.InvalidFields(fe) }
    return nil
}
```

Response DTO (di `params.go`, bukan di `base`; hanya feature ini yang memakainya):

```go
type CredentialsDTO struct {
    Email             string `json:"email"`
    TemporaryPassword string `json:"temporary_password"` // string biasa; JANGAN base.Secret (marshal "***")
    LoginURL          string `json:"login_url"`
}
type InviteResponse struct {
    Member      base.MemberDTO  `json:"member"`
    Created     bool            `json:"created"`
    EmailSent   bool            `json:"email_sent"`
    EmailStatus string          `json:"email_status"`
    ExpiresAt   time.Time       `json:"expires_at"`
    Credentials *CredentialsDTO `json:"credentials,omitempty"`
}
```
`DecodeJSON` menolak field tak dikenal, body kosong, JSON rusak, dan tipe salah -> 400.

**M3. `inviteuser/usecase.go`**

```go
type Tx interface {
    LockProject(ctx context.Context, id string) (base.Project, bool, error)
    LockMembers(ctx context.Context, projectID string) ([]base.Member, error)
    InsertMember(ctx context.Context, m base.Member) (inserted bool, err error) // ON CONFLICT DO NOTHING
}
type Repository interface {
    FindAccess(ctx context.Context, projectID, userID string) (p base.Project, memberRole string, found bool, err error)
    InTx(ctx context.Context, fn func(Tx) error) error
}

type Usecase struct {
    repo  Repository
    users users.Lookup
    audit audit.Recorder
    clock base.Clock
    log   *slog.Logger
}
func NewUsecase(r Repository, u users.Lookup, a audit.Recorder, c base.Clock, l *slog.Logger) *Usecase
func (u *Usecase) Execute(ctx context.Context, projectID string, in InviteRequest, meta base.RequestMeta) (InviteResponse, error)
```

Alur `Execute` mengikuti Summary langkah 1 sampai 11 persis. Detail pembungkusan error:
- find access: `fmt.Errorf("inviteuser: find access: %w", err)`.
- pre-check anggota target: `fmt.Errorf("inviteuser: find target access: %w", err)`. `FindAccess(projectID, found.ID)` mengembalikan `found=false` hanya bila project hilang -> `ErrProjectNotFound`.
- get by email: `fmt.Errorf("inviteuser: get user by email: %w", err)`.
- invite: kembalikan `err` apa adanya bila `errors.As(err, new(*errs.Error))`; selain itu `fmt.Errorf("inviteuser: invite: %w", err)` (keduanya tetap terbaca `errors.As`; pembungkusan `%w` aman, tidak perlu cabang khusus, cukup satu `fmt.Errorf("inviteuser: invite: %w", err)`).
- tx: `lock project`, `lock members`, `insert member` dibungkus; `ErrProjectNotFound`, `ErrForbidden`, `ErrAlreadyMember` dikembalikan apa adanya.
- `draft.UserID = invited.UserID` setelah langkah 5 (bentuk kanonik dari auth, bukan input klien).
- Rule bisnis TIDAK di usecase: role di `base.NewMember`, izin di `base.Require`, keanggotaan di `base.FindMember`. Usecase hanya orkestrasi, tanpa branching bisnis baru selain pemetaan hasil kirim ke respons.
- Pemetaan hasil: `sendOK := (resendErr == nil)`. `emailSent=resp.EmailSent`, `emailStatus=resp.EmailStatus`, `expiresAt=resp.ExpiresAt`, `creds=resp.Credentials` bila sukses; jika error: `emailSent=false`, `emailStatus=invitation.EmailStatusFailed`, `expiresAt=invited.ExpiresAt`, `creds=nil`. `Credentials` TIDAK pernah di-log, diaudit, atau disimpan di variabel yang masuk `slog`.

**M4. `inviteuser/repository.go`**

Satu-satunya yang menyentuh SQL feature ini. `FindAccess`, `LockProject`, `LockMembers` didelegasikan ke `base/store`. `InsertMember` memakai SQL yang sama dengan `addmember` (disalin; feature tidak boleh meng-import feature lain, rule 4.3). Bila kelak dipindah ke `base/store`, itu refactor terpisah yang menyentuh `addmember`, di luar plan ini.

```sql
INSERT INTO project_members (project_id, user_id, role, added_by, added_by_label, added_at)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (project_id, user_id) DO NOTHING
```
`inserted = RowsAffected() == 1`. Struktur `Repo`/`txRepo`/`InTx` sama dengan `addmember/repository.go`.

**M5. `inviteuser/handler.go`**

```go
type executor interface {
    Execute(ctx context.Context, projectID string, in InviteRequest, meta base.RequestMeta) (InviteResponse, error)
}
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    var in InviteRequest
    if err := respond.DecodeJSON(r, &in); err != nil { respond.WriteError(w, r, err); return }
    if err := in.Validate(); err != nil { respond.WriteError(w, r, err); return }
    meta := base.RequestMeta{IP: middleware.KeyByIP(r), UserAgent: truncate(r.UserAgent(), 512)}
    res, err := h.uc.Execute(r.Context(), chi.URLParam(r, "projectID"), in, meta)
    if err != nil { respond.WriteError(w, r, err); return }
    if res.Credentials != nil { w.Header().Set("Cache-Control", "no-store") }
    respond.WriteJSON(w, http.StatusCreated, res)
}
```
`truncate` disalin kecil seperti di `addmember/handler.go` (belum ada helper bersama di `base`; bila sudah ada saat implement, pakai itu dan jangan buat salinan). Handler tidak me-log body atau respons. Validasi bentuk terjadi sebelum otorisasi dan jawabannya tidak bergantung pada project/email, jadi tidak membocorkan apa pun.

**M6. `inviteuser/init.go`**

```go
func Init(d app.Deps) {
    s := store.New(d.DB)
    uc := NewUsecase(newRepo(d.DB, s), users.New(d.Bus), audit.New(d.Bus, d.Log), base.SystemClock, d.Log)
    d.API.With(middleware.RequireAuth()).Post("/projects/{projectID}/invitations", newHandler(uc).ServeHTTP)
}
```
Param `{projectID}` (README). Route GET `/projects/{projectID}/invitations` milik `listinvitations` memakai pola sama (method berbeda, tidak bentrok). Tambah satu baris `inviteuser.Init(d)` di `management/base.go` (`Register`); jangan menyentuh baris lain.

**M7. OpenAPI (`api/modules/project/management.yml`).** Tambah operasi `inviteToProject` pada path `/projects/{projectID}/invitations` (method post; path yang sama akan dipakai get oleh plan listinvitations, jadi gabungkan di satu path item, jangan duplikasi key), tag `project-members`, parameter `XRequestedWith` + `ProjectID`, request `InviteMemberRequest` (`email` wajib, `role` enum `[owner, member]` default `member`, `name` opsional maks 100), respons 201 `InviteMemberResponse` (header `Cache-Control` dijelaskan: `no-store` bila `credentials` ada), skema `InvitationCredentials` dirujuk dari `core/auth.yml` (foundation 4.10) via `$ref` bila lintas file diizinkan oleh bundler repo, selain itu definisikan `InvitationCredentialsInline` lokal. Error `BadRequest/Unauthorized/Forbidden/NotFound/Conflict/TooManyRequests/Internal` dari `../infra/foundation.yml`; deskripsi 409 menyebut `ALREADY_MEMBER` (code `CONFLICT`) dan `ALREADY_REGISTERED` (code `ALREADY_REGISTERED`, details.reason `registered|inactive|not_developer`), 429 menyebut `details.scope` `invite|resend`. Skema `Member` mendapat `invitation_pending`, `invitation_expires_at` hanya bila belum ditambah plan listinvitations (cek dulu, jangan duplikasi). Lalu `make api-bundle`, commit `api/openapi.yml`, `make api-check` lulus.

**M8. Verifikasi akhir:** `go vet ./...`, `go test ./internal/domain/project/...`, `go test -tags integration ./internal/domain/project/...`, archtest (`inviteuser` hanya meng-import `base`, `base/store`, `base/users`, `base/audit`, infra, lib; tidak feature lain; tidak meng-import `core/auth`), `make api-check`.

## Migration

Tidak ada. Memakai `projects` dan `project_members` dari `backend/migrations/project/20261022090000000_create_projects.sql` (PK `(project_id, user_id)` menjadi dasar `ON CONFLICT`; CHECK `project_members_role_check` jaring pengaman). Tabel `user_invitations` milik auth (migrasi core foundation overview bagian 5) dan tidak disentuh dari management.

## Changes

Baru (semua di `backend/internal/domain/project/internal/management/inviteuser/`):
- `init.go`, `handler.go`, `params.go`, `usecase.go`, `repository.go`
- `params_test.go`, `usecase_test.go`, `handler_test.go`, `repository_integration_test.go` (tag `integration`)

Ubah:
- `backend/internal/domain/project/internal/management/base/users/users.go`: interface `Lookup` + `BusLookup` mendapat `GetByEmail`, `Invite`, `ResendInvitation`; `users_test.go` menguji ketiganya (endpoint, request diteruskan persis, error terbungkus `%w`, `*errs.Error` tetap terbaca, `ErrNotRegistered` terbaca).
- Fake `users.Lookup` di tes feature lain (stub tiga method) agar tetap compile.
- `backend/internal/domain/project/internal/management/base.go`: `inviteuser.Init(d)`.
- `backend/internal/domain/project/internal/management/base/dto.go`: HANYA bila dua field `invitation_*` belum ada (lihat prasyarat).
- `backend/internal/domain/project/internal/management/management_scenarios_integration_test.go`: skenario lintas module (bagian Test Scenario).
- `api/modules/project/management.yml`, `api/openapi.yml`.

Tidak diubah: `lib/constants/bus.go`, `lib/contracts/**`, migrasi, `internal/infra/**`, `go.mod`, usecase management lain, seluruh `core/auth`.

## Test Scenario

Penamaan: `TestT12_AC<n>_...` untuk kriteria penerimaan; lainnya `Test<Fungsi>_<Kasus>`.

Params (`params_test.go`):
- Email kosong / spasi saja -> `fields.email="wajib diisi"`; `" Baru@Contoh.ID "` -> `baru@contoh.id`; 255 rune -> `fields.email`; email berisi spasi tengah -> `fields.email`; `role` dan `name` tidak ditolak di sini; `name` di-trim.

Unit usecase (`usecase_test.go`; repo palsu mencatat urutan dan flag `inTx`, users palsu mencatat urutan dan `inTx` serta argumen, audit palsu, clock tetap, logger ke buffer):
- Sukses akun baru, aktor owner: urutan persis `find` -> `getByEmail` -> `invite` -> `intx` -> `resend` -> `get` (by id) -> `audit`. Tidak ada `getByEmail/invite/resend/get/audit` yang terjadi saat `inTx==true`. Argumen `invite`: `Deliver=false`, `InvitedBy=actor.ID`, `InvitedByLabel=actor.Label`, `ProjectName=nama project`, `Email` ternormalisasi, `Name` diteruskan. Argumen `resend`: `Initial=true` (karena `Created=true`), `UserID=invited.UserID`. Member yang di-insert: `Role=member`, `UserID=invited.UserID`, `AddedBy=actor.ID`, `AddedByLabel=email aktor`, `AddedAt=clock`.
- Hasil: `email_sent=true`, `email_status=sent`, `Credentials==nil`; `member.invitation_pending=true`, `invitation_expires_at=expires`.
- `TestT12_AC1_...`: seluruh `InviteResponse` di-marshal ke JSON, string tidak memuat `temporary_password`/`credentials` dan tidak memuat password palsu yang dikembalikan fake auth di `Credentials` saat sukses; argumen audit dan buffer log tidak memuat password.
- `TestT12_AC4_AlreadyMember`: `getByEmail` Found dan `FindAccess(target)` menunjukkan anggota -> `ErrAlreadyMember`; `invite`, `intx`, `resend`, audit tidak dipanggil.
- `TestT12_AC4_AlreadyRegistered`: `invite` mengembalikan `ErrAlreadyRegistered("registered")` (bungkus `%w` dari wrapper) -> error diteruskan, `errors.As` menghasilkan kode `ALREADY_REGISTERED` dan details `reason`; `intx`, `resend`, audit tidak dipanggil. Ulangi untuk `inactive` dan `not_developer`.
- `TestT12_AC4_PendingAccountReused` (K6): `getByEmail` Found non-anggota, `invite` -> `Created=false` -> `intx` insert -> `resend` dengan `Initial=false` -> respons `created=false`; tidak ada pembuatan akun ganda (satu panggilan `invite`).
- `RATE_LIMITED` dari `invite` (scope `invite`) -> diteruskan; tidak ada insert/audit. `INVALID_ARGUMENT` dari `invite` (fields.email) -> diteruskan.
- `TestT12_AC5_MemberForbidden`: aktor `member` -> `ErrForbidden`; tidak ada panggilan bus sama sekali (`getByEmail`=0) dan `intx`=0, audit 0. Aktor non-anggota -> `ErrProjectNotFound`, `getByEmail`=0. Project hilang -> 404. Super_admin non-anggota lolos.
- Role `"admin"`/`"OWNER"` -> `fields.role`; `getByEmail`=0 (validasi sebelum bus). Role `""` -> member. Role `owner` oleh owner -> member di-insert dengan role owner.
- `TestT12_AC9_SendFailsFallbackCredentials`: `resend` sukses dengan `EmailSent=false`, `EmailStatus=failed|not_configured`, `Credentials` terisi -> respons meneruskan `credentials` (email, temporary_password, login_url) dan `email_sent=false`; audit tetap 1 entri tanpa password.
- `TestT12_AC9_ResendBusErrorDegrades`: `resend` mengembalikan error biasa -> `Execute` TIDAK error; respons `email_sent=false`, `email_status=failed`, `credentials==nil`, `expires_at` dari `invite`; keanggotaan sudah di-insert; log error ada (berisi `project_id`, `user_id`, tanpa email/password); audit 1 entri. Ulangi untuk `bus.ErrNotRegistered`, `RATE_LIMITED` (K6), `CONFLICT INVITATION_NOT_PENDING`.
- `TestT12_AC10_NoCredentialsOnSuccess`: `Credentials` dari fake non-nil tetapi `EmailSent=true` (kontrak tidak mungkin, tetap diuji defensif): usecase meneruskan `Credentials` hanya bila `!EmailSent`. Implementasi: `if resp.EmailSent { creds=nil }`.
- `get` by id gagal/`Found=false` -> tetap sukses, DTO fallback (`Email` ternormalisasi, `Name` dari input atau nil), log warning; audit tetap 1.
- Tx: `LockProject ok=false` -> `ErrProjectNotFound` (setelah `invite` sudah dipanggil; `resend` dan audit tidak dipanggil); hitung ulang level: pre-check owner, daftar terkunci menunjukkan aktor `member` -> `ErrForbidden`, tidak ada insert/resend/audit; target sudah di daftar terkunci -> `ErrAlreadyMember`, `InsertMember` tidak dipanggil; `inserted=false` -> `ErrAlreadyMember`; error `LockMembers`/`InsertMember` -> terbungkus (bukan `*errs.Error`), `resend` dan audit tidak dipanggil.
- Error `getByEmail` (termasuk `ErrNotRegistered`) -> terbungkus, 500, `invite`=0.
- `user_id` yang disimpan dan Target audit = `invited.UserID` (bukan input klien).
- Audit: tepat satu event `Action=project.member.invited`, `ProjectID`, `ProjectLabel`, `TargetType="project_member"`, `Target=UserID`, `Args=={user_id, user_label: email, role, created}` (tepat empat kunci), `Meta` diteruskan.
- Tanpa identitas -> `CodeUnauthorized`.

Handler (`handler_test.go`, httptest):
- 201 + JSON sesuai contoh (snake_case, RFC3339 UTC); tanpa `credentials` -> kunci `credentials` tidak ada di JSON dan header `Cache-Control` tidak diset `no-store` (kriteria 10).
- Dengan `credentials` -> `Cache-Control: no-store` dan kunci `credentials` berisi tiga field.
- Body kosong / JSON rusak / field tak dikenal (`{"emailAddress":"x"}`) / `email` bertipe angka -> 400; email kosong -> 400 `details.fields.email`.
- Pemetaan: `ErrForbidden` -> 403; `ErrProjectNotFound` -> 404; `ErrAlreadyMember` -> 409 `CONFLICT` reason `ALREADY_MEMBER`; `ErrAlreadyRegistered` terbungkus -> 409 `ALREADY_REGISTERED`; `ErrInviteRateLimited` -> 429 `RATE_LIMITED` dengan `retry_after_seconds`; error biasa -> 500 generik tanpa bocor pesan; `trace_id` ada.
- `projectID` dari `chi.URLParam` diteruskan apa adanya (router chi sungguhan); `UserAgent` > 512 rune dipotong; `IP` dari `KeyByIP`.
- Handler tidak menulis body request ke log (cek buffer logger di handler test).

Repository integrasi (`repository_integration_test.go`, Postgres nyata, `db.TestDB(t)`):
- `InsertMember` sukses: baris dengan `added_by`, `added_by_label`, `added_at` benar; `inserted=true`.
- Insert kedua `(project,user)` sama -> `inserted=false`, satu baris.
- Role `"admin"` melanggar CHECK -> error; project tidak ada -> pelanggaran FK.
- `LockProject` di tx pertama memblokir tx kedua sampai commit (dua goroutine): tx kedua melihat anggota baru -> `ErrAlreadyMember`.

Integrasi lintas module (`management_scenarios_integration_test.go`, fake bus `get-user` (Email dan UserID), `invite-user`, `resend-invitation`, `audit.record` lewat `registerIfAbsent`; fake mencatat panggilan dan memeriksa bahwa tidak ada panggilan datang saat koneksi tx management terbuka, dengan menghitung `pg_stat_activity` state `idle in transaction` atau flag yang dikunci mutex dari tx hook):
- `TestT12_AC1_OwnerInvitesNewEmail`: owner A `POST invitations {email, role}` -> 201; satu baris `project_members` (role, `added_by`=A, `added_by_label`=email A); fake `invite-user` dipanggil sekali dengan `Deliver=false`, `resend-invitation` sekali dengan `Initial=true` setelah baris anggota ada (fake memverifikasi dengan query ke DB saat dipanggil); audit tepat satu `project.member.invited` dengan `args` empat kunci dan `actor_label` email A; body respons dan args audit tidak mengandung password.
- `TestT12_AC4_AlreadyMemberRejected`: anggota B diundang lagi memakai emailnya -> 409 `ALREADY_MEMBER`; fake `invite-user` dipanggil 0 kali; `project_members` tetap; audit 0.
- `TestT12_AC4_AlreadyRegistered`: fake `invite-user` mengembalikan `ALREADY_REGISTERED` -> 409 `ALREADY_REGISTERED`; tidak ada baris `project_members` baru; audit 0.
- `TestT12_AC4_PendingAccount`: fake `get-user` Email mengembalikan akun menunggu, `invite-user` `Created=false` -> 201 `created=false`; `resend-invitation` `Initial=false`; satu baris anggota.
- `TestT12_AC5_MemberForbidden_NonMember404`: member B -> 403; non-anggota C -> 404 dengan body identik 404 "project tidak ditemukan"; fake `get-user`, `invite-user`, `resend-invitation` dipanggil 0 kali (tidak bisa jadi oracle); tidak ada baris baru, audit 0.
- `TestT12_AC9_ResendFailsStillMember`: `resend-invitation` mengembalikan error -> 201 `email_sent=false`, `email_status=failed`, tanpa `credentials`; baris anggota ada; audit 1 entri; request lain (resend dari plan sebelah) tidak diperlukan.
- `TestT12_AC9_FallbackCredentials`: `resend-invitation` -> `EmailSent=false`, `Credentials` terisi -> 201 dengan `credentials` dan `Cache-Control: no-store`; audit dan log buffer tidak memuat password; GET `members` setelahnya tidak memuat kredensial.
- `TestT12_AC10_SuccessHasNoCredentialsKey`: JSON respons mentah tidak memuat substring `credentials` maupun `temporary_password`.
- `TestT12_Invite_ConcurrentSameEmail`: dua owner mengundang email yang sama bersamaan (20 putaran, project baru; fake `invite-user` idempotent per email dengan `Created=true` untuk panggilan pertama): tiap putaran satu 201 dan satu 409 `ALREADY_MEMBER` (bisa terdeteksi di pra-cek atau di tx); satu baris anggota; satu entri audit.
- `TestT12_Invite_ActorDemotedRace`: A dan C owner; A menurunkan C ke member bersamaan dengan C mengundang. Bila demote lebih dulu -> C 403 dan tidak ada baris anggota baru; tidak ada keadaan di mana C (sudah member) berhasil mengundang. Catat: `invite-user` mungkin sudah membuat akun menunggu (diterima, overview 3.1).
- Tidak ada panggilan bus di dalam tx: fake handler `get-user`/`invite-user`/`resend-invitation`/`audit.record` mencoba `SELECT ... FOR UPDATE NOWAIT` pada baris project; harus berhasil (tx management sudah selesai atau belum mulai), membuktikan tidak ada lock yang masih terbuka saat panggilan bus.
- CSRF: tanpa `X-Requested-With` -> 403 `CSRF_REQUIRED`; tanpa sesi -> 401.
- `TestRoutesRegistered` memuat `POST /api/projects/{projectID}/invitations`.
- Audit fail-open: `record` gagal -> respons tetap 201.

## E2E Scenarios

Prasyarat: auth provider asli dan Mailpit (`MAIL_DRIVER=smtp`, `SMTP_TLS=none`, host `localhost:1025`) menyala; UI dari `frontend-plan.md`.

Positif:
1. (Kriteria 1) Owner membuka Members -> "Tambah anggota" -> mengetik `baru@contoh.id` (tidak ada hasil) -> "Undang `baru@contoh.id` lewat email", role Member -> kirim. Respons 201 `email_sent=true`, tanpa `credentials`. Anggota baru muncul dengan penanda "belum menerima undangan". Mailpit menerima satu email berisi URL login dan password sementara. DB hanya berisi hash, log aplikasi dan audit tanpa password; audit memuat `project.member.invited`.
2. (Kriteria 4) Mengundang email yang akunnya masih menunggu (pengguna dari skenario 1 dihapus keanggotaannya via revoke, lalu diundang ulang ke project lain milik owner) -> 201 `created=false`, tidak ada akun ganda, password baru berlaku, email kedua di Mailpit.
3. (Kriteria 9) Hentikan Mailpit, undang email baru -> 201 `email_sent=false`, `email_status=failed`, dan (karena auth tetap mengembalikan kredensial untuk kegagalan kirim) `credentials` tampil di dialog sekali tampil dengan `Cache-Control: no-store`; tutup dialog lalu buka lagi -> kredensial tidak bisa diambil ulang (tidak ada endpoint baca). Audit `user.invitation.email_failed` tanpa password.
4. (Kriteria 9) Jalankan app dengan `MAIL_DRIVER` kosong -> undangan 201 `email_sent=false`, `email_status=not_configured`, `credentials` ada; dialog menampilkan keterangan "email belum diatur".
5. Super_admin non-anggota mengundang ke project dengan role owner -> 201, anggota owner tercatat.

Negatif:
1. (Kriteria 4) Email milik akun terdaftar (diterima atau dibuat `createuser`) -> 409 `ALREADY_REGISTERED`; UI mengarahkan memakai pencarian biasa; tidak ada anggota baru.
2. (Kriteria 4) Email yang sudah anggota project -> 409 `CONFLICT` `ALREADY_MEMBER`.
3. (Kriteria 5) Member biasa memanggil POST langsung -> 403; developer non-anggota -> 404 seragam; tombol undang tidak tampil bagi member.
4. Undangan ke-21 dalam satu jam oleh pengundang yang sama -> 429 `RATE_LIMITED` `scope=invite`; tidak ada anggota baru.
5. Email tidak valid (`bukan-email`) -> 400 `fields.email` dari auth; email kosong -> 400 dari handler; `role: "admin"` -> 400 `fields.role`; field tak dikenal -> 400.
6. Tanpa `X-Requested-With` -> 403 `CSRF_REQUIRED`; tanpa login -> 401.
7. Klik kirim dua kali cepat: UI menonaktifkan tombol; bila tetap dua request, yang kedua 409 `ALREADY_MEMBER`, satu anggota, satu entri audit.
8. Langkah kirim gagal total (auth mati sesaat setelah invite, disimulasikan di tes scenario): 201 `email_sent=false`, `failed`, tanpa `credentials`; daftar undangan menampilkan "email gagal terkirim" dan "Kirim ulang" (plan `management-resendinvitation`).

## Request/Response body

Format error seragam (envelope `Error` dari `api/modules/infra/foundation.yml`):

```json
{ "error": { "code": "NOT_FOUND", "message": "project tidak ditemukan", "details": null }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

### POST /api/projects/{projectID}/invitations

Header: `X-Requested-With: grimoire`, `Content-Type: application/json`, cookie sesi.

Request lengkap (`role` dan `name` opsional):
```json
{ "email": "baru@contoh.id", "role": "member", "name": "Budi" }
```

Request minimal:
```json
{ "email": "baru@contoh.id" }
```

201, email terkirim (tidak ada kunci `credentials`, tanpa `Cache-Control: no-store`):
```json
{
  "member": {
    "user_id": "7c1f3c2e-5b7a-4d7e-9a51-1f0b8a3e6d21",
    "name": "Budi",
    "email": "baru@contoh.id",
    "is_active": true,
    "found": true,
    "role": "member",
    "added_at": "2026-10-21T08:15:00Z",
    "added_by": "0192f5a0-aaaa-7aaa-8bbb-00000000000a",
    "added_by_label": "owner@contoh.id",
    "invitation_pending": true,
    "invitation_expires_at": "2026-10-28T08:15:00Z"
  },
  "created": true,
  "email_sent": true,
  "email_status": "sent",
  "expires_at": "2026-10-28T08:15:00Z"
}
```

201, fallback (header `Cache-Control: no-store`; `email_status` bernilai `failed` atau `not_configured`):
```json
{
  "member": {
    "user_id": "7c1f3c2e-5b7a-4d7e-9a51-1f0b8a3e6d21",
    "name": "baru",
    "email": "baru@contoh.id",
    "is_active": true,
    "found": true,
    "role": "member",
    "added_at": "2026-10-21T08:15:00Z",
    "added_by": "0192f5a0-aaaa-7aaa-8bbb-00000000000a",
    "added_by_label": "owner@contoh.id",
    "invitation_pending": true,
    "invitation_expires_at": "2026-10-28T08:15:00Z"
  },
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

201, langkah terbit/kirim gagal total (anggota tetap tercatat; tanpa `credentials`; tanpa `no-store`; dipulihkan lewat "Kirim ulang"):
```json
{
  "member": {
    "user_id": "7c1f3c2e-5b7a-4d7e-9a51-1f0b8a3e6d21",
    "name": "Budi",
    "email": "baru@contoh.id",
    "is_active": true,
    "found": true,
    "role": "member",
    "added_at": "2026-10-21T08:15:00Z",
    "added_by": "0192f5a0-aaaa-7aaa-8bbb-00000000000a",
    "added_by_label": "owner@contoh.id",
    "invitation_pending": true,
    "invitation_expires_at": "2026-10-28T08:15:00Z"
  },
  "created": true,
  "email_sent": false,
  "email_status": "failed",
  "expires_at": "2026-10-28T08:15:00Z"
}
```

400, email kosong atau format tidak valid (format penuh dari auth, diteruskan):
```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "permintaan tidak valid", "details": { "fields": { "email": "format email tidak valid" } } }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

400, role tidak valid:
```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "permintaan tidak valid", "details": { "fields": { "role": "role harus owner atau member" } } }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

400, field tak dikenal atau JSON rusak: 400 `INVALID_ARGUMENT` dari `respond.DecodeJSON`.

401 (tanpa sesi):
```json
{ "error": { "code": "UNAUTHENTICATED", "message": "perlu login", "details": null }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

403, aktor member (bukan owner/admin):
```json
{ "error": { "code": "FORBIDDEN", "message": "akses ditolak", "details": null }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

403, header CSRF hilang (tes memeriksa `code`, bukan teks):
```json
{ "error": { "code": "CSRF_REQUIRED", "message": "header X-Requested-With wajib", "details": null }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

404, project tidak ada, bukan uuid, atau aktor bukan anggota (seragam):
```json
{ "error": { "code": "NOT_FOUND", "message": "project tidak ditemukan", "details": null }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

409, sudah anggota:
```json
{ "error": { "code": "CONFLICT", "message": "user sudah menjadi anggota", "details": { "reason": "ALREADY_MEMBER" } }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

409, email sudah terdaftar (diteruskan dari auth; `reason` bernilai `registered`, `inactive`, atau `not_developer`):
```json
{ "error": { "code": "ALREADY_REGISTERED", "message": "email sudah terdaftar", "details": { "reason": "registered" } }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

429, kuota undangan per pengundang (diteruskan dari auth; `scope` bernilai `invite`):
```json
{ "error": { "code": "RATE_LIMITED", "message": "terlalu banyak permintaan undangan", "details": { "retry_after_seconds": 1840, "scope": "invite" } }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

500 (termasuk `get-user`/`invite-user` gagal sebelum keanggotaan tercatat; pesan generik):
```json
{ "error": { "code": "INTERNAL", "message": "terjadi kesalahan", "details": null }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

### Bus yang dipanggil (Go, bukan wire JSON)

```go
// 4. cari akun menurut email (setelah otorisasi, sebelum tx)
u, _ := bus.CallTyped[getuser.Response](ctx, d.Bus, constants.BUS_CORE_AUTH_GET_USER,
    getuser.Request{Email: "baru@contoh.id"})

// 5. siapkan akun + undangan, tanpa kirim
inv, _ := bus.CallTyped[inviteuser.Response](ctx, d.Bus, constants.BUS_CORE_AUTH_INVITE_USER,
    inviteuser.Request{Email: "baru@contoh.id", Name: "Budi",
        InvitedBy: "0192f5a0-aaaa-7aaa-8bbb-00000000000a", InvitedByLabel: "owner@contoh.id",
        ProjectName: "Payments", Deliver: false})
// inv == {UserID: "7c1f...6d21", Created: true, ExpiresAt: 2026-10-28T08:15:00Z,
//         EmailSent: false, EmailStatus: "pending", Credentials: nil}

// 7. setelah commit tx management: terbit password + kirim email
rs, err := bus.CallTyped[resendinvitation.Response](ctx, d.Bus, constants.BUS_CORE_AUTH_RESEND_INVITATION,
    resendinvitation.Request{UserID: inv.UserID,
        InvitedBy: "0192f5a0-aaaa-7aaa-8bbb-00000000000a", InvitedByLabel: "owner@contoh.id",
        ProjectName: "Payments", Initial: inv.Created})
// err != nil -> degradasi 201 email_sent=false email_status=failed tanpa credentials

// 9. data tampilan anggota
shown, _ := bus.CallTyped[getuser.Response](ctx, d.Bus, constants.BUS_CORE_AUTH_GET_USER,
    getuser.Request{UserID: inv.UserID})
```

### Audit yang dikirim (`core.audit.record`, fail-open, setelah commit dan langkah kirim)

```go
record.Request{
    ActorType: record.ActorUser, ActorID: "0192f5a0-aaaa-7aaa-8bbb-00000000000a", ActorLabel: "owner@contoh.id",
    ProjectID: "0192f5a0-1111-7aaa-8bbb-000000000001", ProjectLabel: "Payments",
    Action: record.ActionProjectMemberInvited, TargetType: "project_member",
    Target: "7c1f3c2e-5b7a-4d7e-9a51-1f0b8a3e6d21",
    Args: map[string]any{"user_id": "7c1f3c2e-5b7a-4d7e-9a51-1f0b8a3e6d21", "user_label": "baru@contoh.id",
        "role": "member", "created": true},
    Status: record.StatusOK, IP: "10.0.0.5", UserAgent: "Mozilla/5.0", TraceID: "01JB2X9K6Q4Y7V3N8M5T2R1C0A",
}
```
Args tidak pernah memuat `credentials`, password, hash, atau `email_status`.
