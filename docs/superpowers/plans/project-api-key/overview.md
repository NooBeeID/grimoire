# T04 project/api-key — Overview Plan (FOUNDATION)

Tiket: `docs/tickets/T04-project-api-key.md`
Sumber aturan: `rules.txt` (menang untuk arsitektur), `docs/tickets/README.md` (konvensi, katalog bus, katalog audit), `docs/superpowers/specs/2026-10-20-grimoire-design.md` (produk), `docs/superpowers/plans/project-management/overview.md` (T03: kontrak `check-access`, `get-project`, `contentcount`, pola `base/`), `docs/superpowers/plans/core-audit-log/overview.md` (kontrak `core.audit.record`).
Konteks yang dicek: belum ada `CLAUDE.md` dan `.claude/rules/`. Tidak ada knowledge doc module api-key. Kode T00 dan T02 di `main`; T03 baru plan (belum ada `backend/migrations/project/*.sql`, belum ada `lib/contracts/project/**`). `api/modules/project/management.yml` sudah ada di `main` (ikut commit plan T03) dan dipakai sebagai pola penulisan OpenAPI.

Dokumen ini adalah plan FOUNDATION untuk module `project/api-key`. Isinya keputusan bersama, `base/`, kontrak bus, migrasi, urutan, dan daftar usecase. Plan rinci per usecase ditulis terpisah di `docs/superpowers/plans/project-api-key/usecase-<slug>.md` dan WAJIB mengikuti dokumen ini. Bila plan usecase bertentangan dengan dokumen ini, dokumen ini yang menang sampai diperbarui. Frontend ditulis di `docs/superpowers/plans/project-api-key/frontend-plan.md` (terpisah).

---

## 0. Prasyarat dan nama infra yang dipakai

- T04 bergantung pada T03. Dari T03 dipakai PERSIS (tidak didefinisikan ulang):
  - tabel `projects` (migrasi `backend/migrations/project/20261022090000000_create_projects.sql`);
  - `lib/contracts/project/management/checkaccess` (`Request{ProjectID, UserID, UserRole}` -> `Response{ProjectExists bool; Level Level}`, `LevelNone|LevelMember|LevelOwner|LevelAdmin`);
  - `lib/contracts/project/management/getproject` (`Request{ProjectID}` -> `Response{Found bool; ID, Name string}`);
  - `lib/contracts/project/management/contentcount` (`Request{ProjectIDs []string}` -> `Response{Counts map[string]int}`, `MaxProjectIDs = 100`);
  - konstanta `BUS_PROJECT_MANAGEMENT_CHECK_ACCESS`, `BUS_PROJECT_MANAGEMENT_GET_PROJECT`, `BUS_PROJECT_APIKEY_COUNT_KEYS`.
- Aturan "tiket yang merge lebih dulu membuat kontrak": bila T03 belum merge saat T04 dikerjakan, file kontrak dan konstanta di atas dibuat PERSIS seperti `docs/superpowers/plans/project-management/overview.md` bagian 5.1-5.4, dan migrasi `projects` dibuat persis seperti bagian 6 plan itu (migrasi T04 punya FK ke `projects`). Bila sudah ada di `main`, pakai yang ada; DILARANG duplikasi.
- Signature publik T00 TIDAK diubah. T04 hanya MENAMBAH konstanta, kontrak, migrasi, module, field config, dan OpenAPI.

| Simbol | Import path | Package |
|---|---|---|
| `Deps{Ctx, Config, Log, DB, Bus, Jobs, API, MCP}` | `grimoire/internal/infra/app` | `app` |
| `Registry.Register`, `Registry.Call`, `Registry.Publish`, `NewRegistry`, `CallTyped[T]`, `Expect[T]`, `ErrNotRegistered`, `ErrBadRequestType` | `grimoire/internal/infra/bus` | `bus` |
| `WriteJSON(w, status, v)`, `WriteError(w, r, err)`, `DecodeJSON(r, dst)`, `StatusFor` | `grimoire/internal/infra/http/respond` | `respond` |
| `RequireAuth()`, `KeyByIP(r)`, `CSRFHeader` | `grimoire/internal/infra/middleware` | `middleware` |
| `DB` (embed `*sqlx.DB`), `DB.BeginTx(ctx, func(*sqlx.Tx) error)`, `Querier`, `TestDB(t)` | `grimoire/internal/infra/db` | `db` |
| `Config` (ditambah 2 field, K8) | `grimoire/internal/infra/config` | `config` |
| `New`, `Wrap`, `WithDetails`, `Error`, `CodeNotFound`, `CodeConflict`, `CodeInvalid`, `CodeForbidden`, `CodeUnauthorized`, `ErrInvalid` | `grimoire/lib/common/errs` | `errs` |
| `IdentityFrom(ctx)` (`Identity{UserID, Email, Name, Role}`), `WithIdentity`, `TraceID(ctx)` | `grimoire/lib/common/reqctx` | `reqctx` |
| `Request`, `Response`, `ActorUser`, `StatusOK`, `ActionAPIKeyCreated`, `ActionAPIKeyRevoked` | `grimoire/lib/contracts/core/audit/record` | `record` |
| `*pgconn.PgError` (`Code`, `ConstraintName`) | `github.com/jackc/pgx/v5/pgconn` | `pgconn` |

- Tidak ada kode error baru, tidak ada `RegisterCodeStatus`. Kasus khusus dibedakan lewat `details.reason` (K9).
- Tidak ada library baru (`pgx/v5`, `sqlx`, `chi`, `google/uuid` sudah di `go.mod`). Kripto hanya stdlib (`crypto/rand`, `crypto/sha256`, `crypto/subtle`, `encoding/base64`).
- Tidak ada job terjadwal, tidak ada cache.

---

## 1. Metadata

| Atribut | Nilai |
|---|---|
| Ticket | T04 |
| Scope | fullstack (backend Go di plan ini; frontend di `frontend-plan.md`) |
| Domain | project |
| Module | api-key (`apikey`) |
| Resource root | `backend/internal/domain/project/internal/apikey` |
| Versi kontrak REST | Tanpa prefix versi di URL (prefix `/api`). `api/modules/project/api-key.yml` memakai `info.version: 0.1.0` |
| Versi kontrak bus | v1 (endpoint baru) |
| Bergantung pada | T00 (merged), T02 (merged), T03 (plan siap, belum merge) |
| Dibutuhkan oleh | T07 (`verify`, `touch`), T03 (`count-keys`, fail-soft) |

Endpoint HTTP (semua di `d.API`, semua `middleware.RequireAuth()`; mutasi wajib header `X-Requested-With: grimoire` dari CSRF T00):

| Endpoint | Method | Version | Siapa (otorisasi di usecase via check-access) | Usecase |
|---|---|---|---|---|
| `/api/projects/{projectID}/api-keys` | POST | - | anggota (member/owner), super_admin | `createkey` |
| `/api/projects/{projectID}/api-keys` | GET | - | anggota, super_admin | `listkeys` |
| `/api/projects/{projectID}/api-keys/{keyID}/revoke` | POST | - | anggota, super_admin | `revokekey` |

Endpoint bus (provider T04):

| Endpoint | Mode | Version | Konstanta | Kontrak | Consumer | Usecase |
|---|---|---|---|---|---|---|
| `project.apikey.verify` | Call | v1 | `BUS_PROJECT_APIKEY_VERIFY` | `lib/contracts/project/apikey/verify` | T07 | `verifykey` |
| `project.apikey.touch` | Publish | v1 | `BUS_PROJECT_APIKEY_TOUCH` | `lib/contracts/project/apikey/touch` | T07 | `touchlastused` |
| `project.apikey.count-keys` | Call | v1 | `BUS_PROJECT_APIKEY_COUNT_KEYS` (milik T03) | `lib/contracts/project/management/contentcount` (milik T03) | T03 (fail-soft) | `countkeys` |

Endpoint bus yang dikonsumsi:

| Endpoint | Mode | Provider | Sifat | Dipakai oleh |
|---|---|---|---|---|
| `project.management.check-access` | Call | T03 | fail-closed (error -> 500) | `createkey`, `listkeys`, `revokekey` (lewat `base/access`) |
| `project.management.get-project` | Call | T03 | fail-soft (label audit kosong) | `createkey`, `revokekey` (lewat `base/audit`, setelah commit) |
| `core.audit.record` | Call | T02 | fail-open (log error) | `createkey`, `revokekey` (lewat `base/audit`, setelah commit) |

`core.auth.get-user` TIDAK dikonsumsi (K2).

---

## 2. Summary Plan

T04 membangun kredensial agent: key bernama per project, plaintext ditampilkan sekali, hanya `SHA-256(secret)` yang disimpan, bisa dicabut dengan efek langsung, dan bisa diverifikasi gateway MCP (T07) lewat bus dengan satu query berindeks dan perbandingan waktu-konstan.

Isinya:
- Satu tabel milik module (`api_keys`) dalam satu migrasi `migrations/project/`, satu-satunya FK lintas module `project_id -> projects(id) ON DELETE CASCADE`.
- Rule murni di `base/`: pembuatan/parsing raw key (`gr_<prefix8>_<secret43>`), hash, validasi nama, batas key aktif, pencabutan, hasil verifikasi waktu-konstan, DTO.
- Tiga subpackage I/O bersama: `base/store` (scan baris + helper SQL bersama), `base/access` (otorisasi via `check-access`), `base/audit` (audit via bus + label project via `get-project`).
- Enam feature: tiga HTTP (`createkey`, `listkeys`, `revokekey`), tiga handler bus (`verifykey`, `touchlastused`, `countkeys`).
- Dua kontrak bus baru (`verify`, `touch`) dan provider untuk kontrak consumer-driven T03 (`count-keys`).
- Dua field config baru (`API_KEY_MAX_ACTIVE_PER_PROJECT`, `PUBLIC_URL`).
- OpenAPI `api/modules/project/api-key.yml`, pembaruan README dan tiket T04.

