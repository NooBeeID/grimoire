# T14 core/git-credential + project/git-sync — Frontend Plan

Tiket: `docs/tickets/T14-project-git-sync.md`. Dokumen induk (menang bila bertentangan): `docs/superpowers/plans/project-git-sync/overview.md` (bagian 4, 6.6, 6.8, 9, 11, 12). Kontrak per endpoint: `usecase-{listcredentials,savecredential,deletecredential,getconfig,runsync}.md`. Kontrak API: `api/modules/core/git-credential.yml`, `api/modules/project/git-sync.yml` (dibuat FOUNDATION).

Konteks yang dibaca: `CLAUDE.md` (tidak ada), `.claude/rules/` (hanya `migrations.md`, tidak relevan untuk FE), `docs/tickets/README.md` (bagian Frontend), `docs/superpowers/specs/2026-10-20-grimoire-design.md` (padanan `*knowledge-hub-design.md`, yang tidak ada), `DESIGN.md`, `.claude/skills/design-taste-frontend/references/app-screens.md` (bagian 7, 8, 11, 12, 13), serta kode FE yang ada: `frontend/.dependency-cruiser.cjs`, `shared/services/http.ts` (`get/put/post/del`, `CSRF_VALUE = 'grimoire'`), `shared/components/{Dialog,Button,Table,EmptyState,ErrorState,CopyField,Spinner,Select,Input}`, `shared/utils/{datetime,clipboard}.ts`, `app/views/ProfileView.svelte`, `app/views/workspace/knowledge/{KnowledgeTab,KnowledgeSidebar,KnowledgeMain,KnowledgeDialogs}.svelte`, `app/views/workspace/ApiSpecsTab.svelte`, `domain/project/api-spec/listspecs/views/ListSpecsView.svelte`, `domain/project/api-spec/uploadspecs/components/StatusBadge.svelte`, `domain/core/audit-log/listentries/components/AuditTable.svelte` (gaya chip agent).

Frontend tidak dipecah per usecase. Plan ini satu-satunya rujukan FE untuk seluruh feature; potongan FE di plan usecase dianggap draf dan diselaraskan oleh bagian 3 (Keputusan FE).

---

## 1. Metadata

| Atribut | Nilai |
|---|---|
| Ticket | T14 |
| Scope | frontend (bagian dari fullstack T14) |
| Domain / Module utama | project / git-sync (`frontend/src/domain/project/git-sync`) |
| Module kedua | core / git-credential (`frontend/src/domain/core/git-credential`) |
| Module FE yang disentuh (aditif) | project / api-spec `listspecs` (slot aksi header), komposisi `app/` |
| Route FE | `/profile` (seksi baru `#token-git`), `/projects/:id/knowledge` (tombol + dialog), `/projects/:id/api-specs` (tombol + dialog). Tidak ada route baru. |
| Migrasi | tidak ada (FE) |

| Endpoint | Method | Version | Dipakai oleh feature FE |
|---|---|---|---|
| `/api/me/git-credentials` | GET | `/api` tanpa prefix versi; `api/modules/core/git-credential.yml` `info.version` 0.1.0 | `core/git-credential/listcredentials` |
| `/api/me/git-credentials/{provider}` | PUT | sama | `core/git-credential/savecredential` |
| `/api/me/git-credentials/{provider}?host=` | DELETE | sama | `core/git-credential/deletecredential` |
| `/api/projects/{projectID}/git-sync?kind=` | GET | `/api`; `api/modules/project/git-sync.yml` `info.version` 0.1.0 | `project/git-sync/getconfig` |
| `/api/projects/{projectID}/git-sync/run` | POST | sama | `project/git-sync/runsync` |

`previewsync` tidak dibuat (Pertanyaan terbuka 1, default tiket). Bus (`core.gitcredential.get-token`, `project.knowledge.upsert-files`, `project.apispec.upsert-specs`) tidak terlihat dari FE.

---

## 2. Summary Plan

1. **Token Git di Profil.** `/profile` mendapat seksi "Token Git" (`id="token-git"`). Seksi ini berisi tabel status PAT per `(provider, host)`, dialog Simpan/Ganti, dan dialog Hapus. Token hanya hidup di `$state` lokal dialog selama diketik. Token tidak masuk store, `localStorage`, URL, atau DOM teks, dan respons server tidak pernah memuatnya (Zod `.strict()` menolak field asing).
2. **Sync dari Git.** Tombol sekunder "Sync dari Git" ditambahkan di header Knowledge (sidebar file) dan header API Spec. Tombol membuka satu dialog `GitSyncDialog` (prop `kind`) milik module `project/git-sync`. Saat dibuka, dialog memuat `getconfig` untuk mengisi form dan menampilkan status token/fitur. Ada dua tindakan: **Sync** (dengan nilai form) dan **Sync ulang** (dengan config tersimpan). Hasilnya laporan per file. Setelah sync selesai dengan status selain `failed`, `onsynced` memuat ulang pohon/daftar.
3. **Batas module.** `git-sync` tidak meng-import `git-credential`. Status token di dialog berasal dari `getconfig` (`has_credential`, `credential_unreadable`, `feature_enabled`) dan dari error `CREDENTIAL_MISSING`/`CREDENTIAL_UNREADABLE`. Tautan ke `/settings#token-git` diambil dari `details.settings_path`, dengan fallback konstanta.
4. **Visual.** Mengikuti DESIGN.md (framed-sheet). Tidak ada token, warna, radius, atau komponen dekoratif baru. Dua usulan perubahan DESIGN.md dicatat terpisah (bagian 10) dan tidak diterapkan diam-diam.

---

## 3. Keputusan FE (penyelarasan antar plan usecase)

| # | Topik | Keputusan | Alasan |
|---|---|---|---|
| F1 | Skema bersama git-sync | Feature baru `domain/project/git-sync/base/` (precedent FE `domain/project/knowledge/base`) memuat Zod `GitSyncConfig`, `GitSyncReport`, `GitSyncReportItem`, `GitSyncIssue`, enum `Kind`, util `reasonKey`, `originLabel`, serta komponen presentasi bersama (`StatusBadge`, `RepoOriginChip`, `ConfigSummary`, `SyncReportTable`). `getconfig` dan `runsync` mengimpornya lewat `base/index.ts`. | `usecase-getconfig.md` dan `usecase-runsync.md` sama-sama mendefinisikan `GitSyncConfigSchema`. Duplikasi dilarang. Import antar feature lewat `index.ts` diizinkan depcruise (`no-cross-feature-internals`). |
| F2 | Pemilik dialog | `GitSyncDialog` ada di `runsync/views/` dengan prop sesuai overview 9 (`kind`, `projectId`, `open`, `onclose`, `onsynced`). Dialog sendiri memuat config lewat `getconfig/index.ts` (`createGitSyncConfigStore`). | Overview menang atas draf runsync (yang mengoper `config/hasCredential` sebagai prop). Dengan begitu dua komposer di `app/` cukup satu baris dan tidak menduplikasi logika muat. Satu module, jadi import feature-ke-feature lewat index sah. |
| F3 | Penamaan file | Mengikuti pola repo: `types.ts` (Zod + tipe), `services/<feature>.service.ts`, `stores/<nama>.store.ts` (Svelte `writable`, bukan `store.svelte.ts`), `components/`, `views/`, `mocks/`, `index.ts`. | Konsisten dengan `api-key/listkeys`, `knowledge/uploadfiles`. Nama `schema.ts`/`service.ts`/`store.svelte.ts` di draf getconfig diganti. |
| F4 | Penyegaran daftar token | `ProfileView` (app/) mengomposisi `GitTokensSection`, `SaveTokenDialog`, dan `DeleteCredentialDialog`, lalu memanggil `gitCredentials.invalidate()` dari `listcredentials/index.ts` setelah `onsaved`/`ondeleted`. `savecredential` dan `deletecredential` tidak mengimpor store. | Menyatukan draf listcredentials (`invalidate`) dan deletecredential (`refresh` oleh section). Satu nama: `invalidate()`. |
| F5 | Tombol Sync saat config belum ada | Bila `config === null`, `has_credential` selalu `false` (getconfig tidak memanggil bus tanpa provider/host). Karena itu tombol **Sync** TIDAK dinonaktifkan karena `has_credential` saat `config === null`. Pengecekan token sync pertama terjadi di server (`CREDENTIAL_MISSING` tampil inline). Tombol dinonaktifkan hanya bila `feature_enabled=false`, atau bila `config !== null && (!has_credential \|\| credential_unreadable)`. Dalam kondisi kedua itu, user yang mengubah URL ke host lain tetap bisa menekan Sync setelah mengedit field (lihat F6). | Menghindari dialog yang terkunci permanen pada sync pertama (kontrak `usecase-getconfig.md` baris 42). |
| F6 | Kunci tombol saat token tidak ada | `syncLocked = feature_enabled === false \|\| (config !== null && !has_credential && formMatchesConfigHost)`. `formMatchesConfigHost` = host hasil parse ringan URL form sama dengan `config.host`. **Sync ulang** terkunci bila `!has_credential \|\| credential_unreadable \|\| !feature_enabled`. | Kriteria 8 dan E2E negatif 2/3 (tombol nonaktif untuk anggota tanpa PAT) tetap terpenuhi tanpa memblokir sync ke host lain. |
| F7 | Warna/kelas di draf savecredential | Draf menyebut tombol utama `accent`, `bg-surface-2`, dan `text-negative`. Diganti menjadi tombol utama `bg-primary` (near-black, sesuai DESIGN.md), `bg-raised`, dan `text-danger`. | DESIGN.md: aksen kuning bukan untuk tombol; token yang ada `raised`/`danger`. |
| F8 | Lebar dialog Sync | `Dialog size="lg"` (`max-w-2xl` = 672px), bukan 720px. | Overview menulis "max 720"; tidak boleh menambah ukuran baru. 672px memuat 4 kolom laporan dengan path dipotong ellipsis. |
| F9 | Chip asal-repo | `RepoOriginChip` memakai gaya chip netral: `rounded-base bg-raised text-fg font-mono text-[13px]` + ikon cabang outline 16px `text-muted`. Kuning tidak dipakai. | DESIGN.md "Wajah produk" #3/#4 membatasi kuning pada ubin logo, tab aktif, chip agent di audit, dan chart. Memakai gaya chip agent untuk repo akan mengaburkan sinyal "manusia vs agent". Overview minta "gaya chip agent", jadi ini dicatat sebagai usulan DESIGN.md (bagian 10, U1), tidak diterapkan diam-diam. |
| F10 | Header CSRF | Ditangani `shared/services/http.ts` (`X-Requested-With: grimoire`). Contoh overview yang menulis `XMLHttpRequest` tidak berlaku untuk FE. | Kode yang ada. |
| F11 | Waktu | `formatDateTime(iso, locale)` dari `shared/utils/datetime.ts` (absolut). Tidak ada util waktu relatif; tidak ditambah. | Tidak ada util relatif di repo; overview "relatif" diganti absolut agar tidak menambah util. |
| F12 | Batal saat sync berjalan | Tombol "Batal" saat `running` membatalkan fetch (`AbortController`) dan menutup dialog. Toast info menyatakan "Sync tetap diselesaikan server; buka lagi untuk melihat hasilnya." Saat dibuka lagi, `getconfig` dimuat ulang. | Server tidak bisa dibatalkan (runsync memakai `finCtx`). User perlu tahu hasil tetap tercatat. |
| F13 | Lokasi tombol Knowledge | Tab Knowledge tidak punya `PageHeader`; baris aksinya ada di `KnowledgeSidebar` (Upload primer `sm`, File baru sekunder `sm`). "Sync dari Git" ditambahkan di baris itu sebagai `secondary sm`, dengan `flex-wrap`. Juga ditambahkan di empty state `KnowledgeMain` (secondary di samping Upload). | Struktur FE yang nyata (Pertanyaan terbuka 2 dicek terhadap FE). |
| F14 | Lokasi tombol API Spec | `ListSpecsView` (domain `api-spec/listspecs`) mendapat prop snippet opsional `headeractions` yang dirender sebelum tombol Upload di header dan di `EmptyState`. `ApiSpecsTab` (app/) mengisinya dengan tombol Sync. | Domain api-spec tidak boleh tahu git-sync; slot generik menjaga batas (komposisi di app/). |

