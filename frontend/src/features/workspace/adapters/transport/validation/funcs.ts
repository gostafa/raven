import type {
  TransportArchiveResult,
  TransportTransferResult,
  TransportCard,
  TransportConfiguration,
  TransportFolder,
  TransportItem,
  TransportMedia,
  TransportReview,
  TransportSchedule,
} from '../types';

export function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function fields(value: Record<string, unknown>, names: string, kind: string): boolean {
  return names.split(' ').every((name) => typeof fieldValue(value, name) === kind);
}

function optional(value: Record<string, unknown>, names: string, kind: string): boolean {
  return names
    .split(' ')
    .every((name) => fieldValue(value, name) === undefined || typeof fieldValue(value, name) === kind);
}
function fieldValue(value: Record<string, unknown>, name: string): unknown {
  return Object.getOwnPropertyDescriptor(value, name)?.value;
}

function list<T>(value: unknown, guard: (item: unknown) => item is T): value is T[] | null {
  return value === null || (Array.isArray(value) && value.every(guard));
}

export function isFolder(value: unknown): value is TransportFolder {
  return (
    isRecord(value) &&
    fields(value, 'id title description', 'string') &&
    list(value.tags, (tag): tag is string => typeof tag === 'string')
  );
}

export function isConfiguration(value: unknown): value is TransportConfiguration {
  return (
    isRecord(value) &&
    fields(
      value,
      'localization_changed preferences_storage_key media_route_prefix text_kind ' +
        'schedules_changed show_reviews open_floating_review close_floating_review',
      'string',
    )
  );
}

export function isArchive(value: unknown): value is TransportArchiveResult {
  return (
    isRecord(value) &&
    fields(value, 'canceled', 'boolean') &&
    fields(value, 'filename', 'string') &&
    fields(value, 'folder_count card_count media_count', 'number') &&
    list(value.folder_ids, (id): id is string => typeof id === 'string')
  );
}

export function isMedia(value: unknown): value is TransportMedia {
  return (
    isRecord(value) &&
    fields(value, 'source_path original_name preview_url', 'string') &&
    fields(value, 'byte_size', 'number')
  );
}

function isSchedule(value: unknown): value is TransportSchedule {
  return (
    isRecord(value) &&
    fields(value, 'due last_review', 'string') &&
    fields(value, 'stability difficulty scheduled_days reps lapses remaining_steps state', 'number')
  );
}

function isReview(value: unknown): value is TransportReview {
  return (
    isRecord(value) &&
    fields(value, 'due review', 'string') &&
    fields(value, 'scheduled_days stability difficulty remaining_steps rating state', 'number')
  );
}

function isItemReview(value: unknown): value is { log: TransportReview; automatic: boolean } {
  return isRecord(value) && typeof value.automatic === 'boolean' && isReview(value.log);
}

function isItemContent(value: Record<string, unknown>): boolean {
  return (
    fields(value, 'id folder_id card_id kind', 'string') &&
    optional(value, 'text original_name mime_type relative_path', 'string') &&
    optional(value, 'byte_size', 'number') &&
    optional(value, 'hint', 'boolean')
  );
}

function isItem(value: unknown): value is TransportItem {
  return (
    isRecord(value) &&
    isItemContent(value) &&
    fields(value, 'position last_notified_due', 'number') &&
    isSchedule(value.schedule) &&
    list(value.reviews, isItemReview)
  );
}

export function isCard(value: unknown): value is TransportCard {
  return (
    isRecord(value) &&
    fields(value, 'id folder_id', 'string') &&
    fields(value, 'created_at due_at last_reviewed', 'number') &&
    (value.items === undefined || list(value.items, isItem))
  );
}

export function decode<T>(value: unknown, guard: (item: unknown) => item is T): T {
  if (!guard(value)) throw new Error('Invalid Raven response');
  return value;
}

export function decodeList<T>(value: unknown, guard: (item: unknown) => item is T): T[] {
  if (value === null) return [];
  if (!Array.isArray(value)) throw new Error('Invalid Raven list response');
  return value.map((item: unknown) => decode(item, guard));
}

export function isTransferResult(value: unknown): value is TransportTransferResult {
  return (
    isRecord(value) &&
    Array.isArray(value.entries) &&
    value.entries.length > 0 &&
    value.entries.every(
      (entry: unknown) =>
        isRecord(entry) &&
        isRecord(entry.source) &&
        fields(entry.source, 'folder_id card_id', 'string') &&
        isCard(entry.card),
    )
  );
}
