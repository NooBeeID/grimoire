# T01 core/auth — Usecase Plan: `listusers`

Tiket: `docs/tickets/T01-core-auth.md`. Plan bersama (keputusan mengikat, `base/`, `base/store`, migrasi, OpenAPI, konfigurasi): `docs/superpowers/plans/core-auth/overview.md`. Dokumen ini hanya memuat usecase `listusers` (U9, gelombang C). Bila bertentangan dengan overview, overview menang.
Konvensi: `docs/tickets/README.md` dan `rules.txt` (rules menang untuk arsitektur).

## Metadata

| Field | Nilai |
|---|---|
| Ticket | T01 |
| Scope | fullstack (backend Go + slice frontend Svelte untuk tabel daftar user) |
| Domain | core |
| Module | auth |
| Feature | `listusers` |
| Resource root | `backend/internal/domain/core/internal/auth` |
| Endpoint | `/api/users` |
| Method | GET |
| Version | tanpa prefix versi di URL; `api/modules/core/auth.yml` `info.version: 0.1.0` |
| Akses | `RequireAuth` + `RequireRole(super_admin)`; developer 403, tanpa sesi 401 |
| Password gate | dicek (bukan route exempt): user ber-flag `must_change_password` mendapat 403 `PASSWORD_CHANGE_REQUIRED` |
| Audit | tidak ada (baca saja, tidak ada di katalog event) |
| Bus | tidak menyediakan dan tidak mengonsumsi endpoint bus |
| AC | AC7 (developer ke `/api/users*` 403); bagian dari AC4 (gate) |

allowedPaths (hanya ini yang boleh disentuh):
- `backend/internal/domain/core/internal/auth/listusers/`
- `backend/internal/domain/core/internal/auth/base/userfilter.go` dan `userfilter_test.go` (file baru milik usecase ini)
- `backend/internal/domain/core/internal/auth/base.go` (tepat satu baris: `listusers.Init(d, shared)`)
- `backend/internal/domain/core/internal/auth/auth_integration_test.go` (tambah baris tabel, lihat Test Scenario)
- `api/modules/core/auth.yml`, `api/openapi.yml` (hanya bila hasil verifikasi menemukan ketidakcocokan, lihat Plan BE-0)
- `frontend/src/domain/core/auth/users/` (slice daftar, lihat Plan FE)
- `frontend/src/app/routes/index.ts`, `frontend/src/app/views/UsersView.svelte`, `frontend/src/app/layouts/AppLayout.svelte` (menu)
- `frontend/src/shared/i18n/{id,en}.json` (kunci `users.*`)

## Konteks yang dimuat

- Tidak ada `CLAUDE.md` dan `.claude/rules/`. Sumber: `rules.txt`, README tiket, `T01-core-auth.md`, overview T01.
- T00 sudah ada di `main` (commit `45a7ad5`). Nama yang terverifikasi di kode, dan menggantikan nama `httpx` di overview bila berbeda:
  - `grimoire/internal/infra/http/respond`: `WriteJSON(w, status, v)`, `WriteError(w, r, err)`, `StatusFor`.
  - `grimoire/internal/infra/middleware`: `RequireAuth()`, `RequireRole(roles ...string)`.
  - `grimoire/internal/infra/db`: `db.Querier` (`sqlx.ExtContext` + `GetContext` + `SelectContext`), `db.TestDB(t)`.
  - `grimoire/internal/infra/app`: `app.Deps{Ctx, Config, Log, DB, Bus, Jobs, API, MCP}`. `API` adalah sub-router `/api`.
  - `grimoire/lib/common/errs` (`CodeInvalid`, `WithDetails`, ...), `grimoire/lib/common/reqctx`.
  - `grimoire/lib/contracts/core/auth/resolvesession`: `RoleSuperAdmin`, `RoleDeveloper`.
  - FE: `shared/services/http.ts` (`get`, `ApiError`, `RequestOptions.query` bertipe `Record<string, string|number|boolean|null|undefined>`, nilai `null/undefined` perlu dicek apakah dilewati di `buildUrl`), komponen `Table`, `Pagination` (berbasis `page`, `pageSize`, `total`), `Input`, `Button`, `Spinner`, `EmptyState`, `ErrorState`, store `i18n` (`t`) dan `session`.
- Prasyarat dari F1 foundation (HARUS sudah ada; bila belum, berhenti dan laporkan, jangan membuatnya di folder feature):
  - `base/user.go`: `User`, `UserDTO`, `User.ToResponse()`, `ValidRole`, `FieldErrors`.
  - `base/like.go`: `LikePattern`.
  - `base/errors.go`: `InvalidFields(fe)`.
  - `base/deps.go`: `Shared`.
  - Migrasi `users` (kolom dan CHECK sesuai overview bagian 6).
  - `api/modules/core/auth.yml` sudah memuat path `GET /api/users` dan schema `UserList`.
  - Kontrak error seragam dan `PASSWORD_CHANGE_REQUIRED` (gate di `NewRouter`).
