<script lang="ts">
  import { locale, t } from '../../../../../shared/stores/i18n';
  import { theme } from '../../../../../shared/stores/theme';
  import type { ToolMetric } from '../types';
  import { chart } from './chart/attachChart';
  import { readChartTheme } from './chart/chartTheme';
  import { toolStatusConfig } from './chart/configs';
  import ChartCard from './ChartCard.svelte';
  import { formatCount } from './format';

  let { rows, status }: { rows: ToolMetric[]; status: 'loading' | 'empty' | 'error' | 'ready' } = $props();

  const cfg = $derived.by(() => {
    void $theme;
    return toolStatusConfig(rows, readChartTheme());
  });
  const colors = $derived.by(() => {
    void $theme;
    const th = readChartTheme();
    return { ok: th.chart1, error: th.danger };
  });
</script>

<ChartCard
  title={$t('dashboard.charts.toolStatus')}
  {status}
  emptyText={$t('dashboard.empty')}
  height={200}
  legend={[
    { label: $t('dashboard.series.ok'), kind: 'bar', color: colors.ok },
    { label: $t('dashboard.series.error'), kind: 'hatch', color: colors.error },
  ]}
  table={{
    columns: ['Tool', $t('dashboard.series.ok'), $t('dashboard.series.error')],
    rows: rows.map((r) => [r.tool, formatCount(r.ok, $locale), formatCount(r.error, $locale)]),
  }}
>
  <!-- svelte-ignore a11y_no_interactive_element_to_noninteractive_role -->
  <canvas role="img" aria-label={$t('dashboard.charts.toolStatus.aria', { n: rows.length })} {@attach chart(() => cfg)}></canvas>
</ChartCard>
