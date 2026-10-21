# T12 auth-purgeinvitations — Plan Usecase

Induk: `docs/superpowers/plans/core-invitation-email/overview.md` (foundation; menang bila bertentangan). Pola acuan kode: `backend/internal/domain/core/internal/auth/purgesessions` (`init.go`, `usecase.go`, `repository.go`, tanpa handler/route).

## 1. Metadata

| Atribut | Nilai |
|---|---|
| Ticket | T12 (K13, kriteria 8 bagian purge) |
| Usecase slug | `auth-purgeinvitations` |
| Scope | backend (tanpa frontend) |
| Domain / Module | core / auth |
| Folder | `backend/internal/domain/core/internal/auth/purgeinvitations` |
| Endpoint HTTP | tidak ada |
| Method | tidak ada |
| Version | tidak ada (tanpa prefix versi) |
| Job | `core.auth.purge-invitations`, `DailyAt: "03:30"` (UTC), didaftarkan di `d.Jobs` |
| Bus dikonsumsi | `project.management.count-memberships` (Call; provider management `countmemberships`) |
| Bus disediakan | tidak ada |
| Audit | tidak ada (job sistem; konsisten dengan `purgesessions`) |
| Bergantung pada | foundation (`Shared.Invitation.PurgeAfter`, tabel `user_invitations`, kontrak `countmemberships`, konstanta bus). Tes integrasi lintas module butuh `management-countmemberships`; tes unit memakai fake. |

## 2. Summary Plan

Job harian menghapus akun undangan yang tidak pernah diterima: `user_invitations.accepted_at IS NULL` dan `expires_at < now - PurgeAfter` (default 30 hari), dan akun tidak punya keanggotaan project.

Fakta yang membentuk desain:

- `project_members.user_id` tanpa FK ke `users` dan auth tidak boleh membaca tabel project. Keanggotaan ditanyakan lewat bus `project.management.count-memberships`. Menghapus user tidak menghapus anggota yatim, jadi cek harus benar sebelum DELETE.
- Fail-closed: bus error, handler error, atau `bus.ErrNotRegistered` berarti tidak ada penghapusan sama sekali pada batch itu dan `Run` mengembalikan error (scheduler mencatat gagal; job jalan lagi besok).
- Cek ulang di SQL: `DELETE` mengulang syarat "undangan menunggu dan lewat batas" supaya akun yang diterima/diundang ulang (Reissue memperpanjang `expires_at`) di antara SELECT dan DELETE tidak terhapus.
- Akun tanpa baris `user_invitations` (`createuser`, `createadmin`) tidak pernah kandidat karena kandidat dipilih dari tabel undangan. Akun yang sudah diterima (`accepted_at` terisi) juga tidak.
- Cascade: `DELETE FROM users` menghapus `sessions` dan `user_invitations` lewat FK `ON DELETE CASCADE` (sudah ada di T01 untuk sessions, di foundation untuk undangan). Tidak ada SQL tambahan.
- Pagination keyset: akun berkeanggotaan > 0 tetap kandidat tiap hari. Tanpa kursor, 500 akun semacam itu akan menghalangi batch selamanya. Karena itu `Run` mengambil batch (LIMIT 500) berurutan memakai kursor `(expires_at, user_id)`, maksimal 20 batch per run (10.000 kandidat), berhenti bila batch kurang dari 500. Ini bukan pemecahan usecase artifisial, melainkan satu loop di `Run`.
- Race yang diterima (bagian 12 overview): member ditambahkan ke akun kedaluwarsa > 30 hari tepat di antara cek count dan DELETE. Tidak ditutup dengan lock lintas module; didokumentasikan di doc comment `Run`.
- Idempoten: run ulang tidak menemukan apa-apa lagi. Log hanya jumlah, tanpa email atau user id.

Rule bisnis: "siapa yang dihapus" adalah rule murni sederhana (tidak ada entity baru). Predikat kedaluwarsa memakai definisi entity `base.Invitation` (`IsPending` + `ExpiresAt`); batas waktu dihitung di usecase (`cutoff = now - PurgeAfter`), strict `<` (tepat di batas tidak dihapus). Tes oracle (bagian 6) memastikan SQL setuju dengan `Invitation`.

