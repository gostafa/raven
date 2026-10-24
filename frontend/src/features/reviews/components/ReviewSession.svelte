<script lang="ts">
  import ReviewHints from './ReviewHints.svelte';

  import { useLocalization, type MessageKey } from '../../localization';
  const l = useLocalization();
  import { onDestroy, onMount, tick } from 'svelte';
  import type { Card, MediaURLPort, PreviewStartPort } from '../../cards';
  import type { Folder } from '../../folders';
  import type { ReviewPort } from '../domain/types';
  import { saveReview } from '../application/funcs';
  import { CardContent } from '../../cards';
  import { stopMedia } from '../../ui';
  import type { ReviewItem } from '../domain/types';

  let {
    review,
    mediaURL,
    nativeStart,
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
    nativeStart?: PreviewStartPort;
    entries: ReviewItem[];
    folders: Pick<Folder, 'id' | 'title'>[];
    floating?: boolean;
    disabled?: boolean;
    saving?: boolean;
    onsaved?: (card: Card) => void;
    onclose: () => void;
  } = $props();
  let index = $state(0);
  let error = $state<MessageKey | ''>('');
  let root: HTMLDivElement;
  let ratingButton = $state<HTMLButtonElement>();
  let completionButton = $state<HTMLButtonElement>();
  const entry = $derived(entries.at(index));
  const card = $derived(entry ? { ...entry.card, items: [entry.item] } : undefined);
  const mediaReview = $derived(entry?.item.kind === 'image' || entry?.item.kind === 'video');

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
      error = 'reviews.your_review_could_not_be_saved_try_again_before_moving_on';
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

<div class="review-session" class:floating class:media-review={mediaReview} bind:this={root}>
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
        <span>{folders.find((folder) => folder.id === card.folderId)?.title ?? $l.t('workspace.folder')}</span>
      {:else}{$l.t('reviews.review_complete')}{/if}
    </span><button
      class="close-button"
      aria-label={$l.t('reviews.close_review')}
      disabled={saving || disabled}
      onclick={close}>✕</button
    >
  </header>
  {#if card && entry}
    <div class="review-content">
      {#key `${card.folderId}/${card.id}/${entry.item.id}`}
        <div class="review-item">
          <CardContent {card} {mediaURL} {nativeStart} reviewing />
        </div>
        <ReviewHints {entry} {mediaURL} {nativeStart} disabled={saving || disabled} />
      {/key}
    </div>
    <footer>
      <p class="review-question">{$l.t('reviews.how_familiar_does_this_feel')}</p>
      <div class="rating-actions">
        <button class="secondary-button" bind:this={ratingButton} disabled={saving || disabled} onclick={() => rate(1)}
          >{$l.t('reviews.again')}</button
        >
        <button class="secondary-button" disabled={saving || disabled} onclick={() => rate(2)}
          >{$l.t('reviews.hard')}</button
        >
        <button class="primary-button" disabled={saving || disabled} onclick={() => rate(3)}
          >{$l.t('reviews.good')}</button
        >
        <button class="secondary-button" disabled={saving || disabled} onclick={() => rate(4)}
          >{$l.t('reviews.easy')}</button
        >
      </div>
      {#if error}<p class="error" role="alert">{error ? $l.t(error) : ''}</p>{/if}
    </footer>
  {:else}
    <div class="review-complete">
      <div class="complete-icon" aria-hidden="true">✓</div>
      <p class="eyebrow">{$l.t('reviews.a_little_more_remembered')}</p>
      <h2>{index ? $l.t('reviews.look_at_you_showing_up') : $l.t('reviews.all_caught_up')}</h2>
      <p>
        {#if index}{$l.t('reviews.practiced', { items: $l.t('common.items', { count: index }) })}{:else}{$l.t(
            'reviews.there_s_nothing_due_right_now_your_next_reviews_will_appear_here',
          )}{/if}
      </p>
      <button class="primary-button" bind:this={completionButton} {disabled} onclick={close}
        >{floating ? $l.t('reviews.done') : $l.t('reviews.back_to_your_space')}</button
      >
    </div>
  {/if}
</div>

<style>
  .review-session {
    --ink: #fff;
    --muted: #a3a3a3;
    position: relative;
    color: var(--ink);
    display: flex;
    flex-direction: column;
    min-height: 0;
    height: 100%;
    gap: 16px;
    padding: 24px;
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
  .review-content {
    display: flex;
    flex-direction: column;
    flex: 1;
    gap: 16px;
    padding: 12px 0;
    min-height: 0;
    overflow-wrap: anywhere;
  }
  .review-item {
    container-type: size;
    display: flex;
    flex-direction: column;
    justify-content: safe center;
    flex: 1;
    min-height: 0;
    overflow: auto;
  }
  .review-item :global(.card-content) {
    flex-shrink: 0;
  }
  .review-content :global(.card-content) {
    width: 100%;
  }
  .review-content :global(.content-item > p) {
    font-size: 18px;
    line-height: 1.8;
  }
  .media-review {
    gap: 12px;
    padding: 16px;
    background: #000;
  }
  .media-review .review-content {
    padding: 0;
  }
  .media-review .review-item {
    overflow: hidden;
  }
  .media-review .review-item :global(.card-content),
  .media-review .review-item :global(.content-item),
  .media-review .review-item :global(.image-preview) {
    height: 100%;
    min-height: 0;
  }
  .media-review .review-item :global(img),
  .media-review .review-item :global(video) {
    width: 100%;
    height: 100%;
    max-height: none;
    object-fit: contain;
    border-radius: 0;
  }
  .media-review footer {
    padding-top: 12px;
    border-top: 1px solid var(--line);
  }
  .media-review .review-question {
    margin-bottom: 10px;
  }

  footer {
    flex-shrink: 0;
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
    * {
      transition: none;
    }
  }
</style>
