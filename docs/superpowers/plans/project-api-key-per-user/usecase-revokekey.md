# T13 project/api-key — Usecase `revokekey` (`POST /api/projects/{projectID}/api-keys/{keyID}/revoke`)

Melengkapi `docs/superpowers/plans/project-api-key-per-user/overview.md` (FOUNDATION). Bila bertentangan, overview menang. Prasyarat: langkah 1 foundation sudah ada, khususnya `base.OwnerScope`, `Actor.OwnerScope()`, `OwnerScope.Permits`, `OwnerScope.Empty` (`base/owner.go`). Bila simbol itu belum ada, hentikan dan laporkan; jangan membuatnya di luar allowedPaths. Hanya satu usecase: `revokekey`. Tidak menyentuh `listkeys`, `createkey`, atau plan lain.

## Metadata

| Field | Nilai |
|---|---|
| Ticket | T13 (revisi T04) |
| Scope | backend (frontend di `frontend-plan.md`) |
| Domain | project |
| Module | api-key (`apikey`) |
| Feature | `revokekey` |
| Endpoint | `/api/projects/{projectID}/api-keys/{keyID}/revoke` |
| Method | POST (tanpa body, tidak di-decode) |
| Version | Tanpa prefix versi (`/api`); `api-key.yml` `info.version` 0.1.0 tetap |
| Auth | `RequireAuth` + CSRF (`X-Requested-With`), tidak berubah. Otorisasi di usecase: `access.Require` lalu scope pemilik |
| Respons sukses | 200 `{key: KeyDTO}` (bentuk tidak berubah) |
| Audit | `apikey.revoked` (`record.ActionAPIKeyRevoked`), setelah commit, fail-open, aktor = pemanggil |
| Bus dikonsumsi | `project.management.check-access` (via `base/access`), `core.audit.record` (via `base/audit`). Tidak ada perubahan kontrak bus |
| Bus disediakan | tidak ada |
| Tabel | `api_keys` (`SELECT ... FOR UPDATE`, `UPDATE`); tanpa perubahan skema (indeks dari migrasi foundation) |
| Kriteria T13 | 2, 3, 6 (cabut), 7 |
| Resource root | `backend/internal/domain/project/internal/apikey` |
| allowedPaths | `.../apikey/revokekey/` saja |

## Summary Plan

Sebelum T13, anggota mana pun dapat mencabut key anggota lain. Setelah T13:

- User biasa hanya dapat mencabut key dengan `created_by = actor.ID`. Key milik orang lain, key project lain, key legacy `created_by IS NULL`, dan keyID acak/bukan uuid semuanya menghasilkan `base.ErrKeyNotFound` (404 `KEY_NOT_FOUND`) dengan respons identik (K3). Pemanggil tidak dapat membedakan "milik orang lain" dari "tidak ada".
- Super Admin (`scope.All`) dapat mencabut key siapa pun, termasuk legacy. Audit mencatat Super Admin sebagai aktor, `revoked_by`/`revoked_by_label` juga Super Admin (sudah dilakukan `Key.Revoke(actor, now)`).
- Key milik sendiri yang sudah dicabut tetap 409 `ALREADY_REVOKED`. Untuk key orang lain yang sudah dicabut, user biasa tetap 404 (tidak boleh bocor status).
- Aturan visibilitas hanya di `base.OwnerScope`. Usecase tidak memeriksa `Role`. Repository hanya membaca `scope.All`/`scope.OwnerID` untuk memilih SQL.
- Filter pemilik diterapkan di SQL (dua konstanta), bukan di memori. Pertahanan berlapis: setelah `found=true`, usecase tetap memeriksa `scope.Permits(k)` sebelum `k.Revoke`, sehingga bila repository salah (mis. regresi SQL) usecase tetap fail-closed.
- Urutan keputusan (jangan diubah): `ActorFrom` (401) -> `access.Require` (non-anggota = 404 project, tanpa tx) -> `actor.OwnerScope()` -> `InTx { LockInProject(scope) -> found? -> Permits? -> Revoke -> MarkRevoked }` -> audit setelah commit.
- Konkurensi tidak berubah: `FOR UPDATE` menyerialkan dua revoke pada key yang sama (satu 200, satu 409, satu audit). Baris yang tidak lolos filter pemilik tidak dikunci sama sekali, jadi user B tidak dapat menahan lock atas key A.
- Handler, `params`, `init.go`, `base.go` modul TIDAK berubah. `MarkRevoked` tidak berubah.

## Plan

Urutan wajib, TDD: tulis/ubah tes dahulu (merah), baru implementasi.

