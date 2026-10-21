<script lang="ts">
  import { untrack } from 'svelte';
  import Button from '../../../../../shared/components/Button.svelte';
  import Dialog from '../../../../../shared/components/Dialog.svelte';
  import TemporaryCredentialsDialog from '../../../../../shared/components/TemporaryCredentialsDialog.svelte';
  import EmptyState from '../../../../../shared/components/EmptyState.svelte';
  import ErrorState from '../../../../../shared/components/ErrorState.svelte';
  import Spinner from '../../../../../shared/components/Spinner.svelte';
  import { ApiError, isApiError } from '../../../../../shared/services/http';
  import { t } from '../../../../../shared/stores/i18n';
  import { toast } from '../../../../../shared/stores/toast';
  import type { TemporaryCredentials } from '../../../../../shared/validation/credentials';
  import { fieldErrorsOf } from '../../../../../shared/validation/error';
  import AddMemberForm from '../components/AddMemberForm.svelte';
  import ConfirmActionDialog from '../components/ConfirmActionDialog.svelte';
  import InvitationsSection from '../components/InvitationsSection.svelte';
  import InviteByEmailForm from '../components/InviteByEmailForm.svelte';
  import MembersTable from '../components/MembersTable.svelte';
  import {
    addMember,
    changeMemberRole,
    listMembers,
    memberErrorKey,
    removeMember,
    searchCandidates,
  } from '../services/members.service';
  import {
    inviteToProject,
    listInvitations,
    resendProjectInvitation,
    revokeInvitation,
    toInviteError,
    type InviteErrorView,
  } from '../services/invitations.service';
  import { inviteOptionFor } from '../model/invite';
  import { createInvitationsListStore } from '../stores/invitationsList.store';
  import { createCandidateSearchStore } from '../stores/candidateSearch.store';
  import { createMembersListStore } from '../stores/membersList.store';
  import { CANDIDATE_DEBOUNCE_MS, type AccessLevel, type Candidate, type Invitation, type Member, type MemberRole } from '../types';

  let {
    projectId,
    accessLevel,
    currentUserId,
    emailEnabled = null,
    onleft,
    onaccesschanged,
    onprojectgone,
  }: {
    projectId: string;
    accessLevel: AccessLevel;
    currentUserId: string;
    emailEnabled?: boolean | null;
    onleft: () => void;
    onaccesschanged: () => void;
    onprojectgone: () => void;
  } = $props();

  const canManage = $derived(accessLevel === 'owner' || accessLevel === 'admin');
  const list = createMembersListStore();
  const search = createCandidateSearchStore();
  const invitations = createInvitationsListStore();

  const toApi = (e: unknown) => (isApiError(e) ? e : new ApiError({ status: 0, code: 'INTERNAL', message: String(e) }));
  const isAbort = (e: unknown, c: AbortController) =>
    c.signal.aborted || (e instanceof DOMException && e.name === 'AbortError');
  const withTrace = (e: ApiError) => (e.traceId ? `${e.message} (${$t('error.trace_id', { id: e.traceId })})` : e.message);

  // ---- muat daftar ----
  let listCtrl: AbortController | null = null;
  async function load(id: string) {
    listCtrl?.abort();
    const c = new AbortController();
    listCtrl = c;
    list.begin();
    try {
      const items = await listMembers(id, c.signal);
      if (c.signal.aborted) return;
      list.resolve(items);
    } catch (e) {
      if (isAbort(e, c)) return;
      const err = toApi(e);
      if (err.status === 404) {
        onprojectgone();
        return;
      }
      list.fail(err);
    }
  }
  $effect(() => {
    const id = projectId;
    void untrack(() => load(id));
    return () => listCtrl?.abort();
  });

  // ---- muat undangan (hanya owner/super_admin) ----
  let invCtrl: AbortController | null = null;
  async function loadInvitations(id: string, silent = false) {
    invCtrl?.abort();
    const c = new AbortController();
    invCtrl = c;
    if (silent) invitations.refresh();
    else invitations.begin();
    try {
      const items = await listInvitations(id, c.signal);
      if (c.signal.aborted) return;
      invitations.resolve(items);
    } catch (e) {
      if (isAbort(e, c)) return;
      const err = toApi(e);
      if (err.status === 404) {
        onprojectgone();
        return;
      }
      if (err.status === 403) {
        invitations.clear();
        onaccesschanged();
        return;
      }
      invitations.fail(err);
    }
  }
  const refreshInvitations = () => loadInvitations(projectId, true);
  $effect(() => {
    const id = projectId;
    if (!canManage) {
      untrack(() => invitations.clear());
      return;
    }
    void untrack(() => loadInvitations(id));
    return () => invCtrl?.abort();
  });

  // ---- penanganan error mutasi ----
  let busyUserId = $state<string | null>(null);
  let addOpen = $state(false);
  let confirm = $state<{ kind: 'demote' | 'remove' | 'leave'; member: Member } | null>(null);

  function closeDialogs() {
    addOpen = false;
    confirm = null;
    invConfirm = null;
  }

  // 404 bisa berarti project hilang atau anggota target hilang: cek ulang daftar.
  async function recheckAfter404(): Promise<void> {
    try {
      const items = await listMembers(projectId);
      list.resolve(items);
      closeDialogs();
      toast.push({ type: 'error', message: $t('project.members.error.gone') });
    } catch (e) {
      const err = toApi(e);
      if (err.status === 404) onprojectgone();
      else toast.fromError(err);
    }
  }

  // Mengembalikan true bila error sudah ditangani dan dialog harus ditutup.
  async function handleError(e: unknown, setInline: (msg: string) => void): Promise<void> {
    const err = toApi(e);
    if (err.status === 403) {
      closeDialogs();
      toast.push({ type: 'error', message: $t('error.forbidden') });
      onaccesschanged();
      return;
    }
    if (err.status === 404) {
      await recheckAfter404();
      return;
    }
    const key = memberErrorKey(err);
    if (key) {
      setInline($t(key));
      if (err.status === 409 && key === 'project.error.ALREADY_MEMBER') void load(projectId);
      return;
    }
    const fields = err.status === 400 ? fieldErrorsOf(err.details) : {};
    const f = fields.user_id ?? fields.role;
    setInline(f ?? withTrace(err));
  }

  // ---- tambah anggota ----
  let selected = $state<Candidate | null>(null);
  let addRole = $state<MemberRole>('member');
  let adding = $state(false);
  let addMode = $state<'search' | 'invite'>('search');
  let inviteEmail = $state('');
  let inviteName = $state('');
  let inviteError = $state<InviteErrorView | null>(null);
  let inviting = $state(false);
  let creds = $state<{ credentials: TemporaryCredentials; emailStatus: 'failed' | 'not_configured'; expiresAt: string } | null>(null);
  let addError = $state<string | null>(null);
  let searchCtrl: AbortController | null = null;
  let searchTimer: ReturnType<typeof setTimeout> | null = null;
  let seq = 0;

  function resetSearch() {
    if (searchTimer) clearTimeout(searchTimer);
    searchTimer = null;
    searchCtrl?.abort();
    search.clear();
  }
  function openAdd() {
    addMode = 'search';
    inviteError = null;
    selected = null;
    addRole = 'member';
    addError = null;
    resetSearch();
    addOpen = true;
  }
  function closeAdd() {
    addOpen = false;
    resetSearch();
  }
  $effect(() => {
    if (!addOpen) untrack(resetSearch);
  });
  $effect(() => () => {
    if (searchTimer) clearTimeout(searchTimer);
    searchCtrl?.abort();
  });

  function oninput(q: string) {
    addError = null;
    if (searchTimer) clearTimeout(searchTimer);
    searchTimer = null;
    searchCtrl?.abort();
    search.setQuery(q);
    if (q.trim() === '') return;
    searchTimer = setTimeout(() => {
      searchTimer = null;
      void runSearch(q);
    }, CANDIDATE_DEBOUNCE_MS);
  }
  async function runSearch(q: string) {
    const c = new AbortController();
    searchCtrl = c;
    const n = ++seq;
    search.begin(n);
    try {
      const items = await searchCandidates(projectId, q, c.signal);
      if (c.signal.aborted) return;
      // Pertahanan ganda: saring yang sudah menjadi anggota.
      const have = new Set($list.items.map((m) => m.user_id));
      search.resolve(n, items.filter((x) => !have.has(x.id)));
      search.setInviteOption(inviteOptionFor(q, $search, $list.items));
    } catch (e) {
      if (isAbort(e, c)) return;
      search.fail(n, toApi(e));
    }
  }
  function onselect(c: Candidate) {
    selected = c;
    addError = null;
  }
  function onclear() {
    selected = null;
    resetSearch();
  }

  function oninvite(email: string) {
    inviteEmail = email;
    inviteName = '';
    inviteError = null;
    addMode = 'invite';
    resetSearch();
  }
  function onback() {
    addMode = 'search';
    inviteError = null;
    resetSearch();
  }
  function onsearchinstead() {
    const email = inviteEmail;
    addMode = 'search';
    inviteError = null;
    oninput(email);
  }

  // Kredensial hanya hidup di state ini; tidak masuk store mana pun.
  function showCredentials(res: { credentials?: TemporaryCredentials; email_status: string; expires_at: string }) {
    if (!res.credentials) return false;
    creds = {
      credentials: res.credentials,
      emailStatus: res.email_status === 'failed' ? 'failed' : 'not_configured',
      expiresAt: res.expires_at,
    };
    return true;
  }

  async function submitInvite() {
    if (inviting) return;
    inviting = true;
    inviteError = null;
    const email = inviteEmail;
    try {
      const res = await inviteToProject(projectId, { email, role: addRole, name: inviteName });
      list.upsert(res.member);
      void refreshInvitations();
      closeAdd();
      if (res.email_sent) {
        toast.push({ type: 'success', message: $t(res.created ? 'project.members.invite.success.sent' : 'project.members.invite.success.resent', { email: res.member.email ?? email }) });
      } else if (!showCredentials(res)) {
        toast.push({ type: 'error', message: $t('project.members.invite.deliveryFailed', { email: res.member.email ?? email }) });
      }
    } catch (e) {
      const view = toInviteError(e, 'invite');
      if (view.kind === 'forbidden') {
        closeDialogs();
        toast.push({ type: 'error', message: $t('error.forbidden') });
        onaccesschanged();
      } else if (view.kind === 'notFound') {
        await recheckAfter404();
      } else {
        inviteError = view;
        if (view.kind === 'alreadyMember') void load(projectId);
      }
    } finally {
      inviting = false;
    }
  }

  async function submitAdd() {
    if (!selected || adding) return;
    adding = true;
    addError = null;
    try {
      const m = await addMember(projectId, selected.id, addRole);
      list.upsert(m);
      closeAdd();
      toast.push({ type: 'success', message: $t('project.members.add.success', { name: m.name ?? m.email ?? '' }) });
    } catch (e) {
      await handleError(e, (msg) => (addError = msg));
    } finally {
      adding = false;
    }
  }

  // ---- ubah peran, keluarkan, keluar ----
  let confirmSubmitting = $state(false);
  let confirmError = $state<string | null>(null);

  async function applyRole(m: Member, role: MemberRole, setInline: (msg: string) => void) {
    try {
      const updated = await changeMemberRole(projectId, m.user_id, role);
      list.upsert(updated);
      confirm = null;
      toast.push({ type: 'success', message: $t('project.members.role.success') });
      if (m.user_id === currentUserId) onaccesschanged();
    } catch (e) {
      await handleError(e, setInline);
    }
  }

  async function onrequestrole(m: Member, role: MemberRole) {
    if (role === 'member') {
      confirmError = null;
      confirm = { kind: 'demote', member: m };
      return;
    }
    // Promosi langsung tanpa dialog.
    busyUserId = m.user_id;
    try {
      await applyRole(m, 'owner', (msg) => toast.push({ type: 'error', message: msg }));
    } finally {
      busyUserId = null;
    }
  }
  function onrequestremove(m: Member) {
    confirmError = null;
    confirm = { kind: 'remove', member: m };
  }
  function onrequestleave(m: Member) {
    confirmError = null;
    confirm = { kind: 'leave', member: m };
  }

  async function runConfirm() {
    const c = confirm;
    if (!c) return;
    confirmSubmitting = true;
    confirmError = null;
    const setInline = (msg: string) => (confirmError = msg);
    try {
      if (c.kind === 'demote') {
        await applyRole(c.member, 'member', setInline);
      } else {
        try {
          await removeMember(projectId, c.member.user_id);
          list.remove(c.member.user_id);
          confirm = null;
          if (c.kind === 'leave') {
            toast.push({ type: 'success', message: $t('project.members.leave.success') });
            onleft();
          } else {
            toast.push({ type: 'success', message: $t('project.members.remove.success') });
          }
        } catch (e) {
          await handleError(e, setInline);
        }
      }
    } finally {
      confirmSubmitting = false;
    }
  }

  // ---- kirim ulang dan batalkan undangan ----
  let invConfirm = $state<{ kind: 'resend' | 'revoke'; item: Invitation } | null>(null);
  let invSubmitting = $state(false);
  let invError = $state<string | null>(null);
  let invBusyUserId = $state<string | null>(null);

  function onrequestresend(item: Invitation) {
    invError = null;
    invConfirm = { kind: 'resend', item };
  }
  function onrequestrevoke(item: Invitation) {
    invError = null;
    invConfirm = { kind: 'revoke', item };
  }
  function invInline(view: InviteErrorView): string {
    switch (view.kind) {
      case 'rateLimited':
        return $t(`invitation.error.rateLimited.${view.scope}`, { minutes: view.minutes });
      case 'notPending':
        return $t('invitation.error.notPending');
      case 'alreadyRegistered':
        return $t(`invitation.error.alreadyRegistered.${view.reason}`);
      case 'lastOwner':
        return $t('project.error.LAST_OWNER');
      case 'generic':
        return view.traceId ? `${view.message} (${$t('error.trace_id', { id: view.traceId })})` : view.message;
      default:
        return $t('error.internal');
    }
  }
  async function runInvConfirm() {
    const c = invConfirm;
    if (!c || invSubmitting) return;
    invSubmitting = true;
    invError = null;
    invBusyUserId = c.item.user_id;
    const { item } = c;
    try {
      if (c.kind === 'resend') {
        const res = await resendProjectInvitation(projectId, item.user_id);
        invConfirm = null;
        void refreshInvitations();
        if (res.email_sent) toast.push({ type: 'success', message: $t('project.invitations.resend.success', { email: item.email }) });
        else showCredentials(res);
      } else {
        await revokeInvitation(projectId, item.user_id);
        invConfirm = null;
        invitations.remove(item.user_id);
        list.remove(item.user_id);
        toast.push({ type: 'success', message: $t('project.invitations.revoke.success') });
      }
    } catch (e) {
      const view = toInviteError(e, 'resend');
      if (view.kind === 'forbidden') {
        invConfirm = null;
        toast.push({ type: 'error', message: $t('error.forbidden') });
        onaccesschanged();
      } else if (view.kind === 'notFound') {
        invConfirm = null;
        await recheckAfter404();
        void refreshInvitations();
      } else if (view.kind === 'notPending') {
        if (c.kind === 'revoke') {
          invError = $t('project.invitations.revoke.notPending');
        } else {
          invConfirm = null;
          toast.push({ type: 'error', message: $t('invitation.error.notPending') });
        }
        void refreshInvitations();
        void load(projectId);
      } else {
        invError = invInline(view);
      }
    } finally {
      invSubmitting = false;
      invBusyUserId = null;
    }
  }
  const invConfirmView = $derived.by(() => {
    if (!invConfirm) return null;
    const email = invConfirm.item.email;
    return invConfirm.kind === 'resend'
      ? { title: $t('project.invitations.resend.title'), message: $t('project.invitations.resend.message', { email }), label: $t('project.invitations.resend') }
      : { title: $t('project.invitations.revoke.title'), message: $t('project.invitations.revoke.message', { email }), label: $t('project.invitations.revoke') };
  });

  const nameOf = (m: Member) => m.name ?? m.email ?? $t('project.members.unknownUser');
  const confirmView = $derived.by(() => {
    if (!confirm) return null;
    const m = confirm.member;
    if (confirm.kind === 'demote') {
      const self = m.user_id === currentUserId;
      return {
        title: $t('project.members.demote.title'),
        message: $t('project.members.demote.message', { name: nameOf(m) }) + (self ? ' ' + $t('project.members.demote.selfWarning') : ''),
        label: $t('project.members.makeMember'),
      };
    }
    if (confirm.kind === 'remove') {
      return {
        title: $t('project.members.remove.title'),
        message: $t('project.members.remove.message', { name: nameOf(m) }),
        label: $t('project.members.remove'),
      };
    }
    return {
      title: $t('project.members.leave.title'),
      message: $t('project.members.leave.message'),
      label: $t('project.members.leave'),
    };
  });
