# T15 core/audit-log `summary`: Plan (fullstack)

Tiket: `docs/tickets/T15-core-audit-log-summary.md`.

Konteks yang dibaca: `CLAUDE.md` (tidak ada); `.claude/rules/migrations.md` (tidak relevan karena tiket ini tanpa migrasi); `rules.txt` bagian 4 (batas import); `docs/tickets/README.md` (konvensi arsitektur, katalog bus baris `discovery.knowledgetools.search-quality-report` yang sudah mencantumkan T15 sebagai consumer, katalog aksi, bagian "Aksi baru"); `docs/superpowers/specs/2026-10-20-grimoire-design.md` (file `*knowledge-hub-design.md` tidak ada, ini padanannya); `DESIGN.md`; `.claude/skills/design-taste-frontend/references/app-screens.md` (bagian 4.4, 5, 6, 11, 12).

Kode yang dicek: `auditlog/base.go`, `auditlog/arch_test.go`, `auditlog/base/{filter,entry}.go`, `auditlog/listentries/*` (pola params/handler/repository, `explain_integration_test.go`, `repository_test.go`), `auditlog/getentry/http_integration_test.go` (pola `X-Test-Role`), `infra/db/tx.go` (`BeginTx` memakai opsi `nil`), `infra/bus/typed.go`, `lib/contracts/core/audit/record/actions.go`, `lib/contracts/discovery/knowledgetools/searchquality/searchquality.go`, `discovery/.../searchquality/{params,repository,init}.go` (pola adapter bus di `repository.go`), `discovery/.../mcpgateway/base/audit.go` (`duration_ms` berupa `int64` milidetik), `cmd/cli/searchquality.go`, `cmd/internal/server/searchquality_report_integration_test.go`, `backend/migrations/core/20261021090000000_create_audit_log.sql`, `frontend/src/app/routes/{index,guard}.ts` (route `/audit-log`/`/admin/users` dengan `roles: ['super_admin']`; role tidak cocok menghasilkan `notfound`), `frontend/src/app/layouts/AppLayout.svelte` (item nav Users dan Audit Log di blok `role === 'super_admin'`), `frontend/src/app/views/AuditLogView.svelte` (tidak diubah oleh T15), `frontend/src/domain/core/audit-log/listentries/components/AuditTable.svelte`, `frontend/.dependency-cruiser.cjs`, `frontend/src/app.css` (token `chart-1/2/3`), `api/modules/core/audit-log.yml`, `api/modules/project/management.yml` (`GET /api/projects`, `limit` maks 100).

Plan ini satu-satunya rujukan untuk T15, backend dan frontend. Feature backend `summary` tidak dipecah: satu usecase (`Summarize`) yang mengorkestrasi window, repository, bus T11, dan pembentukan laporan.

**Revisi penempatan (keputusan pemilik):** ringkasan metrik MCP tidak lagi menjadi tab di halaman Audit Log, melainkan halaman **Dashboard** sendiri (`/dashboard`) yang hanya bisa dilihat Super Admin. Rancangan tab (tab Log/Ringkasan, `?tab=summary`, pembawaan query antar tab, perubahan `AuditLogView`/`ListEntriesView`) dibatalkan seluruhnya. Backend tidak berubah karena revisi ini (tiket K12). Keputusan turunan (route, nav, URL state) ada di tiket K10 sampai K12 dan ditandai belum disetujui pemilik.

---

## 1. Metadata

| Atribut | Nilai |
|---|---|
| Ticket | T15 |
| Scope | fullstack |
| Domain / Module | core / audit-log (`backend/internal/domain/core/internal/auditlog`, `frontend/src/domain/core/audit-log`) |
| Feature baru | backend `auditlog/summary`, frontend `audit-log/summary` |
| Feature diubah | frontend `audit-log/listentries` (`AuditTable` klik baris) |
| App (frontend) | route baru `/dashboard` di `app/routes/index.ts`, view baru `app/views/DashboardView.svelte`, item nav "Dashboard" di `app/layouts/AppLayout.svelte` (khusus super_admin). `AuditLogView.svelte` tidak diubah |
| Migrasi | tidak ada |
| Aksi audit baru | tidak ada |
| Endpoint bus baru | tidak ada (consumer baru `discovery.knowledgetools.search-quality-report`) |
| Dependensi baru | `chart.js` (frontend, versi dikunci tanpa `^`/`~`) |

| Endpoint | Method | Version | Akses |
|---|---|---|---|
| `/api/audit-log/summary` | GET | `/api` tanpa prefix versi; `api/modules/core/audit-log.yml` `info.version` 0.1.0 menjadi 0.2.0 (penambahan path, aditif) | `super_admin` (grup `/audit-log` sudah memasang `middleware.RequireRole`) |
| `/api/projects` (T03, sudah ada) | GET | tidak berubah | dipakai FE untuk pemilih project |

---

## 2. Summary Plan

Tambah feature backend `summary` di `core/audit-log` yang melayani `GET /api/audit-log/summary?range=24h|7d|30d&project_id=<uuid?>`. Feature ini mengembalikan agregat panggilan tool MCP dari `audit_log` dalam satu respons: total, top project (maks 10), pemakaian tool, metrik per tool (ok/error, error rate, p50/p95 latensi), dan trafik per bucket. Bila `project_id` diisi, respons juga memuat kualitas pencarian dari kontrak bus T11.

Keputusan inti:

1. **Definisi tool MCP (K1)** memakai daftar positif `base.ToolActions()` = `record.Actions()` yang berawalan `mcp.` dikurangi `base.GatewayActions()` (5 konstanta `record.ActionMCP*`). Konstanta yang benar untuk penolakan auth adalah `record.ActionMCPAuthRejected = "mcp.auth.rejected"`. Tiket menulisnya `mcp.auth_rejected`, dan itu salah tulis. Predikat SQL berbentuk `action = ANY($1::text[])`, bukan `LIKE 'mcp.%'`, agar `audit_log_action_idx` terpakai. Ada tes kelengkapan.
2. **Agregasi di SQL (K3).** Repository menjalankan tiga query agregat (metrik per tool, top project, trafik) dalam satu transaksi `ReadOnly` + `REPEATABLE READ`, jadi ketiganya membaca satu snapshot. Dari query metrik per tool diturunkan `total_calls`, `total_errors`, dan `tool_usage` di entity, tanpa query keempat. Hasilnya, angka yang tampil di beberapa tempat berasal dari satu variabel.
3. **Latensi** memakai `percentile_cont` atas `CASE WHEN jsonb_typeof(result->'duration_ms') = 'number' THEN (result->>'duration_ms')::numeric END`. `percentile_cont` mengabaikan NULL, sehingga data lama atau data rusak tidak gagal di-cast, dan tool tanpa angka latensi menghasilkan `null`.
4. **Trafik (K6)** memakai `generate_series` + `LEFT JOIN` agregat `date_trunc(unit, occurred_at, 'UTC')`. Unit diambil dari whitelist `base.Bucket` (`hour`/`day`). `base.BucketStarts` di Go menjadi oracle untuk jumlah dan awal bucket, dan dipakai untuk validasi hasil SQL.
5. **Kualitas pencarian (K5)** dipanggil berurutan setelah transaksi SQL selesai, hanya bila `project_id` diisi, dengan timeout 5 detik dan sifat fail-soft. Bila tanpa project, nilainya `search_quality: null`. Bila bus gagal, nilainya `{available:false, error_code:"UNAVAILABLE"}`, respons tetap 200, dan error di-log. Komentar `// null bila ... bus gagal` pada contoh kontrak di tiket dianggap tidak presisi. UI tiket sendiri membedakan "pilih project" dari "available=false karena error, coba lagi".
6. **Label project** diambil dari label terbaru di `audit_log` (`(array_agg(project_label ORDER BY occurred_at DESC, id DESC))[1]`), tanpa bus ke `project.management` dan tanpa penanda "dihapus". Tiket mengizinkan cara ini ("cukup label apa adanya bila penanda sulit"), dan tidak ada titik gagal tambahan.
7. **Tanpa** migrasi, indeks baru, aksi audit, atau endpoint bus baru. `infra/db` tidak diubah. Repository memanggil `d.BeginTxx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})` langsung, dan rollback selalu dijalankan lewat `defer`.

Frontend menambah halaman Dashboard di route baru `/dashboard` (khusus super_admin, guard sama dengan `/audit-log`) dengan item navigasi sendiri di sidebar. `app/views/DashboardView.svelte` merender `PageHeader` "Dashboard" lalu `SummaryView` yang di-lazy-load (`import()`) bersama Chart.js. Isinya filter preset rentang dan project, dua angka utama, lima chart, dan blok kualitas pencarian. `AuditTable` juga diubah agar seluruh baris bisa diklik (K9); selain itu halaman Audit Log tidak berubah.

---

## 3. Plan (langkah berurutan)

Urutan wajib: a, lalu b, c, d, e, f, g, h. Frontend (langkah i sampai l) boleh dimulai paralel setelah langkah f (kontrak OpenAPI sudah ada), memakai mock dulu.

### a. Entity dan rule: `auditlog/base/summary.go` (+ `summary_test.go`)

File baru `backend/internal/domain/core/internal/auditlog/base/summary.go`, tanpa I/O. Package ini sudah meng-import `lib/contracts/core/audit/record`, dan import `lib/contracts/discovery/knowledgetools/searchquality` (tipe netral) juga diizinkan rules 4.4/4.5.

```go
package base

// Range preset ringkasan.
type Range string

const (
	Range24h     Range = "24h"
	Range7d      Range = "7d"
	Range30d     Range = "30d"
	DefaultRange       = Range7d
)

// Bucket adalah satuan date_trunc yang di-whitelist (dipakai sebagai literal parameter SQL).
type Bucket string

const (
	BucketHour Bucket = "hour"
	BucketDay  Bucket = "day"
)

// Interval mengembalikan teks interval Postgres untuk generate_series ("1 hour" / "1 day").
func (b Bucket) Interval() string

// Step mengembalikan time.Duration satu bucket (time.Hour / 24*time.Hour).
func (b Bucket) Step() time.Duration

// SummaryQuery adalah input ringkasan yang sudah diparse (bentuk) oleh params.go.
type SummaryQuery struct {
	Range     Range  // "" berarti DefaultRange (dinormalisasi oleh Validate)
	ProjectID string // "" = semua project; dinormalisasi huruf kecil
}

// Validate menormalisasi Range kosong ke 7d, menolak range lain (field "range"),
// menolak project_id bukan uuid (field "project_id"), dan menurunkan ProjectID ke huruf kecil.
// Error memakai fieldErr yang sudah ada di filter.go (errs.CodeInvalid + details {"field": ...}).
func (q *SummaryQuery) Validate() error

// Window adalah rentang [From, To) dan bucket trafiknya.
type Window struct {
	Range  Range
	From   time.Time // UTC
	To     time.Time // UTC, eksklusif
	Bucket Bucket
}

// NewWindow: To = now.UTC().Truncate(time.Second); From = To - durasi range
// (24h = 24 jam, 7d = 168 jam, 30d = 720 jam). Bucket: 24h -> hour, 7d/30d -> day.
// Prasyarat: r sudah tervalidasi.
func NewWindow(r Range, now time.Time) Window

// BucketStarts mengembalikan awal setiap bucket dari truncate(From, Bucket) (UTC)
// sampai awal bucket terakhir yang < To. Jumlah: 24h -> 24 (From tepat di awal jam) atau 25;
// 7d -> 7 atau 8; 30d -> 30 atau 31.
func (w Window) BucketStarts() []time.Time

// GatewayActions adalah aksi protokol/gateway yang BUKAN panggilan tool (K1). Satu sumber: record.ActionMCP*.
func GatewayActions() []string // initialize, tools_list, auth.rejected, rate_limited, unknown_tool

// ToolActions = record.Actions() yang berawalan "mcp." dikurangi GatewayActions(), urutan katalog.
func ToolActions() []string

// ToolName membuang awalan "mcp." ("mcp.read_file" -> "read_file").
func ToolName(action string) string

// ToolAgg adalah satu baris agregat per aksi tool dari repository.
type ToolAgg struct {
	Action string
	Calls  int
	OK     int
	Errors int
	P50Ms  *float64 // nil bila tidak ada duration_ms numerik
	P95Ms  *float64
}

// ProjectAgg adalah satu baris top project.
type ProjectAgg struct {
	ProjectID    string
	ProjectLabel string // label terbaru di audit_log; "" bila semua NULL
	Calls        int
}

// TrafficPoint adalah satu bucket trafik.
type TrafficPoint struct {
	At     time.Time `json:"at"`
	Calls  int       `json:"calls"`
	Errors int       `json:"errors"`
}

// Aggregates adalah hasil mentah repository dari satu snapshot.
type Aggregates struct {
	Tools    []ToolAgg
	Projects []ProjectAgg // sudah urut calls DESC, project_id ASC, maks TopProjectsLimit
	Traffic  []TrafficPoint
}

const TopProjectsLimit = 10

// --- bentuk respons (tag JSON = kontrak HTTP) ---

type TopProject struct {
	ProjectID    string `json:"project_id"`
	ProjectLabel string `json:"project_label"` // fallback = ProjectID bila label kosong
	Calls        int    `json:"calls"`
}

type ToolUsage struct {
	Tool  string `json:"tool"`
	Calls int    `json:"calls"`
}

type ToolMetric struct {
	Tool      string   `json:"tool"`
	Calls     int      `json:"calls"`
	OK        int      `json:"ok"`
	Error     int      `json:"error"`
	ErrorRate float64  `json:"error_rate"` // Error/Calls (Calls >= 1 selalu)
	P50Ms     *float64 `json:"p50_ms"`     // null bila tanpa data latensi
	P95Ms     *float64 `json:"p95_ms"`
}

type SearchQuality struct {
	Available      bool                      `json:"available"`
	ErrorCode      string                    `json:"error_code,omitempty"` // "UNAVAILABLE" bila Available=false
	Searches       *int                      `json:"searches,omitempty"`
	WindowSeconds  *int                      `json:"window_seconds,omitempty"`
	ZeroResult     *searchquality.Metric     `json:"zero_result,omitempty"`
	FollowUp       *searchquality.Metric     `json:"follow_up,omitempty"`
	Reformulation  *searchquality.Metric     `json:"reformulation,omitempty"`
	RankOpened     []searchquality.RankBucket `json:"rank_opened,omitempty"`
	LegacySearches *int                      `json:"legacy_searches,omitempty"`
}

const SearchQualityUnavailable = "UNAVAILABLE"

// QualityFromReport memetakan Report T11 1:1 (Available=true).
func QualityFromReport(r searchquality.Report) *SearchQuality
// QualityUnavailable mengembalikan {Available:false, ErrorCode:"UNAVAILABLE"}.
func QualityUnavailable() *SearchQuality

type Report struct {
	Range         Range          `json:"range"`
	From          time.Time      `json:"from"`
	To            time.Time      `json:"to"`
	Bucket        Bucket         `json:"bucket"`
	ProjectID     *string        `json:"project_id"`   // null bila semua project
	TotalCalls    int            `json:"total_calls"`  // = jumlah ToolMetric.Calls
	TotalErrors   int            `json:"total_errors"` // = jumlah ToolMetric.Error
	ErrorRate     *float64       `json:"error_rate"`   // TotalErrors/TotalCalls; null bila TotalCalls 0
	TopProjects   []TopProject   `json:"top_projects"`   // tidak pernah null ([] bila kosong)
	ToolUsage     []ToolUsage    `json:"tool_usage"`
	ToolMetrics   []ToolMetric   `json:"tool_metrics"`
	Traffic       []TrafficPoint `json:"traffic"`
	SearchQuality *SearchQuality `json:"search_quality"` // null hanya bila ProjectID kosong
}

// BuildReport menyusun laporan dari agregat. Rule:
// - ToolMetrics/ToolUsage urut Calls DESC, lalu Tool ASC; Tool = ToolName(Action).
// - ErrorRate per tool = Error/Calls; P50/P95 dibulatkan 2 desimal (nil tetap nil).
// - TotalCalls/TotalErrors dijumlah dari ToolMetrics; ErrorRate total null bila 0 panggilan.
// - TopProject.ProjectLabel = ProjectID bila label kosong.
// - Traffic WAJIB sama panjang dan sama awal bucket dengan w.BucketStarts(); bila tidak,
//   kembalikan error internal (penjaga bug SQL/zona waktu).
// - Semua slice non-nil.
func BuildReport(w Window, q SummaryQuery, a Aggregates, sq *SearchQuality) (Report, error)
```

