# T06 / uploadspecs — Plan Usecase

Mengikuti `docs/superpowers/plans/project-api-spec/overview.md` (selanjutnya "overview"). Bila ada pertentangan, overview menang. Bagian overview yang dirujuk ditulis sebagai `OV x.y`.

---

## 1. Metadata

| Atribut | Nilai |
|---|---|
| Ticket | T06 |
| Scope | fullstack (plan ini hanya sisi backend; dialog upload ada di `frontend-plan.md`) |
| Domain | project |
| Module | api-spec (folder Go `apispec`) |
| Feature | `uploadspecs` |
| Resource root | `backend/internal/domain/project/internal/apispec` |
| Endpoint | `/api/projects/{projectID}/api-specs/upload` |
| Method | POST (`multipart/form-data`) |
| Version | tanpa prefix versi; `info.version: 0.1.0` pada `api/modules/project/apispec.yml` |
| Siapa | anggota project dan super_admin (non-anggota: 404) |
| Middleware | `middleware.RequireAuth()`, `middleware.BodyLimit(d.Config.MaxUploadBytes)`, CSRF `X-Requested-With: grimoire` (global T00) |
| Bus dikonsumsi | `project.management.check-access` (Call, fail-closed, via `base/access`); `core.audit.record` (Call, fail-open, setelah commit, via `base/audit`) |
| Bus disediakan | tidak ada |
| Bergantung pada | foundation F (OV 11) selesai: migrasi `api_specs`/`api_spec_endpoints`, config `APISPEC_*`, `base/`, `base/openapi`, `base/store`, `base/access`, `base/audit`, `base/upload`, entri OpenAPI |
| Acceptance criteria yang dipegang | T06-AC1, AC2, AC3, AC6, AC7 |

---

## 2. Summary Plan

`uploadspecs` menerima satu atau banyak file spec OpenAPI (field `files` berulang, field `name` opsional hanya bila tepat satu file), memvalidasi tiap file tanpa pernah mengakses jaringan/filesystem, lalu menyimpannya per project. Nama sama menimpa spec lama dan indeks endpoint lama diganti seluruhnya. Hasil dilaporkan per file (`created|updated|unchanged|skipped|rejected`) beserta alasan dan daftar issue (baris/JSON pointer).

Keputusan khusus usecase ini (turunan OV 3.5, tidak mengubah overview):

| # | Keputusan | Alasan |
|---|---|---|
| U1 | Seluruh body dibaca dulu lewat `upload.Read`, baru diproses. Error baca/413/400-multipart terjadi sebelum `RequireMember`, tetapi tidak bergantung pada project sehingga tidak membocorkan keberadaan project. `RequireMember` adalah langkah pertama di usecase | Handler tidak boleh berisi logika akses; 413 harus membatalkan seluruh request tanpa simpanan parsial |
| U2 | File diproses berurutan sesuai urutan part. Parse/validasi di luar tx; satu tx kecil per file | OV 3.5 / D5 |
| U3 | Dedup nama dalam request: sebuah nama "dipesan" hanya bila file pemiliknya berakhir `created/updated/unchanged`. File berikutnya ber-nama sama menjadi `rejected DUPLICATE_NAME_IN_UPLOAD`. File pertama yang `rejected` tidak memesan nama | Dua file tidak boleh saling menimpa dalam satu request, tetapi file rusak tidak boleh menghalangi file benar |
| U4 | Pengecekan dedup dilakukan SEBELUM `openapi.Parse` (hemat CPU) | Urutan dari spesifikasi tugas |
| U5 | Error infrastruktur pada file ke-N (DB, ctx dibatalkan) membatalkan sisa request dengan 500; file sebelumnya yang sudah commit tetap tersimpan dan sudah diaudit. Mengunggah ulang aman karena hasilnya `unchanged` | Menyamarkan error infra sebagai `rejected` menyesatkan |
| U6 | `23505` (`store.ErrNameTaken`) pada INSERT hanya mungkin bila `renamespec` memindahkan spec lain ke nama itu bersamaan (rename hanya mengunci baris, bukan project). Tx diulang SATU kali (sekarang `LockSpec` menemukan baris -> Update). Gagal lagi -> error infra (U5) | Tanpa kode alasan baru |
| U7 | Audit memakai `context.WithoutCancel(ctx)` agar entri tetap terkirim bila klien memutus koneksi setelah commit | Commit sudah terjadi; audit tidak boleh hilang karena ctx batal |
| U8 | `name` di-`strings.TrimSpace`; kosong setelah trim dianggap tidak diisi. Tidak di-lowercase atau dinormalkan (nama tidak valid -> 400 `INVALID_NAME`) | Pengguna memilih nama sengaja; salah ketik harus terlihat |
| U9 | `name` tidak valid -> 400 untuk seluruh request (bukan per file), karena field hanya sah untuk satu file | `base.ErrInvalidName` sudah berbentuk 400 dengan `details.fields.name` |

---

## 3. Plan (langkah berurutan)

Prasyarat: foundation F selesai dan hijau (`go test ./...`, `make api-check`). Seluruh kode di bawah `backend/internal/domain/project/internal/apispec/uploadspecs/`.

### Langkah 1 — Verifikasi OpenAPI (rule 11.6, sebelum kode)

Di `api/modules/project/apispec.yml` pastikan operasi `POST /projects/{projectID}/api-specs/upload` ada dengan: parameter `XRequestedWith` dan `ProjectID`; requestBody `multipart/form-data` dengan properti `files` (array `string/binary`, wajib, minItems 1) dan `name` (string, opsional, pattern `^[a-z0-9._-]{1,80}$`); respons 200 `ApiSpecUploadResult`, 400, 401, 403, 404, 413, 500 (`Error` dari `foundation.yml`). Skema `ApiSpecFileResult` memuat `file, source_path, name, status (created|updated|unchanged|skipped|rejected), reason_code, reason, issues[], openapi_version, title, api_version, endpoint_count` (nullable sesuai `base.FileResultDTO`). Bila ada selisih dengan bagian 8 dokumen ini, perbaiki yml, `make api-bundle`, `make api-check`.

