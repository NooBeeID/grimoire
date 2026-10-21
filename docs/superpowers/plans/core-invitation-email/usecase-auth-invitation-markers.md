# T12 usecase `auth-invitation-markers` - Penanda undangan aditif di sisi baca core/auth

Foundation: `docs/superpowers/plans/core-invitation-email/overview.md` (menang bila bertentangan). Tiket: `docs/tickets/T12-core-invitation-email.md`.
Gelombang A (lihat overview bagian 9): bebas dari usecase lain, hanya butuh foundation 4.1 sampai 4.10 selesai (tabel `user_invitations`, `base.Invitation`, `store.FindInvitation`, `contracts/getuser` aditif, `finddevelopers` aditif, `app.Deps.Mailer`).

---

## 1. Metadata

| Atribut | Nilai |
|---|---|
| Ticket | T12 |
| Usecase | `auth-invitation-markers` (beberapa folder kecil, satu plan, tidak dipecah) |
| Scope | backend (+ satu respons HTTP yang dikonsumsi frontend) |
| Domain / Module | core / auth (`backend/internal/domain/core/internal/auth`) |
| Folder feature | `getuserbus`, `finddevelopers`, `listusers`, `getuser`, `getme`; entity di `base/user.go` dan `base/invitation.go` |
| Migrasi | tidak ada (tabel dibuat foundation) |

| Endpoint | Method | Version | Auth | Perubahan |
|---|---|---|---|---|
| `/api/users` | GET | - | `RequireAuth` + `RequireRole(super_admin)` | aditif: tiap item punya `invitation` (object atau null) |
| `/api/users/{id}` | GET | - | idem | aditif: `invitation` |
| `/api/auth/me` | GET | - | `RequireAuth` | aditif: `email_enabled` (bool) |
| bus `core.auth.get-user` | Call | aditif | - | `Request.Email`, `Response.Invitation` |
| bus `core.auth.find-developers` | Call | aditif | - | `Developer.InvitationPending` |

Konsumen bus: management `inviteuser` (precheck `ALREADY_MEMBER` / `ALREADY_REGISTERED`), `listinvitations`, `listmembers`, `searchmembercandidates`; T06 dan T03 yang sudah ada tetap jalan tanpa perubahan kode.
Kontrak REST/OpenAPI: tanpa prefix versi; `info.version` auth.yml dinaikkan oleh foundation. Skema `User.invitation` dan `Me.email_enabled` ditulis di plan ini (bagian 5).

---

## 2. Summary Plan

Menambah penanda "akun ini masih undangan menunggu" di semua jalur baca auth tanpa mengubah perilaku lama:

1. `getuserbus`: `Request.Email` (lookup by email bila `UserID` kosong, dinormalisasi) dan `Response.Invitation *getuser.Invitation` (nil bila tidak ada undangan menunggu).
2. `finddevelopers`: `LEFT JOIN user_invitations` -> `Developer.InvitationPending`.
3. `listusers`: satu query dengan `LEFT JOIN` (tanpa N+1) -> `UserDTO.invitation`.
4. `getuser` (HTTP): `UserDTO.invitation` lewat `FindInvitation` (satu baris, dua query tetap murah).
5. `getme`: `email_enabled` dari `d.Mailer.Configured()` (K9), lewat DTO respons khusus `getme` supaya DTO `MeDTO` yang dipakai login tidak ikut berubah.

Semantik "undangan menunggu" = baris `user_invitations` ada dan `accepted_at IS NULL`. Undangan kedaluwarsa tetap "menunggu" dengan `expired=true` (login ditolak, tetapi operator perlu melihat dan mengirim ulang). Akun tanpa baris atau yang sudah diterima: `invitation = null` / `Invitation == nil`. Rule "pending/expired" ada di entity (`Invitation.IsPending`, `IsExpired`), bukan di SQL atau usecase; SQL hanya membawa kolom.

Prinsip aditif: field baru JSON selalu hadir (`invitation: null`, bukan dihilangkan) supaya FE punya bentuk stabil; konsumen lama (T01 FE, T03 management) mengabaikan field asing. Tes lama harus tetap hijau; hanya tes yang membandingkan JSON/struct persis yang diperbarui.

