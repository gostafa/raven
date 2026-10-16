import type { CardPort, MediaPort } from '../../cards';
import type { FolderPort, PreferencesStorage } from '../../folders';
import type { ReviewPort, ReviewEvents } from '../../reviews';
import type { WindowPort } from '../../companion';
interface AppEvents extends ReviewEvents {
  onSchedulesChanged(callback: () => void): () => void;
  onShowReviews(callback: () => void): () => void;
}
export interface AppPorts extends CardPort, FolderPort, MediaPort, ReviewPort, WindowPort {
  preferences: PreferencesStorage;
  events: AppEvents;
  now(): number;
}
