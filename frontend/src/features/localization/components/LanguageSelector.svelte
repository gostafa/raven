<script lang="ts">
  import { useLocalization } from '../adapters/svelte/funcs';
  import type { LanguagePreference } from '../domain/types';
  let { compact = false }: { compact?: boolean } = $props();
  const l = useLocalization();
  let saving = $state(false);
  let failed = $state(false);
  const currentLanguage = $derived(
    $l.preference === 'system' ? $l.t('common.system') : $l.preference === 'ar' ? 'العربية' : 'English',
  );
  const description = $derived($l.t('common.choice', { label: $l.t('common.language'), value: currentLanguage }));

  async function change(event: Event) {
    const select = event.currentTarget as HTMLSelectElement;
    saving = true;
    failed = false;
    try {
      await l.change(select.value as LanguagePreference);
    } catch {
      select.value = $l.preference;
      failed = true;
    } finally {
      saving = false;
    }
  }
</script>

<div class="language-control" class:compact>
  <label title={compact ? undefined : description} data-tooltip={description} class:saving>
    <svg
      width="18"
      height="18"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      stroke-width="1.5"
      aria-hidden="true"
    >
      <circle cx="12" cy="12" r="9" /><ellipse cx="12" cy="12" rx="4" ry="9" /><path d="M3 12h18" />
    </svg>
    <span>{$l.t('common.language')}</span>
    <select
      aria-label={description}
      aria-busy={saving}
      value={$l.preference}
      disabled={saving}
      onchange={(event) => void change(event)}
    >
      <option value="system">{$l.t('common.system')}</option>
      <option value="en" lang="en">English</option>
      <option value="ar" lang="ar">العربية</option>
    </select>
  </label>
  {#if failed}<p role="alert">{$l.t('common.language_could_not_be_saved_try_again')}</p>{/if}
</div>

<style>
  .language-control {
    position: relative;
    margin-block-start: 12px;
    font-size: 11px;
    color: var(--muted);
  }
  label {
    position: relative;
    display: flex;
    align-items: center;
    gap: 8px;
    min-height: 36px;
    border: 1px solid var(--line);
    border-radius: 7px;
    padding: 6px 9px;
  }
  label:focus-within {
    outline: 2px solid var(--focus);
    outline-offset: 2px;
  }
  svg {
    flex-shrink: 0;
  }
  select {
    min-width: 0;
    flex: 1;
    color: var(--ink);
    background: #181818;
    border: 0;
    font: inherit;
    padding: 4px;
    cursor: pointer;
  }
  select:focus-visible {
    outline: none;
  }
  p {
    line-height: 1.6;
  }
  .compact label {
    width: 40px;
    height: 40px;
    justify-content: center;
    padding: 0;
    border-color: transparent;
    border-radius: 8px;
    cursor: pointer;
    transition:
      background 0.18s,
      color 0.18s,
      border-color 0.18s;
  }
  .compact {
    margin: 0;
    flex-shrink: 0;
  }
  .compact label:hover:not(.saving),
  .compact label:focus-within {
    border-color: var(--line-strong);
    background: var(--hover);
    color: var(--ink);
  }
  .compact label:active:not(.saving) {
    background: var(--selected);
  }
  .saving {
    opacity: 0.5;
    cursor: default;
  }
  .compact label::after {
    content: attr(data-tooltip);
    position: absolute;
    z-index: 6;
    inset-block-end: calc(100% + 8px);
    inset-inline-start: var(--language-tip-start, auto);
    inset-inline-end: var(--language-tip-end, 0);
    width: max-content;
    max-width: 180px;
    padding: 7px 9px;
    border: 1px solid var(--line);
    border-radius: 6px;
    background: var(--paper);
    color: var(--ink);
    font-size: 11px;
    line-height: 1.4;
    pointer-events: none;
    visibility: hidden;
  }
  .compact label:hover::after,
  .compact label:focus-within::after {
    visibility: visible;
  }
  .compact p {
    position: absolute;
    z-index: 7;
    inset-block-end: calc(100% + 8px);
    inset-inline-start: var(--language-tip-start, auto);
    inset-inline-end: var(--language-tip-end, 0);
    width: 180px;
    margin: 0;
    padding: 10px;
    border: 1px solid var(--line-strong);
    border-radius: 8px;
    background: var(--paper);
    color: var(--ink);
  }
  .compact span {
    display: none;
  }
  .compact select {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    opacity: 0;
  }
  @media (pointer: coarse) {
    .compact label {
      width: 44px;
      height: 44px;
    }
  }
  @media (prefers-reduced-motion: reduce) {
    .compact label {
      transition: none;
    }
  }
</style>
