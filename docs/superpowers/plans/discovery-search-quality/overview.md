# T11 discovery/knowledge-tools (kualitas pencarian): Overview Plan (FOUNDATION)

Tiket: `docs/tickets/T11-discovery-search-quality.md`
Sumber aturan: `rules.txt` (bagian 2.4, 3.4, 4, 5, 6; menang untuk arsitektur), `docs/tickets/README.md` (konvensi arsitektur, katalog bus, katalog aksi audit), `docs/superpowers/specs/2026-10-20-grimoire-design.md`, plan T02 `docs/superpowers/plans/core-audit-log/*`, plan T08 `docs/superpowers/plans/discovery-knowledge-tools/*`.
Konteks yang dicek: tidak ada `CLAUDE.md`. `.claude/rules/migrations.md` ada, tetapi T11 tidak punya migrasi. Tidak ada knowledge doc khusus module. Tidak ada UI, sehingga tidak ada frontend plan.

Dokumen ini adalah plan FOUNDATION: keputusan bersama, `base/`, kontrak bus, perubahan di luar module, urutan kerja, dan daftar usecase. Plan rinci per usecase ditulis terpisah di `docs/superpowers/plans/discovery-search-quality/usecase-<slug>.md` dan WAJIB mengikuti dokumen ini. Bila keduanya bertentangan, dokumen ini yang berlaku sampai diperbarui.

---

## 0. Prasyarat dan simbol

| Prasyarat | Status (2026-10-21) | Cara cek |
|---|---|---|
| T02 core/audit-log, T05, T07, T08 ter-merge | SUDAH di `main` | `git log main --oneline` |
| `record.ActionMCPSearchKnowledge`, `record.ActionMCPReadFile` | ada di `lib/contracts/core/audit/record/actions.go` | `grep ActionMCPReadFile` |
| Gateway menyalin `Result.Summary` ke `audit_log.result` | ada (`mcpgateway/base/audit.go`, `ToolCallAudit`) | baca file |
| `read_file` mengaudit `target` = path ternormalisasi | ada (`knowledgetools/base/reading.go`, `Target: c.Path`) | baca file |
| `go.yaml.in/yaml/v3` di `go.mod` | ada (v3.0.5) | `grep yaml backend/go.mod` |
| Index `audit_log_project_idx` | ada (migrasi T02) | `grep audit_log_project_idx backend/migrations/core/*` |

| Simbol | Import path |
|---|---|
| `Deps{Bus, DB, Config, ...}` | `grimoire/internal/infra/app` |
| `bus.CallTyped[T]`, `bus.Expect[T]`, `bus.ErrBadRequestType` | `grimoire/internal/infra/bus` |
| `errs.Wrap`, `errs.CodeInvalid`, `errs.ErrInvalid` | `grimoire/lib/common/errs` |
| `record.{Request, Action*, Status*}` | `grimoire/lib/contracts/core/audit/record` |
| `search.Item` | `grimoire/lib/contracts/project/knowledge/search` |
| `toolkit.Result` | `grimoire/lib/contracts/discovery/mcpgateway/toolkit` |

Tidak ada library baru. `go.mod` tidak berubah.

---

## 1. Metadata

| Atribut | Nilai |
|---|---|
| Ticket | T11 (ukuran M) |
| Scope | backend (tiket: UI "Tidak ada") |
| Domain | discovery (provider tambahan di core/auditlog) |
| Module | knowledge-tools (package `knowledgetools`) + core/audit-log (feature baca `scanentries`) |
| Resource root | `backend/internal/domain/discovery/internal/knowledgetools` |
| Kontrak REST / OpenAPI | Tidak ada perubahan. `endpointsChanged = false` |
| Tool MCP | Tidak ada tool baru. Teks dan `structuredContent` `search_knowledge` tidak berubah |
| Migrasi | Tidak ada |

Endpoint yang tersentuh:

| Endpoint | Method | Version | Keterangan |
|---|---|---|---|
| `core.audit.scan-entries` | bus Call (baru) | v1 (kontrak `lib/contracts/core/audit/scanentries`) | provider core/auditlog `scanentries`; consumer knowledgetools `searchquality` |
| `discovery.knowledgetools.search-quality-report` | bus Call (baru) | v1 (kontrak `lib/contracts/discovery/knowledgetools/searchquality`) | provider knowledgetools `searchquality`; consumer `cmd/cli` |
| `grimoire search-quality report` | CLI (baru) | - | perintah di `backend/cmd/cli` |
| `/mcp` `tools/call` `search_knowledge` | POST (tidak berubah) | MCP JSON-RPC 2.0 | hanya `audit_log.result` mendapat `result_paths` |
| `make test-search-eval`, `make search-eval-baseline` | make target (baru) | - | runner golden set |

---

## 2. Summary Plan

T11 menambah dua alat ukur tanpa mengubah algoritma pencarian:

1. **Sinyal produksi.** `SearchHits.Summary()` (entity di `knowledgetools/base`) menambah `result_paths`: maksimal 10 path teratas, urut sama dengan keluaran, tiap path dipotong ke batas `target` audit (1024 byte). Laporan metrik (zero-result, follow-up, rank dibuka, reformulasi) dihitung dari `audit_log` oleh feature baru `knowledgetools/searchquality`. Data audit dibaca lewat endpoint bus baca-saja baru `core.audit.scan-entries` milik core/auditlog, lalu dikonsumsi CLI `grimoire search-quality report` lewat bus `discovery.knowledgetools.search-quality-report`.
2. **Tolok ukur offline.** Golden set (korpus fixture + `golden.yaml`) dijalankan oleh tes integrasi terhadap Postgres sungguhan lewat tool MCP `search_knowledge`. Logika metrik recall@k/MRR dan gerbang baseline ada di paket netral `backend/lib/util/rankeval`.

Rule bisnis (pembentukan `result_paths`, parsing event audit, aturan follow-up/reformulasi/rank) ada di `knowledgetools/base` sebagai fungsi murni. Usecase hanya orkestrasi (paging bus, umpan ke akumulator). SQL hanya di `auditlog/scanentries/repository.go` dan hanya `SELECT`.

