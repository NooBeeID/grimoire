# T05 project/knowledge — Usecase Plan: `search`

Mengikuti `docs/superpowers/plans/project-knowledge/overview.md` (foundation). Bila bertentangan, overview menang.

## 1. Metadata

| Atribut | Nilai |
|---|---|
| Ticket | T05 |
| Scope | fullstack (bagian ini backend; kotak cari dan hasil bersorot di `frontend-plan.md`) |
| Domain | project |
| Module | knowledge |
| Feature | `search` (gelombang C, U10) |
| Resource root | `backend/internal/domain/project/internal/knowledge` |
| Endpoint | `/api/projects/{projectID}/knowledge/search?q=&prefix=&limit=` |
| Method | GET |
| Version | tanpa prefix versi URL (prefix `/api`); OpenAPI `info.version` 0.1.0; tidak ada kontrak bus baru |
| Auth | `middleware.RequireAuth()`; otorisasi project lewat `base/access` (member, owner, admin) |
| Body limit | default |
| Audit | tidak ada (read-only) |
| AC | AC4, AC6, AC9 |

Bus: mengonsumsi `project.management.check-access` (fail-closed, 500) lewat `base/access`. Tidak menyediakan endpoint bus (handler bus pencarian untuk agent adalah feature `bussearch`, U13, yang memakai `store.Search` yang sama).

## 2. Summary Plan

Pencarian full-text di UI untuk satu project. Alur usecase (hanya orkestrasi, tidak dipecah):

1. `access.Require(ctx, projectID)`: non-anggota atau `projectID` bukan uuid -> 404; tanpa sesi -> 401. Dilakukan SEBELUM parse input lain.
2. `base.ParseQuery(q)`: q dibersihkan (kontrol dibuang, trim, dipotong 200 rune, maks 16 term, tiap term/frasa dibungkus kutip). Tanpa term (kosong, hanya simbol) -> langsung 200 `{items: [], total: 0}` tanpa query DB. Kueri > 200 rune TIDAK ditolak, dipotong.
3. `base.NormalizePrefix(prefix)` (opsional; kosong = seluruh project). Gagal -> 400 `base.Invalid(reason)`. Urutan mengikuti spesifikasi: kueri kosong menang atas prefix tidak valid (tidak menyentuh DB, tidak membocorkan apa pun karena akses sudah lolos).
4. `limit`: kosong -> 20; bukan bilangan bulat -> 400 `invalid_limit`; angka di luar rentang di-clamp lewat `base.ClampLimit(n, 20, 50)` (<= 0 -> 20, > 50 -> 50).
5. `repo.Search(ctx, projectID, tsq, base.LikePrefix(prefix), limit)` = `store.Search` (SQL di overview 5.7): `websearch_to_tsquery('simple', $2)`, `ts_rank_cd(search, q, 1)` (normalisasi 1 = bagi 1 + log panjang dokumen), `count(*) OVER ()` sebagai total, top N di subquery (`ORDER BY score DESC, path COLLATE "C"`, `LIMIT N`), baru kemudian `ts_headline('simple', ...)` hanya untuk N baris itu, pada `left(content, 200000)` yang sudah dibersihkan dari karakter `\x01`/`\x02` agar marker tidak bisa dipalsukan isi file.
6. Untuk tiap hit: `base.ParseHeadline(headline)` -> `[]Segment{text, highlight}`. Respons 200 `SearchResultDTO`. Tidak ada HTML di respons; klien merender segmen sebagai teks (anti-XSS, AC5).

Keamanan dan kebenaran:
- Kueri user tidak pernah diteruskan mentah: `ParseQuery` membungkus tiap term dengan kutip sehingga `or`, `-`, `&`, `:`, `!`, `"` tak berpasangan tidak menjadi operator/error sintaks tsquery (AC4, tidak ada 500). Nilai dikirim sebagai parameter ($2), bukan interpolasi.
- Semua query terikat `project_id = $1` (rule 6.10); prefix memakai `LikePrefix` (escape `\ % _`, `docs` -> `docs/%`, jadi `docs2/` dan `docs.md` tidak ikut).
- Peringkat: kolom `search` memberi bobot A pada path, B pada judul, D pada isi, sehingga `login.md` (path memuat "login") berada di atas file panjang yang menyebut "login" sekali (AC4). Skor `float4` Postgres dipindai ke `float64`.
- Isi file tidak di-log; hanya `project_id`, `user_id`, panjang kueri (bukan teks kueri) bila perlu.

