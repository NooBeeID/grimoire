# T14 / project/git-sync / runsync — Plan Usecase

Tiket: `docs/tickets/T14-project-git-sync.md`. Dokumen induk (menang bila bertentangan): `docs/superpowers/plans/project-git-sync/overview.md` (D1-D14, bagian 5, 6.2, 6.3, 6.4, 6.6, 12).
Konteks yang dicek: `CLAUDE.md` tidak ada; `.claude/rules/` hanya `migrations.md` (tidak relevan, usecase ini tanpa migrasi); `docs/tickets/README.md` (konvensi, katalog bus/audit); `docs/superpowers/specs/2026-10-20-grimoire-design.md` (padanan `*knowledge-hub-design.md`); `DESIGN.md` untuk FE. Pola acuan kode: `project/internal/apikey/createkey/{init,handler,params,usecase,repository}.go`, `knowledge/base/access`, `knowledge/base/audit`, `infra/jobs/lock.go` (advisory lock pada koneksi dipin).
Pertanyaan terbuka tiket memakai default tertulis: (1) tanpa `previewsync`, peringatan di dialog; (2) Token Git ada di `ProfileView` (anchor `#token-git`); (3) field "Simpan di folder" (prefix) khusus knowledge.

Catatan nama field: tiket menulis `targetPrefix`, overview (6.8) menetapkan snake_case, jadi body memakai `target_prefix`.

---

## 1. Metadata

| Atribut | Nilai |
|---|---|
| Ticket | T14 |
| Scope | fullstack (backend usecase ini + FE `runsync`) |
| Domain / Module / Feature | project / git-sync (`gitsync`) / `runsync` |
| Folder backend | `backend/internal/domain/project/internal/gitsync/runsync` |
| Folder FE | `frontend/src/domain/project/git-sync/runsync` |
| Endpoint | `/api/projects/{projectID}/git-sync/run` |
| Method | `POST` |
| Version | `/api` tanpa prefix versi; `api/modules/project/git-sync.yml` `info.version` 0.1.0 |
| Middleware | `RequireAuth` + CSRF (global di `api.Use`) + `BodyLimit(16<<10)` + `ExtendTimeout(cfg.GitSyncTimeout)` |
| Siapa | `member`, `owner`, `admin` (super_admin non-anggota ikut). `none` -> 404. Tanpa login -> 401 |
| Bus dipanggil | `project.management.check-access` (via `base/access`), `core.gitcredential.get-token`, `project.knowledge.upsert-files` atau `project.apispec.upsert-specs`, `core.audit.record` (via `base/audit`, fail-open) |
| Bus disediakan | tidak ada |
| Kriteria tiket | 3-12, 13, 14 |
| Ketergantungan | FOUNDATION selesai (langkah 0 overview). Tes unit paralel dengan usecase lain memakai fake bus. `runsync_integration_test.go` butuh `bustoken`, `busupsertfiles`, `busupsertspecs` ter-merge |

---

## 2. Summary Plan

POST yang menarik file dari repo GitHub/GitLab memakai PAT milik user pemanggil, lalu menulis ke knowledge atau API spec lewat bus (jalur upload yang sama). Seluruh rule ada di `gitsync/base` (parsing URL, seleksi, batas, laporan, status). Usecase hanya mengorkestrasi urutan D5:

1. `access.Require` -> `Actor` (404/401 tanpa tulis).
2. `ParseKind`, `ParseRepoURL(url, branch, folder, policy)`, `NormalizeTargetPrefix` (knowledge saja) -> 400 `INVALID_REPO_URL`/`INVALID_ARGUMENT`, 422 `HOST_NOT_ALLOWED`. Nol tulis.
3. Bus `get-token{UserID: actor.ID, Provider, Host}`: `Disabled` -> 503, `!Found` -> 422 `CREDENTIAL_MISSING`, `Unreadable` -> 422 `CREDENTIAL_UNREADABLE`. Nol tulis, config milik A tidak tertimpa oleh B.
4. `repo.TryLock(projectID, kind)`; gagal -> 409 `SYNC_IN_PROGRESS`; `defer unlock`.
5. `repo.UpsertConfig` (tx singkat; kolom `last_*` tidak disentuh).
6. Fase baca (tanpa tulis ke knowledge/apispec): `ListTree` -> `SelectFiles` -> `CheckBatchLimits` -> fetch blob konkurensi 4 dengan pengecekan kumulatif. Kegagalan apa pun di sini -> `SaveResult(failed, ErrorCode)` + audit `git_sync.run` (status error) lalu error HTTP; belum ada file yang ditulis.
7. Fase tulis: satu panggilan bus (`upsert-files` atau `upsert-specs`). `CodeInvalid` -> 400; error lain -> 500; keduanya `SaveResult(failed)` + audit.
8. Gabung item skipped (seleksi) + hasil bus -> `base.Report`, `DeriveStatus`, `SaveResult(success|partial, last_synced_at=now, ...)` dengan `CapForStorage(500)`, audit `git_sync.run`, 200 `{report, config}`.

Keputusan implementasi yang dikunci di plan ini:

- **Konteks untuk langkah akhir.** Bila request habis waktu atau klien putus, `ctx` sudah mati padahal `SaveResult`, audit, dan `unlock` tetap harus jalan. Semuanya memakai `finCtx := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)` (nilai ctx seperti identity/trace_id tetap terbawa, deadline dilepas).
- **Fetch dulu semua, baru tulis (D3).** Semua konten ada di memori sebelum bus upsert dipanggil; kegagalan provider di tengah fetch tidak meninggalkan project setengah tertimpa.
- **File besar.** Entri dengan `Size >= 0 && Size > Limits.MaxFileBytes` tidak di-fetch; langsung `Item{TooLarge:true}`. Size tidak diketahui (GitLab, -1) di-fetch dengan `maxBytes = MaxFileBytes`; `FetchBlob` mengembalikan `tooLarge=true`. Item TooLarge tidak dihitung ke total kumulatif; provider bus yang menolaknya (`too_large` / `FILE_TOO_LARGE` sesuai kosakata module).
- **Tanpa kandidat.** Bila setelah seleksi tidak ada kandidat (semua skipped, atau folder root kosong), bus tidak dipanggil; laporan hanya berisi skipped, status `success`.
- **`target_prefix` untuk api_spec diabaikan** sepenuhnya (tidak divalidasi, config menyimpan `""`).
- **Retry-After.** Error 429 membawa `Details{"retry_after_seconds": n}` (hanya bila diketahui); handler menyalinnya ke header `Retry-After`. Tidak ada tipe error baru di lib.
- **Pool DB.** Lock memegang satu koneksi selama sync (sampai 120 s). Saat implementasi cek `DB_MAX_OPEN_CONNS` default; bila terlalu kecil untuk beberapa sync serentak plus request lain, catat di `docs/configuration.md`. Bila `db.DB` belum mengekspos `Connx`, tambahkan passthrough tipis di `infra/db` (satu method, tanpa logika).
- **Log.** Satu baris `info` per sync selesai: `project_id`, `kind`, `provider`, `host`, `status`, hitungan, durasi. Tidak ada URL penuh, token, atau isi file. Error provider tidak pernah dilog dengan body.

---

## 3. Plan (langkah berurutan)

Prasyarat: FOUNDATION (overview langkah 0) selesai: `secret`, `gitprovider`, `safehttp`, `ExtendTimeout`, config env, konstanta/kontrak bus, `gitsync/base/**` (kind, limits, repourl, prefix, select, limits_check, report, config, dto, actor, errors, access, audit, store), refaktor D1, migrasi `git_sync_configs`. Plan ini tidak mengubah file di luar folder `runsync` kecuali dua baris wiring (langkah 11) dan OpenAPI (langkah 12).

### Backend

1. **`runsync/params.go`**
   ```go
   // RunRequest adalah body POST /projects/{projectID}/git-sync/run.
   type RunRequest struct {
   	Kind         string `json:"kind"`
   	URL          string `json:"url"`
   	Branch       string `json:"branch"`
   	Folder       string `json:"folder"`
   	TargetPrefix string `json:"target_prefix"`
   }
   // RunResponse dibentuk dari base.ReportDTO dan base.ConfigDTO.
   type RunResponse struct {
   	Report base.ReportDTO `json:"report"`
   	Config base.ConfigDTO `json:"config"`
   }
   ```
   Decode dengan `respond.DecodeJSON` (menolak field tak dikenal bila helper itu sudah begitu). Field kosong diperlakukan "tidak diisi". `Config.LastResult` di respons selalu `nil` (isinya sama dengan `report`).

