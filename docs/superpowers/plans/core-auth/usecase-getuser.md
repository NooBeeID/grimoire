# T01 core/auth — Usecase `getuser` (GET /api/users/{id})

Melengkapi `docs/superpowers/plans/core-auth/overview.md` (F1: `base.User`, `base.UserDTO`, `base.ErrUserNotFound`, `base/store`, `auth.Register`/`buildShared`, `auth.yml`). Bila overview bertentangan dengan dokumen ini, overview menang. Hanya satu usecase: `getuser`. Jangan tertukar dengan `getuserbus` (handler bus `core.auth.get-user`, U2) yang folder dan kontraknya terpisah.

## Metadata

| Field | Nilai |
|---|---|
| Ticket | T01 |
| Scope | fullstack (backend Go + service FE kecil) |
| Domain | core |
| Module | auth |
| Feature | `getuser` (gelombang C, U10) |
| Endpoint | `/api/users/{id}` |
| Method | GET |
| Version | tanpa prefix versi di URL (`auth.yml` `info.version: 0.1.0`) |
| Auth | `RequireAuth` + `RequireRole(super_admin)`. Password gate berlaku (tidak exempt) |
| Bus | tidak menyediakan, tidak mengonsumsi |
| Audit | tidak ada (aksi baca, tidak ada di katalog audit) |
| Tabel / migrasi | membaca `users`; tidak ada migrasi |
| Resource root | `backend/internal/domain/core/internal/auth` |
| allowedPaths | `backend/internal/domain/core/internal/auth/getuser/`, `backend/internal/domain/core/internal/auth/base.go` (satu baris `getuser.Init`), `frontend/src/domain/core/auth/users/` (hanya service detail) |

Prasyarat dari F1 (jangan dikerjakan di sini; bila belum ada, hentikan dan laporkan): `base.User` + `ToResponse()` + `UserDTO`, `base.ErrUserNotFound`, `store.Store.FindUserByID` (id bukan uuid mengembalikan `(zero,false,nil)` tanpa query), `PasswordChangeGate`, `auth.Register` + `buildShared`, path `/users/{id}` GET dan schema `User` di `api/modules/core/auth.yml`, migrasi tabel `users`.

## Summary Plan

`getuser` mengembalikan detail satu user untuk Super Admin. Alur: ambil `{id}` dari path, cari lewat repository, bila ditemukan petakan dengan `User.ToResponse()` (tanpa `password_hash`) lalu jawab 200 dengan objek `UserDTO` langsung (tanpa pembungkus, sesuai overview bagian 12). Id bukan uuid atau tidak ada di DB: 404 `NOT_FOUND` `"user tidak ditemukan"`, keduanya tidak dapat dibedakan klien.

Keputusan:
- Tidak ada pengecekan role di usecase. Route sudah dijaga `RequireRole(super_admin)` (overview 15). Usecase hanya orkestrasi: cari lalu petakan.
- Validasi uuid dilakukan `store.FindUserByID` (overview 3.12 dan F1 8). Handler tidak mem-parse uuid, tidak ada `params.go` karena input hanya path param tunggal; handler membaca `chi.URLParam(r, "id")` dan meneruskan string mentah. Ini sengaja: tidak ada dua tempat yang memutuskan "uuid valid".
- Repository feature hanya membungkus `*store.Store` dan hanya mengekspos `FindUserByID`. Tidak ada transaksi, tidak ada `FOR UPDATE`.
- Mapping ke DTO ada di entity (`User.ToResponse`), bukan di handler.
- User nonaktif tetap dikembalikan (`is_active:false`); admin perlu melihatnya.
- Header `Cache-Control: no-store` dipasang: respons memuat data personal admin-only. Kecil dan tidak mengubah kontrak.
- Tidak ada rate limit khusus (limiter global cukup; endpoint admin murah).
- Error infra dibungkus `fmt.Errorf("getuser: find: %w", err)` dan dijawab 500 generik oleh `httpx.WriteError`.

## Plan

Urutan wajib. Tulis test dulu (TDD), lalu implementasi.

### Backend

