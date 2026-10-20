# T01 core/auth — Usecase `getme` (GET /api/auth/me)

Melengkapi `docs/superpowers/plans/core-auth/overview.md` (F1 foundation: `base.MeDTO`, `PasswordGateExempt`, `PasswordChangeGate`, `core.Register`/`auth.Register`, `auth.yml`). Bila overview bertentangan dengan dokumen ini, overview menang. Hanya satu usecase: `getme`.

## Metadata

| Field | Nilai |
|---|---|
| Ticket | T01 |
| Scope | fullstack (backend Go + frontend Svelte) |
| Domain | core |
| Module | auth |
| Feature | `getme` (wave B, U6) |
| Endpoint | `/api/auth/me` |
| Method | GET |
| Version | tanpa prefix versi di URL (`auth.yml` `info.version: 0.1.0`) |
| Auth | `RequireAuth` (semua role). Exempt dari password gate (`"GET /api/auth/me"` sudah ada di `constants.PasswordGateExempt`) |
| Bus | tidak menyediakan, tidak mengonsumsi |
| Tabel / migrasi | tidak ada |
| Resource root | `backend/internal/domain/core/internal/auth` |
| allowedPaths | `backend/internal/domain/core/internal/auth/getme/`, `backend/internal/domain/core/internal/auth/base.go` (satu baris `getme.Init`), `frontend/src/domain/core/auth/getme/` |

Prasyarat dari F1 (jangan dikerjakan di sini; bila belum ada, hentikan dan laporkan): `base.MeDTO` di `base/user.go`, entri `PasswordGateExempt`, `auth.Register` + `buildShared`, path `/auth/me` dan schema `Me` di `api/modules/core/auth.yml`, dan `login` (U4) agar cookie bisa didapat di test e2e. Test integrasi backend tidak butuh `login`: identitas disuntik lewat context.

## Summary Plan

`getme` mengembalikan profil sesi saat ini (`id`, `name`, `email`, `role`, `must_change_password`) dari `reqctx.Identity` yang sudah di-resolve middleware `Session` per request lewat bus `core.auth.resolve-session`. Tidak ada query tambahan, tidak ada repository, tidak ada audit (aksi baca, tidak ada di katalog audit). Karena role, status aktif, dan flag dibaca ulang di setiap request oleh `resolvesession`, data yang dikembalikan selalu segar (perubahan role terlihat pada request berikutnya).

Frontend memakai endpoint ini saat aplikasi mulai (bootstrap) untuk mengisi `session store`: 200 menjadi `setIdentity(...)`, 401 menjadi `setIdentity(null)` (anonymous). Bila `must_change_password=true`, FE mengalihkan ke `/change-password`. Itu hanya UX; penegakan sebenarnya di backend (`PasswordChangeGate`).

Usecase tidak dipecah: satu usecase `Execute`, pemetaan identitas ke DTO. Rule bisnis pemetaan ada di entity (`base.MeFromIdentity`), usecase hanya orkestrasi.

Keputusan:
- Tidak ada repository dan tidak ada transaksi (sesuai overview 3.3).
- Package `base` tidak boleh meng-import `reqctx`? Boleh: `lib/common/reqctx` adalah lib, bukan I/O. Tetapi overview menyatakan `base` tanpa context; maka `MeFromIdentity` menerima `reqctx.Identity` (nilai), bukan `context.Context`. Usecase yang memanggil `reqctx.IdentityFrom(ctx)`.
- Respons tanpa pembungkus (`{id,...}` langsung, bukan `{user:{...}}`) mengikuti overview bagian 12. Beda dengan `login` yang membungkus `{user: Me}`; FE memetakan keduanya ke tipe `Me` yang sama.
- Header `Cache-Control: no-store` dipasang agar profil (dan flag) tidak di-cache browser/proxy. Ini tambahan kecil; tidak mengubah kontrak.
- Identity tidak ada di context (seharusnya mustahil setelah `RequireAuth`): usecase mengembalikan `errs.New(errs.CodeUnauthorized, "perlu login")` sebagai pertahanan berlapis.

## Plan

Urutan wajib. Backend B1 sampai B6, Frontend F1 sampai F7. Frontend bisa paralel setelah B1 (kontrak JSON tetap).

### Backend

