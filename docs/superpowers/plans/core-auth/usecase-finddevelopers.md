# T01 core/auth — Usecase `finddevelopers` (bus `core.auth.find-developers`)

Melengkapi `docs/superpowers/plans/core-auth/overview.md` (F1: `base.LikePattern`, `base/store`, `auth.Register`/`buildShared`, konstanta bus, kontrak). Bila overview bertentangan dengan dokumen ini, overview menang. Hanya satu usecase: `finddevelopers` (U14, gelombang C). Jangan tertukar dengan `listusers` (HTTP `GET /api/users`, U9): di sini hanya role `developer` aktif, hasil tiga field, tanpa paginasi/total, dan "tidak ada hasil" adalah daftar kosong, bukan error.

## Metadata

| Field | Nilai |
|---|---|
| Ticket | T01 |
| Scope | fullstack pada tiket; usecase ini backend saja (tidak ada FE) |
| Domain | core |
| Module | auth |
| Feature | `finddevelopers` (gelombang C, U14) |
| Endpoint | bus `core.auth.find-developers` (konstanta `BUS_CORE_AUTH_FIND_DEVELOPERS`); tidak ada endpoint HTTP |
| Method | Call (sinkron, `bus.Call`/`bus.CallTyped`) |
| Version | kontrak baru, v1 (tanpa prefix versi; perubahan breaking = ubah nama endpoint) |
| Consumer | T03 (picker anggota project). Tidak ada consumer di T01 |
| Auth | tidak ada (jalur internal in-process; otorisasi ada di endpoint HTTP consumer) |
| Audit | tidak ada (aksi baca internal) |
| Tabel / migrasi | membaca `users`; tidak ada migrasi |
| Resource root | `backend/internal/domain/core/internal/auth` |
| allowedPaths | `backend/internal/domain/core/internal/auth/finddevelopers/`, `backend/internal/domain/core/internal/auth/base.go` (satu baris `finddevelopers.Init`), `backend/lib/contracts/core/auth/finddevelopers/`, `backend/lib/constants/bus.go` (satu konstanta, bila belum ada dari F1) |

Prasyarat dari F1 (bila belum ada, jangan dibuat ulang di luar allowedPaths; hentikan dan laporkan): `base.LikePattern` (`base/like.go`), `base.Shared`, `auth.Register` + `buildShared`, `base/store.Store` (`store.New(db.Querier)`), tabel `users` beserta indeks `users_role_active_idx`. Kontrak `finddevelopers` dan konstanta `BUS_CORE_AUTH_FIND_DEVELOPERS` didefinisikan F1 langkah 3-4; bila belum ada, buat persis seperti di bawah (file kecil, stdlib saja).

## Summary Plan

`finddevelopers` adalah provider bus yang mencari developer aktif berdasarkan potongan email atau nama: `{Query, Limit}` menjadi `{Items: [{ID, Name, Email}]}`.

