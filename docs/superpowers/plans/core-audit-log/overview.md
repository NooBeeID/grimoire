# T02 core/audit-log — Foundation Plan (overview)

Tiket: `docs/tickets/T02-core-audit-log.md`. Konvensi: `docs/tickets/README.md` dan `rules.txt` (rules menang untuk soal arsitektur).
Plan per usecase (`record`, `listentries`, `getentry`, `purgeexpired`) ditulis terpisah; dokumen ini hanya memuat bagian bersama.

## Metadata

| Field | Nilai |
|---|---|
| Scope | fullstack (Go backend + Svelte frontend) |
| Domain | core |
| Module | audit-log (folder Go `auditlog`, URL/UI `audit-log`, rule 2.3) |
| Resource root | `backend/internal/domain/core/internal/auditlog` |
| Version | v1 (migrasi pertama module; API baru, tanpa versi di path, prefix `/api`) |

| Endpoint | Method | Version | Akses | Feature |
|---|---|---|---|---|
| bus `core.audit.record` | Call / Publish | v1 | internal (T01, T03-T07) | `record` |
| `/api/audit-log` | GET | v1 | super_admin | `listentries` |
| `/api/audit-log/{id}` | GET | v1 | super_admin | `getentry` |
| job `core.auditlog.purge-expired` | DailyAt 03:15 UTC | v1 | sistem | `purgeexpired` |

Tidak ada POST/PUT/PATCH/DELETE untuk entri audit (AC7).

## Konteks yang dimuat

