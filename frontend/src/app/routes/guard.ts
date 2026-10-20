import type { SessionState } from '../../shared/stores/session';
import { safeNext } from './next';
import type { Match, RouteDef } from './router';

// Guard hanya untuk UX. Otorisasi sebenarnya ada di backend (rules 12.7).
export type Resolution =
  | { kind: 'render'; route: RouteDef; params: Record<string, string> }
  | { kind: 'redirect'; to: string }
  | { kind: 'pending' }
  | { kind: 'notfound' };

function nextOf(fullPath: string): string | null {
  return safeNext(new URL(fullPath, 'http://x').searchParams.get('next'));
}

export function resolve(m: Match | null, s: SessionState, fullPath: string): Resolution {
  if (!m) return { kind: 'notfound' };
  const meta = m.route.meta;
  const render: Resolution = { kind: 'render', route: m.route, params: m.params };

  if (meta?.guestOnly) {
    if (s.status === 'unknown') return { kind: 'pending' };
    if (s.status === 'authenticated') return { kind: 'redirect', to: nextOf(fullPath) ?? '/' };
    return render;
  }
  if (!meta?.auth && !meta?.roles) return render;
  if (s.status === 'unknown') return { kind: 'pending' };
  if (s.status === 'anonymous' || !s.identity) {
    return { kind: 'redirect', to: '/login?next=' + encodeURIComponent(fullPath) };
  }
  if (s.identity.mustChangePassword && !meta.passwordChangeRoute) {
    return { kind: 'redirect', to: '/change-password?next=' + encodeURIComponent(fullPath) };
  }
  if (meta.passwordChangeRoute && !s.identity.mustChangePassword) {
    return { kind: 'redirect', to: nextOf(fullPath) ?? '/' };
  }
  if (meta.roles && !meta.roles.includes(s.identity.role)) return { kind: 'notfound' };
  return render;
}
