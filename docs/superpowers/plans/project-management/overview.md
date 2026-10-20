# T03 project/management — Overview Plan (FOUNDATION)

Tiket: `docs/tickets/T03-project-management.md`
Sumber aturan: `rules.txt` (menang untuk arsitektur), `docs/tickets/README.md` (konvensi, katalog bus, katalog audit), `docs/superpowers/specs/2026-10-20-grimoire-design.md` (produk), `docs/superpowers/plans/foundation/overview.md` (pola infra T00), `docs/superpowers/plans/core-auth/overview.md` (+ `usecase-getuserbus.md`, `usecase-finddevelopers.md`), `docs/superpowers/plans/core-audit-log/overview.md` (+ `usecase-record.md`).
Konteks yang dicek: belum ada `CLAUDE.md` dan `.claude/rules/`. Tidak ada knowledge doc module management. Kode T00 di `main` (`45a7ad5`).

Dokumen ini adalah plan FOUNDATION untuk module `project/management`. Isinya keputusan bersama, `base/`, kontrak bus, migrasi, urutan, dan daftar usecase. Plan rinci per usecase ditulis terpisah di `docs/superpowers/plans/project-management/usecase-<slug>.md` dan WAJIB mengikuti dokumen ini. Bila plan usecase bertentangan dengan dokumen ini, dokumen ini yang menang sampai diperbarui. Frontend ditulis di `docs/superpowers/plans/project-management/frontend-plan.md` (terpisah, tidak dipecah per usecase).

---

## 0. Prasyarat dan nama infra yang dipakai

- T03 bergantung pada T01 (`core.auth.get-user`, `core.auth.find-developers`) dan T02 (`core.audit.record`). Keduanya baru sampai tahap plan. T03 memakai kontrak persis dari plan tersebut dan TIDAK mendefinisikannya ulang. Bila T03 dikerjakan sebelum T01/T02 merge, file kontrak yang dibutuhkan dibuat persis seperti di plan pemiliknya (aturan "tiket yang merge lebih dulu membuat kontrak", plan T01 5.5). Bila sudah ada di `main`, pakai yang ada.
- Signature publik T00 TIDAK diubah. T03 hanya MENAMBAH konstanta, kontrak, migrasi, module, dan OpenAPI.
- Nama package infra final di `main` (BUKAN nama WIP `httpx` yang disebut plan T01):

| Simbol | Import path | Package |
|---|---|---|
| `Deps{Ctx, Config, Log, DB, Bus, Jobs, API, MCP}` | `grimoire/internal/infra/app` | `app` |
| `Registry.Register`, `Registry.Call`, `NewRegistry`, `CallTyped[T]`, `Expect[T]`, `ErrNotRegistered`, `ErrBadRequestType` | `grimoire/internal/infra/bus` | `bus` |
| `WriteJSON(w, status, v)`, `WriteError(w, r, err)`, `DecodeJSON(r, dst)`, `StatusFor` | `grimoire/internal/infra/http/respond` | `respond` |
| `RequireAuth()`, `RequireRole(...)`, `KeyByIP(r)`, `CSRFHeader` | `grimoire/internal/infra/middleware` | `middleware` |
| `DB` (embed `*sqlx.DB`), `DB.BeginTx(ctx, func(*sqlx.Tx) error)`, `Querier`, `TestDB(t)` | `grimoire/internal/infra/db` | `db` |
| `New`, `Wrap`, `WithDetails`, `Error`, `CodeNotFound`, `CodeForbidden`, `CodeConflict`, `CodeInvalid`, `ErrInvalid` | `grimoire/lib/common/errs` | `errs` |
| `IdentityFrom(ctx)` (`Identity{UserID, Email, Name, Role}`), `TraceID(ctx)` | `grimoire/lib/common/reqctx` | `reqctx` |
| `RoleSuperAdmin`, `RoleDeveloper` | `grimoire/lib/contracts/core/auth/resolvesession` | `resolvesession` |

- Tidak ada kode error baru. Tidak ada `RegisterCodeStatus`. Semua kasus khusus memakai kode bawaan (`NOT_FOUND`, `FORBIDDEN`, `CONFLICT`, `INVALID_ARGUMENT`) dan dibedakan lewat `details.reason`.
- Tidak ada env var baru, tidak ada job, tidak ada library baru (`github.com/google/uuid`, `sqlx`, `chi` sudah di `go.mod`).

---

## 1. Metadata

| Atribut | Nilai |
|---|---|
| Ticket | T03 |
| Scope | fullstack (backend Go di plan ini; frontend di `frontend-plan.md`) |
| Domain | project |
| Module | management (`backend/internal/domain/project/internal/management`) |
| Resource root | `backend/internal/domain/project/internal/management` |
| Versi kontrak REST | Tanpa prefix versi di URL (prefix `/api`). `api/modules/project/management.yml` memakai `info.version: 0.1.0` |
| Versi kontrak bus | v1 (endpoint baru) |
| Bergantung pada | T00 (merged), T01 (plan), T02 (plan) |
| Dibutuhkan oleh | T04, T05, T06 (`check-access`, FK `projects(id)`), T07 (`get-project`) |

Endpoint HTTP (semua di `d.API`, semua `middleware.RequireAuth()`; mutasi wajib header `X-Requested-With: grimoire` dari CSRF T00):

| Endpoint | Method | Version | Siapa (otorisasi di usecase) | Usecase |
|---|---|---|---|---|
| `/api/projects` | POST | - | developer, super_admin | `createproject` |
| `/api/projects` | GET | - | developer (project yang ia ikuti), super_admin (semua) | `listprojects` |
| `/api/projects/{projectID}` | GET | - | anggota, super_admin | `getproject` |
| `/api/projects/{projectID}` | PATCH | - | anggota, super_admin | `updateproject` |
| `/api/projects/{projectID}` | DELETE | - | owner, super_admin | `deleteproject` |
| `/api/projects/{projectID}/members` | GET | - | anggota, super_admin | `listmembers` |
| `/api/projects/{projectID}/member-candidates` | GET | - | owner, super_admin | `searchmembercandidates` |
| `/api/projects/{projectID}/members` | POST | - | owner, super_admin | `addmember` |
| `/api/projects/{projectID}/members/{userID}` | DELETE | - | owner, super_admin, diri sendiri | `removemember` |
| `/api/projects/{projectID}/members/{userID}` | PATCH | - | owner, super_admin | `changememberrole` |

Endpoint bus (provider T03):

| Endpoint | Mode | Version | Consumer | Usecase |
|---|---|---|---|---|
| `project.management.check-access` | Call | v1 | T04, T05, T06 | `checkaccess` |
| `project.management.get-project` | Call | v1 | T07 | `getprojectbus` |

Endpoint bus yang dikonsumsi:

| Endpoint | Mode | Provider | Dipakai oleh |
|---|---|---|---|
| `core.auth.get-user` | Call | T01 | `checkaccess`, `addmember`, `listmembers`, `removemember` dan `changememberrole` (label/DTO setelah commit, fail-soft) |
| `core.auth.find-developers` | Call | T01 | `searchmembercandidates` |
| `core.audit.record` | Call (fail-open) | T02 | `createproject`, `updateproject`, `deleteproject`, `addmember`, `removemember`, `changememberrole` |
| `project.knowledge.count-files` | Call (fail-soft) | T05 (D1, belum ada) | `listprojects`, `getproject`, `deleteproject` |
| `project.apispec.count-specs` | Call (fail-soft) | T06 (D1, belum ada) | idem |
| `project.apikey.count-keys` | Call (fail-soft) | T04 (D1, belum ada) | idem |

---

## 2. Summary Plan

T03 membangun project sebagai unit utama beserta keanggotaan developer, dan menjadi satu-satunya sumber keputusan "siapa boleh apa di project ini". Isinya:

- Dua tabel milik module (`projects`, `project_members`) dalam satu migrasi `migrations/project/`.
- Rule murni di `base/`: validasi nama/deskripsi, patch, konfirmasi hapus, level akses (`none|member|owner|admin`) dan izin per aksi, rule keluar/turun peran (owner terakhir), kelayakan kandidat anggota.
- Empat subpackage I/O bersama di bawah `base/`: `base/store` (SQL bersama), `base/audit` (kirim audit via bus, fail-open), `base/users` (lookup user via bus), `base/counts` (jumlah file/spec/key via bus, fail-soft).
- 12 feature: 10 HTTP dan 2 handler bus (`checkaccess`, `getprojectbus`).
- Dua kontrak bus baru yang di-provide (`check-access`, `get-project`) dan satu kontrak consumer-driven netral (`contentcount`) dengan tiga konstanta endpoint yang di-provide T04-T06 nanti.
- OpenAPI `api/modules/project/management.yml`, i18n `project.*`, pembaruan README (katalog bus, konvensi param route) dan tiket T03-T06.

Hal penting soal keamanan:
- Project yang tidak boleh dilihat pemanggil selalu dijawab 404 (rule 7.3), termasuk id bukan uuid, untuk SEMUA 10 route.
- Otorisasi project hanya lewat `base.Require(level, perm)` (HTTP) dan `check-access` (bus). Module lain dilarang menyalin logikanya.
- Semua mutasi anggota dan hapus project berjalan di transaksi yang mengunci baris project (`FOR UPDATE`) dan menghitung ulang level aktor dari baris terkunci, sehingga race tidak menghasilkan project tanpa owner oleh non-admin.
- `check-access` fail-closed: bila `core.auth.get-user` gagal, yang dikembalikan error, bukan akses.

---

## 3. Keputusan bersama (mengikat semua usecase)

### 3.1 Keputusan baru lintas tiket (dicatat di PR; README dan tiket diperbarui di PR yang sama)

