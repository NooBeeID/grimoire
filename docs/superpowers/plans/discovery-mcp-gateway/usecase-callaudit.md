# T07 discovery/mcp-gateway — Usecase `callaudit`

Melengkapi `docs/superpowers/plans/discovery-mcp-gateway/overview.md` (F1-F8: config, aksi audit, `toolkit`, `base/`, `base/auditwriter`, kerangka `mcpgateway.Register`). Bila dokumen ini bertentangan dengan overview, overview menang. Hanya satu usecase: `callaudit` (U1, paralel dengan `authenticate`). Penerapan K14-K18, K21, AC3, AC4, AC7.

## Metadata

| Field | Nilai |
|---|---|
| Ticket | T07 |
| Scope | fullstack pada tiket; usecase ini backend saja (tanpa UI, tanpa OpenAPI) |
| Domain | discovery |
| Module | mcp-gateway (`mcpgateway`) |
| Feature | `callaudit` (U1) |
| Endpoint | Tidak ada endpoint HTTP dan tidak ada endpoint bus. Dipanggil in-process oleh `endpoint` lewat `base.ToolInvoker.Invoke` untuk setiap `tools/call` yang params-nya valid |
| Method | `Invoke(ctx, Principal, name, args) Outcome` (Go interface, sinkron). Terkait HTTP: `POST /mcp` method JSON-RPC `tools/call` |
| Version | MCP 2025-11-25 / 2025-06-18 / 2025-03-26, JSON-RPC 2.0 (kontrak internal v1) |
| Bus dikonsumsi | `core.audit.record` (Call, sinkron, fail-open) lewat `auditwriter.Writer`. Tidak memanggil bus langsung |
| Auth | Sudah diselesaikan `authenticate`; `Principal` datang sebagai argumen |
| Audit | Tepat 1 entri per `Invoke`: `mcp.<tool>` atau `mcp.unknown_tool` |
| Repository / SQL | Tidak ada (tidak ada `repository.go`) |
| Resource root | `backend/internal/domain/discovery/internal/mcpgateway` |
| allowedPaths | `.../mcpgateway/callaudit/` dan satu baris di `.../mcpgateway/base.go` (mengganti stub `callaudit.Init`) |

Prasyarat dari fase F (bila belum ada, hentikan dan laporkan): `base.Catalog` (`Lookup`), `base.Principal`, `base.ToolInvoker`, `base.Outcome`, `base.Truncate`, `base.DecodeArgs`, `base.ToolCallAudit`, `base.UnknownToolAudit`, `auditwriter.Writer`/`auditwriter.New`, `toolkit.{Tool,Handler,Call,Result}`, aksi `mcp.unknown_tool` di `record.Actions()`, field `Config.MCPToolTimeout` (F1) dan `Config.MCPMaxResponseBytes` (sudah ada sejak T00, default 102400).

## Summary Plan

`callaudit` adalah implementasi `base.ToolInvoker`: pembungkus aman di sekeliling Handler tool. Ia menjamin tiga hal untuk setiap panggilan: (1) tool berjalan terisolasi (timeout, recover panic, error disamarkan) sehingga server tidak jatuh dan detail internal tidak bocor ke agent; (2) respons dibatasi ukurannya; (3) TEPAT satu entri audit tertulis, tanpa isi respons.

Alur `Invoke`:

