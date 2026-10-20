# T04 project/api-key — Usecase `createkey` (U4, gelombang paralel setelah F)

Melengkapi `docs/superpowers/plans/project-api-key/overview.md` (selanjutnya "overview"). Bila dokumen ini bertentangan dengan overview, overview menang. Hanya satu usecase: `createkey`. Plan usecase lain tidak disentuh.

## Metadata

| Field | Nilai |
|---|---|
| Ticket | T04 (AC1, AC2, AC4, AC5, AC9; ikut menopang AC3 dan AC6) |
| Scope | fullstack pada tiket; usecase ini backend + verifikasi kontrak OpenAPI. Dialog buat key ada di `frontend-plan.md` |
| Domain | project |
| Module | api-key (`apikey`) |
| Feature | `createkey` |
| Endpoint | `/api/projects/{projectID}/api-keys` |
| Method | POST |
| Version | tanpa prefix versi di URL (prefix `/api`); `api-key.yml` `info.version: 0.1.0` |
| Siapa | anggota project (member/owner) dan super_admin; dicek di usecase lewat `project.management.check-access` (middleware hanya `RequireAuth()`) |
| Header wajib | `X-Requested-With: grimoire` (CSRF T00), `Content-Type: application/json` |
| Respons sukses | 201 `CreateKeyResult` `{key, raw_key, mcp_url}` + header `Cache-Control: no-store` |
| Audit | `apikey.created` (Call `core.audit.record`, fail-open, setelah commit) |
| Bus dikonsumsi | `project.management.check-access` (fail-closed, via `base/access`), `project.management.get-project` (fail-soft, via `base/audit`), `core.audit.record` (fail-open, via `base/audit`). Tidak memakai `core.auth.get-user` (K2) |
| Tabel / migrasi | menulis `api_keys`; migrasi dibuat di F overview (bagian 6), tidak ada migrasi baru di sini |
| Config | `d.Config.APIKeyMaxActivePerProject` (default 20), `d.Config.PublicURL` (opsional), keduanya dari F (K8) |
| Resource root | `backend/internal/domain/project/internal/apikey` |
| allowedPaths | `.../apikey/createkey/`, `.../apikey/base.go` (satu baris `createkey.Init(d)`) |

Prasyarat (dari F overview; bila belum ada, hentikan dan laporkan, jangan dibuat di luar allowedPaths): migrasi `20261023090000000_create_api_keys.sql`; `base` (`RawKey`, `GenerateRawKey`, `Key`, `NewKey`, `ValidateName`, `CheckActiveLimit`, `ToDTO`, `CreateKeyResult`, `MCPURL`, `Actor`, `ActorFrom`, `RequestMeta`, `NewRequestMeta`, `Clock`, `FieldErrors`, `InvalidFields`, `ErrProjectNotFound`, `ErrNameTaken`, `ErrPrefixCollision`, `ErrKeyLimitReached`); `base/store` (`Store`, `New`, `WithTx`, `Querier`, `GetKey`, `KeyColumns`, `Violation`, `CodeUniqueViolation`, `CodeFKViolation`, `ConstraintPrefixUnique`, `ConstraintActiveName`); `base/access` (`Checker`, `New`); `base/audit` (`Recorder`, `Event`, `New`); konstanta `BUS_PROJECT_MANAGEMENT_CHECK_ACCESS`, `BUS_PROJECT_MANAGEMENT_GET_PROJECT`, `BUS_CORE_AUDIT_RECORD`, `record.ActionAPIKeyCreated`; field config K8. Path `POST /projects/{projectID}/api-keys` dan schema `CreateApiKeyRequest`/`CreateApiKeyResponse`/`ApiKey` sudah ada di `api/modules/project/api-key.yml` dari F; usecase ini hanya memverifikasi.

## Summary Plan

`createkey` membuat API key bernama untuk satu project dan mengembalikan plaintext SEKALI.

