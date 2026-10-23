import { autoUpdate, computePosition, flip, offset, shift } from '@floating-ui/dom';
import type { PopoverPositioner } from './types';

/** Uses logical alignment, including RTL, while preserving the native popover's focus handling. */
export function positionPopover(
  ...[anchor, panel, alignment]: Parameters<PopoverPositioner>
): ReturnType<PopoverPositioner> {
  let active = true;
  const dispose = autoUpdate(anchor, panel, () => {
    void computePosition(anchor, panel, {
      strategy: 'fixed',
      placement: `bottom-${alignment}`,
      middleware: [offset(8), flip(), shift({ padding: 8 })],
    })
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