---

## 3. Plan (langkah berurutan)

Urutan TDD: tulis tes per langkah dulu, lalu implementasi. Setelah tiap langkah `go build ./... && go test ./internal/domain/core/...` hijau.

### 3.1 Entity dan DTO murni (`base/`)

File `base/invitation.go` (dibuat foundation 4.6a; tambahkan di bawah entity yang ada):

```go
// InvitationDTO adalah penanda undangan pada respons user (tanpa rahasia).
type InvitationDTO struct {
    Pending        bool       `json:"pending"`          // selalu true bila objek ada
    InvitedAt      time.Time  `json:"invited_at"`       // = Invitation.CreatedAt
    SentAt         *time.Time `json:"sent_at"`
    ExpiresAt      time.Time  `json:"expires_at"`
    Expired        bool       `json:"expired"`
    DeliveryStatus string     `json:"delivery_status"`
    ResendCount    int        `json:"resend_count"`
}

// ToResponse mengembalikan nil bila undangan sudah diterima (bukan menunggu).
func (i Invitation) ToResponse(now time.Time) *InvitationDTO
```

Rule: `!i.IsPending()` -> nil; selain itu `Pending=true`, `Expired=i.IsExpired(now)`, `InvitedAt=i.CreatedAt`. Tidak ada `issued_at` di DTO HTTP (hanya di kontrak bus).

File `base/user.go` (aditif):

- `UserDTO` tambah `Invitation *InvitationDTO \`json:"invitation"\`` (tanpa `omitempty`, jadi `null`).
- `func (u User) ToResponseWithInvitation(inv *Invitation, now time.Time) UserDTO`: memanggil `u.ToResponse()` lalu mengisi `Invitation` dari `inv.ToResponse(now)` (inv nil -> nil). `ToResponse()` lama tetap ada dan menghasilkan `Invitation=nil` (dipakai `createuser`, `updateuser`, `setuserstatus`, `resetpassword`; mereka tidak berubah. Respons `updateuser`/`setuserstatus` ikut punya kunci `invitation: null`, diterima karena aditif; bila ingin akurat, dikerjakan di usecase masing-masing di luar plan ini).
- Fungsi bus-side: `func (i Invitation) ToContract(now time.Time) *getuser.Invitation` diletakkan di `getuserbus` (bukan base, agar base tidak mengimpor kontrak baru selain yang sudah ada). Mapping: `InvitedAt=CreatedAt`, `IssuedAt`, `ExpiresAt`, `SentAt`, `Expired=IsExpired(now)`, `DeliveryStatus`, `ResendCount`; nil bila `!IsPending()`.

### 3.2 `getuserbus` (kontrak getuser aditif)

Kontrak `backend/lib/contracts/core/auth/getuser/getuser.go` sudah ditambah foundation (4.7). Tambahkan di doc comment: "UserID diprioritaskan; Email dipakai hanya bila UserID kosong (setelah trim). Email tidak valid -> Found=false tanpa error."

- `usecase.go`: `Execute(ctx, in getuser.Request)` (ganti signature dari `userID string`; handler menyesuaikan). `NewUsecase(repo, clock base.Clock)`.
  1. `id := strings.TrimSpace(in.UserID)`.
  2. `id != ""` -> `repo.FindUserByID`.
  3. `id == ""` -> `email, err := base.NormalizeEmail(in.Email)`; error normalisasi (kosong/format salah) -> `Response{Found:false}, nil`; selain itu `repo.FindUserByEmail`.
  4. Tidak ketemu -> `Found:false`.
  5. Ketemu -> `repo.FindInvitation(ctx, user.ID)`; isi `Response.Invitation` lewat `toContract(inv, now)`. Error repo dibungkus `fmt.Errorf("getuserbus: ...: %w", err)`.
  Usecase hanya orkestrasi; tidak ada rule baru (pending/expired dari entity).