Hal penting soal keamanan:
- Plaintext key/secret tidak pernah disimpan, di-log, atau di-audit; hanya `prefix`. Respons `createkey` ber-header `Cache-Control: no-store`.
- Verifikasi selalu menghitung SHA-256 + `subtle.ConstantTimeCompare` (dengan `DummyHash` saat prefix tak dikenal) sebelum memeriksa status revoked, sehingga jalur unknown/mismatch/revoked setara waktunya (AC8).
- Tanpa cache: revoke berlaku di panggilan `verify` berikutnya (AC3).
- Project yang tidak boleh dilihat pemanggil -> 404 `project tidak ditemukan` (juga id bukan uuid). Key milik project lain -> 404 `KEY_NOT_FOUND`.
- Batas key aktif ditegakkan di dalam transaksi dengan advisory lock per project, sehingga dua create serentak tidak bisa melampaui batas.

---

## 3. Keputusan bersama (mengikat semua usecase)

### 3.1 Keputusan lintas usecase (dicatat di PR; tiket T04 dan README diperbarui di PR yang sama)

| # | Keputusan | Alasan | Tindakan dokumen |
|---|---|---|---|
| K1 | Enam feature: `createkey`, `listkeys`, `revokekey` (HTTP), `verifykey`, `touchlastused`, `countkeys` (bus) | `countkeys` dibutuhkan karena T03 D1 menetapkan T04 sebagai provider `project.apikey.count-keys` (hitungan key AKTIF) | Tambah baris `countkeys` di tabel Features T04 dan baris di bagian "Disediakan" |
| K2 | Label aktor (`created_by_label`, `revoked_by_label`, audit `ActorLabel`) = `reqctx.Identity.Email`, bukan `core.auth.get-user` | Middleware Session sudah menolak user nonaktif per request (konsisten T03 D6) | Hapus `core.auth.get-user` dari "Dikonsumsi" tiket T04; README katalog: hapus T04 dari consumer `core.auth.get-user` |
| K3 | Otorisasi HTTP hanya lewat `project.management.check-access`, fail-closed (error bus -> 500). `!ProjectExists` atau `Level == none` -> 404 `project tidak ditemukan` (juga projectID bukan uuid). Level `member`/`owner`/`admin` semuanya boleh create/list/revoke | Rule 7.2: tidak ada logika keanggotaan di luar T03. Tiket: anggota (bukan hanya owner) boleh kelola key | - |
| K4 | Route didaftarkan dengan pola penuh di `d.API` (T03 D4), param WAJIB `{projectID}` dan `{keyID}` | Menghindari konflik node chi dengan route T03/T05/T06 | Tiket T04: ganti `{id}`/`{keyId}` di tabel Features menjadi `{projectID}`/`{keyID}` |
| K5 | Audit: `bus.Call` ke `core.audit.record` SETELAH commit, fail-open. `ProjectLabel` dari `project.management.get-project` setelah commit, fail-soft. Panggilan bus TIDAK PERNAH di dalam tx | Pola T03 D5 + keputusan T02 | - |
| K6 | Tidak ada cache verifikasi. Verifikasi = satu query pada `prefix` (unique index) | Tiket: pencabutan harus langsung berlaku | - |
| K7 | Throttle `last_used_at` ditegakkan di SQL: `UPDATE ... WHERE id = $1 AND (last_used_at IS NULL OR last_used_at <= $2 - interval '1 minute')` | Aman multi-instance, tanpa state proses | - |
| K8 | Config baru: `APIKeyMaxActivePerProject int env:"API_KEY_MAX_ACTIVE_PER_PROJECT"` default 20 min 1; `PublicURL string env:"PUBLIC_URL"` opsional, bila diisi wajib URL absolut http/https dengan host, tanpa query/fragment, trailing slash dibuang. `createkey` mengembalikan `mcp_url = PublicURL + "/mcp"` atau `null` | Tiket: batas dapat dikonfigurasi; URL server untuk potongan konfigurasi MCP | `docs/configuration.md`, `.env.example`, tiket T04 (bagian Aturan bisnis: nama env) |
| K9 | Tidak ada kode error baru; `details.reason`: `NAME_TAKEN` (409), `KEY_LIMIT_REACHED` (409, `{reason, limit}`), `ALREADY_REVOKED` (409), `KEY_NOT_FOUND` (404). Validasi -> 400 `INVALID_ARGUMENT` `{fields: {name: "..."}}` | Konsisten T03 | Tiket T04: catat daftar reason |
| K10 | Plaintext key dan secret DILARANG di-log, di-audit, atau disimpan. Log/audit hanya memuat `prefix`. Respons `createkey` ber-header `Cache-Control: no-store` | Tiket: plaintext hanya ada di respons `createkey` | - |

Keputusan tambahan foundation (melengkapi K1-K10, tidak mengubah tiket):

| # | Keputusan |
|---|---|
| K11 | Nama key unik case-sensitive (persis setelah trim), sesuai unique index `(project_id, name)`. `"Claude"` dan `"claude"` dianggap berbeda. |
| K12 | Base64url secret di-decode dengan `base64.RawURLEncoding.Strict()` agar satu key hanya punya satu representasi string (bit sisa karakter terakhir wajib nol). |
| K13 | Retry tabrakan prefix (23505 pada `api_keys_prefix_key`) diorkestrasi usecase `createkey`: tiap percobaan generate key baru dan transaksi baru (tx lama sudah abort). Maks 3 percobaan, lalu 500. |
| K14 | Respons `verify` untuk format salah tidak menyentuh DB. Ini tidak membocorkan informasi rahasia (format bersifat publik), jadi tidak perlu disetarakan waktunya dengan jalur DB. |

### 3.2 Struktur module

```
backend/internal/domain/project/
  base.go                         # ubah: tambah apikey.Register(d) setelah management.Register(d)
  internal/apikey/
    base.go                       # package apikey: Register(d app.Deps) memanggil 6 <feature>.Init(d)
    apikey_integration_test.go    # W: lintas feature (tag integration)
    base/                         # package base: entity + rule, TANPA I/O, TANPA context
      rawkey.go     rawkey_test.go
      key.go        key_test.go
      verify.go     verify_test.go
      dto.go        dto_test.go
      errors.go     errors_test.go
      actor.go  meta.go  clock.go
    base/store/                   # package store: scan + helper SQL bersama
      store.go  pgerr.go  store_integration_test.go
    base/access/                  # package access: otorisasi via check-access
      access.go  access_test.go
    base/audit/                   # package audit: audit via bus (fail-open) + label project (fail-soft)
      audit.go  audit_test.go
    createkey/ listkeys/ revokekey/ verifykey/ touchlastused/ countkeys/
      init.go handler.go [params.go] usecase.go repository.go *_test.go
```

Aturan per feature:
- `init.go`: `func Init(d app.Deps)`. Merakit `store.New(d.DB)` -> repository feature -> usecase (menerima interface) -> handler, lalu mendaftarkan route atau handler bus feature itu sendiri.
- Feature bus tanpa `params.go`. `handler.go` memanggil `bus.Expect[<kontrak>.Request](req)` (rule 6.9).
- Usecase menerima interface: repository feature, `access.Checker`, `audit.Recorder`, `base.Clock`, `io.Reader` (sumber acak, hanya `createkey`), `int` limit dari `d.Config.APIKeyMaxActivePerProject`, `*string`/`string` PublicURL. Bukan tipe konkret (rule 5.6).
- Feature DILARANG saling import (rule 4.3). Yang dipakai bersama lewat `base`, `base/store`, `base/access`, `base/audit`. Arah import: `base/store|access|audit` -> `base`, tidak sebaliknya.
- `BeginTx` dipanggil dari repository feature (`func (r *Repo) InTx(ctx, fn func(*Repo) error) error`), bukan dari usecase. Repository feature membungkus `*store.Store` (`r.s.WithTx(tx)` di dalam tx). Closure `fn` ditulis di usecase dan hanya memanggil method repository + rule entity (tanpa bus).
- SQL khusus feature ada di `repository.go` masing-masing. `base/store` hanya berisi scan baris ke entity, konstanta kolom, validasi uuid, dan klasifikasi error Postgres.

### 3.3 Konvensi handler, actor, request meta, dan log

- Handler membaca `chi.URLParam(r, "projectID")` / `chi.URLParam(r, "keyID")` apa adanya. Validasi uuid TIDAK di handler: `check-access` menjawab `ProjectExists=false` untuk projectID bukan uuid (404), dan store mengembalikan "tidak ketemu" untuk keyID bukan uuid tanpa query (404 `KEY_NOT_FOUND`).
- Handler membangun `base.NewRequestMeta(middleware.KeyByIP(r), r.UserAgent())` (UA dipotong 512 byte di batas rune).
- Usecase HTTP membangun aktor sekali: `actor, ok := base.ActorFrom(reqctx.IdentityFrom(ctx))`. `!ok` -> `errs.New(errs.CodeUnauthorized, "perlu login")`. `Actor{ID: Identity.UserID, Role: Identity.Role, Label: Identity.Email}`.
- Body JSON di-decode dengan `respond.DecodeJSON` (field tak dikenal -> 400). Validasi isi (nama) di entity.
- Error infra dibungkus `fmt.Errorf("<feature>: <langkah>: %w", err)` dan sampai ke klien sebagai 500 generik. Error bisnis selalu `*errs.Error` dari `base/errors.go`.
- Sukses: `respond.WriteJSON`. Gagal: `respond.WriteError`.
- Log field standar: `feature`, `project_id`, `key_id`, `prefix`. DILARANG me-log body request/response, `RawKey`, `Secret`, `SecretHash`, atau `record.Request.Args`.

### 3.4 Otorisasi HTTP (dipakai 3 route, lewat `base/access`)

Urutan wajib di awal tiap usecase HTTP:
1. Bangun aktor (3.3).
2. `if err := uc.access.Require(ctx, projectID, actor); err != nil { return ..., err }`.
3. Lanjut langkah feature.

Perilaku `access.Require` (K3):

| Hasil `check-access` | Hasil `Require` | HTTP |
|---|---|---|
| error (termasuk `bus.ErrNotRegistered`) | `fmt.Errorf("access: check-access: %w", err)` | 500 |
| `!ProjectExists` | `base.ErrProjectNotFound` | 404 |
| `Level == none` atau level tidak dikenal | `base.ErrProjectNotFound` | 404 |
| `member` / `owner` / `admin` | `nil` | lanjut |

