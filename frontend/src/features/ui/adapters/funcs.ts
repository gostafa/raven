export function stopMedia(root: ParentNode | undefined): void {
  root?.querySelectorAll<HTMLMediaElement>('audio, video').forEach((media) => {
    media.pause();
  });
}
