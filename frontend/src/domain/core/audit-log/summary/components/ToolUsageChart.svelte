<script lang="ts">
  import { locale, t } from '../../../../../shared/stores/i18n';
  import { theme } from '../../../../../shared/stores/theme';
  import { chart } from './chart/attachChart';
  import { readChartTheme } from './chart/chartTheme';
  import { toolUsageConfig } from './chart/configs';
  import ChartCard from './ChartCard.svelte';
  import { formatCount } from './format';

  let {
    rows,
    status,
    full = false,
  }: { rows: { tool: string; calls: number }[]; status: 'loading' | 'empty' | 'error' | 'ready'; full?: boolean } = $props();

  const cfg = $derived.by(() => {
    void $theme;
    return toolUsageConfig(rows, readChartTheme());
  });
</script>

<ChartCard
  title={$t('dashboard.charts.toolUsage')}
  {status}
  emptyText={$t('dashboard.empty')}
  height={200}
  class={full ? 'lg:col-span-2' : ''}
  table={{
    columns: ['Tool', $t('dashboard.series.calls')],
    rows: rows.map((r) => [r.tool, formatCount(r.calls, $locale)]),
  }}
>
  <!-- svelte-ignore a11y_no_interactive_element_to_noninteractive_role -->
  <canvas role="img" aria-label={$t('dashboard.charts.toolUsage.aria', { n: rows.length })} {@attach chart(() => cfg)}></canvas>
</ChartCard>
