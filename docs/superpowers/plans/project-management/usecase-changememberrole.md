# T03 project/management — Usecase `changememberrole`

Konteks bersama (migrasi, konstanta bus, kontrak, `base/`, `base/store`, `base/audit`, `base/users`, OpenAPI, i18n) ada di `overview.md` dan dikerjakan di langkah foundation F. Dokumen ini hanya memuat usecase `changememberrole`. Bila bertentangan dengan `overview.md`, overview menang. Tiket: `docs/tickets/T03-project-management.md`. Kriteria: AC3 (owner terakhir tidak bisa diturunkan sendiri; Super Admin bisa), AC2 (member tidak boleh mengelola anggota), AC7 (Super Admin mengelola anggota semua project).

## 1. Metadata

| Atribut | Nilai |
|---|---|
| Scope | fullstack (backend di dokumen ini; dialog/menu ubah peran di tab Members, lihat `frontend-plan.md`) |
| Domain | project |
| Module | management |
| Feature | `changememberrole` |
| Resource root | `backend/internal/domain/project/internal/management` |
| allowedPaths | `backend/internal/domain/project/internal/management/changememberrole/`, baris `changememberrole.Init(d)` di `.../management/base.go` |

| Endpoint | Method | Version | Akses |
|---|---|---|---|
| `/api/projects/{projectID}/members/{userID}` | PATCH | tanpa prefix versi (prefix `/api`; `info.version` 0.1.0) | owner dan super_admin (`Require(PermManageMembers)`); member 403; bukan anggota, project tidak ada, atau `projectID` bukan uuid: 404 |

Header wajib: `X-Requested-With: grimoire` (CSRF T00), `Content-Type: application/json`, sesi login (`RequireAuth`).

Kontrak bus yang dipakai (nama dan tipe persis plan pemilik, tidak didefinisikan ulang):

| Endpoint | Mode | Provider | Dipakai untuk |
|---|---|---|---|
| `core.auth.get-user` (`constants.BUS_CORE_AUTH_GET_USER`; `getuser.Request{UserID}` -> `getuser.Response{Found, ID, Name, Email, Role, IsActive}`) | Call, fail-soft di usecase ini | T01 | label audit (`user_label` = email) dan `MemberDTO` respons, dipanggil SETELAH commit via `base/users` |
| `core.audit.record` (`constants.BUS_CORE_AUDIT_RECORD`; `record.Request` -> `record.Response{Recorded}`; aksi `record.ActionProjectMemberRoleChanged` = `project.member.role_changed`) | Call, fail-open | T02 | audit via `base/audit` |

Tidak ada endpoint bus baru. Tidak memanggil `core.auth.get-user` untuk aktor (overview D6). Tidak memakai `counts`.

## 2. Summary Plan

`changememberrole` mengubah peran seorang anggota antara `owner` dan `member`. Satu usecase, tidak dipecah.

Alur (orkestrasi saja; rule bisnis ada di `base.CheckRoleChange`, `base.ResolveLevel`, `base.Require`, `base.OwnerCount`, `base.FindMember`, `Member.ToDTO`):

1. Bangun aktor dari `reqctx`.
2. Pre-check di luar tx: `FindAccess` -> `ResolveLevel` -> `Require(PermManageMembers)`. Project tidak ada/bukan uuid/aktor bukan anggota -> 404 seragam; member -> 403; owner dan admin lolos. Ini menjawab cepat dan memastikan non-anggota tidak pernah mendapat 400/404-anggota.
3. Satu transaksi (overview 3.5):
   1. `LockProject` (`FOR UPDATE`; hilang -> `ErrProjectNotFound`).
   2. `LockMembers` (`FOR UPDATE`, `ORDER BY user_id`).
   3. Hitung ulang level aktor dari daftar terkunci: `memberRole` aktor = role di daftar ("" bila tidak ada), `level = ResolveLevel(actor.Role, memberRole, true)`, lalu `Require(PermManageMembers)` lagi. Inilah yang membuat race dua owner saling menurunkan menghasilkan tepat satu sukses: tx kedua mendapati aktornya sudah `member` -> 403.
   4. `target, ok := base.FindMember(members, in.UserID)`; `!ok` -> `base.ErrMemberNotFound` (404 "anggota tidak ditemukan"). Dicek SETELAH otorisasi sehingga hanya owner/admin yang dapat mengetahui bahwa user bukan anggota.
   5. `base.CheckRoleChange(actor.ID, level, target, in.Role, base.OwnerCount(members))` (role invalid -> 400; role sama -> no-op; bukan owner/admin -> 403; owner terakhir turun sendiri tanpa admin -> 409 `LAST_OWNER`).
   6. Bila bukan no-op (`in.Role != target.Role`): `UpdateMemberRole`. No-op tidak menulis apa pun.
4. Setelah commit: `users.Get(target.UserID)` fail-soft (error -> `Found=false` dan label audit `""`, ditulis log Warn; aksi sudah commit sehingga tidak boleh gagal).
5. Bila ada perubahan: kirim audit `project.member.role_changed` (`bus.Call`, fail-open, di luar tx). No-op -> 0 entri (D5).
6. Respons 200 `MemberDTO` dengan role baru (`target.ToDTO(user)`); no-op juga 200 dengan data saat ini.

Keputusan khusus usecase ini:

