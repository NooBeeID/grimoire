# T02 core/audit-log — Usecase `getentry` (GET /api/audit-log/{id})

Dokumen ini melengkapi `docs/superpowers/plans/core-audit-log/overview.md` (foundation F1-F8 sudah ada: migrasi, `base.Entry`/`ToDetail`, OpenAPI, i18n). Hanya satu usecase: `getentry`. Konvensi: `docs/tickets/README.md` dan `rules.txt`. Keputusan fail-open (AC8) tidak menyentuh usecase ini (hanya jalur tulis).

## Metadata

| Field | Nilai |
|---|---|
| Scope | fullstack (Go backend + Svelte frontend) |
| Domain | core |
| Module | audit-log (folder Go `auditlog`, UI `audit-log`) |
| Feature | `getentry` |
| Endpoint | `/api/audit-log/{id}` |
| Method | GET (hanya GET; tidak ada ubah/hapus, AC7) |
| Version | v1 (tanpa versi di path, prefix `/api`) |
| Akses | super_admin saja (guard `RequireRole` dipasang di `auditlog/base.go` W1, bukan di feature ini) |
| Bus | tidak menyediakan dan tidak mengonsumsi endpoint bus |
| Resource root | `backend/internal/domain/core/internal/auditlog` |
| allowedPaths | `backend/internal/domain/core/internal/auditlog/getentry/`, `frontend/src/domain/core/audit-log/getentry/` |

Prasyarat dari foundation (tidak dikerjakan di sini): tabel `audit_log` (F1), `base.Entry` + `Entry.ToDetail()` + struct `base.Detail` (F4), kunci i18n `auditLog.detail.*` dan `auditLog.error.load` (F6), skema OpenAPI `AuditEntryDetail` (F5). Bila salah satunya belum ada saat implementasi, hentikan dan laporkan, jangan menambah di luar allowedPaths.

## Summary Plan

Super admin membuka satu entri audit untuk melihat `args` dan `result` penuh (sudah disanitasi saat tulis), IP, user agent, dan trace id. Backend: handler membaca `{id}` dari chi, id yang bukan uuid kanonis diperlakukan sama dengan tidak ada (404 `NOT_FOUND`, tidak membocorkan format), usecase memanggil `repository.Get` lalu mengonversi "tidak ketemu" menjadi `errs.ErrNotFound`, handler menulis `base.Entry.ToDetail()` sebagai JSON. Repository satu `SELECT` dengan semua kolom `WHERE id = $1`, `ip::text`, jsonb di-decode menjadi `map[string]any`. Tanpa transaksi, tanpa join, tanpa entri audit baru untuk aksi melihat (tidak ada di katalog). Frontend: service + skema Zod `AuditEntryDetail`, store id terpilih, panel `AuditEntryDetailPanel` yang menampilkan JSON terformat sebagai teks (Svelte `{text}`, tanpa `{@html}`), diekspor lewat `index.ts` agar `app/` (usecase `listentries`) yang mengomposisikan. Usecase tidak dipecah: satu usecase `Get`.

## Plan

Urutan wajib. Backend B1-B7, Frontend F1-F7. Backend dan frontend bisa paralel setelah B1 (kontrak JSON tetap dari `base.Detail`).

### Backend

**B1. `getentry/params.go`** (transport, tanpa I/O selain membaca request)

```go
package getentry

type Params struct{ ID string }

// parseParams mengambil URLParam "id". Hanya uuid kanonis (36 karakter, huruf kecil
// atau besar, dengan tanda hubung) yang lolos; bentuk lain yang diterima uuid.Parse
// (urn:uuid:, kurung kurawal, tanpa hubung) ditolak. ID dinormalisasi ke lowercase.
// Gagal -> errs.Wrap(errs.CodeNotFound, "resource tidak ditemukan", errs.ErrNotFound)
// (BUKAN 400, agar konsisten dan tidak membocorkan alasan penolakan).
func parseParams(r *http.Request) (Params, error)
```

Aturan: `raw := chi.URLParam(r, "id")`; `len(raw) != 36` -> not found; `u, err := uuid.Parse(raw)`; err -> not found; `Params{ID: u.String()}`. Pesan error tidak memuat nilai input.

**B2. `getentry/usecase.go`** (orkestrasi saja, rule 5.2 dan 5.6)

