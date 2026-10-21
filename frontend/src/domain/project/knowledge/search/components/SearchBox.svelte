<script lang="ts">
  import { onMount } from 'svelte';
  import { t } from '../../../../../shared/stores/i18n';

  let { value = '', onchange }: { value?: string; onchange: (q: string) => void } = $props();
  let text = $state(untrackedValue());
  let timer: ReturnType<typeof setTimeout> | undefined;
  let el: HTMLInputElement | undefined = $state();

  function untrackedValue() {
    return value;
  }
  // Sinkron dari luar (mis. Back) tanpa menimpa ketikan yang sedang diketik.
  $effect(() => {
    if (value !== text && document.activeElement !== el) text = value;
  });

  function emit(v: string, immediate = false) {
    clearTimeout(timer);
    if (immediate) onchange(v);
    else timer = setTimeout(() => onchange(v), 300);
  }

  onMount(() => {
    const onkey = (e: KeyboardEvent) => {
      const a = document.activeElement;
      const typing = a instanceof HTMLInputElement || a instanceof HTMLTextAreaElement || (a as HTMLElement | null)?.isContentEditable;
      if (e.key === '/' && !typing && !e.metaKey && !e.ctrlKey) {
        e.preventDefault();
        el?.focus();
      }
    };
    window.addEventListener('keydown', onkey);
    return () => {
      window.removeEventListener('keydown', onkey);
      clearTimeout(timer);
    };
  });
</script>

<div class="relative">
  <label for="knowledge-search" class="sr-only">{$t('knowledge.search.label')}</label>
  <input
    id="knowledge-search"
    bind:this={el}
    type="search"
    bind:value={text}
    placeholder={$t('knowledge.search.placeholder')}
    oninput={() => emit(text)}
    onkeydown={(e) => {
      if (e.key === 'Escape' && text) {
        text = '';
        emit('', true);
      }
    }}
    class="h-8 w-full rounded-base border border-control bg-surface px-3 text-sm focus-visible:ring-2 focus-visible:ring-accent-ink focus-visible:outline-none"
  />
</div>
