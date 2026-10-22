# T14 usecase `busupsertfiles` — Plan

Tiket: `docs/tickets/T14-project-git-sync.md`. Overview (wajib diikuti, menang bila bertentangan): `docs/superpowers/plans/project-git-sync/overview.md`.
Usecase ini milik module **knowledge (T05)**, bukan gitsync. Ia dikonsumsi `gitsync/runsync`.

## 1. Metadata

| Atribut | Nilai |
|---|---|
| Ticket | T14 (kriteria 3 dan 4) |
| Scope | backend saja |
| Domain / Module | project / knowledge |
| Folder | `backend/internal/domain/project/internal/knowledge/busupsertfiles` |
| Endpoint (bus) | `project.knowledge.upsert-files` |
| Method | Call (sinkron, bukan Publish) |
| Version | v1 (kontrak baru); tidak ada endpoint HTTP, OpenAPI tidak berubah |
| Konstanta | `constants.BUS_PROJECT_KNOWLEDGE_UPSERT_FILES` |
| Kontrak | `backend/lib/contracts/project/knowledge/upsertfiles` |
| Provider / Consumer | T14 knowledge / T14 gitsync `runsync` |
| Aksi audit | `knowledge.upload.completed` (sudah ada), `Args.source = "git_sync"` |

## 2. Summary Plan

Dua langkah berurutan di satu usecase:

1. **Refaktor tanpa perubahan perilaku (D1).** Blok transaksi di `uploadfiles/usecase.go` (`LockProject -> CountFiles -> ExistingHashes -> base.PlanUpload -> InsertFiles/UpdateFiles`) dipindah ke paket bersama `knowledge/base/ingest` (fungsi `Apply`). `uploadfiles` memakainya. Semua tes `uploadfiles` yang ada harus hijau tanpa perubahan asersi. SQL insert/update batch yang sekarang ada di `uploadfiles/repository.go` dipindah ke `base/store` agar tidak diduplikasi di feature baru (repository feature tetap hanya delegasi).
2. **Feature bus baru `busupsertfiles`.** Handler `project.knowledge.upsert-files` menulis banyak file sekaligus dari consumer (gitsync). Aturan sama dengan upload karena memakai `base.ClassifyPart`, `base.PlanUpload`, `ingest.Apply`, `base.Summarize`. Beda dengan upload: tanpa pemeriksaan keanggotaan (consumer sudah `check-access`), aktor dan meta diambil dari Request (bukan ctx), `TargetPrefix` digabung ke path tiap item, tidak ada penghapusan file, dan Args audit membawa `source`.

Keputusan yang diambil di plan ini (tidak ditentukan eksplisit di overview):

- `Items` kosong: kembalikan `Response{}` kosong tanpa error, tanpa transaksi, tanpa audit (consumer dapat memanggilnya saat semua entri dilewati; error di sini akan memperumit runsync). Kontras dengan upload yang mengembalikan `no_files`.
- `Request.Source` harus `upsertfiles.SourceGitSync`; selain itu `*errs.Error CodeInvalid`. Mencegah pemanggil lain memakai pintu ini tanpa audit yang benar.
- `Actor.UserID` divalidasi sebagai uuid (kolom `created_by` bertipe uuid; menolak lebih awal daripada gagal di DB). Ini mencakup syarat "tidak kosong".
- `Index` dipantulkan apa adanya dan diharapkan unik dari consumer. Handler tidak mendeduplikasi Index; urutan stabil (`sort.SliceStable`) dipakai bila ada Index sama.
- Batas jumlah file per sync (`MaxUploadFiles`) dan total byte (`MaxUploadBytes`) adalah tanggung jawab consumer (D3). Provider hanya menegakkan batas per file (`MaxFileBytes`) dan batas isi project (`MaxProjectFiles`), sama dengan upload.
- Path hasil gabungan memakai `base.NormalizePath` lewat `ClassifyPart`, jadi `..`, hidden, tipe, UTF-8, panjang ditangani rule yang sama. `TargetPrefix` TIDAK di-strip slash depan (consumer sudah menormalisasi; `/x` ditolak `path_absolute`).

## 3. Plan (langkah berurutan)

Prasyarat (dikerjakan foundation langkah 0, periksa dulu; buat bila belum ada):