**R0. Verifikasi (tanpa edit).** Pastikan ada `base.OwnerScope{All, OwnerID}`, `base.Actor.OwnerScope()`, `OwnerScope.Permits(Key)`, `OwnerScope.Empty()`, `store.ValidUUID`, `store.GetKey`, `store.KeyColumns`, migrasi foundation (`api_keys_owner_idx`). Jalankan `go test ./backend/internal/domain/project/internal/apikey/revokekey/` untuk baseline hijau.

**R1. Ubah tes dahulu** (lihat bagian Test Scenario): perbarui fake `LockInProject` ke signature baru, perbaiki tes lama yang mengasumsikan "siapa pun boleh mencabut", tambah tes baru. Pastikan gagal kompilasi/merah sebelum implementasi.

**R2. `revokekey/usecase.go`**

1. Ubah kontrak `txRepo`:
   ```go
   // LockInProject: SELECT ... WHERE id AND project_id [AND created_by] FOR UPDATE.
   // scope.All -> tanpa filter pemilik; selain itu hanya baris created_by = scope.OwnerID.
   // keyID/projectID bukan uuid, scope.Empty(), atau OwnerID bukan uuid -> (zero, false, nil) TANPA query.
   LockInProject(ctx context.Context, projectID, keyID string, scope base.OwnerScope) (base.Key, bool, error)
   ```
   `MarkRevoked` dan `repository` tetap.
2. `Execute`, urutan persis:
   1. `actor, ok := base.ActorFrom(reqctx.IdentityFrom(ctx))`; `!ok` -> `errs.New(errs.CodeUnauthorized, "perlu login")`.
   2. `u.access.Require(ctx, in.ProjectID, actor)`; error dikembalikan apa adanya.
   3. `scope := actor.OwnerScope()`.
   4. `InTx`:
      - `k, found, err := tx.LockInProject(ctx, in.ProjectID, in.KeyID, scope)`; error -> `fmt.Errorf("revokekey: lock key: %w", err)`; `!found` -> `base.ErrKeyNotFound`.
      - `if !scope.Permits(k) { return base.ErrKeyNotFound }` (pertahanan berlapis, SEBELUM `Revoke`; tidak memanggil `MarkRevoked`).
      - `k.Revoke(actor, u.clock())` (409 bila sudah dicabut).
      - `tx.MarkRevoked(ctx, k)` seperti sekarang.
   5. Error `InTx` diteruskan seperti sekarang (non-`*errs.Error` dibungkus `revokekey: tx: %w`).
   6. Audit hanya setelah commit sukses, `Actor: actor` (pemanggil, termasuk Super Admin), `Args {name, prefix}` seperti sekarang. Kegagalan (401/404/409/500) = 0 audit.
   7. `return revoked.ToDTO(), nil`.
3. Tidak ada perubahan imports selain yang sudah ada (`base` sudah diimpor). Tidak ada pengecekan `actor.Role` di file ini.

**R3. `revokekey/repository.go`**

1. Pertahankan `lockInProjectSQL` (untuk `scope.All`) persis:
   `SELECT <KeyColumns> FROM api_keys WHERE id = $1 AND project_id = $2 FOR UPDATE`.
2. Tambah:
   ```go
   const lockOwnedInProjectSQL = `SELECT ` + store.KeyColumns +
       ` FROM api_keys WHERE id = $1 AND project_id = $2 AND created_by = $3::uuid FOR UPDATE`
   ```
   DILARANG pola `($3 = '' OR created_by::text = $3)` (overview 4.2).
3. Implementasi:
   ```go
   func (t *txRepoImpl) LockInProject(ctx context.Context, projectID, keyID string, scope base.OwnerScope) (base.Key, bool, error) {
       if !store.ValidUUID(keyID) || !store.ValidUUID(projectID) || scope.Empty() {
           return base.Key{}, false, nil
       }
       if scope.All {
           return t.s.GetKey(ctx, lockInProjectSQL, keyID, projectID)
       }
       if !store.ValidUUID(scope.OwnerID) {
           return base.Key{}, false, nil
       }
       return t.s.GetKey(ctx, lockOwnedInProjectSQL, keyID, projectID, scope.OwnerID)
   }
   ```
   Cek `scope.All` ditaruh sebelum validasi `OwnerID` agar Super Admin tidak terpengaruh `OwnerID` kosong. `scope.Empty()` fail-closed sebelum query.
4. `markRevokedSQL` dan `MarkRevoked` TIDAK berubah (baris sudah dikunci dan diverifikasi dalam tx yang sama).

**R4. Perbaiki fixture tes lama** (wajib agar suite hijau; detail di Test Scenario).

