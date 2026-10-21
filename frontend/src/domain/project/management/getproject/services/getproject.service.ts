import { get } from '../../../../../shared/services/http';
import { ProjectDetailSchema, type ProjectDetail } from '../types';

// Id tidak divalidasi di klien: backend menjawab 404 untuk bukan-uuid (tidak membocorkan format).
export function getProject(id: string, signal?: AbortSignal): Promise<ProjectDetail> {
  return get('/api/projects/' + encodeURIComponent(id), { schema: ProjectDetailSchema, signal });
}
