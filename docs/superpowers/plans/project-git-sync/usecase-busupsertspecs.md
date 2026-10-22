# T14 usecase `busupsertspecs` (project/api-spec) — Plan

Tiket: `docs/tickets/T14-project-git-sync.md`. Foundation: `docs/superpowers/plans/project-git-sync/overview.md` (bagian 4 D1/D2/D8, 6.4.3, 6.4.4, 6.5, 12.6). Bila dokumen ini bertentangan dengan overview, overview menang.

## 1. Metadata

| Atribut | Nilai |
|---|---|
| Ticket | T14 |
| Scope | backend (tanpa HTTP, tanpa frontend) |
| Domain / Module / Feature | project / apispec (T06, T10) / `busupsertspecs` |
| Folder | `backend/internal/domain/project/internal/apispec/busupsertspecs` |
| Endpoint HTTP | tidak ada (handler bus saja) |
| Bus | `project.apispec.upsert-specs` (konstanta `BUS_PROJECT_APISPEC_UPSERT_SPECS`) |
| Method | Call (sinkron, request dikirim sebagai NILAI `upsertspecs.Request`) |
| Version | v1 (kontrak baru, `lib/contracts/project/apispec/upsertspecs`) |
| Consumer | `project/gitsync` `runsync` (T14). Provider tidak memeriksa keanggotaan; consumer wajib check-access. |
| Kriteria tiket | 5 (spec dari git: valid -> dibuat/diperbarui, invalid -> rejected + issues, endpoint diganti penuh) |
| Bergantung pada | Langkah 0 overview: konstanta bus (5.7), kontrak `upsertspecs` (6.4.3), `Event.Actor` di audit apispec (6.5) |
| Bersama usecase | `uploadspecs` (refaktor ke `base/ingest`) |

Catatan bentuk API ingest: instruksi tugas menulis `ProcessFile(ctx, deps, projectID, actor, file, explicitName, reserved, meta, source)`. Overview 6.5 membungkus `projectID/actor/meta/source` ke `ingest.Request`. Plan ini mengikuti overview (`Deps` + `Request`); isinya sama.

## 2. Summary Plan

Dua langkah, berurutan, dalam satu PR.

1. **Refaktor tanpa perubahan perilaku.** `processFile`, `reject`, dan `persist` dipindah dari `uploadspecs/usecase.go` ke paket bersama `apispec/base/ingest` sebagai `ingest.ProcessFile`. Tipe `Tx` dan `Repository` juga pindah ke `ingest` (`uploadspecs.Tx`/`Repository` menjadi alias supaya fake di `usecase_test.go` tetap kompil tanpa ubah asersi). Semua tes `uploadspecs` (termasuk `TestUploadSpecs_Swagger2Created` T10) harus hijau tanpa perubahan asersi.
2. **Feature baru `busupsertspecs`.** Handler bus: `bus.Expect[upsertspecs.Request]` -> validasi `ProjectID` (uuid) dan `Actor.UserID` -> konversi tiap `Item` ke `upload.File` -> `ingest.ProcessFile` berurutan (tx per file, D2) -> petakan `base.FileResult` ke `upsertspecs.ItemResult` -> `Summary`. Aktor audit memakai `Request.Actor` (bukan ctx), Args audit ditambah `"source": "git_sync"`.

Keputusan penting:
- Pipeline per file tidak berubah: `NormalizeSourcePath` -> `FormatFromExt` (bukan spec -> `skipped NOT_SPEC_FILE`) -> `TooLarge` (`FILE_TOO_LARGE`) -> `DeriveName`/`ValidateName` -> `reserved` (`DUPLICATE_NAME_IN_UPLOAD`) -> `openapi.StripBOM`/`Parse` (invalid atau `$ref` eksternal -> `rejected INVALID_SPEC` + issues) -> `ExtractEndpoints` -> persist (`LockProjectSpecs`/`LockSpec`/`CountSpecs`/`DecideUpload`/`Insert|Update`/`ReplaceEndpoints`, retry satu kali pada `store.ErrNameTaken`) -> audit `apispec.uploaded` untuk created/updated saja.
- `explicitName` selalu `""` di bus (nama selalu `DeriveName(basename)`, D8). Nama ganda dalam satu batch ditolak lewat `reserved`.
- Penimpaan: spec dengan nama sama di project diperbarui (`DecisionUpdate`), `ReplaceEndpoints` mengganti SELURUH indeks endpoint. Spec lain yang tidak ada di batch tidak disentuh dan tidak dihapus (sync tidak pernah menghapus).
- Tanpa pemeriksaan keanggotaan. Semua query memakai `Request.ProjectID`; tidak ada jalur yang membaca project dari ctx.
- Error infrastruktur (DB) menghentikan sisa batch dan dikembalikan sebagai error Go; file yang sudah ter-commit tetap (konsisten dengan upload HTTP). Validasi request (uuid/aktor) mengembalikan `*errs.Error` `CodeInvalid` sebelum menulis apa pun.
- Konkurensi: `LockProjectSpecs` serial per project, jadi sync berbarengan dengan upload HTTP aman.
- Tidak ada migrasi, tidak ada route, tidak ada OpenAPI (bus internal).