- `repository.go`: interface `userFinder` ditambah `FindUserByEmail(ctx, email)` dan `FindInvitation(ctx, userID) (base.Invitation, bool, error)`; `Repository` membungkus `store.Store` (SQL sudah ada di store: `FindUserByEmail` T01 dan `FindInvitation` foundation 4.6e). Tidak ada SQL baru.
- `handler.go`: `h.uc.Execute(ctx, in)` (meneruskan seluruh Request).
- `init.go`: `Init(d, sh)` memakai `sh.Clock`: `NewUsecase(NewRepository(d.DB), sh.Clock)`.

### 3.3 `finddevelopers`

- `repository.go`: `developerRow` tambah `InvitationPending bool \`db:"invitation_pending"\``. Query:

```sql
SELECT u.id::text AS id, u.name, u.email::text AS email,
       (i.user_id IS NOT NULL) AS invitation_pending
FROM users u
LEFT JOIN user_invitations i ON i.user_id = u.id AND i.accepted_at IS NULL
WHERE u.role = 'developer'
  AND u.is_active
  AND (u.email ILIKE $1 ESCAPE '\' OR u.name ILIKE $1 ESCAPE '\')
ORDER BY u.name ASC, u.id ASC
LIMIT $2
```
  Relasi `user_id` adalah PK, jadi join paling banyak satu baris per user (tidak menggandakan hasil, urutan/limit tak berubah). Undangan kedaluwarsa tetap `InvitationPending=true`.
- `usecase.go` tidak berubah selain pemetaan (usecase meneruskan slice dari repository apa adanya).
- Kontrak `Developer` sudah punya `InvitationPending bool` dari foundation.

### 3.4 `listusers`

- `repository.go`: beri alias `u` dan prefix semua kolom (kolom `created_at` ada di dua tabel, tanpa prefix ambigu):

```sql
SELECT u.id::text AS id, u.email::text AS email, u.name, u.role, u.is_active, u.must_change_password,
       u.created_at, u.created_by::text AS created_by, u.created_by_label, u.updated_at, u.last_login_at,
       i.user_id IS NOT NULL AS inv_present,
       i.created_at AS inv_created_at, i.issued_at AS inv_issued_at, i.expires_at AS inv_expires_at,
       i.sent_at AS inv_sent_at, i.delivery_status AS inv_delivery_status,
       i.resend_count AS inv_resend_count,
       COUNT(*) OVER() AS total
FROM users u
LEFT JOIN user_invitations i ON i.user_id = u.id AND i.accepted_at IS NULL
```
  `where()` memakai `u.role`, `u.is_active`, `u.email`, `u.name`. `ORDER BY u.name ASC, u.id ASC`. Query hitung cadangan: `SELECT count(*) FROM users u` + clause (tanpa join; filter tidak menyentuh tabel undangan).
- `userRow` tambah kolom `inv_*` bertipe pointer/NULL-able (`*time.Time`, `*string`, `*int`) plus `InvPresent bool`.
- Repository mengembalikan hasil gabungan. Karena `Repository.List` saat ini mengembalikan `[]base.User`, ubah ke tipe baru di `base`:

```go
// UserWithInvitation menggandeng user dan undangan menunggu (nil bila tidak ada).
type UserWithInvitation struct { User User; Invitation *Invitation }
List(ctx, f) (items []base.UserWithInvitation, total int, err error)
```
  Entity `Invitation` yang direkonstruksi dari kolom join hanya mengisi field yang dipakai DTO (`UserID`, `CreatedAt`, `IssuedAt`, `ExpiresAt`, `SentAt`, `DeliveryStatus`, `ResendCount`); `AcceptedAt=nil` (selalu, karena filter join). `UserWithInvitation` ditaruh di `base/user.go`.
- `usecase.go`: `NewUsecase(repo, clock base.Clock)`; `now := u.clock()` satu kali per request; `items[i] = x.User.ToResponseWithInvitation(x.Invitation, now)`.
- `init.go`: `NewUsecase(NewRepo(d.DB), sh.Clock)`.
- Handler tidak berubah (`Output.Items` sudah `[]base.UserDTO`).

### 3.5 `getuser` (HTTP)

