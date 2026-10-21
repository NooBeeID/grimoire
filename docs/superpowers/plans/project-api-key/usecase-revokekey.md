# T04 project/api-key — Usecase `revokekey` (`POST /api/projects/{projectID}/api-keys/{keyID}/revoke`)

Melengkapi `docs/superpowers/plans/project-api-key/overview.md` (F: migrasi `api_keys`, `base.Key.Revoke`, `base.ErrKeyNotFound`, `base.ErrAlreadyRevoked`, `base.KeyEnvelope`, `base/store`, `base/access`, `base/audit`, OpenAPI `api-key.yml`). Bila dokumen ini bertentangan dengan overview, overview menang. Hanya satu usecase: `revokekey` (U6, gelombang paralel setelah F). Prasyarat F harus sudah ada; bila ada simbol F yang belum ada, hentikan dan laporkan (jangan membuatnya di luar allowedPaths).

## Metadata

| Field | Nilai |
|---|---|
| Ticket | T04 |
| Scope | fullstack pada tiket; plan usecase ini backend. UI aksi cabut + konfirmasi dirinci di `frontend-plan.md` |
| Domain | project |
| Module | api-key (`apikey`) |
| Feature | `revokekey` (U6) |
| Endpoint | `/api/projects/{projectID}/api-keys/{keyID}/revoke` |
| Method | POST (tanpa request body; body diabaikan, tidak di-decode) |
| Version | Tanpa prefix versi di URL (prefix `/api`); `api-key.yml` `info.version: 0.1.0` |
| Auth | `middleware.RequireAuth()` + CSRF (`X-Requested-With: grimoire`). Otorisasi di usecase via `access.Require` (member/owner/admin ok; bukan anggota/tidak ada/bukan uuid -> 404 `project tidak ditemukan`) |
| Respons sukses | 200 `{key: KeyDTO}` (`base.KeyEnvelope`) |
| Audit | `apikey.revoked` (`record.ActionAPIKeyRevoked`) via `base/audit` setelah commit, fail-open |
| Bus dikonsumsi | `project.management.check-access` (fail-closed, lewat `base/access`), `project.management.get-project` (fail-soft, label audit, lewat `base/audit`), `core.audit.record` (fail-open, lewat `base/audit`) |
| Bus disediakan | tidak ada |
| Tabel | menulis dan membaca `api_keys` (`SELECT ... FOR UPDATE`, `UPDATE`) |
| Resource root | `backend/internal/domain/project/internal/apikey` |
| allowedPaths | `backend/internal/domain/project/internal/apikey/revokekey/`, `backend/internal/domain/project/internal/apikey/base.go` (satu baris `revokekey.Init(d)`), `api/modules/project/api-key.yml` hanya bila operasi `revokekey` belum lengkap dari F, lalu `make api-bundle` |

## Summary Plan

`revokekey` mencabut satu API key secara permanen dengan efek langsung. Tidak ada endpoint hapus dan tidak ada aktifkan ulang.

Aturan:
- Urutan keputusan (jangan diubah):
  1. Bangun aktor dari `reqctx.Identity`. Tidak login -> 401.
  2. `access.Require(ctx, projectID, actor)` SEBELUM tx dan SEBELUM menyentuh `api_keys`. Project tidak terlihat oleh pemanggil -> 404 `project tidak ditemukan` (seragam, tidak membocorkan ada/tidaknya key).
  3. Tx (`InTx` di repository): `SELECT <KeyColumns> FROM api_keys WHERE id = $1 AND project_id = $2 FOR UPDATE`. Tidak ada, keyID bukan uuid, atau milik project lain -> 404 `KEY_NOT_FOUND`. Filter `project_id` dalam query yang sama menjamin isolasi antar project tanpa kebocoran (key project lain tidak dibedakan dari key yang tidak ada).
  4. Entity `Key.Revoke(actor, now)`: sudah dicabut -> 409 `ALREADY_REVOKED` (struct tidak berubah). Aturan bisnis ada di entity, bukan di usecase/repository.
  5. `UPDATE api_keys SET revoked_at, revoked_by, revoked_by_label ... RETURNING <KeyColumns>`.
  6. Setelah commit: audit `apikey.revoked` dengan `Args = {name, prefix}`. Audit gagal hanya di-log (fail-open), respons tetap 200. `ProjectLabel` dari `get-project` (fail-soft) di dalam `base/audit`.
