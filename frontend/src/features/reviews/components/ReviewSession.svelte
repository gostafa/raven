<script lang="ts">
  import { onDestroy, onMount, tick } from 'svelte';
  import type { Card, MediaURLPort } from '../../cards';
  import type { Folder } from '../../folders';
  import type { ReviewPort } from '../domain/types';
  import { saveReview } from '../application/funcs';
  import { CardContent } from '../../cards';
  import { stopMedia } from '../../ui';
  import type { ReviewItem } from '../domain/types';

  let {
    review,
    mediaURL,
    entries,
    folders,
    floating = false,
    disabled = false,
    saving = $bindable(false),
    onsaved,
    onclose,
  }: {
    review: ReviewPort;
    mediaURL: MediaURLPort;
    entries: ReviewItem[];
    folders: Pick<Folder, 'id' | 'title'>[];
    floating?: boolean;
    disabled?: boolean;
    saving?: boolean;
    onsaved?: (card: Card) => void;
    onclose: () => void;
  } = $props();
  let index = $state(0);
  let error = $state('');
  let root: HTMLDivElement;
  let ratingButton = $state<HTMLButtonElement>();
  let completionButton = $state<HTMLButtonElement>();
  const entry = $derived(entries.at(index));
  const card = $derived(entry ? { ...entry.card, items: [entry.item] } : undefined);

  $effect(() => {
    if (entry || saving || disabled) return;
    const timer = window.setTimeout(close, 5000);
    return () => window.clearTimeout(timer);
  });

  function close() {
    if (saving || disabled) return;
    stopMedia(root);
    onclose();
  }

  async function rate(rating: number) {
    if (!entry || saving || disabled) return;
    saving = true;
    error = '';
    try {
      const updated = await saveReview(review, entry, rating);
      onsaved?.(updated);
      stopMedia(root);
      index += 1;
    } catch {
      error = 'Your review could not be saved. Try again before moving on.';
    } finally {
      saving = false;
      await tick();
      (ratingButton ?? completionButton)?.focus();
    }
  }

  onMount(() => {
    (ratingButton ?? completionButton)?.focus();
  });
  onDestroy(() => stopMedia(root));
</script>

