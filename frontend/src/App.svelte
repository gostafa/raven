<script lang="ts">
  import { Workspace, type AppPorts } from './features/workspace';
  import { Widget } from './features/companion';
  import { FloatingReview } from './features/reviews';
  let { backend }: { backend: AppPorts } = $props();
  const view = new URLSearchParams(window.location.search).get('view');
</script>

{#if view === 'widget'}<Widget {backend} />{:else if view === 'review'}<FloatingReview {backend} />{:else}<Workspace
    {backend}
  />{/if}

<style>
  :global(*) {
    outline: none;
  }
</style>
