# T03 project/management — Usecase `updateproject`

Konteks bersama (migrasi, konstanta bus, kontrak, `base/`, `base/store`, `base/audit`, `base/counts`, OpenAPI, i18n) ada di `overview.md` dan dikerjakan di langkah foundation F. Dokumen ini hanya memuat usecase `updateproject`. Bila bertentangan dengan `overview.md`, overview menang. Tiket: `docs/tickets/T03-project-management.md`. Kriteria: AC2 (member boleh mengubah nama/deskripsi; tidak boleh mengelola anggota).

## 1. Metadata

| Atribut | Nilai |
|---|---|
| Scope | fullstack (backend di dokumen ini; UI tab Settings di `frontend-plan.md`) |
| Domain | project |
| Module | management |
| Feature | `updateproject` |
| Resource root | `backend/internal/domain/project/internal/management` |
| allowedPaths | `backend/internal/domain/project/internal/management/updateproject/`, baris `updateproject.Init(d)` di `.../management/base.go` |

| Endpoint | Method | Version | Akses |
|---|---|---|---|
| `/api/projects/{projectID}` | PATCH | tanpa prefix versi (prefix `/api`; `info.version` 0.1.0) | owner, member, super_admin (`Require(PermEdit)`); bukan anggota dan project tidak ada/bukan uuid: 404 |

Header wajib: `X-Requested-With: grimoire` (CSRF T00), `Content-Type: application/json`, sesi login (`RequireAuth`).

Kontrak bus yang dipakai (nama dan tipe persis plan pemilik, tidak didefinisikan ulang):

| Endpoint | Mode | Provider | Dipakai untuk |
|---|---|---|---|
| `core.audit.record` (`constants.BUS_CORE_AUDIT_RECORD`) | Call, fail-open | T02 | audit `project.updated` via `base/audit` |
| `project.knowledge.count-files`, `project.apispec.count-specs`, `project.apikey.count-keys` | Call, fail-soft | T05/T06/T04 | `counts` di respons via `base/counts` |

Tidak memanggil `core.auth.get-user` (overview D6). Tidak ada endpoint bus baru.

## 2. Summary Plan

`updateproject` mengubah nama dan/atau deskripsi project. Satu usecase, tidak dipecah.

Alur usecase (orkestrasi saja; rule bisnis ada di entity `base.Project.ApplyUpdate` dan `base.Require`):

1. Bangun aktor dari `reqctx`.
2. `FindAccess` (di luar tx) -> `ResolveLevel` -> `base.Require(level, base.PermEdit)`. Bukan anggota/tidak ada/bukan uuid -> 404 seragam; member, owner, admin lolos.
3. Satu transaksi: `LockProject` (`SELECT ... FOR UPDATE`; hilang -> 404) -> `Project.ApplyUpdate(patch, now)` -> bila `len(changed) > 0` jalankan `UPDATE`; bila tidak, tidak menulis apa pun.
4. Setelah commit, hanya bila ada perubahan: kirim audit `project.updated` (`bus.Call`, fail-open; `Args {changed_fields, name_from?, name_to?}`).
5. Susun respons `ProjectDetail` (200): `ListMembers` untuk `member_count` dan `owner_count`, `counts.ForProjects([id])` fail-soft, `NewSummary` dengan `level` dan `memberRole` dari langkah 2.

No-op (semua nilai setelah trim sama dengan yang tersimpan): tetap 200 dengan data saat ini, tanpa UPDATE, `updated_at` tidak berubah, tanpa audit (D5: satu aksi = tepat satu entri, no-op = 0 entri).

Keputusan khusus usecase ini:

- Patch kosong (`{}`; field `null` dianggap tidak diisi) -> 400 `fields.body` dari entity, SETELAH otorisasi, agar pemanggil non-anggota selalu mendapat 404. Error decode JSON (field tak dikenal, tipe salah) tetap 400 dari handler karena tidak bergantung pada project.
- Level aktor tidak dihitung ulang di dalam tx (berbeda dengan mutasi anggota, overview 3.5): `PermEdit` dimiliki member, owner, dan admin sehingga satu-satunya race adalah aktor baru saja dikeluarkan di antara pre-check dan lock. Dampaknya satu edit nama oleh orang yang sedetik lalu masih anggota; diterima, tidak ada pelanggaran invarian.
- `updated_at` ditulis dari nilai entity (`Clock`, UTC) dan dibaca kembali dengan `RETURNING updated_at` agar respons identik dengan nilai DB (presisi mikrodetik), bukan `now()` terpisah. Secara efek sama dengan `updated_at = now()` di overview 3.5.
- Respons memakai `ProjectDetail` (overview bagian 12) sehingga `member_count`/`owner_count` dibaca lewat `store.ListMembers` setelah commit (tanpa SQL baru di feature; hitung di Go dengan `len` dan `base.OwnerCount`). Tabel overview bagian 9 menulis kolom counts `-` untuk updateproject; bagian 12 (kontrak respons) menang karena klien membutuhkan bentuk yang sama dengan `GET`. Kegagalan counts tidak menggagalkan request.
- Deskripsi tidak pernah dikirim ke audit; hanya nama (`name_from`/`name_to`) dan daftar field berubah.

