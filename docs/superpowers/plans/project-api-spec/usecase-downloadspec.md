# T06 / api-spec / downloadspec — Plan Usecase

Mengikuti `docs/superpowers/plans/project-api-spec/overview.md` (selanjutnya "overview"). Bila bertentangan, overview menang. Plan ini mengasumsikan fase F (foundation) overview bagian 11 sudah selesai: `base/` (entity, error, `Clock`, `ActorFrom`), `base/store` (`FindSpec`), `base/access` (`Checker`), migrasi `api_specs`, dan `api/modules/project/apispec.yml` berisi path `/raw`.

---

## 1. Metadata

| Atribut | Nilai |
|---|---|
| Ticket | T06 |
| Scope | fullstack (plan ini: backend + sisipan tombol unduh di FE, lihat Changes) |
| Domain | project |
| Module | api-spec (folder `apispec`) |
| Feature / usecase | `downloadspec` |
| Resource root | `backend/internal/domain/project/internal/apispec` |
| Endpoint | `/api/projects/{projectID}/api-specs/{name}/raw` |
| Method | GET |
| Version | tanpa prefix versi di URL; `apispec.yml` `info.version: 0.1.0` |
| Auth | `middleware.RequireAuth()`; GET tidak butuh header CSRF |
| Siapa | anggota project, super_admin |
| Bus dikonsumsi | `project.management.check-access` (Call, fail-closed) lewat `base/access` |
| Bus disediakan | tidak ada |
| Audit | tidak ada (unduh baca-saja) |
| Migrasi | tidak ada |

---

## 2. Summary Plan

Mengunduh teks asli spec byte-per-byte. Alur usecase:

1. Ambil actor dari identitas (401 bila tidak ada; seharusnya mustahil karena `RequireAuth`).
2. `access.Checker.RequireMember(ctx, projectID)`: non-anggota, project tidak ada, dan `projectID` bukan uuid semuanya menjadi `base.ErrProjectNotFound` (404). Dicek SEBELUM mencari spec (overview 3.4) agar non-anggota tidak bisa membedakan "spec tidak ada" dari "bukan anggota".
3. `store.FindSpec(ctx, projectID, name)`; tidak ada -> `base.ErrSpecNotFound` (404).
4. Usecase mengembalikan `RawFile{Name, Format, Content}`. Handler yang menulis respons (BUKAN `respond.WriteJSON`) dengan header:
   - `Content-Type: application/yaml; charset=utf-8` atau `application/json; charset=utf-8` menurut `Spec.Format`
   - `Content-Disposition: attachment; filename="<name>.<yaml|json>"`
   - `X-Content-Type-Options: nosniff`
   - `Cache-Control: no-store`
   - `Content-Length: len(content)`
5. Error di sisi mana pun sebelum byte pertama ditulis -> `respond.WriteError` (format JSON seragam).

Keputusan feature ini:
- Header HANYA di-set setelah usecase sukses. Dengan begitu respons error tidak membawa `Content-Disposition`/`Content-Type` spec dan `WriteError` bebas mengatur `Content-Type: application/json` miliknya.
- Ekstensi file ditentukan dari `Spec.Format` (`yaml` -> `.yaml`, `json` -> `.json`), bukan dari `source_path`. Ekstensi `.yml` asli tidak dipertahankan (tidak disimpan).
- `filename="<name>.<ext>"` aman tanpa escaping karena `name` dijamin `^[a-z0-9._-]{1,80}$` (CHECK constraint DB + `ValidateName`). Handler tetap membangun nilai lewat `mime.FormatMediaType("attachment", map[string]string{"filename": fn})` agar kutipan benar bila aturan nama berubah; untuk nama valid hasilnya `attachment; filename=<name>.<ext>` atau terkutip, sehingga tes membaca nilai dengan `mime.ParseMediaType` (bukan perbandingan string). Bila ingin bentuk persis `attachment; filename="payment.yaml"`, gunakan `fmt.Sprintf("attachment; filename=%q", fn)`; PILIH `Sprintf` dengan `%q` (nama sudah dibatasi ASCII aman sehingga `%q` identik dengan kutip biasa) agar sama dengan contoh overview bagian 15.
- Isi yang dikirim adalah `Spec.Content` apa adanya, yaitu teks yang disimpan: sudah tanpa BOM (D8) karena upload membuang BOM sebelum hash/simpan. Jadi "identik dengan unggahan" berlaku untuk file tanpa BOM; untuk file ber-BOM yang identik adalah `StripBOM(file)`. Tes mencakup keduanya.
- Tidak ada kompresi/encoding transformasi; tidak ada `ETag`/`Last-Modified` (dengan `no-store` tidak dibutuhkan).
- Method lain pada path ini (POST/PUT/DELETE) -> 405 dari chi; `HEAD` tidak didukung secara khusus.
- Tidak membuka baris `api_spec_endpoints`; hanya satu query ke `api_specs` (selain satu panggilan bus).
- Tidak ada penulisan, tidak ada transaksi, tidak ada audit.

