<script lang="ts">
  import { useLocalization } from '../../localization';
  const l = useLocalization();
  import { onMount, onDestroy } from 'svelte';
  import { createPreviewPlayback } from '../application/funcs';
  import type { PreviewStartPort } from '../ports/types';
  import { Skeleton } from '../../ui';

  let {
    src,
    filename = $l.t('cards.video'),
    hovered = false,
    nativeStart,
  }: {
    src: string;
    filename?: string;
    hovered?: boolean;
    nativeStart?: PreviewStartPort;
  } = $props();
  let video: HTMLVideoElement;
  let preview = $state.raw<ReturnType<typeof createPreviewPlayback>>();
  let hidden = $state(false);
  let ready = $state(false);
  let failed = $state(false);
  const active = $derived(hovered && !hidden);

  function previewFailed() {
    failed = true;
    preview?.stop();
  }
  onMount(() => {
    preview = createPreviewPlayback(video, nativeStart);
    ready = video.readyState >= HTMLMediaElement.HAVE_CURRENT_DATA;
    failed = video.error !== null;
    const hide = () => {
      hidden = document.hidden;
    };
    document.addEventListener('visibilitychange', hide);
    return () => {
      document.removeEventListener('visibilitychange', hide);
    };
  });
  $effect(() => {
    // A native window may retain its initial hidden state after being shown.
    // An explicit hover is authoritative and must allow a new playback attempt.
    if (hovered) hidden = false;
  });
  $effect(() => {
    if (active) preview?.start().catch(previewFailed);
    else preview?.stop();
  });
  onDestroy(() => preview?.stop());
</script>

<div class="video-preview" aria-busy={!ready && !failed}>
  {#if failed}<p class="media-error" role="status">{$l.t('cards.mediaFailed', { name: filename })}</p>{/if}
  <video
    dir="ltr"
    bind:this={video}
    {src}
    muted
    playsinline
    loop
    preload="auto"
    aria-label={filename || $l.t('cards.video_preview')}
    class:pending={!ready || failed}
    onloadeddata={() => {
      ready = true;
    }}
    oncanplay={() => {
      preview?.retry().catch(previewFailed);
    }}
    onpause={() => {
      preview?.retry().catch(previewFailed);
    }}
    onended={() => {
      preview?.retry().catch(previewFailed);
    }}
    onerror={() => {
      failed = true;
      preview?.stop();
    }}
  ></video>
  {#if !ready && !failed}<div class="placeholder"><Skeleton height="100%" radius="5px" /></div>{/if}
</div>

<style>
  .video-preview {
    position: relative;
    width: 100%;
    height: 100%;
    min-height: 0;
    background: #000;
  }
  video {
    display: block;
    width: 100%;
    height: 100%;
    object-fit: cover;
    border-radius: 5px;
  }
  video::-webkit-media-controls,
  video::-webkit-media-controls-start-playback-button {
    display: none;
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
    position: absolute;
    inset: 0;
    margin: 0;
    padding: 4px;
    color: var(--ink);
    font-size: 12px;
    line-height: 1.8;
    overflow-wrap: anywhere;
  }
</style>
