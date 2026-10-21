# T13 project/api-key per user — Usecase `createkey`

Plan usecase. Mengikuti `overview.md` (foundation) di folder yang sama; bila bertentangan, overview yang menang. Tidak ada perubahan struktur: `handler.go`, `params.go`, `init.go` di `createkey/` tidak disentuh.

---

## 1. Metadata

| Atribut | Nilai |
|---|---|
| Ticket | T13 (revisi T04) |
| Scope | backend (usecase ini); fullstack untuk tiket |
| Domain / Module / Feature | project / api-key (`apikey`) / `createkey` |
| Resource root | `backend/internal/domain/project/internal/apikey` |
| Bergantung pada | foundation T13 (migrasi `api_keys_owner_scope`, pesan error baru di `base/errors.go`); T04 merged |
| Kriteria penerimaan T13 | 4, 5, 6 (kuota Super Admin), 8, 9 |

| Endpoint | Method | Version | Perubahan |
|---|---|---|---|
| `/api/projects/{projectID}/api-keys` | POST | tanpa prefix versi (`/api`); `api-key.yml` `info.version` 0.1.0 tetap | Unik nama, kuota, dan advisory lock per `(project, actor.ID)`; kode/reason/bentuk respons sama, hanya pesan error baru (dari foundation) |

Kontrak bus: tidak ada yang baru atau berubah. Yang dikonsumsi tetap: `project.management.check-access` (via `base/access.Require`) dan `core.audit.record` (via `base/audit`, aksi `record.ActionAPIKeyCreated`).

---

## 2. Summary Plan

Satu usecase, satu alur transaksi: `ActorFrom` -> `access.Require` -> `ValidateName` -> loop retry prefix { `InTx`: lock -> count -> `CheckActiveLimit` -> insert } -> audit setelah commit. Alur ini TIDAK dipecah dan urutannya tidak berubah. Yang berubah hanya cakupan lock dan count:

- `Tx.LockProjectKeys(ctx, projectID)` diganti `Tx.LockOwnerKeys(ctx, projectID, ownerID)`.
- `Tx.CountActive(ctx, projectID)` menjadi `Tx.CountActive(ctx, projectID, ownerID)`.
- `ownerID = actor.ID` untuk semua role, termasuk Super Admin (K5). `createkey` TIDAK memakai `base.OwnerScope` (itu untuk baca/cabut); membuat selalu atas nama diri sendiri.
- Advisory lock key menjadi `'apikey:' || project || ':' || owner`, sehingga A dan B tidak saling menunggu, sedangkan dua request A tetap diserialkan (tes kuota serentak tetap deterministik).
- Keunikan nama dijaga index `api_keys_project_active_name_key (project_id, created_by, name) WHERE revoked_at IS NULL` (migrasi foundation). Nama constraint sama, jadi pemetaan `store.ConstraintActiveName` -> `base.ErrNameTaken` di `Insert` tidak berubah.
- `limit` (`APIKeyMaxActivePerProject`) tidak berganti nama; maknanya kini per user per project.
- Tidak ada perubahan: retry prefix collision, `ValidateName`, `CheckActiveLimit`, audit `apikey.created` (aktor = pemanggil, fail-open, setelah commit), respons sekali tampil (`raw_key`, `mcp_url`, `Cache-Control: no-store`).

Catatan perilaku: Super Admin yang membuat key mendapat `created_by = id Super Admin`, dihitung ke kuota Super Admin sendiri, tidak mengurangi kuota A/B.

Catatan konkurensi: dua request dari user yang sama tetap berebut lock yang sama (serial). Dua user berbeda memegang lock berbeda; hash 64-bit dari string `apikey:<project>:<owner>` praktis tanpa tabrakan, dan tabrakan hanya berdampak menunggu, bukan salah hitung.

---

## 3. Plan (langkah berurutan)

Prasyarat: foundation (langkah 1 overview) sudah merge/ada di branch: `base/errors.go` berpesan baru dan migrasi owner-scope ada di `backend/migrations/project/`. Tanpa migrasi, tes integrasi di bawah gagal (A dan B nama sama -> NAME_TAKEN).

