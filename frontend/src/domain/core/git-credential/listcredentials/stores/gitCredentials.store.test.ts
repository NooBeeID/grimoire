import { get } from 'svelte/store';
import { describe, expect, it } from 'vitest';
import { ApiError } from '../../../../../shared/services/http';
import { mockCredentialList } from '../mocks/credentials';
import { createGitCredentialsStore } from './gitCredentials.store';

const err = new ApiError({ status: 500, code: 'INTERNAL', message: 'x' });

describe('gitCredentials store', () => {
  it('same user keeps previous while reloading', () => {
    const s = createGitCredentialsStore();
    s.begin('u1');
    s.resolve('u1', mockCredentialList());
    s.begin('u1');
    const st = get(s);
    expect(st.status === 'loading' && st.previous?.items.length).toBe(2);
  });
  it('different user drops previous', () => {
    const s = createGitCredentialsStore();
    s.begin('u1');
    s.resolve('u1', mockCredentialList());
    s.begin('u2');
    const st = get(s);
    expect(st.status === 'loading' && st.previous).toBeNull();
  });
  it('ignores a stale user result', () => {
    const s = createGitCredentialsStore();
    s.begin('u1');
    s.begin('u2');
    s.resolve('u1', mockCredentialList());
    expect(get(s).status).toBe('loading');
    s.fail('u1', err);
    expect(get(s).status).toBe('loading');
  });
  it('error keeps previous', () => {
    const s = createGitCredentialsStore();
    s.begin('u1');
    s.resolve('u1', mockCredentialList());
    s.begin('u1');
    s.fail('u1', err);
    const st = get(s);
    expect(st.status === 'error' && st.previous).not.toBeNull();
  });
  it('invalidate bumps version; reset -> idle', () => {
    const s = createGitCredentialsStore();
    s.invalidate();
    expect(get(s.version)).toBe(1);
    s.begin('u1');
    s.reset();
    expect(get(s).status).toBe('idle');
  });
  it('never exposes a token property', () => {
    const s = createGitCredentialsStore();
    s.begin('u1');
    s.resolve('u1', mockCredentialList());
    expect(JSON.stringify(get(s))).not.toContain('"token"');
  });
});
