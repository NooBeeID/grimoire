# T14 / project/git-sync / getconfig — Plan Usecase

Tiket: `docs/tickets/T14-project-git-sync.md`. Dokumen induk (menang bila bertentangan): `docs/superpowers/plans/project-git-sync/overview.md` (bagian 4 D5-D7, 6.2, 6.4.1, 6.6, 12.4). Frontend rinci: `frontend-plan.md` (dialog Sync). Pola acuan kode: `backend/internal/domain/project/internal/apikey/listkeys/{init,handler,usecase,repository}.go` dan `knowledge/base/access`.
Konteks yang dicek: `CLAUDE.md` tidak ada; `.claude/rules/` hanya `migrations.md` (tidak relevan, tanpa migrasi).

---

## 1. Metadata

| Atribut | Nilai |
|---|---|
| Ticket | T14 (kriteria penerimaan 8, 9, 12) |
| Scope | fullstack (backend usecase ini + service/store FE `getconfig`) |
| Domain / Module / Feature | project / git-sync (`gitsync`) / `getconfig` |
| Folder backend | `backend/internal/domain/project/internal/gitsync/getconfig` |
| Folder frontend | `frontend/src/domain/project/git-sync/getconfig` |
| Endpoint | `/api/projects/{projectID}/git-sync?kind=knowledge\|api_spec` |
| Method | GET |
| Version | prefix `/api` tanpa versi; `api/modules/project/git-sync.yml` `info.version` 0.1.0 |
| Siapa | anggota project, super_admin (non-anggota boleh); `RequireAuth` |
| Bus dipanggil | `project.management.check-access` (via `base/access`), `core.gitcredential.get-token` (fail-soft) |
| Ketergantungan | FOUNDATION: `gitsync/base` (`Kind`, `ParseKind`, `Config`, `ConfigDTO`, `Report`, `access`, `store`, `errors`), kontrak `gettoken`, konstanta bus, migrasi `git_sync_configs`, `Config.GitFeatureEnabled()`. Tidak bergantung pada usecase lain (tes memakai fake bus get-token). |

---

## 2. Summary Plan

Endpoint baca-saja untuk mengisi dialog "Sync dari Git" (kriteria 9): mengembalikan konfigurasi sync terakhir per `(project, kind)`, hasil sync terakhir (Report dipotong 500 item), dan status PAT milik pemanggil. Alur usecase (urutan wajib):

1. `access.Require(ctx, projectID)` via bus check-access: tanpa login 401, non-anggota/project tak ada 404, `member|owner|admin` lolos (super_admin non-anggota = `admin`).
2. `base.ParseKind(rawKind)` -> 400 `INVALID_ARGUMENT` (details.fields.kind). Akses diperiksa SEBELUM kind, jadi non-anggota tidak bisa menebak validasi (selalu 404).
3. `repo.Get(projectID, kind)` -> `*base.Config` atau `nil` (tidak ada baris = bukan error). Config kosong -> respons 200 `config: null`.
4. Hanya bila `config != nil` dan fitur aktif: bus `core.gitcredential.get-token` dengan `UserID = actor.ID` (identity request, BUKAN `last_synced_by`) dan `(config.Provider, config.Host)`. Hasil hanya dipakai untuk dua boolean:
   - `has_credential = Found && !Unreadable && !Disabled`
   - `credential_unreadable = Found && Unreadable && !Disabled`
   - Error apa pun dari bus (handler belum terdaftar, DB error, kontrak invalid) -> fail-soft: `has_credential=false`, `credential_unreadable=false`, log `warn`, TETAP 200 (bukan 500).
   - Field `Response.Token` tidak pernah dibaca, disalin, di-log, atau dimasukkan ke respons.
5. `feature_enabled = d.Config.GitFeatureEnabled()` (dan `false` bila bus melapor `Disabled`).
6. Susun `GetResponse` dan kirim 200. Tanpa audit (read-only), tanpa tulis, tanpa transaksi.