Performa (AC9): GIN `knowledge_files_search_gin` melayani `search @@ q`; headline dihitung hanya untuk <= 50 baris. Dibuktikan benchmark 5000 file p95 <= 200 ms dan EXPLAIN memakai indeks GIN.

## 3. Plan (langkah berurutan)

Prasyarat: foundation F selesai (`base/` termasuk `query.go`, `snippet.go`, `path.go`, `errors.go`; `base/store` dengan `Search`; `base/access`; migrasi tabel + indeks GIN). Cek `ls backend/internal/domain/project/internal/knowledge/base/store/store.go`; bila belum ada, STOP. Verifikasi `store.Search` sesuai overview 5.7 (subquery top-N lalu `ts_headline`, strip marker, `total` dari `count(*) OVER ()`); celah diperbaiki di `base/store` dengan test, bukan di feature.

1. Buat folder `backend/internal/domain/project/internal/knowledge/search/` (package `search`; bukan konflik dengan paket kontrak `lib/contracts/project/knowledge/search` karena feature ini tidak meng-importnya).
2. `params.go`:
   ```go
   type Input struct {
       ProjectID string
       Query     string // q mentah
       Prefix    string // prefix mentah
       Limit     string // limit mentah ("" = default)
   }
   ```
3. `usecase.go` (interface repository di sini; tanpa `repository.go` karena tanpa SQL khusus feature, dipenuhi `*store.Store`):
   ```go
   type Repository interface {
       Search(ctx context.Context, projectID, tsq, likePrefix string, limit int) ([]base.SearchHit, int, error)
   }
   type Usecase struct {
       access access.Checker
       repo   Repository
   }
   func NewUsecase(a access.Checker, r Repository) *Usecase
   func (u *Usecase) Execute(ctx context.Context, in Input) (base.SearchResultDTO, error) {
       if _, err := u.access.Require(ctx, in.ProjectID); err != nil { return base.SearchResultDTO{}, err }
       empty := base.SearchResultDTO{Items: []base.SearchItemDTO{}}
       tsq, ok := base.ParseQuery(in.Query)
       if !ok { return empty, nil }
       prefix, reason := base.NormalizePrefix(in.Prefix)
       if reason != "" { return empty, base.Invalid(reason) }
       n := 0
       if in.Limit != "" {
           v, err := strconv.Atoi(strings.TrimSpace(in.Limit))
           if err != nil { return empty, base.Invalid(base.ReasonInvalidLimit) }
           n = v
       }
       limit := base.ClampLimit(n, base.SearchDefaultLimit, base.SearchMaxLimit)
       hits, total, err := u.repo.Search(ctx, in.ProjectID, tsq, base.LikePrefix(prefix), limit)
       if err != nil { return empty, fmt.Errorf("search: query: %w", err) }
       items := make([]base.SearchItemDTO, 0, len(hits))
       for _, h := range hits {
           items = append(items, base.SearchItemDTO{Path: h.Path, Title: h.Title, Score: h.Score, Snippet: base.ParseHeadline(h.Headline)})
       }
       return base.SearchResultDTO{Items: items, Total: total}, nil
   }
   ```
   Catatan: `Snippet` tidak boleh nil (`ParseHeadline` mengembalikan slice kosong non-nil untuk headline kosong; bila belum, `base/snippet.go` diperbaiki dengan test). Tidak ada audit, transaksi, atau panggilan bus selain lewat `access`. Nilai `limit` sangat besar (melebihi int) gagal `Atoi` -> `invalid_limit`; angka valid besar di-clamp. Bila `strconv` menolak karena overflow, dianggap `invalid_limit` (diterima, didokumentasikan di OpenAPI).
