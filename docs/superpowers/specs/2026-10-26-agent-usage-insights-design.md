# T19: insight pemakaian agent (dokumen, query kosong, per API key)

| | |
|---|---|
| Domain / Module | core / audit-log (menambah endpoint dan UI di Dashboard T15) |
| Ukuran | M |
| Bergantung pada | T02, T11, T15, T18 (aksi `mcp.read_section` ikut dihitung sebagai pembacaan) |

## Maksud dan alasan

Dashboard T15 menjawab "seberapa banyak" agent memakai MCP, tetapi belum menjawab "apa yang agent baca, apa yang tidak ia temukan, dan siapa (key mana) yang melakukannya". Tiga jawaban itu memberi tahu tim dokumen mana yang berharga, mana yang usang, dan di mana ada celah dokumentasi.

## Tujuan

Untuk satu rentang (24 jam, 7 hari, 30 hari) dan satu project (atau semua project bila belum dipilih, kecuali dua daftar yang butuh project):

1. **Dokumen paling sering dibaca:** path, jumlah pembacaan, jumlah key berbeda, pembacaan terakhir.
2. **Dokumen tak pernah dibaca:** file knowledge yang ada di project tetapi tidak dibaca dalam rentang, dengan `updated_at` file. Butuh project terpilih.
3. **Query hasil kosong:** query ternormalisasi, jumlah kejadian, jumlah key berbeda, kejadian terakhir.
4. **Per API key:** key (label), jumlah panggilan, error, pencarian, pembacaan, terakhir dipakai, tool teratas.

## Non-goals

- Tabel agregat baru, job latar, atau retensi baru (tetap 90 hari, rentang maksimal 30 hari).
- Rekomendasi otomatis ("hapus dokumen ini").
- Insight per pengguna manusia.
- Ekspor CSV.

## Anti-tujuan

- Isi file atau cuplikan bocor ke respons atau audit.
- Query terjemahan bebas dari pengguna dieksekusi sebagai SQL atau pola (selalu parameter).
- Halaman jadi lambat pada project besar (butuh batas baris dan indeks yang sudah ada dipakai).
- Endpoint dapat diakses non-super-admin.

## Keputusan pembangun (tebakan saya)

- **K1** Endpoint baru `GET /api/audit-log/usage?range=&project_id=&section=` (super_admin), terpisah dari `/summary` agar `/summary` tetap ringan. `section` opsional (`top_files|unread_files|zero_queries|keys`) supaya UI memuat bagian demi bagian; tanpa `section`, semua bagian dikembalikan. Tiap bagian gagal sendiri-sendiri (ada `available:false` per bagian) seperti `search_quality` di T15.
- **K2** Sumber data hanya `audit_log`: pembacaan = `mcp.read_file` dan `mcp.read_section` status `ok` dengan `target` = path; query kosong = `mcp.search_knowledge` status `ok` dengan `result.total = 0`, query dinormalisasi huruf kecil dan spasi tunggal (aturan sama dengan reformulasi T11); per key = `actor_type='api_key'` dikelompokkan `actor_id` dan `actor_label`.
- **K3** Daftar file untuk "tak pernah dibaca" diambil lewat bus `project.knowledge.listfiles` (module `core` tidak boleh membaca tabel project). Bila kontrak belum memuat `updated_at`, tambahkan field itu. Maksimal 200 file per respons, `truncated` bila lebih.
- **K4** Setiap daftar dibatasi 20 baris (kecuali tak-pernah-dibaca 50), diurut jumlah menurun lalu path/query menaik.
- **K5** UI: bagian "Wawasan agent" di bawah chart pada halaman Dashboard. Dua tab kecil (Dokumen, Pencarian) plus tabel per key. Daftar yang butuh project menampilkan ajakan memilih project bila belum dipilih. Chart tidak ditambah; empat tabel sudah cukup. Teks i18n id dan en.
- **K6** Tidak ada aksi audit baru (endpoint hanya baca, sama dengan `/summary`).

## Kontrak respons (ringkas)

```
{ range, from, to, project_id,
  top_files:      { available, items:[{path, reads, keys, last_read_at}] },
  unread_files:   { available, items:[{path, updated_at}], truncated },
  zero_queries:   { available, items:[{query, count, keys, last_at}] },
  per_key:        { available, items:[{key_id, label, calls, errors, searches, reads, last_used_at, top_tool}] } }
```

## Yang diserahkan ke pembangun

Bentuk SQL (CTE, indeks yang dipakai, `EXPLAIN` pada data besar), layout feature di `auditlog/usage`, komponen Svelte, dan tes.

## Kriteria penerimaan

- Tes repository dengan data audit sintetis: pembacaan ganda satu key dihitung 1 key; query berbeda huruf besar dihitung satu; file yang dibaca lewat `read_section` ikut dihitung.
- Unread files menampilkan file yang tidak ada di audit, bukan file yang sudah dihapus.
- Non-super-admin mendapat 403; project tak dikenal mendapat 404 seperti `/summary`.
- UI dites untuk keadaan loading, kosong, error per bagian, dan tanpa project.
- OpenAPI (`api/modules/core/audit-log.yml`) diperbarui dan `make api-bundle`, `make api-check`, `make lint`, `make test`, `make test-integration` lulus.
