# T11 usecase `searchquality`: Plan

Tiket: `docs/tickets/T11-discovery-search-quality.md`. Dokumen induk (FOUNDATION, mengikat): `docs/superpowers/plans/discovery-search-quality/overview.md`. Bila bertentangan, overview menang.

Usecase ini runLate: tes integrasi butuh provider `core.audit.scan-entries` (usecase `scanentries`, U2) dan wiring W1 (`knowledgetools/base.go` memanggil `searchquality.Init(d)`) sudah ada. Unit test tidak bergantung pada keduanya.

Prasyarat dari FOUNDATION (sudah ada, TIDAK diubah di sini; `base/` dilarang diubah usecase):
- `lib/constants/bus.go`: `BUS_CORE_AUDIT_SCAN_ENTRIES`, `BUS_DISCOVERY_KNOWLEDGETOOLS_SEARCH_QUALITY_REPORT`.
- Kontrak `lib/contracts/core/audit/scanentries` dan `lib/contracts/discovery/knowledgetools/searchquality`.
- `knowledgetools/base/quality.go`: `AuditEvent`, `EventFromAudit`, `NormalizeQuery`, `QualityAccumulator` (`NewQualityAccumulator`, `Add`, `Report`), konstanta `DefaultQualityWindow`, `MinQualityWindow`, `MaxQualityWindow`, `MaxQualityScanEntries`.

Jika salah satu simbol di atas belum ada saat dikerjakan, berhenti dan laporkan; jangan membuatnya di feature ini.

---

## 1. Metadata

| Atribut | Nilai |
|---|---|
| Ticket | T11 (AC7, AC8, AC9, AC10) |
| Scope | backend |
| Domain | discovery |
| Module | knowledge-tools (package `knowledgetools`) |
| Feature | `searchquality` (baru) |
| Resource root | `backend/internal/domain/discovery/internal/knowledgetools` |
| runLate | ya |
| Migrasi | tidak ada |
| `endpointsChanged` | false (tidak ada REST/OpenAPI; yang berubah hanya katalog bus dan CLI) |

| Endpoint | Method | Version | Peran |
|---|---|---|---|
| `discovery.knowledgetools.search-quality-report` | bus Call (baru, baca-saja) | v1 (kontrak `searchquality`) | feature ini PROVIDER; consumer `cmd/cli` |
| `core.audit.scan-entries` | bus Call (baru, milik core/auditlog) | v1 (kontrak `scanentries`) | feature ini CONSUMER (lewat `repository.go`) |
| `grimoire search-quality report` | CLI (baru) | - | subperintah `backend/cmd/cli` |

allowedPaths (hanya ini yang boleh disentuh):
- `backend/internal/domain/discovery/internal/knowledgetools/searchquality/`
- `backend/cmd/cli/searchquality.go`
- `backend/cmd/cli/searchquality_test.go`
- `backend/cmd/cli/cli.go`
- `backend/cmd/internal/server/searchquality_report_integration_test.go`

---

## 2. Summary Plan

Feature ini menghasilkan laporan kualitas pencarian (zero-result, follow-up, reformulasi, rank dibuka) dari data audit satu project dan rentang waktu, lalu menampilkannya lewat CLI.

Pembagian tanggung jawab (rules 2.4, 4.7):
- `base/quality.go` (sudah ada dari FOUNDATION) memegang SEMUA rule bisnis: parsing event, aturan follow-up/rank/reformulasi, eviction. Feature ini tidak menyalin rule apa pun.
- `params.go` memvalidasi dan menormalisasi request (rule input, bukan rule metrik).
- `usecase.go` hanya orkestrasi: loop halaman scan, ubah entri jadi `AuditEvent`, umpan ke akumulator, ambil `Report`. Satu usecase, tidak dipecah.
- `repository.go` BUKAN SQL. Ia adapter bus ke `core.audit.scan-entries` (module lain dilarang membaca tabel `audit_log`). Dibuat sebagai interface agar usecase bisa dites dengan repo palsu.
- `handler.go` adaptor bus: `bus.Expect`, parse, panggil usecase, kembalikan `searchquality.Response` (nilai, bukan pointer).
- `cmd/cli/searchquality.go` hanya parsing flag, perakitan lewat `server.WithDomains`, `bus.CallTyped`, dan pencetakan. Tidak ada SQL dan tidak ada rule metrik.

Keputusan lokal (tidak bertentangan dengan overview):

