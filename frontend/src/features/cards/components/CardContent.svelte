<script lang="ts">
  import { contentKindMessageKeys } from './consts';
  import { useLocalization } from '../../localization';
  const l = useLocalization();
  import { onDestroy } from 'svelte';
  import type { Card, MediaURLPort } from '../domain/types';
  import { orderedItems } from '../domain/funcs';
  import { stopMedia } from '../../ui';
  import AudioPlayer from './AudioPlayer.svelte';
  import VideoPreview from './VideoPreview.svelte';
  import ExpandedVideo from './ExpandedVideo.svelte';
  import ImagePreview from './ImagePreview.svelte';
  import type { PreviewStartPort } from '../ports/types';

  let {
    card,
    mediaURL,
    compact = false,
    expanded = false,
    reviewing = false,
    nativeStart,
    onopen,
  }: {
    card: Card;
    mediaURL: MediaURLPort;
    compact?: boolean;
    expanded?: boolean;
    reviewing?: boolean;
    nativeStart?: PreviewStartPort;
    onopen?: (itemId: string, opener: HTMLElement) => void;
  } = $props();
  let root: HTMLDivElement;
  let failed = $state<string[]>([]);
  let hovered = $state<number | null>(null);
  let focused = $state<number | null>(null);
  const items = $derived(orderedItems(card));
  const active = $derived(hovered ?? focused);
  const rows = $derived(Math.ceil(items.length / 2));
  const columns = $derived(
    active === null || (active === items.length - 1 && items.length % 2 === 1)
      ? 'minmax(0, 1fr) minmax(0, 1fr)'
      : active % 2 === 0
        ? 'minmax(0, 3fr) minmax(0, 1fr)'
        : 'minmax(0, 1fr) minmax(0, 3fr)',
  );
  const rowTracks = $derived(
    [...new Array<number>(rows).keys()]
      .map((row) => `minmax(0, ${active !== null && Math.floor(active / 2) === row ? '3' : '1'}fr)`)
      .join(' '),
  );
  function reviewTime(due: string): string {
    const date = new Date(due);
    if (!Number.isFinite(date.getTime())) return $l.t('cards.review_time_unavailable');
    return date.getTime() <= Date.now()
      ? $l.t('cards.ready_to_review_now')
      : $l.t('cards.nextReview', { date: $l.date(date) });
  }
  onDestroy(() => {
    stopMedia(root);
  });
</script>

<div
  class="card-content"
  class:compact
  class:expanded
  bind:this={root}
  style:grid-template-columns={compact ? columns : undefined}
  style:grid-template-rows={compact ? rowTracks : undefined}