## 3. Plan (langkah berurutan)

### Langkah A — Prasyarat dari foundation (cek dulu; kerjakan bila belum ada)

A1. `lib/constants/bus.go`: `BUS_PROJECT_APISPEC_UPSERT_SPECS = "project.apispec.upsert-specs"` (komentar Provider/Consumer/Kontrak sesuai overview 5.7).
A2. `lib/contracts/project/apispec/upsertspecs/upsertspecs.go` persis overview 6.4.3 (Actor, Meta, Item, Request, Issue, ItemResult, Summary, Response, `SourceGitSync = "git_sync"`).
A3. `apispec/base/audit/audit.go`: tambah `Actor *base.Actor` pada `Event`. Di `BusRecorder.Record`: bila `ev.Actor != nil` pakai itu; selain itu perilaku lama (`base.ActorFrom(reqctx.IdentityFrom(ctx))`). Tambah tes di `audit_test.go` (lihat bagian 5), tanpa mengubah tes lama.

### Langkah B — Refaktor `base/ingest` (perilaku tetap)

B1. Buat `apispec/base/ingest/ingest.go`:

```go
package ingest

// Tx, Repo: signature persis dari uploadspecs.Tx/Repository saat ini.
type Tx interface {
	LockProjectSpecs(ctx context.Context, projectID string) error
	LockSpec(ctx context.Context, projectID, name string) (base.Spec, bool, error)
	CountSpecs(ctx context.Context, projectID string) (int, error)
	InsertSpec(ctx context.Context, s *base.Spec) error
	UpdateSpecContent(ctx context.Context, s base.Spec) error
	ReplaceEndpoints(ctx context.Context, specID string, es []base.Endpoint) error
}
type Repo interface{ InTx(ctx context.Context, fn func(Tx) error) error }

type Deps struct {
	Repo         Repo
	Audit        audit.Recorder
	Clock        base.Clock
	MaxSpecs     int
	MaxFileBytes int64
}

// Request membawa konteks batch. Source "" = upload HTTP; "git_sync" = bus.
type Request struct {
	ProjectID string
	Actor     base.Actor
	Meta      base.RequestMeta
	Source    string
}

func ProcessFile(ctx context.Context, d Deps, req Request, f upload.File,
	explicitName string, reserved map[string]struct{}) (base.FileResult, error)
```

B2. Isi `ProcessFile` = salinan `processFile` + `reject` + `persist` saat ini, dengan pengganti `u.repo` -> `d.Repo`, `u.clock` -> `d.Clock`, `u.maxSpecs` -> `d.MaxSpecs`, `u.maxFileBytes` -> `d.MaxFileBytes`, `in.ProjectID` -> `req.ProjectID`, `actor` -> `req.Actor`, `in.Meta` -> `req.Meta`. Prefix pesan error `uploadspecs: simpan %q` diubah menjadi `ingest: simpan %q` (cek `grep -rn "uploadspecs: simpan"` di tes; bila ada tes yang membandingkan string, pertahankan prefix lama).
B3. Audit di dalam `ProcessFile`: tetap `d.Audit.Record(context.WithoutCancel(ctx), audit.Event{...})`, setelah tx commit. Tambahan:
   - `Actor: &actor` (salinan lokal dari `req.Actor`) SELALU diisi. Untuk upload HTTP nilainya sama dengan identity di ctx, jadi perilaku sama.
   - Bila `req.Source != ""`: `Args["source"] = req.Source`. Bila kosong: Args identik dengan sekarang (tes `TestUploadSpecs_Audit` tidak berubah).
B4. `uploadspecs/usecase.go`:
   - Hapus `processFile`, `reject`, `persist`, interface `Tx`/`Repository` lokal; ganti dengan `type Tx = ingest.Tx` dan `type Repository = ingest.Repo` (alias) agar `repository.go` dan fake tes tidak berubah.
   - `Usecase.Execute` memanggil `ingest.ProcessFile(ctx, ingest.Deps{Repo: u.repo, Audit: u.audit, Clock: u.clock, MaxSpecs: u.maxSpecs, MaxFileBytes: u.maxFileBytes}, ingest.Request{ProjectID: in.ProjectID, Actor: actor, Meta: in.Meta}, f, explicit, reserved)`. Urutan lain di `Execute` (identity, `RequireMember`, explicit name, cek `ctx.Err()` antar file, `Summarize`) tidak berubah.
   - `NewUsecase` signature tidak berubah. `init.go`, `handler.go`, `repository.go` tidak berubah.
