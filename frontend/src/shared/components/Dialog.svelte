<script lang="ts">
  import type { Snippet } from 'svelte';
  import { t } from '../stores/i18n';

  let {
    open = $bindable(false),
    title,
    onclose,
    dismissible = true,
    onrequestclose,
    size = 'md',
    children,
    footer,
  }: {
    open?: boolean;
    title: string;
    onclose?: () => void;
    dismissible?: boolean;
    // Dipanggil saat dismissible=false dan user menekan Esc, tombol silang, atau backdrop.
    onrequestclose?: () => void;
    size?: 'md' | 'lg';
    children?: Snippet;
    footer?: Snippet;
  } = $props();

  const uid = $props.id();
  const titleId = `dlg-${uid}`;
  let el: HTMLDialogElement | undefined = $state();
  let trigger: HTMLElement | null = null;

  $effect(() => {
    if (!el) return;
    if (open && !el.open) {
      trigger = document.activeElement instanceof HTMLElement ? document.activeElement : null;
      el.showModal();
    } else if (!open && el.open) {
      el.close();
    }
  });

  function handleClose() {
    const wasOpen = open;
    open = false;
    trigger?.focus();
    trigger = null;
    if (wasOpen) onclose?.();
  }

  function close() {
    el?.close();
    handleClose();
  }

  function onBackdrop(e: MouseEvent) {
    if (e.target !== el) return;
    if (dismissible) close();
    else onrequestclose?.();
  }

  function onCancel(e: Event) {
    if (dismissible) return;
    e.preventDefault();
    onrequestclose?.();
  }

  function onCross() {
    if (dismissible) close();
    else onrequestclose?.();
  }
</script>

<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_noninteractive_element_interactions -->
<dialog
  bind:this={el}
  aria-labelledby={titleId}
  onclose={handleClose}
  oncancel={onCancel}
  onclick={onBackdrop}
  class="m-auto w-full {size === 'lg' ? 'max-w-2xl' : 'max-w-lg'} rounded-sheet border border-border bg-surface p-0 text-fg shadow-pop backdrop:bg-black/40"
>
  <div class="flex items-start justify-between gap-4 border-b border-rule p-5">
    <h2 id={titleId} class="text-base font-semibold">{title}</h2>
    <button
      type="button"
      aria-label={$t('common.close')}
      onclick={onCross}
      disabled={!dismissible && !onrequestclose}
      class="rounded-base px-2 py-1 hover:bg-raised focus-visible:ring-2 focus-visible:ring-accent-ink focus-visible:outline-none disabled:opacity-40"
    >
      <span aria-hidden="true">&times;</span>
    </button>
  </div>
  <div class="p-5">{@render children?.()}</div>
  {#if footer}<div class="flex justify-end gap-2 border-t border-rule p-5">{@render footer()}</div>{/if}
</dialog>
