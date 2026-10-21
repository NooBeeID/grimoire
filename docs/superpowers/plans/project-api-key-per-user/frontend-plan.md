# T13 project/api-key per user — Frontend Plan

Tiket: `docs/tickets/T13-project-api-key-per-user.md` (revisi T04).
Mengikuti `overview.md` (foundation) dan `usecase-{listkeys,revokekey,createkey}.md` di folder yang sama; bila bertentangan, overview yang menang. Satu plan frontend untuk seluruh feature (tidak dipecah per usecase).

Konteks yang dibaca: `docs/tickets/README.md` bagian "Frontend (Svelte)", `DESIGN.md`, `.claude/skills/design-taste-frontend/references/app-screens.md`, kode T04 di `frontend/src/domain/project/api-key/**`, `frontend/src/app/views/workspace/ApiKeysTab.svelte`, `frontend/e2e/project-api-key.spec.ts`. Belum ada `CLAUDE.md`; `.claude/rules/` hanya berisi `migrations.md` (tidak relevan untuk FE).

---

## 1. Metadata

| Atribut | Nilai |
|---|---|
| Ticket | T13 (revisi T04) |
| Scope | frontend (bagian dari tiket fullstack) |
| Domain | project |
| Module | api-key (`frontend/src/domain/project/api-key`) |
| Feature yang disentuh | `listkeys` (komponen, view, mocks, tes); `createkey` hanya tes (teks i18n berubah) |
| Komposisi app | `frontend/src/app/views/workspace/ApiKeysTab.svelte` |
| Route | `/projects/:id/api-keys` (sudah ada, tidak berubah; tab workspace) |
| Bergantung pada | Backend T13 (tiga usecase) untuk e2e; unit/komponen tidak bergantung |

| Endpoint | Method | Version | Perubahan yang dirasakan FE |
|---|---|---|---|
| `/api/projects/{projectID}/api-keys` | GET | tanpa prefix versi (`/api`); `api-key.yml` `info.version` 0.1.0 | Isi tersaring per pemilik; skema `ApiKeyList` sama. Zod tidak berubah |
| `/api/projects/{projectID}/api-keys` | POST | sama | Pesan 409 berubah; `details.reason`/`limit` sama. FE memakai teks i18n sendiri |
| `/api/projects/{projectID}/api-keys/{keyID}/revoke` | POST | sama | Key milik orang lain = 404 `KEY_NOT_FOUND` (sudah ditangani T04) |

Kontrak bus: tidak ada (FE tidak memakai bus). Tidak ada perubahan service, types, atau skema Zod.

---

## 2. Summary Plan

Backend sudah menyaring daftar per pemilik, jadi user biasa otomatis hanya melihat key miliknya. Pekerjaan FE kecil dan tidak fungsional bagi user biasa (K8):

1. **Kolom "Dibuat oleh" khusus Super Admin.** `KeysTable` mendapat prop `showOwner?: boolean` (default `false`). Bila `true`, kolom baru `owner` (setelah "Nama") menampilkan `Avatar` bulat 24px + `created_by_label`, atau "Tidak diketahui" (redup, tanpa avatar) untuk key legacy (`created_by_label: null`).
2. **Kolom "Dibuat" menjadi tanggal saja** untuk semua peran. Saat ini kolom itu berisi `"{by} · {at}"`; setelah T13 `{by}` bagi user biasa selalu dirinya sendiri (informasi mati), dan bagi Super Admin pindah ke kolom owner. `createdLine` diganti `createdAtLine` (tanggal saja).
3. **Teks penjelasan.** Intro tab menambah "Key di sini hanya milik Anda."; Super Admin melihat intro khusus `introAdmin`. Hint nama, pesan `NAME_TAKEN`, dan `KEY_LIMIT_REACHED` merujuk "key aktif Anda" / "per user". Empty state menegaskan kepemilikan.
4. **Komposisi.** `ApiKeysTab` meneruskan `showOwner={isSuperAdmin}` dan `isSuperAdmin` (untuk intro) ke `ListKeysView`.
5. **Tes.** Unit komponen/view/tab diperbarui; e2e `project-api-key.spec.ts` mendapat skenario dua user dan Super Admin; asersi teks lama T04 diganti.

Tidak ada route, store, service, atau skema baru. Penanganan 404 `KEY_NOT_FOUND` di `RevokeKeyDialog` (toast + invalidate daftar) sudah ada dari T04 dan cukup: bagi UI, key orang lain memang tidak pernah tampil, sehingga kasus itu hanya muncul lewat panggilan API langsung atau daftar basi.

---

## 3. Visual

### 3.1 Pertanyaan layar dan blok terbesar

| Layar | Pertanyaan | Blok terbesar yang menjawab |
|---|---|---|
| Tab API Keys, user biasa | "Key saya yang mana yang aktif, dan kapan terakhir dipakai agent?" | Panel tabel key (nama, prefix mono, terakhir dipakai, status). Tidak ada kolom owner; satu-satunya pemilik adalah pembaca |
| Tab API Keys, Super Admin | "Key siapa yang aktif di project ini, dan kapan terakhir dipakai?" | Panel tabel yang sama + kolom "Dibuat oleh" (Avatar + email) setelah kolom Nama, sehingga identitas pemilik terbaca sebelum angka waktu |
| Dialog buat key (error) | "Kenapa key saya ditolak?" | Pesan galat field (`NAME_TAKEN`) atau galat form (`KEY_LIMIT_REACHED`) di dalam dialog, merujuk key milik sendiri |

