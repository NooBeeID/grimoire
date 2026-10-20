# T03 project/management — Usecase `removemember`

Konteks bersama (migrasi, konstanta bus, kontrak, `base/`, `base/store`, `base/audit`, `base/users`, OpenAPI, i18n) ada di `overview.md` dan dikerjakan di langkah foundation F. Dokumen ini hanya memuat usecase `removemember`. Bila bertentangan dengan `overview.md`, overview menang. Tiket: `docs/tickets/T03-project-management.md`. Kriteria: AC2 (member tidak boleh mengeluarkan anggota lain), AC3 (owner terakhir tidak bisa keluar sendiri; Super Admin bisa).

## 1. Metadata

| Atribut | Nilai |
|---|---|
| Scope | fullstack (backend di dokumen ini; tombol "Keluarkan"/"Keluar" di tab Members dijelaskan di `frontend-plan.md`) |
| Domain | project |
| Module | management |
| Feature | `removemember` |
| Resource root | `backend/internal/domain/project/internal/management` |
| allowedPaths | `backend/internal/domain/project/internal/management/removemember/`, baris `removemember.Init(d)` di `.../management/base.go` |

| Endpoint | Method | Version | Akses |
|---|---|---|---|
| `/api/projects/{projectID}/members/{userID}` | DELETE | tanpa prefix versi (prefix `/api`; `info.version` 0.1.0) | owner dan super_admin (mengeluarkan siapa pun); anggota mana pun untuk dirinya sendiri (keluar). Member mengeluarkan orang lain: 403. Bukan anggota, project tidak ada, atau `projectID` bukan uuid: 404 |

Header wajib: `X-Requested-With: grimoire` (CSRF T00), sesi login (`RequireAuth`). Tanpa request body. Respons sukses 204 tanpa body.

Kontrak bus yang dipakai (nama dan tipe persis plan pemilik, tidak didefinisikan ulang):

| Endpoint | Mode | Provider | Dipakai untuk |
|---|---|---|---|
| `core.auth.get-user` (`constants.BUS_CORE_AUTH_GET_USER`, kontrak `lib/contracts/core/auth/getuser`: `Request{UserID}` -> `Response{Found, ID, Name, Email, Role, IsActive}`) | Call, fail-soft | T01 | label `user_label` di audit, via `base/users.Lookup.Get`, SETELAH commit |
| `core.audit.record` (`constants.BUS_CORE_AUDIT_RECORD`, `record.Request`, `record.ActionProjectMemberRemoved`) | Call, fail-open | T02 | audit `project.member.removed`, via `base/audit` |

Tidak ada endpoint bus baru. Tidak memanggil `get-user` untuk aktor (overview D6). Tidak memanggil bus di dalam transaksi (overview 3.5).

## 2. Summary Plan

`removemember` mengeluarkan satu anggota dari project, baik owner/Super Admin mengeluarkan anggota, maupun anggota keluar sendiri (`userID == aktor`). Satu usecase, tidak dipecah.

Alur usecase (orkestrasi saja; rule ada di `base.CheckRemove`, `base.ResolveLevel`, `base.Require`, `base.OwnerCount`, `base.FindMember`):

1. Bangun aktor dari `reqctx`.
2. Pre-check di luar tx: `FindAccess(projectID, actor.ID)` -> `!found` -> 404; `level = ResolveLevel(actor.Role, memberRole, true)`; `Require(level, PermView)` (level `none` -> 404 seragam, tidak bocor). Sengaja BUKAN `PermManageMembers`: member boleh lolos karena boleh keluar sendiri.
3. Satu transaksi (overview 3.5): `LockProject` (hilang -> 404) -> `LockMembers` -> hitung ulang level aktor dari baris terkunci (`memberRole` = role aktor di daftar terkunci, "" bila tidak ada) lalu `Require(level, PermView)` lagi -> cari target di daftar terkunci (`FindMember`); tidak ada -> `ErrMemberNotFound` (404) -> `CheckRemove(actor.ID, level, target, OwnerCount(members))` -> `DELETE`.
4. Setelah commit: `Get(userID)` untuk label (fail-soft: error -> label `""`), lalu audit `project.member.removed` (fail-open). Respons 204.

Keputusan khusus usecase ini:

- Urutan cek di dalam tx: visibilitas aktor (404) -> target ada (404 `anggota tidak ditemukan`) -> `CheckRemove` (403/409). Target bukan anggota selalu 404, termasuk `userID` bukan uuid; tidak ada 400 untuk `userID`. Member yang mencoba mengeluarkan user yang bukan anggota mendapat 404 (bukan 403): ia sudah anggota project sehingga tidak ada kebocoran.
- `CheckRemove` memakai level aktor dari baris terkunci, bukan hasil pre-check. Ini yang menjamin race tidak menghasilkan keadaan terlarang: dua owner yang bersamaan keluar/saling mengeluarkan diserialkan oleh `FOR UPDATE` pada baris `projects`; transaksi kedua membaca ulang daftar anggota. Contoh dua owner A dan C keluar bersamaan: yang pertama 204, yang kedua melihat `ownerCount == 1` dan dirinya owner terakhir -> 409 `LAST_OWNER`. Contoh A mengeluarkan C sementara C mengeluarkan A: satu 204, satu 404 (aktor sudah tidak anggota -> level `none`) atau 409; tidak pernah dua-duanya sukses (kecuali Super Admin yang memang boleh membuat project tanpa owner). Tepat satu owner tersisa pada akhirnya (AC3).
- Mengeluarkan diri sendiri oleh aktor yang bukan anggota (Super Admin) mustahil: target tidak ada di daftar -> 404. Super Admin tidak pernah "self".
- Super Admin (level `admin`) boleh mengeluarkan siapa pun, termasuk owner terakhir (project boleh tanpa owner). Audit tetap `self=false`.
- `self` untuk audit = `target.UserID == actor.ID` (dihitung dari baris terkunci).
- `user_label` audit = `Email` hasil `get-user` target. Dipanggil sesudah commit karena aksi sudah terjadi; kegagalan (termasuk `bus.ErrNotRegistered`, atau `Found=false`) TIDAK menggagalkan request: label `""` dan log `Warn`. Berbeda dengan `listmembers` yang 500 karena data itu isi respons utama.
- No-op tidak ada: DELETE selalu mengubah satu baris, jadi selalu tepat satu entri audit saat sukses; 0 entri pada semua jalur gagal (rollback).
- Setelah anggota dihapus, data milik user di project (mis. API key yang ia buat di T04) tidak disentuh. Hanya baris `project_members` yang dihapus.
- Tidak ada validasi uuid di handler (overview 3.3). Usecase hanya menormalkan `userID` dengan `uuid.Parse(...).String()` sebelum tx supaya huruf besar uuid tetap cocok dengan baris DB; gagal parse -> `ErrMemberNotFound` (setelah pre-check visibilitas, agar non-anggota tetap 404 project).

Prasyarat (dari F; bila belum ada hentikan dan selesaikan F dulu, jangan membuatnya dari folder `removemember/`): migrasi `projects`/`project_members`; konstanta bus T03 dan konsumsi; paket `base` (`Actor`, `ActorFrom`, `RequestMeta`, `Member`, `FindMember`, `OwnerCount`, `ResolveLevel`, `Require`, `PermView`, `CheckRemove`, `ErrProjectNotFound`, `ErrMemberNotFound`, `ErrForbidden`, `ErrLastOwner`, `RoleOwner`); `base/store` (`FindAccess`, `LockProject`, `LockMembers`, `WithTx`); `base/audit`; `base/users`; entri OpenAPI `removemember` di `management.yml`; kontrak `record` dan konstanta `ActionProjectMemberRemoved` (T02); kontrak `getuser` (T01).

## 3. Plan (langkah berurutan)

Semua file di `backend/internal/domain/project/internal/management/removemember/`. Urutan TDD: tulis tes dulu per langkah.

1. **Verifikasi prasyarat**: `cd backend && go build ./...` lulus; `grep -n "removemember" ../api/modules/project/management.yml` menunjukkan `DELETE /api/projects/{projectID}/members/{userID}` dengan `operationId: removemember`, parameter `XRequestedWith`, `ProjectID`, `UserID`, respons `204`, error `401/403/404/409/500`. Selisih dengan bagian 8 diperbaiki di `management.yml` di PR ini, lalu `make api-bundle`, commit `api/openapi.yml`, `make api-check` lulus (rule 11.6: spec sebelum handler). Respons 400 hanya untuk kegagalan lain dari middleware; usecase tidak menghasilkan 400.

