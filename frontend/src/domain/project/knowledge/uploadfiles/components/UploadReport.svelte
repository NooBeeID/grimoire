<script lang="ts">
  import { t } from '../../../../../shared/stores/i18n';
  import type { Filter } from '../stores/upload.store';
  import type { ReportItem } from '../types';

  type Counts = Record<'created' | 'updated' | 'unchanged' | 'skipped' | 'rejected' | 'unsupported', number>;

  let {
    items,
    counts,
    filter,
    errors = [],
    cancelled = false,
    onfilter,
  }: {
    items: ReportItem[];
    counts: Counts;
    filter: Filter;
    errors?: string[];
    cancelled?: boolean;
    onfilter: (f: Filter) => void;
  } = $props();

  const filters: Filter[] = ['all', 'failed', 'updated', 'created', 'unchanged', 'skipped'];
  const kinds = ['created', 'updated', 'unchanged', 'skipped', 'rejected', 'unsupported'] as const;
  const statusKey = (s: string) => `knowledge.upload.status.${s}`;

  // Daftar besar dirender bertahap: tampil semua sampai 500, lalu 200 per langkah.
  const ALL_UP_TO = 500;
  const STEP = 200;
  let limit = $state(STEP);
  $effect(() => {
    void filter;
    limit = STEP;
  });
  const visible = $derived(items.length <= ALL_UP_TO ? items : items.slice(0, limit));
</script>

<h3 class="text-sm font-semibold">{$t('knowledge.upload.report')}</h3>
{#if cancelled}<p class="mt-1 text-sm text-warning">{$t('knowledge.upload.cancelled')}</p>{/if}
{#each errors as msg (msg)}<p role="alert" class="mt-1 text-sm text-danger">{msg}</p>{/each}

<dl class="mt-3 grid grid-cols-3 gap-x-4 gap-y-2 text-sm sm:grid-cols-6">
  {#each kinds as k (k)}
    <div class="border-t border-rule pt-1">
      <dt class="text-xs text-muted">{$t(statusKey(k))}</dt>
      <dd class="font-medium">{counts[k]}</dd>
    </div>
  {/each}
</dl>

<div role="radiogroup" aria-label={$t('knowledge.upload.report')} class="mt-3 flex flex-wrap gap-1">
  {#each filters as f (f)}
    <button
      type="button"
      role="radio"
      aria-checked={filter === f}
      onclick={() => onfilter(f)}
      class="h-8 rounded-base border px-2.5 text-[13px] focus-visible:ring-2 focus-visible:ring-accent-ink focus-visible:outline-none {filter === f ? 'border-primary bg-active font-semibold' : 'border-control'}"
    >{$t(`knowledge.upload.filter.${f}`)}</button>
  {/each}
</div>

<div class="mt-3 max-h-80 overflow-auto rounded-card border border-border shadow-panel">
  <table class="w-full text-left text-sm">
    <thead class="sticky top-0 bg-raised text-[13px] text-muted">
      <tr>
        <th class="px-3 py-2 font-medium">{$t('knowledge.new.path')}</th>
        <th class="px-3 py-2 font-medium">{$t('knowledge.upload.statusCol')}</th>
      </tr>
    </thead>
    <tbody>
      {#each visible as r, i (i)}
        <tr class="h-12 border-t border-rule">
          <td class="px-3 py-1 font-mono text-[13px] break-all">{r.path}</td>
          <td class="px-3 py-1">
            {$t(statusKey(r.status))}
            {#if r.reasonText}<span class="block text-xs text-muted">{r.reasonText}</span>{/if}
          </td>
        </tr>
      {/each}
    </tbody>
  </table>
</div>
{#if visible.length < items.length}
  <button
    type="button"
    onclick={() => (limit += STEP)}
    class="mt-2 rounded-base px-2 py-1 text-sm underline focus-visible:ring-2 focus-visible:ring-accent-ink focus-visible:outline-none"
  >{$t('knowledge.upload.showMore')}</button>
{/if}
