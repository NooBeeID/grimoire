import { get, type ApiError } from '../../../../../shared/services/http';
import { AuditEntryDetailSchema, type AuditEntryDetail, type DetailError } from '../types';

export { AuditEntryDetailSchema };

export function getAuditEntry(id: string, signal?: AbortSignal): Promise<AuditEntryDetail> {
  return get(`/api/audit-log/${encodeURIComponent(id)}`, { schema: AuditEntryDetailSchema, signal });
}

export function toDetailError(e: ApiError): DetailError {
  if (e.status === 404) return 'notFound';
  if (e.status === 403) return 'forbidden';
  return 'load';
}