Prasyarat (dari F, bila belum ada hentikan dan selesaikan F dulu, jangan membuatnya dari folder `updateproject/`): migrasi `projects`/`project_members`; konstanta bus T03 dan konsumsi; paket `base` (`Project`, `ProjectPatch`, `ApplyUpdate`, `Require`, `ResolveLevel`, `NewSummary`, `ProjectDetail`, `Actor`, `RequestMeta`, `Clock`, `FieldErrors`, errors); `base/store` (`FindAccess`, `LockProject`, `ListMembers`, `WithTx`); `base/audit`; `base/counts`; entri OpenAPI `updateproject` di `management.yml`; kontrak `record` dan konstanta `ActionProjectUpdated` (T02).

## 3. Plan (langkah berurutan)

Semua file di `backend/internal/domain/project/internal/management/updateproject/`. Urutan TDD: tulis tes dulu per langkah.

1. **Verifikasi prasyarat**: `go build ./...` lulus; `grep -n "updateproject" api/modules/project/management.yml` menunjukkan operasi `PATCH /api/projects/{projectID}` dengan `operationId: updateproject`, `UpdateProjectRequest` (`minProperties: 1`), respons 200 `ProjectDetail`, error 400/401/403(CSRF)/404/500. Bila ada selisih dengan bagian 8 dokumen ini, perbaiki `management.yml` di PR ini, jalankan `make api-bundle`, commit `api/openapi.yml`, `make api-check` lulus (rule 11.6: spec sebelum handler).

2. **`params.go`** (bentuk request, bukan logika bisnis):
   ```go
   // UpdateRequest adalah body PATCH. Pointer membedakan "tidak dikirim" (nil) dari "" (dikirim kosong).
   type UpdateRequest struct {
       Name        *string `json:"name"`
       Description *string `json:"description"`
   }
   func (r UpdateRequest) Patch() base.ProjectPatch {
       return base.ProjectPatch{Name: r.Name, Description: r.Description}
   }
   ```
   Tidak ada validasi panjang/karakter di sini (itu entity). `{"name": null}` terdecode sebagai nil (dianggap tidak dikirim).

3. **`usecase.go`**:
   ```go
   // Sisi consumer (rule 5.6). Repo konkret memenuhi keduanya.
   type Repository interface {
       FindAccess(ctx context.Context, projectID, userID string) (p base.Project, memberRole string, found bool, err error)
       ListMembers(ctx context.Context, projectID string) ([]base.Member, error)
       InTx(ctx context.Context, fn func(TxRepository) error) error
   }
   type TxRepository interface {
       LockProject(ctx context.Context, id string) (base.Project, bool, error)
       UpdateProject(ctx context.Context, p base.Project) (updatedAt time.Time, err error)
   }

   type Input struct {
       ProjectID string
       Patch     base.ProjectPatch
       Meta      base.RequestMeta
   }

   type Usecase struct {
       repo   Repository
       audit  audit.Recorder
       counts counts.Counter
       clock  base.Clock
   }
   func NewUsecase(repo Repository, a audit.Recorder, c counts.Counter, clock base.Clock) *Usecase
   func (u *Usecase) Execute(ctx context.Context, in Input) (base.ProjectDetail, error)
   ```
   Alur `Execute`:
   1. `actor, ok := base.ActorFrom(reqctx.IdentityFrom(ctx))`; `!ok` -> `errs.New(errs.CodeUnauthorized, "perlu login")`.
   2. `_, memberRole, found, err := u.repo.FindAccess(ctx, in.ProjectID, actor.ID)`; `err` -> `fmt.Errorf("updateproject: cari akses: %w", err)`; `!found` -> `base.ErrProjectNotFound`.
   3. `level := base.ResolveLevel(actor.Role, memberRole, true)`; `if err := base.Require(level, base.PermEdit); err != nil { return base.ProjectDetail{}, err }`.
   4. ```go
      var (updated base.Project; changed []string; oldName string)
      err = u.repo.InTx(ctx, func(tx TxRepository) error {
          p, ok, err := tx.LockProject(ctx, in.ProjectID)
          if err != nil { return fmt.Errorf("updateproject: kunci project: %w", err) }
          if !ok { return base.ErrProjectNotFound }
          oldName = p.Name
          changed, err = p.ApplyUpdate(in.Patch, u.clock())
          if err != nil { return err }          // *errs.Error 400 dari entity; rollback
          if len(changed) > 0 {
              at, err := tx.UpdateProject(ctx, p)
              if err != nil { return fmt.Errorf("updateproject: simpan: %w", err) }
              p.UpdatedAt = at
          }
          updated = p
          return nil
      })
      ```
      Error `*errs.Error` dikembalikan apa adanya (tidak di-wrap) agar `respond.WriteError` memetakan status yang benar.
   5. Bila `len(changed) > 0`: `u.audit.Record(ctx, audit.Event{Action: record.ActionProjectUpdated, ProjectID: updated.ID, ProjectLabel: updated.Name, TargetType: "project", Target: updated.ID, Args: updateArgs(changed, oldName, updated.Name), Meta: in.Meta})`. `Record` tidak mengembalikan error (fail-open). Dipanggil di luar tx.
   6. `members, err := u.repo.ListMembers(ctx, updated.ID)`; error -> `fmt.Errorf("updateproject: daftar anggota: %w", err)`.
   7. `cs := u.counts.ForProjects(ctx, []string{updated.ID})[updated.ID]` (zero value `ContentCounts` = semua `nil` bila tidak ada).
   8. `sum := base.NewSummary(updated, level, memberRole, len(members), cs)`; `return base.ProjectDetail{ProjectSummary: sum, OwnerCount: base.OwnerCount(members)}, nil`.

   Fungsi privat `updateArgs(changed []string, oldName, newName string) map[string]any` membentuk `{"changed_fields": changed}` dan, hanya bila `"name"` ada di `changed`, menambah `name_from`/`name_to`. Ini perakitan payload, bukan rule bisnis. Usecase tidak melakukan validasi, tidak menulis SQL, tidak memanggil `bus` langsung.