---

## 3. Keputusan bersama (mengikat semua usecase)

| ID | Keputusan | Alasan |
|---|---|---|
| D1 (K2) | Data audit dibaca lewat bus `core.audit.scan-entries` (provider core/auditlog), BUKAN repository discovery yang `SELECT` ke `audit_log` dan bukan SQL di `cmd/cli` | `rules.txt` 2.4 (module lain dilarang membaca tabel module lain), 4.7 (`cmd/**` hanya wiring). Ini menyimpang dari kalimat tiket "Tidak ada endpoint bus baru"; dicatat di tiket dan katalog (langkah F7). K2 tiket sendiri menyebut fallback ini |
| D2 | Dua endpoint bus baru, keduanya Call sinkron dan read-only: `core.audit.scan-entries`, `discovery.knowledgetools.search-quality-report` | CLI butuh hasil; pola sama dengan `core.auth.create-admin` |
| D3 (K1) | `MaxResultPaths = 10` | Keputusan tiket K1 |
| D4 | Path di `result_paths` dipotong `TruncateUTF8(path, record.MaxTargetBytes)` (1024 byte) | Sama dengan batas `target` audit `read_file`, sehingga pencocokan follow-up persis |
| D5 | Anggaran `result_paths` = 6144 byte JSON-encoded. Bila terlampaui, path berikutnya dibuang dan `result_paths_truncated: true` ditambahkan | `Sanitize` T02 membatasi `result` ke 8192 byte. Bila lewat, SELURUH result diganti `{_truncated, _original_bytes, _keys}` sehingga `count`/`total` hilang diam-diam. 10 path x 1024 byte > 8192, jadi entity wajib membatasi. Sisa ~2 KB cukup untuk `count`, `total`, `bytes`, `original_bytes`, `truncated`, `duration_ms`, `error_code` |
| D6 | Batas audit diangkat ke kontrak: `record.MaxTargetBytes = 1024`, `record.MaxResultJSONBytes = 8192` (file baru `lib/contracts/core/audit/record/limits.go`). `auditlog/base` merujuk konstanta ini | Consumer tidak menyalin angka; module tidak boleh meng-import `auditlog/base` (4.2) |
| D7 (K3) | Jendela default `DefaultQualityWindow = 120s`, rentang flag `--window` 1s..1h (`MaxQualityWindow`). Batas inklusif: `0 < Δ <= window` | Tiket K3, "dalam 120 detik" |
| D8 | Hanya entri `status = ok` yang dihitung, baik search maupun read_file. Entri dengan `actor_id` kosong dilewati | Pencarian gagal (argumen invalid) bukan sinyal kualitas |
| D9 | Query dibandingkan setelah `NormalizeQuery` (`strings.Fields` join satu spasi, lalu `ToLower`) | FTS `'simple'` case-insensitive; beda kapital/spasi bukan reformulasi |
| D10 | Golden set runner berupa tes integrasi di `backend/cmd/internal/server/searcheval_integration_test.go`; logika metrik di `backend/lib/util/rankeval` | `internal/` Go melarang `cmd` meng-import knowledgetools, dan discovery dilarang merakit domain project (4.1). Tes integrasi server sudah merakit seluruh domain dengan Postgres tes |
| D11 (K5) | Gerbang eval: gagal hanya bila recall@1, recall@5, atau MRR keseluruhan turun lebih dari toleransi (default 0.02, env `SEARCH_EVAL_TOLERANCE`) dari `baseline.json`. Per-kind hanya dicetak. `SEARCH_EVAL_UPDATE_BASELINE=1` menulis ulang baseline | Keputusan tiket K5 |
| D12 | Batas aman laporan: `MaxQualityScanEntries = 2_000_000` entri per laporan. Lewat batas -> error `INVALID_ARGUMENT` "rentang terlalu besar" | Mencegah CLI berjalan tanpa batas |
| D13 | Akumulator streaming: event diumpankan urut `(occurred_at, id)` ASC, antrean search terbuka per aktor di-evict saat lewat jendela | Tidak menahan 90 hari data di memori |
| D14 | Tidak ada perubahan skema, retensi, atau sifat append-only `audit_log`. `arch_test` auditlog AC7 tetap hijau | Non-goal tiket |

---

## 4. Kontrak bus dan konstanta (dikerjakan di FOUNDATION)

### 4.1 Konstanta (`backend/lib/constants/bus.go`, tambah)

```go
// core.audit.scan-entries: baca entri audit satu project, urut (occurred_at, id) ASC (T11).
const BUS_CORE_AUDIT_SCAN_ENTRIES = "core.audit.scan-entries"

// discovery.knowledgetools.search-quality-report: laporan metrik kualitas pencarian (T11).
const BUS_DISCOVERY_KNOWLEDGETOOLS_SEARCH_QUALITY_REPORT = "discovery.knowledgetools.search-quality-report"
```

### 4.2 `backend/lib/contracts/core/audit/scanentries/scanentries.go` (baru)

```go
// Package scanentries adalah kontrak bus core.audit.scan-entries (provider T02 core/auditlog,
// feature scanentries; ditambah T11). Baca-saja, selalu dibatasi ProjectID (rules 6.10).
//
// Request dikirim sebagai nilai scanentries.Request; tipe lain -> error membungkus
// bus.ErrBadRequestType. Request invalid -> error membungkus errs.ErrInvalid (pesan tanpa nilai input).
// Paging keyset ASC: kirim Response.Next sebagai Request.After sampai Next == nil.
package scanentries

import "time"

const (
	DefaultLimit = 500
	MaxLimit     = 1000
	MaxActions   = 10
)

type Request struct {
	ProjectID string    // wajib, uuid
	Actions   []string  // wajib, 1..MaxActions, tiap item format aksi audit valid
	From      time.Time // wajib, inklusif
	To        time.Time // nol -> now; eksklusif; harus > From
	After     *Cursor   // nil = dari awal
	Limit     int       // 0 -> DefaultLimit; > MaxLimit -> invalid
}

type Cursor struct {
	OccurredAt time.Time
	ID         string // uuid
}

type Entry struct {
	ID         string
	OccurredAt time.Time
	ActorType  string
	ActorID    string // "" bila NULL
	Action     string
	Target     string // "" bila NULL
	Status     string
	Args       map[string]any // jsonb; tidak pernah nil (kosong -> map{})
	Result     map[string]any // jsonb; tidak pernah nil
}

type Response struct {
	Entries []Entry // tidak pernah nil
	Next    *Cursor // nil bila tidak ada halaman berikutnya
}
```