1. `start := now()`. `Lookup(name)` di katalog.
2. Tidak ada -> tulis `base.UnknownToolAudit(p, name, dur)` (aksi `mcp.unknown_tool`, status `error`, `Args {name}` dipotong 128 byte aman UTF-8), kembalikan `Outcome{UnknownTool: true}`. Handler tidak dipanggil.
3. Ada -> bangun `toolkit.Call{ProjectID: p.ProjectID, Arguments: normalized}`. `ProjectID` SELALU dari Principal; `arguments` (termasuk bila memuat `project_id`) diteruskan apa adanya sebagai JSON opaque dan tidak pernah mempengaruhi `ProjectID` (K14, AC3).
4. `tctx, cancel := context.WithTimeout(ctx, MCPToolTimeout)`. Handler dijalankan di goroutine dengan `defer recover`, hasil dikirim ke channel ber-buffer 1, lalu `select { case r := <-ch; case <-tctx.Done() }`. `select` pada `tctx.Done()` membuat timeout tetap berlaku walau tool mengabaikan ctx; goroutine tool yang terlambat selesai menulis ke channel ber-buffer lalu berakhir sendiri (tidak ada goroutine bocor permanen kecuali tool benar-benar menggantung; itu keterbatasan Go yang didokumentasikan).
5. Pemetaan hasil:
   - Handler sukses: `Result` apa adanya (termasuk `IsError` dari tool, `ErrorCode` Outcome kosong).
   - Handler mengembalikan error Go: `IsError=true`, `ErrorCode="internal"`, teks generik + `trace_id`; error asli hanya ke log (`ErrorContext`), tidak ke agent dan tidak ke audit.
   - Timeout (`tctx.Err()` adalah `DeadlineExceeded`, baik `select` kalah maupun tool mengembalikan `ctx.Err()` yang membungkus `DeadlineExceeded`): `IsError=true`, `ErrorCode="timeout"`, teks timeout.
   - Panic: `IsError=true`, `ErrorCode="panic"`, teks generik; log `ErrorContext` dengan nilai panic dan stack (`debug.Stack()`).
   - Ctx induk dibatalkan klien (`context.Canceled`): dipetakan seperti error Go biasa (`internal`); respons tidak akan terbaca, audit tetap tertulis (writer memakai `WithoutCancel`).
6. `dur := now().Sub(start)` diukur setelah langkah 5, sebelum truncation.
7. `base.Truncate(res.Text, MCPMaxResponseBytes)` -> `Text` baru, `Truncated`, `OriginalBytes` (AC7). Berlaku juga untuk teks buatan gateway (praktis tidak pernah terpotong).
8. `base.ToolCallAudit(p, name, base.DecodeArgs(args), outcome, dur)` lalu `audit.Write(ctx, req)` sinkron, fail-open (log error dan Args/Result tidak pernah di-log ditangani `auditwriter`). `Text` tidak pernah masuk ke builder audit.
9. Kembalikan `Outcome`.

Keputusan lokal (tidak mengubah overview):

| # | Keputusan | Alasan |
|---|---|---|
| C1 | Satu titik tulis audit per jalur (akhir `Invoke` untuk tool terdaftar; satu panggilan di cabang unknown). Tidak ada `defer` penulis audit | Menjamin "tepat satu" secara struktural; tidak ada jalur ganda saat panic/timeout |
| C2 | `Invoke` tidak pernah panic dan tidak pernah mengembalikan error: seluruh kegagalan menjadi `Outcome` | Kontrak `ToolInvoker` |
| C3 | Argumen normalisasi untuk Handler: kosong/`null`/bukan object valid -> `{}`. Untuk audit dipakai `base.DecodeArgs(rawAsli)`, jadi bukan object -> `{_invalid:true}`. `endpoint` sudah menolak non-object (K15); ini hanya sabuk pengaman | Handler selalu menerima object JSON valid |
| C4 | Teks gateway (Indonesia, K25): internal/panic `Terjadi kesalahan internal saat menjalankan tool <name>. trace_id=<id>`; timeout `Tool <name> melebihi batas waktu <timeout>. Coba lagi dengan permintaan yang lebih sempit.` (`<timeout>` = `time.Duration.String()`, mis. `10s`) | Teks dibaca LLM; konsisten dengan penanda truncation |
| C5 | `trace_id` dari `reqctx.TraceID(ctx)`; kosong -> `"-"` | Pesan tidak pernah memuat `trace_id=` kosong |
| C6 | Hasil tool diabaikan bila `err != nil` (Summary/Target dibuang) agar tidak membocorkan metadata dari jalur gagal | Hasil gagal = pesan generik |
| C7 | Konfigurasi dibaca sekali di `Init` (`MCPToolTimeout`, `MCPMaxResponseBytes`); fallback defensif di `NewUsecase` bila nol (10s, 102400) | Mudah dites; config sudah divalidasi saat load |
| C8 | Jam diinjeksi (`Options.Now`) hanya untuk mengukur durasi. Tepat dua pemanggilan `Now()` per jalur tool terdaftar (awal dan setelah eksekusi), tiga TIDAK dipakai; jalur unknown juga dua | Tes `duration_ms` deterministik |
| C9 | Principal tidak dimasukkan ke ctx tool. Tool memakai `Call.ProjectID` saja | Memaksa satu jalur sumber project |

## Plan

