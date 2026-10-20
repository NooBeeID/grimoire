# T03 project/management — Usecase `listmembers`

Mengikuti `docs/superpowers/plans/project-management/overview.md` (overview menang bila bertentangan). Kontrak bus yang dikonsumsi (`core.auth.get-user`) persis dari `docs/superpowers/plans/core-auth/` (T01, `usecase-getuserbus.md`); tidak didefinisikan ulang di sini. Konteks: belum ada `CLAUDE.md` dan `.claude/rules/`; aturan arsitektur dari `docs/tickets/README.md` dan overview.

## 1. Metadata

| Atribut | Nilai |
|---|---|
| Ticket | T03 |
| Scope | fullstack pada tiket; plan ini backend saja. UI daftar anggota (tab Members) dirinci di `frontend-plan.md` |
| Domain | project |
| Module | management |
| Feature | `listmembers` (gelombang B, U8) |
| Endpoint | `GET /api/projects/{projectID}/members` |
| Method | GET |
| Version | tanpa prefix versi di URL (prefix `/api`); `management.yml` `info.version: 0.1.0` |
| Auth | `middleware.RequireAuth()`; otorisasi project di usecase: `base.Require(level, base.PermView)` (anggota, owner, super_admin) |
| Resource root | `backend/internal/domain/project/internal/management` |
| Folder feature | `backend/internal/domain/project/internal/management/listmembers/` |
| Konsumsi bus | `core.auth.get-user` (Call, T01) via `base/users.Lookup`, satu panggilan per anggota. Error -> 500 |
| Audit | tidak ada (aksi baca) |
| Tabel / migrasi | membaca `projects` dan `project_members`; tidak ada migrasi |
| AC | AC2 (member boleh melihat daftar anggota); bagian dari `TestT03_Security_ProjectIsolation` |

## 2. Summary Plan

`listmembers` mengembalikan seluruh anggota sebuah project beserta identitas tampilnya (nama, email, status aktif), tanpa paginasi.

Alur usecase (orkestrasi saja, satu usecase, tidak dipecah):

1. Bangun aktor dari `reqctx.IdentityFrom(ctx)` lewat `base.ActorFrom`; `!ok` -> `errs.New(errs.CodeUnauthorized, "perlu login")`.
2. Otorisasi project sesuai overview 3.4: `store.FindAccess(ctx, projectID, actor.ID)`; `!found` -> `base.ErrProjectNotFound` (404, termasuk `projectID` bukan uuid karena store tidak query); `level := base.ResolveLevel(actor.Role, memberRole, true)`; `base.Require(level, base.PermView)`. Level `none` -> 404 seragam (tidak membocorkan keberadaan project). Tidak ada `get-user` untuk aktor (D6).
3. `store.ListMembers(ctx, projectID)` -> baris `project_members` (urut `user_id`, hanya untuk determinisme).
4. Untuk tiap anggota: `users.Get(ctx, member.UserID)` (`core.auth.get-user`). Error apa pun (termasuk `bus.ErrNotRegistered`) -> hentikan dan kembalikan error terbungkus (500 generik). Alasan: daftar anggota adalah isi utama respons, data wajib lengkap; berbeda dengan label audit yang fail-soft di usecase lain.
5. `member.ToDTO(userResponse)` (entity). `Found=false` -> `name`/`email` null, `found=false`, `is_active=false`. Anggota nonaktif tetap dicantumkan (`is_active=false`); keanggotaan tetap tercatat (aturan tiket).
6. `base.SortMembers(dtos)`: owner dulu, lalu `strings.ToLower(name)` (anggota tanpa nama di akhir grupnya), lalu `user_id`.
7. Kembalikan `{ "items": [...] }`. Project tanpa anggota -> `items: []` (bukan `null`).

Keputusan dan batasan:
- Tanpa paginasi, tanpa audit, tanpa filter.
- Panggilan `get-user` sekuensial dan berhenti di error pertama. Tidak ada endpoint batch di katalog; skala puluhan anggota wajar (overview 14). Sekuensial dipilih agar sederhana dan deterministik; tidak ada goroutine. Pengecekan `ctx.Err()` di tiap iterasi agar klien yang putus tidak menghabiskan panggilan bus.
- `FindAccess` dan `ListMembers` adalah dua query terpisah tanpa transaksi (read-only, tidak butuh snapshot konsisten). Bila project terhapus di antara keduanya, `ListMembers` mengembalikan daftar kosong dan respons tetap 200 `items: []`; dapat diterima.
- Tidak membocorkan field sensitif: `MemberDTO` hanya memuat `user_id, name, email, is_active, found, role, added_at, added_by, added_by_label` (overview 4.7). Role sistem user (`developer`/`super_admin`) tidak diekspos.
- Tidak ada rule bisnis baru; urutan dan pemetaan DTO sudah ada di `base` (foundation). Usecase tidak menulis logika sorting sendiri.
- Tidak ada `repository.go`: tidak ada query khusus feature; `base/store` (satu-satunya yang menyentuh SQL) dipakai langsung lewat interface kecil di usecase (rule 5.6).

