# NooBee Grimoire — Daftar Tiket

Sumber: `docs/superpowers/specs/2026-10-20-grimoire-design.md`.
Aturan: **1 tiket = 1 module**. Satu tiket mencakup backend (Go) dan UI (Svelte) milik module itu.

## Daftar tiket


| ID                                      | Domain                 | Module                                                                           | Ukuran | **Bergantung pada**     | Status                                                                     |
| --------------------------------------- | ---------------------- | -------------------------------------------------------------------------------- | ------ | ----------------------- | -------------------------------------------------------------------------- |
| [T00](T00-foundation.md)                | infra                  | foundation (bus, db, http, static, CLI)                                          | L      | –                       | Selesai (`45a7ad5`)                                                        |
| [T01](T01-core-auth.md)                 | core                   | auth                                                                             | L      | T00                     | Selesai (`329d71d`)                                                        |
| [T02](T02-core-audit-log.md)            | core                   | audit-log                                                                        | M      | T00                     | Selesai (`d0a7c19`; e2e terautentikasi aktif, 1 skenario menunggu T04/T07) |
| [T03](T03-project-management.md)        | project                | management (project + anggota)                                                   | M      | T01, T02                | Selesai (`1b25823`)                                                        |
| [T04](T04-project-api-key.md)           | project                | api-key                                                                          | M      | T03                     | Selesai (`d4265df`)                                                        |
| [T05](T05-project-knowledge.md)         | project                | knowledge                                                                        | L      | T03                     | Selesai (`3221efc`)                                                        |
| [T06](T06-project-api-spec.md)          | project                | api-spec                                                                         | L      | T03                     | Selesai (0a26586)                                                          |
| [T07](T07-discovery-mcp-gateway.md)     | discovery              | mcp-gateway                                                                      | M      | T02, T04                | Selesai (`35c4fa4`)                                                        |
| [T08](T08-discovery-knowledge-tools.md) | discovery              | knowledge-tools                                                                  | S      | T05, T07                | Selesai (`b8699f4`)                                                        |
| [T09](T09-discovery-api-spec-tools.md)  | discovery              | api-spec-tools                                                                   | S      | T06, T07                | Selesai (`741a85c`)                                                        |
| [T10](T10-project-api-spec-swagger2.md) | project                | api-spec (dukungan Swagger 2.0)                                                  | L      | T06, T09                | Selesai (`f44ab86`)                                                        |
| [T11](T11-discovery-search-quality.md)  | discovery              | knowledge-tools (pengukuran kualitas pencarian)                                  | M      | T02, T05, T08           | Selesai (`f266cf8`)                                                        |
| [T12](T12-core-invitation-email.md)     | infra + core + project | mailer, auth, management (undangan anggota lewat email)                          | L      | T01, T02, T03           | Selesai (`b1b54f0`)                                                        |
| [T13](T13-project-api-key-per-user.md)  | project                | api-key (kepemilikan key per user per project)                                   | M      | T03, T04                | Selesai (`3263fa5`)                                                        |
| [T14](T14-project-git-sync.md)          | core + project         | git-credential (PAT per user), git-sync (sync knowledge &amp; API spec dari git) | L      | T01, T02, T03, T05, T06 | Selesai (`e2cbdf5`)                                                        |
| [T15](T15-core-audit-log-summary.md)    | core                   | audit-log (ringkasan metrik MCP + baris tabel bisa diklik)                       | L      | T02, T07, T11           | Selesai (`0ccf83f`)                                                        |
| [T16](T16-project-notification.md)      | project                | notification (siaran email manual ke anggota project)                            | M      | T01, T02, T03, T12      | Tiket baru                                                                 |


Urutan kerja yang disarankan: T00 → (T01, T02 paralel) → T03 → (T04, T05, T06 paralel) → T07 → (T08, T09 paralel).

### Tree dependensi

Root: **T00**. Tiap tiket ditaruh di bawah induk utamanya; dependensi tambahan ditulis di kanan sebagai `+ butuh`. Tiket di cabang yang sama (saudara) bisa dikerjakan paralel.