---

## 4. Route, komposisi, dan struktur folder

### 4.1 Route

Tidak ada route baru. Tabel `app/routes/index.ts` tidak berubah.

| Route | Perubahan | Guard |
|---|---|---|
| `/profile` | seksi `#token-git` + 2 dialog | `auth: true` (ada) |
| `/projects/:id/knowledge` | tombol Sync di `KnowledgeSidebar` + empty state, dialog di `KnowledgeDialogs` | guard workspace (ada) |
| `/projects/:id/api-specs` | tombol Sync di header `ListSpecsView` lewat slot, dialog di `ApiSpecsTab` | guard workspace (ada) |

Anchor `#token-git`: `router.ts` sudah membawa `url.hash`. `ProfileView` membaca `location.hash`. Bila hash = `#token-git`, setelah `GitTokensSection` memanggil `onready`, view memanggil `scrollIntoView({block:'start'})` dan memfokuskan `h2#token-git-title` (`tabindex="-1"`). Ini dijalankan pada mount dan saat `$current.location` berubah (navigasi in-app dari dialog Sync).

### 4.2 Struktur folder

```
frontend/src/domain/core/git-credential/
├── listcredentials/
│   ├── index.ts                 # GitTokensSection, gitCredentials, createGitCredentialsStore, hasCredential, buildRows; type GitCredential, GitCredentialList, TokenRow, GitProvider
│   ├── types.ts (+ types.test.ts)          # Zod strict, buildRows, maskLast4, providerLabel
│   ├── mocks/credentials.ts
│   ├── services/listcredentials.service.ts (+ .test.ts)
│   ├── stores/gitCredentials.store.ts (+ .test.ts)
│   ├── components/{ProviderCell.svelte, GitTokensTable.svelte, components.test.ts}
│   └── views/GitTokensSection.svelte (+ .test.ts)
├── savecredential/
│   ├── index.ts                 # SaveTokenDialog; type SavedCredential, SaveTarget
│   ├── types.ts (+ types.test.ts)          # Zod, validateToken, validateHost, scopeFor
│   ├── services/savecredential.service.ts (+ .test.ts)
│   ├── components/{SaveTokenForm.svelte, ScopeNote.svelte, components.test.ts}
│   └── views/SaveTokenDialog.svelte (+ .test.ts)
└── deletecredential/
    ├── index.ts                 # DeleteCredentialDialog, deleteCredential; type DeleteTarget
    ├── types.ts
    ├── services/deletecredential.service.ts (+ .test.ts)
    └── views/DeleteCredentialDialog.svelte (+ .test.ts)

frontend/src/domain/project/git-sync/
├── base/
│   ├── index.ts                 # skema + tipe + util + komponen presentasi bersama
│   ├── types.ts (+ types.test.ts)          # Kind, GitSyncConfig, GitSyncReport(+Item, Issue, Summary), SETTINGS_PATH
│   ├── url.ts (+ url.test.ts)              # parseRepoUrlLite (hanya UX: auto-isi branch/folder, deteksi host)
│   ├── reason.ts (+ reason.test.ts)        # reasonKey(kind, code), errorKey(code)
│   ├── mocks.ts                 # data contoh bagian 9.5
│   └── components/{StatusBadge.svelte, RepoOriginChip.svelte, ConfigSummary.svelte, SyncReportTable.svelte, ReportSummary.svelte, components.test.ts}
├── getconfig/
│   ├── index.ts                 # getConfig, createGitSyncConfigStore; type GetConfigResponse, ConfigState
│   ├── types.ts (+ types.test.ts)          # GetConfigResponseSchema
│   ├── services/getconfig.service.ts (+ .test.ts)
│   └── stores/gitSyncConfig.store.ts (+ .test.ts)
└── runsync/
    ├── index.ts                 # GitSyncDialog; type RunSyncInput, RunSyncResponse
    ├── types.ts (+ types.test.ts)          # RunSyncInput, RunSyncResponseSchema, SyncError, toSyncError
    ├── services/runsync.service.ts (+ .test.ts)
    ├── stores/runsync.store.ts (+ .test.ts)
    ├── components/{SyncForm.svelte, CredentialNotice.svelte, SyncErrorBlock.svelte, components.test.ts}
    └── views/GitSyncDialog.svelte (+ .test.ts)
```

Komposisi `app/` (diubah):
- `app/views/ProfileView.svelte` (+ `ProfileView.test.ts` baru bila belum ada)
- `app/views/workspace/knowledge/KnowledgeTab.svelte`, `KnowledgeSidebar.svelte`, `KnowledgeMain.svelte`, `KnowledgeDialogs.svelte` (+ `KnowledgeTab.test.ts`)
- `app/views/workspace/ApiSpecsTab.svelte` (+ `ApiSpecsTab.test.ts`)
- `domain/project/api-spec/listspecs/views/ListSpecsView.svelte` (prop snippet `headeractions`, aditif)

Arah import (diuji depcruise dan `test-arch`):
- `git-sync/{getconfig,runsync}` → `git-sync/base/index.ts` (boleh); `runsync` → `getconfig/index.ts` (boleh).
- `git-sync/**` ↛ `domain/core/git-credential/**`, `domain/project/{knowledge,api-spec}/**`.
- `git-credential/{savecredential,deletecredential}` ↛ `listcredentials` (store di-invalidate oleh `app/`).
- `app/**` hanya mengimpor `index.ts` feature.

---

## 5. Kontrak tipe (Zod, snake_case dari API)

Semua skema respons dari server memakai `.strict()` untuk objek yang terkait token (git-credential) agar field tak terduga, termasuk `token`, menjadi `INVALID_RESPONSE` dan tidak masuk store. Skema git-sync cukup memakai `z.object` default (tanpa `.strict()`/`.passthrough()`; field ekstra dibuang saat parse).

### 5.1 `git-credential/listcredentials/types.ts`

```ts
export const GIT_PROVIDERS = ['github', 'gitlab'] as const;
export type GitProvider = (typeof GIT_PROVIDERS)[number];
export const GitCredentialSchema = z.object({
  provider: z.enum(GIT_PROVIDERS),
  host: z.string().min(1),
  last4: z.string().min(1).max(4),
  created_at: z.string().datetime({ offset: true }),
  updated_at: z.string().datetime({ offset: true }),
}).strict();
export const GitCredentialListSchema = z.object({
  enabled: z.boolean(),
  allowed_gitlab_hosts: z.array(z.string().min(1)),
  items: z.array(GitCredentialSchema),
}).strict();
export type TokenRow = { provider: GitProvider; host: string; saved: GitCredential | null };
export function buildRows(items: GitCredential[]): TokenRow[];
// GitHub (github.com) selalu ada; GitLab satu baris per host tersimpan; tanpa GitLab sama sekali -> placeholder gitlab.com.
export const maskLast4 = (l: string) => `…${l}`;
export const providerLabel = (p: GitProvider) => (p === 'github' ? 'GitHub' : 'GitLab');
```

### 5.2 `git-credential/savecredential/types.ts`

```ts
export const SavedCredentialSchema = z.object({
  provider: z.enum(['github', 'gitlab']), host: z.string().min(1), last4: z.string().min(1).max(4),
  updated_at: z.string().datetime({ offset: true }), replaced: z.boolean(),
}).strict();
export type SaveTarget = { provider: GitProvider | null; host: string | null; gitlabHosts: string[] };
export function validateToken(raw: string): 'length' | 'charset' | null;   // trim; 8-512; ASCII 0x21-0x7E
export function scopeFor(p: GitProvider): string[];                        // github ['repo'], gitlab ['read_api','read_repository']
```
Body: GitHub `{ token }`; GitLab `{ host, token }`. Token di-`trim()` sebelum dikirim.

### 5.3 `git-credential/deletecredential/types.ts`

`type DeleteTarget = { provider: GitProvider; host: string; last4: string }`. Service: GitHub tanpa `?host`, GitLab selalu `?host=`.

### 5.4 `git-sync/base/types.ts`

```ts
export const KINDS = ['knowledge', 'api_spec'] as const;
export type Kind = (typeof KINDS)[number];
export const SETTINGS_PATH = '/settings#token-git';
export const ITEM_STATUSES = ['created', 'updated', 'unchanged', 'skipped', 'rejected'] as const;
export const GitSyncIssueSchema = z.object({ code: z.string(), message: z.string(), pointer: z.string(), line: z.number().int() });
export const GitSyncReportItemSchema = z.object({
  repo_path: z.string(), target: z.string(), status: z.enum(ITEM_STATUSES),
  reason_code: z.string(), reason: z.string(), issues: z.array(GitSyncIssueSchema),
  openapi_version: z.string().optional(), title: z.string().optional(), endpoint_count: z.number().int().optional(),
});
export const GitSyncSummarySchema = z.object({ created: z.number().int(), updated: z.number().int(), unchanged: z.number().int(), skipped: z.number().int(), rejected: z.number().int() });
export const GitSyncReportSchema = z.object({
  status: z.enum(['success', 'partial', 'failed']), summary: GitSyncSummarySchema,
  items: z.array(GitSyncReportItemSchema), truncated: z.boolean(), error_code: z.string(), error_message: z.string(),
});
export const GitSyncConfigSchema = z.object({
  provider: z.enum(['github', 'gitlab']), host: z.string(), repo_url: z.string(), repo_path: z.string(),
  branch: z.string(), folder: z.string(), target_prefix: z.string(),
  last_status: z.enum(['success', 'partial', 'failed']).nullable(),
  last_synced_at: z.string().datetime({ offset: true }).nullable(),
  last_synced_by: z.object({ id: z.string(), label: z.string() }).nullable(),
  last_result: GitSyncReportSchema.nullable(),
});
```

### 5.5 `git-sync/getconfig/types.ts`

`GetConfigResponseSchema = z.object({ kind: z.enum(KINDS), feature_enabled: z.boolean(), has_credential: z.boolean(), credential_unreadable: z.boolean(), config: GitSyncConfigSchema.nullable() })`.

