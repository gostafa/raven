import type { Card as TransportCard } from '../../../../../bindings/raven/api/cards/models';
import type { Item as TransportItem, Draft as TransportDraft } from '../../../../../bindings/raven/api/content/models';
import type { Folder as TransportFolder } from '../../../../../bindings/raven/api/folders/models';
import type { Log as TransportReview, Schedule as TransportSchedule } from '../../../../../bindings/raven/api/reviews/models';
import type { Selection as TransportMedia } from '../../../../../bindings/raven/api/media/models';
import type { Card } from '../../../cards';
import type { Folder } from '../../../folders';
import type { AppPorts } from '../../domain/types';
export function folderFromTransport(value: TransportFolder): Folder {
  return { ...value, tags: value.tags ?? [] };
}
export function cardFromTransport(value: TransportCard): Card {
  return {
    id: value.id,
    folderId: value.folder_id,
    createdAt: value.created_at,
    dueAt: value.due_at,
    lastReviewed: value.last_reviewed,
    items: (value.items ?? []).map(itemFromTransport),
  };
}
function scheduleFromTransport(value: TransportSchedule): Card['items'][number]['schedule'] {
  return {
    Due: value.due,
    LastReview: value.last_review,
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
    Due: value.due,
    Review: value.review,
    ScheduledDays: value.scheduled_days,
    Stability: value.stability,
    Difficulty: value.difficulty,
    RemainingSteps: value.remaining_steps,
    Rating: value.rating,
    State: value.state,
  };
}
function itemFromTransport(value: TransportItem): Card['items'][number] {
  return {
    id: value.id,
    folderId: value.folder_id,
    cardId: value.card_id,
    kind: value.kind,
    text: value.text,
    originalName: value.original_name,
    mimeType: value.mime_type,
    relativePath: value.relative_path,
    byteSize: value.byte_size,
    position: value.position,
    lastNotifiedDue: value.last_notified_due,
    schedule: scheduleFromTransport(value.schedule),
    reviews: (value.reviews ?? []).map((review) => ({ automatic: review.automatic, log: reviewFromTransport(review.log) })),
  };
}
export function draftToTransport(value: Parameters<AppPorts['CreateCard']>[0]['items'][number]): TransportDraft {
  return { id: value.id, kind: value.kind, text: value.text, source_path: value.sourcePath };
}
export function mediaFromTransport(value: TransportMedia) {
  return {
    sourcePath: value.source_path,
    originalName: value.original_name,
    byteSize: value.byte_size,
    previewURL: value.preview_url,
  };
}