Keputusan kontrak: `Limit > MaxLimit` ditolak (`ErrInvalid`), bukan di-clamp, agar consumer tidak salah mengira halaman habis. Kolom `actor_label`, `project_label`, `ip`, `user_agent`, `trace_id` sengaja tidak dikembalikan (tidak dibutuhkan; minimisasi data).

SQL acuan (dipakai usecase `scanentries`, parameterized, pola `buildQuery` listentries):

```sql
SELECT id::text, occurred_at, actor_type, actor_id::text, action, target, status, args, result
FROM audit_log
WHERE project_id = $1::uuid
  AND action = ANY($2::text[])
  AND occurred_at >= $3 AND occurred_at < $4
  [AND (occurred_at, id) > ($5::timestamptz, $6::uuid)]   -- hanya bila After != nil
ORDER BY occurred_at ASC, id ASC
LIMIT $n            -- Limit + 1 untuk mendeteksi halaman berikutnya
```

### 4.3 `backend/lib/contracts/discovery/knowledgetools/searchquality/searchquality.go` (baru)

```go
// Package searchquality adalah kontrak bus discovery.knowledgetools.search-quality-report
// (provider T11 knowledgetools/searchquality, consumer cmd/cli). Baca-saja.
// Request invalid -> error membungkus errs.ErrInvalid. Tipe salah -> bus.ErrBadRequestType.
package searchquality

import "time"

type Request struct {
	ProjectID string        // wajib, uuid
	Since     time.Time     // wajib, inklusif
	Until     time.Time     // nol -> now; eksklusif; harus > Since
	Window    time.Duration // 0 -> 120s; 1s..1h
}

type Response struct {
	Report Report `json:"report"`
}

type Report struct {
	ProjectID      string       `json:"project_id"`
	Since          time.Time    `json:"since"`
	Until          time.Time    `json:"until"`
	WindowSeconds  int          `json:"window_seconds"`
	Searches       int          `json:"searches"`        // search status ok dengan At di [Since, Until)
	ZeroResult     Metric       `json:"zero_result"`     // denominator = search dengan total terbaca
	FollowUp       Metric       `json:"follow_up"`       // denominator = search dengan result_paths
	Reformulation  Metric       `json:"reformulation"`   // denominator = Searches
	RankOpened     []RankBucket `json:"rank_opened"`     // "1".."10","not_in_results", urutan tetap, selalu 11 bucket
	LegacySearches int          `json:"legacy_searches"` // search ok tanpa result_paths (entri sebelum T11)
}

type Metric struct {
	Count       int     `json:"count"`
	Denominator int     `json:"denominator"`
	Rate        float64 `json:"rate"` // Count/Denominator, 0 bila Denominator 0
}

type RankBucket struct {
	Label string `json:"label"`
	Count int    `json:"count"`
}

const NotInResults = "not_in_results"
```

### 4.4 Batas audit bersama: `backend/lib/contracts/core/audit/record/limits.go` (baru)

```go
package record

// Batas yang diterapkan provider core.audit.record. Consumer memakai konstanta ini, bukan menyalin angka.
const (
	MaxTargetBytes     = 1024 // Target dipotong ke batas ini (batas UTF-8)
	MaxResultJSONBytes = 8192 // Args dan Result masing-masing; lewat batas -> diganti {_truncated,...}
)
```

`auditlog/base/entry.go`: `MaxTargetBytes = record.MaxTargetBytes`. `auditlog/base/sanitize.go`: `MaxJSONBytes = record.MaxResultJSONBytes`. Nilai tidak berubah; tes `entry_test.go`/`sanitize_test.go` existing harus tetap hijau tanpa diubah.

---

## 5. Entity / base bersama (dikerjakan di FOUNDATION, usecase dilarang mengubah `base/`)

### 5.1 `knowledgetools/base/searching.go` (ubah)

Tambah:

```go
const (
	MaxResultPaths         = 10   // K1
	ResultPathsBudgetBytes = 6144 // anggaran JSON result_paths; sisa ~2 KB untuk kunci audit lain
)

// ResultPaths mengembalikan path hasil teratas untuk audit, urut sama dengan Items.
// Maks MaxResultPaths; tiap path TruncateUTF8(path, record.MaxTargetBytes);
// berhenti bila total panjang JSON (sum len(json.Marshal(p)) + koma) melewati ResultPathsBudgetBytes.
// Tidak pernah nil. Bool kedua true bila ada path dibuang karena anggaran (bukan karena batas 10).
func (h SearchHits) ResultPaths() ([]string, bool)
```

Catatan penghitungan anggaran: pakai `json.Marshal(p)` per string, karena Go meng-escape `<`, `>`, `&` menjadi escape unicode 6 karakter (backslash, `u`, `003c`) sehingga panjang encoded bisa sampai 6x panjang mentah. Hitung `2` (kurung siku) + `sum(len(enc_i))` + `(n-1)` koma.

`Summary()` menjadi:

```go
// Summary adalah metadata audit: count, total, dan result_paths (path hasil teratas, maks 10).
// Path adalah metadata (sudah tercatat sebagai target audit read_file), bukan isi file. T11 membalik
// keputusan T08 ("tanpa path hasil") karena metrik follow-up dan rank dibuka butuh urutan hasil.
// Snippet, skor, isi file, dan query TIDAK masuk (query sudah ada di args.query).
// result_paths_truncated hanya ada (true) bila path dibuang karena anggaran ukuran audit.
func (h SearchHits) Summary() map[string]any
```

