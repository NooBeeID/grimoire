<script lang="ts">
  let {
    value = $bindable(''),
    label,
    name,
    options,
    hint,
    error,
    disabled = false,
    onchange,
  }: {
    value?: string;
    label: string;
    name?: string;
    options: { value: string; label: string }[];
    hint?: string;
    error?: string;
    disabled?: boolean;
    onchange?: (value: string) => void;
  } = $props();

  const uid = $props.id();
  const selId = `sel-${uid}`;
  const describedBy = $derived(
    [error ? `${selId}-err` : null, hint ? `${selId}-hint` : null].filter(Boolean).join(' ') || undefined,
  );
</script>

<div class="flex flex-col gap-1">
  <label for={selId} class="text-sm font-medium">{label}</label>
  <select
    id={selId}
    {name}
    {disabled}
    bind:value
    onchange={() => onchange?.(value)}
    aria-invalid={error ? 'true' : undefined}
    aria-describedby={describedBy}
    class="rounded-base border bg-surface px-3 py-2 focus-visible:ring-2 focus-visible:ring-primary focus-visible:outline-none {error ? 'border-danger' : 'border-border'}"
  >
    {#each options as o (o.value)}
      <option value={o.value}>{o.label}</option>
    {/each}
  </select>
  {#if hint}<p id="{selId}-hint" class="text-sm text-muted">{hint}</p>{/if}
  {#if error}<p id="{selId}-err" class="text-sm text-danger">{error}</p>{/if}
</div>
