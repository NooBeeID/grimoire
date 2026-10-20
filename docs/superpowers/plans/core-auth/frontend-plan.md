# T01 core/auth: Frontend Plan (Svelte)

Tiket: `docs/tickets/T01-core-auth.md`. Plan backend: `overview.md` dan `usecase-*.md` di folder ini. Dokumen ini adalah SATU-SATUNYA plan frontend T01 dan mencakup seluruh feature. Bila slice FE di plan usecase (`usecase-getme.md` F1–F7, `usecase-listusers.md` FE-1–FE-5/APP-1, `usecase-getuser.md` F1) berbeda dengan dokumen ini, dokumen ini yang menang (lihat "Keputusan dan selisih").

## Metadata

| Atribut | Nilai |
|---|---|
| Ticket | T01 |
| Scope | fullstack (dokumen ini: frontend saja) |
| Domain | core |
| Module | auth (`frontend/src/domain/core/auth/`) |
| Kontrak API | `api/modules/core/auth.yml` (`info.version: 0.1.0`), dibuat backend F1 |
| Versi URL | tanpa prefix versi (`/api/...`) |
| Route UI baru | `/login`, `/change-password`, `/profile`, `/admin/users`; `/` menjadi wajib login |
| Akses UI | Menu dan guard hanya UX; otorisasi di backend (rule 12.7) |

Endpoint yang dipakai frontend:

| Endpoint | Method | Version | Dipanggil oleh feature FE | Header khusus |
|---|---|---|---|---|
| `/api/auth/login` | POST | - | `login` | `X-Requested-With` (otomatis `http.ts`), `skipAuthRedirect` |
| `/api/auth/logout` | POST | - | `logout` | `X-Requested-With`, `skipAuthRedirect` |
| `/api/auth/me` | GET | - | `getme` (bootstrap) | `skipAuthRedirect` |
| `/api/auth/change-password` | POST | - | `changepassword` | `X-Requested-With` |
| `/api/users` | GET | - | `users` (daftar) | - |
| `/api/users` | POST | - | `users` (buat) | `X-Requested-With` |
| `/api/users/{id}` | GET | - | `users` (detail untuk edit) | - |
| `/api/users/{id}` | PATCH | - | `users` (edit) | `X-Requested-With` |
| `/api/users/{id}/deactivate` | POST | - | `users` (status) | `X-Requested-With` |
| `/api/users/{id}/activate` | POST | - | `users` (status) | `X-Requested-With` |
| `/api/users/{id}/reset-password` | POST | - | `users` (reset) | `X-Requested-With` |

Endpoint bus (`core.auth.resolve-session`, `get-user`, `find-developers`, `create-admin`), job `purgesessions`, dan CLI `create-admin` tidak punya UI.

Konteks yang dimuat: tidak ada `CLAUDE.md` dan `.claude/rules/`. Sumber: `rules.txt` (bagian 3.2, 12, 13), `docs/tickets/README.md` (bagian "Frontend (Svelte)", "Kontrak API"), tiket T01, `docs/superpowers/specs/2026-10-20-grimoire-design.md`, `overview.md`, semua `usecase-*.md`, dan kode FE T00 di `main`: `shared/services/http.ts` (`request/get/post/patch`, `ApiError`, `isApiError`, `skipAuthRedirect`, `setUnauthorizedHandler`), `shared/stores/{session,i18n,toast}.ts`, `shared/validation/error.ts` (`ErrorEnvelope`, `KnownErrorCode`, `fieldErrorsOf`), `shared/components/{Button,Input,Dialog,Table,Pagination,Spinner,EmptyState,ErrorState,Toast,LocaleSwitch}`, `app/routes/{router,guard,index}.ts`, `app/App.svelte`, `app/layouts/{AppLayout,BlankLayout}.svelte`, `.dependency-cruiser.cjs`, `e2e/smoke.spec.ts`, `playwright.config.ts`.

## Summary Plan

Frontend T01 menambah lima feature di `frontend/src/domain/core/auth/`:

| Feature FE | Isi | Endpoint |
|---|---|---|
| `getme` | Schema `Me`, `fetchMe`, `loadSession` (bootstrap sesi saat aplikasi mulai) | `GET /api/auth/me` |
| `login` | Halaman login, pesan error generik/terkunci/rate limit | `POST /api/auth/login` |
| `logout` | `performLogout` (dipanggil menu user di `AppLayout`) | `POST /api/auth/logout` |
| `changepassword` | Form ganti password; dipakai halaman wajib ganti (`/change-password`) dan Profil (`/profile`) | `POST /api/auth/change-password` |
| `users` | Admin > Users: tabel + filter + paginasi, dialog buat, edit, nonaktif/aktif, reset password, tampilan password sekali | `/api/users*` (7 operasi) |

Feature FE tidak dipetakan 1:1 ke folder backend. Enam usecase backend admin (`createuser`, `listusers`, `getuser`, `updateuser`, `setuserstatus`, `resetpassword`) melayani satu layar dan satu set tipe, jadi digabung menjadi satu feature FE `users`. Import silang antar feature FE dilarang kecuali lewat `index.ts`, jadi menggabungkannya menghindari duplikasi schema.

Komposisi di `app/`:
- bootstrap sesi (`app/boot.ts`) dan penanganan 401 global (sesi direset lalu ke `/login?next=`);
- aturan guard baru: `guestOnly` (login), redirect paksa ke `/change-password` selama `must_change_password`, dan `passwordChangeRoute`;
- navigasi sesuai role (menu Users hanya untuk super_admin), menu user (Profil, Keluar);
- view tipis `UsersView`, `ProfileView`, `SessionErrorView`.

Prinsip keamanan FE:
- `temporary_password` hanya ada di state komponen dialog. Tidak masuk store, URL, `localStorage`, toast, atau log; dihapus saat dialog ditutup (rule 12.9).
- Pesan gagal login sama untuk email tidak ada dan password salah.
- View tidak pernah melakukan navigasi sendiri: perpindahan setelah login, ganti password, dan logout diturunkan dari state sesi oleh guard di `app/`. Domain tidak boleh meng-import router (aturan depcruise `domain-no-app`).

## Plan

Urutan wajib: FE-0, lalu feature (FE-1 sampai FE-5, bisa paralel setelah FE-0 dan setelah kontrak `auth.yml` ada dari backend F1), lalu A (komposisi `app/`), lalu E (e2e). Setiap langkah ditulis TDD: test dulu, lalu implementasi.

### FE-0. Prasyarat bersama di `shared/` (kecil, tanpa import `domain/`)

1. `shared/validation/error.ts` (ubah):
   - tambah `PASSWORD_CHANGE_REQUIRED` ke `KnownErrorCode` (kode bawaan infra sejak T01 D4, bukan milik domain);
   - tambah `RetryAfterDetails = z.object({ retry_after_seconds: z.number().int().nonnegative() })` dan `retryAfterOf(details: unknown): number | null` (dipakai untuk `RATE_LIMITED` T00 dan `ACCOUNT_LOCKED`);
   - tambah `ReasonDetails = z.object({ reason: z.string() })` dan `reasonOf(details: unknown): string | null`.
2. `shared/utils/datetime.ts` (baru bila T02 belum membuatnya): `formatDateTime(iso: string, locale: Locale): string` memakai `Intl.DateTimeFormat(locale === 'id' ? 'id-ID' : 'en-GB', { dateStyle: 'medium', timeStyle: 'short' })`. ISO tidak valid mengembalikan string asal.
3. `shared/components/Select.svelte` (baru): props `value` (`$bindable`), `label`, `name?`, `options: {value: string; label: string}[]`, `error?`, `disabled?`, `hint?`. Struktur aksesibilitas sama dengan `Input.svelte` (`<label for>`, `aria-describedby`, `aria-invalid`). Ekspor di `shared/components/index.ts`.
4. `shared/i18n/id.json` dan `en.json`: kunci di bagian "Kunci i18n". Test paritas kunci yang ada harus lulus.

### FE-1. Feature `getme` (`frontend/src/domain/core/auth/getme/`)

- `types.ts`:
  ```ts
  export const MeSchema = z.object({
    id: z.string().uuid(),
    email: z.string(),
    name: z.string(),
    role: z.enum(['super_admin', 'developer']),
    must_change_password: z.boolean(),
  });
  export type Me = z.infer<typeof MeSchema>;
  export function toIdentity(m: Me): Identity; // camelCase ke shared/stores/session Identity
  ```
- `services/getme.service.ts`: `fetchMe(signal?): Promise<Identity>` = `get('/api/auth/me', { schema: MeSchema, skipAuthRedirect: true, signal })` lalu `toIdentity`. `skipAuthRedirect` wajib karena 401 di bootstrap adalah hasil normal.
- `bootstrap.ts`: `loadSession(deps = { fetchMe, setIdentity }): Promise<void>`.
  - 200: `setIdentity(identity)`.
  - 401: `setIdentity(null)` (anonymous), tidak melempar.
  - Error lain (500, `NETWORK`, `INVALID_RESPONSE`): melempar dan store TIDAK diubah (tetap `unknown`), agar user tidak salah dianggap anonim.
- `index.ts`: ekspor `fetchMe`, `loadSession`, `MeSchema`, `toIdentity`, tipe `Me`.
- Tanpa `stores/` (state sesi sudah di `shared/stores/session.ts`), tanpa komponen.

### FE-2. Feature `login` (`frontend/src/domain/core/auth/login/`)

