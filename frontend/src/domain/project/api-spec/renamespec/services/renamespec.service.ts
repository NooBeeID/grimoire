import { patch } from '../../../../../shared/services/http';
import { RenameResponseSchema, type ApiSpec } from '../types';

export async function renameSpec(projectId: string, name: string, newName: string): Promise<ApiSpec> {
  const r = await patch(
    `/api/projects/${encodeURIComponent(projectId)}/api-specs/${encodeURIComponent(name)}`,
    { name: newName },
    { schema: RenameResponseSchema },
  );
  return r.spec;
}