| # | Keputusan | Alasan | Tindakan dokumen |
|---|---|---|---|
| D1 | Hitungan isi (jumlah file/spec/key) memakai kontrak consumer-driven netral `lib/contracts/project/management/contentcount` (`Request{ProjectIDs []string}` -> `Response{Counts map[string]int}`) dengan tiga konstanta endpoint yang di-provide T05/T06/T04. Consumer T03 fail-soft: error apa pun (termasuk `bus.ErrNotRegistered` sebelum T04-T06 merge) -> hitungan `null` di JSON. Satu Call per provider per request (batch, tanpa N+1) | Tiket meminta jumlah file/spec/key di `getproject`, beranda, dan audit `project.deleted`, tetapi katalog bus tidak punya endpoint itu. T03 merge lebih dulu sehingga tidak boleh gagal hanya karena provider belum ada | Tambah 3 baris katalog bus di README; tambah kalimat "Disediakan" di T04/T05/T06 |
| D2 | Feature tambahan `searchmembercandidates` (`GET /api/projects/{projectID}/member-candidates`) | Autocomplete dialog tambah anggota butuh endpoint sendiri; rule 3.4 melarang dua endpoint dalam satu folder | Tambah baris di tabel Features `T03-project-management.md` |
| D3 | Super Admin yang membuat project TIDAK dijadikan anggota. Project lahir tanpa owner, lalu Super Admin menunjuk owner lewat `addmember` dengan `role: "owner"`. Karena itu `addmember` menerima `role` opsional (default `member`) | Konsisten dengan "Super Admin tidak ditambahkan sebagai anggota lewat fitur ini" dan "project tanpa owner diperbolehkan". Hanya owner/admin yang boleh `addmember`, jadi `role: owner` tidak membuka celah | Ubah baris `createproject` dan `addmember` di tiket |
| D4 | Route didaftarkan dengan pola penuh langsung di `d.API` (tanpa `d.API.Route("/projects", ...)`). Nama param WAJIB `{projectID}` dan `{userID}` | Mount chi pada `/projects` akan bentrok dengan route T04-T06 (`/projects/{projectID}/api-keys`, dst.). Nama param yang sama di posisi yang sama menghindari konflik node chi | Tambah catatan "Route di bawah /api/projects" di bagian Konvensi README |
| D5 | Audit dikirim dengan `bus.Call` SETELAH commit, fail-open. No-op (tidak ada perubahan) -> 0 entri | Pola T01 D2 + keputusan T02 (fail-open). Satu aksi = tepat satu entri | Ubah kalimat "Dikonsumsi" tiket bila perlu |
| D6 | Usecase HTTP T03 TIDAK memanggil `get-user` untuk aktor. `get-user` hanya untuk target: validasi (`addmember`), isi respons (`listmembers`), label audit/DTO setelah commit (`removemember`, `changememberrole`, fail-soft, lihat bagian 9), dan di `checkaccess` | Middleware Session (`resolve-session`) sudah menolak user nonaktif di setiap request; panggilan tambahan hanya menambah latensi | - |

Catatan: T01 dan T02 di plannya sama-sama memakai versi migrasi `20261021090000000`. Itu bentrok di antara mereka (dicatat di risiko); T03 memakai `20261022090000000` yang unik.

### 3.2 Struktur module

```
backend/internal/domain/project/
  base.go                         # ubah: func Register(d app.Deps) { management.Register(d) }
  internal/management/
    base.go                       # package management: Register(d app.Deps) memanggil 12 <feature>.Init(d)
    management_integration_test.go
    base/                         # package base: entity + rule, TANPA I/O, TANPA context
      project.go      project_test.go
      member.go       member_test.go
      access.go       access_test.go
      membership_rules.go  membership_rules_test.go
      errors.go       errors_test.go
      actor.go  meta.go  clock.go
      like.go         like_test.go
      dto.go          dto_test.go
    base/store/                   # package store: SQL bersama
      store.go  store_integration_test.go
    base/audit/                   # package audit: kirim audit via bus (fail-open)
      audit.go  audit_test.go
    base/users/                   # package users: lookup user via bus
      users.go  users_test.go
    base/counts/                  # package counts: jumlah isi via bus (fail-soft)
      counts.go  counts_test.go
    checkaccess/ getprojectbus/ createproject/ listprojects/ getproject/ updateproject/
    deleteproject/ listmembers/ searchmembercandidates/ addmember/ removemember/ changememberrole/
      init.go handler.go [params.go] usecase.go [repository.go] *_test.go
```

Aturan per feature:
- `init.go`: `func Init(d app.Deps)`. Merakit `store.New(d.DB)` -> repository feature -> usecase (menerima interface) -> handler, lalu mendaftarkan route atau handler bus feature itu sendiri.
- Feature bus (`checkaccess`, `getprojectbus`) tanpa `params.go`. `handler.go` memanggil `bus.Expect[<kontrak>.Request](req)` (rule 6.9).
- Usecase menerima interface: repository feature, `audit.Recorder`, `users.Lookup`, `counts.Counter`, `base.Clock`. Bukan tipe konkret (rule 5.6).
- Feature DILARANG saling import (rule 4.3). Yang dipakai bersama lewat `base`, `base/store`, `base/audit`, `base/users`, `base/counts`. Package `base` DILARANG meng-import subpackage-nya sendiri (I/O); arah import: `base/store|audit|users|counts` -> `base`, bukan sebaliknya.
- `base/store` dan `base/*` boleh di-import feature karena archtest T00 memperlakukan semua path di bawah `<module>/base/` sebagai milik module (pola T01 D3).
- `BeginTx` dipanggil dari repository feature (`func (r *Repo) InTx(ctx, fn func(*Repo) error) error`), bukan dari usecase. Repository feature membungkus `*store.Store` (`r.s.WithTx(tx)` di dalam tx).

### 3.3 Konvensi handler, actor, request meta, dan log

- Handler membaca `chi.URLParam(r, "projectID")` / `chi.URLParam(r, "userID")` apa adanya dan meneruskannya ke usecase. Validasi uuid TIDAK dilakukan di handler; store mengembalikan "tidak ketemu" untuk id bukan uuid tanpa query, sehingga usecase menjawab 404 yang sama.
- Handler membangun `base.RequestMeta{IP: middleware.KeyByIP(r), UserAgent: truncate(r.UserAgent(), 512)}`.
- Usecase membangun aktor sekali: `actor, ok := base.ActorFrom(reqctx.IdentityFrom(ctx))`. `!ok` (seharusnya mustahil karena `RequireAuth`) -> `errs.New(errs.CodeUnauthorized, "perlu login")`. `Actor{ID: Identity.UserID, Role: Identity.Role, Label: Identity.Email}`. `base.ActorFrom` menerima `reqctx.Identity` dan nilai `ok` (fungsi murni; reqctx hanya paket tipe).
- Body JSON di-decode dengan `respond.DecodeJSON` (field tak dikenal ditolak 400). Validasi bentuk di `params.go` (tipe, wajib ada); validasi isi (panjang nama, role valid) di entity.
- Error infra dibungkus `fmt.Errorf("<feature>: <langkah>: %w", err)` dan sampai ke klien sebagai 500 generik. Error bisnis selalu `*errs.Error` dari `base/errors.go`.
- Respons sukses memakai `respond.WriteJSON`. Respons gagal memakai `respond.WriteError`.
- Log: tidak pernah me-log `Args` audit atau isi body. Field log standar: `project_id`, `user_id`, `feature`.

### 3.4 Otorisasi project di usecase HTTP (dipakai 9 route ber-`{projectID}`)

Urutan wajib:
1. `p, memberRole, found, err := store.FindAccess(ctx, projectID, actor.ID)`.
2. `!found` -> `base.ErrProjectNotFound` (404).
3. `level := base.ResolveLevel(actor.Role, memberRole, true)` (`userActive=true`, D6).
4. `if err := base.Require(level, base.PermX); err != nil { return err }`. `none` -> 404 (tidak bocor), izin kurang -> 403.
5. Lanjut ke langkah feature.

Peta izin (`access.go`):

| Permission | none | member | owner | admin |
|---|---|---|---|---|
| `PermView` | 404 | ok | ok | ok |
| `PermEdit` | 404 | ok | ok | ok |
| `PermManageMembers` | 404 | 403 | ok | ok |
| `PermDelete` | 404 | 403 | ok | ok |

Self-leave (`removemember` dengan target == aktor) tidak lewat `PermManageMembers`; ditangani `base.CheckRemove` (3.6).

### 3.5 Konkurensi

- `createproject`: satu tx: `INSERT projects ... RETURNING` lalu, bila aktor developer, `INSERT project_members (role 'owner', added_by = aktor)`.
- `updateproject`: `FindAccess` + `Require(PermEdit)` di luar tx, lalu tx: `LockProject` (FOR UPDATE; hilang -> 404) -> `Project.ApplyUpdate` -> `UPDATE ... updated_at = now()` hanya bila `len(changed) > 0`.
- Mutasi anggota (`addmember`, `removemember`, `changememberrole`) dan `deleteproject`: pre-check (3.4) di luar tx untuk menjawab cepat, lalu tx:
  1. `LockProject(ctx, id)` (FOR UPDATE). Hilang -> 404. Ini mutex per project untuk semua mutasi anggota dan hapus.
  2. `LockMembers(ctx, id)` (FOR UPDATE, `ORDER BY user_id`).
  3. Hitung ulang level aktor dari baris terkunci: `memberRole` = role aktor di daftar terkunci ("" bila tidak ada), `level = ResolveLevel(actor.Role, memberRole, true)`, lalu `Require` lagi. `ownerCount` dihitung di Go dari daftar terkunci (FOR UPDATE tidak bisa digabung agregat).
  4. Panggil rule entity (`CheckRemove` / `CheckRoleChange` / cek `ErrAlreadyMember`).
  5. Tulis.
- Dengan READ COMMITTED dan lock pada baris `projects`, dua owner yang saling menurunkan diserialkan: tx kedua membaca ulang daftar anggota, mendapati dirinya sudah `member`, dan mendapat 403. Tepat satu sukses (AC3).
- `addmember`: `INSERT ... ON CONFLICT (project_id, user_id) DO NOTHING`; 0 baris -> `ErrAlreadyMember` (409).
- Panggilan bus (get-user, counts, audit) TIDAK PERNAH dilakukan di dalam tx. `get-user` untuk validasi target (`addmember`) dan counts (`deleteproject`) dipanggil SEBELUM tx; `get-user` untuk label/DTO (`removemember`, `changememberrole`) dan audit dipanggil SETELAH commit.

### 3.6 Rule keanggotaan (`membership_rules.go`)

`CheckRoleChange(actorID string, actorLevel Level, target Member, newRole string, ownerCount int) error`, dicek berurutan:
1. `!ValidMemberRole(newRole)` -> `InvalidFields{"role": "role harus owner atau member"}`.
2. `newRole == target.Role` -> `nil` (no-op; usecase tidak menulis dan tidak mengaudit).
3. `actorLevel` bukan `owner`/`admin` -> `ErrForbidden`.
4. `target.Role == owner && newRole == member && ownerCount <= 1 && actorLevel != admin` -> `ErrLastOwner`.
5. `nil`.

`CheckRemove(actorID string, actorLevel Level, target Member, ownerCount int) error`, dicek berurutan:
1. `self := target.UserID == actorID`.
2. `!self && actorLevel` bukan `owner`/`admin` -> `ErrForbidden` (member hanya boleh keluar sendiri).
3. `target.Role == owner && ownerCount <= 1 && actorLevel != admin` -> `ErrLastOwner`. (Owner yang mengeluarkan owner lain hanya mungkin bila `ownerCount >= 2`, jadi kasus ini praktis berarti owner terakhir keluar sendiri.)
4. `nil`.

Admin selalu boleh (project boleh tanpa owner). Super Admin bukan anggota, jadi tidak pernah menjadi target "self".

### 3.7 Validasi nama dan deskripsi

