# T17 / project/git-sync / getconfig — Plan Usecase (revisi kecil)

Tiket: `docs/tickets/T17-project-git-sync-progress.md`. Dokumen induk (menang bila bertentangan): `docs/superpowers/plans/project-git-sync-progress/overview.md` (D9, D10, §5.4, §11.6, §12). Plan T14 feature ini: `docs/superpowers/plans/project-git-sync/usecase-getconfig.md` (alur akses -> kind -> config -> token tidak berubah).
Konteks yang dicek: `CLAUDE.md` tidak ada; `.claude/rules/` hanya `migrations.md` (tidak relevan, tanpa migrasi); `DESIGN.md` + `app-screens.md` untuk bagian Visual.

---

## 1. Metadata

| Atribut | Nilai |
|---|---|
| Ticket | T17 (revisi T14) |
| Scope | fullstack (backend getconfig + base/store, FE getconfig + dialog) |
| Domain / Module / Feature | project / git-sync (`gitsync`) / `getconfig` (revisi) |
| Folder backend | `backend/internal/domain/project/internal/gitsync/getconfig`, `.../gitsync/base/store` |
| Folder frontend | `frontend/src/domain/project/git-sync/getconfig`, `.../runsync/views/GitSyncDialog.svelte` |
| Endpoint | `/api/projects/{projectID}/git-sync?kind=knowledge\|api_spec` |
| Method | GET |
| Version | `/api` tanpa prefix versi; `api/modules/project/git-sync.yml` `info.version` 0.1.0 -> 0.2.0 (aditif, dibundel bersama runsync di F6 overview) |
| Perubahan kontrak | field aditif `in_progress` (bool, selalu ada) di respons |
| Siapa | anggota project, super_admin; `RequireAuth` (tidak berubah) |
| Bus | tidak ada kontrak baru; tetap `project.management.check-access` dan `core.gitcredential.get-token` (fail-soft) |
| Ketergantungan | `base/store` (`LockKeySQL`, `IsLocked`, dari F4 overview). Tidak bergantung pada usecase runsync; tes memakai lock yang diambil langsung di koneksi lain. |

---

## 2. Summary Plan

Dialog Sync yang dibuka ulang setelah SSE putus atau tab ditutup tidak tahu sync `(project, kind)` masih berjalan di server (sync lepas dari koneksi, D7). Tanpa tahu, user menekan Sync lalu menerima 409 `SYNC_IN_PROGRESS`. Revisi ini menambah `in_progress` ke `GetResponse` agar dialog menampilkan banner "Sync sedang berjalan di server", mengunci tombol Sync, dan menyediakan tombol "Muat ulang status".

Keputusan (mengikat):

1. `in_progress` dibaca dari `pg_locks` oleh `Store.IsLocked`, tanpa mengambil lock. DILARANG `pg_try_advisory_lock` lalu unlock: sync lain yang mencoba `TryLock` di jendela itu mendapat 409 palsu (D9).
2. Kunci memakai teks yang sama dengan `TryLock` lewat konstanta tunggal `store.LockKeySQL`; `IsLocked` tidak boleh menyalin string hashtext.
3. `in_progress` diisi juga saat `config == nil` (sync pertama sedang berjalan; config di-upsert sebelum pull tetapi bisa saja belum terlihat, dan kasus tersebut tetap harus benar).
4. Fail-soft: error cek lock -> `in_progress=false` + log `warn` (tanpa menggagalkan respons; paling buruk user melihat 409 seperti T14).
5. Urutan `Execute` tidak berubah: `access.Require` dulu (non-anggota selalu 404 sebelum ada pembacaan lain), lalu `ParseKind`, lalu `repo.Get`, lalu `repo.InProgress`, lalu status token. Cek lock tidak boleh jalan sebelum akses lolos.
6. Tanpa migrasi, tanpa bus baru, tanpa audit (read-only).
7. Tidak ada informasi siapa yang menjalankan sync dan sejak kapan (tidak ada sumbernya di `pg_locks`; di luar scope). `in_progress` hanya boolean.

