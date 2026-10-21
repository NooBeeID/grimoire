# T06 / api-spec / deletespec — Plan Usecase

Mengikuti `docs/superpowers/plans/project-api-spec/overview.md` (selanjutnya "overview"). Bila bertentangan, overview menang. Plan ini mengasumsikan fase F (foundation) overview bagian 11 selesai: `base/` (`ErrProjectNotFound`, `ErrSpecNotFound`, `Actor`, `ActorFrom`, `RequestMeta`, `Clock`), `base/store` (`New`, `WithTx`, `LockSpec`, `CountEndpoints`), `base/access` (`Checker`), `base/audit` (`Recorder`, `Event`, `New`), migrasi `api_specs` + `api_spec_endpoints` (FK `ON DELETE CASCADE`), dan `api/modules/project/apispec.yml` memuat operasi `DELETE /projects/{projectID}/api-specs/{name}`. Bila ada simbol F yang belum ada, hentikan dan laporkan; jangan membuatnya di luar folder feature ini.

---

## 1. Metadata

| Atribut | Nilai |
|---|---|
| Ticket | T06 |
| Scope | fullstack pada tiket; plan usecase ini backend. Tombol/dialog hapus ada di `frontend-plan.md` (kontrak URL di bawah) |
| Domain | project |
| Module | api-spec (folder `apispec`) |
| Feature / usecase | `deletespec` |
| Resource root | `backend/internal/domain/project/internal/apispec` |
| Endpoint | `/api/projects/{projectID}/api-specs/{name}` |
| Method | DELETE |
| Version | Tanpa prefix versi di URL (prefix `/api`); `apispec.yml` `info.version: 0.1.0` |
| Auth | `middleware.RequireAuth()` + header CSRF `X-Requested-With: grimoire` (dipaksa middleware T00 untuk mutasi) |
| Siapa | anggota project (level member ke atas), super_admin |
| Respons sukses | 204 tanpa body |
| Bus dikonsumsi | `project.management.check-access` (Call, fail-closed, lewat `base/access`); `core.audit.record` (Call, fail-open, setelah commit, lewat `base/audit`) |
| Bus disediakan | tidak ada |
| Audit | `apispec.deleted` `{name, endpoint_count}` |
| Migrasi | tidak ada |
| Tabel | menulis `api_specs` (DELETE; `api_spec_endpoints` ikut cascade), membaca `api_spec_endpoints` (COUNT) |

---

## 2. Summary Plan

Menghapus satu spec beserta indeks endpoint-nya secara permanen (hard delete, rule 8.2). Alur usecase:

1. Ambil `actor` dari identitas (`base.ActorFrom(reqctx.IdentityFrom(ctx))`); tidak ada -> `errs.New(errs.CodeUnauthorized, "perlu login")`.
2. `guard.RequireMember(ctx, projectID)`: non-anggota, project tidak ada, `projectID` bukan uuid -> `base.ErrProjectNotFound` (404). Dicek SEBELUM mencari spec (overview 3.4) agar non-anggota tidak bisa membedakan "spec tidak ada" dari "bukan anggota".
3. Transaksi (`repo.InTx`, `BeginTx` dipanggil dari repository, bukan usecase):
   - `LockSpec(projectID, name)` (`SELECT ... FOR UPDATE`); tidak ada -> `base.ErrSpecNotFound` (404).
   - `CountEndpoints(specID)` -> `endpointCount` (snapshot untuk audit, dihitung di dalam tx setelah lock sehingga konsisten dengan baris yang dihapus).
   - `DeleteSpec(specID)`; `api_spec_endpoints` terhapus oleh FK cascade. Tidak ada DELETE manual per tabel.
4. Setelah commit: `audit.Record(apispec.deleted {name, endpoint_count})`. Gagal audit hanya di-log (fail-open); respons tetap 204.
5. Handler menulis `204 No Content`.

