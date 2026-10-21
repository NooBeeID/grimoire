<script lang="ts">
  import Button from '../../../../../shared/components/Button.svelte';
  import Dialog from '../../../../../shared/components/Dialog.svelte';
  import { t } from '../../../../../shared/stores/i18n';

  let {
    open,
    title,
    message,
    confirmLabel,
    variant = 'danger',
    submitting = false,
    error = null,
    onconfirm,
    oncancel,
  }: {
    open: boolean;
    title: string;
    message: string;
    confirmLabel: string;
    variant?: 'primary' | 'danger';
    submitting?: boolean;
    error?: string | null;
    onconfirm: () => void;
    oncancel: () => void;
  } = $props();
</script>

<Dialog {open} {title} onclose={oncancel}>
  <div class="flex flex-col gap-4">
    <p class="text-sm">{message}</p>
    {#if error}<p role="alert" class="text-sm text-danger">{error}</p>{/if}
    <div class="flex justify-end gap-2">
      <Button variant="secondary" onclick={oncancel} disabled={submitting}>{$t('common.cancel')}</Button>
      <Button {variant} loading={submitting} onclick={onconfirm}>{confirmLabel}</Button>
    </div>
  </div>
</Dialog>
