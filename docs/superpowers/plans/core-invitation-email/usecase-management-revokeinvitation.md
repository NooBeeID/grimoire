# T12 Usecase Plan: management-revokeinvitation

Konteks bersama (config, mailer, migrasi `user_invitations`, kontrak bus, aksi audit, `base/` auth) ada di `overview.md` dan dikerjakan di foundation. Dokumen ini hanya memuat usecase `revokeinvitation`. Bila bertentangan dengan `overview.md`, overview menang. Tiket: `docs/tickets/T12-core-invitation-email.md` (K13, kriteria 5 dan 8 bagian pembatalan). Pola acuan: `docs/superpowers/plans/project-management/usecase-removemember.md` dan kode `management/removemember/`.

## 1. Metadata

| Atribut | Nilai |
|---|---|
| Ticket | T12 |
| Feature | `management-revokeinvitation` |
| Scope | backend (tombol "Batalkan" + dialog konfirmasi di section "Belum menerima undangan" dijelaskan di `frontend-plan.md`) |
| Domain | project |
| Module | management |
| Resource root | `backend/internal/domain/project/internal/management/revokeinvitation` |
| allowedPaths | folder `revokeinvitation/`, baris `revokeinvitation.Init(d)` di `.../management/base.go`, `api/modules/project/management.yml`, `api/openapi.yml` |

| Endpoint | Method | Version | Akses |
|---|---|---|---|
| `/api/projects/{projectID}/invitations/{userID}` | DELETE | tanpa prefix versi (prefix `/api`; naikkan `info.version` minor di `management.yml` bila belum dinaikkan plan T12 lain) | `RequireAuth`; owner project dan super_admin (`PermManageMembers`). Member 403. Non-anggota, project tidak ada, atau `projectID` bukan uuid: 404 |

Header wajib: `X-Requested-With: grimoire` (CSRF T00), sesi login. Tanpa request body. Sukses: 204 tanpa body.

Kontrak bus yang dipakai (tidak didefinisikan ulang di sini):

| Endpoint | Mode | Provider | Dipakai untuk |
|---|---|---|---|
| `core.auth.get-user` (`constants.BUS_CORE_AUTH_GET_USER`, `getuser.Request{UserID}` -> `Response{Found, Email, ..., Invitation *Invitation}`; `Invitation` aditif dari foundation 4.7) | Call, FAIL-CLOSED (error -> 500) | core/auth `getuserbus` | menentukan undangan masih menunggu (`Invitation != nil`) dan label audit (`Email`). Dipanggil SEBELUM tx |
| `core.audit.record` (`record.ActionProjectInvitationRevoked`, foundation 4.8) | Call, fail-open | core/audit | audit, via `base/audit`, SETELAH commit |

Tidak ada endpoint bus baru. Tidak ada panggilan bus di dalam tx (overview 0 poin 3). Tidak ada migrasi.

## 2. Summary Plan

`revokeinvitation` membatalkan undangan yang belum diterima dengan menghapus keanggotaan project dari akun undangan itu. Akun TIDAK dihapus (K13); akun kedaluwarsa tanpa keanggotaan dibersihkan job `purgeinvitations` (usecase lain). Satu usecase, tidak dipecah.

Alur `Execute` (orkestrasi; rule di `base`):

1. Aktor dari `reqctx`; tidak ada -> 401.
2. `FindAccess(projectID, actor.ID)`; `!found` -> `ErrProjectNotFound` (404). `level := ResolveLevel(actor.Role, memberRole, true)`; `Require(level, PermManageMembers)` (none -> 404, member -> 403).
3. Normalkan `userID` dengan `uuid.Parse(...).String()`; gagal -> `ErrMemberNotFound` (404), setelah langkah 2 supaya non-anggota tetap 404 project.
4. `bus get-user{UserID}` (sebelum tx). Error -> 500 (`fmt.Errorf("revokeinvitation: get-user: %w", err)`). `!Found` atau `Invitation == nil` -> `ErrInvitationNotPending` (409 `CONFLICT`, reason `INVITATION_NOT_PENDING`; pesan mengarahkan ke hapus anggota T03). Undangan kedaluwarsa tapi belum diterima tetap `Invitation != nil` dan BOLEH dibatalkan.
5. Satu tx: `LockProject` (hilang -> 404) -> `LockMembers` -> hitung ulang level aktor dari baris terkunci lalu `Require(lvl, PermManageMembers)` lagi -> target harus anggota (`FindMember`), kalau tidak `ErrMemberNotFound` (404) -> `base.CheckRemove(actor.ID, lvl, target, OwnerCount(members))` (owner terakhir -> 409 `LAST_OWNER`) -> `DELETE project_members`.
6. Setelah commit: audit `project.invitation.revoked`, lalu 204.