4. `handler.go`:
   ```go
   type Handler struct{ uc *Usecase }
   func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
       qs := r.URL.Query()
       res, err := h.uc.Execute(r.Context(), Input{
           ProjectID: chi.URLParam(r, "projectID"),
           Query:     qs.Get("q"),
           Prefix:    qs.Get("prefix"),
           Limit:     qs.Get("limit"),
       })
       if err != nil { respond.WriteError(w, r, err); return }
       respond.WriteJSON(w, http.StatusOK, res)
   }
   ```
   Sesuaikan signature `respond.WriteError`/`WriteJSON` dengan kode di `main`. Tambahkan header `Cache-Control: no-store` (data project, hasil bergantung sesi).
5. `init.go`:
   ```go
   func Init(d app.Deps) {
       h := &Handler{uc: NewUsecase(access.New(d.Bus), store.New(d.DB))}
       d.API.With(middleware.RequireAuth()).
           Get("/projects/{projectID}/knowledge/search", h.ServeHTTP)
   }
   ```
   Pola pendaftaran sama dengan feature T03/`listtree`.
6. Tambah satu baris `search.Init(d)` di `backend/internal/domain/project/internal/knowledge/base.go` (`Register`); jangan menyentuh baris feature lain.
7. Verifikasi `api/modules/project/knowledge.yml` untuk path `/knowledge/search` (parameter `q` string, `prefix` string, `limit` integer 1..50 default 20; respons `KnowledgeSearchResult` dengan `snippet: [{text, highlight}]`; 400 `invalid_limit`/alasan path; 401; 404). Perbaiki di file itu bila berbeda, lalu `make api-bundle && make api-check`.
8. Tulis test (bagian 6), termasuk benchmark/integrasi AC9 dan EXPLAIN GIN.
9. Jalankan `go build ./... && go vet ./... && go test ./internal/domain/project/internal/knowledge/search/...` lalu `go test -tags integration ./internal/domain/project/internal/knowledge/...`. Catat output EXPLAIN dan angka p95 di deskripsi PR.

Aturan arsitektur: feature tidak meng-import feature lain; hanya `base`, `base/store`, `base/access`; usecase menerima interface (repository, checker); SQL hanya di `base/store`.

## 4. Migration

Tidak ada. Memakai tabel `knowledge_files`, kolom generated `search`, dan indeks `knowledge_files_search_gin` dari `backend/migrations/project/20261024090000000_create_knowledge_files.sql` (foundation).

## 5. Changes

| File/folder | Perubahan |
|---|---|
| `.../knowledge/search/init.go` | baru: rakit `access.New(d.Bus)`, `store.New(d.DB)`, usecase, handler; daftarkan GET route |
| `.../knowledge/search/handler.go` | baru: baca `projectID`, `q`, `prefix`, `limit`; panggil usecase; tulis 200 / error |
| `.../knowledge/search/params.go` | baru: `Input` |
| `.../knowledge/search/usecase.go` | baru: `Usecase`, interface `Repository`, `Execute` |
| `.../knowledge/search/usecase_test.go` | baru: unit dengan repository dan checker palsu |
| `.../knowledge/search/handler_test.go` | baru: httptest + router chi |
| `.../knowledge/search/search_integration_test.go` | baru (tag `integration`): DB nyata, AC4, prefix, frasa, isolasi, EXPLAIN, benchmark AC9 |
| `.../knowledge/base.go` | tambah baris `search.Init(d)` |
| `api/modules/project/knowledge.yml` | verifikasi path item `/knowledge/search` (dibuat di foundation) |

Tidak mengubah `base/` (kecuali perbaikan celah yang ditemukan, dengan test di foundation), migrasi, config, README, atau feature lain.

## 6. Test Scenario

### Unit usecase (`usecase_test.go`, repository dan checker palsu)