2. **`usecase.go`**:
   ```go
   // Sisi consumer (rule 5.6). Repo konkret memenuhi keduanya.
   type Repository interface {
       FindAccess(ctx context.Context, projectID, userID string) (p base.Project, memberRole string, found bool, err error)
       InTx(ctx context.Context, fn func(TxRepository) error) error
   }
   type TxRepository interface {
       LockProject(ctx context.Context, id string) (base.Project, bool, error)
       LockMembers(ctx context.Context, projectID string) ([]base.Member, error)
       DeleteMember(ctx context.Context, projectID, userID string) error
   }

   type Input struct {
       ProjectID string
       UserID    string
       Meta      base.RequestMeta
   }

   type Usecase struct {
       repo  Repository
       users users.Lookup
       audit audit.Recorder
       log   *slog.Logger
   }
   func NewUsecase(repo Repository, u users.Lookup, a audit.Recorder, log *slog.Logger) *Usecase
   func (u *Usecase) Execute(ctx context.Context, in Input) error
   ```
   Alur `Execute`:
   1. `actor, ok := base.ActorFrom(reqctx.IdentityFrom(ctx))`; `!ok` -> `errs.New(errs.CodeUnauthorized, "perlu login")`.
   2. `_, memberRole, found, err := u.repo.FindAccess(ctx, in.ProjectID, actor.ID)`; `err` -> `fmt.Errorf("removemember: cari akses: %w", err)`; `!found` -> `base.ErrProjectNotFound`.
   3. `level := base.ResolveLevel(actor.Role, memberRole, true)`; `if err := base.Require(level, base.PermView); err != nil { return err }`.
   4. `targetID, err := uuid.Parse(in.UserID)`; err -> `base.ErrMemberNotFound`. `target := targetID.String()`.
   5. ```go
      var removed base.Member
      var project base.Project
      var self bool
      err = u.repo.InTx(ctx, func(tx TxRepository) error {
          p, ok, err := tx.LockProject(ctx, in.ProjectID)
          if err != nil { return fmt.Errorf("removemember: kunci project: %w", err) }
          if !ok { return base.ErrProjectNotFound }
          members, err := tx.LockMembers(ctx, p.ID)
          if err != nil { return fmt.Errorf("removemember: kunci anggota: %w", err) }

          myRole := ""
          if me, ok := base.FindMember(members, actor.ID); ok { myRole = me.Role }
          lvl := base.ResolveLevel(actor.Role, myRole, true)
          if err := base.Require(lvl, base.PermView); err != nil { return err } // dikeluarkan di antara pre-check dan lock -> 404

          t, ok := base.FindMember(members, target)
          if !ok { return base.ErrMemberNotFound }
          if err := base.CheckRemove(actor.ID, lvl, t, base.OwnerCount(members)); err != nil { return err }

          if err := tx.DeleteMember(ctx, p.ID, t.UserID); err != nil {
              return fmt.Errorf("removemember: hapus anggota: %w", err)
          }
          project, removed, self = p, t, t.UserID == actor.ID
          return nil
      })
      if err != nil { return err }
      ```
      `*errs.Error` dikembalikan apa adanya (tidak di-wrap) agar `respond.WriteError` memetakan 403/404/409 dengan benar. `base.ErrMemberNotFound` dari `DeleteMember` (0 baris, jaring pengaman) juga dikembalikan apa adanya.
   6. Setelah commit, label (fail-soft):
      ```go
      label := ""
      if res, err := u.users.Get(ctx, removed.UserID); err != nil {
          u.log.WarnContext(ctx, "removemember: get-user gagal, label audit kosong", "project_id", project.ID, "user_id", removed.UserID, "error", err)
      } else if res.Found {
          label = res.Email
      }
      ```
   7. `u.audit.Record(ctx, audit.Event{Action: record.ActionProjectMemberRemoved, ProjectID: project.ID, ProjectLabel: project.Name, TargetType: "project_member", Target: removed.UserID, Args: map[string]any{"user_id": removed.UserID, "user_label": label, "role": removed.Role, "self": self}, Meta: in.Meta})`. `Record` tidak mengembalikan error (fail-open). Dipanggil di luar tx. `return nil`.

   Usecase tidak menulis SQL, tidak memanggil `bus` langsung, dan tidak memuat rule (semua rule dari `base`).

3. **`repository.go`** (satu-satunya yang menyentuh SQL di feature ini):
   ```go
   type Repo struct { db *db.DB; s *store.Store }
   func newRepo(d *db.DB, s *store.Store) *Repo
   func (r *Repo) FindAccess(...) // delegasi r.s.FindAccess
   func (r *Repo) InTx(ctx context.Context, fn func(TxRepository) error) error {
       return r.db.BeginTx(ctx, func(tx *sqlx.Tx) error {
           return fn(&txRepo{s: r.s.WithTx(tx), q: tx})
       })
   }
   type txRepo struct { s *store.Store; q sqlx.ExtContext }
   func (t *txRepo) LockProject(...)  // delegasi t.s.LockProject
   func (t *txRepo) LockMembers(...)  // delegasi t.s.LockMembers
   func (t *txRepo) DeleteMember(ctx context.Context, projectID, userID string) error
   ```
   SQL tunggal `DeleteMember` (parameterized, rule 5.3):
   ```sql
   DELETE FROM project_members WHERE project_id = $1 AND user_id = $2
   ```
   `RowsAffected() == 0` -> `base.ErrMemberNotFound` (tidak mungkin di bawah lock; jaring pengaman). Error lain dikembalikan mentah.

