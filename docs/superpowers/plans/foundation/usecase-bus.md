# T00 Foundation — Usecase `bus`

Tiket: `docs/tickets/T00-foundation.md` (F2, kriteria T00-AC3). Overview: `docs/superpowers/plans/foundation/overview.md` (bagian 5 adalah sumber kontrak; dokumen ini merinci eksekusinya). Aturan: `rules.txt` bagian 4.4, 4.5, 6.1 sampai 6.9, 13.

Prasyarat: usecase `repo-skeleton-config` selesai (ada `backend/go.mod` module `grimoire`, folder `infra/bus`, `lib/constants`, `lib/contracts`, dan paket `lib/common/reqctx`). Konteks dicek: belum ada `CLAUDE.md` dan `.claude/rules/`.

---

## 1. Metadata

| Atribut | Nilai |
|---|---|
| Ticket | T00 |
| Usecase | `bus` |
| Scope | backend (Go, tanpa frontend, tanpa SQL) |
| Domain | infra |
| Module | foundation (bukan module domain; pola `init/handler/params/usecase/repository` tidak berlaku) |
| Resource root | `backend/internal/infra` |
| Endpoint HTTP | tidak ada |
| Method | tidak ada |
| Version | tidak ada (tanpa prefix versi). Kontrak bus bersifat Go, bukan wire |
| Bergantung pada | `repo-skeleton-config` (hanya `reqctx` untuk test) |
| Dibutuhkan oleh | `http-server` (middleware session), `cli-and-wiring` (`Shutdown`), T01 s.d. T09 |

Endpoint bus yang dimiliki usecase ini sebagai provider: **tidak ada**. Usecase ini hanya menyediakan mesin bus, konstanta, dan kontrak `core.auth.resolve-session`. Provider handler-nya T01, consumer-nya `infra/middleware` (usecase `http-server`).

---

## 2. Summary Plan

Membangun `infra/bus` sebagai satu-satunya jalur interaksi antar module/domain:

- `Registry` peta `endpoint -> Handler`, thread-safe, `Register` panic bila duplikat atau handler nil, urutan registrasi bebas.
- `Call` sinkron memakai ctx pemanggil; endpoint tidak terdaftar menghasilkan error yang membungkus `ErrNotRegistered`.
- `Publish` fire-and-forget di goroutine dengan `context.WithoutCancel` (cancel parent tidak merambat, nilai `reqctx` termasuk `trace_id`, `user_id`, `api_key_id` tetap ada), error di-log, panic di-recover dan di-log dengan stack.
- `Shutdown(ctx)` menolak `Publish` baru lalu menunggu goroutine berjalan sampai selesai atau ctx habis.
- `CallTyped[T]` generik (registry nil = `ErrNilRegistry`, tipe respons salah = `ErrBadResponseType`) dan `Expect[T]` untuk validasi `req` di handler provider (rule 6.9).
- `lib/constants/bus.go` berisi `BUS_CORE_AUTH_RESOLVE_SESSION`; `lib/contracts/core/auth/resolvesession` berisi `Request`/`Response` yang dipakai middleware auth dan nanti diimplementasi T01.

Keputusan desain yang mengikat:

1. `bus` tidak meng-import `domain/**` (rule 4.6) dan tidak meng-import `reqctx` di kode produksi. `context.WithoutCancel` sudah mempertahankan seluruh values, jadi nilai reqctx ikut tanpa kode tambahan. `reqctx` hanya dipakai di test.
2. Keamanan konkurensi `Publish` vs `Shutdown`: `wg.Add` dan flag `closed` dilindungi satu mutex (`life`) supaya tidak terjadi `Add` bersamaan dengan `Wait` pada counter nol (data race pada `sync.WaitGroup`).
3. Panic di handler `Call` tidak di-recover bus; dibiarkan naik ke middleware recover jalur HTTP. Panic di `Publish` selalu di-recover karena tidak ada pemanggil yang menangkap.
4. Log memakai `ErrorContext(ctx2, ...)` supaya handler logger proyek (usecase 1) menambah `trace_id`/`user_id`/`api_key_id` dari ctx. Bus tidak menulis atribut `trace_id` sendiri (hindari kunci ganda).
5. Nama endpoint tidak divalidasi regex di runtime (rule 6.4 ditegakkan lewat review dan satu test pada konstanta). Hanya string kosong yang ditolak (panic) karena pasti salah.
6. `Publish` pada endpoint tidak terdaftar mengembalikan `ErrNotRegistered` segera (bukan diam-diam di goroutine), agar kesalahan wiring terlihat.
7. `Shutdown` idempotent: panggilan kedua tetap menunggu dan mengembalikan hasil yang sama jenisnya.

