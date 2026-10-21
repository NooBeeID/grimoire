# T04 project/api-key — Usecase `verifykey`

Konteks bersama (migrasi `api_keys`, konstanta bus, kontrak `lib/contracts/project/apikey/verify`, entity dan rule di `base/`, `base/store`) ada di `overview.md` dan dikerjakan di foundation F. Dokumen ini hanya memuat usecase `verifykey`. Bila bertentangan dengan `overview.md`, overview menang. Tiket: `docs/tickets/T04-project-api-key.md`. Keputusan terkait: K6 (tanpa cache), K12 (base64url strict), K14 (format salah tidak menyentuh DB), AC3, AC6, AC8.

## Metadata

| Field | Nilai |
|---|---|
| Ticket | T04 |
| Scope | backend (Go) saja; tidak ada UI, tidak ada route HTTP |
| Domain | project |
| Module | api-key (folder Go `apikey`) |
| Feature | `verifykey` |
| Resource root | `backend/internal/domain/project/internal/apikey` |
| allowedPaths | `backend/internal/domain/project/internal/apikey/verifykey/` |

| Endpoint | Method | Version | Akses |
|---|---|---|---|
| bus `project.apikey.verify` (`constants.BUS_PROJECT_APIKEY_VERIFY`) | Call (satu registrasi) | v1 | internal (consumer T07, fail-closed) |

Tidak ada endpoint HTTP.

## Summary Plan

`verifykey` adalah handler bus yang memverifikasi raw key `gr_<prefix8>_<secret43>` untuk gateway MCP (T07). Alur, tanpa dipecah:

1. `bus.Expect[verify.Request]` (nilai, bukan pointer; tipe salah -> error membungkus `bus.ErrBadRequestType`).
2. `base.ParseRawKey(req.RawKey)`; gagal -> `Response{Found:false, Reason:"malformed"}` tanpa query dan tanpa Prefix (K14).
3. Satu query berindeks `SELECT <KeyColumns> FROM api_keys WHERE prefix = $1` (unique index `api_keys_prefix_key`). Tanpa join, tanpa cek keanggotaan pembuat, tanpa cache, tanpa panggilan bus.
4. `base.VerifyOutcome(key, found, secret)` yang SELALU menghitung SHA-256 dan `subtle.ConstantTimeCompare` (memakai `DummyHash` bila prefix tak dikenal) sebelum memeriksa `revoked`. Urutan keputusan: unknown, mismatch, revoked.
5. Sukses -> `Found=true` + `KeyID`, `KeyName`, `Prefix`, `ProjectID`. Selain itu `Found=false` + `Reason` (`unknown`/`mismatch`/`revoked`) + `Prefix` terisi.
6. Error DB (termasuk ctx dibatalkan) -> dikembalikan sebagai error (fail-closed); `Response{}`.

Tidak mengaudit, tidak me-touch (tugas T07), tidak me-log (jalur panas, dan input adalah kredensial; consumer yang me-log error). Keputusan dan rule ada di entity `base/`; usecase hanya orkestrasi: parse, find, outcome, mapping ke `verify.Response`.

Prasyarat dari foundation F (bila belum ada, hentikan dan selesaikan F; jangan membuatnya dari dalam folder `verifykey/`): migrasi `api_keys`, `constants.BUS_PROJECT_APIKEY_VERIFY`, kontrak `verify.Request/Response` + konstanta `Reason*`, `base.ParseRawKey/VerifyOutcome/DummyHash/Key/Reason*`, `store.Store.GetKey`, `store.ValidUUID` (tidak dipakai di sini).

## Plan

Semua langkah di `backend/internal/domain/project/internal/apikey/verifykey/`. Urutan TDD: tes usecase (RED) -> `usecase.go` -> tes handler -> `handler.go` -> `repository.go` + tes integrasi -> `init.go` -> tes timing.

