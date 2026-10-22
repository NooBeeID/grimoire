import { describe, expect, it } from 'vitest';
import { parseRepoUrlLite } from './url';

describe('parseRepoUrlLite', () => {
  it('GitHub tree URL yields branch and folder', () => {
    expect(parseRepoUrlLite('https://github.com/noobee/grimoire-docs/tree/main/docs')).toEqual({
      ok: true,
      provider: 'github',
      host: 'github.com',
      repoPath: 'noobee/grimoire-docs',
      branch: 'main',
      folder: 'docs',
    });
  });
  it('without tree there is no branch', () => {
    const r = parseRepoUrlLite('https://github.com/noobee/grimoire-docs');
    expect(r).toMatchObject({ ok: true, repoPath: 'noobee/grimoire-docs' });
    expect(r).not.toHaveProperty('branch');
  });
  it('drops the .git suffix', () => {
    expect(parseRepoUrlLite('https://github.com/noobee/grimoire-docs.git')).toMatchObject({ repoPath: 'noobee/grimoire-docs' });
  });
  it('blob URL is rejected', () => {
    expect(parseRepoUrlLite('https://github.com/noobee/grimoire-docs/blob/main/a.md')).toEqual({ ok: false, problem: 'blob_url' });
  });
  it('http is https_only, empty is required', () => {
    expect(parseRepoUrlLite('http://github.com/a/b')).toEqual({ ok: false, problem: 'https_only' });
    expect(parseRepoUrlLite('  ')).toEqual({ ok: false, problem: 'required' });
  });
  it('userinfo is flagged and the secret is not echoed', () => {
    const r = parseRepoUrlLite('https://ghp_secret@github.com/noobee/grimoire-docs');
    expect(r).toEqual({ ok: false, problem: 'userinfo' });
    expect(JSON.stringify(r)).not.toContain('ghp_secret');
  });
  it('query and fragment are flagged', () => {
    expect(parseRepoUrlLite('https://github.com/a/b?x=1')).toEqual({ ok: false, problem: 'query' });
    expect(parseRepoUrlLite('https://github.com/a/b#frag')).toEqual({ ok: false, problem: 'query' });
  });
  it('GitLab nested group with /-/tree', () => {
    expect(parseRepoUrlLite('https://gitlab.com/grp/sub/repo/-/tree/main/api')).toEqual({
      ok: true,
      provider: 'gitlab',
      host: 'gitlab.com',
      repoPath: 'grp/sub/repo',
      branch: 'main',
      folder: 'api',
    });
  });
  it('self-hosted host keeps the port', () => {
    expect(parseRepoUrlLite('https://git.noobee.id:8443/grp/repo')).toMatchObject({ provider: 'gitlab', host: 'git.noobee.id:8443', repoPath: 'grp/repo' });
  });
});
