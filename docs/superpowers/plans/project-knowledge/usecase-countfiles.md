# T05 project/knowledge — Usecase `countfiles` (bus `project.knowledge.count-files`)

Melengkapi `docs/superpowers/plans/project-knowledge/overview.md` (F: migrasi `knowledge_files`, konstanta bus, `knowledge/base.go`). Bila dokumen ini bertentangan dengan overview, overview menang. Hanya satu usecase: `countfiles` (U4, gelombang A). Jangan tertukar dengan `buslistfiles` (U11, mengembalikan daftar file); di sini hanya jumlah per project untuk consumer T03.

## Metadata

| Field | Nilai |
|---|---|
| Ticket | T05 |
| Scope | fullstack pada tiket; usecase ini backend saja, tanpa UI dan tanpa REST |
| Domain | project |
| Module | knowledge |
| Feature | `countfiles` (gelombang A, U4) |
| Endpoint | bus `project.knowledge.count-files` (konstanta `BUS_PROJECT_KNOWLEDGE_COUNT_FILES`). Tidak ada route HTTP |
| Method | bus Call (`bus.CallTyped[contentcount.Response]`) |
| Version | v1 (kontrak netral `lib/contracts/project/management/contentcount`, milik T03, tidak diubah) |
| Provider | T05 `countfiles` |
| Consumer | T03 `listprojects`, `getproject`, `deleteproject` (fail-soft di sisi mereka: `ErrNotRegistered` atau error apa pun -> hitungan `null`) |
| Auth / otorisasi | tidak ada. Pemanggil adalah kode internal T03 yang sudah memverifikasi akses ke tiap project. Tidak memakai `base/access` (overview 5.6 butir 6) |
| Bus dikonsumsi | tidak ada |
| Audit | tidak ada (aksi baca, rule 9.3) |
| Tabel / migrasi | membaca `knowledge_files`; tidak ada migrasi baru |
| AC | tidak ada AC T05 khusus; dibuktikan lewat integrasi T03 (`getproject` menampilkan `counts.files`) |
| Resource root | `backend/internal/domain/project/internal/knowledge` |
| allowedPaths | `.../knowledge/countfiles/`, `.../knowledge/base.go` (satu baris `countfiles.Init(d)`), `backend/lib/constants/bus.go` (hanya bila `BUS_PROJECT_KNOWLEDGE_COUNT_FILES` belum ada atau komentarnya perlu diperbarui) |

Prasyarat (dari F atau T03; bila belum ada, buat persis seperti plan T03 overview 5.1 dan 5.4, jangan versi baru):
- `backend/lib/contracts/project/management/contentcount/contentcount.go`: `MaxProjectIDs = 100`, `Request{ProjectIDs []string}`, `Response{Counts map[string]int}`.
- Konstanta `BUS_PROJECT_KNOWLEDGE_COUNT_FILES = "project.knowledge.count-files"` di `backend/lib/constants/bus.go`.
- Migrasi `<VERSION>_create_knowledge_files.sql` (tabel `knowledge_files`, `UNIQUE (project_id, path)`).
- `knowledge.Register(d app.Deps)` dan `project.Register` memanggilnya; `store.New(d.DB)` dari `base/store`.
- Cek awal: `ls backend/lib/contracts/project/management/contentcount` dan `grep -n COUNT_FILES backend/lib/constants/bus.go`.

## Summary Plan

`countfiles` menjawab "berapa file knowledge di tiap project ini" untuk banyak project sekaligus dengan satu query. Alur usecase:

1. Handler: `req, err := bus.Expect[contentcount.Request](r)` lebih dulu (rule 6.9). Salah tipe -> error membungkus `bus.ErrBadRequestType`, dikembalikan apa adanya.
2. Usecase `Count(ctx, ids []string) (map[string]int, error)`:
   - `len(ids) > contentcount.MaxProjectIDs` -> `fmt.Errorf("countfiles: %d project id melebihi batas %d: %w", n, max, errs.ErrInvalid)`.
   - Hasil awal: map kosong (`make(map[string]int, len(ids))`, tidak pernah nil).
   - Untuk tiap id: `uuid.Parse(id)` gagal -> `out[id] = 0`, tidak dikirim ke query. Berhasil -> `out[id] = 0` dan masuk daftar kueri (dedupe berdasarkan UUID kanonik; satu UUID kanonik dapat dipetakan ke beberapa string asli bila beda huruf besar/kecil).
   - Daftar kueri kosong (input kosong atau semua tidak valid) -> kembalikan tanpa query.
   - Satu query via repository; hasil `canonical -> n` dipetakan kembali ke SEMUA string asli yang mem-parse ke UUID itu.
