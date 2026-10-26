import type { MediaLoadHandle, MediaLoadOptions } from './types';

class MediaLoader implements MediaLoadHandle {
  private readonly node: HTMLElement;
  private options: MediaLoadOptions;
  private source: string | undefined;
  private observer: IntersectionObserver | undefined;
  private disposed = false;

  public constructor(node: HTMLElement, options: MediaLoadOptions) {
    this.node = node;
    this.options = options;
    node.addEventListener('pointerenter', this.activate);
    node.addEventListener('focusin', this.activate);
    this.observe();
  }

  private readonly activate = (): void => {
    if (this.disposed || this.source === this.options.source) return;
    this.source = this.options.source;
    this.observer?.disconnect();
    this.options.resolve(this.source);
  };

  private observe(): void {
    this.observer?.disconnect();
    if (!this.options.deferred || this.options.active || typeof IntersectionObserver === 'undefined') {
      this.activate();
      return;
    }
    this.watch();
  }

  private watch(): void {
    const current = new IntersectionObserver(
      (entries) => {
        if (this.observer === current && entries.some((entry) => entry.isIntersecting)) this.activate();
      },
      { rootMargin: '300px' },
    );
    this.observer = current;
    current.observe(this.node);
  }

  public update(next: MediaLoadOptions): void {
    if (this.disposed) return;
    const changed = this.options.source !== next.source;
    this.options = next;
    if (changed) {
      this.source = undefined;
      this.options.resolve(undefined);
    }
    if (this.source !== this.options.source) this.observe();
  }

  public destroy(): void {
    this.disposed = true;
    this.observer?.disconnect();
    this.node.removeEventListener('pointerenter', this.activate);
    this.node.removeEventListener('focusin', this.activate);
  }
}

/** Keep sources attached once activated, and release observers when their owner disappears. */
export function loadMedia(node: HTMLElement, initial: MediaLoadOptions): MediaLoadHandle {
  return new MediaLoader(node, initial);
}