- `repository.go`: interface `userFinder` tambah `FindInvitation`; `Repository` meneruskan ke store.
- `usecase.go`: `NewUsecase(repo, clock)`. `Execute(ctx, id)`: `FindUserByID` -> not found -> `base.ErrUserNotFound`; lalu `FindInvitation(ctx, user.ID)`; `inv` ada -> `&inv` else nil; return `user.ToResponseWithInvitation(invPtr, u.clock())`.
- `init.go`: teruskan `sh.Clock`.

### 3.6 `getme` (`email_enabled`, K9)

- `usecase.go`: DTO respons khusus feature (meng-embed `base.MeDTO` supaya JSON datar dan `MeDTO` milik login/lainnya tidak berubah):

```go
type Output struct {
    base.MeDTO
    EmailEnabled bool `json:"email_enabled"`
}
type mailStatus interface{ Configured() bool }
func NewUsecase(m mailStatus) *Usecase
```
  `Execute` mengembalikan `Output{MeDTO: base.MeFromIdentity(id), EmailEnabled: u.mail != nil && u.mail.Configured()}`. Tanpa identitas -> `errs.CodeUnauthorized` seperti sebelumnya. Tanpa I/O (hanya membaca flag).
- `handler.go`: tipe kembalian usecase disesuaikan; `Cache-Control: no-store` tetap.
- `init.go`: `NewUsecase(d.Mailer)`.
- Semantik: driver `smtp` dan `log` = true; noop/kosong = false (foundation 4.3). FE memakai ini untuk menyembunyikan janji "email akan dikirim" dan menyiapkan dialog kredensial.

### 3.7 Dokumentasi dan kontrak

- `api/modules/core/auth.yml`: skema `UserInvitation` (object nullable) dan properti `User.invitation` (wajib, nullable); properti `Me.email_enabled` (boolean, wajib); enum `delivery_status` memakai `InvitationStatus` dari foundation. Jalankan `make api-bundle` dan `make api-check`; commit `api/openapi.yml`. (Bagian yang sama diedit usecase lain pada path berbeda; pada file yang sama, edit bagian ini saja agar merge bersih.)
- `docs/tickets/README.md`: tidak diubah di sini (katalog bus sudah dicakup foundation).

---

## 4. Migration

Tidak ada. Memakai tabel `user_invitations` dari migrasi core foundation (bagian 5 overview). Tes integrasi usecase ini bergantung pada migrasi itu sudah ada.

---

## 5. Changes (per file/folder)

Semua path relatif ke `backend/internal/domain/core/internal/auth/` kecuali disebut lain.

| File | Aksi | Isi |
|---|---|---|
| `base/invitation.go` | ubah | `InvitationDTO`, `Invitation.ToResponse(now)` |
| `base/invitation_test.go` | ubah | tes `ToResponse` |
| `base/user.go` | ubah | `UserDTO.Invitation`, `UserWithInvitation`, `User.ToResponseWithInvitation` |
| `base/user_test.go` | ubah | tes DTO JSON `invitation: null` dan terisi |
| `getuserbus/usecase.go`, `repository.go`, `handler.go`, `init.go` | ubah | Request.Email, Invitation, clock |
| `getuserbus/{usecase,handler,integration}_test.go` | ubah | tes baru (bagian 6) |
| `finddevelopers/repository.go` | ubah | LEFT JOIN + kolom `invitation_pending` |
| `finddevelopers/{usecase,integration}_test.go` | ubah | tes baru |
| `listusers/repository.go`, `usecase.go`, `init.go` | ubah | join beralias, `UserWithInvitation`, clock |
| `listusers/{usecase_test,repository_integration_test}.go`, `listusers_integration_test.go` (root auth) | ubah | sesuaikan signature dan tes baru |
| `getuser/repository.go`, `usecase.go`, `init.go` | ubah | invitation, clock |
| `getuser/{usecase_test,handler_test,integration_test}.go` | ubah | tes baru |
| `getme/usecase.go`, `handler.go`, `init.go` | ubah | `Output`, `email_enabled` |
| `getme/{usecase_test,handler_test,integration_test}.go` | ubah | `NewUsecase(&mailertest.Fake{})`, tes true/false |
| `backend/lib/contracts/core/auth/getuser/getuser.go` | ubah (doc saja) | catatan prioritas UserID dan Email invalid (struct sudah dari foundation) |
| `api/modules/core/auth.yml`, `api/openapi.yml` | ubah | skema `User.invitation`, `Me.email_enabled` |

