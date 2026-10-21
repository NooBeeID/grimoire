# T12 core/auth — Usecase `inviteuser`

Konteks bersama (config, `infra/mailer`, migrasi `user_invitations`, `base/invitation.go`, `base/invitemail.go`, `base/store`, `base/delivery`, kontrak bus, aksi audit, `Shared`) ada di `overview.md` dan dikerjakan di langkah foundation. Dokumen ini hanya memuat usecase `inviteuser`. Tiket: `docs/tickets/T12-core-invitation-email.md`. Bila dokumen ini bertentangan dengan `overview.md`, `overview.md` menang. Plan ini sengaja satu usecase utuh (satu folder feature, satu PR) dengan dua titik masuk yang berbagi satu `Usecase`; tidak dipecah.

## Metadata

| Field | Nilai |
|---|---|
| Ticket | T12 |
| Scope | fullstack; plan ini hanya sisi backend (Go). UI dialog "Undang developer" ada di `frontend-plan.md` |
| Domain | core |
| Module | auth |
| Feature (folder) | `inviteuser` |
| Resource root | `backend/internal/domain/core/internal/auth` |
| Gelombang | B (setelah foundation dan gelombang A; secara kode hanya bergantung pada foundation) |

| Endpoint | Method | Version | Auth |
|---|---|---|---|
| `/api/users/invite` | POST | tanpa prefix versi (aditif; naikkan minor `info.version` di `api/modules/core/auth.yml`, dikerjakan sekali oleh plan foundation/OpenAPI) | `RequireAuth` + `RequireRole(super_admin)`; terkena `PasswordChangeGate` seperti route `/users` lain |

| Bus | Mode | Peran plan ini |
|---|---|---|
| `core.auth.invite-user` (`lib/contracts/core/auth/inviteuser`) | Call | Provider (consumer: `project/management` `inviteuser`) |
| `core.audit.record` | Call, fail-open | Consumer lewat `base/audit` |

Tidak ada job atau CLI. Tidak ada publish event.

## Summary Plan

Satu `Usecase.Execute(ctx, Input) (Result, error)` melayani dua titik masuk:

1. HTTP `POST /api/users/invite` (Super Admin, halaman Users). `Via="users"`, `Deliver=true`.
2. Handler bus `core.auth.invite-user` (dipanggil management). `Via="project"`, `Deliver` dari request (management memakai `false`).

Alur usecase (orkestrasi saja; semua rule ada di `base/`):

1. Normalisasi dan validasi input (`base.NewUser` dengan `Role=developer`; nama kosong diganti `base.DefaultNameFromEmail`). Semua kesalahan field dikumpulkan jadi satu `INVALID_ARGUMENT`, sebelum hashing dan DB.
2. Baca keadaan target: `FindUserByEmail` + `FindInvitation` (satu method repo). `base.CheckInviteTarget(user, inv)` memberi `InviteCreateNew` atau `InviteReissuePending`, atau `ALREADY_REGISTERED` dengan reason `registered|inactive|not_developer`.
3. Cabang `InviteCreateNew`:
   - Kuota pengundang: `CountInvitationsCreatedBy(inviter, now-1h)` lalu `base.CheckInviterQuota` (default 20/jam) -> 429 `RATE_LIMITED` scope `invite`.
   - Password: `base.GeneratePassword`, hash argon2id. Bila `Deliver=false` password itu dibuang (tidak pernah keluar dari usecase), akun tetap punya hash acak yang tidak diketahui siapa pun.
   - Satu tx: `InsertUser` (`must_change_password=true`, `is_active=true`, `role=developer`, `created_by`, `created_by_label`) + `InsertInvitation` (`base.NewInvitation`, `delivery_status=pending`).
   - Race `base.ErrEmailTaken`: baca ulang, jalankan `CheckInviteTarget` lagi; bila sekarang `InviteReissuePending` lanjut ke cabang reissue, selain itu `ALREADY_REGISTERED`.
4. Cabang `InviteReissuePending` (K6, akun menunggu):
   - `Deliver=false`: tidak mengubah apa pun. Hanya `Invitation.CheckResend` (cek kuota resend TANPA increment) -> 429 scope `resend`. Management kemudian memanggil `core.auth.resend-invitation` (`Initial=true`) yang benar-benar menerbitkan.
   - `Deliver=true`: satu tx dengan lock (`LockUser` lalu `LockInvitation`, urutan sama dengan `resendinvitation`): cek ulang `CheckInviteTarget`, `RegisterResend` (kuota resend naik), `Reissue` (perpanjang), `UpdatePassword(must_change=true)`, hapus semua sesi akun itu. Ini setara resend; logika terbit ada di entity dan store, bukan import feature `resendinvitation` (rule 4.3).
5. Bila `Deliver=true`, setelah commit: render `base.RenderInvitationEmail`, kirim lewat `base/delivery` (satu percobaan; konteks `context.WithoutCancel`), lalu catat `delivery_status` dan `sent_at` di tx kecil (dijaga `issued_at` agar tidak menimpa penerbitan yang lebih baru).
6. Audit (fail-open, setelah commit): `user.invited` selalu; `user.invitation.resent` bila reissue dengan `Deliver=true`; `user.invitation.email_failed` (status `error`) bila status kirim `failed` atau `not_configured`. Tanpa password, hash, atau kunci `temporary_password`.
7. Hasil: `Result` internal. HTTP membentuk 201 `{user, created, email_sent, email_status, expires_at, credentials?}`; bus membentuk `inviteuser.Response`. `credentials` hanya ada bila `Deliver && !EmailSent`.

Keamanan:
- Password sementara hanya hidup sebagai `base.Secret` di dalam usecase sampai dimasukkan ke `invitation.Credentials` pada `Result`. Tidak pernah ke logger, `audit.Event`, `Args`, atau pesan error. Hash tidak ikut respons.
- DTO HTTP memakai `string` biasa untuk `temporary_password` (`base.Secret` ter-marshal `"***"`).
- Enumerasi email lewat `ALREADY_REGISTERED` dapat diterima: HTTP hanya untuk Super Admin; bus hanya dipanggil management setelah cek izin owner/super_admin.
- Email dikirim hanya ke alamat yang diundang (tanpa CC/BCC; `mailer.Message` satu penerima).

## Plan

### Langkah 0 — Prasyarat (cek; jangan dibuat dari folder `inviteuser/`)

Pastikan foundation (overview bagian 4) sudah merge. Yang dipakai plan ini:

- `errs.CodeAlreadyRegistered` dan peta status 409 (4.1).
- Config dan `app.Deps.Mailer` (4.2, 4.4); `Shared.Invitation`, `Shared.LoginURL` (4.6d).
- Migrasi `user_invitations` (bagian 5).
- `base`: `Invitation`, `InvitationPolicy`, `NewInvitation`, `CheckInviteTarget`, `CheckInviterQuota`, `DefaultNameFromEmail`, `Delivery*`, `ErrAlreadyRegistered`, `ErrInviteRateLimited`, `RenderInvitationEmail`, `InvitationEmailData`.
- `base/store`: `FindInvitation`, `LockInvitation`, `InsertInvitation`, `UpdateInvitation`, `CountInvitationsCreatedBy` serta yang sudah ada (`FindUserByEmail`, `InsertUser`, `LockUserByID`, `UpdatePassword`, `DeleteSessionsByUser`).
- `base/delivery`: `Deliver`, `Message`.
- Kontrak `lib/contracts/core/auth/inviteuser`, `.../invitation`; konstanta `BUS_CORE_AUTH_INVITE_USER`; aksi audit `ActionUserInvited`, `ActionUserInvitationResent`, `ActionUserInvitationEmailFailed`.

Bila ada yang belum ada, selesaikan di PR foundation, bukan di PR ini.

### Langkah 1 — TDD: tes dulu, lalu implementasi, semua di `backend/internal/domain/core/internal/auth/inviteuser/`

Tulis `usecase_test.go` (fake repo, fake send, fake audit, fake hasher, clock tetap, rand deterministik) sebelum `usecase.go`. Lalu `handler_test.go`, lalu `repository_integration_test.go`.

1. **`params.go`**

```go
package inviteuser

import (
	"time"

	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/lib/contracts/core/auth/invitation"
)

const (
	ViaUsers   = "users"   // HTTP POST /api/users/invite
	ViaProject = "project" // bus dari management
)

// InviteRequest adalah body POST /api/users/invite.
type InviteRequest struct {
	Email string `json:"email"`
	Name  string `json:"name"` // opsional
}

// Input adalah input usecase (tanpa *http.Request, tanpa tipe kontrak bus).
type Input struct {
	Email, Name               string
	InvitedBy, InvitedByLabel string // id dan label pengundang (label: nama, fallback email)
	ProjectName               string // hanya untuk isi email dan audit
	Deliver                   bool   // false: hanya siapkan (dipakai management)
	Via                       string // ViaUsers | ViaProject
	Meta                      base.RequestMeta
}

// Result adalah hasil usecase. TIDAK dilog dan tidak masuk audit.Event.
type Result struct {
	User        base.User
	Invitation  base.Invitation
	Created     bool
	Now         time.Time               // waktu yang dipakai usecase (untuk Expired di respons)
	EmailStatus string                  // invitation.EmailStatus*; "pending" bila !Deliver
	EmailSent   bool                    // EmailStatus == sent
	Credentials *invitation.Credentials // hanya Deliver && !EmailSent
}

// Bentuk respons HTTP. UserView menyematkan base.UserDTO dan menambah "invitation".
// Bila plan auth-invitation-markers kelak menambah field Invitation pada UserDTO,
// field luar ini menang (shadowing encoding/json), jadi tidak bentrok.
type InvitationView struct {
	Pending        bool       `json:"pending"`
	InvitedAt      time.Time  `json:"invited_at"` // = created_at baris undangan
	SentAt         *time.Time `json:"sent_at"`
	ExpiresAt      time.Time  `json:"expires_at"`
	Expired        bool       `json:"expired"`
	DeliveryStatus string     `json:"delivery_status"`
	ResendCount    int        `json:"resend_count"`
}

type UserView struct {
	base.UserDTO
	Invitation InvitationView `json:"invitation"`
}

type CredentialsDTO struct {
	Email             string `json:"email"`
	TemporaryPassword string `json:"temporary_password"` // string biasa, BUKAN base.Secret
	LoginURL          string `json:"login_url"`
}

type InviteResponse struct {
	User        UserView        `json:"user"`
	Created     bool            `json:"created"`
	EmailSent   bool            `json:"email_sent"`
	EmailStatus string          `json:"email_status"`
	ExpiresAt   time.Time       `json:"expires_at"`
	Credentials *CredentialsDTO `json:"credentials,omitempty"` // tidak ada kuncinya bila nil
}

// toResponse memetakan Result ke JSON HTTP; now dipakai untuk Expired.
func toResponse(r Result, now time.Time) InviteResponse { /* ... */ }
```

   `Credentials` memakai `omitempty` pada pointer: pada email sukses, kunci `credentials` tidak muncul sama sekali (kriteria 10).

2. **`repository.go`** (satu-satunya yang menyentuh SQL; tidak ada SQL baru, hanya komposisi method `base/store`; pola `addmember`)

```go
type Repo struct {
	db *db.DB
	s  *store.Store
}
func NewRepo(d *db.DB) *Repo

// Target adalah keadaan email: User nil bila belum ada akun, Invitation nil bila tanpa undangan.
type Target struct {
	User       *base.User
	Invitation *base.Invitation
}

func (r *Repo) FindTarget(ctx context.Context, email string) (Target, error)               // FindUserByEmail + FindInvitation (bila user ada)
func (r *Repo) CountInvitedSince(ctx context.Context, inviterID string, since time.Time) (int, time.Time, error) // CountInvitationsCreatedBy
func (r *Repo) InTx(ctx context.Context, fn func(Tx) error) error                         // db.BeginTx + store.WithTx
```

   `txRepo` mengimplementasikan `Tx` (interface di `usecase.go`): `InsertUser`, `InsertInvitation`, `LockUser`, `LockInvitation`, `UpdateInvitation`, `UpdatePassword`, `DeleteSessions` (membungkus `DeleteSessionsByUser(id, nil)`). Error dikembalikan apa adanya (termasuk `base.ErrEmailTaken`) agar `errors.Is` jalan melewati `BeginTx`.

3. **`usecase.go`**