2. **`runsync/repository.go`** (satu-satunya SQL). Struct `Repo{db *db.DB, s *store.Store}`; konstruktor `newRepo(d, s)`.
   ```go
   type Repository interface {
   	// TryLock: ok=false, err=nil bila lock dipegang sync lain. unlock dipanggil tepat sekali bila ok=true.
   	TryLock(ctx context.Context, projectID string, kind base.Kind) (unlock func(context.Context) error, ok bool, err error)
   	UpsertConfig(ctx context.Context, c base.Config) (base.Config, error)
   	SaveResult(ctx context.Context, p SaveResultParams) (base.Config, error)
   }
   type SaveResultParams struct {
   	ProjectID string; Kind base.Kind
   	Status    string        // success|partial|failed
   	SyncedAt  time.Time     // dari Clock
   	ByID, ByLabel string
   	Result    base.Report   // sudah CapForStorage
   }
   ```
   - `TryLock`: `Connx(ctx)` dengan timeout 5 s; `SELECT pg_try_advisory_lock(hashtext('grimoire.gitsync.' || $1 || '.' || $2))` (args `projectID`, `string(kind)`). Query error -> `discard(conn)` + return err. `got=false` -> `conn.Close()`, `ok=false`. Sukses -> closure `unlock` menjalankan `pg_advisory_unlock(...)` dengan key yang sama; gagal atau `released=false` -> `discard(conn)` (`conn.Raw` mengembalikan `driver.ErrBadConn`, lalu `Close`) dan return error; sukses -> `conn.Close()`. Salin helper `discard` ke file ini (jangan import `infra/jobs`; infra boleh dipakai tapi `jobs` bukan API publik untuk ini).
   - `UpsertConfig`: dalam `db.BeginTx`:
     ```sql
     INSERT INTO git_sync_configs (project_id, kind, provider, host, repo_url, repo_path, branch, folder, target_prefix)
     VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
     ON CONFLICT (project_id, kind) DO UPDATE SET
       provider=EXCLUDED.provider, host=EXCLUDED.host, repo_url=EXCLUDED.repo_url, repo_path=EXCLUDED.repo_path,
       branch=EXCLUDED.branch, folder=EXCLUDED.folder, target_prefix=EXCLUDED.target_prefix, updated_at=now()
     RETURNING <store.ConfigColumns>
     ```
     FK violation (project hilang) -> `base.ErrProjectNotFound`.
   - `SaveResult`: `UPDATE git_sync_configs SET last_status=$3, last_synced_at=$4, last_synced_by=$5::uuid, last_synced_by_label=$6, last_result=$7::jsonb, updated_at=now() WHERE project_id=$1 AND kind=$2 RETURNING <cols>`. `last_result` = `json.Marshal(base.Report)` lewat helper `store` (tanpa token karena Report memang tidak punya field token). 0 baris -> `base.ErrProjectNotFound`.
   - Semua query hanya memakai `project_id` dari path yang sudah lolos access (rule 6.10); tidak ada SELECT lintas module.

3. **`runsync/usecase.go`** — orkestrasi.
   ```go
   type Accessor interface{ Require(ctx context.Context, projectID string) (base.Actor, error) }
   type Auditor interface{ Record(ctx context.Context, ev audit.Event) }
   type Clock func() time.Time
   type GitClient interface { // diimplementasi *gitprovider.Client
   	Policy() gitprovider.HostPolicy
   	ListTree(ctx context.Context, repo gitprovider.Repo, folder string, token secret.String) ([]gitprovider.TreeEntry, error)
   	FetchBlob(ctx context.Context, repo gitprovider.Repo, sha string, maxBytes int64, token secret.String) ([]byte, bool, error)
   }
   type Usecase struct {
   	access Accessor; repo Repository; git GitClient; bus *bus.Registry
   	audit Auditor; clock Clock; cfg config.Config; log *slog.Logger
   }
   func (u *Usecase) Execute(ctx context.Context, projectID string, in RunRequest, meta base.RequestMeta) (RunResponse, error)
   ```
   Alur `Execute` (setiap nomor = satu fungsi privat kecil agar terbaca; tanpa memecah usecase menjadi package lain):

   a. `actor, err := u.access.Require(ctx, projectID)`; error dikembalikan apa adanya.
   b. `kind, err := base.ParseKind(in.Kind)`. `ref, err := base.ParseRepoURL(in.URL, in.Branch, in.Folder, allowedFn)` dengan `allowedFn := func(p, h string) bool { return u.git.Policy().Allowed(gitprovider.Provider(p), h) }`. `prefix := ""`; bila `kind==KindKnowledge`, `prefix, err = base.NormalizeTargetPrefix(in.TargetPrefix)`. Error -> kembalikan (400/422), tanpa tulis.
   c. `tok, err := bus.CallTyped[gettoken.Response](ctx, u.bus, constants.BUS_CORE_GITCREDENTIAL_GET_TOKEN, gettoken.Request{UserID: actor.ID, Provider: ref.Provider, Host: ref.Host})`. Error bus -> bungkus `fmt.Errorf("runsync: get-token: %w", err)` (jadi 500; pesan tidak memuat token karena Request tidak membawa token). Urutan cek: `Disabled` -> `errs.CodeGitFeatureDisabled`; `!Found` -> `base.ErrCredentialMissing(provider, host)`; `Unreadable` -> `base.ErrCredentialUnreadable(provider, host)`. Keduanya dengan `details{provider, host, settings_path:"/settings#token-git"}` (sentinel dibangun lewat constructor di `base/errors.go`).
   d. `unlock, ok, err := u.repo.TryLock(ctx, projectID, kind)`; `err` -> 500; `!ok` -> `base.ErrSyncInProgress(kind)`. `defer func(){ fctx := fin(ctx); if e := unlock(fctx); e != nil { u.log.WarnContext(fctx, "runsync: unlock gagal", "project_id", projectID, "kind", kind, "error", e) } }()`.
   e. `cfg0 := base.NewConfig(projectID, kind, ref, prefix)`; `saved, err := u.repo.UpsertConfig(ctx, cfg0)`; error -> 500 (belum ada hasil yang perlu dicatat; tidak ada baris config kalau upsert gagal).
   f. **Fase baca** `items, cands, skipped, err := u.pull(ctx, kind, ref, token)` (lihat langkah 4). Bila `err != nil`: `u.fail(ctx, saved, actor, meta, err)` yang (1) memetakan error ke `*errs.Error` (`mapError`), (2) `SaveResult(failed)` dengan `FailedReport(code, message)` (tanpa item) memakai `fin(ctx)`, (3) audit `git_sync.run` status error, (4) mengembalikan `*errs.Error` ke handler.
   g. **Fase tulis** `res, err := u.write(ctx, projectID, kind, prefix, actor, meta, items)`; `err` -> `u.fail` (CodeInvalid dipetakan 400 tanpa mengubah kodenya; selain itu `errs.CodeInternal` dengan `ErrorCode:"INTERNAL"`).
   h. `report := u.buildReport(kind, skipped, cands, res)` (menggabungkan item skipped + ItemResult via `Index` ke `cands[Index].RepoPath`), `report.DeriveStatus()`, `cfg, err := u.repo.SaveResult(fin(ctx), SaveResultParams{... Result: report.CapForStorage(500)})`. Kegagalan `SaveResult` setelah tulis sukses: log `error`, tetap kembalikan 200 dengan config hasil upsert (file sudah masuk; laporan lebih penting dari metadata).
   i. `u.audit.Record(fin(ctx), ...)`; return `RunResponse{Report: report.ToDTO(), Config: cfg.ToDTO() dengan LastResult=nil}`.