- Tidak ada `CLAUDE.md` maupun `.claude/rules/`. Sumber: `rules.txt`, `docs/tickets/README.md`, `docs/tickets/T02-core-audit-log.md`, `docs/superpowers/specs/2026-10-20-grimoire-design.md` (bagian "Audit log"), `docs/superpowers/plans/foundation/overview.md`, kode foundation di `worktree/feat-foundation` (commit `bd1c7aa` + rename yang sedang di-stage).
- Keputusan final: **fail-open** untuk kegagalan penulisan audit (AC8, pertanyaan terbuka #1 di tiket dan README).

### Nama package infra (WAJIB dicek ulang setelah T00 di-merge)

Foundation sedang me-rename folder. Kondisi terakhir di worktree (staged) dan yang dipakai plan ini:

| Kebutuhan | Import path | Package | Simbol |
|---|---|---|---|
| Deps wiring | `grimoire/internal/infra/app` | `app` | `app.Deps{Ctx, Config, Log, DB, Bus, Jobs, API, MCP}` |
| Bus | `grimoire/internal/infra/bus` | `bus` | `Registry.Register`, `bus.CallTyped[T]`, `bus.Expect[T]`, `bus.ErrBadRequestType` |
| Respons HTTP | `grimoire/internal/infra/http/respond` | `respond` | `WriteJSON(w, status, v)`, `WriteError(w, r, err)` |
| Guard role | `grimoire/internal/infra/middleware` | `middleware` | `RequireRole(roles...)` (tanpa identitas 401, role lain 403) |
| Job | `grimoire/internal/infra/jobs` | `jobs` | `jobs.Job{Name, Every, DailyAt, Run}`, `Scheduler.Register` |
| DB | `grimoire/internal/infra/db` | `db` | `*db.DB` (embed `*sqlx.DB`), `db.Querier`, `db.TestDB(t)` |
| Error | `grimoire/lib/common/errs` | `errs` | `ErrInvalid`, `ErrNotFound`, `Wrap(code, msg, err)`, `CodeInvalid`, `CodeNotFound` |
| Konteks | `grimoire/lib/common/reqctx` | `reqctx` | `TraceID(ctx)` |
| Role | `grimoire/lib/contracts/core/auth/resolvesession` | `resolvesession` | `RoleSuperAdmin`, `RoleDeveloper` |

Catatan: `infra/app/deps.go` di WIP masih meng-import `infra/jobs-scheduler`; bila saat implementasi nama final berbeda (mis. masih `http-server`/`jobs-scheduler`, package `httpx`), pakai nama final di `main` dan jangan membuat alias sendiri. `Config.AuditRetentionDays` (env `AUDIT_RETENTION_DAYS`, default 90, min 1) sudah ada; **tidak menambah env baru**. `github.com/google/uuid v1.6.0` sudah di `go.mod` (dipakai untuk `uuid.NewV7` dan `uuid.Parse`).

## Summary Plan

Module audit-log menyediakan satu tabel append-only `audit_log` (tanpa FK, label snapshot), satu handler bus `core.audit.record` yang melayani Call dan Publish (satu registrasi), dua endpoint baca khusus super_admin dengan filter gabungan dan cursor keyset `(occurred_at, id)`, dan job retensi harian yang menghapus dalam batch. Semua rule bisnis (validasi entri, sanitasi argumen, validasi filter, encode/decode cursor, cutoff retensi) ada di `auditlog/base/` tanpa I/O sehingga dipakai bersama oleh keempat feature. Consumer menulis audit dengan pola fail-open yang didokumentasikan di kontrak. Foundation plan ini menyiapkan migrasi, konstanta bus, kontrak, entity, OpenAPI, i18n, dan pembaruan dokumen; empat usecase dikerjakan sesudahnya (tiga paralel, `listentries` terakhir karena komposisi `app/`), lalu wiring module dan tes arsitektur.

## Plan

Urutan wajib. Langkah F1-F8 = foundation (sebelum usecase). U1-U4 = usecase. W1-W2 = wiring.

### F1. Migrasi

Buat `backend/migrations/core/20261021090000000_create_audit_log.sql` (isi lengkap di bagian Migration). Sebelum commit, cek tidak ada versi 17 digit yang sama di `backend/migrations/**` (terutama migrasi T01); bila bentrok, geser milidetik/menit ke atas. Migrasi forward-only, dijalankan runner foundation dalam satu transaksi.

### F2. Konstanta bus

`backend/lib/constants/bus.go`, tambahkan:

```go
// Provider: T02 core/auditlog. Consumer: T01, T03, T04, T05, T06, T07.
// Mode: Call untuk mcp.* dan aksi keamanan (rule 6.3), Publish untuk lainnya.
// Satu handler melayani kedua mode. Kegagalan = fail-open (lihat lib/contracts/core/audit/record).
const BUS_CORE_AUDIT_RECORD = "core.audit.record"
```

Nama endpoint mengikuti katalog README (`core.audit.record`), bukan `core.auditlog.*`.

### F3. Kontrak bus `lib/contracts/core/audit/record/`

Package `record`, stdlib saja (rule 4.5, hanya `time`).

`record.go`:

```go
// Package record adalah kontrak bus core.audit.record (provider T02 core/auditlog).
//
// Pola consumer (FAIL-OPEN, diputuskan T02, AC8):
//
//	_, err := bus.CallTyped[record.Response](ctx, d.Bus, constants.BUS_CORE_AUDIT_RECORD, req)
//	if err != nil {
//		log.ErrorContext(ctx, "audit write failed", "action", req.Action, "err", err)
//	}
//	// lanjutkan melayani permintaan; jangan kembalikan error ke klien/agent.
//
// Untuk Publish: if err := d.Bus.Publish(ctx, constants.BUS_CORE_AUDIT_RECORD, req); err != nil { log sama }.
// Jangan pernah me-log req.Args/req.Result. Jangan kirim isi file: kirim metadata (path, bytes, count).
package record

type Request struct {
	ID           string    // opsional; kosong -> diisi server (uuid v7)
	OccurredAt   time.Time // opsional; nol -> waktu server
	ActorType    string    // ActorUser | ActorAPIKey | ActorAnonymous | ActorSystem
	ActorID      string    // uuid; wajib untuk user/api_key, wajib kosong untuk anonymous/system
	ActorLabel   string    // snapshot: email/nama user, atau "<nama key> (<prefix>)"; anonymous: prefix key bila ada
	ProjectID    string    // uuid atau kosong
	ProjectLabel string    // snapshot nama project
	Action       string    // lihat actions.go
	TargetType   string    // mis. "file", "api_spec", "user", "project", "api_key"
	Target       string    // mis. "docs/auth/login.md"
	Args         map[string]any // akan disanitasi server
	Result       map[string]any // ringkasan: bytes, count, error_code; disanitasi server
	Status       string    // StatusOK | StatusError | StatusDenied
	IP           string    // "1.2.3.4", "::1", atau "host:port"; invalid -> disimpan NULL
	UserAgent    string
	TraceID      string    // kosong -> reqctx.TraceID(ctx)
}

type Response struct{ Recorded bool }

const (
	ActorUser      = "user"
	ActorAPIKey    = "api_key"
	ActorAnonymous = "anonymous"
	ActorSystem    = "system"

	StatusOK     = "ok"
	StatusError  = "error"
	StatusDenied = "denied"
)
```

`actions.go`: satu konstanta per aksi di katalog README:

```go
const (
	ActionMCPListFiles         = "mcp.list_files"
	ActionMCPReadFile          = "mcp.read_file"
	ActionMCPSearchKnowledge   = "mcp.search_knowledge"
	ActionMCPListAPISpecs      = "mcp.list_api_specs"
	ActionMCPListAPIEndpoints  = "mcp.list_api_endpoints"
	ActionMCPGetAPIEndpoint    = "mcp.get_api_endpoint"
	ActionMCPAuthRejected      = "mcp.auth.rejected"
	ActionMCPRateLimited       = "mcp.rate_limited"
	ActionAuthLoginSuccess     = "auth.login.success"
	ActionAuthLoginFailed      = "auth.login.failed"
	ActionAuthLogout           = "auth.logout"
	ActionAuthPasswordChanged  = "auth.password.changed"
	ActionAuthPasswordReset    = "auth.password.reset"
	ActionUserCreated          = "user.created"
	ActionUserUpdated          = "user.updated"
	ActionUserDeactivated      = "user.deactivated"
	ActionUserActivated        = "user.activated"
	ActionProjectCreated       = "project.created"
	ActionProjectUpdated       = "project.updated"
	ActionProjectDeleted       = "project.deleted"
	ActionProjectMemberAdded   = "project.member.added"
	ActionProjectMemberRemoved = "project.member.removed"
	ActionProjectMemberRoleChanged = "project.member.role_changed"
	ActionAPIKeyCreated        = "apikey.created"
	ActionAPIKeyRevoked        = "apikey.revoked"
	ActionKnowledgeFileCreated = "knowledge.file.created"
	ActionKnowledgeFileUpdated = "knowledge.file.updated"
	ActionKnowledgeFileDeleted = "knowledge.file.deleted"
	ActionKnowledgeFolderDeleted   = "knowledge.folder.deleted"
	ActionKnowledgeUploadCompleted = "knowledge.upload.completed"
	ActionAPISpecUploaded      = "apispec.uploaded"
	ActionAPISpecUpdated       = "apispec.updated"
	ActionAPISpecDeleted       = "apispec.deleted"
)

// Actions mengembalikan salinan daftar katalog (dipakai tes dan sumber daftar UI).
func Actions() []string
```

Tiket lain yang menambah aksi baru menambah konstanta di sini dan di katalog README (provider tidak membatasi aksi ke katalog).

Perilaku provider (diimplementasi usecase `record`, dicatat di sini sebagai kontrak):
- `bus.Expect[record.Request](req)`; tipe salah -> error membungkus `bus.ErrBadRequestType` (rule 6.9). Request dikirim sebagai nilai `record.Request` (bukan pointer); dokumentasikan di doc comment.
- Validasi gagal -> error membungkus `errs.ErrInvalid` (pesan menyebut field, tanpa nilai).
- DB gagal / timeout 2 s -> `fmt.Errorf("audit record: %w", err)`, `Response{}`.
- Sukses -> `Response{Recorded: true}`.
- Provider tidak me-log di jalur Call (pemanggil yang log). Jalur Publish sudah di-log bus (`"bus publish failed"`, level error). Provider tidak panic di jalur request. Tidak mengonsumsi endpoint lain.
- Bus `Publish` men-detach cancel (`context.WithoutCancel`), sehingga timeout 2 s di usecase tetap berlaku untuk Publish.

### F4. Entity dan rule bersama `auditlog/base/` (package `base`, tanpa I/O)

File dan isi:

`base/entry.go`
- Re-export nilai enum dari kontrak: `ActorUser = record.ActorUser`, dst.; `StatusOK = record.StatusOK`, dst. (base boleh import contracts; contracts tidak boleh import base).
- `type Entry struct { ID string; OccurredAt time.Time; ActorType, ActorID, ActorLabel, ProjectID, ProjectLabel, Action, TargetType, Target string; Args, Result map[string]any; Status, IP, UserAgent, TraceID string }`. String kosong = NULL untuk kolom nullable (dikonversi di repository).
- `var actionRe = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$`)`; `func ValidAction(s string) bool`.
- `func NewEntry(in record.Request, now time.Time, newID func() string) (Entry, error)` dengan rule:
  1. `Action` wajib dan cocok `actionRe` (panjang maks 128 byte).
  2. `ActorType` salah satu dari 4 nilai. `user`/`api_key`: `ActorID` wajib uuid valid (`uuid.Parse`, disimpan bentuk kanonis lowercase), `ActorLabel` wajib tidak kosong setelah trim. `anonymous`/`system`: `ActorID` wajib kosong; `ActorLabel` kosong -> `"anonymous"`/`"system"`.
  3. `ProjectID` kosong atau uuid valid. `ProjectLabel` tanpa `ProjectID` diperbolehkan (mis. auth.rejected dengan prefix key tanpa project) tetap disimpan.
  4. `Status` salah satu dari 3 nilai.
  5. Pemotongan UTF-8-safe (`TruncateUTF8(s, n)`): ActorLabel 256, ProjectLabel 256, TargetType 64, Target 1024, UserAgent 512, TraceID 128 byte. Semua string di-trim spasi tepi.
  6. IP: `netip.ParseAddr`; bila gagal coba `netip.ParseAddrPort` lalu ambil `Addr()`; tetap gagal -> `""` (bukan error). Simpan `addr.Unmap().String()`.
  7. `OccurredAt.IsZero()` -> `now`; selalu `.UTC()`. `ID` kosong -> `newID()`; terisi -> wajib uuid valid.
  8. `Args = Sanitize(in.Args)`, `Result = Sanitize(in.Result)`; nil -> `map[string]any{}`.
  9. Error: `errs.Wrap(errs.CodeInvalid, "<field> tidak valid", errs.ErrInvalid)` sehingga `errors.Is(err, errs.ErrInvalid)` benar.
- `type Summary struct` (JSON snake_case): `id, occurred_at, actor_type, actor_id (null), actor_label, project_id (null), project_label (null), action, target_type (null), target (null), status`. `func (e Entry) ToSummary() Summary`.
- `type Detail struct`: Summary + `args, result, ip (null), user_agent (null), trace_id`. `func (e Entry) ToDetail() Detail`. Field nullable memakai `*string` agar JSON `null`.

`base/sanitize.go`
- Konstanta: `MaxStringBytes = 2048`, `MaxJSONBytes = 8192`, `MaxDepth = 5`, `MaxSummaryKeys = 50`, `Redacted = "[REDACTED]"`, `Omitted = "[OMITTED]"`, `TooDeep = "[TOO_DEEP]"`, `TruncatedSuffix = "…[truncated]"`.
- `func Sanitize(v map[string]any) map[string]any` rekursif pada `map[string]any` dan `[]any` (tipe lain: angka, bool, nil apa adanya; tipe Go lain di-roundtrip via `json.Marshal`/`Unmarshal` dulu agar struct/[]string tertangani):
  - Kunci dinormalisasi (lowercase, `-` -> `_`). Bila **mengandung** salah satu `password`, `token`, `authorization`, `api_key`, `apikey`, `secret` -> nilai `"[REDACTED]"` (menutup `access_token`, `client_secret`, `X-API-Key`). Kunci asli dipertahankan.
  - Kunci tepat `content` atau `body` -> `"[OMITTED]"`.
  - String > 2048 byte -> dipotong UTF-8-safe + `"…[truncated]"`.
  - Kedalaman > 5 -> `"[TOO_DEEP]"`.
  - Setelah itu `json.Marshal`; bila > 8192 byte -> `{"_truncated": true, "_original_bytes": N, "_keys": [kunci level atas terurut, maks 50]}`.
  - Tidak pernah mengubah map input (salin).
- `func TruncateUTF8(s string, max int) string` (tidak memotong di tengah rune).

`base/filter.go`
- `type Filter struct { ActorType, ActorID, ActorLabel, ProjectID string; Actions, Statuses []string; From, To time.Time; Target string; Cursor *Cursor; Limit int }`.
- `func (f *Filter) Validate() error`: ActorType kosong atau enum; ActorID/ProjectID kosong atau uuid; Actions maks 20, tiap nilai `ValidAction`; Statuses tiap nilai enum (dedupe); `From`, `To` boleh nol, bila keduanya terisi `From < To`; ActorLabel/Target maks 200 byte; `Limit == 0` -> 50, di luar 1..200 -> invalid. Error membungkus `errs.ErrInvalid` dengan `details` `{"field": "<nama>"}` lewat `errs.WithDetails` (+ `Err: errs.ErrInvalid`).
- Konstanta `DefaultLimit = 50`, `MaxLimit = 200`, `MaxActions = 20`, `MaxTextBytes = 200`.
- `func EscapeLike(s string) string` (escape `\`, `%`, `_`) untuk ILIKE `ESCAPE '\'`.