### Langkah 2 — `usecase.go` (orkestrasi, tanpa SQL, tanpa `net/http`)

```go
package uploadspecs

// Input diisi handler. Files berasal dari upload.Read (sudah terbatas ukuran/jumlah).
type Input struct {
    ProjectID string
    Files     []upload.File
    Name      *string          // field "name"; sudah dipastikan len(Files)==1 oleh upload.Read
    Meta      base.RequestMeta
}
type Output struct {
    Results []base.FileResult
    Summary base.UploadSummary
}

// Tx = operasi DB di dalam satu transaksi; diimplementasikan repository.
type Tx interface {
    LockProjectSpecs(ctx context.Context, projectID string) error
    LockSpec(ctx context.Context, projectID, name string) (base.Spec, bool, error)
    CountSpecs(ctx context.Context, projectID string) (int, error)
    InsertSpec(ctx context.Context, s *base.Spec) error        // mengisi s.ID; 23505 -> store.ErrNameTaken
    UpdateSpecContent(ctx context.Context, s base.Spec) error
    ReplaceEndpoints(ctx context.Context, specID string, es []base.Endpoint) error
}
type Repository interface {
    InTx(ctx context.Context, fn func(Tx) error) error
}

type Usecase struct {
    repo         Repository
    guard        access.Checker
    audit        audit.Recorder
    clock        base.Clock
    maxSpecs     int   // Config.APISpecMaxSpecs
    maxFileBytes int64 // Config.APISpecMaxFileBytes (untuk pesan FILE_TOO_LARGE)
}
func NewUsecase(repo Repository, guard access.Checker, audit audit.Recorder, clock base.Clock, maxSpecs int, maxFileBytes int64) *Usecase

func (u *Usecase) Execute(ctx context.Context, in Input) (Output, error)
```

Alur `Execute` (urutan wajib):

1. `actor, ok := base.ActorFrom(reqctx.IdentityFrom(ctx))`; `!ok` -> `errs.New(errs.CodeUnauthorized, "perlu login")`.
2. `u.guard.RequireMember(ctx, in.ProjectID)`; error -> kembalikan apa adanya (404/401/500). Tidak ada akses DB spec sebelum ini.
3. Nama eksplisit: bila `in.Name != nil`, `n := strings.TrimSpace(*in.Name)`; `n == ""` -> anggap nil; selain itu `base.ValidateName(n)`, gagal -> `base.ErrInvalidName` (400, U9).
4. `results := make([]base.FileResult, 0, len(in.Files))`; `reserved := map[string]struct{}{}`.
5. Untuk tiap `f` dalam `in.Files` (berurutan; di awal tiap iterasi `if err := ctx.Err(); err != nil { return Output{}, fmt.Errorf("uploadspecs: dibatalkan: %w", err) }`), panggil `u.processFile(ctx, in, actor, f, explicitName, reserved)` yang mengembalikan `(base.FileResult, error)`. Error -> kembalikan (U5). Tambahkan hasil ke `results`.
6. `return Output{Results: results, Summary: base.Summarize(results)}, nil`.

`processFile` (metode privat; bukan usecase terpisah, rule "jangan memecah artifisial"):

| Langkah | Kondisi | Hasil |
|---|---|---|
| a | `f.Rejected == base.ReasonTooManyFiles` | `rejected`, `TOO_MANY_FILES`. `SourcePath` nil, `Name` kosong |
| b | `base.NormalizeSourcePath(f.RawPath)` error | `rejected`, `INVALID_PATH`. `File = f.FileName` |
| c | `base.FormatFromExt(path.Base(sourcePath))` `ok=false` | `skipped`, `NOT_SPEC_FILE`. `SourcePath` terisi, `Name` kosong |
| d | `f.TooLarge` | `rejected`, `FILE_TOO_LARGE`, `ReasonMessage(code, u.maxFileBytes)` |
| e | nama: `explicitName` bila ada, kalau tidak `base.DeriveName(path.Base(sourcePath))`; lalu `base.ValidateName` | gagal -> `rejected`, `INVALID_NAME` (nama eksplisit sudah divalidasi di langkah 3, jadi cabang ini praktis untuk `DeriveName`) |
| f | `_, dup := reserved[name]` | `rejected`, `DUPLICATE_NAME_IN_UPLOAD` |
| g | `content := openapi.StripBOM(f.Content)`; `doc, issues := openapi.Parse(content, openapi.Format(format), openapi.DefaultLimits)` | `len(issues) > 0` -> `rejected`, `INVALID_SPEC`, `Reason = issues[0].Message`, `Issues` = salinan ke `base.Issue` (maks 20, urut baris). `Name` terisi |
| h | `eps := openapi.ExtractEndpoints(doc)`; `hash := base.HashContent(string(content))`; `meta := base.DocMeta{Title: doc.Title, APIVersion: doc.APIVersion, OpenAPIVersion: doc.OpenAPIVersion}` | - |
| i | `u.persist(ctx, ...)` (tx, di bawah) | `created`/`updated`/`unchanged`/`rejected SPEC_LIMIT_REACHED` |
| j | status `created/updated/unchanged` -> `reserved[name] = struct{}{}` | U3 |
| k | status `created/updated` -> audit | di bawah |

