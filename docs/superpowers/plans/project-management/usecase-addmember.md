# T03 project/management — Usecase `addmember` (U9, gelombang C)

Melengkapi `docs/superpowers/plans/project-management/overview.md` (selanjutnya "overview"). Bila dokumen ini bertentangan dengan overview, overview menang. Hanya satu usecase: `addmember`. Plan usecase lain tidak disentuh.

## Metadata

| Field | Nilai |
|---|---|
| Ticket | T03 (AC2, AC4, AC7) |
| Scope | fullstack pada tiket; usecase ini backend + kontrak OpenAPI. Dialog "Tambah anggota" ada di `frontend-plan.md` |
| Domain | project |
| Module | management |
| Feature | `addmember` |
| Endpoint | `/api/projects/{projectID}/members` |
| Method | POST |
| Version | tanpa prefix versi di URL (prefix `/api`); `management.yml` `info.version: 0.1.0` |
| Siapa | owner project, super_admin (dicek di usecase lewat `base.Require(level, base.PermManageMembers)`; middleware hanya `RequireAuth()`) |
| Header wajib | `X-Requested-With: grimoire` (CSRF T00), `Content-Type: application/json` |
| Respons sukses | 201 `MemberDTO` |
| Audit | `project.member.added` (Call fail-open, setelah commit) |
| Bus dikonsumsi | `core.auth.get-user` (target saja, SEBELUM tx; overview D6) dan `core.audit.record` (lewat `base/audit`) |
| Tabel / migrasi | menulis `project_members`; mengunci `projects`; migrasi dibuat di F overview, tidak ada migrasi baru di sini |
| Resource root | `backend/internal/domain/project/internal/management` |
| allowedPaths | `.../management/addmember/`, `.../management/base.go` (satu baris `addmember.Init(d)`) |

Prasyarat (dari F overview; bila belum ada, hentikan dan laporkan, jangan dibuat di luar allowedPaths): migrasi `20261022090000000_create_projects.sql`; `base` (`Actor`, `ActorFrom`, `RequestMeta`, `Clock`, `Member`, `NewMember`, `RoleMember`, `RoleOwner`, `FindMember`, `ValidateCandidate`, `ResolveLevel`, `Require`, `PermManageMembers`, `ErrProjectNotFound`, `ErrAlreadyMember`, `FieldErrors`, `InvalidFields`, `MemberDTO`, `Member.ToDTO`); `base/store` (`Store`, `New`, `WithTx`, `FindAccess`, `LockProject`, `LockMembers`); `base/users` (`Lookup.Get`); `base/audit` (`Recorder`, `Event`, `New`); kontrak `getuser` (T01) dan `record` + `record.ActionProjectMemberAdded` (T02) dengan bentuk persis plan pemiliknya; konstanta `BUS_CORE_AUTH_GET_USER` dan `BUS_CORE_AUDIT_RECORD`. Path `POST /projects/{projectID}/members` dan schema `AddMemberRequest`/`Member` sudah ada di `api/modules/project/management.yml` dari F; usecase ini hanya memverifikasi.

## Summary Plan

`addmember` menambahkan satu developer aktif sebagai anggota project. Satu usecase, satu `Execute`, tanpa pemecahan artifisial.

