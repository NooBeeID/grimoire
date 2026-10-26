# T19 Insight Pemakaian Agent Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Endpoint baca-saja `GET /api/audit-log/usage` (super_admin) plus bagian "Wawasan agent" di halaman `/dashboard` yang menampilkan dokumen paling sering dibaca, dokumen tak pernah dibaca, query hasil kosong, dan pemakaian per API key.

**Architecture:** Feature backend baru `core/internal/auditlog/usage` (satu usecase `Usage`, tidak dipecah) yang membaca `audit_log` lewat empat query repository terpisah (tiap bagian gagal sendiri, `available:false`), memakai bus `project.management.get-project` (404 project tak dikenal) dan `project.knowledge.list-files` (daftar file untuk "tak pernah dibaca"). Rule bisnis (validasi query, batas, pembentukan bagian, selisih file) di `auditlog/base/usage.go`. Frontend feature baru `frontend/src/domain/core/audit-log/usage/` tanpa Chart.js, di-import statis oleh `app/views/DashboardView.svelte`, memuat tiap bagian dengan request `?section=` sendiri.

**Tech Stack:** Go 1.x, chi, sqlx, Postgres (`audit_log`), `infra/bus`; Svelte 5 (runes), Zod, Vitest + Testing Library; OpenAPI bundle via `make api-bundle`.

**Spec:** `docs/superpowers/specs/2026-10-26-agent-usage-insights-design.md` (dibaca bersama `docs/tickets/T15-core-audit-log-summary.md` dan `docs/superpowers/plans/core-audit-log-summary.md`).

## Global Constraints

- Rentang hanya `24h|7d|30d` (default `7d`), `[from, to)` mundur dari waktu request; pakai ulang `base.Range`, `base.NewWindow` dari `auditlog/base/summary.go`.
- `section` opsional: `top_files|unread_files|zero_queries|keys`; tanpa `section` semua bagian dikembalikan. Kunci respons bagian keempat bernama `per_key` (nilai parameter tetap `keys`).
- Pembacaan = `mcp.read_file` dan `mcp.read_section` status `ok`, `target` = path. Query kosong = `mcp.search_knowledge` status `ok` dengan `result.total = 0`. Per key = `actor_type='api_key'`, dikelompokkan `actor_id`, label = `actor_label` terbaru.
- Normalisasi query: huruf kecil + spasi tunggal + trim (aturan `NormalizeQuery` T11: `strings.ToLower(strings.Join(strings.Fields(q), " "))`).
- Batas: tiap daftar 20 baris, `unread_files` 50 baris; urut jumlah menurun lalu path/query menaik (`COLLATE "C"`).
- Hanya `SELECT` pada `audit_log`; parameterized query selalu; `section`/unit SQL dari whitelist, bukan input mentah.
- Tidak ada migrasi, tabel agregat, job, aksi audit baru, atau endpoint bus baru. Endpoint tidak menulis audit (K6).
- Respons tidak memuat isi file, cuplikan, `args` lain selain query ternormalisasi, maupun `result` mentah.
- Hanya super_admin (grup `/audit-log` yang sudah memasang `RequireRole`).
- `core` tidak membaca tabel project; file lewat `project.knowledge.list-files` (kontrak `lib/contracts/project/knowledge/listfiles` sudah memuat `Item.UpdatedAt`, jadi **kontrak tidak diubah**).
- FE: teks lewat i18n `frontend/src/shared/i18n/{id,en}.json` (kunci datar `dashboard.usage.*`); komponen memakai `Table`, `Button`, `EmptyState`, `ErrorState` dari `shared/components` dan token DESIGN.md (`rounded-card border border-border bg-surface`, tab = segmented pill `glass rounded-full`, aktif `bg-primary text-primary-fg`, path `font-mono`). Tanpa gaya lama (outline tebal, bayangan keras).
- FE: feature tidak meng-import feature lain (`summary`); komposisi hanya di `app/views/DashboardView.svelte` lewat `usage/index.ts`.

### Keputusan plan (menyelesaikan celah/konflik spec)

- **P1** K3 "maks 200 file per respons" vs K4 "tak-pernah-dibaca 50": respons memuat maks **50** item (`base.UnreadLimit`). File dipindai lewat bus maks **2000** (`base.MaxScanFiles`, 4 halaman `listfiles.MaxLimit`=500). `truncated=true` bila unread > 50 atau pemindaian berhenti di batas.
- **P2** "Dua daftar yang butuh project" = `top_files` dan `unread_files` (path hanya bermakna per project). Tanpa `project_id` keduanya `{available:false, error_code:"PROJECT_REQUIRED", items:[]}` tanpa query. `zero_queries` dan `per_key` jalan untuk semua project.
- **P3** Spec menyebut 404 "seperti `/summary`", padahal `/summary` mengembalikan 200 kosong. Kriteria penerimaan menang: `project_id` diisi tetapi `get-project` `Found=false` → `404 NOT_FOUND`. Error infra dari `get-project` → 500 (prasyarat, bukan bagian).
- **P4** Kegagalan bagian (repository atau bus list-files) → `{available:false, error_code:"UNAVAILABLE", items:[]}`, di-log `WarnContext`, respons tetap 200.
- **P5** `section` diisi → hanya bagian itu berisi objek, bagian lain `null`. `section` tak dikenal → 400 `details.field=section`.
- **P6** `mcp.read_section` (T18) belum ada di repo. Bila `record.ActionMCPReadSection` belum ada saat dikerjakan, Task 1 menambah konstanta itu (nilai `"mcp.read_section"`) ke `actions.go`, `Actions()`, `actions_test.go`, dan katalog aksi README. Efek samping yang disengaja: `summary` (T15) ikut menghitungnya sebagai tool.
- **P7** `calls`/`errors` per key memakai `base.ToolActions()` (definisi tool T15); `reads` = aksi baca status `ok`; `searches` = `mcp.search_knowledge` semua status; `top_tool` = tool dengan panggilan terbanyak, seri dipecah nama menaik, tanpa awalan `mcp.`.
- **P8** UI memuat empat bagian dengan empat request paralel (`?section=`), sehingga error HTTP pun hanya mematikan satu tabel. Bagian yang butuh project tidak di-request bila project kosong. Tab Dokumen/Pencarian = state lokal (tidak di URL).

## Review Focus