Keputusan lokal:
- Bila `config == nil`, bus get-token TIDAK dipanggil (provider/host belum diketahui) -> `has_credential=false`. FE membedakan "belum ada config" dari "belum ada token" lewat `config == null`; pengecekan token pada sync pertama dilakukan runsync (`CREDENTIAL_MISSING`). Sesuai contoh overview 12.4.
- Default Pertanyaan terbuka 1: tanpa previewsync; peringatan "file bernama sama akan ditimpa" ditampilkan FE, tidak ada hubungan dengan endpoint ini.
- Pemotongan Report: `config.LastResult.CapForStorage(base.MaxStoredReportItems)` (konstanta 500 di `gitsync/base/report.go`, dipakai juga oleh runsync saat menyimpan). Pemotongan defensif di getconfig menjaga respons kecil walau data lama lebih besar. `Summary` selalu utuh; `truncated=true` bila dipotong.
- `last_synced_by` pada DTO = `{id, label}` dari kolom `last_synced_by` + `last_synced_by_label` (snapshot, tanpa join users); `null` bila kolom NULL.
- Tidak ada token/PAT di respons, log, atau error (D6).

---

## 3. Plan (langkah berurutan)

Prasyarat: FOUNDATION selesai (langkah 0). Feature ini hanya menambah folder `getconfig/**`, satu baris `getconfig.Init(d)` di `gitsync/base.go`, dan satu baris di tes registrasi/route module.

### Langkah 1 — Verifikasi prasyarat foundation (tanpa mengubah kecuali disebut)

Cek bahwa yang berikut ada dengan signature yang dipakai di bawah. Bila ada selisih nama, sesuaikan pemakaian di feature (jangan ubah kontrak):
- `gitsync/base`: `Kind`, `ParseKind`, `Config`, `Config.ToDTO()`, `ConfigDTO`, `ReportDTO`, `SyncedByDTO`, `Report.CapForStorage`, `Actor`, `ErrInvalidKind`, `ErrProjectNotFound`, `ErrUnauthenticated`.
- `gitsync/base/access.Checker` (`Require(ctx, projectID) (base.Actor, error)`).
- `gitsync/base/store`: helper `Columns` (konstanta kolom select `configColumns`) dan `ScanConfig`/`Store` agar SQL kolom tidak diduplikasi dengan runsync. Bila foundation hanya menyediakan versi unexported, ekspor (`store.ConfigColumns`, `store.Store.SelectConfig`) di foundation, bukan menyalin di feature.
- `gitsync/base.go` memanggil `registerCodes()` (`sync.Once`) untuk kode gitsync. Feature ini TIDAK mendaftarkan kode sendiri dan tidak butuh kode baru (hanya `NOT_FOUND`, `INVALID_ARGUMENT`, `UNAUTHORIZED` bawaan). Bila `registerCodes` belum dipasang di `base.go`, pasang di sana (satu tempat), tidak di `getconfig.Init`.
- Konstanta `base.MaxStoredReportItems = 500` di `base/report.go`; bila belum ada, tambahkan di sana (aditif).

### Langkah 2 — `params.go`

```go
package getconfig

// params adalah input terurai dari request (belum divalidasi domain).
type params struct {
	ProjectID string // chi.URLParam "projectID"
	RawKind   string // r.URL.Query().Get("kind"), trim spasi; kosong bila tidak ada
}

func parseParams(r *http.Request) params
```

Tidak ada body. Validasi `kind` bukan tanggung jawab file ini (entity `base.ParseKind`).

### Langkah 3 — `repository.go`

```go
package getconfig

const getSQL = `SELECT ` + store.ConfigColumns + `
FROM git_sync_configs
WHERE project_id = $1 AND kind = $2`

type Repo struct{ s *store.Store }
func newRepo(s *store.Store) *Repo { return &Repo{s: s} }

// Get mengembalikan config (project, kind) atau (nil, nil) bila belum ada.
// Fail-closed: projectID bukan uuid -> (nil, nil) tanpa query.
func (r *Repo) Get(ctx context.Context, projectID string, kind base.Kind) (*base.Config, error)
```

- Satu-satunya yang menyentuh SQL di feature ini. Kolom UUID di-cast `::text` (di `store.ConfigColumns`), `last_result` jsonb di-unmarshal ke `*base.Report` oleh `store.ScanConfig`; jsonb rusak -> `LastResult=nil` + tidak error (data historis jangan mematikan dialog), dicatat oleh store sebagai warn tanpa isi.
- `sql.ErrNoRows` -> `(nil, nil)`. Error lain: `fmt.Errorf("getconfig: get: %w", err)`.
- Waktu dinormalisasi UTC.
- Tidak ada `SELECT *`; tidak ada kolom token (tabel memang tidak menyimpan PAT).

### Langkah 4 — `usecase.go`

