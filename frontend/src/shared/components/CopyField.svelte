<script lang="ts">
  import { onDestroy } from 'svelte';
  import { t } from '../stores/i18n';
  import { copyText } from '../utils/clipboard';
  import Button from './Button.svelte';

  let {
    value,
    label,
    multiline = false,
    rows = 3,
    testid,
  }: { value: string; label: string; multiline?: boolean; rows?: number; testid?: string } = $props();

  const uid = $props.id();
  const fieldId = `cf-${uid}`;
  let field: HTMLInputElement | HTMLTextAreaElement | undefined = $state();
  let copied = $state(false);
  let manual = $state(false);
  let timer: ReturnType<typeof setTimeout> | null = null;

  async function copy() {
    const ok = await copyText(value);
    if (timer) clearTimeout(timer);
    if (ok) {
      manual = false;
      copied = true;
      timer = setTimeout(() => (copied = false), 2000);
    } else {
      copied = false;
      manual = true;
      field?.focus();
      field?.select();
    }
  }
  onDestroy(() => {
    if (timer) clearTimeout(timer);
  });

  const fieldClass =
    'w-full rounded-base border border-control bg-raised px-3 font-mono text-[13px] focus-visible:ring-2 focus-visible:ring-accent-ink focus-visible:outline-none'; // taste-skill-ignore: isi field adalah key dan konfigurasi (kode literal)
</script>

<div class="flex flex-col gap-1">
  <label for={fieldId} class="text-[13px] font-medium">{label}</label>
  <div class="flex items-start gap-2">
    {#if multiline}
      <textarea
        bind:this={field}
        id={fieldId}
        readonly
        {rows}
        {value}
        spellcheck="false"
        autocomplete="off"
        data-testid={testid}
        class="{fieldClass} resize-none py-2"
      ></textarea>
    {:else}
      <input
        bind:this={field}
        id={fieldId}
        readonly
        {value}
        spellcheck="false"
        autocomplete="off"
        data-testid={testid}
        class="{fieldClass} h-8"
      />
    {/if}
    <Button variant="secondary" size="sm" onclick={copy}>{copied ? $t('common.copied') : $t('common.copy')}</Button>
  </div>
  <span role="status" aria-live="polite" class="text-xs text-muted">{manual ? $t('common.copyManual') : ''}</span>
</div>
