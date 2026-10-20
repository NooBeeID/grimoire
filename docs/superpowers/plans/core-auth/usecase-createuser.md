# T01 core/auth — Usecase `createuser`

Konteks bersama (migrasi, config, `base/`, `base/store`, `base/audit`, kontrak, konstanta, OpenAPI, `Shared`) ada di `overview.md` dan dikerjakan di langkah foundation F1. Dokumen ini hanya memuat usecase `createuser`. Tiket: `docs/tickets/T01-core-auth.md`. Bila dokumen ini bertentangan dengan `overview.md`, `overview.md` menang. Plan ini sengaja satu usecase utuh (satu folder feature, satu PR); tidak dipecah.

## Metadata

| Field | Nilai |
|---|---|
| Ticket | T01 |
| Scope | fullstack; plan ini hanya sisi backend (Go). UI dialog "Buat user" ada di `frontend-plan.md` |
| Domain | core |
| Module | auth |
| Feature (folder) | `createuser` |
| Resource root | `backend/internal/domain/core/internal/auth` |
| Gelombang | C (setelah F1 dan gelombang A/B; secara kode hanya bergantung pada F1) |

| Endpoint | Method | Version | Auth | Password gate | Rate limit |
|---|---|---|---|---|---|
| `/api/users` | POST | tanpa prefix versi (`info.version` 0.1.0 di `auth.yml`) | `RequireAuth` + `RequireRole(super_admin)` | dicek (user dengan `must_change_password` mendapat 403 `PASSWORD_CHANGE_REQUIRED`) | tidak ada khusus (hanya batas global T00) |

Tidak ada endpoint bus, job, atau CLI untuk feature ini. Kontrak bus yang DIKONSUMSI: `core.audit.record` (via `base/audit`, mode Call, fail-open). Kontrak bus yang disediakan: tidak ada.

## Summary Plan

`POST /api/users` membuat akun user baru oleh Super Admin.

Alur usecase (orkestrasi saja, rule ada di `base/`):
1. Ambil actor dari `reqctx.IdentityFrom(ctx)`.
2. Bangun entity user lewat `base.NewUser` (normalisasi dan validasi email/nama/role; flag `must_change_password=true`, `is_active=true`, `created_by`, `created_by_label`).
3. Password: bila kosong, server membangkitkan lewat `base.GeneratePassword(Shared.Rand)` dan menandai `generated=true`; bila diisi, divalidasi `PasswordPolicy`. Semua kesalahan validasi dikumpulkan jadi satu `INVALID_ARGUMENT` dengan `details.fields`, SEBELUM hashing (hashing argon2 mahal).
4. Hash argon2id lewat `base.Hasher`.
5. Satu `INSERT ... RETURNING` lewat repository. Unique violation `users_email_key` dipetakan repository ke `base.ErrEmailTaken`, usecase mengubahnya menjadi 409 `CONFLICT` "email sudah terdaftar". Tidak ada pre-check `SELECT` (rawan race; constraint unik adalah sumber kebenaran). Tidak perlu transaksi eksplisit karena hanya satu statement.
6. Setelah insert sukses, kirim audit `user.created` lewat `audit.Recorder` (Call, fail-open, tanpa password).
7. Handler membalas 201 dengan `user` dan `temporary_password` (string hanya bila dibangkitkan server; selain itu `null`), header `Cache-Control: no-store`.

Keamanan:
- Password (input maupun hasil bangkit) tidak pernah masuk log, audit, atau pesan error. Field request memakai `base.Secret`; `temporary_password` hanya ada di struct respons dan tidak pernah dilewatkan ke logger atau `audit.Event`.
- Hash tidak ikut respons (`UserDTO` tidak punya field hash).
- Enumerasi email lewat 409 dapat diterima: endpoint hanya untuk Super Admin.
- Duplikat email dibedakan huruf besar/kecil tidak mungkin: kolom `citext` plus normalisasi lowercase di aplikasi.

## Plan

### Langkah 0 — Prasyarat (cek, jangan dibuat dari dalam folder `createuser/`)

