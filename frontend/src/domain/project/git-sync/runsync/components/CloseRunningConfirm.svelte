<script lang="ts">
  import { tick } from 'svelte';
  import Button from '../../../../../shared/components/Button.svelte';
  import Dialog from '../../../../../shared/components/Dialog.svelte';
  import { t } from '../../../../../shared/stores/i18n';

  let { open, onconfirm, onstay }: { open: boolean; onconfirm: () => void; onstay: () => void } = $props();

  let stayWrap: HTMLElement | undefined = $state();

  // Fokus awal pada aksi aman ("Tetap di sini"); Esc/backdrop/silang = tetap di sini.
  $effect(() => {
    if (!open) return;
    void tick().then(() => stayWrap?.querySelector('button')?.focus());
  });
</script>

<Dialog {open} title={$t('gitSync.progress.closeTitle')} size="md" onclose={onstay}>
  <p class="text-sm text-muted" data-testid="sync-close-confirm">{$t('gitSync.progress.closeBody')}</p>
  {#snippet footer()}
    <span bind:this={stayWrap} class="contents"><Button variant="secondary" onclick={onstay}>{$t('gitSync.progress.stay')}</Button></span>
    <Button variant="primary" onclick={onconfirm}>{$t('gitSync.close')}</Button>
  {/snippet}
</Dialog>