```go
// Interface sisi consumer (rule 5.6).
type Repository interface {
	FindTarget(ctx context.Context, email string) (Target, error)
	CountInvitedSince(ctx context.Context, inviterID string, since time.Time) (int, time.Time, error)
	InTx(ctx context.Context, fn func(Tx) error) error
}

type Tx interface {
	InsertUser(ctx context.Context, u base.User) (base.User, error)
	InsertInvitation(ctx context.Context, inv base.Invitation) error
	LockUser(ctx context.Context, id string) (base.User, bool, error)
	LockInvitation(ctx context.Context, userID string) (base.Invitation, bool, error)
	UpdateInvitation(ctx context.Context, inv base.Invitation) error
	UpdatePassword(ctx context.Context, id, hash string, mustChange bool, now time.Time) error
	DeleteSessions(ctx context.Context, userID string) (int64, error)
}

// SendFunc mengirim email undangan dan mengembalikan base.Delivery* (jangan memuat body/password di log).
// Dirakit di init.go dari delivery.Deliver + d.Mailer; usecase tidak meng-import infra/mailer.
type SendFunc func(ctx context.Context, to string, c base.EmailContent) string

type Usecase struct {
	repo     Repository
	hasher   base.Hasher
	audit    audit.Recorder
	clock    base.Clock
	rand     io.Reader
	policy   base.InvitationPolicy
	loginURL string
	send     SendFunc
}
func NewUsecase(repo Repository, hasher base.Hasher, a audit.Recorder, clock base.Clock, rnd io.Reader,
	policy base.InvitationPolicy, loginURL string, send SendFunc) *Usecase
func (u *Usecase) Execute(ctx context.Context, in Input) (Result, error)
```

   Alur `Execute` (urut, tiap langkah satu kali):

   1. `in.InvitedBy == ""` -> `fmt.Errorf("inviteuser: InvitedBy kosong")` (kesalahan pemrograman pemanggil, jadi 500). `now := u.clock().Truncate(time.Microsecond)` (agar perbandingan `issued_at` dengan Postgres exact).
   2. Validasi: `email, err := base.NormalizeEmail(in.Email)`; bila `err == nil` dan `strings.TrimSpace(in.Name) == ""` -> `name = base.DefaultNameFromEmail(email)`. Lalu `user, fe := base.NewUser(base.NewUserInput{Email: in.Email, Name: name, Role: base.RoleDeveloper, CreatedBy: in.InvitedBy, CreatedByLabel: in.InvitedByLabel, Now: now})`. `len(fe) > 0` -> `base.InvalidFields(fe)`. `user.Email` dari sini adalah email ternormalisasi. Role TIDAK dari input, selalu developer.
   3. `tgt, err := u.repo.FindTarget(ctx, user.Email)`; error -> `fmt.Errorf("inviteuser: cari target: %w", err)`.
   4. `mode, err := base.CheckInviteTarget(tgt.User, tgt.Invitation)`; error -> return apa adanya (sudah `*errs.Error`).
   5. `mode == InviteCreateNew`:
      - `n, oldest, err := u.repo.CountInvitedSince(ctx, in.InvitedBy, now.Add(-time.Hour))`; `base.CheckInviterQuota(n, oldest, now, u.policy)` -> return error.
      - `cred, err := u.newCredential()` (generate + hash; lihat di bawah).
      - `res, err := u.create(ctx, user, cred, in, now)`. Di dalam: `repo.InTx` -> `tx.InsertUser(user dengan PasswordHash=cred.hash)` -> `tx.InsertInvitation(base.NewInvitation(created.ID, in.InvitedBy, in.InvitedByLabel, now, u.policy))`.
      - `errors.Is(err, base.ErrEmailTaken)`: `tgt, err = u.repo.FindTarget(...)`; `mode, err = base.CheckInviteTarget(...)` (error -> return); bila `mode == InviteReissuePending` jatuh ke langkah 6 memakai `cred` yang sama (tidak hash ulang); bila tidak ada user (`tgt.User == nil`, mustahil) -> `fmt.Errorf("inviteuser: email bentrok tapi user tidak ada")`. Hanya satu kali baca ulang, tanpa loop.
   6. `mode == InviteReissuePending`:
      - `!in.Deliver`: `tgt.Invitation.CheckResend(now, u.policy)` -> error 429 scope `resend`. Sukses -> `Result{User, Invitation: *tgt.Invitation, Created: false, EmailStatus: invitation.EmailStatusPending}`; lanjut ke audit (langkah 8) tanpa hash, tanpa tx, tanpa kirim.
      - `in.Deliver`: `cred` (buat bila belum). `repo.InTx`: `LockUser` (tidak ada -> `ErrUserNotFound`), `LockInvitation` (tidak ada -> `base.ErrInvitationNotPending`), `base.CheckInviteTarget(&user, &inv)` ulang (selain `InviteReissuePending` -> return error; menutup race diterima-di-tengah), `inv.RegisterResend(now, policy)` (error 429), `inv.Reissue(now, policy)`, `tx.UpdatePassword(id, cred.hash, true, now)`, `tx.DeleteSessions(id)`, `tx.UpdateInvitation(inv)`. `Created=false`, `resent=true`.
   7. Pengiriman, hanya bila `in.Deliver` (baik create maupun reissue):
      - `c, err := base.RenderInvitationEmail(base.InvitationEmailData{InviteeEmail: user.Email, InviterName: in.InvitedByLabel, ProjectName: in.ProjectName, LoginURL: u.loginURL, TemporaryPassword: cred.pw.Reveal(), ExpiresAt: inv.ExpiresAt})`; error render -> `fmt.Errorf("inviteuser: render email: %w", err)` (tanpa password dalam pesan).
      - `sendCtx := context.WithoutCancel(ctx)`; `status := u.send(sendCtx, user.Email, c)`. Batas waktu dipegang driver mailer (`MAIL_TIMEOUT`); usecase tidak menambah timeout sendiri.
      - Catat status: `repo.InTx(sendCtx, ...)`: `LockInvitation`; bila `!inv.IssuedAt.Equal(issuedAt)` (sudah diterbitkan ulang oleh request lain) lewati; selain itu `inv.MarkDelivery(status, now)` + `UpdateInvitation`. Gagal mencatat hanya di-log (`inviteuser: catat status kirim gagal`, tanpa password) dan TIDAK menggagalkan request (email mungkin sudah terkirim).
      - `EmailSent = status == base.DeliverySent`; `EmailStatus = status`. Bila `!EmailSent`: `Credentials = &invitation.Credentials{Email: user.Email, TemporaryPassword: cred.pw.Reveal(), LoginURL: u.loginURL}`.
   8. Audit (setelah semua sukses; fail-open; `Actor` nil agar dari reqctx; `Target: user.ID`, `TargetType: "user"`, `Meta: in.Meta`):
      - `record.ActionUserInvited`, `Status: ok`, `Args: {email, created, via, project_name?}` (`project_name` hanya bila tidak kosong).
      - `record.ActionUserInvitationResent` bila reissue dan `Deliver`: `Args: {email, via, project_name?, resend_count: inv.ResendCount}`.
      - `record.ActionUserInvitationEmailFailed` bila `Deliver && !EmailSent`: `Status: record.StatusError`, `Args: {email, delivery_status: status, error_class}`. `error_class` = `"not_configured"` untuk `not_configured`, `"send_failed"` untuk `failed`. (`delivery.Deliver` hanya mengembalikan status; bila foundation kelak mengembalikan tahap smtp, pakai nilai itu.)
   9. Return `Result`.

   Helper privat di `usecase.go`:

```go
type credential struct {
	pw   base.Secret // plaintext sementara; tidak pernah dilog
	hash string
}
// newCredential membangkitkan password (base.GeneratePassword(u.rand)) dan menghash-nya.
// Dipanggil paling banyak sekali per Execute (lazy; dipakai ulang bila race memindah cabang).
func (u *Usecase) newCredential() (credential, error)
```

   Error generate/hash dibungkus `fmt.Errorf("inviteuser: generate password: %w", err)` / `"inviteuser: hash: %w"`; tanpa plaintext.

   Aturan: usecase tidak meng-import `net/http`, `database/sql`, `infra/mailer`, atau feature lain; tidak ada SQL; penegakan `super_admin` hanya di router (bus dijaga oleh izin management). Hasher dijalankan di luar tx (argon2 mahal; tx tidak menahan lock selama hashing). Tidak ada panggilan bus di dalam tx (audit dan kirim semuanya setelah commit).

   Catatan konsistensi: urutan lock `LockUser` lalu `LockInvitation` WAJIB sama dengan `resendinvitation` dan `purgeinvitations` agar tidak deadlock.

