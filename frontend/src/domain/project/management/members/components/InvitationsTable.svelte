<script lang="ts">
  import IconButton from '../../../../../shared/components/IconButton.svelte';
  import Avatar from '../../../../../shared/components/Avatar.svelte';
  import Button from '../../../../../shared/components/Button.svelte';
  import Table from '../../../../../shared/components/Table.svelte';
  import { locale, t } from '../../../../../shared/stores/i18n';
  import { formatDateTime } from '../../../../../shared/utils/datetime';
  import type { Invitation } from '../types';
  import InvitationStatusPill from './InvitationStatusPill.svelte';

  let {
    items,
    busyUserId = null,
    onrequestresend,
    onrequestrevoke,
  }: {
    items: Invitation[];
    busyUserId?: string | null;
    onrequestresend: (i: Invitation) => void;
    onrequestrevoke: (i: Invitation) => void;
  } = $props();

  // Kolom "Diundang oleh" dibuang bila kosong di semua baris.
  const hasInviter = $derived(items.some((i) => i.invited_by_label));
  const columns = $derived([
    { key: 'invitee', header: $t('project.invitations.col.invitee') },
    { key: 'role', header: $t('project.invitations.col.role') },
    { key: 'status', header: $t('project.invitations.col.status') },
    { key: 'sent', header: $t('project.invitations.col.sent') },
    { key: 'expires', header: $t('project.invitations.col.expires') },
    ...(hasInviter ? [{ key: 'invitedBy', header: $t('project.invitations.col.invitedBy') }] : []),
    { key: 'actions', header: $t('project.invitations.col.actions'), class: 'text-right' },
  ]);
</script>

{#snippet cell(i: Invitation, col: { key: string })}
  {#if col.key === 'invitee'}
    <span class="flex items-center gap-2.5">
      <Avatar name={i.name} size={28} />
      <span class="min-w-0"><span class="block break-words">{i.name}</span><span class="block break-all text-xs text-muted">{i.email}</span></span>
    </span>
  {:else if col.key === 'role'}
    <span class="inline-flex h-6 items-center rounded-full px-2.5 text-xs font-medium {i.role === 'owner' ? 'bg-pill-neutral-bg text-pill-neutral-ink' : 'bg-raised text-muted'}">{i.role === 'owner' ? $t('project.role.owner') : $t('project.role.member')}</span>
  {:else if col.key === 'status'}
    <InvitationStatusPill invitation={i} />
  {:else if col.key === 'sent'}
    {#if i.sent_at}<time datetime={i.sent_at}>{formatDateTime(i.sent_at, $locale)}</time>{:else}<span class="text-muted">–</span>{/if}
    {#if i.resend_count > 0}<span class="block text-xs text-muted">{$t('project.invitations.resendCount', { n: i.resend_count })}</span>{/if}
  {:else if col.key === 'expires'}
    <time datetime={i.expires_at}>{formatDateTime(i.expires_at, $locale)}</time>
  {:else if col.key === 'invitedBy'}
    {#if i.invited_by_label}<span class="break-all">{i.invited_by_label}</span>{:else}<span class="text-muted">–</span>{/if}
  {:else}
    {@const busy = busyUserId === i.user_id}
    {@const otherBusy = busyUserId !== null && !busy}
    <div class="flex flex-wrap justify-end gap-1">
      <IconButton icon="resend" label={$t('project.invitations.resend')} loading={busy} disabled={otherBusy} ariaLabel={$t('project.invitations.resendAria', { email: i.email })} onclick={() => onrequestresend(i)} />
      <IconButton icon="revoke" tone="danger" label={$t('project.invitations.revoke')} disabled={busy || otherBusy} ariaLabel={$t('project.invitations.revokeAria', { email: i.email })} onclick={() => onrequestrevoke(i)} />
    </div>
  {/if}
{/snippet}

<div class="overflow-x-auto">
  <Table {columns} rows={items} rowKey={(i) => i.user_id} {cell} caption={$t('project.invitations.title')} />
</div>
