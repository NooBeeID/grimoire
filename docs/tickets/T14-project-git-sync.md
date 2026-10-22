# T14 — core / git-credential + project / git-sync (sync knowledge & API spec dari git)

| | |
|---|---|
| Domain / Module | core / git-credential (PAT per user) + project / git-sync (tarik file dari repo) |
| Ukuran | L |
| Bergantung pada | T01, T02, T03, T05, T06 |
| Sumber | permintaan pemilik |

## Ringkasan

Hari ini knowledge (T05) dan API spec (T06) hanya masuk lewat upload manual. Tiket ini menambah tombol **Sync** di halaman Knowledge dan API Spec: user memasukkan URL GitHub/GitLab sampai ke folder target plus branch, lalu Grimoire **menarik** file di folder itu dan menimpa entri yang namanya sama. Akses ke repo privat memakai **Personal Access Token (PAT) milik user**, disimpan sekali per user (bukan per project).

## Tujuan

1. User menyimpan, mengganti, dan menghapus PAT per provider (GitHub, GitLab) di pengaturan akun. PAT terenkripsi, tidak pernah dikembalikan ke klien.
2. Tombol **Sync dari Git** di Knowledge dan di API Spec. Dialog menerima URL repo/folder dan branch.
3. Sync menarik file dari folder target (rekursif) dan menimpa yang bernama/berpath sama di project. Yang tidak ada di git dibiarkan.
4. URL + branch yang terakhir dipakai diingat per project (per jenis: knowledge, api-spec) dan mengisi dialog; ada **Sync ulang** dan info status/waktu sync terakhir.
5. Hasil sync dilaporkan per file (dibuat / diperbarui / dilewati / ditolak + alasan), memakai bentuk laporan upload T05/T06.

## Non-goals

- Push / commit balik ke git (satu arah: git → Grimoire).
- Sync otomatis (terjadwal, webhook, polling). Hanya manual lewat tombol.
- Menghapus file di Grimoire yang sudah tidak ada di git (bukan mirror).
- Riwayat commit, diff antar sync, rollback, pilih commit/tag spesifik (hanya branch).
- Submodule, Git LFS, repo non-GitHub/GitLab (Bitbucket, Gitea).
- OAuth app / GitHub App / deploy key. Hanya PAT.
- Banyak PAT per provider per user.

## Anti-tujuan (dianggap gagal walau fitur jalan)

- PAT muncul di respons API, log, audit, pesan error, atau URL, dalam bentuk apa pun selain "tersimpan" + 4 karakter terakhir.
- PAT satu user dipakai untuk sync yang dipicu user lain.
- URL yang diberikan user dipakai untuk menjangkau jaringan internal (SSRF) atau host di luar yang diizinkan.
- Sync menimpa/menghapus file di luar folder target atau di project lain.
- Sync separuh jalan meninggalkan project dalam keadaan setengah tertimpa tanpa laporan.

## Keputusan pemilik

| Topik | Keputusan |
|---|---|
| Arah | Tarik saja (git → Grimoire). |
| Provider | GitHub dan GitLab, **termasuk GitLab self-hosted** (host lewat allowlist config). |
| PAT | **Satu PAT per provider per user**, terenkripsi, per user (bukan per project). |
| Replace | **Timpa yang sama**, sisanya dibiarkan. |
| Akses | Anggota project memakai PAT miliknya sendiri; URL + branch diingat per project. |
| Filter file | Hanya ekstensi valid per tipe: knowledge `.md`/`.txt`, API spec `.yaml`/`.yml`/`.json`. Lainnya dilewati. Spec invalid dilaporkan per file seperti T06. |
| Metode ambil | REST API GitHub/GitLab (tarball atau tree + raw). Tanpa binary `git` di server. |
| Otomasi | Tidak ada; manual lewat tombol. |

## Keputusan pembangun (mohon dicek)

