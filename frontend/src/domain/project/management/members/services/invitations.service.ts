import { del, get, isApiError, post } from '../../../../../shared/services/http';
import { fieldErrorsOf, reasonOf } from '../../../../../shared/validation/error';
import { minutesOf, rateLimitOf } from '../../../../../shared/validation/credentials';
import { normalizeEmail } from '../model/invite';
import {
  InvitationListSchema,
  InviteResponseSchema,
  ResendResponseSchema,
  type Invitation,
  type InviteInput,
  type InviteResponse,
  type ResendResponse,
} from '../types';

const base = (projectId: string) => `/api/projects/${encodeURIComponent(projectId)}/invitations`;

export async function listInvitations(projectId: string, signal?: AbortSignal): Promise<Invitation[]> {
  const r = await get(base(projectId), { schema: InvitationListSchema, signal });
  return r.items;
}

export function inviteToProject(projectId: string, input: InviteInput): Promise<InviteResponse> {
  const body: Record<string, unknown> = { email: normalizeEmail(input.email), role: input.role };
  if (input.name.trim() !== '') body.name = input.name.trim();
  return post(base(projectId), body, { schema: InviteResponseSchema });
}

export function resendProjectInvitation(projectId: string, userId: string): Promise<ResendResponse> {
  return post(`${base(projectId)}/${encodeURIComponent(userId)}/resend`, undefined, { schema: ResendResponseSchema });
}

export async function revokeInvitation(projectId: string, userId: string): Promise<void> {
  await del(`${base(projectId)}/${encodeURIComponent(userId)}`);
}

export type InviteErrorView =
  | { kind: 'fields'; fields: Partial<Record<'email' | 'role' | 'name', string>> }
  | { kind: 'alreadyMember' }
  | { kind: 'alreadyRegistered'; reason: 'registered' | 'inactive' | 'not_developer' }
  | { kind: 'notPending' }
  | { kind: 'rateLimited'; scope: 'invite' | 'resend'; minutes: number }
  | { kind: 'lastOwner' }
  | { kind: 'forbidden' }
  | { kind: 'notFound' }
  | { kind: 'generic'; message: string; traceId?: string };

export function toInviteError(e: unknown, scope: 'invite' | 'resend' = 'invite'): InviteErrorView {
  if (!isApiError(e)) return { kind: 'generic', message: String(e) };
  const err = e;
  const reason = reasonOf(err.details);
  switch (err.status) {
    case 400: {
      const f = fieldErrorsOf(err.details);
      const fields: Partial<Record<'email' | 'role' | 'name', string>> = {};
      if (f.email) fields.email = f.email;
      if (f.role) fields.role = f.role;
      if (f.name) fields.name = f.name;
      if (Object.keys(fields).length > 0) return { kind: 'fields', fields };
      break;
    }
    case 403:
      return { kind: 'forbidden' };
    case 404:
      return { kind: 'notFound' };
    case 409:
      if (err.code === 'ALREADY_REGISTERED') {
        const r = reason === 'inactive' || reason === 'not_developer' ? reason : 'registered';
        return { kind: 'alreadyRegistered', reason: r };
      }
      if (reason === 'ALREADY_MEMBER') return { kind: 'alreadyMember' };
      if (reason === 'INVITATION_NOT_PENDING') return { kind: 'notPending' };
      if (reason === 'LAST_OWNER') return { kind: 'lastOwner' };
      break;
    case 429: {
      const rl = rateLimitOf(err.details);
      return rl
        ? { kind: 'rateLimited', scope: rl.scope, minutes: minutesOf(rl.seconds) }
        : { kind: 'rateLimited', scope, minutes: 60 };
    }
  }
  return { kind: 'generic', message: err.message, traceId: err.traceId };
}
