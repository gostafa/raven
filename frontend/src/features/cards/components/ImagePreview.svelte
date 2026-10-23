<script lang="ts">
  import { onMount } from 'svelte';
  import { useLocalization } from '../../localization';
  import { Skeleton } from '../../ui';

  const l = useLocalization();
  let { src, filename, compact = false }: { src: string; filename?: string; compact?: boolean } = $props();
  let image = $state<HTMLImageElement>();
  let ready = $state(false);
  let failed = $state(false);

  // CardContent keys this component by source so replacement media starts fresh.
  onMount(() => {
    if (image?.complete) {
      ready = image.naturalWidth > 0;
      failed = !ready;
    }
  });
</script>

<div class="image-preview" class:compact aria-busy={!ready && !failed}>
  {#if failed}
    <p class="media-error" role="status">{$l.t('cards.mediaFailed', { name: filename ?? '' })}</p>
  {:else}
    <img
      bind:this={image}
      {src}
      alt={filename || $l.t('cards.card_image')}
      class:pending={!ready}
      onload={() => (ready = true)}
      onerror={() => (failed = true)}
    />
    {#if !ready}<div class="placeholder"><Skeleton height="100%" radius="5px" /></div>{/if}
  {/if}
</div>

<style>
  .image-preview {
    position: relative;
    width: 100%;
    height: 200px;
    min-height: 0;
    background: #000;
  }
  .compact {
    height: 100%;
  }
  img {
    display: block;
    width: 100%;
    height: 100%;
    object-fit: contain;
    border-radius: 5px;
  }
  .compact img {
    object-fit: cover;
  }
  .pending {
    visibility: hidden;
  }
  .placeholder {
    position: absolute;
    inset: 0;
    pointer-events: none;
  }
  .media-error {
    margin: 0;
    padding: 4px;
    color: var(--ink);
    font-size: 12px;
    line-height: 1.8;
    overflow-wrap: anywhere;
  }
</style>