```go
package getconfig

type ConfigReader interface {
	Get(ctx context.Context, projectID string, kind base.Kind) (*base.Config, error)
}

// CredentialLookup membungkus bus get-token. Hanya status yang dikembalikan; token tidak pernah keluar.
type CredentialLookup interface {
	Status(ctx context.Context, userID, provider, host string) (CredentialStatus, error)
}

type CredentialStatus struct{ Found, Unreadable, Disabled bool }

type GetResponse struct {
	Kind                 string           `json:"kind"`
	Config               *base.ConfigDTO  `json:"config"` // null bila belum ada
	HasCredential        bool             `json:"has_credential"`
	CredentialUnreadable bool             `json:"credential_unreadable"`
	FeatureEnabled       bool             `json:"feature_enabled"`
}

type Usecase struct {
	access  access.Checker
	repo    ConfigReader
	cred    CredentialLookup
	enabled bool          // cfg.GitFeatureEnabled()
	log     *slog.Logger
}

func NewUsecase(a access.Checker, r ConfigReader, c CredentialLookup, featureEnabled bool, log *slog.Logger) *Usecase
func (u *Usecase) Execute(ctx context.Context, in params) (GetResponse, error)
```

Adapter bus (di `usecase.go`, tipe `busCredentials{b *bus.Registry}`):

```go
func (c busCredentials) Status(ctx context.Context, userID, provider, host string) (CredentialStatus, error) {
	res, err := bus.CallTyped[gettoken.Response](ctx, c.b, constants.BUS_CORE_GITCREDENTIAL_GET_TOKEN,
		gettoken.Request{UserID: userID, Provider: provider, Host: host}) // request sebagai NILAI
	if err != nil { return CredentialStatus{}, err }
	return CredentialStatus{Found: res.Found, Unreadable: res.Unreadable, Disabled: res.Disabled}, nil // res.Token dibuang
}
```

Urutan `Execute` (orkestrasi saja; rule ada di entity):
1. `actor, err := u.access.Require(ctx, in.ProjectID)`; err -> return apa adanya (401/404/500).
2. `kind, err := base.ParseKind(in.RawKind)`; err -> return (400).
3. `cfg, err := u.repo.Get(ctx, in.ProjectID, kind)`; err -> `fmt.Errorf("getconfig: %w", err)` (500 generik).
4. `resp := GetResponse{Kind: string(kind), FeatureEnabled: u.enabled}`; `cfg == nil` -> return resp (Config nil -> JSON `null`).
5. `dto := cfg.ToDTO()`; bila `cfg.LastResult != nil` pakai report terpotong: `capped := cfg.LastResult.CapForStorage(base.MaxStoredReportItems)` lalu isi `dto.LastResult` dari `capped`. `resp.Config = &dto`.
6. Bila `u.enabled`: `st, err := u.cred.Status(ctx, actor.ID, cfg.Provider, cfg.Host)`.
   - `err != nil` -> `u.log.WarnContext(ctx, "gitsync getconfig: cek token gagal", "project_id", in.ProjectID, "kind", kind, "error", err)`; biarkan dua boolean `false`.
   - `st.Disabled` -> `resp.FeatureEnabled = false`.
   - Selain itu `resp.HasCredential = st.Found && !st.Unreadable`; `resp.CredentialUnreadable = st.Found && st.Unreadable`.
7. Return `resp, nil`.

Larangan: tidak ada `audit`, `bus.Publish`, penulisan DB, atau penggunaan `last_synced_by` sebagai UserID.

### Langkah 5 — `handler.go`

```go
type handler struct{ uc *Usecase }
func newHandler(uc *Usecase) *handler { return &handler{uc: uc} }
func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	res, err := h.uc.Execute(r.Context(), parseParams(r))
	if err != nil { respond.WriteError(w, r, err); return }
	respond.WriteJSON(w, http.StatusOK, res)
}
```

Header `Cache-Control: no-store` ditambahkan sebelum `WriteJSON` (status PAT per user tidak boleh di-cache proxy).

### Langkah 6 — `init.go` (pola penuh di `d.API`)

```go
// Init mendaftarkan GET /api/projects/{projectID}/git-sync.
func Init(d app.Deps) {
	uc := NewUsecase(access.New(d.Bus), newRepo(store.New(d.DB)),
		busCredentials{b: d.Bus}, d.Config.GitFeatureEnabled(), d.Log)
	d.API.With(middleware.RequireAuth()).Get("/projects/{projectID}/git-sync", newHandler(uc).ServeHTTP)
}
```

