# T12 management-countmemberships — Plan Usecase

Tiket: `docs/tickets/T12-core-invitation-email.md`. Foundation: `docs/superpowers/plans/core-invitation-email/overview.md` (menang bila bertentangan).

Usecase bus-only milik project/management: menghitung jumlah keanggotaan project per user. Konsumen tunggal: job `core.auth.purge-invitations` (core/auth), yang tidak boleh membaca `project_members` karena `project_members.user_id` tidak punya FK ke `users` dan tabel itu milik module lain.

---

## 1. Metadata

| Atribut | Nilai |
|---|---|
| Ticket | T12 |
| Feature | core-invitation-email |
| Scope | backend saja (tanpa HTTP, tanpa frontend) |
| Domain | project |
| Module | management |
| Usecase slug | `management-countmemberships` |
| Folder | `backend/internal/domain/project/internal/management/countmemberships` |
| Endpoint HTTP | Tidak ada |
| Method | - |
| Version | - (kontrak bus baru; tanpa prefix versi) |
| Endpoint bus | `project.management.count-memberships` (Call) |
| Konstanta | `constants.BUS_PROJECT_MANAGEMENT_COUNT_MEMBERSHIPS` (ditambah foundation 4.7) |
| Provider | project/management `countmemberships` |
| Consumer | core/auth `purgeinvitations` |
| Kontrak | `backend/lib/contracts/project/management/countmemberships` (ditambah foundation 4.7) |
| Gelombang | A (paralel, tidak bergantung usecase lain; hanya butuh foundation) |

---

## 2. Summary Plan

Handler bus read-only. Menerima `Request{UserIDs []string}`, mengembalikan `Response{Counts map[string]int}` berisi jumlah baris `project_members` per user dalam satu query `GROUP BY`.

Keputusan:

1. Pola `checkaccess` untuk struktur (`init.go`, `handler.go`, `usecase.go`, `repository.go`, tanpa route HTTP) dan pola `apikey/countkeys` untuk isi (normalisasi id, batas, satu query, repository menerima `[]string` kanonik). Kedua pola sudah ada di repo.
2. Batas `MaxUserIDs = 500`. Lebih dari itu -> error membungkus `errs.ErrInvalid` dengan kode `INVALID_ARGUMENT` (`errs.Wrap(errs.CodeInvalid, ..., errs.ErrInvalid)`). Batas dicek terhadap panjang slice mentah SEBELUM normalisasi/dedupe, supaya perilaku deterministik dan murah.
3. Id bukan uuid diabaikan (tanpa error). Id dikanonikkan (`uuid.Parse(...).String()`, lowercase) dan didedupe. Slice kosong, nil, atau semua id tidak valid -> `Counts` kosong (map non-nil, `{}`), TANPA menyentuh DB.
4. User tanpa keanggotaan TIDAK muncul di `Counts`; konsumen membaca nilai 0 dari map (`counts[id]`). Hanya key hasil kanonik yang muncul; konsumen wajib mencari dengan id kanonik (purge memakai id dari tabel `users` yang sudah kanonik).
5. Tanpa cek akses user dan tanpa audit: dipanggil job sistem tanpa identitas `reqctx`, dan read-only (README: audit hanya untuk mutasi). Tidak ada pemanggilan bus keluar sama sekali (tidak memakai `base/users`), jadi tidak ada risiko dependensi melingkar auth <-> management.
6. Hitungan lintas SEMUA project (tidak difilter per project) karena tujuannya mengetahui apakah user masih punya keanggotaan di project mana pun. Tidak memfilter `is_active` user (tabel tidak punya kolom itu, dan itu data auth).
7. SQL tunggal milik feature ini, jadi diletakkan di `repository.go` feature (bukan `base/store`; rule 5.3: SQL ke `base/store` hanya bila dipakai >1 feature). Repository menerima `db.Querier` langsung (`d.DB`), sehingga `base/store/store.go` management TIDAK perlu diubah (store management belum punya method `Querier()`; tidak menambahnya agar tidak menyentuh file bersama).
8. Rule bisnis: tidak ada entity baru. Normalisasi/batas id adalah validasi argumen kontrak, bukan rule domain, jadi tinggal di `usecase.go` sebagai fungsi kecil (sama seperti `countkeys.normalizeIDs`). Usecase tidak dipecah.
9. Error DB dibungkus `fmt.Errorf("countmemberships: count: %w", err)` (jadi 500/internal di konsumen; konsumen purge memutuskan fail-safe: tidak menghapus apa pun bila error).
10. Indeks: `project_members` dikunci primary key `(project_id, user_id)`; query berdasarkan `user_id` saja butuh indeks `user_id`. Cek saat implement apakah `20261022090000000_create_projects.sql` sudah punya indeks pada `user_id` (kemungkinan ada untuk `listprojects`). Bila TIDAK ada, JANGAN menambah migrasi diam-diam: seq scan pada job harian 03:30 dengan tabel kecil diterima, dan catat di PR. Lihat bagian 4 (Migration).