- Konkurensi: dua revoke serentak pada key yang sama diserialkan oleh `FOR UPDATE`. Yang kedua menunggu, lalu membaca baris yang sudah dicabut dan `Key.Revoke` mengembalikan `ErrAlreadyRevoked`. Hasil: tepat satu 200, satu 409, tepat satu entri audit. Tidak perlu advisory lock (overview 3.5).
- Tanpa cache di mana pun, sehingga `verify` pada panggilan berikutnya langsung `Found=false, Reason=revoked` (AC3). Revoke tidak mengubah `last_used_at`.
- Nama key yang dicabut langsung bebas dipakai `createkey` (partial unique index `WHERE revoked_at IS NULL`, AC4). Tidak ada kode tambahan di usecase ini.
- Aktor yang mencabut tidak harus pembuat key (key tidak terikat pembuat, AC6). Label aktor = `Identity.Email` (K2), tanpa `core.auth.get-user`.
- Respons dibentuk dari baris hasil `RETURNING` (bukan dari baca ulang). Tidak ada `secret_hash` di DTO.
- Tidak memodifikasi sentinel `*errs.Error` milik `base`. Tidak me-log body, tidak ada raw key/secret yang disentuh usecase ini sama sekali.
- Satu aksi sukses = tepat satu entri audit. Gagal (401/404/409/500) = 0 entri.

## Plan

Urutan wajib; tulis test dulu (TDD), lalu implementasi.

**R0. Verifikasi (tanpa edit).** Pastikan ada: `base.Key` + `Key.Revoke`, `base.Actor`, `base.ActorFrom`, `base.RequestMeta`, `base.NewRequestMeta`, `base.Clock`, `base.KeyEnvelope`, `base.KeyDTO` + `Key.ToDTO`, `base.ErrKeyNotFound`, `base.ErrAlreadyRevoked`, `store.Store` (`GetKey`, `Querier`, `WithTx`, `ValidUUID`, `KeyColumns`), `access.Checker`/`access.New`, `audit.Recorder`/`audit.Event`/`audit.New`, `record.ActionAPIKeyRevoked`, tabel `api_keys`, `apikey.Register`. Baca `internal/infra/db` (`DB.BeginTx(ctx, func(*sqlx.Tx) error)`, `TestDB`), `respond` (`WriteJSON`, `WriteError`), `middleware` (`RequireAuth`, `KeyByIP`). Bila `createkey` (U4) sudah ada, ikuti pola `init.go`/`repository.go`-nya persis (pola `InTx`).

**R1. `revokekey/usecase.go`** (orkestrasi saja, rule 5.2 dan 5.6)

```go
package revokekey

// txRepo adalah operasi di dalam transaksi. Hanya SQL; tanpa bus, tanpa rule bisnis.
type txRepo interface {
    // LockInProject: SELECT ... WHERE id AND project_id FOR UPDATE.
    // keyID/projectID bukan uuid -> (zero, false, nil) tanpa query.
    LockInProject(ctx context.Context, projectID, keyID string) (base.Key, bool, error)
    // MarkRevoked: UPDATE field revoked_* dari k (sudah diisi entity) -> RETURNING baris baru.
    MarkRevoked(ctx context.Context, k base.Key) (base.Key, error)
}

type repository interface {
    InTx(ctx context.Context, fn func(txRepo) error) error
}

type Input struct {
    ProjectID string
    KeyID     string
    Meta      base.RequestMeta
}

type Usecase struct {
    repo   repository
    access access.Checker
    audit  audit.Recorder
    clock  base.Clock
}

func NewUsecase(r repository, a access.Checker, au audit.Recorder, c base.Clock) *Usecase

func (u *Usecase) Execute(ctx context.Context, in Input) (base.KeyDTO, error)
```

Alur `Execute` (tepat urutan ini):

1. `actor, ok := base.ActorFrom(reqctx.IdentityFrom(ctx))`; `!ok` -> `errs.New(errs.CodeUnauthorized, "perlu login")`.
2. `u.access.Require(ctx, in.ProjectID, actor)`; error dikembalikan apa adanya (`*errs.Error` 404, atau error infra 500 yang sudah dibungkus `base/access`).
3. `var revoked base.Key`; `err := u.repo.InTx(ctx, func(tx txRepo) error { ... })` dengan isi:
   - `k, found, err := tx.LockInProject(ctx, in.ProjectID, in.KeyID)`; error -> `fmt.Errorf("revokekey: lock key: %w", err)`; `!found` -> `base.ErrKeyNotFound`.
   - `if err := k.Revoke(actor, u.clock()); err != nil { return err }` (409 `ALREADY_REVOKED`).
   - `revoked, err = tx.MarkRevoked(ctx, k)`; error non-`*errs.Error` -> `fmt.Errorf("revokekey: mark revoked: %w", err)`.
