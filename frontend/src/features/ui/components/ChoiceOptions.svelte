<script lang="ts" generics="Value extends string">
  import { useLocalization } from '../../localization';
  import type { ChoiceOption } from '../types';
  const l = useLocalization();
  let {
    label,
    visibleOptions,
    selected,
    multiple,
    search,
    onselect,
  }: {
    label: string;
    visibleOptions: ChoiceOption<Value>[];
    selected: Value[];
    multiple: boolean;
    search: string;
    onselect: (value: Value) => void;
  } = $props();
</script>

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

<style>
  button {
    font: inherit;
    cursor: pointer;
  }

  button:disabled {
    opacity: 0.4;
    cursor: default;
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

  @media (pointer: coarse) {
    .choices button {
      min-height: 44px;
    }
  }
</style>
