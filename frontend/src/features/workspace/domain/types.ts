import type { CardPort, CardSort, CardContentKind, CardProgressRange, MediaPort, PreviewStartPort } from '../../cards';
import type { FolderPort, FolderSort, PreferencesStorage } from '../../folders';
import type { ReviewPort, ReviewEvents } from '../../reviews';
import type { WindowPort } from '../../companion';
import type { ArchivePort } from '../../folderarchive';
import type { LocalizationPort } from '../../localization';
interface AppEvents extends ReviewEvents {
  onSchedulesChanged(callback: () => void): () => void;
  onShowReviews(callback: () => void): () => void;
}
export interface ViewPreferences {
  search: string;
  foldersOpen: boolean;
  folderTags: string[];
  folderSort: FolderSort;
  cardSort: CardSort;
  cardKinds: CardContentKind[];
  cardProgressRange: CardProgressRange;
}
export interface RuntimeConfiguration {
  localizationChanged: string;
  preferencesStorageKey: string;
  mediaRoutePrefix: string;
  textKind: string;
  schedulesChanged: string;
  showReviews: string;
  openFloatingReview: string;
  closeFloatingReview: string;
}
export interface AppPorts
  extends CardPort, FolderPort, MediaPort, ReviewPort, WindowPort, ArchivePort, LocalizationPort {
  configuration: RuntimeConfiguration;
  previewStart: PreviewStartPort;
  preferences: PreferencesStorage;
  events: AppEvents;
  now(): number;
}
