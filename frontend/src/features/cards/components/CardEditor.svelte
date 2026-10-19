<script lang="ts">
  import { onDestroy } from 'svelte';
  import { stopMedia } from '../../ui';
  import { moveItem, textDraft } from '../domain/funcs';
  import { selectMedia } from '../application/funcs';
  import type { EditorItem, MediaKind, MediaPort } from '../domain/types';

  let {
    media,
    items = $bindable<EditorItem[]>([]),
    selecting = $bindable(false),
    disabled = false,
    onerror,
  }: {
    media: MediaPort;
    items: EditorItem[];
    selecting?: boolean;
    disabled?: boolean;
    onerror: (message: string) => void;
  } = $props();
  const kinds: MediaKind[] = ['image', 'audio', 'video'];
  let root: HTMLFieldSetElement;
  let failed = $state<string[]>([]);
  onDestroy(() => stopMedia(root));

  $effect(() => {
    const dialog = root?.closest('dialog');
    const pause = () => stopMedia(root);
    dialog?.addEventListener('close', pause);
    return () => dialog?.removeEventListener('close', pause);
  });

  async function addFiles(kind: MediaKind) {
    selecting = true;
    try {
      const previous = items.length;
      items = await selectMedia(media, kind, items);
      if (items.length > previous) onerror('');
    } catch (error) {
      onerror(`Files could not be selected. ${error instanceof Error ? error.message : 'Try again.'}`);
    } finally {
      selecting = false;
    }
  }
</script>

