# T04 project/api-key — Usecase `listkeys`

Mengikuti `docs/superpowers/plans/project-api-key/overview.md` (overview menang bila bertentangan, kecuali satu penyempurnaan yang dicatat eksplisit di bagian 2, "Penyimpangan terhadap overview"). Kontrak bus yang dikonsumsi (`project.management.check-access`) persis dari plan T03 (`overview.md` T03 bagian 5.2); tidak didefinisikan ulang di sini.

## 1. Metadata

| Atribut | Nilai |
|---|---|
| Ticket | T04 |
| Scope | fullstack pada tiket; plan ini backend saja. Tab "API Keys" (tabel daftar) dirinci di `frontend-plan.md` |
| Domain | project |
| Module | api-key (`apikey`) |
| Feature | `listkeys` (gelombang paralel U5) |
| Endpoint | `GET /api/projects/{projectID}/api-keys` |
| Method | GET |
| Version | tanpa prefix versi di URL (prefix `/api`); `api/modules/project/api-key.yml` `info.version: 0.1.0` |
| Auth | `middleware.RequireAuth()`; otorisasi project di usecase lewat `base/access.Require` (anggota member/owner, super_admin) |
| Resource root | `backend/internal/domain/project/internal/apikey` |
| Folder feature | `backend/internal/domain/project/internal/apikey/listkeys/` |
| Konsumsi bus | `project.management.check-access` (Call, T03) via `base/access`, fail-closed (error -> 500) |
| Audit | tidak ada (aksi baca) |
| Tabel / migrasi | membaca `api_keys`; tidak ada migrasi baru |
| AC | AC1 (anggota boleh, bukan anggota 404), AC2 (respons tidak memuat plaintext/hash); bagian dari `TestT04_Security_ProjectIsolation` |

## 2. Summary Plan

`listkeys` mengembalikan seluruh API key sebuah project, baik yang aktif maupun yang sudah dicabut, tanpa paginasi, tanpa pernah memuat raw key atau hash.

Alur usecase (orkestrasi saja, satu usecase, tidak dipecah):

1. Bangun aktor: `actor, ok := base.ActorFrom(reqctx.IdentityFrom(ctx))`; `!ok` -> `errs.New(errs.CodeUnauthorized, "perlu login")`.
2. Otorisasi (overview 3.4): `uc.access.Require(ctx, projectID, actor)`. Error bus -> 500; `!ProjectExists` atau `Level == none` (termasuk `projectID` bukan uuid) -> `base.ErrProjectNotFound` (404 `project tidak ditemukan`); `member`/`owner`/`admin` -> lanjut.
3. `uc.repo.ListByProject(ctx, projectID)` -> `[]base.Key`, satu SELECT: kolom TANPA `secret_hash`, `WHERE project_id = $1 ORDER BY (revoked_at IS NULL) DESC, created_at DESC, id`.
4. Petakan ke `[]base.KeyDTO` lewat `Key.ToDTO()` (entity, sudah menetapkan `status` active/revoked dan waktu UTC). Kembalikan `base.KeyList{Items}` dengan `Items` selalu slice non-nil (`[]`, bukan `null`).
5. Handler menulis 200 `{ "items": [...] }`.