Pastikan F1 sudah ada. Bila ada yang belum, selesaikan di F1 (PR foundation), bukan di PR ini:
- Migrasi `backend/migrations/core/20261021100000000_create_auth_tables.sql` (tabel `users` dengan `created_by`, `created_by_label`, `users_email_key`).
- `base/` lengkap (`NormalizeEmail`, `ValidateName`, `ValidRole`, `User.ToResponse`, `PasswordPolicy`, `Secret`, `Hasher`, `GeneratePassword`, `FieldErrors`, `InvalidFields`, `EmailConflict`, `ErrEmailTaken`, `RequestMeta`, `Clock`, `Shared`).
- `base/store` dengan `InsertUser` (id kosong memakai default DB `gen_random_uuid()`; `RETURNING` seluruh kolom; 23505 pada `users_email_key` menjadi `base.ErrEmailTaken`).
- `base/audit` dengan `Recorder`, `Event`, `BusRecorder`.
- Konstanta `constants.BUS_CORE_AUDIT_RECORD` dan kontrak `lib/contracts/core/audit/record` (milik T02; yang merge duluan membuatnya).
- `auth/base.go` dengan `Register(d)`, `buildShared`, `registerCodes`.
- Path import setelah T00 merge (berbeda dari catatan WIP di overview): `httpx` = `grimoire/internal/infra/http` (WriteJSON, WriteError, DecodeJSON); middleware ada di package `middleware` = `grimoire/internal/infra/middleware` (`RequireAuth`, `RequireRole`, `KeyByIP`); `db` = `grimoire/internal/infra/db`; `reqctx` = `grimoire/lib/common/reqctx`. Plan ini memakai path tersebut.

Tambahan kecil pada `base/user.go` (F1; bila belum ada, tambahkan di PR F1 atau di PR pertama yang butuh, lalu dipakai bersama `createadmin`):

```go
// NewUser memvalidasi input dan membentuk entity yang siap di-insert (tanpa ID dan PasswordHash).
// Rule bisnis pembuatan user tinggal di sini, bukan di usecase. Semua kesalahan field dikumpulkan.
type NewUserInput struct {
    Email, Name, Role string
    CreatedBy, CreatedByLabel string // actor
    Now time.Time
}
func NewUser(in NewUserInput) (User, FieldErrors)
//   Email -> NormalizeEmail   (gagal: "wajib diisi" bila kosong, selain itu "format email tidak valid")
//   Name  -> ValidateName     (gagal: "wajib diisi" bila kosong, "maksimal 100 karakter", "tidak boleh berisi karakter kontrol")
//   Role  -> ValidRole        (gagal: "role harus super_admin atau developer"; role kosong juga gagal)
//   hasil: IsActive=true, MustChangePassword=true, CreatedBy/CreatedByLabel diisi, CreatedAt=UpdatedAt=Now
//   error tiap field tidak menghentikan pengecekan field lain.
func (p PasswordPolicy) ValidateField(field, pw, email string) FieldErrors // dari overview 4.3
```

`createadmin` memakai `NewUser` yang sama dan menimpa `MustChangePassword=false` serta `Role=super_admin`.

### Langkah 1 — Test dulu (TDD), lalu implementasi, semua di `backend/internal/domain/core/internal/auth/createuser/`

1. **`params.go`**

```go
package createuser

type CreateUserRequest struct {
    Email    string      `json:"email"`
    Name     string      `json:"name"`
    Role     string      `json:"role"`
    Password base.Secret `json:"password"` // opsional; "" atau tidak dikirim = bangkitkan
}

// Input usecase (tanpa *http.Request).
type Input struct {
    Email, Name, Role string
    Password          base.Secret
    Meta              base.RequestMeta
}

type Result struct {
    User              base.UserDTO `json:"user"`
    TemporaryPassword *string      `json:"temporary_password"` // tanpa omitempty: null bila password diberikan admin
}
```

   `Result` TIDAK dilog dan tidak dimasukkan ke `audit.Event`. Satu-satunya jalur keluar adalah `WriteJSON` di handler.

2. **`repository.go`** (satu-satunya yang menyentuh SQL; di sini hanya mendelegasikan ke query bersama di `base/store` karena SQL insert identik dipakai `createadmin`)

