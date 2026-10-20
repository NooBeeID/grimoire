# T03 project/management — Usecase `deleteproject` (`DELETE /api/projects/{projectID}`)

Melengkapi `docs/superpowers/plans/project-management/overview.md` (F: `base.Project.ConfirmDeletion`, `base.Require`, `base.PermDelete`, `base/store`, `base/audit`, `base/counts`, migrasi `projects`/`project_members`, OpenAPI `management.yml`). Bila dokumen ini bertentangan dengan overview, overview menang. Hanya satu usecase: `deleteproject` (U7, gelombang B). Prasyarat F harus sudah ada; bila ada simbol F yang belum ada, hentikan dan laporkan (jangan membuatnya di luar allowedPaths).

## Metadata

| Field | Nilai |
|---|---|
| Ticket | T03 |
| Scope | fullstack pada tiket; plan usecase ini backend. UI zona bahaya "Hapus project" dirinci di `frontend-plan.md` |
| Domain | project |
| Module | management |
| Feature | `deleteproject` (gelombang B, U7) |
| Endpoint | `/api/projects/{projectID}` |
| Method | DELETE (body JSON `{"confirm_name": "<nama project>"}`) |
| Version | Tanpa prefix versi di URL (prefix `/api`); `management.yml` `info.version: 0.1.0` |
| Auth | `middleware.RequireAuth()` + CSRF (`X-Requested-With: grimoire`). Otorisasi project di usecase: `base.Require(level, base.PermDelete)` (owner, admin ok; member 403; bukan anggota/tidak ada/bukan uuid 404) |
| Respons sukses | 204 tanpa body |
| Audit | `project.deleted` via bus `core.audit.record` setelah commit, fail-open |
| Bus dikonsumsi | `core.audit.record` (T02, lewat `base/audit`), `project.knowledge.count-files`, `project.apispec.count-specs`, `project.apikey.count-keys` (fail-soft, lewat `base/counts`). TIDAK memakai `core.auth.get-user` (D6) |
| Bus disediakan | tidak ada |
| Tabel | menulis `projects` (DELETE), cascade ke `project_members` dan tabel T04-T06; membaca `project_members` |
| Resource root | `backend/internal/domain/project/internal/management` |
| allowedPaths | `backend/internal/domain/project/internal/management/deleteproject/`, `backend/internal/domain/project/internal/management/base.go` (satu baris `deleteproject.Init(d)`), `api/modules/project/management.yml` hanya bila operasi `deleteproject` belum lengkap dari F, lalu `make api-bundle` |

## Summary Plan

`deleteproject` menghapus project secara permanen beserta data turunannya setelah pemanggil mengetik nama project persis.

Aturan:
- Urutan keputusan (jangan diubah, menjaga rule 7.3 tidak bocor):
  1. Decode body dan validasi bentuk (`confirm_name` wajib ada). Ini tidak bergantung pada project, jadi tidak membocorkan apa pun.
  2. `FindAccess` -> tidak ada / bukan uuid -> 404; `ResolveLevel`; `Require(PermDelete)`: bukan anggota 404, member 403.
  3. `Project.ConfirmDeletion(confirm)` terhadap nama dari DB: tidak cocok -> 400 `INVALID_ARGUMENT` `details.reason = CONFIRMATION_MISMATCH`. Hanya pemanggil yang sudah berhak yang sampai ke langkah ini, jadi nama project tidak bocor lewat perbedaan respons.
  4. `counts.ForProjects([id])` SEBELUM tx (bus tidak pernah di dalam tx, overview 3.5). Fail-soft: field yang tidak tersedia `nil` dan masuk audit sebagai `null`.
  5. Tx (`InTx` di repository): `LockProject` (FOR UPDATE; hilang -> 404) -> verifikasi ULANG `ConfirmDeletion` terhadap nama dari baris terkunci (nama bisa berubah oleh `updateproject` paralel; tanpa ini nama lama bisa menghapus project yang sudah diganti namanya) -> `LockMembers` -> hitung ulang level aktor dari baris terkunci (`ResolveLevel(actor.Role, memberRole, true)`) dan `Require(PermDelete)` lagi (aktor bisa dikeluarkan/diturunkan sementara) -> `member_count = len(members)` -> `DeleteProject`.
  6. Setelah commit: kirim audit `project.deleted`. Gagal audit hanya di-log (fail-open), request tetap 204.
