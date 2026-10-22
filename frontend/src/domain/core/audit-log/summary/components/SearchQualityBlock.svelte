<script lang="ts">
  import Button from '../../../../../shared/components/Button.svelte';
  import EmptyState from '../../../../../shared/components/EmptyState.svelte';
  import { locale, t } from '../../../../../shared/stores/i18n';
  import type { SearchQuality } from '../types';
  import { formatPercent } from './format';
  import RankChart from './RankChart.svelte';

  let {
    quality,
    loading,
    retrying,
    onretry,
    onpickproject,
  }: {
    quality: SearchQuality | null;
    loading: boolean;
    retrying: boolean;
    onretry: () => void;
    onpickproject: () => void;
  } = $props();

  const metrics = $derived(
    quality && quality.available
      ? ([
          ['zeroResult', quality.zero_result],
          ['followUp', quality.follow_up],
          ['reformulation', quality.reformulation],
        ] as const)
      : [],
  );
</script>

<section class="flex min-w-0 flex-col gap-4 rounded-card border border-border bg-surface p-4 sm:p-6 lg:col-span-2" aria-busy={loading}>
  <h2 class="text-base font-semibold">{$t('dashboard.quality.title')}</h2>
  {#if loading}
    <div class="h-[180px] bg-raised" aria-hidden="true"></div>
  {:else if quality === null}
    <EmptyState title={$t('dashboard.quality.pickProject')}>
      {#snippet action()}
        <Button variant="secondary" size="sm" onclick={onpickproject}>{$t('dashboard.quality.pickProjectAction')}</Button>
      {/snippet}
    </EmptyState>
  {:else if !quality.available}
    <div role="status" class="flex flex-wrap items-center gap-3">
      <p class="text-sm text-muted">{$t('dashboard.quality.unavailable')}</p>
      <Button variant="secondary" size="sm" loading={retrying} onclick={onretry}>{$t('common.retry')}</Button>
    </div>
  {:else}
    <dl class="m-0 grid grid-cols-3 divide-x divide-rule border-y border-rule">
      {#each metrics as [key, m] (key)}
        <div class="flex flex-col gap-1 py-4 pr-2 pl-3 first:pl-0 sm:pl-6 sm:first:pl-0">
          <dt class="text-sm text-muted">{$t(`dashboard.quality.${key}`)}</dt>
          <dd class="m-0 flex flex-wrap items-baseline gap-x-2 text-[28px] leading-9 tabular-nums">
            {formatPercent(m.rate, $locale, m.denominator)}
            <span class="text-sm text-muted">{m.count}/{m.denominator}</span>
          </dd>
        </div>
      {/each}
    </dl>
    <p class="text-xs text-muted">{$t('dashboard.quality.hint')}</p>
    <RankChart buckets={quality.rank_opened} status={quality.searches === 0 ? 'empty' : 'ready'} />
    {#if quality.legacy_searches > 0}
      <p class="text-xs text-muted">{$t('dashboard.quality.legacy', { n: quality.legacy_searches })}</p>
    {/if}
  {/if}
</section>