**B0. Verifikasi prasyarat (tanpa edit).** Pastikan `base.MeDTO` ada dan tag JSON `id,email,name,role,must_change_password`; bila field/tag berbeda, sesuaikan test, bukan DTO F1. Tambahkan `MeFromIdentity` di `base/user.go` HANYA bila belum ada (file F1 milik foundation; ini satu-satunya sentuhan di luar `getme/` dan harus kecil):

```go
// base/user.go
func MeFromIdentity(id reqctx.Identity) MeDTO {
    return MeDTO{ID: id.UserID, Email: id.Email, Name: id.Name, Role: id.Role, MustChangePassword: id.MustChangePassword}
}
```

Bila F1 memilih konstruksi lain (mis. DTO dibangun di usecase), pakai itu dan lewati B0; yang penting pemetaan tidak ada di handler.

**B1. `getme/usecase.go`** (orkestrasi saja, rule 5.2 dan 5.6)

```go
package getme

type Usecase struct{}

func NewUsecase() *Usecase { return &Usecase{} }

// Execute mengembalikan profil dari identitas sesi pada context.
// Tidak ada identitas -> errs.New(errs.CodeUnauthorized, "perlu login").
func (u *Usecase) Execute(ctx context.Context) (base.MeDTO, error) {
    id, ok := reqctx.IdentityFrom(ctx)
    if !ok {
        return base.MeDTO{}, errs.New(errs.CodeUnauthorized, "perlu login")
    }
    return base.MeFromIdentity(id), nil
}
```

Tidak ada interface dependensi karena tidak ada dependensi (rule 5.6 berlaku hanya untuk dependensi).

**B2. `getme/handler.go`**

```go
type Handler struct{ uc *Usecase }

func NewHandler(uc *Usecase) *Handler { return &Handler{uc: uc} }

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
    me, err := h.uc.Execute(r.Context())
    if err != nil {
        httpx.WriteError(w, r, err)
        return
    }
    w.Header().Set("Cache-Control", "no-store")
    httpx.WriteJSON(w, http.StatusOK, me)
}
```

Sesuaikan signature `WriteError` dengan kode T00 yang sebenarnya (baca `backend/internal/infra/http/respond/respond.go`; jangan menebak). `params.go` tidak dibuat: endpoint tanpa input (query dan body diabaikan).

**B3. `getme/init.go`**

```go
func Init(d app.Deps, _ base.Shared) {
    h := NewHandler(NewUsecase())
    d.API.With(httpx.RequireAuth).Get("/auth/me", h.Get)
}
```

Verifikasi saat implementasi bahwa `d.API` adalah sub-router yang di-mount di `/api` (di `router.go` T00: `api` di-`Mount("/api", ...)`); bila di-mount tanpa strip, path-nya `/api/auth/me`. Ikuti pola yang dipakai `logout`/`login` (sesama feature auth) agar konsisten; jangan berimprovisasi. Tidak ada `RequireRole` (semua role boleh) dan tidak ada rate limit khusus (limiter global cukup; endpoint murah).

**B4. Wiring.** Tambah satu baris `getme.Init(d, shared)` di `auth/base.go` dalam `Register`. Satu baris, tanpa mengubah baris lain (minim konflik merge).

**B5. OpenAPI.** Seharusnya sudah ada dari F1. Periksa `api/modules/core/auth.yml`: operasi `GET /auth/me`, `operationId: getMe`, security `sessionCookie`, respons 200 `$ref Me`, 401 `$ref Unauthorized`, 500 `$ref Internal`. Bila belum ada, tambahkan dan jalankan `make api-bundle` + `make api-check`. Jangan menambahkan `X-Requested-With` (GET tidak memerlukan CSRF header).

**B6. Test** (lihat bagian Test Scenario). Tulis test dulu (TDD) lalu implementasi.

### Frontend (`frontend/src/domain/core/auth/getme/`)

Layering rule 12.2: services (fetch + Zod), stores (state), tanpa komponen presentasi besar. Public API hanya lewat `index.ts` (rule 12.3). `shared/` tidak meng-import `domain/` (rule 12.5): `session` store tetap di `shared/stores/session.ts`, yang memanggil `getme` adalah `app/` (komposisi).

**F1. `services/getme.service.ts`**

```ts
import { z } from 'zod';
import { get } from '../../../../../shared/services/http';
import type { Identity } from '../../../../../shared/stores/session';

const MeSchema = z.object({
  id: z.string().uuid(),
  email: z.string(),
  name: z.string(),
  role: z.enum(['super_admin', 'developer']),
  must_change_password: z.boolean(),
});

export type Me = z.infer<typeof MeSchema>;

export function toIdentity(m: Me): Identity {
  return { userId: m.id, email: m.email, name: m.name, role: m.role, mustChangePassword: m.must_change_password };
}

export async function fetchMe(): Promise<Identity> {
  const raw = await get('/api/auth/me', { schema: MeSchema });
  return toIdentity(raw);
}
```

