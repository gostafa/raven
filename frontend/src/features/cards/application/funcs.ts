import type { Card, CardPort, EditorItem, MediaKind, MediaPort } from '../domain/types';
import { editItems, saveItems } from '../domain/funcs';
import type { PlaybackPort, PreviewStartPort, VideoPreviewPort, PreviewPlaybackPort } from '../ports/types';

export async function removeCardPart(port: CardPort, card: Card, itemId: string): Promise<Card | null> {
  const remaining = editItems(card, () => '').filter((item) => item.id !== itemId);
  if (remaining.length) {
    return await port.UpdateCard({ folderId: card.folderId, cardId: card.id, items: saveItems(remaining) });
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

class PreviewPlayback implements PreviewPlaybackPort {
  private wanted = false;
  private stoppedTime = 0;
  private revision = 0;
  public constructor(
    private readonly media: VideoPreviewPort,
    private readonly nativeStart?: PreviewStartPort,
  ) {
    media.defaultMuted = true;
    media.muted = true;
  }
  private settle(): void {
    if (this.wanted) return;
    this.media.pause();
    this.media.currentTime = this.stoppedTime;
  }
  private async nativeAttempt(error: unknown, requestedRevision: number): Promise<void> {
    if (!this.wanted || !this.nativeStart || !isPlaybackDenied(error)) return;
    try {
      await this.nativeStart.start(async () => {
        if (this.wanted && this.revision === requestedRevision) await playMedia(this.media);
      });
      this.settle();
    } catch {
      // Readiness events can retry if the native window is temporarily unavailable.
    }
  }
  private async attempt(): Promise<void> {
    const requestedRevision = this.revision;
    try {
      await playMedia(this.media);
      this.settle();
    } catch (error) {
      await this.nativeAttempt(error, requestedRevision);
    }
  }
  public async retry(): Promise<void> {
    if (this.wanted) await this.attempt();
  }
  public async start(): Promise<void> {
    this.wanted = true;
    await this.attempt();
  }
  public stop(position = 0): void {
    this.wanted = false;
    this.revision++;
    this.stoppedTime = position;
    this.media.pause();
    this.media.currentTime = this.stoppedTime;
  }
}
function isPlaybackDenied(error: unknown): boolean {
  return typeof error === 'object' && error !== null && 'name' in error && error.name === 'NotAllowedError';
}
export function createPreviewPlayback(media: VideoPreviewPort, nativeStart?: PreviewStartPort): PreviewPlaybackPort {
  return new PreviewPlayback(media, nativeStart);
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