Tes `base/summary_test.go` (unit, tanpa DB):
- `TestSummaryQuery_Validate`: `""` menjadi `7d`; `24h`/`7d`/`30d` lolos; `1d`, `7D`, `30d ` (spasi tidak di-trim di sini, trim dilakukan di params), `90d` ditolak dengan details `field=range`; project `not-a-uuid` ditolak dengan `field=project_id`; uuid huruf besar dinormalisasi ke huruf kecil; error `errors.Is(err, errs.ErrInvalid)`.
- `TestNewWindow`: untuk tiap range, `To - From` benar, `To` tanpa sub-detik, UTC, bucket benar.
- `TestBucketStarts`: `now = 2026-10-22T10:30:15Z`, 24h menghasilkan 25 bucket dengan awal `2026-10-21T10:00:00Z`; `now = 10:00:00Z` menghasilkan 24; 7d menghasilkan 8 atau 7; 30d menghasilkan 31 atau 30; setiap elemen tepat di awal jam/hari UTC dan < `To`.
- `TestToolActions_Completeness` (AC3): setiap aksi `record.Actions()` berawalan `mcp.` masuk tepat satu dari `ToolActions()` / `GatewayActions()`; `len(GatewayActions()) == 5` dan berisi tepat `mcp.initialize`, `mcp.tools_list`, `mcp.auth.rejected`, `mcp.rate_limited`, `mcp.unknown_tool`; `ToolActions()` berisi 6 tool saat ini; tidak ada aksi non-`mcp.` di `ToolActions()`.
- `TestBuildReport`: urutan tool (seri calls dipecah dengan nama), `ErrorRate` 0.1 untuk 10/1 (AC4), pembulatan p50/p95, `nil` tetap `null` setelah `json.Marshal`, total = jumlah baris, `error_rate` total null saat 0, label fallback, slice kosong di-marshal sebagai `[]` bukan `null`, traffic beda panjang menghasilkan error.
- `TestQualityFromReport`: 11 bucket rank dan semua field dipetakan; `QualityUnavailable` di-marshal menjadi tepat `{"available":false,"error_code":"UNAVAILABLE"}`.

### b. Repository: `auditlog/summary/repository.go`

Satu file dengan dua adapter: SQL (`Repository`) dan bus (`QualityReporter`), mengikuti pola `knowledgetools/searchquality/repository.go`.

```go
package summary

// Repository adalah satu-satunya yang menyentuh SQL di feature ini. Hanya SELECT.
type Repository interface {
	// Load menjalankan tiga query agregat dalam satu tx ReadOnly REPEATABLE READ.
	// projectID "" = semua project.
	Load(ctx context.Context, w base.Window, projectID string) (base.Aggregates, error)
}

type sqlRepository struct{ d *db.DB }

// NewRepository tidak mendereferensi d (arch_test merakit dengan DB nil).
func NewRepository(d *db.DB) Repository { return &sqlRepository{d: d} }

// QualityReporter adalah port ke provider discovery.knowledgetools.search-quality-report.
type QualityReporter interface {
	Report(ctx context.Context, req searchquality.Request) (searchquality.Report, error)
}

type busQualityReporter struct{ b *bus.Registry }

func NewQualityReporter(b *bus.Registry) QualityReporter { return &busQualityReporter{b: b} }

func (r *busQualityReporter) Report(ctx context.Context, req searchquality.Request) (searchquality.Report, error) {
	res, err := bus.CallTyped[searchquality.Response](ctx, r.b, constants.BUS_DISCOVERY_KNOWLEDGETOOLS_SEARCH_QUALITY_REPORT, req)
	return res.Report, err
}
```

Isi `Load`:

```go
tx, err := r.d.BeginTxx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
if err != nil { return base.Aggregates{}, fmt.Errorf("summary: begin tx: %w", err) }
defer func() { _ = tx.Rollback() }() // read-only: rollback = selesai; tidak perlu commit
// 1) tools, 2) projects, 3) traffic: semua lewat tx.SelectContext
```

Predikat bersama dibuat oleh `buildWhere(w, projectID)` (fragmen literal; nilai lewat placeholder, pola `listentries.buildQuery`):

```
action = ANY($1::text[]) AND occurred_at >= $2 AND occurred_at < $3 [AND project_id = $4::uuid]
```

`$1 = base.ToolActions()`. Predikat project hanya ditambahkan bila diisi (tanpa `($4 IS NULL OR ...)`, supaya planner bisa memilih `audit_log_project_idx`).

Query 1 (metrik per tool):

```sql
SELECT action,
       count(*)                                    AS calls,
       count(*) FILTER (WHERE status = 'ok')       AS ok,
       count(*) FILTER (WHERE status = 'error')    AS errors,
       percentile_cont(0.5)  WITHIN GROUP (ORDER BY CASE WHEN jsonb_typeof(result->'duration_ms') = 'number'
                                                         THEN (result->>'duration_ms')::numeric END) AS p50_ms,
       percentile_cont(0.95) WITHIN GROUP (ORDER BY CASE WHEN jsonb_typeof(result->'duration_ms') = 'number'
                                                         THEN (result->>'duration_ms')::numeric END) AS p95_ms
FROM audit_log
WHERE <where>
GROUP BY action
```

`p50_ms`/`p95_ms` di-scan ke `sql.NullFloat64`. Dipakai `CASE`, bukan `FILTER`, agar cast tidak dievaluasi untuk nilai non-angka.

Query 2 (top project):

```sql
SELECT project_id::text AS project_id,
       COALESCE((array_agg(project_label ORDER BY occurred_at DESC, id DESC))[1], '') AS project_label,
       count(*) AS calls
FROM audit_log
WHERE <where> AND project_id IS NOT NULL
GROUP BY project_id
ORDER BY calls DESC, project_id ASC
LIMIT 10
```

`LIMIT` diambil dari `base.TopProjectsLimit` lewat placeholder.

Query 3 (trafik):

```sql
WITH t AS (
  SELECT date_trunc($u, occurred_at, 'UTC') AS at,
         count(*) AS calls,
         count(*) FILTER (WHERE status = 'error') AS errors
  FROM audit_log
  WHERE <where>
  GROUP BY 1
)
SELECT b.at, COALESCE(t.calls, 0) AS calls, COALESCE(t.errors, 0) AS errors
FROM generate_series(date_trunc($u, $2::timestamptz, 'UTC'),
                     $3::timestamptz - interval '1 microsecond',
                     $i::interval) AS b(at)
LEFT JOIN t ON t.at = b.at
ORDER BY b.at
```

`$u = string(w.Bucket)` (whitelist `hour|day`) dan `$i = w.Bucket.Interval()`. Postgres 16 (docker-compose) mendukung `date_trunc(text, timestamptz, text)`. Hasil `at` di-`.UTC()`.

Kolom yang diseleksi hanya agregat, label, dan `action`/`project_id`. `args`, isi `result`, `target`, `ip`, dan `user_agent` tidak pernah diseleksi (AC10).

### c. Usecase: `auditlog/summary/usecase.go`

```go
const qualityTimeout = 5 * time.Second

type Usecase struct {
	repo    Repository
	quality QualityReporter
	now     func() time.Time
	log     *slog.Logger
}

func NewUsecase(r Repository, q QualityReporter, now func() time.Time, log *slog.Logger) *Usecase

// Summarize: q sudah tervalidasi (q.Validate() di handler).
func (u *Usecase) Summarize(ctx context.Context, q base.SummaryQuery) (base.Report, error) {
	w := base.NewWindow(q.Range, u.now())
	agg, err := u.repo.Load(ctx, w, q.ProjectID)
	if err != nil { return base.Report{}, fmt.Errorf("summary: %w", err) }
	var sq *base.SearchQuality
	if q.ProjectID != "" {
		cctx, cancel := context.WithTimeout(ctx, qualityTimeout)
		rep, err := u.quality.Report(cctx, searchquality.Request{ProjectID: q.ProjectID, Since: w.From, Until: w.To}) // Window 0 = 120s, sama dengan default CLI
		cancel()
		if err != nil {
			u.log.WarnContext(ctx, "audit summary: search quality unavailable", "err", err) // tanpa args/query
			sq = base.QualityUnavailable()
		} else {
			sq = base.QualityFromReport(rep)
		}
	}
	return base.BuildReport(w, q, agg, sq)
}
```

Usecase ini tidak memuat rule bisnis. Window, pemetaan T11, urutan, rasio, dan validasi bucket semuanya ada di `base`. Bila klien membatalkan request (`ctx.Err() != nil`) setelah bus gagal, perilakunya tetap sama: respons tidak terkirim dan tidak ada efek samping.

### d. Params, handler, init: `auditlog/summary/{params,handler,init}.go`

`params.go`:

```go
// Package summary adalah feature GET /api/audit-log/summary (T15).
package summary

// ParseQuery hanya membaca BENTUK: range dan project_id di-TrimSpace; parameter lain diabaikan.
// Nilai divalidasi di base.SummaryQuery.Validate.
func ParseQuery(q url.Values) base.SummaryQuery
```

`handler.go`:

```go
type summarizer interface {
	Summarize(ctx context.Context, q base.SummaryQuery) (base.Report, error)
}
type Handler struct{ uc summarizer }
func NewHandler(uc summarizer) *Handler
// Get melayani GET /summary: ParseQuery -> Validate (400) -> Summarize (500 generik bila error) -> 200 JSON.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request)
```

Pakai `respond.WriteError` dan `respond.WriteJSON`, seperti `listentries`.

`init.go`:

```go
// Init merakit repository -> usecase -> handler dan mendaftarkan GET /summary.
// RequireRole(super_admin) dipasang di grup /audit-log oleh auditlog/base.go.
// Tidak mendereferensi d.DB/d.Bus saat dirakit (arch_test memakai DB nil).
func Init(d app.Deps, r chi.Router) {
	uc := NewUsecase(NewRepository(d.DB), NewQualityReporter(d.Bus), time.Now, d.Log)
	r.Get("/summary", NewHandler(uc).Get)
}
```

Catatan: `/summary` adalah rute statis, dan chi memprioritaskannya di atas `/{id}` (getentry). Tes HTTP tetap membuktikannya.

### e. Wiring module: `auditlog/base.go` dan `auditlog/arch_test.go`

- `base.go`: import `grimoire/internal/domain/core/internal/auditlog/summary`, lalu panggil `summary.Init(d, r)` setelah `getentry.Init(d, r)` di dalam `d.API.Route("/audit-log", ...)`. Komentar `Register` ditambah: "T15: GET /audit-log/summary; consumer bus discovery.knowledgetools.search-quality-report."
- `arch_test.go` `TestOnlyGetRoutes`: ubah ekspektasi menjadi tepat 3 rute GET (`/audit-log/`, `/audit-log/summary`, `/audit-log/{id}`), dan tegaskan `/audit-log/summary` ada di daftar. Ubah komentar menjadi "AC7 T02 + T15". `TestAppendOnlySources` tetap berlaku untuk file baru (hanya SELECT).

### f. Kontrak API: `api/modules/core/audit-log.yml`, `api/root.yml`, `api/openapi.yml`

- Tambah path `/api/audit-log/summary` (operationId `getAuditSummary`, tag `audit-log`), parameter `range` (enum `24h|7d|30d`, default `7d`) dan `project_id` (uuid, opsional). Respons: `200` (schema `AuditSummary`, contoh pada bagian 8), `400` (`foundation.yml#/components/responses/BadRequest`, contoh `field=range`), `401`, `403`. Deskripsi menyebut: khusus super_admin; `[from, to)` mundur dari waktu request; bucket UTC; `search_quality` null tanpa project, `available=false` bila T11 gagal; tidak memuat args/result/query/path; tidak dicatat ke audit.
- Tambah schema `AuditSummary`, `AuditSummaryTopProject`, `AuditSummaryToolUsage`, `AuditSummaryToolMetric` (`p50_ms`/`p95_ms` `type: [number, "null"]`), `AuditSummaryTrafficPoint`, `AuditSummarySearchQuality` (`oneOf` dua bentuk: available true dengan semua field wajib, atau `{available:false, error_code}`), `SearchQualityMetric`, `SearchQualityRankBucket`. Semua dengan `additionalProperties: false`.
- `info.version` 0.1.0 menjadi 0.2.0.
- `api/root.yml`: tambah `/api/audit-log/summary: $ref: ./modules/core/audit-log.yml#/paths/~1api~1audit-log~1summary` (di antara `/api/audit-log` dan `/api/audit-log/{id}`) dan `$ref` schema `AuditSummary` di bagian komponen, mengikuti pola baris 93-97.
- Jalankan `make api-bundle`, `make api-check`, lalu commit `api/openapi.yml` (AC16).

### g. Tes server end-to-end kesetaraan T11 (AC8, AC9)

File baru `backend/cmd/internal/server/auditsummary_integration_test.go` (`//go:build integration`). File ini memakai helper `sqrSetup`/`insert`/`search`/`read` dari `searchquality_report_integration_test.go` (satu package `server`), jadi tidak ada duplikasi helper. Detail di bagian 6.

### h. Dokumen

- `docs/mcp-gateway.md` bagian `## Audit`: satu paragraf yang menyatakan bahwa ringkasan pemakaian tool tampil di halaman Dashboard `/dashboard` (khusus Super Admin; data dari `GET /api/audit-log/summary`), definisi tool = `mcp.*` minus 5 aksi gateway, dan latensi dari `result.duration_ms`.
- `docs/search-quality.md`: catatan bahwa metrik yang sama tampil di halaman Dashboard (`/dashboard`) untuk project terpilih, dengan window 120s default, rentang preset, CLI tetap ada, serta catatan snapshot (laporan T11 memakai koneksi sendiri, jadi bisa berbeda sedikit dari agregat tool bila ada insert bersamaan).
- `docs/tickets/README.md`: baris katalog bus `discovery.knowledgetools.search-quality-report` sudah mencantumkan T15. Tambahkan catatan revisi T15 pada bagian "Perubahan dari rencana awal": (1) `mcp.auth_rejected` di tiket dibaca sebagai `mcp.auth.rejected`; (2) `search_quality` gagal = `{available:false}` bukan `null`; (3) field aditif `total_errors`, `error_rate`, `project_id`; (4) label project dari audit tanpa penanda dihapus; (5) penempatan UI berubah dari tab Audit Log menjadi halaman `/dashboard` khusus Super Admin, endpoint tetap di module audit-log. Tandai T15 di daftar tiket sesuai status saat merge (kolom Module sudah berbunyi "audit-log (dashboard metrik MCP + baris tabel bisa diklik)").
- `docs/tickets/T15-core-audit-log-summary.md` sudah direvisi untuk penempatan Dashboard (K10 sampai K12); saat implementasi tidak diubah isinya, kecuali status bila README memintanya.

### i sampai l. Frontend (detail di bagian 9)

- i. `frontend/package.json`: `chart.js` versi stabil terbaru yang sudah rilis minimal dua minggu (cek `npm view chart.js time --json`; contoh `4.5.0`), ditulis tanpa `^`/`~`, lalu `npm install` agar lockfile ikut.
- j. Feature `summary` dengan mock (`mocks/summary.ts`), kemudian route `/dashboard` + `DashboardView` (lazy `import()` feature `summary`) + item nav "Dashboard" khusus super_admin di `AppLayout`, lalu wiring ke API nyata.
- k. `AuditTable` klik baris (K9). `AuditLogView` tidak disentuh.
- l. i18n, tes unit/komponen, e2e Playwright.

---

## 4. Migration

Tidak ada. Tidak ada tabel, kolom, atau indeks baru. Kueri memakai `audit_log_action_idx (action, occurred_at DESC, id DESC)` dan `audit_log_project_idx (project_id, occurred_at DESC, id DESC)` dari `backend/migrations/core/20261021090000000_create_audit_log.sql`. Bila pengukuran 1 juta baris menunjukkan lambat, indeks parsial diusulkan di tiket lanjutan (K3).

---

## 5. Changes

### Backend (baru)

| File | Isi |
|---|---|
| `backend/internal/domain/core/internal/auditlog/base/summary.go` | Range, Bucket, SummaryQuery.Validate, Window/NewWindow/BucketStarts, GatewayActions/ToolActions/ToolName, Aggregates, Report + BuildReport, SearchQuality + QualityFromReport/QualityUnavailable |
| `.../auditlog/base/summary_test.go` | unit (bagian 3a) |
| `.../auditlog/summary/params.go` | `ParseQuery` |
| `.../auditlog/summary/handler.go` | `Handler.Get` |
| `.../auditlog/summary/usecase.go` | `Usecase.Summarize` |
| `.../auditlog/summary/repository.go` | `sqlRepository.Load` (3 query, 1 tx RO RR), `busQualityReporter` |
| `.../auditlog/summary/init.go` | `Init(d, r)` |
| `.../auditlog/summary/params_test.go` | unit |
| `.../auditlog/summary/handler_test.go` | unit HTTP dengan usecase palsu |
| `.../auditlog/summary/usecase_test.go` | unit dengan repo/reporter palsu |
| `.../auditlog/summary/repository_test.go` | `//go:build integration`, Postgres tes (`db.TestDB`) |
| `.../auditlog/summary/explain_integration_test.go` | `//go:build integration`, EXPLAIN 1 juta baris (skip `-short`) |
| `.../auditlog/summary/http_integration_test.go` | `//go:build integration`, router + RequireRole + DB nyata + reporter palsu |
| `backend/cmd/internal/server/auditsummary_integration_test.go` | e2e server, kesetaraan CLI (AC8), bus gagal (AC9) |

