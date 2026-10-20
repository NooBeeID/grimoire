# T04 project/api-key — Usecase `touchlastused` (bus `project.apikey.touch`)

Melengkapi `docs/superpowers/plans/project-api-key/overview.md` (F: migrasi `api_keys`, konstanta, kontrak `touch`, `base/`, `base/store`, kerangka `apikey.Register`). Bila dokumen ini bertentangan dengan overview, overview menang. Hanya satu usecase: `touchlastused` (U2, gelombang paralel setelah F). Penerapan K7 (throttle di SQL) dan AC7.

## Metadata

| Field | Nilai |
|---|---|
| Ticket | T04 |
| Scope | fullstack pada tiket; usecase ini backend saja (tidak ada FE, tidak ada OpenAPI) |
| Domain | project |
| Module | api-key (`apikey`) |
| Feature | `touchlastused` (U2) |
| Endpoint | bus `project.apikey.touch` (konstanta `BUS_PROJECT_APIKEY_TOUCH`); tidak ada endpoint HTTP |
| Method | Publish (fire-and-forget, `d.Bus.Publish`). Handler tidak mengembalikan respons |
| Version | kontrak baru, v1 (perubahan breaking = ubah nama endpoint) |
| Consumer | T07 (setelah `verify` sukses) |
| Auth | tidak ada (jalur internal in-process; tidak memeriksa aktor/keanggotaan) |
| Audit | tidak ada |
| Tabel / migrasi | menulis `api_keys.last_used_at`; tidak ada migrasi baru |
| Resource root | `backend/internal/domain/project/internal/apikey` |
| allowedPaths | `backend/internal/domain/project/internal/apikey/touchlastused/`, `backend/internal/domain/project/internal/apikey/base.go` (satu baris `touchlastused.Init(d)`), `backend/lib/contracts/project/apikey/touch/`, `backend/lib/constants/bus.go` (satu konstanta, bila belum ada dari F) |

Prasyarat dari F (bila belum ada, hentikan dan laporkan): migrasi `api_keys`, `base.Clock`, `store.ValidUUID`, `store.New` (+ `Querier()`), kerangka `apikey.Register`. Kontrak `touch` dan konstanta `BUS_PROJECT_APIKEY_TOUCH` didefinisikan F langkah 3-4; bila belum ada, buat persis seperti overview 5.1/5.3.

## Summary Plan

`touchlastused` memperbarui `last_used_at` sebuah key setelah consumer (T07) memverifikasinya. Agar tidak membebani DB, penulisan dibatasi paling sering sekali per menit per key. Pembatasan ditegakkan di dalam statement SQL, bukan di memori proses, sehingga benar pada multi-instance dan tidak ada state yang bocor atau perlu dibersihkan.

Aturan:
- Request dibaca sebagai nilai lewat `bus.Expect[touch.Request]`. Pointer atau tipe lain menghasilkan error yang membungkus `bus.ErrBadRequestType` (rule 6.9). Satu-satunya kondisi yang mengembalikan error ke pemanggil Publish.
- `KeyID` bukan uuid (termasuk kosong): no-op, tanpa query, log debug. Tidak error.
- Satu statement tanpa transaksi:
  `UPDATE api_keys SET last_used_at = $2 WHERE id = $1 AND (last_used_at IS NULL OR last_used_at <= $2 - interval '1 minute')`.
  `$2` = `now` dari `base.Clock` yang diinjeksi (bukan `now()` SQL), sehingga test dapat memajukan waktu. Batas inklusif: tepat 1 menit setelah update sebelumnya sudah boleh menulis lagi.
- Rows affected 1 = tertulis; 0 = di-throttle atau key tidak ada (keduanya normal, log debug saja).
- Key yang sudah dicabut tetap boleh di-touch (tidak berpengaruh pada status; statement tidak memfilter `revoked_at`).
- Clock mundur (instance lain jam lebih tua): `last_used_at` tidak pernah dimundurkan karena kondisi `last_used_at <= $2 - 1 menit` tidak terpenuhi bila `$2` lebih kecil. Tidak butuh penanganan tambahan.
- Error DB: `log.WarnContext(ctx, "apikey: touch last_used_at gagal", "feature", "touchlastused", "key_id", id, "error", err)`, lalu handler mengembalikan `nil, nil`. Tidak dikembalikan ke pemanggil Publish (best-effort; `last_used_at` hanya informasi).
- Tanpa audit, tanpa `access`/`audit` subpackage, tanpa transaksi, tanpa cache, tanpa `params.go`, tanpa HTTP.
- Log hanya memuat `key_id` (tidak ada secret, memang tidak tersedia di request).
- Usecase hanya orkestrasi: validasi uuid + clock + panggil repository + keputusan log. Tidak ada rule entity baru (aturan 1 menit adalah kebijakan penulisan di SQL, konstanta `ThrottleInterval` didokumentasikan di repository).