Prasyarat dari foundation F (overview bagian 7). Plan ini TIDAK membuatnya; bila belum ada di branch, selesaikan F dulu dan jangan duplikasi:
- `base.Actor`, `base.ActorFrom`, `base.RequestMeta` tidak dipakai di sini (tanpa audit); `base.ActorFrom`, `base.ResolveLevel`, `base.Require`, `base.PermView`, `base.Member`, `base.MemberDTO`, `Member.ToDTO`, `base.SortMembers`, `base.ErrProjectNotFound`.
- `base/store`: `FindAccess`, `ListMembers`.
- `base/users`: `Lookup` dan `New(b)`.
- Kontrak `lib/contracts/core/auth/getuser` dan konstanta `constants.BUS_CORE_AUTH_GET_USER` (dari T01; bila T01 belum merge, buat persis seperti plan T01 5.1/5.2).
- Path dan schema `GET /api/projects/{projectID}/members` di `api/modules/project/management.yml` (F langkah 8).

## 3. Plan (langkah berurutan)

TDD: tulis test (bagian 7) lebih dulu per langkah.

1. **Verifikasi prasyarat** (tanpa mengubahnya). Baca `backend/internal/infra/app` (`Deps`), `backend/internal/infra/http/respond` (`WriteJSON`, `WriteError`), `backend/internal/infra/middleware` (`RequireAuth`), dan pola `Init` feature HTTP yang sudah ada (mis. `createproject`/`getproject` bila sudah merge) lalu ikuti persis. Pastikan semua simbol di bagian 2 ada.

2. **`listmembers/usecase.go`** (package `listmembers`):

   ```go
   type AccessFinder interface {
       FindAccess(ctx context.Context, projectID, userID string) (p base.Project, memberRole string, found bool, err error)
   }

   type MemberLister interface {
       ListMembers(ctx context.Context, projectID string) ([]base.Member, error)
   }

   // Satu tipe konkret (*store.Store) memenuhi keduanya; usecase menerima dua interface kecil.
   type Usecase struct {
       access  AccessFinder
       members MemberLister
       users   users.Lookup
   }

   func NewUsecase(a AccessFinder, m MemberLister, u users.Lookup) *Usecase

   type Result struct{ Items []base.MemberDTO }

   func (u *Usecase) Execute(ctx context.Context, projectID string) (Result, error)
   ```

   Alur `Execute`, berurutan:
   1. `actor, ok := base.ActorFrom(reqctx.IdentityFrom(ctx))`; `!ok` -> `errs.New(errs.CodeUnauthorized, "perlu login")`.
   2. `_, memberRole, found, err := u.access.FindAccess(ctx, projectID, actor.ID)`; err -> `fmt.Errorf("listmembers: find access: %w", err)`.
   3. `!found` -> `return Result{}, base.ErrProjectNotFound`.
   4. `level := base.ResolveLevel(actor.Role, memberRole, true)`; `if err := base.Require(level, base.PermView); err != nil { return Result{}, err }`.
   5. `ms, err := u.members.ListMembers(ctx, projectID)`; err -> `fmt.Errorf("listmembers: list members: %w", err)`.
   6. `items := make([]base.MemberDTO, 0, len(ms))`; untuk tiap `m` di `ms`:
      - `if err := ctx.Err(); err != nil { return Result{}, fmt.Errorf("listmembers: %w", err) }`;
      - `usr, err := u.users.Get(ctx, m.UserID)`; err -> `fmt.Errorf("listmembers: get-user %s: %w", m.UserID, err)` (hentikan; fail seluruh request);
      - `items = append(items, m.ToDTO(usr))`.
   7. `base.SortMembers(items)`; `return Result{Items: items}, nil`.

   Catatan: `users.Lookup` yang diterima adalah interface (rule 5.6); jangan memakai `*users.BusLookup`. Usecase tidak membangun SQL, tidak membuka transaksi, tidak memanggil bus selain lewat `users.Lookup`.