```go
type Repo struct{ s *store.Store }
func NewRepo(q db.Querier) *Repo
func (r *Repo) Insert(ctx context.Context, u base.User) (base.User, error) // return r.s.InsertUser(ctx, u); error dikembalikan apa adanya (ErrEmailTaken atau infra)
```

   Tidak ada SQL baru di file ini. Interface yang dipakai usecase didefinisikan di `usecase.go` (sisi consumer, rule 5.6).

3. **`usecase.go`**

```go
type Repository interface {
    Insert(ctx context.Context, u base.User) (base.User, error)
}

type Usecase struct {
    repo   Repository
    hasher base.Hasher
    audit  audit.Recorder
    clock  base.Clock
    rand   io.Reader
    policy base.PasswordPolicy
}
func NewUsecase(repo Repository, hasher base.Hasher, a audit.Recorder, clock base.Clock, rnd io.Reader, policy base.PasswordPolicy) *Usecase
func (u *Usecase) Execute(ctx context.Context, in Input) (Result, error)
```

   Alur `Execute`:
   1. `actor, ok := reqctx.IdentityFrom(ctx)`; `!ok` -> `errs.New(errs.CodeUnauthorized, "perlu login")` (pertahanan berlapis; router sudah menjaga).
   2. `user, fe := base.NewUser(base.NewUserInput{Email: in.Email, Name: in.Name, Role: in.Role, CreatedBy: actor.UserID, CreatedByLabel: actor.Email, Now: u.clock()})`.
   3. Password:
      - `pw := in.Password.Reveal()`; `generated := pw == ""`.
      - Bila `!generated` dan email valid (`user.Email != ""`): `fe.Merge(u.policy.ValidateField("password", pw, user.Email))`. Bila email tidak valid, password hanya dicek panjang/byte (tanpa pembanding email).
      - `len(fe) > 0` -> `return Result{}, base.InvalidFields(fe)` (belum ada hashing, belum ada DB, belum ada audit).
      - Bila `generated`: `pw, err = base.GeneratePassword(u.rand)`; error -> `fmt.Errorf("createuser: generate password: %w", err)`.
   4. `hash, err := u.hasher.Hash(pw)`; error -> `fmt.Errorf("createuser: hash: %w", err)` (pesan tidak memuat pw).
   5. `user.PasswordHash = hash`; `created, err := u.repo.Insert(ctx, user)`:
      - `errors.Is(err, base.ErrEmailTaken)` -> `base.EmailConflict()`;
      - error lain -> `fmt.Errorf("createuser: insert: %w", err)`.
   6. `u.audit.Record(ctx, audit.Event{Action: "user.created", TargetType: "user", Target: created.ID, Args: map[string]any{"email": created.Email, "name": created.Name, "role": created.Role, "generated_password": generated}, Status: record.StatusOK, Meta: in.Meta})`. Actor `nil` sehingga diambil dari `reqctx` (actor_type `user`, id = admin, label = email admin). Dipanggil setelah insert, tidak mengembalikan error (fail-open).
   7. `res := Result{User: created.ToResponse()}`; bila `generated`, `res.TemporaryPassword = &pw`. Return.

   Aturan tambahan: usecase tidak mengimpor `net/http`, tidak menyentuh SQL, tidak me-log apa pun yang berasal dari password. Penegakan `super_admin` hanya di router (`RequireRole`); usecase tidak mengulang cek role.

4. **`handler.go`**

```go
type Handler struct{ uc *Usecase }

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
    var req CreateUserRequest
    if err := httpx.DecodeJSON(r, &req); err != nil { httpx.WriteError(w, r, err); return }
    res, err := h.uc.Execute(r.Context(), Input{
        Email: req.Email, Name: req.Name, Role: req.Role, Password: req.Password,
        Meta: base.RequestMeta{IP: middleware.KeyByIP(r), UserAgent: truncate(r.UserAgent(), 512)},
    })
    if err != nil { httpx.WriteError(w, r, err); return }
    w.Header().Set("Cache-Control", "no-store") // selalu, bukan hanya saat ada temporary_password (seragam dan lebih aman)
    httpx.WriteJSON(w, http.StatusCreated, res)
}
```

   Catatan terverifikasi dari kode T00 (merged): package `httpx` berada di `grimoire/internal/infra/http`, middleware di `grimoire/internal/infra/middleware`, `reqctx.Identity{UserID, Email, Name, Role, MustChangePassword}`, dan `DecodeJSON` memakai `DisallowUnknownFields`. `truncate` dipakai juga oleh feature lain; bila F1 sudah punya helper bersama di `base` (mis. `base.TruncateUA`), pakai itu. Jika belum, definisikan helper privat di handler ini dan JANGAN mengimpor feature lain (rule 4.3). Error validasi JSON (body rusak, tipe salah, terlalu besar) diurus `DecodeJSON`/`WriteError` sesuai perilaku T00 (kunci perilakunya dengan test handler, lihat bagian Test).

