# T17 / project/git-sync / runsync (revisi) — Plan Usecase

Tiket: `docs/tickets/T17-project-git-sync-progress.md`. Dokumen induk (menang bila bertentangan): `docs/superpowers/plans/project-git-sync-progress/overview.md` (D1-D11, bagian 5.2-5.5, F1-F5, 11, 12). Plan T14 sebelumnya: `docs/superpowers/plans/project-git-sync/usecase-runsync.md` (perilaku tidak berubah kecuali yang tertulis di sini).
Konteks yang dicek: `CLAUDE.md` tidak ada; `.claude/rules/` hanya `migrations.md` (tidak relevan, tanpa migrasi); `docs/tickets/README.md` (konvensi arsitektur, katalog bus/audit); `docs/superpowers/specs/2026-10-20-grimoire-design.md` (padanan `*knowledge-hub-design.md`); `DESIGN.md` dan `.claude/skills/design-taste-frontend/references/app-screens.md` untuk FE. Kode yang dibaca langsung: `runsync/{usecase,handler,params,init,repository}.go` beserta tes, `frontend/src/domain/project/git-sync/runsync/**`, `frontend/src/shared/services/http.ts`, `shared/components/Dialog.svelte`, `e2e/fixtures/fake-git-server.ts`.

Keputusan pemilik yang dipakai: jeda per request 500ms, configurable env `GIT_SYNC_REQUEST_INTERVAL` (default `500ms`, dikerjakan di foundation F1/F2). Usecase ini hanya mengonsumsinya lewat `gitprovider.Session`.

Prasyarat: foundation F1-F5 hijau (config env, `gitprovider.Session/Wait/Stats`, `statusWriter.FlushError`, `store.LockKeySQL`, `audit.Event.WaitCount/WaitTotal`). Plan ini tidak mengerjakan ulang itu; langkah di bawah hanya menyentuh file runsync, FE, dan dokumen yang disebut.

---

## 1. Metadata

| Atribut | Nilai |
|---|---|
| Ticket | T17 (revisi T14) |
| Scope | fullstack |
| Domain / Module / Feature | project / git-sync (`gitsync`) / `runsync` |
| Folder backend | `backend/internal/domain/project/internal/gitsync/runsync` |
| Folder FE | `frontend/src/domain/project/git-sync/runsync`, `frontend/src/shared/services` |
| Endpoint | `/api/projects/{projectID}/git-sync/run` |
| Method | `POST` |
| Version | `/api` tanpa prefix versi; `api/modules/project/git-sync.yml` `info.version` 0.1.0 -> 0.2.0 (aditif, ditulis di F6 overview; plan ini menyediakan isi skema run) |
| Negosiasi | `Accept` memuat `text/event-stream` -> SSE; selain itu JSON T14 |
| Middleware | `RequireAuth` + CSRF global + `BodyLimit(16<<10)` + `ExtendTimeout(GitSyncTimeout + 30s)` (berubah dari `GitSyncTimeout`) |
| Siapa | `member`, `owner`, `admin`, super_admin; bukan anggota -> 404; tanpa login -> 401 |
| Bus dipanggil | tetap: `project.management.check-access` (via `base/access`), `core.gitcredential.get-token`, `project.knowledge.upsert-files` atau `project.apispec.upsert-specs`, `core.audit.record` (via `base/audit`, fail-open) |
| Bus disediakan / kontrak baru | tidak ada |
| Migrasi | tidak ada |

---

## 2. Summary Plan

Revisi satu usecase yang sudah ada; tidak dipecah menjadi usecase baru. Usecase tetap satu orkestrasi (akses, validasi, token, lock, config, pull, write, simpan hasil, audit); yang berubah hanyalah bentuk eksekusinya:

1. **Pacing**: `GitClient` kini menghasilkan `GitSession` (dari `gitprovider.Client.Session(onWait)`) sehingga semua request tree/blob dalam satu sync ber-pacing (interval 500ms, tunggu rate limit, retry). Usecase tidak tidur sendiri (K8); ia hanya meneruskan `onWait` ke `Sink.Waiting`.
2. **Dua tahap `Start` / `Run.Execute`** (D6): `Start` melakukan akses, parse, token, `TryLock`, `UpsertConfig` dengan ctx request dan mengembalikan `*Run` yang memegang lock. Error di sini (400/403/404/409/422/503) selalu JSON berstatus HTTP, bahkan pada jalur SSE. `Run.Execute(ctx, sink)` menjalankan pull, write, simpan hasil, audit, unlock. `Run.Abort` melepas lock bila `Execute` batal dipanggil.
3. **Progress lewat `Sink`** (non-blocking, D8): `Start(total)` setelah `CheckBatchLimits`, `Progress` per kandidat, `Waiting` dari `onWait`, `Saving` sebelum panggilan bus upsert.
4. **Lepas dari klien** (D7): handler memanggil `Execute` dengan `context.WithTimeout(context.WithoutCancel(r.Context()), GitSyncTimeout)` di jalur JSON maupun SSE. Putusnya klien hanya menghentikan penulisan event, bukan sync.
5. **Handler SSE**: kanal event ber-buffer 64 (drop bila penuh), kanal hasil kapasitas 1, heartbeat 15s, `done`/`error` sebagai event terakhir setelah kanal event dikuras (D8).
6. **Error rate limit**: `mapError` membulatkan `retry_after_seconds` ke atas. Batas tunggu terlampaui = `RATE_LIMITED`, status `failed`, tidak ada panggilan bus upsert (fase tulis belum jalan), `last_result` tersimpan, audit error dengan `wait_count`/`wait_total_seconds`.
7. **FE**: `runSyncStream` (fetch + parser SSE, idle timeout 45s), store bertahap, komponen `SyncProgress.svelte`, dialog menampilkan listing, progress, menunggu, menyimpan, hasil; menutup dialog saat berjalan meminta konfirmasi dan tidak membatalkan server.

Aturan T14 yang tidak berubah: urutan D5, aturan replace, urutan file, bentuk `RunResponse`, token hanya milik pemanggil, token/URL berkredensial/isi file tidak pernah masuk event, log, atau error. Hasil sync dengan atau tanpa pacing identik (hanya waktu dan visibilitas berubah).

Keputusan tambahan usecase ini (tidak bertentangan dengan overview):