| ID | Keputusan | Alasan |
|---|---|---|
| L1 | Rentang scan: `From = Since`, `To = Until + Window`, `Limit = 1000` (`scanentries.MaxLimit`), `Actions = [record.ActionMCPSearchKnowledge, record.ActionMCPReadFile]` | Read yang jatuh di `[Until, Until+Window)` tetap dibutuhkan untuk follow-up search terakhir; akumulator mengabaikan yang di luar `[since, until+window)` |
| L2 | Usecase mengirim `Since`/`Until` dalam UTC | Tampilan konsisten |
| L3 | Batas aman dihitung pada jumlah entri yang DIBACA (bukan jumlah search). Dicek setelah tiap halaman: `total > maxEntries` -> `errs.Wrap(errs.CodeInvalid, "rentang terlalu besar (lebih dari 2000000 entri); persempit --since/--until", errs.ErrInvalid)`. `maxEntries` field usecase, default `base.MaxQualityScanEntries`, bisa diganti tes | Hentikan paging lebih awal, memori akumulator tetap terbatas (D13) |
| L4 | Penjaga loop: bila `Next` sama persis dengan cursor yang baru dipakai, atau halaman kosong tetapi `Next != nil`, kembalikan `fmt.Errorf("searchquality: scan audit: kursor tidak maju")` | Provider yang rusak tidak boleh membuat CLI berputar selamanya |
| L5 | `ctx.Err()` dicek sebelum tiap halaman | CLI punya timeout 10 menit; batalkan bersih |
| L6 | Error dari repository dibungkus `fmt.Errorf("searchquality: scan audit: %w", err)`. `%w` mempertahankan `errors.As(*errs.Error)` sehingga `INVALID_ARGUMENT` dari provider scan tetap dikenali CLI | Sesuai overview 12.4 |
| L7 | Window dari request ditambahkan ke `Until` memakai `time.Time.Add`; tidak ada clamp ke "sekarang" | Rentang masa depan sekadar kosong |
| L8 | AC10 (CLI end-to-end) dijalankan dengan membangun binary `./cmd/cli` dan menjalankannya sebagai proses, BUKAN memanggil `cli.run` langsung | `cmd/cli` adalah `package main`; tidak bisa di-import dari `cmd/internal/server`. Jalur kode yang sama (`cli.run` -> `server.WithDomains` -> bus) tetap dilalui. Pola `buildBinary`/`runBin` ada di `cmd/cli/smoke_integration_test.go` (tidak boleh diubah), jadi helper dibuat ulang ber-prefiks `sqr` di file tes server |

---

## 3. Plan (langkah berurutan)

Kerjakan dengan TDD: tulis tes lalu implementasi per langkah. Semua kode berbahasa Indonesia untuk komentar dan pesan, mengikuti gaya repo.

### Langkah 1. `searchquality/params.go`

```go
package searchquality

// Params adalah request yang sudah tervalidasi dan dinormalisasi.
type Params struct {
	ProjectID string        // uuid kanonik huruf kecil
	Since     time.Time     // UTC
	Until     time.Time     // UTC, > Since
	Window    time.Duration // MinQualityWindow..MaxQualityWindow
}

// ParseParams memvalidasi request. now dipakai bila Until nol. Semua error adalah
// errs.Wrap(errs.CodeInvalid, <pesan>, errs.ErrInvalid); pesan TIDAK memuat nilai input.
func ParseParams(in searchquality.Request, now time.Time) (Params, error)
```

Aturan dan pesan (urutan pemeriksaan tetap):
1. `ProjectID`: `uuid.Parse` (`github.com/google/uuid`, sudah di go.mod) harus sukses -> simpan `u.String()`; gagal -> `"project_id tidak valid"`.
2. `Since.IsZero()` -> `"since wajib diisi"`.
3. `Until.IsZero()` -> `now`. Bila `!Until.After(Since)` -> `"until harus setelah since"`.
4. `Window == 0` -> `base.DefaultQualityWindow`. Bila `< base.MinQualityWindow` atau `> base.MaxQualityWindow` (negatif ikut) -> `"window harus 1s..1h"`. Batas inklusif kedua sisi.

### Langkah 2. `searchquality/repository.go`

```go
// Scanner adalah port ke provider core.audit.scan-entries. Tidak ada SQL di feature ini.
type Scanner interface {
	Scan(ctx context.Context, req scanentries.Request) (scanentries.Response, error)
}

type busScanner struct{ b *bus.Registry }

func NewRepository(b *bus.Registry) Scanner { return &busScanner{b: b} }

func (s *busScanner) Scan(ctx context.Context, req scanentries.Request) (scanentries.Response, error) {
	return bus.CallTyped[scanentries.Response](ctx, s.b, constants.BUS_CORE_AUDIT_SCAN_ENTRIES, req)
}
```

Error dikembalikan apa adanya (pembungkusan di usecase). `d.Bus` bertipe `*bus.Registry`.

### Langkah 3. `searchquality/usecase.go`

```go
type Usecase struct {
	repo       Scanner
	now        func() time.Time
	maxEntries int
}

func NewUsecase(repo Scanner) *Usecase // now=time.Now, maxEntries=base.MaxQualityScanEntries

func (u *Usecase) Report(ctx context.Context, in searchquality.Request) (searchquality.Response, error)
```

Alur `Report` (orkestrasi saja):
1. `p, err := ParseParams(in, u.now())`; error dikembalikan tanpa pembungkus.
2. `acc := base.NewQualityAccumulator(p.Since, p.Until, p.Window)`.
3. `var after *scanentries.Cursor; total := 0`; loop:
   - `ctx.Err()` -> kembalikan error (L5).
   - `resp, err := u.repo.Scan(ctx, scanentries.Request{ProjectID: p.ProjectID, Actions: []string{record.ActionMCPSearchKnowledge, record.ActionMCPReadFile}, From: p.Since, To: p.Until.Add(p.Window), After: after, Limit: scanentries.MaxLimit})`; error -> `fmt.Errorf("searchquality: scan audit: %w", err)` (L6).
   - `total += len(resp.Entries)`; bila `total > u.maxEntries` -> error rentang terlalu besar (L3).
   - tiap entri berurutan: `acc.Add(base.EventFromAudit(e.ID, e.Action, e.ActorID, e.Target, e.Status, e.OccurredAt, e.Args, e.Result))`. Urutan dari provider `(occurred_at, id)` ASC dipertahankan; usecase TIDAK mengurutkan ulang.
   - `resp.Next == nil` -> keluar loop. Selain itu terapkan penjaga L4, lalu `after = resp.Next`.
