<script lang="ts">
  import Button from '../../../../../shared/components/Button.svelte';
  import Dialog from '../../../../../shared/components/Dialog.svelte';
  import Input from '../../../../../shared/components/Input.svelte';
  import Select from '../../../../../shared/components/Select.svelte';
  import { t } from '../../../../../shared/stores/i18n';
  import type { CreateUserInput, Role, User, UserFormError } from '../types';
  import OneTimeSecret from './OneTimeSecret.svelte';

  let {
    open = $bindable(false),
    submitting = false,
    error = null,
    result = null,
    onsubmit,
    onclose,
  }: {
    open?: boolean;
    submitting?: boolean;
    error?: UserFormError | null;
    result?: { user: User; temporaryPassword: string | null } | null;
    onsubmit: (input: CreateUserInput) => void;
    onclose: () => void;
  } = $props();

  let email = $state('');
  let name = $state('');
  let role = $state('developer');
  let mode = $state('generate');
  let password = $state('');
  let local = $state<Record<string, string>>({});

  $effect(() => {
    if (!open) {
      email = name = password = '';
      role = 'developer';
      mode = 'generate';
      local = {};
    }
  });

  const showSecret = $derived(result !== null && result.temporaryPassword !== null);
  const fieldErr = (n: string) => local[n] ?? (error?.kind === 'form' ? error.fields[n] : undefined);
  const roleOptions = $derived([
    { value: 'developer', label: $t('users.role.developer') },
    { value: 'super_admin', label: $t('users.role.super_admin') },
  ]);
  const modeOptions = $derived([
    { value: 'generate', label: $t('users.form.passwordMode.generate') },
    { value: 'manual', label: $t('users.form.passwordMode.manual') },
  ]);

  function submit(e: SubmitEvent) {
    e.preventDefault();
    const errs: Record<string, string> = {};
    if (email.trim() === '') errs.email = $t('form.required');
    if (name.trim() === '') errs.name = $t('form.required');
    if (mode === 'manual' && password === '') errs.password = $t('form.required');
    local = errs;
    if (Object.keys(errs).length > 0) return;
    const input: CreateUserInput = { email: email.trim(), name: name.trim(), role: role as Role };
    if (mode === 'manual' && password !== '') input.password = password;
    onsubmit(input);
  }
</script>

<Dialog bind:open title={$t('users.dialog.createTitle')} {onclose}>
  {#if showSecret && result}
    <div class="flex flex-col gap-3">
      <p>{result.user.email}</p>
      <h3 class="font-semibold">{$t('users.secret.title')}</h3>
      <OneTimeSecret value={result.temporaryPassword ?? ''} />
      <div class="flex justify-end"><Button onclick={() => (open = false)}>{$t('users.secret.done')}</Button></div>
    </div>
  {:else}
    <form onsubmit={submit} novalidate class="flex flex-col gap-3">
      <Input bind:value={email} type="email" label={$t('users.form.email')} autocomplete="off" required disabled={submitting} error={fieldErr('email')} />
      <Input bind:value={name} label={$t('users.form.name')} autocomplete="off" required disabled={submitting} error={fieldErr('name')} />
      <Select bind:value={role} label={$t('users.form.role')} options={roleOptions} disabled={submitting} error={fieldErr('role')} />
      <Select bind:value={mode} label={$t('users.form.passwordMode')} options={modeOptions} disabled={submitting} />
      {#if mode === 'manual'}
        <Input bind:value={password} type="password" label={$t('users.form.password')} autocomplete="off" disabled={submitting} error={fieldErr('password')} />
      {/if}
      {#if error?.kind === 'form' && error.message}
        <p role="alert" class="text-sm text-danger">{error.message}</p>
      {:else if error?.kind === 'generic' || error?.kind === 'notFound'}
        <p role="alert" class="text-sm text-danger">{error.kind === 'notFound' ? $t('users.error.notFound') : $t('error.internal')}</p>
      {/if}
      <div class="flex justify-end gap-2">
        <Button variant="secondary" onclick={() => (open = false)}>{$t('common.cancel')}</Button>
        <Button type="submit" loading={submitting}>{$t('common.save')}</Button>
      </div>
    </form>
  {/if}
</Dialog>