1. **`createkey/usecase.go`**
   - Ubah interface `Tx`:
     ```go
     type Tx interface {
         LockOwnerKeys(ctx context.Context, projectID, ownerID string) error
         CountActive(ctx context.Context, projectID, ownerID string) (int, error)
         Insert(ctx context.Context, k base.Key) (base.Key, error)
     }
     ```
   - Di `Execute`, dalam closure `InTx`: `tx.LockOwnerKeys(ctx, projectID, actor.ID)`, `tx.CountActive(ctx, projectID, actor.ID)`. Pesan wrap error ("createkey: lock", "createkey: count") tidak berubah.
   - Tambahkan komentar pada `Tx`: "ownerID = pemanggil (actor.ID) untuk semua role; kuota dan unik nama per user per project (T13 K5)."
   - Jangan menambah cabang berdasarkan `actor.Role` / `IsSuperAdmin()`. Jangan menyentuh `NewKey`, `CheckActiveLimit`, loop retry, atau audit.
   - Komentar doc paket `createkey` tetap.
2. **`createkey/repository.go`**
   ```go
   qLock  = `SELECT pg_advisory_xact_lock(hashtextextended('apikey:' || $1::text || ':' || $2::text, 0))`
   qCount = `SELECT count(*) FROM api_keys WHERE project_id = $1 AND created_by = $2::uuid AND revoked_at IS NULL`
   ```
   - `LockProjectKeys` diganti `LockOwnerKeys(ctx, projectID, ownerID)` -> `ExecContext(ctx, qLock, projectID, ownerID)`.
   - `CountActive(ctx, projectID, ownerID)` -> `QueryRowxContext(ctx, qCount, projectID, ownerID)`.
   - `qIns`, `Insert`, pemetaan `ConstraintActiveName`/`ConstraintPrefixUnique`/FK: tidak berubah.
   - `actor.ID` selalu UUID dari sesi (kolom `created_by` juga `uuid` dan Insert sudah mensyaratkan hal yang sama). Tidak ada fail-closed tambahan di sini; ID non-UUID sudah gagal di Insert pada T04 dan tetap demikian.
3. **`createkey/usecase_test.go`**: perbarui `fakeTx` (`LockOwnerKeys`, `CountActive` merekam argumen) dan tambah tes baru (bagian Test Scenario 5.1).
4. **`createkey/handler_test.go`**: perbarui asersi pesan `ErrNameTaken` dan `ErrKeyLimitReached` ke pesan baru; kode/status/reason tetap (5.2).
5. **`createkey/repository_integration_test.go`** (tag `integration`): tambah tes repository dan konkurensi per user, serta tes migrasi pada data T04 (5.3).
6. **`apikey_integration_test.go`** (tingkat module; hanya usecase ini yang menyentuh file ini): tambah skenario HTTP dua user, Super Admin, dan regresi verify (5.4). Tes T04 yang ada (AC4, AC5, dst.) memakai satu user dan tetap hijau; ubah hanya asersi yang memuat pesan lama bila ada (`grep -rn "key aktif lain\|key aktif per project" backend/`).
7. **`verifykey/verify_integration_test.go`**: tambah regresi (5.5). Tidak mengubah kode `verifykey`.
8. Verifikasi lokal: `go build ./...`, `go vet ./...`, `go test ./backend/internal/domain/project/internal/apikey/...`, `go test -tags integration ./backend/internal/domain/project/internal/apikey/...`, `go test ./backend/internal/archtest/...`, `grep -rn "LockProjectKeys" backend/` harus kosong.

Tidak ada kontrak bus, handler, route, atau OpenAPI yang diubah di usecase ini (deskripsi OpenAPI dikerjakan foundation).

---

## 4. Migration

Tidak ada migrasi baru di usecase ini. Migrasi `backend/migrations/project/<VERSION>_api_keys_owner_scope.sql` dimiliki foundation. Catatan penamaan sesuai `.claude/rules/migrations.md`: versi 17 digit di-generate saat implementasi (bukan nilai bulat di overview), jadi tes di bawah tidak boleh mengacu nama file; cukup `db.Up(...)` yang menerapkan semua migrasi.

Isi yang diandalkan usecase ini (untuk referensi):
```sql
DROP INDEX IF EXISTS api_keys_project_active_name_key;
CREATE UNIQUE INDEX IF NOT EXISTS api_keys_project_active_name_key
    ON api_keys (project_id, created_by, name) WHERE revoked_at IS NULL;
CREATE INDEX IF NOT EXISTS api_keys_owner_idx ON api_keys (project_id, created_by, created_at DESC);
```
Tes migrasi pada data T04 (kriteria 9) dieksekusi di usecase ini, lihat 5.3.

---

## 5. Test Scenario

