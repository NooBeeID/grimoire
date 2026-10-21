# T12 usecase `management-listinvitations` - Daftar undangan belum diterima di project

Foundation: `docs/superpowers/plans/core-invitation-email/overview.md` (menang bila bertentangan). Tiket: `docs/tickets/T12-core-invitation-email.md`.
Gelombang B (overview bagian 9). Prasyarat: foundation 4.1 sampai 4.10 (khususnya kontrak `getuser` aditif `Response.Invitation` dan `finddevelopers` aditif `Developer.InvitationPending`) dan, untuk tes integrasi lintas module, penyedia bus dari `auth-invitation-markers` (gelombang A). Unit test memakai fake `users.Lookup`, jadi tidak menunggu provider.

---

## 1. Metadata

| Atribut | Nilai |
|---|---|
| Ticket | T12 |
| Usecase | `management-listinvitations` (satu usecase; perubahan base hanyalah penanda aditif di DTO, tidak dipecah ke plan lain) |
| Scope | backend (respons dikonsumsi `frontend-plan.md`) |
| Domain / Module | project / management (`backend/internal/domain/project/internal/management`) |
| Folder feature | `backend/internal/domain/project/internal/management/listinvitations` |
| Migrasi | tidak ada |

| Endpoint | Method | Version | Auth | Perubahan |
|---|---|---|---|---|
| `/api/projects/{projectID}/invitations` | GET | - (tanpa prefix versi) | `RequireAuth`; izin `PermManageMembers` (owner, super_admin). Member 403, non-anggota atau project tidak ada 404 | baru |
| `/api/projects/{projectID}/members` | GET | - | anggota | aditif: `invitation_pending`, `invitation_expires_at` di tiap item. `listmembers/usecase.go` TIDAK berubah |
| `/api/projects/{projectID}/member-candidates` | GET | - | owner/super_admin | aditif: `invitation_pending` di tiap item |

Bus: tidak ada endpoint bus baru. Usecase ini konsumen `core.auth.get-user` (kontrak aditif T12, tanpa `Request.Email` di sini) dan `core.auth.find-developers` (sudah dipakai `searchmembercandidates`).

Catatan koordinasi: path `/api/projects/{projectID}/invitations` dipakai bersama `management-inviteuser` (POST). Masing-masing Init memasang method-nya sendiri (`Get` di sini, `Post` di sana) pada pola yang sama; chi mengizinkan. Di `api/modules/project/management.yml` path item ini dibuat oleh plan ini (operasi `get`); `management-inviteuser` menambah `post` pada item yang sama.

---

## 2. Summary Plan

`listinvitations` menjawab "siapa yang diundang ke project ini tetapi belum menerima". Pola sama dengan `listmembers`:

1. Otorisasi: aktor dari `reqctx`, `FindAccess`, tidak ditemukan -> `ErrProjectNotFound`, `ResolveLevel` lalu `Require(level, PermManageMembers)` (member 403, none 404; urutan sama dengan `searchmembercandidates`).
2. `ListMembers(projectID)` lalu `users.Get` per anggota (get-user per anggota, sama dengan `listmembers`). Satu kegagalan get-user menggagalkan seluruh permintaan (data wajib lengkap, tidak ada hasil parsial).
3. Saring: hanya anggota yang `Invitation != nil` (undangan menunggu menurut provider auth; undangan yang diterima atau akun tanpa undangan menghasilkan nil). `Found=false` tidak mungkin punya undangan sehingga ikut tersaring.
4. Petakan ke `InvitationDTO`, urut `added_at` menurun (tie-break `user_id` naik, stabil).

Perubahan aditif di base management:

