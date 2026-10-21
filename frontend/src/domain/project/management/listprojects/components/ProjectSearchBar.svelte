<script lang="ts">
  import { untrack } from 'svelte';
  import Button from '../../../../../shared/components/Button.svelte';
  import Input from '../../../../../shared/components/Input.svelte';
  import { t } from '../../../../../shared/stores/i18n';

  let { value, onsearch }: { value: string; onsearch: (q: string) => void } = $props();

  let text = $state(untrack(() => value));
  let timer: ReturnType<typeof setTimeout> | null = null;

  // Ikuti perubahan dari luar (mis. reset) tanpa memicu pencarian ulang.
  $effect(() => {
    const v = value;
    untrack(() => {
      if (v !== text.trim()) text = v;
    });
  });
  $effect(() => () => {
    if (timer) clearTimeout(timer);
  });

  function schedule() {
    if (timer) clearTimeout(timer);
    timer = setTimeout(() => {
      timer = null;
      if (text.trim() !== value) onsearch(text.trim());
    }, 300);
  }
  function submit(e: SubmitEvent) {
    e.preventDefault();
    if (timer) clearTimeout(timer);
    timer = null;
    onsearch(text.trim());
  }
  function clear() {
    if (timer) clearTimeout(timer);
    timer = null;
    text = '';
    onsearch('');
  }
</script>

<form role="search" onsubmit={submit} class="flex items-center gap-2">
  <div class="flex-1" oninput={schedule} role="presentation">
    <Input bind:value={text} type="search" label={$t('project.list.search')} placeholder={$t('project.list.search')} hideLabel name="q" />
  </div>
  {#if text}
    <Button variant="ghost" onclick={clear}>
      <span aria-hidden="true">&times;</span>
      <span class="sr-only">{$t('project.list.clearSearch')}</span>
    </Button>
  {/if}
</form>
