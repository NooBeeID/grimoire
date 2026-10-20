import { get } from 'svelte/store';
import { z } from 'zod';
import { isApiError } from '../../../../shared/services/http';
import { t } from '../../../../shared/stores/i18n';
import { fieldErrorsOf, KnownErrorCode, reasonOf } from '../../../../shared/validation/error';

export const RoleSchema = z.enum(['super_admin', 'developer']);
export const UserSchema = z.object({
  id: z.string().uuid(),
  email: z.string(),
  name: z.string(),
  role: RoleSchema,
  is_active: z.boolean(),
  must_change_password: z.boolean(),
  created_at: z.string(),
  created_by: z.string().uuid().nullable(),
  created_by_label: z.string().nullable(),
  updated_at: z.string(),
  last_login_at: z.string().nullable(),
});
export const UserListSchema = z.object({
  items: z.array(UserSchema),
  total: z.number().int().nonnegative(),
  limit: z.number().int().positive(),
  offset: z.number().int().nonnegative(),
});
export const UserWithTempPasswordSchema = z.object({ user: UserSchema, temporary_password: z.string().min(1).nullable() });

export type Role = z.infer<typeof RoleSchema>;
export type User = z.infer<typeof UserSchema>;
export type UserList = z.infer<typeof UserListSchema>;
export type UserWithTempPassword = z.infer<typeof UserWithTempPasswordSchema>;

export type StatusFilter = '' | 'active' | 'inactive';
export interface UsersFilters {
  role: '' | Role;
  status: StatusFilter;
  q: string;
}
export const UserErrorReason = { SELF_ACTION_FORBIDDEN: 'SELF_ACTION_FORBIDDEN', LAST_SUPER_ADMIN: 'LAST_SUPER_ADMIN' } as const;
export const PAGE_SIZE = 20;

export interface CreateUserInput {
  email: string;
  name: string;
  role: Role;
  password?: string;
}

export type UserFormError =
  | { kind: 'form'; fields: Record<string, string>; message?: string }
  | { kind: 'notFound' }
  | { kind: 'generic'; error: unknown };

export function toUserFormError(e: unknown, ctx?: 'reset'): UserFormError {
  if (!isApiError(e)) return { kind: 'generic', error: e };
  const tr = get(t);
  const reason = reasonOf(e.details);
  switch (e.status) {
    case 400: {
      const all = fieldErrorsOf(e.details);
      const { _: formMsg, ...fields } = all;
      if (formMsg !== undefined || Object.keys(fields).length > 0) return { kind: 'form', fields, message: formMsg };
      break;
    }
    case 409:
      if (reason === UserErrorReason.LAST_SUPER_ADMIN) return { kind: 'form', fields: {}, message: tr('users.error.lastSuperAdmin') };
      return { kind: 'form', fields: { email: tr('users.error.emailTaken') } };
    case 403:
      if (e.code === KnownErrorCode.FORBIDDEN && reason === UserErrorReason.SELF_ACTION_FORBIDDEN) {
        return { kind: 'form', fields: {}, message: tr(ctx === 'reset' ? 'users.error.selfReset' : 'users.error.selfAction') };
      }
      return { kind: 'form', fields: {}, message: tr('error.forbidden') };
    case 404:
      return { kind: 'notFound' };
  }
  return { kind: 'generic', error: e };
}