- `MemberDTO` + `invitation_pending` (bool) + `invitation_expires_at` (nullable), diisi `Member.ToDTO(u)` dari `u.Invitation`. Karena `listmembers` sudah memanggil `m.ToDTO(usr)`, UI Members mendapat penanda tanpa mengubah usecase itu.
- `CandidateDTO` + `invitation_pending`, diisi `searchmembercandidates` dari `Developer.InvitationPending`.
- `ErrInvitationNotPending` (409 `CONFLICT`, reason `INVITATION_NOT_PENDING`) di `base/errors.go`. Belum dipakai di usecase ini; ditulis di sini karena plan ini yang pertama dikerjakan di management (overview 4.9), dipakai `management-resendinvitation` dan `management-revokeinvitation`.

Keputusan:

- Tanpa query param (tanpa paging): jumlah anggota project sudah dibatasi wajar oleh T03 dan `listmembers` juga tidak mem-paging. N+1 get-user diterima (preseden `listmembers`); tidak ada endpoint bus batch yang boleh ditambah di tiket ini.
- `expired` dan `delivery_status` berasal dari provider (`getuser.Invitation.Expired/DeliveryStatus`). Management tidak menghitung ulang kedaluwarsa (waktu milik auth; satu sumber kebenaran). Karena itu usecase ini tidak butuh `Clock`.
- `email_failed` = `delivery_status in {failed, not_configured}`. Dihitung di entity `base`, bukan handler.
- `invited_by_label` memakai `Member.AddedByLabel` (dicatat saat anggota ditambahkan, `*string`; null bila baris lama tidak punya label). `added_at` memakai `Member.AddedAt`. Pengundang di `user_invitations` bisa berbeda dari penambah anggota hanya pada kasus langka (undang ulang dari Users); tampilan project mengikuti keanggotaan di project ini.
- `sent_at` null bila belum pernah terkirim berhasil.
- Respons `Cache-Control: no-store` (data per-user, pola `listmembers`). Tidak ada kredensial di respons ini.
- Tidak ada audit (bacaan).

---

## 3. Plan (langkah berurutan)

Tiap langkah diakhiri `go build ./...` dan tes paket terkait hijau. TDD: tulis tes dulu per langkah.

1. **Entity murni `base/invitation.go`** (baru, tanpa I/O):
   ```go
   // InvitationDTO adalah baris daftar undangan belum diterima.
   type InvitationDTO struct {
       UserID         string     `json:"user_id"`
       Email          string     `json:"email"`
       Name           string     `json:"name"`
       Role           string     `json:"role"`
       InvitedByLabel *string    `json:"invited_by_label"`
       AddedAt        time.Time  `json:"added_at"`
       SentAt         *time.Time `json:"sent_at"`
       ExpiresAt      time.Time  `json:"expires_at"`
       Expired        bool       `json:"expired"`
       DeliveryStatus string     `json:"delivery_status"`
       EmailFailed    bool       `json:"email_failed"`
       ResendCount    int        `json:"resend_count"`
   }

   // ToInvitationDTO: ok=false bila u.Found=false atau u.Invitation==nil (bukan undangan menunggu).
   func (m Member) ToInvitationDTO(u getuser.Response) (InvitationDTO, bool)

   // EmailFailed: delivery_status failed atau not_configured.
   func EmailFailed(deliveryStatus string) bool

   // SortInvitations: added_at menurun, tie-break user_id naik (stabil).
   func SortInvitations(items []InvitationDTO)
   ```
   - Waktu di-`UTC()`. `SentAt` disalin (pointer baru, UTC) agar DTO tidak berbagi memori dengan respons bus.
   - Konstanta status diambil dari `lib/contracts/core/auth/invitation` (`EmailStatus*` untuk nilai `sent|failed|not_configured|pending`; `delivery_status` memakai kata yang sama). Jangan menduplikasi string literal; bila konstanta provider bernama `DeliveryStatus*`, pakai itu (cek `getuser.Invitation` hasil foundation 4.7).
