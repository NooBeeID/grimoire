# T10 — project / api-spec (dukungan Swagger 2.0)

| | |
|---|---|
| Domain / Module | project / api-spec (lintas ke discovery / api-spec-tools untuk teks MCP) |
| Ukuran | L |
| Bergantung pada | T06, T09 |
| Sumber | `docs/superpowers/specs/2026-10-21-swagger2-support-design.md` |

## Ringkasan

Tim mengunggah spec API dari proyek lain, contoh nyata `hr-platform/api/openapi.yml` (Swagger 2.0, 3,5 MB, ~77 ribu baris, hasil bundle dari fragmen modul). Hari ini upload ditolak dengan `NOT_OPENAPI_3`. Tiket ini membuat spec `swagger: "2.0"` (YAML dan JSON) bisa diunggah, dibaca tim di UI, dan dikonsumsi agen AI lewat MCP tanpa dikonversi dulu oleh pengguna.

Prinsip pemilik: **"kalau 2.0 pakai 2.0, kalau 3.0 pakai 3.0."** File yang tersimpan dan diunduh selalu sama persis dengan yang diunggah.

## Tujuan

1. Upload dan validasi spec `swagger: "2.0"` dengan perilaku yang sama seperti 3.x untuk nama, duplikat, batas jumlah/ukuran, dan status `created/updated/unchanged/rejected`.
2. Daftar endpoint (method, path, tag, summary, operationId, deprecated) terisi untuk 2.0 dan muncul di UI, pencarian, dan MCP (`list_api_endpoints`).
3. Detail operasi 2.0 bisa dibaca di UI (parameter, body, respons, model, keamanan, server).
4. Tool MCP `get_api_endpoint` mengembalikan operasi 2.0 apa adanya, diberi penanda versi.
5. Spec 3.0/3.1 tidak berubah perilakunya sama sekali.

## Non-goals

- Menyimpan atau mengunduh hasil konversi. Yang disimpan hanya file asli.
- Menulis balik atau mengedit spec.
- Menggabungkan fragmen multi-file atau `$ref` ke file/URL eksternal. Tetap ditolak seperti sekarang; yang diunggah satu file bundle.
- "Try it out", eksekusi request, mock server, diff versi.
- Swagger 1.x.

## Anti-tujuan (dianggap gagal walau fitur jalan)

- Melonggarkan proteksi parser (batas alias, node, kedalaman, ukuran file, larangan `$ref` eksternal / permintaan keluar) diam-diam agar file besar lolos.
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
| Batas parser | Boleh dinaikkan **hanya dengan persetujuan eksplisit pemilik**. Pembangun mengukur, mengusulkan angka beserta data, lalu berhenti dan menunggu (lihat "Spike pengukuran"). |

## Keputusan pembangun (belum disetujui pemilik; mohon dicek)

Nomor merujuk daftar di spec. Butir 2 (format `swagger`) digantikan butir 11, dan butir 3 (nasib `NOT_OPENAPI_3`) digantikan butir 12.

- K1. Versi tersimpan `"2.0"` di `api_specs.openapi_version` (text, NOT NULL, tanpa CHECK); tanpa migrasi. UI menulis "Swagger 2.0" untuk 2.0 dan "OpenAPI x.y.z" untuk 3.x.
- K2. Method yang diindeks untuk 2.0: get, put, post, delete, options, head, patch (tanpa `trace`, tanpa `webhooks`).
- K3. `servers` untuk 2.0 dibentuk dari `schemes` × `host` + `basePath`. `schemes` kosong → `https`. `host` kosong → hanya `basePath`. Berlaku sama di hasil `get_api_endpoint` dan di adapter FE.
- K4. Hasil baca operasi 2.0 (backend, dipakai UI dan MCP): digabung dengan parameter level path (aturan 3.x), `$ref` internal di-inline dengan kedalaman/batas byte yang sama, ditambah `consumes`/`produces` efektif (operasi menimpa root) dan skema keamanan efektif dari `securityDefinitions`.
- K5. Teks MCP (`list_api_specs` dan tool lain yang mencetak versi) menulis "Swagger 2.0" untuk spec 2.0.
- K6. Adapter FE (fungsi murni, diuji unit; letak: `model/` di `getspec`), bekerja pada salinan di memori:
  - `#/definitions/X` → `#/components/schemas/X` (juga `parameters`, `responses`).
  - `in: body` → `requestBody` per MIME di `consumes`.
  - `in: formData` → `requestBody` form (`multipart/form-data` bila ada `type: file`, selain itu `application/x-www-form-urlencoded`); `type: file` → string biner.
  - `responses.*.schema` → `content` per MIME di `produces`; `responses.*.examples` → contoh per MIME.
  - `x-nullable: true` → nullable.
  - `securityDefinitions` (`basic` → http basic, `apiKey`, `oauth2` dengan `flow` → `flows`) → `securitySchemes`.
  - `host/basePath/schemes` → `servers`.