1. Query dengan spasi Unicode (NBSP, tab) atau huruf non-ASCII: `lower()`/`regexp_replace` Postgres harus mengelompokkan sama seperti `NormalizeQuery` Go untuk tab/newline/spasi ganda; tes Task 3 `TestT19_ZeroQueriesNormalizeWhitespace`.
2. Project dengan > 2000 file: respons tetap cepat, `truncated=true`, tidak ada panggilan bus ke-5; tes Task 2 `TestFileLister_StopsAtCap`.
3. Entri audit lama tanpa `result.total` atau `total` berupa string: tidak dihitung dan tidak membuat cast error; tes Task 3 `TestT19_ZeroQueriesIgnoresNonNumericTotal`.
4. Pembacaan berstatus `error` (path salah) tidak menjadikan file "pernah dibaca" dan tidak masuk `top_files`; tes Task 3 `TestT19_ErrorReadsIgnored`.
5. Ganti project/rentang cepat berturut-turut: respons lama tidak menimpa state baru (abort); tes Task 8 `abortsStaleLoad`.

---

## File Structure

Backend (baru): `backend/internal/domain/core/internal/auditlog/base/usage.go` (+`usage_test.go`); `backend/internal/domain/core/internal/auditlog/usage/{init.go,params.go,handler.go,usecase.go,repository.go,ports.go}` + tes `{params_test.go,handler_test.go,usecase_test.go,ports_test.go,repository_test.go,explain_integration_test.go,http_integration_test.go}`.
Backend (ubah): `auditlog/base.go` (daftarkan `usage.Init`), `auditlog/arch_test.go` (`TestOnlyGetRoutes`: 4 rute GET), bila P6: `backend/lib/contracts/core/audit/record/{actions.go,actions_test.go}`.
API: `api/modules/core/audit-log.yml`, `api/root.yml`, `api/openapi.yml` (generated).
Frontend (baru): `frontend/src/domain/core/audit-log/usage/{types.ts,types.test.ts,index.ts}`, `services/usage.service.ts(+test)`, `stores/usageQuery.ts(+test)`, `stores/usage.store.ts(+test)`, `components/{InsightTabs,TopFilesTable,UnreadFilesTable,ZeroQueriesTable,KeyUsageTable,SectionCard}.svelte`, `components/format.ts(+test)`, `components/components.test.ts`, `views/UsageInsightsView.svelte(+UsageInsightsView.test.ts)`, `mocks/usage.ts`.
Frontend (ubah): `frontend/src/app/views/DashboardView.svelte`, `frontend/src/app/views/DashboardView.test.ts`, `frontend/src/shared/i18n/{id,en}.json`.
Docs: `docs/tickets/T19-core-audit-log-usage-insights.md` (baru), `docs/tickets/README.md`.

---

### Task 1: Entity dan rule `base/usage.go` (+ konstanta `read_section` bila perlu)

**Files:**
- Create: `backend/internal/domain/core/internal/auditlog/base/usage.go`
- Test: `backend/internal/domain/core/internal/auditlog/base/usage_test.go`
- Modify (hanya bila P6 berlaku): `backend/lib/contracts/core/audit/record/actions.go`, `backend/lib/contracts/core/audit/record/actions_test.go`

**Interfaces:**
- Consumes: `base.Range`, `base.DefaultRange`, `base.Window`, `base.NewWindow`, `base.ToolActions()`, `base.ToolName`, `isUUID`, `fieldErr` (sudah ada di package `base`); `record.ActionMCPReadFile`, `record.ActionMCPReadSection`, `record.ActionMCPSearchKnowledge`.
- Produces:
```go
type Section string
const (
    SectionTopFiles Section = "top_files"; SectionUnreadFiles Section = "unread_files"
    SectionZeroQueries Section = "zero_queries"; SectionKeys Section = "keys"
)
const (TopFilesLimit = 20; UnreadLimit = 50; ZeroQueriesLimit = 20; PerKeyLimit = 20; MaxScanFiles = 2000)
const (SectionUnavailable = "UNAVAILABLE"; SectionProjectRequired = "PROJECT_REQUIRED")

func ReadActions() []string          // [ActionMCPReadFile, ActionMCPReadSection]
func (s Section) NeedsProject() bool // true untuk top_files, unread_files

type UsageQuery struct { Range Range; ProjectID string; Section Section }
func (q *UsageQuery) Validate() error         // range seperti SummaryQuery; project_id uuid -> lower; section kosong atau whitelist, selain itu fieldErr("section")
func (q UsageQuery) Wants(s Section) bool     // Section=="" || Section==s

type FileRead struct { Path string `json:"path"`; Reads int `json:"reads"`; Keys int `json:"keys"`; LastReadAt time.Time `json:"last_read_at"` }
type UnreadFile struct { Path string `json:"path"`; UpdatedAt time.Time `json:"updated_at"` }
type ZeroQuery struct { Query string `json:"query"`; Count int `json:"count"`; Keys int `json:"keys"`; LastAt time.Time `json:"last_at"` }
type KeyUsage struct { KeyID string `json:"key_id"`; Label string `json:"label"`; Calls, Errors, Searches, Reads int /* json calls, errors, searches, reads */; LastUsedAt time.Time `json:"last_used_at"`; TopTool string `json:"top_tool"` }
type KnownFile struct { Path string; UpdatedAt time.Time }

type TopFilesSection    struct { Available bool `json:"available"`; ErrorCode string `json:"error_code,omitempty"`; Items []FileRead `json:"items"` }
type UnreadFilesSection struct { Available bool; ErrorCode string; Items []UnreadFile; Truncated bool `json:"truncated"` }
type ZeroQueriesSection struct { Available bool; ErrorCode string; Items []ZeroQuery }
type PerKeySection      struct { Available bool; ErrorCode string; Items []KeyUsage }

func TopFilesOK(items []FileRead) *TopFilesSection        // Items nil -> [] ; dipotong TopFilesLimit
func ZeroQueriesOK(items []ZeroQuery) *ZeroQueriesSection
func PerKeyOK(items []KeyUsage) *PerKeySection            // TopTool lewat ToolName (buang "mcp.")
func BuildUnread(files []KnownFile, unread []string, scanCapped bool) *UnreadFilesSection
// Bagian gagal / butuh project: Available false, ErrorCode = code, Items [] (bukan nil), Truncated false
func TopFilesError(code string) *TopFilesSection
func UnreadFilesError(code string) *UnreadFilesSection
func ZeroQueriesError(code string) *ZeroQueriesSection
func PerKeyError(code string) *PerKeySection

type UsageReport struct {
    Range Range `json:"range"`; From time.Time `json:"from"`; To time.Time `json:"to"`; ProjectID *string `json:"project_id"`
    TopFiles *TopFilesSection `json:"top_files"`; UnreadFiles *UnreadFilesSection `json:"unread_files"`
    ZeroQueries *ZeroQueriesSection `json:"zero_queries"`; PerKey *PerKeySection `json:"per_key"`
}
func NewUsageReport(w Window, q UsageQuery) UsageReport // isi range/from/to/project_id, semua bagian nil
```
`BuildUnread`: hanya path di `unread` yang juga ada di `files` (file terhapus tidak pernah muncul), urutan mengikuti `unread`, maks `UnreadLimit`, `UpdatedAt` dari `files` (UTC), `Truncated = len(unreadValid) > UnreadLimit || scanCapped`.

