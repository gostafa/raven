<script lang="ts">
  import { Skeleton } from '../../ui';
  let {
    cardsView,
  }: {
    cardsView: boolean;
  } = $props();
</script>

<div class="workspace-library-heading" aria-hidden="true">
  <div class="workspace-skeleton-heading-copy">
    <Skeleton width="80px" height="9px" />
    <Skeleton width="min(330px, 70vw)" height="46px" />
    <Skeleton width="min(400px, 65vw)" height="14px" />
  </div>
  <div class="workspace-heading-actions"><Skeleton width="125px" height="36px" radius="7px" /></div>
</div>
{#if cardsView}<div class="workspace-section-heading" aria-hidden="true">
    <Skeleton width="min(130px, 45%)" height="18px" /><Skeleton width="min(170px, 40%)" height="9px" />
  </div>{/if}
<div class="workspace-folder-controls" aria-hidden="true">
  <div class="workspace-folder-toolbar">
    <Skeleton width="76px" height="32px" radius="6px" />
    <Skeleton width={cardsView ? '115px' : '76px'} height="32px" radius="6px" />
    {#if cardsView}<Skeleton width="100px" height="32px" radius="6px" />{/if}
  </div>
  <Skeleton width="64px" height="10px" />
</div>
<ul class:workspace-memory-grid={cardsView} class:workspace-folder-grid={!cardsView} aria-hidden="true">
  {#each [0, 1, 2, 3, 4, 5] as index (index)}
    <li class="workspace-control workspace-skeleton-tile" class:workspace-skeleton-memory={cardsView}>
      {#if cardsView}
        <div class="workspace-skeleton-progress"><Skeleton width="30px" height="11px" /></div>
        <div class="workspace-skeleton-preview"><Skeleton height="100%" radius="5px" /></div>
      {:else}
        <div class="workspace-skeleton-folder-figure"><Skeleton height="100%" radius="9px" /></div>
        <div class="workspace-skeleton-folder-copy">
          <Skeleton width={index % 2 ? '55%' : '70%'} height="16px" />
          <Skeleton width="90%" height="11px" />
          <Skeleton width="65%" height="11px" />
          <div class="workspace-skeleton-folder-count"><Skeleton width="50px" height="9px" /></div>
        </div>
      {/if}
    </li>
  {/each}
</ul>
