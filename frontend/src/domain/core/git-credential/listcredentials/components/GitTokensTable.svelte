<script lang="ts">
  import IconButton from '../../../../../shared/components/IconButton.svelte';
  import Button from '../../../../../shared/components/Button.svelte';
  import Table from '../../../../../shared/components/Table.svelte';
  import type { ApiError } from '../../../../../shared/services/http';
  import { locale, t } from '../../../../../shared/stores/i18n';
  import { formatDateTime } from '../../../../../shared/utils/datetime';
  import { maskLast4, type TokenRow } from '../types';
  import ProviderCell from './ProviderCell.svelte';

  let {
    rows,
    loading = false,
    error = null,
    onretry,
    canEdit = true,
    onsave,
    ondelete,
  }: {
    rows: TokenRow[];
    loading?: boolean;
    error?: ApiError | null;
    onretry?: () => void;
    canEdit?: boolean;
    onsave?: (row: TokenRow) => void;
    ondelete?: (row: TokenRow) => void;
  } = $props();

  const columns = $derived([
    { key: 'provider', header: $t('gitCredential.col.provider') },
    { key: 'host', header: $t('gitCredential.col.host') },
    { key: 'status', header: $t('gitCredential.col.status') },
    { key: 'updated', header: $t('gitCredential.col.updated'), class: 'text-right' },
    { key: 'actions', header: $t('gitCredential.col.actions'), class: 'text-right' },
  ]);
  const prov = (r: TokenRow) => $t(`gitCredential.provider.${r.provider}`);
</script>

{#snippet cell(r: TokenRow, col: { key: string })}
  {#if col.key === 'provider'}
    <ProviderCell provider={r.provider} />
  {:else if col.key === 'host'}
    <code class="font-mono text-[13px]">{r.host}</code>
  {:else if col.key === 'status'}
    {#if r.saved}
      <span class="inline-flex items-center gap-2">
        <span class="inline-flex h-5 items-center rounded-base bg-pill-positive-bg px-2 text-xs font-medium whitespace-nowrap text-pill-positive-ink">{$t('gitCredential.status.saved')}</span>
        <code class="font-mono text-[13px] text-muted">{maskLast4(r.saved.last4)}</code>
      </span>
    {:else}
      <span class="inline-flex h-5 items-center rounded-base bg-pill-neutral-bg px-2 text-xs font-medium whitespace-nowrap text-pill-neutral-ink">{$t('gitCredential.status.none')}</span>
    {/if}
  {:else if col.key === 'updated'}
    {#if r.saved}
      <time class="tabular-nums" datetime={r.saved.updated_at}>{formatDateTime(r.saved.updated_at, $locale)}</time>
    {:else}
      <span class="text-muted">–</span>
    {/if}
  {:else if col.key === 'actions'}
    {#if canEdit}
      <div class="flex justify-end gap-1">
        {#if onsave}
          <IconButton icon={r.saved ? 'replace' : 'send'} label={$t(r.saved ? 'gitCredential.action.replace' : 'gitCredential.action.save')} ariaLabel={$t(r.saved ? 'gitCredential.action.replace.aria' : 'gitCredential.action.save.aria', { provider: prov(r), host: r.host })} onclick={() => onsave?.(r)} />
        {/if}
        {#if ondelete && r.saved}
          <IconButton icon="delete" tone="danger" label={$t('gitCredential.action.delete')} ariaLabel={$t('gitCredential.action.delete.aria', { provider: prov(r), host: r.host })} onclick={() => ondelete?.(r)} />
        {/if}
      </div>
    {/if}
  {/if}
{/snippet}

<Table {columns} {rows} rowKey={(r) => r.provider + ':' + r.host} {cell} {loading} {error} {onretry} caption={$t('gitCredential.title')} />