| ID | Keputusan |
|---|---|
| R1 | Counter `done` dinaikkan dan `sink.Progress` dipanggil di bawah satu mutex kecil (`progressEmitter`), sehingga nilai `done` yang diterima sink monoton walau 4 goroutine fetch. Sink non-blocking, jadi mutex tidak menjadi bottleneck. Kandidat `tooLarge` (tanpa fetch) tetap dihitung dan dipancarkan. |
| R2 | `Progress` hanya dipancarkan untuk kandidat yang berhasil diproses (blob diambil atau ditandai tooLarge). Kandidat yang gagal (error provider, batas kumulatif) tidak dipancarkan; run berakhir `error`. |
| R3 | `Start` event dipancarkan walau `total = 0` (`{"total":0}`), lalu langsung `done`. `saving` tidak dipancarkan bila tidak ada kandidat. |
| R4 | `Run.Execute` hanya boleh dipanggil sekali (guard `atomic.Bool`); panggilan kedua mengembalikan error internal tanpa menyentuh lock. `Abort` no-op bila `Execute` sudah dimulai (lock sudah menjadi tanggung jawab `Execute`) dan idempoten (`sync.Once` bersama unlock). |
| R5 | Goroutine `Execute` di handler dibungkus `recover`; panic menjadi hasil error `INTERNAL` generik, lock tetap dilepas lewat defer di `Execute`. |
| R6 | Jalur JSON tetap sinkron: handler menunggu `Execute` (ctx lepas). Klien JSON yang putus tidak menghentikan sync. Respons gagal rate limit tetap 429 + `Retry-After`. |
| R7 | Event `error` memakai `errs.Error` hasil `mapError` apa adanya (code, message, details). Kode `INTERNAL` selalu berpesan generik `terjadi kesalahan internal`. `context.Canceled` (hanya saat shutdown) menjadi `CANCELED`. |
| R8 | Klien tanpa `Accept: text/event-stream` (termasuk `*/*`) selalu mendapat JSON; `*/*` tidak memicu SSE. |
| R9 | FE: bila server lama membalas JSON padahal meminta SSE, service tetap memprosesnya sebagai `done` (toleransi rolling deploy). |

---

## 3. Plan (langkah berurutan)

### Backend

**B1. `runsync/params.go`**
1. Tambah `Sink` (persis overview 5.3), `NopSink`, dan DTO event: `StartEvent{Total int "total"}`, `ProgressEvent{Done, Total int; Path string}`, `WaitingEvent{RetryAfterSeconds int "retry_after_seconds"; Reason string}`, `SavingEvent struct{}`, `ErrorEvent{Code, Message string; Details any "details"}`. Konstanta nama event: `EventStart="start"`, `EventProgress`, `EventWaiting`, `EventSaving`, `EventDone`, `EventError`.
2. Doc comment `Sink`: implementasi harus aman dipakai bersamaan dan tidak boleh memblok; `Execute` memanggilnya dari beberapa goroutine.

**B2. `runsync/usecase.go` — antarmuka dan tipe**
1. Ganti `GitClient`:
   ```go
   type GitClient interface {
       Policy() gitprovider.HostPolicy
       Session(onWait func(gitprovider.Wait)) GitSession
   }
   type GitSession interface {
       ListTree(ctx context.Context, repo gitprovider.Repo, folder string, token secret.String) ([]gitprovider.TreeEntry, error)
       FetchBlob(ctx context.Context, repo gitprovider.Repo, sha string, maxBytes int64, token secret.String) ([]byte, bool, error)
       Stats() gitprovider.Stats
   }
   ```
2. `Usecase` struct tetap. Tambah:
   ```go
   type Run struct {
       uc      *Usecase
       projectID string
       kind    base.Kind
       ref     base.RepoRef
       prefix  string
       token   secret.String
       actor   base.Actor
       meta    base.RequestMeta
       cfg     base.Config          // hasil UpsertConfig
       started time.Time
       unlock  func(context.Context) error
       once    sync.Once            // melepas lock tepat sekali
       running atomic.Bool          // Execute sudah dimulai
   }
   ```
3. `fin(ctx)` dipertahankan (ctx akhir dengan nilai terbawa, cancel/deadline dilepas, timeout 10s) untuk simpan hasil, audit, dan unlock, agar tetap berjalan walau `GitSyncTimeout` habis.

**B3. `Usecase.Start`**
`func (u *Usecase) Start(ctx context.Context, projectID string, in RunRequest, meta base.RequestMeta) (*Run, error)`. Isi = bagian awal `Execute` T14 tanpa perubahan urutan: `access.Require` -> `ParseKind` -> `ParseRepoURL(allowed)` -> `NormalizeTargetPrefix` (knowledge) -> `token` (bus `get-token`) -> `repo.TryLock` (`!ok` -> `base.SyncInProgress(kind)`) -> `repo.UpsertConfig`. Bila `UpsertConfig` gagal, lepaskan lock (`fin` + `unlock`) sebelum mengembalikan error. Sukses: kembalikan `*Run` yang memegang lock. Tidak ada defer unlock di `Start` pada jalur sukses. `started = u.clock()`.

**B4. `Run.Execute`**
`func (r *Run) Execute(ctx context.Context, sink Sink) (RunResponse, error)`:
1. `if !r.running.CompareAndSwap(false, true)` -> error internal (R4). `defer r.release()` (`release` = `once.Do` + `fin` + `unlock`, peringatan log bila gagal, seperti T14).
2. `sink == nil` -> `NopSink{}`.
3. `sess := r.uc.git.Session(func(w gitprovider.Wait) { sink.Waiting(int(math.Ceil(w.Duration.Seconds())), string(w.Reason)) })`.
4. `cands, skipped, items, err := r.uc.pull(ctx, sess, r, sink)`; gagal -> `return RunResponse{}, r.uc.fail(ctx, r.info(sess), err)`.
5. `r.uc.write(ctx, ..., sink)`: bila `len(cands) > 0` panggil `sink.Saving()` tepat sebelum bus upsert; gagal -> `fail`.
6. `report := base.NewReport(append(skipped, written...))`; `SaveResult` (ctx `fin`), `record` dengan `WaitCount/WaitTotal`, log sukses (tambahkan `wait_count`, `wait_total_ms`), kembalikan `RunResponse{Report, Config: dto}` dengan `dto.LastResult = nil` (sama dengan T14).
7. `func (r *Run) Abort(ctx context.Context)`: `if r.running.Load() { return }`; `r.release()` (ctx dari `fin(ctx)`).

**B5. `pull` dan `fetchAll`**
1. `pull(ctx, sess GitSession, r *Run, sink Sink)`: `sess.ListTree` -> `SelectFiles` -> `CheckBatchLimits` -> `sink.Start(len(cands))` -> `fetchAll`.
2. `fetchAll` menerima `sess` dan `sink`; konkurensi tetap 4. Tambah `emitter := &progressEmitter{total: len(cands), sink: sink}` dengan `func (p *progressEmitter) done(path string)` (mutex: `p.n++; sink.Progress(p.n, p.total, path)`). Panggil `emitter.done(c.RepoPath)`:
   - di cabang `tooLarge` sebelum fetch (`continue`);
   - setelah `FetchBlob` sukses dan (bila `tooLarge` dari provider) penandaan, atau setelah `CheckFetchedTotal` lolos dan `out[i].content` diisi.
   Path = `cand.RepoPath` (path relatif repo, tanpa host/token).
3. Perilaku batal-awal (`ctx2.Err()`, `firstErr`) tidak berubah.