Tidak disentuh: `createuser`, `updateuser`, `setuserstatus`, `resetpassword` (tetap memakai `ToResponse()`), `resolvesession`, `login`, `changepassword`, `base/store/store.go` (sudah dari foundation).

Aturan arsitektur dicek: tidak ada import antar feature; SQL baru hanya di `finddevelopers/repository.go` dan `listusers/repository.go` (repository masing-masing); getuser dan getuserbus memakai SQL store yang dipakai >1 feature; interaksi lintas module tetap bus.

---

## 6. Test Scenario

### Unit (tanpa DB, repo fake)

`base`:
- `Invitation.ToResponse`: pending aktif -> `Pending=true, Expired=false`; `now == ExpiresAt` -> `Expired=true`; `AcceptedAt != nil` -> nil; `InvitedAt` = `CreatedAt`.
- `User.ToResponseWithInvitation(nil, now)` -> `Invitation=nil`, JSON memuat `"invitation":null`; dengan undangan -> kunci lengkap bertipe benar; tidak ada `issued_at`/rahasia/password.

`getuserbus` (`usecase_test.go`, repo fake + clock tetap):
- UserID valid, tanpa undangan -> `Found`, `Invitation==nil`.
- UserID dengan undangan menunggu -> `Invitation` terisi, `Expired=false`, `IssuedAt` dan `DeliveryStatus` benar.
- Undangan kedaluwarsa -> `Invitation!=nil`, `Expired=true`.
- Undangan sudah diterima -> `Invitation==nil`.
- Email `"  Baru@Contoh.ID "` dengan UserID kosong -> repo dipanggil dengan `baru@contoh.id`, `Found`.
- UserID dan Email keduanya diisi -> UserID menang (repo email tidak dipanggil).
- Email kosong / format salah dan UserID kosong -> `Found=false`, tanpa error, repo tidak dipanggil.
- Email tidak ada -> `Found=false`.
- User nonaktif -> `Found=true, IsActive=false` (perilaku lama).
- Error repo (user maupun invitation) -> error terbungkus `%w`.
- `handler_test.go`: payload bukan `getuser.Request` -> error `bus.Expect`; Request berisi Email diteruskan lengkap.

`finddevelopers`: usecase meneruskan `InvitationPending` dari repo fake tanpa mengubahnya; kasus lama (query kosong, clamp limit, slice non-nil) tetap lulus.

`listusers` (`usecase_test.go`): item dengan dan tanpa undangan dipetakan benar; `now` dibaca sekali (clock fake menghitung panggilan = 1); error filter/repo seperti sebelumnya.

`getuser` (`usecase_test.go`): not found -> `ErrUserNotFound`; ada undangan -> `invitation` terisi; tanpa -> nil; error `FindInvitation` -> terbungkus.

`getme`:
- `handler_test`: Fake `NotConfigured=false` -> JSON memuat `"email_enabled":true`; `NotConfigured=true` -> `false`; kunci lama (`id,email,name,role,must_change_password`) tetap utuh; `Cache-Control: no-store`; tanpa identitas -> 401.
- `usecase_test`: `NewUsecase(nil)` -> `EmailEnabled=false` tanpa panic.

### Integrasi (`-tags integration`, `db.TestDB` + migrasi)

Fixture: A = akun undangan menunggu (user + baris `user_invitations`), B = undangan diterima (`accepted_at` terisi), C = akun `createuser` (tanpa baris), D = undangan kedaluwarsa (`expires_at` lampau), E = developer nonaktif.