**B0. Verifikasi prasyarat (tanpa edit).** Pastikan simbol F1 di atas ada dan baca signature `httpx.WriteError`/`WriteJSON`/`RequireRole` dari kode T00 yang sebenarnya (jangan menebak; contoh di bawah mengikuti pola feature lain). Baca `login`/`logout`/`getme` yang sudah ada untuk pola `Init` dan pendaftaran route; ikuti persis.

**B1. `getuser/repository.go`** (satu-satunya yang menyentuh SQL, lewat `store`)

```go
package getuser

// userFinder dipakai usecase; diimplementasi Repository.
type userFinder interface {
    FindUserByID(ctx context.Context, id string) (base.User, bool, error)
}

type Repository struct{ s *store.Store }

func NewRepository(q db.Querier) *Repository { return &Repository{s: store.New(q)} }

func (r *Repository) FindUserByID(ctx context.Context, id string) (base.User, bool, error) {
    return r.s.FindUserByID(ctx, id)
}
```

Tidak ada SQL baru di feature ini; SQL ada di `base/store` (F1, dipakai juga `updateuser`/`setuserstatus`/`createuser`).

**B2. `getuser/usecase.go`** (orkestrasi saja, rule 5.2 dan 5.6)

```go
type Usecase struct{ repo userFinder }

func NewUsecase(repo userFinder) *Usecase { return &Usecase{repo: repo} }

// Execute: id tidak ada / bukan uuid -> base.ErrUserNotFound (404).
func (u *Usecase) Execute(ctx context.Context, id string) (base.UserDTO, error) {
    user, found, err := u.repo.FindUserByID(ctx, id)
    if err != nil {
        return base.UserDTO{}, fmt.Errorf("getuser: find: %w", err)
    }
    if !found {
        return base.UserDTO{}, base.ErrUserNotFound
    }
    return user.ToResponse(), nil
}
```

**B3. `getuser/handler.go`**

```go
type Handler struct{ uc *Usecase }

func NewHandler(uc *Usecase) *Handler { return &Handler{uc: uc} }

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
    dto, err := h.uc.Execute(r.Context(), chi.URLParam(r, "id"))
    if err != nil {
        httpx.WriteError(w, r, err)
        return
    }
    w.Header().Set("Cache-Control", "no-store")
    httpx.WriteJSON(w, http.StatusOK, dto)
}
```

Sesuaikan signature `WriteError`/`WriteJSON` dengan T00. Query string dan body diabaikan.

**B4. `getuser/init.go`**

```go
func Init(d app.Deps, _ base.Shared) {
    h := NewHandler(NewUsecase(NewRepository(d.DB)))
    d.API.With(httpx.RequireAuth, httpx.RequireRole(resolvesession.RoleSuperAdmin)).
        Get("/users/{id}", h.Get)
}
```

Verifikasi mount: path akhir harus `/api/users/{id}`. `/users/{id}` juga didaftarkan `updateuser` (PATCH) dan `/users` oleh `createuser`/`listusers`; chi mengizinkan beberapa method di pola yang sama lewat pemanggilan terpisah. Daftarkan hanya GET di sini. Gunakan konstanta role dari kontrak `resolvesession` (bukan string literal). Bila `d.DB` bertipe lain, ikuti yang dipakai feature lain.

**B5. Wiring.** Tambah satu baris `getuser.Init(d, shared)` di `auth/base.go` dalam `Register`. Satu baris, tanpa mengubah baris lain.

**B6. OpenAPI.** Seharusnya sudah ada dari F1. Periksa `api/modules/core/auth.yml`: `GET /users/{id}`, `operationId: getUser`, security `sessionCookie`, parameter path `id` (string, format uuid), respons 200 `$ref User`, 401 `Unauthorized`, 403 `Forbidden`, 404 `NotFound`, 500 `Internal`. Bila belum ada, tambahkan lalu `make api-bundle` dan `make api-check`. Tanpa `X-Requested-With` (GET).

**B7. Test** (bagian Test Scenario).

### Frontend (minimal, service saja)

Layar Users (daftar, dialog, dsb.) dimiliki frontend-plan dan feature lain. Di sini hanya kontrak pembacaan detail, agar form edit bisa memuat satu user.

