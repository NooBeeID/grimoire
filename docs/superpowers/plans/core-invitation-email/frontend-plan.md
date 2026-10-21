# T12 core-invitation-email: Frontend Plan (Svelte)

Tiket: `docs/tickets/T12-core-invitation-email.md`.
Kontrak: `overview.md` (bagian 1, 3, 11) dan `usecase-*.md` di folder ini. Bila plan ini bertentangan dengan `overview.md`, `overview.md` menang.
Konteks yang dibaca: `DESIGN.md`, `.claude/skills/design-taste-frontend/references/app-screens.md`, `docs/tickets/README.md` (bagian Frontend), `.claude/rules/migrations.md` (satu-satunya rule). Tidak ada `CLAUDE.md`.
Kode yang dicek (main `4b75884`): `domain/project/management/members/*`, `domain/core/auth/{users,getme,login,changepassword,logout}/*`, `shared/components/{Dialog,CopyField,Table,EmptyState}.svelte`, `shared/services/http.ts`, `shared/validation/error.ts`, `shared/stores/session.ts`, `app/routes/index.ts`, `app/views/{UsersView.svelte,workspace/MembersTab.svelte}`, `.dependency-cruiser.cjs`, `e2e/helpers/auth.ts`, `playwright.config.ts`.

Satu plan untuk seluruh UI T12. Tidak dipecah per usecase.

---

## 1. Metadata

| Atribut | Nilai |
|---|---|
| Ticket | T12 |
| Feature | core-invitation-email |
| Scope | fullstack; dokumen ini hanya frontend |
| Domain / Module | core / auth (halaman Users, login, ganti password, getme) dan project / management (tab Anggota) |
| Feature FE yang diubah | `frontend/src/domain/project/management/members`, `frontend/src/domain/core/auth/{users,getme,login,changepassword,logout}` |
| Shared baru | `shared/components/TemporaryCredentialsDialog.svelte`, `shared/components/EmailDeliveryNote.svelte`, `shared/stores/mailStatus.ts`, `shared/validation/credentials.ts` |
| Route baru | tidak ada; route yang disentuh: `/projects/:projectID/members`, `/admin/users`, `/login`, `/change-password` |

Endpoint yang dipakai FE:

| Endpoint | Method | Version | Dipakai oleh | Status |
|---|---|---|---|---|
| `/api/users/invite` | POST | tanpa prefix versi (minor `info.version` auth.yml) | Users: dialog "Undang developer" | baru |
| `/api/users/{id}/resend-invitation` | POST | sama | Users: aksi "Kirim ulang undangan" | baru |
| `/api/users`, `/api/users/{id}` | GET | aditif `invitation` | Users: daftar dan dialog ubah | diubah |
| `/api/auth/me` | GET | aditif `email_enabled` | bootstrap sesi, `mailStatus` | diubah |
| `/api/projects/{projectID}/invitations` | POST | minor management.yml | Members: dialog Tambah anggota mode undang | baru |
| `/api/projects/{projectID}/invitations` | GET | sama | Members: section "Belum menerima undangan" | baru |
| `/api/projects/{projectID}/invitations/{userID}/resend` | POST | sama | Members: aksi Kirim ulang | baru |
| `/api/projects/{projectID}/invitations/{userID}` | DELETE | sama | Members: aksi Batalkan | baru |
| `/api/projects/{projectID}/members` | GET | aditif `invitation_pending`, `invitation_expires_at` | Members: tabel anggota | diubah |
| `/api/projects/{projectID}/member-candidates` | GET | aditif `invitation_pending` | Members: combobox kandidat | diubah |
| `/api/auth/login`, `/api/auth/change-password` | POST | perilaku saja, bentuk tetap | teks bantu saja | tidak berubah bentuk |

Semua mutasi memakai `shared/services/http.ts` (header `X-Requested-With: grimoire` otomatis). Tidak ada endpoint bus di FE.

---

## 2. Summary Plan

T12 di UI berarti empat hal:

1. **Tab Anggota project.** Dialog "Tambah anggota" yang sudah ada mendapat jalur kedua: bila teks pencarian berbentuk email dan tidak ada kandidat dengan email itu, daftar kandidat menampilkan opsi "Undang `<email>` lewat email". Memilihnya mengubah dialog ke mode undang (email terkunci, nama opsional, peran, catatan pengiriman email). Di atas tabel anggota, owner/super_admin melihat section "Belum menerima undangan" (hanya bila ada isinya) dengan aksi Kirim ulang dan Batalkan (keduanya lewat konfirmasi). Tabel anggota menandai baris undangan menunggu.
2. **Halaman Users.** Tombol "Undang developer" (aksi utama baru di title row; "Buat user" turun jadi sekunder), penanda undangan di kolom Status, dan aksi baris "Kirim ulang undangan" yang menggantikan "Reset password" pada akun undangan menunggu.
3. **Dialog "Kredensial sementara"** (shared, sekali tampil, tidak bisa ditutup dengan klik latar atau Escape) dipakai empat jalur: undang dari project, undang dari Users, kirim ulang dari project, kirim ulang dari Users. Hanya muncul bila respons membawa `credentials`.
4. **Teks bantu** di halaman login dan ganti password paksa, plus `email_enabled` dari `GET /api/auth/me` yang disimpan di store `mailStatus` agar dialog undang bisa menjelaskan apa yang akan terjadi sebelum dikirim.

Keputusan inti:

- Fitur undangan ditaruh di feature yang sudah memiliki layarnya (`members`, `users`), bukan feature FE baru, karena UI-nya menempel di dialog dan tabel yang sama dan komposisi lintas feature akan memaksa state undangan naik ke `app/`. Ini sama dengan pola T03 (`members` membungkus list/add/remove/role).
- Kredensial sementara hanya hidup di state komponen dialog. Tidak masuk store, tidak di-log, dihapus saat dialog ditutup. Halaman tidak punya cara membacanya lagi (tidak ada endpoint baca).
- Semua data undangan setelah mutasi diambil ulang dari server (refresh diam), bukan dihitung di klien, karena `delivery_status`, `sent_at`, `expires_at` milik auth.

---

## 3. Plan (langkah berurutan)

Singkatan: `MB` = `frontend/src/domain/project/management/members`, `US` = `frontend/src/domain/core/auth/users`, `GM` = `frontend/src/domain/core/auth/getme`, `SH` = `frontend/src/shared`.

TDD per langkah: tulis tes Vitest dulu, lalu implementasi. Setelah tiap langkah: `npm run check && npm run test -- <folder>`.

### Langkah 0. Prasyarat

- Kontrak OpenAPI final untuk auth (akhir gelombang B) dan management (akhir gelombang C) sudah di `api/openapi.yml` (overview bagian 9 langkah 5). Skema Zod di plan ini disalin dari contoh di overview 11 dan usecase 8; bila OpenAPI final berbeda, OpenAPI menang dan plan ini dikoreksi di PR yang sama.
- FE slicing boleh mulai lebih awal dengan mock (`MB/mocks/invitations.ts`, `US/mocks/invitations.ts`) karena bentuk respons sudah dikunci di plan usecase.

### Langkah 1. Shared (generik, tanpa pengetahuan domain)

1. `SH/validation/credentials.ts` (baru):
   ```ts
   // Mengikuti components/schemas/InvitationCredentials di api/modules/core/auth.yml.
   export const TemporaryCredentialsSchema = z.object({
     email: z.string().min(1),
     temporary_password: z.string().min(1),
     login_url: z.string().min(1), // absolut, atau "/login" bila PUBLIC_URL kosong (overview 0.1)
   });
   export type TemporaryCredentials = z.infer<typeof TemporaryCredentialsSchema>;
   export const EmailStatusSchema = z.enum(['sent', 'failed', 'not_configured', 'pending']);
   export type EmailStatus = z.infer<typeof EmailStatusSchema>;
   export const RateLimitScopeSchema = z.object({ retry_after_seconds: z.number().int().nonnegative(), scope: z.enum(['invite', 'resend']) });
   export function rateLimitOf(details: unknown): { seconds: number; scope: 'invite' | 'resend' } | null
   ```
   Alasan di `shared/validation`: skema yang sama dirujuk dua module (management me-`$ref` skema auth, usecase management-inviteuser M7). Isinya bentuk kawat murni, tanpa logika domain.
2. `SH/utils/url.ts` (ubah): tambah `absoluteUrl(url: string, origin: string): string`. URL absolut dikembalikan apa adanya; path relatif (`/login`) digabung dengan `origin`; skema selain `http:`/`https:` dikembalikan sebagai `origin + '/login'` (pertahanan terhadap `javascript:`).
3. `SH/stores/mailStatus.ts` (baru):
   ```ts
   // null = belum diketahui (mis. tepat setelah login; respons login tidak membawa email_enabled).
   export const mailStatus: Readable<boolean | null>;
   export function setMailStatus(v: boolean | null): void;
   export function resetMailStatus(): void;
   ```
4. `SH/components/EmailDeliveryNote.svelte` (baru): props `{ emailEnabled: boolean | null }`. Satu paragraf 13px `text-muted` dengan teks `invitation.note.enabled | disabled | unknown`. Tanpa ikon, tanpa kotak berwarna.
5. `SH/components/TemporaryCredentialsDialog.svelte` (baru):
   ```ts
   let { open = $bindable(false), credentials, emailStatus, expiresAt, onclose }: {
     open?: boolean;
     credentials: TemporaryCredentials | null;
     emailStatus: 'failed' | 'not_configured';
     expiresAt: string; // ISO
     onclose: () => void;
   } = $props();
   ```
   - Memakai `Dialog` dengan `dismissible={false}` (latar dan Escape tidak menutup; tombol X nonaktif seperti perilaku `Dialog` yang ada) dan `size="md"`. Satu-satunya jalan keluar: tombol utama "Saya sudah menyalin".
   - Isi berurutan: kalimat sebab (`invitation.credentials.reason.failed` atau `.not_configured`, menyebut email), kotak peringatan sekali tampil (pola `KeyResult.svelte`: `rounded-base border border-warning px-3 py-2 text-sm font-medium text-warning`, `role="alert"`), `CopyField` Email, `CopyField` Password sementara (`testid="temporary-password"`), `CopyField` URL login (`absoluteUrl(login_url, window.location.origin)`), satu baris "Berlaku sampai {tanggal}" (`formatDateTime`), lalu `CopyField multiline rows={5}` "Pesan untuk penerima" berisi teks siap tempel (`invitation.credentials.messageBody`).
   - `onclose` dipanggil sekali; pemanggil wajib mengosongkan `credentials` (state di pemanggil di-null-kan). Komponen tidak menyimpan salinan di modul.
6. `SH/components/index.ts`: ekspor dua komponen baru.
7. `SH/i18n/{id,en}.json`: kunci namespace `invitation.*` (bagian 3.9).

### Langkah 2. `GM` (getme): `email_enabled`

1. `GM/types.ts`: `MeSchema` tambah `email_enabled: z.boolean()`. `toIdentity` tidak berubah (Identity tetap data user; status mail bukan atribut user).
2. `GM/services/getme.service.ts`: tambah `getMe(signal?): Promise<Me>` (respons mentah tervalidasi). `fetchMe()` yang ada menjadi `getMe().then(toIdentity)` (signature tetap, pemanggil lama tidak berubah).
3. `GM/bootstrap.ts`: `BootstrapDeps` tambah `setMailStatus`; `loadSession` memanggil `getMe`, lalu `setIdentity(toIdentity(m))` dan `setMailStatus(m.email_enabled)`. 401 -> `setIdentity(null)`, `setMailStatus(null)`.
4. `GM/emailStatus.ts` (baru): `refreshEmailStatus(deps = { getMe, setMailStatus }): Promise<void>`. Memanggil `getMe` sekali; sukses -> `setMailStatus(m.email_enabled)`; gagal apa pun -> diam (status tetap `null`, UI memakai teks netral). Dedup: satu promise berjalan dipakai bersama.
5. `GM/index.ts`: ekspor `refreshEmailStatus`.
6. `domain/core/auth/logout/logout.ts`: panggil `resetMailStatus()` di samping `resetSession()`.

### Langkah 3. `MB` (tab Anggota): tipe, service, store