- K7. Validasi dan inline di backend memakai ulang mesin `$ref` 3.x (resolver, batas kedalaman, penjaga siklus) dengan peta awalan `#/definitions/`, `#/parameters/`, `#/responses/`. `$ref` internal tidak ditemukan → `rejected`; `$ref` bukan `#...` → `rejected`. `allOf`, `discriminator`, `collectionFormat` hanya divalidasi struktur dasar dan `$ref`-nya; lewat apa adanya ke hasil baca dan adapter FE.
- K8. Penanda versi di respons `getendpoint`: field opsional di amplop respons (bukan di dalam objek operasi), `omitempty`, hanya muncul untuk 2.0. Keluaran 3.x tidak berubah satu byte pun. `servers` dan keamanan efektif untuk 2.0 memakai field amplop yang sama dengan 3.x.
- K9. Format `swagger`: string `"2.0"`, angka `2.0`, dan integer `2` diterima dan disimpan sebagai `"2.0"`. `"2.0.0"`, `"1.2"`, `"3.0"`, dan nilai non-skalar → `UNSUPPORTED_VERSION`; `swagger: null` → `NOT_OPENAPI`. Dokumen dengan `openapi` dan `swagger` sekaligus: `openapi` menang.
- K10. Kode `NOT_OPENAPI_3` tetap ada di kontrak API dan kunci i18n-nya dibiarkan; ia hanya tidak lagi muncul untuk Swagger 2.0 yang valid.
- K11. Semua teks yang menulis versi (UI `ResultTable`, `OpenApiBadge`, tool MCP) dicari dengan grep dan memakai satu fungsi format "Swagger 2.0" / "OpenAPI x.y.z".
- K12. Pemetaan 2.0 yang tidak dikenal di adapter FE turun dengan aman (tampil apa adanya), tidak melempar error.

## Constraints

- Struktur domain/module/feature yang ada (`backend/internal/domain/project/internal/apispec/...`); parser di `base/openapi`. FE membaca dokumen sebagai JSON utuh (`getspec` mengembalikan `Document`); semua model tampilan bekerja di sisi klien.
- Kontrak API di `api/modules/project/api-spec.yml`, di-bundle lewat `make api-bundle`; perubahan bentuk respons/kode issue ikut ke sana dan `api/openapi.yml` di-commit.
- Tidak ada permintaan jaringan keluar saat memproses spec (jaminan yang sudah dites).
- Teks UI dan pesan dalam bahasa Indonesia, kunci i18n di `frontend/src/shared/i18n/{id,en}.json`.
- Tidak ada feature folder baru yang wajib; nama dan lokasi file baru, pembagian tugas, bentuk persis field penanda versi, struktur internal validator 2.0 (berbagi helper atau cabang terpisah), dan pemilihan fixture 2.0 diserahkan ke pembangun.

## Titik sentuh

Backend:
- `base/openapi/parse.go` (cabang `swagger`), `validate.go` (cabang 2.0), `refs.go` / `resolve.go` (`$ref` ke `#/definitions` dkk., `servers`, `securitySchemes`), `extract.go`.
- Feature T06: `uploadspecs`, `validatespec`, `getendpoint`, `getspec`.
- Teks tool MCP di T09 (`apispectools`) dan kontrak `lib/contracts/project/apispec/getendpoint`.

