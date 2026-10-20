import { isApiError } from '../../../../shared/services/http';
import { logoutRequest } from './services/logout.service';

export type LogoutResult = { ok: true } | { ok: false; error: unknown };

// Tidak menyentuh store sesi/router; pemanggilnya (app/) yang bertanggung jawab.
export async function performLogout(deps: { logoutRequest: () => Promise<void> } = { logoutRequest }): Promise<LogoutResult> {
  try {
    await deps.logoutRequest();
    return { ok: true };
  } catch (error) {
    if (isApiError(error) && error.status === 401) return { ok: true };
    return { ok: false, error };
  }
}