---

## 3. Plan (langkah berurutan)

Prasyarat: F selesai. Bila `base/store.FindSpec` belum ada, itu kewajiban F (overview 6.1), bukan feature ini.

1. **Cek kontrak OpenAPI** (rule 11.6) di `api/modules/project/apispec.yml`: path `/projects/{projectID}/api-specs/{name}/raw` operasi `get` dengan response `200` (content `application/yaml` dan `application/json`, schema `type: string`; header `Content-Disposition`, `X-Content-Type-Options`, `Cache-Control`), `401`, `404` (`Error`). Bila F belum memuatnya, tambahkan (lihat Request/Response). Jalankan `make api-bundle` dan `make api-check`.
2. **Buat folder** `backend/internal/domain/project/internal/apispec/downloadspec/` dengan file di bawah.
3. **`params.go`**: parse path param.
   ```go
   package downloadspec

   // Params diambil apa adanya dari URL; tidak divalidasi di handler (overview 3.3).
   type Params struct {
       ProjectID string
       Name      string
   }
   ```
4. **`usecase.go`**: orkestrasi saja, tanpa SQL, tanpa HTTP.
   ```go
   package downloadspec

   // specFinder dipenuhi *store.Store (base/store). Interface lokal, rule 5.6.
   type specFinder interface {
       FindSpec(ctx context.Context, projectID, name string) (base.Spec, bool, error)
   }

   type RawFile struct {
       Name    string
       Format  string // base.FormatYAML | base.FormatJSON
       Content string
   }

   type Usecase struct {
       specs specFinder
       guard access.Checker
   }

   func NewUsecase(specs specFinder, guard access.Checker) *Usecase

   func (u *Usecase) Execute(ctx context.Context, p Params) (RawFile, error) {
       if _, ok := base.ActorFrom(reqctx.IdentityFrom(ctx)); !ok {
           return RawFile{}, errs.New(errs.CodeUnauthorized, "perlu login")
       }
       if err := u.guard.RequireMember(ctx, p.ProjectID); err != nil {
           return RawFile{}, err
       }
       sp, found, err := u.specs.FindSpec(ctx, p.ProjectID, p.Name)
       if err != nil {
           return RawFile{}, fmt.Errorf("downloadspec: find spec: %w", err)
       }
       if !found {
           return RawFile{}, base.ErrSpecNotFound
       }
       return RawFile{Name: sp.Name, Format: sp.Format, Content: sp.Content}, nil
   }
   ```
   Catatan: `reqctx.IdentityFrom` mengembalikan `(Identity, bool)`; `base.ActorFrom(id, ok)` sesuai overview 4.5. Tidak ada rule bisnis baru di usecase; pemetaan format -> media type/ekstensi adalah rule murni dan ditaruh di entity (langkah 5).