- Catatan overview: `base.User` tidak punya tag `db` dan repository feature tidak boleh bergantung pada row privat `base/store`. Karena itu repository `listusers` punya row struct privat sendiri.

## Summary Plan

`listusers` melayani `GET /api/users`: parse query string (`role`, `status`, `q`, `limit`, `offset`), validasi lewat entity filter (rule di `base/userfilter.go`), ambil satu halaman user dengan predikat dinamis tetapi selalu parameterized, urut `name ASC, id ASC`, lalu bentuk respons `{items, total, limit, offset}` memakai `User.ToResponse` (tanpa hash, termasuk `last_login_at`).

Pembagian tanggung jawab (satu usecase tetap satu folder, tanpa pemecahan buatan):

| Lapisan | Isi |
|---|---|
| `base/userfilter.go` (rule murni) | `UserFilter`, `NewUserFilter(...)`: default limit 20, batas 1..100, offset >= 0, enum role dan status, panjang `q`, normalisasi `q`. Dipakai hanya oleh usecase ini, tetapi berisi rule bisnis sehingga tidak boleh di usecase. |
| `listusers/params.go` | Bentuk query string saja: ambil nilai, `strconv.Atoi`. Tidak ada rule nilai. |
| `listusers/usecase.go` | Orkestrasi: bangun filter (kembalikan 400 bila gagal), panggil repository, bila halaman kosong dengan offset > 0 minta total terpisah, petakan ke DTO. |
| `listusers/repository.go` | Satu-satunya SQL. Membangun WHERE dari fragmen konstan, argumen posisional, `COUNT(*) OVER()`. |
| `listusers/handler.go` | Parse, panggil usecase, `respond.WriteJSON`. |

Keputusan di plan ini:

1. Tanpa migrasi dan tanpa indeks baru. ILIKE `%q%` tidak memakai btree dan jumlah user puluhan (overview D6). Indeks `users_role_active_idx` dari F1 sudah cukup untuk filter role dan status.
2. Kolom `password_hash` TIDAK pernah di-SELECT oleh query ini. `base.User.PasswordHash` tetap kosong. Ini lapisan pengaman kedua di atas `ToResponse`.
3. `total` memakai `COUNT(*) OVER()`. Masalah klasiknya: bila `offset` melewati data, tidak ada baris sehingga `total` tidak terbaca. Untuk kasus itu (baris kosong dan offset > 0) repository menjalankan `SELECT count(*)` dengan WHERE yang sama, agar respons tetap `{items: [], total: N}` yang benar.
4. Validasi dua tahap: kesalahan bentuk (`limit=abc`) dilaporkan oleh params, kesalahan nilai (`limit=500`, `status=gone`) oleh entity. Keduanya memakai format `details.fields`. Bila bentuk salah, validasi nilai tidak dijalankan, sehingga klien bisa menerima 400 dua kali berurutan untuk dua jenis kesalahan berbeda. Ini diterima.
5. Parameter kosong (`role=`, `status=`, `limit=`, `offset=`, `q=`) diperlakukan sama dengan tidak dikirim. `q` yang hanya berisi spasi juga dianggap tidak ada. Alasan: form FE mengirim kolom kosong apa adanya.
6. Parameter tak dikenal diabaikan.
7. Route dipasang per feature dengan `d.API.With(middleware.RequireAuth(), middleware.RequireRole(resolvesession.RoleSuperAdmin)).Get("/users", h.List)`. Tidak memakai `Route("/users", ...)` agar tidak bentrok dengan feature `createuser` (POST pada path yang sama) yang dipasang di PR lain.

## Plan

Urutan wajib. BE-n sebelum FE-n bukan keharusan kode, tetapi kontrak respons (bagian Request/Response) dikunci dulu agar BE dan FE bisa paralel. APP-n terakhir.

### Backend

#### BE-0. Verifikasi OpenAPI (sebelum kode, rule 11.6)

Buka `api/modules/core/auth.yml` dan pastikan operasi `GET /api/users` (`operationId: listUsers`) memuat persis:

| Parameter | Aturan |
|---|---|
| `role` | query, enum `super_admin`, `developer`, opsional |
| `status` | query, enum `active`, `inactive`, opsional |
| `q` | query, string, `maxLength: 100`, opsional |
| `limit` | query, integer, `minimum: 1`, `maximum: 100`, `default: 20` |
| `offset` | query, integer, `minimum: 0`, `default: 0` |

Respons: 200 `UserList {items: User[], total: integer, limit: integer, offset: integer}` (semua `required`), 400 `BadRequest`, 401 `Unauthorized`, 403 `Forbidden` (contoh `FORBIDDEN` dan `PASSWORD_CHANGE_REQUIRED`), 500 `Internal`. Security `sessionCookie`. Jika ada selisih, perbaiki di `auth.yml`, jalankan `make api-bundle`, pastikan `make api-check` lulus.

#### BE-1. `base/userfilter.go` (entity rule, tanpa I/O, tanpa context)