4. **`pull` (fase baca, di `usecase.go`)**:
   1. `entries, err := u.git.ListTree(ctx, gitprovider.Repo{Provider, Host, Path, Branch}, ref.Folder, token)`; konversi ke `[]base.TreeEntry`.
   2. `cands, skipped, err := base.SelectFiles(entries, ref.Folder, kind)`; `errors.Is(err, base.ErrFolderNotFound)` -> `GIT_NOT_FOUND` "folder tidak ditemukan di branch ini" (details `{what:"folder", folder}`).
   3. `lim := base.LimitsFor(kind, u.cfg)`; `base.CheckBatchLimits(cands, lim)` -> `SYNC_LIMIT_EXCEEDED`.
   4. `items, err := u.fetchAll(ctx, repo, cands, lim, token)`:
      - Semaphore channel `make(chan struct{}, 4)`, `sync.WaitGroup`, `ctx2, cancel := context.WithCancel(ctx)`, slice hasil per indeks (tanpa mutex karena indeks unik), `sync.Once` untuk error pertama, `atomic.Int64 total`.
      - Tiap kandidat: bila `Size > lim.MaxFileBytes` (dan `Size >= 0`) -> `TooLarge` tanpa fetch. Selain itu `FetchBlob(ctx2, repo, SHA, lim.MaxFileBytes, token)`; error pertama disimpan lalu `cancel()`. Sukses non-TooLarge: `n := total.Add(len(data))`; `base.CheckFetchedTotal(n, lim)` gagal -> simpan error `SYNC_LIMIT_EXCEEDED`, `cancel()`.
      - Setelah `wg.Wait()`: bila error pertama ada -> kembalikan; bila `ctx.Err() != nil` -> kembalikan `ctx.Err()` (dipetakan 504/dibatalkan). Item diurutkan stabil sesuai urutan `cands` (Index = posisi di `cands`).
      - Tidak ada goroutine bocor: semua worker selesai sebelum fungsi return; `defer cancel()`.
      - Tidak ada dependensi baru (`errgroup` x/sync tidak dipakai).
5. **`write` (fase tulis)**:
   - knowledge: `upsertfiles.Request{ProjectID, Actor{UserID, Role, Label}, TargetPrefix: prefix, Source: upsertfiles.SourceGitSync, Meta{IP, UserAgent}, Items: [{Index, Path: cand.RelPath, Content, TooLarge}]}` via `bus.CallTyped[upsertfiles.Response](ctx, bus, constants.BUS_PROJECT_KNOWLEDGE_UPSERT_FILES, req)` (request NILAI, bukan pointer).
   - api_spec: `upsertspecs.Request{..., Items: [{Index, SourcePath: cand.RepoPath, Content, TooLarge}]}` ke `BUS_PROJECT_APISPEC_UPSERT_SPECS`.
   - Bus ini bisa lama (banyak file): pakai `ctx` request (ber-deadline `GIT_SYNC_TIMEOUT`). Bila deadline habis di tengah tulis, knowledge (satu tx) rollback utuh; apispec (tx per file) sebagian masuk. `SaveResult(failed, TIMEOUT)` menjelaskan; ini perilaku yang sama dengan upload (D2). Dicatat di dokumentasi OpenAPI deskripsi 504.
6. **`buildReport`**: untuk tiap `ItemResult` -> `base.ReportItem{RepoPath: cands[Index].RepoPath, Target: res.Path (knowledge) | res.Name (api_spec), Status, ReasonCode, Reason, Issues}`; knowledge: `ReasonCode = Reason` dari bus, `Reason` teks kosong (FE i18n by kode); api_spec: `ReasonCode`, `Reason`, `Issues`, `OpenAPIVersion`, `Title`, `EndpointCount` (field tambahan di `ReportItem` DTO, `omitempty`; bila foundation belum memuatnya, tambahkan di `base/report.go` + `dto.go` sebagai perubahan aditif). Item skipped dari seleksi: `Status:"skipped"`, `ReasonCode` dari `SelectFiles`, `Target:""`. `NewReport(items)` mengurutkan dan menghitung Summary dari semua item.
7. **`mapError(err) *errs.Error`** (fungsi murni di `usecase.go`, tabel dites):

   | Sumber | Hasil |
   |---|---|
   | `*errs.Error` (sentinel base, SYNC_LIMIT_EXCEEDED, dsb.) | apa adanya |
   | `gitprovider.ErrUnauthorized` | 422 `GIT_TOKEN_REJECTED` "token Git ditolak provider; simpan ulang token" |
   | `gitprovider.ErrForbidden` | 422 `GIT_ACCESS_DENIED` "token tidak punya akses ke repo ini" |
   | `gitprovider.ErrNotFound` | 404 `GIT_NOT_FOUND` "repo/branch tidak ditemukan atau token tidak punya akses" details `{what:"repo"}` |
   | `*gitprovider.RateLimitedError` / `ErrRateLimited` | 429 `RATE_LIMITED` details `{retry_after_seconds}` bila `RetryAfter>0` |
   | `gitprovider.ErrTreeTooLarge` | 422 `GIT_TREE_TOO_LARGE` |
   | `gitprovider.ErrHostNotAllowed` | 422 `HOST_NOT_ALLOWED` |
   | `gitprovider.ErrUpstream` | 502 `GIT_UPSTREAM_ERROR` |
   | `context.DeadlineExceeded` | 504 `TIMEOUT` "sync melebihi batas waktu" |
   | `context.Canceled` | `errs.CodeInternal`-tidak-dipakai; dikembalikan sebagai `context.Canceled` (klien sudah pergi, handler tidak menulis body). `SaveResult` tetap mencatat `failed` dengan `error_code:"CANCELED"` |
   | lainnya (DB, bus non-invalid) | 500 `INTERNAL`, pesan generik; `error_code:"INTERNAL"` |

   Pesan tidak pernah memuat token, URL berquery, atau body provider (D6).

8. **`runsync/handler.go`**
   ```go
   func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
   	w.Header().Set("Cache-Control", "no-store")
   	var in RunRequest
   	if err := respond.DecodeJSON(r, &in); err != nil { respond.WriteError(w, r, err); return }
   	meta := base.NewRequestMeta(middleware.KeyByIP(r), r.UserAgent())
   	res, err := h.uc.Execute(r.Context(), chi.URLParam(r, "projectID"), in, meta)
   	if err != nil {
   		setRetryAfter(w, err) // errs.Error.Details map[string]any -> retry_after_seconds -> header Retry-After (detik)
   		respond.WriteError(w, r, err); return
   	}
   	respond.WriteJSON(w, http.StatusOK, res)
   }
   ```
   `r.Context()` sudah memuat Session (dari middleware setelah Timeout) dan deadline `GitSyncTimeout` dari `ExtendTimeout` (overview 5.5).

9. **`runsync/init.go`**
   ```go
   func Init(d app.Deps, git GitClient) {
   	uc := NewUsecase(access.New(d.Bus), newRepo(d.DB, store.New(d.DB)), git, d.Bus,
   		audit.New(d.Bus, d.Log), time.Now, d.Config, d.Log)
   	d.API.With(
   		middleware.RequireAuth(),
   		middleware.BodyLimit(16<<10),
   		middleware.ExtendTimeout(d.Config.GitSyncTimeout),
   	).Post("/projects/{projectID}/git-sync/run", newHandler(uc).ServeHTTP)
   }
   ```
   Urutan middleware: `RequireAuth` lalu `BodyLimit` lalu `ExtendTimeout`. (Signature `Init(d, git)` mengikuti overview 6.2: `runsync.Init(d, deps)` dengan `deps` memuat `Git`.)

10. **Tes** (bagian 6) ditulis berdampingan (TDD: tulis tes fake bus/gitClient lebih dulu untuk `mapError`, urutan D5, dan canary).

11. **Wiring**: satu baris `runsync.Init(d, deps)` di `gitsync/base.go` (sudah ada di rencana foundation) dan satu entri di `gitsync/routes_test.go` (POST `/api/projects/{id}/git-sync/run` memakai `RequireAuth`, `BodyLimit`, `ExtendTimeout`; 401 tanpa sesi; 403 `CSRF_REQUIRED` tanpa header CSRF).

12. **OpenAPI** `api/modules/project/git-sync.yml`: lengkapi operasi `runSync` (request `GitSyncRunRequest`, respons 200 `GitSyncRunResponse` `{report, config}`, error 400/401/403(CSRF)/404/409/422/429(+header `Retry-After`)/502/503/504 dengan contoh bagian 8). Jalankan `make api-bundle`, commit `api/openapi.yml`, `make api-check`.

13. **Katalog audit/bus** sudah di foundation; tidak ada perubahan `docs/tickets/README.md` dari usecase ini.

### Frontend (`frontend/src/domain/project/git-sync/runsync/`)

Mengikuti struktur domain FE (`components/ services/ stores/ views/ types.ts index.ts`), tanpa import silang ke `domain/core/git-credential` (depcruise). Status PAT berasal dari `getconfig` (`has_credential`, `credential_unreadable`, `feature_enabled`) yang di-prop-kan ke dialog oleh komposisi di `app/`.