4. **`handler.go`**

```go
type Handler struct{ uc *Usecase }
func NewHandler(uc *Usecase) *Handler

// Invite melayani POST /api/users/invite.
func (h *Handler) Invite(w http.ResponseWriter, r *http.Request) {
	var req InviteRequest
	if err := httpx.DecodeJSON(r, &req); err != nil { httpx.WriteError(w, r, err); return }
	actor, ok := reqctx.IdentityFrom(r.Context())
	if !ok { httpx.WriteError(w, r, errs.New(errs.CodeUnauthorized, "perlu login")); return }
	res, err := h.uc.Execute(r.Context(), Input{
		Email: req.Email, Name: req.Name,
		InvitedBy: actor.UserID, InvitedByLabel: inviterLabel(actor),
		Deliver: true, Via: ViaUsers,
		Meta: base.RequestMeta{IP: middleware.KeyByIP(r), UserAgent: base.TruncateUA(r.UserAgent())},
	})
	if err != nil { httpx.WriteError(w, r, err); return }
	if res.Credentials != nil { w.Header().Set("Cache-Control", "no-store") }
	httpx.WriteJSON(w, http.StatusCreated, toResponse(res, now))
}
```

   - `inviterLabel(actor)`: `actor.Name` bila tidak kosong, selain itu `actor.Email`.
   - `DecodeJSON` memakai `DisallowUnknownFields`: body dengan `role`, `password`, `is_active`, dan lain-lain ditolak 400 `field tidak dikenal`. Ini menjaga "role selalu developer" dan "tanpa input password".
   - `Cache-Control: no-store` hanya bila ada `credentials` (sesuai tiket). Handler tidak me-log `res`.
   - Keputusan: `Result` memuat field `Now time.Time` (waktu yang dipakai usecase). Handler memanggil `toResponse(res, res.Now)`; `Expired = res.Invitation.IsExpired(res.Now)`. Handler tidak butuh clock sendiri; `NewHandler(uc)` saja. Pada akun menunggu `Deliver=false` yang sudah kedaluwarsa, `expired=true` muncul apa adanya.

5. **`bushandler.go`** (handler bus; `bus.Handler`)

```go
import contract "grimoire/lib/contracts/core/auth/inviteuser"

type BusHandler struct{ uc *Usecase }

// Handle melayani core.auth.invite-user. Mengembalikan nilai contract.Response (bukan pointer).
func (h *BusHandler) Handle(ctx context.Context, req any) (any, error) {
	in, err := bus.Expect[contract.Request](req)
	if err != nil { return nil, err }
	res, err := h.uc.Execute(ctx, Input{
		Email: in.Email, Name: in.Name,
		InvitedBy: in.InvitedBy, InvitedByLabel: in.InvitedByLabel,
		ProjectName: in.ProjectName, Deliver: in.Deliver, Via: ViaProject,
	})
	if err != nil { return nil, err } // *errs.Error utuh sampai management
	return toContractResponse(res), nil
}
```

   - `toContractResponse`: `UserID`, `Created`, `ExpiresAt = res.Invitation.ExpiresAt`, `EmailSent`, `EmailStatus`, `Credentials` (pointer ke `invitation.Credentials`, nil bila tidak ada).
   - `Meta` kosong di jalur bus (IP/UA tidak ikut); audit tetap membawa actor dari `reqctx` (owner) karena `ctx` diteruskan bus.
   - Error `INVALID_ARGUMENT`, `ALREADY_REGISTERED`, `RATE_LIMITED` diteruskan apa adanya; kode `ALREADY_REGISTERED` sudah ada di peta status bawaan (bukan `RegisterCodeStatus` per module).

6. **`init.go`**

```go
// Init mendaftarkan POST /users/invite (di bawah /api, khusus super_admin) dan handler bus core.auth.invite-user.
func Init(d app.Deps, s base.Shared) {
	send := func(ctx context.Context, to string, c base.EmailContent) string {
		return delivery.Deliver(ctx, d.Mailer, delivery.Message(to, c), d.Log)
	}
	uc := NewUsecase(NewRepo(d.DB), s.Hasher, audit.New(d.Bus, d.Log), s.Clock, s.Rand, s.Invitation, s.LoginURL, send)
	d.API.With(middleware.RequireAuth(), middleware.RequireRole(base.RoleSuperAdmin)).
		Post("/users/invite", NewHandler(uc).Invite)
	d.Bus.Register(constants.BUS_CORE_AUTH_INVITE_USER, (&BusHandler{uc: uc}).Handle)
}
```

   - Route statis `/users/invite` terdaftar bersama `POST /users` (createuser) dan `POST /users/{id}/...`; chi memprioritaskan segmen statis, tidak bentrok dengan `/users/{id}`. Pakai `d.API.With(...).Post(...)`, bukan `Route("/users", ...)`.
   - `d.Mailer` tidak pernah nil (noop bila tidak dikonfigurasi). Feature ini tidak meng-import `infra/mailer`.
   - `Register` bus ganda panic (rule 5.9); `Init` dipanggil sekali.

7. **`auth/base.go`**: tambah SATU baris `inviteuser.Init(d, shared)` di `Register` (plus import). Jangan mengubah baris feature lain. Karena `app.Deps` tidak punya `Config` di `Init`, semua kebijakan lewat `Shared`.

### Langkah 2 — OpenAPI (kontrak dulu, rule 11.6)

Di `api/modules/core/auth.yml`: path `POST /users/invite` (operationId `inviteUser`), schema `InviteUserRequest {email (wajib), name}` dengan `additionalProperties: false`, `InviteUserResponse` (merujuk `User` + `invitation` dari `InvitationStatus` foundation, `created`, `email_sent`, `email_status` enum `sent|failed|not_configured`, `expires_at`, `credentials` merujuk `InvitationCredentials`, tidak required). Respons 201 mendokumentasikan header `Cache-Control: no-store` (hanya saat `credentials`), 400, 401, 403 (`FORBIDDEN` dan `PASSWORD_CHANGE_REQUIRED`), 409 (`ALREADY_REGISTERED`, `details.reason` enum `registered|inactive|not_developer`), 429 (`RATE_LIMITED`, `details.retry_after_seconds`, `details.scope=invite`), 500. Header wajib `X-Requested-With`, `security: sessionCookie`. Path ditambahkan ke `api/root.yml`, jalankan `make api-bundle`, commit `api/openapi.yml`, pastikan `make api-check` lulus.