4. `return searchquality.Response{Report: acc.Report(p.ProjectID)}, nil`.

Catatan: `Report.Since/Until/WindowSeconds` diisi oleh akumulator dari parameter yang dinormalisasi; usecase tidak mengisi ulang.

### Langkah 4. `searchquality/handler.go`

```go
type reporter interface {
	Report(ctx context.Context, in searchquality.Request) (searchquality.Response, error)
}
type handler struct{ uc reporter }

func (h *handler) handle(ctx context.Context, req any) (any, error) {
	in, err := bus.Expect[searchquality.Request](req) // tipe lain/pointer -> membungkus bus.ErrBadRequestType
	if err != nil {
		return nil, err
	}
	return h.uc.Report(ctx, in)
}
```

Map error: tidak ada konversi tambahan. `*errs.Error` (invalid) lewat apa adanya; error infra sudah dibungkus `searchquality: scan audit: ...`. Response dikembalikan sebagai nilai `searchquality.Response` agar `bus.CallTyped[searchquality.Response]` berhasil.

### Langkah 5. `searchquality/init.go`

```go
// Init mendaftarkan handler bus discovery.knowledgetools.search-quality-report.
// Dipanggil dari knowledgetools/base.go pada tahap wiring (W1), bukan dari feature lain.
func Init(d app.Deps) {
	uc := NewUsecase(NewRepository(d.Bus))
	d.Bus.Register(constants.BUS_DISCOVERY_KNOWLEDGETOOLS_SEARCH_QUALITY_REPORT, (&handler{uc: uc}).handle)
}
```

Registrasi ganda = panic; hanya satu panggilan `Init` (W1). Jangan menyentuh `knowledgetools/base.go`.

### Langkah 6. `backend/cmd/cli/searchquality.go`

Isi:

```go
type searchQualityArgs struct {
	ProjectID string
	Since     time.Time
	Until     time.Time // nol = default sisi server (now)
	Window    time.Duration
	JSON      bool
}

const searchQualityTimeout = 10 * time.Minute

func parseSince(s string, now time.Time) (time.Time, error)
func (c *cli) searchQuality(ctx context.Context, args []string) int
func (c *cli) callSearchQuality(ctx context.Context, a searchQualityArgs) (searchquality.Report, error)
func (c *cli) reportSearchQualityError(err error)
func writeQualityText(w io.Writer, r searchquality.Report)
func writeQualityJSON(w io.Writer, r searchquality.Report) error
```

`parseSince(s, now)` (dipakai untuk `--since` dan `--until`):
- `""` -> error `"kosong"`.
- Format relatif `^\d+d$` -> `now.Add(-N*24h)`; N harus > 0 dan <= 3650.
- `time.ParseDuration` sukses (mis. `24h`, `90m`) dan durasi > 0 -> `now.Add(-d)`.
- `time.Parse(time.RFC3339, s)` -> UTC.
- `time.Parse("2006-01-02", s)` -> tengah malam UTC.
- selain itu error. Pesan error CLI: `--since tidak terbaca (pakai 7d, 24h, RFC3339, atau YYYY-MM-DD)` tanpa menyertakan nilai.

`c.searchQuality(ctx, args)`:
1. `len(args) == 0 || args[0] != "report"` -> `c.badUsage("pemakaian: grimoire search-quality report --project ID --since 7d [--until ...] [--window 120s] [--json]")` (exit 2).
2. `flag.NewFlagSet("search-quality report", flag.ContinueOnError)`, `SetOutput(io.Discard)`; flag: `--project`, `--since`, `--until`, `--window` (string, default `""`; dibaca `time.ParseDuration`), `--json` (bool). `flag.ErrHelp` -> cetak usage ke stdout, exit 0. Error parse lain -> `badUsage(err.Error())`.
3. `fs.NArg() > 0` -> `badUsage("argumen tidak dikenal: %q")`.
4. `--project` kosong -> `badUsage("--project wajib diisi")`. (Validitas uuid sengaja diserahkan ke bus -> exit 1 dengan pesan `project_id tidak valid`, sesuai overview 12.4.)
5. `--since` kosong -> `badUsage("--since wajib diisi")`; tak terbaca -> `badUsage("--since tidak terbaca (...)")`.
6. `--until` bila diisi: tak terbaca -> `badUsage`; hasil `!After(since)` -> `badUsage("--until harus setelah --since")`.
7. `--window` bila diisi: tak terbaca -> `badUsage`; di luar `[time.Second, time.Hour]` -> `badUsage("--window harus 1s..1h")`. Konstanta CLI diambil dari `searchquality` kontrak? Kontrak tidak mengekspor batas, dan `base` tidak bisa di-import dari `cmd`. Karena itu CLI memakai konstanta lokal `minWindow = time.Second`, `maxWindow = time.Hour` (duplikasi nilai disengaja dan murah; bus tetap menjadi sumber kebenaran, tes integrasi menjaga keduanya sejalan).
8. `now := time.Now()` diambil SEKALI di `searchQuality` (lewat field `c.now func() time.Time` agar tes deterministik; tambahkan ke struct `cli` di `cli.go`, default `time.Now` di `newCLI`).
9. Panggil `c.searchQualityRun(ctx, a)` (field seam). Error -> `reportSearchQualityError`, exit 1. Sukses -> cetak, exit 0.