1. `types.ts`: Zod `SyncReportItemSchema` (`repo_path, target, status` enum `created|updated|unchanged|skipped|rejected`, `reason_code, reason, issues[{code,message,pointer,line}]`, opsional `openapi_version, title, endpoint_count`), `SyncSummarySchema`, `SyncReportSchema` (`status` enum `success|partial|failed`, `summary`, `items`, `truncated`, `error_code`, `error_message`), `GitSyncConfigSchema` (field snake_case, `last_synced_by:{id,label}|null`, `last_result: SyncReport|null`), `RunSyncResponseSchema = {report, config}`, `RunSyncInput {kind, url, branch, folder, target_prefix?}`. Skema error envelope memakai util `shared/services/http.ts` yang ada (`ApiError` dengan `code`, `message`, `details`); `details.settings_path` dibaca untuk `CREDENTIAL_*`.
2. `services/runsync.service.ts`: `runSync(projectID, input): Promise<RunSyncResponse>` -> `POST /api/projects/{id}/git-sync/run` lewat `shared/services/http.ts` (CSRF header otomatis), `AbortSignal` didukung (tombol Batal menutup dialog; server tetap menyelesaikan sync dan mencatatnya), timeout klien 130 s (> `GIT_SYNC_TIMEOUT` default 120 s), validasi Zod respons; pemetaan error -> `SyncError{code, message, retryAfter?, settingsPath?}`. Untuk `target_prefix` kosong pada api_spec, field tidak dikirim.
3. `stores/runsync.store.ts` (Svelte store): state `idle | running | done | error`; aksi `start(projectID, kind, input)`, `reset()`, derived `canSubmit` (url tidak kosong, tidak running, `has_credential` dan `feature_enabled`). Guard dobel-klik: `start` saat `running` diabaikan. Tidak ada token di store (FE tidak pernah memegang token).
4. `components/`:
   - `RepoOriginChip.svelte`: chip mono `github.com/noobee/grimoire-docs@main:docs/` dari `host, repo_path, branch, folder` (gaya chip agent).
   - `ConfigSummary.svelte`: "Terakhir: {relatif} oleh {label} · {StatusBadge}" + ringkasan angka + `RepoOriginChip`; kosong -> "Belum pernah sync".
   - `StatusBadge.svelte`: pil `success`=positive, `partial`=warning, `failed`=negative; status item: `created`=positive, `updated`=neutral, `unchanged`=neutral redup, `skipped`=neutral, `rejected`=negative.
   - `SyncReportTable.svelte`: panel tabel kolom Path repo (mono), Tujuan (mono), Status (`StatusBadge`), Alasan (i18n dari `reason_code`: knowledge memakai kunci reason knowledge yang sudah ada, api_spec memakai kunci reason apispec; fallback ke `reason`, lalu kode mentah) + tombol salin alasan (`CopyField`/util salin yang ada). Baris `rejected` dengan `issues` dapat diperluas (pointer mono, line). Daftar panjang: tinggi maks dengan scroll internal, header sticky.
5. `views/GitSyncDialog.svelte` (prop `kind`, `projectID`, `config`, `featureEnabled`, `hasCredential`, `credentialUnreadable`, `onSynced`, `onClose`): memakai `shared/components/Dialog`, `Input`, `Button`, `Spinner`. Dua state tampilan (lihat Visual). Setelah `done` dengan `status != failed`, `onSynced()` dipanggil sekali (memuat ulang pohon/daftar) dan dialog tetap terbuka menampilkan hasil sampai ditutup user.
6. Komposisi tombol: `app/views/workspace/knowledge/KnowledgeTab.svelte` dan `KnowledgeDialogs.svelte` (tombol sekunder "Sync dari Git" di `PageHeader`, dialog dirender di `KnowledgeDialogs`, `onSynced` = muat ulang `listtree`), serta `app/views/workspace/ApiSpecsTab.svelte` (`onSynced` = muat ulang `listspecs`). Aksi utama tetap Upload. `getconfig` dipanggil saat dialog dibuka (domain `git-sync/getconfig`, rencana terpisah) untuk mengisi form dan status PAT.
7. `index.ts` mengekspor `GitSyncDialog` dan tipe publik saja.
8. i18n `frontend/src/shared/i18n/{id,en}.json`, kunci `gitSync.*`: `button`, `dialog.title.knowledge|apiSpec`, label field (`url`, `branch`, `folder`, `targetPrefix`), `targetPrefixPlaceholder` ("/ (root)"), `overwriteWarning` ("File bernama sama akan ditimpa. File yang tidak ada di repo tidak dihapus."), `run`, `rerun`, `running` ("Menarik dari {provider}…"), `lastSync`, `neverSynced`, `status.*`, `itemStatus.*`, `summary.*`, `errors.<KODE>` untuk seluruh kode di overview 6.6 yang relevan (`CREDENTIAL_MISSING`, `CREDENTIAL_UNREADABLE`, `INVALID_REPO_URL`, `HOST_NOT_ALLOWED`, `GIT_TOKEN_REJECTED`, `GIT_ACCESS_DENIED`, `GIT_NOT_FOUND`, `SYNC_LIMIT_EXCEEDED`, `SYNC_IN_PROGRESS`, `GIT_TREE_TOO_LARGE`, `RATE_LIMITED`, `GIT_UPSTREAM_ERROR`, `GIT_FEATURE_DISABLED`, `TIMEOUT`), `setupToken` ("Atur Token Git"), `copyReason`. `i18n-keys.test.ts` harus hijau (id dan en sama kuncinya).

### Visual (DESIGN.md, framed-sheet; dikerjakan sesuai `design-taste-frontend/references/app-screens.md`)

- **Pertanyaan layar.** "Apa yang akan ditarik dari repo ini, dan setelah dijalankan file mana yang berubah?" Satu dialog, dua state, tanpa halaman baru.
- **Pola DESIGN.md.** Dialog `shadow-pop` lebar maks 720, header title + chip asal-repo; badan form dengan baris label atas; hasil memakai panel tabel (radius 8, `shadow-panel`, tinggi baris 48, kontrol 32).
- **State Form.** URL (mono, wajib), Branch, Folder (terisi otomatis dari URL; user dapat mengubah), "Simpan di folder" (hanya knowledge, placeholder "/ (root)"), blok `text-warning` berisi peringatan menimpa, `ConfigSummary` bila ada config. Tombol sekunder "Sync ulang" (menjalankan config tersimpan tanpa edit) dan tombol utama "Sync". Tanpa PAT atau fitur nonaktif: tombol Sync dan Sync ulang nonaktif, kotak info dengan tautan "Atur Token Git" ke `/settings#token-git`; `credential_unreadable` memakai teks "Token perlu disimpan ulang".
- **State Berjalan.** Tombol utama berganti teks "Menarik dari GitHub…" + `Spinner` kecil, field dinonaktifkan; tombol Batal menutup dialog.
- **State Hasil.** Baris ringkasan angka `tabular-nums` (dibuat, diperbarui, tidak berubah, dilewati, ditolak) + `StatusBadge` keseluruhan, lalu `SyncReportTable`. Status gagal total: banner `pill-negative` berisi pesan error terjemahan, tanpa tabel.
- **Elemen khas.** `RepoOriginChip` mono; pil status; baris ditolak dapat diperluas untuk issues (pointer mono).
- **Token.** `bg-surface`, `border-border`, `shadow-pop`, `shadow-panel`, `text-muted`, `pill-{positive,negative,warning,neutral}`, `text-warning`; `accent-ink` hanya cincin fokus. Radius 6/8/16. Tanpa warna baru, tanpa gradien, tanpa emoji.
- **Data contoh.** Repo `noobee/grimoire-docs`, branch `main`, folder `docs`; file `auth/login-flow.md`, `api/rate-limit.md`, `img/diagram.png` (dilewati); spec `billing.yaml` (14 endpoint), `payments.json` (ditolak, pointer `#/paths/~1charges/get`); GitLab `git.noobee.id`; pengguna "Rina".

---

## 4. Migration

Tidak ada. Tabel `git_sync_configs` dibuat di FOUNDATION (`backend/migrations/project/<VERSION>_create_git_sync_configs.sql`, overview 7.2). Usecase ini hanya memakai `UNIQUE (project_id, kind)` untuk `ON CONFLICT` dan constraint `target_prefix` kosong untuk `api_spec`.

