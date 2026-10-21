# Dukungan Swagger 2.0 pada API spec project

Tanggal: 2026-10-21. Status: menunggu persetujuan.

## Maksud

Tim mengunggah spec API dari proyek lain (contoh nyata: `hr-platform/api/openapi.yml`, Swagger 2.0, 3,5 MB, ~77 ribu baris, hasil bundle dari fragmen modul). Hari ini upload ditolak dengan `NOT_OPENAPI_3` ("Swagger 2.0 tidak didukung"). Tujuannya: spec Swagger 2.0 bisa diunggah, dibaca tim di UI, dan dikonsumsi agen AI lewat MCP, tanpa dikonversi dulu oleh pengguna.

Prinsip dari pemilik: **"kalau 2.0 pakai 2.0, kalau 3.0 pakai 3.0."** File yang tersimpan dan diunduh selalu sama persis dengan yang diunggah.

## Tujuan

1. Upload dan validasi spec `swagger: "2.0"` (YAML dan JSON) ke project, dengan perilaku yang sama seperti 3.x untuk nama, duplikat, batas jumlah/ukuran, status `created/updated/unchanged/rejected`.
2. Daftar endpoint (method, path, tag, summary, operationId, deprecated) terisi untuk 2.0 dan muncul di UI, pencarian, dan MCP (`list_api_endpoints`).
3. Detail operasi 2.0 bisa dibaca di UI (parameter, body, respons, model, keamanan, server).
4. Tool MCP `get_api_endpoint` mengembalikan operasi 2.0 apa adanya, diberi penanda versi.
5. Spec 3.0/3.1 tidak berubah perilakunya sama sekali.

## Bukan tujuan

- Menyimpan atau mengunduh hasil konversi. Yang disimpan hanya file asli.
- Menulis balik atau mengedit spec.
- Menggabungkan fragmen multi-file atau `$ref` ke file/URL eksternal. Tetap ditolak seperti sekarang; yang diunggah satu file bundle.
- "Try it out", eksekusi request, mock server, diff versi.
- Swagger 1.x.

## Anti-tujuan (dianggap gagal walau fitur jalan)

- Melonggarkan proteksi parser (batas alias, node, kedalaman, ukuran file, larangan `$ref` eksternal / permintaan keluar) diam-diam agar file besar lolos. Kalau file hr-platform melampaui batas, itu keputusan terpisah milik pemilik, bukan dilonggarkan oleh pembangun.
- Spec 2.0 rusak lolos validasi lalu baru gagal saat dibuka.
- Perilaku 3.x berubah (urutan endpoint, pesan issue, bentuk keluaran MCP untuk 3.x).

## Keputusan pemilik

| Topik | Keputusan |
|---|---|
| Penyimpanan | Asli apa adanya. Unduh = file yang diunggah. |
| Parser/validasi/ekstraksi | Native 2.0, tanpa konversi di backend. |
| Validasi | Setara 3.x: struktur `paths`, operasi, parameter, `$ref` internal ke `#/definitions`, `#/parameters`, `#/responses`, `operationId` unik, batas dan bentuk issue yang sama. |
| UI | Satu tampilan. Adapter di frontend memetakan dokumen 2.0 ke bentuk yang dipakai komponen yang ada, hanya di lapisan tampilan. Dokumen tersimpan tidak berubah. |
| MCP | Operasi 2.0 apa adanya + penanda versi. Tidak dinormalisasi ke 3.x. |

## Keputusan saya (belum disetujui pemilik; mohon dicek)

