<script lang="ts">
  import type { Snippet } from 'svelte';
  import Spinner from './Spinner.svelte';

  let {
    variant = 'primary',
    size = 'md',
    type = 'button',
    disabled = false,
    loading = false,
    onclick,
    children,
  }: {
    variant?: 'primary' | 'secondary' | 'danger' | 'ghost';
    size?: 'sm' | 'md';
    type?: 'button' | 'submit' | 'reset';
    disabled?: boolean;
    loading?: boolean;
    onclick?: (e: MouseEvent) => void;
    children?: Snippet;
  } = $props();

  const variants = {
    primary: 'bg-primary text-primary-fg hover:opacity-90',
    secondary: 'border border-border bg-surface text-fg hover:bg-black/5',
    danger: 'bg-danger text-white hover:opacity-90',
    ghost: 'bg-transparent text-fg hover:bg-black/5',
  };
  const sizes = { sm: 'px-2.5 py-1 text-sm', md: 'px-4 py-2 text-base' };
</script>

<button
  {type}
  disabled={disabled || loading}
  aria-busy={loading}
  {onclick}
  class="inline-flex items-center justify-center gap-2 rounded-base font-medium focus-visible:ring-2 focus-visible:ring-primary focus-visible:ring-offset-2 focus-visible:outline-none disabled:cursor-not-allowed disabled:opacity-60 {variants[variant]} {sizes[size]}"
>
  {#if loading}<Spinner size={14} />{/if}
  {@render children?.()}
</button>