Keputusan feature ini:
- Tanpa body request dan tanpa konfirmasi nama (tiket tidak meminta; konfirmasi adalah urusan dialog FE). Body diabaikan.
- Cascade sepenuhnya oleh DB (`api_spec_endpoints.spec_id ... ON DELETE CASCADE`). Usecase hanya `DELETE FROM api_specs WHERE id = $1`. Dibuktikan oleh tes integrasi (endpoint indeks hilang) dan tes `pg_constraint` T03 AC6.
- `DELETE` dilakukan berdasarkan `id` baris yang terkunci, bukan `(project_id, name)`, sehingga tidak bisa menghapus spec lain bila nama berubah di antara lock dan delete. Dengan lock `FOR UPDATE` itu mustahil, tetapi kondisi `project_id = $2` tetap disertakan sebagai sabuk pengaman isolasi antar project (rule 6.10 semangat yang sama).
- Konkurensi: dua DELETE bersamaan pada spec yang sama -> yang kedua menunggu lock; setelah yang pertama commit, `LockSpec` tidak menemukan baris -> 404. Tepat satu 204 dan tepat satu audit. DELETE vs `renamespec` paralel: pemenang lock menentukan; yang kalah melihat nama lama hilang -> 404 (dapat diterima, klien memuat ulang daftar).
- DELETE vs `uploadspecs` paralel untuk nama yang sama: `uploadspecs` memegang advisory lock project lalu `LockSpec`; `deletespec` hanya `LockSpec`. Serialisasi cukup dari row lock: bila upload menang, delete menghapus versi baru (urutan sah); bila delete menang, upload melihat spec hilang dan membuatnya `created`. Tidak perlu advisory lock di sini.
- Audit satu aksi = tepat satu entri. Penghapusan gagal (401/404/500) = 0 entri. Bus tidak pernah dipanggil di dalam tx (check-access sebelum tx, audit sesudah commit).
- Audit `Target` = nama spec; `Args` tanpa isi spec. Entri tetap ada setelah spec hilang (T02 tanpa FK).
- Nama yang dihapus bisa langsung dipakai lagi oleh upload/rename (unik per project hanya untuk baris hidup).
- Sentinel `*errs.Error` milik `base` tidak dimodifikasi.

---

## 3. Plan (langkah berurutan)

Urutan wajib; tulis test dulu (TDD), lalu implementasi.

**D0. Verifikasi (tanpa edit).** Pastikan ada: `base.ActorFrom`, `base.RequestMeta`, `base.Clock`, `base.ErrSpecNotFound`, `base.ErrProjectNotFound`, `store.Store` (`LockSpec`, `CountEndpoints`, `WithTx`), `access.Checker`/`access.New`, `audit.Recorder`/`audit.Event`/`audit.New`, `record.ActionAPISpecDeleted`, migrasi `api_specs`/`api_spec_endpoints` dengan cascade. Baca `internal/infra/db` (`DB.BeginTx(ctx, func(*sqlx.Tx) error)`, `TestDB`), `respond.WriteError`, `middleware.RequireAuth`/`KeyByIP`. Bila `renamespec` atau `uploadspecs` sudah ada, ikuti pola `InTx`/`repository.go`/`init.go`-nya persis.

**D1. Cek kontrak OpenAPI** (rule 11.6) di `api/modules/project/apispec.yml`: operasi `delete` pada `/projects/{projectID}/api-specs/{name}` (`operationId: deleteApiSpec`), parameter `XRequestedWith`, `ProjectID`, `SpecName`; response `204`, `401`, `403` (CSRF), `404`. Bila F belum lengkap, tambahkan (fragmen di bagian 8), lalu `make api-bundle` dan `make api-check`.

**D2. `deletespec/params.go`**

```go
package deletespec

// Params diambil apa adanya dari URL; tidak divalidasi di handler (overview 3.3).
// Nama tidak divalidasi bentuknya: nama tak valid cukup tidak ditemukan (404),
// agar respons identik dengan "spec tidak ada" dan tidak membuka oracle.
type Params struct {
    ProjectID string
    Name      string
}
```

**D3. `deletespec/usecase.go`** (orkestrasi saja; tanpa SQL, tanpa HTTP, rule 5.2/5.6)

