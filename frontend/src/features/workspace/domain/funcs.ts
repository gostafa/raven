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

export function parseViewPreferences(value: unknown): ViewPreferences {
  const defaults = defaultViewPreferences();
  if (value === null || typeof value !== 'object' || Array.isArray(value)) return defaults;
  const data = value as Record<string, unknown>;
  const range = data.cardProgressRange as Record<string, unknown> | null | undefined;
  const validRange =
    typeof range?.minimum === 'number' &&
    typeof range.maximum === 'number' &&
    Number.isFinite(range.minimum) &&
    Number.isFinite(range.maximum) &&
    range.minimum >= 0 &&
    range.maximum <= 100 &&
    range.minimum <= range.maximum;
  return {
    search: typeof data.search === 'string' ? data.search : defaults.search,
    foldersOpen: typeof data.foldersOpen === 'boolean' ? data.foldersOpen : defaults.foldersOpen,
    folderTags: Array.isArray(data.folderTags)
      ? [...new Set(data.folderTags.filter((tag): tag is string => typeof tag === 'string' && tag.length > 0))]
      : defaults.folderTags,
    folderSort: choice(
      data.folderSort,
      ['default', 'tags-desc', 'tags-asc', 'cards-desc', 'cards-asc', 'review-asc', 'review-desc'],
      defaults.folderSort,
    ),
    cardSort: choice(
      data.cardSort,
      ['created-desc', 'created-asc', 'review-asc', 'review-desc', 'progress-asc', 'progress-desc'],
      defaults.cardSort,
    ),
    cardKinds: Array.isArray(data.cardKinds)
      ? [
          ...new Set(
            data.cardKinds.filter(
              (kind): kind is ViewPreferences['cardKinds'][number] =>
                typeof kind === 'string' && ['text', 'image', 'audio', 'video'].includes(kind),
            ),
          ),
        ]
      : defaults.cardKinds,
    cardProgressRange: validRange
      ? { minimum: range.minimum as number, maximum: range.maximum as number }
      : defaults.cardProgressRange,
  };
}