Field hasil untuk `created|updated|unchanged`: `Name`, `SourcePath`, `Status`, `Issues = []base.Issue{}`, `OpenAPIVersion`, `Title`, `APIVersion`, `EndpointCount = len(eps)`, `ReasonCode/Reason` kosong. Untuk `rejected INVALID_SPEC`: `EndpointCount` nol dan DTO menjadikannya `null` (lihat catatan DTO pada Langkah 5). `Issues` tidak pernah nil.

`persist` (dalam `repo.InTx`; maksimal dua percobaan hanya untuk `store.ErrNameTaken`, U6):

```go
var decision base.UploadDecision
var saved base.Spec
err := u.repo.InTx(ctx, func(tx Tx) error {
    if err := tx.LockProjectSpecs(ctx, projectID); err != nil { return err }
    existing, found, err := tx.LockSpec(ctx, projectID, name)
    if err != nil { return err }
    count, err := tx.CountSpecs(ctx, projectID)
    if err != nil { return err }
    var ex *base.Spec
    if found { ex = &existing }
    decision = base.DecideUpload(ex, hash, count, u.maxSpecs)
    switch decision {
    case base.DecisionUnchanged, base.DecisionRejectLimit:
        return nil // tanpa tulis
    case base.DecisionCreate:
        saved = base.NewSpec(projectID, name, &sourcePath, meta, string(content), format, actor, u.clock())
        if err := tx.InsertSpec(ctx, &saved); err != nil { return err }
    case base.DecisionUpdate:
        saved = existing
        saved.ApplyContent(&sourcePath, meta, string(content), format, actor, u.clock())
        if err := tx.UpdateSpecContent(ctx, saved); err != nil { return err }
    }
    return tx.ReplaceEndpoints(ctx, saved.ID, eps) // endpoint lama hilang seluruhnya
})
```

Pemetaan decision -> status: Create -> `created`; Update -> `updated`; Unchanged -> `unchanged`; RejectLimit -> `rejected`, `SPEC_LIMIT_REACHED`, `ReasonMessage(code, u.maxSpecs)`. `RejectLimit` tidak memesan nama.

Audit (setelah `InTx` kembali nil, hanya `created`/`updated`):

```go
u.audit.Record(context.WithoutCancel(ctx), audit.Event{
    Action:    record.ActionAPISpecUploaded,
    ProjectID: in.ProjectID,
    Target:    name,
    Args: map[string]any{
        "name": name, "source_path": sourcePath, "openapi_version": doc.OpenAPIVersion,
        "endpoint_count": len(eps), "size_bytes": len(content), "overwritten": decision == base.DecisionUpdate,
    },
    Meta: in.Meta,
})
```

`Args` TANPA isi spec. `Record` fail-open (tidak mengembalikan error). Tidak ada panggilan bus di dalam `InTx`.

### Langkah 3 — `repository.go` (satu-satunya yang menyentuh SQL feature ini)

```go
type Repo struct{ db *db.DB; s *store.Store }
func newRepo(d *db.DB, s *store.Store) *Repo

func (r *Repo) InTx(ctx context.Context, fn func(Tx) error) error {
    return r.db.BeginTx(ctx, func(tx *sqlx.Tx) error {
        return fn(&txRepo{s: r.s.WithTx(tx), q: tx})
    })
}
type txRepo struct{ s *store.Store; q db.Querier }
```

`txRepo` meneruskan `LockProjectSpecs`, `LockSpec`, `CountSpecs`, `ReplaceEndpoints` ke `store`, dan menulis dua fungsi milik feature ini:

```sql
-- InsertSpec
INSERT INTO api_specs (project_id, name, source_path, title, api_version, openapi_version,
    content, content_format, content_hash, size_bytes,
    created_at, created_by, created_by_label, updated_at, updated_by, updated_by_label)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$11,$12,$13)
RETURNING id;

-- UpdateSpecContent (created_* TIDAK diubah)
UPDATE api_specs SET source_path=$3, title=$4, api_version=$5, openapi_version=$6,
    content=$7, content_format=$8, content_hash=$9, size_bytes=$10,
    updated_at=$11, updated_by=$12, updated_by_label=$13
WHERE project_id=$1 AND id=$2;
```

Catatan: parameter `UpdateSpecContent` memakai `$1=project_id`, `$2=id`. `content_hash` dikirim sebagai `[]byte` dari `[32]byte`; waktu `.UTC()`. Row struct privat bertag `db`. `23505` pada `api_specs_project_name_key` -> `store.ErrNameTaken` (via helper di `store`; bila belum diekspor sebagai fungsi, `errors.As(err, &pgconn.PgError)` di sini). Error lain dibungkus `fmt.Errorf("uploadspecs: insert spec: %w", err)`. Urutan kunci: project advisory lock lebih dulu, baru kunci baris spec; `renamespec`/`deletespec` hanya mengunci baris, sehingga tidak ada urutan terbalik (tidak ada deadlock).

### Langkah 4 — `handler.go` dan `init.go`

`handler.go` (adaptor HTTP saja):

```go
type Handler struct{ uc Executor; maxFileBytes int64; maxFiles int }
type Executor interface{ Execute(ctx context.Context, in Input) (Output, error) }

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    form, err := upload.Read(r, h.maxFileBytes, h.maxFiles) // path asli dari Content-Disposition mentah
    if err != nil { respond.WriteError(w, r, err); return }
    out, err := h.uc.Execute(r.Context(), Input{
        ProjectID: chi.URLParam(r, "projectID"),
        Files:     form.Files,
        Name:      form.Name,
        Meta:      base.RequestMeta{IP: middleware.KeyByIP(r), UserAgent: truncate(r.UserAgent(), 512)},
    })
    if err != nil { respond.WriteError(w, r, err); return }
    results := make([]base.FileResultDTO, len(out.Results))
    for i, fr := range out.Results { results[i] = base.ToFileResultDTO(fr) }
    respond.WriteJSON(w, r, http.StatusOK, uploadResponse{Results: results, Summary: base.UploadSummaryDTO(out.Summary)})
}
```

