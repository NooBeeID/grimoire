# T02 core/audit-log — Usecase `record`

Konteks bersama (migrasi, konstanta bus, kontrak `lib/contracts/core/audit/record`, entity `auditlog/base/`) ada di `overview.md` dan dikerjakan di langkah foundation F1-F4. Dokumen ini hanya memuat usecase `record`. Tiket: `docs/tickets/T02-core-audit-log.md`. Keputusan: **fail-open** (AC8).

## Metadata

| Field | Nilai |
|---|---|
| Scope | backend (Go) saja; tidak ada UI |
| Domain | core |
| Module | audit-log (folder Go `auditlog`) |
| Feature | `record` |
| Resource root | `backend/internal/domain/core/internal/auditlog` |
| allowedPaths | `backend/internal/domain/core/internal/auditlog/record/` |

| Endpoint | Method | Version | Akses |
|---|---|---|---|
| bus `core.audit.record` (`constants.BUS_CORE_AUDIT_RECORD`) | Call dan Publish (satu registrasi, satu handler) | v1 | internal (T01, T03-T07) |

Tidak ada route HTTP.

## Summary Plan

`record` adalah handler bus yang menyimpan satu entri audit. Handler memeriksa tipe request (`record.Request` sebagai nilai), usecase mengisi `TraceID` dari `reqctx` bila kosong, memasang timeout 2 detik, membentuk `base.Entry` lewat `base.NewEntry` (validasi + sanitasi ada di entity, bukan di usecase), lalu repository menjalankan satu INSERT parameterized. Sisi provider fail-open: usecase tidak pernah panic, tidak me-log di jalur Call, dan hanya mengembalikan error (`errs.ErrInvalid` untuk validasi, `audit record: %w` untuk DB/timeout) dengan `Response{}` (Recorded=false). Pemanggil yang menangkap error, me-log level error, dan tetap melayani permintaan. Jalur Publish sudah di-log oleh bus (`bus publish failed`).

Satu usecase, tidak dipecah. Rule bisnis (validasi, truncate, redaksi) sudah di `base/` (foundation F4); `record` tidak menduplikasi.

Prasyarat (harus sudah ada dari foundation): migrasi `audit_log` (F1), `constants.BUS_CORE_AUDIT_RECORD` (F2), kontrak `record.Request/Response` (F3), `base.NewEntry`, `base.Entry` (F4). Bila belum ada saat implementasi, hentikan dan selesaikan foundation lebih dulu; jangan membuatnya dari dalam folder `record/`.

## Plan

Urutan langkah (semua di `backend/internal/domain/core/internal/auditlog/record/`):

1. **`usecase.go`** (tulis dulu bersama tesnya, TDD).
   - Interface (sisi consumer, rule 5.6):
     ```go
     type Repository interface {
         Insert(ctx context.Context, e base.Entry) error
     }
     ```
   - ```go
     const insertTimeout = 2 * time.Second

     type Usecase struct {
         repo  Repository
         now   func() time.Time
         newID func() string
     }
     func NewUsecase(repo Repository, now func() time.Time, newID func() string) *Usecase
     func (u *Usecase) Execute(ctx context.Context, req record.Request) (record.Response, error)
     ```
   - Alur `Execute` (orkestrasi saja):
     1. `if req.TraceID == "" { req.TraceID = reqctx.TraceID(ctx) }`.
     2. `e, err := base.NewEntry(req, u.now(), u.newID)`; `err != nil` -> `return record.Response{}, err` (sudah membungkus `errs.ErrInvalid`; jangan di-wrap ulang dan jangan menyertakan nilai input).
     3. `ctx, cancel := context.WithTimeout(ctx, insertTimeout); defer cancel()`.
     4. `if err := u.repo.Insert(ctx, e); err != nil { return record.Response{}, fmt.Errorf("audit record: %w", err) }`.
     5. `return record.Response{Recorded: true}, nil`.
   - Tidak ada `log.*`, tidak ada `panic`, tidak ada `recover` (bus `Publish` sudah punya recover). Tidak membaca/menulis tabel lain, tidak memanggil bus.
   - Catatan Publish: bus memakai `context.WithoutCancel`, jadi timeout 2 s dari langkah 3 tetap satu-satunya batas waktu.