5. **Rule murni di `base/`** (tambah ke `base/spec.go`, bagian dari feature ini karena hanya dipakai di sini; boleh ditaruh di `downloadspec/` bila tim memilih lokal, tetapi default: `base`, dengan unit test di `spec_test.go`):
   ```go
   // MediaType mengembalikan nilai Content-Type unduhan raw untuk format spec.
   func (s Spec) MediaType() string     // yaml -> "application/yaml; charset=utf-8"; json -> "application/json; charset=utf-8"
   // FileName mengembalikan "<name>.<yaml|json>".
   func (s Spec) FileName() string
   ```
   Agar usecase tidak perlu membawa `Spec` utuh, `RawFile` dapat menyimpan `MediaType` dan `FileName` hasil pemanggilan dua method itu (`sp.MediaType()`, `sp.FileName()`) menggantikan `Name`/`Format`. PILIH varian ini: `RawFile{FileName, MediaType, Content string}`; handler jadi tipis. Format tidak dikenal (tidak mungkin lolos CHECK DB) -> `MediaType` fallback `application/octet-stream` dan `FileName` tanpa ekstensi diubah menjadi `.txt`; tidak di-panic.
6. **`handler.go`**: satu-satunya tempat yang menulis ke `http.ResponseWriter`.
   ```go
   type Handler struct{ uc *Usecase }

   func newHandler(uc *Usecase) *Handler { return &Handler{uc: uc} }

   func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
       p := Params{
           ProjectID: chi.URLParam(r, "projectID"),
           Name:      chi.URLParam(r, "name"),
       }
       f, err := h.uc.Execute(r.Context(), p)
       if err != nil {
           respond.WriteError(w, r, err)
           return
       }
       hd := w.Header()
       hd.Set("Content-Type", f.MediaType)
       hd.Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", f.FileName))
       hd.Set("X-Content-Type-Options", "nosniff")
       hd.Set("Cache-Control", "no-store")
       hd.Set("Content-Length", strconv.Itoa(len(f.Content)))
       w.WriteHeader(http.StatusOK)
       if _, err := io.WriteString(w, f.Content); err != nil {
           // klien putus; header sudah terkirim, tidak bisa kirim error JSON
           slog.WarnContext(r.Context(), "downloadspec: tulis respons gagal", "feature", "downloadspec", "error", err)
       }
   }
   ```
   Handler tidak menyentuh body request, tidak memakai `WriteJSON`. Log tidak memuat isi spec. Gunakan logger yang sama dengan konvensi feature lain (`d.Log` diteruskan ke `newHandler(uc, d.Log)` bila konvensi repo menghindari `slog` global; ikuti pola `uploadspecs`/T03 saat diimplementasi).
7. **`init.go`**: perakitan dan route (overview 8).
   ```go
   func Init(d app.Deps) {
       s := store.New(d.DB)
       uc := NewUsecase(s, access.New(d.Bus))
       h := newHandler(uc, d.Log)
       d.API.With(middleware.RequireAuth()).
           Get("/projects/{projectID}/api-specs/{name}/raw", h.ServeHTTP)
   }
   ```
   Tanpa `BodyLimit` (GET). Tidak ada `repository.go` karena tidak ada SQL sendiri; `*store.Store` dipakai lewat interface lokal (overview 3.2).
8. **Wiring**: tambahkan satu baris `downloadspec.Init(d)` di `backend/internal/domain/project/internal/apispec/base.go` (`Register`).
9. **Tes** (bagian 6): tulis unit test usecase/handler, lalu tes integrasi.
10. **FE kecil** (lihat Changes): tombol "Unduh" memakai `<a href=".../raw" download>` (GET navigasi, sesi cookie ikut). Detail UI ada di `frontend-plan.md`; di sini hanya kontrak URL.
11. **Verifikasi akhir**: `go vet ./...`, `golangci-lint`, `go test ./internal/domain/project/internal/apispec/...`, `go test -tags integration ...`, `make api-check`, archtest `TestT00_AC4_RepoIsClean`.

Kontrak bus: tidak ada yang baru. Yang dikonsumsi: `project.management.check-access` (`checkaccess.Request{ProjectID, UserID, UserRole}` -> `Response{ProjectExists, Level}`), dibungkus `base/access`; perilaku sesuai tabel overview 3.4.

---

## 4. Migration

Tidak ada. Memakai tabel `api_specs` dari migrasi foundation `backend/migrations/project/20261023090000600_create_api_specs.sql`. Tidak ada indeks baru: lookup `(project_id, name)` dilayani `api_specs_project_name_key`.

