// Data mock sesuai skema KnowledgeTree (api/modules/project/knowledge.yml).
import type { TreeLevel } from '../types';

const T = '2026-10-21T09:30:00Z';
export const rootLevel: TreeLevel = {
  prefix: '',
  folders: [{ name: 'docs', path: 'docs', file_count: 4 }],
  files: [{ name: 'README.md', path: 'README.md', size_bytes: 320, updated_at: T }],
  total_files: 5,
};
export const docsLevel: TreeLevel = {
  prefix: 'docs',
  folders: [{ name: 'auth', path: 'docs/auth', file_count: 2 }],
  files: [
    { name: 'arsitektur.md', path: 'docs/arsitektur.md', size_bytes: 2048, updated_at: T },
    { name: 'catatan.txt', path: 'docs/catatan.txt', size_bytes: 90, updated_at: T },
  ],
  total_files: 4,
};
export const authLevel: TreeLevel = {
  prefix: 'docs/auth',
  folders: [],
  files: [
    { name: 'login.md', path: 'docs/auth/login.md', size_bytes: 1200, updated_at: T },
    { name: 'sesi.md', path: 'docs/auth/sesi.md', size_bytes: 800, updated_at: T },
  ],
  total_files: 2,
};
export const emptyRoot: TreeLevel = { prefix: '', folders: [], files: [], total_files: 0 };