Aturan:
- Body: `{ "user_id": "<uuid>", "role": "member" | "owner" }`. `role` opsional, default `member` (overview D3). `user_id` wajib (string tidak kosong setelah trim) — dicek di handler/params sebagai validasi bentuk: kosong -> 400 `fields.user_id = "wajib diisi"`. Isi `user_id` TIDAK divalidasi sebagai uuid di sini; nilai bukan uuid dijawab `get-user` dengan `Found=false` -> 400 `USER_NOT_FOUND` (AC4).
- Otorisasi (overview 3.4), sebelum apa pun yang menyentuh bus: `store.FindAccess(projectID, actor.ID)`; tidak ketemu atau id bukan uuid -> `base.ErrProjectNotFound` (404). `level = base.ResolveLevel(actor.Role, memberRole, true)`; `base.Require(level, base.PermManageMembers)`: `none` -> 404, `member` -> 403, `owner`/`admin` lolos. Dengan begitu non-anggota tidak bisa memakai endpoint ini sebagai oracle keberadaan user (get-user tidak pernah dipanggil sebelum lolos otorisasi).
- Validasi role lewat entity: `base.NewMember(projectID, userID, role, actor, now)` (role `""` -> member; selain owner/member -> 400 `fields.role`). Dijalankan setelah otorisasi dan sebelum bus, sehingga request cacat tidak memicu panggilan bus.
- Validasi kandidat (AC4) lewat `u.Get(targetID)` (`core.auth.get-user`) lalu `base.ValidateCandidate(resp)`: tidak ada/bukan uuid -> 400 `USER_NOT_FOUND`; role bukan developer (termasuk super_admin) -> 400 `USER_NOT_DEVELOPER`; nonaktif -> 400 `USER_INACTIVE`. Urutan cek persis overview 4.2 (super_admin nonaktif -> `USER_NOT_DEVELOPER`). Error bus (termasuk `bus.ErrNotRegistered`) -> dibungkus, 500 (fail-closed: tanpa verifikasi, tidak boleh menambah anggota).
- `user_id` yang disimpan adalah `resp.ID` dari get-user (bentuk kanonik uuid), bukan string mentah dari klien, agar perbedaan huruf besar/kecil pada uuid tidak membuat duplikat semu.
- Panggilan bus `get-user` dilakukan SEBELUM transaksi; tidak ada panggilan bus di dalam tx (overview 3.5).
- Transaksi (overview 3.5): `LockProject` (FOR UPDATE; hilang -> 404) -> `LockMembers` -> hitung ulang level aktor dari daftar terkunci (`FindMember(members, actor.ID)`; `memberRole` "" bila tidak ada) lalu `Require(PermManageMembers)` lagi (aktor bisa saja baru dikeluarkan/diturunkan oleh transaksi lain antara pre-check dan lock) -> bila target sudah ada di daftar terkunci -> `base.ErrAlreadyMember` (409) -> `INSERT ... ON CONFLICT (project_id, user_id) DO NOTHING`; 0 baris terpengaruh -> `base.ErrAlreadyMember` (jaring pengaman) -> commit.
- `added_by` = id aktor, `added_by_label` = email aktor (snapshot, rule 8.7), `added_at` dari `Clock`.
- Admin (super_admin) boleh menambah dengan `role: owner` pada project tanpa owner (AC7, D3). Owner juga boleh menambah owner lain. Tidak ada rule `ownerCount` pada penambahan.
- Owner menambah dirinya sendiri / anggota yang sudah ada -> 409 `ALREADY_MEMBER`. Super_admin sebagai target selalu 400 `USER_NOT_DEVELOPER`.
- Audit SETELAH commit lewat `audit.Recorder` (fail-open, D5): tepat satu entri `project.member.added` per penambahan sukses; 400/403/404/409/500 -> 0 entri. `ProjectLabel` = nama project dari baris terkunci. `Args` = `{user_id, user_label (email target), role}`; tidak ada data sensitif; body tidak di-log.
- Respons: `m.ToDTO(userResp)` -> `MemberDTO` (`found=true`, `is_active=true`, name/email dari get-user, `added_by`/`added_by_label` aktor). Tidak ada panggilan get-user kedua.
- Race kecil yang diterima: target dinonaktifkan di antara get-user dan INSERT. Dampaknya anggota nonaktif yang akses-nya sudah ditolak `check-access` (AC5) dan terlihat `is_active=false` di `listmembers`; tidak ada celah akses. Dicatat, tidak ditangani.
- Tidak idempotent: pengulangan -> 409.
- Error infra dibungkus `fmt.Errorf("addmember: <langkah>: %w", err)` dan menjadi 500 generik. Rollback otomatis bila fn mengembalikan error (`db.BeginTx`).

## Plan

Urutan wajib. Tulis test dulu (TDD), lalu implementasi.

**B0. Verifikasi (tanpa edit).** Pastikan simbol prasyarat ada. Baca `app.Deps`, `db.DB.BeginTx(ctx, func(*sqlx.Tx) error)`, `respond.DecodeJSON/WriteJSON/WriteError`, `middleware.RequireAuth/KeyByIP`, `bus.CallTyped`, dan pola `Init` feature yang sudah ada (mis. `createproject`/`checkaccess` bila sudah merge) lalu ikuti. Pastikan `getuser.Response` di `main` bentuknya `{Found, ID, Name, Email, Role, IsActive}`; bila beda, sesuaikan hanya di `base/users` (di luar allowedPaths: laporkan).