4. Error dari `InTx` (termasuk begin/commit) dikembalikan; bila bukan `*errs.Error`, dibungkus `revokekey: tx: %w` (500 generik). Tidak ada audit bila error.
5. Setelah commit sukses: `u.audit.Record(ctx, audit.Event{Actor: actor, Action: record.ActionAPIKeyRevoked, ProjectID: in.ProjectID, KeyID: revoked.ID, Args: map[string]any{"name": revoked.Name, "prefix": revoked.Prefix}, Meta: in.Meta})`. Tidak mengembalikan error (fail-open).
6. `return revoked.ToDTO(), nil`.

Catatan: `in.ProjectID` yang dipakai di audit sudah divalidasi `check-access` (`ProjectExists=true` berarti uuid valid), jadi aman dipakai apa adanya. Closure `fn` hanya memanggil method repository dan rule entity; tidak ada bus di dalam tx.

**R2. `revokekey/repository.go`** (satu-satunya yang menyentuh SQL lewat store; SQL khusus feature ada di sini)

```go
type Repo struct {
    db *db.DB
    s  *store.Store
}

func newRepo(d *db.DB, s *store.Store) *Repo

func (r *Repo) InTx(ctx context.Context, fn func(txRepo) error) error {
    return r.db.BeginTx(ctx, func(tx *sqlx.Tx) error {
        return fn(&txRepoImpl{s: r.s.WithTx(tx)})
    })
}

type txRepoImpl struct{ s *store.Store }

const lockInProjectSQL = `SELECT ` + store.KeyColumns + ` FROM api_keys WHERE id = $1 AND project_id = $2 FOR UPDATE`

func (t *txRepoImpl) LockInProject(ctx context.Context, projectID, keyID string) (base.Key, bool, error) {
    if !store.ValidUUID(keyID) || !store.ValidUUID(projectID) {
        return base.Key{}, false, nil
    }
    return t.s.GetKey(ctx, lockInProjectSQL, keyID, projectID)
}

const markRevokedSQL = `UPDATE api_keys SET revoked_at = $2, revoked_by = $3, revoked_by_label = $4
    WHERE id = $1 AND revoked_at IS NULL RETURNING ` + store.KeyColumns

// MarkRevoked memakai nilai revoked_* dari entity (k.RevokedAt, *k.RevokedBy, *k.RevokedByLabel).
// Tidak ada baris ter-update (mustahil setelah FOR UPDATE, penjaga defensif) -> base.ErrAlreadyRevoked.
func (t *txRepoImpl) MarkRevoked(ctx context.Context, k base.Key) (base.Key, error)
```

`MarkRevoked` memakai `s.GetKey` dengan SQL di atas (`RETURNING` men-scan `KeyColumns`). `sql.ErrNoRows` yang dipetakan store menjadi `found=false` diterjemahkan ke `base.ErrAlreadyRevoked`. Nilai `*string` revoked_by/label di-deref dengan penjaga nil (entity `Revoke` selalu mengisinya). Klausa `AND project_id` tidak diperlukan di UPDATE karena baris sudah dikunci dan diverifikasi project-nya pada langkah SELECT yang sama dalam tx.

**R3. `revokekey/handler.go`**

```go
type usecase interface {
    Execute(ctx context.Context, in Input) (base.KeyDTO, error)
}

type handler struct{ uc usecase }

func newHandler(uc usecase) *handler

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    in := Input{
        ProjectID: chi.URLParam(r, "projectID"),
        KeyID:     chi.URLParam(r, "keyID"),
        Meta:      base.NewRequestMeta(middleware.KeyByIP(r), r.UserAgent()),
    }
    dto, err := h.uc.Execute(r.Context(), in)
    if err != nil {
        respond.WriteError(w, r, err)
        return
    }
    respond.WriteJSON(w, http.StatusOK, base.KeyEnvelope{Key: dto})
}
```

Tanpa `params.go` (tidak ada body; body dibiarkan tidak dibaca). Validasi uuid tidak di handler (overview 3.3).

