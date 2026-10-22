# T15 — core / audit-log (dashboard metrik penggunaan MCP + baris tabel bisa diklik)

| | |
|---|---|
| Domain / Module | core / audit-log (membaca hasil T11 lewat bus) |
| Ukuran | L |
| Bergantung pada | T02, T07, T11 |
| Sumber | permintaan pemilik (ingin melihat pemakaian MCP tanpa membaca log satu per satu) |

## Ringkasan

Halaman Audit Log (T02) hanya berupa tabel entri. Untuk menjawab "project mana yang paling sering diakses agent" atau "tool MCP mana yang paling sering dipakai dan paling sering gagal", Super Admin harus menghitung sendiri dari tabel. Padahal semua datanya sudah ada di `audit_log` (`action = mcp.<tool>`, `status`, `project_id`, `result.duration_ms`).

Tiket ini menambah:

1. **Halaman "Dashboard"** (`/dashboard`, khusus Super Admin): kumpulan chart dari data `audit_log` pada rentang waktu dan project yang dipilih. Halaman ini berdiri sendiri, dengan item navigasi sendiri di sidebar, dan bukan bagian dari halaman Audit Log.
2. **Perbaikan kecil di tabel log:** seluruh baris bisa diklik untuk membuka detail. Saat ini hanya teks waktu yang bisa diklik (`AuditTable.svelte`), sehingga pengguna mengira baris tidak interaktif.

## Tujuan

- Super Admin melihat, untuk rentang 24 jam / 7 hari / 30 hari dan (opsional) satu project:
  - **Project paling sering diakses** (bar chart horizontal, 10 teratas, berdasar jumlah panggilan tool MCP).
  - **Tool MCP paling sering dipakai** (bar chart, jumlah panggilan per tool).
  - **Metrik per tool** (hanya chart): status ok/error per tool (stacked bar) dan latensi p50/p95 per tool (grouped bar).
  - **Trafik** panggilan MCP per waktu (line chart; per jam untuk 24 jam, per hari untuk 7/30 hari), dengan garis kedua untuk error.
  - **Kualitas pencarian** (hasil T11) untuk project terpilih: zero-result rate, follow-up rate, reformulation rate, dan distribusi rank yang dibuka.
- Klik di mana pun pada baris tabel log membuka detail entri.

## Non-goals

- Mengubah cara audit dicatat, skema `audit_log`, retensi (90 hari), atau sifat append-only.
- Dashboard real-time/streaming. Data diambil saat halaman Dashboard dibuka atau filter diubah (ada tombol muat ulang).
- Rentang tanggal bebas, ekspor CSV/gambar, alert/notifikasi ambang.
- Ringkasan untuk aksi non-MCP (login, project dibuat, key dicabut, dan sebagainya). Hanya panggilan tool MCP.
- Tampilan untuk non-Super Admin. Dashboard dan audit log tetap khusus Super Admin.
- Mengubah halaman Audit Log selain klik baris (K9). Audit Log tidak mendapat tab, title row baru, atau tautan ke Dashboard.
- Mengubah `/` (daftar project, `HomeView`) untuk siapa pun, termasuk Super Admin.
- Menghitung ulang atau mengubah definisi metrik T11. Tiket ini hanya menampilkan.

## Anti-tujuan (dianggap gagal walau fitur jalan)

- Query ringkasan membuat halaman atau database lambat secara berarti saat tabel besar (lihat Risiko dan K3).
- Isi `args`/`result` mentah, query pencarian, atau path file bocor ke respons ringkasan. Ringkasan hanya berisi angka agregat dan label (nama tool, nama project).
- Bundle halaman lain membengkak karena Chart.js ikut termuat di luar `/dashboard`.
- Item navigasi Dashboard terlihat oleh non-Super Admin, `/dashboard` bisa dibuka non-Super Admin, atau sesi non-Super Admin memicu request ke `/api/audit-log/summary`.
- Kegagalan mengambil kualitas pencarian (bus T11) menggagalkan seluruh ringkasan.
- Chart hanya bisa dipahami lewat warna (tidak terbaca pengguna buta warna / pembaca layar).
- Perubahan klik baris merusak navigasi keyboard atau fokus kembali ke baris saat detail ditutup (perilaku T02).