4. **`handler.go`**:
   ```go
   type handler struct{ uc *Usecase }
   func (h handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
       err := h.uc.Execute(r.Context(), Input{
           ProjectID: chi.URLParam(r, "projectID"),
           UserID:    chi.URLParam(r, "userID"),
           Meta:      base.RequestMeta{IP: middleware.KeyByIP(r), UserAgent: truncate(r.UserAgent(), 512)},
       })
       if err != nil { respond.WriteError(w, r, err); return }
       w.WriteHeader(http.StatusNoContent)
   }
   ```
   `truncate` (potong ke 512 byte aman UTF-8) helper privat di `handler.go`. Handler tidak membaca body, tidak memvalidasi uuid, tidak me-log apa pun. Tidak ada `params.go` (tanpa body; param route dibaca di handler, sesuai pola overview 3.3).

5. **`init.go`**:
   ```go
   func Init(d app.Deps) {
       s := store.New(d.DB)
       uc := NewUsecase(newRepo(d.DB, s), users.New(d.Bus), audit.New(d.Bus, d.Log), d.Log)
       d.API.With(middleware.RequireAuth()).Delete("/projects/{projectID}/members/{userID}", handler{uc: uc}.ServeHTTP)
   }
   ```
   Pola penuh langsung di `d.API`, nama param `{projectID}` dan `{userID}` (overview D4). Tanpa `Route`/`Mount`.

6. **Wiring**: tambah satu baris `removemember.Init(d)` di `management/base.go` (PR usecase ini). Jangan menyentuh baris `Init` feature lain.

7. **Tes** sesuai bagian 6, urutan: unit usecase (RED -> hijau), handler, repository integrasi, integrasi feature.

8. **Verifikasi**: `cd backend && go build ./... && go vet ./... && go test ./internal/domain/project/internal/management/removemember/...`; dengan `TEST_DATABASE_URL`: `go test -tags integration ./internal/domain/project/internal/management/removemember/...`; `make api-check`; archtest `TestT00_AC4_RepoIsClean` lulus (tidak ada import antar-feature, SQL hanya di `repository.go`).

Kontrak bus (ringkas, sisi consumer):

| Aspek | Nilai |
|---|---|
| Urutan Call | `get-user` (target, setelah commit) lalu `audit.Record`; maksimal 1 + 1 Call per request, 0 pada jalur gagal sebelum commit |
| `get-user` | `bus.CallTyped[getuser.Response](ctx, b, constants.BUS_CORE_AUTH_GET_USER, getuser.Request{UserID: target})` lewat `users.BusLookup`; error/`Found=false` -> label `""` |
| Audit | `bus.CallTyped[record.Response](ctx, b, constants.BUS_CORE_AUDIT_RECORD, record.Request{...})` lewat `audit.BusRecorder`; error di-log (`audit record gagal`) dan DITELAN |
| `record.Request` | `ActorType: record.ActorUser`, `ActorID` = user id aktor, `ActorLabel` = email aktor, `ProjectID`, `ProjectLabel` = nama project saat aksi, `Action: record.ActionProjectMemberRemoved` (`project.member.removed`), `TargetType: "project_member"`, `Target` = user id yang dikeluarkan, `Args {user_id, user_label, role, self}`, `Status: record.StatusOK`, `IP`, `UserAgent`, `TraceID` (diisi `audit.BusRecorder`) |

## 4. Migration

Tidak ada. Tabel `projects` dan `project_members` dibuat migrasi foundation `backend/migrations/project/20261022090000000_create_projects.sql` (overview bagian 6). DELETE berdasarkan PK `(project_id, user_id)`; tidak perlu kolom atau indeks baru.

## 5. Changes (per file/folder)

| File | Aksi | Isi |
|---|---|---|
| `backend/internal/domain/project/internal/management/removemember/init.go` | buat | rakit store -> repo -> usecase -> handler; `d.API.With(RequireAuth()).Delete("/projects/{projectID}/members/{userID}", ...)` |
| `.../removemember/handler.go` | buat | baca param route, bangun `Input`/`RequestMeta`, 204 / `WriteError`, helper `truncate` |
| `.../removemember/usecase.go` | buat | interface `Repository`/`TxRepository`, `Input`, `Usecase.Execute` |
| `.../removemember/repository.go` | buat | `Repo` (delegasi ke `store`), `InTx`, `txRepo.DeleteMember` (satu DELETE) |
| `.../removemember/usecase_test.go` | buat | unit dengan repo/users/audit palsu |
| `.../removemember/handler_test.go` | buat | `httptest`, usecase di balik repo palsu |
| `.../removemember/repository_test.go` | buat | integrasi (tag `integration`, `db.TestDB`) |
| `.../removemember/removemember_integration_test.go` | buat | integrasi HTTP penuh (tag `integration`) |
| `backend/internal/domain/project/internal/management/base.go` | ubah | tambah `removemember.Init(d)` |
| `api/modules/project/management.yml`, `api/openapi.yml` | verifikasi, ubah hanya bila selisih | operasi `removemember` (bagian 8) |
| `frontend/src/domain/project/management/members/` | lihat `frontend-plan.md` | tombol "Keluarkan" (owner/admin) dan "Keluar" (semua anggota), dialog konfirmasi, pesan `project.error.LAST_OWNER`; di luar cakupan backend plan ini |