1. **`usecase.go`** (tulis bersama `usecase_test.go`).
   ```go
   // Repository: sisi consumer (rule 5.6).
   type Repository interface {
       // FindByPrefix: tepat satu query berindeks. Tidak ketemu -> (zero, false, nil).
       FindByPrefix(ctx context.Context, prefix string) (base.Key, bool, error)
   }

   type Usecase struct{ repo Repository }
   func NewUsecase(repo Repository) *Usecase

   func (u *Usecase) Execute(ctx context.Context, in verify.Request) (verify.Response, error)
   ```
   Alur `Execute`:
   1. `prefix, secret, ok := base.ParseRawKey(in.RawKey)`; `!ok` -> `return verify.Response{Found: false, Reason: verify.ReasonMalformed}, nil` (repo TIDAK dipanggil).
   2. `k, found, err := u.repo.FindByPrefix(ctx, prefix)`; `err != nil` -> `return verify.Response{}, fmt.Errorf("verifykey: find by prefix: %w", err)`. Pesan error tidak memuat `RawKey`, secret, maupun prefix.
   3. `ok, reason := base.VerifyOutcome(k, found, secret)`. Dipanggil tanpa syarat, juga saat `!found` (jalur unknown harus membayar hash+compare yang sama; AC8). DILARANG memeriksa `found`/`k.Active()` di usecase sebelum `VerifyOutcome`, DILARANG early return sebelumnya.
   4. `!ok` -> `return verify.Response{Found: false, Prefix: prefix, Reason: reason}, nil`.
   5. `return verify.Response{Found: true, KeyID: k.ID, KeyName: k.Name, Prefix: k.Prefix, ProjectID: k.ProjectID}, nil` (`Reason` kosong).
   - Tidak ada `log.*`, tidak ada cache/map di struct, tidak ada jam (`time.Now`), tidak ada panggilan bus, tidak ada akses `d.Config`. Satu-satunya dependensi: `Repository`.
   - Pilihan alasan untuk Prefix pada unknown: yang diisi adalah prefix hasil parse (input pemanggil), karena key tak ada di DB. Itu satu-satunya sumber Prefix pada `Found=false` non-malformed.

2. **`repository.go`**: satu-satunya file yang menyentuh SQL.
   ```go
   type Repo struct{ s *store.Store }
   func newRepo(s *store.Store) *Repo { return &Repo{s: s} }

   const qFindByPrefix = `SELECT ` + store.KeyColumns + ` FROM api_keys WHERE prefix = $1`

   func (r *Repo) FindByPrefix(ctx context.Context, prefix string) (base.Key, bool, error) {
       return r.s.GetKey(ctx, qFindByPrefix, prefix)
   }
   ```
   - Argumen `$1` parameterized (rule 5.3); tidak ada concat input. Tanpa `FOR UPDATE`, tanpa tx, tanpa join, tanpa filter `revoked_at` di SQL (keputusan revoked ada di entity agar jalur tetap seragam dan alasan `revoked` bisa dilaporkan).
   - Prefix sudah tervalidasi `^[a-z0-9]{8}$` oleh `ParseRawKey` sebelum sampai sini.
   - `GetKey` memetakan `sql.ErrNoRows` ke `(zero, false, nil)`; error lain dikembalikan mentah.
   - Satu query per panggilan; tes memastikan lewat hitungan query (lihat Test Scenario).

3. **`handler.go`**: adapter bus (rule 6.9).
   ```go
   type handler struct{ uc *Usecase }
   func newHandler(uc *Usecase) *handler { return &handler{uc: uc} }

   func (h *handler) Handle(ctx context.Context, req any) (any, error) {
       in, err := bus.Expect[verify.Request](req) // *verify.Request, string, nil -> ErrBadRequestType
       if err != nil { return nil, err }
       res, err := h.uc.Execute(ctx, in)
       if err != nil { return verify.Response{}, err }
       return res, nil
   }
   ```
   Mengembalikan `verify.Response` sebagai NILAI agar `bus.CallTyped[verify.Response]` cocok.