B5. Tes `base/ingest/ingest_test.go`: fake Tx/Repo ringan (lihat bagian 5.1) untuk memverifikasi unit `ProcessFile` secara langsung. Tes `uploadspecs` TIDAK diubah dan menjadi regresi utama.
B6. Jalankan: `go test ./backend/internal/domain/project/internal/apispec/...` dan `go test -tags integration` untuk paket `apispec`. Wajib hijau SEBELUM lanjut ke langkah C. Commit terpisah: `refactor(apispec): pindahkan orkestrasi upload ke base/ingest`.

### Langkah C — Feature `busupsertspecs`

C1. `init.go`:

```go
package busupsertspecs

// Init mendaftarkan handler bus project.apispec.upsert-specs. Tidak ada route HTTP.
func Init(d app.Deps) {
	s := store.New(d.DB)
	clock := base.Clock(base.SystemClock)
	uc := NewUsecase(newRepo(d.DB, s), audit.New(d.Bus, d.Log, clock), clock,
		d.Config.APISpecMaxSpecs, d.Config.APISpecMaxFileBytes)
	d.Bus.Register(constants.BUS_PROJECT_APISPEC_UPSERT_SPECS, newHandler(uc).Handle)
}
```

C2. `handler.go`: `Handler{uc Executor}`; `Handle(ctx, req any) (any, error)`: `in, err := bus.Expect[upsertspecs.Request](req)`; error dikembalikan apa adanya; `res, err := h.uc.Execute(ctx, in)`; mengembalikan `res` bertipe `upsertspecs.Response` (NILAI, bukan pointer, supaya `bus.CallTyped[upsertspecs.Response]` cocok). Handler tipis, tanpa logika.

C3. `usecase.go`:

```go
type Repository = ingest.Repo

type Usecase struct {
	repo         Repository
	audit        audit.Recorder
	clock        base.Clock
	maxSpecs     int
	maxFileBytes int64
}

func NewUsecase(repo Repository, rec audit.Recorder, clock base.Clock, maxSpecs int, maxFileBytes int64) *Usecase

func (u *Usecase) Execute(ctx context.Context, in upsertspecs.Request) (upsertspecs.Response, error)
```

Urutan `Execute` (orkestrasi saja; rule bisnis tetap di `base`/`ingest`):
1. Validasi: `ProjectID` bukan uuid (pakai `store.ValidUUID` atau helper uuid yang sudah dipakai module; jika tidak ada, `uuid.Parse`) atau `strings.TrimSpace(Actor.UserID) == ""` -> `errs.New(errs.CodeInvalid, "permintaan tidak valid")` dengan `details.fields` (`project_id` / `actor`). Tanpa tulis apa pun.
2. `actor := base.Actor{ID: in.Actor.UserID, Label: in.Actor.Label}`; `meta := base.NewRequestMeta(in.Meta.IP, in.Meta.UserAgent)`; `source := in.Source` (kosong -> default `upsertspecs.SourceGitSync`; nilai lain yang tidak kosong dipakai apa adanya, dibatasi 32 rune).
3. Untuk setiap `Item` BERURUTAN: cek `ctx.Err()` (batal -> `fmt.Errorf("busupsertspecs: dibatalkan: %w", err)`), konversi `upload.File{FileName: path.Base(strings.ReplaceAll(item.SourcePath, "\\", "/")), RawPath: item.SourcePath, Content: item.Content, TooLarge: item.TooLarge}` (`Rejected` kosong), lalu `ingest.ProcessFile(ctx, deps, req, file, "", reserved)` dengan `reserved` satu map untuk seluruh batch. Error -> return `(Response{}, err)` (hentikan sisa; file sebelumnya sudah ter-commit).
4. Petakan `FileResult` -> `ItemResult` (fungsi murni `toItemResult(index int, rawPath string, fr base.FileResult)` di `usecase.go`):
   - `Index` = `item.Index`; `SourcePath` = `*fr.SourcePath` bila non-nil, selain itu `item.SourcePath` mentah;
   - `Name`, `Status` = `string(fr.Status)`, `ReasonCode`, `Reason`, `OpenAPIVersion`, `Title`, `EndpointCount`;
   - `Issues` = salinan ke `[]upsertspecs.Issue` (selalu slice non-nil, kosong bila tidak ada). `APIVersion` TIDAK ada di kontrak, jadi diabaikan.