## Plan

Urutan wajib. Tulis test dulu (TDD), lalu implementasi.

**B0. Verifikasi (tanpa edit).** Pastikan simbol F ada. Baca `backend/internal/infra/bus/bus.go` (`Registry.Register(endpoint string, h Handler)`; `type Handler func(ctx context.Context, req any) (any, error)`; panic bila endpoint terdaftar dua kali; `Publish` memanggil handler dan mencatat error handler sebagai "bus publish failed") dan `typed.go` (`Expect`). Ikuti pola `Init` bus pada overview bagian 7.

**B1. Kontrak** `backend/lib/contracts/project/apikey/touch/touch.go` (bila belum ada dari F; stdlib saja) persis overview 5.3:

```go
package touch

type Request struct {
	KeyID string
}
```

**B2. Konstanta** `BUS_PROJECT_APIKEY_TOUCH = "project.apikey.touch"` di `backend/lib/constants/bus.go` (bila belum ada dari F).

**B3. Test (merah dulu)**: `usecase_test.go`, `handler_test.go`, `repository_integration_test.go` (bagian Test Scenario).

**B4. `repository.go`** (satu-satunya yang menyentuh SQL):

```go
package touchlastused

// ThrottleInterval didokumentasikan di sini; nilai 1 menit ditulis literal di SQL (K7, tiket T04).
const touchSQL = `UPDATE api_keys SET last_used_at = $2
WHERE id = $1 AND (last_used_at IS NULL OR last_used_at <= $2::timestamptz - interval '1 minute')`

type Repo struct{ q db.Querier }

func newRepo(s *store.Store) *Repo { return &Repo{q: s.Querier()} }

// Touch mengembalikan jumlah baris yang diubah (1 = tertulis, 0 = di-throttle / key tidak ada).
func (r *Repo) Touch(ctx context.Context, id string, now time.Time) (int64, error) {
	res, err := r.q.ExecContext(ctx, touchSQL, id, now.UTC())
	if err != nil {
		return 0, fmt.Errorf("touchlastused: update: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("touchlastused: rows affected: %w", err)
	}
	return n, nil
}
```

Catatan: cast `$2::timestamptz` wajib agar pgx menentukan tipe parameter yang dipakai dua kali (SET dan WHERE).

**B5. `usecase.go`**:

```go
type Repository interface {
	Touch(ctx context.Context, id string, now time.Time) (int64, error)
}

type Usecase struct {
	repo  Repository
	clock base.Clock
	log   *slog.Logger
}

func NewUsecase(repo Repository, clock base.Clock, log *slog.Logger) *Usecase

// Touch tidak pernah mengembalikan error: kegagalan DB hanya di-log (warn).
func (uc *Usecase) Touch(ctx context.Context, req touch.Request) {
	if !store.ValidUUID(req.KeyID) {
		uc.log.DebugContext(ctx, "apikey: touch diabaikan, key_id bukan uuid", "feature", "touchlastused")
		return
	}
	n, err := uc.repo.Touch(ctx, req.KeyID, uc.clock())
	if err != nil {
		uc.log.WarnContext(ctx, "apikey: touch last_used_at gagal", "feature", "touchlastused", "key_id", req.KeyID, "error", err)
		return
	}
	uc.log.DebugContext(ctx, "apikey: touch", "feature", "touchlastused", "key_id", req.KeyID, "written", n == 1)
}
```

Catatan: `ValidUUID` bergantung pada `store` (arah import `touchlastused -> base/store`, diizinkan). Bila ingin usecase bebas dari `store`, cek uuid boleh dipindah ke handler; pilih salah satu dan konsisten, default: usecase.

**B6. `handler.go`**:

```go
type Handler struct{ uc *Usecase }

func newHandler(uc *Usecase) *Handler { return &Handler{uc: uc} }

func (h *Handler) Handle(ctx context.Context, req any) (any, error) {
	r, err := bus.Expect[touch.Request](req)
	if err != nil {
		return nil, err
	}
	h.uc.Touch(ctx, r)
	return nil, nil
}
```

**B7. `init.go`**:

```go
func Init(d app.Deps) {
	clock := base.Clock(func() time.Time { return time.Now().UTC() })
	uc := NewUsecase(newRepo(store.New(d.DB)), clock, d.Log)
	d.Bus.Register(constants.BUS_PROJECT_APIKEY_TOUCH, newHandler(uc).Handle)
}
```

