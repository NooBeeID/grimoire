<script lang="ts" generics="T">
  import type { Snippet } from 'svelte';
  import { t } from '../stores/i18n';
  import type { ApiError } from '../services/http';
  import EmptyState from './EmptyState.svelte';
  import ErrorState from './ErrorState.svelte';
  import Spinner from './Spinner.svelte';

  interface Column {
    key: string;
    header: string;
    class?: string;
  }

  let {
    columns,
    rows,
    rowKey,
    cell,
    loading = false,
    error = null,
    emptyText,
    caption,
    onretry,
  }: {
    columns: Column[];
    rows: T[];
    rowKey: (row: T) => string | number;
    cell?: Snippet<[T, Column]>;
    loading?: boolean;
    error?: ApiError | null;
    emptyText?: string;
    caption?: string;
    onretry?: () => void;
  } = $props();
</script>

<div class="overflow-hidden rounded-card border border-border bg-surface shadow-panel">
<table class="w-full border-collapse text-left text-sm" aria-busy={loading}>
  {#if caption}<caption class="sr-only">{caption}</caption>{/if}
  <thead>
    <tr class="bg-raised">
      {#each columns as col (col.key)}
        <th scope="col" class="h-10 px-4 text-[13px] font-medium text-muted {col.class ?? ''}">{col.header}</th>
      {/each}
    </tr>
  </thead>
  <tbody>
    {#if loading}
      <tr><td colspan={columns.length} class="p-6 text-center"><Spinner /></td></tr>
    {:else if error}
      <tr><td colspan={columns.length}><ErrorState {error} {onretry} /></td></tr>
    {:else if rows.length === 0}
      <tr><td colspan={columns.length}><EmptyState title={emptyText ?? $t('table.empty')} /></td></tr>
    {:else}
      {#each rows as row (rowKey(row))}
        <tr class="h-12 border-t border-rule transition-colors hover:bg-raised/60">
          {#each columns as col (col.key)}
            <td class="px-4 py-2 {col.class ?? ''}">
              {#if cell}{@render cell(row, col)}{:else}{(row as Record<string, unknown>)[col.key] ?? ''}{/if}
            </td>
          {/each}
        </tr>
      {/each}
    {/if}
  </tbody>
</table>
</div>