1. `MB/types.ts` (ubah):
   - `MemberSchema` tambah `invitation_pending: z.boolean()`, `invitation_expires_at: z.string().datetime({ offset: true }).nullable()`.
   - `CandidateSchema` tambah `invitation_pending: z.boolean()`.
   - Baru:
     ```ts
     export const InvitationSchema = z.object({
       user_id: z.string().uuid(),
       email: z.string(),
       name: z.string(),
       role: z.enum(MEMBER_ROLES),
       invited_by_label: z.string().nullable(),
       added_at: z.string().datetime({ offset: true }),
       sent_at: z.string().datetime({ offset: true }).nullable(),
       expires_at: z.string().datetime({ offset: true }),
       expired: z.boolean(),
       delivery_status: EmailStatusSchema,
       email_failed: z.boolean(),
       resend_count: z.number().int().nonnegative(),
     });
     export const InvitationListSchema = z.object({ items: z.array(InvitationSchema) });
     export const InviteResponseSchema = z.object({
       member: MemberSchema,
       created: z.boolean(),
       email_sent: z.boolean(),
       email_status: EmailStatusSchema,
       expires_at: z.string().datetime({ offset: true }),
       credentials: TemporaryCredentialsSchema.optional(),
     });
     export const ResendResponseSchema = InviteResponseSchema.pick({ email_sent: true, email_status: true, expires_at: true, credentials: true });
     export type Invitation = z.infer<typeof InvitationSchema>;
     export type InviteResponse = z.infer<typeof InviteResponseSchema>;
     export type ResendResponse = z.infer<typeof ResendResponseSchema>;
     export interface InviteInput { email: string; role: MemberRole; name: string }
     export const MAX_EMAIL = 254;
     export const MAX_NAME = 100;
     ```
   - `KNOWN_REASONS` tambah `'INVITATION_NOT_PENDING'`.
2. `MB/model/invite.ts` (baru, murni):
   ```ts
   export function normalizeEmail(q: string): string           // trim + lowercase
   export function isEmailLike(q: string): boolean             // 3..254 karakter, tepat satu '@', tanpa spasi, ada '.' setelah '@'
   // Opsi undang di combobox: 'invite' | 'member' (email itu sudah anggota) | null.
   export function inviteOptionFor(q: string, search: CandidateState, members: Member[]): 'invite' | 'member' | null
   export function invitationStatus(i: Pick<Invitation, 'expired' | 'delivery_status'>): 'expired' | 'failed' | 'not_configured' | 'pending' | 'sent'
   export function sortInvitations(items: Invitation[]): Invitation[] // added_at menurun, tie-break user_id naik (sama dengan backend)
   ```
   Aturan `inviteOptionFor`: `null` bila `!isEmailLike(q)` atau `search.status !== 'ready'`; `'member'` bila ada `members[i].email` sama dengan `normalizeEmail(q)`; `null` bila ada kandidat dengan email sama persis (dia dipilih lewat jalur biasa); selain itu `'invite'`. Jadi opsi juga muncul bila ada kandidat lain yang mirip tetapi tidak sama persis (lihat bagian 10 keputusan F3).
   Aturan `invitationStatus` (satu status per baris, prioritas): `expired` > `failed` > `not_configured` > `pending` > `sent`.
3. `MB/services/invitations.service.ts` (baru, tanpa state):
   ```ts
   export async function listInvitations(projectId: string, signal?: AbortSignal): Promise<Invitation[]>   // GET .../invitations
   export function inviteToProject(projectId: string, input: InviteInput): Promise<InviteResponse>        // POST .../invitations
   export function resendProjectInvitation(projectId: string, userId: string): Promise<ResendResponse>    // POST .../invitations/{userID}/resend
   export async function revokeInvitation(projectId: string, userId: string): Promise<void>               // DELETE .../invitations/{userID}
   export type InviteErrorView =
     | { kind: 'fields'; fields: Partial<Record<'email' | 'role' | 'name', string>> }
     | { kind: 'alreadyMember' }
     | { kind: 'alreadyRegistered'; reason: 'registered' | 'inactive' | 'not_developer' }
     | { kind: 'notPending' }
     | { kind: 'rateLimited'; scope: 'invite' | 'resend'; minutes: number }
     | { kind: 'lastOwner' }
     | { kind: 'forbidden' } | { kind: 'notFound' }
     | { kind: 'generic'; message: string; traceId?: string };
   export function toInviteError(e: unknown): InviteErrorView
   ```
   - Body invite: `{ email: normalizeEmail(input.email), role: input.role }` plus `name` hanya bila `input.name.trim() !== ''` (handler menolak field tak dikenal, bukan field kosong; tetap dikirim minimal agar tes kontrak mudah).
   - `encodeURIComponent` pada `projectId` dan `userId` (pola `members.service.ts`).
   - Pemetaan error: 400 -> `fieldErrorsOf`; 409 `CONFLICT` + reason `ALREADY_MEMBER` -> `alreadyMember`; 409 kode `ALREADY_REGISTERED` + `reasonOf` -> `alreadyRegistered` (reason tak dikenal -> `registered`); 409 reason `INVITATION_NOT_PENDING` -> `notPending`; 409 reason `LAST_OWNER` -> `lastOwner`; 429 -> `rateLimitOf` (menit = `max(1, ceil(s/60))`; tanpa details -> scope dari pemanggil, menit 60); 403 -> `forbidden`; 404 -> `notFound`; lainnya `generic` dengan `traceId`.
4. `MB/stores/invitationsList.store.ts` (baru): pola `membersList.store.ts`.
   ```ts
   interface InvitationsState { status: 'idle' | 'loading' | 'ready' | 'error'; items: Invitation[]; error: ApiError | null; refreshing: boolean }
   begin() / refresh() (refreshing=true, item lama tetap) / resolve(items) (sortInvitations) / fail(e) / remove(userId) / clear()
   ```
5. `MB/stores/candidateSearch.store.ts` (ubah): `CandidateState` tambah `inviteOption: 'invite' | 'member' | null`. Method baru `setInviteOption(v)`. `move(delta)` berputar pada `items.length + (inviteOption === 'invite' ? 1 : 0)`; indeks terakhir = opsi undang. `setQuery`/`clear` me-reset `inviteOption=null`.

### Langkah 4. `MB` komponen

1. `MB/components/CandidateCombobox.svelte` (ubah):
   - Prop baru `oninvite: (email: string) => void`.
   - Setiap kandidat dengan `invitation_pending` menampilkan teks redup `project.members.add.candidatePending` setelah email (bukan pil; ini metadata, bukan status baris tabel).
   - Bila `search.inviteOption === 'invite'`: `li role="option"` terakhir berisi teks `project.members.invite.option` dengan email di `<span class="break-all font-medium">`; `id=optionId(items.length)`; `aria-selected` mengikuti `activeIndex`; mousedown/Enter memanggil `oninvite(normalizeEmail(search.q))`. Bila daftar kandidat kosong, baris "Tidak ada developer aktif yang cocok" tetap tampil di atas opsi ini.
   - Bila `inviteOption === 'member'`: baris `role="presentation"` redup `project.members.invite.alreadyMember`.
   - Bila query berbentuk email tetapi pencarian gagal (`status==='error'`): tidak ada opsi undang (hindari mengundang orang yang sebenarnya ada); pesan error yang ada tetap.
   - Pengumuman `aria-live` menambahkan "atau undang lewat email" saat opsi ada.
2. `MB/components/InviteByEmailForm.svelte` (baru): presentasi mode undang.
   - Props: `{ email, role = $bindable(), name = $bindable(), emailEnabled, submitting, error: InviteErrorView | null, onsubmit, onback, onsearchinstead }`.
   - Isi: baris email terkunci (pola "selected-candidate" yang ada: `rounded-base border border-border bg-surface px-3 py-2`, `data-testid="invite-email"`) dengan tombol teks "Ganti" (`onback`); `Input` Nama (opsional, `maxlength=100`, hint `project.members.invite.nameHint`); fieldset Peran (radio member/owner, salinan markup `AddMemberForm`); `EmailDeliveryNote`; pesan error; footer Batal (sekunder) + "Kirim undangan" (primary, `loading={submitting}`).
   - Pesan error per `kind`: `fields` -> di bawah field terkait (email ditampilkan di bawah baris email); `alreadyMember` -> `project.error.ALREADY_MEMBER`; `alreadyRegistered.registered` -> teks `invitation.error.alreadyRegistered.registered` + tombol ghost "Cari developer" (`onsearchinstead`, kembali ke mode cari dengan query = email); `inactive` / `not_developer` -> teks masing-masing; `rateLimited` -> teks dengan menit; `generic` -> pesan + ID jejak.
3. `MB/components/AddMemberForm.svelte` (ubah): tidak berubah perilakunya untuk mode cari; meneruskan `oninvite` ke combobox.
4. `MB/components/InvitationsTable.svelte` (baru): presentasi.
   - Props: `{ items, busyUserId, onrequestresend, onrequestrevoke }`.
   - Kolom: Undangan (Avatar bulat 28 + nama, baris kedua 12px `text-muted` email `break-all`), Peran (pil sama dengan `MembersTable`), Status (`InvitationStatusPill`), Dikirim (`<time>` `formatDateTime(sent_at)` atau `–` abu bila null; baris kedua 12px redup `project.invitations.resendCount` bila `resend_count > 0`), Berlaku sampai (`formatDateTime(expires_at)`), Diundang oleh (`invited_by_label` atau `–`), Aksi (ghost `Kirim ulang`, ghost `Batalkan`, `ariaLabel` menyebut email).
   - Kolom "Diundang oleh" dibuang bila kosong di semua baris (aturan DESIGN).
   - Dibungkus `overflow-x-auto` seperti `MembersTable` (bagian 9.2 catatan ponsel).
5. `MB/components/InvitationStatusPill.svelte` (baru): satu pil per baris dari `invitationStatus()`:

   | Status | Pil | Teks (id) |
   |---|---|---|
   | `expired` | `bg-pill-warning-bg text-pill-warning-ink` | Kedaluwarsa |
   | `failed` | `bg-pill-negative-bg text-pill-negative-ink` | Email gagal terkirim |
   | `not_configured` | `bg-pill-neutral-bg text-pill-neutral-ink` | Kirim manual |
   | `pending` | `bg-pill-neutral-bg text-pill-neutral-ink` | Belum terkirim |
   | `sent` | `bg-pill-neutral-bg text-pill-neutral-ink` | Menunggu login |

   Kelas pil identik dengan pil di `UsersTable` (`inline-flex h-5 flex-none items-center rounded-full px-2 text-xs font-medium whitespace-nowrap`). Warna hanya pada `expired` dan `failed` (baris yang perlu tindakan).
6. `MB/components/InvitationsSection.svelte` (baru): bingkai section.
   - Header `h3` 16px `project.invitations.title` + jumlah redup (dari `items.length`), indikator `refreshing` redup.
   - `status==='error'` -> `ErrorState` kompak di dalam bingkai section dengan Coba lagi (gagal memuat undangan tidak menutupi tabel anggota).
   - `status==='ready' && items.length===0` -> section tidak dirender sama sekali (bagian 10 F2).
   - `loading` pertama -> tidak dirender (tidak ada spinner kedua di atas tabel anggota; tabel anggota sudah punya spinner). Bila `refresh`, item lama tetap tampil.
7. `MB/components/MembersTable.svelte` (ubah):
   - `hasStatus` juga true bila ada `invitation_pending`.
   - Sel status: `invitation_pending` -> pil netral `project.members.status.invited`, atau pil warning `project.members.status.invitedExpired` bila `invitation_expires_at <= now` (now dari prop `now: Date`, default `new Date()` per render, agar tes deterministik). Prioritas status yang ada (`!found`, `!is_active`) tetap di atas penanda undangan. Satu penanda per baris.
   - Aksi baris tidak berubah (Jadikan owner/member, Keluarkan). Kirim ulang dan Batalkan hanya di section undangan, agar baris anggota tetap dua aksi (aturan DESIGN kebab).

### Langkah 5. `MB/views/MembersView.svelte` (ubah)

Props baru: `emailEnabled: boolean | null`.

State baru (semua `$state` lokal view, bukan store global):

```ts
const invitations = createInvitationsListStore();
let addMode = $state<'search' | 'invite'>('search');
let inviteEmail = $state('');
let inviteName = $state('');
let inviteError = $state<InviteErrorView | null>(null);
let inviting = $state(false);
let creds = $state<{ credentials: TemporaryCredentials; emailStatus: 'failed' | 'not_configured'; expiresAt: string } | null>(null);
let invConfirm = $state<{ kind: 'resend' | 'revoke'; item: Invitation } | null>(null);
let invConfirmSubmitting = $state(false);
let invConfirmError = $state<string | null>(null);
let invBusyUserId = $state<string | null>(null);
```

