import { getBlob } from '../../../../../shared/services/http';

export async function downloadSpec(
  projectId: string,
  name: string,
  format: 'yaml' | 'json',
  signal?: AbortSignal,
): Promise<{ blob: Blob; fileName: string }> {
  const { blob } = await getBlob(`/api/projects/${encodeURIComponent(projectId)}/api-specs/${encodeURIComponent(name)}/raw`, { signal });
  return { blob, fileName: name + (format === 'json' ? '.json' : '.yaml') };
}