`callSearchQuality` (pola `callCreateAdmin`): `server.LoadConfig(c.getenv, c.stderr)` (`!ok` -> `errAlreadyReported`), `logger.NewWithWriter(cfg, c.stderr)` (stdout bersih), `context.WithTimeout(ctx, searchQualityTimeout)`, lalu

```go
err := server.WithDomains(ctx, cfg, log, func(d app.Deps) error {
	res, err := bus.CallTyped[searchquality.Response](ctx, d.Bus,
		constants.BUS_DISCOVERY_KNOWLEDGETOOLS_SEARCH_QUALITY_REPORT,
		searchquality.Request{ProjectID: a.ProjectID, Since: a.Since, Until: a.Until, Window: a.Window})
	if err != nil { return err }
	rep = res.Report
	return nil
})
if err != nil && !isReportable(err) { log.Error("search-quality gagal", "error", err) }
```

`reportSearchQualityError`: `errAlreadyReported` -> diam; `*errs.Error` -> `search-quality: <Message>`; `pgErr 42P01` -> `tabel belum ada; jalankan "grimoire migrate up" lebih dulu`; lainnya -> `search-quality report gagal; periksa log`. Tidak pernah mencetak `err.Error()` mentah.

Keluaran teks (`writeQualityText`), persis format overview 12.3:

```
Kualitas pencarian project <project_id>
Rentang  <since RFC3339 UTC> .. <until RFC3339 UTC> (jendela <N>s)
Pencarian                 <searches>
Zero-result               <c>/<d>   <rate*100 %.1f>%
Follow-up                 <c>/<d>   <rate>%
Reformulasi               <c>/<d>   <rate>%
Rank dibuka               1:<n>  2:<n> ... 10:<n>  tidak di hasil:<n>
Entri lama dilewati       <legacy> (tanpa result_paths; tidak ikut follow-up/rank)
```

- Label kolom kiri dipadatkan ke lebar 26 (`%-26s`). Denominator 0 mencetak `0/0   0.0%`.
- Bucket dicetak sesuai urutan `Report.RankOpened` (sudah tetap 11 bucket); label `not_in_results` dipetakan ke teks `tidak di hasil`. Bucket tak dikenal dicetak dengan labelnya apa adanya (tahan perubahan kontrak).
- Waktu dicetak `t.UTC().Format(time.RFC3339)`.

Keluaran JSON (`--json`): `json.NewEncoder(c.stdout)` dengan `SetIndent("", "  ")` mengenkode `searchquality.Response{Report: rep}` (bentuk persis 12.3, satu objek, newline di akhir). Tidak ada teks lain di stdout.

### Langkah 7. `backend/cmd/cli/cli.go` (ubah)

- Impor tambahan: tidak ada yang baru selain yang sudah dipakai file lain (import baru khusus ada di `searchquality.go`).
- Tambah field di `cli`: `now func() time.Time` dan `searchQualityRun func(ctx context.Context, a searchQualityArgs) (searchquality.Report, error)`. Karena `cli.go` tidak boleh mengimpor lebih dari yang perlu, tipe `searchquality.Report` diimpor di `cli.go` sebagai `"grimoire/lib/contracts/discovery/knowledgetools/searchquality"`.
- `newCLI`: `c.now = time.Now`; `c.searchQualityRun = c.callSearchQuality`.
- `run`: tambah `case "search-quality": return c.searchQuality(ctx, args[1:])`.
- `usage`: tambah baris pada daftar perintah:

```
  search-quality report --project ID --since 7d
                                           laporan kualitas pencarian dari audit
                                           (opsi: --until, --window 120s, --json)
```

- Jangan mengubah perilaku perintah lain. Tes `main_test.go` yang ada (mis. yang membandingkan usage) harus tetap hijau; periksa dulu dan, bila ada yang menguji isi `usage` persis, laporkan alih-alih mengedit `main_test.go` (di luar allowedPaths).

### Langkah 8. Tes (lihat bagian 6) lalu verifikasi

```
cd backend
go build ./... && go vet ./...
go test ./internal/domain/discovery/... ./cmd/cli/... ./internal/archtest/...
go test -tags integration -run 'SearchQuality|SQR' ./cmd/internal/server/...   # setelah U2 dan W1 ada
```

---

## 4. Migration

Tidak ada. Tidak ada tabel baru, tidak ada perubahan skema/index. Feature ini tidak menyentuh SQL; pembacaan `audit_log` dilakukan provider `scanentries` memakai index `audit_log_project_idx`. `.claude/rules/migrations.md` tidak relevan.

---

## 5. Changes

| File | Perubahan |
|---|---|
| `backend/internal/domain/discovery/internal/knowledgetools/searchquality/init.go` | baru: `Init(d)` daftar handler bus |
| `.../searchquality/handler.go` | baru: `handler.handle` (`bus.Expect`, panggil usecase) |
| `.../searchquality/params.go` | baru: `Params`, `ParseParams` |
| `.../searchquality/usecase.go` | baru: `Usecase`, `NewUsecase`, `Report` (loop paging, batas aman, penjaga kursor) |
| `.../searchquality/repository.go` | baru: interface `Scanner`, adapter bus `NewRepository` |
| `.../searchquality/params_test.go`, `usecase_test.go`, `handler_test.go` | baru: unit test |
| `backend/cmd/cli/searchquality.go` | baru: subperintah, `parseSince`, perakitan bus, pencetak teks/JSON |
| `backend/cmd/cli/searchquality_test.go` | baru: unit test CLI |
| `backend/cmd/cli/cli.go` | ubah: `case "search-quality"`, baris usage, field `now` dan `searchQualityRun`, inisialisasi di `newCLI` |
| `backend/cmd/internal/server/searchquality_report_integration_test.go` | baru: `//go:build integration`, AC7-AC10 |

