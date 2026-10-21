<script lang="ts">
  import { untrack } from 'svelte';
  import Button from '../../../../../shared/components/Button.svelte';
  import EmptyState from '../../../../../shared/components/EmptyState.svelte';
  import ErrorState from '../../../../../shared/components/ErrorState.svelte';
  import Pagination from '../../../../../shared/components/Pagination.svelte';
  import Spinner from '../../../../../shared/components/Spinner.svelte';
  import { ApiError, isApiError } from '../../../../../shared/services/http';
  import { t } from '../../../../../shared/stores/i18n';
  import ProjectSearchBar from '../components/ProjectSearchBar.svelte';
  import ProjectTable from '../components/ProjectTable.svelte';
  import { listProjects, parseListQuery, toSearchParams } from '../services/listprojects.service';
  import { createProjectListStore } from '../stores/projectList.store';
  import { PAGE_SIZE } from '../types';

  let {
    query,
    onquerychange,
    canCreate,
    oncreate,
  }: {
    query: URLSearchParams;
    onquerychange: (next: URLSearchParams) => void;
    canCreate: boolean;
    oncreate: () => void;
  } = $props();

  const store = createProjectListStore();
  const parsed = $derived(parseListQuery(query));
  const key = $derived(toSearchParams(parsed).toString());
  let retryTick = $state(0);
  let announce = $state('');
  let jumped = false;
  let ctrl: AbortController | null = null;

  const toApi = (e: unknown) => (isApiError(e) ? e : new ApiError({ status: 0, code: 'INTERNAL', message: String(e) }));

  async function load(q: typeof parsed, k: string) {
    ctrl?.abort();
    const c = new AbortController();
    ctrl = c;
    store.begin(k);
    try {
      const res = await listProjects(q, c.signal);
      if (c.signal.aborted) return;
      store.resolve(k, res);
      announce = $t('project.list.resultCount', { total: res.total });
      if (res.items.length === 0 && res.total > 0 && q.page > 1 && !jumped) {
        jumped = true;
        const next = toSearchParams({ q: q.q, page: Math.ceil(res.total / PAGE_SIZE) });
        onquerychange(next);
      }
    } catch (e) {
      if (c.signal.aborted || (e instanceof DOMException && e.name === 'AbortError')) return;
      store.fail(k, toApi(e));
    }
  }

  $effect(() => {
    void retryTick;
    const k = key;
    const q = untrack(() => parsed);
    void load(q, k);
    return () => ctrl?.abort();
  });

  function search(q: string) {
    onquerychange(toSearchParams({ q, page: 1 }));
  }
  function goto(page: number) {
    onquerychange(toSearchParams({ q: parsed.q, page }));
  }

  const data = $derived($store.status === 'ready' ? $store.data : $store.status === 'loading' ? $store.previous : null);
  const loadingFirst = $derived($store.status === 'loading' && $store.previous === null);
  const busy = $derived($store.status === 'loading');
</script>

<section class="flex flex-col gap-4" aria-busy={busy}>
  <header class="flex flex-wrap items-center justify-between gap-2">
    <h2 class="text-xl font-semibold">{$t('project.list.title')}</h2>
    {#if canCreate}<Button onclick={oncreate}>{$t('project.create.open')}</Button>{/if}
  </header>

  <ProjectSearchBar value={parsed.q} onsearch={search} />

  <div class="sr-only" aria-live="polite">{announce}</div>

  {#if loadingFirst}
    <div class="p-8 text-center"><Spinner /></div>
  {:else if $store.status === 'error'}
    <ErrorState error={$store.error} onretry={() => retryTick++} />
  {:else if data}
    {#if data.items.length === 0 && data.total === 0}
      {#if parsed.q}
        <EmptyState title={$t('project.list.emptyFiltered', { q: parsed.q })}>
          {#snippet action()}<Button variant="secondary" onclick={() => search('')}>{$t('project.list.reset')}</Button>{/snippet}
        </EmptyState>
      {:else}
        <EmptyState title={$t('project.list.empty')} description={$t('project.list.emptyHint')}>
          {#snippet action()}{#if canCreate}<Button onclick={oncreate}>{$t('project.create.open')}</Button>{/if}{/snippet}
        </EmptyState>
      {/if}
    {:else}
      <div class={busy ? 'opacity-60' : ''}>
        <ProjectTable items={data.items} />
      </div>
      {#if data.total > PAGE_SIZE}
        <Pagination page={parsed.page} pageSize={PAGE_SIZE} total={data.total} onchange={goto} />
      {/if}
    {/if}
  {/if}
</section>
