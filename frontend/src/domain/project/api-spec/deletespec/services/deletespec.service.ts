import { del } from '../../../../../shared/services/http';

export function deleteSpec(projectId: string, name: string): Promise<void> {
  return del(`/api/projects/${encodeURIComponent(projectId)}/api-specs/${encodeURIComponent(name)}`);
}
