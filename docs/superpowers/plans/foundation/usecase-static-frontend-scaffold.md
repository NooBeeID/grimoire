# T00 Foundation — Usecase `static-frontend-scaffold`

Overview: `docs/superpowers/plans/foundation/overview.md` (mengikat; dokumen ini tidak boleh bertentangan). Tiket: `docs/tickets/T00-foundation.md` (F5, F10, AC1, AC2). Aturan: `rules.txt` 3.2, 4.6, 12.1-12.9.

---

## 1. Metadata

| Atribut | Nilai |
|---|---|
| Ticket | T00 |
| Usecase | `static-frontend-scaffold` (langkah 6 di overview) |
| Scope | fullstack (Go `infra/static` + `frontend/`) |
| Domain | infra |
| Module | foundation |
| Resource root | `backend/internal/infra` (Go) dan `frontend/` |
| Bergantung | langkah 1 (`config`, `logger`, `errs`), langkah 5 (`httpx.WriteError` untuk 404 JSON `/api` dan `/mcp`) |
| Dipakai oleh | langkah 7 (`cli-and-wiring` memasang handler sebagai NotFound root; `make build` menyalin dist) |

| Endpoint | Method | Version | Auth | Keterangan |
|---|---|---|---|---|
| `/` dan `/*` selain `/api/**`, `/mcp/**`, `/healthz`, `/readyz` | GET, HEAD | - | publik | File ter-embed, atau fallback SPA, atau proxy Vite (dev) |
| `/assets/*` | GET, HEAD | - | publik | Aset ber-hash, `immutable` |
| `/api/**` dan `/mcp/**` yang sampai ke handler static | semua | - | - | Tidak pernah dilayani static: 404 JSON `NOT_FOUND` |
| path lain, method selain GET/HEAD | POST dst. | - | - | 405 `METHOD_NOT_ALLOWED` + header `Allow: GET, HEAD` |

Endpoint bus: tidak ada (provider maupun consumer). Migrasi: tidak ada. Aksi audit: tidak ada. Tidak ada perubahan `api/modules/**` (static bukan bagian kontrak REST). Tidak ada env baru: `DEV_VITE_URL` dan `APP_ENV` sudah didefinisikan langkah 1 (tabel overview 9); usecase ini hanya memakainya.

---

## 2. Summary Plan

Usecase ini punya dua bagian yang satu sama lain koheren (satu PR): (a) paket Go `infra/static` yang menyajikan SPA ter-embed dengan fallback, header cache, dan proxy Vite di mode dev; (b) scaffold `frontend/` (Vite + Svelte 5 + TS + Tailwind v4 + Zod) berisi klien HTTP, router + guard role, i18n, dan komponen dasar, yang `vite build`-nya menghasilkan `frontend/dist` untuk disalin `make build` ke `backend/internal/infra/static/dist`.

Tidak ada entity, usecase, repository, atau SQL. Tidak dipecah lagi: handler static hanya satu `http.Handler` dengan tabel keputusan; sisi frontend adalah satu scaffold. `go build` tanpa FE tetap jalan karena `dist/index.html` placeholder di-commit.

Keputusan khusus usecase ini:
1. `static.New` TIDAK mendaftar route sendiri. Ia mengembalikan `http.Handler`; langkah 7 memasangnya sebagai `root.NotFound(h.ServeHTTP)` dan `root.MethodNotAllowed` tidak diubah. Dengan begitu `/healthz`, `/readyz`, `/api`, `/mcp` yang terdaftar menang atas static, dan static hanya menerima sisa. Guard `/api`/`/mcp` di handler tetap ada sebagai pertahanan berlapis (mis. `/api/zzz` yang lolos ke root NotFound).
2. 404 untuk `/api`/`/mcp` memakai `httpx.WriteError(w, r, errs.New(errs.CodeNotFound, "resource tidak ditemukan"))` agar format error seragam (imports infra->infra diizinkan, tidak melanggar 4.6).
3. Aset hilang: path berekstensi apa pun (termasuk di bawah `/assets/`) -> 404 teks polos `404 page not found`; path di bawah `/assets/` tanpa ekstensi juga 404 (bukan fallback), agar bundle lama tidak mendapat HTML sebagai JS.
4. Proxy dev hanya aktif bila `cfg.AppEnv == "development" && cfg.DevViteURL != ""` (config sudah menolak `DEV_VITE_URL` di production). Di mode ini `fsys` diabaikan.
5. Router frontend buatan sendiri (history API), i18n buatan sendiri; tidak menambah library router. Satu-satunya dependensi runtime FE: `svelte`, `zod`. Dev: `vite`, `@sveltejs/vite-plugin-svelte`, `typescript`, `svelte-check`, `tailwindcss`, `@tailwindcss/vite`, `vitest`, `jsdom`, `@testing-library/svelte`, `@playwright/test`, `dependency-cruiser` (config milik langkah 8; di sini hanya struktur folder yang menyiapkannya). Versi dipin tepat di `package.json` + `package-lock.json` di-commit (pilih rilis stabil terbaru saat implementasi; Svelte `^5`, Tailwind `^4`).