4. **`init.go`**:
   ```go
   func Init(d app.Deps) {
       uc := NewUsecase(newRepo(store.New(d.DB)))
       d.Bus.Register(constants.BUS_PROJECT_APIKEY_VERIFY, newHandler(uc).Handle)
   }
   ```
   Satu `Register` (registrasi ganda = panic, rule 6.1). Tidak ada `params.go` (feature bus, rule 3.2 overview) dan tidak membuat file kosong.

5. **Wiring**: baris `verifykey.Init(d)` di `apikey/base.go` ditulis pada tahap W (atau oleh implementer U1 bila W belum ada; hanya menambah satu baris, tidak mengubah baris feature lain).

6. **Tes** (lihat Test Scenario).

7. **Verifikasi**:
   - `cd backend && go build ./... && go vet ./... && go test ./internal/domain/project/internal/apikey/verifykey/...`
   - Dengan `TEST_DATABASE_URL`: `go test -tags integration ./internal/domain/project/internal/apikey/verifykey/...`
   - Benchmark: `go test -run '^$' -bench . ./internal/domain/project/internal/apikey/verifykey/`
   - Archtest `TestT00_AC4_RepoIsClean` tetap lulus (feature tidak meng-import feature lain).

Kontrak bus (ringkas; definisi lengkap di `overview.md` 5.2):

| Kondisi | Hasil |
|---|---|
| Request bukan `verify.Request` (nilai) | `nil, err` membungkus `bus.ErrBadRequestType` |
| Format salah (panjang, scheme, prefix, base64url tidak kanonik, > 256 byte, kosong) | `Found=false`, `Reason="malformed"`, `Prefix=""`, tanpa query |
| Prefix tidak ada di DB | `Found=false`, `Reason="unknown"`, `Prefix` terisi |
| Prefix ada, secret salah (aktif maupun dicabut) | `Found=false`, `Reason="mismatch"`, `Prefix` terisi |
| Prefix ada, secret benar, key dicabut | `Found=false`, `Reason="revoked"`, `Prefix` terisi |
| Prefix ada, secret benar, aktif | `Found=true`, `KeyID`, `KeyName`, `Prefix`, `ProjectID`, `Reason=""` |
| Error DB / ctx dibatalkan | `verify.Response{}`, `err != nil` (consumer WAJIB fail-closed) |

## Migration

Tidak ada. Tabel `api_keys` dan unique index `api_keys_prefix_key` dibuat oleh migrasi foundation `backend/migrations/project/<VERSION>_create_api_keys.sql`. Query verifikasi memakai index itu. Tes integrasi memakai `db.TestDB(t)` yang sudah menerapkan semua migrasi.

## Changes

| File | Aksi | Isi |
|---|---|---|
| `.../apikey/verifykey/init.go` | buat | rakit repo -> usecase -> `d.Bus.Register(BUS_PROJECT_APIKEY_VERIFY, ...)` |
| `.../apikey/verifykey/handler.go` | buat | `bus.Expect[verify.Request]`, panggil usecase, kembalikan `verify.Response` nilai |
| `.../apikey/verifykey/usecase.go` | buat | interface `Repository`, `Usecase`, `Execute` |
| `.../apikey/verifykey/repository.go` | buat | `Repo.FindByPrefix` (satu SELECT by prefix via `store.GetKey`) |
| `.../apikey/verifykey/usecase_test.go` | buat | unit dengan repo palsu + benchmark + uji statistik CPU |
| `.../apikey/verifykey/handler_test.go` | buat | tipe request salah, nilai respons, propagasi error, registrasi tunggal, kesamaan konstanta Reason |
| `.../apikey/verifykey/repository_test.go` | buat | integrasi (tag `integration`) |
| `.../apikey/verifykey/verify_integration_test.go` | buat | integrasi via bus nyata: skenario AC3/AC6, hitungan query, uji statistik timing AC8 (tag `integration`) |
| `.../apikey/base.go` | ubah (satu baris) | `verifykey.Init(d)` (boleh ditunda ke W) |

