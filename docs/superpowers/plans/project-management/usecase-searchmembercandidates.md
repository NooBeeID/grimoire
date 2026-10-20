# T03 project/management — Usecase `searchmembercandidates` (GET /api/projects/{projectID}/member-candidates)

Melengkapi `docs/superpowers/plans/project-management/overview.md` (F: migrasi, `base/`, `base/store`, `base/users`, `management/base.go`, `management.yml`). Bila dokumen ini bertentangan dengan overview, overview menang. Hanya satu usecase: `searchmembercandidates` (U10, gelombang C, keputusan D2). Jangan tertukar dengan `listmembers` (daftar anggota saat ini) maupun `addmember` (yang memvalidasi ulang kandidat lewat `get-user`; hasil endpoint ini hanya saran UI, bukan jaminan).

## Metadata

| Field | Nilai |
|---|---|
| Ticket | T03 |
| Scope | fullstack pada tiket; plan usecase ini backend saja. FE (autocomplete dialog tambah anggota) ada di `frontend-plan.md` |
| Domain | project |
| Module | management |
| Feature | `searchmembercandidates` (gelombang C, U10, D2) |
| Endpoint | `/api/projects/{projectID}/member-candidates?q=&limit=` |
| Method | GET |
| Version | tanpa prefix versi di URL; `management.yml` `info.version: 0.1.0` |
| Auth | `middleware.RequireAuth()`. Otorisasi project di usecase: `FindAccess` -> `ResolveLevel` -> `Require(PermManageMembers)` |
| Siapa | owner, super_admin (`admin`). member -> 403. Bukan anggota / project tidak ada / id bukan uuid -> 404 |
| Bus dikonsumsi | `core.auth.find-developers` (T01, `Limit: finddevelopers.MaxLimit` = 50) lewat `base/users`. Tidak memanggil `get-user` (D6) dan tidak memakai `base/counts` |
| Bus disediakan | tidak ada |
| Audit | tidak ada (aksi baca; keputusan D2) |
| Tabel / migrasi | membaca `projects` dan `project_members`; tidak ada migrasi |
| AC | AC2 (member tidak bisa mengelola anggota -> 403); mendukung AC4 di sisi UI (kandidat hanya developer aktif) |
| Resource root | `backend/internal/domain/project/internal/management` |
| allowedPaths | `.../management/searchmembercandidates/`, `.../management/base.go` (satu baris `searchmembercandidates.Init(d)`) |

Prasyarat dari F (jangan dibuat di sini; bila belum ada, hentikan dan laporkan): `base.Actor`/`ActorFrom`, `base.ResolveLevel`, `base.Require`/`PermManageMembers`, `base.ErrProjectNotFound`, `base.InvalidFields`/`FieldErrors`, `base.Member`, `base.CandidateDTO` (json `id`, `name`, `email`), `base/store.Store.FindAccess` dan `ListMembers`, `base/users.Lookup` (`FindDevelopers(ctx, q, limit)`), `management.Register`, path `GET /api/projects/{projectID}/member-candidates` + schema `MemberCandidateList` di `api/modules/project/management.yml`. Kontrak `core.auth.find-developers` dan konstanta `BUS_CORE_AUTH_FIND_DEVELOPERS` berasal dari T01 (atau dibuat F persis seperti plan T01); jangan didefinisikan ulang.

## Summary Plan

Endpoint autocomplete untuk dialog "tambah anggota": owner atau Super Admin mengetik potongan nama/email, server mengembalikan developer aktif yang BELUM menjadi anggota project.

Alur usecase (urutan wajib):

