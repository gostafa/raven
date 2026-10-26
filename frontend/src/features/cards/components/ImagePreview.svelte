<script lang="ts">
  import { onMount } from 'svelte';
  import { useLocalization } from '../../localization';
  import { Skeleton } from '../../ui';
  import { loadMedia } from './viewport/funcs';

  const l = useLocalization();
  let {
    src,
    filename,
    compact = false,
    expanded = false,
    active = false,
  }: {
    src: string;
    filename?: string;
    compact?: boolean;
    expanded?: boolean;
    active?: boolean;
  } = $props();
  let resolved = $state<string>();
  let image = $state<HTMLImageElement>();
  let ready = $state(false);
  let failed = $state(false);

  $effect(() => {
    ready = false;
    failed = src.length === 0;
  });

  // CardContent keys this component by source so replacement media starts fresh.
  onMount(() => {
    if (image?.complete === true && image.getAttribute('src') !== null) {
      ready = image.naturalWidth > 0;
      failed = !ready;
    }
  });
</script>

<div
  class="image-preview"
  class:compact
  class:expanded
  class:loading={!ready && !failed}
  aria-busy={!ready && !failed}
  use:loadMedia={{
    source: src,
    deferred: compact,
    active,
    resolve: (source) => {
      resolved = source;
    },
  }}
>
  {#if failed}
    <p class="media-error" role="status">{$l.t('cards.mediaFailed', { name: filename ?? '' })}</p>
  {:else}
    <img
      bind:this={image}
      src={resolved}
      decoding="async"
      alt={(filename ?? '') || $l.t('cards.card_image')}
      class:pending={!ready}
      onload={() => {
        ready = true;
      }}
      onerror={() => {
        failed = true;
      }}
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
  .expanded {
    width: max-content;
    max-width: 100%;
    height: auto;
  }
  .expanded.loading {
    min-width: min(320px, calc(100vw - 26px));
    min-height: min(180px, calc(100svh - 26px));
  }
  .expanded img {
    width: auto;
    max-width: 100%;
    height: auto;
    max-height: calc(100svh - 26px);
    border-radius: 0;
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