Tidak menyentuh file lain di luar folder `verifykey/` selain baris wiring di atas. Tidak mengubah `base/`, kontrak, konstanta, atau OpenAPI (verify tidak punya HTTP).

## Test Scenario

Penamaan: tes yang membuktikan kriteria memakai `TestT04_ACn_*` (rule 13.4); lainnya `Test<Unit>_<Kasus>`. Usecase dites dengan repo palsu (rule 13.2); repository dan alur nyata dengan Postgres (rule 13.3). Rule `VerifyOutcome` sendiri sudah diuji unit di `base/` (foundation); di sini diuji bahwa usecase memakainya dengan benar.

Helper test: `rawKey(t)` memakai `base.GenerateRawKey(rand.Reader)`; `fakeRepo` menyimpan `calls int`, `lastPrefix`, hasil `(Key, found, err)` yang bisa diatur; `keyFor(raw, active bool)` membuat `base.Key` dengan `SecretHash = base.HashSecret(raw.Secret)`, ID/ProjectID/Name uuid dan nama tetap, `RevokedAt` terisi bila `!active`.

### Unit usecase (`usecase_test.go`)

- `TestVerify_ValidActive`: repo mengembalikan key aktif -> `Response{Found:true, KeyID, KeyName, Prefix, ProjectID}` sama persis dengan key, `Reason == ""`; repo dipanggil sekali dengan `lastPrefix == raw.Prefix`.
- `TestT04_AC3_RevokedKeyNotFound`: key dicabut, secret benar -> `Found=false`, `Reason=="revoked"`, `Prefix == raw.Prefix`, `KeyID/KeyName/ProjectID` kosong.
- `TestT04_AC8_WrongSecretIsMismatch`: prefix ada, secret lain yang format-valid -> `Found=false`, `Reason=="mismatch"`, `Prefix` terisi, field identitas kosong.
- `TestVerify_WrongSecretOnRevokedKeyIsMismatch`: key dicabut + secret salah -> `mismatch` (urutan: mismatch sebelum revoked).
- `TestVerify_UnknownPrefix`: repo `found=false` -> `Found=false`, `Reason=="unknown"`, `Prefix == raw.Prefix`.
- `TestVerify_MalformedNeverHitsRepo` (table): `""`, `"abc"`, `"gr_"`, key valid dipotong 1 karakter, key valid ditambah 1 karakter, scheme `GR_...`/`gx_...`, prefix huruf besar, pemisah kedua bukan `_`, secret berisi `=`/`+`/`/`, secret non-kanonik (K12: ubah karakter terakhir menjadi bentuk dengan bit sisa non-nol), string 300 byte, `"gr_"+strings.Repeat("a",8)+"_"+...` salah panjang -> `Found=false`, `Reason=="malformed"`, `Prefix == ""`, `err == nil`, `fakeRepo.calls == 0`.
- `TestVerify_SurroundingWhitespaceAccepted`: `"  " + raw.Full + "\n"` -> sama dengan key valid (sesuai doc kontrak: spasi di tepi diabaikan).
- `TestT04_AC8_FailClosedOnRepoError`: repo mengembalikan `errors.New("conn refused")` -> `Response{}` (Found=false), `err != nil`, `errors.Is(err, sentinel)`, `strings.HasPrefix(err.Error(), "verifykey: ")`; pesan error TIDAK mengandung `raw.Full`, `raw.Secret`, maupun `raw.Prefix`.
- `TestVerify_ContextCanceled`: repo palsu mengembalikan `ctx.Err()` pada ctx batal -> error (bukan `Found=false`).
- `TestVerify_NoSecretInResponse`: untuk semua jalur, marshal `fmt.Sprintf("%+v", res)` tidak mengandung `raw.Secret` dan `raw.Full`.
- `TestVerify_UsecaseHasNoStateOrCache`: refleksi atas `Usecase`: field hanya `repo`; dua `Execute` berurutan dengan repo yang berubah dari aktif ke dicabut -> hasil kedua langsung `revoked` (K6, tanpa cache), repo dipanggil 2 kali.
- `TestVerify_ReasonConstantsMatchBase`: `verify.ReasonMalformed == base.ReasonMalformed`, begitu juga `Unknown`, `Revoked`, `Mismatch` (menjaga kesamaan string antara kontrak dan entity).