4. **`repository.go`** (satu-satunya yang menyentuh SQL di feature ini):
   ```go
   type Repo struct { db *db.DB; s *store.Store }
   func newRepo(d *db.DB, s *store.Store) *Repo
   func (r *Repo) FindAccess(...)  // delegasi r.s.FindAccess
   func (r *Repo) ListMembers(...) // delegasi r.s.ListMembers
   func (r *Repo) InTx(ctx context.Context, fn func(TxRepository) error) error {
       return r.db.BeginTx(ctx, func(tx *sqlx.Tx) error {
           return fn(&txRepo{s: r.s.WithTx(tx), q: tx})
       })
   }
   type txRepo struct { s *store.Store; q sqlx.ExtContext }
   func (t *txRepo) LockProject(ctx, id) (base.Project, bool, error) { return t.s.LockProject(ctx, id) }
   func (t *txRepo) UpdateProject(ctx context.Context, p base.Project) (time.Time, error)
   ```
   SQL tunggal `UpdateProject` (parameterized, rule 5.3):
   ```sql
   UPDATE projects SET name = $2, description = $3, updated_at = $4 WHERE id = $1 RETURNING updated_at
   ```
   `sql.ErrNoRows` -> `base.ErrProjectNotFound` (tidak mungkin terjadi di bawah lock; jaring pengaman). Hasil `.UTC()`. Pelanggaran CHECK `projects_name_len`/`projects_description_len` dikembalikan sebagai error infra (500), karena entity sudah memvalidasi.

5. **`handler.go`**:
   ```go
   type handler struct{ uc *Usecase }
   func (h handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
       var req UpdateRequest
       if err := respond.DecodeJSON(r, &req); err != nil { respond.WriteError(w, r, err); return }
       out, err := h.uc.Execute(r.Context(), Input{
           ProjectID: chi.URLParam(r, "projectID"),
           Patch:     req.Patch(),
           Meta:      base.RequestMeta{IP: middleware.KeyByIP(r), UserAgent: truncate(r.UserAgent(), 512)},
       })
       if err != nil { respond.WriteError(w, r, err); return }
       respond.WriteJSON(w, http.StatusOK, out)
   }
   ```
   `truncate` (potong ke 512 rune/byte aman UTF-8) boleh helper privat di `handler.go`. Handler tidak memvalidasi uuid dan tidak me-log body.

6. **`init.go`**:
   ```go
   func Init(d app.Deps) {
       s := store.New(d.DB)
       uc := NewUsecase(
           newRepo(d.DB, s),
           audit.New(d.Bus, d.Log),
           counts.New(d.Bus, d.Log),
           base.Clock(func() time.Time { return time.Now().UTC() }),
       )
       d.API.With(middleware.RequireAuth()).Patch("/projects/{projectID}", handler{uc: uc}.ServeHTTP)
   }
   ```
   Pola penuh langsung di `d.API` dan nama param `{projectID}` (overview D4). Tanpa `Route`/`Mount`.

