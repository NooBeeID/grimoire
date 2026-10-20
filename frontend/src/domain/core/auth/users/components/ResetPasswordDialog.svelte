<script lang="ts">
  import Button from '../../../../../shared/components/Button.svelte';
  import Dialog from '../../../../../shared/components/Dialog.svelte';
  import Input from '../../../../../shared/components/Input.svelte';
  import Select from '../../../../../shared/components/Select.svelte';
  import { t } from '../../../../../shared/stores/i18n';
  import type { User, UserFormError } from '../types';
  import OneTimeSecret from './OneTimeSecret.svelte';

  let {
    open = $bindable(false),
    user = null,
    submitting = false,
    error = null,
    temporaryPassword = null,
    onsubmit,
    onclose,
  }: {
    open?: boolean;
    user?: User | null;
    submitting?: boolean;
    error?: UserFormError | null;
    temporaryPassword?: string | null;
    onsubmit: (password?: string) => void;
    onclose: () => void;
  } = $props();

  let mode = $state('generate');
  let password = $state('');
  let local = $state<string | undefined>();

  $effect(() => {
    if (!open) {
      mode = 'generate';
      password = '';
      local = undefined;
    }
  });

  const modeOptions = $derived([
    { value: 'generate', label: $t('users.form.passwordMode.generate') },
    { value: 'manual', label: $t('users.form.passwordMode.manual') },
  ]);
  const message = $derived.by(() => {
    if (!error) return null;
    if (error.kind === 'form') return error.message ?? null;
    if (error.kind === 'notFound') return $t('users.error.notFound');
    return $t('error.internal');
  });
  const pwErr = $derived(local ?? (error?.kind === 'form' ? error.fields.password : undefined));

  function submit(e: SubmitEvent) {
    e.preventDefault();
    if (mode === 'manual' && password === '') {
      local = $t('form.required');
      return;
    }
    local = undefined;
    onsubmit(mode === 'manual' ? password : undefined);
  }
</script>

<Dialog bind:open title={$t('users.dialog.resetTitle')} {onclose}>
  {#if temporaryPassword !== null}
    <div class="flex flex-col gap-3">
      <h3 class="font-semibold">{$t('users.secret.title')}</h3>
      <OneTimeSecret value={temporaryPassword} />
      <div class="flex justify-end"><Button onclick={() => (open = false)}>{$t('users.secret.done')}</Button></div>
    </div>
  {:else}
    <form onsubmit={submit} novalidate class="flex flex-col gap-3">
      <p>{user ? $t('users.dialog.resetBody', { name: user.name }) : ''}</p>
      <Select bind:value={mode} label={$t('users.form.passwordMode')} options={modeOptions} disabled={submitting} />
      {#if mode === 'manual'}
        <Input bind:value={password} type="password" label={$t('users.form.password')} autocomplete="off" disabled={submitting} error={pwErr} />
      {/if}
      {#if message}<p role="alert" class="text-sm text-danger">{message}</p>{/if}
      <div class="flex justify-end gap-2">
        <Button variant="secondary" onclick={() => (open = false)}>{$t('common.cancel')}</Button>
        <Button type="submit" variant="danger" loading={submitting}>{$t('users.action.resetPassword')}</Button>
      </div>
    </form>
  {/if}
</Dialog>