Keterbatasan yang dicatat: `pg_locks` hanya melihat lock pada database yang sama (benar, satu Postgres satu database). Lock dilepas saat koneksi proses sync mati (crash), sehingga `in_progress` tidak pernah menggantung.

---

## 3. Plan (langkah berurutan)

### Langkah 1 — `base/store/store.go`: `IsLocked`

Bila F4 overview sudah menambahkan `LockKeySQL` dan `IsLocked`, verifikasi saja kesesuaiannya dengan di bawah dan lanjut ke langkah 2; selain itu tambahkan di sini.

```go
// LockKeySQL: kunci advisory (project, kind); $1 = project_id, $2 = kind.
// Satu-satunya definisi; dipakai TryLock (runsync) dan IsLocked.
const LockKeySQL = "hashtext('grimoire.gitsync.' || $1::text || '.' || $2::text)"

const isLockedSQL = `WITH k AS (SELECT (` + LockKeySQL + `)::bigint AS v)
SELECT EXISTS (
  SELECT 1 FROM pg_locks, k
  WHERE locktype = 'advisory' AND granted AND objsubid = 1
    AND database = (SELECT oid FROM pg_database WHERE datname = current_database())
    AND classid = ((k.v >> 32) & 4294967295)::oid
    AND objid   = (k.v & 4294967295)::oid)`

// IsLocked true bila advisory lock (project, kind) sedang dipegang sesi mana pun.
// Read-only: tidak mengambil lock. projectID bukan uuid -> (false, nil) tanpa query.
func (s *Store) IsLocked(ctx context.Context, projectID string, kind base.Kind) (bool, error) {
	if !ValidUUID(projectID) {
		return false, nil
	}
	var locked bool
	if err := s.q.GetContext(ctx, &locked, isLockedSQL, projectID, string(kind)); err != nil {
		return false, fmt.Errorf("store: is locked: %w", err)
	}
	return locked, nil
}
```

Catatan teknis wajib dipahami developer:
- `hashtext` bertipe int4. `pg_try_advisory_lock(bigint)` menerima int4 lewat sign-extension; pada key bigint, `classid` = 32 bit atas dan `objid` = 32 bit bawah, `objsubid=1` menandai bentuk satu argumen bigint. Hash negatif menghasilkan `classid = 4294967295`; karena itu cast `::bigint` dulu, lalu `>> 32` dan mask `& 4294967295`. Jangan memakai `hashtext(...)::oid` langsung.
- `$1::text` pada `LockKeySQL` sudah ada di T14; `projectID` tetap dibatasi uuid valid oleh `ValidUUID` sebelum query (konsisten dengan `SelectConfig`).
- Menambah import `fmt` di `store.go` bila belum ada.
- Doc comment paket `store` diperbarui: baris "(upsert config, SaveResult, lock) ada di repository runsync" menjadi "teks kunci lock dan pembacaan status lock ada di sini; pengambilan/pelepasan lock ada di runsync".

### Langkah 2 — `getconfig/repository.go`: `InProgress`

```go
// InProgress true bila sync (project, kind) sedang berjalan (advisory lock dipegang).
// Delegasi ke store.IsLocked; tidak mengambil lock.
func (r *Repo) InProgress(ctx context.Context, projectID string, kind base.Kind) (bool, error) {
	ok, err := r.s.IsLocked(ctx, projectID, kind)
	if err != nil {
		return false, fmt.Errorf("getconfig: in progress: %w", err)
	}
	return ok, nil
}
```

Repository tetap satu-satunya yang menyentuh SQL di feature ini (lewat store bersama). Tidak ada SQL baru di folder `getconfig`.

### Langkah 3 — `getconfig/usecase.go`

1. Interface:

```go
type ConfigReader interface {
	Get(ctx context.Context, projectID string, kind base.Kind) (*base.Config, error)
	InProgress(ctx context.Context, projectID string, kind base.Kind) (bool, error)
}
```