1. **Nilai versi tersimpan: `"2.0"`** di kolom `api_specs.openapi_version` (text, NOT NULL, tanpa CHECK). Tidak perlu migrasi. Lencana dan teks menampilkan "Swagger 2.0" untuk 2.0, "OpenAPI x.y.z" untuk 3.x.
2. **Format `swagger`:** terima string `"2.0"` dan angka `2.0` (banyak file YAML menulisnya tanpa tanda kutip). Nilai lain (`"1.2"`, `"3.0"`) → `UNSUPPORTED_VERSION`. Dokumen yang punya `openapi` dan `swagger` sekaligus: `openapi` menang (perilaku 3.x tetap).
3. **Kode issue:** `NOT_OPENAPI_3` tidak lagi dipakai untuk Swagger 2.0 yang valid. Kodenya dipertahankan di kontrak bila masih dibutuhkan, atau dihapus beserta kunci i18n `apispec.issue.NOT_OPENAPI_3` bila tidak ada pemakai lagi; builder yang memutuskan setelah memeriksa pemakai.
4. **Method yang diindeks untuk 2.0:** get, put, post, delete, options, head, patch (2.0 tidak punya `trace`; tidak ada `webhooks`).
5. **`servers` untuk 2.0** dibentuk dari `schemes` × `host` + `basePath`. `schemes` kosong → `https`. `host` kosong → hanya `basePath` (relatif). Berlaku sama di hasil `get_api_endpoint` dan di adapter FE.
6. **Hasil baca operasi 2.0** (backend, dipakai UI dan MCP): operasi digabung dengan parameter level path (aturan 3.x yang sama), `$ref` internal di-inline dengan kedalaman/batas byte yang sama, ditambah `consumes`/`produces` efektif (operasi menimpa root) dan skema keamanan efektif dari `securityDefinitions`. Field penanda versi ditambahkan di kontrak `lib/contracts/project/apispec/getendpoint`.
7. **Teks MCP** (`list_api_specs` dan sejenisnya) menulis "Swagger 2.0" bukan "OpenAPI 2.0" untuk spec 2.0.
8. **Adapter FE** (fungsi murni, diuji unit; letak: modul `model/` di `getspec`): menulis ulang `#/definitions/X` → `#/components/schemas/X` (juga `parameters`, `responses`); `in: body` → `requestBody` per MIME di `consumes`; `in: formData` → `requestBody` form (`multipart/form-data` bila ada `type: file`, selain itu `application/x-www-form-urlencoded`), `type: file` → string biner; `responses.*.schema` → `content` per MIME di `produces`; `responses.*.examples` → contoh per MIME; `x-nullable: true` → nullable; `securityDefinitions` (`basic` → http basic, `apiKey`, `oauth2` dengan `flow` → `flows`) → `securitySchemes`; `host/basePath/schemes` → `servers`. Adapter bekerja pada salinan di memori.

9. **Validasi dan inline di backend memakai ulang mesin `$ref` yang sama dengan 3.x** (resolver, batas kedalaman, penjaga siklus), dengan peta awalan 2.0: `#/definitions/`, `#/parameters/`, `#/responses/`. `$ref` internal yang tidak ditemukan → `rejected` (sama seperti 3.x); `$ref` bukan `#...` → `rejected`. `allOf`, `discriminator`, dan `collectionFormat` tidak divalidasi secara semantik (cukup struktur dasar dan `$ref`-nya); mereka lewat apa adanya ke hasil baca dan ke adapter FE.
10. **Penanda versi di respons `getendpoint`:** field opsional di amplop respons (bukan di dalam objek operasi), tampil hanya untuk spec 2.0 (`omitempty`). Keluaran 3.x tidak berubah satu byte pun. `servers` dan keamanan efektif untuk 2.0 ditempatkan di field amplop yang sama dengan yang dipakai 3.x.
11. **Format `swagger`:** string `"2.0"`, angka `2.0`, dan integer `2` diterima dan disimpan sebagai `"2.0"`. `"2.0.0"`, `"1.2"`, dan nilai non-skalar → `UNSUPPORTED_VERSION`; `swagger: null` → `NOT_OPENAPI`.
12. **Kode `NOT_OPENAPI_3` tetap ada di kontrak API** (tidak dihapus) agar klien di luar repo tidak patah; ia hanya tidak lagi muncul untuk Swagger 2.0 yang valid. Kunci i18n-nya dibiarkan.
13. **Daftar teks yang menulis versi** (UI `ResultTable`, `OpenApiBadge`, teks tool MCP `list_api_specs` dan tool lain yang mencetak versi) dicari builder dengan grep; semuanya memakai satu fungsi format "Swagger 2.0" / "OpenAPI x.y.z".

## Kendala

- Backend Go dengan struktur domain/module/feature yang ada (`backend/internal/domain/project/internal/apispec/...`); parser ada di `base/openapi`. Dokumen dibaca FE sebagai JSON utuh (`getspec` mengembalikan `Document`), dan semua model tampilan FE bekerja di sisi klien.
- Kontrak API ada di `api/modules/project/api-spec.yml` dan di-bundle ke `api/openapi.yml` lewat `make api-bundle`; perubahan bentuk respons/kode issue harus ikut ke sana.
- Tidak ada permintaan jaringan keluar dari server saat memproses spec (jaminan yang sudah dites).
- Teks UI dan pesan dalam bahasa Indonesia, kunci i18n di `frontend/src/shared/i18n/{id,en}.json`.

## Titik sentuh yang sudah teridentifikasi

