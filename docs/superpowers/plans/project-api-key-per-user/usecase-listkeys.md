# T13 project/api-key per user: usecase `listkeys`

Plan usecase. Wajib mengikuti `overview.md` di folder yang sama (bila bertentangan, overview menang). Bergantung pada foundation (Langkah 1 overview): `base.OwnerScope`, `Actor.OwnerScope()`, migrasi index owner.

## 1. Metadata

| Atribut | Nilai |
|---|---|
| Ticket | T13 (revisi T04) |
| Scope | backend (frontend ada di `frontend-plan.md`) |
| Domain / Module | project / api-key (`apikey`) |
| Feature | `listkeys` |
| Resource root | `backend/internal/domain/project/internal/apikey/listkeys` |
| Kriteria penerimaan T13 | 1, 6 (bagian "lihat"), 7, 10 |

| Endpoint | Method | Version | Perubahan |
|---|---|---|---|
| `/api/projects/{projectID}/api-keys` | GET | tanpa prefix versi (`/api`); `api-key.yml` `info.version` 0.1.0 tetap | Isi tersaring per pemilik; bentuk respons sama |

Bus: tidak ada kontrak baru atau berubah. Yang dikonsumsi tetap `project.management.check-access` lewat `base/access.Require`. Audit: tidak ada (GET tidak diaudit).

## 2. Summary Plan

Di T04, `GET .../api-keys` mengembalikan semua key project kepada setiap anggota. T13 mengubahnya: user biasa hanya menerima key dengan `created_by` = dirinya; Super Admin menerima semua key (termasuk key legacy `created_by IS NULL`, K4).

Prinsip:

- Aturan visibilitas hanya di `base.OwnerScope` (foundation). Usecase tidak memeriksa `Role`: ia memanggil `actor.OwnerScope()` lalu meneruskan hasilnya ke repository.
- Filter di SQL, dua konstanta SQL (K2). Repository memilih berdasarkan `scope.All`. DILARANG `($2 = '' OR created_by::text = $2)` dan DILARANG menyaring di memori.
- Fail-closed: `projectID` bukan uuid, `scope.Empty()`, atau (`!scope.All` dan `OwnerID` bukan uuid) -> `[]base.Key{}` non-nil tanpa query.
- Urutan di usecase: `ActorFrom` (tanpa identitas -> 401) -> `access.Require` (non-anggota -> `ErrProjectNotFound` 404) -> `actor.OwnerScope()` -> repo.
- `listColumns` tidak berubah (tanpa `secret_hash`). Urutan tetap `(revoked_at IS NULL) DESC, created_at DESC, api_keys.id`.
- Handler, `init.go`, DTO, dan respons REST tidak berubah.

## 3. Plan (langkah berurutan)

Prasyarat: foundation sudah ada di branch (`base/owner.go` dengan `OwnerScope`, `Empty()`, `Actor.OwnerScope()`; migrasi `api_keys_owner_idx`). Bila belum, kerjakan Langkah 1 overview lebih dulu.

1. **Tes dulu (merah)**: ubah `listkeys/usecase_test.go` (bagian 6.1): fake repo menerima dan merekam `scope`. Tambah kasus scope developer, super_admin, non-anggota, tanpa identitas.
2. **`listkeys/usecase.go`**:
   - Ubah interface:
     ```go
     type KeyLister interface {
         ListByProject(ctx context.Context, projectID string, scope base.OwnerScope) ([]base.Key, error)
     }
     ```
   - `Execute(ctx, projectID)` (signature publik tetap): `ActorFrom` -> `access.Require` -> `scope := actor.OwnerScope()` -> `u.repo.ListByProject(ctx, projectID, scope)` -> map ke `KeyDTO`. Tanpa `if actor.Role == ...`, tanpa penyaringan hasil.
   - Perbarui komentar `Execute`: "key milik pemanggil; super_admin semua".