**B6. `fail`, `record`, `mapError`**
1. `runInfo` ditambah `stats gitprovider.Stats`; `fail` dan sukses mengisinya dari `sess.Stats()` tepat sebelum `record`.
2. `record` mengisi `audit.Event.WaitCount = stats.Waits`, `WaitTotal = stats.TotalWait`.
3. `mapError`: `rateLimited(int(math.Ceil(rl.RetryAfter.Seconds())))`; cabang `ErrRateLimited` tanpa `RateLimitedError` tetap `rateLimited(0)`. Pesan tetap `provider Git membatasi permintaan; coba lagi nanti`.
4. `fail` menyimpan `FailedReport(code, msg)` ke `SaveResult` (status `failed`) seperti T14; tidak ada bus upsert karena `write` belum dipanggil.

**B7. `runsync/init.go`**
1. Adapter:
   ```go
   type gitAdapter struct{ c *gitprovider.Client }
   func (a gitAdapter) Policy() gitprovider.HostPolicy { return a.c.Policy() }
   func (a gitAdapter) Session(onWait func(gitprovider.Wait)) GitSession { return a.c.Session(onWait) }
   ```
   (`*gitprovider.Session` memenuhi `GitSession`.) `base.Shared.Git` tetap `*gitprovider.Client`.
2. `NewUsecase(..., gitAdapter{sh.Git}, ...)`; handler dibuat `newHandler(uc, handlerOpts{timeout: d.Config.GitSyncTimeout, heartbeat: 15 * time.Second})`.
3. `middleware.ExtendTimeout(d.Config.GitSyncTimeout + 30*time.Second)` (Start dan penulisan respons JSON memakai ctx ini; Execute memakai ctx lepas sendiri).

**B8. `runsync/repository.go`**
Hapus konstanta `lockKeySQL`; `TryLock` dan unlock memakai `store.LockKeySQL` (perilaku identik). Tidak ada method baru.

**B9. `runsync/handler.go`**
Struktur: `type handler struct { uc *Usecase; timeout, heartbeat time.Duration }`.

```
ServeHTTP:
  Cache-Control: no-store
  DecodeJSON -> error: WriteError (JSON)
  meta := base.NewRequestMeta(...)
  run, err := uc.Start(r.Context(), projectID, in, meta)
  err != nil: setRetryAfter + WriteError (JSON, juga bila Accept SSE)
  if !wantsSSE(r): serveJSON(w, r, run) else serveSSE(w, r, run)
```
- `wantsSSE`: parse `Accept` per media range (pisah `,`, buang parameter `;q=`), true bila ada tepat `text/event-stream` (R8).
- `execCtx(r)`: `context.WithTimeout(context.WithoutCancel(r.Context()), h.timeout)`.
- `serveJSON`: `ectx, cancel := execCtx(r)`; `res, err := run.Execute(ectx, NopSink{})`; error -> `setRetryAfter` + `WriteError`; sukses -> `WriteJSON 200`.
- `serveSSE`:
  1. Set header: `Content-Type: text/event-stream; charset=utf-8`, `Cache-Control: no-store`, `X-Accel-Buffering: no`, `Connection: keep-alive`; jangan set `Content-Length`.
  2. `rc := http.NewResponseController(w)`; `ferr := rc.Flush()`. Bila `errors.Is(ferr, http.ErrNotSupported)`: hapus header SSE, `run.Abort(r.Context())`, `WriteError` 500 internal generik, return (header belum ditulis berkat `FlushError` foundation F3).
  3. Buat `events := make(chan sseEvent, 64)`, `sink := &sseSink{ch: events}`, `done := make(chan outcome, 1)`. Jalankan goroutine: `defer recover` -> outcome error internal (R5); `ectx, cancel := execCtx(r)`; `res, err := run.Execute(ectx, sink)`; `done <- outcome{res, err}`.
  4. Bila `ferr != nil` (klien sudah pergi): return setelah menjalankan goroutine (sync tetap selesai).
  5. Loop `select`: `ev := <-events` -> `writeEvent`; `out := <-done` -> `drain(events)` lalu tulis `done` atau `error` (via `errorEvent(err)`), return; `<-ticker.C` -> tulis `: ping\n\n`; `<-r.Context().Done()` -> return. Setiap tulis diikuti `rc.Flush()`; error tulis -> return (klien putus; goroutine tetap jalan).
- `sseSink`: kirim ke `ch` dengan `select { case ch <- ev: default: }` (drop bila penuh). `Start` dan `Saving` memakai jalur yang sama (boleh drop pada klien sangat lambat; `done` tetap membawa hasil lengkap).
- `writeEvent`: `event: <name>\ndata: <json compact>\n\n`. `errorEvent(err)`: `errors.As(*errs.Error)` -> `{code, message, details}` (Internal -> pesan generik); `context.Canceled` -> `CANCELED`; selain itu `INTERNAL` generik. `details` `null` bila kosong.
- Tidak ada log yang memuat body event; log hanya project_id/kind/status.

### Frontend

**F-1. `shared/services/sse.ts` (baru) dan `shared/services/http.ts`**
1. `createSseParser(onEvent: (name: string, data: string) => void): { push(chunk: string): void; end(): void }`. Aturan: buffer baris parsial; terminator `\n`, `\r\n`, `\r` (bila chunk berakhir `\r`, tahan sampai chunk berikut); baris kosong = dispatch (hanya bila ada `data`); baris diawali `:` diabaikan (heartbeat); `event:` default `message`; `data:` multi-baris digabung `\n`; satu spasi awal setelah `:` dibuang; BOM di awal stream dibuang; field lain (`id`, `retry`) diabaikan; `end()` membuang event yang belum selesai.
2. `http.ts`: `openStream(method, url, { body, signal, headers }): Promise<Response>`: `buildUrl`, `Accept: text/event-stream`, `CSRF_HEADER` untuk mutasi, `Content-Type: application/json`, `credentials: 'same-origin'`; kegagalan fetch dipetakan seperti `request` (AbortError dilempar apa adanya, selain itu `ApiError` NETWORK). Non-2xx: baca teks -> `toApiError` (+ `notifyUnauthorized` pada 401) -> throw.

**F-2. `runsync/types.ts`**
1. Skema zod: `SyncStartEvent {total:int>=0}`, `SyncProgressEvent {done,total:int; path:string}`, `SyncWaitingEvent {retry_after_seconds:int>=0; reason:string}`, `SyncSavingEvent {}`, `RunSyncResponseSchema` (sudah ada) untuk `done`, `SyncErrorEventSchema {code,message,details?}`.
2. `type SyncEvent = {type:'start'...} | ...`; `parseSyncEvent(name, data): SyncEvent | null`: nama tak dikenal, JSON rusak, atau skema gagal -> `null` (diabaikan).
3. `toSyncErrorFromEvent(e)`: membungkus ke `ApiError({status:200, code, message, details})` lalu `toSyncError` agar `retryAfter`, `field`, `limit` konsisten dengan jalur JSON.
4. Tambah `isStreamLost(err: SyncError)`: `code` `NETWORK` atau `TIMEOUT` dengan `status 0`.

