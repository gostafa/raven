import type { MediaPlaybackPort } from '../types';

export const stopMedia: MediaPlaybackPort['stopMedia'] = (root) => {
  root?.querySelectorAll<HTMLMediaElement>('audio, video').forEach((media) => {
    media.pause();
  });
};