1. Aktor dari `base.ActorFrom(reqctx.IdentityFrom(ctx))`; gagal -> `errs.New(errs.CodeUnauthorized, "perlu login")`.
2. `p, memberRole, found, err := repo.FindAccess(ctx, projectID, actor.ID)`; `!found` -> `base.ErrProjectNotFound` (404).
3. `level := base.ResolveLevel(actor.Role, memberRole, true)`; `base.Require(level, base.PermManageMembers)`. none -> 404 (tidak bocor), member -> 403.
4. Validasi query SETELAH otorisasi (agar pemanggil yang tidak berhak selalu mendapat 404/403, tidak pernah 400 yang membedakan): `parseQuery(rawQ, rawLimit)`. Gagal -> `base.InvalidFields` (400).
5. `q` kosong (setelah trim) -> `Items: []` tanpa `ListMembers` dan tanpa bus.
6. `members, err := repo.ListMembers(ctx, p.ID)` -> himpunan `user_id` anggota.
7. `devs, err := users.FindDevelopers(ctx, q, finddevelopers.MaxLimit)` (selalu 50, bukan `limit` klien, agar setelah anggota dibuang masih ada sisa untuk dipotong).
8. Buang developer yang `ID`-nya ada di himpunan anggota; potong ke `limit`; petakan ke `base.CandidateDTO`. Urutan dari provider (`name, id`) dipertahankan.

Keputusan:
- Otorisasi sebelum validasi param. Handler tidak mem-parse `limit` (hanya meneruskan string mentah); parsing murni ada di `params.go` dan dipanggil usecase setelah `Require`. Ini mencegah bocornya keberadaan project lewat perbedaan 400 vs 404.
- `q`: `strings.TrimSpace`; maksimal 100 rune (lebih -> 400 `fields.q` = `"q maksimal 100 karakter"`, tidak dipotong diam-diam; konsisten dengan `listprojects`). Kosong bukan error: `{ "items": [] }`.
- `limit`: kosong -> 10 (`DefaultLimit` lokal feature = 10); integer 1..50; di luar itu atau bukan integer -> 400 `fields.limit` = `"limit harus 1-50"`. `limit` tidak diperiksa bila `q` kosong? Tetap divalidasi (validasi selalu sebelum jalur kosong) agar kontrak konsisten.
- Keterbatasan yang diterima: bila 50 hasil teratas provider seluruhnya sudah anggota, hasil bisa kurang dari `limit` padahal ada kandidat lain di urutan berikutnya. Mitigasi: user mengetik lebih spesifik. Untuk skala puluhan developer ini wajar; tidak ada paginasi atau pemanggilan berulang (diputuskan di tugas: Limit 50 sekali).
- Hasil hanya saran. Race (user dinonaktifkan atau ditambahkan orang lain setelah pencarian) ditangani `addmember` (`USER_INACTIVE`, `ALREADY_MEMBER`). Tidak ada validasi ulang di sini.
- Anggota yang nonaktif tidak muncul karena provider hanya mengembalikan developer aktif; anggota nonaktif juga ada di himpunan anggota sehingga tidak relevan.
- Error bus/DB dibungkus (`fmt.Errorf("searchmembercandidates: <langkah>: %w", err)`) dan dijawab 500 generik. Kegagalan `find-developers` TIDAK diubah menjadi daftar kosong (UI tidak boleh menyimpulkan "tidak ada kandidat"). `bus.ErrNotRegistered` ikut jadi 500 (kesalahan wiring).
- Hanya tiga field per kandidat (`id`, `name`, `email`). Tidak ada role, status, atau hash. `Cache-Control: no-store` dipasang di 200.
- Log: tidak pernah me-log `q` (bisa berisi potongan email). Boleh log `project_id`, `feature`, dan `result_count` pada level Debug.
- Tanpa audit (D2): pencarian adalah aksi baca.
- Usecase hanya orkestrasi; rule akses di `base`, SQL di `base/store`, bus di `base/users`. Normalisasi `q`/`limit` adalah sanitasi parameter (`params.go`), bukan rule bisnis entity.

## Plan

Urutan wajib. Tulis test dulu (TDD), lalu implementasi.

