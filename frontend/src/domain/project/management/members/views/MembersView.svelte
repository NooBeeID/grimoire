<script lang="ts">
  import { untrack } from 'svelte';
  import Button from '../../../../../shared/components/Button.svelte';
  import Dialog from '../../../../../shared/components/Dialog.svelte';
  import EmptyState from '../../../../../shared/components/EmptyState.svelte';
  import ErrorState from '../../../../../shared/components/ErrorState.svelte';
  import Spinner from '../../../../../shared/components/Spinner.svelte';
  import { ApiError, isApiError } from '../../../../../shared/services/http';
  import { t } from '../../../../../shared/stores/i18n';
  import { toast } from '../../../../../shared/stores/toast';
  import { fieldErrorsOf } from '../../../../../shared/validation/error';
  import AddMemberForm from '../components/AddMemberForm.svelte';
  import ConfirmActionDialog from '../components/ConfirmActionDialog.svelte';
  import MembersTable from '../components/MembersTable.svelte';
  import {
    addMember,
    changeMemberRole,
    listMembers,
    memberErrorKey,
    removeMember,
    searchCandidates,
  } from '../services/members.service';
  import { createCandidateSearchStore } from '../stores/candidateSearch.store';
  import { createMembersListStore } from '../stores/membersList.store';
  import { CANDIDATE_DEBOUNCE_MS, type AccessLevel, type Candidate, type Member, type MemberRole } from '../types';

  let {
    projectId,
    accessLevel,
    currentUserId,
    onleft,
    onaccesschanged,
    onprojectgone,
  }: {
    projectId: string;
    accessLevel: AccessLevel;
    currentUserId: string;
    onleft: () => void;
    onaccesschanged: () => void;
    onprojectgone: () => void;
  } = $props();

  const canManage = $derived(accessLevel === 'owner' || accessLevel === 'admin');
  const list = createMembersListStore();
  const search = createCandidateSearchStore();

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

  // ---- penanganan error mutasi ----
  let busyUserId = $state<string | null>(null);
  let addOpen = $state(false);
  let confirm = $state<{ kind: 'demote' | 'remove' | 'leave'; member: Member } | null>(null);

  function closeDialogs() {
    addOpen = false;
    confirm = null;
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
    <h2 class="text-lg font-semibold">
      {$t('project.members.title')}{#if $list.status === 'ready'} <span class="text-muted">({$list.items.length})</span>{/if}
    </h2>
    {#if canManage}<Button onclick={openAdd}>{$t('project.members.add.open')}</Button>{/if}
  </header>

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

<Dialog open={addOpen} title={$t('project.members.add.title')} onclose={closeAdd}>
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
    onsubmit={submitAdd}
    oncancel={closeAdd}
  />
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
