<script lang="ts">
  import { t } from '../../../../../shared/stores/i18n';
  import type { TreeAction, TreeNodeRef, TreeRow } from '../types';
  import NodeMenu from './NodeMenu.svelte';

  let {
    rows,
    selected,
    onselect,
    ontoggle,
    onaction,
    onretry,
  }: {
    rows: TreeRow[];
    selected: string | null;
    onselect: (ref: TreeNodeRef) => void;
    ontoggle: (path: string) => void;
    onaction: (ref: TreeNodeRef, action: TreeAction) => void;
    onretry: (prefix: string) => void;
  } = $props();

  let focusKey = $state<string | null>(null);
  let menuFor = $state<string | null>(null);
  let list: HTMLElement | undefined = $state();

  const items = $derived(rows.filter((r) => r.kind === 'folder' || r.kind === 'file'));
  const current = $derived(items.find((r) => r.key === focusKey) ?? items.find((r) => r.path === selected) ?? items[0]);

  const ref = (r: TreeRow): TreeNodeRef => ({ kind: r.kind as 'folder' | 'file', path: r.path, name: r.name, fileCount: r.fileCount });

  function activate(r: TreeRow) {
    if (r.kind === 'folder') {
      ontoggle(r.path);
      onselect(ref(r));
    } else onselect(ref(r));
  }

  function moveFocus(key: string) {
    focusKey = key;
    queueMicrotask(() => list?.querySelector<HTMLElement>(`[data-key="${CSS.escape(key)}"]`)?.focus());
  }

  function onkey(e: KeyboardEvent, r: TreeRow) {
    const i = items.findIndex((x) => x.key === r.key);
    const go = (n: number) => items[n] && moveFocus(items[n]!.key);
    switch (e.key) {
      case 'ArrowDown': e.preventDefault(); go(i + 1); break;
      case 'ArrowUp': e.preventDefault(); go(i - 1); break;
      case 'Home': e.preventDefault(); go(0); break;
      case 'End': e.preventDefault(); go(items.length - 1); break;
      case 'ArrowRight':
        e.preventDefault();
        if (r.kind === 'folder') {
          if (!r.expanded) ontoggle(r.path);
          else go(i + 1);
        }
        break;
      case 'ArrowLeft':
        e.preventDefault();
        if (r.kind === 'folder' && r.expanded) ontoggle(r.path);
        else {
          const p = items.findIndex((x) => x.kind === 'folder' && x.path === r.parent);
          if (p >= 0) go(p);
        }
        break;
      case 'Enter':
      case ' ':
        e.preventDefault();
        activate(r);
        break;
      case 'Escape':
        menuFor = null;
        break;
    }
  }
</script>

<ul bind:this={list} role="tree" aria-label={$t('knowledge.tree.label')} class="flex flex-col text-sm">
  {#each rows as r (r.key)}
    {#if r.kind === 'folder' || r.kind === 'file'}
      <li
        role="treeitem"
        aria-level={r.depth + 1}
        aria-selected={selected === r.path}
        aria-expanded={r.kind === 'folder' ? !!r.expanded : undefined}
        class="group relative"
      >
        <div
          class="flex items-center gap-1 rounded-base pr-1 {selected === r.path ? 'bg-active' : 'hover:bg-raised'}"
          style="padding-left: {r.depth * 14 + 4}px"
        >
          <button
            type="button"
            data-key={r.key}
            tabindex={current?.key === r.key ? 0 : -1}
            title={r.path}
            onclick={() => activate(r)}
            onkeydown={(e) => onkey(e, r)}
            onfocus={() => (focusKey = r.key)}
            class="flex min-w-0 flex-1 items-center gap-1.5 rounded-base py-1 text-left focus-visible:ring-2 focus-visible:ring-accent-ink focus-visible:outline-none"
          >
            <span class="w-3 shrink-0 text-center text-muted" aria-hidden="true">{r.kind === 'folder' ? (r.expanded ? '▾' : '▸') : ''}</span>
            <span class="min-w-0 break-all">{r.name}</span>
            {#if r.kind === 'folder'}
              <span class="ml-auto shrink-0 pl-2 text-xs text-muted">{$t('knowledge.tree.fileCount', { count: r.fileCount ?? 0 })}</span>
            {/if}
          </button>
          <NodeMenu
            node={ref(r)}
            open={menuFor === r.key}
            ontoggle={() => (menuFor = menuFor === r.key ? null : r.key)}
            onclose={() => (menuFor = null)}
            onaction={(a) => onaction(ref(r), a)}
          />
        </div>
      </li>
    {:else}
      <li role="none" class="py-1 text-xs text-muted" style="padding-left: {r.depth * 14 + 24}px">
        {#if r.kind === 'loading'}
          <span aria-busy="true">{$t('common.loading')}</span>
        {:else if r.kind === 'error'}
          {$t('knowledge.tree.loadError')}
          <button type="button" class="ml-1 underline focus-visible:ring-2 focus-visible:ring-accent-ink focus-visible:outline-none" onclick={() => onretry(r.path)}>{$t('common.retry')}</button>
        {:else}
          {$t('knowledge.tree.folderEmpty')}
        {/if}
      </li>
    {/if}
  {/each}
</ul>