- K1. Dua module sesuai kepemilikan data: `core/git-credential` (PAT, milik user, lintas project) dan `project/git-sync` (konfigurasi sync + eksekusi). Interaksi lewat `infra/bus` (`core.gitcredential.get-token` dipanggil git-sync; PAT plaintext hanya lewat bus internal, tidak lewat REST). Bila pemilik ingin satu module, gabung ke `project/git-sync`; ini panggilan saya.
- K2. Enkripsi PAT: AES-256-GCM, kunci dari env baru `GIT_CREDENTIAL_KEY` (base64 32 byte, wajib bila fitur aktif; aplikasi gagal start/fitur nonaktif dengan pesan jelas bila kosong). Kolom menyimpan `nonce || ciphertext`. Tabel `git_credentials(user_id, provider, host, token_enc, last4, created_at, updated_at, unique(user_id, provider, host))`. Untuk GitLab self-hosted, satu PAT per `host`; GitHub selalu `github.com`. "Satu PAT per provider" dibaca sebagai satu per (provider, host).
- K3. Parsing URL (satu fungsi di `base`), menerima:
  - GitHub: `https://github.com/{owner}/{repo}/tree/{branch}/{folder...}` atau `https://github.com/{owner}/{repo}` (+ field branch/folder terpisah).
  - GitLab: `https://{host}/{group}/{subgroup...}/{repo}/-/tree/{branch}/{folder...}`.
  Dialog juga punya field **Branch** eksplisit yang menang atas branch di URL. Branch dengan `/` (mis. `feature/x`) ambigu di URL `tree/...`, sehingga field Branch eksplisit dipakai bila diisi; bila tidak, branch = segmen pertama setelah `tree`. Folder kosong = root repo.
- K4. Keamanan host (SSRF): hanya `https`; host harus `github.com`/`api.github.com`, `gitlab.com`, atau ada di env `GIT_SYNC_ALLOWED_HOSTS` (daftar host GitLab self-hosted). Setelah resolve DNS, tolak IP privat/loopback/link-local/metadata (cek di dial, bukan hanya di string). Redirect tidak diikuti ke host lain. Timeout total dan batas ukuran respons.
- K5. Pengambilan file: GitHub lewat `GET /repos/{o}/{r}/git/trees/{branch}?recursive=1` + blob/raw, atau tarball; GitLab lewat `GET /projects/{id}/repository/tree?path=&ref=&recursive=true` + `files/{path}/raw`. Pilih yang paling sedikit request dan hormati rate limit (respons 403/429 → error `RATE_LIMITED` dengan pesan jelas). Detail diputuskan saat plan.
- K6. Batas: pakai env yang sudah ada (`MAX_FILE_BYTES`, `APISPEC_MAX_FILE_BYTES`, `MAX_UPLOAD_FILES`, `MAX_UPLOAD_BYTES`, `MAX_PROJECT_FILES`, `APISPEC_MAX_SPECS`) sebagai batas sync. Melebihi batas jumlah/total → sync ditolak sebelum ada yang tertulis, dengan pesan jelas. File tunggal terlalu besar → `rejected` per file.
- K7. Penulisan memakai **usecase upload yang sudah ada** (lewat bus/antarmuka module knowledge dan api-spec), bukan menulis tabel mereka langsung, sehingga validasi, indeks full-text/endpoint, dan audit "timpa" tetap satu jalur. Tiket ini hanya menambah pintu masuk batch non-multipart yang dipanggil git-sync (kontrak bus baru di T05/T06). Apakah satu transaksi atau per file mengikuti perilaku upload T05/T06; laporan per file tetap.
- K8. Pemetaan nama/path: knowledge — path relatif terhadap folder target, ditambah prefix tujuan opsional (field "Simpan di folder" di Grimoire, default root) agar sync dari `docs/` tidak bentrok dengan folder lain; API spec — nama = nama file tanpa ekstensi (slug seperti T06), `source_path` = path di repo. Dua file di git yang menghasilkan nama sama → yang kedua `rejected` (`DUPLICATE_NAME`).
- K9. Konfigurasi tersimpan: tabel `git_sync_configs(project_id, kind('knowledge'|'api_spec'), provider, host, repo_url, branch, folder, target_prefix, last_status, last_synced_at, last_synced_by, last_result jsonb)`, `unique(project_id, kind)`. Tidak menyimpan PAT. Anggota lain melihat URL/branch tersimpan, tetapi saat sync memakai PAT **dirinya**; bila ia belum punya PAT untuk provider/host itu → `CREDENTIAL_MISSING` dengan tautan ke pengaturan PAT.
- K10. Sync berjalan sinkron dalam request dengan timeout, dan satu sync aktif per `(project, kind)` (advisory lock; kedua → 409 `SYNC_IN_PROGRESS`). Bila nanti terbukti lambat, pindah ke job latar belakang di tiket lain.
- K11. PAT divalidasi saat disimpan dengan satu panggilan ringan ke provider (mis. `GET /user`); gagal → tidak disimpan, pesan "token tidak valid / tidak punya akses". Tidak ada pengecekan scope repo spesifik.