3. **`listkeys/repository.go`**:
   - Pertahankan `listColumns` dan `listByProjectSQL` apa adanya (jalur `scope.All`).
   - Tambah:
     ```go
     const listByOwnerSQL = `SELECT ` + listColumns + `
     FROM api_keys
     WHERE project_id = $1 AND created_by = $2::uuid
     ORDER BY (revoked_at IS NULL) DESC, created_at DESC, api_keys.id`
     ```
   - `ListByProject(ctx, projectID string, scope base.OwnerScope)`:
     ```go
     if !store.ValidUUID(projectID) || scope.Empty() {
         return []base.Key{}, nil
     }
     if scope.All {
         keys, err = r.s.SelectKeys(ctx, listByProjectSQL, projectID)
     } else {
         if !store.ValidUUID(scope.OwnerID) {
             return []base.Key{}, nil
         }
         keys, err = r.s.SelectKeys(ctx, listByOwnerSQL, projectID, scope.OwnerID)
     }
     ```
     Error dibungkus `fmt.Errorf("listkeys: select keys: %w", err)` seperti sebelumnya. Hasil kosong dari `SelectKeys` harus tetap slice non-nil (perilaku T04 yang ada; pertahankan tesnya).
   - Key legacy `created_by NULL` tidak cocok `created_by = $2::uuid`, jadi hanya muncul di jalur All (K4) tanpa kode tambahan.
4. **Helper tes** `listkeys/integration_helpers_test.go` (tag `integration`): tambah `mkOwnedKey(t, d, pid, name, prefix, ownerID, ownerLabel string, created time.Time, revokedAt *time.Time) string` yang mengisi `created_by` dan `created_by_label`. `mkKey` lama (created_by NULL) dipertahankan untuk kasus legacy. Tidak ada FK `created_by` ke tabel user (modul project hanya mereferensikan `project_id`), jadi UUID bebas dipakai. Definisikan konstanta `userA = "0192f5a0-aaaa-7aaa-8bbb-00000000000a"`, `userB = "0192f5a0-bbbb-7aaa-8bbb-00000000000b"`.
5. **`listkeys/repository_integration_test.go`**: sesuaikan pemanggilan lama dengan `base.OwnerScope{All: true}`; tambah kasus isolasi (bagian 6.2).
6. **`listkeys/listkeys_integration_test.go`** (HTTP): ubah `X-Test-User` dari `"A"`/`"B"` menjadi UUID nyata (`userA`, `userB`) karena `OwnerID` non-uuid sekarang menghasilkan daftar kosong. ACL map dan awalan `S` untuk super_admin disesuaikan (mis. `S` + uuid, atau cek role lewat header terpisah `X-Test-Role`). Perbarui `TestT04_AC2_NoSecretAndOrderingAndIsolation` yang kini mengandalkan B melihat semua key (lihat bagian 6.3). Tambah kasus AC1/AC6/AC7/AC10.
7. **`listkeys/handler_test.go`** (unit, fake): sesuaikan `fakeRepo` baru; tambah kasus bahwa handler meneruskan identitas ke scope (bagian 6.3).
8. **Verifikasi**: `go test ./backend/internal/domain/project/internal/apikey/listkeys/...`, lalu `go test -tags integration` pada package yang sama, `go vet`, `go test ./backend/internal/archtest/...`. Pastikan `grep -rn "ListByProject(" backend/` tidak menyisakan pemanggil dengan signature lama (mis. fake di `apikey_integration_test.go` tingkat module; bila ada, itu milik plan `createkey` untuk disentuh, koordinasikan: jangan ubah file itu di plan ini, cukup laporkan).

Kontrak bus: tidak ada perubahan.

## 4. Migration

Tidak ada migrasi di usecase ini. Bergantung pada migrasi foundation `backend/migrations/project/20261022160000000_api_keys_owner_scope.sql` (menambah `api_keys_owner_idx (project_id, created_by, created_at DESC)` yang dipakai `listByOwnerSQL`). Catatan aturan repo (`.claude/rules/migrations.md`): timestamp migrasi foundation wajib di-generate saat implementasi; bila berubah dari nilai di overview, usecase ini tidak terpengaruh (tidak merujuk nama file).