Keputusan dan batasan:
- Tanpa paginasi, tanpa filter, tanpa audit, tanpa cache. Jumlah baris dibatasi praktis oleh batas key aktif (default 20) ditambah key dicabut yang tetap tampil (tiket: tidak ada hapus).
- Key dicabut SELALU ikut (status `revoked`, `revoked_at`, `revoked_by`, `revoked_by_label`), diurutkan setelah semua key aktif.
- Urutan ditegakkan di SQL, bukan di Go; tiebreak `id` menjadikan urutan deterministik bila `created_at` kembar. Tidak ada logika sorting di usecase.
- Tidak ada rule bisnis baru; pemetaan DTO dan status sudah ada di `base` (foundation).
- Label aktor (`created_by_label`, `revoked_by_label`) diambil dari snapshot kolom (K2); tidak ada panggilan `core.auth.get-user`.
- Isolasi project: query selalu memfilter `project_id`; key project lain tidak mungkin ikut.
- Penyimpangan terhadap overview (disengaja, sesuai spesifikasi usecase ini): overview bagian 9 menyebut `listkeys` men-scan `KeyColumns` lengkap termasuk `secret_hash`. Di plan ini repository MEMILIH kolom tanpa `secret_hash` (konstanta lokal `listColumns` di `repository.go` = `store.KeyColumns` dikurangi `secret_hash`), sehingga hash tidak pernah keluar dari DB pada jalur baca daftar (defense in depth). Tetap memakai satu jalur scan bersama `store.SelectKeys`: `keyRow` memakai `sqlx` yang menoleransi kolom lebih sedikit dari field struct; `Key.SecretHash` hasilnya `nil`. Entity `ToDTO` tidak membaca `SecretHash`, jadi tidak ada efek samping. Bila saat implementasi `store.SelectKeys` terbukti gagal untuk kolom yang kurang, perbaiki di `base/store` (foundation, dengan test) dan JANGAN kembali ke `SELECT` penuh. Setelah plan ini diimplementasi, perbarui kalimat terakhir overview bagian 9 agar konsisten.

Prasyarat dari foundation F (overview bagian 7). Plan ini TIDAK membuatnya; bila belum ada di branch, selesaikan F dulu dan jangan duplikasi:
- `base`: `Actor`, `ActorFrom`, `Key`, `Key.ToDTO`, `KeyDTO`, `KeyList`, `ErrProjectNotFound`.
- `base/store`: `New`, `Store.SelectKeys`, `ValidUUID`, konstanta `KeyColumns`.
- `base/access`: `Checker`, `New(b)`.
- Kontrak T03 `lib/contracts/project/management/checkaccess` dan konstanta `constants.BUS_PROJECT_MANAGEMENT_CHECK_ACCESS`.
- Migrasi `backend/migrations/project/20261023090000000_create_api_keys.sql` (index `api_keys_project_idx`).
- Path dan schema `GET /api/projects/{projectID}/api-keys` di `api/modules/project/api-key.yml` (F langkah 10).

## 3. Plan (langkah berurutan)

TDD: tulis test (bagian 7) lebih dulu per langkah.

1. **Verifikasi prasyarat** (tanpa mengubahnya). Baca `backend/internal/infra/app` (`Deps`), `infra/http/respond` (`WriteJSON`, `WriteError`), `infra/middleware` (`RequireAuth`), dan pola `Init` feature HTTP yang sudah ada di `main` (mis. feature T01/T03) agar bentuk `Get(...)` sama. Pastikan simbol foundation di atas ada.

2. **`listkeys/repository.go`** (package `listkeys`). Satu-satunya yang menyentuh SQL feature ini:

   ```go
   // listColumns = store.KeyColumns tanpa secret_hash. Hash tidak pernah dibaca pada jalur daftar.
   const listColumns = "id, project_id, name, prefix, created_by, created_by_label, created_at, " +
       "last_used_at, revoked_at, revoked_by, revoked_by_label"

   const listByProjectSQL = `SELECT ` + listColumns + `
   FROM api_keys
   WHERE project_id = $1
   ORDER BY (revoked_at IS NULL) DESC, created_at DESC, id`

   type Repo struct{ s *store.Store }

   func newRepo(s *store.Store) *Repo

   // ListByProject mengembalikan semua key (aktif dulu). Hasil kosong -> slice kosong non-nil.
   // projectID bukan uuid -> (kosong, nil) tanpa query (pertahanan berlapis; access.Require sudah menolak 404 lebih dulu).
   func (r *Repo) ListByProject(ctx context.Context, projectID string) ([]base.Key, error)
   ```

   `ListByProject`: `if !store.ValidUUID(projectID) { return []base.Key{}, nil }`; selain itu `r.s.SelectKeys(ctx, listByProjectSQL, projectID)`, error dibungkus `fmt.Errorf("listkeys: select keys: %w", err)`. Tanpa transaksi (read-only, satu statement).

