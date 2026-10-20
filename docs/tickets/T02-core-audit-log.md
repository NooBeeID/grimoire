# T02 — core / audit-log

| | |
|---|---|
| Domain / Module | core / audit-log |
| Ukuran | M |
| Bergantung pada | T00 |
| Dibutuhkan oleh | hampir semua tiket (penulis log), T07 (kritis) |

## Ringkasan

Satu audit log untuk aktivitas agent dan manusia. Kebutuhan utamanya: **bisa mengaudit apa saja yang dilakukan agent**. Hanya Super Admin yang melihat; retensi 90 hari; append-only dari sisi aplikasi.

## Tujuan

- Setiap panggilan tool MCP tercatat lengkap (siapa/key mana, project mana, tool apa, argumen, hasil ringkas, kapan, dari IP mana).
- Aksi manusia yang sensitif tercatat di tabel yang sama.
- Super Admin bisa menelusuri dan memfilter.

## Non-goals

Menyimpan isi file yang dibaca agent atau isi respons lengkap; ekspor CSV/SIEM; alert real-time; akses untuk developer; tamper-proofing kriptografis.

## User story

- Sebagai Super Admin, saya melihat file apa saja yang dibaca agent "claude-code-budi" di project X minggu ini.
- Sebagai Super Admin, saya melihat percobaan akses dengan key yang salah atau sudah dicabut.
- Sebagai Super Admin, saya melihat siapa yang menghapus sebuah project atau mencabut sebuah key.

## Features

| Folder | Deskripsi | Titik masuk | Siapa |
|---|---|---|---|
| `record` | Menyimpan satu entri; memotong/membersihkan argumen | bus `core.audit.record` (Call dan Publish) | internal |
| `listentries` | Daftar entri + filter + paginasi cursor | `GET /api/audit-log` | super_admin |
| `getentry` | Detail satu entri (argumen & ringkasan hasil penuh) | `GET /api/audit-log/{id}` | super_admin |
| `purgeexpired` | Hapus entri lebih tua dari retensi | job harian | sistem |

Tidak ada endpoint ubah/hapus entri. Tidak boleh ada.

## Isi entri

```
audit_log(id bigserial/uuid pk, occurred_at timestamptz,
  actor_type text  -- user | api_key | anonymous | system
  actor_id uuid null, actor_label text,        -- snapshot nama/email atau nama+prefix key
  project_id uuid null, project_label text null,-- snapshot nama project
  action text,                                  -- lihat katalog di README
  target_type text null, target text null,      -- mis. 'file', 'docs/auth/login.md'
  args jsonb,                                   -- argumen yang sudah dibersihkan
  status text,                                  -- ok | error | denied
  result jsonb,                                 -- ringkasan: bytes, jumlah hasil, kode error
  ip inet null, user_agent text null, trace_id text)
```

Snapshot label wajib: entri harus tetap bermakna setelah user/key/project dihapus. **Tidak ada foreign key** ke tabel module lain.

Indeks: `(occurred_at desc)`, `(project_id, occurred_at desc)`, `(actor_type, actor_id, occurred_at desc)`, `(action, occurred_at desc)`.

## Aturan bisnis

- **Pembersihan argumen:** kunci bernama `password`, `token`, `authorization`, `api_key`, `secret` dibuang/ditutup; nilai string dipotong (default 2 KB); total `args` dibatasi (mis. 8 KB).
- **Isi file tidak disimpan**, hanya metadata (path, ukuran, jumlah hasil).
- **Retensi:** `AUDIT_RETENTION_DAYS` (default 90). Job harian menghapus entri yang lebih tua, dalam batch kecil agar tidak mengunci tabel.
- **Append-only:** tidak ada kode yang melakukan UPDATE/DELETE selain `purgeexpired`. Disarankan role DB aplikasi tanpa hak UPDATE pada tabel ini bila memungkinkan (didokumentasikan di panduan deploy).
- **Mode tulis:** panggilan agent (`mcp.*`) dan aksi keamanan memakai `Call` (sinkron) sehingga tidak hilang saat proses mati; aksi lain boleh `Publish`. Bila `Call` gagal, perilaku mengikuti pertanyaan terbuka #1.
- Penulisan harus murah: satu INSERT, tanpa join.

## Kontrak bus

Disediakan (`lib/contracts/core/audit/record`): `core.audit.record` — request berisi seluruh kolom di atas (kecuali id/waktu bila kosong → diisi server). Response: `{Recorded bool}`.

Tidak mengonsumsi endpoint lain.

## UI (Svelte, `domain/core/audit-log/...`)

Menu "Audit Log" khusus Super Admin:
- Tabel: waktu, aktor (ikon agent vs manusia), project, aksi, target, status.
- Filter: jenis aktor, aktor tertentu (autocomplete), project, aksi (multi), status, rentang waktu (preset: 1 jam, 24 jam, 7 hari, 30 hari, 90 hari), teks pada target.
- Detail di panel samping: argumen, ringkasan hasil, IP, user agent, trace id.
- Paginasi cursor; tautan dalam dari halaman API key ke log yang sudah terfilter pada key itu (opsional, nilai tambah).

## Kriteria penerimaan

1. Satu panggilan `read_file` lewat MCP menghasilkan satu entri dengan aktor `api_key`, nama key, project, path, ukuran byte, status ok, IP.
2. Panggilan dengan key dicabut menghasilkan entri `mcp.auth.rejected`, status `denied`, aktor `anonymous` dengan prefix key bila ada.
3. Setelah project dan key dihapus, entri lama masih tampil dengan label snapshot.
4. Argumen berisi `"password": "x"` tersimpan sebagai tertutup; isi file tidak pernah muncul di tabel (tes).
5. Developer memanggil `GET /api/audit-log` → 403.
6. Entri berumur >90 hari hilang setelah job berjalan; entri baru tidak terpengaruh.
7. Tidak ada rute atau fungsi repository yang meng-update/menghapus entri selain purge (dicek lewat tes/grep arsitektur).
8. Filter gabungan (project + aksi + rentang waktu) memakai indeks (cek `EXPLAIN` pada data uji 1 juta baris).

## Pertanyaan terbuka

1. **Fail-closed vs fail-open** bila penulisan audit untuk panggilan agent gagal. Rekomendasi: fail-closed (panggilan MCP ditolak dengan error), karena tujuan log adalah audit. Perlu keputusanmu.
