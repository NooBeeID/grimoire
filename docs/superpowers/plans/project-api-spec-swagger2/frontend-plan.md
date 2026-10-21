# T10 project/api-spec (Swagger 2.0) — Frontend Plan

Overview (menang bila bertentangan sampai diperbarui): `docs/superpowers/plans/project-api-spec-swagger2/overview.md` (rujukan `OV <bagian>`).
Plan usecase backend: `usecase-uploadspecs.md` (jalur REST, fixture e2e `swagger-2.0.yaml`) dan `usecase-getendpoint.md` (bus + MCP).
Tiket: `docs/tickets/T10-project-api-spec-swagger2.md`. Spec: `docs/superpowers/specs/2026-10-21-swagger2-support-design.md`.
Konteks yang dibaca: tidak ada `CLAUDE.md`; `.claude/rules/migrations.md` (tidak relevan, tanpa migrasi); `docs/tickets/README.md` bagian "Frontend (Svelte)"; `DESIGN.md`; `.claude/skills/design-taste-frontend/references/app-screens.md`; kode `frontend/src/domain/project/api-spec/**` di `main`.

Prinsip pemilik: **"kalau 2.0 pakai 2.0, kalau 3.0 pakai 3.0."** Frontend tidak mengubah dokumen tersimpan dan tidak mengirim apa pun balik. Pemetaan 2.0 -> bentuk tampilan 3.x terjadi **hanya di memori, di lapisan tampilan**, lewat fungsi murni `adaptSwagger2` (K6, K12). Unduh (`/raw`) tetap memberi byte asli (tanpa perubahan FE).

---

## 1. Metadata

| Atribut | Nilai |
|---|---|
| Ticket | T10 |
| Scope | frontend (bagian dari feature fullstack `project-api-spec-swagger2`) |
| Domain / Module | project / api-spec |
| Feature yang disentuh | `getspec` (utama), `uploadspecs`, `listspecs` (hanya tes), shared (`OpenApiBadge`, util label versi, i18n) |
| Feature folder baru | tidak ada |
| Route baru | tidak ada |
| Kontrak API | `api/modules/project/api-spec.yml` (bentuk tidak berubah; hanya contoh/deskripsi oleh plan `uploadspecs`) |

Endpoint yang dikonsumsi FE (semua sudah ada, tanpa prefix versi; prefix `/api`):

| Endpoint | Method | Version | Feature FE | Perubahan yang terlihat di FE |
|---|---|---|---|---|
| `/api/projects/{projectID}/api-specs` | GET | - | `listspecs` | baris `openapi_version: "2.0"` -> badge "Swagger 2.0" |
| `/api/projects/{projectID}/api-specs/{name}` | GET | - | `getspec` | `document` bisa berkunci `swagger: "2.0"`; zod menerima, adapter memetakan |
| `/api/projects/{projectID}/api-specs/upload` | POST (multipart, `X-Requested-With: grimoire`) | - | `uploadspecs` | hasil `created` dengan `openapi_version: "2.0"`; label "Swagger 2.0" |
| `/api/projects/{projectID}/api-specs/validate` | POST (multipart) | - | `uploadspecs` | hasil `valid` untuk 2.0 |
| `/api/projects/{projectID}/api-specs/{name}/raw` | GET | - | `downloadspec` | tidak ada perubahan (byte asli) |

Route yang terdampak (tanpa perubahan definisi, sudah ada di `app/workspace/tabs.ts` + `app/views/workspace/ApiSpecsTab.svelte`):

| Route | View | Isi yang berubah |
|---|---|---|
| `/projects/:projectID/api-specs` | `ListSpecsView` + `UploadSpecsDialog` | badge versi, hasil upload |
| `/projects/:projectID/api-specs/:name` | `SpecDetailView` -> `ApiOverview` | overview 2.0 (server dari host/basePath/schemes, skema keamanan dari `securityDefinitions`) |
| `/projects/:projectID/api-specs/:name?op=METHOD%20/path` | `SpecDetailView` -> `OperationView` | parameter, body, form, respons, model, keamanan, server untuk 2.0 |

---

## 2. Summary Plan

1. **Label versi tunggal (K11, OV D9):** `shared/utils/openapi.ts` -> `openApiLabel(v)` ("Swagger 2.0" bila `v === "2.0"`, selain itu "OpenAPI " + v) dan `isSwagger2Version(v)`. Dipakai `OpenApiBadge` dan `ResultTable`. Tidak ada teks "OpenAPI {versi}" lain yang ditulis tangan (grep di langkah akhir).
2. **Zod (`getspec/types.ts`):** dokumen detail diterima bila berbentuk 3.x (skema lama, tidak berubah) **atau** Swagger 2.0 (`swagger` = `"2.0"` atau angka `2`, `info.title`/`info.version` string). Dokumen yang memuat `openapi` selalu dibaca sebagai 3.x (OV D2).
3. **Adapter (`getspec/model/swagger2.ts`):** `isSwagger2(doc)` dan `adaptSwagger2(doc)`, fungsi murni, tidak memutasi masukan, tidak melempar, tidak meng-inline `$ref` `#/definitions` (jadi aman untuk model sirkular). Aturan pemetaan persis di bagian 3 langkah 3.
4. **Service (`getspec.service.ts`):** setelah validasi zod, bila `isSwagger2(document)` maka `document = adaptSwagger2(document)`; `spec.openapi_version` tetap `"2.0"` (sumber kebenaran versi untuk badge). Dokumen 3.x dikembalikan identik (referensi sama).
5. **Komponen tampilan (`OperationView`, `ApiOverview`, `ServersList`, `SecuritySchemesList`, `SchemaTree`):** tidak diubah strukturnya; mereka menerima dokumen berbentuk 3.x hasil adapter. Perubahan hanya bila tes 2.0 menunjukkan celah (mis. parameter tanpa `schema`), dan celah itu ditutup di adapter, bukan di komponen.
6. **Upload:** `ResultTable` memakai `openApiLabel`; tes yang mengunci penolakan 2.0 dibalik; `apispec.list.emptyHint` menyebut Swagger 2.0.
7. **E2E:** spec Playwright baru `e2e/api-spec-swagger2.spec.ts` (UI + API) memakai fixture `e2e/fixtures/apispec/*`; satu kasus MCP 2.0 di `e2e/api-spec-tools.spec.ts`.

Keputusan khusus FE (turunan K6, K11, K12; tidak mengubah OV):

| # | Keputusan | Alasan |
|---|---|---|
| FE1 | Adapter dijalankan di `getspec.service.ts` setelah zod, bukan di komponen. Store, model (`operations.ts`, `refs.ts`, `schema.ts`, `security.ts`, `examples.ts`, `search.ts`) dan semua komponen hanya melihat bentuk 3.x. | Satu titik pemetaan; komponen 3.x yang ada tidak bercabang per versi. Service tetap tanpa state (adapter murni). |
| FE2 | Adapter menulis `openapi: "3.0.3"` pada dokumen hasil adaptasi hanya agar memenuhi tipe `OpenApiDocument` di memori. Tidak ada komponen yang membaca `document.openapi` (sudah di-grep); versi yang ditampilkan selalu dari `spec.openapi_version`. Kunci asli `swagger` tidak disalin. | Tipe tunggal untuk komponen; tidak ada label versi palsu yang tampil. |
| FE3 | `$ref` `#/definitions/X` ditulis ulang menjadi `#/components/schemas/X` tanpa inline. `$ref` `#/parameters/X` dan `#/responses/X` **di dalam operasi** di-resolve satu langkah (salinan teradaptasi) karena bentuk 3.x-nya bergantung pada `consumes`/`produces` efektif operasi tersebut; salinan teradaptasi juga ditaruh di `components.parameters` / `components.responses` (memakai `consumes`/`produces` root) untuk kelengkapan. | Body/form tidak bisa ditentukan tanpa melihat `in`; MIME respons bergantung pada operasi. Skema tetap lewat `$ref` sehingga sirkular ditangani `deref` yang ada. |
| FE4 | Penulisan ulang `$ref` dan `x-nullable` sadar-skema: hanya menelusuri kunci skema (`properties`, `patternProperties`, `additionalProperties`, `items`, `allOf`, `anyOf`, `oneOf`, `not`). Nilai literal (`example`, `examples`, `x-example`, `default`, `enum`, `x-*`) disalin apa adanya. Properti bernama `example`/`default` tetap ditelusuri sebagai skema karena berada di bawah `properties`. | Sejalan dengan backend (refs.go melewati `examples` 2.0); contoh tidak diubah. |
| FE5 | `OpenApiBadge`: teks terlihat "Swagger 2.0" untuk `"2.0"` (aria-label "Swagger 2.0"); 3.x tidak berubah (teks "3.1", aria-label "Versi OpenAPI 3.1.0"). | "2.0" sendirian terbaca sebagai versi API di samping `api_version` (mis. `1.0.0`); kata "Swagger" membedakan format. |
| FE6 | Bila adapter tetap melempar (bug), service menangkapnya dan melempar `ApiError({status: 0, code: 'INVALID_RESPONSE'})`, sehingga view menampilkan `ErrorState` yang ada (dengan tombol coba lagi), bukan layar putih. Bagian tak dikenal di dokumen tidak melempar (K12): disalin apa adanya. | Error tetap di dalam blok yang gagal (app-screens 11). |