5. `Summary` = hitung dari `base.Summarize(results)` lalu salin ke `upsertspecs.Summary`.

C4. `repository.go`: `Repo{db, s}` dengan `InTx` -> `db.BeginTx` -> `txRepo{s: r.s.WithTx(tx), q: tx}` yang memenuhi `ingest.Tx`. Lima method (`LockProjectSpecs`, `LockSpec`, `CountSpecs`, `ReplaceEndpoints` delegasi ke `store`; `InsertSpec`, `UpdateSpecContent` SQL yang sama persis dengan `uploadspecs/repository.go`, dengan `store.MapNameTaken` untuk `ErrNameTaken`). Prefix error: `busupsertspecs: insert spec` / `busupsertspecs: update spec`. `var _ ingest.Tx = (*txRepo)(nil)`. SQL disalin apa adanya karena repository per-feature adalah satu-satunya penyentuh SQL (konvensi README); deduplikasi ke `base/store` bukan bagian tiket ini.

C5. `apispec/base.go`: di blok "Handler bus" tambah satu baris `busupsertspecs.Init(d)` dan import-nya (tepat satu baris tambahan agar tidak konflik dengan usecase paralel lain).

C6. `apispec/routes_test.go`: tambah asersi `d.Bus` memiliki handler `BUS_PROJECT_APISPEC_UPSERT_SPECS` (mengikuti pola asersi `listspecsbus` yang sudah ada di file itu); satu baris/satu blok kecil.

C7. `docs/tickets/README.md` katalog bus: tambah baris `| \`project.apispec.upsert-specs\` | Call | T14 | T14 (gitsync) |` setelah baris `project.apispec.get-endpoint`, dan perluas paragraf lokasi kontrak: "Kontrak `project.apispec.upsert-specs` ada di `lib/contracts/project/apispec/upsertspecs` (konstanta `BUS_PROJECT_APISPEC_UPSERT_SPECS`); handler tidak memeriksa keanggotaan, selalu membatasi query pada `ProjectID` request, tidak menghapus spec, dan mencatat audit `apispec.uploaded` dengan Args `source: git_sync` memakai aktor dari request." Bila foundation sudah menambah baris yang sama, jangan duplikasi.

C8. Tes (bagian 5), lalu `go vet`, `golangci-lint`, `go test ./backend/internal/archtest/...`.

## 4. Migration

Tidak ada. Memakai tabel `api_specs` dan `api_endpoints` yang sudah ada (T06). Tidak ada kolom baru: `source_path` menyimpan path repo, sumber sync hanya muncul di audit (`source: git_sync`).

## 5. Changes (per file/folder)

Semua di `backend/internal/domain/project/internal/apispec/` kecuali disebut lain.

| File | Jenis | Isi |
|---|---|---|
| `base/ingest/ingest.go` | baru | `Tx`, `Repo`, `Deps`, `Request`, `ProcessFile` (+ `reject`, `persist` privat) |
| `base/ingest/ingest_test.go` | baru | tes unit `ProcessFile` dengan fake Tx |
| `base/audit/audit.go` | ubah (aditif) | `Event.Actor *base.Actor`; `Record` memakainya bila non-nil |
| `base/audit/audit_test.go` | ubah (tambah tes) | tes `Event.Actor`; tes lama tidak diubah (file baru bila belum ada) |
| `uploadspecs/usecase.go` | ubah | hapus `processFile/reject/persist`; `Tx`/`Repository` jadi alias `ingest`; `Execute` memanggil `ingest.ProcessFile` |
| `uploadspecs/{handler,init,repository}.go` | tetap | tidak berubah |
| `uploadspecs/{usecase,handler}_test.go` | tetap | tidak boleh berubah asersinya |
| `busupsertspecs/init.go` | baru | registrasi bus |
| `busupsertspecs/handler.go` | baru | `bus.Expect` + delegasi |
| `busupsertspecs/usecase.go` | baru | validasi, konversi Item -> `upload.File`, loop `ProcessFile`, mapping hasil |
| `busupsertspecs/repository.go` | baru | `InTx` + `txRepo` (memenuhi `ingest.Tx`) |
| `busupsertspecs/handler_test.go` | baru | tipe request salah, delegasi, error diteruskan |
| `busupsertspecs/usecase_test.go` | baru | skenario 6.2 |
| `busupsertspecs/repository_integration_test.go` | baru | tag `integration`, Postgres nyata |
| `base.go` | ubah (1 baris + import) | `busupsertspecs.Init(d)` |
| `routes_test.go` | ubah (kecil) | handler bus `upsert-specs` terdaftar |
| `backend/lib/constants/bus.go` | foundation | konstanta (cek sudah ada) |
| `backend/lib/contracts/project/apispec/upsertspecs/upsertspecs.go` | foundation | kontrak (cek sudah ada) |
| `docs/tickets/README.md` | ubah | baris katalog bus + paragraf kontrak |

