<script lang="ts">
  import type { Kind } from '../../base';
  import type { FormTouched, FormValues } from '../types';
  import SyncForm from './SyncForm.svelte';

  // Pemegang $state agar bind: di SyncForm reaktif saat diuji sendirian.
  let { kind, errors = {} }: { kind: Kind; errors?: { url?: string; branch?: string; folder?: string; target_prefix?: string } } = $props();
  let values = $state<FormValues>({ url: '', branch: '', folder: '', targetPrefix: '' });
  let touched = $state<FormTouched>({ branch: false, folder: false });
</script>

<SyncForm {kind} bind:values bind:touched {errors} />