Tidak diubah (di luar allowedPaths): `knowledgetools/base.go` (W1), `knowledgetools/base/**` (FOUNDATION), `main_test.go`, `smoke_integration_test.go`, `mcpgateway_integration_test.go`, `knowledgetools_integration_test.go`, `go.mod`, katalog bus.

Kepatuhan arsitektur: paket `searchquality` hanya mengimpor `knowledgetools/base`, `infra/app`, `infra/bus`, `lib/common/errs`, `lib/constants`, `lib/contracts/{core/audit/record,core/audit/scanentries,discovery/knowledgetools/searchquality}`, dan `github.com/google/uuid`. Tidak mengimpor `auditlog`, tidak ada `database/sql`, tidak ada string SQL.

---

## 6. Test Scenario

### 6.1 Unit `params_test.go` (tabel)

- ProjectID kosong, `"x"`, uuid terpotong -> `errors.Is(err, errs.ErrInvalid)` dan pesan `project_id tidak valid`; pesan tidak memuat nilai input (assert `!strings.Contains(msg, input)`).
- uuid huruf besar -> diterima, `Params.ProjectID` huruf kecil.
- `Since` nol -> `since wajib diisi`.
- `Until` nol -> `Params.Until == now`. `Until == Since` dan `Until < Since` -> `until harus setelah since`. `Until` nol tetapi `now <= Since` -> `until harus setelah since`.
- `Window` 0 -> 120s. 1s, 1h -> diterima. 999ms, 1h+1ns, -1s -> `window harus 1s..1h`.
- Semua error bertipe `*errs.Error` dengan `Code == errs.CodeInvalid`.

### 6.2 Unit `usecase_test.go` (repo palsu `fakeScanner`)

`fakeScanner` mencatat setiap `scanentries.Request` dan mengembalikan halaman terskrip.
- Paging: 3 halaman (1000, 1000, 5 entri) -> 3 panggilan; request ke-1 `After == nil`, ke-2 `After == Next halaman 1`, ke-3 `After == Next halaman 2`; berhenti saat `Next == nil`. Semua request: `Actions == [mcp.search_knowledge, mcp.read_file]`, `From == Since`, `To == Until + Window`, `Limit == 1000`, `ProjectID` kanonik.
- Hasil identik dengan umpan langsung ke `base.QualityAccumulator` pada rangkaian event yang sama (search + read hasil ke-2 dalam 30 dtk, query berbeda 60 dtk) -> `FollowUp {1,..}`, `rank_opened["2"] == 1`, `Reformulation.Count == 1`. Ini menguji bahwa `EventFromAudit` dipanggil dengan argumen yang benar (field `Target`, `Args["query"]`, `Result["result_paths"]`, `ActorID`).
- Tanpa data: `Entries == []`, `Next == nil` -> 1 panggilan; `Searches == 0`, 11 bucket rank, semua rate 0.
- Window default: request `Window == 0` -> `To == Until + 120s` dan `Report.WindowSeconds == 120`.
- Error repo: repo mengembalikan `sentinel` -> `errors.Is(err, sentinel)` benar dan `err.Error()` diawali `searchquality: scan audit:`. Repo mengembalikan `*errs.Error` (INVALID_ARGUMENT) -> `errors.As` tetap menemukannya.
- Batas aman: `maxEntries` diset 2500, 3 halaman @1000 -> error `errors.Is(err, errs.ErrInvalid)`, pesan memuat `rentang terlalu besar`, dan scan berhenti pada halaman ke-3 (tidak memanggil halaman ke-4). Tepat sama dengan batas (2500 entri total) -> sukses.
- Penjaga kursor: `Next` sama dengan `After` yang baru dipakai -> error `kursor tidak maju`; halaman kosong dengan `Next != nil` -> error yang sama.
- Konteks dibatalkan sebelum scan -> `errors.Is(err, context.Canceled)`, repo tidak dipanggil.
- Params invalid -> repo tidak dipanggil sama sekali.
- Entri `status=error`, aksi lain, atau `ActorID == ""` yang lolos dari repo palsu tetap diabaikan akumulator (cek tidak mengubah `Searches`).

### 6.3 Unit `handler_test.go` (pola `auth/createadmin/handler_test.go`)

- `Register` handler pada `bus.NewRegistry`, `bus.CallTyped[searchquality.Response]` dengan request valid dan usecase palsu -> Response diteruskan.
- `b.Call(ctx, endpoint, "string-salah")` dan pointer `&searchquality.Request{}` -> `errors.Is(err, bus.ErrBadRequestType)`.
- Request invalid -> error bertipe `*errs.Error` kode `INVALID_ARGUMENT` lewat bus.

### 6.4 Unit CLI `searchquality_test.go` (harness `newTestCLI` dari `main_test.go`, `searchQualityRun` diganti palsu)