7. **Wiring**: tambah satu baris `updateproject.Init(d)` di `management/base.go` (PR usecase ini). Jangan menyentuh baris `Init` feature lain.

8. **Tes** sesuai bagian 6, urutan: unit usecase (RED -> hijau), handler, repository integrasi, integrasi feature.

9. **Verifikasi**: `cd backend && go build ./... && go vet ./... && go test ./internal/domain/project/internal/management/updateproject/...`; dengan `TEST_DATABASE_URL`: `go test -tags integration ./internal/domain/project/internal/management/updateproject/...`; `make api-check`; archtest `TestT00_AC4_RepoIsClean` lulus (tidak ada import antar-feature, SQL hanya di `repository.go`).

Kontrak bus (ringkas, sisi consumer):

| Aspek | Nilai |
|---|---|
| Audit | `bus.CallTyped[record.Response](ctx, b, constants.BUS_CORE_AUDIT_RECORD, record.Request{...})` lewat `audit.BusRecorder`; error di-log (`audit record gagal`) dan DITELAN; maksimal 1 Call per request, 0 pada no-op |
| Counts | tiga `bus.CallTyped[contentcount.Response]` berurutan lewat `counts.BusCounter`, 1 id per Call; error/`ErrNotRegistered` -> field `null` |
| `record.Request` | `ActorType: record.ActorUser`, `ActorID` = user id, `ActorLabel` = email, `ProjectID`, `ProjectLabel` = nama SETELAH update, `Action: record.ActionProjectUpdated`, `TargetType: "project"`, `Target` = project id, `Args`, `Status: record.StatusOK`, `IP`, `UserAgent`, `TraceID` (diisi `audit.BusRecorder`) |

## 4. Migration

Tidak ada. Tabel `projects` dan `project_members` dibuat migrasi foundation `backend/migrations/project/20261022090000000_create_projects.sql` (overview bagian 6). Usecase ini tidak menambah kolom atau indeks (UPDATE berdasarkan PK).

## 5. Changes (per file/folder)

| File | Aksi | Isi |
|---|---|---|
| `backend/internal/domain/project/internal/management/updateproject/init.go` | buat | rakit store -> repo -> usecase -> handler; `d.API.With(RequireAuth()).Patch("/projects/{projectID}", ...)` |
| `.../updateproject/handler.go` | buat | decode body, bangun `Input`/`RequestMeta`, `WriteJSON` 200 / `WriteError` |
| `.../updateproject/params.go` | buat | `UpdateRequest` (pointer) + `Patch()` |
| `.../updateproject/usecase.go` | buat | interface `Repository`/`TxRepository`, `Input`, `Usecase.Execute`, `updateArgs` |
| `.../updateproject/repository.go` | buat | `Repo` (delegasi ke `store`), `InTx`, `txRepo.UpdateProject` (satu UPDATE ... RETURNING) |
| `.../updateproject/usecase_test.go` | buat | unit dengan repo/audit/counts palsu |
| `.../updateproject/handler_test.go` | buat | `httptest`, usecase palsu |
| `.../updateproject/repository_test.go` | buat | integrasi (tag `integration`, `db.TestDB`) |
| `.../updateproject/updateproject_integration_test.go` | buat | integrasi HTTP penuh (tag `integration`) |
| `backend/internal/domain/project/internal/management/base.go` | ubah | tambah `updateproject.Init(d)` |
| `api/modules/project/management.yml`, `api/openapi.yml` | verifikasi, ubah hanya bila selisih | operasi `updateproject` (bagian 8) |
| `frontend/src/domain/project/management/updateproject/` | lihat `frontend-plan.md` | form Settings (nama, deskripsi); di luar cakupan backend plan ini |

Tidak diubah: `base/` (kecuali bug yang ditemukan; bila rule perlu berubah, ubah di `base/` beserta tesnya, bukan di usecase), `base/store`, `infra/**`, feature lain, `go.mod`, konfigurasi.

## 6. Test Scenario

Penamaan: tes kriteria `TestT03_ACn_*`; lainnya `Test<Unit>_<Kasus>`. Rule bisnis `ApplyUpdate`/`Require`/`ResolveLevel` sudah dites di `base/` (F); tes di sini membuktikan orkestrasi dan wiring, tanpa menduplikasi tabel rule.

### Unit usecase (`usecase_test.go`; repo, audit, counts, clock palsu)

Repo palsu mencatat urutan panggilan, menyimpan `Project` terkunci, dan bisa dipaksa error per metode. Audit palsu menyimpan `Event`. Identitas disuntik lewat `reqctx.WithIdentity`.

