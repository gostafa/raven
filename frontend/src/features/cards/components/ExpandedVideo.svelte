<script lang="ts">
  import { useLocalization } from '../../localization';

  const l = useLocalization();
  let {
    src,
    filename,
    onerror,
  }: {
    src: string;
    filename?: string;
    onerror: () => void;
  } = $props();
  let width = $state(0);
  let height = $state(0);
  const aspectRatio = $derived(width > 0 && height > 0 ? width / height : 16 / 9);
</script>

<!-- svelte-ignore a11y_media_has_caption (Imported videos do not include separate caption files.) -->
<video
  dir="ltr"
  controls
  playsinline
  preload="metadata"
  aria-label={filename !== undefined && filename.length > 0 ? filename : $l.t('cards.video')}
  {src}
  bind:videoWidth={width}
  bind:videoHeight={height}
  style:--video-ratio={aspectRatio}
  {onerror}
></video>

<style>
  video {
    display: block;
    width: min(calc(100vw - 26px), calc((100svh - 26px) * var(--video-ratio)));
    max-width: 100%;
    height: auto;
    max-height: calc(100svh - 26px);
    aspect-ratio: var(--video-ratio);
    object-fit: contain;
    background: #000;
  }
  video:focus-visible {
    outline: 2px solid var(--muted);
    outline-offset: -2px;
  }
</style>