**F-3. `runsync/services/runsync.service.ts`**
1. Hapus `CLIENT_TIMEOUT_MS`; tambah `export const IDLE_TIMEOUT_MS = 45_000`.
2. `runSyncStream(projectId, input, onEvent: (e: SyncEvent) => void, signal?: AbortSignal): Promise<void>`: body sama dengan `runSync` lama; `openStream('POST', .../git-sync/run, ...)`; bila `Content-Type` JSON (R9) parse `RunSyncResponseSchema` dan emit `done`; selain itu baca `res.body.getReader()` + `TextDecoder({stream:true})` -> `createSseParser` -> `parseSyncEvent` -> `onEvent`. Timer idle di-reset tiap chunk (heartbeat ikut), habis -> `ctrl.abort(new DOMException('timeout','TimeoutError'))`. Alasan abort dipertahankan (AbortError vs TimeoutError) seperti kode lama. Stream berakhir tanpa event `done`/`error` -> throw `ApiError NETWORK` (dipetakan sebagai stream lost).
3. Pertahankan ekspor `runSync` lama? Tidak: dihapus, semua pemakai dialihkan ke store baru.

**F-4. `runsync/stores/runsync.store.ts`**
State:
```ts
type Progress =
  | { phase: 'listing' }
  | { phase: 'fetching'; done: number; total: number; path: string }
  | { phase: 'waiting'; done: number; total: number | null; path: string; retryAfter: number; until: number; reason: string }
  | { phase: 'saving'; total: number };
type RunState = { status: 'idle' } | { status: 'running'; progress: Progress } | { status: 'done'; report; config } | { status: 'error'; error: SyncError; lost: boolean };
```
Transisi: `start()` -> `running/listing`; `start` event -> `fetching{done:0,total}`; `progress` -> `fetching`; `waiting` -> `waiting` (menyimpan `until = Date.now() + retry_after_seconds*1000`, mewarisi done/total/path terakhir; `total=null` bila belum ada `start`); event berikutnya (`progress`/`saving`/`done`/`error`) mengakhiri waiting; `saving` -> `saving`; `done` -> `done`; `error` -> `error`. Event setelah terminal diabaikan. `lost = isStreamLost(err)` agar dialog menawarkan "Muat ulang status". `abort()` meng-abort stream tanpa pesan error (state kembali `idle`; server tetap jalan). `reset()` seperti sekarang. Helper murni `remainingSeconds(until, now)` = `max(0, ceil((until-now)/1000))`.

**F-5. `runsync/components/SyncProgress.svelte` (baru)** — lihat bagian Visual. Props: `progress: Progress`, `now: number` (dari dialog; ticker 1 detik hanya saat `waiting`). `data-testid` stabil: `sync-progress`, `sync-progress-bar`, `sync-progress-path`, `sync-waiting`, `sync-saving`.

**F-6. `runsync/views/GitSyncDialog.svelte`**
1. `running` tetap `$run.status === 'running'`; saat berjalan, form dan peringatan timpa disembunyikan dan diganti `<SyncProgress>`.
2. Ticker `now` (setInterval 1000ms) hanya aktif saat `progress.phase === 'waiting'`; dibersihkan di `onDestroy`/saat fase berubah.
3. Dialog `dismissible` tetap true saat berjalan; `cancel()` saat running membuka konfirmasi (state `confirmClose`) yang dirender sebagai `Dialog size="md"` kedua: judul "Tutup dialog sync?", isi "Sync tetap berjalan di server. Hasilnya bisa dilihat saat dialog dibuka lagi.", tombol utama "Tutup" (`variant="primary"`: `run.abort()`, toast info `gitSync.cancelledToast`, `onclose()`), sekunder "Tetap di sini". Tidak ada pembatalan ke server.
4. Saat dialog dibuka: `cfg.load(projectId, kind)` sudah ada di `$effect`; tidak diubah (membaca ulang `last_result` dan `in_progress` yang disediakan plan getconfig).
5. `lost` pada error: tampilkan pesan `gitSync.progress.lost` + tombol sekunder "Muat ulang status" yang memanggil `cfg.load(...)` dan `run.reset()`.
6. `SyncErrorBlock`: tambah `RATE_LIMITED` ke daftar `retryable`; tombol "Coba lagi" `disabled` selama `seconds > 0` (memakai `cooldown` yang ada di dialog); `onretry` memanggil `start(false)`.
7. Footer saat berjalan: hanya "Tutup" ghost (memicu konfirmasi); tombol Sync/Sync ulang disembunyikan.
8. Teks `gitSync.runningHint` diganti (tidak lagi "bisa sampai 2 menit").

**F-7. i18n `frontend/src/shared/i18n/{id,en}.json`** (kunci baru, id lalu en)
| Kunci | id | en |
|---|---|---|
| `gitSync.progress.listing` | Mengambil daftar file… | Fetching file list… |
| `gitSync.progress.count` | {done} dari {total} | {done} of {total} |
| `gitSync.progress.percent` | {percent}% | {percent}% |
| `gitSync.progress.waiting` | Menunggu batas request provider, lanjut dalam {seconds} detik | Waiting for provider rate limit, resuming in {seconds} s |
| `gitSync.progress.waitingAnnounce` | Sync menunggu batas request provider. | Sync is waiting for the provider rate limit. |
| `gitSync.progress.saving` | Menyimpan… | Saving… |
| `gitSync.progress.lost` | Koneksi ke server terputus. Sync mungkin masih berjalan di server. | Connection to the server was lost. The sync may still be running on the server. |
| `gitSync.progress.reloadStatus` | Muat ulang status | Reload status |
| `gitSync.progress.closeTitle` | Tutup dialog sync? | Close sync dialog? |
| `gitSync.progress.closeBody` | Sync tetap berjalan di server. Hasilnya bisa dilihat saat dialog dibuka lagi. | The sync keeps running on the server. You can see the result when you reopen this dialog. |
| `gitSync.progress.stay` | Tetap di sini | Stay here |
| `gitSync.runningHint` (ubah) | Sync berjalan di server; progress tampil di sini. | The sync runs on the server; progress is shown here. |

`gitSync.errors.RATE_LIMITED`/`RATE_LIMITED_RETRY` dan `common.retry` dipakai ulang. Kunci `gitSync.inProgress.*` milik plan getconfig.

### Dokumen dan OpenAPI (bagian run; penyatuan oleh F6 overview)
- `api/modules/project/git-sync.yml` operasi run: respons 200 menambah `text/event-stream` (`schema: {type: string}`, deskripsi format, rujukan skema `GitSyncEventStart|Progress|Waiting|Saving|Done|Error`, contoh dari bagian 7.2); respons 429 tetap (jalur JSON, header `Retry-After`); 409/422 tetap JSON; param header `Accept` didokumentasikan. `make api-bundle`, commit `api/openapi.yml`, `make api-check`.
- `docs/tickets/README.md` katalog audit `git_sync.run`: `result` + `wait_count`, `wait_total_seconds`.
- `docs/configuration.md` dan `.env.example` dikerjakan di F1 foundation (tidak diulang di sini).

---

## 4. Migration

Tidak ada. `git_sync_configs.last_result` (jsonb) dan advisory lock sudah ada sejak T14. Tidak ada kontrak bus baru dan tidak ada perubahan `lib/constants/bus.go`, `lib/contracts/**`.