### 3.5 Konkurensi

- `createkey` (satu tx di repository, per percobaan):
  1. `SELECT pg_advisory_xact_lock(hashtextextended('apikey:' || $1::text, 0))` (mutex per project untuk pembuatan key; lepas otomatis saat commit/rollback).
  2. `SELECT count(*) FROM api_keys WHERE project_id = $1 AND revoked_at IS NULL`.
  3. `base.CheckActiveLimit(count, limit)` (di closure usecase).
  4. `INSERT INTO api_keys (project_id, name, prefix, secret_hash, created_by, created_by_label, created_at) VALUES (...) RETURNING <keyColumns>`.
  - 23505 constraint `api_keys_project_active_name_key` -> `base.ErrNameTaken` (409).
  - 23505 constraint `api_keys_prefix_key` -> `base.ErrPrefixCollision` (sentinel internal) -> usecase mengulang dengan key baru (K13).
  - 23503 (FK `project_id`, project dihapus di tengah) -> `base.ErrProjectNotFound` (404).
  - Revoke tidak mengambil advisory lock: revoke hanya mengurangi jumlah key aktif, tidak bisa membuat batas terlampaui.
- `revokekey` (tx): `SELECT <keyColumns> FROM api_keys WHERE id = $1 AND project_id = $2 FOR UPDATE` (tidak ada -> `ErrKeyNotFound`) -> `Key.Revoke(actor, now)` (sudah dicabut -> `ErrAlreadyRevoked`) -> `UPDATE api_keys SET revoked_at = $3, revoked_by = $4, revoked_by_label = $5 WHERE id = $1 RETURNING <keyColumns>`. Dua revoke serentak diserialkan oleh `FOR UPDATE`: tepat satu 200, lainnya 409 `ALREADY_REVOKED`, tepat satu audit.
- `touchlastused`: satu statement (K7), tanpa tx.
- `verifykey`, `listkeys`, `countkeys`: satu SELECT, tanpa tx.
- Panggilan bus (check-access, get-project, audit) TIDAK PERNAH di dalam tx. check-access sebelum tx; get-project + audit setelah commit.

### 3.6 Audit (`base/audit`)

| Action | Usecase | TargetType / Target | Args |
|---|---|---|---|
| `apikey.created` (`record.ActionAPIKeyCreated`) | createkey | `api_key` / key id | `{"name": <nama>, "prefix": <prefix>}` |
| `apikey.revoked` (`record.ActionAPIKeyRevoked`) | revokekey | `api_key` / key id | `{"name": <nama>, "prefix": <prefix>}` |

Tepat satu entri per aksi yang sukses. Gagal (400/404/409/500) -> 0 entri. Audit gagal -> respons tetap sukses. Tidak menambah aksi baru di `actions.go`.

---

## 4. Entity dan rule (`base/`, package `base`)

Semua fungsi murni: tanpa context, tanpa I/O jaringan/DB (satu-satunya pembaca adalah `io.Reader` yang diinjeksi ke `GenerateRawKey`). Diuji unit tanpa mock (rule 13.1). Import yang diizinkan: stdlib, `lib/common/errs`, `lib/common/reqctx` (tipe `Identity`).

### 4.1 `base/rawkey.go`

```go
const (
    KeyScheme        = "gr"
    PrefixLen        = 8
    PrefixAlphabet   = "abcdefghijklmnopqrstuvwxyz0123456789"
    SecretBytes      = 32
    SecretEncodedLen = 43                                      // base64.RawURLEncoding(32 byte)
    RawKeyLen        = len(KeyScheme) + 1 + PrefixLen + 1 + SecretEncodedLen // 55
    MaxRawKeyInput   = 256                                     // input lebih panjang langsung ditolak
)

type RawKey struct {
    Full   string // "gr_<prefix>_<secret>"; HANYA untuk respons createkey
    Prefix string
    Secret string
}

// GenerateRawKey membaca byte acak dari r (produksi: crypto/rand.Reader).
// Prefix: tiap byte acak b diterima bila b < 252 (36*7), karakter = PrefixAlphabet[b%36] (rejection sampling,
// tanpa bias); ulangi sampai 8 karakter. Secret: 32 byte dari r, base64.RawURLEncoding.
// Error baca r dibungkus "rawkey: read random: %w".
func GenerateRawKey(r io.Reader) (RawKey, error)

// ParseRawKey memakai posisi tetap, bukan split (secret base64url boleh memuat '_' dan '-'):
//   s = strings.TrimSpace(s); len(s) > MaxRawKeyInput -> false
//   len(s) == RawKeyLen, s[:3] == "gr_", s[3:11] semua di PrefixAlphabet, s[11] == '_',
//   base64.RawURLEncoding.Strict().DecodeString(s[12:]) sukses dan menghasilkan 32 byte (K12).
func ParseRawKey(s string) (prefix, secret string, ok bool)

func HashSecret(secret string) []byte // sha256.Sum256([]byte(secret)), 32 byte
var DummyHash = HashSecret("grimoire-dummy-secret")
```

Catatan: hash dihitung atas string secret ter-encode (43 karakter), bukan byte mentah hasil decode. Keduanya setara secara keamanan; yang penting konsisten antara `NewKey` dan `MatchSecret`.

### 4.2 `base/key.go`

```go
const (
    NameMinRunes = 2
    NameMaxRunes = 60
    TargetType   = "api_key" // TargetType audit
)

type Key struct {
    ID             string
    ProjectID      string
    Name           string
    Prefix         string
    SecretHash     []byte
    CreatedBy      *string
    CreatedByLabel *string
    CreatedAt      time.Time
    LastUsedAt     *time.Time
    RevokedAt      *time.Time
    RevokedBy      *string
    RevokedByLabel *string
}

// ValidateName: strings.TrimSpace; utf8.RuneCountInString 2..60 -> selain itu
//   FieldErrors{"name": "nama key wajib 2-60 karakter"};
//   ada rune unicode.IsControl -> FieldErrors{"name": "nama key tidak boleh mengandung karakter kontrol"}.
// Cek panjang lebih dulu. Mengembalikan nama hasil trim; FieldErrors nil bila valid.
func ValidateName(s string) (string, FieldErrors)

// NewKey: ValidateName (gagal -> InvalidFields); SecretHash = HashSecret(raw.Secret); Prefix = raw.Prefix;
// CreatedBy = &actor.ID, CreatedByLabel = &actor.Label (nil bila Label kosong); CreatedAt = now.UTC().
// ID diisi DB. Secret plaintext TIDAK disimpan di struct.
func NewKey(projectID, name string, raw RawKey, actor Actor, now time.Time) (Key, error)

// CheckActiveLimit: activeCount >= limit -> ErrKeyLimitReached(limit); selain itu nil.
func CheckActiveLimit(activeCount, limit int) error

func (k Key) Active() bool // RevokedAt == nil

// Revoke: !k.Active() -> ErrAlreadyRevoked (struct tidak berubah);
// selain itu RevokedAt = &now.UTC(), RevokedBy = &actor.ID, RevokedByLabel = &actor.Label.
func (k *Key) Revoke(actor Actor, now time.Time) error
```

### 4.3 `base/verify.go`

```go
const (
    ReasonMalformed = "malformed" // nilai string SAMA dengan konstanta kontrak verify (diuji di verifykey)
    ReasonUnknown   = "unknown"
    ReasonRevoked   = "revoked"
    ReasonMismatch  = "mismatch"
)

// MatchSecret = subtle.ConstantTimeCompare(stored, HashSecret(secret)) == 1.
func MatchSecret(stored []byte, secret string) bool

// VerifyOutcome SELALU menghitung hash + compare terlebih dulu:
//   stored := k.SecretHash; if !found { stored = DummyHash }
//   match := MatchSecret(stored, secret)
// lalu, berurutan: !found -> (false, ReasonUnknown); !match -> (false, ReasonMismatch);
// !k.Active() -> (false, ReasonRevoked); selain itu (true, "").
// Urutan "mismatch sebelum revoked": secret salah pada key dicabut dilaporkan mismatch.
func VerifyOutcome(k Key, found bool, secret string) (ok bool, reason string)
```

`base` tidak meng-import package kontrak `verify` (kontrak hanya di-import oleh feature `verifykey`). Kesamaan string reason diuji di `verifykey`.

### 4.4 `base/dto.go`

```go
const (
    StatusActive  = "active"
    StatusRevoked = "revoked"
)

type KeyDTO struct {
    ID             string     `json:"id"`
    Name           string     `json:"name"`
    Prefix         string     `json:"prefix"`
    Status         string     `json:"status"`           // active | revoked
    CreatedBy      *string    `json:"created_by"`
    CreatedByLabel *string    `json:"created_by_label"`
    CreatedAt      time.Time  `json:"created_at"`
    LastUsedAt     *time.Time `json:"last_used_at"`
    RevokedAt      *time.Time `json:"revoked_at"`
    RevokedBy      *string    `json:"revoked_by"`
    RevokedByLabel *string    `json:"revoked_by_label"`
}

func (k Key) ToDTO() KeyDTO // semua waktu .UTC(); TIDAK ada field hash/secret

type CreateKeyResult struct {
    Key    KeyDTO  `json:"key"`
    RawKey string  `json:"raw_key"`
    MCPURL *string `json:"mcp_url"` // nil -> null
}

type KeyList struct{ Items []KeyDTO `json:"items"` } // selalu [] (bukan null) bila kosong
type KeyEnvelope struct{ Key KeyDTO `json:"key"` }  // respons revokekey

// MCPURL: publicURL "" -> nil; selain itu publicURL + "/mcp" (publicURL sudah tanpa trailing slash dari config).
func MCPURL(publicURL string) *string
```

### 4.5 `base/errors.go`