## Keputusan pemilik

| Topik | Keputusan |
|---|---|
| Penempatan | Halaman **Dashboard** sendiri, hanya Super Admin yang bisa melihatnya. Mengganti keputusan awal (tab "Ringkasan" di halaman Audit Log), yang dibatalkan. |
| Library chart | Chart.js. |
| Rentang waktu | Preset 24 jam / 7 hari / 30 hari, default 7 hari, ditambah filter project. |
| Metrik tambahan | Error rate + latensi per tool, trafik per jam/hari, kualitas pencarian (T11). Key paling aktif dan auth ditolak **tidak** ikut. |

## Keputusan pembangun (belum disetujui pemilik; mohon dicek)

- K1. **Definisi "tool MCP":** entri dengan `action` berawalan `mcp.` **kecuali** aksi protokol/gateway: `mcp.initialize`, `mcp.tools_list`, `mcp.auth_rejected`, `mcp.rate_limited`, `mcp.unknown_tool`. Daftar pengecualian diambil dari konstanta `record.ActionMCP*` (satu sumber), bukan string tersebar, dan dipastikan lengkap saat plan.
- K2. **Definisi metrik:** `calls` = jumlah entri tool; `error_rate` = `status = 'error'` / `calls`; latensi p50/p95 = `percentile_cont` atas `(result->>'duration_ms')::numeric` pada entri yang punya angka itu. Tool tanpa data latensi menampilkan kosong, bukan 0.
- K3. **Query langsung di SQL** (`GROUP BY` di repository feature baru), bukan memuat entri ke memori. Memakai indeks yang ada (`audit_log_action_idx`, `audit_log_project_idx`, `audit_log_occurred_idx`); tidak ada migrasi indeks baru di tiket ini. Bila pengukuran di data besar menunjukkan lambat, tambah indeks parsial pada tiket lanjutan. Ada batas keras rentang 30 hari dan `LIMIT` 10 untuk top project.
- K4. **Satu endpoint** `GET /api/audit-log/summary` mengembalikan semua agregat sekaligus (satu panggilan FE, satu snapshot waktu yang konsisten). Parameter: `range` (`24h|7d|30d`, default `7d`), `project_id` (opsional).
- K5. **Kualitas pencarian lewat bus.** Usecase ringkasan memanggil `discovery.knowledgetools.search-quality-report` (kontrak T11) hanya bila `project_id` diisi, karena kontrak itu per project. Tanpa project, bagian ini tidak dikembalikan dan UI menampilkan ajakan memilih project. Bila bus gagal, `search_quality` dikembalikan `null` dengan penanda error; sisa ringkasan tetap terkirim. Audit-log menjadi consumer baru kontrak T11 (catat di README, tabel kontrak bus).
- K6. **Bucket trafik:** jam (UTC, ditampilkan di zona waktu browser) untuk `24h`, hari untuk `7d` dan `30d`. Bucket kosong tetap dikirim dengan nilai 0 agar garis tidak melompat. Dibuat di SQL dengan `generate_series`.
- K7. **Chart.js** dipasang versi pasti, hanya komponen yang dipakai (`BarController`, `LineController`, `BarElement`, `LineElement`, `PointElement`, `CategoryScale`, `LinearScale`, `Tooltip`, `Legend`) agar tree-shaking efektif. Feature `summary` (dan Chart.js bersamanya) di-import dinamis (`import()`) oleh `DashboardView`, jadi halaman lain tidak membayar ukuran Chart.js. Warna chart mengikuti token tema (terang/gelap) dan tidak hanya membedakan seri lewat warna (label langsung, pola, atau legenda + tooltip).
- K8. **Aksesibilitas chart:** tiap chart punya `aria-label` ringkas dan tabel data tersembunyi visual (`sr-only`) berisi angka yang sama. Ini bukan tabel yang tampil di UI, sehingga tetap sesuai permintaan "chart saja" untuk metrik per tool.
- K9. **Klik baris:** `<tr>` mendapat `onclick` dan `cursor-pointer`, tombol waktu tetap ada sebagai target fokus keyboard (`data-entry-id`, dipakai untuk mengembalikan fokus). Tidak menambah `role="button"` pada `<tr>` agar semantik tabel tidak rusak. Klik pada teks yang sedang diseleksi (drag select) tidak membuka detail.
- K10. **State di URL:** `/dashboard?range=24h|7d|30d&project=<uuid>`, sehingga filter bisa dibagikan dan bertahan saat muat ulang. Tidak ada kunci `tab`. Karena Dashboard adalah route sendiri, kuncinya tidak berbagi ruang dengan filter Audit Log (`range`/`project_id` milik `/audit-log` tidak dibaca atau ditulis Dashboard).
- K11. **Route dan navigasi:** route baru `/dashboard` dengan meta `{ auth: true, roles: ['super_admin'], layout: 'app', titleKey: 'dashboard.title' }` di `frontend/src/app/routes/index.ts`, pola sama dengan `/audit-log` dan `/admin/users`. Non-Super Admin mendapat perlakuan guard yang sama dengan `/audit-log` (`guard.ts`: role tidak cocok menghasilkan `notfound`). Satu item navigasi "Dashboard" ditambahkan di `frontend/src/app/layouts/AppLayout.svelte` di dalam blok `role === 'super_admin'` yang sudah memuat Users dan Audit Log, sebagai item pertama blok itu (urutan: Project, Dashboard, Users, Audit Log), dengan ikon outline 16px baru `chart` di cabang ikon yang ada. `/` tetap daftar project (`HomeView`) untuk semua user. View aplikasi `frontend/src/app/views/DashboardView.svelte` (baru) merender `PageHeader` "Dashboard" lalu `SummaryView` dari feature `summary`.
- K12. **Endpoint tetap di module audit-log.** Backend tidak berubah karena perpindahan ke Dashboard: endpoint tetap `GET /api/audit-log/summary` (feature `summary` di `core/internal/auditlog`, khusus Super Admin lewat `RequireRole` grup `/audit-log`). Alasannya, datanya milik module audit-log; halaman Dashboard hanya konsumen. Struktur FE tetap `frontend/src/domain/core/audit-log/summary/`. Alternatif `/api/dashboard/...` dicatat di Pertanyaan terbuka.

