<script lang="ts">
  import { ImagePreview, VideoPreview, type MediaURLPort, type PreviewStartPort } from '../../cards';
  import type { FolderPaper } from '../domain/types';

  let {
    active = false,
    papers,
    mediaURL,
    nativeStart,
    reducedMotion = false,
  }: {
    active?: boolean;
    papers: readonly FolderPaper[];
    mediaURL: MediaURLPort;
    nativeStart?: PreviewStartPort;
    reducedMotion?: boolean;
  } = $props();
</script>

<div class="folder" class:open={active} aria-hidden="true">
  <span class="folder-back"></span>
  <span class="paper-stack" style:--paper-count={papers.length}>
    {#each papers as paper, index (paper.key)}
      <span class="sheet" style:--index={index}>
        {#if active}
          {#key mediaURL(paper.item)}
            {#if paper.kind === 'video'}
              <VideoPreview
                src={mediaURL(paper.item)}
                filename={paper.item.originalName}
                hovered={!reducedMotion}
                {nativeStart}
              />
              <span class="video-mark"
                ><svg width="12" height="12" viewBox="0 0 16 16" fill="currentColor"><path d="M5 3v10l8-5z" /></svg
                ></span
              >
            {:else}
              <ImagePreview src={mediaURL(paper.item)} filename={paper.item.originalName} compact />
            {/if}
          {/key}
        {/if}
      </span>
    {/each}
  </span>
  <span class="folder-face">
    <span class="folder-tab"></span>
    <span class="folder-front"></span>
  </span>
</div>

<style>
  .folder,
  .folder * {
    box-sizing: border-box;
  }

  .folder {
    position: absolute;
    inset: 0;
    pointer-events: none;
    isolation: isolate;
    perspective: 1000px;
    --folder-line: var(--line);
  }

  .folder.open {
    --folder-line: var(--line-strong);
  }

  .folder-back,
  .folder-tab,
  .folder-front {
    position: absolute;
    border: 1px solid var(--folder-line);
    border-radius: 10px;
    transition: border-color 200ms ease;
  }

  .folder-back {
    z-index: 0;
    inset: 36px 8px 8px;
    background: #181818;
  }

  .folder-face {
    position: absolute;
    z-index: 2;
    inset: 0;
    transform-origin: center bottom;
    transition: transform 300ms ease;
  }

  .folder.open .folder-face {
    transform: rotateX(-12deg);
  }

  .paper-stack {
    position: absolute;
    z-index: 1;
    top: 0px;
    inset-inline: 0;
    margin-inline: auto;
    width: clamp(120px, 36%, 150px);
    height: clamp(140px, 18vw, 200px);
    opacity: 0;
    transform: translateY(0) scale(0.9);
    transition:
      transform 300ms ease,
      opacity 200ms ease;
  }

  .sheet {
    position: absolute;
    inset: 0;
    z-index: calc(var(--paper-count) - var(--index));
    border: 1px solid var(--line-strong);
    border-radius: 8px;
    background: #181818;
    overflow: hidden;
    transform-origin: 50% 100%;
    --fan-offset: calc(var(--index) - (var(--paper-count) - 1) / 2);
    transform: translateX(calc(var(--fan-offset) * 12px)) rotate(calc(var(--fan-offset) * 12deg));
  }

  .folder.open .sheet {
    pointer-events: auto;
  }

  .sheet:hover {
    z-index: 1000;
  }

  .video-mark {
    position: absolute;
    top: 8px;
    inset-inline-end: 8px;
    display: grid;
    place-items: center;
    width: 22px;
    height: 22px;
    border: 1px solid var(--line-strong);
    border-radius: 50%;
    background: #000;
    color: #fff;
  }

  .folder-tab {
    z-index: 2;
    top: 24px;
    inset-inline-start: 0;
    width: 40%;
    height: 35px;
    border-bottom: 0;
    border-end-start-radius: 0;
    border-end-end-radius: 0;
    background: #000;
  }

  .folder-front {
    z-index: 3;
    inset: 48px 0 0;
    background: #000;
    border-start-start-radius: 0;
  }

  .folder-front::before {
    content: '';
    position: absolute;
    top: -1px;
    inset-inline-start: 0;
    width: calc(40% - 1px);
    height: 1px;
    background: #000;
  }

  .folder.open .paper-stack {
    transform: translateY(-90px) scale(1);
    opacity: 1;
  }

  @media (max-width: 600px) {
    .sheet {
      transform: translateX(calc(var(--fan-offset) * 9px)) rotate(calc(var(--fan-offset) * 9deg));
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .folder * {
      transition: none;
    }

    .folder.open .folder-face {
      transform: none;
    }
  }
</style>