```go
const (
    ReasonNameTaken       = "NAME_TAKEN"
    ReasonKeyLimitReached = "KEY_LIMIT_REACHED"
    ReasonAlreadyRevoked  = "ALREADY_REVOKED"
    ReasonKeyNotFound     = "KEY_NOT_FOUND"
)

var (
    ErrProjectNotFound = errs.New(errs.CodeNotFound, "project tidak ditemukan")
    ErrKeyNotFound     = errs.WithDetails(errs.CodeNotFound, "API key tidak ditemukan",
        map[string]any{"reason": ReasonKeyNotFound})
    ErrNameTaken       = errs.WithDetails(errs.CodeConflict, "nama key sudah dipakai key aktif lain di project ini",
        map[string]any{"reason": ReasonNameTaken})
    ErrAlreadyRevoked  = errs.WithDetails(errs.CodeConflict, "API key sudah dicabut",
        map[string]any{"reason": ReasonAlreadyRevoked})

    // ErrPrefixCollision sentinel internal (bukan *errs.Error): repository -> usecase createkey untuk retry (K13).
    // Tidak pernah sampai ke klien; setelah 3 kali, usecase membungkusnya sebagai error infra (500).
    ErrPrefixCollision = errors.New("apikey: prefix collision")
)

// ErrKeyLimitReached -> 409, pesan fmt.Sprintf("batas %d key aktif per project tercapai; cabut key yang tidak dipakai", limit),
// details {"reason": "KEY_LIMIT_REACHED", "limit": limit}. Instance baru tiap panggilan.
func ErrKeyLimitReached(limit int) *errs.Error

type FieldErrors map[string]string // implements error; Error() menggabungkan key terurut
func (fe FieldErrors) Merge(other FieldErrors) FieldErrors
// InvalidFields -> errs.WithDetails(CodeInvalid, "permintaan tidak valid", map[string]any{"fields": fe})
func InvalidFields(fe FieldErrors) *errs.Error
```

Sentinel `*errs.Error` dipakai bersama (pointer yang sama); usecase TIDAK boleh memodifikasinya. Test memakai `errors.Is` atau memeriksa `Code` + `details.reason`.

### 4.6 `base/actor.go`, `base/meta.go`, `base/clock.go`

```go
type Actor struct{ ID, Role, Label string } // Label = email aktor (K2)
func ActorFrom(id reqctx.Identity, ok bool) (Actor, bool) // ok=false atau UserID kosong -> false

type RequestMeta struct{ IP, UserAgent string }
func NewRequestMeta(ip, ua string) RequestMeta // UA dipotong maks 512 byte tanpa memotong rune

type Clock func() time.Time // produksi: func() time.Time { return time.Now().UTC() }
```

---

## 5. Kontrak bus dan konstanta endpoint

### 5.1 `backend/lib/constants/bus.go` (ditambah)

```go
// Provider: T04 project/apikey. Consumer: T07 discovery/mcp-gateway.
// Kontrak: lib/contracts/project/apikey/verify (Call, fail-closed di consumer)
// dan lib/contracts/project/apikey/touch (Publish, fire-and-forget).
const BUS_PROJECT_APIKEY_VERIFY = "project.apikey.verify"
const BUS_PROJECT_APIKEY_TOUCH  = "project.apikey.touch"
```

`BUS_PROJECT_APIKEY_COUNT_KEYS = "project.apikey.count-keys"` milik kontrak T03 (plan T03 5.1). Tambahkan HANYA bila belum ada (bersama `BUS_PROJECT_MANAGEMENT_CHECK_ACCESS`, `BUS_PROJECT_MANAGEMENT_GET_PROJECT`, dst. persis plan T03). `BUS_CORE_AUDIT_RECORD` sudah ada di `main`.

### 5.2 `backend/lib/contracts/project/apikey/verify/verify.go` (baru, stdlib saja)

```go
// Package verify adalah kontrak bus project.apikey.verify (provider T04 project/apikey, consumer T07).
//
// Request dikirim sebagai nilai verify.Request (bukan pointer). Tipe lain -> error membungkus bus.ErrBadRequestType.
//
// Semantik:
//   - Key valid dan aktif           -> Found=true,  KeyID/KeyName/Prefix/ProjectID terisi, Reason "".
//   - Format salah                  -> Found=false, Reason=ReasonMalformed, Prefix "" (tanpa query DB).
//   - Prefix tidak dikenal          -> Found=false, Reason=ReasonUnknown,   Prefix terisi.
//   - Prefix dikenal, secret salah  -> Found=false, Reason=ReasonMismatch,  Prefix terisi.
//   - Key dicabut (secret benar)    -> Found=false, Reason=ReasonRevoked,   Prefix terisi.
//   - Error DB                      -> err != nil. Consumer WAJIB menolak (fail-closed).
//
// Reason dan Prefix HANYA untuk audit internal (mcp.auth.rejected). DILARANG meneruskan Reason ke agent/klien:
// respons ke luar harus seragam untuk semua Found=false.
// Tanpa cache: key yang dicabut langsung Found=false pada panggilan berikutnya.
// Provider tidak mengaudit dan tidak me-touch; itu tugas consumer (T07).
//
// Pola consumer (T07):
//
//	res, err := bus.CallTyped[verify.Response](ctx, b, constants.BUS_PROJECT_APIKEY_VERIFY, verify.Request{RawKey: raw})
//	if err != nil { /* tolak 401/503, log error; fail-closed */ }
//	if !res.Found { /* tolak seragam; audit mcp.auth.rejected dengan res.Prefix + res.Reason */ }
package verify

const (
    ReasonMalformed = "malformed"
    ReasonUnknown   = "unknown"
    ReasonRevoked   = "revoked"
    ReasonMismatch  = "mismatch"
)

type Request struct {
    RawKey string // "gr_<prefix>_<secret>"; spasi di tepi diabaikan provider
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

### 5.3 `backend/lib/contracts/project/apikey/touch/touch.go` (baru, stdlib saja)

```go
// Package touch adalah kontrak bus project.apikey.touch (Publish; provider T04, consumer T07).
//
// Dipanggil consumer setelah verifikasi sukses: d.Bus.Publish(ctx, constants.BUS_PROJECT_APIKEY_TOUCH, touch.Request{KeyID: id}).
// Provider memperbarui last_used_at paling sering sekali per menit per key (ditegakkan di SQL, aman multi-instance).
// KeyID bukan uuid atau tidak ada -> no-op (log debug). Error DB -> log warn, tidak dikembalikan.
// Tipe request salah -> error membungkus bus.ErrBadRequestType (bus mencatatnya sebagai "bus publish failed").
// Tidak ada respons.
package touch

type Request struct {
    KeyID string
}
```

### 5.4 Provider `project.apikey.count-keys` (kontrak T03 `contentcount`)

Perilaku wajib (mengikuti doc comment `contentcount` T03 5.4):

| Kondisi | Hasil |
|---|---|
| `req` bukan `contentcount.Request` | error membungkus `bus.ErrBadRequestType` |
| `len(ProjectIDs) > contentcount.MaxProjectIDs` | `errs.Wrap(errs.CodeInvalid, "maksimal 100 project id", errs.ErrInvalid)` |
| id bukan uuid | diabaikan |
| setelah filter kosong | `Response{Counts: map[string]int{}}`, tanpa query |
| normal | satu query `SELECT project_id, count(*) FROM api_keys WHERE project_id = ANY($1::uuid[]) AND revoked_at IS NULL GROUP BY project_id`; project tanpa key aktif boleh absen dari `Counts` |
| error DB | error terbungkus (consumer T03 fail-soft) |

Id duplikat didedupe sebelum query. Kunci map memakai string uuid kanonik dari DB (`project_id::text`); input uuid huruf besar dinormalisasi `uuid.Parse(id).String()` agar consumer menemukan entrinya (consumer T03 selalu mengirim bentuk kanonik).

### 5.5 Kontrak yang dikonsumsi

- `checkaccess` (T03 5.2) lewat `base/access` (3.4).
- `getproject` (T03 5.3) lewat `base/audit`: `Found=false` atau error -> `ProjectLabel = ""` + log warn `"apikey: get-project untuk label audit gagal"` (error) / tanpa log (Found=false).
- `record` (T02, di `main`): `Request{ActorType, ActorID, ActorLabel, ProjectID, ProjectLabel, Action, TargetType, Target, Args, Status, IP, UserAgent, TraceID}` -> `Response{Recorded}`.

### 5.6 Subpackage I/O bersama

`base/store/store.go` (package `store`):

```go
// KeyColumns dipakai semua SELECT/RETURNING yang men-scan base.Key.
const KeyColumns = "id, project_id, name, prefix, secret_hash, created_by, created_by_label, created_at, " +
    "last_used_at, revoked_at, revoked_by, revoked_by_label"

const (
    ConstraintPrefixUnique = "api_keys_prefix_key"
    ConstraintActiveName   = "api_keys_project_active_name_key"
)

type Store struct{ q db.Querier }
func New(q db.Querier) *Store
func (s *Store) WithTx(tx *sqlx.Tx) *Store
func (s *Store) Querier() db.Querier // untuk Exec/QueryRow sederhana di repository feature

// GetKey menjalankan query (SQL dari repository feature) yang mengembalikan KeyColumns satu baris.
// sql.ErrNoRows -> (zero, false, nil).
func (s *Store) GetKey(ctx context.Context, query string, args ...any) (base.Key, bool, error)
// SelectKeys untuk banyak baris; hasil kosong -> slice kosong non-nil.
func (s *Store) SelectKeys(ctx context.Context, query string, args ...any) ([]base.Key, error)