3. **`listkeys/usecase.go`**:

   ```go
   type KeyLister interface {
       ListByProject(ctx context.Context, projectID string) ([]base.Key, error)
   }

   type Usecase struct {
       access access.Checker
       repo   KeyLister
   }

   func NewUsecase(a access.Checker, r KeyLister) *Usecase

   func (u *Usecase) Execute(ctx context.Context, projectID string) (base.KeyList, error)
   ```

   Alur `Execute`, berurutan:
   1. `actor, ok := base.ActorFrom(reqctx.IdentityFrom(ctx))`; `!ok` -> `base.KeyList{}, errs.New(errs.CodeUnauthorized, "perlu login")`.
   2. `if err := u.access.Require(ctx, projectID, actor); err != nil { return base.KeyList{}, err }` (error dikembalikan apa adanya; `Require` sudah membungkus error infra).
   3. `keys, err := u.repo.ListByProject(ctx, projectID)`; err -> `base.KeyList{}, fmt.Errorf("listkeys: list: %w", err)`.
   4. `items := make([]base.KeyDTO, 0, len(keys))`; tiap `k` -> `items = append(items, k.ToDTO())`.
   5. `return base.KeyList{Items: items}, nil`.

   Usecase menerima interface (`access.Checker`, `KeyLister`), bukan tipe konkret (rule 5.6). Tidak membangun SQL, tidak membuka transaksi, tidak memanggil bus selain lewat `access.Checker`, tidak mengurutkan ulang.

4. **`listkeys/handler.go`**:

   ```go
   type handler struct{ uc *Usecase }

   func newHandler(uc *Usecase) *handler

   func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
       res, err := h.uc.Execute(r.Context(), chi.URLParam(r, "projectID"))
       if err != nil {
           respond.WriteError(w, r, err)
           return
       }
       respond.WriteJSON(w, http.StatusOK, res) // base.KeyList -> {"items":[...]}
   }
   ```

   Tanpa `params.go`: tidak ada body dan tidak ada query param; input satu-satunya adalah path param `projectID` yang diteruskan apa adanya (overview 3.3, validasi uuid di `check-access`). Query string diabaikan. Tidak ada header `Cache-Control: no-store` (respons tidak memuat rahasia).

5. **`listkeys/init.go`**:

   ```go
   func Init(d app.Deps) {
       uc := NewUsecase(access.New(d.Bus), newRepo(store.New(d.DB)))
       d.API.With(middleware.RequireAuth()).Get("/projects/{projectID}/api-keys", newHandler(uc).ServeHTTP)
   }
   ```

   Pola route penuh langsung di `d.API` (K4), nama param `{projectID}` (jangan `Mount`/`Route` pada `/projects`). Sesuaikan bentuk `Get(...)` dengan feature HTTP lain di `main`. CSRF T00 hanya menolak method mutasi, jadi GET tidak butuh `X-Requested-With`.

6. **Wiring**: tambah satu baris `listkeys.Init(d)` di `backend/internal/domain/project/internal/apikey/base.go` (fungsi `Register`). Hanya satu baris tambahan, tanpa mengubah baris lain (minimal konflik merge dengan usecase paralel).

7. **OpenAPI**: verifikasi operasi `listkeys` di `api/modules/project/api-key.yml` sesuai bagian 8 (operationId `listkeys`, 200 `ApiKeyList`, 401, 404). Perbaiki hanya bagian operasi ini bila ada selisih, lalu `make api-bundle`, commit `api/openapi.yml`, `make api-check` lulus. Tidak ada path baru di `api/root.yml`.

