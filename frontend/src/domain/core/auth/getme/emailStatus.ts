import { setMailStatus } from '../../../../shared/stores/mailStatus';
import { getMe } from './services/getme.service';
import type { Me } from './types';

export interface EmailStatusDeps {
  getMe: () => Promise<Me>;
  setMailStatus: (v: boolean | null) => void;
}

let inflight: Promise<void> | null = null;

// Mengambil email_enabled sekali; gagal apa pun diam (status tetap null, UI memakai teks netral).
export function refreshEmailStatus(deps: EmailStatusDeps = { getMe, setMailStatus }): Promise<void> {
  if (inflight) return inflight;
  const p = deps
    .getMe()
    .then((m) => deps.setMailStatus(m.email_enabled))
    .catch(() => undefined)
    .finally(() => {
      if (inflight === p) inflight = null;
    });
  inflight = p;
  return p;
}