- Nama: `strings.TrimSpace`; `utf8.RuneCountInString` 2..100; tidak boleh mengandung karakter kontrol (`unicode.IsControl`, termasuk `\n`, `\t`). Disimpan hasil trim. Tidak harus unik.
- Deskripsi: `strings.TrimSpace`; 0..1000 rune; karakter kontrol dilarang kecuali `\n`, `\r`, `\t`. Default `""`.
- Pesan field: `name` -> `"nama project wajib 2-100 karakter"` / `"nama project tidak boleh mengandung karakter kontrol"`; `description` -> `"deskripsi maksimal 1000 karakter"` / `"deskripsi mengandung karakter tidak valid"`.
- CHECK di DB (`char_length`) adalah jaring pengaman, bukan sumber pesan.

### 3.8 Paginasi, pencarian, urutan

- `listprojects`: `limit` default 20, 1..100 (di luar itu 400 `fields.limit`); `offset` default 0, >= 0 (`fields.offset`); bukan integer -> 400. `q` di-trim, maks 100 rune (`fields.q`); kosong = tanpa filter. Filter `p.name ILIKE $n ESCAPE '\'` dengan `base.LikePattern(q)`. Urutan `updated_at DESC, id DESC`. `total` dari `COUNT(*) OVER()`. Bila hasil kosong dan `offset > 0`, repository menjalankan query `COUNT(*)` terpisah dengan filter yang sama agar `total` tetap benar.
- `listmembers`: tanpa paginasi. Urutan di Go setelah enrichment: role `owner` dulu, lalu `strings.ToLower(name)` (anggota tanpa nama karena `Found=false` di akhir grupnya), lalu `user_id`.
- `searchmembercandidates`: `q` di-trim; kosong -> `items: []` tanpa bus; maks 100 rune. `limit` default 10, 1..50 (di luar 400). Panggil `find-developers` dengan `Limit: finddevelopers.MaxLimit` (50), buang yang sudah anggota, potong ke `limit`.
- Tanpa indeks untuk ILIKE nama (skala puluhan-ratusan project, pola T01 D6).

### 3.9 Hitungan isi (D1, `base/counts`)

- `Counter.ForProjects(ctx, ids []string) map[string]base.ContentCounts`. Dedupe `ids`; kosong -> map kosong tanpa bus.
- Untuk tiap endpoint (`BUS_PROJECT_KNOWLEDGE_COUNT_FILES`, `BUS_PROJECT_APISPEC_COUNT_SPECS`, `BUS_PROJECT_APIKEY_COUNT_KEYS`): satu `bus.CallTyped[contentcount.Response](ctx, b, ep, contentcount.Request{ProjectIDs: ids})`.
  - Sukses: tiap id mendapat pointer ke `resp.Counts[id]` (id tanpa entri = 0).
  - `errors.Is(err, bus.ErrNotRegistered)` -> `log.DebugContext(ctx, "content count belum tersedia", "endpoint", ep)`; field tetap `nil`.
  - Error lain -> `log.WarnContext(ctx, "content count gagal", "endpoint", ep, "error", err)`; field tetap `nil`.
- Tidak pernah mengembalikan error. Tiga Call berurutan (sederhana; tiap Call satu query provider).
- Consumer tidak pernah mengirim lebih dari `contentcount.MaxProjectIDs` (100) id; `listprojects` sudah dibatasi `limit <= 100`.

---

## 4. Entity dan rule (`base/`, package `base`)

Semua fungsi murni: tanpa context, tanpa I/O. Diuji unit tanpa mock (rule 13.1). Import yang diizinkan: stdlib, `lib/common/errs`, `lib/common/reqctx` (tipe `Identity`), `lib/contracts/core/auth/{getuser,resolvesession}`, `lib/contracts/project/management/checkaccess`.

### 4.1 `base/project.go`

```go
const (
    NameMinRunes        = 2
    NameMaxRunes        = 100
    DescriptionMaxRunes = 1000
)

type Project struct {
    ID             string
    Name           string
    Description    string
    CreatedBy      *string
    CreatedByLabel *string
    CreatedAt      time.Time
    UpdatedAt      time.Time
}

func ValidateName(s string) (string, FieldErrors)        // 3.7; nil bila valid
func ValidateDescription(s string) (string, FieldErrors) // 3.7
// NewProject memvalidasi keduanya (FieldErrors digabung) dan mengisi CreatedBy/CreatedByLabel dari actor
// (ID dan Label), CreatedAt=UpdatedAt=now.UTC(). ID diisi DB.
func NewProject(name, description string, actor Actor, now time.Time) (Project, error)

type ProjectPatch struct{ Name, Description *string }
func (p ProjectPatch) Empty() bool // keduanya nil
// ApplyUpdate: Empty -> InvalidFields{"body": "minimal satu field (name, description) wajib diisi"}.
// Validasi tiap field yang diisi; nilai (setelah trim) sama dengan sekarang tidak dihitung berubah.
// changed berurutan ["name","description"]. Error validasi digabung -> InvalidFields.
func (p *Project) ApplyUpdate(patch ProjectPatch, now time.Time) (changed []string, err error) // set UpdatedAt=now bila changed tidak kosong
// ConfirmDeletion: confirm == p.Name persis (tanpa trim, case-sensitive) -> nil; selain itu ErrConfirmationMismatch.
func (p Project) ConfirmDeletion(confirm string) error
func (p Project) ToDTO() ProjectDTO
```

Untuk `ApplyUpdate`, kunci field error saat patch kosong adalah `"body"` dengan pesan `"minimal satu field (name, description) wajib diisi"`.

### 4.2 `base/member.go`

```go
const (
    RoleOwner  = "owner"
    RoleMember = "member"
)

type Member struct {
    ProjectID    string
    UserID       string
    Role         string
    AddedBy      *string
    AddedByLabel *string
    AddedAt      time.Time
}

func ValidMemberRole(r string) bool // owner | member
// NewMember: role "" -> member; role tidak valid -> InvalidFields{"role": "role harus owner atau member"}.
func NewMember(projectID, userID, role string, actor Actor, now time.Time) (Member, error)
// ValidateCandidate (AC4), dicek berurutan:
//   !u.Found                                 -> UserNotEligible(ReasonUserNotFound)
//   u.Role != resolvesession.RoleDeveloper   -> UserNotEligible(ReasonUserNotDeveloper)  (termasuk super_admin)
//   !u.IsActive                              -> UserNotEligible(ReasonUserInactive)
func ValidateCandidate(u getuser.Response) error
func OwnerCount(ms []Member) int
func FindMember(ms []Member, userID string) (Member, bool)
// ToDTO menggabungkan baris anggota dengan hasil get-user. u.Found=false -> name/email null, found=false, is_active=false.
func (m Member) ToDTO(u getuser.Response) MemberDTO
```

### 4.3 `base/access.go`

```go
type Level = checkaccess.Level
const (
    LevelNone   = checkaccess.LevelNone
    LevelMember = checkaccess.LevelMember
    LevelOwner  = checkaccess.LevelOwner
    LevelAdmin  = checkaccess.LevelAdmin
)

// ResolveLevel:
//   userRole == super_admin                      -> admin (tanpa melihat keanggotaan)
//   userRole != developer                        -> none
//   membershipRole == "" || !userActive          -> none
//   membershipRole == owner                      -> owner
//   membershipRole == member                     -> member
//   lainnya                                      -> none
func ResolveLevel(userRole, membershipRole string, userActive bool) Level

type Permission int
const (
    PermView Permission = iota + 1
    PermEdit
    PermManageMembers
    PermDelete
)

// Require mengikuti tabel 3.4: none -> ErrProjectNotFound; member untuk ManageMembers/Delete -> ErrForbidden.
// Level tidak dikenal diperlakukan none. Karena Level adalah alias tipe kontrak, Require ditulis sebagai
// fungsi: func Require(l Level, p Permission) error. (Method tidak bisa ditambahkan ke tipe package lain.)
func Require(l Level, p Permission) error
func CanManageMembers(l Level) bool // owner | admin
```

Catatan implementasi: `Level` adalah alias ke tipe kontrak, sehingga method tidak dapat ditambahkan. Plan usecase memanggil `base.Require(level, base.PermView)`.

### 4.4 `base/membership_rules.go`

Persis seperti 3.6:

```go
const ReasonLastOwner = "LAST_OWNER"
func CheckRoleChange(actorID string, actorLevel Level, target Member, newRole string, ownerCount int) error
func CheckRemove(actorID string, actorLevel Level, target Member, ownerCount int) error
```

### 4.5 `base/errors.go`

```go
const (
    ReasonAlreadyMember        = "ALREADY_MEMBER"
    ReasonConfirmationMismatch = "CONFIRMATION_MISMATCH"
    ReasonUserNotFound         = "USER_NOT_FOUND"
    ReasonUserNotDeveloper     = "USER_NOT_DEVELOPER"
    ReasonUserInactive         = "USER_INACTIVE"
)

var (
    ErrProjectNotFound      = errs.New(errs.CodeNotFound, "project tidak ditemukan")
    ErrMemberNotFound       = errs.New(errs.CodeNotFound, "anggota tidak ditemukan")
    ErrForbidden            = errs.New(errs.CodeForbidden, "akses ditolak")
    ErrLastOwner            = errs.WithDetails(errs.CodeConflict,
        "owner terakhir tidak bisa keluar atau diturunkan; angkat owner lain lebih dulu",
        map[string]any{"reason": ReasonLastOwner})
    ErrAlreadyMember        = errs.WithDetails(errs.CodeConflict, "user sudah menjadi anggota",
        map[string]any{"reason": ReasonAlreadyMember})
    ErrConfirmationMismatch = errs.WithDetails(errs.CodeInvalid, "nama konfirmasi tidak cocok",
        map[string]any{"reason": ReasonConfirmationMismatch})
)

// UserNotEligible -> 400 INVALID_ARGUMENT, details {"reason": reason}. Pesan:
//   USER_NOT_FOUND      "user tidak ditemukan"
//   USER_NOT_DEVELOPER  "hanya developer yang bisa ditambahkan sebagai anggota; Super Admin tidak perlu menjadi anggota"
//   USER_INACTIVE       "user nonaktif tidak bisa ditambahkan sebagai anggota"
func UserNotEligible(reason string) *errs.Error

type FieldErrors map[string]string // implements error; Error() menggabungkan key terurut
func (fe FieldErrors) Merge(other FieldErrors) FieldErrors
// InvalidFields -> errs.WithDetails(CodeInvalid, "permintaan tidak valid", map[string]any{"fields": fe})
func InvalidFields(fe FieldErrors) *errs.Error
```

Sentinel `*errs.Error` dipakai bersama (pointer yang sama). Usecase TIDAK boleh memodifikasinya. Test memakai `errors.Is` terhadap pointer itu atau memeriksa `Code` + `details.reason`.

### 4.6 `base/actor.go`, `base/meta.go`, `base/clock.go`, `base/like.go`