---

## 5. Changes (per file/folder)

| File / folder | Jenis |
|---|---|
| `backend/internal/domain/project/internal/gitsync/runsync/init.go` | baru |
| `.../runsync/handler.go` | baru |
| `.../runsync/params.go` | baru |
| `.../runsync/usecase.go` | baru (alur D5, `pull`, `fetchAll`, `write`, `buildReport`, `mapError`, `fin`) |
| `.../runsync/repository.go` | baru (`TryLock`, `UpsertConfig`, `SaveResult`) |
| `.../runsync/handler_test.go` | baru |
| `.../runsync/usecase_test.go` | baru |
| `.../runsync/repository_integration_test.go` | baru (tag `integration`) |
| `.../runsync/runsync_integration_test.go` | baru (tag `integration`) |
| `.../gitsync/base.go` | ubah: satu baris `runsync.Init(d, deps)` |
| `.../gitsync/routes_test.go` | ubah: satu entri route |
| `.../gitsync/base/report.go`, `base/dto.go` | ubah aditif bila belum ada: `OpenAPIVersion`, `Title`, `EndpointCount` (omitempty) pada `ReportItem` |
| `backend/internal/infra/db/` | ubah hanya bila `Connx` belum ada: passthrough tipis |
| `api/modules/project/git-sync.yml`, `api/openapi.yml` | ubah: operasi `runSync` |
| `frontend/src/domain/project/git-sync/runsync/{types.ts,index.ts}` | baru |
| `.../runsync/services/runsync.service.ts` (+ `.test.ts`) | baru |
| `.../runsync/stores/runsync.store.ts` (+ `.test.ts`) | baru |
| `.../runsync/components/{RepoOriginChip,ConfigSummary,StatusBadge,SyncReportTable}.svelte` (+ `components.test.ts`) | baru |
| `.../runsync/views/GitSyncDialog.svelte` (+ `.test.ts`) | baru |
| `frontend/src/app/views/workspace/knowledge/{KnowledgeTab,KnowledgeDialogs}.svelte` (+ `KnowledgeTab.test.ts`) | ubah: tombol + dialog |
| `frontend/src/app/views/workspace/ApiSpecsTab.svelte` (+ `ApiSpecsTab.test.ts`) | ubah: tombol + dialog |
| `frontend/src/shared/i18n/{id,en}.json` | ubah: `gitSync.*` |
| `frontend/e2e/git-sync.spec.ts`, `frontend/e2e/fixtures/fake-git-server.ts`, `frontend/e2e/helpers/gitsync.ts` | baru |

Tidak diubah: `app.Deps`, `cmd/**`, `gitcredential/**`, `knowledge/**`, `apispec/**` (hanya diakses via bus).

---

## 6. Test Scenario

Semua unit memakai `bus.NewRegistry` dengan handler palsu (mencatat request), `fakeGit` (`GitClient`, mencatat panggilan `token`/`repo`/`sha`, bisa mengembalikan error per metode), `fakeRepo` (mencatat urutan: `lock`, `upsert`, `save`, `unlock`), `fakeAudit`, `fakeAccess`. Tidak ada jaringan nyata.

### 6.1 `handler_test.go`

Tabel pemetaan error -> status/kode (usecase palsu mengembalikan error), memanggil `gitsync.registerCodes()` bukan `RegisterCodeStatus` langsung:

| Error usecase | Status | Kode |
|---|---|---|
| tanpa identity (ctx) | 401 | `UNAUTHORIZED` |
| `ErrProjectNotFound` (non-anggota) | 404 | `NOT_FOUND` |
| `ErrInvalidKind`, body JSON rusak, field tak dikenal | 400 | `INVALID_ARGUMENT` |
| `ErrInvalidRepoURL` | 400 | `INVALID_REPO_URL` |
| `ErrInvalidTargetPrefix` | 400 | `INVALID_ARGUMENT` |
| `ErrHostNotAllowed` | 422 | `HOST_NOT_ALLOWED` |
| `CREDENTIAL_MISSING` / `CREDENTIAL_UNREADABLE` | 422 | kode sama; `details.settings_path == "/settings#token-git"` |
| `GIT_TOKEN_REJECTED`, `GIT_ACCESS_DENIED`, `GIT_TREE_TOO_LARGE`, `SYNC_LIMIT_EXCEEDED` | 422 | kode sama |
| `GIT_NOT_FOUND` | 404 | `GIT_NOT_FOUND` |
| `SYNC_IN_PROGRESS` | 409 | kode sama |
| `RATE_LIMITED` dengan `retry_after_seconds=60` | 429 | header `Retry-After: 60`; tanpa nilai -> tanpa header |
| `GIT_UPSTREAM_ERROR` | 502 | kode sama |
| `GIT_FEATURE_DISABLED` | 503 | kode sama |
| `TIMEOUT` | 504 | kode sama |
| error tak dikenal | 500 | `INTERNAL`, pesan generik |

Tambahan: sukses -> 200 JSON `{report, config}` dengan snake_case dan `Cache-Control: no-store`; body > 16 KiB -> 413; semua body error berformat `{"error":{code,message,details},"trace_id"}`; handler meneruskan `projectID` dari URL dan `meta` IP/UA.

### 6.2 `usecase_test.go`

Urutan dan tanpa-tulis:
- Urutan D5: `Require` -> parse -> `get-token` -> `lock` -> `upsert` -> `ListTree` -> fetch -> bus upsert -> `save` -> audit -> `unlock` (dicek dari log panggilan).
- `Require` gagal (404/401): nol panggilan lain.
- `kind` invalid, URL invalid (http, userinfo, tanpa branch), `HOST_NOT_ALLOWED` (host GitLab tidak di allowlist), prefix `a/../b`: nol panggilan bus/lock/upsert/save.
- Token: `Disabled` -> 503; `!Found` -> `CREDENTIAL_MISSING` dengan details `{provider, host, settings_path}`; `Unreadable` -> `CREDENTIAL_UNREADABLE`. Ketiganya: nol `lock`/`upsert`/`save`/`ListTree`.
- **User B tanpa PAT walau config dari A**: fake repo berisi config A; B (identity lain) -> `CREDENTIAL_MISSING`; `upsert` dan `save` tidak dipanggil; config A utuh.
- **Token yang dipakai = token UserID identity**: fake get-token menjawab token berbeda per `UserID`; assert `Request.UserID == actor.ID` dan `fakeGit` menerima token milik actor itu, bukan `last_synced_by` dari config.
- Lock: `TryLock ok=false` -> 409 `SYNC_IN_PROGRESS`, nol `upsert`/`ListTree`; unlock dipanggil tepat sekali pada sukses dan pada semua jalur gagal setelah lock; unlock gagal hanya di-log, tidak mengubah respons.

Sukses:
- Knowledge: tree `docs/auth/login-flow.md`, `docs/api/rate-limit.md`, `docs/img/diagram.png`, folder `docs`, prefix `guides`: bus menerima `Items.Path` = `auth/login-flow.md` / `api/rate-limit.md` (struktur dipertahankan, tanpa prefix folder git), `TargetPrefix="guides"`, `Source="git_sync"`, `Actor` dari identity, `Meta` IP/UA; png menjadi skipped `unsupported_type` tanpa fetch; report: 2 created + 1 skipped, status `success`; `SaveResult(success)` dengan `last_synced_by` = actor.ID dan label; config tersimpan memuat `target_prefix="guides"`.
- api_spec: 3 spec (valid, `$ref` eksternal, `.txt`) -> bus `upsert-specs` menerima `SourcePath` = path repo; hasil `rejected INVALID_SPEC` untuk satu -> `partial`; `.txt` -> skipped `NOT_SPEC_FILE`; `target_prefix` dikirim user diabaikan (config `""`, tidak divalidasi walau berisi `..`).
- Branch eksplisit menang atas branch di URL (`ListTree` menerima `main`); branch `feature/x` eksplisit dengan URL `.../tree/feature/x/docs` -> branch `feature/x`, folder `docs`.
- Hasil bus `unchanged` -> report `unchanged`; knowledge dengan `too_large` dari bus -> `rejected`.
- Tanpa kandidat: nol panggilan bus; status `success`; `SaveResult(success)`.
- Laporan > 500 item: respons memuat semua item; yang disimpan terpotong 500 dengan `truncated=true`, Summary utuh.