### 5.1 `createkey/usecase_test.go` (unit, fake Tx)

Ubah `fakeTx`: `LockOwnerKeys(_, projectID, ownerID)` dan `CountActive(_, projectID, ownerID)` menyimpan argumen ke `fakeRepo` (mis. `lockArgs [][2]string`, `countArgs [][2]string`) dan tetap menambah journal "lock"/"count". Tes lama (`SuccessOrderAndResult`, `Limit`, `PrefixCollisionRetry`, `InfraErrors`, dst.) tetap berlaku; urutan journal tetap `access,tx,lock,count,insert,tx-end,audit`.

| Tes | Setup | Harapan |
|---|---|---|
| `TestExecute_LockAndCountUseProjectAndActor` | identitas `user-b` developer | lock dan count dipanggil tepat sekali dengan `(pid, "user-b")`; key yang di-insert `CreatedBy == "user-b"` |
| `TestExecute_SuperAdminUsesOwnID` | identitas `user-s` role `super_admin` | lock dan count `(pid, "user-s")`; bukan string kosong; `CreatedBy == "user-s"`; sukses, audit `ActionAPIKeyCreated` dengan aktor Super Admin |
| `TestExecute_LimitReached_DetailsLimit` | `rp.count = 20`, limit 20 | `errs.CodeConflict`, `details.reason == KEY_LIMIT_REACHED`, `details.limit == 20`; tidak ada insert, tidak ada audit; pesan = pesan baru ("per user per project") |
| `TestExecute_LimitBoundary` | count 19 sukses, count 20 gagal (limit 20) | seperti `TestExecute_Limit` yang ada |
| `TestExecute_NameTakenPassedThrough` | `insertErrs[0] = base.ErrNameTaken` | error yang sama diteruskan apa adanya (`errors.Is`/reason `NAME_TAKEN`), tanpa retry (`txCalls == 1`), tanpa audit |
| `TestExecute_PrefixCollisionRetry_RelocksSameOwner` | `insertErrs = {ErrPrefixCollision}` | tx dijalankan 2x; kedua lock dan count dipanggil dengan `(pid, actor.ID)` yang sama; sukses attempt kedua; audit sekali |
| `TestExecute_AccessDenied_NothingElse` (ada) | access error | tidak ada tx, lock, count, insert, audit |
| `TestExecute_NoIdentity_Unauthorized` (ada) | tanpa identitas | 401, tidak ada lock |
| `TestExecute_InfraErrors` (ada) | lockErr / countErr | error di-wrap, tanpa audit |

### 5.2 `createkey/handler_test.go`

- `TestHandler_ErrorMapping`: 409 NAME_TAKEN -> message `nama key sudah dipakai key aktif Anda yang lain di project ini`, `details.reason == NAME_TAKEN`; 409 KEY_LIMIT_REACHED -> message `batas 20 key aktif per user per project tercapai; cabut key yang tidak dipakai`, `details == {reason, limit: 20}`. Status, code `CONFLICT`, reason tetap. Tes lain (Created, BadBody, UserAgentTruncated, `Cache-Control: no-store`) tidak berubah.

### 5.3 `createkey/repository_integration_test.go` (tag `integration`)

Helper baru: `mkKeyFor(t, pid, name, ownerID, label)` yang memanggil `base.NewKey(..., base.Actor{ID: ownerID, Label: label}, ...)` dan TIDAK menol-kan `CreatedBy` (helper `mkKey` lama tetap, memakai `created_by NULL`). `created_by` tanpa FK ke users (migrasi T04), jadi UUID fiksi cukup. Konstanta `ownerA = "0192f5a0-aaaa-7aaa-8bbb-00000000000a"`, `ownerB = ...0b`, `ownerS = ...5a`. `execute` ditambah varian `executeAs(uc, pid, userID, role, name)`.

