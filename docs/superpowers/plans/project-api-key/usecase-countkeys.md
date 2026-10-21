# T04 project/api-key — Usecase `countkeys` (bus `project.apikey.count-keys`)

Melengkapi `docs/superpowers/plans/project-api-key/overview.md` (F: migrasi `api_keys`, konstanta, `base/store`, kerangka `apikey.Register`) dan memenuhi kontrak consumer-driven T03 `lib/contracts/project/management/contentcount` (T03 overview 5.4). Bila dokumen ini bertentangan dengan overview, overview menang. Hanya satu usecase: `countkeys` (U3, gelombang paralel setelah F). Tidak menyentuh usecase lain.

## Metadata

| Field | Nilai |
|---|---|
| Ticket | T04 |
| Scope | fullstack pada tiket; usecase ini backend saja (tanpa FE, tanpa OpenAPI) |
| Domain | project |
| Module | api-key (`apikey`) |
| Feature | `countkeys` (U3) |
| Endpoint | bus `project.apikey.count-keys` (konstanta `BUS_PROJECT_APIKEY_COUNT_KEYS`, milik T03; tidak ada endpoint HTTP) |
| Method | Call (sinkron, `bus.Call` / `bus.CallTyped`) |
| Version | v1, kontrak `contentcount` milik T03 (perubahan breaking = ubah nama endpoint) |
| Consumer | T03 `listprojects`, `getproject`, `deleteproject` (fail-soft; error -> hitungan `null`) |
| Auth | tidak ada (jalur internal in-process; otorisasi project ada di consumer) |
| Audit | tidak ada |
| Tabel | membaca `api_keys`; tidak ada migrasi baru |
| Resource root | `backend/internal/domain/project/internal/apikey` |
| allowedPaths | `.../apikey/countkeys/`, `.../apikey/base.go` (satu baris `countkeys.Init(d)`), `backend/lib/constants/bus.go` dan `backend/lib/contracts/project/management/contentcount/` hanya bila belum ada dari F/T03 |

Prasyarat (dari F; bila `store.New` / `apikey.Register` belum ada, hentikan dan laporkan): tabel `api_keys` + index `api_keys_project_idx`, `base/store.New(db.Querier)`, `apikey/base.go`, kontrak `contentcount`, konstanta `BUS_PROJECT_APIKEY_COUNT_KEYS`. Bila kontrak/konstanta T03 belum merge, buat PERSIS seperti T03 overview 5.1 dan 5.4 (jangan duplikasi bila sudah ada).

## Summary Plan

`countkeys` memberi T03 jumlah API key AKTIF (`revoked_at IS NULL`) per project, dalam satu Call batch tanpa N+1.

Aturan (overview 5.4, kontrak `contentcount`):
- Request dibaca sebagai nilai lewat `bus.Expect[contentcount.Request]`; pointer/tipe lain -> error membungkus `bus.ErrBadRequestType` (rule 6.9), tanpa panic, tanpa query.
- `len(ProjectIDs) > contentcount.MaxProjectIDs` (100) -> `errs.Wrap(errs.CodeInvalid, "maksimal 100 project id", errs.ErrInvalid)`. Dicek pada panjang input mentah, SEBELUM filter/dedupe, tanpa query.
- Id bukan uuid diabaikan (bukan error). Id valid dinormalisasi ke bentuk kanonik (`uuid.Parse(id).String()`, huruf kecil) dan didedupe sebelum query.
- Setelah filter kosong (input kosong/nil atau semua bukan uuid) -> `Response{Counts: map[string]int{}}` (map non-nil), TANPA query.
- Satu query `GROUP BY project_id`, hanya key aktif, hanya project di daftar (rule 6.10: tidak bocor project lain). Project tanpa key aktif boleh absen dari `Counts` (consumer membaca 0). Key dicabut tidak dihitung.
- Kunci map = `project_id::text` dari DB (kanonik), sehingga consumer yang mengirim bentuk kanonik selalu menemukan entrinya.
- Error DB -> `fmt.Errorf("countkeys: count active: %w", err)` (consumer T03 fail-soft).
- Respons dikembalikan sebagai nilai (`contentcount.Response`), bukan pointer, agar `bus.CallTyped[contentcount.Response]` cocok.
- Usecase hanya orkestrasi (validasi batas, normalisasi id, panggil repository). Tidak ada rule entity baru: tidak ada perubahan di `base/`. Tidak ada transaksi, audit, clock, access, HTTP, `params.go`.
- Pembatasan 100 id membuat array parameter kecil; index `api_keys_project_idx (project_id, created_at DESC)` melayani filter `project_id`.

## Plan

Urutan wajib. Tulis test dulu (TDD), lalu implementasi.

