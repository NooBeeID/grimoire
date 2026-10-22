<script lang="ts">
  import { locale, t } from '../../../../../shared/stores/i18n';
  import { theme } from '../../../../../shared/stores/theme';
  import type { TopProject } from '../types';
  import { chart } from './chart/attachChart';
  import { readChartTheme } from './chart/chartTheme';
  import { topProjectsConfig } from './chart/configs';
  import ChartCard from './ChartCard.svelte';
  import { formatCount } from './format';

  let {
    rows,
    status,
    onpick,
  }: { rows: TopProject[]; status: 'loading' | 'empty' | 'error' | 'ready'; onpick?: (projectId: string) => void } = $props();

  const cfg = $derived.by(() => {
    void $theme;
    return topProjectsConfig(rows, readChartTheme(), onpick);
  });
</script>

<ChartCard
  title={$t('dashboard.charts.topProjects')}
  {status}
  emptyText={$t('dashboard.empty')}
  height={200}
  table={{
    columns: [$t('dashboard.project.label'), $t('dashboard.series.calls')],
    rows: rows.map((r) => [r.project_label, formatCount(r.calls, $locale)]),
  }}
>
  <!-- svelte-ignore a11y_no_interactive_element_to_noninteractive_role -->
  <canvas role="img" aria-label={$t('dashboard.charts.topProjects.aria', { n: rows.length })} {@attach chart(() => cfg)}></canvas>
</ChartCard>
