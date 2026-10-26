<script lang="ts">
  import { t } from '../../../../../shared/stores/i18n';

  let {
    terms = $bindable([]),
    draft = $bindable(''),
    label,
    hint,
    error,
    disabled = false,
  }: {
    terms?: string[];
    draft?: string;
    label: string;
    hint?: string;
    error?: string;
    disabled?: boolean;
  } = $props();

  const uid = $props.id();
  const inputId = `terms-${uid}`;
  const describedBy = $derived([error ? `${inputId}-err` : null, hint ? `${inputId}-hint` : null].filter(Boolean).join(' ') || undefined);

  function commit() {
    const v = draft.trim();
    if (v === '') return;
    terms = [...terms, v];
    draft = '';
  }
  function onkeydown(e: KeyboardEvent) {
    if (e.key === 'Enter' || e.key === ',') {
      e.preventDefault();
      commit();
    } else if (e.key === 'Backspace' && draft === '' && terms.length > 0) {
      terms = terms.slice(0, -1);
    }
  }
  function onpaste(e: ClipboardEvent) {
    const text = e.clipboardData?.getData('text') ?? '';
    if (!text.includes(',') && !text.includes('\n')) return;
    e.preventDefault();
    const parts = text.split(/[,\n]/).map((p) => p.trim()).filter(Boolean);
    terms = [...terms, ...parts];
  }
  function remove(i: number) {
    terms = terms.filter((_, j) => j !== i);
  }
</script>

<div class="flex flex-col gap-1">
  <label for={inputId} class="text-[13px] font-medium">{label}</label>
  <div class="outlined flex min-h-10 flex-wrap items-center gap-1.5 rounded-card bg-surface/80 px-2 py-1.5 {error ? '!border-danger' : ''}">
    {#each terms as term, i (i)}
      <span class="inline-flex items-center gap-1 rounded-full bg-raised py-0.5 pr-1 pl-2.5 text-[13px]">
        {term}
        <button
          type="button"
          {disabled}
          aria-label={$t('searchSynonym.form.removeTerm', { term })}
          onclick={() => remove(i)}
          class="grid size-6 place-items-center rounded-full text-muted hover:bg-active hover:text-fg focus-visible:ring-2 focus-visible:ring-accent-ink focus-visible:outline-none"
        >
          <svg viewBox="0 0 16 16" width="10" height="10" aria-hidden="true" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round"><path d="M3 3l10 10M13 3L3 13" /></svg>
        </button>
      </span>
    {/each}
    <input
      id={inputId}
      type="text"
      bind:value={draft}
      {disabled}
      autocomplete="off"
      {onkeydown}
      {onpaste}
      onblur={commit}
      aria-invalid={error ? 'true' : undefined}
      aria-describedby={describedBy}
      class="min-w-32 flex-1 border-0 bg-transparent px-1.5 py-1 text-[13px] outline-none placeholder:text-muted"
    />
  </div>
  {#if error}<p id="{inputId}-err" role="alert" class="text-sm text-danger">{error}</p>{/if}
  {#if hint}<p id="{inputId}-hint" class="text-sm text-muted">{hint}</p>{/if}
</div>