## Constraints

- Struktur domain/module/feature yang ada. `summary` adalah feature baru di `core/internal/auditlog`. Interaksi dengan discovery hanya lewat `infra/bus`; tidak ada import package module lain (aturan T00).
- Hanya `SELECT` pada `audit_log`.
- Endpoint khusus `super_admin` lewat grup `/audit-log` yang sudah memasang `middleware.RequireRole`.
- Spesifikasi API: tambah path di `api/modules/core/audit-log.yml`, lalu `make api-bundle` dan commit `api/openapi.yml`.
- Teks UI bahasa Indonesia; kunci i18n di `frontend/src/shared/i18n/{id,en}.json`.
- Frontend mengikuti pola domain: `frontend/src/domain/core/audit-log/summary/{services,stores,components,views,mocks}`; respons divalidasi Zod di service; slicing dengan mock dulu, lalu wiring ke API nyata. Halaman dirakit di `app/` (`app/views/DashboardView.svelte`), yang hanya meng-import `summary/index.ts`.
- Chart.js satu-satunya dependensi baru, versi dikunci.

## Features

| Folder | Perubahan | Titik masuk | Siapa |
|---|---|---|---|
| `summary` (baru, backend) | Agregat top project, pemakaian tool, metrik per tool, trafik; memanggil bus T11 bila `project_id` ada | `GET /api/audit-log/summary` | super_admin |
| `summary` (baru, frontend) | Isi halaman Dashboard: filter rentang + project, kartu kosong/error per chart, 5 chart | `/dashboard` (lewat `DashboardView`) | super_admin |
| `app` (ubah, frontend) | Route `/dashboard`, `DashboardView` (baru), item nav "Dashboard" khusus super_admin | `/dashboard`, sidebar | super_admin |
| `listentries` (ubah, frontend) | Baris tabel bisa diklik penuh | `/audit-log` | super_admin |

## Kontrak respons (ringkas)