---

## 3. Plan (langkah berurutan)

### Bagian A — Go `infra/static`

**A1. Placeholder dan embed**
1. Buat `backend/internal/infra/static/dist/index.html` (di-commit): HTML minimal valid dengan `<title>Grimoire</title>` dan `<div id="app"></div>` serta komentar `<!-- placeholder; diganti hasil make build -->`. Ini yang membuat `go:embed all:dist` selalu punya isi.
2. `.gitignore` (sudah dijadwalkan langkah 1: `backend/internal/infra/static/dist/*` kecuali `index.html`). Verifikasi baris: `backend/internal/infra/static/dist/*` lalu `!backend/internal/infra/static/dist/index.html`. Jika langkah 1 belum memuatnya, tambahkan di sini (file `.gitignore` hanya baris itu).
3. `static.go`:
   ```go
   package static

   //go:embed all:dist
   var distFS embed.FS

   // Embedded mengembalikan sub-FS berakar di dist.
   func Embedded() fs.FS // fs.Sub(distFS, "dist"); panic bila gagal (tak mungkin)

   // New membuat handler. fsys boleh nil -> Embedded().
   func New(cfg config.Config, fsys fs.FS, log *slog.Logger) (http.Handler, error)
   ```
   `New` error bila: `fsys` tidak punya `index.html` (pesan: `static: index.html tidak ditemukan di FS`), atau `DevViteURL` tidak bisa di-parse saat mode dev.

**A2. Handler (tabel keputusan, urutan persis)**
Normalisasi: `p := path.Clean("/" + r.URL.Path)`; tolak (404 teks) bila `r.URL.Path` mengandung `\x00` atau `..` setelah decode (Clean sudah menetralkan; tetap uji traversal). Kunci FS = `strings.TrimPrefix(p, "/")`, kosong -> `index.html`.

| # | Kondisi | Hasil |
|---|---|---|
| 1 | `p == "/api"` atau awalan `/api/`; sama untuk `/mcp` | 404 JSON seragam (`httpx.WriteError`), semua method, dievaluasi PERTAMA (juga di mode dev) |
| 2 | mode dev proxy aktif (dan lolos baris 1) | `proxy.ServeHTTP` (lihat A3), semua method diteruskan agar HMR/Vite berfungsi |
| 3a | method bukan GET/HEAD | 405 JSON seragam `METHOD_NOT_ALLOWED`, `Allow: GET, HEAD` |
| 3 | kunci FS ada dan bukan direktori | sajikan file; `Content-Type` dari ekstensi (`mime.TypeByExtension`, fallback sniff); bila awalan `assets/` -> `Cache-Control: public, max-age=31536000, immutable`; `index.html` -> `Cache-Control: no-cache`; selain itu (mis. `favicon.svg`, `robots.txt`) -> `Cache-Control: no-cache` |
| 4 | `p` diawali `/assets/` (file tak ada) | 404 teks polos (`http.NotFound`) |
| 5 | `path.Ext(p) != ""` (file berekstensi tak ada) | 404 teks polos |
| 6 | selain itu (tanpa ekstensi, tak ada di FS) | sajikan `index.html` 200, `Cache-Control: no-cache`, `Content-Type: text/html; charset=utf-8` |

Catatan implementasi: baca file lewat `fs.ReadFile` + `http.ServeContent` (nama, modtime nol, `bytes.NewReader`) agar HEAD, Range, dan ETag via `If-None-Match` tidak perlu kode sendiri; jangan pakai `http.FileServer` (redirect `/index.html` -> `/` dan listing direktori tidak diinginkan). Permintaan `/index.html` langsung: sajikan 200 (tanpa redirect). Direktori (`/assets`, `/assets/`) -> tabel baris 4 (404) bila di bawah assets, selain itu fallback baris 6.

**A3. Proxy dev**
- `httputil.ReverseProxy{Rewrite: func(pr *httputil.ProxyRequest){ pr.SetURL(target); pr.Out.Host = target.Host; pr.SetXForwarded() }}`. `ReverseProxy` Go meneruskan `Upgrade: websocket` (HMR) otomatis.
- `ErrorHandler`: log warn `vite dev server tidak terjangkau` + `trace_id`; balas 502 teks `vite dev server tidak berjalan; jalankan npm run dev di frontend/`.
- Header keamanan: CSP dev ditangani `SecurityHeaders` (overview 8.4), bukan di sini.
- Proxy tidak pernah menerima `/api` atau `/mcp`: baris 1 (guard) dievaluasi sebelum baris 2 (proxy). Tes `TestStatic_DevProxy` memverifikasi Vite palsu tidak menerima request `/api/x`.