**R4. `revokekey/init.go`**

```go
package revokekey

func Init(d app.Deps) {
    s := store.New(d.DB)
    uc := NewUsecase(newRepo(d.DB, s), access.New(d.Bus), audit.New(d.Bus, d.Log),
        base.Clock(func() time.Time { return time.Now().UTC() }))
    d.API.With(middleware.RequireAuth()).Post("/projects/{projectID}/api-keys/{keyID}/revoke", newHandler(uc).ServeHTTP)
}
```

Param WAJIB `{projectID}` dan `{keyID}` (K4). Jangan `Mount`/`Route` pada `/projects`.

**R5. Wiring.** Tambah satu baris `revokekey.Init(d)` di `backend/internal/domain/project/internal/apikey/base.go` (sertakan import). Tidak mengubah baris `Init` feature lain.

**R6. OpenAPI.** Pastikan operasi `revokekey` di `api/modules/project/api-key.yml` sesuai bagian Request/Response di bawah (200 `ApiKeyEnvelope`; 401, 403 CSRF, 404 dua varian, 409 `ALREADY_REVOKED`; parameter path `ProjectID` dan `KeyID` berformat uuid; tanpa requestBody). Bila sudah lengkap dari F, jangan ubah. Jika mengubah, jalankan `make api-bundle` lalu `make api-check`.

**R7. Verifikasi akhir.** `go vet ./...`, `go test ./internal/domain/project/internal/apikey/revokekey/...`, `go test -tags integration ./internal/domain/project/internal/apikey/revokekey/...`, archtest `TestT00_AC4_RepoIsClean`.

## Migration

Tidak ada. Tabel `api_keys` dibuat di foundation: `backend/migrations/project/<VERSION>_create_api_keys.sql`. Usecase ini hanya memakai kolom `revoked_at`, `revoked_by`, `revoked_by_label`, CHECK `api_keys_revoked_consistent`, dan partial unique index `api_keys_project_active_name_key` yang sudah ada.

## Changes

Baru:
- `backend/internal/domain/project/internal/apikey/revokekey/init.go`
- `backend/internal/domain/project/internal/apikey/revokekey/handler.go`
- `backend/internal/domain/project/internal/apikey/revokekey/usecase.go`
- `backend/internal/domain/project/internal/apikey/revokekey/repository.go`
- `backend/internal/domain/project/internal/apikey/revokekey/usecase_test.go` (unit, repo/access/audit palsu)
- `backend/internal/domain/project/internal/apikey/revokekey/handler_test.go` (`httptest`, usecase palsu)
- `backend/internal/domain/project/internal/apikey/revokekey/repository_integration_test.go` (tag `integration`, `db.TestDB`)

Diubah:
- `backend/internal/domain/project/internal/apikey/base.go`: tambah `revokekey.Init(d)`.
- `api/modules/project/api-key.yml` dan `api/openapi.yml`: hanya bila operasi belum lengkap dari F.

Tidak diubah: `base/` (entity `Key.Revoke`, error, DTO sudah dari F), `base/store`, `base/access`, `base/audit`, feature lain, `internal/infra/**`, migrasi, kontrak bus (tidak ada kontrak baru).

Tidak ada import antar feature (rule 4.3). Verifikasi lintas feature (`verify`, `createkey`) diuji di `apikey_integration_test.go` (W), bukan di package ini.

## Test Scenario

Penamaan: `TestT04_ACn_<Deskripsi>` untuk kriteria penerimaan; lainnya `Test<Fungsi>_<Kasus>`.

### Unit entity (sudah di F `base/key_test.go`; di sini hanya dipastikan ada, tidak diduplikasi)

- `Key.Revoke`: key aktif -> `RevokedAt` = `now.UTC()`, `RevokedBy` = actor.ID, `RevokedByLabel` = actor.Label; key sudah dicabut -> `ErrAlreadyRevoked` dan seluruh field tidak berubah (ada di F; bila tidak ada, tambahkan di `base/key_test.go` sebagai satu-satunya tambahan di luar folder feature, lalu laporkan).

### Unit usecase (`usecase_test.go`, repo/access/audit palsu, clock tetap)