Aturan:
- Urutan wajib di usecase: aktor dari reqctx -> `access.Require` (404 bila bukan anggota/project tak ada/projectID bukan uuid) -> `base.ValidateName` -> `base.GenerateRawKey(rand)` -> `repo.InTx` -> audit setelah commit -> susun hasil. Otorisasi SEBELUM validasi sehingga non-anggota tidak bisa menebak apa pun dari pesan validasi.
- Validasi nama (2-60 rune setelah trim, tanpa karakter kontrol) hidup di entity (`ValidateName`, juga dipanggil `NewKey`). Usecase memanggil `ValidateName` lebih awal hanya agar request invalid tidak membuang random/tx; `NewKey` tetap validasi ulang sebagai penjaga entity. Body tak dikenal/rusak -> 400 dari `respond.DecodeJSON`.
- Satu transaksi per percobaan (di repository): (1) `pg_advisory_xact_lock(hashtextextended('apikey:'||project_id, 0))`, (2) `count(*)` key aktif, (3) `base.CheckActiveLimit(count, limit)` di closure usecase (rule di entity, bukan di SQL/usecase), (4) `INSERT ... RETURNING`. Hanya `prefix` dan `SHA-256(secret)` yang masuk SQL; `RawKey.Full` dan `RawKey.Secret` tidak pernah melewati repository (Key dari `NewKey` tidak memuat secret).
- Pemetaan error Postgres di repository (`store.Violation`): 23505 `api_keys_project_active_name_key` -> `base.ErrNameTaken` (409 `NAME_TAKEN`); 23505 `api_keys_prefix_key` -> `base.ErrPrefixCollision` (sentinel internal); 23503 -> `base.ErrProjectNotFound` (404, project terhapus di tengah jalan). Constraint lain -> error infra terbungkus (500).
- Retry K13: bila `errors.Is(err, base.ErrPrefixCollision)`, usecase membuat `RawKey` baru dan menjalankan tx baru (tx lama abort). Maks 3 percobaan (`maxPrefixAttempts = 3`); setelah itu `fmt.Errorf("createkey: prefix collision setelah %d percobaan: %w", ...)` -> 500. Retry ada di usecase karena itu orkestrasi; pemeriksaan limit dan nama diulang otomatis di tiap tx.
- Konkurensi: advisory lock per project menserialkan create; dua create serentak saat sisa 1 slot -> tepat satu 201, satu 409 `KEY_LIMIT_REACHED` (AC5). Lock lepas otomatis saat commit/rollback. Panggilan bus TIDAK PERNAH di dalam tx.
- Audit `apikey.created` dikirim SETELAH commit lewat `audit.Recorder` (fail-open, K5): `Args = {"name": <nama>, "prefix": <prefix>}`, `Target = key id`. Tepat satu entri per key yang berhasil dibuat; 400/404/409/500 -> 0 entri. Label project dari `get-project` fail-soft ada di `base/audit`, bukan di usecase.
- K10: raw key/secret/hash DILARANG di log, audit, error, atau test snapshot yang tersimpan. Log hanya `feature`, `project_id`, `key_id`, `prefix`. Handler tidak me-log body. Respons ber-header `Cache-Control: no-store` (juga `Pragma: no-cache`) agar tidak tersimpan cache perantara/browser.
- `mcp_url` = `base.MCPURL(cfg.PublicURL)`: `PUBLIC_URL + "/mcp"` atau `null`.
- Label pembuat (`created_by_label`) = email aktor dari `reqctx.Identity` (K2). Pembuat tidak mengikat key (key tetap hidup walau pembuat keluar, AC6; tidak ada kolom/aturan yang menghubungkannya).
- Tidak ada idempotensi: request ulang dengan nama sama -> 409 `NAME_TAKEN` selama key pertama aktif.
- Error infra dibungkus `fmt.Errorf("createkey: <langkah>: %w", err)` dan menjadi 500 generik.

## Plan

Urutan wajib. Tulis test dulu (TDD), lalu implementasi.

**B0. Verifikasi (tanpa edit).** Pastikan simbol prasyarat ada dan cocok dengan overview bagian 4-5. Baca `app.Deps`, `db.DB.BeginTx(ctx, func(*sqlx.Tx) error)`, `respond.*`, `middleware.RequireAuth/KeyByIP`, dan pola `Init` feature lain (mis. `management/createproject` bila sudah merge, atau `core/*`). Bila `base/store` belum punya `WithTx`/`GetKey`/`Violation`, hentikan dan laporkan.

**B1. `createkey/params.go`**

```go
type CreateRequest struct {
    Name string `json:"name"`
}
```
Hanya bentuk. `name` tidak ada -> `""` lalu ditolak entity (`fields.name`). `respond.DecodeJSON` menolak field tak dikenal, body kosong/rusak, dan tipe salah (400).

**B2. `createkey/usecase.go`**

```go
const maxPrefixAttempts = 3

// Tx = operasi di dalam satu transaksi create. Implementasi: repository.
type Tx interface {
    LockProjectKeys(ctx context.Context, projectID string) error                  // advisory lock per project
    CountActive(ctx context.Context, projectID string) (int, error)
    Insert(ctx context.Context, k base.Key) (base.Key, error)                      // RETURNING; memetakan 23505/23503 ke error base
}
type Repository interface {
    InTx(ctx context.Context, fn func(Tx) error) error
}

type Usecase struct {
    repo      Repository
    access    access.Checker
    audit     audit.Recorder
    clock     base.Clock
    rnd       io.Reader // produksi: crypto/rand.Reader
    limit     int       // cfg.APIKeyMaxActivePerProject
    publicURL string    // cfg.PublicURL
    log       *slog.Logger
}
func NewUsecase(r Repository, a access.Checker, au audit.Recorder, c base.Clock, rnd io.Reader, limit int, publicURL string, log *slog.Logger) *Usecase

func (u *Usecase) Execute(ctx context.Context, projectID string, in CreateRequest, meta base.RequestMeta) (base.CreateKeyResult, error)
```