- Cascade: `project_members` dan tabel T04-T06 memakai `REFERENCES projects(id) ON DELETE CASCADE`; usecase hanya `DELETE FROM projects WHERE id = $1`. Tidak ada DELETE manual per tabel (rule: kopling DB hanya lewat FK cascade). Keamanannya dibuktikan test `pg_constraint` (AC6).
- `confirm_name` dibandingkan persis oleh entity: tanpa trim, case-sensitive (overview 4.1). Nama yang disimpan sudah ter-trim.
- Snapshot audit: `ProjectLabel` = nama dari baris terkunci; `Args = {name, member_count, counts:{files,specs,keys}}`. Entri audit tetap ada setelah project hilang (T02 tanpa FK).
- Satu aksi = tepat satu entri audit. Penghapusan gagal (400/403/404/500) = 0 entri.
- Dua DELETE bersamaan: yang kedua menunggu lock, lalu `LockProject` tidak menemukan baris -> 404; tepat satu 204 dan tepat satu audit.
- Tidak ada soft delete, tidak ada undo, tidak ada event/job lain.
- Tidak memodifikasi sentinel `*errs.Error` milik `base`.

## Plan

Urutan wajib; tulis test dulu (TDD), lalu implementasi.

**D0. Verifikasi (tanpa edit).** Pastikan ada: `base.Project`+`ConfirmDeletion`, `base.ActorFrom`, `base.ResolveLevel`, `base.Require`, `base.PermDelete`, `base.FindMember`, `base.RequestMeta`, `base.ContentCounts`, `base.ErrProjectNotFound`, `base.ErrConfirmationMismatch`, `base.InvalidFields`, `store.Store` (`FindAccess`, `LockProject`, `LockMembers`, `WithTx`), `audit.Recorder`/`audit.Event`/`audit.New`, `counts.Counter`/`counts.New`, `record.ActionProjectDeleted`, tabel `projects`/`project_members`, `management.Register`. Baca `internal/infra/db` (`DB.BeginTx(ctx, func(*sqlx.Tx) error)`, `TestDB`), `respond.DecodeJSON` (body kosong -> 400 "body wajib diisi"; field tak dikenal -> 400), `internal/infra/middleware` (`RequireAuth`, `KeyByIP`). Bila `updateproject` (U6) sudah ada, ikuti pola `init.go`/`repository.go`-nya persis (pola `InTx`).

**D1. `deleteproject/params.go`** (validasi bentuk saja)

```go
package deleteproject

// Request adalah body DELETE. Pointer agar "field hilang" dibedakan dari "string kosong".
type Request struct {
    ConfirmName *string `json:"confirm_name"`
}

// Validate: ConfirmName == nil -> base.InvalidFields{"confirm_name": "wajib diisi"} (400).
// String kosong lolos ke entity dan menghasilkan CONFIRMATION_MISMATCH (nama project minimal 2 karakter, tidak pernah cocok).
func (r Request) Validate() error
```

**D2. `deleteproject/usecase.go`** (orkestrasi saja, rule 5.2 dan 5.6)

```go
package deleteproject

type txRepo interface {
    LockProject(ctx context.Context, id string) (base.Project, bool, error)
    LockMembers(ctx context.Context, projectID string) ([]base.Member, error)
    DeleteProject(ctx context.Context, id string) (bool, error) // true bila 1 baris terhapus
}

type repository interface {
    FindAccess(ctx context.Context, projectID, userID string) (p base.Project, memberRole string, found bool, err error)
    InTx(ctx context.Context, fn func(txRepo) error) error
}

type Input struct {
    ProjectID   string
    ConfirmName string
    Meta        base.RequestMeta
}

type Usecase struct {
    repo   repository
    counts counts.Counter
    audit  audit.Recorder
}

func NewUsecase(r repository, c counts.Counter, a audit.Recorder) *Usecase

func (u *Usecase) Execute(ctx context.Context, in Input) error
```

Alur `Execute` (tepat urutan ini):

