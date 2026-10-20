<script lang="ts">
  import Button from '../../../../../shared/components/Button.svelte';
  import { t } from '../../../../../shared/stores/i18n';

  let { value }: { value: string } = $props();

  let copied = $state(false);
  let manual = $state(false);
  let input: HTMLInputElement | undefined = $state();
  let timer: ReturnType<typeof setTimeout> | null = null;
  $effect(() => () => {
    if (timer !== null) clearTimeout(timer);
  });

  async function copy() {
    try {
      if (!navigator.clipboard) throw new Error('no clipboard');
      await navigator.clipboard.writeText(value);
      manual = false;
      copied = true;
      if (timer !== null) clearTimeout(timer);
      timer = setTimeout(() => (copied = false), 2000);
    } catch {
      manual = true;
      input?.focus();
      input?.select();
    }
  }
</script>

<div class="flex flex-col gap-2">
  <p role="note" class="text-sm text-danger">{$t('users.secret.onceWarning')}</p>
  <div class="flex gap-2">
    <input
      bind:this={input}
      readonly
      value={value}
      aria-label={$t('users.secret.title')}
      class="min-w-0 flex-1 rounded-base border border-border bg-surface px-3 py-2 font-mono"
    />
    <Button variant="secondary" onclick={copy}>{copied ? $t('users.secret.copied') : $t('users.secret.copy')}</Button>
  </div>
  {#if manual}<p class="text-sm text-muted">{$t('users.secret.copyManual')}</p>{/if}
</div>
