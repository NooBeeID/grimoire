# T17 / project/git-sync — Frontend Plan (progress SSE + status sync di server)

Tiket: `docs/tickets/T17-project-git-sync-progress.md`. Dokumen induk: `overview.md` (D1-D11, §11, §12), `usecase-runsync.md` (§3 Frontend F-1..F-7, §9 Visual), `usecase-getconfig.md` (Langkah 6-7, Visual). Plan ini **menyatukan** seluruh pekerjaan frontend kedua usecase menjadi satu urutan kerja; frontend tidak dipecah per usecase. Bila ada perbedaan, bagian "Keputusan FE" (§2.1) di dokumen ini yang berlaku untuk frontend dan alasannya dicatat.

Konteks yang dibaca: `CLAUDE.md` tidak ada; `.claude/rules/` hanya `migrations.md` (tidak relevan); `docs/tickets/README.md` bagian "Frontend (Svelte)" dan "Kontrak API"; `docs/superpowers/specs/2026-10-20-grimoire-design.md` (padanan `*knowledge-hub-design.md`); `DESIGN.md`; `.claude/skills/design-taste-frontend/references/app-screens.md` (§2.2, §11, §12, §13). Kode yang dibaca langsung: `frontend/src/domain/project/git-sync/{base,getconfig,runsync}/**`, `frontend/src/shared/services/http.ts`, `frontend/src/shared/components/Dialog.svelte`, `frontend/src/app.css` (token), `frontend/src/app/workspace/tabs.ts`, `frontend/src/app/views/workspace/{ApiSpecsTab.svelte,knowledge/KnowledgeDialogs.svelte}`, `frontend/src/shared/i18n/id.json`, `frontend/e2e/{git-sync.spec.ts,fixtures/fake-git-server.ts}`, `backend/.../gitsync/base/errors.go` (kode error).

Keputusan pemilik yang relevan untuk FE: jeda 500ms per request ke provider (env `GIT_SYNC_REQUEST_INTERVAL`, backend). FE tidak menampilkan jeda ini; FE hanya menampilkan tunggu rate limit (`waiting`) yang dilaporkan server.

---

## 1. Metadata

| Atribut | Nilai |
|---|---|
| Ticket | T17 (revisi T14) |
| Scope | fullstack (dokumen ini: bagian frontend) |
| Domain / Module | project / git-sync |
| Feature FE | `frontend/src/domain/project/git-sync/runsync` (revisi besar), `.../getconfig` (revisi kecil), `frontend/src/shared/services` (helper stream baru), `frontend/src/shared/components/Dialog.svelte` (prop aditif) |
| Route yang memuat fitur | `/projects/:projectID/knowledge` (`KnowledgeDialogs.svelte` -> `GitSyncDialog kind="knowledge"`), `/projects/:projectID/api-specs` (`ApiSpecsTab.svelte` -> `GitSyncDialog kind="api_spec"`). Tidak ada route baru; dialog dibuka dari tombol "Sync dari Git" di kedua tab. |
| Kontrak API | `api/modules/project/git-sync.yml` (`info.version` 0.1.0 -> 0.2.0, aditif) |
| Siapa | anggota project (`member`, `owner`, `admin`) dan super_admin |
| Bus | tidak ada (FE tidak menyentuh bus) |
| Migrasi | tidak ada |

| Endpoint | Method | Version | Pemakai FE | Perubahan yang dikonsumsi |
|---|---|---|---|---|
| `/api/projects/{projectID}/git-sync/run` | POST | `/api` tanpa prefix versi; spec 0.2.0 | `runsync/services/runsync.service.ts` | Kirim `Accept: text/event-stream`; respons 200 `text/event-stream` dengan event `start`, `progress`, `waiting`, `saving`, `done`, `error` (+ heartbeat `: ping`). Error sebelum stream tetap JSON berstatus HTTP (400/401/403/404/409/422/503). |
| `/api/projects/{projectID}/git-sync?kind=knowledge\|api_spec` | GET | `/api`; spec 0.2.0 | `getconfig/services/getconfig.service.ts` | Field aditif `in_progress: boolean` |

---

## 2. Summary Plan

Saat ini dialog Sync mengirim `POST .../run` lalu diam sampai laporan datang (timeout klien 130 detik), dan langsung gagal bila provider membatasi request. Setelah T17:

1. **Stream progress.** Service membuka `fetch` POST dengan `Accept: text/event-stream` (bukan `EventSource`, karena butuh POST + header CSRF + cookie sesi), mengurai SSE dengan parser kecil tanpa dependensi, memvalidasi tiap event dengan Zod, dan meneruskan event ke store. Idle timeout 45 detik (direset tiap byte, termasuk heartbeat) menggantikan `CLIENT_TIMEOUT_MS`.
2. **Store bertahap.** `runsync.store` punya fase `listing` (indeterminate) -> `fetching` (`n dari total`) -> `waiting` (hitung mundur) -> `saving` -> `done`/`error`. Status `error` membawa penanda `lost` bila stream terputus.
3. **Dialog menampilkan progress**, menggantikan form saat berjalan. Menunggu rate limit tampil sebagai status netral dengan hitung mundur, bukan error.
4. **Menutup saat berjalan** memunculkan konfirmasi "Sync tetap berjalan di server…". Menutup hanya memutus stream; server tetap jalan.
5. **Dibuka ulang saat sync masih di server**: `getconfig.in_progress === true` -> banner "Sync sedang berjalan di server", tombol Sync terkunci, aksi "Muat ulang status".
6. **Error final** memakai pesan T14; `RATE_LIMITED` kini punya tombol "Coba lagi" yang aktif setelah hitung mundur `retry_after_seconds` habis.
7. Laporan akhir (`ReportSummary`, `SyncReportTable`) tidak berubah.

### 2.1 Keputusan FE (mengikat implementasi frontend)

