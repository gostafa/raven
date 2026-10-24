import type { ViewPreferences } from './types';

export function defaultViewPreferences(): ViewPreferences {
  return {
    search: '',
    foldersOpen: true,
    folderTags: [],
    folderSort: 'default',
    cardSort: 'created-desc',
    cardKinds: [],
    cardProgressRange: { minimum: 0, maximum: 100 },
  };
}

function choice<T extends string>(value: unknown, options: readonly T[], fallback: T): T {
  return options.find((option) => option === value) ?? fallback;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === 'object' && !Array.isArray(value);
}
function progressRange(value: unknown): ViewPreferences['cardProgressRange'] {
  if (!isRecord(value)) return { minimum: 0, maximum: 100 };
  const { minimum, maximum } = value;
  if (!validPercentage(minimum) || !validPercentage(maximum) || minimum > maximum) return { minimum: 0, maximum: 100 };
  return { minimum, maximum };
}
function validPercentage(value: unknown): value is number {
  return typeof value === 'number' && Number.isFinite(value) && value >= 0 && value <= 100;
}
function folderTags(value: unknown): string[] {
  return Array.isArray(value)
    ? [...new Set(value.filter((tag): tag is string => typeof tag === 'string' && tag.length > 0))]
    : [];
}
function cardKinds(value: unknown): ViewPreferences['cardKinds'] {
  if (!Array.isArray(value)) return [];
  return [
    ...new Set(
      value.filter(
        (kind): kind is ViewPreferences['cardKinds'][number] =>
          kind === 'text' || kind === 'image' || kind === 'audio' || kind === 'video',
      ),
    ),
  ];
}
function sorting(data: Record<string, unknown>): Pick<ViewPreferences, 'folderSort' | 'cardSort'> {
  return {
    folderSort: choice(
      data.folderSort,
      ['default', 'tags-desc', 'tags-asc', 'cards-desc', 'cards-asc', 'review-asc', 'review-desc'],
      'default',
    ),
    cardSort: choice(
      data.cardSort,
      ['created-desc', 'created-asc', 'review-asc', 'review-desc', 'progress-asc', 'progress-desc'],
      'created-desc',
    ),
  };
}
export function parseViewPreferences(value: unknown): ViewPreferences {
  if (!isRecord(value)) return defaultViewPreferences();
  return {
    search: typeof value.search === 'string' ? value.search : '',
    foldersOpen: typeof value.foldersOpen === 'boolean' ? value.foldersOpen : true,
    folderTags: folderTags(value.folderTags),
    cardKinds: cardKinds(value.cardKinds),
    cardProgressRange: progressRange(value.cardProgressRange),
    ...sorting(value),
  };
}