- `TestT03_AC2_MemberCanEditName`: aktor developer, `memberRole="member"`, patch `{Name:"Baru"}` -> `LockProject` dipanggil, `UpdateProject` dipanggil sekali dengan nama "Baru", respons `Name=="Baru"`, `AccessLevel=="member"`, `MyRole=="member"`.
- `TestUpdate_OwnerEdits`: owner -> sukses, `AccessLevel=="owner"`.
- `TestUpdate_SuperAdminNotMember`: aktor `super_admin`, `memberRole=""` -> sukses, `AccessLevel=="admin"`, `MyRole==nil`, `IsMember==false`.
- `TestUpdate_NonMemberGets404`: `found=true`, `memberRole=""`, developer -> `errors.Is(err, base.ErrProjectNotFound)`; `InTx`, audit, counts tidak dipanggil.
- `TestUpdate_ProjectNotFound`: `found=false` -> `ErrProjectNotFound`, tidak ada tx.
- `TestUpdate_DeletedBetweenCheckAndLock`: `FindAccess` found, `LockProject` ok=false -> `ErrProjectNotFound`; `UpdateProject` tidak dipanggil; audit kosong.
- `TestUpdate_EmptyPatch400`: patch `{}` -> `*errs.Error` `CodeInvalid`, `details.fields.body == "minimal satu field (name, description) wajib diisi"`; `UpdateProject` tidak dipanggil; audit kosong. Urutan: otorisasi lebih dulu (non-anggota dengan patch kosong tetap 404).
- `TestUpdate_ValidationError`: nama `"A"` -> 400 `fields.name`; deskripsi 1001 rune -> `fields.description`; keduanya salah -> kedua kunci ada; tidak ada UPDATE dan tidak ada audit.
- `TestUpdate_NoopSkipsWriteAndAudit`: patch nama sama persis dengan yang tersimpan (juga dengan spasi tepi `"  Payments  "`) -> sukses 200, `UpdateProject` TIDAK dipanggil, audit 0 event, `UpdatedAt` sama dengan sebelumnya.
- `TestUpdate_AuditArgsNameChanged`: nama berubah -> tepat 1 event: `Action==record.ActionProjectUpdated`, `ProjectLabel=="Baru"` (nama baru), `TargetType=="project"`, `Target==ID`, `Args["changed_fields"]==["name"]`, `name_from=="Lama"`, `name_to=="Baru"`, `Meta` sama dengan input.
- `TestUpdate_AuditArgsDescriptionOnly`: hanya deskripsi berubah -> `changed_fields==["description"]`, TIDAK ada `name_from`/`name_to`, dan `Args` tidak memuat nilai deskripsi.
- `TestUpdate_AuditArgsBoth`: keduanya berubah -> `changed_fields==["name","description"]` (urut), `name_from/to` ada.
- `TestUpdate_ClearDescription`: `Description` menunjuk `""` pada project berdeskripsi -> berubah (`["description"]`), UPDATE dengan deskripsi kosong.
- `TestUpdate_UsesRepoUpdatedAt`: `UpdateProject` mengembalikan waktu berbeda dari clock (mis. dibulatkan mikrodetik) -> respons `UpdatedAt` memakai nilai repo.
- `TestUpdate_AuditAfterTx`: urutan panggilan palsu = `FindAccess, InTx(begin), LockProject, UpdateProject, InTx(commit), audit.Record, ListMembers, counts`. Audit tidak pernah dipanggil di dalam fungsi tx.
- `TestUpdate_TxErrorNoAudit`: `UpdateProject` error -> error terbungkus (`errors.Is` ke error asal), audit 0 event, tidak ada respons.
- `TestUpdate_AuditFailureDoesNotFail`: audit palsu "gagal" (Recorder tidak mengembalikan error; simulasikan lewat `BusRecorder` di tes integrasi) -> respons tetap 200.
- `TestUpdate_ResponseCounts`: `ListMembers` mengembalikan 3 anggota (2 owner) -> `MemberCount==3`, `OwnerCount==2`; counts palsu `files=3` -> `Counts.Files==3`, `Specs/Keys==nil`; counts kosong -> semua `nil`.
- `TestUpdate_ListMembersErrorAfterCommit`: error `ListMembers` -> error 500 terbungkus, tetapi audit sudah terkirim satu kali (perubahan sudah commit). Perilaku didokumentasikan di komentar tes.
- `TestUpdate_NoIdentity`: konteks tanpa identitas -> `CodeUnauthorized`, tanpa panggilan repo.

### Unit handler (`handler_test.go`; usecase di balik interface palsu atau repo palsu)