## Constraints

- Struktur domain/module/feature yang ada. Interaksi antar module hanya lewat `infra/bus`.
- OpenAPI: modul baru di `api/modules/core/git-credential.yml` dan `api/modules/project/git-sync.yml`, lalu `make api-bundle` dan commit `api/openapi.yml`.
- Migrasi baru di `backend/migrations/core/` dan `backend/migrations/project/`.
- Teks UI bahasa Indonesia; kunci i18n di `frontend/src/shared/i18n/{id,en}.json`.
- Tidak menambah dependensi besar; HTTP client standar library dengan transport kustom (cek IP) dari `infra`.
- Variabel env baru didokumentasikan di `docs/configuration.md` dan `.env.example`.

## Features

| Folder | Deskripsi | Titik masuk | Siapa |
|---|---|---|---|
| `core/git-credential/savecredential` | Simpan/ganti PAT (validasi ke provider, enkripsi) | `PUT /api/me/git-credentials/{provider}` (body: `host`, `token`) | user login |
| `core/git-credential/listcredentials` | Status PAT milik sendiri (provider, host, `last4`, diperbarui) | `GET /api/me/git-credentials` | user login |
| `core/git-credential/deletecredential` | Hapus PAT | `DELETE /api/me/git-credentials/{provider}?host=` | user login |
| `core/git-credential/bustoken` | Handler bus: kembalikan PAT terdekripsi untuk `(userID, provider, host)` | bus `core.gitcredential.get-token` | internal |
| `project/git-sync/getconfig` | Konfigurasi + hasil sync terakhir per `kind` | `GET /api/projects/{id}/git-sync?kind=` | anggota, super_admin |
| `project/git-sync/previewsync` | (opsional, lihat Pertanyaan terbuka) parse URL dan daftar file yang akan disentuh tanpa menulis | `POST /api/projects/{id}/git-sync/preview` | anggota, super_admin |
| `project/git-sync/runsync` | Simpan config, ambil file, tulis lewat upload T05/T06, kembalikan laporan | `POST /api/projects/{id}/git-sync/run` (body: `kind`, `url`, `branch`, `folder`, `targetPrefix`) | anggota, super_admin |

## Aturan bisnis

