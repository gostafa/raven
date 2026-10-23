import type { PreviewEventsPort, PreviewStartPort } from '../../ports/types';

export function createNativePreviewStarter(events: PreviewEventsPort): PreviewStartPort {
  return {
    start(play) {
      return new Promise<void>((resolve, reject) => {
        const name = `raven:video-preview:${events.newKey()}`;
        // Wails delivers the echoed event through native JavaScript evaluation.
        // Call play synchronously in that callback to retain WebKit's permission.
        const off = events.once(name, () => {
          clearTimeout(timeout);
          off();
          void play().then(resolve, reject);
        });
        const timeout = setTimeout(() => {
          off();
          reject(new Error('The native video preview did not respond.'));
        }, 5000);
        void events.emit(name).catch((error: unknown) => {
          clearTimeout(timeout);
          off();
          reject(
            error instanceof Error ? error : new Error('The native video preview request failed.', { cause: error }),
          );
        });
      });
    },
  };
}