Alur `Execute` (orkestrasi saja, satu fungsi; jangan dipecah menjadi service):
1. `actor, ok := base.ActorFrom(reqctx.IdentityFrom(ctx))`; `!ok` -> `errs.New(errs.CodeUnauthorized, "perlu login")`.
2. `if err := u.access.Require(ctx, projectID, actor); err != nil { return zero, err }` (404 / 500 apa adanya; jangan dibungkus ulang agar `*errs.Error` tetap terdeteksi).
3. `name, fe := base.ValidateName(in.Name)`; `fe != nil` -> `base.InvalidFields(fe)`.
4. `now := u.clock()`.
5. Loop `for attempt := 1; attempt <= maxPrefixAttempts; attempt++`:
   - `raw, err := base.GenerateRawKey(u.rnd)`; err -> `fmt.Errorf("createkey: generate key: %w", err)`.
   - `key, err := base.NewKey(projectID, name, raw, actor, now)`; err dikembalikan apa adanya (entity).
   - `var created base.Key`; `err = u.repo.InTx(ctx, func(tx Tx) error { ... })` dengan isi:
     - `tx.LockProjectKeys(ctx, projectID)`
     - `n, err := tx.CountActive(ctx, projectID)`
     - `if err := base.CheckActiveLimit(n, u.limit); err != nil { return err }` (`*errs.Error`, jangan dibungkus)
     - `created, err = tx.Insert(ctx, key)`
     - error infra dibungkus `fmt.Errorf("createkey: lock: %w", err)`, dst; error bisnis (`*errs.Error`, `ErrPrefixCollision`) dikembalikan apa adanya.
   - `errors.Is(err, base.ErrPrefixCollision)` -> `u.log.WarnContext(ctx, "createkey: prefix collision, mengulang", "feature", "createkey", "project_id", projectID, "attempt", attempt)` lalu `continue` (prefix tidak ikut di-log: key itu dibuang).
   - `err != nil` -> `return zero, err`.
   - sukses -> `break` loop dengan `rawFull = raw.Full`, `created`.
   - Setelah loop tanpa sukses: `fmt.Errorf("createkey: prefix collision setelah %d percobaan: %w", maxPrefixAttempts, base.ErrPrefixCollision)`.
6. Audit: `u.audit.Record(ctx, audit.Event{Actor: actor, Action: record.ActionAPIKeyCreated, ProjectID: projectID, KeyID: created.ID, Args: map[string]any{"name": created.Name, "prefix": created.Prefix}, Meta: meta})`. Tidak mengembalikan error.
7. `return base.CreateKeyResult{Key: created.ToDTO(), RawKey: rawFull, MCPURL: base.MCPURL(u.publicURL)}, nil`.

Catatan penting:
- `rawFull` hanya hidup di variabel lokal `Execute` sampai return; tidak diteruskan ke repo, audit, atau log.
- Tidak ada rule bisnis di usecase (nama, limit, format key ada di `base`). `maxPrefixAttempts` adalah kebijakan orkestrasi, bukan rule domain.
- Satu `now` untuk semua percobaan cukup (created_at = waktu request).

**B3. `createkey/repository.go`** (satu-satunya yang menyentuh SQL feature ini)

```go
type Repo struct {
    db *db.DB
    s  *store.Store // di luar tx: store.New(d.DB); di dalam tx: s.WithTx(tx)
}
func newRepo(d *db.DB, s *store.Store) *Repo

func (r *Repo) InTx(ctx context.Context, fn func(Tx) error) error {
    return r.db.BeginTx(ctx, func(tx *sqlx.Tx) error {
        return fn(&Repo{db: r.db, s: r.s.WithTx(tx)})
    })
}
```
SQL:
```sql
SELECT pg_advisory_xact_lock(hashtextextended('apikey:' || $1::text, 0));

SELECT count(*) FROM api_keys WHERE project_id = $1 AND revoked_at IS NULL;

INSERT INTO api_keys (project_id, name, prefix, secret_hash, created_by, created_by_label, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING <store.KeyColumns>;
```
- `LockProjectKeys`: `$1` bukan uuid valid tidak mungkin sampai sini (check-access sudah menolak), tetapi lock memakai teks sehingga tidak butuh cast uuid. Eksekusi lewat `s.Querier().ExecContext`.
- `CountActive`: `QueryRowxContext(...).Scan(&n)`; `project_id = $1` dengan string uuid valid (sudah lolos check-access). Jangan interpolasi string.
- `Insert`: pakai `r.s.GetKey(ctx, insertSQL, args...)` agar scan memakai `keyRow` bersama (waktu `.UTC()`, `[]byte` di-copy). Hasil `found=false` mustahil -> error infra. Saat error: `code, constraint, ok := store.Violation(err)`:
  - `ok && code == store.CodeUniqueViolation && constraint == store.ConstraintActiveName` -> `base.ErrNameTaken`
  - `ok && code == store.CodeUniqueViolation && constraint == store.ConstraintPrefixUnique` -> `base.ErrPrefixCollision`
  - `ok && code == store.CodeFKViolation` -> `base.ErrProjectNotFound`
  - lainnya -> `fmt.Errorf("createkey: insert: %w", err)`
