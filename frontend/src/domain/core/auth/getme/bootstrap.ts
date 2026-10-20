import { isApiError } from '../../../../shared/services/http';
import { setIdentity, type Identity } from '../../../../shared/stores/session';
import { fetchMe } from './services/getme.service';

export interface BootstrapDeps {
  fetchMe: () => Promise<Identity>;
  setIdentity: (i: Identity | null) => void;
}

// 401 = anonim (hasil normal). Error lain dilempar dan store tidak diubah.
export async function loadSession(deps: BootstrapDeps = { fetchMe, setIdentity }): Promise<void> {
  try {
    deps.setIdentity(await deps.fetchMe());
  } catch (e) {
    if (isApiError(e) && e.status === 401) {
      deps.setIdentity(null);
      return;
    }
    throw e;
  }
}