---

## 3. Plan (langkah berurutan)

### Langkah 1 — Konstanta dan kontrak (tanpa dependensi, bisa dikerjakan pertama)

1. Buat `backend/lib/constants/bus.go`:
   ```go
   package constants

   // BUS_CORE_AUTH_RESOLVE_SESSION
   // Provider: T01 core/auth. Consumer: infra/middleware (session auth).
   const BUS_CORE_AUTH_RESOLVE_SESSION = "core.auth.resolve-session"
   ```
   Hanya import stdlib (tidak ada import sama sekali). Tidak membuat file konstanta lain.
2. Buat `backend/lib/contracts/core/auth/resolvesession/resolvesession.go`:
   ```go
   package resolvesession

   // Request: Token adalah nilai mentah cookie grimoire_session.
   type Request struct {
       Token string
   }

   // Response: sesi tidak valid/kedaluwarsa/user nonaktif = Found false, err nil (rule 6.7).
   type Response struct {
       Found              bool
       UserID             string
       Email              string
       Name               string
       Role               string // RoleSuperAdmin | RoleDeveloper
       MustChangePassword bool
   }

   const (
       RoleSuperAdmin = "super_admin"
       RoleDeveloper  = "developer"
   )
   ```
   Hanya import stdlib; tidak boleh import `internal/**` atau `lib/common/**` (rule 4.5, diperkuat arch test usecase `architecture-guard`).

### Langkah 2 — `bus.go`

File `backend/internal/infra/bus/bus.go`, package `bus`.

```go
type Handler func(ctx context.Context, req any) (any, error)

var (
    ErrNotRegistered   = errors.New("bus: endpoint not registered")
    ErrNilRegistry     = errors.New("bus: nil registry (wiring error)")
    ErrShuttingDown    = errors.New("bus: shutting down")
    ErrBadRequestType  = errors.New("bus: unexpected request type")
    ErrBadResponseType = errors.New("bus: unexpected response type")
)

type Registry struct {
    log      *slog.Logger
    mu       sync.RWMutex       // melindungi handlers
    handlers map[string]Handler
    life     sync.Mutex         // melindungi closed, inflight, dan wg.Add
    closed   bool
    inflight int
    wg       sync.WaitGroup
}

func NewRegistry(log *slog.Logger) *Registry // log nil -> slog.Default()
func (r *Registry) Register(endpoint string, h Handler)
func (r *Registry) Call(ctx context.Context, endpoint string, req any) (any, error)
func (r *Registry) Publish(ctx context.Context, endpoint string, req any) error
func (r *Registry) Shutdown(ctx context.Context) error
```

Perilaku per fungsi (urut implementasi):

1. `Register`: `endpoint == ""` atau `h == nil` -> panic. Lock tulis; bila sudah ada -> `panic(fmt.Sprintf("bus: endpoint %q registered twice", endpoint))`.
2. `lookup(endpoint) (Handler, bool)`: RLock. Dipakai `Call` dan `Publish`.
3. `Call`:
   - `r == nil` -> `nil, ErrNilRegistry`.
   - Tidak ada handler -> `nil, fmt.Errorf("%w: %s", ErrNotRegistered, endpoint)`.
   - Selain itu `return h(ctx, req)` apa adanya (tanpa goroutine, tanpa recover, tanpa timeout tambahan).
4. `Publish`:
   - `r == nil` -> `ErrNilRegistry`.
   - `life.Lock()`; bila `closed` -> unlock, `ErrShuttingDown`.
   - `lookup`; tidak ada -> unlock, `fmt.Errorf("%w: %s", ErrNotRegistered, endpoint)`.
   - `wg.Add(1)`, `inflight++`, `life.Unlock()`.
   - `ctx2 := context.WithoutCancel(ctx)`.
   - `go r.run(ctx2, endpoint, h, req)`; return `nil`.