### Benchmark dan uji statistik CPU (`usecase_test.go`)

Jalur unknown dan mismatch dengan repo palsu yang mengembalikan instan, sehingga yang terukur hanya kode usecase+entity.

- `BenchmarkVerify_Unknown`, `BenchmarkVerify_Mismatch`, `BenchmarkVerify_Revoked`, `BenchmarkVerify_Valid`: `b.ReportAllocs()`, key dibuat di luar loop. Hanya untuk inspeksi manual dan pemantauan regresi; tidak ada assert.
- `TestT04_AC8_CPUPathUnknownVsMismatchComparable` (skip bila `testing.Short()`): interleave `N=2000` iterasi bergantian (unknown, mismatch, unknown, mismatch, ...) agar drift CPU/GC terbagi rata; ukur tiap panggilan dengan `time.Now()/time.Since`; buang 10% iterasi awal (warm-up); hitung median tiap jalur. Assert longgar: `ratio := median(unknown)/median(mismatch)` berada di `[0.5, 2.0]` ATAU selisih mutlak median < `20µs`. Pesan gagal mencetak kedua median. Jika lingkungan memang berisik, tes memakai `t.Skip` hanya bila median salah satu jalur < 100ns (hash dioptimasi hilang; sinyal bahwa tes tidak bermakna) dan SEKALIGUS test struktural di bawah tetap lulus.
- `TestT04_AC8_StructuralHashBeforeStatusCheck`: struktural, tidak bergantung waktu. Repo palsu menghitung panggilan; untuk empat jalur (unknown, mismatch, revoked, valid) tegaskan bahwa hanya satu panggilan `FindByPrefix` terjadi dan tidak ada jalur yang kembali tanpa melewati `VerifyOutcome`. Cara praktis: `usecase.go` mendeklarasikan `var outcome = base.VerifyOutcome` (variabel paket, tidak diekspor); tes mengganti sementara dengan pembungkus penghitung (pulihkan via `t.Cleanup`) dan menegaskan ia dipanggil tepat sekali pada keempat jalur valid-format, termasuk `found=false`, dan nol kali pada jalur malformed. Tes ini TIDAK paralel (mengubah variabel paket).

### Unit handler (`handler_test.go`)

- `TestHandler_WrongTypeRejected`: request `&verify.Request{}`, `verify.Response{}`, `"gr_..."` (string), `nil`, `touch.Request{}` -> `errors.Is(err, bus.ErrBadRequestType)`; repo tidak dipanggil.
- `TestHandler_ReturnsResponseValue`: sukses -> `any` bertipe `verify.Response` (type assertion nilai, bukan pointer); `bus.CallTyped[verify.Response]` pada registry nyata cocok.
- `TestHandler_PropagatesErrorFailClosed`: usecase gagal -> error diteruskan; `bus.CallTyped` mengembalikan error dan `Response{}` (Found=false).
- `TestInit_SingleRegistration`: `Init` pada `bus.NewRegistry` baru tidak panic dan `Call` menemukan handler (bukan `bus.ErrNotRegistered`); `Init` kedua pada registry yang sama panic. (Memakai `app.Deps` minimal dengan `Bus` dan `DB`; untuk unit tanpa DB, uji `d.Bus.Register(..., newHandler(NewUsecase(fake)).Handle)` dan biarkan `Init` penuh ke tes integrasi.)