### Langkah 3 — Verifikasi

`go build ./...`, `go vet ./...`, `go test ./internal/domain/core/internal/auth/...`, `go test -tags integration` untuk tes integrasi, `internal/archtest` (inviteuser hanya mengimpor `base`, `base/store`, `base/audit`, `base/delivery`, `lib/*`, `infra/{app,bus,http,middleware,db}`; tidak mengimpor feature lain dan tidak mengimpor `infra/mailer`), `make api-check`.

## Migration

Tidak ada migrasi baru di plan ini. Memakai `user_invitations` dari migrasi foundation `backend/migrations/core/<VERSION>_create_user_invitations.sql` dan `users` dari `20261021100000000_create_auth_tables.sql`. Kolom yang ditulis feature ini: `users` (`email`, `name`, `password_hash`, `role=developer`, `is_active=true`, `must_change_password=true`, `created_by`, `created_by_label`, `created_at`, `updated_at`); `user_invitations` (`user_id`, `invited_by`, `invited_by_label`, `issued_at`, `expires_at`, `sent_at`, `delivery_status`, `resend_count`, `resend_window_*`, `created_at`).

## Changes

Baru (semua di `backend/internal/domain/core/internal/auth/inviteuser/`):
- `init.go`, `handler.go`, `bushandler.go`, `params.go`, `usecase.go`, `repository.go`
- `usecase_test.go`, `handler_test.go`, `bushandler_test.go`, `repository_integration_test.go` (tag `integration`), `integration_test.go` (router + bus nyata, tag `integration`)

Diubah:
- `backend/internal/domain/core/internal/auth/base.go`: satu baris `inviteuser.Init(d, shared)` + import.
- `backend/internal/domain/core/internal/auth/auth_integration_test.go`: tambah baris tabel (`POST /api/users/invite` pada `TestT01_AC7_DeveloperForbiddenOnUsers`, `TestRoutesRegistered`, tes audit, tes "password tidak pernah dilog atau diaudit").
- `api/modules/core/auth.yml`, `api/root.yml`, `api/openapi.yml`: path `POST /users/invite` (Langkah 2).

Tidak diubah (milik foundation atau plan lain): `base/*`, `base/store`, `base/delivery`, `infra/*`, `lib/contracts/*`, `lib/constants/bus.go`, feature lain, `docs/tickets/*`.

## Test Scenario

Penamaan tes kriteria penerimaan: `TestT12_ACn_...`. Tes unit memakai fake `Repository`/`Tx` (in-memory, mendukung simulasi `ErrEmailTaken`), fake `audit.Recorder`, fake `SendFunc` (status dapat diatur `sent|failed|not_configured`, mencatat panggilan), fake `Hasher` (mencatat input), `Clock` tetap, `Rand` deterministik. Rule entity diuji di `base/` (foundation) tanpa mock.

### Unit usecase (`usecase_test.go`)

| Test | Skenario | Ekspektasi |
|---|---|---|
| `TestExecute_NewAccount_Deliver_Sent` | email baru, `Deliver=true`, send `sent` | user di-insert `role=developer`, `IsActive`, `MustChangePassword`, `CreatedBy=inviter`; invitation `NewInvitation`; send dipanggil sekali ke email ternormalisasi; `EmailSent=true`, `EmailStatus=sent`, `Credentials==nil`; invitation `delivery_status=sent`, `sent_at=now` |
| `TestExecute_Deliver_NotConfigured` | send `not_configured` | `EmailSent=false`, `EmailStatus=not_configured`, `Credentials` terisi (email, password 16 karakter, login URL); `delivery_status=not_configured`, `sent_at=nil`; audit `email_failed` status `error` |
| `TestExecute_Deliver_Failed` | send `failed` | sama seperti di atas dengan `failed`; akun dan undangan tetap tercatat (tidak ada rollback) |
| `TestExecute_Deliver_False_OnlyPrepares` | `Deliver=false`, email baru | send TIDAK dipanggil; `EmailStatus=pending`; `Credentials==nil`; invitation `pending`; hash tersimpan = output hasher tetapi plaintext tidak ada di `Result` maupun di mana pun |
| `TestExecute_PasswordMatchesCredentialsAndHash` | `Deliver=true` fallback | hasher menerima persis password yang ada di `Credentials`; `PasswordHash` ke repo = output hasher, bukan plaintext |
| `TestExecute_DefaultNameFromEmail` | `Name=""` / spasi, `budi.santoso@x.id` | `Name=="budi santoso"`; user ke repo memakainya |
| `TestExecute_NameProvidedKept` | `Name="Budi S"` | dipakai apa adanya (trim) |
| `TestExecute_EmailNormalized` | `" Baru@Contoh.ID "` | repo menerima `baru@contoh.id`; email send ke alamat itu |
| `TestExecute_InvalidEmail` | `""`, `"a"`, `"Budi <b@x.id>"`, 255 byte | `INVALID_ARGUMENT` `fields.email`; repo/hasher/send/audit tidak dipanggil |
| `TestExecute_InvalidName` | 101 rune, karakter kontrol | `fields.name`; tidak ada efek samping |
| `TestExecute_RoleAlwaysDeveloper` | tidak ada jalur input role | user yang di-insert selalu `developer` |
| `TestExecute_AlreadyRegistered_Registered` | user ada tanpa undangan (dibuat `createuser`) | `ALREADY_REGISTERED` `reason=registered`; hasher/tx tidak dipanggil |
| `TestExecute_AlreadyRegistered_Accepted` | user + undangan `accepted_at` terisi | `ALREADY_REGISTERED` `registered` |
| `TestExecute_AlreadyRegistered_Inactive` | user nonaktif (dengan atau tanpa undangan) | reason `inactive` |
| `TestExecute_AlreadyRegistered_NotDeveloper` | user `super_admin` | reason `not_developer` |
| `TestExecute_InviterQuotaExceeded` | `CountInvitedSince` = 20, oldest 10 menit lalu | `RATE_LIMITED`, `scope=invite`, `retry_after_seconds=3000`; tidak ada hash/insert |
| `TestExecute_InviterQuotaBoundary19` | count 19 | lolos |
| `TestExecute_QuotaNotAppliedToReissue` | K6, count 20 | tidak 429 scope invite (kuota resend yang berlaku) |
| `TestExecute_K6_Deliver_False_NoIncrement` | akun menunggu, `Deliver=false` | tidak ada tx/UpdateInvitation/UpdatePassword/hapus sesi; `Created=false`; `EmailStatus=pending`; `ResendWindowCount` tidak berubah |
| `TestExecute_K6_Deliver_False_ResendQuotaExceeded` | menunggu, `ResendWindowCount=3` dalam jam | `RATE_LIMITED` `scope=resend` |
| `TestExecute_K6_Deliver_True_Reissues` | menunggu (juga kedaluwarsa), `Deliver=true`, send `sent` | `Created=false`; password baru di-hash dan `UpdatePassword(must_change=true)`; semua sesi dihapus; invitation `Reissue` (expires = now+TTL), `ResendCount++`, `ResendWindowCount++`; email dikirim; audit `user.invited(created=false)` + `user.invitation.resent` |
| `TestExecute_K6_Deliver_True_ResendQuotaExceeded` | window penuh | 429 scope `resend`; tidak ada perubahan password/sesi, tidak ada email |
| `TestExecute_K6_AcceptedDuringTx` | tx lock membaca undangan sudah `accepted` | `ALREADY_REGISTERED`; tidak ada update |
| `TestExecute_RaceEmailTaken_BecomesReissue` | `InsertUser` -> `ErrEmailTaken`, baca ulang = akun menunggu | beralih ke reissue; hash TIDAK dihitung ulang (satu panggilan hasher); hasil `Created=false` |
| `TestExecute_RaceEmailTaken_BecomesAlreadyRegistered` | baca ulang = akun terdaftar/diterima | `ALREADY_REGISTERED` |
| `TestExecute_InsertError_Wrapped` | repo error generik | `inviteuser: ...` terbungkus; bukan `*errs.Error`; tanpa password; tidak ada send/audit |
| `TestExecute_HasherError_NoLeak` / `TestExecute_RandError` | hasher/rand gagal | error terbungkus tanpa plaintext; tanpa insert |
| `TestExecute_DeliveryRecordFails_StillSuccess` | `UpdateInvitation` (catat status) gagal setelah kirim | request tetap sukses sesuai status kirim; error di-log tanpa password |
| `TestExecute_DeliveryRecordSkippedWhenReissuedConcurrently` | `issued_at` baris berbeda saat catat status | status tidak ditimpa |
| `TestExecute_SendUsesDetachedContext` | ctx pemanggil dibatalkan sebelum kirim | `SendFunc` menerima ctx yang `Err()==nil` |
| `TestExecute_EmailContentHasInviterAndProject` | `ProjectName="Alpha"`, label "Owner Satu" | `EmailContent` yang dikirim memuat keduanya, URL login, password; `Subject` tanpa CR/LF |
| `TestExecute_AuditUserInvited_Shape` | create sukses via `users` | tepat satu `user.invited`: `Target=<id>`, `TargetType=user`, Args = `{email, created=true, via=users}` saja (tanpa `project_name` bila kosong), `Status=ok`, `Meta` diteruskan, `Actor==nil` |
| `TestExecute_AuditViaProject` | `Via=project`, `ProjectName` terisi | Args memuat `via=project`, `project_name` |
| `TestExecute_AuditEmailFailedShape` | send `failed` / `not_configured` | `user.invitation.email_failed`, `Status=error`, Args = `{email, delivery_status, error_class}` |
| `TestExecute_AuditNeverContainsSecrets` | semua mode | marshal semua `audit.Event` ke JSON; tidak memuat plaintext, hash, atau kunci `temporary_password` |
| `TestExecute_NoAuditOnValidationOrConflict` | 400, 409, 429 | 0 event audit |
| `TestExecute_NoLogLeak` | semua mode, logger buffer | buffer log tidak memuat plaintext password atau hash |
| `TestExecute_EmptyInvitedBy` | `InvitedBy=""` | error internal, tanpa efek samping |