- Sukses: `access.Require` dipanggil dengan `(projectID, actor{ID, Role, Label=email})`; `LockInProject` dipanggil dengan `(projectID, keyID)`; `MarkRevoked` menerima key dengan `RevokedAt = clock()`, `RevokedBy = actor.ID`, `RevokedByLabel = email`; DTO `status = revoked`; audit dipanggil tepat sekali dengan `Action = record.ActionAPIKeyRevoked`, `KeyID`, `ProjectID`, `Args = {name, prefix}` (tanpa kunci lain), `Meta` dari input; audit dipanggil SETELAH `InTx` selesai (urutan dicatat palsu).
- Tidak ada identitas di ctx -> `CodeUnauthorized`; access, repo, dan audit tidak dipanggil.
- `access.Require` mengembalikan `base.ErrProjectNotFound` -> dikembalikan apa adanya; `InTx` tidak dipanggil; audit 0.
- `access.Require` mengembalikan error infra -> dikembalikan (500); `InTx` tidak dipanggil.
- `LockInProject` found=false -> `ErrKeyNotFound` (`details.reason = KEY_NOT_FOUND`); `MarkRevoked` tidak dipanggil; audit 0.
- Key sudah dicabut (palsu mengembalikan key dengan `RevokedAt` terisi) -> `ErrAlreadyRevoked`; `MarkRevoked` tidak dipanggil; audit 0.
- `LockInProject` error DB -> error terbungkus `revokekey: lock key`, bukan `*errs.Error`; audit 0.
- `MarkRevoked` error DB -> error terbungkus; audit 0.
- `InTx` (commit) error -> error terbungkus `revokekey: tx`; audit 0.
- Audit palsu mencatat panggilan; dengan recorder yang "gagal" (recorder fail-open, tidak mengembalikan error) usecase tetap mengembalikan DTO.
- Sentinel tidak dimodifikasi: bandingkan pointer/`details` `ErrKeyNotFound`/`ErrAlreadyRevoked` sebelum dan sesudah.

### Unit handler (`handler_test.go`, `httptest` + `chi.NewRouter`)

- Route `POST /projects/{projectID}/api-keys/{keyID}/revoke`: usecase palsu menerima `Input.ProjectID`/`KeyID` sesuai URL dan `Meta.UserAgent` dari header; sukses -> 200, `Content-Type: application/json`, body `{"key": {...}}` dengan `status:"revoked"`; JSON tidak memuat kunci `secret`, `secret_hash`, `hash`, `raw_key`.
- Body request diabaikan: POST dengan body sampah (`not json`) tetap 200.
- Usecase mengembalikan `ErrKeyNotFound` -> 404 + `details.reason = KEY_NOT_FOUND`; `ErrProjectNotFound` -> 404 `project tidak ditemukan`; `ErrAlreadyRevoked` -> 409 + `ALREADY_REVOKED`; error generik -> 500 tanpa membocorkan pesan internal; semua memuat `trace_id`.

### Integrasi repository + usecase nyata (`repository_integration_test.go`, tag `integration`, `db.TestDB`)

Fixture lewat SQL: project P1, P2 (`INSERT INTO projects (name) VALUES ($1) RETURNING id`), key K1 (aktif, P1), K2 (aktif, P2), user A (member P1), user B (member P1). `access` palsu (peta `(projectID, userID) -> Level`, user di luar peta -> `ProjectExists=true, Level=none`, project acak -> `ProjectExists=false`), audit palsu yang merekam, atau `audit.New` dengan bus nyata dan handler `core.audit.record` palsu untuk memeriksa `record.Request`.

