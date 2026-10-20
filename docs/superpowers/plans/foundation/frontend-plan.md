# T00 Foundation — Frontend Plan

Tiket: `docs/tickets/T00-foundation.md` (F5, F10, F11, AC1, AC2).
Acuan mengikat: `docs/superpowers/plans/foundation/overview.md` (bagian 3.1, 8.4, 8.5, 10, 11, 13, 15) dan `usecase-static-frontend-scaffold.md` (bagian B). Penegakan import FE: `usecase-architecture-guard.md` bagian C. Aturan: `rules.txt` 3.2, 12.1-12.9, 13.
Konteks yang dicek: belum ada `CLAUDE.md`, belum ada `.claude/rules/`, belum ada folder `frontend/`. Tidak ada knowledge doc module (T00 bukan module domain).

Dokumen ini adalah SATU plan frontend untuk seluruh feature foundation. Frontend tidak dipecah per usecase. Bila ada perbedaan dengan plan usecase, plan ini menyebutnya eksplisit di bagian 2.2 ("Keputusan dan penyesuaian").

---

## 1. Metadata

| Atribut | Nilai |
|---|---|
| Ticket | T00 |
| Feature | foundation (scaffold frontend) |
| Scope | fullstack; dokumen ini hanya sisi frontend (`frontend/**`) plus titik sambung ke `infra/static` dan makefile |
| Domain | infra (bukan domain bisnis) |
| Module | foundation (bukan module domain) |
| Lokasi kode | `frontend/src/{app,shared}`; `frontend/src/domain/{core,project,discovery}/` hanya `.gitkeep` |
| Kontrak API | `api/modules/infra/foundation.yml` (`info.version: 0.1.0`): `components/schemas/Error` dan `components/responses/*` |
| Versi paket FE | `frontend/package.json` `"version": "0.1.0"` |
| Versi URL | tanpa prefix versi (`/api/...` polos) |

### 1.1 Endpoint HTTP yang relevan bagi frontend

| Endpoint | Method | Version | Dipakai FE? | Keterangan |
|---|---|---|---|---|
| `/` dan `/*` (selain `/api/**`, `/mcp/**`, `/healthz`, `/readyz`) | GET, HEAD | - | ya (dimuat browser) | `index.html` + fallback SPA dari `infra/static`; di dev diproksi ke Vite |
| `/assets/*` | GET, HEAD | - | ya (dimuat browser) | Aset ber-hash Vite, `Cache-Control: immutable` |
| `/api/**` | semua | - | lewat `shared/services/http.ts` | T00 tidak memanggil endpoint `/api` apa pun saat boot. Klien hanya disiapkan untuk tiket lain |
| `/healthz`, `/readyz` | GET | - | tidak dari UI | Dipakai Playwright `webServer.url` dan CI, bukan oleh aplikasi |
| `/mcp/**` | - | - | tidak | Bukan urusan UI |

### 1.2 Route SPA (client-side)

| Path | View | Layout | `meta` | Pemilik |
|---|---|---|---|---|
| `/` | `app/views/HomeView.svelte` | `blank` | `{ layout: 'blank' }` (publik) | T00 |
| `*` (tidak cocok) | `app/views/NotFoundView.svelte` | `blank` | - | T00 |
| `/login` | belum ada (jatuh ke NotFoundView sampai T01) | - | - | T01 |

Endpoint bus: tidak ada (FE tidak menyentuh bus). Aksi audit: tidak ada.

---

## 2. Summary Plan