### 5.6 `git-sync/runsync/types.ts`

```ts
export type RunSyncInput = { kind: Kind; url: string; branch: string; folder: string; target_prefix?: string };
export const RunSyncResponseSchema = z.object({ report: GitSyncReportSchema, config: GitSyncConfigSchema });
export type SyncError = {
  code: string; message: string; status: number; traceId?: string;
  field?: 'url' | 'branch' | 'folder' | 'target_prefix';   // dari details.field / details.fields
  retryAfter?: number;                                      // details.retry_after_seconds atau header
  settingsPath?: string;                                    // details.settings_path (default SETTINGS_PATH untuk CREDENTIAL_*)
  limit?: { files: number; max_files: number; known_bytes: number; max_total_bytes: number };
};
export function toSyncError(e: unknown): SyncError;  // ApiError -> SyncError; AbortError -> code 'ABORTED'; lainnya -> NETWORK
```
`target_prefix` hanya dikirim untuk `kind=knowledge`. String di-`trim()`.

---

## 6. Service, store, dan state

### 6.1 Services (tanpa state; validasi Zod; error = `ApiError` dari `shared/services/http.ts`)

| Fungsi | Panggilan |
|---|---|
| `listCredentials(signal?)` | `get('/api/me/git-credentials', { schema: GitCredentialListSchema, signal })` |
| `saveCredential(provider, body)` | `put('/api/me/git-credentials/' + enc(provider), body, { schema: SavedCredentialSchema })` |
| `deleteCredential(target)` | `del('/api/me/git-credentials/' + enc(provider), { query: provider==='gitlab' ? { host } : undefined })` |
| `getConfig(projectId, kind, signal?)` | `get('/api/projects/' + enc(projectId) + '/git-sync', { query: { kind }, schema: GetConfigResponseSchema, signal })` |
| `runSync(projectId, input, signal?)` | `post('/api/projects/' + enc(projectId) + '/git-sync/run', body, { schema: RunSyncResponseSchema, signal: any([signal, AbortSignal.timeout(130_000)]) })`. Bila `AbortSignal.any` tidak ada, gabungkan manual. Timeout klien 130 s > `GIT_SYNC_TIMEOUT` 120 s. |

Tidak ada service yang me-log body. `saveCredential` tidak menerima `signal` (pola `createKey`; membatalkan PUT yang sedang memverifikasi tidak bermakna).

### 6.2 Stores

**`gitCredentials` (listcredentials/stores/gitCredentials.store.ts)**: state `idle | loading{userId, previous} | ready{userId, data} | error{userId, error, previous}`. Aksinya `begin/resolve/fail/invalidate/reset`, plus `version` (Readable<number>, naik saat `invalidate`). Hasil untuk `userId` basi diabaikan. Saat `session.identity` berubah atau logout, `reset()` dipanggil (subscribe di `GitTokensSection`). Store ini tidak pernah menyimpan token.

**`createGitSyncConfigStore()` (getconfig/stores/gitSyncConfig.store.ts)**: satu instance per dialog. State `idle | loading{previous} | ready{data} | error{error, previous}`. `load(projectId, kind)` membatalkan load sebelumnya lewat `AbortController` dan tidak menduplikasi load untuk kunci yang sama. `setFromRun(config)` memperbarui `data.config` setelah run sukses tanpa fetch ulang; `last_result` diisi dari `report` karena respons run mengirim `config.last_result = null` (overview 12.5). `reset()` dipanggil saat dialog ditutup.

**`createRunSyncStore()` (runsync/stores/runsync.store.ts)**: satu instance per dialog.

```
idle ──start()──▶ running ──200──▶ done{report, config}
  ▲                  │ ──error──▶ error{SyncError}
  │                  └─abort()──▶ idle (toast F12)
  └──────reset()─────┘ (dari done/error, atau "Ubah lalu sync lagi")
```
- `start(projectId, input)` diabaikan bila sedang `running` (guard klik ganda).
- `done` dengan `report.status === 'failed'` tidak terjadi lewat 200 (gagal total = error HTTP). Bila tetap terjadi, ditampilkan sebagai hasil `failed` tanpa memanggil `onsynced`.
- Store tidak menyimpan URL berisi kredensial. URL form dengan userinfo ditolak klien (6.3) sebelum dikirim.

### 6.3 Validasi klien (UX saja; server otoritatif)

`parseRepoUrlLite(raw)` di `git-sync/base/url.ts`:
- Kosong menghasilkan `required`. Bukan `https://` menghasilkan `https_only`. Ada userinfo (`user@`) menghasilkan `userinfo`, dan nilai input **dikosongkan** dari state laporan error. Ada `?`/`#` menghasilkan `query`.
- `github.com/{o}/{r}/tree/{b}/{folder}` mengisi otomatis Branch dan Folder **hanya bila field itu belum disentuh user** (`touched` per field). GitLab `/-/tree/{b}/{folder}` diperlakukan sama.
- Mengembalikan `{ provider, host, repoPath, branch?, folder? }` untuk `RepoOriginChip` pratinjau dan F6.
- `/blob/` menghasilkan `blob_url` ("gunakan URL folder (tree)").

Branch ber-`/` tetap ambigu di URL, jadi field Branch eksplisit selalu dikirim apa adanya bila diisi. Teks bantu di bawah Branch berbunyi: "Isi bila nama branch memuat / (mis. feature/x)."

---

## 7. Komponen dan view

### 7.1 git-credential

| Komponen | Props | Perilaku |
|---|---|---|
| `ProviderCell` | `provider` | Ikon outline 16px `currentColor` (SVG inline generik: GitHub = lingkaran-cabang, GitLab = cabang; bukan logo merek) + nama. |
| `GitTokensTable` | `rows, loading, error, onretry, canEdit, onsave?, ondelete?` | Membungkus `shared/Table`. Kolom: Provider, Host (mono), Status, Diperbarui (rata kanan, `formatDateTime`), Aksi. Tombol aksi hanya bila `canEdit` dan callback ada. `aria-label` memuat provider + host. |
| `GitTokensSection` (view) | `onsave?(target), ondelete?(target), onadd?(target), onready?()` | Memuat `listCredentials` dengan `AbortController`, subscribe `gitCredentials.version`, lalu merender judul `h2#token-git-title`, satu kalimat intro, tabel, tautan "Tambah host GitLab" (bila ada host yang diizinkan dan belum tersimpan), dan catatan scope. |
| `ScopeNote` | `provider` | Blok `bg-raised rounded-base` berisi scope mono (`repo`; `read_api` + `read_repository`). |
| `SaveTokenForm` | `provider?, host?, gitlabHosts, disabled, errors, onsubmit` | Provider `Select` (terkunci bila diberikan). Host `Select` dari `gitlabHosts` (hanya GitLab). Token `Input type="password" autocomplete="off" spellcheck="false" name="git-token"` berfont mono. `ScopeNote`. Catatan "Token tidak bisa dilihat lagi setelah disimpan." |
| `SaveTokenDialog` (view) | `open, target: SaveTarget, onclose, onsaved(SavedCredential)` | Token hanya di `$state` lokal, dikosongkan saat tutup dan saat sukses. Saat submit: `validateToken`, lalu `saveCredential`, lalu `onsaved`, tutup, dan toast. Selama request: tombol `loading` + teks "Memverifikasi ke GitHub…", `dismissible=false`. Fokus awal di Token. |
| `DeleteCredentialDialog` (view) | `open, target: DeleteTarget\|null, onclose, ondeleted` | Baris ringkas: ikon + provider + host mono + `…7Qx2`. Kalimat konsekuensi. Tombol `danger` "Hapus". 404 menutup dialog, menampilkan toast "Token sudah tidak ada", lalu memanggil `ondeleted`. |

### 7.2 git-sync

| Komponen | Props | Perilaku |
|---|---|---|
| `StatusBadge` (base) | `status` (run: `success\|partial\|failed`; item: 5 status) | Pil `h-5 rounded-full px-2 text-xs font-medium`. success/created = positive; partial = warning; failed/rejected = negative; updated/skipped = neutral; unchanged = neutral + teks `text-muted`. Teks selalu ada; pil tidak `shrink` (`flex-none whitespace-nowrap`). |
| `RepoOriginChip` (base) | `host, repoPath, branch, folder` | `github.com/noobee/grimoire-docs@main:docs/`. Mono 13px, `rounded-base bg-raised`, ikon cabang 16px `text-muted`, ellipsis di tengah repo (tooltip `title` berisi teks penuh), dan tombol salin ikon 12px. |
| `ReportSummary` (base) | `summary, kind` | Satu baris divided (hairline `border-rule`): label 12px `text-muted` di atas angka 16px `tabular-nums`, dengan urutan dibuat · diperbarui · tidak berubah · dilewati · ditolak. Nol ditulis `0`. Tidak ada ikon atau warna. Angka ditolak > 0 berwarna `text-danger` (satu tanda merah). |
| `ConfigSummary` (base) | `config, kind` | Bila `last_synced_at` null: "Belum pernah sync" (abu). Bila ada: "Terakhir {formatDateTime} oleh {label}" + `StatusBadge`. Bila status failed: satu kalimat `error_message` terjemahan dari `errorKey(last_result.error_code)`. Ringkasan angka dari `last_result.summary` (teks inline kecil). `RepoOriginChip` dari config. |
| `SyncReportTable` (base) | `items, kind, truncated, total` | Panel tabel (`shared/Table`, radius 8, `shadow-panel`, baris 48) dengan kolom Path repo (mono, identitas terlebar), Tujuan (mono; api_spec = nama spec + `endpoint_count` endpoint sebagai sufiks redup; kosong = `–`), Status (`StatusBadge`), Alasan (teks + tombol ikon salin). Baris `rejected` dengan `issues` punya tombol chevron `aria-expanded` yang membuka sub-baris berisi tiap issue: `code` mono, `message`, `pointer` mono, `line` bila > 0. Tinggi maks 360px dengan scroll internal, header `sticky`. Urutan: rejected → created → updated → skipped → unchanged, lalu `repo_path`. `truncated` menambah satu baris catatan: "Menampilkan {n} dari {total} file; ringkasan dihitung dari semua file." |
| `SyncForm` (runsync) | `kind, values, touched, errors, disabled, onchange` | Field URL (mono, wajib), Branch, Folder, dan "Simpan di folder" (hanya knowledge, placeholder "/ (root)", teks bantu "Folder di Grimoire tempat file disimpan."). Label di atas field, kontrol 32px. Galat per field lewat `aria-invalid` + `aria-describedby`. |
| `CredentialNotice` (runsync) | `state: 'missing'\|'unreadable'\|'disabled', provider, host, settingsPath` | Kotak catatan `bg-pill-warning-bg text-pill-warning-ink rounded-card` (pola notice app-screens 11) berisi satu kalimat + tautan "Atur Token Git" (`text-accent-ink underline-offset-2 hover:underline`) ke `settingsPath`. Untuk `disabled`: tanpa tautan, teks admin + `GIT_CREDENTIAL_KEY` mono. |
| `SyncErrorBlock` (runsync) | `error: SyncError, onretry?` | `role="alert"`. Pesan terjemahan `gitSync.errors.<KODE>` (fallback `error.message`). Detail: `SYNC_LIMIT_EXCEEDED` menampilkan "{files} dari maks {max_files} file"; `RATE_LIMITED` menampilkan hitung mundur `retryAfter`; `CREDENTIAL_*` merender `CredentialNotice`; `trace_id` 12px `text-muted`; tombol "Coba lagi" sekunder bila `onretry` ada. |
| `GitSyncDialog` (view, runsync) | `open, projectId, kind, onclose, onsynced` | Lihat 7.3. |

