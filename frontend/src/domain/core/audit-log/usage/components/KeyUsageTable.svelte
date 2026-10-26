<script lang="ts">
  import Table from '../../../../../shared/components/Table.svelte';
  import { locale, t } from '../../../../../shared/stores/i18n';
  import { formatDateTime } from '../../../../../shared/utils/datetime';
  import type { SectionState } from '../stores/usage.store';
  import type { KeyUsage } from '../types';
  import { sectionError } from './format';
  import SectionCard from './SectionCard.svelte';

  let { state, onretry }: { state: SectionState<KeyUsage>; onretry: () => void } = $props();

  const columns = $derived([
    { key: 'label', header: $t('dashboard.usage.col.key') },
    { key: 'calls', header: $t('dashboard.usage.col.calls'), class: 'text-right tabular-nums' },
    { key: 'errors', header: $t('dashboard.usage.col.errors'), class: 'text-right tabular-nums' },
    { key: 'searches', header: $t('dashboard.usage.col.searches'), class: 'text-right tabular-nums' },
    { key: 'reads', header: $t('dashboard.usage.col.reads'), class: 'text-right tabular-nums' },
    { key: 'last_used_at', header: $t('dashboard.usage.col.lastUsed') },
    { key: 'top_tool', header: $t('dashboard.usage.col.topTool') },
  ]);
</script>

<SectionCard title={$t('dashboard.usage.perKey')} {state}>
  <Table
    {columns}
    rows={state.items}
    rowKey={(r) => r.key_id}
    loading={state.status === 'loading'}
    error={sectionError(state.error, $t('dashboard.usage.unavailable'))}
    emptyText={$t('dashboard.usage.empty.perKey')}
    caption={$t('dashboard.usage.perKey')}
    {onretry}
  >
    {#snippet cell(r, col)}
      {#if col.key === 'label'}<span class="font-medium">{r.label}</span>
      {:else if col.key === 'last_used_at'}<span class="text-muted">{formatDateTime(r.last_used_at, $locale)}</span>
      {:else if col.key === 'top_tool'}<span class="font-mono text-[13px]">{r.top_tool}</span>
      {:else}{r[col.key as 'calls' | 'errors' | 'searches' | 'reads']}{/if}
    {/snippet}
  </Table>

</SectionCard>