Keputusan khusus:

- **Urutan 409 vs 404.** Undangan tidak menunggu dicek sebelum keanggotaan target. Akun yang bukan anggota project ini tetapi punya undangan menunggu di project lain lolos langkah 4 lalu 404 `anggota tidak ditemukan` di tx. Akun yang sudah menerima (atau akun biasa, atau id tak dikenal) selalu 409 `INVITATION_NOT_PENDING`, termasuk bila ia bukan anggota; owner sudah anggota project (lolos langkah 2) sehingga tidak ada kebocoran.
- **Kenapa get-user sebelum tx.** Bus dilarang di dalam tx. Konsekuensi: ada jendela kecil antara get-user dan tx di mana penerima bisa menerima undangan (ganti password). Hasilnya keanggotaan yang sama tetap dihapus seperti `removemember` oleh owner, dan audit tetap `project.invitation.revoked`. Diterima dan didokumentasikan (bagian 9); tidak ada celah keamanan karena aktor tetap wajib `PermManageMembers`.
- **Label audit** (`user_label`) = `Email` dari respons get-user langkah 4 (tidak ada panggilan kedua).
- **`role`** di audit = role anggota terkunci (`target.Role`), bukan dari request.
- **Fail-closed berbeda dari `removemember`**: di sini hasil get-user menentukan 409, jadi tidak bisa fail-soft. `bus.ErrNotRegistered` -> 500.
- **Revoke tidak menyentuh `user_invitations`/akun** dan tidak mencabut sesi. Pemilik akun yang masih menunggu tetap bisa login, tetapi tidak punya project; bila tidak pernah login, purge membersihkan setelah `INVITATION_PURGE_AFTER`.
- **Owner terakhir.** Undangan bisa berrole `owner`. `CheckRemove` menjaga: pembatalan satu-satunya owner oleh owner -> 409 `LAST_OWNER`; Super Admin (level admin) boleh. Karena owner yang mengundang sudah ada, kasus ini hanya muncul untuk project yang ownernya hanya penerima undangan itu (diundang Super Admin).
- **Self.** Aktor tidak bisa menjadi target yang undangannya menunggu (sesi terkunci gate ganti password), jadi tidak ada flag `self` di audit.
- **Idempotensi.** Panggilan kedua setelah sukses: target bukan anggota lagi. Bila akun masih punya undangan menunggu (di project lain) -> 404; bila tidak -> 409 setelah get-user. Keduanya sah, tidak ada audit.

Prasyarat (bila belum ada, hentikan dan selesaikan dulu; jangan membuatnya dari folder `revokeinvitation/`):

- Foundation: `ActionProjectInvitationRevoked` (4.8), `getuser.Response.Invitation` dan `getuserbus` yang mengisinya (`auth-invitation-markers`, gelombang A).
- `management/base/errors.go`: `ReasonInvitationNotPending`, `ErrInvitationNotPending` (ditulis plan `management-listinvitations`, gelombang B; lihat overview 4.9).
- Dari T03 (sudah ada): `base.Actor/ActorFrom`, `RequestMeta`, `Member`, `FindMember`, `OwnerCount`, `ResolveLevel`, `Require`, `PermManageMembers`, `CheckRemove`, `ErrProjectNotFound`, `ErrMemberNotFound`, `base/store` (`FindAccess`, `LockProject`, `LockMembers`, `WithTx`), `base/audit`.
- Usecase ini TIDAK bergantung pada `base/users` method baru (`GetByEmail`/`Invite`/`ResendInvitation`) dari `management-inviteuser`. Ia memakai interface sempit sisi consumer (rule 5.6) yang dipenuhi `users.BusLookup.Get` yang sudah ada, jadi bisa dikerjakan paralel.

## 3. Plan (langkah berurutan)

Semua file di `backend/internal/domain/project/internal/management/revokeinvitation/`. Urutan TDD: tes dulu per langkah.

### Langkah 1. Verifikasi prasyarat dan OpenAPI dulu