`Text()`, `Structured()`, `Result()` tidak berubah. Helper `TruncateUTF8(s string, max int) string` ditambah lokal di `knowledgetools/base/format.go` (salinan perilaku `auditlog/base.TruncateUTF8`; module tidak boleh meng-import auditlog).

### 5.2 `knowledgetools/base/quality.go` (baru, murni)

```go
const (
	DefaultQualityWindow  = 120 * time.Second
	MinQualityWindow      = time.Second
	MaxQualityWindow      = time.Hour
	MaxQualityScanEntries = 2_000_000
)

type AuditEvent struct {
	ID, ActorID, Action, Target, Status string
	At       time.Time
	Query    string   // args.query (search saja)
	Total    int
	HasTotal bool     // result.total terbaca sebagai angka
	Paths    []string // result.result_paths
	HasPaths bool     // kunci result_paths ada dan berbentuk array string
}

// EventFromAudit membaca entri audit mentah. Tipe tak terduga dianggap tidak ada; tidak panic.
// total: float64 (dari jsonb), json.Number, int. result_paths: []any berisi string (item non-string -> HasPaths=false).
func EventFromAudit(id, action, actorID, target, status string, at time.Time, args, result map[string]any) AuditEvent

// NormalizeQuery: strings.Join(strings.Fields(q), " ") lalu strings.ToLower.
func NormalizeQuery(q string) string

type QualityReport = searchquality.Report

type QualityAccumulator struct{ /* since, until, window, per-aktor antrean search terbuka, counter */ }

func NewQualityAccumulator(since, until time.Time, window time.Duration) *QualityAccumulator
// Add WAJIB dipanggil urut (At, ID) ASC. Event di luar [since, until+window) diabaikan.
func (a *QualityAccumulator) Add(e AuditEvent)
// Report menutup semua search terbuka lalu mengembalikan laporan. Rate dihitung di sini.
func (a *QualityAccumulator) Report(projectID string) QualityReport
```

Aturan akumulator (rule bisnis, diuji di `quality_test.go`):

1. Event dengan `Status != ok` atau `ActorID == ""` diabaikan seluruhnya (termasuk sebagai "search berikutnya").
2. Search dengan `At` di `[since, until)` = **dievaluasi**: `Searches++`. Search di `[until, until+window)` hanya berperan sebagai "search berikutnya" untuk reformulasi; read_file di rentang itu tetap dipakai untuk follow-up.
3. **Zero-result:** denominator = search dievaluasi dengan `HasTotal`; count = `Total == 0`.
4. **Legacy:** search dievaluasi tanpa `HasPaths` -> `LegacySearches++`, tidak ikut denominator follow-up dan rank, tetap ikut zero-result dan reformulasi.
5. **Follow-up dan rank:** untuk search `s` (dievaluasi, `HasPaths`) dan read_file `r` aktor sama dengan `0 < r.At - s.At <= window`: read pertama yang `Target ∈ s.Paths` menetapkan follow-up dengan `rank` = indeks 1-based pertama. Bila ada read dalam jendela tetapi tidak satu pun cocok, saat jendela ditutup -> bucket `not_in_results` (tidak dihitung follow-up). Tanpa read sama sekali -> tidak masuk bucket mana pun. Denominator follow-up = search dievaluasi dengan `HasPaths`.
6. **Reformulasi:** search berikutnya yang langsung menyusul (search ok pertama aktor sama setelah `s`) dengan `0 < Δ <= window` dan `NormalizeQuery` berbeda -> `s` dihitung reformulasi. Query sama -> tidak. Hanya search langsung berikutnya yang dinilai. Δ = 0 (timestamp identik) tidak dihitung.
7. Satu search bisa dihitung follow-up walaupun ada search lain di antaranya (definisi tiket; dicatat sebagai perkiraan di `docs/search-quality.md`).
8. Eviction: setiap `Add`, search terbuka dengan `e.At - s.At > window` ditutup (finalisasi bucket `not_in_results` bila perlu) dan dibuang.
9. `RankOpened` selalu 11 bucket urutan `"1".."10","not_in_results"`, termasuk yang 0.

---

## 6. Plan (langkah berurutan)

### FOUNDATION (satu agent, sebelum usecase)

- **F1.** `backend/lib/constants/bus.go`: tambah dua konstanta (4.1).
- **F2.** `backend/lib/contracts/core/audit/record/limits.go` (baru, 4.4). Ubah `auditlog/base/entry.go` (`MaxTargetBytes = record.MaxTargetBytes`) dan `auditlog/base/sanitize.go` (`MaxJSONBytes = record.MaxResultJSONBytes`).
- **F3.** `backend/lib/contracts/core/audit/scanentries/scanentries.go` (baru, 4.2).
- **F4.** `backend/lib/contracts/discovery/knowledgetools/searchquality/searchquality.go` (baru, 4.3).
- **F5.** `knowledgetools/base/format.go`: tambah `TruncateUTF8`. `knowledgetools/base/searching.go`: `MaxResultPaths`, `ResultPathsBudgetBytes`, `ResultPaths()`, `Summary()` baru + komentar (5.1). Tes di `format_test.go`, `searching_test.go`.
- **F6.** `knowledgetools/base/quality.go` (baru, 5.2) + `quality_test.go`.
- **F7.** Dokumen kontrak: `docs/tickets/README.md` (dua baris katalog bus + paragraf kontrak, lihat 8), `docs/tickets/T11-discovery-search-quality.md` bagian "Kontrak bus" diberi catatan keputusan D1/K2.
- **F8.** Verifikasi: `cd backend && go build ./... && go vet ./... && go test ./internal/domain/discovery/... ./internal/domain/core/... ./lib/... ./internal/archtest/...`.

### USECASE (paralel setelah foundation; `searchquality` runLate)

- **U1 `searchknowledge`**: tes + dokumen `docs/mcp-gateway.md` (bagian Audit).
- **U2 `scanentries`**: provider bus di core/auditlog + `scanentries.Init(d)` di `auditlog/base.go`.
- **U3 `searcheval`**: `lib/util/rankeval`, fixture, tes integrasi, makefile, `docs/search-quality.md`.
- **U4 `searchquality`** (runLate, butuh U2 untuk integrasi): provider bus laporan + CLI.

