// Data mock sesuai skema MemberList / MemberCandidateList (api/modules/project/management.yml).
const uid = (n: number) => `0192f5a0-cccc-7aaa-8bbb-${String(n).padStart(12, '0')}`;

export const mockMember = (n: number, over: Record<string, unknown> = {}) => ({
  user_id: uid(n),
  name: `User ${n}`,
  email: `user${n}@tim.id`,
  is_active: true,
  found: true,
  role: 'member' as 'owner' | 'member',
  added_at: '2026-10-22T09:00:00Z',
  added_by: uid(1),
  added_by_label: 'user1@tim.id',
  ...over,
});
export const mockMemberList = (...items: ReturnType<typeof mockMember>[]) => ({ items });
export const mockCandidate = (n: number, over: Record<string, unknown> = {}) => ({
  id: uid(100 + n),
  name: `Budi ${n}`,
  email: `budi${n}@tim.id`,
  ...over,
});
export const mockCandidateList = (...items: ReturnType<typeof mockCandidate>[]) => ({ items });
export const memberId = uid;
