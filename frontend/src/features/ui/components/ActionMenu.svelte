<script lang="ts">
  import { useLocalization } from '../../localization';
  const l = useLocalization();
  import { trackPopover } from '../adapters/positioning/funcs';
  import { menuFocusIndex } from '../domain/funcs';

  let {
    label,
    actions,
    disabled = false,
    appearance = 'default',
  }: {
    label: string;
    actions: { label: string; onselect: (opener: HTMLButtonElement) => void }[];
    disabled?: boolean;
    appearance?: 'default' | 'quiet';
  } = $props();

  const id = $props.id();
  let trigger: HTMLButtonElement;
  let menu: HTMLDivElement;
  let expanded = $state(false);

  function close(restoreFocus = false) {
    menu.hidePopover();
    if (restoreFocus) trigger.focus();
  }

  function items() {
    return Array.from(menu.querySelectorAll<HTMLButtonElement>('[role="menuitem"]'));
  }

  function open(last = false) {
    menu.showPopover();
    const buttons = items();
    buttons[last ? buttons.length - 1 : 0]?.focus();
  }

  function triggerKey(event: KeyboardEvent) {
    if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
      event.preventDefault();
      open(event.key === 'ArrowUp');
    }
  }

  function focusMenu(event: KeyboardEvent) {
    event.preventDefault();
    const buttons = items();
    const current = buttons.findIndex((button) => button === document.activeElement);
    const next = menuFocusIndex(event.key, current, buttons.length);
    if (next !== null) buttons.at(next)?.focus();
  }
  function menuKey(event: KeyboardEvent) {
    if (event.key === 'Escape') {
      event.preventDefault();
      event.stopPropagation();
      close(true);
    } else if (event.key === 'Tab') {
      close(true);
    } else if (['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) {
      focusMenu(event);
    }
  }

  $effect(() => {
    const direction = $l.direction;
    if (!expanded || direction.length === 0) return;
    return trackPopover(trigger, menu, {
      alignment: 'end',
      dismiss: () => {
        close();
      },
    });
  });
</script>

<span
  class="action-menu"
  onfocusout={(event) => {
    if (!event.currentTarget.contains(event.relatedTarget instanceof Node ? event.relatedTarget : null)) close();
  }}
>
  <button
    bind:this={trigger}
    type="button"
    class="menu-trigger"
    class:quiet={appearance === 'quiet'}
    aria-label={label}
    title={label}
    aria-haspopup="menu"
    aria-expanded={expanded}
    aria-controls={id}
    popovertarget={id}
    {disabled}
    onclick={(event) => {
      event.preventDefault();
      if (expanded) close(true);
      else open();
    }}
    onkeydown={triggerKey}
  >
    <svg width="18" height="18" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"
      ><circle cx="5" cy="12" r="1.8" /><circle cx="12" cy="12" r="1.8" /><circle cx="19" cy="12" r="1.8" /></svg
    >
  </button>
  <div
    bind:this={menu}
    {id}
    class="menu"
    popover="auto"
    role="menu"
    aria-label={label}
    tabindex="-1"
    onbeforetoggle={(event) => {
      expanded = event.newState === 'open';
    }}
    onkeydown={menuKey}
  >
    {#each actions as action (action.label)}
      <button
        type="button"
        role="menuitem"
        tabindex="-1"
        onclick={() => {
          close(true);
          action.onselect(trigger);
        }}>{action.label}</button
      >
    {/each}
  </div>
</span>

<style>
  .action-menu {
    display: inline-flex;
    flex-shrink: 0;
  }
  .menu-trigger {
    display: grid;
    place-items: center;
    width: 36px;
    height: 36px;
    padding: 0;
    border: 1px solid transparent;
    border-radius: 7px;
    background: var(--paper);
    color: var(--muted);
  }
  .menu-trigger:hover,
  .menu-trigger[aria-expanded='true'] {
    background: var(--hover);
    border-color: var(--line);
    color: var(--ink);
  }
  .menu-trigger:disabled {
    opacity: 0.5;
    cursor: default;
  }
  .menu-trigger.quiet {
    background: transparent;
  }
  .menu-trigger.quiet:hover,
  .menu-trigger.quiet[aria-expanded='true'] {
    background: var(--hover);
  }
  .menu {
    position: fixed;
    inset: auto;
    margin: 0;
    padding: 5px;
    min-width: 156px;
    max-width: calc(100vw - 16px);
    max-height: calc(100svh - 16px);
    overflow: auto;
    border: 1px solid var(--line);
    border-radius: 9px;
    background: var(--paper);
    color: var(--ink);
    box-shadow: 0 12px 35px #0009;
  }
  .menu button {
    display: block;
    width: 100%;
    min-height: 38px;
    padding: 9px 12px;
    border: 0;
    border-radius: 5px;
    background: transparent;
    color: inherit;
    text-align: start;
    font-size: 12px;
  }
  .menu button:hover,
  .menu button:focus {
    background: var(--selected);
  }
  @media (pointer: coarse) {
    .menu-trigger {
      width: 44px;
      height: 44px;
    }
    .menu button {
      min-height: 44px;
    }
  }
</style>