**B0. Verifikasi prasyarat (tanpa edit).** Pastikan simbol F di atas ada. Baca signature `respond.WriteJSON`, `respond.WriteError`, `middleware.RequireAuth()` di kode T00, dan pola `Init` feature gelombang A/B yang sudah ada (`getproject`, `listmembers`); ikuti persis. Pastikan `base.CandidateDTO` ber-tag json `id`, `name`, `email` (bila tidak, laporkan ke F; jangan menambah tipe lokal untuk response). Pastikan `base/users.BusLookup.FindDevelopers` meneruskan `finddevelopers.Request{Query, Limit}`.

**B1. `searchmembercandidates/params.go`** (sanitasi parameter murni, tanpa I/O)

```go
package searchmembercandidates

const (
    defaultLimit = 10
    maxLimit     = 50  // sama dengan finddevelopers.MaxLimit
    maxQueryRunes = 100
)

type query struct {
    Q     string
    Limit int
}

// parseQuery: q di-trim (maks 100 rune), limit kosong -> 10, integer 1..50.
// Kesalahan digabung ke base.FieldErrors lalu dibungkus base.InvalidFields.
func parseQuery(rawQ, rawLimit string) (query, error) {
    fe := base.FieldErrors{}
    q := strings.TrimSpace(rawQ)
    if utf8.RuneCountInString(q) > maxQueryRunes {
        fe["q"] = "q maksimal 100 karakter"
    }
    limit := defaultLimit
    if s := strings.TrimSpace(rawLimit); s != "" {
        n, err := strconv.Atoi(s)
        if err != nil || n < 1 || n > maxLimit {
            fe["limit"] = "limit harus 1-50"
        } else {
            limit = n
        }
    }
    if len(fe) > 0 {
        return query{}, base.InvalidFields(fe)
    }
    return query{Q: q, Limit: limit}, nil
}
```

**B2. `searchmembercandidates/repository.go`** (feature ini tidak punya SQL khusus; hanya mendelegasikan ke `store`)

```go
package searchmembercandidates

type Repo struct{ s *store.Store }

func newRepo(s *store.Store) *Repo { return &Repo{s: s} }

func (r *Repo) FindAccess(ctx context.Context, projectID, userID string) (base.Project, string, bool, error) {
    return r.s.FindAccess(ctx, projectID, userID)
}

func (r *Repo) ListMembers(ctx context.Context, projectID string) ([]base.Member, error) {
    return r.s.ListMembers(ctx, projectID)
}
```

Repository ini boleh dihapus dan `*store.Store` dipakai langsung sebagai implementasi interface usecase bila pola feature lain di `main` melakukannya; ikuti pola yang sudah ada. Tidak ada transaksi dan tidak ada `FOR UPDATE`.

**B3. `searchmembercandidates/usecase.go`**