**B0. Verifikasi (tanpa edit).** Pastikan simbol F ada. Baca `backend/internal/infra/bus/bus.go` (`type Handler func(ctx context.Context, req any) (any, error)`; `Register` panik bila endpoint terdaftar dua kali) dan `typed.go` (`Expect`). Cek apakah `grep -rn BUS_PROJECT_APIKEY_COUNT_KEYS backend/lib/constants` sudah ada; bila belum, tambah persis T03 overview 5.1 (satu baris). Cek apakah `lib/contracts/project/management/contentcount` ada; bila belum, buat persis T03 overview 5.4.

**B1. `countkeys/repository.go`** (satu-satunya SQL feature ini)

```go
type Repo struct{ s *store.Store }

func newRepo(s *store.Store) *Repo { return &Repo{s: s} }

type countRow struct {
    ProjectID string `db:"project_id"`
    N         int    `db:"n"`
}

const qCountActive = `
SELECT project_id::text AS project_id, count(*) AS n
FROM api_keys
WHERE project_id = ANY($1::text[]::uuid[]) AND revoked_at IS NULL
GROUP BY project_id`

// CountActiveByProjects: ids sudah kanonik, unik, 1..100 (dijamin usecase). Hanya key aktif.
func (r *Repo) CountActiveByProjects(ctx context.Context, ids []string) (map[string]int, error)
```

- Jalankan lewat `r.s.Querier().SelectContext(ctx, &rows, qCountActive, ids)`. Cast `$1::text[]::uuid[]` sengaja dipakai agar `[]string` selalu ter-encode benar oleh driver pgx stdlib (tanpa `pq.Array`, tanpa library baru) dan filter tetap sargable pada kolom `project_id` bertipe uuid.
- Hasil di-map ke `map[string]int` non-nil (kosong bila tidak ada baris).
- Wajib `ids` non-kosong; repository tidak memvalidasi (tanggung jawab usecase).

**B2. `countkeys/usecase.go`**

```go
type repository interface {
    CountActiveByProjects(ctx context.Context, ids []string) (map[string]int, error)
}

type Usecase struct{ repo repository }

func NewUsecase(repo repository) *Usecase { return &Usecase{repo: repo} }

func (uc *Usecase) Count(ctx context.Context, req contentcount.Request) (contentcount.Response, error)
```

Alur `Count`:
1. `len(req.ProjectIDs) > contentcount.MaxProjectIDs` -> `return contentcount.Response{}, errs.Wrap(errs.CodeInvalid, "maksimal 100 project id", errs.ErrInvalid)`. Cek signature sebenarnya `errs.Wrap` di `backend/lib/common/errs` dan ikuti persis (pola overview 5.4); `errors.Is(err, errs.ErrInvalid)` harus benar.
2. `ids := normalizeIDs(req.ProjectIDs)` (fungsi privat di file ini): untuk tiap input, `u, err := uuid.Parse(s)`; error -> lewati; `k := u.String()`; lewati bila sudah ada di set; pertahankan urutan kemunculan pertama. Hasil `[]string` non-nil.
3. `len(ids) == 0` -> `return contentcount.Response{Counts: map[string]int{}}, nil` tanpa memanggil repo.
4. `counts, err := uc.repo.CountActiveByProjects(ctx, ids)`; error -> `fmt.Errorf("countkeys: count active: %w", err)`.
5. `return contentcount.Response{Counts: counts}, nil` (jaga `counts` non-nil).

Catatan: `uuid.Parse` menerima bentuk `{...}`, `urn:uuid:...`, dan 32 hex tanpa tanda hubung; semuanya dinormalisasi ke kanonik, itu perilaku yang diinginkan (id yang dikirim consumer selalu kanonik).

**B3. `countkeys/handler.go`**

```go
type handler struct{ uc *Usecase }

func newHandler(uc *Usecase) *handler { return &handler{uc: uc} }

// Handle memenuhi bus.Handler.
func (h *handler) Handle(ctx context.Context, req any) (any, error) {
    in, err := bus.Expect[contentcount.Request](req)
    if err != nil { return nil, err }
    return h.uc.Count(ctx, in)   // nilai contentcount.Response, bukan pointer
}
```

Hati-hati: bila `Count` mengembalikan error, handler harus mengembalikan `(nil, err)` (bukan `Response{}` ber-tipe non-nil dalam `any`); tulis eksplisit `res, err := ...; if err != nil { return nil, err }; return res, nil`.

**B4. `countkeys/init.go`**

```go
func Init(d app.Deps) {
    uc := NewUsecase(newRepo(store.New(d.DB)))
    d.Bus.Register(constants.BUS_PROJECT_APIKEY_COUNT_KEYS, newHandler(uc).Handle)
}
```