| ID | Keputusan | Alasan |
|---|---|---|
| FE1 | Satu plan, satu urutan: shared stream -> types -> service -> store -> getconfig -> komponen -> dialog -> i18n -> e2e. | Kedua usecase menyentuh `GitSyncDialog.svelte`; dikerjakan berurutan oleh satu pelaksana menghindari konflik. |
| FE2 | `runSync` (JSON) dihapus; satu-satunya jalur adalah `runSyncStream`. Bila server membalas `application/json` padahal diminta SSE (rolling deploy), service memperlakukannya sebagai event `done` (R9). | Satu jalur kode, tetap toleran server lama. |
| FE3 | Bar indeterminate (fase `listing`) = **track kosong `bg-raised` tanpa isian dan tanpa garis diagonal**, label kiri "Mengambil daftar file…", label kanan `–` abu, `role="progressbar"` tanpa `aria-valuenow`, `aria-busy="true"` pada blok. | Overview §12 menyebut "bergaris diagonal"; pola garis butuh `repeating-linear-gradient`, sedangkan DESIGN.md "Aturan wajib" melarang gradien dan lint design-taste menandainya. Track kosong + teks sudah menyatakan "belum ada angka" sesuai aturan nilai hilang = `–`. Ini satu-satunya penyimpangan dari overview, dicatat di sini. |
| FE4 | Saat berjalan, `Dialog` tetap `dismissible={false}` (Esc/backdrop tidak menutup diam-diam), tetapi tombol silang dan Esc **meminta konfirmasi** lewat prop aditif baru `onrequestclose` di `shared/components/Dialog.svelte`. | `usecase-runsync` F-6.3 meminta `dismissible` true + konfirmasi; dengan `<dialog>` native, Esc pada `dismissible` true langsung menutup elemen sebelum konfirmasi bisa tampil. Prop aditif memberi perilaku yang diminta tiket tanpa race. Pemakai `Dialog` lain tidak berubah (prop opsional). |
| FE5 | Hitung mundur dihitung dari jam klien: `until = Date.now() + retry_after_seconds*1000` saat event `waiting` diterima; ticker 1 detik hanya aktif pada fase `waiting`. | Server tidak mengirim ulang event tiap detik; hemat render. |
| FE6 | Selama `waiting`, `done/total/path` terakhir dipertahankan dan bar membeku; event berikutnya apa pun mengakhiri `waiting`. `waiting` sebelum `start` -> `total = null` dan bar tetap indeterminate. | K6 tiket: tahap listing pun bisa kena rate limit. |
| FE7 | Tidak ada polling otomatis untuk `in_progress`; user menekan "Muat ulang status". | Non-goal tiket (tanpa job/polling); dicatat di plan getconfig. |
| FE8 | Tidak ada tombol Batalkan; tidak menampilkan sisa jatah rate limit di luar fase menunggu. | D11 overview. |
| FE9 | Kode error kredensial yang dipakai FE adalah `CREDENTIAL_MISSING`/`CREDENTIAL_UNREADABLE` (sesuai `gitsync/base/errors.go`), bukan `GIT_CREDENTIAL_MISSING` yang tertulis di contoh overview §11.4. | Kode backend nyata; `SyncErrorBlock` sudah memetakan kode ini. |

---

## 3. Plan (langkah berurutan)

Semua path relatif ke `frontend/src/` kecuali disebut lain. Import lintas feature hanya lewat `index.ts`; komponen runsync hanya mengimpor `../../base`, `../../getconfig` (via `index.ts`), dan `shared`.

### L1. `shared/services/sse.ts` (baru) — parser SSE

```ts
export interface SseParser { push(chunk: string): void; end(): void }
export function createSseParser(onEvent: (name: string, data: string) => void): SseParser;
```

Aturan (spesifikasi WHATWG, subset): buffer baris parsial antar chunk; terminator `\n`, `\r\n`, `\r` (chunk yang berakhir `\r` ditahan sampai chunk berikut untuk membedakan `\r\n`); baris kosong = dispatch, hanya bila ada `data`; baris diawali `:` diabaikan (heartbeat); `event:` default `message`; beberapa `data:` digabung dengan `\n`; satu spasi setelah `:` dibuang; BOM di awal stream dibuang; `id`/`retry`/field lain diabaikan; `end()` membuang event yang belum ditutup baris kosong. Tanpa state global, tanpa dependensi.

### L2. `shared/services/http.ts` — `openStream`

```ts
export interface StreamOptions { body?: unknown; signal?: AbortSignal; headers?: Record<string, string> }
export async function openStream(method: string, url: string, o?: StreamOptions): Promise<Response>;
```

- Membangun request sama seperti `request`: `buildUrl`, `credentials: 'same-origin'`, `Content-Type: application/json` bila ada body, `CSRF_HEADER: CSRF_VALUE` untuk mutasi, plus `Accept: text/event-stream`.
- Kegagalan `fetch`: `AbortError`/`TimeoutError` dilempar apa adanya; lainnya -> `ApiError` kode `NETWORK` status 0 (sama dengan `request`).
- Non-2xx: `await res.text()` -> `toApiError(status, text, requestId)`; 401 memanggil `notifyUnauthorized()`; lalu throw. 2xx dikembalikan mentah (body belum dibaca).
- Ekspor lewat `shared/services/http.ts` saja (tidak menambah file index).

### L3. `domain/project/git-sync/runsync/types.ts` — skema event

Tambahan (yang lama tetap):

```ts
export const SyncStartEventSchema    = z.object({ total: z.number().int().min(0) });
export const SyncProgressEventSchema = z.object({ done: z.number().int().min(0), total: z.number().int().min(0), path: z.string() });
export const SyncWaitingEventSchema  = z.object({ retry_after_seconds: z.number().int().min(0), reason: z.string() });
export const SyncSavingEventSchema   = z.object({}).passthrough();
export const SyncErrorEventSchema    = z.object({ code: z.string(), message: z.string(), details: z.unknown().optional().nullable() });
// done = RunSyncResponseSchema (sudah ada)

export type SyncEvent =
  | { type: 'start'; total: number }
  | { type: 'progress'; done: number; total: number; path: string }
  | { type: 'waiting'; retryAfter: number; reason: string }
  | { type: 'saving' }
  | { type: 'done'; result: RunSyncResponse }
  | { type: 'error'; error: SyncError };

export function parseSyncEvent(name: string, data: string): SyncEvent | null; // nama tak dikenal, JSON rusak, skema gagal -> null
export function toSyncErrorFromEvent(e: z.infer<typeof SyncErrorEventSchema>): SyncError; // via new ApiError({status: 200, ...}) -> toSyncError
export function isStreamLost(e: SyncError): boolean; // (code NETWORK atau TIMEOUT) dan status 0
```

`reason` disimpan sebagai string (tidak enum) agar nilai baru dari server tidak membuat event dibuang; UI tidak membedakan teks per reason.

### L4. `runsync/services/runsync.service.ts` — `runSyncStream`

```ts
export const IDLE_TIMEOUT_MS = 45_000;
export async function runSyncStream(projectId: string, input: RunSyncInput, onEvent: (e: SyncEvent) => void, signal?: AbortSignal): Promise<void>;
```