3. Handler mengembalikan `contentcount.Response{Counts: out}`.

Keputusan:
- Setiap id yang diminta ada di `Counts` (0 bila tanpa file atau bukan uuid). Ini lebih ketat daripada komentar kontrak T03 ("boleh tidak ada"), dan tetap kompatibel dengan consumer yang membaca `Counts[id]` (0 sama dengan tidak ada).
- Id bukan uuid TIDAK menjadi error: diabaikan (tidak diquery) dan dihitung 0, sehingga satu id rusak tidak menggagalkan batch.
- Kunci map memakai string persis seperti yang dikirim pemanggil (consumer T03 melakukan lookup dengan id miliknya sendiri). Query memakai UUID kanonik, jadi `ABC...` huruf besar tetap benar.
- Isolasi (rule 6.10): query hanya menghitung baris `project_id = ANY($1)` dari daftar yang diminta; tidak ada hitungan project lain bocor ke map.
- Tanpa audit, tanpa check-access, tanpa log isi. Log hanya bila error infra: `feature=countfiles`, jumlah id (bukan daftar id).
- Tanpa `params.go` (tidak ada HTTP). Ada `repository.go` karena SQL khusus feature (agregat GROUP BY tidak dipakai usecase lain).
- Performa: `UNIQUE (project_id, path)` punya `project_id` sebagai kolom depan, jadi `project_id = ANY(...)` memakai indeks itu (index-only scan bila memungkinkan). Maks 100 id per panggilan.
- Perbedaan nilai tak terduga: `count(*)` bertipe bigint; dipindai ke `int64` lalu dikonversi `int`.

## Plan

Urutan wajib. Tulis test dulu (TDD), lalu implementasi.

**B0. Verifikasi prasyarat (tanpa edit).** Cek simbol di bagian Prasyarat. Baca signature nyata `bus.Expect`, `Registry.Register`, dan tipe handler bus di `backend/internal/infra/bus`; baca bentuk `Init` feature bus lain yang sudah ada di `main` (mis. handler `getuser` di T01 atau `checkaccess` T03) dan ikuti persis. Bila `contentcount` atau konstanta belum ada, tambahkan persis plan T03 overview 5.1/5.4.

**B1. `countfiles/repository.go`**

```go
package countfiles

type Repository struct{ s *store.Store }

func NewRepository(s *store.Store) *Repository { return &Repository{s: s} }

// CountByProject mengembalikan jumlah file per project untuk uuid KANONIK (lowercase).
// Project tanpa file tidak muncul di map (usecase mengisi 0).
func (r *Repository) CountByProject(ctx context.Context, projectIDs []string) (map[string]int, error) {
    var rows []struct {
        ProjectID string `db:"project_id"`
        N         int64  `db:"n"`
    }
    err := sqlx.SelectContext(ctx, r.s.Q(),  &rows, `
        SELECT project_id::text AS project_id, count(*) AS n
        FROM knowledge_files
        WHERE project_id = ANY($1::uuid[])
        GROUP BY project_id`, projectIDs)
    // err -> fmt.Errorf("countfiles: count: %w", err)
    // map[row.ProjectID] = int(row.N)
}
```

Catatan driver: `[]string` ke `$1::uuid[]` dengan pgx stdlib biasanya diterima. Bila driver menolak, bungkus dengan `pgtype.FlatArray[string]` (atau tipe array yang sudah dipakai repository T00/T03); jangan mengubah query menjadi `project_id::text = ANY(...)` karena mematikan indeks. Gunakan helper `sqlx.SelectContext`/`Select` sesuai bentuk `db.Querier` di T00.

**B2. `countfiles/usecase.go`**

```go
package countfiles

type counter interface {
    CountByProject(ctx context.Context, projectIDs []string) (map[string]int, error)
}

type Usecase struct{ repo counter }

func NewUsecase(repo counter) *Usecase { return &Usecase{repo: repo} }

func (u *Usecase) Count(ctx context.Context, ids []string) (map[string]int, error)
```

