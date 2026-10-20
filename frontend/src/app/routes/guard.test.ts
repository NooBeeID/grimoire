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

  describe('T01 rules', () => {
    const must: SessionState = {
      status: 'authenticated',
      identity: { userId: '1', email: 'a@b.c', name: 'A', role: 'developer', mustChangePassword: true },
    };
    it('guestOnly: pending / render / redirect to safe next', () => {
      const g = m({ guestOnly: true });
      expect(resolve(g, unknown, '/login').kind).toBe('pending');
      expect(resolve(g, anon, '/login').kind).toBe('render');
      expect(resolve(g, auth('developer'), '/login?next=%2Fadmin%2Fusers')).toEqual({ kind: 'redirect', to: '/admin/users' });
      expect(resolve(g, auth('developer'), '/login?next=%2F%2Fevil.com')).toEqual({ kind: 'redirect', to: '/' });
      expect(resolve(g, auth('developer'), '/login')).toEqual({ kind: 'redirect', to: '/' });
    });
    it('forces change-password while flag set', () => {
      expect(resolve(m({ auth: true }), must, '/profile?x=1')).toEqual({
        kind: 'redirect',
        to: '/change-password?next=%2Fprofile%3Fx%3D1',
      });
      expect(resolve(m({ auth: true, passwordChangeRoute: true }), must, '/change-password').kind).toBe('render');
    });
    it('change-password route redirects away when flag cleared', () => {
      const r = m({ auth: true, passwordChangeRoute: true });
      expect(resolve(r, auth('developer'), '/change-password?next=%2Fprofile')).toEqual({ kind: 'redirect', to: '/profile' });
      expect(resolve(r, auth('developer'), '/change-password')).toEqual({ kind: 'redirect', to: '/' });
    });
    it('must-change takes priority over role mismatch', () => {
      expect(resolve(m({ roles: ['super_admin'] }), must, '/admin/users').kind).toBe('redirect');
    });
  });
});