```go
package deletespec

type txRepo interface {
    LockSpec(ctx context.Context, projectID, name string) (base.Spec, bool, error)
    CountEndpoints(ctx context.Context, specID string) (int, error)
    DeleteSpec(ctx context.Context, projectID, specID string) (bool, error) // true bila 1 baris terhapus
}

type repository interface {
    InTx(ctx context.Context, fn func(txRepo) error) error
}

type Usecase struct {
    repo  repository
    guard access.Checker
    audit audit.Recorder
}

func NewUsecase(repo repository, guard access.Checker, rec audit.Recorder) *Usecase

func (u *Usecase) Execute(ctx context.Context, p Params, meta base.RequestMeta) error {
    if _, ok := base.ActorFrom(reqctx.IdentityFrom(ctx)); !ok {
        return errs.New(errs.CodeUnauthorized, "perlu login")
    }
    if err := u.guard.RequireMember(ctx, p.ProjectID); err != nil {
        return err
    }
    var (
        name  string
        count int
    )
    err := u.repo.InTx(ctx, func(tx txRepo) error {
        sp, found, err := tx.LockSpec(ctx, p.ProjectID, p.Name)
        if err != nil {
            return fmt.Errorf("deletespec: lock spec: %w", err)
        }
        if !found {
            return base.ErrSpecNotFound
        }
        if count, err = tx.CountEndpoints(ctx, sp.ID); err != nil {
            return fmt.Errorf("deletespec: count endpoints: %w", err)
        }
        deleted, err := tx.DeleteSpec(ctx, p.ProjectID, sp.ID)
        if err != nil {
            return fmt.Errorf("deletespec: delete spec: %w", err)
        }
        if !deleted { // mustahil di bawah lock; jaga-jaga
            return base.ErrSpecNotFound
        }
        name = sp.Name
        return nil
    })
    if err != nil {
        return err
    }
    u.audit.Record(ctx, audit.Event{
        Action:    record.ActionAPISpecDeleted,
        ProjectID: p.ProjectID,
        Target:    name,
        Args:      map[string]any{"name": name, "endpoint_count": count},
        Meta:      meta,
    })
    return nil
}
```

Catatan: `*errs.Error` dari dalam callback (`ErrSpecNotFound`) lewat `InTx` tanpa dibungkus agar `errors.Is`/`respond` tetap memetakannya ke 404 (repository `InTx` hanya me-rollback dan meneruskan error apa adanya). Tidak ada rule bisnis baru di usecase; tidak ada entity baru.

**D4. `deletespec/repository.go`** (satu-satunya yang menyentuh SQL feature ini)

```go
package deletespec

type Repo struct {
    db *db.DB
    s  *store.Store
}

func newRepo(d *db.DB, s *store.Store) *Repo

func (r *Repo) InTx(ctx context.Context, fn func(txRepo) error) error {
    return r.db.BeginTx(ctx, func(tx *sqlx.Tx) error {
        return fn(&txRepoImpl{s: r.s.WithTx(tx), q: tx})
    })
}

type txRepoImpl struct {
    s *store.Store // LockSpec, CountEndpoints
    q db.Querier
}

func (t *txRepoImpl) LockSpec(ctx, projectID, name) (base.Spec, bool, error)   { return t.s.LockSpec(ctx, projectID, name) }
func (t *txRepoImpl) CountEndpoints(ctx, specID) (int, error)                  { return t.s.CountEndpoints(ctx, specID) }

// DeleteSpec: satu-satunya SQL milik feature ini. Endpoint indeks ikut terhapus oleh FK cascade.
//   DELETE FROM api_specs WHERE id = $1 AND project_id = $2
// RowsAffected == 1 -> true. id/projectID bukan uuid -> (false, nil) tanpa query.
func (t *txRepoImpl) DeleteSpec(ctx, projectID, specID string) (bool, error)
```

Parameter selalu bind (`$1`, `$2`), tidak ada interpolasi (rule 8.x). Tidak ada `DELETE FROM api_spec_endpoints` manual.

**D5. `deletespec/handler.go`**

```go
type Handler struct{ uc *Usecase }

func newHandler(uc *Usecase) *Handler { return &Handler{uc: uc} }

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    p := Params{
        ProjectID: chi.URLParam(r, "projectID"),
        Name:      chi.URLParam(r, "name"),
    }
    meta := base.RequestMeta{IP: middleware.KeyByIP(r), UserAgent: truncate(r.UserAgent(), 512)}
    if err := h.uc.Execute(r.Context(), p, meta); err != nil {
        respond.WriteError(w, r, err)
        return
    }
    w.WriteHeader(http.StatusNoContent)
}
```

