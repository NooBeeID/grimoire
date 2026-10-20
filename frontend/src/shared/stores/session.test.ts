import { get } from 'svelte/store';
import { describe, expect, it } from 'vitest';
import { resetSession, session, setIdentity } from './session';

describe('session', () => {
  it('transitions', () => {
    expect(get(session).status).toBe('unknown');
    setIdentity({ userId: '1', email: 'a@b.c', name: 'A', role: 'developer', mustChangePassword: false });
    expect(get(session).status).toBe('authenticated');
    setIdentity(null);
    expect(get(session)).toEqual({ status: 'anonymous', identity: null });
    resetSession();
    expect(get(session).status).toBe('unknown');
  });
});
