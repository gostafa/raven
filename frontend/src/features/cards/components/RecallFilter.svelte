<script lang="ts">
  import { useLocalization, parseLocalizedNumber } from '../../localization';
  import { positionPopover } from '../../ui';
  const l = useLocalization();
  import type { CardProgressRange } from '../domain/types';

  let { range, onchange }: { range: CardProgressRange; onchange: (range: CardProgressRange) => void } = $props();
  const id = $props.id();
  let disclosure: HTMLDetailsElement;
  let trigger: HTMLElement;
  let panel: HTMLDivElement;
  let expanded = $state(false);
  const active = $derived(range.minimum > 0 || range.maximum < 100);
  const presets = $derived([
    { label: $l.t('cards.any'), minimum: 0, maximum: 100 },
    { label: $l.t('cards.0_only'), minimum: 0, maximum: 0 },
    { label: $l.t('cards.up_to_50'), minimum: 0, maximum: 50 },
    { label: $l.t('cards.80_and_above'), minimum: 80, maximum: 100 },
  ]);

  function update(bound: 'minimum' | 'maximum', value: number) {
    if (!Number.isFinite(value)) return range;
    const percentage = Math.max(0, Math.min(100, Math.round(value)));
    const next =
      bound === 'minimum'
        ? { minimum: Math.min(percentage, range.maximum), maximum: range.maximum }
        : { minimum: range.minimum, maximum: Math.max(percentage, range.minimum) };
    onchange(next);
    return next;
  }

  function dismiss(event: PointerEvent) {
    if (event.target instanceof Node && !disclosure.contains(event.target)) disclosure.open = false;
  }

  $effect(() => {
    const direction = $l.direction;
    if (expanded && direction) return positionPopover(trigger, panel, 'start');
  });
</script>

<svelte:window
  onpointerdown={dismiss}
  onkeydown={(event) => {
    if (event.key === 'Escape' && disclosure.open) {
      event.preventDefault();
      event.stopPropagation();
      disclosure.open = false;
      trigger.focus();
    }
  }}
/>

<details
  bind:this={disclosure}
  ontoggle={(event) => (expanded = event.currentTarget.open)}
  onfocusout={(event) => {
    if (event.relatedTarget instanceof Node && !event.currentTarget.contains(event.relatedTarget))
      disclosure.open = false;
  }}