1. `cd backend && go build ./...` hijau; `grep -n "ErrInvitationNotPending" internal/domain/project/internal/management/base/errors.go` ada; `grep -n "ActionProjectInvitationRevoked" lib/contracts/core/audit/record/actions.go` ada; `getuser.Response` punya `Invitation`.
2. Tambah operasi di `api/modules/project/management.yml` (rule 11.6, spec sebelum handler): `DELETE /api/projects/{projectID}/invitations/{userID}`, `operationId: revokeinvitation`, tag sama dengan `removemember`, parameter `XRequestedWith`, `ProjectID`, `UserID` (komponen yang sama dengan `removemember`), respons `204`, `401`, `403`, `404`, `409`, `500` (schema `Error` dari `foundation.yml`). Deskripsi 409 menyebut dua reason: `INVITATION_NOT_PENDING` dan `LAST_OWNER`. Tambah path ke `api/root.yml` bila path `/projects/{projectID}/invitations/{userID}` belum terdaftar. Lalu `make api-bundle`, commit `api/openapi.yml`, `make api-check` hijau.

### Langkah 2. `usecase.go`

```go
// Sisi consumer (rule 5.6). Repo konkret memenuhi keduanya.
type Repository interface {
    FindAccess(ctx context.Context, projectID, userID string) (p base.Project, memberRole string, found bool, err error)
    InTx(ctx context.Context, fn func(TxRepository) error) error
}
type TxRepository interface {
    LockProject(ctx context.Context, id string) (base.Project, bool, error)
    LockMembers(ctx context.Context, projectID string) ([]base.Member, error)
    DeleteMember(ctx context.Context, projectID, userID string) error
}

// userGetter dipenuhi users.BusLookup. Sempit sengaja: hanya Get.
type userGetter interface {
    Get(ctx context.Context, userID string) (getuser.Response, error)
}

type Input struct {
    ProjectID string
    UserID    string
    Meta      base.RequestMeta
}

type Usecase struct {
    repo  Repository
    users userGetter
    audit audit.Recorder
    log   *slog.Logger
}
func NewUsecase(repo Repository, u userGetter, a audit.Recorder, log *slog.Logger) *Usecase
func (u *Usecase) Execute(ctx context.Context, in Input) error
```

Alur `Execute`:

1. `actor, ok := base.ActorFrom(reqctx.IdentityFrom(ctx))`; `!ok` -> `errs.New(errs.CodeUnauthorized, "perlu login")`.
2. `p0, memberRole, found, err := u.repo.FindAccess(...)`; err -> `fmt.Errorf("revokeinvitation: cari akses: %w", err)`; `!found` -> `base.ErrProjectNotFound`. `level := base.ResolveLevel(actor.Role, memberRole, true)`; `if err := base.Require(level, base.PermManageMembers); err != nil { return err }`.
3. `id, err := uuid.Parse(in.UserID)`; err -> `base.ErrMemberNotFound`. `target := id.String()`.
4. ```go
   res, err := u.users.Get(ctx, target)
   if err != nil { return fmt.Errorf("revokeinvitation: get-user: %w", err) }
   if !res.Found || res.Invitation == nil { return base.ErrInvitationNotPending }
   label := res.Email
   ```
5. Tx (sama dengan `removemember` kecuali permission):
   ```go
   var removed base.Member
   var project base.Project
   err = u.repo.InTx(ctx, func(tx TxRepository) error {
       p, ok, err := tx.LockProject(ctx, in.ProjectID)
       if err != nil { return fmt.Errorf("revokeinvitation: kunci project: %w", err) }
       if !ok { return base.ErrProjectNotFound }
       members, err := tx.LockMembers(ctx, p.ID)
       if err != nil { return fmt.Errorf("revokeinvitation: kunci anggota: %w", err) }
       myRole := ""
       if me, ok := base.FindMember(members, actor.ID); ok { myRole = me.Role }
       lvl := base.ResolveLevel(actor.Role, myRole, true)
       if err := base.Require(lvl, base.PermManageMembers); err != nil { return err } // diturunkan/dikeluarkan di antara pre-check dan lock
       t, ok := base.FindMember(members, target)
       if !ok { return base.ErrMemberNotFound }
       if err := base.CheckRemove(actor.ID, lvl, t, base.OwnerCount(members)); err != nil { return err }
       if err := tx.DeleteMember(ctx, p.ID, t.UserID); err != nil {
           return fmt.Errorf("revokeinvitation: hapus anggota: %w", err)
       }
       project, removed = p, t
       return nil
   })
   if err != nil { return err }
   ```
   `*errs.Error` dikembalikan apa adanya (tanpa wrap) supaya `respond.WriteError` memetakan 403/404/409. Variabel `p0` dari langkah 2 tidak dipakai lagi (`_`).
