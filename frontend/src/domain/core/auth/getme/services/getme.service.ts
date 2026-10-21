import { get } from '../../../../../shared/services/http';
import type { Identity } from '../../../../../shared/stores/session';
import { MeSchema, toIdentity, type Me } from '../types';

export function getMe(signal?: AbortSignal): Promise<Me> {
  return get('/api/auth/me', { schema: MeSchema, skipAuthRedirect: true, signal });
}

export async function fetchMe(signal?: AbortSignal): Promise<Identity> {
  return toIdentity(await getMe(signal));
}