```
GET /api/audit-log/summary?range=7d&project_id=<uuid?>

{
  "range": "7d", "from": "...", "to": "...", "bucket": "day",
  "total_calls": 1234,
  "top_projects": [ { "project_id": "...", "project_label": "...", "calls": 420 } ],
  "tool_usage":   [ { "tool": "read_file", "calls": 600 } ],
  "tool_metrics": [ { "tool": "read_file", "calls": 600, "ok": 590, "error": 10,
                      "error_rate": 0.0167, "p50_ms": 8, "p95_ms": 40 } ],
  "traffic":      [ { "at": "...", "calls": 90, "errors": 2 } ],
  "search_quality": {            // null bila project_id kosong atau bus gagal
    "available": true,
    "zero_result": { "count": 4, "denominator": 60, "rate": 0.067 },
    "follow_up": { ... }, "reformulation": { ... },
    "rank_opened": [ { "label": "1", "count": 30 }, ... ],
    "legacy_searches": 0
  }
}
```

`tool` berisi nama tool tanpa awalan `mcp.`. Bila project terisi, `top_projects` berisi tepat satu elemen (project itu) dan UI menyembunyikan chart tersebut.

## Aturan bisnis

- Rentang `24h|7d|30d` dihitung mundur dari saat permintaan; `[from, to)`. Nilai lain ditolak `400`.
- `project_id` tidak valid (bukan uuid) → `400`. Project yang tidak ada atau belum punya aktivitas → ringkasan kosong (semua angka 0), bukan `404`.
- Entri berstatus `error` tetap dihitung sebagai panggilan; `denied` tidak muncul pada aksi tool (itu aksi gateway, dikecualikan K1).
- Project yang sudah dihapus tetap muncul memakai `project_label` yang tersimpan di entri audit, dengan penanda "dihapus" bila project tidak ada lagi (cukup label apa adanya bila penanda sulit; tidak boleh error).
- Tidak ada aksi audit baru: pembacaan ringkasan tidak dicatat (sama seperti `listentries`/`getentry`).

## Model data

Tidak ada tabel, kolom, atau migrasi baru. Pembacaan `audit_log` saja.

## Kontrak bus

Tidak ada endpoint bus baru. Consumer baru untuk `discovery.knowledgetools.search-quality-report` (provider T11), dengan `Request{ProjectID, Since: from, Until: to}`.

## UI (Svelte)

- Sidebar (`AppLayout`): item "Dashboard" (ikon `chart`) hanya untuk Super Admin, sebelum Users dan Audit Log; aktif (`aria-current="page"`) di `/dashboard`.
- `/dashboard` (`DashboardView`): title row `PageHeader` "Dashboard" + satu kalimat redup tentang data, lalu isi feature `summary` yang di-lazy-load (Spinner selama chunk dimuat; ErrorState + coba lagi bila chunk gagal).
- Isi halaman Dashboard:
  - Bar filter: pemilih rentang (3 preset) dan pemilih project (opsional, "Semua project"), tombol muat ulang.
  - Kartu angka kecil: total panggilan, error rate keseluruhan.
  - Chart: Project teratas (bar horizontal), Tool paling sering dipakai (bar), Status per tool (stacked bar ok/error), Latensi per tool (grouped bar p50/p95), Trafik (line, dua seri).
  - Blok Kualitas pencarian: tiga angka + bar distribusi rank. Tanpa project: kartu ajakan "Pilih project untuk melihat kualitas pencarian". Bila `available=false` karena error: pesan singkat dengan tombol coba lagi, tanpa memengaruhi chart lain.
  - Setiap chart punya status sendiri: memuat (skeleton), kosong ("Belum ada panggilan MCP pada rentang ini"), error.
  - Responsif sampai lebar ponsel: chart satu kolom, label sumbu dipotong dengan `title`/tooltip untuk nama panjang.
- Halaman Audit Log: tidak berubah kecuali `AuditTable` baris penuh bisa diklik (K9).

## Titik sentuh