```go
type repository interface {
    FindAccess(ctx context.Context, projectID, userID string) (base.Project, string, bool, error)
    ListMembers(ctx context.Context, projectID string) ([]base.Member, error)
}

type Input struct {
    ProjectID string
    RawQ      string
    RawLimit  string
}

type Usecase struct {
    repo  repository
    users users.Lookup
}

func NewUsecase(repo repository, u users.Lookup) *Usecase { return &Usecase{repo: repo, users: u} }

func (u *Usecase) Execute(ctx context.Context, in Input) ([]base.CandidateDTO, error) {
    actor, ok := base.ActorFrom(reqctx.IdentityFrom(ctx))
    if !ok {
        return nil, errs.New(errs.CodeUnauthorized, "perlu login")
    }
    p, memberRole, found, err := u.repo.FindAccess(ctx, in.ProjectID, actor.ID)
    if err != nil {
        return nil, fmt.Errorf("searchmembercandidates: find access: %w", err)
    }
    if !found {
        return nil, base.ErrProjectNotFound
    }
    level := base.ResolveLevel(actor.Role, memberRole, true)
    if err := base.Require(level, base.PermManageMembers); err != nil {
        return nil, err
    }
    qry, err := parseQuery(in.RawQ, in.RawLimit)
    if err != nil {
        return nil, err
    }
    if qry.Q == "" {
        return []base.CandidateDTO{}, nil // tanpa ListMembers, tanpa bus
    }
    members, err := u.repo.ListMembers(ctx, p.ID)
    if err != nil {
        return nil, fmt.Errorf("searchmembercandidates: list members: %w", err)
    }
    isMember := make(map[string]struct{}, len(members))
    for _, m := range members {
        isMember[m.UserID] = struct{}{}
    }
    devs, err := u.users.FindDevelopers(ctx, qry.Q, finddevelopers.MaxLimit)
    if err != nil {
        return nil, fmt.Errorf("searchmembercandidates: find developers: %w", err)
    }
    out := make([]base.CandidateDTO, 0, qry.Limit)
    for _, d := range devs {
        if _, skip := isMember[d.ID]; skip {
            continue
        }
        out = append(out, base.CandidateDTO{ID: d.ID, Name: d.Name, Email: d.Email})
        if len(out) == qry.Limit {
            break
        }
    }
    return out, nil
}
```

Sesuaikan signature `reqctx.IdentityFrom` dengan T00. Slice hasil tidak pernah `nil`. Filter anggota dibandingkan sebagai string id (kedua sisi uuid kanonik huruf kecil dari Postgres/T01).

**B4. `searchmembercandidates/handler.go`**

```go
type Handler struct{ uc *Usecase }

func newHandler(uc *Usecase) *Handler { return &Handler{uc: uc} }

type response struct {
    Items []base.CandidateDTO `json:"items"`
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    items, err := h.uc.Execute(r.Context(), Input{
        ProjectID: chi.URLParam(r, "projectID"),
        RawQ:      r.URL.Query().Get("q"),
        RawLimit:  r.URL.Query().Get("limit"),
    })
    if err != nil {
        respond.WriteError(w, r, err)
        return
    }
    w.Header().Set("Cache-Control", "no-store")
    respond.WriteJSON(w, http.StatusOK, response{Items: items})
}
```

Tidak ada `RequestMeta` (tanpa audit), tidak ada decode body (body diabaikan).

**B5. `searchmembercandidates/init.go`**

```go
func Init(d app.Deps) {
    s := store.New(d.DB)
    uc := NewUsecase(newRepo(s), users.New(d.Bus))
    d.API.With(middleware.RequireAuth()).Get("/projects/{projectID}/member-candidates", newHandler(uc).ServeHTTP)
}
```

Nama param WAJIB `{projectID}` (D4). Route didaftarkan dengan pola penuh langsung di `d.API`.

**B6. Wiring.** Tambah satu baris `searchmembercandidates.Init(d)` di `management/base.go` dalam `Register` (satu baris, tanpa mengubah baris lain).

**B7. OpenAPI.** Seharusnya lengkap dari F. Verifikasi `api/modules/project/management.yml`: `GET /api/projects/{projectID}/member-candidates`, `operationId: searchMemberCandidates`, tag `project-members`, parameter `$ref ProjectID`, `Q` (`maxLength: 100`), dan `limit` (integer, min 1, max 50, default 10); respons 200 `MemberCandidateList`, 400 `BadRequest`, 401 `Unauthorized`, 403 `Forbidden` (member), 404 `NotFound` ("juga untuk project yang bukan milik pemanggil; tidak bocor"), 500 `Internal`. Tanpa `X-Requested-With` (GET). Bila ada perubahan: `make api-bundle`, lalu `make api-check`.

**B8. Test** (bagian Test Scenario), lalu `go vet ./...`, `go test ./internal/domain/project/...`, `go test -tags integration ./internal/domain/project/internal/management/searchmembercandidates/...`, dan archtest `TestT00_AC4_RepoIsClean`.

## Migration

