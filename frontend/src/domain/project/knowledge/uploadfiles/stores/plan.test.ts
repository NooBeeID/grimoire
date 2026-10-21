import { describe, expect, it } from 'vitest';
import type { UploadTarget } from '../types';
import { isSupported } from '../../base';
import { planUpload } from './plan';

const file = (path: string, size = 10) => ({ path, file: new File(['x'.repeat(size)], path.split('/').pop()!) });
const knowledge: UploadTarget = { key: 'knowledge', labelKey: 'k', accepts: isSupported, upload: async () => [] };
const spec: UploadTarget = { key: 'spec', labelKey: 's', accepts: (p) => /\.(ya?ml|json)$/i.test(p), upload: async () => [] };

describe('planUpload', () => {
  it('skips hidden files without sending them', () => {
    const p = planUpload([file('.git/config'), file('a/.DS_Store'), file('a.md')], [knowledge], '');
    expect(p.skipped.map((s) => s.path)).toEqual(['.git/config', 'a/.DS_Store']);
    expect(p.byTarget.get('knowledge')!.map((f) => f.path)).toEqual(['a.md']);
  });
  it('reports unsupported types', () => {
    const p = planUpload([file('logo.png'), file('x.markdown'), file('y.txt')], [knowledge], '');
    expect(p.unsupported.map((s) => s.path)).toEqual(['logo.png']);
    expect(p.byTarget.get('knowledge')).toHaveLength(2);
  });
  it('routes yaml to an extra target and prefixes the target dir', () => {
    const p = planUpload([file('openapi.yaml'), file('a.md')], [knowledge, spec], 'docs');
    expect(p.byTarget.get('spec')![0]!.path).toBe('docs/openapi.yaml');
    expect(p.byTarget.get('knowledge')![0]!.path).toBe('docs/a.md');
  });
  it('sums bytes of sendable files only', () => {
    const p = planUpload([file('a.md', 5), file('b.png', 99), file('.x/c.md', 7)], [knowledge], '');
    expect(p.totalBytes).toBe(5);
  });
});