```go
const (
    DefaultUserLimit = 20
    MaxUserLimit     = 100
    MaxUserQueryLen  = 100 // rune
    StatusActive     = "active"
    StatusInactive   = "inactive"
)

// UserFilter adalah filter daftar user yang sudah tervalidasi dan dinormalisasi.
type UserFilter struct {
    Role     string  // "" = semua; selain itu super_admin | developer
    IsActive *bool   // nil = semua; true = active; false = inactive
    Query    string  // sudah di-trim; "" = tanpa pencarian
    Limit    int     // 1..100
    Offset   int     // >= 0
}

// UserFilterInput adalah nilai mentah dari transport. Limit/Offset nil = tidak dikirim.
type UserFilterInput struct {
    Role, Status, Query string
    Limit, Offset       *int
}

// NewUserFilter memvalidasi semua field sekaligus (FieldErrors gabungan), lalu menormalisasi.
func NewUserFilter(in UserFilterInput) (UserFilter, error)

// LikePattern mengembalikan pola ILIKE ter-escape (memakai base.LikePattern); "" bila Query kosong.
func (f UserFilter) LikePattern() string
```

Aturan dan pesan field error (kunci = nama parameter):
- `role`: `strings.TrimSpace`; kosong lolos; selain itu harus `ValidRole`, kalau tidak: `"role harus super_admin atau developer"`.
- `status`: kosong lolos; `active` menjadi `IsActive=&true`; `inactive` menjadi `&false`; lainnya: `"harus active atau inactive"`.
- `q`: `strings.TrimSpace`; lebih dari 100 rune: `"maksimal 100 karakter"`. Karakter kontrol (`unicode.IsControl`) ditolak dengan `"mengandung karakter tidak valid"`. Sebagai pengecualian tidak ada: tab dan newline juga ditolak.
- `limit`: nil menjadi 20; `< 1`: `"minimal 1"`; `> 100`: `"maksimal 100"`.
- `offset`: nil menjadi 0; `< 0`: `"minimal 0"`; `> 1_000_000`: `"terlalu besar"` (mencegah offset absurd, bukan batas bisnis).
- Error dikembalikan sebagai `FieldErrors`; usecase mengubahnya lewat `base.InvalidFields`.

#### BE-2. `listusers/params.go` (bentuk query saja)

```go
// parseQuery mengubah query string menjadi UserFilterInput. Hanya memeriksa BENTUK angka.
// Mengembalikan FieldErrors (kosong bila lolos). Tidak ada rule nilai di sini.
func parseQuery(q url.Values) (base.UserFilterInput, base.FieldErrors)
```

- `role`, `status`, `q`: `q.Get(k)` apa adanya (trim dilakukan entity).
- `limit`, `offset`: bila `q.Get(k) != ""`, `strconv.Atoi`; gagal menghasilkan `fe[k] = "harus berupa angka bulat"`; sukses menghasilkan pointer ke nilai.
- Pesan tidak memuat nilai input. Panjang string query diperiksa oleh `BodyLimit`/server T00, bukan di sini.

#### BE-3. `listusers/repository.go` (satu-satunya SQL)

```go
type Repository interface {
    // List mengembalikan satu halaman (kolom tanpa password_hash) urut name ASC, id ASC,
    // dan total baris yang cocok filter. Halaman kosong: total diambil lewat Count bila Offset > 0.
    List(ctx context.Context, f base.UserFilter) (users []base.User, total int, err error)
}

type Repo struct{ q db.Querier }
func NewRepo(q db.Querier) *Repo
```

Row privat:

```go
type userRow struct {
    ID                 string     `db:"id"`
    Email              string     `db:"email"`
    Name               string     `db:"name"`
    Role               string     `db:"role"`
    IsActive           bool       `db:"is_active"`
    MustChangePassword bool       `db:"must_change_password"`
    CreatedAt          time.Time  `db:"created_at"`
    CreatedBy          *string    `db:"created_by"`
    CreatedByLabel     *string    `db:"created_by_label"`
    UpdatedAt          time.Time  `db:"updated_at"`
    LastLoginAt        *time.Time `db:"last_login_at"`
    Total              int        `db:"total"`
}
```

Query dibangun dari fragmen konstan; nilai pengguna HANYA lewat argumen posisional (`$n`), tidak pernah disambung ke string SQL:

```sql
SELECT id::text AS id, email::text AS email, name, role, is_active, must_change_password,
       created_at, created_by::text AS created_by, created_by_label, updated_at, last_login_at,
       COUNT(*) OVER() AS total
FROM users
WHERE 1=1
  [AND role = $n]
  [AND is_active = $n]
  [AND (email ILIKE $n ESCAPE '\' OR name ILIKE $n ESCAPE '\')]
ORDER BY name ASC, id ASC
LIMIT $n OFFSET $n
```

