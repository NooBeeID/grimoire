# T13 project/api-key per user — Overview Plan (FOUNDATION)

Tiket: `docs/tickets/T13-project-api-key-per-user.md` (revisi T04).
Sumber aturan: `docs/tickets/README.md` (konvensi arsitektur, katalog bus, katalog audit), `docs/superpowers/specs/*knowledge-hub-design.md` / `2026-10-20-grimoire-design.md` (produk), `docs/superpowers/plans/project-api-key/overview.md` (foundation T04: `base/`, `store`, `access`, audit, K-keputusan T04).
Konteks yang dicek: belum ada `CLAUDE.md` dan `.claude/rules/`. Tidak ada knowledge doc khusus module api-key selain plan T04. Kode T04 sudah di `main`.

Dokumen ini adalah plan FOUNDATION. Isinya keputusan bersama, perubahan `base/`, migrasi, dokumen/kontrak, urutan, dan daftar usecase. Plan rinci per usecase ditulis terpisah di `docs/superpowers/plans/project-api-key-per-user/usecase-<slug>.md` dan WAJIB mengikuti dokumen ini; bila bertentangan, dokumen ini yang menang. Frontend ditulis di `docs/superpowers/plans/project-api-key-per-user/frontend-plan.md`; ringkasannya ada di bagian 9 agar developer backend tahu batasnya.

---

## 1. Metadata

| Atribut | Nilai |
|---|---|
| Ticket | T13 (revisi T04) |
| Scope | fullstack (backend di plan ini dan plan usecase; frontend di `frontend-plan.md`) |
| Domain | project |
| Module | api-key (`apikey`) |
| Resource root | `backend/internal/domain/project/internal/apikey` |
| Bergantung pada | T03 (merged), T04 (merged); regresi T07 (MCP gateway, merged) |

| Endpoint | Method | Version | Perubahan |
|---|---|---|---|
| `/api/projects/{projectID}/api-keys` | GET | tanpa prefix versi (`/api`), `api-key.yml` `info.version` 0.1.0 tetap | Isi tersaring per pemilik; bentuk respons sama |
| `/api/projects/{projectID}/api-keys` | POST | sama | Unik nama dan kuota per user per project; pesan error baru, kode/reason sama |
| `/api/projects/{projectID}/api-keys/{keyID}/revoke` | POST | sama | Hanya key milik pemanggil (Super Admin: semua); key orang lain = 404 KEY_NOT_FOUND |
| bus `project.apikey.verify`, `project.apikey.touch`, `project.apikey.count-keys` | bus | v1 | TIDAK berubah (K7) |

---

## 2. Summary Plan

API key di T04 adalah milik project: semua anggota melihat dan bisa mencabut semua key, serta berbagi kuota dan ruang nama. T13 mengubah kepemilikan menjadi **per user per project** dengan pemilik = `api_keys.created_by` (K1), tanpa kolom baru.

- Aturan "siapa boleh lihat/cabut apa" dipusatkan di satu entity `base.OwnerScope` (K2). Super Admin = `{All: true}`; user lain = `{OwnerID: actor.ID}`. Key legacy `created_by IS NULL` hanya terlihat oleh scope All (K4).
- Scope diterapkan di SQL (repository), dengan dua konstanta SQL per query (dengan/tanpa filter pemilik). Tidak ada penyaringan di memori.
- `revokekey`: key milik orang lain diperlakukan identik dengan key tidak ada (404 KEY_NOT_FOUND) (K3).
- `createkey`: advisory lock, hitung kuota, dan unik nama per `(project, actor.ID)` untuk semua role termasuk Super Admin (K5). Migrasi mengganti unique index dengan nama yang sama sehingga pemetaan `ErrNameTaken` tetap (K6).
- `verifykey`, `touchlastused`, `countkeys` TIDAK disentuh (K7). Tidak ada perubahan kontrak bus, tidak ada aksi audit baru, tidak ada perubahan bentuk respons REST.
- Risiko audit log tertutup: halaman audit sudah khusus Super Admin (bagian 3).
- Frontend: kolom "Dibuat oleh" hanya untuk Super Admin, penyesuaian teks i18n, e2e dua user.

---

## 3. Konteks terverifikasi di kode (bukan asumsi)