Batas dan kegagalan (semua: nol panggilan bus upsert, `SaveResult(failed, ErrorCode)`, audit `git_sync.run` status error, `unlock` dipanggil):
- Jumlah kandidat > `MaxFiles` -> 422 `SYNC_LIMIT_EXCEEDED`; nol `FetchBlob`.
- `Size` total diketahui > `MaxTotalBytes` -> sama.
- Total kumulatif saat fetch melewati `MaxUploadBytes` (Size -1) -> `SYNC_LIMIT_EXCEEDED`; fetch lain dibatalkan; tidak ada goroutine tersisa (cek dengan `goleak`-style `runtime.NumGoroutine` sebelum/sesudah atau WaitGroup di fake).
- Satu file `Size > MaxFileBytes` -> tidak di-fetch, `TooLarge=true` dikirim ke bus, item rejected, sync tetap jalan.
- `FetchBlob` melapor `tooLarge` (Size -1) -> idem.
- Folder tidak ada -> 404 `GIT_NOT_FOUND` "folder tidak ditemukan di branch ini" (`what=folder`).
- Tabel provider error: `ErrUnauthorized` -> 422 `GIT_TOKEN_REJECTED`; `ErrForbidden` -> 422 `GIT_ACCESS_DENIED`; `ErrNotFound` -> 404 `GIT_NOT_FOUND` (`what=repo`); rate limit dengan `RetryAfter=30s` -> 429 + `retry_after_seconds=30`; tanpa RetryAfter -> tanpa details; `ErrTreeTooLarge` -> 422; `ErrUpstream` -> 502; `context.DeadlineExceeded` -> 504 `TIMEOUT`. Dites untuk `ListTree` dan `FetchBlob`.
- Deadline: ctx dengan deadline lewat; `SaveResult` dan audit tetap terpanggil dan menerima ctx yang belum mati (assert `ctx.Err()==nil` di fake) dan membawa nilai identity.
- Konkurensi fetch: fake mencatat puncak panggilan serentak; assert <= 4 dan = 4 pada 10 file berlatensi buatan.
- Bus upsert: `CodeInvalid` -> 400 `INVALID_ARGUMENT` + `SaveResult(failed)`; error lain -> 500 `INTERNAL` + `SaveResult(failed, "INTERNAL")`.
- `SaveResult` gagal setelah tulis sukses -> tetap 200 (log error).
- Audit: Args `{kind, provider, host, repo_path, branch, folder, target_prefix}`, Result `{status, created, updated, unchanged, skipped, rejected, error_code}`; audit gagal (fail-open) tidak mengubah hasil.
- **Canary PAT** `ghp_CANARY0123456789abcdefCANARY` sebagai token fake get-token; logger buffer slog JSON level debug. Jalankan sukses, 401 provider, 404, rate limit, 502, dan bus 500. Assert canary tidak ada di: buffer log, JSON respons dan pesan error, setiap event audit (di-marshal ke JSON), argumen `SaveResult` (termasuk `last_result` hasil marshal), maupun `fmt.Sprintf("%+v")` atas error. `fakeGit` mendeteksi token yang diterima hanya lewat `Reveal()` di dalam fake.

SSRF di level usecase (memakai `gitprovider.Client` nyata + `safehttp` dengan resolver palsu, tanpa jaringan):
- Host GitLab self-hosted di allowlist yang resolve ke `10.0.0.5` -> error `ErrUpstream`/blocked dipetakan 502 `GIT_UPSTREAM_ERROR`; tidak ada koneksi keluar; `SaveResult(failed)`; tidak ada panggilan bus upsert.
- Idem untuk `127.0.0.1`, `169.254.169.254`, `[fc00::1]`.
- DNS rebinding (jawaban pertama IP publik, kedua privat) tetap ditolak saat dial.
- `httptest` server yang membalas 302 ke host lain -> tidak diikuti (error upstream), header token tidak terkirim ke host kedua.
- Host tidak di allowlist -> 422 `HOST_NOT_ALLOWED` sebelum token diminta.

### 6.3 `repository_integration_test.go` (tag `integration`, Postgres nyata)

- `UpsertConfig` insert lalu update (URL/branch/folder/prefix berubah) -> satu baris (unique `(project_id, kind)`), `last_*` tidak tersentuh, `created_at` tetap, `updated_at` maju; dua kind berbeda -> dua baris; project tak ada -> `ErrProjectNotFound`; `target_prefix` non-kosong untuk `api_spec` ditolak constraint (error, bukan panic).
- `SaveResult`: menyimpan status/waktu/pelaku/label dan `last_result` jsonb (round trip ke `base.Report`); status `failed` dengan `error_code`; 0 baris -> `ErrProjectNotFound`; `last_result::text` tidak memuat string canary.
- `TryLock`: dua `TryLock` serentak untuk `(P, knowledge)` -> tepat satu `ok`; `(P, api_spec)` dan `(Q, knowledge)` bukan yang sama sehingga tidak bentrok; setelah `unlock` -> `TryLock` berikutnya `ok`; setelah koneksi pemegang diputus paksa (`pg_terminate_backend` atau `conn.Close` tanpa unlock) -> lock lepas dan `TryLock` berikutnya `ok`; `unlock` dipanggil dua kali tidak panik; pool tetap sehat (tidak ada koneksi bocor: `db.Stats().InUse == 0` sesudahnya).

### 6.4 `runsync_integration_test.go` (tag `integration`, end-to-end in-process)

Setup: Postgres uji + migrasi core/project; `httptest.Server` meniru GitHub (`/user`, `/repos/{o}/{r}/git/trees/{branch}?recursive=1`, `/git/blobs/{sha}`) dengan `GIT_SYNC_INSECURE_TEST_HOSTS=<host:port>` dan `GIT_SYNC_GITHUB_API_BASE`; `GIT_CREDENTIAL_KEY` uji; module nyata `gitcredential` (bustoken), `knowledge` (busupsertfiles), `apispec` (busupsertspecs), `management` (check-access), audit nyata. Dua user A, B; project P (A dan B anggota), Q (project lain). PAT A disimpan lewat repository gitcredential (terenkripsi).
- Knowledge: sync pertama -> file `auth/login-flow.md`, `api/rate-limit.md` dibuat dengan struktur path benar; `diagram.png` skipped. `files` hanya di P bertambah; `files` di Q tidak berubah (hitung dan hash sebelum/sesudah).
- Sync ulang tanpa perubahan -> semua `unchanged`; konten berubah -> `updated` hanya file itu.
- File Grimoire yang tidak ada di git (`notes/manual.md` dibuat via upload) tetap ada dan tidak berubah.
- Prefix `guides` -> path `guides/auth/login-flow.md`; folder git tanpa prefix tidak menyentuh path non-prefix.
- API spec: `billing.yaml` valid dibuat (endpoint terindeks), `payments.json` dengan `$ref` eksternal rejected dengan issues, `partial`; sync ulang yang mengubah `billing.yaml` mengganti indeks endpoint sepenuhnya (jumlah endpoint baru, tanpa sisa lama); spec lain di P (diunggah manual) tidak tersentuh; spec Q tidak tersentuh; dua file berbasename sama di folder berbeda -> yang kedua `rejected DUPLICATE_NAME_IN_UPLOAD`.
- Batas: konfigurasi `MAX_UPLOAD_FILES=2` dengan 3 kandidat -> 422 `SYNC_LIMIT_EXCEEDED`, tabel knowledge/apispec tidak berubah, config `last_status='failed'`, `last_result.error_code='SYNC_LIMIT_EXCEEDED'`.
- Kegagalan provider (server uji membalas 401, 404, 403 rate-limit, 500): status HTTP dan kode sesuai; tabel knowledge/apispec tidak berubah; config `failed`.
- Keamanan: B tanpa PAT -> `CREDENTIAL_MISSING`, baris config dari A tidak berubah (bandingkan semua kolom). Server uji mencatat header `Authorization` yang diterima: pada sync A hanya token A; tidak pernah token lain.
- Dua sync serentak `(P, knowledge)` (server uji menahan respons tree dengan channel) -> satu 200, satu 409 `SYNC_IN_PROGRESS`; setelah selesai sync ketiga sukses (lock lepas).
- Audit: `audit_log` memuat `git_sync.run` (status ok/error) dan tidak memuat canary; `knowledge.upload.completed` / `apispec.uploaded` ber-`source: git_sync` dengan aktor = user pemicu.
- Canary di semua kolom `git_sync_configs`, `audit_log`, dan buffer log: tidak ditemukan.
- Non-anggota -> 404; tanpa sesi -> 401; tanpa CSRF -> 403 `CSRF_REQUIRED`; super_admin non-anggota dengan PAT sendiri -> 200.
- Timeout: `GIT_SYNC_TIMEOUT` kecil + server uji lambat -> 504 `TIMEOUT`; config `failed` `error_code='TIMEOUT'`; lock lepas.