Urutan wajib. Tulis test dulu (TDD), lalu implementasi.

**B0. Verifikasi (tanpa edit).** Cek simbol prasyarat di atas. Baca `backend/internal/infra/bus/bus.go` (`Registry.Register`, `Call`), `backend/internal/domain/discovery/internal/mcpgateway/base/{invoker,audit,truncate,args}.go`, dan `base/auditwriter/auditwriter.go`. Pastikan signature builder sesuai overview 5.8 (`ToolCallAudit(p, tool string, args map[string]any, o Outcome, dur time.Duration) record.Request`, `UnknownToolAudit(p, name string, dur time.Duration) record.Request`). Bila beda, ikuti kode yang ada dan laporkan.

**B1. Test dulu** `callaudit/usecase_test.go` (bagian Test Scenario). Helper test (di file test, bukan produksi):
- `fakeWriter` yang mengimplementasikan `auditwriter.Writer` dan menyimpan `[]record.Request` dengan mutex.
- `newUC(t, tools, opt)` merakit `base.NewCatalog(tools)` + `NewUsecase`.
- `fakeClock` berurutan untuk `duration_ms`.
- Tool palsu: `okTool`, `errTool`, `panicTool`, `blockTool` (memblokir di channel, mengabaikan ctx), `ctxTool` (menunggu `ctx.Done()` lalu `return ctx.Err()`), `bigTool(n)`.
- Konstanta `secret = "SECRET-CONTENT-123"` untuk asersi kebocoran.
Jalankan: gagal (RED).

**B2. `callaudit/usecase.go`.**

```go
package callaudit

type Options struct {
	Timeout  time.Duration
	MaxBytes int
	Now      func() time.Time
}

type Usecase struct {
	cat   base.Catalog
	audit auditwriter.Writer
	log   *slog.Logger
	opt   Options
}

var _ base.ToolInvoker = (*Usecase)(nil)

func NewUsecase(cat base.Catalog, audit auditwriter.Writer, log *slog.Logger, opt Options) *Usecase // default C7

func (u *Usecase) Invoke(ctx context.Context, p base.Principal, name string, args json.RawMessage) base.Outcome
```

Pseudocode `Invoke`:

```go
start := u.opt.Now()
tool, ok := u.cat.Lookup(name)
if !ok {
	u.audit.Write(ctx, base.UnknownToolAudit(p, name, u.opt.Now().Sub(start)))
	return base.Outcome{UnknownTool: true}
}
run := u.run(ctx, p, tool, args)         // runResult{res toolkit.Result, code string}
dur := u.opt.Now().Sub(start)
text, truncated, orig := base.Truncate(run.res.Text, u.opt.MaxBytes)
run.res.Text = text
out := base.Outcome{Result: run.res, Truncated: truncated, OriginalBytes: orig, ErrorCode: run.code}
u.audit.Write(ctx, base.ToolCallAudit(p, tool.Name, base.DecodeArgs(args), out, dur))
return out
```

Pseudocode `run` (method privat, bukan usecase terpisah):

```go
call := toolkit.Call{ProjectID: p.ProjectID, Arguments: normalizeArgs(args)} // C3
tctx, cancel := context.WithTimeout(ctx, u.opt.Timeout)
defer cancel()

type done struct { res toolkit.Result; err error; panicked bool }
ch := make(chan done, 1) // ber-buffer: goroutine terlambat tidak pernah terblokir
go func() {
	defer func() {
		if r := recover(); r != nil {
			u.log.ErrorContext(ctx, "mcp tool panic", "tool", tool.Name, "panic", fmt.Sprint(r), "stack", string(debug.Stack()))
			ch <- done{panicked: true}
		}
	}()
	res, err := tool.Handler(tctx, call)
	ch <- done{res: res, err: err}
}()

select {
case d := <-ch:
	switch {
	case d.panicked:                  return gatewayFailure(tool.Name, ctx, "panic", u.opt.Timeout)
	case d.err != nil && isDeadline(tctx, d.err):  log warn; return timeout
	case d.err != nil:                log error (tool, err); return internal
	default:                          return runResult{res: d.res}
	}
case <-tctx.Done():
	if errors.Is(tctx.Err(), context.DeadlineExceeded) { log warn "mcp tool timeout"; return timeout }
	log warn "mcp tool canceled"; return internal  // klien memutus koneksi
}
```

