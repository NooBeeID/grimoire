# T06 project/api-spec — Usecase `renamespec`

Konteks bersama (migrasi `api_specs`, konstanta bus, kontrak, `base/`, `base/store`, `base/access`, `base/audit`, OpenAPI, i18n) ada di `overview.md` dan dikerjakan di langkah foundation F. Dokumen ini hanya memuat usecase `renamespec`. Bila bertentangan dengan `overview.md`, overview menang. Tiket: `docs/tickets/T06-project-api-spec.md`. Kriteria yang disentuh: AC7 (non-anggota 404) dan perilaku identitas/timpa pada bagian "Aturan bisnis" tiket (ubah nama selama unik).

## 1. Metadata

| Atribut | Nilai |
|---|---|
| Ticket | T06 |
| Scope | fullstack (backend di dokumen ini; dialog "Ubah nama" di `frontend-plan.md`) |
| Domain | project |
| Module | api-spec (folder Go `apispec`) |
| Feature | `renamespec` |
| Resource root | `backend/internal/domain/project/internal/apispec` |
| allowedPaths | `.../apispec/renamespec/`, baris `renamespec.Init(d)` di `.../apispec/base.go` |

| Endpoint | Method | Version | Akses |
|---|---|---|---|
| `/api/projects/{projectID}/api-specs/{name}` | PATCH | tanpa prefix versi (prefix `/api`; `info.version` 0.1.0 pada `api/modules/project/apispec.yml`) | anggota project, super_admin. Bukan anggota, project tidak ada, `projectID` bukan uuid: 404 |

Header wajib: `X-Requested-With: grimoire` (CSRF T00), `Content-Type: application/json`, sesi login (`RequireAuth`).

Kontrak bus yang dipakai (nama dan tipe persis plan pemilik; tidak ada endpoint bus baru):

| Endpoint | Mode | Provider | Dipakai untuk |
|---|---|---|---|
| `project.management.check-access` (`constants.BUS_PROJECT_MANAGEMENT_CHECK_ACCESS`) | Call, fail-closed | T03 | `access.Checker.RequireMember` |
| `core.audit.record` (`constants.BUS_CORE_AUDIT_RECORD`) | Call, fail-open, setelah commit | T02 | `audit.Recorder` aksi `record.ActionAPISpecUpdated` |

Tidak memanggil `core.auth.get-user` (overview D6).

## 2. Summary Plan

`renamespec` mengganti nama (slug) sebuah spec di dalam satu project. Satu usecase, tidak dipecah. Hanya kolom `name` dan metadata update yang berubah; `content`, `content_hash`, dan baris `api_spec_endpoints` tidak disentuh (endpoint terikat `spec_id`, bukan nama).

Pembagian tanggung jawab:

- `params.go`: validasi BENTUK saja (JSON valid, tanpa field tak dikenal lewat `respond.DecodeJSON`, field `name` wajib ada dan bukan `null`). Nama lama diambil handler dari `chi.URLParam(r, "name")` apa adanya.
- Entity `base.ValidateName` (sudah ada di foundation): rule format nama baru. Gagal -> `base.ErrInvalidName` (400, `INVALID_ARGUMENT`, `details.reason = INVALID_NAME`, `fields.name`).
- Entity `base.Spec.Rename(newName, actor, now) bool`: menentukan no-op (false bila nama sama) dan mengisi `Name`, `UpdatedAt`, `UpdatedBy`, `UpdatedByLabel`.
- Repository: satu-satunya yang menyentuh SQL, tx lewat `InTx`; memetakan unique violation menjadi `store.ErrNameTaken`.
- Usecase: orkestrasi saja.

Alur usecase:

1. Bangun `actor` dari `reqctx.IdentityFrom(ctx)` (`base.ActorFrom`); gagal -> 401.
2. `base.ValidateName(newName)`; gagal -> 400 `INVALID_NAME`. Dijalankan SEBELUM `RequireMember`: hasilnya hanya bergantung pada input, tidak membocorkan keberadaan project/spec. Konsekuensi: non-anggota yang mengirim nama tidak valid mendapat 400, non-anggota dengan nama valid mendapat 404 (diterima, tidak ada informasi project yang bocor).
3. `guard.RequireMember(ctx, projectID)` (overview 3.4). Bukan anggota / project tidak ada / bukan uuid -> `ErrProjectNotFound` (404). Dilakukan sebelum spec dicari.
4. Satu transaksi (`repo.InTx`):
   a. `LockSpec(projectID, oldName)` (`SELECT ... FOR UPDATE`); tidak ada -> `base.ErrSpecNotFound` (404).
   b. `spec.Rename(newName, actor, now)`; bila `false` (nama sama) -> no-op: baca ringkasan lewat `FindSpecMeta`, return tanpa UPDATE.
   c. `UPDATE api_specs SET name, updated_at, updated_by, updated_by_label WHERE id`; `23505` pada `api_specs_project_name_key` -> `store.ErrNameTaken` -> usecase memetakan ke `base.ErrNameTaken` (409 `CONFLICT`, `details.reason = NAME_TAKEN`).
   d. `FindSpecMeta(projectID, newName)` di dalam tx yang sama (ringkasan + `endpoint_count`, konsisten dengan baris yang baru di-UPDATE).
5. Setelah commit, hanya bila benar-benar berganti nama: `audit.Record` `apispec.updated` `{from, to}` (fail-open; kegagalan hanya di-log).
6. Handler menulis 200 `{"spec": <SpecDTO>}` lewat `base.ToSummaryDTO`.

Keputusan khusus usecase ini:

- **No-op** (nama baru identik dengan nama lama setelah lock): 200 dengan data spec saat ini, `updated_at` TIDAK berubah, tanpa UPDATE, tanpa audit (overview 6.3: no-op tidak diaudit).
- **Nama baru dipakai spec lain**: 409 `NAME_TAKEN`. Pengecekan dibiarkan ke unique constraint (tidak ada pre-check SELECT) agar bebas race; tx dibatalkan oleh error.
- **Nama lama tidak divalidasi bentuknya** (rule overview 3.3): nama lama apa pun yang tidak ketemu -> 404 tanpa kasus khusus.
- **Tidak ada kunci advisory proyek** (`LockProjectSpecs`): rename tidak mengubah jumlah spec, sehingga tidak berinteraksi dengan batas `APISPEC_MAX_SPECS`. Serialisasi dengan upload nama yang sama dijamin oleh `FOR UPDATE` pada baris spec dan unique constraint.
- **Rename lalu upload dengan nama lama**: tidak ada tombstone/alias. Nama lama bebas, jadi `uploadspecs` dengan nama lama membuat spec BARU (`created`, id berbeda). Spec yang sudah di-rename tetap tidak berubah. Diuji lintas feature.
- **Case**: `ValidateName` menolak huruf besar, jadi `Payment` -> 400, bukan dianggap sama dengan `payment`.
- Audit: `Target` = nama baru, `Args` hanya `{from, to}` (tanpa isi spec).

Prasyarat (dari F; bila belum ada hentikan dan selesaikan F dulu, jangan membuatnya dari folder `renamespec/`): migrasi `api_specs` (T06 + `projects` T03); `base` (`Spec`, `SpecSummary`, `ValidateName`, `Rename`, `ErrInvalidName`, `ErrNameTaken`, `ErrSpecNotFound`, `ErrProjectNotFound`, `Actor`, `ActorFrom`, `RequestMeta`, `Clock`, `SpecDTO`, `ToSummaryDTO`); `base/store` (`LockSpec`, `FindSpecMeta`, `WithTx`, `ErrNameTaken`); `base/access`; `base/audit`; konstanta/kontrak `record.ActionAPISpecUpdated`; entri OpenAPI `renamespec` di `apispec.yml`.

## 3. Plan (langkah berurutan)

Semua file baru di `backend/internal/domain/project/internal/apispec/renamespec/`. Urutan TDD: tulis tes dulu per langkah.

1. **Verifikasi prasyarat**: `go build ./...` lulus. `grep -n "renamespec" api/modules/project/apispec.yml` menunjukkan `PATCH /api/projects/{projectID}/api-specs/{name}` dengan `operationId: renamespec`, body `RenameApiSpecRequest {name (string, required)}`, respons 200 `{spec: ApiSpec}`, error 400/401/403 (CSRF)/404/409/500. Bila ada selisih dengan bagian 8 dokumen ini, perbaiki `apispec.yml` di PR ini, `make api-bundle`, commit `api/openapi.yml`, `make api-check` lulus (rule 11.6: spec sebelum handler).