- `parseSince` dengan `now = 2026-10-21T10:00:00Z`: `7d` -> `2026-10-14T10:00:00Z`; `24h` -> `2026-10-20T10:00:00Z`; `90m`; `2026-10-01T08:00:00+07:00` -> `2026-10-01T01:00:00Z`; `2026-10-01` -> `2026-10-01T00:00:00Z`. Gagal: `""`, `abc`, `0d`, `-1d`, `-5h`, `2026-13-40`, `3651d`.
- `search-quality report --project P --since 7d`: seam menerima `Since == now-7d` (dengan `c.now` tetap), `Until` nol, `Window` nol, `JSON false`, exit 0, stdout memuat `Zero-result`, `Follow-up`, `Reformulasi`, `Rank dibuka`, `Entri lama dilewati`.
- `--json`: stdout lolos `json.Valid`, bisa di-`Unmarshal` ke `searchquality.Response`, tidak mengandung teks `Zero-result`; stderr kosong dari teks laporan.
- Golden teks: dengan `Report` dari overview 12.3, stdout sama persis dengan blok teks 12.3 (termasuk `61/100   61.0%` dan `tidak di hasil:9`).
- Denominator 0 mencetak `0/0   0.0%`; semua bucket 0 tetap tercetak.
- Flag wajib: tanpa `--project` -> exit 2, stderr memuat `--project wajib diisi` dan `Pemakaian:`, stdout kosong, seam TIDAK dipanggil. Tanpa `--since` -> idem. `--since abc` -> exit 2. `--until` sebelum `--since` -> exit 2. `--window 2h`, `--window 500ms`, `--window abc` -> exit 2. Argumen sisa `foo` -> exit 2. `search-quality` tanpa subperintah atau `search-quality foo` -> exit 2. `--window 1s` dan `--window 1h` -> diterima.
- Flag valid diteruskan: `--until 2026-10-21 --window 5m` -> seam menerima `Until == 2026-10-21T00:00:00Z`, `Window == 5m`.
- Error seam: `*errs.Error` `INVALID_ARGUMENT` "project_id tidak valid" -> exit 1, stderr `search-quality: project_id tidak valid`, stdout kosong. `errors.New("secret internal")` -> exit 1, stderr `search-quality report gagal; periksa log` dan tidak memuat `secret internal`. `errAlreadyReported` -> exit 1, tidak ada pesan tambahan.
- `help` / `usage` memuat `search-quality report`.

### 6.5 Integrasi `searchquality_report_integration_test.go` (`//go:build integration`)

Helper (prefiks `sqr`, tidak bentrok dengan `setupMCP`/`se*`/`sqa*`):
- `sqrSetup(t)`: `dbURL, schema, admin := testDBURL(t)`; `cfg := testCfg(t, dbURL, {"RUN_MIGRATION":"true"})`; `startRun(t, cfg, WithAppHook(...))` untuk mendapat `app.Deps`; kembalikan `{t, dbURL, schema, admin, deps}`. Tidak butuh user atau project sungguhan karena `audit_log.project_id` tanpa FK.
- `sqrInsert(row)`: `INSERT INTO <schema>.audit_log (occurred_at, actor_type, actor_id, actor_label, project_id, action, target, status, args, result) VALUES (...)` dengan `args`/`result` dari `json.Marshal`. `actor_type='api_key'`, `actor_label='k'`. `status` default `ok`. Hanya INSERT (trigger melarang UPDATE; tidak ada UPDATE/DELETE di tes).
- `sqrReport(req)`: `bus.CallTyped[searchquality.Response](ctx, deps.Bus, constants.BUS_DISCOVERY_KNOWLEDGETOOLS_SEARCH_QUALITY_REPORT, req)`.
- Tiap subtes memakai `project_id` uuid baru (`uuid.NewString()`) sehingga hermetik. Waktu dasar `t0 := time.Now().UTC().Add(-2*time.Hour).Truncate(time.Second)`.
- Pembuat baris: `sqrSearch(project, actor, at, query, paths []string)` -> `args {"query": q}`, `result {"count": n, "total": n, "result_paths": paths}`; `sqrRead(project, actor, at, path)` -> `action mcp.read_file`, `target path`.