2. `GetResponse` tambah `InProgress bool \`json:"in_progress"\`` (tanpa `omitempty`; selalu ada; letakkan paling akhir).
3. Di `Execute`, setelah `repo.Get` sukses dan `resp` dibuat, SEBELUM cabang `cfg == nil`:

```go
resp := GetResponse{Kind: string(kind), FeatureEnabled: u.enabled}
resp.InProgress = u.inProgress(ctx, in.ProjectID, kind)
if cfg == nil {
	return resp, nil
}
```

4. Helper kecil di file yang sama (fail-soft, satu tempat log):

```go
func (u *Usecase) inProgress(ctx context.Context, projectID string, kind base.Kind) bool {
	ok, err := u.repo.InProgress(ctx, projectID, kind)
	if err != nil {
		u.log.WarnContext(ctx, "gitsync getconfig: cek lock sync gagal", "project_id", projectID, "kind", string(kind), "error", err)
		return false
	}
	return ok
}
```

5. Tidak ada perubahan pada `NewUsecase`, `init.go`, `handler.go`, `params.go`, route, atau `busCredentials`. `Repo` otomatis memenuhi `ConfigReader` yang diperluas; `init.go` tetap `newRepo(store.New(d.DB))`.
6. Error `repo.Get` tetap membatalkan respons (500) seperti T14; hanya error cek lock yang fail-soft. Cek lock dilakukan setelah `repo.Get` agar kegagalan DB yang sama tidak diperlakukan ganda.

### Langkah 4 — Tes backend (lihat §6)

Perbarui `fakeRepo` di `usecase_test.go` (field `inProgress bool`, `inProgressErr error`, `ipCalls int`, `ipProj string`, `ipKind base.Kind`, urutan panggilan). Tambah tes di `usecase_test.go`, `handler_test.go`, `repository_integration_test.go`, `base/store/store_test.go`.

### Langkah 5 — OpenAPI (`api/modules/project/git-sync.yml`)

Di `GitSyncConfigResponse` tambah ke `required` dan `properties` (bundle `api/openapi.yml` dikerjakan sekali di F6 overview; plan ini hanya mengubah modul sumber):

```yaml
      required: [kind, feature_enabled, has_credential, credential_unreadable, config, in_progress]
      properties:
        in_progress:
          type: boolean
          description: >-
            true bila sync untuk (project, kind) ini sedang berjalan di server (termasuk yang sedang menunggu
            rate limit provider). Dibaca tanpa mengambil lock; bila pengecekan gagal nilainya false.
```

Perbarui `example` respons bila ada (`"in_progress": false`). Jangan menjalankan `make api-bundle` di langkah ini bila runsync masih mengerjakan file yang sama; F6 menjalankannya sekali.

### Langkah 6 — Frontend `getconfig/types.ts` dan `getconfig/stores/gitSyncConfig.store.ts`

1. `types.ts`: tambah `in_progress: z.boolean().default(false)` ke `GetConfigResponseSchema` (default false demi kompatibilitas dengan server lama/mock tanpa field). Tipe `GetConfigResponse` hasil `z.infer` sudah memuat `in_progress: boolean` (output).
2. `gitSyncConfig.store.ts`:
   - `load` otomatis meneruskan field karena data datang dari `getConfig` (zod). Tidak ada perubahan alur.
   - `setFromRun`: tambahkan `in_progress: false` pada objek hasil (`{ ...base, has_credential: true, credential_unreadable: false, in_progress: false, config: {...} }`), karena sync baru saja selesai.
   - Tambahkan method `refresh(projectId, kind): Promise<void>` = alias `load` untuk kunci yang sama. TIDAK perlu; `load` untuk kunci sama sudah mempertahankan `previous` dan tidak memblok bila tidak sedang `loading`. Jangan menambah API baru; dialog memanggil `cfg.load(projectId, kind)`.
3. `index.ts` tidak berubah.

### Langkah 7 — Frontend dialog (`runsync/views/GitSyncDialog.svelte`)

