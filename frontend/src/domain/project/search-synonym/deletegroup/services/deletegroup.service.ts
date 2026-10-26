import { del } from '../../../../../shared/services/http';

export async function deleteGroup(projectId: string, groupId: string): Promise<void> {
  await del(`/api/projects/${encodeURIComponent(projectId)}/search/synonyms/${encodeURIComponent(groupId)}`);
}
