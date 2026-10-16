import type { MediaURLPort } from '../domain/types';

export const mediaURL: MediaURLPort = (item) =>
  `/media/${[item.folderId, item.cardId, item.id].map(encodeURIComponent).join('/')}`;