- 200 dan body JSON sesuai bagian 8; `Content-Type: application/json`.
- Body `{"nama":"x"}` (field tak dikenal) -> 400 dari `DecodeJSON`; usecase tidak dipanggil.
- Body bukan JSON / `name` berupa angka (`{"name":5}`) -> 400.
- Body `{}` diteruskan ke usecase dengan `Patch{nil,nil}` (400 `fields.body` dibangkitkan usecase/entity, bukan handler).
- `{"name": null}` -> `Patch.Name==nil`.
- `chi.URLParam("projectID")` diteruskan apa adanya, termasuk `"bukan-uuid"` (handler tidak memvalidasi).
- `RequestMeta.UserAgent` dipotong 512; `IP` dari `middleware.KeyByIP`.
- Mapping error: `ErrProjectNotFound` -> 404 `NOT_FOUND`; `ErrForbidden` -> 403; `InvalidFields` -> 400 `INVALID_ARGUMENT`; error infra -> 500 `INTERNAL` tanpa detail internal.
- Tanpa sesi -> 401 (middleware `RequireAuth` terpasang di `Init`; dites di integrasi).

### Repository (`repository_test.go`, tag `integration`, Postgres nyata via `db.TestDB`)

- `UpdateProject` mengubah `name`, `description`, `updated_at`; `created_at`/`created_by` tidak berubah; mengembalikan `updated_at` mikrodetik UTC yang sama dengan hasil `SELECT`.
- `UpdateProject` pada id tak ada -> `ErrProjectNotFound`.
- `InTx`: error dari `fn` -> rollback (nama tidak berubah); sukses -> commit.
- Penguncian: tx A `LockProject` lalu tidur; tx B `LockProject` pada project yang sama memblokir sampai A commit, lalu melihat data hasil A (dua goroutine, kanal sinkron, batas waktu tes 5 detik).
- Pelanggaran CHECK (nama 1 karakter dipaksa lewat repo) -> error DB mentah (jaring pengaman; bukan jalur normal).

### Integrasi feature (`updateproject_integration_test.go`, tag `integration`)

Bus nyata (`bus.NewRegistry`), `db.TestDB`, router `chi` dengan middleware uji yang menyuntik identitas, handler palsu `core.audit.record` (menyimpan `record.Request`), `core.auth.get-user` bila diperlukan fixture, dan provider count palsu bila diuji. Project dan anggota dibuat langsung lewat SQL/`store` agar test tidak bergantung pada feature lain.

- `TestT03_AC2_MemberCanEditProject`: A owner, B member. B `PATCH {name:"Payments v2"}` -> 200, `name=="Payments v2"`, `access_level=="member"`, `my_role=="member"`, `updated_at` > sebelumnya; `SELECT` DB cocok; tepat 1 entri audit.
- `TestT03_AC2_MemberStillCannotManage`: B (member) tetap 403 pada `POST members`/`DELETE project` (regresi lintas fitur; boleh ditandai di `management_integration_test.go` W, di sini cukup memastikan PATCH tidak memberi hak lebih).
- `TestUpdate_OwnerAndSuperAdmin`: A owner -> 200 `access_level=="owner"`; S super_admin (bukan anggota) -> 200 `access_level=="admin"`, `my_role==null`, `is_member==false`.
- `TestT03_Security_NonMemberCannotUpdate` (rule 13.6): C developer non-anggota `PATCH` -> 404 `NOT_FOUND` "project tidak ditemukan" identik (byte-per-byte kecuali `trace_id`) dengan respons untuk uuid acak dan untuk `"bukan-uuid"`; baris DB tidak berubah; 0 audit.
- `TestUpdate_DeactivatedMemberStillInTable`: keanggotaan tercatat tetapi user nonaktif ditolak oleh middleware Session (`resolve-session`) sebelum usecase; di tes ini disimulasikan sesi ditolak -> 401. (Usecase sengaja tidak memanggil get-user, D6.)
- `TestUpdate_NoopReturns200NoAudit`: kirim nama yang sama -> 200, `updated_at` identik dengan sebelum, `SELECT updated_at` tidak berubah, 0 entri audit.
- `TestUpdate_EmptyBody400`: `{}` -> 400 `fields.body`; `{"name":null,"description":null}` -> 400 `fields.body`.
- `TestUpdate_Validation400`: nama `" "`, `"A"`, 101 rune, `"a\nb"` -> 400 `fields.name` (pesan sesuai 3.7); deskripsi 1001 rune dan `\x00` -> 400 `fields.description`; tidak ada perubahan DB dan 0 audit.
- `TestUpdate_TrimAndUnicode`: `"  Proyék Ñandú  "` -> tersimpan `"Proyék Ñandú"`; 100 rune multibyte lulus, 101 gagal.
- `TestUpdate_DescriptionOnlyAudit`: ubah hanya deskripsi -> `record.Request.Args` berisi `changed_fields:["description"]`, tanpa `name_from`/`name_to`, `ProjectLabel` = nama tak berubah; string deskripsi tidak muncul di JSON `Args`.
- `TestUpdate_AuditPayload`: ubah nama -> `record.Request`: `Action=="project.updated"`, `ActorType=="user"`, `ActorID`, `ActorLabel` = email, `ProjectID`, `ProjectLabel` = nama baru, `TargetType=="project"`, `Target`, `Status=="ok"`, `IP`, `UserAgent`, `TraceID` tidak kosong, `Args.name_from/name_to`.
- `TestUpdate_AuditFailOpen`: handler `core.audit.record` mengembalikan error, dan skenario lain: endpoint tidak terdaftar -> PATCH tetap 200, data tersimpan, log error `audit record gagal` tercatat (tangkap lewat `slog` handler uji), tanpa isi body di log.
- `TestT03_Counts_FailSoft_Update`: tanpa provider count -> 200, `counts` semua `null`; provider palsu files=3 -> `files:3`, `specs/keys:null`.
- `TestUpdate_ConcurrentPatches`: dua goroutine bersamaan, A mengubah nama jadi "X", B mengubah deskripsi jadi "Y" (diulang 20 kali) -> keduanya 200, hasil akhir memuat nama "X" DAN deskripsi "Y" (tidak ada lost update antar field karena `ApplyUpdate` bekerja pada baris terkunci terbaru), 2 entri audit.
- `TestUpdate_ConcurrentSameField`: dua PATCH nama berbeda bersamaan -> keduanya 200, nama akhir salah satunya, `changed_fields` tiap audit konsisten dengan nama yang terlihat saat itu (`name_from` entri kedua == `name_to` entri pertama).
- `TestUpdate_RaceWithDelete`: PATCH dan `DELETE` (SQL langsung) bersamaan -> PATCH 200 atau 404, tidak pernah 500; tidak ada baris yatim.
- `TestUpdate_CSRFAndAuth`: tanpa `X-Requested-With` -> 403 `CSRF_REQUIRED`; tanpa sesi -> 401; keduanya sebelum usecase.
- `TestUpdate_RouteRegistered`: `chi.Walk` memuat `PATCH /api/projects/{projectID}`; tidak ada bentrok pola dengan `GET/DELETE` pada path yang sama.

