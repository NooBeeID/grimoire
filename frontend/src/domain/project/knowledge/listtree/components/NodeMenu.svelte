<script lang="ts">
  import { t } from '../../../../../shared/stores/i18n';
  import type { TreeAction, TreeNodeRef } from '../types';

  let {
    node,
    open,
    ontoggle,
    onclose,
    onaction,
  }: {
    node: TreeNodeRef;
    open: boolean;
    ontoggle: () => void;
    onclose: () => void;
    onaction: (action: TreeAction) => void;
  } = $props();

  let trigger: HTMLButtonElement | undefined = $state();
  let menu: HTMLElement | undefined = $state();

  const folderActions: TreeAction[] = ['newFileHere', 'uploadHere', 'copyPath', 'deleteFolder'];
  const fileActions: TreeAction[] = ['open', 'move', 'copyPath', 'deleteFile'];
  const actionKey: Record<TreeAction, string> = {
    open: 'knowledge.actions.open',
    copyPath: 'knowledge.actions.copyPath',
    move: 'knowledge.actions.move',
    deleteFile: 'knowledge.actions.delete',
    newFileHere: 'knowledge.actions.newFileHere',
    uploadHere: 'knowledge.actions.uploadHere',
    deleteFolder: 'knowledge.actions.deleteFolder',
  };
  const actions = $derived(node.kind === 'folder' ? folderActions : fileActions);

  $effect(() => {
    if (open) queueMicrotask(() => menu?.querySelector<HTMLElement>('[role="menuitem"]')?.focus());
  });

  function close(refocus: boolean) {
    onclose();
    if (refocus) trigger?.focus();
  }

  function onkeydown(e: KeyboardEvent) {
    const items = [...(menu?.querySelectorAll<HTMLElement>('[role="menuitem"]') ?? [])];
    const i = items.indexOf(document.activeElement as HTMLElement);
    if (e.key === 'Escape') {
      e.preventDefault();
      e.stopPropagation();
      close(true);
    } else if (e.key === 'ArrowDown') {
      e.preventDefault();
      items[(i + 1) % items.length]?.focus();
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      items[(i - 1 + items.length) % items.length]?.focus();
    } else if (e.key === 'Tab') close(false);
  }
</script>

<button
  bind:this={trigger}
  type="button"
  tabindex="-1"
  aria-haspopup="menu"
  aria-expanded={open}
  aria-label={$t('knowledge.actions.menu', { name: node.name })}
  onclick={ontoggle}
  class="shrink-0 rounded-base px-1.5 py-0.5 text-muted hover:bg-active focus-visible:ring-2 focus-visible:ring-accent-ink focus-visible:outline-none"
>
  <span aria-hidden="true">⋯</span>
</button>
{#if open}
  <div
    bind:this={menu}
    role="menu"
    tabindex="-1"
    {onkeydown}
    class="absolute top-full right-1 z-10 mt-1 flex min-w-44 flex-col glass rounded-card p-1.5 shadow-pop"
  >
    {#each actions as a (a)}
      <button
        type="button"
        role="menuitem"
        tabindex="-1"
        onclick={() => {
          onclose();
          onaction(a);
        }}
        class="rounded-base px-2 py-1.5 text-left hover:bg-raised focus-visible:ring-2 focus-visible:ring-accent-ink focus-visible:outline-none {a === 'deleteFile' || a === 'deleteFolder' ? 'text-danger' : ''}"
      >
        {$t(actionKey[a])}
      </button>
    {/each}
  </div>
{/if}