**A4. Wiring (dilakukan langkah 7, dicatat di sini sebagai kontrak)**
`h, err := static.New(cfg, nil, log); root.NotFound(h.ServeHTTP)`. Handler tidak dibungkus middleware tambahan; ia sudah di bawah rantai root (Recover, TraceID, AccessLog, SecurityHeaders). Karena `BodyLimit` root memengaruhi hanya body, tidak ada dampak.

### Bagian B — Scaffold `frontend/`

**B1. Tooling dasar** (`frontend/`)
- `package.json`: `"type":"module"`; scripts: `dev` (`vite`), `build` (`vite build`), `preview`, `check` (`svelte-check --tsconfig ./tsconfig.json && tsc --noEmit -p tsconfig.node.json`), `test` (`vitest`), `test:e2e` (`playwright test`), `test:arch` (placeholder dibuat langkah 8; jangan didefinisikan di sini).
- `vite.config.ts`: plugin `svelte()` dan `tailwindcss()`; `build: { outDir: 'dist', assetsDir: 'assets', emptyOutDir: true, sourcemap: false }`; `server: { port: 5173, strictPort: true }`. Tanpa proxy `/api` di Vite: di dev browser membuka `http://localhost:8080` (Go mem-proxy Vite), jadi cookie dan `/api` satu origin. Output hash default Vite (`assets/index-<hash>.js`) sudah sesuai aturan immutable.
- `tsconfig.json` (strict, `moduleResolution: bundler`, `verbatimModuleSyntax`, alias `@/*` -> `src/*`), `tsconfig.node.json`, `svelte.config.js` (`vitePreprocess`), `vitest.config.ts` (environment `jsdom`, `include: src/**/*.test.ts`, exclude `e2e/`), `playwright.config.ts` (lihat B8).
- `index.html` (root Vite): `<html lang="id">`, `<div id="app">`, `<script type="module" src="/src/main.ts">`. Tanpa inline script (CSP `script-src 'self'`).
- `src/app.css`: `@import "tailwindcss";` dan token tema (`@theme`) minimal. Tidak ada font eksternal (CSP `font-src 'self'`).
- `.gitignore` frontend tidak perlu (root sudah mengabaikan `node_modules`, `dist`, `playwright-report`, `test-results`).

**B2. Struktur folder** (sesuai rules 3.2; `.gitkeep` pada folder kosong)
```
frontend/src/
  main.ts
  app.css
  app/
    App.svelte
    routes/{index.ts,router.ts,guard.ts,*.test.ts}
    layouts/{BlankLayout.svelte,AppLayout.svelte}
    views/{HomeView.svelte,NotFoundView.svelte}
  domain/{core,project,discovery}/.gitkeep
  shared/
    components/{Button,Input,Dialog,Toast,Table,Pagination}.svelte, toast.ts, index.ts
    services/{http.ts,http.test.ts}
    stores/{session.ts,i18n.ts,i18n.test.ts}
    validation/error.ts
    utils/.gitkeep
    i18n/{id.json,en.json}
frontend/e2e/smoke.spec.ts
```
Aturan: `shared/` DILARANG meng-import `domain/` (12.5). `app/` boleh meng-import `shared/` dan `domain/*/index.ts`. Views di `app/views` hanya untuk halaman kerangka; view fitur nanti milik tiket lain di `domain/**/views` dan diregistrasi di `app/routes/index.ts`.

**B3. `shared/validation/error.ts`**
```ts
export const ErrorEnvelope = z.object({
  error: z.object({ code: z.string(), message: z.string(), details: z.unknown().nullable().optional() }),
  trace_id: z.string().optional(),
});
```

