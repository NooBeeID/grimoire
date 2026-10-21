<script lang="ts">
  import { untrack } from 'svelte';
  import EmptyState from '../../../../../shared/components/EmptyState.svelte';
  import ErrorState from '../../../../../shared/components/ErrorState.svelte';
  import { ApiError, isApiError } from '../../../../../shared/services/http';
  import { t } from '../../../../../shared/stores/i18n';
  import { errorMessage } from '../../base';
  import ScopeToggle from '../components/ScopeToggle.svelte';
  import SearchResults from '../components/SearchResults.svelte';
  import { searchKnowledge } from '../services/search.service';
  import { createSearchStore } from '../stores/search.store';
  import { DEFAULT_LIMIT, MAX_LIMIT } from '../types';

  let {
    projectId,
    q,
    scope,
    contextDir,
    onscopechange,
    onopen,
    onprojectgone,
  }: {
    projectId: string;
    q: string;
    scope: string;
    contextDir: string;
    onscopechange: (scope: string) => void;
    onopen: (path: string) => void;
    onprojectgone: () => void;
  } = $props();

  const store = createSearchStore();
  let limit = $state(DEFAULT_LIMIT);
  let retry = $state(0);
  let ctrl: AbortController | null = null;

  $effect(() => {
    const query = q.trim();
    const prefix = scope;
    const lim = limit;
    void retry;
    void projectId;
    if (query === '') {
      ctrl?.abort();
      store.reset();
      return;
    }
    untrack(async () => {
      ctrl?.abort();
      const c = new AbortController();
      ctrl = c;
      const key = `${query}|${prefix}|${lim}`;
      store.begin(key);
      try {
        const r = await searchKnowledge(projectId, { q: query, prefix, limit: lim }, c.signal);
        if (!c.signal.aborted) store.resolve(key, r);
      } catch (e) {
        if (c.signal.aborted || (e instanceof DOMException && e.name === 'AbortError')) return;
        const err = isApiError(e) ? e : new ApiError({ status: 0, code: 'INTERNAL', message: String(e) });
        if (err.status === 404) onprojectgone();
        store.fail(key, err);
      }
    });
    return () => ctrl?.abort();
  });

  // Kueri baru mengembalikan batas ke default.
  let lastQ = untrack(() => q);
  $effect(() => {
    if (q !== lastQ) {
      lastQ = q;
      limit = DEFAULT_LIMIT;
    }
  });

  const shown = $derived($store.status === 'ready' ? $store.result : $store.status === 'loading' ? $store.previous : null);
</script>

{#if q.trim() !== ''}
  <div class="flex flex-col gap-2" aria-busy={$store.status === 'loading'}>
    <ScopeToggle {contextDir} {scope} onchange={onscopechange} />
    {#if $store.status === 'error'}
      {#if $store.error.status === 400}
        <p role="alert" class="px-1 text-sm text-danger">{errorMessage($store.error, $t)}</p>
      {:else}
        <ErrorState error={$store.error} onretry={() => retry++} />
      {/if}
    {:else if !shown}
      <p class="px-1 text-sm text-muted">{$t('common.loading')}</p>
    {:else if shown.items.length === 0}
      <EmptyState title={$t('knowledge.search.empty')} />
    {:else}
      <SearchResults
        items={shown.items}
        total={shown.total}
        {limit}
        dim={$store.status === 'loading'}
        {onopen}
        onmore={() => (limit = MAX_LIMIT)}
      />
    {/if}
  </div>
{/if}
