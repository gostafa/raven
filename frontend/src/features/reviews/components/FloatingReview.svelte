<script lang="ts">
  import { useLocalization, type MessageKey } from '../../localization';
  const l = useLocalization();
  import { onMount } from 'svelte';
  import type { Folder } from '../../folders';
  import type { AppPorts } from '../../workspace';
  import { loadDueReview } from '../application/funcs';
  import { stopMedia } from '../../ui';
  import type { ReviewItem } from '../domain/types';
  import ReviewSession from './ReviewSession.svelte';

  let { backend }: { backend: AppPorts } = $props();
  let status = $state<'loading' | 'ready' | 'error' | 'closed'>('loading');
  let entries = $state<ReviewItem[]>([]);
  let folders = $state<Folder[]>([]);
  let saving = $state(false);
  let closing = $state(false);
  let closeError = $state<MessageKey | ''>('');
  let generation = $state(0);
  let root: HTMLElement;

  async function load() {
    if (saving || closing) return;
    const request = ++generation;
    status = 'loading';
    entries = [];
    closeError = '';
    try {
      const loaded = await loadDueReview(backend, backend, backend.now);
      if (request !== generation) return;
      entries = loaded.entries;
      folders = loaded.folders;
      status = 'ready';
    } catch {
      if (request === generation) status = 'error';
    }
  }

  async function close() {
    if (saving || closing) return;
    closing = true;
    stopMedia(root);
    try {
      await backend.HideFloatingReview();
      ++generation;
      entries = [];
      status = 'closed';
      closeError = '';
    } catch {
      closeError = 'reviews.could_not_close_this_review_please_try_again';
    } finally {
      closing = false;
    }
  }

  function handleKey(event: KeyboardEvent) {
    if (event.key === 'Escape') {
      event.preventDefault();
      void close();
    }
  }

  onMount(() => {
    const open = backend.events.onOpenFloatingReview(() => {
      void load();
    });
    const dismiss = backend.events.onCloseFloatingReview(() => {
      void close();
    });
    // The first native show can precede event subscription. Mount also loads a
    // fresh session so the first click never depends on receiving that event.
    void load();
    return () => {
      ++generation;
      open();
      dismiss();
      stopMedia(root);
    };
  });
</script>

<svelte:window onkeydown={handleKey} />
<main class="floating-review" aria-label={$l.t('reviews.floating_review')} bind:this={root}>
  {#if status === 'ready'}
    {#key generation}<ReviewSession
        review={backend}
        mediaURL={backend.mediaURL}
        nativeStart={backend.previewStart}
        {entries}
        {folders}
        floating
        disabled={closing}
        bind:saving
        onclose={() => {
          void close();
        }}
      />{/key}
  {:else if status !== 'closed'}
    <header>
      <span>{$l.t('reviews.review')}</span><button
        aria-label={$l.t('reviews.close_review')}
        disabled={closing}
        onclick={() => close()}>✕</button
      >
    </header>
    <div class="status" role="status">
      {#if status === 'loading'}<p>{$l.t('reviews.loading_your_reviews')}</p>
      {:else}<h1>{$l.t('reviews.could_not_load_your_reviews')}</h1>
        <p>{$l.t('reviews.your_memories_are_still_saved_try_again_to_reconnect_to_your_local_database')}</p>
        <button class="retry" onclick={() => load()}>{$l.t('workspace.try_again')}</button>{/if}
    </div>
  {/if}
  {#if closeError}<p class="error" role="alert">{closeError ? $l.t(closeError) : ''}</p>{/if}
</main>

<style>
  .floating-review {
    --ink: #fff;
    --muted: #a3a3a3;
    display: flex;
    flex-direction: column;
    height: 100svh;
    overflow: auto;
    border: 1px solid var(--line);
    border-radius: 15px;
    background: #000;
    color: #fff;
  }
  header {
    margin: 24px 24px 0;
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
  }
  header span {
    font-size: 8px;
    font-weight: 600;
    letter-spacing: 1.5px;
  }
  header button {
    width: 30px;
    height: 30px;
    border: 0;
    border-radius: 6px;
    background: transparent;
    color: var(--muted);
  }
  header button:hover {
    background: var(--hover);
    color: #fff;
  }
  .status {
    padding: 24px;
    display: flex;
    flex: 1;
    flex-direction: column;
    justify-content: center;
    align-items: center;
    text-align: center;
    min-height: 0;
    overflow: auto;
  }
  h1 {
    font:
      26px/1.3 Georgia,
      serif;
  }
  p {
    font-size: 12px;
    line-height: 1.8;
  }
  .retry {
    padding: 11px 16px;
    border: 1px solid var(--line);
    border-radius: 6px;
    background: #fff;
    color: #000;
  }
  .retry:hover {
    border-color: var(--line-strong);
    background: #151515;
    color: #fff;
  }
  .error {
    margin: 0 24px 24px;
    padding: 10px;
    border: 1px solid var(--line);
    border-radius: 6px;
  }
</style>