- Checker `ErrProjectNotFound` -> error itu; repository TIDAK dipanggil, walau `q`/`prefix`/`limit` tidak valid (non-anggota tetap 404).
- Checker error infra -> error terbungkus; repository tidak dipanggil.
- `q` kosong, spasi, `"""`, `&&&` -> `{Items: [], Total: 0}` (Items non-nil), repository tidak dipanggil.
- `q` 300 rune -> repository menerima tsq dari 200 rune pertama (bukan error).
- `q = "login"` -> repository menerima `tsq == "\"login\""`, `likePrefix == "%"`, `limit == 20`.
- `q = a & b : c or -d !e` dan `"unclosed` -> tidak error; tsq berkutip.
- `prefix` `docs` dan `docs/` -> `likePrefix == "docs/%"`; `../x`, `/docs`, `docs//a` -> 400 dengan `details.reason` `path_traversal`/`path_absolute`/`path_empty_segment`; repository tidak dipanggil. `q` kosong + prefix `../x` -> 200 kosong (urutan spesifikasi).
- `limit`: kosong -> 20; `5` -> 5; `0`, `-3` -> 20; `51`, `1000` -> 50; `abc`, `1.5` -> 400 `invalid_limit`.
- Repository error -> error terbungkus `search: query: ...` (500 generik).
- Hits dipetakan: headline `a \x01login\x02 b` -> 3 segmen (`highlight` hanya di tengah); headline kosong -> `snippet: []` non-nil; urutan hits dipertahankan; `total` diteruskan.

### Unit rule (di foundation; verifikasi mencakup)

- `base/query_test.go` (overview 11), `base/snippet_test.go` (ParseHeadline, marker yatim), `base/path_test.go` (`NormalizePrefix`, `LikePrefix`).

### Unit handler (`handler_test.go`)

- `GET .../search?q=login&prefix=docs&limit=5` -> usecase menerima nilai persis itu; 200 JSON dengan `items`, `total`.
- Tanpa `q` -> 200 `{"items":[],"total":0}`.
- `q` ter-URL-encode (`%22reset%20password%22`) diteruskan sebagai `"reset password"`.
- `projectID` bukan uuid -> 404; tanpa sesi -> 401 (`RequireAuth`); error 400 berformat seragam dengan `trace_id`.
- Respons tidak mengandung `<` atau tag HTML yang dibuat feature (isi file berisi `<script>` muncul sebagai teks segmen apa adanya, bukan di-escape ganda dan bukan HTML pembungkus `<mark>`).

### Integrasi (`search_integration_test.go`, tag `integration`, `db.TestDB`)

Fixture lewat SQL test / `createfile` repository (kolom `search` dihitung DB). Dua project.

