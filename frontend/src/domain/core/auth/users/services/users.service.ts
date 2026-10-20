import { get, patch, post } from '../../../../../shared/services/http';
import {
  UserListSchema,
  UserSchema,
  UserWithTempPasswordSchema,
  type CreateUserInput,
  type Role,
  type StatusFilter,
  type User,
  type UserList,
  type UserWithTempPassword,
} from '../types';

const uid = (id: string) => encodeURIComponent(id);

export interface ListUsersParams {
  role: '' | Role;
  status: StatusFilter;
  q: string;
  limit: number;
  offset: number;
}

export function listUsers(p: ListUsersParams, signal?: AbortSignal): Promise<UserList> {
  const q = p.q.trim();
  return get('/api/users', {
    schema: UserListSchema,
    signal,
    query: { role: p.role || undefined, status: p.status || undefined, q: q || undefined, limit: p.limit, offset: p.offset },
  });
}

export function createUser(input: CreateUserInput): Promise<UserWithTempPassword> {
  const body: Record<string, unknown> = { email: input.email, name: input.name, role: input.role };
  if (input.password) body.password = input.password;
  return post('/api/users', body, { schema: UserWithTempPasswordSchema });
}

export function updateUser(id: string, changes: Partial<{ name: string; email: string; role: Role }>): Promise<User> {
  if (Object.keys(changes).length === 0) throw new Error('updateUser: patch kosong');
  return patch(`/api/users/${uid(id)}`, changes, { schema: UserSchema });
}

export function setUserStatus(id: string, active: boolean): Promise<User> {
  return post(`/api/users/${uid(id)}/${active ? 'activate' : 'deactivate'}`, undefined, { schema: UserSchema });
}

export function resetUserPassword(id: string, password?: string): Promise<UserWithTempPassword> {
  return post(`/api/users/${uid(id)}/reset-password`, password ? { password } : {}, { schema: UserWithTempPasswordSchema });
}