3. **`listmembers/handler.go`**:

   ```go
   type handler struct{ uc *Usecase }

   func newHandler(uc *Usecase) *handler

   type response struct {
       Items []base.MemberDTO `json:"items"`
   }

   func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
       res, err := h.uc.Execute(r.Context(), chi.URLParam(r, "projectID"))
       if err != nil {
           respond.WriteError(w, r, err)
           return
       }
       respond.WriteJSON(w, http.StatusOK, response{Items: res.Items})
   }
   ```

   Tanpa `params.go`: tidak ada body dan tidak ada query param; satu-satunya input adalah path param yang diteruskan apa adanya (overview 3.3, validasi uuid dilakukan store). Query string yang tidak dikenal diabaikan. Error infra (get-user gagal, DB) otomatis menjadi 500 generik oleh `respond.WriteError` karena bukan `*errs.Error`; pesan internal tidak bocor ke klien.

4. **`listmembers/init.go`**:

   ```go
   func Init(d app.Deps) {
       s := store.New(d.DB)
       uc := NewUsecase(s, s, users.New(d.Bus))
       d.API.With(middleware.RequireAuth()).Get("/projects/{projectID}/members", newHandler(uc).ServeHTTP)
   }
   ```

   Pola route penuh langsung di `d.API` (D4), nama param `{projectID}`. Sesuaikan bentuk `Get(...)` dengan yang dipakai feature HTTP lain di `main` (mis. `http.HandlerFunc`). Tidak ada GET dengan CSRF; middleware CSRF T00 hanya menolak method mutasi.

5. **Wiring**: tambah satu baris `listmembers.Init(d)` di `backend/internal/domain/project/internal/management/base.go` (fungsi `Register`). Hanya satu baris, tanpa mengubah baris lain (minimal konflik merge dengan usecase paralel).

6. **OpenAPI**: verifikasi operasi `listMembers` di `api/modules/project/management.yml` sesuai bagian 8; perbaiki hanya bagian operasi ini bila ada selisih, lalu `make api-bundle` dan commit `api/openapi.yml`; `make api-check` harus lulus. Tidak ada path baru di `api/root.yml` (path `/api/projects/{projectID}/members` sudah ada dari F, dipakai bersama `addmember`).

7. **Test** (bagian 7): `usecase_test.go`, `handler_test.go`, `listmembers_integration_test.go` (tag `integration`).

8. **Verifikasi**: `cd backend && go vet ./... && go test ./internal/domain/project/... && go test -tags integration ./internal/domain/project/internal/management/listmembers/...`; `make api-check`; archtest T00 (`TestT00_AC4_RepoIsClean`) lulus (feature hanya meng-import `base`, `base/store`, `base/users`, `lib/*`, `infra/*`, tidak meng-import feature lain).

Kontrak bus yang dikonsumsi (persis plan T01, tidak didefinisikan ulang):

```go
bus.CallTyped[getuser.Response](ctx, b, constants.BUS_CORE_AUTH_GET_USER, getuser.Request{UserID: id})
// getuser.Response{Found, ID, Name, Email, Role string, IsActive bool}
// bukan uuid / tidak ada -> Found=false, err nil. Nonaktif -> Found=true, IsActive=false.
```

Dipanggil lewat `users.Lookup.Get` (foundation). Feature ini tidak menyediakan endpoint bus.

## 4. Migration

Tidak ada. Memakai tabel dari `backend/migrations/project/20261022090000000_create_projects.sql` (foundation). Hanya BACA.

Query (di `base/store`, milik foundation; feature ini tidak menulis SQL):

```sql
-- FindAccess
SELECT p.id, p.name, p.description, p.created_by, p.created_by_label, p.created_at, p.updated_at,
       COALESCE(m.role, '') AS member_role
FROM projects p
LEFT JOIN project_members m ON m.project_id = p.id AND m.user_id = $2
WHERE p.id = $1

-- ListMembers
SELECT project_id, user_id, role, added_by, added_by_label, added_at
FROM project_members
WHERE project_id = $1
ORDER BY user_id
```

Keduanya terlayani PK `(project_id, user_id)`.

## 5. Changes

