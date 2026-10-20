# T06 project/api-spec — Usecase Plan: `countspecs`

Tiket: `docs/tickets/T06-project-api-spec.md`. Plan bersama (keputusan mengikat, `base/`, migrasi, config): `docs/superpowers/plans/project-api-spec/overview.md`. Dokumen ini hanya memuat usecase `countspecs` (U11, gelombang A). Bila bertentangan dengan overview, overview menang. Kontrak consumer-driven dimiliki T03: `docs/superpowers/plans/project-management/overview.md` bagian 5.1 dan 5.4 (D1).
Konvensi: `docs/tickets/README.md` dan `rules.txt`. Tidak ada `CLAUDE.md`, `.claude/rules/`, maupun knowledge doc module apispec.

## Metadata

| Field | Nilai |
|---|---|
| Ticket | T06 |
| Scope | backend saja (handler bus; tanpa HTTP, tanpa UI) |
| Domain | project |
| Module | api-spec (folder Go `apispec`) |
| Feature | `countspecs` |
| Resource root | `backend/internal/domain/project/internal/apispec` |
| Endpoint (bus) | `project.apispec.count-specs` (konstanta `constants.BUS_PROJECT_APISPEC_COUNT_SPECS`, milik T03) |
| Method (mode bus) | Call |
| Version | v1 (kontrak `lib/contracts/project/management/contentcount`, milik T03) |
| Provider / Consumer | T06 / T03 (`listprojects`, `getproject`, `deleteproject`; consumer fail-soft) |
| Akses | Tidak memeriksa akses user (pemanggil internal). WAJIB membatasi query pada `ProjectIDs` request (rule 6.10) |
| Audit | Tidak ada |
| Bus dikonsumsi | Tidak ada |
| AC | AC7 (handler bus tidak pernah mengembalikan data project lain) |

allowedPaths (hanya ini yang boleh disentuh):
- `backend/internal/domain/project/internal/apispec/countspecs/`
- `backend/internal/domain/project/internal/apispec/base.go` (tepat satu baris: `countspecs.Init(d)`)
- `backend/internal/domain/project/internal/apispec/apispec_integration_test.go` (hanya menambah tes count, bila file sudah ada)

## Konteks yang dimuat

- Overview T06: 3.2 (struktur feature bus: tanpa `params.go`, `handler.go` memakai `bus.Expect`), 7.5 (provider `contentcount`), 7.7 (katalog), 9 (migrasi `api_specs`), 17 (ringkasan U11).
- Overview T03: 5.4 (doc comment `contentcount`: `MaxProjectIDs = 100`, `Request{ProjectIDs []string}`, `Response{Counts map[string]int}`), D1 (consumer fail-soft: error apa pun menjadi hitungan `null`).
- Infra di `main`: `bus.Handler = func(ctx, req any) (any, error)`, `bus.Expect[T](req)` (mengembalikan error yang membungkus `bus.ErrBadRequestType`), `Registry.Register(endpoint, h)`; `errs.Wrap(code, msg, err)`, `errs.ErrInvalid`, `errs.CodeInvalid`; `db.Querier`.
- Prasyarat (HARUS sudah ada; bila belum, berhenti dan laporkan, jangan dibuat di folder feature kecuali disebut di bawah):
  - Migrasi `api_specs` (foundation F, `backend/migrations/project/20261023090000600_create_api_specs.sql`).
  - Konstanta `BUS_PROJECT_APISPEC_COUNT_SPECS` di `backend/lib/constants/bus.go` dan paket `backend/lib/contracts/project/management/contentcount`. Keduanya milik T03. Bila T03 belum merge, foundation F membuatnya persis seperti T03 5.1/5.4 (aturan "yang merge lebih dulu membuat kontrak"). Usecase ini TIDAK membuat atau mengubahnya, dan tidak menduplikasi konstanta.
  - `apispec/base.go` dengan `Register(d app.Deps)` (kerangka F).

## Summary Plan

`countspecs` adalah handler bus Call `project.apispec.count-specs` yang melayani consumer T03: diberi daftar `ProjectIDs`, mengembalikan jumlah API spec per project untuk ditampilkan di daftar/detail project dan dicatat di audit `project.deleted`.