```
T00 foundation (ROOT)
├── T01 core/auth
│   └── T03 project/management            + butuh T02
│       ├── T04 project/api-key
│       │   ├── T13 project/api-key per user   (revisi T04)
│       │   └── T07 discovery/mcp-gateway  + butuh T02
│       │       ├── T08 discovery/knowledge-tools   + butuh T05
│       │       │   └── T11 discovery/search-quality    + butuh T02, T05
│       │       │       └── T15 core/audit-log summary   + butuh T02
│       │       └── T09 discovery/api-spec-tools    + butuh T06
│       ├── T05 project/knowledge
│       │   └── T14 git-sync (PAT + sync knowledge/api-spec)   + butuh T06
│       ├── T12 mailer + auth + management (undangan email)   + butuh T02
│       │   └── T16 project/notification (siaran email)
│       └── T06 project/api-spec
│           └── T10 project/api-spec Swagger 2.0   + butuh T09
└── T02 core/audit-log                     (dipakai T03 dan T07)
```

Urutan kerja per gelombang (semua tiket dalam satu gelombang bisa paralel):


| Gelombang | Tiket              |
| --------- | ------------------ |
| 1         | T00                |
| 2         | T01, T02           |
| 3         | T03                |
| 4         | T04, T05, T06      |
| 5         | T07                |
| 6         | T08, T09           |
| 7         | T10, T11, T12      |
| 8         | T13, T14, T15, T16 |


Jalur kritis: T00 → T01 → T03 → T04 → T07 → T08/T09 (6 gelombang).

Catatan: T00 bukan module domain. Ia ada karena bus, koneksi DB, server HTTP, dan penyajian static Svelte dibutuhkan semua module.

## Perubahan dari rencana awal (tolong diperiksa)

Ini keputusan saya, bukan keputusan yang sudah kamu setujui.

1. **Module `project` dipecah menjadi `management`, `api-key`, `knowledge`, `api-spec`.** Rencana awalmu hanya menyebut knowledge dan api-spec. Project, anggota, dan API key butuh rumah, dan API key harus bisa diverifikasi lewat bus oleh discovery.
2. **Domain `discovery` dipecah menjadi `mcp-gateway`, `knowledge-tools`, `api-spec-tools`.** Gateway mengurus transport, auth key, rate limit, dan audit. Tool-nya dipisah per sumber data.
3. **Tabel `files` di desain dipecah menjadi `knowledge_files` dan `api_specs`.** Dua module tidak boleh berbagi satu tabel. Desain perlu diperbarui agar sesuai.
4. **Aturan arsitektur lamamu yang sengaja TIDAK dipakai**, karena bertentangan dengan desain yang sudah disetujui:
  - SCD2 / effective-dated data: tidak dipakai. Desain memutuskan tanpa riwayat versi dan hapus permanen.
  - Soft delete (`deleted_at`): tidak dipakai. Desain memutuskan hard delete.
  - Multi-tenant dan override per-client: tidak dipakai (tim internal).
  - RabbitMQ, Redis, cmd/worker, cmd/cron: tidak dipakai. Satu binary dan satu Postgres. Job terjadwal berjalan di dalam binary.
  - RBAC action catalog: tidak dipakai. Hanya 2 role user (`super_admin`, `developer`) plus API key.

**Catatan revisi T15 (dashboard metrik MCP):**

1. `mcp.auth_rejected` di tiket dibaca sebagai konstanta sebenarnya `mcp.auth.rejected`.
2. `search_quality` yang gagal dimuat bernilai `{available:false, error_code:"UNAVAILABLE"}`, bukan `null`; `null` hanya bila `project_id` kosong.
3. Field aditif pada respons: `total_errors`, `error_rate`, `project_id`.
4. Label project berasal dari label terbaru di `audit_log`, tanpa penanda "dihapus".
5. Penempatan UI berubah dari tab Audit Log menjadi halaman `/dashboard` khusus Super Admin; endpoint tetap di module audit-log.

## Konvensi arsitektur (berlaku untuk semua tiket)

### Backend (Go)

```
backend/
├── cmd/
│   ├── {http,cli}/                 # http: server; cli: create-admin, migrate
│   └── internal/{server,buildinfo}/ # perakitan bersama dua entrypoint
├── internal/
│   ├── domain/
│   │   ├── core/
│   │   │   ├── base.go             # wiring domain
│   │   │   └── internal/{auth,auditlog,gitcredential}/
│   │   ├── project/internal/{management,apikey,knowledge,apispec,gitsync}/
│   │   └── discovery/internal/{mcpgateway,knowledgetools,apispectools}/
│   ├── infra/{bus,db,http,middleware,config,static,logger,jobs,app,mailer,crypto,safehttp,gitprovider}/  # app: hanya struct Deps; mailer: pengirim email generik; crypto/secretbox: AES-256-GCM; safehttp: http.Client anti-SSRF; gitprovider: klien REST GitHub/GitLab
│   ├── archtest/                   # tes arsitektur (tanpa kode produksi)
│   └── eventregistry/              # hanya bila ada event bernama
├── lib/{common,constants,contracts,util}/
└── migrations/{core,project}/      # YYYYMMDDHHMMSSmmm_*.sql
```

