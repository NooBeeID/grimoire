# T20: pencarian lebih toleran (sinonim, typo, variasi kata, urutan, cuplikan)

| | |
|---|---|
| Domain / Module | project / knowledge (pencarian) dan discovery / knowledge-tools (tool `search_knowledge`); module baru project / search-synonym untuk kamus sinonim |
| Ukuran | L |
| Bergantung pada | T05, T08, T11 (alat ukur), T18 (parser heading `mdoutline`) |
| Dikerjakan | terakhir dari tiga, karena paling berisiko |

## Maksud dan alasan

`search_knowledge` memakai full-text Postgres config `simple`: tanpa sinonim, tanpa stemming, tanpa toleransi typo. Baseline T11 (40 pasangan) menunjukkan recall@5 0.5875 dan MRR 0.6, dengan kind `synonym` 0.0625 dan `typo` 0. Agent yang bertanya dengan kata berbeda dari yang tertulis ("login" padahal dokumen memakai "autentikasi") pulang dengan hasil kosong atau meleset.

## Tujuan

Empat perbaikan, semuanya di Postgres tanpa layanan eksternal:

1. **Sinonim:** kamus sinonim per project yang dikelola manusia; query diperluas ke kelompok sinonim.
2. **Typo dan variasi kata:** toleransi salah ketik (trigram) dan stemming Indonesia dan Inggris (mengirim, dikirim, pengiriman; deploy, deployment, deploys).
3. **Urutan:** judul, path, dan heading berbobot lebih tinggi daripada isi; kecocokan frasa utuh dinaikkan.
4. **Cuplikan:** snippet diambil dari bagian (heading) yang paling cocok, dan hasil memuat nama heading itu supaya agent bisa lanjut `read_section` (T18).

## Non-goals

- Embedding, pgvector, layanan atau model eksternal.
- Pencarian lintas project.
- Mengubah kontrak argumen `search_knowledge` (hanya hasil bertambah field opsional `section`).
- Sinonim otomatis hasil pembelajaran.
- Terjemahan lintas bahasa.

## Anti-tujuan

- Operator tsquery atau pola trigram dari input pengguna dieksekusi mentah (tetap hanya term berkutip dan parameter).
- Recall naik tetapi presisi runtuh (hasil sampah di peringkat atas).
- Pengindeksan ulang memblokir tulisan, atau migrasi gagal pada database berisi data.
- Kamus sinonim bisa diubah oleh agent (tool tetap read-only; hanya UI dan API manusia).

## Keputusan pemilik

- Hanya Postgres, tanpa layanan eksternal. Semua empat masalah (sinonim, typo/variasi, urutan, cuplikan) harus diatasi.
- Kamus sinonim per project, dikelola manusia (tebakan yang sudah dikonfirmasi lewat rangkuman).

## Keputusan pembangun (tebakan saya, boleh dikoreksi)

- **K1** Ukuran keberhasilan memakai golden set T11. Target setelah selesai (di korpus fixture): recall@5 keseluruhan >= 0.80, kind `typo` recall@5 >= 0.50, kind `synonym` recall@5 >= 0.50 (dengan fixture kamus), MRR >= 0.70, tanpa penurunan kind `exact-name` dan `phrase` lebih dari toleransi. Baseline diperbarui memakai `make search-eval-baseline` dengan alasan di pesan commit. Bila target tidak tercapai, jangan turunkan target diam-diam: laporkan angkanya.
- **K2** Kamus sinonim: tabel `knowledge_synonym_groups(id, project_id, terms text[], created_at, updated_at, *_by_label)`, satu baris = satu kelompok setara (2 sampai 10 term, huruf kecil). API: list, create, update, delete di bawah `/api/projects/{id}/search/synonyms`, hanya anggota dengan hak ubah project. UI: tab baru "Pencarian" di workspace project (`workspaceTabs`) berisi daftar kelompok dan form tambah/ubah/hapus, i18n id dan en.
- **K3** Stemming: pembangun memeriksa config teks yang tersedia di `postgres:16` (apakah ada snowball `indonesian`). Bila tidak ada, query-side stemming ringan untuk imbuhan Indonesia (me-, di-, pe-, -kan, -an, -i, -nya dan sejenisnya) ditulis sebagai fungsi murni di `backend/lib/util` dan diterapkan pada term query dan token indeks (kolom generated tidak bisa memanggil Go, sehingga alternatifnya tabel term ternormalisasi atau stemming hanya sisi query dengan kecocokan prefix; pembangun memilih dan mencatat alasannya).
- **K4** Typo: ekstensi `pg_trgm` (ada di image `postgres:16`). Term query yang tidak punya kecocokan lexeme dicari kemiripan trigram terhadap kosakata project (judul, path, heading, term sering). Ambang kemiripan dan batas jumlah kandidat per term dicatat sebagai konstanta bernama. Pembuatan ekstensi di migrasi memakai `IF NOT EXISTS`; bila peran database tidak boleh membuat ekstensi, migrasi harus gagal dengan pesan jelas.
- **K5** Pembobotan: path/judul A, heading B, isi D (kini path A, judul B, isi D); kecocokan frasa utuh pada judul atau heading mendapat tambahan skor. Nilai bobot diselaraskan lewat golden set, bukan ditebak.
- **K6** Cuplikan: bagi isi menurut heading (parser `mdoutline` T18), nilai bagian yang paling cocok, ambil `ts_headline` dari bagian itu saja; tambahkan field hasil `section` (teks heading, kosong bila di luar heading). Format snippet lama (`**term**`) tetap.
- **K7** Pengindeksan ulang: migrasi forward-only dan idempoten; pembangunan ulang indeks pada tabel besar memakai `CREATE INDEX CONCURRENTLY` bila transaksi migrasi mengizinkan, atau dicatat sebagai risiko di tiket.
- **K8** Aksi audit tidak berubah (`mcp.search_knowledge`), tetapi `result.summary` boleh memuat `expanded` (true bila sinonim atau typo dipakai) supaya T19 dan T11 bisa melaporkannya. Entri lama tetap valid.

## Yang diserahkan ke pembangun

Rancangan SQL tepat (CTE, indeks GIN/GiST trigram), nama bus dan kontrak untuk kamus sinonim, layout module `project/search-synonym` dan feature, komponen Svelte, fixture kamus untuk golden set (`synonyms.yaml`), serta tes.

## Kriteria penerimaan

- Golden set memenuhi target K1 atau laporan angka jujur beserta penyebab.
- Tes unit untuk: perluasan sinonim (dua arah, satu kelompok), query berisi operator tsquery tetap aman, typo ringan ditemukan, query kosong dan sangat panjang tetap ditangani.
- Tes integrasi: tambah kelompok sinonim lewat API lalu `search_knowledge` menemukan dokumen lewat sinonim; hapus kelompok dan hasil kembali.
- UI dites: daftar kosong, tambah, ubah, hapus, validasi (term terlalu sedikit, duplikat), error API.
- OpenAPI dan katalog bus/tiket di `docs/tickets/README.md` diperbarui; `make lint`, `make test`, `make test-integration`, dan `make test-search-eval` lulus.