2. **`params.go`** (package `renamespec`):

   ```go
   type renameRequest struct {
       Name *string `json:"name"`
   }

   // parseRename mendekode body dan memeriksa bentuk. Tidak memeriksa format nama (itu rule entity).
   // Body rusak / field tak dikenal -> 400 dari respond.DecodeJSON.
   // name hilang atau null -> 400 INVALID_ARGUMENT, details.fields.name = "wajib diisi".
   func parseRename(w http.ResponseWriter, r *http.Request) (newName string, err error)
   ```

   Gunakan `respond.DecodeJSON(w, r, &req)` (sesuai signature di `infra/http/respond`). Kegagalan field wajib: `errs.WithDetails(errs.CodeInvalid, "permintaan tidak valid", map[string]any{"fields": map[string]string{"name": "wajib diisi"}})`. String kosong `""` lolos params dan ditolak `ValidateName` (reason `INVALID_NAME`).

3. **`repository.go`**:

   ```go
   type Repo struct {
       s *store.Store
       db *db.DB // nil pada instance tx-scoped
   }
   func newRepo(d *db.DB, s *store.Store) *Repo

   // InTx membungkus d.BeginTx; fn menerima Tx yang terikat tx (s.WithTx(tx)). Usecase tidak melihat sqlx.
   func (r *Repo) InTx(ctx context.Context, fn func(Tx) error) error

   // Metode Tx (pada *Repo tx-scoped):
   func (r *Repo) LockSpec(ctx, projectID, name string) (base.Spec, bool, error)       // delegasi ke store.LockSpec
   func (r *Repo) FindSpecMeta(ctx, projectID, name string) (base.SpecSummary, bool, error) // delegasi ke store
   // RenameSpec menulis spec.Name, UpdatedAt, UpdatedBy, UpdatedByLabel (nilai dari entity, UTC) ke baris spec.ID.
   //   UPDATE api_specs SET name=$2, updated_at=$3, updated_by=$4, updated_by_label=$5 WHERE id=$1
   //   errors.As(err, &pgconn.PgError) dengan Code "23505" dan ConstraintName "api_specs_project_name_key"
   //   -> return store.ErrNameTaken. RowsAffected == 0 -> error infra "renamespec: rename: baris hilang" (mustahil
   //   karena FOR UPDATE; menjadi 500 bila terjadi).
   func (r *Repo) RenameSpec(ctx context.Context, spec base.Spec) error
   ```

   Hanya `repository.go` dan `base/store` yang berisi SQL. Row struct tidak diperlukan (UPDATE tanpa hasil baca).

4. **`usecase.go`**:

   ```go
   type Tx interface {
       LockSpec(ctx context.Context, projectID, name string) (base.Spec, bool, error)
       RenameSpec(ctx context.Context, spec base.Spec) error
       FindSpecMeta(ctx context.Context, projectID, name string) (base.SpecSummary, bool, error)
   }
   type Repository interface {
       InTx(ctx context.Context, fn func(Tx) error) error
   }

   type Usecase struct {
       repo  Repository
       guard access.Checker
       audit audit.Recorder
       now   base.Clock
   }
   func NewUsecase(repo Repository, guard access.Checker, rec audit.Recorder, now base.Clock) *Usecase

   type Input struct {
       ProjectID string
       OldName   string
       NewName   string
       Actor     base.Actor
       Meta      base.RequestMeta
   }

   func (u *Usecase) Execute(ctx context.Context, in Input) (base.SpecSummary, error)
   ```

   `Execute`:
   1. `if err := base.ValidateName(in.NewName); err != nil { return zero, err }`.
   2. `if err := u.guard.RequireMember(ctx, in.ProjectID); err != nil { return zero, err }`.
   3. `renamed := false; var out base.SpecSummary`. `err := u.repo.InTx(ctx, func(tx Tx) error {`
      - `spec, ok, err := tx.LockSpec(ctx, in.ProjectID, in.OldName)`; err -> bungkus `fmt.Errorf("renamespec: lock: %w", err)`; `!ok` -> `base.ErrSpecNotFound`.
      - `renamed = spec.Rename(in.NewName, in.Actor, u.now())`; bila `renamed`: `tx.RenameSpec(ctx, spec)`; `errors.Is(err, store.ErrNameTaken)` -> `base.ErrNameTaken`; error lain dibungkus `renamespec: rename: %w`.
      - `sum, ok, err := tx.FindSpecMeta(ctx, in.ProjectID, spec.Name)`; `!ok` -> error infra (mustahil dalam tx); `out = sum`.
      `})`; err -> return.
   4. `if renamed { u.audit.Record(ctx, audit.Event{Action: record.ActionAPISpecUpdated, ProjectID: in.ProjectID, Target: in.NewName, Args: map[string]any{"from": in.OldName, "to": in.NewName}, Meta: in.Meta}) }` (setelah commit; tidak mengembalikan error).
   5. `return out, nil`.

   Usecase tidak mengimpor `sqlx`, `pgconn`, atau SQL (rule 5).