---

## 5. Changes (per file/folder)

| File | Jenis | Isi |
|---|---|---|
| `backend/.../runsync/params.go` | ubah | `Sink`, `NopSink`, DTO dan nama event |
| `backend/.../runsync/usecase.go` | ubah | `GitClient`/`GitSession`, `Start`, `Run` (`Execute`, `Abort`, `release`), `progressEmitter`, `pull`/`fetchAll` dengan sink dan session, stats ke audit, `mapError` `math.Ceil` |
| `backend/.../runsync/handler.go` | ubah | negosiasi Accept, `serveJSON`, `serveSSE`, `sseSink`, heartbeat, `errorEvent`, ctx lepas |
| `backend/.../runsync/init.go` | ubah | `gitAdapter`, opsi handler, `ExtendTimeout(+30s)` |
| `backend/.../runsync/repository.go` | ubah | `store.LockKeySQL` |
| `backend/.../runsync/usecase_test.go` | ubah | `fakeGit` menjadi `Session` palsu (onWait terprogram, `Stats`), `fakeSink`, tes baru |
| `backend/.../runsync/handler_test.go` | ubah | tes SSE, JSON lama, heartbeat, disconnect |
| `backend/.../runsync/handler_router_test.go` | baru | tes lewat router nyata dengan `accesslog` + `Timeout` + `ExtendTimeout` |
| `backend/.../runsync/repository_integration_test.go` | ubah | `TryLock` memakai `store.LockKeySQL` |
| `frontend/src/shared/services/sse.ts` (+ `sse.test.ts`) | baru | parser SSE |
| `frontend/src/shared/services/http.ts` (+ `http.test.ts`) | ubah | `openStream` |
| `frontend/src/domain/project/git-sync/runsync/types.ts` (+ `types.test.ts`) | ubah | skema event, `parseSyncEvent`, `toSyncErrorFromEvent`, `isStreamLost` |
| `.../runsync/services/runsync.service.ts` (+ test) | ubah | `runSyncStream`, `IDLE_TIMEOUT_MS`, hapus `runSync`/`CLIENT_TIMEOUT_MS` |
| `.../runsync/stores/runsync.store.ts` (+ test) | ubah | state bertahap, `remainingSeconds` |
| `.../runsync/components/SyncProgress.svelte` | baru | progress, menunggu, menyimpan |
| `.../runsync/components/SyncErrorBlock.svelte` | ubah | `RATE_LIMITED` retryable, disabled saat cooldown |
| `.../runsync/components/components.test.ts` | ubah | tes `SyncProgress`, `SyncErrorBlock` |
| `.../runsync/views/GitSyncDialog.svelte` (+ `GitSyncDialog.test.ts`) | ubah | tahap progress, konfirmasi tutup, muat ulang status |
| `.../runsync/index.ts` | ubah | ekspor tipe baru bila perlu |
| `frontend/src/shared/i18n/{id,en}.json` | ubah | kunci F-7 |
| `frontend/e2e/fixtures/fake-git-server.ts`, `frontend/e2e/git-sync.spec.ts` | ubah | mode rate limit, 429 sekali, delay per blob, skenario progress |
| `api/modules/project/git-sync.yml`, `api/openapi.yml` | ubah | bagian run (SSE) |
| `docs/tickets/README.md` | ubah | katalog audit `git_sync.run` |

Tidak disentuh: `getconfig/**` (plan getconfig), `base/` (foundation), `infra/**` (foundation), migrations, bus.

---

## 6. Test Scenario

### 6.1 Unit usecase (`usecase_test.go`, `GitSession` palsu, `go test -race`)
Pertahankan seluruh tes T14 (dibuat hijau lewat `Start` + `Execute(ctx, NopSink{})` dengan helper `runAll(e, in)` agar laporan identik) lalu tambah:
1. Urutan event (`fakeSink` merekam): `start{total}` -> `progress` x total -> `saving`, lalu `Execute` mengembalikan laporan. Tanpa `waiting` bila fake tidak memanggil `onWait`.
2. Total dan `done`: 5 kandidat termasuk 1 `tooLarge` (tanpa fetch) -> 5 event progress, `done` monoton 1..5 walau fetch konkuren (delay acak), `path` = `RepoPath` relatif; `tooLarge` ikut dihitung.
3. `waiting` diteruskan: fake memanggil `onWait(Wait{37*time.Second+200ms, WaitRateLimitExhausted})` -> sink menerima `retry_after_seconds=38`, `reason=rate_limit_exhausted`; `Wait{2s, rate_limited}` -> 2.
4. Laporan identik T14: dengan atau tanpa sink (`NopSink` vs `fakeSink`) `RunResponse` sama (golden dari T14).
5. Gagal batas tunggu: fake `FetchBlob` mengembalikan `&RateLimitedError{RetryAfter: 1712s}` pada file ke-3 -> `Execute` mengembalikan `errs.Error` `RATE_LIMITED` dengan `details.retry_after_seconds == 1712`; `SaveResult` dipanggil dengan status `failed` dan `error_code RATE_LIMITED`; bus `upsert-files` TIDAK dipanggil; event `saving` tidak ada; audit error memuat `WaitCount`/`WaitTotal` dari `Stats` palsu; unlock terpanggil.
6. `RetryAfter` 1500ms -> `retry_after_seconds == 2` (dibulatkan ke atas); `RetryAfter 0` -> tanpa details.
7. Sink lambat/penuh tidak memblok `Execute`: diuji di `handler_test` (`sseSink` buffer 1 yang tidak dikuras, 200 kandidat, `Execute` selesai tanpa deadlock). Di usecase, kontrak Sink (tidak memblok) diuji dengan sink yang mencatat jumlah panggilan secara konkuren di bawah `-race`.
8. Ctx lepas: `Execute` dengan ctx yang induknya dibatalkan setelah `Start` tetap selesai (ctx lepas via `WithoutCancel`); `SaveResult` dan unlock dipanggil dengan ctx hidup.
9. `Start` saat Run lain aktif: `Start` pertama (belum `Execute`, atau `Execute` sedang menunggu rate limit lewat fake yang memblok di `onWait`) lalu `Start` kedua -> `SYNC_IN_PROGRESS` 409 (`fakeRepo.TryLock` memodelkan lock tunggal). Setelah `Execute` selesai atau `Abort`, `Start` baru berhasil.
10. `Abort`: idempoten (dua kali -> satu `unlock`); `Abort` sesudah `Execute` mulai no-op; `Execute` dua kali -> error internal tanpa unlock ganda.
11. Error `Start` tidak mengubah state: kredensial hilang/tak terbaca/dinonaktifkan, host tidak diizinkan, akses ditolak -> nol `lock`, nol `upsert`, nol `save` (sama dengan T14).
12. `UpsertConfig` gagal di `Start` -> lock dilepas dan error dikembalikan.
13. `Stats` sukses: sync tanpa tunggu menghasilkan audit `WaitCount=0`; dengan 2 tunggu total 40s -> `WaitCount=2`, `WaitTotal=40s`.
14. Tooling aman: token canary tidak muncul di event sink, log, error, audit.
15. `TestMapErrorTable` diperbarui: pembulatan ke atas, `ErrRateLimited` tanpa detail.
16. Tanpa kandidat: `start{0}`, tanpa `saving`, bus tidak dipanggil, laporan kosong sukses (seperti T14).
17. Timeout `GitSyncTimeout` terlampaui di tengah: error `TIMEOUT`, status `failed`, final steps memakai ctx hidup (`TestDeadlineFinalStepsUseLiveContext` disesuaikan).

