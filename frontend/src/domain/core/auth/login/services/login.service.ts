import { post } from '../../../../../shared/services/http';
import type { Identity } from '../../../../../shared/stores/session';
import { toIdentity } from '../../getme';
import { LoginResponseSchema, type LoginInput } from '../types';

// Email dikirim apa adanya; server yang trim dan lowercase.
export async function login(input: LoginInput, signal?: AbortSignal): Promise<Identity> {
  const res = await post('/api/auth/login', input, { schema: LoginResponseSchema, skipAuthRedirect: true, signal });
  return toIdentity(res.user);
}