`isDeadline(tctx, err)` = `errors.Is(err, context.DeadlineExceeded) && errors.Is(tctx.Err(), context.DeadlineExceeded)`. Helper `gatewayFailure(name, ctx, code, timeout)` membangun `toolkit.Result{Text: msg, IsError: true}` sesuai C4/C5 dan `runResult.code`. `normalizeArgs(raw)`: trim spasi; kosong, `null`, atau bukan `{...}` valid (`json.Valid`) -> `json.RawMessage("{}")`; selain itu `raw`.

Aturan penulisan:
- Log memakai `*Context(ctx, ...)` dengan key `tool`, `err`/`panic`/`stack`, tidak pernah `args`, `Text`, atau `Summary`.
- Tidak ada `os.Exit`, tidak ada `panic` buatan, tidak ada import `bus`/`db`/`http`.
- Teks gateway tidak memuat pesan error asli.

**B3. `callaudit/init.go`.**

```go
package callaudit

// Init merakit invoker dari config. Dipanggil mcpgateway.Register (F7); mengembalikan interface, bukan *Usecase.
func Init(d app.Deps, cat base.Catalog, audit auditwriter.Writer) base.ToolInvoker {
	return NewUsecase(cat, audit, d.Log, Options{
		Timeout:  d.Config.MCPToolTimeout,
		MaxBytes: d.Config.MCPMaxResponseBytes,
		Now:      time.Now,
	})
}
```
Tanpa `handler.go`, tanpa `params.go`, tanpa `repository.go`, tanpa route, tanpa registrasi bus.

**B4. Wiring.** Di `mcpgateway/base.go` ganti stub `callaudit.Init` (bila F7 memakai stub) dengan pemanggilan sesungguhnya; baris `inv := callaudit.Init(d, cat, audit)` sudah sesuai overview F7. Tidak ada perubahan lain di `base.go`.

**B5. Verifikasi.** `go vet ./...`, `go test -race ./internal/domain/discovery/internal/mcpgateway/callaudit/...`, `go test ./internal/archtest/...`, `golangci-lint run`. Tidak ada tes integrasi Postgres untuk usecase ini (tidak ada SQL); cakupan end-to-end ada di I1/I2 overview.

## Migration

Tidak ada. Tidak ada tabel, tidak ada file `YYYYMMDDHHMMSSmmm_*.sql`.

## Changes

| File/folder | Aksi | Isi |
|---|---|---|
| `backend/internal/domain/discovery/internal/mcpgateway/callaudit/init.go` | baru | `Init(d, cat, audit) base.ToolInvoker` |
| `.../callaudit/usecase.go` | baru | `Options`, `Usecase`, `NewUsecase`, `Invoke`, `run`, `gatewayFailure`, `normalizeArgs`, `isDeadline` |
| `.../callaudit/usecase_test.go` | baru | semua tes bagian Test Scenario |
| `.../mcpgateway/base.go` | ubah (satu baris) | ganti stub dengan `callaudit.Init(d, cat, audit)` |
| `base/**`, `base/auditwriter/**`, `toolkit`, `config`, `record` | tidak berubah | disediakan fase F |
| `api/**`, `backend/migrations/**`, `lib/constants/bus.go`, `go.mod` | tidak berubah | - |

Dependensi import `callaudit`: `grimoire/internal/infra/app`, `.../mcpgateway/base`, `.../mcpgateway/base/auditwriter`, `grimoire/lib/contracts/discovery/mcpgateway/toolkit`, `grimoire/lib/common/reqctx`, stdlib (`context`, `encoding/json`, `errors`, `fmt`, `log/slog`, `runtime/debug`, `time`, `bytes`). Tidak meng-import module/domain lain (rules 4).

## Test Scenario

Semua tes unit, tanpa Postgres, dijalankan dengan `-race`. Penulis audit palsu (`fakeWriter`) untuk asersi isi entri; satu tes memakai `auditwriter.New` nyata di atas `bus.NewRegistry` (bus palsu dengan handler `core.audit.record` buatan) untuk asersi fail-open dan log. Principal uji: `KeyID=k-1`, `KeyName=claude-laptop`, `KeyPrefix=a1b2c3d4`, `ProjectID=P-A`, `ProjectName=Payments`, `IP=10.0.0.7`, `UserAgent=claude-code/2.1.0`. Timeout uji dikecilkan (mis. 50 ms) menggantikan 10 detik.

