# T17 — project / git-sync (pacing rate limit + progress streaming)

| | |
|---|---|
| Domain / Module | project / git-sync (revisi T14); `infra/gitprovider` dan `infra/http` untuk pacing + SSE |
| Ukuran | M |
| Bergantung pada | T14 |
| Sumber | permintaan pemilik |

## Ringkasan

Sync dari git (T14) gagal langsung dengan `RATE_LIMITED` ("The Git provider is rate limiting requests; retry in N seconds") begitu jatah API provider habis, dan user tidak melihat apa pun selama sync berjalan. Tiket ini membuat **tiap request ke provider git di-pace** (menunggu sesuai sinyal rate limit provider, lalu lanjut) dan menampilkan **progress per file** ke UI lewat **SSE**.

## Tujuan

1. Sebelum/sesudah tiap request ke GitHub/GitLab, klien provider membaca header rate limit (`X-RateLimit-Remaining`/`Reset`, `RateLimit-*`, `Retry-After`). Bila jatah habis atau provider membalas 429/403 rate limit, request **menunggu** sampai boleh lagi lalu dilanjutkan, bukan menggagalkan sync.
2. Sync melaporkan progress **per file**: `n` dari `total`, nama file terakhir, dan status "menunggu rate limit, lanjut dalam N detik".
3. UI dialog Sync menampilkan progress bar + hitung mundur tunggu, lalu hasil akhir per file (bentuk laporan T14 tidak berubah).
4. Sync yang sudah mulai **tetap berjalan sampai selesai di backend** walau tab/koneksi SSE putus, supaya project tidak tertimpa setengah jalan.
5. Total waktu tunggu dibatasi; melewati batas → sync gagal dengan pesan jelas, bukan menggantung.

## Non-goals

- Job latar belakang dengan ID + polling, atau melanjutkan sync setelah reload halaman (transport sudah diputuskan SSE).
- Sync terjadwal/otomatis, pembatalan dari UI, antrean lintas project.
- Pacing untuk request ke luar git provider (mailer, dsb.).
- Mengubah aturan replace, pemetaan nama/path, atau batas file T14.

## Anti-tujuan (dianggap gagal walau fitur jalan)

- Sync menunggu tanpa batas, atau menahan koneksi/goroutine tanpa timeout.
- Sync berhenti di tengah (karena SSE putus atau tunggu terlalu lama) dan meninggalkan project setengah tertimpa tanpa laporan. Perilaku tulis T14 (gagal total tidak menulis apa pun) tetap berlaku.
- Event progress memuat PAT, URL dengan kredensial, atau isi file.
- Dua sync aktif untuk `(project, kind)` yang sama (aturan K10 T14 tetap).
- Header/detail rate limit provider dipercaya mentah sehingga tunggu bisa dibuat sangat lama oleh provider (wajib dibatasi).

## Keputusan pemilik

| Topik | Keputusan |
|---|---|
| Wait | **Per request** ke provider git, mengikuti sinyal rate limit provider (bukan menunggu sekali setelah error). |
| Progress | **Per file: n dari total.** |
| Transport | **Streaming SSE.** |
| Scope | Tiket baru T17; T14 tidak diubah. |

## Keputusan pembangun (mohon dicek)

- K1. **Pacing adaptif, bukan jeda tetap.** Klien provider membaca header rate limit di tiap respons. Bila `remaining` = 0 → tidur sampai `reset` (atau `Retry-After`); pada 429/403-rate-limit → tidur lalu ulangi request yang sama (retry terbatas, mis. 3 kali). Tidak ada jeda buatan bila jatah masih longgar.
- K2. **Batas tunggu.** Satu tunggu dibatasi (mis. 120 dtk) dan total tunggu per sync dibatasi; di atas itu → `RATE_LIMITED` dengan `retry_after_seconds`, sama seperti sekarang. Nilai lewat env baru (mis. `GIT_SYNC_MAX_WAIT`, `GIT_SYNC_MAX_TOTAL_WAIT`). `GIT_SYNC_TIMEOUT` (default 2 mnt, T14 K4) harus dinaikkan atau dibaca ulang karena sekarang tunggu ikut dihitung; cek validasi `GIT_SYNC_TIMEOUT > HTTP_DEFAULT_TIMEOUT` di `infra/config/git.go`. Putuskan di plan.
- K3. **Sync lepas dari request.** Eksekusi sync berjalan dengan context yang tidak ikut batal saat klien SSE putus (tetap dibatasi timeout total), sedangkan SSE hanya "penonton". Konsekuensi: perlu mekanisme agar event tidak memblokir sync bila klien lambat/putus (kanal ber-buffer, event yang tak terkirim dibuang; event terakhir/hasil akhir tetap disimpan di `git_sync_configs.last_result` seperti T14 sehingga UI bisa memuatnya dari `getconfig` setelah reload).
- K4. **Endpoint.** `POST /api/projects/{id}/git-sync/run` saat ini mengembalikan JSON laporan. Opsi: (a) endpoint baru `POST .../git-sync/run/stream` (`text/event-stream`) dan endpoint lama dipertahankan; atau (b) `run` memilih respons dari header `Accept: text/event-stream`. Rekomendasi (b) agar satu pintu, klien lama tetap jalan. Putuskan di plan. Pakai `fetch` + stream (bukan `EventSource`) karena butuh POST + header auth; periksa cara auth FE saat ini.
- K5. **Bentuk event** (nama event final di plan): `start {total}` (setelah daftar file diketahui), `progress {done, total, path}`, `waiting {retry_after_seconds, reason}`, `done {report}` (laporan per file sama dengan T14), `error {code, message, details}`. Tidak ada token/isi file di event. Heartbeat komentar SSE berkala agar proxy tidak memutus koneksi saat menunggu lama.
- K6. **Definisi "total".** Total diketahui setelah tahap daftar file (tree) selesai, sebelum unduh. Tahap daftar file sendiri bisa kena rate limit, jadi sebelum `start` UI menampilkan tahap "mengambil daftar file" tanpa angka (progress indeterminate), lalu beralih ke `n dari total`.
- K7. **Infra HTTP.** Server SSE butuh `http.Flusher`, bebas dari middleware yang membungkus/menunda respons (timeout handler, kompresi, `Content-Length`), dan write deadline yang lebih panjang untuk rute ini. Belum ada SSE di codebase (`grep event-stream` kosong); cek middleware di `infra/middleware` dan konfigurasi server (`WriteTimeout`) saat plan.
- K8. **Titik pacing.** Hanya `infra/gitprovider` (satu tempat yang sudah mendeteksi `RateLimitedError` di `client.go`) yang mengatur tunggu; `runsync` hanya menerima callback `OnWait`/`OnProgress` untuk dilaporkan ke SSE. Usecase T14 tetap mengurus laporan dan penulisan.
- K9. Audit `git_sync.run` (T14) ditambah ringkasan: total waktu tunggu dan jumlah kali menunggu. Event progress sendiri tidak diaudit.