| Tes | Skenario | Harapan |
|---|---|---|
| `TestRepo_CountActive_PerOwner` | A punya 2 aktif + 1 dicabut, B punya 1 aktif, 1 key legacy `created_by NULL` aktif | `CountActive(pid, A)=2`, `(pid, B)=1`, `(pid, S)=0`; key legacy tidak dihitung ke siapa pun |
| `TestRepo_CountActive_ScopedToProject` | A punya key di P1 dan P2 | hitungan per project terpisah |
| `TestRepo_InsertSameNameDifferentOwners` | A dan B insert `ci` di project yang sama | keduanya sukses |
| `TestRepo_InsertSameNameSameOwner_NameTaken` | A insert `ci` dua kali | kedua -> `base.ErrNameTaken` (pemetaan `ConstraintActiveName` tetap) |
| `TestRepo_NameReusableAfterRevoke` | A insert `ci`, set `revoked_at`, insert `ci` lagi | sukses |
| `TestRepo_LockOwnerKeys_Independent` | tx1 mengunci `(P, A)` dan menahan; tx2 mengunci `(P, B)` | tx2 tidak menunggu (selesai dalam batas waktu pendek, mis. 2 dtk); tx3 `(P, A)` menunggu sampai tx1 commit (diverifikasi dengan channel/urutan, bukan sleep tebal) |
| `TestUsecase_ConcurrentLimit` (ada) | 8 goroutine, user sama (A), limit 1 | tepat 1 sukses, 7 `KEY_LIMIT_REACHED`, aktif=1; diulang 5 ronde. Tetap hijau tanpa ubahan selain helper |
| `TestUsecase_ConcurrentLimit_SameOwnerTwice` | A x2 serentak, limit 1, banyak ronde (mis. 10) | tepat satu sukses, satu `KEY_LIMIT_REACHED` |
| `TestUsecase_ConcurrentDifferentOwners_BothSucceed` | A dan B serentak, nama berbeda dan juga nama sama, limit 1 | keduanya sukses, aktif=2; banyak ronde |
| `TestUsecase_LimitPerOwner` | limit 1: A pertama sukses, A kedua `KEY_LIMIT_REACHED` (details.limit=1), B masih sukses | sesuai |
| `TestUsecase_SuperAdminQuotaSeparate` | limit 1: A sudah 1 key; Super Admin membuat key | sukses; A tetap `KEY_LIMIT_REACHED` pada key kedua; Super Admin kedua -> `KEY_LIMIT_REACHED`; key S `created_by == ownerS`; kuota A tidak berkurang (A mencabut lalu bisa buat lagi tanpa pengaruh S) |
| `TestMigration_OwnerScope_OnT04Data` | Lihat di bawah | Lihat di bawah |

`TestMigration_OwnerScope_OnT04Data` (kriteria 9; membuktikan migrasi aman pada data lama):
1. `db.TestDB(t)`, terapkan migrasi hanya sampai T04 (memakai fasilitas runner untuk berhenti sebelum versi migrasi owner-scope; bila runner tidak mendukung batas versi, jalankan `db.Up` penuh lalu pulihkan keadaan T04 dengan `DROP INDEX api_keys_project_active_name_key; CREATE UNIQUE INDEX api_keys_project_active_name_key ON api_keys (project_id, name) WHERE revoked_at IS NULL; DROP INDEX IF EXISTS api_keys_owner_idx`, lalu isi data).
2. Isi data T04: dua key aktif nama berbeda (dibuat user A dan B), satu key legacy aktif `created_by NULL`, satu key dicabut bernama sama dengan key aktif.
3. Jalankan ulang isi migrasi owner-scope (baca dari file lewat glob `*_api_keys_owner_scope.sql` di `backend/migrations/project`, bukan nama tetap). Tanpa error; jumlah baris tetap (4); semua kolom utuh.
4. `pg_indexes`: `api_keys_project_active_name_key` memuat `(project_id, created_by, name)` dan `WHERE (revoked_at IS NULL)`; `api_keys_owner_idx` ada.
5. Menjalankan isi migrasi sekali lagi tidak error (idempoten).
6. Setelah migrasi: insert dua key aktif nama sama oleh user berbeda sukses; oleh user sama -> `store.Violation` mengembalikan constraint `store.ConstraintActiveName` dan `Insert` memetakannya ke `ErrNameTaken`.

### 5.4 `apikey_integration_test.go` (HTTP end-to-end in-process; user `uidA`, `uidB` developer, `uidS` super_admin; `e.p1`)