Kriteria penerimaan:

- `TestT07_AC4_ExactlyOneAuditPerCall` (tabel, satu sub-test per jalur; tiap sub-test memeriksa `len(entries)==1`, aksi benar, dan `secret` TIDAK ada di `fmt.Sprintf("%#v", entries)` maupun di Text yang dikembalikan untuk jalur gagal):
  - sukses: tool mengembalikan `Text` berisi `secret`, `Target="docs/a.md"`, `TargetType="file"`, `Summary{count:3}` -> aksi `mcp.read_file`, status `ok`, `Target/TargetType` dari Result, `Result` memuat `bytes`, `original_bytes`, `truncated=false`, `duration_ms`, `count=3`; `Text` tidak ada di entri.
  - tool `IsError:true` (err nil): status `error`, `error_code` tidak ada di Result (Outcome.ErrorCode kosong) kecuali tool menyertakannya di Summary.
  - error Go bermuatan `secret`: Outcome `IsError`, `ErrorCode=internal`, Text memuat `trace_id=` dan tidak memuat `secret`; status audit `error`; Result `error_code=internal`.
  - timeout (tool `blockTool` mengabaikan ctx): `Invoke` kembali dalam batas wajar (< 1 detik dengan timeout 50 ms), `ErrorCode=timeout`, audit `error_code=timeout`.
  - timeout kooperatif (tool `ctxTool` mengembalikan `ctx.Err()`): hasil sama, `ErrorCode=timeout`, bukan `internal`.
  - panic (nilai panic mengandung `secret`): `ErrorCode=panic`, Text generik tanpa `secret`; log memuat `mcp tool panic` dan `stack`; audit `error_code=panic`.
  - unknown: aksi `mcp.unknown_tool`, status `error`, `Args {name}`, `Result.error_code=unknown_tool`, `Outcome.UnknownTool=true`, handler apa pun tidak dipanggil.
- `TestT07_AC4_NoTextInAudit`: `Summary` tool berisi key `bytes: 1` -> nilai ditimpa ke panjang Text sebenarnya (key gateway menang); key lain (`count`) tetap ada.
- `TestT07_AC7_Truncate500KB`: tool mengembalikan 512000 byte ASCII, `MaxBytes=102400` -> `len(Text) <= 102400`, memuat `[truncated:`, `Truncated=true`, `OriginalBytes=512000`; audit `bytes == len(Text)`, `original_bytes == 512000`, `truncated == true`. Varian multibyte (emoji 4 byte, aksara 3 byte, total 500 KB, batas jatuh di tengah rune) -> `utf8.ValidString(Text)`.
- `TestT07_AC7_NotTruncatedWithinLimit`: tepat sama dengan batas tidak dipotong.
- `TestT07_AC3_ProjectIDFromPrincipal`: `args = {"path":"x","project_id":"P-B"}`, Principal `P-A` -> tool menerima `Call.ProjectID == "P-A"`, `Call.Arguments` identik byte dengan input (project_id tidak diubah dan tidak dibuang), audit `ProjectID == "P-A"`, `ProjectLabel == "Payments"`. Varian: `args` kosong dan `null` -> tool menerima `{}`; `args` array -> tool menerima `{}`, audit `Args == {_invalid:true}`.
- `TestT07_ToolTimeoutUsesConfiguredDuration`: dengan `Options.Timeout=30ms` tool yang tidur 200 ms (mengabaikan ctx) menghasilkan timeout; dengan `Timeout=500ms` tool yang tidur 20 ms sukses. Teks timeout memuat `30ms`.
- `TestT07_PanicRecovered_ServerSurvives`: setelah satu tool panic, panggilan berikutnya ke tool sehat pada usecase yang sama berhasil (tidak ada state rusak); proses uji tidak crash.
- `TestT07_LateToolDoesNotBlockOrLeak`: setelah timeout, lepaskan `blockTool`; goroutine selesai (WaitGroup di tool) tanpa deadlock walau tidak ada pembaca channel; `go test -race` bersih.
- `TestT07_AuditFailureFailOpen`: `auditwriter.New` nyata di atas registry dengan handler `core.audit.record` yang mengembalikan error. Hasil tool tetap dikembalikan utuh (`Text`, `IsError=false`), log (buffer `slog`) memuat `audit write failed` dan `mcp.read_file`, dan log TIDAK memuat nilai `Args`/`Result` (mis. path atau `secret`). Handler audit dipanggil tepat sekali dan sudah selesai saat `Invoke` kembali (sinkron).
- `TestT07_AuditWrittenEvenIfClientCanceled`: ctx induk dibatalkan saat tool berjalan (`ctxTool`) -> `Outcome.ErrorCode=internal`, entri audit tetap tertulis tepat satu kali.
- `TestT07_DurationMeasured`: `fakeClock` maju 1500 ms antar dua panggilan `Now()` -> `Result["duration_ms"] == int64(1500)`; jalur unknown juga memuat `duration_ms`.
- `TestT07_UnknownToolNameTruncated`: nama 300 byte (campuran multibyte) -> `Args["name"]` berukuran <= 128 byte dan UTF-8 valid; `Outcome.UnknownTool`.
- `TestT07_ActorFieldsFromPrincipal`: entri memuat `ActorType=api_key`, `ActorID=k-1`, `ActorLabel="claude-laptop (a1b2c3d4)"`, `IP`, `UserAgent`; `TraceID` kosong di entri (diisi provider dari ctx).
- `TestNewUsecaseDefaults`: `Options` nol -> timeout 10 detik, `MaxBytes` 102400, `Now` non-nil; `log` nil tidak menyebabkan panic.