Baru:
- `backend/internal/domain/project/internal/management/listmembers/init.go`
- `backend/internal/domain/project/internal/management/listmembers/handler.go`
- `backend/internal/domain/project/internal/management/listmembers/usecase.go`
- `backend/internal/domain/project/internal/management/listmembers/usecase_test.go`
- `backend/internal/domain/project/internal/management/listmembers/handler_test.go`
- `backend/internal/domain/project/internal/management/listmembers/listmembers_integration_test.go`

Ubah:
- `backend/internal/domain/project/internal/management/base.go`: tambah `listmembers.Init(d)` di `Register`.
- `api/modules/project/management.yml` dan `api/openapi.yml`: hanya bila operasi `listMembers` belum sesuai bagian 8.

Tidak diubah oleh usecase ini (sudah dari foundation): `base/*` (termasuk `Member.ToDTO`, `SortMembers`), `base/store`, `base/users`, kontrak, `lib/constants/bus.go`, migrasi, `api/root.yml`, i18n. Frontend (tab Members) di `frontend-plan.md`.

Catatan jika ditemukan kekurangan di foundation (mis. `SortMembers` belum menangani `name=nil`): perbaiki di `base/member`/`base/dto` beserta unit test-nya sebagai bagian foundation, bukan di dalam feature ini.

## 6. Keamanan dan error

| Kondisi | Hasil |
|---|---|
| Tanpa sesi | 401 (middleware `RequireAuth`) |
| `projectID` bukan uuid / project tidak ada | 404 `NOT_FOUND` "project tidak ditemukan" |
| Developer bukan anggota | 404 yang sama persis (tidak bocor, rule 7.3) |
| Developer anggota (member atau owner) | 200 |
| Super Admin (anggota atau bukan) | 200 |
| `get-user` gagal untuk salah satu anggota (error bus/DB, termasuk `ErrNotRegistered`) | 500 `INTERNAL`, tanpa isi parsial |
| `get-user` `Found=false` untuk anggota | 200, item dengan `found=false`, `name/email=null`, `is_active=false` |
| DB gagal | 500 |

Otorisasi tidak pernah memakai input klien selain `projectID`; identitas dari konteks (`reqctx`). Log tidak memuat email/nama anggota; field log standar `project_id`, `feature=listmembers`.

## 7. Test Scenario

Penamaan: `TestT03_ACn_...` untuk AC; lainnya `Test<Fungsi>_<Kasus>`.

### Unit usecase (`usecase_test.go`, repo dan `users.Lookup` palsu, hitung panggilan)

Identitas disuntikkan lewat `reqctx.WithIdentity`.

- `TestExecute_NoIdentity`: konteks tanpa identitas -> `CodeUnauthorized`; repo dan users tidak dipanggil.
- `TestExecute_ProjectNotFound`: `found=false` -> `base.ErrProjectNotFound`; `ListMembers` dan `Get` tidak dipanggil.
- `TestExecute_NonMemberDeveloperGets404`: `found=true`, `memberRole=""`, role developer -> `ErrProjectNotFound` (pesan dan code sama dengan project tak ada); `ListMembers` = 0 panggilan.
- `TestT03_AC2_MemberCanListMembers`: `memberRole=member` -> sukses; `ListMembers` dipanggil 1x.
- `TestExecute_OwnerCanList`: `memberRole=owner` -> sukses.
- `TestExecute_SuperAdminNotMember`: role `super_admin`, `memberRole=""` -> sukses; tidak ada `Get` untuk aktor (hitung panggilan `Get` hanya untuk anggota).
- `TestExecute_EnrichesEveryMember`: 3 anggota -> `Get` dipanggil tepat 3x dengan `UserID` masing-masing; DTO memuat `name`, `email`, `is_active`, `found=true`, `role`, `added_at`, `added_by`, `added_by_label`.
- `TestExecute_SortsOwnerFirstThenName`: anggota (member "budi", owner "Zed", member "Andi", owner "adi") -> urutan `adi(owner)`, `Zed(owner)`, `Andi`, `budi` (nama case-insensitive).
- `TestExecute_UnknownUserKeptWithFoundFalse`: `Get` mengembalikan `Found=false` -> item `found=false`, `name=nil`, `email=nil`, `is_active=false`, tetap diurut di akhir grup rolenya; tidak error.
- `TestExecute_InactiveUserListed`: `Found=true, IsActive=false` -> item `is_active=false`, tetap muncul.
- `TestExecute_EmptyMembersReturnsEmptySlice`: `ListMembers` -> `[]` -> `Result.Items` non-nil, `len==0` (JSON `[]`); `Get` = 0.
- `TestExecute_GetUserErrorFailsWhole`: anggota ke-2 dari 3 gagal -> error terbungkus, `errors.Is` ke error asal, `Result` kosong, `Get` hanya dipanggil 2x (berhenti).
- `TestExecute_GetUserNotRegistered`: error `bus.ErrNotRegistered` -> `errors.Is(err, bus.ErrNotRegistered)` benar, bukan `*errs.Error` (jadi 500).
- `TestExecute_FindAccessErrorWrapped` / `TestExecute_ListMembersErrorWrapped`: error repo terbungkus, tidak ada `Get`.
- `TestExecute_ContextCancelledStopsEnrichment`: konteks dibatalkan di tengah -> error `context.Canceled`, `Get` tidak dipanggil lagi.
- `TestExecute_ProjectIDNotUUIDPassedToStore`: `projectID="bukan-uuid"` diteruskan apa adanya ke `FindAccess` (repo palsu `found=false`) -> 404.