Subtes:
- `TestT11_AC7_FollowUpRank`: search K1 pada `t0` dengan paths `[a.md,b.md,c.md]`; `read b.md` K1 pada `t0+30s` -> `FollowUp {Count 1, Denominator 1, Rate 1}`, bucket `"2" == 1`, bucket lain 0. Kasus negatif, masing-masing project sendiri dengan satu search + satu read yang tidak boleh dihitung: (a) read oleh aktor K2 -> `FollowUp.Count 0`, `Denominator 1`, semua bucket 0; (b) read pada project lain (project berbeda, aktor sama) -> 0; (c) read `t0+3min` -> 0 dan bucket `not_in_results` 0 (di luar jendela, tidak dianggap read dalam jendela). Plus kasus tepi: read `t0+120s` -> dihitung.
- `TestT11_AC8_Reformulation`: K1 search "login" `t0`, search "oauth login" `t0+60s` -> `Searches 2`, `Reformulation {Count 1, Denominator 2}`. Project lain: "login" lalu "login" lagi `t0+60s` -> `Reformulation.Count 0`. Tambahan: "Login  " vs "login" (beda kapital/spasi) -> 0; search berikutnya `t0+130s` -> 0.
- `TestT11_AC9_LegacyEntries`: baris `mcp.search_knowledge` dengan `result {"count":0,"total":0}` (tanpa `result_paths`) dan satu lagi `{"count":2,"total":2}`, ditambah `read_file` 20 dtk setelah search kedua -> tidak error; `ZeroResult {1, 2}`, `LegacySearches 2`, `FollowUp.Denominator 0`, `FollowUp.Count 0`, 11 bucket semuanya 0.
- `TestT11_PagingAcrossPages`: sisipkan 2.300 baris `mcp.read_file` (aktor sama, `occurred_at` identik pada sebagian) + 3 search dengan hasil yang diharapkan; laporan benar dan tidak ada entri terlewat atau ganda (membuktikan loop `Limit=1000` melalui `Next`). Gunakan satu `INSERT ... SELECT generate_series` agar cepat.
- `TestT11_RangeBoundaries`: search tepat pada `Until` tidak dihitung (`Searches` tidak bertambah) tetapi read pada `Until+30s` menjadi follow-up untuk search pada `Until-10s`; search pada `Since-1s` tidak ikut; search pada `Since` ikut.
- `TestT11_OnlyOkAndOnlyProject`: baris `status=error`/`denied`, `actor_id` NULL, dan baris project lain tidak mengubah laporan.
- `TestT11_ErrorsViaBus`: `ProjectID "bukan-uuid"` -> `errors.Is(err, errs.ErrInvalid)`; `Since` nol; `Window 2h`; `Until <= Since` -> `ErrInvalid`. Tipe body pointer -> `bus.ErrBadRequestType` (lewat `deps.Bus.Call`).
- `TestT11_UnknownProject`: project tanpa data -> semua nol, rate 0, 11 bucket berurutan `1..10,not_in_results`.
- `TestT11_AC10_CLI`: `sqrBuildCLI(t)` (sync.Once, `go build -o <tmp> ./cmd/cli` dengan `cmd.Dir` = `filepath.Join(dir, "..", "..", "..")` dari `runtime.Caller(0)`; hapus binary di `t.Cleanup`). Seed data seperti AC7 + AC8 dengan waktu dalam 7 hari. Jalankan binary dengan env `DATABASE_URL=<dbURL>`, `SESSION_SECRET=<testSecret>`, `HOME`/`PATH` minimal, `cmd.Dir = t.TempDir()`:
  - `search-quality report --project P --since 7d` -> exit 0; stdout memuat `Zero-result`, `Follow-up`, `Reformulasi`, `Rank dibuka`, `Entri lama dilewati`, dan `Follow-up` menampilkan `1/1`.
  - `... --json` -> exit 0; `json.Valid(stdout)`; `Unmarshal` ke `searchquality.Response`: `report.project_id == P`, `follow_up.count == 1`, `len(rank_opened) == 11`, `rank_opened[1].count == 1`; stdout tidak mengandung baris log (stderr terpisah).
  - Negatif: tanpa `--project` -> exit 2, stdout kosong, stderr `Pemakaian:`; `--project bukan-uuid --since 7d` -> exit 1, stderr `search-quality: project_id tidak valid`; `--since 7d --window 2h` -> exit 2.
  - Catatan L8: pengujian ini memakai proses biner, bukan pemanggilan `cli.run` langsung, karena `package main` tidak bisa di-import.
- Semua tes seed dilindungi `testDBURL` (skip bila `TEST_DATABASE_URL` kosong, gagal di CI).

### 6.6 Arsitektur

- `internal/archtest` hijau (import dibatasi seperti bagian Changes).
- Tidak ada string `audit_log` di luar komentar pada paket `searchquality`: `grep -rn "audit_log" backend/internal/domain/discovery/internal/knowledgetools/searchquality/` hanya boleh mengenai komentar atau nama tes.

---

## 7. E2E Scenarios

Positif:
1. Agent mencari lalu `read_file` hasil ke-2 dalam 30 detik -> `grimoire search-quality report --project <id> --since 7d` mencetak `Follow-up 1/1 100.0%` dan `Rank dibuka ... 2:1 ...`.
2. Agent mencari "login", 60 detik kemudian "oauth login" -> `Reformulasi 1/2 50.0%`.
3. `--json` menghasilkan satu objek JSON valid (`report.*` sesuai 12.3) di stdout; log (bila ada) hanya di stderr, sehingga `... --json | jq .` bekerja.
4. `--since 2026-10-14 --until 2026-10-21 --window 5m` menghitung rentang tetap dan jendela 5 menit.
5. Rentang tanpa data -> laporan nol, rate 0, 11 bucket, exit 0.
6. Rentang besar yang masih di bawah batas (mis. 2.300 entri) -> sukses dengan banyak halaman scan.

Negatif:
1. `read_file` oleh key lain, project lain, atau 3 menit kemudian -> follow-up 0 (AC7).
2. Query yang sama persis diulang -> reformulasi 0 (AC8).
3. Entri audit lama tanpa `result_paths` -> tidak error, masuk Zero-result, tidak masuk Follow-up/Rank, tercatat di "Entri lama dilewati" (AC9).
4. CLI tanpa `--project` atau `--since`, `--since` tak terbaca, `--window 2h`, `--until` <= `--since`, argumen tambahan -> exit 2, usage di stderr, stdout kosong.
5. `--project` bukan uuid -> bus `INVALID_ARGUMENT`, CLI exit 1, stderr `search-quality: project_id tidak valid`.
6. Rentang yang memuat lebih dari 2.000.000 entri -> exit 1, stderr `search-quality: rentang terlalu besar (lebih dari 2000000 entri); persempit --since/--until`.
7. DB tidak terjangkau -> exit 1, stderr `search-quality report gagal; periksa log` (detail di log stderr, tanpa DSN).
8. Tabel belum dimigrasi -> exit 1, stderr menyarankan `grimoire migrate up`.
9. Provider `scanentries` belum terdaftar (wiring kurang) -> exit 1 dengan pesan generik; detail error bus di log stderr. Tes integrasi menangkapnya karena gagal.
10. Consumer lain memanggil endpoint laporan dengan pointer -> error membungkus `bus.ErrBadRequestType`.