Sesuaikan opsi validasi dengan `RequestOptions<T>` di `shared/services/http.ts` (baca file; bila validasi dilakukan manual `MeSchema.parse(await get(...))`, ikuti itu). Path import relatif dihitung dari lokasi file nyata.

**F2. `stores/` tidak diperlukan.** State sesi sudah di `shared/stores/session.ts` (`setIdentity`, `resetSession`). Jangan membuat store baru yang menduplikasi.

**F3. `bootstrap.ts`** (fungsi orkestrasi tipis di dalam feature, bukan komponen):

```ts
// Memuat sesi saat aplikasi start. 401 -> anonymous (bukan error). Error lain dilempar
// agar app menampilkan state gagal-muat dan tidak salah mengira user anonim.
export async function loadSession(deps = { fetchMe, setIdentity }): Promise<void> {
  try {
    deps.setIdentity(await deps.fetchMe());
  } catch (e) {
    if (isApiError(e) && e.status === 401) { deps.setIdentity(null); return; }
    throw e;
  }
}
```

Catatan: `http.ts` memiliki `setUnauthorizedHandler` yang bisa mengalihkan ke `/login` pada 401. Untuk `/api/auth/me` 401 adalah hasil normal saat bootstrap; periksa `request` apakah handler dipanggil untuk 401 dan bila ya beri opsi (mis. `skipUnauthorizedHandler`) hanya bila memang sudah ada. Bila tidak ada opsi, JANGAN mengubah `shared/services/http.ts` di usecase ini; catat sebagai temuan di PR dan gunakan guard route (`resolve()` sudah meredirect anonim ke `/login?next=`), sehingga redirect ganda tidak terjadi karena handler dipasang setelah `loadSession` selesai.

**F4. `redirect.ts`**: fungsi murni `postLoadRedirect(identity: Identity | null, currentPath: string): string | null`.
- `identity?.mustChangePassword === true` dan `currentPath !== '/change-password'` menghasilkan `'/change-password'`.
- Selain itu `null` (tanpa redirect).
Bukan keamanan; hanya UX (rule 12.7).

**F5. `index.ts`**: ekspor `fetchMe`, `loadSession`, `postLoadRedirect`, tipe `Me`.

**F6. Komposisi di `app/` (di luar allowedPaths feature, minimal):** `App.svelte` (atau `main.ts`) memanggil `loadSession()` sekali saat mount, lalu `postLoadRedirect` ke router. Sebelum selesai, `session.status === 'unknown'` sehingga guard merender `PendingView` (sudah ada). Bila `loadSession` melempar error non-401, tampilkan view gagal-muat dengan tombol coba lagi. Bagian ini boleh ditunda ke wiring akhir (W/frontend-plan) bila `login` belum ada; catat di PR.

**F7. i18n.** Tambah kunci `auth.session.loadFailed` (id/en) di sumber i18n modul auth bila view gagal-muat dibuat. Tidak ada string hard-coded (rule 12.8).

## Migration

Tidak ada. `getme` tidak menyentuh tabel; seluruh data dari `reqctx.Identity`. Migrasi `20261021100000000_create_auth_tables.sql` milik F1.

## Changes

Backend (baru):
- `backend/internal/domain/core/internal/auth/getme/init.go`
- `backend/internal/domain/core/internal/auth/getme/handler.go`
- `backend/internal/domain/core/internal/auth/getme/usecase.go`
- `backend/internal/domain/core/internal/auth/getme/usecase_test.go`
- `backend/internal/domain/core/internal/auth/getme/handler_test.go`
- `backend/internal/domain/core/internal/auth/getme/integration_test.go`

Backend (ubah, minimal):
- `backend/internal/domain/core/internal/auth/base.go`: satu baris `getme.Init(d, shared)`.
- `backend/internal/domain/core/internal/auth/base/user.go` + `user_test.go`: `MeFromIdentity`, hanya bila belum ada dari F1.

Tidak dibuat: `params.go`, `repository.go` (tidak ada input dan tidak ada SQL).

API:
- `api/modules/core/auth.yml`: verifikasi/lengkapi operasi `getMe` (bila F1 belum); regenerasi `api/openapi.yml` lewat `make api-bundle`.

