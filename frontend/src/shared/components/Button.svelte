<script lang="ts">
  import type { Snippet } from 'svelte';
  import Spinner from './Spinner.svelte';

  let {
    variant = 'primary',
    size = 'md',
    type = 'button',
    disabled = false,
    loading = false,
    ariaLabel,
    onclick,
    children,
  }: {
    variant?: 'primary' | 'secondary' | 'danger' | 'ghost';
    size?: 'sm' | 'md';
    type?: 'button' | 'submit' | 'reset';
    disabled?: boolean;
    loading?: boolean;
    ariaLabel?: string;
    onclick?: (e: MouseEvent) => void;
    children?: Snippet;
  } = $props();

  const variants = {
    primary: 'bg-primary text-primary-fg shadow-button hover:opacity-90',
    secondary: 'border border-control bg-surface text-fg shadow-xs hover:bg-raised',
    danger: 'bg-danger text-white shadow-button hover:opacity-90',
    ghost: 'bg-transparent text-fg hover:bg-raised',
  };
  const sizes = { sm: 'h-7 px-2.5 text-[13px]', md: 'h-8 px-3.5 text-sm' };
</script>

<button
  {type}
  disabled={disabled || loading}
  aria-busy={loading}
  aria-label={ariaLabel}
  {onclick}
  class="inline-flex shrink-0 items-center justify-center gap-2 rounded-base font-medium whitespace-nowrap focus-visible:ring-2 focus-visible:ring-accent-ink focus-visible:ring-offset-2 focus-visible:outline-none disabled:cursor-not-allowed disabled:opacity-60 {variants[variant]} {sizes[size]}"
>
  {#if loading}<Spinner size={14} />{/if}
  {@render children?.()}
</button>
