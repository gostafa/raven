<script lang="ts" generics="Value extends string">
  import ChoiceOptions from './ChoiceOptions.svelte';
  import ChoiceTrigger from './ChoiceTrigger.svelte';
  import { trackPopover } from '../adapters/positioning/funcs';
  import { menuFocusIndex } from '../domain/funcs';

  import { useLocalization } from '../../localization';
  const l = useLocalization();
  import { tick } from 'svelte';
  import type { ChoiceOption } from '../types';

  let {
    label,
    options,
    selected,
    onselect,
    multiple = false,
    searchable = false,
    onclear,
    disabled = false,
    hint = '',
  }: {
    label: string;
    options: ChoiceOption<Value>[];
    selected: Value[];
    onselect: (value: Value) => void;
    multiple?: boolean;
    searchable?: boolean;
    onclear?: () => void;
    disabled?: boolean;
    hint?: string;
  } = $props();

  const id = $props.id();
  let trigger = $state<HTMLButtonElement>();
  let panel: HTMLDivElement;
  let searchInput = $state<HTMLInputElement>();
  let expanded = $state(false);
  let search = $state('');
  const visibleOptions = $derived(
    options.filter((option) => option.label.toLowerCase().includes(search.trim().toLowerCase())),
  );
  const currentLabel = $derived(options.find((option) => selected.includes(option.value))?.label);

  function close(restoreFocus = false) {
    panel.hidePopover();
    if (restoreFocus) trigger?.focus();
  }

  function closeAfterFailure() {
    close(true);
  }

  function items() {
    return Array.from(panel.querySelectorAll<HTMLButtonElement>('[data-choice]'));
  }

  function focusOption(last: boolean) {
    const buttons = items();
    const checked = buttons.find((button) => button.getAttribute('aria-checked') === 'true');
    (last ? buttons.at(-1) : (checked ?? buttons[0]))?.focus();
  }
  function focusSearch() {
    searchInput?.focus();
  }
  async function open(last = false, focusChoice = false) {
    search = '';
    await tick();
    panel.showPopover();
    if (searchable && !focusChoice) focusSearch();
    else focusOption(last);
  }

  function triggerKey(event: KeyboardEvent) {
    if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
      event.preventDefault();
      open(event.key === 'ArrowUp', true).catch(closeAfterFailure);
    }
  }

  function focusPanel(event: KeyboardEvent) {
    if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) return;
    event.preventDefault();
    const buttons = items();
    const current = buttons.findIndex((button) => button === document.activeElement);
    const next = menuFocusIndex(event.key, current, buttons.length);
    if (next !== null) buttons.at(next)?.focus();
  }
  function panelKey(event: KeyboardEvent) {
    if (event.key === 'Escape') {
      event.preventDefault();
      event.stopPropagation();
      close(true);
      return;
    }
    const inSearch = event.target === searchInput;
    if (inSearch && !['ArrowDown', 'ArrowUp'].includes(event.key)) return;
    focusPanel(event);
  }

  $effect(() => {
    const direction = $l.direction;
    if (!expanded || direction.length === 0 || !trigger) return;
    return trackPopover(trigger, panel, {
      alignment: 'start',
      dismiss: () => {
        close();
      },
      ignorePanelScroll: true,
    });
  });
</script>

<div
  class="choice-menu"
  onfocusout={(event) => {
    if (!event.currentTarget.contains(event.relatedTarget instanceof Node ? event.relatedTarget : null)) close();
  }}
>
  <ChoiceTrigger
    bind:trigger
    {label}
    {currentLabel}
    {multiple}
    selectedCount={selected.length}
    {expanded}
    {id}
    {disabled}
    onclick={(event: MouseEvent) => {
      event.preventDefault();
      if (expanded) close(true);
      else open().catch(closeAfterFailure);
    }}
    onkeydown={triggerKey}
  />

  <div
    bind:this={panel}
    {id}
    class="choice-panel"
    popover="auto"
    role="dialog"
    aria-label={label}
    tabindex="-1"
    onbeforetoggle={(event) => {
      expanded = event.newState === 'open';
    }}
    onkeydown={panelKey}
  >
    <div class="panel-heading">
      <span>{label}</span>
      {#if multiple && onclear}<button
          class="clear-button"
          type="button"
          disabled={!selected.length}
          onclick={() => {
            onclear?.();
          }}>{$l.t('ui.clear')}</button
        >{/if}
    </div>
    {#if searchable}<div class="choice-search">
        <svg
          width="14"
          height="14"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="1.7"
          aria-hidden="true"><circle cx="10.5" cy="10.5" r="6.5" /><path d="m16 16 4.5 4.5" /></svg
        >
        <input
          bind:this={searchInput}
          bind:value={search}
          type="search"
          aria-label={$l.t('common.search', { name: label })}
          placeholder={$l.t('common.search', { name: label })}
        />
      </div>{/if}
    <ChoiceOptions
      {label}
      {visibleOptions}
      {selected}
      {multiple}
      {search}
      onselect={(value: Value) => {
        onselect(value);
        if (!multiple) close(true);
      }}
    />
    {#if hint}<p class="panel-note">{hint}</p>{/if}
  </div>
</div>

<style>
  .choice-menu {
    display: inline-flex;
    min-width: 0;
    max-width: 100%;
  }
  button {
    font: inherit;
    cursor: pointer;
  }

  .choice-panel {
    position: fixed;
    inset: auto;
    display: none;
    flex-direction: column;
    width: 268px;
    max-width: calc(100vw - 16px);
    max-height: min(390px, calc(100svh - 16px));
    margin: 0;
    padding: 7px;
    box-sizing: border-box;
    border: 1px solid var(--line);
    border-radius: 11px;
    background: var(--paper, #000);
    color: var(--ink, #fff);
    box-shadow: 0 16px 45px #000b;
    font-size: 12px;
  }
  .choice-panel:popover-open {
    display: flex;
  }
  .panel-heading {
    display: flex;
    align-items: center;
    justify-content: space-between;
    min-height: 32px;
    padding: 3px 9px 9px;
    color: var(--muted);
    font-size: 10px;
  }
  .clear-button {
    padding: 4px 0 4px 8px;
    border: 0;
    background: none;
    color: var(--ink);
    font-size: 10px;
  }
  button:disabled {
    opacity: 0.4;
    cursor: default;
  }
  .choice-search {
    display: flex;
    align-items: center;
    gap: 7px;
    flex-shrink: 0;
    margin: 0 3px 7px;
    padding: 7px 9px;
    border: 1px solid var(--line);
    border-radius: 6px;
    color: var(--muted);
  }
  .choice-search input {
    width: 100%;
    min-width: 0;
    border: 0;
    padding: 0;
    outline: none;
    background: transparent;
    color: var(--ink);
    font: inherit;
    font-size: 11px;
  }
  .choice-search:focus-within {
    border-color: var(--line-strong);
    outline: 1px solid var(--line-strong);
  }

  .panel-note {
    flex-shrink: 0;
    margin: 7px 3px 0;
    padding: 10px 6px 4px;
    border-top: 1px solid var(--line);
    color: var(--muted);
    font-size: 10px;
    line-height: 1.5;
  }

  button:focus-visible {
    outline: 1px solid var(--focus, #bdbdbd);
    outline-offset: -2px;
  }
  .clear-button:focus-visible {
    outline-offset: 3px;
  }
  @media (pointer: coarse) {
    .clear-button {
      min-height: 44px;
    }
  }
</style>