---

## 8. Request/Response body

### 8.1 Bus `discovery.knowledgetools.search-quality-report`

Request (nilai struct Go `searchquality.Request`; JSON = nama field):

```json
{"ProjectID": "0192f0a4-6c1e-7b3a-9c55-2f1e8d7a4b10", "Since": "2026-10-14T00:00:00Z", "Until": "0001-01-01T00:00:00Z", "Window": 120000000000}
```

Response (`searchquality.Response`, sama dengan keluaran `--json`):

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

Tanpa data (project baru):

```json
{
  "report": {
    "project_id": "0192f0a4-6c1e-7b3a-9c55-2f1e8d7a4b10",
    "since": "2026-10-14T09:15:02Z",
    "until": "2026-10-21T09:15:02Z",
    "window_seconds": 120,
    "searches": 0,
    "zero_result": {"count": 0, "denominator": 0, "rate": 0},
    "follow_up": {"count": 0, "denominator": 0, "rate": 0},
    "reformulation": {"count": 0, "denominator": 0, "rate": 0},
    "rank_opened": [
      {"label": "1", "count": 0}, {"label": "2", "count": 0}, {"label": "3", "count": 0},
      {"label": "4", "count": 0}, {"label": "5", "count": 0}, {"label": "6", "count": 0},
      {"label": "7", "count": 0}, {"label": "8", "count": 0}, {"label": "9", "count": 0},
      {"label": "10", "count": 0}, {"label": "not_in_results", "count": 0}
    ],
    "legacy_searches": 0
  }
}
```

### 8.2 Bus `core.audit.scan-entries` (dipanggil feature ini; satu halaman)

Request:

```json
{
  "ProjectID": "0192f0a4-6c1e-7b3a-9c55-2f1e8d7a4b10",
  "Actions": ["mcp.search_knowledge", "mcp.read_file"],
  "From": "2026-10-14T00:00:00Z",
  "To": "2026-10-21T09:17:02Z",
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
      "Result": {"count": 3, "total": 3, "result_paths": ["docs/api/rate-limit.md", "docs/ops/nginx.md", "docs/faq.md"], "bytes": 812}
    }
  ],
  "Next": {"OccurredAt": "2026-10-15T08:31:00.000001Z", "ID": "0192f0b1-0000-7000-8000-00000000a002"}
}
```

Halaman terakhir: `"Next": null`.

### 8.3 CLI

Teks (default), sukses, exit 0:

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

### 8.4 Format error

Bus (`*errs.Error`):

```json
{"code": "INVALID_ARGUMENT", "message": "project_id tidak valid"}
```

Pesan lain dari params (tanpa nilai input): `"since wajib diisi"`, `"until harus setelah since"`, `"window harus 1s..1h"`. Dari usecase: `"rentang terlalu besar (lebih dari 2000000 entri); persempit --since/--until"`.

Kegagalan infra: error Go biasa `searchquality: scan audit: <cause>` (tanpa `*errs.Error`). Tipe body salah: `bus: unexpected request type: want=searchquality.Request got=*searchquality.Request` (`errors.Is(err, bus.ErrBadRequestType)`).

CLI:

| Kondisi | stderr | exit |
|---|---|---|
| Flag wajib hilang / tak terbaca / window di luar rentang / `--until` <= `--since` / argumen sisa | pesan `--since wajib diisi` dst. + usage | 2 |
| `*errs.Error` dari bus | `search-quality: project_id tidak valid` | 1 |
| Tabel belum ada | `tabel belum ada; jalankan "grimoire migrate up" lebih dulu` | 1 |
| Kegagalan infra | `search-quality report gagal; periksa log` | 1 |
| Sukses | (log saja) | 0 |

---

## 9. Urutan kerja ringkas untuk developer

1. Pastikan prasyarat FOUNDATION ada (`go build ./...` hijau, simbol `base.QualityAccumulator` ada).
2. `params.go` + `params_test.go`.
3. `repository.go`, `usecase.go` + `usecase_test.go` (repo palsu).
4. `handler.go`, `init.go` + `handler_test.go`.
5. `cmd/cli/searchquality.go`, ubah `cmd/cli/cli.go`, `searchquality_test.go`.
6. Tulis `searchquality_report_integration_test.go`. Jalankan setelah U2 (`scanentries`) ter-merge dan W1 memanggil `searchquality.Init(d)`; sebelum itu cukup lolos kompilasi (`go vet -tags integration ./cmd/internal/server/...`).
7. `go test ./...` dan `go test -tags integration ./cmd/internal/server/... -run 'T11_'` hijau; `make lint` bersih.

## 10. Risiko dan catatan

- AC10 dijalankan via biner (L8) karena `package main`; jika tim lebih suka memanggil `cli.run` langsung, pindahkan tes itu ke `cmd/cli/searchquality_integration_test.go` (di luar allowedPaths, perlu persetujuan).
- Duplikasi `minWindow/maxWindow` di CLI adalah satu-satunya duplikasi nilai; bus tetap menolak nilai di luar rentang.
- Follow-up adalah perkiraan; pembacaan tren dijelaskan di `docs/search-quality.md` (milik U3).
- Memori dibatasi oleh eviction akumulator (D13) dan batas 2.000.000 entri (L3); satu halaman = maksimum 1000 entri di memori.
