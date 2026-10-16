import type { ContentItem } from '../domain/types';
export function mediaURL(item: ContentItem): string {
  return `/media/${[item.folderId, item.cardId, item.id].map(encodeURIComponent).join('/')}`;
}