### Handler (`handler_test.go`, `httptest` + `chi` router dengan route `/projects/{projectID}/members`)

- Sukses -> 200, `Content-Type: application/json`, body `{"items":[...]}` dengan nama field persis `user_id, name, email, is_active, found, role, added_at, added_by, added_by_label`; `added_at` RFC3339 UTC.
- `items` kosong -> body `{"items":[]}` (bukan `null`).
- `name`/`email` `null` bila `found=false`.
- Error `base.ErrProjectNotFound` -> 404 dengan format error seragam dan `trace_id`.
- Error non-`*errs.Error` (get-user gagal) -> 500, body tidak memuat teks error internal.
- `projectID` dari `chi.URLParam` diteruskan ke usecase (spy).
- Handler tidak membaca body/query (request dengan body acak tetap 200).

### Integrasi (`listmembers_integration_test.go`, tag `integration`, `db.TestDB(t)`, bus nyata `bus.NewRegistry(log)`, handler palsu `core.auth.get-user`, router `chi` + middleware identitas uji)

Fixture: A (developer aktif, owner), B (developer aktif, member), D (developer nonaktif, member), X (member yang tidak dikenal `get-user`, `Found=false`), C (developer bukan anggota), S (super_admin). Project P dan baris `project_members` dibuat langsung via SQL.

- `TestT03_AC2_MemberCanSeeMembers`: B `GET /api/projects/{P}/members` -> 200; memuat A, B, D, X; urutan owner dulu (A), lalu nama.
- `TestListMembers_SuperAdminSeesAll`: S (bukan anggota) -> 200 dengan isi sama; penghitung panggilan `get-user` = jumlah anggota (tidak ada untuk S).
- `TestListMembers_DeactivatedAndUnknownMembers`: D -> `is_active=false, found=true`; X -> `found=false, name=null, email=null`.
- `TestT03_Security_ProjectIsolation_ListMembers` (rule 13.6): C (bukan anggota) -> 404 `NOT_FOUND` "project tidak ditemukan"; project uuid acak -> 404 dengan body identik (selain `trace_id`); `projectID="bukan-uuid"` -> 404 identik; `get-user` tidak dipanggil sama sekali pada ketiga kasus.
- `TestListMembers_Unauthenticated`: tanpa identitas/sesi -> 401 (melalui `middleware.RequireAuth`).
- `TestListMembers_GetUserUnregistered`: registry tanpa `get-user` -> 500 (fail, bukan daftar parsial); S dan B sama-sama 500 karena data wajib lengkap.
- `TestListMembers_GetUserErrorOnOneMember`: handler palsu error untuk B saja -> 500.
- `TestListMembers_ProjectWithoutMembers`: project tanpa baris anggota (dibuat S, D3) dilihat S -> 200 `{"items":[]}`.
- `TestListMembers_ReadOnly`: jumlah baris `projects`/`project_members` dan jumlah entri audit tidak berubah; handler palsu `core.audit.record` tidak dipanggil (tanpa audit).
- `TestListMembers_AfterMemberRemoved`: hapus baris B via SQL -> respons berikutnya tidak memuat B (tanpa cache); B kini mendapat 404.

`TestRoutesRegistered` (W) mencakup pasangan `GET /api/projects/{projectID}/members`.

## 8. E2E Scenarios (Playwright terhadap binary; butuh T01, T02 merge)