### WIRING (setelah semua usecase)

- **W1.** `knowledgetools/base.go`: panggil `searchquality.Init(d)` di `Register` (registrasi bus, bukan tool), perbarui komentar package/fungsi ("mendaftarkan tiga tool read-only dan satu endpoint bus laporan kualitas pencarian"). Signature `Register(d app.Deps) []toolkit.Tool` tetap.
- **W2.** Pastikan `knowledgetools/base_test.go` (golden `tools_list.golden.json`) tetap identik: tidak ada tool baru.
- **W3.** Verifikasi penuh: `make lint`, `go test ./...`, `go test -tags integration ./...` (termasuk `TestSearchEval`), cek registrasi bus ganda tidak terjadi (`TestRunWires...` di `cmd/internal/server`).

---

## 7. Migration

Tidak ada. Tidak ada perubahan skema, index, retensi, atau sifat append-only `audit_log`. Index existing `audit_log_project_idx` (project_id, occurred_at) cukup untuk query scan.

---

## 8. Changes

| File / folder | Perubahan | Tahap |
|---|---|---|
| `backend/lib/constants/bus.go` | + `BUS_CORE_AUDIT_SCAN_ENTRIES`, `BUS_DISCOVERY_KNOWLEDGETOOLS_SEARCH_QUALITY_REPORT` | F1 |
| `backend/lib/contracts/core/audit/record/limits.go` | baru: `MaxTargetBytes`, `MaxResultJSONBytes` | F2 |
| `backend/internal/domain/core/internal/auditlog/base/entry.go` | `MaxTargetBytes` merujuk kontrak | F2 |
| `backend/internal/domain/core/internal/auditlog/base/sanitize.go` | `MaxJSONBytes` merujuk kontrak | F2 |
| `backend/lib/contracts/core/audit/scanentries/scanentries.go` | baru: kontrak scan | F3 |
| `backend/lib/contracts/discovery/knowledgetools/searchquality/searchquality.go` | baru: kontrak laporan | F4 |
| `backend/internal/domain/discovery/internal/knowledgetools/base/format.go` (+ `_test`) | + `TruncateUTF8` | F5 |
| `backend/internal/domain/discovery/internal/knowledgetools/base/searching.go` (+ `_test`) | + `ResultPaths`, `Summary` baru, komentar | F5 |
| `backend/internal/domain/discovery/internal/knowledgetools/base/quality.go` (+ `_test`) | baru: event, normalisasi query, akumulator | F6 |
| `docs/tickets/README.md` | + 2 baris katalog, paragraf kontrak T11 | F7 |
| `docs/tickets/T11-discovery-search-quality.md` | catatan keputusan K2 di "Kontrak bus" | F7 |
| `backend/internal/domain/discovery/internal/knowledgetools/searchknowledge/` | tes saja (+ `testdata/` golden) | U1 |
| `backend/cmd/internal/server/searchquality_audit_integration_test.go` | baru | U1 |
| `docs/mcp-gateway.md` | bagian Audit: `result_paths` | U1 |
| `backend/internal/domain/core/internal/auditlog/scanentries/` | baru: init, handler, params, usecase, repository + tes | U2 |
| `backend/internal/domain/core/internal/auditlog/base.go` | + `scanentries.Init(d)` (bus saja, di luar route super_admin) | U2 |
| `backend/lib/util/rankeval/` | baru: golden, metrik, baseline, compare + tes | U3 |
| `backend/testdata/searcheval/{corpus/**.md, golden.yaml, baseline.json}` | baru | U3 |
| `backend/cmd/internal/server/searcheval_integration_test.go` | baru: `TestSearchEval` | U3 |
| `makefile` | + `test-search-eval`, `search-eval-baseline` | U3 |
| `docs/search-quality.md` | baru | U3 |
| `backend/internal/domain/discovery/internal/knowledgetools/searchquality/` | baru: init, handler, params, usecase, repository + tes | U4 |
| `backend/cmd/cli/searchquality.go`, `searchquality_test.go`, `cli.go` | subperintah + usage | U4 |
| `backend/cmd/internal/server/searchquality_report_integration_test.go` | baru | U4 |
| `backend/internal/domain/discovery/internal/knowledgetools/base.go` | + `searchquality.Init(d)`, komentar | W1 |

allowedPaths antar usecase tidak tumpang tindih. Helper tes integrasi bersama (mis. varian `setupMCP` dengan `MCP_RATE_PER_MINUTE` tinggi) hanya boleh ditambah di file baru milik usecase masing-masing; `mcpgateway_integration_test.go` dan `knowledgetools_integration_test.go` tidak diubah. Nama helper diberi prefiks per file (`sqa*`, `sqr*`, `se*`) agar tidak bentrok dalam package `server`.

Teks katalog untuk `docs/tickets/README.md`:

```
| `core.audit.scan-entries` | Call (baca-saja) | T02 (feature `scanentries`, ditambah T11) | T11 |
| `discovery.knowledgetools.search-quality-report` | Call (baca-saja) | T11 | `cmd/cli` |
```

Paragraf: "Kontrak `core.audit.scan-entries`: `lib/contracts/core/audit/scanentries` (konstanta `BUS_CORE_AUDIT_SCAN_ENTRIES`); selalu dibatasi `ProjectID`, filter `Actions` dan `[From, To)`, keyset ASC `(occurred_at, id)`, `Limit` maks 1000, hanya `SELECT`. Kontrak `discovery.knowledgetools.search-quality-report`: `lib/contracts/discovery/knowledgetools/searchquality` (konstanta `BUS_DISCOVERY_KNOWLEDGETOOLS_SEARCH_QUALITY_REPORT`). Batas audit bersama `record.MaxTargetBytes`/`record.MaxResultJSONBytes` di `lib/contracts/core/audit/record/limits.go`. T11 sengaja menambah bus alih-alih membaca `audit_log` dari discovery/CLI (rules 2.4, 4.7)."