Alur: `bus.Expect[contentcount.Request]` -> tolak bila `len(ProjectIDs) > contentcount.MaxProjectIDs` (error membungkus `errs.ErrInvalid`) -> buang id bukan uuid dan duplikat -> bila tersisa nol id, kembalikan `Counts` kosong tanpa query -> satu query `SELECT project_id, count(*) FROM api_specs WHERE project_id = ANY($1::uuid[]) GROUP BY project_id` -> `Response{Counts}`.

Keputusan di plan ini:

1. Satu feature = satu handler bus (rule 3.4). Tanpa transaksi, tanpa audit, tanpa `access`/`check-access` (pemanggil internal; isolasi dijamin oleh `WHERE project_id = ANY($1)`, yaitu hanya id yang diminta).
2. Tanpa `params.go` (rule 6.9; handler langsung `bus.Expect`). Tanpa entity baru: ini murni agregat baca, tidak ada rule bisnis selain batas jumlah id dan penyaringan uuid.
3. SQL milik feature ini (hanya dipakai di sini), jadi ditulis di `countspecs/repository.go`, bukan di `base/store` (overview 6.1: "count per project batch -> countspecs"). Repository adalah satu-satunya yang menyentuh SQL.
4. Rule bisnis "batas `MaxProjectIDs`" dan "id bukan uuid diabaikan" tinggal di usecase karena ini validasi request bus yang tidak punya entity; usecase tetap hanya orkestrasi tipis (validasi batas, normalisasi id, panggil repo). Tidak dipecah lebih jauh.
5. Validasi uuid memakai `uuid.Parse` (`github.com/google/uuid`, sudah dipakai `base/store`; bila go.mod belum memuatnya, pakai paket yang dipakai store di F). Id dikanonkan ke huruf kecil bentuk `uuid.String()` untuk query dan DEDUP. Kunci `Counts` yang dikembalikan memakai bentuk kanonik dari Postgres (`project_id::text`, huruf kecil). Konsekuensi: id request berhuruf besar menghasilkan kunci huruf kecil. Consumer T03 sudah memegang id dari DB (huruf kecil), jadi aman; dicatat di tes.
6. Project tanpa spec boleh absen dari `Counts` (consumer membaca 0). `Counts` TIDAK PERNAH `nil` (selalu `map[string]int{}` minimal), agar consumer yang menulis ke map atau meng-encode tidak panik/`null`.
7. Pemeriksaan batas dilakukan pada `len(ProjectIDs)` MENTAH (sebelum dedup/filter), sesuai doc comment kontrak. 100 id tepat masih diterima; 101 ditolak walau sebagian tidak valid.
8. `ProjectIDs` nil/kosong -> `Counts` kosong, tanpa query, tanpa error.
9. Error DB dibungkus `fmt.Errorf("countspecs: count by projects: %w", err)`. Handler bus mengembalikan error apa adanya (consumer fail-soft). Error batas: `errs.Wrap(errs.CodeInvalid, "project_ids melebihi batas", errs.ErrInvalid)` (konsisten dengan pola overview 7.2: kontrak hanya stdlib, provider yang membungkus `errs.ErrInvalid`).
10. Log: tidak me-log daftar id. Tidak ada log pada jalur sukses.
11. Tes request bukan tipe yang benar: `bus.Expect` mengembalikan error membungkus `bus.ErrBadRequestType`; handler meneruskannya tanpa diubah. Request dikirim sebagai NILAI (`contentcount.Request{...}`), bukan pointer.

## Plan

Urutan wajib; tulis tes dulu (TDD) per langkah.

### BE-0. Verifikasi prasyarat

- `grep BUS_PROJECT_APISPEC_COUNT_SPECS backend/lib/constants/bus.go` harus ada tepat satu kali; `backend/lib/contracts/project/management/contentcount/contentcount.go` harus memuat `MaxProjectIDs = 100`, `Request{ProjectIDs []string}`, `Response{Counts map[string]int}`.
- Tabel `api_specs` memiliki kolom `project_id uuid` dan indeks `api_specs_project_name_key (project_id, name)` (prefix `project_id` melayani `ANY` + `GROUP BY`; tanpa indeks tambahan).

Bila ada yang hilang: berhenti dan laporkan ke pemilik foundation.

### BE-1. `countspecs/repository.go` (satu-satunya SQL)