Yang TIDAK ada: route baru, feature folder baru, store baru, komponen baru, perubahan `downloadspec`, perubahan token/warna/radius/font di `app.css`, konversi yang dikirim ke server.

---

## 3. Plan (langkah berurutan)

Semua path relatif ke `frontend/` kecuali disebut. `GS` = `src/domain/project/api-spec/getspec`, `US` = `src/domain/project/api-spec/uploadspecs`.
Setiap langkah: tulis tes gagal dulu, jalankan (`npx vitest run <path>`), implementasi, jalankan lagi hijau, commit.

Prasyarat: backend foundation OV F1 sudah di branch yang sama (kontrak REST tidak berubah bentuk; FE bisa dikerjakan paralel dengan usecase backend sejak F1, OV "Setelah foundation"). Unit/komponen FE tidak bergantung pada backend; E2E (langkah 9) butuh backend T10 lengkap.

### Langkah 1 — Label versi (`src/shared/utils/openapi.ts`, baru)

```ts
export const SWAGGER2_VERSION = '2.0';
export const isSwagger2Version = (v: string | null | undefined): boolean => v === SWAGGER2_VERSION;
// "Swagger 2.0" untuk "2.0", selain itu "OpenAPI <v>". Satu-satunya tempat teks label versi dibentuk (K11).
export function openApiLabel(v: string): string;
```

- Tes `src/shared/utils/openapi.test.ts`: `"2.0"` -> `Swagger 2.0`; `"3.0.3"` -> `OpenAPI 3.0.3`; `"3.1.0"` -> `OpenAPI 3.1.0`; `"2.0.0"` -> `OpenAPI 2.0.0` (tidak pernah tersimpan, tapi tidak boleh dianggap Swagger); `isSwagger2Version(null)` -> false.
- "Swagger" dan "OpenAPI" adalah nama format, tidak diterjemahkan (sama di `id`/`en`), sehingga util ini tidak memakai i18n.
- Commit: `feat(project-api-spec): util label versi Swagger 2.0 / OpenAPI`.

### Langkah 2 — Badge dan teks hasil upload

1. `src/shared/components/OpenApiBadge.svelte`:
   - `isSwagger2Version(version)` -> teks `Swagger 2.0`, `title="Swagger 2.0"`, `aria-label="Swagger 2.0"`.
   - Selain itu perilaku lama persis (teks `3.1`, `title="3.1.0"`, aria-label `{$t('apispec.openapiVersion')} 3.1.0`).
   - Kelas tidak berubah: `inline-flex h-5 items-center rounded-full bg-pill-neutral-bg px-2 text-xs font-medium text-pill-neutral-ink`.
2. `US/components/ResultTable.svelte` baris 48: `· OpenAPI {r.openapi_version}` -> `· {openApiLabel(r.openapi_version)}` (import dari `shared/utils/openapi`).
3. Grep wajib setelah langkah ini: `grep -rn "OpenAPI {" frontend/src` dan `grep -rn "'OpenAPI '" frontend/src` hanya boleh menemukan `shared/utils/openapi.ts`.
4. Tes:
   - `src/domain/project/api-spec/listspecs/components/components.test.ts`: tambah `OpenApiBadge 2.0 menampilkan "Swagger 2.0" dengan aria-label Swagger 2.0`; tes lama `3.1.0` tidak diubah.
   - `GS/components/components.test.ts`: `SpecHeader` dengan `openapi_version: "2.0"` memuat label `Swagger 2.0`; tes lama 3.0.3 tidak diubah.
   - `US/components/components.test.ts`: `ResultTable` hasil `created` 2.0 menulis `Swagger Petstore · Swagger 2.0 · 1.0.0 · 4 endpoint`; hasil 3.x tetap `· OpenAPI 3.0.3 ·`.
5. Commit: `feat(project-api-spec): badge dan hasil upload menulis "Swagger 2.0"`.

### Langkah 3 — Adapter `GS/model/swagger2.ts` (baru) + mock

Ekspor publik internal feature (tidak diekspor dari `GS/index.ts`):

```ts
import type { Obj } from './guards';
export function isSwagger2(doc: unknown): boolean;   // objek, tanpa kunci `openapi`, `swagger` === "2.0" atau 2
export function adaptSwagger2(doc: Obj): Obj;        // murni, tidak memutasi, tidak melempar untuk bentuk tak dikenal
```

Disusun dari fungsi kecil per aturan (semua memakai `asObj/asArr/asStr/isObj` dari `guards.ts`):