1. **AC4 peringkat**: `login.md` pendek (judul `# Login`, isi menyebut login) dan `guide.md` panjang (>= 30 KB, menyebut `login` sekali) -> `q=login`: `login.md` urutan 1, skor `login.md` > skor `guide.md`; `total == 2`.
2. **AC4 kueri aneh**: `"`, `&`, `:`, `or`, `-`, `!`, `"unclosed`, `a & b : c`, `(((`, `\`, kueri 500 karakter, string berisi NUL/kontrol, emoji -> semuanya 200 (tidak pernah 500); yang tanpa term mengembalikan kosong.
3. **Frasa**: file X berisi "reset password flow", file Y berisi "password ... reset" terpisah; `q="reset password"` hanya X; `q=reset password` (tanpa kutip) memuat X dan Y.
4. **Operator dinetralkan**: `q=login or -guide` diperlakukan sebagai term literal (bukan OR/NOT); `q=-login` mencari kata `login`, bukan negasi.
5. **Prefix**: file `docs/a.md`, `docs2/b.md`, `docs.md`, `docs/auth/c.md`, semua memuat kata `alfa`; `prefix=docs` -> hanya `docs/a.md` dan `docs/auth/c.md`; `prefix=docs/` setara; `prefix=docs/auth` -> satu file; prefix berisi `_`/`%` literal (`a_b` vs `axb`).
6. **Isolasi (AC6/AC8)**: project A dan B masing-masing punya `same.md` dan kata unik (`kunciA`, `kunciB`); pencarian di A atas `kunciB` -> kosong, `total 0`; hasil A tidak pernah memuat path/isi B.
7. **Non-anggota 404 (AC6)**: user bukan anggota -> 404 `project tidak ditemukan`; `projectID` bukan uuid -> 404; super_admin -> 200. Memakai handler check-access T03 asli, atau palsu bila terlalu berat.
8. **Limit dan total**: 60 file memuat kata sama; default -> 20 item, `total 60`; `limit=5` -> 5; `limit=999` -> 50; `limit=0` -> 20; urutan deterministik (skor lalu path).
9. **Snippet**: kata cocok muncul sebagai segmen `highlight: true`; isi file berisi karakter `\x01`/`\x02` mentah tidak merusak segmen (marker di isi dibuang di SQL); isi berisi `<script>alert(1)</script>` kembali sebagai teks segmen; file 1 MB dengan kata cocok setelah 200000 karakter pertama tidak menimbulkan error (D4; headline fallback bisa tanpa sorotan, tidak 500).
10. **Judul dan path**: kata hanya ada di path (`payments/refund.md`, isi tanpa kata itu) ditemukan lewat `q=refund`; `q=login` cocok `docs/auth/login.md` karena path `/ . _ -` jadi spasi; kata hanya di judul ditemukan dan berperingkat di atas kemunculan di isi saja.
11. **Pemindahan (move)**: setelah `movefile`, pencarian kata dari nama baru menemukan file; kata dari nama lama tidak.
12. **Tanpa hasil**: kata tak ada -> 200 `{items: [], total: 0}`.
13. **EXPLAIN (rule 8.8)**: isi 5000 baris (beberapa project), `ANALYZE knowledge_files`; `EXPLAIN` bagian filter `search @@ websearch_to_tsquery(...)` dengan `project_id` dan prefix memakai `knowledge_files_search_gin` (Bitmap Index Scan) atau, bila planner memilih `UNIQUE (project_id, path)`, tetap dicatat; test mengharapkan teks plan memuat `knowledge_files_search_gin` untuk kueri berselektivitas wajar (kata yang ada di <= ~5% baris). Tidak memakai `SET enable_seqscan = off` sebagai bukti. Simpan keluaran di deskripsi PR.
14. **AC9 benchmark/integrasi**: 5000 file campuran 1-50 KB dalam satu project (kosakata acak dari ~2000 kata, beberapa kata sering, beberapa langka, campuran bahasa Indonesia/Inggris); 50 kueri campuran (satu kata, dua kata, frasa, dengan prefix, karakter aneh), masing-masing dijalankan 3x setelah 1x pemanasan melalui HTTP handler penuh (router + bus check-access palsu cepat); hitung p95; `p95 <= 200 ms`. Dilewati dengan `t.Skip` bila `testing.Short()`. Sediakan juga `BenchmarkSearch` (`go test -tags integration -bench Search`). Catat p50/p95 di PR.
15. **Keamanan SQL**: kueri berisi `'; DROP TABLE knowledge_files; --` -> 200, tabel utuh (parameterized).

Registrasi: `TestRoutesRegistered` di W memastikan route GET `/api/projects/{projectID}/knowledge/search` terdaftar.

## 7. E2E Scenarios (Playwright terhadap binary)

Positif:
1. Setelah upload folder `docs/` (termasuk `docs/auth/login.md` dan satu file panjang yang menyebut "login" sekali), cari `login` di tab Knowledge -> `login.md` di urutan pertama, kata "login" tersorot (elemen highlight dirender dari segmen, bukan `innerHTML`).
2. Klik hasil -> viewer membuka file tersebut.
3. Cari dengan frasa `"reset password"` -> hanya file dengan frasa itu.
4. Cari dengan prefix (filter folder `docs/auth`) -> hanya file di bawah folder itu.
5. Kotak cari dikosongkan -> pohon kembali tampil, tanpa request error; kueri kosong -> daftar hasil kosong tanpa pesan galat.
6. Super Admin (bukan anggota) mencari di project -> hasil muncul.
7. Setelah memindah file ke nama baru, pencarian menemukannya dengan kata dari nama baru.