6. Audit setelah commit:
   ```go
   u.audit.Record(ctx, audit.Event{
       Action: record.ActionProjectInvitationRevoked, ProjectID: project.ID, ProjectLabel: project.Name,
       TargetType: "project_member", Target: removed.UserID,
       Args: map[string]any{"user_id": removed.UserID, "user_label": label, "role": removed.Role},
       Meta: in.Meta,
   })
   return nil
   ```
   Args tidak memuat password, hash, atau kredensial apa pun (tidak ada yang dimiliki usecase ini).

Usecase tidak menulis SQL, tidak memanggil `bus` langsung, rule hanya dari `base`.

### Langkah 3. `repository.go`

Salinan pola `removemember/repository.go` (repository satu-satunya yang menyentuh SQL):

```go
const deleteMemberSQL = `DELETE FROM project_members WHERE project_id = $1 AND user_id = $2`

type Repo struct{ db *db.DB; s *store.Store }
func newRepo(d *db.DB, s *store.Store) *Repo
func (r *Repo) FindAccess(...)                  // delegasi r.s.FindAccess
func (r *Repo) InTx(ctx, fn)                    // r.db.BeginTx -> &txRepo{s: r.s.WithTx(tx), q: tx}
func (t *txRepo) LockProject(...)               // delegasi store
func (t *txRepo) LockMembers(...)               // delegasi store
func (t *txRepo) DeleteMember(ctx, projectID, userID) error // 0 baris -> base.ErrMemberNotFound (jaring pengaman)
```

Catatan duplikasi: `deleteMemberSQL` sama dengan milik `removemember`. Dua feature memakai SQL identik; dibiarkan terpisah agar plan ini tidak menyentuh feature T03. Bila reviewer menginginkan, promosikan ke `base/store.DeleteMember` dalam PR terpisah (rule 5.3); bukan prasyarat T12.

### Langkah 4. `handler.go`

```go
type executor interface{ Execute(ctx context.Context, in Input) error }
type handler struct{ uc executor }
```
`ServeHTTP`: `Input{ProjectID: chi.URLParam(r,"projectID"), UserID: chi.URLParam(r,"userID"), Meta: base.RequestMeta{IP: middleware.KeyByIP(r), UserAgent: truncate(r.UserAgent(), 512)}}`. Error -> `respond.WriteError(w, r, err)`; sukses -> `w.WriteHeader(http.StatusNoContent)`. Tanpa validasi uuid di handler. Fungsi `truncate` disalin (tidak boleh import antar feature).

### Langkah 5. `init.go`

```go
func Init(d app.Deps) {
    s := store.New(d.DB)
    uc := NewUsecase(newRepo(d.DB, s), users.New(d.Bus), audit.New(d.Bus, d.Log), d.Log)
    d.API.With(middleware.RequireAuth()).Delete("/projects/{projectID}/invitations/{userID}", handler{uc: uc}.ServeHTTP)
}
```
Param `{projectID}` dan `{userID}` (aturan README untuk `/projects`). Route ini hidup berdampingan dengan `POST .../invitations/{userID}/resend` dan `GET/POST .../invitations` milik plan lain; chi mengizinkan method berbeda pada pohon yang sama.

### Langkah 6. Wiring

`project/internal/management/base.go`: tambah `revokeinvitation.Init(d)` di `Register`, berdampingan dengan Init feature lain (urutan bebas, tanpa dependensi antar-Init).

### Langkah 7. Tes dan verifikasi

`cd backend && go build ./... && go vet ./... && go test ./internal/domain/project/internal/management/revokeinvitation/...`; dengan `TEST_DATABASE_URL`: `go test -tags integration ./internal/domain/project/internal/management/revokeinvitation/...`; `make api-check`; archtest `TestT00_AC4_RepoIsClean` hijau (tanpa import antar-feature, SQL hanya di `repository.go`).

## 4. Migration

Tidak ada. Tidak ada tabel atau kolom baru. `DELETE` hanya menyentuh `project_members` (T03). Tabel `user_invitations` (foundation) tidak disentuh oleh usecase ini.

## 5. Changes (per file/folder)

