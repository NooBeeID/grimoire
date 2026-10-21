# DESIGN.md — Grimoire (NooBee)

Sumber kebenaran visual untuk seluruh frontend. Plan frontend merujuk file ini; `frontend/src/app.css` adalah turunannya (ubah file ini dulu, lalu `app.css`).

Arah: **framed-sheet** dari tasteskill (`.claude/skills/design-taste-frontend`, `references/app-screens.md`), seed `2129925594`, kind `devtools`, terang dan gelap lewat toggle (ikon di sidebar dan halaman login; pilihan disimpan di `localStorage` `grimoire.theme`, default mengikuti sistem; tema ditentukan `public/theme-init.js` sebelum render dan `shared/stores/theme.ts`, lewat `data-theme` di `<html>`). Sheet mentahnya ada di `.taste-skill/app/direction.json` (`node .agents/skills/design-taste-frontend/scripts/draw.mjs --show`).

**Kenapa framed-sheet.** Grimoire dipakai tim kecil dalam sesi kerja panjang: membaca dan mengedit Markdown, memindai tabel key, spec, dan audit. Itu butuh chrome yang diam dan permukaan baca yang putih. Warna hanya di data dan status, bukan di kerangka.

## Wajah produk (yang membuat ini bukan template)

1. **Mono untuk satu pekerjaan.** IBM Plex Mono khusus untuk hal yang dibaca mesin: path file knowledge, prefiks/ID API key, `operationId`, method + path endpoint, aksi audit (`project.apikey.create`), URL MCP. Jangan untuk judul atau label.
2. **Kerangka diberi rona dari produk.** `--color-ground` `#e8eff2` (biru-abu dingin) menjadi warna kerangka; sheet putih terangkat dari kerangka dengan bayangan lembut, sehingga ada dua lapisan yang terbaca. Rona hanya di kerangka, bukan di data.
3. **Aksen kuning** dengan dua token: `accent` `#f2b705` (gelap: `#f5c518`) untuk isian dan garis: ubin logo (glyph `on-accent` gelap), garis bawah tab aktif 2px, chip agent di audit, seri chart utama. `accent-ink` `#7a5600` (gelap: `#f5c518`) untuk teks, ikon item nav aktif, cincin fokus, caret, karena kuning cerah tidak lolos kontras 4.5:1 di atas putih. Seleksi teks memakai kuning 24%. Bukan di tombol, bukan di latar panel. Teal `#1b6c80` tetap dipakai hanya sebagai seri chart ketiga.
4. **Elemen khas: jejak agent.** Baris yang berasal dari agent (API key, entri audit `actor_type=api_key`) memakai chip kotak kuning-pucat dengan ikon agent, label mono, dan nama aksesibel; manusia memakai chip bulat netral. Manusia vs agent harus terbaca sekilas di tabel audit.
5. **Identitas pucat.** `Avatar` (inisial + hue stabil dari nama, 6 hue) untuk orang (bulat) dan project (kotak, radius 8). Nada pucat, tidak pernah dipakai untuk status atau chart.

## Token

Semua token ada di `frontend/src/app.css` (`@theme` + override gelap). Nama lama T00 (`primary`, `danger`, `surface`, `fg`, `muted`, `border`, `rounded-base`) dipertahankan sebagai alias. Pakai nama baru di kode baru.