## 3. Plan (langkah berurutan)

Prasyarat: foundation 4.1–4.10 selesai (`Shared.Invitation`, migrasi `user_invitations`, konstanta bus `BUS_PROJECT_MANAGEMENT_COUNT_MEMBERSHIPS`, paket kontrak `countmemberships` dengan `MaxUserIDs = 500`).

1. Tes dulu (TDD): `usecase_test.go` dengan `fakeRepo` dan `fakeCounter` (bagian 6).
2. `repository.go`. Struct `Repository{q db.Querier}`, `NewRepository(q)`. Dua method, hanya file ini yang memuat SQL:
   ```go
   type Candidate struct {
       UserID    string
       ExpiresAt time.Time
   }
   // Cursor: nil = dari awal.
   func (r *Repository) ListCandidates(ctx context.Context, cutoff time.Time, after *Candidate, limit int) ([]Candidate, error)
   func (r *Repository) DeleteUsers(ctx context.Context, ids []string, cutoff time.Time) (int64, error)
   ```
   SQL:
   ```sql
   -- listCandidatesSQL: $1 cutoff, $2 after.expires_at (NULL = awal), $3 after.user_id (NULL = awal), $4 limit
   SELECT i.user_id::text AS user_id, i.expires_at
   FROM user_invitations i
   WHERE i.accepted_at IS NULL
     AND i.expires_at < $1
     AND ($2::timestamptz IS NULL OR (i.expires_at, i.user_id) > ($2::timestamptz, $3::uuid))
   ORDER BY i.expires_at, i.user_id
   LIMIT $4

   -- deleteUsersSQL: $1 ids (uuid[]), $2 cutoff. Cek ulang kondisi di SQL.
   DELETE FROM users u
   WHERE u.id = ANY($1::uuid[])
     AND u.role = 'developer'
     AND EXISTS (SELECT 1 FROM user_invitations i
                 WHERE i.user_id = u.id
                   AND i.accepted_at IS NULL
                   AND i.expires_at < $2)
   ```
   - Gunakan `pq.Array`/helper array yang sudah dipakai repo (cek `store`/`listusers`); jangan tambah library. `ids` kosong: return `0, nil` tanpa query.
   - `ListCandidates` memakai `r.q.SelectContext` ke struct bertag `db`. Error dibungkus `fmt.Errorf("purgeinvitations: ...: %w", err)`.
   - `u.role = 'developer'` adalah pengaman tambahan: Super Admin tidak pernah dihapus job walau suatu hari tertulis baris undangan.
3. `memberships.go`. Adapter bus (satu-satunya file yang menyentuh bus):
   ```go
   type busCounter struct{ b *bus.Registry }
   func (c busCounter) Count(ctx context.Context, userIDs []string) (map[string]int, error) {
       res, err := bus.CallTyped[countmemberships.Response](ctx, c.b,
           constants.BUS_PROJECT_MANAGEMENT_COUNT_MEMBERSHIPS,
           countmemberships.Request{UserIDs: userIDs})
       if err != nil { return nil, fmt.Errorf("purgeinvitations: count-memberships: %w", err) }
       return res.Counts, nil
   }
   ```
   Import feature ini terbatas: `lib/constants`, `lib/contracts/project/management/countmemberships`, `infra/bus` (aturan antar module). `ErrNotRegistered` terbungkus `%w`, jadi `errors.Is(err, bus.ErrNotRegistered)` tetap benar.