## Constraints

- Struktur domain/module/feature yang ada. Interaksi antar module hanya lewat `infra/bus`.
- OpenAPI: perbarui `api/modules/project/git-sync.yml` (respons `text/event-stream` dan skema event), lalu `make api-bundle` dan commit `api/openapi.yml`.
- Teks UI bahasa Indonesia; kunci i18n di `frontend/src/shared/i18n/{id,en}.json`.
- Tidak menambah dependensi besar; SSE ditulis dengan library standar.
- Env baru didokumentasikan di `docs/configuration.md` dan `.env.example`.
- Perubahan di T14 dicatat di tiket ini, bukan dengan mengubah isi T14.

## Features

| Folder | Deskripsi | Titik masuk | Siapa |
|---|---|---|---|
| `infra/gitprovider` (pacing) | Baca header rate limit, tunggu, retry terbatas; callback `OnWait` | internal | internal |
| `project/git-sync/runsync` (progress) | Emit event progress/waiting/done lewat callback; sync lepas dari koneksi klien | `POST /api/projects/{id}/git-sync/run` (`Accept: text/event-stream`) | anggota, super_admin |
| `frontend/domain/project/git-sync/runsync` | Konsumsi stream, progress bar, hitung mundur tunggu, hasil akhir | dialog Sync Knowledge dan API Spec | anggota, super_admin |

## Aturan bisnis

- Pacing tidak mengubah hasil sync: file yang sama, urutan yang sama, laporan yang sama dengan T14; hanya waktu dan visibilitasnya yang berubah.
- Satu sync aktif per `(project, kind)` tetap (409 `SYNC_IN_PROGRESS`); sync yang masih menunggu rate limit dihitung aktif.
- Bila SSE putus, sync selesai di backend dan hasilnya tersimpan; user yang membuka dialog lagi melihat status/hasil terakhir dari `getconfig`.
- Melebihi batas tunggu → status `failed`, tidak ada file yang ditulis (sebelum fase tulis), pesan menyebut kapan boleh mencoba lagi.

## UI (Svelte)

- Dialog Sync: setelah klik Sync, tahap "Mengambil daftar file…" (indeterminate), lalu progress bar `n dari total` + nama file terakhir.
- Saat menunggu: pesan "Menunggu batas request provider, lanjut dalam N detik" dengan hitung mundur; progress bar tampak berhenti, bukan error.
- Hasil akhir per file seperti T14; error final memakai pesan yang ada, ditambah "coba lagi" bila `retry_after_seconds` ada.
- Menutup dialog saat sync berjalan: peringatan bahwa sync tetap berlanjut di server; membuka lagi menampilkan status terakhir.

## Titik sentuh

Backend: `infra/gitprovider/client.go` (pacing), `project/internal/gitsync/runsync/*` (callback + SSE handler), `infra/config/git.go` (env baru, validasi timeout), middleware/server HTTP (flush, write deadline), `docs/configuration.md`, `.env.example`.
Frontend: `domain/project/git-sync/runsync/*` (service stream, store progress, dialog), i18n.
Lainnya: `api/modules/project/git-sync.yml`, `api/openapi.yml`, `docs/tickets/README.md`.

## Risiko

- **SSE tertahan proxy/middleware** (buffering, timeout, kompresi) sehingga progress tidak tampil atau koneksi putus. Mitigasi: heartbeat, tes dengan middleware sebenarnya, `X-Accel-Buffering: no`.
- **Goroutine/koneksi menggantung** karena tunggu panjang. Mitigasi: batas tunggu K2, timeout total, context yang jelas.
- **Sync tak terkendali setelah klien pergi** (tidak ada pembatalan). Mitigasi: timeout total; pembatalan eksplisit di luar scope.
- **Provider memberi `Retry-After`/reset sangat jauh**: tidak boleh dipatuhi mentah (K2), jatuh ke error `RATE_LIMITED`.
- **Pacing menyembunyikan masalah repo terlalu besar**: sync jadi berjam-jam. Mitigasi: batas total tunggu dan batas file T14.

## Pertanyaan terbuka

- Batas tunggu: berapa nilai default per tunggu dan total, dan perlu env terpisah atau cukup memperpanjang `GIT_SYNC_TIMEOUT`?
- Opsi endpoint K4: `Accept` header (satu pintu) atau endpoint `/stream` terpisah?
- Perlu tombol **Batalkan** sekarang, atau cukup di tiket lanjutan?
- Perlu menampilkan sisa jatah rate limit provider ke user (mis. "tersisa 12 request"), atau hanya saat menunggu?