Backend: `backend/internal/domain/core/internal/auditlog/summary/` (baru), `auditlog/base.go` (wiring route + consumer bus), `lib/contracts/core/audit/record` (hanya dibaca untuk konstanta aksi), `api/modules/core/audit-log.yml`, `api/openapi.yml`.
Frontend: `frontend/src/app/routes/index.ts` (route `/dashboard`), `frontend/src/app/views/DashboardView.svelte` (baru), `frontend/src/app/layouts/AppLayout.svelte` (item nav), `frontend/src/domain/core/audit-log/summary/` (baru), `frontend/src/domain/core/audit-log/listentries/components/AuditTable.svelte`, `frontend/package.json`, `frontend/src/shared/i18n/{id,en}.json`. `frontend/src/app/views/AuditLogView.svelte` tidak diubah.
Lainnya: `docs/mcp-gateway.md` (bagian Audit, satu paragraf rujukan ke halaman `/dashboard`), `docs/search-quality.md` (catatan: metrik juga tampil di halaman Dashboard), `docs/tickets/README.md`.

## Aksi audit baru

Tidak ada.

## Catatan revisi

- T02: halaman Audit Log **tidak** mendapat tab. Satu-satunya perubahan pada layar T02 adalah klik baris penuh (K9). Daftar fitur T02 tetap; `summary` ditambahkan oleh tiket ini dan tampil di halaman Dashboard terpisah.
- T11: pertanyaan terbuka nomor 3 ("tampilkan di UI Super Admin atau cukup CLI?") dijawab sebagian: tampil di halaman Dashboard Super Admin (`/dashboard`, untuk project terpilih), CLI tetap ada.
- T15 sendiri: rancangan awal "tab Ringkasan di halaman Audit Log" dibatalkan oleh keputusan pemilik dan diganti halaman Dashboard. Backend tidak terpengaruh (K12).

## Risiko

- **Biaya query pada tabel besar.** Ringkasan 30 hari meng-agregasi seluruh entri `mcp.*` pada rentang itu. Mitigasi awal: rentang maksimal 30 hari, filter `action` + `occurred_at` memakai indeks yang ada, satu round-trip agregat. Perlu diukur dengan data uji besar (mis. 1 juta entri) sebelum selesai; hasilnya dicatat di PR. Bila terlalu lambat: indeks parsial atau tabel agregat harian (tiket lanjutan).
- **Latensi dari JSON.** `result->>'duration_ms'` tidak terindeks dan entri lama bisa tidak punya angka itu; percentile dihitung hanya pada entri yang punya nilai.
- **Kualitas pencarian dibatasi project.** Kontrak T11 per project, jadi tanpa filter project bagian ini tidak bisa menampilkan angka global. Disengaja (K5).
- **Interpretasi.** Metrik follow-up/reformulasi adalah perkiraan tren (lihat Risiko T11). Dicatat di tooltip atau teks bantuan, bukan klaim mutlak.
- **Chart.js menambah dependensi** dan permukaan pembaruan keamanan. Dikunci versinya dan dimuat hanya di `/dashboard`.
- **Zona waktu.** Bucket dihitung UTC di server; batas hari di UI bisa berbeda dari hari lokal pengguna. Ditampilkan apa adanya dengan label zona waktu pada sumbu.
- **Nama "Dashboard" vs isi.** Halaman bernama umum tetapi saat ini hanya berisi metrik MCP dari audit log. Bila nanti menampung metrik non-audit, penempatan endpoint perlu ditinjau (Pertanyaan terbuka 5).

## Kriteria penerimaan