```go
type Actor struct{ ID, Role, Label string } // Label = email aktor (snapshot audit dan *_by_label)
func ActorFrom(id reqctx.Identity, ok bool) (Actor, bool) // ok=false atau UserID kosong -> false
func (a Actor) IsSuperAdmin() bool

type RequestMeta struct{ IP, UserAgent string }
type Clock func() time.Time   // produksi: func() time.Time { return time.Now().UTC() }
func LikePattern(q string) string // escape \ -> \\, % -> \%, _ -> \_, lalu "%"+q+"%" (salinan pola T01; module tidak saling import)
```

### 4.7 `base/dto.go`

```go
type ProjectDTO struct {
    ID             string    `json:"id"`
    Name           string    `json:"name"`
    Description    string    `json:"description"`
    CreatedBy      *string   `json:"created_by"`
    CreatedByLabel *string   `json:"created_by_label"`
    CreatedAt      time.Time `json:"created_at"`
    UpdatedAt      time.Time `json:"updated_at"`
}

type ContentCounts struct {
    Files *int `json:"files"`
    Specs *int `json:"specs"`
    Keys  *int `json:"keys"`
}

type ProjectSummary struct {
    ProjectDTO
    MyRole      *string       `json:"my_role"`      // "owner" | "member" | null
    AccessLevel Level         `json:"access_level"` // "owner" | "member" | "admin"
    IsMember    bool          `json:"is_member"`
    MemberCount int           `json:"member_count"`
    Counts      ContentCounts `json:"counts"`
}

type ProjectDetail struct {
    ProjectSummary
    OwnerCount int `json:"owner_count"`
}

type MemberDTO struct {
    UserID       string    `json:"user_id"`
    Name         *string   `json:"name"`
    Email        *string   `json:"email"`
    IsActive     bool      `json:"is_active"`
    Found        bool      `json:"found"`
    Role         string    `json:"role"`
    AddedAt      time.Time `json:"added_at"`
    AddedBy      *string   `json:"added_by"`
    AddedByLabel *string   `json:"added_by_label"`
}

type CandidateDTO struct{ ID, Name, Email string } // json: id, name, email

// NewSummary: myRole "" -> MyRole nil, IsMember false.
func NewSummary(p Project, level Level, myRole string, memberCount int, c ContentCounts) ProjectSummary
func SortMembers(ms []MemberDTO) // 3.8
```

Waktu di JSON selalu UTC RFC3339 (`time.Time` dari DB di-`.UTC()` di store).

---

## 5. Kontrak bus dan konstanta endpoint

### 5.1 `backend/lib/constants/bus.go` (ditambah)

```go
// Provider: T03 project/management. Consumer: T04, T05, T06 (check-access), T07 (get-project).
const BUS_PROJECT_MANAGEMENT_CHECK_ACCESS = "project.management.check-access"
const BUS_PROJECT_MANAGEMENT_GET_PROJECT  = "project.management.get-project"

// Kontrak: lib/contracts/project/management/contentcount. Consumer: T03 (fail-soft).
// Provider: T05 (files), T06 (specs), T04 (keys aktif). Sebelum provider merge, Call -> ErrNotRegistered.
const BUS_PROJECT_KNOWLEDGE_COUNT_FILES = "project.knowledge.count-files"
const BUS_PROJECT_APISPEC_COUNT_SPECS   = "project.apispec.count-specs"
const BUS_PROJECT_APIKEY_COUNT_KEYS     = "project.apikey.count-keys"
```

Dikonsumsi (sudah/akan dibuat T01/T02, JANGAN diduplikasi): `BUS_CORE_AUTH_GET_USER = "core.auth.get-user"`, `BUS_CORE_AUTH_FIND_DEVELOPERS = "core.auth.find-developers"`, `BUS_CORE_AUDIT_RECORD = "core.audit.record"`. Bila T03 merge sebelum pemiliknya, baris itu ditambahkan persis seperti di plan T01 5.1 dan T02 F2.

### 5.2 `lib/contracts/project/management/checkaccess/checkaccess.go` (baru)

```go
// Package checkaccess adalah kontrak bus project.management.check-access (provider T03).
//
// Satu-satunya tempat keputusan akses project (rule 7.2). Module lain DILARANG menyalin logika keanggotaan.
//
// Pola consumer (T04-T06):
//
//	res, err := bus.CallTyped[checkaccess.Response](ctx, b, constants.BUS_PROJECT_MANAGEMENT_CHECK_ACCESS,
//		checkaccess.Request{ProjectID: pid, UserID: id.UserID, UserRole: id.Role})
//	if err != nil { return err }                              // 500; fail-closed
//	if !res.ProjectExists || res.Level == checkaccess.LevelNone {
//		return errs.New(errs.CodeNotFound, "project tidak ditemukan") // 404, rule 7.3 (bukan 403)
//	}
//	// level cukup -> lanjut. Aksi yang butuh owner/admin: Level member -> 403 FORBIDDEN.
//
// Request dikirim sebagai nilai (bukan pointer).
package checkaccess

type Level string

const (
    LevelNone   Level = "none"
    LevelMember Level = "member"
    LevelOwner  Level = "owner"
    LevelAdmin  Level = "admin" // UserRole == super_admin
)

type Request struct {
    ProjectID string
    UserID    string
    UserRole  string // super_admin | developer
}

type Response struct {
    ProjectExists bool
    Level         Level
}
```

Perilaku provider (diimplementasi `checkaccess`):

| Kondisi | Hasil |
|---|---|
| `req` bukan `checkaccess.Request` | error membungkus `bus.ErrBadRequestType` |
| `UserID` kosong/bukan uuid, atau `UserRole` bukan `super_admin`/`developer` | `errs.Wrap(errs.CodeInvalid, "<field> tidak valid", errs.ErrInvalid)` |
| `ProjectID` bukan uuid atau tidak ada | `{ProjectExists: false, Level: none}`, err nil (AC8) |
| `super_admin` | `{true, admin}` tanpa get-user |
| developer bukan anggota | `{true, none}` tanpa get-user |
| developer anggota, get-user `!Found` atau `!IsActive` | `{true, none}` (AC5) |
| developer anggota aktif | `{true, owner|member}` sesuai role keanggotaan |
| error get-user (termasuk `ErrNotRegistered`) atau DB | error terbungkus (fail-closed) |

Satu query: `SELECT p.id, ..., m.role FROM projects p LEFT JOIN project_members m ON m.project_id = p.id AND m.user_id = $2 WHERE p.id = $1` (`store.FindAccess`). Tanpa audit. Tidak ada cache.

### 5.3 `lib/contracts/project/management/getproject/getproject.go` (baru)

```go
// Package getproject adalah kontrak bus project.management.get-project (provider T03, consumer T07).
// ProjectID bukan uuid atau tidak ada -> Found=false, err nil. Tipe request salah -> ErrBadRequestType.
package getproject

type Request struct{ ProjectID string }
type Response struct {
    Found bool
    ID    string
    Name  string
}
```

### 5.4 `lib/contracts/project/management/contentcount/contentcount.go` (baru, D1)

```go
// Package contentcount adalah kontrak consumer-driven untuk hitungan isi per project.
// Consumer: T03 (listprojects, getproject, deleteproject), fail-soft.
// Provider (satu endpoint per module, kontrak sama):
//   constants.BUS_PROJECT_KNOWLEDGE_COUNT_FILES (T05): jumlah file knowledge
//   constants.BUS_PROJECT_APISPEC_COUNT_SPECS   (T06): jumlah API spec
//   constants.BUS_PROJECT_APIKEY_COUNT_KEYS     (T04): jumlah key AKTIF (belum dicabut)
// Provider WAJIB: bus.Expect[Request]; hanya menghitung baris dengan project_id di ProjectIDs (rule 6.10);
// id bukan uuid diabaikan; id tanpa baris boleh tidak ada di Counts (consumer membaca 0);
// len(ProjectIDs) > MaxProjectIDs -> error membungkus errs.ErrInvalid; satu query GROUP BY.
package contentcount

const MaxProjectIDs = 100

type Request struct{ ProjectIDs []string }
type Response struct{ Counts map[string]int }
```

### 5.5 Kontrak yang dikonsumsi (persis dari plan T01/T02)

- `lib/contracts/core/auth/getuser`: `Request{UserID string}` -> `Response{Found bool; ID, Name, Email, Role string; IsActive bool}`. Bukan uuid/tidak ada -> `Found=false`, err nil. Nonaktif -> `Found=true, IsActive=false`.
- `lib/contracts/core/auth/finddevelopers`: `DefaultLimit = 10`, `MaxLimit = 50`, `Request{Query string; Limit int}` -> `Response{Items []Developer}`, `Developer{ID, Name, Email string}`. Hanya developer aktif; `Query` kosong -> `[]`; urut `name, id`.
- `lib/contracts/core/audit/record`: `Request{ID, OccurredAt, ActorType, ActorID, ActorLabel, ProjectID, ProjectLabel, Action, TargetType, Target, Args, Result, Status, IP, UserAgent, TraceID}` -> `Response{Recorded bool}`. Konstanta `ActorUser`, `StatusOK`, dan dari `actions.go`: `ActionProjectCreated`, `ActionProjectUpdated`, `ActionProjectDeleted`, `ActionProjectMemberAdded`, `ActionProjectMemberRemoved`, `ActionProjectMemberRoleChanged`.

### 5.6 Subpackage I/O bersama

`base/store/store.go` (package `store`):

```go
type Store struct{ q db.Querier }
func New(q db.Querier) *Store
func (s *Store) WithTx(tx *sqlx.Tx) *Store
// id bukan uuid (uuid.Parse gagal) -> (zero, false, nil) tanpa query. Berlaku untuk semua fungsi ber-id.
func (s *Store) FindProject(ctx context.Context, id string) (base.Project, bool, error)
func (s *Store) FindAccess(ctx context.Context, projectID, userID string) (p base.Project, memberRole string, found bool, err error)
//   userID bukan uuid -> tetap cari project; memberRole "" (LEFT JOIN dengan NULL)
func (s *Store) LockProject(ctx context.Context, id string) (base.Project, bool, error) // FOR UPDATE; wajib di tx
func (s *Store) ListMembers(ctx context.Context, projectID string) ([]base.Member, error) // ORDER BY user_id
func (s *Store) LockMembers(ctx context.Context, projectID string) ([]base.Member, error) // FOR UPDATE; wajib di tx
```

Row struct privat dengan tag `db`; entity `base` tanpa tag `db`. Waktu di-`.UTC()`.

`base/audit/audit.go` (package `audit`):