Tidak ada. Feature hanya membaca `projects` dan `project_members` dari `backend/migrations/project/20261022090000000_create_projects.sql` (milik F) dan memanggil bus `core.auth.find-developers` (membaca `users` di module `core`, bukan lewat SQL).

## Changes

Backend (baru):
- `backend/internal/domain/project/internal/management/searchmembercandidates/init.go`
- `backend/internal/domain/project/internal/management/searchmembercandidates/handler.go`
- `backend/internal/domain/project/internal/management/searchmembercandidates/params.go`
- `backend/internal/domain/project/internal/management/searchmembercandidates/usecase.go`
- `backend/internal/domain/project/internal/management/searchmembercandidates/repository.go`
- `backend/internal/domain/project/internal/management/searchmembercandidates/params_test.go`
- `backend/internal/domain/project/internal/management/searchmembercandidates/usecase_test.go`
- `backend/internal/domain/project/internal/management/searchmembercandidates/handler_test.go`
- `backend/internal/domain/project/internal/management/searchmembercandidates/integration_test.go` (tag `integration`)

Backend (ubah, minimal):
- `backend/internal/domain/project/internal/management/base.go`: satu baris `searchmembercandidates.Init(d)`.

API:
- `api/modules/project/management.yml`: verifikasi/lengkapi `searchMemberCandidates`; regenerasi `api/openapi.yml` hanya bila berubah.

Tidak dibuat: migrasi, kontrak bus baru, konstanta baru, entri audit, kode error baru, perubahan i18n (dimiliki F/frontend-plan).

Folder `searchmembercandidates/` hanya meng-import `base`, `base/store`, `base/users`, `lib/contracts/core/auth/finddevelopers` (hanya konstanta `MaxLimit`; kontrak boleh diimpor, rule 4.5), `lib/common/{errs,reqctx}`, `infra/{app,http/respond,middleware}`, `chi`. Tidak meng-import feature lain (rule 4.3) dan tidak memanggil `bus` langsung (lewat `base/users`).

## Test Scenario

Penamaan AC: `TestT03_ACn_<Deskripsi>`; lainnya `Test<Fungsi>_<Kasus>`.

### Unit `params.go` (murni, tanpa mock)

- `TestParseQuery_Defaults`: `("", "")` -> `Q=""`, `Limit=10`; `("  bud  ", "")` -> `Q="bud"`.
- `TestParseQuery_LimitBoundaries`: `"1"` dan `"50"` lulus; `"0"`, `"51"`, `"-1"`, `"abc"`, `"1.5"`, `"10x"` -> `InvalidFields` dengan `fields.limit = "limit harus 1-50"`.
- `TestParseQuery_QLength`: 100 rune multibyte lulus; 101 rune -> `fields.q = "q maksimal 100 karakter"`; spasi di ujung tidak dihitung (di-trim lebih dulu).
- `TestParseQuery_BothInvalidMerged`: `q` 101 rune dan `limit=0` -> satu error dengan kedua key.
- `TestParseQuery_SpaceOnlyLimitIsDefault`: `rawLimit="  "` -> 10.

### Unit usecase (repository dan `users.Lookup` palsu lewat interface)

