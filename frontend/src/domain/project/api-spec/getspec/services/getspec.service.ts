import { ApiError, get } from '../../../../../shared/services/http';
import { get as readStore } from 'svelte/store';
import { t } from '../../../../../shared/stores/i18n';
import { adaptSwagger2, isSwagger2 } from '../model/swagger2';
import { ApiSpecDetailSchema, type ApiSpecDetail, type OpenApiDocument } from '../types';

export async function getSpec(projectId: string, name: string, signal?: AbortSignal): Promise<ApiSpecDetail> {
  const wire = await get(`/api/projects/${encodeURIComponent(projectId)}/api-specs/${encodeURIComponent(name)}`, {
    schema: ApiSpecDetailSchema,
    signal,
  });
  if (!isSwagger2(wire.document)) return wire as ApiSpecDetail; // 3.x: referensi sama
  try {
    return { ...wire, document: adaptSwagger2(wire.document) as OpenApiDocument };
  } catch {
    throw new ApiError({ status: 0, code: 'INVALID_RESPONSE', message: readStore(t)('error.invalid_response') });
  }
}
