import { autoUpdate, computePosition, flip, offset, shift } from '@floating-ui/dom';
import type { PopoverPositioner, PopoverTracker } from './types';

function computePopoverPosition(...[anchor, panel, alignment]: Parameters<PopoverPositioner>) {
  return computePosition(anchor, panel, {
    strategy: 'fixed',
    placement: `bottom-${alignment}`,
    middleware: [offset(8), flip(), shift({ padding: 8 })],
  });
}

/** Uses logical alignment, including RTL, while preserving the native popover's focus handling. */
export function positionPopover(
  ...[anchor, panel, alignment]: Parameters<PopoverPositioner>
): ReturnType<PopoverPositioner> {
  let active = true;
  const dispose = autoUpdate(anchor, panel, () => {
    computePopoverPosition(anchor, panel, alignment)
      .then(({ x, y }) => {
        if (active) Object.assign(panel.style, { left: `${String(x)}px`, top: `${String(y)}px` });
      })
      .catch(() => {
        active = false;
      });
  });
  return () => {
    active = false;
    dispose();
  };
}

/** Tracks an open panel and releases its positioning and dismissal listeners together. */
export function trackPopover(...[anchor, panel, options]: Parameters<PopoverTracker>): ReturnType<PopoverTracker> {
  const { alignment, dismiss, ignorePanelScroll } = options;
  const release = positionPopover(anchor, panel, alignment);
  const onScroll = (event: Event) => {
    if (ignorePanelScroll === true && event.target instanceof Node && panel.contains(event.target)) return;
    dismiss();
  };
  window.addEventListener('scroll', onScroll, true);
  window.addEventListener('resize', dismiss);
  return () => {
    release();
    window.removeEventListener('scroll', onScroll, true);
    window.removeEventListener('resize', dismiss);
  };
}
