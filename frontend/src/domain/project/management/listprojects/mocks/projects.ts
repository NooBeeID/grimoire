// Data mock sesuai skema ProjectList (api/modules/project/management.yml).
export const mockProject = (n: number, over: Record<string, unknown> = {}) => ({
  id: `0192f5a0-1111-7aaa-8bbb-${String(n).padStart(12, '0')}`,
  name: `Project ${n}`,
  description: 'Layanan pembayaran internal',
  created_by: '0192f5a0-aaaa-7aaa-8bbb-00000000000a',
  created_by_label: 'a@tim.id',
  created_at: '2026-10-22T09:00:00Z',
  updated_at: '2026-10-22T10:15:00Z',
  my_role: 'member' as 'owner' | 'member' | null,
  access_level: 'member' as 'owner' | 'member' | 'admin',
  is_member: true,
  member_count: 2,
  counts: { files: null, specs: null, keys: null } as { files: number | null; specs: number | null; keys: number | null },
  ...over,
});

export const mockProjectList = (count: number, total = count, offset = 0) => ({
  items: Array.from({ length: count }, (_, i) => mockProject(offset + i + 1)),
  total,
  limit: 20,
  offset,
});