Isi `Count`: batas `MaxProjectIDs`; `out` terisi 0 untuk semua id; `byCanon := map[string][]string{}` (UUID kanonik -> string asli); `uuid.Parse` per id (`parsed.String()` sebagai kanonik); `canon := keys(byCanon)` terurut agar query deterministik; `len(canon) == 0` -> return `out, nil`; `counts, err := u.repo.CountByProject(ctx, canon)` (error -> `fmt.Errorf("countfiles: %w", err)`, tanpa menelan); untuk tiap `canon, n` di `counts`, isi `out[orig] = n` untuk semua `orig` di `byCanon[canon]` (kanonik yang tidak diminta diabaikan sebagai jaring pengaman). Usecase tidak menyentuh `sqlx`, tidak me-log.

**B3. `countfiles/handler.go`**

```go
func handle(uc *Usecase) bus.Handler { // sesuaikan dengan tipe handler nyata infra/bus
    return func(ctx context.Context, req any) (any, error) {
        in, err := bus.Expect[contentcount.Request](req)
        if err != nil { return nil, err }
        counts, err := uc.Count(ctx, in.ProjectIDs)
        if err != nil { return nil, err }
        return contentcount.Response{Counts: counts}, nil
    }
}
```

Bentuk persis mengikuti `bus.Expect` di infra (ia dapat menerima `*http.Request`-nya sendiri atau `any`; ikuti signature nyata dari B0). Respons berupa value `contentcount.Response`, bukan pointer, sama dengan provider lain.

**B4. `countfiles/init.go`**

```go
func Init(d app.Deps) {
    uc := NewUsecase(NewRepository(store.New(d.DB)))
    d.Bus.Register(constants.BUS_PROJECT_KNOWLEDGE_COUNT_FILES, handle(uc))
}
```

Registrasi ganda = panic (sifat bus), jadi `Init` hanya dipanggil sekali dari `knowledge.Register`.

**B5. `knowledge/base.go`**: tambah satu baris `countfiles.Init(d)` (impor `.../knowledge/countfiles`). Jangan mengubah baris `Init` feature lain.

**B6. Komentar konstanta**: di `backend/lib/constants/bus.go` pastikan komentar di atas `BUS_PROJECT_KNOWLEDGE_COUNT_FILES` berbunyi "Provider: T05 (handler `countfiles`). Consumer: T03. Kontrak: lib/contracts/project/management/contentcount." Jangan menduplikasi konstanta.

**B7. Verifikasi**: `go build ./... && go vet ./...`; `go test ./internal/domain/project/internal/knowledge/countfiles/...`; `go test -tags integration ./internal/domain/project/internal/knowledge/countfiles/...`; `internal/archtest` lulus (feature tidak meng-import feature lain; hanya `base/store`, `lib/contracts`, `infra/bus`).

## Migration

Tidak ada. Memakai tabel `knowledge_files` dari migrasi F `backend/migrations/project/<VERSION>_create_knowledge_files.sql`. Indeks yang dipakai: `knowledge_files_project_id_path_key` (UNIQUE `(project_id, path)`).

## Changes

| File/folder | Perubahan |
|---|---|
| `backend/internal/domain/project/internal/knowledge/countfiles/init.go` | baru: merakit repository + usecase, mendaftarkan handler bus |
| `.../countfiles/handler.go` | baru: `bus.Expect[contentcount.Request]` lalu panggil usecase |
| `.../countfiles/usecase.go` | baru: batas 100, validasi uuid, dedupe kanonik, mapping hasil, isi 0 |
| `.../countfiles/repository.go` | baru: satu query GROUP BY |
| `.../countfiles/usecase_test.go` | baru: unit dengan repository palsu |
| `.../countfiles/handler_test.go` | baru: salah tipe, sukses, error repository, melalui registry bus nyata |
| `.../countfiles/repository_integration_test.go` | baru (tag `integration`): `db.TestDB`, SQL nyata |
| `.../knowledge/base.go` | tambah `countfiles.Init(d)` |
| `backend/lib/constants/bus.go` | hanya komentar `COUNT_FILES` (dan tambah konstanta bila T03 belum membuatnya) |
| `backend/lib/contracts/project/management/contentcount/contentcount.go` | hanya dibuat bila belum ada (persis T03 5.4); bila ada, TIDAK diubah |