5. `run` (private):
   ```go
   defer func() {
       if p := recover(); p != nil {
           r.log.ErrorContext(ctx, "bus publish panic", "endpoint", endpoint, "panic", fmt.Sprint(p), "stack", string(debug.Stack()))
       }
       r.life.Lock(); r.inflight--; r.life.Unlock()
       r.wg.Done()
   }()
   if _, err := h(ctx, req); err != nil {
       r.log.ErrorContext(ctx, "bus publish failed", "endpoint", endpoint, "error", err)
   }
   ```
   Urutan defer: recover dan log dulu, lalu `Done` terakhir, sehingga `Shutdown` tidak kembali sebelum log panic tertulis.
6. `Shutdown(ctx)`:
   - `life.Lock(); r.closed = true; n := r.inflight; life.Unlock()`.
   - `done := make(chan struct{}); go func(){ r.wg.Wait(); close(done) }()`.
   - `select { case <-done: return nil; case <-ctx.Done(): ... }`.
   - Pada timeout: baca `inflight` lagi, `r.log.Warn("bus shutdown timed out", "inflight", n)`, return `fmt.Errorf("bus: shutdown with %d publish still running: %w", n, ctx.Err())`. Goroutine penunggu `wg.Wait` dibiarkan selesai sendiri (tidak bocor permanen karena akhirnya selesai saat handler selesai).
   - WAJIB fast path: bila `n == 0` setelah set `closed`, langsung `return nil` sebelum `select`. Alasan: `select` dengan `done` dan `ctx.Done()` sama-sama siap memilih acak, sehingga tanpa fast path `Shutdown` tanpa inflight bisa salah mengembalikan error saat ctx sudah kedaluwarsa (membuat `TestShutdown_Idempotent` flaky).
   - `r == nil` -> `ErrNilRegistry`.

### Langkah 3 — `typed.go`

File `backend/internal/infra/bus/typed.go`:

```go
func CallTyped[T any](ctx context.Context, b *Registry, endpoint string, req any) (T, error) {
    var zero T
    if b == nil { return zero, ErrNilRegistry }
    res, err := b.Call(ctx, endpoint, req)
    if err != nil { return zero, err }          // diteruskan apa adanya
    out, ok := res.(T)
    if !ok { return zero, fmt.Errorf("%w: endpoint=%s want=%T got=%T", ErrBadResponseType, endpoint, zero, res) }
    return out, nil
}

func Expect[T any](req any) (T, error) {
    out, ok := req.(T)
    if !ok { var zero T; return zero, fmt.Errorf("%w: want=%T got=%T", ErrBadRequestType, zero, req) }
    return out, nil
}
```

Catatan: `want=%T` dengan `zero` bertipe interface menghasilkan `<nil>`; untuk T interface gunakan `reflect.TypeFor[T]()` (Go 1.22+, aman karena go.mod >= 1.23) agar pesan memuat nama tipe yang benar. Pakai `reflect.TypeFor[T]()` di kedua fungsi. Respons `nil` dengan err nil dianggap `ErrBadResponseType` (`got=<nil>`). Konvensi provider: handler mengembalikan nilai (`resolvesession.Response{}`), bukan pointer, kecuali kontrak menyatakan lain; consumer memakai tipe yang sama persis.

### Langkah 4 — Test (`bus_test.go`, `typed_test.go`)

Lihat bagian 6. Alat bantu di test: `captureHandler` (implementasi `slog.Handler` yang menyimpan `slog.Record` beserta `reqctx.TraceID(ctx)`/`UserID(ctx)` dari ctx yang diteruskan ke `Handle`, dilindungi mutex), dan `eventually` kecil berbasis channel (bukan `time.Sleep` untuk sinkronisasi; `time.After` hanya sebagai batas gagal 2 detik).

### Langkah 5 — Verifikasi

1. `cd backend && go vet ./internal/infra/bus/... ./lib/...`
2. `cd backend && go test -race -count=3 ./internal/infra/bus/... ./lib/constants/... ./lib/contracts/...`
3. `go list -deps ./internal/infra/bus | grep grimoire` hanya boleh menampilkan paket bus sendiri (kode produksi); di `-test` boleh tambah `lib/common/reqctx`, `lib/contracts/...`.
4. Pastikan `lib/contracts/core/auth/resolvesession` hanya mengimpor stdlib: `go list -f '{{.Imports}}' ./lib/contracts/...`.

