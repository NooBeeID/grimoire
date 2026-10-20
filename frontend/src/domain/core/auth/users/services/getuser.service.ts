import { get } from '../../../../../shared/services/http';
import { UserSchema, type User } from '../types';

export function fetchUser(id: string, signal?: AbortSignal): Promise<User> {
  return get(`/api/users/${encodeURIComponent(id)}`, { schema: UserSchema, signal });
}