5. **`handler.go`**:

   ```go
   type handler struct{ uc *Usecase }
   func newHandler(uc *Usecase) *handler
   func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request)
   ```

   Alur: `actor, ok := base.ActorFrom(reqctx.IdentityFrom(r.Context()))` (`!ok` -> `errs.New(errs.CodeUnauthorized, "perlu login")`); `newName, err := parseRename(w, r)`; `in := Input{ProjectID: chi.URLParam(r, "projectID"), OldName: chi.URLParam(r, "name"), NewName: newName, Actor: actor, Meta: base.RequestMeta{IP: middleware.KeyByIP(r), UserAgent: truncate(r.UserAgent(), 512)}}`; `sum, err := h.uc.Execute(r.Context(), in)`; sukses -> `respond.WriteJSON(w, http.StatusOK, renameResponse{Spec: base.ToSummaryDTO(sum)})` dengan `type renameResponse struct{ Spec base.SpecDTO \`json:"spec"\` }`; error -> `respond.WriteError(...)` (sesuai signature `infra/http/respond`). Urutan: aktor dulu, lalu params; error params tidak bergantung project. Fungsi `truncate` kecil privat di `handler.go` (potong pada batas rune, 512).

6. **`init.go`**:

   ```go
   func Init(d app.Deps) {
       clock := base.Clock(func() time.Time { return time.Now().UTC() })
       uc := NewUsecase(newRepo(d.DB, store.New(d.DB)), access.New(d.Bus), audit.New(d.Bus, d.Log, clock), clock)
       d.API.With(middleware.RequireAuth()).
           Patch("/projects/{projectID}/api-specs/{name}", newHandler(uc).ServeHTTP)
   }
   ```

   Route PATCH pada `{name}` tidak bentrok dengan route statis `upload`/`validate` (hanya POST); spec bernama `upload` tetap bisa di-rename (dibuktikan tes router di W, dan satu kasus di tes integrasi di bawah).

7. **Wiring**: tambah satu baris `renamespec.Init(d)` di `backend/internal/domain/project/internal/apispec/base.go` (import `.../apispec/renamespec`). Jangan mengubah baris `Init` feature lain; bila konflik merge, simpan keduanya.

8. **Tes** (bagian 6) dikerjakan bersama langkah 2-5; semua hijau, lalu `go vet ./...` dan `golangci-lint run` bersih.

9. **Verifikasi akhir**: `go test ./internal/domain/project/internal/apispec/renamespec/...`; `go test -tags integration ./internal/domain/project/internal/apispec/renamespec/...`; `make api-check`; archtest (`TestT00_AC4_RepoIsClean`) lulus (tidak ada import lintas feature/module, SQL hanya di repository/store).

## 4. Migration

Tidak ada. Memakai tabel `api_specs` dari migrasi foundation `backend/migrations/project/20261023090000600_create_api_specs.sql` (constraint `api_specs_project_name_key` dan `api_specs_name_check` sudah mencakup unik per project dan format nama). Tidak ada kolom/indeks baru: UPDATE memakai primary key `id`, lookup memakai `api_specs_project_name_key`.

## 5. Changes

Baru (semua di `backend/internal/domain/project/internal/apispec/renamespec/`):
- `init.go`: `Init(d app.Deps)`, rakitan dan route PATCH.
- `handler.go`: `handler`, `renameResponse`, `truncate`.
- `params.go`: `renameRequest`, `parseRename`.
- `usecase.go`: `Tx`, `Repository`, `Usecase`, `Input`, `Execute`.
- `repository.go`: `Repo`, `newRepo`, `InTx`, `LockSpec`, `FindSpecMeta`, `RenameSpec`.
- `params_test.go`, `usecase_test.go`, `repository_integration_test.go`, `handler_test.go`, `renamespec_integration_test.go`.

