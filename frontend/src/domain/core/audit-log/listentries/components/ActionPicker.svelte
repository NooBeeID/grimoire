<script lang="ts">
  import { t } from '../../../../../shared/stores/i18n';
  import { ACTION_GROUPS, AUDIT_ACTIONS, MAX_ACTIONS } from '../types';

  let { value, onchange }: { value: string[]; onchange: (v: string[]) => void } = $props();

  let search = $state('');
  const uid = $props.id();
  const q = $derived(search.trim().toLowerCase());
  const other = $derived(value.filter((a) => !AUDIT_ACTIONS.includes(a)));
  const groups = $derived([
    ...ACTION_GROUPS.map((g) => ({ key: g.key as string, actions: [...g.actions] as string[] })),
    ...(other.length ? [{ key: 'other', actions: other }] : []),
  ]);
  const atLimit = $derived(value.length >= MAX_ACTIONS);

  function toggle(a: string, checked: boolean) {
    onchange(checked ? [...value, a] : value.filter((x) => x !== a));
  }
</script>

<details class="rounded-base border border-control bg-surface">
  <summary class="flex h-8 cursor-pointer items-center px-2.5 text-sm">
    {$t('auditLog.filter.action')}: {$t('auditLog.filter.selectedCount', { n: value.length })}
  </summary>
  <div class="flex flex-col gap-2 p-3">
    <label for="{uid}-search" class="sr-only">{$t('auditLog.filter.searchAction')}</label>
    <input
      id="{uid}-search"
      type="search"
      bind:value={search}
      placeholder={$t('auditLog.filter.searchAction')}
      class="rounded-base border border-border bg-surface px-2 py-1 text-sm"
    />
    {#if atLimit}<p class="text-xs text-muted">{$t('auditLog.filter.actionLimit')}</p>{/if}
    <div class="max-h-64 overflow-auto">
      {#each groups as g (g.key)}
        {@const visible = g.actions.filter((a) => !q || a.includes(q))}
        {#if visible.length}
          <fieldset class="mb-2">
            <legend class="text-xs font-semibold text-muted">{$t(`auditLog.actionGroup.${g.key}`)}</legend>
            {#each visible as a (a)}
              {@const checked = value.includes(a)}
              <label class="flex items-center gap-2 py-0.5 font-mono text-xs">
                <input
                  type="checkbox"
                  {checked}
                  disabled={!checked && atLimit}
                  onchange={(e) => toggle(a, e.currentTarget.checked)}
                />
                {a}
              </label>
            {/each}
          </fieldset>
        {/if}
      {/each}
    </div>
  </div>
</details>
