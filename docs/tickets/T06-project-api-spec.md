# T06 — project / api-spec

| | |
|---|---|
| Domain / Module | project / api-spec |
| Ukuran | L |
| Bergantung pada | T03 |
| Dibutuhkan oleh | T09 |

## Ringkasan

Penyimpanan, validasi, dan tampilan terender spesifikasi OpenAPI (YAML/JSON) per project. Satu project boleh punya banyak spec (mis. dipisah per module/domain). Menyediakan akses per-endpoint yang hemat token untuk agent lewat T09.

## Tujuan

- Upload spec (satu file atau dalam folder), divalidasi sebagai OpenAPI.
- Spec dirender sebagai dokumentasi API yang enak dibaca di web.
- Agent bisa mendaftar endpoint dan mengambil detail satu endpoint tanpa memuat seluruh spec.

## Non-goals

Swagger 2.0, "Try it out"/eksekusi request ke server API, mock server, diff antar versi spec, pembuatan spec dari kode, editor spec di web (upload dulu), referensi `$ref` ke URL eksternal/file lokal.

## User story

- Sebagai developer, saya mengunggah `auth.yaml`, `payment.yaml`, dan `notification.yaml` ke satu project dan melihat masing-masing sebagai dokumentasi terpisah.
- Sebagai developer, saya mengunggah ulang `payment.yaml` yang diperbarui dan spec lama tertimpa.
- Sebagai agent, saya melihat daftar endpoint di spec `payment` lalu mengambil detail `POST /payments` saja.

## Features

| Folder | Deskripsi | Titik masuk | Siapa |
|---|---|---|---|
| `uploadspecs` | Upload satu/banyak file spec; validasi; timpa berdasarkan nama; laporan per file | `POST /api/projects/{id}/api-specs/upload` (multipart) | anggota, super_admin |
| `listspecs` | Daftar spec (nama, judul, versi OpenAPI, versi API, jumlah endpoint, diperbarui) | `GET /api/projects/{id}/api-specs` | anggota, super_admin |
| `getspec` | Metadata + dokumen terparsir untuk render | `GET /api/projects/{id}/api-specs/{name}` | anggota, super_admin |
| `downloadspec` | Unduh teks asli | `GET /api/projects/{id}/api-specs/{name}/raw` | anggota, super_admin |
| `renamespec` | Ganti nama spec | `PATCH /api/projects/{id}/api-specs/{name}` | anggota, super_admin |
| `deletespec` | Hapus spec | `DELETE /api/projects/{id}/api-specs/{name}` | anggota, super_admin |
| `validatespec` | Logika validasi/parsing (dipakai upload; juga endpoint "validasi tanpa menyimpan") | `POST /api/projects/{id}/api-specs/validate` | anggota, super_admin |
| `busreader` | Handler bus untuk agent: `list-specs`, `list-endpoints`, `get-endpoint` | bus `project.apispec.*` | internal |

## Aturan bisnis

### Identitas & timpa
- Spec dikenali dengan **nama** (slug: huruf kecil, angka, `-`, `_`, `.`; 1–80 karakter), unik per project. Default nama: nama file tanpa ekstensi.
- Mengunggah spec dengan nama yang sama menimpa spec itu. Ubah nama diperbolehkan selama unik.
- `source_path` (path asli di folder yang diunggah) disimpan sebagai informasi.

### Validasi
- Format: `.yaml`, `.yml`, `.json`. Ukuran maks 5 MB (konfigurabel). UTF-8.
- Harus dokumen OpenAPI **3.0.x atau 3.1.x** yang valid secara struktural (`openapi`, `info`, `paths`/`webhooks`/`components`, tipe field benar). `$ref` internal (`#/...`) harus terselesaikan.
- **Keamanan parser (wajib):** referensi eksternal (URL atau file) **tidak pernah di-fetch** (mencegah SSRF dan pembacaan file server); spec dengan `$ref` eksternal ditolak dengan pesan jelas atau ref-nya dibiarkan tidak terselesaikan dan ditandai (pilihan builder, tetapi tanpa fetch). YAML dibatasi terhadap "billion laughs" (batas alias/kedalaman/ukuran).
- File `.json`/`.yaml` yang bukan OpenAPI (mis. `package.json`) → `rejected` dengan alasan "bukan OpenAPI".
- Pesan error validasi menyebut lokasi (baris/JSON pointer) bila memungkinkan.

### Indeks endpoint
Saat upload, endpoint diekstrak ke tabel indeks (method, path, operationId, ringkasan, tag, deprecated) agar `list-endpoints` cepat tanpa mem-parse ulang spec.

