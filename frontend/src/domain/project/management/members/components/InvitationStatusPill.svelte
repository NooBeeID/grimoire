<script lang="ts">
  import { t } from '../../../../../shared/stores/i18n';
  import { invitationStatus } from '../model/invite';
  import type { Invitation } from '../types';

  let { invitation }: { invitation: Pick<Invitation, 'expired' | 'delivery_status'> } = $props();
  const status = $derived(invitationStatus(invitation));
  // Warna hanya pada baris yang perlu tindakan.
  const tone = $derived(
    status === 'expired'
      ? 'bg-pill-warning-bg text-pill-warning-ink'
      : status === 'failed'
        ? 'bg-pill-negative-bg text-pill-negative-ink'
        : 'bg-pill-neutral-bg text-pill-neutral-ink',
  );
</script>

<span class="inline-flex h-5 flex-none items-center rounded-full px-2 text-xs font-medium whitespace-nowrap {tone}">{$t(`project.invitations.status.${status}`)}</span>