Frontend T00 adalah kerangka SPA Vite + Svelte 5 + TypeScript + Tailwind v4 + Zod yang di-build ke `frontend/dist`, disalin `make build` ke `backend/internal/infra/static/dist`, lalu di-embed binary Go. Isinya:
- **app/**: bootstrap (`main.ts`, `App.svelte`), router history-API buatan sendiri, guard role (fungsi murni), dua layout, dua view kerangka (Home, NotFound), boundary error global.
- **shared/**: klien HTTP `http.ts` (cookie same-origin, header CSRF `X-Requested-With: grimoire` pada mutasi, normalisasi error seragam ke `ApiError`, redirect 401, validasi Zod), skema Zod error seragam, store sesi (kosong, diisi T01), store i18n (id/en), store toast, dan komponen dasar (Button, Input, Dialog, Toast, Table, Pagination) plus tiga komponen state (Spinner, EmptyState, ErrorState) dan LocaleSwitch.
- **domain/**: hanya `.gitkeep` per domain. Feature domain pertama dibuat T01 dengan pola `domain/<domain>/<module>/<feature>/{components,services,stores,views,types.ts,index.ts}`.
- **Tooling**: Vitest (jsdom + testing-library), Playwright smoke (AC1/AC2), `svelte-check`, struktur yang siap untuk dependency-cruiser (konfigurasi milik usecase architecture-guard).

Tidak ada SQL, tidak ada service domain, tidak ada panggilan API saat boot. Halaman `/` kosong berjudul i18n adalah bukti AC1/AC2.

### 2.1 Pola feature untuk tiket berikutnya (dibakukan di sini, belum diisi)

```
frontend/src/domain/<domain>/<module>/<feature>/
  services/<feature>.service.ts   # pakai shared/services/http.ts + schema Zod; tanpa state
  stores/<feature>.store.ts       # svelte/store; tanpa fetch
  components/*.svelte             # presentasi; tidak memanggil service
  views/<Feature>View.svelte      # komposisi tipis untuk satu route; satu-satunya yang memanggil service + store
  types.ts                        # tipe hasil z.infer dari schema
  index.ts                        # API publik: view, tipe, fungsi service yang boleh dipakai feature lain
```
Route didaftarkan di `app/routes/index.ts` dengan meng-import `index.ts` feature (aturan depcruise `outside-domain-only-via-index`). T00 menulis pola ini sebagai komentar kepala di `app/routes/index.ts`; tidak membuat feature contoh.

### 2.2 Keputusan dan penyesuaian terhadap plan usecase

1. **Toast store dipindah ke `shared/stores/toast.ts`** (usecase menaruhnya di `shared/components/toast.ts`). Alasan: rule 12.2 menempatkan state di `stores/`. `shared/components/index.ts` tetap me-re-export `toast` agar API publik sama.
2. **Tambahan komponen state**: `Spinner.svelte`, `EmptyState.svelte`, `ErrorState.svelte`, `LocaleSwitch.svelte`. Kecil, tanpa dependensi, dibutuhkan agar status loading/empty/error dan pergantian bahasa (E2E positif 4) punya implementasi baku yang dipakai ulang T01+.
3. **Boundary error global** di `App.svelte` memakai `<svelte:boundary>` (Svelte 5) dengan `ErrorState` sebagai fallback, ditambah listener `unhandledrejection` yang menampilkan toast untuk `ApiError` yang tidak ditangani.
4. **Konflik Playwright** (usecase static: "tidak memulai server sendiri"; usecase architecture-guard: "webServer di playwright.config.ts"): diselesaikan dengan `webServer` yang `reuseExistingServer: true`. Bila server sudah hidup (`make dev` atau `make test-e2e` yang menjalankan binary di background) Playwright memakainya; bila tidak (job CI `e2e`), Playwright menjalankan `../bin/grimoire`. Keduanya terpenuhi tanpa mengubah plan lain.
5. **Status sesi `unknown` di T00**: tidak ada yang mengisi sesi di T00, jadi route ber-`auth` akan `pending` terus. T00 hanya punya route publik sehingga ini tidak terlihat. T01 menambah bootstrap sesi (`/api/auth/me` dengan `skipAuthRedirect`) di `main.ts` lewat `index.ts` feature-nya. Didokumentasikan sebagai komentar `TODO(T01)` di `main.ts`.

---

## 3. Plan (langkah berurutan)

Prasyarat backend untuk verifikasi penuh: usecase `http-server` (rantai middleware, CSP) dan `static-frontend-scaffold` bagian A (handler static + proxy Vite). Langkah F1-F9 bisa dikerjakan tanpa backend (Vitest + `vite dev` langsung); F10 butuh backend.

### F1. Tooling dasar `frontend/`

1. `package.json`:
   - `"name": "grimoire-frontend"`, `"private": true`, `"version": "0.1.0"`, `"type": "module"`.
   - `dependencies`: `svelte` (^5, dipin eksak), `zod` (^3 terbaru yang stabil, dipin eksak).
   - `devDependencies` (dipin eksak, rilis stabil terbaru yang berumur > 2 minggu): `vite`, `@sveltejs/vite-plugin-svelte`, `typescript`, `svelte-check`, `tailwindcss` (^4), `@tailwindcss/vite`, `vitest`, `jsdom`, `@testing-library/svelte`, `@testing-library/jest-dom`, `@testing-library/user-event`, `@playwright/test`, `@types/node`.
   - `scripts`: `dev`=`vite`, `build`=`vite build`, `preview`=`vite preview`, `check`=`svelte-check --tsconfig ./tsconfig.json && tsc --noEmit -p tsconfig.node.json`, `test`=`vitest`, `test:e2e`=`playwright test`. (`depcruise` dan `test:arch` ditambah usecase architecture-guard; jangan didefinisikan di sini.)
   - `package-lock.json` di-commit (dibutuhkan `npm ci` di `make build` dan CI).
2. `vite.config.ts`: plugin `svelte()` dan `tailwindcss()`; `resolve.alias: { '@': fileURLToPath(new URL('./src', import.meta.url)) }`; `build: { outDir: 'dist', assetsDir: 'assets', emptyOutDir: true, sourcemap: false }`; `server: { port: 5173, strictPort: true }`. Tanpa proxy `/api` (browser membuka `:8080`, Go mem-proxy Vite).
3. `tsconfig.json`: `strict`, `noUncheckedIndexedAccess`, `moduleResolution: "bundler"`, `verbatimModuleSyntax`, `target: "ES2022"`, `paths: { "@/*": ["src/*"] }`, `types: ["svelte", "vite/client"]`, `include: ["src/**/*"]`, `exclude: ["test/**", "e2e/**"]`. `tsconfig.node.json` untuk `vite.config.ts`, `vitest.config.ts`, `playwright.config.ts`, `e2e/**`.
4. `svelte.config.js`: `preprocess: vitePreprocess()`; `compilerOptions: { runes: true }`.
5. `vitest.config.ts`: `mergeConfig(viteConfig, { test: { environment: 'jsdom', include: ['src/**/*.test.ts'], exclude: ['e2e/**', 'test/**'], setupFiles: ['src/test-setup.ts'], restoreMocks: true } })`; `resolve.conditions: ['browser']` agar Svelte 5 memakai build browser di jsdom.
6. `src/test-setup.ts`: `import '@testing-library/jest-dom/vitest'`; stub `HTMLDialogElement.prototype.showModal/close` bila jsdom belum mendukung.
7. `index.html`: `<html lang="id">`, `<meta charset>`, `<meta name="viewport">`, `<title>Grimoire</title>`, `<link rel="icon" href="/favicon.svg">`, `<div id="app"></div>`, `<script type="module" src="/src/main.ts"></script>`. Tanpa script atau style inline (CSP `script-src 'self'`).
8. `public/favicon.svg` (ikon sederhana, tanpa font eksternal).
9. `src/app.css`: `@import "tailwindcss";` + `@theme` minimal (warna `--color-primary`, `--color-danger`, `--color-surface`, `--color-muted`, radius, font `system-ui`). Dark mode lewat `prefers-color-scheme` di token. Tidak ada font eksternal (CSP `font-src 'self'`).

### F2. Struktur folder (rules 3.2; `.gitkeep` pada folder kosong)

```
frontend/
  package.json, package-lock.json, index.html, vite.config.ts, vitest.config.ts,
  tsconfig.json, tsconfig.node.json, svelte.config.js, playwright.config.ts
  public/favicon.svg
  e2e/smoke.spec.ts
  src/
    main.ts
    app.css
    test-setup.ts
    app/
      App.svelte
      routes/   index.ts, router.ts, guard.ts, router.test.ts, guard.test.ts
      layouts/  BlankLayout.svelte, AppLayout.svelte
      views/    HomeView.svelte, NotFoundView.svelte, PendingView.svelte, app.test.ts
    domain/
      core/.gitkeep
      project/.gitkeep
      discovery/.gitkeep
    shared/
      components/ Button.svelte, Input.svelte, Dialog.svelte, Toast.svelte (ToastHost),
                  Table.svelte, Pagination.svelte, Spinner.svelte, EmptyState.svelte,
                  ErrorState.svelte, LocaleSwitch.svelte, index.ts, components.test.ts
      services/   http.ts, http.test.ts
      stores/     session.ts, i18n.ts, toast.ts, i18n.test.ts, toast.test.ts, session.test.ts
      validation/ error.ts, error.test.ts
      utils/      .gitkeep
      i18n/       id.json, en.json
```

Batas import (ditegakkan depcruise dari usecase architecture-guard):
- `shared/**` tidak boleh meng-import `domain/**` maupun `app/**`.
- `domain/**` tidak boleh meng-import `app/**`; antarfeature hanya lewat `index.ts`.
- `app/**` dan `main.ts` boleh meng-import `shared/**` dan `domain/**/<feature>/index.ts` saja.

### F3. Kontrak error: `shared/validation/error.ts`

Mengikuti `components/schemas/Error` di `api/modules/infra/foundation.yml`:
```ts
export const ErrorEnvelope = z.object({
  error: z.object({
    code: z.string().min(1),
    message: z.string(),
    details: z.unknown().nullable().optional(), // server selalu kirim (null bila kosong); klien toleran
  }),
  trace_id: z.string().optional(),
});
export type ErrorEnvelope = z.infer<typeof ErrorEnvelope>;