### Integrasi repository (`repository_test.go`, tag `integration`, `db.TestDB(t)`)

Fixture SQL (bukan import module T03): `INSERT INTO projects (name) VALUES ($1) RETURNING id`; key disisipkan lewat SQL langsung dengan `secret_hash = base.HashSecret(secret)` dan `prefix` dari `GenerateRawKey`.

- `TestFindByPrefix_Found`: key tersimpan -> `found=true`; `ID`, `ProjectID`, `Name`, `Prefix` sama; `SecretHash` sama dengan `HashSecret(secret)` (32 byte); `RevokedAt == nil`.
- `TestFindByPrefix_NotFound`: prefix acak -> `(zero, false, nil)`, tanpa error.
- `TestFindByPrefix_RevokedKeyStillReturned`: key dicabut tetap dikembalikan dengan `RevokedAt` terisi (UTC) agar entity yang memutuskan `revoked`.
- `TestFindByPrefix_ContextCanceled`: ctx dibatalkan -> error, bukan `found=false`.
- `TestFindByPrefix_UsesPrefixIndex`: `EXPLAIN (FORMAT JSON) SELECT ... WHERE prefix = $1` setelah `SET LOCAL enable_seqscan = off` (tabel kecil) memuat `api_keys_prefix_key` (Index Scan); menjaga bahwa query tetap berindeks tanpa join.
- `TestFindByPrefix_SQLParameterized`: input prefix berisi `' OR '1'='1` langsung ke repository (melewati ParseRawKey) -> `found=false`, tabel utuh.

### Integrasi alur bus (`verify_integration_test.go`, tag `integration`)

Registry nyata `bus.NewRegistry(logger)` + `verifykey.Init(app.Deps{Bus: reg, DB: testDB, ...})`. TIDAK mendaftarkan `check-access`, `get-project`, `core.audit.record`, maupun `core.auth.get-user`: bukti bahwa verify tidak bergantung pada module lain.