1. Body sama dengan `runSync` lama (trim; `target_prefix` hanya untuk `knowledge`).
2. Gabungkan sinyal user dan timer idle ke satu `AbortController` (alasan abort dipertahankan: `AbortError` vs `TimeoutError`, seperti kode lama).
3. `openStream('POST', /api/projects/{id}/git-sync/run, { body, signal })`.
4. `Content-Type` diawali `application/json` -> parse `RunSyncResponseSchema` -> `onEvent({type:'done'})` -> selesai (FE2).
5. Selain itu: `res.body.getReader()` + `TextDecoder('utf-8', {stream:true})` -> `createSseParser` -> `parseSyncEvent` -> `onEvent`. Reset timer idle setiap chunk diterima.
6. Setelah event terminal (`done`/`error`), berhenti membaca (`reader.cancel()`), event sesudahnya diabaikan.
7. Stream selesai tanpa terminal -> throw `ApiError` `NETWORK` (dipetakan `lost`).
8. Hapus `runSync` dan `CLIENT_TIMEOUT_MS`; tidak ada pemakai lain (dicek: hanya store).

### L5. `runsync/progress.ts` (baru) — helper murni

```ts
export function remainingSeconds(until: number, now: number): number;   // max(0, ceil((until-now)/1000))
export function percentOf(done: number, total: number): number;         // total 0 -> 100; floor(done/total*100), dibatasi 0..100
export function truncateMiddle(path: string, max = 48): string;         // 'docs/…/login-flow.md'; selalu mempertahankan nama file utuh
export function announceBucket(seconds: number): number;                // ceil(seconds/5)*5, untuk aria-live tiap 5 detik
```

### L6. `runsync/stores/runsync.store.ts` — state bertahap

```ts
export type Progress =
  | { phase: 'listing' }
  | { phase: 'fetching'; done: number; total: number; path: string }
  | { phase: 'waiting'; done: number; total: number | null; path: string; retryAfter: number; until: number }
  | { phase: 'saving'; total: number };

export type RunState =
  | { status: 'idle' }
  | { status: 'running'; progress: Progress }
  | { status: 'done'; report: RunSyncResponse['report']; config: RunSyncResponse['config'] }
  | { status: 'error'; error: SyncError; lost: boolean };

export interface RunSyncStore extends Readable<RunState> {
  start(projectId: string, input: RunSyncInput): Promise<RunState>; // resolve saat terminal
  abort(): void;  // putus stream, kembali idle, server tetap jalan
  reset(): void;
}
export function createRunSyncStore(deps?: { now?: () => number }): RunSyncStore;
```

Transisi: `start()` -> `running/listing`; `start{total}` -> `fetching{done:0,total,path:''}`; `progress` -> `fetching` (abaikan bila `done` lebih kecil dari nilai sekarang, menjaga monoton); `waiting` -> `waiting` (warisi done/total/path; `total=null` bila masih `listing`; `until = now() + retryAfter*1000`); event lain mengakhiri `waiting`; `saving` -> `saving`; `done` -> `done`; `error` -> `error{lost:false}`; exception service -> `error{lost: isStreamLost(err)}` kecuali abort user (tetap `idle`). Event setelah terminal diabaikan. `start` saat sudah `running` mengembalikan snapshot (tanpa request kedua).

### L7. `getconfig/types.ts` dan `getconfig/stores/gitSyncConfig.store.ts`

1. `GetConfigResponseSchema` + `in_progress: z.boolean().default(false)` (server lama/mock tanpa field tetap valid).
2. `setFromRun`: hasil gabungan memuat `in_progress: false`.
3. `index.ts` getconfig tidak berubah (tipe ikut lewat `z.infer`). `base/mocks.ts`: `mockGetConfigResponse` mendapat `in_progress: false` default + opsi override.

### L8. `shared/components/Dialog.svelte` — prop aditif `onrequestclose`

```ts
onrequestclose?: () => void; // dipanggil saat dismissible=false dan user menekan Esc, tombol silang, atau backdrop
```

- `onCancel`: bila `!dismissible`, `preventDefault()` lalu `onrequestclose?.()`.
- Tombol silang: `disabled={!dismissible && !onrequestclose}`; klik saat `!dismissible` memanggil `onrequestclose` alih-alih `close()`.
- `onBackdrop`: saat `!dismissible`, panggil `onrequestclose?.()`.
- Tanpa prop ini perilaku identik dengan sekarang (regresi diuji).

### L9. `runsync/components/SyncProgress.svelte` (baru)

Props: `{ progress: Progress; now: number }`. Render lihat §Visual Layar A. `data-testid`: `sync-progress`, `sync-progress-bar`, `sync-progress-count`, `sync-progress-percent`, `sync-progress-path`, `sync-waiting`, `sync-saving`. `role="progressbar"` dengan `aria-valuemin=0`, `aria-valuemax={total}`, `aria-valuenow={done}`, `aria-valuetext="7 dari 42"`; fase listing tanpa `aria-valuenow`. Region `aria-live="polite"` (sr-only) berisi: satu pengumuman saat mulai menunggu (`gitSync.progress.waitingAnnounce`), lalu teks hitung mundur yang hanya berubah per `announceBucket` (tiap 5 detik).

### L10. `runsync/components/InProgressNotice.svelte` (baru)

Props: `{ loading: boolean; onreload: () => void }`. `role="status"`, `data-testid="sync-in-progress"`. Render lihat §Visual Layar D.

### L11. `runsync/components/CloseRunningConfirm.svelte` (baru)

Props: `{ open: boolean; onconfirm: () => void; onstay: () => void }`. `Dialog size="md"` dengan judul `gitSync.progress.closeTitle`, isi `closeBody`, footer: `Button variant="secondary"` "Tetap di sini" (fokus awal, `autofocus`) dan `Button variant="primary"` "Tutup". Esc/backdrop = "Tetap di sini". `data-testid="sync-close-confirm"`.

### L12. `runsync/components/SyncErrorBlock.svelte`

1. Tambah `RATE_LIMITED` ke `retryable`; tombol "Coba lagi" `disabled={(seconds ?? 0) > 0}`.
2. Prop opsional `onreloadstatus?: () => void`: bila `lost`, tampilkan teks `gitSync.progress.lost` (`text-warning`, ikon peringatan) + `Button variant="secondary" size="sm"` "Muat ulang status" alih-alih pesan `text-danger`. Prop `lost?: boolean`.

### L13. `runsync/views/GitSyncDialog.svelte`

1. State turunan baru:
   ```ts
   const running = $derived($run.status === 'running');
   const progress = $derived($run.status === 'running' ? $run.progress : null);
   const serverBusy = $derived(data?.in_progress === true && !running && $run.status !== 'done');
   let confirmClose = $state(false);
   let now = $state(Date.now());
   ```
