<script lang="ts">
  import { onMount } from 'svelte';
  import { reviewItems } from '../../reviews';
  import type { Card } from '../../cards';
  import type { AppPorts } from '../../workspace';
  let { backend }: { backend: AppPorts } = $props();
  let cards = $state<Card[]>([]);
  let now = $state(0);
  const dueCount = $derived(reviewItems(cards).filter((entry) => entry.dueAt <= now).length);
  let error = $state('');
  let widgetPointerStart: { x: number; y: number } | undefined;
  async function refresh() {
    try {
      cards = await backend.ListCards();
      now = backend.now();
    } catch {
      /* Keep the last snapshot. */
    }
  }
  async function openFromWidget(event: MouseEvent) {
    const moved =
      event.detail !== 0 &&
      widgetPointerStart &&
      Math.hypot(event.screenX - widgetPointerStart.x, event.screenY - widgetPointerStart.y) > 4;
    widgetPointerStart = undefined;
    if (moved) return;
    try {
      error = '';
      await backend.ToggleFloatingReview();
    } catch {
      error = 'Raven could not complete that action. Please try again.';
    }
  }
  function focus() {
    void refresh();
    void backend.RefreshReminders();
  }
  onMount(() => {
    const dispose = backend.events.onSchedulesChanged(() => {
      void refresh();
    });
    void refresh();
    const timer = window.setInterval(() => {
      now = backend.now();
    }, 1000);
    return () => {
      dispose();
      window.clearInterval(timer);
    };
  });
</script>

<svelte:window onfocus={focus} />
<div class="widget" style="--custom-contextmenu: raven;">
  <button
    class="widget-logo"
    aria-label={dueCount
      ? `${dueCount} ${dueCount === 1 ? 'item' : 'items'} ready to review. Toggle floating review`
      : 'Toggle floating review'}
    title={`${dueCount ? `${dueCount} ${dueCount === 1 ? 'item' : 'items'} ready to review · ` : ''}Drag to move · click to toggle floating review · right-click for more`}
    onpointerdown={(event) => {
      if (event.button === 0) widgetPointerStart = { x: event.screenX, y: event.screenY };
    }}
    onclick={openFromWidget}
    ><img src="/raven.svg" alt="" draggable="false" />{#if dueCount}<span class="widget-due" aria-hidden="true"
        >{dueCount > 9 ? '9+' : dueCount}</span
      >{/if}</button
  >
  {#if error}<span class="sr-only" role="alert">{error}</span>{/if}
</div>

<style>
  .widget {
    position: fixed;
    inset: 0;
    margin: auto;
    /* Wails' macOS content view is one point smaller than its webview. */
    width: calc(min(100dvw, 100dvh) - 2px);
    height: calc(min(100dvw, 100dvh) - 2px);
    aspect-ratio: 1;
    background: #fff;
    border-radius: 50%;
    clip-path: circle(50% at 50% 50%);
    overflow: hidden;
    cursor: grab;
    user-select: none;
    --wails-draggable: drag;
  }
  .widget-logo {
    -webkit-appearance: none;
    appearance: none;
    position: absolute;
    inset: 0;
    display: grid;
    place-items: center;
    width: 100%;
    height: 100%;
    min-width: 0;
    min-height: 0;
    padding: 2px;
    border: 0;
    border-radius: 50%;
    overflow: hidden;
    background: transparent;
    box-shadow: none;
    cursor: inherit;
    --wails-draggable: drag;
  }
  .widget-due {
    position: absolute;
    top: 10%;
    right: 10%;
    display: grid;
    place-items: center;
    width: 23px;
    height: 23px;
    padding: 0;
    border: 2px solid white;
    border-radius: 50%;
    background: #000;
    color: #fff;
    font-size: 11px;
    line-height: 1;
    font-weight: 600;
    pointer-events: none;
  }
  .widget:active,
  .widget-logo:active {
    cursor: grabbing;
  }
  .widget-logo img {
    width: 80%;
    height: 80%;
    object-fit: contain;
    pointer-events: none;
  }
  .widget-logo:hover {
    opacity: 0.8;
  }
  .widget-logo:focus-visible {
    outline-offset: -3px;
    outline: 2px solid var(--focus);
  }
  .sr-only {
    position: absolute;
    width: 1px;
    height: 1px;
    padding: 0;
    margin: -1px;
    overflow: hidden;
    clip: rect(0, 0, 0, 0);
    white-space: nowrap;
    border: 0;
  }
</style>
