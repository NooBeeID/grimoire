import { post } from '../../../../../shared/services/http';

export async function logoutRequest(): Promise<void> {
  await post('/api/auth/logout', undefined, { skipAuthRedirect: true });
}