Tanpa `BodyLimit`/`ExtendTimeout` (GET cepat, timeout default). CSRF/Session sudah global di `api.Use`. Route `/projects/...` memakai `With(...).Get`, bukan `Route` (konvensi README).

### Langkah 7 — Wiring dan OpenAPI

- `gitsync/base.go`: tambahkan `getconfig.Init(d)` di `Register` (satu baris; berdampingan dengan `runsync.Init`).
- Tes registrasi module (`gitsync/routes_test.go` atau yang disediakan foundation): tambahkan asersi `GET /projects/{projectID}/git-sync` terdaftar dan memakai `RequireAuth` (tanpa sesi -> 401).
- `api/modules/project/git-sync.yml`: foundation sudah memuat path dan schema. Verifikasi operasi `getGitSyncConfig`: parameter query `kind` (enum `knowledge|api_spec`, required), respons 200 `GitSyncConfigResponse` (`kind`, `config` nullable, `has_credential`, `credential_unreadable`, `feature_enabled`), 400/401/404 `Error`. Rapikan deskripsi/contoh sesuai bagian 8; jalankan `make api-bundle` + `make api-check`; commit `api/openapi.yml`.

### Langkah 8 — Tes (bagian 5) lalu verifikasi

`go build ./...`, `go test ./internal/domain/project/internal/gitsync/getconfig/...`, `go test -tags integration` untuk paket yang sama, `backend/internal/archtest`, lint, `make api-check`.

### Langkah 9 — Frontend (kontrak mengikuti bagian 8; komposisi dialog ada di `frontend-plan.md`)

Folder `frontend/src/domain/project/git-sync/getconfig/` (mengikuti struktur `domain/<domain>/<module>/<feature>` yang ada di FE; ekspor lewat `index.ts`):

- `schema.ts` (Zod, snake_case dari API):
  - `GitSyncKind = z.enum(['knowledge','api_spec'])`
  - `GitSyncIssue = {code, message, pointer, line: number}`
  - `GitSyncReportItem = {repo_path, target, status: enum('created'|'updated'|'unchanged'|'skipped'|'rejected'), reason_code, reason, issues: Issue[].default([]), openapi_version?, title?, endpoint_count?}`
  - `GitSyncReport = {status: enum('success'|'partial'|'failed'), summary: {created,updated,unchanged,skipped,rejected}, items, truncated: boolean, error_code, error_message}`
  - `GitSyncConfig = {provider: enum('github'|'gitlab'), host, repo_url, repo_path, branch, folder, target_prefix, last_status: enum|null, last_synced_at: string|null, last_synced_by: {id,label}|null, last_result: GitSyncReport|null}`
  - `GetConfigResponse = {kind, config: GitSyncConfig.nullable(), has_credential, credential_unreadable, feature_enabled}`
  - Skema `GitSyncReport`/`GitSyncConfig` didefinisikan di `domain/project/git-sync/shared` (atau `schema.ts` bersama) agar `runsync` memakai ulang; sesuaikan dengan keputusan struktur di `frontend-plan.md` (jangan duplikasi).
- `service.ts`: `getConfig(projectID, kind): Promise<GetConfigResponse>` memakai klien HTTP bersama (`shared/api`), `GET /projects/${projectID}/git-sync?kind=${kind}`, parse dengan Zod; error 400/404/401 diteruskan sebagai `ApiError` standar.
- `store.svelte.ts` (atau pola store FE yang ada): state per kunci `${projectID}:${kind}` -> `{status: 'idle'|'loading'|'ready'|'error', data, error}`; `load(projectID, kind)`, `invalidate(projectID, kind)`, `setFromRun(projectID, kind, config)` agar `runsync` dapat memperbarui ringkasan tanpa fetch ulang; `reset()` saat logout/ganti project. Panggilan `load` yang sedang berjalan untuk kunci sama tidak diduplikasi. Tanpa persist ke localStorage.
- Dipakai `GitSyncDialog` (komposisi di `app/views/...`, bukan di domain getconfig): saat dialog dibuka -> `load`; `config` mengisi URL/Branch/Folder/"Simpan di folder" otomatis; ringkasan "Terakhir: {waktu} oleh {label} · {status} · {angka}" dari `last_result.summary`; tombol **Sync ulang** (memanggil `runsync` dengan nilai config tersimpan; nonaktif bila `!has_credential` atau `!feature_enabled`); bila `config != null && !has_credential` (atau `credential_unreadable`) tampil kotak info "Atur Token Git" -> `/profile#token-git`, tombol Sync nonaktif; `feature_enabled=false` -> teks "Sync Git belum dikonfigurasi oleh admin". Tanpa import silang ke `domain/core/git-credential` (depcruise).
- i18n (`id.json`, `en.json`) kunci `gitSync.config.*`: `lastSync`, `neverSynced`, `statusSuccess|Partial|Failed`, `syncAgain`, `credentialMissing`, `credentialUnreadable`, `featureDisabled`, `loadFailed`. Mengikuti kunci yang didefinisikan `frontend-plan.md` bila sudah ada.

