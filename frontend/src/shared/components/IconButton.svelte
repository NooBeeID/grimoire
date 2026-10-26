<script lang="ts">
  import Icon, { type IconName } from './Icon.svelte';
  import Spinner from './Spinner.svelte';

  let {
    icon,
    label,
    ariaLabel,
    tone = 'default',
    disabled = false,
    loading = false,
    onclick,
  }: {
    icon: IconName;
    label: string;
    ariaLabel?: string;
    tone?: 'default' | 'danger';
    disabled?: boolean;
    loading?: boolean;
    onclick?: (e: MouseEvent) => void;
  } = $props();
</script>

<button
  type="button"
  title={label}
  aria-label={ariaLabel}
  aria-busy={loading}
  disabled={disabled || loading}
  {onclick}
  class="inline-flex h-8 w-8 shrink-0 items-center justify-center rounded-full border border-transparent bg-transparent hover:bg-raised disabled:cursor-not-allowed disabled:opacity-50 {tone === 'danger' ? 'text-danger' : 'text-muted hover:text-fg'}"
>
  {#if loading}<Spinner size={14} />{:else}<Icon name={icon} />{/if}
  <span class="sr-only">{label}</span>
</button>