Diubah:
- `backend/internal/domain/project/internal/apispec/base.go`: tambah `renamespec.Init(d)`.
- `api/modules/project/apispec.yml`, `api/root.yml`, `api/openapi.yml` (generated): hanya bila langkah 1 menemukan selisih.

Tidak diubah: `base/`, `base/store`, `base/access`, `base/audit` (dipakai apa adanya; bila ternyata ada kekurangan, hentikan dan perbarui foundation/overview, bukan menambal dari folder feature), module/feature lain, `infra/**`, migrasi, `lib/constants`, `lib/contracts`.

## 6. Test Scenario

Penamaan: `TestT06_ACn_...` untuk kriteria penerimaan, selain itu `Test<Fungsi>_<Kasus>`.

### 6.1 Unit `params_test.go` (tanpa mock)

- Body `{"name":"payment-v2"}` -> nama terbaca.
- Body `{}` dan `{"name":null}` -> 400 `INVALID_ARGUMENT`, `fields.name = "wajib diisi"`.
- Body `{"name":"x","extra":1}`, JSON rusak, body kosong, `name` bertipe angka -> 400 (dari `DecodeJSON`).
- `{"name":""}` lolos params (ditolak di entity).

### 6.2 Unit `usecase_test.go` (repository, guard, audit fake; clock tetap)

- Sukses: spec `payment` -> `payment-v2`; `RenameSpec` dipanggil sekali dengan `Name = payment-v2`, `UpdatedBy = actor.ID`, `UpdatedByLabel = actor.Label`, `UpdatedAt = now`; return `SpecSummary` dari `FindSpecMeta`; audit dipanggil sekali dengan `Action = apispec.updated`, `Target = payment-v2`, `Args = {from: payment, to: payment-v2}` dan tanpa kunci lain.
- Nama tidak valid (`Bad Name`, `""`, 81 karakter, `a/b`, `..`): error `INVALID_NAME`; guard, repo, dan audit TIDAK dipanggil.
- Non-anggota (guard mengembalikan `ErrProjectNotFound`): error itu dikembalikan; repo tidak dipanggil, audit tidak dipanggil.
- Guard error infra: dikembalikan (500), repo tidak dipanggil.
- `LockSpec` -> not found: `ErrSpecNotFound`; audit tidak dipanggil; `RenameSpec` tidak dipanggil.
- Nama sama (`payment` -> `payment`): return ringkasan saat ini, `RenameSpec` TIDAK dipanggil, audit TIDAK dipanggil.
- `RenameSpec` mengembalikan `store.ErrNameTaken`: hasil `errors.Is(err, base.ErrNameTaken)`; audit TIDAK dipanggil.
- `RenameSpec`/`LockSpec` error infra lain: error terbungkus, bukan `*errs.Error`; audit tidak dipanggil.
- Audit gagal/fail-open: recorder fake tidak mengembalikan error (kontrak), hasil tetap sukses.
- Urutan panggilan (fake mencatat): `guard` -> `LockSpec` -> `RenameSpec` -> `FindSpecMeta` -> (commit) -> `audit`.

### 6.3 Handler `handler_test.go` (`httptest`, usecase dengan fake)

- 200 dengan body `{"spec": {...}}`, `Content-Type: application/json`; field JSON sesuai `SpecDTO` (termasuk `source_path` null, waktu UTC RFC3339).
- Pemetaan error ke status dan kode: 400 `INVALID_ARGUMENT` (`reason: INVALID_NAME`), 404 `NOT_FOUND`, 409 `CONFLICT` (`reason: NAME_TAKEN`), 401 tanpa identitas, 500 generik untuk error infra (tanpa membocorkan detail), `trace_id` selalu ada.
- `chi` URL param: `{name}` lama diteruskan persis (`payment`), `projectID` persis.

### 6.4 Integrasi repository `repository_integration_test.go` (tag `integration`, `db.TestDB(t)`)

- `RenameSpec` mengubah `name`, `updated_*`; `content`, `content_hash`, `created_*`, dan baris `api_spec_endpoints` tidak berubah; `endpoint_count` pada ringkasan sama.
- Nama sudah dipakai spec lain di project yang sama -> `store.ErrNameTaken` dan tx rollback (nama asli utuh).
- Nama yang sama di project LAIN tidak konflik (unique per project).
- `InTx`: error dari `fn` membatalkan perubahan.
- Dua rename paralel pada spec yang sama (goroutine, `FOR UPDATE`): yang kedua menunggu lalu melihat nama baru; hasil akhir konsisten, tidak ada baris ganda.
- Dua rename paralel dari dua spec berbeda ke nama tujuan yang sama: tepat satu sukses, satu `ErrNameTaken`.