---

## 4. Migration

Tidak ada. Tabel `git_sync_configs` dibuat oleh migrasi FOUNDATION `backend/migrations/project/<VERSION>_create_git_sync_configs.sql` (overview bagian 7.2). Query memakai unique `(project_id, kind)` sebagai index (lookup titik).

---

## 5. Changes (per file/folder)

| File | Jenis |
|---|---|
| `backend/internal/domain/project/internal/gitsync/getconfig/init.go` | baru |
| `.../getconfig/handler.go` | baru |
| `.../getconfig/params.go` | baru |
| `.../getconfig/usecase.go` | baru (termasuk `busCredentials`) |
| `.../getconfig/repository.go` | baru |
| `.../getconfig/handler_test.go` | baru |
| `.../getconfig/usecase_test.go` | baru |
| `.../getconfig/repository_integration_test.go` | baru (`//go:build integration`) |
| `backend/internal/domain/project/internal/gitsync/base.go` | ubah: `getconfig.Init(d)` (1 baris) |
| `backend/internal/domain/project/internal/gitsync/routes_test.go` (atau tes registrasi foundation) | ubah: asersi route (1 blok) |
| `backend/internal/domain/project/internal/gitsync/base/report.go` | ubah bila perlu: `MaxStoredReportItems = 500` (aditif) |
| `backend/internal/domain/project/internal/gitsync/base/store/*` | ubah bila perlu: ekspor kolom/scan (aditif) |
| `api/modules/project/git-sync.yml`, `api/openapi.yml` | verifikasi/rapikan operasi GET |
| `frontend/src/domain/project/git-sync/getconfig/{schema.ts,service.ts,store.svelte.ts,index.ts}` + `*.test.ts` | baru |
| `frontend/src/shared/i18n/{id,en}.json` | ubah: kunci `gitSync.config.*` |

Tidak berubah: `app.Deps`, module core/gitcredential, knowledge, apispec.

---

## 6. Test Scenario

Fake yang dipakai: `fakeAccess` (return `base.Actor{ID:"u1",Role:"developer",Label:"a@tim.id"}` atau error; catat `projectID`), `fakeRepo` (return config/nil/error; catat argumen, jumlah panggilan), `fakeCred` (return `CredentialStatus`/error; catat `userID, provider, host`, jumlah panggilan), logger buffer.

### 6.1 `usecase_test.go` (unit)

