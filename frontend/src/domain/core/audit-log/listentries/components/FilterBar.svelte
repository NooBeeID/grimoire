<script lang="ts">
  import { untrack } from 'svelte';
  import Button from '../../../../../shared/components/Button.svelte';
  import { locale, t } from '../../../../../shared/stores/i18n';
  import { validateFilter } from '../stores/filterQuery';
  import type { Suggestions } from '../stores/listentries.store';
  import {
    ACTOR_TYPES,
    AUDIT_STATUSES,
    EMPTY_FILTER,
    MAX_TEXT,
    RANGE_PRESETS,
    UUID_RE,
    type ActorType,
    type AuditFilter,
    type AuditStatus,
    type RangeKey,
  } from '../types';
  import ActionPicker from './ActionPicker.svelte';
  import { formatDateTime } from './format';

  let {
    value,
    suggestions,
    errors = {},
    onapply,
    onreset,
    disabled = false,
  }: {
    value: AuditFilter;
    suggestions: Suggestions;
    errors?: Partial<Record<keyof AuditFilter, string>>;
    onapply: (f: AuditFilter) => void;
    onreset: () => void;
    disabled?: boolean;
  } = $props();

  const uid = $props.id();
  const clone = (f: AuditFilter): AuditFilter => ({ ...f, actions: [...f.actions], statuses: [...f.statuses] });
  let draft = $state<AuditFilter>(clone(EMPTY_FILTER));
  let projectText = $state('');
  let localErrors = $state<Partial<Record<keyof AuditFilter, string>>>({});

  function projectLabelFor(id: string): string {
    return suggestions.projects.find((p) => p.projectId === id)?.projectLabel ?? id;
  }

  // Sinkron ulang draft saat filter dari URL berubah.
  $effect(() => {
    const v = value;
    untrack(() => {
      draft = clone(v);
      projectText = v.projectId ? projectLabelFor(v.projectId) : '';
      localErrors = {};
    });
  });

  const allErrors = $derived({ ...errors, ...localErrors });
  const err = (k: keyof AuditFilter) => (allErrors[k] ? $t(allErrors[k] as string) : undefined);

  function onActorInput() {
    const s = suggestions.actors.find((a) => a.actorId && a.actorLabel === draft.actorLabel);
    if (s && s.actorId) {
      draft.actorType = s.actorType;
      draft.actorId = s.actorId;
    } else if (draft.actorId) {
      draft.actorId = '';
    }
  }

  function resolveProject(): boolean {
    const text = projectText.trim();
    if (!text) {
      draft.projectId = '';
      return true;
    }
    const byLabel = suggestions.projects.find((p) => p.projectLabel === text);
    if (byLabel) {
      draft.projectId = byLabel.projectId;
      return true;
    }
    if (UUID_RE.test(text)) {
      draft.projectId = text;
      return true;
    }
    return false;
  }

  function toggleStatus(s: AuditStatus, on: boolean) {
    draft.statuses = on ? [...draft.statuses, s] : draft.statuses.filter((x) => x !== s);
  }

  function submit(e: SubmitEvent) {
    e.preventDefault();
    const errs: Partial<Record<keyof AuditFilter, string>> = {};
    if (!resolveProject()) errs.projectId = 'auditLog.filter.projectInvalid';
    Object.assign(errs, validateFilter(draft), errs.projectId ? { projectId: errs.projectId } : {});
    localErrors = errs;
    if (Object.keys(errs).length) return;
    onapply(clone(draft));
  }

  const inputCls = 'h-8 w-full rounded-base border bg-surface px-2.5 text-sm';
  const customLabel = $derived(
    value.range === 'custom'
      ? $t('auditLog.range.custom', {
          from: value.from ? formatDateTime(value.from, $locale) : '…',
          to: value.to ? formatDateTime(value.to, $locale) : '…',
        })
      : '',
  );
</script>

<form
  role="search"
  aria-label={$t('auditLog.filter.label')}
  onsubmit={submit}
  class="grid grid-cols-1 gap-x-4 gap-y-3 rounded-card border border-border bg-raised p-4 sm:grid-cols-2 lg:grid-cols-4"
