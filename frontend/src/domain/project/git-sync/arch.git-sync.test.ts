import { describe, expect, it } from 'vitest';

const sources = import.meta.glob<string>('./**/*.{ts,svelte}', { query: '?raw', import: 'default', eager: true });
const files = Object.entries(sources).filter(([f]) => !f.endsWith('arch.git-sync.test.ts'));

// Batas module (di luar jangkauan depcruise): git-sync tidak mengenal git-credential, knowledge, maupun api-spec.
describe('git-sync module boundary', () => {
  it('has source files to scan', () => expect(files.length).toBeGreaterThan(10));
  it.each(files)('%s does not import core/git-credential, project/knowledge or project/api-spec', (_f, src) => {
    const imports = [...src.matchAll(/from\s+['"]([^'"]+)['"]/g)].map((m) => m[1]!);
    const bad = imports.filter((i) => /domain\/core\/|\/git-credential\b|domain\/project\/(knowledge|api-spec)/.test(i));
    expect(bad).toEqual([]);
  });
});