Aturan arsitektur yang dijaga: `busupsertspecs` hanya meng-import `base/**`, `lib/contracts/**`, `lib/constants`, `infra/app`, `infra/bus`, `infra/db`; tidak meng-import `uploadspecs` atau feature lain (rule 4.3). `ingest` hanya meng-import `base`, `base/audit`, `base/openapi`, `base/store` (untuk `ErrNameTaken`), `base/upload` (tipe `File`), dan tidak menyentuh SQL.

## 6. Test Scenario

### 6.1 `base/ingest/ingest_test.go` (unit, fake Tx)

Fake repo mencatat urutan panggilan, jumlah tx, dan mengizinkan injeksi `ErrNameTaken`. Kasus:
1. Create: urutan `LockProjectSpecs -> LockSpec -> CountSpecs -> InsertSpec -> ReplaceEndpoints`; status `created`; audit 1x dengan Args `name, source_path, openapi_version, endpoint_count, size_bytes, overwritten=false`; Args TIDAK memuat `source` bila `Source==""`.
2. `Source="git_sync"`: Args memuat `"source":"git_sync"`; `Event.Actor` = `req.Actor` (bukan dari ctx; ctx sengaja tanpa identity).
3. Update (hash beda): `UpdateSpecContent` + `ReplaceEndpoints`, `overwritten=true`, audit 1x.
4. Unchanged (hash sama): tidak ada Insert/Update/ReplaceEndpoints, tidak ada audit.
5. Batas spec (`CountSpecs >= MaxSpecs`, nama baru): `rejected SPEC_LIMIT_REACHED`, field meta (`OpenAPIVersion/Title/APIVersion/EndpointCount`) dikosongkan, tanpa audit.
6. `ErrNameTaken` sekali lalu sukses (retry); dua kali -> error terbungkus `store.ErrNameTaken`.
7. Pipeline non-persist (tanpa tx sama sekali): `TooManyFiles`, `INVALID_PATH` (mis. `../x.yaml`), `NOT_SPEC_FILE` -> `skipped`, `FILE_TOO_LARGE` (pesan memuat `MaxFileBytes`), `INVALID_NAME`, `DUPLICATE_NAME_IN_UPLOAD` (reserved berisi nama), `INVALID_SPEC` (YAML rusak), `INVALID_SPEC` untuk `$ref` eksternal (`issues` tidak kosong, `Reason` = pesan issue pertama, maksimal `openapi.MaxIssues`).
8. BOM dibuang sebelum parse; Swagger 2.0 valid -> `created` dengan `OpenAPIVersion=2.0` (regresi T10).
9. `reserved` terisi hanya setelah sukses persist (created/updated/unchanged), tidak untuk rejected.
10. Audit memakai `context.WithoutCancel` (ctx dibatalkan setelah tx tidak membatalkan audit).

### 6.2 `busupsertspecs/usecase_test.go` (unit)