Tidak diubah: `base/` (bila rule `CheckRemove` perlu berubah, ubah di `base/` beserta tesnya, bukan di usecase), `base/store`, `infra/**`, feature lain, `go.mod`, konfigurasi, `docs/tickets/**`.

## 6. Test Scenario

Penamaan: tes kriteria `TestT03_ACn_*`; lainnya `Test<Unit>_<Kasus>`. Tabel rule `CheckRemove`/`ResolveLevel`/`Require` sudah dites di `base/` (F); tes di sini membuktikan orkestrasi, urutan cek, dan wiring, tanpa menduplikasi tabel rule.

### Unit usecase (`usecase_test.go`; repo, users, audit palsu)

Repo palsu mencatat urutan panggilan, menyimpan daftar anggota terkunci, dan bisa dipaksa error per metode. Users palsu bisa dipaksa error / `Found=false`. Audit palsu menyimpan `Event`. Identitas disuntik lewat `reqctx.WithIdentity`.

- `TestRemove_OwnerRemovesMember`: A owner, B member -> `DeleteMember(project, B)` dipanggil sekali; audit 1 event dengan `Args{user_id:B, user_label:"b@tim.id", role:"member", self:false}`.
- `TestT03_AC2_MemberCannotRemoveOthers`: B member mengeluarkan C (member) -> `errors.Is(err, base.ErrForbidden)`; `DeleteMember` tidak dipanggil; audit 0; `get-user` tidak dipanggil.
- `TestRemove_MemberLeavesSelf`: B member, target B -> sukses; `self:true`; `DeleteMember(project, B)`.
- `TestRemove_OwnerLeavesWhenAnotherOwner`: A dan C owner, A keluar sendiri -> sukses, `self:true`, `role:"owner"`.
- `TestT03_AC3_LastOwnerCannotLeave`: A satu-satunya owner (ada member B), target A -> `errors.Is(err, base.ErrLastOwner)` (409, `details.reason == "LAST_OWNER"`); tidak ada DELETE, tidak ada audit.
- `TestT03_AC3_SuperAdminRemovesLastOwner`: aktor `super_admin` (bukan anggota), target A owner terakhir -> sukses; `self:false`; project sekarang tanpa owner.
- `TestRemove_OwnerRemovesOtherOwner`: A dan C owner, A mengeluarkan C -> sukses (`ownerCount=2`).
- `TestRemove_SuperAdminRemovesMember`: S mengeluarkan B -> sukses, `self:false`.
- `TestRemove_SuperAdminCannotSelfLeave`: S dengan `userID=S.ID` -> `ErrMemberNotFound` (S bukan anggota); tidak ada DELETE.
- `TestRemove_TargetNotMember404`: owner A dengan target uuid valid yang bukan anggota -> `errors.Is(err, base.ErrMemberNotFound)` (404, pesan `anggota tidak ditemukan`); audit 0; `get-user` tidak dipanggil.
- `TestRemove_TargetNotUUID404`: `userID == "bukan-uuid"` -> `ErrMemberNotFound`; `InTx` tidak dipanggil.
- `TestRemove_TargetUppercaseUUID`: `userID` uuid huruf besar milik B -> dinormalkan, B terhapus.
- `TestRemove_MemberRemovingNonMemberGets404`: B member mengeluarkan uuid non-anggota -> `ErrMemberNotFound` (bukan 403).
- `TestRemove_NonMemberActorGets404`: developer C bukan anggota -> `ErrProjectNotFound`; `InTx`, `get-user`, audit tidak dipanggil. Berlaku juga bila target bukan uuid atau bukan anggota (tetap `ErrProjectNotFound`, bukan `ErrMemberNotFound`).
- `TestRemove_ProjectNotFound`: `FindAccess found=false` -> `ErrProjectNotFound`, tanpa tx.
- `TestRemove_DeletedBetweenCheckAndLock`: `FindAccess` found, `LockProject ok=false` -> `ErrProjectNotFound`; tidak ada DELETE; audit 0.
- `TestRemove_ActorRemovedBetweenCheckAndLock`: pre-check B member, tetapi daftar terkunci tidak memuat B -> `ErrProjectNotFound`; tidak ada DELETE.
- `TestRemove_ActorDemotedBetweenCheckAndLock`: pre-check A owner, daftar terkunci A sudah `member`, target C -> `ErrForbidden` (level dihitung ulang dari baris terkunci); tidak ada DELETE.
- `TestRemove_LevelRecomputedFromLockedRows`: pre-check `memberRole="owner"` tetapi data terkunci menunjukkan A satu-satunya owner dan target A -> `ErrLastOwner`.
- `TestRemove_GetUserFailureDoesNotFail`: `users.Get` error (dan variasi `bus.ErrNotRegistered`, `Found=false`) -> `Execute` nil; audit tetap 1 event dengan `user_label == ""`; log Warn tercatat.
- `TestRemove_AuditAfterTx`: urutan panggilan palsu = `FindAccess, InTx(begin), LockProject, LockMembers, DeleteMember, InTx(commit), users.Get, audit.Record`. Tidak ada `users.Get`/`audit.Record` di dalam fungsi tx.
- `TestRemove_AuditPayload`: `ProjectLabel == project.Name` (snapshot), `TargetType == "project_member"`, `Target == userID target`, `Meta` sama dengan input, `Args` hanya 4 kunci (`user_id`, `user_label`, `role`, `self`).
- `TestRemove_TxErrorNoAudit`: `DeleteMember`/`LockMembers`/`LockProject` error -> error terbungkus (`errors.Is` ke error asal), audit 0, `get-user` tidak dipanggil.
- `TestRemove_NoIdentity`: konteks tanpa identitas -> `CodeUnauthorized`, tanpa panggilan repo.

