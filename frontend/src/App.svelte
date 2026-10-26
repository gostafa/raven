<script lang="ts">
  import { provideLocalization, type Localization } from './features/localization';
  import { onDestroy, untrack } from 'svelte';
  import { Workspace, type AppPorts } from './features/workspace';
  import { Widget } from './features/companion';
  import { FloatingReview } from './features/reviews';
  let { backend, localization }: { backend: AppPorts; localization: Localization } = $props();
  provideLocalization(untrack(() => localization));
  onDestroy(() => {
    localization.dispose();
  });
  const view = new URLSearchParams(window.location.search).get('view');
</script>

{#if view === 'widget'}<Widget {backend} />{:else if view === 'review'}<FloatingReview {backend} />{:else}<Workspace
    {backend}
  />{/if}