Ikuti signature `respond.WriteJSON/WriteError` persis seperti handler T01/T02 yang sudah ada di `main` (periksa sebelum menulis). `uploadResponse{Results []base.FileResultDTO "json:results"; Summary base.UploadSummaryDTO "json:summary"}` didefinisikan di `handler.go`. Handler tidak memvalidasi uuid dan tidak memanggil check-access. Tidak ada `params.go` (tidak ada JSON body; form dibaca `base/upload`).

`init.go`:

```go
func Init(d app.Deps) {
    s := store.New(d.DB)
    clock := base.Clock(func() time.Time { return time.Now().UTC() })
    uc := NewUsecase(newRepo(d.DB, s), access.New(d.Bus), audit.New(d.Bus, d.Log, clock), clock,
        d.Config.APISpecMaxSpecs, d.Config.APISpecMaxFileBytes)
    h := &Handler{uc: uc, maxFileBytes: d.Config.APISpecMaxFileBytes, maxFiles: d.Config.MaxUploadFiles}
    d.API.With(middleware.RequireAuth(), middleware.BodyLimit(d.Config.MaxUploadBytes)).
        Post("/projects/{projectID}/api-specs/upload", h.ServeHTTP)
}
```

### Langkah 5 — Wiring dan DTO

- Tambah satu baris `uploadspecs.Init(d)` di `apispec/base.go` (urutan bebas; jangan menyentuh baris feature lain).
- `base.ToFileResultDTO` sudah ada dari foundation. Verifikasi bahwa untuk `rejected` sebelum parse/`skipped`, `endpoint_count`, `openapi_version`, `title`, `api_version` menjadi `null` (nilai kosong/`EndpointCount==0` pada status `rejected|skipped` -> `null`; pada `created|updated|unchanged` `endpoint_count` tetap angka walau 0). Bila fungsi DTO foundation belum membedakan ini, perbaiki di `base/dto.go` beserta `dto_test.go` (bukan di feature).
- Feature ini tidak boleh meng-import feature lain (rule 4.3); hanya `base`, `base/access`, `base/audit`, `base/openapi`, `base/store`, `base/upload`.

### Langkah 6 — Tes dan verifikasi

Tulis tes (bagian 6), jalankan `go vet ./...`, `go test ./internal/domain/project/internal/apispec/...`, `go test -tags integration ...`, `golangci-lint run`, archtest `TestT00_AC4_RepoIsClean`, `make api-check`.

---

## 4. Migration

Tidak ada. Tabel `api_specs` dan `api_spec_endpoints` dibuat oleh foundation: `backend/migrations/project/20261023090000600_create_api_specs.sql` (OV 9). Feature ini bergantung pada constraint `api_specs_project_name_key` (UNIQUE `project_id, name`) dan FK `ON DELETE CASCADE` dari endpoint ke spec.

---

## 5. Changes (per file/folder)

Baru (semua di `backend/internal/domain/project/internal/apispec/uploadspecs/`):

| File | Isi |
|---|---|
| `init.go` | `Init(d app.Deps)`: rakit store -> repo -> usecase -> handler, daftarkan route POST |
| `handler.go` | `Handler.ServeHTTP`, `Executor`, `uploadResponse`, `truncate` |
| `usecase.go` | `Input`, `Output`, `Tx`, `Repository`, `Usecase.Execute`, `processFile`, `persist`, helper pemetaan issue |
| `repository.go` | `Repo.InTx`, `txRepo` (`InsertSpec`, `UpdateSpecContent`, delegasi ke `store`) |
| `usecase_test.go` | tes unit dengan fake (bagian 6.1) |
| `handler_test.go` | tes `httptest` dengan `Executor` palsu (6.2) |
| `repository_integration_test.go` | tes DB nyata, tag `integration` (6.3) |
| `uploadspecs_integration_test.go` | tes end-to-end backend untuk AC (6.4), tag `integration` |
| `testdata/` | `billion-laughs.yaml`, spec kecil valid 3.0 dan 3.1, `package.json`, `broken.yaml`, `swagger-2.0.yaml`, `ref-http.yaml` (template dengan placeholder `__PORT__`), salinan dari `base/openapi/testdata` bila lebih mudah (atau helper memuat lewat path relatif `../base/openapi/testdata`) |

Diubah:

| File | Perubahan |
|---|---|
| `backend/internal/domain/project/internal/apispec/base.go` | tambah `uploadspecs.Init(d)` |
| `backend/internal/domain/project/internal/apispec/base/dto.go` (+ `dto_test.go`) | hanya bila perlu: aturan `null` untuk status `rejected`/`skipped` (Langkah 5) |
| `api/modules/project/apispec.yml`, `api/root.yml`, `api/openapi.yml` | hanya bila ada selisih dari Langkah 1 |

Tidak diubah: `infra/**`, module lain, kontrak bus (feature ini tidak menyediakan endpoint bus), migrasi.

Kontrak bus yang dipakai (tanpa perubahan):

| Endpoint | Request | Response | Perilaku |
|---|---|---|---|
| `project.management.check-access` | `checkaccess.Request{ProjectID, UserID, UserRole}` | `checkaccess.Response{ProjectExists, Level}` | error/unregistered -> 500; `!ProjectExists` atau `LevelNone` -> `base.ErrProjectNotFound` (404) |
| `core.audit.record` | `record.Request{ActorType: ActorUser, ActorID, ActorLabel, ProjectID, Action: ActionAPISpecUploaded, TargetType: "api_spec", Target: name, Args, Status: StatusOK, IP, UserAgent, TraceID, OccurredAt}` | `record.Response` | fail-open; dipanggil di luar tx |