## 5. Changes (per file)

| File | Jenis | Isi |
|---|---|---|
| `.../apikey/listkeys/usecase.go` | ubah | Interface `KeyLister` menerima `scope base.OwnerScope`; `Execute` menambah `actor.OwnerScope()` |
| `.../apikey/listkeys/repository.go` | ubah | Tambah `listByOwnerSQL`; `ListByProject` memilih SQL dari `scope.All`; fail-closed |
| `.../apikey/listkeys/usecase_test.go` | ubah | Fake repo merekam scope; kasus baru |
| `.../apikey/listkeys/repository_integration_test.go` | ubah | Kasus isolasi dua user, legacy, All |
| `.../apikey/listkeys/listkeys_integration_test.go` | ubah | Identitas uuid, kasus HTTP dua user |
| `.../apikey/listkeys/integration_helpers_test.go` | ubah | `mkOwnedKey`, konstanta user |
| `.../apikey/listkeys/handler_test.go` | ubah | Fake repo baru |
| `.../apikey/listkeys/handler.go`, `init.go` | TIDAK berubah | |
| `.../apikey/base/**` | TIDAK disentuh di plan ini | Milik foundation |

## 6. Test Scenario

### 6.1 Unit: `usecase_test.go` (fake access dan fake repo)

`fakeRepo` merekam `calls`, `project`, `scope`. `authCtxAs(id, role)` membangun identitas via `reqctx.WithIdentity`.

| Kasus | Setup | Harapan |
|---|---|---|
| Tanpa identitas | `context.Background()` | `errs.CodeUnauthorized` (401), access dan repo tidak dipanggil |
| Developer | Identity `{UserID: "u-a", Role: "developer"}` | repo dipanggil 1x dengan `scope == OwnerScope{OwnerID: "u-a"}`; `All == false` |
| Super Admin | `{UserID: "u-s", Role: "super_admin"}` | `scope == OwnerScope{All: true}`, `OwnerID == ""` |
| Role kosong/tak dikenal | `{UserID: "u-x", Role: ""}` | `scope == {OwnerID: "u-x"}` (bukan All) |
| Non-anggota | access mengembalikan `base.ErrProjectNotFound` | error itu diteruskan (`errors.Is`), `repo.calls == 0` (scope tidak dihitung/dipakai) |
| Error infra access | access mengembalikan `errors.New("boom")` | diteruskan apa adanya (bukan `*errs.Error`), `repo.calls == 0` |
| Access menerima actor | developer, project `proj-1` | `access.actor == Actor{ID:"u1", Role:"developer", Label:"a@tim.id"}` dan project benar (kasus T04 dipertahankan) |
| Urutan | fake mencatat urutan panggilan | `access.Require` lebih dulu dari repo |
| Tanpa filter memori | repo mengembalikan 3 key (`CreatedBy` campur: sendiri, orang lain, nil) untuk developer | Hasil memuat ketiganya persis (usecase tidak menyaring; saringan tanggung jawab repo). Ini menjaga agar filter tidak diam-diam pindah ke memori |
| Map DTO | kasus T04 (aktif, dicabut, last_used_at) | Dipertahankan |
| Kosong | repo `nil` atau `[]` | `Items` non-nil, JSON `{"items":[]}` |
| Error repo | repo error | dibungkus, `errors.Is(err, boom)`, bukan `*errs.Error`, `Items == nil` |
| Tanpa field rahasia | key dengan `SecretHash` terisi | JSON tidak memuat `secret`, `hash`, `raw_key`, byte hash |

### 6.2 Integration repository: `repository_integration_test.go` (tag `integration`, DB test + migrasi nyata)

Seed: project P1 dan P2; user A (`userA`) dan B (`userB`).