**F1. `frontend/src/domain/core/auth/users/services/getuser.service.ts`**

```ts
import { z } from 'zod';
import { get } from '../../../../../shared/services/http';

export const UserSchema = z.object({
  id: z.string().uuid(),
  email: z.string(),
  name: z.string(),
  role: z.enum(['super_admin', 'developer']),
  is_active: z.boolean(),
  must_change_password: z.boolean(),
  created_at: z.string(),
  created_by: z.string().uuid().nullable(),
  created_by_label: z.string().nullable(),
  updated_at: z.string(),
  last_login_at: z.string().nullable(),
});
export type UserDetail = z.infer<typeof UserSchema>;

export async function fetchUser(id: string): Promise<UserDetail> {
  return UserSchema.parse(await get(`/api/users/${encodeURIComponent(id)}`));
}
```

Sesuaikan dengan `shared/services/http.ts` (opsi `schema` bila ada). Bila `UserSchema` sudah didefinisikan oleh feature `users` lain (listusers/createuser) di folder yang sama, impor, jangan duplikasi; bila file bersama belum ada, buat `users/types.ts` kecil dan koordinasikan lewat PR. Ekspor `fetchUser` dari `users/index.ts` (tambah ekspor, jangan menimpa ekspor feature lain). Tidak ada store, komponen, atau route di usecase ini. Pesan 404 ditangani pemanggil lewat `ApiError.status === 404`.

## Migration

Tidak ada. `getuser` hanya membaca tabel `users` dari `backend/migrations/core/20261021090000000_create_auth_tables.sql` (milik F1).

## Changes

Backend (baru):
- `backend/internal/domain/core/internal/auth/getuser/init.go`
- `backend/internal/domain/core/internal/auth/getuser/handler.go`
- `backend/internal/domain/core/internal/auth/getuser/usecase.go`
- `backend/internal/domain/core/internal/auth/getuser/repository.go`
- `backend/internal/domain/core/internal/auth/getuser/usecase_test.go`
- `backend/internal/domain/core/internal/auth/getuser/handler_test.go`
- `backend/internal/domain/core/internal/auth/getuser/integration_test.go` (tag `integration`)

Backend (ubah, minimal):
- `backend/internal/domain/core/internal/auth/base.go`: satu baris `getuser.Init(d, shared)`.

Tidak dibuat: `params.go` (hanya path param, tanpa body/query), migrasi, kontrak bus, konstanta baru.

API:
- `api/modules/core/auth.yml`: verifikasi/lengkapi `getUser`; regenerasi `api/openapi.yml` bila ada perubahan.

Frontend (baru):
- `frontend/src/domain/core/auth/users/services/getuser.service.ts` (+ `.test.ts`)
- ekspor `fetchUser` di `frontend/src/domain/core/auth/users/index.ts`

Dokumen: tidak ada perubahan.

Folder `getuser/` hanya meng-import `base`, `base/store`, `lib/contracts/core/auth/resolvesession` (konstanta role), `infra/app`, `infra/db`, `httpx`, `chi`. Tidak meng-import feature lain (rule 4.3).

## Test Scenario

Penamaan AC mengikuti rule 13.4. `getuser` ikut menambah barisnya di AC7 (developer 403 di `/api/users*`) dan AC4 (gate), dimiliki `listusers`/`changepassword`.

### Unit usecase (repository fake in-memory lewat interface `userFinder`)
- `TestUsecase_Execute_Found`: user ada, hasil `UserDTO` sama dengan `user.ToResponse()`; semua field termasuk `LastLoginAt` nil dan non-nil.
- `TestUsecase_Execute_NotFound`: repo `(zero,false,nil)` mengembalikan `base.ErrUserNotFound` (`*errs.Error`, `CodeNotFound`).
- `TestUsecase_Execute_InactiveUserStillReturned`: `IsActive=false` tetap 200-equivalent (DTO `is_active:false`).
- `TestUsecase_Execute_RepoError`: error infra dibungkus (`errors.Is` ke error asal), bukan `*errs.Error` NOT_FOUND.
- `TestUsecase_Execute_DTOHasNoHash`: marshal JSON hasil tidak memuat `password_hash` maupun nilai hash dari user uji.