- Admin (Super Admin) selalu boleh, termasuk menurunkan owner terakhir (project tanpa owner diperbolehkan, `owner_count` jadi 0). Super Admin bukan anggota, jadi tidak pernah menjadi target.
- Owner boleh menurunkan owner lain (selama `ownerCount >= 2`, otomatis benar karena aktor sendiri owner) dan menurunkan dirinya sendiri bila ada owner lain. Owner terakhir yang menurunkan diri -> 409 `LAST_OWNER`.
- Dua owner yang sama-sama menurunkan DIRI SENDIRI secara bersamaan: serial lewat lock; yang pertama sukses (ada owner lain), yang kedua mendapat 409 `LAST_OWNER` karena sekarang owner terakhir. Tepat satu sukses, project tidak pernah kehilangan owner oleh non-admin.
- `userID` dibandingkan apa adanya dengan `user_id` dari DB (teks uuid huruf kecil). `userID` bukan uuid atau berhuruf besar -> tidak cocok -> 404 anggota tidak ditemukan; tidak ada query tambahan dan tidak ada validasi uuid di handler.
- Body: `role` wajib. Field tak dikenal atau tipe salah -> 400 dari `respond.DecodeJSON`. `role` tidak dikirim/`null`/`""` -> 400 `fields.role = "wajib diisi"` dari `Validate()` di `params.go` (validasi bentuk, tidak bergantung project, boleh sebelum otorisasi). Nilai selain `owner|member` -> 400 `fields.role = "role harus owner atau member"` dari entity (SETELAH otorisasi dan setelah target ditemukan).
- `projects.updated_at` TIDAK disentuh (perubahan keanggotaan bukan perubahan project).
- Hanya kolom `role` yang diubah; `added_by`/`added_at` tetap (siapa menambahkan, bukan siapa mengubah peran; pelaku perubahan ada di audit).
- `user_label` audit = email target dari get-user (konsisten dengan label aktor yang berupa email).

Prasyarat (dari F; bila belum ada, hentikan dan selesaikan F dulu, jangan membuatnya dari folder `changememberrole/`): migrasi `projects`/`project_members`; konstanta bus T03 dan konsumsi; paket `base` (`Member`, `OwnerCount`, `FindMember`, `Member.ToDTO`, `MemberDTO`, `ValidMemberRole`, `CheckRoleChange`, `ReasonLastOwner`, `ResolveLevel`, `Require`, `PermManageMembers`, `Actor`, `ActorFrom`, `RequestMeta`, `Clock` bila dibutuhkan, `FieldErrors`/`InvalidFields`, `ErrProjectNotFound`, `ErrMemberNotFound`, `ErrForbidden`, `ErrLastOwner`); `base/store` (`FindAccess`, `LockProject`, `LockMembers`, `WithTx`); `base/audit`; `base/users`; entri OpenAPI `changememberrole` di `management.yml`; kontrak `getuser` (T01) dan `record` + `ActionProjectMemberRoleChanged` (T02).

## 3. Plan (langkah berurutan)

Semua file di `backend/internal/domain/project/internal/management/changememberrole/`. Urutan TDD: tulis tes dulu per langkah.

1. **Verifikasi prasyarat**: `go build ./...` lulus; `grep -n "changememberrole" api/modules/project/management.yml` menunjukkan operasi `PATCH /api/projects/{projectID}/members/{userID}` dengan `operationId: changememberrole`, request `ChangeMemberRoleRequest` (`role` wajib, enum `owner|member`, `additionalProperties: false`), respons 200 `Member`, error 400/401/403/404/409/500. Bila ada selisih dengan bagian 8, perbaiki `management.yml` di PR ini, jalankan `make api-bundle`, commit `api/openapi.yml`, pastikan `make api-check` lulus (rule 11.6: spec sebelum handler).

2. **`params.go`** (bentuk request):
   ```go
   // ChangeRoleRequest adalah body PATCH. Pointer membedakan "tidak dikirim" dari "".
   type ChangeRoleRequest struct {
       Role *string `json:"role"`
   }
   // Validate memeriksa bentuk saja (wajib ada). Nilai valid/tidaknya adalah rule entity.
   func (r ChangeRoleRequest) Validate() error // nil/"" -> base.InvalidFields(FieldErrors{"role": "wajib diisi"})
   func (r ChangeRoleRequest) Value() string   // *r.Role atau ""
   ```
   Tidak ada pengecekan `owner|member` di sini.