### 6.5 Frontend (vitest)

- `types.test.ts`: Zod menerima contoh respons bagian 8.1 dan 8.2; menolak status di luar enum; field `openapi_version/title/endpoint_count` opsional.
- `runsync.service.test.ts`: sukses memetakan snake_case; error envelope `CREDENTIAL_MISSING` -> `SyncError{code, settingsPath}`; `RATE_LIMITED` membawa `retryAfter`; CSRF header terkirim; respons tak sesuai skema -> error terkontrol; abort.
- `runsync.store.test.ts`: transisi `idle -> running -> done|error`; start dobel diabaikan saat running; `canSubmit` false tanpa credential/feature.
- `components.test.ts`: `StatusBadge` kelas per status; `RepoOriginChip` string benar (dengan dan tanpa folder); `SyncReportTable` merender kolom, alasan ter-i18n, baris ditolak dapat diperluas dan menampilkan pointer, tombol salin menyalin alasan; `ConfigSummary` "Belum pernah sync".
- `GitSyncDialog.test.ts`: form terisi dari config; field "Simpan di folder" hanya untuk knowledge; tanpa PAT -> Sync nonaktif + tautan `/settings#token-git`; `credential_unreadable` -> teks simpan ulang; klik Sync memanggil `runSync` dengan payload benar (api_spec tanpa `target_prefix`); state running menampilkan teks progres; sukses memanggil `onSynced` sekali; error 422 `GIT_NOT_FOUND` menampilkan pesan ter-i18n di dalam dialog; "Sync ulang" memakai config tersimpan.
- `KnowledgeTab.test.ts`, `ApiSpecsTab.test.ts`: tombol "Sync dari Git" muncul sebagai aksi sekunder; membuka dialog; `onSynced` memuat ulang pohon/daftar.
- `npm run depcruise`, `npm run test:arch`, `i18n-keys.test.ts` hijau.

### 6.6 Arch

`backend/internal/archtest` hijau: `runsync` hanya meng-import `gitsync/base/**`, `infra/{bus,db,http,middleware,app,config,gitprovider}`, `lib/{common,constants,contracts}`; tidak meng-import `gitcredential`, `knowledge`, `apispec`. Grep tes: tidak ada `SELECT/INSERT/UPDATE` di luar `repository.go`.

---

## 7. E2E Scenarios (`frontend/e2e/git-sync.spec.ts`)

Setup (overview bagian 11): fixture Node `fake-git-server.ts` meniru GitHub (`GET /user`, tree, blob) dengan endpoint kontrol untuk mengganti isi repo dan mode kegagalan (401, 404, rate limit, lambat); backend e2e dijalankan dengan `GIT_SYNC_INSECURE_TEST_HOSTS`, `GIT_CREDENTIAL_KEY` uji, `GIT_SYNC_GITHUB_API_BASE`. Helper `e2e/helpers/gitsync.ts`: simpan token via API, buat project, buat user B anggota. Token sah fixture `ghp_e2eValidToken7Qx2`.

Positif:
1. User A menyimpan token di `/profile` lalu membuka Knowledge project P > "Sync dari Git" > URL `https://github.com/noobee/grimoire-docs/tree/main/docs`: hasil menampilkan `auth/login-flow.md` dibuat, `api/rate-limit.md` dibuat, `img/diagram.png` dilewati. Dialog ditutup: pohon memuat `auth/login-flow.md`.
2. Buka dialog lagi: URL/branch `main`/folder `docs` terisi, ringkasan "Terakhir … Berhasil · 2 dibuat". "Sync ulang" tanpa perubahan -> semua "tidak berubah".
3. Fixture mengubah `docs/auth/login-flow.md`; "Sync ulang" -> "diperbarui" untuk file itu; `notes/manual.md` (dibuat via upload) tetap ada di pohon.
4. Prefix "Simpan di folder" `guides` -> pohon memuat `guides/auth/login-flow.md`.
5. API Spec > "Sync dari Git" folder `api/openapi`: `billing` dibuat (14 endpoint), `payments.json` ditolak ("$ref eksternal tidak didukung"), baris dapat diperluas menampilkan pointer `#/paths/~1charges/get`; ringkasan "Berhasil sebagian"; daftar spec memuat `billing`.
6. Branch eksplisit `feature/x` (fixture menyajikan branch itu) dengan URL tanpa `tree`: berhasil.
7. Progres: server fixture lambat 1 detik -> tombol menampilkan "Menarik dari GitHub…" lalu hasil.

Negatif:
1. Tanpa token (belum disimpan): dialog menampilkan kotak "Atur Token Git", tombol Sync nonaktif; panggilan API langsung -> 422 `CREDENTIAL_MISSING`.
2. User B (anggota P tanpa token) membuka dialog: ringkasan config A terlihat, Sync nonaktif; API langsung -> `CREDENTIAL_MISSING`; GET config sesudahnya tidak berubah.
3. Hapus token A lalu Sync -> `CREDENTIAL_MISSING`.
4. URL `https://bitbucket.org/x/y` atau host GitLab di luar allowlist -> `HOST_NOT_ALLOWED` tampil di form; tanpa permintaan ke fixture.
5. URL repo tanpa `tree` dan tanpa branch -> pesan "Isi Branch".
6. Folder `tidak-ada` -> pesan "folder tidak ditemukan"; ringkasan config menampilkan "Gagal".
7. Fixture 401 -> "Token Git ditolak provider; simpan ulang token"; fixture 404 repo -> pesan repo/branch tidak ditemukan; fixture rate limit (403 + `x-ratelimit-remaining: 0`) -> pesan rate limit.
8. Melebihi batas (`MAX_UPLOAD_FILES` kecil di backend e2e atau repo fixture besar) -> `SYNC_LIMIT_EXCEEDED` "persempit folder", pohon tidak berubah.
9. Dua sync serentak (dua tab/sesi sama user, fixture menahan respons) -> yang kedua mendapat "sync lain sedang berjalan".
10. Non-anggota memanggil API run -> 404; tanpa login -> 401; tanpa header CSRF -> 403.
11. Token tidak pernah muncul di DOM, di body respons mana pun (`page.on('response')` mencari `ghp_e2eValidToken7Qx2`), atau di localStorage.

Regresi: `knowledge*.spec.ts`, `api-spec*.spec.ts` tetap hijau (header berisi tombol baru).

---

## 8. Request/Response body

Semua mutasi mengirim header CSRF `X-Requested-With: XMLHttpRequest`. Format error: `{"error": {"code", "message", "details"}, "trace_id"}`. `trace_id` contoh: `4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33`.

### 8.1 Request knowledge

```http
POST /api/projects/0192f5a0-1111-7aaa-8bbb-000000000001/git-sync/run
Content-Type: application/json
X-Requested-With: XMLHttpRequest
```
```json
{
  "kind": "knowledge",
  "url": "https://github.com/noobee/grimoire-docs/tree/main/docs",
  "branch": "",
  "folder": "",
  "target_prefix": "guides"
}
```

200:
```json
{
  "report": {
    "status": "success",
    "summary": { "created": 2, "updated": 0, "unchanged": 0, "skipped": 1, "rejected": 0 },
    "items": [
      { "repo_path": "docs/api/rate-limit.md", "target": "guides/api/rate-limit.md", "status": "created", "reason_code": "", "reason": "", "issues": [] },
      { "repo_path": "docs/auth/login-flow.md", "target": "guides/auth/login-flow.md", "status": "created", "reason_code": "", "reason": "", "issues": [] },
      { "repo_path": "docs/img/diagram.png", "target": "", "status": "skipped", "reason_code": "unsupported_type", "reason": "", "issues": [] }
    ],
    "truncated": false,
    "error_code": "",
    "error_message": ""
  },
  "config": {
    "provider": "github",
    "host": "github.com",
    "repo_url": "https://github.com/noobee/grimoire-docs/tree/main/docs",
    "repo_path": "noobee/grimoire-docs",
    "branch": "main",
    "folder": "docs",
    "target_prefix": "guides",
    "last_status": "success",
    "last_synced_at": "2026-10-22T14:05:00Z",
    "last_synced_by": { "id": "0192f5a0-aaaa-7aaa-8bbb-00000000000a", "label": "Rina" },
    "last_result": null
  }
}
```