### Detail endpoint untuk agent (`get-endpoint`)
- Dicari dengan `(spec, method, path)` atau `(spec, operationId)`.
- Hasil: operasi lengkap (parameter, requestBody, responses, security) dengan `$ref` ke skema **dibuka (inlined)** hingga kedalaman terbatas (default 4). Referensi sirkular diganti penanda `{"$circular": "#/components/schemas/X"}`. Skema yang tidak dirujuk tidak disertakan.
- Disertakan: `servers` yang relevan dan `securitySchemes` yang dipakai operasi.
- Ukuran hasil dibatasi (lihat T07); bila terpotong ada penanda dan petunjuk memperkecil `schema_depth`.

### Tampilan terender
- Pengelompokan per tag, lencana method, parameter, body, response, skema (dapat dilipat), contoh, status deprecated, `servers`, skema keamanan. Tanpa tombol eksekusi request.
- Isi spec tidak tepercaya: deskripsi (Markdown) disanitasi seperti T05.

## Model data

```
api_specs(id uuid pk, project_id fk→projects on delete cascade,
  name text, source_path text null, title text, api_version text, openapi_version text,
  content text, content_format text, content_hash bytea, size_bytes int,
  created_at, created_by uuid, created_by_label text,
  updated_at, updated_by uuid, updated_by_label text,
  unique(project_id, name))
api_spec_endpoints(spec_id fk→api_specs on delete cascade,
  method text, path text, operation_id text null, summary text, tags text[], deprecated bool,
  pk(spec_id, method, path))
```

Batas jumlah spec per project (default 100).

## Kontrak bus

Disediakan (`lib/contracts/project/apispec/...`), dibatasi pada `ProjectID` pemanggil:
- `project.apispec.count-specs` Call — kontrak `lib/contracts/project/management/contentcount` (`{ProjectIDs}` → `{Counts map[string]int}`): jumlah spec per project untuk T03. Hanya menghitung `ProjectIDs` yang diberikan; maksimal `contentcount.MaxProjectIDs`.
- Semua tabel dengan `project_id` WAJIB `REFERENCES projects(id) ON DELETE CASCADE`; route memakai param `{projectID}`.
- `project.apispec.list-specs` — `{ProjectID}` → `[{Name, Title, ApiVersion, OpenapiVersion, EndpointCount, UpdatedAt}]`.
- `project.apispec.list-endpoints` — `{ProjectID, Spec?, Tag?, Query?, Limit, Offset}` → `{Items[{Spec, Method, Path, OperationID, Summary, Tags, Deprecated}], Total, Truncated}`.
- `project.apispec.get-endpoint` — `{ProjectID, Spec, Method?, Path?, OperationID?, SchemaDepth}` → `{Found, Spec, Method, Path, Operation(json), Servers, SecuritySchemes, Truncated}` (`Found=false` disertai `Similar[]`).

Dikonsumsi: `project.management.check-access`, `core.auth.get-user`, `core.audit.record`.

Event audit: `apispec.uploaded` (nama, versi OpenAPI, jumlah endpoint, apakah menimpa), `apispec.updated` (rename), `apispec.deleted`.

## UI (Svelte, `domain/project/apispec/...`)

Tab "API Specs":
- Daftar spec sebagai kartu/tabel (nama, judul, versi, jumlah endpoint, diperbarui) + tombol upload.
- Dialog upload (pilih file atau folder; ringkasan hasil per file dengan alasan penolakan yang jelas dan bisa disalin).
- Halaman detail spec: pohon samping (tag → endpoint), panel utama dokumentasi terender, pencarian endpoint, unduh raw, ubah nama, hapus.
- Pemilihan renderer (komponen sendiri atau pustaka) diserahkan ke builder asalkan mematuhi aturan sanitasi dan tanpa eksekusi request.

## Kriteria penerimaan

1. Mengunggah 3 spec valid dengan nama berbeda → 3 entri; mengunggah ulang salah satu → `updated`, endpoint indeksnya diganti sepenuhnya (endpoint yang dihapus dari spec hilang dari indeks).
2. Mengunggah `package.json`, YAML rusak, spec Swagger 2.0, dan spec dengan `$ref: http://...` → semuanya `rejected` dengan alasan yang tepat; server tidak melakukan permintaan keluar (tes dengan listener lokal).
3. YAML "billion laughs" ditolak tanpa menghabiskan memori/CPU berlebihan.
4. `list-endpoints` untuk spec 500 endpoint selesai < 100 ms dan mendukung filter tag/teks/paginasi.
5. `get-endpoint` pada operasi yang merujuk skema bersarang mengembalikan skema ter-inline sesuai kedalaman, menandai referensi sirkular, dan tidak memuat skema lain yang tidak dirujuk.
6. Spec 3.0 dan 3.1 sama-sama dapat diunggah dan dirender (termasuk `nullable` vs tipe array 3.1).
7. Non-anggota mendapat 404; handler bus tidak pernah mengembalikan data project lain.
8. Deskripsi berisi HTML/skrip tidak dieksekusi di tampilan.

## Pertanyaan terbuka

Tidak ada. Dukungan hanya 3.0/3.1 dan perilaku `$ref` eksternal adalah keputusan saya.