| File / folder | Aksi | Isi |
|---|---|---|
| `.../management/revokeinvitation/usecase.go` | buat | `Repository`, `TxRepository`, `userGetter`, `Input`, `Usecase`, `Execute` |
| `.../revokeinvitation/repository.go` | buat | `Repo`, `txRepo`, `deleteMemberSQL` |
| `.../revokeinvitation/handler.go` | buat | handler DELETE, 204 |
| `.../revokeinvitation/init.go` | buat | route + wiring |
| `.../revokeinvitation/usecase_test.go` | buat | unit, fake repo/users/audit |
| `.../revokeinvitation/handler_test.go` | buat | tes handler |
| `.../revokeinvitation/repository_test.go` | buat | integrasi repo (tag `integration`, `db.TestDB`) |
| `.../revokeinvitation/revokeinvitation_integration_test.go` | buat | integrasi HTTP penuh (tag `integration`), termasuk tes kriteria 5 dan 8 |
| `.../management/base.go` | ubah | `revokeinvitation.Init(d)` |
| `api/modules/project/management.yml`, `api/root.yml` (bila perlu), `api/openapi.yml` | ubah | operasi `revokeinvitation` + bundle |
| `docs/tickets/README.md` | TIDAK diubah di plan ini | katalog audit dan status dikerjakan foundation/penutup |

Tidak mengubah feature lain, `base/` management, kontrak bus, maupun module auth.

## 6. Test Scenario

### 6.1 Unit usecase (`usecase_test.go`; fake `Repository`/`TxRepository`, fake `userGetter`, fake `audit.Recorder`)

Fake users dapat mengembalikan `Response`/error per id dan mencatat jumlah panggilan; fake tx mencatat urutan panggilan.

| # | Skenario | Harapan |
|---|---|---|
| 1 | Owner membatalkan undangan `member` pending | nil; `DeleteMember(projectID, target)` dipanggil sekali; tepat satu audit `project.invitation.revoked` dengan Args `{user_id, user_label=email, role="member"}`, `TargetType="project_member"`, `Target=target`, `ProjectLabel=nama` |
| 2 | Super Admin bukan anggota membatalkan | nil; audit tercatat |
| 3 | Undangan kedaluwarsa tapi belum diterima (`Invitation != nil`, `Expired=true`) | nil (boleh dibatalkan) |
| 4 | Tanpa identitas di ctx | 401 `UNAUTHORIZED`; repo tidak dipanggil |
| 5 | `FindAccess` not found (non-anggota) | `ErrProjectNotFound`; users.Get tidak dipanggil |
| 6 | Aktor `member` | `ErrForbidden`; users.Get tidak dipanggil, tidak ada tx |
| 7 | `FindAccess` error DB | error ter-wrap, bukan `*errs.Error` |
| 8 | `userID` bukan uuid | `ErrMemberNotFound`; users.Get tidak dipanggil; non-anggota dengan uuid buruk tetap `ErrProjectNotFound` |
| 9 | uuid huruf besar | dinormalkan lowercase saat memanggil get-user dan tx |
| 10 | get-user `Found=false` | `ErrInvitationNotPending` (409 reason `INVITATION_NOT_PENDING`); tidak ada tx, tidak ada audit |
| 11 | get-user `Found=true`, `Invitation=nil` (sudah diterima) | `ErrInvitationNotPending`; tidak ada tx/audit |
| 12 | get-user error (termasuk `bus.ErrNotRegistered`) | error ter-wrap `%w` (`errors.Is(err, bus.ErrNotRegistered)` benar); bukan `*errs.Error`; tidak ada tx/audit |
| 13 | Undangan pending, target bukan anggota project ini | `ErrMemberNotFound` (404); tidak ada audit |
| 14 | `LockProject` not found (project hilang di antara pre-check dan lock) | `ErrProjectNotFound` |
| 15 | Aktor owner diturunkan jadi member sebelum lock (daftar terkunci berisi aktor sebagai member) | `ErrForbidden`; tidak ada delete |
| 16 | Aktor dikeluarkan sebelum lock | `ErrProjectNotFound` |
| 17 | Target owner terakhir, aktor owner | `ErrLastOwner` (409 `LAST_OWNER`); tidak ada delete/audit |
| 18 | Target owner terakhir, aktor Super Admin | nil; audit `role="owner"` |
| 19 | Target owner, ada owner lain | nil |
| 20 | `DeleteMember` mengembalikan `ErrMemberNotFound` | dikembalikan apa adanya |
| 21 | `DeleteMember` error DB | ter-wrap; tidak ada audit |
| 22 | Urutan: `users.Get` dipanggil sebelum `InTx`; `audit.Record` setelah `InTx` selesai dan tidak di dalam closure | diverifikasi lewat pencatat urutan di fake |
| 23 | Args audit tidak memuat kunci `password`, `temporary_password`, `hash`, `credentials` | cek set kunci tepat tiga: `user_id`, `user_label`, `role` |
| 24 | Jalur gagal mana pun | 0 audit |

