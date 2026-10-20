# T07 — discovery / mcp-gateway

| | |
|---|---|
| Domain / Module | discovery / mcp-gateway |
| Ukuran | M |
| Bergantung pada | T02, T04 |
| Dibutuhkan oleh | T08, T09 |

## Ringkasan

Pintu masuk agent: server MCP lewat HTTP. Mengurus protokol, autentikasi API key, pembatasan akses ke satu project, rate limit, batas ukuran respons, dan **audit setiap panggilan**. Tool konkret disediakan module T08 dan T09.

## Tujuan

- Klien MCP (Claude Code/Desktop) dapat terhubung dengan satu URL dan satu key.
- Agent hanya melihat project milik key-nya; tidak ada cara memilih project lain.
- Setiap panggilan, berhasil atau ditolak, tercatat di audit log.

## Non-goals

Tool yang menulis/mengubah data, OAuth untuk MCP, sesi stateful atau streaming SSE (versi pertama stateless), resource/prompt MCP (hanya tools), akses lewat REST `/api`.

## User story

- Sebagai developer, saya menempelkan potongan konfigurasi dari halaman API Keys dan Claude langsung bisa memakai tool knowledge project saya.
- Sebagai Super Admin, saya melihat di audit log bahwa agent memanggil `read_file` tertentu pada waktu tertentu.
- Sebagai Super Admin, saya melihat ada yang mencoba memakai key yang sudah dicabut.

## Features

| Folder | Deskripsi | Titik masuk | Siapa |
|---|---|---|---|
| `endpoint` | Endpoint MCP Streamable HTTP; routing JSON-RPC (`initialize`, `ping`, `tools/list`, `tools/call`) | `POST /mcp` | agent |
| `authenticate` | Ekstrak key dari header, verifikasi lewat bus, tetapkan project pada context | internal | – |
| `toolregistry` | Katalog tool; menerima pendaftaran tool dari module T08/T09 | wiring | internal |
| `callaudit` | Bungkus tiap `tools/call`: ukur durasi, tulis audit, terapkan batas ukuran/timeout | internal | – |
| `ratelimit` | Pembatas per key dan per IP (kegagalan auth) | internal | – |

`GET /mcp` mengembalikan `405` (tidak ada stream server→klien).

## Protokol

- Transport: MCP Streamable HTTP, mode stateless (tanpa `Mcp-Session-Id`). Versi protokol dinegosiasikan saat `initialize`; tolak versi yang tidak didukung dengan error JSON-RPC yang jelas.
- Metode yang didukung: `initialize`, `notifications/initialized` (diabaikan dengan 202), `ping`, `tools/list`, `tools/call`. Metode lain → `-32601`.
- Kegagalan **tool** (file tidak ada, argumen salah) dikembalikan sebagai hasil `isError: true` dengan pesan yang berguna bagi LLM; kegagalan **protokol** sebagai error JSON-RPC.
- `initialize` mengembalikan `serverInfo` (nama, versi) dan `instructions` singkat yang menjelaskan isi server (project apa, cara menjelajah: list → read → search).
- Semua tool diberi anotasi `readOnlyHint: true`.

## Autentikasi & otorisasi

- Header yang diterima: `Authorization: Bearer <key>` dan `X-API-Key: <key>`. Basic Auth tidak didukung kecuali diputuskan lain (lihat pertanyaan terbuka).
- Key diverifikasi lewat bus `project.apikey.verify` pada **setiap** request (tanpa cache). Hasilnya `ProjectID` dan identitas key dimasukkan ke context.
- Tool **tidak pernah** menerima parameter project; project selalu berasal dari context. Tool handler wajib meneruskan `ProjectID` itu ke bus.
- Tanpa key / key salah / dicabut → HTTP 401 dengan `WWW-Authenticate: Bearer`, tubuh seragam yang tidak membedakan alasan.
- Validasi header `Origin` (bila ada) terhadap daftar yang diizinkan (konfigurasi), untuk mencegah DNS rebinding; permintaan tanpa `Origin` (klien non-browser) diperbolehkan.
- Setelah sukses, `project.apikey.touch` di-Publish.

## Audit (kritis)

