<script lang="ts">
  import { onMount } from 'svelte';
  import Button from '../../../../../shared/components/Button.svelte';
  import EmptyState from '../../../../../shared/components/EmptyState.svelte';
  import Pagination from '../../../../../shared/components/Pagination.svelte';
  import TemporaryCredentialsDialog from '../../../../../shared/components/TemporaryCredentialsDialog.svelte';
  import { t } from '../../../../../shared/stores/i18n';
  import { session } from '../../../../../shared/stores/session';
  import { toast } from '../../../../../shared/stores/toast';
  import type { TemporaryCredentials } from '../../../../../shared/validation/credentials';
  import ConfirmStatusDialog from '../components/ConfirmStatusDialog.svelte';
  import CreateUserDialog from '../components/CreateUserDialog.svelte';
  import EditUserDialog from '../components/EditUserDialog.svelte';
  import InviteDeveloperDialog from '../components/InviteDeveloperDialog.svelte';
  import ResendInvitationDialog from '../components/ResendInvitationDialog.svelte';
  import ResetPasswordDialog from '../components/ResetPasswordDialog.svelte';
  import UsersFilterBar from '../components/UsersFilterBar.svelte';
  import UsersTable from '../components/UsersTable.svelte';
  import { fetchUser } from '../services/getuser.service';
  import { createUser, inviteUser, listUsers, resendUserInvitation, resetUserPassword, setUserStatus, updateUser } from '../services/users.service';
  import { hasFilters, usersList, usersListStore } from '../stores/usersList';
  import { PAGE_SIZE, toUserFormError, type CreateUserInput, type InviteUserInput, type Role, type User, type UserFormError } from '../types';
  import PageHeader from '../../../../../shared/components/PageHeader.svelte';
  import { createUsersListController } from './usersList.controller';

  let { emailEnabled = null }: { emailEnabled?: boolean | null } = $props();

  const controller = createUsersListController({ listUsers, store: usersListStore });
  const selfId = $derived($session.identity?.userId);

  onMount(() => {
    void controller.load();
    return () => controller.dispose();
  });

  // Kredensial sementara hanya hidup di state ini; ditutup -> dihapus.
  let creds = $state<{ credentials: TemporaryCredentials; emailStatus: 'failed' | 'not_configured'; expiresAt: string } | null>(null);
  function showCredentials(res: { credentials?: TemporaryCredentials; email_status: string; expires_at: string }): boolean {
    if (!res.credentials) return false;
    creds = { credentials: res.credentials, emailStatus: res.email_status === 'failed' ? 'failed' : 'not_configured', expiresAt: res.expires_at };
    return true;
  }

  // Dialog undang developer
  let inviteOpen = $state(false);
  let inviteSubmitting = $state(false);
  let inviteError = $state<UserFormError | null>(null);

  function openInvite() {
    inviteError = null;
    inviteOpen = true;
  }
  async function doInvite(input: InviteUserInput) {
    if (inviteSubmitting) return;
    inviteSubmitting = true;
    inviteError = null;
    try {
      const res = await inviteUser(input);
      inviteOpen = false;
      void controller.refresh({ silent: true });
      if (res.email_sent) {
        toast.push({ type: 'success', message: $t(res.created ? 'users.toast.invited' : 'users.toast.invitationResent', { email: res.user.email }) });
      } else if (!showCredentials(res)) {
        toast.push({ type: 'error', message: $t('users.toast.inviteDeliveryFailed') });
      }
    } catch (e) {
      inviteError = toUserFormError(e);
    } finally {
      inviteSubmitting = false;
    }
  }
  function showInList(email: string) {
    inviteOpen = false;
    controller.setFilters({ role: '', status: '', q: email });
  }

  // Dialog kirim ulang undangan
  let resendOpen = $state(false);
  let resendTarget = $state<User | null>(null);
  let resendSubmitting = $state(false);
  let resendError = $state<UserFormError | null>(null);

  function openResend(u: User) {
    resendTarget = u;
    resendError = null;
    resendOpen = true;
  }
  async function doResend() {
    if (!resendTarget || resendSubmitting) return;
    const target = resendTarget;
    resendSubmitting = true;
    resendError = null;
    try {
      const res = await resendUserInvitation(target.id);
      resendOpen = false;
      void controller.refresh({ silent: true });
      if (res.email_sent) toast.push({ type: 'success', message: $t('users.toast.invitationResent', { email: target.email }) });
      else showCredentials(res);
    } catch (e) {
      resendError = toUserFormError(e);
      if (resendError.kind === 'notFound') void controller.refresh({ silent: true });
    } finally {
      resendSubmitting = false;
    }
  }

  // Dialog buat
  let createOpen = $state(false);
  let createSubmitting = $state(false);
  let createError = $state<UserFormError | null>(null);
  let createResult = $state<{ user: User; temporaryPassword: string | null } | null>(null);

  async function doCreate(input: CreateUserInput) {
    createSubmitting = true;
    createError = null;
    try {
      const res = await createUser(input);
      toast.push({ type: 'success', message: $t('users.toast.created') });
      void controller.refresh({ silent: true });
      if (res.temporary_password !== null) createResult = { user: res.user, temporaryPassword: res.temporary_password };
      else createOpen = false;
    } catch (e) {
      createError = toUserFormError(e);
    } finally {
      createSubmitting = false;
    }
  }
  function closeCreate() {
    createResult = null;
    createError = null;
  }

  // Dialog edit
  let editOpen = $state(false);
  let editUser = $state<User | null>(null);
  let editLoading = $state(false);
  let editLoadError = $state<UserFormError | null>(null);
  let editSubmitting = $state(false);
  let editError = $state<UserFormError | null>(null);
  let editTargetId = '';
  let editAbort: AbortController | null = null;

  async function loadEdit() {
    editAbort?.abort();
    const ctl = new AbortController();
    editAbort = ctl;
    editLoading = true;
    editLoadError = null;
    try {
      const u = await fetchUser(editTargetId, ctl.signal);
      if (!ctl.signal.aborted) editUser = u;
    } catch (e) {
      if (!ctl.signal.aborted) {
        editLoadError = toUserFormError(e);
        if (editLoadError.kind === 'notFound') void controller.refresh({ silent: true });
      }
    } finally {
      if (!ctl.signal.aborted) editLoading = false;
    }
  }
  function openEdit(u: User) {
    editTargetId = u.id;
    editUser = null;
    editError = null;
    editOpen = true;
    void loadEdit();
  }
  async function doEdit(changes: Partial<{ name: string; email: string; role: Role }>) {
    editSubmitting = true;
    editError = null;
    try {
      const u = await updateUser(editTargetId, changes);
      usersListStore.replaceUser(u);
      void controller.refresh({ silent: true });
      toast.push({ type: 'success', message: $t('users.toast.updated') });
      editOpen = false;
    } catch (e) {
      editError = toUserFormError(e);
      if (editError.kind === 'notFound') void controller.refresh({ silent: true });
    } finally {
      editSubmitting = false;
    }
  }
  function closeEdit() {
    editAbort?.abort();
    editUser = null;
    editError = null;
    editLoadError = null;
  }

  // Dialog status
  let statusOpen = $state(false);
  let statusUser = $state<User | null>(null);
  let statusSubmitting = $state(false);
  let statusError = $state<UserFormError | null>(null);

  function openStatus(u: User) {
    statusUser = u;
    statusError = null;
    statusOpen = true;
  }
  async function doStatus() {
    if (!statusUser) return;
    const target = statusUser;
    statusSubmitting = true;
    statusError = null;
    try {
      const u = await setUserStatus(target.id, !target.is_active);
      usersListStore.replaceUser(u);
      void controller.refresh({ silent: true });
      toast.push({ type: 'success', message: $t(u.is_active ? 'users.toast.activated' : 'users.toast.deactivated') });
      statusOpen = false;
    } catch (e) {
      statusError = toUserFormError(e);
      if (statusError.kind === 'notFound') void controller.refresh({ silent: true });
    } finally {
      statusSubmitting = false;
    }
  }

  // Dialog reset password (rahasia hanya di state komponen ini)
  let resetOpen = $state(false);
  let resetUser = $state<User | null>(null);
  let resetSubmitting = $state(false);
  let resetError = $state<UserFormError | null>(null);
  let resetSecret = $state<string | null>(null);

  function openReset(u: User) {
    resetUser = u;
    resetError = null;
    resetSecret = null;
    resetOpen = true;
  }
  async function doReset(password?: string) {
    if (!resetUser) return;
    resetSubmitting = true;
    resetError = null;
    try {
      const res = await resetUserPassword(resetUser.id, password);
      usersListStore.replaceUser(res.user);
      void controller.refresh({ silent: true });
      toast.push({ type: 'success', message: $t('users.toast.passwordReset') });
      if (res.temporary_password !== null) resetSecret = res.temporary_password;
      else resetOpen = false;
    } catch (e) {
      resetError = toUserFormError(e, 'reset');
      if (resetError.kind === 'notFound') void controller.refresh({ silent: true });
    } finally {
      resetSubmitting = false;
    }
  }
  function closeReset() {
    resetSecret = null;
    resetError = null;
    resetUser = null;
  }

  const filterError = $derived($usersList.error?.status === 400 ? $usersList.error.message : undefined);
  const tableError = $derived($usersList.error && $usersList.error.status !== 400 ? $usersList.error : null);
  const items = $derived($usersList.data?.items ?? []);