5. **`init.go`**

```go
func Init(d app.Deps, s base.Shared) {
    repo := NewRepo(d.DB)
    uc := NewUsecase(repo, s.Hasher, audit.New(d.Bus, d.Log), s.Clock, s.Rand, s.Password)
    h := &Handler{uc: uc}
    d.API.With(middleware.RequireAuth(), middleware.RequireRole(resolvesession.RoleSuperAdmin)).
        Post("/users", h.Create)
}
```

   Gunakan `d.API.With(...).Post(...)`, BUKAN `d.API.Route("/users", ...)`: feature `listusers` mendaftarkan `GET /users` pada path yang sama dan chi tidak mengizinkan dua `Route`/`Mount` pada pola yang sama. `With` hanya menempelkan middleware ke satu route sehingga aman digabung. Middleware global (`Timeout`, `CSRF`, `Session`, `PasswordChangeGate`) sudah dipasang T00/F1 pada grup `/api`; jangan dipasang ulang.

6. **`auth/base.go`**: tambahkan SATU baris `createuser.Init(d, shared)` pada daftar Init (import `.../auth/createuser`). Jangan mengubah baris feature lain.

### Langkah 2 — OpenAPI (kontrak dulu, rule 11.6)

Verifikasi `api/modules/core/auth.yml` sudah memuat operasi `POST /users` (operationId `createUser`), schema `CreateUserRequest`, `User`, `UserWithTempPassword`, respons 201/400/401/403/409/429/500, header wajib `X-Requested-With`, `security: sessionCookie`. Bila F1 belum lengkap, tambahkan di PR ini, jalankan `make api-bundle`, commit `api/openapi.yml`, pastikan `make api-check` lulus. Respons 201 mendokumentasikan header `Cache-Control: no-store`. Contoh respons 403 mencakup dua kode: `FORBIDDEN` dan `PASSWORD_CHANGE_REQUIRED`.

### Langkah 3 — Verifikasi

`go build ./...`, `go vet ./...`, `go test ./internal/domain/core/internal/auth/...`, `go test -tags integration` untuk test integrasi, `TestT00_AC4_RepoIsClean` (archtest: createuser hanya mengimpor `base`, `base/store`, `base/audit`, `lib/*`, `infra/{app,http,middleware,db}`; tidak mengimpor feature lain), `make api-check`.

## Migration

Tidak ada migrasi baru. Memakai tabel `users` dari `backend/migrations/core/20261021100000000_create_auth_tables.sql` (F1). Kolom yang ditulis: `email`, `name`, `password_hash`, `role`, `is_active` (default true), `must_change_password` (true), `created_by`, `created_by_label`, `created_at`, `updated_at`; `id` dari default DB. Tidak ada foreign key baru (`created_by` sengaja tanpa FK, rule 8.7).

## Changes

Baru (semua di `backend/internal/domain/core/internal/auth/createuser/`):
- `init.go`, `handler.go`, `params.go`, `usecase.go`, `repository.go`
- `usecase_test.go`, `handler_test.go`, `createuser_integration_test.go` (tag `integration`)

Diubah:
- `backend/internal/domain/core/internal/auth/base.go`: satu baris `createuser.Init(d, shared)` + import.

Diubah hanya bila belum ada dari F1 (ditandai di PR sebagai bagian F1, bukan logika createuser):
- `auth/base/user.go` (+ `user_test.go`): `NewUser`, `NewUserInput`.
- `auth/base/password.go` (+ test): `ValidateField`, `FieldErrors.Merge` (di `user.go`/`errors.go`).
- `api/modules/core/auth.yml`, `api/openapi.yml`: operasi `POST /users`.