Backend: `base/openapi/parse.go` (cabang `swagger`), `validate.go` (cabang 2.0), `refs.go`/`resolve.go` (`$ref` ke `#/definitions` dkk., `servers`, `securitySchemes`), `extract.go`, `uploadspecs`, `validatespec`, `getendpoint`, `getspec`, teks tool MCP di gateway/discovery, kontrak `getendpoint`.
Frontend: `getspec/` (service + `model/` + komponen `OperationView`, `ApiOverview`, `ServersList`, `SecuritySchemesList`, `SchemaTree`), `OpenApiBadge`, teks hasil upload (`ResultTable` menulis "OpenAPI {versi}").

## Tes yang harus dibalik (saat ini mengunci penolakan 2.0)

`apispec_integration_test.go`, `validatespec/usecase_test.go`, `uploadspecs/usecase_test.go`, `base/openapi/parse_test.go`, `frontend/.../uploadspecs/{model.test.ts,mocks/results.ts,views/UploadSpecsDialog.test.ts,components/components.test.ts}`, fixture e2e `swagger-2.0.yaml`, serta rencana `docs/superpowers/plans/project-api-spec/*` dan tiket T06 yang menyebut 2.0 sebagai non-goal (perlu catatan revisi, bukan ditulis ulang).

## Kriteria penerimaan

1. Petstore Swagger 2.0 (YAML dan JSON) terunggah `created`, versi tersimpan `2.0`, endpoint terdaftar dan terurut seperti 3.x, unduh = byte yang sama dengan file asli.
2. Swagger 2.0 yang rusak (mis. `$ref: '#/definitions/Hilang'`, `operationId` ganda, parameter tanpa `in`) → `rejected` dengan issue berisi pointer dan nomor baris.
3. `$ref` http(s) atau file di spec 2.0 → `rejected`, tanpa permintaan keluar.
4. Detail operasi 2.0 di UI menampilkan parameter (query/path/header), body, form, respons, model bersarang dan sirkular, keamanan, dan server; konten Markdown tetap disanitasi seperti 3.x.
5. `get_api_endpoint` untuk 2.0 mengembalikan operasi 2.0 ter-inline dengan penanda versi; `list_api_specs` menulis "Swagger 2.0".
6. Seluruh tes 3.0/3.1 yang ada lulus tanpa perubahan selain tes penolakan 2.0 yang dibalik.
7. Uji nyata: `hr-platform/api/openapi.yml` diunggah. Hasil (lolos, atau gagal karena batas mana) dilaporkan apa adanya; tidak ada batas yang diubah tanpa persetujuan.

## Diserahkan ke builder

Nama dan lokasi file baru, pembagian tugas dalam rencana, bentuk persis field penanda versi di kontrak, struktur internal validator 2.0 (berbagi helper dengan 3.x atau cabang terpisah), pemilihan fixture 2.0 untuk tes.

## Risiko yang diketahui

- File hr-platform sangat besar; batas `MaxNodes` (500.000), `MaxAliases` (100), ukuran 5 MB, dan batas inline `$ref` belum diuji terhadapnya. Hasil pengukuran awal terhadap file itu: 0 `$ref` eksternal, 2.481 `$ref` internal, ~14 kandidat anchor/alias YAML, 300 parameter `in: body`, tidak ada `in: formData`, 82 `allOf`, 1 `discriminator`, 79 `collectionFormat`, 1.623 `x-nullable`. Artinya: tidak ada penolak karena `$ref` eksternal, dan `x-nullable` serta `allOf` harus ditangani baik di adapter FE.
- Pembangun mengukur file itu pada langkah pertama (spike pengukuran: jumlah node, alias, waktu parse, ukuran hasil baca) sebelum merencanakan sisanya. Jika batas terlampaui, pekerjaan berhenti di titik itu dan keputusan dikembalikan ke pemilik (lihat "Keputusan terbuka").
- Adapter FE memetakan banyak variasi 2.0 (collectionFormat, `allOf` + `discriminator`, `x-nullable`, parameter `$ref` tingkat path); pemetaan yang tidak dikenal harus turun dengan aman (tampil apa adanya), tidak melempar error.

## Keputusan pemilik tentang batas parser

Jika `hr-platform/api/openapi.yml` melampaui salah satu batas parser, **batas boleh dinaikkan, tetapi hanya dengan persetujuan eksplisit pemilik.** Pembangun mengukur file itu, mengusulkan angka baru beserta alasannya (data pengukuran), lalu berhenti dan menunggu persetujuan sebelum mengubah konstanta apa pun. Fitur dianggap selesai bila file hr-platform lolos upload dan endpoint-nya muncul. Larangan melonggarkan batas tanpa persetujuan di bagian anti-tujuan tetap berlaku.
