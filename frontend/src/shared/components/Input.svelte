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
  } = $props();

  const uid = $props.id();
  const inputId = `in-${uid}`;
  const describedBy = $derived(
    [error ? `${inputId}-err` : null, hint ? `${inputId}-hint` : null].filter(Boolean).join(' ') || undefined,
  );
</script>

<div class="flex flex-col gap-1">
  <label for={inputId} class="text-sm font-medium">{label}</label>
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
    class="rounded-base border bg-surface px-3 py-2 focus-visible:ring-2 focus-visible:ring-primary focus-visible:outline-none {error ? 'border-danger' : 'border-border'}"
  />
  {#if hint}<p id="{inputId}-hint" class="text-sm text-muted">{hint}</p>{/if}
  {#if error}<p id="{inputId}-err" class="text-sm text-danger">{error}</p>{/if}
</div>
