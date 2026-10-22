<script lang="ts">
  import { t } from '../../../../../shared/stores/i18n';
  import { theme } from '../../../../../shared/stores/theme';
  import { chart } from './chart/attachChart';
  import { readChartTheme } from './chart/chartTheme';
  import { rankConfig } from './chart/configs';
  import ChartCard from './ChartCard.svelte';

  let {
    buckets,
    status,
  }: { buckets: { label: string; count: number }[]; status: 'loading' | 'empty' | 'error' | 'ready' } = $props();

  const cfg = $derived.by(() => {
    void $theme;
    return rankConfig(buckets, readChartTheme(), $t);
  });
</script>

<ChartCard
  title={$t('dashboard.quality.rank')}
  {status}
  emptyText={$t('dashboard.quality.noSearches')}
  height={180}
  bare
  table={{
    columns: [$t('dashboard.quality.rank'), $t('dashboard.series.calls')],
    rows: buckets.map((b) => [b.label === 'not_in_results' ? $t('dashboard.quality.notInResults') : b.label, String(b.count)]),
  }}
>
  <!-- svelte-ignore a11y_no_interactive_element_to_noninteractive_role -->
  <canvas role="img" aria-label={$t('dashboard.quality.rank')} {@attach chart(() => cfg)}></canvas>
</ChartCard>