1. `actor, ok := base.ActorFrom(reqctx.IdentityFrom(ctx))`; `!ok` -> `errs.New(errs.CodeUnauthorized, "perlu login")`.
2. `p, memberRole, found, err := u.repo.FindAccess(ctx, in.ProjectID, actor.ID)`; error -> `fmt.Errorf("deleteproject: find access: %w", err)`; `!found` -> `base.ErrProjectNotFound`.
3. `level := base.ResolveLevel(actor.Role, memberRole, true)`; `base.Require(level, base.PermDelete)` -> kembalikan error apa adanya.
4. `p.ConfirmDeletion(in.ConfirmName)` -> kembalikan error (400).
5. `cc := u.counts.ForProjects(ctx, []string{p.ID})[p.ID]` (zero `ContentCounts` bila tidak ada entri; tidak pernah error).
6. `var memberCount int; var name string`; `err = u.repo.InTx(ctx, func(tx txRepo) error { ... })` dengan isi:
   - `locked, ok, err := tx.LockProject(ctx, p.ID)`; error -> bungkus `deleteproject: lock project: %w`; `!ok` -> `base.ErrProjectNotFound`.
   - `locked.ConfirmDeletion(in.ConfirmName)` -> return error (nama berubah di antara pre-check dan lock).
   - `ms, err := tx.LockMembers(ctx, p.ID)`; bungkus error.
   - `m, _ := base.FindMember(ms, actor.ID)` (role "" bila bukan anggota); `base.Require(base.ResolveLevel(actor.Role, m.Role, true), base.PermDelete)` -> return error.
   - `memberCount = len(ms)`; `name = locked.Name`.
   - `deleted, err := tx.DeleteProject(ctx, p.ID)`; bungkus `deleteproject: delete: %w`; `!deleted` -> `base.ErrProjectNotFound`.
7. Error dari `InTx` (termasuk begin/commit) dikembalikan; bila bukan `*errs.Error`, dibungkus `deleteproject: tx: %w` (500 generik). Tidak ada audit bila error.
8. Setelah commit sukses: `u.audit.Record(ctx, audit.Event{Action: record.ActionProjectDeleted, ProjectID: p.ID, ProjectLabel: name, TargetType: "project", Target: p.ID, Args: deletedArgs(name, memberCount, cc), Meta: in.Meta})`.
9. `return nil`.

Helper privat `deletedArgs(name string, memberCount int, cc base.ContentCounts) map[string]any` menghasilkan `{"name": name, "member_count": memberCount, "counts": map[string]any{"files": cc.Files, "specs": cc.Specs, "keys": cc.Keys}}` dengan tiap nilai `*int` di-deref menjadi `int` atau `nil` (JSON `null`) agar tidak ada pointer di Args. Hanya nama, jumlah, dan counts; deskripsi, anggota, dan id user TIDAK masuk audit.

Catatan: tidak ada `time.Now` di usecase ini, jadi tidak butuh `base.Clock`. `ConfirmDeletion` tidak punya efek samping.

**D3. `deleteproject/repository.go`** (satu-satunya yang menyentuh SQL lewat store; query khusus feature ada di sini)

```go
type Repo struct {
    db *db.DB
    s  *store.Store
}

func newRepo(d *db.DB, s *store.Store) *Repo

func (r *Repo) FindAccess(ctx, projectID, userID string) (base.Project, string, bool, error) { return r.s.FindAccess(ctx, projectID, userID) }

func (r *Repo) InTx(ctx context.Context, fn func(txRepo) error) error {
    return r.db.BeginTx(ctx, func(tx *sqlx.Tx) error {
        return fn(&txRepoImpl{s: r.s.WithTx(tx), q: tx})
    })
}

type txRepoImpl struct{ s *store.Store; q sqlx.ExecerContext }
func (t *txRepoImpl) LockProject(ctx, id) (base.Project, bool, error) { return t.s.LockProject(ctx, id) }
func (t *txRepoImpl) LockMembers(ctx, pid) ([]base.Member, error)     { return t.s.LockMembers(ctx, pid) }
// DeleteProject: id bukan uuid -> (false, nil) tanpa query.
// SQL: DELETE FROM projects WHERE id = $1  -> RowsAffected() == 1
func (t *txRepoImpl) DeleteProject(ctx, id string) (bool, error)
```

`DeleteProject` tidak ada di `base/store` (hanya dipakai satu feature); letakkan di repository feature. Tidak ada DELETE manual ke `project_members` maupun tabel T04-T06 (cascade).

**D4. `deleteproject/handler.go`**

