<script lang="ts">
  import Input from '../../../../../shared/components/Input.svelte';
  import Select from '../../../../../shared/components/Select.svelte';
  import { t } from '../../../../../shared/stores/i18n';
  import type { GitProvider } from '../types';
  import ScopeNote from './ScopeNote.svelte';

  let {
    provider = $bindable<GitProvider>('github'),
    host = $bindable(''),
    token = $bindable(''),
    providerLocked = false,
    gitlabHosts,
    disabled = false,
    errors = {},
    onsubmit,
  }: {
    provider?: GitProvider;
    host?: string;
    token?: string;
    providerLocked?: boolean;
    gitlabHosts: string[];
    disabled?: boolean;
    errors?: { token?: string; host?: string };
    onsubmit: () => void;
  } = $props();

  const providers = $derived([
    { value: 'github', label: $t('gitCredential.provider.github') },
    { value: 'gitlab', label: $t('gitCredential.provider.gitlab') },
  ]);
  const hostOptions = $derived(gitlabHosts.map((h) => ({ value: h, label: h })));
</script>

<form
  class="flex flex-col gap-4"
  autocomplete="off"
  onsubmit={(e) => {
    e.preventDefault();
    onsubmit();
  }}
>
  <Select bind:value={provider} label={$t('gitCredential.save.provider')} options={providers} disabled={disabled || providerLocked} />
  {#if provider === 'gitlab'}
    <Select bind:value={host} label={$t('gitCredential.save.host')} options={hostOptions} {disabled} error={errors.host} />
  {/if}
  <Input
    bind:value={token}
    type="password"
    name="git-token"
    autocomplete="off"
    label={$t('gitCredential.save.token')}
    placeholder={$t('gitCredential.save.tokenPlaceholder')}
    hint={$t('gitCredential.noReveal')}
    error={errors.token}
    {disabled}
  />
  <ScopeNote {provider} />
</form>
