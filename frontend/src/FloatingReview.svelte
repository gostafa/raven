<script lang="ts">
  import { onMount } from 'svelte';
  import { Events } from '@wailsio/runtime';
  import { RavenService } from '../bindings/raven';
  import type { Folder } from '../bindings/raven/internal/storage/models';
  import { reviewItems, stopMedia, type ReviewItem } from './cards';
  import ReviewSession from './ReviewSession.svelte';

  let status = $state<'loading' | 'ready' | 'error' | 'closed'>('loading');
  let entries = $state<ReviewItem[]>([]);
  let folders = $state<Folder[]>([]);
  let saving = $state(false);
  let closing = $state(false);
  let closeError = $state('');
  let generation = $state(0);
  let root: HTMLElement;

  async function load() {
    if (saving || closing) return;
    const request = ++generation;
    status = 'loading'; entries = []; closeError = '';
    try {
      const [cards, savedFolders] = await Promise.all([RavenService.ListCards(), RavenService.ListFolders()]);
      if (request !== generation) return;
      entries = reviewItems(cards ?? []).filter((entry) => entry.dueAt <= Date.now());
      folders = savedFolders ?? [];
      status = 'ready';
    } catch { if (request === generation) status = 'error'; }
  }

  async function close() {
    if (saving || closing) return;
    closing = true;
    stopMedia(root);
    try {
      await RavenService.HideFloatingReview();
      ++generation; entries = []; status = 'closed'; closeError = '';
    } catch { closeError = 'Could not close this review. Please try again.'; }
    finally { closing = false; }
  }

  function handleKey(event: KeyboardEvent) {
    if (event.key === 'Escape') { event.preventDefault(); void close(); }
  }

  onMount(() => {
    const open = Events.On('raven:open-floating-review', () => { void load(); });
    const dismiss = Events.On('raven:close-floating-review', () => { void close(); });
    // The first native show can precede event subscription. Mount also loads a
    // fresh session so the first click never depends on receiving that event.
    void load();
    return () => { ++generation; open(); dismiss(); stopMedia(root); };
  });
</script>

<svelte:window onkeydown={handleKey} />
<main class="floating-review" aria-label="Floating review" bind:this={root}>
  {#if status === 'ready'}
    {#key generation}<ReviewSession {entries} {folders} floating disabled={closing} bind:saving onclose={() => { void close(); }} />{/key}
  {:else if status !== 'closed'}
    <header><span>A LITTLE PRACTICE</span><button aria-label="Close review" disabled={closing} onclick={() => close()}>✕</button></header>
    <div class="status" role="status">
      {#if status === 'loading'}<p>Loading your reviews…</p>
      {:else}<h1>Could not load your reviews</h1><p>Your memories are still saved. Try again to reconnect to your local database.</p><button class="retry" onclick={() => load()}>Try again</button>{/if}
    </div>
  {/if}
  {#if closeError}<p class="error" role="alert">{closeError}</p>{/if}
</main>

<style>
  .floating-review { --ink: #fff; --muted: #a3a3a3; display: flex; flex-direction: column; height: 100svh; overflow: hidden; padding: 20px; border: 1px solid var(--line); border-radius: 14px; background: #000; color: #fff; }
  header { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
  header span { font-size: 8px; font-weight: 600; letter-spacing: 1.5px; }
  header button { width: 30px; height: 30px; border: 0; border-radius: 6px; background: transparent; color: var(--muted); }
  header button:hover { background: var(--hover); color: #fff; }
  .status { display: flex; flex: 1; flex-direction: column; justify-content: center; align-items: center; text-align: center; min-height: 0; overflow: auto; }
  h1 { font: 26px/1.3 Georgia, serif; }
  p { font-size: 12px; line-height: 1.8; }
  .retry { padding: 11px 16px; border: 1px solid #fff; border-radius: 6px; background: #fff; color: #000; }
  .retry:hover { border-color: var(--line-strong); background: #151515; color: #fff; }
  .error { padding: 10px; border: 1px solid var(--line); border-radius: 6px; }
</style>