| # | Kasus | Harapan |
|---|---|---|
| 1 | config ada, token Found | `config` terisi (semua field DTO termasuk `last_synced_by{id,label}`), `has_credential=true`, `credential_unreadable=false`, `feature_enabled=true` |
| 2 | config tidak ada (`repo` -> nil) | `Config==nil`, `has_credential=false`, bus get-token TIDAK dipanggil (calls=0), tanpa error |
| 3 | hasCredential false: get-token `Found=false` | `has_credential=false`, `credential_unreadable=false` |
| 4 | unreadable: `Found=true, Unreadable=true` | `has_credential=false`, `credential_unreadable=true` |
| 5 | get-token `Disabled=true` | `feature_enabled=false`, kedua boolean false |
| 6 | get-token error (`errors.New("boom")`, juga `bus.ErrNotRegistered`) | `has_credential=false`, `credential_unreadable=false`, `err==nil` (tidak 500), warn tercatat di log |
| 7 | `enabled=false` dari config (key kosong) | get-token tidak dipanggil, `feature_enabled=false`, config tetap dikembalikan |
| 8 | UserID get-token = identity request | `fakeCred.userID == "u1"` walau `config.LastSyncedBy="u-other"`; provider/host = milik config |
| 9 | akses: non-anggota -> `ErrProjectNotFound` | err 404; `repo` dan `cred` calls=0 |
| 10 | akses: tanpa login -> `ErrUnauthenticated` | err 401; repo calls=0 |
| 11 | akses error infra | error asli (bukan `*errs.Error`) dipropagasi; repo calls=0 |
| 12 | urutan: kind invalid + non-anggota | 404 (akses lebih dulu), repo tidak dipanggil |
| 13 | kind invalid ("", "foo", "Knowledge ") dengan akses OK | `ErrInvalidKind` (400, details.fields.kind); repo calls=0 |
| 14 | super_admin non-anggota (access mengembalikan actor admin) | lolos, respons normal |
| 15 | repo error | error terbungkus `getconfig: ...`, bukan `*errs.Error` (-> 500 generik) |
| 16 | `last_result` 600 item | respons memuat 500 item, `truncated=true`, `summary` tetap penuh (angka tidak berubah) |
| 17 | `last_result` nil, `last_status` nil (config belum pernah sync) | DTO `last_result:null`, `last_status:null`, `last_synced_by:null` |
| 18 | PAT canary: `gettoken.Response.Token = secret.New("ghp_CANARY...")` lewat adapter `busCredentials` dengan bus nyata + handler fake | `json.Marshal(resp)`, buffer log, dan pesan error tidak memuat canary; `CredentialStatus` tidak punya field token (asersi lewat refleksi: field set == {Found,Unreadable,Disabled}) |
| 19 | adapter `busCredentials`: request dikirim sebagai NILAI `gettoken.Request` (bukan pointer); respons tipe salah -> error (fail-soft di usecase) | lulus |

### 6.2 `handler_test.go` (unit, chi router + `reqctx.WithIdentity`)

| # | Kasus | Harapan |
|---|---|---|
| 1 | 200 config ada | `Content-Type: application/json`, `Cache-Control: no-store`, body memuat `"kind":"knowledge"`, `"has_credential":true`, `"feature_enabled":true`, `"last_synced_at":"2026-10-22T14:05:00Z"`, `"last_synced_by":{"id":...,"label":"Rina"}`; semua key snake_case; tidak ada kata `token` selain `has_credential` |
| 2 | 200 config kosong | `"config":null` (bukan 404), `"has_credential":false` |
| 3 | `?kind=api_spec` diteruskan | repo menerima `KindAPISpec`; `projectID` dari URL param diteruskan |
| 4 | tanpa `kind` / `kind=foo` | 400 `INVALID_ARGUMENT`, `details.fields.kind` |
| 5 | non-anggota | 404 `NOT_FOUND`, `error.code` + `trace_id` ada |
| 6 | tanpa identity (handler langsung) | 401 `UNAUTHORIZED` |
| 7 | repo error | 500, body generik tanpa pesan internal |
| 8 | route lewat `Init` pada router uji dengan `RequireAuth` | tanpa sesi 401 sebelum usecase dipanggil (uji di `routes_test.go`) |

Handler test memakai kode bawaan saja (tidak butuh `registerCodes()` gitsync).

### 6.3 `repository_integration_test.go` (tag `integration`, Postgres nyata; helper `setupDB` = `db.TestDB` + `db.Up`)

| # | Kasus | Harapan |
|---|---|---|
| 1 | tabel kosong | `(nil, nil)` |
| 2 | satu baris `(P, knowledge)` | semua kolom terbaca benar (provider, host, repo_url, repo_path, branch, folder, target_prefix, last_status, last_synced_at UTC, last_synced_by, label); id uuid sebagai string |
| 3 | `(P, knowledge)` dan `(P, api_spec)` ada | `Get(..., api_spec)` hanya mengembalikan baris api_spec |
| 4 | `(P, knowledge)` ada, `Get(Q, knowledge)` | `nil` (isolasi project) |
| 5 | `last_result` jsonb valid | `LastResult` terurai (summary, items, `truncated`) |
| 6 | `last_result` NULL | `LastResult == nil` |
| 7 | `last_result` jsonb rusak (mis. `{"items": 5}`) | tidak error, `LastResult == nil` |
| 8 | `projectID` bukan uuid ("abc") | `(nil, nil)` tanpa error / tanpa query |
| 9 | hapus project -> config terhapus (CASCADE) lalu Get | `nil` |

