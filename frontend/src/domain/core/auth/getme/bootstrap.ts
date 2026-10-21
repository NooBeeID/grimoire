import { isApiError } from '../../../../shared/services/http';
import { setMailStatus } from '../../../../shared/stores/mailStatus';
import { setIdentity, type Identity } from '../../../../shared/stores/session';
import { getMe } from './services/getme.service';
import { toIdentity, type Me } from './types';

export interface BootstrapDeps {
  getMe: () => Promise<Me>;
  setIdentity: (i: Identity | null) => void;
  setMailStatus: (v: boolean | null) => void;
}

// 401 = anonim (hasil normal). Error lain dilempar dan store tidak diubah.
export async function loadSession(deps: BootstrapDeps = { getMe, setIdentity, setMailStatus }): Promise<void> {
  try {
    const me = await deps.getMe();
    deps.setIdentity(toIdentity(me));
    deps.setMailStatus(me.email_enabled);
  } catch (e) {
    if (isApiError(e) && e.status === 401) {
      deps.setIdentity(null);
      deps.setMailStatus(null);
      return;
    }
    throw e;
  }
}