### 6.5 Integrasi feature `renamespec_integration_test.go` (tag `integration`; router `chi` + middleware identitas uji; bus nyata dengan handler palsu `check-access` (peta project -> level) dan `core.audit.record` (merekam); fixture spec disisipkan lewat `base/store`/SQL bantu, bukan lewat `uploadspecs`, agar tes tidak bergantung feature lain)

- `TestT06_Rename_Success_OldNameGone`: rename `payment` -> `payment-v2` -> 200; query store: nama lama tidak ada (setara `GET /{name}` lama 404), nama baru ada (200); `endpoint_count`, `content_hash`, `size_bytes` identik.
- Konflik: rename ke nama spec lain -> 409 `NAME_TAKEN`; kedua spec tidak berubah; audit tidak terekam.
- Nama tidak valid -> 400 `INVALID_NAME` untuk `Bad Name`, `""`, `a/b`, 81 karakter; DB tidak berubah.
- `TestT06_AC7_Rename_NonMemberNotFound`: non-anggota (project ada) -> 404 `project tidak ditemukan`; `projectID` bukan uuid -> 404; spec milik project lain (anggota project A mengirim ke project B tempat dia bukan anggota) -> 404; spec project A tidak berubah. Super admin non-anggota -> 200 (level admin dari check-access).
- Spec tidak ada (`ghost`) oleh anggota -> 404 `spec tidak ditemukan`.
- Spec dari project lain dengan nama sama dipakai di project sendiri: anggota project A me-rename `payment` di project A saat project B juga punya `payment` -> hanya A yang berubah.
- No-op: rename `payment` -> `payment` -> 200; `updated_at` tidak berubah; audit tidak terekam.
- Audit: sukses menghasilkan tepat satu entri `apispec.updated`, `ActorLabel` dari identitas, `Target = payment-v2`, `Args {from, to}`; handler audit yang gagal -> respons tetap 200 (fail-open).
- Check-access gagal (`ErrNotRegistered`/error) -> 500, DB tidak berubah (fail-closed).
- Spec bernama `upload` dapat di-rename lewat `PATCH .../api-specs/upload` (route statis hanya POST) dan hasilnya 200.
- Tanpa header `X-Requested-With` -> 403 `CSRF_REQUIRED` (middleware T00 aktif di router uji bila tersedia; jika router uji tidak memasangnya, kasus ini dicakup E2E).
- `TestT06_Rename_ThenUploadOldName` (lintas feature; ditempatkan di `apispec_integration_test.go` pada langkah W setelah `uploadspecs` merge, tidak di folder ini): rename `payment` -> `payment-v2`, upload `payment.yaml` -> `created` dengan id baru; kedua spec ada, spec `payment-v2` tidak berubah.

## 7. E2E Scenarios (Playwright; butuh T03 merge dan UI dari `frontend-plan.md`)

Positif:
1. Anggota membuka halaman detail spec `payment`, klik "Ubah nama", mengisi `payment-v2` dan menyimpan -> dialog tertutup, judul/URL berubah ke `.../api-specs/payment-v2`, daftar spec menampilkan nama baru; membuka URL lama -> halaman 404.
2. Setelah rename, mengunggah `payment.yaml` -> muncul spec baru `payment` (`created`) di samping `payment-v2`.
3. Menyimpan tanpa mengubah nama (nama sama) -> dialog tertutup tanpa pesan error; `updated_at` tidak berubah.
4. Super Admin (bukan anggota) me-rename spec -> berhasil.

Negatif:
1. Mengisi nama yang sudah dipakai spec lain -> pesan konflik inline ("nama spec sudah dipakai di project ini" via i18n `NAME_TAKEN`), dialog tetap terbuka, nama tidak berubah.
2. Mengisi `Bad Name` / nama kosong / `a/b` -> pesan "nama spec tidak valid" pada field.
3. Developer bukan anggota membuka URL spec project lain -> halaman 404 (tombol ubah nama tidak pernah terjangkau); mengirim PATCH langsung -> 404.
4. Request PATCH tanpa header CSRF -> 403 `CSRF_REQUIRED`.
5. Spec dihapus pengguna lain saat dialog terbuka lalu disimpan -> pesan "spec tidak ditemukan" dan kembali ke daftar.