2. **`repository.go`**: satu-satunya file yang menyentuh SQL.
   ```go
   type Repo struct{ db sqlx.ExtContext } // terima *db.DB (embed *sqlx.DB) dari Init; boleh db.Querier bila foundation menyediakan
   func NewRepo(db sqlx.ExtContext) *Repo
   func (r *Repo) Insert(ctx context.Context, e base.Entry) error
   ```
   SQL tunggal, argumen `$n`, tanpa concat input (rule 5.3), tanpa join, tanpa transaksi:
   ```sql
   INSERT INTO audit_log
     (id, occurred_at, actor_type, actor_id, actor_label, project_id, project_label,
      action, target_type, target, args, status, result, ip, user_agent, trace_id)
   VALUES
     ($1::uuid, $2, $3, $4::uuid, $5, $6::uuid, $7, $8, $9, $10, $11::jsonb, $12, $13::jsonb, $14::inet, $15, $16)
   ```
   Pemetaan argumen:
   - `actor_id`, `project_id`, `project_label`, `target_type`, `target`, `ip`, `user_agent`: string kosong -> `nil` (NULL) lewat helper privat `nullIfEmpty(s string) any`.
   - `actor_label` dan `trace_id` NOT NULL: kirim apa adanya (trace_id kosong tersimpan `''`, sesuai default migrasi).
   - `args`/`result`: `json.Marshal(e.Args)` lalu kirim sebagai `string(b)` (bukan `[]byte`, agar driver pgx tidak memperlakukannya sebagai bytea) dengan cast `::jsonb`. Entity menjamin map non-nil (`{}`).
   - `occurred_at`: `e.OccurredAt` (sudah UTC).
   - Error dikembalikan mentah (dibungkus di usecase). Jangan menyertakan nilai args di pesan error.
   - Tidak ada fungsi UPDATE/DELETE di file ini (diperiksa tes arsitektur W2 AC7).
3. **`handler.go`**: adapter bus.
   ```go
   type handler struct{ uc *Usecase }
   func (h handler) Handle(ctx context.Context, req any) (any, error) {
       in, err := bus.Expect[record.Request](req) // tipe salah (mis. *record.Request) -> error membungkus bus.ErrBadRequestType
       if err != nil { return nil, err }
       resp, err := h.uc.Execute(ctx, in)
       if err != nil { return record.Response{}, err }
       return resp, nil
   }
   ```
   Mengembalikan `record.Response` sebagai nilai (agar `bus.CallTyped[record.Response]` cocok). Saat tipe salah kembalikan `nil, err` (konsisten dengan konvensi bus; pemanggil tidak memakai respons ketika err != nil).
4. **`init.go`**:
   ```go
   func Init(d app.Deps) {
       repo := NewRepo(d.DB)
       uc := NewUsecase(repo, time.Now, newUUIDv7)
       d.Bus.Register(constants.BUS_CORE_AUDIT_RECORD, handler{uc: uc}.Handle)
   }
   func newUUIDv7() string { return uuid.Must(uuid.NewV7()).String() }
   ```
   Satu `Register`, tidak ada registrasi kedua untuk mode lain (registrasi ganda = panic, rule 6.1). Nama import infra mengikuti paket final di `main` pasca T00 (lihat catatan overview).