```go
package countspecs

type repo struct{ q db.Querier }

func newRepo(q db.Querier) *repo { return &repo{q: q} }

type countRow struct {
    ProjectID string `db:"project_id"`
    Total     int    `db:"total"`
}

// CountByProjects: ids sudah uuid kanonik, unik, tidak kosong (dijamin usecase).
func (r *repo) CountByProjects(ctx context.Context, ids []string) (map[string]int, error) {
    const q = `SELECT project_id::text AS project_id, count(*)::int AS total
               FROM api_specs
               WHERE project_id = ANY($1::uuid[])
               GROUP BY project_id`
    var rows []countRow
    if err := r.q.SelectContext(ctx, &rows, q, pq.Array(ids)); err != nil {
        return nil, err
    }
    out := make(map[string]int, len(rows))
    for _, row := range rows {
        out[row.ProjectID] = row.Total
    }
    return out, nil
}
```

Catatan: sesuaikan cara mengirim array (`pq.Array` atau `pgtype`/`pgx` array) dengan driver yang dipakai `infra/db` di `main`; baca `store.ReplaceEndpoints` dan `infra/db` di F sebelum menulis, jangan menebak. Parameter selalu bind variable (rule SQL), tidak ada string concat.

### BE-2. `countspecs/usecase.go`

```go
package countspecs

type counter interface {
    CountByProjects(ctx context.Context, ids []string) (map[string]int, error)
}

type Usecase struct{ repo counter }

func NewUsecase(repo counter) *Usecase { return &Usecase{repo: repo} }

func (u *Usecase) Execute(ctx context.Context, in contentcount.Request) (contentcount.Response, error) {
    if len(in.ProjectIDs) > contentcount.MaxProjectIDs {
        return contentcount.Response{}, errs.Wrap(errs.CodeInvalid,
            "project_ids melebihi batas", errs.ErrInvalid)
    }
    ids := normalizeIDs(in.ProjectIDs) // parse uuid, buang tidak valid, dedup, urutan pertama-muncul
    if len(ids) == 0 {
        return contentcount.Response{Counts: map[string]int{}}, nil
    }
    counts, err := u.repo.CountByProjects(ctx, ids)
    if err != nil {
        return contentcount.Response{}, fmt.Errorf("countspecs: count by projects: %w", err)
    }
    if counts == nil {
        counts = map[string]int{}
    }
    return contentcount.Response{Counts: counts}, nil
}
```

`normalizeIDs` adalah fungsi privat murni di `usecase.go` (tanpa context, tanpa I/O): `uuid.Parse(s)` gagal -> lewati; hasil `u.String()`; dedup memakai `map[string]struct{}`.

### BE-3. `countspecs/handler.go`

```go
package countspecs

type Handler struct{ uc *Usecase }

func newHandler(uc *Usecase) *Handler { return &Handler{uc: uc} }

func (h *Handler) Handle(ctx context.Context, req any) (any, error) {
    in, err := bus.Expect[contentcount.Request](req)
    if err != nil {
        return nil, err // membungkus bus.ErrBadRequestType
    }
    return h.uc.Execute(ctx, in)
}
```

Mengembalikan `contentcount.Response` sebagai NILAI (konsumen memakai `bus.CallTyped[contentcount.Response]`, yang menegaskan tipe nilai).

### BE-4. `countspecs/init.go`

```go
func Init(d app.Deps) {
    uc := NewUsecase(newRepo(d.DB))
    d.Bus.Register(constants.BUS_PROJECT_APISPEC_COUNT_SPECS, newHandler(uc).Handle)
}
```

Tambahkan satu baris `countspecs.Init(d)` ke `apispec/base.go` (jangan menyentuh baris feature lain). Tidak ada route HTTP.

### BE-5. Verifikasi

`go vet ./...`, `golangci-lint`, `go test ./internal/domain/project/internal/apispec/countspecs/...`, tes integrasi (tag `integration`), archtest `TestT00_AC4_RepoIsClean` (feature tidak meng-import feature lain; hanya `base/*` bila perlu, di sini tidak perlu). `make api-check` tidak terpengaruh (tidak ada perubahan OpenAPI).

### Frontend

Tidak ada (handler internal). Efek terlihat tidak langsung di UI T03 (jumlah spec pada detail project) dan diuji E2E T03.

## Migration

Tidak ada. Tabel `api_specs` dibuat foundation F (`backend/migrations/project/20261023090000600_create_api_specs.sql`). Feature ini hanya membaca.

Catatan performa (rule 8.8): catat `EXPLAIN` query pada 100 project x 100 spec di PR; harapannya index scan/bitmap pada `api_specs_project_name_key` (prefix `project_id`) dengan HashAggregate/GroupAggregate, tanpa seq scan pada tabel besar. Pada tabel kecil planner boleh memilih seq scan; yang diverifikasi adalah query tunggal tanpa N+1.