### Unit handler (`handler_test.go`)

- 204 dengan body kosong (tanpa `Content-Type` JSON wajib).
- `chi.URLParam("projectID")` dan `("userID")` diteruskan apa adanya, termasuk `"bukan-uuid"` (handler tidak memvalidasi).
- `RequestMeta.UserAgent` dipotong 512 byte, tetap UTF-8 valid; `IP` dari `middleware.KeyByIP`.
- Body request (kalau ada) diabaikan, tidak memengaruhi hasil.
- Mapping error: `ErrProjectNotFound` -> 404 `NOT_FOUND`; `ErrMemberNotFound` -> 404 `NOT_FOUND` (pesan `anggota tidak ditemukan`); `ErrForbidden` -> 403 `FORBIDDEN`; `ErrLastOwner` -> 409 `CONFLICT` dengan `details.reason == "LAST_OWNER"`; error infra -> 500 `INTERNAL` tanpa detail internal.

### Repository (`repository_test.go`, tag `integration`, Postgres nyata via `db.TestDB`)

- `DeleteMember` menghapus tepat satu baris `(project_id, user_id)`; baris anggota lain dan baris `projects` tidak berubah.
- `DeleteMember` pada pasangan yang tidak ada -> `ErrMemberNotFound`.
- `InTx`: error dari `fn` -> rollback (anggota tetap ada); sukses -> commit.
- Penguncian: tx A `LockProject` lalu tidur; tx B `LockProject` pada project yang sama memblokir sampai A commit, lalu melihat daftar anggota hasil A via `LockMembers` (dua goroutine, kanal sinkron, batas waktu tes 5 detik).
- SQL terparameter: `userID` berisi `'); DROP TABLE project_members;--` tidak merusak (usecase menolak lebih dulu, repo dites langsung: 0 baris terhapus, tabel utuh).

### Integrasi feature (`removemember_integration_test.go`, tag `integration`)

Bus nyata (`bus.NewRegistry`), `db.TestDB`, router `chi` dengan middleware uji yang menyuntik identitas, handler palsu `core.auth.get-user` (A, B, C developer aktif; S super_admin) dan `core.audit.record` (menyimpan `record.Request`). Project dan anggota dibuat langsung lewat SQL/`store` agar tes tidak bergantung pada feature lain.