### 7.3 `GitSyncDialog`: alur dan state

Saat `open` menjadi true: `cfg.load(projectId, kind)`, `run.reset()`, dan form diisi dari `config` (URL = `repo_url`, Branch = `branch`, Folder = `folder`, prefix = `target_prefix`) bila ada; `touched` di-reset.

| State dialog | Isi | Footer |
|---|---|---|
| Memuat config | Form dengan field `disabled` dan skeleton satu baris di slot `ConfigSummary` (bar abu setinggi teks, tanpa shimmer) | Batal (secondary) |
| Gagal memuat config | `ErrorState` kecil di slot ringkasan + "Coba lagi". Form tetap dapat diisi manual (config tidak wajib untuk sync). | Batal, Sync |
| Form (config ada) | `ConfigSummary` di atas, `SyncForm`, peringatan timpa (`text-warning` 13px + ikon info outline): "File bernama sama akan ditimpa. File yang tidak ada di repo tidak dihapus.", `CredentialNotice` bila F6 | Batal (ghost), Sync ulang (secondary, terkunci per F6), Sync (primary) |
| Form (config belum ada) | Satu kalimat abu "Belum pernah sync untuk {Knowledge/API Spec} project ini." di slot ringkasan, `SyncForm` kosong, dan peringatan timpa | Batal, Sync |
| Fitur nonaktif (`feature_enabled=false`) | `CredentialNotice state=disabled`, form `disabled` tetapi tetap terbaca | Tutup |
| Berjalan | Field `disabled`. Tombol Sync `loading` dengan teks "Menarik dari GitHub…" (provider dari parse URL atau config). Di bawah form: satu baris abu "Sync berjalan di server; bisa sampai 2 menit." | Batal (abort, F12) |
| Galat run | `SyncErrorBlock` di atas form. Field yang ditunjuk (`details.field`/`fields`) diberi galat inline, form tetap terisi | Batal, Sync ulang, Sync |
| Hasil | `RepoOriginChip` (dari `config` respons), `StatusBadge` keseluruhan, `ReportSummary`, `SyncReportTable` | "Sync lagi" (secondary, kembali ke state Form via `run.reset()`), Tutup (primary) |

Aturan:
- Tombol Sync aktif bila URL lolos `parseRepoUrlLite`, tidak `running`, tidak `syncLocked` (F6).
- Sync ulang mengirim `{kind, url: config.repo_url, branch: config.branch, folder: config.folder, target_prefix: config.target_prefix}` tanpa membaca form.
- Setelah `done` dengan status `success`/`partial`, dialog memanggil `onsynced()` tepat sekali per run dan `cfg.setFromRun(config, report)`. Dialog tetap terbuka.
- `Esc`/klik backdrop menutup dialog kecuali saat `running`. Saat `running`, hanya tombol Batal yang menutup (dengan abort).
- Tidak ada token di DOM/state dialog.
- Peran `aria-live="polite"` pada wadah hasil/galat agar pembaca layar mengumumkan "Sync selesai: 2 dibuat, 1 dilewati".

### 7.4 Komposisi `app/`

**ProfileView.svelte**
- Kontainer luar `mx-auto flex max-w-2xl flex-col gap-6`. Blok identitas + `ChangePasswordView` dibungkus `max-w-sm` agar tidak melebar.
- Urutan halaman: h1 "Profil", identitas, Ganti password, rule 1px `border-rule`, lalu `<section id="token-git" aria-labelledby="token-git-title" class="scroll-mt-6">` berisi `GitTokensSection`.
- State lokal: `saveTarget: SaveTarget | null`, `deleteTarget: DeleteTarget | null`, dan `allowedGitlabHosts`.
- `onsave/onadd` membuka `SaveTokenDialog`; `ondelete` membuka `DeleteCredentialDialog`; `onsaved`/`ondeleted` memanggil `gitCredentials.invalidate()`.

**Knowledge**
- `KnowledgeSidebar`: prop baru `onsync: () => void`; baris aksi menjadi `flex flex-wrap gap-2` dengan isi Upload (primary sm), File baru (secondary sm), Sync dari Git (secondary sm, ikon cabang 16px).
- `KnowledgeMain`: prop `onsync`; empty state menampilkan Upload (primary) dan "Sync dari Git" (secondary).
- `KnowledgeTab`: `dialog = { kind: 'gitSync' }`; `onsynced = refresh` (`treeRefreshKey++`) + `viewerReloadKey++` (file yang sedang dibuka mungkin ditimpa). Bila editor `dirty`, tidak ada pemaksaan: konflik versi ditangani alur simpan yang ada (`version_conflict`).
- `KnowledgeDialogs`: `DialogState` ditambah `{ kind: 'gitSync' }`; render `<GitSyncDialog open={dialog?.kind==='gitSync'} {projectId} kind="knowledge" {onclose} onsynced={onsynced} />`.

**API Spec**
- `ListSpecsView`: prop opsional `headeractions?: Snippet` dirender di `header` sebelum tombol Upload (`flex gap-2`) dan di `EmptyState` action.
- `ApiSpecsTab`: `let syncOpen = $state(false)`; snippet tombol "Sync dari Git" (secondary); `<GitSyncDialog open={syncOpen} projectId={ctx.project.id} kind="api_spec" onclose={() => (syncOpen=false)} onsynced={() => list.invalidate()} />`. Hanya pada tampilan daftar (`item === null`). Di halaman detail spec tidak ada tombol Sync.

---

## 8. Error, empty, dan loading state (matriks)

| Layar/blok | Loading | Empty | Error |
|---|---|---|---|
| Tabel Token Git | `Table loading`: baris kerangka 48px dengan bar abu di sel; header langsung tampil | Tidak ada empty murni: baris GitHub "Belum ada" + placeholder GitLab `gitlab.com` "Belum ada" | `ErrorState` di dalam panel + "Coba lagi"; bila `previous` ada, tabel lama tetap tampil di atas pesan satu baris |
| Token Git, fitur nonaktif (`enabled=false`) | – | Panel berbingkai: "Sync Git belum dikonfigurasi oleh admin (`GIT_CREDENTIAL_KEY`)." Baris tersimpan tetap tampil tanpa aksi | – |
| Dialog Simpan token | Tombol `loading` "Memverifikasi ke GitHub…", form `disabled` | – | `TOKEN_INVALID` → galat field Token; `HOST_NOT_ALLOWED` → galat field Host; `INVALID_ARGUMENT` → `fieldErrorsOf`; `RATE_LIMITED` → galat form + "coba lagi dalam {n} detik"; `GIT_UPSTREAM_ERROR` → galat form + trace id; `GIT_FEATURE_DISABLED` → galat form + form dinonaktifkan; lainnya → `error.internal` + trace id. Nilai token tetap di input (kecuali sukses). |
| Dialog Hapus token | Tombol `danger loading`, `dismissible=false` | – | 404 → tutup + toast info + `ondeleted`; lainnya → pesan + trace id di dialog |
| Dialog Sync: config | Skeleton satu baris di slot ringkasan | "Belum pernah sync…" | `ErrorState` kecil + Coba lagi; 404 `NOT_FOUND` → tutup dialog + panggil `onprojectgone` komposer bila ada (project hilang/akses dicabut) |
| Dialog Sync: run | Tombol `loading` + teks provider; field `disabled` | Run sukses dengan 0 item (folder hanya berisi tipe tak didukung semua dilewati, atau folder root kosong) → tabel menampilkan baris yang ada; bila `items=[]`: satu kalimat dalam bingkai panel "Tidak ada file di folder ini." | Per kode (tabel 8.1) |
| Header Knowledge/API Spec | – | Empty state memuat tombol Sync sebagai jalan kedua | – |

### 8.1 Pemetaan kode error run → UI

| Kode | Status | Tampilan | Field disorot |
|---|---|---|---|
| `CREDENTIAL_MISSING` | 422 | `CredentialNotice missing` + tautan `details.settings_path` | – |
| `CREDENTIAL_UNREADABLE` | 422 | `CredentialNotice unreadable` ("Token perlu disimpan ulang") | – |
| `GIT_FEATURE_DISABLED` | 503 | `CredentialNotice disabled`, form dikunci | – |
| `INVALID_REPO_URL` | 400 | pesan server (sudah Indonesia) | `details.field` (`branch`) atau `url` |
| `INVALID_ARGUMENT` | 400 | `fieldErrorsOf(details)` | `target_prefix`/`kind`/`url` |
| `HOST_NOT_ALLOWED` | 422 | "Host Git tidak diizinkan; minta admin menambahkannya ke `GIT_SYNC_ALLOWED_HOSTS`." | url |
| `GIT_TOKEN_REJECTED` | 422 | pesan + tautan Atur Token Git | – |
| `GIT_ACCESS_DENIED` | 422 | pesan | url |
| `GIT_NOT_FOUND` | 404 | `details.what`: `repo` → url; `branch` → branch; `folder` → folder ("Folder {folder} tidak ditemukan di branch ini") | sesuai `what` |
| `SYNC_LIMIT_EXCEEDED` | 422 | "{files} file, melebihi batas {max_files} per sync; persempit folder." | folder |
| `GIT_TREE_TOO_LARGE` | 422 | pesan "persempit folder" | folder |
| `SYNC_IN_PROGRESS` | 409 | "Sync lain untuk project ini sedang berjalan. Coba lagi sebentar lagi." + Coba lagi | – |
| `RATE_LIMITED` | 429 | pesan + hitung mundur `retry_after_seconds`; tombol Sync terkunci sampai 0 | – |
| `GIT_UPSTREAM_ERROR` | 502 | pesan + trace id + Coba lagi | – |
| `TIMEOUT` (server 504) / abort timeout klien | 504 / – | "Sync melebihi batas waktu. Hasil sebagian mungkin sudah tersimpan; buka lagi untuk melihat status terakhir." | – |
| `NOT_FOUND` | 404 | tutup dialog + `onprojectgone` | – |
| `UNAUTHORIZED` | 401 | ditangani `http.ts` (redirect login) | – |
| `CSRF_REQUIRED`, `INTERNAL`, `NETWORK`, `INVALID_RESPONSE` | – | pesan umum + trace id | – |

Kunci `NOT_FOUND` saat getconfig/run 404: dialog tidak menebak penyebab; perilaku sama dengan tab lain (`onprojectgone` → `ctx.reload()`).

### 8.2 Reason per item