8. **Test** (bagian 7): `usecase_test.go`, `handler_test.go`, `repository_integration_test.go`, `listkeys_integration_test.go` (dua terakhir tag `integration`).

9. **Verifikasi**: `cd backend && go vet ./... && go test ./internal/domain/project/... && go test -tags integration ./internal/domain/project/internal/apikey/listkeys/...`; `make api-check`; archtest T00 (`TestT00_AC4_RepoIsClean`) lulus (feature tidak meng-import feature lain, tidak ada SQL di luar repository, tidak ada import module/domain lain).

Kontrak bus yang dikonsumsi (persis plan T03, tidak didefinisikan ulang):

```go
bus.CallTyped[checkaccess.Response](ctx, b, constants.BUS_PROJECT_MANAGEMENT_CHECK_ACCESS,
    checkaccess.Request{ProjectID: projectID, UserID: actor.ID, UserRole: actor.Role})
// checkaccess.Response{ProjectExists bool; Level Level} dengan Level = LevelNone | LevelMember | LevelOwner | LevelAdmin
// projectID bukan uuid -> ProjectExists=false, err nil.
```

Dipanggil lewat `access.Checker.Require` (foundation). Feature ini tidak menyediakan endpoint bus.

## 4. Migration

Tidak ada. Memakai tabel `api_keys` dari `backend/migrations/project/20261023090000000_create_api_keys.sql` (foundation). Hanya BACA.

Query (milik `listkeys/repository.go`):

```sql
SELECT id, project_id, name, prefix, created_by, created_by_label, created_at,
       last_used_at, revoked_at, revoked_by, revoked_by_label
FROM api_keys
WHERE project_id = $1
ORDER BY (revoked_at IS NULL) DESC, created_at DESC, id
```

Terlayani `api_keys_project_idx (project_id, created_at DESC)` untuk filter project; urutan tambahan `(revoked_at IS NULL)` dan `id` dihitung di memori untuk puluhan baris, wajar. Tidak butuh index baru.

## 5. Changes

Baru:
- `backend/internal/domain/project/internal/apikey/listkeys/init.go`
- `backend/internal/domain/project/internal/apikey/listkeys/handler.go`
- `backend/internal/domain/project/internal/apikey/listkeys/usecase.go`
- `backend/internal/domain/project/internal/apikey/listkeys/repository.go`
- `backend/internal/domain/project/internal/apikey/listkeys/usecase_test.go`
- `backend/internal/domain/project/internal/apikey/listkeys/handler_test.go`
- `backend/internal/domain/project/internal/apikey/listkeys/repository_integration_test.go`
- `backend/internal/domain/project/internal/apikey/listkeys/listkeys_integration_test.go`

Ubah:
- `backend/internal/domain/project/internal/apikey/base.go`: tambah `listkeys.Init(d)` di `Register`.
- `api/modules/project/api-key.yml` dan `api/openapi.yml`: hanya bila operasi `listkeys` belum sesuai bagian 8.
- `docs/superpowers/plans/project-api-key/overview.md` bagian 9: sesuaikan kalimat tentang `listkeys` men-scan `secret_hash` (lihat penyimpangan di bagian 2). Dilakukan oleh developer pada PR implementasi, bukan oleh pembuat plan ini.

Tidak diubah oleh usecase ini (sudah dari foundation): `base/*` (termasuk `Key.ToDTO`, `KeyList`, `ErrProjectNotFound`), `base/store`, `base/access`, `base/audit`, kontrak, `lib/constants/bus.go`, migrasi, `api/root.yml`, config, i18n. Frontend (tab "API Keys") di `frontend-plan.md`.

Catatan jika ditemukan kekurangan di foundation (mis. `KeyList` memberi `null` untuk slice kosong, atau `SelectKeys` gagal untuk kolom yang kurang): perbaiki di `base/dto.go` / `base/store` beserta unit test-nya sebagai bagian foundation, bukan di dalam feature ini.