4. `usecase.go`. Orkestrasi saja:
   ```go
   const (
       JobName    = "core.auth.purge-invitations"
       DailyAt    = "03:30"
       batchSize  = 500 // harus <= countmemberships.MaxUserIDs (dijaga tes)
       maxBatches = 20
   )
   type repository interface {
       ListCandidates(ctx context.Context, cutoff time.Time, after *Candidate, limit int) ([]Candidate, error)
       DeleteUsers(ctx context.Context, ids []string, cutoff time.Time) (int64, error)
   }
   type membershipCounter interface {
       Count(ctx context.Context, userIDs []string) (map[string]int, error)
   }
   type Usecase struct { repo repository; counter membershipCounter; clock base.Clock; purgeAfter time.Duration; log *slog.Logger }
   func New(repo repository, counter membershipCounter, clock base.Clock, purgeAfter time.Duration, log *slog.Logger) *Usecase // panic bila purgeAfter <= 0
   func (u *Usecase) Run(ctx context.Context) error
   ```
   Alur `Run` (clock dibaca SEKALI, pola purgesessions):
   1. `start := u.clock()`, `cutoff := start.Add(-u.purgeAfter)`; `ctx.Err()` -> return error terbungkus.
   2. Loop sampai `maxBatches`: `cands := repo.ListCandidates(cutoff, cursor, batchSize)`; error -> return (log sebagian bila sudah ada yang terhapus). Kosong -> berhenti.
   3. `counts := counter.Count(ids)`. Error (termasuk `ErrNotRegistered`) -> log error (`"auth purge invitations dibatalkan: count-memberships gagal"`, jumlah terhapus sejauh ini, tanpa id), `return fmt.Errorf("purgeinvitations: %w", err)`. Tidak ada DELETE pada batch ini. Batch sebelumnya yang sudah commit tetap sah (setiap batch lengkap telah dicek).
   4. `deletable` = id dengan `counts[id] == 0` (kunci absen = 0, sesuai kontrak). `counts[id] > 0` dihitung `retained`.
   5. `n := repo.DeleteUsers(deletable, cutoff)`; error -> return terbungkus. `deleted += n`.
   6. Geser kursor ke elemen terakhir `cands`; bila `len(cands) < batchSize` berhenti.
   7. Log info `"auth purge invitations selesai"` dengan `candidates`, `retained`, `deleted`, `batches`, `duration_ms` (pola `purgesessions`: `wall := time.Now()`). Tanpa email, tanpa user id.
   - Doc comment `Run` menyebut race count-lalu-delete dan bahwa DELETE mengecek ulang kondisi.
5. `init.go`:
   ```go
   func newJob(run func(context.Context) error) jobs.Job { return jobs.Job{Name: JobName, DailyAt: DailyAt, Run: run} }
   func Init(d app.Deps, s base.Shared) {
       uc := New(NewRepository(d.DB), busCounter{b: d.Bus}, s.Clock, s.Invitation.PurgeAfter, d.Log)
       d.Jobs.Register(newJob(uc.Run))
   }
   ```
   Tanpa handler, route, atau endpoint bus.
6. Wiring: `backend/internal/domain/core/internal/auth/base.go` tambah import `purgeinvitations` dan `purgeinvitations.Init(d, shared)` tepat setelah `purgesessions.Init(d, shared)` (baris ~90). Pastikan `Shared.Invitation.PurgeAfter` terisi dari `cfg.InvitationPurgeAfter` oleh foundation `buildShared`.
7. Dokumentasi: tidak ada perubahan OpenAPI (tanpa HTTP). Katalog bus README (foundation) sudah memuat consumer `purgeinvitations`.
8. Jalankan `go build ./...`, `go vet ./...`, tes unit, lalu `-tags integration`, dan `internal/archtest`.

## 4. Migration

Tidak ada. Memakai tabel `user_invitations` (foundation, `backend/migrations/core/<VERSION>_create_user_invitations.sql`) dan FK cascade yang sudah ada. Index parsial `user_invitations_pending_expires_idx (expires_at) WHERE accepted_at IS NULL` dari foundation melayani query kandidat. Tes integrasi memverifikasi bahwa `sessions` ikut terhapus (FK cascade T01); bila ternyata bukan CASCADE, itu bug migrasi T01 yang harus dilaporkan, bukan diakali di repository.

## 5. Changes

