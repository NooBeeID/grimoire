import { get } from 'svelte/store';
import { isApiError } from '../../../../shared/services/http';
import { t } from '../../../../shared/stores/i18n';
import { fieldErrorsOf, KnownErrorCode, retryAfterOf } from '../../../../shared/validation/error';

export interface ChangePasswordInput {
  current_password: string;
  new_password: string;
}

export type ChangePasswordErrorView =
  | { kind: 'fields'; fields: Record<string, string> }
  | { kind: 'conflict' }
  | { kind: 'rateLimited'; retryAfterSeconds: number | null }
  | { kind: 'generic'; message: string; traceId?: string };

export function toChangePasswordError(e: unknown): ChangePasswordErrorView {
  if (!isApiError(e)) return { kind: 'generic', message: get(t)('error.internal') };
  if (e.code === KnownErrorCode.CONFLICT) return { kind: 'conflict' };
  if (e.code === KnownErrorCode.RATE_LIMITED) return { kind: 'rateLimited', retryAfterSeconds: retryAfterOf(e.details) };
  if (e.code === KnownErrorCode.INVALID_ARGUMENT) {
    const fields = fieldErrorsOf(e.details);
    if (Object.keys(fields).length > 0) return { kind: 'fields', fields };
  }
  return { kind: 'generic', message: e.message, traceId: e.traceId };
}