## 6. Keamanan dan error

| Kondisi | Hasil |
|---|---|
| Tanpa sesi | 401 (middleware `RequireAuth`) |
| `projectID` bukan uuid / project tidak ada | 404 `NOT_FOUND` "project tidak ditemukan" |
| Developer bukan anggota | 404 yang sama persis (tidak bocor, rule 7.3) |
| Developer anggota (member atau owner) | 200 |
| Super Admin (anggota atau bukan) | 200 |
| `check-access` gagal (error bus, handler tidak terdaftar) | 500 `INTERNAL` (fail-closed), tanpa isi |
| DB gagal | 500 |
| Project tanpa key | 200 `{ "items": [] }` |

Aturan data:
- Respons TIDAK PERNAH memuat raw key, secret, atau hash. `KeyDTO` tidak punya field tersebut, dan SQL tidak membaca `secret_hash`. Test JSON memeriksa string mentah respons.
- Otorisasi tidak memakai input klien selain `projectID`; identitas dari konteks (`reqctx`).
- Log: field standar `feature=listkeys`, `project_id`. Tidak me-log body respons. Tidak ada log sukses per request di usecase.
- Key milik project lain tidak pernah muncul (filter `project_id`), dan project yang tidak boleh dilihat pemanggil selalu 404 seragam.

## 7. Test Scenario

Penamaan: `TestT04_ACn_...` untuk AC; lainnya `Test<Fungsi>_<Kasus>`.

### Unit usecase (`usecase_test.go`, `access.Checker` dan `KeyLister` palsu, hitung panggilan)

Identitas disuntikkan lewat `reqctx.WithIdentity`.

- `TestExecute_NoIdentity`: konteks tanpa identitas -> `CodeUnauthorized`; access dan repo tidak dipanggil.
- `TestExecute_AccessDeniedReturnsProjectNotFound`: `Require` mengembalikan `base.ErrProjectNotFound` -> error yang sama (`errors.Is`), `ListByProject` = 0 panggilan.
- `TestExecute_AccessInfraErrorPropagates`: `Require` mengembalikan error non-`*errs.Error` -> diteruskan (`errors.Is` ke error asal, bukan `*errs.Error` sehingga 500); repo tidak dipanggil.
- `TestExecute_AccessReceivesActorFromIdentity`: palsu `Require` merekam `(projectID, actor)`; harus `Actor{ID: UserID, Role: Role, Label: Email}` dari identitas dan `projectID` persis seperti diberikan.
- `TestT04_AC1_MemberAllowed`: `Require` nil -> sukses; repo dipanggil 1x dengan `projectID` yang sama.
- `TestExecute_MapsEveryKeyToDTO`: 3 key (2 aktif, 1 dicabut) -> 3 `KeyDTO`, urutan input dipertahankan (usecase tidak mengurutkan ulang); `status` `active`/`revoked`; `revoked_at`, `revoked_by`, `revoked_by_label`, `last_used_at` terisi sesuai sumber; waktu UTC.
- `TestExecute_EmptyReturnsEmptySlice`: repo `[]` atau `nil` -> `Items` non-nil, `len==0`; `json.Marshal` menghasilkan `{"items":[]}`.
- `TestExecute_RepoErrorWrapped`: repo error -> `errors.Is` ke error asal, bukan `*errs.Error`; hasil `KeyList{}`.
- `TestExecute_DTOHasNoSecretFields`: key sumber dengan `SecretHash` terisi 32 byte -> `json.Marshal(result)` tidak memuat kunci `secret`, `secret_hash`, `hash`, `raw_key` dan tidak memuat representasi hex/base64 hash.

### Handler (`handler_test.go`, `httptest`, usecase dengan palsu, router `chi` dengan pola `/projects/{projectID}/api-keys`)

