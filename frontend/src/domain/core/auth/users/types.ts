import { get } from 'svelte/store';
import { z } from 'zod';
import { isApiError } from '../../../../shared/services/http';
import { t } from '../../../../shared/stores/i18n';
import { EmailStatusSchema, minutesOf, rateLimitOf, TemporaryCredentialsSchema } from '../../../../shared/validation/credentials';
import { fieldErrorsOf, KnownErrorCode, reasonOf } from '../../../../shared/validation/error';

export const RoleSchema = z.enum(['super_admin', 'developer']);
// Mengikuti components/schemas/UserInvitation di api/modules/core/auth.yml (kontrak T12).
export const UserInvitationSchema = z.object({
  pending: z.literal(true),
  invited_at: z.string(),
  sent_at: z.string().nullable(),
  expires_at: z.string(),
  expired: z.boolean(),
  delivery_status: EmailStatusSchema,
  resend_count: z.number().int().nonnegative(),
});
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
  // Kunci wajib di semua respons user (null bila bukan akun undangan).
  invitation: UserInvitationSchema.nullable(),
});
export const UserListSchema = z.object({
  items: z.array(UserSchema),
  total: z.number().int().nonnegative(),
  limit: z.number().int().positive(),
  offset: z.number().int().nonnegative(),
});
export const UserWithTempPasswordSchema = z.object({ user: UserSchema, temporary_password: z.string().min(1).nullable() });

export const InviteUserResponseSchema = z.object({
  user: UserSchema,
  created: z.boolean(),
  email_sent: z.boolean(),
  email_status: EmailStatusSchema,
  expires_at: z.string(),
  credentials: TemporaryCredentialsSchema.optional(),
});
export const ResendUserInvitationResponseSchema = InviteUserResponseSchema.pick({
  email_sent: true,
  email_status: true,
  expires_at: true,
  credentials: true,
});
export type UserInvitation = z.infer<typeof UserInvitationSchema>;
export type InviteUserResponse = z.infer<typeof InviteUserResponseSchema>;
export type ResendUserInvitationResponse = z.infer<typeof ResendUserInvitationResponseSchema>;
export interface InviteUserInput {
  email: string;
  name: string;
}

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
  | { kind: 'form'; fields: Record<string, string>; message?: string; reason?: string }
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
      if (e.code === 'ALREADY_REGISTERED') {
        const r = reason === 'inactive' || reason === 'not_developer' ? reason : 'registered';
        return { kind: 'form', fields: { email: tr(`invitation.error.alreadyRegistered.${r}`) }, reason: r };
      }
      if (reason === 'INVITATION_NOT_PENDING') return { kind: 'form', fields: {}, message: tr('invitation.error.notPending') };
      if (reason === UserErrorReason.LAST_SUPER_ADMIN) return { kind: 'form', fields: {}, message: tr('users.error.lastSuperAdmin') };
      return { kind: 'form', fields: { email: tr('users.error.emailTaken') } };
    case 403:
      if (e.code === KnownErrorCode.FORBIDDEN && reason === UserErrorReason.SELF_ACTION_FORBIDDEN) {
        return { kind: 'form', fields: {}, message: tr(ctx === 'reset' ? 'users.error.selfReset' : 'users.error.selfAction') };
      }
      return { kind: 'form', fields: {}, message: tr('error.forbidden') };
    case 404:
      return { kind: 'notFound' };
    case 429: {
      const rl = rateLimitOf(e.details);
      const scope = rl?.scope ?? 'invite';
      return { kind: 'form', fields: {}, message: tr(`invitation.error.rateLimited.${scope}`, { minutes: minutesOf(rl?.seconds ?? 3600) }) };
    }
  }
  return { kind: 'generic', error: e };
}