**B1. `addmember/params.go`**

```go
type AddMemberRequest struct {
    UserID string `json:"user_id"`
    Role   string `json:"role"` // opsional; "" -> member
}

// Validate hanya bentuk. Isi (uuid valid? role valid? developer aktif?) bukan urusan di sini.
func (r *AddMemberRequest) Validate() error {
    r.UserID = strings.TrimSpace(r.UserID)
    if r.UserID == "" {
        return base.InvalidFields(base.FieldErrors{"user_id": "wajib diisi"})
    }
    return nil
}
```
`respond.DecodeJSON` menolak field tak dikenal, body kosong, JSON rusak, dan tipe salah (mis. `user_id` angka) dengan 400. Validasi isi `role` ada di `base.NewMember`.

**B2. `addmember/usecase.go`**

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
}
func NewUsecase(r Repository, u users.Lookup, a audit.Recorder, c base.Clock) *Usecase

func (u *Usecase) Execute(ctx context.Context, projectID string, in AddMemberRequest, meta base.RequestMeta) (base.MemberDTO, error)
```

Alur `Execute` (orkestrasi saja; rule bisnis ada di `base`):
1. `actor, ok := base.ActorFrom(reqctx.IdentityFrom(ctx))`; `!ok` -> `errs.New(errs.CodeUnauthorized, "perlu login")`.
2. Pre-check akses: `_, memberRole, found, err := u.repo.FindAccess(ctx, projectID, actor.ID)`; err -> `fmt.Errorf("addmember: find access: %w", err)`; `!found` -> `base.ErrProjectNotFound`; `base.Require(base.ResolveLevel(actor.Role, memberRole, true), base.PermManageMembers)` -> kembalikan err apa adanya.
3. `now := u.clock()`; `draft, err := base.NewMember(projectID, in.UserID, in.Role, actor, now)`; err (400 `fields.role`) dikembalikan apa adanya. (Draft hanya untuk validasi role; `UserID` diganti `resp.ID` di langkah 5.)
4. `target, err := u.users.Get(ctx, in.UserID)`; err -> `fmt.Errorf("addmember: get user: %w", err)`. `if err := base.ValidateCandidate(target); err != nil { return base.MemberDTO{}, err }`.
5. `draft.UserID = target.ID`.
6. `var project base.Project`; `err = u.repo.InTx(ctx, func(tx Tx) error { ... })`:
   - `p, ok, err := tx.LockProject(ctx, projectID)`; err -> bungkus `"lock project"`; `!ok` -> `base.ErrProjectNotFound`; `project = p`.
   - `ms, err := tx.LockMembers(ctx, projectID)`; err -> bungkus `"lock members"`.
   - `mine, _ := base.FindMember(ms, actor.ID)` (role "" bila tidak ada); `base.Require(base.ResolveLevel(actor.Role, mine.Role, true), base.PermManageMembers)`; err dikembalikan apa adanya (rollback).
   - `if _, exists := base.FindMember(ms, draft.UserID); exists { return base.ErrAlreadyMember }`.
   - `inserted, err := tx.InsertMember(ctx, draft)`; err -> bungkus `"insert member"`; `!inserted` -> `base.ErrAlreadyMember`.
7. Error dari `InTx` dikembalikan apa adanya (sudah `*errs.Error` atau terbungkus).
8. Audit setelah commit: `u.audit.Record(ctx, audit.Event{Action: record.ActionProjectMemberAdded, ProjectID: project.ID, ProjectLabel: project.Name, TargetType: "project_member", Target: draft.UserID, Args: map[string]any{"user_id": draft.UserID, "user_label": target.Email, "role": draft.Role}, Meta: meta})`. Tidak mengembalikan error.
9. `return draft.ToDTO(target), nil`.

Tidak ada rule bisnis di usecase: kelayakan kandidat di `base.ValidateCandidate`, role di `base.NewMember`, izin di `base.Require`. Tidak ada panggilan bus di dalam `InTx`.

**B3. `addmember/repository.go`** (satu-satunya yang menyentuh SQL untuk feature ini; query bersama dipakai dari `base/store`)

```go
type Repo struct {
    db *db.DB
    s  *store.Store // di luar tx: store.New(db); di dalam tx: s.WithTx(tx)
}
func newRepo(d *db.DB, s *store.Store) *Repo
func (r *Repo) FindAccess(ctx, projectID, userID string) (base.Project, string, bool, error) { return r.s.FindAccess(ctx, projectID, userID) }
func (r *Repo) InTx(ctx context.Context, fn func(Tx) error) error {
    return r.db.BeginTx(ctx, func(tx *sqlx.Tx) error {
        return fn(&txRepo{s: r.s.WithTx(tx), q: tx})
    })
}
// txRepo: LockProject/LockMembers didelegasikan ke store; InsertMember di bawah.
```
SQL khusus feature (placeholder posisional, tanpa string concat):
```sql
INSERT INTO project_members (project_id, user_id, role, added_by, added_by_label, added_at)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (project_id, user_id) DO NOTHING;
```
`inserted = (RowsAffected() == 1)`. `user_id`/`added_by` string uuid -> kolom `uuid` (nilai sudah kanonik dari get-user dan id aktor dari sesi). Waktu `added_at` disimpan UTC.

**B4. `addmember/handler.go`**

```go
type Handler struct{ uc *Usecase }
func newHandler(uc *Usecase) *Handler
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    var in AddMemberRequest
    if err := respond.DecodeJSON(r, &in); err != nil { respond.WriteError(w, r, err); return }
    if err := in.Validate(); err != nil { respond.WriteError(w, r, err); return }
    meta := base.RequestMeta{IP: middleware.KeyByIP(r), UserAgent: truncate(r.UserAgent(), 512)}
    res, err := h.uc.Execute(r.Context(), chi.URLParam(r, "projectID"), in, meta)
    if err != nil { respond.WriteError(w, r, err); return }
    respond.WriteJSON(w, http.StatusCreated, res)
}
```
`projectID` diteruskan apa adanya (overview 3.3). `truncate`: pakai helper bersama bila sudah ada di `base`; jangan buat salinan kedua.

Catatan urutan: validasi bentuk (`user_id` kosong) terjadi sebelum otorisasi dan jawabannya tidak bergantung pada project, jadi tidak membocorkan keberadaan project.

**B5. `addmember/init.go`**

```go
func Init(d app.Deps) {
    s := store.New(d.DB)
    uc := NewUsecase(
        newRepo(d.DB, s),
        users.New(d.Bus),
        audit.New(d.Bus, d.Log),
        base.Clock(func() time.Time { return time.Now().UTC() }),
    )
    d.API.With(middleware.RequireAuth()).Post("/projects/{projectID}/members", newHandler(uc).ServeHTTP)
}
```
Pola penuh langsung di `d.API`, nama param `{projectID}` (D4). Route `GET .../members` milik `listmembers` memakai pola yang sama (method berbeda, tidak bentrok).

**B6. Wiring**: tambah satu baris `addmember.Init(d)` di `management/base.go` (`Register`). Jangan menyentuh baris Init feature lain.

**B7. OpenAPI**: verifikasi operasi `addMember` di `api/modules/project/management.yml` (tag `project-members`, parameter `XRequestedWith` + `ProjectID`, request `AddMemberRequest` dengan `user_id` wajib dan `role` enum `[owner, member]` default `member`, respons 201 `Member`, error `BadRequest/Unauthorized/Forbidden/NotFound/Conflict/Internal` via `../infra/foundation.yml`; deskripsi 400 menyebut `details.reason` `USER_NOT_FOUND|USER_NOT_DEVELOPER|USER_INACTIVE`, 409 `ALREADY_MEMBER`, 404 juga untuk project yang bukan milik pemanggil). Bila ada selisih dengan bagian "Request/Response body" di bawah, perbaiki di sana, jalankan `make api-bundle`, pastikan `make api-check` lulus.

**B8. Verifikasi akhir**: `go vet ./...`, `go test ./internal/domain/project/...`, `go test -tags integration ./internal/domain/project/...`, archtest `TestT00_AC4_RepoIsClean` (`addmember` hanya meng-import `base`, `base/store`, `base/users`, `base/audit`, infra, lib; tidak meng-import feature lain).

## Migration

Tidak ada di usecase ini. Memakai tabel `projects` dan `project_members` dari `backend/migrations/project/20261022090000000_create_projects.sql` (F overview bagian 6). Bergantung pada PK `(project_id, user_id)` (dasar `ON CONFLICT`) dan CHECK `project_members_role_check` sebagai jaring pengaman.

## Changes

Baru:
- `backend/internal/domain/project/internal/management/addmember/init.go`
- `.../addmember/handler.go`
- `.../addmember/params.go`
- `.../addmember/usecase.go`
- `.../addmember/repository.go`
- `.../addmember/params_test.go`
- `.../addmember/usecase_test.go` (unit, repo/users/audit palsu)
- `.../addmember/handler_test.go` (httptest, usecase dengan palsu)
- `.../addmember/repository_integration_test.go` (tag `integration`, `db.TestDB(t)`)

Ubah:
- `backend/internal/domain/project/internal/management/base.go`: tambah `addmember.Init(d)`.
- `api/modules/project/management.yml` hanya bila verifikasi B7 menemukan selisih (lalu `api/openapi.yml` regenerasi).

Tidak diubah: `lib/constants/bus.go`, `lib/contracts/**`, migrasi, `internal/infra/**`, `go.mod`, `base/` dan subpackage-nya (semua dari F), usecase lain.

## Test Scenario

Penamaan: `TestT03_AC<n>_...` untuk kriteria penerimaan; lainnya `Test<Fungsi>_<Kasus>`.

Params (`params_test.go`):
- `user_id` kosong / spasi saja -> `fields.user_id = "wajib diisi"`; `" abc "` -> di-trim `abc`; `role` tidak divalidasi di sini.

Unit usecase (`usecase_test.go`; repo palsu pencatat urutan panggilan dan menyimpan state anggota, users palsu, audit palsu pencatat event, clock tetap):
- Sukses sebagai owner: urutan panggilan `FindAccess` -> `users.Get` -> `InTx{LockProject, LockMembers, InsertMember}` -> `audit.Record`; `users.Get` TIDAK dipanggil di dalam `InTx` dan audit TIDAK dipanggil di dalam `InTx` (pencatat menandai flag `inTx`). Member yang di-insert: `Role=member` (default), `UserID=target.ID`, `AddedBy=actor.ID`, `AddedByLabel=email aktor`, `AddedAt=clock`. DTO: `found=true`, `is_active=true`, name/email dari get-user.
- `role: "owner"` oleh owner -> member di-insert dengan role owner.
- `TestT03_AC7_AdminAssignsOwnerOnOwnerlessProject`: aktor super_admin (bukan anggota), project tanpa anggota; `role: owner` -> sukses, role owner, `AddedBy` = admin; tidak ada rule ownerCount yang menolak.
- `TestT03_AC2_MemberCannotAdd`: aktor developer berstatus member -> `ErrForbidden`; `users.Get` tidak dipanggil, `InTx` tidak dipanggil, audit 0.
- Aktor bukan anggota -> `ErrProjectNotFound`; `users.Get` tidak dipanggil. Project tidak ada / id bukan uuid -> idem (repo palsu mengembalikan `found=false`).
- `TestT03_AC4_*` (unit): get-user `Found=false` -> `INVALID_ARGUMENT` `details.reason=USER_NOT_FOUND`; role super_admin aktif -> `USER_NOT_DEVELOPER`; super_admin nonaktif -> `USER_NOT_DEVELOPER`; developer nonaktif -> `USER_INACTIVE`. Di semua kasus `InTx` tidak dipanggil dan audit 0.
- Role `"admin"` / `"OWNER"` -> `INVALID_ARGUMENT` `fields.role`; `users.Get` tidak dipanggil (validasi sebelum bus).
- Role `""` -> member.
- Target sudah anggota (terdeteksi di daftar terkunci) -> `ErrAlreadyMember`; `InsertMember` tidak dipanggil; audit 0.
- `InsertMember` mengembalikan `inserted=false` (ON CONFLICT) -> `ErrAlreadyMember`; audit 0.
- Hitung ulang level di dalam tx: pre-check lolos (owner) tetapi daftar terkunci menunjukkan aktor sudah `member` (disimulasikan repo palsu) -> `ErrForbidden`; tidak ada insert, audit 0. Aktor sudah tidak ada di daftar terkunci -> `ErrForbidden`/404 sesuai `Require` (developer tanpa keanggotaan -> `none` -> `ErrProjectNotFound`).
- `LockProject` ok=false (project dihapus antara pre-check dan lock) -> `ErrProjectNotFound`.
- Super_admin aktor tetap lolos hitung ulang meski tidak ada di daftar anggota.
- Error `users.Get` (termasuk `bus.ErrNotRegistered`) -> error terbungkus (bukan `*errs.Error`), `errors.Is(err, bus.ErrNotRegistered)` benar, `InTx` tidak dipanggil, audit 0.
- Error `LockMembers`/`InsertMember` -> terbungkus; audit 0.
- `user_id` dari klien huruf besar, get-user mengembalikan `ID` huruf kecil -> yang di-insert dan di Target audit adalah `target.ID`.
- Audit: tepat 1 event `Action=project.member.added`, `ProjectID`, `ProjectLabel=nama project dari baris terkunci`, `TargetType="project_member"`, `Target=target.ID`, `Args=={user_id, user_label: email target, role}`, `Meta` diteruskan.
- Aktor tanpa identitas di ctx -> `CodeUnauthorized`.

Handler (`handler_test.go`, httptest):
- 201 + JSON tepat sesuai contoh (snake_case, waktu RFC3339 UTC).
- Body kosong / JSON rusak / field tak dikenal (`{"userId":"x"}`) / `user_id` bertipe angka -> 400; `user_id` kosong -> 400 `details.fields.user_id`.
- `ErrForbidden` -> 403; `ErrProjectNotFound` -> 404; `ErrAlreadyMember` -> 409 `details.reason=ALREADY_MEMBER`; `UserNotEligible(...)` -> 400 `details.reason`; error biasa -> 500 generik tanpa bocor pesan internal; `trace_id` ada di semua error.
- `projectID` dari `chi.URLParam` diteruskan apa adanya (test dengan router chi sungguhan); `UserAgent` > 512 rune dipotong; `IP` dari `KeyByIP`.

Repository integrasi (`repository_integration_test.go`, Postgres nyata):
- `InsertMember` sukses: baris ada dengan `added_by`, `added_by_label`, `added_at` benar; `inserted=true`.
- Insert kedua untuk `(project, user)` sama -> `inserted=false`, tidak error, baris tetap satu (ON CONFLICT).
- Role `"admin"` melanggar CHECK -> error DB (jaring pengaman); tx rollback.
- Project tidak ada -> pelanggaran FK (`project_id`), tidak berpura-pura sukses.
- `LockProject` pada tx pertama memblokir `addmember` tx kedua sampai commit (dua goroutine): tx kedua melihat anggota baru dan menghasilkan `ErrAlreadyMember`.

Integrasi lintas feature (ditambahkan ke `management_integration_test.go` oleh W, tag `integration`; usecase ini menyumbang kasus berikut):
- `TestT03_AC4_RejectIneligibleCandidates`: fixture A (owner), D (developer nonaktif), S (super_admin), B (developer aktif). A tambah D -> 400 `USER_INACTIVE`; tambah S -> 400 `USER_NOT_DEVELOPER`; tambah uuid acak -> 400 `USER_NOT_FOUND`; tambah `"bukan-uuid"` -> 400 `USER_NOT_FOUND`; tambah B -> 201; tambah B lagi -> 409 `ALREADY_MEMBER`. Setelah semua itu `project_members` hanya berisi A dan B; audit hanya 1 entri `project.member.added`.
- `TestT03_AC2_MemberCannotAddMembers` (bagian add): B (member) `POST members` (C) -> 403; tidak ada baris baru; audit 0. C (bukan anggota) `POST members` -> 404 dengan body identik 404 "project tidak ditemukan" (tidak bocor).
- `TestT03_AC7_AdminAssignsOwner`: S `POST /api/projects` (project tanpa owner, `member_count=0`), lalu S `POST members {user_id: A, role: owner}` -> 201 `role=owner`; A kini `GET /api/projects/{id}` -> `my_role=owner`, `owner_count=1`.
- `TestT03_AC5_AddedMemberAccess`: setelah B ditambah, `check-access` B -> `{true, member}`.
- `TestT03_Audit_MemberAdded`: tepat satu entri `project.member.added` dengan `actor_label` email aktor, `project_label` nama project, `target_type=project_member`, `target=user B`, `args.role`; audit handler error -> request tetap 201 (fail-open) dan error ter-log tanpa args; request gagal -> 0 entri.
- `TestT03_AddMember_ConcurrentSameTarget`: dua goroutine A dan A2 (owner) menambah B bersamaan, 20 putaran dengan project baru -> tiap putaran tepat satu 201 dan satu 409 `ALREADY_MEMBER`, satu baris anggota, satu entri audit.
- `TestT03_AddMember_ActorDemotedRace`: A dan C owner; A menurunkan C ke member bersamaan dengan C menambah B. Hasil serial: bila demote lebih dulu, C mendapat 403 dan B tidak ditambahkan; bila add lebih dulu, keduanya sukses. Tidak ada keadaan di mana C (sudah member) berhasil menambah.
- Keamanan (rule 13.6): B (bukan anggota) `POST members` ke project PA -> 404 seragam, tidak mengubah data, dan fake get-user TIDAK dipanggil (hitung panggilan 0).
- CSRF: tanpa `X-Requested-With` -> 403 `CSRF_REQUIRED`; tanpa sesi -> 401.
- Kebocoran: respons 201 tidak memuat field di luar `MemberDTO`.
- `TestRoutesRegistered` (W) memuat `POST /api/projects/{projectID}/members`.

## E2E Scenarios

Positif (Playwright terhadap binary; butuh T01 dan T02 merge, UI dari frontend-plan):
1. Owner A membuka tab Members -> "Tambah anggota" -> mengetik "bud" -> autocomplete (`member-candidates`) menampilkan developer aktif yang belum anggota -> pilih Budi, peran default Member -> "Tambah" -> Budi muncul di daftar sebagai member dengan "ditambahkan oleh" A.
2. Owner A menambah Citra dengan memilih peran Owner -> Citra muncul sebagai owner.
3. Budi login -> beranda menampilkan project dengan peran member (AC2).
4. Super Admin membuka project tanpa owner (dibuat sendiri) -> tab Members kosong -> tambah Andi dengan peran Owner -> Andi tampil sebagai owner (AC7).
5. Audit log (Super Admin) memuat `project.member.added` dengan nama project, email aktor, dan id target.

Negatif:
1. Menambah user yang sudah anggota (lewat API langsung, karena autocomplete menyembunyikannya) -> 409 `ALREADY_MEMBER`; UI menampilkan pesan i18n `project.error.ALREADY_MEMBER`.
2. Menambah user nonaktif (API langsung) -> 400 `USER_INACTIVE`; menambah Super Admin -> 400 `USER_NOT_DEVELOPER`; user_id acak / bukan uuid -> 400 `USER_NOT_FOUND`. UI menampilkan pesan i18n sesuai `details.reason` (AC4).
3. Member (Budi) membuka tab Members -> tombol "Tambah anggota" tidak tampil; lewat API langsung -> 403.
4. Developer bukan anggota memanggil `POST /api/projects/<id>/members` langsung -> 404 (tidak bocor).
5. `role: "admin"` atau `"Owner"` lewat API langsung -> 400 `fields.role`.
6. `user_id` kosong -> 400 `fields.user_id`; field tak dikenal -> 400.
7. Tanpa header `X-Requested-With` -> 403 `CSRF_REQUIRED`; tanpa login -> 401.
8. Klik "Tambah" dua kali cepat: UI menonaktifkan tombol saat request berjalan; bila tetap dua request, yang kedua 409 dan hanya satu anggota/entri audit.
9. Dua owner menambah developer yang sama bersamaan -> satu sukses, satu 409; daftar anggota tidak duplikat.

## Request/Response body

Format error seragam T00:

```json
{ "error": { "code": "NOT_FOUND", "message": "project tidak ditemukan", "details": null }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

### POST /api/projects/{projectID}/members

Header: `X-Requested-With: grimoire`, `Content-Type: application/json`, cookie sesi.

Request (`role` opsional, default `member`):
```json
{ "user_id": "0192f5a0-cccc-7aaa-8bbb-00000000000c", "role": "member" }
```

Request minimal:
```json
{ "user_id": "0192f5a0-cccc-7aaa-8bbb-00000000000c" }
```

Request menunjuk owner (Super Admin pada project tanpa owner, AC7):
```json
{ "user_id": "0192f5a0-aaaa-7aaa-8bbb-00000000000a", "role": "owner" }
```

201 (`MemberDTO`):
```json
{
  "user_id": "0192f5a0-cccc-7aaa-8bbb-00000000000c",
  "name": "Citra",
  "email": "c@tim.id",
  "is_active": true,
  "found": true,
  "role": "member",
  "added_at": "2026-10-22T09:10:00Z",
  "added_by": "0192f5a0-aaaa-7aaa-8bbb-00000000000a",
  "added_by_label": "a@tim.id"
}
```

400, user nonaktif (AC4):
```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "user nonaktif tidak bisa ditambahkan sebagai anggota", "details": { "reason": "USER_INACTIVE" } }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

400, bukan developer (Super Admin) (AC4):
```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "hanya developer yang bisa ditambahkan sebagai anggota; Super Admin tidak perlu menjadi anggota", "details": { "reason": "USER_NOT_DEVELOPER" } }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

400, user tidak ditemukan atau `user_id` bukan uuid (AC4):
```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "user tidak ditemukan", "details": { "reason": "USER_NOT_FOUND" } }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

400, `user_id` kosong:
```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "permintaan tidak valid", "details": { "fields": { "user_id": "wajib diisi" } } }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

400, role tidak valid:
```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "permintaan tidak valid", "details": { "fields": { "role": "role harus owner atau member" } } }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

400, field tak dikenal (`{"userId": "x"}`) atau JSON rusak: format error 400 dari `respond.DecodeJSON` (`code: INVALID_ARGUMENT`).

401 (tanpa sesi):
```json
{ "error": { "code": "UNAUTHENTICATED", "message": "perlu login", "details": null }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

403, aktor member (bukan owner/admin):
```json
{ "error": { "code": "FORBIDDEN", "message": "akses ditolak", "details": null }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

403, header CSRF hilang (dari middleware T00; test mengecek `code`, bukan teks pesan):
```json
{ "error": { "code": "CSRF_REQUIRED", "message": "header X-Requested-With wajib", "details": null }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

404, project tidak ada, bukan uuid, atau aktor bukan anggota (seragam, tidak bocor):
```json
{ "error": { "code": "NOT_FOUND", "message": "project tidak ditemukan", "details": null }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

409, sudah anggota:
```json
{ "error": { "code": "CONFLICT", "message": "user sudah menjadi anggota", "details": { "reason": "ALREADY_MEMBER" } }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

500 (termasuk `core.auth.get-user` gagal; pesan generik, detail hanya di log):
```json
{ "error": { "code": "INTERNAL", "message": "terjadi kesalahan", "details": null }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

### Bus yang dipanggil (Go, bukan wire JSON)

```go
u, err := bus.CallTyped[getuser.Response](ctx, d.Bus, constants.BUS_CORE_AUTH_GET_USER,
    getuser.Request{UserID: "0192f5a0-cccc-7aaa-8bbb-00000000000c"})
// u == getuser.Response{Found: true, ID: "0192f5a0-cccc-7aaa-8bbb-00000000000c", Name: "Citra",
//      Email: "c@tim.id", Role: "developer", IsActive: true}
```

### Audit yang dikirim (`core.audit.record`, fail-open, setelah commit)

```go
record.Request{
    ActorType: record.ActorUser, ActorID: "0192f5a0-aaaa-7aaa-8bbb-00000000000a", ActorLabel: "a@tim.id",
    ProjectID: "0192f5a0-1111-7aaa-8bbb-000000000001", ProjectLabel: "Payments",
    Action: record.ActionProjectMemberAdded, TargetType: "project_member", Target: "0192f5a0-cccc-7aaa-8bbb-00000000000c",
    Args: map[string]any{"user_id": "0192f5a0-cccc-7aaa-8bbb-00000000000c", "user_label": "c@tim.id", "role": "member"},
    Status: record.StatusOK, IP: "10.0.0.5", UserAgent: "Mozilla/5.0", TraceID: "01JB2X9K6Q4Y7V3N8M5T2R1C0A",
}
```