Menambah baris di test lintas feature `auth/auth_integration_test.go` (dimiliki F1/W, hanya baris tabel): `createuser` pada tabel `TestT01_AC8_PasswordNeverLoggedOrAudited` dan `TestT01_AC9_EachActionOneAuditEntry`, serta `POST /api/users` pada `TestT01_AC7_DeveloperForbiddenOnUsers` dan `TestRoutesRegistered`.

Tidak diubah: infra T00, feature lain, `docs/tickets/*` (sudah ditangani F1).

## Test Scenario

Penamaan test AC: `TestT01_ACn_...` (rule 13.4). Test unit usecase memakai fake untuk `Repository`, `audit.Recorder`, dan `Hasher` (fake hasher mencatat input untuk memastikan plaintext tidak bocor ke tempat lain), `Clock` tetap, `Rand` deterministik. Rule yang menyangkut entity diuji di `base/` tanpa mock (sudah di F1).

### Unit usecase (`usecase_test.go`)

| Test | Skenario | Ekspektasi |
|---|---|---|
| `TestExecute_GeneratesPasswordWhenEmpty` | `Password==""` | `TemporaryPassword` terisi, panjang 16, hasher menerima nilai yang sama, `generated_password=true` di audit |
| `TestExecute_UsesProvidedPassword` | password valid diisi | `TemporaryPassword==nil`, hasher menerima password itu, `generated_password=false` |
| `TestExecute_PasswordTooShort` | 9 rune | `INVALID_ARGUMENT`, `fields.password="minimal 10 karakter"`, hasher/repo/audit tidak dipanggil |
| `TestExecute_PasswordEqualsEmail` | `password == email` beda huruf | `fields.password="password tidak boleh sama dengan email"` |
| `TestExecute_PasswordTooLong` | 257 byte | `fields.password="maksimal 256 byte"` |
| `TestExecute_CollectsAllFieldErrors` | email, name, role, password semua salah | satu error dengan keempat key di `fields`; repo tidak dipanggil |
| `TestExecute_InvalidEmailFormats` | `""`, `"a"`, `"a@"`, `"Budi <b@x.id>"`, 255 byte | `fields.email` terisi |
| `TestExecute_InvalidName` | kosong, spasi saja, 101 rune, karakter kontrol | `fields.name` terisi; 100 rune multibyte diterima |
| `TestExecute_InvalidRole` | `""`, `"admin"`, `"SUPER_ADMIN"` | `fields.role="role harus super_admin atau developer"` |
| `TestExecute_NormalizesEmail` | `" Budi@Tim.ID "` | repo menerima `budi@tim.id`, nama di-trim |
| `TestExecute_SetsFlagsAndCreator` | actor admin di ctx | user ke repo: `IsActive=true`, `MustChangePassword=true`, `CreatedBy=actor.UserID`, `CreatedByLabel=actor.Email`, `Role` sesuai input, `PasswordHash` = output hasher bukan plaintext |
| `TestExecute_EmailTakenMapsToConflict` | repo mengembalikan `ErrEmailTaken` | `*errs.Error` code `CONFLICT`, pesan `email sudah terdaftar`; audit TIDAK dipanggil |
| `TestExecute_RepoErrorIsWrapped` | repo error generik | error terbungkus `createuser: insert: ...`; bukan `*errs.Error` klien (jadi 500 generik); tidak memuat password |
| `TestExecute_HasherErrorNoLeak` | hasher gagal | error terbungkus tanpa plaintext; repo tidak dipanggil |
| `TestExecute_RandError` | `Rand` gagal | error terbungkus, tanpa insert |
| `TestExecute_NoIdentity` | ctx tanpa identity | `UNAUTHORIZED`, tanpa efek samping |
| `TestExecute_AuditEventShape` | sukses | tepat 1 event: `Action=user.created`, `TargetType=user`, `Target=<id>`, Args = `{email,name,role,generated_password}` saja, `Status=ok`, `Meta` diteruskan, `Actor==nil` |
| `TestExecute_AuditNeverContainsSecrets` | dua mode (generated dan provided) | marshal `Event` ke JSON; tidak memuat plaintext password maupun hash |
| `TestExecute_ResultHasNoHash` | sukses | marshal `Result` tidak memuat `password_hash` maupun nilai hash |