2. `syncLocked`/`rerunLocked` ditambah `|| serverBusy`; `SyncForm disabled` ditambah `|| serverBusy`.
3. Ticker `now`: `$effect` memulai `setInterval(1000)` hanya saat `progress?.phase === 'waiting'`, dibersihkan saat fase berubah/`onDestroy`.
4. Isi saat `running`: `RepoOriginChip` (dari input yang dikirim; disimpan di variabel `runningOrigin` saat `start`) + `<SyncProgress {progress} {now} />`. Form, `ConfigSummary`, peringatan timpa, dan `runningHint` lama disembunyikan.
5. Isi saat tidak berjalan dan `serverBusy`: `<InProgressNotice loading={$cfg.status === 'loading'} onreload={reloadStatus} />` di atas `SyncForm` (setelah `ConfigSummary`/`CredentialNotice`).
6. `reloadStatus()`: `run.reset(); filled = false; void cfg.load(projectId, kind);`. `$effect` dengan `wasBusy` (variabel biasa, `untrack`): transisi `in_progress` true -> false dengan `config.last_status !== 'failed'` memanggil `onsynced()` sekali (guard `announced`).
7. Error: `SyncErrorBlock` menerima `lost={$run.error.lost}`, `onreloadstatus={reloadStatus}`, `seconds={cooldown}`, `onretry={() => start(false)}`; efek cooldown `RATE_LIMITED` yang ada tetap dipakai (kini dari event `error` maupun JSON).
8. Tutup: `Dialog dismissible={!running} onrequestclose={() => (confirmClose = true)}`. Footer saat berjalan hanya `Button variant="ghost"` "Tutup" -> `confirmClose = true`. `CloseRunningConfirm onconfirm`: `run.abort(); toast.push({type:'info', message: $t('gitSync.cancelledToast')}); confirmClose = false; onclose();`. `onstay`: `confirmClose = false`.
9. Sync selesai (`done`): perilaku T14 (`cfg.setFromRun`, `onsynced` bila bukan `failed`). Bila `done` tiba saat konfirmasi terbuka, konfirmasi ditutup otomatis (tidak ada lagi yang perlu diperingatkan).
10. Saat dialog dibuka/ditutup: alur reset yang ada dipertahankan (`run.reset`, `cfg.load`), ditambah `confirmClose = false`.

### L14. `runsync/index.ts`

Tetap `GitSyncDialog`, `RunSyncInput`, `RunSyncResponse`; tambah ekspor tipe `SyncEvent` hanya bila dipakai di luar feature (saat ini tidak; jangan ditambah).

### L15. i18n `shared/i18n/id.json` dan `en.json`

| Kunci | id | en |
|---|---|---|
| `gitSync.progress.listing` | Mengambil daftar file… | Fetching file list… |
| `gitSync.progress.count` | {done} dari {total} | {done} of {total} |
| `gitSync.progress.percent` | {percent}% | {percent}% |
| `gitSync.progress.label` | Progress sync | Sync progress |
| `gitSync.progress.waiting` | Menunggu batas request provider, lanjut dalam {seconds} detik | Waiting for provider rate limit, resuming in {seconds} s |
| `gitSync.progress.waitingAnnounce` | Sync menunggu batas request provider. | Sync is waiting for the provider rate limit. |
| `gitSync.progress.saving` | Menyimpan… | Saving… |
| `gitSync.progress.lost` | Koneksi ke server terputus. Sync mungkin masih berjalan di server. | Connection to the server was lost. The sync may still be running on the server. |
| `gitSync.progress.reloadStatus` | Muat ulang status | Reload status |
| `gitSync.progress.closeTitle` | Tutup dialog sync? | Close sync dialog? |
| `gitSync.progress.closeBody` | Sync tetap berjalan di server. Hasilnya bisa dilihat saat dialog dibuka lagi. | The sync keeps running on the server. You can see the result when you reopen this dialog. |
| `gitSync.progress.stay` | Tetap di sini | Stay here |
| `gitSync.inProgress.title` | Sync sedang berjalan di server | Sync is running on the server |
| `gitSync.inProgress.body` | Sync dimulai dari sesi lain dan tetap berjalan sampai selesai. Muat ulang status untuk melihat hasilnya. | The sync was started from another session and keeps running until it finishes. Reload the status to see the result. |
| `gitSync.inProgress.reload` | Muat ulang status | Reload status |
| `gitSync.runningHint` (ubah) | Sync berjalan di server; progress tampil di sini. | The sync runs on the server; progress is shown here. |

Dipakai ulang: `gitSync.errors.RATE_LIMITED`, `gitSync.errors.RATE_LIMITED_RETRY`, `gitSync.cancelledToast`, `gitSync.close`, `common.retry`. Tes paritas kunci id/en yang ada harus tetap hijau.

### L16. E2E fixture dan spec

1. `frontend/e2e/fixtures/fake-git-server.ts`: opsi per repo `rateLimit: { afterRequests: number; resetSeconds: number }` (kirim `x-ratelimit-remaining: 0` + `x-ratelimit-reset` epoch), `status429Once: boolean` (`Retry-After: 1`), `blobDelayMs: number`.
2. Env server e2e (global-setup): `GIT_SYNC_REQUEST_INTERVAL=0s`, `GIT_SYNC_MAX_WAIT=3s`, `GIT_SYNC_MAX_TOTAL_WAIT=5s`.
3. `frontend/e2e/git-sync.spec.ts`: skenario §6.

### L17. Verifikasi

`npm run test`, `npm run check` (svelte-check), `npm run depcruise`, `npm run test:arch`, `node .agents/skills/design-taste-frontend/scripts/lint.mjs --app frontend/src`, `npx playwright test e2e/git-sync.spec.ts`. Inspect visual (`inspect.mjs --app http://localhost:5173/projects/<id>/knowledge`) bila backend dev jalan; screenshot 1440 dan 390.

### Kontrak bus

Tidak ada. Frontend hanya memakai REST di atas.

---

## 4. Migration

Tidak ada (frontend). Backend juga tanpa migrasi (overview §7).

---

## 5. Changes (per file/folder)