### 6.2 Unit handler (`handler_test.go`, `httptest`)
1. Accept JSON / tanpa Accept / `*/*`: respons JSON T14 (`report`, `config`, `Cache-Control: no-store`), identik dengan tes lama.
2. Accept `text/event-stream`: `Content-Type: text/event-stream; charset=utf-8`, `X-Accel-Buffering: no`, `Cache-Control: no-store`, tanpa `Content-Length`; event berformat `event: x\ndata: {...}\n\n`; urutan `start`, `progress`..., `saving`, `done`; `done` memuat `report` + `config` dengan `last_result:null`.
3. Accept `text/event-stream, application/json;q=0.5` tetap SSE; `application/json, */*` JSON.
4. Error pra-stream: lock sibuk -> 409 `SYNC_IN_PROGRESS` JSON (bukan stream); kredensial hilang -> 422 `CREDENTIAL_MISSING` JSON; body invalid -> 400 JSON; kedua jalur Accept sama.
5. `RATE_LIMITED` di stream: event `error` terakhir dengan `details.retry_after_seconds`; tidak ada event `done`; jalur JSON mengembalikan 429 + `Retry-After`.
6. Klien memutus koneksi di tengah (ctx request dibatalkan setelah event pertama): handler return, `SaveResult` tetap terpanggil, unlock terpanggil, `Execute` selesai (tunggu dengan channel/`Eventually`, tanpa `time.Sleep` panjang).
7. Heartbeat: `heartbeat = 10ms` diinjeksi; saat fake git menahan fetch, `: ping\n\n` muncul; tidak muncul setelah event terakhir.
8. Event tidak memuat token canary, URL berkredensial/berquery, atau isi file (scan seluruh body stream).
9. Sink penuh: buffer 64, fake memancarkan 500 progress tanpa pembaca; `Execute` selesai, handler akhirnya menulis `done` (event progress dibuang tapi `done` utuh dan urutannya terakhir).
10. `done`/`error` selalu event terakhir: tidak ada `progress` setelahnya (uji dengan fake yang memancarkan progress tepat sebelum return).
11. Flush tidak didukung (writer palsu tanpa `Flusher`): 500 JSON generik, `Abort` dipanggil (lock lepas), header SSE tidak bocor.
12. Panic di `Execute` (fake git panic): event `error` `INTERNAL`, lock lepas, handler tidak panic.
13. `go test -race` hijau.

### 6.3 Router nyata (`handler_router_test.go`)
`httptest.NewServer` dengan rantai asli (`accesslog` -> `Timeout` -> `RequireAuth` palsu -> `ExtendTimeout` -> handler): klien membaca event `start` sebelum fake git melepas file berikutnya (sinkronisasi lewat channel), membuktikan `Flush` sampai ke klien melewati `statusWriter`; header SSE hadir; setelah selesai `accesslog` mencatat status 200.

### 6.4 Integration (`-tags integration`)
`repository_integration_test.go`: `TryLock` memakai `store.LockKeySQL` (dua `TryLock` pada `(project,kind)` sama -> kedua `ok=false`; `kind` lain `ok=true`; unlock melepas); regresi kunci identik dengan query `pg_locks` yang dipakai `IsLocked` (diuji penuh di plan getconfig).

### 6.5 Frontend (vitest)
1. `sse.test.ts`: chunk terpotong di tengah nama field/nilai/baris kosong; `\r\n` dan `\r` terpisah chunk; heartbeat `: ping` diabaikan; `data:` multi-baris digabung; event tanpa `data` tidak dispatch; BOM; beberapa event dalam satu chunk; `end()` membuang sisa.
2. `types.test.ts`: tiap skema event valid/invalid; nama event tak dikenal -> `null`; JSON rusak -> `null`; `toSyncErrorFromEvent` memetakan `retry_after_seconds` ke `retryAfter`, `limit`, `field`.
3. `runsync.service.test.ts` (fetch palsu dengan `ReadableStream`): event diteruskan berurutan; idle timeout 45s dengan fake timers (heartbeat mereset); non-200 -> `toSyncError` (409, 422, 401 memicu handler unauthorized); respons JSON (R9) -> `done`; stream berakhir tanpa terminal -> error NETWORK; abort pengguna tidak dianggap timeout; header `Accept`, CSRF, `Content-Type` benar.
4. `runsync.store.test.ts`: transisi `listing -> fetching -> waiting -> fetching -> saving -> done`; `waiting` sebelum `start` (total null); `error` dengan `retryAfter`; event setelah terminal diabaikan; `abort()` kembali `idle` tanpa error; `lost` pada NETWORK/TIMEOUT; `remainingSeconds` dengan fake timers (mulai 38 turun 1 per detik, tidak negatif).
5. `components.test.ts`: `SyncProgress` merender listing (bar indeterminate, teks), fetching (`7 dari 42`, `17%`, path), waiting (pesan + `role`/ikon, bar tanpa kelas error), saving; `aria-live` hanya diperbarui per 5 detik; `SyncErrorBlock` tombol "Coba lagi" untuk `RATE_LIMITED` disabled selama `seconds>0` lalu aktif.
6. `GitSyncDialog.test.ts` (store dan service dimock): dialog merender tiap tahap; form tersembunyi saat berjalan; tutup saat berjalan memunculkan konfirmasi, "Tetap di sini" menutup konfirmasi saja, "Tutup" memanggil `abort` + `onclose` + toast; `done` memanggil `onsynced` sekali; `RATE_LIMITED` menampilkan hitung mundur dan tombol "Coba lagi" aktif setelah 0; stream lost menampilkan "Muat ulang status" yang memanggil `cfg.load`; `onsynced` tidak dipanggil pada `failed`.
7. Lint: `npm run depcruise`, `npm run test:arch`, `node .agents/skills/design-taste-frontend/scripts/lint.mjs --app frontend/src`.

---

## 7. E2E Scenarios (Playwright, `frontend/e2e/git-sync.spec.ts`)

Backend e2e: `GIT_SYNC_REQUEST_INTERVAL=0s`, `GIT_SYNC_MAX_WAIT=3s`, `GIT_SYNC_MAX_TOTAL_WAIT=5s`. Fake git server ditambah opsi per repo: `rateLimit: {afterRequests, resetSeconds}` (kirim `x-ratelimit-remaining: 0` + `x-ratelimit-reset`), `status429Once`, `blobDelayMs`. Tiap tes memakai repo unik.

