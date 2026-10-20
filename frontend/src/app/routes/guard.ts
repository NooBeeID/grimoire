import type { SessionState } from '../../shared/stores/session';
import type { Match, RouteDef } from './router';

// Guard hanya untuk UX. Otorisasi sebenarnya ada di backend (rules 12.7).
export type Resolution =
  | { kind: 'render'; route: RouteDef; params: Record<string, string> }
  | { kind: 'redirect'; to: string }
  | { kind: 'pending' }
  | { kind: 'notfound' };

export function resolve(m: Match | null, s: SessionState, fullPath: string): Resolution {
  if (!m) return { kind: 'notfound' };
  const meta = m.route.meta;
  if (!meta?.auth && !meta?.roles) return { kind: 'render', route: m.route, params: m.params };
  if (s.status === 'unknown') return { kind: 'pending' };
  if (s.status === 'anonymous' || !s.identity) {
    return { kind: 'redirect', to: '/login?next=' + encodeURIComponent(fullPath) };
  }
  if (meta.roles && !meta.roles.includes(s.identity.role)) return { kind: 'notfound' };
  return { kind: 'render', route: m.route, params: m.params };
}