- Argumen insert: `k.ProjectID, k.Name, k.Prefix, k.SecretHash, k.CreatedBy, k.CreatedByLabel, k.CreatedAt` (`*string` nil -> NULL). Tidak ada argumen lain; tidak mungkin ada plaintext.
- Urutan lock -> count -> insert dijamin satu tx oleh `InTx`; rollback otomatis bila `fn` mengembalikan error (`db.BeginTx`).

**B4. `createkey/handler.go`**

```go
type Handler struct{ uc *Usecase }
func newHandler(uc *Usecase) *Handler

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Cache-Control", "no-store")
    w.Header().Set("Pragma", "no-cache")
    var in CreateRequest
    if err := respond.DecodeJSON(r, &in); err != nil { respond.WriteError(w, r, err); return }
    meta := base.NewRequestMeta(middleware.KeyByIP(r), r.UserAgent())
    res, err := h.uc.Execute(r.Context(), chi.URLParam(r, "projectID"), in, meta)
    if err != nil { respond.WriteError(w, r, err); return }
    respond.WriteJSON(w, http.StatusCreated, res)
}
```
Header `no-store` diset di awal sehingga berlaku untuk semua jawaban endpoint ini. Handler tidak validasi uuid dan tidak me-log apa pun yang berisi body/hasil.

**B5. `createkey/init.go`**

```go
func Init(d app.Deps) {
    s := store.New(d.DB)
    uc := NewUsecase(
        newRepo(d.DB, s),
        access.New(d.Bus),
        audit.New(d.Bus, d.Log),
        base.Clock(func() time.Time { return time.Now().UTC() }),
        rand.Reader, // crypto/rand
        d.Config.APIKeyMaxActivePerProject,
        d.Config.PublicURL,
        d.Log,
    )
    d.API.With(middleware.RequireAuth()).Post("/projects/{projectID}/api-keys", newHandler(uc).ServeHTTP)
}
```
Route pola penuh langsung di `d.API` (K4), param WAJIB `{projectID}`; jangan `Route("/projects", ...)`. Import `crypto/rand` (bukan `math/rand`); archtest/lint harus menolak `math/rand` di folder ini (lihat Test Scenario).

**B6. Wiring**: tambah satu baris `createkey.Init(d)` di `apikey/base.go` (`Register`). Jangan menyentuh baris Init feature lain.

**B7. OpenAPI**: verifikasi operasi `createkey` di `api/modules/project/api-key.yml`: parameter `ProjectID` (uuid) + `XRequestedWith`; request `CreateApiKeyRequest` (`name` minLength 2 maxLength 60, `additionalProperties: false`); 201 `CreateApiKeyResponse` dengan header respons `Cache-Control: no-store`; error 400/401/403/404/409 via `../infra/foundation.yml`; 409 mendokumentasikan `NAME_TAKEN` dan `KEY_LIMIT_REACHED` (`details.limit`). Bila ada selisih dengan bagian "Request/Response body" di bawah, perbaiki di file itu, jalankan `make api-bundle`, pastikan `make api-check` lulus.

**B8. Verifikasi akhir**: `go vet ./...`, `go test ./internal/domain/project/...`, `go test -tags integration ./internal/domain/project/...`, archtest `TestT00_AC4_RepoIsClean` (createkey hanya import `base`, `base/store`, `base/access`, `base/audit`, infra, lib; tidak import feature lain; tidak ada `bus.Call` langsung di usecase/repository).

## Migration

Tidak ada di usecase ini. Memakai tabel `api_keys` dari `backend/migrations/project/20261023090000000_create_api_keys.sql` (F overview bagian 6). Usecase bergantung pada:
- unique index `api_keys_prefix_key` (tabrakan prefix -> retry) dan `api_keys_project_active_name_key` (partial `WHERE revoked_at IS NULL`, nama kembar aktif -> 409);
- FK `project_id -> projects(id)` (23503 -> 404);
- CHECK `api_keys_name_len`, `api_keys_prefix_fmt`, `api_keys_secret_hash_len` sebagai jaring pengaman (bukan sumber pesan error).

## Changes