`truncate` memakai helper bersama bila F menyediakannya (mis. di `base/meta.go`); bila belum, tulis fungsi lokal kecil `truncate(s string, n int) string` pemotong rune-aman di `handler.go` (jangan membuat helper di `base` dari plan ini). Handler tidak membaca body. Log tidak memuat isi spec.

**D6. `deletespec/init.go`**

```go
func Init(d app.Deps) {
    clock := base.Clock(func() time.Time { return time.Now().UTC() })
    s := store.New(d.DB)
    uc := NewUsecase(newRepo(d.DB, s), access.New(d.Bus), audit.New(d.Bus, d.Log, clock))
    h := newHandler(uc)
    d.API.With(middleware.RequireAuth()).
        Delete("/projects/{projectID}/api-specs/{name}", h.ServeHTTP)
}
```

Tanpa `BodyLimit` (tidak ada body yang dibaca).

**D7. Wiring**: tambahkan satu baris `deletespec.Init(d)` di `backend/internal/domain/project/internal/apispec/base.go` (`Register`).

**D8. Tes** (bagian 6): tulis unit test usecase/handler/repository-lokal lalu tes integrasi.

**D9. FE (kontrak saja)**: tombol "Hapus" + dialog konfirmasi di halaman detail spec memanggil `DELETE /api/projects/{projectID}/api-specs/{name}` dengan header `X-Requested-With: grimoire`; sukses 204 -> kembali ke daftar spec. Implementasi di `frontend-plan.md`.

**D10. Verifikasi akhir**: `go vet ./...`, `golangci-lint`, `go test ./internal/domain/project/internal/apispec/...`, `go test -tags integration ./internal/domain/project/internal/apispec/...`, `make api-check`, archtest `TestT00_AC4_RepoIsClean`.

Kontrak bus: tidak ada yang baru.
- Dikonsumsi `project.management.check-access`: `checkaccess.Request{ProjectID, UserID, UserRole}` -> `Response{ProjectExists bool; Level}` (semantik `RequireMember` sesuai overview 3.4).
- Dikonsumsi `core.audit.record`: `record.Request{ActorType: ActorUser, ActorID: UserID, ActorLabel: Name|Email, ProjectID, Action: record.ActionAPISpecDeleted, TargetType: "api_spec", Target: <name>, Args: {name, endpoint_count}, Status: StatusOK, IP, UserAgent, TraceID}`.

---

## 4. Migration

Tidak ada. Memakai `backend/migrations/project/<VERSION>_create_api_specs.sql` (foundation). Yang dibutuhkan dari migrasi itu: `api_spec_endpoints.spec_id REFERENCES api_specs(id) ON DELETE CASCADE`. Tidak ada indeks baru: lock/lookup `(project_id, name)` dilayani `api_specs_project_name_key`; hapus endpoint cascade dilayani PK `(spec_id, method, path)` (prefix `spec_id`).

---

## 5. Changes (per file/folder)

Baru, `backend/internal/domain/project/internal/apispec/deletespec/`:
- `init.go` — `Init(d app.Deps)`, route DELETE.
- `handler.go` — path param, meta, panggil usecase, `204`.
- `params.go` — `Params{ProjectID, Name}`.
- `usecase.go` — `Usecase`, interface `repository`/`txRepo`, orkestrasi.
- `repository.go` — `Repo.InTx`, `DeleteSpec` (SQL), pembungkus `store.LockSpec`/`CountEndpoints`.
- `usecase_test.go`, `handler_test.go` — unit (fake repo, fake guard, fake recorder).
- `repository_integration_test.go`, `deletespec_integration_test.go` (tag `integration`) — DB nyata.

Diubah:
- `backend/internal/domain/project/internal/apispec/base.go` — tambah `deletespec.Init(d)`.
- `backend/internal/domain/project/internal/apispec/routes_test.go` — pastikan pasangan `DELETE /api/projects/{projectID}/api-specs/{name}` ada (tahap W; bila file belum ada, W yang membuatnya).
- `api/modules/project/apispec.yml` — operasi `delete` bila belum lengkap, lalu `api/openapi.yml` hasil `make api-bundle`. `api/root.yml` hanya bila path belum terdaftar.