Di dalam module:

```
<module>/
├── base.go                 # wiring module: daftarkan route + handler bus
├── base/                   # entity + business rule (tanpa I/O)
└── <feature>/              # 1 folder = 1 feature
    ├── init.go, handler.go, params.go, usecase.go, repository.go, *_test.go
```

Aturan:

- Domain dan module tidak boleh saling meng-import (ditegakkan `internal/` Go). Interaksi antar module dan antar domain **hanya lewat `infra/bus`**.
- Konstanta endpoint bus di `lib/constants/bus.go`. Tipe request/response/error di `lib/contracts/<domain>/<module>/<nama>/` (package netral).
- `bus.Call` untuk lookup/validasi sinkron. `bus.Publish` untuk efek samping fire-and-forget. Handler yang sama terdaftar sekali; registrasi ganda = panic.
- Hasil "tidak ketemu" dikembalikan sebagai `Found bool`, bukan error.
- Usecase hanya orkestrasi. Rule bisnis ada di entity. Repository satu-satunya yang menyentuh SQL (sqlx, transaksi lewat `BeginTx(ctx, func...)`).
- Route di bawah `/api/projects` didaftarkan dengan pola penuh langsung di `d.API` (tanpa `Route`/`Mount` pada `/projects`). Nama param WAJIB `{projectID}` (dan `{userID}` untuk user). Contoh T04: `d.API.With(middleware.RequireAuth()).Get("/projects/{projectID}/api-keys", h)`.
- Foreign key antar tabel milik module berbeda dibatasi pada `project_id`/`user_id` dengan `ON DELETE CASCADE` (lihat T03). Selain itu tidak ada join lintas module.

### Frontend (Svelte)

```
frontend/src/
├── app/{routes,layouts}/           # bootstrap, guard route, komposisi view lintas module
├── domain/{core,project,discovery}/<module>/<feature>/
│   └── components/ services/ stores/ views/ types.ts index.ts
└── shared/{components,services/http.ts,stores,validation,utils}/
```

- `services/` memanggil API dan memvalidasi respons (Zod); tanpa state. `stores/` untuk state UI/session. `components/` presentasi. `views/` komposisi tipis per route.
- Tiap feature hanya mengekspos API publik lewat `index.ts`. Tidak ada import silang ke internals module lain.
- Build menghasilkan static file yang di-embed ke binary Go (lihat T00).

### Kontrak API

Spesifikasi OpenAPI REST tiap module ada di `api/modules/<domain>/<module>.yml`, di-bundle menjadi `api/openapi.yml` (hasil generate, jangan diedit manual). Prefix REST: `/api`. MCP: `/mcp`.

- Tiap module menambah `$ref` path-nya ke `api/root.yml` di PR yang sama. Komponen bersama (`Error`, responses) dirujuk dari `api/modules/infra/foundation.yml`.
- Setelah mengubah `api/modules/**` atau `api/root.yml`, jalankan `make api-bundle` dan commit `api/openapi.yml`.
- CI menjalankan `make api-check` (gagal bila `openapi.yml` basi/diedit manual, atau ada `api/modules/**.yml` yang tidak dirujuk `api/root.yml`) dan `make api-selftest` (membuktikan `api-check` benar-benar gagal).

### Penegakan otomatis

Batas arsitektur ditegakkan, bukan hanya didokumentasikan:

- Backend: `backend/internal/archtest` memindai seluruh import repo (termasuk `_test.go` dan tag `integration`) terhadap `rules.txt` bagian 4. Tanpa allowlist.
- Frontend: `frontend/.dependency-cruiser.cjs` (`npm run depcruise`) dan `npm run test:arch`/`node scripts/test-arch.mjs` yang membuktikan aturan benar-benar aktif lewat fixture.
- CI `.github/workflows/ci.yml`: job `lint`, `test`, `test-integration`, `api-check`, `depcruise`, `build`, `e2e`. Pelanggaran = build gagal.