```go
type executor interface{ Execute(ctx context.Context, in Input) error }

type Handler struct{ uc executor }

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    var req Request
    if err := respond.DecodeJSON(r, &req); err != nil { respond.WriteError(w, r, err); return }
    if err := req.Validate(); err != nil { respond.WriteError(w, r, err); return }
    err := h.uc.Execute(r.Context(), Input{
        ProjectID:   chi.URLParam(r, "projectID"),
        ConfirmName: *req.ConfirmName,
        Meta:        base.RequestMeta{IP: middleware.KeyByIP(r), UserAgent: truncate(r.UserAgent(), 512)},
    })
    if err != nil { respond.WriteError(w, r, err); return }
    w.WriteHeader(http.StatusNoContent)
}
```

`projectID` diteruskan apa adanya (tanpa validasi uuid di handler; store menjawab "tidak ketemu" -> 404). `truncate` privat di handler (potong per rune, bukan per byte).

**D5. `deleteproject/init.go`**

```go
func Init(d app.Deps) {
    s := store.New(d.DB)
    uc := NewUsecase(newRepo(d.DB, s), counts.New(d.Bus, d.Log), audit.New(d.Bus, d.Log))
    h := &Handler{uc: uc}
    d.API.With(middleware.RequireAuth()).Delete("/projects/{projectID}", h.ServeHTTP)
}
```

Pola route penuh langsung di `d.API` (D4 overview), nama param `{projectID}`.

**D6. Wiring.** Tambah satu baris `deleteproject.Init(d)` di `management/base.go` (`Register`), tanpa mengubah baris lain.

**D7. OpenAPI.** Pastikan operasi `deleteProject` ada di `api/modules/project/management.yml` (dari F): path `/api/projects/{projectID}`, `parameters`: `XRequestedWith`, `ProjectID`; `requestBody` required `DeleteProjectRequest` (`confirm_name` string wajib); respons `204`, `400 BadRequest` (menyebut `details.reason = CONFIRMATION_MISMATCH`), `401`, `403`, `404` ("juga untuk project yang bukan milik pemanggil"), `500`. Bila diubah, `make api-bundle` dan `make api-check` lulus. Jangan mengubah operasi lain.

**D8. Test** (bagian Test Scenario), lalu verifikasi akhir: `go vet ./...`; archtest `TestT00_AC4_RepoIsClean` (folder `deleteproject` hanya meng-import `base`, `base/store`, `base/audit`, `base/counts`, `lib/contracts/core/audit/record`, `lib/constants`, `lib/common/{errs,reqctx}`, `infra/{app,db,middleware}`, `infra/http/respond`; tidak meng-import feature lain, rule 4.3); `go test -race ./internal/domain/project/...`; `go test -tags integration ./internal/domain/project/...`; `make api-check`.

## Migration

Tidak ada migrasi baru. Memakai tabel dari F: `backend/migrations/project/20261022090000000_create_projects.sql`. Kontrak yang diwajibkan ke tabel T04-T06 (`project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE`) diverifikasi test AC6 di bawah; test itu otomatis mencakup tabel T04-T06 begitu migrasinya ada di `backend/migrations/**`.

## Changes

Backend (baru):
- `backend/internal/domain/project/internal/management/deleteproject/init.go`
- `backend/internal/domain/project/internal/management/deleteproject/handler.go`
- `backend/internal/domain/project/internal/management/deleteproject/params.go`
- `backend/internal/domain/project/internal/management/deleteproject/usecase.go`
- `backend/internal/domain/project/internal/management/deleteproject/repository.go`
- `backend/internal/domain/project/internal/management/deleteproject/params_test.go`
- `backend/internal/domain/project/internal/management/deleteproject/usecase_test.go`
- `backend/internal/domain/project/internal/management/deleteproject/handler_test.go`
- `backend/internal/domain/project/internal/management/deleteproject/integration_test.go` (tag `integration`)

Backend (ubah, minimal):
- `backend/internal/domain/project/internal/management/base.go`: satu baris `deleteproject.Init(d)`.

API (ubah hanya bila belum lengkap dari F): `api/modules/project/management.yml`, `api/openapi.yml` (generated).

Frontend: tidak diubah di plan ini (zona bahaya hapus di `frontend-plan.md`, feature `deleteproject` FE).

Tidak dibuat: migrasi, kode error baru, env var, endpoint bus, perubahan `base/` (semua rule sudah di F), perubahan infra.

Catatan koordinasi: test `TestT03_AC6_*` dimiliki usecase ini (file integrasi di folder `deleteproject`). `management_integration_test.go` (W) cukup tidak menduplikasinya; bila W sudah memilikinya, salah satu dihapus agar tidak ada dua sumber kebenaran.