```go
type Recorder interface{ Record(ctx context.Context, ev Event) }
type Event struct {
    Action       string           // record.ActionProject*
    ProjectID    string
    ProjectLabel string           // snapshot nama project
    TargetType   string           // "project" | "project_member"
    Target       string           // project id atau user id target
    Args         map[string]any   // TIDAK pernah memuat data sensitif
    Meta         base.RequestMeta
}
type BusRecorder struct{ b *bus.Registry; log *slog.Logger }
func New(b *bus.Registry, log *slog.Logger) *BusRecorder
// Record: identitas dari reqctx.IdentityFrom(ctx) -> ActorType record.ActorUser, ActorID UserID,
//   ActorLabel Email; Status record.StatusOK; TraceID reqctx.TraceID(ctx); IP/UserAgent dari Meta.
//   Identitas tidak ada -> log error "audit record gagal: identitas kosong", tidak mengirim.
//   bus.CallTyped[record.Response](ctx, b, constants.BUS_CORE_AUDIT_RECORD, req)
//   error -> log.ErrorContext(ctx, "audit record gagal", "action", ev.Action, "project_id", ev.ProjectID, "error", err)
//   TIDAK mengembalikan error (fail-open, D5).
```

`base/users/users.go` (package `users`):

```go
type Lookup interface {
    Get(ctx context.Context, userID string) (getuser.Response, error)
    FindDevelopers(ctx context.Context, q string, limit int) ([]finddevelopers.Developer, error)
}
func New(b *bus.Registry) *BusLookup // bus.CallTyped[getuser.Response] / [finddevelopers.Response]; error dibungkus "users: get-user: %w"
```

`base/counts/counts.go` (package `counts`):

```go
type Counter interface {
    ForProjects(ctx context.Context, ids []string) map[string]base.ContentCounts
}
func New(b *bus.Registry, log *slog.Logger) *BusCounter // perilaku 3.9
```

### 5.7 Katalog README (ditambah di langkah F)

```
| `project.knowledge.count-files` | Call | T05 | T03 |
| `project.apispec.count-specs` | Call | T06 | T03 |
| `project.apikey.count-keys` | Call | T04 | T03 |
```

Baris `project.management.check-access` dan `project.management.get-project` sudah ada; tambah catatan: "kontrak di `lib/contracts/project/management/{checkaccess,getproject}`; pola consumer di doc comment `checkaccess`". Baris `core.audit.record`: T03 memakai Call.

Tambah di bagian Konvensi README, subbagian Backend:

> Route di bawah `/api/projects` didaftarkan dengan pola penuh langsung di `d.API` (tanpa `Route`/`Mount` pada `/projects`). Nama param WAJIB `{projectID}` (dan `{userID}` untuk user). Contoh T04: `d.API.With(middleware.RequireAuth()).Get("/projects/{projectID}/api-keys", h)`.

---

## 6. Migration

Satu file: `backend/migrations/project/20261022090000000_create_projects.sql`

Sebelum commit, pastikan tidak ada versi 17 digit yang sama di `backend/migrations/**`. Bila bentrok, geser menit ke atas.

```sql
-- gen_random_uuid() bawaan Postgres >= 13 (dev/CI postgres:16); tanpa pgcrypto.

CREATE TABLE projects (
    id               uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    name             text        NOT NULL,
    description      text        NOT NULL DEFAULT '',
    created_by       uuid        NULL,
    created_by_label text        NULL,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT projects_name_len CHECK (char_length(name) BETWEEN 2 AND 100),
    CONSTRAINT projects_description_len CHECK (char_length(description) <= 1000)
);
CREATE INDEX projects_updated_idx ON projects (updated_at DESC, id DESC);

CREATE TABLE project_members (
    project_id     uuid        NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    user_id        uuid        NOT NULL,  -- tanpa FK ke users (rule 8.5); konsistensi lewat bus
    role           text        NOT NULL,
    added_by       uuid        NULL,
    added_by_label text        NULL,
    added_at       timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT project_members_pkey PRIMARY KEY (project_id, user_id),
    CONSTRAINT project_members_role_check CHECK (role IN ('owner','member'))
);
CREATE INDEX project_members_user_idx ON project_members (user_id);
```

Catatan:
- Forward-only, tanpa `CONCURRENTLY` (runner T00 membungkus tiap file dalam transaksi).
- `created_by`/`added_by` tanpa FK (module lain); label snapshot disimpan (rule 8.7).
- PK `(project_id, user_id)` melayani lookup keanggotaan dan `ListMembers`; `project_members_user_idx` melayani `listprojects` developer (`JOIN ... m.user_id = $1`).
- `migrations/project/.keep` boleh tetap (embed `all:project`).
- Tabel T04-T06 WAJIB `project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE`; test pg_constraint T03 (AC6) akan gagal bila ada FK ke `projects(id)` tanpa cascade.

---

## 7. Plan (langkah berurutan)

```
F  foundation module (migrasi, konstanta, kontrak, base/ + subpackage + test, OpenAPI, i18n, dokumen)
 ├── gelombang A (paralel): U1 checkaccess, U2 getprojectbus, U3 createproject
 ├── gelombang B (paralel): U4 listprojects, U5 getproject, U6 updateproject, U7 deleteproject, U8 listmembers
 └── gelombang C (paralel): U9 addmember, U10 searchmembercandidates, U11 removemember, U12 changememberrole
W  wiring final: management/base.go (12 Init), project/base.go, test integrasi lintas feature, TestRoutesRegistered
```

Feature hanya bergantung pada `base`, jadi gelombang B dan C secara kode bisa paralel setelah F. Urutan gelombang dibuat agar bus untuk T04-T07 dan pembuatan project (data uji untuk gelombang berikutnya) siap lebih dulu. Tiap usecase menambah satu baris `<feature>.Init(d)` di `management/base.go` di PR-nya sendiri.

### F — foundation module

1. **Migrasi**: buat `backend/migrations/project/20261022090000000_create_projects.sql` (bagian 6). Jalankan `make migrate-up` lokal (atau `go test ./internal/infra/db/...` dengan TestDB) untuk memastikan file valid.
2. **Konstanta**: tambah 5 konstanta di `backend/lib/constants/bus.go` (5.1). Bila T01/T02 belum merge, tambah juga `BUS_CORE_AUTH_GET_USER`, `BUS_CORE_AUTH_FIND_DEVELOPERS`, `BUS_CORE_AUDIT_RECORD` persis seperti plan pemiliknya. `lib/constants/bus_test.go` (bila ada) tetap lulus.
3. **Kontrak** (stdlib saja, rule 4.5):
   - `backend/lib/contracts/project/management/checkaccess/checkaccess.go` (5.2);
   - `backend/lib/contracts/project/management/getproject/getproject.go` (5.3);
   - `backend/lib/contracts/project/management/contentcount/contentcount.go` (5.4);
   - bila belum ada di `main`: `lib/contracts/core/auth/getuser/getuser.go`, `lib/contracts/core/auth/finddevelopers/finddevelopers.go`, `lib/contracts/core/audit/record/{record.go,actions.go}` persis plan T01 5.2-5.3 dan T02 F3.