</script>

<section class="flex flex-col gap-4">
  <PageHeader title={$t('users.title')}>
    {#snippet action()}
      {#if $usersList.refreshing}<span class="text-sm text-muted" role="status">{$t('common.loading')}</span>{/if}
      <Button variant="secondary" onclick={() => (createOpen = true)}>{$t('users.create')}</Button>
      <Button onclick={openInvite}>{$t('users.invite.open')}</Button>
    {/snippet}
  </PageHeader>

  <UsersFilterBar
    filters={$usersList.filters}
    hasFilters={$hasFilters}
    error={filterError}
    onchange={(p) => controller.setFilters(p)}
  />

  {#if !$usersList.loading && !tableError && $usersList.data && items.length === 0 && $usersList.data.total === 0}
    <EmptyState title={$hasFilters ? $t('users.emptyFiltered') : $t('users.empty')}>
      {#snippet action()}
        {#if !$hasFilters}
          <Button onclick={openInvite}>{$t('users.invite.open')}</Button>
          <Button variant="secondary" onclick={() => (createOpen = true)}>{$t('users.create')}</Button>
        {:else}
          <Button variant="secondary" onclick={() => controller.setFilters({ role: '', status: '', q: '' })}>{$t('users.filter.reset')}</Button>
        {/if}
      {/snippet}
    </EmptyState>
  {:else}
    <UsersTable
      users={items}
      {selfId}
      loading={$usersList.loading && !$usersList.data}
      error={tableError}
      onretry={() => controller.load()}
      onedit={openEdit}
      onstatus={openStatus}
      onreset={openReset}
      onresend={openResend}
    />
  {/if}

  {#if $usersList.data && $usersList.data.total > PAGE_SIZE}
    <Pagination page={$usersList.page} pageSize={PAGE_SIZE} total={$usersList.data.total} onchange={(n) => controller.setPage(n)} />
  {/if}
</section>

<CreateUserDialog
  bind:open={createOpen}
  submitting={createSubmitting}
  error={createError}
  result={createResult}
  onsubmit={doCreate}
  onclose={closeCreate}
/>
<EditUserDialog
  bind:open={editOpen}
  user={editUser}
  loading={editLoading}
  loadError={editLoadError}
  isSelf={editUser?.id === selfId}
  submitting={editSubmitting}
  error={editError}
  onsubmit={doEdit}
  onretry={loadEdit}
  onclose={closeEdit}
/>
<ConfirmStatusDialog bind:open={statusOpen} user={statusUser} submitting={statusSubmitting} error={statusError} onconfirm={doStatus} onclose={() => (statusError = null)} />
<ResetPasswordDialog
  bind:open={resetOpen}
  user={resetUser}
  submitting={resetSubmitting}
  error={resetError}
  temporaryPassword={resetSecret}
  onsubmit={doReset}
  onclose={closeReset}
/>
{#if inviteOpen}
<InviteDeveloperDialog
  bind:open={inviteOpen}
  submitting={inviteSubmitting}
  error={inviteError}
  {emailEnabled}
  onsubmit={doInvite}
  onshowinlist={showInList}
  onclose={() => (inviteError = null)}
/>
{/if}
{#if resendOpen}
<ResendInvitationDialog
  bind:open={resendOpen}
  user={resendTarget}
  submitting={resendSubmitting}
  error={resendError}
  onconfirm={doResend}
  onclose={() => (resendError = null)}
/>
{/if}
<TemporaryCredentialsDialog
  open={creds !== null}
  credentials={creds?.credentials ?? null}
  emailStatus={creds?.emailStatus ?? 'not_configured'}
  expiresAt={creds?.expiresAt ?? ''}
  onclose={() => (creds = null)}
/>