Positif:
1. Sync knowledge 5 file: dialog menampilkan "Mengambil daftar file…", lalu `n dari 5` dengan nama file (mono), lalu "Menyimpan…", lalu laporan per file identik T14; file muncul di tab Knowledge.
2. Rate limit setelah request ke-2 dengan reset 2s: muncul "Menunggu batas request provider, lanjut dalam 2 detik", angka turun, bar tidak berwarna error, lalu sync sukses.
3. `status429Once`: sync tetap sukses, laporan sama dengan tanpa 429.
4. Tutup dialog saat berjalan (`blobDelayMs`): konfirmasi muncul; "Tutup" menutup dialog; buka lagi -> banner "Sync sedang berjalan di server" dan tombol Sync nonaktif (plan getconfig); setelah selesai "Muat ulang status" menampilkan hasil terakhir.
5. Klien lama: `request.post` tanpa `Accept: text/event-stream` mendapat JSON T14 (200 `report`+`config`).
6. Respons SSE mentah (`request.post` dengan Accept SSE) berisi urutan `start`, `progress`, `saving`, `done`, content-type benar.

Negatif:
7. Reset 60s (> `MAX_WAIT` 3s): event `error` `RATE_LIMITED` `retry_after_seconds` 60 (hitung batas atas); UI menampilkan pesan rate limit dengan hitung mundur dan tombol "Coba lagi" nonaktif; tidak ada file baru di Knowledge.
8. Dua sesi menekan Sync bersamaan: yang kedua menerima 409 `SYNC_IN_PROGRESS` sebagai JSON (bukan stream).
9. Token belum disimpan: 422 `CREDENTIAL_MISSING` JSON sebelum stream; UI menampilkan `CredentialNotice` seperti T14.
10. Stream yang direkam tidak memuat token canary (`VALID_TOKEN`) maupun isi file.
11. Repo tidak ditemukan / token ditolak: event `error` `GIT_NOT_FOUND`/`GIT_TOKEN_REJECTED`, UI menampilkan pesan lapangan yang sama dengan T14.
12. Reload tab saat sync berjalan: sync tetap selesai di server, `last_result` terlihat saat dialog dibuka ulang.

---

## 8. Request/Response body

### 8.1 Request (tidak berubah dari T14)
```http
POST /api/projects/0b7e5a52-3c1f-4d1e-9a51-6f1f0c2d9e10/git-sync/run
Content-Type: application/json
Accept: text/event-stream
X-Requested-With: grimoire

{"kind":"knowledge","url":"https://github.com/noobee/grimoire-docs/tree/main/docs","branch":"","folder":"","target_prefix":"handbook"}
```
`kind=api_spec` tanpa `target_prefix`.

### 8.2 SSE sukses (200)
```
HTTP/1.1 200 OK
Content-Type: text/event-stream; charset=utf-8
Cache-Control: no-store
X-Accel-Buffering: no
Connection: keep-alive

event: start
data: {"total":3}

event: progress
data: {"done":1,"total":3,"path":"docs/auth/login-flow.md"}

event: waiting
data: {"retry_after_seconds":38,"reason":"rate_limit_exhausted"}

: ping

event: progress
data: {"done":2,"total":3,"path":"docs/auth/session-cookie.md"}

event: progress
data: {"done":3,"total":3,"path":"docs/onboarding/setup-local.md"}

event: saving
data: {}

event: done
data: {"report":{"status":"success","summary":{"created":2,"updated":1,"unchanged":0,"skipped":0,"rejected":0},"items":[{"repo_path":"docs/auth/login-flow.md","target":"handbook/auth/login-flow.md","status":"created","reason_code":"","reason":"","issues":[]},{"repo_path":"docs/auth/session-cookie.md","target":"handbook/auth/session-cookie.md","status":"updated","reason_code":"","reason":"","issues":[]},{"repo_path":"docs/onboarding/setup-local.md","target":"handbook/onboarding/setup-local.md","status":"created","reason_code":"","reason":"","issues":[]}],"truncated":false,"error_code":"","error_message":""},"config":{"provider":"github","host":"github.com","repo_url":"https://github.com/noobee/grimoire-docs","repo_path":"noobee/grimoire-docs","branch":"main","folder":"docs","target_prefix":"handbook","last_status":"success","last_synced_at":"2026-10-22T08:14:03Z","last_synced_by":{"id":"5d2c9a7e-1b44-4f0b-8c1e-2a9d7f3e6b21","label":"Reyhan"},"last_result":null}}

```
Urutan event: `waiting` bisa muncul di fase listing (sebelum `start`) maupun fetch; `waiting` hanya untuk tunggu rate limit, bukan interval 500ms.

### 8.3 SSE gagal setelah Start (status 200, event terakhir `error`)
```
event: start
data: {"total":120}

event: progress
data: {"done":61,"total":120,"path":"docs/api/errors.md"}

event: error
data: {"code":"RATE_LIMITED","message":"provider Git membatasi permintaan; coba lagi nanti","details":{"retry_after_seconds":1712}}

```
Kode lain di event `error`: `GIT_TOKEN_REJECTED`, `GIT_ACCESS_DENIED`, `GIT_NOT_FOUND`, `GIT_TREE_TOO_LARGE`, `SYNC_LIMIT_EXCEEDED`, `GIT_UPSTREAM_ERROR`, `TIMEOUT`, `INVALID_ARGUMENT`, `CANCELED`, `INTERNAL` (pesan `terjadi kesalahan internal`). `details` = `null` bila tidak ada. Event `error` untuk gagal sebelum listing selesai tidak didahului `start`.

Penyimpanan: status `failed` dan `last_result` (laporan gagal dengan `error_code`/`error_message`) tersimpan; tidak ada file yang ditulis.

### 8.4 Error sebelum stream (JSON, jalur SSE maupun JSON)
```http
HTTP/1.1 409 Conflict
Content-Type: application/json

{"error":{"code":"SYNC_IN_PROGRESS","message":"sync lain untuk project dan jenis ini sedang berjalan","details":{"kind":"knowledge"}},"trace_id":"7f3c2a9e4b1d4c08"}
```
```http
HTTP/1.1 422 Unprocessable Entity
Content-Type: application/json

{"error":{"code":"CREDENTIAL_MISSING","message":"Anda belum menyimpan token Git untuk github.com","details":{"provider":"github","host":"github.com","settings_path":"/profile#git-credentials"}},"trace_id":"7f3c2a9e4b1d4c08"}
```
```http
HTTP/1.1 400 Bad Request
Content-Type: application/json

{"error":{"code":"INVALID_REPO_URL","message":"URL repo tidak valid","details":{"field":"url"}},"trace_id":"7f3c2a9e4b1d4c08"}
```
Pesan dan details mengikuti konstanta T14 di `gitsync/base/errors.go` (kode kredensial aktual mengikuti konstanta di sana). Sebelum stream: 401 tanpa login, 404 project tidak ada/bukan anggota, 503 fitur dinonaktifkan.