<div class="review-session" class:floating bind:this={root}>
  <header>
    <span class="review-folder">
      {#if card}
        <svg
          width="16"
          height="16"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="1.7"
          stroke-linecap="round"
          stroke-linejoin="round"
          aria-hidden="true"
        >
          <path d="M3 7a2 2 0 0 1 2-2h5l2 2h7a2 2 0 0 1 2 2v10a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2Z" />
        </svg>
        <span>{folders.find((folder) => folder.id === card.folderId)?.title ?? 'Folder'}</span>
      {:else}Review complete{/if}
    </span><button class="close-button" aria-label="Close review" disabled={saving || disabled} onclick={close}
      >✕</button
    >
  </header>
  {#if card && entry}
    <div
      class="review-progress"
      role="progressbar"
      aria-label="Review progress"
      aria-valuenow={index}
      aria-valuemin="0"
      aria-valuemax={entries.length}
    >
      <span style={`width: ${(index / entries.length) * 100}%`}></span>
    </div>
    <div class="review-meta">
      <span>RECALL</span><span>{index + 1} / {entries.length}</span>
    </div>
    <div class="review-content">
      <p class="eyebrow" id="review-title">WHAT YOU WANT TO REMEMBER</p>
      {#key `${card.folderId}/${card.id}/${entry.item.id}`}<CardContent {card} {mediaURL} />{/key}
    </div>
    <footer>
      <p class="review-question">How familiar does this feel?</p>
      <div class="rating-actions">
        <button class="secondary-button" bind:this={ratingButton} disabled={saving || disabled} onclick={() => rate(1)}
          >Again</button
        >
        <button class="secondary-button" disabled={saving || disabled} onclick={() => rate(2)}>Hard</button>
        <button class="primary-button" disabled={saving || disabled} onclick={() => rate(3)}>Good</button>
        <button class="secondary-button" disabled={saving || disabled} onclick={() => rate(4)}>Easy</button>
      </div>
      {#if error}<p class="error" role="alert">{error}</p>{/if}
    </footer>
  {:else}
    <div class="review-complete">
      <div class="complete-icon" aria-hidden="true">✓</div>
      <p class="eyebrow">A LITTLE MORE REMEMBERED</p>
      <h2 id="review-title">{index ? 'Look at you, showing up.' : 'All caught up'}</h2>
      <p>
        {#if index}You practiced {index}
          {index === 1 ? 'item' : 'items'}. Your next reviews are scheduled.{:else}There’s nothing due right now. Your
          next reviews will appear here.{/if}
      </p>
      <button class="primary-button" bind:this={completionButton} {disabled} onclick={close}
        >{floating ? 'Done' : 'Back to your space'}</button
      >
    </div>
  {/if}
</div>

<style>
  .review-session {
    --ink: #fff;
    --muted: #a3a3a3;
    color: var(--ink);
    display: flex;
    flex-direction: column;
    min-height: 0;
    height: 100%;
    gap: 16px;
  }
  .floating {
    height: 100%;
  }
  header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    flex-shrink: 0;
  }
  .review-folder {
    display: flex;
    align-items: center;
    gap: 8px;
    min-width: 0;
    font-size: 11px;
    font-weight: 600;
  }
  .review-folder svg {
    flex-shrink: 0;
  }
  .review-folder > span {
    overflow-wrap: anywhere;
  }
  .close-button {
    display: grid;
    place-items: center;
    width: 30px;
    height: 30px;
    flex-shrink: 0;
    padding: 0;
    border: 0;
    border-radius: 50%;
    background: transparent;
    color: var(--muted);
  }
  .close-button:hover {
    background: var(--hover);
    color: var(--ink);
  }
  .eyebrow {
    margin: 0 0 13px;
    font-size: 8px;
    font-weight: 600;
    letter-spacing: 1.5px;
    line-height: 1.7;
    color: var(--muted);
  }
  .review-progress {
    flex-shrink: 0;
    height: 2px;
    width: 100%;
    border-radius: 5px;
    background: var(--line);
    overflow: hidden;
  }
  .review-progress span {
    display: block;
    height: 100%;
    background: #fff;
    transition: width 0.2s;
  }
  .review-meta {
    display: flex;
    justify-content: space-between;
    gap: 12px;
    font-size: 10px;
    color: var(--muted);
    letter-spacing: 1px;
    flex-shrink: 0;
  }
  .review-meta > span:first-child {
    min-width: 0;
    overflow-wrap: anywhere;
  }
  .review-meta > span:last-child {
    flex-shrink: 0;
  }
  .review-content {
    display: flex;
    flex-direction: column;
    justify-content: center;
    flex: 1;
    padding: 24px 0;
    min-height: 180px;
    overflow-wrap: anywhere;
  }
  .review-content :global(.card-content) {
    width: 100%;
  }
  .review-content :global(.content-item > p) {
    font-size: 18px;
    line-height: 1.8;
  }
  .review-content :global(img),
  .review-content :global(video) {
    height: auto;
    max-height: 360px;
    object-fit: contain;
  }
  footer {
    flex-shrink: 0;
    border-top: 1px solid var(--line);
    padding-top: 20px;
  }
  .review-question {
    text-align: center;
    font-size: 11px;
    margin: 0 0 16px;
    color: var(--muted);
  }
  .rating-actions {
    display: grid;
    grid-template-columns: repeat(4, minmax(0, 1fr));
    gap: 6px;
  }
  .primary-button,
  .secondary-button {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    min-height: 44px;
    padding: 11px 6px;
    border: 1px solid var(--line);
    border-radius: 6px;
    font-size: 11px;
    font-weight: 500;
    line-height: 1.5;
    transition:
      background 0.18s,
      border-color 0.18s;
  }
  .primary-button {
    color: #000;
    border-color: #fff;
    background: #fff;
  }
  .secondary-button {
    background: #000;
    color: #fff;
  }
  .primary-button:hover,
  .secondary-button:hover {
    border-color: var(--line-strong);
    background: #151515;
    color: #fff;
  }
  button:disabled {
    opacity: 0.5;
    cursor: default;
  }
  button:focus-visible {
    outline: 2px solid var(--muted);
    outline-offset: 3px;
  }
  .error {
    padding: 10px;
    margin: 12px 0 0;
    border: 1px solid var(--line);
    border-radius: 6px;
    font-size: 11px;
    line-height: 1.7;
  }
  .review-complete {
    flex: 1;
    min-height: 0;
    overflow: auto;
    padding: 20px 0 8px;
    text-align: center;
  }
  .complete-icon {
    display: grid;
    place-items: center;
    width: 65px;
    height: 65px;
    border: 1px solid var(--line);
    border-radius: 50%;
    font-size: 30px;
    margin: 0 auto 28px;
  }
  h2 {
    margin: 0;
    font:
      28px/1.25 Georgia,
      serif;
    letter-spacing: -0.8px;
    overflow-wrap: anywhere;
  }
  .review-complete > p:not(.eyebrow) {
    margin: 15px 0 28px;
    font-size: 12px;
    line-height: 1.9;
  }
  @media (prefers-reduced-motion: reduce) {
    *,
    .review-progress span {
      transition: none;
    }
  }
</style>
