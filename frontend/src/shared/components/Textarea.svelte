<script lang="ts">
  let {
    value = $bindable(''),
    label,
    name,
    hint,
    error,
    required = false,
    disabled = false,
    rows = 4,
    maxlength,
  }: {
    value?: string;
    label: string;
    name?: string;
    hint?: string;
    error?: string;
    required?: boolean;
    disabled?: boolean;
    rows?: number;
    maxlength?: number;
  } = $props();

  const uid = $props.id();
  const areaId = `ta-${uid}`;
  const describedBy = $derived(
    [error ? `${areaId}-err` : null, hint ? `${areaId}-hint` : null].filter(Boolean).join(' ') || undefined,
  );
</script>

<div class="flex flex-col gap-1">
  <label for={areaId} class="text-[13px] font-medium">{label}</label>
  <textarea
    id={areaId}
    {name}
    {rows}
    {maxlength}
    {required}
    {disabled}
    bind:value
    aria-invalid={error ? 'true' : undefined}
    aria-describedby={describedBy}
    class="rounded-base border bg-surface px-3 py-2 shadow-xs focus-visible:ring-2 focus-visible:ring-accent-ink focus-visible:outline-none {error ? 'border-danger' : 'border-control'}"
  ></textarea>
  {#if hint}<p id="{areaId}-hint" class="text-sm text-muted">{hint}</p>{/if}
  {#if error}<p id="{areaId}-err" class="text-sm text-danger">{error}</p>{/if}
</div>