- `backend/lib/constants/bus.go`: tambah
  ```go
  // Provider: T14 project/knowledge (feature busupsertfiles). Consumer: project/gitsync runsync.
  // Kontrak: lib/contracts/project/knowledge/upsertfiles. Menulis; tanpa cek keanggotaan (consumer wajib check-access).
  const BUS_PROJECT_KNOWLEDGE_UPSERT_FILES = "project.knowledge.upsert-files"
  ```
- `backend/lib/contracts/project/knowledge/upsertfiles/upsertfiles.go`: persis bagian 6.4.2 overview (`SourceGitSync`, `Actor{UserID,Role,Label}`, `Meta{IP,UserAgent}`, `Item{Index,Path,Content,TooLarge}`, `Request{ProjectID,Actor,TargetPrefix,Source,Meta,Items}`, `ItemResult{Index,Path,Status,Reason}`, `Summary{Created,Updated,Unchanged,Skipped,Rejected}`, `Response{Items,Summary}`). Doc comment paket: provider tidak memeriksa keanggotaan, selalu membatasi query pada `ProjectID`, tidak menghapus file; request dikirim sebagai NILAI.

### Langkah 1 — refaktor (commit tersendiri: "refactor(knowledge): ekstrak ingest.Apply dan SQL tulis batch")

1.1 Buat `knowledge/base/ingest/ingest.go`:

```go
// Package ingest berisi orkestrasi tulis batch file knowledge yang dipakai bersama
// uploadfiles dan busupsertfiles (D1/K7). Dipanggil DI DALAM transaksi pemanggil.
package ingest

type Tx interface {
	LockProject(ctx context.Context, projectID string) error
	CountFiles(ctx context.Context, projectID string) (int, error)
	ExistingHashes(ctx context.Context, projectID string, paths []string) (map[string][]byte, error)
	InsertFiles(ctx context.Context, files []base.File) error
	UpdateFiles(ctx context.Context, files []base.File) error
}

// Apply: LockProject -> CountFiles -> ExistingHashes(paths incoming) -> base.PlanUpload(incoming, existing, count, maxProjectFiles)
// -> NewFile untuk created/updated (bytesWritten += SizeBytes) -> InsertFiles (jika ada) -> UpdateFiles (jika ada).
// Mengembalikan keputusan terencana (urut Index, hanya untuk incoming) dan bytesWritten.
func Apply(ctx context.Context, tx Tx, projectID string, actor base.Actor,
	incoming []base.Incoming, maxProjectFiles int) ([]base.Decision, int, error)
```

Isi = salinan harfiah blok `InTx` saat ini, termasuk pesan pembungkus error yang sama (`lock project`, `count files`, `existing hashes`, `insert files`, `update files`), dan `byIndex` map. Hanya `incoming` kosong yang tidak ditangani di sini (pemanggil menjaganya: tidak membuka tx bila `incoming` kosong).

1.2 Pindahkan SQL tulis ke `knowledge/base/store/store.go` (atau file baru `base/store/write.go` di paket yang sama): `const chunkSize = 100`, `insertSQL`, `updateSQL`, `hashRow`, `toColumns`, dan method `(*Store) ExistingHashes`, `(*Store) InsertFiles`, `(*Store) UpdateFiles` (isi sama persis dengan `uploadfiles/repository.go`; pesan "rows affected" memakai awalan `store:` menggantikan `uploadfiles:`). Tambahkan `var _ ingest.Tx` TIDAK dilakukan di store (store tidak boleh meng-import `ingest`; cukup `ingest` yang menjadi konsumen interface).

1.3 `uploadfiles/repository.go`: `Repo` tinggal delegasi ke `r.s.ExistingHashes/InsertFiles/UpdateFiles` (hapus SQL, `columns`, `write`, `chunkSize`). Interface `repository` di `usecase.go` tetap (memuat `InTx(func(repository) error)` + 5 method), sehingga memenuhi `ingest.Tx` secara struktural.

1.4 `uploadfiles/usecase.go`: di dalam `u.repo.InTx(...)` ganti blok panjang dengan
```go
var err error
planned, bytesWritten, err = ingest.Apply(ctx, tx, in.ProjectID, actor, incoming, u.limits.MaxProjectFiles)
return err
```
Pembungkus `fmt.Errorf("uploadfiles: batch: %w", err)` di luar tetap. Urutan pemanggilan DB, audit setelah commit, dan hasil tidak berubah. Hapus import `fmt` bila tidak terpakai lagi.

