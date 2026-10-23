<script lang="ts">
  import { onMount } from 'svelte';
  import type { FolderPaper } from '../domain/types';
  import { mountFolderFigure, prepareFolderStage } from '../adapters/hairline/funcs';

  let { papers, active = false }: { papers: readonly FolderPaper[]; active?: boolean } = $props();
  let host: HTMLDivElement;
  let root: ShadowRoot;
  let mounted = $state(false);
  let figure = $state.raw<ReturnType<typeof mountFolderFigure>>();
  onMount(() => {
    root = host.attachShadow({ mode: 'open' });
    mounted = true;
    return () => {
      figure?.destroy();
      root.replaceChildren();
    };
  });
  $effect(() => {
    if (!mounted) return;
    const stage = document.createElement('div');
    root.append(stage);
    const svg = prepareFolderStage(root, stage);
    const handle = mountFolderFigure({ stage, svg, read: document.createElement('span'), papers, controlled: true }, 8);
    figure = handle;
    return () => {
      handle.destroy();
      stage.remove();
    };
  });
  $effect(() => {
    figure?.activate(active);
  });
</script>

<div class="figure" bind:this={host} aria-hidden="true"></div>

<style>
  .figure {
    display: block;
    width: 100%;
    aspect-ratio: 5 / 4;
    overflow: hidden;
    --hairline-plate: #000;
    --hairline-hi: #fff;
    --hairline-edge: #858585;
    --hairline-mid: #666;
    --hairline-lo: #3b3b3b;
  }
</style>