## Test Scenario

Penamaan: `TestT03_ACn_<Deskripsi>` untuk AC tiket; lainnya `Test<Fungsi>_<Kasus>`. Fake repository/counts/audit dibuat di `usecase_test.go` (fake menerapkan interface `repository`, `txRepo`, `counts.Counter`, `audit.Recorder`); tidak ada mock framework.

### Unit `params.go`
- `TestRequest_Validate_MissingField`: `{}` -> `fields.confirm_name = "wajib diisi"`.
- `TestRequest_Validate_EmptyStringPasses`: `{"confirm_name": ""}` lolos validasi bentuk.
- Decode lewat `respond.DecodeJSON`: body kosong -> 400; JSON rusak -> 400; field tak dikenal (`{"name":"x"}`) -> 400; tipe salah (`{"confirm_name": 5}`) -> 400.

### Unit usecase (fake)
- `TestExecute_Success_Owner`: owner + nama cocok: `DeleteProject` dipanggil sekali; audit dikirim tepat sekali dengan `Action=project.deleted`, `ProjectID`, `ProjectLabel=nama`, `TargetType=project`, `Target=id`, `Args.name`, `Args.member_count` (= jumlah anggota terkunci), `Args.counts`.
- `TestExecute_Success_SuperAdminNotMember`: aktor super_admin tanpa baris anggota -> sukses (level admin), `member_count` sesuai.
- `TestExecute_MemberForbidden`: member -> `base.ErrForbidden`; `ForProjects`, `InTx`, `DeleteProject`, audit tidak dipanggil.
- `TestExecute_NonMemberNotFound`: bukan anggota -> `base.ErrProjectNotFound`; tidak ada efek samping.
- `TestExecute_UnknownProject`: `FindAccess` found=false -> `ErrProjectNotFound`.
- `TestExecute_ConfirmationMismatch`: nama beda huruf besar, spasi tambahan, kosong -> `ErrConfirmationMismatch`; `ForProjects`/`InTx`/audit tidak dipanggil.
- `TestExecute_MemberWithWrongNameGets403NotMismatch`: member + nama salah -> 403 (otorisasi sebelum konfirmasi, tidak membocorkan nama).
- `TestExecute_NameChangedBeforeLock`: pre-check cocok, tetapi `LockProject` mengembalikan nama baru -> `ErrConfirmationMismatch`; `DeleteProject` tidak dipanggil; audit 0.
- `TestExecute_ActorDemotedBeforeLock`: pre-check owner, tetapi `LockMembers` memuat aktor sebagai member -> `ErrForbidden`; tidak ada delete/audit.
- `TestExecute_ActorRemovedBeforeLock`: aktor developer tidak ada lagi di `LockMembers` -> `ErrProjectNotFound`.
- `TestExecute_ProjectGoneBeforeLock`: `LockProject` found=false -> `ErrProjectNotFound`; `DeleteProject` `false` -> `ErrProjectNotFound`; audit 0.
- `TestExecute_CountsFailSoft`: counter mengembalikan semua `nil` -> sukses; `Args.counts` = `{files:nil,specs:nil,keys:nil}`; counter dipanggil SEBELUM `InTx` (urutan dicatat fake); counter tidak pernah mengembalikan error.
- `TestExecute_CountsPassedThrough`: files=3, specs=1, keys=nil -> `Args.counts = {files:3, specs:1, keys:nil}`.
- `TestExecute_RepoErrors`: error `FindAccess`, `LockProject`, `LockMembers`, `DeleteProject`, `InTx` masing-masing -> error terbungkus (`errors.Is` ke asal), bukan `*errs.Error` bisnis; audit 0.
- `TestExecute_AuditFailureIsFailOpen`: recorder yang gagal (recorder fake tidak mengembalikan error, karena kontraknya fail-open) tidak mengubah hasil nil; pastikan `Execute` tidak bergantung pada hasil audit.
- `TestExecute_NoIdentity`: konteks tanpa identitas -> `CodeUnauthorized`, tanpa query.
- `TestExecute_AuditNeverContainsDescription`: `Args` hanya berisi kunci `name`, `member_count`, `counts` (tidak ada `description`, `members`, `user_id`).
- `TestExecute_BusNotCalledInsideTx`: fake mencatat flag `inTx`; `counts` dan `audit` dipanggil saat `inTx == false`.