### Backend (ubah)

| File | Perubahan |
|---|---|
| `backend/internal/domain/core/internal/auditlog/base.go` | import + `summary.Init(d, r)` di grup `/audit-log` |
| `backend/internal/domain/core/internal/auditlog/arch_test.go` | 3 rute GET, `/audit-log/summary` wajib ada |

Tidak berubah (hanya dibaca): `lib/contracts/core/audit/record/actions.go`, `lib/contracts/discovery/knowledgetools/searchquality/searchquality.go`, `lib/constants/bus.go`, `infra/db/tx.go`.

### API

| File | Perubahan |
|---|---|
| `api/modules/core/audit-log.yml` | path `/api/audit-log/summary`, schema `AuditSummary*`, `info.version` 0.2.0 |
| `api/root.yml` | `$ref` path + schema |
| `api/openapi.yml` | hasil `make api-bundle` |

### Frontend

| File | Perubahan |
|---|---|
| `frontend/package.json`, `package-lock.json` | `"chart.js": "<versi pasti>"` |
| `frontend/src/domain/core/audit-log/summary/types.ts` | Zod schema + tipe `AuditSummary`, `SummaryRange`, `SummaryQuery` |
| `.../summary/services/summary.service.ts` (+ `.test.ts`) | `getSummary(q, signal)` (GET `/api/audit-log/summary`, parse Zod) |
| `.../summary/services/projectOptions.service.ts` (+ `.test.ts`) | `listProjectOptions(q, signal)` (GET `/api/projects?limit=100[&q=]`, Zod minimal `{id,name}`) |
| `.../summary/stores/summary.store.ts` (+ `.test.ts`) | state `idle/loading/ready/error`, abort request lama, `reload()`, `retryQuality()` |
| `.../summary/stores/summaryQuery.ts` (+ `.test.ts`) | parse/serialize `/dashboard?range=&project=` |
| `.../summary/components/chartSetup.ts` | `Chart.register(...)` 9 komponen (K7), baca token CSS `--color-chart-*`/`--color-muted`/`--color-rule`, opsi dasar |
| `.../summary/components/ChartCard.svelte` | bingkai kartu: judul, status (skeleton/kosong/error), `<canvas aria-label>`, `sr-only` tabel data |
| `.../summary/components/{TopProjectsChart,ToolUsageChart,ToolStatusChart,ToolLatencyChart,TrafficChart}.svelte` | lima chart |
| `.../summary/components/configs.ts` (+ `.test.ts`) | fungsi murni data -> konfigurasi Chart.js (dites tanpa canvas) |
| `.../summary/components/SummaryFilterBar.svelte` | segmented 24 jam/7 hari/30 hari, pemilih project, tombol muat ulang |
| `.../summary/components/SummaryFigures.svelte` | strip dua angka (total panggilan, error rate) |
| `.../summary/components/SearchQualityBlock.svelte` | 3 rasio + bar rank; ajakan pilih project; error + coba lagi |
| `.../summary/components/components.test.ts` | tes komponen |
| `.../summary/mocks/summary.ts` | data contoh (bagian 9.4) |
| `.../summary/views/SummaryView.svelte` | komposisi isi halaman Dashboard |
| `.../summary/index.ts` | `export { default as SummaryView }` dan tipe |
| `frontend/src/app/routes/index.ts` | route `/dashboard`, meta `{ auth: true, roles: ['super_admin'], layout: 'app', titleKey: 'dashboard.title' }` |
| `frontend/src/app/views/DashboardView.svelte` (+ `.test.ts`) | baru: `PageHeader` "Dashboard" + `SummaryView` via `import()` |
| `frontend/src/app/layouts/AppLayout.svelte` | item nav "Dashboard" (ikon `chart`) di blok super_admin |
| `frontend/src/app/views/app.test.ts` | item nav dan guard `/dashboard` |
| `frontend/src/domain/core/audit-log/listentries/components/AuditTable.svelte` (+ `components.test.ts`) | klik baris (K9) |
| `frontend/src/shared/i18n/{id,en}.json` | kunci `dashboard.*` |
| `frontend/e2e/audit-log-summary.spec.ts` | e2e Playwright |

### Dokumen

`docs/mcp-gateway.md`, `docs/search-quality.md`, `docs/tickets/README.md` (lihat langkah h).

---

## 6. Test Scenario

Penamaan tes integrasi mengikuti pola `TestT15_ACn_*`. Tag `integration` untuk tes yang butuh Postgres.

### 6.1 Unit `base/summary_test.go`

Lihat langkah a.

### 6.2 Unit `summary/params_test.go`

- `range=%207d%20` menjadi `7d` (trim); `project_id` kosong atau spasi menjadi `""`; parameter tak dikenal (`foo=bar`, `action=...`) diabaikan; `range` berulang memakai nilai pertama.

### 6.3 Unit `summary/usecase_test.go` (repo dan reporter palsu, `now` tetap)

- Tanpa project: reporter tidak dipanggil, `SearchQuality == nil`, `Load` menerima window yang benar (`From = now-7d` saat range default).
- Dengan project: reporter dipanggil satu kali dengan `ProjectID` huruf kecil, `Since=From`, `Until=To`, `Window=0`; context reporter punya deadline ≤ 5s; hasil dipetakan `Available=true`.
- Reporter mengembalikan error (dan juga reporter yang memblok lebih dari timeout memakai `now`/ctx palsu): hasil `Available=false`, `ErrorCode="UNAVAILABLE"`, `Summarize` tidak error, log Warn tercatat tanpa field args.
- Repo error: dikembalikan dibungkus `summary:`, reporter tidak dipanggil.
- Urutan: reporter dipanggil setelah `Load` kembali (fake mencatat urutan).

### 6.4 Unit `summary/handler_test.go` (usecase palsu)

- `range=1d`, `range=90d`, `range=7D` menghasilkan 400 `INVALID_ARGUMENT`, `details.field=range`, dan usecase tidak dipanggil.
- `project_id=abc` menghasilkan 400 `details.field=project_id`.
- Usecase error menghasilkan 500 `INTERNAL` dengan pesan generik (tidak membocorkan error SQL).
- Sukses menghasilkan 200, `Content-Type: application/json`, kunci root tepat: `range, from, to, bucket, project_id, total_calls, total_errors, error_rate, top_projects, tool_usage, tool_metrics, traffic, search_quality`.

### 6.5 Integrasi `summary/repository_test.go` (Postgres tes, bukan mock; AC15)

Seed langsung `INSERT INTO audit_log` (pola `listentries/repository_test.go`), window tetap `w = Window{From: T0, To: T0+7d, Bucket: day}` dengan `T0` di awal hari agar bucket deterministik.

- **AC2**: project A 5x `mcp.read_file`, project B 2x `mcp.search_knowledge` menghasilkan `Projects` = [A:5, B:2]; `Tools` read_file 5, search_knowledge 2.
- **AC3**: tambahkan masing-masing 3 entri untuk `mcp.initialize`, `mcp.tools_list`, `mcp.auth.rejected` (status denied, project NULL), `mcp.rate_limited`, `mcp.unknown_tool`, dan juga `auth.login.success`. Hasil tidak berubah dari AC2.
- **AC4**: 10x `read_file` dengan `duration_ms` 10,20,...,100 dan 1 berstatus error menghasilkan ok 9, errors 1; p50 = 55, p95 = 95.5 (nilai `percentile_cont`). Tambahan entri `result = '{}'`, `result = '{"duration_ms":"abc"}'`, `result = '{"duration_ms":null}'`, dan `result` NULL (bila kolom nullable) tidak mengubah p50/p95 dan tidak error. Tool dengan semua entri tanpa angka menghasilkan p50/p95 NULL.
- **AC5**: window 24h (`Bucket hour`) dengan `From` tidak di awal jam menghasilkan 25 baris, sama dengan `w.BucketStarts()`, termasuk bucket kosong bernilai 0; 7d menghasilkan 7 baris saat `From` di awal hari; 30d menghasilkan 30/31; panggilan error terhitung di `errors` bucket yang benar.
- **AC6**: entri di `From - 1µs` dan tepat `To` tidak dihitung, sedangkan entri tepat `From` dihitung. Dengan `projectID=A`, hanya entri A yang dihitung di ketiga query.
- **Top 10**: 12 project dengan jumlah berbeda menghasilkan tepat 10, urut calls DESC; seri dipecah dengan `project_id ASC`.
- **Label**: project yang sama dengan label lama `"Payments"` lalu label baru `"Payments v2"` menghasilkan `"Payments v2"`. Entri `project_label` NULL menghasilkan `""` (fallback di entity). Project yang baris `projects`-nya tidak ada (tabel milik T03, tidak disentuh) tetap muncul.
- **Kosong**: project tanpa aktivitas menghasilkan Tools/Projects kosong, Traffic penuh nol, tanpa error.
- **Tx bersih**: setelah `Load` kembali (sukses, dan juga dengan ctx yang sudah dibatalkan), `d.Stats().InUse == 0`. Opsi `ReadOnly`/`RepeatableRead` diverifikasi lewat code review, tanpa tes khusus.

### 6.6 Integrasi `summary/explain_integration_test.go` (K3, skip `-short`)

- Seed 1 juta baris (pola `listentries/explain_integration_test.go`) yang tersebar 90 hari (`now() - g * interval '7.776 seconds'`), campuran aksi tool, gateway, dan non-MCP, serta 20 project; lalu `ANALYZE audit_log`.
- `EXPLAIN (FORMAT TEXT)` ketiga query untuk (1) 24h tanpa project dan (2) 30d dengan `project_id`. Assert: rencana memuat `audit_log_action_idx` atau `audit_log_project_idx` (Index Scan/Bitmap Index Scan) dan tidak memuat `Seq Scan on audit_log`.
- Kasus 30d tanpa project tidak di-assert (planner sah memilih Seq Scan). Sebagai gantinya, catat `EXPLAIN (ANALYZE, BUFFERS)` dan waktunya lewat `t.Logf`. Angka ini disalin ke deskripsi PR (risiko 1).

### 6.7 Integrasi `summary/http_integration_test.go`

Router (pola `getentry/http_integration_test.go`): `withIdentity` (header `X-Test-Role`), lalu `r.Route("/api/audit-log", ...)` dengan `RequireRole(super_admin)`, `Init(d, r)` milik summary, dan handler dummy `r.Get("/{id}", ...)` yang mengembalikan 418. `listentries`/`getentry` tidak di-import (rules 4.3). Handler dummy itu membuktikan `/summary` tidak jatuh ke `/{id}`. Deps: DB tes nyata (`db.TestDB`), `Log`, dan `Bus` = `bus.NewRegistry` dengan handler palsu untuk `BUS_DISCOVERY_KNOWLEDGETOOLS_SEARCH_QUALITY_REPORT` (bisa diganti per tes: sukses, error, atau tidak didaftarkan).

- **AC1/otorisasi**: tanpa identitas menghasilkan 401 `UNAUTHORIZED`; role `developer` menghasilkan 403 `FORBIDDEN`; `super_admin` menghasilkan 200.
- **Rute**: `GET /api/audit-log/summary` tidak pernah 418; `POST/PUT/DELETE /api/audit-log/summary` menghasilkan 405.
- **AC7**: `range=1d` menghasilkan 400; `project_id=xyz` menghasilkan 400; project uuid acak tanpa aktivitas menghasilkan 200 dengan `total_calls=0`, `error_rate=null`, `top_projects=[]`, `tool_usage=[]`, `tool_metrics=[]`, traffic penuh nol, dan `search_quality.available=true` (dari handler palsu).
- **AC8 (bentuk)**: dengan project, `search_quality` sama dengan `Report` palsu yang dikembalikan handler bus; tanpa project menghasilkan `search_quality: null` dan handler bus tidak dipanggil.
- **AC9**: handler bus mengembalikan error menghasilkan 200, `search_quality = {"available":false,"error_code":"UNAVAILABLE"}`, dan field lain terisi normal. Bus tanpa handler terdaftar (`ErrNotRegistered`) memberi hasil sama.
- **AC10**: seed entri tool dengan `args = {"query":"rahasia-query","path":"docs/secret.md"}`, `target = "docs/secret.md"`, `result = {"result_paths":["docs/secret.md"],"duration_ms":5}`. Assert: body mentah tidak mengandung `rahasia-query`, `docs/secret.md`, `result_paths`; walk rekursif JSON tidak punya kunci `args`, `result`, `target`, `query`, `path`, `ip`, `user_agent`, `trace_id` (kecuali `trace_id` pada error, yang memang bukan jalur 200).
- **Tidak ada audit**: jumlah baris `audit_log` sebelum dan sesudah request sama.

### 6.8 Arsitektur

- `auditlog/arch_test.go` (langkah e).
- `backend/internal/archtest` (sudah ada) harus tetap hijau. Feature `summary` hanya meng-import `infra/{app,bus,db,http/respond}`, `lib/{constants,contracts/...,common/errs}`, dan `auditlog/base`.

### 6.9 Frontend unit (Vitest)

Lihat bagian 9.6.

---

## 7. E2E Scenarios

### 7.1 Backend server (`cmd/internal/server/auditsummary_integration_test.go`, binary nyata via `startRun`)

Positif:
1. **TestT15_AC8_EqualsCLI**: seed data T11 (search + read file, memakai helper `sqrEnv.search/read`) untuk project P dalam 2 jam terakhir. Panggil `GET /api/audit-log/summary?range=24h&project_id=P` sebagai super_admin melalui server yang berjalan: buat super admin dan login dengan helper yang sudah dipakai tes `cmd/internal/server` lain (cari `create-admin`/`login` di `*_integration_test.go` package ini dan pakai ulang, jangan membuat helper baru bila sudah ada). Ambil `from`/`to` dari respons; jalankan `grimoire search-quality report --project P --since <from> --until <to> --json` (pola `exec` di file T11) lalu bandingkan field demi field: `searches`, `zero_result`, `follow_up`, `reformulation`, `rank_opened` (11 bucket, urutan sama), `legacy_searches`, `window_seconds` (=120).
2. **TestT15_AC2_AC3_EndToEnd**: seed tool dan gateway untuk dua project; assert `top_projects`, `tool_usage`, `total_calls` mengecualikan 5 aksi gateway.

Negatif:
3. **AC9 (bus gagal)** tidak diuji di level server. Handler T11 pada binary nyata tidak bisa diganti, karena bus mem-panic pada registrasi ganda. Kegagalan bus diuji di `http_integration_test.go` (6.7), memakai registry tes dengan handler error dan tanpa handler, serta di Playwright no.7. Catat "AC9 dicakup 6.7 + 7.2 no.7" di PR.
4. **TestT15_AC7_Validation**: `range=12h` menghasilkan 400 envelope `INVALID_ARGUMENT` dengan `details.field=range`; `project_id=not-uuid` menghasilkan 400; tanpa sesi menghasilkan 401; sesi developer menghasilkan 403.

### 7.2 Frontend Playwright (`frontend/e2e/audit-log-summary.spec.ts`, terhadap binary sungguhan, AC15)

Positif:
1. Super admin mengklik item nav "Dashboard" di sidebar: URL `/dashboard`, item itu `aria-current=page`, h1 "Dashboard", rentang "7 hari" terpilih, lima kartu chart punya `aria-label`, blok kualitas pencarian menampilkan ajakan pilih project (AC1, AC8).
2. Klik "24 jam": URL menjadi `/dashboard?range=24h`, request `/api/audit-log/summary?range=24h` terkirim, sumbu trafik per jam. Reload halaman mempertahankan rentang (AC11).
3. Pilih project dari pemilih: URL `project=<id>`, chart Project teratas disembunyikan, blok kualitas pencarian menampilkan tiga rasio dan bar rank (AC8).
4. Halaman Audit Log: klik sel aktor pada sebuah baris membuka panel detail; tutup dengan Escape/tombol, fokus kembali ke tombol waktu baris itu; Tab ke tombol waktu + Enter membuka detail (AC14).
5. Jaringan: saat membuka `/` dan `/audit-log` tidak ada request chunk yang memuat `chart` (cek `page.on('request')` terhadap chunk `summary`/`chart.js` dari manifest); chunk itu baru diminta setelah membuka `/dashboard` (AC13).

Negatif:
6. Sesi developer: item nav "Dashboard" tidak ada; membuka `/dashboard` langsung diperlakukan sama seperti `/audit-log` untuk non-super-admin (guard `notfound`, heading "Dashboard" tidak ada), dan tidak ada request ke `/api/audit-log/summary` (AC1).
7. `page.route('/api/audit-log/summary*')` dengan respons `search_quality:{available:false,error_code:"UNAVAILABLE"}`: chart lain tampil, blok kualitas menampilkan pesan error + tombol "Coba lagi"; klik tombol mengirim ulang request (AC9).
8. Respons 500 untuk summary menampilkan satu error di strip angka dengan tombol coba lagi (kartu chart "Data tidak tersedia", lihat FE-0 no.3), dan halaman Audit Log tetap berfungsi.
9. Drag-select teks target di halaman Audit Log tidak membuka detail (AC14).