- PAT dipakai hanya oleh pemiliknya. Sync yang dipicu user X memakai PAT user X; tidak ada fallback ke PAT user lain atau PAT bersama.
- Super Admin yang bukan anggota project tetap bisa sync dengan PAT miliknya sendiri.
- Replace: knowledge ditimpa per path (tidak berubah bila `content_hash` sama → status `unchanged`), API spec ditimpa per nama seperti T06. Tidak ada penghapusan.
- Sync tidak menyentuh file di luar `targetPrefix` (knowledge) atau spec lain yang tidak ada di folder git.
- Konten dari git diperlakukan sama dengan upload: validasi path (tanpa `..`, tanpa path absolut), UTF-8, batas ukuran, validasi OpenAPI, penolakan `$ref` eksternal.
- Setiap sync menulis `git_sync_configs` (status, waktu, ringkasan) walau sebagian file ditolak. Gagal total (auth, 404 repo/branch/folder, rate limit, melebihi batas) → status `failed` tanpa menulis file apa pun.

## Model data

```
core.git_credentials(user_id uuid, provider text, host text, token_enc bytea, last4 text,
  created_at, updated_at, unique(user_id, provider, host))

project.git_sync_configs(project_id uuid, kind text, provider text, host text, repo_url text,
  branch text, folder text, target_prefix text, last_status text, last_synced_at timestamptz,
  last_synced_by uuid, last_result jsonb, unique(project_id, kind))
```

## Kontrak bus

- `core.gitcredential.get-token` — `{UserID, Provider, Host}` → `{Token}` atau `ErrCredentialNotFound`. Hanya dipanggil git-sync.
- Kontrak batch-tulis di T05/T06 (nama final di plan), mis. `project.knowledge.upsert-files` dan `project.apispec.upsert-specs` — `{ProjectID, Actor, Items[]}` → laporan per item. Memakai logika `uploadfiles`/`uploadspecs` yang ada.

## UI (Svelte)

- **Pengaturan akun → "Token Git"**: kartu GitHub dan GitLab (host untuk self-hosted); status "Tersimpan (…abcd)", tombol simpan/ganti/hapus; teks: scope minimal (`repo` read / `read_api` + `read_repository`), token tidak bisa dilihat lagi.
- **Knowledge** dan **API Spec**: tombol **Sync dari Git** di header. Dialog: URL, Branch, Folder (terisi dari URL), (knowledge) "Simpan di folder"; ringkasan config terakhir, tombol **Sync ulang**; progres; hasil per file (dibuat/diperbarui/tidak berubah/dilewati/ditolak) dengan alasan yang bisa disalin.
- Bila PAT belum ada: dialog menampilkan ajakan ke pengaturan Token Git, tombol Sync nonaktif.
- Setelah sukses, daftar/pohon memuat ulang.

## Titik sentuh

Backend: `core/internal/gitcredential/*` (baru), `project/internal/gitsync/*` (baru), `infra` (HTTP client aman + enkripsi, bila belum ada), kontrak bus T05/T06, `migrations/core`, `migrations/project`, `docs/configuration.md`, `.env.example`.
Frontend: `domain/core/git-credential/...`, `domain/project/git-sync/...`, header Knowledge dan API Spec, i18n.
Lainnya: `api/modules/core/git-credential.yml`, `api/modules/project/git-sync.yml`, `api/openapi.yml`, `docs/tickets/README.md`.

## Aksi audit baru

- `git_credential.saved`, `git_credential.deleted` (provider, host; **tanpa** token/last4).
- `git_sync.run` (kind, repo, branch, folder, jumlah dibuat/diperbarui/tidak berubah/dilewati/ditolak, status). Tidak memuat PAT. Aksi tulis di knowledge/api-spec tetap tercatat oleh audit masing-masing.

## Risiko

- **Kebocoran PAT**: log request/error, respons error provider yang memantulkan header, atau serialisasi struct. Mitigasi: tipe `Secret` yang `String()`-nya disamarkan, tes yang memeriksa PAT tidak muncul di log/respons/audit.
- **SSRF** lewat URL/self-hosted host dan redirect. Mitigasi K4; wajib tes (IP privat, redirect ke localhost, DNS rebinding).
- **Kunci enkripsi hilang/berganti** membuat semua PAT tak terbaca. Mitigasi: dokumentasi; dekripsi gagal → `CREDENTIAL_UNREADABLE` dan user diminta simpan ulang, bukan 500.
- **Rate limit** provider dan repo besar. Mitigasi: batas jumlah/ukuran di K6, error `RATE_LIMITED`.
- **Menimpa hasil edit manual** di Grimoire (replace tanpa konfirmasi). Mitigasi: dialog menampilkan peringatan "file bernama sama akan ditimpa"; lihat Pertanyaan terbuka soal preview.
- Sync sinkron panjang bisa melewati timeout proxy (K10).