| Fungsi | Aturan |
|---|---|
| `rewriteRef(ref)` | `#/definitions/` -> `#/components/schemas/`, `#/parameters/` -> `#/components/parameters/`, `#/responses/` -> `#/components/responses/`. Sisa segmen (termasuk `~1`, `~0`) tidak diubah. Ref lain (mis. `#/paths/...`, `#`) dibiarkan. |
| `adaptSchema(node)` | Sadar-skema (FE4). `$ref` -> `rewriteRef`. `x-nullable: true` -> tambah `nullable: true` (kunci `x-nullable` tetap). `type: "file"` -> `type: "string", format: "binary"`. Telusuri `properties`/`patternProperties` (map skema), `additionalProperties` (skema atau boolean), `items` (skema atau array), `allOf`/`anyOf`/`oneOf` (array), `not`. `discriminator` (string di 2.0) dan `collectionFormat` lewat apa adanya (K7). Nilai non-objek dikembalikan apa adanya. |
| `paramSchema(p)` | Parameter non-body 2.0 menyimpan tipe di level parameter. Pindahkan `type, format, items, enum, default, maximum, exclusiveMaximum, minimum, exclusiveMinimum, maxLength, minLength, pattern, maxItems, minItems, uniqueItems, multipleOf, collectionFormat` ke `schema` (lalu `adaptSchema`). `x-example` -> `example`. Sisa kunci (`name, in, description, required, allowEmptyValue, x-*`) tetap di parameter. |
| `effective(op, root, key)` | `consumes`/`produces`: operasi punya array -> itu (termasuk `[]`); selain itu root; selain itu `[]`. |
| `resolveLocal(doc, node, prefix)` | Bila `node.$ref` diawali `#/parameters/` atau `#/responses/`, ambil target dari dokumen ASLI lewat `resolvePointer` (`refs.ts`), satu langkah, dengan penjaga rantai (maks 16 lompatan, sama dengan `MAX_HOPS`). Tidak ketemu -> kembalikan node dengan `$ref` ditulis ulang (UI menampilkan apa adanya). |
| `adaptOperation(doc, item, op)` | Gabung parameter path-level + operasi dengan kunci `in:name` (operasi menang, aturan `buildInfo`), setelah `resolveLocal`. `in: body` (maks satu) -> `requestBody` = `{ description, required: p.required === true, content: { <mime>: { schema: adaptSchema(p.schema) } } }` untuk tiap MIME di `consumes` efektif; `consumes` kosong -> `application/json`. `in: formData` -> `requestBody.content[<ct>].schema = { type: "object", properties: { <name>: paramSchema(p).schema + description }, required: [...] }`, `<ct>` = `multipart/form-data` bila ada `type: file`, selain itu `application/x-www-form-urlencoded` (K6). Parameter lain -> `paramSchema`. Body/formData dihapus dari daftar `parameters` operasi DAN path-level di salinan. `consumes`/`produces` operasi dihapus dari salinan setelah dipakai. `security`, `deprecated`, `tags`, `summary`, `description`, `operationId`, `externalDocs`, `x-*` disalin. |
| `adaptResponses(doc, op, produces)` | Tiap kode: `resolveLocal`; `{ description, headers: tiap header -> { description, schema: paramSchema(h).schema } }`; `schema` ada -> `content[<mime>] = { schema }` untuk tiap MIME di `produces` efektif (kosong -> `application/json`); `examples` (peta MIME -> nilai literal) -> `content[<mime>].example = nilai` (MIME yang tidak ada di `produces` tetap ditambahkan, dengan `schema` bila ada). Tanpa `schema` dan tanpa `examples` -> tanpa `content` (mis. 204). |
| `adaptSecurity(defs)` | `basic` -> `{ type: "http", scheme: "basic", description }`; `apiKey` disalin; `oauth2` + `flow`: `implicit` -> `flows.implicit {authorizationUrl, scopes}`, `password` -> `flows.password {tokenUrl, scopes}`, `application` -> `flows.clientCredentials {tokenUrl, scopes}`, `accessCode` -> `flows.authorizationCode {authorizationUrl, tokenUrl, scopes}`; `flow` tak dikenal -> `flows[<flow>] = { authorizationUrl?, tokenUrl?, scopes }`; `type` tak dikenal -> salin apa adanya. |
| `swagger2Servers(root)` | OV D10, sama persis dengan backend: per scheme string (urutan dokumen, duplikat dibuang) `{ url: "<scheme>://<host><basePath>" }`; `schemes` kosong/tidak ada -> `["https"]`; `host` kosong/bukan string -> `[{ url: basePath }]`; `basePath` kosong/bukan string -> `"/"`. Tidak pernah array kosong. |
| `adaptSwagger2(doc)` | Root baru: `openapi: "3.0.3"` (FE2), `info`, `tags`, `externalDocs`, `security`, `x-*` disalin; `servers = swagger2Servers(root)`; `paths` = tiap path item: `parameters` non-body (adaptasi `paramSchema`, `$ref` ditulis ulang) + tiap method di `get, put, post, delete, options, head, patch` -> `adaptOperation`; kunci lain path item (`x-*`, `trace`, `$ref`) disalin apa adanya (`trace` tidak muncul di daftar endpoint backend sehingga tidak bisa dipilih). `components.schemas` = `definitions` lewat `adaptSchema`; `components.parameters` = `parameters` root (non-body via `paramSchema`; body/formData disalin apa adanya); `components.responses` = `responses` root lewat `adaptResponses` dengan `produces` root; `components.securitySchemes` = `adaptSecurity(securityDefinitions)`. Kunci `swagger, host, basePath, schemes, consumes, produces, definitions, parameters, responses, securityDefinitions` tidak disalin. |

Mock baru `GS/mocks/detail-swagger2.ts` (data contoh di bagian "Visual" butir 5): `mockDocumentSwagger2`, `mockEndpointsSwagger2`, `mockDetailSwagger2()`; plus `mockDocumentSwagger2Xss` di `GS/mocks/detail-xss.ts` (deskripsi berisi `<img onerror>`, `<script>`, `javascript:` di `info.description`, deskripsi body param, deskripsi `securityDefinitions`, `externalDocs.url`, `host` berisi `javascript:` ).

Tes `GS/model/swagger2.test.ts` (nama tes = aturan; lihat bagian 6.1). Commit: `feat(project-api-spec): adapter Swagger 2.0 ke bentuk tampilan di frontend`.

### Langkah 4 — Zod (`GS/types.ts`)

```ts
// 3.x: tidak berubah.
export const OpenApiDocumentSchema = z.object({ openapi: z.string(), info: z.object({ title: z.string(), version: z.string() }).passthrough() }).passthrough();
// Swagger 2.0 (T10). JSON tersimpan: "2.0" (string) atau 2 (YAML `swagger: 2.0` / JSON `2`).
export const Swagger2DocumentSchema = z
  .object({ swagger: z.union([z.literal('2.0'), z.literal(2)]), info: z.object({ title: z.string(), version: z.string() }).passthrough() })
  .passthrough();
export const SpecDocumentSchema = z.union([OpenApiDocumentSchema, Swagger2DocumentSchema]); // 3.x dicoba dulu (OV D2)
// Bentuk kawat GET detail.
export const ApiSpecDetailSchema = z.object({ spec: ApiSpecSchema, endpoints: z.array(ApiSpecEndpointSchema), document: SpecDocumentSchema });
export type ApiSpecDetailWire = z.infer<typeof ApiSpecDetailSchema>;
// Bentuk yang dipakai store dan komponen: dokumen selalu berbentuk 3.x (2.0 sudah diadaptasi di service).
export type ApiSpecDetail = Omit<ApiSpecDetailWire, 'document'> & { document: OpenApiDocument };
```

- `HTTP_METHODS` tidak diubah (endpoint 2.0 dari backend tidak pernah `TRACE`, D5).
- Tes `GS/types.test.ts`: dokumen 2.0 string dan angka lolos; `swagger: "1.2"`, `swagger: "2.0.0"`, `swagger: null`, tanpa `info.title` -> gagal; dokumen ber-`openapi` + `swagger` lolos lewat cabang 3.x; tes 3.x lama tidak diubah.
- Commit bersama langkah 5.

### Langkah 5 — Service (`GS/services/getspec.service.ts`)

```ts
export async function getSpec(projectId: string, name: string, signal?: AbortSignal): Promise<ApiSpecDetail> {
  const wire = await get(url, { schema: ApiSpecDetailSchema, signal });
  if (!isSwagger2(wire.document)) return wire as ApiSpecDetail; // 3.x: referensi sama
  try {
    return { ...wire, document: adaptSwagger2(wire.document) as OpenApiDocument };
  } catch {
    throw new ApiError({ status: 0, code: 'INVALID_RESPONSE', message: tr('error.invalid_response') }); // FE6
  }
}
```

(`tr` = penerjemah yang sudah dipakai `http.ts`; ikuti cara `http.ts` mengambil pesan `error.invalid_response`.)

- Tes `GS/services/getspec.service.test.ts`: lihat 6.2. Tes lama (`mockDetail()` 3.x, `INVALID_RESPONSE` untuk body kosong, 404) tidak diubah.
- Commit: `feat(project-api-spec): getSpec menerima dan memetakan dokumen Swagger 2.0`.

### Langkah 6 — Verifikasi tampilan detail 2.0 (tanpa perubahan komponen bila tes hijau)

Tes `GS/views/SpecDetailView.test.ts` dan `GS/security.test.ts` (6.3) dengan `mockDetailSwagger2()`. Bila tes menunjukkan komponen tidak menampilkan sesuatu yang sudah benar di dokumen hasil adapter, perbaiki adapter dulu; ubah komponen hanya bila bentuk 3.x yang sah memang belum ditangani (contoh yang dicek: `nullable` pada `typeLabel` sudah ada; `format: binary` tampil `string (binary)`; `allOf` tampil di `schemaChildren`). Catat setiap perubahan komponen di commit.

Commit: `test(project-api-spec): detail Swagger 2.0 di SpecDetailView dan sanitasi`.

### Langkah 7 — Balik tes upload yang mengunci penolakan 2.0 + teks kosong