Aturan (dari tiket dan overview 5.3, 3.12):
- Hanya `role = 'developer' AND is_active = true`. `super_admin` dan user nonaktif tidak pernah muncul.
- `Query` di-trim. Kosong (atau hanya spasi) menghasilkan `Items` kosong TANPA query DB.
- Pencocokan `ILIKE` pada email ATAU nama, memakai `base.LikePattern(q)` (escape `\`, `%`, `_`, dibungkus `%...%`) dengan `ESCAPE '\'`. Query `%` atau `_` tidak menjadi wildcard. Nilai selalu bind parameter, tidak pernah digabung ke string SQL.
- `Limit <= 0` menjadi 10 (`DefaultLimit`); `Limit > 50` di-clamp ke 50 (`MaxLimit`); bukan error.
- Urutan `name ASC, id ASC` (deterministik).
- `Items` tidak pernah `nil`: kosong adalah `[]finddevelopers.Developer{}`.
- Pembatas defensif: `Query` dipotong maksimal 100 rune sebelum `LikePattern` (nama dibatasi 100 rune, email 254 byte; pola panjang tidak berguna dan hanya menambah beban `ILIKE`). Pemotongan bukan error. Dipotong di batas rune, bukan byte, agar UTF-8 tidak rusak.
- Hanya tiga field yang dikembalikan (`ID`, `Name`, `Email`). Tidak ada `Role`, `IsActive`, hash, atau `must_change_password`.
- Hanya error infra (DB gagal) yang menjadi error, dibungkus `fmt.Errorf("finddevelopers: find: %w", err)`. Consumer tidak boleh menyimpulkan "tidak ada developer" dari kegagalan DB.
- Response dan request dipakai sebagai nilai (bukan pointer): `bus.Expect[finddevelopers.Request](req)` dan handler mengembalikan `finddevelopers.Response`, agar `bus.CallTyped[finddevelopers.Response]` cocok (pola sama dengan `getuserbus`). Tipe request salah menghasilkan error yang membungkus `bus.ErrBadRequestType` (rule 6.9), bukan panic.
- Normalisasi input (trim, clamp limit, potong panjang) adalah sanitasi parameter, bukan rule bisnis domain, sehingga ditaruh sebagai fungsi kecil murni di `usecase.go` (dapat diuji tanpa mock). Rule "siapa yang boleh dicari" (developer aktif) hidup di satu tempat: klausa SQL repository, dikunci test integrasi. Tidak ada method entity baru, tidak ada perubahan `base/`.
- Usecase tidak memakai `base/audit`, hasher, clock, atau transaksi. Satu SELECT tanpa transaksi.
- Query khusus feature ini (`FindActiveDevelopers`) tinggal di `repository.go` milik feature, bukan di `base/store` (overview F1 langkah 8). Tidak ada tabel/kolom/indeks baru. Tanpa indeks `lower(name)` (keputusan D6: puluhan user, seq scan wajar; `users_role_active_idx` sudah mempersempit).
- Tidak ada `params.go` (bukan HTTP). `handler.go` berisi handler bus. Tidak ada route HTTP, tidak ada perubahan OpenAPI.

## Plan

Urutan wajib. Tulis test dulu (TDD), lalu implementasi.

**B0. Verifikasi (tanpa edit).** Pastikan simbol F1 di atas ada. Baca `bus.Registry.Register(endpoint string, h bus.Handler)` (`type Handler func(ctx context.Context, req any) (any, error)`; panic bila endpoint terdaftar dua kali) dan `bus.Expect`. Baca `getuserbus` (U2) bila sudah ada untuk pola `Init` bus dan bentuk repository; ikuti persis. Baca `base/like.go` untuk kontrak `LikePattern` (sudah termasuk pembungkus `%...%`). Cek `d.DB` bertipe yang dipakai feature lain (`db.Querier`/`*db.DB`) dan sesuaikan `NewRepository`.

**B1. Kontrak `backend/lib/contracts/core/auth/finddevelopers/finddevelopers.go`** (bila belum ada dari F1; hanya stdlib, rule 4.5)

```go
// Package finddevelopers adalah kontrak bus core.auth.find-developers.
// Hanya role developer yang aktif. Query kosong -> Items kosong.
// Limit <= 0 -> DefaultLimit; Limit > MaxLimit -> MaxLimit (di-clamp, bukan error).
package finddevelopers

const (
    DefaultLimit = 10
    MaxLimit     = 50
)

type Request struct {
    Query string
    Limit int
}

type Developer struct {
    ID    string
    Name  string
    Email string
}

type Response struct {
    Items []Developer // tidak pernah nil; kosong = []Developer{}
}
```

**B2. Konstanta** `backend/lib/constants/bus.go`: pastikan ada

```go
const BUS_CORE_AUTH_FIND_DEVELOPERS = "core.auth.find-developers"
```

Jangan menduplikasi bila F1 sudah menambahkannya. `lib/constants/bus_test.go` T00 harus tetap lulus.

**B3. `finddevelopers/repository.go`** (satu-satunya yang menyentuh SQL)

```go
package finddevelopers

type Repository struct{ q db.Querier }

func NewRepository(q db.Querier) *Repository { return &Repository{q: q} }

// developerRow adalah row privat ber-tag db; base.User tidak diberi tag db (overview F1 langkah 8).
type developerRow struct {
    ID    string `db:"id"`
    Name  string `db:"name"`
    Email string `db:"email"`
}

// FindActiveDevelopers mengembalikan developer aktif yang email/namanya cocok dengan pattern.
// pattern WAJIB hasil base.LikePattern (sudah di-escape dan dibungkus %). Selalu slice non-nil.
func (r *Repository) FindActiveDevelopers(ctx context.Context, pattern string, limit int) ([]finddevelopers.Developer, error) {
    const q = `
SELECT id, name, email
FROM users
WHERE role = 'developer'
  AND is_active
  AND (email ILIKE $1 ESCAPE '\' OR name ILIKE $1 ESCAPE '\')
ORDER BY name ASC, id ASC
LIMIT $2`
    var rows []developerRow
    if err := r.q.SelectContext(ctx, &rows, q, pattern, limit); err != nil {
        return nil, err
    }
    out := make([]finddevelopers.Developer, 0, len(rows))
    for _, row := range rows {
        out = append(out, finddevelopers.Developer{ID: row.ID, Name: row.Name, Email: row.Email})
    }
    return out, nil
}
```

Catatan: `email` bertipe `citext`; `ILIKE` pada `citext` valid. Gunakan literal backtick untuk SQL agar `'\'` tidak perlu di-escape ganda di Go. Pastikan `standard_conforming_strings` default `on` (default Postgres) sehingga `ESCAPE '\'` berarti satu backslash. Nilai `id` uuid dipindai ke `string` (pola yang sama dengan `store`); bila `store` memakai tipe lain untuk uuid, ikuti pola itu.

**B4. `finddevelopers/usecase.go`** (orkestrasi saja, rule 5.2 dan 5.6)

```go
package finddevelopers

const maxQueryRunes = 100

type developerFinder interface {
    FindActiveDevelopers(ctx context.Context, pattern string, limit int) ([]contract.Developer, error)
}

type Usecase struct{ repo developerFinder }

func NewUsecase(repo developerFinder) *Usecase { return &Usecase{repo: repo} }

func (u *Usecase) Execute(ctx context.Context, in contract.Request) (contract.Response, error) {
    q := normalizeQuery(in.Query)
    if q == "" {
        return contract.Response{Items: []contract.Developer{}}, nil // tanpa query DB
    }
    items, err := u.repo.FindActiveDevelopers(ctx, base.LikePattern(q), normalizeLimit(in.Limit))
    if err != nil {
        return contract.Response{}, fmt.Errorf("finddevelopers: find: %w", err)
    }
    if items == nil {
        items = []contract.Developer{}
    }
    return contract.Response{Items: items}, nil
}

// normalizeQuery: trim, lalu potong ke maxQueryRunes rune (bukan byte).
func normalizeQuery(s string) string { /* strings.TrimSpace; bila utf8.RuneCountInString > 100 -> []rune(s)[:100]; trim lagi */ }

// normalizeLimit: <=0 -> DefaultLimit; > MaxLimit -> MaxLimit.
func normalizeLimit(n int) int { /* ... */ }
```

Alias import kontrak (`contract "grimoire/lib/contracts/core/auth/finddevelopers"`) menghindari bentrok dengan nama package feature `finddevelopers`. Setelah pemotongan, `TrimSpace` dijalankan lagi (pemotongan bisa menyisakan spasi di ujung). Hasil trim kosong berarti jalur tanpa query.

**B5. `finddevelopers/handler.go`** (handler bus)

```go
type Handler struct{ uc *Usecase }

func NewHandler(uc *Usecase) *Handler { return &Handler{uc: uc} }

// Handle adalah bus.Handler untuk core.auth.find-developers.
func (h *Handler) Handle(ctx context.Context, req any) (any, error) {
    in, err := bus.Expect[contract.Request](req)
    if err != nil {
        return nil, err // membungkus bus.ErrBadRequestType
    }
    return h.uc.Execute(ctx, in) // nilai contract.Response, bukan pointer
}
```

Handler tidak menulis log untuk hasil kosong (kasus normal). Query pencarian tidak dilog (bisa berisi potongan email; hindari data pribadi di log).

**B6. `finddevelopers/init.go`**

```go
func Init(d app.Deps, _ base.Shared) {
    h := NewHandler(NewUsecase(NewRepository(d.DB)))
    d.Bus.Register(constants.BUS_CORE_AUTH_FIND_DEVELOPERS, h.Handle)
}
```

Ikuti tipe `d.DB` yang dipakai feature lain. Tidak ada route HTTP.

**B7. Wiring.** Tambah satu baris `finddevelopers.Init(d, shared)` di `auth/base.go` dalam `Register`. Satu baris, tanpa mengubah baris lain (minimal konflik merge).

**B8. Test** (bagian Test Scenario).

**B9. Verifikasi akhir.** `go vet ./...`, archtest `TestT00_AC4_RepoIsClean` (folder `finddevelopers` hanya meng-import `base`, `lib/contracts/...`, `lib/constants`, `infra/app`, `infra/bus`, `infra/db`; tidak meng-import feature lain, rule 4.3), `go test -race ./internal/domain/core/... ./lib/...`, dan test integrasi dengan tag `integration` bila repo memisahkannya.

## Migration

Tidak ada. `finddevelopers` hanya membaca tabel `users` dari `backend/migrations/core/20261021090000000_create_auth_tables.sql` (milik F1). Indeks `users_role_active_idx (role, is_active)` sudah ada; tidak ada indeks tambahan (D6).

## Changes

Backend (baru):
- `backend/internal/domain/core/internal/auth/finddevelopers/init.go`
- `backend/internal/domain/core/internal/auth/finddevelopers/handler.go`
- `backend/internal/domain/core/internal/auth/finddevelopers/usecase.go`
- `backend/internal/domain/core/internal/auth/finddevelopers/repository.go`
- `backend/internal/domain/core/internal/auth/finddevelopers/usecase_test.go`
- `backend/internal/domain/core/internal/auth/finddevelopers/handler_test.go`
- `backend/internal/domain/core/internal/auth/finddevelopers/integration_test.go`
- `backend/lib/contracts/core/auth/finddevelopers/finddevelopers.go` (bila belum dibuat F1)

Backend (ubah, minimal):
- `backend/internal/domain/core/internal/auth/base.go`: satu baris `finddevelopers.Init(d, shared)`.
- `backend/lib/constants/bus.go`: `BUS_CORE_AUTH_FIND_DEVELOPERS` (hanya bila belum ada).

Tidak dibuat: `params.go`, migrasi, route HTTP, perubahan `auth.yml`, kode frontend, event audit, perubahan `base/` atau `base/store`.

Dokumen: katalog `docs/tickets/README.md` sudah memuat baris `core.auth.find-developers` (Call, T01, consumer T03); tidak perlu diubah.

## Test Scenario

Penamaan mengikuti rule 13.4. Usecase ini tidak memiliki AC bernomor di tiket; test memakai nama `Test...` biasa.

### Unit murni (tanpa mock)
- `TestNormalizeLimit` (tabel): `-5`, `0` menjadi 10; `1` menjadi 1; `10` menjadi 10; `50` menjadi 50; `51` dan `1000000` menjadi 50.
- `TestNormalizeQuery` (tabel): `"  sar  "` menjadi `"sar"`; `""`, `"   "`, `"\t\n"` menjadi `""`; 150 rune `a` dipotong ke 100; 150 rune multibyte (mis. `"é"`) dipotong tepat 100 rune dan tetap UTF-8 valid; string yang dipotong tepat sebelum spasi tidak menyisakan spasi ujung.

### Unit usecase (repository fake lewat interface `developerFinder`)
- `TestUsecase_Execute_EmptyQueryNoRepoCall`: `""` dan `"   "` menghasilkan `Items` non-nil dan kosong (`len==0`, `!= nil`); fake mencatat 0 panggilan.
- `TestUsecase_Execute_PassesEscapedPattern`: query `"50%_a\\"` diteruskan ke repo sebagai `base.LikePattern("50%_a\\")` (fake mencatat argumen).
- `TestUsecase_Execute_DefaultLimit`: `Limit: 0` diteruskan sebagai 10.
- `TestUsecase_Execute_ClampLimit`: `Limit: 500` diteruskan sebagai 50; `Limit: 7` tetap 7.
- `TestUsecase_Execute_TrimsQuery`: `" sar "` diteruskan sebagai pola dari `"sar"`.
- `TestUsecase_Execute_MapsItems`: item dari repo diteruskan apa adanya, urutan dipertahankan.
- `TestUsecase_Execute_NilFromRepoBecomesEmptySlice`: repo mengembalikan `(nil, nil)` menghasilkan `Items` non-nil kosong.
- `TestUsecase_Execute_RepoError`: repo gagal menghasilkan `Response{}` dan error yang `errors.Is` ke error asal; bukan daftar kosong.

### Handler (bus nyata `bus.NewRegistry`, usecase memakai repo fake)
- `TestHandler_Handle_ReturnsValueResponse`: lewat `bus.CallTyped[finddevelopers.Response]` berhasil (membuktikan nilai, bukan pointer).
- `TestHandler_Handle_BadRequestType`: `registry.Call(ctx, BUS_CORE_AUTH_FIND_DEVELOPERS, "string-salah")` dan pointer `&finddevelopers.Request{}` menghasilkan `errors.Is(err, bus.ErrBadRequestType)`, tanpa panic.
- `TestHandler_Handle_EmptyResultIsNotError`: hasil kosong, `err == nil`.
- `TestHandler_Handle_PropagatesInfraError`: error infra sampai ke pemanggil.
- `TestInit_RegistersEndpoint`: setelah `Init`, `Call` ke endpoint tidak menghasilkan `bus.ErrNotRegistered`; `Init` dua kali pada registry yang sama panik (perilaku `Register`; didokumentasikan, bukan diubah).

### Integrasi (Postgres nyata `db.TestDB(t)` + migrasi, bus nyata, `finddevelopers.Init`)
Seed lewat `store.InsertUser` / SQL langsung di helper test: developer aktif "Sari Wulandari" `sari@tim.id`, "Andi Saputra" `andi@tim.id`, "Budi Santoso" `budi@tim.id`; developer NONAKTIF "Sari Nonaktif" `sari.off@tim.id`; super_admin aktif "Sari Admin" `sari.admin@tim.id`; developer "50% Diskon" `diskon_50@tim.id`; developer "Zed" `zed@tim.id`.
- `TestFindDevelopers_Integration_OnlyActiveDevelopers`: query `"sari"` hanya mengembalikan "Sari Wulandari"; developer nonaktif dan super_admin tidak ada.
- `TestFindDevelopers_Integration_MatchesEmailAndName`: `"andi"` cocok lewat nama dan email; `"@tim.id"` cocok lewat email; `"SANTOSO"` cocok case-insensitive.
- `TestFindDevelopers_Integration_OrderedByNameThenID`: `"@tim.id"` menghasilkan urutan nama ascending; dua developer bernama sama terurut `id` ascending (seed dua baris nama sama dengan id diketahui).
- `TestFindDevelopers_Integration_DefaultLimit10`: seed 15 developer `dev01..dev15@tim.id`; `Limit: 0` menghasilkan 10 item pertama menurut nama.
- `TestFindDevelopers_Integration_ClampLimit50`: seed 60 developer; `Limit: 1000` menghasilkan tepat 50; `Limit: 3` menghasilkan 3.
- `TestFindDevelopers_Integration_EmptyQuery`: `""` dan `"  "` menghasilkan `Items` kosong non-nil; hitung query lewat wrapper `Querier` penghitung atau DB tertutup: bila `Querier` dibuat gagal total, hasil tetap sukses (membuktikan tidak ada query).
- `TestFindDevelopers_Integration_WildcardEscaped`: `"%"` dan `"_"` tidak cocok dengan semua baris (hasil hanya yang benar-benar mengandung karakter itu: `"%"` mengembalikan "50% Diskon"; `"_"` mengembalikan `diskon_50@tim.id`); `"\\"` tidak error dan tidak mengembalikan semua baris.
- `TestFindDevelopers_Integration_SQLInjectionInert`: query `"'; DROP TABLE users;--"` menghasilkan daftar kosong, tabel `users` masih ada dan berisi baris seed.
- `TestFindDevelopers_Integration_NoMatch`: `"tidak-ada-xyz"` menghasilkan `Items` kosong non-nil, `err == nil`.
- `TestFindDevelopers_Integration_DeactivateExcludesImmediately`: developer ditemukan; `UPDATE users SET is_active=false`; panggilan berikutnya tidak lagi memuatnya (tanpa cache). Setelah U12 ada, ganti pemicu dengan route `deactivate`.
- `TestFindDevelopers_Integration_RoleChangeExcludes`: developer diubah menjadi `super_admin` di DB; tidak lagi muncul.
- `TestFindDevelopers_Integration_NoSensitiveFields`: `reflect` atas `finddevelopers.Developer` hanya memiliki tiga field (`ID`, `Name`, `Email`); `fmt.Sprintf("%+v")` hasil tidak memuat `$argon2id$`.
- `TestFindDevelopers_Integration_LongQueryTruncated`: query 10.000 karakter tidak error dan selesai cepat (hasil kosong).
- `TestFindDevelopers_Integration_Concurrent`: 20 goroutine memanggil paralel; hasil konsisten (jalankan `-race`).
- `TestFindDevelopers_Integration_NoAudit`: handler `core.audit.record` palsu menghitung panggilan; hasilnya 0.

### Tambahan lintas usecase (dimiliki `auth_integration_test.go`, W)
- Baris di tabel AC8/AC9: `finddevelopers` menghasilkan 0 `record.Request` dan tidak ada plaintext/hash di log.
- Pemeriksaan wiring: setelah `core.Register(d)`, `Call` ke `BUS_CORE_AUTH_FIND_DEVELOPERS` tidak mengembalikan `ErrNotRegistered`.

## E2E Scenarios

Endpoint ini tidak punya antarmuka HTTP/UI, sehingga E2E Playwright tidak berlaku langsung. Verifikasinya lewat test integrasi in-process di atas dan, begitu consumer ada (T03: dialog "tambah anggota"), lewat E2E consumer tersebut. Skenario perilaku yang harus tetap benar:

Positif:
1. Consumer mengetik `"sar"`: developer aktif yang nama/emailnya mengandung "sar" muncul, urut nama, maksimal 10.
2. Consumer meminta `Limit: 50`: maksimal 50 hasil; `Limit: 200` tetap 50.
3. Pencarian case-insensitive dan cocok di tengah kata (`"WULAN"` menemukan "Sari Wulandari").
4. Super Admin menonaktifkan developer lewat `setuserstatus`; pencarian berikutnya tidak lagi menampilkannya (dicek di integrasi lintas feature setelah U12 ada).
5. Setelah `updateuser` mengubah nama/email atau role, pencarian berikutnya mengikuti data terbaru.
6. Server menyala dengan semua domain terdaftar (smoke): `Call` ke endpoint ini dari `cmd/internal/server.WithDomains` tidak `ErrNotRegistered`.

Negatif:
1. Query kosong atau spasi: daftar kosong tanpa error (picker tidak menampilkan seluruh user).
2. Tidak ada kecocokan: daftar kosong tanpa error.
3. Query `%`, `_`, atau `\`: diperlakukan harfiah, bukan wildcard; tidak membocorkan seluruh daftar developer.
4. Query berisi SQL (`'; DROP TABLE users;--`): tidak berefek.
5. Super admin dan user nonaktif tidak pernah muncul, apa pun query-nya.
6. Tipe request salah: error `ErrBadRequestType`, bukan panic.
7. DB tidak tersedia: error terbungkus, bukan daftar kosong (consumer tidak salah menyimpulkan tidak ada developer).
8. Respons tidak pernah memuat hash password, role, atau flag `must_change_password`.

## Request/Response body

Bus bersifat in-process (Go), bukan JSON di wire. Tidak ada endpoint HTTP dan tidak ada format error HTTP; error bus dikembalikan sebagai `error` Go.

### Pemanggilan oleh consumer

```go
res, err := bus.CallTyped[finddevelopers.Response](ctx, d.Bus, constants.BUS_CORE_AUTH_FIND_DEVELOPERS,
    finddevelopers.Request{Query: "sar", Limit: 0}) // Limit 0 -> 10
```

Hasil ada kecocokan:
```go
finddevelopers.Response{Items: []finddevelopers.Developer{
    {ID: "7d1e3b52-90aa-4f1c-8b2d-1c3e5a7b9d02", Name: "Sari Wulandari", Email: "sari@tim.id"},
    {ID: "b3c4d5e6-1122-4a3b-9c8d-0f1e2d3c4b5a", Name: "Sarah Putri", Email: "sarah@tim.id"},
}}
```

Query kosong atau spasi (tanpa query DB):
```go
finddevelopers.Request{Query: "   "} // -> finddevelopers.Response{Items: []finddevelopers.Developer{}}
```

Tidak ada kecocokan (`err == nil`):
```go
finddevelopers.Request{Query: "tidak-ada-xyz"} // -> finddevelopers.Response{Items: []finddevelopers.Developer{}}
```

Limit di-clamp:
```go
finddevelopers.Request{Query: "dev", Limit: 500}  // diproses sebagai Limit 50
finddevelopers.Request{Query: "dev", Limit: -1}   // diproses sebagai Limit 10
```

Wildcard dianggap harfiah:
```go
finddevelopers.Request{Query: "50%"} // pola "%50\%%" -> hanya yang mengandung teks "50%"
```

### Error

Tipe request salah:
```go
_, err := d.Bus.Call(ctx, constants.BUS_CORE_AUTH_FIND_DEVELOPERS, "string-salah")
// errors.Is(err, bus.ErrBadRequestType) == true
```

Endpoint belum terdaftar (salah wiring, bukan kasus normal):
```go
// errors.Is(err, bus.ErrNotRegistered) == true
```

Kegagalan infra:
```go
// err = fmt.Errorf("finddevelopers: find: %w", <error DB>); Response zero value (Items nil)
```

Endpoint HTTP lain pada module ini memakai format error seragam T00 `{ "error": { "code", "message", "details" }, "trace_id" }`; tidak berlaku di sini karena tidak ada lapisan HTTP. Consumer T03 yang mengubah error bus menjadi respons HTTP-nya sendiri.

## Definisi selesai

- Semua test di atas hijau (termasuk `-race`); `go vet`, archtest `TestT00_AC4_RepoIsClean` lulus.
- Hanya developer aktif yang ditemukan; query kosong menghasilkan daftar kosong tanpa query DB; limit default 10, maksimal 50; urut `name ASC, id ASC`; wildcard ter-escape.
- Tidak ada perubahan di luar allowedPaths selain satu baris di `auth/base.go`.