**B4. `shared/services/http.ts`**
```ts
export class ApiError extends Error {
  code: string; status: number; details: unknown; traceId?: string;
  constructor(i: { code: string; message: string; status: number; details?: unknown; traceId?: string })
}
export interface RequestOptions<T> {
  body?: unknown;                 // objek -> JSON; FormData/Blob -> apa adanya
  query?: Record<string, string | number | boolean | undefined | null>;
  schema?: z.ZodType<T>;
  signal?: AbortSignal;
  skipAuthRedirect?: boolean;     // login/me
}
export function request<T = unknown>(method: string, url: string, o?: RequestOptions<T>): Promise<T>;
export const get, post, put, patch, del;           // helper
export function setUnauthorizedHandler(fn: (nextPath: string) => void): void; // default: navigate ke /login?next=
```
Perilaku:
1. `credentials: 'same-origin'` selalu. Header `Accept: application/json`.
2. `X-Requested-With: grimoire` HANYA bila method (uppercase) di {POST, PUT, PATCH, DELETE}. GET/HEAD tidak.
3. Body objek biasa -> `JSON.stringify` + `Content-Type: application/json`; `FormData` -> tanpa Content-Type (browser set boundary). `query` di-encode, nilai undefined/null dilewati.
4. `fetch` melempar (jaringan) -> `ApiError{code:'NETWORK', status:0, message: t('error.network')}`. Abort tetap dilempar apa adanya (`AbortError`), bukan dibungkus.
5. 204 atau body kosong pada 2xx -> `undefined`.
6. Non-2xx: coba `res.json()`; `ErrorEnvelope.safeParse` sukses -> `ApiError{code,message,details,traceId,status}`; gagal (body bukan JSON seragam, mis. HTML 502) -> `code:'INTERNAL'`, `message: t('error.internal')`, `traceId` dari header `X-Request-Id` bila ada.
7. Status 401 dan `!skipAuthRedirect` -> panggil handler (default `window.location.assign('/login?next=' + encodeURIComponent(pathname+search))` lewat `router.navigate` bila router sudah terpasang; hindari loop bila sudah di `/login`), lalu tetap `throw` ApiError.
8. 2xx dengan `schema`: `schema.safeParse(json)`; gagal -> `ApiError{code:'INVALID_RESPONSE', status: res.status, details: zodError.issues}`.
9. Pesan error default memakai `t(...)` dari i18n store (bukan string hard-coded); `message` dari server dipertahankan apa adanya bila ada.
10. Tidak menyimpan state; tidak ada retry.

`http.ts` mengimpor `shared/stores/i18n` (sesama shared). Tidak boleh mengimpor `app/` (cegah siklus): redirect default dilakukan lewat handler yang dipasang `app/routes/router.ts` saat bootstrap (`setUnauthorizedHandler(p => navigate('/login?next=' + encodeURIComponent(p), {replace:true}))`); default bila belum dipasang: `window.location.assign`.

**B5. `shared/stores/session.ts`**
```ts
export type Role = 'super_admin' | 'developer';
export interface Identity { userId: string; email: string; name: string; role: Role; mustChangePassword: boolean }
export const session: Writable<{ status: 'unknown'|'anonymous'|'authenticated'; identity: Identity|null }>;
export function setIdentity(i: Identity | null): void; // diisi T01
```
Awal: `status:'unknown'`. Tidak ada fetch di store (12.2). T00 tidak memanggil `/api` apa pun saat boot, jadi halaman kosong tidak menghasilkan 401/404 di console.

**B6. Router dan guard** (`app/routes/`)
- `index.ts`: 
  ```ts
  export interface RouteMeta { auth?: boolean; roles?: Role[]; layout?: 'blank'|'app' }
  export interface RouteDef { path: string; view: Component; meta?: RouteMeta }
  export const routes: RouteDef[]  // T00: '/' -> HomeView (meta: {layout:'blank'}); fallback NotFoundView
  ```
- `router.ts`: store `current` (`{path, query, route|null}`); `navigate(to, {replace?})` memakai `history.pushState/replaceState`; listener `popstate`; intersep klik `<a>` same-origin (`data-router-ignore` untuk lewati). Pencocokan path: segmen statis dan `:param`; tanpa regex rumit.
- `guard.ts`: `export function resolve(route: RouteDef|null, s: SessionState, fullPath: string): { kind:'render'; route } | { kind:'redirect'; to:string } | { kind:'notfound' }`. Aturan:
  1. route null -> `notfound`.
  2. `meta.auth || meta.roles` dan `s.status==='anonymous'` -> `redirect /login?next=<fullPath terenkode>`.
  3. status `unknown` pada route yang butuh auth -> `render` sementara ditahan (`kind:'pending'` ditambahkan; App menampilkan layout kosong sampai T01 mengisi session). Tambahkan varian `{kind:'pending'}`.
  4. `meta.roles` ada dan role identitas tidak termasuk -> `notfound` (rule 7.3, tidak bocor).
  5. selain itu `render`.
  Fungsi murni agar mudah dites. Komentar di file: guard hanya UX; otorisasi sebenarnya di backend (12.7).