1. `US/mocks/results.ts`:
   - `mockRejected` diganti menjadi penolakan versi yang masih nyata: `file: 'legacy.yaml'`, `reason: 'versi Swagger "1.2" tidak didukung (hanya 2.0)'`, issue `{ code: 'UNSUPPORTED_VERSION', message: 'versi Swagger "1.2" tidak didukung (hanya 2.0) (baris 1, #/swagger)', pointer: '#/swagger', line: 1 }`.
   - Tambah `mockSwagger2Created()`: `file: 'petstore.yaml'`, `source_path: 'specs/petstore.yaml'`, `name: 'petstore'`, `status: 'created'`, `openapi_version: '2.0'`, `title: 'Swagger Petstore'`, `api_version: '1.0.0'`, `endpoint_count: 4`.
   - Tambah `mockSwagger2Unresolved()`: `broken.yaml`, `UNRESOLVED_REF`, pointer `#/paths/~1pets/get/responses/200/schema/$ref`, `line: 21` (teks persis disalin dari keluaran tes integration backend saat tersedia; lihat `usecase-uploadspecs.md` bagian 8).
2. `US/model.test.ts`: ekspektasi `issueText(mockRejected())` menjadi `legacy.yaml [rejected/INVALID_SPEC]: versi Swagger "1.2" tidak didukung (hanya 2.0)\n  - UNSUPPORTED_VERSION versi Swagger "1.2" tidak didukung (hanya 2.0) (baris 1, #/swagger)`.
3. `US/components/components.test.ts`: `expect.stringContaining('NOT_OPENAPI_3')` -> `'UNSUPPORTED_VERSION'`; tambah tes `ResultTable` untuk `mockSwagger2Created` (label `Swagger 2.0`, tautan nama setelah upload) dan `mockSwagger2Unresolved` (pointer dan baris tampil di blok issue).
4. `US/views/UploadSpecsDialog.test.ts`: `T06-AC2` mengharapkan `UNSUPPORTED_VERSION`; tambah `T10-AC1: upload Swagger 2.0 tampil created dengan label Swagger 2.0 dan memanggil onuploaded sekali`; tambah `T10: validasi Swagger 2.0 tampil valid tanpa onuploaded`.
5. i18n `src/shared/i18n/id.json` dan `en.json`:
   - `apispec.list.emptyHint` id: `Unggah file OpenAPI 3.0/3.1 atau Swagger 2.0 (.yaml, .yml, .json) satu per satu atau satu folder sekaligus.`; en: `Upload OpenAPI 3.0/3.1 or Swagger 2.0 files (.yaml, .yml, .json) one by one or a whole folder.`
   - `apispec.intro` id: `Spec OpenAPI atau Swagger yang dibaca agent untuk mengetahui endpoint project ini.`; en: `OpenAPI or Swagger specs that agents read to learn this project's endpoints.`
   - Kunci lain (`apispec.reason.INVALID_SPEC` "Dokumen OpenAPI tidak valid", `apispec.openapiVersion`) tidak diubah agar tes 3.x tetap. Tidak ada kunci `apispec.issue.NOT_OPENAPI_3` di repo saat ini (sudah di-grep); K10 tidak memerlukan tindakan FE.
   - Tes paritas kunci i18n yang ada (bila ada) tetap hijau.
6. Commit: `test(project-api-spec): balik tes FE penolakan Swagger 2.0`.

### Langkah 8 — Lint arsitektur, desain, dan seluruh tes FE

```
cd frontend
npx vitest run src/domain/project/api-spec src/shared src/app
npm run lint && npm run check          # svelte-check / tsc sesuai script yang ada
npm run depcruise && npm run test:arch
cd .. && node .agents/skills/design-taste-frontend/scripts/lint.mjs --app frontend/src
```

`depcruise`: `getspec/model/swagger2.ts` hanya mengimpor `./guards`, `./refs`; `ResultTable` dan `OpenApiBadge` hanya mengimpor `shared/utils/openapi`. Tidak ada import feature-ke-feature.

### Langkah 9 — E2E

Fixture (file nyata, dibaca dengan `fs.readFileSync`):

| File | Pemilik | Isi |
|---|---|---|
| `e2e/fixtures/apispec/swagger-2.0.yaml` | plan `uploadspecs` langkah 6 | petstore 2.0 valid: `/pets` GET/POST (POST: `in: body` `$ref: '#/definitions/Pet'`, `security: [{api_key: []}]`), `/pets/{petId}` GET/DELETE (DELETE `deprecated: true`), `definitions` Pet/Error, `host: petstore.example.com`, `basePath: /v1`, `schemes: [https]`, `securityDefinitions.api_key` apiKey header `X-API-Key` |
| `e2e/fixtures/apispec/swagger-2.0.json` | plan ini | isi sama dalam JSON (`"swagger": "2.0"`) |
| `e2e/fixtures/apispec/swagger-2.0-unresolved-ref.yaml` | plan ini | `$ref: '#/definitions/Hilang'` |
| `e2e/fixtures/apispec/swagger-1.2.yaml` | plan ini | `swagger: "1.2"` |

Helper `e2e/helpers/apispecs.ts`: tambah `readSpecFixture(name): SpecFile` dan `uploadSpecsRaw(request, projectId, files)` (sama seperti `uploadSpecsApi` tetapi tidak meng-assert `rejected == 0`, untuk kasus negatif). `uploadSpecsApi` lama tidak diubah.

Spec baru `e2e/api-spec-swagger2.spec.ts` (skenario di bagian 7) dan satu tes `P13` di `e2e/api-spec-tools.spec.ts` (MCP `list_api_specs` menulis `Swagger 2.0`, `get_api_endpoint` 2.0 memuat `openapi_version`). Asersi teks MCP 3.x yang ada (baris 182-184) tidak diubah.

Jalankan: backend dev (`make dev`, `make seed`), lalu `cd frontend && npx playwright test e2e/api-spec-swagger2.spec.ts e2e/api-spec-tools.spec.ts`.

Commit: `test(project-api-spec): e2e Swagger 2.0 (upload, detail, MCP)`.

---

## 4. Migration

Tidak ada. FE tidak menyentuh basis data; kolom `api_specs.openapi_version` sudah menyimpan `"2.0"` apa adanya (OV 6).

---

## 5. Changes (per file/folder)

| File | Perubahan |
|---|---|
| `src/shared/utils/openapi.ts` (baru) | `SWAGGER2_VERSION`, `isSwagger2Version`, `openApiLabel` |
| `src/shared/utils/openapi.test.ts` (baru) | tes label |
| `src/shared/components/OpenApiBadge.svelte` | cabang 2.0: teks/title/aria-label "Swagger 2.0"; 3.x identik |
| `src/shared/i18n/id.json`, `en.json` | `apispec.list.emptyHint`, `apispec.intro` menyebut Swagger 2.0 |
| `GS/model/swagger2.ts` (baru) | `isSwagger2`, `adaptSwagger2` + fungsi aturan |
| `GS/model/swagger2.test.ts` (baru) | tes adapter (6.1) |
| `GS/mocks/detail-swagger2.ts` (baru) | mock dokumen/endpoint/detail 2.0 (data contoh "Visual" butir 5) |
| `GS/mocks/detail-xss.ts` | tambah `mockDocumentSwagger2Xss`, `mockDetailSwagger2Xss` |
| `GS/types.ts` | `Swagger2DocumentSchema`, `SpecDocumentSchema`, `ApiSpecDetailSchema` memakai union, tipe `ApiSpecDetailWire`; `ApiSpecDetail` tetap bertipe dokumen 3.x |
| `GS/types.test.ts` | kasus 2.0 |
| `GS/services/getspec.service.ts` | adaptasi setelah zod; FE6 |
| `GS/services/getspec.service.test.ts` | kasus 2.0 (6.2) |
| `GS/views/SpecDetailView.test.ts` | kasus 2.0 (6.3) |
| `GS/security.test.ts` | sanitasi 2.0 |
| `GS/components/components.test.ts` | `SpecHeader` 2.0 |
| `GS/components/*.svelte`, `GS/model/{operations,refs,schema,security,examples,search}.ts`, `GS/stores/*`, `GS/index.ts` | TIDAK diubah (kecuali celah yang terbukti di langkah 6, dicatat di commit) |
| `US/components/ResultTable.svelte` | `openApiLabel` |
| `US/mocks/results.ts` | `mockRejected` jadi `UNSUPPORTED_VERSION`; `mockSwagger2Created`, `mockSwagger2Unresolved` |
| `US/model.test.ts`, `US/components/components.test.ts`, `US/views/UploadSpecsDialog.test.ts` | dibalik + tes 2.0 |
| `src/domain/project/api-spec/listspecs/components/components.test.ts` | badge 2.0 |
| `src/domain/project/api-spec/listspecs/mocks/specs.ts` | tambah satu baris spec 2.0 (`hr-platform`) untuk tes tabel |
| `e2e/fixtures/apispec/swagger-2.0.json`, `swagger-2.0-unresolved-ref.yaml`, `swagger-1.2.yaml` (baru) | fixture e2e |
| `e2e/helpers/apispecs.ts` | `readSpecFixture`, `uploadSpecsRaw` |
| `e2e/api-spec-swagger2.spec.ts` (baru) | skenario bagian 7 |
| `e2e/api-spec-tools.spec.ts` | `P13` Swagger 2.0 via MCP |
| `src/domain/project/api-spec/downloadspec/**`, `renamespec/**`, `deletespec/**`, `src/app/**`, `src/app.css`, `DESIGN.md` | TIDAK diubah |