Tidak ada perubahan OpenAPI (bukan REST), tidak ada config baru, tidak ada aksi audit baru, tidak ada frontend.

## Test Scenario

### Unit (`usecase_test.go`, repository palsu yang mencatat argumen dan jumlah panggilan)

- Batch beberapa project: input `[A, B, C]`, repo mengembalikan `{A:12, B:3}` -> `{A:12, B:3, C:0}`; repo dipanggil TEPAT SEKALI dengan ketiga id.
- Project tanpa file -> `0` ada di map (kunci hadir).
- Id bukan uuid (`"bukan-uuid"`, `""`, `"123"`): muncul di map dengan 0, TIDAK dikirim ke repo.
- Campuran uuid valid dan tidak valid: repo hanya menerima yang valid; tidak ada error.
- Semua id tidak valid atau input kosong/nil: map kosong atau semua 0, repo TIDAK dipanggil (jumlah panggilan 0); map tidak pernah nil.
- Duplikat id (`[A, A]`): repo menerima A sekali; map berisi A dengan nilai benar.
- UUID huruf besar: `strings.ToUpper(A)` dan `A` sama-sama diminta -> keduanya terisi nilai yang sama; repo menerima kanonik sekali.
- Tepat 100 id valid -> ok; 101 id -> error `errors.Is(err, errs.ErrInvalid)`, repo tidak dipanggil.
- Repo error -> error dikembalikan terbungkus (`errors.Is(err, sentinel)` benar), bukan map parsial.
- Repo mengembalikan kanonik yang tidak diminta -> diabaikan.

### Handler (`handler_test.go`, registry bus nyata + repository palsu)

- Salah tipe request (`"x"`, `contentcount.Response{}`, `*contentcount.Request` bila infra hanya menerima value) -> `errors.Is(err, bus.ErrBadRequestType)`, repo tidak dipanggil.
- `bus.CallTyped[contentcount.Response](ctx, reg, constants.BUS_PROJECT_KNOWLEDGE_COUNT_FILES, contentcount.Request{ProjectIDs: ...})` mengembalikan map yang benar.
- Endpoint yang tidak diinisialisasi -> `errors.Is(err, bus.ErrNotRegistered)` (kontrol: ini perilaku yang di-fail-soft T03).

### Integrasi (`repository_integration_test.go`, tag `integration`, `db.TestDB`)

Fixture: tabel `projects` (insert langsung via SQL test) dan `knowledge_files` (insert SQL langsung).
- Batch beberapa project dalam SATU query: project A 3 file, B 1 file, C 0 file -> `{A:3, B:1}` dari repo, usecase melengkapi `C:0`. Pastikan hanya satu statement yang dikirim (bungkus `Querier` penghitung, atau cek `pg_stat_statements` bila tersedia; minimal, `Querier` pembungkus menghitung panggilan = 1).
- Isolasi: file project D (tidak diminta) tidak mempengaruhi hasil dan tidak muncul di map.
- Project tanpa file -> 0; id bukan uuid + uuid valid dalam satu panggilan -> tidak error, nilai benar.
- Hapus file lalu hitung ulang -> berkurang; hapus baris `projects` (cascade) -> 0.
- EXPLAIN (rule 8.8): untuk `project_id = ANY($1::uuid[])` pada 5000 baris tersebar di 50 project, plan memakai `knowledge_files_project_id_path_key` (tanpa Seq Scan setelah `ANALYZE`). Catat hasil di deskripsi PR.
- Jumlah besar: 100 project x 50 file tetap satu query dan hasil tepat.

### Integrasi lintas T03 (`knowledge_integration_test.go` bagian W, tag `integration`)