Catatan implementasi:
- Pola ILIKE dari `f.LikePattern()` (satu argumen dipakai dua kali lewat `$n` yang sama). Escape `\` ditulis di string Go raw (`` ` ``) agar tidak ter-escape dua kali.
- `total` = `rows[0].Total` bila ada baris. Bila tidak ada baris dan `Offset > 0`: jalankan `SELECT count(*) FROM users WHERE ...` dengan fragmen WHERE yang sama (fungsi privat `where(f) (clause string, args []any)` dipakai kedua query agar tidak menyimpang). Bila tidak ada baris dan `Offset == 0`: `total = 0`.
- Hasil dipetakan ke `[]base.User`; slice tidak pernah nil (`make([]base.User, 0, len(rows))`).
- `sql.ErrNoRows` tidak berlaku untuk `SelectContext`. Error lain dibungkus `fmt.Errorf("listusers: query users: %w", err)`.
- Tanpa transaksi (satu SELECT; READ COMMITTED cukup).

#### BE-4. `listusers/usecase.go` (orkestrasi)

```go
type Input = base.UserFilterInput

type Output struct {
    Items  []base.UserDTO `json:"items"`
    Total  int            `json:"total"`
    Limit  int            `json:"limit"`
    Offset int            `json:"offset"`
}

type Usecase struct{ repo Repository }
func NewUsecase(repo Repository) *Usecase

func (u *Usecase) Execute(ctx context.Context, in Input) (Output, error)
```

Langkah `Execute`:
1. `f, err := base.NewUserFilter(in)`; bila `err` adalah `base.FieldErrors`, kembalikan `base.InvalidFields(fe)` (400).
2. `users, total, err := u.repo.List(ctx, f)`; error dibungkus `fmt.Errorf("listusers: list: %w", err)` (500 generik; tidak ada SQL ke klien).
3. `items := make([]base.UserDTO, len(users))`; isi dengan `users[i].ToResponse()`.
4. Kembalikan `Output{Items: items, Total: total, Limit: f.Limit, Offset: f.Offset}` (limit/offset adalah nilai efektif setelah default).

Tidak ada pengecekan role di sini (route sudah dijaga `RequireRole`), tidak ada audit, tidak ada `Clock`/`Hasher`/`audit.Recorder`.

#### BE-5. `listusers/handler.go`

```go
type Handler struct{ uc *Usecase }

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
    in, fe := parseQuery(r.URL.Query())
    if len(fe) > 0 { respond.WriteError(w, r, base.InvalidFields(fe)); return }
    out, err := h.uc.Execute(r.Context(), in)
    if err != nil { respond.WriteError(w, r, err); return }
    respond.WriteJSON(w, http.StatusOK, out)
}
```

Header `Cache-Control: no-store` dipasang (daftar user berisi data pribadi dan tidak boleh di-cache proxy).

#### BE-6. `listusers/init.go` dan wiring

```go
func Init(d app.Deps, s base.Shared) {
    h := &Handler{uc: NewUsecase(NewRepo(d.DB))}
    d.API.With(
        middleware.RequireAuth(),
        middleware.RequireRole(resolvesession.RoleSuperAdmin),
    ).Get("/users", h.List)
}
```

`s base.Shared` tidak dipakai (dipertahankan agar signature `Init` seragam; gunakan `_ base.Shared`). Tambahkan satu baris `listusers.Init(d, shared)` di `auth/base.go` pada PR ini.

Catatan: `*db.DB` memenuhi `db.Querier` (dijamin `var _ Querier = (*DB)(nil)` di T00).

#### BE-7. Test (lihat Test Scenario)

Tulis test unit entity dan usecase lebih dulu (TDD), lalu integrasi repository dan HTTP.

### Frontend (slice daftar user)

Struktur mengikuti `docs/superpowers/plans/core-auth/frontend-plan.md` bila sudah ada (folder `domain/core/auth/users/`). Bila rencana FE belum ada, buat hanya file di bawah ini. Fitur lain (buat, ubah, nonaktif, reset password) ditambah oleh usecase masing-masing; `index.ts` hanya ditambah ekspor, tidak ditulis ulang.

#### FE-1. `types.ts` dan `services/users.service.ts`

- Zod schema `userSchema` (id, email, name, role enum, is_active, must_change_password, created_at, created_by nullable, created_by_label nullable, updated_at, last_login_at nullable) dan `userListSchema {items: userSchema[], total: int, limit: int, offset: int}`. Tipe diturunkan lewat `z.infer`.
- `listUsers(params: {role?: Role; status?: 'active'|'inactive'; q?: string; limit: number; offset: number}, opts?: {signal?: AbortSignal}): Promise<UserList>`: memanggil `get('/api/users', {query, schema: userListSchema, signal})`. Nilai kosong dan `undefined` dibuang dari query sebelum dikirim.
- Periksa signature sebenarnya `RequestOptions` di `shared/services/http.ts` (nama opsi schema dan signal); sesuaikan, jangan membuat client HTTP baru.

#### FE-2. `stores/usersList.ts`