## Kriteria penerimaan

1. User menyimpan PAT GitHub valid → `GET /api/me/git-credentials` menampilkan `last4` saja; token tak pernah ada di respons mana pun. PAT tidak valid → ditolak, tidak tersimpan.
2. Hapus PAT → sync yang butuh provider itu menghasilkan `CREDENTIAL_MISSING`.
3. Sync knowledge dari URL `.../tree/main/docs` repo privat dengan PAT sah: semua `.md`/`.txt` di `docs/` rekursif masuk dengan struktur path dipertahankan; file lain dilewati; laporan per file benar.
4. Sync ulang dengan isi berubah: file yang namanya sama ditimpa (`updated`), yang sama persis `unchanged`; file di Grimoire yang tidak ada di git **tetap ada**.
5. Sync API spec: tiap `.yaml/.yml/.json` valid menjadi/menimpa spec bernama sama; spec invalid atau `$ref` eksternal → `rejected` dengan alasan, yang lain tetap masuk; indeks endpoint diganti sepenuhnya untuk spec yang ditimpa.
6. Branch eksplisit mengalahkan branch di URL; branch ber-`/` (`feature/x`) bekerja bila diisi eksplisit.
7. URL GitLab self-hosted: berhasil bila host ada di `GIT_SYNC_ALLOWED_HOSTS`, ditolak (`HOST_NOT_ALLOWED`) bila tidak. Host yang resolve ke IP privat/loopback ditolak, redirect ke host lain tidak diikuti.
8. Dua anggota: sync oleh A memakai PAT A; B tanpa PAT mendapat `CREDENTIAL_MISSING` walau config tersimpan dari A. PAT A tidak pernah dipakai untuk request B.
9. URL + branch + folder tersimpan; membuka dialog lagi terisi otomatis; **Sync ulang** menjalankan config yang sama. Status dan waktu sync terakhir tampil.
10. Melebihi batas jumlah/ukuran file → ditolak utuh sebelum menulis; repo/branch/folder tidak ada → 404 yang jelas; rate limit → `RATE_LIMITED`.
11. Dua sync serentak untuk `(project, kind)` yang sama → satu jalan, satu `SYNC_IN_PROGRESS`.
12. Non-anggota project: 404 (pola T03). Tanpa login: 401.
13. Audit mencatat `git_credential.saved/deleted` dan `git_sync.run` tanpa PAT; pencarian di log aplikasi dan audit tidak menemukan PAT uji.
14. Tes unit (parser URL, enkripsi, pemetaan nama/path, filter ekstensi), integrasi (replace, unchanged, tanpa hapus, batas, lock), tes SSRF/host, handler, e2e (simpan PAT, sync dengan server git palsu, hasil tampil), `make api-check`, lint, dan arch test hijau.

## Pertanyaan terbuka

1. **Preview sebelum sync**: perlu langkah "lihat file yang akan ditimpa" sebelum menulis, atau cukup peringatan di dialog + laporan sesudahnya? (Default tiket: hanya peringatan; `previewsync` opsional.)
2. **Lokasi PAT di UI**: halaman "Pengaturan akun" baru, atau bagian di menu profil yang sudah ada? Perlu dicek terhadap struktur FE saat plan.
3. **Folder tujuan knowledge**: apakah field "Simpan di folder" (prefix) cukup, atau sync selalu ke root?
