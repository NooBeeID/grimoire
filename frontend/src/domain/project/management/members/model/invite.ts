import type { CandidateState } from '../stores/candidateSearch.store';
import { MAX_EMAIL, type Invitation, type Member } from '../types';

export function normalizeEmail(q: string): string {
  return q.trim().toLowerCase();
}

// 3..254 karakter, tepat satu '@', tanpa spasi, ada '.' setelah '@'.
export function isEmailLike(q: string): boolean {
  const s = q.trim();
  if (s.length < 3 || s.length > MAX_EMAIL || /\s/.test(s)) return false;
  const parts = s.split('@');
  if (parts.length !== 2) return false;
  const [local, domain] = parts as [string, string];
  return local.length > 0 && domain.includes('.') && !domain.startsWith('.') && !domain.endsWith('.');
}

// 'invite' = tawarkan opsi undang; 'member' = email itu sudah anggota; null = tidak ada opsi.
export function inviteOptionFor(q: string, search: Pick<CandidateState, 'status' | 'items'>, members: Pick<Member, 'email'>[]): 'invite' | 'member' | null {
  if (!isEmailLike(q) || search.status !== 'ready') return null;
  const email = normalizeEmail(q);
  if (members.some((m) => (m.email ?? '').toLowerCase() === email)) return 'member';
  if (search.items.some((c) => c.email.toLowerCase() === email)) return null;
  return 'invite';
}

export type InvitationStatus = 'expired' | 'failed' | 'not_configured' | 'pending' | 'sent';

// Satu status per baris; prioritas expired > failed > not_configured > pending > sent.
export function invitationStatus(i: Pick<Invitation, 'expired' | 'delivery_status'>): InvitationStatus {
  return i.expired ? 'expired' : i.delivery_status === 'sent' ? 'sent' : i.delivery_status;
}

// added_at menurun, tie-break user_id naik (sama dengan backend).
export function sortInvitations(items: Invitation[]): Invitation[] {
  return [...items].sort((a, b) => {
    const at = Date.parse(a.added_at);
    const bt = Date.parse(b.added_at);
    if (at !== bt) return bt - at;
    return a.user_id < b.user_id ? -1 : a.user_id > b.user_id ? 1 : 0;
  });
}
