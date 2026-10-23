import type { Card, CardPort, EditorItem, MediaKind, MediaPort } from '../domain/types';
import { editItems, saveItems } from '../domain/funcs';
import type { PlaybackPort, PreviewStartPort, VideoPreviewPort } from '../ports/types';

export async function removeCardPart(port: CardPort, card: Card, itemId: string): Promise<Card | null> {
  const remaining = editItems(card, () => '').filter((item) => item.id !== itemId);
  if (remaining.length) {
    return port.UpdateCard({ folderId: card.folderId, cardId: card.id, items: saveItems(remaining) });
  }
  await port.DeleteCard(card.folderId, card.id);
  return null;
}

export function seekMedia(media: PlaybackPort, time: number): void {
  if (Number.isFinite(media.duration) && media.duration > 0 && Number.isFinite(time)) {
    media.currentTime = Math.max(0, Math.min(time, media.duration));
  }
}

export async function playMedia(media: PlaybackPort): Promise<void> {
  if (media.ended) media.currentTime = 0;
  await media.play();
}

export function createPreviewPlayback(media: VideoPreviewPort, nativeStart?: PreviewStartPort) {
  media.defaultMuted = true;
  media.muted = true;
  let wanted = false;
  let stoppedTime = 0;
  let revision = 0;
  function settle() {
    if (!wanted) {
      media.pause();
      media.currentTime = stoppedTime;
    }
  }
  async function attempt() {
    const requestedRevision = revision;
    try {
      await playMedia(media);
      settle();
    } catch (error) {
      if (
        wanted &&
        nativeStart &&
        typeof error === 'object' &&
        error !== null &&
        'name' in error &&
        error.name === 'NotAllowedError'
      ) {
        try {
          await nativeStart.start(async () => {
            if (wanted && revision === requestedRevision) await playMedia(media);
          });
          settle();
        } catch {
          // Readiness events can retry if the native window is temporarily unavailable.
        }
      }
    }
  }
  return {
    async retry() {
      if (wanted) await attempt();
    },
    async start() {
      wanted = true;
      await attempt();
    },
    stop(position = 0) {
      wanted = false;
      revision++;
      stoppedTime = position;
      media.pause();
      media.currentTime = stoppedTime;
    },
  };
}
export async function selectMedia(port: MediaPort, kind: MediaKind, items: EditorItem[]): Promise<EditorItem[]> {
  const files = await port.SelectMediaFiles(kind);
  return [
    ...items,
    ...files.map((file) => ({
      key: port.newKey(),
      kind,
      sourcePath: file.sourcePath,
      originalName: file.originalName,
      previewURL: file.previewURL,
    })),
  ];
}
