<script lang="ts">
  import type { Snippet } from 'svelte';
  import { t } from '../../../../../shared/stores/i18n';
  import type { LegendItem } from './chart/legend';
  import { MOBILE_PLOT_HEIGHT, narrow } from './chart/narrow';
  import ChartLegend from './ChartLegend.svelte';

  let {
    title,
    status,
    emptyText,
    legend,
    meta,
    table,
    height = 220,
    bare = false,
    class: klass = '',
    children,
  }: {
    title: string;
    status: 'loading' | 'empty' | 'error' | 'ready';
    emptyText: string;
    legend?: LegendItem[];
    meta?: string;
    table: { columns: string[]; rows: string[][] };
    height?: 180 | 200 | 220;
    bare?: boolean;
    class?: string;
    children?: Snippet;
  } = $props();

  const plotHeight = $derived($narrow ? MOBILE_PLOT_HEIGHT : height);
</script>

<figure class="m-0 flex min-w-0 flex-col gap-4 {bare ? '' : 'rounded-card border border-border bg-surface p-4 sm:p-6'} {klass}" aria-busy={status === 'loading'}>
  <figcaption class="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1">
    <span class={bare ? 'text-sm font-medium text-muted' : 'text-base font-semibold'}>{title}</span>
    {#if legend && legend.length > 0 && status === 'ready'}<ChartLegend items={legend} />{/if}
  </figcaption>
  <div class="relative" style="height:{plotHeight}px">
    {#if status === 'loading'}
      <div class="h-full w-full bg-raised" aria-hidden="true"></div>
    {:else if status === 'empty'}
      <p class="grid h-full place-items-center text-sm text-muted">{emptyText}</p>
    {:else if status === 'error'}
      <p class="grid h-full place-items-center text-sm text-muted">{$t('dashboard.unavailable')}</p>
    {:else}
      <div class="h-full">{@render children?.()}</div>
      <table class="sr-only">
        <caption>{title}</caption>
        <thead><tr>{#each table.columns as c (c)}<th scope="col">{c}</th>{/each}</tr></thead>
        <tbody>
          {#each table.rows as r, i (i)}<tr>{#each r as cell, j (j)}<td>{cell}</td>{/each}</tr>{/each}
        </tbody>
      </table>
    {/if}
  </div>
  {#if meta && status === 'ready'}<p class="text-xs text-muted">{meta}</p>{/if}
</figure>
