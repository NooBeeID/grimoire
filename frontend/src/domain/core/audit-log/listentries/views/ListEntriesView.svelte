<script lang="ts">
  import { untrack } from 'svelte';
  import Button from '../../../../../shared/components/Button.svelte';
  import { isApiError, ApiError } from '../../../../../shared/services/http';
  import { t } from '../../../../../shared/stores/i18n';
  import AuditTable from '../components/AuditTable.svelte';
  import FilterBar from '../components/FilterBar.svelte';
  import { listEntries, resolveRange } from '../services/listentries.service';
  import { filterFromQuery, filterKey, filterToQuery, validateFilter } from '../stores/filterQuery';
  import PageHeader from '../../../../../shared/components/PageHeader.svelte';
  import { createListEntriesStore } from '../stores/listentries.store';
  import { EMPTY_FILTER } from '../types';

  let {
    query,
    selectedId,
    onquerychange,
    onselect,
  }: {
    query: URLSearchParams;
    selectedId: string | null;
    onquerychange: (q: URLSearchParams) => void;
    onselect: (id: string) => void;
  } = $props();

  const store = createListEntriesStore();
  const rows = store.rows;
  const hasMore = store.hasMore;
  const suggestions = store.suggestions;

  const filter = $derived(filterFromQuery(query));
  const key = $derived(filterKey(filter));
  const filtered = $derived(key !== filterKey(EMPTY_FILTER));
  const errors = $derived(validateFilter(filter));
  let announce = $state('');
  let refreshTick = $state(0);
  let ctrl: AbortController | null = null;

  const toApi = (e: unknown) => (isApiError(e) ? e : new ApiError({ status: 0, code: 'INTERNAL', message: String(e) }));
  const isAbort = (e: unknown, c: AbortController) =>
    c.signal.aborted || (e instanceof DOMException && e.name === 'AbortError');

  async function loadFirst(f: typeof filter, k: string) {
    ctrl?.abort();
    const c = new AbortController();
    ctrl = c;
    const range = resolveRange(f, new Date());
    store.reset(k, range);
    store.begin();
    try {
      const res = await listEntries(f, range, null, c.signal);
      if (c.signal.aborted) return;
      store.pushPage({ cursor: null, items: res.items, nextCursor: res.nextCursor });
      announce = $t('auditLog.resultCount', { n: res.items.length });
    } catch (e) {
      if (isAbort(e, c)) return;
      store.fail(toApi(e));
    }
  }

  $effect(() => {
    void refreshTick;
    const k = key;
    // Hanya key yang memicu muat ulang; perubahan query lain (mis. entry) diabaikan.
    const f = untrack(() => filter);
    void untrack(() => loadFirst(f, k));
    return () => ctrl?.abort();
  });

  async function loadMore(cursorOverride?: string | null) {
    const s = $store;
    const cursor = cursorOverride ?? $store.pages.at(-1)?.nextCursor ?? null;
    if (!cursor || s.moreStatus === 'loading') return;
    const c = ctrl ?? new AbortController();
    store.beginMore();
    try {
      const res = await listEntries(filter, s.range, cursor, c.signal);
      if (c.signal.aborted) return;
      store.pushPage({ cursor, items: res.items, nextCursor: res.nextCursor });
      announce = $t('auditLog.resultCount', { n: $rows.length });
    } catch (e) {
      if (isAbort(e, c)) return;
      store.failMore(toApi(e));
    }
  }

  const apply = (f: typeof filter) => onquerychange(filterToQuery(f, query));
  const reset = () => onquerychange(filterToQuery(EMPTY_FILTER, query));
</script>

<section class="flex min-w-0 flex-col gap-4">
  <PageHeader title={$t('auditLog.title')} description={$t('auditLog.subtitle')}>
    {#snippet action()}
      <Button variant="secondary" onclick={() => refreshTick++}>{$t('auditLog.refresh')}</Button>
    {/snippet}
  </PageHeader>

  <FilterBar value={filter} suggestions={$suggestions} {errors} onapply={apply} onreset={reset} />

  <div class="sr-only" aria-live="polite">{announce}</div>

  <AuditTable
    rows={$rows}
    status={$store.status}
    error={$store.error}
    {selectedId}
    hasMore={$hasMore}
    moreStatus={$store.moreStatus}
    moreError={$store.moreError}
    {filtered}
    {onselect}
    onloadmore={() => loadMore()}
    onretry={() => refreshTick++}
    onretrymore={() => loadMore()}
    onresetfilter={reset}
  />
</section>