- `TestT04_AC3_NewKeyValidThenRevoked`: buat key (SQL fixture) -> `bus.CallTyped[verify.Response]` -> `Found=true`, `KeyID`, `KeyName`, `Prefix`, `ProjectID` benar -> `UPDATE api_keys SET revoked_at = now(), ...` -> panggilan berikutnya langsung `Found=false`, `Reason=="revoked"`, `Prefix` terisi (tanpa jeda, tanpa cache; K6). (Versi lewat endpoint `revokekey` ada di tes W lintas feature.)
- `TestT04_AC6_CreatorRemovedOrDeactivatedKeyStillValid`: key dengan `created_by` uuid acak yang tidak ada di tabel mana pun dan `created_by_label='budi@tim.id'` (pembuat dikeluarkan dari project / dinonaktifkan: tidak ada tabel `users`/`project_members` yang dibaca) -> `Found=true`. Tegaskan lewat hitungan query bahwa tidak ada akses ke tabel selain `api_keys` (lihat tes hitungan query). Tambah varian: `created_by IS NULL` -> tetap `Found=true`.
- `TestVerify_ExactlyOneQuery`: bungkus `db.Querier` dengan penghitung (decorator di tes) dan jalankan sukses, unknown, mismatch, revoked -> tepat 1 query `... FROM api_keys WHERE prefix = $1` per panggilan; jalur malformed -> 0 query. SQL yang tertangkap tidak mengandung `JOIN` dan hanya merujuk `api_keys`.
- `TestVerify_TwoKeysSamePrefixImpossible`: INSERT dua key berprefix sama -> 23505 `api_keys_prefix_key` (pagar DB yang menjamin `WHERE prefix=$1` paling banyak satu baris).
- `TestVerify_PrefixKnownSecretOfOtherKey`: dua key A dan B; raw key = prefix A + secret B -> `mismatch`, bukan `Found=true` untuk salah satunya.
- `TestVerify_CascadeOnProjectDelete`: `DELETE FROM projects WHERE id = P` -> key hilang, `verify` -> `Found=false`, `Reason=="unknown"`.
- `TestVerify_FailClosedOnDBError`: tutup pool/gunakan `Querier` yang mengembalikan error -> `bus.CallTyped` error, `Found=false`; tidak ada panic.
- `TestVerify_NoSecretInLogs`: logger penangkap pada registry; jalankan semua jalur (termasuk error DB) -> keluaran log tidak mengandung `raw.Full` maupun `raw.Secret`.
- `TestT04_AC8_DBPathUnknownVsMismatchComparable` (skip bila `testing.Short()`): fixture satu key aktif. Dua input: A = prefix tak dikenal + secret acak valid-format, B = prefix key + secret salah valid-format. Interleave `N=600` pasang panggilan lewat `reg.Call` (tiap panggilan = satu query berindeks + hash+compare), warm-up 50 pasang dibuang, ukur tiap panggilan, hitung median A dan B. Assert longgar: `ratio` median A/B di `[0.6, 1.7]` ATAU selisih mutlak < `300µs`. Tes ini memotret kebocoran besar (mis. hash dilewati untuk unknown); kebocoran mikrodetik bukan sasarannya. Bila gagal, tes mengulang SATU kali (total 2 percobaan) sebelum `t.Fatalf` untuk menekan flaky di CI; log kedua median pada setiap percobaan. Dokumentasikan di komentar tes bahwa jaminan utama adalah struktural (`TestT04_AC8_StructuralHashBeforeStatusCheck` + tes `VerifyOutcome` di `base/`), dan tes ini hanya sabuk pengaman.
- `TestVerify_MalformedFastPathDocumented`: jalur malformed memang lebih cepat dari jalur DB (tidak dibandingkan); komentar tes merujuk K14 bahwa format key publik sehingga tidak membocorkan rahasia. Tidak ada assert waktu.

### Cakupan kriteria penerimaan

| AC | Tes |
|---|---|
| AC3 (verify bagian) | `TestT04_AC3_RevokedKeyNotFound`, `TestT04_AC3_NewKeyValidThenRevoked`, `TestVerify_UsecaseHasNoStateOrCache` |
| AC6 (verify bagian) | `TestT04_AC6_CreatorRemovedOrDeactivatedKeyStillValid`, `TestVerify_ExactlyOneQuery` |
| AC8 | `TestT04_AC8_WrongSecretIsMismatch`, `TestT04_AC8_StructuralHashBeforeStatusCheck`, `TestT04_AC8_CPUPathUnknownVsMismatchComparable`, `TestT04_AC8_DBPathUnknownVsMismatchComparable`, benchmark |
| Fail-closed | `TestT04_AC8_FailClosedOnRepoError`, `TestVerify_FailClosedOnDBError` |
| Tipe request salah | `TestHandler_WrongTypeRejected` |

## E2E Scenarios

Feature ini tidak punya UI atau HTTP, jadi tidak ada skenario Playwright sendiri. Skenario dijalankan di level bus (tes integrasi di atas) dan lewat consumer T07 setelah T07 tersedia.

Positif:
1. Developer membuat key lewat `createkey`; gateway MCP (T07) memanggil `Call` dengan `Authorization: Bearer gr_...` -> `Found=true` dengan `ProjectID` yang benar; T07 lalu mem-Publish `touch`.
2. Developer mencabut key; permintaan MCP berikutnya (tanpa jeda) -> `Found=false`, `Reason=revoked`, `Prefix` terisi; T07 menolak seragam dan mengaudit `mcp.auth.rejected` dengan prefix + reason.
3. Pembuat key dikeluarkan dari project atau dinonaktifkan -> key tetap `Found=true` sampai dicabut anggota project.
4. Key valid dengan spasi di tepi header -> tetap valid.
5. Project dihapus -> key ikut hilang (CASCADE) -> `Found=false`, `Reason=unknown`.