### 6.2 Handler (`handler_test.go`)

| # | Skenario | Harapan |
|---|---|---|
| 1 | Sukses | 204, body kosong; `Input.ProjectID`/`UserID` diambil dari chi param; `Meta.IP` terisi |
| 2 | Executor mengembalikan `ErrInvitationNotPending` | 409, `error.code="CONFLICT"`, `details.reason="INVITATION_NOT_PENDING"` |
| 3 | `ErrForbidden`, `ErrProjectNotFound`, `ErrMemberNotFound`, `ErrLastOwner` | 403, 404, 404, 409 (reason `LAST_OWNER`) |
| 4 | error biasa | 500, tanpa bocor pesan internal |
| 5 | User-Agent 600 rune | terpotong 512 rune |

### 6.3 Integrasi repo (`repository_test.go`, `-tags integration`, `db.TestDB`)

1. `DeleteMember` menghapus tepat satu baris target; baris anggota lain tidak berubah.
2. `DeleteMember` target yang tidak ada -> `ErrMemberNotFound`.
3. `FindAccess`/`LockProject`/`LockMembers` melalui `InTx` konsisten dengan store (owner/member/none).
4. Rollback: error di closure setelah `DeleteMember` -> baris tetap ada.
5. Dua `InTx` bersamaan pada project yang sama diserialkan oleh `LockProject` (tx kedua menunggu).

### 6.4 Integrasi HTTP (`revokeinvitation_integration_test.go`, `-tags integration`)

Server uji: router asli dengan `RequireAuth`, `db.TestDB`, bus berisi handler `core.auth.get-user` palsu (peta `userID -> getuser.Response`, `Invitation` terisi untuk yang menunggu) dan perekam audit. Pola `itSetup` dari `removemember_integration_test.go`.

| # | Skenario | Harapan |
|---|---|---|
| 1 | `TestT12_AC8_OwnerRevokesPending` owner membatalkan anggota undangan | 204; baris `project_members` hilang; satu audit `project.invitation.revoked`; anggota lain utuh |
| 2 | `TestT12_AC5_MemberForbidden` member project | 403; baris tetap; get-user tidak dipanggil |
| 3 | `TestT12_AC5_NonMemberNotFound` developer bukan anggota | 404; baris tetap |
| 4 | Super Admin bukan anggota | 204 |
| 5 | Tanpa sesi | 401 |
| 6 | Undangan sudah diterima (`Invitation=nil`) | 409 `INVITATION_NOT_PENDING`; baris `project_members` tetap |
| 7 | Target bukan anggota tetapi punya undangan menunggu | 404 |
| 8 | `projectID` bukan uuid / project tak ada | 404 |
| 9 | `userID` bukan uuid | 404 |
| 10 | Owner terakhir (undangan role owner) dibatalkan owner lain yang tersisa? Project dengan satu owner = penerima undangan, aktor owner lain mustahil; gunakan aktor Super Admin | Super Admin 204; aktor owner dengan `ownerCount=1` yang menjadi target dirinya tidak relevan. Tambahan: project dengan dua owner (A, undangan B) A membatalkan B -> 204 |
| 11 | Owner terakhir = target dan aktor owner tak mungkin; uji `LAST_OWNER` lewat ordering: project hanya punya satu owner (undangan) dan aktor Super Admin ada di kasus 4; kasus `LAST_OWNER` murni diuji di unit 17 | tercatat sebagai batasan tes, bukan celah |
| 12 | Panggilan DELETE ulang setelah sukses | 409 (akun tanpa undangan lain) atau 404 (masih ada undangan di project lain); tidak ada audit baru |
| 13 | Akun tidak dihapus | tabel `users`/`user_invitations` (bila ada di DB uji) tidak berubah oleh revoke (K13) |
| 14 | Dua pembatalan paralel untuk target yang sama | satu 204, satu 404; satu audit |
| 15 | Header `X-Requested-With` tidak ada | ditolak middleware CSRF (403) sesuai T00 |

Kasus 11 sengaja dicatat sebagai pengarah: jangan menulis tes HTTP yang tidak bisa dibangun; `LAST_OWNER` dicakup tes unit 17.