Tidak diubah: `base/**` (tidak ada rule/entity baru), `infra/**`, `cmd/**`, konstanta bus, kontrak bus, config, migrasi, README katalog (aksi audit `apispec.deleted` sudah ada di katalog), module lain.

---

## 6. Test Scenario

Penamaan: `TestT06_ACn_*` untuk kriteria penerimaan, selain itu `Test<Unit>_<Kasus>`.

### 6.1 Unit usecase (`usecase_test.go`, fake `repository`/`txRepo`, fake `access.Checker`, fake `audit.Recorder`)
- Sukses: urutan panggilan tepat `RequireMember` -> `InTx{LockSpec, CountEndpoints, DeleteSpec}` -> `audit.Record`; audit menerima `Action == record.ActionAPISpecDeleted`, `ProjectID`, `Target == name`, `Args == {name, endpoint_count: N}`, `Meta` diteruskan; `Args` tidak memuat kunci `content`.
- Tanpa identitas di ctx -> `CodeUnauthorized`; guard, repo, audit TIDAK dipanggil.
- Guard `ErrProjectNotFound` -> diteruskan; repo dan audit tidak dipanggil (asersi hitungan 0; urutan akses dulu).
- Guard error infra -> error diteruskan (bukan nil), repo/audit tidak dipanggil.
- `LockSpec found=false` -> `errors.Is(err, base.ErrSpecNotFound)`; `CountEndpoints`/`DeleteSpec` tidak dipanggil; audit tidak dipanggil.
- `LockSpec` error -> error terbungkus `deletespec: lock spec:`, bukan `*errs.Error` bisnis (-> 500); audit 0.
- `CountEndpoints` error -> terbungkus, `DeleteSpec` tidak dipanggil, audit 0.
- `DeleteSpec` error -> terbungkus; audit 0.
- `DeleteSpec` mengembalikan `false` -> `ErrSpecNotFound`, audit 0.
- `InTx` mengembalikan error commit -> error diteruskan, audit 0.
- `endpoint_count` 0 (spec tanpa endpoint, mis. `webhooks-only`) -> `Args["endpoint_count"] == 0`, tetap sukses.
- Audit `Record` tidak mengembalikan error (fail-open): usecase sukses meski recorder mencatat kegagalan internal.
- `projectID` dan `name` diteruskan persis ke `LockSpec`.

### 6.2 Unit handler (`handler_test.go`, `httptest` + chi router + usecase fake)
- Sukses: status 204, body kosong, tidak ada `Content-Type` JSON.
- Param `projectID` dan `name` dibaca dari URL chi dan diteruskan; `RequestMeta.IP` terisi, `UserAgent` dipotong 512.
- 404 spec: JSON `{"error":{"code":"NOT_FOUND","message":"spec tidak ditemukan"},...}`; 404 project: `project tidak ditemukan`.
- 401 dan 500 memakai format error seragam; 500 tidak membocorkan pesan internal.
- Body request (apa pun) diabaikan, tidak memengaruhi hasil.

### 6.3 Integrasi repository (`repository_integration_test.go`, tag `integration`, `db.TestDB(t)`)
- `DeleteSpec` menghapus tepat satu baris; baris `api_spec_endpoints` spec itu hilang (cascade); spec lain di project yang sama tidak tersentuh.
- `DeleteSpec` dengan `projectID` salah (project lain) -> `false`, baris tetap ada (sabuk pengaman isolasi).
- id bukan uuid -> `(false, nil)` tanpa query.
- `InTx`: error di dalam callback -> rollback (spec dan endpoint tetap ada).