```go
type Repository interface {
	Get(ctx context.Context, id string) (entry base.Entry, found bool, err error)
}

type Usecase struct{ repo Repository }

func NewUsecase(repo Repository) *Usecase

// Get: repo.Get -> !found => errs.ErrNotFound (dibungkus errs.Wrap CodeNotFound).
// err repo => fmt.Errorf("get audit entry: %w", err).
func (u *Usecase) Get(ctx context.Context, p Params) (base.Entry, error)
```

Catatan desain: README mengatur "tidak ketemu = `Found bool`" untuk kontrak bus. Ini endpoint HTTP, jadi usecase menerjemahkan `found=false` ke `errs.ErrNotFound` (sesuai instruksi usecase ini) dan handler cukup `respond.WriteError`, yang memetakan `CodeNotFound` ke 404. Tidak ada rule bisnis baru, karena itu tidak ada method entity baru; pembentukan JSON memakai `Entry.ToDetail()` dari foundation.

**B3. `getentry/repository.go`** (satu-satunya yang menyentuh SQL)

```go
type repository struct{ db db.Querier }   // *db.DB memenuhi db.Querier
func NewRepository(q db.Querier) Repository

const getSQL = `
SELECT id::text, occurred_at, actor_type, actor_id::text, actor_label,
       project_id::text, project_label, action, target_type, target,
       args, result, status, ip::text, user_agent, trace_id
FROM audit_log
WHERE id = $1`
```

- Query memakai `GetContext(ctx, &row, getSQL, id)`; `sql.ErrNoRows` -> `(base.Entry{}, false, nil)`; error lain dibungkus `fmt.Errorf("select audit_log: %w", err)`.
- `id` dikirim sebagai string; jika uuid valid sudah dijamin B1. Tidak ada concat SQL (rule 5.3).
- `row` struct lokal dengan `sql.NullString` untuk kolom nullable (`actor_id`, `project_id`, `project_label`, `target_type`, `target`, `ip`, `user_agent`) dan `[]byte` untuk `args`/`result`. `ip::text` pada `inet` host menghasilkan `203.0.113.5` (tanpa `/32`) atau `::1`; untuk jaga-jaga, hapus suffix `/32` atau `/128` bila ada (fungsi kecil `trimHostMask`, diuji).
- jsonb di-`json.Unmarshal` ke `map[string]any`; bila hasil `nil` jadikan `map[string]any{}`. Unmarshal gagal -> error bungkus (data rusak = 500, tidak bocor).
- `OccurredAt` dikembalikan `.UTC()`.
- Mapping ke `base.Entry` (string kosong = NULL, selaras konvensi entity). Tidak memanggil `NewEntry` (data sudah tersanitasi saat tulis; hindari menyaring ulang yang bisa mengubah bacaan).
- Tidak ada UPDATE/DELETE di file ini (arch_test AC7).

**B4. `getentry/handler.go`**

```go
type Handler struct{ uc *Usecase }

func (h *Handler) ServeHTTP / Get(w http.ResponseWriter, r *http.Request) {
	p, err := parseParams(r);     if err != nil { respond.WriteError(w, r, err); return }
	e, err := h.uc.Get(r.Context(), p); if err != nil { respond.WriteError(w, r, err); return }
	respond.WriteJSON(w, http.StatusOK, e.ToDetail())
}
```

Handler tidak berisi logika bisnis. Tanpa log tambahan pada 404 (bukan error server). 500 ditangani `respond.WriteError` (generic message, log di sana).

**B5. `getentry/init.go`**

```go
func Init(d app.Deps, r chi.Router) {
	h := NewHandler(NewUsecase(NewRepository(d.DB)))
	r.Get("/{id}", h.Get)
}
```

Rute terpasang di sub-router `/audit-log` yang sudah memakai `RequireRole(resolvesession.RoleSuperAdmin)` (W1). Tidak ada rute lain (tanpa POST/PUT/PATCH/DELETE, tanpa `r.Method`/`r.Handle`). Konflik dengan `listentries` `r.Get("/", ...)` tidak ada.

**B6. Tes backend** (lihat Test Scenario). Ditulis bersama kode (TDD): `params_test.go`, `usecase_test.go`, `handler_test.go`, `repository_test.go` (tag `integration`), `http_integration_test.go` (tag `integration`, AC5).

