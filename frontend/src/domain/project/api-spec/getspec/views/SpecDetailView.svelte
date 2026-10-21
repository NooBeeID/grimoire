<script lang="ts">
  import type { Snippet } from 'svelte';
  import { untrack } from 'svelte';
  import EmptyState from '../../../../../shared/components/EmptyState.svelte';
  import ErrorState from '../../../../../shared/components/ErrorState.svelte';
  import Spinner from '../../../../../shared/components/Spinner.svelte';
  import { ApiError, isApiError } from '../../../../../shared/services/http';
  import { t } from '../../../../../shared/stores/i18n';
  import ApiOverview from '../components/ApiOverview.svelte';
  import EndpointSidebar from '../components/EndpointSidebar.svelte';
  import OperationView from '../components/OperationView.svelte';
  import SpecHeader from '../components/SpecHeader.svelte';
  import { endpointKey, getOperation, parseOpParam } from '../model/operations';
  import { getSpec } from '../services/getspec.service';
  import { createSpecDetailStore, type SpecDetailStore } from '../stores/specDetail.store';
  import type { ApiSpec } from '../types';

  let {
    projectId,
    name,
    selectedOp = null,
    hrefForOp,
    backHref,
    actions,
    store: given,
  }: {
    projectId: string;
    name: string;
    selectedOp?: string | null;
    hrefForOp: (key: string | null) => string;
    backHref: string;
    actions?: Snippet<[ApiSpec]>;
    store?: SpecDetailStore;
  } = $props();

  const store = untrack(() => given ?? createSpecDetailStore());
  const query = store.query;
  const tree = store.tree;
  const matches = store.matches;
  let retry = $state(0);
  let ctrl: AbortController | null = null;

  async function load(id: string, n: string) {
    ctrl?.abort();
    const c = new AbortController();
    ctrl = c;
    store.start();
    try {
      const d = await getSpec(id, n, c.signal);
      if (!c.signal.aborted) store.set(d);
    } catch (e) {
      if (c.signal.aborted || (e instanceof DOMException && e.name === 'AbortError')) return;
      store.fail(isApiError(e) ? e : new ApiError({ status: 0, code: 'INTERNAL', message: String(e) }));
    }
  }
  $effect(() => {
    const id = projectId;
    const n = name;
    void retry;
    void untrack(() => load(id, n));
    return () => ctrl?.abort();
  });

  const op = $derived(parseOpParam(selectedOp));
  const detail = $derived($store.status === 'ready' ? $store.detail : null);
  const info = $derived(detail && op ? getOperation(detail.document, op.method, op.path) : null);
</script>

{#if $store.status === 'error' && $store.error.status === 404}
  <EmptyState title={$t('apispec.detail.notFound')} description={$t('apispec.detail.notFoundHint')}>
    {#snippet action()}<a href={backHref} class="text-accent-ink underline">{$t('apispec.detail.backToList')}</a>{/snippet}
  </EmptyState>
{:else if $store.status === 'error'}
  <ErrorState error={$store.error} onretry={() => retry++} />
{:else if !detail}
  <div class="flex justify-center p-8" aria-busy="true"><Spinner /></div>
{:else}
  <div class="flex flex-col gap-6">
    <SpecHeader spec={detail.spec} {actions} />
    <div class="grid gap-6 md:grid-cols-[18rem_1fr]">
      <details class="md:contents" open>
        <summary class="cursor-pointer text-sm font-semibold md:hidden">{$t('apispec.detail.endpoints')}</summary>
        <aside>
          <EndpointSidebar
            tree={$tree}
            matches={$matches}
            query={$query}
            selected={selectedOp}
            total={detail.endpoints.length}
            hrefFor={(ep) => hrefForOp(endpointKey(ep.method, ep.path))}
            onquery={(q) => store.setQuery(q)}
          />
        </aside>
      </details>
      <div class="min-w-0">
        {#if selectedOp && info && op}
          <OperationView doc={detail.document} method={op.method} path={op.path} {info} />
        {:else if selectedOp}
          <EmptyState title={$t('apispec.detail.opNotFound')}>
            {#snippet action()}<a href={hrefForOp(null)} class="text-accent-ink underline">{$t('apispec.detail.overview')}</a>{/snippet}
          </EmptyState>
        {:else}
          <ApiOverview doc={detail.document} />
        {/if}
      </div>
    </div>
  </div>
{/if}