### Handler (httptest, chi nyata, usecase memakai repo fake)
- `TestHandler_Get_200`: body JSON persis contoh di bawah; `Content-Type: application/json`; `Cache-Control: no-store`.
- `TestHandler_Get_404_NotFound`: error `ErrUserNotFound` menjadi 404 `NOT_FOUND` dengan body error seragam dan `trace_id`.
- `TestHandler_Get_PassesRawIDToUsecase`: id `bukan-uuid` diteruskan apa adanya (tanpa 400).
- `TestHandler_Get_IgnoresQuery`: `?x=1` tetap 200.
- `TestHandler_Get_500_Generic`: error infra menjadi 500 dengan pesan generik tanpa teks error asal.

### Integrasi (Postgres nyata `db.TestDB(t)` + migrasi, router lengkap `httpx.NewRouter` + `getuser.Init`, bus nyata dengan handler `resolve-session` palsu)
- `TestGetUser_Integration_Found`: sisip user via `store.InsertUser`; admin (resolve-session palsu `super_admin`, `MustChangePassword=false`) memanggil `GET /api/users/{id}` menghasilkan 200 dan setiap kolom cocok dengan baris DB.
- `TestGetUser_Integration_UnknownUUID_404`: uuid valid tetapi tidak ada menghasilkan 404 `NOT_FOUND`.
- `TestGetUser_Integration_InvalidUUID_404`: `/api/users/abc`, `/api/users/123`, `/api/users/%27%3B--` menghasilkan 404 `NOT_FOUND` yang sama (bukan 400/500), tanpa error DB di log.
- `TestGetUser_Integration_InactiveUser_200`.
- `TestT01_AC7_GetUser_DeveloperForbidden`: resolve-session palsu `developer` menghasilkan 403 `FORBIDDEN`, body user tidak bocor.
- `TestGetUser_Unauthenticated_401`: tanpa cookie, 401 `UNAUTHORIZED`.
- `TestT01_AC4_GetUser_BlockedByPasswordGate`: admin dengan `MustChangePassword=true` menghasilkan 403 `PASSWORD_CHANGE_REQUIRED`.
- `TestGetUser_MethodNotAllowed`: `POST /api/users/{id}` tanpa handler lain terdaftar menghasilkan 405 (atau perilaku router T00); test memeriksa kode dan status.
- `TestGetUser_NoAuditAndNoBus`: bus palsu menghitung panggilan; satu request hanya memicu satu `resolve-session`, nol `core.audit.record`.
- `TestGetUser_ResponseExcludesPasswordHash`: body mentah tidak mengandung `$argon2id$` atau `password_hash`.

### Tambahan lintas usecase (di `auth_integration_test.go`, W)
- `TestRoutesRegistered` memuat `GET /api/users/{id}`.
- Baris AC7: developer memanggil `GET /api/users/{id}` mendapat 403.
- Baris AC8/AC9: `getuser` menghasilkan 0 `record.Request` dan tidak membocorkan hash ke log.

### Frontend (vitest)
- `getuser.service.test.ts`: respons valid lolos Zod; `role` di luar enum atau `id` bukan uuid ditolak; 404 menjadi `ApiError` status 404; `id` di-encode pada URL.

## E2E Scenarios (Playwright terhadap binary)

Positif:
1. Admin login lalu `fetch('/api/users/<id developer>')` dari konteks halaman: 200, `email`, `name`, `role`, `is_active`, `last_login_at` sesuai data yang dibuat sebelumnya lewat UI/API.
2. Admin membuka dialog edit user (memuat detail lewat endpoint ini): form terisi nama, email, dan role saat ini (bergantung pada UI dari feature `users` lain; jika belum ada, cukup verifikasi lewat request API).
3. User yang sudah dinonaktifkan: detail tetap 200 dengan `is_active:false`.
4. Setelah `updateuser` mengubah nama, `GET` berikutnya menunjukkan nama baru dan `updated_at` lebih baru.