// Kode yang diketahui server T00 + kode khusus klien. Bukan enum tertutup: module lain boleh menambah kode.
export const KnownErrorCode = {
  NOT_FOUND: 'NOT_FOUND', FORBIDDEN: 'FORBIDDEN', CONFLICT: 'CONFLICT',
  INVALID_ARGUMENT: 'INVALID_ARGUMENT', UNAUTHORIZED: 'UNAUTHORIZED', CSRF_REQUIRED: 'CSRF_REQUIRED',
  RATE_LIMITED: 'RATE_LIMITED', PAYLOAD_TOO_LARGE: 'PAYLOAD_TOO_LARGE', METHOD_NOT_ALLOWED: 'METHOD_NOT_ALLOWED',
  TIMEOUT: 'TIMEOUT', UNAVAILABLE: 'UNAVAILABLE', INTERNAL: 'INTERNAL',
  NETWORK: 'NETWORK', INVALID_RESPONSE: 'INVALID_RESPONSE', // khusus klien
} as const;

// Helper untuk form (dipakai T01+): ambil details.fields bila bentuknya Record<string,string>.
export const FieldErrors = z.object({ fields: z.record(z.string()) });
export function fieldErrorsOf(details: unknown): Record<string, string>;
```

### F4. Klien HTTP: `shared/services/http.ts`

API publik:
```ts
export class ApiError extends Error {
  readonly name = 'ApiError';
  readonly status: number;        // 0 untuk NETWORK
  readonly code: string;
  readonly details: unknown;      // null bila tidak ada
  readonly traceId?: string;
  constructor(i: { status: number; code: string; message: string; details?: unknown; traceId?: string });
}
export function isApiError(e: unknown): e is ApiError;

