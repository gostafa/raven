import type {
  TransportConfiguration,
  TransportCard,
  TransportItem,
  TransportDraft,
  TransportFolder,
  TransportReview,
  TransportSchedule,
  TransportMedia,
} from './types';
import type { RuntimeConfiguration } from '../../domain/types';
import type { Card } from '../../../cards';
import type { Folder } from '../../../folders';
import type { AppPorts } from '../../domain/types';
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
    text: value.text,
    originalName: value.original_name,
    mimeType: value.mime_type,
    relativePath: value.relative_path,
    byteSize: value.byte_size,
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
export function mediaFromTransport(value: TransportMedia) {
  return {
    sourcePath: value.source_path,
    originalName: value.original_name,
    byteSize: value.byte_size,
    previewURL: value.preview_url,
  };
}

export function configurationFromTransport(value: TransportConfiguration): RuntimeConfiguration {
  return {
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
