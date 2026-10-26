<script lang="ts">
  import Button from '../../../../../shared/components/Button.svelte';
  import Dialog from '../../../../../shared/components/Dialog.svelte';
  import { isApiError } from '../../../../../shared/services/http';
  import { t } from '../../../../../shared/stores/i18n';
  import { toast } from '../../../../../shared/stores/toast';
  import { reasonOf } from '../../../../../shared/validation/error';
  import type { SynonymGroup } from '../../listgroups';
  import TermsInput from '../components/TermsInput.svelte';
  import { normalizeTerms, type TermsIssue } from '../model/terms';
  import { createGroup, updateGroup } from '../services/savegroup.service';

  let {
    open,
    projectId,
    group,
    onclose,
    onsaved,
    onstale,
    onprojectgone,
  }: {
    open: boolean;
    projectId: string;
    group: SynonymGroup | null;
    onclose: () => void;
    onsaved: (g: SynonymGroup) => void;
    onstale: () => void;
    onprojectgone: () => void;
  } = $props();

  let terms = $state<string[]>([]);
  let draft = $state('');
  let submitting = $state(false);
  let formError = $state<string | null>(null);
  let traceId = $state<string | undefined>();

  $effect(() => {
    if (open) {
      terms = group ? [...group.terms] : [];
      draft = '';
      formError = null;
      traceId = undefined;
    }
  });

  // Term yang sedang diketik ikut dihitung supaya submit tidak kehilangan isian terakhir.
  const current = $derived(draft.trim() === '' ? terms : [...terms, draft]);
  const check = $derived(normalizeTerms(current));
  // Pesan "kurang term" baru muncul setelah ada isian, supaya form kosong tidak langsung merah.
  const issue = $derived<TermsIssue | null>(current.length === 0 ? null : check.issue);

  async function submit() {
    if (submitting || check.issue) return;
    submitting = true;
    formError = null;
    traceId = undefined;
    try {
      const saved = group ? await updateGroup(projectId, group.id, check.terms) : await createGroup(projectId, check.terms);
      onclose();
      onsaved(saved);
      toast.push({ type: 'success', message: $t('searchSynonym.saved') });
    } catch (err) {
      if (!isApiError(err)) {
        formError = String(err);
      } else {
        const reason = reasonOf(err.details);
        if (err.status === 409 && reason === 'term_in_use') {
          const used = (err.details as { terms?: unknown } | null)?.terms;
          formError = $t('searchSynonym.error.term_in_use', { terms: Array.isArray(used) ? used.join(', ') : '' });
        } else if (err.status === 409 && reason === 'group_limit') {
          formError = $t('searchSynonym.error.group_limit');
        } else if (err.status === 404 && group) {
          // Kelompok sudah dihapus orang lain; bila project yang hilang, daftar yang dimuat ulang melaporkannya.
          onclose();
          toast.push({ type: 'error', message: $t('searchSynonym.gone') });
          onstale();
        } else if (err.status === 404) {
          onclose();
          onprojectgone();
        } else if (err.status === 400 && reason && reason in VALIDATION) {
          formError = $t(VALIDATION[reason]!);
        } else {
          formError = err.message;
          traceId = err.traceId;
        }
      }
    } finally {
      submitting = false;
    }
  }

  const VALIDATION: Record<string, string> = {
    too_few_terms: 'searchSynonym.validation.tooFew',
    too_many_terms: 'searchSynonym.validation.tooMany',
    term_empty: 'searchSynonym.validation.empty',
    term_too_long: 'searchSynonym.validation.tooLong',
    term_invalid_char: 'searchSynonym.validation.invalidChar',
    duplicate_term: 'searchSynonym.validation.duplicate',
  };
</script>

<Dialog {open} title={$t(group ? 'searchSynonym.edit.title' : 'searchSynonym.create.title')} dismissible={!submitting} {onclose}>
  <form onsubmit={(e) => { e.preventDefault(); void submit(); }} class="flex flex-col gap-4" novalidate>
    <TermsInput
      bind:terms
      bind:draft
      label={$t('searchSynonym.form.terms')}
      hint={$t('searchSynonym.form.hint')}
      error={issue ? $t(`searchSynonym.validation.${issue}`) : undefined}
      disabled={submitting}
    />
    <div role="alert">
      {#if formError}
        <p class="text-sm text-danger">{formError}</p>
        {#if traceId}<p class="text-xs text-muted">{$t('error.trace_id', { id: traceId })}</p>{/if}
      {/if}
    </div>
  </form>
  {#snippet footer()}
    <Button variant="secondary" disabled={submitting} onclick={onclose}>{$t('common.cancel')}</Button>
    <Button loading={submitting} disabled={check.issue !== null} onclick={submit}>{$t('searchSynonym.form.submit')}</Button>
  {/snippet}
</Dialog>