export interface RequestOptions<T> {
  body?: unknown;                 // objek biasa -> JSON; FormData/Blob/URLSearchParams -> apa adanya
  query?: Record<string, string | number | boolean | null | undefined>;
  schema?: z.ZodType<T>;
  signal?: AbortSignal;
  skipAuthRedirect?: boolean;     // dipakai T01 untuk login/me
  headers?: Record<string, string>;
}
export function request<T = unknown>(method: string, url: string, o?: RequestOptions<T>): Promise<T>;
export const get:   <T>(url: string, o?: Omit<RequestOptions<T>, 'body'>) => Promise<T>;
export const post:  <T>(url: string, body?: unknown, o?: RequestOptions<T>) => Promise<T>;
export const put:   <T>(url: string, body?: unknown, o?: RequestOptions<T>) => Promise<T>;
export const patch: <T>(url: string, body?: unknown, o?: RequestOptions<T>) => Promise<T>;
export const del:   <T>(url: string, o?: RequestOptions<T>) => Promise<T>;
export function setUnauthorizedHandler(fn: ((nextPath: string) => void) | null): void;
export const CSRF_HEADER = 'X-Requested-With';
export const CSRF_VALUE = 'grimoire';
```

Perilaku (urutan persis):
1. URL harus relatif same-origin (diawali `/`); URL absolut ke origin lain -> `throw new Error` (bug programmer, bukan ApiError).
2. `credentials: 'same-origin'`; header `Accept: application/json`.
3. Method di-uppercase. `X-Requested-With: grimoire` HANYA untuk `POST`, `PUT`, `PATCH`, `DELETE` (overview 8.5, F11, AC6). `GET`/`HEAD` tanpa header itu.
4. Body: objek biasa/array -> `JSON.stringify` + `Content-Type: application/json`; `FormData`/`Blob`/`URLSearchParams` -> tanpa set `Content-Type`. `undefined` -> tanpa body.
5. `query`: `URLSearchParams`, nilai `null`/`undefined` dilewati, boolean/number di-`String()`.
6. `fetch` melempar: bila `AbortError` (atau `signal.aborted`) -> lempar ulang apa adanya; selain itu -> `ApiError{ status: 0, code: 'NETWORK', message: t('error.network') }`.
7. 2xx: status 204 atau body kosong -> `undefined`. Selain itu parse JSON; JSON rusak -> `ApiError{ code: 'INVALID_RESPONSE' }`. Bila `schema` ada -> `schema.safeParse`; gagal -> `ApiError{ status, code: 'INVALID_RESPONSE', message: t('error.invalid_response'), details: issues }`.
8. Non-2xx: baca teks, coba `JSON.parse` + `ErrorEnvelope.safeParse`.
   - Sukses -> `ApiError{ status, code, message (dari server, apa adanya), details ?? null, traceId: trace_id ?? header X-Request-Id }`.
   - Gagal (HTML 502 proxy, teks polos) -> `ApiError{ status, code: 'INTERNAL', message: t('error.internal'), traceId: header X-Request-Id }`.
9. Status 401 dan `!skipAuthRedirect` dan `location.pathname !== '/login'` -> panggil handler unauthorized dengan `pathname + search` saat ini, lalu tetap `throw`. Default handler (bila belum dipasang): `window.location.assign('/login?next=' + encodeURIComponent(p))`.
10. Tanpa retry, tanpa cache, tanpa state modul selain referensi handler unauthorized.
11. Pesan default diambil dengan `get(t)(key)` dari `shared/stores/i18n` (bukan string hard-coded).

### F5. Store bersama (`shared/stores/`)

**`session.ts`**
```ts
export type Role = 'super_admin' | 'developer';
export interface Identity { userId: string; email: string; name: string; role: Role; mustChangePassword: boolean }
export type SessionStatus = 'unknown' | 'anonymous' | 'authenticated';
export interface SessionState { status: SessionStatus; identity: Identity | null }
export const session: Readable<SessionState>;            // awal { status: 'unknown', identity: null }
export function setIdentity(i: Identity | null): void;    // null -> anonymous; dipanggil T01
export function resetSession(): void;                     // kembali ke unknown (test, logout T01)
```
Tidak ada fetch (12.2). Tidak menyimpan ke localStorage (identitas berasal dari cookie HttpOnly via `/api` di T01).

**`i18n.ts`**
```ts
export type Locale = 'id' | 'en';
export const SUPPORTED_LOCALES: readonly Locale[];
export const locale: Writable<Locale>;
export const t: Readable<(key: string, params?: Record<string, string | number>) => string>;
export function setLocale(l: Locale): void;
```
- Locale awal: `localStorage['grimoire.locale']` (try/catch, hanya nilai valid) -> `navigator.language` berawalan `en` -> `en` -> default `id`.
- `setLocale` menyimpan ke localStorage (try/catch, gagal diabaikan) dan mengubah `document.documentElement.lang`.
- Interpolasi `{nama}`; kunci hilang -> kembalikan kunci, `console.warn` hanya saat `import.meta.env.DEV`; locale `en` tanpa kunci -> jatuh ke `id` lalu ke kunci.
- Kamus JSON datar bertitik di `shared/i18n/{id,en}.json`, di-import statis (dua kamus kecil, tidak perlu lazy load).

Kunci minimal (identik di dua file):
```
app.name, app.loading,
home.title, home.subtitle,
notfound.title, notfound.description, notfound.back,
common.save, common.cancel, common.close, common.confirm, common.loading, common.empty, common.retry, common.reload,
error.network, error.internal, error.invalid_response, error.unauthorized, error.forbidden,
error.unexpected.title, error.unexpected.description, error.trace_id,
table.empty, pagination.prev, pagination.next, pagination.page_of, pagination.label,
toast.close, locale.label, locale.id, locale.en
```

**`toast.ts`**
```ts
export type ToastType = 'success' | 'error' | 'info';
export interface ToastItem { id: number; type: ToastType; message: string; traceId?: string; timeoutMs: number }
export const toasts: Readable<ToastItem[]>;
export const toast: {
  push(i: { type: ToastType; message: string; traceId?: string; timeoutMs?: number }): number; // default 5000; error 8000; 0 = tidak auto-tutup
  dismiss(id: number): void;
  fromError(e: unknown): number; // ApiError -> message + traceId; selain itu t('error.internal')
  clear(): void;
};
```
Maksimal 5 toast tampil; yang tertua dibuang. Timer dibersihkan saat `dismiss`.

### F6. Komponen bersama (`shared/components/`)

Semua Svelte 5 runes (`$props`, `$bindable`, snippet), Tailwind v4 utility, semua teks lewat `$t`, tanpa dependensi UI eksternal, tanpa memanggil service (12.2).

| Komponen | Props / snippet | State & perilaku | Aksesibilitas |
|---|---|---|---|
| `Button` | `variant: 'primary'\|'secondary'\|'danger'\|'ghost'` (default primary), `size: 'sm'\|'md'`, `type` (default `button`), `disabled`, `loading`, `onclick`, snippet `children` | `loading` -> disabled + `Spinner` kecil | `aria-busy` saat loading; fokus terlihat (`focus-visible:ring`) |
| `Input` | `value` (`$bindable`), `label`, `name`, `type`, `placeholder`, `hint?`, `error?`, `required`, `disabled`, `autocomplete?` | id unik (`$props.id()` atau counter) | `<label for>`; `aria-invalid` bila `error`; `aria-describedby` ke hint/error |
| `Dialog` | `open` (`$bindable`), `title`, `onclose?`, snippet `children`, `footer?` | `<dialog>` + `showModal()`/`close()` di `$effect`; Esc dan klik backdrop menutup; fokus kembali ke elemen pemicu | `aria-labelledby` ke judul; tombol tutup `aria-label={$t('common.close')}` |
| `ToastHost` (`Toast.svelte`) | - | berlangganan `toasts`; tombol tutup per item; tampilkan `traceId` kecil bila ada | region `role="status"` `aria-live="polite"`; item error `role="alert"` |
| `Table<T>` | `columns: {key, header, class?}[]` (header = kunci i18n atau teks dari caller), `rows: T[]`, `rowKey`, snippet `cell?(row, col)`, `loading`, `error?: ApiError\|null`, `emptyText?`, `caption?`, `onretry?` | 3 status: loading -> baris skeleton/`Spinner`; error -> `ErrorState` dalam satu sel `colspan`; kosong -> `EmptyState` (`emptyText ?? $t('table.empty')`) | `<caption>` (bisa `sr-only`); `aria-busy` saat loading |
| `Pagination` | `page` (1-based), `pageSize`, `total`, `onchange(page)` | `pages = max(1, ceil(total/pageSize))`; prev disabled di 1, next disabled di `pages` | `<nav aria-label={$t('pagination.label')}>`; `aria-current="page"` |
| `Spinner` | `size?`, `label?` (default `$t('common.loading')`) | animasi CSS; hormati `prefers-reduced-motion` | `role="status"` + teks `sr-only` |
| `EmptyState` | `title?`, `description?`, snippet `action?` | default `$t('common.empty')` | heading semantik |
| `ErrorState` | `error: unknown`, `onretry?` | tampilkan `message` (ApiError) atau `t('error.internal')`; tampilkan `trace_id` bila ada; tombol retry bila `onretry` | `role="alert"` |
| `LocaleSwitch` | - | `<select>` id/en terikat `locale` -> `setLocale` | `<label>` `sr-only` `$t('locale.label')` |

`index.ts` mengekspor semua komponen + re-export `toast` dari `shared/stores/toast`.

### F7. Router, guard, dan rute (`app/routes/`)

**`index.ts`**
```ts
export interface RouteMeta { auth?: boolean; roles?: Role[]; layout?: 'blank' | 'app'; titleKey?: string }
export interface RouteDef { path: string; view: Component<any>; meta?: RouteMeta }
export const routes: RouteDef[] = [
  { path: '/', view: HomeView, meta: { layout: 'blank', titleKey: 'home.title' } },
];
export const notFoundRoute: RouteDef = { path: '*', view: NotFoundView, meta: { layout: 'blank', titleKey: 'notfound.title' } };
```
Komentar kepala berisi pola feature (bagian 2.1) dan cara mendaftarkan route dari `domain/**/index.ts`.

**`router.ts`**
```ts
export interface Location { path: string; query: URLSearchParams; fullPath: string }
export interface Match { route: RouteDef; params: Record<string, string> }
export const current: Readable<{ location: Location; match: Match | null }>;
export function navigate(to: string, opts?: { replace?: boolean }): void;
export function match(path: string, defs: RouteDef[]): Match | null;  // murni
export function startRouter(defs: RouteDef[]): () => void;          // pasang popstate + intersep klik; kembalikan fungsi lepas
```
- Pencocokan: segmen statis dan `:param` (decodeURIComponent), trailing slash diabaikan, tanpa wildcard selain fallback.
- Intersep klik `<a>`: hanya klik kiri tanpa modifier, `target` kosong/`_self`, same-origin, tanpa `download`, tanpa `data-router-ignore`; lalu `preventDefault()` + `navigate`.
- `navigate` ke path sama tidak menambah entri history.
- Setelah navigasi: `document.title = $t(titleKey) + ' · ' + $t('app.name')`; scroll ke atas.
- `startRouter` juga memanggil `setUnauthorizedHandler(p => navigate('/login?next=' + encodeURIComponent(p), { replace: true }))`.

**`guard.ts`** (fungsi murni; komentar "guard hanya UX, otorisasi di backend - rules 12.7")
```ts
export type Resolution =
  | { kind: 'render'; route: RouteDef; params: Record<string, string> }
  | { kind: 'redirect'; to: string }
  | { kind: 'pending' }
  | { kind: 'notfound' };