`reasonKey(kind, code)`: knowledge → `knowledge.reason.${code}`, api_spec → `apispec.reason.${code}`. Bila kunci tidak ada di i18n (fungsi `$t` mengembalikan kunci itu sendiri), dipakai `reason` dari server, lalu kode mentah mono. Kunci yang butuh parameter (`knowledge.reason.too_large` `{max}`) diberi `max` = "batas" (teks i18n `gitSync.reason.limitUnknown`) karena laporan sync tidak membawa angka batas. Tombol salin menyalin `"{repo_path}: {teks alasan}"` lewat `copyText`, lalu menampilkan toast sukses/gagal.

---

## 9. Visual (DESIGN.md, framed-sheet)

### 9.1 Profil → Token Git

1. **Pertanyaan:** "Token Git mana yang sudah saya simpan, untuk host apa, dan kapan terakhir diganti?" Blok terbesar: tabel panel Token Git.
2. **Pola dan kerangka:** pola **settings** (app-screens 8.2) dalam `AppLayout` (sidebar + sheet putih). Halaman sempit, jadi label section ditumpuk di atas panel (h2 16px/600 + satu kalimat 14px `text-muted`), bukan dua kolom. Isi section memakai pola **list** (tabel panel radius 8, `shadow-panel`, header 13px abu pada pita `bg-raised`, baris 48px, rule `border-rule`). Aksi destruktif (Hapus) adalah ghost per baris, dengan dialog konfirmasi yang menyebut objeknya. Satu model simpan: tiap aksi lewat dialog, tanpa tombol Simpan halaman. Tombol utama layar tetap milik Ganti password; seksi ini hanya memakai ghost/secondary.
3. **Elemen khas di layar pertama:** host dalam mono (`github.com`, `git.noobee.id`), `…7Qx2` mono redup di samping pil "Tersimpan", dan blok scope mono `repo` / `read_api` + `read_repository` di bawah tabel ("Show what the setting produces": user melihat persis izin yang dibutuhkan).
4. **Token/utility:** `bg-surface`, `border-border`, `rounded-card`, `shadow-panel`, `bg-raised` (header tabel, blok scope), `border-rule`, `text-fg`/`text-muted`, `font-mono`, `bg-pill-positive-bg text-pill-positive-ink` (Tersimpan), `bg-pill-neutral-bg text-pill-neutral-ink` (Belum ada), `text-accent-ink` (tautan "Tambah host GitLab", fokus), `rounded-base` kontrol 32px, `tabular-nums`. Nilai hilang ditulis `–` abu.
5. **Data contoh:**

   | Provider | Host | Status | Diperbarui |
   |---|---|---|---|
   | GitHub | `github.com` | Tersimpan `…a9F3` | 22 Okt 2026 14.00 |
   | GitLab | `git.noobee.id` | Tersimpan `…7Qx2` | 22 Okt 2026 14.10 |
   | GitLab | `gitlab.com` | Belum ada | – |

   `allowed_gitlab_hosts`: `gitlab.com`, `git.noobee.id`. Fitur nonaktif: `enabled=false`, `items=[]`.

### 9.2 Dialog Simpan/Ganti token

1. **Pertanyaan:** "Token apa, untuk host mana, dan izin apa yang dibutuhkan?" Blok terbesar: field Token + catatan scope.
2. **Pola:** dialog `shared/Dialog size="md"` (`max-w-lg`, `rounded-sheet`, `shadow-pop`). Form label-di-atas, jarak 16px antar field, satu tombol utama `primary` "Simpan" di kanan bawah, Batal `secondary`.
3. **Elemen khas:** `ScopeNote` mono berubah mengikuti provider; input token bertipe password dengan font mono; judul "Ganti token GitLab · git.noobee.id" menyebut host mono.
4. **Token:** `bg-surface`, `shadow-pop`, `rounded-sheet`, `border-control` (input), `bg-raised rounded-base` (scope), `text-muted` (catatan), `text-danger` (galat field), `ring-accent-ink` (fokus). Bukan `accent` di tombol, bukan `bg-surface-2` (F7).
5. **Data contoh:** Provider GitLab, Host `git.noobee.id`, token placeholder "glpat-…" (placeholder, bukan nilai), galat "Token tidak valid atau tidak punya akses".

### 9.3 Dialog Hapus token

1. **Pertanyaan:** "Token mana yang akan dihapus, dan apa akibatnya?" Blok terbesar: baris identitas token + kalimat konsekuensi.
2. **Pola:** dialog konfirmasi (pola `RevokeKeyDialog`), `size="md"`, tombol `danger` sebagai satu-satunya aksi kuat.
3. **Elemen khas:** `GitLab · git.noobee.id · …7Qx2` (host dan last4 mono).
4. **Token:** `shadow-pop`, `rounded-sheet`, `text-muted`, tombol `danger`.
5. **Data contoh:** "Hapus token GitLab (git.noobee.id)? Sync yang memakai token ini akan berhenti bekerja untuk Anda."

### 9.4 Dialog Sync dari Git (Form + Hasil)

1. **Pertanyaan:** state Form: "Apa yang akan ditarik, dari repo/branch/folder mana, ke mana?" Blok terbesar: form + `ConfigSummary`. State Hasil: "File mana yang berubah, dan mana yang ditolak beserta alasannya?" Blok terbesar: `SyncReportTable`.
2. **Pola dan kerangka:** dialog `size="lg"` (`max-w-2xl`, F8) di atas workspace project (title row + tab garis bawah tetap terlihat di belakang backdrop). Form memakai pola settings (label di atas, kontrol 32px). Hasil memakai pola **list** (panel tabel, baris 48px, satu penanda status per baris, kolom identitas = Path repo terlebar). Ringkasan angka = satu kontainer dibagi hairline (app-screens 5/7.4), bukan kartu KPI. Tombol di header tab: "Sync dari Git" `secondary`; aksi utama tab tetap Upload.
3. **Elemen khas di layar pertama:** `RepoOriginChip` mono `github.com/noobee/grimoire-docs@main:docs/` di bawah judul dialog; path repo dan path tujuan mono di tabel; pointer issue `#/paths/~1charges/get` mono pada baris ditolak yang diperluas.
4. **Token/utility:** `bg-surface`, `rounded-sheet`, `shadow-pop` (dialog); `rounded-card`, `shadow-panel`, `bg-raised`, `border-rule` (tabel); `bg-pill-{positive,warning,negative,neutral}-bg` + `text-pill-…-ink` (status); `text-warning` (peringatan timpa); `bg-pill-warning-bg text-pill-warning-ink rounded-card` (CredentialNotice); `text-danger` (angka ditolak > 0 dan pesan galat saja); `font-mono`; `tabular-nums`; `bg-primary text-primary-fg` (satu tombol utama per state); `text-accent-ink` (tautan Atur Token Git, fokus); `bg-raised rounded-base` (chip asal-repo, F9). Transisi 120ms ease-out hanya untuk hover/tekan/buka-tutup baris; `prefers-reduced-motion` dihormati.
5. **Data contoh:**
   - Config tersimpan: `https://github.com/noobee/grimoire-docs/tree/main/docs`, branch `main`, folder `docs`, prefix kosong, "Terakhir 22 Okt 2026 21.05 oleh Rina · Berhasil sebagian".
   - Hasil knowledge (`summary` dihitung dari baris): `docs/auth/login-flow.md` → `auth/login-flow.md` dibuat; `docs/api/rate-limit.md` → `api/rate-limit.md` dibuat; `docs/img/diagram.png` → `–` dilewati ("Hanya file .md, .markdown, dan .txt yang didukung"); `docs/legacy/../x.md` → `–` ditolak ("Path tidak boleh memuat segmen . atau .."). Ringkasan: 2 · 0 · 0 · 1 · 1.
   - Hasil api_spec: `api/openapi/billing.yaml` → `billing` (14 endpoint) dibuat; `api/openapi/payments.json` → `payments` ditolak, issue `EXTERNAL_REF` pointer `#/paths/~1charges/get`. Ringkasan: 1 · 0 · 0 · 0 · 1, status "Berhasil sebagian".
   - Sync ulang tanpa perubahan: 2 tidak berubah, 1 dilewati.

### 9.5 Ponsel (≤ 640px)

- Dialog mengisi lebar layar dikurangi 16px gutter. Tombol footer tetap satu baris (Batal sebagai ghost).
- `SyncReportTable` menjadi daftar dua baris: baris 1 berisi Path repo mono + `StatusBadge` di kanan; baris 2 (13px abu) berisi tujuan · alasan; tombol salin di kanan. Tidak ada scroll horizontal halaman.
- Tabel Token Git menjadi daftar dua baris: baris 1 berisi Provider + pil status; baris 2 berisi host mono · diperbarui; aksi ghost di kanan.
- Baris aksi sidebar Knowledge sudah `flex-wrap`. Target sentuh minimal 32px (DESIGN.md) dengan padding baris 44px.

---

## 10. Usulan perubahan DESIGN.md (catatan, tidak diterapkan)

- **U1. Chip asal-repo.** Overview 9 meminta `RepoOriginChip` "gaya chip agent" (kuning pucat). DESIGN.md hanya mengizinkan kuning untuk logo, tab aktif, chip agent di audit, dan chart. Plan ini memakai chip netral `bg-raised` (F9). Bila pemilik ingin asal-repo menjadi "wajah produk" kedua, tambahkan baris di DESIGN.md "Wajah produk": "Asal sync git: chip mono `bg-raised` + ikon cabang" (rekomendasi, tetap netral), atau izinkan `bg-accent/20` secara eksplisit. Keputusan manusia.
- **U2. Tabel Pola per layar.** Tambah dua baris ke "Pola per layar":
  - `/profile` seksi Token Git — "Token Git mana yang saya simpan dan untuk host apa?" — settings bertumpuk + tabel panel.
  - Dialog Sync dari Git — "Apa yang ditarik dan file mana yang berubah?" — dialog lg: form → laporan panel tabel.
- **U3. Lebar dialog.** Tidak ada perubahan; `lg` (672px) dipakai. Dicatat agar plan lain tidak menulis 720.

---

## 11. i18n (`frontend/src/shared/i18n/{id,en}.json`)

Kunci baru (id dan en wajib sama; `i18n-keys.test.ts` hijau). Teks id ditampilkan. Kunci yang sudah didaftar di plan usecase dipakai apa adanya; daftar ini adalah gabungan final.

