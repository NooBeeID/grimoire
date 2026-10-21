<script lang="ts">
  let {
    value = $bindable(''),
    label,
    name,
    type = 'text',
    placeholder,
    hint,
    error,
    required = false,
    disabled = false,
    autocomplete,
    hideLabel = false,
  }: {
    value?: string;
    label: string;
    name?: string;
    type?: string;
    placeholder?: string;
    hint?: string;
    error?: string;
    required?: boolean;
    disabled?: boolean;
    autocomplete?: AutoFill;
    hideLabel?: boolean;
  } = $props();

  const uid = $props.id();
  const inputId = `in-${uid}`;
  const describedBy = $derived(
    [error ? `${inputId}-err` : null, hint ? `${inputId}-hint` : null].filter(Boolean).join(' ') || undefined,
  );
</script>

<div class="flex flex-col gap-1">
  <label for={inputId} class={hideLabel ? 'sr-only' : 'text-[13px] font-medium'}>{label}</label>
  <input
    id={inputId}
    {name}
    {type}
    {placeholder}
    {required}
    {disabled}
    {autocomplete}
    bind:value
    aria-invalid={error ? 'true' : undefined}
    aria-describedby={describedBy}
    class="h-8 rounded-base border bg-surface px-3 text-sm shadow-xs focus-visible:ring-2 focus-visible:ring-accent-ink focus-visible:outline-none {error ? 'border-danger' : 'border-control'}"
  />
  {#if hint}<p id="{inputId}-hint" class="text-sm text-muted">{hint}</p>{/if}
  {#if error}<p id="{inputId}-err" class="text-sm text-danger">{error}</p>{/if}
</div>
