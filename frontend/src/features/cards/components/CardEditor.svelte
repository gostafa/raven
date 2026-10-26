<script lang="ts">
  import EditorMedia from './EditorMedia.svelte';
  import { contentKindMessageKeys } from './consts';
  import { useLocalization, type MessageKey } from '../../localization';
  const l = useLocalization();
  import { onDestroy, tick } from 'svelte';
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
    onerror: (message: MessageKey | '') => void;
  } = $props();
  const kinds: MediaKind[] = ['image', 'audio', 'video'];
  let root: HTMLFieldSetElement;
  let removeDialog = $state<HTMLDialogElement>();
  let removal = $state<EditorItem | null>(null);
  onDestroy(() => {
    stopMedia(root);
  });

  $effect(() => {
    const dialog = root.closest('dialog');
    const pause = () => {
      stopMedia(root);
    };
    dialog?.addEventListener('close', pause);
    return () => dialog?.removeEventListener('close', pause);
  });

  function canRemove(target: EditorItem | null): boolean {
    return !disabled && !selecting && target !== null;
  }

  async function addFiles(kind: MediaKind) {
    selecting = true;
    try {
      const previous = items.length;
      items = await selectMedia(media, kind, items);
      if (items.length > previous) onerror('');
    } catch {
      onerror('cards.files_could_not_be_selected_try_again');
    } finally {
      selecting = false;
    }
  }
</script>

<fieldset bind:this={root} disabled={disabled || selecting}>
  <legend>{$l.t('cards.what_you_want_to_remember')}</legend>
  <ol aria-label={$l.t('cards.card_content')}>
    {#each items as item, index (item.key)}
      <li>
        <div class="item-header">
          <span>{$l.number(index + 1)} · {$l.t(contentKindMessageKeys[item.kind])}</span>
          <div class="item-actions">
            <button
              type="button"
              aria-label={$l.t('cards.moveUp', { count: index + 1 })}
              disabled={index === 0}
              onclick={() => {
                items = moveItem(items, item.key, -1);
              }}>↑</button
            >
            <button
              type="button"
              aria-label={$l.t('cards.moveDown', { count: index + 1 })}
              disabled={index === items.length - 1}
              onclick={() => {
                items = moveItem(items, item.key, 1);
              }}>↓</button
            >
            <button
              type="button"
              aria-label={$l.t('cards.removeItem', { count: index + 1 })}
              onclick={() => {
                removal = item;
                removeDialog?.showModal();
              }}>{$l.t('cards.remove')}</button
            >
          </div>
        </div>
        <label class="remember-switch">
          <input
            type="checkbox"
            role="switch"
            aria-label={$l.t('cards.rememberItem', { count: index + 1 })}
            checked={item.hint !== true}
            onchange={(event) => {
              item.hint = !event.currentTarget.checked;
            }}
          />
          <span
            >{$l.t('cards.remember_this')}
            <small>{item.hint === true ? $l.t('cards.hint_for_other_items') : $l.t('cards.included_in_reviews')}</small
            ></span
          >
        </label>
        {#if item.kind === 'text'}<textarea
            dir="auto"
            aria-label={$l.t('cards.textNumber', { count: index + 1 })}
            bind:value={item.text}
            rows="3"
            placeholder={$l.t('cards.what_do_you_want_to_remember')}></textarea>
        {:else}
          <EditorMedia {item} />
        {/if}
      </li>
    {/each}
  </ol>
  <div class="add-actions">
    <button
      type="button"
      onclick={() => {
        items = [...items, textDraft('', media.newKey())];
      }}>{$l.t('cards.text')}</button
    >{#each kinds as kind (kind)}<button type="button" onclick={() => addFiles(kind)}
        >+ {kind === 'image'
          ? $l.t('cards.images')
          : kind === 'audio'
            ? $l.t('cards.audio')
            : $l.t('cards.videos')}</button
      >{/each}
  </div>
</fieldset>
<p class="hint">{$l.t('cards.mix_text_and_files_in_any_order_empty_text_blocks_are_skipped')}</p>
{#if selecting}<p class="hint" role="status">{$l.t('cards.choosing_files')}</p>{/if}

<dialog
  bind:this={removeDialog}
  aria-labelledby="remove-item-title"
  onclick={(event) => {
    if (event.target === removeDialog) removeDialog?.close();
  }}
  onclose={() => {
    removal = null;
  }}
>
  <h2 id="remove-item-title">{$l.t('cards.remove_item')}</h2>
  <p>{$l.t('cards.removeDescription')}</p>
  <div class="dialog-actions">
    <button type="button" onclick={() => removeDialog?.close()}>{$l.t('cards.cancel')}</button>
    <button
      type="button"
      class="confirm-button"
      disabled={disabled || selecting || !removal}
      onclick={async () => {
        const target = removal;
        if (!canRemove(target)) return;
        items = items.filter((candidate) => candidate.key !== target?.key);
        removeDialog?.close();
        await tick();
        root?.querySelector<HTMLButtonElement>('button:not(:disabled)')?.focus();
      }}>{$l.t('cards.remove_item')}</button
    >
  </div>
</dialog>

<style>
  dialog {
    width: min(500px, calc(100vw - 36px));
    max-height: calc(100svh - 36px);
    box-sizing: border-box;
    margin: auto;
    padding: 28px;
    overflow-y: auto;
    border: 1px solid var(--line);
    border-radius: 15px;
    background: #000;
    color: #fff;
    box-shadow: 0 30px 100px #00000055;
  }
  dialog::backdrop {
    -webkit-backdrop-filter: blur(8px);
    backdrop-filter: blur(8px);
    background: transparent;
  }
  dialog h2 {
    margin: 0 0 10px;
    font:
      28px/1.2 Georgia,
      serif;
  }
  dialog p {
    font-size: 12px;
    line-height: 1.8;
  }
  .dialog-actions {
    display: flex;
    justify-content: flex-end;
    gap: 9px;
    margin-top: 25px;
  }
  .dialog-actions button {
    padding: 11px 16px;
  }
  .confirm-button {
    background: #fff;
    color: #000;
  }
  .confirm-button:hover {
    color: #fff;
  }
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

  .hint {
    font-size: 10px;
    color: var(--muted);
    line-height: 1.6;
  }
  :global([dir='rtl']) .remember-switch input:checked::before {
    transform: translateX(-12px);
  }
</style>