2. **Penanda di `MemberDTO` dan `CandidateDTO`** (`base/dto.go`, `base/member.go`):
   - `MemberDTO` tambah `InvitationPending bool \`json:"invitation_pending"\`` dan `InvitationExpiresAt *time.Time \`json:"invitation_expires_at"\``.
   - `Member.ToDTO(u)`: bila `u.Found && u.Invitation != nil` -> `InvitationPending=true`, `InvitationExpiresAt=&expires(UTC)`. Selain itu false/nil. Penanda tetap `true` walau sudah kedaluwarsa (UI memakai `invitation_expires_at` untuk label "kedaluwarsa"; tiket: tampil "belum menerima undangan").
   - `CandidateDTO` tambah `InvitationPending bool \`json:"invitation_pending"\``.
3. **`base/errors.go`**: tambah `ReasonInvitationNotPending = "INVITATION_NOT_PENDING"` dan
   `ErrInvitationNotPending = errs.WithDetails(errs.CodeConflict, "undangan sudah diterima atau tidak ada", map[string]any{"reason": ReasonInvitationNotPending})`.
4. **`searchmembercandidates/usecase.go`**: satu baris, `base.CandidateDTO{ID: d.ID, Name: d.Name, Email: d.Email, InvitationPending: d.InvitationPending}`. Tidak ada perubahan lain (field `Developer.InvitationPending` dari foundation 4.7).
5. **Feature `listinvitations/`** (ikuti struktur `listmembers`):
   - `usecase.go`
     ```go
     type AccessFinder interface {
         FindAccess(ctx context.Context, projectID, userID string) (p base.Project, memberRole string, found bool, err error)
     }
     type MemberLister interface {
         ListMembers(ctx context.Context, projectID string) ([]base.Member, error)
     }
     type Usecase struct { access AccessFinder; members MemberLister; users users.Lookup }
     func NewUsecase(a AccessFinder, m MemberLister, u users.Lookup) *Usecase
     type Result struct{ Items []base.InvitationDTO }
     func (u *Usecase) Execute(ctx context.Context, projectID string) (Result, error)
     ```
     Urutan `Execute`: `ActorFrom` (gagal -> `CodeUnauthorized` "perlu login") -> `FindAccess` (error dibungkus `listinvitations: find access: %w`) -> `!found` -> `ErrProjectNotFound` -> `Require(ResolveLevel(actor.Role, memberRole, true), PermManageMembers)` -> `ListMembers` -> loop: cek `ctx.Err()` per anggota, `users.Get` (error: `fmt.Errorf("listinvitations: get-user %s: %w", id, err)`, kembalikan, tidak melanjutkan), `ToInvitationDTO`, `append` bila ok -> `SortInvitations` -> `Result`. Usecase hanya orkestrasi; filter dan urutan ada di entity.
   - Anggota yang tidak punya undangan tetap memicu satu `Get` (tidak ada cara tahu tanpa bertanya). Tidak ada optimasi prematur; catat sebagai debt di komentar.
   - `handler.go`: `executor` interface, `response{Items []base.InvitationDTO \`json:"items"\`}`, `items == nil` -> `[]`, `Cache-Control: no-store`, 200 lewat `respond.WriteJSON`, error lewat `respond.WriteError`. Param `chi.URLParam(r, "projectID")`. Body dan query diabaikan.
   - `init.go`:
     ```go
     func Init(d app.Deps) {
         s := store.New(d.DB)
         uc := NewUsecase(s, s, users.New(d.Bus))
         d.API.With(middleware.RequireAuth()).Get("/projects/{projectID}/invitations", newHandler(uc).ServeHTTP)
     }
     ```
     Tanpa repository baru: `store.Store` sudah menyediakan `FindAccess` dan `ListMembers` (dipakai `listmembers`). Tidak ada SQL baru, sehingga tidak ada `repository.go`.