- `TestHandler_OK`: 200, `Content-Type: application/json`, body `{"items":[...]}` sesuai snapshot bagian 9 (field lengkap, `last_used_at: null`).
- `TestHandler_EmptyItemsIsArray`: body persis `{"items":[]}`.
- `TestHandler_PassesProjectIDParam`: path `/projects/abc/api-keys` -> usecase menerima `"abc"` apa adanya.
- `TestHandler_ErrorMapping`: `ErrProjectNotFound` -> 404 + body error seragam T00 (`error.code=NOT_FOUND`, `trace_id`); `CodeUnauthorized` -> 401; error biasa -> 500 generik tanpa bocor pesan internal.
- `TestHandler_IgnoresQueryAndBody`: `?limit=1&x=y` dan body acak tidak mengubah hasil.

### Repository (`repository_integration_test.go`, tag `integration`, `db.TestDB(t)`)

Fixture: `INSERT INTO projects (name) VALUES ($1) RETURNING id` untuk P1 dan P2; baris `api_keys` lewat SQL dengan `created_at`, `revoked_at` eksplisit.

- `TestRepo_OrderActiveFirstThenCreatedDescThenID`: P1 berisi: `A` aktif `t=10:00`, `B` aktif `t=11:00`, `C` dicabut `t=12:00`, `D` dicabut `t=08:00`, `E` aktif `t=10:00` (kembar dengan A, id lebih besar dari A). Urutan hasil: `B`, lalu `A`/`E` menurut `id` naik, lalu `C`, lalu `D`.
- `TestRepo_RevokedKeysIncluded`: key dicabut ada di hasil dengan `RevokedAt`, `RevokedBy`, `RevokedByLabel` terisi.
- `TestRepo_FiltersByProject`: key milik P2 tidak muncul saat list P1 (dan sebaliknya).
- `TestRepo_EmptyProject`: project tanpa key -> slice kosong non-nil, tanpa error.
- `TestRepo_NonUUIDProjectIDNoQuery`: `projectID="bukan-uuid"` -> slice kosong, tanpa error (tidak ada error 22P02).
- `TestRepo_DoesNotReadSecretHash`: key dengan `secret_hash` terisi -> `Key.SecretHash` kosong/`nil` pada hasil list; sekaligus membuktikan `SelectKeys` menoleransi kolom yang kurang. Pastikan `listByProjectSQL` tidak mengandung substring `secret_hash` (assert pada konstanta).
- `TestRepo_TimesAreUTC`: `CreatedAt`, `LastUsedAt`, `RevokedAt` ber-`Location` UTC.
- `TestRepo_NullableColumns`: key dengan `created_by`, `created_by_label`, `last_used_at` NULL ter-scan sebagai `nil`.

### Integrasi HTTP end-to-end (`listkeys_integration_test.go`, tag `integration`)

Setup: `db.TestDB(t)`, `bus.NewRegistry`, `chi.NewRouter()` sebagai `d.API` dengan middleware uji penyuntik `reqctx.WithIdentity` dan `RequireAuth` nyata; `listkeys.Init(d)`. Handler bus palsu untuk `project.management.check-access` (peta `(projectID, userID) -> Level`; super_admin -> `admin`; `projectID` bukan uuid -> `ProjectExists=false`). Identitas: A (developer, owner P1), B (developer, member P1), C (developer, bukan anggota), S (super_admin). Project P1 dan P2 lewat SQL fixture; key diisi lewat SQL dengan secret/hash yang diketahui test.