</script>

<section class="flex flex-col gap-4">
  <header class="flex flex-wrap items-center justify-between gap-2">
    <h2 class="text-base font-semibold">
      {$t('project.members.title')}{#if $list.status === 'ready'}<span class="ml-2 font-normal text-muted">{$list.items.length}</span>{/if}
    </h2>
    {#if canManage}<Button onclick={openAdd}>{$t('project.members.add.open')}</Button>{/if}
  </header>

  {#if canManage}
    <InvitationsSection
      state={$invitations}
      busyUserId={invBusyUserId}
      onretry={() => loadInvitations(projectId)}
      {onrequestresend}
      {onrequestrevoke}
    />
  {/if}

  {#if $list.status === 'idle' || $list.status === 'loading'}
    <div class="p-8 text-center" aria-busy="true"><Spinner /></div>
  {:else if $list.status === 'error'}
    <ErrorState error={$list.error} onretry={() => load(projectId)} />
  {:else if $list.items.length === 0}
    <EmptyState title={$t('project.members.empty')}>
      {#snippet action()}{#if canManage}<Button onclick={openAdd}>{$t('project.members.add.open')}</Button>{/if}{/snippet}
    </EmptyState>
  {:else}
    <MembersTable items={$list.items} {currentUserId} {accessLevel} {busyUserId} {onrequestrole} {onrequestremove} {onrequestleave} />
  {/if}
</section>

<Dialog open={addOpen} title={addMode === 'invite' ? $t('project.members.invite.title') : $t('project.members.add.title')} onclose={closeAdd}>
  {#if addMode === 'invite'}
    <InviteByEmailForm
      email={inviteEmail}
      bind:role={addRole}
      bind:name={inviteName}
      {emailEnabled}
      submitting={inviting}
      error={inviteError}
      onsubmit={submitInvite}
      {onback}
      oncancel={closeAdd}
      {onsearchinstead}
    />
  {:else}
    <AddMemberForm
      search={$search}
      {selected}
      bind:role={addRole}
      submitting={adding}
      error={addError}
      {oninput}
      onmove={(d) => search.move(d)}
      {onselect}
      {onclear}
      {oninvite}
      onsubmit={submitAdd}
      oncancel={closeAdd}
    />
  {/if}
</Dialog>

{#if confirmView}
  <ConfirmActionDialog
    open={confirm !== null}
    title={confirmView.title}
    message={confirmView.message}
    confirmLabel={confirmView.label}
    variant={confirm?.kind === 'demote' ? 'primary' : 'danger'}
    submitting={confirmSubmitting}
    error={confirmError}
    onconfirm={runConfirm}
    oncancel={() => (confirm = null)}
  />
{/if}

{#if invConfirmView}
  <ConfirmActionDialog
    open={invConfirm !== null}
    title={invConfirmView.title}
    message={invConfirmView.message}
    confirmLabel={invConfirmView.label}
    variant={invConfirm?.kind === 'resend' ? 'primary' : 'danger'}
    submitting={invSubmitting}
    error={invError}
    onconfirm={runInvConfirm}
    oncancel={() => (invConfirm = null)}
  />
{/if}

<TemporaryCredentialsDialog
  open={creds !== null}
  credentials={creds?.credentials ?? null}
  emailStatus={creds?.emailStatus ?? 'not_configured'}
  expiresAt={creds?.expiresAt ?? ''}
  onclose={() => (creds = null)}
/>