- `getuserbus` integrasi: Email -> A mengembalikan `Invitation` benar; B dan C `nil`; D `Expired=true`; kontrak: `Request{UserID: A.ID}` dan `Request{Email: A.Email}` menghasilkan `Response` identik (tes kontrak Email vs UserID); email huruf besar sama dengan huruf kecil; id bukan uuid -> `Found=false`.
- `finddevelopers` integrasi: A dan D `InvitationPending=true`; B dan C `false`; E tidak muncul; urutan dan limit identik dengan sebelum perubahan (tidak ada baris ganda).
- `listusers` repository integrasi: satu halaman memuat A..D dengan `Invitation` benar; `total` benar dengan filter `role`, `is_active`, `q`; offset melewati data -> jalur count cadangan tetap benar dengan alias `u`; filter `q` yang cocok dengan kolom `created_at`-ambigu tidak error (regresi alias). Hitung query: gunakan wrapper `Querier` penghitung di tes untuk menegaskan tepat satu `SelectContext` per `List` (tidak ada N+1).
- `getuser` integrasi: A -> `invitation` terisi; B/C -> `null`; id tidak ada -> 404.
- `getme` integrasi (yang ada) disesuaikan: respons memuat `email_enabled` sesuai Deps (`mailer.New` config kosong -> false).

Regresi wajib hijau: seluruh tes T01 (`auth_integration_test`, `listusers_integration_test`, tes T03/T06 yang memanggil `get-user`/`find-developers`).

---

## 7. E2E Scenarios

Positif:
1. Super Admin membuka halaman Users setelah mengundang `baru@contoh.id`: `GET /api/users` mengembalikan item dengan `invitation.pending=true`, `delivery_status="sent"`; baris akun lain `invitation:null`.
2. Penerima login dan mengganti password (`auth-login-changepassword`): `GET /api/users/{id}` mengembalikan `invitation:null`.
3. Undangan dibiarkan lewat `expires_at`: `GET /api/users/{id}` menampilkan `invitation.expired=true`; setelah kirim ulang `expired=false` dan `resend_count` naik.
4. Management mencari kandidat: item developer berundangan menunggu punya `invitation_pending=true` (melalui bus `find-developers`); developer biasa `false`.
5. Management `inviteuser` memanggil `get-user {Email}`: akun menunggu ditemukan dengan `Invitation`; email baru -> `Found=false`.
6. `GET /api/auth/me` pada deploy tanpa SMTP: `email_enabled=false`; dengan Mailpit/SMTP aktif: `true`.

Negatif:
1. Developer (bukan super_admin) memanggil `GET /api/users` atau `GET /api/users/{id}` -> 403 (tidak berubah); tanpa sesi -> 401.
2. `GET /api/users/{id}` id tidak ada / bukan uuid -> 404 `NOT_FOUND`.
3. `get-user` bus dengan UserID dan Email kosong -> `Found=false`, bukan error.
4. `GET /api/auth/me` tanpa sesi -> 401 `UNAUTHORIZED`.
5. Tidak ada kunci rahasia (`temporary_password`, hash, token) di respons manapun dari usecase ini.

---

## 8. Request/Response body

Semua snake_case. Semua GET tanpa body request. Header `Cache-Control: no-store`.

### 8.1 `GET /api/users?role=developer&limit=20` (200)

```json
{
  "items": [
    {
      "id": "7c1f3c2e-5b7a-4d7e-9a51-1f0b8a3e6d21",
      "email": "baru@contoh.id",
      "name": "baru",
      "role": "developer",
      "is_active": true,
      "must_change_password": true,
      "created_at": "2026-10-21T08:15:00Z",
      "created_by": "11111111-2222-4333-8444-555555555555",
      "created_by_label": "Owner Satu",
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
    {
      "id": "9a0d4b6e-1c2f-4e3a-8b7c-6d5e4f3a2b10",
      "email": "budi@contoh.id",
      "name": "Budi",
      "role": "developer",
      "is_active": true,
      "must_change_password": false,
      "created_at": "2026-09-01T02:00:00Z",
      "created_by": null,
      "created_by_label": null,
      "updated_at": "2026-09-01T02:00:00Z",
      "last_login_at": "2026-10-20T07:30:00Z",
      "invitation": null
    }
  ],
  "total": 2,
  "limit": 20,
  "offset": 0
}
```

### 8.2 `GET /api/users/{id}` (200, undangan kedaluwarsa dengan email gagal)

