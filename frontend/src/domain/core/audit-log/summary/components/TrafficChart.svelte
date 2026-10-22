<script lang="ts">
  import { locale, t } from '../../../../../shared/stores/i18n';
  import { theme } from '../../../../../shared/stores/theme';
  import { bucketLabel, formatCount, utcOffsetCaption } from './format';
  import type { TrafficPoint } from '../types';
  import { chart } from './chart/attachChart';
  import { DESKTOP_MAX_TICKS, MOBILE_MAX_TICKS, narrow } from './chart/narrow';
  import { readChartTheme } from './chart/chartTheme';
  import { trafficConfig } from './chart/configs';
  import ChartCard from './ChartCard.svelte';

  let {
    points,
    bucket,
    status,
    totalCalls,
  }: { points: TrafficPoint[]; bucket: 'hour' | 'day'; status: 'loading' | 'empty' | 'error' | 'ready'; totalCalls: number } = $props();

  const cfg = $derived.by(() => {
    void $theme;
    return trafficConfig(points, bucket, $locale, readChartTheme(), $narrow ? MOBILE_MAX_TICKS : DESKTOP_MAX_TICKS);
  });
  const peak = $derived(points.reduce((m, p) => (p.calls > m.calls ? p : m), points[0] ?? { at: '', calls: 0, errors: 0 }));
  const ariaLabel = $derived(
    $t('dashboard.charts.traffic.aria', {
      n: points.length,
      max: formatCount(peak.calls, $locale),
      at: peak.at ? bucketLabel(peak.at, bucket, $locale) : '–',
    }),
  );
  const colors = $derived.by(() => {
    void $theme;
    const th = readChartTheme();
    return { calls: th.chart1, errors: th.danger };
  });
  const meta = $derived(
    (totalCalls === 0 ? $t('dashboard.empty') + '. ' : '') +
      (bucket === 'hour' ? $t('dashboard.timezone', { zone: utcOffsetCaption() }) : $t('dashboard.timezoneUtcDay')),
  );
</script>

<ChartCard
  title={$t('dashboard.charts.traffic')}
  {status}
  emptyText={$t('dashboard.empty')}
  {meta}
  height={220}
  legend={[
    { label: $t('dashboard.series.calls'), kind: 'line', color: colors.calls },
    { label: $t('dashboard.series.errors'), kind: 'line', color: colors.errors },
  ]}
  table={{
    columns: [$t('dashboard.charts.traffic'), $t('dashboard.series.calls'), $t('dashboard.series.errors')],
    rows: points.map((p) => [bucketLabel(p.at, bucket, $locale), String(p.calls), String(p.errors)]),
  }}
  class="lg:col-span-2"
>
  <!-- svelte-ignore a11y_no_interactive_element_to_noninteractive_role -->
  <canvas role="img" aria-label={ariaLabel} {@attach chart(() => cfg)}></canvas>
</ChartCard>