3. **`usecase.go`**:
   ```go
   // Sisi consumer (rule 5.6). Repo konkret memenuhi keduanya.
   type Repository interface {
       FindAccess(ctx context.Context, projectID, userID string) (p base.Project, memberRole string, found bool, err error)
       InTx(ctx context.Context, fn func(TxRepository) error) error
   }
   type TxRepository interface {
       LockProject(ctx context.Context, id string) (base.Project, bool, error)
       LockMembers(ctx context.Context, projectID string) ([]base.Member, error)
       UpdateMemberRole(ctx context.Context, projectID, userID, role string) error
   }

   type Input struct {
       ProjectID string
       UserID    string // dari path, apa adanya
       Role      string
       Meta      base.RequestMeta
   }

   type Usecase struct {
       repo  Repository
       users users.Lookup
       audit audit.Recorder
       log   *slog.Logger
   }
   func NewUsecase(repo Repository, u users.Lookup, a audit.Recorder, log *slog.Logger) *Usecase
   func (u *Usecase) Execute(ctx context.Context, in Input) (base.MemberDTO, error)
   ```
   Alur `Execute`:
   1. `actor, ok := base.ActorFrom(reqctx.IdentityFrom(ctx))`; `!ok` -> `errs.New(errs.CodeUnauthorized, "perlu login")`.
   2. `_, memberRole, found, err := u.repo.FindAccess(ctx, in.ProjectID, actor.ID)`; `err` -> `fmt.Errorf("changememberrole: cari akses: %w", err)`; `!found` -> `base.ErrProjectNotFound`. `level := base.ResolveLevel(actor.Role, memberRole, true)`; `if err := base.Require(level, base.PermManageMembers); err != nil { return base.MemberDTO{}, err }`.
   3. ```go
      var (project base.Project; target base.Member; newRole string; changed bool)
      err = u.repo.InTx(ctx, func(tx TxRepository) error {
          p, ok, err := tx.LockProject(ctx, in.ProjectID)
          if err != nil { return fmt.Errorf("changememberrole: kunci project: %w", err) }
          if !ok { return base.ErrProjectNotFound }
          ms, err := tx.LockMembers(ctx, p.ID)
          if err != nil { return fmt.Errorf("changememberrole: kunci anggota: %w", err) }
          // hitung ulang level aktor dari baris terkunci (overview 3.5 langkah 3)
          mine, _ := base.FindMember(ms, actor.ID)          // zero Member bila bukan anggota -> Role ""
          lvl := base.ResolveLevel(actor.Role, mine.Role, true)
          if err := base.Require(lvl, base.PermManageMembers); err != nil { return err }
          t, ok := base.FindMember(ms, in.UserID)
          if !ok { return base.ErrMemberNotFound }
          if err := base.CheckRoleChange(actor.ID, lvl, t, in.Role, base.OwnerCount(ms)); err != nil { return err }
          project, target, newRole = p, t, in.Role
          if in.Role == t.Role { return nil }               // no-op: tanpa UPDATE
          if err := tx.UpdateMemberRole(ctx, p.ID, t.UserID, in.Role); err != nil {
              return fmt.Errorf("changememberrole: simpan: %w", err)
          }
          changed = true
          return nil
      })
      ```
      Error `*errs.Error` dikembalikan apa adanya (tidak di-wrap) agar `respond.WriteError` memetakan status yang benar. Catatan: `FindMember` mengembalikan `(Member, bool)`; hasil `ok` aktor boleh diabaikan karena `ResolveLevel` memperlakukan role kosong sebagai `none`. Perbandingan `in.Role == t.Role` di usecase hanyalah penentu "tulis atau tidak" setelah `CheckRoleChange` lolos (rule no-op tetap di entity: `CheckRoleChange` mengembalikan nil untuk role sama). Role invalid sudah ditolak `CheckRoleChange` sebelum sampai ke sini.
   4. Setelah commit: `user, err := u.users.Get(ctx, target.UserID)`; error -> `u.log.WarnContext(ctx, "changememberrole: get-user gagal", "project_id", project.ID, "user_id", target.UserID, "error", err)` dan `user = getuser.Response{}` (Found=false). Tidak pernah menggagalkan request.
   5. Bila `changed`: `u.audit.Record(ctx, audit.Event{Action: record.ActionProjectMemberRoleChanged, ProjectID: project.ID, ProjectLabel: project.Name, TargetType: "project_member", Target: target.UserID, Args: map[string]any{"user_id": target.UserID, "user_label": user.Email, "role_from": target.Role, "role_to": newRole}, Meta: in.Meta})`. `Record` tidak mengembalikan error (fail-open). Dipanggil di luar tx.
   6. `target.Role = newRole`; `return target.ToDTO(user), nil`. (`ToDTO` dengan `Found=false` menghasilkan `name/email: null`, `found=false`, `is_active=false`.)

   Usecase tidak menulis SQL, tidak memvalidasi role sendiri, tidak memanggil `bus` langsung. Penggantian role pada struct `target` hanya untuk komposisi respons.

