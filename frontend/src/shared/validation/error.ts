import { z } from 'zod';

// Mengikuti components/schemas/Error di api/modules/infra/foundation.yml.
export const ErrorEnvelope = z.object({
  error: z.object({
    code: z.string().min(1),
    message: z.string(),
    details: z.unknown().nullable().optional(),
  }),
  trace_id: z.string().optional(),
});
export type ErrorEnvelope = z.infer<typeof ErrorEnvelope>;

export const HealthStatus = z.object({ status: z.enum(['ok', 'unavailable']) });
export type HealthStatus = z.infer<typeof HealthStatus>;

export const KnownErrorCode = {
  NOT_FOUND: 'NOT_FOUND',
  FORBIDDEN: 'FORBIDDEN',
  CONFLICT: 'CONFLICT',
  INVALID_ARGUMENT: 'INVALID_ARGUMENT',
  UNAUTHORIZED: 'UNAUTHORIZED',
  CSRF_REQUIRED: 'CSRF_REQUIRED',
  RATE_LIMITED: 'RATE_LIMITED',
  PAYLOAD_TOO_LARGE: 'PAYLOAD_TOO_LARGE',
  METHOD_NOT_ALLOWED: 'METHOD_NOT_ALLOWED',
  TIMEOUT: 'TIMEOUT',
  UNAVAILABLE: 'UNAVAILABLE',
  INTERNAL: 'INTERNAL',
  PASSWORD_CHANGE_REQUIRED: 'PASSWORD_CHANGE_REQUIRED',
  NETWORK: 'NETWORK',
  INVALID_RESPONSE: 'INVALID_RESPONSE',
} as const;

export const FieldErrors = z.object({ fields: z.record(z.string()) });

export function fieldErrorsOf(details: unknown): Record<string, string> {
  const r = FieldErrors.safeParse(details);
  return r.success ? r.data.fields : {};
}

export const RetryAfterDetails = z.object({ retry_after_seconds: z.number().int().nonnegative() });
export function retryAfterOf(details: unknown): number | null {
  const r = RetryAfterDetails.safeParse(details);
  return r.success ? r.data.retry_after_seconds : null;
}

export const ReasonDetails = z.object({ reason: z.string() });
export function reasonOf(details: unknown): string | null {
  const r = ReasonDetails.safeParse(details);
  return r.success ? r.data.reason : null;
}