5. **`params.go`**: tidak dibuat. Tidak ada transport HTTP dan tidak ada mapping Request->input yang diperlukan (`record.Request` langsung dipakai `base.NewEntry`). Bila implementer merasa perlu, isinya hanya konstanta/mapping murni, tanpa logika bisnis; jangan membuat file kosong.
6. **Tes** (lihat Test Scenario). Urutan: `usecase_test.go` (RED lalu hijau), `handler_test.go`, `repository_test.go` (tag `integration`), `publish_test.go` (tag `integration`).
7. **Verifikasi**: `cd backend && go build ./... && go vet ./... && go test ./internal/domain/core/internal/auditlog/record/...`, lalu dengan `TEST_DATABASE_URL`: `go test -tags integration ./internal/domain/core/internal/auditlog/record/...`.

Kontrak bus (ringkas, definisi lengkap di overview F3):

| Aspek | Nilai |
|---|---|
| Endpoint | `core.audit.record` |
| Request | `record.Request` (nilai, bukan pointer) |
| Response sukses | `record.Response{Recorded: true}` |
| Validasi gagal | error membungkus `errs.ErrInvalid` (`errors.Is`), `Response{}` |
| Tipe request salah | error membungkus `bus.ErrBadRequestType` |
| DB gagal / timeout 2 s | `fmt.Errorf("audit record: %w", err)`, `Response{}` |
| Logging provider | tidak ada di jalur Call; Publish di-log bus |

## Migration

Tidak ada migrasi baru di usecase ini. Tabel `audit_log`, 4 indeks, dan trigger `audit_log_no_update` dibuat oleh migrasi foundation F1 `backend/migrations/core/20261021090000000_create_audit_log.sql`. Tes integrasi `record` menerapkannya ke schema uji (lihat di bawah).

## Changes

| File | Aksi | Isi |
|---|---|---|
| `.../auditlog/record/init.go` | buat | rakit repo -> usecase -> `d.Bus.Register(BUS_CORE_AUDIT_RECORD, ...)`; `newUUIDv7` |
| `.../auditlog/record/handler.go` | buat | `bus.Expect[record.Request]`, panggil usecase, kembalikan `record.Response` |
| `.../auditlog/record/usecase.go` | buat | interface `Repository`, `Usecase`, `Execute`, `insertTimeout` |
| `.../auditlog/record/repository.go` | buat | `Repo.Insert` (satu INSERT), `nullIfEmpty` |
| `.../auditlog/record/usecase_test.go` | buat | unit dengan repo palsu |
| `.../auditlog/record/handler_test.go` | buat | unit tipe request salah, sukses, propagasi error |
| `.../auditlog/record/repository_test.go` | buat | integrasi (tag `integration`) |
| `.../auditlog/record/publish_test.go` | buat | integrasi Publish via bus nyata (tag `integration`) |

Tidak menyentuh file di luar folder `record/`. Pemanggilan `record.Init(d)` dari `auditlog/base.go` ditulis di langkah wiring W1 (bukan bagian usecase ini).

## Test Scenario

Penamaan: tes yang membuktikan kriteria memakai `TestT02_ACn_*` (rule 13.4); lainnya `Test<Unit>_<Kasus>`. Usecase dites dengan repo palsu (rule 13.2); repository dengan Postgres nyata (rule 13.3). `now` dan `newID` diinjeksi tetap.

### Unit usecase (`usecase_test.go`, repo palsu menyimpan `base.Entry` terakhir dan bisa dipaksa error)