- `TestT04_AC1_MemberOwnerSuperAdminCanList`: A, B, S masing-masing `GET /api/projects/<P1>/api-keys` -> 200.
- `TestT04_AC1_NonMemberGets404`: C -> 404 `NOT_FOUND` "project tidak ditemukan", `details` null; body identik dengan 404 untuk project yang tidak ada (UUID acak).
- `TestListKeys_NonUUIDProjectID404`: `/api/projects/bukan-uuid/api-keys` -> 404 `project tidak ditemukan`.
- `TestListKeys_Unauthenticated401`: tanpa identitas/sesi -> 401.
- `TestT04_AC2_ResponseNeverContainsRawKeyOrHash`: P1 punya key dengan `secret_hash = sha256(secret)` dan raw key `gr_<prefix>_<secret>` yang diketahui test. Baca body MENTAH sebagai string: tidak memuat `secret`, `hash`, `raw_key`, nilai `secret` (43 karakter), raw key lengkap, hex maupun base64 dari `secret_hash`. Decode ke `map[string]any` per item dan assert himpunan kunci item persis: `id, name, prefix, status, created_by, created_by_label, created_at, last_used_at, revoked_at, revoked_by, revoked_by_label`. `prefix` boleh ada.
- `TestListKeys_ActiveFirstThenNewest`: fixture urutan bagian repository -> urutan `id` di JSON sesuai (aktif dulu, `created_at` turun, `id` naik); semua key aktif mendahului semua key dicabut.
- `TestListKeys_RevokedKeyStillListedWithStatus`: key dicabut tampil dengan `status="revoked"`, `revoked_at` ISO-8601 UTC (`...Z`), `revoked_by_label` terisi; key aktif `status="active"`, `revoked_at/revoked_by/revoked_by_label` = `null`.
- `TestListKeys_LastUsedAtShown`: `last_used_at` terisi tampil sebagai ISO-8601 UTC; yang belum pernah dipakai `null`.
- `TestT04_Security_ProjectIsolation` (bagian list, rule 13.6): key P2 tidak muncul di list P1 (cek `id` dan `name`); A (owner P1, bukan anggota P2) memanggil list P2 -> 404; C memanggil list P1 -> 404 seragam.
- `TestListKeys_EmptyProject`: P1 baru tanpa key -> 200 `{"items":[]}` (cek `[]` bukan `null` di JSON mentah).
- `TestListKeys_CheckAccessFailureIs500`: handler `check-access` mengembalikan error (dan variasi: tidak terdaftar) -> 500 `INTERNAL`, tanpa isi daftar, pesan generik.
- `TestListKeys_MembershipRemovedMeansNoAccess`: setelah palsu `check-access` mengubah B menjadi `none`, B mendapat 404 sedangkan A tetap 200 (key tidak terikat pembuat, akses tetap mengikuti keanggotaan saat ini).
- `TestListKeys_NoSecretInLogs`: logger buffer selama request; output tidak memuat secret/hash/raw key (log boleh memuat `project_id`).
- `TestRoutesRegistered` lintas feature (W) sudah mencakup pasangan `GET /api/projects/{projectID}/api-keys`; tidak diulang di sini.

## 8. E2E Scenarios

Playwright terhadap binary (butuh T01, T03, dan frontend T04). Sisi UI dirinci di `frontend-plan.md`; cakupan yang relevan dengan `listkeys`:

Positif:
1. Developer B (member, bukan owner) login -> buka project -> tab "API Keys" project baru -> keadaan kosong ("belum ada key") dan tombol "Buat key".
2. Setelah dua key dibuat (misal "ci-pipeline", "claude-code-budi"), tabel memuat kolom nama, prefix, dibuat oleh (email) dan kapan, terakhir dipakai ("-" bila belum), status "aktif". Key terbaru di atas.
3. Cabut salah satu key -> tabel di-refresh: key dicabut berpindah ke bawah semua key aktif, tampil redup dengan status "dicabut", dicabut oleh dan kapan. Reload halaman -> urutan dan status tetap sama.
4. Key lengkap (`gr_...`) tidak tampil di tabel maupun di sumber respons jaringan `GET /api/projects/{id}/api-keys` (periksa respons di Playwright: tidak ada `raw_key`).
5. Super Admin (bukan anggota) membuka project -> tab "API Keys" memuat daftar yang sama (200).
6. Dua project dengan key berbeda: tab project A hanya menampilkan key project A.