- `TestT03_AC2_MemberCannotRemoveOthers_HTTP`: A owner, B dan C member. B `DELETE members/C` -> 403 `FORBIDDEN`; C tetap anggota; 0 audit.
- `TestT03_AC2_MemberCanLeave`: B `DELETE members/B` -> 204; `SELECT` DB: B bukan anggota; `GET /projects/{id}` oleh B -> 404; 1 audit dengan `self:true`.
- `TestT03_AC3_LastOwnerCannotLeave_HTTP`: A satu-satunya owner `DELETE members/A` -> 409 `LAST_OWNER`; A tetap owner; 0 audit.
- `TestT03_AC3_SuperAdminCanRemoveLastOwner`: S `DELETE members/A` -> 204; `owner_count` project 0 (via SQL); project tetap ada; 1 audit `self:false`.
- `TestT03_AC3_ConcurrentOwnersLeave`: A dan C owner (project baru tiap putaran); dua goroutine bersamaan `DELETE members/A` oleh A dan `DELETE members/C` oleh C; diulang 20 kali -> tiap putaran tepat satu 204 dan satu 409 `LAST_OWNER`, tepat satu owner tersisa, tepat 1 audit.
- `TestT03_AC3_ConcurrentMutualRemoval`: A dan C owner; A `DELETE members/C` bersamaan dengan C `DELETE members/A`; 20 putaran -> tidak pernah dua 204 (pasangan hasil salah satu: 204 + 404, atau 204 + 409); minimal satu anggota tersisa, tepat satu owner tersisa; jumlah audit == jumlah 204.
- `TestRemove_OwnerRemovesOwnerWhenTwo`: A `DELETE members/C` (C owner, ada owner lain) -> 204.
- `TestRemove_TargetNotMember404`: A `DELETE members/<uuid acak>` -> 404 `anggota tidak ditemukan`; `DELETE members/bukan-uuid` -> 404 identik; 0 audit.
- `TestT03_Security_NonMemberCannotRemove` (rule 13.6): C developer non-anggota `DELETE members/B` -> 404 `project tidak ditemukan`, byte-per-byte identik (kecuali `trace_id`) dengan respons untuk project uuid acak dan untuk `projectID` `"bukan-uuid"`; juga identik untuk target uuid acak/bukan-uuid; B tetap anggota; 0 audit. Pesan 404 project berbeda dari 404 anggota (`anggota tidak ditemukan`) hanya muncul untuk aktor yang memang anggota/admin.
- `TestRemove_AuditPayload`: A mengeluarkan B -> `record.Request`: `Action == "project.member.removed"`, `ActorType == "user"`, `ActorID == A`, `ActorLabel == "a@tim.id"`, `ProjectID`, `ProjectLabel == "Payments"`, `TargetType == "project_member"`, `Target == B`, `Args == {user_id: B, user_label: "b@tim.id", role: "member", self: false}`, `Status == "ok"`, `IP`, `UserAgent`, `TraceID` tidak kosong.
- `TestRemove_AuditFailOpen`: handler `core.audit.record` error, dan skenario endpoint tidak terdaftar -> DELETE tetap 204, anggota terhapus, log error `audit record gagal` tercatat, tanpa isi body di log.
- `TestRemove_GetUserFailSoft`: `core.auth.get-user` tidak terdaftar / error -> DELETE tetap 204; audit terkirim dengan `user_label == ""`.
- `TestRemove_DeactivatedTargetCanBeRemoved`: B nonaktif (get-user palsu `IsActive=false`) tetap bisa dikeluarkan owner -> 204, `user_label` tetap terisi.
- `TestRemove_ProjectSurvivesWithoutMembers`: A owner sekaligus satu-satunya anggota; S mengeluarkan A -> `projects` tetap ada, `project_members` kosong untuk project itu.
- `TestRemove_RaceWithDeleteProject`: `DELETE members/B` bersamaan dengan penghapusan project (SQL langsung) -> 204 atau 404, tidak pernah 500; tidak ada baris yatim.
- `TestRemove_CSRFAndAuth`: tanpa `X-Requested-With` -> 403 `CSRF_REQUIRED`; tanpa sesi -> 401; keduanya sebelum usecase, 0 perubahan DB.
- `TestRemove_RouteRegistered`: `chi.Walk` memuat `DELETE /api/projects/{projectID}/members/{userID}` dan `PATCH` pada pola yang sama (changememberrole) tanpa bentrok.

## 7. E2E Scenarios (Playwright terhadap binary; butuh T01 dan T02 merge, UI dari `frontend-plan.md`)

Positif:
1. Owner A membuka tab Members project "Payments" -> menekan "Keluarkan" pada member B -> dialog konfirmasi -> B hilang dari tabel. Super Admin membuka audit log -> entri `project.member.removed` dengan aktor A, `user_label` b@tim.id, `self: false`.
2. Member B menekan "Keluar" -> konfirmasi -> diarahkan ke beranda, "Payments" tidak lagi tampil; audit memuat `self: true`.
3. Dua owner A dan C: A menekan "Keluar" -> sukses; C kini satu-satunya owner.
4. Super Admin membuka project yang ia bukan anggotanya -> mengeluarkan owner terakhir -> sukses, tab Members kosong, Super Admin dapat menunjuk owner baru lewat tambah anggota.