### Langkah 6 — Catatan dokumen (bukan file baru milik usecase ini)

Perubahan `docs/tickets/README.md` (catatan bahwa konstanta dan kontrak `core.auth.resolve-session` didefinisikan T00) dimiliki usecase `architecture-guard` (overview bagian 12). Usecase ini TIDAK mengubahnya.

---

## 4. Migration

Tidak ada. Bus tidak menyentuh SQL.

---

## 5. Changes (per file/folder)

Baru:

| File | Isi |
|---|---|
| `backend/internal/infra/bus/bus.go` | `Handler`, sentinel error, `Registry`, `NewRegistry`, `Register`, `Call`, `Publish`, `Shutdown`, `run` privat |
| `backend/internal/infra/bus/typed.go` | `CallTyped[T]`, `Expect[T]` |
| `backend/internal/infra/bus/bus_test.go` | Test registry, Call, Publish, Shutdown |
| `backend/internal/infra/bus/typed_test.go` | Test `CallTyped`, `Expect`, kontrak resolvesession |
| `backend/lib/constants/bus.go` | `BUS_CORE_AUTH_RESOLVE_SESSION` |
| `backend/lib/constants/bus_test.go` | Nama konstanta cocok pola rule 6.4 |
| `backend/lib/contracts/core/auth/resolvesession/resolvesession.go` | `Request`, `Response`, `RoleSuperAdmin`, `RoleDeveloper` |

Tidak diubah: semua file lain (termasuk `app/deps.go`; field `Bus *bus.Registry` ditambahkan di usecase `cli-and-wiring` saat finalisasi `Deps`, atau oleh `http-server` bila perlu lebih awal, sesuai overview 4.4).

API publik yang dikunci (tidak boleh berubah signature oleh usecase lain): semua signature di langkah 2 dan 3.

---

## 6. Test Scenario

Semua test berjalan dengan `-race`. Penamaan mengikuti rule 13.4 (`TestT00_AC3_...` untuk perilaku yang disebut kriteria penerimaan 3). Tidak butuh Postgres.

### bus_test.go

| Test | Skenario |
|---|---|
| `TestT00_AC3_RegisterDuplicatePanics` | Register endpoint sama dua kali; `recover()` menghasilkan pesan berisi `registered twice` dan nama endpoint. |
| `TestRegister_NilHandlerOrEmptyEndpointPanics` | Handler nil panic; endpoint `""` panic. |
| `TestT00_AC3_RegisterOrderIndependent` | Dua registry, endpoint A dan B didaftar urutan terbalik; `Call` tiap endpoint memberi hasil sama di kedua registry. |
| `TestT00_AC3_CallUnregistered` | `errors.Is(err, ErrNotRegistered)` dan pesan memuat nama endpoint; respons nil. |
| `TestCall_PassesThroughResponseAndError` | Handler mengembalikan nilai: diterima persis; handler mengembalikan sentinel error: `errors.Is` lolos tanpa dibungkus ulang. |
| `TestCall_UsesCallerContext` | Ctx dengan `reqctx.WithTraceID` terbaca di handler; cancel ctx terlihat di handler (sinkron, tanpa detach). |
| `TestCall_PanicPropagates` | Panic handler `Call` naik ke pemanggil (bukan di-recover bus). |
| `TestT00_AC3_PublishCarriesContextValues` | Ctx berisi `trace_id`, `user_id`, `api_key_id`; handler menyalin ketiganya ke channel; nilai sama. |
| `TestT00_AC3_PublishSurvivesParentCancel` | Parent `WithCancel`; `Publish`; `cancel()` segera; handler menunggu channel `release`, lalu memeriksa `ctx.Err() == nil` dan `ctx.Done()` nil; selesai normal. |
| `TestT00_AC3_PublishPanicRecovered` | Handler panic("boom"); proses tetap hidup; log capture memuat record level error dengan `endpoint` dan `panic=boom` dan `stack` tidak kosong; `Publish` berikutnya ke endpoint lain tetap dieksekusi. |
| `TestPublish_ErrorLogged` | Handler mengembalikan error; record `bus publish failed` dengan `endpoint`, `error`, dan `trace_id` (dari ctx) tercatat; `Publish` sendiri mengembalikan nil. |
| `TestPublish_UnregisteredReturnsError` | `errors.Is(err, ErrNotRegistered)`, tidak ada goroutine dimulai (`Shutdown` langsung nil). |
| `TestShutdown_WaitsForInflight` | Handler tertahan; `Shutdown` dijalankan di goroutine dengan ctx 2 detik; belum kembali sebelum `release`; kembali nil setelah handler selesai. |
| `TestShutdown_TimesOut` | Handler tertahan lebih lama dari deadline 50ms; `Shutdown` mengembalikan error dengan `errors.Is(err, context.DeadlineExceeded)`; record warn memuat jumlah inflight 1; release di `t.Cleanup`. |
| `TestT00_AC3_PublishAfterShutdownRejected` | Setelah `Shutdown` selesai (atau sedang berjalan), `Publish` -> `ErrShuttingDown`; handler tidak dipanggil. |
| `TestShutdown_Idempotent` | Dua kali `Shutdown` tanpa inflight -> keduanya nil. |
| `TestShutdown_NoInflightReturnsImmediately` | Nol publish -> nil kurang dari 100ms. |
| `TestConcurrent_RegisterCallPublish` | 50 goroutine memanggil `Call` dan `Publish` paralel sambil `Register` endpoint lain; `-race` bersih; hitungan handler Publish sama dengan jumlah Publish sukses. |
| `TestConcurrent_PublishVsShutdown` | Banyak `Publish` bersaing dengan `Shutdown`; tiap `Publish` sukses pasti dieksekusi sebelum `Shutdown` kembali nil; tidak ada panic `WaitGroup misuse`; `-race` bersih. |
| `TestNilRegistry_CallAndPublish` | Receiver nil -> `ErrNilRegistry` (bukan nil-pointer panic). |