- `TestT04_AC3_RevokeThenVerifyImmediatelyNotFound`: revoke K1 sukses; baca ulang baris K1 lewat `store.GetKey` by prefix lalu `base.VerifyOutcome(k, true, secret)` mengembalikan `ok=false, reason=revoked` TANPA cache; kolom `revoked_at` terisi, `revoked_by` = A, `revoked_by_label` = email A. (Verifikasi lewat bus `project.apikey.verify` yang sesungguhnya dilakukan di `apikey_integration_test.go` W, karena feature dilarang saling import.)
- `TestT04_Revoke_SecondRevokeConflict`: revoke K1 kedua kali (user A atau B) -> 409 `ALREADY_REVOKED`; `revoked_at`/`revoked_by` tidak berubah dari revoke pertama; audit tetap 1 entri.
- `TestT04_Security_ConcurrentRevokeExactlyOne`: 10 goroutine serentak (dengan `sync.WaitGroup` dan gerbang start) me-revoke K1 -> tepat satu sukses, sembilan `ErrAlreadyRevoked`; tepat satu panggilan audit; diulang 5 kali dengan key baru tiap iterasi. Jalankan dengan `-race`.
- `TestT04_Security_ProjectIsolation`: revoke `(P1, K2)` (K2 milik P2) -> 404 `KEY_NOT_FOUND`; baris K2 tetap `revoked_at IS NULL`; audit 0. Revoke `(P2, K1)` pun 404.
- `TestT04_Revoke_NonUUIDKeyID`: keyID `bukan-uuid` -> 404 `KEY_NOT_FOUND`, tanpa error SQL (tidak ada 500); keyID uuid valid yang tidak ada -> 404 `KEY_NOT_FOUND`.
- `TestT04_Revoke_NonMemberGets404`: `access` mengembalikan `Level=none` -> 404 `project tidak ditemukan` (bukan `KEY_NOT_FOUND`); K1 tetap aktif; audit 0. Super_admin (`Level=admin`) bukan anggota -> sukses.
- `TestT04_AC6_RevokerNeedNotBeCreator`: K1 dibuat oleh A; B (member lain) me-revoke -> 200; `created_by` tetap A, `revoked_by` = B.
- `TestT04_AC4_RevokedNameReusable`: setelah revoke K1 (nama "ci"), `INSERT INTO api_keys` baru dengan project dan nama yang sama berhasil (partial unique index); `INSERT` kedua dengan nama itu saat key baru masih aktif melanggar constraint `api_keys_project_active_name_key`. (Alur lewat `createkey` penuh ada di W AC4.)
- `TestT04_AC9_AuditRevoked`: tepat satu `record.Request` dengan `Action = apikey.revoked`, `ActorType = user`, `ActorID` = A, `ActorLabel` = email A, `ProjectID` = P1, `ProjectLabel` = nama P1 (dari `get-project` palsu), `TargetType = api_key`, `Target` = K1, `Args = {name, prefix}` persis dua kunci, `Status = ok`, `TraceID` terisi; Args tidak memuat raw key/secret/hash. `get-project` error atau `Found=false` -> `ProjectLabel = ""` dan audit tetap terkirim. Handler `core.audit.record` error atau tidak terdaftar -> respons tetap sukses (200 sementara baris sudah dicabut), error dicatat di log.
- `TestT04_Revoke_NoSecretInLogs`: logger berbuffer; setelah revoke (sukses dan gagal), output tidak mengandung secret, `secret_hash`, atau nilai `Args`; boleh memuat `prefix`/`key_id`.
- `TestT04_Revoke_ResponseShape`: DTO hasil revoke: `status = revoked`, `revoked_at` UTC, `last_used_at` tidak berubah, field lain identik dengan baris sebelum revoke.
- `TestT04_Revoke_NoReactivation`: tidak ada route yang bisa membuat `revoked_at` kembali NULL (periksa `chi.Walk`: tidak ada DELETE/PUT/PATCH di `/projects/{projectID}/api-keys/{keyID}`); revoke ulang tidak mengubah baris.
- `TestRoutesRegistered_Revoke`: `chi.Walk` memuat `POST /projects/{projectID}/api-keys/{keyID}/revoke` dengan `RequireAuth` terpasang (request tanpa identitas -> 401).

### Lintas feature (ditulis di W `apikey_integration_test.go`, bukan di sini; rujukan)

- `TestT04_AC3_VerifyImmediatelyAfterCreateAndRevoke`, `TestT04_AC4_DuplicateActiveNameRejected_ReusableAfterRevoke`, `TestT04_Security_RevokedKeyRejected` (dua revoke serentak lewat HTTP: tepat satu 200, satu 409, satu audit), `TestT04_AC9_AuditCreatedAndRevoked`.

## E2E Scenarios

Playwright terhadap binary (butuh T01, T03, dan frontend T04). Aksi cabut dari tab "API Keys".

