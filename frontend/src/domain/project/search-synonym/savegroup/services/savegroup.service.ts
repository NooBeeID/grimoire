import { post, put } from '../../../../../shared/services/http';
import { SynonymGroupSchema, type SynonymGroup } from '../../listgroups';

const base = (projectId: string) => `/api/projects/${encodeURIComponent(projectId)}/search/synonyms`;

export function createGroup(projectId: string, terms: string[]): Promise<SynonymGroup> {
  return post(base(projectId), { terms }, { schema: SynonymGroupSchema });
}

export function updateGroup(projectId: string, groupId: string, terms: string[]): Promise<SynonymGroup> {
  return put(`${base(projectId)}/${encodeURIComponent(groupId)}`, { terms }, { schema: SynonymGroupSchema });
}
