import { t } from '../../../../../shared/stores/i18n';
import { get } from 'svelte/store';
import { formatDateTime } from '../../../../../shared/utils/datetime';
import type { Locale } from '../../../../../shared/stores/i18n';
import type { ApiKey } from '../types';

const tr = (k: string, p?: Record<string, string | number>) => get(t)(k, p);

export function createdAtLine(key: ApiKey, locale: Locale): string {
  return formatDateTime(key.created_at, locale);
}

export function revokedLine(key: ApiKey, locale: Locale): string {
  return tr('project.apikey.revokedBy', {
    by: key.revoked_by_label ?? tr('project.apikey.unknownUser'),
    at: key.revoked_at ? formatDateTime(key.revoked_at, locale) : '-',
  });
}
