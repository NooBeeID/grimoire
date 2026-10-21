<script lang="ts">
  import { t } from '../../../../../shared/stores/i18n';
  import type { CandidateState } from '../stores/candidateSearch.store';
  import type { Candidate } from '../types';

  let {
    search,
    selected = null,
    oninput,
    onmove,
    onselect,
    onclear,
  }: {
    search: CandidateState;
    selected?: Candidate | null;
    oninput: (q: string) => void;
    onmove: (delta: number) => void;
    onselect: (c: Candidate) => void;
    onclear: () => void;
  } = $props();

  const uid = $props.id();
  const inputId = `cb-${uid}`;
  const listId = `cb-list-${uid}`;
  let text = $state('');
  let focused = $state(false);
  let dismissed = $state(false);

  const hasQuery = $derived(search.q.trim() !== '');
  const open = $derived(focused && !dismissed && !selected && hasQuery);
  const optionId = (i: number) => `${listId}-opt-${i}`;
  const active = $derived(search.activeIndex >= 0 ? optionId(search.activeIndex) : undefined);
  const announce = $derived(
    search.status === 'ready' && search.items.length > 0 ? $t('project.members.add.resultCount', { n: search.items.length }) : '',
  );

  function input(e: Event) {
    text = (e.currentTarget as HTMLInputElement).value;
    dismissed = false;
    oninput(text);
  }
  function choose(c: Candidate) {
    text = '';
    dismissed = false;
    onselect(c);
  }
  function keydown(e: KeyboardEvent) {
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      dismissed = false;
      onmove(1);
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      dismissed = false;
      onmove(-1);
    } else if (e.key === 'Enter') {
      const c = open && search.activeIndex >= 0 ? search.items[search.activeIndex] : undefined;
      if (c) {
        e.preventDefault();
        choose(c);
      }
    } else if (e.key === 'Escape' && open) {
      // Escape pertama hanya menutup daftar; Escape kedua diteruskan ke Dialog.
      e.preventDefault();
      e.stopPropagation();
      dismissed = true;
    }
  }
  function clear() {
    text = '';
    onclear();
  }
</script>

<div class="relative flex flex-col gap-1">
  <label for={inputId} class="text-sm font-medium">{$t('project.members.add.search')}</label>
  {#if selected}
    <div class="flex items-center justify-between gap-2 rounded-base border border-border bg-surface px-3 py-2">
      <span class="min-w-0 break-words" data-testid="selected-candidate">{selected.name} <span class="text-muted">{selected.email}</span></span>
      <button type="button" onclick={clear} class="rounded-base px-2 py-1 text-sm underline focus-visible:ring-2 focus-visible:ring-accent-ink focus-visible:outline-none">
        {$t('project.members.add.clear')}
      </button>
    </div>
  {:else}
    <input
      id={inputId}
      type="text"
      role="combobox"
      autocomplete="off"
      aria-autocomplete="list"
      aria-expanded={open}
      aria-controls={listId}
      aria-activedescendant={open ? active : undefined}
      value={text}
      oninput={input}
      onkeydown={keydown}
      onfocus={() => (focused = true)}
      onblur={() => (focused = false)}
      class="rounded-base border border-border bg-surface px-3 py-2 focus-visible:ring-2 focus-visible:ring-accent-ink focus-visible:outline-none"
    />
  {/if}
  <ul
    id={listId}
    role="listbox"
    aria-label={$t('project.members.add.search')}
    hidden={!open}
    class="absolute top-full z-10 mt-1 max-h-60 w-full overflow-auto rounded-base border border-border bg-surface shadow" data-ui="overlay"
  >
    {#if open}
      {#if search.status === 'loading'}
        <li role="presentation" class="px-3 py-2 text-sm text-muted">{$t('project.members.add.searching')}</li>
      {:else if search.status === 'error'}
        <li role="presentation" class="px-3 py-2 text-sm text-danger">{$t('project.members.add.searchError')}</li>
      {:else if search.status === 'ready' && search.items.length === 0}
        <li role="presentation" class="px-3 py-2 text-sm text-muted">{$t('project.members.add.noCandidates')}</li>
      {:else}
        {#each search.items as c, i (c.id)}
          <!-- svelte-ignore a11y_click_events_have_key_events -->
          <li
            id={optionId(i)}
            role="option"
            aria-selected={i === search.activeIndex}
            onmousedown={(e) => {
              e.preventDefault();
              choose(c);
            }}
            class="cursor-pointer px-3 py-2 text-sm {i === search.activeIndex ? 'bg-active' : 'hover:bg-raised'}"
          >
            <span class="break-words">{c.name}</span> <span class="break-all text-muted">{c.email}</span>
          </li>
        {/each}
      {/if}
    {/if}
  </ul>
  <div class="sr-only" aria-live="polite">{announce}</div>
</div>
