<script lang="ts">
  import { t } from '../../../../../shared/stores/i18n';
  import type { FileStatus } from '../types';

  let { status }: { status: FileStatus } = $props();
  const tone = $derived(
    status === 'created' || status === 'updated' || status === 'valid'
      ? 'bg-pill-positive-bg text-pill-positive-ink'
      : status === 'rejected'
        ? 'bg-pill-negative-bg text-pill-negative-ink'
        : status === 'skipped'
          ? 'bg-pill-warning-bg text-pill-warning-ink'
          : 'bg-pill-neutral-bg text-pill-neutral-ink',
  );
</script>

<span class="inline-flex h-5 items-center rounded-full px-2 text-xs font-medium {tone}">{$t(`apispec.status.${status}`)}</span>