Negatif:
1. Owner terakhir A menekan "Keluar" -> pesan i18n `project.error.LAST_OWNER`; A tetap owner.
2. Member B hanya melihat tombol "Keluar" pada barisnya sendiri (tidak ada "Keluarkan"); lewat API langsung `DELETE members/C` -> 403 dan UI menampilkan pesan akses ditolak.
3. Developer C membuka `/projects/<id>` project lain -> "tidak ditemukan"; `DELETE` via API -> 404 yang sama dengan id acak.
4. `DELETE` ke `userID` yang bukan anggota (via API) -> 404 `anggota tidak ditemukan`; UI menyegarkan daftar anggota.
5. Dua tab: tab 1 mengeluarkan B, tab 2 menekan "Keluarkan" untuk B yang sama -> 404 dan UI menyegarkan daftar tanpa error mentah.
6. Dua owner menekan "Keluar" hampir bersamaan di dua browser -> satu sukses, satu mendapat `LAST_OWNER`.
7. `DELETE` tanpa header `X-Requested-With` -> 403 `CSRF_REQUIRED`; tanpa login -> 401.

## 8. Request/Response body

Format error seragam T00:

```json
{ "error": { "code": "NOT_FOUND", "message": "project tidak ditemukan", "details": null }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

### DELETE /api/projects/{projectID}/members/{userID}

Header: `X-Requested-With: grimoire`. Tanpa request body. `userID` adalah id user yang dikeluarkan; bila sama dengan id pemanggil berarti keluar sendiri.

Contoh: `DELETE /api/projects/0192f5a0-1111-7aaa-8bbb-000000000001/members/0192f5a0-bbbb-7aaa-8bbb-00000000000b`

204 (dikeluarkan atau keluar sendiri): tanpa body.

401 (tanpa sesi): `{ "error": { "code": "UNAUTHENTICATED", ... } }` sesuai middleware T00.

403 (member mengeluarkan orang lain):
```json
{ "error": { "code": "FORBIDDEN", "message": "akses ditolak", "details": null }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

403 (tanpa header CSRF): `{ "error": { "code": "CSRF_REQUIRED", ... } }` dari middleware T00.

404 (pemanggil bukan anggota, project tidak ada, atau `projectID` bukan uuid; respons identik):
```json
{ "error": { "code": "NOT_FOUND", "message": "project tidak ditemukan", "details": null }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

404 (target bukan anggota, atau `userID` bukan uuid; hanya untuk pemanggil yang boleh melihat project):
```json
{ "error": { "code": "NOT_FOUND", "message": "anggota tidak ditemukan", "details": null }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

409 (owner terakhir keluar sendiri; Super Admin tidak terkena):
```json
{
  "error": {
    "code": "CONFLICT",
    "message": "owner terakhir tidak bisa keluar atau diturunkan; angkat owner lain lebih dulu",
    "details": { "reason": "LAST_OWNER" }
  },
  "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A"
}
```

500: `{ "error": { "code": "INTERNAL", "message": "terjadi kesalahan", "details": null }, "trace_id": "..." }` (pesan generik sesuai T00; detail hanya di log server).

Tidak ada respons 400 dari usecase ini.

### Entri audit yang dikirim (`record.Request`, contoh B keluar sendiri)

```go
record.Request{
    ActorType: record.ActorUser, ActorID: "0192f5a0-bbbb-7aaa-8bbb-00000000000b", ActorLabel: "b@tim.id",
    ProjectID: "0192f5a0-1111-7aaa-8bbb-000000000001", ProjectLabel: "Payments",
    Action: record.ActionProjectMemberRemoved, TargetType: "project_member", Target: "0192f5a0-bbbb-7aaa-8bbb-00000000000b",
    Args: map[string]any{"user_id": "0192f5a0-bbbb-7aaa-8bbb-00000000000b", "user_label": "b@tim.id", "role": "member", "self": true},
    Status: record.StatusOK, IP: "10.0.0.5", UserAgent: "Mozilla/5.0", TraceID: "01JB2X9K6Q4Y7V3N8M5T2R1C0A",
}
```

Contoh owner A mengeluarkan B: `ActorID`/`ActorLabel` = A, `Target` = B, `Args {"user_id": B, "user_label": "b@tim.id", "role": "member", "self": false}`. Bila `get-user` gagal: `"user_label": ""`. Hanya satu entri per DELETE sukses, nol entri bila gagal.

### Skema OpenAPI operasi (`api/modules/project/management.yml`, harus ada sejak F)

- `DELETE /api/projects/{projectID}/members/{userID}`, `operationId: removemember`, `tags: [project-members]`.
- Parameter: `$ref` `XRequestedWith` (header, wajib), `ProjectID` (path, `format: uuid`), `UserID` (path, `format: uuid`).
- Tanpa request body.
- Respons: `204` (tanpa konten); `401` Unauthorized; `403` Forbidden (member mengeluarkan orang lain, atau CSRF); `404` NotFound (deskripsi: "project bukan milik pemanggil / tidak ada (tidak bocor), atau target bukan anggota"); `409` Conflict (`details.reason: LAST_OWNER`); `500` Internal. Semua `$ref: ../infra/foundation.yml#/components/responses/...`.