Alur:

1. **Muat.** `load(projectId)` yang ada tetap. Bila `canManage`, `loadInvitations(projectId)` dipanggil paralel dengan `AbortController` sendiri. 404 dari invitations -> `onprojectgone()` (sama dengan members); 403 -> `invitations.clear()` dan `onaccesschanged()` (akses turun saat tab terbuka); error lain -> `invitations.fail`. Member biasa tidak pernah memanggil endpoint ini.
2. **Pencarian.** Setelah `search.resolve(...)`, hitung `search.setInviteOption(inviteOptionFor(q, $search, $list.items))`. Bila query berubah, `setQuery` me-reset.
3. **Masuk mode undang.** `oninvite(email)`: `inviteEmail=email`, `inviteName=''`, `addRole` tetap (pilihan peran dibawa), `inviteError=null`, `addMode='invite'`, `resetSearch()`. Fokus ke field Nama.
4. **Kembali.** `onback` -> `addMode='search'`, query dikosongkan. `onsearchinstead` -> `addMode='search'` dan combobox diisi `inviteEmail` lalu `oninput(inviteEmail)` dijalankan (pencarian ulang).
5. **Kirim undangan.** `submitInvite()`: guard `inviting`; `inviteToProject(projectId, { email, role: addRole, name })`.
   - Sukses: `list.upsert(res.member)`; `void refreshInvitations()`; tutup dialog tambah.
     - `email_sent=true`: toast sukses `project.members.invite.success.sent` (atau `.resent` bila `created=false`).
     - `email_sent=false` dan `credentials` ada: buka `TemporaryCredentialsDialog` dengan `creds = { credentials, emailStatus: email_status, expiresAt: expires_at }`. Tidak ada toast (dialog sudah memberi tahu).
     - `email_sent=false` tanpa `credentials` (langkah terbit gagal total, overview 3.1): toast `type: 'error'` `project.members.invite.deliveryFailed` (anggota sudah tercatat; arahkan ke Kirim ulang).
   - Error: `toInviteError`. `forbidden` -> perilaku `handleError` yang ada (tutup dialog, toast, `onaccesschanged`). `notFound` -> `recheckAfter404`. `alreadyMember` -> pesan inline + `load(projectId)`. Lainnya -> `inviteError` inline, dialog tetap terbuka.
6. **Kirim ulang** (`onrequestresend(item)`): buka `ConfirmActionDialog` (komponen yang ada) dengan judul `project.invitations.resend.title`, pesan `project.invitations.resend.message` (menyebut email; password lama langsung tidak berlaku), tombol varian `primary`. Konfirmasi -> `resendProjectInvitation`.
   - `email_sent=true`: tutup, toast `project.invitations.resend.success`, refresh.
   - fallback `credentials`: tutup konfirmasi, buka `TemporaryCredentialsDialog`, refresh.
   - Error: `rateLimited` -> pesan inline di dialog konfirmasi (menit); `notPending` -> tutup, toast `invitation.error.notPending`, refresh undangan dan anggota; `alreadyRegistered.inactive` -> inline `invitation.error.alreadyRegistered.inactive`; `forbidden`/`notFound` -> jalur `handleError`/`recheckAfter404` yang ada (404 "anggota tidak ditemukan" dan "project tidak ditemukan" dibedakan oleh `recheckAfter404`).
7. **Batalkan** (`onrequestrevoke(item)`): `ConfirmActionDialog` varian `danger`, judul `project.invitations.revoke.title`, pesan `project.invitations.revoke.message` (akses hilang, akun tetap ada sampai dibersihkan otomatis, Super Admin bisa menonaktifkan; sesuai catatan UI di `usecase-management-revokeinvitation.md` bagian 9). Konfirmasi -> `revokeInvitation`.
   - 204: `invitations.remove(userId)`, `list.remove(userId)`, toast `project.invitations.revoke.success`.
   - `notPending` -> inline `project.invitations.revoke.notPending` + refresh kedua daftar (dua tab membatalkan bersamaan, E2E usecase revoke positif 5).
   - `lastOwner` -> inline `project.error.LAST_OWNER`.
   - `forbidden`/`notFound` -> jalur yang ada.
8. **Tutup kredensial.** `onclose` -> `creds = null`. Fokus kembali ke tombol "Tambah anggota" (perilaku `Dialog` mengembalikan fokus ke pemicu; karena dialog tambah sudah tertutup, pemicunya tombol header).
9. **Satu dialog aktif.** Saat `creds` diisi, semua dialog lain sudah ditutup lebih dulu (tidak ada dua `<dialog>` modal bertumpuk).

Markup urutan dalam `<section>`: header (judul + jumlah + tombol Tambah anggota), `InvitationsSection` (hanya `canManage`), lalu tabel anggota (state loading/error/empty yang ada). Dialog tambah: judul `project.members.add.title` di mode cari, `project.members.invite.title` di mode undang.

### Langkah 6. `app/views/workspace/MembersTab.svelte` (ubah)

```svelte
<script lang="ts">
  import { onMount } from 'svelte';
  import { refreshEmailStatus } from '../../../domain/core/auth/getme';
  import { mailStatus } from '../../../shared/stores/mailStatus';
  ...
  const canManage = $derived(ctx.project.access_level === 'owner' || ctx.project.access_level === 'admin');
  onMount(() => { if (canManage && $mailStatus === null) void refreshEmailStatus(); });
</script>
<MembersView ... emailEnabled={$mailStatus} />
```

Komposisi lintas module (core/auth getme + project/management members) hanya di `app/`, sesuai `.dependency-cruiser.cjs` (`outside-domain-only-via-index`).

### Langkah 7. `US` (halaman Users)

1. `US/types.ts` (ubah):
   ```ts
   export const UserInvitationSchema = z.object({
     pending: z.literal(true),
     invited_at: z.string(),
     sent_at: z.string().nullable(),
     expires_at: z.string(),
     expired: z.boolean(),
     delivery_status: EmailStatusSchema,
     resend_count: z.number().int().nonnegative(),
   });
   // UserSchema: tambah invitation: UserInvitationSchema.nullable()
   export const InviteUserResponseSchema = z.object({
     user: UserSchema,
     created: z.boolean(),
     email_sent: z.boolean(),
     email_status: EmailStatusSchema,
     expires_at: z.string(),
     credentials: TemporaryCredentialsSchema.optional(),
   });
   export const ResendUserInvitationResponseSchema = InviteUserResponseSchema.pick({ email_sent: true, email_status: true, expires_at: true, credentials: true });
   export interface InviteUserInput { email: string; name: string }
   ```
   - Catatan kontrak: `invitation` WAJIB ada (boleh `null`) di respons `listusers`/`getuser`. Respons `createuser`/`updateuser`/`setuserstatus`/`resetpassword` memakai `ToResponse()` lama yang menghasilkan `invitation: null` (usecase markers 3.1). Bila implementasi backend ternyata menghilangkan kunci itu pada endpoint lain, skema memakai `.nullable().default(null)` agar tidak memecah dialog lama; tes service mengunci kedua bentuk.
   - `toUserFormError` (ubah): 409 kode `ALREADY_REGISTERED` -> `{ kind: 'form', fields: { email: tr('invitation.error.alreadyRegistered.<reason>') }, reason }` (tipe `UserFormError` tambah `reason?: string` pada `kind:'form'`); 409 reason `INVITATION_NOT_PENDING` -> `{ kind: 'form', fields: {}, message: tr('invitation.error.notPending') }`; 429 -> `{ kind: 'form', fields: {}, message: tr('invitation.error.rateLimited.<scope>', { minutes }) }`. Kasus 409 lama (email taken, last super admin) tidak berubah.
2. `US/services/users.service.ts` (ubah):
   ```ts
   export function inviteUser(input: InviteUserInput): Promise<InviteUserResponse>  // POST /api/users/invite {email, name?}
   export function resendUserInvitation(id: string): Promise<ResendUserInvitationResponse> // POST /api/users/{id}/resend-invitation, tanpa body
   ```
   `name` hanya dikirim bila tidak kosong setelah trim. Email dinormalisasi trim + lowercase.
3. `US/model/invitation.ts` (baru, murni): `userInvitationBadge(u: User): 'expired' | 'failed' | 'pending' | null` (`null` bila `invitation===null`; `failed` mencakup `failed` dan `not_configured`; prioritas `expired` > `failed` > `pending`).
4. `US/components/InviteDeveloperDialog.svelte` (baru):
   - Props: `{ open = $bindable(), submitting, error: UserFormError | null, emailEnabled, onsubmit(input), onshowinlist(email), onclose }`.
   - Form `novalidate`: `Input` Email (wajib, `type="email"`, `autocomplete="off"`), `Input` Nama (opsional, hint `users.invite.nameHint`), `EmailDeliveryNote`, pesan form, footer Batal + "Kirim undangan" (primary).
   - Validasi lokal: email kosong -> `form.required`; bukan bentuk email -> `users.invite.emailInvalid` (format final tetap dari server).
   - Bila `error.reason === 'registered'`: di bawah field email muncul tombol ghost "Tampilkan di daftar" (`onshowinlist(email)`).
   - Tidak menampilkan kredensial sendiri; view membuka `TemporaryCredentialsDialog` setelah dialog ini ditutup.
5. `US/components/ResendInvitationDialog.svelte` (baru): konfirmasi kirim ulang. Props `{ open = $bindable(), user, submitting, error, onconfirm, onclose }`. Teks `users.dialog.resendBody` (sesi akun itu berakhir, password sementara lama tidak berlaku). Tombol "Kirim ulang" varian `primary` (bukan danger: tidak menghapus data).
6. `US/components/UsersTable.svelte` (ubah) sel Status:
   - Pil aktif/nonaktif tetap.
   - Bila `userInvitationBadge(u)` tidak null: pil kedua menggantikan pil "Wajib ganti password" (akun undangan selalu `must_change_password=true`, jadi dua pil itu mengatakan hal yang sama): `pending` -> neutral `users.badge.invited`, `failed` -> negative `users.badge.inviteFailed`, `expired` -> warning `users.badge.invitedExpired`. Baris kedua 12px `text-muted`: `users.invitation.expires` dengan tanggal `expires_at`.
   - Kolom Login terakhir: tetap ("Belum pernah" untuk undangan baru).
7. `US/components/UserRowActions.svelte` (ubah): prop baru `onresend`. Bila `user.invitation !== null && user.is_active`: aksi "Reset password" diganti "Kirim ulang undangan" (tetap tiga aksi per baris; reset password pada akun undangan menunggu tidak menerima undangan dan membingungkan). Akun nonaktif dengan undangan: tidak ada aksi kirim ulang (server menjawab `ALREADY_REGISTERED inactive`); aksi Aktifkan tetap.
8. `US/views/UsersListView.svelte` (ubah):
   - Props baru `emailEnabled: boolean | null`.
   - Title row: tombol "Undang developer" menjadi aksi utama (`variant` default primary); "Buat user" menjadi `variant="secondary"`. Satu tombol utama per layar tetap terjaga. Empty state tanpa filter menampilkan "Undang developer" (primary) dan "Buat user" (secondary).
   - `doInvite(input)`: `inviteUser`; sukses -> tutup dialog undang, `controller.refresh({ silent: true })`; `email_sent` -> toast `users.toast.invited` atau `users.toast.invitationResent` (`created=false`); fallback -> `credsState = {...}` membuka `TemporaryCredentialsDialog`; tanpa kredensial dan tidak terkirim (tidak mungkin di jalur ini menurut kontrak, ditangani defensif) -> toast error `users.toast.inviteDeliveryFailed`.
   - `onshowinlist(email)`: tutup dialog, `controller.setFilters({ role: '', status: '', q: email })`.
   - `doResend()`: `resendUserInvitation(user.id)`; sukses -> tutup konfirmasi, refresh diam, toast atau dialog kredensial; error -> `toUserFormError`, `notFound` -> refresh diam (pola yang ada).