Lintas feature (diselesaikan di I1 `mcpgateway/base_test.go`, bukan di plan ini, tetapi `callaudit` harus memenuhinya): `TestT07_AC3_ProjectIDFromKeyOnly`, `TestT07_AC4_ExactlyOneAuditNoContent`, dan `TestT07_AC1_JSONRPCFlow` memakai invoker nyata ini lewat `POST /mcp`.

Infra: `go vet`, `golangci-lint`, archtest (tanpa pelanggaran baru) wajib hijau.

## E2E Scenarios

Usecase ini tidak punya UI atau route sendiri. Skenario berikut ditambahkan ke `frontend/e2e/mcp-gateway.spec.ts` (milik I6; dikerjakan di sana, bukan di plan ini) dan bergantung pada `endpoint` + `authenticate`.

Positif:
1. (Setelah T08 merge; sebelum itu `test.fixme`) `tools/call read_file` dengan key valid -> 200, `result.isError=false`, `content[0].text` terisi; di `/audit-log?actor_type=api_key&actor_id=<keyId>` ada tepat satu baris `mcp.read_file` status ok, dan isi file tidak tampil di detail baris.
2. (Setelah T08) arguments memuat `project_id` project lain -> hasil tetap dari project milik key; baris audit memakai project milik key.

Negatif (bisa aktif pada T07 karena katalog kosong):
3. `tools/call` nama `write_file` -> 200 dengan `error.code == -32602`, `error.message == "Unknown tool: write_file"`; filter aksi `mcp.unknown_tool` di audit log menampilkan tepat satu baris status error dengan target `write_file`.
4. `tools/call` dengan params tidak valid (`arguments` string) -> `-32602` dan TIDAK ada baris audit baru (tanggung jawab `endpoint`, diverifikasi dengan menghitung baris sebelum dan sesudah).
5. (Setelah T08) tool timeout/panic tidak dapat diprovokasi lewat UI; dicakup tes unit. Tidak ada skenario e2e.

Catatan: opsi filter `mcp.unknown_tool` di UI audit log berasal dari I6 overview (`ACTION_GROUPS`).

## Request/Response body

`callaudit` tidak punya body HTTP sendiri; kontraknya `Invoke` dan entri audit. Bagian ini juga menunjukkan pemetaan `Outcome` menjadi respons JSON-RPC oleh `endpoint` (untuk kejelasan, bukan milik usecase ini).

### Pemanggilan Go

```go
out := inv.Invoke(ctx, principal, "read_file", json.RawMessage(`{"path":"docs/auth/login.md"}`))
// out.UnknownTool=false, out.Result.Text sudah dipotong, out.Truncated, out.OriginalBytes, out.ErrorCode
```

### Respons JSON-RPC hasil pemetaan (HTTP 200)