Positif:
1. Developer B (member, bukan owner) membuka tab "API Keys", menekan "Cabut" pada key "claude-code-budi", dialog konfirmasi muncul menyebut nama key dan prefix; konfirmasi -> baris langsung tampil redup dengan status "dicabut", "dicabut oleh b@tim.id" dan waktu; tombol "Cabut" pada baris itu hilang.
2. Setelah dicabut, reload halaman -> baris tetap tampil sebagai "dicabut"; key tidak bisa dihapus (tidak ada tombol hapus).
3. Klien MCP yang memakai key tersebut ditolak pada permintaan berikutnya (di-cover E2E T07; di sini cukup `POST /mcp` dengan key itu mengembalikan 401 seragam bila T07 sudah ada).
4. Setelah mencabut, buat key baru dengan nama yang sama dengan key yang baru dicabut -> sukses.
5. Super Admin (bukan anggota) mencabut key di project mana pun -> sukses; tautan "Audit Log" terfilter menampilkan entri `apikey.revoked` dengan nama dan prefix (hanya Super Admin yang melihat tautan).
6. Developer lain (anggota project, bukan pembuat key) mencabut key milik rekannya -> sukses.

Negatif:
1. Dua tab terbuka; tab 1 mencabut key, tab 2 (daftar basi) menekan "Cabut" pada key yang sama -> pesan i18n `project.apikey.error.ALREADY_REVOKED`, tabel di-refresh dan menampilkan key sebagai dicabut.
2. Developer C (bukan anggota) membuka tab/URL langsung `/projects/<P1>/api-keys` -> halaman "tidak ditemukan"; `POST` revoke langsung via fetch -> 404 `project tidak ditemukan`.
3. Mencabut key via URL project lain (`POST /api/projects/<P1>/api-keys/<keyID P2>/revoke`) -> 404 `KEY_NOT_FOUND`; key P2 tetap aktif di UI P2.
4. `POST /api/projects/<id>/api-keys/<bukan-uuid>/revoke` -> 404 `KEY_NOT_FOUND`.
5. `POST` tanpa header `X-Requested-With: grimoire` -> 403 `CSRF_REQUIRED`; key tetap aktif.
6. Tanpa login -> 401.
7. Setelah revoke gagal (404/409), halaman Audit Log Super Admin tidak mendapat entri `apikey.revoked` baru.

## Request/Response body

Semua error memakai format seragam T00. Header mutasi: `X-Requested-With: grimoire`. Tidak ada `Content-Type` wajib karena tidak ada body.

### POST /api/projects/0192f5a0-1111-7aaa-8bbb-000000000001/api-keys/0192f5a0-2222-7aaa-8bbb-000000000001/revoke

Request: tanpa body (body, bila ada, diabaikan).

200:
```json
{
  "key": {
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
}
```

401 (tanpa sesi):
```json
{ "error": { "code": "UNAUTHORIZED", "message": "perlu login", "details": null }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

403 (tanpa header CSRF, dari middleware T00):
```json
{ "error": { "code": "CSRF_REQUIRED", "message": "...", "details": null }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

404 (bukan anggota, project tidak ada, atau projectID bukan uuid):
```json
{ "error": { "code": "NOT_FOUND", "message": "project tidak ditemukan", "details": null }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

404 (key tidak ada, keyID bukan uuid, atau key milik project lain):
```json
{ "error": { "code": "NOT_FOUND", "message": "API key tidak ditemukan", "details": { "reason": "KEY_NOT_FOUND" } }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

409 (sudah dicabut):
```json
{ "error": { "code": "CONFLICT", "message": "API key sudah dicabut", "details": { "reason": "ALREADY_REVOKED" } }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

500 (error infra, termasuk `check-access` gagal; pesan generik):
```json
{ "error": { "code": "INTERNAL", "message": "terjadi kesalahan internal", "details": null }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

### Audit (`record.Request` yang dikirim setelah commit)

```go
record.Request{
    ActorType: record.ActorUser, ActorID: "0192f5a0-aaaa-7aaa-8bbb-00000000000a", ActorLabel: "a@tim.id",
    ProjectID: "0192f5a0-1111-7aaa-8bbb-000000000001", ProjectLabel: "Payments",
    Action: record.ActionAPIKeyRevoked, TargetType: "api_key", Target: "0192f5a0-2222-7aaa-8bbb-000000000001",
    Args: map[string]any{"name": "claude-code-budi", "prefix": "k3x9q2ma"},
    Status: record.StatusOK, IP: "10.0.0.5", UserAgent: "Mozilla/5.0", TraceID: "01JB2X9K6Q4Y7V3N8M5T2R1C0A",
}
```

Kode error `INTERNAL`/`UNAUTHORIZED` mengikuti nilai `errs` T00 yang sebenarnya; sesuaikan string contoh dengan kode yang ada di `lib/common/errs` bila berbeda (tidak ada kode error baru).