Frontend (baru):
- `frontend/src/domain/core/auth/getme/services/getme.service.ts` (+ `.test.ts`)
- `frontend/src/domain/core/auth/getme/bootstrap.ts` (+ `.test.ts`)
- `frontend/src/domain/core/auth/getme/redirect.ts` (+ `.test.ts`)
- `frontend/src/domain/core/auth/getme/index.ts`

Frontend (ubah, tipis, bisa ditunda ke wiring): `frontend/src/app/` (`App.svelte`/`main.ts`) memanggil `loadSession`; kunci i18n bila perlu.

Dokumen: tidak ada perubahan (`PasswordGateExempt`, README, dan tiket diubah di F1).

## Test Scenario

Penamaan AC mengikuti rule 13.4. `getme` adalah bagian dari AC4 (gate must_change_password); AC4 end-to-end dimiliki `changepassword`, `getme` menambah barisnya.

### Unit (tanpa mock, rule 13.1)
- `TestMeFromIdentity`: semua field terpetakan; `MustChangePassword` true/false; JSON hasil marshal memiliki tepat kunci `id,email,name,role,must_change_password` (tanpa `password_hash`, `is_active`, dll).
- `TestUsecase_Execute_ReturnsIdentity`: ctx via `reqctx.WithIdentity` mengembalikan DTO sesuai.
- `TestUsecase_Execute_NoIdentity`: ctx kosong mengembalikan `*errs.Error` dengan `Code == CodeUnauthorized`.
- `TestUsecase_Execute_MustChangePasswordTrue`: flag diteruskan apa adanya.

### Handler (httptest, `d.API` chi nyata)
- `TestHandler_Get_200`: body JSON persis contoh di bawah; `Content-Type: application/json`; header `Cache-Control: no-store`.
- `TestHandler_Get_IgnoresQueryAndBody`: `GET /auth/me?x=1` dengan body acak tetap 200.

### Integrasi (router lengkap `httpx.NewRouter` + `getme.Init`, bus nyata dengan handler `resolve-session` palsu; tanpa DB)
- `TestT01_AC4_GetMe_ExemptFromPasswordGate`: resolve-session palsu mengembalikan `MustChangePassword=true`; `GET /api/auth/me` dengan cookie `grimoire_session=tok` menjawab 200 dan `must_change_password:true`; sebagai kontrol, rute non-exempt buatan test (`GET /api/_probe`) di router yang sama menjawab 403 `PASSWORD_CHANGE_REQUIRED`.
- `TestGetMe_Unauthenticated_401`: tanpa cookie, 401 `UNAUTHORIZED`, body error seragam dengan `trace_id`.
- `TestGetMe_InvalidSession_401`: resolve-session palsu `Found=false`, 401.
- `TestGetMe_MethodNotAllowed`: `POST /api/auth/me` mengembalikan 405 (`METHOD_NOT_ALLOWED`) lewat penanganan router T00. CSRF header dikirim agar bukan 403.
- `TestGetMe_NoExtraQueries`: bus palsu menghitung panggilan; satu request `me` menghasilkan tepat satu panggilan `resolve-session` dan getme sendiri tidak memanggil bus apa pun. (Tidak ada DB di test ini, jadi tidak mungkin ada query.)
- `TestGetMe_ReflectsRoleChangeNextRequest`: resolve-session palsu mengembalikan `developer` lalu `super_admin`; dua request berurutan menunjukkan role terbaru.

### Tambahan lintas usecase (di `auth_integration_test.go`, W)
- Baris `getme` pada tabel AC4: user baru dari `createuser` login, `GET /api/auth/me` 200 dengan `must_change_password:true`, setelah `change-password` 200 dengan `false`.
- Baris AC5: setelah user dinonaktifkan, `GET /api/auth/me` dengan cookie lama menghasilkan 401 dan `Set-Cookie` dengan `Max-Age=0` (perilaku middleware `Session`, diverifikasi di sini).
- `TestRoutesRegistered` memuat `GET /api/auth/me`.
- Baris AC8/AC9: `me` tidak menghasilkan entri audit (0 `record.Request`) dan tidak membocorkan hash/token ke log.

