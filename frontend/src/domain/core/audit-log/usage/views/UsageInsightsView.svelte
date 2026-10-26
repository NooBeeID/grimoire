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
  let tab = $state<'top' | 'unread' | 'search'>('top');
  // Daftar panjang dipotong ke PREVIEW baris; sisanya lewat tombol.
  const PREVIEW = 10;
  let expanded = $state(false);
  const cut = <T,>(st: { items: T[] }) => (expanded ? st.items : st.items.slice(0, PREVIEW));
  const total = $derived(tab === 'top' ? $store.top_files.items.length : tab === 'unread' ? $store.unread_files.items.length : $store.zero_queries.items.length);
  $effect(() => {
    void tab;
    expanded = false;
  });
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

  {#if tab === 'top'}
    <TopFilesTable state={{ ...$store.top_files, items: cut($store.top_files) }} onretry={() => store.retry('top_files')} />
  {:else if tab === 'unread'}
    <UnreadFilesTable state={{ ...$store.unread_files, items: cut($store.unread_files) }} onretry={() => store.retry('unread_files')} />
  {:else}
    <ZeroQueriesTable state={{ ...$store.zero_queries, items: cut($store.zero_queries) }} onretry={() => store.retry('zero_queries')} />
  {/if}
  {#if total > PREVIEW}
    <button type="button" class="self-start cursor-pointer rounded-full px-4 py-1.5 text-sm font-medium text-accent-ink hover:bg-raised" onclick={() => (expanded = !expanded)}>
      {expanded ? $t('dashboard.usage.showLess') : $t('dashboard.usage.showMore', { n: total })}
    </button>
  {/if}

  <KeyUsageTable state={$store.keys} onretry={() => store.retry('keys')} />
</section>
