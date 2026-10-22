<script lang="ts">
  import { untrack, type Snippet } from 'svelte';
  import Button from '../../../../../shared/components/Button.svelte';
  import EmptyState from '../../../../../shared/components/EmptyState.svelte';
  import ErrorState from '../../../../../shared/components/ErrorState.svelte';
  import { ApiError, isApiError } from '../../../../../shared/services/http';
  import { t } from '../../../../../shared/stores/i18n';
  import SpecQuota from '../components/SpecQuota.svelte';
  import SpecTable from '../components/SpecTable.svelte';
  import { listSpecs } from '../services/listspecs.service';
  import type { SpecListStore } from '../stores/specList.store';

  let {
    projectId,
    store,
    onupload,
    onprojectgone,
    headeractions,
  }: {
    projectId: string;
    store: SpecListStore;
    onupload: () => void;
    onprojectgone: () => void;
    // Slot aksi tambahan sebelum tombol Upload (diisi komposer app/; domain ini tidak tahu isinya).
    headeractions?: Snippet;
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
      const r = await listSpecs(id, c.signal);
      if (!c.signal.aborted) store.resolve(id, r.items, r.total, r.limit);
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
  const quota = $derived(
    $store.status === 'ready' || $store.status === 'loading' ? { total: $store.total, limit: $store.limit } : null,
  );
</script>

<section class="flex flex-col gap-4" aria-busy={busy}>
  <header class="flex flex-wrap items-start justify-between gap-2">
    <div>
      <h2 class="text-base font-semibold">{$t('apispec.title')}</h2>
      <p class="mt-1 max-w-prose text-sm text-muted">{$t('apispec.intro')}</p>
      {#if quota && quota.limit > 0}<div class="mt-1"><SpecQuota total={quota.total} limit={quota.limit} /></div>{/if}
    </div>
    <div class="flex flex-wrap gap-2">
      {@render headeractions?.()}
      <Button onclick={onupload}>{$t('apispec.upload.open')}</Button>
    </div>
  </header>

  {#if $store.status === 'error'}
    <ErrorState error={$store.error} onretry={() => store.invalidate()} />
  {:else if $store.status === 'ready' && rows.length === 0}
    <EmptyState title={$t('apispec.list.empty')} description={$t('apispec.list.emptyHint')}>
      {#snippet action()}
        <div class="flex flex-wrap justify-center gap-2">
          {@render headeractions?.()}
          <Button onclick={onupload}>{$t('apispec.upload.open')}</Button>
        </div>
      {/snippet}
    </EmptyState>
  {:else if $store.status === 'notFound'}
    <div></div>
  {:else}
    <SpecTable items={rows} {projectId} loading={firstLoad} />
  {/if}
</section>