### Unit handler (`handler_test.go`, httptest, usecase asli dengan fake repo/send)

- `TestHandler_Created201_EmailSent`: 201, `Content-Type` JSON, body punya `user.invitation.delivery_status=sent`, `email_sent=true`, TIDAK ada kunci `credentials` (kriteria 10), TIDAK ada `Cache-Control: no-store`.
- `TestHandler_Created201_FallbackNoStore`: send `failed`/`not_configured` -> 201, `credentials{email,temporary_password,login_url}` ada, header `Cache-Control: no-store`.
- `TestHandler_Developer403`: memakai router dengan `RequireRole(super_admin)` dan identitas developer -> 403 `FORBIDDEN`; usecase tidak dijalankan, tidak ada baris.
- `TestHandler_Anonymous401`.
- `TestHandler_InvalidEmail400`: `{"email":"bukan-email"}` -> 400 `INVALID_ARGUMENT` `details.fields.email`.
- `TestHandler_UnknownFieldRejected`: body dengan `role`, `password`, `is_active` -> 400 `field tidak dikenal`; tidak ada akun dibuat.
- `TestHandler_EmptyAndMalformedBody400`, `TestHandler_WrongTypes400` (`{"email":5}`).
- `TestHandler_AlreadyRegistered409`: kode `ALREADY_REGISTERED` dan `details.reason`.
- `TestHandler_RateLimited429`: `RATE_LIMITED` dengan `details.retry_after_seconds` dan `scope`.
- `TestHandler_InternalErrorHidesDetail`: 500 tanpa teks SQL/SMTP/password.
- `TestHandler_NotLoggedOnDecodeError`: body invalid yang memuat string aneh tidak muncul di log berisi password (logger buffer).
- `TestHandler_LabelFallsBackToEmail`: identitas tanpa `Name` -> `InvitedByLabel=email`.

### Unit handler bus (`bushandler_test.go`)

- `TestBus_Handle_ReturnsValueResponse`: `bus.CallTyped[contract.Response]` sukses, `UserID` terisi, `EmailStatus=pending` untuk `Deliver=false`.
- `TestBus_Handle_BadRequestType`: string salah dan `*contract.Request` (pointer) -> `bus.ErrBadRequestType`.
- `TestBus_Handle_PropagatesErrsError`: `ALREADY_REGISTERED`, `RATE_LIMITED`, `INVALID_ARGUMENT` lewat `errors.As` ke `*errs.Error` dengan `details` utuh.
- `TestBus_Handle_DeliverFalseNoSend`: `SendFunc` tidak dipanggil, `Credentials==nil`.
- `TestBus_Handle_DeliverTrueFallbackCredentials`: `Credentials` di response bus terisi.
- `TestBus_Handle_ViaProjectAndProjectNameInAudit`.
- `TestInit_RegistersEndpointAndRoute`: bus terdaftar, route `POST /api/users/invite` ada; `Init` kedua panic (pola `getuserbus`).

### Integrasi repo (Postgres nyata via `db.TestDB(t)`, migrasi `migrations.FS`; `repository_integration_test.go`)