6. **Wiring `management/base.go`**: tambahkan `listinvitations.Init(d)` (pola import dan urutan sama dengan `listmembers.Init`). `management-inviteuser` memasang `Post` pada pola yang sama di Init-nya; urutan Init bebas.
7. **OpenAPI `api/modules/project/management.yml`**:
   - Path item baru `/api/projects/{projectID}/invitations` (parameters `ProjectID`) dengan `get` `operationId: listInvitations`, tag `project-invitations`, `security: [{ sessionCookie: [] }]`, respons 200 `InvitationList`, 401, 403, 404 (`ProjectNotFound`), 500. Deskripsi: owner dan super_admin; member 403; bukan anggota 404; urut `added_at` menurun; gagal baca data user -> 500.
   - Skema baru `Invitation` (required semua 12 field) dan `InvitationList`. Skema `Member`: tambah `invitation_pending` (boolean, required) dan `invitation_expires_at` (date-time nullable, required). `MemberCandidate`: tambah `invitation_pending` (boolean, required). Naikkan `info.version` minor sesuai foundation bila belum.
   - Tambahkan path baru ke `api/root.yml` (hanya bila `management-inviteuser` belum melakukannya; satu entri untuk path ini). Lalu `make api-bundle` dan commit `api/openapi.yml`; `make api-check`.
   - Contoh respons 200 identik dengan bagian 7.
8. **Dokumen**: `docs/tickets/README.md` katalog bus: catatan konsumen `core.auth.get-user` bertambah `management listinvitations`. Tidak ada aksi audit baru.

Kontrak bus yang dikonsumsi (tidak diubah di sini):

```go
// core.auth.get-user (aditif T12), dipanggil via users.Lookup.Get:
getuser.Request{UserID: id}
getuser.Response{Found, Name, Email, Role, IsActive, Invitation *getuser.Invitation}
// getuser.Invitation{InvitedAt, IssuedAt, ExpiresAt, SentAt *time.Time, Expired bool, DeliveryStatus string, ResendCount int}
```

---

## 4. Migration

Tidak ada. Tabel `user_invitations` dibuat migrasi core foundation (overview bagian 5). Module management tidak menyentuh tabelnya (hanya lewat bus).

---

## 5. Changes (per file/folder)

| File / folder | Aksi | Isi |
|---|---|---|
| `management/base/invitation.go` | baru | `InvitationDTO`, `Member.ToInvitationDTO`, `EmailFailed`, `SortInvitations` |
| `management/base/invitation_test.go` | baru | unit entity (bagian 6) |
| `management/base/dto.go` | ubah | `MemberDTO.InvitationPending/InvitationExpiresAt`, `CandidateDTO.InvitationPending` |
| `management/base/member.go` | ubah | `Member.ToDTO` mengisi penanda dari `u.Invitation` |
| `management/base/member_test.go` | ubah | kasus penanda di `ToDTO` |
| `management/base/errors.go`, `errors_test.go` | ubah | `ReasonInvitationNotPending`, `ErrInvitationNotPending` |
| `management/searchmembercandidates/usecase.go`, `usecase_test.go`, `handler_test.go` | ubah | isi dan uji `invitation_pending` |
| `management/listmembers/handler_test.go`, `usecase_test.go`, `listmembers_integration_test.go` | ubah (tes saja) | tambah kunci JSON baru di daftar field wajib; `usecase.go` dan `handler.go` tidak berubah |
| `management/listinvitations/{usecase,handler,init}.go` | baru | usecase, handler, route |
| `management/listinvitations/{usecase,handler}_test.go` | baru | unit |
| `management/listinvitations/listinvitations_integration_test.go` | baru | integrasi (tag `integration`) |
| `management/base.go` | ubah | `listinvitations.Init(d)` |
| `management/management_scenarios_integration_test.go` | ubah | skenario e2e backend (bagian 7) |
| `api/modules/project/management.yml`, `api/root.yml`, `api/openapi.yml` | ubah | path, skema, bundle |
| `docs/tickets/README.md` | ubah | catatan konsumen get-user |

Aturan arsitektur terpenuhi: akses user hanya lewat `base/users` (bus), tidak ada SQL baru, usecase hanya orkestrasi, filter/urut/penanda di entity `base`, tidak ada import antar feature.

---

## 6. Test Scenario

### Unit