| Fakta | Lokasi |
|---|---|
| Daftar tanpa filter pemilik: `WHERE project_id = $1` | `listkeys/repository.go` (`listByProjectSQL`) |
| Revoke tanpa filter pemilik: `WHERE id = $1 AND project_id = $2 FOR UPDATE` | `revokekey/repository.go` (`lockInProjectSQL`) |
| Lock dan kuota per project: `pg_advisory_xact_lock(hashtextextended('apikey:' \|\| $1::text, 0))`, `count(*) ... WHERE project_id = $1 AND revoked_at IS NULL` | `createkey/repository.go` (`qLock`, `qCount`) |
| Unique per project: `api_keys_project_active_name_key ON api_keys (project_id, name) WHERE revoked_at IS NULL` | `backend/migrations/project/20261022095716034_create_api_keys.sql` |
| `store.ConstraintActiveName = "api_keys_project_active_name_key"` | `base/store/store.go` |
| `RoleSuperAdmin = "super_admin"` | `backend/lib/contracts/core/auth/resolvesession/resolvesession.go` |
| Pola `IsSuperAdmin` lewat `resolvesession` | `backend/internal/domain/project/internal/management/base/actor.go` |
| `Actor{ID, Role, Label}` belum punya `IsSuperAdmin` | `base/actor.go` |
| `Key.CreatedBy *string`, `NewKey` selalu mengisi `CreatedBy` | `base/key.go` |
| DTO sudah memuat `created_by` dan `created_by_label` | `base/dto.go` |