| Kasus | Seed | Harapan |
|---|---|---|
| Isolasi A | P1: A punya `a1` (aktif), `a2` (dicabut); B punya `b1` (aktif) | `ListByProject(P1, {OwnerID: userA})` -> persis `a1`, `a2`; tanpa `b1` |
| Isolasi B | seed sama | `{OwnerID: userB}` -> hanya `b1` |
| Scope All | seed di atas + key legacy `legacy` (`created_by NULL`) | `{All: true}` -> `a1`, `a2`, `b1`, `legacy` |
| Legacy tersembunyi | seed sama | `{OwnerID: userA}` dan `{OwnerID: userB}` tidak memuat `legacy` |
| Urutan aktif dulu | A: `kA`(t0), `kB`(t0+1h), `kC` dicabut (t0+2h), `kD` dicabut (t0-2h), `kE`(t0) | Urutan: `kB`, `kA`/`kE` (tiebreak id naik), `kC`, `kD`; berlaku di jalur owner dan All (salin kasus `TestRepo_OrderAndRevoked` T04 untuk kedua jalur) |
| Project lain tidak ikut | A punya key di P1 dan P2 | List P1 scope A hanya key P1; scope All P1 tidak memuat key P2 |
| Kosong | P3 tanpa key | slice non-nil panjang 0 untuk kedua jalur |
| projectID bukan uuid | `"bukan-uuid"` | slice kosong non-nil, nil error, tanpa query |
| Scope kosong | `OwnerScope{}` | slice kosong non-nil, tanpa query (fail-closed, walau P1 berisi key) |
| OwnerID bukan uuid | `{OwnerID: "bukan-uuid"}` | slice kosong non-nil, nil error (tanpa error cast Postgres) |
| `All` menang atas `OwnerID` | `{All: true, OwnerID: userA}` | Dokumentasikan perilaku: semua key (All dicek lebih dulu). Tes ini mengunci bahwa `OwnerScope()` tidak pernah menghasilkan kombinasi itu |
| secret_hash tidak di-scan | key dengan hash non-nol | `len(got[i].SecretHash) == 0` pada kedua jalur; `listByProjectSQL` dan `listByOwnerSQL` tidak mengandung `secret_hash` |
| Field cabut | key dicabut | `RevokedAt`, `RevokedBy`, `RevokedByLabel` terisi, zona UTC; `CreatedBy`/`CreatedByLabel` terisi pada key berpemilik |
| Pemakaian index (opsional) | `EXPLAIN` tidak wajib | Dilewati; cukup pastikan query valid |

### 6.3 Handler dan HTTP integrasi

`handler_test.go` (unit, fake): `fakeRepo` baru. Kasus T04 dipertahankan (OK, items array kosong, param `projectID`, mapping error 404/401/500 dengan `trace_id` dan tanpa bocor pesan internal). Tambah: identitas developer `u1` -> `repo.scope == {OwnerID: "u1"}`; identitas super_admin -> `{All: true}`.

`listkeys_integration_test.go` (tag `integration`, router chi + `Init` + bus fake `check-access`, identitas lewat header `X-Test-User` yang dipetakan ke `reqctx.WithIdentity`):

| Kasus | Setup | Harapan |
|---|---|---|
| AC1 dua user | A anggota (Owner), B anggota (Member) di P1; A punya `ka`, B punya `kb` | GET sebagai A: 200, `items` hanya `ka`; sebagai B: hanya `kb` |
| AC1 user tanpa key | C anggota tanpa key | 200 `{"items":[]}` |
| AC6 Super Admin | + key legacy (`created_by NULL`) | GET sebagai super_admin: `ka`, `kb`, legacy; legacy memuat `created_by: null`, `created_by_label: null` |
| AC7 non-anggota | D bukan anggota P1 | 404 `project tidak ditemukan`; uuid acak/`bukan-uuid` -> 404; tanpa identitas -> 401 |
| Identik 404 non-anggota | D ke P1 vs project tidak ada | Body sama setelah `trace_id` dihapus |
| AC10 tanpa rahasia | key dengan hash non-nol dan prefix dikenal | Body tidak memuat `secret`, `hash`, `raw_key`, hex hash; tiap item punya persis 11 field (`id,name,prefix,status,created_by,created_by_label,created_at,last_used_at,revoked_at,revoked_by,revoked_by_label`) |
| Isolasi project | key milik A di P2 | A (anggota P1 saja) ke P2 -> 404; GET P1 tidak memuat key P2 |
| Urutan | aktif, aktif lebih baru, dicabut | urutan aktif dulu lalu `created_at` turun (kasus T04 diadaptasi dengan `mkOwnedKey` milik A, pemanggil A) |
| Keanggotaan dicabut | `delete(acl, B|P1)` | B 404, A 200 |
| Check-access gagal | bus error / handler tidak terdaftar | 500 `INTERNAL`, tanpa bocor pesan |