`base/invitation_test.go` (tabel):

| Kasus | Harapan |
|---|---|
| `u.Found=false` | `ok=false` |
| `Found=true`, `Invitation=nil` (akun biasa atau sudah diterima) | `ok=false` |
| undangan menunggu, `delivery_status=sent`, `SentAt` terisi | DTO lengkap: `email`, `name`, `role` dari member, `added_at` UTC, `invited_by_label` dari `AddedByLabel`, `email_failed=false` |
| `delivery_status=failed` | `email_failed=true`, `sent_at=null` |
| `delivery_status=not_configured` | `email_failed=true` |
| `delivery_status=pending` | `email_failed=false` |
| `Expired=true` | `expired=true`, tetap `ok=true` |
| `AddedByLabel=nil` | `invited_by_label=null` |
| `SortInvitations` tiga item beda `added_at` | urut menurun; dua item `added_at` sama -> `user_id` naik; stabil terhadap pemanggilan ulang |
| `EmailFailed` | tabel empat status plus string tak dikenal (`false`) |

`base/member_test.go`: `ToDTO` dengan `Invitation` -> `invitation_pending=true` dan `invitation_expires_at` UTC; `Invitation=nil` -> false/nil; `Found=false` -> false/nil; undangan kedaluwarsa -> tetap `true`. `errors_test.go`: `ErrInvitationNotPending` = `CONFLICT`, `details.reason=INVITATION_NOT_PENDING`.

`searchmembercandidates/usecase_test.go`: developer dengan `InvitationPending=true` -> `CandidateDTO.InvitationPending=true`; yang lain false; filter anggota dan `limit` tidak berubah.

`listinvitations/usecase_test.go` (fake `AccessFinder`/`MemberLister`/`users.Lookup` seperti di `listmembers`):

1. Tanpa identitas -> `CodeUnauthorized`, tanpa panggilan repo.
2. `found=false` -> `ErrProjectNotFound`, `ListMembers` dan `Get` tidak dipanggil.
3. Aktor developer dengan `memberRole=""` (non-anggota) -> `ErrProjectNotFound` (404).
4. Aktor `member` -> `ErrForbidden`, `ListMembers`/`Get` tidak dipanggil.
5. Aktor `owner` dan `super_admin` (non-anggota) -> sukses.
6. Filter: lima anggota (dua undangan menunggu, satu diterima/Invitation nil, satu `Found=false`, satu owner biasa) -> hanya dua item.
7. Urutan `added_at` menurun walau `ListMembers` mengembalikan urutan naik.
8. `FindAccess` error -> error dibungkus, bukan 404. `ListMembers` error -> dibungkus.
9. Error `Get` untuk anggota kedua dari tiga -> seluruh `Execute` gagal, `errors.Is` ke error asli, tidak ada hasil parsial, anggota ketiga tidak dipanggil.
10. `ctx` dibatalkan di tengah loop -> error `ctx`, sisa anggota tidak dipanggil.
11. Tanpa undangan sama sekali -> `Items` kosong (bukan error).
12. `Get` dipanggil tepat sekali per anggota, id sesuai urutan `ListMembers`.

`listinvitations/handler_test.go`: 200 dengan semua 12 kunci JSON; `Items=nil` -> `"items":[]` (bukan `null`); `sent_at` null ter-encode `null`; header `Content-Type: application/json` dan `Cache-Control: no-store`; query dan body diabaikan; `projectID` diteruskan ke usecase; `ErrProjectNotFound` -> 404, `ErrForbidden` -> 403, `CodeUnauthorized` -> 401, error biasa -> 500 tanpa membocorkan pesan internal.

`listmembers` handler/usecase tests: tambah dua kunci baru pada daftar kunci wajib dan satu kasus `Invitation` terisi (tes saja, kode tidak berubah; ini membuktikan `usecase.go` tidak perlu diubah).

### Integrasi (`-tags integration`, `db.TestDB`)