>
  <div class="flex flex-col gap-1">
    <label for="{uid}-atype" class="text-[13px] font-medium">{$t('auditLog.filter.actorType')}</label>
    <select
      id="{uid}-atype"
      class="{inputCls} border-control"
      value={draft.actorType}
      onchange={(e) => {
        draft.actorType = e.currentTarget.value as ActorType | '';
        if (!draft.actorType) draft.actorId = '';
      }}
    >
      <option value="">{$t('auditLog.filter.any')}</option>
      {#each ACTOR_TYPES as a (a)}<option value={a}>{$t(`auditLog.actorType.${a}`)}</option>{/each}
    </select>
  </div>

  <div class="flex flex-col gap-1">
    <label for="{uid}-actor" class="text-[13px] font-medium">{$t('auditLog.filter.actor')}</label>
    <input
      id="{uid}-actor"
      class="{inputCls} {err('actorLabel') ? 'border-danger' : 'border-control'}"
      list="{uid}-actors"
      maxlength={MAX_TEXT}
      bind:value={draft.actorLabel}
      oninput={onActorInput}
      aria-invalid={err('actorLabel') ? 'true' : undefined}
    />
    <datalist id="{uid}-actors">
      {#each suggestions.actors as a (`${a.actorType}|${a.actorId}|${a.actorLabel}`)}
        <option value={a.actorLabel}></option>
      {/each}
    </datalist>
    {#if draft.actorId}
      <span class="inline-flex w-fit items-center gap-1 rounded-base border border-border px-2 py-0.5 text-xs">
        {$t('auditLog.filter.actorIdChip', { id: draft.actorId.slice(0, 8) })}
        <button
          type="button"
          aria-label={$t('auditLog.filter.removeActorId')}
          onclick={() => (draft.actorId = '')}><span aria-hidden="true">×</span></button
        >
      </span>
    {/if}
    {#if err('actorLabel')}<p class="text-sm text-danger">{err('actorLabel')}</p>{/if}
    {#if err('actorId')}<p class="text-sm text-danger">{err('actorId')}</p>{/if}
  </div>

  <div class="flex flex-col gap-1">
    <label for="{uid}-project" class="text-[13px] font-medium">{$t('auditLog.filter.project')}</label>
    <input
      id="{uid}-project"
      class="{inputCls} {err('projectId') ? 'border-danger' : 'border-control'}"
      list="{uid}-projects"
      bind:value={projectText}
      aria-invalid={err('projectId') ? 'true' : undefined}
      aria-describedby={err('projectId') ? `${uid}-project-err` : undefined}
    />
    <datalist id="{uid}-projects">
      {#each suggestions.projects as p (p.projectId)}
        {#if p.projectLabel}<option value={p.projectLabel}></option>{/if}
      {/each}
    </datalist>
    {#if err('projectId')}<p id="{uid}-project-err" class="text-sm text-danger">{err('projectId')}</p>{/if}
  </div>

  <div class="flex flex-col gap-1">
    <span class="text-[13px] font-medium">{$t('auditLog.filter.action')}</span>
    <ActionPicker value={draft.actions} onchange={(v) => (draft.actions = v)} />
    {#if err('actions')}<p class="text-sm text-danger">{err('actions')}</p>{/if}
  </div>

  <fieldset class="flex flex-col gap-1">
    <legend class="text-[13px] font-medium">{$t('auditLog.filter.status')}</legend>
    <div class="flex flex-wrap gap-3">
      {#each AUDIT_STATUSES as s (s)}
        <label class="flex items-center gap-1 text-sm">
          <input
            type="checkbox"
            checked={draft.statuses.includes(s)}
            onchange={(e) => toggleStatus(s, e.currentTarget.checked)}
          />
          {$t(`auditLog.status.${s}`)}
        </label>
      {/each}
    </div>
  </fieldset>

  <div class="flex flex-col gap-1">
    <label for="{uid}-range" class="text-[13px] font-medium">{$t('auditLog.filter.range')}</label>
    <select
      id="{uid}-range"
      class="{inputCls} {err('range') ? 'border-danger' : 'border-control'}"
      value={draft.range}
      onchange={(e) => (draft.range = e.currentTarget.value as RangeKey)}
    >
      {#each RANGE_PRESETS as p (p.key)}<option value={p.key}>{$t(`auditLog.range.${p.key}`)}</option>{/each}
      <option value="all">{$t('auditLog.range.all')}</option>
      {#if value.range === 'custom'}<option value="custom" disabled>{customLabel}</option>{/if}
    </select>
    {#if err('range')}<p class="text-sm text-danger">{err('range')}</p>{/if}
  </div>

  <div class="flex flex-col gap-1">
    <label for="{uid}-target" class="text-[13px] font-medium">{$t('auditLog.filter.target')}</label>
    <input
      id="{uid}-target"
      class="{inputCls} {err('target') ? 'border-danger' : 'border-control'}"
      maxlength={MAX_TEXT}
      bind:value={draft.target}
      aria-invalid={err('target') ? 'true' : undefined}
    />
    {#if err('target')}<p class="text-sm text-danger">{err('target')}</p>{/if}
  </div>

  <div class="flex items-end gap-2">
    <Button type="submit" {disabled}>{$t('auditLog.filter.apply')}</Button>
    <Button variant="secondary" {disabled} onclick={onreset}>{$t('auditLog.filter.reset')}</Button>
  </div>
</form>