### Unit handler (`handler_test.go`, httptest, usecase asli dengan fake repo)

- `TestHandler_Created201`: status 201, `Content-Type` JSON, `Cache-Control: no-store`, body sesuai bentuk di bagian Request/Response.
- `TestHandler_TemporaryPasswordNullWhenProvided`: body memuat `"temporary_password":null` (kunci ada).
- `TestHandler_InvalidJSON400`: body rusak -> 400 `INVALID_ARGUMENT`.
- `TestHandler_WrongTypes400`: `{"email": 5}` -> 400.
- `TestHandler_UnknownFieldRejected`: `DecodeJSON` T00 memakai `DisallowUnknownFields` (terverifikasi di `respond.go`), jadi body dengan `is_active`, `must_change_password`, `password_hash`, atau `id` ditolak 400 `INVALID_ARGUMENT` pesan `field tidak dikenal`, `details.field=<nama>`; repo tidak dipanggil dan tidak ada baris dibuat.
- `TestHandler_EmptyBody400`: tanpa body -> 400 `body wajib diisi`.
- `TestHandler_ConflictMapsTo409`, `TestHandler_InternalErrorHidesDetail` (500 tanpa teks SQL/pgx/password).
- `TestHandler_SecretNotLoggedOnDecodeError`: body invalid yang memuat `password` tidak muncul di log (logger buffer).

### Integrasi (Postgres nyata via `db.TestDB(t)`, migrasi `migrations.FS` diterapkan; hasher argon2 parameter kecil)

`createuser_integration_test.go`:
- `TestCreateUser_PersistsRow`: setelah 201, baris ada: `email` lowercase, `role`, `is_active=true`, `must_change_password=true`, `created_by=<admin id>`, `created_by_label=<admin email>`, `last_login_at IS NULL`, `password_hash` berawalan `$argon2id$` dan `hasher.Verify(hash, temporary_password)==true`, `created_at≈now`.
- `TestCreateUser_ProvidedPasswordVerifies`: hash cocok dengan password yang dikirim.
- `TestCreateUser_DuplicateEmailCaseInsensitive`: `Sari@Tim.ID` lalu `sari@tim.id` -> 409; hanya satu baris.
- `TestCreateUser_DuplicateAgainstInactiveUser`: email milik user nonaktif tetap 409.
- `TestCreateUser_ConcurrentSameEmail`: 10 goroutine dengan email sama -> tepat satu 201, sisanya 409, tanpa 500.
- `TestCreateUser_RoleCheckConstraintNotReachable`: role invalid ditolak sebelum DB (400), bukan oleh CHECK.

`auth_integration_test.go` (router lengkap `httpx.NewRouter` + `core.Register`, bus nyata, handler `core.audit.record` palsu yang menampung request bila T02 belum merge):
- `TestT01_AC7_DeveloperForbiddenOnUsers` (baris `POST /api/users`): developer -> 403 `FORBIDDEN`, tidak ada baris baru, tidak ada audit.
- `TestCreateUser_AnonymousIs401`: tanpa cookie -> 401 `UNAUTHORIZED`.
- `TestCreateUser_MissingCSRFHeader403`: tanpa `X-Requested-With` -> 403 `CSRF_REQUIRED`.
- `TestT01_AC4_MustChangePasswordGate` (bagian createuser): admin berflag `must_change_password` memanggil `POST /api/users` -> 403 `PASSWORD_CHANGE_REQUIRED`; user tidak dibuat.
- `TestT01_AC9_EachActionOneAuditEntry` (baris createuser): tepat 1 `record.Request` `Action=user.created`, `ActorType=user`, `ActorID=<admin>`, `ActorLabel=<email admin>`, `TargetType=user`, `Target=<id baru>`, `Status=ok`, `IP` dan `UserAgent` terisi, `TraceID` sama dengan `trace_id` respons. Request gagal (400/409) menghasilkan 0 entri.
- `TestT01_AC8_PasswordNeverLoggedOrAudited` (baris createuser, dua mode): buffer log aplikasi dan JSON seluruh `record.Request` tidak memuat plaintext password (diberikan maupun dibangkitkan), prefiks `$argon2id$`, atau hash.
- `TestAudit_UnregisteredDoesNotFailRequest` (baris createuser): tanpa handler audit, `POST /api/users` tetap 201 dan baris user ada; log berisi `audit record gagal`.
- `TestCreateUser_NewUserCanLoginAndMustChange` (lintas feature; aktif setelah `login` dan `getme` merge): user baru login dengan password sementara -> `GET /api/auth/me` menunjukkan `must_change_password=true`.
- `TestRoutesRegistered`: `chi.Walk` memuat `POST /api/users`.