**Risiko audit log (Pertanyaan terbuka #2 T13) DITUTUP**:
- `backend/internal/domain/core/internal/auditlog/base.go` memasang `middleware.RequireRole(resolvesession.RoleSuperAdmin)` pada seluruh grup `/audit-log` (listentries dan getentry).
- Route FE `/audit-log` di `frontend/src/app/routes/index.ts` ber-meta `roles: ['super_admin']`.
- Link audit di tab API Keys hanya dipasang bila `isSuperAdmin` (`frontend/src/app/views/workspace/ApiKeysTab.svelte`).

Jadi nama/prefix key di event `apikey.created`/`apikey.revoked` tidak bocor ke non-Super Admin. Tidak ada perubahan kode audit. Saat implementasi, jawab Pertanyaan terbuka #2 dan butir Risiko "Log audit" di T13 dengan rujukan tiga file di atas.

Pertanyaan terbuka #1 (`countkeys`) tetap memakai default tiket (K7: jumlah total project). Tidak memblokir.

---

## 4. Entity bersama di `base/`

### 4.1 File baru `base/owner.go` (tanpa I/O)

```go
package base

import "grimoire/lib/contracts/core/auth/resolvesession"

// IsSuperAdmin: aturan peran tunggal untuk module api-key.
func (a Actor) IsSuperAdmin() bool { return a.Role == resolvesession.RoleSuperAdmin }

// OwnerScope menentukan key mana yang boleh dilihat/dicabut actor di sebuah project.
// Sengaja struct (bukan string kosong = semua) agar ID kosong tidak pernah berarti "semua".
type OwnerScope struct {
	All     bool   // true hanya untuk super_admin
	OwnerID string // dipakai bila !All
}

// OwnerScope: super_admin -> {All: true}; selain itu -> {OwnerID: a.ID}.
func (a Actor) OwnerScope() OwnerScope {
	if a.IsSuperAdmin() {
		return OwnerScope{All: true}
	}
	return OwnerScope{OwnerID: a.ID}
}

// Permits: All -> true; selain itu hanya key dengan CreatedBy == OwnerID.
// Key legacy (CreatedBy nil) hanya lolos untuk All (K4).
func (s OwnerScope) Permits(k Key) bool {
	if s.All {
		return true
	}
	return s.OwnerID != "" && k.CreatedBy != nil && *k.CreatedBy == s.OwnerID
}

// Empty: scope tidak bisa mengizinkan apa pun (fail-closed).
func (s OwnerScope) Empty() bool { return !s.All && s.OwnerID == "" }
```

- Import `grimoire/lib/contracts/core/auth/resolvesession` di `base` diizinkan (lib/contracts netral; pola sama dengan `management/base/actor.go`; lolos `backend/internal/archtest`).
- Ini SATU-SATUNYA tempat aturan visibilitas. Usecase tidak boleh memeriksa `Role` sendiri; repository hanya membaca `scope.All`/`scope.OwnerID`.
- `IsSuperAdmin` boleh ditaruh di `base/actor.go` bila developer lebih suka; tesnya tetap di `base/owner_test.go`. Jangan duplikasi.

### 4.2 Pemakaian scope di repository (aturan wajib bagi tiga usecase)

- Pilih satu dari DUA konstanta SQL berdasarkan `scope.All`: tanpa filter pemilik (All) atau dengan `AND created_by = $n::uuid` (pemilik). DILARANG `($2 = '' OR created_by::text = $2)`: tidak memakai index dan rawan "kosong = semua".
- Fail-closed sebelum query: `scope.Empty()` atau `!scope.All && !store.ValidUUID(scope.OwnerID)` -> listkeys mengembalikan `[]base.Key{}`; revokekey mengembalikan `found=false`. Tanpa query.
- `createkey` TIDAK memakai `OwnerScope`; kuota/unik nama memakai `actor.ID` untuk semua role (K5).

### 4.3 Pesan error di `base/errors.go`

Kode, reason, dan details TIDAK berubah. Hanya pesan:

| Simbol | Pesan lama | Pesan baru |
|---|---|---|
| `ErrNameTaken` | `nama key sudah dipakai key aktif lain di project ini` | `nama key sudah dipakai key aktif Anda yang lain di project ini` |
| `ErrKeyLimitReached(limit)` | `batas %d key aktif per project tercapai; cabut key yang tidak dipakai` | `batas %d key aktif per user per project tercapai; cabut key yang tidak dipakai` |

Pesan tidak menyebut pemilik lain (anti-tujuan). Perbarui `base/errors_test.go` dan tes handler/integrasi lain yang meng-assert pesan lama (cari dengan `grep -rn "key aktif lain\|key aktif per project" backend/`).

---

## 5. Kontrak bus dan konstanta endpoint

Tidak ada perubahan.
- `lib/constants/bus.go`: tidak disentuh (`BUS_PROJECT_APIKEY_VERIFY`, `BUS_PROJECT_APIKEY_TOUCH`, `BUS_PROJECT_APIKEY_COUNT_KEYS` tetap).
- `lib/contracts/project/apikey/**` dan `lib/contracts/project/management/contentcount`: tidak disentuh.
- Bus yang tetap dikonsumsi module ini: `project.management.check-access` (via `base/access.Require`, dipanggil SEBELUM scope; non-anggota tetap 404 project), `core.audit.record` (via `base/audit`, fail-open, aksi `record.ActionAPIKeyCreated`/`ActionAPIKeyRevoked`, aktor = pemanggil).
- Katalog bus dan katalog audit di `docs/tickets/README.md` tidak berubah.

---

## 6. Migration

File baru: `backend/migrations/project/20261022160000000_api_keys_owner_scope.sql`.
Timestamp wajib > `20261022105305340` (migrasi terakhir di folder). Bila saat implementasi sudah ada migrasi `project` dengan timestamp >= nilai di atas, ganti dengan timestamp saat implementasi (`YYYYMMDDHHMMSSmmm`) dan sesuaikan rujukan di catatan revisi T04. JANGAN mengedit migrasi T04.

```sql
-- T13: kepemilikan key per user per project. Memperlonggar unique, tidak bisa gagal pada data T04.
-- Nama index dipertahankan agar store.ConstraintActiveName dan pemetaan ErrNameTaken tetap (K6).
-- created_by NULL dianggap berbeda satu sama lain oleh Postgres; dapat diterima (K4).
DROP INDEX IF EXISTS api_keys_project_active_name_key;
CREATE UNIQUE INDEX IF NOT EXISTS api_keys_project_active_name_key
    ON api_keys (project_id, created_by, name) WHERE revoked_at IS NULL;
CREATE INDEX IF NOT EXISTS api_keys_owner_idx ON api_keys (project_id, created_by, created_at DESC);
```

- Runner `infra/db/migrate.go` menjalankan tiap file dalam satu tx dengan simple protocol: TANPA `CONCURRENTLY`.
- `api_keys_project_idx` dibiarkan (dipakai jalur Super Admin dan `countkeys`).
- Idempoten (`IF EXISTS`/`IF NOT EXISTS`). Tidak ada down-migration (konvensi repo).

---

## 7. Plan (langkah berurutan)

### Langkah 1 — FOUNDATION (satu commit, sebelum tiga usecase)

1. Buat `base/owner.go` (bagian 4.1) dan `base/owner_test.go` (bagian 10.1).
2. Ubah pesan di `base/errors.go` (bagian 4.3); perbarui `base/errors_test.go`.
3. Buat migrasi (bagian 6).
4. Dokumen konfigurasi:
   - `docs/configuration.md` baris `API_KEY_MAX_ACTIVE_PER_PROJECT`: deskripsi -> `Batas API key aktif per user per project (nama env dipertahankan).`
   - `.env.example` baris 35: komentar di atasnya -> `# Batas API key aktif per user per project (nama env dipertahankan)`.
5. OpenAPI `api/modules/project/api-key.yml` (deskripsi dan contoh saja, schema tidak berubah):
   - GET: "Hanya key milik pemanggil (`created_by` = pemanggil). super_admin melihat key semua anggota, termasuk key tanpa pemilik." Contoh 200 diganti dua item milik user yang sama (lihat bagian 12.1).
   - POST: "Nama 2-60 karakter dan unik di antara key aktif milik pemanggil di project ini; maksimal 20 key aktif per user per project (`API_KEY_MAX_ACTIVE_PER_PROJECT`). Key yang dibuat super_admin dihitung ke kuota super_admin sendiri." Contoh 409 memakai pesan baru.
   - revoke: "Hanya key milik pemanggil; super_admin boleh mencabut key siapa pun. Key milik user lain dijawab 404 KEY_NOT_FOUND, identik dengan key yang tidak ada." Contoh 200 diganti agar `revoked_by` = `created_by` (pemilik mencabut sendiri).
   - Jalankan `make api-bundle`, commit `api/openapi.yml`, `make api-check` hijau.
6. `docs/tickets/T04-project-api-key.md`: tambah bagian `## Catatan revisi` tepat sebelum `## Pertanyaan terbuka`:
   ```
   ## Catatan revisi

   - Direvisi T13: (a) daftar dan pencabutan key dibatasi ke pemilik (`created_by`), Super Admin tetap melihat/mencabut semua; (b) batas key aktif dan unik nama berlaku per user per project (migrasi `backend/migrations/project/20261022160000000_api_keys_owner_scope.sql`); (c) `count-keys` tetap menghitung seluruh project.
   ```
   Isi T04 lain tidak ditulis ulang.
7. `docs/tickets/T13-project-api-key-per-user.md`: jawab Pertanyaan terbuka #2 dan butir Risiko "Log audit" dengan rujukan bagian 3 (audit-log khusus Super Admin, terverifikasi).
8. Verifikasi foundation: `go test ./backend/internal/domain/project/internal/apikey/base/...`, `go test ./backend/internal/archtest/...`, migrasi berjalan di DB test (`make` target migrasi/integration yang ada).

Catatan: setelah langkah 1 tetapi sebelum usecase selesai, perilaku runtime sementara: unique index sudah per user, tetapi `createkey` masih mengunci/menghitung per project. Aman (lebih ketat), jadi foundation boleh merge dahulu.

### Langkah 2 — tiga usecase (boleh paralel setelah langkah 1, file tidak tumpang tindih)

| Slug | Plan | File utama |
|---|---|---|
| `listkeys` | `usecase-listkeys.md` | `listkeys/usecase.go`, `listkeys/repository.go`, tes |
| `revokekey` | `usecase-revokekey.md` | `revokekey/usecase.go`, `revokekey/repository.go`, tes |
| `createkey` | `usecase-createkey.md` | `createkey/usecase.go`, `createkey/repository.go`, tes |

Handler, `init.go`, `params.go`, dan `base.go` module TIDAK berubah di ketiganya. `apikey_integration_test.go` (tingkat module) disentuh oleh `createkey` saja; usecase lain menambah skenario di file tes feature masing-masing untuk menghindari konflik.

### Langkah 3 — frontend (paralel dengan langkah 2; tanpa perubahan API)

Lihat `frontend-plan.md` (ringkasan bagian 9).

### Langkah 4 — verifikasi akhir

`go test ./...`; tes integrasi (tag `integration`) module apikey dan `verifykey/verify_integration_test.go`; `backend/internal/archtest`; lint; `make api-check`; `npm run test`; `npm run depcruise`; `npm run test:arch`; e2e `frontend/e2e/project-api-key.spec.ts` dan regresi `frontend/e2e/mcp-gateway.spec.ts`. Status T13 di `docs/tickets/README.md` diisi saat merge (bukan bagian plan).

---

## 8. Keputusan lintas usecase

- K2: scope di SQL, dua konstanta per query, dipilih repository dari `scope.All`. Usecase: `actor.OwnerScope()` -> repo. Tanpa filter di memori.
- Urutan di usecase: `ActorFrom` (tanpa identitas -> 401) -> `access.Require` (non-anggota -> `ErrProjectNotFound` 404) -> scope -> repo.
- `revokekey`: pertahanan berlapis di dalam tx: `found=false` -> `ErrKeyNotFound`; lalu `if !scope.Permits(k) { return base.ErrKeyNotFound }` sebelum `k.Revoke`. Sudah dicabut milik sendiri -> `ErrAlreadyRevoked` 409.
- `createkey`: `LockOwnerKeys(ctx, projectID, actor.ID)` dengan `hashtextextended('apikey:' || $1::text || ':' || $2::text, 0)`; `CountActive(ctx, projectID, actor.ID)` dengan `created_by = $2::uuid`. Super Admin pakai ID sendiri.
- Audit tetap `apikey.created`/`apikey.revoked`, aktor = pemanggil (Super Admin tercatat sebagai aktor saat mencabut key orang lain), dicatat hanya setelah commit sukses, fail-open seperti T04.
- Respons 404 key orang lain identik dengan keyID acak: status, `error.code`, `error.message`, `error.details` sama; hanya `trace_id` yang berbeda per request. Tes membandingkan body setelah menghapus `trace_id`.
- Config `APIKeyMaxActivePerProject` tetap nama dan default (20), kini dibaca sebagai batas per user per project.

---

## 9. Frontend (ringkasan; rinci di `frontend-plan.md`)

- Tidak ada perubahan service/types/Zod (DTO sudah memuat `created_by_label`).
- `frontend/src/domain/project/api-key/listkeys/components/KeysTable.svelte`: prop `showOwner?: boolean` (default false). Bila true: kolom "Dibuat oleh" (`project.apikey.col.owner`) berisi `created_by_label ?? unknownUser`; kolom "Dibuat" cukup tanggal.
- `frontend/src/domain/project/api-key/listkeys/views/ListKeysView.svelte` meneruskan `showOwner`; `frontend/src/app/views/workspace/ApiKeysTab.svelte` mengisi `showOwner={isSuperAdmin}`.
- i18n `frontend/src/shared/i18n/{id,en}.json` (kunci flat): `project.apikey.intro` (+ "Key di sini hanya milik Anda."), baru `project.apikey.introAdmin` ("Sebagai Super Admin Anda melihat key semua anggota project."), `project.apikey.emptyHint`, `project.apikey.create.nameHint` ("unik di antara key aktif Anda"), `project.apikey.error.NAME_TAKEN` ("Anda sudah punya key aktif dengan nama ini"), `project.apikey.error.KEY_LIMIT_REACHED` ("Batas {limit} key aktif per user tercapai. Cabut key yang tidak dipakai."), baru `project.apikey.col.owner` ("Dibuat oleh" / "Created by").
- Visual: tanpa layar baru; pola tabel DESIGN.md yang sudah ada; tanpa token baru.
- Tes: `components.test.ts` (kolom owner hanya saat `showOwner`), `ListKeysView.test.ts`, `ApiKeysTab.test.ts` (introAdmin untuk Super Admin), e2e (bagian 11).

---

## 10. Test Scenario (foundation)

### 10.1 `base/owner_test.go` (unit, tabel)

| Kasus | Input | Harapan |
|---|---|---|
| developer | `Actor{ID: "u-a", Role: "developer"}` | `OwnerScope() == {OwnerID: "u-a"}`, `IsSuperAdmin() == false` |
| super_admin | `Actor{ID: "u-s", Role: "super_admin"}` | `OwnerScope() == {All: true}` (OwnerID kosong), `IsSuperAdmin() == true` |
| role kosong/tak dikenal | `Actor{ID: "u-x", Role: ""}` | `{OwnerID: "u-x"}` (bukan All) |
| Permits milik sendiri | scope `{OwnerID: "u-a"}`, key `CreatedBy="u-a"` | true |
| Permits milik orang lain | scope `{OwnerID: "u-a"}`, key `CreatedBy="u-b"` | false |
| Permits legacy (developer) | scope `{OwnerID: "u-a"}`, `CreatedBy=nil` | false |
| Permits legacy (super_admin) | scope `{All: true}`, `CreatedBy=nil` | true |
| Permits scope kosong | `OwnerScope{}`, key `CreatedBy=ptr("")` | false |
| Empty | `OwnerScope{}` true; `{All:true}` false; `{OwnerID:"u-a"}` false | |

### 10.2 `base/errors_test.go`

- `ErrNameTaken`: code `CONFLICT`, `details.reason == NAME_TAKEN`, pesan baru persis, tidak memuat kata "lain di project" tanpa "Anda".
- `ErrKeyLimitReached(3)`: code `CONFLICT`, `details == {reason: KEY_LIMIT_REACHED, limit: 3}`, pesan `batas 3 key aktif per user per project tercapai; cabut key yang tidak dipakai`; instance berbeda per panggilan (tes T04 tetap).

### 10.3 Migrasi (integration, tag `integration`; dieksekusi di plan `createkey`, dicantumkan di sini sebagai syarat foundation)

- DB berisi data T04 (dua key aktif nama berbeda milik dua user, satu key legacy `created_by NULL`, satu key dicabut bernama sama dengan key aktif): migrasi sukses, semua baris utuh.
- Setelah migrasi: `pg_indexes` memuat `api_keys_project_active_name_key` dengan definisi `(project_id, created_by, name) WHERE (revoked_at IS NULL)` dan `api_keys_owner_idx`.
- Menjalankan ulang isi migrasi tidak error (idempoten).
- Insert dua key aktif nama sama oleh user berbeda sukses; oleh user sama -> pelanggaran constraint bernama `store.ConstraintActiveName`.

### 10.4 Archtest dan regresi

- `backend/internal/archtest`: `base` mengimpor `lib/contracts/core/auth/resolvesession` lolos.
- Seluruh tes T04 yang tidak meng-assert pesan lama tetap hijau tanpa perubahan.

Skenario per usecase (unit fake repo, repository integration, handler) ada di plan usecase masing-masing.

---

## 11. E2E Scenarios (`frontend/e2e/project-api-key.spec.ts`)

Positif:
1. Developer A dan B anggota project P. Masing-masing membuat key via `createKeyApi` (`a-key`, `b-key`). Tab API Keys sebagai A hanya menampilkan `a-key`; `b-key` tidak ada. Sebaliknya untuk B.
2. A dan B masing-masing membuat key bernama `shared-name` di P: keduanya sukses.
3. A mencabut `a-key` dari UI: status berubah "dicabut".
4. Super Admin membuka tab API Keys P: melihat key A dan B, kolom "Dibuat oleh" berisi email A dan B, teks `introAdmin` tampil.
5. Super Admin mencabut key B dari UI: sukses; (opsional) halaman audit menampilkan aktor Super Admin.
6. Regresi `frontend/e2e/mcp-gateway.spec.ts`: key buatan developer tetap memberi akses MCP ke project-nya.

Negatif:
1. B memanggil `revokeKeyApi(P, keyA)`: 404 `KEY_NOT_FOUND`; key A tetap aktif saat dilihat A.
2. B memanggil `revokeKeyApi(P, <uuid acak>)`: 404 dengan `error` identik dengan kasus 1.
3. A membuat key kedua bernama `a-key` (masih aktif): dialog menampilkan "Anda sudah punya key aktif dengan nama ini".
4. User biasa tidak melihat kolom "Dibuat oleh".
5. Non-anggota membuka tab/API: 404 project (tidak berubah).
6. Asersi teks lama di spec T04 (`NAME_TAKEN`, `KEY_LIMIT_REACHED`, intro) diperbarui ke teks baru.

---

## 12. Request/Response body

Format error (tidak berubah, `infra/http/respond`): `{"error": {"code", "message", "details"}, "trace_id"}`.

### 12.1 GET `/api/projects/{projectID}/api-keys` (developer A)

Request: tanpa body, cookie sesi.

200:
```json
{
  "items": [
    {
      "id": "0192f5a0-2222-7aaa-8bbb-000000000003",
      "name": "ci-pipeline",
      "prefix": "a1b2c3d4",
      "status": "active",
      "created_by": "0192f5a0-aaaa-7aaa-8bbb-00000000000a",
      "created_by_label": "a@tim.id",
      "created_at": "2026-10-23T10:00:00Z",
      "last_used_at": "2026-10-23T11:42:00Z",
      "revoked_at": null,
      "revoked_by": null,
      "revoked_by_label": null
    },
    {
      "id": "0192f5a0-2222-7aaa-8bbb-000000000001",
      "name": "claude-code-lama",
      "prefix": "k3x9q2ma",
      "status": "revoked",
      "created_by": "0192f5a0-aaaa-7aaa-8bbb-00000000000a",
      "created_by_label": "a@tim.id",
      "created_at": "2026-10-23T09:00:00Z",
      "last_used_at": null,
      "revoked_at": "2026-10-23T12:00:00Z",
      "revoked_by": "0192f5a0-aaaa-7aaa-8bbb-00000000000a",
      "revoked_by_label": "a@tim.id"
    }
  ]
}
```
Super Admin: bentuk sama, `items` memuat key semua anggota (dan key legacy dengan `created_by: null`, `created_by_label: null`). Developer tanpa key: `{"items": []}`.

404 (non-anggota / project tidak ada):
```json
{ "error": { "code": "NOT_FOUND", "message": "project tidak ditemukan", "details": null }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```

### 12.2 POST `/api/projects/{projectID}/api-keys`

Request (header `X-Requested-With: XMLHttpRequest`):
```json
{ "name": "claude-code-budi" }
```
201 (tidak berubah; `Cache-Control: no-store`):
```json
{
  "key": {
    "id": "0192f5a0-2222-7aaa-8bbb-000000000004",
    "name": "claude-code-budi",
    "prefix": "m7p2x9qa",
    "status": "active",
    "created_by": "0192f5a0-bbbb-7aaa-8bbb-00000000000b",
    "created_by_label": "b@tim.id",
    "created_at": "2026-10-23T13:00:00Z",
    "last_used_at": null,
    "revoked_at": null,
    "revoked_by": null,
    "revoked_by_label": null
  },
  "raw_key": "gr_m7p2x9qa_Q2xhdWRlLWNvZGUtYnVkaS1zZWNyZXQtZXhhbXBsZS0xMjM0NTY",
  "mcp_url": "https://grimoire.tim.id/mcp"
}
```
(Format `raw_key` mengikuti T04 tanpa perubahan.)

409 NAME_TAKEN (pesan baru):
```json
{ "error": { "code": "CONFLICT", "message": "nama key sudah dipakai key aktif Anda yang lain di project ini", "details": { "reason": "NAME_TAKEN" } }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```
409 KEY_LIMIT_REACHED (pesan baru):
```json
{ "error": { "code": "CONFLICT", "message": "batas 20 key aktif per user per project tercapai; cabut key yang tidak dipakai", "details": { "reason": "KEY_LIMIT_REACHED", "limit": 20 } }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```
400 (tidak berubah):
```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "permintaan tidak valid", "details": { "fields": { "name": "nama key wajib 2-60 karakter" } } }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```

### 12.3 POST `/api/projects/{projectID}/api-keys/{keyID}/revoke`

Request: tanpa body, header `X-Requested-With`.

200 (A mencabut key A):
```json
{
  "key": {
    "id": "0192f5a0-2222-7aaa-8bbb-000000000003",
    "name": "ci-pipeline",
    "prefix": "a1b2c3d4",
    "status": "revoked",
    "created_by": "0192f5a0-aaaa-7aaa-8bbb-00000000000a",
    "created_by_label": "a@tim.id",
    "created_at": "2026-10-23T10:00:00Z",
    "last_used_at": "2026-10-23T11:42:00Z",
    "revoked_at": "2026-10-23T12:30:00Z",
    "revoked_by": "0192f5a0-aaaa-7aaa-8bbb-00000000000a",
    "revoked_by_label": "a@tim.id"
  }
}
```
Super Admin mencabut key B: bentuk sama, `created_by` = B, `revoked_by` = Super Admin.

404 (B mencabut key A, ATAU keyID acak/bukan uuid; identik kecuali `trace_id`):
```json
{ "error": { "code": "NOT_FOUND", "message": "API key tidak ditemukan", "details": { "reason": "KEY_NOT_FOUND" } }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```
409 (A mencabut ulang key A):
```json
{ "error": { "code": "CONFLICT", "message": "API key sudah dicabut", "details": { "reason": "ALREADY_REVOKED" } }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```

Bus (`project.apikey.verify`, `touch`, `count-keys`): request/response tidak berubah; lihat `docs/superpowers/plans/project-api-key/overview.md`.

---

## 13. Changes (per file/folder)

| File | Jenis | Oleh |
|---|---|---|
| `backend/internal/domain/project/internal/apikey/base/owner.go` | baru | foundation |
| `backend/internal/domain/project/internal/apikey/base/owner_test.go` | baru | foundation |
| `backend/internal/domain/project/internal/apikey/base/errors.go` | ubah pesan | foundation |
| `backend/internal/domain/project/internal/apikey/base/errors_test.go` | ubah | foundation |
| `backend/migrations/project/20261022160000000_api_keys_owner_scope.sql` | baru | foundation |
| `docs/configuration.md`, `.env.example` | ubah deskripsi | foundation |
| `api/modules/project/api-key.yml`, `api/openapi.yml` | ubah deskripsi/contoh, bundle ulang | foundation |
| `docs/tickets/T04-project-api-key.md` | tambah `## Catatan revisi` | foundation |
| `docs/tickets/T13-project-api-key-per-user.md` | jawab Pertanyaan terbuka #2 / Risiko audit | foundation |
| `.../apikey/listkeys/{usecase.go,repository.go,*_test.go}` | ubah | usecase `listkeys` |
| `.../apikey/revokekey/{usecase.go,repository.go,*_test.go}` | ubah | usecase `revokekey` |
| `.../apikey/createkey/{usecase.go,repository.go,*_test.go}`, `.../apikey/apikey_integration_test.go` | ubah | usecase `createkey` |
| `frontend/src/domain/project/api-key/listkeys/{components/KeysTable.svelte,views/ListKeysView.svelte,tes}`, `frontend/src/app/views/workspace/ApiKeysTab.svelte`, `frontend/src/shared/i18n/{id,en}.json`, `frontend/e2e/project-api-key.spec.ts` | ubah | frontend |
| `verifykey/`, `touchlastused/`, `countkeys/`, `lib/constants/bus.go`, `lib/contracts/**`, audit-log | TIDAK berubah | - |

---

## 14. Daftar usecase

| Slug | Ringkas | Kriteria penerimaan T13 |
|---|---|---|
| `listkeys` | GET daftar hanya key milik pemanggil; Super Admin semua. `KeyLister.ListByProject(ctx, projectID string, scope base.OwnerScope)`; `listByProjectSQL` (All) dan `listByOwnerSQL` (`AND created_by = $2::uuid`), urutan dan `listColumns` tetap. | 1, 6 (lihat), 7, 10 |
| `revokekey` | Revoke hanya key milik pemanggil; key orang lain 404 identik. `LockInProject(ctx, projectID, keyID string, scope base.OwnerScope)`; `lockInProjectSQL` (All) dan `lockOwnedInProjectSQL` (`AND created_by = $3::uuid FOR UPDATE`); `scope.Permits` sebelum `Revoke`. | 2, 3, 6 (cabut), 7 |
| `createkey` | Kuota, unik nama, dan advisory lock per `(project, actor.ID)`. `LockOwnerKeys(ctx, projectID, ownerID)`, `CountActive(ctx, projectID, ownerID)`; tes migrasi pada data T04. | 4, 5, 6 (kuota Super Admin), 8, 9 |

Kriteria 11 (tes lengkap, api-check, lint, arch) dipenuhi bersama di langkah 4.