`base/cursor.go`
- `type Cursor struct { OccurredAt time.Time; ID string }`.
- `func EncodeCursor(c Cursor) string`: `base64.RawURLEncoding` dari `"<RFC3339Nano UTC>|<uuid>"`.
- `func DecodeCursor(s string) (Cursor, error)`: rusak/uuid invalid/waktu invalid -> `errs.ErrInvalid` (field `cursor`).
- `func (e Entry) Cursor() Cursor`.

`base/retention.go`
- `func PurgeCutoff(now time.Time, days int) time.Time { return now.UTC().Add(-time.Duration(days) * 24 * time.Hour) }`.

Unit test di `base/*_test.go` (tanpa mock, rule 13.1) — lihat Test Scenario.

### F5. OpenAPI

Buat `api/modules/core/audit-log.yml` (paths `GET /api/audit-log`, `GET /api/audit-log/{id}`; schemas `AuditEntrySummary`, `AuditEntryDetail`, `AuditEntryList`; error memakai `components/responses` `BadRequest`, `Unauthorized`, `Forbidden`, `NotFound` dari `api/modules/infra/foundation.yml` dengan `$ref` relatif `../infra/foundation.yml#/components/...`, sama seperti cara bundler foundation me-resolve). Query param `action` dan `status` `style: form, explode: true` (berulang). Lalu `make api-bundle` dan pastikan `make api-check` lolos. Contoh payload ada di bagian Request/Response body.

