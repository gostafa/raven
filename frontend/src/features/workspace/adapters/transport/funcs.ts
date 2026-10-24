import type { LocalizationState } from '../../../localization';
import { isRecord } from './validation/funcs';
import type {
  TransportConfiguration,
  TransportCard,
  TransportItem,
  TransportDraft,
  TransportFolder,
  TransportReview,
  TransportSchedule,
  TransportMedia,
  TransportArchiveResult,
} from './types';
import type { RuntimeConfiguration, AppPorts } from '../../domain/types';
import type { Card } from '../../../cards';
import type { Folder } from '../../../folders';
import type { ArchiveResult } from '../../../folderarchive';

export function archiveFromTransport(value: TransportArchiveResult): ArchiveResult {
  return {
    canceled: value.canceled,
    filename: value.filename,
    folderIds: value.folder_ids ?? [],
    folderCount: value.folder_count,
    cardCount: value.card_count,
    mediaCount: value.media_count,
  };
}
export function folderFromTransport(value: TransportFolder): Folder {
  return { ...value, tags: value.tags ?? [] };
}
export function cardFromTransport(value: TransportCard, textKind: string): Card {
  return {
    id: value.id,
    folderId: value.folder_id,
    createdAt: value.created_at,
    dueAt: value.due_at,
    lastReviewed: value.last_reviewed,
    items: (value.items ?? []).map((item) => itemFromTransport(item, textKind)),
  };
}
function scheduleFromTransport(value: TransportSchedule): Card['items'][number]['schedule'] {
  return {
    Due: timestampFromTransport(value.due),
    LastReview: timestampFromTransport(value.last_review),
    Stability: value.stability,
    Difficulty: value.difficulty,
    ScheduledDays: value.scheduled_days,
    Reps: value.reps,
    Lapses: value.lapses,
    RemainingSteps: value.remaining_steps,
    State: value.state,
  };
}
function reviewFromTransport(value: TransportReview): Card['items'][number]['reviews'][number]['log'] {
  return {
    Due: timestampFromTransport(value.due),
    Review: timestampFromTransport(value.review),
    ScheduledDays: value.scheduled_days,
    Stability: value.stability,
    Difficulty: value.difficulty,
    RemainingSteps: value.remaining_steps,
    Rating: value.rating,
    State: value.state,
  };
}
function itemFromTransport(value: TransportItem, textKind: string): Card['items'][number] {
  return {
    id: value.id,
    folderId: value.folder_id,
    cardId: value.card_id,
    kind: value.kind === textKind ? 'text' : value.kind,
    ...itemContent(value),
    position: value.position,
    hint: value.hint ?? false,
    lastNotifiedDue: value.last_notified_due,
    schedule: scheduleFromTransport(value.schedule),
    reviews: (value.reviews ?? []).map((review) => ({
      automatic: review.automatic,
      log: reviewFromTransport(review.log),
    })),
  };
}
function itemContent(
  value: TransportItem,
): Pick<Card['items'][number], 'text' | 'originalName' | 'mimeType' | 'relativePath' | 'byteSize'> {
  return {
    text: value.text,
    originalName: value.original_name,
    mimeType: value.mime_type,
    relativePath: value.relative_path,
    byteSize: value.byte_size,
  };
}
export function draftToTransport(
  value: Parameters<AppPorts['CreateCard']>[0]['items'][number],
  textKind: string,
): TransportDraft {
  return {
    id: value.id,
    kind: value.kind === 'text' ? textKind : value.kind,
    text: value.text,
    source_path: value.sourcePath,
    ...(value.hint === undefined ? {} : { hint: value.hint }),
  };
}
export function mediaFromTransport(value: TransportMedia): Awaited<ReturnType<AppPorts['SelectMediaFiles']>>[number] {
  return {
    sourcePath: value.source_path,
    originalName: value.original_name,
    byteSize: value.byte_size,
    previewURL: value.preview_url,
  };
}

export function configurationFromTransport(value: TransportConfiguration): RuntimeConfiguration {
  return {
    localizationChanged: value.localization_changed,
    preferencesStorageKey: value.preferences_storage_key,
    mediaRoutePrefix: value.media_route_prefix,
    textKind: value.text_kind,
    schedulesChanged: value.schedules_changed,
    showReviews: value.show_reviews,
    openFloatingReview: value.open_floating_review,
    closeFloatingReview: value.close_floating_review,
  };
}

function timestampFromTransport(value: unknown): string {
  if (typeof value !== 'string') throw new Error('Raven returned an invalid review timestamp.');
  return value;
}

export function parseTransport(payload: string): unknown {
  return JSON.parse(payload) as unknown;
}

export function localizationFromTransport(value: unknown): LocalizationState {
  if (!isRecord(value)) throw new Error('Invalid localization response');
  const { preference, locale, direction, revision } = value;
  if (preference !== 'system' && preference !== 'en' && preference !== 'ar')
    throw new Error('Invalid localization response');
  if (locale !== 'en' && locale !== 'ar') throw new Error('Invalid localization response');
  validateLocalizationMetadata({ direction, revision }, locale);
  return { preference, locale, direction: locale === 'ar' ? 'rtl' : 'ltr', revision: Number(revision) };
}
function validateLocalizationMetadata(data: Record<string, unknown>, locale: string): void {
  if (
    data.direction !== (locale === 'ar' ? 'rtl' : 'ltr') ||
    !Number.isSafeInteger(data.revision) ||
    Number(data.revision) < 0
  )
    throw new Error('Invalid localization response');
}