Baru:
- `backend/internal/domain/project/internal/apikey/createkey/init.go`
- `.../createkey/handler.go`
- `.../createkey/params.go`
- `.../createkey/usecase.go`
- `.../createkey/repository.go`
- `.../createkey/usecase_test.go` (unit, fake repo/access/audit/clock/rand)
- `.../createkey/handler_test.go` (httptest, usecase dengan fake)
- `.../createkey/repository_integration_test.go` (tag `integration`, `db.TestDB(t)`)

Ubah:
- `backend/internal/domain/project/internal/apikey/base.go`: tambah `createkey.Init(d)`.
- `api/modules/project/api-key.yml` hanya bila verifikasi B7 menemukan selisih (lalu `api/openapi.yml` diregenerasi).

Tidak diubah: `lib/constants/bus.go`, `lib/contracts/**`, migrasi, `internal/infra/**`, `go.mod`, `base/**`, usecase lain.

Test lintas feature (`apikey_integration_test.go`) dimiliki W; usecase ini menyumbang kasus di bagian Test Scenario dan memastikan lulus dengan Init yang ada.

## Test Scenario

Penamaan: `TestT04_AC<n>_...` untuk kriteria penerimaan; lainnya `Test<Fungsi>_<Kasus>`.

Unit entity (sudah dimiliki F di `base/*_test.go`; usecase ini TIDAK menduplikasi, hanya memastikan ada): format/parse key (`GenerateRawKey`/`ParseRawKey`), nama 2-60 rune, `CheckActiveLimit` (19/20 lolos, 20/20 dan 25/20 -> 409 `KEY_LIMIT_REACHED` `limit=20`), `NewKey` tidak menyimpan secret, `SecretHash == SHA256(secret)`.

Unit usecase (`usecase_test.go`; fake repo pencatat urutan, fake access, fake audit pencatat event, clock tetap, `rnd` deterministik `bytes.NewReader`):
- Urutan panggilan sukses: `access.Require` -> `InTx` { `LockProjectKeys` -> `CountActive` -> `Insert` } -> `audit.Record`. Audit dipanggil setelah `InTx` selesai (pencatatan urutan global). `Require` dipanggil sebelum `GenerateRawKey` (reader tidak terbaca bila access menolak: sisa byte reader utuh).
- Access menolak `ErrProjectNotFound`: error itu dikembalikan apa adanya, repo 0 panggilan, audit 0, reader tak terbaca. Access error infra (non-`*errs.Error`) -> 500-style error, repo 0, audit 0.
- Identitas tak ada di ctx -> `CodeUnauthorized`; access dan repo tidak dipanggil.
- Nama `"A"`, spasi saja, `"a\tb"`, 61 rune -> `INVALID_ARGUMENT` dengan `fields.name`; repo 0, audit 0, reader tak terbaca. Nama `"  claude  "` -> disimpan `"claude"`. 60 rune multibyte lulus.
- Limit: fake `CountActive` = limit -> `KEY_LIMIT_REACHED` (`details.limit`), `Insert` TIDAK dipanggil, audit 0. Count = limit-1 -> sukses.
- `Insert` mengembalikan `ErrNameTaken` -> 409 `NAME_TAKEN`, audit 0. Mengembalikan `ErrProjectNotFound` -> 404, audit 0.
- Retry prefix: `Insert` mengembalikan `ErrPrefixCollision` dua kali lalu sukses -> `InTx` dipanggil 3 kali, setiap percobaan memakai `Prefix` dan `SecretHash` berbeda (reader menyediakan byte berbeda), hasil memakai key percobaan ke-3 (`raw_key` = key ke-3), audit tepat 1 dengan prefix ke-3. Tiga kali tabrakan -> error non-`*errs.Error` yang `errors.Is(err, base.ErrPrefixCollision)`; `InTx` tepat 3 kali; audit 0.
- Error infra pada `LockProjectKeys`/`CountActive`/`Insert` -> error terbungkus, bukan `*errs.Error`, audit 0.
- Reader gagal (`iotest.ErrReader`) -> error "generate key" terbungkus; repo 0, audit 0.
- Hasil sukses: `Key.Status=="active"`, `CreatedBy=actor.ID`, `CreatedByLabel=email`, `RawKey` panjang 55 dan `== "gr_"+Key.Prefix+"_"+secret`, `MCPURL` nil bila `publicURL==""` dan `"https://g.tim.id/mcp"` bila diisi.
- Audit sukses: tepat 1 event `Action=apikey.created`, `ProjectID`, `KeyID=created.ID`, `Args == {"name": nama, "prefix": prefix}` (tidak ada key lain), `Meta` diteruskan, `Actor.Label=email`.
- Raw key tidak bocor: logger berbuffer; setelah skenario sukses dan skenario retry, output log tidak mengandung `RawKey.Full` maupun secret; `Event.Args` dan seluruh isi yang dilihat fake repo (termasuk `Key` yang di-`fmt.Sprintf("%+v")`) tidak mengandung secret/raw key.
- Berkas sumber feature tidak meng-import `math/rand` (tes memindai `go/parser` folder `createkey`).