1.5 Tes ingest (lihat bagian 6) dan jalankan seluruh tes `uploadfiles` (unit + `-tags integration`) serta `knowledge_integration_test.go` sebelum lanjut. Tidak boleh ada perubahan asersi di tes lama. Bila `TestInsertFilesChunksAndColumns` atau tes pesan error memeriksa awalan `uploadfiles:`, sesuaikan HANYA awalan itu dan catat di pesan commit.

### Langkah 2 — feature `busupsertfiles` (commit: "feat(knowledge): handler bus project.knowledge.upsert-files")

Folder `knowledge/busupsertfiles/`, mengikuti pola `buslistfiles` (handler tipis, usecase orkestrasi, repository delegasi ke store).

2.1 `init.go`
```go
func Init(d app.Deps) {
	limits := base.LimitsFrom(d.Config)
	uc := NewUsecase(NewRepo(store.New(d.DB)), audit.New(d.Bus, d.Log), limits)
	h := &Handler{uc: uc}
	d.Bus.Register(constants.BUS_PROJECT_KNOWLEDGE_UPSERT_FILES, h.Handle)
}
```
Tidak ada route HTTP, tidak ada `access` (sengaja).

2.2 `handler.go`
```go
type Handler struct{ uc *Usecase }

func (h *Handler) Handle(ctx context.Context, req any) (any, error) {
	in, err := bus.Expect[upsertfiles.Request](req) // pointer / tipe lain -> bus.ErrBadRequestType
	if err != nil { return nil, err }
	return h.uc.Execute(ctx, in)
}
```
Mengembalikan `upsertfiles.Response` sebagai NILAI (bukan pointer), agar `bus.CallTyped[upsertfiles.Response]` bekerja.

2.3 `repository.go`
```go
type repository interface {
	InTx(ctx context.Context, fn func(ingest.Tx) error) error
}
type Repo struct{ s *store.Store }
func NewRepo(s *store.Store) *Repo
func (r *Repo) InTx(ctx, fn func(ingest.Tx) error) error { return r.s.InTx(ctx, func(ts *store.Store) error { return fn(ts) }) }
```
`*store.Store` memenuhi `ingest.Tx` (method `LockProject`, `CountFiles`, `ExistingHashes`, `InsertFiles`, `UpdateFiles` dari langkah 1.2). Tidak ada SQL di feature ini. Semua query di store sudah memakai `projectID` argumen, sehingga selalu dibatasi `Request.ProjectID`.

2.4 `usecase.go`
```go
type Usecase struct {
	repo   repository
	audit  audit.Recorder
	limits base.Limits
}
func NewUsecase(repo repository, au audit.Recorder, limits base.Limits) *Usecase

const maxAuditPaths = 20

func (u *Usecase) Execute(ctx context.Context, in upsertfiles.Request) (upsertfiles.Response, error)
```
Alur `Execute` (berurutan, tidak dipecah menjadi usecase lain):