Negatif:
1. Developer memanggil `GET /api/users/<id>`: 403 `FORBIDDEN`.
2. Tanpa cookie: 401 `UNAUTHORIZED`.
3. Admin memanggil dengan uuid acak yang tidak ada: 404 `NOT_FOUND`.
4. Admin memanggil `/api/users/bukan-uuid`: 404 `NOT_FOUND`, pesan sama dengan kasus tidak ada.
5. Admin ber-flag `must_change_password` memanggil endpoint ini: 403 `PASSWORD_CHANGE_REQUIRED`.
6. Admin dinonaktifkan saat tab terbuka, lalu memanggil endpoint: 401.
7. Respons tidak pernah memuat `password_hash` pada skenario mana pun.

## Request/Response body

Semua error memakai format seragam T00: `{ "error": { "code", "message", "details" }, "trace_id" }`. `GET` tidak memerlukan header `X-Requested-With`.

### GET /api/users/{id}

Request: tanpa body, tanpa query. Cookie `grimoire_session=<token>` milik super_admin. Contoh: `GET /api/users/7d1e3b52-90aa-4f1c-8b2d-1c3e5a7b9d02`.

200 (header `Cache-Control: no-store`; objek `UserDTO` tanpa pembungkus):
```json
{
  "id": "7d1e3b52-90aa-4f1c-8b2d-1c3e5a7b9d02",
  "email": "sari@tim.id",
  "name": "Sari",
  "role": "developer",
  "is_active": true,
  "must_change_password": false,
  "created_at": "2026-10-21T09:00:00Z",
  "created_by": "0b7c6a4e-2f1d-4c8a-9e3b-5d6f7a8b9c0d",
  "created_by_label": "admin@tim.id",
  "updated_at": "2026-10-21T09:05:00Z",
  "last_login_at": "2026-10-21T09:10:00Z"
}
```

200 untuk user nonaktif yang belum pernah login (nilai null tetap ada sebagai kunci):
```json
{
  "id": "9a2f4c6e-1b3d-4e5f-8a7b-0c1d2e3f4a5b",
  "email": "andi@tim.id",
  "name": "Andi",
  "role": "developer",
  "is_active": false,
  "must_change_password": true,
  "created_at": "2026-10-21T09:00:00Z",
  "created_by": null,
  "created_by_label": null,
  "updated_at": "2026-10-21T11:00:00Z",
  "last_login_at": null
}
```

404 (id tidak ada atau bukan uuid):
```json
{
  "error": { "code": "NOT_FOUND", "message": "user tidak ditemukan", "details": null },
  "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33"
}
```

401 (tanpa sesi valid; `Set-Cookie: grimoire_session=; Path=/; Max-Age=0` bila cookie dikirim tetapi tidak valid):
```json
{
  "error": { "code": "UNAUTHORIZED", "message": "perlu login", "details": null },
  "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a34"
}
```

403 (role developer):
```json
{
  "error": { "code": "FORBIDDEN", "message": "akses ditolak", "details": null },
  "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a35"
}
```

403 (admin wajib ganti password):
```json
{
  "error": { "code": "PASSWORD_CHANGE_REQUIRED", "message": "password wajib diganti sebelum melanjutkan", "details": null },
  "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a36"
}
```

500 (kegagalan infra; pesan generik, tanpa SQL/detail internal):
```json
{
  "error": { "code": "INTERNAL", "message": "terjadi kesalahan internal", "details": null },
  "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a37"
}
```

Schema OpenAPI (`User`): properti wajib `id` (string uuid), `email`, `name`, `role` (enum `super_admin|developer`), `is_active`, `must_change_password`, `created_at` (date-time), `created_by` (uuid, nullable), `created_by_label` (string, nullable), `updated_at` (date-time), `last_login_at` (date-time, nullable).

## Definisi selesai

- Semua test di atas hijau; `go vet`, archtest `TestT00_AC4_RepoIsClean`, `make api-check`, dan lint FE lulus.
- Respons tidak pernah memuat hash password; id non-uuid dan id tak dikenal menghasilkan 404 identik.
- Tidak ada perubahan di luar allowedPaths selain satu baris di `auth/base.go` dan ekspor di `users/index.ts`.