Catatan adaptasi: `TestT04_AC2_NoSecretAndOrderingAndIsolation` memakai `mkKey` (created_by NULL) dan membaca sebagai B (developer). Setelah T13, key itu tidak terlihat B. Ganti: seed dengan `mkOwnedKey` milik pemanggil, atau baca sebagai super_admin untuk kasus key legacy. Jangan melonggarkan scope demi tes.

Regresi lintas: `verifykey/verify_integration_test.go` tidak disentuh dan harus tetap hijau (dijalankan di langkah 4 overview).

## 7. E2E Scenarios

Spec Playwright dimiliki plan frontend (`frontend/e2e/project-api-key.spec.ts`); bagian ini mendefinisikan perilaku `listkeys` yang diuji.

Positif:
1. Developer A dan B anggota project P; masing-masing membuat key (`a-key`, `b-key`). Tab API Keys sebagai A hanya menampilkan `a-key`; sebagai B hanya `b-key`.
2. Super Admin membuka tab API Keys P: melihat `a-key` dan `b-key`, kolom "Dibuat oleh" berisi email A dan B.
3. User anggota tanpa key melihat keadaan kosong (`emptyHint`), bukan error.
4. Setelah A mencabut `a-key`, A masih melihatnya dengan status "dicabut" (daftar mencakup key dicabut milik sendiri).

Negatif:
1. Non-anggota membuka tab/memanggil `GET .../api-keys`: 404 project.
2. Respons API sebagai B tidak memuat `a-key`, `prefix`, atau `id` key A (periksa teks body).
3. Tanpa sesi: 401, UI diarahkan ke login (perilaku T04).

## 8. Request/Response body

Format error (tidak berubah, `infra/http/respond`): `{"error": {"code", "message", "details"}, "trace_id"}`.

### GET `/api/projects/{projectID}/api-keys`

Request: tanpa body, cookie sesi. `GET` tidak memerlukan `X-Requested-With`.

200 sebagai developer A (hanya key milik A; aktif dulu, lalu `created_at` turun):
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

200 sebagai Super Admin: bentuk sama, `items` memuat key semua anggota, termasuk key legacy:
```json
{
  "items": [
    {
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
    {
      "id": "0192f5a0-2222-7aaa-8bbb-000000000009",
      "name": "legacy-key",
      "prefix": "z9y8x7w6",
      "status": "active",
      "created_by": null,
      "created_by_label": null,
      "created_at": "2026-10-22T08:00:00Z",
      "last_used_at": null,
      "revoked_at": null,
      "revoked_by": null,
      "revoked_by_label": null
    }
  ]
}
```

200 developer tanpa key:
```json
{ "items": [] }
```

401 (tanpa sesi):
```json
{ "error": { "code": "UNAUTHORIZED", "message": "perlu login", "details": null }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```

404 (non-anggota atau project tidak ada/bukan uuid):
```json
{ "error": { "code": "NOT_FOUND", "message": "project tidak ditemukan", "details": null }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```

500 (gangguan infra, pesan internal tidak bocor):
```json
{ "error": { "code": "INTERNAL", "message": "terjadi kesalahan internal", "details": null }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```
(Teks 401/500 mengikuti keluaran `respond.WriteError` yang sudah ada; tes memeriksa `code` dan `trace_id`, bukan teks persis.)

Respons tidak pernah memuat `secret_hash`, `raw_key`, atau key utuh.