---

## 6. Test Scenario (unit / komponen, Vitest + Testing Library)

### 6.1 `GS/model/swagger2.test.ts`

Semua tes memakai `mockDocumentSwagger2` atau dokumen inline kecil; dokumen masukan dibekukan dalam (`deepFreeze`) untuk membuktikan tidak dimutasi.

- `isSwagger2`: `{swagger: "2.0"}` dan `{swagger: 2}` -> true; `{openapi: "3.0.3"}` -> false; `{openapi: "3.0.3", swagger: "2.0"}` -> false (OV D2); `{swagger: "1.2"}`, `null`, array -> false.
- `adaptSwagger2 menulis ulang $ref definitions/parameters/responses`; `$ref` dengan `~1` tetap; `$ref` `#/paths/...` dibiarkan.
- `tidak menulis ulang $ref di dalam example/examples/x-example/default/enum`; `properti bernama "example" tetap ditelusuri sebagai skema`.
- `body tanpa consumes -> application/json`; `body dengan consumes dua MIME -> dua entri content dengan schema sama`; `consumes operasi menimpa root`; `consumes operasi [] -> application/json`.
- `body.required dan description pindah ke requestBody`; `body tidak lagi ada di parameters operasi maupun path-level`.
- `formData tanpa file -> application/x-www-form-urlencoded dengan properties + required`; `formData dengan type file -> multipart/form-data, file menjadi string binary`.
- `parameter query/header/path: type, format, enum, items, collectionFormat pindah ke schema`; `x-example -> example`.
- `parameter path-level via $ref '#/parameters/TenantHeader' di-resolve dan digabung; operasi menang untuk in+name sama`.
- `parameter $ref ke body di '#/parameters/X' menjadi requestBody`.
- `respons dengan schema -> content per MIME di produces`; `produces kosong -> application/json`; `respons tanpa schema -> tanpa content (204)`; `respons examples -> example per MIME, termasuk MIME di luar produces`; `respons $ref '#/responses/NotFound' di-resolve dengan produces operasi`; `header respons type -> schema`.
- `x-nullable true -> nullable true`, termasuk di dalam `properties` bersarang dan `items`.
- `allOf + discriminator string lewat apa adanya`; `collectionFormat lewat apa adanya`.
- `securityDefinitions basic -> http basic`; `apiKey disalin`; `oauth2 implicit/password/application/accessCode -> flows implicit/password/clientCredentials/authorizationCode dengan scopes`; `oauth2 flow tak dikenal -> flows[flow]`; `type tak dikenal disalin`.
- Server (tabel, cermin `TestResolveOperation_Swagger2_Servers` backend): `schemes [http, https] + host + /v1` -> `http://h/v1`, `https://h/v1`; tanpa schemes -> `https://h/v1`; tanpa host -> `[{url: "/v1"}]`; tanpa basePath -> `https://h/`; tanpa host dan basePath -> `[{url: "/"}]`; schemes duplikat dibuang; schemes berisi non-string dilewati.
- `definitions sirkular tidak membuat adapter berulang`: `Node.children[] -> Node` selesai, hasil tetap berisi `$ref: '#/components/schemas/Node'`; lalu `schemaChildren` pada hasil menandai `circular: true` pada level kedua.
- `trace di path item disalin apa adanya dan tidak diadaptasi`.
- `bentuk tak dikenal tidak melempar` (tabel): `paths` berupa array, operasi berupa string, `parameters` berupa objek, `responses` null, `securityDefinitions` array, `definitions` angka -> tidak melempar, kunci disalin atau dilewati.
- `tidak memutasi masukan` (deepFreeze; `JSON.stringify` masukan sebelum/sesudah sama).
- `hasil memenuhi OpenApiDocumentSchema` (FE2) dan `info/tags/externalDocs/x-* disalin`.
- Kinerja: dokumen sintetis 2.0 dengan 600 path x 2 operasi, 1.500 definisi, 2.500 `$ref` (mendekati angka hr-platform) selesai tanpa error dan tanpa rekursi berlebih; waktu dicatat di output tes (tidak dijadikan asersi keras agar tidak flaky).

### 6.2 `GS/services/getspec.service.test.ts`

- `getSpec memetakan dokumen 2.0 sebelum dikembalikan`: respons `mockDetailSwagger2()` -> `document.openapi === "3.0.3"`, `document.components.schemas.Employee` ada, `document.swagger` tidak ada, `spec.openapi_version === "2.0"`, `endpoints` tidak berubah.
- `getSpec membiarkan dokumen 3.x identik` (`toBe` referensi dokumen hasil parse zod, `toEqual` dengan mock).
- `swagger angka 2 diterima`.
- `swagger "1.2" -> INVALID_RESPONSE` (zod gagal).
- `adapter melempar -> INVALID_RESPONSE` (spy `adaptSwagger2` melempar, FE6).
- Tes lama (URL ter-encode, 404) tetap.

### 6.3 `GS/views/SpecDetailView.test.ts`, `GS/security.test.ts`, komponen

`SpecDetailView` dengan fetch distub `mockDetailSwagger2()`:

- Loading: `aria-busy="true"` + `Spinner` tampil sebelum respons; tidak ada flash konten 3.x kosong.
- Header: judul `HR Platform API`, badge `Swagger 2.0`, `1.12.0`.
- Overview (tanpa `op`): server `https://api.hr-platform.noobee.id/v1` (mono, tautan aman); skema keamanan `bearerAuth · apiKey · header Authorization`, `basicAuth · http · basic`, `hrOauth · oauth2` dengan `authorizationCode`, `authorizationUrl`, `tokenUrl`, scope `employees:read`, `employees:write`; tag `employees`, `leave`.
- Sidebar: 5 endpoint, grup tag `employees` (3) dan `leave` (2); pencarian `leave` menyisakan 2.
- `?op=POST /employees`: Request body `wajib`, `application/json`, pohon skema `Employee` dengan `allOf[0]` (`Person`) dan properti `employeeNo`, `orgUnit`; `orgUnit.parent` bertanda sirkular; respons `201`, `400` (`Error`), keamanan `hrOauth` scope `employees:write`.
- `?op=POST /employees/{employeeId}/documents`: Parameter · path `employeeId` `string` wajib; Request body `multipart/form-data`, properti `file` `string (binary)` wajib, `category` `string`; respons `201`, `413`.
- `?op=GET /employees/{employeeId}`: respons `200` `application/json` dengan `terminatedAt` bertipe `string (date-time) | null` (dari `x-nullable`); respons `404` dari `#/responses/NotFound` dengan contoh JSON per MIME.
- `?op=GET /leave-requests`: Parameter · query `status` `array<string>`, Parameter · header `X-Tenant-ID` (dari `$ref` path-level) wajib; operasi deprecated (`line-through` + `Usang`).
- `?op=DELETE /employees/{employeeId}` (tidak ada di dokumen) -> EmptyState `opNotFound` dengan tautan ke overview.
- `?op=TRACE /employees` -> `parseOpParam` menerima, `getOperation` tidak menemukan -> EmptyState `opNotFound`.
- Error: 404 -> EmptyState "spec tidak ditemukan" + kembali ke daftar; 500 -> `ErrorState` + coba lagi memanggil fetch ulang; dokumen `swagger: "1.2"` -> `ErrorState` `INVALID_RESPONSE`.
- Empty: spec 2.0 dengan `paths: {}` dan `endpoints: []` -> sidebar menampilkan empty state yang ada (bingkai tetap), overview tetap tampil dengan server dan keamanan.

