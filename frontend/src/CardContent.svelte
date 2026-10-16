<script lang="ts">
  import { onDestroy } from 'svelte';
  import type { Card } from '../bindings/raven/internal/storage/models';
  import { mediaURL, orderedItems, stopMedia } from './cards';

  let { card, compact = false }: { card: Card; compact?: boolean } = $props();
  let root: HTMLDivElement;
  let failed = $state<string[]>([]);
  let hovered = $state<number | null>(null);
  let focused = $state<number | null>(null);
  const items = $derived(orderedItems(card));
  const active = $derived(hovered ?? focused);
  const rows = $derived(Math.ceil(items.length / 2));
  const columns = $derived(active === null || (active === items.length - 1 && items.length % 2 === 1)
    ? 'minmax(0, 1fr) minmax(0, 1fr)'
    : active % 2 === 0 ? 'minmax(0, 3fr) minmax(0, 1fr)' : 'minmax(0, 1fr) minmax(0, 3fr)');
  const rowTracks = $derived(Array.from({ length: rows }, (_, row) => `minmax(0, ${active !== null && Math.floor(active / 2) === row ? 3 : 1}fr)`).join(' '));
  function reviewTime(due: string): string {
    const date = new Date(due);
    if (!Number.isFinite(date.getTime())) return 'Review time unavailable';
    return date.getTime() <= Date.now() ? 'Ready to review now' : `Next review: ${date.toLocaleString(undefined, { month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit' })}`;
  }
  onDestroy(() => stopMedia(root));
</script>

<div class="card-content" class:compact bind:this={root} style:grid-template-columns={compact ? columns : undefined} style:grid-template-rows={compact ? rowTracks : undefined}>
  {#each items as item, index (item.id)}
    <!-- svelte-ignore a11y_no_noninteractive_tabindex (Keyboard focus expands the preview and reveals its review time.) -->
    <div class="content-item" class:media-item={item.kind !== 'text'} class:active={compact && active === index}
      role="group" aria-label={`${item.kind} content`} tabindex={compact ? 0 : undefined}
      onpointerenter={(event) => { if (compact && event.pointerType === 'mouse') hovered = index; }} onpointerleave={() => { hovered = null; }}
      onfocusin={() => { if (compact) focused = index; }} onfocusout={(event) => { if (!event.currentTarget.contains(event.relatedTarget as Node)) focused = null; }}>
      {#if item.kind === 'text'}<p>{item.text}</p>
      {:else if failed.includes(item.id)}<p class="media-error" role="status">“{item.originalName}” could not be loaded or played. Check that the file is available and its format is supported on this device.</p>
      {:else if item.kind === 'image'}<img src={mediaURL(item)} alt={item.originalName || 'Card image'} onerror={() => failed = [...failed, item.id]} />
      {:else if item.kind === 'audio'}<audio controls preload="none" aria-label={item.originalName} src={mediaURL(item)} onerror={() => failed = [...failed, item.id]}></audio>{#if !compact}<span class="media-name">{item.originalName}</span>{/if}
      {:else if item.kind === 'video'}
        <!-- svelte-ignore a11y_media_has_caption (Imported videos do not currently include separate caption files.) -->
        <video controls playsinline preload={compact ? 'metadata' : 'none'} aria-label={item.originalName} src={mediaURL(item)} onerror={() => failed = [...failed, item.id]}></video>{#if !compact}<span class="media-name">{item.originalName}</span>{/if}
      {/if}
      {#if compact}<span class="review-time">{reviewTime(item.schedule.Due)}</span>{/if}
    </div>
  {/each}
</div>

<style>
  .card-content { display: grid; gap: 18px; }
  .content-item { min-width: 0; }
  .media-item { padding: 0; background: #000; }
  p { white-space: pre-wrap; overflow-wrap: anywhere; margin: 0; font-size: 15px; line-height: 1.8; color: var(--ink); }
  img, video { display: block; width: 100%; height: 200px; object-fit: contain; border-radius: 5px; background: #000; }
  audio { width: 100%; display: block; }
  .compact { grid-template-columns: repeat(2, minmax(0, 1fr)); grid-auto-rows: minmax(0, 1fr); height: 100%; min-height: 0; gap: 1px; background: var(--line); }
  .compact { transition: grid-template-columns 300ms ease, grid-template-rows 300ms ease; }
  .compact .content-item { position: relative; min-height: 0; overflow: hidden; background: #000; }
  .compact .content-item:last-child:nth-child(odd) { grid-column: 1 / -1; }
  .compact .media-item { padding: 0; }
  .compact .content-item:not(.media-item) { display: flex; align-items: center; justify-content: center; text-align: center; }
  .compact p { padding: 10px; font-size: 14px; line-height: 1.6; }
  .compact img, .compact video { height: 100%; border-radius: 5px; object-fit: cover; }
  .compact audio { height: 100%; min-height: 32px; }
  .compact .content-item:focus-visible { outline: 1px solid var(--focus); outline-offset: -1px; }
  .review-time { position: absolute; inset: auto 0 0; padding: 7px 9px; border-top: 1px solid var(--line); background: #000; color: var(--muted); font-size: 10px; line-height: 1.5; opacity: 0; pointer-events: none; }
  .active .review-time { opacity: 1; }
  .compact .active:not(.media-item) { padding-bottom: 44px; }
  .compact .active audio, .compact .active video { height: calc(100% - 44px); }
  @media (prefers-reduced-motion: reduce) { .compact { transition: none; } }
  .media-name { display: block; margin-top: 6px; color: var(--muted); font-size: 10px; overflow-wrap: anywhere; }
  .media-error { font-size: 12px; padding: 4px; }
</style>