---

## 6. Test Scenario

Nama: `TestT06_ACn_<Deskripsi>` untuk AC; lainnya `TestUploadSpecs_<Kasus>`.

### 6.1 Unit usecase (`usecase_test.go`; fake `Repository`/`Tx` in-memory, fake `access.Checker`, `audit.Recorder` perekam, clock tetap; tanpa mock framework)

Akses dan input:
1. Tanpa identitas di ctx -> 401, guard tidak dipanggil.
2. Guard mengembalikan `ErrProjectNotFound` -> error yang sama; fake repo tidak menerima satu pun panggilan (membuktikan tidak ada akses DB sebelum akses lolos); audit tidak dipanggil.
3. Guard error infra -> error (fail-closed), tidak `nil`.
4. `Name` = `"Bad Name"` -> `ErrInvalidName` (400), tidak ada tx. `Name` = `"  "` -> dianggap tidak diisi, nama dari file. `Name` = `"payment-v2"` dengan satu file `payment.yaml` -> nama `payment-v2`.

Per file (tiap kasus memeriksa `Status`, `ReasonCode`, `Issues`, dan bahwa repo tidak dipanggil untuk non-persist):
5. `.txt` -> `skipped NOT_SPEC_FILE`; `README` tanpa ekstensi -> `skipped`.
6. `Rejected: TOO_MANY_FILES` -> `rejected TOO_MANY_FILES`.
7. `RawPath` `../x.yaml`, `/abs.yaml`, `C:\x.yaml` -> `rejected INVALID_PATH`.
8. `TooLarge` -> `rejected FILE_TOO_LARGE`, pesan memuat angka batas.
9. `.yaml` -> nama `.yaml` (hasil `DeriveName` kosong) -> `rejected INVALID_NAME`.
10. Isi `package.json` -> `rejected INVALID_SPEC`, issue `NOT_OPENAPI`; `Reason == Issues[0].Message`.
11. Swagger 2.0 -> `NOT_OPENAPI_3`; YAML rusak -> `SYNTAX` dengan `Line > 0`; `$ref: http://...` -> `EXTERNAL_REF`; `$ref` internal hilang -> `UNRESOLVED_REF`; `openapi: 4.0.0` -> `UNSUPPORTED_VERSION`; billion laughs -> `YAML_LIMIT`.
12. BOM di awal: hash dan `Content` tersimpan tanpa BOM; hasil `created`.
13. Dua file `a/payment.yaml` dan `b/payment.yaml` valid -> pertama `created`, kedua `rejected DUPLICATE_NAME_IN_UPLOAD`. Variasi U3: file pertama ber-nama sama tetapi rusak -> `rejected INVALID_SPEC`, file kedua valid -> `created` (nama tidak dipesan oleh yang gagal).
14. Urutan hasil sama dengan urutan input; `Summary` cocok (`Summarize`).

Keputusan upload (fake `Tx` mengembalikan `existing`/`count`):
15. Tidak ada spec -> `created`, `InsertSpec` + `ReplaceEndpoints(len(eps))` dipanggil sekali, `CreatedBy/UpdatedBy` = actor, label = `Name` (fallback `Email`).
16. Ada, hash beda -> `updated`, `UpdateSpecContent` (bukan Insert), `CreatedAt/By` tidak berubah, `Updated*` = actor/clock, `ReplaceEndpoints` dipanggil dengan endpoint baru saja.
17. Ada, hash sama -> `unchanged`, tidak ada tulis, tidak ada audit.
18. `count == maxSpecs` dan nama baru -> `rejected SPEC_LIMIT_REACHED`, tidak ada tulis, tidak ada audit, nama tidak dipesan.
19. `count == maxSpecs` dan nama sudah ada dengan konten beda -> `updated` (menimpa tetap boleh). `count == maxSpecs` dan konten sama -> `unchanged`.
20. Urutan panggilan tx tepat: `LockProjectSpecs` -> `LockSpec` -> `CountSpecs` -> tulis (verifikasi dengan fake perekam urutan).
21. `InsertSpec` mengembalikan `store.ErrNameTaken` sekali lalu sukses pada percobaan kedua (kini `LockSpec` menemukan baris) -> `updated`; dua kali gagal -> error 500, bukan `rejected`.
22. Error infra pada file ke-2 dari 3 -> `Execute` mengembalikan error; file ke-1 sudah diaudit; file ke-3 tidak diproses.
23. ctx dibatalkan di antara file -> error membungkus `context.Canceled`.

Audit:
24. `created` -> satu `Record` dengan `Action == ActionAPISpecUploaded`, `Target == name`, `Args.overwritten == false`; `updated` -> `overwritten == true`; `Args` tidak memuat kunci `content`.
25. Audit dipanggil SETELAH `InTx` kembali (fake merekam bahwa `InTx` sudah selesai saat `Record` dipanggil) dan dengan ctx yang tidak ikut batal (`context.WithoutCancel`).
26. `rejected`, `skipped`, `unchanged` -> tidak ada audit. Tiga file `created` -> tepat 3 entri.
27. Recorder fail-open: recorder fake yang mencatat error internal tidak mengubah hasil; respons tetap sukses.

### 6.2 Handler (`handler_test.go`, `httptest`, `Executor` palsu)

