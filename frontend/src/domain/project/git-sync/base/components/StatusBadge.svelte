<script lang="ts">
  import { t } from '../../../../../shared/stores/i18n';
  import type { ItemStatus, RunStatus } from '../types';

  let { status }: { status: RunStatus | ItemStatus } = $props();

  const tone = $derived(
    status === 'success' || status === 'created'
      ? 'bg-pill-positive-bg text-pill-positive-ink'
      : status === 'partial'
        ? 'bg-pill-warning-bg text-pill-warning-ink'
        : status === 'failed' || status === 'rejected'
          ? 'bg-pill-negative-bg text-pill-negative-ink'
          : status === 'unchanged'
            ? 'bg-pill-neutral-bg text-muted'
            : 'bg-pill-neutral-bg text-pill-neutral-ink',
  );
  const label = $derived(
    status === 'success' || status === 'partial' || status === 'failed' ? $t(`gitSync.status.${status}`) : $t(`gitSync.itemStatus.${status}`),
  );
</script>

<span class="inline-flex h-5 flex-none items-center rounded-full px-2 text-xs font-medium whitespace-nowrap {tone}" data-status={status}>{label}</span>