## 8. Request/Response body

Format error seragam (rule 11.1):

```json
{ "error": { "code": "NOT_FOUND", "message": "project tidak ditemukan" }, "trace_id": "6f1c2a0b9d8e4f17" }
```

### PATCH /api/projects/{projectID}/api-specs/{name}

Header: `X-Requested-With: grimoire`, `Content-Type: application/json`. Path `{name}` = nama lama.

Request:

```json
{ "name": "payment-v2" }
```

Response 200 (berganti nama maupun no-op; no-op mengembalikan data saat ini dengan `updated_at` lama):

```json
{
  "spec": {
    "name": "payment-v2",
    "title": "Payment API",
    "openapi_version": "3.0.3",
    "api_version": "1.4.0",
    "format": "yaml",
    "source_path": "specs/payment.yaml",
    "size_bytes": 40211,
    "endpoint_count": 2,
    "created_at": "2026-10-23T09:00:00Z",
    "created_by_label": "Budi",
    "updated_at": "2026-10-25T08:00:00Z",
    "updated_by_label": "Sari"
  }
}
```

Error:

400 nama tidak valid (rule entity):

```json
{
  "error": {
    "code": "INVALID_ARGUMENT",
    "message": "nama spec tidak valid: huruf kecil, angka, '-', '_', '.'; 1-80 karakter",
    "details": { "reason": "INVALID_NAME", "fields": { "name": "nama spec tidak valid" } }
  },
  "trace_id": "6f1c2a0b9d8e4f17"
}
```

400 field wajib hilang / `null`:

```json
{
  "error": {
    "code": "INVALID_ARGUMENT",
    "message": "permintaan tidak valid",
    "details": { "fields": { "name": "wajib diisi" } }
  },
  "trace_id": "6f1c2a0b9d8e4f17"
}
```

400 body rusak / field tak dikenal (dari `respond.DecodeJSON`; teks pesan mengikuti implementasi infra, kode tetap `INVALID_ARGUMENT`).

401 tanpa sesi:

```json
{ "error": { "code": "UNAUTHORIZED", "message": "perlu login" }, "trace_id": "6f1c2a0b9d8e4f17" }
```

403 tanpa header CSRF (dari middleware T00): `{"error":{"code":"CSRF_REQUIRED","message":"..."},"trace_id":"..."}`.

404 non-anggota / project tidak ada / `projectID` bukan uuid:

```json
{ "error": { "code": "NOT_FOUND", "message": "project tidak ditemukan" }, "trace_id": "6f1c2a0b9d8e4f17" }
```

404 spec tidak ada (hanya untuk anggota):

```json
{ "error": { "code": "NOT_FOUND", "message": "spec tidak ditemukan" }, "trace_id": "6f1c2a0b9d8e4f17" }
```

409 nama sudah dipakai:

```json
{
  "error": {
    "code": "CONFLICT",
    "message": "nama spec sudah dipakai di project ini",
    "details": { "reason": "NAME_TAKEN" }
  },
  "trace_id": "6f1c2a0b9d8e4f17"
}
```

500 (error infra, mis. check-access gagal, DB):

```json
{ "error": { "code": "INTERNAL", "message": "terjadi kesalahan" }, "trace_id": "6f1c2a0b9d8e4f17" }
```

(Kode dan pesan 500 persis mengikuti `respond.WriteError`; jangan hardcode di tes selain status 500 dan keberadaan `trace_id`.)

### Audit (`record.Request` yang dikirim setelah commit, hanya bila berganti nama)

```go
record.Request{
    ActorType: record.ActorUser, ActorID: "<user uuid>", ActorLabel: "Sari",
    ProjectID: "0b6c...", ProjectLabel: "",
    Action: record.ActionAPISpecUpdated, TargetType: "api_spec", Target: "payment-v2",
    Args: map[string]any{"from": "payment", "to": "payment-v2"},
    Status: record.StatusOK, IP: "10.0.0.5", UserAgent: "Mozilla/5.0 ...", TraceID: "6f1c2a0b9d8e4f17",
}
```

Tidak ada kontrak bus baru yang disediakan feature ini.
