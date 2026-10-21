<script lang="ts">
  import { untrack } from 'svelte';
  import ErrorState from '../../../../../shared/components/ErrorState.svelte';
  import { ApiError, isApiError } from '../../../../../shared/services/http';
  import { t } from '../../../../../shared/stores/i18n';
  import { ancestors } from '../../base';
  import { getTreeLevel } from '../services/listtree.service';
  import { createTreeStore } from '../stores/tree.store';
  import type { TreeAction, TreeNodeRef } from '../types';
  import TreeView from '../components/TreeView.svelte';

  let {
    projectId,
    selectedPath,
    selectedIsDir = false,
    refreshKey,
    onopenfile,
    onopendir,
    onaction,
    onprojectgone,
    onempty,
  }: {
    projectId: string;
    selectedPath: string | null;
    selectedIsDir?: boolean;
    refreshKey: number;
    onopenfile: (path: string) => void;
    onopendir: (path: string) => void;
    onaction: (ref: TreeNodeRef, action: TreeAction) => void;
    onprojectgone: () => void;
    onempty?: (isEmpty: boolean) => void;
  } = $props();

  const store = createTreeStore();
  const rows = store.rows;
  const root = store.root;
  const ctrls = new Map<string, AbortController>();
  const inflight = new Set<string>();
  let gone = false;

  async function load(prefix: string, force = false) {
    if (!force && (store.has(prefix) || inflight.has(prefix))) return;
    inflight.add(prefix);
    ctrls.get(prefix)?.abort();
    const c = new AbortController();
    ctrls.set(prefix, c);
    store.beginLoad(prefix);
    try {
      const lvl = await getTreeLevel(projectId, prefix, c.signal);
      inflight.delete(prefix);
      if (c.signal.aborted) return;
      store.resolve(prefix, lvl);
      if (prefix === '') onempty?.(lvl.total_files === 0);
    } catch (e) {
      inflight.delete(prefix);
      if (c.signal.aborted || (e instanceof DOMException && e.name === 'AbortError')) return;
      const err = isApiError(e) ? e : new ApiError({ status: 0, code: 'INTERNAL', message: String(e) });
      if (err.status === 404 && !gone) {
        gone = true;
        onprojectgone();
      }
      store.fail(prefix, err);
    }
  }

  // Muat root saat project berubah.
  let loadedFor: string | null = null;
  $effect(() => {
    const id = projectId;
    if (id !== loadedFor) {
      loadedFor = id;
      untrack(() => void load('', true));
    }
  });
  $effect(() => () => ctrls.forEach((c) => c.abort()));

  // Buka leluhur path terpilih, dimuat berurutan dari atas.
  $effect(() => {
    const sel = selectedPath;
    void projectId;
    if (!sel) return;
    untrack(async () => {
      const chain = selectedIsDir ? [...ancestors(sel), sel] : ancestors(sel);
      store.expand(chain);
      for (const p of chain) await load(p);
    });
  });

  let lastRefresh = untrack(() => refreshKey);
  $effect(() => {
    const k = refreshKey;
    if (k === lastRefresh) return;
    lastRefresh = k;
    untrack(() => {
      const prefixes = store.invalidateAll();
      void Promise.all(prefixes.map((p) => load(p, true)));
    });
  });

  function toggle(path: string) {
    if (store.toggle(path)) void load(path);
  }
</script>

<div aria-busy={$root?.status === 'loading' && !$root.level}>
  {#if $root?.status === 'error' && !$root.level}
    <ErrorState error={$root.error} onretry={() => load('', true)} />
  {:else if !$root || ($root.status === 'loading' && !$root.level)}
    <p class="p-3 text-sm text-muted">{$t('common.loading')}</p>
  {:else if $root.level && $root.level.total_files === 0}
    <p class="p-3 text-sm text-muted">{$t('knowledge.tree.empty')}</p>
  {:else}
    <TreeView
      rows={$rows}
      selected={selectedPath}
      onselect={(r) => (r.kind === 'file' ? onopenfile(r.path) : onopendir(r.path))}
      ontoggle={toggle}
      {onaction}
      onretry={(p) => load(p, true)}
    />
  {/if}
</div>