4. **base/** (package `base`): semua file bagian 4 beserta unit test.
5. **base/store**: bagian 5.6 + `store_integration_test.go` (tag `integration`, `db.TestDB(t)`).
6. **base/audit**, **base/users**, **base/counts**: bagian 5.6 + test dengan bus nyata (`bus.NewRegistry(log)`) dan handler palsu.
7. **Kerangka wiring**:
   - `backend/internal/domain/project/internal/management/base.go`: `package management; func Register(d app.Deps) { /* <feature>.Init(d) ditambah tiap usecase */ }`;
   - `backend/internal/domain/project/base.go`: `func Register(d app.Deps) { management.Register(d) }` (hapus komentar no-op).
8. **OpenAPI** (SEBELUM kode handler, rule 11.6): `api/modules/project/management.yml` dengan 5 path (10 operasi) dan schema bagian 12; parameter `XRequestedWith`, `ProjectID`, `UserID` didefinisikan di `components.parameters` file itu (pola `auth.yml`); `Error` dan responses dirujuk dari `../infra/foundation.yml`. Tambah `$ref` path dan schema ke `api/root.yml`, jalankan `make api-bundle`, commit `api/openapi.yml`, pastikan `make api-check` lulus.
9. **i18n**: tambah kunci `project.*` di `frontend/src/shared/i18n/id.json` dan `en.json` (paritas kunci id/en). Daftar kunci dirinci di `frontend-plan.md`; minimal: `project.list.*`, `project.create.*`, `project.detail.*`, `project.settings.*`, `project.delete.*`, `project.members.*`, `project.role.owner`, `project.role.member`, `project.badge.not_member`, `project.error.<REASON>` untuk `LAST_OWNER`, `ALREADY_MEMBER`, `CONFIRMATION_MISMATCH`, `USER_NOT_FOUND`, `USER_NOT_DEVELOPER`, `USER_INACTIVE`.
10. **Dokumen**:
    - `docs/tickets/README.md`: katalog bus (5.7), catatan konvensi route (D4);
    - `docs/tickets/T03-project-management.md`: baris `createproject` (D3: "developer jadi owner; Super Admin tidak dijadikan anggota"), baris `addmember` (`role` opsional), baris baru `searchmembercandidates` (D2), kalimat audit memakai Call (D5);
    - `docs/tickets/T04-project-api-key.md`, `T05-project-knowledge.md`, `T06-project-api-spec.md`: di bagian "Disediakan" tambahkan masing-masing `project.apikey.count-keys` / `project.knowledge.count-files` / `project.apispec.count-specs` Call, kontrak `lib/contracts/project/management/contentcount` (key: hanya yang aktif); dan kalimat "tabel dengan `project_id` WAJIB `REFERENCES projects(id) ON DELETE CASCADE`; route memakai param `{projectID}`".

### U1-U12

Rincian di plan per usecase (bagian 15). Tiap usecase:
- membuat folder feature (`init.go`, `handler.go`, `params.go` bila HTTP, `usecase.go`, `repository.go` bila ada query khusus, test);
- menambah `<feature>.Init(d)` di `management/base.go`;
- memastikan path/schema-nya sudah benar di `management.yml` (seharusnya lengkap sejak F);
- menulis test AC yang menjadi tanggung jawabnya (bagian 10).

Pendaftaran route di `init.go` (contoh pola, D4):

```go
func Init(d app.Deps) {
    s := store.New(d.DB)
    uc := NewUsecase(newRepo(d.DB, s), audit.New(d.Bus, d.Log), base.Clock(func() time.Time { return time.Now().UTC() }))
    h := newHandler(uc)
    d.API.With(middleware.RequireAuth()).Post("/projects", h.ServeHTTP)
}
```

Feature bus:

```go
func Init(d app.Deps) {
    uc := NewUsecase(store.New(d.DB), users.New(d.Bus))
    d.Bus.Register(constants.BUS_PROJECT_MANAGEMENT_CHECK_ACCESS, newHandler(uc).Handle)
}
```

### W — wiring final

1. Verifikasi `management/base.go` memanggil ke-12 `Init` (urutan bebas, rule 6.1).
2. `management_integration_test.go` (tag `integration`): bus nyata, `db.TestDB(t)`, router dari `httpx.NewRouter` atau `chi.NewRouter()` + middleware identitas uji; handler palsu untuk `core.auth.resolve-session`, `core.auth.get-user`, `core.auth.find-developers`, `core.audit.record`, dan tiga endpoint count bila T01/T02/T04-T06 belum terdaftar (dengan helper `registerIfAbsent` yang hanya mendaftar di registry test baru, bukan registry produksi).
3. `TestRoutesRegistered`: `chi.Walk` atas `d.API` memuat tepat 10 pasangan method+pola dari bagian 1, dan bus memuat `check-access` serta `get-project` (Call dengan request valid tidak mengembalikan `ErrNotRegistered`).
4. `make api-check`, `go vet ./...`, archtest `TestT00_AC4_RepoIsClean`, `npm run depcruise`, `npm run test:arch` lulus.

---

## 8. Changes (per file/folder)

Lib dan migrasi (F):
- `backend/lib/constants/bus.go` (ubah): 5 konstanta T03 (+ konstanta T01/T02 bila belum ada).
- `backend/lib/contracts/project/management/checkaccess/checkaccess.go` (baru).
- `backend/lib/contracts/project/management/getproject/getproject.go` (baru).
- `backend/lib/contracts/project/management/contentcount/contentcount.go` (baru).
- `backend/lib/contracts/core/auth/{getuser,finddevelopers}/*.go`, `backend/lib/contracts/core/audit/record/{record,actions}.go` (baru HANYA bila belum ada; isi persis plan T01/T02).
- `backend/migrations/project/20261022090000000_create_projects.sql` (baru).

Domain:
- `backend/internal/domain/project/base.go` (ubah): `management.Register(d)`.
- `backend/internal/domain/project/internal/management/base.go` (baru).
- `backend/internal/domain/project/internal/management/base/{project,member,access,membership_rules,errors,actor,meta,clock,like,dto}.go` + `*_test.go` (baru).
- `backend/internal/domain/project/internal/management/base/store/{store.go,store_integration_test.go}` (baru).
- `backend/internal/domain/project/internal/management/base/audit/{audit.go,audit_test.go}` (baru).
- `backend/internal/domain/project/internal/management/base/users/{users.go,users_test.go}` (baru).
- `backend/internal/domain/project/internal/management/base/counts/{counts.go,counts_test.go}` (baru).
- `backend/internal/domain/project/internal/management/<12 feature>/...` (baru, per plan usecase).
- `backend/internal/domain/project/internal/management/management_integration_test.go` (baru, W).

API dan dokumen:
- `api/modules/project/management.yml` (baru), `api/root.yml` (ubah), `api/openapi.yml` (generated).
- `frontend/src/shared/i18n/{id,en}.json` (ubah): kunci `project.*`.
- `docs/tickets/README.md`, `docs/tickets/T03-project-management.md`, `docs/tickets/T04-project-api-key.md`, `docs/tickets/T05-project-knowledge.md`, `docs/tickets/T06-project-api-spec.md` (ubah).

Frontend (rinci di `frontend-plan.md`): `frontend/src/domain/project/management/{listprojects,createproject,getproject,updateproject,deleteproject,members}/`, layout workspace project dan komposisi tab di `frontend/src/app/`.

Tidak diubah: `internal/infra/**`, `cmd/**`, `infra/config` (tanpa env baru), `go.mod`.

---

## 9. Konsumsi per usecase (ringkas, acuan plan usecase)

| Usecase | store | repository feature | users | counts | audit |
|---|---|---|---|---|---|
| checkaccess | FindAccess | - | Get (anggota developer) | - | - |
| getprojectbus | FindProject | - | - | - | - |
| createproject | - | InsertProject, InsertMember (tx) | - | - (0 tanpa Call, lihat catatan) | project.created |
| listprojects | - | ListForDeveloper / ListAll | - | ForProjects(ids halaman) | - |
| getproject | FindAccess | MemberStats(members, owners) | - | ForProjects([id]) | - |
| updateproject | FindAccess, LockProject | UpdateProject | - | - | project.updated |
| deleteproject | FindAccess, LockProject, LockMembers | DeleteProject | - | ForProjects([id]) | project.deleted |
| listmembers | FindAccess, ListMembers | - | Get per anggota | - | - |
| searchmembercandidates | FindAccess, ListMembers | - | FindDevelopers(q, 50) | - | - |
| addmember | FindAccess, LockProject, LockMembers | InsertMember (ON CONFLICT DO NOTHING) | Get (target) | - | project.member.added |
| removemember | FindAccess, LockProject, LockMembers | DeleteMember | Get (target, untuk user_label audit; gagal -> label "") | - | project.member.removed |
| changememberrole | FindAccess, LockProject, LockMembers | UpdateMemberRole | Get (target, untuk MemberDTO dan label) | - | project.member.role_changed |

Catatan `createproject`: respons 201 memakai `counts: {files: 0, specs: 0, keys: 0}` tanpa Call (project baru pasti kosong), `member_count` 1 (developer) atau 0 (super_admin), `owner_count` tidak ada (bukan detail).
Catatan `removemember`/`changememberrole`: get-user untuk target dipanggil SETELAH commit hanya untuk label audit dan DTO; kegagalan get-user di sini tidak menggagalkan aksi (aksi sudah commit): `user_label` audit `""`, `MemberDTO.found=false`. Ini berbeda dengan `listmembers` (error -> 500) karena di sana data adalah isi respons utama.

---

## 10. Test Scenario

Penamaan: `TestT03_ACn_<Deskripsi>` untuk kriteria penerimaan; lainnya `Test<Fungsi>_<Kasus>`.

### Unit `base/` (tanpa mock)

- `ValidateName`: `"A"` gagal; `"AB"` lulus; 100 rune multibyte lulus, 101 gagal; `"  Nama  "` -> `"Nama"`; `"a\nb"` gagal; spasi saja gagal.
- `ValidateDescription`: 1000 rune lulus, 1001 gagal; `\n`/`\t` lulus; `\x00` gagal; kosong lulus.
- `NewProject`: error nama + deskripsi digabung dalam satu `FieldErrors`; `CreatedBy`/`CreatedByLabel` dari aktor.
- `ApplyUpdate`: patch kosong -> 400 `fields.body`; nama sama -> `changed` kosong, `UpdatedAt` tidak berubah; nama dan deskripsi berubah -> `["name","description"]`; nama invalid tidak mengubah struct.
- `ConfirmDeletion`: persis cocok -> nil; beda huruf besar, spasi tambahan, kosong -> `ErrConfirmationMismatch`.
- `ResolveLevel`: tabel lengkap (super_admin anggota/bukan -> admin; developer owner/member/""; developer nonaktif -> none; role tak dikenal -> none; membershipRole aneh -> none).
- `Require`: seluruh 16 sel tabel 3.4 + level tak dikenal -> 404.
- `CheckRoleChange`: no-op; member -> 403; owner terakhir turun sendiri -> 409 `LAST_OWNER`; owner turunkan owner lain saat ownerCount=2 -> nil; admin turunkan owner terakhir -> nil; member -> owner oleh owner -> nil; role invalid -> 400.
- `CheckRemove`: member keluar sendiri -> nil; member keluarkan orang lain -> 403; owner terakhir keluar sendiri -> 409; owner keluar saat ada owner lain -> nil; admin keluarkan owner terakhir -> nil; owner keluarkan member -> nil.
- `ValidateCandidate`: `!Found` -> `USER_NOT_FOUND`; role super_admin -> `USER_NOT_DEVELOPER`; developer nonaktif -> `USER_INACTIVE`; super_admin nonaktif -> `USER_NOT_DEVELOPER` (urutan).
- `NewMember`: role kosong -> member; `"admin"` -> 400.
- `LikePattern`: `50%_a\b` -> `%50\%\_a\\b%`.
- `SortMembers`: owner dulu; nama case-insensitive; `name=nil` di akhir grup; tie-break `user_id`.
- `NewSummary`/`Member.ToDTO`: JSON snapshot (`my_role: null`, `counts.files: null`, `name: null` bila tidak ditemukan).
- `errors.go`: tiap sentinel menghasilkan status yang benar lewat `respond.StatusFor` dan `details.reason` yang benar (test di `base/errors_test.go` hanya memeriksa `Code` dan `Details`; status diuji di test handler).

### Subpackage I/O

- `store` (integrasi Postgres, `db.TestDB`): `FindProject` id bukan uuid -> false tanpa query (pakai Querier pencatat); `FindAccess` anggota/bukan anggota/project tidak ada/userID bukan uuid; `LockProject` memblokir tx kedua sampai tx pertama commit (dua goroutine, tx kedua melihat data terbaru); `ListMembers` urut `user_id`; CASCADE: hapus project menghapus anggota.
- `audit` (bus nyata): handler palsu menerima `record.Request` dengan `ActorType=user`, `ActorID`, `ActorLabel=email`, `ProjectID`, `ProjectLabel`, `TraceID`, `Status=ok`; endpoint tidak terdaftar -> log error tanpa panic, tanpa error; handler error -> log error.
- `users`: Get meneruskan `getuser.Request{UserID}`; error dibungkus dan `errors.Is(err, bus.ErrNotRegistered)` tetap benar.
- `counts`: tidak ada provider -> semua nil, log Debug; satu provider error -> hanya field itu nil, log Warn; id tanpa entri -> 0; ids kosong -> tanpa Call; ids duplikat didedupe; tepat 3 Call untuk 20 id.

### Integrasi lintas usecase (`management_integration_test.go`, tag `integration`)

Fixture: user palsu di handler `get-user`/`find-developers` palsu: A, B, C (developer aktif), D (developer nonaktif), S (super_admin). Identitas disuntikkan lewat handler palsu `resolve-session` atau middleware uji yang memanggil `reqctx.WithIdentity`.

- `TestT03_AC1_CreatorIsOwnerAndInvisibleToOthers`: A `POST /projects` -> 201, `my_role=owner`; `GET /projects` oleh B tidak memuatnya; `GET /projects/{id}` oleh B -> 404 `project tidak ditemukan`.
- `TestT03_AC2_MemberCanEditButNotManage`: A tambah B -> 201; B `GET /projects` memuatnya; B `PATCH` nama -> 200; B `POST members` (C) -> 403; B `DELETE members/A` -> 403; B `PATCH members/A` -> 403; B `DELETE /projects/{id}` -> 403; B `GET member-candidates` -> 403.
- `TestT03_AC3_LastOwnerCannotLeaveOrDemoteSelf`: A `DELETE members/A` -> 409 `LAST_OWNER`; A `PATCH members/A {role: member}` -> 409; S `PATCH members/A {role: member}` -> 200 (project tanpa owner, `owner_count=0`); S `DELETE members/A` -> 204.
- `TestT03_AC3_ConcurrentMutualDemotion`: A dan C owner; dua goroutine bersamaan A menurunkan C dan C menurunkan A, diulang 20 kali dengan project baru -> tiap putaran tepat satu 200 dan satu 403, `owner_count == 1`.
- `TestT03_AC4_RejectIneligibleCandidates`: tambah D -> 400 `USER_INACTIVE`; tambah S -> 400 `USER_NOT_DEVELOPER`; tambah uuid acak -> 400 `USER_NOT_FOUND`; tambah `"bukan-uuid"` -> 400 `USER_NOT_FOUND`; tambah B dua kali -> 409 `ALREADY_MEMBER`.
- `TestT03_AC5_DeactivatedMemberHasNoAccess`: B anggota; get-user palsu mengubah B `IsActive=false`; `check-access` B -> `{true, none}`; A tetap `owner`; S -> `admin` tanpa memanggil get-user (hitung panggilan).
- `TestT03_AC6_DeleteRequiresConfirmationAndCascades`: buat tabel dummy `project_child(id uuid primary key default gen_random_uuid(), project_id uuid not null references projects(id) on delete cascade)` di schema test; isi satu baris; `DELETE` dengan `confirm_name` salah -> 400 `CONFIRMATION_MISMATCH`, data utuh; body kosong -> 400; benar -> 204; `projects`, `project_members`, `project_child` untuk id itu kosong; tepat satu entri audit `project.deleted` dengan `args.name`, `args.member_count`, `args.counts`.
- `TestT03_AC6_AllProjectFKsCascade`: query `pg_constraint` (`contype='f' AND confrelid='projects'::regclass`) -> semua `confdeltype='c'`, minimal 1 (project_members). Tabel T04-T06 otomatis tercakup saat migrasinya ada.
- `TestT03_AC7_SuperAdminSeesAndManagesAll`: A dan C masing-masing membuat project; S `GET /projects` melihat keduanya dengan `access_level=admin`, `is_member=false`, `my_role=null`; S `POST /projects` -> 201 `member_count=0`; S tambah A `role=owner` -> 201; S mengubah peran dan mengeluarkan anggota project C -> sukses.
- `TestT03_AC8_CheckAccessUnknownProject`: uuid acak -> `{false, none}`, err nil; `"bukan-uuid"` -> sama; tipe request salah -> `ErrBadRequestType`; `UserRole="agent"` -> `errors.Is(err, errs.ErrInvalid)`.
- `TestT03_Security_ProjectIsolation` (rule 13.6): A membuat project PA; B (bukan anggota) memanggil ke-10 route terhadap PA -> 404 seragam (`code=NOT_FOUND`, pesan sama, tanpa field lain), dan data PA tidak berubah; B juga mendapat 404 dengan id bukan uuid. `check-access` B pada PA -> `none`. `get-project` tidak dibatasi aktor (internal T07), hanya mengembalikan id+nama.
- `TestT03_Audit_OneEntryPerAction`: create/update/add/role-change/remove/delete masing-masing tepat satu entri dengan aksi dan `project_label` benar; update no-op dan role-change no-op -> 0 entri; audit handler yang gagal tidak menggagalkan request (fail-open).
- `TestT03_Counts_FailSoft`: tanpa provider count -> `counts` semua `null`, status 200; dengan provider palsu files=3 -> `files: 3`, lainnya `null`.
- `TestRoutesRegistered`: bagian 7 W.3.

Test per usecase (unit usecase dengan repo/bus palsu, handler dengan `httptest`) dirinci di plan usecase masing-masing.

---

## 11. E2E Scenarios (Playwright terhadap binary; butuh T01 dan T02 merge)

Positif:
1. Developer A login -> beranda kosong -> "Project baru" (nama "Payments", deskripsi) -> masuk workspace, tab Members memuat A sebagai owner.
2. A membuka dialog tambah anggota, mengetik "bud" -> autocomplete menampilkan developer aktif yang belum anggota -> pilih B -> B muncul sebagai member.
3. B login -> beranda menampilkan "Payments" dengan peran member -> tab Settings: ubah nama -> tersimpan; tab Members hanya menampilkan tombol "Keluar".
4. A mengangkat B menjadi owner, lalu A keluar -> A tidak melihat project lagi; B owner.
5. Super Admin login -> beranda menampilkan semua project dengan penanda "bukan anggota" -> membuka project, mengeluarkan anggota, menunjuk owner.
6. B (owner) membuka Settings -> zona bahaya -> mengetik nama persis -> project terhapus, kembali ke beranda tanpa project itu; audit log (Super Admin) memuat `project.deleted`.

Negatif:
1. Developer C membuka URL `/projects/<id PA>` langsung -> halaman "tidak ditemukan" (API 404).
2. Member mencoba aksi kelola anggota lewat API langsung (tombol disembunyikan) -> 403, UI menampilkan pesan akses ditolak.
3. Owner terakhir menekan "Keluar" -> pesan i18n `project.error.LAST_OWNER`.
4. Tambah anggota dengan user nonaktif (lewat API langsung, karena autocomplete hanya menampilkan developer aktif) -> 400 `USER_INACTIVE`.
5. Hapus project dengan nama konfirmasi salah -> tombol nonaktif di UI; lewat API -> 400 `CONFIRMATION_MISMATCH`.
6. Buat project dengan nama 1 karakter -> pesan field `name`.
7. `POST /api/projects` tanpa header `X-Requested-With` -> 403 `CSRF_REQUIRED`.
8. Tanpa login `GET /api/projects` -> 401.

---

## 12. Request/Response body

Semua error memakai format seragam T00:

```json
{ "error": { "code": "NOT_FOUND", "message": "project tidak ditemukan", "details": null }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

Header mutasi: `X-Requested-With: grimoire`, `Content-Type: application/json`.

### POST /api/projects

Request:
```json
{ "name": "Payments", "description": "Layanan pembayaran internal" }
```

201 (developer A):
```json
{
  "id": "0192f5a0-1111-7aaa-8bbb-000000000001",
  "name": "Payments",
  "description": "Layanan pembayaran internal",
  "created_by": "0192f5a0-aaaa-7aaa-8bbb-00000000000a",
  "created_by_label": "a@tim.id",
  "created_at": "2026-10-22T09:00:00Z",
  "updated_at": "2026-10-22T09:00:00Z",
  "my_role": "owner",
  "access_level": "owner",
  "is_member": true,
  "member_count": 1,
  "counts": { "files": 0, "specs": 0, "keys": 0 }
}
```

201 (super_admin S): sama, tetapi `"my_role": null`, `"access_level": "admin"`, `"is_member": false`, `"member_count": 0`.

400:
```json
{
  "error": {
    "code": "INVALID_ARGUMENT",
    "message": "permintaan tidak valid",
    "details": { "fields": { "name": "nama project wajib 2-100 karakter", "description": "deskripsi maksimal 1000 karakter" } }
  },
  "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A"
}
```

Field tak dikenal (`{"nama": "x"}`) -> 400 dari `respond.DecodeJSON`.

### GET /api/projects?q=pay&limit=20&offset=0

200:
```json
{
  "items": [
    {
      "id": "0192f5a0-1111-7aaa-8bbb-000000000001",
      "name": "Payments",
      "description": "Layanan pembayaran internal",
      "created_by": "0192f5a0-aaaa-7aaa-8bbb-00000000000a",
      "created_by_label": "a@tim.id",
      "created_at": "2026-10-22T09:00:00Z",
      "updated_at": "2026-10-22T10:15:00Z",
      "my_role": "member",
      "access_level": "member",
      "is_member": true,
      "member_count": 2,
      "counts": { "files": null, "specs": null, "keys": null }
    }
  ],
  "total": 1,
  "limit": 20,
  "offset": 0
}
```

400 (`limit=500`):
```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "permintaan tidak valid", "details": { "fields": { "limit": "limit harus 1-100" } } }, "trace_id": "..." }
```

### GET /api/projects/{projectID}

200: `ProjectSummary` di atas + `"owner_count": 1`.

404 (bukan anggota, tidak ada, atau bukan uuid):
```json
{ "error": { "code": "NOT_FOUND", "message": "project tidak ditemukan", "details": null }, "trace_id": "..." }
```

### PATCH /api/projects/{projectID}

Request (minimal satu field):
```json
{ "name": "Payments v2" }
```

200: `ProjectDetail` (bentuk sama dengan `GET /api/projects/{projectID}`; `member_count`, `owner_count`, dan `counts` diambil dengan cara yang sama seperti getproject). No-op juga 200 dengan data saat ini, tanpa audit.

400 (body `{}`):
```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "permintaan tidak valid", "details": { "fields": { "body": "minimal satu field (name, description) wajib diisi" } } }, "trace_id": "..." }
```

### DELETE /api/projects/{projectID}

Request:
```json
{ "confirm_name": "Payments v2" }
```

204 tanpa body.

400:
```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "nama konfirmasi tidak cocok", "details": { "reason": "CONFIRMATION_MISMATCH" } }, "trace_id": "..." }
```

403 (member):
```json
{ "error": { "code": "FORBIDDEN", "message": "akses ditolak", "details": null }, "trace_id": "..." }
```

### GET /api/projects/{projectID}/members

200:
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
    }
  ]
}
```

### GET /api/projects/{projectID}/member-candidates?q=bud&limit=10

200:
```json
{ "items": [ { "id": "0192f5a0-cccc-7aaa-8bbb-00000000000c", "name": "Budiman", "email": "budiman@tim.id" } ] }
```

`q` kosong -> `{ "items": [] }`. Member -> 403. Bukan anggota -> 404.

### POST /api/projects/{projectID}/members

Request:
```json
{ "user_id": "0192f5a0-cccc-7aaa-8bbb-00000000000c", "role": "member" }
```

201: `MemberDTO` (seperti item di listmembers).

400 (AC4):
```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "user nonaktif tidak bisa ditambahkan sebagai anggota", "details": { "reason": "USER_INACTIVE" } }, "trace_id": "..." }
```
```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "hanya developer yang bisa ditambahkan sebagai anggota; Super Admin tidak perlu menjadi anggota", "details": { "reason": "USER_NOT_DEVELOPER" } }, "trace_id": "..." }
```
```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "user tidak ditemukan", "details": { "reason": "USER_NOT_FOUND" } }, "trace_id": "..." }
```

400 (`user_id` kosong):
```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "permintaan tidak valid", "details": { "fields": { "user_id": "wajib diisi" } } }, "trace_id": "..." }
```

409:
```json
{ "error": { "code": "CONFLICT", "message": "user sudah menjadi anggota", "details": { "reason": "ALREADY_MEMBER" } }, "trace_id": "..." }
```

### DELETE /api/projects/{projectID}/members/{userID}

204 tanpa body (tanpa request body).

409:
```json
{ "error": { "code": "CONFLICT", "message": "owner terakhir tidak bisa keluar atau diturunkan; angkat owner lain lebih dulu", "details": { "reason": "LAST_OWNER" } }, "trace_id": "..." }
```

404 (target bukan anggota):
```json
{ "error": { "code": "NOT_FOUND", "message": "anggota tidak ditemukan", "details": null }, "trace_id": "..." }
```

### PATCH /api/projects/{projectID}/members/{userID}

Request:
```json
{ "role": "owner" }
```

200: `MemberDTO` dengan role baru (no-op juga 200). 409 `LAST_OWNER` dan 403 seperti di atas. Role invalid:
```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "permintaan tidak valid", "details": { "fields": { "role": "role harus owner atau member" } } }, "trace_id": "..." }
```

### Bus (Go, bukan JSON di wire)

```go
res, err := bus.CallTyped[checkaccess.Response](ctx, d.Bus, constants.BUS_PROJECT_MANAGEMENT_CHECK_ACCESS,
    checkaccess.Request{ProjectID: "0192f5a0-1111-7aaa-8bbb-000000000001", UserID: "0192f5a0-bbbb-7aaa-8bbb-00000000000b", UserRole: "developer"})
// res == checkaccess.Response{ProjectExists: true, Level: checkaccess.LevelMember}

g, err := bus.CallTyped[getproject.Response](ctx, d.Bus, constants.BUS_PROJECT_MANAGEMENT_GET_PROJECT,
    getproject.Request{ProjectID: "0192f5a0-1111-7aaa-8bbb-000000000001"})
// g == getproject.Response{Found: true, ID: "0192f5a0-1111-7aaa-8bbb-000000000001", Name: "Payments"}

c, err := bus.CallTyped[contentcount.Response](ctx, d.Bus, constants.BUS_PROJECT_KNOWLEDGE_COUNT_FILES,
    contentcount.Request{ProjectIDs: []string{"0192f5a0-1111-7aaa-8bbb-000000000001"}})
// c == contentcount.Response{Counts: map[string]int{"0192f5a0-1111-7aaa-8bbb-000000000001": 12}}
```

### Audit (`record.Request` yang dikirim, contoh `project.member.removed` self-leave)

```go
record.Request{
    ActorType: record.ActorUser, ActorID: "0192f5a0-bbbb-7aaa-8bbb-00000000000b", ActorLabel: "b@tim.id",
    ProjectID: "0192f5a0-1111-7aaa-8bbb-000000000001", ProjectLabel: "Payments",
    Action: record.ActionProjectMemberRemoved, TargetType: "project_member", Target: "0192f5a0-bbbb-7aaa-8bbb-00000000000b",
    Args: map[string]any{"user_id": "0192f5a0-bbbb-7aaa-8bbb-00000000000b", "user_label": "b@tim.id", "role": "member", "self": true},
    Status: record.StatusOK, IP: "10.0.0.5", UserAgent: "Mozilla/5.0", TraceID: "01JB2X9K6Q4Y7V3N8M5T2R1C0A",
}
```

Katalog audit T03:

| Action | Usecase | Target | Args |
|---|---|---|---|
| `project.created` | createproject | project/id | `{name}` |
| `project.updated` | updateproject | project/id | `{changed_fields, name_from?, name_to?}` (name_* hanya bila nama berubah) |
| `project.deleted` | deleteproject | project/id | `{name, member_count, counts: {files, specs, keys}}` (null bila tidak tersedia; dihitung sebelum DELETE) |
| `project.member.added` | addmember | project_member/user_id | `{user_id, user_label, role}` |
| `project.member.removed` | removemember | project_member/user_id | `{user_id, user_label, role, self}` |
| `project.member.role_changed` | changememberrole | project_member/user_id | `{user_id, user_label, role_from, role_to}` |

`ProjectLabel` selalu snapshot nama project saat aksi (untuk `project.updated`: nama setelah update). Deskripsi project tidak dikirim ke audit.

---

## 13. Skema OpenAPI (`api/modules/project/management.yml`, ringkas)

- `info: {title: Grimoire Project Management, version: 0.1.0}`.
- `components.parameters`: `XRequestedWith` (header, wajib di POST/PATCH/DELETE), `ProjectID` (path `projectID`, `format: uuid`), `UserID` (path `userID`, `format: uuid`), `Q`, `Limit`, `Offset`.
- `components.schemas`: `Project`, `ContentCounts` (tiap field `type: [integer, "null"]`), `ProjectSummary`, `ProjectDetail`, `ProjectList` (`items`, `total`, `limit`, `offset`), `CreateProjectRequest`, `UpdateProjectRequest` (`minProperties: 1`), `DeleteProjectRequest` (`confirm_name` wajib), `Member`, `MemberList`, `MemberCandidate`, `MemberCandidateList`, `AddMemberRequest` (`user_id` wajib, `role` enum opsional default member), `ChangeMemberRoleRequest`.
- Tiap operasi: `operationId` = nama feature, `tags: [projects]` atau `[project-members]`, respons error `$ref: ../infra/foundation.yml#/components/responses/{BadRequest,Unauthorized,Forbidden,NotFound,Conflict,Internal}`; deskripsi 404 menyebut "juga untuk project yang bukan milik pemanggil (tidak bocor)".
- `api/root.yml`: 5 path (`/api/projects`, `/api/projects/{projectID}`, `/api/projects/{projectID}/members`, `/api/projects/{projectID}/members/{userID}`, `/api/projects/{projectID}/member-candidates`) dan schema `Project`, `ProjectSummary`, `ProjectDetail`, `ProjectList`, `Member`, `MemberList`, `MemberCandidateList`.

---

## 14. Risiko dan catatan

- T01 dan T02 sama-sama merencanakan `20261021090000000` untuk migrasi `core`. Itu harus diselesaikan di antara mereka; T03 aman dengan `20261022090000000`.
- `listmembers` memanggil get-user sekali per anggota (tidak ada endpoint batch di katalog). Wajar untuk jumlah anggota kecil (puluhan). Bila nanti lambat, tambah endpoint batch di T01 (tiket baru, bukan di T03).
- Hitungan isi `null` sampai T04-T06 merge; UI menampilkan "-". Ini disengaja (D1).
- D3 mengubah makna "pembuat jadi owner" untuk Super Admin. Mohon ditinjau di PR; alternatifnya (Super Admin jadi owner) bertentangan dengan "Super Admin tidak ditambahkan sebagai anggota".
- `get-project` tidak memeriksa aktor. Ia hanya untuk konsumsi internal (T07 setelah verifikasi key) dan hanya mengembalikan id + nama.
- Bila T01/T02 merge dengan bentuk kontrak berbeda dari plan, yang terdampak hanya `base/users`, `base/audit`, dan `checkaccess`; sesuaikan ke kode di `main`.

---

## 15. Daftar usecase (plan rinci ditulis terpisah di `docs/superpowers/plans/project-management/usecase-<slug>.md`)

| # | Slug | Gelombang | Titik masuk | Ringkasan | AC |
|---|---|---|---|---|---|
| U1 | `checkaccess` | A | bus `project.management.check-access` (Call) | `Expect[checkaccess.Request]`; validasi UserID/UserRole (ErrInvalid); satu query LEFT JOIN; super_admin -> admin tanpa get-user; developer anggota -> get-user, nonaktif/tidak ada -> none; project tak ada/bukan uuid -> `ProjectExists=false` tanpa error; error get-user fail-closed. Tanpa audit | AC5, AC8 |
| U2 | `getprojectbus` | A | bus `project.management.get-project` (Call) | `{ProjectID}` -> `{Found, ID, Name}` via `store.FindProject`; bukan uuid/tidak ada -> Found=false; tipe salah -> ErrBadRequestType. Tanpa audit, tanpa HTTP | - (T07) |
| U3 | `createproject` | A | `POST /api/projects` | Validasi di entity; satu tx INSERT project (+ owner bila developer, D3). 201 ProjectSummary. Audit `project.created` | AC1, AC7 |
| U4 | `listprojects` | B | `GET /api/projects` | Developer: hanya yang diikuti; super_admin: semua + is_member. ILIKE, limit/offset, urut updated_at DESC, total, member_count, my_role/access_level, counts batch fail-soft | AC1, AC7 |
| U5 | `getproject` | B | `GET /api/projects/{projectID}` | FindAccess -> ResolveLevel -> Require(PermView); 404 tanpa bocor; detail + owner_count + counts | AC1, AC2 |
| U6 | `updateproject` | B | `PATCH /api/projects/{projectID}` | Require(PermEdit); tx FOR UPDATE -> ApplyUpdate -> UPDATE bila berubah; no-op 200 tanpa audit. Audit `project.updated` | AC2 |
| U7 | `deleteproject` | B | `DELETE /api/projects/{projectID}` | Require(PermDelete); ConfirmDeletion; hitung member_count + counts; tx LockProject -> DELETE (cascade). 204. Audit `project.deleted`. Test pg_constraint + tabel dummy | AC6 |
| U8 | `listmembers` | B | `GET /api/projects/{projectID}/members` | Require(PermView); enrich get-user per anggota; urut owner, nama, id; error get-user -> 500 | AC2 |
| U9 | `addmember` | C | `POST /api/projects/{projectID}/members` | Require(PermManageMembers); get-user + ValidateCandidate (400 reason); tx lock -> INSERT ON CONFLICT DO NOTHING (409). 201. Audit `project.member.added` | AC2, AC4, AC7 |
| U10 | `searchmembercandidates` | C | `GET /api/projects/{projectID}/member-candidates` | Require(PermManageMembers); q kosong -> []; find-developers Limit 50, buang anggota, potong ke limit. Tanpa audit (D2) | AC2 |
| U11 | `removemember` | C | `DELETE /api/projects/{projectID}/members/{userID}` | Tx lock -> hitung ulang level -> CheckRemove (403/409/admin boleh) -> DELETE; target bukan anggota 404. 204. Audit `project.member.removed {self}` | AC2, AC3 |
| U12 | `changememberrole` | C | `PATCH /api/projects/{projectID}/members/{userID}` | Tx lock -> CheckRoleChange (no-op 200 tanpa audit; 409 LAST_OWNER; admin boleh) -> UPDATE; race dua owner tepat satu sukses. 200 MemberDTO. Audit `project.member.role_changed` | AC3 |

Frontend: `docs/superpowers/plans/project-management/frontend-plan.md` (ditulis terpisah): `frontend/src/domain/project/management/{listprojects,createproject,getproject,updateproject,deleteproject,members}/` dengan `services` (Zod) / `stores` / `components` / `views` / `index.ts`; layout workspace project dengan tab-slot dan komposisi tab Knowledge/API Specs/API Keys di `app/` (rule 12.4); penanda "bukan anggota" untuk Super Admin; zona bahaya hapus dengan ketik nama; tombol menyesuaikan `access_level`, otorisasi tetap di backend.
