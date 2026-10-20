<script lang="ts">
  import Button from '../../../../../shared/components/Button.svelte';
  import Dialog from '../../../../../shared/components/Dialog.svelte';
  import ErrorState from '../../../../../shared/components/ErrorState.svelte';
  import Input from '../../../../../shared/components/Input.svelte';
  import Select from '../../../../../shared/components/Select.svelte';
  import Spinner from '../../../../../shared/components/Spinner.svelte';
  import { t } from '../../../../../shared/stores/i18n';
  import type { Role, User, UserFormError } from '../types';

  let {
    open = $bindable(false),
    user = null,
    loading = false,
    loadError = null,
    isSelf = false,
    submitting = false,
    error = null,
    onsubmit,
    onretry,
    onclose,
  }: {
    open?: boolean;
    user?: User | null;
    loading?: boolean;
    loadError?: UserFormError | null;
    isSelf?: boolean;
    submitting?: boolean;
    error?: UserFormError | null;
    onsubmit: (changes: Partial<{ name: string; email: string; role: Role }>) => void;
    onretry?: () => void;
    onclose: () => void;
  } = $props();

  let name = $state('');
  let email = $state('');
  let role = $state('developer');
  let local = $state<Record<string, string>>({});

  $effect(() => {
    if (user) {
      name = user.name;
      email = user.email;
      role = user.role;
      local = {};
    }
  });

  const changes = $derived.by(() => {
    const c: Partial<{ name: string; email: string; role: Role }> = {};
    if (!user) return c;
    if (name.trim() !== user.name) c.name = name.trim();
    if (email.trim() !== user.email) c.email = email.trim();
    if (role !== user.role) c.role = role as Role;
    return c;
  });
  const unchanged = $derived(Object.keys(changes).length === 0);
  const fieldErr = (n: string) => local[n] ?? (error?.kind === 'form' ? error.fields[n] : undefined);
  const roleOptions = $derived([
    { value: 'developer', label: $t('users.role.developer') },
    { value: 'super_admin', label: $t('users.role.super_admin') },
  ]);

  function submit(e: SubmitEvent) {
    e.preventDefault();
    const errs: Record<string, string> = {};
    if (name.trim() === '') errs.name = $t('form.required');
    if (email.trim() === '') errs.email = $t('form.required');
    local = errs;
    if (Object.keys(errs).length > 0 || unchanged) return;
    onsubmit(changes);
  }
</script>

<Dialog bind:open title={$t('users.dialog.editTitle')} {onclose}>
  {#if loading}
    <div class="flex justify-center p-6"><Spinner /></div>
  {:else if loadError?.kind === 'notFound'}
    <div class="flex flex-col gap-3">
      <p role="alert" class="text-danger">{$t('users.error.notFound')}</p>
      <div class="flex justify-end"><Button onclick={() => (open = false)}>{$t('common.close')}</Button></div>
    </div>
  {:else if loadError}
    <ErrorState error={loadError.kind === 'generic' ? loadError.error : null} {onretry} />
  {:else if user}
    <form onsubmit={submit} novalidate class="flex flex-col gap-3">
      <Input bind:value={name} label={$t('users.form.name')} required disabled={submitting} error={fieldErr('name')} />
      <Input bind:value={email} type="email" label={$t('users.form.email')} required disabled={submitting} error={fieldErr('email')} />
      <Select
        bind:value={role}
        label={$t('users.form.role')}
        options={roleOptions}
        disabled={submitting || isSelf}
        hint={isSelf ? $t('users.self.cannotChangeRole') : undefined}
        error={fieldErr('role')}
      />
      {#if error?.kind === 'form' && error.message}
        <p role="alert" class="text-sm text-danger">{error.message}</p>
      {:else if error?.kind === 'notFound'}
        <p role="alert" class="text-sm text-danger">{$t('users.error.notFound')}</p>
      {:else if error?.kind === 'generic'}
        <p role="alert" class="text-sm text-danger">{$t('error.internal')}</p>
      {/if}
      <div class="flex justify-end gap-2">
        <Button variant="secondary" onclick={() => (open = false)}>{$t('common.cancel')}</Button>
        <Button type="submit" loading={submitting} disabled={unchanged}>{$t('common.save')}</Button>
      </div>
    </form>
  {/if}
</Dialog>