### typed_test.go

| Test | Skenario |
|---|---|
| `TestT00_AC3_CallTyped_NilRegistry` | `CallTyped[string](ctx, nil, "x", nil)` -> `ErrNilRegistry`, nilai nol. |
| `TestT00_AC3_CallTyped_WrongResponseType` | Handler mengembalikan `int`, T = `string` -> `errors.Is(err, ErrBadResponseType)`, pesan memuat `want=string got=int`. |
| `TestCallTyped_NilResponse` | Handler mengembalikan `(nil, nil)` -> `ErrBadResponseType`. |
| `TestCallTyped_HandlerErrorPassesThrough` | Error handler lolos `errors.Is`, nilai nol. |
| `TestCallTyped_Unregistered` | `ErrNotRegistered` lolos. |
| `TestCallTyped_ResolveSessionContract` | Provider palsu mengembalikan `resolvesession.Response{Found:true, UserID:"u1", Role:resolvesession.RoleDeveloper}` untuk `constants.BUS_CORE_AUTH_RESOLVE_SESSION`; consumer memakai `CallTyped[resolvesession.Response]` dengan `Request{Token:"t"}` dan menerima nilai sama; provider memakai `Expect[resolvesession.Request]`. |
| `TestExpect_WrongRequestType` | `Expect[resolvesession.Request]("x")` -> `ErrBadRequestType`, pesan memuat `want=resolvesession.Request got=string`. |
| `TestExpect_OK` | Tipe benar -> nilai dan nil. |
| `TestExpect_InterfaceType` | `T` berupa interface: pesan memuat nama interface, bukan `<nil>`. |

### lib/constants/bus_test.go

- `TestBusConstants_NamingRule64`: `BUS_CORE_AUTH_RESOLVE_SESSION` cocok regex `^[a-z]+\.[a-z]+\.[a-z]+(-[a-z]+)*$` dan sama dengan `"core.auth.resolve-session"` (nilai ini kontrak lintas tiket; berubah = break T01).

---

## 7. E2E Scenarios

Bus tidak punya permukaan HTTP sendiri; e2e dijalankan melalui binary saat usecase `http-server` dan `cli-and-wiring` selesai. Skenario di bawah dimiliki usecase tersebut, tetapi bus harus lulus semuanya (dipakai sebagai acceptance checklist saat integrasi).

