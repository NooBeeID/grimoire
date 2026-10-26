<script lang="ts">
  import { onDestroy } from 'svelte';
  import { t } from '../../../../../shared/stores/i18n';
  import InsightTabs from '../components/InsightTabs.svelte';
  import KeyUsageTable from '../components/KeyUsageTable.svelte';
  import TopFilesTable from '../components/TopFilesTable.svelte';
  import UnreadFilesTable from '../components/UnreadFilesTable.svelte';
  import ZeroQueriesTable from '../components/ZeroQueriesTable.svelte';
  import { createUsageStore } from '../stores/usage.store';
  import { usageFilterFromQuery, usageFilterKey } from '../stores/usageQuery';

  let { query }: { query: URLSearchParams } = $props();

  const store = createUsageStore();
  let tab = $state<'documents' | 'search'>('documents');
  const filter = $derived(usageFilterFromQuery(query));
  const key = $derived(usageFilterKey(filter));

  $effect(() => {
    void key;
    store.load(filter);
  });
  onDestroy(() => store.dispose());
</script>

<section class="flex flex-col gap-4" aria-labelledby="usage-title" data-testid="usage-insights">
  <header class="flex flex-wrap items-end justify-between gap-3">
    <div>
      <h2 id="usage-title" class="text-lg font-semibold tracking-tight">{$t('dashboard.usage.title')}</h2>
      <p class="mt-1 text-sm text-muted">{$t('dashboard.usage.subtitle')}</p>
    </div>
    <InsightTabs value={tab} onchange={(v) => (tab = v)} />
  </header>

  {#if tab === 'documents'}
    <div class="grid gap-4 lg:grid-cols-2">
      <TopFilesTable state={$store.top_files} onretry={() => store.retry('top_files')} />
      <UnreadFilesTable state={$store.unread_files} onretry={() => store.retry('unread_files')} />
    </div>
  {:else}
    <ZeroQueriesTable state={$store.zero_queries} onretry={() => store.retry('zero_queries')} />
  {/if}

  <KeyUsageTable state={$store.keys} onretry={() => store.retry('keys')} />
</section>