Identitas disuntikkan dengan `reqctx.WithIdentity`.
- `TestUsecase_Execute_OwnerGetsCandidates`: aktor developer `owner`; provider mengembalikan 3 developer, 1 sudah anggota -> 2 kandidat tanpa anggota, urutan provider terjaga.
- `TestUsecase_Execute_SuperAdminAllowed`: super_admin bukan anggota (`memberRole=""`) -> sukses.
- `TestUsecase_Execute_MemberForbidden`: `memberRole=member` -> `base.ErrForbidden`; `ListMembers` dan `FindDevelopers` TIDAK dipanggil (AC2).
- `TestUsecase_Execute_NonMemberIs404`: developer bukan anggota -> `base.ErrProjectNotFound`; tidak ada panggilan lain.
- `TestUsecase_Execute_NotFound`: `found=false` -> `base.ErrProjectNotFound` (pointer sama dengan kasus non-anggota).
- `TestUsecase_Execute_AuthBeforeValidation`: non-anggota dengan `limit=999` -> 404 (bukan 400); member dengan `limit=999` -> 403; owner dengan `limit=999` -> 400.
- `TestUsecase_Execute_EmptyQueryNoBus`: `q=""` dan `q="   "` -> slice non-nil kosong; `ListMembers` dan `FindDevelopers` 0 panggilan.
- `TestUsecase_Execute_PassesTrimmedQueryAndMax50`: `q="  bud "` diteruskan sebagai `"bud"` dan `limit` pemanggilan provider selalu 50, apa pun `limit` klien.
- `TestUsecase_Execute_TruncatesToLimit`: provider 50 hasil, tidak ada anggota, `limit=5` -> 5 item pertama.
- `TestUsecase_Execute_DefaultLimit10`: `limit` kosong dan provider 50 hasil -> 10 item.
- `TestUsecase_Execute_FiltersMembersBeforeTruncate`: 5 hasil teratas adalah anggota, `limit=3` -> 3 kandidat non-anggota berikutnya (bukan 0).
- `TestUsecase_Execute_AllMembersReturnsEmpty`: semua hasil provider anggota -> `[]` non-nil, tanpa error.
- `TestUsecase_Execute_MapsOnlyThreeFields`: `CandidateDTO` hanya memuat `ID`, `Name`, `Email` (reflect jumlah field = 3).
- `TestUsecase_Execute_NoIdentity`: konteks tanpa identitas -> `CodeUnauthorized`, repo tidak dipanggil.
- `TestUsecase_Execute_FindAccessError`: error repo dibungkus (`errors.Is` ke asal), bukan `*errs.Error`.
- `TestUsecase_Execute_ListMembersError`: error dibungkus; `FindDevelopers` tidak dipanggil.
- `TestUsecase_Execute_FindDevelopersError`: error bus dibungkus (termasuk `errors.Is(err, bus.ErrNotRegistered)` tetap benar), BUKAN daftar kosong.
- `TestUsecase_Execute_NoAuditAndNoGetUser`: lookup palsu menghitung `Get` = 0.

### Unit handler (`httptest`, usecase dengan repo/lookup palsu, router chi nyata)

- `TestHandler_Get_OK`: 200, `Content-Type: application/json`, `Cache-Control: no-store`, body `{"items":[{"id","name","email"}]}`.
- `TestHandler_Get_EmptyItemsIsArray`: hasil kosong -> `{"items":[]}` (bukan `null`).
- `TestHandler_Get_PassesRawParams`: `q` dan `limit` mentah diteruskan apa adanya; `projectID="bukan-uuid"` diteruskan tanpa validasi di handler.
- `TestHandler_Get_BadLimit400Body`: `limit=0` (owner) -> 400 `INVALID_ARGUMENT` dengan `details.fields.limit`.
- `TestHandler_Get_Forbidden403Body`: member -> 403 `{"error":{"code":"FORBIDDEN","message":"akses ditolak","details":null},...}`.
- `TestHandler_Get_NotFound404Body`: -> 404 `{"error":{"code":"NOT_FOUND","message":"project tidak ditemukan","details":null},...}`.
- `TestHandler_Get_InternalIs500Generic`: error infra -> 500 tanpa membocorkan pesan asal.
- `TestHandler_Get_IgnoresBody`: body sembarang tidak berpengaruh.

### Integrasi (tag `integration`, `db.TestDB(t)`, bus nyata, handler palsu `find-developers`)