Negatif:
1. Developer C (bukan anggota) membuka `/projects/<P1>/api-keys` (URL langsung) -> halaman "tidak ditemukan"; `GET /api/projects/<P1>/api-keys` -> 404.
2. `/projects/bukan-uuid/api-keys` -> halaman "tidak ditemukan"; API 404.
3. Tanpa login `GET /api/projects/<P1>/api-keys` -> 401, UI diarahkan ke login.
4. Developer yang dikeluarkan dari project (tab masih terbuka) lalu me-refresh daftar -> UI menampilkan "tidak ditemukan"/akses hilang (404), bukan daftar basi.

## 9. Request/Response body

Semua error memakai format seragam T00:

```json
{ "error": { "code": "NOT_FOUND", "message": "project tidak ditemukan", "details": null }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

### GET /api/projects/{projectID}/api-keys

Tanpa body dan tanpa query param. Tidak butuh header `X-Requested-With` (bukan mutasi). Contoh: `GET /api/projects/0192f5a0-1111-7aaa-8bbb-000000000001/api-keys`.

200 (aktif dulu, lalu `created_at` turun, lalu `id`; key dicabut tetap tampil):

```json
{
  "items": [
    {
      "id": "0192f5a0-2222-7aaa-8bbb-000000000003",
      "name": "ci-pipeline",
      "prefix": "a1b2c3d4",
      "status": "active",
      "created_by": "0192f5a0-aaaa-7aaa-8bbb-00000000000a",
      "created_by_label": "a@tim.id",
      "created_at": "2026-10-23T10:00:00Z",
      "last_used_at": "2026-10-23T11:42:00Z",
      "revoked_at": null,
      "revoked_by": null,
      "revoked_by_label": null
    },
    {
      "id": "0192f5a0-2222-7aaa-8bbb-000000000002",
      "name": "staging-bot",
      "prefix": "z9y8x7w6",
      "status": "active",
      "created_by": "0192f5a0-bbbb-7aaa-8bbb-00000000000b",
      "created_by_label": "b@tim.id",
      "created_at": "2026-10-23T09:30:00Z",
      "last_used_at": null,
      "revoked_at": null,
      "revoked_by": null,
      "revoked_by_label": null
    },
    {
      "id": "0192f5a0-2222-7aaa-8bbb-000000000001",
      "name": "claude-code-budi",
      "prefix": "k3x9q2ma",
      "status": "revoked",
      "created_by": "0192f5a0-bbbb-7aaa-8bbb-00000000000b",
      "created_by_label": "b@tim.id",
      "created_at": "2026-10-23T09:00:00Z",
      "last_used_at": "2026-10-23T11:00:00Z",
      "revoked_at": "2026-10-23T12:00:00Z",
      "revoked_by": "0192f5a0-aaaa-7aaa-8bbb-00000000000a",
      "revoked_by_label": "a@tim.id"
    }
  ]
}
```

Catatan: tidak ada field `raw_key`, `secret`, atau `secret_hash` di respons mana pun. `created_by`, `created_by_label` bisa `null` (pembuat tidak tercatat).

200 (project tanpa key):

```json
{ "items": [] }
```

401 (tanpa sesi, dari `RequireAuth`):

```json
{ "error": { "code": "UNAUTHORIZED", "message": "perlu login", "details": null }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

404 (bukan anggota, project tidak ada, atau `projectID` bukan uuid; tidak dibedakan):

```json
{ "error": { "code": "NOT_FOUND", "message": "project tidak ditemukan", "details": null }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

500 (mis. `check-access` atau DB gagal; pesan generik, tanpa detail internal):

```json
{ "error": { "code": "INTERNAL", "message": "terjadi kesalahan pada server", "details": null }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

Pesan dan kode 401/500 mengikuti format error T00 yang berlaku di `main`; nilai di atas ilustratif, test memeriksa `error.code` dan keberadaan `trace_id`, bukan teks pesan 401/500.
