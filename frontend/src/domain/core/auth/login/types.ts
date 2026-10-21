import { get } from 'svelte/store';
import { z } from 'zod';
import { isApiError } from '../../../../shared/services/http';
import { t } from '../../../../shared/stores/i18n';
import { fieldErrorsOf, KnownErrorCode, retryAfterOf } from '../../../../shared/validation/error';
import { MeSchema } from '../getme';

export interface LoginInput {
  email: string;
  password: string;
}

export const LoginResponseSchema = z.object({ user: MeSchema.omit({ email_enabled: true }) });

export const AuthErrorCode = { INVALID_CREDENTIALS: 'INVALID_CREDENTIALS', ACCOUNT_LOCKED: 'ACCOUNT_LOCKED' } as const;

export type LoginErrorView =
  | { kind: 'invalid' }
  | { kind: 'locked'; retryAfterSeconds: number | null }
  | { kind: 'rateLimited'; retryAfterSeconds: number | null }
  | { kind: 'fields'; fields: Record<string, string> }
  | { kind: 'generic'; message: string; traceId?: string };

export function toLoginError(e: unknown): LoginErrorView {
  if (!isApiError(e)) return { kind: 'generic', message: get(t)('error.internal') };
  switch (e.code) {
    case AuthErrorCode.INVALID_CREDENTIALS:
      return { kind: 'invalid' };
    case AuthErrorCode.ACCOUNT_LOCKED:
      return { kind: 'locked', retryAfterSeconds: retryAfterOf(e.details) };
    case KnownErrorCode.RATE_LIMITED:
      return { kind: 'rateLimited', retryAfterSeconds: retryAfterOf(e.details) };
    case KnownErrorCode.INVALID_ARGUMENT: {
      const fields = fieldErrorsOf(e.details);
      if (Object.keys(fields).length > 0) return { kind: 'fields', fields };
      break;
    }
  }
  return { kind: 'generic', message: e.message, traceId: e.traceId };
}
