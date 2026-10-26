# DESIGN.md — Grimoire (NooBee)

Sumber kebenaran visual frontend. `frontend/src/app.css` adalah turunannya (ubah file ini dulu, lalu `app.css`).

Arah: **developer portal yang bersih** (rujukan: Stripe Docs, Linear). Menggantikan arah "soft block" bergaya cartoon (outline tebal, bayangan keras, maskot Bibi) atas permintaan pengguna. Terang dan gelap lewat toggle (`data-theme`, `grimoire.theme`). Mode: Operate (alat kerja), dengan Read untuk panel Markdown.

## Wajah produk

1. **Kaca di atas latar ambient.** Sidebar dan panel kerja adalah panel `.glass` mengambang (blur 24px, tepi putih tipis) di atas latar lembut bercahaya biru, madu, dan lilac. Tabel dan kartu isi tetap putih hampir opak supaya data terbaca.
2. **Satu aksen tenang.** Aksi utama = tombol ink (hampir hitam; terbalik di gelap), satu per layar. Biru (`accent-ink`) hanya untuk tautan dan fokus. Madu (`brand`) hanya pada tanda logo dan sorotan hasil pencarian.
3. **Tipografi.** Inter Variable untuk semua teks; IBM Plex Mono hanya untuk kode, path, dan data yang dibaca mesin. Isi 14px, judul layar 24px bobot 600, tracking -0.02em. Header tabel 12px bobot 500 warna redup.
4. **Tab sebagai segmented pill** dalam wadah kaca; tab aktif = pil ink. Item nav aktif = pil madu (`bg-brand`, teks ink).
5. **Tabel ringan.** Header `raised` dengan garis bawah tipis, baris dipisah `rule`, tinggi 48-56px, tanpa zebra.
6. **Agent vs manusia** tetap terbaca sekilas: agent = kotak ink berglyph bot, manusia = ikon orang di tile netral.
7. **Gelap.** Ground `#0b0c0f`, surface `#101216`, garis `#262a32`, aksi utama terang.

## Token

Semua di `app.css`. Nama lama (`primary`, `danger`, `surface`, `fg`, `muted`, `border`) tetap sebagai alias.

| Peran | Utility | Terang | Gelap |
|---|---|---|---|
| Kerangka (sidebar) | `bg-ground` | `#f7f8fa` | `#0b0c0f` |
| Permukaan | `bg-surface` | `#ffffff` | `#101216` |
| Teks / aksi utama | `text-fg` `bg-primary` | `#0f1216` | `#ecedf0` |
| Garis | `.outlined` `border-outline` | `#e1e4ea` | `#2a2e37` |
| Isian netral (hover/aktif) | `bg-raised` `bg-active` `bg-accent` | `#f3f4f7` `#eceef3` | `#171a20` `#1d2128` |
| Tautan, fokus | `text-accent-ink` | `#3358d4` | `#8aa6ff` |
| Teks redup | `text-muted` | `#5b6270` | `#9aa1ad` |
| Madu (logo, sorotan) | `bg-brand` | `#ffcf26` | `#ffcf26` |
| Status | `text-positive` `text-danger` `text-warning` | `#0b7a49` `#c4283f` `#8a4b00` | `#5fe0a6` `#ff8a98` `#ffc266` |
| Bayangan | `shadow-pop` hanya untuk menu/dialog/toast | lembut | lembut + hairline |

Bentuk fully rounded: tombol, input, select, tab, nav, dan pil memakai `rounded-full`; radius 14 / 22 / 30 (`rounded-base` / `-card` / `-sheet`) untuk textarea, kartu, dan panel.

## Aturan

- Aksi utama = tombol ink, satu per layar. Biru hanya untuk tautan dan fokus.
- Gradien hanya pada latar ambient (`--ambient`), tidak pernah pada tombol atau teks. Glass hanya untuk kerangka, menu, dan dialog, bukan di dalam data.
- Fokus: outline 2px `accent-ink` offset 2px. Status tidak hanya warna, target sentuh >= 32px, kontras >= 4,5:1.
- Gerak: transisi warna 120ms saja; mati di `prefers-reduced-motion`.
- Tanpa maskot dan dekorasi. Karakter ada di tipografi dan ketelitian detail.

## Yang belum dikerjakan

Pola per layar untuk Knowledge (pohon file, panel Markdown), filter bar audit (checkbox/radio kustom), chart, dan direction contract di surface brief.