## Changes

Backend (baru):
- `backend/internal/domain/project/internal/apispec/countspecs/init.go`
- `backend/internal/domain/project/internal/apispec/countspecs/handler.go`
- `backend/internal/domain/project/internal/apispec/countspecs/usecase.go`
- `backend/internal/domain/project/internal/apispec/countspecs/repository.go`
- `backend/internal/domain/project/internal/apispec/countspecs/usecase_test.go`
- `backend/internal/domain/project/internal/apispec/countspecs/handler_test.go`
- `backend/internal/domain/project/internal/apispec/countspecs/countspecs_integration_test.go` (tag `integration`)

Backend (ubah):
- `backend/internal/domain/project/internal/apispec/base.go`: tambah `countspecs.Init(d)`.

Tidak diubah: `base/*`, feature lain, `lib/constants`, `lib/contracts`, `infra/**`, migrasi, OpenAPI, frontend, dokumen (README katalog sudah memuat/akan memuat baris `project.apispec.count-specs` lewat foundation F 7.7).

## Test Scenario

Penamaan: `TestT06_AC7_<Deskripsi>` untuk kriteria penerimaan; lainnya `Test<Fungsi>_<Kasus>`.

### Unit `usecase_test.go` (fake `counter`, tanpa DB)

| Tes | Skenario | Harapan |
|---|---|---|
| `TestExecute_BatchSeveralProjects` | 3 uuid valid; fake mengembalikan `{a:3, b:1}` | `Counts == {a:3, b:1}`; c absen; repo dipanggil sekali dengan ketiga id |
| `TestExecute_InvalidIDsIgnored` | `["<uuid a>", "x", "", "123", "<uuid b>"]` | repo menerima hanya `[a, b]`; tanpa error |
| `TestExecute_AllInvalidSkipsQuery` | `["x", "y"]` | `Counts` kosong non-nil, repo TIDAK dipanggil |
| `TestExecute_EmptyAndNilRequest` | `ProjectIDs` nil dan `[]` | `Counts` non-nil kosong, tanpa error, repo tidak dipanggil |
| `TestExecute_DedupAndCanonicalCase` | `[a, a, A-uppercase]` | repo menerima satu id berhuruf kecil |
| `TestExecute_LimitExactlyMaxAccepted` | 100 id | diterima |
| `TestExecute_OverLimitRejected` | 101 id (sebagian bukan uuid) | `errors.Is(err, errs.ErrInvalid)`, `Code == CodeInvalid`, repo tidak dipanggil |
| `TestExecute_LimitCountsRawLength` | 101 entri yang semuanya duplikat/invalid | tetap ditolak (batas pada panjang mentah) |
| `TestExecute_RepoErrorWrapped` | repo error | error membungkus (`errors.Is` ke error asli), prefix `countspecs: count by projects`; bukan `*errs.Error` |
| `TestExecute_NilCountsFromRepoBecomesEmptyMap` | repo mengembalikan `nil, nil` | `Counts` non-nil kosong |

### Unit `handler_test.go`

- `TestHandle_WrongRequestType`: kirim `"string"`, `contentcount.Request` pointer, dan `nil` -> `errors.Is(err, bus.ErrBadRequestType)`; usecase tidak dipanggil.
- `TestHandle_ReturnsResponseByValue`: hasil bertipe `contentcount.Response` (nilai), dapat di-`bus.CallTyped` lewat registry nyata (`bus.NewRegistry(log)`, `Register`, `CallTyped[contentcount.Response]`).
- `TestHandle_PropagatesInvalidError`: > 100 id lewat registry -> `errors.Is(err, errs.ErrInvalid)`.

### Integration `countspecs_integration_test.go` (tag `integration`, `db.TestDB(t)`, migrasi T03 + T06)

Setup: buat project A, B, C (C tanpa spec), seed `api_specs` langsung lewat SQL/`store` (feature tidak bergantung pada `uploadspecs`): A punya 3 spec, B punya 1 spec; user seed untuk kolom `created_by`.

