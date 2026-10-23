<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { FolderPaper } from '../domain/types';
  import FolderFigure from './FolderFigure.svelte';

  let { papers, children }: { papers: readonly FolderPaper[]; children: Snippet<[Snippet]> } = $props();
  let hovered = $state(false);
  let focused = $state(false);
</script>

<div
  class="folder-interaction"
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
  {#snippet illustration()}<FolderFigure {papers} active={hovered || focused} />{/snippet}
  {@render children(illustration)}
</div>

<style>
  .folder-interaction {
    position: relative;
    height: 100%;
  }
</style>
