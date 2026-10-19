export type MediaKind = 'image' | 'audio' | 'video';
interface Schedule {
  Due: string;
  Stability: number;
  Difficulty: number;
  ScheduledDays: number;
  Reps: number;
  Lapses: number;
  State: number;
  LastReview: string;
  RemainingSteps: number;
}
interface ReviewLog {
  Rating: number;
  Due: string;
  ScheduledDays: number;
  Review: string;
  State: number;
  Stability: number;
  Difficulty: number;
  RemainingSteps: number;
}
interface ItemReview {
  log: ReviewLog;
  automatic: boolean;
}
export interface ContentItem {
  hint?: boolean;
  schedule: Schedule;
  reviews: ItemReview[];
  lastNotifiedDue: number;
  id: string;
  folderId: string;
  cardId: string;
  position: number;
  kind: string;
  text?: string;
  originalName?: string;
  mimeType?: string;
  byteSize?: number;
  relativePath?: string;
}
export interface Card {
  id: string;
  folderId: string;
  createdAt: number;
  dueAt: number;
  lastReviewed: number;
  items: ContentItem[];
}
export interface ItemDraft {
  hint?: boolean;
  id?: string;
  kind: string;
  text?: string;
  sourcePath?: string;
}
export type EditorItem = ItemDraft & { key: string; originalName?: string; previewURL?: string };
interface SelectedMedia {
  sourcePath: string;
  originalName: string;
  byteSize: number;
  previewURL: string;
}
export interface CardPort {
  ListCards(): Promise<Card[]>;
  CreateCard(input: { folderId: string; items: ItemDraft[] }): Promise<Card>;
  UpdateCard(input: { folderId: string; cardId: string; items: ItemDraft[] }): Promise<Card>;
  DeleteCard(folderId: string, cardId: string): Promise<void>;
}
export type MediaURLPort = (item: ContentItem) => string;
export interface MediaPort {
  mediaURL: MediaURLPort;
  SelectMediaFiles(kind: MediaKind): Promise<SelectedMedia[]>;
  newKey(): string;
}