| Kunci | id |
|---|---|
| `gitCredential.title` / `.intro` | Token Git / Token pribadi untuk menarik file dari GitHub/GitLab. Hanya Anda yang memakainya. |
| `gitCredential.col.{provider,host,status,updated,actions}` | Provider / Host / Status / Diperbarui / Aksi |
| `gitCredential.status.{saved,none}` | Tersimpan / Belum ada |
| `gitCredential.provider.{github,gitlab}` | GitHub / GitLab |
| `gitCredential.action.{save,replace,delete}` (+ `.aria`) | Simpan / Ganti / Hapus (Simpan token {provider} untuk {host}, …) |
| `gitCredential.addGitlabHost` | Tambah host GitLab |
| `gitCredential.disabled` | Sync Git belum dikonfigurasi oleh admin ({key}). |
| `gitCredential.scope.label` / `.github` / `.gitlab` | Izin minimal / repo (baca) / read_api, read_repository |
| `gitCredential.noReveal` | Token tidak bisa dilihat lagi setelah disimpan. |
| `gitCredential.save.{title,titleReplace,provider,host,token,tokenPlaceholder,submit,verifying,saved}` | Simpan token Git / Ganti token {provider} · {host} / Provider / Host / Token / Tempel token di sini / Simpan / Memverifikasi ke {provider}… / Token {provider} disimpan |
| `gitCredential.save.validation.{length,charset,host}` | Token harus 8–512 karakter / Token hanya boleh berisi karakter ASCII tanpa spasi / Pilih host GitLab |
| `gitCredential.save.error.{TOKEN_INVALID,HOST_NOT_ALLOWED,GIT_FEATURE_DISABLED,RATE_LIMITED,RATE_LIMITED_RETRY,GIT_UPSTREAM_ERROR,generic}` | Token tidak valid atau tidak punya akses / Host Git tidak diizinkan; minta admin menambahkannya ke {key} / Sync Git belum dikonfigurasi oleh admin / Provider Git membatasi permintaan / coba lagi dalam {seconds} detik / Provider Git tidak dapat dihubungi, coba lagi / Token gagal disimpan |
| `gitCredential.delete.{title,confirm,submit,success,gone}` | Hapus token Git / Hapus token {provider}? Sync yang memakai token ini akan berhenti bekerja untuk Anda. / Hapus / Token {provider} dihapus / Token sudah tidak ada |
| `gitSync.button` | Sync dari Git |
| `gitSync.dialog.title.{knowledge,api_spec}` | Sync knowledge dari Git / Sync API spec dari Git |
| `gitSync.field.{url,branch,folder,targetPrefix}` | URL repo atau folder / Branch / Folder di repo / Simpan di folder |
| `gitSync.help.{url,branch,folder,targetPrefix}` | Contoh: https://github.com/noobee/grimoire-docs/tree/main/docs / Isi bila nama branch memuat / (mis. feature/x) / Kosong = root repo / Folder di Grimoire tempat file disimpan |
| `gitSync.targetPrefixPlaceholder` | / (root) |
| `gitSync.overwriteWarning` | File bernama sama akan ditimpa. File yang tidak ada di repo tidak dihapus. |
| `gitSync.{run,rerun,again,close,cancel}` | Sync / Sync ulang / Sync lagi / Tutup / Batal |
| `gitSync.running` / `.runningHint` | Menarik dari {provider}… / Sync berjalan di server; bisa sampai 2 menit. |
| `gitSync.cancelledToast` | Sync tetap diselesaikan server; buka lagi untuk melihat hasilnya. |
| `gitSync.config.{lastSync,neverSynced,loadFailed}` | Terakhir {time} oleh {label} / Belum pernah sync untuk {kind} project ini. / Konfigurasi sync gagal dimuat |
| `gitSync.status.{success,partial,failed}` | Berhasil / Berhasil sebagian / Gagal |
| `gitSync.itemStatus.{created,updated,unchanged,skipped,rejected}` | Dibuat / Diperbarui / Tidak berubah / Dilewati / Ditolak |
| `gitSync.summary.{created,updated,unchanged,skipped,rejected}` | dibuat / diperbarui / tidak berubah / dilewati / ditolak |
| `gitSync.report.{repoPath,target,status,reason,endpoints,truncated,empty,issues,copyReason,copied}` | Path repo / Tujuan / Status / Alasan / {n} endpoint / Menampilkan {n} dari {total} file; ringkasan dihitung dari semua file. / Tidak ada file di folder ini. / Detail masalah / Salin alasan / Alasan disalin |
| `gitSync.reason.limitUnknown` | batas |
| `gitSync.credential.{missing,unreadable,disabled,setup}` | Anda belum menyimpan token Git untuk {host}. / Token Git Anda untuk {host} tidak dapat dibaca; simpan ulang token. / Sync Git belum dikonfigurasi oleh admin ({key}). / Atur Token Git |
| `gitSync.validation.{required,https_only,userinfo,query,blob_url}` | Isi URL repo / Hanya URL https yang didukung / Jangan sertakan nama pengguna atau token di URL / URL tidak boleh memuat ? atau # / Gunakan URL folder (tree), bukan file (blob) |
| `gitSync.errors.<KODE>` untuk `CREDENTIAL_MISSING, CREDENTIAL_UNREADABLE, INVALID_REPO_URL, HOST_NOT_ALLOWED, GIT_TOKEN_REJECTED, GIT_ACCESS_DENIED, GIT_NOT_FOUND, GIT_NOT_FOUND_folder, GIT_NOT_FOUND_branch, SYNC_LIMIT_EXCEEDED, SYNC_IN_PROGRESS, GIT_TREE_TOO_LARGE, RATE_LIMITED, GIT_UPSTREAM_ERROR, GIT_FEATURE_DISABLED, TIMEOUT` | teks per tabel 8.1 |

---

## 12. Plan (langkah berurutan)

FE bisa dikerjakan paralel dengan langkah 1 backend (overview 8), memakai `mocks` dan kontrak bagian 5/15. E2E menunggu backend.

1. **i18n**: tambah semua kunci bagian 11 ke `id.json` dan `en.json`. Jalankan `npm run test -- i18n-keys`.
2. **`git-credential/listcredentials`**: `types.ts` (+ test), `mocks/credentials.ts`, service (+ test), store (+ test), `ProviderCell`, `GitTokensTable` (+ test), `GitTokensSection` (+ test), `index.ts`.
3. **`git-credential/savecredential`**: `types.ts` (+ test), service (+ test), `ScopeNote`, `SaveTokenForm` (+ test), `SaveTokenDialog` (+ test), `index.ts`.
4. **`git-credential/deletecredential`**: `types.ts`, service (+ test), `DeleteCredentialDialog` (+ test), `index.ts`.
5. **Komposisi Profil**: ubah `app/views/ProfileView.svelte` (lebar, seksi `#token-git`, dua dialog, `invalidate`, scroll ke anchor) dan tambah `ProfileView.test.ts`.
6. **`git-sync/base`**: `types.ts` (+ test), `url.ts` (+ test), `reason.ts` (+ test), `mocks.ts`, komponen `StatusBadge`, `RepoOriginChip`, `ReportSummary`, `ConfigSummary`, `SyncReportTable` (+ `components.test.ts`), `index.ts`.
7. **`git-sync/getconfig`**: `types.ts` (+ test), service (+ test), `gitSyncConfig.store.ts` (+ test), `index.ts`.
8. **`git-sync/runsync`**: `types.ts` (+ test, termasuk `toSyncError`), service (+ test), `runsync.store.ts` (+ test), `SyncForm`, `CredentialNotice`, `SyncErrorBlock` (+ test), `GitSyncDialog` (+ test), `index.ts`.
9. **Slot api-spec**: `ListSpecsView.svelte` mendapat prop `headeractions?: Snippet` (+ satu kasus test di test listspecs yang ada; tes lama tetap hijau).
10. **Komposisi workspace**: `KnowledgeSidebar` (`onsync`), `KnowledgeMain` (`onsync` di empty state), `KnowledgeDialogs` (`gitSync`), `KnowledgeTab` (`onsynced`), `ApiSpecsTab` (snippet + dialog). Perbarui `KnowledgeTab.test.ts` dan `ApiSpecsTab.test.ts`.
11. **Arch**: tambahkan fixture di `scripts/test-arch.mjs` bila skrip memakai fixture per aturan (`git-sync/runsync` → `core/git-credential` harus gagal). Jalankan `npm run depcruise`.
12. **E2E**: `frontend/e2e/fixtures/fake-git-server.ts` (overview 11) dan `frontend/e2e/git-sync.spec.ts` (bagian 14). Konfigurasi env backend e2e di `global-setup.ts`/`playwright.config.ts`: `GIT_CREDENTIAL_KEY`, `GIT_SYNC_INSECURE_TEST_HOSTS`, `GIT_SYNC_GITHUB_API_BASE`.
13. **Design check**: `node .agents/skills/design-taste-frontend/scripts/lint.mjs --app frontend/src` pada file yang diubah. `inspect.mjs --app http://localhost:5173/profile` bila dev server + backend tersedia. Perbaiki FAIL.
14. **Verifikasi**: `cd frontend && npm run test && npm run depcruise && npm run test:arch && npm run lint && npm run build`, lalu `npx playwright test e2e/git-sync.spec.ts e2e/knowledge.spec.ts e2e/api-spec-tools.spec.ts`.

Kontrak bus: tidak ada yang disentuh FE. FE hanya memakai REST bagian 1. Bus `core.gitcredential.get-token`, `project.knowledge.upsert-files`, `project.apispec.upsert-specs` didefinisikan di overview 5.7/6.4.

---

## 13. Migration

Tidak ada untuk frontend. Migrasi backend (`backend/migrations/core/<VERSION>_create_git_credentials.sql`, `backend/migrations/project/<VERSION>_create_git_sync_configs.sql`, versi `YYYYMMDDHHMMSSmmm` di-generate saat implementasi) ada di overview bagian 7.

---

## 14. Changes (per file/folder)

| File / folder | Jenis |
|---|---|
| `frontend/src/domain/core/git-credential/listcredentials/**` | baru |
| `frontend/src/domain/core/git-credential/savecredential/**` | baru |
| `frontend/src/domain/core/git-credential/deletecredential/**` | baru |
| `frontend/src/domain/project/git-sync/base/**` | baru |
| `frontend/src/domain/project/git-sync/getconfig/**` | baru |
| `frontend/src/domain/project/git-sync/runsync/**` | baru |
| `frontend/src/app/views/ProfileView.svelte` (+ `ProfileView.test.ts`) | ubah: `max-w-2xl`, seksi `#token-git`, dialog, anchor |
| `frontend/src/app/views/workspace/knowledge/KnowledgeSidebar.svelte` | ubah: prop `onsync`, tombol, `flex-wrap` |
| `frontend/src/app/views/workspace/knowledge/KnowledgeMain.svelte` | ubah: prop `onsync` di empty state |
| `frontend/src/app/views/workspace/knowledge/KnowledgeDialogs.svelte` | ubah: `DialogState` `gitSync`, render `GitSyncDialog` |
| `frontend/src/app/views/workspace/knowledge/KnowledgeTab.svelte` (+ test) | ubah: buka dialog, `onsynced` muat ulang pohon + viewer |
| `frontend/src/app/views/workspace/ApiSpecsTab.svelte` (+ test) | ubah: tombol via slot, `GitSyncDialog`, `list.invalidate()` |
| `frontend/src/domain/project/api-spec/listspecs/views/ListSpecsView.svelte` (+ test) | ubah aditif: prop `headeractions?: Snippet` |
| `frontend/src/shared/i18n/{id,en}.json` | ubah: kunci bagian 11 |
| `frontend/scripts/test-arch.mjs` | ubah bila perlu: fixture larangan `git-sync` → `git-credential` |
| `frontend/e2e/fixtures/fake-git-server.ts` | baru |
| `frontend/e2e/git-sync.spec.ts` | baru |
| `frontend/e2e/global-setup.ts` / `playwright.config.ts` | ubah: env backend e2e + start/stop fake git server |
| `frontend/src/shared/**` (komponen, http, token CSS), `frontend/src/app.css`, `DESIGN.md` | TIDAK berubah |