>
  <summary bind:this={trigger}>
    <span>{$l.t('cards.recall_percentage')}</span>
    <span class="value"
      >{active
        ? range.minimum === range.maximum
          ? $l.percent(range.minimum)
          : `${$l.percent(range.minimum)}–${$l.percent(range.maximum)}`
        : $l.t('cards.any')}</span
    >
    <span aria-hidden="true">⌄</span>
  </summary>
  <div class="panel" bind:this={panel}>
    <div class="heading">
      <strong>{$l.t('cards.recall_percentage')}</strong><button
        type="button"
        disabled={!active}
        onclick={() => onchange({ minimum: 0, maximum: 100 })}>{$l.t('cards.reset')}</button
      >
    </div>
    <p>{$l.t('cards.choose_any_range_cards_update_as_you_adjust')}</p>
    <div class="presets" aria-label={$l.t('cards.quick_recall_ranges')}>
      {#each presets as preset (preset.label)}
        <button
          type="button"
          aria-pressed={range.minimum === preset.minimum && range.maximum === preset.maximum}
          onclick={() => onchange({ minimum: preset.minimum, maximum: preset.maximum })}>{preset.label}</button
        >
      {/each}
    </div>
    <div class="bounds">
      <div class="bound">
        <label for={`${id}-minimum`}>{$l.t('cards.minimum')}</label>
        <div class="number">
          <input
            id={`${id}-minimum`}
            type="text"
            inputmode="numeric"
            dir="auto"
            value={$l.number(range.minimum, { useGrouping: false })}
            onchange={(event) => {
              event.currentTarget.value = $l.number(
                update('minimum', parseLocalizedNumber(event.currentTarget.value)).minimum,
                { useGrouping: false },
              );
            }}
          /><span>{$l.locale === 'ar' ? '٪' : '%'}</span>
        </div>
        <input
          type="range"
          aria-label={$l.t('cards.minimum_recall_percentage')}
          min="0"
          max={range.maximum}
          step="1"
          value={range.minimum}
          oninput={(event) => update('minimum', event.currentTarget.valueAsNumber)}
        />
      </div>
      <span class="separator" aria-hidden="true">–</span>
      <div class="bound">
        <label for={`${id}-maximum`}>{$l.t('cards.maximum')}</label>
        <div class="number">
          <input
            id={`${id}-maximum`}
            type="text"
            inputmode="numeric"
            dir="auto"
            value={$l.number(range.maximum, { useGrouping: false })}
            onchange={(event) => {
              event.currentTarget.value = $l.number(
                update('maximum', parseLocalizedNumber(event.currentTarget.value)).maximum,
                { useGrouping: false },
              );
            }}
          /><span>{$l.locale === 'ar' ? '٪' : '%'}</span>
        </div>
        <input
          type="range"
          aria-label={$l.t('cards.maximum_recall_percentage')}
          min={range.minimum}
          max="100"
          step="1"
          value={range.maximum}
          oninput={(event) => update('maximum', event.currentTarget.valueAsNumber)}
        />
      </div>
    </div>
    <p class="note">{$l.t('cards.both_limits_are_included_uses_the_percentage_shown_on_each_card')}</p>
  </div>
</details>

<style>
  details {
    position: relative;
    max-width: 100%;
  }
  summary {
    display: flex;
    align-items: center;
    gap: 9px;
    min-height: 38px;
    box-sizing: border-box;
    padding: 8px 11px;
    border: 1px solid var(--line);
    border-radius: 8px;
    background: var(--paper, #000);
    color: var(--ink, #fff);
    font-size: 11px;
    cursor: pointer;
    list-style: none;
  }
  summary::-webkit-details-marker {
    display: none;
  }
  summary:hover,
  details[open] summary {
    border-color: var(--line-strong);
    background: var(--hover);
  }
  .value {
    color: var(--muted);
  }
  .panel {
    position: fixed;
    inset: auto;
    z-index: 10;
    width: min(300px, calc(100vw - 48px));
    padding: 16px;
    box-sizing: border-box;
    border: 1px solid var(--line);
    border-radius: 11px;
    background: #000;
    color: #fff;
    box-shadow: 0 16px 45px #000b;
    font-size: 11px;
  }
  .heading {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 12px;
  }
  .heading strong {
    font-weight: 500;
  }
  p {
    margin: 8px 0 14px;
    color: var(--muted);
    line-height: 1.5;
  }
  button {
    font: inherit;
    cursor: pointer;
    color: inherit;
  }
  .heading button {
    border: 0;
    background: none;
    padding: 4px;
  }
  button:disabled {
    opacity: 0.4;
    cursor: default;
  }
  .presets {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
    margin-bottom: 18px;
  }
  .presets button {
    padding: 7px 9px;
    border: 1px solid var(--line);
    border-radius: 6px;
    background: #000;
    font-size: 10px;
  }
  .presets button:hover {
    border-color: var(--line-strong);
  }
  .presets button[aria-pressed='true'] {
    background: #fff;
    color: #000;
  }
  .bounds {
    display: flex;
    align-items: center;
    gap: 12px;
  }
  .bound {
    flex: 1;
    min-width: 0;
  }
  label {
    display: block;
    margin-bottom: 7px;
    color: var(--muted);
    font-size: 10px;
  }
  .number {
    display: flex;
    align-items: center;
    gap: 4px;
    padding: 8px;
    border: 1px solid var(--line);
    border-radius: 6px;
  }
  .number:focus-within {
    border-color: var(--line-strong);
  }
  .number span,
  .separator {
    color: var(--muted);
  }
  input[type='text'] {
    width: 100%;
    min-width: 0;
    padding: 0;
    border: 0;
    background: #000;
    color: #fff;
    font: inherit;
    font-size: 13px;
  }
  input[type='range'] {
    width: 100%;
    margin: 12px 0 0;
    accent-color: #fff;
    cursor: pointer;
  }
  .note {
    margin: 14px 0 0;
    padding-top: 12px;
    border-top: 1px solid var(--line);
    font-size: 10px;
  }
  summary:focus-visible,
  button:focus-visible,
  input:focus-visible {
    outline: 1px solid var(--line-strong);
    outline-offset: 3px;
  }
  @media (pointer: coarse) {
    summary,
    button,
    .number {
      min-height: 44px;
    }
    input[type='range'] {
      min-height: 24px;
    }
  }
</style>