Rincian lengkap dan nomor final ada di FE-9.

---

## 8. Request/Response body

### 8.1 Request

```
GET /api/audit-log/summary?range=7d&project_id=0192f5a0-1111-7aaa-8bbb-000000000001
Cookie: <sesi super_admin>
```

Tanpa body. `range` opsional (default `7d`). `project_id` opsional.

### 8.2 200: tanpa project

```json
{
  "range": "7d",
  "from": "2026-10-15T10:30:15Z",
  "to": "2026-10-22T10:30:15Z",
  "bucket": "day",
  "project_id": null,
  "total_calls": 1234,
  "total_errors": 31,
  "error_rate": 0.0251,
  "top_projects": [
    { "project_id": "0192f5a0-1111-7aaa-8bbb-000000000001", "project_label": "Payments", "calls": 420 },
    { "project_id": "0192f5a0-1111-7aaa-8bbb-000000000002", "project_label": "Onboarding Mobile", "calls": 311 }
  ],
  "tool_usage": [
    { "tool": "read_file", "calls": 600 },
    { "tool": "search_knowledge", "calls": 402 },
    { "tool": "list_files", "calls": 150 },
    { "tool": "get_api_endpoint", "calls": 82 }
  ],
  "tool_metrics": [
    { "tool": "read_file", "calls": 600, "ok": 590, "error": 10, "error_rate": 0.0167, "p50_ms": 8, "p95_ms": 40 },
    { "tool": "search_knowledge", "calls": 402, "ok": 386, "error": 16, "error_rate": 0.0398, "p50_ms": 35.5, "p95_ms": 182 },
    { "tool": "list_files", "calls": 150, "ok": 150, "error": 0, "error_rate": 0, "p50_ms": 4, "p95_ms": 11 },
    { "tool": "get_api_endpoint", "calls": 82, "ok": 77, "error": 5, "error_rate": 0.061, "p50_ms": null, "p95_ms": null }
  ],
  "traffic": [
    { "at": "2026-10-15T00:00:00Z", "calls": 90, "errors": 2 },
    { "at": "2026-10-16T00:00:00Z", "calls": 0, "errors": 0 },
    { "at": "2026-10-17T00:00:00Z", "calls": 210, "errors": 5 },
    { "at": "2026-10-18T00:00:00Z", "calls": 188, "errors": 3 },
    { "at": "2026-10-19T00:00:00Z", "calls": 175, "errors": 4 },
    { "at": "2026-10-20T00:00:00Z", "calls": 240, "errors": 9 },
    { "at": "2026-10-21T00:00:00Z", "calls": 221, "errors": 6 },
    { "at": "2026-10-22T00:00:00Z", "calls": 110, "errors": 2 }
  ],
  "search_quality": null
}
```

Catatan: `total_calls` = jumlah `tool_metrics[].calls`, dan `traffic` berisi 8 bucket karena `from` tidak tepat di awal hari. Contoh `error_rate` dibulatkan untuk dokumen; server tidak membulatkan rasio (hanya p50/p95 2 desimal).

### 8.3 200: dengan project, T11 berhasil

```json
{
  "range": "24h",
  "from": "2026-10-21T10:30:15Z",
  "to": "2026-10-22T10:30:15Z",
  "bucket": "hour",
  "project_id": "0192f5a0-1111-7aaa-8bbb-000000000001",
  "total_calls": 65,
  "total_errors": 1,
  "error_rate": 0.0154,
  "top_projects": [
    { "project_id": "0192f5a0-1111-7aaa-8bbb-000000000001", "project_label": "Payments", "calls": 65 }
  ],
  "tool_usage": [ { "tool": "search_knowledge", "calls": 60 }, { "tool": "read_file", "calls": 5 } ],
  "tool_metrics": [
    { "tool": "search_knowledge", "calls": 60, "ok": 59, "error": 1, "error_rate": 0.0167, "p50_ms": 31, "p95_ms": 140 },
    { "tool": "read_file", "calls": 5, "ok": 5, "error": 0, "error_rate": 0, "p50_ms": 6, "p95_ms": 9.8 }
  ],
  "traffic": [
    { "at": "2026-10-21T10:00:00Z", "calls": 2, "errors": 0 },
    { "at": "2026-10-21T11:00:00Z", "calls": 0, "errors": 0 }
  ],
  "search_quality": {
    "available": true,
    "searches": 59,
    "window_seconds": 120,
    "zero_result": { "count": 4, "denominator": 59, "rate": 0.0678 },
    "follow_up": { "count": 31, "denominator": 55, "rate": 0.5636 },
    "reformulation": { "count": 7, "denominator": 59, "rate": 0.1186 },
    "rank_opened": [
      { "label": "1", "count": 18 }, { "label": "2", "count": 6 }, { "label": "3", "count": 3 },
      { "label": "4", "count": 1 }, { "label": "5", "count": 1 }, { "label": "6", "count": 0 },
      { "label": "7", "count": 0 }, { "label": "8", "count": 0 }, { "label": "9", "count": 0 },
      { "label": "10", "count": 0 }, { "label": "not_in_results", "count": 2 }
    ],
    "legacy_searches": 0
  }
}
```

(`traffic` dipotong di contoh ini; respons nyata berisi 25 bucket.)

### 8.4 200: dengan project, T11 gagal (AC9)

Sama seperti 8.3, kecuali:

```json
{ "search_quality": { "available": false, "error_code": "UNAVAILABLE" } }
```

### 8.5 200: project tanpa aktivitas (AC7)

```json
{
  "range": "7d", "from": "2026-10-15T10:30:15Z", "to": "2026-10-22T10:30:15Z", "bucket": "day",
  "project_id": "0192f5a0-1111-7aaa-8bbb-0000000000ff",
  "total_calls": 0, "total_errors": 0, "error_rate": null,
  "top_projects": [], "tool_usage": [], "tool_metrics": [],
  "traffic": [ { "at": "2026-10-15T00:00:00Z", "calls": 0, "errors": 0 } ],
  "search_quality": {
    "available": true, "searches": 0, "window_seconds": 120,
    "zero_result": { "count": 0, "denominator": 0, "rate": 0 },
    "follow_up": { "count": 0, "denominator": 0, "rate": 0 },
    "reformulation": { "count": 0, "denominator": 0, "rate": 0 },
    "rank_opened": [ { "label": "1", "count": 0 } ],
    "legacy_searches": 0
  }
}
```

(`traffic` dan `rank_opened` dipotong; nyatanya 8 bucket dan 11 bucket.)

### 8.6 Error (format seragam `foundation.yml#/components/schemas/Error`)

400, range tidak valid:

```json
{
  "error": { "code": "INVALID_ARGUMENT", "message": "range tidak valid", "details": { "field": "range" } },
  "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33"
}
```

400, project_id bukan uuid:

```json
{
  "error": { "code": "INVALID_ARGUMENT", "message": "project_id tidak valid", "details": { "field": "project_id" } },
  "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33"
}
```

401:

```json
{ "error": { "code": "UNAUTHORIZED", "message": "perlu login", "details": null }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```

403 (bukan super_admin):

```json
{ "error": { "code": "FORBIDDEN", "message": "akses ditolak", "details": null }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```

500 (SQL gagal; pesan generik, tanpa detail SQL):

```json
{ "error": { "code": "INTERNAL", "message": "terjadi kesalahan internal", "details": null }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```

(Teks pesan 401/403/500 mengikuti `respond.WriteError` yang sudah ada; jangan membuat teks baru.)

### 8.7 Kontrak bus yang dipakai (tidak berubah)

```go
// consumer: auditlog/summary/repository.go
bus.CallTyped[searchquality.Response](ctx5s, d.Bus,
	constants.BUS_DISCOVERY_KNOWLEDGETOOLS_SEARCH_QUALITY_REPORT,
	searchquality.Request{ProjectID: q.ProjectID, Since: w.From, Until: w.To /* Window: 0 -> 120s */})
// error apa pun (ErrNotRegistered, errs.ErrInvalid, DeadlineExceeded, ...) -> search_quality {available:false}
```

---

## 9. Frontend

### 9.1 Struktur dan aliran

- Route `/dashboard` (`app/routes/index.ts`, meta `{ auth: true, roles: ['super_admin'], layout: 'app', titleKey: 'dashboard.title' }`) merender `app/views/DashboardView.svelte`. View ini di-import statis oleh tabel route (ukurannya kecil), lalu merender `PageHeader` "Dashboard" dan `{#await import('../../domain/core/audit-log/summary')}` dengan Spinner selama chunk dimuat. Import dinamis ini memisahkan chunk Chart.js dari halaman lain. `index.ts` summary di-import dari `app/` (sesuai depcruise `outside-domain-only-via-index`). Karena `DashboardView` hanya dirender setelah guard mengizinkan, sesi non-super-admin tidak pernah memicu `import()` maupun request ke `/api/audit-log/summary`. `AuditLogView.svelte` tidak berubah.
- Item nav "Dashboard" ditambahkan di `app/layouts/AppLayout.svelte` di blok `role === 'super_admin'` (sebelum Users dan Audit Log), dengan cabang ikon `chart` baru. `isActive` yang ada (`path === href`) sudah cukup untuk `/dashboard`.
- `summaryQuery.ts`: `parse(q) -> { range: '24h'|'7d'|'30d' (default '7d', nilai asing -> '7d'), projectId: string | null }`; `serialize` selalu menulis `range` (URL eksplisit, termasuk default `7d`) dan `project` bila ada, tanpa kunci lain. Perubahan filter memakai `navigate('/dashboard?' + q, { replace: true })`.
- `summary.service.ts`: `get('/api/audit-log/summary', { params: { range, project_id } , signal })`, parse dengan `AuditSummarySchema` (Zod, `.strict()` di root agar kunci tak terduga terdeteksi di tes), `search_quality: z.union([z.null(), AvailableSchema, UnavailableSchema])`.
- `projectOptions.service.ts`: memanggil endpoint T03 `GET /api/projects?limit=100` sekali. Bila `total > 100`, pemilih menjadi input pencarian dengan debounce 250ms (`q=`). Schema Zod minimal `{ items: [{ id, name }], total }` (`.passthrough()` pada item, karena itu milik module lain). Bila project di URL tidak ada di daftar (project dihapus), label diambil dari `top_projects[0].project_label` atau ditampilkan id-nya.
- `summary.store.ts`: `load(query)` membatalkan request sebelumnya (`AbortController`), state `{ status, data, error }`. `reload()` untuk tombol muat ulang. `retryQuality()` memanggil ulang endpoint yang sama (tidak ada endpoint terpisah) dan hanya blok kualitas yang menampilkan memuat; chart lain mempertahankan data lama sampai respons tiba.
- `chartSetup.ts`: `Chart.register(BarController, LineController, BarElement, LineElement, PointElement, CategoryScale, LinearScale, Tooltip, Legend)`. Tidak memakai `chart.js/auto`. Warna dibaca dari `getComputedStyle(document.documentElement)` (`--color-chart-1`, `--color-chart-2`, `--color-muted`, `--color-rule`, `--color-fg`, `--color-danger`). Chart di-update saat `data-theme` berubah (MutationObserver pada `<html>`, dilepas di `onDestroy`). Hormati `prefers-reduced-motion`: `animation: false` selalu, karena gerak hanya untuk umpan balik.
- `configs.ts` (fungsi murni, dites): `topProjectsConfig`, `toolUsageConfig`, `toolStatusConfig`, `toolLatencyConfig`, `trafficConfig`, `rankConfig`. Label sumbu panjang dipotong 24 karakter dengan `…`; nama lengkap ada di tooltip.
- `ChartCard.svelte`: props `title`, `status: 'loading'|'empty'|'error'|'ready'`, `ariaLabel`, `table: { columns, rows }`, `onretry`. Render `<figure>`: `<figcaption>` judul 16px; saat loading tampil skeleton blok abu `bg-raised` setinggi plot (tanpa shimmer); saat kosong tampil teks "Belum ada panggilan MCP pada rentang ini" di dalam bingkai yang sama; saat error tampil `ErrorState` + coba lagi; saat siap tampil `<canvas role="img" aria-label=...>` + `<table class="sr-only">` dengan angka yang sama (K8).

### 9.2 Isi halaman Dashboard (urutan atas ke bawah)

0. Title row `PageHeader`: h1 "Dashboard" + satu kalimat redup tentang data ("Pemakaian tool MCP oleh agent, dari audit log").
1. Baris filter (urutan app-screens 4.4: cakupan kiri, utilitas kanan): segmented "24 jam | 7 hari | 30 hari" (`role="radiogroup"`, tinggi 32), pemilih project "Semua project", lalu di kanan tombol sekunder "Muat ulang" + teks redup "Data s.d. 10.30 (WIB)" dari `to`.
2. Strip angka (satu kontainer dipisah garis 1px, bukan ubin per angka): "Panggilan tool" `1.234` (28px weight 400) dan "Error rate" `2,5%` dengan sufiks redup "31 error". `error_rate` null ditampilkan `–` abu.
3. Grid dua kolom di ≥1024px, satu kolom di bawahnya:
   - Trafik (lebar penuh, baris pertama karena tiket meminta pergerakan waktu): line `chart-1` untuk panggilan, line kedua untuk error berwarna `danger` putus-putus 4/3 (seri dibedakan warna + gaya garis + legenda bertitik di header kartu). Sumbu x memakai tanggal/jam lokal browser dengan label zona waktu di caption kartu ("Waktu: WIB (UTC+7); bucket dihitung UTC").
   - Project teratas: bar horizontal urut, `chart-1`, angka di ujung bar (label langsung). Disembunyikan bila project dipilih.
   - Tool paling sering dipakai: bar horizontal urut, `chart-1`, angka langsung.
   - Status per tool: stacked bar horizontal ok (`chart-1`) / error (`danger` dengan pola garis diagonal lewat `CanvasPattern`), seam 2px; legenda "ok", "error" dengan swatch yang sama.
   - Latensi per tool: grouped bar p50 (`chart-1`) / p95 (`chart-2`), satuan "ms" di sumbu; tool dengan `null` ditulis "–" di tabel sr-only dan bar-nya kosong (bukan 0).
4. Blok Kualitas pencarian (lebar penuh): tiga angka (Zero-result, Follow-up, Reformulasi) dalam format `6,8%` dengan sufiks redup `4/59`, teks bantuan "Perkiraan tren, bukan angka mutlak (lihat docs/search-quality.md)", dan bar kolom distribusi rank `1..10, tidak di hasil` (`chart-1`, `not_in_results` abu `muted`). Tanpa project: `EmptyState` "Pilih project untuk melihat kualitas pencarian" + tombol sekunder yang memfokuskan pemilih project. `available=false`: teks "Kualitas pencarian tidak dapat dimuat." + tombol "Coba lagi".

### 9.3 Visual

| Layar | Pertanyaan | Pola DESIGN.md | Blok terbesar yang menjawab |
|---|---|---|---|
| `/dashboard` | Seberapa banyak agent memakai MCP, lewat tool apa, dan di mana gagal atau lambat? | Pola baru "ringkasan analitik" (usulan DESIGN.md di FE-11): title row `PageHeader` tanpa tab; sheet putih, kartu chart radius 8 berborder `border-border` tanpa bayangan lebih tebal dari sheet | Chart Trafik (lebar penuh, paling atas) |
| `/audit-log` | Siapa (manusia atau agent) melakukan apa, kapan? | Tidak berubah, hanya baris menjadi target klik | Tabel |

- **Wajah produk yang dipakai**: mono (IBM Plex Mono) untuk nama tool di sumbu dan tabel sr-only (`read_file`, `search_knowledge`), karena nama itu dibaca mesin. Nama project memakai sans. Aksen kuning hanya pada seri data utama (`chart-1`) dan chip agent; tidak ada kuning di tombol atau latar kartu. Teal `chart-3` tidak dipakai (tidak ada seri ketiga). Status error memakai `danger` + pola + teks, tidak hanya warna.
- **Elemen khas**: judul kartu memakai "jejak agent" ringan, yaitu chip kotak kuning pucat dengan ikon agent di samping judul "Panggilan tool" pada strip angka, untuk menegaskan bahwa semua angka di halaman ini berasal dari agent (actor `api_key`). Chip yang sama dipakai tabel audit, tanpa chip baru.
- **Token**: `bg-surface`, `border-border`, `rounded-card`, `text-muted` untuk label dan tick, gridline `--color-rule` 1px horizontal saja (3-5 garis), tanpa garis sumbu y, tick 12px, `tabular-nums`, `chart-1`/`chart-2`, `text-danger`, `rounded-base` untuk segmented. Gap 24, padding kartu 24, tinggi plot 200px (trafik 220px), bar sudut siku.
- **Aturan yang dicek**: tanpa gradien/glow, tanpa sparkline di KPI, tanpa chart pelangi (maks 2 hue + danger), tanpa animasi masuk, legenda hanya untuk chart multi-seri (status, latensi, trafik), satu tombol utama per layar (tidak ada; "Muat ulang" sekunder), nol ditulis `0`, nilai hilang `–`.
- **Ponsel** (<640px): filter menjadi dua baris (segmented selebar penuh di bawah 480px), chart satu kolom, label sumbu dipotong dengan `title`, tabel sr-only tetap.
- **State per kartu**: memuat (skeleton statis), kosong (teks di bingkai yang sama), error (ErrorState + coba lagi), siap.