**B7. Verifikasi backend**: `go build ./...`, `go vet ./...`, `go test ./internal/domain/core/internal/auditlog/getentry/...`, lalu `go test -tags=integration` dengan `TEST_DATABASE_URL`. Pastikan `grep -iE "update\s+audit_log|delete\s+from\s+audit_log|truncate" getentry/` kosong.

### Frontend (`frontend/src/domain/core/audit-log/getentry/`)

Layering rule 12.2: service tanpa state, store tanpa fetch, component tanpa panggil service, view tipis. Tidak mengimpor `listentries/` (rule 12.4). Tidak ada string hard-coded: semua lewat `$t(...)` dengan kunci `auditLog.detail.*` dari foundation.

**F1. `types.ts`**

Tipe `AuditEntryDetail` diturunkan dari Zod (`z.infer`). Tipe `ActorType = 'user'|'api_key'|'anonymous'|'system'` dan `EntryStatus = 'ok'|'error'|'denied'` didefinisikan lokal di file ini (tidak mengimpor dari `listentries/types.ts`).

**F2. `services/getEntry.ts`** (+ skema Zod di `services/schema.ts` atau di file yang sama)

```ts
export const AuditEntryDetailSchema = z.object({
  id: z.string(),
  occurred_at: z.string(),
  actor_type: z.enum(['user','api_key','anonymous','system']),
  actor_id: z.string().nullable(),
  actor_label: z.string(),
  project_id: z.string().nullable(),
  project_label: z.string().nullable(),
  action: z.string(),
  target_type: z.string().nullable(),
  target: z.string().nullable(),
  status: z.enum(['ok','error','denied']),
  args: z.record(z.string(), z.unknown()),
  result: z.record(z.string(), z.unknown()),
  ip: z.string().nullable(),
  user_agent: z.string().nullable(),
  trace_id: z.string(),
});

export function getAuditEntry(id: string, signal?: AbortSignal): Promise<AuditEntryDetail> {
  return http.get(`/api/audit-log/${encodeURIComponent(id)}`, { schema: AuditEntryDetailSchema, signal });
}
```

Gunakan helper `get` atau `request('GET', ...)` dari `shared/services/http.ts` (sesuaikan dengan ekspor yang ada). 404/401/403 diteruskan sebagai `ApiError` (401 sudah ditangani handler global shared). Tidak ada retry, tidak ada cache.

**F3. `stores/selectedEntry.ts`**

```ts
export const selectedEntryId = writable<string | null>(null);
export function selectEntry(id: string | null): void
export function clearEntry(): void
```

Hanya state id (UI). Sinkronisasi `?entry=<id>` pada URL adalah tanggung jawab `app/` (view `AuditLogView`, usecase `listentries`) yang menyetel store ini; getentry tidak menyentuh router. Tidak memakai localStorage. Id tidak disimpan jika bukan string non-kosong.

**F4. `components/AuditEntryDetailPanel.svelte`** (presentasi murni)

Props: `entry: AuditEntryDetail | null`, `loading: boolean`, `error: 'notFound' | 'load' | null`, `onclose: () => void`. Tampilan:
- Header `auditLog.detail.title` + tombol tutup (`auditLog.detail.close`, `aria-label`).
- `loading` -> `Spinner` shared. `error==='notFound'` -> `EmptyState`/teks `auditLog.detail.notFound`. `error==='load'` -> `ErrorState` `auditLog.error.load`.
- Ringkasan: waktu (format lokal), aktor (`auditLog.actorType.*` + label), project, aksi, target, status (`auditLog.status.*`).
- Bagian `auditLog.detail.args` dan `auditLog.detail.result`: `<pre>` berisi `JSON.stringify(value, null, 2)` dirender sebagai teks Svelte (`{formatted}`). DILARANG `{@html}`, `innerHTML`, atau markdown render. Nilai berbentuk `"<img src=x onerror=alert(1)>"` tampil literal.
- Bagian `ip`, `userAgent`, `traceId`; nilai null ditampilkan "-" (bukan string i18n baru; gunakan karakter `-`). Lihat juga `actor_id`/`project_id` sebagai teks kecil monospace untuk pelacakan.
- Fungsi helper `formatJSON(v: unknown): string` di `components/format.ts` (try/catch, fallback `String(v)`), diuji.
- `pre` dengan `overflow:auto; white-space:pre-wrap; word-break:break-word` agar tidak memecah layout 360 px.