---

## 15. Test Scenario (vitest + @testing-library/svelte; fetch dipalsukan)

### 15.1 Tipe dan util

| File | Kasus |
|---|---|
| `listcredentials/types.test.ts` | respons 12.1 lolos; field `token` tambahan → gagal (`.strict()`); `last4` 5 karakter → gagal; `buildRows`: `[]` → GitHub + placeholder `gitlab.com`; GitLab 2 host → urut host; `maskLast4('7Qx2')` = `…7Qx2` |
| `savecredential/types.test.ts` | `validateToken`: 7 karakter → `length`; 513 → `length`; spasi/`é` → `charset`; trim; `scopeFor`; skema respons dengan `token` → gagal |
| `git-sync/base/types.test.ts` | contoh 12.4 dan 12.5 lolos; `last_result: null` lolos; status item tak dikenal → gagal; field opsional api_spec |
| `git-sync/base/url.test.ts` | tabel: GitHub tree (branch `main`, folder `docs`); tanpa tree (branch kosong); `.git` dibuang; `/blob/` → `blob_url`; `http://` → `https_only`; `https://tok@github.com/...` → `userinfo` dan hasil tidak memuat `tok`; `?x=1` → `query`; GitLab `/-/tree/main/api` → path `grp/sub/repo`; `git.noobee.id:8443` host dengan port |
| `git-sync/base/reason.test.ts` | `reasonKey('knowledge','unsupported_type')` = `knowledge.reason.unsupported_type`; `reasonKey('api_spec','INVALID_SPEC')`; fallback ke `reason` lalu kode bila kunci tidak ada |
| `runsync/types.test.ts` | `toSyncError`: `INVALID_REPO_URL` + `details.field=branch` → `field:'branch'`; `CREDENTIAL_MISSING` tanpa `settings_path` → default `SETTINGS_PATH`; `RATE_LIMITED` `retry_after_seconds` 60; `SYNC_LIMIT_EXCEEDED` → `limit`; `GIT_NOT_FOUND what=folder` → `field:'folder'`; `AbortError` → `ABORTED`; error non-API → `NETWORK` |

### 15.2 Services

- `listcredentials.service.test.ts`: GET `/api/me/git-credentials`, tanpa CSRF; respons invalid → `INVALID_RESPONSE`; `signal` diteruskan.
- `savecredential.service.test.ts`: PUT `/api/me/git-credentials/github` body persis `{"token":"…"}`; GitLab `{"host":"git.noobee.id","token":"…"}`; header `X-Requested-With: grimoire`; 422 `TOKEN_INVALID` → `ApiError.code`.
- `deletecredential.service.test.ts`: GitHub tanpa `?host`; GitLab `?host=git.noobee.id`; 204 → `undefined`; 404 → `ApiError.status 404`.
- `getconfig.service.test.ts`: GET `/api/projects/{id}/git-sync?kind=knowledge`; `projectId` di-encode; `config:null` lolos.
- `runsync.service.test.ts`: POST body knowledge memuat `target_prefix`; body api_spec tanpa `target_prefix`; trim; CSRF; timeout klien 130 s dengan fake timers → `ABORTED`/timeout dipetakan ke `TIMEOUT` lokal; `signal` user membatalkan.

### 15.3 Stores

- `gitCredentials.store.test.ts`: `begin/resolve` user sama mempertahankan `previous`; user beda mengosongkan; `resolve` dengan userId basi diabaikan; `invalidate` menaikkan `version`; `reset` → idle; tidak ada properti `token` di state mana pun.
- `gitSyncConfig.store.test.ts`: `load` ganda untuk kunci sama → satu fetch; `load` baru membatalkan yang lama; `setFromRun(config, report)` mengisi `last_result` dari report; error mempertahankan `previous`.
- `runsync.store.test.ts`: `start` saat `running` diabaikan (satu fetch); 200 → `done`; 422 → `error` dengan `SyncError`; abort → `idle`; `reset` dari `done`.

### 15.4 Komponen dan view

| File | Kasus |
|---|---|
| `GitTokensTable` / `components.test.ts` | pil "Tersimpan" + `…7Qx2` mono; "Belum ada" + `–`; tombol aksi tidak dirender tanpa callback atau saat `canEdit=false`; `aria-label` memuat host; loading menampilkan baris kerangka; error menampilkan Coba lagi |
| `GitTokensSection.test.ts` | memuat sekali saat mount; `version` naik → muat ulang; `enabled=false` → teks admin dengan `GIT_CREDENTIAL_KEY` mono, tanpa aksi; "Tambah host GitLab" hanya bila ada host yang belum tersimpan; `onready` dipanggil setelah data siap |
| `SaveTokenDialog.test.ts` | sukses: service dipanggil sekali, `onsaved`, dialog tertutup, input dikosongkan; `document.body.textContent` tidak memuat token; `localStorage`/`sessionStorage` tidak memuat token; `TOKEN_INVALID` → galat di field Token dan nilai input tetap; `HOST_NOT_ALLOWED` → galat Host; `RATE_LIMITED` → hitung mundur; `GIT_UPSTREAM_ERROR` → trace id; `GIT_FEATURE_DISABLED` → form dinonaktifkan; klik ganda → satu request; validasi klien mencegah request; buka ulang → form kosong; GitHub tanpa field Host; scope berubah saat provider diganti |
| `DeleteCredentialDialog.test.ts` | teks memuat provider + host; konfirmasi → service, `onclose`, `ondeleted`, toast; 404 → toast "Token sudah tidak ada" + `ondeleted`; 500 → pesan dan trace id, dialog tetap terbuka; Batal tidak memanggil service; tidak bisa ditutup saat submit |
| `ProfileView.test.ts` | seksi `#token-git` ada dengan `h2#token-git-title`; kontainer `max-w-2xl`; Simpan membuka dialog; setelah `onsaved` → `gitCredentials.invalidate` dipanggil; hash `#token-git` → `scrollIntoView` + fokus h2; tes profil/ganti password lama tetap hijau |
| `git-sync/base/components.test.ts` | `StatusBadge` 8 status → kelas pil dan teks benar (tidak warna saja); `RepoOriginChip` merender `github.com/noobee/grimoire-docs@main:docs/` dan folder kosong → tanpa `:`; `ReportSummary` menulis `0` untuk nol dan `text-danger` hanya pada ditolak > 0; `ConfigSummary` "Belum pernah sync" / status failed menampilkan pesan error; `SyncReportTable`: urutan rejected dulu; baris rejected dengan issues bisa diperluas (`aria-expanded`) dan memperlihatkan pointer mono; salin alasan memanggil `copyText`; `truncated` menampilkan catatan; `items=[]` menampilkan kalimat kosong di dalam bingkai |
| `runsync/components.test.ts` | `SyncForm`: URL mengisi Branch/Folder otomatis bila belum disentuh; tidak menimpa field yang sudah disentuh; "Simpan di folder" hanya untuk knowledge; `CredentialNotice` tautan `href=/settings#token-git`; `SyncErrorBlock` per kode di tabel 8.1 |
| `GitSyncDialog.test.ts` | buka → `getConfig` dipanggil dengan `kind`; config ada → form terisi + `ConfigSummary`; config null → "Belum pernah sync" dan Sync AKTIF walau `has_credential=false` (F5); config ada + `has_credential=false` + URL sama host → Sync dan Sync ulang nonaktif + `CredentialNotice missing`; `credential_unreadable` → teks simpan ulang; `feature_enabled=false` → form terkunci; URL diganti ke host lain → Sync aktif lagi (F6); Sync → state berjalan (tombol `loading`, teks "Menarik dari GitHub…", field disabled, Esc tidak menutup); 200 partial → tabel hasil, `onsynced` tepat sekali, ringkasan sama dengan jumlah baris; 200 lalu "Sync lagi" → kembali ke form terisi; Sync ulang mengirim nilai config, bukan nilai form yang diedit; `CREDENTIAL_MISSING` → notice + tautan; `INVALID_REPO_URL field=branch` → galat di Branch; `SYNC_IN_PROGRESS` → pesan + Coba lagi; Batal saat berjalan → fetch di-abort, dialog tertutup, toast `cancelledToast`, `onsynced` tidak dipanggil; 404 → `onclose` + tidak crash; DOM tidak memuat kata `token` selain label/tautan |
| `KnowledgeTab.test.ts` | tombol "Sync dari Git" ada di sidebar dan di empty state; klik → dialog kind knowledge; `onsynced` → `treeRefreshKey` naik (pohon memuat ulang) |
| `ApiSpecsTab.test.ts` / `ListSpecsView` test | tombol Sync tampil di header daftar dan empty state, tidak tampil di detail spec; `onsynced` → `list.invalidate()`; ListSpecsView tanpa `headeractions` tetap seperti sebelumnya |

### 15.5 Arsitektur

- `npm run depcruise` hijau.
- `npm run test:arch`: fixture baru membuktikan `src/domain/project/git-sync/runsync/x.ts` → `src/domain/core/git-credential/listcredentials/index.ts` gagal (`no-cross-feature-internals` hanya mengizinkan index; batas lintas module ditegakkan oleh review + test ini sebagai aturan proyek). Bila skrip tidak mendukung aturan baru, cukup tes vitest `arch.git-sync.test.ts` yang memindai import statis file `git-sync/**` dan gagal bila ada `domain/core/` atau `domain/project/(knowledge|api-spec)`.

---

## 16. E2E Scenarios (`frontend/e2e/git-sync.spec.ts`)

Setup sesuai overview 11. `fake-git-server.ts` (Node `http`) meniru `GET /user`, `GET /repos/{o}/{r}/git/trees/{branch}?recursive=1`, `GET /repos/{o}/{r}/git/blobs/{sha}`, dan endpoint kontrol `POST /__control` (`{ files, mode: 'ok'|'rate_limited'|'unauthorized' }`). Server berjalan di `127.0.0.1:<port>`. Backend e2e: `GIT_SYNC_INSECURE_TEST_HOSTS=127.0.0.1:<port>`, `GIT_CREDENTIAL_KEY=<base64 32 byte uji>`, `GIT_SYNC_GITHUB_API_BASE=http://127.0.0.1:<port>`. Token sah `ghp_e2eValidToken7Qx2`. Isi repo awal `noobee/grimoire-docs@main`: `docs/auth/login-flow.md`, `docs/api/rate-limit.md`, `docs/img/diagram.png`, `api/openapi/billing.yaml` (14 endpoint), `api/openapi/payments.json` (`$ref` eksternal). Pemantau `page.on('response')` mengumpulkan semua body JSON; di akhir tiap tes diasersi tidak ada yang memuat `ghp_e2eValidToken7Qx2`.

### Positif