- Route `/login` TIDAK dibuat T00 (milik T01); redirect ke `/login` tanpa route akan jatuh ke NotFoundView sampai T01, dan itu diterima.
- `layouts/BlankLayout.svelte` (slot saja, `<main>` dengan `role="main"`), `AppLayout.svelte` (kerangka kosong: `<header>`, `<main>`, slot; tanpa nav, nav ditambah tiket lain).
- `App.svelte`: berlangganan `current` + `session`, memanggil `resolve`, merender layout + view, `<ToastHost />` di akar.
- `views/HomeView.svelte`: `<h1>{$t('home.title')}</h1>` + `<p>{$t('home.subtitle')}</p>`, `data-testid="home-title"`. `NotFoundView.svelte`: judul `t('notfound.title')`, tautan kembali ke `/`.

**B7. i18n** (`shared/stores/i18n.ts`, `shared/i18n/{id,en}.json`)
- `locale: Writable<'id'|'en'>`; awal: `localStorage['grimoire.locale']` (try/catch, nilai valid saja), lalu `navigator.language` berawalan `en` -> `en`, default `id`. Perubahan locale disimpan (try/catch) dan memperbarui `document.documentElement.lang`.
- `t: Readable<(key: string, params?: Record<string, string|number>) => string>` (derived dari locale, supaya komponen reaktif: `$t('key')`). Interpolasi `{nama}`. Kunci hilang -> kembalikan kunci, `console.warn` hanya bila `import.meta.env.DEV`.
- Kamus JSON datar bertitik. Kunci minimal: `app.name`, `home.title`, `home.subtitle`, `notfound.title`, `notfound.back`, `common.{save,cancel,close,loading,empty,confirm}`, `error.{network,internal,invalid_response,unauthorized}`, `table.empty`, `pagination.{prev,next,page_of}`, `toast.close`.
- Test kelengkapan: `id.json` dan `en.json` memiliki himpunan kunci identik.

**B8. Komponen dasar** (`shared/components/`; semua teks dari `$t`, tanpa string hard-coded; aksesibel; Tailwind v4 utility; tanpa dependensi UI)
- `Button.svelte`: props `variant: 'primary'|'secondary'|'danger'|'ghost'`, `size`, `type` (default `button`), `disabled`, `loading` (menonaktifkan + `aria-busy`), `onclick`; snippet `children`.
- `Input.svelte`: props `value` (`$bindable`), `label`, `type`, `name`, `placeholder`, `error?`, `hint?`, `required`, `disabled`; `<label for>` terhubung via id unik, `aria-invalid`, `aria-describedby` ke error/hint.
- `Dialog.svelte`: props `open` (`$bindable`), `title`, `onclose`; snippets `children`, `footer`. Memakai elemen `<dialog>` + `showModal()`/`close()`; Esc menutup; fokus kembali ke pemicu; `aria-labelledby`; tombol tutup dengan `aria-label={$t('common.close')}`.
- `Toast.svelte` + `toast.ts`: store `toasts`, `toast.push({type:'success'|'error'|'info', message, timeoutMs?=5000})`, `toast.dismiss(id)`; `<ToastHost>` merender daftar dengan `role="status"`/`aria-live="polite"` (`assertive` untuk error). Pesan error boleh diisi dari `ApiError.message` + `traceId` (tampilkan trace id kecil agar mudah dilaporkan).
- `Table.svelte`: generik `columns: {key, header, class?}[]`, `rows: T[]`, `rowKey: (r:T)=>string`, snippet `cell(row, col)` opsional, `loading`, `emptyText?` (default `$t('table.empty')`); `<caption>` opsional untuk aksesibilitas.
- `Pagination.svelte`: props `page` (1-based), `pageSize`, `total`, `onchange(page)`; tombol prev/next dengan `disabled` di batas; teks `t('pagination.page_of', {page, pages})`; `nav aria-label`.
- `shared/components/index.ts` mengekspor semuanya.

**B9. Bootstrap**
- `main.ts`: impor `app.css`; `mount(App, { target: document.getElementById('app')! })`; pasang `setUnauthorizedHandler`.

**B10. Playwright** (`playwright.config.ts`, `e2e/smoke.spec.ts`)
- `use.baseURL = process.env.E2E_BASE_URL ?? 'http://localhost:8080'`; satu project chromium; tidak memulai server sendiri (dijalankan `make dev` atau `make test-e2e` dari overview 11).
- Smoke T00-AC1: buka `/`, harapkan status 200, `data-testid=home-title` terlihat dengan teks dari i18n `id`, tidak ada `console.error`/`pageerror`, tidak ada respons >= 400 untuk request same-origin.