**F5. `views/AuditEntryDetailView.svelte`** (komposisi tipis, satu-satunya yang memanggil service)

Props: `id: string | null`, `onclose`. Reaktif terhadap `id`: batalkan request sebelumnya (`AbortController`), set loading, panggil `getAuditEntry`, tangkap `ApiError`: status 404 -> `error='notFound'`; lainnya -> `error='load'`; abaikan `AbortError`. Render `AuditEntryDetailPanel`. Tidak ada state global selain props.

**F6. `index.ts`** (API publik, rule 12.3)

```ts
export { default as AuditEntryDetailPanel } from './components/AuditEntryDetailPanel.svelte';
export { default as AuditEntryDetailView } from './views/AuditEntryDetailView.svelte';
export { selectedEntryId, selectEntry, clearEntry } from './stores/selectedEntry';
export { getAuditEntry } from './services/getEntry';
export type { AuditEntryDetail } from './types';
```

Komposisi (mengimpor `getentry` bersama `listentries` di `AuditLogView.svelte`, route, menu) ada di usecase `listentries` dan tidak dikerjakan di sini.

**F7. Verifikasi frontend**: `npm run check` (svelte-check), `npm run lint` bila ada, `npx vitest run src/domain/core/audit-log/getentry`. Pastikan tidak ada `{@html` di folder (grep sebagai langkah verifikasi; juga sebagai tes).

## Migration

Tidak ada. Memakai tabel `audit_log` dari `backend/migrations/core/20261021090000000_create_audit_log.sql` (foundation F1). Tidak ada env baru, dependensi Go (`github.com/google/uuid`, `chi`, `sqlx` sudah ada) maupun npm baru (`zod` sudah ada).

## Changes

| File | Aksi |
|---|---|
| `backend/internal/domain/core/internal/auditlog/getentry/init.go` | buat |
| `backend/internal/domain/core/internal/auditlog/getentry/params.go` | buat |
| `backend/internal/domain/core/internal/auditlog/getentry/handler.go` | buat |
| `backend/internal/domain/core/internal/auditlog/getentry/usecase.go` | buat |
| `backend/internal/domain/core/internal/auditlog/getentry/repository.go` | buat |
| `backend/internal/domain/core/internal/auditlog/getentry/params_test.go` | buat |
| `backend/internal/domain/core/internal/auditlog/getentry/usecase_test.go` | buat (repo palsu) |
| `backend/internal/domain/core/internal/auditlog/getentry/handler_test.go` | buat (usecase dengan repo palsu, `httptest`, chi) |
| `backend/internal/domain/core/internal/auditlog/getentry/repository_test.go` | buat (`//go:build integration`) |
| `backend/internal/domain/core/internal/auditlog/getentry/http_integration_test.go` | buat (`//go:build integration`, AC5) |
| `frontend/src/domain/core/audit-log/getentry/types.ts` | buat |
| `frontend/src/domain/core/audit-log/getentry/services/getEntry.ts` | buat |
| `frontend/src/domain/core/audit-log/getentry/services/getEntry.test.ts` | buat |
| `frontend/src/domain/core/audit-log/getentry/stores/selectedEntry.ts` (+ `.test.ts`) | buat |
| `frontend/src/domain/core/audit-log/getentry/components/AuditEntryDetailPanel.svelte` (+ `.test.ts`) | buat |
| `frontend/src/domain/core/audit-log/getentry/components/format.ts` (+ `.test.ts`) | buat |
| `frontend/src/domain/core/audit-log/getentry/views/AuditEntryDetailView.svelte` (+ `.test.ts`) | buat |
| `frontend/src/domain/core/audit-log/getentry/index.ts` | buat |

Tidak diubah (di luar allowedPaths, milik foundation/wiring/listentries): `auditlog/base.go`, `auditlog/base/*`, `api/modules/core/audit-log.yml`, i18n JSON, `app/` routes/views/layout.

## Test Scenario

Penamaan: `TestT02_ACn_*` untuk bukti kriteria (rule 13.4), lainnya `Test<Unit>_<Kasus>`. Integrasi memakai `db.TestDB(t)` dan tag `integration` (rule 13.3). Repo palsu hanya di tes usecase/handler (rule 13.2); entity tanpa mock.

### Unit backend