| Peran | Utility | Terang | Catatan |
|---|---|---|---|
| Kerangka (sidebar + top bar) | `bg-ground` | `#f9fbfc` | tanpa garis antara sidebar dan top bar |
| Sheet / panel | `bg-surface` | `#ffffff` | sheet: 1px `border-border`, radius 16, `shadow-sheet`; panel tabel: radius 8, `shadow-panel` |
| Panel kedua (hover, code block) | `bg-raised` | `#f4f6f8` | |
| Baris terpilih | `bg-active` | `#e3eaee` | baris terpilih di tabel; item nav aktif = pil putih `bg-surface` + `shadow-xs` |
| Bayangan | `shadow-xs` `shadow-button` `shadow-panel` `shadow-sheet` `shadow-pop` | berlapis, warna ink produk | xs: kontrol/pil aktif; button: tombol utama; panel: tabel; sheet: sheet + login; pop: dialog/toast |
| Garis | `border-border` / `border-rule` | `#e8eaec` / `#f0f1f2` | kartu tidak boleh berborder lebih tebal dari sheet |
| Kontrol | `border-control` | `#cdced1` | input, tombol sekunder |
| Teks / redup | `text-fg` / `text-muted` | `#131416` / `#5d5f60` | |
| Aksi utama | `bg-primary text-primary-fg` | `#131416` / putih | **satu per layar**, near-black |
| Aksen | `bg-accent` `border-accent` (isian/garis), `text-accent-ink` `ring-accent-ink` (teks/fokus), `text-on-accent` (di atas kuning) | `#f2b705` / `#7a5600` / `#1d1600` | lihat "Wajah produk" #3 |
| Status | `text-positive` `text-danger` `text-warning` | `#1e7b4f` `#c4242a` `#92400e` | selalu disertai teks/ikon, tidak warna saja |
| Pil status | `bg-pill-{positive,negative,warning,neutral}-bg text-pill-…-ink` | lihat `app.css` | |
| Chart | `chart-1/2/3` | `#c98f00` `#2f70bb` `#1b6c80` | chart-1 = data utama |

Radius: kontrol 6 (`rounded-base`), kartu 8 (`rounded-card`), sheet 16 (`rounded-sheet`). Tidak ada radius lain.
Ukuran: judul 22, seksi 16, KPI 28 (weight 400), badan 14, kecil 12; tidak ada teks di bawah 12px (min. 11px untuk label tabel). Bobot 400/500/600; 600 hanya untuk label dan tab aktif. Sentence case, tanpa huruf kapital berjarak.
Ritme: satu nilai gap (24) horizontal dan vertikal, padding sheet 24, tinggi kontrol 32 (tombol utama di title row 36), tinggi baris tabel 48.
Angka: `tabular-nums` global di `body`; angka rata kanan; satuan sebagai sufiks redup.
Font dimuat dari `@fontsource/*` (di-bundle Vite, `font-src 'self'` aman). Jangan menambah CDN font.

## Kerangka (shell)

`AppLayout`: sidebar 228px di atas `bg-ground` + **sheet putih** inset 8px dari atas, kanan, dan bawah jendela (menempel ke sidebar di kiri), radius 16, 1px border, tanpa shadow. Sidebar memuat tanda + nama produk (buku terbuka di ubin kuning), item navigasi dengan ikon outline 16px, lalu di bawah: baris user (inisial + nama, tautan ke profil), `LocaleSwitch`, dan "Keluar". **Tidak ada top bar di desktop** (tidak ada isi nyata untuk bar itu selama belum ada pencarian global); top bar 48px hanya ada di ponsel (nama produk + tombol Menu yang membuka sidebar). Item aktif = pil `bg-active` + teks `text-fg` weight 500; tidak ada bar kiri berwarna.

Judul layar memakai `PageHeader` (h1 22px, satu kalimat redup opsional, aksi nyata di kanan). Tidak ada sapaan di layar kerja.

> Status: kerangka, `Button`, `Input`/`Select`/`Textarea`, `Table`, `Dialog`, `Toast`, pil status, dan layar daftar project, workspace/anggota, audit log, users, serta login sudah memakai token ini. Tabel membuang kolom yang kosong di semua baris (mis. Files/Specs sebelum T05/T06). Aksi baris anggota/user masih tombol ghost (belum kebab menu); ganti ke menu bila baris mulai memuat lebih dari dua aksi.

## Pola per layar

Setiap layar menjawab **satu pertanyaan**; blok terbesar menjawabnya. Tulis pertanyaannya di plan.