---

## 9. Daftar usecase

| Slug | Ringkas | Bergantung | runLate | allowedPaths |
|---|---|---|---|---|
| `searchknowledge` | Tes (unit + integrasi) bahwa audit `search_knowledge` membawa `result_paths`; snapshot Text/Structured; dokumen Audit | foundation | tidak | `backend/internal/domain/discovery/internal/knowledgetools/searchknowledge/`, `backend/cmd/internal/server/searchquality_audit_integration_test.go`, `docs/mcp-gateway.md` |
| `scanentries` | Provider bus `core.audit.scan-entries` di core/auditlog | foundation | tidak | `backend/internal/domain/core/internal/auditlog/scanentries/`, `backend/internal/domain/core/internal/auditlog/base.go` |
| `searcheval` | `lib/util/rankeval`, fixture golden, `TestSearchEval`, make target, `docs/search-quality.md` | foundation | tidak | `backend/lib/util/rankeval/`, `backend/testdata/searcheval/`, `backend/cmd/internal/server/searcheval_integration_test.go`, `makefile`, `docs/search-quality.md` |
| `searchquality` | Provider bus laporan + CLI `grimoire search-quality report` | foundation, `scanentries` (integrasi) | ya | `backend/internal/domain/discovery/internal/knowledgetools/searchquality/`, `backend/cmd/cli/searchquality.go`, `backend/cmd/cli/searchquality_test.go`, `backend/cmd/cli/cli.go`, `backend/cmd/internal/server/searchquality_report_integration_test.go` |

Wiring (`knowledgetools/base.go`) dikerjakan tahap W, bukan usecase.

Ringkasan per usecase (rincian di plan usecase masing-masing):

- **searchknowledge**: tanpa ubah kode feature. Tes AC1 (3 hasil -> 3 path urut sama dengan teks), AC2 (15 hasil -> 10 path, count 15, total sebenarnya), AC3 (tak ada `snippet`/`score`/`content`/`query` di Summary), AC4 (`result_paths: []` bukan null, `total: 0`), AC5 (snapshot `testdata/*.golden` Text/Structured; boleh dibuat setelah foundation karena F5 tidak menyentuh Text/Structured), AC6 (bus `core.audit.record` gagal -> respons agent sama). Path panjang/multibyte dan anggaran 6144 byte memunculkan `result_paths_truncated` tanpa `_truncated` dari Sanitize.
- **scanentries**: handler `bus.Expect[scanentries.Request]` -> params (validasi; `auditlog/base.ValidAction`, uuid kanonik) -> usecase (`repo.Scan(limit+1)`, bentuk `Next`) -> repository (SQL 4.2, unmarshal jsonb ke map, NULL -> ""). Tanpa route HTTP.
- **searcheval**: recall@k per query = `|expect ∩ top-k| / |expect|`, dirata-rata; MRR = `1/rank` expect pertama di top-10, 0 bila tidak ada. `golden.yaml` 30-50 pasang, >= 3 per kind (`exact-name|keyword|synonym|typo|phrase`), ditulis dari kebutuhan nyata sebelum menjalankan pencarian. Baseline dihasilkan dari implementasi saat ini.
- **searchquality**: params (ProjectID uuid, Since wajib, Until default now dan > Since, Window default 120s rentang 1s..1h) -> usecase (loop scan `Actions=[mcp.search_knowledge, mcp.read_file]`, `From=Since`, `To=Until+Window`, `Limit=1000` sampai `Next == nil`; > `MaxQualityScanEntries` -> error) -> `base.EventFromAudit` -> `QualityAccumulator.Add` -> `Report`. Repository = adapter bus. CLI memanggil bus lewat `server.WithDomains` (pola `callCreateAdmin`), stdout bersih, log ke stderr.

---

## 10. Test Scenario

### Unit (foundation)

`knowledgetools/base/searching_test.go`:
- 3 item -> `result_paths` = 3 path urut sama dengan Items, `count=3`, `total` sama dengan input; tanpa kunci `result_paths_truncated`.
- 15 item, Total 40 -> 10 path pertama, `count=15`, `total=40`.
- 0 item -> `result_paths` bertipe `[]string{}` (json `[]`, bukan `null`), `total=0`.
- Path 1500 byte multibyte (mis. "é" berulang) -> dipotong <= 1024 byte, `utf8.ValidString` true.
- 10 path @1024 byte -> total JSON <= 6144, sebagian dibuang, `result_paths_truncated: true`; assert `len(json.Marshal(Summary())) + 512 < record.MaxResultJSONBytes` (512 = ruang kunci gateway `bytes`, `original_bytes`, `truncated`, `duration_ms`, `error_code`). Tes ini tidak meng-import `auditlog/base`.
- Path berisi `<>&` dihitung dengan panjang ter-escape.
- Summary tidak punya kunci `snippet`, `score`, `content`, `query`, `items`.
- `Text()` dan `Structured()` sama dengan ekspektasi literal sebelum perubahan (regresi).

`knowledgetools/base/format_test.go`: `TruncateUTF8` (max <= 0, ASCII, multibyte di batas, string pendek utuh).