### 8.2 Request api_spec (`target_prefix` diabaikan)

```json
{ "kind": "api_spec", "url": "https://github.com/noobee/grimoire-docs", "branch": "main", "folder": "api/openapi" }
```

200 (partial):
```json
{
  "report": {
    "status": "partial",
    "summary": { "created": 1, "updated": 0, "unchanged": 0, "skipped": 0, "rejected": 1 },
    "items": [
      { "repo_path": "api/openapi/billing.yaml", "target": "billing", "status": "created", "reason_code": "", "reason": "", "issues": [],
        "openapi_version": "3.0.3", "title": "Billing API", "endpoint_count": 14 },
      { "repo_path": "api/openapi/payments.json", "target": "payments", "status": "rejected", "reason_code": "INVALID_SPEC", "reason": "$ref eksternal tidak didukung",
        "issues": [ { "code": "EXTERNAL_REF", "message": "$ref eksternal tidak didukung", "pointer": "#/paths/~1charges/get", "line": 0 } ] }
    ],
    "truncated": false,
    "error_code": "",
    "error_message": ""
  },
  "config": {
    "provider": "github", "host": "github.com",
    "repo_url": "https://github.com/noobee/grimoire-docs/tree/main/api/openapi",
    "repo_path": "noobee/grimoire-docs", "branch": "main", "folder": "api/openapi", "target_prefix": "",
    "last_status": "partial", "last_synced_at": "2026-10-22T14:05:00Z",
    "last_synced_by": { "id": "0192f5a0-aaaa-7aaa-8bbb-00000000000a", "label": "Rina" },
    "last_result": null
  }
}
```

### 8.3 Request branch ber-`/` eksplisit

```json
{ "kind": "knowledge", "url": "https://github.com/noobee/grimoire-docs/tree/feature/x/docs", "branch": "feature/x", "folder": "", "target_prefix": "" }
```
Hasil: `config.branch = "feature/x"`, `config.folder = "docs"`.

### 8.4 Error

401 (tanpa login):
```json
{ "error": { "code": "UNAUTHORIZED", "message": "autentikasi diperlukan", "details": null }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```
404 (non-anggota):
```json
{ "error": { "code": "NOT_FOUND", "message": "project tidak ditemukan", "details": null }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```
400 (kind invalid):
```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "permintaan tidak valid", "details": { "fields": { "kind": "kind harus knowledge atau api_spec" } } }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```
400 (URL):
```json
{ "error": { "code": "INVALID_REPO_URL", "message": "isi Branch", "details": { "field": "branch" } }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```
422 HOST_NOT_ALLOWED:
```json
{ "error": { "code": "HOST_NOT_ALLOWED", "message": "host Git tidak diizinkan; minta admin menambahkannya ke GIT_SYNC_ALLOWED_HOSTS", "details": { "host": "git.contoh.com" } }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```
503 GIT_FEATURE_DISABLED:
```json
{ "error": { "code": "GIT_FEATURE_DISABLED", "message": "sync Git belum dikonfigurasi oleh admin", "details": null }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```
422 CREDENTIAL_MISSING:
```json
{ "error": { "code": "CREDENTIAL_MISSING", "message": "Anda belum menyimpan token Git untuk github.com", "details": { "provider": "github", "host": "github.com", "settings_path": "/settings#token-git" } }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```
422 CREDENTIAL_UNREADABLE:
```json
{ "error": { "code": "CREDENTIAL_UNREADABLE", "message": "token Git Anda tidak dapat dibaca (kunci server berganti); simpan ulang token", "details": { "provider": "github", "host": "github.com", "settings_path": "/settings#token-git" } }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```
409 SYNC_IN_PROGRESS:
```json
{ "error": { "code": "SYNC_IN_PROGRESS", "message": "sync lain untuk project dan jenis ini sedang berjalan", "details": { "kind": "knowledge" } }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```
422 SYNC_LIMIT_EXCEEDED:
```json
{ "error": { "code": "SYNC_LIMIT_EXCEEDED", "message": "folder berisi 742 file, melebihi batas 500 file per sync; persempit folder", "details": { "files": 742, "max_files": 500, "known_bytes": 3145728, "max_total_bytes": 52428800 } }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```
404 GIT_NOT_FOUND (repo/branch):
```json
{ "error": { "code": "GIT_NOT_FOUND", "message": "repo/branch tidak ditemukan atau token tidak punya akses", "details": { "what": "repo" } }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```
404 GIT_NOT_FOUND (folder):
```json
{ "error": { "code": "GIT_NOT_FOUND", "message": "folder tidak ditemukan di branch ini", "details": { "what": "folder", "folder": "tidak-ada" } }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```
422 GIT_TOKEN_REJECTED:
```json
{ "error": { "code": "GIT_TOKEN_REJECTED", "message": "token Git ditolak provider; simpan ulang token", "details": null }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```
422 GIT_ACCESS_DENIED:
```json
{ "error": { "code": "GIT_ACCESS_DENIED", "message": "token tidak punya akses ke repo ini", "details": null }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```
422 GIT_TREE_TOO_LARGE:
```json
{ "error": { "code": "GIT_TREE_TOO_LARGE", "message": "repo terlalu besar untuk dibaca sekaligus; persempit folder", "details": null }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```
429 RATE_LIMITED (header `Retry-After: 60`):
```json
{ "error": { "code": "RATE_LIMITED", "message": "provider Git membatasi permintaan; coba lagi nanti", "details": { "retry_after_seconds": 60 } }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```
502 GIT_UPSTREAM_ERROR:
```json
{ "error": { "code": "GIT_UPSTREAM_ERROR", "message": "provider Git tidak dapat dihubungi, coba lagi", "details": null }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```
504 TIMEOUT:
```json
{ "error": { "code": "TIMEOUT", "message": "sync melebihi batas waktu", "details": null }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```
413 (body > 16 KiB):
```json
{ "error": { "code": "PAYLOAD_TOO_LARGE", "message": "ukuran permintaan melebihi batas", "details": null }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```

### 8.5 Bus (bentuk Go, request dikirim sebagai nilai)

```go
// get-token (UserID dari identity request)
gettoken.Request{UserID: "0192f5a0-aaaa-7aaa-8bbb-00000000000a", Provider: "github", Host: "github.com"}

// knowledge
upsertfiles.Request{ProjectID: "0192f5a0-1111-7aaa-8bbb-000000000001",
	Actor: upsertfiles.Actor{UserID: "0192f5a0-aaaa-7aaa-8bbb-00000000000a", Role: "developer", Label: "Rina"},
	TargetPrefix: "guides", Source: upsertfiles.SourceGitSync,
	Meta:  upsertfiles.Meta{IP: "10.1.2.3", UserAgent: "Mozilla/5.0"},
	Items: []upsertfiles.Item{{Index: 0, Path: "api/rate-limit.md", Content: []byte("# Rate limit")},
		{Index: 1, Path: "auth/login-flow.md", Content: []byte("# Login")}}}

// api_spec
upsertspecs.Request{ProjectID: "...", Actor: upsertspecs.Actor{...}, Source: upsertspecs.SourceGitSync, Meta: upsertspecs.Meta{...},
	Items: []upsertspecs.Item{{Index: 0, SourcePath: "api/openapi/billing.yaml", Content: []byte("openapi: 3.0.3 ...")}}}
```

### 8.6 Audit

```json
{ "action": "git_sync.run", "project_id": "0192f5a0-1111-7aaa-8bbb-000000000001", "target_type": "git_sync", "target": "api_spec",
  "args": { "kind": "api_spec", "provider": "github", "host": "github.com", "repo_path": "noobee/grimoire-docs", "branch": "main", "folder": "api/openapi", "target_prefix": "" },
  "result": { "status": "partial", "created": 1, "updated": 0, "unchanged": 0, "skipped": 0, "rejected": 1, "error_code": "" }, "status": "ok" }
```
Gagal setelah config tersimpan: `result.status="failed"`, `result.error_code="GIT_NOT_FOUND"`, `status="error"`. Tidak ada token, URL berquery, atau isi file.
