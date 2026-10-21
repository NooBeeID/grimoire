// Data mock sesuai skema KnowledgeFile (api/modules/project/knowledge.yml).
import { mockFile, V1 } from '../../base';
import type { KnowledgeFile } from '../types';

export const loginDoc: KnowledgeFile = mockFile('docs/auth/login.md', {
  title: 'Alur login',
  content: '# Alur login\n\nPengguna masuk dengan email dan kata sandi.\n\n```mermaid\nflowchart LR\n  A[Form] --> B[Sesi]\n```\n',
  version: V1,
});
export const notesTxt: KnowledgeFile = mockFile('docs/catatan.txt', {
  title: '',
  content: '# bukan heading\nrapat arsitektur, 21 Oktober',
});
export const emptyDoc: KnowledgeFile = mockFile('docs/kosong.md', { title: '', content: '', size_bytes: 0 });
export const hostileDoc: KnowledgeFile = mockFile('docs/xss.md', {
  title: '<img src=x onerror=alert(1)>',
  content: '# xss\n\n<script>alert(1)</script>\n\n[klik](javascript:alert(1))\n',
});