9. `app/views/UsersView.svelte` (ubah): `onMount` -> `if ($mailStatus === null) void refreshEmailStatus()`; teruskan `emailEnabled={$mailStatus}`.
10. `US/index.ts`: tidak berubah (view tetap satu-satunya ekspor yang dipakai `app/`).

### Langkah 8. Login dan ganti password

1. `domain/core/auth/login/views/LoginView.svelte`: di bawah `LoginForm`, paragraf 13px `text-muted` `auth.login.invitationHint`. Tidak ada perubahan logika: login kedaluwarsa menjawab 401 `INVALID_CREDENTIALS` identik dengan password salah, jadi pesan yang ada sudah benar dan UI tidak boleh menebak sebabnya.
2. `domain/core/auth/changepassword/views/ChangePasswordView.svelte`: pada `mode==='forced'`, setelah `forcedDescription`, paragraf 13px `text-muted` `auth.changePassword.invitationHint`. Error kedaluwarsa (`400 fields.current_password` dengan pesan server "password sementara sudah kedaluwarsa, minta pengundang mengirim ulang undangan") sudah tampil lewat jalur `fields` yang ada; tes menguncinya.

### Langkah 9. i18n (`SH/i18n/id.json`, `SH/i18n/en.json`)

Semua teks lewat kunci; `i18n-keys.test.ts` yang ada memaksa paritas id/en.

| Kunci | id | en |
|---|---|---|
| `invitation.note.enabled` | Penerima mendapat email berisi alamat login dan password sementara. | The invitee gets an email with the sign-in address and a temporary password. |
| `invitation.note.disabled` | Email belum diatur di server ini. Setelah undangan dibuat, kredensial sementara ditampilkan sekali untuk Anda kirim sendiri. | Email is not set up on this server. After the invitation is created, the temporary credentials are shown once for you to send yourself. |
| `invitation.note.unknown` | Bila email gagal terkirim, kredensial sementara ditampilkan sekali untuk Anda kirim sendiri. | If the email cannot be sent, the temporary credentials are shown once for you to send yourself. |
| `invitation.credentials.title` | Kredensial sementara | Temporary credentials |
| `invitation.credentials.reason.failed` | Email undangan ke {email} gagal terkirim. | The invitation email to {email} could not be sent. |
| `invitation.credentials.reason.not_configured` | Email belum diatur di server, jadi undangan untuk {email} tidak dikirim lewat email. | Email is not set up on the server, so the invitation for {email} was not emailed. |
| `invitation.credentials.onceWarning` | Kredensial ini hanya ditampilkan sekali dan tidak bisa dibuka lagi. Kirim ke penerima lewat kanal yang aman. | These credentials are shown only once and cannot be opened again. Send them to the invitee over a safe channel. |
| `invitation.credentials.email` | Email | Email |
| `invitation.credentials.password` | Password sementara | Temporary password |
| `invitation.credentials.loginUrl` | Alamat login | Sign-in address |
| `invitation.credentials.expires` | Berlaku sampai {date} | Valid until {date} |
| `invitation.credentials.message` | Pesan untuk penerima | Message for the invitee |
| `invitation.credentials.messageBody` | Anda diundang ke Grimoire.\nMasuk di: {url}\nEmail: {email}\nPassword sementara: {password}\nBerlaku sampai {date}. Setelah masuk Anda diminta mengganti password. | You have been invited to Grimoire.\nSign in at: {url}\nEmail: {email}\nTemporary password: {password}\nValid until {date}. You will be asked to change the password after signing in. |
| `invitation.credentials.done` | Saya sudah menyalin | I have copied it |
| `invitation.error.alreadyRegistered.registered` | Email ini sudah punya akun. Cari dengan nama atau email lalu tambahkan langsung. | This email already has an account. Search by name or email and add it directly. |
| `invitation.error.alreadyRegistered.inactive` | Akun dengan email ini nonaktif. Minta Super Admin mengaktifkannya. | The account for this email is deactivated. Ask a Super Admin to activate it. |
| `invitation.error.alreadyRegistered.not_developer` | Email ini milik Super Admin dan tidak bisa diundang sebagai developer. | This email belongs to a Super Admin and cannot be invited as a developer. |
| `invitation.error.notPending` | Undangan sudah diterima atau tidak ada lagi. | The invitation was already accepted or no longer exists. |
| `invitation.error.rateLimited.invite` | Batas undangan per jam tercapai. Coba lagi dalam {minutes} menit. | Hourly invitation limit reached. Try again in {minutes} min. |
| `invitation.error.rateLimited.resend` | Undangan ini terlalu sering dikirim ulang. Coba lagi dalam {minutes} menit. | This invitation was resent too often. Try again in {minutes} min. |
| `project.members.add.candidatePending` | undangan menunggu | invitation pending |
| `project.members.invite.option` | Undang {email} lewat email | Invite {email} by email |
| `project.members.invite.alreadyMember` | Email ini sudah menjadi anggota | This email is already a member |
| `project.members.invite.title` | Undang lewat email | Invite by email |
| `project.members.invite.email` | Email | Email |
| `project.members.invite.change` | Ganti | Change |
| `project.members.invite.name` | Nama (opsional) | Name (optional) |
| `project.members.invite.nameHint` | Kosongkan untuk memakai bagian depan email. | Leave empty to use the part before the @. |
| `project.members.invite.submit` | Kirim undangan | Send invitation |
| `project.members.invite.searchInstead` | Cari developer | Search developers |
| `project.members.invite.success.sent` | Undangan dikirim ke {email} | Invitation sent to {email} |
| `project.members.invite.success.resent` | Undangan dikirim ulang ke {email} | Invitation resent to {email} |
| `project.members.invite.deliveryFailed` | {email} ditambahkan, tetapi email undangan gagal dikirim. Pakai Kirim ulang di Belum menerima undangan. | {email} was added, but the invitation email failed. Use Resend under Not yet accepted. |
| `project.members.status.invited` | Undangan menunggu | Invitation pending |
| `project.members.status.invitedExpired` | Undangan kedaluwarsa | Invitation expired |
| `project.invitations.title` | Belum menerima undangan | Not yet accepted |
| `project.invitations.col.invitee` | Undangan | Invitee |
| `project.invitations.col.role` | Peran | Role |
| `project.invitations.col.status` | Status | Status |
| `project.invitations.col.sent` | Dikirim | Sent |
| `project.invitations.col.expires` | Berlaku sampai | Valid until |
| `project.invitations.col.invitedBy` | Diundang oleh | Invited by |
| `project.invitations.col.actions` | Aksi | Actions |
| `project.invitations.resendCount` | dikirim ulang {n}x | resent {n}x |
| `project.invitations.status.expired` | Kedaluwarsa | Expired |
| `project.invitations.status.failed` | Email gagal terkirim | Email failed |
| `project.invitations.status.not_configured` | Kirim manual | Send manually |
| `project.invitations.status.pending` | Belum terkirim | Not sent yet |
| `project.invitations.status.sent` | Menunggu login | Awaiting sign-in |
| `project.invitations.resend` | Kirim ulang | Resend |
| `project.invitations.resendAria` | Kirim ulang undangan ke {email} | Resend invitation to {email} |
| `project.invitations.revoke` | Batalkan | Revoke |
| `project.invitations.revokeAria` | Batalkan undangan {email} | Revoke invitation for {email} |
| `project.invitations.resend.title` | Kirim ulang undangan? | Resend invitation? |
| `project.invitations.resend.message` | Password sementara baru dibuat untuk {email}; password lama langsung tidak berlaku. | A new temporary password is issued for {email}; the old one stops working at once. |
| `project.invitations.resend.success` | Undangan dikirim ulang ke {email} | Invitation resent to {email} |
| `project.invitations.revoke.title` | Batalkan undangan? | Revoke invitation? |
| `project.invitations.revoke.message` | {email} kehilangan akses ke project ini. Akunnya tetap ada sampai dibersihkan otomatis; Super Admin bisa menonaktifkannya dari halaman Users. | {email} loses access to this project. The account stays until it is cleaned up automatically; a Super Admin can deactivate it from Users. |
| `project.invitations.revoke.success` | Undangan dibatalkan | Invitation revoked |
| `project.invitations.revoke.notPending` | Undangan ini sudah diterima. Gunakan Keluarkan di daftar anggota. | This invitation was already accepted. Use Remove in the member list. |
| `project.invitations.loadError` | Gagal memuat undangan. | Could not load invitations. |
| `project.error.INVITATION_NOT_PENDING` | Undangan sudah diterima atau tidak ada lagi. | The invitation was already accepted or no longer exists. |
| `users.invite.open` | Undang developer | Invite developer |
| `users.invite.title` | Undang developer | Invite developer |
| `users.invite.email` | Email | Email |
| `users.invite.name` | Nama (opsional) | Name (optional) |
| `users.invite.nameHint` | Kosongkan untuk memakai bagian depan email. | Leave empty to use the part before the @. |
| `users.invite.emailInvalid` | Format email tidak valid | Invalid email format |
| `users.invite.submit` | Kirim undangan | Send invitation |
| `users.invite.showInList` | Tampilkan di daftar | Show in list |
| `users.badge.invited` | Undangan menunggu | Invitation pending |
| `users.badge.invitedExpired` | Undangan kedaluwarsa | Invitation expired |
| `users.badge.inviteFailed` | Email undangan gagal | Invitation email failed |
| `users.invitation.expires` | Berlaku s.d. {date} | Valid until {date} |
| `users.action.resendInvitation` | Kirim ulang undangan | Resend invitation |
| `users.dialog.resendTitle` | Kirim ulang undangan | Resend invitation |
| `users.dialog.resendBody` | Password sementara baru dikirim ke {email}. Password lama dan semua sesi akun ini langsung berakhir. | A new temporary password is sent to {email}. The old one and every session of this account end at once. |
| `users.dialog.resendSubmit` | Kirim ulang | Resend |
| `users.toast.invited` | Undangan dikirim ke {email} | Invitation sent to {email} |
| `users.toast.invitationResent` | Undangan dikirim ulang ke {email} | Invitation resent to {email} |
| `users.toast.inviteDeliveryFailed` | Undangan dibuat, tetapi email gagal dikirim. Pakai Kirim ulang undangan. | Invitation created, but the email failed. Use Resend invitation. |
| `auth.login.invitationHint` | Diundang lewat email? Masuk dengan password sementara dari email undangan; Anda akan diminta menggantinya. | Invited by email? Sign in with the temporary password from the invitation; you will be asked to change it. |
| `auth.changePassword.invitationHint` | Masuk dengan password sementara dari email undangan? Isi password itu di Password saat ini. | Signed in with the temporary password from an invitation? Enter it under Current password. |

### Langkah 10. Mock dan E2E helper

1. `MB/mocks/invitations.ts` dan `US/mocks/invitations.ts`: data contoh bagian 9.5 (dipakai tes komponen dan view). Total dihitung dari baris (`items.length`), tidak diketik terpisah.
2. `frontend/e2e/helpers/invitations.ts` (baru; file yang sama dirujuk `usecase-auth-login-changepassword.md` bagian 8. Siapa yang menulis lebih dulu membuat file, yang lain menambah fungsi; tanpa duplikasi nama):
   ```ts
   export async function inviteProjectApi(request, projectId, body: { email: string; role?: 'owner' | 'member'; name?: string })
   export async function inviteUserApi(request, body: { email: string; name?: string })
   export async function listInvitationsApi(request, projectId)
   export function expireInvitationDb(email: string): boolean // psql UPDATE user_invitations; false bila DATABASE_URL/psql tidak ada -> test.skip
   ```
3. Spec baru `frontend/e2e/invitations.spec.ts` (bagian 7). Server e2e tidak mengatur `MAIL_DRIVER` (lihat `playwright.config.ts`), jadi jalur nyata selalu fallback `not_configured`. Jalur `email_sent=true` dan `failed` diuji dengan `page.route` yang mengganti respons POST (bentuk dari bagian 8).

### Langkah 11. Verifikasi

```bash
cd frontend
npm run check && npm run lint && npm run test && npm run depcruise && npm run test:arch
S=../.agents/skills/design-taste-frontend
node $S/scripts/lint.mjs --app src/domain/project/management/members src/domain/core/auth/users src/shared/components
npm run build && npx playwright test e2e/invitations.spec.ts e2e/users.spec.ts e2e/project-management.spec.ts e2e/auth.spec.ts
```