Handler (`handler_test.go`, httptest + chi router dengan pola `/projects/{projectID}/api-keys`):
- 201 + JSON sesuai contoh (kunci snake_case, `mcp_url` null atau string, `last_used_at: null`, waktu RFC3339 UTC), header `Cache-Control: no-store`; `{projectID}` dari URL diteruskan ke usecase.
- Body `{"nama":"x"}`, kosong, JSON rusak, `name` bertipe angka -> 400; header `Cache-Control: no-store` tetap ada.
- Error validasi -> 400 dengan `error.details.fields.name`; `ErrProjectNotFound` -> 404 `project tidak ditemukan`; `ErrNameTaken` -> 409 `details.reason=NAME_TAKEN`; limit -> 409 `details.reason=KEY_LIMIT_REACHED` + `details.limit`; error biasa -> 500 generik tanpa bocor pesan internal; `trace_id` ada di semua error.
- `UserAgent` > 512 byte dipotong di `RequestMeta` (tanpa memecah rune); `IP` dari `KeyByIP`.
- Body respons error tidak pernah memuat `raw_key`.

Repository integrasi (`repository_integration_test.go`, Postgres nyata, fixture project via SQL `INSERT INTO projects (name) VALUES ($1) RETURNING id`):
- Sukses: baris `api_keys` ada; `id` dibangkitkan DB; `created_at` UTC; `secret_hash` 32 byte `== sha256(secret)`; `created_by_label` tersimpan; `last_used_at`, `revoked_*` NULL.
- AC2 tabel: `SELECT row_to_json(k)::text FROM api_keys k` dan seluruh kolom bertipe teks tidak mengandung `raw_key` maupun `secret`; `prefix` (8 huruf kecil/angka) adalah satu-satunya fragmen key yang tersimpan; tidak ada kolom `secret`/`raw_key` (introspeksi `information_schema.columns`).
- Nama kembar aktif -> `ErrNameTaken`; setelah key lama di-`UPDATE ... SET revoked_at = now()` (SQL fixture) nama sama boleh (AC4). `"Claude"` dan `"claude"` keduanya boleh (K11).
- Prefix kembar (dua insert dengan prefix sama) -> `ErrPrefixCollision`; project_id acak yang tidak ada -> `ErrProjectNotFound`.
- Atomik: `fn` mengembalikan error setelah `Insert` -> tabel tetap kosong (rollback).
- CHECK menolak nama 1 karakter / prefix `ABCDEFGH` / hash 31 byte bila entity dilewati (jaring pengaman).
- Limit serentak (AC5): limit 1 dengan 0 key aktif, 8 goroutine memanggil `Execute` penuh (usecase asli + repo nyata + access palsu "allow") dengan nama berbeda -> tepat satu sukses, sisanya `KEY_LIMIT_REACHED`, `count(*) aktif == 1`. Kasus "sisa 1 slot": limit 3, 2 key aktif, 2 create serentak -> tepat satu 201 dan satu 409; ulangi 10 kali dengan project baru (tidak flaky karena advisory lock).
- Create di project berbeda tidak saling memblokir: 2 project paralel, keduanya sukses pada limit 1 (lock per project).