## Katalog endpoint bus


| Endpoint                                         | Mode                                                                                        | Provider                                  | Consumer                                          |
| ------------------------------------------------ | ------------------------------------------------------------------------------------------- | ----------------------------------------- | ------------------------------------------------- |
| `core.auth.resolve-session`                      | Call                                                                                        | T01                                       | T00 (middleware)                                  |
| `core.auth.create-admin`                         | Call                                                                                        | T01                                       | `cmd/cli`                                         |
| `core.auth.get-user`                             | Call                                                                                        | T01                                       | T03, T06                                          |
| `core.auth.find-developers`                      | Call                                                                                        | T01                                       | T03                                               |
| `core.audit.record`                              | Call / Publish (satu handler untuk keduanya)                                                | T02                                       | T01, T03, T04, T05, T06, T07                      |
| `project.management.check-access`                | Call                                                                                        | T03                                       | T04, T05, T06                                     |
| `project.management.get-project`                 | Call                                                                                        | T03                                       | T07                                               |
| `project.knowledge.count-files`                  | Call                                                                                        | T05                                       | T03                                               |
| `project.apispec.count-specs`                    | Call                                                                                        | T06                                       | T03                                               |
| `project.apikey.count-keys`                      | Call                                                                                        | T04                                       | T03                                               |
| `project.apikey.verify`                          | Call                                                                                        | T04                                       | T07                                               |
| `project.apikey.touch`                           | Publish                                                                                     | T04                                       | T07                                               |
| `project.knowledge.list-files`                   | Call                                                                                        | T05                                       | T08                                               |
| `project.knowledge.read-file`                    | Call                                                                                        | T05                                       | T08                                               |
| `project.knowledge.search`                       | Call                                                                                        | T05                                       | T08                                               |
| `project.apispec.list-specs`                     | Call                                                                                        | T06                                       | T09                                               |
| `project.apispec.list-endpoints`                 | Call                                                                                        | T06                                       | T09                                               |
| `project.apispec.get-endpoint`                   | Call                                                                                        | T06                                       | T09                                               |
| `discovery.mcpgateway.register-tool`             | wiring (`lib/contracts/discovery/mcpgateway/toolkit` + `internal/domain/discovery/base.go`) | T07                                       | T08, T09                                          |
| `core.auth.invite-user`                          | Call                                                                                        | T12                                       | T12 (management `inviteuser`)                     |
| `core.auth.resend-invitation`                    | Call                                                                                        | T12                                       | T12 (management `inviteuser`, `resendinvitation`) |
| `project.management.count-memberships`           | Call (baca-saja)                                                                            | T12                                       | T12 (auth `purgeinvitations`)                     |
| `core.audit.scan-entries`                        | Call (baca-saja)                                                                            | T02 (feature `scanentries`, ditambah T11) | T11                                               |
| `discovery.knowledgetools.search-quality-report` | Call (baca-saja)                                                                            | T11                                       | `cmd/cli`, T15 (`core/auditlog` summary)          |
| `core.gitcredential.get-token`                   | Call                                                                                        | T14                                       | T14 (gitsync `runsync`, `getconfig`)              |
| `project.knowledge.upsert-files`                 | Call                                                                                        | T14                                       | T14 (gitsync `runsync`)                           |
| `project.apispec.upsert-specs`                   | Call                                                                                        | T14                                       | T14 (gitsync `runsync`)                           |
| `project.management.list-member-ids`             | Call (baca-saja)                                                                            | T16                                       | T16 (notification `sendbroadcast`)                |
| `core.auth.get-contacts`                         | Call (baca-saja)                                                                            | T16                                       | T16 (notification `sendbroadcast`)                |


Kontrak T05 ada di `lib/contracts/project/knowledge/{listfiles,readfile,search}`; `count-files` memakai `lib/contracts/project/management/contentcount`. `list-files` mengembalikan juga `Folders` (subfolder langsung bila `!Recursive`). `read-file`: `Offset/Limit` dalam byte, dipotong pada batas UTF-8. Provider T05 tidak menulis audit (gateway T07 yang mengaudit).