## 7. E2E Scenarios

Memakai server uji penuh (`db.TestDB`, auth asli, `mailertest.Fake`) pada lapisan HTTP, atau Playwright terhadap binary bila UI sudah ada (`frontend-plan.md`). Bagian ini hanya bagian pembatalan dari kriteria 5 dan 8.

Positif:

1. **Kriteria 8 (pembatalan).** Owner mengundang `baru@contoh.id` (`POST /api/projects/{id}/invitations`), `GET .../invitations` memuat item itu, `DELETE .../invitations/{userID}` -> 204. `GET /api/projects/{id}/members` tidak lagi memuat user itu; `GET .../invitations` kosong; audit `project.invitation.revoked` tercatat dengan `user_label=baru@contoh.id` dan role; akun masih ada (`GET /api/users/{id}` oleh super_admin menampilkan `invitation.pending=true`); percobaan login dengan password sementara masih berhasil (akun tidak dihapus, K13) tetapi `getme` tidak memuat project itu.
2. **Pembatalan undangan kedaluwarsa.** `expires_at` dimundurkan, owner membatalkan -> 204.
3. **Undang ulang setelah dibatalkan.** Setelah revoke, owner mengundang email yang sama lagi -> 201 (`created=false`, akun menunggu dipakai ulang sesuai K6); keanggotaan tercatat kembali.
4. **Super Admin membatalkan** undangan di project yang bukan miliknya -> 204.
5. **UI** (bila e2e browser): klik "Batalkan" pada section "Belum menerima undangan", dialog konfirmasi muncul, konfirmasi -> baris hilang tanpa reload, toast sukses; membatalkan dua kali dari dua tab -> tab kedua menampilkan pesan "undangan sudah tidak ada/diterima" dan daftar disegarkan.

Negatif:

1. **Kriteria 5.** Member (bukan owner) `DELETE` -> 403, keanggotaan target tetap. Non-anggota -> 404. Tanpa sesi -> 401.
2. **Kriteria 8 (batas).** Undangan sudah diterima (penerima sudah ganti password) -> 409 `CONFLICT` reason `INVITATION_NOT_PENDING`; keanggotaan tetap; pesan menunjuk ke hapus anggota. Owner lalu memakai `DELETE /api/projects/{id}/members/{userID}` (T03) -> 204.
3. Target bukan anggota -> 404.
4. Id bukan uuid -> 404.
5. Mesin bus auth tidak terdaftar (simulasi) -> 500, keanggotaan tetap, tidak ada audit.
6. Setelah revoke, endpoint resend untuk user yang sama di project itu -> 404 (bukan anggota); tidak ada kredensial yang bisa diambil lewat respons revoke (204 tanpa body).

## 8. Request/Response body

### 8.1 `DELETE /api/projects/{projectID}/invitations/{userID}`

Request:

```
DELETE /api/projects/0b8e5a64-2d3f-4c61-9d2c-6f1e7a9b3c10/invitations/7c1f3c2e-5b7a-4d7e-9a51-1f0b8a3e6d21
X-Requested-With: grimoire
Cookie: <sesi>
```
Tanpa body.

204 (tanpa body, tanpa `Content-Type`):

```
HTTP/1.1 204 No Content
```

### 8.2 Error (envelope `Error` dari `foundation.yml`; `trace_id` ditambahkan `httpx.WriteError` bila ada)

401:
```json
{ "error": { "code": "UNAUTHORIZED", "message": "perlu login" } }
```

403 (member bukan owner):
```json
{ "error": { "code": "FORBIDDEN", "message": "akses ditolak" } }
```

404 (non-anggota, project tak ada, `projectID` bukan uuid):
```json
{ "error": { "code": "NOT_FOUND", "message": "project tidak ditemukan" } }
```

404 (target bukan anggota, `userID` bukan uuid):
```json
{ "error": { "code": "NOT_FOUND", "message": "anggota tidak ditemukan" } }
```

409 (undangan sudah diterima / bukan undangan):
```json
{ "error": { "code": "CONFLICT", "message": "undangan sudah diterima atau tidak ada", "details": { "reason": "INVITATION_NOT_PENDING" } } }
```

409 (owner terakhir, aktor owner):
```json
{ "error": { "code": "CONFLICT", "message": "owner terakhir tidak bisa keluar atau diturunkan; angkat owner lain lebih dulu", "details": { "reason": "LAST_OWNER" } } }
```

