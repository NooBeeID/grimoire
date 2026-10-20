<script lang="ts">
  import Button from '../../../../../shared/components/Button.svelte';
  import Input from '../../../../../shared/components/Input.svelte';
  import Select from '../../../../../shared/components/Select.svelte';
  import { t } from '../../../../../shared/stores/i18n';
  import type { Role, StatusFilter, UsersFilters } from '../types';

  let {
    filters,
    hasFilters = false,
    error,
    onchange,
  }: {
    filters: UsersFilters;
    hasFilters?: boolean;
    error?: string;
    onchange: (partial: Partial<UsersFilters>) => void;
  } = $props();

  let q = $state('');
  $effect(() => {
    q = filters.q;
  });

  const roleOptions = $derived([
    { value: '', label: $t('users.filter.all') },
    { value: 'super_admin', label: $t('users.role.super_admin') },
    { value: 'developer', label: $t('users.role.developer') },
  ]);
  const statusOptions = $derived([
    { value: '', label: $t('users.filter.all') },
    { value: 'active', label: $t('users.status.active') },
    { value: 'inactive', label: $t('users.status.inactive') },
  ]);

  function onsearch(e: Event) {
    const v = (e.target as HTMLInputElement).value.slice(0, 100);
    q = v;
    onchange({ q: v });
  }
</script>

<div class="flex flex-wrap items-end gap-3">
  <Select label={$t('users.filter.role')} options={roleOptions} value={filters.role} onchange={(v) => onchange({ role: v as '' | Role })} />
  <Select label={$t('users.filter.status')} options={statusOptions} value={filters.status} onchange={(v) => onchange({ status: v as StatusFilter })} />
  <div class="flex min-w-48 flex-col gap-1" oninput={onsearch} role="presentation">
    <Input bind:value={q} type="search" label={$t('users.filter.search')} {error} />
  </div>
  {#if hasFilters}
    <Button variant="ghost" onclick={() => onchange({ role: '', status: '', q: '' })}>{$t('users.filter.reset')}</Button>
  {/if}
</div>