### 9.4 Data contoh (mocks/summary.ts)

Memakai kata produk: project "Payments", "Onboarding Mobile", "Ledger Core", "Partner Portal"; tool `read_file`, `search_knowledge`, `list_files`, `get_api_endpoint`, `list_api_endpoints`, `list_api_specs`; label key `nbk_4f9a…`. Angka konsisten dengan contoh 8.2 (`total_calls` dihitung dari `tool_metrics`, traffic dijumlah sama dengan total). Ada juga satu mock `empty`, satu `qualityUnavailable`, dan satu `withProject` (8.3). Dilarang Acme, lorem, atau delta "+12%".

### 9.5 Klik baris `AuditTable` (K9, AC14)

- `<tr>` mendapat `onclick={(e) => rowClick(e, r.id)}` dan kelas `cursor-pointer`. Tidak ada `role="button"`, tidak ada `tabindex` pada `<tr>`.
- `rowClick`: abaikan bila `window.getSelection()?.toString()` tidak kosong dan seleksinya berada di dalam baris (drag-select); abaikan bila `e.target.closest('a,button,input,select,textarea')` bukan tombol waktu baris itu, agar klik pada tombol waktu tidak memicu dua kali (tombol waktu tetap punya `onclick` sendiri, sedangkan handler baris melewatkan event dari tombol itu). Panggil `onselect(id)`.
- Tombol waktu tetap target fokus keyboard (`data-entry-id`), sehingga pengembalian fokus di `AuditLogView.onclose` tidak berubah.

### 9.6 Tes frontend (Vitest + Testing Library)

- `summary.service.test.ts`: URL dan query benar (`range`, `project_id` hanya bila ada); respons valid lolos parse; kunci tambahan `args` di root ditolak Zod (penjaga AC10); `search_quality` tiga bentuk; 400/403 diteruskan sebagai `ApiError`.
- `projectOptions.service.test.ts`: `limit=100`, `q` saat mencari, schema minimal.
- `summaryQuery.test.ts`: default `7d`, nilai asing menjadi `7d`, round-trip serialize, kunci lain dibuang.
- `summary.store.test.ts`: abort request lama saat filter berubah cepat; error per status; `retryQuality` hanya mengubah status blok kualitas.
- `configs.test.ts` (AC15 "data masuk, konfigurasi Chart.js benar"): label dan data sesuai input; seri error memakai pola dan `borderDash`; `p50_ms` null menjadi `null` di dataset (bukan 0); trafik memakai label waktu lokal; jumlah titik = jumlah bucket.
- `components.test.ts`: `ChartCard` empat state; `aria-label` ada; tabel sr-only berisi angka sama; `SearchQualityBlock` tiga keadaan (null, available, unavailable + klik "Coba lagi" memanggil callback); top project disembunyikan saat project dipilih. Chart.js di-mock (`vi.mock('chart.js')`) untuk memastikan `register` dipanggil dengan 9 komponen.
- `DashboardView.test.ts`: h1 "Dashboard"; setelah `import()` selesai `SummaryView` tampil; perubahan filter menavigasi ke `/dashboard?range=...` dengan `replace`.
- `app.test.ts`: item nav "Dashboard" hanya untuk super_admin; developer di `/dashboard` mendapat not found tanpa fetch summary.
- `listentries/components/components.test.ts` (`AuditTable`): klik sel aktor memanggil `onselect(id)` sekali; klik tombol waktu memanggil sekali (bukan dua); seleksi teks aktif tidak memanggil; tombol waktu tetap fokusable.
- Ukuran bundle (AC13): setelah `npm run build`, skrip tes kecil (atau langkah CI yang ada) memastikan chunk entri `index-*.js` tidak memuat string `"chart.js"`/`BarController`; tercakup juga oleh e2e 7.2 no.5.
- Jalankan `npm run depcruise`, `npm run test:arch`, dan `node .agents/skills/design-taste-frontend/scripts/lint.mjs --app frontend/src` pada file yang diubah.

### 9.7 i18n (`frontend/src/shared/i18n/{id,en}.json`)

`dashboard.title` "Dashboard" / "Dashboard", `dashboard.menu` "Dashboard" / "Dashboard", `dashboard.subtitle`, `dashboard.range.{24h,7d,30d}` "24 jam/7 hari/30 hari", `dashboard.allProjects` "Semua project", `dashboard.reload` "Muat ulang", `dashboard.asOf` "Data s.d. {time}", `dashboard.figures.{calls,errorRate,errors}`, `dashboard.charts.{traffic,topProjects,toolUsage,toolStatus,toolLatency}` (+ `.aria` dengan placeholder angka), `dashboard.empty` "Belum ada panggilan MCP pada rentang ini", `dashboard.timezone` "Waktu: {tz}; bucket dihitung UTC", `dashboard.quality.{title,zeroResult,followUp,reformulation,rank,notInResults,pickProject,pickProjectAction,unavailable,retry,hint}`. Tidak ada kunci `auditLog.tabs.*` atau `auditLog.summary.*`. Teks bahasa Indonesia, sentence case. Daftar final ada di FE-3 no.9.

---

## 10. Risiko dan catatan untuk developer

1. **Performa tabel besar.** 30d tanpa project bisa mencakup sekitar sepertiga isi tabel (retensi 90 hari), sehingga Seq Scan/Bitmap Scan sah dipakai planner. `percentile_cont` juga membaca heap (`result` jsonb). Ukur dengan 1 juta baris (6.6) dan catat di PR. Indeks parsial ditunda ke tiket lanjutan.
2. **EXPLAIN flaky.** Assert hanya kasus selektif (24h, dan 30d + project) setelah `ANALYZE`, dengan data jauh lebih luas dari window.
3. **Timeout bus 5s** menambah latensi terburuk bila T11 lambat (T11 memindai entri via `core.audit.scan-entries` berhalaman 1000). Kondisi ini diterima karena fail-soft dan hanya terjadi bila project diisi.
4. **Snapshot** hanya untuk 3 query SQL. Laporan T11 memakai koneksi sendiri, jadi didokumentasikan (langkah h).
5. **Tool MCP baru** yang tidak masuk `record.Actions()` tidak terhitung. Aturan README "Aksi baru" dan tes kelengkapan (3a) menjaganya.
6. **AC8** bergantung pada `Window=0` (120s) yang sama dengan default CLI. Tes 7.1 no.1 akan menangkap perubahan default.
7. **arch_test** merakit dengan `DB` nil. `summary.Init`/`NewRepository`/`NewQualityReporter` tidak boleh mendereferensi saat dirakit.
8. **Kebocoran data.** Repository hanya menyeleksi kolom agregat; tes HTTP 6.7 (AC10) dan Zod `.strict()` di FE menjaganya.
9. **Transaksi**: selalu `defer tx.Rollback()`; jangan memanggil bus selama tx terbuka (koneksi pool tertahan dan T11 butuh koneksi lain).
10. **Pertanyaan terbuka tiket** (auto-refresh, perbandingan periode, tabel agregat harian, key paling aktif) tidak dikerjakan. Cukup tombol muat ulang.

---

## Frontend Plan

Bagian ini adalah rujukan implementasi frontend T15. Isinya merinci bagian 9 dan **menggantikannya** pada tiga titik yang bertentangan dengan kode yang ada (lihat FE-0). Bagian backend (1 sampai 8, 10) tidak berubah. Halaman target adalah `/dashboard` (route baru, khusus super_admin); rancangan tab di halaman Audit Log dibatalkan dan tidak ada sisa pekerjaan untuk `AuditLogView`, `ListEntriesView`, atau tab.

Konteks tambahan yang dicek untuk bagian ini: `frontend/src/app/views/AuditLogView.svelte` dan `.test.ts` (tidak diubah; dicek untuk pola `urlFor`/`onquerychange` dan pengembalian fokus), `app/routes/index.ts` (route `/audit-log` dan `/admin/users` dengan `roles: ['super_admin']`; route di-import statis), `app/routes/guard.ts` (role tidak cocok menghasilkan `{ kind: 'notfound' }`), `app/layouts/AppLayout.svelte` (array `links`: Project untuk semua, lalu Users dan Audit Log hanya bila `$session.identity?.role === 'super_admin'`; ikon outline 16px per cabang `l.icon`; `isActive` = `path === href`), `app/views/app.test.ts` (tes "users menu and route only for super_admin"), `app/views/UsersView.svelte` (pola view app tipis yang merender view feature), `domain/core/audit-log/listentries/{types.ts,index.ts,views/ListEntriesView.svelte,components/AuditTable.svelte,components/ActorIcon.svelte,stores/filterQuery.ts,mocks/entries.ts}`, `shared/components/{PageHeader,EmptyState,ErrorState,Select,Button,Spinner}.svelte`, `shared/services/http.ts` (`get(url, { query, schema, signal })`), `shared/stores/{theme,i18n}.ts`, `shared/i18n/id.json` (kunci datar `"auditLog.x.y"`), `frontend/.dependency-cruiser.cjs`, `frontend/package.json`, `vite.config.ts`, `playwright.config.ts`, `e2e/audit-log.spec.ts`, `e2e/helpers/{auth,projects,apikeys,knowledge-tools}.ts`, `api/modules/project/management.yml` (`GET /api/projects`, super_admin melihat semua project).

### FE-0. Koreksi terhadap bagian 9

| No | Bagian 9 menulis | Fakta di kode | Keputusan FE |
|---|---|---|---|
| 1 | Zod `AuditSummarySchema`, tipe `AuditSummary` | `listentries/types.ts` sudah mengekspor `AuditSummary`/`AuditSummarySchema` untuk **baris** entri, dan `listentries/index.ts` mengekspornya | Nama di feature `summary`: `UsageSummary`, `UsageSummarySchema`. Kata "summary" untuk baris entri tidak disentuh. |
| 2 | Garis error trafik putus-putus 4/3 | `app-screens.md` 6.3/6.1: garis putus-putus hanya untuk grid dan prakiraan | Garis error padat 1.5px `danger`, garis panggilan 2px `chart-1`. Seri dibedakan lewat legenda + tooltip + tabel `sr-only` (opsi yang diizinkan anti-tujuan tiket). |
| 3 | `ErrorState` + coba lagi di **setiap** kartu bila endpoint gagal | Satu endpoint, jadi lima tombol "Coba lagi" merah yang sama hanyalah noise (`app-screens.md` 11 "Red is a signal") | Error ditampilkan sekali di strip angka (kalimat + "Coba lagi"). Kartu chart tetap punya status `error` sendiri: bingkai dan judul tetap, plus kalimat abu "Data tidak tersedia". Pengecualian: blok kualitas pencarian punya error dan tombol sendiri (AC9). |

### FE-1. Metadata

| Atribut | Nilai |
|---|---|
| Scope | frontend (bagian dari fullstack T15) |
| Domain / Module / Feature | core / audit-log / `summary` (baru), `listentries` (ubah: klik baris saja); `app/` (route, `DashboardView`, nav) |
| Route | `/dashboard?range=24h\|7d\|30d&project=<uuid>` (baru), meta `{ auth: true, roles: ['super_admin'], layout: 'app', titleKey: 'dashboard.title' }`, guard sama dengan `/audit-log`. `/audit-log` dan `/` tidak berubah |
| Navigasi | item "Dashboard" di sidebar `AppLayout`, hanya untuk super_admin, sebelum Users dan Audit Log |
| Dependensi baru | `chart.js` (versi pasti, lihat FE-3 langkah 1) |
| Migrasi | tidak ada |

| Endpoint | Method | Version | Dipakai oleh |
|---|---|---|---|
| `/api/audit-log/summary` | GET | `api/modules/core/audit-log.yml` 0.2.0 (baru di T15) | `summary/services/summary.service.ts` |
| `/api/projects?limit=100` | GET | `api/modules/project/management.yml` 0.2.0 (tidak berubah) | `summary/services/projectOptions.service.ts` |
| `/api/audit-log`, `/api/audit-log/{id}` | GET | tidak berubah | halaman Audit Log (T02) |

### FE-2. Summary Plan

Route baru `/dashboard` (khusus super_admin) merender `app/views/DashboardView.svelte`: title row `PageHeader` "Dashboard" lalu `SummaryView` yang dimuat lewat `import()`, sehingga Chart.js hanya diunduh ketika `/dashboard` dibuka (AC13). Sidebar mendapat item "Dashboard" yang hanya dirender untuk super_admin. Non-super-admin yang membuka `/dashboard` mendapat not found dari guard yang sama dengan `/audit-log`, tanpa request ke API ringkasan (AC1). `AuditLogView` tidak berubah.

Feature `summary` memanggil `GET /api/audit-log/summary` satu kali per kombinasi filter dan memvalidasi respons dengan Zod `.strict()`. Hasilnya ditampilkan dalam satu baris filter (preset rentang, project, muat ulang, cap waktu data), strip dua angka, lima chart Chart.js, dan blok kualitas pencarian. Setiap kartu menurunkan statusnya sendiri (memuat, kosong, error, siap) dari satu store. Pemilih project memakai `GET /api/projects?limit=100`. Kegagalan daftar project tidak menghalangi ringkasan.

`AuditTable` diubah agar seluruh baris bisa diklik tanpa mengubah semantik tabel maupun pengembalian fokus (K9, AC14).

### FE-3. Plan (langkah berurutan)

Langkah 1 sampai 6 memakai mock (`mocks/summary.ts`, lewat `vi.stubGlobal('fetch')` di tes, tanpa mode mock di build), sehingga bisa dikerjakan paralel dengan backend setelah kontrak OpenAPI (langkah f backend) ada. Langkah 7 adalah wiring ke binary nyata.

1. **Dependensi.** Jalankan `npm view chart.js time --json`, lalu pilih versi stabil terbaru yang sudah rilis minimal 14 hari (kandidat saat plan ditulis: seri 4.5.x). Tulis tanpa `^`/`~` di `dependencies`, lalu `npm install` agar `package-lock.json` ikut. Tidak ada `chartjs-adapter-*` atau plugin lain (sumbu waktu memakai `CategoryScale` dengan label yang sudah diformat, karena bucket teratur).
2. **Kontrak tipe** `summary/types.ts`:
   - `SUMMARY_RANGES = ['24h','7d','30d'] as const`, `DEFAULT_SUMMARY_RANGE = '7d'`, `type SummaryRange`.
   - `interface SummaryFilter { range: SummaryRange; projectId: string | null }`.
   - Zod (semua objek `.strict()`): `TopProjectSchema`, `ToolUsageSchema`, `ToolMetricSchema` (`p50_ms`/`p95_ms`: `z.number().nullable()`), `TrafficPointSchema` (`at: z.string().datetime({ offset: true })`), `QualityMetricSchema {count, denominator, rate}`, `RankBucketSchema {label, count}`, `SearchQualityAvailableSchema` (`available: z.literal(true)` + semua field wajib), `SearchQualityUnavailableSchema` (`{ available: z.literal(false), error_code: z.literal('UNAVAILABLE') }`), `UsageSummarySchema` (13 kunci root sesuai 8.2, `search_quality: z.union([z.null(), Available, Unavailable])`, `bucket: z.enum(['hour','day'])`, `range: z.enum(SUMMARY_RANGES)`, `error_rate: z.number().nullable()`, `project_id: z.string().uuid().nullable()`).
   - `ProjectOptionSchema = z.object({ id: z.string().uuid(), name: z.string() }).passthrough()` dan `ProjectOptionListSchema = z.object({ items: z.array(ProjectOptionSchema), total: z.number().int() }).passthrough()`. Dibuat longgar karena milik module lain.
   - Tipe turunan `z.infer` diekspor: `UsageSummary`, `ToolMetric`, `TrafficPoint`, `SearchQuality`, `ProjectOption`.
3. **Service** (tanpa state):
   - `services/summary.service.ts`: `getUsageSummary(f: SummaryFilter, signal?: AbortSignal): Promise<UsageSummary>`, yang memanggil `get('/api/audit-log/summary', { query: { range: f.range, project_id: f.projectId ?? undefined }, schema: UsageSummarySchema, signal })`. Error HTTP diteruskan sebagai `ApiError` dari `shared/services/http`. Kegagalan Zod mengikuti perilaku `http.ts` yang ada (`error.invalid_response`).
   - `services/projectOptions.service.ts`: `listProjectOptions(signal?)`, yang menjalankan `get('/api/projects', { query: { limit: 100 }, schema: ProjectOptionListSchema, signal })` dan mengembalikan `{ options: ProjectOption[]; total: number }`, diurutkan ulang `name` naik (locale-aware `localeCompare`).