1. Validasi: `uuid.Parse(in.ProjectID)` gagal -> `base.Invalid(base.ReasonInvalidProjectID)`; `uuid.Parse(in.Actor.UserID)` gagal -> `errs.WithDetails(errs.CodeInvalid, "aktor tidak valid", ...)` (pakai pola `base.InvalidMsg` dengan `ReasonInvalidProjectID`-setara bila tidak ada Reason khusus; boleh menambah `ReasonInvalidActor Reason = "invalid_actor"` di `base/reasons.go` + pesan + `errors_test`/reason test yang sudah ada untuk memastikan semua Reason punya pesan); `in.Source != upsertfiles.SourceGitSync` -> `*errs.Error CodeInvalid` (reason `invalid_source`, tambahkan konstanta sama bila perlu). Project ID dinormalisasi `pid.String()` dan dipakai di semua langkah berikut.
2. `prefix, reason := base.NormalizePrefix(in.TargetPrefix)`; `reason != ""` -> `base.InvalidMsg(reason, fmt.Sprintf("prefix tujuan %q tidak valid: %s", in.TargetPrefix, reason.Message()))` (`*errs.Error` CodeInvalid, details `{"reason": ...}`), SEBELUM tulis apa pun. Prefix yang sudah valid dipakai sebagai-ada (tanpa trailing slash).
3. `len(in.Items) == 0` -> return `upsertfiles.Response{Items: []upsertfiles.ItemResult{}}` (tanpa tx, tanpa audit).
4. Untuk tiap `it` di `in.Items` (urut slice): `rawName := it.Path` bila `prefix == ""`, selain itu `prefix + "/" + it.Path`. Panggil `base.ClassifyPart(it.Index, rawName, it.Content, it.TooLarge, u.limits.MaxFileBytes)`. Bila `ok == false`, tampung `Decision` ke `rejected`; bila `ok`, tampung `Incoming`. (Jangan menolak `Path` kosong secara khusus: `prefix + "/"` + "" akan menghasilkan `path_empty_segment`, dan prefix kosong + "" menghasilkan `path_empty`; keduanya dari rule yang sama.)
5. Bila `incoming` tidak kosong: `u.repo.InTx(ctx, func(tx ingest.Tx) error { planned, bytesWritten, err = ingest.Apply(ctx, tx, projectID, actor, incoming, u.limits.MaxProjectFiles); return err })` dengan `actor := base.Actor{ID: in.Actor.UserID, Role: in.Actor.Role, Label: in.Actor.Label}`. Error dibungkus `fmt.Errorf("busupsertfiles: batch: %w", err)` dan dikembalikan TANPA audit (error infrastruktur, sama dengan upload).
6. `all := append(decisions, planned...)`, `base.SortByIndex(all)`, `sum := base.Summarize(all)`, `paths, trunc := base.ChangedPaths(all, maxAuditPaths)`.
7. Audit SETELAH commit (fail-open): `u.audit.Record(ctx, audit.Event{Action: record.ActionKnowledgeUploadCompleted, ProjectID: projectID, Actor: actor, TargetType: audit.TargetKnowledge, Target: "", Args: map[string]any{"files": len(in.Items), "source": in.Source}, Result: map[string]any{"created","updated","unchanged","skipped","rejected","bytes": bytesWritten,"paths": paths,"paths_truncated": trunc}, Meta: base.RequestMeta{IP: in.Meta.IP, UserAgent: in.Meta.UserAgent}})`. Result identik dengan upload. Isi file tidak pernah masuk Args/Result/log.
8. Bangun `Response`: untuk tiap decision (sudah urut Index) `ItemResult{Index: d.Index, Path: d.Path, Status: string(d.Status), Reason: string(d.Reason)}`; `Summary` dari `sum`. `Path` = path akhir (setelah prefix) untuk yang lolos, atau nama mentah terpotong 1024 rune untuk yang ditolak path-nya (perilaku `ClassifyPart`).

2.5 Wiring: `knowledge/base.go`, bagian "Handler bus", tambah satu baris `busupsertfiles.Init(d)` (+ import). Tidak ada perubahan lain di file itu.

2.6 Katalog: `docs/tickets/README.md` bagian "Katalog endpoint bus", tambah baris
`| \`project.knowledge.upsert-files\` | Call | T14 | T14 (gitsync) |`
dan satu kalimat lokasi kontrak di paragraf kontrak (`lib/contracts/project/knowledge/upsertfiles`, konstanta `BUS_PROJECT_KNOWLEDGE_UPSERT_FILES`, tanpa cek keanggotaan, tidak menghapus). Jika foundation sudah menambahkan baris ini, jangan digandakan.

2.7 Tulis tes (bagian 6) lalu jalankan: `go test ./internal/domain/project/internal/knowledge/...`, `go test -tags integration ./internal/domain/project/internal/knowledge/...`, `go test ./internal/archtest/...`, lint.

## 4. Migration

Tidak ada. Tabel `knowledge_files` dan kolom yang dipakai sudah ada (T05). Tidak ada perubahan schema.

## 5. Changes

| File / folder | Jenis |
|---|---|
| `backend/lib/constants/bus.go` | ubah (bila belum oleh foundation): konstanta `BUS_PROJECT_KNOWLEDGE_UPSERT_FILES` |
| `backend/lib/contracts/project/knowledge/upsertfiles/upsertfiles.go` | baru (bila belum oleh foundation) |
| `knowledge/base/ingest/{ingest.go,ingest_test.go}` | baru |
| `knowledge/base/store/store.go` (atau `write.go`) + `store_integration_test.go` | ubah: tambah `ExistingHashes`, `InsertFiles`, `UpdateFiles` (dipindah dari uploadfiles) |
| `knowledge/base/reasons.go` (+ tes reason yang ada) | ubah, hanya bila menambah `invalid_actor` / `invalid_source` |
| `knowledge/uploadfiles/repository.go` | ubah: delegasi ke store |
| `knowledge/uploadfiles/usecase.go` | ubah: pakai `ingest.Apply` |
| `knowledge/busupsertfiles/{init.go,handler.go,usecase.go,repository.go}` | baru |
| `knowledge/busupsertfiles/{handler_test.go,usecase_test.go,repository_integration_test.go}` | baru |
| `knowledge/base.go` | ubah: `busupsertfiles.Init(d)` |
| `docs/tickets/README.md` | ubah: baris katalog bus + kalimat kontrak |