<fieldset bind:this={root} disabled={disabled || selecting}>
  <legend>What you want to remember</legend>
  <ol aria-label="Card content">
    {#each items as item, index (item.key)}
      <li>
        <div class="item-header">
          <span>{index + 1} · {item.kind}</span>
          <div class="item-actions">
            <button
              type="button"
              aria-label={`Move item ${index + 1} up`}
              disabled={index === 0}
              onclick={() => (items = moveItem(items, item.key, -1))}>↑</button
            >
            <button
              type="button"
              aria-label={`Move item ${index + 1} down`}
              disabled={index === items.length - 1}
              onclick={() => (items = moveItem(items, item.key, 1))}>↓</button
            >
            <button
              type="button"
              aria-label={`Remove item ${index + 1}`}
              onclick={() => (items = items.filter((candidate) => candidate.key !== item.key))}>Remove</button
            >
          </div>
        </div>
        <label class="remember-switch">
          <input
            type="checkbox"
            role="switch"
            aria-label={`Remember item ${index + 1}`}
            checked={!item.hint}
            onchange={(event) => (item.hint = !event.currentTarget.checked)}
          />
          <span>Remember this <small>{item.hint ? 'Hint for other items' : 'Included in reviews'}</small></span>
        </label>
        {#if item.kind === 'text'}<textarea
            aria-label={`Card text ${index + 1}`}
            bind:value={item.text}
            rows="3"
            placeholder="What do you want to remember?"></textarea>
        {:else}
          {#if failed.includes(item.key)}<p class="hint" role="status">
              This file could not be loaded or played. Check that it is available and its format is supported on this
              device.
            </p>
          {:else if item.previewURL}
            {#if item.kind === 'image'}<img
                src={item.previewURL}
                alt={item.originalName || 'Card image'}
                onerror={() => (failed = [...failed, item.key])}
              />
            {:else if item.kind === 'audio'}<audio
                controls
                preload="metadata"
                src={item.previewURL}
                aria-label={item.originalName}
                onerror={() => (failed = [...failed, item.key])}
              ></audio>
            {:else if item.kind === 'video'}
              <!-- svelte-ignore a11y_media_has_caption (Imported videos do not include separate caption files.) -->
              <video
                controls
                playsinline
                preload="metadata"
                src={item.previewURL}
                aria-label={item.originalName}
                onerror={() => (failed = [...failed, item.key])}
              ></video>
            {/if}
          {/if}
          <p class="filename">
            {item.originalName}<span>{item.id ? 'Saved file' : 'Will be copied when you save'}</span>
          </p>
        {/if}
      </li>
    {/each}
  </ol>
  <div class="add-actions">
    <button type="button" onclick={() => (items = [...items, textDraft('', media.newKey())])}>+ Text</button
    >{#each kinds as kind (kind)}<button type="button" onclick={() => addFiles(kind)}
        >+ {kind === 'image' ? 'Images' : kind === 'audio' ? 'Audio' : 'Videos'}</button
      >{/each}
  </div>
</fieldset>
<p class="hint">Mix text and files in any order. Empty text blocks are skipped.</p>
{#if selecting}<p class="hint" role="status">Choosing files…</p>{/if}

<style>
  .remember-switch {
    display: flex;
    align-items: center;
    gap: 9px;
    margin-bottom: 10px;
    font-size: 12px;
    cursor: pointer;
  }
  .remember-switch input {
    appearance: none;
    width: 30px;
    height: 18px;
    margin: 0;
    border: 1px solid var(--line-strong);
    border-radius: 12px;
    background: #181818;
    cursor: pointer;
  }
  .remember-switch input::before {
    content: '';
    display: block;
    width: 12px;
    height: 12px;
    margin: 2px;
    border-radius: 50%;
    background: #a3a3a3;
  }
  .remember-switch input:checked {
    background: #fff;
  }
  .remember-switch input:checked::before {
    background: #000;
    transform: translateX(12px);
  }
  .remember-switch input:focus-visible {
    outline: 2px solid var(--muted);
    outline-offset: 3px;
  }
  .remember-switch small {
    display: block;
    color: var(--muted);
    font-size: 10px;
    margin-top: 3px;
  }
  fieldset {
    border: 1px solid var(--line);
    border-radius: 8px;
    margin: 20px 0 0;
    padding: 14px;
    min-width: 0;
  }
  legend {
    padding: 0 6px;
    font-size: 12px;
    color: var(--ink);
  }
  ol {
    list-style: none;
    margin: 0;
    padding: 0;
  }
  li {
    border-bottom: 1px solid var(--line);
    padding: 0 0 12px;
    margin-bottom: 12px;
  }
  .item-header,
  .item-actions,
  .add-actions {
    display: flex;
    align-items: center;
    gap: 7px;
    flex-wrap: wrap;
  }
  .item-header {
    justify-content: space-between;
    margin-bottom: 8px;
    color: var(--muted);
    font-size: 10px;
  }
  button {
    color: var(--ink);
    background: #000;
    border: 1px solid var(--line);
    border-radius: 5px;
    padding: 6px 8px;
    font: inherit;
    font-size: 11px;
    cursor: pointer;
  }
  button:hover {
    border-color: var(--line-strong);
    background: #151515;
  }
  button:disabled,
  fieldset:disabled button {
    opacity: 0.5;
    cursor: default;
  }
  textarea {
    width: 100%;
    padding: 10px;
    resize: vertical;
    border: 1px solid var(--line);
    border-radius: 6px;
    color: #fff;
    background: #000;
    font: inherit;
    font-size: 12px;
    line-height: 1.6;
  }
  button:focus-visible,
  textarea:focus-visible {
    outline: none;
  }
  .filename {
    overflow-wrap: anywhere;
    margin: 0;
    color: var(--ink);
    font-size: 12px;
    line-height: 1.6;
  }
  img,
  video {
    display: block;
    width: 100%;
    max-height: 320px;
    object-fit: contain;
    border-radius: 6px;
    background: #000;
    margin-bottom: 8px;
  }
  audio {
    display: block;
    width: 100%;
    margin-bottom: 8px;
  }
  .filename span {
    display: block;
    font-size: 10px;
    color: var(--muted);
    margin-top: 3px;
  }
  .hint {
    font-size: 10px;
    color: var(--muted);
    line-height: 1.6;
  }
</style>