Positif:
1. `GET /api/<route publik>` tanpa cookie `grimoire_session` -> bus TIDAK dipanggil, request diproses sebagai anonim.
2. Dengan bus palsu terdaftar untuk `core.auth.resolve-session` yang mengembalikan `Found:true`, request ber-cookie mendapat `Identity` di ctx (test integrasi `http-server`, `TestSession_FoundSetsIdentity`).
3. `Publish` dari handler HTTP tetap selesai walau klien memutus koneksi (ctx request dibatalkan) dan `trace_id` request yang sama muncul di log handler `Publish`.
4. SIGTERM saat ada `Publish` berjalan -> `Run` memanggil `bus.Shutdown` dengan `BUS_SHUTDOWN_TIMEOUT` dan menunggu; proses keluar 0 setelah goroutine selesai (`TestServe_GracefulShutdownWaitsInflight` di `cli-and-wiring`).

Negatif:
1. Request dengan cookie `grimoire_session=abc` sebelum T01 (endpoint belum terdaftar) -> middleware menerima `ErrNotRegistered` -> 500 `INTERNAL` pesan generik; detail error hanya di log dengan `trace_id` (gagal tertutup, disengaja).
2. Handler `Publish` melewati `BUS_SHUTDOWN_TIMEOUT` -> `Run` mencatat error shutdown bus dan exit code 1, tetapi urutan shutdown lain (db close) tetap dijalankan.
3. `Publish` setelah shutdown dimulai -> `ErrShuttingDown`; pemanggil (module) hanya mencatat log, tidak crash.
4. Wiring yang lupa membuat registry (`nil`) -> `CallTyped` mengembalikan `ErrNilRegistry`, bukan panic nil-pointer, dan pesan jelas di log.

---

## 8. Request/Response body

### 8.1 Kontrak bus `core.auth.resolve-session` (Go, bukan JSON di wire)

Request:
```go
resolvesession.Request{Token: "k3J9...raw-cookie-value"}
```

Response sesi valid:
```go
resolvesession.Response{
    Found: true, UserID: "0b7c3a52-1d4e-4e0a-9d52-6f8d9a7c1b10",
    Email: "budi@tim.id", Name: "Budi", Role: "developer", MustChangePassword: false,
}
```

Response sesi tidak valid / kedaluwarsa / user nonaktif (bukan error, rule 6.7):
```go
resolvesession.Response{Found: false}
```

Pemakaian consumer (middleware, rule 6.6):
```go
res, err := bus.CallTyped[resolvesession.Response](ctx, b, constants.BUS_CORE_AUTH_RESOLVE_SESSION, resolvesession.Request{Token: v})
```

Pemakaian provider (rule 6.9):
```go
r.Register(constants.BUS_CORE_AUTH_RESOLVE_SESSION, func(ctx context.Context, req any) (any, error) {
    in, err := bus.Expect[resolvesession.Request](req)
    if err != nil { return nil, err }
    // ... usecase.Resolve(ctx, in.Token)
})
```

### 8.2 Contoh error bus (string Go)

```
bus: endpoint not registered: core.auth.resolve-session
bus: unexpected response type: endpoint=core.auth.resolve-session want=resolvesession.Response got=*resolvesession.Response
bus: unexpected request type: want=resolvesession.Request got=string
bus: nil registry (wiring error)
bus: shutting down
bus: shutdown with 1 publish still running: context deadline exceeded
```

### 8.3 Contoh log (JSON)

Publish gagal:
```json
{"time":"2026-10-20T08:00:00Z","level":"ERROR","msg":"bus publish failed","endpoint":"core.audit.record","error":"insert audit: connection refused","trace_id":"0b1c...","user_id":"0b7c..."}
```

Publish panic:
```json
{"time":"2026-10-20T08:00:01Z","level":"ERROR","msg":"bus publish panic","endpoint":"project.apikey.touch","panic":"boom","stack":"goroutine 42 [running]:\n...","trace_id":"0b1c...","api_key_id":"7d2e..."}
```

Shutdown timeout:
```json
{"time":"2026-10-20T08:00:06Z","level":"WARN","msg":"bus shutdown timed out","inflight":1}
```

### 8.4 Format error HTTP yang dihasilkan dari kegagalan bus di middleware

Bus sendiri tidak menulis JSON. Saat middleware session menerima error bus (termasuk `ErrNotRegistered`), `httpx.WriteError` menghasilkan (status 500):

```json
{
  "error": {
    "code": "INTERNAL",
    "message": "terjadi kesalahan internal",
    "details": null
  },
  "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33"
}
```

Detail `bus: endpoint not registered: ...` hanya muncul di log server dengan `trace_id` yang sama, tidak pernah di body.
