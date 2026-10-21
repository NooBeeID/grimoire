<script lang="ts">
  import Button from '../../../../../shared/components/Button.svelte';
  import { t } from '../../../../../shared/stores/i18n';
  import type { CandidateState } from '../stores/candidateSearch.store';
  import type { Candidate, MemberRole } from '../types';
  import CandidateCombobox from './CandidateCombobox.svelte';

  let {
    search,
    selected = null,
    role = $bindable<MemberRole>('member'),
    submitting = false,
    error = null,
    oninput,
    onmove,
    onselect,
    onclear,
    onsubmit,
    oncancel,
  }: {
    search: CandidateState;
    selected?: Candidate | null;
    role?: MemberRole;
    submitting?: boolean;
    error?: string | null;
    oninput: (q: string) => void;
    onmove: (delta: number) => void;
    onselect: (c: Candidate) => void;
    onclear: () => void;
    onsubmit: () => void;
    oncancel: () => void;
  } = $props();

  function submit(e: SubmitEvent) {
    e.preventDefault();
    if (selected && !submitting) onsubmit();
  }
</script>

<form onsubmit={submit} class="flex flex-col gap-4">
  <CandidateCombobox {search} {selected} {oninput} {onmove} {onselect} {onclear} />
  <fieldset class="flex flex-col gap-1" disabled={submitting}>
    <legend class="text-sm font-medium">{$t('project.members.add.role')}</legend>
    <label class="flex items-center gap-2 text-sm">
      <input type="radio" name="role" value="member" bind:group={role} /> {$t('project.role.member')}
    </label>
    <label class="flex items-center gap-2 text-sm">
      <input type="radio" name="role" value="owner" bind:group={role} /> {$t('project.role.owner')}
    </label>
    <p class="text-sm text-muted">{$t('project.members.add.ownerHint')}</p>
  </fieldset>
  {#if error}<p role="alert" class="text-sm text-danger">{error}</p>{/if}
  <div class="flex justify-end gap-2">
    <Button variant="secondary" onclick={oncancel} disabled={submitting}>{$t('common.cancel')}</Button>
    <Button type="submit" disabled={!selected} loading={submitting}>{$t('project.members.add.submit')}</Button>
  </div>
</form>
