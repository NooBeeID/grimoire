# T18 MCP Outline Tools Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Tiga tool MCP read-only baru (`get_file_outline`, `read_section`, `list_recent_changes`) supaya agent bisa membaca knowledge secara bertahap dan tahu apa yang berubah.

**Architecture:** Parser heading murni di `backend/lib/util/mdoutline` (dipakai lagi oleh T20). Ketiga tool adalah feature baru di module `discovery/knowledgetools` (satu folder per tool, pola `readfile`); rule (outline, section, merge perubahan) di `knowledgetools/base`. Data diambil lewat bus: perluasan aditif `project.knowledge.read-file` dan `project.apispec.list-specs`, plus satu bus baru `project.knowledge.list-changes` dengan indeks pendukung. Gateway T07 tetap mengurus key, timeout, rate limit, batas respons, dan audit.

**Tech Stack:** Go 1.x, sqlx + Postgres, `infra/bus`, `lib/contracts/discovery/mcpgateway/toolkit`, tes `go test` (unit) dan `-tags integration` (Postgres container dev).

**Spec:** `docs/superpowers/specs/2026-10-26-mcp-outline-tools-design.md`

## Global Constraints

- Semua tool read-only; gateway menambah `readOnlyHint: true` (rules 9.1).
- Skema `additionalProperties:false`, parse ketat lewat `base.DecodeStrict`; `project_id`, `projectId`, `project` ditolak dengan pesan "ditentukan oleh API key" (rules 7.4, `base.ProjectArgAliases`).
- Aksi audit: `mcp.get_file_outline`, `mcp.read_section`, `mcp.list_recent_changes`. Tool tidak menulis audit sendiri; `Summary` hanya metadata (`count`, plus `total` untuk `list_recent_changes`), tanpa isi file, tanpa nilai string selain `error_code` (rules 9.3, 10.3).
- `target_type`/`target`: `file` + path untuk `get_file_outline` dan `read_section`; `list_recent_changes` tanpa target (`""`/`""`).
- `read_section`: argumen `path`, `heading` (teks persis tanpa `#`), `occurrence` (default 1), `limit` (byte, default `base.DefaultReadLimit(max)`, maks `base.MaxReadLimit(MCP_MAX_RESPONSE_BYTES)`), tidak memotong di tengah rune UTF-8. Heading tidak ketemu = `IsError` dengan daftar heading mirip, bukan error Go.
- `list_recent_changes`: `since` wajib (RFC3339 atau `<n>h`/`<n>d`), jendela maks 90 hari; `kind` `files|specs|all` default `all`; `path` prefix file; `limit` 1..50 default 20; urut `updated_at` menurun; baris: `kind`, `path` atau `name`, `updated_at`, `created` (`created_at == updated_at`), `size_bytes`. `updated_by_label` TIDAK dikembalikan (K5).
- Parser: ATX `#`..`######` dan Setext; abaikan fenced code (```` ``` ```` dan `~~~`) dan blok HTML; baris dihitung dari 1. File non-Markdown: outline kosong + `size_bytes`, bukan error.
- Nilai di luar rentang DITOLAK, bukan dipotong (konvensi tool knowledge).
- Bus: nama `<domain>.<module>.<aksi>` dengan tanda hubung; kontrak di `lib/contracts/...` hanya stdlib; handler provider cek tipe dengan `bus.Expect` dan membatasi query pada `ProjectID` (rules 6.4, 6.5, 6.9, 6.10).
- Migrasi forward-only `backend/migrations/project/YYYYMMDDHHMMSSmmm_<nama>.sql`; query berat dibuktikan EXPLAIN (rules 8.3, 8.8).
- Tanpa endpoint REST baru, jadi `api/modules/**` tidak berubah.

## Keputusan K6 (dipilih dari kode nyata)

| Kebutuhan | Keputusan | Alasan |
|---|---|---|
| Isi utuh + `updated_at` untuk outline/section | **Perluas** `project.knowledge.read-file`: `Request.Whole bool`, `Response.UpdatedAt time.Time` | Provider sekarang memotong `Limit` ke `MCPMaxResponseBytes` (`busreadfile.effectiveLimit`), padahal file boleh sampai `MAX_FILE_BYTES` (1 MiB). Paging read-file butuh ~11 panggilan dan offset bisa bergeser bila file diedit di tengah. Bus outline baru di provider akan memindah parser ke domain project dan menduplikasi logika `Similar`. Perluasan aditif: `Whole=false` (nilai nol) = perilaku lama, consumer `read_file` tidak berubah. |
| Perubahan file sejak T | **Bus baru** `project.knowledge.list-changes` (kontrak `lib/contracts/project/knowledge/listchanges`) + indeks `(project_id, updated_at DESC)` | `list-files` urut path, tanpa filter waktu, tanpa `created_at`, dan maks 500 per halaman untuk 5000 file per project. Nama memakai tanda hubung (rules 6.4); `listchanges` adalah nama package. |
| Perubahan spec sejak T | **Perluas** `project.apispec.list-specs`: `Spec.CreatedAt time.Time`, `Spec.SizeBytes int` | `base.SpecSummary` sudah memuat dua kolom itu; spec per project maks `APISPEC_MAX_SPECS` (100), jadi filter `since` di entity tool murah. Bus `project.apispec.list-changes` tidak dibuat (YAGNI, rules 1.5). |
| Tempat `list_recent_changes` | Module `knowledgetools` (bukan `apispectools`) | Satu tool = satu feature = satu folder; tool ini membaca dua bus, dan module tool boleh memanggil bus mana pun lewat kontrak. |

## Review Focus

1. YAML front matter (`---\ntitle: x\n---`) di awal file: baris penutup `---` tidak boleh menjadikan `title: x` heading Setext h2. Tes: `TestParse_FrontMatterSkipped` (Task 1).
2. `---` setelah baris kosong atau setelah item list (`- a`) adalah thematic break, bukan heading Setext. Tes: `TestParse_ThematicBreakNotSetext` (Task 1).
3. File CRLF dan file ber-BOM: heading tetap terdeteksi, `Text` tanpa `\r`/BOM, offset byte dihitung dari isi mentah sehingga `read_section` mengiris tepat. Tes: `TestParse_CRLFAndBOM` (Task 1) dan `TestNewSection_CRLF` (Task 7).
4. File dengan ribuan heading: outline dipotong eksplisit dengan penanda, bukan dipotong generik oleh gateway, dan `Structured` hanya berisi heading yang ditampilkan. Tes: `TestNewOutline_BudgetTruncates` (Task 6).
5. Banyak file berubah dalam satu transaksi (upload/git-sync, `updated_at` identik): urutan stabil `updated_at DESC, path COLLATE "C"`, merge file+spec stabil (`file` sebelum `spec`). Tes: `TestRepo_TieBreakByPath` (Task 4) dan `TestMergeChanges_StableTieBreak` (Task 8).

---

### Task 1: Parser `lib/util/mdoutline`

**Files:**
- Create: `backend/lib/util/mdoutline/mdoutline.go`
- Create: `backend/lib/util/mdoutline/mdoutline_test.go`
- Create: `backend/lib/util/mdoutline/testdata/{fence.md,setext.md,duplicate.md,noheading.md,frontmatter.md,html.md}`

**Interfaces:**
- Consumes: stdlib saja (rules 4.9).
- Produces:
```go
type Heading struct {
	Level int    // 1..6 (Setext: '=' -> 1, '-' -> 2)
	Text  string // trim spasi; ATX tanpa penutup " ###"; Setext multi-baris digabung satu spasi; tanpa \r/BOM
	Line  int    // baris pertama heading, mulai 1 (Setext: baris pertama paragraf)
	Start int    // offset byte awal baris heading di isi mentah
	End   int    // offset byte eksklusif: Start heading berikutnya dengan Level <= Level ini, atau len(content)
}
func Parse(content string) []Heading                                      // tidak pernah nil
func Find(hs []Heading, text string, occurrence int) (h Heading, count int, ok bool) // cocok persis setelah TrimSpace(text); ok = 1 <= occurrence <= count
```

Aturan yang harus diikuti implementasi (tidak ditentukan tes satu per satu):
- Baris dipisah `\n`; `\r` di akhir baris dibuang untuk deteksi, tidak untuk offset. BOM `﻿` di awal isi diabaikan untuk deteksi.
- Front matter: bila baris 1 tepat `---`, lewati sampai baris `---` atau `...` berikutnya (tanpa penutup = bukan front matter).
- ATX: indentasi 0-3 spasi, 1-6 `#`, lalu spasi/tab atau akhir baris (`#tag`, `#!/bin/sh` bukan heading). Heading kosong (`#`) tetap heading dengan `Text ""`.
- Indentasi >= 4 kolom (tab = 4) di luar paragraf = indented code, bukan heading.
- Fence: 0-3 spasi lalu >= 3 `` ` `` atau `~`; ditutup oleh karakter sama dengan panjang >= pembuka dan hanya spasi sesudahnya; fence tak tertutup berlaku sampai EOF.
- HTML: baris diawali (0-3 spasi) `<!--` diabaikan sampai baris memuat `-->`; baris diawali `<` + huruf atau `</` diabaikan sampai baris kosong.
- Setext: underline `=+` / `-+` (0-3 spasi, spasi di belakang boleh) hanya bila baris sebelumnya bagian paragraf (tidak kosong, bukan heading, bukan fence/HTML/code, bukan item list `- `/`* `/`+ `/`1. `, bukan `>`).

- [ ] **Step 1: Tulis tes gagal**

```go
func TestT18_AC1_FenceIgnored(t *testing.T)        // testdata/fence.md: "# Real", ```` ```sh\n# not\n``` ````, "~~~\n## not\n~~~", "````\n```\n# not\n````", "## After"
	// want texts == []string{"Real", "After"}; levels {1,2}
func TestT18_AC1_Setext(t *testing.T)              // setext.md: "Judul\n=====\n\nSub\n---\n"
	// want {Level:1,Text:"Judul",Line:1,Start:0}, {Level:2,Text:"Sub",Line:4}
func TestParse_SetextMultiLine(t *testing.T)       // "a\nb\n===\n" -> {Level:1, Text:"a b", Line:1, Start:0}
func TestT18_AC1_DuplicateHeadings(t *testing.T)   // duplicate.md: "# Intro\n## Setup\nx\n# Intro\n## Setup\ny\n"
	// Find(hs,"Setup",2) -> ok, count 2, h.Line == 5; Find(hs,"Setup",3) -> ok false, count 2
func TestT18_AC1_NoHeadings(t *testing.T)          // noheading.md dan "" -> len 0, not nil
func TestParse_SectionEnds(t *testing.T)           // "# A\nx\n## A1\ny\n# B\nz\n": A.End == B.Start; A1.End == B.Start; B.End == len
func TestParse_ATXEdgeCases(t *testing.T)          // "#tag", "#!/bin/sh", "    # code", "\t# code" -> 0 heading;
	// "  ## Two ##  " -> Text "Two"; "####### seven" -> 0; "#" -> Text ""
func TestParse_HTMLBlockIgnored(t *testing.T)      // html.md: "<!--\n# no\n-->\n<div>\n# no\n</div>\n\n# yes" -> {"yes"}
func TestParse_FrontMatterSkipped(t *testing.T)    // "---\ntitle: x\n---\n# Real\n" -> {"Real"} Line 4
func TestParse_ThematicBreakNotSetext(t *testing.T) // "a\n\n---\n- item\n---\n" -> 0 heading
func TestParse_CRLFAndBOM(t *testing.T)            // c := "﻿# A\r\nx\r\n## B\r\n": Text "A","B"; A.Start == 0; B.Start == len("﻿# A\r\nx\r\n") (11); A.End == B.End == len(c)
func TestFind_TrimsArgument(t *testing.T)          // Find(hs,"  Intro ",1) ok
```

- [ ] **Step 2: Jalankan, pastikan gagal**

Run: `cd backend && go test ./lib/util/mdoutline/...`
Expected: FAIL (`undefined: Parse`).

- [ ] **Step 3: Implementasi `Parse` dan `Find` di `mdoutline.go`**

Satu pass baris-per-baris dengan state (`inFence`, `fenceChar`, `fenceLen`, `inHTML`, `htmlComment`, `paraStart`, `paraLine`, `paraText`), lalu pass kedua mengisi `End` (scan maju mencari `Level <=`). Doc comment package menyebut aturan di atas dan bahwa T20 memakainya.

- [ ] **Step 4: Jalankan, pastikan lulus**

Run: `cd backend && go test -race ./lib/util/mdoutline/... && go test ./internal/archtest/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/lib/util/mdoutline
git commit -m "feat(discovery-mcp-outline-tools): parser heading Markdown mdoutline"
```

---

### Task 2: Aksi audit dan daftar aksi

**Files:**
- Modify: `backend/lib/contracts/core/audit/record/actions.go` (konstanta + `Actions()`)
- Modify: `backend/lib/contracts/core/audit/record/actions_test.go` (`catalog`, jumlah 46 -> 49)
- Modify: `backend/internal/domain/core/internal/auditlog/base/summary_test.go:128` (`len(ToolActions()) != 6` -> `9`)
- Modify: `frontend/src/domain/core/audit-log/listentries/types.ts` (grup `mcp`)
- Modify: `frontend/src/domain/core/audit-log/listentries/components/components.test.ts:206` (tambahkan tiga aksi ke `arrayContaining`)

**Interfaces:**
- Produces: `record.ActionMCPGetFileOutline = "mcp.get_file_outline"`, `record.ActionMCPReadSection = "mcp.read_section"`, `record.ActionMCPListRecentChanges = "mcp.list_recent_changes"`; ketiganya di `Actions()` setelah `ActionMCPGetAPIEndpoint`. Dashboard (`auditlog/base.ToolActions()`) otomatis memuatnya.

- [ ] **Step 1: Tes gagal**: tambah tiga literal ke `catalog`, ubah `46` -> `49`; di `summary_test.go` ubah ke `9` dan tambah assertion `slices.Contains(ToolActions(), "mcp.read_section")`; di `components.test.ts` tambah `'mcp.get_file_outline', 'mcp.read_section', 'mcp.list_recent_changes'`.
- [ ] **Step 2:** Run `cd backend && go test ./lib/contracts/core/audit/record/ ./internal/domain/core/internal/auditlog/base/` -> FAIL (49 vs 46). `cd frontend && npm test -- --run src/domain/core/audit-log` -> FAIL.
- [ ] **Step 3:** Tambah konstanta + entri `Actions()`; tambah tiga string ke `ACTION_GROUPS[mcp]` di `types.ts`.
- [ ] **Step 4:** Ulangi dua perintah Step 2 -> PASS.
- [ ] **Step 5: Commit** `feat(discovery-mcp-outline-tools): aksi audit mcp.get_file_outline, mcp.read_section, mcp.list_recent_changes`

---

### Task 3: Perluasan `project.knowledge.read-file` (Whole + UpdatedAt)

**Files:**
- Modify: `backend/lib/contracts/project/knowledge/readfile/readfile.go`
- Modify: `backend/internal/domain/project/internal/knowledge/busreadfile/usecase.go`
- Test: `backend/internal/domain/project/internal/knowledge/busreadfile/usecase_test.go`, `busreadfile_integration_test.go`

**Interfaces:**
- Produces (aditif):
```go
type Request struct {
	ProjectID string
	Path      string
	Offset    int
	Limit     int
	Whole     bool // true: kembalikan isi utuh (<= MAX_FILE_BYTES), abaikan Limit dan clamp MaxReadBytes; Offset WAJIB 0
}
type Response struct {
	// field lama tetap
	UpdatedAt time.Time // UTC, diisi bila Found
}
```
Doc comment package: tambah kalimat "Whole hanya untuk consumer yang tidak meneruskan isi mentah ke klien (T18 outline/section); read_file tidak memakainya."

- [ ] **Step 1: Tes gagal** (fake `files` yang ada di `usecase_test.go`)
```go
func TestWholeReturnsEntireFileBeyondMaxRead(t *testing.T) // Limits{MaxReadBytes:10}, content 100 byte, Whole:true
	// Content len 100, TotalBytes 100, NextOffset 100, Truncated false
func TestWholeWithOffsetInvalid(t *testing.T)   // Whole:true, Offset:5 -> errors.Is(err, errs.ErrInvalid)
func TestUpdatedAtFilled(t *testing.T)          // file.UpdatedAt = ts -> resp.UpdatedAt.Equal(ts) && Location()==UTC (Whole dan non-Whole)
func TestIntegrationWholeAndUpdatedAt(t *testing.T) // (integration) file 200 KiB, Whole -> len == 200 KiB, UpdatedAt tidak nol
```
- [ ] **Step 2:** Run `cd backend && go test ./internal/domain/project/internal/knowledge/busreadfile/` -> FAIL (`unknown field Whole`).
- [ ] **Step 3:** Di `Usecase.Execute`: setelah found, cabang `req.Whole` (Offset != 0 -> `base.InvalidMsg(base.ReasonInvalidOffset, "whole tidak bisa digabung dengan offset")`; selain itu isi utuh). Isi `UpdatedAt: f.UpdatedAt` di kedua cabang.
- [ ] **Step 4:** Run `cd backend && go test -race ./internal/domain/project/internal/knowledge/busreadfile/ ./internal/domain/discovery/...` -> PASS; `make test-integration` cukup untuk paket ini: `cd backend && TEST_DATABASE_URL=postgres://grimoire:grimoire@localhost:5433/grimoire_test?sslmode=disable go test -race -tags integration ./internal/domain/project/internal/knowledge/busreadfile/` -> PASS.
- [ ] **Step 5: Commit** `feat(discovery-mcp-outline-tools): read-file Whole dan UpdatedAt (aditif)`

---

### Task 4: Bus baru `project.knowledge.list-changes` + indeks

**Files:**
- Create: `backend/lib/contracts/project/knowledge/listchanges/listchanges.go`
- Modify: `backend/lib/constants/bus.go`
- Create: `backend/migrations/project/20261026090000000_knowledge_files_updated_idx.sql`
- Create: `backend/internal/domain/project/internal/knowledge/buslistchanges/{init.go,handler.go,usecase.go,repository.go}`
- Test: `.../buslistchanges/{usecase_test.go,handler_test.go,repository_integration_test.go}`
- Modify: `backend/internal/domain/project/internal/knowledge/base.go` (tambah `buslistchanges.Init(d)` setelah `buslistfiles.Init(d)`)

**Interfaces:**
- Produces:
```go
// lib/constants/bus.go
// Provider: T18 project/knowledge (buslistchanges). Consumer: T18 discovery/knowledgetools (list_recent_changes).
// Kontrak: lib/contracts/project/knowledge/listchanges. Baca-saja, dibatasi ProjectID (rule 6.10).
const BUS_PROJECT_KNOWLEDGE_LIST_CHANGES = "project.knowledge.list-changes"

// package listchanges
const (
	DefaultLimit = 20
	MaxLimit     = 50
)
type Request struct {
	ProjectID string
	Since     time.Time // wajib (nol -> errs.ErrInvalid); filter updated_at >= Since
	Prefix    string    // "" = semua; satu "/" di depan/belakang ditoleransi
	Limit     int       // 0 -> DefaultLimit; > MaxLimit -> MaxLimit
}
type Item struct {
	Path               string
	SizeBytes          int
	CreatedAt, UpdatedAt time.Time // UTC
}
type Response struct {
	Items []Item // urut UpdatedAt DESC, Path byte order ASC; tidak pernah nil
	Total int    // semua baris cocok, bukan hanya halaman
}

// buslistchanges
type Repository interface {
	ListChanges(ctx context.Context, projectID string, since time.Time, likePrefix string, limit int) ([]listchanges.Item, int, error)
}
func New(repo Repository) *Usecase
func (u *Usecase) Execute(ctx context.Context, in listchanges.Request) (listchanges.Response, error)
func NewRepo(s *store.Store) *Repo
func Init(d app.Deps)
```
- SQL (`repository.go`, konstanta `listChangesSQL`):
```sql
SELECT path, size_bytes, created_at, updated_at, count(*) OVER () AS total
FROM knowledge_files
WHERE project_id = $1 AND updated_at >= $2 AND path LIKE $3 ESCAPE '\'
ORDER BY updated_at DESC, path COLLATE "C"
LIMIT $4
```
- Migrasi: `-- T18: list-changes (project.knowledge.list-changes) urut updated_at per project.` lalu `CREATE INDEX IF NOT EXISTS knowledge_files_project_updated_idx ON knowledge_files (project_id, updated_at DESC);`

Validasi di usecase mengikuti `buslistfiles`: `uuid.Parse` -> `base.Invalid(base.ReasonInvalidProjectID)`; `base.NormalizePrefix(base.StripLeadingSlash(...))` -> `base.InvalidMsg`; `base.LikePrefix`; `Since.IsZero()` -> `base.InvalidMsg(..., "since wajib diisi")`.

- [ ] **Step 1: Tes gagal**
```go
func TestExecute_ForwardsAndClampsLimit(t *testing.T) // Limit 0 -> 20, 999 -> 50; like == base.LikePrefix("docs/")
func TestExecute_InvalidInputs(t *testing.T)          // ProjectID "x", Since nol, Prefix "a/../b" -> errors.Is(err, errs.ErrInvalid), repo tidak dipanggil
func TestExecute_EmptyItemsNotNil(t *testing.T)
func TestHandle_WrongRequestType(t *testing.T)        // &Request{}, "s", nil -> bus.ErrBadRequestType
func TestInit_RegistersEndpoint(t *testing.T)
// repository_integration_test.go (//go:build integration), env seperti buslistfiles
func TestRepo_SinceFilterAndOrder(t *testing.T)       // 3 file, satu updated_at lama -> 2 item, urut desc, Total 2
func TestRepo_TieBreakByPath(t *testing.T)            // updated_at identik -> path "B.md" sebelum "a.md" (byte order)
func TestRepo_CreatedFlagSource(t *testing.T)         // insert baru: CreatedAt.Equal(UpdatedAt); setelah UPDATE: tidak sama
func TestRepo_IsolationAndPrefix(t *testing.T)        // project lain tidak muncul; prefix "docs" hanya docs/**; "_" literal
func TestRepo_TotalBeyondLimit(t *testing.T)          // 5 cocok, limit 2 -> len 2, Total 5
func TestExplainUsesUpdatedIndex(t *testing.T)        // 5000 baris updated_at now()-60d + 10 baris now(), ANALYZE;
	// EXPLAIN listChangesSQL dengan since now()-1h -> strings.Contains(plan, "knowledge_files_project_updated_idx"); t.Logf(plan)
```
- [ ] **Step 2:** Run `cd backend && go test ./internal/domain/project/internal/knowledge/buslistchanges/` -> FAIL (paket belum ada).
- [ ] **Step 3:** Buat kontrak, konstanta, migrasi, feature (handler `bus.Expect[listchanges.Request]`, error -> `nil, err`), wiring `base.go`.
- [ ] **Step 4:** Run `cd backend && go test -race ./internal/domain/project/... ./internal/archtest/...` -> PASS; `cd backend && TEST_DATABASE_URL=postgres://grimoire:grimoire@localhost:5433/grimoire_test?sslmode=disable go test -race -tags integration ./internal/domain/project/internal/knowledge/buslistchanges/ -v -run 'TestRepo|TestExplain'` -> PASS dan plan tercetak (tempel ke PR untuk rules 8.8).
- [ ] **Step 5: Commit** `feat(discovery-mcp-outline-tools): bus project.knowledge.list-changes dan indeks updated_at`

---

### Task 5: Perluasan `project.apispec.list-specs` (CreatedAt + SizeBytes)

**Files:**
- Modify: `backend/lib/contracts/project/apispec/listspecs/listspecs.go`
- Modify: `backend/internal/domain/project/internal/apispec/listspecsbus/usecase.go`
- Test: `backend/internal/domain/project/internal/apispec/listspecsbus/usecase_test.go`

**Interfaces:**
- Produces (aditif): `type Spec struct { Name, Title, ApiVersion, OpenapiVersion string; EndpointCount int; UpdatedAt time.Time; CreatedAt time.Time; SizeBytes int }` (CreatedAt UTC).

- [ ] **Step 1: Tes gagal**: di `TestExecute_MapsFields` isi `CreatedAt: ts2, SizeBytes: 321` pada `base.SpecSummary` dan pada `want` (`CreatedAt: ts2.UTC(), SizeBytes: 321`); tambah cek `res.Items[0].CreatedAt.Location() == time.UTC`.
- [ ] **Step 2:** Run `cd backend && go test ./internal/domain/project/internal/apispec/listspecsbus/` -> FAIL.
- [ ] **Step 3:** Petakan dua field di loop `Execute`.
- [ ] **Step 4:** Run `cd backend && go test -race ./internal/domain/project/internal/apispec/... ./internal/domain/discovery/internal/apispectools/...` -> PASS (golden apispectools tidak berubah).
- [ ] **Step 5: Commit** `feat(discovery-mcp-outline-tools): list-specs membawa CreatedAt dan SizeBytes (aditif)`

---

### Task 6: Tool `get_file_outline`

**Files:**
- Modify: `backend/internal/domain/discovery/internal/knowledgetools/base/names.go`
- Create: `backend/internal/domain/discovery/internal/knowledgetools/base/outline.go`, `outline_test.go`
- Create: `backend/internal/domain/discovery/internal/knowledgetools/getfileoutline/{init.go,params.go,handler.go,usecase.go,repository.go}` + `{params_test.go,usecase_test.go,handler_test.go}`

**Interfaces:**
- Consumes: `mdoutline.Parse` (Task 1); `rf.Request{Whole:true}` / `rf.Response.UpdatedAt` (Task 3); `base.NewNotFound`, `base.ArgResult`, `base.BusResult`, `base.NormalizePath`, `base.DecodeStrict`, `base.HumanSize`, `base.FormatTime`, `base.RFC3339`, `base.MaxReadLimit`.
- Produces:
```go
// base/names.go
const (
	ToolGetFileOutline    = "get_file_outline"
	ToolReadSection       = "read_section"
	ToolListRecentChanges = "list_recent_changes"
)
// base/outline.go
func IsMarkdown(path string) bool // ekstensi .md atau .markdown, case-insensitive
type Outline struct {
	Path      string
	SizeBytes int
	UpdatedAt time.Time
	Markdown  bool
	Headings  []mdoutline.Heading // semua
	Shown     int                 // jumlah yang muat budget
}
func NewOutline(path string, resp readfile.Response, budget int) Outline // parse hanya bila IsMarkdown
func (o Outline) Text() string
func (o Outline) Structured() map[string]any // path, size_bytes, updated_at (RFC3339), markdown, total_headings, truncated, headings[{level,text,line,start,end}] (hanya Shown)
func (o Outline) Summary() map[string]any    // {"count": Shown, "total": len(Headings), "bytes": SizeBytes, "truncated": bool}
func (o Outline) Result() toolkit.Result     // TargetType "file", Target Path, implement base.ReadOutcome
// getfileoutline
type Params struct{ Path string }
func Parse(raw json.RawMessage) (Params, *base.ArgError) // allowed: path
type Reader interface{ Read(ctx context.Context, req rf.Request) (rf.Response, error) }
func (u *Usecase) Execute(ctx context.Context, projectID string, p Params) (base.ReadOutcome, error)
func Init(d app.Deps) toolkit.Tool // budget = base.MaxReadLimit(d.Config.MCPMaxResponseBytes)
```
Format teks (dipin tes):
- Header: `<path> (<HumanSize>, diperbarui <FormatTime>): <N> heading`
- Baris heading: `L<line> <"#"*level> <text> [<start>-<end>]`, diindentasi dua spasi per level di atas 1.
- Penutup: `Baca satu bagian dengan read_section(path, heading); heading ganda pakai occurrence.`
- Terpotong: `[outline terpotong: menampilkan <Shown> dari <total> heading; sisanya mulai byte <Start heading ke-Shown+1>, baca dengan read_file offset=<itu>]`
- Tanpa heading (Markdown): `Tidak ada heading Markdown di <path> (<HumanSize>). Baca dengan read_file.`
- Non-Markdown: `<path> bukan file Markdown; outline kosong (<HumanSize>). Baca dengan read_file.`
- Tool: `Title: "Kerangka file knowledge"`, Description diawali `Daftar heading satu file Markdown knowledge (read-only).` dan menyebut `read_section`.

- [ ] **Step 1: Tes gagal**
```go
// base/outline_test.go
func TestIsMarkdown(t *testing.T)                 // a.md, A.MD, x.markdown true; a.txt, md false
func TestNewOutline_TextAndStructured(t *testing.T) // "# A\nx\n## B\n": Text memuat "L1 # A [0-", "  L3 ## B [", "2 heading"; Structured["headings"] len 2, updated_at RFC3339
func TestNewOutline_NonMarkdown(t *testing.T)     // path a.txt isi "# x": Shown 0, Text memuat "bukan file Markdown", Summary["count"]==0, !IsError
func TestNewOutline_NoHeadings(t *testing.T)
func TestNewOutline_BudgetTruncates(t *testing.T) // 5000 heading, budget 4096: len(Text) <= 4096, Shown < 5000, Text memuat "[outline terpotong:", len(Structured headings) == Shown
func TestOutline_SummaryHasNoContent(t *testing.T) // isi "SECRET-XYZ" sebagai heading: json(Summary) tidak memuat "SECRET-XYZ", tidak ada nilai string
// getfileoutline
func TestParse_Valid(t *testing.T)                // "/docs/a.md" -> "docs/a.md"
func TestParse_Invalid(t *testing.T)              // {}, folder "docs/", "..", project_id/projectId/project ("ditentukan oleh API key"), field asing ("path"), [] 
func TestExecute_RequestsWhole(t *testing.T)      // fake Reader menerima rf.Request{ProjectID:"p", Path:"docs/a.md", Whole:true}
func TestExecute_NotFound(t *testing.T)           // -> base.NotFound dengan Similar
func TestHandler_InvalidArgs_BusNotCalled(t *testing.T)
func TestHandler_ErrInvalid_BecomesIsError(t *testing.T) // Summary["error_code"]=="invalid_request"
func TestHandler_OtherError_IsGoError(t *testing.T)       // errors.Is(err, want); bus kosong -> bus.ErrNotRegistered
func TestInit_Tool(t *testing.T)                  // Name "get_file_outline", schema additionalProperties false, required ["path"], tanpa properti berisi "project"
```
- [ ] **Step 2:** Run `cd backend && go test ./internal/domain/discovery/internal/knowledgetools/...` -> FAIL.
- [ ] **Step 3:** Implementasi mengikuti pola `readfile` (repository.go = `bus.CallTyped[rf.Response](ctx, b, constants.BUS_PROJECT_KNOWLEDGE_READ_FILE, req)`); hint BusResult `"Periksa path dengan list_files."`.
- [ ] **Step 4:** Run `cd backend && go test -race ./internal/domain/discovery/internal/knowledgetools/base/ ./internal/domain/discovery/internal/knowledgetools/getfileoutline/` -> PASS.
- [ ] **Step 5: Commit** `feat(discovery-mcp-outline-tools): tool get_file_outline`

---

### Task 7: Tool `read_section`

**Files:**
- Create: `backend/internal/domain/discovery/internal/knowledgetools/base/section.go`, `section_test.go`
- Create: `backend/internal/domain/discovery/internal/knowledgetools/readsection/{init.go,params.go,handler.go,usecase.go,repository.go}` + `{params_test.go,usecase_test.go,handler_test.go}`

**Interfaces:**
- Consumes: `mdoutline.Parse`, `mdoutline.Find` (Task 1); `rf.Request{Whole:true}` (Task 3); `base.IsMarkdown` (Task 6); `base.TruncateUTF8`, `base.MaxReadLimit`, `base.DefaultReadLimit`, `base.IntArg`.
- Produces:
```go
// base/section.go
type Section struct {
	Path       string
	Heading    mdoutline.Heading
	Occurrence int
	Content    string // isi[Start : min(End, cut)], cut pada batas rune lewat TruncateUTF8
	Truncated  bool
	NextOffset int    // offset byte absolut untuk read_file bila Truncated, selain itu -1
}
func NewSection(path, content string, h mdoutline.Heading, occurrence, limit int) Section
func (s Section) Result() toolkit.Result // Text = Content (+ penanda); Structured {path,heading,level,line,occurrence,start,end,truncated,next_offset}; Summary {"count":1,"bytes":len(Content),"truncated":bool,"occurrence":n}
type HeadingMiss struct {
	Path, Heading     string
	Occurrence, Count int      // Count = jumlah heading persis sama (0 bila tidak ada)
	Similar           []string // maks 5, format "<#*level> <text> (L<line>)"
}
func SimilarHeadings(hs []mdoutline.Heading, text string, max int) []string
	// skor: sama case-insensitive (0) < salah satu memuat yang lain case-insensitive (1) < berbagi >=1 kata case-insensitive (2); tanpa skor dibuang; seri -> Line naik
func (m HeadingMiss) Result() toolkit.Result // IsError; Summary {"count":0,"error_code":"heading_not_found","similar_count":n}
type NotMarkdown struct{ Path string }
func (n NotMarkdown) Result() toolkit.Result // IsError; Summary {"count":0,"error_code":"not_markdown"}
// readsection
type Params struct {
	Path, Heading string
	Occurrence    int // default 1, 1..1000
	Limit         int // default base.DefaultReadLimit(max), 1..max
}
func Parse(raw json.RawMessage, maxLimit int) (Params, *base.ArgError) // allowed: path, heading, occurrence, limit; heading 1..500 byte setelah TrimSpace
func (u *Usecase) Execute(ctx context.Context, projectID string, p Params) (base.ReadOutcome, error)
	// Read Whole -> !Found: base.NewNotFound; !IsMarkdown: base.NotMarkdown; Find gagal: HeadingMiss{Count, Similar}; ok: NewSection
func Init(d app.Deps) toolkit.Tool
```
Teks yang dipin tes:
- Terpotong: `\n\n[bagian terpotong: lanjutkan dengan read_file offset=<NextOffset>; bagian berakhir di byte <End>]`
- Tidak ketemu: baris 1 `Heading tidak ditemukan di <path>: "<heading>"`; bila Similar ada `Mirip:` + `- ...`; penutup `Panggil get_file_outline untuk melihat semua heading.`
- Occurrence lewat: `Heading "<heading>" hanya muncul <Count> kali di <path>; gunakan occurrence 1 sampai <Count>.`
- Non-Markdown: `read_section hanya untuk file Markdown (.md, .markdown): <path>. Baca dengan read_file.`
- Tool: `Title: "Baca satu bagian file knowledge"`, Description diawali `Baca satu bagian file Markdown knowledge (read-only), dari heading sampai sebelum heading selevel atau lebih tinggi.`

- [ ] **Step 1: Tes gagal**
```go
// base/section_test.go
func TestT18_AC2_SectionExact(t *testing.T)     // "# A\nx\n## A1\ny\n# B\nz\n", A: Content == "# A\nx\n## A1\ny\n", !strings.Contains(Content,"# B")
func TestT18_AC2_SectionUTF8Cut(t *testing.T)   // heading + "日本語..." 3-byte, limit memotong di tengah rune: utf8.ValidString(Content), len(Content) <= limit, NextOffset == h.Start+len(Content), Truncated
func TestNewSection_LastSectionToEOF(t *testing.T)
func TestNewSection_CRLF(t *testing.T)          // "# A\r\nx\r\n# B\r\n": Content == "# A\r\nx\r\n"
func TestSimilarHeadings(t *testing.T)          // hs {"Setup","Setup Guide","Install"}, "setup" -> ["# Setup (L..)","# Setup Guide (L..)"]; "zzz" -> []
func TestHeadingMiss_Text(t *testing.T)         // memuat `Heading tidak ditemukan di docs/a.md: "Foo"` dan "get_file_outline"; Count>0 -> "hanya muncul 2 kali"
// readsection
func TestParse_Defaults(t *testing.T)           // {"path":"a.md","heading":" Setup "} -> Params{"a.md","Setup",1,40000} (max 101888)
func TestParse_Invalid(t *testing.T)            // heading hilang/kosong, occurrence 0/1001/"1", limit 0/max+1, project_id, field asing
func TestExecute_DuplicateOccurrence(t *testing.T) // isi duplicate.md, occurrence 2 -> Section.Heading.Line == 5
func TestExecute_OccurrenceTooHigh(t *testing.T)   // occurrence 3 -> HeadingMiss Count 2
func TestExecute_NotMarkdown(t *testing.T)
func TestExecute_NotFound(t *testing.T)
func TestHandler_SummaryHasNoContent(t *testing.T) // isi "SECRET-CONTENT-123" tidak ada di json(Summary)
func TestHandler_ProjectFromCallOnly(t *testing.T) // project_id ditolak; fake bus menerima ProjectID dari Call
func TestInit_Tool(t *testing.T)                // Name "read_section", required ["path","heading"], limit.maximum == MaxReadLimit(config)
```
- [ ] **Step 2:** Run `cd backend && go test ./internal/domain/discovery/internal/knowledgetools/...` -> FAIL.
- [ ] **Step 3:** Implementasi; hint BusResult `"Periksa path dengan list_files."`.
- [ ] **Step 4:** Run `cd backend && go test -race ./internal/domain/discovery/internal/knowledgetools/base/ ./internal/domain/discovery/internal/knowledgetools/readsection/` -> PASS.
- [ ] **Step 5: Commit** `feat(discovery-mcp-outline-tools): tool read_section`

---

### Task 8: Tool `list_recent_changes`

**Files:**
- Create: `backend/internal/domain/discovery/internal/knowledgetools/base/changes.go`, `changes_test.go`
- Create: `backend/internal/domain/discovery/internal/knowledgetools/listrecentchanges/{init.go,params.go,handler.go,usecase.go,repository.go}` + `{params_test.go,usecase_test.go,handler_test.go}`

**Interfaces:**
- Consumes: `listchanges.Request/Response`, `constants.BUS_PROJECT_KNOWLEDGE_LIST_CHANGES` (Task 4); `listspecs.Spec.CreatedAt/SizeBytes`, `constants.BUS_PROJECT_APISPEC_LIST_SPECS` (Task 5); `base.NormalizePrefix`, `base.IntArg`, `base.HumanSize`, `base.FormatTime`, `base.RFC3339`.
- Produces:
```go
// base/changes.go
const MaxChangesWindow = 90 * 24 * time.Hour
type Change struct {
	Kind      string // "file" | "spec"
	Path      string // kind file
	Name      string // kind spec
	UpdatedAt time.Time
	Created   bool   // CreatedAt.Equal(UpdatedAt)
	SizeBytes int
}
type ChangePage struct {
	Since time.Time
	Kind  string // "files" | "specs" | "all"
	Items []Change
	Total int
}
func MergeChanges(since time.Time, kind string, files listchanges.Response, specs []listspecs.Spec, limit int) ChangePage
	// specs difilter UpdatedAt >= since; gabung; urut UpdatedAt desc, lalu Kind ("file" < "spec"), lalu Path/Name byte order; ambil limit; Total = files.Total + jumlah spec cocok
func (p ChangePage) Result() toolkit.Result // TargetType "", Target ""; Structured {since, kind, total, truncated, items[{kind, path|name, updated_at, created, size_bytes}]}; Summary {"count":len(Items),"total":Total}
// listrecentchanges
type Params struct {
	Since  time.Time
	Kind   string
	Prefix string
	Limit  int
}
func Parse(raw json.RawMessage, now time.Time) (Params, *base.ArgError) // allowed: since, kind, path, limit
func ParseSince(raw string, now time.Time) (time.Time, *base.ArgError)
	// RFC3339 (time.RFC3339Nano) atau ^[1-9][0-9]*(h|d)$; hasil UTC; now-since > MaxChangesWindow -> tolak; since > now -> tolak
type FileChanges interface{ ListChanges(ctx context.Context, req listchanges.Request) (listchanges.Response, error) }
type SpecLister interface{ ListSpecs(ctx context.Context, req listspecs.Request) (listspecs.Response, error) }
func (u *Usecase) Execute(ctx context.Context, projectID string, p Params) (base.ChangePage, error)
	// kind files|all -> FileChanges(Since, Prefix, Limit); specs|all -> SpecLister; lalu base.MergeChanges
func Init(d app.Deps) toolkit.Tool
func newTool(d app.Deps, now func() time.Time) toolkit.Tool // dipakai tes; Init = newTool(d, time.Now)
```
Aturan argumen: `kind` enum `files|specs|all` (lain ditolak dengan daftar nilai); `path` dengan `kind: "specs"` ditolak (`path hanya berlaku untuk kind files atau all`); `limit` 1..50 default 20; `since` maxLength 64.
Teks yang dipin tes:
- Header: `<len> perubahan sejak <RFC3339 since> (total <Total>):`
- Baris: `- [file] <path> · dibuat|diperbarui · <HumanSize> · <FormatTime>` dan `- [spec] <name> · ...`
- Terpotong (`Total > len`): `[ada <Total-len> perubahan lagi; persempit since/path/kind atau naikkan limit (maks 50)]`
- Kosong: `Tidak ada perubahan sejak <RFC3339 since>.`
- Tool: `Title: "Perubahan terbaru"`, Description diawali `Daftar file knowledge dan spec API yang berubah sejak waktu tertentu (read-only).` dan menyebut `file yang dihapus tidak muncul`.

- [ ] **Step 1: Tes gagal**
```go
// base/changes_test.go
func TestMergeChanges_OrderAndLimit(t *testing.T)    // 2 file + 2 spec waktu berbeda, limit 3 -> urut desc, len 3, Total 4
func TestMergeChanges_StableTieBreak(t *testing.T)   // waktu sama: file "a.md" sebelum spec "a"; "B.md" sebelum "a.md"
func TestMergeChanges_SpecsFilteredBySince(t *testing.T)
func TestMergeChanges_CreatedFlag(t *testing.T)      // CreatedAt==UpdatedAt -> Created true; else false
func TestChangePage_ResultShape(t *testing.T)        // Target ""; Summary keys tepat {count,total}; Structured items tidak memuat "updated_by"; teks "dibuat"/"diperbarui"
// listrecentchanges
func TestParseSince(t *testing.T)                    // now tetap: "24h","7d","90d", RFC3339 ok; "91d","2161h", RFC3339 >90 hari, masa depan, "0h", "1w", "" ditolak dengan Field "since"
func TestParse_Defaults(t *testing.T)                // {"since":"7d"} -> Kind "all", Limit 20, Prefix ""
func TestParse_Invalid(t *testing.T)                 // since hilang, kind "x", limit 0/51, kind specs + path, project_id/projectId/project, field asing
func TestExecute_KindRouting(t *testing.T)           // files: hanya FileChanges dipanggil; specs: hanya SpecLister; all: keduanya; ProjectID dari argumen projectID
func TestExecute_ForwardsSincePrefixLimit(t *testing.T)
func TestHandler_ErrInvalid_BecomesIsError(t *testing.T)
func TestHandler_OtherError_IsGoError(t *testing.T)
func TestInit_Tool(t *testing.T)                     // Name "list_recent_changes", required ["since"], kind enum 3 nilai, limit maximum 50
```
- [ ] **Step 2:** Run `cd backend && go test ./internal/domain/discovery/internal/knowledgetools/...` -> FAIL.
- [ ] **Step 3:** Implementasi; `repository.go` berisi dua adapter bus (`bus.CallTyped[listchanges.Response]`, `bus.CallTyped[listspecs.Response]`); hint BusResult `"Periksa since dan path."`.
- [ ] **Step 4:** Run `cd backend && go test -race ./internal/domain/discovery/internal/knowledgetools/base/ ./internal/domain/discovery/internal/knowledgetools/listrecentchanges/` -> PASS.
- [ ] **Step 5: Commit** `feat(discovery-mcp-outline-tools): tool list_recent_changes`

---

### Task 9: Registrasi tool, golden `tools/list`, allowlist import

**Files:**
- Modify: `backend/internal/domain/discovery/internal/knowledgetools/base.go`
- Modify: `backend/internal/domain/discovery/internal/knowledgetools/base_test.go`
- Modify (regenerate): `backend/internal/domain/discovery/internal/knowledgetools/testdata/tools_list.golden.json`

**Interfaces:**
- Consumes: `getfileoutline.Init`, `readsection.Init`, `listrecentchanges.Init` (Task 6-8).
- Produces: `knowledgetools.Register(d) []toolkit.Tool` mengembalikan 6 tool. `discovery/base.go` tidak berubah (sudah `append(knowledgetools.Register(d)...)`).

- [ ] **Step 1: Tes gagal**: ganti `TestRegister_ThreeReadOnlyTools` menjadi `TestRegister_ReadOnlyTools` dengan `want := []string{"get_file_outline","list_files","list_recent_changes","read_file","read_section","search_knowledge"}`; tambah ke `exact` di `TestT08_AC7_ImportAllowlist`: `"grimoire/lib/util/mdoutline"`, `"grimoire/lib/contracts/project/knowledge/listchanges"`, `"grimoire/lib/contracts/project/apispec/listspecs"` (komentar `// T18`).
- [ ] **Step 2:** Run `cd backend && go test ./internal/domain/discovery/internal/knowledgetools/` -> FAIL (3 vs 6, golden beda).
- [ ] **Step 3:** Tambah tiga `Init(d)` ke slice di `Register`; perbarui doc comment ("enam tool"). Regenerate: `cd backend && go test ./internal/domain/discovery/internal/knowledgetools/ -run TestT08_AC6_ToolsListContract -update`, periksa `git diff` golden: hanya tiga entri baru, entri lama identik.
- [ ] **Step 4:** Run `cd backend && go test -race ./internal/domain/discovery/... ./internal/archtest/... ./cmd/...` -> PASS (gateway memvalidasi `mcp.<nama>` ada di `record.Actions()` saat start).
- [ ] **Step 5: Commit** `feat(discovery-mcp-outline-tools): daftarkan tiga tool dan perbarui golden tools/list`

---

### Task 10: Tes integrasi end-to-end (`mcpEnv`)

**Files:**
- Create: `backend/cmd/internal/server/knowledgetools_outline_integration_test.go` (`//go:build integration`)

**Interfaces:**
- Consumes: `setupMCP`, `e.mcpCall`, `e.seedKnowledge`, `e.newProject`, `e.asUpload`, `e.countAudit`, `e.rest` (sesi super admin), `listBody`.

- [ ] **Step 1: Tulis tes** (fixture: `docs/guide.md` = `"# Intro\nhalo 日本\n```sh\n# bukan\n```\n## Setup\nlangkah\n# Intro\n## Setup\nkedua\n"`; `notes.txt`; spec `petstore` lewat `asUpload`; project B dengan `secret/b.md`)
```go
func TestT18_AC3_ToolsListed(t *testing.T)            // tools/list memuat tiga nama + "readOnlyHint":true
func TestT18_Int_Outline(t *testing.T)                // get_file_outline docs/guide.md: !IsError, StructuredContent["updated_at"] != "", len(headings)==4 (heading di fence tidak ada);
	// notes.txt -> !IsError, headings kosong, size_bytes > 0; countAudit("mcp.get_file_outline") == 2
func TestT18_AC2_ReadSectionExact(t *testing.T)       // heading "Setup" occurrence 2 -> Text == "## Setup\nkedua\n"; heading "Intro" -> tidak memuat "kedua"; "Nope" -> IsError + "get_file_outline"
	// limit kecil memotong di "日本" -> utf8.ValidString(Text sebelum penanda); countAudit("mcp.read_section") == jumlah panggilan
func TestT18_Int_RecentChanges(t *testing.T)          // since "1h" kind all -> items memuat file docs/guide.md (created true) dan spec petstore; tidak memuat secret/b.md;
	// update file via REST PUT lalu panggil lagi -> created false; kind specs + path -> IsError; audit target kosong: SELECT target FROM audit_log WHERE action='mcp.list_recent_changes' -> ""
func TestT18_AntiGoal_ProjectArgRejected(t *testing.T) // tiap tool dengan {"project_id": projB, ...} -> IsError, Text memuat "ditentukan oleh API key"
func TestT18_AC3_DashboardAggregates(t *testing.T)    // setelah panggilan di atas: GET /api/audit-log/summary?range=24h -> tool_usage memuat tool "get_file_outline", "read_section", "list_recent_changes"
func TestT18_Int_AuditHasNoContent(t *testing.T)      // isi file berisi "SECRET-OUTLINE-42"; SELECT result::text FROM audit_log WHERE action LIKE 'mcp.%' tidak memuatnya
```
Rute update file REST mengikuti tes T05 yang ada (`knowledge_integration_test.go`); jangan menebak, salin pemanggilannya.
- [ ] **Step 2:** Run `make dev-test-db` lalu `cd backend && TEST_DATABASE_URL=postgres://grimoire:grimoire@localhost:5433/grimoire_test?sslmode=disable go test -race -tags integration ./cmd/internal/server/ -run 'TestT18' -v` -> semua PASS (bila ada yang gagal, perbaiki di task pemilik kode, bukan di tes).
- [ ] **Step 3: Commit** `test(discovery-mcp-outline-tools): integrasi tiga tool lewat /mcp`

---

### Task 11: Dokumentasi, katalog, dan tiket T18

**Files:**
- Create: `docs/tickets/T18-discovery-mcp-outline-tools.md` (isi spec + bagian "Keputusan K6 (final)" menyalin tabel K6 di atas)
- Modify: `docs/tickets/README.md`
- Modify: `docs/mcp-gateway.md`

- [ ] **Step 1: `docs/tickets/README.md`**
  - Daftar tiket: baris `| [T18](T18-discovery-mcp-outline-tools.md) | discovery | knowledge-tools (outline, section, perubahan terbaru) | M | T07, T08, T09 | Tiket baru |` (status diubah ke `Selesai (\`<sha>\`)` saat merge).
  - Tree: di bawah T08 tambah `│       │       ├── T18 discovery outline/section/recent-changes   + butuh T09`.
  - Gelombang: tambah baris `| 10 | T18 |`.
  - Katalog bus: baris `project.knowledge.list-changes` (Call, baca-saja; Provider T18; Consumer T18 knowledgetools `listrecentchanges`); kolom Consumer `project.knowledge.read-file` jadi `T08, T18`, `project.apispec.list-specs` jadi `T09, T18`.
  - Paragraf kontrak baru: "Kontrak T18: `lib/contracts/project/knowledge/listchanges` (`BUS_PROJECT_KNOWLEDGE_LIST_CHANGES`; urut `updated_at DESC, path`, `Since` wajib, `Limit` maks 50, indeks `knowledge_files_project_updated_idx`). Perubahan aditif: `read-file` menerima `Request.Whole` (isi utuh, Offset 0) dan mengembalikan `Response.UpdatedAt`; `list-specs` mengembalikan `Spec.CreatedAt` dan `Spec.SizeBytes`. Parser heading bersama di `lib/util/mdoutline`."
  - Katalog aksi, baris Agent: sisipkan `mcp.get_file_outline`, `mcp.read_section`, `mcp.list_recent_changes` setelah `mcp.get_api_endpoint`.
- [ ] **Step 2: `docs/mcp-gateway.md`** bagian "Tool knowledge": "tiga tool" -> "enam tool"; tambah tiga baris tabel (argumen dan rentang persis dari Global Constraints, penanda terpotong dari Task 6-8); tambah tiga contoh `tools/call`:
```json
{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"get_file_outline","arguments":{"path":"docs/guides/a.md"}}}
{"jsonrpc":"2.0","id":8,"method":"tools/call","params":{"name":"read_section","arguments":{"path":"docs/guides/a.md","heading":"Setup","occurrence":2}}}
{"jsonrpc":"2.0","id":9,"method":"tools/call","params":{"name":"list_recent_changes","arguments":{"since":"7d","kind":"files","path":"docs","limit":20}}}
```
  Catat bahwa `list_recent_changes` tidak melaporkan file/spec yang dihapus (hard delete, rules 8.2) dan tidak mengembalikan identitas pengubah.
- [ ] **Step 3: Verifikasi penuh**
  - `make lint` -> exit 0 (go vet, golangci-lint, svelte-check, depcruise).
  - `make test` -> PASS (backend `go test -race ./...`, frontend `npm test -- --run`).
  - `make test-integration` -> PASS.
  - `cd backend && go test ./lib/contracts/core/audit/record/ -run TestActions_MatchesCatalog` -> PASS (literal `catalog` = katalog README).
- [ ] **Step 4: Commit** `docs(discovery-mcp-outline-tools): katalog bus/aksi, mcp-gateway, dan tiket T18`