Integrasi lintas feature (ditambahkan ke `apikey_integration_test.go` oleh W, tag `integration`; usecase ini menyumbang kasus berikut; fixture: A owner P1, B member non-owner P1, C bukan anggota, S super_admin):
- `TestT04_AC1_MemberCreatesKey_NonMember404_SuperAdminOK`: B `POST` -> 201; C `POST` -> 404 `project tidak ditemukan` (tidak ada baris `api_keys` baru, 0 audit); S `POST` -> 201; projectID `bukan-uuid` -> 404; project acak yang tidak ada -> 404 (pesan sama dengan C).
- `TestT04_AC2_PlaintextOnlyInCreateResponse` bagian create: respons memuat `raw_key` 55 karakter cocok pola `^gr_[a-z0-9]{8}_[A-Za-z0-9_-]{43}$` dan `key.prefix == raw_key[3:11]`; header `Cache-Control: no-store`; secret tidak ditemukan di tabel.
- `TestT04_AC3_...` (bagian awal): key hasil create langsung lolos `project.apikey.verify` (`Found=true`, KeyID/KeyName/Prefix/ProjectID sama dengan respons).
- `TestT04_AC4_DuplicateActiveNameRejected_ReusableAfterRevoke` bagian create: "ci" 201 -> "ci" 409 `NAME_TAKEN` -> (revoke dari U6) -> "ci" 201.
- `TestT04_AC5_ActiveLimitEnforced` bagian create: config uji limit 3; 3 sukses, ke-4 409 `KEY_LIMIT_REACHED` `limit=3`; dua create serentak saat sisa 1 slot (diulang 10 kali) -> tepat satu 201 dan satu 409, jumlah aktif == limit.
- `TestT04_AC6_CreatorRemovalDoesNotKillKey`: A membuat key; check-access palsu diubah agar A `none`; `verify` key itu tetap `Found=true`.
- `TestT04_AC9_AuditCreated`: tepat satu `apikey.created` dengan `ActorLabel` email, `ProjectLabel` nama project, `Target` = key id, `Args == {name, prefix}`; request audit tidak mengandung raw key/secret (serialisasi JSON request diperiksa); create gagal (400/404/409) -> 0 audit; handler `core.audit.record` yang error atau tidak terdaftar -> respons tetap 201 dan error ter-log tanpa `args`; get-project error -> audit tetap terkirim dengan `ProjectLabel=""`.
- `TestT04_Security_NoSecretInLogs` bagian create: logger berbuffer selama create (sukses, 409, 400); output tidak mengandung raw key/secret.
- CSRF: `POST` tanpa `X-Requested-With` -> 403 `CSRF_REQUIRED`; tanpa sesi -> 401 (middleware T00; test mengecek `code`).
- `TestT04_CascadeOnProjectDelete` bagian awal: key hasil create hilang saat project dihapus.
- `TestRoutesRegistered` memuat `POST /api/projects/{projectID}/api-keys`.

## E2E Scenarios

Positif (Playwright terhadap binary; butuh T01, T03 merge dan UI dari frontend-plan):
1. Developer B (member, bukan owner) login -> buka project -> tab "API Keys" kosong -> "Buat key" -> isi "claude-code-budi" -> submit -> layar hasil menampilkan key `gr_...` lengkap, tombol salin, peringatan "tidak akan ditampilkan lagi", potongan `claude mcp add --transport http claude-code-budi <URL>/mcp --header "Authorization: Bearer gr_..."` dan JSON generik. `<URL>` = `mcp_url` tanpa `/mcp` bila ada, selain itu `window.location.origin`.
2. Tutup dialog -> tabel memuat baris nama, prefix, dibuat oleh `b@tim.id`, terakhir dipakai "-", status aktif; reload halaman -> key lengkap tidak tampil di mana pun.
3. Developer A (owner) juga bisa membuat key; B melihat key buatan A di tabel (key tidak terikat pembuat).
4. Super Admin (bukan anggota) membuka project -> membuat key sukses; tautan Audit Log terfilter tampil dan memuat `apikey.created` dengan nama + prefix (tanpa key).
5. Buat key dengan nama yang sama dengan key yang sudah dicabut -> sukses.
6. Key hasil create dipakai pada klien MCP uji (T07 bila sudah ada) -> diterima; bila T07 belum ada, diverifikasi lewat Go integrasi `verify`.

Negatif:
1. Developer C (bukan anggota) membuka tab API Keys project P1 lewat URL langsung -> halaman "tidak ditemukan"; `POST /api/projects/<P1>/api-keys` -> 404 `project tidak ditemukan`.
2. Nama 1 karakter -> pesan field `name` "nama key wajib 2-60 karakter"; tidak ada key terbuat, tidak ada audit.
3. Nama 61 karakter atau berisi karakter kontrol (lewat API) -> 400 `fields.name`.
4. Nama yang dipakai key aktif -> pesan i18n `project.apikey.error.NAME_TAKEN`; dialog tetap terbuka dan input terjaga.
5. Batas key aktif tercapai -> pesan i18n `project.apikey.error.KEY_LIMIT_REACHED` memuat angka batas dari `details.limit`.
6. Tanpa header `X-Requested-With` -> 403 `CSRF_REQUIRED`; tanpa login -> 401; body dengan field tak dikenal (`{"nama":"x"}`) -> 400.
7. Submit ganda cepat: UI menonaktifkan tombol saat request berjalan; bila dua request lolos (via API), kedua dengan nama sama -> satu 201, satu 409 `NAME_TAKEN`.
8. Setelah menutup dialog hasil, tidak ada cara memunculkan key lagi (tombol/endpoint tidak ada); `GET /api/projects/<P1>/api-keys` tidak memuat `raw_key`.

## Request/Response body

Format error seragam T00:

```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "permintaan tidak valid", "details": { "fields": { "name": "nama key wajib 2-60 karakter" } } }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

### POST /api/projects/{projectID}/api-keys

Header: `X-Requested-With: grimoire`, `Content-Type: application/json`, cookie sesi.

Request:
```json
{ "name": "claude-code-budi" }
```

201, header respons `Cache-Control: no-store`, `Pragma: no-cache`, aktor developer B (member), `PUBLIC_URL=https://grimoire.tim.id`:
```json
{
  "key": {
    "id": "0192f5a0-2222-7aaa-8bbb-000000000001",
    "name": "claude-code-budi",
    "prefix": "k3x9q2ma",
    "status": "active",
    "created_by": "0192f5a0-bbbb-7aaa-8bbb-00000000000b",
    "created_by_label": "b@tim.id",
    "created_at": "2026-10-23T09:00:00Z",
    "last_used_at": null,
    "revoked_at": null,
    "revoked_by": null,
    "revoked_by_label": null
  },
  "raw_key": "gr_k3x9q2ma_Q2xhdWRlLWNvZGUtYnVkaS1zZWNyZXQtZXhhbXBsZS0xMjM0NTY",
  "mcp_url": "https://grimoire.tim.id/mcp"
}
```

201, `PUBLIC_URL` tidak diset, aktor super_admin (bukan anggota):
```json
{
  "key": {
    "id": "0192f5a0-2222-7aaa-8bbb-000000000002",
    "name": "ci-pipeline",
    "prefix": "a1b2c3d4",
    "status": "active",
    "created_by": "0192f5a0-ffff-7aaa-8bbb-00000000000f",
    "created_by_label": "admin@tim.id",
    "created_at": "2026-10-23T09:05:00Z",
    "last_used_at": null,
    "revoked_at": null,
    "revoked_by": null,
    "revoked_by_label": null
  },
  "raw_key": "gr_a1b2c3d4_c2VjcmV0LWV4YW1wbGUtYmVkYS1jaS1waXBlbGluZS0wMDA",
  "mcp_url": null
}
```

400, validasi nama (panjang):
```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "permintaan tidak valid", "details": { "fields": { "name": "nama key wajib 2-60 karakter" } } }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

400, karakter kontrol:
```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "permintaan tidak valid", "details": { "fields": { "name": "nama key tidak boleh mengandung karakter kontrol" } } }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

400, field tak dikenal (`{"nama":"x"}`), body kosong atau JSON rusak: format error 400 dari `respond.DecodeJSON` (`code: INVALID_ARGUMENT`).

401 (tanpa sesi):
```json
{ "error": { "code": "UNAUTHORIZED", "message": "perlu login", "details": null }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

403 (header CSRF hilang, dari middleware T00; test mengecek `code`, bukan teks pesan):
```json
{ "error": { "code": "CSRF_REQUIRED", "message": "header X-Requested-With wajib", "details": null }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

404 (bukan anggota, project tidak ada, atau projectID bukan uuid):
```json
{ "error": { "code": "NOT_FOUND", "message": "project tidak ditemukan", "details": null }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

409, nama dipakai key aktif:
```json
{ "error": { "code": "CONFLICT", "message": "nama key sudah dipakai key aktif lain di project ini", "details": { "reason": "NAME_TAKEN" } }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

409, batas key aktif:
```json
{ "error": { "code": "CONFLICT", "message": "batas 20 key aktif per project tercapai; cabut key yang tidak dipakai", "details": { "reason": "KEY_LIMIT_REACHED", "limit": 20 } }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

500 (termasuk tabrakan prefix 3 kali dan kegagalan check-access; pesan generik, detail hanya di log tanpa key):
```json
{ "error": { "code": "INTERNAL", "message": "terjadi kesalahan", "details": null }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

### Bus yang dipanggil (Go, bukan wire JSON)

```go
// 1. otorisasi (sebelum tx), fail-closed
checkaccess.Request{ProjectID: "0192f5a0-1111-7aaa-8bbb-000000000001", UserID: "0192f5a0-bbbb-7aaa-8bbb-00000000000b", UserRole: "developer"}
// -> checkaccess.Response{ProjectExists: true, Level: checkaccess.LevelMember}

// 2. setelah commit, fail-soft (label project)
getproject.Request{ProjectID: "0192f5a0-1111-7aaa-8bbb-000000000001"}
// -> getproject.Response{Found: true, ID: "...", Name: "Payments"}

// 3. setelah commit, fail-open
record.Request{
    ActorType: record.ActorUser, ActorID: "0192f5a0-bbbb-7aaa-8bbb-00000000000b", ActorLabel: "b@tim.id",
    ProjectID: "0192f5a0-1111-7aaa-8bbb-000000000001", ProjectLabel: "Payments",
    Action: record.ActionAPIKeyCreated, TargetType: "api_key", Target: "0192f5a0-2222-7aaa-8bbb-000000000001",
    Args: map[string]any{"name": "claude-code-budi", "prefix": "k3x9q2ma"},
    Status: record.StatusOK, IP: "10.0.0.5", UserAgent: "Mozilla/5.0", TraceID: "01JB2X9K6Q4Y7V3N8M5T2R1C0A",
}
```