| Tes | Skenario | Harapan |
|---|---|---|
| `TestT13_AC4_SameNameAcrossUsers` | A dan B (keduanya anggota `p1`) `create` nama `shared` | keduanya 201 |
| `TestT13_AC4_SameUserSameName_NameTaken` | A `create` `ci` dua kali | kedua 409, `NAME_TAKEN`, pesan baru; B `create` `ci` tetap 201 |
| `TestT13_AC4_RevokedNameReusableByOwner` | A buat `ci`, A revoke, A buat `ci` lagi | 201 |
| `TestT13_AC5_LimitPerUser` | `setup(t, 1)`: A buat `a1` 201; A buat `a2` -> 409 `KEY_LIMIT_REACHED` dengan `limit:1`; B buat `b1` 201 | sesuai; setelah A revoke `a1`, A buat `a2` 201 |
| `TestT13_AC5_ConcurrentSameUser` | `setup(t, 1)`, A x2 serentak (banyak ronde, project baru tiap ronde dengan acl A) | kode terurut `[201, 409]`, aktif=1 |
| `TestT13_AC5_ConcurrentDifferentUsers` | `setup(t, 1)`, A dan B serentak (banyak ronde) | keduanya 201, aktif=2 |
| `TestT13_AC6_SuperAdminQuotaOwn` | `setup(t, 1)`: A buat 1 key; Super Admin buat 1 key 201; Super Admin kedua 409 `KEY_LIMIT_REACHED`; A kedua 409; B 201 | kuota A/B tidak terpengaruh key Super Admin dan sebaliknya; key S berisi `created_by == uidS` (cek via list Super Admin) |
| `TestT13_AC4_ErrorDoesNotLeakOtherUsersName` | B punya `rahasia`; A buat `rahasia` | 201 (tidak ada NAME_TAKEN yang membocorkan); body 409 pada kasus A duplikat sendiri tidak memuat nama/ID user lain |
| `TestT13_AC9_ConstraintMappingStill` | dua user nama sama OK; satu user nama sama -> `NAME_TAKEN` (sudah di atas) | memastikan `ConstraintActiveName` tetap dipetakan |
| Pembaruan tes T04 | Asersi pesan lama (bila ada) diganti pesan baru | tes T04 lain tidak diubah |

### 5.5 Regresi `verifykey/verify_integration_test.go`

- `TestVerify_KeysFromDifferentOwnersVerifyToProject`: A dan B (anggota `p1`) membuat key lewat usecase `createkey` (atau helper env yang memanggilnya), `bus project.apikey.verify` untuk `raw_key` A dan B: `Found == true`, `ProjectID == p1`, `KeyName` benar, `KeyID` sesuai. Setelah A mencabut key A: verify key A `Found == false` (mengikuti perilaku T04), verify key B tetap `Found == true`. Tidak ada perubahan kode `verifykey`.
- Tes MCP T07 (`mcp-gateway`) dijalankan di langkah verifikasi akhir tiket; tidak diubah di sini.

### 5.6 Regresi lain

- `go test ./backend/internal/archtest/...` hijau (tidak ada impor baru di `createkey`).
- `countkeys` tes tetap hijau tanpa ubahan (K7).
- Tes yang memakai `LockProjectKeys` tidak boleh tersisa (`grep` kosong).

---

## 6. E2E Scenarios

Spec `frontend/e2e/project-api-key.spec.ts` dimiliki plan frontend; usecase ini memberi skenario yang harus dicakup untuk createkey (backend sudah dites lewat 5.4).

Positif:
1. Developer A dan B anggota project P. A membuat `shared-name` lewat dialog "Buat key": sukses, dialog menampilkan raw key sekali. B membuat `shared-name` di P: sukses (tidak ada NAME_TAKEN).
2. Kuota per user: dengan env e2e `API_KEY_MAX_ACTIVE_PER_PROJECT=2` (bila tersedia; jika tidak, lewat API helper), A membuat 2 key; key ke-3 A ditolak; B masih bisa membuat key.
3. A mencabut `shared-name` miliknya, lalu membuat `shared-name` lagi: sukses.
4. Super Admin membuat key di P: sukses; key muncul di tab API Keys Super Admin dengan kolom "Dibuat oleh" berisi email Super Admin; kuota A tidak berubah.
5. Regresi MCP: key buatan A dan B (`createKeyApi`) masing-masing tetap memberi akses MCP ke P (`frontend/e2e/mcp-gateway.spec.ts`).

Negatif:
1. A membuat key kedua bernama sama saat yang pertama masih aktif: dialog menampilkan "Anda sudah punya key aktif dengan nama ini" (`project.apikey.error.NAME_TAKEN`); key tidak bertambah.
2. A mencapai batas: dialog menampilkan "Batas {limit} key aktif per user tercapai. Cabut key yang tidak dipakai." (`project.apikey.error.KEY_LIMIT_REACHED`).
3. Nama tidak valid (1 karakter / 61 karakter): 400 `INVALID_ARGUMENT` dengan `details.fields.name`, dialog menampilkan galat field (tidak berubah).
4. Non-anggota memanggil POST: 404 `project tidak ditemukan` (tidak berubah).
5. Tanpa header `X-Requested-With`: ditolak CSRF seperti T04 (tidak berubah).

