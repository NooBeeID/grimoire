<script lang="ts">
  import MarkdownView from '../../../../../shared/components/MarkdownView.svelte';
  import { t } from '../../../../../shared/stores/i18n';
  import { safeHref } from '../../../../../shared/utils/url';
  import { asObj, asStr } from '../model/guards';

  // `scopes` terisi bila daftar berasal dari security requirement sebuah operasi.
  let { schemes }: { schemes: Array<{ name: string; scheme: Record<string, unknown> | null; scopes?: string[] }> } = $props();

  const flows = (s: Record<string, unknown>) =>
    Object.entries(asObj(s.flows)).map(([kind, f]) => {
      const o = asObj(f);
      return {
        kind,
        urls: (['authorizationUrl', 'tokenUrl', 'refreshUrl'] as const).flatMap((k): Array<[string, string]> => {
          const u = asStr(o[k]);
          return u === null ? [] : [[k, u]];
        }),
        scopes: Object.entries(asObj(o.scopes)).map(([n, d]) => ({ n, d: asStr(d) })),
      };
    });
</script>

{#snippet url(u: string)}
  {#if safeHref(u)}<a class="text-accent-ink underline-offset-2 hover:underline" href={safeHref(u)} rel="noopener noreferrer" target="_blank">{u}</a>{:else}{u}{/if}
{/snippet}

<ul class="mt-1 flex flex-col gap-3">
  {#each schemes as { name, scheme, scopes } (name)}
    <li class="text-sm">
      <span class="font-mono text-[13px] font-medium">{name}</span>
      {#if scheme === null}
        <span class="text-xs text-muted"> · {$t('apispec.doc.securityUnknown')}</span>
      {:else}
        <span class="text-xs text-muted">
          · {asStr(scheme.type) ?? '?'}{#if asStr(scheme.scheme)} · {asStr(scheme.scheme)}{/if}{#if asStr(scheme.bearerFormat)} ({asStr(scheme.bearerFormat)}){/if}{#if asStr(scheme.type) === 'apiKey'} · {asStr(scheme.in) ?? ''} {asStr(scheme.name) ?? ''}{/if}
        </span>
        {#if asStr(scheme.description)}<div class="text-xs text-muted"><MarkdownView source={asStr(scheme.description)!} /></div>{/if}
        {@const oidc = asStr(scheme.openIdConnectUrl)}
        {#if oidc}<p class="font-mono text-xs break-all">{@render url(oidc)}</p>{/if}
        {#each flows(scheme) as f (f.kind)}
          <div class="mt-1 pl-3 text-xs">
            <span class="font-mono">{f.kind}</span>
            {#each f.urls as [k, u] (k)}<p class="font-mono break-all">{k}: {@render url(u)}</p>{/each}
            {#if f.scopes.length > 0}
              <ul class="text-muted">{#each f.scopes as s (s.n)}<li><code class="font-mono">{s.n}</code>{s.d ? ` · ${s.d}` : ''}</li>{/each}</ul>
            {/if}
          </div>
        {/each}
      {/if}
      {#if scopes && scopes.length > 0}
        <p class="text-xs text-muted">{$t('apispec.doc.scopes')}: <code class="font-mono">{scopes.join(', ')}</code></p>
      {/if}
    </li>
  {/each}
</ul>