- [ ] **Step 1: (P6) Cek konstanta.** Run: `grep -n ActionMCPReadSection backend/lib/contracts/core/audit/record/actions.go`. Bila kosong: tulis tes di `actions_test.go` `TestActions_IncludesReadSection` (`slices.Contains(Actions(), "mcp.read_section")`), jalankan `cd backend && go test ./lib/contracts/core/audit/record/` (FAIL), tambah `ActionMCPReadSection = "mcp.read_section"` setelah `ActionMCPReadFile` dan di `Actions()`, jalankan lagi (PASS). Juga tambahkan ke katalog aksi README (Task 11).

- [ ] **Step 2: Tulis tes gagal di `usage_test.go`**
```go
func TestUsageQuery_Validate(t *testing.T)        // tabel: {"" ,"",""}->Range 7d ok; Section "keys" ok; "files"->field "section"; Range "1y"->field "range"; ProjectID "x"->field "project_id"; ProjectID upper -> lower
func TestUsageQuery_Wants(t *testing.T)           // Section "" -> Wants semua 4 true; Section keys -> hanya SectionKeys true
func TestSection_NeedsProject(t *testing.T)       // top_files,unread_files true; zero_queries,keys false
func TestReadActions(t *testing.T)                // == []string{"mcp.read_file","mcp.read_section"}
func TestT19_AC2_BuildUnreadExcludesDeleted(t *testing.T) // files [a.md,b.md], unread [b.md,old.md] -> Items [{b.md,updatedAt(b)}], Truncated false
func TestBuildUnread_Truncation(t *testing.T)     // 51 unread valid -> len 50, Truncated true; 3 unread + scanCapped -> Truncated true
func TestBuildUnread_EmptyIsArray(t *testing.T)   // json.Marshal -> `"items":[]`
func TestPerKeyOK_TopToolName(t *testing.T)       // TopTool "mcp.read_file" -> "read_file"
func TestSectionsLimit(t *testing.T)              // TopFilesOK(25 item) -> 20; ZeroQueriesOK(nil) -> Items != nil
func TestNewUsageReport_JSONShape(t *testing.T)   // keys: range,from,to,project_id,top_files,unread_files,zero_queries,per_key; semua bagian null
```
- [ ] **Step 3: Jalankan.** Run: `cd backend && go test ./internal/domain/core/internal/auditlog/base/ -run 'Usage|Section|ReadActions|T19|BuildUnread|PerKey|Sections' -v` → FAIL (undefined).
- [ ] **Step 4: Implementasi `usage.go`** sesuai Interfaces. Tanpa I/O, tanpa context.
- [ ] **Step 5: Jalankan lagi** perintah Step 3 → PASS; plus `go test ./internal/domain/core/internal/auditlog/...` tetap hijau.
- [ ] **Step 6: Commit**
```bash
git add backend/internal/domain/core/internal/auditlog/base/usage.go backend/internal/domain/core/internal/auditlog/base/usage_test.go backend/lib/contracts/core/audit/record/
git commit -m "feat(core-audit-log-usage): entity dan rule insight pemakaian agent

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

### Task 2: Port bus (get-project, list-files)

**Files:**
- Create: `backend/internal/domain/core/internal/auditlog/usage/ports.go`
- Test: `backend/internal/domain/core/internal/auditlog/usage/ports_test.go`

**Interfaces:**
- Consumes: `bus.CallTyped`, `constants.BUS_PROJECT_MANAGEMENT_GET_PROJECT`, `constants.BUS_PROJECT_KNOWLEDGE_LIST_FILES`, `getproject.{Request,Response}`, `listfiles.{Request,Response,MaxLimit}`, `base.KnownFile`, `base.MaxScanFiles`.
- Produces:
```go
type ProjectFinder interface { Exists(ctx context.Context, projectID string) (bool, error) }
type FileLister interface { ListAll(ctx context.Context, projectID string) (files []base.KnownFile, capped bool, err error) }
func NewProjectFinder(b *bus.Registry) ProjectFinder
func NewFileLister(b *bus.Registry) FileLister
```
`ListAll`: loop `listfiles.Request{ProjectID, Recursive: true, Limit: listfiles.MaxLimit, Offset}`; berhenti saat `!res.Truncated` atau `len(files) >= base.MaxScanFiles` (`capped = res.Truncated` pada halaman terakhir yang diambil); `UpdatedAt` di-UTC-kan. Tidak mendereferensi bus saat dibuat.

- [ ] **Step 1: Tulis tes gagal** (registry asli `bus.NewRegistry`, handler palsu yang mencatat request):
```go
func TestProjectFinder_FoundAndMissing(t *testing.T) // Found true -> (true,nil); Found false -> (false,nil); req.ProjectID diteruskan
func TestProjectFinder_BusError(t *testing.T)       // handler error -> err != nil
func TestFileLister_Paginates(t *testing.T)         // 3 halaman (500,500,10; Truncated true,true,false) -> 1010 file, capped false, Offset 0/500/1000, Recursive true, Limit 500
func TestFileLister_StopsAtCap(t *testing.T)        // provider selalu Truncated true -> tepat 4 panggilan, 2000 file, capped true
func TestFileLister_Error(t *testing.T)             // ErrNotRegistered -> err
```
- [ ] **Step 2: Jalankan.** Run: `cd backend && go test ./internal/domain/core/internal/auditlog/usage/ -run 'ProjectFinder|FileLister' -v` → FAIL.
- [ ] **Step 3: Implementasi `ports.go`.**
- [ ] **Step 4: Jalankan** Step 2 → PASS.
- [ ] **Step 5: Commit** `feat(core-audit-log-usage): port bus get-project dan list-files` (file `usage/ports.go`, `usage/ports_test.go`).

### Task 3: Repository (4 bagian)

**Files:**
- Create: `backend/internal/domain/core/internal/auditlog/usage/repository.go`
- Test: `backend/internal/domain/core/internal/auditlog/usage/repository_test.go` (`//go:build integration`), `backend/internal/domain/core/internal/auditlog/usage/explain_integration_test.go`