Frontend:
- `getspec/` (service + `model/` + komponen `OperationView`, `ApiOverview`, `ServersList`, `SecuritySchemesList`, `SchemaTree`), `OpenApiBadge`, teks hasil upload (`ResultTable`).

## Spike pengukuran (langkah pertama, memblokir sisanya)

Sebelum merencanakan sisanya, ukur `hr-platform/api/openapi.yml`: jumlah node, alias, waktu parse, ukuran hasil baca, dan dampaknya ke batas `MaxNodes` (500.000), `MaxAliases` (100), ukuran file 5 MB, serta batas inline `$ref`.

Pengukuran awal: 0 `$ref` eksternal, 2.481 `$ref` internal, ~14 kandidat anchor/alias YAML, 300 parameter `in: body`, tanpa `in: formData`, 82 `allOf`, 1 `discriminator`, 79 `collectionFormat`, 1.623 `x-nullable`.

Jika ada batas terlampaui: **berhenti**, laporkan data dan usulan angka baru beserta alasannya, tunggu persetujuan eksplisit pemilik sebelum mengubah konstanta apa pun. Fitur dianggap selesai bila file hr-platform lolos upload dan endpoint-nya muncul.

## Tes yang harus dibalik (saat ini mengunci penolakan 2.0)

- Backend: `apispec_integration_test.go`, `validatespec/usecase_test.go`, `uploadspecs/usecase_test.go`, `base/openapi/parse_test.go`.
- Frontend: `uploadspecs/{model.test.ts, mocks/results.ts, views/UploadSpecsDialog.test.ts, components/components.test.ts}`.
- E2E: fixture `swagger-2.0.yaml`.
- Dokumen: `docs/superpowers/plans/project-api-spec/*` dan T06 (yang menyebut 2.0 sebagai non-goal) diberi catatan revisi, tidak ditulis ulang.

## Kontrak bus

Tidak ada endpoint bus baru. Respons `project.apispec.get-endpoint` mendapat field amplop opsional penanda versi (K8); selain itu tidak ada perubahan bentuk untuk 3.x.

## Risiko

- File hr-platform sangat besar; batas parser belum diuji terhadapnya (lihat spike).
- Adapter FE memetakan banyak variasi 2.0 (`collectionFormat`, `allOf` + `discriminator`, `x-nullable`, parameter `$ref` tingkat path).

## Kriteria penerimaan

1. Petstore Swagger 2.0 (YAML dan JSON) terunggah `created`, versi tersimpan `2.0`, endpoint terdaftar dan terurut seperti 3.x, unduh = byte yang sama dengan file asli.
2. Swagger 2.0 rusak (mis. `$ref: '#/definitions/Hilang'`, `operationId` ganda, parameter tanpa `in`) → `rejected` dengan issue berisi pointer dan nomor baris.
3. `$ref` http(s) atau file di spec 2.0 → `rejected`, tanpa permintaan keluar.
4. Detail operasi 2.0 di UI menampilkan parameter (query/path/header), body, form, respons, model bersarang dan sirkular, keamanan, dan server; konten Markdown tetap disanitasi seperti 3.x.
5. `get_api_endpoint` untuk 2.0 mengembalikan operasi 2.0 ter-inline dengan penanda versi; `list_api_specs` menulis "Swagger 2.0".
6. Seluruh tes 3.0/3.1 yang ada lulus tanpa perubahan selain tes penolakan 2.0 yang dibalik; keluaran 3.x tidak berubah.
7. Uji nyata: `hr-platform/api/openapi.yml` diunggah. Hasil (lolos, atau gagal karena batas mana) dilaporkan apa adanya; tidak ada batas yang diubah tanpa persetujuan.
8. `make api-check` lulus; lolos tes arsitektur T00.

## Pertanyaan terbuka

1. Persetujuan pemilik atas "Keputusan pembangun" K1-K12.
2. Jika file hr-platform melampaui batas parser: angka baru yang disetujui (diputuskan setelah spike).
