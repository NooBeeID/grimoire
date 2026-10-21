<script lang="ts">
  import Button from '../../../../../shared/components/Button.svelte';
  import Dialog from '../../../../../shared/components/Dialog.svelte';
  import { t } from '../../../../../shared/stores/i18n';
  import type { User, UserFormError } from '../types';

  let {
    open = $bindable(false),
    user = null,
    submitting = false,
    error = null,
    onconfirm,
    onclose,
  }: {
    open?: boolean;
    user?: User | null;
    submitting?: boolean;
    error?: UserFormError | null;
    onconfirm: () => void;
    onclose: () => void;
  } = $props();

  const message = $derived.by(() => {
    if (!error) return null;
    if (error.kind === 'form') return error.message ?? Object.values(error.fields)[0] ?? null;
    if (error.kind === 'notFound') return $t('users.error.notFound');
    return $t('error.internal');
  });
</script>

<Dialog bind:open title={$t('users.dialog.resendTitle')} {onclose}>
  <div class="flex flex-col gap-4">
    <p class="text-sm">{user ? $t('users.dialog.resendBody', { email: user.email }) : ''}</p>
    {#if message}<p role="alert" class="text-sm text-danger">{message}</p>{/if}
    <div class="flex justify-end gap-2">
      <Button variant="secondary" onclick={() => (open = false)} disabled={submitting}>{$t('common.cancel')}</Button>
      <Button loading={submitting} onclick={onconfirm}>{$t('users.dialog.resendSubmit')}</Button>
    </div>
  </div>
</Dialog>
