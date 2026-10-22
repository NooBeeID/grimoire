<script lang="ts">
  import { onMount, untrack } from 'svelte';
  import SearchQualityBlock from '../components/SearchQualityBlock.svelte';
  import SummaryFigures from '../components/SummaryFigures.svelte';
  import SummaryToolbar from '../components/SummaryToolbar.svelte';
  import ToolLatencyChart from '../components/ToolLatencyChart.svelte';
  import ToolStatusChart from '../components/ToolStatusChart.svelte';
  import ToolUsageChart from '../components/ToolUsageChart.svelte';
  import TopProjectsChart from '../components/TopProjectsChart.svelte';
  import TrafficChart from '../components/TrafficChart.svelte';
  import { createProjectOptionsStore } from '../stores/projectOptions.store';
  import { createSummaryStore } from '../stores/summary.store';
  import { summaryFilterFromQuery, summaryFilterKey, summaryFilterToQuery } from '../stores/summaryQuery';
  import { DEFAULT_SUMMARY_RANGE, type SummaryRange } from '../types';

  let {
    query,
    onquerychange,
  }: { query: URLSearchParams; onquerychange: (q: URLSearchParams) => void } = $props();

  const store = createSummaryStore();
  const projects = createProjectOptionsStore();

  const filter = $derived(summaryFilterFromQuery(query));
  const key = $derived(summaryFilterKey(filter));
  const data = $derived($store.data);
  const loading = $derived($store.status === 'idle' || $store.status === 'loading');
  const failed = $derived($store.status === 'error');
  const hasProject = $derived(filter.projectId !== null);

  $effect(() => {
    void key;
    const f = untrack(() => filter);
    void store.load(f);
  });
  onMount(() => {
    void projects.load();
    return () => {
      store.dispose();
      projects.dispose();
    };
  });

  const chartStatus = (empty: boolean) => (loading ? 'loading' : failed ? 'error' : empty ? 'empty' : 'ready') as
    | 'loading'
    | 'error'
    | 'empty'
    | 'ready';

  const setRange = (r: SummaryRange) => onquerychange(summaryFilterToQuery({ ...filter, range: r }));
  const setProject = (id: string | null) => onquerychange(summaryFilterToQuery({ ...filter, projectId: id }));
  const reset = () => onquerychange(summaryFilterToQuery({ range: DEFAULT_SUMMARY_RANGE, projectId: null }));
  const fallbackLabel = $derived(data?.top_projects[0]?.project_label ?? null);
  let root: HTMLElement | undefined = $state();
  const focusPicker = () => root?.querySelector<HTMLElement>('select')?.focus();
  const metrics = $derived(data?.tool_metrics ?? []);
  // Bila jumlah kartu setengah lebar ganjil, kartu terakhir melebar penuh.
  const oddCards = $derived(hasProject);
</script>

<div bind:this={root} class="flex flex-col gap-6">
  <SummaryToolbar
    range={filter.range}
    projectId={filter.projectId}
    projects={$projects}
    {fallbackLabel}
    loadedAt={$store.loadedAt}
    refreshing={$store.refreshing}
    refreshError={$store.refreshError !== null}
    onrange={setRange}
    onproject={setProject}
    onreload={() => void store.reload()}
    onretryprojects={() => void projects.retry()}
  />

  <SummaryFigures
    status={$store.status}
    {data}
    error={$store.error}
    refreshing={$store.refreshing}
    onretry={() => void store.reload()}
    onreset={reset}
  />

  <div class="grid grid-cols-1 gap-6 lg:grid-cols-2">
    <TrafficChart
      points={data?.traffic ?? []}
      bucket={data?.bucket ?? 'day'}
      totalCalls={data?.total_calls ?? 0}
      status={chartStatus(false)}
    />
    {#if hasProject}
      <SearchQualityBlock
        quality={data?.search_quality ?? null}
        {loading}
        retrying={$store.qualityRetrying}
        onretry={() => void store.retryQuality()}
        onpickproject={focusPicker}
      />
    {:else}
      <TopProjectsChart rows={data?.top_projects ?? []} status={chartStatus((data?.top_projects.length ?? 0) === 0)} onpick={setProject} />
    {/if}
    <ToolUsageChart rows={data?.tool_usage ?? []} status={chartStatus((data?.tool_usage.length ?? 0) === 0)} />
    <ToolStatusChart rows={metrics} status={chartStatus(metrics.length === 0)} />
    <ToolLatencyChart rows={metrics} status={chartStatus(metrics.length === 0)} full={oddCards} />
    {#if !hasProject}
      <SearchQualityBlock
        quality={data?.search_quality ?? null}
        {loading}
        retrying={$store.qualityRetrying}
        onretry={() => void store.retryQuality()}
        onpickproject={focusPicker}
      />
    {/if}
  </div>
</div>