## E2E Scenarios

Playwright terhadap binary (dengan `grimoire migrate up` dan admin dari `create-admin`). Dialog UI diurus `frontend-plan.md`; di sini kasus yang menyentuh endpoint ini.

Positif:
1. Admin membuka Admin -> Users -> "Buat user", mengisi nama, email, role `developer`, password dikosongkan. Dialog sukses menampilkan password sementara sekali dengan tombol salin; baris user muncul di tabel dengan status aktif. Menutup dialog lalu membuka ulang detail tidak menampilkan password lagi (state hanya di komponen, tidak ada di storage/URL).
2. Admin mengisi password sendiri (valid): dialog sukses tidak menampilkan password sementara.
3. Developer baru login dengan password sementara, otomatis diarahkan ke `/change-password`; setelah mengganti password, bisa masuk beranda.
4. Admin membuat user dengan role `super_admin`; user itu setelah ganti password melihat menu Users.
5. Email dengan huruf besar dan spasi (`" Sari@Tim.ID "`) tersimpan sebagai `sari@tim.id`.

Negatif:
1. Email sudah terdaftar (beda huruf besar/kecil): 409; form menampilkan "email sudah terdaftar" pada field email; dialog tetap terbuka.
2. Email tidak valid, nama kosong, role kosong: 400; pesan per field tampil sekaligus.
3. Password 9 karakter atau sama dengan email: 400 dengan pesan pada field password.
4. Developer membuka `/admin/users` langsung -> NotFound di UI; `fetch POST /api/users` manual -> 403 `FORBIDDEN`.
5. Tanpa login (fetch manual) -> 401; tanpa header `X-Requested-With` -> 403 `CSRF_REQUIRED`.
6. User dengan flag ganti password (fetch manual) -> 403 `PASSWORD_CHANGE_REQUIRED`.
7. Klien mengirim `is_active:false` atau `must_change_password:false` di body -> 400 `field tidak dikenal` (`DecodeJSON` menolak field tak dikenal); tidak ada user dibuat.
8. Sesi admin kedaluwarsa saat dialog terbuka -> 401, UI redirect ke `/login`, password sementara tidak tertinggal.
9. Setelah pembuatan, halaman audit log (T02, bila sudah ada) menampilkan satu entri `user.created` tanpa password.

## Request/Response body

Semua error memakai format seragam T00: `{ "error": { "code", "message", "details" }, "trace_id" }`. Request mutasi wajib header `X-Requested-With: grimoire`, `Content-Type: application/json`, dan cookie `grimoire_session`.

### Request, password dikosongkan (dibangkitkan server)

```
POST /api/users
```
```json
{
  "email": " Sari@Tim.ID ",
  "name": "Sari Wulandari",
  "role": "developer"
}
```

### Request, password diberikan admin

```json
{
  "email": "budi@tim.id",
  "name": "Budi",
  "role": "super_admin",
  "password": "awal-yang-panjang-1"
}
```

### 201 Created (password dibangkitkan), header `Cache-Control: no-store`

