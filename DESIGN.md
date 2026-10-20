# DESIGN.md — Grimoire (NooBee)

Sumber kebenaran visual untuk seluruh frontend. Plan frontend merujuk file ini; `frontend/src/app.css` adalah turunannya (ubah file ini dulu, lalu `app.css`).

Arah: **framed-sheet** dari tasteskill (`.claude/skills/design-taste-frontend`, `references/app-screens.md`), seed `2129925594`, kind `devtools`, terang dengan varian gelap. Sheet mentahnya ada di `.taste-skill/app/direction.json` (`node .agents/skills/design-taste-frontend/scripts/draw.mjs --show`).

**Kenapa framed-sheet.** Grimoire dipakai tim kecil dalam sesi kerja panjang: membaca dan mengedit Markdown, memindai tabel key, spec, dan audit. Itu butuh chrome yang diam dan permukaan baca yang putih. Warna hanya di data dan status, bukan di kerangka.

## Wajah produk (yang membuat ini bukan template)

1. **Mono untuk satu pekerjaan.** IBM Plex Mono khusus untuk hal yang dibaca mesin: path file knowledge, prefiks/ID API key, `operationId`, method + path endpoint, aksi audit (`project.apikey.create`), URL MCP. Jangan untuk judul atau label.
2. **Kerangka diberi rona dari produk.** `--color-ground` `#f9fbfc` (biru-abu sangat rendah) menjadi satu-satunya "warna" kerangka. Sheet tetap putih.
3. **Aksen teal `#1b6c80`** hanya di: logo, garis bawah tab aktif 2px, seri chart utama, dan satu state terpilih. Bukan di tombol, bukan di latar.
4. **Elemen khas: jejak agent.** Baris yang berasal dari agent (API key, entri audit `actor_type=api_key`) selalu memakai ikon agent + nama aksesibel, dan ID-nya mono. Manusia vs agent harus terbaca sekilas di tabel audit.

## Token

Semua token ada di `frontend/src/app.css` (`@theme` + override gelap). Nama lama T00 (`primary`, `danger`, `surface`, `fg`, `muted`, `border`, `rounded-base`) dipertahankan sebagai alias. Pakai nama baru di kode baru.

| Peran | Utility | Terang | Catatan |
|---|---|---|---|
| Kerangka (sidebar + top bar) | `bg-ground` | `#f9fbfc` | tanpa garis antara sidebar dan top bar |
| Sheet / kartu | `bg-surface` | `#ffffff` | sheet: 1px `border-border`, radius 16, **tanpa shadow** |
| Panel kedua (hover, code block) | `bg-raised` | `#f4f6f8` | |
| Item aktif netral | `bg-active` | `#ebedee` | pil abu, bukan warna |
| Garis | `border-border` / `border-rule` | `#e8eaec` / `#f0f1f2` | kartu tidak boleh berborder lebih tebal dari sheet |
| Kontrol | `border-control` | `#cdced1` | input, tombol sekunder |
| Teks / redup | `text-fg` / `text-muted` | `#131416` / `#5d5f60` | |
| Aksi utama | `bg-primary text-primary-fg` | `#131416` / putih | **satu per layar**, near-black |
| Aksen | `text-accent`, `border-accent` | `#1b6c80` | lihat "Wajah produk" #3 |
| Status | `text-positive` `text-danger` `text-warning` | `#1e7b4f` `#c4242a` `#92400e` | selalu disertai teks/ikon, tidak warna saja |
| Pil status | `bg-pill-{positive,negative,warning,neutral}-bg text-pill-…-ink` | lihat `app.css` | |
| Chart | `chart-1/2/3` | `#1b6c80` `#2f70bb` `#a86f0c` | chart-1 = data utama |

Radius: kontrol 6 (`rounded-base`), kartu 8 (`rounded-card`), sheet 16 (`rounded-sheet`). Tidak ada radius lain.
Ukuran: judul 22, seksi 16, KPI 28 (weight 400), badan 14, kecil 12; tidak ada teks di bawah 12px (min. 11px untuk label tabel). Bobot 400/500/600; 600 hanya untuk label dan tab aktif. Sentence case, tanpa huruf kapital berjarak.
Ritme: satu nilai gap (24) horizontal dan vertikal, padding sheet 24, tinggi kontrol 32 (tombol utama di title row 36), tinggi baris tabel 48.
Angka: `tabular-nums` global di `body`; angka rata kanan; satuan sebagai sufiks redup.
Font dimuat dari `@fontsource/*` (di-bundle Vite, `font-src 'self'` aman). Jangan menambah CDN font.

## Kerangka (shell)

`AppLayout` target: sidebar 228px + top bar 48px berbagi satu `bg-ground` (bentuk L), konten berupa **sheet putih** yang menempel ke sidebar/top bar dan berjarak 8px dari tepi kanan dan bawah jendela, radius 16, 1px border, tanpa shadow. Top bar memuat pencarian di tengah (bila ada), `LocaleSwitch`, dan tombol akun. Item sidebar aktif = pil `bg-active` atau teks `text-fg` + weight 500; tidak ada bar kiri berwarna, tidak ada pil berwarna. Di ponsel kerangka hilang: sheet jadi layar, top bar 48px (menu, judul, satu aksi), sidebar menjadi sheet menu.

> Status: `AppLayout.svelte` saat ini masih header atas dari T00. Migrasi ke kerangka ini dikerjakan di tiket UI berikutnya (lihat "Penerapan"). Jangan membangun layout paralel.

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
- Tanpa gradien, glow, glass, shadow saat diam (shadow hanya untuk overlay nyata: menu, dialog, toast), ikon dalam kotak/lingkaran berwarna, sparkline di KPI, emoji, chart pelangi, huruf kapital berjarak, zebra, header tabel berlatar.
- Tabel: header 12–13px abu tanpa fill dan tanpa rule, baris 48px, rule 1px `border-rule`, kolom identitas paling lebar, angka rata kanan, satu penanda status per baris, satu kebab di kanan. Kolom yang kosong di semua baris dibuang.
- Tidak ada teks sapaan ("Selamat datang kembali 👋"), tidak ada hero di layar kerja; judul 22px.
- Nol ditulis `0`, nilai hilang ditulis `–` (abu). Empty state tetap menjaga bingkai bagian itu dan memberi aksi yang relevan.
- Data contoh (mock FE slicing, fixture test visual) memakai kata dan angka produk ini: nama project nyata-masuk-akal, path knowledge seperti `docs/auth/login-flow.md`, prefiks key seperti `nbk_4f9a…`. Dilarang Acme, John Doe, lorem, "+12%" pada akun baru. Angka yang tampil dua kali adalah satu variabel; total dihitung dari baris.
- Gerak hanya sebagai umpan balik (tekan, hover, fokus, dialog/menu buka). Tidak ada animasi masuk. Hormati `prefers-reduced-motion`.
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

Pekerjaan yang belum dikerjakan oleh dokumen ini: migrasi `AppLayout` ke kerangka framed-sheet; restyle `Button`, `Table`, `Dialog`, `Input`, `Toast` (tanpa `shadow-lg` saat diam) ke token baru; bagian **Visual** di plan T01–T06 yang sudah ditulis.