### Handler (`httptest`, usecase fake lewat interface `executor`)
- `TestHandler_204`: sukses -> 204, body kosong; `Input` memuat `projectID` dari chi URL param, `ConfirmName`, `Meta.IP`, `Meta.UserAgent` terpotong 512 rune.
- `TestHandler_BodyErrors`: body kosong/JSON rusak/field tak dikenal/`{}` -> 400 `INVALID_ARGUMENT`; usecase tidak dipanggil.
- `TestHandler_ErrorMapping`: `ErrProjectNotFound` -> 404; `ErrForbidden` -> 403; `ErrConfirmationMismatch` -> 400 dengan `details.reason == "CONFIRMATION_MISMATCH"`; error biasa -> 500 dengan pesan generik (tidak membocorkan error asal).
- `TestHandler_ProjectIDPassedVerbatim`: `projectID` = `bukan-uuid` diteruskan tanpa diubah.

### Integrasi (Postgres nyata `db.TestDB(t)` + migrasi project, bus nyata `bus.NewRegistry`, `deleteproject.Init`, router `chi` + middleware uji yang menyuntik `reqctx.WithIdentity`; project/anggota dibuat dengan SQL fixture, bukan lewat feature lain; handler palsu `core.audit.record` menyimpan semua `record.Request`; handler palsu count didaftarkan per test)
- `TestT03_AC6_DeleteRequiresConfirmationAndCascades`: buat tabel dummy `project_child(id uuid primary key default gen_random_uuid(), project_id uuid not null references projects(id) on delete cascade)` di schema test dan isi 2 baris; project punya 2 anggota. Langkah: (a) `confirm_name` salah -> 400 `CONFIRMATION_MISMATCH`, `projects`/`project_members`/`project_child` utuh, audit 0; (b) body kosong -> 400, data utuh; (c) `confirm_name` benar oleh owner -> 204; ketiga tabel kosong untuk id itu; (d) tepat satu entri audit `project.deleted`: `Args.name`, `Args.member_count == 2`, `Args.counts`, `ProjectID`, `ProjectLabel == nama`, `ActorType=user`, `ActorID`, `IP`, `UserAgent` terisi dari request (header di-set di test).
- `TestT03_AC6_AllProjectFKsCascade`: query `SELECT conrelid::regclass::text, conname, confdeltype FROM pg_constraint WHERE contype = 'f' AND confrelid = 'projects'::regclass`; kumpulan hasil minimal berisi `project_members` dan `project_child`; SEMUA `confdeltype = 'c'`. Pesan gagal menyebut nama tabel dan constraint yang melanggar. Query diekstrak ke helper `nonCascadeFKs(t, db) []string`.
- `TestT03_AC6_FKCheckDetectsViolation`: pada `TestDB` terpisah buat `bad_child(project_id uuid references projects(id))` (tanpa cascade); `nonCascadeFKs` mengembalikan `bad_child` (membuktikan test AC6 bisa gagal). Sekaligus: `DELETE` project yang punya baris `bad_child` mengembalikan error FK dan project tetap ada (dokumentasi perilaku; hanya di test ini).
- `TestT03_AC6_OwnerMemberAdmin`: owner -> 204; member dengan nama benar -> 403 dan data utuh, audit 0; super_admin bukan anggota -> 204 (satu entri audit, `ActorID` = super_admin, `member_count` sesuai).
- `TestDelete_NonMemberAndUnknownIsUniform404`: developer bukan anggota, uuid acak, `bukan-uuid`, `' OR 1=1`, string 500 karakter -> semua 404 dengan body identik (`code=NOT_FOUND`, `message="project tidak ditemukan"`, `details=null`); project nyata tidak berubah; audit 0. Dibandingkan byte-per-byte kecuali `trace_id`.
- `TestDelete_RequiresAuthAndCSRF`: tanpa sesi -> 401; tanpa `X-Requested-With` -> 403 `CSRF_REQUIRED` (router uji memasang middleware CSRF T00); project utuh.
- `TestDelete_OtherProjectsUntouched`: dua project dengan anggota dan `project_child`; hapus satu; yang lain beserta anggota dan child tetap utuh (jumlah baris sama).
- `TestDelete_SecurityIsolation`: A (owner PA) tidak bisa menghapus PB milik C: 404, PB utuh. (rule 13.6)
- `TestDelete_CountsFromProviders`: handler palsu `project.knowledge.count-files` -> `{id: 3}`, `project.apispec.count-specs` -> `{id: 1}`, `project.apikey.count-keys` tidak terdaftar -> audit `Args.counts = {files:3, specs:1, keys:null}`; status 204.
- `TestDelete_CountsAllUnavailable`: tidak ada provider count -> 204, `Args.counts` ketiganya `null`.
- `TestDelete_AuditFailOpen`: handler `core.audit.record` mengembalikan error (dan skenario kedua: endpoint tidak terdaftar) -> tetap 204 dan project terhapus.
- `TestDelete_ConcurrentDeletes`: 5 goroutine DELETE project yang sama dengan nama benar -> tepat satu 204, sisanya 404; tepat satu entri audit; jalankan dengan `-race`.
- `TestDelete_RenameRaceRejectsStaleName`: tx uji meng-`UPDATE projects SET name = 'Baru'` dan menahan lock; goroutine DELETE dengan nama lama lolos pre-check lalu menunggu di `LockProject` (tunggu terdeteksi lewat polling `pg_stat_activity` `wait_event_type = 'Lock'` maks 2 detik); commit tx uji -> DELETE mengembalikan 400 `CONFIRMATION_MISMATCH`; project masih ada dengan nama `Baru`; audit 0. Lalu DELETE dengan `Baru` -> 204.
- `TestDelete_DemotedWhileWaiting`: owner A menunggu di `LockProject` sementara tx uji menurunkan A menjadi member lalu commit -> DELETE A mengembalikan 403; project utuh; audit 0.
- `TestDelete_AuditArgsSnapshot`: nama dengan karakter Unicode/kutip (`Proyek "Ä" 日本`) tersimpan utuh di `ProjectLabel` dan `Args.name`.
- `TestInit_RegistersRoute`: `chi.Walk` memuat `DELETE /projects/{projectID}` (atau pola penuh `/api/projects/{projectID}` sesuai cara router uji dimount) tepat sekali.