Positif:
1. Developer A (owner) membuka workspace project -> tab Members menampilkan A (owner) lebih dulu, lalu anggota lain urut nama; tiap baris menampilkan nama, email, peran.
2. Developer B (member) membuka tab Members -> daftar tampil penuh; aksi kelola anggota (tambah, ubah peran, keluarkan orang lain) tidak ditampilkan, hanya tombol "Keluar" untuk dirinya.
3. Super Admin membuka project yang bukan miliknya -> tab Members tampil penuh dengan penanda "bukan anggota" di header project.
4. Anggota yang dinonaktifkan tampil dengan badge "Nonaktif"; anggota yang akunnya tidak ditemukan tampil sebagai "Pengguna tidak ditemukan" (`found=false`) tanpa memecahkan halaman.
5. Setelah menambah anggota lewat dialog tambah anggota, daftar dimuat ulang dan memuat anggota baru pada posisi urut yang benar.

Negatif:
1. Developer C (bukan anggota) membuka URL `/projects/<id>` langsung -> halaman "project tidak ditemukan" (API 404 pada `members`).
2. Id project acak atau bukan uuid -> halaman "tidak ditemukan" yang sama.
3. Sesi berakhir -> API 401 -> diarahkan ke login.
4. `core.auth.get-user` gagal (simulasi dengan menghentikan penyedia pada lingkungan uji) -> tab Members menampilkan state error umum dengan tombol coba lagi (i18n `common.error`), bukan daftar parsial.
5. Project tanpa anggota (dibuat Super Admin) -> tab Members menampilkan empty state "Belum ada anggota" dan, bagi Super Admin, ajakan menunjuk owner.

## 9. Request/Response body

Request: `GET /api/projects/0192f5a0-1111-7aaa-8bbb-000000000001/members`, tanpa body, tanpa query. Cookie sesi; tidak perlu `X-Requested-With` (bukan mutasi).

200 OK:

```json
{
  "items": [
    {
      "user_id": "0192f5a0-aaaa-7aaa-8bbb-00000000000a",
      "name": "Andi",
      "email": "a@tim.id",
      "is_active": true,
      "found": true,
      "role": "owner",
      "added_at": "2026-10-22T09:00:00Z",
      "added_by": "0192f5a0-aaaa-7aaa-8bbb-00000000000a",
      "added_by_label": "a@tim.id"
    },
    {
      "user_id": "0192f5a0-bbbb-7aaa-8bbb-00000000000b",
      "name": "Budi",
      "email": "b@tim.id",
      "is_active": false,
      "found": true,
      "role": "member",
      "added_at": "2026-10-22T09:05:00Z",
      "added_by": "0192f5a0-aaaa-7aaa-8bbb-00000000000a",
      "added_by_label": "a@tim.id"
    },
    {
      "user_id": "0192f5a0-dddd-7aaa-8bbb-00000000000d",
      "name": null,
      "email": null,
      "is_active": false,
      "found": false,
      "role": "member",
      "added_at": "2026-10-22T09:10:00Z",
      "added_by": null,
      "added_by_label": null
    }
  ]
}
```

200 OK, project tanpa anggota:

```json
{ "items": [] }
```

401 (tanpa sesi):

```json
{ "error": { "code": "UNAUTHENTICATED", "message": "perlu login", "details": null }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

(kode dan pesan persis mengikuti `RequireAuth` T00; contoh di atas ilustratif.)

404 (bukan anggota, project tidak ada, atau `projectID` bukan uuid; tiga kasus identik):

```json
{ "error": { "code": "NOT_FOUND", "message": "project tidak ditemukan", "details": null }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

500 (get-user atau DB gagal; pesan generik, tanpa detail internal):

```json
{ "error": { "code": "INTERNAL", "message": "terjadi kesalahan internal", "details": null }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

(kode dan pesan 500 persis mengikuti `respond.WriteError` T00.)

Skema OpenAPI operasi (di `api/modules/project/management.yml`, `operationId: listMembers`, `tags: [project-members]`):
- parameter `$ref: '#/components/parameters/ProjectID'` (path `projectID`, `format: uuid`);
- 200: schema `MemberList` (`items: Member[]`; `Member.name` dan `Member.email` bertipe `[string, "null"]`, `added_by` dan `added_by_label` `[string, "null"]`, `role` enum `owner|member`);
- 401, 404, 500 dirujuk dari `../infra/foundation.yml#/components/responses/{Unauthorized,NotFound,Internal}`; deskripsi 404: "juga untuk project yang bukan milik pemanggil (tidak bocor)";
- tanpa parameter header `XRequestedWith` (bukan mutasi).
