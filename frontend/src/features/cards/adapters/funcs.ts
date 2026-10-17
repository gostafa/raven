import type { MediaURLPort } from '../domain/types';

export function createMediaURL(prefix: string): MediaURLPort {
  return (item) => `${prefix}${[item.folderId, item.cardId, item.id].map(encodeURIComponent).join('/')}`;
}