Perubahan terbatas pada cabang non-`done` dan footer; tidak menyentuh alur run lokal (milik usecase runsync, yang menyentuh file yang sama: koordinasi, lihat §8).

1. State turunan:
   ```ts
   const serverBusy = $derived(data?.in_progress === true && !running);
   ```
   `running` = sync dimulai dari dialog ini (store runsync). Banner tidak tampil saat `running` atau `$run.status === 'done'`, karena tahap progress runsync yang menjadi tampilan.
2. Kunci tombol: `syncLocked` dan `rerunLocked` ditambah `|| serverBusy`; `canSync` otomatis ikut (karena memakai `syncLocked`). `SyncForm disabled` ditambah `|| serverBusy`.
3. Banner di atas `SyncForm` (setelah `ConfigSummary`/`CredentialNotice`, sebelum form), komponen baru kecil `runsync/components/InProgressNotice.svelte` (props: `loading: boolean`, `onreload: () => void`), `role="status"`, berisi:
   - judul `gitSync.inProgress.title` ("Sync sedang berjalan di server"),
   - satu kalimat `gitSync.inProgress.body`,
   - tombol sekunder kecil `gitSync.inProgress.reload` ("Muat ulang status"), `loading` saat `$cfg.status === 'loading'`.
4. Muat ulang: `onreload = () => { filled = false; void cfg.load(projectId, kind); }`. `filled=false` membuat `$effect` pengisian form mengisi ulang dari config terbaru (form terkunci selama banner, jadi tidak ada ketikan user yang tertimpa). Hasil: bila `in_progress` kini false, banner hilang, tombol Sync aktif, dan `ConfigSummary` menampilkan `last_status`/`last_result` terbaru.
5. Transisi sukses: bila setelah muat ulang `in_progress` berubah dari true ke false dan `config.last_status !== 'failed'`, panggil `onsynced()` satu kali (guard `announced` yang sudah ada) agar daftar knowledge/API spec di halaman induk ikut dimuat ulang. Implementasi: `$effect` yang membandingkan nilai `in_progress` sebelumnya (variabel biasa `wasBusy`) dengan yang sekarang, dibungkus `untrack`.
6. Footer: tidak berubah; `Batal` tetap menutup dialog. Tidak ada polling otomatis (user memutuskan lewat tombol; di luar scope).
7. i18n `frontend/src/shared/i18n/{id,en}.json` (kunci `gitSync.inProgress.*` milik plan ini):
   - id: `"gitSync.inProgress.title": "Sync sedang berjalan di server"`, `"gitSync.inProgress.body": "Sync dimulai dari sesi lain dan tetap berjalan sampai selesai. Muat ulang status untuk melihat hasilnya."`, `"gitSync.inProgress.reload": "Muat ulang status"`.
   - en: `"Sync is running on the server"`, `"The sync was started from another session and keeps running until it finishes. Reload the status to see the result."`, `"Reload status"`.
   Tambahkan berdampingan dengan kunci `gitSync.*` yang ada (urutan alfabetis mengikuti berkas).

### Visual (frontend)

| Pertanyaan layar | Jawaban |
|---|---|
| Apakah sync ini masih jalan, perlu saya tunggu, dan apa yang bisa saya lakukan? | Banner netral di atas form: "Sync sedang berjalan di server", penjelasan satu kalimat, satu aksi "Muat ulang status". Tombol Sync dan Sync ulang nonaktif. |