### F6. i18n

Tambah kunci `auditLog.*` di `frontend/src/shared/i18n/id.json` dan `en.json` (file bersama; ditulis di foundation agar allowedPaths usecase tidak tumpang tindih):

`auditLog.menu`, `auditLog.title`, `auditLog.empty`, `auditLog.loadMore`, `auditLog.columns.{time,actor,project,action,target,status}`, `auditLog.actorType.{user,api_key,anonymous,system}`, `auditLog.status.{ok,error,denied}`, `auditLog.filter.{actorType,actor,project,action,status,range,target,apply,reset,any}`, `auditLog.range.{1h,24h,7d,30d,90d,all}`, `auditLog.detail.{title,args,result,ip,userAgent,traceId,close,notFound}`, `auditLog.error.load`.

### F7. Frontend types bersama

Tidak ada folder `base` di frontend. Daftar aksi untuk filter (salinan katalog) dan tipe enum ditaruh di `frontend/src/domain/core/audit-log/listentries/types.ts` oleh usecase `listentries`; `getentry/types.ts` memiliki tipe detailnya sendiri (tidak saling import, rule 12.4).

### F8. Dokumen

- `docs/tickets/README.md`: baris katalog `core.audit.record` -> tambah kolom/teks kontrak `lib/contracts/core/audit/record`, "satu handler untuk Call dan Publish"; tambah subbagian pendek "Pola fail-open penulisan audit" (snippet consumer seperti F3) dan "Aksi baru: tambah konstanta di `lib/contracts/core/audit/record/actions.go` + katalog ini".
- `rules.txt` 10.4 -> `Fail-open: bila audit gagal ditulis, error di-log level error dan panggilan agent tetap dilayani (diputuskan T02).` (rule 15.3). Catat alasan singkat di desain `docs/superpowers/specs/2026-10-20-grimoire-design.md` bagian "Audit log" (satu kalimat: risiko sebagian aksi tak tercatat diterima demi ketersediaan MCP).
- `docs/configuration.md`: bagian baru "Audit log": `AUDIT_RETENTION_DAYS`, jadwal purge 03:15 UTC batch 5000, trigger append-only, dan saran deploy `REVOKE UPDATE, TRUNCATE ON audit_log FROM <app_role>;` (DELETE tetap dibutuhkan job purge).

### U1-U4. Usecase (plan masing-masing ditulis agent lain)

| Urutan | Slug | Ringkas | allowedPaths |
|---|---|---|---|
| paralel | `record` | handler bus `core.audit.record` (Call & Publish, satu registrasi), INSERT tunggal | `backend/internal/domain/core/internal/auditlog/record/` |
| paralel | `getentry` | `GET /api/audit-log/{id}` + panel detail FE | `backend/.../auditlog/getentry/`, `frontend/src/domain/core/audit-log/getentry/` |
| paralel | `purgeexpired` | job harian retensi, DELETE batch | `backend/.../auditlog/purgeexpired/` |
| runLate | `listentries` | `GET /api/audit-log` filter + cursor, tabel/filter FE, komposisi `app/` (route, view, menu) | `backend/.../auditlog/listentries/`, `frontend/src/domain/core/audit-log/listentries/`, `frontend/src/app/routes/index.ts`, `frontend/src/app/views/AuditLogView.svelte`, `frontend/src/app/layouts/AppLayout.svelte` |

Signature init feature (dipanggil dari `auditlog/base.go`):

```go
func record.Init(d app.Deps)
func listentries.Init(d app.Deps, r chi.Router)   // r.Get("/", h)
func getentry.Init(d app.Deps, r chi.Router)      // r.Get("/{id}", h)
func purgeexpired.Init(d app.Deps)
```

Tiap `init.go` merakit repository konkret -> usecase (menerima interface, rule 5.6) -> handler.