**B8. Wiring**: tambah satu baris `touchlastused.Init(d)` di `backend/internal/domain/project/internal/apikey/base.go` (bersama Init feature lain, tanpa mengubah baris milik usecase lain).

**B9. Verifikasi**: `go vet ./...`, `go test ./internal/domain/project/internal/apikey/touchlastused/...`, `go test -tags integration ./internal/domain/project/internal/apikey/touchlastused/...`, archtest `TestT00_AC4_RepoIsClean`.

## Migration

Tidak ada. Memakai tabel `api_keys` dari `backend/migrations/project/20261023090000000_create_api_keys.sql` (kolom `last_used_at timestamptz NULL`, PK `id` melayani lookup). Tidak butuh index tambahan.

## Changes

- `backend/internal/domain/project/internal/apikey/touchlastused/init.go` (baru): `Init(d)` mendaftarkan handler Publish.
- `.../touchlastused/handler.go` (baru): `bus.Expect[touch.Request]`, delegasi ke usecase, selalu `nil, nil` bila tipe benar.
- `.../touchlastused/usecase.go` (baru): cek uuid, clock, panggil repository, log.
- `.../touchlastused/repository.go` (baru): satu UPDATE throttle, rows affected.
- `.../touchlastused/usecase_test.go`, `handler_test.go`, `repository_integration_test.go` (baru).
- `backend/internal/domain/project/internal/apikey/base.go` (ubah): tambah `touchlastused.Init(d)`.
- `backend/lib/contracts/project/apikey/touch/touch.go`, `backend/lib/constants/bus.go` (hanya bila belum ada dari F).
- Tidak diubah: `api/**`, migrasi, `base/` (kecuali bila `Clock`/`ValidUUID` belum ada dari F), frontend, config.

## Test Scenario

Penamaan: `TestT04_AC7_...` untuk AC; lainnya `Test<Fungsi>_<Kasus>`.

**Unit usecase (`usecase_test.go`, repo palsu + clock tetap + logger penangkap):**
- uuid valid, repo mengembalikan (1, nil): repo dipanggil sekali dengan `id` dan `now` dari clock (UTC); tidak ada log warn.
- uuid valid, repo (0, nil): tidak ada log warn, tidak ada panic.
- `KeyID` `""`, `"bukan-uuid"`, `"123"`, `"' OR 1=1 --"`: repo TIDAK dipanggil; log debug; tanpa warn.
- repo mengembalikan error: tepat satu log level WARN memuat `key_id` dan `error`; `Touch` tidak panic dan tidak mengembalikan apa pun.
- Log tidak memuat string selain field yang dirancang (tidak ada raw key/secret; request memang tidak membawanya).

**Unit handler (`handler_test.go`, bus nyata `bus.NewRegistry(log)`, usecase dengan repo palsu):**
- `Publish(ctx, BUS_PROJECT_APIKEY_TOUCH, touch.Request{KeyID: uuid})` -> `nil`, repo dipanggil sekali.
- Request pointer `&touch.Request{...}` -> error `errors.Is(err, bus.ErrBadRequestType)`, repo tidak dipanggil.
- Request `string`/`nil`/struct lain -> `ErrBadRequestType`.
- Repo error -> `Publish` mengembalikan `nil` (tidak diteruskan).
- Handler terdaftar di endpoint yang benar: `Publish` setelah `Init` tidak mengembalikan `bus.ErrNotRegistered`; `Init` dua kali pada registry sama -> panic (duplikat; didokumentasikan perilaku bus, sekadar sanity).