### Lintas usecase (dimiliki W, bukan di sini)
- `TestRoutesRegistered` (10 route) dan `TestT03_Audit_OneEntryPerAction` memakai endpoint ini; tidak diulang di sini.

## E2E Scenarios

Playwright terhadap binary; butuh T01 dan T02 merge dan FE `deleteproject` (frontend-plan).

Positif:
1. Owner B membuka Settings -> zona bahaya -> klik "Hapus project" -> dialog; tombol konfirmasi nonaktif sampai nama diketik persis -> klik -> project hilang, redirect ke beranda, daftar tidak memuat project itu; `GET /api/projects/{id}` -> 404.
2. Super Admin (bukan anggota) membuka project -> Settings -> hapus dengan nama benar -> sukses; beranda tidak memuat project itu.
3. Setelah penghapusan, Super Admin membuka audit log -> ada entri `project.deleted` dengan label nama project, `args.name`, `args.member_count`, `args.counts`; entri tetap terbaca padahal project sudah hilang.
4. Project berisi anggota, file knowledge, spec, dan key (setelah T04-T06 ada) -> setelah hapus, semua data turunan hilang (tidak bisa diakses lewat API; verifikasi DB di test integrasi).

Negatif:
1. Mengetik nama dengan huruf besar/kecil berbeda atau spasi di ujung -> tombol tetap nonaktif; lewat API langsung -> 400 `CONFIRMATION_MISMATCH` dan UI (bila dipakai) menampilkan pesan i18n `project.error.CONFIRMATION_MISMATCH`.
2. Member: bagian zona bahaya tidak tampil; lewat API langsung -> 403 `FORBIDDEN`, project tetap ada.
3. Developer bukan anggota membuka `/projects/<id>` -> halaman "tidak ditemukan"; DELETE langsung -> 404 (tidak membocorkan keberadaan).
4. Dua tab menghapus project yang sama berurutan -> yang kedua mendapat 404, UI menampilkan "tidak ditemukan" dan kembali ke beranda.
5. Nama project diganti di tab lain lalu tab pertama menghapus dengan nama lama -> 400 `CONFIRMATION_MISMATCH`; project tetap ada.
6. `DELETE` tanpa header `X-Requested-With` -> 403 `CSRF_REQUIRED`; tanpa login -> 401.
7. `DELETE` tanpa body atau `{}` -> 400 `INVALID_ARGUMENT`.

## Request/Response body

