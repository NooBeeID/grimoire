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
    primary: 'border border-primary bg-primary text-primary-fg shadow-button sticker-press',
    secondary: 'outlined bg-surface/80 text-fg shadow-xs backdrop-blur hover:bg-surface',
    danger: 'border border-danger bg-danger text-on-danger shadow-button sticker-press',
    ghost: 'border border-transparent bg-transparent text-fg hover:bg-raised',
  };
  const sizes = { sm: 'h-8 px-4 text-[13px]', md: 'h-10 px-5 text-sm' };
</script>

<button
  {type}
  disabled={disabled || loading}
  aria-busy={loading}
  aria-label={ariaLabel}
  {onclick}
  class="inline-flex shrink-0 items-center justify-center gap-2 rounded-full font-medium whitespace-nowrap disabled:cursor-not-allowed disabled:opacity-60 disabled:shadow-none {variants[variant]} {sizes[size]}"
>
  {#if loading}<Spinner size={14} />{/if}
  {@render children?.()}
</button>
