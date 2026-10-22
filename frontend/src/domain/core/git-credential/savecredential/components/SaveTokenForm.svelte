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
    hostLocked = false,
    gitlabHosts,
    disabled = false,
    errors = {},
    onsubmit,
  }: {
    provider?: GitProvider;
    host?: string;
    token?: string;
    providerLocked?: boolean;
    hostLocked?: boolean;
    gitlabHosts: string[];
    disabled?: boolean;
    errors?: { token?: string; host?: string };
    onsubmit: () => void;
  } = $props();

  const providers = $derived([
    { value: 'github', label: $t('gitCredential.provider.github') },
    { value: 'gitlab', label: $t('gitCredential.provider.gitlab') },
  ]);
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
    <Input
      bind:value={host}
      name="git-host"
      autocomplete="off"
      label={$t('gitCredential.save.host')}
      placeholder={gitlabHosts[0] ?? 'gitlab.com'}
      hint={$t('gitCredential.save.hostHint')}
      error={errors.host}
      disabled={disabled || hostLocked}
    />
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
