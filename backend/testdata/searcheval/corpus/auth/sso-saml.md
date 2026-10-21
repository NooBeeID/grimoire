# SSO dengan SAML

Pelanggan enterprise bisa login lewat SSO memakai SAML 2.0. Dokumen ini untuk engineer yang membantu onboarding pelanggan.

## Istilah

- IdP: identity provider milik pelanggan, biasanya Okta atau Azure AD.
- SP: Payments Platform sebagai service provider.

## Langkah setup dengan Okta

1. Pelanggan membuat aplikasi SAML 2.0 baru di Okta.
2. Isi ACS URL dan Entity ID dari halaman pengaturan SSO kami.
3. Pelanggan mengunduh metadata XML dari IdP dan mengunggahnya ke dashboard.
4. Kami memverifikasi sertifikat di metadata dan mengaktifkan koneksi.

## Attribute mapping

| Atribut SAML | Field kami |
|---|---|
| `email` | email user |
| `displayName` | nama |
| `groups` | role (lihat roles-permissions) |

## Catatan

- Clock skew maksimum 2 menit; periksa NTP bila assertion ditolak.
- Sertifikat IdP perlu diperbarui sebelum kedaluwarsa, minta pelanggan mengunggah metadata baru.
- Login lewat SSO tidak memakai password lokal.
