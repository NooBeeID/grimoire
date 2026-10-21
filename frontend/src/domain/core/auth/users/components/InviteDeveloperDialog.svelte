<script lang="ts">
  import Button from '../../../../../shared/components/Button.svelte';
  import Dialog from '../../../../../shared/components/Dialog.svelte';
  import EmailDeliveryNote from '../../../../../shared/components/EmailDeliveryNote.svelte';
  import Input from '../../../../../shared/components/Input.svelte';
  import { t } from '../../../../../shared/stores/i18n';
  import type { InviteUserInput, UserFormError } from '../types';

  let {
    open = $bindable(false),
    submitting = false,
    error = null,
    emailEnabled,
    onsubmit,
    onshowinlist,
    onclose,
  }: {
    open?: boolean;
    submitting?: boolean;
    error?: UserFormError | null;
    emailEnabled: boolean | null;
    onsubmit: (input: InviteUserInput) => void;
    onshowinlist: (email: string) => void;
    onclose: () => void;
  } = $props();

  let email = $state('');
  let name = $state('');
  let local = $state<string | undefined>();

  $effect(() => {
    if (!open) {
      email = name = '';
      local = undefined;
    }
  });

  const looksLikeEmail = (v: string) => /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(v);
  const emailErr = $derived(local ?? (error?.kind === 'form' ? error.fields.email : undefined));
  const message = $derived.by(() => {
    if (!error) return null;
    if (error.kind === 'form') return error.message ?? null;
    if (error.kind === 'notFound') return $t('users.error.notFound');
    return $t('error.internal');
  });

  function submit(e: SubmitEvent) {
    e.preventDefault();
    if (submitting) return;
    const v = email.trim();
    if (v === '') local = $t('form.required');
    else if (!looksLikeEmail(v)) local = $t('users.invite.emailInvalid');
    else {
      local = undefined;
      onsubmit({ email: v, name: name.trim() });
    }
  }
</script>

<Dialog bind:open title={$t('users.invite.title')} {onclose}>
  <form onsubmit={submit} novalidate class="flex flex-col gap-3">
    <Input bind:value={email} type="email" label={$t('users.invite.email')} autocomplete="off" required disabled={submitting} error={emailErr} />
    {#if error?.kind === 'form' && error.reason === 'registered'}
      <div><Button variant="ghost" size="sm" onclick={() => onshowinlist(email.trim().toLowerCase())}>{$t('users.invite.showInList')}</Button></div>
    {/if}
    <Input bind:value={name} label={$t('users.invite.name')} hint={$t('users.invite.nameHint')} autocomplete="off" disabled={submitting} error={error?.kind === 'form' ? error.fields.name : undefined} />
    <EmailDeliveryNote {emailEnabled} />
    {#if message}<p role="alert" class="text-sm text-danger">{message}</p>{/if}
    <div class="flex justify-end gap-2">
      <Button variant="secondary" onclick={() => (open = false)} disabled={submitting}>{$t('common.cancel')}</Button>
      <Button type="submit" loading={submitting}>{$t('users.invite.submit')}</Button>
    </div>
  </form>
</Dialog>