func ValidUUID(s string) bool // uuid.Parse sukses; dipakai repository untuk return "tidak ketemu" tanpa query
```

Row struct privat `keyRow` ber-tag `db` + `toEntity()` (waktu di-`.UTC()`, `[]byte` di-copy). Entity `base` tanpa tag `db`.

`base/store/pgerr.go`:

```go
// Violation mengembalikan kode SQLSTATE dan nama constraint bila err (atau error yang dibungkusnya) adalah *pgconn.PgError.
func Violation(err error) (code, constraint string, ok bool)
const (
    CodeUniqueViolation = "23505"
    CodeFKViolation     = "23503"
)
```

`base/access/access.go` (package `access`):

```go
type Checker interface {
    Require(ctx context.Context, projectID string, actor base.Actor) error
}
type BusChecker struct{ b *bus.Registry }
func New(b *bus.Registry) *BusChecker
// Require: bus.CallTyped[checkaccess.Response](ctx, b, constants.BUS_PROJECT_MANAGEMENT_CHECK_ACCESS,
//   checkaccess.Request{ProjectID: projectID, UserID: actor.ID, UserRole: actor.Role}); perilaku tabel 3.4.
```

`base/audit/audit.go` (package `audit`):

```go
type Recorder interface{ Record(ctx context.Context, ev Event) }
type Event struct {
    Actor     base.Actor
    Action    string         // record.ActionAPIKeyCreated | record.ActionAPIKeyRevoked
    ProjectID string
    KeyID     string         // Target
    Args      map[string]any // {name, prefix}; DILARANG memuat raw key/secret/hash
    Meta      base.RequestMeta
}
type BusRecorder struct{ b *bus.Registry; log *slog.Logger }
func New(b *bus.Registry, log *slog.Logger) *BusRecorder
// Record:
//   1. label := "" ; g, err := bus.CallTyped[getproject.Response](ctx, b, BUS_PROJECT_MANAGEMENT_GET_PROJECT, getproject.Request{ProjectID})
//      err -> log.WarnContext(...); g.Found -> label = g.Name.
//   2. req := record.Request{ActorType: record.ActorUser, ActorID: ev.Actor.ID, ActorLabel: ev.Actor.Label,
//        ProjectID: ev.ProjectID, ProjectLabel: label, Action: ev.Action, TargetType: base.TargetType, Target: ev.KeyID,
//        Args: ev.Args, Status: record.StatusOK, IP: ev.Meta.IP, UserAgent: ev.Meta.UserAgent, TraceID: reqctx.TraceID(ctx)}
//   3. bus.CallTyped[record.Response](ctx, b, constants.BUS_CORE_AUDIT_RECORD, req)
//      err -> log.ErrorContext(ctx, "audit record gagal", "action", ev.Action, "project_id", ev.ProjectID, "key_id", ev.KeyID, "error", err)
//   Tidak mengembalikan error (fail-open, K5). Tidak me-log Args.
```

---

## 6. Migration

Satu file: `backend/migrations/project/<VERSION>_create_api_keys.sql`

`<VERSION>` bukan nilai tetap: di-generate saat implementasi dengan `python3 -c "import datetime;print(datetime.datetime.now().strftime('%Y%m%d%H%M%S%f')[:17])"` (milidetik asli, bukan `000`; lihat `.claude/rules/migrations.md`) (format `YYYYMMDDHHMMSSmmm`, 17 digit sesuai validasi runner). Karena dibuat saat implement, otomatis lebih besar dari migrasi T03 (`create_projects`). Sebelum commit, pastikan tidak ada versi kembar di `backend/migrations/**`; bila kembar, generate ulang.

```sql
-- T04 project/api-key. gen_random_uuid() bawaan Postgres >= 13.
-- Hanya prefix dan SHA-256(secret) yang disimpan; plaintext key tidak pernah masuk DB.

CREATE TABLE api_keys (
    id               uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id       uuid        NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name             text        NOT NULL,
    prefix           text        NOT NULL,
    secret_hash      bytea       NOT NULL,
    created_by       uuid        NULL,  -- tanpa FK ke users (rule 8.5); label snapshot di bawah
    created_by_label text        NULL,
    created_at       timestamptz NOT NULL DEFAULT now(),
    last_used_at     timestamptz NULL,
    revoked_at       timestamptz NULL,
    revoked_by       uuid        NULL,  -- tanpa FK (rule 8.5)
    revoked_by_label text        NULL,
    CONSTRAINT api_keys_name_len CHECK (char_length(name) BETWEEN 2 AND 60),
    CONSTRAINT api_keys_prefix_fmt CHECK (prefix ~ '^[a-z0-9]{8}$'),
    CONSTRAINT api_keys_secret_hash_len CHECK (octet_length(secret_hash) = 32),
    CONSTRAINT api_keys_revoked_consistent CHECK (revoked_at IS NOT NULL OR (revoked_by IS NULL AND revoked_by_label IS NULL))
);

CREATE UNIQUE INDEX api_keys_prefix_key ON api_keys (prefix);
CREATE UNIQUE INDEX api_keys_project_active_name_key ON api_keys (project_id, name) WHERE revoked_at IS NULL;
CREATE INDEX api_keys_project_idx ON api_keys (project_id, created_at DESC);
```

Catatan:
- Forward-only, tanpa `CONCURRENTLY` (runner T00 membungkus tiap file dalam transaksi).
- Satu-satunya FK lintas module: `project_id` ON DELETE CASCADE. Test `TestT03_AC6_AllProjectFKsCascade` (pg_constraint) tetap lulus.
- `api_keys_prefix_key` melayani `verify` (satu index lookup). `api_keys_project_active_name_key` menegakkan K11 dan juga dipakai `count(*) ... revoked_at IS NULL`. `api_keys_project_idx` melayani `listkeys` dan `countkeys`.
- CHECK `name_len` adalah jaring pengaman; pesan validasi berasal dari entity.

---

## 7. Plan (langkah berurutan)

```
F  foundation (migrasi, konstanta, kontrak verify/touch, config K8, base/ + base/store + base/access + base/audit + test,
   OpenAPI, kerangka apikey/base.go, dokumen)
 └── gelombang paralel (semua hanya bergantung pada F):
     U1 verifykey, U2 touchlastused, U3 countkeys, U4 createkey, U5 listkeys, U6 revokekey
W  wiring final: apikey/base.go (6 Init), project/base.go, test integrasi lintas feature, TestRoutesRegistered
```

### F — foundation module

1. **Prasyarat T03**: cek `main`. Bila `backend/migrations/project/20261022090000000_create_projects.sql`, `lib/contracts/project/management/{checkaccess,getproject,contentcount}` dan konstanta T03 belum ada, buat persis plan T03 (bagian 5.1-5.4 dan 6). Bila sudah ada, jangan sentuh.
2. **Migrasi** (generate `<VERSION>` dulu dengan `python3 -c "import datetime;print(datetime.datetime.now().strftime('%Y%m%d%H%M%S%f')[:17])"` (milidetik asli, bukan `000`; lihat `.claude/rules/migrations.md`)): buat `backend/migrations/project/<VERSION>_create_api_keys.sql` (bagian 6). Jalankan `go test -tags integration ./internal/infra/db/...` (TestDB menjalankan semua migrasi) untuk memastikan file valid.
3. **Konstanta**: tambah `BUS_PROJECT_APIKEY_VERIFY`, `BUS_PROJECT_APIKEY_TOUCH` di `backend/lib/constants/bus.go` (5.1).
4. **Kontrak** (stdlib saja, rule 4.5): `backend/lib/contracts/project/apikey/verify/verify.go` (5.2), `backend/lib/contracts/project/apikey/touch/touch.go` (5.3).
5. **Config (K8)**, `backend/internal/infra/config/config.go`:
   - field `APIKeyMaxActivePerProject int \`env:"API_KEY_MAX_ACTIVE_PER_PROJECT"\`` dan `PublicURL string \`env:"PUBLIC_URL"\``;
   - di `Load`: `c.APIKeyMaxActivePerProject, _ = l.intv("API_KEY_MAX_ACTIVE_PER_PROJECT", 20, 1)`;
   - `PUBLIC_URL` (opsional): `url.Parse` gagal, scheme bukan http/https, host kosong, `RawQuery != ""`, `Fragment != ""`, atau ada userinfo -> `l.fail("PUBLIC_URL", "harus URL absolut http(s) dengan host, tanpa query/fragment (nilai %q)", v)`; valid -> `strings.TrimRight(v, "/")`. Path boleh (mis. `https://tim.id/grimoire`);
   - `LogValue`: tambah `slog.String("public_url", c.PublicURL)`, `slog.Int("api_key_max_active_per_project", c.APIKeyMaxActivePerProject)`;
   - `docs/configuration.md`: dua baris tabel (nama, tipe, default, wajib, validasi, kegunaan);
   - `.env.example`: `# API_KEY_MAX_ACTIVE_PER_PROJECT=20` dan `# PUBLIC_URL=https://grimoire.tim.id`;
   - `config_test.go`: kasus tabel (lihat 10). `docs_test.go` dan `envexample_test.go` harus tetap lulus.
6. **base/** (package `base`): semua file bagian 4 beserta unit test.
7. **base/store**: bagian 5.6 + `store_integration_test.go` (tag `integration`, `db.TestDB(t)`).
8. **base/access**, **base/audit**: bagian 5.6 + test dengan bus nyata (`bus.NewRegistry(log)`) dan handler palsu.
9. **Kerangka wiring**:
   - `backend/internal/domain/project/internal/apikey/base.go`: `package apikey; func Register(d app.Deps) { /* <feature>.Init(d) ditambah tiap usecase */ }`;
   - `backend/internal/domain/project/base.go`: tambah `apikey.Register(d)` setelah `management.Register(d)` (bila T03 belum merge dan `project/base.go` masih no-op, isi `func Register(d app.Deps) { apikey.Register(d) }`; T03 menambah `management.Register(d)` di depannya saat merge).
10. **OpenAPI** (SEBELUM kode handler, rule 11.6): `api/modules/project/api-key.yml` (bagian 13); tambah `$ref` path dan schema ke `api/root.yml`; `make api-bundle`; commit `api/openapi.yml`; `make api-check` lulus.
11. **Dokumen**:
    - `docs/tickets/README.md`: katalog bus (baris `project.apikey.verify`/`touch` diberi catatan kontrak `lib/contracts/project/apikey/{verify,touch}`; baris `project.apikey.count-keys | Call | T04 | T03` bila belum ditambah T03; hapus T04 dari consumer `core.auth.get-user` (K2)); status T04 diperbarui oleh workflow.
    - `docs/tickets/T04-project-api-key.md`: K1 (baris `countkeys` di Features + "Disediakan"), K2 (hapus `core.auth.get-user` dari "Dikonsumsi", tambah `project.management.get-project` fail-soft untuk label audit), K4 (param `{projectID}`/`{keyID}`), K8 (env), K9 (daftar reason).

### U1-U6

Rincian di plan per usecase (bagian 15). Tiap usecase:
- membuat folder feature (`init.go`, `handler.go`, `params.go` bila HTTP ber-body, `usecase.go`, `repository.go`, test);
- menambah `<feature>.Init(d)` di `apikey/base.go`;
- memastikan path/schema-nya benar di `api-key.yml` (lengkap sejak F);
- menulis test AC yang menjadi tanggung jawabnya (bagian 10).

Pola `init.go` HTTP (K4):

```go
func Init(d app.Deps) {
    s := store.New(d.DB)
    uc := NewUsecase(newRepo(d.DB, s), access.New(d.Bus), audit.New(d.Bus, d.Log),
        base.Clock(func() time.Time { return time.Now().UTC() }), rand.Reader,
        d.Config.APIKeyMaxActivePerProject, d.Config.PublicURL, d.Log)
    d.API.With(middleware.RequireAuth()).Post("/projects/{projectID}/api-keys", newHandler(uc).ServeHTTP)
}
```

Pola `init.go` bus:

```go
func Init(d app.Deps) {
    uc := NewUsecase(newRepo(store.New(d.DB)))
    d.Bus.Register(constants.BUS_PROJECT_APIKEY_VERIFY, newHandler(uc).Handle)
}
```

### W — wiring final

1. Verifikasi `apikey/base.go` memanggil ke-6 `Init` dan `project/base.go` memanggil `apikey.Register(d)`.
2. `apikey_integration_test.go` (tag `integration`): `db.TestDB(t)`, bus nyata, `chi.NewRouter()` sebagai `d.API` + middleware uji yang menyuntikkan `reqctx.WithIdentity`. Handler palsu untuk `project.management.check-access` (peta `(projectID, userID) -> Level`, super_admin -> admin), `project.management.get-project`, dan `core.audit.record` (merekam request) didaftarkan di registry test baru. Fixture project: `INSERT INTO projects (name) VALUES ($1) RETURNING id` lewat SQL test (data fixture, bukan import module T03).
3. `TestRoutesRegistered`: `chi.Walk` memuat tepat 3 pasangan method+pola bagian 1; Call `verify`/`count-keys` dengan request valid dan Publish `touch` tidak mengembalikan `ErrNotRegistered`.
4. `go vet ./...`, `go test ./...`, `go test -tags integration ./...`, archtest `TestT00_AC4_RepoIsClean`, `make api-check` lulus.

---

## 8. Changes (per file/folder)

Lib, config, migrasi (F):
- `backend/lib/constants/bus.go` (ubah): `BUS_PROJECT_APIKEY_VERIFY`, `BUS_PROJECT_APIKEY_TOUCH` (+ konstanta T03 bila belum ada).
- `backend/lib/contracts/project/apikey/verify/verify.go` (baru).
- `backend/lib/contracts/project/apikey/touch/touch.go` (baru).
- `backend/lib/contracts/project/management/{checkaccess,getproject,contentcount}/*.go` (baru HANYA bila T03 belum merge; isi persis plan T03).
- `backend/migrations/project/<VERSION>_create_api_keys.sql` (baru).
- `backend/migrations/project/20261022090000000_create_projects.sql` (baru HANYA bila T03 belum merge).
- `backend/internal/infra/config/config.go` (ubah): 2 field + validasi + `LogValue`.
- `backend/internal/infra/config/config_test.go` (ubah): kasus K8.
- `docs/configuration.md`, `.env.example` (ubah).

Domain:
- `backend/internal/domain/project/base.go` (ubah): `apikey.Register(d)`.
- `backend/internal/domain/project/internal/apikey/base.go` (baru).
- `backend/internal/domain/project/internal/apikey/base/{rawkey,key,verify,dto,errors,actor,meta,clock}.go` + `*_test.go` (baru).
- `backend/internal/domain/project/internal/apikey/base/store/{store.go,pgerr.go,store_integration_test.go}` (baru).
- `backend/internal/domain/project/internal/apikey/base/access/{access.go,access_test.go}` (baru).
- `backend/internal/domain/project/internal/apikey/base/audit/{audit.go,audit_test.go}` (baru).
- `backend/internal/domain/project/internal/apikey/{createkey,listkeys,revokekey,verifykey,touchlastused,countkeys}/...` (baru, per plan usecase).
- `backend/internal/domain/project/internal/apikey/apikey_integration_test.go` (baru, W).

API dan dokumen:
- `api/modules/project/api-key.yml` (baru), `api/root.yml` (ubah), `api/openapi.yml` (generated).
- `docs/tickets/README.md`, `docs/tickets/T04-project-api-key.md` (ubah).

Frontend (rinci di `frontend-plan.md`): `frontend/src/domain/project/apikey/{listkeys,createkey,revokekey}/` (services Zod, stores, components, views, `index.ts`), tab "API Keys" dikomposisikan di workspace project `frontend/src/app/`, kunci i18n `project.apikey.*` termasuk `project.apikey.error.<REASON>`.

Tidak diubah: `internal/infra/**` selain `config`, `cmd/**`, `go.mod`, `lib/contracts/core/**` (aksi audit sudah ada).

---

## 9. Konsumsi per usecase (ringkas, acuan plan usecase)

| Usecase | access | repository feature | store | audit | lainnya |
|---|---|---|---|---|---|
| createkey | Require | `InTx`, `LockProjectKeys`, `CountActive`, `Insert` | `GetKey` (RETURNING), `Violation` | apikey.created | `GenerateRawKey(rand)`, `NewKey`, `CheckActiveLimit`, `MCPURL`, retry K13 |
| listkeys | Require | `ListByProject` (`ORDER BY (revoked_at IS NULL) DESC, created_at DESC, id`) | `SelectKeys` | - | `ToDTO` |
| revokekey | Require | `InTx`, `LockInProject` (FOR UPDATE), `MarkRevoked` | `GetKey`, `ValidUUID` | apikey.revoked | `Key.Revoke` |
| verifykey | - | `FindByPrefix` | `GetKey` | - | `ParseRawKey`, `VerifyOutcome` |
| touchlastused | - | `Touch(id, now) (rowsAffected int64, error)` | `Querier`, `ValidUUID` | - | Clock |
| countkeys | - | `CountActiveByProjects(ids)` | `Querier`, `ValidUUID` | - | - |

`listkeys` men-scan `KeyColumns` lengkap (termasuk `secret_hash`) lewat store bersama, tetapi `ToDTO` tidak pernah mengeluarkan hash; test JSON memastikan tidak ada `secret`/`hash` di respons. (Alternatif: kolom tanpa hash; tidak dipilih agar satu jalur scan.)

---

## 10. Test Scenario

Penamaan: `TestT04_ACn_<Deskripsi>` untuk kriteria penerimaan; lainnya `Test<Fungsi>_<Kasus>`.

### Unit `base/` (tanpa mock)

- `GenerateRawKey`: dengan `crypto/rand.Reader` -> `len(Full)==55`, `Full == "gr_"+Prefix+"_"+Secret`, `ParseRawKey(Full)` mengembalikan prefix/secret yang sama; reader deterministik (`bytes.NewReader`) -> hasil deterministik; reader berisi byte >= 252 di awal -> byte itu dilewati (rejection sampling); reader kosong/error -> error terbungkus; 1000 key berbeda semua (sanity).
- `ParseRawKey`: valid; spasi di tepi -> valid; panjang 54/56 -> false; scheme `GR_`/`gx_` -> false; prefix huruf besar atau `-` -> false; `s[11] != '_'` -> false; secret memuat `_` dan `-` -> valid; secret karakter `=`/`+`/`/` -> false; secret dengan bit sisa non-nol (non-kanonik) -> false (K12); input 257 byte -> false tanpa decode; string kosong -> false.
- `HashSecret`: 32 byte, deterministik; `DummyHash` != hash secret acak.
- `ValidateName`: `"a"` gagal; `"ab"` lulus; 60 rune multibyte lulus, 61 gagal; `"  claude  "` -> `"claude"`; `"a\tb"` gagal pesan kontrol; spasi saja gagal pesan panjang.
- `NewKey`: nama invalid -> 400 `fields.name`; `SecretHash == HashSecret(raw.Secret)`; struct tidak memuat secret (refleksi: tidak ada field string bernilai `raw.Secret` atau `raw.Full`); label kosong -> `CreatedByLabel nil`.
- `CheckActiveLimit`: (19,20) nil; (20,20) dan (25,20) -> 409 `KEY_LIMIT_REACHED` dengan `details.limit == 20`.
- `Key.Revoke`: aktif -> field revoked terisi UTC; sudah dicabut -> `ErrAlreadyRevoked` dan struct tidak berubah.
- `MatchSecret`/`VerifyOutcome`: tabel found/!found × secret benar/salah × aktif/dicabut -> (true,""), unknown, mismatch, revoked, mismatch-untuk-dicabut-dengan-secret-salah.
- `ToDTO`/`CreateKeyResult`/`KeyList`: snapshot JSON (`status`, `last_used_at: null`, `mcp_url: null`, `items: []` bukan `null`); marshal tidak memuat kunci `secret`, `secret_hash`, `hash`.
- `MCPURL`: `""` -> nil; `"https://g.tim.id"` -> `"https://g.tim.id/mcp"`.
- `ActorFrom`, `NewRequestMeta` (UA 600 byte multibyte -> <= 512 byte UTF-8 valid).
- `errors.go`: tiap sentinel memeriksa `Code` + `details.reason`; `ErrKeyLimitReached` instance baru tiap panggilan.

### Subpackage I/O

- `store` (integrasi, `db.TestDB`): `GetKey` baris ada/tidak ada; `SelectKeys` kosong -> slice non-nil; waktu UTC; `Violation` mengenali 23505 + nama constraint pada INSERT prefix kembar dan nama aktif kembar, 23503 pada project_id acak; CHECK constraint menolak prefix `ABCDEFGH`, hash 31 byte, nama 1 karakter; nama sama boleh setelah key lama dicabut (partial unique index); CASCADE: hapus project menghapus key-nya.
- `access` (bus nyata, handler palsu): member/owner/admin -> nil; `ProjectExists=false` -> `ErrProjectNotFound`; `Level=none` -> `ErrProjectNotFound`; level tak dikenal -> 404; handler error / tidak terdaftar -> error non-`*errs.Error` (500), `errors.Is(err, bus.ErrNotRegistered)` benar; request yang diterima handler = `checkaccess.Request{ProjectID, UserID: actor.ID, UserRole: actor.Role}`.
- `audit` (bus nyata): handler palsu menerima `record.Request` dengan `ActorType=user`, `ActorID`, `ActorLabel=email`, `ProjectID`, `ProjectLabel` dari get-project, `TargetType=api_key`, `Target=keyID`, `Args={name,prefix}`, `Status=ok`, `TraceID`; get-project error atau `Found=false` -> `ProjectLabel=""` dan audit tetap terkirim; audit handler error/tidak terdaftar -> log error, tanpa panic, tanpa error; output log tidak memuat nilai `Args`.
- `config`: default `APIKeyMaxActivePerProject=20`; `0` -> error "minimal 1"; `abc` -> error; `PUBLIC_URL` kosong -> `""`; `https://g.tim.id/` -> `https://g.tim.id`; `https://tim.id/grimoire/` -> `https://tim.id/grimoire`; `ftp://x`, `g.tim.id`, `https://g.tim.id?a=1`, `https://g.tim.id#x`, `https://u:p@g.tim.id` -> error.

### Integrasi lintas usecase (`apikey_integration_test.go`, tag `integration`)

Fixture identitas: A (developer, owner P1), B (developer, member P1, bukan owner), C (developer, bukan anggota), S (super_admin). Project P1, P2 dibuat lewat SQL fixture.

- `TestT04_AC1_MemberCreatesKey_NonMember404_SuperAdminOK`: B `POST` -> 201; C `POST` -> 404 `project tidak ditemukan`; S `POST` -> 201; projectID `bukan-uuid` -> 404.
- `TestT04_AC2_PlaintextOnlyInCreateResponse`: respons create memuat `raw_key` 55 karakter dan header `Cache-Control: no-store`; `GET` list tidak memuat `raw_key` maupun secret (cek JSON mentah tidak mengandung string secret); `SELECT row_to_json(k)::text FROM api_keys k` tidak mengandung raw key maupun secret; `secret_hash == sha256(secret)`.
- `TestT04_AC3_VerifyImmediatelyAfterCreateAndRevoke`: create -> `verify` `Found=true` dengan KeyID/KeyName/Prefix/ProjectID benar -> revoke -> `verify` berikutnya `Found=false`, `Reason=revoked`, `Prefix` terisi.
- `TestT04_AC4_DuplicateActiveNameRejected_ReusableAfterRevoke`: create "ci" -> 201; create "ci" lagi -> 409 `NAME_TAKEN`; revoke -> create "ci" -> 201; list memuat dua "ci" (satu revoked).
- `TestT04_AC5_ActiveLimitEnforced`: limit 3 (config uji): 3 create sukses, ke-4 -> 409 `KEY_LIMIT_REACHED` `limit=3`; revoke satu -> create sukses; dua create serentak saat sisa 1 slot (diulang 10 kali dengan project baru) -> tepat satu 201 dan satu 409, jumlah aktif == limit.
- `TestT04_AC6_CreatorRemovalDoesNotKillKey`: A membuat key; check-access palsu diubah agar A `none`; `verify` tetap `Found=true`; A sekarang mendapat 404 di list.
- `TestT04_AC7_TouchWritesOncePerMinute`: dengan clock uji, 5 touch dalam menit yang sama -> `last_used_at` hanya berubah sekali (rows affected 1 lalu 0); clock +61 detik -> berubah lagi; list menampilkan `last_used_at`.
- `TestT04_AC8_MismatchTimingComparableToUnknown`: dirinci di plan `verifykey` (median durasi jalur unknown vs mismatch, banyak iterasi, toleransi longgar; dilewati dengan `testing.Short()`), ditambah assert struktural: `VerifyOutcome` dipanggil di kedua jalur (unit).
- `TestT04_AC9_AuditCreatedAndRevoked`: tepat satu `apikey.created` dan satu `apikey.revoked` dengan `Args={name,prefix}`, `ProjectLabel` = nama project; tidak ada raw key/secret di request audit; create gagal (409/400/404) -> 0 audit; audit handler gagal -> respons tetap 201/200.
- `TestT04_Security_ProjectIsolation` (rule 13.6): key milik P2 tidak muncul di list P1; revoke `/projects/P1/api-keys/<key P2>/revoke` -> 404 `KEY_NOT_FOUND` dan key P2 tetap aktif; keyID `bukan-uuid` -> 404 `KEY_NOT_FOUND`; C memanggil 3 route terhadap P1 -> 404 seragam.
- `TestT04_Security_RevokedKeyRejected` (rule 13.6): sama dengan AC3 ditambah dua revoke serentak -> tepat satu 200, satu 409 `ALREADY_REVOKED`, tepat satu audit.
- `TestT04_Security_NoSecretInLogs`: logger buffer selama create/verify/revoke; output tidak mengandung raw key/secret; mengandung prefix.
- `TestT04_CountKeys_OnlyActive`: P1 dua aktif + satu dicabut, P2 satu aktif -> `Counts{P1:2, P2:1}`; id `bukan-uuid` diabaikan; 101 id -> `errors.Is(err, errs.ErrInvalid)`.
- `TestT04_CascadeOnProjectDelete`: `DELETE FROM projects WHERE id = P1` -> key P1 hilang, `verify` key itu `Found=false, Reason=unknown`.
- `TestRoutesRegistered`: bagian 7 W.3.

Test per usecase (unit usecase dengan repo/bus palsu, handler dengan `httptest`) dirinci di plan usecase masing-masing.

---

## 11. E2E Scenarios (Playwright terhadap binary; butuh T01, T03 merge dan frontend T04)

Positif:
1. Developer B (member, bukan owner) login -> buka project -> tab "API Keys" kosong -> "Buat key" dengan nama "claude-code-budi" -> layar hasil menampilkan key `gr_...` lengkap, tombol salin, peringatan "tidak akan ditampilkan lagi", dan potongan `claude mcp add --transport http claude-code-budi <URL>/mcp --header "Authorization: Bearer gr_..."` serta JSON generik. URL = `PUBLIC_URL` bila diset, selain itu origin browser.
2. Tutup dialog -> tabel memuat baris nama, prefix, dibuat oleh `b@tim.id`, terakhir dipakai "-", status aktif; key lengkap tidak tampil lagi di mana pun (reload halaman).
3. B mencabut key dengan konfirmasi -> baris tampil redup dengan status "dicabut", dicabut oleh dan kapan.
4. Buat key baru dengan nama yang sama dengan key yang sudah dicabut -> sukses.
5. Super Admin (bukan anggota) membuka project -> bisa membuat dan mencabut key; tautan "Audit Log" terfilter untuk key tampil hanya bagi Super Admin dan memuat `apikey.created`/`apikey.revoked`.

Negatif:
1. Developer C (bukan anggota) membuka `/projects/<P1>/api-keys` (URL langsung) -> halaman "tidak ditemukan"; API 404.
2. Buat key dengan nama 1 karakter -> pesan field `name` "nama key wajib 2-60 karakter".
3. Buat key dengan nama yang dipakai key aktif -> pesan i18n `project.apikey.error.NAME_TAKEN`.
4. Batas key aktif tercapai -> pesan i18n `project.apikey.error.KEY_LIMIT_REACHED` memuat angka batas.
5. Cabut key yang sudah dicabut di tab lain -> pesan `project.apikey.error.ALREADY_REVOKED`, tabel di-refresh.
6. `POST /api/projects/{id}/api-keys` tanpa header `X-Requested-With` -> 403 `CSRF_REQUIRED`.
7. Tanpa login `GET /api/projects/{id}/api-keys` -> 401.

---

## 12. Request/Response body

Semua error memakai format seragam T00:

```json
{ "error": { "code": "NOT_FOUND", "message": "project tidak ditemukan", "details": null }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

Header mutasi: `X-Requested-With: grimoire`, `Content-Type: application/json`.

### POST /api/projects/{projectID}/api-keys

Request:
```json
{ "name": "claude-code-budi" }
```

201 (header `Cache-Control: no-store`):
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

`mcp_url` bernilai `null` bila `PUBLIC_URL` tidak diset (FE memakai `window.location.origin + "/mcp"`).

400 (validasi nama):
```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "permintaan tidak valid", "details": { "fields": { "name": "nama key wajib 2-60 karakter" } } }, "trace_id": "..." }
```

400 (field tak dikenal, mis. `{"nama": "x"}`): dari `respond.DecodeJSON`.

404 (bukan anggota, project tidak ada, atau projectID bukan uuid):
```json
{ "error": { "code": "NOT_FOUND", "message": "project tidak ditemukan", "details": null }, "trace_id": "..." }
```

409 (nama dipakai key aktif):
```json
{ "error": { "code": "CONFLICT", "message": "nama key sudah dipakai key aktif lain di project ini", "details": { "reason": "NAME_TAKEN" } }, "trace_id": "..." }
```

409 (batas key aktif):
```json
{ "error": { "code": "CONFLICT", "message": "batas 20 key aktif per project tercapai; cabut key yang tidak dipakai", "details": { "reason": "KEY_LIMIT_REACHED", "limit": 20 } }, "trace_id": "..." }
```

403 (tanpa header CSRF, dari middleware T00):
```json
{ "error": { "code": "CSRF_REQUIRED", "message": "...", "details": null }, "trace_id": "..." }
```

### GET /api/projects/{projectID}/api-keys

200 (aktif dulu, lalu `created_at DESC`, lalu `id`):
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
      "id": "0192f5a0-2222-7aaa-8bbb-000000000001",
      "name": "claude-code-budi",
      "prefix": "k3x9q2ma",
      "status": "revoked",
      "created_by": "0192f5a0-bbbb-7aaa-8bbb-00000000000b",
      "created_by_label": "b@tim.id",
      "created_at": "2026-10-23T09:00:00Z",
      "last_used_at": null,
      "revoked_at": "2026-10-23T12:00:00Z",
      "revoked_by": "0192f5a0-aaaa-7aaa-8bbb-00000000000a",
      "revoked_by_label": "a@tim.id"
    }
  ]
}
```

Project tanpa key -> `{ "items": [] }`. 404 seperti di atas.

### POST /api/projects/{projectID}/api-keys/{keyID}/revoke

Tanpa request body (body diabaikan; tidak di-decode).

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

404 (key tidak ada, keyID bukan uuid, atau milik project lain):
```json
{ "error": { "code": "NOT_FOUND", "message": "API key tidak ditemukan", "details": { "reason": "KEY_NOT_FOUND" } }, "trace_id": "..." }
```

409:
```json
{ "error": { "code": "CONFLICT", "message": "API key sudah dicabut", "details": { "reason": "ALREADY_REVOKED" } }, "trace_id": "..." }
```

### Bus (Go, bukan JSON di wire)

```go
res, err := bus.CallTyped[verify.Response](ctx, d.Bus, constants.BUS_PROJECT_APIKEY_VERIFY,
    verify.Request{RawKey: "gr_k3x9q2ma_Q2xhdWRlLWNvZGUtYnVkaS1zZWNyZXQtZXhhbXBsZS0xMjM0NTY"})
// valid:   verify.Response{Found: true, KeyID: "0192f5a0-2222-7aaa-8bbb-000000000001", KeyName: "claude-code-budi",
//                          Prefix: "k3x9q2ma", ProjectID: "0192f5a0-1111-7aaa-8bbb-000000000001"}
// dicabut: verify.Response{Found: false, Prefix: "k3x9q2ma", Reason: verify.ReasonRevoked}
// salah format ("abc"): verify.Response{Found: false, Reason: verify.ReasonMalformed}

err = d.Bus.Publish(ctx, constants.BUS_PROJECT_APIKEY_TOUCH, touch.Request{KeyID: "0192f5a0-2222-7aaa-8bbb-000000000001"})

c, err := bus.CallTyped[contentcount.Response](ctx, d.Bus, constants.BUS_PROJECT_APIKEY_COUNT_KEYS,
    contentcount.Request{ProjectIDs: []string{"0192f5a0-1111-7aaa-8bbb-000000000001", "bukan-uuid"}})
// c == contentcount.Response{Counts: map[string]int{"0192f5a0-1111-7aaa-8bbb-000000000001": 1}}
```

### Audit (`record.Request` yang dikirim, contoh `apikey.created`)

```go
record.Request{
    ActorType: record.ActorUser, ActorID: "0192f5a0-bbbb-7aaa-8bbb-00000000000b", ActorLabel: "b@tim.id",
    ProjectID: "0192f5a0-1111-7aaa-8bbb-000000000001", ProjectLabel: "Payments",
    Action: record.ActionAPIKeyCreated, TargetType: "api_key", Target: "0192f5a0-2222-7aaa-8bbb-000000000001",
    Args: map[string]any{"name": "claude-code-budi", "prefix": "k3x9q2ma"},
    Status: record.StatusOK, IP: "10.0.0.5", UserAgent: "Mozilla/5.0", TraceID: "01JB2X9K6Q4Y7V3N8M5T2R1C0A",
}
```

---

## 13. Skema OpenAPI (`api/modules/project/api-key.yml`, ringkas)

- `info: {title: Grimoire Project API Keys, version: 0.1.0}`.
- `components.parameters`: `XRequestedWith` (header, wajib di POST), `ProjectID` (path `projectID`, `format: uuid`), `KeyID` (path `keyID`, `format: uuid`), didefinisikan di file ini (pola `management.yml`).
- `components.schemas`:
  - `ApiKey`: semua field `KeyDTO` (4.4); `status` enum `[active, revoked]`; field nullable memakai `type: [string, "null"]`, waktu `format: date-time`; `additionalProperties: false`.
  - `CreateApiKeyRequest`: `{name: string, minLength 2, maxLength 60}` wajib, `additionalProperties: false`.
  - `CreateApiKeyResponse`: `{key: ApiKey, raw_key: string (pattern ^gr_[a-z0-9]{8}_[A-Za-z0-9_-]{43}$), mcp_url: [string, "null"] format uri}`; deskripsi: "raw_key hanya dikembalikan sekali; simpan sekarang".
  - `ApiKeyList`: `{items: ApiKey[]}`.
  - `ApiKeyEnvelope`: `{key: ApiKey}`.
- Operasi:
  - `POST /api/projects/{projectID}/api-keys`: `operationId: createkey`, `tags: [project-api-keys]`, 201 `CreateApiKeyResponse` dengan header respons `Cache-Control: no-store`; 400, 401, 403 (CSRF), 404, 409 (`NAME_TAKEN`, `KEY_LIMIT_REACHED`).
  - `GET /api/projects/{projectID}/api-keys`: `operationId: listkeys`, 200 `ApiKeyList`; 401, 404.
  - `POST /api/projects/{projectID}/api-keys/{keyID}/revoke`: `operationId: revokekey`, 200 `ApiKeyEnvelope`; 401, 403 (CSRF), 404 (`project tidak ditemukan` atau `KEY_NOT_FOUND`), 409 (`ALREADY_REVOKED`).
  - Respons error `$ref: ../infra/foundation.yml#/components/responses/{BadRequest,Unauthorized,Forbidden,NotFound,Conflict,Internal}`; deskripsi 404 menyebut "juga untuk project yang bukan milik pemanggil dan key milik project lain (tidak bocor)".
- `api/root.yml`: 2 path (`/api/projects/{projectID}/api-keys`, `/api/projects/{projectID}/api-keys/{keyID}/revoke`) dan schema `ApiKey`, `CreateApiKeyRequest`, `CreateApiKeyResponse`, `ApiKeyList`, `ApiKeyEnvelope`.

---

## 14. Risiko dan catatan

- T04 bergantung pada T03 yang belum merge. Bila T04 dikerjakan lebih dulu, foundation membuat kontrak dan migrasi `projects` persis plan T03; bentuk yang berbeda saat T03 merge harus disesuaikan ke kode di `main` (terdampak: `base/access`, `base/audit`, `countkeys`).
- Rute `/api/projects/{projectID}/api-keys` dan route T03 `/api/projects/{projectID}` memakai nama param yang sama di posisi yang sama (K4/T03 D4); jangan `Mount`/`Route` pada `/projects`.
- Uji timing (AC8) rawan flaky di CI; plan `verifykey` wajib memakai toleransi longgar, banyak iterasi, median, dan `testing.Short()` skip. Jaminan utama adalah struktural: `VerifyOutcome` selalu menghitung hash+compare, dan query DB sama untuk jalur unknown/mismatch.
- `last_used_at` bisa tertinggal hingga 1 menit (desain). Touch untuk key yang sudah dicabut tetap menulis (tidak berbahaya).
- Kedaluwarsa key (pertanyaan terbuka tiket) TIDAK dikerjakan; menambahkannya nanti = kolom `expires_at` + satu cek di `VerifyOutcome`.
- Tidak ada rate limit khusus di endpoint create; batas key aktif + CSRF + session sudah membatasi penyalahgunaan. Rate limit verifikasi adalah tanggung jawab T07.

---

## 15. Daftar usecase (plan rinci ditulis terpisah di `docs/superpowers/plans/project-api-key/usecase-<slug>.md`)

| # | Slug | Titik masuk | Ringkasan | AC |
|---|---|---|---|---|
| U1 | `verifykey` | bus `project.apikey.verify` (Call) | `Expect[verify.Request]` -> `ParseRawKey` (gagal -> `Found=false, malformed`, tanpa query) -> satu query `WHERE prefix=$1` (tanpa join, tanpa cek keanggotaan, tanpa cache) -> `VerifyOutcome` (hash+compare selalu, `DummyHash` bila tak dikenal) -> `Found=true` + KeyID/KeyName/Prefix/ProjectID, atau `Found=false` + Reason + Prefix. Error DB -> error (fail-closed). Tanpa audit/touch. Test timing statistik | AC3, AC6, AC8 |
| U2 | `touchlastused` | bus `project.apikey.touch` (Publish) | `Expect[touch.Request]`; KeyID bukan uuid -> no-op (debug); satu UPDATE throttle K7 dengan clock diinjeksi; key dicabut tetap boleh; error DB -> log warn, return nil | AC7 |
| U3 | `countkeys` | bus `project.apikey.count-keys` (Call) | `Expect[contentcount.Request]`; > 100 id -> `ErrInvalid`; id bukan uuid diabaikan; kosong -> tanpa query; satu query GROUP BY hanya key aktif | - (T03 D1) |
| U4 | `createkey` | `POST /api/projects/{projectID}/api-keys` | Aktor -> `access.Require` -> `ValidateName` -> `GenerateRawKey(crypto/rand)` -> `repo.InTx`: advisory lock, count aktif, `CheckActiveLimit`, INSERT (23505 nama -> 409 `NAME_TAKEN`; 23505 prefix -> retry maks 3; 23503 -> 404) -> setelah commit audit `apikey.created`. 201 `{key, raw_key, mcp_url}` + `Cache-Control: no-store` | AC1, AC2, AC4, AC5, AC9 |
| U5 | `listkeys` | `GET /api/projects/{projectID}/api-keys` | `access.Require` -> SELECT per project urut aktif dulu, `created_at DESC`, `id` -> 200 `{items}` termasuk key dicabut. Tanpa paginasi; tanpa raw key/hash | AC1, AC2 |
| U6 | `revokekey` | `POST /api/projects/{projectID}/api-keys/{keyID}/revoke` | `access.Require` -> `repo.InTx`: SELECT FOR UPDATE `WHERE id AND project_id` (tak ada -> 404 `KEY_NOT_FOUND`) -> `Key.Revoke` (409 `ALREADY_REVOKED`) -> UPDATE -> setelah commit audit `apikey.revoked`. 200 `{key}`. Tidak ada hapus/aktifkan ulang | AC3, AC4, AC9 |

Frontend: `docs/superpowers/plans/project-api-key/frontend-plan.md` (ditulis terpisah): `frontend/src/domain/project/apikey/{listkeys,createkey,revokekey}/` dengan `services` (Zod, kontrak REST bagian 12) / `stores` / `components` / `views` / `index.ts`; tab "API Keys" dikomposisikan di workspace project (`app/`); dialog hasil dengan salin key, peringatan sekali tampil, potongan konfigurasi Claude Code dan JSON generik (URL dari `mcp_url` atau `window.location.origin + "/mcp"`); konfirmasi cabut; baris dicabut redup; tautan Audit Log terfilter hanya untuk Super Admin.
