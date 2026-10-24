<script lang="ts">
  import type { Snippet } from 'svelte';
  import { onMount } from 'svelte';
  import type { MediaURLPort, PreviewStartPort } from '../../cards';
  import type { FolderPaper } from '../domain/types';
  import AnimatedFolder from './AnimatedFolder.svelte';

  let {
    children,
    papers,
    mediaURL,
    nativeStart,
  }: {
    children: Snippet;
    papers: readonly FolderPaper[];
    mediaURL: MediaURLPort;
    nativeStart?: PreviewStartPort;
  } = $props();
  let hovered = $state(false);
  let focused = $state(false);
  let reducedMotion = $state(false);
  const active = $derived(hovered || focused);
  onMount(() => {
    const preference = window.matchMedia('(prefers-reduced-motion: reduce)');
    const update = () => {
      reducedMotion = preference.matches;
    };
    update();
    preference.addEventListener('change', update);
    return () => {
      preference.removeEventListener('change', update);
    };
  });
</script>

<div
  class="folder-interaction"
  class:active
  role="group"
  onpointerenter={(event) => {
    if (event.pointerType !== 'touch') hovered = true;
  }}
  onpointerleave={() => {
    hovered = false;
  }}
  onpointercancel={() => {
    hovered = false;
  }}
  onfocusin={() => {
    focused = true;
  }}
  onfocusout={(event) => {
    if (!(event.relatedTarget instanceof Node) || !event.currentTarget.contains(event.relatedTarget)) focused = false;
  }}
>
  <AnimatedFolder {active} {papers} {mediaURL} {nativeStart} {reducedMotion} />
  <div class="folder-surface" class:active>
    {@render children()}
  </div>
</div>

<style>
  .folder-interaction {
    position: relative;
    height: 100%;
    isolation: isolate;
    perspective: 1000px;
  }
  .folder-interaction.active {
    z-index: 1;
  }
  .folder-surface {
    position: relative;
    z-index: 3;
    height: 100%;
    transform-origin: center bottom;
    transition: transform 300ms ease;
  }
  .folder-surface.active {
    transform: rotateX(-12deg);
  }
  @media (prefers-reduced-motion: reduce) {
    .folder-surface,
    .folder-surface.active {
      transform: none;
      transition: none;
    }
  }
</style>