State: `filters {role, status, q}`, `page` (mulai 1), `pageSize` (tetap 20), `data`, `loading`, `error`.
- `offset = (page-1)*pageSize`.
- Mengubah filter mereset `page=1`. Input `q` di-debounce 300 ms.
- Setiap fetch baru membatalkan fetch sebelumnya (`AbortController`) dan respons basi diabaikan (nomor urut permintaan), agar respons lambat tidak menimpa hasil terbaru.
- Bila respons `total` > 0 tetapi `items` kosong dan `page > 1` (data berkurang), lompat ke halaman terakhir yang valid sekali, tanpa loop.
- Error 401 ditangani handler global `setUnauthorizedHandler`; 403 tampil di `ErrorState` (bukan mengarah ke data kosong).
- `refresh()` diekspos untuk dipanggil setelah aksi buat/ubah/nonaktif oleh feature lain.

#### FE-3. Komponen presentasi (`components/`)

- `UsersFilterBar.svelte`: select role (Semua, Super Admin, Developer), select status (Semua, Aktif, Nonaktif), input pencarian. Props dan callback saja; tanpa import router.
- `UsersTable.svelte`: memakai `shared/components/Table`; kolom Nama, Email, Role, Status, Login terakhir (`last_login_at` null menjadi teks "Belum pernah"; tanggal diformat sesuai locale i18n). Slot aksi (`actions` snippet) disediakan kosong untuk diisi feature lain. Kolom status berupa teks (Aktif/Nonaktif), bukan hanya warna.
- Keadaan: memuat (`Spinner`), kosong tanpa filter ("Belum ada user"), kosong dengan filter ("Tidak ada user yang cocok" dan tombol reset filter), error (`ErrorState` dengan tombol coba lagi).
- Paginasi: `shared/components/Pagination` (`page`, `pageSize`, `total`, `onchange`).

#### FE-4. `views/UsersListView.svelte` dan `index.ts`

View tipis: menyusun `UsersFilterBar`, `UsersTable`, `Pagination` dengan store. Hanya `index.ts` yang mengekspos `UsersListView`, `refreshUsers`, dan tipe `User`.

#### FE-5. i18n

Kunci baru di `shared/i18n/id.json` dan `en.json` (kedua file, test paritas kunci i18n harus lulus): `users.title`, `users.filter.role`, `users.filter.status`, `users.filter.search`, `users.filter.reset`, `users.role.super_admin`, `users.role.developer`, `users.status.active`, `users.status.inactive`, `users.col.name`, `users.col.email`, `users.col.role`, `users.col.status`, `users.col.lastLogin`, `users.neverLoggedIn`, `users.empty`, `users.emptyFiltered`, `users.error.load`.

#### APP-1. Komposisi di `app/`

- `app/views/UsersView.svelte`: membungkus `UsersListView` dari `index.ts` (feature tidak meng-import router).
- `app/routes/index.ts`: route `/admin/users` dengan `meta.roles: ['super_admin']` (guard mengarah ke NotFound untuk developer).
- `app/layouts/AppLayout.svelte`: menu "Users" hanya terlihat bila `session.role === 'super_admin'`.
- Bila route atau menu sudah ditambah usecase lain (mis. `createuser`), cukup lengkapi; jangan duplikasi.

## Migration

Tidak ada. Tabel `users` dan indeks `users_role_active_idx` dibuat F1 (`backend/migrations/core/20261021090000000_create_auth_tables.sql`). Tidak ada indeks `lower(name)` (overview D6).

## Changes

Backend:
- `backend/internal/domain/core/internal/auth/base/userfilter.go` (baru): `UserFilter`, `UserFilterInput`, `NewUserFilter`, `LikePattern`, konstanta.
- `backend/internal/domain/core/internal/auth/base/userfilter_test.go` (baru).
- `backend/internal/domain/core/internal/auth/listusers/init.go` (baru).
- `backend/internal/domain/core/internal/auth/listusers/handler.go` (baru).
- `backend/internal/domain/core/internal/auth/listusers/params.go` (baru).
- `backend/internal/domain/core/internal/auth/listusers/usecase.go` (baru).
- `backend/internal/domain/core/internal/auth/listusers/repository.go` (baru).
- `backend/internal/domain/core/internal/auth/listusers/{params,usecase,handler}_test.go`, `repository_integration_test.go` (baru).
- `backend/internal/domain/core/internal/auth/base.go` (ubah, satu baris): `listusers.Init(d, shared)`.
- `backend/internal/domain/core/internal/auth/auth_integration_test.go` (ubah atau buat): baris `GET /api/users` pada `TestT01_AC7_DeveloperForbiddenOnUsers`, plus `TestT01_ListUsers_*` (lihat di bawah).
- `api/modules/core/auth.yml`, `api/openapi.yml`: hanya bila BE-0 menemukan selisih.

Frontend:
- `frontend/src/domain/core/auth/users/types.ts`, `services/users.service.ts`, `stores/usersList.ts`, `components/{UsersFilterBar,UsersTable}.svelte`, `views/UsersListView.svelte`, `index.ts` (baru atau ditambah), beserta `*.test.ts`.
- `frontend/src/app/views/UsersView.svelte` (baru atau ubah), `app/routes/index.ts`, `app/layouts/AppLayout.svelte` (ubah).
- `frontend/src/shared/i18n/id.json`, `en.json` (ubah).