| Layar | Pertanyaan | Pola |
|---|---|---|
| `/` daftar project | Project mana yang saya buka? | list: title row (judul + satu tombol "Project baru"), toolbar (cari, jumlah), tabel tanpa kartu pembungkus |
| Workspace project (`/projects/:id/:tab`) | Apa isi project ini? | title row + tab garis bawah 2px (lebar sebesar label) pada rule 1px |
| Tab Knowledge | File mana yang agent baca, dan apa isinya? | dua kolom: pohon file (mono) + panel baca/edit Markdown; tidak ada KPI |
| Tab API Spec | Endpoint apa yang tersedia? | list spec → detail: daftar endpoint (method pil netral + path mono) |
| Tab API Keys | Key mana yang aktif dan kapan terakhir dipakai? | list; dialog hasil key: peringatan "sekali tampil" + `CopyField` mono |
| `/audit-log` | Siapa (manusia atau agent) melakukan apa, kapan? | list + filter bar + panel detail kanan (overlay di layar sempit) |
| `/admin/users` | Siapa saja yang punya akses? | list dengan status pil |
| `/login`, ganti password | — | `BlankLayout`: satu sheet terpusat, satu tombol utama |

## Aturan wajib

- Warna di data dan status, bukan di kerangka. Target <3% piksel berwarna.
- Tanpa gradien, glow, glass, ikon dalam kotak berwarna (kecuali chip agent dan ubin logo, lihat "Wajah produk"), sparkline di KPI, emoji, chart pelangi, huruf kapital berjarak, zebra. Bayangan selalu berlapis (kontak tipis + hamparan lembut, offset + blur) dan berwarna ink produk; tidak ada halo tanpa offset dan tidak ada bayangan keras.
- Tabel: dibungkus panel (radius 8, border, `shadow-panel`), header 13px abu medium pada pita `bg-raised`, baris 48px, rule 1px `border-rule`, hover `bg-raised/60`, kolom identitas paling lebar dengan `Avatar`, angka rata kanan, satu penanda status per baris. Kolom yang kosong di semua baris dibuang.
- Tidak ada teks sapaan ("Selamat datang kembali 👋"), tidak ada hero di layar kerja; judul 22px.
- Nol ditulis `0`, nilai hilang ditulis `–` (abu). Empty state tetap menjaga bingkai bagian itu dan memberi aksi yang relevan.
- Data contoh (mock FE slicing, fixture test visual) memakai kata dan angka produk ini: nama project nyata-masuk-akal, path knowledge seperti `docs/auth/login-flow.md`, prefiks key seperti `nbk_4f9a…`. Dilarang Acme, John Doe, lorem, "+12%" pada akun baru. Angka yang tampil dua kali adalah satu variabel; total dihitung dari baris.
- Gerak hanya sebagai umpan balik (tekan, hover, fokus, dialog/menu buka), transisi 120ms ease-out, tekan menggeser 0.5px. Tidak ada animasi masuk. Hormati `prefers-reduced-motion`.
- Permukaan bawaan browser ikut didesain: seleksi teks kuning 24%, caret dan cincin fokus `accent-ink`, scrollbar tipis.
- A11y tetap berlaku seperti di plan: kontras 4.5:1, fokus terlihat, status tidak hanya warna, target sentuh ≥ 32px.

## Penerapan

| Kapan | Siapa | Apa |
|---|---|---|
| Plan | `frontend-architect` | baca file ini; tiap `frontend-plan.md` punya bagian **Visual** (pertanyaan layar, pola yang dipakai, wajah/elemen khas, state, data contoh) |
| Implementasi | `frontend-developer` | pakai token dan pola di sini; jalankan `lint.mjs --app` pada file yang diubah |
| Setelah FE selesai | `workflows/implement-feature.js` | tahap **Design check**: lint, inspect (bila halaman bisa dibuka tanpa backend), perbaiki FAIL |
| Perubahan arah | manusia | ubah file ini, `app.css`, lalu catat di commit |

Perintah (dari root repo):

```bash
S=.agents/skills/design-taste-frontend
node $S/scripts/lint.mjs --app frontend/src            # statis, selalu bisa
node $S/scripts/inspect.mjs --app http://localhost:5173/<route>   # butuh dev server + halaman yang bisa dibuka
```

Belum dikerjakan: bagian **Visual** di plan T01–T06 yang sudah ditulis; kebab menu untuk aksi baris; pencarian global (dan top bar desktop bersamanya).