Tahap Design check (`workflows/implement-feature.js`): `inspect.mjs --app` pada `/projects/<id>/members` dan `/admin/users` dengan dev server dan mock.

---

## 4. Migration

Tidak ada. Frontend tidak memiliki migrasi. Migrasi T12 hanya `backend/migrations/core/<VERSION>_create_user_invitations.sql` (overview bagian 5).

---

## 5. Changes (per file/folder)

| File | Aksi | Isi |
|---|---|---|
| `frontend/src/shared/validation/credentials.ts` (+ `.test.ts`) | baru | `TemporaryCredentialsSchema`, `EmailStatusSchema`, `rateLimitOf` |
| `frontend/src/shared/utils/url.ts` (+ tes) | ubah | `absoluteUrl` |
| `frontend/src/shared/stores/mailStatus.ts` (+ tes) | baru | store `boolean \| null` |
| `frontend/src/shared/components/TemporaryCredentialsDialog.svelte` | baru | dialog sekali tampil |
| `frontend/src/shared/components/EmailDeliveryNote.svelte` | baru | catatan pengiriman |
| `frontend/src/shared/components/index.ts`, `components.test.ts` | ubah | ekspor + tes |
| `frontend/src/shared/i18n/id.json`, `en.json` | ubah | kunci bagian 3.9 |
| `frontend/src/domain/core/auth/getme/{types.ts,services/getme.service.ts,bootstrap.ts,emailStatus.ts,index.ts}` (+ tes) | ubah/baru | `email_enabled`, `getMe`, `refreshEmailStatus` |
| `frontend/src/domain/core/auth/logout/logout.ts` (+ tes) | ubah | `resetMailStatus` |
| `frontend/src/domain/core/auth/login/views/LoginView.svelte` (+ tes) | ubah | teks bantu |
| `frontend/src/domain/core/auth/changepassword/views/ChangePasswordView.svelte` (+ tes) | ubah | teks bantu |
| `frontend/src/domain/core/auth/users/types.ts` (+ `types.test.ts`) | ubah | skema undangan, `toUserFormError` |
| `frontend/src/domain/core/auth/users/model/invitation.ts` (+ tes) | baru | `userInvitationBadge` |
| `frontend/src/domain/core/auth/users/services/users.service.ts` (+ tes) | ubah | `inviteUser`, `resendUserInvitation` |
| `frontend/src/domain/core/auth/users/components/InviteDeveloperDialog.svelte` (+ tes) | baru | dialog undang |
| `frontend/src/domain/core/auth/users/components/ResendInvitationDialog.svelte` (+ tes) | baru | konfirmasi kirim ulang |
| `frontend/src/domain/core/auth/users/components/{UsersTable,UserRowActions}.svelte` (+ tes) | ubah | penanda + aksi |
| `frontend/src/domain/core/auth/users/views/UsersListView.svelte` (+ tes) | ubah | wiring |
| `frontend/src/domain/core/auth/users/mocks/invitations.ts` | baru | data contoh |
| `frontend/src/domain/project/management/members/types.ts` | ubah | skema undangan, penanda member/candidate |
| `frontend/src/domain/project/management/members/model/invite.ts` (+ tes) | baru | logika murni undangan |
| `frontend/src/domain/project/management/members/services/invitations.service.ts` (+ tes) | baru | 4 panggilan + `toInviteError` |
| `frontend/src/domain/project/management/members/services/members.service.test.ts` | ubah | fixture dengan kunci baru |
| `frontend/src/domain/project/management/members/stores/invitationsList.store.ts` (+ tes) | baru | state daftar undangan |
| `frontend/src/domain/project/management/members/stores/candidateSearch.store.ts` (+ tes) | ubah | `inviteOption`, `move` |
| `frontend/src/domain/project/management/members/components/{CandidateCombobox,AddMemberForm,MembersTable}.svelte` (+ tes) | ubah | opsi undang, penanda |
| `frontend/src/domain/project/management/members/components/{InviteByEmailForm,InvitationsTable,InvitationStatusPill,InvitationsSection}.svelte` | baru | presentasi |
| `frontend/src/domain/project/management/members/components/components.test.ts` | ubah | tes komponen baru |
| `frontend/src/domain/project/management/members/views/MembersView.svelte`, `MembersView.test.ts` | ubah | wiring |
| `frontend/src/domain/project/management/members/mocks/{members,invitations}.ts` | ubah/baru | kunci baru + data contoh |
| `frontend/src/app/views/workspace/MembersTab.svelte` | ubah | `emailEnabled` |
| `frontend/src/app/views/UsersView.svelte` | ubah | `emailEnabled` |
| `frontend/src/app/views/ProjectWorkspaceView.test.ts`, `app/views/app.test.ts` | ubah | mock `/api/auth/me` dengan `email_enabled` |
| `frontend/e2e/helpers/invitations.ts` | baru | helper API + DB |
| `frontend/e2e/invitations.spec.ts` | baru | skenario bagian 7 |
| `frontend/e2e/users.spec.ts`, `project-management.spec.ts` | ubah (bila perlu) | locator tombol "Buat user" tetap by name; respons mock memuat kunci baru |

Tidak berubah: `app/routes/index.ts` (tidak ada route baru), `shared/services/http.ts`, `Dialog.svelte`, `CopyField.svelte`, `app.css` (tidak ada token baru).

---

## 6. Test Scenario (Vitest + Testing Library)

### 6.1 shared

| File | Skenario |
|---|---|
| `validation/credentials.test.ts` | Credentials valid lolos; `temporary_password` kosong ditolak; `EmailStatusSchema` menolak `queued`; `rateLimitOf` membaca `{retry_after_seconds, scope}`, `null` untuk details tanpa scope atau scope asing |
| `utils/url.test.ts` | `absoluteUrl('/login', 'https://g.tim.id')` -> `https://g.tim.id/login`; URL absolut https dipertahankan; `javascript:alert(1)` -> `origin/login` |
| `stores/mailStatus.test.ts` | awal `null`; set/reset |
| `components.test.ts` TemporaryCredentialsDialog | tiga field berisi nilai benar; URL relatif diresolusi ke origin; pesan siap tempel memuat URL, email, password, tanggal; kalimat sebab berbeda untuk `failed` vs `not_configured`; peringatan `role="alert"`; Escape dan klik latar tidak menutup; tombol "Saya sudah menyalin" memanggil `onclose` sekali; setelah `credentials=null` tidak ada teks password di DOM; tombol Salin memakai `copyText` (mock) dan gagal salin menampilkan `common.copyManual` |
| `components.test.ts` EmailDeliveryNote | tiga varian teks untuk `true`/`false`/`null` |
| `i18n-keys.test.ts` (ada) | paritas kunci id/en hijau |

### 6.2 getme / logout / login / changepassword

- `getme.service.test.ts`: `getMe` memvalidasi `email_enabled`; respons tanpa `email_enabled` -> `INVALID_RESPONSE`; `fetchMe` tetap mengembalikan `Identity` tanpa field mail.
- `bootstrap.test.ts`: sukses mengisi identity dan `mailStatus=false|true`; 401 -> identity null dan `mailStatus=null`; error lain dilempar tanpa mengubah store.
- `emailStatus.test.ts`: `refreshEmailStatus` mengisi store; dua panggilan bersamaan -> satu `getMe`; gagal -> store tetap `null`, tanpa throw.
- `logout.test.ts`: `resetMailStatus` dipanggil.
- `LoginView.test.ts`: teks `auth.login.invitationHint` tampil; 401 tetap `auth.login.error.invalid` (tidak ada pesan khusus kedaluwarsa).
- `ChangePasswordView.test.ts`: mode forced menampilkan `invitationHint`, mode profile tidak; 400 `fields.current_password` berisi pesan kedaluwarsa server tampil di bawah field Password saat ini.

### 6.3 users

- `types.test.ts`: `UserSchema` menerima `invitation: null` dan objek lengkap; `InviteUserResponseSchema` dengan dan tanpa `credentials`; `toUserFormError`: 409 `ALREADY_REGISTERED` tiap reason -> field email dengan teks reason dan `reason` terisi; 409 `INVITATION_NOT_PENDING` -> message; 429 scope `invite`/`resend` -> menit dibulatkan ke atas (1840 dtk -> 31 menit); 409 lama (email taken, last super admin) tidak berubah.
- `model/invitation.test.ts`: tabel `userInvitationBadge` (null, sent, pending, failed, not_configured, expired + failed -> expired).
- `users.service.test.ts`: `inviteUser` mengirim `{email: "baru@tim.id"}` tanpa `name` saat nama kosong, dengan `name` saat diisi, email dinormalisasi; URL `POST /api/users/invite`; `resendUserInvitation` mengirim POST tanpa body ke `/api/users/<encoded id>/resend-invitation`.
- `InviteDeveloperDialog.test.ts`: email kosong -> `form.required` tanpa memanggil `onsubmit`; email `bukan-email` -> `users.invite.emailInvalid`; submit valid memanggil `onsubmit({email, name})`; tombol nonaktif dan loading saat `submitting`; error `registered` menampilkan "Tampilkan di daftar" yang memanggil `onshowinlist(email)`; tiga varian `EmailDeliveryNote`.
- `ResendInvitationDialog.test.ts`: teks menyebut email; konfirmasi memanggil `onconfirm`; pesan error tampil `role="alert"`.
- `UsersTable.test.ts`: baris undangan menunggu menampilkan pil "Undangan menunggu" dan tidak menampilkan "Wajib ganti password"; kedaluwarsa -> pil warning; gagal -> pil negative; baris kedua tanggal; baris tanpa undangan tetap seperti T01.
- `UserRowActions.test.ts`: undangan aktif -> "Kirim ulang undangan" menggantikan "Reset password"; undangan nonaktif -> tidak ada kirim ulang; akun biasa -> tetap "Reset password".
- `UsersListView.test.ts` (fetch dimock):
  1. Tombol "Undang developer" adalah satu-satunya tombol primary di title row; "Buat user" sekunder.
  2. Undang sukses terkirim -> toast `users.toast.invited`, dialog tertutup, daftar di-refresh (GET kedua), tidak ada dialog kredensial.
  3. `created=false` -> toast `invitationResent`.
  4. Fallback `not_configured` -> `TemporaryCredentialsDialog` terbuka berisi password; tutup -> password hilang dari DOM; refresh tidak memunculkannya lagi.
  5. 409 `registered` -> pesan di field email; "Tampilkan di daftar" mengisi filter `q` dan menutup dialog.
  6. 429 -> pesan menit di dialog; dialog tetap terbuka.
  7. Kirim ulang dari baris: konfirmasi -> sukses toast; fallback -> dialog kredensial; 404 -> refresh diam dan pesan `users.error.notFound`.
  8. Double click "Kirim undangan" -> satu POST.

### 6.4 members

- `model/invite.test.ts`: `isEmailLike` (`baru@tim.id` true; `baru@tim` false; `a b@tim.id` false; 255 karakter false; dua `@` false); `normalizeEmail(' Baru@Tim.ID ')`; `inviteOptionFor` tabel (bukan email -> null; status loading -> null; kandidat email sama persis -> null; kandidat mirip saja -> `invite`; nol kandidat -> `invite`; email anggota -> `member`, perbandingan tanpa beda huruf); `invitationStatus` prioritas; `sortInvitations` urutan `added_at` menurun dan tie-break.
- `invitations.service.test.ts`: URL dan method keempat panggilan, `encodeURIComponent` pada id; body invite tanpa `name` kosong; respons tanpa `credentials` lolos dan `credentials` undefined; respons dengan `delivery_status` asing -> `INVALID_RESPONSE`; `toInviteError` untuk setiap baris tabel error di bagian 8.9 (termasuk 409 `ALREADY_REGISTERED` tanpa reason -> `registered`, 429 tanpa details -> menit 60); 204 revoke -> `void`.
- `invitationsList.store.test.ts`: begin/resolve mengurutkan; refresh mempertahankan item lama dengan `refreshing=true`; remove; fail; clear.
- `candidateSearch.store.test.ts`: `move` memasukkan opsi undang ke siklus (indeks terakhir) hanya bila `inviteOption==='invite'`; `setQuery` me-reset `inviteOption`; tanpa opsi perilaku lama tetap.
- `CandidateCombobox.test.ts`: opsi "Undang baru@tim.id lewat email" tampil saat nol kandidat; ArrowDown lalu Enter memanggil `oninvite('baru@tim.id')`; klik (mousedown) memanggil `oninvite`; `inviteOption='member'` menampilkan teks "sudah menjadi anggota" tanpa opsi; kandidat `invitation_pending` menampilkan "undangan menunggu"; query bukan email tidak menampilkan opsi; error pencarian tidak menampilkan opsi; `aria-activedescendant` menunjuk opsi undang saat aktif.
- `components.test.ts`:
  - `InviteByEmailForm`: email terkunci tampil dan "Ganti" memanggil `onback`; peran default member; submit memanggil `onsubmit`; error `fields.email` tampil di bawah baris email; `alreadyRegistered.registered` menampilkan tombol "Cari developer"; `rateLimited` menampilkan menit; tombol nonaktif saat `submitting`.
  - `InvitationStatusPill`: lima status -> teks dan kelas pil sesuai tabel langkah 4.5.
  - `InvitationsTable`: kolom "Diundang oleh" hilang bila semua null; `sent_at` null -> `–`; `resend_count=2` -> "dikirim ulang 2x"; aksi memanggil callback dengan item; `aria-label` aksi menyebut email; baris sibuk menonaktifkan aksi baris lain.
  - `InvitationsSection`: `ready` kosong -> tidak dirender; error -> `ErrorState` dengan Coba lagi; jumlah di header = `items.length`.
  - `MembersTable`: kolom Status muncul karena `invitation_pending`; pil "Undangan menunggu"; `invitation_expires_at` lampau (prop `now`) -> "Undangan kedaluwarsa"; `!found` tetap menang.
