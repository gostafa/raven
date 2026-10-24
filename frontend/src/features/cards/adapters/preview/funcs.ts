import type { PreviewEventsPort, PreviewStartPort } from '../../ports/types';
import type { PreviewCompletion } from './types';

class NativePreviewRequest {
  private timer: ReturnType<typeof setTimeout> | undefined;
  private off: () => void = () => {
    /* Registration has not completed yet. */
  };
  public constructor(
    private readonly events: PreviewEventsPort,
    private readonly play: () => Promise<void>,
    private readonly completion: PreviewCompletion,
  ) {}
  private dispose(): void {
    clearTimeout(this.timer);
    this.off();
  }
  public start(): void {
    const name = `raven:video-preview:${this.events.newKey()}`;
    this.timer = setTimeout(() => {
      this.dispose();
      this.completion.reject(new Error('The native video preview did not respond.'));
    }, 5000);
    this.off = this.events.once(name, () => {
      this.ready();
    });
    this.events.emit(name).catch((error: unknown) => {
      this.dispose();
      this.completion.reject(
        error instanceof Error ? error : new Error('The native video preview request failed.', { cause: error }),
      );
    });
  }
  private ready(): void {
    this.dispose();
    // Keep play synchronous in Wails' native callback to retain WebKit's permission.
    try {
      this.play().then(
        () => {
          this.completion.resolve();
        },
        (error: unknown) => {
          this.completion.reject(error);
        },
      );
    } catch (error) {
      this.completion.reject(error);
    }
  }
}
export function createNativePreviewStarter(events: PreviewEventsPort): PreviewStartPort {
  return {
    start(play) {
      return new Promise<void>((resolve, reject) => {
        new NativePreviewRequest(events, play, { resolve, reject }).start();
      });
    },
  };
}