- Pola DESIGN.md: `Dialog` yang ada (`shadow-pop`, radius 16); banner mengikuti pola pesan inline netral (bukan error, bukan peringatan merah) sejajar `CredentialNotice` (blok bersudut `rounded-card`, padding 12-16).
- Elemen khas: ikon jam 16px di kiri judul (stroke 1.5, `currentColor`, `aria-hidden`), judul 14px medium, isi 13px `text-muted`, tombol `Button variant="secondary" size="sm"` di kanan (di ponsel turun ke baris sendiri). Tidak ada animasi berputar; status tidak hanya warna (ikon + teks).
- Token (diverifikasi di `frontend/src/app.css`): `bg-pill-neutral-bg` untuk latar, `text-pill-neutral-ink` untuk judul (bukan `-fg`), `text-muted`, `rounded-card`, `rounded-base` (tombol). Tanpa warna aksen kuning (aksen hanya untuk isian progress milik runsync).
- Data contoh (fixture vitest): project `p1`, repo `noobee/grimoire-docs` (`mockConfig()`), kind `knowledge`, `in_progress: true`. Dilarang Acme/lorem.
- Aksesibilitas: `role="status"` (pembaca layar mengumumkan perubahan), tombol dapat difokus dengan label jelas, kontras minimal AA.
- Lint: `node .agents/skills/design-taste-frontend/scripts/lint.mjs --app frontend/src` harus hijau.

---

## 4. Migration

Tidak ada. `in_progress` dibaca dari `pg_locks` (advisory lock T14), bukan kolom baru. Aturan `.claude/rules/migrations.md` tidak terpicu (tidak ada file `migrations/*.sql`).

---

## 5. Changes (per file/folder)

| File | Jenis | Isi |
|---|---|---|
| `backend/internal/domain/project/internal/gitsync/base/store/store.go` | ubah | `LockKeySQL` (bila belum dari F4), `isLockedSQL`, `Store.IsLocked`, doc paket |
| `.../gitsync/base/store/store_test.go` | ubah | `IsLocked` projectID bukan uuid -> false tanpa query |
| `.../gitsync/getconfig/repository.go` | ubah | `Repo.InProgress` |
| `.../gitsync/getconfig/usecase.go` | ubah | `ConfigReader.InProgress`, `GetResponse.InProgress`, helper `inProgress` fail-soft, isi sebelum cabang `cfg == nil` |
| `.../gitsync/getconfig/usecase_test.go` | ubah | `fakeRepo` diperluas, tes baru §6.1 |
| `.../gitsync/getconfig/handler_test.go` | ubah | tes `in_progress` ada di JSON |
| `.../gitsync/getconfig/repository_integration_test.go` | ubah | tes `IsLocked`/`InProgress` §6.2 |
| `api/modules/project/git-sync.yml` | ubah | `in_progress` di `GitSyncConfigResponse` (bundle di F6) |
| `frontend/src/domain/project/git-sync/getconfig/types.ts` (+ `types.test.ts`) | ubah | zod `in_progress` default false |
| `frontend/src/domain/project/git-sync/getconfig/stores/gitSyncConfig.store.ts` (+ test) | ubah | `setFromRun` set `in_progress: false`; field diteruskan dari load |
| `frontend/src/domain/project/git-sync/runsync/components/InProgressNotice.svelte` (+ entri di `components.test.ts`) | baru | banner + tombol muat ulang |
| `frontend/src/domain/project/git-sync/runsync/views/GitSyncDialog.svelte` (+ `GitSyncDialog.test.ts`) | ubah | `serverBusy`, kunci tombol/form, banner, muat ulang, `onsynced` pada transisi |
| `frontend/src/shared/i18n/{id,en}.json` | ubah | `gitSync.inProgress.{title,body,reload}` |
| `frontend/e2e/git-sync.spec.ts` | ubah | skenario §7 (fixture fake server milik runsync dipakai; lihat koordinasi) |

Tidak disentuh: `getconfig/{handler,params,init}.go`, route, `lib/constants/bus.go`, `lib/contracts/**`, `migrations/**`, `runsync/**` (kecuali pemindahan `lockKeySQL` -> `store.LockKeySQL` yang dimiliki F4/runsync).

Koordinasi: `GitSyncDialog.svelte`, `i18n/*.json`, dan `git-sync.yml` juga diubah plan runsync. Kunci i18n berbeda (`gitSync.progress.*` vs `gitSync.inProgress.*`), jadi merge cukup menyatukan; untuk dialog, jaga perubahan plan ini kecil dan terpisah (derived + banner + footer lock) agar mudah di-rebase. `serverBusy` tidak boleh bergantung pada state tahap progress runsync selain `running`.