| File | Aksi | Isi |
|---|---|---|
| `backend/internal/domain/core/internal/auth/purgeinvitations/init.go` | baru | `JobName`, `DailyAt`, `newJob`, `Init` |
| `.../purgeinvitations/usecase.go` | baru | `Usecase`, `New`, `Run`, antarmuka `repository` dan `membershipCounter` |
| `.../purgeinvitations/repository.go` | baru | `Candidate`, `ListCandidates`, `DeleteUsers` (satu-satunya SQL) |
| `.../purgeinvitations/memberships.go` | baru | adapter `busCounter` ke `project.management.count-memberships` |
| `.../purgeinvitations/usecase_test.go` | baru | unit dengan fake repo dan fake counter |
| `.../purgeinvitations/memberships_test.go` | baru | unit adapter dengan `bus.Registry` nyata |
| `.../purgeinvitations/init_test.go` | baru | unit: `newJob` nama dan jadwal |
| `.../purgeinvitations/repository_integration_test.go` | baru | tag `integration` |
| `.../purgeinvitations/init_integration_test.go` | baru | tag `integration` |
| `backend/internal/domain/core/internal/auth/base.go` | ubah | import dan `purgeinvitations.Init(d, shared)` |
| `backend/internal/domain/core/internal/auth/base_test.go` | ubah | bila ada tes yang menghitung job/Init terdaftar, tambahkan ekspektasi job baru |

Tidak diubah: `base/`, kontrak (milik foundation/management), `lib/constants/bus.go` (foundation), audit, OpenAPI.

## 6. Test Scenario

### Unit (`usecase_test.go`, `memberships_test.go`, `init_test.go`)

Fake: `fakeRepo` merekam `ListCandidates(cutoff, after, limit)` dan `DeleteUsers(ids, cutoff)`, mengembalikan batch yang diskrip; `fakeCounter` mengembalikan `map` skrip atau error, merekam jumlah panggilan.

| Skenario | Ekspektasi |
|---|---|
| Anggota > 0 tidak dihapus | kandidat A (count 2), B (absen), C (count 0): `DeleteUsers` menerima `[B, C]`; log `retained=1 deleted=2` |
| Semua kandidat punya anggota | `DeleteUsers` tidak dipanggil atau dipanggil dengan slice kosong tanpa efek; `Run` nil |
| Bus gagal (error generik) | `Run` error, `DeleteUsers` 0 panggilan, error membungkus penyebab (`errors.Is`) |
| Bus `ErrNotRegistered` | sama: fail-closed, `errors.Is(err, bus.ErrNotRegistered)` true |
| Handler bus mengembalikan `*errs.Error` | tidak dihapus, error diteruskan |
| Batas waktu tepat | clock `T`, `PurgeAfter=720h`: `ListCandidates` menerima `cutoff == T-720h` (satu kali pembacaan clock); tes oracle di integrasi memastikan `expires_at == cutoff` tidak terpilih |
| Clock dibaca sekali | counter pemanggilan clock == 1 walau banyak batch |
| Tidak ada kandidat | `Run` nil, counter 0 panggilan, log `candidates=0 deleted=0` |
| Pagination | repo skrip 2 batch penuh (500) lalu 3: kursor batch 2 = elemen terakhir batch 1; berhenti setelah batch < 500; counter dipanggil per batch dengan <= 500 id |
| Batch pertama sudah dihapus lalu batch kedua count gagal | `Run` error; batch pertama tercatat di log sebagian (`deleted` sejauh ini); tidak ada DELETE batch kedua |
| `maxBatches` | repo yang selalu mengembalikan batch penuh: berhenti tepat di 20 batch, `Run` nil |
| Repo `ListCandidates` error | `Run` error, counter tidak dipanggil |
| Repo `DeleteUsers` error | `Run` error terbungkus |
| ctx sudah dibatalkan | error, repo tidak dipanggil |
| `New` panic | `purgeAfter <= 0` panic |
| `batchSize <= countmemberships.MaxUserIDs` | assert konstanta (mencegah drift kontrak) |
| Log tanpa PII | buffer slog tidak memuat `@` maupun user id kandidat |
| Idempoten | run kedua dengan repo kosong: nil, tanpa efek |
| Adapter `busCounter` | registry nyata tanpa handler -> `errors.Is(err, bus.ErrNotRegistered)`; handler terdaftar mengembalikan `Counts` -> diteruskan; request memuat `UserIDs` utuh; handler error -> dibungkus |
| `newJob` | `Name == "core.auth.purge-invitations"`, `DailyAt == "03:30"`, `Run != nil` |