4. **Store** (state UI):
   - `stores/summaryQuery.ts`: `summaryFilterFromQuery(q: URLSearchParams): SummaryFilter` (range asing atau kosong menjadi `7d`; `project` bukan uuid diabaikan dan menjadi `null`, sama dengan cara `filterQuery.ts` menolak uuid rusak), `summaryFilterToQuery(f): URLSearchParams` (selalu `range`, lalu `project` bila ada; kunci lain tidak ditulis), `summaryFilterKey(f)`. Tidak ada kunci `tab`; `/dashboard` adalah route sendiri sehingga tidak ada kunci yang dibagi dengan filter `/audit-log`.
   - `stores/summary.store.ts`: `createSummaryStore()` dengan state
     ```ts
     type SummaryState = {
       status: 'idle' | 'loading' | 'ready' | 'error';   // status muat pertama untuk filter ini
       data: UsageSummary | null;                         // data terakhir yang sah untuk key ini
       error: ApiError | null;
       refreshing: boolean;                               // muat ulang saat data sudah ada
       refreshError: ApiError | null;                     // muat ulang gagal; data lama tetap tampil
       qualityRetrying: boolean;                          // hanya blok kualitas yang memuat
       loadedAt: Date | null;                             // dari `to` respons (cap "Data s.d.")
       key: string;
     };
     ```
     Method: `load(f)` (key berubah: `data=null`, `status=loading`, abort request lama lewat `AbortController`), `reload()` (key sama: `refreshing=true`, data lama dipertahankan), `retryQuality()` (memanggil endpoint yang sama; hanya `qualityRetrying` yang berubah; saat respons tiba seluruh `data` diganti agar satu snapshot, AC9), dan `dispose()`. Respons yang kalah balapan (key lama) dibuang.
   - `stores/projectOptions.store.ts`: `createProjectOptionsStore()` dengan state `{ status, options, total, error }` dan method `load()`/`retry()`. Dimuat sekali saat halaman Dashboard dibuka.
5. **Komponen** (`summary/components/`, presentasi saja; data dan callback lewat props):
   - `format.ts`: `formatCount(n, locale)` (`Intl.NumberFormat`, `id` menjadi `1.234`), `formatPercent(rate, locale)` (1 desimal, `null` atau `denominator 0` menjadi `–`), `formatMs(v, locale)` (`null` menjadi `–`, lalu sufiks ` ms`), `bucketLabel(at, bucket, locale)` (hour: jam lokal `HH.mm` id / `HH:mm` en; day: tanggal **UTC** `d MMM`, karena bucket hari dihitung UTC), `timezoneCaption(bucket, locale)` (mis. "Waktu: Asia/Jakarta (UTC+7)" untuk hour, "Tanggal dihitung UTC" untuk day), `truncate(label, 24)`.
   - `chart/chartSetup.ts`: `Chart.register(BarController, LineController, BarElement, LineElement, PointElement, CategoryScale, LinearScale, Tooltip, Legend)` sekali (K7). `Chart.defaults`: `animation: false`, `responsive: true`, `maintainAspectRatio: false`, font dari `getComputedStyle(body).fontFamily` ukuran 12. Tidak memakai `chart.js/auto`.
   - `chart/chartTheme.ts`: `readChartTheme(): ChartTheme` membaca `--color-chart-1`, `--color-chart-2`, `--color-danger`, `--color-muted`, `--color-rule`, `--color-fg`, `--color-surface` dari `document.documentElement`. Tidak ada hex di file ini. `hatch(color, bg)` membuat `CanvasPattern` garis diagonal 45 derajat 6px untuk segmen error, dan memakai objek gaya yang sama untuk swatch legenda (`app-screens.md` 6.4).
   - `chart/configs.ts` (fungsi murni, inti tes AC15): `trafficConfig(points, bucket, locale, theme)`, `topProjectsConfig(rows, theme, onpick)`, `toolUsageConfig(rows, theme)`, `toolStatusConfig(metrics, theme)`, `toolLatencyConfig(metrics, theme)`, `rankConfig(buckets, theme, t)`. Aturan bersama: grid horizontal saja (`--color-rule` 1px), tanpa garis sumbu y, tick 12px `--color-muted`, `beginAtZero`, `ticks.precision: 0` untuk hitungan, `maxRotation: 0`, x label 4 sampai 8 (`autoSkip` + `maxTicksLimit: 8`), bar `borderRadius: 0`, stacked `borderWidth` 2 dengan warna `--color-surface` sebagai seam, garis `tension: 0` dan `pointRadius: 0` (`pointHoverRadius: 4`), tooltip teks polos. Nama tool di sumbu memakai `font.family` mono (IBM Plex Mono). Label lebih dari 24 karakter dipotong, sedangkan nama lengkap ada di tooltip. Legenda Chart.js dimatikan (`display: false`) karena legenda di header kartu dirender HTML dari objek gaya yang sama.
   - `chart/attachChart.ts`: attachment Svelte 5 (`{@attach chart(() => config)}`) yang membuat `new Chart(canvas, config)` saat mount, memanggil `chart.update('none')` saat config berubah, dan `destroy()` saat unmount. Komponen ini berlangganan store `theme` (`shared/stores/theme`), lalu setelah `tick()` membaca ulang `readChartTheme()` dan membangun ulang config. Bukan `MutationObserver`, karena store sudah menjadi satu sumber.
   - `ChartCard.svelte`: props `{ title, ariaLabel, status: 'loading'|'empty'|'error'|'ready', emptyText, legend?: LegendItem[], meta?: string, table: { columns: string[]; rows: (string)[][] }, height: 200|220, children (snippet canvas) }`. Kartu berupa `<figure>` (`rounded-card border border-border bg-surface p-6`, tanpa shadow tambahan), dengan `<figcaption>` judul 16px weight 600 dan legenda titik 8px + label 12px abu di kanan header. `loading` menampilkan blok `bg-raised` statis setinggi plot (tanpa shimmer). `empty` menampilkan kalimat 14px `text-muted` di tengah bingkai setinggi plot. `error` menampilkan kalimat abu "Data tidak tersedia". `ready` menampilkan `<canvas role="img" aria-label>` + `<table class="sr-only">` dengan `caption` = judul dan angka yang sama dengan chart (K8). `meta` (caption zona waktu) tampil 12px abu di bawah plot.
   - `ChartLegend.svelte`: item `{ label, kind: 'line'|'bar'|'hatch', color }`. Swatch berupa SVG kecil (garis 2px, kotak, atau kotak dengan `<pattern>` diagonal), dibangun dari gaya yang sama dengan dataset.
   - `TrafficChart.svelte`, `TopProjectsChart.svelte`, `ToolUsageChart.svelte`, `ToolStatusChart.svelte`, `ToolLatencyChart.svelte`, `RankChart.svelte`: masing-masing membungkus `ChartCard` + config yang sesuai + baris tabel `sr-only`.
   - `RangeSegmented.svelte`: `<fieldset>` + `<legend class="sr-only">Rentang</legend>` + tiga `<input type="radio" class="sr-only peer">` dengan `<label>`, sehingga panah keyboard berfungsi tanpa kode. Lintasan `bg-raised rounded-base h-8 p-0.5`. Segmen aktif `bg-surface shadow-xs text-fg font-medium`, segmen lain `text-muted`. Fokus `ring-2 ring-accent-ink`. Lebar sesuai isi, dan penuh hanya di bawah 480px.
   - `ProjectPicker.svelte`: memakai `shared/components/Select.svelte` (label "Project", opsi pertama "Semua project"). Bila `projectId` di URL tidak ada di opsi (project dihapus, atau di luar 100 pertama), opsi tambahan disisipkan dengan label `top_projects[0].project_label`, atau 8 karakter awal id dalam mono bila label tidak ada. Bila `total > 100`, hint 12px tampil: "Menampilkan 100 project. Project lain bisa dipilih dengan mengklik batangnya di Project teratas." Bila status `error`, select dinonaktifkan dan muncul teks "Daftar project gagal dimuat" + tombol teks "Coba lagi". Ringkasan tetap jalan.
   - `SummaryToolbar.svelte`: kiri `RangeSegmented` + `ProjectPicker`, kanan cap "Data s.d. 10.30" (12px abu, dari `loadedAt`) + `Button variant="secondary"` "Muat ulang" (`loading` saat `refreshing`). Bila `refreshError` ada, kalimat abu "Gagal memuat ulang. Menampilkan data pukul 10.30." + tombol teks "Coba lagi" tampil di bawahnya (`role="status"`).
   - `SummaryFigures.svelte`: satu kontainer `rounded-card border border-border bg-surface` yang dibagi dua kolom oleh garis 1px `border-rule`. Sel 1 berisi label "Panggilan tool" dengan `AgentChip` di depannya dan nilai 28px weight 400 `formatCount(total_calls)`. Sel 2 berisi label "Error rate" dan nilai `formatPercent(error_rate)` dengan sufiks redup "31 error". Saat `loading`, nilai diganti bar abu setinggi 28px. Saat `error` (muat pertama gagal), satu kalimat "Ringkasan tidak dapat dimuat." + trace id + "Coba lagi" merentang dua kolom (`role="alert"`).
   - `AgentChip.svelte`: kotak `h-6 w-6 rounded-card bg-accent/20 text-accent-ink` dengan ikon agent yang sama dengan `listentries/components/ActorIcon.svelte` (path SVG disalin, karena depcruise melarang import internals feature lain), plus `aria-hidden` karena label teks sudah ada. Usulan pemindahan ke `shared/components/` dicatat di FE-11.
   - `SearchQualityBlock.svelte`: props `{ quality: SearchQuality | null; projectSelected: boolean; retrying: boolean; onretry; onpickproject }`. Bila `quality === null`: bingkai + judul "Kualitas pencarian" + `EmptyState` "Pilih project untuk melihat kualitas pencarian" + `Button secondary` "Pilih project" yang memfokuskan select project. Bila `available=false`: kalimat "Kualitas pencarian tidak dapat dimuat." + "Coba lagi" (`loading` saat `retrying`), tanpa memengaruhi kartu lain. Bila `available=true`: strip tiga angka (Zero-result, Follow-up, Reformulasi), masing-masing `6,8%` + sufiks redup `4/59`, dengan `denominator 0` ditampilkan `–`. Di bawahnya teks bantuan 12px "Perkiraan tren dari jeda 120 detik, bukan angka mutlak." lalu `RankChart`. Bila `searches === 0`: angka `–` dan RankChart dalam status `empty` "Belum ada pencarian pada rentang ini". Bila `legacy_searches > 0`: catatan 12px "{n} pencarian lama tanpa data peringkat tidak dihitung."
6. **View** `summary/views/SummaryView.svelte`: props `{ query: URLSearchParams; onquerychange: (q: URLSearchParams) => void }`. Filter diturunkan dari `query`, lalu `$effect` pada `summaryFilterKey` memanggil `store.load(f)` (pola `untrack` seperti `ListEntriesView`). Perubahan rentang atau project memanggil `onquerychange(summaryFilterToQuery(f))`, yang kemudian dijalankan `navigate('/dashboard?...', { replace: true })` oleh `DashboardView`. `SummaryView` tidak merender h1 (h1 milik `DashboardView`). Klik batang Project teratas menjalankan set `projectId`. `index.ts`: `export { default as SummaryView } from './views/SummaryView.svelte'; export type { SummaryFilter, SummaryRange } from './types';`. Tidak ada yang meng-import `summary/index.ts` secara statis.
7. **Komposisi app** (K10, K11 tiket):
   - `app/routes/index.ts` (ubah): `import DashboardView from '../views/DashboardView.svelte';` dan entri `{ path: '/dashboard', view: DashboardView, meta: { auth: true, roles: ['super_admin'], layout: 'app', titleKey: 'dashboard.title' } }`, diletakkan setelah `/audit-log`. Import statis view ini aman untuk AC13 karena `DashboardView` sendiri tidak meng-import `summary` atau `chart.js` secara statis.
   - `app/views/DashboardView.svelte` (baru), mengikuti pola `AuditLogView` (`query = $derived($current.location.query)`, `urlFor`): `const urlFor = (q) => '/dashboard' + (q.size ? '?' + q.toString() : '')`, `onquerychange(q) => navigate(urlFor(q), { replace: true })`. Markup: `<PageHeader title={$t('dashboard.title')} description={$t('dashboard.subtitle')} />`, lalu `{#await import('../../domain/core/audit-log/summary') then m}<m.SummaryView {query} {onquerychange} />{:catch e}<ErrorState error={e} onretry={reloadChunk} />{/await}` dengan `Spinner` selama chunk dimuat. `reloadChunk` menyetel ulang promise `import()` (state lokal) agar `{#await}` dievaluasi ulang.
   - `app/layouts/AppLayout.svelte` (ubah): di array `links`, blok `role === 'super_admin'` menjadi `[{ href: '/dashboard', label: $t('dashboard.menu'), icon: 'chart' }, { href: '/admin/users', ... }, { href: '/audit-log', ... }]`. Tambah cabang `{:else if l.icon === 'chart'}` di SVG ikon (outline 16px, stroke 1.5 seperti ikon lain; tiga batang vertikal, mis. `<path d="M5 19v-6M12 19V5M19 19v-9" />`). Tidak ada perubahan lain di sidebar.
   - `AuditLogView.svelte`, `AuditLogView.test.ts`, dan `listentries/views/ListEntriesView.svelte` **tidak diubah** (h1 "Audit Log" tetap di `ListEntriesView`).
8. **Klik baris `AuditTable`** (K9, AC14) sesuai 9.5, ditambah detail berikut. `<tr onclick={(e) => onRowClick(e, r.id)} class="cursor-pointer ...">`. `onRowClick` mengabaikan event bila (a) `e.target.closest('button,a,input,select,textarea,label')` ada, sehingga tombol waktu tetap memakai handler sendiri dan tidak terpanggil dua kali, atau (b) `window.getSelection()` tidak kosong dan `anchorNode` berada di dalam `e.currentTarget`. Tidak ada `role`/`tabindex` pada `<tr>`. Fungsi penentu dipisah ke `listentries/components/rowClick.ts` (`shouldOpenFromRowClick(e, row)`) agar bisa dites.
9. **i18n** (`shared/i18n/{id,en}.json`, kunci datar): `dashboard.title` ("Dashboard" / "Dashboard", dipakai `titleKey` route dan h1), `dashboard.menu` ("Dashboard" / "Dashboard", item nav), `dashboard.subtitle` ("Pemakaian tool MCP oleh agent, dari audit log." / "MCP tool usage by agents, from the audit log."), `dashboard.range.label|24h|7d|30d`, `dashboard.project.label|all|more|loadFailed`, `dashboard.reload`, `dashboard.asOf`, `dashboard.reloadFailed`, `dashboard.loadFailed`, `dashboard.chunkFailed`, `dashboard.figures.calls|errorRate|errors`, `dashboard.charts.traffic|topProjects|toolUsage|toolStatus|toolLatency` (+ `.aria` berplaceholder angka, mis. "Trafik panggilan tool, {n} bucket, puncak {max} pada {at}"), `dashboard.series.calls|errors|ok|error|p50|p95`, `dashboard.empty`, `dashboard.emptyLatency`, `dashboard.unavailable`, `dashboard.timezone`, `dashboard.timezoneUtcDay`, `dashboard.quality.title|zeroResult|followUp|reformulation|rank|notInResults|pickProject|pickProjectAction|unavailable|hint|noSearches|legacy`. Tidak ada kunci `auditLog.tabs.*`/`auditLog.summary.*`; kunci `auditLog.*` yang ada tidak diubah. Teks Indonesia, sentence case. Teks Inggris setara.
10. **Penjaga bundle (AC13)**: tambahkan `frontend/scripts/check-lazy-chunks.mjs`. Skrip membaca `dist/.vite/manifest.json`, sehingga `build.manifest: true` perlu ditambahkan di `vite.config.ts`. Skrip gagal bila chunk entri (`index.html`) atau `imports` statisnya memuat modul `node_modules/chart.js`. Script npm `check:chunks` dijalankan setelah `npm run build` di job `build` CI (`.github/workflows/ci.yml`, satu baris).
11. **Wiring dan verifikasi**: jalankan terhadap binary nyata (`make build`), lalu `npm run check`, `npm test`, `npm run depcruise`, `npm run build && npm run check:chunks`, `npx playwright test e2e/audit-log-summary.spec.ts e2e/audit-log.spec.ts`, dan `node .agents/skills/design-taste-frontend/scripts/lint.mjs --app frontend/src`. Terakhir, `inspect.mjs --app http://localhost:5173/dashboard` di 1280px dan 390px, tema terang dan gelap.

### FE-4. Migration

Tidak ada.

### FE-5. Changes