- `types.ts`:
  - `LoginInput {email: string; password: string}`;
  - `LoginResponseSchema = z.object({ user: MeSchema })` (impor `MeSchema` dari `../getme` index, bukan internal);
  - konstanta `AuthErrorCode = { INVALID_CREDENTIALS, ACCOUNT_LOCKED }`;
  - `LoginErrorView` (union): `{kind:'invalid'} | {kind:'locked'; retryAfterSeconds: number|null} | {kind:'rateLimited'; retryAfterSeconds: number|null} | {kind:'fields'; fields: Record<string,string>} | {kind:'generic'; message: string; traceId?: string}`;
  - fungsi murni `toLoginError(e: unknown): LoginErrorView`.
- `services/login.service.ts`: `login(input, signal?): Promise<Identity>` = `post('/api/auth/login', input, { schema: LoginResponseSchema, skipAuthRedirect: true })` lalu `toIdentity(res.user)`. Email dikirim apa adanya (server yang trim dan lowercase).
- `components/LoginForm.svelte` (presentasi, state lokal field):
  - props `submitting: boolean`, `error: LoginErrorView | null`, `cooldownSeconds: number`, `onsubmit(input)`;
  - field `Input` email (`type=email`, `autocomplete=username`, `required`) dan password (`type=password`, `autocomplete=current-password`, `required`);
  - validasi klien hanya "wajib diisi" (format email tidak dicek, sama dengan server);
  - region error `role="alert"` di atas tombol; tombol `loading={submitting}` dan `disabled` selama `cooldownSeconds > 0` (teks tombol menampilkan sisa detik).
- `views/LoginView.svelte` (satu-satunya yang memanggil service dan store):
  - submit: `submitting=true`, `login(...)`. Sukses: `setIdentity(identity)`. Navigasi tidak dilakukan view; guard `guestOnly` mengalihkan ke `next` yang aman, atau ke `/change-password` bila flag aktif.
  - `invalid`: tampilkan `auth.login.error.invalid`; kosongkan field password; fokus ke password.
  - `locked`: tampilkan `auth.login.error.locked` dengan `{minutes} = ceil(retry/60)` (fallback `auth.login.error.lockedNoTime` bila detail tidak ada). Tombol tidak dikunci, karena lockout berlaku per email dan user boleh mengetik email lain.
  - `rateLimited`: `auth.login.error.rateLimited`; mulai hitung mundur `cooldownSeconds` (default 60 bila detail tidak ada) dengan `setInterval` 1 detik yang dibersihkan di `$effect` cleanup.
  - `fields`: teruskan pesan per field ke `Input.error`.
  - `generic`: `ErrorState` (pesan dari `ApiError`, `trace_id`).
- `index.ts`: ekspor `LoginView`.

### FE-3. Feature `logout` (`frontend/src/domain/core/auth/logout/`)

- `services/logout.service.ts`: `logoutRequest(): Promise<void>` = `post('/api/auth/logout', undefined, { skipAuthRedirect: true })`.
- `logout.ts`: `performLogout(deps = { logoutRequest }): Promise<{ ok: true } | { ok: false; error: unknown }>`.
  - 204 atau 401 (sesi sudah mati): `{ok:true}`.
  - Error lain: `{ok:false, error}`; sesi lokal tidak diubah karena cookie HttpOnly tidak bisa dihapus dari JS.
  - Tidak menyentuh store sesi dan router; pemanggilnya `app/` (lihat A4).
- `index.ts`: ekspor `performLogout`.

### FE-4. Feature `changepassword` (`frontend/src/domain/core/auth/changepassword/`)

- `types.ts`:
  - `ChangePasswordInput {current_password; new_password}`;
  - `ChangePasswordErrorView`: `{kind:'fields'; fields} | {kind:'conflict'} | {kind:'rateLimited'; retryAfterSeconds} | {kind:'generic'; message; traceId?}`;
  - `toChangePasswordError(e)`.
- `services/changepassword.service.ts`: `changePassword(input): Promise<void>` = `post('/api/auth/change-password', input)` (204 tanpa body; 401 dibiarkan ke handler global).
- `components/ChangePasswordForm.svelte`:
  - field `current_password` (`autocomplete=current-password`), `new_password` (`autocomplete=new-password`, hint `auth.changePassword.hint`), `confirm_password` (khusus klien);
  - validasi klien: ketiganya wajib; `confirm === new` (`auth.changePassword.error.mismatch`); `new !== current` (`auth.changePassword.error.sameAsCurrent`). Panjang minimal TIDAK dicek di klien karena `PASSWORD_MIN_LENGTH` dapat diatur di server dan tidak diekspos; pesan panjang datang dari `details.fields.new_password`;
  - props `submitting`, `error`, `cooldownSeconds`, `onsubmit(input)`; `resetKey` (number) untuk mengosongkan semua field setelah sukses.
- `views/ChangePasswordView.svelte` dengan prop `mode: 'forced' | 'profile'`:
  - `forced`: judul `auth.changePassword.forcedTitle`, penjelasan `auth.changePassword.forcedDescription`, dan tautan "Keluar" lewat callback prop `onlogout` yang diisi `app/`.
  - Sukses (kedua mode): `setIdentity({ ...get(session).identity!, mustChangePassword: false })`, toast sukses `auth.changePassword.success` (menyebut sesi lain dikeluarkan), form dikosongkan. Pada `forced`, guard (`passwordChangeRoute`) otomatis mengalihkan ke `next` atau `/`.
  - `fields`: pesan per field dari server (`current_password: "password lama salah"`, `new_password: ...`).
  - `conflict` (409, password diubah di tab lain): `auth.changePassword.error.conflict` dan saran muat ulang.
  - `rateLimited`: pesan dan hitung mundur seperti login.
- `index.ts`: ekspor `ChangePasswordView`.

### FE-5. Feature `users` (`frontend/src/domain/core/auth/users/`)

**types.ts**
```ts
export const RoleSchema = z.enum(['super_admin', 'developer']);
export const UserSchema = z.object({
  id: z.string().uuid(), email: z.string(), name: z.string(), role: RoleSchema,
  is_active: z.boolean(), must_change_password: z.boolean(),
  created_at: z.string(), created_by: z.string().uuid().nullable(), created_by_label: z.string().nullable(),
  updated_at: z.string(), last_login_at: z.string().nullable(),
});
export const UserListSchema = z.object({ items: z.array(UserSchema), total: z.number().int().nonnegative(),
  limit: z.number().int().positive(), offset: z.number().int().nonnegative() });
export const UserWithTempPasswordSchema = z.object({ user: UserSchema, temporary_password: z.string().min(1).nullable() });
export type User = z.infer<typeof UserSchema>; // dst.
export type StatusFilter = '' | 'active' | 'inactive';
export interface UsersFilters { role: '' | Role; status: StatusFilter; q: string }
export const UserErrorReason = { SELF_ACTION_FORBIDDEN: 'SELF_ACTION_FORBIDDEN', LAST_SUPER_ADMIN: 'LAST_SUPER_ADMIN' } as const;
export const PAGE_SIZE = 20;
```
`UserSchema` hanya didefinisikan di sini (menggantikan duplikasi di plan getuser/listusers).