---

## 3. Plan (langkah berurutan)

Prasyarat (foundation 4.7, sudah ada sebelum usecase ini): konstanta bus `BUS_PROJECT_MANAGEMENT_COUNT_MEMBERSHIPS` di `backend/lib/constants/bus.go` dan paket kontrak `backend/lib/contracts/project/management/countmemberships/countmemberships.go`:

```go
// Package countmemberships adalah kontrak bus project.management.count-memberships.
// Provider: project/management countmemberships. Consumer: core/auth purgeinvitations.
//
// Pola consumer:
//
//	res, err := bus.CallTyped[countmemberships.Response](ctx, b,
//		constants.BUS_PROJECT_MANAGEMENT_COUNT_MEMBERSHIPS,
//		countmemberships.Request{UserIDs: ids})
//	if err != nil { return err }          // fail-safe: jangan hapus apa pun
//	n := res.Counts[id]                   // absen = 0
//
// Request dikirim sebagai nilai (bukan pointer). len(UserIDs) > MaxUserIDs -> error
// membungkus errs.ErrInvalid (INVALID_ARGUMENT). Id bukan uuid diabaikan.
package countmemberships

const MaxUserIDs = 500

type Request struct{ UserIDs []string }

// Counts: user tanpa keanggotaan absen (dibaca sebagai 0). Selalu non-nil.
type Response struct{ Counts map[string]int }
```

Jika ternyata belum ada (foundation belum merge), buat persis seperti di atas; jangan mengubah bentuknya.

Langkah implementasi (TDD: tes dulu per langkah):

1. **`repository.go`** (`backend/internal/domain/project/internal/management/countmemberships/repository.go`)

   ```go
   package countmemberships

   import (
       "context"

       "grimoire/internal/infra/db"
   )

   // Repo adalah akses SQL untuk feature countmemberships.
   type Repo struct{ q db.Querier }

   func newRepo(q db.Querier) *Repo { return &Repo{q: q} }

   type countRow struct {
       UserID string `db:"user_id"`
       N      int    `db:"n"`
   }

   const qCountByUsers = `
   SELECT user_id::text AS user_id, count(*) AS n
   FROM project_members
   WHERE user_id = ANY($1::uuid[])
   GROUP BY user_id`

   // CountByUsers: ids sudah kanonik, unik, 1..MaxUserIDs (dijamin usecase).
   // User tanpa keanggotaan tidak ada di hasil.
   func (r *Repo) CountByUsers(ctx context.Context, ids []string) (map[string]int, error)
   ```

   Implementasi: `SelectContext(ctx, &rows, qCountByUsers, pq.Array(ids))` atau cara yang sama dengan `countkeys` (cek driver di `internal/infra/db`: `countkeys` melempar `[]string` langsung dengan cast `$1::text[]::uuid[]`). Ikuti PERSIS konvensi `countkeys/repository.go`; bila driver pgx stdlib menerima `[]string` langsung, gunakan cast `$1::text[]::uuid[]` seperti countkeys dan sesuaikan teks SQL (hasil ekuivalen dengan SQL tiket `ANY($1::uuid[])`). Bangun map dari baris.