Tidak berubah: `base/store` (query list ada di repository feature), `Shared`, konfigurasi, konstanta bus, kontrak bus, migrasi, `docs/tickets/README.md`.

## Test Scenario

Penamaan integrasi mengikuti rule 13.4 (`TestT01_ACn_...`). Integrasi memakai `db.TestDB(t)` dan menerapkan migrasi `migrations.FS`.

### Unit entity (`base/userfilter_test.go`, tanpa mock)
- `TestNewUserFilter_Defaults`: input kosong menghasilkan `Limit=20`, `Offset=0`, `Role=""`, `IsActive=nil`, `Query=""`.
- `TestNewUserFilter_Role`: `super_admin` dan `developer` lolos; `" developer "` di-trim lolos; `admin` dan `DEVELOPER` ditolak dengan pesan `role harus super_admin atau developer`.
- `TestNewUserFilter_Status`: `active` menjadi `*IsActive == true`; `inactive` menjadi `false`; `gone` dan `ACTIVE` ditolak.
- `TestNewUserFilter_LimitBounds` (tabel): 1 dan 100 lolos; 0, -1 menghasilkan `minimal 1`; 101 dan 500 menghasilkan `maksimal 100`.
- `TestNewUserFilter_OffsetBounds`: 0 lolos; -1 ditolak; 1_000_001 ditolak.
- `TestNewUserFilter_Query`: `"  sari  "` menjadi `"sari"`; spasi saja menjadi `""`; 100 rune multibyte lolos; 101 rune ditolak; `"a\nb"` dan `"a\x00b"` ditolak.
- `TestNewUserFilter_AggregatesErrors`: `role=x`, `status=y`, `limit=999` menghasilkan satu `FieldErrors` berisi tiga kunci.
- `TestUserFilter_LikePattern`: `Query="50%_a\\"` menghasilkan `"%50\%\_a\\\\%"`; `Query=""` menghasilkan `""`.

### Unit feature (`listusers/*_test.go`, repository palsu)
- `TestParseQuery_Shape`: `limit=abc` dan `offset=1.5` menghasilkan `fe["limit"]`/`fe["offset"]`; `limit=` kosong tidak menghasilkan error dan pointer nil; `limit=20` menghasilkan pointer 20; parameter tak dikenal diabaikan.
- `TestUsecase_InvalidFilterIs400NoRepoCall`: `status=gone` menghasilkan `*errs.Error` `CodeInvalid` dengan `details.fields.status`, repository tidak dipanggil.
- `TestUsecase_MapsToDTOWithoutHash`: repository palsu mengembalikan `User` dengan `PasswordHash: "$argon2id$secret"`; JSON `Output` tidak memuat `password_hash` maupun `$argon2id`; memuat `last_login_at`.
- `TestUsecase_EmptyItemsIsNotNil`: hasil kosong di-marshal sebagai `"items":[]`, bukan `null`.
- `TestUsecase_EffectiveLimitOffsetEchoed`: tanpa limit, `Output.Limit==20`, `Offset==0`.
- `TestUsecase_RepoErrorWrapped500`: error repository menghasilkan error non-`*errs.Error` dengan pesan berawalan `listusers:`; `respond.WriteError` menghasilkan 500 `INTERNAL` generik (tidak memuat teks SQL).
- `TestHandler_Headers`: respons 200 membawa `Cache-Control: no-store` dan `Content-Type: application/json`.

