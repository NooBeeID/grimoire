import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { Component } from 'svelte';
import { ApiError, get as httpGet, setUnauthorizedHandler } from '../../shared/services/http';
import { current, match, navigate, startRouter, type RouteDef } from './router';

const view = (() => {}) as unknown as Component;
const defs: RouteDef[] = [
  { path: '/', view },
  { path: '/projects/:id', view },
  { path: '/x', view },
];

describe('match', () => {
  it('matches static, params, decode, trailing slash', () => {
    expect(match('/', defs)?.route.path).toBe('/');
    expect(match('/projects/abc', defs)?.params).toEqual({ id: 'abc' });
    expect(match('/projects/a%20b', defs)?.params.id).toBe('a b');
    expect(match('/x/', defs)?.route.path).toBe('/x');
    expect(match('/nope', defs)).toBeNull();
  });
});

describe('router', () => {
  let stop: () => void;
  beforeEach(() => {
    history.replaceState({}, '', '/');
    stop = startRouter(defs);
  });
  afterEach(() => {
    stop();
    document.body.innerHTML = '';
  });

  it('navigate uses pushState / replaceState and dedupes', () => {
    const push = vi.spyOn(history, 'pushState');
    const rep = vi.spyOn(history, 'replaceState');
    navigate('/x');
    expect(push).toHaveBeenCalledTimes(1);
    navigate('/x');
    expect(push).toHaveBeenCalledTimes(1);
    navigate('/projects/1', { replace: true });
    expect(rep).toHaveBeenCalledTimes(1);
    expect(get(current).match?.params).toEqual({ id: '1' });
  });

  it('popstate updates current and parses query', () => {
    history.pushState({}, '', '/x?a=1');
    window.dispatchEvent(new PopStateEvent('popstate'));
    expect(get(current).location.path).toBe('/x');
    expect(get(current).location.query.get('a')).toBe('1');
  });

  function click(attrs: string, init: MouseEventInit = {}) {
    document.body.innerHTML = `<a id="a" ${attrs}>l</a>`;
    const ev = new MouseEvent('click', { bubbles: true, cancelable: true, button: 0, ...init });
    document.getElementById('a')!.dispatchEvent(ev);
    return ev;
  }

  it('intercepts same-origin link clicks', () => {
    const ev = click('href="/x"');
    expect(ev.defaultPrevented).toBe(true);
    expect(get(current).location.path).toBe('/x');
  });
  it.each([
    ['data-router-ignore', 'href="/x" data-router-ignore', {}],
    ['target blank', 'href="/x" target="_blank"', {}],
    ['other origin', 'href="https://other.test/x"', {}],
    ['ctrl', 'href="/x"', { ctrlKey: true }],
    ['meta', 'href="/x"', { metaKey: true }],
    ['download', 'href="/x" download', {}],
  ])('ignores %s', (_n, attrs, init) => {
    const ev = click(attrs, init);
    expect(ev.defaultPrevented).toBe(false);
    expect(get(current).location.path).toBe('/');
  });

  it('installs unauthorized handler; stop removes it', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('{"error":{"code":"UNAUTHORIZED","message":"m"}}', { status: 401 })));
    history.pushState({}, '', '/x?q=1');
    await httpGet('/api/y').catch((e) => expect(e).toBeInstanceOf(ApiError));
    expect(window.location.pathname).toBe('/login');
    expect(window.location.search).toBe('?next=' + encodeURIComponent('/x?q=1'));
    stop();
    const h = vi.fn();
    setUnauthorizedHandler(h);
    stop = () => {};
    vi.unstubAllGlobals();
  });

  it('stop removes listeners', () => {
    stop();
    const ev = click('href="/x"');
    expect(ev.defaultPrevented).toBe(false);
    stop = startRouter(defs);
  });
});
