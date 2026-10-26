<script lang="ts">
  import Table from '../../../../../shared/components/Table.svelte';
  import { locale, t } from '../../../../../shared/stores/i18n';
  import { formatDateTime } from '../../../../../shared/utils/datetime';
  import type { SectionState } from '../stores/usage.store';
  import type { UnreadFile } from '../types';
  import { sectionError } from './format';
  import SectionCard from './SectionCard.svelte';

  let { state, onretry }: { state: SectionState<UnreadFile>; onretry: () => void } = $props();

  const columns = $derived([
    { key: 'path', header: $t('dashboard.usage.col.path') },
    { key: 'updated_at', header: $t('dashboard.usage.col.updated') },
  ]);
</script>

<SectionCard title={$t('dashboard.usage.unread')} {state}>
  <Table
    {columns}
    rows={state.items}
    rowKey={(r) => r.path}
    loading={state.status === 'loading'}
    error={sectionError(state.error, $t('dashboard.usage.unavailable'))}
    emptyText={$t('dashboard.usage.empty.unread')}
    caption={$t('dashboard.usage.unread')}
    {onretry}
  >
    {#snippet cell(r, col)}
      {#if col.key === 'path'}<span class="block max-w-[22rem] truncate font-mono text-[13px]" title={r.path}>{r.path}</span>
      {:else}<span class="text-muted">{formatDateTime(r.updated_at, $locale)}</span>{/if}
    {/snippet}
  </Table>
  {#if state.truncated}
    <p class="mt-3 text-xs text-muted">{$t('dashboard.usage.unread.truncated', { n: state.items.length })}</p>
  {/if}
</SectionCard>
