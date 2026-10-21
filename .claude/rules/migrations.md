---
paths:
  - "backend/migrations/**"
---

# Aturan file migrasi

- Lokasi: `backend/migrations/{core,project}/`. Domain `core` ke `core/`, domain `project` ke `project/`.
- Nama file: `YYYYMMDDHHMMSSmmm_<snake_case>.sql`, tepat 17 digit, divalidasi runner dengan `^\d{17}_[a-z0-9_]+\.sql$`.
- Versi (17 digit pertama) harus unik secara global, lintas `core` dan `project`. Duplikat ditolak runner.
- Timestamp WAJIB di-generate saat file dibuat (saat implement), bukan ditulis di plan atau ditebak. Perintah (macOS dan Linux):

  ```
  python3 -c "import datetime;print(datetime.datetime.now().strftime('%Y%m%d%H%M%S%f')[:17])"
  ```

  `date +%N` tidak tersedia di macOS, jangan dipakai.
- Bagian `HHMMSSmmm` harus angka waktu sebenarnya. DILARANG `000000000`, `090000000`, atau pola bulat/karangan lain. Bagian milidetik jangan diisi `000` kecuali memang hasil generate.
- Versi harus lebih besar dari migrasi yang dirujuk lewat FK (mis. `create_projects` milik T03). Karena di-generate saat implement, ini otomatis terpenuhi; cek ulang bila memindahkan atau me-rename file.
- Sebelum commit: `ls backend/migrations/*/ | cut -c1-17 | sort | uniq -d` harus kosong. Bila ada kembar, generate ulang file yang baru.
- Migrasi yang sudah ter-merge ke `main` tidak diubah isinya dan tidak di-rename. Perubahan skema = migrasi baru.
- Plan memakai placeholder `<VERSION>_<nama>.sql`; ganti dengan hasil generate saat membuat file, lalu perbarui semua rujukan nama file di kode dan dokumen.
- FK antar tabel milik module berbeda hanya ke `project_id`/`user_id` dengan `ON DELETE CASCADE` (lihat README tiket).