Kontrak `project.apikey.verify` dan `project.apikey.touch` ada di `lib/contracts/project/apikey/{verify,touch}` (konstanta `BUS_PROJECT_APIKEY_VERIFY`, `BUS_PROJECT_APIKEY_TOUCH`). Kontrak `project.management.check-access` dan `get-project` ada di `lib/contracts/project/management/{checkaccess,getproject}`; pola consumer ada di doc comment `checkaccess`. Hitungan isi (`count-files`, `count-specs`, `count-keys`) memakai kontrak netral `lib/contracts/project/management/contentcount` (`Request{ProjectIDs}` -&gt; `Response{Counts}`); T03 memanggilnya fail-soft (hitungan `null` bila provider belum ada). T03 memakai `core.audit.record` lewat Call.

Kontrak `project.apispec.list-specs`, `list-endpoints`, `get-endpoint` ada di `lib/contracts/project/apispec/{listspecs,listendpoints,getendpoint}` (konstanta `BUS_PROJECT_APISPEC_LIST_SPECS`, `BUS_PROJECT_APISPEC_LIST_ENDPOINTS`, `BUS_PROJECT_APISPEC_GET_ENDPOINT`); handler bus T06 tidak memeriksa akses user tetapi selalu membatasi query pada `ProjectID` request. `project.apispec.count-specs` memakai kontrak `contentcount`. Respons `get-endpoint` membawa penanda versi aditif `OpenAPIVersion` ("2.0" hanya untuk Swagger 2.0, "" untuk OpenAPI 3.x) beserta `Consumes`/`Produces` (array JSON, nil untuk 3.x).

Kontrak `core.audit.record`: `lib/contracts/core/audit/record` (konstanta `constants.BUS_CORE_AUDIT_RECORD`), request dikirim sebagai nilai `record.Request`.

Kontrak `core.audit.scan-entries`: `lib/contracts/core/audit/scanentries` (konstanta `BUS_CORE_AUDIT_SCAN_ENTRIES`); selalu dibatasi `ProjectID`, filter `Actions` dan `[From, To)`, keyset ASC `(occurred_at, id)`, `Limit` maks 1000, hanya `SELECT`. Kontrak `discovery.knowledgetools.search-quality-report`: `lib/contracts/discovery/knowledgetools/searchquality` (konstanta `BUS_DISCOVERY_KNOWLEDGETOOLS_SEARCH_QUALITY_REPORT`). Batas audit bersama `record.MaxTargetBytes`/`record.MaxResultJSONBytes` di `lib/contracts/core/audit/record/limits.go`. T11 sengaja menambah bus alih-alih membaca `audit_log` dari discovery/CLI (rules 2.4, 4.7).

Catatan: konstanta dan kontrak `core.auth.resolve-session` didefinisikan T00 (`lib/constants/bus.go`, `lib/contracts/core/auth/resolvesession`); handler diimplementasi T01.

TODO (ditambah T01): `core.auth.create-admin` (Call, provider T01, consumer `cmd/cli`). T01 menambah konstanta, kontrak, dan menyempurnakan baris katalog ini; sampai saat itu `create-admin` di CLI mencetak `create-admin belum tersedia (butuh T01)`.

(`register-tool` bukan endpoint runtime; `toolkit.Result.Structured` (opsional, JSON object) diteruskan gateway sebagai `structuredContent`, dipakai T08; mekanisme pendaftaran tool dijelaskan di T07.)

Kontrak T12: `lib/contracts/core/auth/{invitation,inviteuser,resendinvitation}` (konstanta `BUS_CORE_AUTH_INVITE_USER`, `BUS_CORE_AUTH_RESEND_INVITATION`) dan `lib/contracts/project/management/countmemberships` (`BUS_PROJECT_MANAGEMENT_COUNT_MEMBERSHIPS`). `Credentials` pada respons tidak boleh di-log, diaudit, atau di-Publish. Perubahan aditif: `core.auth.get-user` menerima `Request.Email` (dipakai bila `UserID` kosong) dan mengembalikan `Response.Invitation` (nil bila tidak ada undangan menunggu), konsumen bertambah management `inviteuser`/`listinvitations`; `core.auth.find-developers` mengembalikan `Developer.InvitationPending`. Kode error bersama baru `ALREADY_REGISTERED` (409) di `lib/common/errs`. Infra generik baru `infra/mailer` (`app.Deps.Mailer`; driver `smtp`, `log`, noop).

