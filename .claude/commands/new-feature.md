---
description: Implementasi plan dari tiket (contoh /new-feature T01 atau /new-feature T01 merge)
argument-hint: <T01 | path plan> [merge] [catatan tambahan]
---

Jalankan workflow `new-feature` dengan tool Workflow untuk: $ARGUMENTS

Langkah:
1. Kata pertama argumen adalah ID tiket (mis. `T01`) atau path plan. Jika ID, turunkan `feature` dari `docs/tickets/<ID>-*.md` (nama file tanpa awalan ID dan `.md`), lalu `planPath` = `docs/superpowers/plans/<feature>/overview.md` bila ada, kalau tidak `docs/superpowers/plans/<feature>.md`. Jika plan belum ada, sarankan `/plan-from-usecase <ID>` dan berhenti.
2. Jika kata `merge` ada di argumen, set `targetBranch: 'main'` (squash-merge, tanpa push). Tanpa kata itu, tidak ada merge dan worktree dibiarkan untuk direview. Sisa argumen menjadi `notes`.
3. Panggil Workflow dengan `name: 'new-feature'` dan args `{planPath, targetBranch?, notes?}`.
4. Laporkan status akhir (DONE, EVAL_EXHAUSTED, atau NEEDS_HUMAN_INPUT), `worktreePath`/branch, dan alasan bila gagal.
5. Update Status tiket di `docs/tickets/README.md` (hanya jika argumen berupa ID tiket):
   - hasil `DONE` dengan `mergedToBranch` terisi: `Selesai (<sha>)`, dengan `<sha>` dari `git rev-parse --short HEAD` di branch tujuan setelah merge.
   - hasil `DONE` tanpa merge: `Siap review (feat/<slug>)`.
   - `EVAL_EXHAUSTED` atau `NEEDS_HUMAN_INPUT`: jangan ubah status.
6. Jika hasil workflow `NEEDS_HUMAN_INPUT` dan child workflow tampak memakai perilaku lama (script sudah diedit tapi hasilnya tidak berubah), jalankan `implement-feature` / `test` lewat `scriptPath` (`.claude/workflows/<nama>.js`) dan bukan lewat nama.

Nilai kolom Status di `docs/tickets/README.md` (baris tiket yang bersangkutan, hanya sel Status): `Belum` -> `Plan siap` -> `Siap review (feat/<slug>)` -> `Selesai (<sha>)`. Ubah hanya dengan Edit pada satu sel itu. Jangan git commit, dan beri tahu pengguna bahwa perubahan README belum di-commit.