2. **`usecase.go`**

   ```go
   package countmemberships

   type repository interface {
       CountByUsers(ctx context.Context, ids []string) (map[string]int, error)
   }

   type Usecase struct{ repo repository }

   func NewUsecase(repo repository) *Usecase { return &Usecase{repo: repo} }

   // Count menghitung keanggotaan per user (satu query, maksimal MaxUserIDs id).
   func (uc *Usecase) Count(ctx context.Context, req countmemberships.Request) (countmemberships.Response, error)
   ```

   Alur `Count` (orkestrasi saja):
   1. `len(req.UserIDs) > countmemberships.MaxUserIDs` -> `errs.Wrap(errs.CodeInvalid, "maksimal 500 user id", errs.ErrInvalid)`.
   2. `ids := normalizeIDs(req.UserIDs)`; bila kosong -> `Response{Counts: map[string]int{}}` tanpa memanggil repo.
   3. `counts, err := uc.repo.CountByUsers(ctx, ids)`; error -> bungkus `%w`.
   4. `counts == nil` -> `map[string]int{}`. Kembalikan `Response{Counts: counts}`.

   `normalizeIDs`: salinan `countkeys.normalizeIDs` (parse uuid, `String()` kanonik, dedupe urutan kemunculan pertama). Disalin, BUKAN diimpor, karena lintas module/domain dilarang; fungsi kecil tidak layak jadi paket bersama.

   Format uuid tanpa tanda hubung / huruf besar yang valid untuk `uuid.Parse` diterima dan dikanonikkan. Konsekuensi: kunci `Counts` selalu bentuk kanonik.

3. **`handler.go`**: salinan pola `checkaccess`/`countkeys`: `bus.Expect[countmemberships.Request](req)`; salah tipe -> error `bus.ErrBadRequestType` (dikembalikan apa adanya); sukses mengembalikan `Response` sebagai nilai (bukan pointer).

4. **`init.go`**

   ```go
   // Init mendaftarkan handler bus project.management.count-memberships.
   func Init(d app.Deps) {
       uc := NewUsecase(newRepo(d.DB))
       d.Bus.Register(constants.BUS_PROJECT_MANAGEMENT_COUNT_MEMBERSHIPS, newHandler(uc).Handle)
   }
   ```

5. **`management/base.go`**: tambah import `.../management/countmemberships` dan di blok "Handler bus" tambah `countmemberships.Init(d)` setelah `checkaccess.Init(d)` / `getprojectbus.Init(d)`. Tidak ada route REST baru.

6. Cek: `go build ./...`, `go vet ./...`, `go test ./internal/domain/project/...`, `go test -tags integration ./internal/domain/project/internal/management/...`, arch test (`internal/archtest`) harus hijau (feature tidak meng-import module lain; hanya `base`-nya sendiri tidak dipakai, hanya `infra/db`, `infra/bus`, `lib`).

7. Catatan untuk implementer `auth-purgeinvitations`: konsumen memecah daftar kandidat menjadi chunk <= `countmemberships.MaxUserIDs` (500) per panggilan. Plan purge yang memiliki hal itu; plan ini hanya menjamin batas.

---

## 4. Migration

Tidak ada. Tabel `project_members` sudah ada (`backend/migrations/project/20261022090000000_create_projects.sql`). Verifikasi indeks `user_id` saat implement (lihat Summary butir 10); jika tidak ada, tetap tanpa migrasi pada T12 (job harian, tabel kecil), dicatat sebagai catatan PR.

---

## 5. Changes (per file/folder)

| File / folder | Aksi | Isi |
|---|---|---|
| `backend/internal/domain/project/internal/management/countmemberships/init.go` | baru | `Init(d app.Deps)` mendaftarkan handler bus |
| `.../countmemberships/handler.go` | baru | `bus.Expect[Request]`, delegasi ke `Usecase.Count` |
| `.../countmemberships/usecase.go` | baru | batas 500, `normalizeIDs`, orkestrasi repo, wrap error |
| `.../countmemberships/repository.go` | baru | satu query `GROUP BY` atas `project_members` |
| `.../countmemberships/usecase_test.go` | baru | unit usecase (fake repo) |
| `.../countmemberships/handler_test.go` | baru | unit handler |
| `.../countmemberships/countmemberships_integration_test.go` | baru | tag `integration`, DB nyata (`db.TestDB`) |
| `backend/internal/domain/project/internal/management/base.go` | ubah | `countmemberships.Init(d)` di blok handler bus |
| `backend/lib/constants/bus.go`, `backend/lib/contracts/project/management/countmemberships/` | dari foundation | tidak diubah di usecase ini (hanya dipakai) |

Tidak diubah: `base/store/store.go`, `base/users`, OpenAPI (`api/modules/project/management.yml`, tanpa HTTP), audit (`actions.go`), frontend, `docs/tickets/README.md` (katalog bus diperbarui oleh foundation).

---

## 6. Test Scenario

### 6.1 Unit usecase (`usecase_test.go`, fake repo seperti `countkeys`)

