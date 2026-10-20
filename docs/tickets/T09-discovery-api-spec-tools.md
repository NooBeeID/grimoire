# T09 — discovery / api-spec-tools

| | |
|---|---|
| Domain / Module | discovery / api-spec-tools |
| Ukuran | S |
| Bergantung pada | T06, T07 |

## Ringkasan

Tool MCP untuk menjelajah API spec project dengan hemat token: daftar spec → daftar endpoint → detail satu endpoint. Module tipis di atas bus T06.

## Tujuan

- Agent tidak perlu memuat seluruh spec untuk mengetahui satu endpoint.
- Alur penelusuran jelas: `list_api_specs` → `list_api_endpoints` → `get_api_endpoint`.

## Non-goals

Mengeksekusi request ke API, mengunduh spec mentah penuh, mengubah spec, akses lintas project.

## Features (1 feature = 1 folder)

| Folder | Tool MCP | Bus yang dipanggil |
|---|---|---|
| `listapispecs` | `list_api_specs` | `project.apispec.list-specs` |
| `listapiendpoints` | `list_api_endpoints` | `project.apispec.list-endpoints` |
| `getapiendpoint` | `get_api_endpoint` | `project.apispec.get-endpoint` |
| `register` | mendaftarkan ketiga tool ke gateway (T07) | – |

## Spesifikasi tool

Project selalu dari context gateway; tidak ada parameter project.

### `list_api_specs`
- Tanpa input. Output: tiap spec `name`, `title`, `api_version`, `openapi_version`, jumlah endpoint, diperbarui.
- Deskripsi: titik awal; `name` dipakai pada tool lain.

### `list_api_endpoints`
- Input: `spec` (opsional; kosong = semua spec, dikelompokkan per spec), `tag` (opsional), `query` (opsional, cocokkan path/ringkasan/operationId), `limit` (default 100, maks 300), `offset`.
- Output: baris `METHOD /path — ringkasan [operationId] (tag)`, penanda deprecated, total, dan petunjuk paginasi bila terpotong.
- `spec` tidak dikenal → `isError` dengan daftar nama spec yang valid.

### `get_api_endpoint`
- Input: `spec` (wajib bila project punya >1 spec; bila hanya satu boleh dihilangkan), pilih salah satu: (`method` + `path`) atau `operation_id`; `schema_depth` (default 4, maks 8).
- Output: JSON operasi dengan skema ter-inline sesuai kedalaman, `servers` relevan, `securitySchemes` yang dipakai, dan penanda sirkular; bila terpotong ada penanda dan saran menurunkan `schema_depth`.
- Tidak ketemu → `isError` dengan saran endpoint terdekat (`Similar`) dan petunjuk memakai `list_api_endpoints`.

## Aturan bisnis

- Semua tool `readOnlyHint: true`.
- Validasi argumen: method case-insensitive (dinormalisasi ke huruf besar) dan hanya method HTTP valid; `path` harus persis seperti di spec (termasuk `{param}`).
- Tidak ada logika akses di sini; project dibatasi oleh key dan T06.
- Hanya mengimpor `lib/constants`, `lib/contracts/...`, `infra/bus`.
- Audit dilakukan oleh gateway; hasil tool menyertakan metadata ringkas (jumlah endpoint, ukuran byte).

## Kontrak bus

Dikonsumsi: tiga endpoint `project.apispec.*` (T06). Tidak menyediakan endpoint bus.

## UI

Tidak ada.

## Kriteria penerimaan

1. Project dengan 3 spec: `list_api_specs` menampilkan ketiganya; `list_api_endpoints` tanpa `spec` mengelompokkan per spec; dengan `tag` dan `query` menyaring dengan benar.
2. `get_api_endpoint` untuk operasi yang memakai skema bersarang mengembalikan skema ter-inline, penanda sirkular, dan tidak memuat skema yang tidak dirujuk.
3. Project dengan >1 spec dan `spec` dihilangkan → error yang mencantumkan nama spec yang valid; project dengan 1 spec berhasil tanpa `spec`.
4. Endpoint salah ketik memunculkan saran terdekat.
5. Respons lebih besar dari batas dipotong dengan penanda dan petunjuk `schema_depth`.
6. Argumen `project_id` tidak berpengaruh apa pun.
7. Lolos tes arsitektur T00 dan tes kontrak `tools/list`.

## Pertanyaan terbuka

Tidak ada.