### W1. Wiring module

`backend/internal/domain/core/internal/auditlog/base.go` (ditulis TERAKHIR agar build tidak putus):

```go
package auditlog

func Register(d app.Deps) {
	record.Init(d) // d.Bus.Register(constants.BUS_CORE_AUDIT_RECORD, h)
	d.API.Route("/audit-log", func(r chi.Router) {
		r.Use(middleware.RequireRole(resolvesession.RoleSuperAdmin))
		listentries.Init(d, r)
		getentry.Init(d, r)
	})
	purgeexpired.Init(d)
}
```

`backend/internal/domain/core/base.go`: tambah `auditlog.Register(d)` di `Register` (T01 juga menambah `auth.Register(d)`; konflik merge cukup diselesaikan dengan menyimpan kedua baris).

### W2. Tes arsitektur module (AC7)

`backend/internal/domain/core/internal/auditlog/arch_test.go` (package `auditlog`):
- Scan semua `*.go` (termasuk non-test kecuali file ini) di bawah `auditlog/`: regex `(?i)\bupdate\s+audit_log\b` tidak ada di mana pun; `(?i)\bdelete\s+from\s+audit_log\b` hanya di `purgeexpired/repository.go`; `(?i)\btruncate\b.*audit_log` tidak ada.
- Rakit router chi baru + `bus.NewRegistry` + scheduler palsu/nyata, panggil `Register`, lalu `chi.Walk`: semua rute di bawah `/audit-log` bermethod `GET` saja (dan HEAD bila chi menambahkannya); tepat 2 rute.
- Integrasi (build tag `integration`): `UPDATE audit_log SET status='ok'` -> error `audit_log is append-only`.

## Migration

File: `backend/migrations/core/20261021090000000_create_audit_log.sql`

```sql
CREATE TABLE audit_log (
    id            uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    occurred_at   timestamptz NOT NULL DEFAULT now(),
    actor_type    text        NOT NULL CHECK (actor_type IN ('user','api_key','anonymous','system')),
    actor_id      uuid        NULL,
    actor_label   text        NOT NULL,
    project_id    uuid        NULL,
    project_label text        NULL,
    action        text        NOT NULL,
    target_type   text        NULL,
    target        text        NULL,
    args          jsonb       NOT NULL DEFAULT '{}'::jsonb,
    status        text        NOT NULL CHECK (status IN ('ok','error','denied')),
    result        jsonb       NOT NULL DEFAULT '{}'::jsonb,
    ip            inet        NULL,
    user_agent    text        NULL,
    trace_id      text        NOT NULL DEFAULT ''
);

-- id sebagai tie-breaker keyset (occurred_at DESC, id DESC).
CREATE INDEX audit_log_occurred_idx ON audit_log (occurred_at DESC, id DESC);
CREATE INDEX audit_log_project_idx  ON audit_log (project_id, occurred_at DESC, id DESC);
CREATE INDEX audit_log_actor_idx    ON audit_log (actor_type, actor_id, occurred_at DESC, id DESC);
CREATE INDEX audit_log_action_idx   ON audit_log (action, occurred_at DESC, id DESC);

-- Append-only dari sisi DB: UPDATE selalu ditolak. DELETE diizinkan (hanya job purge).
CREATE FUNCTION audit_log_block_update() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'audit_log is append-only';
END;
$$;

CREATE TRIGGER audit_log_no_update
    BEFORE UPDATE ON audit_log
    FOR EACH ROW EXECUTE FUNCTION audit_log_block_update();
```

- Tanpa foreign key (rule 8.6). Tanpa `pg_trgm` (ILIKE selalu dikombinasikan dengan predikat waktu/indeks). Tanpa `CONCURRENTLY` (tabel baru, runner transaksional).
- `gen_random_uuid()` hanya default cadangan; aplikasi selalu mengirim uuid v7 (rule 8.4, terurut waktu).
- Cek bentrok versi dengan migrasi T01 sebelum merge.

## Changes

| File / folder | Aksi | Langkah |
|---|---|---|
| `backend/migrations/core/20261021090000000_create_audit_log.sql` | buat | F1 |
| `backend/lib/constants/bus.go` | ubah (tambah `BUS_CORE_AUDIT_RECORD`) | F2 |
| `backend/lib/contracts/core/audit/record/record.go` | buat | F3 |
| `backend/lib/contracts/core/audit/record/actions.go` | buat | F3 |
| `backend/lib/contracts/core/audit/record/actions_test.go` | buat (semua konstanta unik, cocok regex aksi) | F3 |
| `backend/internal/domain/core/internal/auditlog/base/entry.go` | buat | F4 |
| `backend/internal/domain/core/internal/auditlog/base/sanitize.go` | buat | F4 |
| `backend/internal/domain/core/internal/auditlog/base/filter.go` | buat | F4 |
| `backend/internal/domain/core/internal/auditlog/base/cursor.go` | buat | F4 |
| `backend/internal/domain/core/internal/auditlog/base/retention.go` | buat | F4 |
| `backend/internal/domain/core/internal/auditlog/base/*_test.go` | buat | F4 |
| `api/modules/core/audit-log.yml` | buat | F5 |
| `api/openapi.yml` | regenerate (`make api-bundle`) | F5 |
| `frontend/src/shared/i18n/id.json`, `en.json` | ubah (kunci `auditLog.*`) | F6 |
| `docs/tickets/README.md` | ubah (katalog bus + pola fail-open) | F8 |
| `rules.txt` | ubah 10.4 | F8 |
| `docs/superpowers/specs/2026-10-20-grimoire-design.md` | ubah (catatan fail-open) | F8 |
| `docs/configuration.md` | ubah (bagian "Audit log") | F8 |
| `backend/internal/domain/core/internal/auditlog/{record,listentries,getentry,purgeexpired}/` | buat (plan usecase) | U1-U4 |
| `frontend/src/domain/core/audit-log/{listentries,getentry}/` | buat (plan usecase) | U1-U4 |
| `frontend/src/app/routes/index.ts`, `app/views/AuditLogView.svelte`, `app/layouts/AppLayout.svelte` | ubah/buat (usecase listentries) | U4 |
| `backend/internal/domain/core/internal/auditlog/base.go` | buat | W1 |
| `backend/internal/domain/core/base.go` | ubah (`auditlog.Register(d)`) | W1 |
| `backend/internal/domain/core/internal/auditlog/arch_test.go` | buat | W2 |

