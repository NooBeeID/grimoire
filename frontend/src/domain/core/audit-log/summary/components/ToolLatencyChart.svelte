<script lang="ts">
  import { locale, t } from '../../../../../shared/stores/i18n';
  import { theme } from '../../../../../shared/stores/theme';
  import type { ToolMetric } from '../types';
  import { chart } from './chart/attachChart';
  import { readChartTheme } from './chart/chartTheme';
  import { toolLatencyConfig } from './chart/configs';
  import ChartCard from './ChartCard.svelte';
  import { formatMs } from './format';

  let { rows, status, full = false }: { rows: ToolMetric[]; status: 'loading' | 'empty' | 'error' | 'ready'; full?: boolean } = $props();

  const cfg = $derived.by(() => {
    void $theme;
    return toolLatencyConfig(rows, readChartTheme());
  });
  const colors = $derived.by(() => {
    void $theme;
    const th = readChartTheme();
    return { p50: th.chart1, p95: th.chart2 };
  });
  const noLatency = $derived(rows.length > 0 && rows.every((r) => r.p50_ms === null && r.p95_ms === null));
  const shown = $derived(status === 'ready' && noLatency ? 'empty' : status);
</script>

<ChartCard
  title={$t('dashboard.charts.toolLatency')}
  status={shown}
  emptyText={noLatency ? $t('dashboard.emptyLatency') : $t('dashboard.empty')}
  height={200}
  class={full ? 'lg:col-span-2' : ''}
  legend={[
    { label: $t('dashboard.series.p50'), kind: 'bar', color: colors.p50 },
    { label: $t('dashboard.series.p95'), kind: 'bar', color: colors.p95 },
  ]}
  table={{
    columns: ['Tool', $t('dashboard.series.p50'), $t('dashboard.series.p95')],
    rows: rows.map((r) => [r.tool, formatMs(r.p50_ms, $locale), formatMs(r.p95_ms, $locale)]),
  }}
>
  <!-- svelte-ignore a11y_no_interactive_element_to_noninteractive_role -->
  <canvas role="img" aria-label={$t('dashboard.charts.toolLatency.aria', { n: rows.length })} {@attach chart(() => cfg)}></canvas>
</ChartCard>