`params_test.go` (chi route context dengan URLParam `id`):
- uuid v7 kanonis -> `Params.ID` sama; uuid huruf besar -> dinormalisasi lowercase.
- `""`, `"abc"`, `"not-a-uuid"`, uuid tanpa hubung (32 hex), `"urn:uuid:<uuid>"`, `"{<uuid>}"`, uuid + spasi, uuid 37 karakter, `"../etc/passwd"`, string SQL `"' OR 1=1 --"` -> error `errors.Is(err, errs.ErrNotFound)` dan kode `CodeNotFound`; pesan error tidak memuat input.

`usecase_test.go` (repo palsu):
- repo `found=true` -> mengembalikan entry yang sama, tanpa error.
- `found=false` -> `errors.Is(err, errs.ErrNotFound)`.
- repo error -> error dibungkus (`errors.Is(err, sentinel)`), BUKAN `ErrNotFound`.
- `ctx` diteruskan dan `id` yang sama sampai ke repo (cek argumen).

`handler_test.go` (router chi mini, tanpa guard):
- `TestGetEntry_InvalidID_404`: `/not-a-uuid` -> 404, `error.code == "NOT_FOUND"`, repo palsu tidak dipanggil.
- `TestGetEntry_NotFound_404`: uuid valid, repo `found=false` -> 404 `NOT_FOUND`, body sama persis dengan kasus id invalid (tidak membedakan; kecuali `trace_id`).
- `TestGetEntry_OK_AllFields`: entry lengkap (api_key, project, target, args dengan `"api_key":"[REDACTED]"`, result, ip `203.0.113.5`, user_agent, trace_id) -> 200, `Content-Type: application/json`, decode ke map: tepat 16 kunci (`id, occurred_at, actor_type, actor_id, actor_label, project_id, project_label, action, target_type, target, status, args, result, ip, user_agent, trace_id`), nilai sama, `occurred_at` RFC3339 UTC.
- `TestGetEntry_NullableFields`: entri AC2 (anonymous, label `gk_zz99`, tanpa actor/project/target/ip/ua) -> `actor_id`, `project_id`, `project_label`, `target_type`, `target`, `ip`, `user_agent` bernilai JSON `null`; `args` `{}` bukan `null`.
- `TestGetEntry_InternalError_500`: repo error -> 500 `INTERNAL`, body tidak memuat teks error asli.
- `TestGetEntry_OnlyGetRegistered`: `chi.Walk` pada router hasil `Init` -> tepat satu rute `GET /{id}`; `POST/PUT/PATCH/DELETE /<uuid>` -> 405.

### Integrasi backend (Postgres nyata)

`repository_test.go`:
- `TestRepository_Get_RoundTrip`: INSERT langsung via SQL (bukan via `record`, supaya getentry mandiri) dengan semua kolom terisi termasuk `ip='203.0.113.5'::inet`, `args`/`result` jsonb bersarang (angka, bool, null, array) -> `Get` mengembalikan `found=true`, semua field sama, angka JSON dibaca sebagai `float64`, `occurred_at` UTC dengan presisi mikrodetik, `IP == "203.0.113.5"` (tanpa `/32`).
- IPv6: `::1` -> `"::1"`.
- Kolom NULL (`actor_id`, `project_id`, `project_label`, `target_type`, `target`, `ip`, `user_agent`) -> string kosong di `Entry`; `args`/`result` `'{}'` -> map kosong non-nil.
- `TestRepository_Get_NotFound`: uuid acak -> `found=false`, `err == nil`.
- `TestT02_AC3_SnapshotLabelsWithoutReferencedRows`: INSERT dengan `actor_id` dan `project_id` uuid yang tidak punya baris di tabel mana pun (tabel `users`/`projects` bahkan boleh belum ada) -> `Get` sukses, `ActorLabel` dan `ProjectLabel` snapshot utuh.
- `TestT02_AC4_ContentNotReturned`: entri yang tersimpan dengan `result={"bytes":12,"content":"[OMITTED]"}` -> dibaca apa adanya (repository tidak menyentuh/mengisi ulang).
- Isolasi: dua entri dengan id berbeda -> `Get(a)` hanya mengembalikan a.
- Context dibatalkan -> error (bukan `found=false`).