### Integrasi repo (`repository_integration_test.go`, `db.TestDB` + `db.Migrate`, pola `purgesessions`)

Seed: helper `seedUser(email, role)`, `seedInvitation(userID, expiresAt, acceptedAt)`, `seedSession(userID)`. `N` tetap, `cutoff = N - 720h`.

| Skenario | Ekspektasi |
|---|---|
| Akun diterima (`accepted_at` terisi, `expires_at` 40 hari lalu) | bukan kandidat; `DeleteUsers([id], cutoff)` mengembalikan 0; user tetap ada |
| Akun `createuser` (tanpa baris undangan) | bukan kandidat; `DeleteUsers` 0; user tetap ada |
| Akun belum lewat 30 hari (`expires_at = cutoff + 1s`) dan `expires_at` 5 hari lalu | tidak kandidat, tidak terhapus |
| Batas tepat (`expires_at == cutoff`) | tidak kandidat (strict `<`), tidak terhapus |
| `expires_at = cutoff - 1s`, belum diterima | kandidat; `DeleteUsers` mengembalikan 1; user dan baris undangan hilang |
| Cascade | seed 2 sesi untuk akun terhapus: `sessions` untuk user itu 0; sesi user lain utuh |
| Cek ulang di SQL | kandidat terpilih lalu diterima (`UPDATE accepted_at`) sebelum `DeleteUsers`: 0 terhapus. Diundang ulang (`expires_at` dimajukan ke masa depan) sebelum `DeleteUsers`: 0 terhapus |
| Super Admin dengan baris undangan kedaluwarsa | tidak terhapus (guard `role='developer'`) |
| Hanya id yang diberikan | dua kandidat valid, `DeleteUsers([satu])` hanya menghapus satu |
| `ids` kosong | 0 tanpa error, tanpa query |
| Pagination keyset | seed 5 kandidat dengan `expires_at` kembar sebagian; `ListCandidates(limit=2)` tiga kali dengan kursor mengembalikan semua 5 tanpa duplikat dan terurut `(expires_at, user_id)` |
| Oracle vs entity | kombinasi `expires_at ∈ {cutoff-1s, cutoff, cutoff+1s}` x `accepted_at ∈ {nil, set}`: terpilih jika dan hanya jika `Invitation.IsPending()` dan `ExpiresAt.Before(cutoff)` |
| Plan query | `EXPLAIN` tidak diwajibkan; cukup pastikan query jalan pada 1.000 baris di bawah 1 detik |
| Lintas module (bila `management-countmemberships` sudah ada) | registry nyata dengan handler fake count: akun berkeanggotaan tidak dihapus, akun tanpa keanggotaan dihapus lewat `Usecase.Run` penuh |

### Integrasi init (`init_integration_test.go`)

| Skenario | Ekspektasi |
|---|---|
| Job terdaftar | `Init(deps, shared)` dengan `jobs.New(d, log)` berhasil; `Init` kedua panic (nama duplikat), pola `TestInit_RegistersJob` purgesessions |
| Nama dan jadwal | `jobs` mengekspos daftar job terdaftar (atau gunakan `newJob(nil)`): `Name == JobName` dan `DailyAt == "03:30"`. Bila `jobs.Scheduler` tidak mengekspos daftar, andalkan tes `newJob` unit plus panic duplikat |
| `Shared.Invitation.PurgeAfter` nol | `Init` panic dari `New` (kesalahan wiring), diuji sekali |
| Wiring penuh | `auth.Register` pada `app.Deps` lengkap tidak panic dan memuat kedua job purge (`core.auth.purge-sessions`, `core.auth.purge-invitations`) |

## 7. E2E Scenarios

Dijalankan terhadap server nyata + Postgres + modul management terpasang. Job dipicu langsung lewat `Usecase.Run`/scheduler uji (jam dimajukan), karena tidak ada endpoint HTTP. Mengacu kriteria 8 tiket.

