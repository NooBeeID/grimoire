---
description: Buat plan dari tiket di docs/tickets (contoh /plan-from-usecase T01)
argument-hint: <T01 | path tiket> [catatan tambahan]
---

Jalankan workflow `plan-from-usecase` dengan tool Workflow untuk tiket: $ARGUMENTS

Langkah:
1. Kata pertama argumen adalah ID tiket (mis. `T01`) atau path file. Jika ID, cari `docs/tickets/<ID>-*.md`. Sisa argumen (jika ada) menjadi `notes`. Jika tiket tidak ditemukan atau argumen kosong, tampilkan daftar di `docs/tickets/README.md` dan berhenti.
2. Baca tiket. Dari tabel di atasnya ambil `Domain / Module` (mis. "core / auth") untuk `domain` dan `module`. Untuk T00 (infra / foundation) pakai domain `infra`, module `foundation`.
3. `feature` = nama file tanpa awalan ID dan `.md` (mis. `T01-core-auth.md` menjadi `core-auth`).
4. Panggil Workflow dengan `name: 'plan-from-usecase'` dan args `{ticket: <path>, feature, domain, module, scope: 'fullstack', notes}`. Tiket proyek ini mencakup backend (Go) dan UI (Svelte), jadi scope default `fullstack`. Pakai `backend` atau `frontend` hanya jika tiketnya jelas hanya satu sisi.
5. Setelah selesai, laporkan `planPath`, daftar usecase (dan mana yang `ok:false`), serta status. Jika `NEEDS_HUMAN_INPUT`, sampaikan alasannya.
6. Jika status workflow `DONE`, ubah Status tiket itu di `docs/tickets/README.md` menjadi `Plan siap`. Jika bukan `DONE`, jangan ubah status.

Nilai kolom Status di `docs/tickets/README.md` (baris tiket yang bersangkutan, hanya sel Status): `Belum` -> `Plan siap` -> `Siap review (feat/<slug>)` -> `Selesai (<sha>)`. Ubah hanya dengan Edit pada satu sel itu. Jangan git commit, dan beri tahu pengguna bahwa perubahan README belum di-commit.