Negatif:
1. Kueri `" & : or -` dan `"unclosed` -> tidak ada toast error, UI menampilkan "tidak ada hasil" atau hasil valid (bukan 500).
2. File berisi `<script>alert(1)</script>` ditemukan lewat pencarian -> cuplikan tampil sebagai teks, tidak ada dialog `alert` (AC5).
3. Developer non-anggota membuka kueri langsung `page.request.get(.../search?q=login)` project lain -> 404.
4. `page.request.get(.../search?q=a&limit=abc)` -> 400 `invalid_limit`; `limit=1000` -> 200 dengan paling banyak 50 item.
5. `prefix=../x` dengan `q=login` -> 400 `path_traversal`.
6. Tanpa sesi -> 401 dan UI mengarahkan ke login.
7. Kueri sangat panjang (>200 karakter) -> 200 (dipotong), UI tidak error.

## 8. Request/Response body

Permintaan: `GET /api/projects/0192f0c4-7b00-7d11-8a00-1a2b3c4d5e6f/knowledge/search?q=login&prefix=docs&limit=20` (GET, tanpa body, tanpa header CSRF).

200:

```json
{
  "items": [
    {
      "path": "docs/auth/login.md",
      "title": "Login",
      "score": 0.83,
      "snippet": [
        { "text": "Alur ", "highlight": false },
        { "text": "login", "highlight": true },
        { "text": " memakai email dan password", "highlight": false }
      ]
    },
    {
      "path": "docs/guide.md",
      "title": "Panduan",
      "score": 0.04,
      "snippet": [
        { "text": "… setelah ", "highlight": false },
        { "text": "login", "highlight": true },
        { "text": " pengguna diarahkan ke dashboard …", "highlight": false }
      ]
    }
  ],
  "total": 2
}
```

200 kueri tanpa kata (`q=`, `q=%22%26%3A`) atau tanpa hasil:

```json
{ "items": [], "total": 0 }
```

400 `limit` bukan bilangan bulat (`?q=login&limit=abc`):

```json
{
  "error": {
    "code": "INVALID_ARGUMENT",
    "message": "limit harus bilangan bulat",
    "details": { "reason": "invalid_limit" }
  },
  "trace_id": "0192f0c4-7b00-7d11-8a00-1a2b3c4d5e6f"
}
```

400 prefix tidak valid (`?q=login&prefix=../x`):

```json
{
  "error": {
    "code": "INVALID_ARGUMENT",
    "message": "path tidak boleh memuat segmen . atau ..",
    "details": { "reason": "path_traversal" }
  },
  "trace_id": "0192f0c4-7b00-7d11-8a00-1a2b3c4d5e6f"
}
```

Alasan 400 prefix lain: `path_absolute`, `path_empty_segment`, `path_control_char`, `path_too_long`, `segment_too_long`, `invalid_encoding`.

401 tanpa sesi:

```json
{ "error": { "code": "UNAUTHORIZED", "message": "perlu login" }, "trace_id": "..." }
```

404 non-anggota atau `projectID` bukan uuid:

```json
{ "error": { "code": "NOT_FOUND", "message": "project tidak ditemukan" }, "trace_id": "..." }
```

500 (check-access gagal atau DB gagal): error generik tanpa detail internal, mis. `{"error":{"code":"INTERNAL","message":"terjadi kesalahan"},"trace_id":"..."}` (sesuai format `respond` T00).

Bus: tidak ada endpoint bus baru. Konsumsi: `bus.CallTyped[checkaccess.Response](ctx, b, constants.BUS_PROJECT_MANAGEMENT_CHECK_ACCESS, checkaccess.Request{ProjectID, UserID, UserRole})` melalui `base/access`.

Audit: tidak ada entri untuk usecase ini.