500:
```json
{ "error": { "code": "INTERNAL", "message": "terjadi kesalahan internal" } }
```
(kode dan pesan 500 mengikuti `respond.WriteError` yang ada; isi di atas menunjukkan bentuk, bukan menambah field.)

### 8.3 Bus (dikonsumsi, bukan disediakan)

`core.auth.get-user` request dan respons yang dibaca usecase ini (potongan; kontrak penuh di foundation 4.7):

```json
// Request
{ "UserID": "7c1f3c2e-5b7a-4d7e-9a51-1f0b8a3e6d21" }
// Response (undangan menunggu)
{ "Found": true, "ID": "7c1f3c2e-5b7a-4d7e-9a51-1f0b8a3e6d21", "Email": "baru@contoh.id",
  "Invitation": { "InvitedAt": "2026-10-21T08:15:00Z", "IssuedAt": "2026-10-21T08:15:00Z",
                  "ExpiresAt": "2026-10-28T08:15:00Z", "SentAt": null, "Expired": false,
                  "DeliveryStatus": "failed", "ResendCount": 1 } }
// Response (sudah diterima atau akun biasa): "Invitation": null
```

### 8.4 Audit yang dikirim (`record.Request`, lewat `base/audit`)

```json
{
  "action": "project.invitation.revoked",
  "project_id": "0b8e5a64-2d3f-4c61-9d2c-6f1e7a9b3c10",
  "project_label": "Knowledge Hub",
  "target_type": "project_member",
  "target": "7c1f3c2e-5b7a-4d7e-9a51-1f0b8a3e6d21",
  "args": { "user_id": "7c1f3c2e-5b7a-4d7e-9a51-1f0b8a3e6d21", "user_label": "baru@contoh.id", "role": "member" }
}
```
Aktor, IP, dan user agent diisi `base/audit` dari `reqctx` dan `RequestMeta`. Tidak ada password atau kredensial.

### 8.5 Skema OpenAPI operasi (`api/modules/project/management.yml`)

```yaml
/projects/{projectID}/invitations/{userID}:
  delete:
    operationId: revokeinvitation
    summary: Batalkan undangan yang belum diterima (menghapus keanggotaan; akun tidak dihapus)
    parameters:
      - $ref: '#/components/parameters/XRequestedWith'
      - $ref: '#/components/parameters/ProjectID'
      - $ref: '#/components/parameters/UserID'
    responses:
      '204': { description: Undangan dibatalkan }
      '401': { $ref: '#/components/responses/Unauthorized' }
      '403': { $ref: '#/components/responses/Forbidden' }
      '404': { $ref: '#/components/responses/NotFound' }
      '409': { description: "CONFLICT; details.reason INVITATION_NOT_PENDING atau LAST_OWNER", content: { application/json: { schema: { $ref: 'foundation.yml#/components/schemas/Error' } } } }
      '500': { $ref: '#/components/responses/Internal' }
```
Nama komponen `parameters`/`responses` disamakan dengan yang dipakai operasi `removemember` di file yang sama; sesuaikan bila berbeda.

## 9. Catatan lintas plan dan risiko

- **Jendela race get-user vs tx.** Penerima bisa menerima undangan di antara langkah 4 dan tx; keanggotaan tetap terhapus (setara `removemember` oleh owner) dan tercatat revoked. Peluang kecil, dampak sama dengan aksi owner yang sah. Diterima.
- **Ketergantungan urutan.** Butuh `ErrInvitationNotPending` dari `management-listinvitations` dan `getuser.Invitation` dari `auth-invitation-markers`. Bila dikerjakan paralel, definisikan keduanya di foundation commit terpisah, bukan di folder `revokeinvitation/`.
- **SQL identik dengan `removemember`.** Sengaja tidak dipromosikan ke store agar tidak menyentuh T03 (lihat langkah 3).
- **Akun menunggu tanpa keanggotaan.** Dibersihkan `auth-purgeinvitations` setelah kedaluwarsa + `INVITATION_PURGE_AFTER` dan `project.management.count-memberships == 0`. Revoke sengaja tidak memanggil auth untuk menghapus akun (K13, bus dilarang dalam tx, tidak ada endpoint bus hapus).
- **UI.** Dialog konfirmasi wajib menjelaskan: "Akun tetap ada sampai dibersihkan otomatis; Super Admin bisa menonaktifkannya dari halaman Users." Tombol hanya untuk owner/super_admin; ditangani `frontend-plan.md`.
