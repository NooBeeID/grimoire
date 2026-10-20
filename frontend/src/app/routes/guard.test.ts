import { describe, expect, it } from 'vitest';
import type { Component } from 'svelte';
import type { SessionState } from '../../shared/stores/session';
import { resolve } from './guard';
import type { Match, RouteMeta } from './router';

const view = (() => {}) as unknown as Component;
const m = (meta?: RouteMeta): Match => ({ route: { path: '/', view, meta }, params: {} });
const unknown: SessionState = { status: 'unknown', identity: null };
const anon: SessionState = { status: 'anonymous', identity: null };
const auth = (role: 'developer' | 'super_admin'): SessionState => ({
  status: 'authenticated',
  identity: { userId: '1', email: 'a@b.c', name: 'A', role, mustChangePassword: false },
});

describe('guard', () => {
  it('null -> notfound', () => expect(resolve(null, unknown, '/').kind).toBe('notfound'));
  it('public renders for unknown and anonymous', () => {
    expect(resolve(m({ layout: 'blank' }), unknown, '/').kind).toBe('render');
    expect(resolve(m(), anon, '/').kind).toBe('render');
  });
  it('auth + unknown -> pending', () => expect(resolve(m({ auth: true }), unknown, '/').kind).toBe('pending'));
  it('auth + anonymous -> redirect with next', () => {
    expect(resolve(m({ auth: true }), anon, '/projects/1?tab=key')).toEqual({
      kind: 'redirect',
      to: '/login?next=%2Fprojects%2F1%3Ftab%3Dkey',
    });
  });
  it('role mismatch -> notfound; match -> render', () => {
    expect(resolve(m({ roles: ['super_admin'] }), auth('developer'), '/').kind).toBe('notfound');
    expect(resolve(m({ roles: ['super_admin'] }), auth('super_admin'), '/').kind).toBe('render');
  });
  it('auth + developer renders', () => expect(resolve(m({ auth: true }), auth('developer'), '/').kind).toBe('render'));
});
