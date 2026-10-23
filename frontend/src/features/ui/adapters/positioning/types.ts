/** Consumer contract for a component-owned popover positioning lifecycle. */
export type PopoverPositioner = (anchor: HTMLElement, panel: HTMLElement, alignment: 'start' | 'end') => () => void;