---

## 6. Test Scenario

### 6.1 Unit usecase (`usecase_test.go`, repo palsu)

1. `in_progress` true: `fakeRepo.inProgress=true`, config ada -> `res.InProgress == true`; field lain (config, kredensial) tetap benar.
2. `in_progress` false: default fake -> `res.InProgress == false`.
3. Config nil + lock true (sync pertama berjalan): `res.Config == nil`, `res.InProgress == true`, bus get-token tidak dipanggil (`cred.calls == 0`).
4. Error cek lock: `inProgressErr = errors.New("boom")` -> `res.InProgress == false`, `err == nil`, log memuat `level=WARN` dan `project_id`/`kind`, respons lain utuh (config terisi, kredensial dievaluasi seperti biasa).
5. Akses diperiksa lebih dulu: `fakeAccess{err: ...}` (401/404) -> error dikembalikan, `fakeRepo.ipCalls == 0` dan `calls == 0`. Kind tidak valid -> 400 dan `ipCalls == 0`.
6. Argumen yang diteruskan: `ipProj == "p1"`, `ipKind == base.KindAPISpec` untuk `kind=api_spec`.
7. `repo.Get` error -> 500 (`getconfig: ...`), `ipCalls == 0` (cek lock tidak dijalankan).
8. Regresi T14: semua tes lama (credential states, feature disabled, no config) tetap hijau.

### 6.2 Integration (`repository_integration_test.go`, tag `integration`, `db.TestDB`)

Helper `holdLock(t, d, projectID, kind) (release func())`: ambil koneksi terpisah `d.Connx(ctx)` lalu `SELECT pg_advisory_lock(` + `store.LockKeySQL` + `)` dengan argumen `(projectID, kind)`; `release` menjalankan `pg_advisory_unlock` dan `conn.Close()`; daftarkan `t.Cleanup(release)` idempoten. (Alternatif setara bila `runsync` boleh diimpor tanpa melanggar aturan arsitektur: tidak dipakai di sini; satu feature tidak mengimpor feature lain.)

Helper `pickBySign(t, d, kind) (neg, pos string)`: buat project uuid acak berulang (maksimal 64 percobaan, `gen_random_uuid()::text` tanpa menyisipkan baris project) sampai `SELECT (` + `store.LockKeySQL` + `)::bigint < 0` ditemukan sekali dan `>= 0` sekali; `t.Fatal` bila tidak ketemu (peluang 2^-64).

Kasus:
1. Tidak ada lock -> `r.InProgress` false (project valid, kind knowledge).
2. `holdLock(p, knowledge)` -> `InProgress(p, knowledge)` true; setelah `release()` -> false.
3. Kind lain: lock `(p, knowledge)` -> `InProgress(p, api_spec)` false.
4. Project lain: lock `(p, knowledge)` -> `InProgress(q, knowledge)` false.
5. Hash negatif: `neg` dikunci -> true, setelah unlock false. Hash positif: `pos` dikunci -> true, setelah unlock false. Kedua tanda wajib terdeteksi (membuktikan sign-extension `classid`).
6. projectID bukan uuid (`"abc"`) -> false, nil, tanpa query (aman walau DB dipakai).
7. Tidak mengambil lock: saat `holdLock` aktif, panggil `InProgress` berkali-kali, lalu `release()`; sebuah koneksi lain `pg_try_advisory_lock(...)` langsung berhasil (true) -> `IsLocked` tidak meninggalkan lock tersisa. Selain itu, saat tidak ada lock, `InProgress` lalu `pg_try_advisory_lock` pada koneksi lain tetap true.
8. Lock hilang saat koneksi pemegang ditutup tanpa unlock (`conn.Raw` -> `driver.ErrBadConn` lalu close, atau `pg_terminate_backend`) -> `InProgress` false (tidak menggantung).
9. Sesi pemegang lock adalah koneksi pool lain, sedangkan `Store` memakai pool: hasil tetap true (pg_locks lintas sesi).
10. Regresi: `TestRepoGet` lama tetap hijau.