- `TestRecord_Success`: request api_key lengkap -> `Response{Recorded:true}`, repo menerima 1 `Entry` dengan `ID == newID()`, `OccurredAt == now` (UTC) saat input nol, field lain sesuai.
- `TestRecord_TraceIDFromContext`: `req.TraceID == ""` dan `reqctx.WithTraceID(ctx, "t-123")` -> Entry.TraceID `"t-123"`. `req.TraceID = "explicit"` -> tetap `"explicit"` (ctx diabaikan).
- `TestT02_AC4_PasswordRedacted`: `Args{"password":"hunter2","path":"a.md"}` -> Entry.Args `password == "[REDACTED]"`, `path` utuh; marshal Entry.Args tidak mengandung `hunter2`.
- `TestT02_AC4_ContentOmitted`: `Result{"content":"isi file rahasia","bytes":12}` dan `Args{"content":"..."}` -> `"[OMITTED]"`; `bytes` utuh; marshal tidak mengandung `isi file rahasia`.
- `TestRecord_LongStringTruncated`: string 3000 byte di Args -> <= 2048 byte + sufiks `…[truncated]`, `utf8.ValidString`.
- `TestRecord_ArgsOver8KBReplacedByMarker`: 10 kunci x 1500 byte -> Entry.Args `{"_truncated":true,"_original_bytes":N,"_keys":[...]}` dengan N > 8192.
- `TestT02_AC2_AnonymousWithKeyPrefixAccepted`: ActorType anonymous, ActorID kosong, ActorLabel `"gk_ab12"`, Action `mcp.auth.rejected`, Status denied -> Recorded true, Entry.ActorLabel `"gk_ab12"`, ActorID kosong.
- `TestT02_AC2_AnonymousWithActorIDRejected`: anonymous dengan ActorID uuid -> `errors.Is(err, errs.ErrInvalid)`, `Recorded == false`, repo TIDAK dipanggil.
- `TestRecord_InvalidRejected` (table): Action kosong/`"MCP.read"`/`"read_file"`, ActorType `"admin"`, user tanpa ActorID, Status `"failed"`, ProjectID `"x"`, ID `"abc"` -> semua `errs.ErrInvalid`, repo tidak dipanggil, pesan error tidak memuat nilai input rahasia.
- `TestT02_AC8_RepoErrorReturned`: repo mengembalikan `errors.New("conn refused")` -> `Recorded == false`, `err != nil`, `errors.Is(err, sentinel)`, `strings.HasPrefix(err.Error(), "audit record: ")`; tidak panic (jalankan di bawah `require.NotPanics`/defer recover yang gagal bila panic).
- `TestT02_AC8_RepoTimeout`: repo palsu memblokir sampai `ctx.Done()` lalu mengembalikan `ctx.Err()`; ctx induk tanpa deadline -> `Execute` kembali dalam sekitar 2 s (toleransi, atau buat `insertTimeout` variabel paket yang di-override tes menjadi 20 ms) dengan `errors.Is(err, context.DeadlineExceeded)`, `Recorded == false`. Juga cek repo palsu menerima ctx ber-deadline (`ctx.Deadline()` ok, <= 2 s dari sekarang).
- `TestRecord_NoLogOnFailure`: logger default `slog` diganti handler penangkap (atau tidak ada logger di usecase); pastikan tidak ada record log yang dihasilkan saat gagal (usecase tidak punya dependensi logger, tes mengasersi dengan menjalankan tanpa menyuntik logger apa pun dan cek kompilasi struct `Usecase` tidak punya field `log`).

### Unit handler (`handler_test.go`)

- `TestHandler_WrongTypeRejected`: request `*record.Request`, `record.Response{}`, `"string"`, `nil` -> `errors.Is(err, bus.ErrBadRequestType)`, repo tidak dipanggil.
- `TestHandler_ReturnsResponseValue`: sukses -> `any` bertipe `record.Response` (type assertion nilai, bukan pointer).
- `TestHandler_PropagatesError`: usecase gagal -> error diteruskan; resp (jika dibaca) `Recorded == false`.
- `TestInit_SingleRegistration`: `Init` pada `bus.NewRegistry` baru tidak panic; `Init` kedua pada registry yang sama panic (registrasi ganda); `Call` dan `Publish` ke endpoint yang sama menemukan handler.

### Integrasi repository (`repository_test.go`, tag `integration`, `db.TestDB(t)`)

Helper `setup(t)`: `db.TestDB(t)` lalu terapkan migrasi core dengan `db.Migrate(ctx, d, os.DirFS("../../../../../../migrations/core"), log)` (jalur relatif dari folder `record/` ke `backend/migrations/core`; sesuaikan bila foundation menyediakan helper migrasi uji atau `migrations.FS` embed).