### 6.4 Integrasi lintas komponen (`deletespec_integration_test.go`, tag `integration`, bus nyata, handler palsu `check-access` dengan peta project -> level, handler palsu `core.audit.record` yang merekam)
- `TestT06_ACdelete_IndexEndpointsGone`: upload `petstore-3.0.yaml` (via repository/insert langsung atau `uploadspecs`); `COUNT(*) FROM api_spec_endpoints WHERE spec_id=...` > 0; DELETE -> 204; baris `api_specs` dan seluruh baris `api_spec_endpoints` hilang; `list-endpoints` via bus untuk nama itu -> `SpecFound=false`; `GET .../api-specs/{name}` dan `.../raw` -> 404; `list-specs` tidak memuatnya.
- `TestT06_AC7_DeleteDoesNotTouchOtherProject`: project A dan B masing-masing punya spec bernama sama (`payment`) dan spec lain; hapus `payment` di A -> `payment` di B beserta endpoint-nya utuh; spec lain di A utuh; `count-specs` A turun 1, B tetap.
- `TestT06_AC7_NonMember404`: non-anggota -> 404 `project tidak ditemukan`; `projectID` bukan uuid -> 404 yang sama tanpa query ke `api_specs`; anggota B menghapus lewat `projectID` A -> 404; respons 404 non-anggota untuk spec yang ADA identik (status, kode, pesan) dengan spec yang TIDAK ADA (tanpa oracle); spec tetap ada setelah percobaan non-anggota; audit 0 kali.
- Anggota menghapus nama yang tidak ada di project sendiri -> 404 `spec tidak ditemukan`.
- Super admin (level `admin` dari check-access) non-anggota -> 204.
- `TestT06_ProjectDeleteCascadesSpecs` (bergantung T03 `deleteproject`/FK): hapus project (via `DELETE FROM projects` pada DB uji atau usecase T03 bila tersedia) -> `api_specs` dan `api_spec_endpoints` project itu hilang; project lain tidak tersentuh.
- Audit: sukses -> tepat 1 entri `apispec.deleted` dengan `Args {name, endpoint_count}` (nilai count sama dengan jumlah baris indeks sebelum hapus), `TargetType == "api_spec"`, `ActorLabel` benar; gagal (404/401) -> 0 entri.
- Audit handler gagal/tidak terdaftar -> respons tetap 204, spec tetap terhapus.
- Check-access tidak terdaftar (`ErrNotRegistered`) -> 500 (fail-closed), spec TIDAK terhapus.
- Konkurensi: dua DELETE paralel pada spec yang sama -> tepat satu 204 dan satu 404, tepat satu entri audit.
- Nama bisa dipakai ulang: setelah DELETE, upload nama yang sama -> `created` (bukan `updated`/`NAME_TAKEN`); rename spec lain ke nama itu -> 200.
- Setelah DELETE spec ke-`APISPEC_MAX_SPECS`, slot kembali: upload spec baru yang sebelumnya `SPEC_LIMIT_REACHED` kini `created`.
- Tanpa sesi -> 401 (`RequireAuth`); tanpa header CSRF -> 403 `CSRF_REQUIRED`; spec tidak terhapus pada kedua kasus.

### 6.5 Routing (`routes_test.go`)
- `DELETE /api/projects/{id}/api-specs/{name}` terdaftar.
- Spec bernama `upload`/`validate`/`raw`: `DELETE .../api-specs/upload` dan `.../validate` menghapus spec bernama itu (bukan 405), karena route statis hanya POST dan chi melanjutkan ke node `{name}` (overview 8).
- `DELETE .../api-specs` (tanpa nama) -> 405.

---

## 7. E2E Scenarios (Playwright; butuh T03 merge dan UI detail spec dari `frontend-plan.md`)

Positif:
1. Anggota membuka detail spec `payment`, klik "Hapus", konfirmasi di dialog -> kembali ke daftar; kartu `payment` hilang; spec lain tetap tampil.
2. Setelah hapus, membuka ulang URL detail `payment` -> halaman 404; tab pencarian endpoint tidak lagi memuat endpoint spec itu.
3. Hapus lalu unggah ulang `payment.yaml` -> hasil `created` (bukan `updated`).
4. Super Admin (bukan anggota) menghapus spec di project mana pun -> berhasil.
5. Membatalkan dialog konfirmasi -> spec tidak terhapus, tidak ada request DELETE.
6. Menghapus project (T03) yang memiliki spec -> spec tidak lagi dapat diakses (daftar project lain tidak berubah).