### Bagian C — Makefile (potongan milik usecase ini; makefile lengkap oleh langkah 7)
Target `build` (overview 11) memanggil, dalam urutan: `cd frontend && npm ci && npm run build`; `rm -rf backend/internal/infra/static/dist/* && cp -R frontend/dist/. backend/internal/infra/static/dist/`; baru `go build`. Usecase ini menyumbang target `fe-build` (dua langkah pertama) dan memastikan `dist/index.html` placeholder TIDAK hilang di working tree git setelah build: karena hasil build menimpa `index.html`, tambahkan target `fe-restore-placeholder` (`git checkout -- backend/internal/infra/static/dist/index.html`) yang dipakai developer sebelum commit, dan sebutkan di `docs/configuration.md` bagian "Frontend". Pendekatan yang dipilih: file `index.html` tetap dilacak (tanpa `assume-unchanged`), dan CI (`build` job) bekerja di checkout bersih sehingga tidak ada masalah; developer lokal cukup `git checkout` file itu. Dokumentasikan, jangan otomatiskan.

---

## 4. Migration

Tidak ada.

---

## 5. Changes (per file/folder)

Baru:
- `backend/internal/infra/static/static.go`
- `backend/internal/infra/static/static_test.go`
- `backend/internal/infra/static/dist/index.html` (placeholder, dilacak git)
- `frontend/package.json`, `frontend/package-lock.json`, `frontend/index.html`, `frontend/vite.config.ts`, `frontend/tsconfig.json`, `frontend/tsconfig.node.json`, `frontend/svelte.config.js`, `frontend/vitest.config.ts`, `frontend/playwright.config.ts`
- `frontend/src/main.ts`, `frontend/src/app.css`
- `frontend/src/app/App.svelte`
- `frontend/src/app/routes/{index.ts,router.ts,guard.ts,guard.test.ts,router.test.ts}`
- `frontend/src/app/layouts/{BlankLayout.svelte,AppLayout.svelte}`
- `frontend/src/app/views/{HomeView.svelte,NotFoundView.svelte}`
- `frontend/src/domain/{core,project,discovery}/.gitkeep`
- `frontend/src/shared/components/{Button,Input,Dialog,Toast,Table,Pagination}.svelte`, `toast.ts`, `index.ts`, dan `components.test.ts` (render dasar)
- `frontend/src/shared/services/{http.ts,http.test.ts}`
- `frontend/src/shared/stores/{session.ts,i18n.ts,i18n.test.ts}`
- `frontend/src/shared/validation/error.ts`
- `frontend/src/shared/utils/.gitkeep`
- `frontend/src/shared/i18n/{id.json,en.json}`
- `frontend/e2e/smoke.spec.ts`

Diubah (hanya bila belum ada dari langkah 1/7):
- `.gitignore`: pastikan `frontend/node_modules/`, `frontend/dist/`, `backend/internal/infra/static/dist/*` + `!backend/internal/infra/static/dist/index.html`, `playwright-report/`, `test-results/`.
- `makefile`: target `fe-build`, `fe-restore-placeholder` (target `build` final milik langkah 7).
- `docs/configuration.md`: bagian "Frontend" (cara `make dev`, bahwa browser membuka `:8080`, `make build`, catatan placeholder). Tidak menambah env baru, sehingga tes AC7 tidak terpengaruh.

Tidak diubah: `rules.txt`, `docs/tickets/*`, `api/**`, kode domain, overview.

---

## 6. Test Scenario

