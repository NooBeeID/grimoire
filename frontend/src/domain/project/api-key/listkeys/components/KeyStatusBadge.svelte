<script lang="ts">
  import { t } from '../../../../../shared/stores/i18n';
  import type { ApiKey } from '../types';

  let { status }: { status: ApiKey['status'] } = $props();
</script>

<span
  class="inline-flex h-6 items-center rounded-full px-2.5 text-xs font-medium {status === 'active'
    ? 'bg-pill-positive-bg text-pill-positive-ink'
    : 'bg-pill-neutral-bg text-pill-neutral-ink'}">{$t(`project.apikey.status.${status}`)}</span
>