Fake `Repository` (salin pola `fakeRepo` di `uploadspecs/usecase_test.go`) dan fake audit.
1. **Valid create**: 1 item `api/openapi/billing.yaml` -> `ItemResult{Index, SourcePath:"api/openapi/billing.yaml", Name:"billing", Status:"created", OpenAPIVersion, Title, EndpointCount}`; `Summary.Created=1`; audit `apispec.uploaded` dengan Args `source=git_sync`, aktor = `Request.Actor`, `ProjectID` = request.
2. **Update dengan nama sama**: spec `billing` sudah ada dengan hash beda -> `updated`; `ReplaceEndpoints` dipanggil dengan endpoint baru saja; `Summary.Updated=1`; `source_path` tersimpan = path repo baru.
3. **Unchanged**: konten identik -> `unchanged`, tanpa audit, tanpa write.
4. **Invalid spec**: YAML rusak -> `rejected INVALID_SPEC` + `Issues`; tanpa tx.
5. **`$ref` eksternal**: `rejected INVALID_SPEC`, issue `Pointer` terisi (mis. `#/paths/~1charges/get`), `Reason` = pesan issue.
6. **Nama ganda dalam batch**: `a/billing.yaml` dan `b/billing.json` -> yang pertama `created`, yang kedua `rejected DUPLICATE_NAME_IN_UPLOAD`; hanya satu audit.
7. **Non-spec skipped**: `docs/readme.md` -> `skipped NOT_SPEC_FILE`; `Summary.Skipped=1`.
8. **Too large**: `TooLarge=true`, `Content=nil` -> `rejected FILE_TOO_LARGE`, pesan memuat batas; item lain di batch tetap diproses.
9. **Batas spec**: `maxSpecs=1`, spec pertama created, kedua nama lain -> `rejected SPEC_LIMIT_REACHED`; spec yang sudah ada tetap bisa diperbarui saat penuh.
10. **Validasi request**: `ProjectID` bukan uuid -> `*errs.Error CodeInvalid`, nol tx, nol audit; `Actor.UserID` kosong -> sama.
11. **Path tidak valid**: `SourcePath` kosong atau `../../x.yaml` -> `rejected INVALID_PATH`; `ItemResult.SourcePath` = nilai mentah item.
12. **Urutan dan Index**: 3 item (Index 5, 2, 9) -> `Items` mengikuti urutan input dan memantulkan Index masing-masing.
13. **Error infra** pada tx ke-2 -> error dikembalikan, file ke-1 sudah ter-commit (fake mencatat), item ke-3 tidak diproses.
14. **Ctx dibatalkan antar file** -> error membungkus `context.Canceled`, sisa item tidak diproses.
15. **Aktor dari request, bukan ctx**: ctx dengan identity user lain -> audit tetap memakai `Request.Actor`; ctx tanpa identity tetap sukses.
16. **Items kosong** -> `Response{Items: [] (non-nil), Summary: {}}`, tanpa error.
17. **PAT/isi spec tidak bocor**: Args audit tidak memuat konten spec (cek keys sama dengan upload).

### 6.3 `busupsertspecs/handler_test.go` (unit)

1. `Handle(ctx, &upsertspecs.Request{})` (pointer) atau tipe lain -> error dari `bus.Expect` (pesan jelas), usecase tidak dipanggil.
2. Request nilai valid -> usecase dipanggil, hasil bertipe `upsertspecs.Response` (type assertion nilai).
3. Error usecase (`*errs.Error`) diteruskan tanpa pembungkus tambahan.
4. Lewat registry nyata: `bus.CallTyped[upsertspecs.Response](ctx, reg, constants.BUS_PROJECT_APISPEC_UPSERT_SPECS, req)` berhasil dan bus mendaftarkan handler tepat sekali.

### 6.4 `base/audit/audit_test.go` (aditif)

`Event.Actor` non-nil dipakai walau ctx tanpa identity (record terkirim, `ActorID/ActorLabel` dari event); `Actor` nil + ctx tanpa identity tetap dilewati dengan log error (perilaku lama); `Actor` nil + ctx berisi identity memakai ctx (perilaku lama).

### 6.5 `busupsertspecs/repository_integration_test.go` (tag `integration`, Postgres nyata)

Pakai pola `newITEnv` di `apispec_integration_test.go` (`db.TestDB`, `db.Up`, seed project). Handler dipanggil lewat bus nyata dengan repository nyata.
1. **Spec lain tidak tersentuh**: seed spec `legacy` dan `billing` (via upload/insert); kirim batch berisi hanya `billing` (konten baru) -> `legacy` tetap ada dengan konten/hash/endpoint tak berubah; `billing` diperbarui.
2. **Endpoint diganti sepenuhnya**: spec `billing` awal punya endpoint `GET /a`, `GET /b`; sync dengan konten berisi hanya `GET /c` -> tabel endpoint untuk spec itu hanya `GET /c` (A dan B hilang), `spec.id` tetap sama (update, bukan insert ulang).
3. **Create**: spec baru terbuat dengan `created_by` = `Actor.UserID` dan `created_by_label` = `Actor.Label`; `source_path` = path repo; endpoint terindeks.
4. **Unchanged**: sync kedua dengan konten sama -> `unchanged`, `updated_at` tidak berubah.
5. **Isolasi project**: batch untuk project P1 tidak membuat/mengubah spec di P2 meski nama sama; `CountSpecs` per project terpisah.
6. **Batas spec** dengan `maxSpecs` kecil pada env: spec ke-(N+1) `rejected SPEC_LIMIT_REACHED`, tidak ada baris baru.
7. **Rejected tidak menulis**: batch `[valid, $ref eksternal, YAML rusak]` -> hanya satu baris spec; audit hanya satu (`apispec.uploaded`).
8. **Audit lewat bus nyata**: baris audit menunjukkan `actor_id` = `Request.Actor.UserID` (ctx tanpa identity) dan `args.source = "git_sync"`; `args` tidak memuat konten spec.
9. **Konkurensi**: dua panggilan paralel dengan nama sama pada project sama -> tidak ada duplikat baris (satu `created`, satu `updated`/`unchanged`), tanpa error `ErrNameTaken` bocor ke hasil.
10. **Tx per file**: batch `[valid, valid]` dengan error infra disuntikkan pada file kedua (mis. ctx dibatalkan setelah file pertama) -> file pertama tetap tersimpan.
11. **Regresi upload HTTP**: seluruh `apispec_integration_test.go` (upload multipart, Swagger 2.0, overwrite, limit, audit) hijau tanpa perubahan setelah refaktor.