- `MembersView.test.ts` (fetch dimock per URL):
  1. Owner: GET members dan GET invitations dipanggil; section tampil di atas tabel anggota dengan dua baris.
  2. Member: GET invitations tidak dipanggil; tidak ada section; tidak ada tombol tambah.
  3. Invitations 500 -> section menampilkan error dan Coba lagi; tabel anggota tetap tampil.
  4. Invitations 404 -> `onprojectgone`; 403 -> section hilang dan `onaccesschanged`.
  5. Ketik `baru@tim.id` (debounce dengan fake timers) -> kandidat kosong -> opsi undang -> pilih -> judul dialog berubah "Undang lewat email"; isi nama "Nadia Putri", pilih Owner, kirim -> body `{email:"baru@tim.id", role:"owner", name:"Nadia Putri"}`.
  6. Respons `email_sent=true` -> toast sukses, baris anggota baru dengan pil "Undangan menunggu", GET invitations dipanggil ulang, tidak ada dialog kredensial.
  7. Respons fallback -> `TemporaryCredentialsDialog` terbuka dengan password `q7XkR2mVtz9HbNcP`; tutup -> password tidak ada di DOM; tidak ada store yang memuat string itu (cek `get(invitations)` dan `get($list)` di-serialize).
  8. Respons `email_sent=false` tanpa `credentials` -> toast error `deliveryFailed`, anggota tetap ditambahkan.
  9. 409 `ALREADY_MEMBER` -> pesan inline + GET members ulang; 409 `ALREADY_REGISTERED registered` -> pesan + "Cari developer" mengembalikan mode cari dengan query terisi email.
  10. 429 `scope=invite` 1840 dtk -> "Coba lagi dalam 31 menit".
  11. 403 saat undang -> dialog tertutup, toast `error.forbidden`, `onaccesschanged`.
  12. Kirim ulang: konfirmasi -> POST resend -> toast -> GET invitations ulang; fallback -> dialog kredensial; 429 resend -> pesan inline di konfirmasi; 409 `INVITATION_NOT_PENDING` -> toast + refresh kedua daftar.
  13. Batalkan: konfirmasi menampilkan pesan K13 -> DELETE -> baris hilang dari section dan tabel anggota tanpa GET ulang; 409 `INVITATION_NOT_PENDING` -> pesan `revoke.notPending` + refresh; 409 `LAST_OWNER` -> `project.error.LAST_OWNER`.
  14. `emailEnabled=false` -> catatan "Email belum diatur" di mode undang; `null` -> catatan netral.
  15. Klik "Kirim undangan" dua kali cepat -> satu POST.

### 6.5 Arsitektur dan desain

- `npm run depcruise` dan `npm run test:arch` hijau: `shared/*` baru tidak meng-import `domain/`; `app/views/*` hanya meng-import `getme/index.ts` dan `members/index.ts`; tidak ada import `domain/core/auth/users` dari `domain/project/**` atau sebaliknya.
- `lint.mjs --app` tanpa FAIL pada folder yang diubah (warna baru, radius baru, teks < 11px, emoji).

---

## 7. E2E Scenarios (Playwright, `frontend/e2e/invitations.spec.ts`)

Server e2e memakai binary tanpa `MAIL_DRIVER` (mailer noop), jadi respons nyata selalu `email_sent=false`, `email_status=not_configured`, `credentials` ada. Email tes memakai `uniqueEmail('inv')` (domain `tim.id`, helper yang ada).

Positif:

1. **T12-AC1/AC9 owner mengundang dari dialog.** Owner membuat project lewat API, membuka `/projects/<id>/members`, "Tambah anggota", mengetik email baru, memilih opsi "Undang ... lewat email", nama "Nadia Putri", peran Member, "Kirim undangan". Dialog "Kredensial sementara" tampil dengan email, password (`data-testid="temporary-password"`, 16 karakter), URL login berawalan origin, kalimat "Email belum diatur". Respons POST ber-header `Cache-Control: no-store` (ditangkap `page.waitForResponse`). Tutup -> baris muncul di "Belum menerima undangan" dengan pil "Kirim manual" dan di tabel anggota dengan "Undangan menunggu". Muat ulang halaman -> password tidak ada di DOM mana pun (`page.content()` tidak memuat string itu).
2. **T12-AC2 penerima masuk dan menerima.** Password dari langkah 1 dipakai di konteks browser baru: `/login` -> teks bantu undangan terlihat -> masuk -> diarahkan ke `/change-password` dengan teks bantu undangan -> ganti ke `STRONG_PASSWORD` -> beranda memuat project itu. Kembali ke konteks owner: muat ulang tab Anggota -> section undangan tidak ada (kosong), baris anggota tanpa pil undangan.
3. **Jalur terkirim (mock).** `page.route('**/api/projects/*/invitations', ...)` mengembalikan contoh 8.2 `email_sent=true` untuk POST -> toast "Undangan dikirim ke ..." dan tidak ada dialog kredensial.
4. **Kirim ulang dari project.** Undangan dibuat lewat `inviteProjectApi`; owner menekan "Kirim ulang", konfirmasi -> dialog kredensial dengan password berbeda dari yang pertama; login dengan password lama di konteks lain -> "Email atau password salah"; password baru berhasil.
5. **T12-AC8 batalkan.** Owner menekan "Batalkan" -> dialog memuat kalimat "Akunnya tetap ada" -> konfirmasi -> baris hilang dari kedua daftar tanpa reload, toast. Super Admin di `/admin/users` mencari email itu -> baris masih ada dengan pil "Undangan menunggu".
6. **T12-AC6 Super Admin mengundang dari Users.** `/admin/users` -> "Undang developer" (tombol primary) -> email baru, nama kosong -> "Kirim undangan" -> dialog kredensial -> tutup -> baris baru bernama bagian lokal email dengan pil "Undangan menunggu" dan aksi "Kirim ulang undangan" (tanpa "Reset password").
7. **Kirim ulang dari Users memulihkan kedaluwarsa (AC3).** Undangan dibuat lewat `inviteUserApi`, `expireInvitationDb(email)` (skip bila tidak ada `DATABASE_URL`/`psql`) -> muat ulang Users -> pil "Undangan kedaluwarsa" -> "Kirim ulang undangan" -> konfirmasi -> dialog kredensial -> pil kembali "Undangan menunggu"; login dengan password baru berhasil.
8. **K6 undang ulang email menunggu dari project lain.** Email yang sudah menunggu (dari skenario 5) diundang ke project kedua -> respons `created=false` -> dialog kredensial (password baru); tidak ada baris user ganda di Users.
9. **i18n.** Konteks `locale=en`: tombol "Invite developer", judul dialog "Temporary credentials".
10. **Mailpit (opsional, skip bila `E2E_MAILPIT_URL` kosong).** Server dengan `MAIL_DRIVER=smtp` ke Mailpit: undang dari project -> toast terkirim, tidak ada dialog kredensial; `GET $E2E_MAILPIT_URL/api/v1/messages` memuat satu pesan ke email itu berisi URL login (T12-AC12).

Negatif:

1. **T12-AC4 email sudah terdaftar.** Email milik developer siap (`createReadyUserApi`) diketik di combobox -> kandidat itu muncul dan opsi undang TIDAK muncul (email sama persis). Lewat API `inviteProjectApi` -> 409 `ALREADY_REGISTERED`. Di Users, "Undang developer" dengan email itu -> pesan di field email + "Tampilkan di daftar" memfilter daftar ke email itu.
2. **Sudah anggota.** Email anggota yang ada diketik -> baris "Email ini sudah menjadi anggota", tanpa opsi undang. Dua tab mengundang email baru yang sama bersamaan -> tab kedua menampilkan "User sudah menjadi anggota", satu baris anggota.
3. **T12-AC5 member.** Member membuka tab Anggota -> tidak ada tombol tambah, tidak ada section undangan, tidak ada request ke `/invitations` (dicek `page.on('request')`). `request.post` langsung -> 403; developer non-anggota -> 404.
4. **Developer membuka `/admin/users`** -> NotFound (perilaku T01); `POST /api/users/invite` -> 403.
5. **T12-AC7 rate limit (mock).** `page.route` mengembalikan 429 `scope=resend` `retry_after_seconds=1840` untuk resend -> pesan "Coba lagi dalam 31 menit" di dialog konfirmasi; tombol tidak dikirim ulang otomatis.
6. **Email tidak valid.** Mengetik `bukan-email` -> tidak ada opsi undang. Users: `bukan-email` -> pesan lokal, tidak ada POST.
7. **Batalkan undangan yang sudah diterima.** Undang, penerima mengganti password lewat API, owner (tab lama tanpa reload) menekan "Batalkan" -> pesan "Undangan ini sudah diterima. Gunakan Keluarkan..." dan daftar diperbarui.
8. **Login kedaluwarsa (AC3).** Setelah `expireInvitationDb`, login dengan password sementara -> pesan "Email atau password salah" (sama persis dengan password salah).
9. **Respons rusak.** `page.route` GET invitations mengembalikan `{"items":[{"email":1}]}` -> section menampilkan error "Respons server tidak sesuai format" dengan Coba lagi; tabel anggota tetap.
10. **Tanpa sesi.** Membuka `/projects/<id>/members` tanpa login -> ke `/login` lalu kembali (perilaku T03, tidak ada request undangan sebelum login).

---

## 8. Request/Response body

Semua JSON snake_case. Mutasi mengirim `X-Requested-With: grimoire` (otomatis dari `http.ts`) dan cookie `grimoire_session`. Contoh memakai data bagian 9.5 (project "Payments Gateway" `6f2d8d3a-1c1e-4a55-8f40-3d2b6a9e0c11`, owner Raka Pratama).

### 8.1 `GET /api/auth/me` (aditif)

```json
{
  "id": "2b6a0f5e-8f3a-4c0a-9d2e-5c1f7a9b3e10",
  "email": "raka@tim.id",
  "name": "Raka Pratama",
  "role": "developer",
  "must_change_password": false,
  "email_enabled": false
}
```

### 8.2 `POST /api/projects/{projectID}/invitations`

Request:
```json
{ "email": "nadia.putri@tim.id", "role": "member", "name": "Nadia Putri" }
```
Request minimal (nama kosong tidak dikirim):
```json
{ "email": "bagas@tim.id", "role": "member" }
```

201 terkirim (tanpa kunci `credentials`):
```json
{
  "member": {
    "user_id": "7c1f3c2e-5b7a-4d7e-9a51-1f0b8a3e6d21",
    "name": "Nadia Putri",
    "email": "nadia.putri@tim.id",
    "is_active": true,
    "found": true,
    "role": "member",
    "added_at": "2026-10-21T08:15:00Z",
    "added_by": "2b6a0f5e-8f3a-4c0a-9d2e-5c1f7a9b3e10",
    "added_by_label": "raka@tim.id",
    "invitation_pending": true,
    "invitation_expires_at": "2026-10-28T08:15:00Z"
  },
  "created": true,
  "email_sent": true,
  "email_status": "sent",
  "expires_at": "2026-10-28T08:15:00Z"
}
```