Header: `X-Requested-With: grimoire`, `Content-Type: application/json`, cookie sesi.

Semua error memakai format seragam T00:

```json
{ "error": { "code": "<CODE>", "message": "<pesan>", "details": null }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

### Request

`DELETE /api/projects/0192f5a0-1111-7aaa-8bbb-000000000001`

```json
{ "confirm_name": "Payments v2" }
```

### 204 No Content

Tanpa body.

### 400 nama konfirmasi tidak cocok

```json
{
  "error": {
    "code": "INVALID_ARGUMENT",
    "message": "nama konfirmasi tidak cocok",
    "details": { "reason": "CONFIRMATION_MISMATCH" }
  },
  "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A"
}
```

### 400 field wajib hilang (`{}`)

```json
{
  "error": {
    "code": "INVALID_ARGUMENT",
    "message": "permintaan tidak valid",
    "details": { "fields": { "confirm_name": "wajib diisi" } }
  },
  "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A"
}
```

### 400 body kosong / JSON rusak / field tak dikenal (dari `respond.DecodeJSON`)

```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "body wajib diisi", "details": null }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

(`"JSON tidak valid"` untuk JSON rusak; pesan field tak dikenal mengikuti `DecodeJSON`.)

### 401 tanpa sesi

```json
{ "error": { "code": "UNAUTHENTICATED", "message": "perlu login", "details": null }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

(Kode dan pesan persis mengikuti `RequireAuth` T00.)

### 403 member, atau tanpa header CSRF

```json
{ "error": { "code": "FORBIDDEN", "message": "akses ditolak", "details": null }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

Tanpa `X-Requested-With`: `code` `CSRF_REQUIRED` dari middleware T00.

### 404 bukan anggota, tidak ada, atau id bukan uuid

```json
{ "error": { "code": "NOT_FOUND", "message": "project tidak ditemukan", "details": null }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

### 500

```json
{ "error": { "code": "INTERNAL", "message": "terjadi kesalahan internal", "details": null }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

(Pesan generik; error asal hanya di log. Kode dan pesan persis mengikuti `respond.WriteError` T00.)

### Audit yang dikirim (`record.Request`, via `base/audit`)

```go
record.Request{
    ActorType: record.ActorUser, ActorID: "0192f5a0-aaaa-7aaa-8bbb-00000000000a", ActorLabel: "a@tim.id",
    ProjectID: "0192f5a0-1111-7aaa-8bbb-000000000001", ProjectLabel: "Payments v2",
    Action: record.ActionProjectDeleted, TargetType: "project", Target: "0192f5a0-1111-7aaa-8bbb-000000000001",
    Args: map[string]any{
        "name": "Payments v2", "member_count": 3,
        "counts": map[string]any{"files": 12, "specs": 2, "keys": nil},
    },
    Status: record.StatusOK, IP: "10.0.0.5", UserAgent: "Mozilla/5.0", TraceID: "01JB2X9K6Q4Y7V3N8M5T2R1C0A",
}
```

### Bus count yang dikonsumsi (fail-soft)

```go
bus.CallTyped[contentcount.Response](ctx, d.Bus, constants.BUS_PROJECT_KNOWLEDGE_COUNT_FILES,
    contentcount.Request{ProjectIDs: []string{"0192f5a0-1111-7aaa-8bbb-000000000001"}})
// -> contentcount.Response{Counts: map[string]int{"0192f5a0-1111-7aaa-8bbb-000000000001": 12}}
// bus.ErrNotRegistered / error lain -> field counts terkait nil (Debug/Warn log), penghapusan tetap lanjut.
```

## Definisi selesai

- Semua test di atas hijau (termasuk `-race` dan `-tags integration`); `go vet`, `make api-check`, dan archtest `TestT00_AC4_RepoIsClean` lulus.
- AC6 terbukti: nama konfirmasi salah ditolak tanpa mengubah data; nama benar menghapus project, anggota, dan tabel dummy; semua FK ke `projects(id)` ber-`ON DELETE CASCADE`; tepat satu entri audit `project.deleted` dengan snapshot nama dan hitungan.
- Member selalu 403; bukan anggota/tidak ada/bukan uuid selalu 404 seragam; tidak ada bus call di dalam tx; kegagalan audit atau count tidak menggagalkan penghapusan.
- Tidak ada perubahan di luar allowedPaths selain satu baris di `management/base.go`.