### Go: `backend/internal/infra/static/static_test.go`
Fixture: `fstest.MapFS{ "index.html": "<html>INDEX</html>", "assets/app-abc123.js": "console.log(1)", "assets/app-abc123.css": "a{}", "favicon.svg": "<svg/>" }`, handler dibuat dengan `config.Config{AppEnv:"production"}`.
- `TestStatic_RootServesIndexNoCache`: GET `/` -> 200, body berisi INDEX, `Content-Type` HTML, `Cache-Control: no-cache`.
- `TestStatic_IndexHTMLDirect`: GET `/index.html` -> 200 tanpa redirect.
- `TestStatic_SPAFallback`: GET `/projects/abc`, `/admin/users/1/` -> 200 body INDEX, `no-cache`.
- `TestStatic_AssetsImmutable`: GET `/assets/app-abc123.js` -> 200, `Cache-Control: public, max-age=31536000, immutable`, `Content-Type` JavaScript; css sama.
- `TestStatic_NonAssetFileNoCache`: `/favicon.svg` -> 200 `no-cache`.
- `TestStatic_MissingAssetWithExtension404`: `/assets/missing.js`, `/foo.png`, `/assets/noext` -> 404 dan body BUKAN INDEX.
- `TestStatic_NeverHandlesAPIOrMCP`: GET/POST `/api`, `/api/x`, `/api/x/y.js`, `/mcp`, `/mcp/x` -> 404, `Content-Type: application/json`, body memuat `"code":"NOT_FOUND"` dan `trace_id`; body bukan INDEX. `/apix` BUKAN `/api` -> fallback 200.
- `TestStatic_PostReturns405`: POST `/` -> 405 + `Allow: GET, HEAD`, JSON `METHOD_NOT_ALLOWED`.
- `TestStatic_HeadRequest`: HEAD `/` -> 200 tanpa body, header sama.
- `TestStatic_PathTraversal`: `/../etc/passwd`, `/%2e%2e/x`, `/assets/../index.html` tidak keluar FS (hasil 200 index/404 sesuai tabel, tidak ada isi di luar fixture).
- `TestStatic_ConditionalGet`: kirim `If-Modified-Since`/ETag bila ada -> tidak 5xx (sanity `ServeContent`).
- `TestNew_ErrorWithoutIndex`: MapFS tanpa `index.html` -> error.
- `TestStatic_DevProxy`: `httptest.Server` sebagai Vite palsu (balas `vite:<path>`; juga menerima upgrade WebSocket sederhana dengan `Connection: Upgrade` dan membalas 101 lalu echo, atau minimal memeriksa header `Upgrade` diteruskan); config `AppEnv:"development"`, `DevViteURL: fake.URL` -> GET `/`, `/src/main.ts`, `/@vite/client` diproksi; `/api/x` TETAP 404 JSON dan Vite palsu tidak menerima request; header `Host` yang diterima Vite = host target.
- `TestStatic_DevProxyUnreachable`: URL target tertutup -> 502 teks.
- `TestStatic_ProductionIgnoresViteURL`: `AppEnv:"production"` dengan `DevViteURL` terisi (dibuat langsung tanpa `config.Load`) -> tidak memproksi.
- `TestT00_AC2_EmbeddedFSServes`: `static.New(cfg, nil, log)` memakai `Embedded()` nyata (placeholder atau hasil build); dipasang sebagai `NotFound` pada router `httpx.NewRouter` penuh dengan `Pinger` palsu: `/` dan `/some/route` -> 200 HTML; `/healthz` -> 200 JSON (bukan HTML); `/api/nope` -> 404 JSON; `/assets/nonexistent.js` -> 404. (Tes ini bergantung langkah 5; bila dikerjakan paralel, gunakan `chi.NewRouter` sederhana dengan `/healthz` palsu lebih dulu lalu ganti.)
- `TestEmbedded_HasIndex`: `fs.Stat(Embedded(), "index.html")` ok.
Jalankan dengan `-race`.

### Vitest (`frontend`, jsdom, `fetch` di-stub dengan `vi.stubGlobal`)
`http.test.ts`:
- `sends X-Requested-With only on mutations`: POST/PUT/PATCH/DELETE memiliki header `grimoire`; GET tidak (periksa `init.headers`).
- `uses credentials same-origin` untuk semua method.
- `serializes JSON body and sets content-type`; `FormData` tidak diberi Content-Type.
- `appends query and skips undefined`.
- `normalizes uniform error`: respons 400 `{error:{code:'INVALID_ARGUMENT',message:'x',details:{fields:{name:'wajib'}}},trace_id:'t1'}` -> `ApiError` dengan `code`, `message`, `details`, `traceId:'t1'`, `status:400`.
- `non-uniform body -> INTERNAL`: 502 HTML -> `code:'INTERNAL'`, `status:502`, `traceId` dari header `X-Request-Id` bila ada.
- `network failure -> NETWORK` (`status:0`).
- `abort propagates` sebagai `AbortError`.
- `401 triggers unauthorized handler with current path` lalu tetap throw; `skipAuthRedirect:true` tidak memanggil handler; tidak memanggil handler bila sudah di `/login` (tanpa loop).
- `zod schema failure -> INVALID_RESPONSE`; sukses mengembalikan data ter-parse.
- `204 -> undefined`.
- `403 CSRF_REQUIRED` dipetakan biasa (tanpa redirect).
`guard.test.ts`: anonim ke route `auth` -> redirect `/login?next=%2Fprojects%2F1%3Ftab%3Dkey`; status `unknown` -> `pending`; route publik anonim -> render; role tidak cocok (developer ke route `roles:['super_admin']`) -> `notfound`; role cocok -> render; route null -> `notfound`; `roles` tanpa identitas tapi `authenticated`? (tidak mungkin) diabaikan.
`router.test.ts`: `navigate` memanggil `pushState`, `replace` memakai `replaceState`, popstate memperbarui `current`, pencocokan `:param`, query ter-parse, klik `<a>` same-origin diintersep dan `data-router-ignore` tidak.
`i18n.test.ts`: default `id`; ganti locale mengubah output `t`; interpolasi; kunci hilang -> kunci; kunci `id.json` == kunci `en.json`; localStorage melempar -> tetap jalan.
`components.test.ts` (testing-library): Button `loading` -> disabled + `aria-busy`; Input menghubungkan label dan error via `aria-describedby`; Dialog membuka/menutup dan memanggil `onclose` saat Esc; Toast muncul lalu hilang setelah timeout (fake timers); Table menampilkan teks kosong i18n; Pagination menonaktifkan prev di halaman 1 dan next di halaman terakhir, memanggil `onchange`.
Tes statis (di `i18n.test.ts` atau skrip kecil): tidak ada teks literal di `.svelte` komponen dasar (grep sederhana atas node teks non-whitespace di luar `{...}`) - opsional bila murah; minimal tinjau saat review.