1. Multipart valid dua file -> `Input.Files` berisi `RawPath` utuh (`specs/payment.yaml`); 200 dengan JSON `{results, summary}` persis bentuk bagian 8; `Content-Type: application/json`.
2. Non-multipart (`application/json`) -> 400 `details.reason == NOT_MULTIPART`; `Executor` tidak dipanggil.
3. Nol file -> 400 `NO_FILES`. `name` + dua file -> 400 `NAME_WITH_MULTIPLE_FILES`.
4. Body melebihi `BodyLimit` (router uji memasang `middleware.BodyLimit`) -> 413 `PAYLOAD_TOO_LARGE`; `Executor` tidak dipanggil.
5. File melebihi `maxFileBytes` -> diteruskan dengan `TooLarge=true`, request tetap 200; part berikutnya tetap terbaca.
6. Part ke-(`maxFiles`+1) -> `Rejected: TOO_MANY_FILES`.
7. `Executor` mengembalikan `errs` 404 -> 404 dengan format error seragam; error biasa -> 500 generik tanpa detail internal.
8. Field tak dikenal diabaikan. `Meta.UserAgent` dipotong 512.

### 6.3 Repository integration (`repository_integration_test.go`, `db.TestDB(t)`, migrasi T03 + T06)

1. `InsertSpec` + `UpdateSpecContent`: `created_*` tetap, `updated_*` berubah, `content_hash` 32 byte round-trip, waktu UTC.
2. Insert nama ganda -> `store.ErrNameTaken`.
3. `ReplaceEndpoints` pada update: endpoint lama (mis. `GET /old`) hilang, endpoint baru ada; spec 0 endpoint menghapus semua baris.
4. `InTx` rollback: fungsi mengembalikan error setelah Insert -> tidak ada baris tersimpan.
5. `LockProjectSpecs` men-serialisasi: tx A menahan lock; tx B (goroutine) baru lanjut setelah A commit (ukur dengan channel, bukan sleep panjang).
6. Konkurensi batas: `maxSpecs=2`, 1 spec ada, dua `Execute` paralel dengan nama berbeda (usecase nyata + repo nyata) -> tepat satu `created` dan satu `rejected SPEC_LIMIT_REACHED`; `count == 2` di DB.
7. Rename bersaing (U6): goroutine memindahkan spec X ke nama `Y` sementara `Execute` mengunggah `Y` -> hasil `created` (rename menang waktu) atau `updated` (upload menang lalu rename ditolak 409); tidak pernah 500 dan tidak ada baris ganda.

### 6.4 Backend end-to-end (`uploadspecs_integration_test.go`, tag `integration`)

Bus nyata (`bus.NewRegistry`), `db.TestDB(t)`, router chi dengan `RequireAuth` uji, handler palsu `check-access` (peta project -> level) dan `core.audit.record` (perekam), dua project A dan B, user anggota A, user bukan anggota, super_admin.

- `TestT06_AC1_UploadThreeThenOverwrite`: satu request dengan `auth.yaml`, `payment.yaml` (endpoint `GET /payments`, `POST /payments`, `GET /payments/{id}`), `notification.yaml` -> 3 `created`, `summary.created == 3`. Unggah ulang `payment.yaml` hanya dengan `GET /payments` dan `POST /payments/refund` -> `updated`; tabel `api_spec_endpoints` untuk spec itu berisi tepat dua baris itu (`GET /payments/{id}` hilang); `created_at` tidak berubah, `updated_at` maju. Unggah ulang identik -> `unchanged` dan jumlah entri audit tidak bertambah. Audit: 4 entri `apispec.uploaded` (3 `overwritten=false`, 1 `true`).
- `TestT06_AC2_RejectedWithReasons`: satu request dengan `package.json`, `broken.yaml`, `swagger-2.0.yaml`, dan `ref-http.yaml` yang `$ref`-nya menunjuk `http://127.0.0.1:<port>/x.yaml` pada `httptest.NewServer` lokal (handler menghitung request). Hasil: empat `rejected` dengan issue `NOT_OPENAPI`, `SYNTAX` (ber-`line`), `NOT_OPENAPI_3`, `EXTERNAL_REF`; hitungan request listener == 0; `SELECT count(*) FROM api_specs` == 0; tidak ada audit. Variasi `$ref` `file:///etc/passwd` dan `./common.yaml` -> `EXTERNAL_REF`.
- `TestT06_AC3_BillionLaughs`: upload `billion-laughs.yaml` lewat HTTP -> `rejected` `YAML_LIMIT`; seluruh `ServeHTTP` < 1 detik dan `TotalAlloc` selisih < 50 MB (`runtime.ReadMemStats`); server tetap melayani request berikutnya. Varian: kedalaman 200 dan siklus alias.
- `TestT06_AC6_OpenAPI30And31`: upload `petstore-3.0.yaml` (`nullable: true`) dan `petstore-3.1.json` (`type: ["string","null"]`) dalam satu request -> dua `created`, `openapi_version` `3.0.3` dan `3.1.0`; isi tersimpan sama persis dengan input.
- `TestT06_AC7_NonMemberNotFound`: non-anggota -> 404 `project tidak ditemukan`, tidak ada baris di DB, tidak ada audit; `projectID` bukan uuid -> 404 yang sama; project tidak ada -> 404 yang sama (respons identik byte-per-byte kecuali `trace_id`); super_admin non-anggota -> 200.
- Batas: `APISPEC_MAX_SPECS=2` dan tiga file valid -> `created, created, rejected SPEC_LIMIT_REACHED`; unggah ulang salah satunya (konten beda) -> `updated`.
- Isolasi: spec `payment` di project A dan B bernama sama -> keduanya `created`, tidak saling menimpa.
- Per-file independen: satu file rusak di tengah dua file valid -> dua `created`, satu `rejected`; yang valid tersimpan.
- Fail-open audit: handler `core.audit.record` mengembalikan error -> respons tetap 200, spec tersimpan.
- CSRF: tanpa header `X-Requested-With` -> 403 (diuji di lapisan T00; cukup pastikan route tidak mengecualikannya). Tanpa sesi -> 401.
- Nama: file `Specs/Payment API.v2.yaml` -> nama `payment-api.v2`, `source_path` `Specs/Payment API.v2.yaml`; file dengan `name=payment-v2` -> nama itu; `name` + 2 file -> 400.

