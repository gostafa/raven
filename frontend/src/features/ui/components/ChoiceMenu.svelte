<script lang="ts" generics="Value extends string">
  import { onDestroy } from 'svelte';
  import { positionPopover } from '../adapters/positioning/funcs';
  let releasePosition = () => {};
  onDestroy(() => releasePosition());

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
  let trigger: HTMLButtonElement;
  let panel: HTMLDivElement;
  let searchInput = $state<HTMLInputElement>();
  let expanded = $state(false);
  let search = $state('');
  const visibleOptions = $derived(
    options.filter((option) => option.label.toLowerCase().includes(search.trim().toLowerCase())),
  );
  const currentLabel = $derived(options.find((option) => selected.includes(option.value))?.label);

  function close(restoreFocus = false) {
    releasePosition();
    panel.hidePopover();
    if (restoreFocus) trigger.focus();
  }

  function items() {
    return Array.from(panel.querySelectorAll<HTMLButtonElement>('[data-choice]'));
  }

  async function open(last = false, focusChoice = false) {
    search = '';
    await tick();
    panel.showPopover();
    releasePosition();
    releasePosition = positionPopover(trigger, panel, 'start');
    if (searchable && !focusChoice) searchInput?.focus();
    else {
      const buttons = items();
      const checked = buttons.find((button) => button.getAttribute('aria-checked') === 'true');
      (last ? buttons.at(-1) : (checked ?? buttons[0]))?.focus();
    }
  }

  function triggerKey(event: KeyboardEvent) {
    if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
      event.preventDefault();
      void open(event.key === 'ArrowUp', true);
    }
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
    if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) return;
    event.preventDefault();
    const buttons = items();
    const current = buttons.indexOf(document.activeElement as HTMLButtonElement);
    const next =
      event.key === 'Home'
        ? 0
        : event.key === 'End'
          ? buttons.length - 1
          : current < 0
            ? event.key === 'ArrowUp'
              ? buttons.length - 1
              : 0
            : (current + (event.key === 'ArrowDown' ? 1 : -1) + buttons.length) % buttons.length;
    buttons.at(next)?.focus();
  }

  $effect(() => {
    const direction = $l.direction;
    if (!expanded) return;
    if (direction) {
      releasePosition();
      releasePosition = positionPopover(trigger, panel, 'start');
    }
    const dismiss = (event: Event) => {
      if (event.target instanceof Node && panel.contains(event.target)) return;
      close();
    };
    window.addEventListener('scroll', dismiss, true);
    window.addEventListener('resize', dismiss);
    return () => {
      window.removeEventListener('scroll', dismiss, true);
      window.removeEventListener('resize', dismiss);
    };
  });
</script>

<div
  class="choice-menu"
  onfocusout={(event) => {
    if (!event.currentTarget.contains(event.relatedTarget as Node | null)) close();
  }}