**R5. Verifikasi**
- `go test ./backend/internal/domain/project/internal/apikey/revokekey/...`
- `go test -tags integration ./backend/internal/domain/project/internal/apikey/revokekey/...` (butuh DB test + migrasi foundation).
- `go test ./backend/internal/archtest/...`, lint, `go vet`.
- Tinjau bahwa `grep -n "Role" revokekey/usecase.go` kosong (tak ada pemeriksaan peran di usecase).

## Migration

Tidak ada di usecase ini. Migrasi `backend/migrations/project/20261022160000000_api_keys_owner_scope.sql` dimiliki foundation (menambah `api_keys_owner_idx (project_id, created_by, created_at DESC)`; query lock `id = $1 AND project_id = $2 AND created_by = $3` memakai PK `id`, sehingga indeks baru tidak wajib untuk revoke). Tes integrasi di sini menjalankan `db.Up` sehingga memakai skema final.

## Changes

| File | Perubahan |
|---|---|
| `revokekey/usecase.go` | `txRepo.LockInProject` menerima `scope base.OwnerScope`; `Execute` memanggil `actor.OwnerScope()`, meneruskan ke repo, `scope.Permits(k)` sebelum `Revoke` |
| `revokekey/repository.go` | tambah `lockOwnedInProjectSQL`; `LockInProject` memilih SQL dari `scope.All`; fail-closed (`Empty`, uuid tidak valid) tanpa query |
| `revokekey/usecase_test.go` | fake `LockInProject` merekam `scope` dan memfilter sesuai scope; skenario baru; perbaiki skenario lama |
| `revokekey/repository_integration_test.go` | helper `key` menerima `owner` (kolom `created_by`); skenario isolasi/legacy/konkurensi baru; perbaiki skenario lama |
| `revokekey/handler_test.go` | skenario byte-identik 404 memakai `Usecase` asli + fake repo ber-scope |
| `handler.go`, `init.go`, `params` (bila ada), `../base.go` | TIDAK berubah |

## Test Scenario

### Unit: `usecase_test.go` (fake txRepo)

Perubahan fake: `fakeTx` menyimpan `lockScope base.OwnerScope`; `LockInProject(_, p, k, scope)` mensimulasikan repo nyata: mengembalikan `found=false` bila `!scope.All && (key.CreatedBy == nil || *key.CreatedBy != scope.OwnerID)` (opsi: bendera `ignoreScope bool` untuk menguji pertahanan berlapis, mengembalikan key apa adanya walau scope tak cocok). Tambah penghitung `lockCalls`.

Perbaikan tes lama:
- `activeKey()` harus `CreatedBy = "u1"` (sama dengan `UserID` di `authed()`), karena sekarang hanya pemilik yang boleh mencabut. Update asersi `lockArgs` jadi `(p1, k1)` + `lockScope == base.OwnerScope{OwnerID: "u1"}`.
- Tes lama yang mengasumsikan "aktor bukan pembuat tetap boleh mencabut" (AC6 T04) diganti oleh skenario di bawah (jangan dihapus tanpa pengganti).

Skenario baru (tabel):

| Kasus | Setup | Harapan |
|---|---|---|
| Developer meneruskan scope owner | `authed()` developer `u1` | `lockScope == {OwnerID:"u1"}`, `All=false` |
| Milik sendiri sukses | key `CreatedBy=u1` | DTO `revoked`; `MarkRevoked` 1x; audit 1x; urutan `access,begin,lock,mark,commit,audit` |
| Key orang lain | key `CreatedBy="u2"`, fake memfilter -> `found=false` | `errors.Is(err, base.ErrKeyNotFound)`; `markedCalls == 0`; audit 0; tidak ada `commit` |
| Key legacy (`CreatedBy=nil`) oleh developer | fake memfilter | `ErrKeyNotFound`; mark 0; audit 0 |
| Pertahanan berlapis | fake `ignoreScope=true` mengembalikan key `u2` | `ErrKeyNotFound` (dari `Permits`), `markedCalls == 0`, audit 0 |
| Super Admin cabut key orang lain | identitas `{UserID:"su", Email:"su@tim.id", Role:"super_admin"}`, key `CreatedBy=u2` | sukses; `lockScope == {All:true}`; `marked.RevokedBy == "su"`, `RevokedByLabel == "su@tim.id"`; `audit.Events[0].Actor.ID == "su"`; `Actor.Role == "super_admin"` |
| Super Admin cabut key legacy | `CreatedBy=nil` | sukses, audit 1x |
| Sudah dicabut milik sendiri | key `CreatedBy=u1`, `RevokedAt` terisi | `ErrAlreadyRevoked`; mark 0; audit 0 |
| Sudah dicabut milik orang lain (developer) | key `u2` revoked, fake memfilter | `ErrKeyNotFound` (bukan 409; tidak bocor status) |
| Non-anggota | `acc.err = base.ErrProjectNotFound` | `ErrProjectNotFound`; `inTxCall == 0`; audit 0 (tes `AccessErrors` yang ada tetap) |
| Tidak login | ctx kosong | 401; tak ada panggilan (tetap) |
| Key tidak ada | `found=false` | `ErrKeyNotFound` (tetap) |
| Error lock/mark/commit | seperti tes lama | dibungkus, 0 audit (tetap) |
| Hasil `ErrKeyNotFound` identik | bandingkan error kasus "orang lain" dan "tidak ada" | pointer sentinel sama (`==`/`errors.Is` ke `base.ErrKeyNotFound`) |