`GS/security.test.ts`: `mockDetailSwagger2Xss` -> `expectClean` lulus di overview dan operasi (Markdown disanitasi seperti 3.x, AC4); `host: "javascript:alert(1)"` menghasilkan URL server yang ditampilkan sebagai teks, bukan tautan; tidak ada `alert`.

Komponen: badge 2.0 (listspecs), `SpecHeader` 2.0, `ResultTable` 2.0 (langkah 2 dan 7).

### 6.4 `uploadspecs` (dibalik)

- `model.test.ts`: `issueText` untuk `UNSUPPORTED_VERSION`.
- `components.test.ts`: salin memuat `UNSUPPORTED_VERSION`; `created` 2.0 berlabel `Swagger 2.0`; `UNRESOLVED_REF` menampilkan `(baris 21, #/paths/~1pets/get/responses/200/schema/$ref)`.
- `UploadSpecsDialog.test.ts`: `T06-AC2` dengan `UNSUPPORTED_VERSION`; `T10-AC1` upload 2.0 -> tautan `petstore`, `onuploaded` sekali; validasi 2.0 -> status `Valid`, `onuploaded` tidak dipanggil.
- Seluruh tes 3.x lain tidak diubah.

### 6.5 Regresi

- Semua tes `getspec` 3.0/3.1/webhooks/circular/xss lama lulus tanpa diubah.
- `npm run depcruise`, `npm run test:arch`, `svelte-check` hijau; `lint.mjs --app` tanpa FAIL pada file yang diubah.

---

## 7. E2E Scenarios (`e2e/api-spec-swagger2.spec.ts`, Playwright, backend nyata)

Setup: login sebagai super admin seed, buat project `hr-platform-e2e` lewat API (pola helper yang ada), mode serial.

Positif:
1. **P1 upload UI 2.0:** buka `/projects/:id/api-specs`, "Unggah spec", pilih `fixtures/apispec/swagger-2.0.yaml`, "Unggah" -> baris hasil `Dibuat`, teks `Swagger Petstore · Swagger 2.0 · 1.0.0 · 4 endpoint`, tautan `swagger-2.0`.
2. **P2 daftar spec:** tabel menampilkan badge `Swagger 2.0` (aria-label `Swagger 2.0`) di baris spec itu; spec 3.0.3 yang diunggah lewat `petstoreYaml` di baris lain tetap badge `3.0`.
3. **P3 overview:** buka detail -> server `https://petstore.example.com/v1`, skema keamanan `api_key · apiKey · header X-API-Key`, 4 endpoint di sidebar urut `GET /pets`, `POST /pets`, `DELETE /pets/{petId}`, `GET /pets/{petId}` (urutan sidebar = aturan `buildTagTree`, sama dengan 3.x).
4. **P4 operasi berbody:** klik `POST /pets` -> URL `?op=POST%20%2Fpets`, Request body `application/json` dengan properti `id`, `name` wajib; keamanan `api_key`.
5. **P5 operasi deprecated:** `DELETE /pets/{petId}` tampil `Usang`, parameter path `petId` wajib.
6. **P6 JSON + unduh:** unggah `swagger-2.0.json` lewat API -> `created`; GET `/raw` -> byte sama dengan file fixture (`Buffer.equals`).
7. **P7 unggah ulang:** file YAML yang sama lewat UI -> `Tidak berubah`.
8. **P8 validasi saja:** "Validasi saja" dengan file 2.0 -> `Valid`, daftar spec tidak bertambah.
9. **P9 3.x tidak berubah:** detail spec 3.0.3 (`petstoreYaml`) menampilkan server `https://petstore.noobee.id/v1` dan badge `3.0` seperti sebelum T10.
10. **P13 di `api-spec-tools.spec.ts`:** setelah upload 2.0, `list_api_specs` memuat `| Swagger 2.0 |`; `get_api_endpoint(spec, POST, /pets)` memuat `"openapi_version":"2.0"`, `"consumes":["application/json"]`, `servers` `https://petstore.example.com/v1`; spec 3.x tetap tanpa tiga kunci itu.

Negatif:
1. **N1 ref hilang:** unggah `swagger-2.0-unresolved-ref.yaml` di UI -> `Ditolak`, `Dokumen OpenAPI tidak valid`, blok issue memuat `UNRESOLVED_REF`, pointer `#/...` dan `baris`; tombol "Salin" menyalin teks issue; daftar spec tidak bertambah.
2. **N2 versi tak didukung:** `swagger-1.2.yaml` -> `Ditolak`, issue `UNSUPPORTED_VERSION` dengan `#/swagger`.
3. **N3 campuran:** satu unggahan berisi `swagger-2.0.yaml` (sudah ada -> `Tidak berubah`), `swagger-1.2.yaml`, `README.md` -> ringkasan menampilkan hanya status di atas nol; baris lokal `README.md` `Dilewati`.
4. **N4 operasi tidak ada:** buka `?op=TRACE%20%2Fpets` pada spec 2.0 -> "Endpoint tidak ditemukan" dengan tautan ke overview (tanpa error konsol).
5. **N5 spec tidak ada:** `/projects/:id/api-specs/tidak-ada` -> empty state 404 dengan tautan kembali ke daftar.
6. **N6 isolasi:** user non-anggota membuka detail spec 2.0 -> 404 (empty state), tidak bocor judul.
7. **N7 tanpa permintaan keluar dari browser:** selama membuka detail spec 2.0 dengan `host: petstore.example.com`, `page.on('request')` tidak mencatat request ke host selain origin aplikasi (adapter dan tampilan tidak pernah fetch server/`$ref`).

---

## 8. Request/Response body

### GET detail Swagger 2.0 (`/api/projects/7d1f0c8e-2b4a-4c1e-9f3a-5e6d7c8b9a01/api-specs/hr-platform`) — kawat (dari backend, apa adanya)

```json
{
  "spec": {
    "name": "hr-platform",
    "title": "HR Platform API",
    "openapi_version": "2.0",
    "api_version": "1.12.0",
    "format": "yaml",
    "source_path": "api/openapi.yml",
    "size_bytes": 18432,
    "endpoint_count": 2,
    "created_at": "2026-10-21T09:00:00Z",
    "created_by_label": "Rina Pratiwi",
    "updated_at": "2026-10-21T09:00:00Z",
    "updated_by_label": "Rina Pratiwi"
  },
  "endpoints": [
    { "method": "POST", "path": "/employees", "operation_id": "createEmployee", "summary": "Tambah karyawan", "tags": ["employees"], "deprecated": false },
    { "method": "POST", "path": "/employees/{employeeId}/documents", "operation_id": "uploadEmployeeDocument", "summary": "Unggah dokumen karyawan", "tags": ["employees"], "deprecated": false }
  ],
  "document": {
    "swagger": "2.0",
    "info": { "title": "HR Platform API", "version": "1.12.0" },
    "host": "api.hr-platform.noobee.id",
    "basePath": "/v1",
    "schemes": ["https"],
    "consumes": ["application/json"],
    "produces": ["application/json"],
    "tags": [{ "name": "employees", "description": "Data karyawan" }],
    "securityDefinitions": {
      "hrOauth": {
        "type": "oauth2",
        "flow": "accessCode",
        "authorizationUrl": "https://auth.hr-platform.noobee.id/oauth/authorize",
        "tokenUrl": "https://auth.hr-platform.noobee.id/oauth/token",
        "scopes": { "employees:read": "Baca karyawan", "employees:write": "Ubah karyawan" }
      }
    },
    "security": [{ "hrOauth": ["employees:read"] }],
    "paths": {
      "/employees": {
        "post": {
          "operationId": "createEmployee",
          "summary": "Tambah karyawan",
          "tags": ["employees"],
          "security": [{ "hrOauth": ["employees:write"] }],
          "parameters": [
            { "name": "body", "in": "body", "required": true, "schema": { "$ref": "#/definitions/Employee" } }
          ],
          "responses": {
            "201": { "description": "Karyawan dibuat", "schema": { "$ref": "#/definitions/Employee" } },
            "400": { "description": "Input tidak valid", "schema": { "$ref": "#/definitions/Error" } }
          }
        }
      },
      "/employees/{employeeId}/documents": {
        "parameters": [{ "name": "employeeId", "in": "path", "required": true, "type": "string" }],
        "post": {
          "operationId": "uploadEmployeeDocument",
          "summary": "Unggah dokumen karyawan",
          "tags": ["employees"],
          "consumes": ["multipart/form-data"],
          "parameters": [
            { "name": "file", "in": "formData", "required": true, "type": "file" },
            { "name": "category", "in": "formData", "type": "string", "enum": ["ktp", "npwp", "kontrak"] }
          ],
          "responses": { "201": { "description": "Dokumen tersimpan" } }
        }
      }
    },
    "definitions": {
      "Employee": {
        "type": "object",
        "required": ["employeeNo", "fullName"],
        "properties": {
          "employeeNo": { "type": "string", "example": "EMP-00412" },
          "fullName": { "type": "string" },
          "terminatedAt": { "type": "string", "format": "date-time", "x-nullable": true }
        }
      },
      "Error": { "type": "object", "properties": { "code": { "type": "string" }, "message": { "type": "string" } } }
    }
  }
}
```