**Interfaces:**
- Consumes: `*db.DB`, `base.Window`, `base.ReadActions()`, `base.ToolActions()`, `record.ActionMCPSearchKnowledge`, tipe item Task 1.
- Produces:
```go
type Repository interface {
    TopFiles(ctx context.Context, w base.Window, projectID string, limit int) ([]base.FileRead, error)
    UnreadPaths(ctx context.Context, w base.Window, projectID string, paths []string, limit int) ([]string, error)
    ZeroQueries(ctx context.Context, w base.Window, projectID string, limit int) ([]base.ZeroQuery, error) // projectID "" = semua
    PerKey(ctx context.Context, w base.Window, projectID string, limit int) ([]base.KeyUsage, error)       // projectID "" = semua
}
func NewRepository(d *db.DB) Repository
```
Bentuk SQL (tiap method satu `SelectContext`, read-only, semua nilai placeholder; predikat waktu `occurred_at >= $from AND occurred_at < $to`):
- `TopFiles`: `action = ANY($reads) AND status='ok' AND project_id=$p::uuid AND target <> ''` → `GROUP BY target`, `count(*) reads`, `count(DISTINCT actor_id) keys`, `max(occurred_at) last_read_at`, `ORDER BY reads DESC, target COLLATE "C" ASC LIMIT $limit`.
- `UnreadPaths`: `SELECT p FROM unnest($paths::text[]) p WHERE NOT EXISTS (SELECT 1 FROM audit_log a WHERE a.project_id=$p AND a.action = ANY($reads) AND a.status='ok' AND <waktu> AND a.target = p) ORDER BY p COLLATE "C" LIMIT $limit`. `paths` kosong → kembalikan `[]` tanpa query.
- `ZeroQueries`: ekspresi `nq = lower(btrim(regexp_replace(args->>'query', '[[:space:]]+', ' ', 'g')))`; `action=$search AND status='ok' AND jsonb_typeof(result->'total')='number' AND (result->>'total')::numeric = 0 AND nq <> ''` (+ project bila ada) → `GROUP BY nq`, `count(*)`, `count(DISTINCT actor_id)`, `max(occurred_at)`, `ORDER BY count DESC, nq COLLATE "C" LIMIT $limit`. Cek `jsonb_typeof` dalam `CASE` agar cast tidak dievaluasi pada string.
- `PerKey`: CTE `e` = `actor_type='api_key' AND actor_id IS NOT NULL AND action = ANY($tools)` (+ project); agregat per `actor_id`: label `(array_agg(actor_label ORDER BY occurred_at DESC, id DESC))[1]`, `calls`, `errors` (`status='error'`), `searches` (`action=$search`), `reads` (`action = ANY($reads) AND status='ok'`), `last_used_at`; `top_tool` dari CTE `(actor_id, action, count)` + `DISTINCT ON (actor_id) ORDER BY actor_id, count DESC, action ASC`; urut `calls DESC, label ASC, key_id ASC LIMIT $limit`. `TopTool` dikembalikan mentah (`mcp.*`), dibuang awalan oleh `base.PerKeyOK`.

- [ ] **Step 1: Tulis tes integrasi gagal** (helper `setupDB`/`insert` meniru `summary/repository_test.go`, ditambah kolom `actor_id`, `actor_label`, `target`, `args`):
```go
func TestT19_AC1_DuplicateReadsOneKey(t *testing.T)        // key K baca a.md 3x -> TopFiles[0]={a.md, Reads 3, Keys 1, LastReadAt = waktu terakhir}
func TestT19_AC1_ReadSectionCounted(t *testing.T)          // 1 read_file + 2 read_section a.md -> Reads 3; UnreadPaths([a.md,b.md]) == [b.md]
func TestT19_AC1_QueryCaseInsensitiveOneRow(t *testing.T)  // "Rate Limit", "rate limit", " RATE limit " total 0 oleh 2 key -> 1 baris {query "rate limit", Count 3, Keys 2}
func TestT19_ZeroQueriesNormalizeWhitespace(t *testing.T)  // "rate\tlimit", "rate\n limit" -> satu baris "rate limit"
func TestT19_ZeroQueriesIgnoresNonNumericTotal(t *testing.T) // result {} , {"total":"0"}, {"total":2} -> tidak ada baris, err nil
func TestT19_ErrorReadsIgnored(t *testing.T)               // read_file status error b.md -> tidak di TopFiles; UnreadPaths memuat b.md
func TestT19_WindowAndProjectIsolation(t *testing.T)       // entri project lain & di luar [from,to) tidak dihitung di 4 method
func TestT19_OrderingAndLimit(t *testing.T)                // 25 path -> 20 baris; seri reads diurut path naik ("A.md" < "a.md" COLLATE C)
func TestT19_PerKeyCounts(t *testing.T)                    // K1: 2 read_file ok, 1 read_file error, 1 search, 1 mcp.initialize -> Calls 4, Errors 1, Searches 1, Reads 2, TopTool "mcp.read_file", Label = label terbaru; actor_type user diabaikan
func TestT19_PerKeyTopToolTieBreak(t *testing.T)           // 2 list_files, 2 read_file -> "mcp.list_files"
func TestT19_UnreadPathsEmptyInput(t *testing.T)           // paths nil -> [] , err nil
```
- [ ] **Step 2: Tulis `explain_integration_test.go`** `TestT19_ExplainUsesIndex`: seed ~20k baris campuran, `EXPLAIN (FORMAT JSON)` tiap query → plan memuat `audit_log_project_idx` (TopFiles, UnreadPaths) atau `audit_log_action_idx` (ZeroQueries, PerKey tanpa project), bukan hanya `Seq Scan` (pola `summary/explain_integration_test.go`). Catat waktu eksekusi di PR.
- [ ] **Step 3: Jalankan.** Run: `make dev-test-db && cd backend && go test -tags integration ./internal/domain/core/internal/auditlog/usage/ -run 'T19_' -v` → FAIL.
- [ ] **Step 4: Implementasi `repository.go`.** Tidak ada `fmt.Sprintf` berisi input; konstanta SQL saja.
- [ ] **Step 5: Jalankan** Step 3 → PASS; `go test ./internal/domain/core/internal/auditlog/` (`TestAppendOnlySources`) PASS.
- [ ] **Step 6: Commit** `feat(core-audit-log-usage): query repository empat bagian insight`.

### Task 4: Usecase `Usage`