Setiap `tools/call` menghasilkan satu entri `core.audit.record` dengan: aktor `api_key` (id, nama+prefix), project (id+nama), `action = mcp.<nama_tool>`, argumen (dibersihkan), status `ok|error`, ringkasan hasil (ukuran byte, jumlah item, kode error), IP, user agent, trace id, durasi. Tambahan:
- `mcp.auth.rejected` untuk 401 (aktor `anonymous`, prefix key bila terbaca, IP).
- `mcp.rate_limited` untuk penolakan karena limit.
- `tools/list` dan `initialize` dicatat ringan (`mcp.initialize`, `mcp.tools_list`) agar terlihat kapan sebuah agent terhubung.
- Penulisan memakai `Call` sinkron. Perilaku saat gagal: **fail-open** (diputuskan): tangkap error, catat ke log aplikasi (level error), tetap layani panggilan tool.
- Nama aksi audit harus identik dengan nama tool agar mudah difilter.

## Batas & ketahanan

- Rate limit per key (default 120 permintaan/menit, konfigurabel) dan per IP untuk percobaan autentikasi gagal (mis. 20/menit) → 429 dengan `Retry-After`.
- Batas ukuran body request (mis. 64 KB). Batas ukuran respons tool (default 100 KB); bila lebih, hasil dipotong dengan penanda `truncated` dan petunjuk (offset/limit). Batas ditegakkan di gateway sebagai jaring pengaman selain batas di tiap tool.
- Timeout per panggilan tool (default 10 detik).
- Panic di tool di-recover menjadi error tool; tidak menjatuhkan server.

## Pendaftaran tool

Setiap module tool (T08, T09) mendeskripsikan tool-nya (nama, deskripsi, JSON Schema input, handler) lewat mekanisme netral yang disepakati saat wiring, tanpa mengimpor gateway atau sebaliknya. Pilihan implementasi diserahkan ke builder (mis. paket kontrak netral `lib/contracts/discovery/toolkit` berisi tipe `Tool` + registrasi di `domain/discovery/base.go`, atau endpoint bus `discovery.<module>.tools` yang dibaca gateway). Syaratnya: urutan registrasi tidak berpengaruh, nama tool unik (duplikat = panic saat start), dan gateway tidak mengimpor package tool.

## Model data

Tidak ada tabel sendiri. Rate limiter di memori (satu instance). Bila nanti multi-instance, perlu penyimpanan bersama; di luar cakupan.

## Kontrak bus

Dikonsumsi: `project.apikey.verify`, `project.apikey.touch`, `project.management.get-project` (label project untuk audit), `core.audit.record`.
Disediakan: tidak ada endpoint bus; menyediakan mekanisme pendaftaran tool seperti di atas.

## UI

Tidak ada UI sendiri. Potongan konfigurasi ada di T04; telusur aktivitas ada di T02.

## Kriteria penerimaan

1. Claude Code berhasil `claude mcp add` dengan URL + header Bearer, `tools/list` menampilkan tool T08/T09, dan satu panggilan `read_file` berjalan sampai selesai (tes manual terdokumentasi + tes integrasi JSON-RPC).
2. Request tanpa key, dengan key acak, dan dengan key dicabut sama-sama 401 dengan tubuh identik; masing-masing menghasilkan entri `mcp.auth.rejected`.
3. Key milik project A tidak bisa membaca file project B dengan cara apa pun (tool tidak menerima project id; tes yang mencoba menyuntikkan argumen `project_id`).
4. Setiap panggilan tool menghasilkan tepat satu entri audit dengan argumen dan ringkasan hasil; isi file tidak muncul di entri.
5. Pencabutan key berlaku pada request berikutnya.
6. Melebihi rate limit → 429 + entri `mcp.rate_limited`; permintaan lain dari key lain tidak terpengaruh.
7. Respons tool 500 KB dipotong ke batas dengan penanda `truncated`.
8. Duplikasi nama tool saat wiring membuat aplikasi gagal start dengan pesan jelas.
9. Request dengan `Origin` yang tidak diizinkan ditolak 403.

## Pertanyaan terbuka

1. ~~Fail-closed vs fail-open~~ **Diputuskan: fail-open.** Audit yang gagal ditulis tidak menggagalkan panggilan tool; error di-log. Lihat T02.
2. **Basic Auth:** perlu didukung? Kamu menyebutnya di awal. Rekomendasi: tidak; Bearer dan `X-API-Key` cukup untuk klien MCP umum.
