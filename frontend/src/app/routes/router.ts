import type { Component } from 'svelte';
import { get, writable, type Readable } from 'svelte/store';
import { setUnauthorizedHandler } from '../../shared/services/http';
import { t } from '../../shared/stores/i18n';
import type { Role } from '../../shared/stores/session';

export interface RouteMeta {
  auth?: boolean;
  roles?: Role[];
  layout?: 'blank' | 'app';
  titleKey?: string;
}
export interface RouteDef {
  path: string;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  view: Component<any>;
  meta?: RouteMeta;
}
export interface Location {
  path: string;
  query: URLSearchParams;
  fullPath: string;
}
export interface Match {
  route: RouteDef;
  params: Record<string, string>;
}
export interface CurrentState {
  location: Location;
  match: Match | null;
}

let defs: RouteDef[] = [];
let fallback: RouteDef | null = null;

function readLocation(): Location {
  return {
    path: window.location.pathname,
    query: new URLSearchParams(window.location.search),
    fullPath: window.location.pathname + window.location.search,
  };
}

function normalize(p: string): string {
  return p.length > 1 ? p.replace(/\/+$/, '') : p;
}

export function match(path: string, list: RouteDef[]): Match | null {
  const segs = normalize(path).split('/').filter(Boolean);
  for (const route of list) {
    const pat = normalize(route.path).split('/').filter(Boolean);
    if (pat.length !== segs.length) continue;
    const params: Record<string, string> = {};
    let ok = true;
    for (let i = 0; i < pat.length; i++) {
      const p = pat[i]!;
      const s = segs[i]!;
      if (p.startsWith(':')) {
        try {
          params[p.slice(1)] = decodeURIComponent(s);
        } catch {
          ok = false;
          break;
        }
      } else if (p !== s) {
        ok = false;
        break;
      }
    }
    if (ok) return { route, params };
  }
  return null;
}

function compute(): CurrentState {
  const location = readLocation();
  return { location, match: match(location.path, defs) };
}

const store = writable<CurrentState>({ location: readLocation(), match: null });
export const current: Readable<CurrentState> = { subscribe: store.subscribe };

function applyTitle(m: Match | null) {
  const route = m?.route ?? fallback;
  const key = route?.meta?.titleKey;
  const tr = get(t);
  document.title = key ? `${tr(key)} · ${tr('app.name')}` : tr('app.name');
}

function refresh() {
  const s = compute();
  store.set(s);
  applyTitle(s.match);
}

export function navigate(to: string, opts: { replace?: boolean } = {}): void {
  const url = new URL(to, window.location.origin);
  const target = url.pathname + url.search + url.hash;
  const here = window.location.pathname + window.location.search + window.location.hash;
  if (target === here) {
    refresh();
    return;
  }
  if (opts.replace) window.history.replaceState({}, '', target);
  else window.history.pushState({}, '', target);
  refresh();
  window.scrollTo?.(0, 0);
}

function onClick(e: MouseEvent) {
  if (e.defaultPrevented || e.button !== 0 || e.ctrlKey || e.metaKey || e.shiftKey || e.altKey) return;
  const a = (e.target as Element | null)?.closest?.('a');
  if (!a || !a.hasAttribute('href')) return;
  if (a.hasAttribute('data-router-ignore') || a.hasAttribute('download')) return;
  const target = a.getAttribute('target');
  if (target && target !== '_self') return;
  const url = new URL(a.href, window.location.href);
  if (url.origin !== window.location.origin) return;
  e.preventDefault();
  navigate(url.pathname + url.search + url.hash);
}

export function startRouter(routeDefs: RouteDef[], notFound?: RouteDef): () => void {
  defs = routeDefs;
  fallback = notFound ?? null;
  const onPop = () => refresh();
  window.addEventListener('popstate', onPop);
  document.addEventListener('click', onClick);
  setUnauthorizedHandler((p) => navigate('/login?next=' + encodeURIComponent(p), { replace: true }));
  refresh();
  return () => {
    window.removeEventListener('popstate', onPop);
    document.removeEventListener('click', onClick);
    setUnauthorizedHandler(null);
  };
}