### Integration: `repository_integration_test.go` (tag `integration`)

Perubahan helper: `key(t, pid, owner, name)` memasukkan `created_by` (`$n::uuid`, `nil` untuk legacy). Buat dua user uuid tetap: A `0192f5a0-aaaa-7aaa-8bbb-00000000000a`, B `0192f5a0-bbbb-7aaa-8bbb-00000000000b`, Super Admin `0192f5a0-cccc-7aaa-8bbb-00000000000c`. `uctx()` diparameterkan oleh (id, email, role). Perbaiki tes lama agar key dibuat dengan owner = user yang mencabut (kecuali yang memang menguji lintas user).

Skenario:
1. `Revoke_SuccessThenConflict`: A membuat/mencabut key A -> 200; ulang -> `ErrAlreadyRevoked`; baris tidak berubah; audit tepat 1.
2. `CrossUser_NotFound_RowUntouched`: key A; B `Execute` -> `ErrKeyNotFound`; baris key A tetap `revoked_at IS NULL`, `revoked_by IS NULL`; audit 0.
3. `Lock_ScopeB_OnKeyA_NotFoundAndNotLocked`: langsung pada `txRepoImpl`. Tx 1 (goroutine) mengunci key A dengan scope A dan menahan. Di tx 2 `LockInProject(scope B)` atas key A kembali cepat dengan `found=false` (tidak menunggu lock; beri timeout 2 dtk, gagal bila terblokir). Setelah tx 1 selesai, baris tidak berubah.
4. `Lock_ScopeAll_FindsLegacy_DeveloperDoesNot`: key `created_by NULL`; scope `{All:true}` -> found; scope `{OwnerID:A}` -> `found=false`; Super Admin `Execute` sukses dan mencabut key legacy.
5. `SuperAdmin_RevokesOtherUsersKey`: key B dicabut Super Admin -> sukses; `revoked_by == SuperAdmin`, `created_by` tetap B; audit 1x (aktor Super Admin ditangkap via `syncAudit` yang merekam event).
6. `Lock_FailClosed_NoQuery`: `OwnerScope{}` -> `found=false`; `{OwnerID:"bukan-uuid"}` -> `found=false`; keyID bukan uuid -> `found=false`; tanpa error SQL (cast uuid tak pernah dijalankan).
7. `ProjectIsolation`: key A di project P1, revoke lewat P2 (anggota sama) -> `ErrKeyNotFound`; Super Admin juga `ErrKeyNotFound` karena project beda.
8. `ConcurrentRevokeExactlyOne`: dua revoke serentak key A oleh A -> tepat satu sukses, satu `ErrAlreadyRevoked`, audit 1 (tes lama dengan owner).
9. `Concurrent_OtherUserCannotBlock`: A dan B serentak: A mencabut key A, B mencabut key A -> B selalu `ErrKeyNotFound`, A sukses; audit 1.
10. `NonMemberGets404`, `NonUUIDKeyID`, `AC4_RevokedNameReusable` (tes lama, key milik pencabut).

### Handler: `handler_test.go`

Tes lama (`fakeUC`) tetap. Tambah pengujian ujung-ke-ujung ringan dengan `NewUsecase` asli + `fakeRepo` ber-scope (in-memory, memfilter seperti repo nyata) + `fakeAccess` lolos + `fakeAudit`, dirutekan lewat chi seperti `serve`, identitas disuntik ke `req.Context()` via `reqctx.WithIdentity`:

1. `B_RevokesKeyOfA_vs_RandomUUID_IdenticalBody`: user B `POST /projects/p1/api-keys/<keyA>/revoke` dan `POST .../<uuid acak>/revoke`. Keduanya status 404. Decode JSON, hapus `trace_id` (bila middleware menambahkannya; di handler test mungkin tidak ada, tetap hapus bila ada), marshal ulang dan bandingkan **byte-identik**: `{"error":{"code":"NOT_FOUND","message":"API key tidak ditemukan","details":{"reason":"KEY_NOT_FOUND"}}}`. Juga uji keyID bukan uuid (`not-a-uuid`) -> body identik.
2. Setelah kasus 1: key A di fake repo masih aktif (`RevokedAt == nil`), `fakeAudit.events` kosong.
3. `OwnerRevokes_200_AuditOnce`: A mencabut key A -> 200 `status=revoked`, audit 1.
4. `SuperAdmin_RevokesB_200`: 200 `revoked_by == super admin id`; audit aktor = Super Admin.
5. `AlreadyRevokedOwn_409`: A mencabut key A yang sudah dicabut -> 409 `ALREADY_REVOKED`.

### Arch/regresi
- `go test ./backend/internal/archtest/...` hijau (tidak ada impor baru lintas module).
- Tes `verifykey` tidak disentuh dan harus tetap hijau.

## E2E Scenarios

Ditulis di `frontend/e2e/project-api-key.spec.ts` (milik plan frontend/foundation; bagian ini mendaftar skenario yang bergantung pada `revokekey`):

Positif:
1. Developer A mencabut `a-key` dari UI: status baris menjadi "dicabut"; aksi tidak muncul untuk key yang sudah dicabut.
2. A mencabut key lalu `revokeKeyApi` kedua kali pada key yang sama: 409 `ALREADY_REVOKED`.
3. Super Admin membuka tab API Keys project P, mencabut key B dari UI: sukses; halaman audit (khusus Super Admin) menampilkan `apikey.revoked` dengan aktor Super Admin.
4. Regresi `mcp-gateway.spec.ts`: key yang dicabut pemiliknya langsung ditolak gateway; key lain tetap berfungsi.

Negatif:
1. B memanggil `revokeKeyApi(P, keyA)`: 404 `KEY_NOT_FOUND`; saat A memuat daftar, `a-key` masih aktif; halaman audit tidak memuat event `apikey.revoked` baru.
2. B memanggil `revokeKeyApi(P, <uuid acak>)`: 404 dengan `error` (code, message, details) identik dengan kasus 1.
3. B memanggil revoke dengan keyID bukan uuid: 404 `KEY_NOT_FOUND` (sama).
4. Non-anggota memanggil revoke: 404 `project tidak ditemukan` (tidak berubah).
5. Tanpa header CSRF / tanpa sesi: ditolak seperti T04 (403 / 401, tidak berubah).

## Request/Response body

Format error umum (`infra/http/respond`): `{"error": {"code", "message", "details"}, "trace_id"}`. Request: `POST /api/projects/{projectID}/api-keys/{keyID}/revoke`, tanpa body, header `X-Requested-With`, cookie sesi.

### 200: A mencabut key A (juga bentuk untuk Super Admin; saat itu `created_by` = pemilik, `revoked_by` = Super Admin)
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

### 200: Super Admin mencabut key B
```json
{
  "key": {
    "id": "0192f5a0-2222-7aaa-8bbb-000000000004",
    "name": "claude-code-budi",
    "prefix": "m7p2x9qa",
    "status": "revoked",
    "created_by": "0192f5a0-bbbb-7aaa-8bbb-00000000000b",
    "created_by_label": "b@tim.id",
    "created_at": "2026-10-23T13:00:00Z",
    "last_used_at": null,
    "revoked_at": "2026-10-23T14:00:00Z",
    "revoked_by": "0192f5a0-cccc-7aaa-8bbb-00000000000c",
    "revoked_by_label": "admin@tim.id"
  }
}
```

### 404: B mencabut key A, ATAU keyID acak/bukan uuid (identik kecuali `trace_id`)
```json
{ "error": { "code": "NOT_FOUND", "message": "API key tidak ditemukan", "details": { "reason": "KEY_NOT_FOUND" } }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```

### 404: non-anggota / project tidak ada
```json
{ "error": { "code": "NOT_FOUND", "message": "project tidak ditemukan", "details": null }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```

### 409: A mencabut ulang key A
```json
{ "error": { "code": "CONFLICT", "message": "API key sudah dicabut", "details": { "reason": "ALREADY_REVOKED" } }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```

### 401: tanpa sesi
```json
{ "error": { "code": "UNAUTHORIZED", "message": "perlu login", "details": null }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```

Bus: tidak ada perubahan kontrak (`project.apikey.verify`, `touch`, `count-keys`, `project.management.check-access`, `core.audit.record` tetap).
