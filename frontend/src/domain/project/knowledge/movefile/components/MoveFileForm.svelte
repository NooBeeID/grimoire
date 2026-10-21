<script lang="ts">
  import { tick } from 'svelte';
  import Input from '../../../../../shared/components/Input.svelte';
  import { t } from '../../../../../shared/stores/i18n';
  import { stemRange } from './stemRange';

  let {
    from,
    to = $bindable(''),
    error,
    active,
    onsubmit,
  }: {
    from: string;
    to?: string;
    error?: string;
    // true saat dialog terbuka: fokus ke input dan pilih nama file tanpa ekstensi.
    active: boolean;
    onsubmit: (e: Event) => void;
  } = $props();

  let root: HTMLElement | undefined = $state();

  $effect(() => {
    if (!active) return;
    void tick().then(() => {
      const input = root?.querySelector('input');
      if (!input) return;
      input.focus();
      const [a, b] = stemRange(from);
      input.setSelectionRange(a, b);
    });
  });
</script>

<form bind:this={root} id="move-file-form" {onsubmit} novalidate class="flex flex-col gap-3">
  <p class="font-mono text-[13px] break-all text-muted">{from}</p>
  <Input bind:value={to} label={$t('knowledge.move.to')} {error} />
</form>