**B5. Wiring.** Tambah `countkeys.Init(d)` di `backend/internal/domain/project/internal/apikey/base.go` (satu baris, di samping Init feature lain; jangan menyentuh Init lain). Import dari feature lain DILARANG (rule 4.3); feature ini hanya mengimpor `base/store`, kontrak `contentcount`, `constants`, `bus`, `errs`, `app`.

**B6. Verifikasi.** `go vet ./...`; `go test ./internal/domain/project/internal/apikey/countkeys/...`; `go test -tags integration ./internal/domain/project/internal/apikey/...`; archtest `TestT00_AC4_RepoIsClean` lulus (tidak ada import lintas feature/module).

## Migration

Tidak ada. Memakai tabel `api_keys` dan index `api_keys_project_idx` dari migrasi F `backend/migrations/project/<VERSION>_create_api_keys.sql`.

## Changes

- `backend/internal/domain/project/internal/apikey/countkeys/init.go` (baru): `Init(d)` mendaftarkan handler bus.
- `backend/internal/domain/project/internal/apikey/countkeys/handler.go` (baru): `bus.Expect[contentcount.Request]`, delegasi ke usecase.
- `backend/internal/domain/project/internal/apikey/countkeys/usecase.go` (baru): batas 100, normalisasi/dedupe uuid, short-circuit kosong, orkestrasi repo.
- `backend/internal/domain/project/internal/apikey/countkeys/repository.go` (baru): satu query GROUP BY key aktif.
- `backend/internal/domain/project/internal/apikey/countkeys/usecase_test.go`, `handler_test.go`, `repository_integration_test.go` (baru).
- `backend/internal/domain/project/internal/apikey/base.go` (ubah): tambah `countkeys.Init(d)`.
- `backend/lib/constants/bus.go`, `backend/lib/contracts/project/management/contentcount/contentcount.go` (HANYA bila belum ada dari F/T03; isi persis T03).
- Tidak diubah: `base/`, feature lain, migrasi, `api/**`, frontend, `docs/**` (README/tiket sudah ditangani F).

## Test Scenario

Penamaan: `Test<Fungsi>_<Kasus>`; integrasi memakai tag `integration` dan `db.TestDB(t)`.

### Unit usecase (`usecase_test.go`, repo palsu yang merekam argumen dan jumlah panggilan)

- `TestCount_EmptyRequest_NoQuery`: `ProjectIDs` nil dan `[]` -> `Counts` non-nil kosong (`reflect`: bukan nil), repo 0 panggilan.
- `TestCount_AllInvalidIDs_NoQuery`: `["bukan-uuid", "", "123"]` -> Counts kosong, repo 0 panggilan.
- `TestCount_TooManyIDs_Invalid`: 101 id (valid sekalipun) -> `errors.Is(err, errs.ErrInvalid)` benar, `*errs.Error` ber-Code `CodeInvalid`, repo 0 panggilan. Tepat 100 id valid -> sukses (batas inklusif).
- `TestCount_LimitCheckedOnRawLength`: 101 elemen yang semuanya bukan uuid -> tetap error invalid (cek sebelum filter).
- `TestCount_IgnoresInvalid_PassesOnlyValid`: campuran valid + `bukan-uuid` -> repo menerima hanya id valid.
- `TestCount_NormalizesAndDedupes`: huruf besar, `{...}`, dan duplikat -> repo menerima satu id kanonik huruf kecil, urutan kemunculan pertama.
- `TestCount_PassesRepoCountsThrough`: repo mengembalikan `{p1:2}` -> `Response.Counts == {p1:2}`; repo mengembalikan nil map -> Counts non-nil kosong.
- `TestCount_RepoError_Wrapped`: repo error -> `errors.Is(err, sentinel)`, pesan berawalan `countkeys: count active`, bukan `*errs.Error` (infra).

### Unit handler (`handler_test.go`, registry nyata `bus.NewRegistry(log)`)

- `TestHandle_WrongRequestType`: kirim `&contentcount.Request{}` (pointer), `"x"`, `nil`, dan struct tipe lain -> `errors.Is(err, bus.ErrBadRequestType)`, repo 0 panggilan, hasil `nil`.
- `TestHandle_ReturnsValueType`: `bus.CallTyped[contentcount.Response]` berhasil (bukan pointer).
- `TestHandle_ErrorReturnsNilResult`: usecase error -> handler mengembalikan `(nil, err)`.
- `TestInit_RegistersEndpoint`: `Init(d)` dengan registry baru -> `Call(BUS_PROJECT_APIKEY_COUNT_KEYS, contentcount.Request{})` tidak `ErrNotRegistered`; `Init` kedua pada registry yang sama panik (sama dengan semantik `Register`; opsional).

### Integrasi repository + bus (`repository_integration_test.go`, `db.TestDB`)

Fixture: project P1, P2, P3 (`INSERT INTO projects (name) VALUES ($1) RETURNING id`), key via INSERT SQL langsung (hash 32 byte, prefix unik 8 karakter `[a-z0-9]`).