### 6.3 Handler (`handler_test.go`)

1. 200 dengan `inProgress=true`: body memuat `"in_progress":true`; dengan false: `"in_progress":false` (field selalu ada).
2. Config null + in_progress true: body memuat `"config":null` dan `"in_progress":true`.
3. Error cek lock tidak mengubah status: 200 + `"in_progress":false`.
4. `Cache-Control: no-store` tetap ada (nilai harus segar).
5. Error 400/401/404 tidak memuat `in_progress` (envelope error biasa).

### 6.4 Unit store (`base/store/store_test.go`)

1. `IsLocked` dengan projectID bukan uuid -> `(false, nil)` tanpa query (gunakan Querier palsu yang `t.Fatal` bila dipanggil).
2. Konstanta: `isLockedSQL` mengandung `LockKeySQL` (satu sumber kebenaran) dan tidak mengandung `pg_try_advisory_lock`/`pg_advisory_unlock` (jaga larangan D9).

### 6.5 Frontend vitest

1. `types.test.ts`: respons tanpa `in_progress` -> parse sukses, `in_progress === false`; dengan `true` -> true; nilai bukan boolean (`"yes"`) ditolak.
2. `gitSyncConfig.store.test.ts`: `load` meneruskan `in_progress: true`; `setFromRun` setelah load dengan `in_progress: true` menghasilkan `in_progress === false`; `load` ulang untuk kunci sama mengganti nilai (true -> false) dan mempertahankan `previous` selama loading.
3. `components.test.ts` (`InProgressNotice`): render judul "Sync sedang berjalan di server" dan tombol "Muat ulang status"; klik memanggil `onreload`; `loading` menonaktifkan tombol; `role="status"` ada.
4. `GitSyncDialog.test.ts`:
   - getConfig `in_progress: true`: banner tampil, tombol "Sync" dan "Sync ulang" nonaktif, field form nonaktif; tidak ada request `/run` meski tombol diklik.
   - `in_progress: false`: banner tidak ada (regresi).
   - Klik "Muat ulang status": request GET kedua dikirim; respons kedua `in_progress: false` + config dengan `last_status: 'success'` -> banner hilang, tombol Sync aktif, `onsynced` dipanggil satu kali.
   - Muat ulang masih `in_progress: true`: banner tetap, `onsynced` tidak dipanggil.
   - Muat ulang menghasilkan `last_status: 'failed'`: banner hilang, `onsynced` tidak dipanggil.
   - Config null + `in_progress: true`: banner tampil, tombol Sync nonaktif.
   - Saat `running` dari dialog ini, banner tidak tampil (tahap progress milik runsync).
   - Kegagalan muat ulang (500): `ErrorState`/data sebelumnya dipertahankan (perilaku store), banner tetap.
5. Lint desain dan `npm run test:arch` hijau (komponen baru berada di `runsync/components`, impor hanya dari `../../base` dan `shared`).

Perintah verifikasi: `go test ./internal/domain/project/internal/gitsync/...`, `go test -tags integration ./internal/domain/project/internal/gitsync/...`, `npm run test`, `npm run depcruise`, `npm run test:arch`.

---

## 7. E2E Scenarios (Playwright, `frontend/e2e/git-sync.spec.ts`)

Memakai fake git server dengan opsi `delayMs` per blob (milik plan runsync). Env server e2e: `GIT_SYNC_REQUEST_INTERVAL=0s`.