```json
{
  "user": {
    "id": "7d1e5b2a-3c4f-4a6d-8e91-0f2b3c4d5e6f",
    "email": "sari@tim.id",
    "name": "Sari Wulandari",
    "role": "developer",
    "is_active": true,
    "must_change_password": true,
    "created_at": "2026-10-21T09:00:00Z",
    "created_by": "0b7c6a4e-2f1d-4c8a-9e3b-5d6f7a8b9c0d",
    "created_by_label": "admin@tim.id",
    "updated_at": "2026-10-21T09:00:00Z",
    "last_login_at": null
  },
  "temporary_password": "Kq7mZp3RtX9aHw2N"
}
```

### 201 Created (password diberikan admin)

```json
{
  "user": {
    "id": "9a8b7c6d-5e4f-4a3b-8c2d-1e0f9a8b7c6d",
    "email": "budi@tim.id",
    "name": "Budi",
    "role": "super_admin",
    "is_active": true,
    "must_change_password": true,
    "created_at": "2026-10-21T09:02:00Z",
    "created_by": "0b7c6a4e-2f1d-4c8a-9e3b-5d6f7a8b9c0d",
    "created_by_label": "admin@tim.id",
    "updated_at": "2026-10-21T09:02:00Z",
    "last_login_at": null
  },
  "temporary_password": null
}
```

### 400 Bad Request, validasi (semua field dikumpulkan)

```json
{
  "error": {
    "code": "INVALID_ARGUMENT",
    "message": "permintaan tidak valid",
    "details": {
      "fields": {
        "email": "format email tidak valid",
        "name": "wajib diisi",
        "role": "role harus super_admin atau developer",
        "password": "minimal 10 karakter"
      }
    }
  },
  "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33"
}
```

Pesan lain untuk `password`: `"password tidak boleh sama dengan email"`, `"maksimal 256 byte"`.

### 400 Bad Request, JSON rusak

```json
{
  "error": { "code": "INVALID_ARGUMENT", "message": "JSON tidak valid", "details": null },
  "trace_id": "..."
}
```

Varian lain dari `DecodeJSON` T00 (semua 400 `INVALID_ARGUMENT`): `"body wajib diisi"` (body kosong), `"tipe field tidak sesuai"` dengan `details: {"field": "email"}`, dan `"field tidak dikenal"` dengan `details: {"field": "is_active"}`. Body lebih besar dari batas: 413 `PAYLOAD_TOO_LARGE`.

### 401 Unauthorized (tanpa sesi)

```json
{
  "error": { "code": "UNAUTHORIZED", "message": "perlu login", "details": null },
  "trace_id": "..."
}
```

### 403 Forbidden (developer)

```json
{
  "error": { "code": "FORBIDDEN", "message": "akses ditolak", "details": null },
  "trace_id": "..."
}
```

### 403 Forbidden (admin wajib ganti password)

```json
{
  "error": { "code": "PASSWORD_CHANGE_REQUIRED", "message": "password wajib diganti sebelum melanjutkan", "details": null },
  "trace_id": "..."
}
```

### 403 Forbidden (tanpa header CSRF)

```json
{
  "error": { "code": "CSRF_REQUIRED", "message": "header X-Requested-With wajib untuk request mutasi", "details": null },
  "trace_id": "..."
}
```

### 409 Conflict (email sudah terdaftar)

```json
{
  "error": { "code": "CONFLICT", "message": "email sudah terdaftar", "details": null },
  "trace_id": "..."
}
```

### 500 Internal (kegagalan DB/hasher; tanpa detail internal)

```json
{
  "error": { "code": "INTERNAL", "message": "terjadi kesalahan internal", "details": null },
  "trace_id": "..."
}
```

### Contoh `record.Request` yang dikirim ke `core.audit.record` (sukses)

```go
record.Request{
    ActorType: "user", ActorID: "0b7c6a4e-2f1d-4c8a-9e3b-5d6f7a8b9c0d", ActorLabel: "admin@tim.id",
    Action: "user.created", TargetType: "user", Target: "7d1e5b2a-3c4f-4a6d-8e91-0f2b3c4d5e6f",
    Args: map[string]any{"email": "sari@tim.id", "name": "Sari Wulandari", "role": "developer", "generated_password": true},
    Status: "ok", IP: "10.0.0.5", UserAgent: "Mozilla/5.0 ...", TraceID: "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33",
}
```

Tidak ada kunci `password`, `temporary_password`, atau hash di `Args`/`Result`.