| File | Status | Isi |
|---|---|---|
| `frontend/package.json`, `frontend/package-lock.json` | ubah | `"chart.js": "<versi pasti>"`, script `check:chunks` |
| `frontend/vite.config.ts` | ubah | `build.manifest: true` |
| `frontend/scripts/check-lazy-chunks.mjs` | baru | penjaga AC13 |
| `.github/workflows/ci.yml` | ubah | job `build`: `npm run check:chunks` setelah build |
| `frontend/src/domain/core/audit-log/summary/types.ts` (+ `types.test.ts`) | baru | konstanta, Zod, tipe |
| `.../summary/services/summary.service.ts` (+ `.test.ts`) | baru | `getUsageSummary` |
| `.../summary/services/projectOptions.service.ts` (+ `.test.ts`) | baru | `listProjectOptions` |
| `.../summary/stores/summaryQuery.ts` (+ `.test.ts`) | baru | parse/serialize URL |
| `.../summary/stores/summary.store.ts` (+ `.test.ts`) | baru | state ringkasan |
| `.../summary/stores/projectOptions.store.ts` (+ `.test.ts`) | baru | state pemilih project |
| `.../summary/components/format.ts` (+ `.test.ts`) | baru | format angka/waktu |
| `.../summary/components/chart/{chartSetup,chartTheme,configs,attachChart}.ts` (+ `configs.test.ts`, `chartSetup.test.ts`) | baru | Chart.js |
| `.../summary/components/{ChartCard,ChartLegend,TrafficChart,TopProjectsChart,ToolUsageChart,ToolStatusChart,ToolLatencyChart,RankChart,RangeSegmented,ProjectPicker,SummaryToolbar,SummaryFigures,AgentChip,SearchQualityBlock}.svelte` | baru | presentasi |
| `.../summary/components/components.test.ts` | baru | tes komponen |
| `.../summary/views/SummaryView.svelte` (+ `SummaryView.test.ts`) | baru | komposisi isi halaman Dashboard |
| `.../summary/mocks/summary.ts` | baru | data contoh FE-6 (5) |
| `.../summary/index.ts` | baru | `SummaryView`, tipe |
| `frontend/src/app/routes/index.ts` | ubah | route `/dashboard` (meta `roles: ['super_admin']`, `titleKey: 'dashboard.title'`) |
| `frontend/src/app/views/DashboardView.svelte` (+ `DashboardView.test.ts`) | baru | `PageHeader` "Dashboard", lazy `import()` `SummaryView`, `onquerychange` ke `/dashboard` |
| `frontend/src/app/layouts/AppLayout.svelte` | ubah | item nav "Dashboard" (ikon `chart`) di blok super_admin |
| `frontend/src/app/views/app.test.ts` | ubah | item nav Dashboard hanya super_admin; guard `/dashboard` |
| `frontend/src/domain/core/audit-log/listentries/components/AuditTable.svelte` | ubah | klik baris |
| `.../listentries/components/rowClick.ts` (+ tes di `components.test.ts`) | baru | `shouldOpenFromRowClick` |
| `frontend/src/shared/i18n/{id,en}.json` | ubah | kunci FE-3 no.9 |
| `frontend/e2e/audit-log-summary.spec.ts` | baru | e2e |
| `frontend/e2e/audit-log.spec.ts` | ubah | tambah kasus klik baris dan drag-select (AC14) |

Tidak diubah: `app/views/AuditLogView.svelte` dan `.test.ts`, `listentries/views/ListEntriesView.svelte`, `app/routes/guard.ts` (perilaku `notfound` untuk role tidak cocok sudah ada), `app/views/HomeView.svelte` (`/` tetap daftar project), `shared/components/*`, `app.css`, dan `DESIGN.md`.

### FE-6. Visual

**(1) Pertanyaan per layar dan blok terbesar**

| Layar | Pertanyaan | Blok terbesar yang menjawab |
|---|---|---|
| `/dashboard` (tanpa project) | Bagaimana agent memakai MCP pada rentang ini: seberapa banyak, lewat tool apa, dan di mana gagal atau lambat? | Kartu **Trafik** lebar penuh di baris pertama grid (220px), dengan strip angka tepat di atasnya sebagai jawaban satu kalimat ("1.234 panggilan, 2,5% error") |
| `/dashboard?project=<id>` | Bagaimana agent memakai MCP di project ini, dan apakah pencariannya menemukan yang dicari? | Trafik tetap blok terbesar. Kartu Project teratas hilang, dan blok Kualitas pencarian naik tepat di bawah Trafik |
| `/audit-log` | Siapa (manusia atau agent) melakukan apa, kapan? (tidak berubah) | Tabel. Baris kini seluruhnya menjadi target klik |

**(2) Pola DESIGN.md dan kerangka**

- Kerangka `AppLayout` (sidebar 228px di atas `bg-ground` + sheet putih radius 16). Tidak ada top bar desktop. Sidebar mendapat satu item "Dashboard" (ikon outline 16px, pil aktif yang sama dengan item lain) hanya untuk super_admin; ini mengikuti aturan item nav DESIGN.md, bukan komponen baru.
- Title row `PageHeader` (h1 22px "Dashboard" + satu kalimat abu tentang data: "Pemakaian tool MCP oleh agent, dari audit log."). Tidak ada aksi utama hitam di title row, tidak ada tab.
- `/dashboard`: pola baru "ringkasan analitik" dari `app-screens.md` 4.3/4.4/5/6/11/12, dengan urutan baris filter (cakupan kiri, utilitas kanan), strip angka terbagi garis, grid kartu chart, lalu blok kualitas. DESIGN.md belum punya baris "Pola per layar" untuk layar ini, sehingga usulannya ada di FE-11.
- `/audit-log`: pola yang ada (list + filter bar + panel detail kanan), tanpa perubahan selain klik baris.
- Tata letak: gap 24 vertikal dan horizontal. Grid `lg:grid-cols-2`. Trafik dan Kualitas pencarian `col-span-full`. Bila jumlah kartu setengah lebar ganjil (project dipilih: Tool usage, Status, Latensi), kartu terakhir `col-span-full`.

Urutan dari atas: title row, baris filter (24px ke strip angka), strip angka, Trafik, Project teratas | Tool paling sering dipakai, Status per tool | Latensi per tool, Kualitas pencarian. Bila project dipilih: title row, filter, angka, Trafik, Kualitas pencarian, Tool usage | Status, Latensi (penuh).

**(3) Elemen khas produk di layar pertama**

- **Jejak agent**: `AgentChip` (kotak kuning pucat, ikon agent) di depan label "Panggilan tool" pada strip angka. Chip yang sama dengan baris agent di tabel audit menegaskan bahwa seluruh halaman ini berisi aktivitas agent, bukan manusia.
- **Mono untuk yang dibaca mesin**: nama tool `read_file`, `search_knowledge`, `get_api_endpoint` di sumbu y kartu Tool, Status, dan Latensi, serta di tabel `sr-only`. Nama project tetap sans.
- **Aksen kuning hanya di data utama**: seri "panggilan" (`chart-1`) di Trafik, Project teratas, Tool usage, segmen ok, p50, dan rank, serta ikon item nav aktif (`text-accent-ink`, perilaku `AppLayout` yang ada). Tidak ada kuning di tombol, latar kartu, atau segmented.

**(4) Token dan utility (tanpa hex baru)**

| Pemakaian | Token / utility |
|---|---|
| Kartu, strip angka | `bg-surface border border-border rounded-card p-6` (tanpa `shadow-*`, karena kartu tidak boleh lebih tebal dari sheet) |
| Pemisah strip, grid chart | `border-rule` / `--color-rule` 1px |
| Judul kartu | 16px weight 600 `text-fg`. Label angka 14px `text-muted`. Nilai KPI 28px weight 400 `tabular-nums` |
| Tick, legenda, caption, cap waktu | 12px `text-muted` / `--color-muted` |
| Seri utama | `--color-chart-1` |
| Seri kedua (p95) | `--color-chart-2` |
| Error (garis trafik, segmen status) | `--color-danger` + `CanvasPattern` diagonal pada segmen, `text-danger` hanya untuk ikon/kalimat error yang memang status |
| `not_in_results` di rank | `--color-muted` |
| Segmented | `bg-raised rounded-base h-8`, aktif `bg-surface shadow-xs` |
| Skeleton | `bg-raised` statis |
| Fokus | `ring-2 ring-accent-ink` |
| Agent chip | `rounded-card bg-accent/20 text-accent-ink` (sama dengan `AuditTable`) |

`chart-3` tidak dipakai. Jumlah hue di layar adalah dua seri (`chart-1`, `chart-2`) + `danger` + abu, di bawah batas peringatan inspector (empat). Tidak ada gradien, glow, sparkline, animasi masuk (`animation: false`), legenda untuk chart satu seri, atau sudut bar membulat.

**(5) Data contoh (`mocks/summary.ts`)**

- `mockSummary7d`: sama dengan 8.2, dengan `top_projects` dilengkapi agar total = 1.234: Payments 420, Onboarding Mobile 311, Ledger Core 268, Partner Portal 150, Grimoire Docs 85. `tool_metrics` 4 tool (600/402/150/82, error 10/16/0/5, `get_api_endpoint` latensi `null`), dengan `total_calls` = 1.234, `total_errors` = 31, dan `error_rate` = 31/1234 yang **dihitung di mock** dari baris, bukan ditulis tangan. Traffic 8 bucket (90, 0, 210, 188, 175, 240, 221, 110) berjumlah 1.234. Bucket 16 Okt bernilai 0 untuk menguji garis nol.
- `mockSummary24hProject`: sama dengan 8.3 (Payments, `search_knowledge` 60 dan `read_file` 5), 25 bucket per jam yang dibangkitkan dari `from`, dengan `search_quality` available: zero-result 4/59, follow-up 31/55, reformulasi 7/59, dan rank 18/6/3/1/1/0/0/0/0/0 + `not_in_results` 2.
- `mockSummaryQualityUnavailable`: `mockSummary24hProject` dengan `search_quality: { available: false, error_code: 'UNAVAILABLE' }`.
- `mockSummaryEmpty`: 8.5 lengkap (8 bucket nol, 11 bucket rank nol, `error_rate: null`).
- `mockProjectOptions`: `{ items: [Payments, Onboarding Mobile, Ledger Core, Partner Portal, Grimoire Docs], total: 5 }` dengan uuid `0192f5a0-1111-7aaa-8bbb-00000000000N`.
- Tidak ada Acme, lorem, John Doe, atau delta "+12%". Jam contoh "Data s.d. 10.30".

Contoh tampilan strip angka (id): `Panggilan tool 1.234` | `Error rate 2,5% 31 error`. Pada `mockSummaryEmpty`: `0` | `–`.

### FE-7. State per blok

| Blok | loading (muat pertama) | ready | empty | error | refreshing |
|---|---|---|---|---|---|
| Baris filter | aktif sepenuhnya; cap waktu disembunyikan | cap "Data s.d. {to lokal}" | sama dengan ready | cap disembunyikan | tombol "Muat ulang" `loading`, cap lama tetap |
| Strip angka | bar abu 28px per nilai | angka | `0` dan `–` | satu kalimat + trace id + "Coba lagi" merentang | nilai lama, `aria-busy` |
| Trafik | skeleton 220px | garis | `total_calls = 0`: garis nol tetap digambar (bucket nol dari server) + kalimat "Belum ada panggilan MCP pada rentang ini" di caption | "Data tidak tersedia" | data lama |
| Project teratas | skeleton | bar | kalimat empty di bingkai | "Data tidak tersedia" | data lama |
| Tool usage, Status | skeleton | bar | kalimat empty | "Data tidak tersedia" | data lama |
| Latensi | skeleton | bar | tidak ada tool: kalimat empty; ada tool tapi semua `null`: "Belum ada data latensi pada rentang ini" | "Data tidak tersedia" | data lama |
| Kualitas pencarian | skeleton (bila project dipilih) | 3 angka + rank | tanpa project: ajakan pilih project; `searches = 0`: angka `–` + rank empty | `available=false`: kalimat + "Coba lagi" (`retryQuality`) | `qualityRetrying`: hanya blok ini yang menampilkan Spinner kecil di tombol |
| Pemilih project | select nonaktif + label "Memuat project…" | opsi | 0 project: hanya "Semua project" | nonaktif + "Daftar project gagal dimuat" + "Coba lagi" | - |
| Chunk halaman Dashboard (`DashboardView`) | title row tampil, `Spinner` di bawahnya | - | - | title row tetap + `ErrorState` + "Coba lagi" (memanggil `import()` lagi) | - |
| Item nav "Dashboard" | - | tampil hanya bila `role === 'super_admin'` | - | - | - |

Kode error HTTP: 400 (URL berisi `range`/`project` yang lolos parse FE tetapi ditolak server, mestinya tidak terjadi) ditampilkan sebagai error strip **tanpa** "Coba lagi", ditambah tombol "Reset filter" yang menjalankan `onquerychange(?range=7d)` (menjadi `/dashboard?range=7d`). 401 ditangani `http.ts` (redirect login). 403 menampilkan error tanpa coba lagi (normalnya guard route sudah mencegah, mis. role berubah di tengah sesi). 500 atau jaringan menampilkan error dengan coba lagi. Non-super-admin tidak pernah sampai ke state mana pun di tabel ini: guard merender not found sebelum `DashboardView` dipasang.

**Ponsel (<640px)**: title row tetap. Item "Dashboard" ada di menu ponsel yang sama (sidebar yang dibuka tombol Menu). Baris filter menjadi dua baris (segmented + select di baris 1 dan menjadi penuh di bawah 480px; cap waktu + muat ulang di baris 2 rata kanan). Strip angka tetap dua kolom (genap). Kartu chart satu kolom, tinggi plot 180px, x label maksimal 4 (`maxTicksLimit: 4`), padding kartu 16. Halaman tidak pernah menggulir menyamping. Tablet (641 sampai 1023px) memakai satu kolom kartu.

### FE-8. Test Scenario (Vitest + Testing Library, jsdom)

`chart.js` di-mock di tes komponen (`vi.mock('chart.js', ...)`, `Chart` palsu mencatat `config`, `update`, `destroy`) karena jsdom tidak punya canvas 2D. `configs.test.ts` tidak butuh mock karena fungsinya murni.

- `types.test.ts`: `mockSummary7d`, `mockSummary24hProject`, `mockSummaryQualityUnavailable`, dan `mockSummaryEmpty` lolos `UsageSummarySchema`; kunci root tambahan `args` ditolak (`.strict()`, penjaga AC10); kunci `query` di dalam `tool_metrics[0]` ditolak; `p50_ms: null` diterima; `search_quality: { available: false }` tanpa `error_code` ditolak; `bucket: 'week'` ditolak.
- `summary.service.test.ts`: tanpa project, URL `= /api/audit-log/summary?range=7d` (tanpa `project_id`); dengan project ada `project_id=<uuid>`; `signal` diteruskan; respons 400 dengan envelope `INVALID_ARGUMENT` menghasilkan `ApiError` `status 400`, `details.field = 'range'`; respons yang tidak cocok schema menghasilkan error `invalid_response`.
- `projectOptions.service.test.ts`: URL `limit=100`; hasil diurutkan menurut nama; field tambahan item (`my_role`, `counts`) tidak membuat gagal.
- `summaryQuery.test.ts`: query kosong menjadi `{7d, null}`; `range=90d`/`range=custom`/`range=` menjadi `7d`; `project=not-uuid` menjadi `null`; round-trip `{24h, uuid}` menghasilkan tepat `range=24h&project=<uuid>`; `{7d, null}` menghasilkan tepat `range=7d`; kunci asing di input (`tab`, `entry`, `project_id`, `actor_type`) tidak ikut diserialisasi.
- `summary.store.test.ts`: `load` dua kali cepat dengan key berbeda membatalkan request pertama (`signal.aborted`), dan respons pertama yang terlambat tidak menimpa; `load` sukses menghasilkan `ready`, `loadedAt = to`; gagal menghasilkan `error` dan `data = null`; `reload` saat `ready` membuat `refreshing = true`, `data` lama tetap, dan saat gagal `refreshError` terisi dengan `status` tetap `ready`; `retryQuality` hanya mengubah `qualityRetrying`, lalu `data` diganti seluruhnya saat sukses.
- `projectOptions.store.test.ts`: `load` sekali; error lalu `retry` menjadi `ready`.
- `format.test.ts`: `formatCount(1234,'id') = '1.234'` dan `'en' = '1,234'`; `formatPercent(0.0251,'id') = '2,5%'`; `formatPercent(null) = '–'`; `formatPercent(0) = '0%'` (nol sungguhan tetap `0`); `formatMs(null) = '–'`; `bucketLabel('2026-10-16T00:00:00Z','day','id')` menghasilkan "16 Okt" di zona waktu jsdom mana pun (UTC); `bucketLabel(hour)` mengikuti TZ lokal (tes menyetel `process.env.TZ='Asia/Jakarta'`, `10:00Z` menjadi `17.00`); `truncate` 24 karakter + `…`.
- `configs.test.ts` (AC12, AC15):
  - `trafficConfig`: `labels.length = points.length` (8 untuk 7d, 25 untuk 24h); dataset 0 `data` = calls, `borderColor = theme.chart1`, `borderWidth 2`, `tension 0`, `pointRadius 0`; dataset 1 = errors, `borderColor = theme.danger`, `borderWidth 1.5`, tanpa `borderDash`; `scales.y.beginAtZero`, `grid.color = theme.rule`, `border.display = false`; `scales.x.grid.display = false`, `ticks.maxRotation = 0`, `maxTicksLimit 8`; `plugins.legend.display = false`.
  - `topProjectsConfig`: `indexAxis 'y'`; urutan mengikuti input; label > 24 karakter dipotong dan tooltip `title` mengembalikan nama penuh; `onClick` pada elemen indeks 1 memanggil `onpick(top_projects[1].project_id)`.
  - `toolStatusConfig`: dua dataset `stacked`, ok `backgroundColor = theme.chart1`, error `backgroundColor` berupa pattern (objek dari `hatch`), `borderColor = theme.surface`, `borderWidth 2`; jumlah ok + error per tool = calls.
  - `toolLatencyConfig`: `p50_ms: null` menjadi `null` di dataset (bukan 0); dua dataset `chart1`/`chart2`; label sumbu y "ms".
  - `rankConfig`: 11 label, terakhir "tidak di hasil", warna terakhir `theme.muted`.
  - Semua config: `animation: false`, `borderRadius 0` pada bar, font tick 12, font sumbu kategori tool = mono.
