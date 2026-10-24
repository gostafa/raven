/** Playback operations supplied by a media UI adapter. */
export interface PlaybackPort {
  play(): Promise<void>;
  pause(): void;
  currentTime: number;
  duration: number;
  ended: boolean;
}

/** Native mute state required before a video preview's first play request. */
export interface VideoPreviewPort extends PlaybackPort {
  muted: boolean;
  defaultMuted: boolean;
}

/** Starts playback inside a native callback when a webview requires that context. */
export interface PreviewStartPort {
  start(play: () => Promise<void>): Promise<void>;
}

/** Native event transport consumed by the preview-start adapter. */
export interface PreviewEventsPort {
  once(name: string, callback: () => void): () => void;
  emit(name: string): Promise<unknown>;
  newKey(): string;
}

export interface PreviewPlaybackPort {
  retry(): Promise<void>;
  start(): Promise<void>;
  stop(position?: number): void;
}