| File | Jenis | Isi |
|---|---|---|
| `frontend/src/shared/services/sse.ts` | baru | `createSseParser` |
| `frontend/src/shared/services/sse.test.ts` | baru | tes parser |
| `frontend/src/shared/services/http.ts` | ubah | `openStream`, `StreamOptions` |
| `frontend/src/shared/services/http.test.ts` | ubah | tes `openStream` |
| `frontend/src/shared/components/Dialog.svelte` | ubah | prop opsional `onrequestclose` |
| `frontend/src/shared/components/components.test.ts` | ubah | tes `onrequestclose` + regresi |
| `frontend/src/domain/project/git-sync/runsync/types.ts` (+ `types.test.ts`) | ubah | skema event, `SyncEvent`, `parseSyncEvent`, `toSyncErrorFromEvent`, `isStreamLost` |
| `.../runsync/progress.ts` (+ `progress.test.ts`) | baru | `remainingSeconds`, `percentOf`, `truncateMiddle`, `announceBucket` |
| `.../runsync/services/runsync.service.ts` (+ test) | ubah | `runSyncStream`, `IDLE_TIMEOUT_MS`; hapus `runSync`, `CLIENT_TIMEOUT_MS` |
| `.../runsync/stores/runsync.store.ts` (+ test) | ubah | `Progress`, `RunState` bertahap, `lost` |
| `.../runsync/components/SyncProgress.svelte` | baru | bar progress, menunggu, menyimpan |
| `.../runsync/components/InProgressNotice.svelte` | baru | banner sync berjalan di server |
| `.../runsync/components/CloseRunningConfirm.svelte` | baru | konfirmasi tutup |
| `.../runsync/components/SyncErrorBlock.svelte` | ubah | `RATE_LIMITED` retryable dengan cooldown, mode `lost` |
| `.../runsync/components/components.test.ts` | ubah | tes komponen baru |
| `.../runsync/views/GitSyncDialog.svelte` (+ `GitSyncDialog.test.ts`) | ubah | fase progress, konfirmasi, banner, muat ulang |
| `.../getconfig/types.ts` (+ test) | ubah | `in_progress` default false |
| `.../getconfig/stores/gitSyncConfig.store.ts` (+ test) | ubah | `setFromRun` -> `in_progress: false` |
| `.../base/mocks.ts` | ubah | fixture `in_progress`, event SSE contoh |
| `frontend/src/shared/i18n/{id,en}.json` | ubah | kunci §L15 |
| `frontend/e2e/fixtures/fake-git-server.ts` | ubah | rate limit, 429, delay |
| `frontend/e2e/git-sync.spec.ts` | ubah | skenario T17 |
| `frontend/e2e/global-setup.ts` | ubah | env pacing e2e |

Tidak disentuh: `app/routes/**`, `app/workspace/tabs.ts`, `ApiSpecsTab.svelte`, `KnowledgeDialogs.svelte` (props `GitSyncDialog` tidak berubah), `app.css`, `DESIGN.md`.

---

## 6. Test Scenario

### 6.1 Unit (vitest)

`sse.test.ts`
1. Satu event utuh dalam satu chunk -> `onEvent('start', '{"total":3}')`.
2. Chunk terpotong di tengah `event:`, di tengah JSON, dan di antara dua `\n` -> hasil sama dengan chunk utuh.
3. `\r\n` dan `\r` terpisah antar chunk -> tidak menghasilkan event kosong ganda.
4. `: ping` diabaikan; event tanpa `data` tidak di-dispatch.
5. `data:` dua baris -> digabung `\n`. BOM di awal dibuang. Beberapa event dalam satu chunk -> urutan dijaga. `end()` membuang sisa.

`http.test.ts` (`openStream`)
6. Header `Accept: text/event-stream`, `X-Requested-With: grimoire`, `Content-Type`, `credentials: same-origin`.
7. 409 body envelope -> `ApiError` `SYNC_IN_PROGRESS` dengan `details.kind`; 401 memanggil handler unauthorized; fetch gagal -> `NETWORK`; abort -> `AbortError` dilempar apa adanya.

`types.test.ts` (runsync)
8. Tiap skema event valid/invalid (`total: -1`, `done: "1"` ditolak).
9. `parseSyncEvent('unknown', ...)` -> `null`; JSON rusak -> `null`; `done` dengan `RunSyncResponse` valid -> `{type:'done'}`.
10. `toSyncErrorFromEvent({code:'RATE_LIMITED', message, details:{retry_after_seconds:1712}})` -> `retryAfter 1712`; `SYNC_LIMIT_EXCEEDED` -> `limit` + `field 'folder'`; `details: null` aman.
11. `isStreamLost`: `NETWORK`/`TIMEOUT` status 0 -> true; `RATE_LIMITED` -> false.

`progress.test.ts`
12. `remainingSeconds(until, now)` 38 -> 37 -> … -> 0, tidak negatif; pembulatan ke atas (37.2 s -> 38).
13. `percentOf(7,42) = 16`, `percentOf(0,0) = 100`, `percentOf(43,42) = 100`.
14. `truncateMiddle('docs/very/long/nested/folder/structure/auth/login-flow.md', 32)` mempertahankan `login-flow.md` dan awal `docs/`; path pendek tidak diubah.
15. `announceBucket(38) = 40`, `announceBucket(35) = 35`.

`runsync.service.test.ts` (fetch palsu dengan `ReadableStream`)
16. Event diteruskan berurutan `start, progress x3, saving, done`; reader di-cancel setelah `done`.
17. Idle timeout dengan fake timers: tanpa byte 45 detik -> error `TIMEOUT`; heartbeat tiap 15 detik mencegah timeout.
18. Non-2xx (409, 422 `CREDENTIAL_MISSING`, 401) -> throw `ApiError` dengan kode benar sebelum event apa pun.
19. Respons `application/json` (FE2) -> satu event `done`.
20. Stream berakhir tanpa terminal -> `NETWORK`. Abort user -> `AbortError` (bukan TIMEOUT).
21. Body request: `target_prefix` hanya untuk `knowledge`, nilai di-trim.

`runsync.store.test.ts`
22. `listing -> fetching(0/42) -> fetching(7/42) -> waiting(38) -> fetching(8/42) -> saving -> done`.
23. `waiting` sebelum `start` -> `total: null`, lalu `start` -> `fetching`.
24. `progress` mundur (done 5 setelah 7) diabaikan.
25. `error` event `RATE_LIMITED` -> `error{lost:false, retryAfter}`; exception NETWORK -> `lost:true`.
26. Event setelah `done` diabaikan; `abort()` saat berjalan -> `idle` tanpa error; `start` ganda -> satu request.

`getconfig` tests
27. Respons tanpa `in_progress` -> `false`; `true` diteruskan; `"yes"` ditolak.
28. `setFromRun` setelah load `in_progress:true` -> `false`.

`components.test.ts`
29. `SyncProgress` listing: teks "Mengambil daftar file…", label kanan `–`, progressbar tanpa `aria-valuenow`, tanpa kelas `bg-accent` di isian.
30. `SyncProgress` fetching 7/42: "7 dari 42", "16%", path mono terpotong dengan `title` path penuh, `aria-valuenow=7`.
31. `SyncProgress` waiting (now dimajukan 3 detik): "lanjut dalam 35 detik", isian berkelas `bg-border` (bukan `bg-danger`/`text-danger`), ikon jam ada; sr-only hanya berubah di kelipatan 5.
32. `SyncProgress` saving: "Menyimpan…", bar 100%.
33. `InProgressNotice`: judul, tombol "Muat ulang status" memanggil `onreload`; `loading` menonaktifkan tombol; `role="status"`.
34. `CloseRunningConfirm`: fokus awal di "Tetap di sini"; Esc memanggil `onstay`.
35. `SyncErrorBlock` `RATE_LIMITED` dengan `seconds=12` -> "Coba lagi" disabled; `seconds=0` -> aktif; mode `lost` menampilkan "Muat ulang status".
36. `Dialog`: tanpa `onrequestclose` dan `dismissible=false` tombol silang disabled (regresi); dengan `onrequestclose` Esc/silang memanggilnya dan dialog tetap terbuka.