- `TestCountKeys_OnlyActiveCounted`: P1 = 2 aktif + 1 dicabut, P2 = 1 aktif, P3 = 0 -> `Counts == {P1:2, P2:1}` (P3 absen atau 0; assert `Counts[P3] == 0`). Key dicabut tidak dihitung.
- `TestCountKeys_OtherProjectsNotIncluded` (rule 13.6): request hanya `[P1]` -> `Counts` hanya berisi kunci P1 (P2 tidak muncul walau punya key).
- `TestCountKeys_AllRevoked_Absent`: P1 hanya key dicabut -> `Counts[P1]` absen/0.
- `TestCountKeys_InvalidIDsIgnored`: `[P1, "bukan-uuid"]` -> tidak error, `Counts == {P1:2}`.
- `TestCountKeys_UppercaseUUID_KeyedCanonical`: kirim `strings.ToUpper(P1)` -> kunci hasil = P1 kanonik huruf kecil.
- `TestCountKeys_Over100_ErrInvalid`: 101 id -> `errors.Is(err, errs.ErrInvalid)`.
- `TestCountKeys_WrongRequestType_ErrBadRequestType`: lewat `bus.Call` dengan tipe salah -> `errors.Is(err, bus.ErrBadRequestType)`.
- `TestCountKeys_SingleQuery`: bungkus `db.Querier` penghitung (atau `pg_stat_statements` bila tidak memungkinkan, cukup penghitung di repo palsu pada unit test): 100 id valid -> tepat satu query (tidak ada N+1).
- `TestCountKeys_ProjectDeletedCascade`: `DELETE FROM projects WHERE id = P1` -> Count P1 absen/0.
- Parameter array: pastikan 1 id, 100 id, dan id duplikat (setelah dedupe) ter-encode benar oleh driver (menjamin cast `::text[]::uuid[]` bekerja).

Test lintas-usecase `TestT04_CountKeys_OnlyActive` di `apikey_integration_test.go` (W) memakai handler ini lewat bus dengan key yang dibuat `createkey`/`revokekey`; tidak ditulis di usecase ini.

## E2E Scenarios

Tidak ada UI/HTTP milik usecase ini; efeknya terlihat lewat consumer T03 (butuh T01 dan T03 merge).

Positif:
1. Project P berisi 2 key aktif dan 1 dicabut -> `GET /api/projects/{P}` (T03) menampilkan `counts.keys == 2`; setelah satu key dicabut -> 1.
2. Beranda/daftar project (`GET /api/projects`) menampilkan jumlah key per project dari satu Call batch; project tanpa key menampilkan 0.
3. `DELETE /api/projects/{P}` (T03) mencatat audit `project.deleted` dengan `counts.keys` = jumlah key aktif saat itu.

Negatif:
1. Provider belum terdaftar / error DB -> consumer fail-soft: `counts.keys` bernilai `null`, `GET /api/projects/{P}` tetap 200 (diuji di T03; di sini dipastikan error provider berupa error biasa, bukan panic).
2. Consumer mengirim lebih dari 100 id -> provider error invalid; consumer T03 tidak pernah melakukannya (`limit <= 100`), sehingga tidak ada efek HTTP.

## Request/Response body

Tidak ada body HTTP. Kontrak bus Go (bukan JSON di wire):

```go
c, err := bus.CallTyped[contentcount.Response](ctx, d.Bus, constants.BUS_PROJECT_APIKEY_COUNT_KEYS,
    contentcount.Request{ProjectIDs: []string{
        "0192f5a0-1111-7aaa-8bbb-000000000001",
        "0192f5a0-1111-7aaa-8bbb-000000000002",
        "bukan-uuid",
    }})
// P1 punya 2 key aktif + 1 dicabut, P2 tanpa key, "bukan-uuid" diabaikan:
// c == contentcount.Response{Counts: map[string]int{"0192f5a0-1111-7aaa-8bbb-000000000001": 2}}
// (P2 absen; consumer membaca 0)

// Kosong atau semua bukan uuid, tanpa query:
// contentcount.Request{} -> contentcount.Response{Counts: map[string]int{}}

// Lebih dari 100 id:
// err != nil, errors.Is(err, errs.ErrInvalid) == true, pesan "maksimal 100 project id"

// Tipe request salah (pointer / tipe lain):
// err != nil, errors.Is(err, bus.ErrBadRequestType) == true

// Error DB:
// err = "countkeys: count active: <driver error>" (bukan *errs.Error; consumer fail-soft)
```

Format error HTTP seragam T00 tidak berlaku (tidak ada endpoint HTTP). Bila error ini bocor ke HTTP lewat consumer, T03 yang memetakannya (fail-soft menjadi `null`, bukan error ke klien).