export function resolve(m: Match | null, s: SessionState, fullPath: string): Resolution;
```
Urutan aturan:
1. `m === null` -> `notfound`.
2. Route tidak butuh auth (`!meta.auth && !meta.roles`) -> `render`.
3. `s.status === 'unknown'` -> `pending`.
4. `s.status === 'anonymous'` -> `redirect` ke `/login?next=<encodeURIComponent(fullPath)>`.
5. `meta.roles` ada dan `s.identity.role` tidak termasuk -> `notfound` (rule 7.3: tidak membocorkan keberadaan).
6. Selain itu -> `render`.

### F8. Komposisi aplikasi (`app/`)

- **`App.svelte`**: membaca `$current` dan `$session`, memanggil `resolve`.
  - `redirect` -> `navigate(to, { replace: true })` di `$effect` (tidak render apa pun di antaranya).
  - `pending` -> layout `blank` + `PendingView` (Spinner tengah layar, `aria-busy="true"`, teks `$t('app.loading')`).
  - `notfound` -> layout `blank` + `NotFoundView`.
  - `render` -> layout sesuai `meta.layout` (default `app`) + `<route.view params={params} />`.
  - Dibungkus `<svelte:boundary onerror={log}>` dengan snippet `failed(error, reset)` yang merender `ErrorState` (judul `error.unexpected.title`, tombol `common.reload` memanggil `location.reload()`, tombol `common.retry` memanggil `reset`).
  - `<ToastHost />` di luar boundary agar tetap tampil saat view gagal.
- **`layouts/BlankLayout.svelte`**: `<main id="main" class="min-h-dvh ...">` + `LocaleSwitch` kecil di pojok + snippet `children`.
- **`layouts/AppLayout.svelte`**: kerangka kosong: tautan "skip to content" (`sr-only focus:not-sr-only`), `<header>` berisi `$t('app.name')` dan `LocaleSwitch`, `<main id="main">`, snippet `children`. Navigasi ditambah tiket lain lewat props/snippet, bukan di T00.
- **`views/HomeView.svelte`**: `<h1 data-testid="home-title">{$t('home.title')}</h1>` + `<p>{$t('home.subtitle')}</p>`. Tidak memanggil API.
- **`views/NotFoundView.svelte`**: `<h1 data-testid="notfound-title">{$t('notfound.title')}</h1>`, deskripsi, tautan `<a href="/">{$t('notfound.back')}</a>`.
- **`views/PendingView.svelte`**: Spinner + `app.loading`.
- **`main.ts`**: import `app.css`; `startRouter(routes)`; `mount(App, { target: document.getElementById('app')! })`; `window.addEventListener('unhandledrejection', e => { if (isApiError(e.reason)) { toast.fromError(e.reason); e.preventDefault(); } })`. Komentar `TODO(T01): bootstrap sesi lewat domain/core/auth/<feature>/index.ts`.

### F9. State, loading, empty, dan error (ringkasan baku untuk semua tiket)

| Situasi | Tempat | Tampilan |
|---|---|---|
| Boot, sesi `unknown`, route butuh auth | App | `PendingView` |
| Route tidak dikenal / role tidak cocok | App + guard | `NotFoundView` (200 dari server berkat fallback SPA) |
| Anonim ke route ber-auth | guard | redirect `/login?next=...` (sampai T01: jatuh ke NotFoundView) |
| Data tabel dimuat | `Table loading` | skeleton/Spinner, `aria-busy` |
| Data tabel kosong | `Table` | `EmptyState` (`table.empty`) |
| Gagal muat data | `Table error` / `ErrorState` | pesan ApiError + trace id + tombol retry |
| Aksi mutasi gagal | caller | `toast.fromError(e)` |
| 401 dari API | `http.ts` | redirect `/login?next=` (kecuali `skipAuthRedirect`, kecuali sudah di `/login`) |
| 403 `CSRF_REQUIRED` | `http.ts` | ApiError biasa (bug klien; tampil sebagai toast oleh caller), tidak redirect |
| Jaringan putus | `http.ts` | ApiError `NETWORK` (`error.network`) |
| Respons tidak sesuai skema | `http.ts` | ApiError `INVALID_RESPONSE` |
| Exception render yang tak tertangani | `<svelte:boundary>` | `ErrorState` + reload/retry |
| Promise ApiError tak tertangani | `main.ts` | toast error |
| Vite mati di dev | server Go | 502 teks polos dari proxy (bukan urusan SPA) |

### F10. Integrasi build dan E2E

1. `playwright.config.ts`:
   ```ts
   const baseURL = process.env.E2E_BASE_URL ?? 'http://localhost:8080';
   export default defineConfig({
     testDir: 'e2e', fullyParallel: true, retries: process.env.CI ? 1 : 0,
     use: { baseURL, trace: 'retain-on-failure' },
     projects: [{ name: 'chromium', use: devices['Desktop Chrome'] }],
     webServer: process.env.E2E_BASE_URL ? undefined : {
       command: '../bin/grimoire', url: `${baseURL}/healthz`,
       reuseExistingServer: true, timeout: 60_000,
       env: { HTTP_ADDR: '127.0.0.1:8080', COOKIE_SECURE: 'false' }, // DATABASE_URL & SESSION_SECRET dari env proses (CI)
     },
   });
   ```
   Bila `E2E_BASE_URL` di-set, Playwright tidak menyentuh server (mode `make dev` atau server eksternal).
2. `e2e/smoke.spec.ts`: skenario bagian 7 yang ditandai `[pw]`.
3. Titik sambung makefile (target milik usecase cli-and-wiring/static): `fe-build` (`npm ci && npm run build` lalu salin `frontend/dist/.` ke `backend/internal/infra/static/dist/`), `fe-restore-placeholder`. FE plan tidak mengubah resep `build`, `test`, `lint`; hanya memastikan skrip npm yang dipanggil (`build`, `test -- --run`, `check`, `test:e2e`) ada.
4. Verifikasi CSP di dev: Vite dev menyuntikkan `<script type="module" src="/@vite/client">` (bukan inline) sehingga cocok dengan `script-src 'self'`; HMR lewat `ws:` diizinkan oleh CSP dev (overview 8.4). Bila ada plugin yang menyuntik script inline, plugin itu tidak boleh dipakai.

---

## 4. Migration

Tidak ada. Frontend tidak memiliki migrasi, dan T00 secara keseluruhan tidak menambah file `YYYYMMDDHHMMSSmmm_*.sql` (overview bagian 6).

---

## 5. Changes (per file/folder)

Baru (`frontend/`):
- `package.json`, `package-lock.json`, `index.html`, `vite.config.ts`, `vitest.config.ts`, `tsconfig.json`, `tsconfig.node.json`, `svelte.config.js`, `playwright.config.ts`
- `public/favicon.svg`
- `e2e/smoke.spec.ts`
- `src/main.ts`, `src/app.css`, `src/test-setup.ts`
- `src/app/App.svelte`
- `src/app/routes/index.ts`, `router.ts`, `guard.ts`, `router.test.ts`, `guard.test.ts`
- `src/app/layouts/BlankLayout.svelte`, `AppLayout.svelte`
- `src/app/views/HomeView.svelte`, `NotFoundView.svelte`, `PendingView.svelte`, `app.test.ts`
- `src/domain/core/.gitkeep`, `src/domain/project/.gitkeep`, `src/domain/discovery/.gitkeep`
- `src/shared/components/Button.svelte`, `Input.svelte`, `Dialog.svelte`, `Toast.svelte`, `Table.svelte`, `Pagination.svelte`, `Spinner.svelte`, `EmptyState.svelte`, `ErrorState.svelte`, `LocaleSwitch.svelte`, `index.ts`, `components.test.ts`
- `src/shared/services/http.ts`, `http.test.ts`
- `src/shared/stores/session.ts`, `i18n.ts`, `toast.ts`, `session.test.ts`, `i18n.test.ts`, `toast.test.ts`
- `src/shared/validation/error.ts`, `error.test.ts`
- `src/shared/utils/.gitkeep`
- `src/shared/i18n/id.json`, `en.json`

Diubah (hanya bila belum dilakukan usecase lain):
- `.gitignore` (root): `frontend/node_modules/`, `frontend/dist/`, `frontend/playwright-report/`, `frontend/test-results/`, `backend/internal/infra/static/dist/*` + `!backend/internal/infra/static/dist/index.html`.
- `makefile`: target `fe-build`, `fe-restore-placeholder` (resep `build`/`dev`/`test`/`lint` final milik usecase cli-and-wiring dan architecture-guard).
- `docs/configuration.md`: bagian "Frontend" (`make dev` membuka `http://localhost:8080`, bukan `:5173`; `make build`; cara memulihkan placeholder; env Playwright `E2E_BASE_URL`). `E2E_BASE_URL` dicatat di bagian env test, bukan tabel Config (tidak memengaruhi test AC7).
- `docs/superpowers/plans/foundation/usecase-static-frontend-scaffold.md`: catatan singkat yang merujuk penyesuaian bagian 2.2 (toast store, komponen state, webServer Playwright).

Bukan milik plan ini (disebut agar tidak dobel): `frontend/.dependency-cruiser.cjs`, `frontend/scripts/test-arch.mjs`, `frontend/test/depcruise-fixtures/**`, skrip `depcruise`/`test:arch` (usecase architecture-guard); `backend/internal/infra/static/**` (usecase static bagian A).

Tidak diubah: `api/**` (FE hanya membaca kontrak), `rules.txt`, `docs/tickets/*`, kode backend.

---

## 6. Test Scenario

Vitest (jsdom). `fetch` di-stub dengan `vi.stubGlobal('fetch', vi.fn())`; `history`/`location` jsdom asli; fake timers untuk toast. Semua test berjalan via `npm test -- --run` dan di CI job `frontend`.

### 6.1 `shared/services/http.test.ts`
1. `adds X-Requested-With only on POST/PUT/PATCH/DELETE` (tabel method; GET/HEAD tidak punya header; nilai persis `grimoire`). (AC6 sisi klien)
2. `lowercase method still treated as mutation` (`'post'` -> header ada).
3. `always uses credentials same-origin` dan `Accept: application/json`.
4. `serializes plain object as JSON with content-type`; `FormData body has no content-type`; `undefined body sends no body`.
5. `query skips null/undefined and encodes values` (`{a:1,b:undefined,c:'x y'}` -> `?a=1&c=x+y`).
6. `rejects absolute cross-origin URL` (throw Error, bukan ApiError).
7. `normalizes uniform error envelope`: 400 `INVALID_ARGUMENT` + `details.fields` + `trace_id:'t1'` -> ApiError `{status:400, code, message, details, traceId:'t1'}`.
8. `details missing -> null`.
9. `non-uniform error body -> INTERNAL`: 502 `text/html` -> `code:'INTERNAL'`, `status:502`, `traceId` dari header `X-Request-Id`, message = `t('error.internal')` locale aktif.
10. `network failure -> NETWORK status 0`.
11. `abort propagates as AbortError` (bukan ApiError).
12. `401 calls unauthorized handler with pathname+search then throws` (set `history.pushState({}, '', '/projects/1?tab=key')` -> handler menerima `/projects/1?tab=key`).
13. `401 with skipAuthRedirect does not call handler`.
14. `401 while on /login does not call handler` (anti loop).
15. `default unauthorized handler uses location.assign` (spy) bila handler belum dipasang.
16. `403 CSRF_REQUIRED maps to ApiError without redirect`.
17. `2xx with schema returns parsed data`; `schema mismatch -> INVALID_RESPONSE with issues in details`; `invalid JSON on 2xx -> INVALID_RESPONSE`.
18. `204 and empty 200 -> undefined`.
19. `server message preserved verbatim; locale only affects client-generated messages` (ganti locale `en`, ulang kasus 9 dan 10).

### 6.2 `shared/validation/error.test.ts`
- Envelope valid dengan `details: null`, objek, array -> lolos; tanpa `trace_id` -> lolos; `code` kosong -> gagal; bukan objek -> gagal.
- `fieldErrorsOf({fields:{name:'wajib'}})` -> `{name:'wajib'}`; input lain -> `{}`.

### 6.3 `shared/stores/i18n.test.ts`
- Default `id` (localStorage kosong, `navigator.language='id-ID'`).
- `navigator.language='en-US'` tanpa localStorage -> `en`.
- localStorage `'en'` menang atas navigator; nilai invalid (`'fr'`) diabaikan.
- `setLocale('en')` mengubah output `$t`, menyimpan ke localStorage, dan set `document.documentElement.lang='en'`.
- Interpolasi `pagination.page_of` dengan `{page:2,pages:5}`.
- Kunci hilang -> kunci itu sendiri (+ `console.warn` di DEV); kunci hanya ada di `id` saat locale `en` -> nilai `id`.
- `localStorage` melempar (getter/setItem di-stub throw) -> tidak crash, default `id`.
- `id.json` dan `en.json` punya himpunan kunci identik dan tidak ada nilai kosong.

### 6.4 `shared/stores/session.test.ts` dan `toast.test.ts`
- Session awal `unknown`; `setIdentity(x)` -> `authenticated`; `setIdentity(null)` -> `anonymous`; `resetSession` -> `unknown`.
- Toast: `push` menambah dan auto-hilang setelah 5000 ms (info) / 8000 ms (error); `timeoutMs:0` tidak hilang; `dismiss` membersihkan timer; maksimal 5 item; `fromError(ApiError)` membawa `message` + `traceId`; `fromError(new Error())` memakai `error.internal`.

### 6.5 `app/routes/guard.test.ts` (tabel)
| Kasus | Session | Route | Hasil |
|---|---|---|---|
| route null | apa pun | - | `notfound` |
| publik | `unknown` | `/` | `render` |
| publik | `anonymous` | `/` | `render` |
| auth | `unknown` | `meta.auth` | `pending` |
| auth | `anonymous` | `/projects/1?tab=key` | `redirect` ke `/login?next=%2Fprojects%2F1%3Ftab%3Dkey` |
| roles | `authenticated` developer | `roles:['super_admin']` | `notfound` |
| roles | `authenticated` super_admin | `roles:['super_admin']` | `render` |
| auth | `authenticated` developer | `meta.auth` | `render` |

### 6.6 `app/routes/router.test.ts`
- `match`: `/` cocok `/`; `/projects/:id` cocok `/projects/abc` dengan `params.id='abc'`; `/projects/a%20b` di-decode; trailing slash diabaikan; tidak cocok -> null.
- `navigate` memakai `pushState`; `{replace:true}` memakai `replaceState`; navigate ke path sama tidak menambah entri.
- `popstate` memperbarui `current`; query ter-parse ke `URLSearchParams`.
- Intersep klik `<a href="/x">` -> `preventDefault` + `current.location.path === '/x'`; tidak diintersep untuk `data-router-ignore`, `target="_blank"`, origin lain, klik dengan `ctrlKey/metaKey`, `download`.
- `startRouter` memasang unauthorized handler yang memanggil `navigate('/login?next=...', {replace:true})`; fungsi lepas mencabut listener.

### 6.7 `shared/components/components.test.ts` (@testing-library/svelte)
- Button: `loading` -> `disabled` + `aria-busy="true"` + Spinner; `onclick` tidak terpanggil saat disabled; `type` default `button`.
- Input: label terhubung (`getByLabelText`); `error` -> `aria-invalid="true"` dan `aria-describedby` menunjuk teks error; binding `value` berubah saat mengetik.
- Dialog: `open=true` memanggil `showModal`; Esc memanggil `onclose` dan `open=false`; tombol tutup punya `aria-label` i18n; fokus kembali ke pemicu.
- ToastHost: menampilkan item dari store; error memakai `role="alert"`; trace id tampil; tombol tutup menghapus.
- Table: `loading` -> `aria-busy` + spinner; `rows=[]` -> teks `table.empty`; `error` -> ErrorState dengan tombol retry memanggil `onretry`; snippet `cell` dipakai.
- Pagination: halaman 1 prev disabled; halaman terakhir next disabled; klik next memanggil `onchange(2)`; `total=0` -> `pages=1`, keduanya disabled; teks `page_of`.
- EmptyState/ErrorState/Spinner: teks default dari i18n; ErrorState menampilkan `trace_id` dari ApiError.
- LocaleSwitch: memilih `en` memanggil `setLocale('en')` dan teks komponen lain ikut berubah.

### 6.8 `app/views/app.test.ts` (render `App.svelte` penuh)
- Path `/` -> `home-title` tampil dengan teks `id`.
- Path `/zzz` -> `notfound-title` tampil; klik tautan kembali -> Home tanpa reload (spy `location.assign` tidak dipanggil).
- Route uji ber-`auth` disuntik lewat props/test-only defs: sesi `unknown` -> PendingView (`aria-busy`); `setIdentity(null)` -> URL berubah ke `/login?next=...` via `replaceState`.
- View yang melempar saat render -> boundary menampilkan `error.unexpected.title`, ToastHost tetap ada.
- `document.title` = `home.title · app.name`.

### 6.9 Statis dan tooling
- `npm run check` (svelte-check + tsc) tanpa error dan tanpa warning a11y.
- `npm run build` menghasilkan `dist/index.html` + `dist/assets/index-<hash>.js|css`; tidak ada `<script>` inline di `dist/index.html` (asersi kecil di `app.test.ts` atau langkah CI: `grep -c '<script>' dist/index.html` = 0).
- depcruise (dari usecase architecture-guard) lulus pada `src/`.
- Review manual: tidak ada string UI hard-coded di `.svelte` (rule 12.8).

---

## 7. E2E Scenarios

Playwright (chromium) terhadap server Go (`make dev` atau binary hasil `make build`). Kasus bertanda `[pw]` diotomatiskan di `e2e/smoke.spec.ts`; sisanya checklist manual/CI shell di plan usecase terkait.

Positif:
1. `[pw]` **T00-AC1/AC2 home**: `GET /` -> respons 200, `home-title` terlihat dengan teks kamus `id`, `<html lang="id">`, tidak ada `console.error`/`pageerror`, tidak ada respons same-origin >= 400.
2. `[pw]` **Cache header**: respons dokumen `/` punya `cache-control: no-cache`; setiap request `/assets/*` yang dimuat halaman punya `immutable`.
3. `[pw]` **Deep link fallback**: `GET /projects/abc` langsung -> 200 HTML, NotFoundView tampil (belum ada route projects), tanpa error console.
4. `[pw]` **Navigasi klien**: dari NotFoundView klik "kembali" -> Home tanpa reload penuh (tanda `window.__navMarker` yang diset sebelum klik masih ada).
5. `[pw]` **Locale**: pilih `en` di LocaleSwitch -> judul berbahasa Inggris dan `lang="en"`; reload -> tetap `en` (localStorage).
6. `[pw]` **Riwayat**: Home -> `/zzz` (lewat `page.evaluate` navigate atau tautan uji) -> tombol Back browser kembali ke Home.
7. Manual (AC1): `make dev`, ubah teks di `HomeView.svelte` -> HMR lewat proxy Go `:8080` tanpa reload penuh; CSP tidak memblokir `@vite/client` maupun websocket.
8. Manual/CI shell (AC2): binary di folder kosong menyajikan bundle nyata (`/` memuat `/assets/index-`).

Negatif:
1. `[pw]` **Aset hilang**: `request.get('/assets/missing.js')` -> 404 dan body bukan HTML (tidak mengandung `<div id="app">`).
2. `[pw]` **API tidak dikenal**: `request.get('/api/does-not-exist')` -> 404, `content-type` JSON, body lolos `ErrorEnvelope` dengan `code: 'NOT_FOUND'` dan `trace_id` tidak kosong (memvalidasi kontrak yang dipakai `http.ts` terhadap server nyata).
3. `[pw]` **CSRF**: `request.post('/api/anything')` tanpa header -> 403 `CSRF_REQUIRED`; dengan header `X-Requested-With: grimoire` -> bukan 403 CSRF (404 JSON). (AC6 end-to-end)
4. `[pw]` **localStorage diblokir**: `page.addInitScript` membuat `localStorage` melempar -> halaman tetap tampil dalam `id`, tanpa `pageerror`.
5. `[pw]` **Cookie sesi sebelum T01**: request dengan cookie `grimoire_session=abc` ke `/` -> SPA tetap tampil (static tidak lewat session middleware) dan tidak ada panggilan `/api` dari UI, sehingga tidak ada error console.
6. Manual: `make dev` dengan Vite mati -> `/` 502 teks petunjuk; ini perilaku server, SPA tidak terlibat.
7. Vitest (bukan Playwright, karena belum ada endpoint ber-auth): 401 dari `/api` mengarahkan ke `/login?next=...`; diuji di 6.1 no. 12-15.

---

## 8. Request/Response body

### 8.1 Format error seragam (dari `api/modules/infra/foundation.yml`, dipakai `ErrorEnvelope`)
```json
{
  "error": {
    "code": "INVALID_ARGUMENT",
    "message": "permintaan tidak valid",
    "details": { "fields": { "name": "wajib diisi" } }
  },
  "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33"
}
```

### 8.2 404 `/api` tidak dikenal
```http
GET /api/does-not-exist HTTP/1.1
Accept: application/json
```
```json
{
  "error": { "code": "NOT_FOUND", "message": "resource tidak ditemukan", "details": null },
  "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33"
}
```

### 8.3 403 CSRF (mutasi tanpa header)
```http
POST /api/projects HTTP/1.1
Content-Type: application/json

{"name":"Grimoire"}
```
```json
{
  "error": {
    "code": "CSRF_REQUIRED",
    "message": "header X-Requested-With wajib untuk request mutasi",
    "details": null
  },
  "trace_id": "0b1c2d3e-0000-4000-8000-000000000001"
}
```

### 8.4 Request mutasi yang benar dari `http.ts`
```http
POST /api/projects HTTP/1.1
Accept: application/json
Content-Type: application/json
X-Requested-With: grimoire
Cookie: grimoire_session=...

{"name":"Grimoire"}
```
`GET` tidak membawa `X-Requested-With` maupun `Content-Type`.

### 8.5 Error lain yang harus ditangani klien
```json
{ "error": { "code": "UNAUTHORIZED", "message": "perlu login", "details": null }, "trace_id": "..." }
```
```json
{ "error": { "code": "FORBIDDEN", "message": "akses ditolak", "details": null }, "trace_id": "..." }
```
```json
{ "error": { "code": "PAYLOAD_TOO_LARGE", "message": "ukuran body melebihi batas", "details": { "limit_bytes": 1048576 } }, "trace_id": "..." }
```
```json
{ "error": { "code": "RATE_LIMITED", "message": "terlalu banyak permintaan, coba lagi nanti", "details": { "retry_after_seconds": 3 } }, "trace_id": "..." }
```
```json
{ "error": { "code": "TIMEOUT", "message": "permintaan melebihi batas waktu", "details": null }, "trace_id": "..." }
```
```json
{ "error": { "code": "INTERNAL", "message": "terjadi kesalahan internal", "details": null }, "trace_id": "..." }
```

### 8.6 Hasil normalisasi `ApiError` di klien
Dari 8.1 (status 400):
```ts
ApiError { status: 400, code: 'INVALID_ARGUMENT', message: 'permintaan tidak valid',
           details: { fields: { name: 'wajib diisi' } }, traceId: '4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33' }
```
Kode yang dibuat klien (tidak pernah dikirim server):
```ts
ApiError { status: 0,   code: 'NETWORK',          message: 'Tidak dapat terhubung ke server.', details: null }
ApiError { status: 502, code: 'INTERNAL',         message: 'Terjadi kesalahan pada server.',   details: null, traceId: '<X-Request-Id bila ada>' }
ApiError { status: 200, code: 'INVALID_RESPONSE', message: 'Respons server tidak sesuai format.', details: [ /* ZodIssue[] */ ] }
```
(Teks `message` di atas adalah nilai `id.json` untuk `error.network`, `error.internal`, `error.invalid_response`; versi `en` di `en.json`.)

### 8.7 Health (dipakai Playwright `webServer.url`, bukan UI)
```json
{ "status": "ok" }
```

### 8.8 Respons static yang diamati E2E
```
GET /                  -> 200 text/html; charset=utf-8, Cache-Control: no-cache
GET /projects/abc      -> 200 text/html (fallback index.html), Cache-Control: no-cache
GET /assets/index-3fa9c1.js -> 200 text/javascript, Cache-Control: public, max-age=31536000, immutable
GET /assets/missing.js -> 404 text/plain "404 page not found"
```

### 8.9 Contoh kamus i18n (`shared/i18n/id.json`, potongan)
```json
{
  "app.name": "Grimoire",
  "app.loading": "Memuat…",
  "home.title": "Grimoire",
  "home.subtitle": "Pusat knowledge dan API spec tim.",
  "notfound.title": "Halaman tidak ditemukan",
  "notfound.back": "Kembali ke beranda",
  "error.network": "Tidak dapat terhubung ke server.",
  "error.internal": "Terjadi kesalahan pada server.",
  "error.invalid_response": "Respons server tidak sesuai format.",
  "error.trace_id": "Kode jejak: {id}",
  "table.empty": "Belum ada data.",
  "pagination.page_of": "Halaman {page} dari {pages}"
}
```