### Integrasi repository (`repository_integration_test.go`, Postgres nyata)
Seed: 7 user dengan nama campur huruf besar dan kecil, dua `super_admin` (satu nonaktif), lima `developer`, satu dengan `last_login_at` terisi, satu `name='50%_off'`, dua dengan nama sama (`Sari`) berbeda id.
- `TestListUsers_OrderNameThenID`: urutan `name ASC, id ASC` stabil; dua `Sari` terurut oleh id.
- `TestListUsers_FilterRole`, `TestListUsers_FilterStatus`, `TestListUsers_FilterRoleAndStatus` (kombinasi AND).
- `TestListUsers_SearchMatchesNameOrEmailCaseInsensitive`: `q=SAR` cocok dengan nama `Sari` dan email `x@sari.id`.
- `TestListUsers_SearchEscapesWildcards`: `q=%` hanya mengembalikan nama yang benar-benar memuat `%` (`50%_off`), bukan semua baris; `q=_` dan `q=\` juga literal. Mencegah ILIKE injection.
- `TestListUsers_SQLInjectionInQuery`: `q="'; DROP TABLE users; --"` tidak error, mengembalikan 0 item, tabel `users` masih ada.
- `TestListUsers_Pagination`: limit 3 offset 0 lalu offset 3 lalu offset 6; gabungan halaman sama dengan daftar penuh, tanpa duplikat atau lompatan; `total` sama di setiap halaman.
- `TestListUsers_OffsetBeyondEnd_TotalStillCorrect`: `offset=100` menghasilkan `items=[]` dan `total=7` (jalur `Count`); dengan filter, total sesuai filter.
- `TestListUsers_EmptyDB`: `items=[]`, `total=0`.
- `TestListUsers_NeverSelectsPasswordHash`: kolom `password_hash` kosong pada hasil, meski di DB terisi.
- `TestListUsers_LastLoginAtNullAndSet`: `nil` untuk yang belum login, waktu UTC untuk yang sudah.

### Integrasi HTTP (`auth_integration_test.go`, router lengkap, bus nyata)
- `TestT01_AC7_DeveloperForbiddenOnUsers`: file ini milik bersama. Bila belum ada, buat tabel route (`GET /api/users`, `POST /api/users`, `GET /api/users/{id}`, `PATCH /api/users/{id}`, `POST /api/users/{id}/deactivate`, `/activate`, `/reset-password`) dan assert developer mendapat 403 `FORBIDDEN`. Usecase ini mengisi baris `GET /api/users`; baris route yang belum diimplementasikan ditandai `t.Skip` per subtest sampai usecasenya masuk. Tambahan khusus: tanpa cookie 401 `UNAUTHORIZED`.
- `TestT01_ListUsers_SuperAdminOK`: login admin (flag sudah dibersihkan), `GET /api/users` 200, bentuk `{items,total,limit,offset}`, `limit==20`, `offset==0`.
- `TestT01_AC4_ListUsersBlockedByPasswordGate`: super_admin dengan `must_change_password=true` mendapat 403 `PASSWORD_CHANGE_REQUIRED` (bukan 200 dan bukan 403 `FORBIDDEN`).
- `TestT01_ListUsers_ValidationErrors`: `status=gone&limit=500` menghasilkan 400 dengan `details.fields.status` dan `details.fields.limit`; `limit=abc` 400 `details.fields.limit`; `role=admin` 400.
- `TestT01_ListUsers_ResponseNeverContainsHash`: body mentah tidak memuat `password_hash`, `$argon2id`, atau string hash user seed (bagian dari semangat AC8).
- `TestT01_ListUsers_ReflectsDeactivationImmediately`: user dinonaktifkan langsung terlihat sebagai `is_active:false` dan terfilter keluar dari `status=active`.
- `TestT01_ListUsers_NoAuditEntry`: setelah beberapa GET, handler audit palsu tidak menerima `record.Request` (AC9: tidak ada event untuk aksi baca).

### Frontend (vitest)
- `users.service.test.ts`: query hanya memuat nilai terisi; respons valid lolos Zod; respons tanpa `total` atau `last_login_at` yang bukan string/null ditolak; error 400 menghasilkan `ApiError` dengan `details.fields`.
- `usersList.test.ts`: mengubah filter mereset halaman ke 1; debounce `q` 300 ms (fake timer, hanya satu fetch); respons lama tidak menimpa respons baru (urutan terbalik); halaman kosong dengan `total>0` melompat ke halaman terakhir sekali; error 403 mengisi `error`.
- `UsersTable.test.ts`: `last_login_at` null menampilkan "Belum pernah"; status dirender sebagai teks; kosong tanpa filter versus kosong dengan filter menampilkan pesan berbeda.
- `UsersFilterBar.test.ts`: callback dipanggil dengan nilai terpilih; tombol reset mengosongkan.
- Guard: `guard.test.ts` ditambah kasus developer ke `/admin/users` menghasilkan view NotFound.
- Paritas i18n (`id.json` dan `en.json` punya kunci `users.*` yang sama).

### Arsitektur dan kualitas
- `TestT00_AC4_RepoIsClean` (archtest) lulus: `listusers` tidak meng-import feature lain, hanya `base`, `lib`, dan infra.
- `go vet`, `golangci-lint`, `make api-check` lulus; `npm run depcruise`, `npm run lint`, `npm test` lulus.

## E2E Scenarios

Playwright terhadap binary; prasyarat: `migrate up`, admin dibuat lewat `create-admin`, beberapa user seed lewat API `createuser`.

Positif:
1. Admin login lalu membuka `/admin/users`: tabel tampil dengan kolom Nama, Email, Role, Status, Login terakhir; urutan nama ascending; user yang belum pernah login menampilkan "Belum pernah".
2. Filter role "Developer": hanya developer; filter status "Nonaktif": hanya user nonaktif; kombinasi keduanya memberi irisan.
3. Pencarian `sar` (debounce): daftar menyempit ke nama atau email yang mengandung `sar` tanpa memandang huruf besar kecil; menghapus teks mengembalikan daftar penuh.
4. Pencarian `%` atau `_` tidak menampilkan semua user (literal).
5. Dengan lebih dari 20 user: halaman 1 berisi 20 baris, tombol Berikutnya membuka halaman 2 dengan sisa baris; kembali ke halaman 1 menampilkan baris yang sama. Mengubah filter mengembalikan ke halaman 1.
6. Admin menonaktifkan seorang user (fitur lain), kembali ke daftar, status berubah menjadi Nonaktif tanpa muat ulang penuh (melalui `refresh`).
7. Reload di `/admin/users?...` mempertahankan sesi dan daftar muncul kembali.

Negatif:
1. Developer membuka `/admin/users` langsung: UI menampilkan NotFound; panggilan manual `fetch('/api/users')` mendapat 403 `FORBIDDEN`.
2. Tanpa login: `/admin/users` mengarahkan ke `/login`; `GET /api/users` 401.
3. Super admin dengan password sementara: redirect ke `/change-password`; `GET /api/users` manual 403 `PASSWORD_CHANGE_REQUIRED`.
4. `GET /api/users?limit=500` dan `?status=gone` dan `?limit=abc` manual: 400 dengan `details.fields` per kolom.
5. Pencarian tanpa hasil menampilkan "Tidak ada user yang cocok" dan tombol reset yang berfungsi.
6. API gagal (server mengembalikan 500): `ErrorState` dengan tombol coba lagi; setelah server pulih, klik coba lagi memuat daftar.
7. Sesi admin dinonaktifkan di tab lain: pemuatan ulang daftar menghasilkan 401 dan redirect ke `/login`.
8. Pencarian `'; DROP TABLE users; --` mengembalikan daftar kosong, aplikasi tetap normal.

