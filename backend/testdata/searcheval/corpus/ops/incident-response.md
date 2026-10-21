# Incident response

Panduan saat terjadi gangguan di production.

## Severity

| Level | Contoh | Respons |
|---|---|---|
| SEV1 | payment gagal untuk semua merchant | semua on-call, update tiap 15 menit |
| SEV2 | satu fitur penting turun | on-call primer, update tiap 30 menit |
| SEV3 | gangguan kecil | ditangani jam kerja |

## Peran

- Incident commander: mengoordinasi dan memutuskan.
- Ops lead: melakukan mitigasi teknis.
- Comms: memperbarui status page dan pelanggan.

## Langkah

1. Buka channel #incident-YYYYMMDD dan tentukan severity.
2. Mitigasi dulu (rollback, matikan fitur, scale up), analisis akar masalah kemudian.
3. Catat timeline di dokumen incident.
4. Setelah pulih, umumkan dan tutup channel.

## On-call

Rotasi on-call berganti tiap Senin pukul 10:00. Primer dan sekunder selalu ada. Serah terima dilakukan lewat catatan singkat di Slack.

## Postmortem

Postmortem blameless wajib untuk SEV1 dan SEV2 dalam 5 hari kerja: apa yang terjadi, dampak, akar masalah, dan tindakan perbaikan beserta pemilik dan tenggat.
