<script lang="ts">
  import { untrack } from 'svelte';
  import Button from '../../../../../shared/components/Button.svelte';
  import EmptyState from '../../../../../shared/components/EmptyState.svelte';
  import ErrorState from '../../../../../shared/components/ErrorState.svelte';
  import { ApiError, isApiError } from '../../../../../shared/services/http';
  import { t } from '../../../../../shared/stores/i18n';
  import KeysTable from '../components/KeysTable.svelte';
  import { listKeys } from '../services/listkeys.service';
  import { activeCount, type KeyListStore } from '../stores/keyList.store';
  import type { ApiKey } from '../types';

  let {
    projectId,
    store,
    oncreate,
    onrevoke,
    onprojectgone,
    auditHref,
  }: {
    projectId: string;
    store: KeyListStore;
    oncreate: () => void;
    onrevoke: (key: ApiKey) => void;
    onprojectgone: () => void;
    auditHref?: (key: ApiKey) => string;
  } = $props();

  let ver = $state(0);
  $effect(() => store.version.subscribe((v) => (ver = v)));
  let ctrl: AbortController | null = null;

  async function load(id: string) {
    ctrl?.abort();
    const c = new AbortController();
    ctrl = c;
    store.begin(id);
    try {
      const r = await listKeys(id, c.signal);
      if (!c.signal.aborted) store.resolve(id, r.items);
    } catch (e) {
      if (c.signal.aborted || (e instanceof DOMException && e.name === 'AbortError')) return;
      store.fail(id, isApiError(e) ? e : new ApiError({ status: 0, code: 'INTERNAL', message: String(e) }));
    }
  }

  $effect(() => {
    const id = projectId;
    void ver;
    void untrack(() => load(id));
    return () => ctrl?.abort();
  });

  let gone = false;
  $effect(() => {
    if ($store.status === 'notFound' && !gone) {
      gone = true;
      onprojectgone();
    }
  });

  const rows = $derived($store.status === 'ready' ? $store.items : $store.status === 'loading' || $store.status === 'error' ? ($store.previous ?? []) : []);
  const busy = $derived($store.status === 'idle' || $store.status === 'loading');
  const firstLoad = $derived(busy && rows.length === 0);
</script>

<section class="flex flex-col gap-4" aria-busy={busy}>
  <header class="flex flex-wrap items-center justify-between gap-2">
    <div>
      <h2 class="text-base font-semibold">
        {$t('project.apikey.title')}{#if $store.status === 'ready'}<span class="ml-2 font-normal text-muted">{$t('project.apikey.activeCount', { n: activeCount($store) })}</span>{/if}
      </h2>
      <p class="mt-1 max-w-prose text-sm text-muted">{$t('project.apikey.intro')}</p>
    </div>
    <Button onclick={oncreate}>{$t('project.apikey.create.open')}</Button>
  </header>

  {#if $store.status === 'error'}
    <ErrorState error={$store.error} onretry={() => store.invalidate()} />
  {:else if $store.status === 'ready' && rows.length === 0}
    <EmptyState title={$t('project.apikey.empty')} description={$t('project.apikey.emptyHint')}>
      {#snippet action()}<Button onclick={oncreate}>{$t('project.apikey.create.open')}</Button>{/snippet}
    </EmptyState>
  {:else if $store.status === 'notFound'}
    <div></div>
  {:else}
    <KeysTable
      items={rows}
      loading={firstLoad}
      {onrevoke}
      {auditHref}
    />
  {/if}
</section>
