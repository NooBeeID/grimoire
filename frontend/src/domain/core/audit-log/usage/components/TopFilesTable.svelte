<script lang="ts">
  import Table from '../../../../../shared/components/Table.svelte';
  import { locale, t } from '../../../../../shared/stores/i18n';
  import { formatDateTime } from '../../../../../shared/utils/datetime';
  import type { SectionState } from '../stores/usage.store';
  import type { FileRead } from '../types';
  import { sectionError } from './format';
  import SectionCard from './SectionCard.svelte';

  let { state, onretry }: { state: SectionState<FileRead>; onretry: () => void } = $props();

  const columns = $derived([
    { key: 'path', header: $t('dashboard.usage.col.path') },
    { key: 'reads', header: $t('dashboard.usage.col.reads'), class: 'text-right tabular-nums' },
    { key: 'keys', header: $t('dashboard.usage.col.keys'), class: 'text-right tabular-nums' },
    { key: 'last_read_at', header: $t('dashboard.usage.col.lastRead') },
  ]);
</script>

<SectionCard title={$t('dashboard.usage.topFiles')} {state}>
  <Table
    {columns}
    rows={state.items}
    rowKey={(r) => r.path}
    loading={state.status === 'loading'}
    error={sectionError(state.error, $t('dashboard.usage.unavailable'))}
    emptyText={$t('dashboard.usage.empty.topFiles')}
    caption={$t('dashboard.usage.topFiles')}
    {onretry}
  >
    {#snippet cell(r, col)}
      {#if col.key === 'path'}<span class="block max-w-[22rem] truncate font-mono text-[13px]" title={r.path}>{r.path}</span>
      {:else if col.key === 'last_read_at'}<span class="text-muted">{formatDateTime(r.last_read_at, $locale)}</span>
      {:else}{r[col.key as 'reads' | 'keys']}{/if}
    {/snippet}
  </Table>

</SectionCard>