Positif:
1. Sync dimulai dari tab A (blob lambat), tab A ditutup (konfirmasi tutup). Di tab B buka dialog Sync: banner "Sync sedang berjalan di server" tampil, tombol Sync nonaktif. Setelah sync selesai di server, klik "Muat ulang status": banner hilang, ringkasan menampilkan hasil terakhir (dibuat/dilewati sesuai `last_result`), tombol Sync aktif, dan pohon Knowledge di halaman terisi (karena `onsynced`).
2. Sync pertama (belum ada config) berjalan lambat; dialog baru di tab lain menampilkan banner walau tanpa `ConfigSummary` (config null) dan form kosong terkunci.
3. Kind terpisah: sync `knowledge` berjalan, dialog `api_spec` pada project yang sama tidak menampilkan banner dan bisa Sync.
4. API: `GET /api/projects/{id}/git-sync?kind=knowledge` via `request.get` saat sync berjalan -> `in_progress: true`; sesudah selesai -> `false`.

Negatif:
5. Muat ulang saat sync masih berjalan: banner tetap, tombol Sync tetap nonaktif, tidak ada 409 (tidak ada request `/run` terkirim).
6. Banner tidak tampil di project lain yang sedang tidak sync (isolasi project).
7. Non-anggota: `GET` -> 404 (tanpa `in_progress`); tanpa login -> 401.
8. Respons `getconfig` gagal 500 saat muat ulang: pesan error tampil, data sebelumnya (banner) dipertahankan, tidak crash.
9. Respons API tidak memuat PAT/canary token; `in_progress` hanya boolean.

---

## 8. Request/Response body

### 8.1 Request

```http
GET /api/projects/0b7e5a52-3c1f-4d1e-9a51-6f1f0c2d9e10/git-sync?kind=knowledge
Cookie: <sesi>
```

### 8.2 200, sync berjalan, config ada

```json
{
  "kind": "knowledge",
  "config": {
    "provider": "github",
    "host": "github.com",
    "repo_url": "https://github.com/noobee/grimoire-docs",
    "repo_path": "noobee/grimoire-docs",
    "branch": "main",
    "folder": "docs",
    "target_prefix": "handbook",
    "last_status": "success",
    "last_synced_at": "2026-10-22T08:14:03Z",
    "last_synced_by": { "id": "5d2c9a7e-1b44-4f0b-8c1e-2a9d7f3e6b21", "label": "Reyhan" },
    "last_result": null
  },
  "has_credential": true,
  "credential_unreadable": false,
  "feature_enabled": true,
  "in_progress": true
}
```

(`last_result` pada contoh dipersingkat `null`; respons nyata memuat laporan terakhir seperti T14, dipotong 500 item.)

### 8.3 200, sync pertama berjalan, config belum ada

```json
{"kind":"api_spec","config":null,"has_credential":false,"credential_unreadable":false,"feature_enabled":true,"in_progress":true}
```

### 8.4 200, tidak ada sync berjalan (atau cek lock gagal, fail-soft)

```json
{"kind":"knowledge","config":null,"has_credential":false,"credential_unreadable":false,"feature_enabled":true,"in_progress":false}
```

Header: `Content-Type: application/json`, `Cache-Control: no-store`.

### 8.5 Format error (tidak berubah dari T14; tanpa field `in_progress`)

```http
HTTP/1.1 400 Bad Request

{"error":{"code":"INVALID_ARGUMENT","message":"kind tidak valid","details":{"fields":{"kind":"harus knowledge atau api_spec"}}},"trace_id":"7f3c2a9e4b1d4c08"}
```

```http
HTTP/1.1 404 Not Found

{"error":{"code":"NOT_FOUND","message":"project tidak ditemukan","details":null},"trace_id":"7f3c2a9e4b1d4c08"}
```

```http
HTTP/1.1 401 Unauthorized

{"error":{"code":"UNAUTHORIZED","message":"perlu login","details":null},"trace_id":"7f3c2a9e4b1d4c08"}
```

(Pesan persis mengikuti kode bawaan T14 di `gitsync/base/errors.go`.)

### 8.6 Skema OpenAPI

`GitSyncConfigResponse.in_progress: boolean` (required), seperti Langkah 5. Zod FE: `in_progress: z.boolean().default(false)`.