Kontrak T14: `lib/contracts/core/gitcredential/gettoken` (`BUS_CORE_GITCREDENTIAL_GET_TOKEN`), `lib/contracts/project/knowledge/upsertfiles` (`BUS_PROJECT_KNOWLEDGE_UPSERT_FILES`), `lib/contracts/project/apispec/upsertspecs` (`BUS_PROJECT_APISPEC_UPSERT_SPECS`). `get-token` mengembalikan `Token secret.String` (tipe tersamar di `lib/common/secret`): token tidak boleh di-log, diaudit, di-Publish, atau dikirim ke klien; pemanggil wajib mengisi `UserID` dari identity request. Dua handler upsert menulis tanpa cek keanggotaan: consumer wajib `check-access` lebih dulu; query selalu dibatasi `ProjectID` request. Orkestrasi tulis upload dipakai bersama lewat `knowledge/base/ingest` dan `apispec/base/ingest`. Kode error bersama baru di `lib/common/errs`: `HOST_NOT_ALLOWED` (422), `GIT_UPSTREAM_ERROR` (502), `GIT_FEATURE_DISABLED` (503). Env baru: `GIT_CREDENTIAL_KEY`, `GIT_SYNC_ALLOWED_HOSTS`, `GIT_SYNC_TIMEOUT`, `GIT_SYNC_INSECURE_TEST_HOSTS`, `GIT_SYNC_GITHUB_API_BASE` (lihat `docs/configuration.md`).

### Pola fail-open penulisan audit

Bila audit gagal ditulis, error di-log level error dan permintaan tetap dilayani (keputusan T02). Consumer:

```go
_, err := bus.CallTyped[record.Response](ctx, d.Bus, constants.BUS_CORE_AUDIT_RECORD, req)
if err != nil {
	log.ErrorContext(ctx, "audit write failed", "action", req.Action, "err", err)
}
// lanjutkan; jangan kembalikan error ke klien/agent.
```

Untuk `Publish`, log error yang sama bila `d.Bus.Publish` mengembalikan error. Jangan me-log `Args`/`Result` dan jangan mengirim isi file (kirim metadata: path, bytes, count).

### Aksi baru

Tambah konstanta di `backend/lib/contracts/core/audit/record/actions.go` (dan di `Actions()`), perbarui `actions_test.go`, lalu tambahkan ke katalog di bawah.

## Katalog aksi audit

Penamaan: `<area>.<objek>.<aksi>`, diawali `mcp.` untuk panggilan agent.

- Agent: `mcp.list_files`, `mcp.read_file`, `mcp.search_knowledge`, `mcp.list_api_specs`, `mcp.list_api_endpoints`, `mcp.get_api_endpoint`, `mcp.auth.rejected`, `mcp.rate_limited`, `mcp.initialize`, `mcp.tools_list`, `mcp.unknown_tool`.
- Auth/user: `auth.login.success`, `auth.login.failed`, `auth.logout`, `auth.password.changed`, `auth.password.reset`, `user.created`, `user.updated`, `user.deactivated`, `user.activated`, `user.invited`, `user.invitation.resent`, `user.invitation.accepted`, `user.invitation.email_failed` (T12).
- Project: `project.created`, `project.updated`, `project.deleted`, `project.member.added`, `project.member.removed`, `project.member.role_changed`, `project.member.invited`, `project.invitation.revoked` (T12).
- Key: `apikey.created`, `apikey.revoked`.
- Konten: `knowledge.file.created`, `knowledge.file.updated`, `knowledge.file.deleted`, `knowledge.file.moved`, `knowledge.folder.deleted`, `knowledge.upload.completed`, `apispec.uploaded`, `apispec.updated`, `apispec.deleted`.
- Git (T14): `git_credential.saved`, `git_credential.deleted`, `git_sync.run`.

## Pertanyaan terbuka lintas tiket (butuh keputusan kamu)

1. ~~**Kegagalan audit untuk panggilan agent**~~ **Diputuskan: fail-open.** Jika menulis audit gagal, panggilan MCP tetap dilayani dan error hanya di-log. Lihat T02 dan T07.
2. ~~**Basic Auth untuk agent:**~~ **Diputuskan: tidak didukung (hanya Bearer dan `X-API-Key`).** kamu awalnya menyebut "basic Auth atau apiKey". Rekomendasi saya hanya `Authorization: Bearer <key>` dan `X-API-Key`. Basic Auth dikecualikan kecuali klien MCP yang kamu pakai membutuhkannya. Lihat T07.
3. **Kedaluwarsa API key:** apakah key perlu tanggal kedaluwarsa opsional? Saya anggap tidak untuk versi pertama. Lihat T04.