---

## 5. Changes (per file/folder)

Baru, `backend/internal/domain/project/internal/apispec/downloadspec/`:
- `init.go` — `Init(d app.Deps)`, route GET `/projects/{projectID}/api-specs/{name}/raw`.
- `handler.go` — membaca path param, memanggil usecase, menulis header + byte langsung.
- `params.go` — `Params{ProjectID, Name}`.
- `usecase.go` — `Usecase`, `RawFile{FileName, MediaType, Content}`, interface `specFinder`.
- `usecase_test.go`, `handler_test.go` — unit.
- `download_integration_test.go` (tag `integration`) — DB nyata.

Diubah:
- `backend/internal/domain/project/internal/apispec/base/spec.go` — tambah `Spec.MediaType()` dan `Spec.FileName()`.
- `backend/internal/domain/project/internal/apispec/base/spec_test.go` — tes dua method itu.
- `backend/internal/domain/project/internal/apispec/base.go` — tambah `downloadspec.Init(d)`.
- `backend/internal/domain/project/internal/apispec/routes_test.go` — pastikan pasangan `GET /api/projects/{projectID}/api-specs/{name}/raw` ada (bagian W; bila file belum ada, ditambahkan oleh tahap W, bukan di sini).
- `api/modules/project/apispec.yml` — path `/raw` (bila belum ada), lalu `api/openapi.yml` hasil `make api-bundle`. `api/root.yml`: `$ref` path (escape `~1`) bila belum ada.
- Frontend (kontrak saja, implementasi di `frontend-plan.md`): `frontend/src/domain/project/apispec/getspec/...` tombol "Unduh" -> `<a href="/api/projects/{projectID}/api-specs/{name}/raw" download>`; i18n kunci `apispec.download`. Tidak memakai `fetch`+blob agar tidak memuat file besar ke memori JS.

Tidak diubah: `infra/**`, `cmd/**`, konstanta bus, kontrak bus, config, migrasi, module lain, dan README katalog (tidak ada endpoint bus/audit baru).

---

## 6. Test Scenario

Penamaan: `TestT06_ACn_*` untuk kriteria penerimaan, selain itu `Test<Unit>_<Kasus>`.

### 6.1 Unit `base` (tanpa mock)
- `Spec.MediaType`: `yaml` -> `application/yaml; charset=utf-8`; `json` -> `application/json; charset=utf-8`; format asing -> `application/octet-stream`.
- `Spec.FileName`: `{Name:"payment", Format:"yaml"}` -> `payment.yaml`; `{Name:"v1.2_x-y", Format:"json"}` -> `v1.2_x-y.json`.

### 6.2 Unit usecase (`usecase_test.go`, fake `specFinder` + fake `access.Checker`)
- Anggota + spec ada -> `RawFile` dengan `Content` sama persis dengan `Spec.Content` (termasuk `\r\n`, tab, baris akhir tanpa newline, karakter Unicode).
- Tanpa identitas di ctx -> `CodeUnauthorized`, guard dan finder TIDAK dipanggil.
- Guard mengembalikan `ErrProjectNotFound` -> error itu diteruskan; finder TIDAK dipanggil (urutan akses dulu; asersi hitungan panggilan = 0).
- Guard mengembalikan error infra -> error diteruskan (bukan nil), finder tidak dipanggil.
- Finder `found=false` -> `errors.Is(err, base.ErrSpecNotFound)`.
- Finder error -> error terbungkus `downloadspec: find spec:` dan BUKAN `*errs.Error` bisnis (-> 500).
- `projectID` dan `name` diteruskan persis ke finder.