`GitSyncDialog.test.ts` (service dan getconfig dimock)
37. Klik Sync -> form hilang, `sync-progress` muncul dengan fase listing; event mengalir -> `n dari total` -> laporan T14; `onsynced` dipanggil sekali.
38. Fase waiting menampilkan hitung mundur yang turun dengan fake timers; ticker berhenti setelah fase berubah (tidak ada interval tersisa).
39. Klik "Tutup" saat berjalan -> konfirmasi; "Tetap di sini" menutup konfirmasi saja; "Tutup" memanggil `abort`, toast info, `onclose`; Esc saat berjalan -> konfirmasi (bukan menutup).
40. `done` saat konfirmasi terbuka -> konfirmasi tertutup, laporan tampil.
41. Event `error` `RATE_LIMITED` 1712 -> pesan "coba lagi dalam 1712 detik", tombol "Coba lagi" disabled sampai 0, lalu klik memicu request baru.
42. Stream lost -> pesan `lost` + "Muat ulang status" -> `cfg.load` dipanggil, run kembali idle.
43. getconfig `in_progress:true` -> banner, tombol Sync & Sync ulang disabled, field form disabled, klik Sync tidak mengirim request.
44. Muat ulang -> `in_progress:false` + `last_status:'success'` -> banner hilang, `onsynced` sekali; `last_status:'failed'` -> tidak memanggil `onsynced`; masih `true` -> banner tetap.
45. `config: null` + `in_progress: true` -> banner tampil, form kosong terkunci.
46. Error 409 `SYNC_IN_PROGRESS` dari JSON sebelum stream -> `SyncErrorBlock` T14 (regresi). 422 `CREDENTIAL_MISSING` -> `CredentialNotice`.

### 6.2 Arsitektur dan lint
47. `npm run depcruise`, `npm run test:arch` (`arch.git-sync.test.ts`): komponen baru hanya mengimpor `../../base`, `../../getconfig` (index), `shared`.
48. `lint.mjs --app frontend/src` hijau (tanpa gradien, warna baru, radius baru).

---

## 7. E2E Scenarios (Playwright, `frontend/e2e/git-sync.spec.ts`)

Env e2e: `GIT_SYNC_REQUEST_INTERVAL=0s`, `GIT_SYNC_MAX_WAIT=3s`, `GIT_SYNC_MAX_TOTAL_WAIT=5s`. Repo fake unik per tes.

Positif
1. Sync knowledge 5 file di `/projects/:id/knowledge`: "Mengambil daftar file…" -> `n dari 5` + nama file mono -> "Menyimpan…" -> laporan per file identik T14; pohon Knowledge memuat file baru.
2. `rateLimit {afterRequests:2, resetSeconds:2}`: "Menunggu batas request provider, lanjut dalam 2 detik" muncul, angka turun, bar tidak berwarna error, sync selesai sukses.
3. `status429Once`: sukses, laporan sama dengan tanpa 429.
4. Sync API spec di `/projects/:id/api-specs`: alur progress sama, laporan menampilkan jumlah endpoint.
5. Tutup saat berjalan (`blobDelayMs`): konfirmasi -> "Tutup"; buka lagi -> banner "Sync sedang berjalan di server", Sync nonaktif; setelah server selesai, "Muat ulang status" -> banner hilang, ringkasan hasil terakhir dari `last_result`, daftar file terisi.
6. Sync pertama (tanpa config) lambat, dialog tab lain: banner tampil tanpa `ConfigSummary`.
7. Kind terpisah: sync `knowledge` berjalan, dialog `api_spec` project sama tidak menampilkan banner dan bisa Sync.
8. Klien lama: `request.post` tanpa `Accept: text/event-stream` -> JSON T14 (200 `report` + `config`).

Negatif
9. `rateLimit {resetSeconds:60}` (> MAX_WAIT 3s): event `error` `RATE_LIMITED`, `retry_after_seconds` 60; UI pesan rate limit + hitung mundur, "Coba lagi" nonaktif; tidak ada file baru.
10. Dua sesi menekan Sync bersamaan: sesi kedua menerima 409 `SYNC_IN_PROGRESS` JSON; UI menampilkan pesan T14, bukan progress.
11. Token belum disimpan: 422 `CREDENTIAL_MISSING` sebelum stream; `CredentialNotice` tampil.
12. Repo/folder tidak ada: event `error` `GIT_NOT_FOUND` -> pesan di field URL/folder seperti T14.
13. Reload halaman saat sync berjalan: sync tetap selesai di server; dialog dibuka ulang menampilkan hasil dari `last_result`.
14. Rekaman stream mentah tidak memuat token canary maupun isi file; `in_progress` hanya boolean.
15. Non-anggota: `GET .../git-sync` -> 404, `POST .../run` -> 404 JSON (tanpa stream); tanpa login -> 401 dan FE mengarah ke login.
16. Muat ulang getconfig gagal 500: banner dan data sebelumnya dipertahankan, pesan error tampil, tidak crash.

---

## 8. Request/Response body

### 8.1 Request run (dari `runSyncStream`)

```http
POST /api/projects/0b7e5a52-3c1f-4d1e-9a51-6f1f0c2d9e10/git-sync/run
Content-Type: application/json
Accept: text/event-stream
X-Requested-With: grimoire
Cookie: <sesi>

{"kind":"knowledge","url":"https://github.com/noobee/grimoire-docs/tree/main/docs","branch":"","folder":"","target_prefix":"handbook"}
```

API spec: tanpa `target_prefix`: `{"kind":"api_spec","url":"https://gitlab.com/noobee/payments-api/-/tree/main/openapi","branch":"","folder":""}`.

### 8.2 Respons SSE sukses (200)

```
HTTP/1.1 200 OK
Content-Type: text/event-stream; charset=utf-8
Cache-Control: no-store
X-Accel-Buffering: no

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

Pemetaan FE: `start` -> `fetching 0/3`; `progress` -> `fetching`; `waiting` -> `waiting` (38 detik); heartbeat -> hanya reset idle timer; `saving` -> `saving`; `done` -> `done`.

### 8.3 Respons SSE gagal setelah mulai (200, event terakhir `error`)

```
event: start
data: {"total":120}

event: progress
data: {"done":61,"total":120,"path":"docs/api/errors.md"}

event: error
data: {"code":"RATE_LIMITED","message":"provider Git membatasi permintaan; coba lagi nanti","details":{"retry_after_seconds":1712}}

