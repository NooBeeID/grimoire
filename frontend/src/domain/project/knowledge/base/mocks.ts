// Data mock sesuai skema api/modules/project/knowledge.yml; dipakai test dan scaffolding.
import type { KnowledgeFile } from './types';

export const V1 = 'a'.repeat(64);
export const V2 = 'b'.repeat(64);
const user = { id: '0192f5a0-aaaa-7aaa-8bbb-00000000000a', label: 'ana@tim.id' };

export const mockFile = (path: string, over: Partial<KnowledgeFile> = {}): KnowledgeFile => ({
  path,
  title: 'Alur login',
  content: '# Alur login\n\nPengguna masuk dengan email dan kata sandi.\n',
  size_bytes: 61,
  version: V1,
  created_at: '2026-10-20T08:00:00Z',
  created_by: user,
  updated_at: '2026-10-21T09:30:00Z',
  updated_by: user,
  ...over,
});