### 6.3 Unit handler (`handler_test.go`, `httptest` + usecase dengan fake)
- Sukses YAML: status 200, `Content-Type == "application/yaml; charset=utf-8"`, `Content-Disposition == `attachment; filename="payment.yaml"``, `X-Content-Type-Options == nosniff`, `Cache-Control == no-store`, `Content-Length` = jumlah byte, body byte-identik.
- Sukses JSON: `application/json; charset=utf-8`, filename `.json`.
- Body besar (5 MB) dikirim utuh (bandingkan hash SHA-256).
- Konten ber-karakter `<script>` tidak di-escape (tidak lewat JSON/HTML encoder).
- Error 404 spec: status 404, `Content-Type` JSON (`application/json`), body `{"error":{"code":"NOT_FOUND","message":"spec tidak ditemukan"},"trace_id":...}`, TIDAK ada header `Content-Disposition`.
- Error 404 project, 401, 500: format error seragam; 500 tidak membocorkan pesan internal.
- Penulisan gagal di tengah (`ResponseWriter` yang error) -> tidak panik, tidak menulis ulang JSON error.

### 6.4 Integrasi (`download_integration_test.go`, tag `integration`, `db.TestDB(t)`, bus nyata, handler palsu `check-access` dengan peta project -> level)
- `TestT06_AC_DownloadIdenticalHash`: unggah lewat `uploadspecs` (atau `store`/repository insert bila gelombang A belum merge) file `petstore-3.0.yaml` dan `petstore-3.1.json` dari `base/openapi/testdata`; unduh keduanya; `sha256(body) == sha256(file asli)` dan header sesuai format.
- Varian BOM: unggah `bom.json`; unduh -> body == `openapi.StripBOM(file)` (hash sama), bukan file ber-BOM.
- Varian CRLF dan baris akhir tanpa newline: hash tetap sama.
- `TestT06_AC7_NonMember404`: user bukan anggota -> 404 `project tidak ditemukan`; `projectID` bukan uuid -> 404 yang sama, tanpa query ke `api_specs`; user anggota project B meminta spec project A lewat `projectID` A -> 404; anggota project A meminta nama yang ada di project B tetapi tidak di A -> 404 `spec tidak ditemukan`.
- Respons 404 non-anggota untuk spec yang ADA dan yang TIDAK ADA identik (kode, pesan, status) — mencegah oracle keberadaan spec.
- Nama tidak ada pada project milik sendiri -> 404 `spec tidak ditemukan`.
- Super admin (level `admin` dari check-access) non-anggota -> 200.
- Setelah `renamespec`: nama lama 404, nama baru 200 dengan filename baru.
- Setelah `deletespec`: 404.
- Tanpa sesi -> 401 (`RequireAuth`), tanpa mencapai usecase.
- Check-access handler tidak terdaftar (`ErrNotRegistered`) -> 500 (fail-closed), tidak ada byte spec yang bocor.
- Tanpa baris audit tercatat (handler palsu `core.audit.record` dipanggil 0 kali).

### 6.5 Routing (`routes_test.go`)
- `GET /api/projects/{id}/api-specs/{name}/raw` terdaftar; `POST` pada path yang sama -> 405.
- Spec bernama `raw`/`upload`/`validate`: `GET .../api-specs/raw` -> `getspec` (bukan download); `GET .../api-specs/raw/raw` -> download spec bernama `raw`.

---

## 7. E2E Scenarios (Playwright; butuh T03 merge dan UI detail spec dari `frontend-plan.md`)

Positif:
1. Anggota membuka halaman detail `payment`, klik "Unduh"; file terunduh bernama `payment.yaml`, isinya identik (hash) dengan file yang diunggah sebelumnya.
2. Spec JSON (`petstore-3.1.json`): unduhan bernama `<name>.json`, isi identik.
3. Super Admin (bukan anggota) mengunduh spec project mana pun: berhasil.
4. Setelah ubah nama `payment` -> `payment-v2`, tombol unduh menghasilkan `payment-v2.yaml`.
5. Mengunggah ulang versi baru lalu mengunduh: isi sama dengan versi terbaru (bukan cache; `no-store`).

