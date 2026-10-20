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

  const deactivating = $derived(user?.is_active ?? true);
  const message = $derived.by(() => {
    if (!error) return null;
    if (error.kind === 'form') return error.message ?? Object.values(error.fields)[0] ?? null;
    if (error.kind === 'notFound') return $t('users.error.notFound');
    return $t('error.internal');
  });
</script>

<Dialog bind:open title={deactivating ? $t('users.dialog.deactivateTitle') : $t('users.dialog.activateTitle')} {onclose}>
  <p>{user ? $t(deactivating ? 'users.dialog.deactivateBody' : 'users.dialog.activateBody', { name: user.name }) : ''}</p>
  {#if message}<p role="alert" class="mt-3 text-sm text-danger">{message}</p>{/if}
  {#snippet footer()}
    <Button variant="secondary" onclick={() => (open = false)}>{$t('common.cancel')}</Button>
    <Button variant={deactivating ? 'danger' : 'primary'} loading={submitting} onclick={onconfirm}>
      {deactivating ? $t('users.action.deactivate') : $t('users.action.activate')}
    </Button>
  {/snippet}
</Dialog>