Tidak ada env baru, tidak ada dependensi baru.

## Test Scenario

Penamaan `TestT02_ACn_*` untuk tes yang membuktikan kriteria (rule 13.4), selain itu `Test<Unit>_<Kasus>`. Integrasi: build tag `integration`, `db.TestDB(t)` (rule 13.3).

### Foundation (base/, unit tanpa mock)

`entry_test.go` (NewEntry, table-driven, clock & newID tetap):
- api_key lengkap valid -> Entry terisi, OccurredAt = now UTC, ID = newID().
- `OccurredAt` diberikan di zona +07 -> tersimpan `.UTC()`.
- ID diberikan uuid valid -> dipakai; ID `"abc"` -> ErrInvalid.
- Action kosong, `"MCP.read"`, `"read_file"` (tanpa titik), `"mcp..x"` -> ErrInvalid; `"project.member.role_changed"`, `"mcp.read_file"` valid; aksi di luar katalog `"foo.bar"` valid.
- ActorType `"admin"` -> ErrInvalid. `user` tanpa ActorID -> ErrInvalid; `user` dengan ActorLabel `"  "` -> ErrInvalid.
- `TestT02_AC2_AnonymousWithKeyPrefix`: anonymous, ActorID kosong, label `"gk_ab12"`, action `mcp.auth.rejected`, status denied -> valid. anonymous dengan ActorID terisi -> ErrInvalid. anonymous label kosong -> `"anonymous"`; system label kosong -> `"system"`.
- ProjectID `"x"` -> ErrInvalid; kosong valid.
- Status `"failed"` -> ErrInvalid.
- Pemotongan: Target 2000 byte multibyte (`"é"` berulang) -> <= 1024 byte dan `utf8.ValidString`.
- IP: `"203.0.113.5"`, `"::1"`, `"203.0.113.5:4431"`, `"[::1]:80"`, `"::ffff:10.0.0.1"` -> diparse (yang terakhir jadi `10.0.0.1`); `"not-an-ip"` -> `""`, tanpa error.
- Args nil -> `{}`. Pesan error tidak mengandung nilai input (cek `!strings.Contains(err.Error(), nilaiRahasia)`).
- `ToSummary` tidak punya field args/result/ip/user_agent (cek JSON marshal kunci); `ToDetail` punya semuanya; field kosong nullable -> `null`.

`sanitize_test.go`:
- `TestT02_AC4_PasswordRedacted`: `{"password":"x"}` -> `"[REDACTED]"`; varian `Password`, `access_token`, `client_secret`, `X-API-Key`, `apiKey`, `Authorization`, nested `{"auth":{"token":"t"}}`, dalam slice `[{"secret":"s"}]` -> semua `[REDACTED]`; JSON hasil tidak mengandung `"x"`, `"t"`, `"s"` sebagai nilai.
- `TestT02_AC4_ContentOmitted`: `{"content":"isi file rahasia","path":"a.md","bytes":12}` -> content `[OMITTED]`, path & bytes utuh; `body` juga; `contents` (bukan tepat) tidak diomit.
- String 3000 byte -> <= 2048 byte + suffix, UTF-8 valid.
- Kedalaman 7 -> level ke-6 `[TOO_DEEP]`.
- Total > 8 KB (mis. 10 kunci x 1500 byte) -> `{"_truncated":true,"_original_bytes":N,"_keys":[...]}` dengan N > 8192, kunci terurut, maks 50.
- Input tidak termutasi. Angka/bool/nil dipertahankan. Struct Go dan `[]string` diterima.

`filter_test.go`: default limit 50; limit 0 -> 50; 201 -> invalid; -1 -> invalid; 21 aksi -> invalid; aksi format salah -> invalid; status `"x"` -> invalid; From >= To -> invalid; From saja valid; actor_id/project_id bukan uuid -> invalid; teks 201 byte -> invalid; `EscapeLike("50%_a\\")` -> `50\%\_a\\\\`.

`cursor_test.go`: round-trip Encode/Decode menjaga nanodetik dan id; `"!!!"`, base64 tanpa `|`, waktu salah, uuid salah -> ErrInvalid.

`retention_test.go`: `PurgeCutoff(2026-10-20T03:15Z, 90)` = `2026-07-22T03:15Z`; input non-UTC dinormalisasi.

`lib/contracts/core/audit/record/actions_test.go`: semua nilai di `Actions()` unik, jumlah = 33, cocok regex aksi, cocok daftar katalog README (daftar literal di tes).

Migrasi (integrasi, di `record/repository_test.go` atau tes migrasi foundation): tabel ada, CHECK menolak `actor_type='x'` dan `status='x'`, trigger menolak UPDATE, DELETE diizinkan, 4 indeks ada (`pg_indexes`).

### Ringkasan pemetaan AC ke usecase (detail di plan usecase)