### 6.6 Regresi wajib langkah B

`uploadspecs/handler_test.go` dan `usecase_test.go` tidak diubah dan hijau (khususnya `TestUploadSpecs_Audit`, `TestUploadSpecs_AuditSurvivesCancel`, `TestUploadSpecs_NameTakenRetry`, `TestUploadSpecs_TxCallOrder`, `TestUploadSpecs_Swagger2Created`). `go test -tags integration` paket apispec hijau. `archtest` hijau.

## 7. E2E Scenarios

Feature ini internal (bus), tidak ada layar atau route sendiri. Cakupan E2E dilakukan melalui `frontend/e2e/git-sync.spec.ts` (overview bagian 11) dan regresi upload.

Positif:
1. Regresi upload API Spec lewat UI (`frontend/e2e/api-spec*.spec.ts`) tetap hijau setelah refaktor (upload `.yaml`, `.json`, Swagger 2.0, timpa nama sama, batas).
2. E2E git-sync positif 5: API Spec > Sync dari Git folder `api/openapi`: `billing.yaml` -> `billing` dibuat (14 endpoint), tampil di daftar API Spec dan indeks endpoint terbaca.
3. Sync ulang repo yang sama -> `tidak berubah`; ubah `billing.yaml` di fixture -> `diperbarui`, endpoint lama yang dihapus dari spec hilang dari indeks; spec yang diunggah manual (`manual-spec`) tetap ada.

Negatif:
1. `payments.json` dengan `$ref` eksternal -> baris `ditolak` dengan issue pointer `#/paths/~1charges/get`; ringkasan "Berhasil sebagian"; spec lain tetap tersimpan.
2. File non-spec di folder (`README.md`, `.png`) tidak diambil/ditulis (dilewati di `gitsync`; bila sampai ke bus, `skipped NOT_SPEC_FILE`).
3. Nama berbeda folder tetapi basename sama (`v1/billing.yaml`, `v2/billing.yaml`) -> satu dibuat, satu `DUPLICATE_NAME_IN_UPLOAD`.
4. Pemanggilan bus oleh consumer yang tidak check-access adalah tanggung jawab consumer: dicakup tes `runsync` (non-anggota -> 404 sebelum bus dipanggil). Tidak ada endpoint HTTP untuk dipanggil langsung, jadi tidak ada skenario akses langsung dari klien.

## 8. Request/Response body

### 8.1 Bus Request (nilai Go, bukan pointer)

```go
upsertspecs.Request{
	ProjectID: "0192f5a0-1111-7aaa-8bbb-000000000001",
	Actor:     upsertspecs.Actor{UserID: "0192f5a0-aaaa-7aaa-8bbb-00000000000a", Role: "developer", Label: "Rina"},
	Source:    upsertspecs.SourceGitSync,
	Meta:      upsertspecs.Meta{IP: "10.1.2.3", UserAgent: "Mozilla/5.0"},
	Items: []upsertspecs.Item{
		{Index: 0, SourcePath: "api/openapi/billing.yaml", Content: []byte("openapi: 3.0.3\ninfo: { title: Billing API, version: 1.0.0 }\npaths: {...}")},
		{Index: 1, SourcePath: "api/openapi/payments.json", Content: []byte(`{"openapi":"3.0.3","paths":{"/charges":{"get":{"responses":{"200":{"$ref":"other.yaml#/x"}}}}}}`)},
		{Index: 2, SourcePath: "api/openapi/README.md", Content: []byte("# api")},
		{Index: 3, SourcePath: "api/openapi/huge.yaml", TooLarge: true},
	},
}
```

### 8.2 Bus Response (campuran)