### 8.5 Jalur JSON (tanpa `Accept: text/event-stream`)
Sukses: 200 `{"report":...,"config":...}` identik T14. Gagal batas tunggu:
```http
HTTP/1.1 429 Too Many Requests
Retry-After: 1712
Content-Type: application/json

{"error":{"code":"RATE_LIMITED","message":"provider Git membatasi permintaan; coba lagi nanti","details":{"retry_after_seconds":1712}},"trace_id":"7f3c2a9e4b1d4c08"}
```

### 8.6 Skema event (untuk OpenAPI)
`GitSyncEventStart {total:int}`, `GitSyncEventProgress {done:int,total:int,path:string}`, `GitSyncEventWaiting {retry_after_seconds:int, reason: enum[rate_limit_exhausted, rate_limited]}`, `GitSyncEventSaving {}`, `GitSyncEventDone = RunGitSyncResponse`, `GitSyncEventError {code,message,details}`.

---

## 9. Visual (FE wajib)

Sumber: `DESIGN.md` (Wajah produk #3 aksen kuning hanya untuk isian/garis, status selalu dengan teks/ikon, radius kontrol `rounded-base`) dan `design-taste-frontend/references/app-screens.md` (layar app: pertanyaan per layar, satu hal utama per layar, tidak ada hiasan).

### Layar A: Dialog Sync, tahap berjalan (menggantikan form)
- Pertanyaan layar: "Sudah sampai mana sync ini, dan apakah sedang menunggu?"
- Pola DESIGN.md: `Dialog size="lg"` (`shadow-pop`, radius 16) yang ada. Isi berurutan: `RepoOriginChip` (asal repo), blok progress, kalimat status. Form dan peringatan timpa disembunyikan saat berjalan.
- Elemen khas: progress bar tinggi 8px, radius penuh, track `bg-raised`, isian `bg-accent`; label ujung `7 dari 42` (kiri) dan `17%` (kanan), `tabular-nums`; nama file terakhir di bawah bar, IBM Plex Mono 12px `text-muted`, dipotong di tengah (`docs/…/login-flow.md`, `title` berisi path penuh). Transisi lebar 120ms ease-out.
  - Listing: bar indeterminate statis bergaris diagonal (tanpa animasi gerak; `prefers-reduced-motion` dihormati), teks "Mengambil daftar file…".
  - Menunggu: lebar bar membeku di nilai terakhir, isian menjadi `bg-border` (bukan merah), baris status `text-warning` dengan ikon jam + "Menunggu batas request provider, lanjut dalam 38 detik" (ikon + teks, bukan hanya warna). Hitung mundur dari waktu klien; `aria-live="polite"` memperbarui maksimal tiap 5 detik, plus satu pengumuman saat mulai menunggu (`waitingAnnounce`).
  - Menyimpan: bar penuh, teks "Menyimpan…".
- Token: `bg-accent`, `bg-raised`, `bg-border`, `text-muted`, `text-warning`, `text-accent-ink` (fokus), `rounded-base`, `rounded-full` hanya bar. Tidak ada gradien, tidak ada glow, tidak ada spinner di samping bar.
- Data contoh (fixture vitest): project "Grimoire Docs", repo `noobee/grimoire-docs`, total 42, `done` 7, path `docs/auth/login-flow.md`, `docs/onboarding/setup-local.md`, tunggu 38 detik, `retry_after_seconds` 1712. Dilarang Acme/lorem.
- Footer saat berjalan: satu tombol ghost "Tutup".

### Layar B: Konfirmasi tutup saat berjalan
- Pertanyaan: "Kalau saya tutup sekarang, apakah sync-nya batal?"
- Pola: `Dialog size="md"` kecil kedua; judul 1 baris, satu kalimat penjelasan ("Sync tetap berjalan di server. Hasilnya bisa dilihat saat dialog dibuka lagi."), tombol utama near-black "Tutup" dan sekunder "Tetap di sini". Fokus awal di "Tetap di sini" (aksi aman).
- Token: tombol standar `Button primary/secondary`; tanpa warna bahaya (menutup tidak merusak).

### Layar C: Dialog Sync, hasil dan error
- Pertanyaan: "Apa yang berubah, dan bila gagal, kapan boleh coba lagi?"
- Hasil: `ReportSummary` + `SyncReportTable` T14 tanpa perubahan.
- Error rate limit: `SyncErrorBlock` dengan `text-danger` ("provider Git membatasi permintaan; coba lagi dalam N detik"), tombol sekunder "Coba lagi" nonaktif sampai hitung mundur 0 (memakai `cooldown` dialog). Stream terputus: teks `text-warning` + tombol sekunder "Muat ulang status".
- Data contoh: `retry_after_seconds` 1712 (tampil "28 menit"-setara tidak diformat khusus; teks memakai detik sesuai kunci i18n yang ada).

Teks UI Indonesia; kunci id/en di F-7. Banner "Sync sedang berjalan di server" (dialog dibuka ulang) dirancang di plan getconfig dan hanya dikonsumsi dialog ini lewat `data.in_progress`.

---

## 10. Urutan implementasi

1. B1 params -> B2/B3/B4/B5/B6 usecase (tes unit `usecase_test.go` diperbarui dulu: `fakeGit` jadi `Session`, lalu tes baru) -> B7 init -> B8 repository -> B9 handler (+ tes handler, router, integration).
2. F-1 `sse.ts`/`http.ts` -> F-2 types -> F-3 service -> F-4 store -> F-5 `SyncProgress` -> F-6 dialog + F-7 i18n (FE bisa paralel dengan B9 memakai mock stream).
3. E2E setelah backend dan FE jalan; fake git server diperluas lebih dulu.
4. OpenAPI/dokumen (bagian run) di F6 overview.
5. Verifikasi: `go build ./...`, `go vet ./...`, `go test -race ./internal/domain/project/internal/gitsync/...`, `go test -tags integration ./internal/domain/project/internal/gitsync/...`, `npm run test`, `npm run depcruise`, `npm run test:arch`, lint design-taste, `make api-check`, `npx playwright test e2e/git-sync.spec.ts`.

## 11. Risiko khusus usecase ini

| Risiko | Mitigasi |
|---|---|
| Lock bocor bila `Start` sukses tapi `Execute` tak pernah jalan (flush gagal, panic di handler) | `Abort` di handler untuk flush tidak didukung; `Execute` melepas lewat defer; goroutine `recover` |
| Event `progress` menyusul setelah `done` | handler menguras kanal event sebelum menulis event terakhir; sink dibuang setelah `done` |
| Proxy menahan stream | `X-Accel-Buffering: no`, heartbeat 15s, tes router nyata |
| Klien lambat membuat goroutine fetch menunggu | sink non-blocking (drop), mutex progress hanya melindungi counter |
| Perubahan ke ctx lepas pada jalur JSON mengubah perilaku timeout | `GitSyncTimeout` tetap membatasi; `ExtendTimeout` memberi cadangan 30s untuk `Start` dan tulis respons |
| Pembulatan detik membuat `Retry-After` header lebih besar 1 detik dari T14 | disengaja (ceil); tes `mapError` memperbarui ekspektasi |