**Files:**
- Create: `backend/internal/domain/core/internal/auditlog/usage/usecase.go`
- Test: `backend/internal/domain/core/internal/auditlog/usage/usecase_test.go`

**Interfaces:**
- Consumes: `Repository` (Task 3), `ProjectFinder`, `FileLister` (Task 2), `base.*` (Task 1).
- Produces:
```go
type Usecase struct { /* repo, projects, files, now, log */ }
func NewUsecase(r Repository, p ProjectFinder, f FileLister, now func() time.Time, log *slog.Logger) *Usecase
func (u *Usecase) Usage(ctx context.Context, q base.UsageQuery) (base.UsageReport, error) // q sudah tervalidasi
```
Alur: `w := base.NewWindow(q.Range, u.now())`; bila `ProjectID != ""` → `Exists` (timeout 5s): err → `fmt.Errorf("usage: project: %w", err)`; false → `errs.Wrap(errs.CodeNotFound, "project tidak ditemukan", errs.ErrNotFound)`. Lalu tiap bagian dengan `q.Wants(s)`, berurutan: butuh project dan kosong → `PROJECT_REQUIRED`; error repo/bus → log `WarnContext(ctx, "audit usage: section unavailable", "section", s, "err", err)` + `UNAVAILABLE`. `unread_files`: `ListAll` (timeout 5s) → `UnreadPaths(..., paths, base.UnreadLimit+1)` → `base.BuildUnread`.

- [ ] **Step 1: Tulis tes gagal** (fake repo/finder/lister mencatat panggilan):
```go
func TestUsage_AllSectionsNoProject(t *testing.T)     // TopFiles/UnreadFiles ErrorCode PROJECT_REQUIRED, repo.TopFiles & lister tidak dipanggil; ZeroQueries/PerKey Available true
func TestT19_AC3_UnknownProject404(t *testing.T)      // Exists false -> errors.Is(err, errs.ErrNotFound); repo tidak dipanggil
func TestUsage_ProjectLookupErrorIs500(t *testing.T)  // Exists err -> err != nil, bukan ErrNotFound
func TestUsage_SectionFailSoft(t *testing.T)          // repo.ZeroQueries err -> ZeroQueries {false,"UNAVAILABLE",[]}, bagian lain Available true, log memuat "section=zero_queries"
func TestUsage_ListFilesFailSoft(t *testing.T)        // lister err -> UnreadFiles UNAVAILABLE; UnreadPaths tidak dipanggil
func TestUsage_SectionOnly(t *testing.T)              // Section keys -> hanya PerKey != nil; repo hanya PerKey dipanggil
func TestUsage_UnreadPassesPathsAndLimit(t *testing.T) // UnreadPaths menerima semua path dari lister dan limit 51; capped diteruskan ke Truncated
func TestUsage_WindowFromNow(t *testing.T)            // now tetap -> From = now-168h untuk 7d, diteruskan ke repo
```
- [ ] **Step 2: Jalankan.** Run: `cd backend && go test ./internal/domain/core/internal/auditlog/usage/ -run 'Usage|T19_AC3' -v` → FAIL.
- [ ] **Step 3: Implementasi `usecase.go`.** Hanya orkestrasi; pembentukan bagian lewat konstruktor `base`.
- [ ] **Step 4: Jalankan** Step 2 → PASS.
- [ ] **Step 5: Commit** `feat(core-audit-log-usage): usecase orkestrasi insight`.

### Task 5: Handler, params, init, route `/usage`

**Files:**
- Create: `backend/internal/domain/core/internal/auditlog/usage/{params.go,handler.go,init.go}`
- Modify: `backend/internal/domain/core/internal/auditlog/base.go` (import `usage`, `usage.Init(d, r)` di grup `/audit-log` setelah `summary.Init`; komentar `T19: GET /audit-log/usage`), `backend/internal/domain/core/internal/auditlog/arch_test.go` (`TestOnlyGetRoutes`: empat rute GET termasuk `/api/audit-log/usage`)
- Test: `usage/params_test.go`, `usage/handler_test.go`, `usage/http_integration_test.go` (`//go:build integration`)

**Interfaces:**
- Consumes: `Usecase.Usage` (Task 4), `respond.WriteJSON`, `respond.WriteError`.
- Produces:
```go
func ParseQuery(q url.Values) base.UsageQuery // TrimSpace range, project_id, section
type usager interface { Usage(ctx context.Context, q base.UsageQuery) (base.UsageReport, error) }
func NewHandler(uc usager) *Handler
func (h *Handler) Get(w http.ResponseWriter, r *http.Request)
func Init(d app.Deps, r chi.Router) // NewUsecase(NewRepository(d.DB), NewProjectFinder(d.Bus), NewFileLister(d.Bus), time.Now, d.Log); r.Get("/usage", ...)
```

- [ ] **Step 1: Tulis tes gagal**
```go
func TestParseQuery(t *testing.T)                        // " 24h "->"24h"; section " keys "->"keys"; parameter lain diabaikan
func TestHandler_Invalid(t *testing.T)                   // ?range=1y, ?project_id=x, ?section=files -> 400 INVALID_ARGUMENT, details.field sesuai
func TestHandler_NotFound(t *testing.T)                  // uc errs.ErrNotFound -> 404 NOT_FOUND
func TestHandler_InternalErrorGeneric(t *testing.T)      // uc error "pq: secret" -> 500, body tidak memuat "pq"
func TestHandler_OK(t *testing.T)                        // 200, JSON berisi 8 kunci, uc menerima query ternormalisasi
// http_integration_test.go (pola summary: withIdentity X-Test-Role, bus nyata dengan handler get-project/list-files palsu, DB nyata)
func TestT19_AC3_HTTP_Authorization(t *testing.T)        // tanpa identity 401; developer 403; super_admin 200
func TestT19_AC3_HTTP_UnknownProject404(t *testing.T)    // get-project Found=false -> 404
func TestT19_HTTP_RouteNotShadowedByID(t *testing.T)     // GET /api/audit-log/usage tidak ditangani /{id}
func TestT19_HTTP_EndToEndShape(t *testing.T)            // seed audit + list-files [a.md,b.md,c.md], read a.md -> top_files[0].path a.md, unread [b.md,c.md] updated_at dari bus
func TestT19_HTTP_NoContentLeakAndNoAudit(t *testing.T)  // seed args {"query":"x","secret":"s"}, result {"snippet":"ISI"} -> body tidak memuat "ISI","secret","snippet"; jumlah baris audit_log tidak bertambah
```
- [ ] **Step 2: Jalankan.** Run: `cd backend && go test ./internal/domain/core/internal/auditlog/... -v -run 'ParseQuery|Handler|OnlyGetRoutes'` → FAIL; integrasi: `go test -tags integration ./internal/domain/core/internal/auditlog/usage/ -run HTTP -v` → FAIL.
- [ ] **Step 3: Implementasi** `params.go`, `handler.go` (validasi `q.Validate()` lalu usecase), `init.go`, ubah `base.go` dan `arch_test.go`.
- [ ] **Step 4: Jalankan** Step 2 → PASS; `cd backend && go test ./internal/archtest/...` PASS.
- [ ] **Step 5: Commit** `feat(core-audit-log-usage): handler dan route GET /api/audit-log/usage`.