- `TestRepo_CreateInTx_PersistsUserAndInvitation`: setelah `InsertUser` + `InsertInvitation`, baris users (`role=developer`, `must_change_password=true`, `created_by`, hash `$argon2id$`) dan `user_invitations` (`delivery_status=pending`, `expires_at`, `invited_by`, `invited_by_label`) ada; `FindTarget` mengembalikan keduanya.
- `TestRepo_TxRollbackOnInvitationFailure`: kegagalan `InsertInvitation` (mis. `invited_by` bukan uuid) membatalkan `InsertUser` (tidak ada akun yatim).
- `TestRepo_DuplicateEmailCaseInsensitive`: `Baru@X.id` lalu `baru@x.id` -> `base.ErrEmailTaken` dari `InsertUser`; satu baris saja.
- `TestRepo_ConcurrentSameEmail`: 10 goroutine memanggil `Execute` (usecase asli, hasher kecil, send fake) dengan email sama -> tepat satu `Created=true`, sisanya `Created=false` (K6, sebagian bisa 429 `resend` setelah 3), tanpa 500, tepat satu baris `users` dan satu baris `user_invitations`.
- `TestRepo_CountInvitedSince`: hanya undangan milik pengundang itu dalam jendela; undangan reissue tidak menambah hitungan; `oldest` benar.
- `TestRepo_ReissueTx_RevokesSessionsAndResetsHash`: akun menunggu dengan dua sesi -> setelah reissue Deliver=true, sesi 0, `password_hash` berganti, `must_change_password=true`, `expires_at` maju, `resend_count=1`, `delivery_status` akhir sesuai send.
- `TestRepo_LockOrder_NoDeadlock`: dua tx paralel reissue pada akun sama selesai tanpa deadlock (LockUser lalu LockInvitation).
- `TestRepo_DeliveryRecordGuardedByIssuedAt`: pencatatan status dengan `issuedAt` usang tidak mengubah baris.

### Integrasi router + bus (`integration_test.go`; `httpx.NewRouter` + `core.Register`, bus nyata, `mailertest.Fake` di `Deps.Mailer`, handler `core.audit.record` penampung)

- `TestT12_AC6_SuperAdminInvitesFromUsers`: Super Admin `POST /api/users/invite` -> 201; akun developer tanpa project; `mailertest.Fake.Sent` berisi tepat satu email ke alamat itu dengan URL login dan password sementara; respons tanpa `credentials`.
- `TestT12_AC6_DeveloperForbidden`: developer -> 403; tidak ada baris; tidak ada audit.
- `TestT12_AC4_AlreadyRegistered`: email dari `createuser` dan akun yang sudah menerima -> 409 `ALREADY_REGISTERED`; akun menunggu -> 201 `created=false`, password berganti, email kedua.
- `TestT12_AC9_FallbackWhenMailerFails`: `Fake.Err` diisi -> 201 `email_sent=false`, `credentials` ada, `Cache-Control: no-store`; `delivery_status=failed` di DB; audit `user.invitation.email_failed`; `mailer` noop (`NotConfigured`) -> `email_status=not_configured`.
- `TestT12_AC10_NoCredentialsOnSuccess`: kunci `credentials` tidak ada pada JSON mentah.
- `TestT12_AC1_PasswordNeverLoggedOrAudited`: buffer log aplikasi dan JSON seluruh `record.Request` tidak memuat plaintext, `$argon2id$`, atau hash, pada mode sukses dan fallback; `mailer` log/Fake saja yang memuat password di isi email.
- `TestT12_Credentials_NotReadableAgain`: setelah fallback, `GET /api/users/{id}` dan `GET /api/users` tidak memuat password; tidak ada endpoint yang mengembalikannya.
- `TestT12_InviterRateLimit`: 20 undangan sukses, ke-21 -> 429 `scope=invite` `retry_after_seconds` > 0; pengundang lain tidak terpengaruh.
- `TestT12_BusDeliverFalseThenAccountCanLoginOnlyAfterIssue`: bus `Deliver=false` membuat akun menunggu; login dengan password apa pun gagal (password dibuang); setelah `Deliver=true` (reissue) login dengan password dari `credentials` berhasil dan `must_change_password=true`.
- `TestT12_AuditEachActionOneEntry`: undangan baru sukses menghasilkan tepat satu `user.invited` (actor = Super Admin, `ActorLabel` sesuai, `TraceID` sama dengan respons); fallback menambah satu `user.invitation.email_failed`; 400/409/429 menghasilkan 0 entri.
- `TestAudit_UnregisteredDoesNotFailRequest`: tanpa handler audit, `POST /api/users/invite` tetap 201.
- `TestRoutesRegistered`: `chi.Walk` memuat `POST /api/users/invite`.

## E2E Scenarios

Playwright terhadap binary dengan Mailpit (`docker compose up`), `grimoire migrate up`, dan admin dari `create-admin`. Dialog UI diurus `frontend-plan.md`; di sini kasus yang menyentuh endpoint ini.

Positif:
1. (K1/6) Super Admin membuka Admin -> Users -> "Undang developer", mengisi `baru@contoh.id`, nama kosong -> sukses: toast email terkirim, baris `baru` muncul dengan penanda "undangan menunggu". Mailpit (`:8025`) menampilkan satu email berisi URL login dan password sementara.
2. Penerima login memakai password sementara dari Mailpit, diarahkan ke `/change-password`, mengganti password, lalu masuk; di daftar Users penanda "undangan menunggu" hilang (diterima; diverifikasi bersama plan `auth-login-changepassword`).
3. Nama diisi (`Budi S`): akun bernama `Budi S`. Nama kosong dengan email `budi.santoso@contoh.id` menghasilkan nama `budi santoso`.
4. Mengundang email yang akunnya masih menunggu (dari halaman Users): tidak ada akun ganda, email kedua tiba di Mailpit, password lama tidak berlaku, password baru berlaku.
5. Mailpit dihentikan (atau `MAIL_DRIVER` kosong): undangan tetap 201; dialog "Kredensial sementara" tampil sekali dengan email, password, URL login; tutup lalu buka ulang Users/detail tidak menampilkan password lagi. Header respons `Cache-Control: no-store`. Log aplikasi dan audit tanpa password.
6. Undangan dari management (bus, `Deliver=false` lalu penerbitan oleh `resend-invitation`): diverifikasi di plan `management-inviteuser`; di sini cukup bahwa email target tidak menerima email dari langkah `Deliver=false`.
7. Email dengan huruf besar dan spasi (`" Baru@Contoh.ID "`) tersimpan `baru@contoh.id`.

Negatif:
1. Email sudah terdaftar (akun `createuser` atau sudah menerima undangan, beda huruf besar/kecil): 409 `ALREADY_REGISTERED`; form menampilkan pesan arahan "gunakan pencarian developer" dan dialog tetap terbuka.
2. Akun nonaktif atau Super Admin dengan email itu: 409 `ALREADY_REGISTERED` (reason `inactive` / `not_developer`).
3. Email tidak valid: 400 dengan pesan pada field email.
4. Developer biasa: `/admin/users` NotFound di UI; `fetch POST /api/users/invite` manual -> 403 `FORBIDDEN`.
5. Tanpa login -> 401; tanpa header `X-Requested-With` -> 403 `CSRF_REQUIRED`; pengguna wajib-ganti-password -> 403 `PASSWORD_CHANGE_REQUIRED`.
6. Body berisi `role` atau `password` -> 400 `field tidak dikenal`; tidak ada akun dibuat.
7. Undangan ke-21 dalam satu jam oleh Super Admin yang sama -> 429 `RATE_LIMITED` scope `invite`; UI menampilkan sisa waktu.
8. Kirim undangan ke akun menunggu keempat kali dalam satu jam -> 429 scope `resend`.
9. Aplikasi start tanpa konfigurasi email (noop): `POST /api/users/invite` tetap bekerja lewat fallback; `MAIL_DRIVER=log` + `APP_ENV=production` gagal start (diverifikasi di foundation).
10. Setelah undangan, halaman audit (T02) menampilkan `user.invited` (dan `user.invitation.email_failed` pada fallback) tanpa password.

