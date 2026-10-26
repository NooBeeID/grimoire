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
    rowClass,
    rowAttrs,
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
    rowClass?: (row: T) => string;
    rowAttrs?: (row: T) => Record<string, string>;
  } = $props();

  const dataAttrs = (row: T): Record<string, string> =>
    Object.fromEntries(Object.entries(rowAttrs?.(row) ?? {}).filter(([k]) => k.startsWith('data-')));
</script>

<div class="outlined overflow-hidden rounded-card bg-surface/85 shadow-panel">
<table class="w-full border-collapse text-left text-sm" aria-busy={loading}>
  {#if caption}<caption class="sr-only">{caption}</caption>{/if}
  <thead>
    <tr class="border-b border-outline bg-raised text-muted">
      {#each columns as col (col.key)}
        <th scope="col" class="h-10 px-4 text-xs font-medium {col.class ?? ''}">{col.header}</th>
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
        <tr class="h-12 border-t border-rule transition-colors hover:bg-accent-soft/60 {rowClass?.(row) ?? ''}" {...dataAttrs(row)}>
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