Negatif:
1. Developer bukan anggota membuka langsung URL `/api/projects/<id>/api-specs/payment/raw` -> 404 JSON, tidak ada file terunduh.
2. URL dengan nama spec yang tidak ada -> 404 JSON (`spec tidak ditemukan`); UI menampilkan pesan, bukan file kosong.
3. Spec baru dihapus di tab lain lalu klik "Unduh" -> 404, UI menampilkan pesan gagal.
4. Pengguna tidak login membuka URL raw -> 401 JSON.
5. Spec yang deskripsinya memuat `<script>alert(1)</script>`: unduhan tetap `attachment` dengan `nosniff`; membuka URL raw tidak menjalankan skrip dan tidak dirender sebagai HTML (tidak ada dialog).

---

## 8. Request/Response body

### GET /api/projects/{projectID}/api-specs/{name}/raw

Request: tanpa body. Header opsional apa pun diabaikan; cookie sesi wajib. Header CSRF tidak diperlukan (GET).

Contoh: `GET /api/projects/0b6c1f3e-5a2d-4e7b-9c10-2f8a6d4e1b33/api-specs/payment/raw`

Response 200 (YAML):

```
HTTP/1.1 200 OK
Content-Type: application/yaml; charset=utf-8
Content-Disposition: attachment; filename="payment.yaml"
X-Content-Type-Options: nosniff
Cache-Control: no-store
Content-Length: 40211

openapi: 3.0.3
info:
  title: Payment API
  version: 1.4.0
paths:
  /payments:
    get:
      operationId: listPayments
      responses:
        "200":
          description: OK
```

Response 200 (JSON), spec tersimpan sebagai JSON:

```
HTTP/1.1 200 OK
Content-Type: application/json; charset=utf-8
Content-Disposition: attachment; filename="petstore.json"
X-Content-Type-Options: nosniff
Cache-Control: no-store
Content-Length: 18234

{"openapi":"3.1.0","info":{"title":"Petstore","version":"1.0.0"},"paths":{}}
```

Body adalah `Spec.Content` apa adanya (tanpa BOM; tidak di-parse, tidak di-format ulang, tidak di-escape).

Error (selalu JSON seragam, `Content-Type: application/json`, tanpa `Content-Disposition`):

404 spec tidak ada (anggota):

```json
{ "error": { "code": "NOT_FOUND", "message": "spec tidak ditemukan" }, "trace_id": "6f1c2a0b9d8e4f17" }
```

404 non-anggota, project tidak ada, atau `projectID` bukan uuid:

```json
{ "error": { "code": "NOT_FOUND", "message": "project tidak ditemukan" }, "trace_id": "6f1c2a0b9d8e4f17" }
```

401 tanpa sesi:

```json
{ "error": { "code": "UNAUTHORIZED", "message": "perlu login" }, "trace_id": "6f1c2a0b9d8e4f17" }
```

500 (mis. check-access tidak tersedia atau DB error; pesan generik):

```json
{ "error": { "code": "INTERNAL", "message": "terjadi kesalahan internal" }, "trace_id": "6f1c2a0b9d8e4f17" }
```

(Kode dan pesan 401/500 mengikuti `respond.mapError` yang sudah ada; implementer menyalin nilai aktualnya ke `apispec.yml` dan tes, bukan mengarang.)

### Fragmen OpenAPI (`api/modules/project/apispec.yml`)

```yaml
/projects/{projectID}/api-specs/{name}/raw:
  get:
    operationId: downloadApiSpec
    summary: Unduh teks asli spec
    tags: [project-apispec]
    parameters:
      - $ref: '#/components/parameters/ProjectID'
      - $ref: '#/components/parameters/SpecName'
    responses:
      '200':
        description: Teks spec asli, byte-per-byte
        headers:
          Content-Disposition: { schema: { type: string }, example: 'attachment; filename="payment.yaml"' }
          X-Content-Type-Options: { schema: { type: string, enum: [nosniff] } }
          Cache-Control: { schema: { type: string, enum: [no-store] } }
        content:
          application/yaml: { schema: { type: string } }
          application/json: { schema: { type: string } }
      '401': { $ref: '../infra/foundation.yml#/components/responses/Unauthorized' }
      '404': { $ref: '../infra/foundation.yml#/components/responses/NotFound' }
```

(Nama `$ref` response menyesuaikan yang sebenarnya ada di `foundation.yml`; cek sebelum menulis.)