Tidak berubah: `app.Deps`, `uploadfiles` handler/params, OpenAPI, FE, module lain.

## 6. Test Scenario

### 6.1 Unit `base/ingest` (`ingest_test.go`, fake `Tx` mencatat urutan panggilan)

- Urutan panggilan: `LockProject, CountFiles, ExistingHashes, InsertFiles, UpdateFiles` (Insert/Update dilewati bila kosong).
- `ExistingHashes` menerima semua path incoming (urut sesuai incoming).
- created baru -> `InsertFiles` dengan `base.NewFile(projectID, ...)`, `CreatedBy = actor.ID`; updated -> `UpdateFiles`; unchanged -> tidak ada tulis; `bytesWritten` = jumlah `SizeBytes` created+updated.
- Kuota: `count + created >= maxProjectFiles` -> `rejected project_file_limit`; file existing (update) tetap lolos saat kuota penuh.
- Duplikat path dalam batch -> yang kedua `rejected duplicate_in_request`.
- Error tiap langkah dibungkus dengan pesan yang diharapkan dan menghentikan langkah berikutnya (mis. error `LockProject` -> tidak ada `CountFiles`).

### 6.2 Regresi `uploadfiles`

Seluruh `usecase_test.go` (`TestDBCallOrderAndAuditAfterCommit`, `TestUnchangedNotWritten`, `TestDuplicateInRequest`, `TestQuota`, `TestRepoErrorNoAudit`, `TestAuditEvent`, ...), `handler_test.go`, `params_test.go`, dan `repository_integration_test.go` hijau tanpa perubahan asersi.

### 6.3 Unit `busupsertfiles/handler_test.go`

- Tipe salah (`struct{}`, `"x"`, `*upsertfiles.Request`) -> `errors.Is(err, bus.ErrBadRequestType)`, repo/audit tidak dipanggil.
- Sukses lewat `bus.CallTyped[upsertfiles.Response]` terhadap registry nyata: respons nilai, `Items` urut Index.
- `Init` mendaftarkan konstanta tepat sekali (registrasi kedua = panic, diuji di `routes`/registrasi tes module bila ada).

### 6.4 Unit `busupsertfiles/usecase_test.go` (fake repo `InTx` + fake audit, pola `uploadfiles/usecase_test.go`)

Validasi:
- ProjectID bukan uuid -> `*errs.Error CodeInvalid`; tanpa transaksi/audit.
- `Actor.UserID` kosong / bukan uuid -> CodeInvalid.
- `Source` kosong atau `"upload"` -> CodeInvalid.
- Prefix `a/../b`, `..`, `/abs`, `//`, karakter kontrol, > 512 rune -> CodeInvalid dengan `details.reason` sesuai (`path_traversal`, `path_absolute`, ...), nol tulis, nol audit.
- `Items` kosong -> `Response` kosong, tanpa tx, tanpa audit.