1. **Simpan token.** `/profile` → seksi Token Git: GitHub "Belum ada". Klik Simpan, isi token sah, Simpan → pil "Tersimpan" + `…7Qx2`; input token tidak ada lagi di DOM; tidak ada respons yang memuat token.
2. **Sync knowledge.** Project "Grimoire Docs" → Knowledge → "Sync dari Git" → URL `https://github.com/noobee/grimoire-docs/tree/main/docs`. Branch dan Folder terisi otomatis (`main`, `docs`). Peringatan timpa terlihat. Sync → tombol "Menarik dari GitHub…" → hasil: `auth/login-flow.md` Dibuat, `api/rate-limit.md` Dibuat, `docs/img/diagram.png` Dilewati; ringkasan 2 · 0 · 0 · 1 · 0, status "Berhasil". Tutup → pohon memuat `auth/login-flow.md`.
3. **Config diingat + Sync ulang.** Buka dialog lagi: URL, branch `main`, folder `docs` terisi; ringkasan "Terakhir … oleh {nama} · Berhasil"; chip `github.com/noobee/grimoire-docs@main:docs/`. Sync ulang → semua "Tidak berubah" (+ 1 dilewati).
4. **Timpa tanpa hapus.** Buat manual `notes/manual.md` lewat UI. Fixture mengubah isi `docs/auth/login-flow.md`. Sync ulang → `auth/login-flow.md` Diperbarui; `notes/manual.md` masih ada di pohon; membuka `auth/login-flow.md` menampilkan isi baru.
5. **Sync API spec.** API Spec → Sync dari Git, URL `https://github.com/noobee/grimoire-docs`, Branch `main`, Folder `api/openapi`. Hasil: `billing` Dibuat (14 endpoint), `payments` Ditolak; status "Berhasil sebagian"; perluas baris `payments` → pointer `#/paths/~1charges/get` terlihat; Salin alasan → toast "Alasan disalin". Tutup → daftar spec memuat `billing`.
6. **Prefix tujuan.** Knowledge → dialog → "Simpan di folder" `guides` → Sync → tujuan `guides/auth/login-flow.md`; pohon memuat folder `guides`.
7. **Tautan Atur Token Git.** Dari `CredentialNotice` klik "Atur Token Git" → `/settings#token-git`, seksi Token Git di viewport dan judulnya fokus.

### Negatif

1. **Token tidak sah.** Fixture mode `unauthorized`; simpan token → galat di field Token "Token tidak valid atau tidak punya akses"; status tetap "Belum ada"; nilai input masih ada.
2. **Hapus token lalu sync.** Hapus token GitHub (dialog menyebut `github.com` + `…7Qx2`) → baris "Belum ada". Buka dialog Sync Knowledge (config ada) → `CredentialNotice` "Anda belum menyimpan token Git untuk github.com" + tautan; Sync dan Sync ulang nonaktif. `request.post` langsung ke `/run` → 422 `CREDENTIAL_MISSING`.
3. **Anggota lain tanpa token.** User B (anggota, tanpa PAT) membuka dialog → config A terlihat (URL, ringkasan, "oleh {A}"), tombol nonaktif; API → `CREDENTIAL_MISSING`; reload sebagai A → config dan waktu sync terakhir tidak berubah.
4. **Host tidak diizinkan.** URL `https://bitbucket.org/noobee/grimoire-docs` → Sync → galat di URL "Host Git tidak diizinkan…".
5. **Tanpa branch.** URL `https://github.com/noobee/grimoire-docs`, Branch kosong → galat di Branch "isi Branch".
6. **Validasi klien.** URL `http://github.com/…` → tombol Sync nonaktif + "Hanya URL https yang didukung"; URL dengan `ghp_x@github.com` → pesan userinfo, dan tidak ada request `/run` yang terkirim (dicek lewat `page.on('request')`).
7. **Folder tidak ada.** Folder `tidak-ada` → "Folder tidak-ada tidak ditemukan di branch ini" di field Folder; buka dialog lagi → ringkasan "Gagal".
8. **Rate limit.** Fixture mode `rate_limited` (403 + `x-ratelimit-remaining: 0`) → pesan rate limit; tombol Sync terkunci selama hitung mundur bila `Retry-After` ada.
9. **Akses.** Non-anggota `GET /api/projects/{P}/git-sync?kind=knowledge` → 404; tanpa login → 401; membuka `/projects/{P}/knowledge` sebagai non-anggota tidak menampilkan tombol Sync (halaman 404 workspace yang ada).
10. **Fitur nonaktif** (job e2e kedua tanpa `GIT_CREDENTIAL_KEY`, ditandai `test.skip` bila env tidak tersedia): `/profile` → teks admin `GIT_CREDENTIAL_KEY`; dialog Sync → form terkunci.

---

## 17. Request/Response body (yang dikonsumsi FE)

Format error (semua endpoint), dibaca `toApiError` di `shared/services/http.ts`:
```json
{ "error": { "code": "TOKEN_INVALID", "message": "token tidak valid atau tidak punya akses", "details": { "provider": "gitlab", "host": "git.noobee.id" } }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```
Semua mutasi mengirim `X-Requested-With: grimoire` dan cookie sesi.

### 17.1 GET `/api/me/git-credentials`
```json
{
  "enabled": true,
  "allowed_gitlab_hosts": ["gitlab.com", "git.noobee.id"],
  "items": [
    { "provider": "github", "host": "github.com", "last4": "a9F3", "created_at": "2026-10-22T07:00:00Z", "updated_at": "2026-10-22T07:00:00Z" },
    { "provider": "gitlab", "host": "git.noobee.id", "last4": "7Qx2", "created_at": "2026-10-22T07:05:00Z", "updated_at": "2026-10-22T07:10:00Z" }
  ]
}
```
Fitur nonaktif: `{ "enabled": false, "allowed_gitlab_hosts": ["gitlab.com"], "items": [] }`.

### 17.2 PUT `/api/me/git-credentials/{provider}`
Request GitHub: `{ "token": "ghp_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxa9F3" }`
Request GitLab: `{ "host": "git.noobee.id", "token": "glpat-xxxxxxxxxxxxxxxx7Qx2" }`
200:
```json
{ "provider": "gitlab", "host": "git.noobee.id", "last4": "7Qx2", "updated_at": "2026-10-22T07:10:00Z", "replaced": true }
```
Error: 422 `TOKEN_INVALID` (contoh di atas); 422 `HOST_NOT_ALLOWED`
```json
{ "error": { "code": "HOST_NOT_ALLOWED", "message": "host Git tidak diizinkan; minta admin menambahkannya ke GIT_SYNC_ALLOWED_HOSTS", "details": { "host": "git.contoh.com" } }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```
400 `INVALID_ARGUMENT`
```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "permintaan tidak valid", "details": { "fields": { "token": "token wajib 8-512 karakter ASCII tanpa spasi" } } }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```
429 `RATE_LIMITED` (`details.retry_after_seconds`), 502 `GIT_UPSTREAM_ERROR`, 503 `GIT_FEATURE_DISABLED` (bentuk sama, `details: null` bila tidak ada).

### 17.3 DELETE `/api/me/git-credentials/{provider}?host=git.noobee.id`
204 tanpa body. 404:
```json
{ "error": { "code": "NOT_FOUND", "message": "token tidak ditemukan", "details": null }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```

### 17.4 GET `/api/projects/{projectID}/git-sync?kind=knowledge`
```json
{
  "kind": "knowledge",
  "feature_enabled": true,
  "has_credential": true,
  "credential_unreadable": false,
  "config": {
    "provider": "github", "host": "github.com",
    "repo_url": "https://github.com/noobee/grimoire-docs/tree/main/docs",
    "repo_path": "noobee/grimoire-docs", "branch": "main", "folder": "docs", "target_prefix": "",
    "last_status": "partial", "last_synced_at": "2026-10-22T14:05:00Z",
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
      "truncated": false, "error_code": "", "error_message": ""
    }
  }
}
```
Belum ada config: `{ "kind": "api_spec", "feature_enabled": true, "has_credential": false, "credential_unreadable": false, "config": null }`.
400:
```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "permintaan tidak valid", "details": { "fields": { "kind": "kind harus knowledge atau api_spec" } } }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```
404:
```json
{ "error": { "code": "NOT_FOUND", "message": "project tidak ditemukan", "details": null }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```

### 17.5 POST `/api/projects/{projectID}/git-sync/run`
Request knowledge (form):
```json
{ "kind": "knowledge", "url": "https://github.com/noobee/grimoire-docs/tree/main/docs", "branch": "", "folder": "", "target_prefix": "guides" }
```
Request api_spec (tanpa `target_prefix`):
```json
{ "kind": "api_spec", "url": "https://github.com/noobee/grimoire-docs", "branch": "main", "folder": "api/openapi" }
```
200:
```json
{
  "report": {
    "status": "partial",
    "summary": { "created": 1, "updated": 0, "unchanged": 0, "skipped": 0, "rejected": 1 },
    "items": [
      { "repo_path": "api/openapi/billing.yaml", "target": "billing", "status": "created", "reason_code": "", "reason": "", "issues": [], "openapi_version": "3.0.3", "title": "Billing API", "endpoint_count": 14 },
      { "repo_path": "api/openapi/payments.json", "target": "payments", "status": "rejected", "reason_code": "INVALID_SPEC", "reason": "$ref eksternal tidak didukung",
        "issues": [ { "code": "EXTERNAL_REF", "message": "$ref eksternal tidak didukung", "pointer": "#/paths/~1charges/get", "line": 0 } ],
        "openapi_version": "", "title": "", "endpoint_count": 0 }
    ],
    "truncated": false, "error_code": "", "error_message": ""
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
Error yang dipetakan (tabel 8.1):
```json
{ "error": { "code": "CREDENTIAL_MISSING", "message": "Anda belum menyimpan token Git untuk github.com", "details": { "provider": "github", "host": "github.com", "settings_path": "/settings#token-git" } }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```
```json
{ "error": { "code": "INVALID_REPO_URL", "message": "isi Branch", "details": { "field": "branch" } }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```
```json
{ "error": { "code": "GIT_NOT_FOUND", "message": "folder tidak ditemukan di branch ini", "details": { "what": "folder", "folder": "tidak-ada" } }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```
```json
{ "error": { "code": "SYNC_LIMIT_EXCEEDED", "message": "folder berisi 742 file, melebihi batas 500 file per sync; persempit folder", "details": { "files": 742, "max_files": 500, "known_bytes": 3145728, "max_total_bytes": 52428800 } }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```
```json
{ "error": { "code": "SYNC_IN_PROGRESS", "message": "sync lain untuk project dan jenis ini sedang berjalan", "details": { "kind": "knowledge" } }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```
```json
{ "error": { "code": "RATE_LIMITED", "message": "provider Git membatasi permintaan; coba lagi nanti", "details": { "retry_after_seconds": 60 } }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```
Kode lain (`CREDENTIAL_UNREADABLE`, `HOST_NOT_ALLOWED`, `GIT_TOKEN_REJECTED`, `GIT_ACCESS_DENIED`, `GIT_TREE_TOO_LARGE`, `GIT_UPSTREAM_ERROR`, `GIT_FEATURE_DISABLED`, `TIMEOUT`) memakai bentuk yang sama (overview 12.5).

Bentuk internal FE (bukan wire), hasil `toSyncError`:
```json
{ "code": "GIT_NOT_FOUND", "message": "folder tidak ditemukan di branch ini", "status": 404, "traceId": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33", "field": "folder" }
```
