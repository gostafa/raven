export interface MediaLoadOptions {
  source: string;
  deferred: boolean;
  active: boolean;
  resolve: (source: string | undefined) => void;
}

export interface MediaLoadHandle {
  update(options: MediaLoadOptions): void;
  destroy(): void;
}
