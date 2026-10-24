<script lang="ts">
  import { tick } from 'svelte';
  import { useLocalization } from '../../localization';
  import {
    CardContent,
    contentKindMessageKeys,
    type Card,
    type MediaURLPort,
    type PreviewStartPort,
  } from '../../cards';
  import { stopMedia } from '../../ui';
  import type { ReviewItem } from '../domain/types';
  const l = useLocalization();
  let {
    entry,
    mediaURL,
    nativeStart,
    disabled,
  }: {
    entry: ReviewItem;
    mediaURL: MediaURLPort;
    nativeStart?: PreviewStartPort;
    disabled: boolean;
  } = $props();
  let hintDialog = $state<HTMLDialogElement>();
  let hintsVisible = $state(false);
  let selectedHint = $state<Card['items'][number]>();
  const hints = $derived(entry.card.items.filter((item) => item.id !== entry.item.id));
  async function showHint(hint: Card['items'][number]) {
    selectedHint = hint;
    await tick();
    hintDialog?.showModal();
  }
</script>

{#if hints.length}
  <button
    class="hint-button"
    aria-label={hintsVisible ? $l.t('reviews.hide_hints') : $l.t('reviews.show_hints')}
    title={hintsVisible ? $l.t('reviews.hide_hints') : $l.t('reviews.show_hints')}
    aria-expanded={hintsVisible}
    {disabled}
    onclick={() => (hintsVisible = !hintsVisible)}
  >
    <svg
      class="hint-icon"
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
      <path d="M9 18h6M10 22h4M8.1 14.5a6 6 0 1 1 7.8 0c-.9.7-.9 1.5-.9 2.5H9c0-1-.1-1.8-.9-2.5Z" />
    </svg>
  </button>
  {#if hintsVisible}
    <div class="hint-grid" role="group" aria-label={$l.t('reviews.choose_a_hint')}>
      {#each hints as hint, hintIndex (hint.id)}
        <button
          class="hint-tile"
          aria-label={$l.t('reviews.openHint', {
            count: hintIndex + 1,
            name:
              hint.kind === 'text' ? (hint.text ?? '') : hint.originalName || $l.t(contentKindMessageKeys[hint.kind]),
          })}
          aria-haspopup="dialog"
          {disabled}
          onclick={() => showHint(hint)}
        >
          {#if hint.kind === 'image'}
            <img src={mediaURL(hint)} alt={hint.originalName || $l.t('reviews.hintNumber', { count: hintIndex + 1 })} />
          {:else if hint.kind === 'text'}
            <span class="hint-preview">{hint.text}</span>
          {:else}
            <span class="hint-media-icon" aria-hidden="true">{hint.kind === 'audio' ? '♪' : '▶'}</span>
            <bdi class="hint-media-name">{hint.originalName || $l.t(contentKindMessageKeys[hint.kind])}</bdi>
          {/if}
        </button>
      {/each}
    </div>
  {/if}
  <dialog
    class="hint-dialog"
    aria-label={$l.t('reviews.hint')}
    bind:this={hintDialog}
    onclose={() => {
      if (hintDialog) stopMedia(hintDialog);
    }}
    onkeydown={(event) => {
      if (event.key === 'Escape') event.stopPropagation();
    }}
  >
    <header>
      <h2 class="hint-title">{$l.t('reviews.hint')}</h2>
      <button class="close-button" aria-label={$l.t('reviews.close_hint')} onclick={() => hintDialog?.close()}>✕</button
      >
    </header>
    <div class="hint-content">
      {#if selectedHint}
        <CardContent card={{ ...entry.card, items: [selectedHint] }} {mediaURL} {nativeStart} reviewing />
      {/if}
    </div>
  </dialog>
{/if}

<style>
  header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    flex-shrink: 0;
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

  .hint-button {
    position: absolute;
    top: 62px;
    inset-inline-end: 21px;
    z-index: 1;
    display: grid;
    place-items: center;
    width: 36px;
    height: 36px;
    padding: 0;
    border: 1px solid var(--line);
    border-radius: 6px;
    background: #000;
    color: var(--muted);
  }
  .hint-button:hover {
    border-color: var(--line-strong);
    background: #151515;
    color: var(--ink);
  }
  .hint-grid {
    position: absolute;
    top: 106px;
    inset-inline-end: 21px;
    z-index: 2;
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: 6px;
    width: min(252px, calc(100% - 42px));
    max-height: calc(100% - 130px);
    overflow: auto;
    padding: 8px;
    box-sizing: border-box;
    border: 1px solid var(--line);
    border-radius: 8px;
    background: #000;
  }
  .hint-tile {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    aspect-ratio: 1;
    min-width: 0;
    overflow: hidden;
    padding: 8px;
    border: 1px solid var(--line);
    border-radius: 6px;
    background: #000;
    color: #fff;
  }
  .hint-tile:hover {
    border-color: var(--line-strong);
    background: #151515;
  }
  .hint-tile img {
    width: 100%;
    height: 100%;
    object-fit: cover;
  }
  .hint-preview {
    display: -webkit-box;
    -webkit-box-orient: vertical;
    -webkit-line-clamp: 3;
    line-clamp: 3;
    overflow: hidden;
    overflow-wrap: anywhere;
    font-size: 10px;
    line-height: 1.5;
  }
  .hint-media-icon {
    font-size: 20px;
  }
  .hint-media-name {
    width: 100%;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: 9px;
    margin-top: 5px;
  }
  .hint-dialog {
    box-sizing: border-box;
    width: min(720px, calc(100vw - 32px));
    max-height: calc(100svh - 32px);
    padding: 20px;
    border: 1px solid var(--line);
    border-radius: 12px;
    background: #000;
    color: #fff;
    overflow: auto;
  }
  .hint-dialog::backdrop {
    background: #000b;
  }
  .hint-dialog header {
    margin-bottom: 16px;
  }
  .hint-title {
    font: inherit;
    font-size: 16px;
    font-weight: 600;
    letter-spacing: normal;
  }
  .hint-content {
    border-top: 1px solid var(--line);
    padding-top: 16px;
    overflow-wrap: anywhere;
  }
  .hint-content :global(.content-item > p) {
    font-size: 15px;
    line-height: 1.8;
  }

  button:disabled {
    opacity: 0.5;
    cursor: default;
  }
  button:focus-visible {
    outline: 2px solid var(--muted);
    outline-offset: 3px;
  }

  h2 {
    margin: 0;
    font:
      28px/1.25 Georgia,
      serif;
    letter-spacing: -0.8px;
    overflow-wrap: anywhere;
  }

  @media (prefers-reduced-motion: reduce) {
    * {
      transition: none;
    }
  }
</style>
