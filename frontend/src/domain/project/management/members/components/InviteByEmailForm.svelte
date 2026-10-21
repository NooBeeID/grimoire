<script lang="ts">
  import Button from '../../../../../shared/components/Button.svelte';
  import EmailDeliveryNote from '../../../../../shared/components/EmailDeliveryNote.svelte';
  import Input from '../../../../../shared/components/Input.svelte';
  import { t } from '../../../../../shared/stores/i18n';
  import type { InviteErrorView } from '../services/invitations.service';
  import type { MemberRole } from '../types';

  let {
    email,
    role = $bindable<MemberRole>('member'),
    name = $bindable(''),
    emailEnabled,
    submitting = false,
    error = null,
    onsubmit,
    onback,
    oncancel,
    onsearchinstead,
  }: {
    email: string;
    role?: MemberRole;
    name?: string;
    emailEnabled: boolean | null;
    submitting?: boolean;
    error?: InviteErrorView | null;
    onsubmit: () => void;
    onback: () => void;
    oncancel: () => void;
    onsearchinstead: () => void;
  } = $props();

  function submit(e: SubmitEvent) {
    e.preventDefault();
    if (!submitting) onsubmit();
  }

  const fieldError = (k: 'email' | 'role' | 'name') => (error?.kind === 'fields' ? error.fields[k] : undefined);
  const message = $derived.by(() => {
    if (!error || error.kind === 'fields') return null;
    switch (error.kind) {
      case 'alreadyMember':
        return $t('project.error.ALREADY_MEMBER');
      case 'alreadyRegistered':
        return $t(`invitation.error.alreadyRegistered.${error.reason}`);
      case 'rateLimited':
        return $t(`invitation.error.rateLimited.${error.scope}`, { minutes: error.minutes });
      case 'notPending':
        return $t('invitation.error.notPending');
      case 'lastOwner':
        return $t('project.error.LAST_OWNER');
      case 'forbidden':
        return $t('error.forbidden');
      case 'notFound':
        return $t('project.members.error.gone');
      default:
        return error.traceId ? `${error.message} (${$t('error.trace_id', { id: error.traceId })})` : error.message;
    }
  });
</script>

<form onsubmit={submit} novalidate class="flex flex-col gap-4">
  <div class="flex flex-col gap-1">
    <span class="text-[13px] font-medium">{$t('project.members.invite.email')}</span>
    <div class="flex items-center justify-between gap-2 rounded-base border border-border bg-surface px-3 py-2">
      <span class="min-w-0 break-all" data-testid="invite-email">{email}</span>
      <button
        type="button"
        onclick={onback}
        disabled={submitting}
        class="rounded-base px-2 py-1 text-sm underline focus-visible:ring-2 focus-visible:ring-accent-ink focus-visible:outline-none disabled:opacity-60"
      >
        {$t('project.members.invite.change')}
      </button>
    </div>
    {#if fieldError('email')}<p role="alert" class="text-sm text-danger">{fieldError('email')}</p>{/if}
  </div>

  <Input bind:value={name} label={$t('project.members.invite.name')} hint={$t('project.members.invite.nameHint')} autocomplete="off" disabled={submitting} error={fieldError('name')} />

  <fieldset class="flex flex-col gap-1" disabled={submitting}>
    <legend class="text-sm font-medium">{$t('project.members.add.role')}</legend>
    <label class="flex items-center gap-2 text-sm">
      <input type="radio" name="invite-role" value="member" bind:group={role} /> {$t('project.role.member')}
    </label>
    <label class="flex items-center gap-2 text-sm">
      <input type="radio" name="invite-role" value="owner" bind:group={role} /> {$t('project.role.owner')}
    </label>
    <p class="text-sm text-muted">{$t('project.members.add.ownerHint')}</p>
    {#if fieldError('role')}<p role="alert" class="text-sm text-danger">{fieldError('role')}</p>{/if}
  </fieldset>

  <EmailDeliveryNote {emailEnabled} />

  {#if message}
    <div class="flex flex-wrap items-center gap-2">
      <p role="alert" class="text-sm text-danger">{message}</p>
      {#if error?.kind === 'alreadyRegistered' && error.reason === 'registered'}
        <Button variant="ghost" size="sm" onclick={onsearchinstead}>{$t('project.members.invite.searchInstead')}</Button>
      {/if}
    </div>
  {/if}

  <div class="flex justify-end gap-2">
    <Button variant="secondary" onclick={oncancel} disabled={submitting}>{$t('common.cancel')}</Button>
    <Button type="submit" loading={submitting}>{$t('project.members.invite.submit')}</Button>
  </div>
</form>