- `count-files` terdaftar setelah `knowledge.Register`: `bus.CallTyped[contentcount.Response]` untuk 2 project + 1 id bukan uuid -> `{pidA: n, pidB: 0, "bukan-uuid": 0}`.
- Dengan handler T03 asli (atau `getproject` T03 melalui router): `GET /api/projects/{id}` menampilkan `counts.files` = jumlah file sesudah `createfile` dua kali; sebelum T05 terdaftar (registry tanpa `knowledge.Register`) `counts.files` = `null` dan response 200 (fail-soft, perilaku T03).
- `GET /api/projects` (listprojects): `counts.files` tiap project sesuai; satu panggilan bus per request (batch, tanpa N+1).
- Hapus file (`deletefile`) lalu `getproject` -> hitungan turun.

## E2E Scenarios

Tidak ada UI atau REST milik usecase ini; skenario E2E (Playwright terhadap binary, butuh T01, T02, T03 merge) memverifikasi efek di UI T03 yang mengonsumsi bus.

Positif:
1. Developer membuat project, upload 3 file knowledge (lewat `uploadfiles` atau `createfile`), lalu membuka daftar project: kartu project menampilkan jumlah file 3.
2. Buka detail project: ringkasan menampilkan 3 file; hapus satu file, muat ulang detail -> 2.
3. Project baru tanpa file menampilkan 0 (bukan tanda `-`), sementara project dengan hitungan `null` (provider tidak tersedia) menampilkan penanda "tidak tersedia" sesuai desain FE T03.

Negatif:
1. Hitungan hanya untuk project yang boleh dilihat: developer non-anggota tidak melihat project lain di daftar, jadi tidak ada hitungan project orang lain di layar (pemanggil T03 yang menyaring; provider tidak memeriksa akses).
2. Bila `count-files` gagal (disimulasikan di test integrasi dengan handler yang error), halaman daftar/detail project tetap 200 dengan hitungan `null`; tidak ada error di UI.
3. Endpoint bus tidak bisa dipanggil dari luar (tidak ada route HTTP `/api/...` yang menyajikannya): `GET /api/projects/{id}/knowledge/count` -> 404.

## Request/Response body

Tidak ada body HTTP. Kontrak bus (Go, bukan JSON di wire):

```go
res, err := bus.CallTyped[contentcount.Response](ctx, b, constants.BUS_PROJECT_KNOWLEDGE_COUNT_FILES,
    contentcount.Request{ProjectIDs: []string{pidA, pidB, pidC, "bukan-uuid"}})
// sukses:
// contentcount.Response{Counts: map[string]int{pidA: 12, pidB: 3, pidC: 0, "bukan-uuid": 0}}

bus.CallTyped[contentcount.Response](ctx, b, constants.BUS_PROJECT_KNOWLEDGE_COUNT_FILES,
    contentcount.Request{ProjectIDs: nil})
// contentcount.Response{Counts: map[string]int{}}, err nil (tanpa query)

bus.CallTyped[contentcount.Response](ctx, b, constants.BUS_PROJECT_KNOWLEDGE_COUNT_FILES, "salah tipe")
// err membungkus bus.ErrBadRequestType

bus.CallTyped[contentcount.Response](ctx, b, constants.BUS_PROJECT_KNOWLEDGE_COUNT_FILES,
    contentcount.Request{ProjectIDs: <101 id>})
// err membungkus errs.ErrInvalid: "countfiles: 101 project id melebihi batas 100: invalid argument"

// provider belum diinisialisasi (mis. T05 belum merge):
// err membungkus bus.ErrNotRegistered -> consumer T03 fail-soft
```

Query SQL (satu-satunya):

```sql
SELECT project_id::text AS project_id, count(*) AS n
FROM knowledge_files
WHERE project_id = ANY($1::uuid[])
GROUP BY project_id
```

Pantulan di REST T03 (milik T03, untuk konteks; format error seragam rule 11.1):

```json
{ "id": "0192f0c4-7b00-7d11-8a00-1a2b3c4d5e6f", "name": "Payment Service",
  "counts": { "files": 12, "specs": null, "keys": null } }
```

`files: null` bila provider tidak tersedia atau gagal; angka (termasuk 0) bila `count-files` sukses. Error bus tidak pernah bocor ke klien REST; di sisi bus error bersifat internal (`errors.Is`, bukan JSON). Bila suatu saat error ditampilkan lewat HTTP, bentuknya tetap:

```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "countfiles: 101 project id melebihi batas 100" }, "trace_id": "0192f0c4-7b00-7d11-8a00-1a2b3c4d5e6f" }
```