Perilaku:
- created / updated / unchanged: repo palsu dengan `existing` hash; status dan `Summary` benar; unchanged tidak ditulis.
- Prefix diterapkan: prefix `guides`, item `auth/login.md` -> `ItemResult.Path == "guides/auth/login.md"` dan `InsertFiles` menerima path tersebut. Prefix kosong -> path apa adanya.
- `..` di dalam `Item.Path` (`a/../b.md`) -> `rejected path_traversal`, Path mentah; item lain tetap diproses.
- Hidden (`.github/x.md`, `docs/.env.md`) -> `skipped hidden`.
- Tipe tidak didukung (`img/x.png`) -> `rejected unsupported_type` (consumer normalnya sudah menyaring; ini lapis kedua).
- `TooLarge=true` -> `rejected too_large`, `Content` nil tidak dibaca; isi > `MaxFileBytes` tanpa flag -> `rejected too_large`.
- UTF-8 tidak valid / NUL -> `rejected invalid_encoding`.
- Duplikat path dalam satu request (dua Index, path sama setelah prefix) -> yang kedua `rejected duplicate_in_request`.
- Batas project per file: `MaxProjectFiles` = 3, project berisi 2, tiga file baru -> satu created, dua `rejected project_file_limit`; file existing diperbarui tetap lolos.
- Semua item ditolak -> tidak membuka tx, tetap audit (Result `rejected = n`).
- Urutan `Response.Items` mengikuti `Index` meski input tidak berurutan dan campuran rejected/planned.
- Audit: tepat satu event setelah commit (urutan log `tx` lalu `audit`); `Action == knowledge.upload.completed`, `ProjectID`, `Actor` dari Request (bukan ctx), `Meta` dari Request, `Args == {files: len(Items), source: "git_sync"}`, `Result` memuat `created/updated/unchanged/skipped/rejected/bytes/paths/paths_truncated`; `paths` maks 20 dengan `paths_truncated` benar. Canary isi file ("RAHASIA-ISI") tidak muncul di Args/Result.
- Error repo -> error dibungkus `busupsertfiles: batch:`, tanpa audit.
- Tanpa pemeriksaan akses: usecase tidak punya dependensi `access` (dijamin oleh konstruktor).

### 6.5 Integration `repository_integration_test.go` (tag `integration`, Postgres nyata, memakai `Usecase` + `Repo` sungguhan dan fake audit)

- Prefix diterapkan ke DB: request prefix `guides` -> baris `knowledge_files.path = 'guides/auth/login.md'`, `created_by`/`created_by_label` dari Request.Actor.
- File di luar prefix tidak tersentuh: seed `other/x.md`, `guides/old.md`, `README.md`; upsert di prefix `guides` file `new.md` dan `old.md` (isi beda) -> `other/x.md` dan `README.md` identik (`updated_at`, hash); hanya `guides/old.md` diperbarui.
- File yang tidak ada di request tetap ada: seed `guides/keep.md`, upsert tanpa `keep.md` -> `keep.md` masih ada (tidak ada penghapusan).
- Re-upsert isi sama -> semua `unchanged`, `updated_at` tidak berubah.
- Isolasi project: dua project A dan B dengan path sama `docs/a.md`; upsert ke A tidak mengubah baris B; `ExistingHashes` A tidak melihat B; kuota dihitung per project.
- Atomik: satu transaksi untuk seluruh batch (paksa kegagalan di `UpdateFiles` lewat constraint/ctx batal -> tidak ada baris dari `InsertFiles` yang tersisa).
- Konkurensi kuota: dua `Execute` paralel pada project hampir penuh tidak melampaui `MaxProjectFiles` (advisory lock `LockProject`).
- Batch > 100 file (chunk) benar (250 file, seperti tes `InsertFiles` upload).

### 6.6 Store (`base/store/store_integration_test.go`)

Pindahkan/duplikasi kasus `ExistingHashes` (uuid tidak valid -> map kosong), `InsertFiles`/`UpdateFiles` chunk dan `rows affected` mismatch ke tes store; tes repository upload lama tetap lulus melalui delegasi.

### 6.7 Arch

`backend/internal/archtest` hijau: `busupsertfiles` hanya meng-import `knowledge/base/...`, `lib/...`, `infra/bus`, `infra/app`; tidak meng-import feature lain (`uploadfiles`) maupun module lain.

## 7. E2E Scenarios

Handler ini tidak punya endpoint HTTP, jadi tidak ada e2e Playwright sendiri. Dicakup lewat e2e `frontend/e2e/git-sync.spec.ts` (overview bagian 11) dan integration `gitsync/runsync` terhadap handler nyata. Skenario yang bergantung pada usecase ini:

Positif:
1. Sync knowledge `docs/` tanpa prefix: `auth/login-flow.md` dan `api/rate-limit.md` `created`; sync ulang -> `unchanged`; isi repo diubah -> `updated`; `notes/manual.md` buatan manual tetap ada (tidak ada penghapusan).
2. Prefix "Simpan di folder" `guides` -> file masuk `guides/auth/login-flow.md`; pohon knowledge menampilkan folder `guides`.
3. Satu baris audit `knowledge.upload.completed` dengan `args.source = git_sync` muncul di audit log project, bersama `git_sync.run`.