### Dokumen yang diterima komponen (hasil `adaptSwagger2`, hanya di memori, tidak pernah dikirim)

```json
{
  "openapi": "3.0.3",
  "info": { "title": "HR Platform API", "version": "1.12.0" },
  "servers": [{ "url": "https://api.hr-platform.noobee.id/v1" }],
  "tags": [{ "name": "employees", "description": "Data karyawan" }],
  "security": [{ "hrOauth": ["employees:read"] }],
  "paths": {
    "/employees": {
      "post": {
        "operationId": "createEmployee",
        "summary": "Tambah karyawan",
        "tags": ["employees"],
        "security": [{ "hrOauth": ["employees:write"] }],
        "parameters": [],
        "requestBody": {
          "required": true,
          "content": { "application/json": { "schema": { "$ref": "#/components/schemas/Employee" } } }
        },
        "responses": {
          "201": { "description": "Karyawan dibuat", "content": { "application/json": { "schema": { "$ref": "#/components/schemas/Employee" } } } },
          "400": { "description": "Input tidak valid", "content": { "application/json": { "schema": { "$ref": "#/components/schemas/Error" } } } }
        }
      }
    },
    "/employees/{employeeId}/documents": {
      "parameters": [{ "name": "employeeId", "in": "path", "required": true, "schema": { "type": "string" } }],
      "post": {
        "operationId": "uploadEmployeeDocument",
        "summary": "Unggah dokumen karyawan",
        "tags": ["employees"],
        "parameters": [],
        "requestBody": {
          "required": true,
          "content": {
            "multipart/form-data": {
              "schema": {
                "type": "object",
                "required": ["file"],
                "properties": {
                  "file": { "type": "string", "format": "binary" },
                  "category": { "type": "string", "enum": ["ktp", "npwp", "kontrak"] }
                }
              }
            }
          }
        },
        "responses": { "201": { "description": "Dokumen tersimpan" } }
      }
    }
  },
  "components": {
    "schemas": {
      "Employee": {
        "type": "object",
        "required": ["employeeNo", "fullName"],
        "properties": {
          "employeeNo": { "type": "string", "example": "EMP-00412" },
          "fullName": { "type": "string" },
          "terminatedAt": { "type": "string", "format": "date-time", "x-nullable": true, "nullable": true }
        }
      },
      "Error": { "type": "object", "properties": { "code": { "type": "string" }, "message": { "type": "string" } } }
    },
    "parameters": {},
    "responses": {},
    "securitySchemes": {
      "hrOauth": {
        "type": "oauth2",
        "flows": {
          "authorizationCode": {
            "authorizationUrl": "https://auth.hr-platform.noobee.id/oauth/authorize",
            "tokenUrl": "https://auth.hr-platform.noobee.id/oauth/token",
            "scopes": { "employees:read": "Baca karyawan", "employees:write": "Ubah karyawan" }
          }
        }
      }
    }
  }
}
```

`requestBody.required` bernilai `true` hanya bila parameter body (atau salah satu parameter formData) `required: true`; selain itu kunci tidak ditulis. Array `required` pada skema form hanya berisi nama parameter formData yang wajib. Daftar `parameters` operasi menjadi `[]` setelah body/formData dipindah.

### Upload / validate (bentuk tidak berubah; contoh lengkap di `usecase-uploadspecs.md` bagian 8)

Satu baris hasil 2.0 `created` yang dipakai mock FE `mockSwagger2Created()`:

```json
{
  "file": "petstore.yaml",
  "source_path": "specs/petstore.yaml",
  "name": "petstore",
  "status": "created",
  "reason_code": null,
  "reason": null,
  "issues": [],
  "openapi_version": "2.0",
  "title": "Swagger Petstore",
  "api_version": "1.0.0",
  "endpoint_count": 4
}
```

Baris penolakan versi (`mockRejected()` setelah dibalik):

```json
{
  "file": "legacy.yaml",
  "source_path": "legacy.yaml",
  "name": "legacy",
  "status": "rejected",
  "reason_code": "INVALID_SPEC",
  "reason": "versi Swagger \"1.2\" tidak didukung (hanya 2.0)",
  "issues": [
    { "code": "UNSUPPORTED_VERSION", "message": "versi Swagger \"1.2\" tidak didukung (hanya 2.0) (baris 1, #/swagger)", "pointer": "#/swagger", "line": 1 }
  ],
  "openapi_version": null,
  "title": null,
  "api_version": null,
  "endpoint_count": null
}
```

### Format error

Envelope server (tidak berubah), contoh spec tidak ditemukan / bukan anggota (404):

```json
{
  "error": { "code": "NOT_FOUND", "message": "spec tidak ditemukan", "details": null },
  "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33"
}
```

Upload tanpa file (400):

```json
{
  "error": { "code": "INVALID_ARGUMENT", "message": "tidak ada file", "details": { "reason": "NO_FILES" } },
  "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33"
}
```

Error sisi klien (bukan respons server): dokumen gagal zod (mis. `swagger: "1.2"` tersimpan dari data lama) atau adapter melempar (FE6) -> `ApiError` `{ status: 0 | 200, code: "INVALID_RESPONSE", message: "<error.invalid_response>" }`, ditampilkan `ErrorState` dengan tombol coba lagi.

---

## 9. Visual

Tidak ada layar baru. T10 mengisi tiga layar yang sudah ada dengan data 2.0; semua memakai pola dan token DESIGN.md tanpa tambahan.

### 9.1 Pertanyaan per layar dan blok terbesar

| Layar | Pertanyaan | Blok terbesar yang menjawab |
|---|---|---|
| Tab API Spec, daftar (`/projects/:id/api-specs`) | Spec mana yang tersedia, dan formatnya apa? | tabel spec (panel radius 8); kolom identitas nama + judul; badge versi `Swagger 2.0` / `3.1` di kolom versi |
| Detail spec, overview (`/projects/:id/api-specs/:name`) | Endpoint apa yang tersedia di API ini? | sidebar endpoint (method pil netral + path mono, per tag) di kiri; overview (server, skema keamanan, tag) di kanan |
| Detail operasi (`?op=METHOD /path`) | Bagaimana memanggil endpoint ini (parameter, body, respons, auth)? | panel operasi: judul method + path mono, lalu Request body (MIME mono + pohon skema) dan Response |
| Dialog unggah | File mana yang masuk, dan kenapa yang lain ditolak? | daftar hasil per file (status pil + path mono + label versi + blok issue mono dengan pointer dan baris) |

Temuan utama per layar ditaruh di posisi yang dibaca pertama: di dialog unggah, baris `Ditolak` memuat kode issue + pointer + baris (mis. `UNRESOLVED_REF ... (baris 21, #/paths/~1pets/get/responses/200/schema/$ref)`); di header detail, badge `Swagger 2.0` di samping judul menjawab "format apa" tanpa banner.