`knowledgetools/base/quality_test.go` (tabel, waktu sintetis, satu project):
- `EventFromAudit`: total `float64(3)`, `json.Number("3")`, `int(3)` -> 3; total string -> HasTotal false; `result_paths` `[]any{"a","b"}` -> HasPaths; `[]any{"a", 1}` -> HasPaths false; kunci tidak ada -> HasPaths false; args nil -> tidak panic.
- `NormalizeQuery("  Login  OAuth ")` == `NormalizeQuery("login oauth")`.
- Follow-up rank 2: search A (paths [p1,p2,p3]) t0, read p2 t0+30s -> FollowUp {1,1,1.0}, bucket "2"=1.
- read aktor lain -> tidak dihitung; read t0+180s -> tidak dihitung; read tepat t0+120s -> dihitung; read t0 (Δ=0) -> tidak.
- read p9 (tidak di hasil) dalam jendela -> bucket `not_in_results`=1, FollowUp count 0.
- read p9 lalu p1 dalam jendela -> follow-up rank 1, `not_in_results` 0.
- Reformulasi: query "login" t0, "oauth login" t0+60s -> Reformulation count 1 (denominator 2). Query sama diulang -> 0. Beda hanya kapital -> 0. Search berikutnya t0+130s -> 0.
- Hanya search langsung berikutnya: A, B (sama dengan A), C (beda) dalam jendela -> A tidak reformulasi, B reformulasi.
- Legacy: search tanpa result_paths total 0 -> ZeroResult count 1, LegacySearches 1, FollowUp denominator 0, rate 0.
- Status error diabaikan (search maupun read).
- Batas rentang: search pada `until` tidak dihitung tetapi memicu reformulasi untuk search pada `until-10s`; read pada `until+30s` menjadi follow-up untuk search `until-10s`.
- RankOpened selalu 11 bucket urut tetap.
- Eviction: 10.000 search satu aktor dengan jarak 200s -> antrean internal tetap <= 1 (assert lewat helper tes package-internal).

Kontrak: `lib/contracts/core/audit/record/limits_test.go` (opsional) assert nilai 1024/8192. Tes existing `auditlog/base/*_test.go` tetap hijau.

### Unit dan integrasi (usecase) - ringkasan, rincian di plan usecase

- `scanentries`: params (tiap field invalid -> `ErrInvalid`, pesan tanpa nilai input), usecase dengan repo palsu (paging, `Next == nil` di halaman terakhir), integrasi repository (`//go:build integration`, `db.TestDB`): hanya project diminta, hanya actions diminta, From inklusif/To eksklusif, 3 halaman tanpa duplikat/terlewat pada `occurred_at` identik, args/result kembali map; tipe body salah -> `bus.ErrBadRequestType`.
- `searchquality`: unit usecase (repo palsu: paging, batas aman, error bus dibungkus), unit CLI (parse `--since`, `--json` lolos `json.Valid`, flag wajib -> exit 2), integrasi AC7-AC10 dengan INSERT `audit_log` sintetis.
- `searchknowledge`: AC1-AC6.
- `searcheval`: unit `rankeval` (AC12: turun 0.03 gagal, naik lulus, turun 0.01 lulus; validasi golden), integrasi `TestSearchEval` (AC11, AC13).

### Arsitektur

- `internal/archtest` hijau: knowledgetools hanya meng-import `lib/constants`, `lib/contracts/...`, `infra/bus`, `infra/app`; `lib/util/rankeval` tidak meng-import `internal/**`/`cmd/**`.
- `auditlog/arch_test.go` (AC7 T02) hijau: tidak ada `UPDATE`/`DELETE`/`TRUNCATE audit_log` baru.

---

## 11. E2E Scenarios

Tidak ada UI, jadi E2E = skenario lintas lapis di tes integrasi `backend/cmd/internal/server` (server sungguhan + Postgres tes) dan CLI.

Positif:
1. Agent memanggil `search_knowledge` (3 hasil) -> `audit_log` baris `mcp.search_knowledge` punya `result.result_paths` 3 path urut, `count=3`, `total=3`; teks hasil ke agent identik dengan sebelum T11.
2. Agent mencari lalu `read_file` hasil ke-2 dalam 30 detik -> `grimoire search-quality report --project <id> --since 7d` mencetak Follow-up 1/1 (100.0%), Rank dibuka "2: 1".
3. Agent mencari "login", 60 detik kemudian "oauth login" -> Reformulasi 1/2.
4. `--json` -> stdout JSON valid sesuai 12.3, stderr boleh berisi log.
5. `make test-search-eval` mencetak recall@1, recall@5, MRR, rincian per kind, pasangan gagal; lulus terhadap `baseline.json`.

Negatif:
1. `read_file` oleh key lain, project lain, atau 3 menit kemudian -> follow-up 0.
2. Query sama persis diulang -> reformulasi 0.
3. Entri audit lama tanpa `result_paths` -> laporan tidak error; masuk Zero-result, tidak masuk Follow-up/Rank, tercatat di "entri lama dilewati".
4. `core.audit.record` gagal -> respons `search_knowledge` ke agent sama persis (fail-open).
5. CLI tanpa `--project` atau `--since`, `--since` tak terbaca, `--window 2h` -> exit 2, usage ke stderr, stdout kosong.
6. `--project` bukan uuid -> bus mengembalikan `INVALID_ARGUMENT`, CLI exit 1 dengan pesan aman.
7. Rentang yang memuat > 2.000.000 entri -> error "rentang terlalu besar", exit 1.
8. Consumer memanggil `core.audit.scan-entries` dengan pointer/tipe lain -> error membungkus `bus.ErrBadRequestType`.
9. Konfigurasi pencarian diganti lebih buruk (simulasi) -> `TestSearchEval` gagal dengan daftar metrik yang turun > 0.02.
10. Project tidak dikenal / tidak punya data -> laporan nol semua, semua rate 0, 11 bucket ada.

---

## 12. Request/Response body

Bus dipanggil dengan nilai struct Go; JSON di bawah adalah representasi field-nya (scanentries tanpa tag json, ditulis dengan nama field). Hanya `searchquality.Response` yang punya tag json karena dicetak CLI `--json`.

### 12.1 `core.audit.scan-entries`

Request:

```json
{
  "ProjectID": "0192f0a4-6c1e-7b3a-9c55-2f1e8d7a4b10",
  "Actions": ["mcp.search_knowledge", "mcp.read_file"],
  "From": "2026-10-14T00:00:00Z",
  "To": "2026-10-21T00:02:00Z",
  "After": {"OccurredAt": "2026-10-15T08:30:12.123456Z", "ID": "0192f0b1-0000-7000-8000-00000000a001"},
  "Limit": 1000
}
```

Response:

```json
{
  "Entries": [
    {
      "ID": "0192f0b1-0000-7000-8000-00000000a002",
      "OccurredAt": "2026-10-15T08:31:00.000001Z",
      "ActorType": "api_key",
      "ActorID": "0192f0a9-1111-7aaa-8bbb-0000000000c1",
      "Action": "mcp.search_knowledge",
      "Target": "docs",
      "Status": "ok",
      "Args": {"query": "rate limit", "path": "docs", "limit": 10},
      "Result": {
        "count": 3, "total": 3,
        "result_paths": ["docs/api/rate-limit.md", "docs/ops/nginx.md", "docs/faq.md"],
        "bytes": 812, "original_bytes": 812, "truncated": false, "duration_ms": 14
      }
    },
    {
      "ID": "0192f0b1-0000-7000-8000-00000000a003",
      "OccurredAt": "2026-10-15T08:31:30Z",
      "ActorType": "api_key",
      "ActorID": "0192f0a9-1111-7aaa-8bbb-0000000000c1",
      "Action": "mcp.read_file",
      "Target": "docs/ops/nginx.md",
      "Status": "ok",
      "Args": {"path": "docs/ops/nginx.md"},
      "Result": {"bytes": 2048, "original_bytes": 2048, "truncated": false, "duration_ms": 6}
    }
  ],
  "Next": {"OccurredAt": "2026-10-15T08:31:30Z", "ID": "0192f0b1-0000-7000-8000-00000000a003"}
}
```

Halaman terakhir: `"Next": null`. Tanpa data: `{"Entries": [], "Next": null}`.

### 12.2 `audit_log.result` untuk `mcp.search_knowledge` (setelah T11)

```json
{
  "count": 15,
  "total": 42,
  "result_paths": ["docs/a.md", "docs/b.md", "docs/c.md", "docs/d.md", "docs/e.md",
                   "docs/f.md", "docs/g.md", "docs/h.md", "docs/i.md", "docs/j.md"],
  "bytes": 3120, "original_bytes": 3120, "truncated": false, "duration_ms": 21
}
```

Tanpa hasil: `{"count": 0, "total": 0, "result_paths": [], ...}`. Anggaran terlampaui: `"result_paths": [...7 path...], "result_paths_truncated": true`.

### 12.3 `discovery.knowledgetools.search-quality-report`

Request:

```json
{"ProjectID": "0192f0a4-6c1e-7b3a-9c55-2f1e8d7a4b10", "Since": "2026-10-14T00:00:00Z", "Until": "0001-01-01T00:00:00Z", "Window": 120000000000}
```

Response (juga keluaran persis `grimoire search-quality report --json`):

```json
{
  "report": {
    "project_id": "0192f0a4-6c1e-7b3a-9c55-2f1e8d7a4b10",
    "since": "2026-10-14T00:00:00Z",
    "until": "2026-10-21T09:15:02Z",
    "window_seconds": 120,
    "searches": 120,
    "zero_result": {"count": 18, "denominator": 120, "rate": 0.15},
    "follow_up": {"count": 61, "denominator": 100, "rate": 0.61},
    "reformulation": {"count": 27, "denominator": 120, "rate": 0.225},
    "rank_opened": [
      {"label": "1", "count": 38}, {"label": "2", "count": 12}, {"label": "3", "count": 5},
      {"label": "4", "count": 2}, {"label": "5", "count": 2}, {"label": "6", "count": 1},
      {"label": "7", "count": 0}, {"label": "8", "count": 1}, {"label": "9", "count": 0},
      {"label": "10", "count": 0}, {"label": "not_in_results", "count": 9}
    ],
    "legacy_searches": 20
  }
}
```

Keluaran teks (default):

```
Kualitas pencarian project 0192f0a4-6c1e-7b3a-9c55-2f1e8d7a4b10
Rentang  2026-10-14T00:00:00Z .. 2026-10-21T09:15:02Z (jendela 120s)
Pencarian                 120
Zero-result               18/120   15.0%
Follow-up                 61/100   61.0%
Reformulasi               27/120   22.5%
Rank dibuka               1:38  2:12  3:5  4:2  5:2  6:1  7:0  8:1  9:0  10:0  tidak di hasil:9
Entri lama dilewati       20 (tanpa result_paths; tidak ikut follow-up/rank)
```

### 12.4 Format error

Error bus memakai `*errs.Error` (pola existing). Request invalid:

```json
{"code": "INVALID_ARGUMENT", "message": "project_id tidak valid"}
```

Contoh pesan lain (tanpa nilai input): `"since wajib diisi"`, `"until harus setelah since"`, `"window harus 1s..1h"`, `"actions tidak valid"`, `"limit tidak valid"`, `"after tidak valid"`, `"rentang terlalu besar (lebih dari 2000000 entri); persempit --since/--until"`.

Tipe body salah: error Go `bus: unexpected request type: want=scanentries.Request got=*scanentries.Request` (`errors.Is(err, bus.ErrBadRequestType)`).

Kegagalan infra di provider scan: `fmt.Errorf("scanentries: query: %w", err)`; di laporan: `fmt.Errorf("searchquality: scan audit: %w", err)`.

CLI:

| Kondisi | stderr | exit |
|---|---|---|
| Flag wajib hilang / tak terbaca / window di luar rentang | `search-quality: --since wajib diisi` + usage | 2 |
| `errs.Error` dari bus (`INVALID_ARGUMENT`) | `search-quality: project_id tidak valid` | 1 |
| Kegagalan infra (DB tidak terjangkau) | `search-quality report gagal` (detail di log stderr) | 1 |
| Sukses | (log saja) | 0 |

---

## 13. Risiko dan catatan

- Follow-up adalah perkiraan (agent bisa membuka karena alasan lain, atau cuplikan sudah cukup); dibaca sebagai tren. Ditulis di `docs/search-quality.md`.
- Retensi 90 hari: laporan `--json` disimpan manual bila tren jangka panjang dibutuhkan.
- Path yang dipotong 1024 byte tetap cocok dengan `target` read_file karena batasnya sama (D4). Path dibuang karena anggaran (D5) hanya mungkin bila path rata-rata > ~600 byte; dampaknya rank > posisi terakhir tercatat masuk `not_in_results`.
- Penyimpangan dari tiket ("Tidak ada endpoint bus baru") tercatat di tiket dan katalog (F7).