Fixture: developer aktif A (owner), B (member), C ("Budiman"), D ("Budi Santoso"), E ("Bud Nonmember"); S super_admin; handler palsu `core.auth.find-developers` yang memfilter daftar tetap berdasarkan substring dan menghormati `Limit`. `core.audit.record` palsu menghitung panggilan.
- `TestRepo_ListMembers_ForCandidates`: project dengan A (owner) dan B (member) -> dua `user_id`; project lain tidak ikut.
- `TestT03_AC2_SearchCandidates_MemberGets403`: B (member) `GET .../member-candidates?q=bud` -> 403; handler `find-developers` palsu 0 panggilan.
- `TestT03_SearchCandidates_OwnerExcludesExistingMembers`: A owner, B member; `q=b` -> hasil memuat C, D, E dan TIDAK memuat A maupun B.
- `TestT03_SearchCandidates_SuperAdminSees`: S (bukan anggota) -> 200 dengan kandidat.
- `TestT03_SearchCandidates_AfterAddMemberDisappears`: C belum anggota -> muncul; INSERT C sebagai anggota -> permintaan berikutnya tidak memuat C (tanpa cache).
- `TestT03_SearchCandidates_RespectsLimit`: `limit=2` -> tepat 2; `limit=50` valid; `limit=51` -> 400.
- `TestT03_SearchCandidates_EmptyQuery`: `q` kosong atau spasi -> `{"items":[]}`; handler `find-developers` palsu 0 panggilan.
- `TestT03_SearchCandidates_ProviderCalledWithMax50`: handler palsu mencatat `Limit==50` dan `Query` ter-trim.
- `TestT03_SearchCandidates_ProviderMissing`: `find-developers` tidak terdaftar (`ErrNotRegistered`) -> 500 generik (bukan 200 kosong).
- `TestT03_SearchCandidates_ProviderError`: handler palsu mengembalikan error -> 500 generik.
- `TestT03_SearchCandidates_NoAudit`: audit palsu 0 panggilan pada sukses maupun gagal.
- `TestT03_Security_SearchCandidates_Isolation` (rule 13.6): developer bukan anggota (X) -> 404 untuk project A; body identik (kode, pesan, details) dengan project uuid acak dan `bukan-uuid`; X dengan `limit=999` tetap 404; handler `find-developers` palsu 0 panggilan; hasil tidak pernah memuat hash atau role.
- `TestT03_SearchCandidates_Unauthenticated`: tanpa sesi -> 401 dari `RequireAuth`.

## E2E Scenarios

Butuh T01 dan T02 merge (Playwright terhadap binary).

Positif:
1. Owner A membuka tab Members -> "Tambah anggota", mengetik "bud" -> daftar menampilkan developer aktif yang namanya/emailnya mengandung "bud" dan belum anggota; memilih "Budiman" lalu mengonfirmasi menambahkannya sebagai member.
2. Setelah Budiman ditambahkan, dialog dibuka lagi dan mengetik "bud" -> Budiman tidak muncul lagi.
3. Super Admin membuka project yang bukan miliknya dan mengetik di dialog tambah anggota -> kandidat tampil (untuk menunjuk owner pertama pada project tanpa owner).
4. Kolom pencarian dikosongkan -> daftar kandidat dikosongkan tanpa request error (UI tidak menampilkan semua developer).
5. Mengetik cepat beberapa karakter -> UI men-debounce dan hanya hasil request terakhir yang tampil.