| AC | Bukti | Pemilik |
|---|---|---|
| AC1 | entri api_key lengkap (key label, project, path, bytes, ok, IP inet) tersimpan via bus Call | record (integrasi) |
| AC2 | anonymous + prefix, `mcp.auth.rejected`, denied | base (unit) + record (integrasi) |
| AC3 | project_id/actor_id tanpa baris di tabel lain tersimpan; dibaca kembali dengan label snapshot | record, getentry, listentries |
| AC4 | password tertutup, isi file tidak ada di kolom jsonb (query `args::text`) | base (unit) + record (integrasi) |
| AC5 | developer -> 403, anonim -> 401, super_admin -> 200 | listentries, getentry (HTTP) |
| AC6 | entri 91 hari hilang, 89 hari dan baru tetap, beberapa batch | purgeexpired (integrasi) |
| AC7 | grep SQL + chi.Walk hanya GET + trigger | wiring W2 |
| AC8 | sisi provider: DB gagal/timeout -> error dikembalikan, tanpa panic, Recorded false; bukti end-to-end MCP tetap sukses milik T07 | record (unit) |
| AC9 | EXPLAIN 1 juta baris: tanpa `Seq Scan on audit_log` | listentries (integrasi) |

## E2E Scenarios

Positif:
1. Super admin login -> menu "Audit Log" tampil -> `/audit-log` memuat tabel urut terbaru, ikon agent untuk `api_key`/`anonymous`, ikon manusia untuk `user`.
2. Filter project X + aksi `mcp.read_file` + preset 7 hari -> hanya entri cocok; URL query tersinkron (`?project_id=..&action=mcp.read_file&from=..`); reload mempertahankan filter.
3. "Muat berikutnya" memakai `next_cursor`; tidak ada duplikat/lompatan saat dua entri punya `occurred_at` sama; halaman terakhir -> tombol hilang (`next_cursor: null`).
4. Klik baris -> panel samping `?entry=<id>` menampilkan args/result JSON terformat, IP, user agent, trace id; `args` berisi `"<img onerror>"` tampil sebagai teks.
5. Deep link `/audit-log?actor_type=api_key&actor_id=<id>` -> terfilter ke key itu.
6. Project dan key dihapus (T03/T04) -> entri lama masih tampil dengan label snapshot (AC3).
7. Job purge berjalan -> entri > retensi hilang, entri baru tetap (AC6).

Negatif:
1. Developer membuka `/audit-log` (UI) -> halaman tidak ditemukan (guard meta.roles); `GET /api/audit-log` -> 403 `FORBIDDEN` (AC5).
2. Tanpa sesi -> `GET /api/audit-log` 401 `UNAUTHORIZED`; UI redirect ke login.
3. `limit=500`, `action=BAD`, `status=x`, `from` > `to`, `actor_id=abc`, `cursor=rusak` -> 400 `INVALID_ARGUMENT` dengan `details.field`.
4. `GET /api/audit-log/not-a-uuid` dan uuid tak ada -> 404 `NOT_FOUND`; panel menampilkan `auditLog.detail.notFound`.
5. `POST/PUT/PATCH/DELETE /api/audit-log[/id]` -> 405 `METHOD_NOT_ALLOWED` (tidak ada rute ubah/hapus, AC7).
6. Bus Call dengan tipe request salah (`*record.Request` atau struct lain) -> error `bus: unexpected request type`.
7. DB audit mati saat MCP dipanggil -> panggilan tetap sukses, log aplikasi berisi `audit write failed` level error (bukti end-to-end di T07, AC8).

## Request/Response body

### Bus `core.audit.record` (Go, Call)

```go
resp, err := bus.CallTyped[record.Response](ctx, d.Bus, constants.BUS_CORE_AUDIT_RECORD, record.Request{
	ActorType:    record.ActorAPIKey,
	ActorID:      "0192f0c4-7a1e-7c3a-9b1d-3c4e5f6a7b8c",
	ActorLabel:   "claude-code-budi (gk_ab12)",
	ProjectID:    "0192f0c4-0000-7000-8000-000000000001",
	ProjectLabel: "Payments",
	Action:       record.ActionMCPReadFile,
	TargetType:   "file",
	Target:       "docs/auth/login.md",
	Args:         map[string]any{"path": "docs/auth/login.md"},
	Result:       map[string]any{"bytes": 4812},
	Status:       record.StatusOK,
	IP:           "203.0.113.5",
	UserAgent:    "claude-code/2.1",
})
// sukses: resp == record.Response{Recorded: true}, err == nil
// gagal:  resp == record.Response{}, err != nil  -> consumer log "audit write failed" lalu lanjut (fail-open)
```

Error provider (nilai `err.Error()`): `actor_id tidak valid: invalid argument` (validasi, `errors.Is(err, errs.ErrInvalid)`), `audit record: context deadline exceeded` (timeout), `bus: unexpected request type ...` (tipe salah).

### `GET /api/audit-log?project_id=0192f0c4-0000-7000-8000-000000000001&action=mcp.read_file&action=mcp.search_knowledge&status=ok&from=2026-10-13T00:00:00Z&to=2026-10-20T00:00:00Z&limit=2`

200:

```json
{
  "items": [
    {
      "id": "0192f0c4-7b00-7d11-8a00-1a2b3c4d5e6f",
      "occurred_at": "2026-10-19T08:14:22.123456Z",
      "actor_type": "api_key",
      "actor_id": "0192f0c4-7a1e-7c3a-9b1d-3c4e5f6a7b8c",
      "actor_label": "claude-code-budi (gk_ab12)",
      "project_id": "0192f0c4-0000-7000-8000-000000000001",
      "project_label": "Payments",
      "action": "mcp.read_file",
      "target_type": "file",
      "target": "docs/auth/login.md",
      "status": "ok"
    },
    {
      "id": "0192f0c4-7aff-7c00-8b00-0f0e0d0c0b0a",
      "occurred_at": "2026-10-19T08:14:20.000000Z",
      "actor_type": "api_key",
      "actor_id": "0192f0c4-7a1e-7c3a-9b1d-3c4e5f6a7b8c",
      "actor_label": "claude-code-budi (gk_ab12)",
      "project_id": "0192f0c4-0000-7000-8000-000000000001",
      "project_label": "Payments",
      "action": "mcp.search_knowledge",
      "target_type": null,
      "target": null,
      "status": "ok"
    }
  ],
  "next_cursor": "MjAyNi0xMC0xOVQwODoxNDoyMFp8MDE5MmYwYzQtN2FmZi03YzAwLThiMDAtMGYwZTBkMGMwYjBh"
}
```