`http_integration_test.go` (router chi nyata: middleware identitas uji seperti `withIdentity` di foundation + `RequireRole(resolvesession.RoleSuperAdmin)` + `getentry.Init` dengan repo nyata):
- `TestT02_AC5_GetEntry_Roles`: anonim (tanpa identitas) -> 401 `UNAUTHORIZED`; `developer` -> 403 `FORBIDDEN`; `super_admin` -> 200 dengan detail. Untuk id yang tidak ada, developer tetap 403 (guard sebelum lookup; tidak bisa menebak id).
- `TestT02_AC7_NoMutationRoutes`: `POST/PUT/PATCH/DELETE /<uuid>` oleh super_admin -> 405 `METHOD_NOT_ALLOWED`; setelah itu baris di DB tidak berubah.
- `TestT02_AC3_DetailAfterReferencesDeleted`: entri dengan project/key id yang tidak ada baris -> 200 dengan label snapshot.

### Unit frontend (Vitest + Testing Library, `vi.mock`/`fetch` palsu)

`services/getEntry.test.ts`:
- respons 200 valid -> objek terparsing; URL `/api/audit-log/<id>` dengan `encodeURIComponent`; method GET tanpa header CSRF.
- respons tanpa `trace_id` atau `actor_type` di luar enum -> skema gagal (error validasi, bukan data setengah jadi).
- 404 -> `ApiError` `status 404`, `code NOT_FOUND`; 403 -> `ApiError` 403.
- `args` berisi `"<img src=x onerror=alert(1)>"` -> lolos sebagai string tanpa transformasi.

`stores/selectedEntry.test.ts`: awal `null`; `selectEntry('x')`; `clearEntry()`; `selectEntry('')` -> `null`.

`components/AuditEntryDetailPanel.test.ts`:
- `TestT02_HTMLArgsRenderedAsText`: `args = {"note":"<img src=x onerror=alert(1)>","html":"<script>alert(1)</script>"}` -> `container.querySelector('img')` dan `script` bernilai `null`; teks literal `<img src=x onerror=alert(1)>` ada di `pre`; tidak ada atribut `onerror` di DOM.
- Args/result diformat dengan indentasi 2 spasi; `result` kosong -> `{}`.
- Field null (ip, user agent, target) -> "-"; tidak ada teks "null"/"undefined".
- `loading` -> spinner, tanpa data; `error='notFound'` -> teks `auditLog.detail.notFound` (terjemahan id); `error='load'` -> `auditLog.error.load`.
- Tombol tutup memanggil `onclose`; dapat difokus dan punya `aria-label`.
- Locale `en` memakai teks Inggris (tidak ada string hard-coded; rule 12.8).
- Tes statis: `grep` seluruh file di folder `getentry/` tidak memuat `{@html` maupun `innerHTML`.

`components/format.test.ts`: objek bersarang, `BigInt`/siklus (try/catch fallback), string panjang tidak dipotong di UI.

`views/AuditEntryDetailView.test.ts`: `id` berubah A -> B membatalkan request A (hasil lambat A tidak menimpa B); 404 -> panel `notFound`; 500 -> panel `load`; `id=null` -> tidak ada request.

### Arsitektur/aturan (lintas)

- Tes arsitektur foundation W2 (`auditlog/arch_test.go`) mencakup folder ini: tidak ada `UPDATE`/`DELETE`/`TRUNCATE` pada `audit_log` di `getentry/`; tepat satu rute `GET` dari getentry.
- Tes arsitektur frontend (T00 F9): `getentry/` tidak mengimpor `listentries/` atau internals module lain; `shared/` tidak mengimpor `domain/`.

## E2E Scenarios

Positif:
1. Super admin membuka `/audit-log`, klik baris entri `mcp.read_file` -> `AuditEntryDetailView` memuat `GET /api/audit-log/{id}`; panel menampilkan args (`path`, `api_key: "[REDACTED]"`), result (`bytes`, `content: "[OMITTED]"`), IP `203.0.113.5`, user agent, trace id. (Pemasangan `?entry=<id>` di URL ditangani `listentries`/`app/`.)
2. Buka langsung `/audit-log?entry=<id>` (deep link/reload) -> panel terisi tanpa harus mengklik baris.
3. Entri AC2 (`mcp.auth.rejected`, anonymous, label `gk_zz99`, denied) -> panel menampilkan aktor "anonymous" + label prefix, project "-", `result.reason = invalid_key`.
4. Entri dengan `args` bernilai `<img onerror>` -> tampil sebagai teks, tidak ada gambar/dialog alert.
5. Project dan API key sudah dihapus (T03/T04) -> entri lama tetap dapat dibuka dengan label snapshot (AC3).
6. Pindah dari entri A ke B -> panel diperbarui, request A dibatalkan; tombol tutup mengosongkan `selectedEntryId`.