>
  <button
    bind:this={trigger}
    type="button"
    class="choice-trigger"
    aria-label={multiple
      ? $l.t('common.selection', { label, selected: $l.t('common.selected', { count: selected.length }) })
      : $l.t('common.choice', { label, value: currentLabel ?? '' })}
    aria-haspopup="dialog"
    aria-expanded={expanded}
    aria-controls={id}
    popovertarget={id}
    {disabled}
    onclick={(event) => {
      event.preventDefault();
      expanded ? close(true) : void open();
    }}
    onkeydown={triggerKey}
  >
    <svg
      width="15"
      height="15"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      stroke-width="1.6"
      stroke-linecap="round"
      stroke-linejoin="round"
      aria-hidden="true"
    >
      {#if multiple}<path d="M3 5v6l9 9 8-8-9-9H5a2 2 0 0 0-2 2Z" /><circle cx="7.5" cy="7.5" r="1" />
      {:else}<path d="M4 6h16M7 12h10M10 18h4" />{/if}
    </svg>
    <span class="trigger-label">{label}</span>
    {#if multiple}
      {#if selected.length}<span class="selection-count">{$l.number(selected.length)}</span>{/if}
    {:else if currentLabel}<span class="current-choice">{currentLabel}</span>{/if}
    <svg
      class="chevron"
      width="12"
      height="12"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      stroke-width="1.8"
      aria-hidden="true"><path d="m6 9 6 6 6-6" /></svg
    >
  </button>
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
          onclick={() => onclear?.()}>{$l.t('ui.clear')}</button
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
    <div class="choices" role="menu" aria-label={label}>
      {#each visibleOptions as option (option.value)}
        <button
          type="button"
          data-choice
          role={multiple ? 'menuitemcheckbox' : 'menuitemradio'}
          aria-checked={selected.includes(option.value)}
          tabindex="-1"
          onclick={() => {
            onselect(option.value);
            if (!multiple) close(true);
          }}
        >
          {#if multiple}<span class="checkbox" class:checked={selected.includes(option.value)} aria-hidden="true">
              {#if selected.includes(option.value)}<svg
                  width="11"
                  height="11"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  stroke-width="2.5"><path d="m5 12 4 4L19 6" /></svg
                >{/if}
            </span>{/if}
          <span class="option-label">{option.label}</span>
          {#if !multiple && selected.includes(option.value)}<svg
              width="15"
              height="15"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              stroke-width="2"
              aria-hidden="true"><path d="m5 12 4 4L19 6" /></svg
            >{/if}
        </button>
      {/each}
    </div>
    {#if !visibleOptions.length}<p class="empty-choices" role="status">
        {search ? $l.t('ui.no_matches_found') : $l.t('ui.no_options_available')}
      </p>{/if}
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
  .choice-trigger {
    display: flex;
    align-items: center;
    gap: 9px;
    max-width: 100%;
    min-height: 38px;
    padding: 8px 11px;
    border: 1px solid var(--line);
    border-radius: 8px;
    background: var(--paper);
    color: var(--ink);
    font-size: 11px;
  }
  .choice-trigger:hover,
  .choice-trigger[aria-expanded='true'] {
    border-color: var(--line-strong);
    background: var(--hover);
  }
  .choice-trigger > svg {
    flex-shrink: 0;
    color: var(--muted);
  }
  .trigger-label {
    font-weight: 500;
  }
  .current-choice {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: var(--muted);
  }
  .selection-count {
    display: grid;
    place-items: center;
    min-width: 18px;
    height: 18px;
    padding: 0 4px;
    border-radius: 4px;
    background: #fff;
    color: #000;
    font-size: 10px;
  }
  .chevron {
    margin-inline-start: 3px;
    transition: transform 0.15s;
  }
  [aria-expanded='true'] .chevron {
    transform: rotate(180deg);
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
  .choices {
    min-height: 0;
    overflow-y: auto;
    overscroll-behavior: contain;
  }
  .choices button {
    display: flex;
    align-items: center;
    gap: 10px;
    width: 100%;
    min-height: 38px;
    padding: 9px;
    border: 0;
    border-radius: 5px;
    background: transparent;
    color: inherit;
    text-align: start;
    font-size: 11px;
  }
  .choices button:hover,
  .choices button:focus-visible {
    background: var(--selected);
  }
  .choices button[aria-checked='true'] {
    color: #fff;
  }
  .option-label {
    flex: 1;
    min-width: 0;
    overflow-wrap: anywhere;
  }
  .checkbox {
    display: grid;
    place-items: center;
    width: 15px;
    height: 15px;
    flex-shrink: 0;
    border: 1px solid var(--line-strong);
    border-radius: 4px;
  }
  .checkbox.checked {
    background: #fff;
    color: #000;
    border-color: #fff;
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
  .empty-choices {
    padding: 17px 9px;
    margin: 0;
    color: var(--muted);
    font-size: 11px;
  }
  button:focus-visible {
    outline: 1px solid var(--focus, #bdbdbd);
    outline-offset: -2px;
  }
  .choice-trigger:focus-visible,
  .clear-button:focus-visible {
    outline-offset: 3px;
  }
  @media (pointer: coarse) {
    .choice-trigger,
    .choices button,
    .clear-button {
      min-height: 44px;
    }
  }
  @media (prefers-reduced-motion: reduce) {
    .chevron {
      transition: none;
    }
  }
</style>