Halaman terakhir: `"next_cursor": null`. Tanpa hasil: `{"items": [], "next_cursor": null}`.

400 (filter invalid):

```json
{
  "error": { "code": "INVALID_ARGUMENT", "message": "limit tidak valid", "details": { "field": "limit" } },
  "trace_id": "4f1c2b7e9a0d4e6f"
}
```

401:

```json
{ "error": { "code": "UNAUTHORIZED", "message": "perlu login", "details": null }, "trace_id": "4f1c2b7e9a0d4e70" }
```

403 (developer):

```json
{ "error": { "code": "FORBIDDEN", "message": "akses ditolak", "details": null }, "trace_id": "4f1c2b7e9a0d4e71" }
```

### `GET /api/audit-log/0192f0c4-7b00-7d11-8a00-1a2b3c4d5e6f`

200:

```json
{
  "id": "0192f0c4-7b00-7d11-8a00-1a2b3c4d5e6f",
  "occurred_at": "2026-10-19T08:14:22.123456Z",
  "actor_type": "api_key",
  "actor_id": "0192f0c4-7a1e-7c3a-9b1d-3c4e5f6a7b8c",
  "actor_label": "claude-code-budi (gk_ab12)",
  "project_id": "0192f0c4-0000-7000-8000-000000000001",
  "project_label": "Payments",
  "action": "mcp.read_file",
  "target_type": "file",
  "target": "docs/auth/login.md",
  "status": "ok",
  "args": { "path": "docs/auth/login.md", "api_key": "[REDACTED]" },
  "result": { "bytes": 4812, "content": "[OMITTED]" },
  "ip": "203.0.113.5",
  "user_agent": "claude-code/2.1",
  "trace_id": "9c2e41aa07b34f15"
}
```

Contoh entri AC2 (detail): `"actor_type": "anonymous", "actor_id": null, "actor_label": "gk_zz99", "project_id": null, "project_label": null, "action": "mcp.auth.rejected", "status": "denied", "args": {}, "result": {"reason": "invalid_key"}`.

404 (id bukan uuid atau tidak ada):

```json
{ "error": { "code": "NOT_FOUND", "message": "resource tidak ditemukan", "details": null }, "trace_id": "4f1c2b7e9a0d4e72" }
```

405 (mis. `DELETE /api/audit-log/{id}`):

```json
{ "error": { "code": "METHOD_NOT_ALLOWED", "message": "method tidak diizinkan", "details": null }, "trace_id": "4f1c2b7e9a0d4e73" }
```

(Pesan 405 mengikuti teks foundation; kodenya yang stabil.)

## Keputusan lintas usecase

- PK uuid v7 dari aplikasi (rule 8.4); cursor keyset `(occurred_at, id)` (rule 11.5); urutan selalu `occurred_at DESC, id DESC`.
- `record`: satu INSERT, tanpa join, tanpa transaksi multi-statement, timeout 2 s.
- Repository per feature hanya berisi SQL feature itu: record -> INSERT; listentries -> SELECT summary dengan builder predikat tetap + argumen `$n` (tanpa concat input, rule 5.3), `action = ANY($n)`, `status = ANY($n)`, `ILIKE $n ESCAPE '\'`; getentry -> SELECT by id (`ip::text`); purgeexpired -> satu-satunya `DELETE` (`DELETE FROM audit_log WHERE id IN (SELECT id FROM audit_log WHERE occurred_at < $1 ORDER BY occurred_at LIMIT $2)`). Tidak ada UPDATE di mana pun.
- Purge: `Name: "core.auditlog.purge-expired"`, `DailyAt: "03:15"`, batch 5000, jeda 100 ms, cutoff dihitung sekali per run, berhenti bila affected < batch atau ctx selesai, log info `total_deleted`, `batches`, `duration_ms`; error dikembalikan ke scheduler; tidak menulis entri audit.
- Melihat audit log tidak menghasilkan entri audit (tidak ada di katalog).
- Autocomplete aktor/project tanpa endpoint baru (rule 1.5): filter `actor_id` persis (indeks, deep link dari halaman API key) atau `actor_label` contains; saran autocomplete UI dari data halaman yang sudah dimuat + pilihan terakhir. Endpoint saran = follow-up opsional, bukan scope T02.

## Daftar usecase

| Slug | Deskripsi singkat |
|---|---|
| `record` | Handler bus `core.audit.record` (Call & Publish, satu registrasi): Expect -> TraceID dari ctx -> `base.NewEntry` -> INSERT (timeout 2 s); fail-open sisi provider (error dikembalikan, tanpa log/panic). |
| `listentries` | `GET /api/audit-log`: parse query -> `base.Filter.Validate` -> SELECT keyset limit+1 -> `{items, next_cursor}`; FE tabel + filter + cursor; komposisi `app/` (route, view, menu). runLate. |
| `getentry` | `GET /api/audit-log/{id}`: id bukan uuid/tidak ada -> 404; detail penuh; FE panel detail diekspor lewat `index.ts`. |
| `purgeexpired` | Job harian 03:15 UTC: DELETE batch 5000 entri lebih tua dari `AUDIT_RETENTION_DAYS`. |