```go
upsertspecs.Response{
	Items: []upsertspecs.ItemResult{
		{Index: 0, SourcePath: "api/openapi/billing.yaml", Name: "billing", Status: "created",
			OpenAPIVersion: "3.0.3", Title: "Billing API", EndpointCount: 14, Issues: []upsertspecs.Issue{}},
		{Index: 1, SourcePath: "api/openapi/payments.json", Name: "payments", Status: "rejected",
			ReasonCode: "INVALID_SPEC", Reason: "$ref eksternal tidak didukung",
			Issues: []upsertspecs.Issue{{Code: "EXTERNAL_REF", Message: "$ref eksternal tidak didukung", Pointer: "#/paths/~1charges/get", Line: 0}}},
		{Index: 2, SourcePath: "api/openapi/README.md", Status: "skipped",
			ReasonCode: "NOT_SPEC_FILE", Reason: "bukan file spec (.yaml/.yml/.json)", Issues: []upsertspecs.Issue{}},
		{Index: 3, SourcePath: "api/openapi/huge.yaml", Status: "rejected",
			ReasonCode: "FILE_TOO_LARGE", Reason: "ukuran file melebihi batas 2097152 byte", Issues: []upsertspecs.Issue{}},
	},
	Summary: upsertspecs.Summary{Created: 1, Updated: 0, Unchanged: 0, Skipped: 1, Rejected: 2},
}
```

Bentuk JSON setara (untuk dokumentasi dan fixture tes; bus sendiri tidak men-serialize):

```json
{
  "items": [
    { "index": 0, "source_path": "api/openapi/billing.yaml", "name": "billing", "status": "created", "reason_code": "", "reason": "", "issues": [], "openapi_version": "3.0.3", "title": "Billing API", "endpoint_count": 14 },
    { "index": 1, "source_path": "api/openapi/payments.json", "name": "payments", "status": "rejected", "reason_code": "INVALID_SPEC", "reason": "$ref eksternal tidak didukung",
      "issues": [ { "code": "EXTERNAL_REF", "message": "$ref eksternal tidak didukung", "pointer": "#/paths/~1charges/get", "line": 0 } ],
      "openapi_version": "", "title": "", "endpoint_count": 0 },
    { "index": 2, "source_path": "api/openapi/README.md", "name": "", "status": "skipped", "reason_code": "NOT_SPEC_FILE", "reason": "bukan file spec (.yaml/.yml/.json)", "issues": [], "openapi_version": "", "title": "", "endpoint_count": 0 },
    { "index": 3, "source_path": "api/openapi/huge.yaml", "name": "", "status": "rejected", "reason_code": "FILE_TOO_LARGE", "reason": "ukuran file melebihi batas 2097152 byte", "issues": [], "openapi_version": "", "title": "", "endpoint_count": 0 }
  ],
  "summary": { "created": 1, "updated": 0, "unchanged": 0, "skipped": 1, "rejected": 2 }
}
```

Catatan: `Name` kosong untuk `NOT_SPEC_FILE` dan `FILE_TOO_LARGE` karena pipeline berhenti sebelum `DeriveName`.

### 8.3 Error bus (error Go, tidak ada HTTP)

Tipe request salah (`bus.Expect`): error biasa, mis. `bus: tipe request tidak sesuai: want upsertspecs.Request, got *upsertspecs.Request`. Consumer memperlakukannya sebagai bug (500).

`ProjectID` bukan uuid:
```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "permintaan tidak valid", "details": { "fields": { "project_id": "project_id harus uuid" } } } }
```

`Actor.UserID` kosong:
```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "permintaan tidak valid", "details": { "fields": { "actor": "aktor wajib diisi" } } } }
```

Kegagalan DB / ctx dibatalkan: error Go terbungkus (`busupsertspecs: dibatalkan: context canceled`, `ingest: simpan "billing": ...`) tanpa `*errs.Error`; consumer (`runsync`) memetakannya menjadi 500 atau `TIMEOUT` (504) sesuai ctx.

### 8.4 Respons HTTP upload yang TIDAK berubah (regresi)

`POST /api/projects/{projectID}/api-specs/upload` tetap mengembalikan `{ "results": [ { "file", "source_path", "name", "status", "reason_code", "reason", "issues", "openapi_version", "title", "api_version", "endpoint_count" } ], "summary": { "created", "updated", "unchanged", "skipped", "rejected" } }` persis seperti sebelum refaktor; bentuk error tetap `{"error": {"code", "message", "details"}, "trace_id"}`.

### 8.5 Audit (`record.Request` ringkas, hasil sync)

```json
{ "action": "apispec.uploaded", "actor_type": "user", "actor_id": "0192f5a0-aaaa-7aaa-8bbb-00000000000a", "actor_label": "Rina",
  "project_id": "0192f5a0-1111-7aaa-8bbb-000000000001", "target_type": "api_spec", "target": "billing",
  "args": { "name": "billing", "source_path": "api/openapi/billing.yaml", "openapi_version": "3.0.3", "endpoint_count": 14, "size_bytes": 20480, "overwritten": false, "source": "git_sync" },
  "status": "ok" }
```