### Playwright
- `smoke.spec.ts` (T00-AC1), lihat B10.

---

## 7. E2E Scenarios

Positif:
1. **T00-AC1** `make dev` -> buka `http://localhost:8080/` -> 200, judul i18n terlihat, console bersih, tidak ada request gagal. Ubah teks di `HomeView.svelte` -> halaman terbarui via HMR (websocket lewat proxy Go) tanpa reload penuh.
2. **T00-AC2** `make build` -> salin `bin/grimoire` ke folder kosong, jalankan dengan `DATABASE_URL` + `SESSION_SECRET` -> `GET /` 200 HTML bundel nyata (bukan placeholder: mengandung `/assets/index-`), `GET /projects/abc` 200 HTML, `GET /assets/<hash>.js` 200 `immutable`, `GET /` punya `no-cache`, `GET /healthz` 200.
3. Navigasi klien: dari `/` ke path tak dikenal (mis. `/zzz`) menampilkan NotFoundView tanpa reload; refresh langsung di `/zzz` tetap 200 (fallback) dan menampilkan NotFoundView.
4. Ganti locale `en` lalu reload -> judul berbahasa Inggris (tersimpan di localStorage); localStorage diblokir -> tetap `id` tanpa crash.
5. Build tanpa FE: `git clone` bersih lalu `go build ./...` (tanpa `npm`) sukses dan binary menyajikan placeholder di `/`.

Negatif:
1. `GET /assets/missing.js` -> 404 teks polos (bukan HTML).
2. `GET /api/does-not-exist` -> 404 JSON `NOT_FOUND` + `trace_id`; `GET /mcp/x` (sebelum T07) -> 404 JSON.
3. `POST /` -> 405 JSON; `DELETE /projects/abc` -> 405.
4. `make dev` dengan Vite mati -> `GET /` 502 dengan pesan petunjuk; `/healthz` tetap 200.
5. Produksi dengan `DEV_VITE_URL` terisi -> gagal start oleh config (tercakup langkah 1; diverifikasi lagi di sini lewat tes `ProductionIgnoresViteURL` bahwa handler tidak memproksi).
6. Request `/api/x` yang menghasilkan 401 dari UI (setelah T01) mengarahkan ke `/login?next=...`; di T00 diverifikasi lewat Vitest dengan fetch palsu.

---

## 8. Request/Response body

### `GET /` (embedded) -> 200
```
HTTP/1.1 200 OK
Content-Type: text/html; charset=utf-8
Cache-Control: no-cache
X-Request-Id: 0b1c...
```
Body: isi `index.html` hasil build.

### `GET /assets/index-3fa9c1.js` -> 200
```
Content-Type: text/javascript; charset=utf-8
Cache-Control: public, max-age=31536000, immutable
```

### `GET /assets/missing.js` -> 404 (teks polos)
```
404 page not found
```

### `GET /api/does-not-exist` (atau path `/api/**` yang sampai ke static) -> 404
```json
{
  "error": { "code": "NOT_FOUND", "message": "resource tidak ditemukan", "details": null },
  "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33"
}
```

### `POST /` -> 405 (header `Allow: GET, HEAD`)
```json
{
  "error": { "code": "METHOD_NOT_ALLOWED", "message": "method tidak diizinkan", "details": null },
  "trace_id": "0b1c2d3e-0000-4000-8000-000000000001"
}
```

### Dev, Vite tidak berjalan -> 502 (teks polos)
```
vite dev server tidak berjalan; jalankan npm run dev di frontend/
```

### Kontrak klien: `ApiError` hasil normalisasi (TypeScript)
Respons server:
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
menjadi:
```ts
ApiError {
  status: 400,
  code: 'INVALID_ARGUMENT',
  message: 'permintaan tidak valid',
  details: { fields: { name: 'wajib diisi' } },
  traceId: '4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33'
}
```
Kode sisi klien (bukan dari server): `NETWORK` (status 0), `INTERNAL` (body bukan format seragam), `INVALID_RESPONSE` (gagal skema Zod).

### Request mutasi dari klien
```
POST /api/projects HTTP/1.1
Content-Type: application/json
X-Requested-With: grimoire
Cookie: grimoire_session=...
```
`GET` tidak membawa `X-Requested-With`.
