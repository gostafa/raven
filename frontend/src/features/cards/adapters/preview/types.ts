export interface PreviewCompletion {
  resolve(): void;
  reject(error: unknown): void;
}