Negatif:
1. Developer bukan anggota memanggil DELETE pada spec project lain (via URL/konsol) -> 404; spec tetap ada di project asal.
2. Spec sudah dihapus di tab lain lalu klik "Hapus" -> 404; UI menampilkan pesan "spec tidak ditemukan" dan menyegarkan daftar, bukan galat kosong.
3. Pengguna tidak login -> 401, diarahkan ke login.
4. Request DELETE tanpa header CSRF -> 403 `CSRF_REQUIRED`, spec tetap ada.
5. Kegagalan server (500) -> dialog menampilkan pesan umum, spec tidak hilang dari daftar.

---

## 8. Request/Response body

### DELETE /api/projects/{projectID}/api-specs/{name}

Request: tanpa body. Header wajib `X-Requested-With: grimoire` dan cookie sesi.

Contoh: `DELETE /api/projects/0b6c1f3e-5a2d-4e7b-9c10-2f8a6d4e1b33/api-specs/payment`

Response 204 (tanpa body):

```
HTTP/1.1 204 No Content
```

Error (format seragam rule 11.1, `Content-Type: application/json`):

404 spec tidak ada (pemanggil anggota):

```json
{ "error": { "code": "NOT_FOUND", "message": "spec tidak ditemukan" }, "trace_id": "6f1c2a0b9d8e4f17" }
```

404 non-anggota, project tidak ada, atau `projectID` bukan uuid:

```json
{ "error": { "code": "NOT_FOUND", "message": "project tidak ditemukan" }, "trace_id": "6f1c2a0b9d8e4f17" }
```

401 tanpa sesi:

```json
{ "error": { "code": "UNAUTHORIZED", "message": "perlu login" }, "trace_id": "6f1c2a0b9d8e4f17" }
```

403 tanpa header CSRF:

```json
{ "error": { "code": "CSRF_REQUIRED", "message": "header X-Requested-With wajib" }, "trace_id": "6f1c2a0b9d8e4f17" }
```

500 (mis. check-access tidak tersedia atau DB error; pesan generik):

```json
{ "error": { "code": "INTERNAL", "message": "terjadi kesalahan internal" }, "trace_id": "6f1c2a0b9d8e4f17" }
```

(Kode/pesan 401, 403, dan 500 mengikuti `respond`/middleware T00 yang sudah ada; implementer menyalin nilai aktual ke `apispec.yml` dan tes, bukan mengarang.)

### Audit (`record.Request` yang dikirim lewat `base/audit`)

```go
record.Request{
    ActorType: record.ActorUser, ActorID: "<user uuid>", ActorLabel: "Budi",
    ProjectID: "0b6c1f3e-5a2d-4e7b-9c10-2f8a6d4e1b33", ProjectLabel: "",
    Action: record.ActionAPISpecDeleted, TargetType: "api_spec", Target: "payment",
    Args: map[string]any{"name": "payment", "endpoint_count": 12},
    Status: record.StatusOK, IP: "10.0.0.5", UserAgent: "Mozilla/5.0 ...", TraceID: "6f1c2a0b9d8e4f17",
}
```

### Fragmen OpenAPI (`api/modules/project/apispec.yml`)

```yaml
/projects/{projectID}/api-specs/{name}:
  delete:
    operationId: deleteApiSpec
    summary: Hapus spec beserta indeks endpoint-nya
    tags: [project-apispec]
    parameters:
      - $ref: '#/components/parameters/XRequestedWith'
      - $ref: '#/components/parameters/ProjectID'
      - $ref: '#/components/parameters/SpecName'
    responses:
      '204': { description: Spec dihapus permanen }
      '401': { $ref: '../infra/foundation.yml#/components/responses/Unauthorized' }
      '403': { $ref: '../infra/foundation.yml#/components/responses/Forbidden' }
      '404': { $ref: '../infra/foundation.yml#/components/responses/NotFound' }
```

(Nama `$ref` response menyesuaikan yang sebenarnya ada di `foundation.yml`; cek sebelum menulis. Operasi `delete` berbagi path item dengan `get` dan `patch` yang dimiliki usecase lain; jangan menimpa operasi mereka saat menggabungkan.)
