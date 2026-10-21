<script lang="ts">
  import { t } from '../../../../../shared/stores/i18n';
  import { summarize } from '../model';
  import { FILE_STATUSES, type ApiSpecFileResult } from '../types';
  import StatusBadge from './StatusBadge.svelte';

  let { results }: { results: ApiSpecFileResult[] } = $props();
  const counts = $derived(summarize(results));
</script>

<ul role="status" class="flex flex-wrap items-center gap-3 text-sm" aria-label={$t('apispec.upload.results')}>
  {#each FILE_STATUSES.filter((s) => counts[s] > 0) as s (s)}
    <li class="inline-flex items-center gap-1.5"><StatusBadge status={s} /><span class="tabular-nums">{counts[s]}</span></li>
  {/each}
</ul>