```json
{
  "id": "7c1f3c2e-5b7a-4d7e-9a51-1f0b8a3e6d21",
  "email": "baru@contoh.id",
  "name": "baru",
  "role": "developer",
  "is_active": true,
  "must_change_password": true,
  "created_at": "2026-10-01T08:15:00Z",
  "created_by": "11111111-2222-4333-8444-555555555555",
  "created_by_label": "Owner Satu",
  "updated_at": "2026-10-01T08:15:00Z",
  "last_login_at": null,
  "invitation": {
    "pending": true,
    "invited_at": "2026-10-01T08:15:00Z",
    "sent_at": null,
    "expires_at": "2026-10-08T08:15:00Z",
    "expired": true,
    "delivery_status": "failed",
    "resend_count": 1
  }
}
```

### 8.3 `GET /api/auth/me` (200)

```json
{
  "id": "9a0d4b6e-1c2f-4e3a-8b7c-6d5e4f3a2b10",
  "email": "owner@contoh.id",
  "name": "Owner Satu",
  "role": "developer",
  "must_change_password": false,
  "email_enabled": false
}
```

### 8.4 Bus `core.auth.get-user` (struct Go, bukan HTTP)

Request by email:
```json
{ "UserID": "", "Email": " Baru@Contoh.ID " }
```
Response (undangan menunggu):
```json
{
  "Found": true, "ID": "7c1f3c2e-5b7a-4d7e-9a51-1f0b8a3e6d21", "Name": "baru", "Email": "baru@contoh.id",
  "Role": "developer", "IsActive": true,
  "Invitation": {
    "InvitedAt": "2026-10-21T08:15:00Z", "IssuedAt": "2026-10-21T08:15:00Z",
    "ExpiresAt": "2026-10-28T08:15:00Z", "SentAt": "2026-10-21T08:15:01Z",
    "Expired": false, "DeliveryStatus": "sent", "ResendCount": 0
  }
}
```
Tidak ditemukan: `{ "Found": false, "Invitation": null }` (tanpa error).

### 8.5 Bus `core.auth.find-developers` (Developer)

```json
{ "Items": [ { "ID": "7c1f3c2e-5b7a-4d7e-9a51-1f0b8a3e6d21", "Name": "baru", "Email": "baru@contoh.id", "InvitationPending": true } ] }
```

### 8.6 Error (tidak berubah, envelope `httpx.WriteError`)

401:
```json
{ "error": { "code": "UNAUTHORIZED", "message": "perlu login" } }
```
403:
```json
{ "error": { "code": "FORBIDDEN", "message": "akses ditolak" } }
```
404 (`GET /api/users/{id}`):
```json
{ "error": { "code": "NOT_FOUND", "message": "user tidak ditemukan" } }
```
400 (`GET /api/users?limit=abc`, perilaku T01):
```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "permintaan tidak valid", "details": { "fields": { "limit": "harus berupa angka" } } } }
```
Teks `message` persis mengikuti `base.ErrUserNotFound` dan `base.InvalidFields` yang ada; contoh menunjukkan bentuk, bukan teks baru.

---

## 9. Risiko dan catatan

- Perubahan signature `getuserbus.Usecase.Execute`, `listusers.Repository.List`, dan `NewUsecase` di empat feature memaksa pembaruan tes yang ada; ini internal module (tidak ada konsumen luar), tetapi harus selesai dalam satu commit agar build hijau.
- Alias `u.` di `listusers`: risiko kolom ambigu hanya muncul di runtime; tes integrasi filter `q` dan jalur count cadangan wajib ada.
- `email_enabled=true` untuk driver `log` (dev). Itu disengaja (overview 4.3: `Configured()=true`).
- Undangan kedaluwarsa tetap `invitation_pending=true` di `find-developers` dan `Invitation != nil` di `get-user`; management memutuskan tampilan (mis. menandai "kedaluwarsa" lewat `Expired`). Keputusan ini mengikuti overview 3.3 dan tidak menunggu persetujuan tambahan.
- `updateuser` dan `setuserstatus` masih memakai `ToResponse()` sehingga `invitation` selalu `null` di respons keduanya, walau akun itu undangan menunggu. FE harus memuat ulang daftar/detail setelah aksi tersebut (perilaku T01 sudah memuat ulang); tidak dianggap bug.