### 6.4 Arch / lain

- `backend/internal/archtest`: `getconfig` hanya mengimpor `gitsync/base`, `lib/contracts`, `lib/constants`, `infra/{bus,app,middleware,http/respond}`; TIDAK mengimpor `gitcredential`, `knowledge`, `apispec`.
- `make api-check` hijau.

### 6.5 Frontend (vitest)

- `schema.test.ts`: contoh respons 12.4 (config ada) lolos; `config:null` lolos; `kind` salah ditolak; `last_result.items[].issues` default `[]`; field tak dikenal diabaikan.
- `service.test.ts`: URL dan query benar (`?kind=api_spec`), parse sukses, 404 -> error `ApiError` dengan `code NOT_FOUND`.
- `store.test.ts`: `load` mengisi per kunci `(projectID, kind)` tanpa saling menimpa; dua `load` serentak -> satu fetch; `invalidate` memicu fetch ulang; `setFromRun` memperbarui ringkasan; error menyetel `status='error'`.
- Komponen (di tes dialog `frontend-plan.md`): form terisi dari config, `has_credential=false` -> tombol Sync dan Sync ulang nonaktif + kotak "Atur Token Git", `credential_unreadable=true` -> teks simpan ulang, `feature_enabled=false` -> teks admin.

---

## 7. E2E Scenarios (`frontend/e2e/git-sync.spec.ts`, fixture fake git server dari overview bagian 11)

Positif:
1. Project baru, buka Knowledge > "Sync dari Git": form kosong, tanpa ringkasan "Terakhir" (request `GET .../git-sync?kind=knowledge` -> 200 `config:null`).
2. Setelah sync pertama sukses, tutup dan buka dialog lagi: URL, branch `main`, folder `docs` terisi; ringkasan "Terakhir: ... · Berhasil · 2 dibuat"; "Sync ulang" aktif.
3. Dua kind terpisah: API Spec dialog tidak menampilkan config knowledge (request `kind=api_spec` -> `config:null`).
4. Setelah "Sync ulang" sukses, ringkasan memperlihatkan waktu dan status terbaru tanpa reload halaman.
5. Super_admin non-anggota membuka dialog: 200 dan config terlihat.
6. Anggota B tanpa PAT: config A terlihat (URL/branch/ringkasan) tetapi `has_credential=false`: kotak "Atur Token Git" tampil dengan tautan ke `/profile#token-git`, tombol Sync dan Sync ulang nonaktif.

Negatif:
1. Non-anggota memanggil `GET /api/projects/{P}/git-sync?kind=knowledge` -> 404; tanpa login -> 401; `kind=foo` oleh anggota -> 400.
2. User menghapus PAT lalu membuka dialog: `has_credential=false`, tombol nonaktif; menyimpan ulang PAT lalu membuka dialog: kembali aktif.
3. Backend dijalankan dengan `GIT_CREDENTIAL_KEY` berbeda dari saat PAT disimpan: dialog menampilkan "token tidak dapat dibaca, simpan ulang" (`credential_unreadable=true`), bukan error 500.
4. Backend tanpa `GIT_CREDENTIAL_KEY`: `feature_enabled=false`, dialog menampilkan "Sync Git belum dikonfigurasi oleh admin".
5. Config dengan sync terakhir gagal (folder `tidak-ada`): ringkasan menampilkan "Gagal" dan pesan error terakhir; form tetap terisi.
6. Pemeriksaan jaringan: tidak ada respons `GET /git-sync` yang memuat string token uji (`ghp_e2eValidToken7Qx2`).

---

## 8. Request/Response body

Format error seragam: `{"error": {"code", "message", "details"}, "trace_id"}`. Request: GET tanpa body, tanpa header CSRF khusus (metode aman). Sesi lewat cookie.

### 8.1 Request

```
GET /api/projects/0192f5a0-1111-7aaa-8bbb-000000000001/git-sync?kind=knowledge
```

### 8.2 200 — config ada, PAT milik pemanggil tersedia

