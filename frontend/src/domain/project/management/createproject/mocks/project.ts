// Data mock sesuai skema ProjectSummary hasil POST /api/projects (201).
export const mockCreated = (over: Record<string, unknown> = {}) => ({
  id: '0192f5a0-1111-7aaa-8bbb-000000000001',
  name: 'Payments',
  description: 'Layanan pembayaran internal',
  created_by: '0192f5a0-aaaa-7aaa-8bbb-00000000000a',
  created_by_label: 'a@tim.id',
  created_at: '2026-10-22T09:00:00Z',
  updated_at: '2026-10-22T09:00:00Z',
  my_role: 'owner',
  access_level: 'owner',
  is_member: true,
  member_count: 1,
  counts: { files: 0, specs: 0, keys: 0 },
  ...over,
});