>
  {#each items as item, index (item.id)}
    <!-- svelte-ignore a11y_no_noninteractive_tabindex (Keyboard focus expands the preview and reveals its review time.) -->
    <div
      class="content-item"
      class:media-item={item.kind !== 'text'}
      class:audio-item={item.kind === 'audio'}
      class:video-item={item.kind === 'video'}
      class:active={compact && active === index}
      role="group"
      aria-label={$l.t('cards.content', { kind: $l.t(contentKindMessageKeys[item.kind]) })}
      tabindex={compact ? 0 : undefined}
      onpointerenter={(event) => {
        if (compact && event.pointerType === 'mouse') hovered = index;
      }}
      onmouseenter={() => {
        if (compact) hovered = index;
      }}
      onmouseleave={() => {
        hovered = null;
      }}
      onpointerleave={() => {
        hovered = null;
      }}
      onfocusin={() => {
        if (compact) focused = index;
      }}
      onfocusout={(event) => {
        if (!event.currentTarget.contains(event.relatedTarget instanceof Node ? event.relatedTarget : null))
          focused = null;
      }}
    >
      {#if item.kind === 'text'}<p dir="auto">{item.text}</p>
      {:else if item.kind === 'image'}
        {#key mediaURL(item)}<ImagePreview
            src={mediaURL(item)}
            filename={item.originalName}
            {compact}
            {expanded}
            active={active === index}
          />{/key}
      {:else if item.kind === 'video' && compact}<div class="video-slot">
          {#key mediaURL(item)}<VideoPreview
              src={mediaURL(item)}
              filename={item.originalName}
              hovered={hovered === index}
              focused={focused === index}
              {nativeStart}
            />{/key}
        </div>
      {:else if failed.includes(item.id)}<p class="media-error" role="status">
          {$l.t('cards.mediaFailed', { name: item.originalName ?? '' })}
        </p>
      {:else if item.kind === 'audio'}<AudioPlayer
          filename={item.originalName}
          {compact}
          autoplay={reviewing}
          hideFilename={reviewing}
          hovered={compact && hovered === index}
          focused={compact && focused === index}
          {nativeStart}
          src={mediaURL(item)}
          onerror={() => {
            failed = [...failed, item.id];
          }}
        />
      {:else if item.kind === 'video' && expanded}
        <ExpandedVideo
          src={mediaURL(item)}
          filename={item.originalName}
          onerror={() => {
            failed = [...failed, item.id];
          }}
        />
      {:else if item.kind === 'video'}
        <video
          dir="ltr"
          controls
          autoplay={reviewing}
          muted={reviewing}
          playsinline
          preload={reviewing ? 'auto' : 'none'}
          aria-label={item.originalName}
          src={mediaURL(item)}
          onerror={() => {
            failed = [...failed, item.id];
          }}
        ></video>{#if !reviewing}<span class="media-name">{item.originalName}</span>{/if}
      {/if}
      {#if compact}<span class="review-time"
          >{item.hint === true ? $l.t('cards.hint_for_other_items') : reviewTime(item.schedule.Due)}</span
        >{/if}
      {#if onopen}
        <button
          class="open-part"
          type="button"
          aria-label={$l.t('cards.openPart', { kind: $l.t(contentKindMessageKeys[item.kind]), count: index + 1 })}
          onclick={(event) => {
            onopen?.(item.id, event.currentTarget);
          }}
        ></button>
      {/if}
    </div>
  {/each}
</div>

<style>
  .card-content {
    display: grid;
    gap: 18px;
  }
  .content-item {
    position: relative;
    min-width: 0;
  }
  .expanded {
    width: max-content;
    max-width: calc(100vw - 26px);
    min-height: 0;
    grid-template-columns: minmax(0, 1fr);
    gap: 0;
  }
  .expanded .content-item {
    display: flex;
    flex-direction: column;
    justify-content: safe center;
    min-height: 0;
    max-height: calc(100svh - 26px);
    overflow-y: auto;
  }
  .expanded .content-item > p {
    width: max-content;
    max-width: min(100%, 64ch);
    box-sizing: border-box;
    margin-inline: auto;
    padding: 64px 24px 24px;
    text-align: center;
    font-size: clamp(18px, 2.2vw, 28px);
    line-height: 1.7;
  }
  .expanded .media-item {
    overflow: hidden;
  }
  .expanded .audio-item {
    padding-top: 64px;
  }
  .expanded .audio-item :global(.audio-player) {
    box-sizing: border-box;
    width: min(720px, calc(100vw - 26px));
    margin: auto;
  }
  .open-part {
    position: absolute;
    inset: 0;
    width: 100%;
    border: 0;
    padding: 0;
    background: transparent;
    cursor: pointer;
  }
  .open-part:focus-visible {
    outline: 2px solid var(--muted);
    outline-offset: -2px;
  }
  .audio-item :global(.play),
  .audio-item :global(input) {
    position: relative;
    z-index: 1;
  }
  .media-item {
    padding: 0;
    background: #000;
  }
  p {
    white-space: pre-wrap;
    overflow-wrap: anywhere;
    margin: 0;
    font-size: 15px;
    line-height: 1.8;
    color: var(--ink);
  }
  video {
    display: block;
    width: 100%;
    height: 200px;
    object-fit: contain;
    border-radius: 5px;
    background: #000;
  }
  .compact {
    grid-template-columns: repeat(2, minmax(0, 1fr));
    grid-auto-rows: minmax(0, 1fr);
    height: 100%;
    min-height: 0;
    gap: 1px;
    background: var(--line);
  }
  .compact {
    transition:
      grid-template-columns 300ms ease,
      grid-template-rows 300ms ease;
  }
  .compact .content-item {
    position: relative;
    min-height: 0;
    overflow: hidden;
    background: #000;
  }
  .compact .content-item:last-child:nth-child(odd) {
    grid-column: 1 / -1;
  }
  .compact .media-item {
    padding: 0;
  }
  .compact .content-item:not(.media-item) {
    display: flex;
    align-items: center;
    justify-content: center;
    text-align: center;
  }
  .compact p {
    padding: 10px;
    font-size: 14px;
    line-height: 1.6;
  }
  .compact video {
    height: 100%;
    border-radius: 5px;
    object-fit: cover;
  }
  .video-slot {
    height: 100%;
    min-height: 0;
  }
  .compact .content-item:focus-visible {
    outline: 2px solid #a3a3a3;
    outline-offset: -2px;
  }
  .review-time {
    position: absolute;
    inset: auto 0 0;
    padding: 7px 9px;
    border-top: 1px solid var(--line);
    background: #000;
    color: var(--muted);
    font-size: 10px;
    line-height: 1.5;
    opacity: 0;
    pointer-events: none;
  }
  .active .review-time {
    opacity: 1;
  }
  .compact .active:not(.media-item) {
    padding-bottom: 44px;
  }
  .compact .active.video-item {
    padding-bottom: 44px;
  }
  .compact .audio-item {
    padding-bottom: 44px;
  }
  @media (prefers-reduced-motion: reduce) {
    .compact {
      transition: none;
    }
  }
  .media-name {
    display: block;
    margin-top: 6px;
    color: var(--muted);
    font-size: 10px;
    overflow-wrap: anywhere;
  }
  .media-error {
    font-size: 12px;
    padding: 4px;
  }
</style>