201 fallback (header `Cache-Control: no-store`):
```json
{
  "member": {
    "user_id": "c4a1e9d2-7b3f-4e8a-9c5d-2f6b1a0e8d34",
    "name": "bagas",
    "email": "bagas@tim.id",
    "is_active": true,
    "found": true,
    "role": "member",
    "added_at": "2026-10-21T09:02:00Z",
    "added_by": "2b6a0f5e-8f3a-4c0a-9d2e-5c1f7a9b3e10",
    "added_by_label": "raka@tim.id",
    "invitation_pending": true,
    "invitation_expires_at": "2026-10-28T09:02:00Z"
  },
  "created": true,
  "email_sent": false,
  "email_status": "not_configured",
  "expires_at": "2026-10-28T09:02:00Z",
  "credentials": {
    "email": "bagas@tim.id",
    "temporary_password": "q7XkR2mVtz9HbNcP",
    "login_url": "https://grimoire.tim.id/login"
  }
}
```

201 langkah terbit gagal total (anggota tercatat, tanpa `credentials`): sama dengan contoh terkirim kecuali `"email_sent": false, "email_status": "failed"`.

### 8.3 `GET /api/projects/{projectID}/invitations`

200 (`Cache-Control: no-store`):
```json
{
  "items": [
    {
      "user_id": "c4a1e9d2-7b3f-4e8a-9c5d-2f6b1a0e8d34",
      "email": "bagas@tim.id",
      "name": "bagas",
      "role": "member",
      "invited_by_label": "raka@tim.id",
      "added_at": "2026-10-21T09:02:00Z",
      "sent_at": null,
      "expires_at": "2026-10-28T09:02:00Z",
      "expired": false,
      "delivery_status": "failed",
      "email_failed": true,
      "resend_count": 1
    },
    {
      "user_id": "7c1f3c2e-5b7a-4d7e-9a51-1f0b8a3e6d21",
      "email": "nadia.putri@tim.id",
      "name": "Nadia Putri",
      "role": "member",
      "invited_by_label": "raka@tim.id",
      "added_at": "2026-10-21T08:15:00Z",
      "sent_at": "2026-10-21T08:15:01Z",
      "expires_at": "2026-10-28T08:15:00Z",
      "expired": false,
      "delivery_status": "sent",
      "email_failed": false,
      "resend_count": 0
    },
    {
      "user_id": "3a9d1e47-62c0-4f0b-8e1d-5c7a9b2f4e10",
      "email": "yoga.saputra@tim.id",
      "name": "Yoga Saputra",
      "role": "owner",
      "invited_by_label": "raka@tim.id",
      "added_at": "2026-10-12T03:00:00Z",
      "sent_at": "2026-10-12T03:00:01Z",
      "expires_at": "2026-10-19T03:00:00Z",
      "expired": true,
      "delivery_status": "sent",
      "email_failed": false,
      "resend_count": 0
    }
  ]
}
```
Kosong: `{ "items": [] }`.

### 8.4 `POST /api/projects/{projectID}/invitations/{userID}/resend` dan `POST /api/users/{id}/resend-invitation`

Tanpa body. 200 terkirim:
```json
{ "email_sent": true, "email_status": "sent", "expires_at": "2026-10-28T10:30:00Z" }
```
200 fallback (`Cache-Control: no-store`):
```json
{
  "email_sent": false,
  "email_status": "failed",
  "expires_at": "2026-10-28T10:30:00Z",
  "credentials": {
    "email": "bagas@tim.id",
    "temporary_password": "Zp4wNe8TqYv3KdRh",
    "login_url": "/login"
  }
}
```
(`login_url` relatif bila `PUBLIC_URL` kosong; UI meresolusi ke `window.location.origin`.)

### 8.5 `DELETE /api/projects/{projectID}/invitations/{userID}`

204 tanpa body.

### 8.6 `POST /api/users/invite`

Request:
```json
{ "email": "dimas.anggoro@tim.id", "name": "Dimas Anggoro" }
```
Request nama kosong (kunci `name` tidak dikirim):
```json
{ "email": "rani@tim.id" }
```
201 fallback (`Cache-Control: no-store`):
```json
{
  "user": {
    "id": "e8b2c6a1-4d3f-4a9e-8b1c-7d6e5f4a3b21",
    "email": "rani@tim.id",
    "name": "rani",
    "role": "developer",
    "is_active": true,
    "must_change_password": true,
    "created_at": "2026-10-21T10:05:00Z",
    "created_by": "0b7c6a4e-2f1d-4c8a-9e3b-5d6f7a8b9c0d",
    "created_by_label": "Sinta Admin",
    "updated_at": "2026-10-21T10:05:00Z",
    "last_login_at": null,
    "invitation": {
      "pending": true,
      "invited_at": "2026-10-21T10:05:00Z",
      "sent_at": null,
      "expires_at": "2026-10-28T10:05:00Z",
      "expired": false,
      "delivery_status": "not_configured",
      "resend_count": 0
    }
  },
  "created": true,
  "email_sent": false,
  "email_status": "not_configured",
  "expires_at": "2026-10-28T10:05:00Z",
  "credentials": {
    "email": "rani@tim.id",
    "temporary_password": "Hm8tYq3RkW9vNcPz",
    "login_url": "https://grimoire.tim.id/login"
  }
}
```
201 terkirim: sama tanpa `credentials`, `email_sent: true`, `email_status: "sent"`, `invitation.sent_at` terisi.

### 8.7 `GET /api/users` (item, aditif)

```json
{
  "items": [
    {
      "id": "e8b2c6a1-4d3f-4a9e-8b1c-7d6e5f4a3b21",
      "email": "rani@tim.id",
      "name": "rani",
      "role": "developer",
      "is_active": true,
      "must_change_password": true,
      "created_at": "2026-10-21T10:05:00Z",
      "created_by": "0b7c6a4e-2f1d-4c8a-9e3b-5d6f7a8b9c0d",
      "created_by_label": "Sinta Admin",
      "updated_at": "2026-10-21T10:05:00Z",
      "last_login_at": null,
      "invitation": { "pending": true, "invited_at": "2026-10-21T10:05:00Z", "sent_at": null, "expires_at": "2026-10-28T10:05:00Z", "expired": false, "delivery_status": "not_configured", "resend_count": 0 }
    },
    {
      "id": "9a0d4b6e-1c2f-4e3a-8b7c-6d5e4f3a2b10",
      "email": "raka@tim.id",
      "name": "Raka Pratama",
      "role": "developer",
      "is_active": true,
      "must_change_password": false,
      "created_at": "2026-09-01T02:00:00Z",
      "created_by": null,
      "created_by_label": null,
      "updated_at": "2026-09-01T02:00:00Z",
      "last_login_at": "2026-10-21T07:30:00Z",
      "invitation": null
    }
  ],
  "total": 2,
  "limit": 20,
  "offset": 0
}
```

### 8.8 Aditif lain

`GET /api/projects/{projectID}/members` item tanpa undangan: `"invitation_pending": false, "invitation_expires_at": null`.
`GET /api/projects/{projectID}/member-candidates?q=nadia&limit=10`:
```json
{ "items": [ { "id": "7c1f3c2e-5b7a-4d7e-9a51-1f0b8a3e6d21", "name": "Nadia Putri", "email": "nadia.putri@tim.id", "invitation_pending": true } ] }
```

### 8.9 Format error dan pemetaan UI

