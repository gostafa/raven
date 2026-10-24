<script lang="ts">
  import './style.css';
  import WorkspaceSkeleton from './WorkspaceSkeleton.svelte';
</script>

<main class="workspace-app-shell" aria-busy="true">
  <section class="workspace-content">
    <div class="workspace-page-container">
      <WorkspaceSkeleton cardsView={false} />
    </div>
  </section>
</main>
