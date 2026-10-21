// Data mock sesuai skema ProjectDetail (GET /api/projects/{projectID}).
export const mockDetail = (over: Record<string, unknown> = {}) => ({
  id: '0192f5a0-1111-7aaa-8bbb-000000000001',
  name: 'Payments',
  description: 'Layanan pembayaran\ninternal',
  created_by: '0192f5a0-aaaa-7aaa-8bbb-00000000000a',
  created_by_label: 'a@tim.id',
  created_at: '2026-10-22T09:00:00Z',
  updated_at: '2026-10-22T10:15:00Z',
  my_role: 'owner' as 'owner' | 'member' | null,
  access_level: 'owner' as 'owner' | 'member' | 'admin',
  is_member: true,
  member_count: 2,
  counts: { files: null, specs: null, keys: null } as { files: number | null; specs: number | null; keys: number | null },
  owner_count: 1,
  ...over,
});