`fakeRepo` mencatat `calls` dan `got` (id yang diterima) serta mengembalikan `result`/`err`.

| # | Skenario | Harapan |
|---|---|---|
| 1 | `UserIDs` nil | `Counts` non-nil kosong, `calls == 0` |
| 2 | `UserIDs` `[]string{}` | sama |
| 3 | Semua id bukan uuid (`"abc"`, `""`, `"123"`) | `Counts` kosong, `calls == 0`, tanpa error |
| 4 | Campuran valid dan tidak valid | repo hanya menerima id valid; id tidak valid diabaikan |
| 5 | Normalisasi: uuid huruf besar dan tanpa tanda hubung | repo menerima bentuk kanonik lowercase |
| 6 | Duplikat (`a`, `a`, `A` kanonik sama) | repo menerima satu id, urutan kemunculan pertama |
| 7 | Tepat 500 id valid | lolos, `calls == 1`, repo menerima 500 id |
| 8 | 501 id (walau duplikat/tidak valid, panjang mentah > 500) | `errors.Is(err, errs.ErrInvalid)`, kode `INVALID_ARGUMENT`, `calls == 0` |
| 9 | Repo mengembalikan map berisi sebagian id | `Counts` diteruskan apa adanya (user tanpa keanggotaan absen) |
| 10 | Repo mengembalikan `nil` map tanpa error | `Counts` non-nil kosong |
| 11 | Repo error | error membungkus (`errors.Is(err, errRepo)`), `Response` kosong, pesan memuat `countmemberships` |
| 12 | Repo menerima 500 id valid unik (tidak dipotong/di-chunk di usecase) | satu panggilan repo |

### 6.2 Unit handler (`handler_test.go`)

| # | Skenario | Harapan |
|---|---|---|
| 1 | Request bertipe salah (`"salah"`, pointer `*Request`, `struct{}`) | `errors.Is(err, bus.ErrBadRequestType)`, repo tidak dipanggil |
| 2 | Request valid | mengembalikan `countmemberships.Response` (nilai) dengan `Counts` dari usecase |
| 3 | Error usecase (501 id) | diteruskan apa adanya, `errors.Is(err, errs.ErrInvalid)` |
| 4 | Lewat `bus.Registry` (`Register` + `bus.CallTyped[Response]`) | tipe respons cocok |

### 6.3 Integrasi repo/handler (`countmemberships_integration_test.go`, tag `integration`)

Setup mengikuti `checkaccess_integration_test.go`: `db.TestDB(t)`, `db.Up`, `bus.NewRegistry(nil)`, register handler dengan `NewUsecase(newRepo(d))`. Seed: tiga project P1, P2, P3; user A (anggota P1 dan P2 dan P3 = 3), B (anggota P1 = 1), C (anggota P2 dan P3 = 2), D (tanpa keanggotaan). Tidak ada baris tabel `users` yang diperlukan (tidak ada FK; justru itu inti kasusnya).

| # | Skenario | Harapan |
|---|---|---|
| 1 | Hitungan lintas project untuk `[A, B, C]` | `{A:3, B:1, C:2}` |
| 2 | `[A, D]` | `Counts` hanya berisi `A`; `D` absen (`_, ok := Counts[D]` false) |
| 3 | Hanya `[D]` | `Counts` kosong non-nil |
| 4 | Id tidak ada di tabel (uuid acak) | absen |
| 5 | Campur id valid dan `"bukan-uuid"` | tidak error; id tidak valid diabaikan; id valid terhitung |
| 6 | Id huruf besar untuk user A | kunci hasil kanonik lowercase |
| 7 | Hanya menghitung id yang diminta | user lain di tabel tidak ikut muncul (rule 6.10) |
| 8 | Setelah `DELETE FROM project_members WHERE user_id=A AND project_id=P3` | hitungan A turun jadi 2 |
| 9 | Keanggotaan role `owner` dan `member` sama-sama dihitung | A (owner di P1, member di P2) = 2 |
| 10 | 500 id uuid acak + satu id nyata | lolos, hasil hanya id nyata |
| 11 | 501 id | `errors.Is(err, errs.ErrInvalid)` lewat `bus.CallTyped` |
| 12 | Tanpa `reqctx` identitas pada ctx (`context.Background()`) | berhasil (tidak ada cek akses) |
| 13 | Tidak ada panggilan audit | register handler fake `BUS_CORE_AUDIT_RECORD` yang menghitung; tetap 0 |
| 14 | Tidak ada panggilan `core.auth.get-user` | tidak registrasi auth; tetap berhasil (tidak ada dependensi keluar) |