---

## 7. E2E Scenarios (Playwright, UI dari `frontend-plan.md`; butuh T03 merge)

Positif:
1. Anggota membuka tab "API Specs", memilih folder berisi `auth.yaml`, `payment.yaml`, `notification.yaml`, dan `README.md` -> dialog menampilkan ringkasan: 3 dibuat, 1 dilewati (`README.md`, alasan "bukan file spec"); daftar spec menampilkan 3 kartu dengan judul, versi, jumlah endpoint.
2. Unggah ulang `payment.yaml` yang diubah -> baris berstatus "diperbarui"; halaman detail menampilkan endpoint baru dan endpoint lama sudah hilang.
3. Unggah ulang file identik -> "tidak berubah"; tidak ada perubahan pada waktu "diperbarui".
4. Unggah satu file dengan kolom nama diisi `payment-v2` -> spec bernama `payment-v2`.
5. Unggah spec OpenAPI 3.0 dan 3.1 sekaligus -> keduanya dibuat dan dapat dibuka.
6. Super Admin bukan anggota mengunggah -> berhasil.

Negatif:
1. Unggah `package.json`, YAML rusak, Swagger 2.0, dan spec ber-`$ref: http://...` -> empat baris "ditolak", tiap baris menampilkan alasan dengan baris/pointer, tombol salin menyalin teks alasan; daftar spec tidak berubah.
2. Unggah file 6 MB -> "ditolak: ukuran file melebihi batas"; file lain pada request yang sama tetap diproses.
3. Unggah file `.txt` saja -> "dilewati".
4. Dua file ber-nama dasar sama dari folder berbeda -> yang kedua "ditolak: nama duplikat dalam upload".
5. Project sudah berisi batas maksimum spec -> spec baru "ditolak: batas spec per project tercapai"; menimpa spec yang ada tetap berhasil.
6. Isi kolom nama `Bad Name` -> pesan validasi nama tidak valid, tidak ada yang tersimpan.
7. Isi kolom nama saat memilih dua file -> UI mencegah/menampilkan pesan "nama hanya untuk satu file".
8. Developer bukan anggota membuka URL upload project lain -> halaman 404.
9. Request mutasi tanpa header CSRF (via `request` Playwright) -> 403.
10. Body total melebihi `MAX_UPLOAD_BYTES` -> pesan "ukuran upload melebihi batas"; tidak ada spec yang tersimpan.

---

## 8. Request/Response body

Format error seragam (rule 11.1): `{"error":{"code","message","details?"},"trace_id"}`.

### Request

`POST /api/projects/0b6c3e8a-6f0d-4a3b-9c1e-2d7f5a1b8c90/api-specs/upload`
Header: `Content-Type: multipart/form-data; boundary=b`, `X-Requested-With: grimoire`, cookie sesi.

```
--b
Content-Disposition: form-data; name="files"; filename="specs/payment.yaml"
Content-Type: application/yaml

openapi: 3.0.3
info: { title: Payment API, version: 1.4.0 }
paths:
  /payments:
    get:
      operationId: listPayments
      responses: { "200": { description: OK } }
--b
Content-Disposition: form-data; name="files"; filename="specs/package.json"
Content-Type: application/json

{"name":"x","version":"1.0.0"}
--b
Content-Disposition: form-data; name="files"; filename="specs/notes.txt"
Content-Type: text/plain

catatan
--b--
```

Satu file dengan nama eksplisit: tambahkan bagian `Content-Disposition: form-data; name="name"` dengan isi `payment-v2` (hanya sah bila tepat satu file).

### Response 200 (campuran)

```json
{
  "results": [
    {
      "file": "payment.yaml",
      "source_path": "specs/payment.yaml",
      "name": "payment",
      "status": "updated",
      "reason_code": null,
      "reason": null,
      "issues": [],
      "openapi_version": "3.0.3",
      "title": "Payment API",
      "api_version": "1.4.0",
      "endpoint_count": 1
    },
    {
      "file": "package.json",
      "source_path": "specs/package.json",
      "name": "package",
      "status": "rejected",
      "reason_code": "INVALID_SPEC",
      "reason": "bukan dokumen OpenAPI",
      "issues": [
        { "code": "NOT_OPENAPI", "message": "bukan dokumen OpenAPI", "pointer": "#", "line": 1 }
      ],
      "openapi_version": null,
      "title": null,
      "api_version": null,
      "endpoint_count": null
    },
    {
      "file": "notes.txt",
      "source_path": "specs/notes.txt",
      "name": null,
      "status": "skipped",
      "reason_code": "NOT_SPEC_FILE",
      "reason": "bukan file spec (.yaml/.yml/.json)",
      "issues": [],
      "openapi_version": null,
      "title": null,
      "api_version": null,
      "endpoint_count": null
    }
  ],
  "summary": { "created": 0, "updated": 1, "unchanged": 0, "skipped": 1, "rejected": 1 }
}
```

### Contoh hasil per status lain

`created` (sama dengan `updated`, `status: "created"`).

`unchanged`:

```json
{ "file": "auth.yaml", "source_path": "auth.yaml", "name": "auth", "status": "unchanged",
  "reason_code": null, "reason": null, "issues": [],
  "openapi_version": "3.1.0", "title": "Auth API", "api_version": "2.0.0", "endpoint_count": 5 }
```

`rejected` karena `$ref` eksternal:

