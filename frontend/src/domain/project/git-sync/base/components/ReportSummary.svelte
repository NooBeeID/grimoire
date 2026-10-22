<script lang="ts">
  import { t } from '../../../../../shared/stores/i18n';
  import { ITEM_STATUSES, type GitSyncSummary } from '../types';

  let { summary }: { summary: GitSyncSummary } = $props();
</script>

<dl class="grid grid-cols-5 divide-x divide-rule rounded-card border border-border" data-testid="report-summary">
  {#each ITEM_STATUSES as s (s)}
    <div class="flex flex-col gap-0.5 px-3 py-2">
      <dt class="text-xs text-muted">{$t(`gitSync.summary.${s}`)}</dt>
      <dd class="text-base font-medium tabular-nums {s === 'rejected' && summary[s] > 0 ? 'text-danger' : ''}" data-key={s}>{summary[s]}</dd>
    </div>
  {/each}
</dl>