## 7. E2E Scenarios (Playwright terhadap binary; butuh T01 dan T02 merge, UI dari `frontend-plan.md`)

Positif:
1. Member B login -> membuka project "Payments" -> tab Settings -> mengubah nama menjadi "Payments v2" dan menyimpan -> toast sukses, judul workspace dan daftar di beranda menampilkan "Payments v2". Super Admin membuka audit log -> ada entri `project.updated` dengan `name_from: Payments`, `name_to: Payments v2`, aktor B.
2. Owner A mengosongkan deskripsi -> tersimpan, deskripsi tampil kosong setelah reload.
3. A menekan Simpan tanpa mengubah apa pun -> tombol Simpan nonaktif di UI (tidak ada request); bila request dikirim lewat API dengan nilai sama -> 200 dan audit log tidak bertambah.
4. Super Admin (bukan anggota) membuka project dari beranda -> mengubah deskripsi -> sukses; project tidak menjadikan Super Admin anggota.

Negatif:
1. Developer C (bukan anggota) membuka `/projects/<id>` langsung -> halaman "tidak ditemukan"; `PATCH` via API -> 404 yang sama dengan id acak.
2. Nama 1 karakter di form -> pesan field `name` (i18n) dan tidak ada request; lewat API -> 400 `fields.name`.
3. Deskripsi 1001 karakter -> pesan field `description`; lewat API -> 400.
4. `PATCH {}` via API -> 400 `fields.body`.
5. `PATCH` tanpa header `X-Requested-With` -> 403 `CSRF_REQUIRED`; tanpa login -> 401.
6. Dua tab: tab 1 menghapus project, tab 2 menyimpan perubahan -> UI menampilkan "tidak ditemukan" (API 404) dan kembali ke beranda.
7. Anggota yang baru dikeluarkan menyimpan perubahan dari tab yang masih terbuka -> 404 (bukan anggota lagi) dan UI menampilkan "tidak ditemukan".

## 8. Request/Response body

Format error seragam T00:

```json
{ "error": { "code": "NOT_FOUND", "message": "project tidak ditemukan", "details": null }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

### PATCH /api/projects/{projectID}

Header: `X-Requested-With: grimoire`, `Content-Type: application/json`. Minimal satu field (`name` atau `description`); field lain ditolak.

Request (ubah nama saja):
```json
{ "name": "Payments v2" }
```

Request (ubah keduanya):
```json
{ "name": "Payments v2", "description": "Layanan pembayaran internal, termasuk refund" }
```

Request (kosongkan deskripsi):
```json
{ "description": "" }
```

200 (berubah; sama persis bentuknya dengan `GET /api/projects/{projectID}`, oleh member B):
```json
{
  "id": "0192f5a0-1111-7aaa-8bbb-000000000001",
  "name": "Payments v2",
  "description": "Layanan pembayaran internal",
  "created_by": "0192f5a0-aaaa-7aaa-8bbb-00000000000a",
  "created_by_label": "a@tim.id",
  "created_at": "2026-10-22T09:00:00Z",
  "updated_at": "2026-10-22T10:30:12.481903Z",
  "my_role": "member",
  "access_level": "member",
  "is_member": true,
  "member_count": 2,
  "counts": { "files": null, "specs": null, "keys": null },
  "owner_count": 1
}
```

200 (Super Admin, bukan anggota): sama, dengan `"my_role": null`, `"access_level": "admin"`, `"is_member": false`.

200 (no-op, nilai sama setelah trim): bentuk sama, `updated_at` TIDAK berubah dari nilai tersimpan, tidak ada entri audit.

400 (body `{}` atau semua field `null`):
```json
{
  "error": {
    "code": "INVALID_ARGUMENT",
    "message": "permintaan tidak valid",
    "details": { "fields": { "body": "minimal satu field (name, description) wajib diisi" } }
  },
  "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A"
}
```

400 (validasi isi, kedua field salah):
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

400 (nama mengandung karakter kontrol): `details.fields.name == "nama project tidak boleh mengandung karakter kontrol"`. 400 (deskripsi mengandung karakter tidak valid): `details.fields.description == "deskripsi mengandung karakter tidak valid"`.

400 (field tak dikenal, mis. `{"nama":"x"}`, atau tipe salah): ditolak `respond.DecodeJSON` dengan `code: INVALID_ARGUMENT` (pesan dari T00).

401 (tanpa sesi): `{ "error": { "code": "UNAUTHENTICATED", ... } }` sesuai middleware T00.

403 (tanpa header CSRF): `{ "error": { "code": "CSRF_REQUIRED", ... } }` dari middleware T00. Tidak ada 403 `FORBIDDEN` dari usecase ini: semua level yang terlihat (member, owner, admin) memiliki `PermEdit`; level `none` selalu 404.

404 (bukan anggota, project tidak ada, atau `projectID` bukan uuid; respons identik):
```json
{ "error": { "code": "NOT_FOUND", "message": "project tidak ditemukan", "details": null }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

500: `{ "error": { "code": "INTERNAL", "message": "terjadi kesalahan", "details": null }, "trace_id": "..." }` (pesan generik sesuai T00; detail hanya di log server).

### Entri audit yang dikirim (`record.Request`, contoh nama berubah oleh member B)

```go
record.Request{
    ActorType: record.ActorUser, ActorID: "0192f5a0-bbbb-7aaa-8bbb-00000000000b", ActorLabel: "b@tim.id",
    ProjectID: "0192f5a0-1111-7aaa-8bbb-000000000001", ProjectLabel: "Payments v2",
    Action: record.ActionProjectUpdated, TargetType: "project", Target: "0192f5a0-1111-7aaa-8bbb-000000000001",
    Args: map[string]any{"changed_fields": []string{"name"}, "name_from": "Payments", "name_to": "Payments v2"},
    Status: record.StatusOK, IP: "10.0.0.5", UserAgent: "Mozilla/5.0", TraceID: "01JB2X9K6Q4Y7V3N8M5T2R1C0A",
}
```

Hanya deskripsi berubah: `Args: {"changed_fields": ["description"]}` (tanpa `name_from`/`name_to`, tanpa nilai deskripsi), `ProjectLabel` = nama saat ini. Kedua berubah: `changed_fields` = `["name","description"]`.

### Skema OpenAPI operasi (`api/modules/project/management.yml`, harus ada sejak F)

- `PATCH /api/projects/{projectID}`, `operationId: updateproject`, `tags: [projects]`.
- Parameter: `$ref` `XRequestedWith` (header, wajib) dan `ProjectID` (path, `format: uuid`).
- Request body `UpdateProjectRequest`: `properties: {name: {type: string, minLength: 2, maxLength: 100}, description: {type: string, maxLength: 1000}}`, `minProperties: 1`, `additionalProperties: false`.
- Respons: `200` `ProjectDetail`; `400` BadRequest; `401` Unauthorized; `403` Forbidden (CSRF); `404` NotFound (deskripsi: "juga untuk project yang bukan milik pemanggil (tidak bocor)"); `500` Internal. Semua `$ref: ../infra/foundation.yml#/components/responses/...`.