Negatif:
1. Developer memanggil `GET /api/audit-log/{id}` -> 403 `FORBIDDEN`; UI `/audit-log` tidak tersedia untuk developer (guard `meta.roles` di `app/`).
2. Tanpa sesi -> 401 `UNAUTHORIZED`; UI redirect ke login (handler global `shared/services/http.ts`).
3. `GET /api/audit-log/not-a-uuid` -> 404 `NOT_FOUND`; uuid valid yang tidak ada -> 404 dengan body identik; panel menampilkan `auditLog.detail.notFound`.
4. `POST/PUT/PATCH/DELETE /api/audit-log/{id}` -> 405 `METHOD_NOT_ALLOWED` (AC7).
5. DB mati saat membuka detail -> 500 `INTERNAL` generik; panel menampilkan `auditLog.error.load`; tidak ada stack/SQL bocor.
6. Jaringan putus -> panel `auditLog.error.load`, dapat ditutup, daftar tetap berfungsi.

## Request/Response body

### `GET /api/audit-log/0192f0c4-7b00-7d11-8a00-1a2b3c4d5e6f`

Header: cookie sesi super_admin. Tanpa body.

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

200 (entri AC2, anonymous ditolak):

```json
{
  "id": "0192f0c4-8000-7a00-9c00-aabbccddeeff",
  "occurred_at": "2026-10-19T09:01:05.000000Z",
  "actor_type": "anonymous",
  "actor_id": null,
  "actor_label": "gk_zz99",
  "project_id": null,
  "project_label": null,
  "action": "mcp.auth.rejected",
  "target_type": null,
  "target": null,
  "status": "denied",
  "args": {},
  "result": { "reason": "invalid_key" },
  "ip": "198.51.100.7",
  "user_agent": null,
  "trace_id": "5d0a7e13b2c94f60"
}
```

400: tidak digunakan oleh endpoint ini (id invalid memakai 404 sengaja).

401:

```json
{ "error": { "code": "UNAUTHORIZED", "message": "perlu login", "details": null }, "trace_id": "4f1c2b7e9a0d4e70" }
```

403 (developer):

```json
{ "error": { "code": "FORBIDDEN", "message": "akses ditolak", "details": null }, "trace_id": "4f1c2b7e9a0d4e71" }
```

404 (id bukan uuid atau tidak ada; body identik):

```json
{ "error": { "code": "NOT_FOUND", "message": "resource tidak ditemukan", "details": null }, "trace_id": "4f1c2b7e9a0d4e72" }
```

405 (`DELETE /api/audit-log/{id}` dan method ubah lain):

```json
{ "error": { "code": "METHOD_NOT_ALLOWED", "message": "method tidak diizinkan", "details": null }, "trace_id": "4f1c2b7e9a0d4e73" }
```

500:

```json
{ "error": { "code": "INTERNAL", "message": "terjadi kesalahan", "details": null }, "trace_id": "4f1c2b7e9a0d4e74" }
```

(Teks pesan 405/500 mengikuti `respond`/foundation; kode error yang stabil. Sesuaikan teks pada tes dengan keluaran `respond.WriteError` aktual, dan asersi utama pada `error.code` serta status.)

## Catatan dan risiko

- Info yang sensitif sudah disaring saat tulis (`base.Sanitize`); getentry tidak menyaring ulang tetapi UI tidak pernah merender sebagai HTML, sehingga data yang tidak dipercaya (args dari agent) aman.
- `ip::text` dipakai agar tidak perlu codec `inet` di driver; fungsi `trimHostMask` mengantisipasi bentuk `x/32`.
- Pesan 404 sama untuk id invalid dan id tidak ada, sehingga penyerang (walau sudah super_admin) tidak mendapat perbedaan perilaku; guard role tetap sebelum lookup.
- Nama package infra (`respond`, `middleware`, `db.Querier`, `app.Deps`) harus dicek ulang terhadap `main` setelah T00 di-merge; gunakan nama final tanpa alias buatan sendiri.