## Request/Response body

Semua error memakai format seragam T00: `{ "error": { "code", "message", "details" }, "trace_id" }`. `GET` tidak butuh header `X-Requested-With`, namun cookie `grimoire_session` wajib.

### GET /api/users

Contoh: `GET /api/users?role=developer&status=active&q=sar&limit=20&offset=0`

200 (header `Cache-Control: no-store`):

```json
{
  "items": [
    {
      "id": "7d1e2c3a-5b4f-4a6d-8e9f-0a1b2c3d4e5f",
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
    },
    {
      "id": "9a8b7c6d-1e2f-4a3b-b4c5-d6e7f8091a2b",
      "email": "sarah@tim.id",
      "name": "Sarah",
      "role": "developer",
      "is_active": true,
      "must_change_password": true,
      "created_at": "2026-10-21T10:00:00Z",
      "created_by": "0b7c6a4e-2f1d-4c8a-9e3b-5d6f7a8b9c0d",
      "created_by_label": "admin@tim.id",
      "updated_at": "2026-10-21T10:00:00Z",
      "last_login_at": null
    }
  ],
  "total": 2,
  "limit": 20,
  "offset": 0
}
```

200 tanpa parameter (default): `limit: 20`, `offset: 0`, semua user urut `name, id`.

200 hasil kosong (offset melewati data; `total` tetap total yang cocok filter):

```json
{ "items": [], "total": 7, "limit": 20, "offset": 100 }
```

400 nilai tidak valid (`status=gone&limit=500`):

```json
{
  "error": {
    "code": "INVALID_ARGUMENT",
    "message": "permintaan tidak valid",
    "details": { "fields": { "status": "harus active atau inactive", "limit": "maksimal 100" } }
  },
  "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33"
}
```

400 bentuk salah (`limit=abc`):

```json
{
  "error": {
    "code": "INVALID_ARGUMENT",
    "message": "permintaan tidak valid",
    "details": { "fields": { "limit": "harus berupa angka bulat" } }
  },
  "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a34"
}
```

400 role salah (`role=admin`):

```json
{
  "error": {
    "code": "INVALID_ARGUMENT",
    "message": "permintaan tidak valid",
    "details": { "fields": { "role": "role harus super_admin atau developer" } }
  },
  "trace_id": "..."
}
```

401 tanpa sesi atau sesi tidak valid:

```json
{ "error": { "code": "UNAUTHORIZED", "message": "perlu login", "details": null }, "trace_id": "..." }
```

403 developer (AC7):

```json
{ "error": { "code": "FORBIDDEN", "message": "akses ditolak", "details": null }, "trace_id": "..." }
```

403 password wajib diganti:

```json
{ "error": { "code": "PASSWORD_CHANGE_REQUIRED", "message": "password wajib diganti sebelum melanjutkan", "details": null }, "trace_id": "..." }
```

500 (kegagalan DB; tanpa detail internal):

```json
{ "error": { "code": "INTERNAL", "message": "terjadi kesalahan internal", "details": null }, "trace_id": "..." }
```

(Teks pesan 500 mengikuti `respond.WriteError` T00; test memeriksa kode `INTERNAL` dan bahwa pesan tidak memuat SQL.)

### Bus

Tidak ada kontrak bus untuk usecase ini.

### Catatan keamanan ringkas
- Hash password tidak pernah dibaca dari DB oleh query ini dan tidak ada di `UserDTO`.
- Input pencarian hanya masuk sebagai argumen terparameter dengan wildcard ter-escape.
- Respons tidak di-cache. Tidak ada log yang memuat nilai `q` (log akses T00 hanya mencatat path, bukan query).