`listinvitations_integration_test.go` (pola `listmembers_integration_test.go`: bus fake terdaftar untuk `BUS_CORE_AUTH_GET_USER`, `store.New(d)`, chi router):

| Kasus | Harapan |
|---|---|
| project dengan owner, member biasa (Invitation nil), dua undangan menunggu | 200, hanya dua undangan, urut `added_at` menurun (baris `project_members.added_at` di-set eksplisit) |
| member (bukan owner) memanggil | 403 `FORBIDDEN` |
| developer non-anggota memanggil | 404 `NOT_FOUND` |
| project tidak ada atau id bukan uuid | 404 |
| super_admin non-anggota | 200 |
| get-user gagal untuk satu anggota | 500, tidak ada hasil parsial |
| provider `get-user` tidak terdaftar | 500 |
| `invited_by_label` null di DB | `null` di JSON |
| anggota di project lain | tidak muncul (isolasi per project) |

Perluasan integrasi lain: `listmembers_integration_test.go` memverifikasi `invitation_pending/invitation_expires_at` pada pengguna dengan undangan; `management_scenarios_integration_test.go` untuk `member-candidates` memverifikasi `invitation_pending` dari `finddevelopers` fake.

Lintas module (bila `auth-invitation-markers` sudah merged; memakai harness `setupWith` yang meregistrasi provider auth asli bila tersedia, jika tidak fake yang meniru kontrak): akun dibuat lewat `invite-user` lalu ditambahkan sebagai anggota -> muncul di daftar; setelah `AcceptInvitation` -> hilang dari daftar tetapi tetap ada di `members`.

---

## 7. E2E Scenarios

Skenario backend end-to-end (HTTP + DB + bus, `management_scenarios_integration_test.go`):

Positif:
1. Owner mengundang `baru@contoh.id` (jalur `management-inviteuser`) lalu `GET /invitations` -> satu item `role=member`, `delivery_status=sent`, `email_failed=false`, `expired=false`, `invited_by_label` nama owner, `expires_at` = terbit + TTL.
2. Dua undangan dengan waktu berbeda -> yang terbaru di atas.
3. Undangan dengan SMTP mati (`delivery_status=failed`) atau email tidak dikonfigurasi (`not_configured`) -> `email_failed=true`, `sent_at=null`; setelah Kirim ulang berhasil -> `email_failed=false`, `resend_count` naik, `sent_at` terisi.
4. Undangan kedaluwarsa (`expires_at` dimundurkan) -> `expired=true`, tetap muncul.
5. Penerima login dan mengganti password -> undangan hilang dari daftar; `GET /members` memuat dia dengan `invitation_pending=false`.
6. Undangan dibatalkan (`revokeinvitation`) -> hilang dari daftar dan dari `members`.
7. `GET /members` untuk anggota hasil undangan menunggu -> `invitation_pending=true` dan `invitation_expires_at` terisi; `GET /member-candidates?q=` untuk developer dengan undangan menunggu yang belum jadi anggota (undangan dari Users) -> `invitation_pending=true`.
8. Super admin non-anggota melihat daftar.

Negatif:
1. Member biasa -> 403; developer non-anggota -> 404; project tidak ada -> 404; tanpa sesi -> 401.
2. Provider auth gagal untuk satu anggota -> 500, tanpa daftar parsial.
3. Tidak ada undangan -> 200 `{"items": []}`.
4. Metode lain pada path (PUT/PATCH) -> 405 (route hanya GET/POST).
5. Tidak ada audit baru tercatat oleh GET (cek `auditList()` tidak bertambah).
6. Respons tidak pernah memuat `temporary_password`, `credentials`, atau hash (cek string body).

UI (Playwright, `frontend/e2e/project-management.spec.ts`, ditulis bersama `frontend-plan.md`): owner membuka daftar undangan di halaman Members, melihat status "Email gagal" untuk `email_failed=true`, "Kedaluwarsa" untuk `expired=true`; member tidak melihat tab undangan.

---