- `chartSetup.test.ts`: `Chart.register` dipanggil sekali dengan tepat 9 komponen K7, termasuk saat modul di-import dua kali.
- `components.test.ts`:
  - `ChartCard` empat status: `loading` tanpa canvas dengan skeleton `aria-hidden`; `empty` menampilkan teks empty dan judul tetap ada; `error` menampilkan "Data tidak tersedia"; `ready` menampilkan `canvas[role=img]` dengan `aria-label` dan `table.sr-only` yang barisnya sama dengan input.
  - `TrafficChart` dengan `mockSummaryEmpty` tetap membuat Chart (garis nol) dan menampilkan kalimat empty.
  - `ToolLatencyChart` dengan semua `null` menampilkan "Belum ada data latensi".
  - `ChartLegend` untuk status menampilkan swatch `hatch` dengan `<pattern>`.
  - Pergantian store `theme` dari `light` ke `dark` memanggil `update` pada Chart palsu dengan warna yang dibaca ulang. Unmount memanggil `destroy`.
  - `SummaryFigures`: `1.234`, `2,5%`, "31 error"; `error_rate null` menjadi `–`; status error menampilkan satu `role=alert` dengan tombol "Coba lagi" yang memanggil callback; `AgentChip` ada di sel pertama.
  - `SearchQualityBlock` tiga keadaan: `null` menampilkan ajakan, dan klik "Pilih project" memanggil `onpickproject`; `available=false` menampilkan kalimat, dan klik "Coba lagi" memanggil `onretry` (tombol `loading` saat `retrying`); `available=true` menampilkan `6,8%`, `4/59`, `56,4%`, `11,9%`; `denominator 0` menampilkan `–`; `legacy_searches 3` menampilkan catatan.
  - `RangeSegmented`: tiga radio, `7 hari` `checked`; panah kanan pada radio terpilih memilih `30 hari` (perilaku native) dan memanggil `onchange('30d')`.
  - `ProjectPicker`: opsi "Semua project" + 5; `projectId` yang tidak ada di opsi disisipkan dengan label dari prop `fallbackLabel`; `total 140` menampilkan hint; status error membuat select `disabled` dan menampilkan tombol coba lagi.
- `SummaryView.test.ts` (fetch di-stub dengan mock): query kosong memicu satu fetch `range=7d`; `SummaryView` tidak merender `h1`; mengganti rentang memanggil `onquerychange` dengan `range=24h`, dan setelah query baru diteruskan terjadi satu fetch baru; dengan `project` kartu Project teratas tidak dirender dan blok kualitas menampilkan angka; respons `mockSummaryQualityUnavailable` membuat chart lain tetap `ready` dan klik "Coba lagi" di blok kualitas menghasilkan tepat satu fetch tambahan; respons 500 menampilkan satu `role=alert` di strip dan lima kartu berstatus error tanpa tombol; respons 400 menampilkan tombol "Reset filter"; daftar project 500 tetap membuat ringkasan tampil.
- `DashboardView.test.ts` (baru; fetch di-stub dengan mock): `/dashboard` merender h1 "Dashboard" tepat satu kali, `Spinner` selama `import()` belum selesai, lalu judul kartu "Trafik"; mengganti rentang ke "24 jam" mengubah `location` menjadi `/dashboard?range=24h` lewat `replace` (panjang `history` tidak bertambah); `import()` yang gagal (`vi.doMock` modul summary melempar) menampilkan `ErrorState` dengan h1 tetap ada, dan "Coba lagi" memanggil `import()` lagi.
- `app.test.ts` (ubah tes "users menu and route only for super_admin" atau tambah tes sejajar): developer di `/` tidak melihat link "Dashboard" (selain Users dan Audit Log); super_admin melihat link "Dashboard" di `navigation` "Navigasi utama", urutannya sebelum "Users"; developer yang di-`boot('/dashboard')` mendapat tampilan not found dan `fetch` tidak pernah dipanggil dengan URL `/api/audit-log/summary`; super_admin di `/dashboard` membuat link "Dashboard" `aria-current=page`.
- `AuditLogView.test.ts`: tidak diubah dan harus tetap hijau (bukti bahwa halaman Audit Log tidak tersentuh).
- `listentries/components/components.test.ts` (tambahan, AC14): klik `<td>` aktor memanggil `onselect(id)` tepat sekali; klik tombol waktu memanggil tepat sekali (bukan dua); dengan seleksi teks aktif di dalam baris (`window.getSelection` di-stub) klik tidak memanggil; `<tr>` tidak punya `role` maupun `tabindex`; tombol waktu tetap fokusable dan Enter memanggil `onselect`. Unit `shouldOpenFromRowClick` mencakup target `button`, `a`, seleksi di luar baris (tetap membuka), dan seleksi di dalam baris (tidak membuka).
- Arsitektur: `npm run depcruise` hijau (summary tidak meng-import `listentries`/`getentry`/`project/*`; `app/` hanya meng-import `summary/index.ts`, dan hanya secara dinamis dari `DashboardView`; `domain/` tidak meng-import `app/`).
- Bundle: `npm run build && npm run check:chunks` lulus. Tes negatif skrip memakai fixture manifest kecil `scripts/__fixtures__/manifest-bad.json` yang membuat skrip keluar dengan kode 1.

### FE-9. E2E Scenarios (Playwright, `frontend/e2e/audit-log-summary.spec.ts`, terhadap binary nyata)

Setup per tes memakai helper yang sudah ada: `adminContext(browser)`, `createProjectApi` (dua project bernama unik, A dan B), `createKeyApi(request, A.id)` / `(B.id)`, lalu `callTool(request, key, 'list_files', {})` 5 kali untuk A dan `callTool(request, keyB, 'search_knowledge', { query: 'login' })` 2 kali untuk B. Karena DB e2e dipakai bersama, assert hanya pada data yang dibuat tes ini, dengan filter `project`, atau dengan "≥" pada tampilan tanpa project.

Positif:

1. **Default lewat navigasi (AC1)**: login sebagai admin, buka `/`, klik link "Dashboard" di `navigation` "Navigasi utama". URL menjadi `/dashboard`, link itu `aria-current=page`, radio "7 hari" `checked`, heading h1 "Dashboard" satu kali, dan 5 `figure` dengan `canvas[role=img][aria-label]` (Trafik, Project teratas, Tool, Status, Latensi). Blok "Kualitas pencarian" menampilkan "Pilih project untuk melihat kualitas pencarian". Request yang terkirim tepat `GET /api/audit-log/summary?range=7d`.
2. **Ganti rentang (AC11)**: klik "24 jam". URL menjadi `/dashboard?range=24h`, request `range=24h` terkirim, dan tabel `sr-only` Trafik punya 24 atau 25 baris. `page.reload()` membuat radio "24 jam" tetap `checked`.
3. **Pilih project (AC8, AC11)**: pilih project A di select. URL berisi `project=<A.id>`, kartu Project teratas tidak ada, tabel `sr-only` Tool berisi baris `list_files` dengan `5`, strip angka menampilkan `5`, dan blok kualitas menampilkan tiga label angka (Zero-result, Follow-up, Reformulasi) dan chart rank. Reload mempertahankan project.
4. **Project teratas tanpa filter** (rentang 24 jam): tabel `sr-only` Project teratas memuat baris A (`5`) dan B (`2`), dengan A di atas B bila keduanya masuk 10 besar (assert urutan relatif saja). Klik batang di kanvas tidak diuji di e2e karena koordinat kanvas rapuh. Perilaku `onClick` lalu `onpick(project_id)` dicakup `configs.test.ts`, dan jalur memilih project lewat select dicakup no.3.
5. **Lazy chunk (AC13)**: `page.on('request')` mencatat URL. Buka `/` (tunggu daftar project), lalu `/audit-log` (tunggu tabel). Tidak ada request ke chunk yang isinya memuat `chart.js`: ambil daftar chunk dari `dist/.vite/manifest.json` (dibaca di tes lewat `fs`, entri `src/domain/core/audit-log/summary/index.ts` dan `node_modules/chart.js/...`), lalu klik link nav "Dashboard" dan pastikan chunk itu baru diminta.
6. **Klik baris di halaman Audit Log (AC14)** (di `audit-log.spec.ts`): buat entri lewat aksi nyata (pola tes T02), lalu klik sel Aksi pada baris itu. Panel `role=dialog` "Audit entry" terbuka dan URL berisi `entry=`. Tekan tombol tutup, dan fokus berada di tombol waktu baris itu (`data-entry-id`). Tab sampai tombol waktu + Enter membuka detail lagi.
7. **Tema gelap (AC12)**: `page.emulateMedia({ colorScheme: 'dark' })` dengan `localStorage` kosong pada `/dashboard`. Kanvas dirender, dan `getComputedStyle` `--color-chart-1` gelap terbaca (nilai token, bukan hex di tes). Screenshot dilampirkan ke artefak (tanpa snapshot pembanding).
8. **Ponsel**: viewport 390x844. `document.documentElement.scrollWidth <= 390`, kartu satu kolom (lebar kartu Trafik dan Tool sama), dan segmented penuh.

Negatif:

9. **Non-super-admin (AC1)**: login sebagai developer (`createReadyUserApi`). Di `/`, link "Dashboard" tidak ada di sidebar (`toHaveCount(0)`, pola `project-api-key.spec.ts` untuk link Audit Log). Lalu buka `/dashboard` langsung: respons 200 (SPA), perilakunya sama dengan `/audit-log` untuk non-super-admin (heading h1 "Dashboard" tidak ada dan tidak ada `figure`/`canvas`, memakai pola assert `audit-log.spec.ts` baris 58), tidak ada request ke `/api/audit-log/summary` (`page.on('request')`, tunggu 300ms seperti tes T02), dan tidak ada error konsol.
10. **Kualitas pencarian gagal (AC9)**: `page.route('**/api/audit-log/summary?*project_id=*', ...)` mengembalikan `mockSummaryQualityUnavailable` (disalin sebagai JSON fixture e2e). Lima chart tampil, blok kualitas menampilkan "Kualitas pencarian tidak dapat dimuat." dan "Coba lagi". Klik tombol itu menghasilkan tepat satu request tambahan, dan setelah route dilepas (`unroute`) angka kualitas tampil.
11. **Server 500**: route summary mengembalikan 500 envelope `INTERNAL`. Strip menampilkan satu `role=alert` dengan trace id dan "Coba lagi", sedangkan kartu chart menampilkan "Data tidak tersedia" tanpa tombol. Klik link nav "Audit Log" membuat tabel tampil normal.
12. **Parameter URL rusak**: `/dashboard?range=90d&project=bukan-uuid`. FE menormalisasi request menjadi `range=7d` tanpa `project_id` (tidak ada 400), dan select menampilkan "Semua project".
13. **Daftar project gagal**: route `/api/projects*` mengembalikan 500. Select nonaktif dengan "Daftar project gagal dimuat", sedangkan ringkasan tetap tampil.
14. **Drag-select tidak membuka detail (AC14)** (di `audit-log.spec.ts`): `page.mouse` drag di sel target sebuah baris. URL tidak berisi `entry=` dan dialog tidak ada.

### FE-10. Request/Response body yang dikonsumsi FE

Request ringkasan (dibangun `getUsageSummary`):

```
GET /api/audit-log/summary?range=24h&project_id=0192f5a0-1111-7aaa-8bbb-000000000001
Accept: application/json
Cookie: <sesi super_admin>
```

Respons 200 yang divalidasi `UsageSummarySchema`: persis bentuk 8.2 (tanpa project), 8.3 (project, T11 berhasil), 8.4 (`search_quality: {"available":false,"error_code":"UNAVAILABLE"}`), dan 8.5 (project tanpa aktivitas). FE tidak menambah atau mengubah field.

Request daftar project:

```
GET /api/projects?limit=100
```

Respons yang dipakai (field lain dibiarkan oleh `.passthrough()`):

```json
{
  "items": [
    { "id": "0192f5a0-1111-7aaa-8bbb-000000000001", "name": "Payments", "description": "Layanan pembayaran internal", "my_role": null, "access_level": "admin", "is_member": false, "member_count": 2, "counts": { "files": 12, "specs": 2, "keys": 1 }, "created_by": "0192f5a0-aaaa-7aaa-8bbb-00000000000a", "created_by_label": "a@tim.id", "created_at": "2026-10-22T09:00:00Z", "updated_at": "2026-10-22T10:15:00Z" },
    { "id": "0192f5a0-1111-7aaa-8bbb-000000000002", "name": "Onboarding Mobile", "description": "", "my_role": null, "access_level": "admin", "is_member": false, "member_count": 3, "counts": { "files": 4, "specs": 1, "keys": 2 }, "created_by": "0192f5a0-aaaa-7aaa-8bbb-00000000000a", "created_by_label": "a@tim.id", "created_at": "2026-10-20T08:00:00Z", "updated_at": "2026-10-21T16:40:00Z" }
  ],
  "total": 2,
  "limit": 100,
  "offset": 0
}
```

Error yang ditangani FE (envelope `foundation.yml#/components/schemas/Error`, lihat 8.6):

```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "range tidak valid", "details": { "field": "range" } }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```

| Status | `code` | Perilaku UI |
|---|---|---|
| 400 | `INVALID_ARGUMENT` | strip: kalimat error + "Reset filter" (tanpa coba lagi) |
| 401 | `UNAUTHORIZED` | `http.ts` mengarahkan ke `/login?next=...` |
| 403 | `FORBIDDEN` | strip: kalimat error, tanpa coba lagi (normalnya guard route sudah mencegah) |
| 500 | `INTERNAL` | strip: kalimat + trace id + "Coba lagi"; kartu "Data tidak tersedia" |
| 0 (jaringan) | `INTERNAL` dari `toApiError` | sama dengan 500 |
| 200, `search_quality.available=false` | - | hanya blok kualitas yang menampilkan error + "Coba lagi" |

Contoh tabel `sr-only` yang dirender untuk Status per tool (dari `mockSummary7d`):

| Tool | ok | error | error rate |
|---|---|---|---|
| `read_file` | 590 | 10 | 1,7% |
| `search_knowledge` | 386 | 16 | 4,0% |
| `list_files` | 150 | 0 | 0% |
| `get_api_endpoint` | 77 | 5 | 6,1% |

### FE-11. Catatan dan usulan perubahan DESIGN.md (tidak diterapkan diam-diam)

1. **Tambah baris pola** di tabel "Pola per layar": `/dashboard` (khusus Super Admin) | "Bagaimana agent memakai MCP: seberapa banyak, lewat tool apa, di mana gagal atau lambat?" | "title row tanpa tab; baris filter (preset kiri, muat ulang + cap waktu kanan); strip angka terbagi garis; grid kartu chart radius 8 tanpa bayangan; seri utama `chart-1`, error `danger` + arsir". Perubahan ini butuh persetujuan manusia sesuai tabel "Penerapan".
2. **Arsir untuk seri error** (`CanvasPattern` diagonal) belum disebut DESIGN.md. Usul: "Seri error di chart memakai `danger` + arsir diagonal; swatch legenda memakai arsir yang sama." Tidak ada token baru.
3. **`AgentChip`** saat ini diduplikasi (markup di `AuditTable` dan `summary/components/AgentChip.svelte`) karena depcruise melarang import internals feature. Usul tiket lanjutan: pindahkan chip agent (dengan prop `label`, tanpa tipe domain) ke `shared/components/ActorChip.svelte`, lalu pakai di keduanya.
4. **Status di DESIGN.md** ("layar ... audit log ... sudah memakai token ini") perlu ditambah "dashboard (T15)" setelah merge.
5. **Ikon nav `chart`** adalah satu cabang ikon outline 16px baru di `AppLayout` (aturan "item navigasi dengan ikon outline 16px" di bagian Kerangka DESIGN.md). Bukan komponen atau token baru; tidak perlu perubahan DESIGN.md.
6. Tidak ada warna, radius, font, shadow, atau komponen dekoratif baru. Semua nilai warna chart dibaca dari token `app.css` saat runtime.