### Task 6: OpenAPI

**Files:**
- Modify: `api/modules/core/audit-log.yml` (path `/api/audit-log/usage` diletakkan sebelum `/api/audit-log/{id}`; skema `AuditUsage`, `AuditUsageTopFiles`, `AuditUsageUnreadFiles`, `AuditUsageZeroQueries`, `AuditUsagePerKey` + item; `info.version` naik minor), `api/root.yml` (path + `components.schemas.AuditUsage`), `api/openapi.yml` (generated)

**Interfaces:**
- Produces: `operationId: getAuditUsage`; parameter `range` (enum 24h/7d/30d, default 7d), `project_id` (uuid), `section` (enum `top_files|unread_files|zero_queries|keys`); respons 200 (contoh lengkap: tanpa project, `top_files`/`unread_files` `{available:false,error_code:PROJECT_REQUIRED,items:[]}`), 400 (`details.field`), 401, 403, 404 (`foundation.yml#/components/responses/NotFound`), 500. Tiap bagian `nullable: true`; `error_code` enum `UNAVAILABLE|PROJECT_REQUIRED`; `items.maxItems` 20 (unread 50). Deskripsi menyebut: tidak dicatat audit, tidak memuat isi file, query dinormalisasi.

- [ ] **Step 1: Ubah dua file yml.**
- [ ] **Step 2: Run:** `make api-bundle && make api-check` → keduanya exit 0; `grep -n "/api/audit-log/usage" api/openapi.yml` → ada.
- [ ] **Step 3: Commit** `docs(api): kontrak GET /api/audit-log/usage (T19)`.

### Task 7: FE types Zod, query parser, service

**Files:**
- Create: `frontend/src/domain/core/audit-log/usage/types.ts`, `types.test.ts`, `stores/usageQuery.ts`, `stores/usageQuery.test.ts`, `services/usage.service.ts`, `services/usage.service.test.ts`, `mocks/usage.ts`, `index.ts`

**Interfaces:**
- Produces:
```ts
export const USAGE_RANGES = ['24h', '7d', '30d'] as const; export type UsageRange = (typeof USAGE_RANGES)[number];
export const USAGE_SECTIONS = ['top_files', 'unread_files', 'zero_queries', 'keys'] as const; export type UsageSection = (typeof USAGE_SECTIONS)[number];
export const SECTION_KEY = { top_files: 'top_files', unread_files: 'unread_files', zero_queries: 'zero_queries', keys: 'per_key' } as const;
export const PROJECT_SECTIONS: readonly UsageSection[] = ['top_files', 'unread_files'];
export interface UsageFilter { range: UsageRange; projectId: string | null }
export const AgentUsageSchema // z.object({range, from, to, project_id: uuid|null, top_files, unread_files, zero_queries, per_key}).strict(); tiap bagian .nullable(), item .strict(), datetime({offset:true}), error_code z.enum(['UNAVAILABLE','PROJECT_REQUIRED']).optional()
export type AgentUsage = z.infer<typeof AgentUsageSchema>; export type FileRead, UnreadFile, ZeroQuery, KeyUsage
export function usageFilterFromQuery(q: URLSearchParams): UsageFilter // kunci URL sama dengan Dashboard: range, project (uuid)
export const usageFilterKey = (f: UsageFilter) => string             // `${range}|${projectId ?? ''}`
export function getAgentUsage(f: UsageFilter, section: UsageSection, signal?: AbortSignal): Promise<AgentUsage>
// GET /api/audit-log/usage, query {range, project_id ?? undefined, section}
// index.ts: export { default as UsageInsightsView } from './views/UsageInsightsView.svelte'
```
- [ ] **Step 1: Tulis tes gagal**
```ts
// types.test.ts
it('parses full mock')                    // AgentUsageSchema.parse(mockUsageAll) ok
it('accepts null sections')               // hanya per_key terisi -> ok
it('rejects extra field in item')         // top_files.items[0].snippet -> throw (strict, anti-bocor)
it('rejects unknown error_code')          // 'BOOM' -> throw
// usageQuery.test.ts
it('defaults invalid range to 7d and drops non-uuid project')
// usage.service.test.ts
it('sends range, project_id, section')    // fetch URL memuat range=30d&project_id=<id>&section=keys
it('omits project_id when null')
it('throws ApiError on schema mismatch')
```
- [ ] **Step 2: Run:** `cd frontend && npx vitest run src/domain/core/audit-log/usage` → FAIL.
- [ ] **Step 3: Implementasi** file-file di atas; `mocks/usage.ts` mengekspor `mockUsageAll`, `mockUsageNoProject`, `mockUsageEmpty`.
- [ ] **Step 4: Run** Step 2 → PASS.
- [ ] **Step 5: Commit** `feat(core-audit-log-usage-fe): tipe Zod, parser query, dan service usage`.

### Task 8: FE store per bagian

**Files:**
- Create: `frontend/src/domain/core/audit-log/usage/stores/usage.store.ts`, `stores/usage.store.test.ts`

**Interfaces:**
- Consumes: `getAgentUsage`, `UsageFilter`, `UsageSection`, `SECTION_KEY`, `PROJECT_SECTIONS`.
- Produces:
```ts
export type SectionStatus = 'loading' | 'ready' | 'empty' | 'error' | 'needs_project';
export interface SectionState<T> { status: SectionStatus; items: T[]; truncated: boolean; error: ApiError | 'unavailable' | null }
export interface UsageState { key: string; top_files: SectionState<FileRead>; unread_files: SectionState<UnreadFile>; zero_queries: SectionState<ZeroQuery>; keys: SectionState<KeyUsage> }
export interface UsageStore extends Readable<UsageState> { load(f: UsageFilter): void; retry(s: UsageSection): Promise<void>; dispose(): void }
export function createUsageStore(fetcher = getAgentUsage): UsageStore
```
Aturan: `load` membatalkan semua request lama (satu `AbortController` per bagian), lalu tiap bagian paralel; tanpa project bagian `PROJECT_SECTIONS` langsung `needs_project` tanpa fetch. Respons `available:false` `PROJECT_REQUIRED` → `needs_project`; `UNAVAILABLE` → `error` dengan `error:'unavailable'`; HTTP error → `error` dengan `ApiError`; items kosong → `empty`.