Negatif:
1. Prefix tidak valid (`a/../b`) yang lolos validasi consumer -> bus mengembalikan CodeInvalid; sync gagal tanpa file tertulis (diuji integration runsync dengan handler nyata).
2. Path repo `docs/legacy/../x.md` -> item `rejected path_traversal` di laporan; file lain tetap tertulis (`partial`).
3. File dengan `TooLarge` -> `rejected too_large` di laporan; project penuh -> `rejected project_file_limit`.
4. Konkurensi: sync dan upload manual bersamaan pada project yang sama tidak melampaui kuota.

## 8. Request/Response body

Tidak ada HTTP. Kontrak bus (request dikirim sebagai NILAI):

Request:
```go
upsertfiles.Request{
	ProjectID:    "0192f5a0-1111-7aaa-8bbb-000000000001",
	Actor:        upsertfiles.Actor{UserID: "0192f5a0-aaaa-7aaa-8bbb-00000000000a", Role: "developer", Label: "Rina"},
	TargetPrefix: "guides",
	Source:       upsertfiles.SourceGitSync,
	Meta:         upsertfiles.Meta{IP: "10.1.2.3", UserAgent: "Mozilla/5.0"},
	Items: []upsertfiles.Item{
		{Index: 0, Path: "auth/login-flow.md", Content: []byte("# Login")},
		{Index: 1, Path: "api/rate-limit.md", Content: []byte("# Rate limit")},
		{Index: 2, Path: "big/manual.md", TooLarge: true},
		{Index: 3, Path: "legacy/../x.md", Content: []byte("x")},
		{Index: 4, Path: ".github/notes.md", Content: []byte("x")},
	},
}
```

Response sukses (nilai):
```go
upsertfiles.Response{
	Items: []upsertfiles.ItemResult{
		{Index: 0, Path: "guides/auth/login-flow.md", Status: "created"},
		{Index: 1, Path: "guides/api/rate-limit.md", Status: "unchanged"},
		{Index: 2, Path: "guides/big/manual.md", Status: "rejected", Reason: "too_large"},
		{Index: 3, Path: "guides/legacy/../x.md", Status: "rejected", Reason: "path_traversal"},
		{Index: 4, Path: "guides/.github/notes.md", Status: "skipped", Reason: "hidden"},
	},
	Summary: upsertfiles.Summary{Created: 1, Unchanged: 1, Skipped: 1, Rejected: 2},
}
```

Error (Go, dipetakan ke JSON bila sampai ke HTTP oleh consumer; handler bus mengembalikan `*errs.Error`):
```go
// prefix tidak valid
&errs.Error{Code: errs.CodeInvalid, Message: `prefix tujuan "a/../b" tidak valid: path tidak boleh memuat segmen . atau ..`,
	Details: map[string]any{"reason": base.ReasonPathTraversal}}
// ProjectID bukan uuid
&errs.Error{Code: errs.CodeInvalid, Message: "ProjectID tidak valid", Details: map[string]any{"reason": base.ReasonInvalidProjectID}}
// tipe request salah
bus.ErrBadRequestType
```
Bentuk JSON bila consumer meneruskannya (format standar):
```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "prefix tujuan \"a/../b\" tidak valid: path tidak boleh memuat segmen . atau ..", "details": { "reason": "path_traversal" } }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```
Consumer `runsync` menerjemahkan Response ini ke `ReportItem` (`repo_path` dari `Index`, `target` = `ItemResult.Path`, `reason_code` = `ItemResult.Reason`).

Audit (`record.Request` ringkas, aktor dari Request):
```json
{ "action": "knowledge.upload.completed", "actor_type": "user", "actor_id": "0192f5a0-aaaa-7aaa-8bbb-00000000000a", "actor_label": "Rina",
  "project_id": "0192f5a0-1111-7aaa-8bbb-000000000001", "target_type": "knowledge", "target": "",
  "args": { "files": 5, "source": "git_sync" },
  "result": { "created": 1, "updated": 0, "unchanged": 1, "skipped": 1, "rejected": 2, "bytes": 7, "paths": ["guides/auth/login-flow.md"], "paths_truncated": false },
  "ip": "10.1.2.3", "user_agent": "Mozilla/5.0", "status": "ok" }
```