Envelope (dibaca `toApiError` di `http.ts`):
```json
{ "error": { "code": "ALREADY_REGISTERED", "message": "email sudah terdaftar", "details": { "reason": "registered" } }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

| Status | `code` / `details` | Endpoint | UI |
|---|---|---|---|
| 400 | `INVALID_ARGUMENT`, `details.fields.{email,role,name}` | invite (dua jalur) | pesan di field |
| 400 | `INVALID_ARGUMENT`, `details.fields.current_password` (kedaluwarsa) | change-password | pesan di field Password saat ini |
| 401 | `UNAUTHORIZED` (beberapa plan menulis `UNAUTHENTICATED`; UI memakai status, bukan code) | semua | `notifyUnauthorized` yang ada, ke `/login` |
| 401 | `INVALID_CREDENTIALS` | login | "Email atau password salah" (sama untuk kedaluwarsa) |
| 403 | `FORBIDDEN` / `PASSWORD_CHANGE_REQUIRED` / `CSRF_REQUIRED` | semua | project: tutup dialog, toast, `onaccesschanged`; Users: pesan form |
| 404 | `NOT_FOUND` "project tidak ditemukan" / "anggota tidak ditemukan" / "user tidak ditemukan" | project & users | project: `recheckAfter404`/`onprojectgone`; Users: refresh diam + "User tidak ditemukan" |
| 409 | `CONFLICT` reason `ALREADY_MEMBER` | invite project | inline + reload anggota |
| 409 | `ALREADY_REGISTERED` reason `registered` / `inactive` / `not_developer` | invite (dua jalur), resend (`inactive`) | teks per reason; `registered` memberi tombol ke pencarian/daftar |
| 409 | `CONFLICT` reason `INVITATION_NOT_PENDING` | resend, revoke | toast/inline + refresh |
| 409 | `CONFLICT` reason `LAST_OWNER` | revoke | `project.error.LAST_OWNER` |
| 429 | `RATE_LIMITED`, `details.retry_after_seconds`, `details.scope` `invite`/`resend` | invite, resend | "Coba lagi dalam N menit" |
| 500 | `INTERNAL` | semua | pesan generik + ID jejak |
| 200/201 | respons tidak lolos Zod | semua | `INVALID_RESPONSE` -> `ErrorState` / pesan generik |

Contoh 429:
```json
{ "error": { "code": "RATE_LIMITED", "message": "terlalu banyak permintaan undangan", "details": { "retry_after_seconds": 1840, "scope": "resend" } }, "trace_id": "4f6c2a1e-8d3b-4c55-9a1f-2b7e0d9c1a33" }
```
Contoh 409 sudah anggota:
```json
{ "error": { "code": "CONFLICT", "message": "user sudah menjadi anggota", "details": { "reason": "ALREADY_MEMBER" } }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```
Contoh 409 undangan tidak menunggu:
```json
{ "error": { "code": "CONFLICT", "message": "undangan sudah diterima atau tidak ada", "details": { "reason": "INVITATION_NOT_PENDING" } }, "trace_id": "01JB2X9K6Q4Y7V3N8M5T2R1C0A" }
```

---

## 9. Visual

### 9.1 Pertanyaan per layar dan blok terbesar

| Layar | Pertanyaan | Blok terbesar yang menjawab |
|---|---|---|
| Tab Anggota (`/projects/:id/members`) | Siapa yang punya akses ke project ini, dan siapa yang belum masuk? | Tabel anggota (tetap blok terbesar). Section "Belum menerima undangan" di atasnya, hanya bila ada isinya, karena berisi baris yang perlu tindakan (app-screens 2.2: blok yang membawa aksi naik). |
| Dialog Tambah anggota, mode undang | Bagaimana orang ini bisa masuk ke project? | Baris email terkunci + peran + tombol "Kirim undangan"; catatan pengiriman tepat di atas tombol. |
| Dialog Kredensial sementara | Apa yang harus saya kirim ke penerima, dan di mana? | Tiga `CopyField` (email, password, alamat login) dan "Pesan untuk penerima"; peringatan sekali tampil di atasnya. |
| `/admin/users` | Siapa saja yang punya akses? (tidak berubah) | Tabel user; status undangan dibaca di kolom Status, satu pil per baris. |
| Dialog Undang developer | Siapa yang saya undang? | Field email. |
| `/login`, `/change-password` | tidak berubah (`BlankLayout`) | Form; teks bantu undangan 13px redup di bawah form/deskripsi. |

Temuan utama (app-screens 2.2) di tab Anggota: baris yang perlu tindakan ("Email gagal terkirim", "Kedaluwarsa") adalah satu-satunya warna di section, sehingga terbaca lebih dulu daripada baris "Menunggu login".

### 9.2 Pola DESIGN.md dan kerangka

- Kerangka: `AppLayout` yang ada (sidebar `bg-ground`, sheet putih). Tidak ada perubahan shell, tidak ada top bar desktop.
- Tab Anggota: pola **workspace project** (title row + tab garis bawah) dengan isi pola **list**: header section (`h2`/`h3` 16px + jumlah redup + satu tombol), tabel dibungkus panel (`Table.svelte`: radius 8, border, `shadow-panel`, header 13px abu pada pita `bg-raised`, baris 48px, rule `border-rule`).
- `/admin/users`: pola **list dengan status pil** yang ada. Title row tetap satu tombol primary ("Undang developer"); "Buat user" sekunder di sebelahnya.
- Dialog: pola dialog yang ada (`Dialog.svelte`: `rounded-sheet`, `shadow-pop`, header dengan rule). Dialog kredensial memakai pola "dialog hasil key" dari tab API Keys (peringatan sekali tampil + `CopyField` mono).
- Ponsel: tabel undangan memakai `overflow-x-auto` di dalam bingkainya sendiri, sama dengan `MembersTable` T03 (halaman tidak pernah scroll ke samping). Bentuk dua baris per row di bawah 640px (app-screens 12) belum dipakai tabel mana pun di repo; tidak diperkenalkan di T12 agar dua tabel di satu tab tetap seragam (usulan di 9.7).

### 9.3 Elemen khas produk di layar pertama

- **Avatar pucat** (inisial + hue stabil, bulat) di kolom Undangan dan tabel anggota. Orang undangan langsung tampil sebagai orang di tim, bukan baris email kosong.
- **Mono untuk satu pekerjaan**: password sementara dan alamat login di dialog kredensial memakai `CopyField` (mono 13px) karena keduanya disalin dan dibaca mesin/form login. Email, nama, dan label tetap memakai font teks.
- **Status hanya pada data**: pil `pill-negative` "Email gagal terkirim" dan `pill-warning` "Kedaluwarsa" adalah satu-satunya warna baru di tab Anggota; kerangka tetap tanpa warna.
- **Aksen kuning** tidak dipakai untuk fitur ini (tidak ada tab baru, chip agent, atau seri chart). Cincin fokus dan caret tetap `accent-ink` dari komponen yang ada.

### 9.4 Token / utility yang dipakai (tanpa nilai hex baru)

- Permukaan: `bg-surface`, `bg-raised` (header tabel, hover `bg-raised/60`), `bg-active` (opsi aktif combobox), `border-border`, `border-rule`, `border-control` (field `CopyField`, input).
- Teks: `text-fg`, `text-muted`, `text-danger` (pesan error), `text-warning` + `border-warning` (kotak peringatan sekali tampil, pola `KeyResult`).
- Pil: `bg-pill-neutral-bg text-pill-neutral-ink`, `bg-pill-negative-bg text-pill-negative-ink`, `bg-pill-warning-bg text-pill-warning-ink`, `bg-pill-positive-bg text-pill-positive-ink` (status aktif yang ada di Users).
- Radius: `rounded-base` (kontrol, kotak peringatan), `rounded-card` (panel tabel), `rounded-sheet` (dialog), `rounded-full` (pil). Tidak ada radius lain.
- Bayangan: `shadow-panel` (tabel), `shadow-pop` (dialog, dropdown combobox lewat `data-ui="overlay"` yang ada). Tidak ada bayangan baru.
- Fokus: `focus-visible:ring-2 focus-visible:ring-accent-ink`.
- Ukuran: judul section 16px (`text-base font-semibold`), badan 14px, sub-baris dan teks bantu 12-13px; tidak ada teks di bawah 12px. Angka (`resend_count`, jumlah) mengikuti `tabular-nums` global.
- Tombol: `Button` varian `primary` (satu per layar/dialog), `secondary`, `ghost` (aksi baris), `danger` (konfirmasi Batalkan).

### 9.5 Data contoh (mock FE slicing dan fixture tes)

`MB/mocks/invitations.ts` (project "Payments Gateway", owner Raka Pratama `raka@tim.id`, "hari ini" = 2026-10-21 10:30 UTC, TTL 7 hari):

| Undangan | Peran | Status (`invitationStatus`) | Dikirim | Berlaku sampai | Diundang oleh | `resend_count` |
|---|---|---|---|---|---|---|
| bagas (`bagas@tim.id`) | Member | failed -> "Email gagal terkirim" | – | 28 Okt 2026 09.02 | raka@tim.id | 1 |
| Nadia Putri (`nadia.putri@tim.id`) | Member | sent -> "Menunggu login" | 21 Okt 2026 08.15 | 28 Okt 2026 08.15 | raka@tim.id | 0 |
| Yoga Saputra (`yoga.saputra@tim.id`) | Owner | expired -> "Kedaluwarsa" | 12 Okt 2026 03.00 | 19 Okt 2026 03.00 | raka@tim.id | 0 |

Jumlah di header section = `items.length` = 3. Anggota di `MB/mocks/members.ts`: Raka Pratama (owner), Sari Wulandari (member, `sari.w@tim.id`), plus tiga baris undangan di atas dengan `invitation_pending: true` (5 baris; jumlah di header anggota dihitung dari `items.length`). `expires_at` setiap baris anggota identik dengan baris undangannya (satu variabel di file mock, dua tampilan).

`US/mocks/invitations.ts`: Sinta Admin (super_admin, akun sendiri), Raka Pratama (developer biasa, login terakhir 21 Okt 07.30), rani (`rani@tim.id`, undangan `not_configured` -> pil "Email undangan gagal"), Dimas Anggoro (`dimas.anggoro@tim.id`, undangan `sent` -> "Undangan menunggu", berlaku s.d. 28 Okt 2026), Yoga Saputra (kedaluwarsa). Total = 5 dari baris.

Kredensial contoh: `rani@tim.id` / `Hm8tYq3RkW9vNcPz` / `https://grimoire.tim.id/login`. Tidak ada Acme, John Doe, lorem, atau angka vanity.

### 9.6 State per permukaan

| Permukaan | Loading | Empty | Error |
|---|---|---|---|
| Tabel anggota | spinner yang ada di bingkai | EmptyState yang ada + tombol Tambah anggota | `ErrorState` + Coba lagi (yang ada) |
| Section "Belum menerima undangan" | tidak dirender saat muat pertama; saat refresh baris lama tetap + "Memuat..." redup di header | tidak dirender (keputusan F2) | bingkai section dengan `ErrorState` kompak + Coba lagi; tabel anggota tidak terpengaruh |
| Combobox kandidat | "Mencari..." (yang ada) | "Tidak ada developer aktif yang cocok" + opsi undang bila email | "Pencarian gagal" tanpa opsi undang |
| Dialog undang (dua jalur) | tombol "Kirim undangan" loading, field nonaktif | n/a | pesan per field / form, dialog tetap terbuka |
| Konfirmasi Kirim ulang / Batalkan | tombol konfirmasi loading | n/a | pesan inline `role="alert"` |
| Dialog kredensial | n/a (hanya dibuka dengan data) | n/a | gagal salin -> pesan "salin manual" dan teks terseleksi (perilaku `CopyField`) |
| Tabel Users | perilaku T01 | perilaku T01 (empty kini menawarkan "Undang developer") | perilaku T01 |
| `mailStatus` | `null` -> catatan netral | n/a | gagal ambil -> tetap `null`, catatan netral |

Hover/fokus/disabled: semua dari komponen yang ada (`Button`, `Table`, combobox). Aksi baris lain nonaktif saat satu baris sibuk (pola `busyUserId` T03). Gerak hanya umpan balik bawaan komponen; tidak ada animasi masuk.

### 9.7 Usulan perubahan DESIGN.md (catatan, tidak dikerjakan diam-diam)

1. Tabel "Pola per layar": tambah baris tab Anggota (`/projects/:id/members`) dengan pertanyaan "Siapa yang punya akses ke project ini, dan siapa yang belum masuk?" dan pola "list; section undangan menunggu di atas tabel bila ada isinya".
2. "Wajah produk" butir 1 (mono): tambahkan "password sementara dan alamat login di dialog kredensial" ke daftar hal yang dibaca mesin, agar pemakaian `CopyField` mono di sini tercatat, bukan pengecualian diam.
3. "Status" baris Users: catat bahwa pil undangan menggantikan pil "Wajib ganti password" (satu penanda per baris).
4. Bentuk tabel di ponsel (dua baris per row di bawah 640px, app-screens 12) belum diputuskan untuk tabel mana pun; usul diputuskan sekali untuk semua tabel daftar, bukan per tiket.
5. Kebab menu baris: T12 sengaja tidak menambah aksi ketiga di baris anggota (Kirim ulang/Batalkan ada di section terpisah) dan menukar Reset password dengan Kirim ulang di baris user undangan, sehingga batas "lebih dari dua aksi -> kebab" tidak dilanggar lebih jauh. Butir "Belum dikerjakan: kebab menu" tetap berlaku.

---

## 10. Keputusan dan selisih dengan overview / tiket

| ID | Keputusan | Alasan |
|---|---|---|
| F1 | Undangan diimplementasikan di feature `members` dan `users` yang ada, bukan feature FE baru | UI-nya bagian dari dialog dan tabel yang sama; menghindari state lintas feature di `app/` |
| F2 | Section "Belum menerima undangan" tidak dirender bila kosong | Tidak ada aksi untuk section kosong; tabel anggota tetap menjaga bingkainya sendiri. Tiket: "Pembangun boleh menggabungkan tampilannya di UI" |
| F3 | Opsi undang muncul bila teks berbentuk email dan tidak ada kandidat/anggota dengan email itu persis (bukan hanya saat hasil kosong) | Kandidat yang mirip (`nadia@tim.id` vs `nadia.putri@tim.id`) tidak boleh menghalangi undangan; email yang sama persis tetap dipilih lewat jalur `addmember` T03 |
| F4 | "Undang developer" menjadi aksi utama di Users, "Buat user" sekunder | Tujuan tiket 2 (Super Admin mengundang tanpa membuat password); satu primary per layar |
| F5 | Pada akun undangan menunggu, "Reset password" diganti "Kirim ulang undangan" | Reset tidak menerima undangan dan membuat dua jalur password sementara; baris tetap tiga aksi |
| F6 | `email_enabled` disimpan di store `mailStatus` terpisah dari `Identity`, diisi bootstrap dan diambil malas oleh `app/` | Respons login tidak membawa `email_enabled`; Identity tetap data user |
| F7 | Kirim ulang selalu lewat konfirmasi (project dan Users) | Kirim ulang mencabut sesi dan membatalkan password lama (aturan bisnis tiket) |
| F8 | Setelah mutasi, daftar undangan diambil ulang, kecuali revoke (hapus lokal dari dua daftar) | Status kirim dan masa berlaku milik auth; revoke tidak punya data baru untuk diambil |
| F9 | Route resend Users memakai `{id}` (overview 0, fakta 4); route project memakai `{userID}` | Ikut kontrak backend |
| F10 | Kode 401 dibaca dari status HTTP, bukan `code` | Plan usecase tidak seragam (`UNAUTHORIZED` vs `UNAUTHENTICATED`); `http.ts` sudah memakai status |

---

## 11. Risiko dan catatan

- Kredensial di DOM: hanya selama dialog terbuka. Tes view memastikan string password tidak tersisa di DOM atau store setelah ditutup. `CopyField` memakai `readonly input` yang bisa tertangkap ekstensi browser; risiko sama dengan dialog API key T04 dan diterima.
- `login_url` relatif bergantung pada origin halaman; bila aplikasi di balik proxy dengan origin berbeda dari `PUBLIC_URL`, operator wajib mengisi `PUBLIC_URL` (sudah wajib untuk SMTP).
- Pesan error kedaluwarsa di ganti password berasal dari server (bahasa Indonesia saja, pola T01); di locale `en` pesan itu tetap Indonesia. Diterima, sama dengan pesan field lain dari server.
- Debounce combobox + opsi undang: tes memakai fake timers agar opsi tidak berkedip di antara hasil pencarian.
- Bila OpenAPI final menamai field berbeda dari contoh di plan usecase, Zod akan menolak (`INVALID_RESPONSE`) dan tes service gagal lebih dulu; perbaikan dilakukan di `types.ts` saja.
- Semua K-pembangun tiket (K3, K5/K6, K13) belum disetujui pemilik; UI mengikuti plan backend apa adanya. Bila K13 berubah (revoke menghapus akun), hanya teks `project.invitations.revoke.message` yang berubah.