```json
{
  "kind": "knowledge",
  "feature_enabled": true,
  "has_credential": true,
  "credential_unreadable": false,
  "config": {
    "provider": "github",
    "host": "github.com",
    "repo_url": "https://github.com/noobee/grimoire-docs/tree/main/docs",
    "repo_path": "noobee/grimoire-docs",
    "branch": "main",
    "folder": "docs",
    "target_prefix": "",
    "last_status": "partial",
    "last_synced_at": "2026-10-22T14:05:00Z",
    "last_synced_by": { "id": "0192f5a0-aaaa-7aaa-8bbb-00000000000a", "label": "Rina" },
    "last_result": {
      "status": "partial",
      "summary": { "created": 2, "updated": 0, "unchanged": 0, "skipped": 1, "rejected": 1 },
      "items": [
        { "repo_path": "docs/api/rate-limit.md", "target": "api/rate-limit.md", "status": "created", "reason_code": "", "reason": "", "issues": [] },
        { "repo_path": "docs/auth/login-flow.md", "target": "auth/login-flow.md", "status": "created", "reason_code": "", "reason": "", "issues": [] },
        { "repo_path": "docs/img/diagram.png", "target": "", "status": "skipped", "reason_code": "unsupported_type", "reason": "", "issues": [] },
        { "repo_path": "docs/legacy/../x.md", "target": "", "status": "rejected", "reason_code": "path_traversal", "reason": "", "issues": [] }
      ],
      "truncated": false,
      "error_code": "",
      "error_message": ""
    }
  }
}
```

### 8.3 200 — belum ada config (bukan 404)

```json
{ "kind": "api_spec", "feature_enabled": true, "has_credential": false, "credential_unreadable": false, "config": null }
```

### 8.4 200 — config ada, pemanggil (anggota B) belum punya PAT

```json
{
  "kind": "api_spec",
  "feature_enabled": true,
  "has_credential": false,
  "credential_unreadable": false,
  "config": {
    "provider": "gitlab", "host": "git.noobee.id",
    "repo_url": "https://git.noobee.id/grp/sub/repo/-/tree/main/api",
    "repo_path": "grp/sub/repo", "branch": "main", "folder": "api", "target_prefix": "",
    "last_status": "success", "last_synced_at": "2026-10-22T09:30:00Z",
    "last_synced_by": { "id": "0192f5a0-aaaa-7aaa-8bbb-00000000000a", "label": "Rina" },
    "last_result": {
      "status": "success",
      "summary": { "created": 0, "updated": 1, "unchanged": 0, "skipped": 0, "rejected": 0 },
      "items": [
        { "repo_path": "api/billing.yaml", "target": "billing", "status": "updated", "reason_code": "", "reason": "", "issues": [],
          "openapi_version": "3.0.3", "title": "Billing API", "endpoint_count": 14 }
      ],
      "truncated": false, "error_code": "", "error_message": ""
    }
  }
}
```

### 8.5 200 — PAT tidak terbaca (kunci server berganti)

Sama seperti 8.2 dengan `"has_credential": false, "credential_unreadable": true`.

### 8.6 200 — fitur nonaktif (`GIT_CREDENTIAL_KEY` kosong)

```json
{ "kind": "knowledge", "feature_enabled": false, "has_credential": false, "credential_unreadable": false, "config": null }
```

### 8.7 200 — Report dipotong (hasil sync lama dengan > 500 item)

`last_result.items` berisi 500 item, `"truncated": true`, `summary` tetap total sebenarnya (mis. `"created": 612`).

### 8.8 Error

400 kind tidak valid atau hilang:
```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "permintaan tidak valid", "details": { "fields": { "kind": "kind harus knowledge atau api_spec" } } }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```

401 tanpa login:
```json
{ "error": { "code": "UNAUTHORIZED", "message": "perlu login", "details": null }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```

404 non-anggota atau project tidak ada:
```json
{ "error": { "code": "NOT_FOUND", "message": "project tidak ditemukan", "details": null }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```

500 (DB error pada repository; pesan generik, tanpa detail internal):
```json
{ "error": { "code": "INTERNAL", "message": "terjadi kesalahan", "details": null }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```
(Kode dan pesan 500 mengikuti `respond.WriteError` yang ada; contoh di atas ilustrasi bentuk.)

Kegagalan bus get-token TIDAK pernah menghasilkan 500: respons 200 dengan `has_credential=false`.

### 8.9 Bus yang dipanggil (bentuk Go, request sebagai nilai)

```go
gettoken.Request{UserID: "0192f5a0-aaaa-7aaa-8bbb-00000000000a", Provider: "github", Host: "github.com"}
// Response yang dibaca: hanya Found, Unreadable, Disabled. Token diabaikan.
```

### 8.10 Audit

Tidak ada (read-only).
