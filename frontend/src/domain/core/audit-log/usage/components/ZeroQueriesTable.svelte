<script lang="ts">
  import Table from '../../../../../shared/components/Table.svelte';
  import { locale, t } from '../../../../../shared/stores/i18n';
  import { formatDateTime } from '../../../../../shared/utils/datetime';
  import type { SectionState } from '../stores/usage.store';
  import type { ZeroQuery } from '../types';
  import { sectionError } from './format';
  import SectionCard from './SectionCard.svelte';

  let { state, onretry }: { state: SectionState<ZeroQuery>; onretry: () => void } = $props();

  const columns = $derived([
    { key: 'query', header: $t('dashboard.usage.col.query') },
    { key: 'count', header: $t('dashboard.usage.col.count'), class: 'text-right tabular-nums' },
    { key: 'keys', header: $t('dashboard.usage.col.keys'), class: 'text-right tabular-nums' },
    { key: 'last_at', header: $t('dashboard.usage.col.lastAt') },
  ]);
</script>

<SectionCard title={$t('dashboard.usage.zeroQueries')} {state}>
  <Table
    {columns}
    rows={state.items}
    rowKey={(r) => r.query}
    loading={state.status === 'loading'}
    error={sectionError(state.error, $t('dashboard.usage.unavailable'))}
    emptyText={$t('dashboard.usage.empty.zeroQueries')}
    caption={$t('dashboard.usage.zeroQueries')}
    {onretry}
  >
    {#snippet cell(r, col)}
      {#if col.key === 'query'}<span class="block max-w-[22rem] truncate" title={r.query}>{r.query}</span>
      {:else if col.key === 'last_at'}<span class="text-muted">{formatDateTime(r.last_at, $locale)}</span>
      {:else}{r[col.key as 'count' | 'keys']}{/if}
    {/snippet}
  </Table>

</SectionCard>