Temuan (finding) yang ditaruh di baris bawah judul: intro tab. User biasa: "...Key di sini hanya milik Anda." Super Admin: "Sebagai Super Admin Anda melihat key semua anggota project." Ini kalimat tentang data (cakupan daftar), bukan instruksi UI.

### 3.2 Pola DESIGN.md dan kerangka

- Pola: **list** (baris "Tab API Keys" di tabel "Pola per layar": "Key mana yang aktif dan kapan terakhir dipakai?"; dialog hasil key tidak berubah).
- Kerangka: `AppLayout` framed-sheet (sidebar `bg-ground`, sheet putih radius 16) → workspace project (title row + tab garis bawah 2px `border-accent`) → tab API Keys. Tidak ada layar, route, atau kerangka baru.
- Header seksi tetap: h2 16px "API Keys" + jumlah aktif redup, intro satu kalimat `text-muted`, satu tombol utama "Buat key" (`bg-primary`) di kanan. Tombol utama tetap satu per layar.
- Tabel: komponen `Table` bersama (panel radius 8, `border-border`, `shadow-panel`, header 13px `text-muted` pada pita `bg-raised`, baris 48px, rule `border-rule`, hover `bg-raised/60`). Satu penanda status per baris (`KeyStatusBadge`). Baris dicabut tetap `opacity-60` seperti T04.
- Kolom owner adalah kolom identitas orang: `Avatar` bulat (DESIGN "Identitas pucat") 24px, email 14px `text-fg`, `truncate` satu baris dengan `title` berisi email lengkap. Kolom "Nama" tetap kolom identitas utama (terlebar).
- Aturan "kolom kosong di semua baris dibuang" terpenuhi: kolom owner tidak dirender untuk user biasa (seluruh isinya akan sama), dan kolom "Dibuat" tidak lagi mengulang nama pembuat.
- Urutan kolom Super Admin: Nama · Dibuat oleh · Prefix · Dibuat · Terakhir dipakai · Status · Aksi. Urutan user biasa: Nama · Prefix · Dibuat · Terakhir dipakai · Status · Aksi (sama dengan T04).

### 3.3 Elemen khas produk di layar pertama

