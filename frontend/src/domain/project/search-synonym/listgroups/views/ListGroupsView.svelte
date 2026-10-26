<script lang="ts">
  import { untrack } from 'svelte';
  import Button from '../../../../../shared/components/Button.svelte';
  import EmptyState from '../../../../../shared/components/EmptyState.svelte';
  import ErrorState from '../../../../../shared/components/ErrorState.svelte';
  import { ApiError, isApiError } from '../../../../../shared/services/http';
  import { t } from '../../../../../shared/stores/i18n';
  import SynonymGroupsTable from '../components/SynonymGroupsTable.svelte';
  import { listGroups } from '../services/listgroups.service';
  import type { GroupListStore } from '../stores/groupList.store';
  import type { SynonymGroup } from '../types';

  let {
    projectId,
    store,
    oncreate,
    onedit,
    ondelete,
    onprojectgone,
  }: {
    projectId: string;
    store: GroupListStore;
    oncreate: () => void;
    onedit: (g: SynonymGroup) => void;
    ondelete: (g: SynonymGroup) => void;
    onprojectgone: () => void;
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
      const r = await listGroups(id, c.signal);
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

  const rows = $derived(
    $store.status === 'ready' ? $store.items : $store.status === 'loading' || $store.status === 'error' ? ($store.previous ?? []) : [],
  );
  const busy = $derived($store.status === 'idle' || $store.status === 'loading');
  const firstLoad = $derived(busy && rows.length === 0);
</script>

<section class="flex flex-col gap-4" aria-busy={busy}>
  <header class="flex flex-wrap items-center justify-between gap-2">
    <div>
      <h2 class="text-base font-semibold">{$t('searchSynonym.title')}</h2>
      <p class="mt-1 max-w-prose text-sm text-muted">{$t('searchSynonym.intro')}</p>
    </div>
    <Button onclick={oncreate}>{$t('searchSynonym.create.open')}</Button>
  </header>

  {#if $store.status === 'error'}
    <ErrorState error={$store.error} onretry={() => store.invalidate()} />
  {:else if $store.status === 'ready' && rows.length === 0}
    <EmptyState title={$t('searchSynonym.empty')} description={$t('searchSynonym.emptyHint')} />
  {:else if $store.status === 'notFound'}
    <div></div>
  {:else}
    <SynonymGroupsTable items={rows} loading={firstLoad} {onedit} {ondelete} />
  {/if}
</section>
