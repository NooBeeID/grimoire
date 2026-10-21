import { del, get, patch, post, type ApiError } from '../../../../../shared/services/http';
import { truncateRunes } from '../../../../../shared/utils/text';
import { reasonOf } from '../../../../../shared/validation/error';
import {
  CANDIDATE_LIMIT,
  CandidateListSchema,
  KNOWN_REASONS,
  MAX_CANDIDATE_QUERY,
  MemberListSchema,
  MemberSchema,
  type Candidate,
  type Member,
  type MemberRole,
} from '../types';

const base = (projectId: string) => `/api/projects/${encodeURIComponent(projectId)}`;

export async function listMembers(projectId: string, signal?: AbortSignal): Promise<Member[]> {
  const r = await get(`${base(projectId)}/members`, { schema: MemberListSchema, signal });
  return r.items;
}

export async function searchCandidates(projectId: string, q: string, signal?: AbortSignal): Promise<Candidate[]> {
  const query = truncateRunes(q.trim(), MAX_CANDIDATE_QUERY);
  if (query === '') return [];
  const r = await get(`${base(projectId)}/member-candidates`, {
    query: { q: query, limit: CANDIDATE_LIMIT },
    schema: CandidateListSchema,
    signal,
  });
  return r.items;
}

export function addMember(projectId: string, userId: string, role: MemberRole): Promise<Member> {
  return post(`${base(projectId)}/members`, { user_id: userId, role }, { schema: MemberSchema });
}

export async function removeMember(projectId: string, userId: string): Promise<void> {
  await del(`${base(projectId)}/members/${encodeURIComponent(userId)}`);
}

export function changeMemberRole(projectId: string, userId: string, role: MemberRole): Promise<Member> {
  return patch(`${base(projectId)}/members/${encodeURIComponent(userId)}`, { role }, { schema: MemberSchema });
}

// Kunci i18n untuk error yang dikenal; null bila tidak dikenal.
export function memberErrorKey(e: ApiError): string | null {
  const reason = reasonOf(e.details);
  if (reason && (KNOWN_REASONS as readonly string[]).includes(reason)) return `project.error.${reason}`;
  if (e.status === 403) return 'error.forbidden';
  if (e.status === 404) return 'project.members.error.gone';
  return null;
}