```json
{ "file": "ext.yaml", "source_path": "ext.yaml", "name": "ext", "status": "rejected",
  "reason_code": "INVALID_SPEC",
  "reason": "referensi eksternal tidak didukung dan tidak di-fetch: http://127.0.0.1:41233/x.yaml (baris 12, #/paths/~1pets/get/responses/200/content/application~1json/schema)",
  "issues": [
    { "code": "EXTERNAL_REF",
      "message": "referensi eksternal tidak didukung dan tidak di-fetch: http://127.0.0.1:41233/x.yaml (baris 12, #/paths/~1pets/get/responses/200/content/application~1json/schema)",
      "pointer": "#/paths/~1pets/get/responses/200/content/application~1json/schema", "line": 12 }
  ],
  "openapi_version": null, "title": null, "api_version": null, "endpoint_count": null }
```

`rejected` sintaks YAML: `{"code":"SYNTAX","message":"YAML tidak valid: ... (baris 7)","pointer":null,"line":7}`. `rejected` billion laughs: `{"code":"YAML_LIMIT","message":"dokumen YAML terlalu kompleks (alias/kedalaman)","pointer":null,"line":null}`.

Alasan non-parse:

```json
{ "file": "big.yaml", "source_path": "big.yaml", "name": "big", "status": "rejected",
  "reason_code": "FILE_TOO_LARGE", "reason": "ukuran file melebihi batas 5242880 byte", "issues": [],
  "openapi_version": null, "title": null, "api_version": null, "endpoint_count": null }
{ "file": "x.yaml", "source_path": "b/payment.yaml", "name": "payment", "status": "rejected",
  "reason_code": "DUPLICATE_NAME_IN_UPLOAD", "reason": "nama duplikat dalam upload", "issues": [], "...": "..." }
{ "file": "new.yaml", "source_path": "new.yaml", "name": "new", "status": "rejected",
  "reason_code": "SPEC_LIMIT_REACHED", "reason": "batas 100 spec per project tercapai", "issues": [], "...": "..." }
```

### Error request

400 tidak ada file:

```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "tidak ada file", "details": { "reason": "NO_FILES" } }, "trace_id": "6f1c2a0b9d8e4f17" }
```

400 bukan multipart:

```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "request harus multipart/form-data", "details": { "reason": "NOT_MULTIPART" } }, "trace_id": "6f1c2a0b9d8e4f17" }
```

400 `name` dengan banyak file:

```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "field name hanya boleh dipakai saat mengunggah tepat satu file", "details": { "reason": "NAME_WITH_MULTIPLE_FILES" } }, "trace_id": "6f1c2a0b9d8e4f17" }
```

400 nama tidak valid:

```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "nama spec tidak valid: huruf kecil, angka, '-', '_', '.'; 1-80 karakter", "details": { "reason": "INVALID_NAME", "fields": { "name": "nama spec tidak valid" } } }, "trace_id": "6f1c2a0b9d8e4f17" }
```

401 tanpa sesi: `{ "error": { "code": "UNAUTHORIZED", "message": "perlu login" }, "trace_id": "..." }`

403 tanpa header CSRF (dari T00): `{ "error": { "code": "FORBIDDEN", "message": "...", "details": { "reason": "CSRF_REQUIRED" } }, "trace_id": "..." }` (bentuk persis mengikuti middleware T00).

404 non-anggota / project tidak ada / `projectID` bukan uuid (respons identik):

```json
{ "error": { "code": "NOT_FOUND", "message": "project tidak ditemukan" }, "trace_id": "6f1c2a0b9d8e4f17" }
```

413 body melebihi `MAX_UPLOAD_BYTES` (seluruh request, tidak ada yang tersimpan):

```json
{ "error": { "code": "PAYLOAD_TOO_LARGE", "message": "ukuran upload melebihi batas" }, "trace_id": "6f1c2a0b9d8e4f17" }
```

500 (error infrastruktur, U5): `{ "error": { "code": "INTERNAL", "message": "terjadi kesalahan internal" }, "trace_id": "..." }`

### Audit yang dikirim (per file `created`/`updated`)

```go
record.Request{
    ActorType: record.ActorUser, ActorID: "<user uuid>", ActorLabel: "Budi",
    ProjectID: "0b6c3e8a-6f0d-4a3b-9c1e-2d7f5a1b8c90",
    Action: record.ActionAPISpecUploaded, TargetType: "api_spec", Target: "payment",
    Args: map[string]any{"name": "payment", "source_path": "specs/payment.yaml", "openapi_version": "3.0.3",
        "endpoint_count": 1, "size_bytes": 412, "overwritten": true},
    Status: record.StatusOK, IP: "10.0.0.5", UserAgent: "Mozilla/5.0 ...", TraceID: "6f1c2a0b9d8e4f17",
}
```

---

## 9. Risiko dan catatan khusus usecase

1. Memori: semua part dibaca ke memori (maks `MAX_UPLOAD_BYTES`, default 50 MB) lalu diparse satu per satu; node YAML dilepas antar file karena diproses berurutan. Jangan menjalankan parse paralel.
2. Waktu request: parse sampai `MAX_UPLOAD_FILES` file bisa memakan waktu; ctx diperiksa antar file. Tidak ada timeout tambahan di usecase (timeout server di T00).
3. Pesan `reason` untuk `INVALID_SPEC` adalah `Issues[0].Message` yang sudah memuat lokasi; FE memakai `reason_code` untuk i18n dan `issues[]` untuk detail.
4. `source_path` selalu ikut ditimpa saat `updated` (kecuali `unchanged`); `created_*` tidak pernah berubah pada timpa.
5. Plan ini tidak menyentuh `validatespec`; kedua usecase berbagi urutan validasi yang sama tetapi TIDAK saling import (rule 4.3). Bila logika per file (langkah a-h) mulai diduplikasi, angkat helper murni ke `base/` pada PR terpisah setelah kedua usecase ada.