- Prefix key dalam **IBM Plex Mono** (`font-mono`, `displayPrefix` → `gr_m7p2x9qa_…`): wajah produk #1 (mono untuk hal yang dibaca mesin).
- "Terakhir dipakai" = jejak pemakaian agent; nilai hilang ditulis `–` redup dengan `aria-label` "Belum pernah dipakai".
- Super Admin: **Avatar pucat** per pemilik di kolom "Dibuat oleh" (wajah produk #5), sehingga key milik Ana dan Budi terbaca terpisah sekilas tanpa warna status.
- Tab aktif bergaris bawah kuning `border-accent` 2px (kerangka workspace, tidak berubah).

### 3.4 Token / utility yang dipakai (tanpa nilai baru)

`bg-surface`, `bg-raised`, `bg-raised/60`, `border-border`, `border-rule`, `shadow-panel`, `rounded-card`, `text-fg`, `text-muted`, `text-accent-ink` (tautan Audit Log, sudah ada), `ring-accent-ink` (fokus), `bg-primary text-primary-fg` (tombol "Buat key"), `bg-pill-positive-bg text-pill-positive-ink` / `bg-pill-neutral-bg text-pill-neutral-ink` (via `KeyStatusBadge`, sudah ada), `font-mono`, `text-[13px]` (prefix, sudah ada), `text-xs` (baris "Dicabut oleh"), `tabular-nums` (global), `Avatar` (`shape="circle"`, `size={24}`).
Tidak ada warna, radius, font, shadow, atau komponen dekoratif baru. Tidak ada usulan perubahan `DESIGN.md`.

Catatan kecil (bukan penyimpangan): `KeysTable` saat ini menulis nilai hilang dengan `-` (hyphen). DESIGN.md meminta `–` (en dash). Ganti di sel "Terakhir dipakai" sekalian karena file disentuh; asersi tes `toHaveTextContent('-')` disesuaikan ke `–`.

### 3.5 Data contoh (mocks dan fixture tes)

Project "Payment Gateway Docs", tiga orang di tim kecil. Dipakai di `listkeys/mocks/keys.ts` (helper baru) dan tes komponen/tab:

| name | prefix | pemilik (`created_by_label`) | status | created_at | last_used_at |
|---|---|---|---|---|---|
| `claude-code-ana` | `a1b2c3d4` | `ana@tim.id` | active | 2026-10-23T10:00:00Z | 2026-10-23T11:42:00Z |
| `ci-pipeline` | `m7p2x9qa` | `ana@tim.id` | active | 2026-10-22T08:15:00Z | – |
| `claude-code-budi` | `k3x9q2ma` | `budi@tim.id` | revoked (oleh `admin@tim.id`) | 2026-10-21T09:00:00Z | 2026-10-21T16:05:00Z |
| `cursor-budi` | `p4q8r2zt` | `budi@tim.id` | active | 2026-10-23T13:00:00Z | – |
| `legacy-key` | `z9y8x7w6` | `null` (legacy) | active | 2026-10-20T08:00:00Z | – |

- Tampilan Ana (developer): 2 baris miliknya; header "2 key aktif" dihitung dari baris (`activeCount`), bukan diketik.
- Tampilan Super Admin: 5 baris; header "4 key aktif" (dihitung dari baris aktif).
- Dilarang Acme/John Doe/lorem. Prefix mengikuti format `gr_<8>_…` dari T04.

---

## 4. Route, komponen, state

### 4.1 Route

Tidak berubah. `/projects/:id/api-keys` (tab workspace, guard anggota/Super Admin dari T03/T04). Halaman `/audit-log` tetap `roles: ['super_admin']` (`frontend/src/app/routes/index.ts`), sudah diverifikasi di overview bagian 3; tidak ada perubahan.

### 4.2 Komponen dan view

| Unit | Lokasi | Perubahan |
|---|---|---|
| `KeysTable.svelte` | `listkeys/components/` | Prop baru `showOwner?: boolean = false`. `columns` disusun `$derived` dengan kolom `owner` disisipkan setelah `name` hanya bila `showOwner`. Snippet `cell`: `owner` → `<span class="flex min-w-0 items-center gap-2"><Avatar name={label} size={24} /><span class="truncate" title={label}>{label}</span></span>`; label null → `<span class="text-muted">{$t('project.apikey.unknownUser')}</span>` tanpa Avatar. `created` → `<time datetime=…>{createdAtLine(k, $locale)}</time>`. Nilai hilang `–` |
| `format.ts` | `listkeys/components/` | `createdLine` diganti `createdAtLine(key, locale) = formatDateTime(key.created_at, locale)`; `revokedLine` tetap ("Dicabut oleh {by} pada {at}", karena Super Admin bisa mencabut key user, dan pemilik perlu tahu siapa) |
| `ListKeysView.svelte` | `listkeys/views/` | Prop baru `showOwner?: boolean = false` (diteruskan ke `KeysTable`) dan `isSuperAdmin?: boolean = false` (memilih teks intro: `introAdmin` bila true, selain itu `intro`). Tidak ada logika peran lain di view; peran ditentukan oleh komposisi app |
| `index.ts` | `listkeys/` | Tidak berubah (props bukan ekspor baru). `format.ts` tetap internal |
| `mocks/keys.ts` | `listkeys/mocks/` | Tambah `mockTeamKeys()` (lima baris tabel 3.5) dan `ANA_ID`, `BUDI_ID`, `ADMIN_ID` |
| `ApiKeysTab.svelte` | `app/views/workspace/` | Teruskan `showOwner={isSuperAdmin}` dan `{isSuperAdmin}` ke `ListKeysView`. `auditHref` tetap hanya untuk Super Admin |
| `CreateKeyDialog.svelte`, `CreateKeyForm.svelte`, `RevokeKeyDialog.svelte`, services, types | `createkey/`, `revokekey/` | Tidak berubah. Teks berasal dari i18n |

Batas arsitektur: `KeysTable` mengimpor `Avatar` dari `shared/components` (diizinkan). Tidak ada import lintas feature/module. `isSuperAdmin` dibaca dari `shared/stores/session` hanya di `app/` (komposisi), bukan di `domain/`.

### 4.3 State

- `keyList.store` (`idle | loading | ready | error | notFound`, `previous`, `version`, `upsert`, `invalidate`) tidak berubah. `activeCount($store)` tetap dihitung dari item yang diterima, sehingga otomatis "key aktif saya" bagi user biasa dan "semua key aktif" bagi Super Admin.
- `upsert` dari hasil create Super Admin: key baru membawa `created_by_label` = email Super Admin, jadi kolom owner langsung terisi tanpa GET ulang.
- Tidak ada state baru. `showOwner`/`isSuperAdmin` adalah prop turunan dari `$session.identity.role`. Bila sesi berubah peran (logout/login ulang), tab dirender ulang oleh router.

### 4.4 Loading, empty, error

| State | Perilaku | Berubah? |
|---|---|---|
| Loading pertama | `Table` dengan `Spinner` di dalam panel (bingkai tetap), `aria-busy` di section | Tidak |
| Loading ulang | baris `previous` tetap tampil, `aria-busy=true` | Tidak |
| Empty (user biasa tanpa key, walau anggota lain punya key) | `EmptyState` title "Belum ada API key Anda" + hint "Key yang dibuat anggota lain tidak ditampilkan di sini. Buat key untuk menghubungkan agent Anda ke project ini." + tombol "Buat key" | Teks berubah |
| Empty (Super Admin, project tanpa key sama sekali) | title "Belum ada API key di project ini" (`emptyAdmin`) + hint `emptyHint` umum | Baru |
| Error GET | `ErrorState` + coba lagi (`store.invalidate()`) | Tidak |
| 404 project | `onprojectgone()` → halaman "Project tidak ditemukan" | Tidak |
| Create 409 `NAME_TAKEN` | galat field: "Anda sudah punya key aktif dengan nama ini" | Teks berubah |
| Create 409 `KEY_LIMIT_REACHED` | galat form: "Batas {limit} key aktif per user tercapai. Cabut key yang tidak dipakai." | Teks berubah |
| Revoke 404 `KEY_NOT_FOUND` | toast "Key tidak ditemukan. Daftar diperbarui." + `invalidate` | Tidak |
| Revoke 409 `ALREADY_REVOKED` | toast + `invalidate` | Tidak |

Agar empty state membedakan peran tanpa logika baru di domain, `ListKeysView` memakai prop `isSuperAdmin` yang sama dengan intro.

---

## 5. Plan (langkah berurutan)

Tidak menunggu backend untuk langkah 1-6 (tanpa perubahan API). Langkah 7 (e2e) butuh backend T13 di branch.

1. **i18n** `frontend/src/shared/i18n/id.json` dan `en.json` (kunci flat, posisi di blok `project.apikey.*`):

   | Kunci | id | en | Status |
   |---|---|---|---|
   | `project.apikey.intro` | `Key dipakai agent (mis. Claude Code) untuk membaca project ini lewat MCP. Key hanya bisa membaca dan tidak bisa dipakai untuk login. Key di sini hanya milik Anda.` | `Keys let agents (e.g. Claude Code) read this project over MCP. Keys are read-only and cannot be used to sign in. Only your own keys are listed here.` | ubah |
   | `project.apikey.introAdmin` | `Key dipakai agent (mis. Claude Code) untuk membaca project ini lewat MCP. Sebagai Super Admin Anda melihat key semua anggota project.` | `Keys let agents (e.g. Claude Code) read this project over MCP. As a Super Admin you see the keys of every project member.` | baru |
   | `project.apikey.empty` | `Belum ada API key Anda` | `You have no API keys yet` | ubah |
   | `project.apikey.emptyHint` | `Key yang dibuat anggota lain tidak ditampilkan di sini. Buat key untuk menghubungkan agent Anda ke project ini.` | `Keys created by other members are not listed here. Create a key to connect your agent to this project.` | ubah |
   | `project.apikey.emptyAdmin` | `Belum ada API key di project ini` | `No API keys in this project yet` | baru |
   | `project.apikey.emptyAdminHint` | `Buat key untuk menghubungkan agent ke project ini.` | `Create a key to connect an agent to this project.` | baru |
   | `project.apikey.col.owner` | `Dibuat oleh` | `Created by` | baru |
   | `project.apikey.create.nameHint` | `2-60 karakter, unik di antara key aktif Anda. Contoh: claude-code-budi` | `2-60 characters, unique among your active keys. Example: claude-code-budi` | ubah |
   | `project.apikey.error.NAME_TAKEN` | `Anda sudah punya key aktif dengan nama ini` | `You already have an active key with this name` | ubah |
   | `project.apikey.error.KEY_LIMIT_REACHED` | `Batas {limit} key aktif per user tercapai. Cabut key yang tidak dipakai.` | `You have reached the limit of {limit} active keys per user. Revoke unused keys.` | ubah |
   | `project.apikey.createdBy` | – | – | hapus (tidak dipakai lagi setelah `createdAtLine`); pastikan `grep -rn "apikey.createdBy" frontend/src` kosong |

   Pesan tidak menyebut nama/pemilik lain (anti-tujuan T13). Tes kesetaraan kunci id/en (bila ada di `shared/i18n`) harus tetap hijau.
2. **`listkeys/components/format.ts`**: ganti `createdLine` dengan `createdAtLine`; perbarui `format.test.ts`.
3. **`listkeys/components/KeysTable.svelte`**: prop `showOwner`, kolom `owner` kondisional, sel `created` tanggal saja, `–` untuk nilai hilang, import `Avatar`. Tes di `components.test.ts` (bagian 7.1) ditulis dulu (merah), lalu implementasi.
4. **`listkeys/views/ListKeysView.svelte`**: prop `showOwner`, `isSuperAdmin`; pilih `intro`/`introAdmin` dan `empty`/`emptyAdmin` (+ hint). Tes `ListKeysView.test.ts`.
5. **`listkeys/mocks/keys.ts`**: `mockTeamKeys()` dan konstanta ID (tabel 3.5). Mock yang ada tetap.
6. **`app/views/workspace/ApiKeysTab.svelte`**: teruskan `showOwner={isSuperAdmin}` dan `{isSuperAdmin}`. Tes `ApiKeysTab.test.ts`.
7. **`createkey/views/CreateKeyDialog.test.ts`**: perbarui asersi teks `NAME_TAKEN` (`/sudah punya key aktif dengan nama ini/`) dan `KEY_LIMIT_REACHED` (`/Batas 20 key aktif per user tercapai/`). Kode dialog tidak berubah.
8. **e2e** `frontend/e2e/helpers/apikeys.ts`: tambah `revokeKeyRawApi(request, projectId, keyId)` yang mengembalikan `APIResponse` tanpa `expect(200)` (untuk asersi 404). `frontend/e2e/project-api-key.spec.ts`: skenario T13 (bagian 8) dan pembaruan asersi T04 (`T04-N3`, `T04-N4`, intro/empty bila diasersi).
9. **Verifikasi**: `npm run lint`, `npm run check` (svelte-check), `npm run test`, `npm run depcruise`, `npm run test:arch`, `node .agents/skills/design-taste-frontend/scripts/lint.mjs --app frontend/src/domain/project/api-key frontend/src/app/views/workspace/ApiKeysTab.svelte`, lalu `npx playwright test e2e/project-api-key.spec.ts e2e/mcp-gateway.spec.ts` dengan backend T13. Design check `inspect.mjs --app` pada `/projects/<id>/api-keys` bila dev server + backend tersedia (1440, 768, 390): kolom owner tidak memicu scroll horizontal halaman (tabel sudah dibungkus `overflow-x-auto`).

---

## 6. Migration

Tidak ada (frontend). Migrasi backend `backend/migrations/project/<YYYYMMDDHHMMSSmmm>_api_keys_owner_scope.sql` dimiliki foundation (overview bagian 6).

---

## 7. Test Scenario (unit / komponen, Vitest + Testing Library)

### 7.1 `listkeys/components/components.test.ts` (KeysTable)

| Kasus | Setup | Harapan |
|---|---|---|
| Default tanpa kolom owner | `props(mockTeamKeys().slice(0,2))` | `columnheader` tidak memuat "Dibuat oleh"; tidak ada elemen `.avatar`; teks `ana@tim.id` tidak muncul di sel mana pun |
| Kolom owner untuk Super Admin | `showOwner: true`, lima baris | ada `columnheader` "Dibuat oleh" di indeks 1 (setelah "Nama"); baris `cursor-budi` memuat `budi@tim.id` dan satu `.avatar` berinisial "B" |
| Legacy tanpa pemilik | `showOwner: true`, `legacy-key` (`created_by_label: null`) | sel owner berisi "Tidak diketahui" dengan kelas `text-muted`, tanpa `.avatar` |
| Kolom Dibuat tanggal saja | default dan `showOwner: true` | `time[datetime="2026-10-23T10:00:00Z"]` ada; teksnya tidak memuat `@` maupun `·` |
| Belum pernah dipakai | `last_used_at: null` | `getByLabelText('Belum pernah dipakai')` berteks `–` |
| Baris dicabut | `claude-code-budi` revoked oleh admin | `opacity-60`, `data-status="revoked"`, "Dicabut oleh admin@tim.id pada …", tanpa tombol Cabut (tes T04 dipertahankan) |
| Email panjang | label 60 karakter, `showOwner: true` | span email punya `truncate` dan `title` = email lengkap |
| Tetap aman | tes T04 `raw_key` tidak dirender, nama literal (XSS), tanpa tombol Hapus, tautan audit hanya bila `auditHref` | tetap hijau |

Hapus/ganti tes T04 `unknown creator label` (kini hanya berlaku saat `showOwner: true`, dicakup kasus "Legacy").

### 7.2 `listkeys/components/format.test.ts`

- `createdAtLine` mengembalikan `formatDateTime(created_at, locale)` untuk `id` dan `en`; tidak memuat label pembuat.
- `revokedLine` tidak berubah (label null → "Tidak diketahui", `revoked_at` null → `-`).

### 7.3 `listkeys/views/ListKeysView.test.ts`

| Kasus | Harapan |
|---|---|
| Developer (default props), respons 2 key | intro berisi "Key di sini hanya milik Anda."; tidak ada "Dibuat oleh"; header "2 key aktif" |
| `isSuperAdmin: true, showOwner: true`, respons 5 key | intro `introAdmin` ("Sebagai Super Admin Anda melihat key semua anggota project."); kolom "Dibuat oleh"; header "4 key aktif" (dihitung dari baris) |
| Developer, `{"items": []}` | `EmptyState` "Belum ada API key Anda" + hint "Key yang dibuat anggota lain tidak ditampilkan di sini…" + tombol "Buat key" |
| Super Admin, `{"items": []}` | "Belum ada API key di project ini" |
| Error 500 | `ErrorState`, tombol coba lagi memanggil GET kedua (tes T04 tetap) |
| 404 project | `onprojectgone` dipanggil sekali (tes T04 tetap) |
| Loading | `aria-busy="true"` dan spinner di dalam panel tabel (tes T04 tetap) |

### 7.4 `app/views/workspace/ApiKeysTab.test.ts`

| Kasus | Harapan |
|---|---|
| Developer | tanpa kolom "Dibuat oleh", tanpa tautan Audit Log, intro milik sendiri |
| Super Admin | kolom "Dibuat oleh" berisi `ana@tim.id`; intro `introAdmin`; tautan audit dengan href terfilter (tes T04 dipertahankan) |
| Super Admin membuat key | setelah dialog selesai, baris baru menampilkan email Super Admin (`a@tim.id` di fixture) di kolom owner tanpa GET kedua |
| Create/revoke tanpa GET kedua | tes T04 dipertahankan |
| Revoke 404 `KEY_NOT_FOUND` | toast "Key tidak ditemukan. Daftar diperbarui." dan GET ulang satu kali (kunci perilaku T04 sekarang juga untuk key milik orang lain) |

Fixture `mockKey` lokal di file ini diperbarui agar `created_by` berisi UUID (bukan `null`) kecuali kasus legacy.

### 7.5 `createkey/views/CreateKeyDialog.test.ts`

- `NAME_TAKEN`: teks "Anda sudah punya key aktif dengan nama ini", input tetap berisi nilai, form tetap terbuka.
- `KEY_LIMIT_REACHED` `limit: 20`: "Batas 20 key aktif per user tercapai. Cabut key yang tidak dipakai."
- `KEY_LIMIT_REACHED` tanpa `limit` numerik: jatuh ke `e.message` server (perilaku T04 tetap).
- `CreateKeyForm`: hint "2-60 karakter, unik di antara key aktif Anda…" tampil sebagai deskripsi field (`aria-describedby`).

### 7.6 Arsitektur dan desain

- `npm run depcruise` dan `npm run test:arch`: `KeysTable` → `shared/components/Avatar` diizinkan; tidak ada import `shared/stores/session` dari `domain/project/api-key/**`.
- `lint.mjs --app` pada file yang diubah: tanpa FAIL (tidak ada hex baru, radius baru, emoji).

---

## 8. E2E Scenarios (`frontend/e2e/project-api-key.spec.ts`, Playwright, binary sungguhan)

Setup bersama T13: `admin = adminContext`, `ana = createReadyUserApi(admin.request, { name: 'Ana E2E' })`, `budi = createReadyUserApi(..., { name: 'Budi E2E' })`. Ana login dan membuat project lewat `createProjectApi` (Ana = owner), lalu `addMemberApi(anaCtx.request, proj.id, budi.id, 'member')`.

### Positif

| ID | Skenario | Harapan |
|---|---|---|
| T13-P1 | Ana `createKeyApi(a-key)`, Budi `createKeyApi(b-key)`. Ana membuka `/projects/:id/api-keys` | baris `a-key` tampil; `b-key` tidak ada di halaman; intro memuat "Key di sini hanya milik Anda."; kolom "Dibuat oleh" tidak ada (`columnheader` count 0) |
| T13-P2 | Budi membuka tab yang sama | hanya `b-key`; `listKeysApi(budi)` tidak memuat id/prefix/nama `a-key` (cek `JSON.stringify`) |
| T13-P3 | Ana dan Budi masing-masing membuat key bernama `shared-<uniq>` lewat dialog UI | keduanya menampilkan raw key sekali; tidak ada pesan NAME_TAKEN |
| T13-P4 | Ana mencabut `a-key` dari UI | baris "Dicabut"; Budi `listKeysApi` tidak berubah; Ana membuat ulang nama `a-key` → sukses (dua baris, satu aktif satu dicabut) |
| T13-P5 | Super Admin (non-anggota) membuka tab project | melihat `a-key` dan `b-key`; kolom "Dibuat oleh" berisi email Ana dan Budi di baris masing-masing; intro "Sebagai Super Admin Anda melihat key semua anggota project." |
| T13-P6 | Super Admin mencabut `b-key` dari UI | baris "Dicabut" dengan "Dicabut oleh e2e-admin@tim.id"; Budi melihat `b-key` dicabut dengan baris yang sama; audit `apikey.revoked` dengan target `b-key` ada (via `admin.request.get('/api/audit-log?target=…')`) |
| T13-P7 | Super Admin membuat key lewat dialog | baris baru langsung menampilkan `e2e-admin@tim.id` di kolom "Dibuat oleh"; Ana tidak melihat key itu |
| T13-P8 | Kuota per user: Ana diisi sampai `KEY_LIMIT_REACHED` lewat API (pola T04-N4, batas dibaca dari `details.limit`) | Budi `createKeyApi` masih 201 |
| Regresi MCP | `frontend/e2e/mcp-gateway.spec.ts` tanpa perubahan | key buatan anggota tetap memberi akses MCP |

### Negatif

| ID | Skenario | Harapan |
|---|---|---|
| T13-N1 | Budi `revokeKeyRawApi(proj, aKey.id)` | 404, `error.code = NOT_FOUND`, `error.details.reason = KEY_NOT_FOUND`; `listKeysApi(ana)` menunjukkan `a-key` masih `active` |
| T13-N2 | Budi `revokeKeyRawApi(proj, crypto.randomUUID())` | 404; objek `error` identik dengan T13-N1 (bandingkan setelah membuang `trace_id`) |
| T13-N3 | Budi `revokeKeyRawApi(proj, 'bukan-uuid')` | 404 dengan `error` identik |
| T13-N4 | Ana membuat key kedua bernama sama dengan key aktifnya lewat dialog | galat "Anda sudah punya key aktif dengan nama ini"; `raw-key` tidak tampil; jumlah key Ana tetap |
| T13-N5 | Ana mencapai batas lalu membuat key lewat dialog | galat "Batas {limit} key aktif per user tercapai" |
| T13-N6 | Budi membuka tab | tidak ada kolom "Dibuat oleh", tidak ada tautan Audit Log; `/audit-log` untuk Budi tetap ditolak (redirect/403 sesuai T02) |
| T13-N7 | Non-anggota membuka tab / GET API | "Project tidak ditemukan" / 404 (tes T04 tetap) |
| T13-N8 | Budi tanpa key membuka tab saat Ana punya key | `EmptyState` "Belum ada API key Anda" dan hint anggota lain; tidak ada baris tabel |

Pembaruan tes T04 di file yang sama: `T04-N3` asersi teks → "Anda sudah punya key aktif dengan nama ini"; `T04-N4` → `Batas ${limit} key aktif per user tercapai`; `T04-AC1` asersi "Belum ada API key" → "Belum ada API key Anda". `T04-P5` tetap (Super Admin membuat dan mencabut key sendiri; sekarang juga memeriksa kolom owner berisi email admin).

---

## 9. Request/Response body (yang dikonsumsi FE)

Format error baku (`infra/http/respond`), dibaca `shared/services/http.ts` menjadi `ApiError { status, code, message, details, traceId }`:
```json
{ "error": { "code": "NOT_FOUND", "message": "API key tidak ditemukan", "details": { "reason": "KEY_NOT_FOUND" } }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```

### GET `/api/projects/{projectID}/api-keys`

Request: tanpa body, cookie sesi.

200 sebagai Ana (developer; hanya miliknya):
```json
{
  "items": [
    {
      "id": "0192f5a0-2222-7aaa-8bbb-000000000001",
      "name": "claude-code-ana",
      "prefix": "a1b2c3d4",
      "status": "active",
      "created_by": "0192f5a0-aaaa-7aaa-8bbb-00000000000a",
      "created_by_label": "ana@tim.id",
      "created_at": "2026-10-23T10:00:00Z",
      "last_used_at": "2026-10-23T11:42:00Z",
      "revoked_at": null,
      "revoked_by": null,
      "revoked_by_label": null
    },
    {
      "id": "0192f5a0-2222-7aaa-8bbb-000000000002",
      "name": "ci-pipeline",
      "prefix": "m7p2x9qa",
      "status": "active",
      "created_by": "0192f5a0-aaaa-7aaa-8bbb-00000000000a",
      "created_by_label": "ana@tim.id",
      "created_at": "2026-10-22T08:15:00Z",
      "last_used_at": null,
      "revoked_at": null,
      "revoked_by": null,
      "revoked_by_label": null
    }
  ]
}
```

200 sebagai Super Admin (semua anggota + legacy; dipotong ke dua baris yang relevan untuk kolom owner):
```json
{
  "items": [
    {
      "id": "0192f5a0-2222-7aaa-8bbb-000000000004",
      "name": "cursor-budi",
      "prefix": "p4q8r2zt",
      "status": "active",
      "created_by": "0192f5a0-bbbb-7aaa-8bbb-00000000000b",
      "created_by_label": "budi@tim.id",
      "created_at": "2026-10-23T13:00:00Z",
      "last_used_at": null,
      "revoked_at": null,
      "revoked_by": null,
      "revoked_by_label": null
    },
    {
      "id": "0192f5a0-2222-7aaa-8bbb-000000000009",
      "name": "legacy-key",
      "prefix": "z9y8x7w6",
      "status": "active",
      "created_by": null,
      "created_by_label": null,
      "created_at": "2026-10-20T08:00:00Z",
      "last_used_at": null,
      "revoked_at": null,
      "revoked_by": null,
      "revoked_by_label": null
    }
  ]
}
```

200 developer tanpa key: `{ "items": [] }`.

404 non-anggota: `{ "error": { "code": "NOT_FOUND", "message": "project tidak ditemukan", "details": null }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }`

### POST `/api/projects/{projectID}/api-keys`

Request (header `X-Requested-With`):
```json
{ "name": "claude-code-budi" }
```

201 (bentuk tidak berubah):
```json
{
  "key": {
    "id": "0192f5a0-2222-7aaa-8bbb-000000000004",
    "name": "claude-code-budi",
    "prefix": "m7p2x9qa",
    "status": "active",
    "created_by": "0192f5a0-bbbb-7aaa-8bbb-00000000000b",
    "created_by_label": "budi@tim.id",
    "created_at": "2026-10-23T13:00:00Z",
    "last_used_at": null,
    "revoked_at": null,
    "revoked_by": null,
    "revoked_by_label": null
  },
  "raw_key": "gr_m7p2x9qa_Q2xhdWRlLWNvZGUtYnVkaS1zZWNyZXQtZXhhbXBsZS0xMjM0NTY",
  "mcp_url": "https://grimoire.tim.id/mcp"
}
```

409 NAME_TAKEN (FE menampilkan i18n `project.apikey.error.NAME_TAKEN`, bukan `message`):
```json
{ "error": { "code": "CONFLICT", "message": "nama key sudah dipakai key aktif Anda yang lain di project ini", "details": { "reason": "NAME_TAKEN" } }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```

409 KEY_LIMIT_REACHED (FE memakai `details.limit`):
```json
{ "error": { "code": "CONFLICT", "message": "batas 20 key aktif per user per project tercapai; cabut key yang tidak dipakai", "details": { "reason": "KEY_LIMIT_REACHED", "limit": 20 } }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```

400 (field):
```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "permintaan tidak valid", "details": { "fields": { "name": "nama key wajib 2-60 karakter" } } }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```

### POST `/api/projects/{projectID}/api-keys/{keyID}/revoke`

Request: tanpa body, header `X-Requested-With`.

200 Super Admin mencabut key Budi:
```json
{
  "key": {
    "id": "0192f5a0-2222-7aaa-8bbb-000000000004",
    "name": "cursor-budi",
    "prefix": "p4q8r2zt",
    "status": "revoked",
    "created_by": "0192f5a0-bbbb-7aaa-8bbb-00000000000b",
    "created_by_label": "budi@tim.id",
    "created_at": "2026-10-23T13:00:00Z",
    "last_used_at": null,
    "revoked_at": "2026-10-23T14:00:00Z",
    "revoked_by": "0192f5a0-cccc-7aaa-8bbb-00000000000c",
    "revoked_by_label": "admin@tim.id"
  }
}
```

404 (key milik orang lain, keyID acak, atau bukan uuid; identik kecuali `trace_id`):
```json
{ "error": { "code": "NOT_FOUND", "message": "API key tidak ditemukan", "details": { "reason": "KEY_NOT_FOUND" } }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```

409 (sudah dicabut, milik sendiri):
```json
{ "error": { "code": "CONFLICT", "message": "API key sudah dicabut", "details": { "reason": "ALREADY_REVOKED" } }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```

401 (tanpa sesi; FE mengarahkan ke login):
```json
{ "error": { "code": "UNAUTHORIZED", "message": "perlu login", "details": null }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```

---

## 10. Changes (per file)

| File | Jenis | Isi |
|---|---|---|
| `frontend/src/domain/project/api-key/listkeys/components/KeysTable.svelte` | ubah | prop `showOwner`, kolom `owner` (Avatar + email / "Tidak diketahui"), sel `created` tanggal saja, `–` |
| `frontend/src/domain/project/api-key/listkeys/components/format.ts` | ubah | `createdLine` → `createdAtLine` |
| `frontend/src/domain/project/api-key/listkeys/components/format.test.ts` | ubah | 7.2 |
| `frontend/src/domain/project/api-key/listkeys/components/components.test.ts` | ubah | 7.1 |
| `frontend/src/domain/project/api-key/listkeys/views/ListKeysView.svelte` | ubah | prop `showOwner`, `isSuperAdmin`; intro/empty per peran |
| `frontend/src/domain/project/api-key/listkeys/views/ListKeysView.test.ts` | ubah | 7.3 |
| `frontend/src/domain/project/api-key/listkeys/mocks/keys.ts` | ubah | `mockTeamKeys()`, konstanta ID |
| `frontend/src/domain/project/api-key/createkey/views/CreateKeyDialog.test.ts` | ubah | asersi teks 7.5 |
| `frontend/src/domain/project/api-key/createkey/components/components.test.ts` | ubah (bila mengasersi hint) | hint baru |
| `frontend/src/app/views/workspace/ApiKeysTab.svelte` | ubah | teruskan `showOwner`, `isSuperAdmin` |
| `frontend/src/app/views/workspace/ApiKeysTab.test.ts` | ubah | 7.4 |
| `frontend/src/shared/i18n/id.json`, `frontend/src/shared/i18n/en.json` | ubah | kunci di langkah 1 |
| `frontend/e2e/helpers/apikeys.ts` | ubah | `revokeKeyRawApi` |
| `frontend/e2e/project-api-key.spec.ts` | ubah | skenario T13 + pembaruan asersi T04 |
| `listkeys/{services,stores,types.ts,index.ts}`, `createkey/**` (kode), `revokekey/**`, `shared/components/**`, `app/routes/**`, `DESIGN.md`, `app.css` | TIDAK berubah | |

---

## 11. Risiko dan catatan

- **K7 tidak terlihat di UI**: `listprojects/types.ts` membawa `counts.keys`, tetapi `ProjectTable.svelte` tidak merender angka key. Jadi selisih "jumlah total project vs key milik sendiri" tidak muncul di FE saat ini. Bila kelak kolom key ditambahkan ke daftar project, angka itu harus diberi label "key aktif di project" atau menunggu keputusan Pertanyaan terbuka #1 T13.
- **Audit log**: tautan Audit Log tetap hanya untuk Super Admin dan route `/audit-log` khusus `super_admin` (terverifikasi di overview bagian 3), jadi nama key orang lain tidak bocor lewat FE.
- **Fixture tes T04** yang memakai `created_by: null` untuk developer kini tidak realistis (backend tidak akan mengembalikannya ke developer). Ganti dengan UUID pemilik agar tes mencerminkan kontrak baru; `null` hanya untuk kasus legacy Super Admin.
- Keputusan desain di plan ini yang sedikit melampaui overview bagian 9: kolom "Dibuat" menjadi tanggal saja untuk **semua** peran (overview menyebutnya untuk mode `showOwner`), karena bagi user biasa nama pembuat selalu dirinya sendiri dan DESIGN.md melarang kolom yang isinya tidak membedakan baris. Empty state Super Admin memakai kunci baru `emptyAdmin`/`emptyAdminHint` agar hint "anggota lain tidak ditampilkan" tidak salah bagi Super Admin.