```

FE: `error{code:'RATE_LIMITED', retryAfter:1712, lost:false}`; cooldown 1712 detik; "Coba lagi" nonaktif sampai 0. Kode lain yang mungkin: `GIT_TOKEN_REJECTED`, `GIT_ACCESS_DENIED`, `GIT_NOT_FOUND`, `GIT_TREE_TOO_LARGE`, `SYNC_LIMIT_EXCEEDED`, `GIT_UPSTREAM_ERROR`, `TIMEOUT`, `CANCELED`, `INVALID_ARGUMENT`, `INTERNAL` (`"message":"terjadi kesalahan internal","details":null`).

### 8.4 Error sebelum stream (JSON; dipetakan `openStream` -> `ApiError` -> `toSyncError`)

```http
HTTP/1.1 409 Conflict
Content-Type: application/json

{"error":{"code":"SYNC_IN_PROGRESS","message":"sync lain untuk project dan jenis ini sedang berjalan","details":{"kind":"knowledge"}},"trace_id":"7f3c2a9e4b1d4c08"}
```

```http
HTTP/1.1 422 Unprocessable Entity
Content-Type: application/json

{"error":{"code":"CREDENTIAL_MISSING","message":"token Git untuk host ini belum disimpan","details":{"provider":"github","host":"github.com","settings_path":"/profile#git-credentials"}},"trace_id":"7f3c2a9e4b1d4c08"}
```

```http
HTTP/1.1 400 Bad Request
Content-Type: application/json

