import type { Translate } from '../../../../shared/stores/i18n';
import { isApiError } from '../../../../shared/services/http';
import { reasonOf } from '../../../../shared/validation/error';
import { formatBytes } from '../../../../shared/utils/bytes';
import { REASONS, TooLargeDetailsSchema } from './types';

type Tr = Translate;

export function reasonMessageKey(reason: string | null): string | null {
  return reason !== null && (REASONS as readonly string[]).includes(reason) ? `knowledge.reason.${reason}` : null;
}

export function errorMessage(e: unknown, tr: Tr): string {
  if (!isApiError(e)) return tr('error.internal');
  const key = reasonMessageKey(reasonOf(e.details));
  if (key) {
    const big = TooLargeDetailsSchema.safeParse(e.details);
    return tr(key, { max: big.success ? formatBytes(big.data.max_bytes) : "" }).trim();
  }
  return e.message;
}

export const isNotFound = (e: unknown): boolean => isApiError(e) && e.status === 404;