4. **`repository.go`** (satu-satunya yang menyentuh SQL di feature ini):
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
   func (t *txRepo) LockProject(ctx, id) (base.Project, bool, error)    // delegasi store
   func (t *txRepo) LockMembers(ctx, projectID) ([]base.Member, error)   // delegasi store
   func (t *txRepo) UpdateMemberRole(ctx context.Context, projectID, userID, role string) error
   ```
   SQL tunggal `UpdateMemberRole` (parameterized, rule 5.3):
   ```sql
   UPDATE project_members SET role = $3 WHERE project_id = $1 AND user_id = $2
   ```
   `RowsAffected() == 0` -> `base.ErrMemberNotFound` (jaring pengaman; tidak mungkin di bawah lock). Pelanggaran CHECK `project_members_role_check` dikembalikan sebagai error infra (500) karena entity sudah memvalidasi. `projectID`/`userID` yang dikirim sudah berasal dari baris terkunci (uuid valid).

5. **`handler.go`**:
   ```go
   type handler struct{ uc *Usecase }
   func (h handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
       var req ChangeRoleRequest
       if err := respond.DecodeJSON(r, &req); err != nil { respond.WriteError(w, r, err); return }
       if err := req.Validate(); err != nil { respond.WriteError(w, r, err); return }
       out, err := h.uc.Execute(r.Context(), Input{
           ProjectID: chi.URLParam(r, "projectID"),
           UserID:    chi.URLParam(r, "userID"),
           Role:      req.Value(),
           Meta:      base.RequestMeta{IP: middleware.KeyByIP(r), UserAgent: truncate(r.UserAgent(), 512)},
       })
       if err != nil { respond.WriteError(w, r, err); return }
       respond.WriteJSON(w, http.StatusOK, out)
   }
   ```
   `truncate` (potong ke 512 dengan aman UTF-8) helper privat di `handler.go`. Handler tidak memvalidasi uuid dan tidak me-log body.

6. **`init.go`**:
   ```go
   func Init(d app.Deps) {
       s := store.New(d.DB)
       uc := NewUsecase(newRepo(d.DB, s), users.New(d.Bus), audit.New(d.Bus, d.Log), d.Log)
       d.API.With(middleware.RequireAuth()).Patch("/projects/{projectID}/members/{userID}", handler{uc: uc}.ServeHTTP)
   }
   ```
   Pola penuh langsung di `d.API`, param `{projectID}` dan `{userID}` (overview D4). Tanpa `Route`/`Mount`. Route ini berbagi pola dengan `DELETE` milik `removemember` (method berbeda, tidak bentrok).

7. **Wiring**: tambah satu baris `changememberrole.Init(d)` di `management/base.go` (PR usecase ini). Jangan menyentuh baris `Init` feature lain.

8. **Tes** sesuai bagian 6, urutan: unit usecase (RED -> hijau), handler, repository integrasi, integrasi feature (termasuk race).

9. **Verifikasi**: `cd backend && go build ./... && go vet ./... && go test ./internal/domain/project/internal/management/changememberrole/...`; dengan `TEST_DATABASE_URL`: `go test -tags integration ./internal/domain/project/internal/management/changememberrole/...` (jalankan balapan dengan `-race -count=3`); `make api-check`; archtest `TestT00_AC4_RepoIsClean` lulus (tidak ada import antar-feature, SQL hanya di `repository.go`).

Kontrak bus (ringkas, sisi consumer):

| Aspek | Nilai |
|---|---|
| get-user | `bus.CallTyped[getuser.Response](ctx, b, constants.BUS_CORE_AUTH_GET_USER, getuser.Request{UserID})` lewat `users.BusLookup`; 1 Call per request, SETELAH commit; error/`ErrNotRegistered` ditelan (log Warn) |
| Audit | `bus.CallTyped[record.Response](ctx, b, constants.BUS_CORE_AUDIT_RECORD, record.Request{...})` lewat `audit.BusRecorder`; error di-log (`audit record gagal`) dan DITELAN; maksimal 1 Call per request, 0 pada no-op atau error |
| `record.Request` | `ActorType: record.ActorUser`, `ActorID` = user id aktor, `ActorLabel` = email aktor, `ProjectID`, `ProjectLabel` = nama project, `Action: record.ActionProjectMemberRoleChanged`, `TargetType: "project_member"`, `Target` = user id target, `Args`, `Status: record.StatusOK`, `IP`, `UserAgent`, `TraceID` (diisi `audit.BusRecorder`) |

## 4. Migration

Tidak ada. Tabel `project_members` (PK `(project_id, user_id)`, CHECK `role IN ('owner','member')`) dibuat migrasi foundation `backend/migrations/project/20261022090000000_create_projects.sql` (overview bagian 6). Usecase ini tidak menambah kolom atau indeks (UPDATE berdasarkan PK).

## 5. Changes (per file/folder)

| File | Aksi | Isi |
|---|---|---|
| `backend/internal/domain/project/internal/management/changememberrole/init.go` | buat | rakit store -> repo -> usecase -> handler; `d.API.With(RequireAuth()).Patch("/projects/{projectID}/members/{userID}", ...)` |
| `.../changememberrole/handler.go` | buat | decode body, `Validate()`, bangun `Input`/`RequestMeta`, `WriteJSON` 200 / `WriteError` |
| `.../changememberrole/params.go` | buat | `ChangeRoleRequest` (pointer) + `Validate()` + `Value()` |
| `.../changememberrole/usecase.go` | buat | interface `Repository`/`TxRepository`, `Input`, `Usecase.Execute` |
| `.../changememberrole/repository.go` | buat | `Repo` (delegasi ke `store`), `InTx`, `txRepo.UpdateMemberRole` (satu UPDATE) |
| `.../changememberrole/usecase_test.go` | buat | unit dengan repo/users/audit palsu |
| `.../changememberrole/handler_test.go` | buat | `httptest` |
| `.../changememberrole/repository_test.go` | buat | integrasi (tag `integration`, `db.TestDB`) |
| `.../changememberrole/changememberrole_integration_test.go` | buat | integrasi HTTP penuh + race (tag `integration`) |
| `backend/internal/domain/project/internal/management/base.go` | ubah | tambah `changememberrole.Init(d)` |
| `api/modules/project/management.yml`, `api/openapi.yml` | verifikasi, ubah hanya bila selisih | operasi `changememberrole` (bagian 8) |
| `frontend/src/domain/project/management/members/` | lihat `frontend-plan.md` | menu ubah peran per baris anggota (owner/admin saja); di luar cakupan backend plan ini |

Tidak diubah: `base/` (kecuali bug yang ditemukan; bila rule perlu berubah, ubah di `base/` beserta tesnya, bukan di usecase), `base/store`, `infra/**`, feature lain (termasuk `removemember`), `go.mod`, konfigurasi.

## 6. Test Scenario

Penamaan: tes kriteria `TestT03_ACn_*`; lainnya `Test<Unit>_<Kasus>`. Tabel rule `CheckRoleChange`/`Require`/`ResolveLevel` sudah dites di `base/` (F); tes di sini membuktikan orkestrasi, urutan, dan wiring tanpa menduplikasi tabel itu.

### Unit usecase (`usecase_test.go`; repo, users, audit palsu)

Repo palsu mencatat urutan panggilan, menyimpan daftar anggota terkunci yang dapat diprogram, dan bisa dipaksa error per metode. Audit palsu menyimpan `Event`. Users palsu dapat dipaksa error. Identitas disuntik lewat `reqctx.WithIdentity`.

- `TestChangeRole_OwnerPromotesMember`: aktor owner A, target B member, `Role:"owner"` -> `UpdateMemberRole(project, B, "owner")` dipanggil sekali; DTO `Role=="owner"`; 1 event audit.
- `TestChangeRole_OwnerDemotesOtherOwner`: owner A dan C, A menurunkan C -> sukses, `UpdateMemberRole` dengan "member".
- `TestT03_AC3_OwnerDemotesSelfWhenAnotherOwnerExists`: owner A dan C, A menurunkan diri -> sukses.
- `TestT03_AC3_LastOwnerCannotDemoteSelf`: A satu-satunya owner, B member, A menurunkan diri -> `errors.Is(err, base.ErrLastOwner)`, `details.reason=="LAST_OWNER"`; `UpdateMemberRole` tidak dipanggil; audit kosong.
- `TestT03_AC3_SuperAdminCanDemoteLastOwner`: aktor `super_admin` (bukan anggota), A satu-satunya owner -> sukses; `AccessLevel` admin tidak bocor ke respons (hanya `MemberDTO`); 1 event audit.
- `TestT03_AC7_SuperAdminPromotesMember`: super_admin menaikkan member jadi owner -> sukses.
- `TestT03_AC2_MemberForbidden`: aktor developer `member` -> `errors.Is(err, base.ErrForbidden)` pada pre-check; `InTx` tidak dipanggil; get-user dan audit tidak dipanggil.
- `TestChangeRole_NonMemberActorGets404`: developer bukan anggota (`memberRole=""`) -> `ErrProjectNotFound`; `InTx` tidak dipanggil.
- `TestChangeRole_ProjectNotFound`: `found=false` -> `ErrProjectNotFound`, tanpa tx.
- `TestChangeRole_DeletedBetweenCheckAndLock`: `FindAccess` found, `LockProject` ok=false -> `ErrProjectNotFound`; tidak ada UPDATE.
- `TestChangeRole_TargetNotMember`: `userID` tidak ada di daftar terkunci (termasuk `"bukan-uuid"`) -> `ErrMemberNotFound`; tidak ada UPDATE/audit/get-user.
- `TestChangeRole_NonMemberActorWithMissingTargetStill404Project`: non-anggota + target tak ada -> tetap `ErrProjectNotFound` (bukan `ErrMemberNotFound`; urutan otorisasi lebih dulu).
- `TestChangeRole_InvalidRole400`: role `"admin"`, `"OWNER"`, `" owner"` (aktor owner, target ada) -> `*errs.Error` `CodeInvalid`, `details.fields.role == "role harus owner atau member"`; tidak ada UPDATE/audit.
- `TestChangeRole_NoopSkipsWriteAndAudit`: target owner, `Role:"owner"` -> sukses, `UpdateMemberRole` TIDAK dipanggil, audit 0 event, DTO `Role=="owner"`, get-user tetap dipanggil (untuk DTO).
- `TestChangeRole_NoopOnLastOwnerIsOK`: A owner terakhir, `Role:"owner"` pada dirinya -> 200 no-op (bukan 409).
- `TestChangeRole_RecomputesActorLevelInsideTx` (inti AC3): `FindAccess` mengembalikan `memberRole="owner"` tetapi daftar terkunci memuat aktor sebagai `member` (disimulasikan seolah sudah diturunkan tx lain) -> `ErrForbidden`; `UpdateMemberRole` tidak dipanggil.
- `TestChangeRole_ActorRemovedBeforeLock`: aktor sudah tidak ada di daftar terkunci -> `ErrProjectNotFound` (level none).
- `TestChangeRole_AuditPayload`: sukses -> tepat 1 event: `Action==record.ActionProjectMemberRoleChanged`, `ProjectID`, `ProjectLabel==nama project`, `TargetType=="project_member"`, `Target==userID target`, `Args=={"user_id", "user_label":"b@tim.id", "role_from":"member", "role_to":"owner"}`, `Meta` sama dengan input.
- `TestChangeRole_AuditAfterTxAndGetUser`: urutan panggilan palsu = `FindAccess, InTx(begin), LockProject, LockMembers, UpdateMemberRole, InTx(commit), users.Get, audit.Record`. Get-user dan audit tidak pernah dipanggil di dalam fungsi tx.
- `TestChangeRole_GetUserFailureIsSoft`: users palsu error -> tetap sukses; DTO `Found==false`, `Name==nil`, `Email==nil`, `IsActive==false`, `Role` baru; event audit `user_label==""`; log Warn tercatat.
- `TestChangeRole_GetUserNotFound`: `Found=false` tanpa error -> DTO `found=false`, `user_label==""`, tetap sukses.
- `TestChangeRole_TxErrorNoAudit`: `UpdateMemberRole` error -> error terbungkus (`errors.Is` ke error asal), audit 0, get-user tidak dipanggil.
- `TestChangeRole_LockErrors`: `LockProject` error dan `LockMembers` error -> error terbungkus, tanpa audit.
- `TestChangeRole_DTOKeepsAddedFields`: `AddedAt`, `AddedBy`, `AddedByLabel` pada DTO sama dengan baris sebelum perubahan.
- `TestChangeRole_NoIdentity`: konteks tanpa identitas -> `CodeUnauthorized`, tanpa panggilan repo.

### Unit handler (`handler_test.go`)

- 200 dan body JSON sesuai bagian 8; `Content-Type: application/json`.
- Body `{"rol":"owner"}` (field tak dikenal) -> 400 dari `DecodeJSON`; usecase tidak dipanggil.
- Body bukan JSON, `{"role":5}` -> 400.
- Body `{}`, `{"role":null}`, `{"role":""}` -> 400 `fields.role == "wajib diisi"`; usecase tidak dipanggil.
- `{"role":"admin"}` diteruskan ke usecase apa adanya (400 nilai dibangkitkan entity).
- `chi.URLParam("projectID")` dan `("userID")` diteruskan apa adanya, termasuk `"bukan-uuid"`.
- `RequestMeta.UserAgent` dipotong 512; `IP` dari `middleware.KeyByIP`.
- Mapping error: `ErrProjectNotFound`/`ErrMemberNotFound` -> 404 `NOT_FOUND` (pesan masing-masing); `ErrForbidden` -> 403 `FORBIDDEN`; `ErrLastOwner` -> 409 `CONFLICT` dengan `details.reason=="LAST_OWNER"`; `InvalidFields` -> 400 `INVALID_ARGUMENT`; error infra -> 500 `INTERNAL` tanpa detail internal.

### Repository (`repository_test.go`, tag `integration`, Postgres nyata via `db.TestDB`)

- `UpdateMemberRole` mengubah hanya kolom `role`; `added_by`, `added_by_label`, `added_at` dan `projects.updated_at` tidak berubah.
- `UpdateMemberRole` pada pasangan yang tidak ada -> `ErrMemberNotFound`.
- `UpdateMemberRole` dengan role `"admin"` (dipaksa lewat repo) -> error DB mentah dari CHECK (jaring pengaman, bukan jalur normal).
- `InTx`: error dari `fn` -> rollback (role tidak berubah); sukses -> commit.
- Penguncian: tx A `LockProject` + `LockMembers` lalu tidur; tx B `LockProject` pada project sama memblokir sampai A commit, lalu `LockMembers` melihat role hasil A (dua goroutine, kanal sinkron, batas waktu tes 5 detik).

### Integrasi feature (`changememberrole_integration_test.go`, tag `integration`)

Bus nyata (`bus.NewRegistry`), `db.TestDB`, router `chi` dengan middleware uji yang menyuntik identitas, handler palsu `core.auth.get-user` (fixture A, B, C developer aktif; S super_admin) dan `core.audit.record` (menyimpan `record.Request`). Project dan anggota dibuat langsung lewat SQL/`store` agar tes tidak bergantung pada feature lain.

- `TestT03_AC3_LastOwnerCannotDemoteSelf_HTTP`: A satu-satunya owner, B member; A `PATCH members/A {role:"member"}` -> 409 `LAST_OWNER`; DB: A tetap owner; 0 audit.
- `TestT03_AC3_SuperAdminDemotesLastOwner`: S `PATCH members/A {role:"member"}` -> 200 `role=="member"`; `SELECT count(*) ... role='owner'` = 0; 1 audit; S tetap bukan anggota (tidak ada baris S di `project_members`).
- `TestT03_AC3_OwnerPromotesThenSelfDemotes`: A `PATCH members/B {role:"owner"}` -> 200; A `PATCH members/A {role:"member"}` -> 200 (B owner); 2 audit berurutan `role_from/role_to` benar.
- `TestT03_AC3_ConcurrentMutualDemotion` (balapan inti): A dan C owner, B member; dua goroutine dilepas bersamaan lewat kanal awal: A `PATCH members/C {member}` dan C `PATCH members/A {member}`. Diulang 20 kali dengan project baru -> tiap putaran tepat satu 200 dan satu 403 (yang kalah sudah turun jadi member lewat tx pemenang), `count(owner) == 1`, tepat 1 entri audit.
- `TestT03_AC3_ConcurrentSelfDemotion`: A dan C owner; A `PATCH members/A {member}` dan C `PATCH members/C {member}` bersamaan, 20 putaran -> tepat satu 200 dan satu 409 `LAST_OWNER`, `count(owner) == 1`, tepat 1 audit.
- `TestChangeRole_ConcurrentWithRemove`: A dan C owner; A `PATCH members/C {member}` bersamaan dengan SQL langsung `DELETE FROM project_members WHERE user_id = C` -> PATCH 200 atau 404 `anggota tidak ditemukan`, tidak pernah 500, tidak ada baris yatim; invarian `count(owner) >= 1`.
- `TestChangeRole_RaceWithDeleteProject`: PATCH dan `DELETE FROM projects` (SQL langsung) bersamaan -> PATCH 200 atau 404, tidak pernah 500.
- `TestT03_AC2_MemberCannotChangeRole`: B (member) `PATCH members/A {member}` -> 403 `FORBIDDEN`; B `PATCH members/B {owner}` (naik sendiri) -> 403; DB tidak berubah; 0 audit.
- `TestT03_Security_NonMemberCannotChangeRole` (rule 13.6): D developer non-anggota `PATCH` -> 404 `NOT_FOUND` "project tidak ditemukan" identik (byte-per-byte kecuali `trace_id`) dengan respons untuk uuid acak dan `"bukan-uuid"` sebagai `projectID`; juga identik walau `userID` target tidak ada; baris DB tidak berubah; 0 audit.
- `TestChangeRole_TargetNotMember404`: A owner `PATCH members/<uuid acak>` dan `members/bukan-uuid` -> 404 `anggota tidak ditemukan`.
- `TestChangeRole_Noop200NoAudit`: A `PATCH members/B {member}` saat B sudah member -> 200 `role=="member"`; 0 audit; DB tidak berubah.
- `TestChangeRole_InvalidRole400`: `{"role":"admin"}` -> 400 `fields.role` "role harus owner atau member"; `{}` dan `{"role":null}` -> 400 `fields.role` "wajib diisi"; DB tidak berubah; 0 audit.
- `TestChangeRole_UnknownField400`: `{"role":"owner","x":1}` -> 400.
- `TestChangeRole_DeactivatedTargetCanStillBeChanged`: get-user palsu menandai B `IsActive=false` -> owner tetap boleh mengubah peran B (keanggotaan tercatat); 200, DTO `is_active==false`.
- `TestChangeRole_GetUserDown`: tanpa handler `core.auth.get-user` terdaftar -> PATCH tetap 200, DTO `found:false`, `name/email:null`; data tersimpan; audit `user_label==""`; log Warn tercatat.
- `TestChangeRole_AuditPayload`: `record.Request`: `Action=="project.member.role_changed"`, `ActorType=="user"`, `ActorID`, `ActorLabel` = email aktor, `ProjectID`, `ProjectLabel` = nama project, `TargetType=="project_member"`, `Target` = user id target, `Status=="ok"`, `IP`, `UserAgent`, `TraceID` tidak kosong, `Args` persis `{user_id, user_label, role_from, role_to}` tanpa field lain.
- `TestChangeRole_AuditFailOpen`: handler `core.audit.record` error, dan skenario endpoint tidak terdaftar -> PATCH tetap 200, role tersimpan, log error `audit record gagal` tercatat, tanpa isi body di log.
- `TestChangeRole_DoesNotTouchProjectUpdatedAt`: `projects.updated_at` sama sebelum dan sesudah.
- `TestChangeRole_CSRFAndAuth`: tanpa `X-Requested-With` -> 403 `CSRF_REQUIRED`; tanpa sesi -> 401; keduanya sebelum usecase.
- `TestChangeRole_RouteRegistered`: `chi.Walk` memuat `PATCH /api/projects/{projectID}/members/{userID}`; tidak bentrok dengan `DELETE` pada path yang sama.

## 7. E2E Scenarios (Playwright terhadap binary; butuh T01 dan T02 merge, UI dari `frontend-plan.md`)

Positif:
1. Owner A membuka tab Members project "Payments", memilih menu peran pada baris B (member) -> "Jadikan owner" -> baris B berubah menjadi owner tanpa reload penuh; Super Admin melihat entri `project.member.role_changed` (`role_from: member`, `role_to: owner`, aktor A) di audit log.
2. Setelah B owner, A menurunkan dirinya menjadi member -> berhasil; A tetap melihat project, tombol kelola anggota hilang (hanya "Keluar"); B owner.
3. Super Admin (bukan anggota) membuka project yang owner tunggalnya A -> menurunkan A menjadi member -> berhasil; project tanpa owner; Super Admin kemudian menaikkan B menjadi owner.
4. Memilih peran yang sama dengan peran sekarang lewat API -> 200, audit log tidak bertambah.

Negatif:
1. Owner terakhir A menurunkan dirinya -> UI menampilkan pesan i18n `project.error.LAST_OWNER`; peran tidak berubah.
2. Member B membuka tab Members -> tidak ada menu ubah peran; `PATCH` via API langsung -> 403 dan UI menampilkan pesan akses ditolak.
3. Developer C (bukan anggota) memanggil `PATCH /api/projects/<id>/members/<user>` -> 404 yang identik dengan id project acak; membuka `/projects/<id>` langsung -> halaman "tidak ditemukan".
4. Owner memanggil API dengan `role: "admin"` -> 400 `fields.role`; dengan body `{}` -> 400 `fields.role` "wajib diisi".
5. Dua tab owner A dan C saling menurunkan pada saat hampir bersamaan -> satu berhasil, yang lain mendapat 403; setelah reload tepat satu owner tersisa.
6. Tab lama: anggota sudah dikeluarkan lewat tab lain, lalu menu ubah peran dipakai -> 404 "anggota tidak ditemukan"; UI memuat ulang daftar anggota.
7. `PATCH` tanpa header `X-Requested-With` -> 403 `CSRF_REQUIRED`; tanpa login -> 401.

## 8. Request/Response body

Format error seragam T00:

```json
{ "error": { "code": "NOT_FOUND", "message": "project tidak ditemukan", "details": null }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

### PATCH /api/projects/{projectID}/members/{userID}

Header: `X-Requested-With: grimoire`, `Content-Type: application/json`. Body wajib memuat `role`; field lain ditolak.

Request (naikkan jadi owner):
```json
{ "role": "owner" }
```

Request (turunkan jadi member):
```json
{ "role": "member" }
```

200 (berubah; `MemberDTO`, sama bentuknya dengan item `listmembers`):
```json
{
  "user_id": "0192f5a0-bbbb-7aaa-8bbb-00000000000b",
  "name": "Budi",
  "email": "b@tim.id",
  "is_active": true,
  "found": true,
  "role": "owner",
  "added_at": "2026-10-22T09:05:00Z",
  "added_by": "0192f5a0-aaaa-7aaa-8bbb-00000000000a",
  "added_by_label": "a@tim.id"
}
```

200 (no-op, peran sudah sama): bentuk sama dengan `role` saat ini; tidak ada UPDATE dan tidak ada entri audit.

200 (get-user gagal setelah commit; perubahan tetap tersimpan):
```json
{
  "user_id": "0192f5a0-bbbb-7aaa-8bbb-00000000000b",
  "name": null,
  "email": null,
  "is_active": false,
  "found": false,
  "role": "owner",
  "added_at": "2026-10-22T09:05:00Z",
  "added_by": "0192f5a0-aaaa-7aaa-8bbb-00000000000a",
  "added_by_label": "a@tim.id"
}
```

400 (nilai role tidak valid):
```json
{
  "error": {
    "code": "INVALID_ARGUMENT",
    "message": "permintaan tidak valid",
    "details": { "fields": { "role": "role harus owner atau member" } }
  },
  "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A"
}
```

400 (`role` tidak dikirim, `null`, atau `""`):
```json
{
  "error": {
    "code": "INVALID_ARGUMENT",
    "message": "permintaan tidak valid",
    "details": { "fields": { "role": "wajib diisi" } }
  },
  "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A"
}
```

400 (field tak dikenal, mis. `{"rol":"owner"}`, atau tipe salah): ditolak `respond.DecodeJSON` dengan `code: INVALID_ARGUMENT` (pesan dari T00).

401 (tanpa sesi): `{ "error": { "code": "UNAUTHENTICATED", ... } }` sesuai middleware T00.

403 (tanpa header CSRF): `{ "error": { "code": "CSRF_REQUIRED", ... } }` dari middleware T00.

403 (aktor adalah member project, atau pada tx ternyata sudah diturunkan oleh tx lain):
```json
{ "error": { "code": "FORBIDDEN", "message": "akses ditolak", "details": null }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

404 (aktor bukan anggota, project tidak ada, atau `projectID` bukan uuid; respons identik):
```json
{ "error": { "code": "NOT_FOUND", "message": "project tidak ditemukan", "details": null }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

404 (aktor owner/admin, tetapi `userID` bukan anggota project ini atau bukan uuid):
```json
{ "error": { "code": "NOT_FOUND", "message": "anggota tidak ditemukan", "details": null }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

409 (owner terakhir menurunkan dirinya; Super Admin tidak terkena):
```json
{ "error": { "code": "CONFLICT", "message": "owner terakhir tidak bisa keluar atau diturunkan; angkat owner lain lebih dulu", "details": { "reason": "LAST_OWNER" } }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

500: `{ "error": { "code": "INTERNAL", "message": "terjadi kesalahan", "details": null }, "trace_id": "..." }` (pesan generik sesuai T00; detail hanya di log server).

### Entri audit yang dikirim (`record.Request`, contoh Owner A menaikkan B menjadi owner)

```go
record.Request{
    ActorType: record.ActorUser, ActorID: "0192f5a0-aaaa-7aaa-8bbb-00000000000a", ActorLabel: "a@tim.id",
    ProjectID: "0192f5a0-1111-7aaa-8bbb-000000000001", ProjectLabel: "Payments",
    Action: record.ActionProjectMemberRoleChanged, TargetType: "project_member", Target: "0192f5a0-bbbb-7aaa-8bbb-00000000000b",
    Args: map[string]any{"user_id": "0192f5a0-bbbb-7aaa-8bbb-00000000000b", "user_label": "b@tim.id", "role_from": "member", "role_to": "owner"},
    Status: record.StatusOK, IP: "10.0.0.5", UserAgent: "Mozilla/5.0", TraceID: "01JB2X9K6Q4Y7V3N8M5T2R1C0A",
}
```

Kontrak get-user yang dikirim: `getuser.Request{UserID: "0192f5a0-bbbb-7aaa-8bbb-00000000000b"}` -> `getuser.Response{Found: true, ID: "...", Name: "Budi", Email: "b@tim.id", Role: "developer", IsActive: true}`.

### Skema OpenAPI operasi (`api/modules/project/management.yml`, harus ada sejak F)

- `PATCH /api/projects/{projectID}/members/{userID}`, `operationId: changememberrole`, `tags: [project-members]`.
- Parameter: `$ref` `XRequestedWith` (header, wajib), `ProjectID` (path, `format: uuid`), `UserID` (path, `format: uuid`).
- Request body `ChangeMemberRoleRequest`: `required: [role]`, `properties: {role: {type: string, enum: [owner, member]}}`, `additionalProperties: false`.
- Respons: `200` `Member`; `400` BadRequest; `401` Unauthorized; `403` Forbidden (CSRF, member, atau diturunkan oleh tx lain); `404` NotFound (deskripsi: "project tidak ditemukan, juga untuk project yang bukan milik pemanggil (tidak bocor); atau anggota tidak ditemukan"); `409` Conflict (`details.reason: LAST_OWNER`); `500` Internal. Semua `$ref: ../infra/foundation.yml#/components/responses/...`.