{"error":{"code":"INVALID_REPO_URL","message":"URL repo tidak valid","details":{"field":"url"}},"trace_id":"7f3c2a9e4b1d4c08"}
```

(Teks `message` mengikuti konstanta `gitsync/base/errors.go`; FE menampilkan teks i18n berdasarkan `code`, `message` hanya fallback.) 404 `NOT_FOUND` -> dialog ditutup dan `onprojectgone` dipanggil (perilaku T14). 401 -> `notifyUnauthorized`.

### 8.5 Jalur JSON (toleransi FE2)

Server membalas `Content-Type: application/json` 200 `{"report":{...},"config":{...}}` (bentuk §8.2 event `done`) -> FE mengubahnya jadi satu event `done`.

### 8.6 getconfig

```http
GET /api/projects/0b7e5a52-3c1f-4d1e-9a51-6f1f0c2d9e10/git-sync?kind=knowledge
```

```json
{"kind":"knowledge","config":{"provider":"github","host":"github.com","repo_url":"https://github.com/noobee/grimoire-docs","repo_path":"noobee/grimoire-docs","branch":"main","folder":"docs","target_prefix":"handbook","last_status":"success","last_synced_at":"2026-10-22T08:14:03Z","last_synced_by":{"id":"5d2c9a7e-1b44-4f0b-8c1e-2a9d7f3e6b21","label":"Reyhan"},"last_result":null},"has_credential":true,"credential_unreadable":false,"feature_enabled":true,"in_progress":true}
```

Sync pertama berjalan: `{"kind":"api_spec","config":null,"has_credential":false,"credential_unreadable":false,"feature_enabled":true,"in_progress":true}`.

Error (tanpa `in_progress`):

```json
{"error":{"code":"NOT_FOUND","message":"project tidak ditemukan","details":null},"trace_id":"7f3c2a9e4b1d4c08"}
```

---

## 9. Visual

Sumber: `DESIGN.md` (token, "Wajah produk" #1 dan #3, "Aturan wajib": status tidak hanya warna, gerak hanya umpan balik 120ms, tanpa gradien/animasi masuk, nilai hilang `–`) dan `app-screens.md` (§2.2 satu pertanyaan per layar, §11 loading/error di dalam blok yang gagal + "Limits and usage: a bar at the true ratio with both ends labelled", §12 ponsel, §13 larangan).

Semua layar ada di dalam `Dialog size="lg"` yang sudah ada (sheet `bg-surface`, `rounded-sheet`, `border-border`, `shadow-pop`, header/footer dengan rule `border-rule`). Tidak ada layar/route baru.

### Layar A — Dialog Sync, sedang berjalan

1. **Pertanyaan:** "Sudah sampai mana sync ini, dan apakah sedang menunggu?" Blok terbesar: blok progress (bar + label dua ujung + nama file terakhir) yang menggantikan form.
2. **Pola dan kerangka:** pola detail sederhana di dalam dialog (bukan list/settings). Urutan: `RepoOriginChip` (asal repo, sudah ada) -> blok progress -> satu kalimat status. Form, peringatan timpa, dan tombol Sync disembunyikan; footer hanya tombol ghost "Tutup".
3. **Elemen khas di layar pertama:**
   - Bar 8px (`h-2`), `rounded-full` (hanya untuk bar ini, lihat catatan DESIGN.md di bawah), track `bg-raised`, isian `bg-accent` (aksen kuning untuk isian, Wajah produk #3), lebar = `percentOf`, `transition-[width] duration-120 ease-out`, `motion-reduce:transition-none`.
   - Label dua ujung di atas bar: kiri `7 dari 42` (`text-sm text-fg`), kanan `16%` (`text-sm text-muted`), keduanya `tabular-nums`, angka kanan rata kanan.
   - Di bawah bar: nama file terakhir dalam IBM Plex Mono (`font-mono text-xs text-muted`, 12px), dipotong di tengah (`docs/…/login-flow.md`), `title` berisi path penuh. Mono di sini sesuai Wajah produk #1 (path file knowledge).
   - **Listing (FE3):** track kosong, label kiri "Mengambil daftar file…", label kanan `–` (`text-muted`), tanpa animasi.
   - **Menunggu:** lebar bar membeku, isian berubah ke `bg-border`; baris status `text-warning text-[13px]` dengan ikon jam 16px outline (`stroke="currentColor"`, `aria-hidden`) + "Menunggu batas request provider, lanjut dalam 38 detik". Tidak ada merah, tidak ada spinner.
   - **Menyimpan:** bar penuh `bg-accent`, label kiri "Menyimpan…", kanan `100%`.
4. **Token/utility:** `bg-surface`, `bg-raised`, `bg-accent`, `bg-border`, `text-fg`, `text-muted`, `text-warning`, `font-mono`, `rounded-full` (bar), `rounded-base` (tombol), `ring-accent-ink` (fokus), `gap-3`/`gap-4` sesuai dialog yang ada. Tanpa hex baru, tanpa shadow tambahan.
5. **Data contoh:** project "Grimoire Docs", repo `noobee/grimoire-docs` branch `main` folder `docs`, total 42, done 7 (16%), path `docs/auth/login-flow.md` lalu `docs/onboarding/setup-local.md`, tunggu 38 detik, reason `rate_limit_exhausted`. Angka yang muncul dua kali (42 di label dan `aria-valuemax`) berasal dari satu variabel `total`.

### Layar B — Konfirmasi tutup saat berjalan

1. **Pertanyaan:** "Kalau saya tutup sekarang, apakah sync-nya batal?" Blok terbesar: satu kalimat jawaban "Sync tetap berjalan di server. Hasilnya bisa dilihat saat dialog dibuka lagi."
2. **Pola:** `Dialog size="md"` kedua (dialog native bertumpuk di top layer), judul satu baris, satu kalimat, footer dua tombol.
3. **Elemen khas:** tombol utama near-black "Tutup" (`Button variant="primary"`, satu aksi utama per layar), sekunder "Tetap di sini" dengan fokus awal (aksi aman). Tanpa warna bahaya karena menutup tidak merusak data.
4. **Token:** `Button` primary/secondary standar, `text-muted` untuk kalimat.
5. **Data contoh:** judul "Tutup dialog sync?" untuk kind Knowledge project "Grimoire Docs".

### Layar C — Dialog Sync, hasil dan error

1. **Pertanyaan:** "Apa yang berubah, dan bila gagal, kapan boleh coba lagi?" Blok terbesar: `SyncReportTable` (sukses) atau `SyncErrorBlock` (gagal).
2. **Pola:** sama dengan T14 (detail: chip asal + `StatusBadge` + `ReportSummary` + tabel panel `rounded-card shadow-panel`). Tidak berubah.
3. **Elemen khas:** path repo/tujuan mono di tabel (T14). Error rate limit: `text-danger` satu baris "Provider Git membatasi permintaan; coba lagi dalam 1712 detik." + `Button variant="secondary" size="sm"` "Coba lagi" nonaktif sampai hitung mundur 0 (satu tanda merah per objek, sesuai §11). Stream terputus: `text-warning` + ikon peringatan + "Muat ulang status" (bukan merah: hasilnya belum tentu gagal).
4. **Token:** `text-danger`, `text-warning`, `text-muted` (trace id), `Button` secondary.
5. **Data contoh:** summary `2 dibuat, 1 diperbarui, 0 tidak berubah, 0 dilewati, 0 ditolak` (total 3 = jumlah baris), `retry_after_seconds` 1712, trace id `7f3c2a9e4b1d4c08`.

### Layar D — Dialog Sync dibuka ulang saat sync masih di server

1. **Pertanyaan:** "Apakah sync ini masih jalan, dan apa yang bisa saya lakukan?" Blok terbesar: banner `InProgressNotice` di atas form yang terkunci.
2. **Pola:** pesan inline netral sejajar `CredentialNotice` (pola settings/form dialog T14); bukan error, bukan peringatan.
3. **Elemen khas:** blok `rounded-card bg-pill-neutral-bg p-3`, ikon jam 16px, judul 14px medium `text-pill-neutral-ink` "Sync sedang berjalan di server", satu kalimat 13px `text-muted`, tombol `Button variant="secondary" size="sm"` "Muat ulang status" di kanan (di ponsel turun ke baris sendiri). Tombol Sync dan Sync ulang nonaktif; tidak ada spinner berputar.
4. **Token:** `bg-pill-neutral-bg`, `text-pill-neutral-ink`, `text-muted`, `rounded-card`, `rounded-base`. Tanpa aksen kuning (aksen milik isian progress).
5. **Data contoh:** kind `knowledge`, repo `noobee/grimoire-docs`, `in_progress: true`, `last_synced_by` "Reyhan" pada `ConfigSummary`.

### State loading / empty / error (ringkas)

| State | Tampilan |
|---|---|
| Loading config saat buka dialog | bar abu datar `h-5 w-2/3 rounded-base bg-raised` (sudah ada), tanpa shimmer |
| Listing (belum ada total) | Layar A fase listing (track kosong, `–`) |
| Empty (`total = 0`) | `start{total:0}` lalu `done`: laporan T14 "Tidak ada file di folder ini." (`gitSync.report.empty`) di dalam bingkai tabel |
| Menunggu rate limit | Layar A fase menunggu (warning, bukan error) |
| Error sebelum stream | `SyncErrorBlock`/`CredentialNotice`/error field form (T14) |
| Error dalam stream | Layar C |
| Stream putus | Layar C mode `lost` |
| Gagal muat ulang status | `ErrorState` di dalam blok config, data sebelumnya dipertahankan |

### Ponsel (390px)

Dialog sudah `w-full max-w-2xl` dengan margin; label dua ujung tetap satu baris (`7 dari 42` / `16%`); path dipotong lebih pendek (`truncateMiddle` max 32 saat lebar < 480 lewat `matchMedia`, atau CSS `max-w-full` + `title`); banner menumpuk tombol di bawah teks; target sentuh tombol >= 32px (komponen `Button` yang ada). Tidak ada scroll horizontal.

### Catatan usulan perubahan DESIGN.md (tidak diterapkan diam-diam)

- DESIGN.md menulis "Radius: kontrol 6, kartu 8, sheet 16. Tidak ada radius lain." Bar progress memakai `rounded-full` (sudah dipakai untuk pil/avatar bulat dan dipakai overview §12). Usulan: tambahkan satu baris di DESIGN.md "Token": "Bar rasio (progress, pemakaian): tinggi 8px, `rounded-full`, track `bg-raised`, isian `bg-accent`; saat jeda isian `bg-border`." Sampai disetujui, implementasi memakai `rounded-full` hanya pada elemen bar ini dan perubahan DESIGN.md diajukan di PR yang sama untuk ditinjau manusia.
- Bar indeterminate bergaris dari overview tidak dipakai (FE3); tidak perlu perubahan DESIGN.md.

---

## 10. Urutan implementasi

1. L1 `sse.ts` -> L2 `openStream` -> L3 types -> L4 service -> L5 helpers -> L6 store (semua bisa dikerjakan dengan mock stream sebelum backend selesai).
2. L7 getconfig -> L8 Dialog prop -> L9-L12 komponen -> L13 dialog -> L15 i18n.
3. L16 e2e setelah backend U1/U2 hijau.
4. L17 verifikasi + design check.

## 11. Risiko FE

| Risiko | Mitigasi |
|---|---|
| Chunk SSE terpotong sembarang | parser berbasis buffer + tes chunk terpotong |
| Interval hitung mundur bocor | ticker hanya di fase waiting, dibersihkan di `onDestroy` dan saat fase berubah; tes fake timers |
| User mengira menutup = batal | konfirmasi eksplisit + toast `cancelledToast` + banner saat dibuka ulang |
| Server lama membalas JSON | FE2 |
| Pembaca layar dibanjiri hitung mundur | `aria-live` per 5 detik + satu pengumuman awal |
