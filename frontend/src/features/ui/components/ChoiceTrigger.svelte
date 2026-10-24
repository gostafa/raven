<script lang="ts">
  import { useLocalization } from '../../localization';
  const l = useLocalization();
  let {
    trigger = $bindable(),
    label,
    currentLabel,
    multiple,
    selectedCount,
    expanded,
    id,
    disabled,
    onclick,
    onkeydown,
  }: {
    trigger?: HTMLButtonElement;
    label: string;
    currentLabel?: string;
    multiple: boolean;
    selectedCount: number;
    expanded: boolean;
    id: string;
    disabled: boolean;
    onclick: (event: MouseEvent) => void;
    onkeydown: (event: KeyboardEvent) => void;
  } = $props();
</script>

<button
  bind:this={trigger}
  type="button"
  class="choice-trigger"
  aria-label={multiple
    ? $l.t('common.selection', { label, selected: $l.t('common.selected', { count: selectedCount }) })
    : $l.t('common.choice', { label, value: currentLabel ?? '' })}
  aria-haspopup="dialog"
  aria-expanded={expanded}
  aria-controls={id}
  popovertarget={id}
  {disabled}
  {onclick}
  {onkeydown}
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
    {#if selectedCount}<span class="selection-count">{$l.number(selectedCount)}</span>{/if}
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

<style>
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

  button:disabled {
    opacity: 0.4;
    cursor: default;
  }

  button:focus-visible {
    outline: 1px solid var(--focus, #bdbdbd);
    outline-offset: -2px;
  }
  .choice-trigger:focus-visible {
    outline-offset: 3px;
  }
  @media (pointer: coarse) {
    .choice-trigger {
      min-height: 44px;
    }
  }
  @media (prefers-reduced-motion: reduce) {
    .chevron {
      transition: none;
    }
  }
</style>