## 8. Request/Response body

### 8.1 Request

`GET /api/projects/0192f5a0-aaaa-7aaa-8bbb-000000000001/invitations`

Tanpa body, tanpa query. Header: cookie sesi. Tidak memerlukan `X-Requested-With` (GET).

### 8.2 200

Header `Cache-Control: no-store`.

```json
{
  "items": [
    {
      "user_id": "7c1f3c2e-5b7a-4d7e-9a51-1f0b8a3e6d21",
      "email": "baru@contoh.id",
      "name": "Budi",
      "role": "member",
      "invited_by_label": "Owner Satu",
      "added_at": "2026-10-21T08:15:00Z",
      "sent_at": null,
      "expires_at": "2026-10-28T08:15:00Z",
      "expired": false,
      "delivery_status": "failed",
      "email_failed": true,
      "resend_count": 1
    },
    {
      "user_id": "3a9d1e47-62c0-4f0b-8e1d-5c7a9b2f4e10",
      "email": "sari@contoh.id",
      "name": "sari",
      "role": "owner",
      "invited_by_label": "Owner Satu",
      "added_at": "2026-10-14T03:00:00Z",
      "sent_at": "2026-10-14T03:00:01Z",
      "expires_at": "2026-10-21T03:00:00Z",
      "expired": true,
      "delivery_status": "sent",
      "email_failed": false,
      "resend_count": 0
    }
  ]
}
```

Kosong:

```json
{ "items": [] }
```

### 8.3 Perubahan aditif

`GET /api/projects/{projectID}/members` (item):

```json
{
  "user_id": "7c1f3c2e-5b7a-4d7e-9a51-1f0b8a3e6d21",
  "name": "Budi",
  "email": "baru@contoh.id",
  "is_active": true,
  "found": true,
  "role": "member",
  "added_at": "2026-10-21T08:15:00Z",
  "added_by": "0192f5a0-1111-7aaa-8bbb-000000000011",
  "added_by_label": "Owner Satu",
  "invitation_pending": true,
  "invitation_expires_at": "2026-10-28T08:15:00Z"
}
```

Anggota tanpa undangan menunggu: `"invitation_pending": false, "invitation_expires_at": null`.

`GET /api/projects/{projectID}/member-candidates?q=baru` (item):

```json
{ "id": "7c1f3c2e-5b7a-4d7e-9a51-1f0b8a3e6d21", "name": "Budi", "email": "baru@contoh.id", "invitation_pending": true }
```

### 8.4 Error

Bentuk envelope mengikuti `httpx.WriteError` yang ada (`error.code`, `error.message`, `error.details`, `trace_id` bila ada).

401 tanpa sesi:

```json
{ "error": { "code": "UNAUTHORIZED", "message": "perlu login" } }
```

403 (anggota biasa):

```json
{ "error": { "code": "FORBIDDEN", "message": "akses ditolak" } }
```

404 (non-anggota, project tidak ada, id tidak valid; tidak dibedakan):

```json
{ "error": { "code": "NOT_FOUND", "message": "project tidak ditemukan" } }
```

500 (get-user atau DB gagal, tanpa detail internal):

```json
{ "error": { "code": "INTERNAL", "message": "terjadi kesalahan internal" } }
```

---

## 9. Risiko dan catatan

- N+1 get-user: biaya sama dengan `listmembers`; project dengan ratusan anggota akan lambat. Mitigasi di luar lingkup (butuh bus batch get-users); catat bila T11/T12 pasca-rilis menunjukkan masalah.
- Skema respons `Member` berubah aditif: klien lama aman (kunci tambahan). Tes kontrak OpenAPI (`make api-check`) memaksa spesifikasi mengikuti.
- `expired` bergantung pada jam provider auth; selisih waktu antar pemanggilan tidak berarti karena satu proses.
- Kode `ErrInvitationNotPending` ditulis di sini tetapi belum dipakai; linter `unused` tidak memprotes karena diekspor di paket `base`.
