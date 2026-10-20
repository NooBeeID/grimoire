<script lang="ts">
  import Button from '../../../../../shared/components/Button.svelte';
  import EmptyState from '../../../../../shared/components/EmptyState.svelte';
  import ErrorState from '../../../../../shared/components/ErrorState.svelte';
  import Spinner from '../../../../../shared/components/Spinner.svelte';
  import type { ApiError } from '../../../../../shared/services/http';
  import { locale, t } from '../../../../../shared/stores/i18n';
  import type { AuditSummary } from '../types';
  import ActorIcon from './ActorIcon.svelte';
  import { formatDateTime } from './format';
  import StatusBadge from './StatusBadge.svelte';

  let {
    rows,
    status,
    error,
    selectedId,
    hasMore,
    moreStatus,
    moreError,
    filtered,
    onselect,
    onloadmore,
    onretry,
    onretrymore,
    onresetfilter,
  }: {
    rows: AuditSummary[];
    status: 'idle' | 'loading' | 'ready' | 'error';
    error: ApiError | null;
    selectedId: string | null;
    hasMore: boolean;
    moreStatus: 'idle' | 'loading' | 'error';
    moreError: ApiError | null;
    filtered: boolean;
    onselect: (id: string) => void;
    onloadmore: () => void;
    onretry: () => void;
    onretrymore: () => void;
    onresetfilter: () => void;
  } = $props();

  const loading = $derived(status === 'loading' || status === 'idle');
  // 400 (filter rusak) dan 403 tidak bisa dicoba ulang.
  const retryable = $derived(!!error && error.status !== 403 && error.status !== 400);
  const invalidField = $derived(
    error?.status === 400 && error.details && typeof error.details === 'object'
      ? String((error.details as { field?: unknown }).field ?? '')
      : '',
  );
  const cols = ['time', 'actor', 'project', 'action', 'target', 'status'] as const;
  const hideSm: Record<string, string> = { project: 'hidden sm:table-cell', target: 'hidden sm:table-cell' };
</script>

<div class="overflow-x-auto" aria-busy={loading}>
  <table class="w-full border-collapse text-left text-sm">
    <caption class="sr-only">{$t('auditLog.title')}</caption>
    <thead>
      <tr class="border-b border-border">
        {#each cols as c (c)}
          <th scope="col" class="px-3 py-2 font-semibold {hideSm[c] ?? ''}">{$t(`auditLog.columns.${c}`)}</th>
        {/each}
      </tr>
    </thead>
    <tbody>
      {#if loading}
        <tr><td colspan={cols.length} class="p-6 text-center"><Spinner /></td></tr>
      {:else if error}
        <tr>
          <td colspan={cols.length}>
            <ErrorState {error} onretry={retryable ? onretry : undefined} />
            {#if error.status === 400}
              <div class="flex flex-col items-center gap-2 pb-4">
                <p class="text-sm">{$t('auditLog.error.invalidFilter', { field: invalidField })}</p>
                <Button variant="secondary" size="sm" onclick={onresetfilter}>{$t('auditLog.filter.reset')}</Button>
              </div>
            {/if}
          </td>
        </tr>
      {:else if rows.length === 0}
        <tr>
          <td colspan={cols.length}>
            {#if filtered}
              <EmptyState title={$t('auditLog.emptyFiltered')}>
                {#snippet action()}
                  <Button variant="secondary" size="sm" onclick={onresetfilter}>{$t('auditLog.filter.reset')}</Button>
                {/snippet}
              </EmptyState>
            {:else}
              <EmptyState title={$t('auditLog.empty')} description={$t('auditLog.emptyHint')} />
            {/if}
          </td>
        </tr>
      {:else}
        {#each rows as r (r.id)}
          <tr
            class="border-b border-border {selectedId === r.id ? 'bg-black/5' : ''}"
            aria-current={selectedId === r.id ? 'true' : undefined}
          >
            <td class="px-3 py-2">
              <button
                type="button"
                data-entry-id={r.id}
                title={r.occurred_at}
                onclick={() => onselect(r.id)}
                class="text-left underline-offset-2 hover:underline focus-visible:ring-2 focus-visible:ring-primary focus-visible:outline-none"
              >
                {formatDateTime(r.occurred_at, $locale)}
              </button>
            </td>
            <td class="px-3 py-2">
              <span class="inline-flex items-center gap-1.5"><ActorIcon type={r.actor_type} />{r.actor_label}</span>
            </td>
            <td class="px-3 py-2 {hideSm['project']}">{r.project_label ?? '-'}</td>
            <td class="px-3 py-2 font-mono text-xs">{r.action}</td>
            <td class="px-3 py-2 {hideSm['target']}">
              <span class="block max-w-[24ch] truncate" title={r.target ?? ''}>{r.target ?? '-'}</span>
            </td>
            <td class="px-3 py-2"><StatusBadge status={r.status} /></td>
          </tr>
        {/each}
      {/if}
    </tbody>
  </table>
</div>

{#if status === 'ready' && rows.length > 0}
  <div class="flex flex-col items-center gap-2 p-4" aria-live="polite">
    {#if hasMore}
      <Button variant="secondary" disabled={moreStatus === 'loading'} onclick={onloadmore}>
        {#if moreStatus === 'loading'}<Spinner size={14} />{/if}
        {$t('auditLog.loadMore')}
      </Button>
      {#if moreStatus === 'loading'}<p class="text-sm text-muted">{$t('auditLog.loadingMore')}</p>{/if}
    {:else}
      <p class="text-xs text-muted">{$t('auditLog.endOfList')}</p>
    {/if}
    {#if moreStatus === 'error' && moreError}
      <div role="alert" class="flex items-center gap-2 text-sm">
        <span class="text-danger">{moreError.message}</span>
        <Button variant="secondary" size="sm" onclick={onretrymore}>{$t('common.retry')}</Button>
      </div>
    {/if}
  </div>
{/if}