Positif:
1. Owner mengundang `lama@contoh.id` ke project P, lalu membatalkan undangan (keanggotaan hilang, akun tetap). `user_invitations.expires_at` di-set 31 hari lalu. Job jalan -> user, baris undangan, dan sesi (bila ada) terhapus; email `lama@contoh.id` kini bisa diundang lagi sebagai akun baru (`created=true`).
2. Akun undangan kedaluwarsa > 30 hari, tidak pernah punya keanggotaan (diundang dari halaman Users) -> terhapus.
3. Job dijalankan dua kali berturut-turut -> run kedua `deleted=0`, tidak error (idempoten).
4. Lebih dari 500 kandidat (seed 520, 10 di antaranya berkeanggotaan): semua 510 tanpa keanggotaan terhapus dalam satu run (dua batch), 10 tetap ada.

Negatif:
5. Akun kedaluwarsa > 30 hari tetapi masih menjadi anggota project (undangan tidak dibatalkan) -> tidak terhapus; keanggotaan dan akun utuh; log `retained>=1`.
6. Akun kedaluwarsa 29 hari -> tidak terhapus; akun tepat 30 hari (`expires_at == now - 720h`) -> tidak terhapus.
7. Akun yang sudah diterima (login pertama + ganti password) walau `expires_at` lama -> tidak terhapus. Akun `createuser` tanpa baris undangan -> tidak terhapus.
8. Management tidak terpasang / `count-memberships` tidak terdaftar, atau handler mengembalikan error -> tidak ada akun yang terhapus (hitung `users` sebelum dan sesudah sama), `Run` mengembalikan error, job tercatat gagal, log tidak memuat email.
9. Akun kedaluwarsa lama tetapi diundang ulang sebelum job (K6, `expires_at` diperpanjang) -> tidak terhapus (cek ulang SQL atau bukan kandidat).
10. Super Admin tidak pernah terhapus oleh job.
11. Audit: setelah job, `GET /api/audit` tidak memuat entri baru dari job (job tanpa audit); log hanya berisi jumlah.

## 8. Request/Response body

Tidak ada endpoint HTTP, sehingga tidak ada body REST dan tidak ada perubahan OpenAPI. Kontrak yang disentuh adalah bus `project.management.count-memberships` (didefinisikan foundation, disediakan `management-countmemberships`), dikonsumsi bukan disediakan. Bentuk payload dan perilaku error:

Request (nilai, bukan pointer; maks 500 id, id bukan uuid diabaikan provider):
```json
{ "UserIDs": ["7c1f3c2e-5b7a-4d7e-9a51-1f0b8a3e6d21", "0a9d2b64-1c3e-4f57-8d20-3b6e9c7f1a44", "d41e8f90-62b7-4a1c-9e35-5f0a7b2c8d13"] }
```

Response (user tanpa keanggotaan absen dan dibaca 0):
```json
{ "Counts": { "7c1f3c2e-5b7a-4d7e-9a51-1f0b8a3e6d21": 2 } }
```
Hasil: akun `0a9d...` dan `d41e...` dihapus; `7c1f...` dipertahankan.

Error (semuanya diperlakukan sama, fail-closed, tidak ada DELETE):

| Kondisi | Perilaku `Run` |
|---|---|
| `bus.ErrNotRegistered` (management belum terpasang) | return `purgeinvitations: count-memberships: bus: endpoint not registered: project.management.count-memberships` |
| `*errs.Error` dari handler (mis. `INVALID_ARGUMENT`) | return terbungkus `%w`, tanpa DELETE |
| error DB/handler lain | return terbungkus `%w`, tanpa DELETE |

Log sukses (slog, tanpa email dan tanpa id), contoh:
```
level=INFO msg="auth purge invitations selesai" candidates=12 retained=2 deleted=10 batches=1 duration_ms=34
```
Log gagal (fail-closed):
```
level=ERROR msg="auth purge invitations dibatalkan: count-memberships gagal" deleted=0 err="bus: endpoint not registered: project.management.count-memberships"
```