- [ ] **Step 1: Tulis tes gagal**
```ts
it('loads four sections in parallel with project')       // fetcher dipanggil 4x dengan section berbeda
it('skips project sections without project')             // fetcher hanya zero_queries & keys; top_files.status 'needs_project'
it('marks unavailable section as error only')            // zero_queries available:false -> error 'unavailable'; keys 'ready'
it('http error affects one section')                     // fetcher reject untuk keys -> keys 'error' ApiError; lainnya ready
it('empty items -> empty status')
it('retry reloads one section')                          // retry('keys') -> fetcher 1x section keys
it('abortsStaleLoad')                                    // load(A) lalu load(B); resolve A belakangan -> state tetap B, key B
it('keeps truncated flag for unread_files')
```
- [ ] **Step 2: Run:** `cd frontend && npx vitest run src/domain/core/audit-log/usage/stores` → FAIL.
- [ ] **Step 3: Implementasi** `usage.store.ts`.
- [ ] **Step 4: Run** Step 2 → PASS.
- [ ] **Step 5: Commit** `feat(core-audit-log-usage-fe): store insight per bagian`.

### Task 9: FE komponen tabel, view, i18n

**Files:**
- Create: `usage/components/{SectionCard.svelte,InsightTabs.svelte,TopFilesTable.svelte,UnreadFilesTable.svelte,ZeroQueriesTable.svelte,KeyUsageTable.svelte,format.ts,format.test.ts,components.test.ts}`, `usage/views/{UsageInsightsView.svelte,UsageInsightsView.test.ts}`
- Modify: `frontend/src/shared/i18n/id.json`, `frontend/src/shared/i18n/en.json`

**Interfaces:**
- Consumes: `createUsageStore`, `usageFilterFromQuery`, `usageFilterKey`, `SectionState`; shared `Table`, `Button`, `EmptyState`, `ErrorState`; `formatDateTime(iso, locale)` dari `shared/utils/datetime`.
- Produces:
```ts
// format.ts
export function sectionError(e: ApiError | 'unavailable' | null, message: string): ApiError | null // 'unavailable' -> new ApiError({status:200, code:'UNAVAILABLE', message})
// SectionCard.svelte props: { title: string; description?: string; state: SectionState<unknown>; onretry: () => void; children: Snippet }
//   needs_project -> EmptyState(title=$t('dashboard.usage.needsProject'), description=$t('dashboard.usage.needsProject.hint'))
//   lainnya -> render children (Table menangani loading/error/empty via props loading, error=sectionError(...), emptyText, onretry)
//   wadah: <section class="rounded-card border border-border bg-surface p-4 sm:p-6"> judul text-base font-semibold
// InsightTabs.svelte props: { value: 'documents' | 'search'; onchange: (v) => void } — role="tablist", tombol role="tab" aria-selected, aria-controls; wadah "glass inline-flex gap-1 rounded-full p-1", aktif "bg-primary text-primary-fg shadow-button", lainnya "text-muted hover:bg-surface/70"; panah kiri/kanan berpindah tab
// TopFilesTable.svelte    props { state: SectionState<FileRead>; onretry }   kolom path(font-mono, truncate + title), reads, keys, last_read_at
// UnreadFilesTable.svelte props { state: SectionState<UnreadFile>; onretry } kolom path, updated_at; truncated -> catatan text-xs text-muted dashboard.usage.unread.truncated {n}
// ZeroQueriesTable.svelte props { state: SectionState<ZeroQuery>; onretry }  kolom query(font-mono), count, keys, last_at
// KeyUsageTable.svelte    props { state: SectionState<KeyUsage>; onretry }   kolom label, calls, errors, searches, reads, last_used_at, top_tool(font-mono)
// UsageInsightsView.svelte props { query: URLSearchParams } — judul h2 "dashboard.usage.title" + deskripsi redup; InsightTabs (Dokumen: TopFiles + Unread berdampingan lg:grid-cols-2; Pencarian: ZeroQueries); di bawah tab selalu KeyUsageTable; $effect pada usageFilterKey memanggil store.load; onDestroy dispose
```
Kunci i18n (id / en), semua di kedua file:
`dashboard.usage.title` Wawasan agent / Agent insights; `dashboard.usage.subtitle` Apa yang dibaca agent, apa yang tidak ia temukan, dan key mana yang memakainya. / What agents read, what they could not find, and which keys did it.; `dashboard.usage.tab.label` Jenis wawasan / Insight type; `dashboard.usage.tab.documents` Dokumen / Documents; `dashboard.usage.tab.search` Pencarian / Search; `dashboard.usage.topFiles` Dokumen paling sering dibaca / Most read documents; `dashboard.usage.unread` Dokumen tak pernah dibaca / Never read documents; `dashboard.usage.unread.truncated` Menampilkan {n} pertama. / Showing the first {n}.; `dashboard.usage.zeroQueries` Pencarian tanpa hasil / Searches with no results; `dashboard.usage.perKey` Pemakaian per API key / Usage per API key; `dashboard.usage.col.path` Path / Path; `.col.reads` Dibaca / Reads; `.col.keys` Key / Keys; `.col.lastRead` Terakhir dibaca / Last read; `.col.updated` Diperbarui / Updated; `.col.query` Query / Query; `.col.count` Kejadian / Occurrences; `.col.lastAt` Terakhir / Last seen; `.col.key` Key / Key; `.col.calls` Panggilan / Calls; `.col.errors` Error / Errors; `.col.searches` Pencarian / Searches; `.col.lastUsed` Terakhir dipakai / Last used; `.col.topTool` Tool teratas / Top tool; `dashboard.usage.empty.topFiles` Belum ada dokumen yang dibaca pada rentang ini / No documents read in this range; `dashboard.usage.empty.unread` Semua dokumen sudah dibaca pada rentang ini / Every document was read in this range; `dashboard.usage.empty.zeroQueries` Tidak ada pencarian tanpa hasil / No searches without results; `dashboard.usage.empty.perKey` Belum ada panggilan dari API key / No API key calls yet; `dashboard.usage.needsProject` Pilih project / Pick a project; `dashboard.usage.needsProject.hint` Pilih project di filter atas untuk melihat daftar ini. / Pick a project in the filter above to see this list.; `dashboard.usage.unavailable` Bagian ini gagal dimuat. / This section failed to load.