### 9.2 Pola DESIGN.md dan kerangka

- Kerangka: `AppLayout` (sidebar 228px di `bg-ground`, sheet putih radius 16) + `PageHeader`/title row workspace + tab garis bawah 2px `border-accent` (Tab API Spec). Tidak berubah.
- Daftar spec: pola **list** (title row + toolbar + tabel dalam panel, baris 48, header pita `bg-raised`, rule `border-rule`).
- Detail spec: pola **list -> detail** baris "Tab API Spec" DESIGN.md (dua kolom `md:grid-cols-[18rem_1fr]`, sidebar endpoint + panel baca). Di ponsel sidebar terlipat dalam `<details>` (sudah ada).
- Dialog unggah: `Dialog` (`shadow-pop`) dengan satu tombol utama "Unggah" (`bg-primary`), "Validasi saja" sekunder (`border-control`).
- Tidak ada pola settings yang terlibat.

### 9.3 Elemen khas produk di layar pertama

- **Mono untuk satu pekerjaan** (DESIGN.md "Wajah produk" 1): method + path endpoint, `operationId`, URL server hasil `host/basePath/schemes`, MIME (`application/json`, `multipart/form-data`), nama skema keamanan, pointer issue (`#/paths/~1pets/...`). Label badge "Swagger 2.0" bukan mono (itu label, bukan data mesin).
- Badge versi tetap pil netral `bg-pill-neutral-bg text-pill-neutral-ink` (bukan status, jadi tanpa warna); versi 2.0 tidak diberi warna peringatan karena 2.0 adalah format yang didukung, bukan masalah.
- Status di dialog memakai pil status yang ada (`StatusBadge`: positive/negative/neutral) selalu dengan teks.
- Jejak agent tidak muncul di layar ini (tidak ada baris dari agent); tidak ditambah.

### 9.4 Token / utility yang dipakai (tanpa nilai hex baru)

`bg-ground`, `bg-surface`, `bg-raised` (blok issue, header tabel), `border-border`, `border-rule`, `border-control`, `rounded-base` (6), `rounded-card` (8), `rounded-full` (pil), `shadow-panel` (tabel), `shadow-pop` (dialog), `text-fg`, `text-muted`, `text-warning` (`wajib`, `Usang`), `text-accent-ink` (tautan, cincin fokus `ring-accent-ink`), `bg-pill-neutral-bg`/`text-pill-neutral-ink` (badge versi, method), `font-mono`, `text-[13px]`, `text-xs`, `text-sm`, `text-base`, `tabular-nums` (jumlah endpoint). Tidak ada warna, radius, font, bayangan, atau komponen dekoratif baru.

### 9.5 Data contoh (mock `GS/mocks/detail-swagger2.ts`, dipakai tes komponen/view)

Spec `hr-platform` (mewakili file nyata pemilik), judul `HR Platform API`, `api_version` `1.12.0`, `format: yaml`, `source_path: api/openapi.yml`, `size_bytes: 18432`, diunggah oleh `Rina Pratiwi`, `host: api.hr-platform.noobee.id`, `basePath: /v1`, `schemes: [https]`. `endpoint_count = endpoints.length = 5`:

| Method | Path | operationId | Tag | Isi yang diuji |
|---|---|---|---|---|
| GET | `/employees/{employeeId}` | `getEmployee` | employees | path param; respons 200 `Employee` (`terminatedAt` `x-nullable`); 404 via `#/responses/NotFound` dengan `examples` `application/json` `{ "code": "EMPLOYEE_NOT_FOUND", "message": "Karyawan EMP-00412 tidak ditemukan" }` |
| POST | `/employees` | `createEmployee` | employees | body `$ref: '#/definitions/Employee'` (`allOf` `Person` + `discriminator: kind`), `orgUnit` -> `OrgUnit.parent` sirkular; security `hrOauth` `employees:write` |
| POST | `/employees/{employeeId}/documents` | `uploadEmployeeDocument` | employees | `consumes: [multipart/form-data]`, formData `file` (`type: file`, wajib) + `category` enum `ktp`/`npwp`/`kontrak`; respons 201, 413 |
| GET | `/leave-requests` | `listLeaveRequests` | leave | deprecated; query `status` `array<string>` `collectionFormat: csv` enum `pending`/`approved`/`rejected`; header path-level `$ref: '#/parameters/TenantHeader'` (`X-Tenant-ID`, wajib) |
| POST | `/leave-requests/{leaveRequestId}/approve` | `approveLeaveRequest` | leave | `security` operasi `[{ basicAuth: [] }]` menimpa root (skema `basic` -> `http basic`); respons 204 tanpa `schema` -> tanpa content |

`securityDefinitions`: `bearerAuth` (apiKey, header `Authorization`), `basicAuth` (basic), `hrOauth` (oauth2 `accessCode`, scope `employees:read` "Baca karyawan", `employees:write` "Ubah karyawan"). Baris daftar spec mock (`listspecs/mocks/specs.ts`): `hr-platform` · `HR Platform API` · `Swagger 2.0` · `1.12.0` · `5` endpoint, berdampingan dengan spec 3.x yang sudah ada (`payment`, `Payment API`, `3.0.3`). Tidak ada Acme, John Doe, lorem, atau angka hias.

Fixture e2e memakai petstore (`Swagger Petstore`, `petstore.example.com`) karena isinya ditetapkan kontrak backend dan dipakai bersama tes Go; itu fixture uji format, bukan data tampilan.

### 9.6 State per layar

| State | Daftar spec | Detail / operasi | Dialog unggah |
|---|---|---|---|
| Loading | perilaku lama (tabel dengan baris placeholder/`Spinner` yang ada) | `Spinner` `aria-busy` sampai zod + adapter selesai (dokumen besar seperti hr-platform tetap satu fase loading, tanpa render parsial) | tombol `busy`, input dinonaktifkan (lama) |
| Empty | `EmptyState` dengan `apispec.list.emptyHint` baru (menyebut Swagger 2.0) + aksi unggah | spec tanpa endpoint: bingkai sidebar tetap + kalimat kosong yang ada; overview tetap tampil. Operasi tidak ada -> `EmptyState` `opNotFound` + tautan overview | tidak ada baris sebelum memilih file (lama) |
| Error | `ErrorState` + coba lagi (lama) | 404 -> `EmptyState` + kembali ke daftar; lain/`INVALID_RESPONSE`/adapter gagal -> `ErrorState` di dalam blok + coba lagi | error request (413, 400) -> pesan di dialog (lama); per file `Ditolak` + issue mono + Salin |

### 9.7 Usulan perubahan DESIGN.md (catatan, tidak dikerjakan diam-diam)

- Baris "Tab API Spec" di tabel "Pola per layar" bisa ditambah: "versi spec: pil netral; `Swagger 2.0` ditulis penuh, 3.x disingkat `3.1`." Tidak ada perubahan token. Bila pemilik setuju, ubah DESIGN.md lalu commit terpisah; implementasi FE5 tidak bergantung pada perubahan ini.

---

## 10. Risiko dan catatan

1. K1-K12 belum disetujui pemilik (OV 3). Bila K3 (servers), K6 (pemetaan adapter) atau K11 (label) berubah, plan ini diperbarui bersama overview.
2. Dokumen besar (hr-platform 3,5 MB, ~2.481 `$ref`): adapter berjalan sekali per pemuatan detail di thread utama; tes kinerja 6.1 mencatat waktunya. Bila terasa lambat di uji nyata, laporkan angka; jangan memindahkan pemetaan ke backend (bertentangan dengan keputusan pemilik "adapter di frontend").
3. `#/parameters` dan `#/responses` di-resolve satu langkah di adapter (FE3); `#/definitions` tidak pernah di-inline sehingga sirkular aman lewat `deref` yang ada.
4. Teks persis issue 2.0 di mock (`UNRESOLVED_REF`) disalin dari keluaran tes integration backend saat tersedia; sebelum itu mock memakai teks ilustratif dari OV bagian 10.
5. Fixture e2e `swagger-2.0.yaml` dimiliki plan `uploadspecs` (langkah 6); plan ini hanya menambah tiga fixture lain dan spec e2e yang membacanya.