Negatif:
1. Member project membuka dialog tambah anggota lewat URL/state paksa atau memanggil API langsung -> 403; UI menampilkan pesan akses ditolak (tombol tambah anggota memang disembunyikan untuk member).
2. Developer C membuka `/api/projects/<id PA>/member-candidates?q=a` langsung -> 404 yang sama dengan project yang tidak ada.
3. `GET .../member-candidates?q=a&limit=100` oleh owner -> 400 `fields.limit`.
4. Query 101 karakter -> 400 `fields.q`.
5. Query `%`, `_`, atau `\` -> diperlakukan harfiah (tidak mengembalikan semua developer).
6. Developer nonaktif atau Super Admin tidak pernah muncul sebagai kandidat; bila dipaksa ditambahkan lewat API `addmember` -> 400 `USER_INACTIVE` / `USER_NOT_DEVELOPER`.
7. Tanpa login -> 401 dan UI mengarahkan ke login.
8. Kegagalan layanan pencarian -> UI menampilkan pesan error generik di dropdown, bukan "tidak ada hasil".

## Request/Response body

Request: tanpa body. Header `Cookie` sesi. Tanpa `X-Requested-With` (GET). Query: `q` (opsional, maks 100 karakter), `limit` (opsional, 1-50, default 10).

```
GET /api/projects/0192f5a0-1111-7aaa-8bbb-000000000001/member-candidates?q=bud&limit=10
```

### 200 OK (ada hasil)

Header: `Content-Type: application/json`, `Cache-Control: no-store`.

```json
{
  "items": [
    { "id": "0192f5a0-cccc-7aaa-8bbb-00000000000c", "name": "Budiman", "email": "budiman@tim.id" },
    { "id": "0192f5a0-dddd-7aaa-8bbb-00000000000d", "name": "Budi Santoso", "email": "budi@tim.id" }
  ]
}
```

### 200 OK (q kosong, tidak ada hasil, atau semua sudah anggota)

```json
{ "items": [] }
```

### 400 limit atau q tidak valid (hanya untuk owner/admin; muncul setelah otorisasi)

`?q=bud&limit=100`:

```json
{
  "error": {
    "code": "INVALID_ARGUMENT",
    "message": "permintaan tidak valid",
    "details": { "fields": { "limit": "limit harus 1-50" } }
  },
  "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A"
}
```

`q` lebih dari 100 karakter:

```json
{
  "error": {
    "code": "INVALID_ARGUMENT",
    "message": "permintaan tidak valid",
    "details": { "fields": { "q": "q maksimal 100 karakter" } }
  },
  "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A"
}
```

### 401 tanpa sesi

```json
{ "error": { "code": "UNAUTHENTICATED", "message": "perlu login", "details": null }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

(Kode dan pesan persis mengikuti `RequireAuth` T00; tabel di atas hanya ilustrasi bentuk.)

### 403 member

```json
{ "error": { "code": "FORBIDDEN", "message": "akses ditolak", "details": null }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

### 404 bukan anggota, project tidak ada, atau id bukan uuid

```json
{ "error": { "code": "NOT_FOUND", "message": "project tidak ditemukan", "details": null }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

### 500 (DB atau `find-developers` gagal)

```json
{ "error": { "code": "INTERNAL", "message": "terjadi kesalahan pada server", "details": null }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

(Kode dan pesan 500 persis mengikuti `respond.WriteError` T00.)

### Bus yang dipanggil (Go, bukan JSON di wire)

```go
devs, err := bus.CallTyped[finddevelopers.Response](ctx, d.Bus, constants.BUS_CORE_AUTH_FIND_DEVELOPERS,
    finddevelopers.Request{Query: "bud", Limit: 50}) // Limit selalu finddevelopers.MaxLimit
// devs.Items: [{ID, Name, Email}, ...] urut name, id; anggota dibuang di usecase, lalu dipotong ke limit klien.
```

## Definisi selesai

- Semua test di atas hijau (`-race`); `go vet`, archtest `TestT00_AC4_RepoIsClean` lulus.
- Owner dan Super Admin mendapat developer aktif non-anggota; member 403; non-anggota/tidak ada/bukan uuid 404 seragam; validasi param hanya setelah otorisasi.
- `q` kosong tidak memanggil bus; `find-developers` selalu dipanggil dengan Limit 50; hasil dipotong ke `limit` (default 10, maks 50); kegagalan provider menjadi 500, bukan daftar kosong.
- Tanpa audit, tanpa migrasi, tanpa perubahan di luar allowedPaths selain satu baris di `management/base.go`.