---

## 7. Request/Response body

Format error baku (`infra/http/respond`): `{"error": {"code", "message", "details"}, "trace_id"}`. Request memakai cookie sesi dan header `X-Requested-With: XMLHttpRequest`.

### POST `/api/projects/{projectID}/api-keys`

Request:
```json
{ "name": "claude-code-budi" }
```

201 (bentuk tidak berubah; `Cache-Control: no-store`; B membuat key di project P):
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

201 oleh Super Admin: bentuk sama, `created_by` = id Super Admin, `created_by_label` = email Super Admin.

400 (tidak berubah):
```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "permintaan tidak valid", "details": { "fields": { "name": "nama key wajib 2-60 karakter" } } }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```

401 (tanpa sesi):
```json
{ "error": { "code": "UNAUTHORIZED", "message": "perlu login", "details": null }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```

404 (non-anggota / project tidak ada; tidak berubah):
```json
{ "error": { "code": "NOT_FOUND", "message": "project tidak ditemukan", "details": null }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```

409 NAME_TAKEN (pesan baru; berlaku hanya bila pemanggil sendiri sudah punya key aktif bernama sama):
```json
{ "error": { "code": "CONFLICT", "message": "nama key sudah dipakai key aktif Anda yang lain di project ini", "details": { "reason": "NAME_TAKEN" } }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```

409 KEY_LIMIT_REACHED (pesan baru; limit = `API_KEY_MAX_ACTIVE_PER_PROJECT`, kini per user):
```json
{ "error": { "code": "CONFLICT", "message": "batas 20 key aktif per user per project tercapai; cabut key yang tidak dipakai", "details": { "reason": "KEY_LIMIT_REACHED", "limit": 20 } }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```

500 (prefix collision habis percobaan / infra; tidak berubah, tanpa detail internal):
```json
{ "error": { "code": "INTERNAL", "message": "terjadi kesalahan internal", "details": null }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```

Bus: tidak ada request/response baru. `project.apikey.verify`, `touch`, `count-keys` tidak berubah.

---

## 8. Changes (per file)

| File | Jenis |
|---|---|
| `backend/internal/domain/project/internal/apikey/createkey/usecase.go` | ubah: `Tx.LockOwnerKeys`, `Tx.CountActive(ctx, projectID, ownerID)`, pemanggilan dengan `actor.ID` |
| `backend/internal/domain/project/internal/apikey/createkey/repository.go` | ubah: `qLock`, `qCount`, `LockOwnerKeys`, `CountActive` |
| `backend/internal/domain/project/internal/apikey/createkey/usecase_test.go` | ubah `fakeTx`; tes baru 5.1 |
| `backend/internal/domain/project/internal/apikey/createkey/handler_test.go` | ubah asersi pesan |
| `backend/internal/domain/project/internal/apikey/createkey/repository_integration_test.go` | tes baru 5.3 (termasuk migrasi pada data T04) |
| `backend/internal/domain/project/internal/apikey/apikey_integration_test.go` | tes baru 5.4 |
| `backend/internal/domain/project/internal/apikey/verifykey/verify_integration_test.go` | tes regresi 5.5 |
| `createkey/init.go`, `createkey/handler.go`, `createkey/params.go`, `base/**`, `lib/constants/bus.go`, `lib/contracts/**`, migrasi, OpenAPI, dokumen | TIDAK diubah di usecase ini (milik foundation atau tidak berubah) |

## 9. Risiko dan catatan

- Antara foundation merge dan usecase ini selesai, index sudah per user tetapi lock/count masih per project: aman (lebih ketat). Usecase ini boleh paralel dengan `listkeys`/`revokekey` karena file tidak tumpang tindih.
- Jangan mengubah `Insert` agar memetakan constraint lain; satu-satunya kegagalan unique nama berasal dari index bernama `api_keys_project_active_name_key`.
- Super Admin yang membuat key banyak memakai kuota sendiri, bukan kuota pemilik lain; sesuai K5.
- Key legacy `created_by NULL` tidak masuk hitungan siapa pun (dan, karena NULL dianggap berbeda oleh unique index, tidak menimbulkan konflik nama).