**Integrasi repository + usecase (`repository_integration_test.go`, tag `integration`, `db.TestDB(t)`):**
Fixture: `INSERT INTO projects (name) VALUES ('P') RETURNING id`, lalu `INSERT INTO api_keys (project_id, name, prefix, secret_hash) VALUES ($1,'k','abcd1234', decode(repeat('00',32),'hex')) RETURNING id` (data fixture lewat SQL, bukan import module lain). Clock uji berupa variabel yang bisa dimajukan.
- `TestT04_AC7_TouchOncePerMinute`: `last_used_at` awal NULL. Touch pertama pada T0 -> rows affected 1 dan `last_used_at == T0`. Touch ke-2..5 pada T0+10s, +30s, +59s, +59.9s -> rows affected 0 dan `last_used_at` tetap T0. Maju ke T0+60s (batas inklusif) -> rows 1, `last_used_at == T0+60s`. Maju T0+61s lalu T0+121s (> 1 menit dari update terakhir) -> ter-update lagi sesuai aturan (121s: rows 1; 61s setelah 60s: rows 0 karena hanya 1 detik).
- `TestTouch_ConcurrentSingleWinner`: 20 goroutine men-touch key yang sama dengan `now` sama (key `last_used_at` NULL) -> jumlah rows affected total tepat 1.
- `TestTouch_ClockBackwardsNeverRegresses`: setelah update di T0, touch dengan now = T0-5 menit -> rows 0, `last_used_at` tetap T0.
- `TestTouch_UnknownKeyIDNoOp`: uuid acak yang tidak ada -> rows 0, error nil; lewat usecase tidak ada log warn; tabel tidak berubah.
- `TestTouch_NonUUIDNoQuery`: lewat bus `Publish` dengan `KeyID: "bukan-uuid"` -> nil, tidak ada error dan tidak ada baris berubah (tidak ada error cast uuid dari Postgres karena query tidak dijalankan).
- `TestTouch_RevokedKeyStillTouchable`: key dengan `revoked_at` terisi -> touch menulis `last_used_at` (rows 1); `revoked_at`, `revoked_by` tidak berubah.
- `TestTouch_OnlyTargetRowChanged`: dua key; touch key A tidak mengubah `last_used_at` key B.
- `TestTouch_DBErrorSwallowed`: gunakan `db.TestDB` lalu tutup koneksi (`d.Close()`) atau context yang sudah dibatalkan; `Publish` mengembalikan nil, log warn tertangkap.
- `TestTouch_NoSecretInLogs`: semua output log selama test tidak memuat string `gr_`.
- Lintas module (dicakup juga di `apikey_integration_test.go` W oleh `TestT04_AC7_TouchWritesOncePerMinute`): setelah touch, `GET /api/projects/{id}/api-keys` menampilkan `last_used_at`.

## E2E Scenarios

Usecase ini tidak punya UI. Efeknya terlihat di kolom "terakhir dipakai" tab API Keys (dicakup E2E T04 dan T07). Dari sisi touch:

Positif:
1. Setelah agent MCP memakai key (T07 memverifikasi lalu Publish touch), buka tab "API Keys": kolom "terakhir dipakai" berubah dari "-" menjadi waktu relatif (pada E2E T07, bukan T04, karena butuh gateway).
2. Beberapa panggilan MCP beruntun dalam semenit: nilai "terakhir dipakai" tidak berubah di dalam menit itu (diverifikasi lewat API list, `last_used_at` identik); panggilan setelah > 1 menit memperbarui nilai.

Negatif:
1. Key yang dicabut lalu dipakai agent: `verify` menolak (T07), tidak ada touch; bila touch tetap dipublikasikan, status tetap "dicabut" dan tidak ada error di UI.
2. Publish touch dengan KeyID tidak valid/unknown tidak menimbulkan error 5xx atau efek apa pun pada API (diperiksa lewat test integrasi karena bus tidak terbuka ke HTTP).

## Request/Response body

Tidak ada HTTP. Kontrak bus (Go, bukan JSON di wire):

```go
// Consumer (T07), setelah verify sukses:
err := d.Bus.Publish(ctx, constants.BUS_PROJECT_APIKEY_TOUCH,
	touch.Request{KeyID: "0192f5a0-2222-7aaa-8bbb-000000000001"})
// err == nil pada semua kasus normal, termasuk throttled, key tidak ada, KeyID bukan uuid, dan error DB.

// Tipe salah:
err = d.Bus.Publish(ctx, constants.BUS_PROJECT_APIKEY_TOUCH, &touch.Request{KeyID: "x"})
// errors.Is(err, bus.ErrBadRequestType) == true
// (bus mencatatnya sebagai "bus publish failed"; tidak ada respons)
```

Contoh SQL efektif (diparameterkan):

```sql
UPDATE api_keys SET last_used_at = '2026-10-23T11:42:00Z'
WHERE id = '0192f5a0-2222-7aaa-8bbb-000000000001'
  AND (last_used_at IS NULL OR last_used_at <= '2026-10-23T11:42:00Z'::timestamptz - interval '1 minute');
-- rows affected: 1 (tertulis) | 0 (di-throttle / key tidak ada)
```

Log (tidak ada body error ke klien karena tidak ada HTTP):

```
level=WARN msg="apikey: touch last_used_at gagal" feature=touchlastused key_id=0192f5a0-2222-7aaa-8bbb-000000000001 error="touchlastused: update: ..."
level=DEBUG msg="apikey: touch diabaikan, key_id bukan uuid" feature=touchlastused
```

Format error HTTP seragam T00 tidak berlaku untuk usecase ini karena tidak ada endpoint HTTP; satu-satunya error yang keluar dari handler adalah `fmt.Errorf("bus: unexpected request type: want=touch.Request got=%T", req)` yang membungkus `bus.ErrBadRequestType`.