1. Super Admin melihat item navigasi "Dashboard" di sidebar dan membuka `/dashboard`: terlihat title row "Dashboard", lima chart, dan blok kualitas pencarian untuk rentang default 7 hari. Non-Super Admin tidak melihat item navigasi itu; membuka `/dashboard` langsung diperlakukan sama seperti `/audit-log` (guard router yang sama, halaman tidak tersedia), dan tidak ada request ke `/api/audit-log/summary`.
2. Data uji: project A 5 panggilan `mcp.read_file`, project B 2 panggilan `mcp.search_knowledge` → `top_projects` berurutan A lalu B; `tool_usage` read_file 5 dan search_knowledge 2.
3. Entri `mcp.initialize`, `mcp.tools_list`, `mcp.auth_rejected`, `mcp.rate_limited`, `mcp.unknown_tool` tidak ikut `total_calls`, `tool_usage`, maupun `tool_metrics`.
4. 10 panggilan `read_file` (1 berstatus `error`) → `error_rate` 0.1. Latensi p50/p95 benar terhadap `duration_ms` data uji; entri tanpa `duration_ms` tidak ikut percentile dan tidak membuat error.
5. Rentang `24h` menghasilkan bucket per jam, `7d` dan `30d` per hari; bucket tanpa panggilan bernilai 0 dan jumlah bucket sesuai rentang.
6. Entri di luar `[from, to)` tidak dihitung. Dengan `project_id`, hanya entri project itu yang dihitung.
7. `range` tidak valid atau `project_id` bukan uuid → `400`. Project tanpa aktivitas → `200` dengan semua angka 0 dan UI menampilkan keadaan kosong per chart.
8. Dengan `project_id`, `search_quality` terisi sama dengan keluaran `grimoire search-quality report` untuk project dan rentang yang sama. Tanpa `project_id`, `search_quality` null dan UI menampilkan ajakan pilih project.
9. Bus T11 gagal (disimulasikan): respons tetap `200`, chart lain tetap tampil, blok kualitas pencarian menampilkan pesan error dengan tombol coba lagi.
10. Respons tidak memuat `args`, isi `result`, query pencarian, atau path file (tes memeriksa bentuk respons).
11. Mengganti rentang atau project memperbarui URL `/dashboard` (`range`, `project`), dan memuat ulang halaman mengembalikan keadaan yang sama.
12. Tiap chart punya `aria-label` dan data setara untuk pembaca layar; seri tidak dibedakan hanya dengan warna; tampilan terbaca di tema terang dan gelap.
13. Membuka halaman selain `/dashboard` (mis. `/`, `/audit-log`) tidak mengunduh Chart.js (diverifikasi lewat ukuran chunk atau jaringan); Chart.js hanya dimuat saat `/dashboard` dibuka.
14. Di halaman Audit Log, klik di mana pun pada baris (bukan hanya teks waktu) membuka panel detail; menutup detail mengembalikan fokus ke tombol waktu baris itu; navigasi keyboard (Tab ke tombol waktu, Enter) tetap berfungsi; menyeleksi teks target dengan drag tidak membuka detail.
15. Tes: unit untuk perhitungan agregat (repository dengan Postgres tes, bukan mock), tes handler/otorisasi, tes service Zod dan store FE, tes komponen chart (data masuk → konfigurasi Chart.js benar), tes `AuditTable` untuk klik baris, tes item nav dan guard `/dashboard`, dan e2e Playwright: buka `/dashboard`, ganti rentang, pilih project.
16. `make api-bundle` bersih; `api/openapi.yml` ter-commit memuat `/audit-log/summary`.

## Pertanyaan terbuka

1. Perlu auto-refresh berkala (mis. tiap 60 detik) atau cukup tombol muat ulang? Saat ini hanya tombol.
2. Perlu bandingkan dengan periode sebelumnya (mis. "naik 12% dari 7 hari lalu") pada kartu angka? Ditunda.
3. Bila data audit besar membuat query lambat, apakah kita mau tabel agregat harian (tiket lanjutan) atau cukup membatasi ke 7 hari?
4. Apakah "key paling aktif" dan "auth ditolak/rate limit" dijadikan tiket lanjutan? Sengaja tidak masuk T15 sesuai keputusan pemilik, tetapi datanya sudah ada.
5. Endpoint tetap `GET /api/audit-log/summary` (K12). Bila nanti Dashboard menampung metrik non-audit (mis. jumlah file knowledge, spec, key aktif), apakah kita pindah ke namespace `/api/dashboard/...` (module sendiri yang mengagregasi lewat bus) atau tiap module tetap menyediakan endpoint ringkasannya sendiri?
6. Posisi item "Dashboard" di sidebar (K11: pertama di blok Super Admin) dan apakah Super Admin perlu mendarat di `/dashboard` setelah login. Saat ini `/` tetap daftar project untuk semua user.