Negatif:
1. Header berisi string acak/format salah -> `Found=false`, `Reason=malformed`, tanpa query DB.
2. Prefix valid, secret salah -> `Found=false`, `Reason=mismatch`; waktu respons setara dengan prefix tak dikenal.
3. Prefix tak dikenal -> `Found=false`, `Reason=unknown`, `Prefix` terisi untuk audit.
4. Database mati -> `err != nil`; T07 menolak (fail-closed), tidak pernah menganggap key valid.
5. Request bertipe pointer atau struct lain -> error membungkus `bus.ErrBadRequestType`.
6. Consumer meneruskan `Reason` ke klien -> dilarang oleh kontrak; ditegakkan di tes T07 (di luar plan ini).

## Request/Response body

Tidak ada HTTP; kontrak adalah tipe Go yang dikirim lewat `infra/bus` (bukan JSON di wire). Format error bus berupa Go `error`; tidak ada body JSON `{"error":...}` karena tidak ada respons HTTP (consumer T07 yang menerjemahkan ke 401/503 miliknya).

Request dan Response (`backend/lib/contracts/project/apikey/verify`):

```go
type Request struct {
    RawKey string // "gr_<prefix8>_<secret43>"; spasi di tepi diabaikan
}

type Response struct {
    Found     bool
    KeyID     string
    KeyName   string
    Prefix    string
    ProjectID string
    Reason    string // kosong bila Found=true
}
```

Pemanggilan dan hasil (contoh):

```go
raw := "gr_k3x9q2ma_Q2xhdWRlLWNvZGUtYnVkaS1zZWNyZXQtZXhhbXBsZS0xMjM0NTY"
res, err := bus.CallTyped[verify.Response](ctx, d.Bus, constants.BUS_PROJECT_APIKEY_VERIFY, verify.Request{RawKey: raw})

// 1. Valid dan aktif (err == nil)
verify.Response{
    Found: true, KeyID: "0192f5a0-2222-7aaa-8bbb-000000000001", KeyName: "claude-code-budi",
    Prefix: "k3x9q2ma", ProjectID: "0192f5a0-1111-7aaa-8bbb-000000000001",
}

// 2. Dicabut, secret benar (err == nil)
verify.Response{Found: false, Prefix: "k3x9q2ma", Reason: verify.ReasonRevoked}

// 3. Prefix dikenal, secret salah (err == nil)
verify.Response{Found: false, Prefix: "k3x9q2ma", Reason: verify.ReasonMismatch}

// 4. Prefix tidak dikenal (err == nil)
verify.Response{Found: false, Prefix: "zzzzzzzz", Reason: verify.ReasonUnknown}

// 5. Format salah, mis. RawKey "abc" (err == nil; tanpa query DB)
verify.Response{Found: false, Reason: verify.ReasonMalformed}

// 6. Error DB: res == verify.Response{} dan
err = fmt.Errorf("verifykey: find by prefix: %w", dbErr) // consumer WAJIB menolak (fail-closed)

// 7. Tipe request salah: Call(..., &verify.Request{...}) atau Call(..., "gr_...")
err = fmt.Errorf("%w: want=verify.Request got=*verify.Request", bus.ErrBadRequestType)
```

Pola consumer (T07), untuk acuan tes konsumen:

```go
res, err := bus.CallTyped[verify.Response](ctx, b, constants.BUS_PROJECT_APIKEY_VERIFY, verify.Request{RawKey: raw})
if err != nil { /* tolak 401/503, log error; fail-closed */ }
if !res.Found { /* tolak seragam; audit mcp.auth.rejected dengan res.Prefix + res.Reason; jangan teruskan Reason ke klien */ }
// sukses: lanjut dengan res.ProjectID, lalu Publish touch{KeyID: res.KeyID}
```