Sukses:
```json
{"jsonrpc":"2.0","id":3,"result":{"content":[{"type":"text","text":"# Login\n\nAlur login memakai cookie session..."}],"isError":false}}
```
Dipotong (AC7):
```json
{"jsonrpc":"2.0","id":3,"result":{"content":[{"type":"text","text":"...\n\n[truncated: respons dipotong pada 102280 byte dari 512000 byte. Persempit permintaan dengan argumen offset/limit.]"}],"isError":false,"_meta":{"truncated":true,"original_bytes":512000}}}
```
Kegagalan tool (dari tool, `ErrorCode` kosong):
```json
{"jsonrpc":"2.0","id":3,"result":{"content":[{"type":"text","text":"File \"docs/auth/logn.md\" tidak ditemukan. Mungkin maksudnya: docs/auth/login.md"}],"isError":true}}
```
Timeout (`ErrorCode=timeout`):
```json
{"jsonrpc":"2.0","id":3,"result":{"content":[{"type":"text","text":"Tool read_file melebihi batas waktu 10s. Coba lagi dengan permintaan yang lebih sempit."}],"isError":true}}
```
Error Go / panic (`ErrorCode=internal` / `panic`; teks sama, detail hanya di log):
```json
{"jsonrpc":"2.0","id":3,"result":{"content":[{"type":"text","text":"Terjadi kesalahan internal saat menjalankan tool read_file. trace_id=01HZX3K9Q7"}],"isError":true}}
```
Tool tidak dikenal (`Outcome.UnknownTool=true`, error protokol, tanpa `isError`):
```json
{"jsonrpc":"2.0","id":3,"error":{"code":-32602,"message":"Unknown tool: write_file"}}
```

### Entri audit yang dikirim ke `core.audit.record` (satu per panggilan)

Sukses (`mcp.read_file`):
```json
{
  "ActorType": "api_key", "ActorID": "0192f0c4-7a1e-7c3a-9b1d-3c4e5f6a7b8c", "ActorLabel": "claude-laptop (a1b2c3d4)",
  "ProjectID": "0192f0c4-0000-7000-8000-00000000000a", "ProjectLabel": "Payments",
  "Action": "mcp.read_file", "TargetType": "file", "Target": "docs/auth/login.md",
  "Args": {"path": "docs/auth/login.md"},
  "Result": {"bytes": 2048, "original_bytes": 2048, "truncated": false, "duration_ms": 12},
  "Status": "ok", "IP": "10.0.0.7", "UserAgent": "claude-code/2.1.0"
}
```
Dipotong:
```json
{
  "Action": "mcp.read_file", "TargetType": "file", "Target": "docs/big.md",
  "Args": {"path": "docs/big.md"},
  "Result": {"bytes": 102400, "original_bytes": 512000, "truncated": true, "duration_ms": 31},
  "Status": "ok"
}
```
(field aktor/project/IP/UA sama seperti contoh sukses)

Timeout:
```json
{
  "Action": "mcp.read_file", "Args": {"path": "docs/slow.md"},
  "Result": {"bytes": 87, "original_bytes": 87, "truncated": false, "duration_ms": 10001, "error_code": "timeout"},
  "Status": "error"
}
```
Panic:
```json
{
  "Action": "mcp.read_file", "Args": {"path": "docs/x.md"},
  "Result": {"bytes": 96, "original_bytes": 96, "truncated": false, "duration_ms": 2, "error_code": "panic"},
  "Status": "error"
}
```
Arguments bukan object:
```json
{"Action": "mcp.read_file", "Args": {"_invalid": true}, "Result": {"bytes": 41, "original_bytes": 41, "truncated": false, "duration_ms": 0}, "Status": "ok"}
```
Tool tidak dikenal:
```json
{
  "ActorType": "api_key", "ActorID": "0192f0c4-7a1e-7c3a-9b1d-3c4e5f6a7b8c", "ActorLabel": "claude-laptop (a1b2c3d4)",
  "ProjectID": "0192f0c4-0000-7000-8000-00000000000a", "ProjectLabel": "Payments",
  "Action": "mcp.unknown_tool", "TargetType": "tool", "Target": "write_file",
  "Args": {"name": "write_file"},
  "Result": {"error_code": "unknown_tool", "duration_ms": 0},
  "Status": "error", "IP": "10.0.0.7", "UserAgent": "claude-code/2.1.0"
}
```

### Log fail-open (tidak ada body ke klien)

Bila `core.audit.record` gagal, klien tetap menerima hasil tool normal. Log aplikasi (level error): `audit write failed action=mcp.read_file err="..."`; tanpa `Args`/`Result`.
