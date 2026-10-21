import type { User } from '../types';

// null bila bukan akun undangan. 'failed' mencakup failed dan not_configured. Prioritas expired > failed > pending.
export function userInvitationBadge(u: Pick<User, 'invitation'>): 'expired' | 'failed' | 'pending' | null {
  const i = u.invitation;
  if (!i) return null;
  if (i.expired) return 'expired';
  if (i.delivery_status === 'failed' || i.delivery_status === 'not_configured') return 'failed';
  return 'pending';
}
