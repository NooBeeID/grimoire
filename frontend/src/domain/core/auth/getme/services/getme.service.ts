import { get } from '../../../../../shared/services/http';
import type { Identity } from '../../../../../shared/stores/session';
import { MeSchema, toIdentity } from '../types';

export async function fetchMe(signal?: AbortSignal): Promise<Identity> {
  const me = await get('/api/auth/me', { schema: MeSchema, skipAuthRedirect: true, signal });
  return toIdentity(me);
}
