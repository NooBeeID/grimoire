// Data mock sesuai skema SynonymGroupList (api/modules/project/search-synonym.yml).
import type { SynonymGroup } from '../types';

export const GROUP_ID = (n: number) => `0192f5a0-3333-7aaa-8bbb-${String(n).padStart(12, '0')}`;

export const mockGroup = (n: number, over: Partial<SynonymGroup> = {}): SynonymGroup => ({
  id: GROUP_ID(n),
  terms: ['login', 'autentikasi', 'masuk'],
  created_at: '2026-10-23T10:00:00Z',
  updated_at: '2026-10-23T10:00:00Z',
  created_by_label: 'ana@tim.id',
  updated_by_label: 'ana@tim.id',
  ...over,
});

export const mockGroups = (): SynonymGroup[] => [
  mockGroup(1),
  mockGroup(2, {
    terms: ['deploy', 'rilis', 'peluncuran'],
    created_at: '2026-10-23T11:20:00Z',
    updated_at: '2026-10-24T09:05:00Z',
    updated_by_label: 'budi@tim.id',
  }),
  mockGroup(3, { terms: ['pengiriman', 'delivery'], created_at: '2026-10-24T08:30:00Z', updated_at: '2026-10-24T08:30:00Z', created_by_label: 'budi@tim.id', updated_by_label: 'budi@tim.id' }),
];

export const mockGroupList = (...items: SynonymGroup[]) => ({ items: items.length ? items : mockGroups() });