- `TestT02_AC1_FullAPIKeyEntryStored`: `Insert` entri api_key lengkap (actor_label `claude-code-budi (gk_ab12)`, project_id + label `Payments`, action `mcp.read_file`, target_type `file`, target `docs/auth/login.md`, args `{path}`, result `{bytes:4812}`, status ok, IP `203.0.113.5`, UA, trace_id). Baca balik dengan SQL tes (bukan lewat kode produksi): semua kolom cocok; `host(ip) = '203.0.113.5'`; `pg_typeof(ip)::text = 'inet'`; `occurred_at` sama (UTC); `args->>'path'` dan `(result->>'bytes')::int = 4812`.
- `TestT02_AC1_ViaBusCall`: daftarkan handler lewat `Init` pada `bus.NewRegistry` dan DB uji, `bus.CallTyped[record.Response]` dengan request sebagai pada contoh di bawah -> `Recorded:true`, tepat 1 baris.
- `TestT02_AC2_AnonymousEntry`: anonymous label `gk_zz99`, `mcp.auth.rejected`, denied -> `actor_id IS NULL`, `project_id IS NULL`, `project_label IS NULL`, `actor_label = 'gk_zz99'`.
- `TestT02_AC3_NoForeignKeys`: `project_id`/`actor_id` uuid acak yang tidak ada di tabel lain tersimpan sukses; tambahan: `SELECT count(*) FROM pg_constraint WHERE conrelid = 'audit_log'::regclass AND contype = 'f'` = 0.
- `TestT02_AC4_NoSecretsInJSONB`: lewat usecase+repo nyata kirim `Args{"password":"hunter2","token":"tok-abc","content":"ISI-FILE-RAHASIA","path":"a.md"}` dan `Result{"content":"ISI-FILE-RAHASIA"}`; query `SELECT args::text || result::text FROM audit_log` tidak mengandung `hunter2`, `tok-abc`, `ISI-FILE-RAHASIA`; mengandung `[REDACTED]` dan `[OMITTED]`; `args->>'path' = 'a.md'`.
- `TestRecord_EmptyStringsBecomeNull`: target_type/target/user_agent/ip kosong -> kolom `IS NULL`; `trace_id = ''` bila kosong.
- `TestRecord_IPFormats`: `"::1"`, `"203.0.113.5:4431"` (disimpan `203.0.113.5` oleh entity) tersimpan sebagai inet; `"not-an-ip"` -> NULL tanpa error.
- `TestRecord_JSONBDefaultsToEmptyObject`: Args/Result nil -> `args = '{}'`, `result = '{}'`.
- `TestRecord_DuplicateIDError`: dua `Insert` dengan ID sama -> error kedua (PK), dibungkus `audit record:` di tingkat usecase; tidak ada upsert.
- `TestRecord_ContextCanceled`: ctx sudah dibatalkan -> `Insert` mengembalikan error (bukan panic), tidak ada baris.
- `TestRecord_SQLParameterized`: nilai berisi `'); DROP TABLE audit_log;--` di Target/ActorLabel tersimpan literal dan tabel tetap ada.

### Integrasi Publish (`publish_test.go`, tag `integration`)

- `TestPublish_WritesEntry`: registry nyata (`bus.NewRegistry(logger)`), `Init(deps)` dengan DB uji, `reg.Publish(ctx, constants.BUS_CORE_AUDIT_RECORD, record.Request{...})`, `reg.Shutdown(ctx)` (menunggu in-flight) lalu tepat 1 baris tersimpan. Gunakan `Shutdown` atau polling singkat dengan batas waktu, bukan `time.Sleep` buta.
- `TestPublish_InvalidRequestLoggedByBus`: Publish request invalid (Action kosong) dengan logger penangkap -> tidak ada baris, bus me-log `bus publish failed` level error, tidak panic.
- `TestPublish_CanceledParentStillWrites`: ctx pemanggil dibatalkan segera setelah Publish -> entri tetap tertulis (bus memakai `WithoutCancel`, timeout 2 s tetap berlaku).
- `TestPublish_PointerRequestRejected`: Publish `&record.Request{}` -> bus me-log kegagalan `bus.ErrBadRequestType`, tidak ada baris.