**services/** (tanpa state; semua lewat `shared/services/http.ts` + Zod)
- `users.service.ts`:
  - `listUsers({role, status, q, limit, offset}, signal?)`: `GET /api/users`. Nilai kosong dan `q` yang hanya spasi dibuang dari query. Schema `UserListSchema`.
  - `createUser({email, name, role, password?})`: `POST /api/users`. `password` dikirim hanya bila mode manual; mode otomatis tidak mengirim key `password`. Schema `UserWithTempPasswordSchema`.
  - `updateUser(id, patch: Partial<{name, email, role}>)`: `PATCH /api/users/{id}`; hanya field yang berubah. Bila patch kosong, fungsi melempar `Error` pemrograman (view tidak boleh memanggil). Schema `UserSchema`.
  - `setUserStatus(id, active: boolean)`: `POST /api/users/{id}/activate|deactivate` tanpa body. Schema `UserSchema`.
  - `resetUserPassword(id, password?: string)`: `POST /api/users/{id}/reset-password` dengan body `{}` atau `{password}`. Schema `UserWithTempPasswordSchema`.
  - Semua `id` di-`encodeURIComponent`.
- `getuser.service.ts`: `fetchUser(id, signal?)`: `GET /api/users/{id}`, schema `UserSchema` (sesuai `usecase-getuser.md` F1, tetapi memakai `UserSchema` dari `types.ts` dan opsi `schema` milik `http.ts`).

**stores/usersList.ts** (svelte/store, tanpa fetch)
- State: `{ filters: UsersFilters; page: number; data: UserList | null; loading: boolean; refreshing: boolean; error: ApiError | null }`. Awal: filter kosong, `page=1`, `data=null`, `loading=false`.
- Mutator murni: `setFilters(partial)` (selalu `page=1`), `setPage(n)`, `startLoading(silent)`, `succeed(data)`, `fail(err)`, `replaceUser(user)` (ganti baris dengan id sama), `reset()`.
- Derived: `offset = (page-1)*PAGE_SIZE`; `hasFilters` (ada filter terisi).

**views/usersList.controller.ts** (orkestrasi milik lapisan view; diuji tanpa DOM)
- `createUsersListController({ listUsers, store, debounceMs = 300, setTimer, clearTimer })` mengembalikan `{ load(), setFilters(partial), setPage(n), refresh({silent}), dispose() }`.
- Perubahan `q` di-debounce 300 ms; perubahan `role`, `status`, dan `page` langsung memuat.
- Setiap fetch membatalkan fetch sebelumnya (`AbortController`). Respons basi diabaikan lewat nomor urut. `AbortError` tidak dianggap error.
- Bila `total > 0`, `items` kosong, dan `page > 1` (data berkurang setelah nonaktif/filter): pindah sekali ke halaman terakhir valid `ceil(total/PAGE_SIZE)`, tanpa loop (flag satu kali per load).
- `refresh({silent:true})` dipanggil setelah mutasi: data lama tetap tampil (`refreshing`), tanpa spinner penuh.
- `dispose()` membatalkan request dan timer (dipanggil saat view unmount).

**components/** (presentasi; tidak memanggil service; data lewat props dan callback)
- `UsersFilterBar.svelte`: `Select` role (Semua/Super Admin/Developer), `Select` status (Semua/Aktif/Nonaktif), `Input` pencarian (`type=search`, maks 100 karakter lewat `maxlength`), tombol "Reset filter" bila ada filter. Callback `onchange(partial)`.
- `UsersTable.svelte`: memakai `shared/components/Table`. Kolom Nama, Email, Role, Status, Login terakhir, Aksi.
  - Status ditampilkan sebagai teks "Aktif/Nonaktif" plus badge; badge "Wajib ganti password" bila `must_change_password`.
  - `last_login_at` null menjadi "Belum pernah"; selain itu `formatDateTime`.
  - Baris akun sendiri diberi label "(Anda)".
  - Snippet aksi per baris diisi `UserRowActions`.
- `UserRowActions.svelte`: tombol Edit, Nonaktifkan/Aktifkan, Reset password. Prop `isSelf`. Untuk baris sendiri: tombol Nonaktifkan dan Reset password `disabled` dengan tooltip/teks bantuan (`users.self.cannotDeactivate`, `users.self.useProfile`). Edit tetap aktif, tetapi role dikunci di dialog. UX saja; backend tetap menolak (403).
- `CreateUserDialog.svelte` (memakai `Dialog`):
  - Tahap `form`: `Input` email, `Input` nama (maks 100), `Select` role (default `developer`), `Select` mode password (`generate` default, atau `manual`). Mode manual menampilkan `Input` password (`type=password`, `autocomplete=off`) dan tidak mengirim password kosong.
  - Validasi klien: email, nama, dan role wajib; password wajib bila manual.
  - Tahap `secret` (hanya bila `temporary_password` tidak null): `OneTimeSecret`. Tombol tutup bertuliskan "Saya sudah menyimpan password". Bila mode manual (null), dialog langsung tertutup dengan toast sukses.
  - Props `open` (bindable), `submitting`, `error`, `result: {user, temporaryPassword} | null`, `onsubmit(input)`, `onclose()`. State rahasia ada di komponen ini dan view pemanggil saja, dan dihapus (`null`) pada `onclose`.
- `EditUserDialog.svelte`:
  - Tahap memuat (detail via `fetchUser` di view; `Spinner`), error (404 menjadi "User tidak ditemukan" dan tombol tutup; lainnya `ErrorState` dengan coba lagi), lalu form nama/email/role yang diisi dari detail.
  - Tombol Simpan `disabled` bila tidak ada perubahan.
  - `isSelf`: `Select` role `disabled` dengan teks bantuan `users.self.cannotChangeRole`.
- `ConfirmStatusDialog.svelte`: konfirmasi nonaktif (menjelaskan bahwa semua sesi user langsung berakhir) atau aktifkan. Tombol `danger` untuk nonaktif. Menampilkan error form (403 self / 409 last admin) di region `role="alert"`.
- `ResetPasswordDialog.svelte`:
  - Tahap `confirm` menjelaskan bahwa sesi user dihapus dan user wajib ganti password. Ada `Select` mode (`generate`/`manual`) dan `Input` password untuk mode manual.
  - Tahap `secret`: `OneTimeSecret`.
  - Error field `password`, 403 self (`users.error.selfReset`), 404.
- `OneTimeSecret.svelte`:
  - `<input readonly>` berisi nilai (font mono, `aria-label`), tombol "Salin" (`navigator.clipboard.writeText`) yang menampilkan "Tersalin" 2 detik;
  - fallback bila clipboard tidak tersedia atau ditolak: pilih teks input dan tampilkan `users.secret.copyManual`;
  - peringatan `users.secret.onceWarning` di `role="note"`;
  - nilai tidak pernah dirender di atribut `title`/`data-*` dan tidak dikirim ke `console`.

**views/UsersListView.svelte** (komposisi tipis; satu-satunya pemanggil service/store di feature ini)
- Mount: `controller.load()`. Unmount: `controller.dispose()`.
- `isSelf(user) = user.id === $session.identity?.userId`.
- Buat: `createUser`. 201 membuka tahap rahasia bila ada, lalu `refresh({silent:true})` dan toast `users.toast.created`.
- Edit: buka dialog, panggil `fetchUser(id)` (abort saat dialog ditutup), lalu `updateUser(id, diff)`. Sukses: `replaceUser`, `refresh({silent:true})`, toast `users.toast.updated`.
- Status: `setUserStatus`. Sukses: `replaceUser`, `refresh({silent:true})`, toast `users.toast.deactivated` atau `activated`.
- Reset: `resetUserPassword` lalu tahap rahasia, `replaceUser`, toast `users.toast.passwordReset` (tanpa nilai password).
- Pemetaan error dialog (fungsi murni `toUserFormError(e)` di `types.ts`):
  - 400: `fieldErrorsOf(details)` ke field; kunci `_` menjadi pesan form;
  - 409 dengan `reasonOf = LAST_SUPER_ADMIN`: `users.error.lastSuperAdmin`;
  - 409 lainnya (email): field `email` = `users.error.emailTaken`;
  - 403 dengan `reason = SELF_ACTION_FORBIDDEN`: `users.error.selfAction`;
  - 403 lainnya: `error.forbidden`;
  - 404: `users.error.notFound` dan `refresh`;
  - lainnya: `ErrorState`.
- Header halaman: judul `users.title`, tombol "Buat user".

**index.ts**: ekspor `UsersListView`, `resetUsersState` (alias `store.reset`), tipe `User`, `Role`.

### A. Komposisi `app/` (setelah semua `index.ts` ada)

**A1. `app/routes/router.ts`** (ubah tipe saja): `RouteMeta` ditambah `guestOnly?: boolean` dan `passwordChangeRoute?: boolean`.

**A2. `app/routes/next.ts`** (baru): `safeNext(raw: string | null): string | null`. Hanya menerima path relatif sama-origin:
- diawali `/`;
- bukan `//` dan bukan `/\`;
- tanpa skema dan tanpa karakter kontrol;
- panjang maksimal 2048;
- bukan `/login` dan bukan `/change-password` (beserta query-nya).

Selain itu `null`. Mencegah open redirect.

**A3. `app/routes/guard.ts`** (ubah). Fungsi `resolve(m, s, fullPath)` dengan urutan:
1. `!m`: `notfound`.
2. `meta.guestOnly`:
   - `unknown`: `pending`;
   - `authenticated`: `redirect` ke `safeNext(query.next) ?? '/'` (bila flag aktif, langkah 5 di route tujuan mengalihkan lagi ke `/change-password`);
   - selain itu: `render`.
3. Tanpa `auth` dan tanpa `roles`: `render`.
4. `unknown`: `pending`. `anonymous`: `redirect /login?next=<fullPath>` (perilaku lama).
5. `identity.mustChangePassword && !meta.passwordChangeRoute`: `redirect /change-password?next=<encode(fullPath)>`.
6. `meta.passwordChangeRoute && !identity.mustChangePassword`: `redirect safeNext(query.next) ?? '/'`.
7. `roles` tidak cocok: `notfound`.
8. `render`.

`query.next` dibaca dari `fullPath` lewat `new URL(fullPath, 'http://x').searchParams`.

**A4. `app/boot.ts`** (baru):
- `bootState: Readable<{kind:'loading'} | {kind:'ready'} | {kind:'failed'; error: unknown}>`.
- `boot(deps = { loadSession })`: set `loading`, `await loadSession()`, lalu `ready`. Error: `failed` (session tetap `unknown`).
- `installAuthHandlers()`: `setUnauthorizedHandler((next) => { resetUsersState(); setIdentity(null); navigate('/login?next=' + encodeURIComponent(next), { replace: true }) })`. Menggantikan handler bawaan `startRouter`, sehingga store tidak lagi menyimpan identitas basi saat sesi mati (AC5 di UI).
- `logoutAndLeave(deps = { performLogout })`: `performLogout()`. Bila ok, dalam satu tick sinkron: `resetUsersState()`, `navigate('/login', { replace: true })`, `setIdentity(null)` (guard dievaluasi sekali setelah keduanya berubah, jadi tidak ada `next`). Bila gagal: `toast.fromError(error)`.

**A5. `main.ts`** (ubah): hapus `TODO(T01)`. Urutan: `startRouter(routes, notFoundRoute)`, `installAuthHandlers()`, `void boot()`, `mount(App)`. Handler `unhandledrejection` tetap.

**A6. `app/App.svelte`** (ubah): bila `res.kind === 'pending'` dan `$bootState.kind === 'failed'`, render `SessionErrorView` (BlankLayout) dengan tombol coba lagi yang memanggil `boot()`. Route publik (`/login`) tetap bisa dirender.

**A7. `app/routes/index.ts`** (ubah):
```ts
import { LoginView } from '../../domain/core/auth/login';
import ChangePasswordRouteView from '../views/ChangePasswordRouteView.svelte'; // pembungkus app
import ProfileView from '../views/ProfileView.svelte';
import UsersView from '../views/UsersView.svelte';
export const routes: RouteDef[] = [
  { path: '/', view: HomeView, meta: { auth: true, layout: 'app', titleKey: 'home.title' } },
  { path: '/login', view: LoginView, meta: { guestOnly: true, layout: 'blank', titleKey: 'auth.login.title' } },
  { path: '/change-password', view: ChangePasswordRouteView, meta: { auth: true, passwordChangeRoute: true, layout: 'blank', titleKey: 'auth.changePassword.forcedTitle' } },
  { path: '/profile', view: ProfileView, meta: { auth: true, layout: 'app', titleKey: 'auth.profile.title' } },
  { path: '/admin/users', view: UsersView, meta: { auth: true, roles: ['super_admin'], layout: 'app', titleKey: 'users.title' } },
];
```
Import `HomeView` dan `NotFoundView` yang sudah ada tetap.

**A8. View `app/views/`** (baru, tipis, menerima prop `params` lalu mengabaikannya):
- `ChangePasswordRouteView.svelte`: `<ChangePasswordView mode="forced" onlogout={logoutAndLeave} />`.
- `ProfileView.svelte`: kartu identitas dari `$session.identity` (nama, email, label role) dan `<ChangePasswordView mode="profile" />`.
- `UsersView.svelte`: `<UsersListView />`.
- `SessionErrorView.svelte`: judul `auth.session.loadFailed`, `ErrorState` (pesan dan trace id), tombol `common.retry`.
- `HomeView.svelte` (ubah kecil): sapaan `home.greeting` dengan nama user. `data-testid="home-title"` tetap ada.

**A9. `app/layouts/AppLayout.svelte`** (ubah):
- `<nav aria-label={$t('nav.main')}>` berisi Beranda (`/`) dan Users (`/admin/users`, hanya bila `$session.identity?.role === 'super_admin'`). `aria-current="page"` untuk route aktif. T02 kemudian menambah satu `<a>` Audit Log di nav yang sama.
- Menu user di kanan: nama user, tautan Profil (`/profile`), tombol Keluar (`logoutAndLeave`; `loading` selama request), dan `LocaleSwitch`.

### E. E2E dan verifikasi

- `npm run check`, `npm run test`, `npm run depcruise`, `npm run build`, lalu Playwright (bagian E2E).
- Depcruise harus lulus tanpa allowlist: `app/` hanya meng-import `domain/core/auth/<feature>/index.ts`; `login` meng-import `getme/index.ts` saja; `shared/` tidak meng-import `domain/`.

### Matriks state UI

| Layar | Loading | Empty | Error | Lainnya |
|---|---|---|---|---|
| Bootstrap (route auth) | `PendingView` selama `me` | - | 500/network: `SessionErrorView` + coba lagi (bukan ke login) | 401: ke `/login?next=` tanpa pesan |
| `/login` | Tombol `loading`, field `disabled` | - | invalid (generik), locked (menit), rateLimited (hitung mundur), fields, generik + trace id | Sudah login: redirect `next` atau `/` |
| `/change-password` | Tombol `loading` | - | fields, conflict, rateLimited, generik | Flag false: redirect `next` atau `/`; tautan Keluar |
| `/profile` | Tombol `loading` | - | sama dengan di atas | Sukses: toast, form dikosongkan |
| `/admin/users` tabel | Muat pertama: `Spinner` di `Table`; refresh: data lama + indikator kecil | Tanpa filter: `users.empty` + tombol Buat user; dengan filter: `users.emptyFiltered` + Reset filter | `ErrorState` + coba lagi (403/500/network); 400 filter: pesan field di filter bar | Developer: `NotFoundView` dan tanpa fetch |
| Dialog buat | Tombol `loading` | - | field (400), email bentrok (409), generik | Tahap rahasia sekali tampil |
| Dialog edit | `Spinner` saat `fetchUser` | - | 404 (tutup + refresh), 403 self, 409 last admin/email, field | Simpan disabled bila tidak berubah |
| Dialog status | Tombol `loading` | - | 403 self, 409 last admin, 404 | Baris sendiri: tombol disabled |
| Dialog reset | Tombol `loading` | - | field `password`, 403 self, 404 | Tahap rahasia sekali tampil |
| Logout | Tombol `loading` | - | Toast error, tetap di halaman | 401 dianggap sukses |
| Sesi mati di tengah jalan | - | - | Request berikut 401: identitas direset, ke `/login?next=` | - |

### Kunci i18n (id / en)

| Kunci | id | en |
|---|---|---|
| `nav.main` (bila belum ada) | Navigasi utama | Main navigation |
| `nav.home` | Beranda | Home |
| `nav.users` | Users | Users |
| `nav.profile` | Profil | Profile |
| `nav.logout` | Keluar | Sign out |
| `home.greeting` | Halo, {name} | Hello, {name} |
| `auth.login.title` | Masuk | Sign in |
| `auth.login.email` / `auth.login.password` | Email / Password | Email / Password |
| `auth.login.submit` | Masuk | Sign in |
| `auth.login.wait` | Tunggu {seconds} dtk | Wait {seconds}s |
| `auth.login.error.invalid` | Email atau password salah | Incorrect email or password |
| `auth.login.error.locked` | Terlalu banyak percobaan login. Coba lagi dalam {minutes} menit. | Too many sign-in attempts. Try again in {minutes} minutes. |
| `auth.login.error.lockedNoTime` | Terlalu banyak percobaan login. Coba lagi nanti. | Too many sign-in attempts. Try again later. |
| `auth.login.error.rateLimited` | Terlalu banyak permintaan dari perangkat ini. Coba lagi sebentar lagi. | Too many requests from this device. Try again shortly. |
| `form.required` | Wajib diisi | Required |
| `auth.changePassword.forcedTitle` | Ganti password | Change password |
| `auth.changePassword.forcedDescription` | Anda wajib mengganti password sementara sebelum melanjutkan. | You must replace your temporary password before continuing. |
| `auth.changePassword.current` / `.new` / `.confirm` | Password saat ini / Password baru / Ulangi password baru | Current password / New password / Repeat new password |
| `auth.changePassword.hint` | Gunakan password panjang yang tidak sama dengan email Anda. | Use a long password that differs from your email. |
| `auth.changePassword.submit` | Simpan password | Save password |
| `auth.changePassword.success` | Password diganti. Sesi di perangkat lain telah dikeluarkan. | Password changed. Other sessions have been signed out. |
| `auth.changePassword.error.mismatch` | Password baru tidak sama | Passwords do not match |
| `auth.changePassword.error.sameAsCurrent` | Password baru harus berbeda dari password saat ini | New password must differ from the current one |
| `auth.changePassword.error.conflict` | Password sudah diubah di tempat lain. Muat ulang halaman. | Password was changed elsewhere. Reload the page. |
| `auth.profile.title` | Profil | Profile |
| `auth.profile.role` | Role | Role |
| `auth.session.loadFailed` | Gagal memuat sesi | Could not load your session |
| `users.title` | Users | Users |
| `users.create` | Buat user | Create user |
| `users.filter.role` / `.status` / `.search` / `.reset` / `.all` | Role / Status / Cari nama atau email / Reset filter / Semua | Role / Status / Search name or email / Reset filters / All |
| `users.role.super_admin` / `.developer` | Super Admin / Developer | Super Admin / Developer |
| `users.status.active` / `.inactive` | Aktif / Nonaktif | Active / Inactive |
| `users.badge.mustChange` | Wajib ganti password | Must change password |
| `users.col.name` / `.email` / `.role` / `.status` / `.lastLogin` / `.actions` | Nama / Email / Role / Status / Login terakhir / Aksi | Name / Email / Role / Status / Last sign-in / Actions |
| `users.neverLoggedIn` | Belum pernah | Never |
| `users.you` | (Anda) | (you) |
| `users.empty` | Belum ada user | No users yet |
| `users.emptyFiltered` | Tidak ada user yang cocok | No matching users |
| `users.error.load` | Gagal memuat daftar user | Could not load users |
| `users.action.edit` / `.deactivate` / `.activate` / `.resetPassword` | Ubah / Nonaktifkan / Aktifkan / Reset password | Edit / Deactivate / Activate / Reset password |
| `users.form.email` / `.name` / `.role` / `.passwordMode` / `.password` | Email / Nama / Role / Password awal / Password | Email / Name / Role / Initial password / Password |
| `users.form.passwordMode.generate` / `.manual` | Bangkitkan otomatis / Tetapkan sendiri | Generate automatically / Set manually |
| `users.dialog.createTitle` / `.editTitle` / `.deactivateTitle` / `.activateTitle` / `.resetTitle` | Buat user / Ubah user / Nonaktifkan user / Aktifkan user / Reset password | Create user / Edit user / Deactivate user / Activate user / Reset password |
| `users.dialog.deactivateBody` | {name} tidak bisa login dan semua sesinya langsung berakhir. | {name} will be unable to sign in and all sessions end immediately. |
| `users.dialog.activateBody` | {name} dapat login kembali. | {name} will be able to sign in again. |
| `users.dialog.resetBody` | Semua sesi {name} dihapus dan ia wajib mengganti password saat login. | All of {name}'s sessions are removed and a password change is required at next sign-in. |
| `users.secret.title` | Password sementara | Temporary password |
| `users.secret.onceWarning` | Password ini hanya ditampilkan sekali. Salin dan berikan ke user lewat jalur aman. | This password is shown only once. Copy it and share it through a secure channel. |
| `users.secret.copy` / `.copied` / `.copyManual` | Salin / Tersalin / Salin manual: teks sudah dipilih | Copy / Copied / Copy manually: text is selected |
| `users.secret.done` | Saya sudah menyimpan password | I have saved the password |
| `users.self.cannotDeactivate` | Tidak bisa menonaktifkan akun sendiri | You cannot deactivate your own account |
| `users.self.cannotChangeRole` | Tidak bisa mengubah role akun sendiri | You cannot change your own role |
| `users.self.useProfile` | Gunakan halaman Profil untuk mengganti password sendiri | Use the Profile page to change your own password |
| `users.error.emailTaken` | Email sudah terdaftar | Email is already registered |
| `users.error.lastSuperAdmin` | Minimal harus ada satu super admin aktif | At least one active super admin is required |
| `users.error.selfAction` | Tidak boleh menonaktifkan atau menurunkan role akun sendiri | You cannot deactivate or demote your own account |
| `users.error.selfReset` | Gunakan ganti password untuk akun sendiri | Use change password for your own account |
| `users.error.notFound` | User tidak ditemukan | User not found |
| `users.error.noChanges` | Tidak ada perubahan | No changes |
| `users.toast.created` / `.updated` / `.deactivated` / `.activated` / `.passwordReset` | User dibuat / User diperbarui / User dinonaktifkan / User diaktifkan / Password direset | User created / User updated / User deactivated / User activated / Password reset |

### Keputusan dan selisih dengan plan usecase

| # | Keputusan | Alasan |
|---|---|---|
| K1 | Feature FE `users` mencakup enam usecase admin backend | Satu layar dan satu set schema; import silang internals dilarang, jadi pemisahan akan menduplikasi `UserSchema` |
| K2 | `postLoadRedirect` (`usecase-getme.md` F4) TIDAK dibuat; redirect paksa dipindah ke guard (A3 langkah 5–6) | Guard dievaluasi di setiap navigasi, sedangkan `postLoadRedirect` hanya sekali saat bootstrap. Aturan juga tetap di `app/`, jadi domain tidak perlu router |
| K3 | View tidak memanggil `navigate`; perpindahan diturunkan dari `session` oleh guard (`guestOnly`, `passwordChangeRoute`) | `domain-no-app` melarang domain meng-import router; ini membuat alur login dan ganti password teruji di `guard.test.ts` |
| K4 | `getme` memakai `skipAuthRedirect` (opsi sudah ada di `http.ts`) | Menjawab catatan F3 `usecase-getme.md`; tidak perlu mengubah `http.ts` |
| K5 | Handler 401 global (A4) mereset sesi dan state users sebelum ke `/login` | Handler bawaan T00 hanya navigasi, sehingga identitas basi tetap tersimpan di store |
| K6 | Panjang minimal password tidak dicek di klien | `PASSWORD_MIN_LENGTH` dapat diatur dan tidak diekspos API; server yang memutuskan |
| K7 | Pesan field dari server (`details.fields`) ditampilkan apa adanya (berbahasa Indonesia). Pesan untuk kode error yang dikenal (`INVALID_CREDENTIALS`, `ACCOUNT_LOCKED`, `RATE_LIMITED`, `CONFLICT`, reason admin) memakai kunci i18n | Backend tidak mengirim kode per field. Batasan ini diterima: di locale `en`, pesan validasi field tetap berbahasa Indonesia |
| K8 | Filter daftar user tidak disinkronkan ke URL | Tidak diminta tiket (rule 1.5); dapat ditambah kemudian |
| K9 | `/` menjadi wajib login, sehingga smoke e2e T00 disesuaikan (lihat Changes) | Desain: setelah login user masuk dashboard; aplikasi tanpa halaman publik selain login |
| K10 | Rate limit IP login tidak diuji end-to-end terhadap server nyata; UI 429 diuji dengan `page.route` | Batas 20/menit per IP dibagi oleh seluruh suite paralel dari satu IP, sehingga test nyata akan flaky. Server e2e memakai `LOGIN_RATE_PER_MINUTE=1000`; perilaku server diuji di `TestLogin_RateLimitPerIP` (backend) |

## Migration

Tidak ada. Frontend tidak memiliki migrasi. Tabel auth dibuat backend di `backend/migrations/core/20261021090000000_create_auth_tables.sql` (overview bagian 6).

## Changes

`frontend/src/shared/`:
- `validation/error.ts`, `validation/error.test.ts` (ubah): `PASSWORD_CHANGE_REQUIRED`, `retryAfterOf`, `reasonOf`.
- `utils/datetime.ts`, `utils/datetime.test.ts` (baru bila belum ada dari T02).
- `components/Select.svelte` (baru), `components/index.ts` (ubah: ekspor `Select`), `components/components.test.ts` (ubah: kasus `Select`).
- `i18n/id.json`, `i18n/en.json` (ubah): kunci tabel i18n.

`frontend/src/domain/core/auth/getme/` (baru):
- `types.ts`, `services/getme.service.ts`, `services/getme.service.test.ts`, `bootstrap.ts`, `bootstrap.test.ts`, `index.ts`.

`frontend/src/domain/core/auth/login/` (baru):
- `types.ts`, `types.test.ts` (`toLoginError`), `services/login.service.ts`, `services/login.service.test.ts`, `components/LoginForm.svelte`, `components/LoginForm.test.ts`, `views/LoginView.svelte`, `views/LoginView.test.ts`, `index.ts`.

`frontend/src/domain/core/auth/logout/` (baru):
- `services/logout.service.ts`, `services/logout.service.test.ts`, `logout.ts`, `logout.test.ts`, `index.ts`.

`frontend/src/domain/core/auth/changepassword/` (baru):
- `types.ts`, `types.test.ts`, `services/changepassword.service.ts`, `services/changepassword.service.test.ts`, `components/ChangePasswordForm.svelte`, `components/ChangePasswordForm.test.ts`, `views/ChangePasswordView.svelte`, `views/ChangePasswordView.test.ts`, `index.ts`.

`frontend/src/domain/core/auth/users/` (baru):
- `types.ts`, `types.test.ts` (schema, `toUserFormError`);
- `services/users.service.ts`, `services/users.service.test.ts`, `services/getuser.service.ts`, `services/getuser.service.test.ts`;
- `stores/usersList.ts`, `stores/usersList.test.ts`;
- `views/usersList.controller.ts`, `views/usersList.controller.test.ts`, `views/UsersListView.svelte`, `views/UsersListView.test.ts`;
- `components/{UsersFilterBar,UsersTable,UserRowActions,CreateUserDialog,EditUserDialog,ConfirmStatusDialog,ResetPasswordDialog,OneTimeSecret}.svelte` beserta `*.test.ts`;
- `index.ts`.

`frontend/src/app/`:
- `routes/router.ts` (ubah tipe `RouteMeta`).
- `routes/next.ts`, `routes/next.test.ts` (baru).
- `routes/guard.ts`, `routes/guard.test.ts` (ubah: aturan A3; kasus lama tetap).
- `routes/index.ts` (ubah: lima route, hapus komentar contoh).
- `boot.ts`, `boot.test.ts` (baru).
- `App.svelte` (ubah: `SessionErrorView` saat boot gagal).
- `layouts/AppLayout.svelte` (ubah: nav dan menu user).
- `views/ChangePasswordRouteView.svelte`, `views/ProfileView.svelte`, `views/UsersView.svelte`, `views/SessionErrorView.svelte` (baru); `views/HomeView.svelte` (ubah: sapaan).
- `views/app.test.ts` (ubah: alur bootstrap, nav per role, logout).
- `frontend/src/main.ts` (ubah: `installAuthHandlers`, `boot`; hapus TODO).

`frontend/e2e/` dan konfigurasi:
- `playwright.config.ts` (ubah): `globalSetup: './e2e/global-setup.ts'`; env server `LOGIN_RATE_PER_MINUTE=1000`.
- `e2e/global-setup.ts` (baru): jalankan `../bin/grimoire migrate up`, lalu `create-admin --email e2e-admin@tim.id --name "E2E Admin"` dengan `CREATE_ADMIN_PASSWORD` dari env (`E2E_ADMIN_PASSWORD`, default khusus dev). Exit 1 dengan `email sudah terdaftar` dianggap sukses (idempoten).
- `e2e/helpers/auth.ts` (baru): `loginUi(page, email, pw)`, `apiLogin(request, email, pw)`, `createUserApi(request, {role, password?})` (email unik per test: `u-${Date.now()}-${rand}@tim.id`), `adminContext(browser)`.
- `e2e/auth.spec.ts`, `e2e/users.spec.ts` (baru).
- `e2e/smoke.spec.ts` (ubah, K9):
  - "home renders" menjadi "login renders" (`/login`, `login-title`, tanpa console error, tanpa respons >= 400 selain `401 /api/auth/me`);
  - "deep link" memeriksa `notfound-title`; tautan beranda kemudian berakhir di `/login?next=%2F`;
  - "back button", "locale switch", dan "blocked localStorage" memakai `/login`;
  - "session cookie before T01" diganti "cookie sesi palsu": `/` berakhir di `/login`, `Set-Cookie Max-Age=0` diterima, dan tanpa console error.

Tidak berubah: `shared/services/http.ts`, `shared/stores/session.ts`, `shared/stores/toast.ts`, `.dependency-cruiser.cjs`, `api/**` (milik backend F1).

## Test Scenario

Vitest + `@testing-library/svelte` + `user-event`. Service diuji dengan `fetch` palsu (`vi.stubGlobal('fetch', ...)`), pola `shared/services/http.test.ts`. Komponen diuji tanpa service (props/callback). View diuji dengan service di-mock lewat `vi.mock` modul service feature itu sendiri.

### shared
- `error.test.ts`: `retryAfterOf({retry_after_seconds: 812}) === 812`; `null`, `{}`, `{retry_after_seconds: -1}`, dan string menjadi `null`. `reasonOf({reason:'LAST_SUPER_ADMIN'})`; bentuk salah menjadi `null`.
- `datetime.test.ts`: format `id` dan `en` berbeda; ISO tidak valid dikembalikan apa adanya.
- `components.test.ts` (`Select`): label terhubung, `aria-invalid` saat error, `bind:value` berubah lewat `selectOptions`.

### getme
- `getme.service.test.ts`: 200 dipetakan ke `Identity` camelCase; role di luar enum dan `id` bukan uuid menjadi `INVALID_RESPONSE`; 401 menjadi `ApiError` 401 dan handler unauthorized TIDAK dipanggil (`skipAuthRedirect`).
- `bootstrap.test.ts`: sukses `setIdentity(identity)`; 401 `setIdentity(null)` tanpa melempar; 500, `NETWORK`, dan `INVALID_RESPONSE` melempar dan `setIdentity` tidak dipanggil.

### login
- `login.service.test.ts`: request `POST /api/auth/login` membawa `X-Requested-With: grimoire` dan body JSON persis input; 200 menghasilkan `Identity`; 401 `INVALID_CREDENTIALS` tidak memanggil handler unauthorized.
- `types.test.ts` (`toLoginError`, tabel): 401 `INVALID_CREDENTIALS` menjadi `invalid`; 429 `ACCOUNT_LOCKED` dengan 812 menjadi `locked` 812; tanpa detail menjadi `locked` null; 429 `RATE_LIMITED` menjadi `rateLimited`; 400 dengan fields menjadi `fields`; 403 `CSRF_REQUIRED`, 500, dan `NETWORK` menjadi `generic` dengan trace id.
- `LoginForm.test.ts`: submit kosong menampilkan `form.required` per field tanpa `onsubmit`; input valid memanggil `onsubmit` sekali; `cooldownSeconds=5` membuat tombol disabled dengan teks `Tunggu 5 dtk`; error tampil di `role=alert`; atribut `autocomplete` benar.
- `LoginView.test.ts`:
  - sukses memanggil `setIdentity` dengan identity dari respons;
  - `invalid` menampilkan pesan generik, password dikosongkan, email tetap;
  - `locked` 812 detik menampilkan "14 menit";
  - `rateLimited` 3 detik: tombol disabled lalu aktif kembali setelah `vi.advanceTimersByTime(3000)`;
  - double submit saat `submitting` tidak mengirim dua request;
  - teks pesan untuk "email tidak ada" dan "password salah" identik (kedua respons 401 sama).

### logout
- `logout.service.test.ts`: `POST /api/auth/logout` dengan header CSRF, tanpa body.
- `logout.test.ts`: 204 dan 401 menghasilkan `{ok:true}`; 500 dan `NETWORK` menghasilkan `{ok:false}`.

### changepassword
- `changepassword.service.test.ts`: body `{current_password, new_password}` (tanpa `confirm`); 204 resolve `undefined`; 401 memanggil handler unauthorized (tidak di-skip).
- `types.test.ts`: 400 fields, 409 menjadi `conflict`, 429 menjadi `rateLimited`, lainnya `generic`.
- `ChangePasswordForm.test.ts`: konfirmasi beda menampilkan `mismatch`; baru sama dengan lama menampilkan `sameAsCurrent`; wajib diisi; `resetKey` berubah mengosongkan field; field error server tampil di field yang tepat.
- `ChangePasswordView.test.ts`:
  - sukses: `setIdentity` dengan `mustChangePassword:false`, toast sukses, form kosong;
  - 400 `current_password: "password lama salah"` tampil di field password saat ini;
  - 409 menampilkan pesan conflict;
  - `mode="forced"` menampilkan judul wajib dan tombol Keluar yang memanggil `onlogout`; `mode="profile"` tanpa tombol Keluar.

### users
- `users.service.test.ts`:
  - `listUsers` membuang kosong/spasi dari query (`role=''`, `q='  '` tidak dikirim) dan mengirim `limit`/`offset`;
  - `createUser` mode otomatis tidak memuat key `password`; mode manual memuatnya;
  - `updateUser` dengan patch kosong melempar tanpa fetch, dan dengan `{name}` hanya mengirim `name`;
  - `setUserStatus(id,false)` ke `/deactivate`, `(id,true)` ke `/activate`;
  - `resetUserPassword(id)` mengirim `{}`;
  - id berisi `/` di-encode;
  - respons dengan `password_hash` ekstra tetap lolos (Zod strip) dan `password_hash` tidak ada di hasil;
  - `temporary_password: ""` ditolak Zod.
- `getuser.service.test.ts`: valid lolos; role/uuid salah menjadi `INVALID_RESPONSE`; 404 menjadi `ApiError` 404.
- `usersList.test.ts`: `setFilters` mereset `page=1`; `replaceUser` hanya mengganti baris dengan id sama; `reset` kembali ke awal; `offset` derived benar.
- `usersList.controller.test.ts` (fake timers, `listUsers` palsu):
  - ketik `q` tiga kali dalam 300 ms hanya menghasilkan satu panggilan;
  - ganti role langsung memanggil dengan `offset=0`;
  - respons A lambat dan B cepat: state berisi B, A di-abort atau diabaikan;
  - `AbortError` tidak masuk `error`;
  - `page=3`, total 41, items kosong: satu kali pindah ke page 3 lalu berhenti (tanpa loop bila tetap kosong);
  - `refresh({silent:true})` menjaga `data` lama selama memuat;
  - `dispose` membatalkan timer dan request.
- `types.test.ts` (`toUserFormError`): 409 `LAST_SUPER_ADMIN` menjadi pesan form; 409 tanpa reason menjadi field email; 403 `SELF_ACTION_FORBIDDEN`; 400 `_` menjadi pesan form; 404.
- `UsersTable.test.ts`: kolom dan header benar; `last_login_at` null menjadi "Belum pernah"; status berupa teks; badge wajib ganti; baris sendiri berlabel "(Anda)".
- `UserRowActions.test.ts`: `isSelf` membuat Nonaktifkan dan Reset password disabled dengan teks bantuan; user nonaktif menampilkan Aktifkan, bukan Nonaktifkan.
- `CreateUserDialog.test.ts`:
  - mode manual tanpa password tidak submit;
  - `result.temporaryPassword` menampilkan `OneTimeSecret`;
  - tombol "Saya sudah menyimpan" memanggil `onclose`; setelah dibuka ulang, nilai rahasia tidak ada di DOM;
  - error 409 tampil di field email.
- `EditUserDialog.test.ts`: Simpan disabled tanpa perubahan; `isSelf` membuat role disabled; diff hanya field berubah; tahap 404.
- `ConfirmStatusDialog.test.ts` dan `ResetPasswordDialog.test.ts`: teks berisi nama user; error alert; tahap rahasia pada reset.
- `OneTimeSecret.test.ts`: klik Salin memanggil `navigator.clipboard.writeText(value)` dan menampilkan "Tersalin"; clipboard menolak membuat teks terpilih dan pesan manual tampil; `console.log/info/error` tidak pernah dipanggil dengan nilai (spy).
- `UsersListView.test.ts`:
  - muat pertama spinner, lalu baris;
  - kosong tanpa filter dan kosong dengan filter (tombol reset);
  - 403/500 menampilkan `ErrorState` dan "Coba lagi" memuat ulang;
  - alur buat lengkap: `createUser` dipanggil, password sementara tampil sekali, refresh terpanggil, toast tanpa password;
  - setelah dialog ditutup, `temporary_password` tidak ada di `document.body.innerHTML`, `localStorage`, `location.href`, atau state store (`get(store)` di-serialize);
  - nonaktif sukses mengganti baris;
  - error 409 last admin tampil di dialog.

### app
- `next.test.ts` (tabel): `/admin/users?x=1` diterima; `//evil.com`, `/\evil`, `https://evil.com`, `javascript:alert(1)`, `/login`, `/login?next=/`, `/change-password`, string > 2048, `null`, dan `''` menjadi `null`.
- `guard.test.ts` (kasus lama tetap, ditambah):
  - `guestOnly`: unknown menjadi pending; anonymous menjadi render; authenticated dengan `?next=%2Fadmin%2Fusers` menjadi redirect `/admin/users`; `?next=//evil.com` menjadi `/`;
  - mustChange pada route auth `/admin/users?x=1` menjadi redirect `/change-password?next=%2Fadmin%2Fusers%3Fx%3D1`;
  - mustChange pada `passwordChangeRoute` menjadi render;
  - flag false pada `passwordChangeRoute` dengan `next=/profile` menjadi redirect `/profile`;
  - developer + mustChange pada `/admin/users` menjadi redirect ke change-password lebih dulu (bukan notfound).
- `boot.test.ts`:
  - `boot` sukses menghasilkan `ready`; gagal menghasilkan `failed` dan session tetap `unknown`; retry setelah gagal menghasilkan `ready`;
  - handler unauthorized: session menjadi anonymous, `resetUsersState` terpanggil, navigasi ke `/login?next=...` replace;
  - `logoutAndLeave` sukses: session anonymous dan path `/login` tanpa `next`; gagal: toast error dan session tidak berubah.
- `app.test.ts`:
  - route auth saat boot memuat menampilkan `PendingView`;
  - boot gagal menampilkan `SessionErrorView` dan retry;
  - super_admin melihat menu Users; developer tidak melihatnya;
  - developer membuka `/admin/users` mendapat `notfound-title`, tanpa fetch `/api/users`;
  - `document.title` = `Users · Grimoire`;
  - tombol Keluar memanggil `POST /api/auth/logout` lalu ke `/login`.
- Arsitektur: `npm run depcruise` lulus; `scripts/test-arch.mjs` lulus tanpa perubahan fixture.

## E2E Scenarios

Playwright terhadap binary (`../bin/grimoire`, `COOKIE_SECURE=false`, `LOGIN_RATE_PER_MINUTE=1000`). `global-setup` menjalankan migrasi dan membuat `e2e-admin@tim.id`. Setiap test membuat user dengan email unik lewat API admin, sehingga test dapat berjalan paralel.

Positif:
1. **Login admin**: buka `/` dan diarahkan ke `/login?next=%2F`. Login `e2e-admin@tim.id` mendarat di `/` (`home-title`, sapaan nama). Menu Users terlihat. Cookie `grimoire_session` ber-`HttpOnly`, dan `document.cookie` tidak memuatnya.
2. **Deep link setelah login**: anonim membuka `/admin/users?x=1`, login, lalu mendarat di `/admin/users`.
3. **Reload memulihkan sesi**: setelah login, reload `/admin/users` tetap di halaman tanpa melewati `/login`.
4. **Buat developer tanpa password**: dialog menampilkan password sementara sekali. Tombol Salin mengisi clipboard (izin `clipboard-read`/`clipboard-write` diberikan di context), dan nilainya sama dengan teks input. Setelah dialog ditutup, nilai tidak ada di DOM; baris baru tampil dengan badge "Wajib ganti password". `localStorage`, `sessionStorage`, dan URL tidak memuat nilai.
5. **Login pertama developer**: login dengan password sementara diarahkan ke `/change-password`. Navigasi manual ke `/` tetap kembali ke `/change-password`. Setelah ganti password, mendarat di `/`; menu Users tidak tampil; reload tidak lagi mengalihkan.
6. **Profil**: developer membuka `/profile`, mengganti password, dan melihat toast sukses. Sesi kedua (context lain, login sebelumnya) mendapat 401 di request berikut dan berakhir di `/login`.
7. **Cari, filter, paginasi**: buat 25 user dengan prefiks unik. Pencarian prefiks menampilkan 20 dan Pagination "Halaman 1 dari 2". Halaman 2 menampilkan 5. Filter status Nonaktif tanpa hasil menampilkan `users.emptyFiltered`, dan Reset filter mengembalikan daftar.
8. **Edit dan promosi role**: admin mengubah nama dan role developer menjadi Super Admin; baris berubah. Developer (sesi aktif) reload: menu Users muncul tanpa login ulang.
9. **Reset password**: admin mereset password developer; dialog menampilkan password baru sekali. Tab developer pada request berikutnya berakhir di `/login`. Login dengan password baru diarahkan ke `/change-password`.
10. **Nonaktif lalu aktif**: admin menonaktifkan developer (konfirmasi), dan status menjadi Nonaktif. Developer gagal login dengan pesan generik. Setelah diaktifkan, developer bisa login.
11. **Logout**: klik Keluar mendarat di `/login`; `GET /api/auth/me` dari halaman menjadi 401; tombol Back ke `/` kembali ke `/login`.
12. **Locale**: ganti ke English di halaman login; label dan pesan error `INVALID_CREDENTIALS` dalam bahasa Inggris.

Negatif:
1. **Pesan login seragam**: email tidak terdaftar dan password salah menghasilkan teks `role=alert` yang identik ("Email atau password salah"), dan tidak ada petunjuk keberadaan akun.
2. **Lockout**: 5 kali password salah untuk satu email unik, lalu password benar menampilkan "Terlalu banyak percobaan login. Coba lagi dalam 15 menit." dan tetap di `/login`. Email lain tetap bisa login dari halaman yang sama.
3. **Rate limit IP (UI)**: `page.route('/api/auth/login')` membalas 429 `RATE_LIMITED` `retry_after_seconds: 3`. Pesan tampil, tombol disabled dengan hitung mundur, lalu aktif lagi.
4. **Developer membuka `/admin/users`**: `notfound-title`, tanpa request ke `/api/users`. `fetch('/api/users')` manual dari halaman menghasilkan 403 `FORBIDDEN`.
5. **Gate password**: user ber-flag (sebelum ganti) memanggil `fetch('/api/users')` manual: 403 `PASSWORD_CHANGE_REQUIRED`; `fetch('/api/auth/me')` 200.
6. **Aksi pada akun sendiri**: di baris admin sendiri, tombol Nonaktifkan dan Reset password disabled, dan role di dialog edit terkunci. `fetch` manual `POST /api/users/<self>/deactivate` dengan `X-Requested-With` menghasilkan 403 `SELF_ACTION_FORBIDDEN`.
7. **Email duplikat**: membuat user dengan email yang sudah ada (beda huruf besar/kecil) menampilkan "Email sudah terdaftar" di field email, dan dialog tetap terbuka.
8. **Validasi password**: ganti password dengan 9 karakter, atau sama dengan email, menampilkan pesan server di field password baru. Konfirmasi tidak sama ditolak di klien tanpa request (diverifikasi lewat `page.on('request')`).
9. **Password lama salah**: pesan "password lama salah" di field password saat ini.
10. **Dinonaktifkan saat tab terbuka**: developer berada di `/profile`; admin menonaktifkan lewat API. Aksi berikutnya developer (submit form) mendapat 401, UI ke `/login?next=%2Fprofile`, dan nama user tidak lagi tampil.
11. **Bootstrap gagal**: `page.route('/api/auth/me')` membalas 500. Route `/` menampilkan "Gagal memuat sesi" dan Coba lagi (bukan halaman login). Setelah route dilepas, Coba lagi memulihkan.
12. **Open redirect**: `/login?next=//evil.com` lalu login mendarat di `/` (origin tetap).
13. **Respons rusak**: `page.route('/api/users*')` membalas 200 `{"items": "x"}`. Tabel menampilkan `ErrorState` "Respons server tidak sesuai format", tanpa crash.
14. **Cookie palsu**: cookie `grimoire_session=abc`, buka `/`: berakhir di `/login`, tanpa console error.

## Request/Response body

Format error seragam T00 untuk semua respons gagal:
```json
{ "error": { "code": "STRING", "message": "string", "details": null }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```
Semua mutasi mengirim header `X-Requested-With: grimoire` (ditambahkan otomatis oleh `http.ts`). Tanpa header itu server menjawab 403 `CSRF_REQUIRED`. Cookie sesi `grimoire_session` dikirim otomatis (`credentials: 'same-origin'`).

### Pemetaan aksi UI ke request

| Aksi UI | Request |
|---|---|
| Aplikasi dibuka | `GET /api/auth/me` |
| Submit form login | `POST /api/auth/login` `{email, password}` |
| Klik Keluar | `POST /api/auth/logout` (tanpa body) |
| Submit ganti password (paksa atau profil) | `POST /api/auth/change-password` `{current_password, new_password}` |
| Buka `/admin/users` | `GET /api/users?limit=20&offset=0` |
| Ketik "sar", pilih role Developer, status Aktif, halaman 2 | `GET /api/users?role=developer&status=active&q=sar&limit=20&offset=20` |
| Submit dialog Buat (otomatis) | `POST /api/users` `{email, name, role}` |
| Submit dialog Buat (manual) | `POST /api/users` `{email, name, role, password}` |
| Buka dialog Ubah | `GET /api/users/{id}` |
| Simpan dialog Ubah (nama dan role berubah) | `PATCH /api/users/{id}` `{name, role}` |
| Konfirmasi Nonaktifkan / Aktifkan | `POST /api/users/{id}/deactivate` / `/activate` |
| Konfirmasi Reset (otomatis / manual) | `POST /api/users/{id}/reset-password` `{}` / `{password}` |

### POST /api/auth/login
Request:
```json
{ "email": "Budi@Tim.ID", "password": "rahasia-panjang-1" }
```
200 (Set-Cookie `grimoire_session=<token>; Path=/; Max-Age=604800; HttpOnly; SameSite=Lax`):
```json
{ "user": { "id": "0b7c6a4e-2f1d-4c8a-9e3b-5d6f7a8b9c0d", "email": "budi@tim.id", "name": "Budi", "role": "developer", "must_change_password": true } }
```
401 (UI: `auth.login.error.invalid`):
```json
{ "error": { "code": "INVALID_CREDENTIALS", "message": "email atau password salah", "details": null }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```
429 akun ditunda (header `Retry-After: 812`; UI: `auth.login.error.locked` dengan `minutes=14`):
```json
{ "error": { "code": "ACCOUNT_LOCKED", "message": "terlalu banyak percobaan login, coba lagi nanti", "details": { "retry_after_seconds": 812 } }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a34" }
```
429 rate limit IP (UI: `auth.login.error.rateLimited` dan hitung mundur 3 detik):
```json
{ "error": { "code": "RATE_LIMITED", "message": "terlalu banyak permintaan, coba lagi nanti", "details": { "retry_after_seconds": 3 } }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a35" }
```
400 (UI: pesan di field email):
```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "permintaan tidak valid", "details": { "fields": { "email": "wajib diisi" } } }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a36" }
```

### POST /api/auth/logout
Request: tanpa body. 204 tanpa body, `Set-Cookie: grimoire_session=; Path=/; Max-Age=0; HttpOnly; SameSite=Lax`. 401 (UI menganggapnya sukses):
```json
{ "error": { "code": "UNAUTHORIZED", "message": "perlu login", "details": null }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a37" }
```

### GET /api/auth/me
200:
```json
{ "id": "0b7c6a4e-2f1d-4c8a-9e3b-5d6f7a8b9c0d", "email": "budi@tim.id", "name": "Budi", "role": "developer", "must_change_password": false }
```
401 (bootstrap: anonymous, tanpa pesan):
```json
{ "error": { "code": "UNAUTHORIZED", "message": "perlu login", "details": null }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a38" }
```
500 (bootstrap: `SessionErrorView`):
```json
{ "error": { "code": "INTERNAL", "message": "terjadi kesalahan internal", "details": null }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a39" }
```

### POST /api/auth/change-password
Request (field `confirm_password` hanya di klien, tidak dikirim):
```json
{ "current_password": "sementara-AbC123", "new_password": "baru-sangat-rahasia" }
```
204 tanpa body (sesi saat ini tetap; sesi lain dicabut).
400 (UI: per field):
```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "permintaan tidak valid", "details": { "fields": { "current_password": "password lama salah", "new_password": "minimal 10 karakter" } } }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a3a" }
```
409 (UI: `auth.changePassword.error.conflict`):
```json
{ "error": { "code": "CONFLICT", "message": "password berubah oleh proses lain, muat ulang dan coba lagi", "details": null }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a3b" }
```
429: bentuk `RATE_LIMITED` di atas.

### 403 password gate (endpoint non-exempt; UI biasanya tidak menerimanya karena guard mengalihkan lebih dulu)
```json
{ "error": { "code": "PASSWORD_CHANGE_REQUIRED", "message": "password wajib diganti sebelum melanjutkan", "details": null }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a3c" }
```

### GET /api/users?role=developer&status=active&q=sar&limit=20&offset=0
200:
```json
{
  "items": [
    { "id": "7d1e3b52-90aa-4f1c-8b2d-1c3e5a7b9d02", "email": "sari@tim.id", "name": "Sari", "role": "developer", "is_active": true,
      "must_change_password": false, "created_at": "2026-10-21T09:00:00Z", "created_by": "0b7c6a4e-2f1d-4c8a-9e3b-5d6f7a8b9c0d",
      "created_by_label": "admin@tim.id", "updated_at": "2026-10-21T09:05:00Z", "last_login_at": "2026-10-21T09:10:00Z" }
  ],
  "total": 1,
  "limit": 20,
  "offset": 0
}
```
200 kosong (UI: `users.emptyFiltered` karena ada filter):
```json
{ "items": [], "total": 0, "limit": 20, "offset": 0 }
```
400 (UI: pesan field di filter bar):
```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "permintaan tidak valid", "details": { "fields": { "status": "harus active atau inactive", "limit": "maksimal 100" } } }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a3d" }
```
403 (developer memanggil manual; UI: `ErrorState`):
```json
{ "error": { "code": "FORBIDDEN", "message": "akses ditolak", "details": null }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a3e" }
```

### POST /api/users
Request (mode otomatis, tanpa key `password`):
```json
{ "email": "sari@tim.id", "name": "Sari", "role": "developer" }
```
Request (mode manual):
```json
{ "email": "sari@tim.id", "name": "Sari", "role": "developer", "password": "awal-yang-panjang-1" }
```
201 (header `Cache-Control: no-store`; UI: tahap `OneTimeSecret`):
```json
{
  "user": {
    "id": "7d1e3b52-90aa-4f1c-8b2d-1c3e5a7b9d02", "email": "sari@tim.id", "name": "Sari", "role": "developer",
    "is_active": true, "must_change_password": true,
    "created_at": "2026-10-21T09:00:00Z", "created_by": "0b7c6a4e-2f1d-4c8a-9e3b-5d6f7a8b9c0d", "created_by_label": "admin@tim.id",
    "updated_at": "2026-10-21T09:00:00Z", "last_login_at": null
  },
  "temporary_password": "Kq7mZp3RtX9aHw2N"
}
```
201 mode manual: `temporary_password` bernilai `null`; dialog langsung tertutup dengan toast.
409 (UI: field email `users.error.emailTaken`):
```json
{ "error": { "code": "CONFLICT", "message": "email sudah terdaftar", "details": null }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a3f" }
```
400:
```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "permintaan tidak valid", "details": { "fields": { "role": "role harus super_admin atau developer", "email": "format email tidak valid" } } }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a40" }
```

### GET /api/users/{id}
200 (`UserDTO` tanpa pembungkus):
```json
{ "id": "7d1e3b52-90aa-4f1c-8b2d-1c3e5a7b9d02", "email": "sari@tim.id", "name": "Sari", "role": "developer", "is_active": true, "must_change_password": false, "created_at": "2026-10-21T09:00:00Z", "created_by": "0b7c6a4e-2f1d-4c8a-9e3b-5d6f7a8b9c0d", "created_by_label": "admin@tim.id", "updated_at": "2026-10-21T09:05:00Z", "last_login_at": null }
```
404 (UI: `users.error.notFound`, dialog ditutup, daftar di-refresh):
```json
{ "error": { "code": "NOT_FOUND", "message": "user tidak ditemukan", "details": null }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a41" }
```

### PATCH /api/users/{id}
Request (hanya field berubah):
```json
{ "name": "Sari W.", "role": "super_admin" }
```
200: `UserDTO` terbaru (bentuk sama dengan GET).
403 menurunkan role sendiri (UI: `users.error.selfAction`):
```json
{ "error": { "code": "FORBIDDEN", "message": "tidak boleh menonaktifkan atau menurunkan role akun sendiri", "details": { "reason": "SELF_ACTION_FORBIDDEN" } }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a42" }
```
409 super admin terakhir (UI: `users.error.lastSuperAdmin`):
```json
{ "error": { "code": "CONFLICT", "message": "minimal harus ada satu super admin aktif", "details": { "reason": "LAST_SUPER_ADMIN" } }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a43" }
```
400 body kosong (UI mencegahnya; bila tetap terjadi, `_` menjadi pesan form):
```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "permintaan tidak valid", "details": { "fields": { "_": "minimal satu field harus diisi" } } }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a44" }
```

### POST /api/users/{id}/deactivate dan /activate
Request: tanpa body. 200: `UserDTO` terbaru (`is_active` false/true). Idempoten: status yang sudah sama tetap 200.
Error: 403 `SELF_ACTION_FORBIDDEN` dan 409 `LAST_SUPER_ADMIN` (bentuk seperti PATCH), serta 404 `NOT_FOUND`.

### POST /api/users/{id}/reset-password
Request (mode otomatis):
```json
{}
```
Request (mode manual):
```json
{ "password": "sementara-baru-12345" }
```
200 (header `Cache-Control: no-store`; UI: tahap `OneTimeSecret` bila tidak null):
```json
{
  "user": { "id": "7d1e3b52-90aa-4f1c-8b2d-1c3e5a7b9d02", "email": "sari@tim.id", "name": "Sari", "role": "developer", "is_active": true, "must_change_password": true, "created_at": "2026-10-21T09:00:00Z", "created_by": "0b7c6a4e-2f1d-4c8a-9e3b-5d6f7a8b9c0d", "created_by_label": "admin@tim.id", "updated_at": "2026-10-21T10:00:00Z", "last_login_at": "2026-10-21T09:10:00Z" },
  "temporary_password": "Hn4wTq8ZmP2kRx7A"
}
```
403 reset akun sendiri (UI: `users.error.selfReset`; tombol sudah disabled):
```json
{ "error": { "code": "FORBIDDEN", "message": "gunakan ganti password untuk akun sendiri", "details": { "reason": "SELF_ACTION_FORBIDDEN" } }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a45" }
```
400 (UI: field password dialog):
```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "permintaan tidak valid", "details": { "fields": { "password": "password tidak boleh sama dengan email" } } }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a46" }
```

### Error sisi klien (`ApiError` dari `http.ts`, tanpa respons server yang valid)
```json
{ "status": 0, "code": "NETWORK", "message": "Tidak dapat terhubung ke server", "details": null }
```
```json
{ "status": 200, "code": "INVALID_RESPONSE", "message": "Respons server tidak sesuai format", "details": [ { "path": ["items"], "message": "Expected array, received string" } ], "traceId": "<X-Request-Id>" }
```
UI menampilkan keduanya lewat `ErrorState` (dengan trace id bila ada). `AbortError` dari request yang dibatalkan tidak ditampilkan.