| Tes | Skenario | Harapan |
|---|---|---|
| `TestT06_AC7_Count_BatchMultipleProjects` | request `[A, B, C]` via bus nyata | `Counts == {A:3, B:1}`; C absen atau 0 (consumer membaca 0) |
| `TestT06_AC7_Count_OnlyRequestedProjects` | request `[A]` saja, padahal B punya spec | `Counts == {A:3}`; B TIDAK ada di map |
| `TestCount_InvalidIDsIgnored` | `[A, "bukan-uuid", "' OR 1=1 --"]` | `{A:3}`, tanpa error SQL (id tidak sampai ke DB) |
| `TestCount_UnknownProjectAbsent` | uuid valid yang tidak ada | `Counts` kosong non-nil |
| `TestCount_SingleQuery` | request 50 project | satu round trip (hitung query lewat wrapper `db.Querier` penghitung, atau `pg_stat_statements` bila tersedia) |
| `TestCount_OverLimit` | 101 id | error `errors.Is(err, errs.ErrInvalid)`; tidak ada query |
| `TestCount_AfterSpecDeleteAndProjectCascade` | hapus satu spec A, lalu hapus project B | A:2; B absen (cascade) |
| `TestCount_NoAuditEmitted` | panggil sukses dengan handler palsu `core.audit.record` | tidak dipanggil |
| `TestRoutesRegistered` (di W, bukan di feature ini) | | bus memuat `project.apispec.count-specs` (Call dengan request valid tidak `ErrNotRegistered`) |

## E2E Scenarios

Tidak ada UI sendiri. Perilaku diverifikasi lewat flow T03 (Playwright, butuh T03 dan T06 merge); hanya dicantumkan sebagai cakupan lintas-module.

Positif:
1. Project dengan 3 spec yang diunggah lewat UI/API: halaman detail project (T03) menampilkan "3 API spec"; daftar project menampilkan hitungan yang sama per kartu (satu request batch, bukan per baris).
2. Project tanpa spec menampilkan 0.
3. Setelah menghapus satu spec, hitungan di detail project turun menjadi 2 setelah muat ulang.

Negatif:
1. Provider belum terdaftar/error (mis. dimatikan di tes): daftar dan detail project T03 tetap tampil, hitungan spec `null`/"-" (consumer fail-soft), tanpa error halaman.
2. Hitungan satu project tidak pernah bocor ke project lain: dua project dengan jumlah spec berbeda menampilkan angka masing-masing.
3. Audit `project.deleted` mencatat jumlah spec saat hapus project (diuji di E2E/integrasi T03; di sini cukup memastikan handler menjawab).

## Request/Response body

Endpoint bus bukan HTTP; kontrak adalah nilai Go (tidak ada JSON di wire, tanpa format error HTTP). Pemanggil memakai `bus.CallTyped`.

### Panggilan sukses

```go
resp, err := bus.CallTyped[contentcount.Response](ctx, d.Bus, constants.BUS_PROJECT_APISPEC_COUNT_SPECS,
    contentcount.Request{ProjectIDs: []string{
        "0b6c2e1a-7d4f-4c1e-9a55-3f2b8c9d1e10", // punya 3 spec
        "9a1f4c0e-2b7d-4e63-8f10-5c9d3a7e1b22", // tanpa spec
        "bukan-uuid",                            // diabaikan
    }})
// err == nil
// resp == contentcount.Response{Counts: map[string]int{
//     "0b6c2e1a-7d4f-4c1e-9a55-3f2b8c9d1e10": 3,
// }}
// project tanpa spec absen; consumer membaca resp.Counts[id] == 0.
```

### Tidak ada id valid / daftar kosong

```go
contentcount.Request{ProjectIDs: nil}
// -> contentcount.Response{Counts: map[string]int{}}, err == nil (tanpa query)
```

### Melebihi batas

```go
contentcount.Request{ProjectIDs: <101 id>}
// -> error: errors.Is(err, errs.ErrInvalid) == true
//    (*errs.Error{Code: CodeInvalid, Message: "project_ids melebihi batas"} membungkus errs.ErrInvalid)
```

### Tipe request salah

```go
d.Bus.Call(ctx, constants.BUS_PROJECT_APISPEC_COUNT_SPECS, "bukan request")
// -> error: errors.Is(err, bus.ErrBadRequestType) == true
```

### Error infra

```go
// DB gagal -> fmt.Errorf("countspecs: count by projects: %w", err); consumer T03 fail-soft (hitungan null).
```

SQL yang dijalankan (satu query, parameter ter-bind):

```sql
SELECT project_id::text AS project_id, count(*)::int AS total
FROM api_specs
WHERE project_id = ANY($1::uuid[])
GROUP BY project_id
```
