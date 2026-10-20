import { post } from '../../../../../shared/services/http';
import type { ChangePasswordInput } from '../types';

// 204 tanpa body; 401 ditangani handler global.
export async function changePassword(input: ChangePasswordInput): Promise<void> {
  await post('/api/auth/change-password', input);
}