- [ ] **Step 1: Tulis tes gagal** (`setLocale('id')`, render dengan store palsu atau `fetch` stub)
```ts
// format.test.ts
it('maps unavailable to ApiError with message')
// components.test.ts
it('T19-AC4 loading: Table shows spinner/aria-busy')            // state.status 'loading'
it('T19-AC4 empty: shows empty text per table')                 // TopFilesTable empty -> 'Belum ada dokumen yang dibaca pada rentang ini'
it('T19-AC4 error per section: ErrorState + retry calls onretry') // 'unavailable' -> 'Bagian ini gagal dimuat.'; klik 'Coba lagi' -> onretry 1x
it('T19-AC4 needs project: SectionCard shows Pilih project')
it('renders path in monospace with title attr')                // td.font-mono[title="docs/a.md"]
it('unread truncated note')                                    // truncated true, 50 item -> 'Menampilkan 50 pertama.'
it('InsightTabs keyboard')                                     // ArrowRight memindah aria-selected ke Pencarian
// UsageInsightsView.test.ts
it('no project: no request for top_files/unread_files')        // fetch URL tidak pernah memuat section=top_files
it('with project: four requests, one failing keeps others')    // keys 500 -> tabel key ErrorState, tabel lain berisi
it('switching tab shows zero queries table')
it('re-loads on query change')                                 // rerender query range=30d -> fetch baru range=30d
```
- [ ] **Step 2: Run:** `cd frontend && npx vitest run src/domain/core/audit-log/usage src/shared/i18n` → FAIL (komponen belum ada; `i18n-keys.test.ts` belum ada kunci).
- [ ] **Step 3: Implementasi** komponen, view, dan kunci i18n.
- [ ] **Step 4: Run** Step 2 → PASS; `cd frontend && npm run lint && npm run depcruise` → bersih.
- [ ] **Step 5: Commit** `feat(core-audit-log-usage-fe): tabel wawasan agent dan i18n id/en`.

### Task 10: Integrasi di DashboardView

**Files:**
- Modify: `frontend/src/app/views/DashboardView.svelte`, `frontend/src/app/views/DashboardView.test.ts`

**Interfaces:**
- Consumes: `UsageInsightsView` dari `../../domain/core/audit-log/usage` (import statis; feature ini tanpa Chart.js).
- Produces: setelah blok `{#await chunk}...{/await}`, `<div class="mt-8"><UsageInsightsView {query} /></div>`; tetap dirender walau chunk summary gagal.

- [ ] **Step 1: Tulis tes gagal di `DashboardView.test.ts`** (perluas `fetchMock`: URL `/api/audit-log/usage` → mock per `section`)
```ts
it('renders Wawasan agent below charts for super_admin')   // heading 'Wawasan agent' ada setelah PageHeader
it('usage still renders when summary chunk fails')         // load reject -> ErrorState chunk + heading 'Wawasan agent'
it('non super_admin triggers no /api/audit-log/usage request')
it('usage chunk does not import chart.js')                 // depcruise/grep: tidak ada import 'chart.js' di domain/core/audit-log/usage (assert lewat import.meta.glob string scan)
```
- [ ] **Step 2: Run:** `cd frontend && npx vitest run src/app/views/DashboardView.test.ts` → FAIL.
- [ ] **Step 3: Implementasi** perubahan `DashboardView.svelte`.
- [ ] **Step 4: Run** Step 2 → PASS; `cd frontend && npm run build` sukses dan chunk utama tidak memuat Chart.js (bandingkan ukuran chunk dengan sebelum perubahan, catat di PR).
- [ ] **Step 5: Commit** `feat(core-audit-log-usage-fe): bagian wawasan agent di Dashboard`.

### Task 11: Tiket T19, README tiket, verifikasi akhir

**Files:**
- Create: `docs/tickets/T19-core-audit-log-usage-insights.md` (isi spec + bagian "Catatan plan" berisi P1-P8 + rujukan plan ini)
- Modify: `docs/tickets/README.md`

- [ ] **Step 1: README.** Tambah baris tabel tiket: `| [T19](T19-core-audit-log-usage-insights.md) | core | audit-log (insight pemakaian agent: dokumen, query kosong, per API key) | M | T02, T11, T15, T18 | Plan siap |`. Tree: di bawah `T15 core/audit-log summary` tambah `└── T19 core/audit-log usage insights   + butuh T18`. Gelombang: baris `10 | T19`. Katalog bus: kolom Consumer `project.knowledge.list-files` jadi `T08, T19 (core/auditlog usage)`, `project.management.get-project` jadi `T07, T19 (core/auditlog usage)`. Bila P6 berlaku: tambah `mcp.read_section` ke katalog aksi Agent. Catatan revisi T19 singkat: P1, P3 (404 vs `/summary`), kontrak `list-files` tidak diubah karena `UpdatedAt` sudah ada.
- [ ] **Step 2: Verifikasi penuh.** Run: `make api-bundle && make api-check && make lint && make test && make test-integration` → semua exit 0; `git status` tidak menyisakan `api/openapi.yml` basi.
- [ ] **Step 3: Commit**
```bash
git add docs/tickets/T19-core-audit-log-usage-insights.md docs/tickets/README.md
git commit -m "docs(tickets): T19 insight pemakaian agent

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

## Cakupan kriteria penerimaan

| Kriteria spec | Task / tes |
|---|---|
| Baca ganda satu key = 1 key; query beda kapital = satu; `read_section` dihitung | Task 3 `TestT19_AC1_*` |
| Unread = file tanpa audit, bukan file terhapus | Task 1 `TestT19_AC2_BuildUnreadExcludesDeleted`, Task 5 `TestT19_HTTP_EndToEndShape` |
| 403 non-super-admin, 404 project tak dikenal | Task 4 `TestT19_AC3_UnknownProject404`, Task 5 `TestT19_AC3_HTTP_*` |
| UI loading, kosong, error per bagian, tanpa project | Task 9 `T19-AC4 *`, `UsageInsightsView.test.ts` |
| OpenAPI + make api-bundle/api-check/lint/test/test-integration | Task 6, Task 11 Step 2 |
| Anti-tujuan: isi file bocor, SQL dari input, lambat, non-admin | Task 5 `NoContentLeakAndNoAudit`, Task 3 (placeholder + `ExplainUsesIndex`), Task 7 `rejects extra field`, Task 5 Authorization |