### Cakupan kriteria penerimaan

| AC | Tes |
|---|---|
| AC1 | `TestT02_AC1_FullAPIKeyEntryStored`, `TestT02_AC1_ViaBusCall` |
| AC2 | `TestT02_AC2_AnonymousWithKeyPrefixAccepted`, `..._AnonymousWithActorIDRejected`, `TestT02_AC2_AnonymousEntry` |
| AC3 | `TestT02_AC3_NoForeignKeys` |
| AC4 | `TestT02_AC4_PasswordRedacted`, `..._ContentOmitted`, `TestT02_AC4_NoSecretsInJSONB` |
| AC8 (sisi provider) | `TestT02_AC8_RepoErrorReturned`, `TestT02_AC8_RepoTimeout`; bukti end-to-end "MCP tetap sukses" milik T07 |

## E2E Scenarios

Feature ini tidak punya UI atau HTTP; skenario dijalankan di level bus (tes integrasi) dan lewat consumer.

Positif:
1. Consumer T07 memanggil `Call` setelah `read_file` sukses: 1 baris `mcp.read_file` dengan aktor api_key, label `<nama key> (<prefix>)`, project, path, bytes, `ok`, IP. Terlihat di `GET /api/audit-log` oleh super_admin (diverifikasi di plan `listentries`).
2. Key dicabut dipakai: T07 memanggil `Call` dengan anonymous + prefix, `mcp.auth.rejected`, `denied` -> tersimpan, `actor_id` NULL.
3. T03/T04 menulis aksi manusia lewat `Publish` (mis. `project.deleted`) -> baris muncul setelah goroutine bus selesai.
4. Project/key dihapus setelah entri ditulis -> entri tetap ada dengan label snapshot (tanpa FK).
5. Request tanpa `TraceID` dari konteks HTTP ber-trace -> kolom `trace_id` terisi dari ctx.

Negatif:
1. Tipe request salah (`*record.Request`, struct lain) -> error `bus: unexpected request type ...`; tidak ada baris.
2. Request invalid (aksi `"MCP.Read"`, anonymous dengan actor_id, status `failed`) -> `errs.ErrInvalid`; tidak ada baris; pemanggil me-log `audit write failed` dan tetap melayani (fail-open).
3. DB audit mati / koneksi putus -> `Call` mengembalikan `audit record: ...`, `Recorded:false`; panggilan MCP di T07 tetap sukses dan log aplikasi berisi `audit write failed` level error (AC8, dibuktikan di T07).
4. DB lambat melebihi 2 s -> `audit record: context deadline exceeded`; pemanggil tidak tertahan lebih dari sekitar 2 s karena audit.
5. Publish gagal -> bus me-log `bus publish failed`; proses tidak crash.
6. Args berisi `password`, `token`, `content` -> tersimpan tertutup/dihilangkan; tidak ada nilai asli di tabel.
7. ID duplikat dari pemanggil -> error PK, tidak menimpa entri lama (append-only).

## Request/Response body

### Call (Go), sukses

