<script lang="ts">
  import { useLocalization, type MessageKey } from '../../localization';
  const l = useLocalization();
  import { onMount } from 'svelte';
  import { reviewItems } from '../../reviews';
  import type { Card } from '../../cards';
  import type { AppPorts } from '../../workspace';
  let { backend }: { backend: AppPorts } = $props();
  let cards = $state<Card[]>([]);
  let now = $state(0);
  const dueCount = $derived(reviewItems(cards).filter((entry) => entry.dueAt <= now).length);
  let error = $state<MessageKey | ''>('');
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
    if (moved === true) return;
    try {
      error = '';
      await backend.ToggleFloatingReview();
    } catch {
      error = 'companion.raven_could_not_complete_that_action_please_try_again';
    }
  }
  function reportFailure() {
    error = 'companion.raven_could_not_complete_that_action_please_try_again';
  }
  function focus() {
    refresh().catch(reportFailure);
    backend.RefreshReminders().catch(reportFailure);
  }
  onMount(() => {
    const dispose = backend.events.onSchedulesChanged(() => {
      refresh().catch(reportFailure);
    });
    refresh().catch(reportFailure);
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
    class:has-due={dueCount > 0}
    aria-label={dueCount
      ? $l.t('widget.reviewLabel', { items: $l.t('common.items', { count: dueCount }) })
      : $l.t('companion.toggle_floating_review')}
    title={$l.t('widget.reviewTitle', {
      items: $l.t('common.items', { count: dueCount }),
      instructions: $l.t('widget.instructions'),
    })}
    onpointerdown={(event) => {
      if (event.button === 0) widgetPointerStart = { x: event.screenX, y: event.screenY };
    }}
    onclick={openFromWidget}
    ><img src="/raven.svg" alt="" draggable="false" />{#if dueCount}<span class="widget-due" aria-hidden="true"
        >{dueCount > 9 ? `${$l.number(9)}+` : $l.number(dueCount)}</span
      >{/if}</button
  >
  {#if error}<span class="sr-only" role="alert">{error ? $l.t(error) : ''}</span>{/if}
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
    container-type: inline-size;
    border-radius: 50%;
    overflow: visible;
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
    overflow: visible;
    background: #fff;
    box-shadow: none;
    cursor: inherit;
    --wails-draggable: drag;
  }
  .widget-logo.has-due {
    inset: auto auto 0 0;
    width: 90%;
    height: 90%;
  }
  .widget-due {
    position: absolute;
    top: -5%;
    right: -5%;
    display: grid;
    place-items: center;
    width: 45%;
    height: 45%;
    padding: 0;
    border: 0;
    border-radius: 50%;
    background: #000;
    color: #fff;
    font-size: 12px;
    line-height: 1;
    font-weight: 700;
    direction: ltr;
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
    outline: 2px solid #555;
    outline-offset: -3px;
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
