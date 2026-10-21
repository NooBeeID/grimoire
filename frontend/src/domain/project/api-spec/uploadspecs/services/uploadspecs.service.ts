import { post } from '../../../../../shared/services/http';
import {
  ApiSpecUploadResultSchema,
  ApiSpecValidateResultSchema,
  type ApiSpecUploadResult,
  type ApiSpecValidateResult,
  type PickedFile,
} from '../types';

const enc = encodeURIComponent;

export function toForm(files: PickedFile[], name?: string): FormData {
  const f = new FormData();
  for (const p of files) f.append('files', p.file, p.path);
  if (name) f.append('name', name);
  return f;
}

export const uploadSpecs = (projectId: string, files: PickedFile[], name?: string, signal?: AbortSignal): Promise<ApiSpecUploadResult> =>
  post(`/api/projects/${enc(projectId)}/api-specs/upload`, toForm(files, name), { schema: ApiSpecUploadResultSchema, signal });

export const validateSpecs = (projectId: string, files: PickedFile[], name?: string, signal?: AbortSignal): Promise<ApiSpecValidateResult> =>
  post(`/api/projects/${enc(projectId)}/api-specs/validate`, toForm(files, name), { schema: ApiSpecValidateResultSchema, signal });