```go
resp, err := bus.CallTyped[record.Response](ctx, d.Bus, constants.BUS_CORE_AUDIT_RECORD, record.Request{
    ActorType:    record.ActorAPIKey,
    ActorID:      "0192f0c4-7a1e-7c3a-9b1d-3c4e5f6a7b8c",
    ActorLabel:   "claude-code-budi (gk_ab12)",
    ProjectID:    "0192f0c4-0000-7000-8000-000000000001",
    ProjectLabel: "Payments",
    Action:       record.ActionMCPReadFile,
    TargetType:   "file",
    Target:       "docs/auth/login.md",
    Args:         map[string]any{"path": "docs/auth/login.md", "api_key": "gk_secret"},
    Result:       map[string]any{"bytes": 4812, "content": "isi file"},
    Status:       record.StatusOK,
    IP:           "203.0.113.5",
    UserAgent:    "claude-code/2.1",
})
// resp == record.Response{Recorded: true}, err == nil
// Tersimpan: args {"path":"docs/auth/login.md","api_key":"[REDACTED]"}, result {"bytes":4812,"content":"[OMITTED]"}
```

### Call, anonymous ditolak key (AC2)

```go
record.Request{
    ActorType:  record.ActorAnonymous,
    ActorLabel: "gk_zz99",
    Action:     record.ActionMCPAuthRejected,
    Status:     record.StatusDenied,
    Result:     map[string]any{"reason": "invalid_key"},
    IP:         "203.0.113.9:51234",
}
// Recorded:true; baris: actor_id NULL, project_id NULL, ip 203.0.113.9
```

### Publish (Go)

```go
if err := d.Bus.Publish(ctx, constants.BUS_CORE_AUDIT_RECORD, record.Request{
    ActorType:    record.ActorUser,
    ActorID:      "0192f0c4-1111-7000-8000-000000000002",
    ActorLabel:   "admin@example.com",
    ProjectID:    "0192f0c4-0000-7000-8000-000000000001",
    ProjectLabel: "Payments",
    Action:       record.ActionProjectDeleted,
    TargetType:   "project",
    Target:       "Payments",
    Status:       record.StatusOK,
}); err != nil {
    log.ErrorContext(ctx, "audit write failed", "action", record.ActionProjectDeleted, "err", err)
}
```

### Kegagalan (nilai `err.Error()` dan perilaku)

| Kasus | `resp` | `err.Error()` / sifat |
|---|---|---|
| Validasi (actor_id pada anonymous) | `record.Response{}` | `INVALID_ARGUMENT: actor_id tidak valid`; `errors.Is(err, errs.ErrInvalid)`; tanpa nilai input |
| Tipe request salah | `nil` | `bus: unexpected request type: want=record.Request got=*record.Request` (membungkus `bus.ErrBadRequestType`) |
| DB error | `record.Response{}` | `audit record: write tcp ...: connection refused` |
| Timeout 2 s | `record.Response{}` | `audit record: context deadline exceeded` |

Pola consumer untuk semua kegagalan (fail-open, tidak ada respons HTTP dari feature ini):

```go
_, err := bus.CallTyped[record.Response](ctx, d.Bus, constants.BUS_CORE_AUDIT_RECORD, req)
if err != nil {
    log.ErrorContext(ctx, "audit write failed", "action", req.Action, "err", err) // jangan log req.Args/req.Result
}
// lanjutkan melayani permintaan
```

Baris tersimpan (representasi kolom) untuk contoh sukses pertama:

```json
{
  "id": "<uuid v7 dari server>",
  "occurred_at": "<waktu server UTC>",
  "actor_type": "api_key",
  "actor_id": "0192f0c4-7a1e-7c3a-9b1d-3c4e5f6a7b8c",
  "actor_label": "claude-code-budi (gk_ab12)",
  "project_id": "0192f0c4-0000-7000-8000-000000000001",
  "project_label": "Payments",
  "action": "mcp.read_file",
  "target_type": "file",
  "target": "docs/auth/login.md",
  "args": { "path": "docs/auth/login.md", "api_key": "[REDACTED]" },
  "status": "ok",
  "result": { "bytes": 4812, "content": "[OMITTED]" },
  "ip": "203.0.113.5",
  "user_agent": "claude-code/2.1",
  "trace_id": "<dari reqctx bila kosong>"
}
```