### Frontend (vitest)
- `getme.service.test.ts`: respons valid dipetakan ke `Identity` camelCase; role di luar enum atau `id` bukan uuid ditolak Zod; 401 menjadi `ApiError` status 401.
- `bootstrap.test.ts`: sukses memanggil `setIdentity(identity)`; 401 memanggil `setIdentity(null)` tanpa melempar; 500/network melempar dan tidak mengubah store.
- `redirect.test.ts`: tabel (mustChange true di `/` menjadi `/change-password`; true di `/change-password` menjadi `null`; false menjadi `null`; identity null menjadi `null`).
- Test guard yang ada (`guard.test.ts`) tidak berubah.

## E2E Scenarios (Playwright terhadap binary)

Positif:
1. Admin hasil `create-admin` login, reload halaman: UI tetap masuk (sesi dipulihkan lewat `GET /api/auth/me`), nama dan menu Users tampil.
2. Developer login: `GET /api/auth/me` mengembalikan `role: developer`; menu Users tidak tampil.
3. Developer baru (password sementara) membuka `/` setelah login: otomatis diarahkan ke `/change-password`; di halaman itu `GET /api/auth/me` tetap 200 (tidak diblok gate).
4. Setelah ganti password, reload: tidak lagi dialihkan; `must_change_password:false`.
5. Admin mengubah role developer menjadi `super_admin`; developer reload tanpa login ulang: menu Users muncul (role terbaru dari `me`).

Negatif:
1. Buka aplikasi tanpa cookie: `GET /api/auth/me` 401, UI menuju `/login?next=...` tanpa pesan error.
2. Cookie palsu/kedaluwarsa: 401, cookie dibersihkan, UI ke `/login`.
3. User dinonaktifkan saat tab terbuka, lalu reload: `me` 401, UI ke `/login`.
4. `curl -X POST /api/auth/me` (dengan `X-Requested-With`): 405.
5. Server mengembalikan 500 pada `me` (mis. DB mati saat resolve-session): UI menampilkan state gagal-muat dengan tombol coba lagi, BUKAN halaman login (user tidak dianggap anonim).
6. User ber-flag memanggil `GET /api/users` manual: 403 `PASSWORD_CHANGE_REQUIRED`, sementara `GET /api/auth/me` pada sesi yang sama 200.

## Request/Response body

Semua error memakai format seragam T00: `{ "error": { "code", "message", "details" }, "trace_id" }`. `GET` tidak memerlukan header `X-Requested-With`.

### GET /api/auth/me

Request: tanpa body, tanpa query. Cookie `grimoire_session=<token>`.

200 (header `Cache-Control: no-store`):
```json
{
  "id": "0b7c6a4e-2f1d-4c8a-9e3b-5d6f7a8b9c0d",
  "email": "budi@tim.id",
  "name": "Budi",
  "role": "developer",
  "must_change_password": false
}
```

200 untuk user yang wajib ganti password (tetap 200, bukan 403):
```json
{
  "id": "7d1e3b52-90aa-4f1c-8b2d-1c3e5a7b9d02",
  "email": "sari@tim.id",
  "name": "Sari",
  "role": "developer",
  "must_change_password": true
}
```

401 (tanpa cookie, token tidak dikenal, kedaluwarsa, idle, atau user nonaktif; `Set-Cookie: grimoire_session=; Path=/; Max-Age=0` bila cookie dikirim tetapi tidak valid):
```json
{
  "error": { "code": "UNAUTHORIZED", "message": "perlu login", "details": null },
  "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33"
}
```

405 (method selain GET):
```json
{
  "error": { "code": "METHOD_NOT_ALLOWED", "message": "method tidak diizinkan", "details": null },
  "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a34"
}
```
(Teks pesan mengikuti implementasi T00; test memeriksa kode dan status, bukan pesan.)

500 (kegagalan infra pada resolve-session; pesan generik, tanpa detail internal):
```json
{
  "error": { "code": "INTERNAL", "message": "terjadi kesalahan internal", "details": null },
  "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a35"
}
```

Schema OpenAPI (`Me`): objek dengan properti wajib `id` (string uuid), `email` (string), `name` (string), `role` (enum `super_admin|developer`), `must_change_password` (boolean).

## Definisi selesai

- Semua test di atas hijau; `go vet`, archtest `TestT00_AC4_RepoIsClean`, `make api-check`, dan lint FE lulus.
- Folder `getme/` tidak meng-import feature lain (rule 4.3); hanya `base`, `lib/common/{errs,reqctx}`, `infra/app`, dan `httpx`.
- Tidak ada perubahan pada file di luar allowedPaths selain dua sentuhan kecil yang disebut di Changes.
