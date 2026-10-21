# Roles dan permissions

Akses di Payments Platform diatur lewat role. Prinsipnya least privilege: beri akses seminimal yang dibutuhkan.

## Role

| Role | Kemampuan |
|---|---|
| `super_admin` | mengelola semua project, user, dan pengaturan sistem |
| `owner` | mengelola satu project dan anggotanya |
| `member` | membaca dan menulis konten project |
| `viewer` | hanya membaca |

## Aturan

- Hanya `super_admin` yang boleh membuat project baru.
- Minimal satu `owner` per project; sistem menolak menghapus owner terakhir.
- Perubahan role dicatat di audit log lengkap dengan siapa yang mengubah.

## Memeriksa permission di kode

```go
if !perm.Can(actor, "project.write", projectID) {
    return ErrForbidden
}
```

Jangan mengecek nama role langsung di handler. Selalu lewat permission supaya mudah diubah.

## Review akses

Tiap kuartal, owner meninjau daftar anggota dan mencabut akses yang tidak lagi dipakai.