## Request/Response body

Semua error memakai format seragam T00: `{ "error": { "code", "message", "details" }, "trace_id" }`. Request mutasi wajib header `X-Requested-With: grimoire`, `Content-Type: application/json`, dan cookie `grimoire_session`. Seluruh JSON snake_case.

### Request HTTP, nama dikosongkan (default dari email)

```
POST /api/users/invite
```
```json
{ "email": " Baru@Contoh.id ", "name": "" }
```

### Request HTTP, nama diisi

```json
{ "email": "budi.santoso@contoh.id", "name": "Budi Santoso" }
```

### 201 Created, email terkirim (tanpa kunci `credentials`)

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
    "created_by": "0b7c6a4e-2f1d-4c8a-9e3b-5d6f7a8b9c0d",
    "created_by_label": "Admin Satu",
    "updated_at": "2026-10-21T08:15:00Z",
    "last_login_at": null,
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

### 201 Created, fallback (header `Cache-Control: no-store`)

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
    "created_by": "0b7c6a4e-2f1d-4c8a-9e3b-5d6f7a8b9c0d",
    "created_by_label": "Admin Satu",
    "updated_at": "2026-10-21T08:15:00Z",
    "last_login_at": null,
    "invitation": {
      "pending": true,
      "invited_at": "2026-10-21T08:15:00Z",
      "sent_at": null,
      "expires_at": "2026-10-28T08:15:00Z",
      "expired": false,
      "delivery_status": "not_configured",
      "resend_count": 0
    }
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

`email_status` pada fallback: `failed` (SMTP menolak/mati) atau `not_configured` (mailer noop).

### 201 Created, akun menunggu diterbitkan ulang (K6, `created=false`)

Bentuk sama dengan di atas; `created=false`, `invitation.resend_count` naik, `expires_at` diperpanjang.

### 400 Bad Request, validasi

```json
{
  "error": {
    "code": "INVALID_ARGUMENT",
    "message": "permintaan tidak valid",
    "details": { "fields": { "email": "format email tidak valid" } }
  },
  "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33"
}
```

Varian lain dari `DecodeJSON` (semua 400 `INVALID_ARGUMENT`): `"JSON tidak valid"`, `"body wajib diisi"`, `"tipe field tidak sesuai"` (`details.field`), `"field tidak dikenal"` (`details.field`, mis. `role`). Body terlalu besar: 413 `PAYLOAD_TOO_LARGE`.

### 401 Unauthorized

```json
{ "error": { "code": "UNAUTHORIZED", "message": "perlu login", "details": null }, "trace_id": "..." }
```

### 403 Forbidden (developer)

```json
{ "error": { "code": "FORBIDDEN", "message": "akses ditolak", "details": null }, "trace_id": "..." }
```

Varian: `PASSWORD_CHANGE_REQUIRED` (pengundang wajib ganti password) dan `CSRF_REQUIRED` (tanpa header).

### 409 Conflict, sudah terdaftar

```json
{
  "error": {
    "code": "ALREADY_REGISTERED",
    "message": "email sudah terdaftar",
    "details": { "reason": "registered" }
  },
  "trace_id": "..."
}
```

`details.reason`: `registered` (akun dibuat `createuser` atau sudah menerima undangan), `inactive`, `not_developer`.

### 429 Too Many Requests

Kuota pengundang (20/jam):
```json
{
  "error": {
    "code": "RATE_LIMITED",
    "message": "terlalu banyak permintaan undangan",
    "details": { "retry_after_seconds": 1840, "scope": "invite" }
  },
  "trace_id": "..."
}
```

Kuota kirim ulang (akun menunggu yang diundang ulang, 3/jam): sama dengan `"scope": "resend"`.

### 500 Internal (kegagalan DB/hasher; tanpa detail internal)

```json
{ "error": { "code": "INTERNAL", "message": "terjadi kesalahan internal", "details": null }, "trace_id": "..." }
```

### Bus `core.auth.invite-user` (Deliver=false, dipanggil management)

Request:
```go
inviteuser.Request{
    Email: "baru@contoh.id", Name: "Budi", InvitedBy: "0b7c6a4e-2f1d-4c8a-9e3b-5d6f7a8b9c0d",
    InvitedByLabel: "Owner Satu", ProjectName: "Alpha", Deliver: false,
}
```
Response (akun baru, hanya siap):
```go
inviteuser.Response{
    UserID: "7c1f3c2e-5b7a-4d7e-9a51-1f0b8a3e6d21", Created: true,
    ExpiresAt: time.Date(2026, 10, 28, 8, 15, 0, 0, time.UTC),
    EmailSent: false, EmailStatus: "pending", Credentials: nil,
}
```
Response (akun menunggu K6, `Deliver=false`): `Created: false`, `ExpiresAt` = masa berlaku saat ini (belum diperpanjang), `EmailStatus: "pending"`.
Error: `*errs.Error` kode `INVALID_ARGUMENT` (`details.fields`), `ALREADY_REGISTERED` (`details.reason`), `RATE_LIMITED` (`details.retry_after_seconds`, `scope` = `invite` | `resend`), diteruskan apa adanya.

### Contoh `record.Request` ke `core.audit.record`

Sukses (akun baru dari halaman Users):
```go
record.Request{
    ActorType: "user", ActorID: "0b7c6a4e-2f1d-4c8a-9e3b-5d6f7a8b9c0d", ActorLabel: "admin@tim.id",
    Action: "user.invited", TargetType: "user", Target: "7c1f3c2e-5b7a-4d7e-9a51-1f0b8a3e6d21",
    Args: map[string]any{"email": "baru@contoh.id", "created": true, "via": "users"},
    Status: "ok", IP: "10.0.0.5", UserAgent: "Mozilla/5.0 ...", TraceID: "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33",
}
```
Email gagal terkirim:
```go
record.Request{
    Action: "user.invitation.email_failed", TargetType: "user", Target: "7c1f3c2e-5b7a-4d7e-9a51-1f0b8a3e6d21",
    Args: map[string]any{"email": "baru@contoh.id", "delivery_status": "failed", "error_class": "send_failed"},
    Status: "error", /* actor, IP, UA, trace seperti di atas */
}
```
Tidak ada kunci `password`, `temporary_password`, atau hash di `Args`, `Result`, maupun log.