Regresi: seluruh tes `management` yang ada tetap hijau; `base_test`/`Register` menyebut handler baru terdaftar (bila ada tes yang menghitung endpoint terdaftar di bus, perbarui daftarnya).

---

## 7. E2E Scenarios

Usecase ini tanpa HTTP; skenario ditinjau lewat bus dan lewat konsumen purge.

Positif:

1. Module `project/management` dirakit lewat `Register(d)` di server nyata -> `bus.CallTyped[Response]` ke `project.management.count-memberships` dengan 2 user anggota dan 1 bukan -> `Counts` memuat dua user, yang ketiga absen.
2. Alur purge (lintas usecase, dirinci di plan purge): akun undangan kedaluwarsa > 30 hari tanpa keanggotaan -> `Counts` tidak memuat id -> dibaca 0 -> akun dihapus. Akun kedaluwarsa dengan keanggotaan di satu project -> `Counts[id] = 1` -> akun TIDAK dihapus.
3. Daftar kandidat purge 1.200 akun -> konsumen memanggil 3 kali (500, 500, 200); semua sukses.
4. Slice kosong -> `Counts` kosong, tanpa query ke DB.

Negatif:

1. Konsumen mengirim 501 id -> error `INVALID_ARGUMENT` (kontrak dilanggar); purge memperlakukan sebagai error dan tidak menghapus apa pun.
2. Request bertipe salah (pointer atau string) -> `bus.ErrBadRequestType`.
3. DB mati/query gagal -> error terbungkus (500 di sisi konsumen); purge fail-safe: tidak ada akun dihapus pada putaran itu, dicoba lagi pada jadwal berikutnya.
4. Provider tidak terdaftar (management gagal diinisialisasi) -> `bus.ErrNotRegistered` di konsumen; purge fail-safe (dijamin plan purge).
5. Id bukan uuid dalam daftar -> diabaikan tanpa error (tidak membuat seluruh panggilan gagal).

---

## 8. Request/Response body

Tidak ada body HTTP. Kontrak bus (nilai Go, bukan JSON di kawat; JSON di bawah hanya representasi).

Request valid:

```json
{ "UserIDs": [
  "7c1f3c2e-5b7a-4d7e-9a51-1f0b8a3e6d21",
  "9a2b4c6d-1e3f-4a5b-8c7d-0e1f2a3b4c5d",
  "bukan-uuid",
  "7C1F3C2E-5B7A-4D7E-9A51-1F0B8A3E6D21"
] }
```

Response (id kedua tidak punya keanggotaan, id bukan uuid diabaikan, duplikat kanonik digabung):

```json
{ "Counts": { "7c1f3c2e-5b7a-4d7e-9a51-1f0b8a3e6d21": 2 } }
```

Request kosong dan respons kosong:

```json
{ "UserIDs": [] }
```
```json
{ "Counts": {} }
```

Error (bus mengembalikan `*errs.Error`; bila suatu saat diteruskan ke HTTP oleh konsumen, bentuk envelope standar `httpx.WriteError`):

501 id:

```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "maksimal 500 user id" } }
```

Request bertipe salah: error Go `bus.ErrBadRequestType` (bukan `errs.Error`; tidak pernah sampai ke HTTP karena hanya konsumen internal yang memanggil).

Kegagalan DB: error Go terbungkus `countmemberships: count: ...`; di HTTP (bila diteruskan) menjadi `INTERNAL` 500 tanpa detail.

SQL final (rujukan tiket):

```sql
SELECT user_id::text, count(*) FROM project_members WHERE user_id = ANY($1::uuid[]) GROUP BY user_id
```

---

## 9. Risiko dan catatan

- Race purge (tanpa FK): member ditambahkan setelah hitungan dibuat dan sebelum `DELETE` akun. Sudah dicatat di overview bagian 12; mitigasi ada di SQL `DELETE` purge (cek ulang kondisi), bukan di usecase ini.
- Hasil adalah snapshot non-transaksional terhadap tabel auth; sengaja, karena modul berbeda.
- Kontrak bersifat consumer-driven; jangan menambah field respons tanpa memperbarui doc comment kontrak dan plan purge.
