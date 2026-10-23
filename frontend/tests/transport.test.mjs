import assert from 'node:assert/strict';
import test from 'node:test';
import {
  configurationFromTransport,
  cardFromTransport,
  draftToTransport,
  mediaFromTransport,
} from '../src/features/workspace/adapters/transport/funcs.ts';

test('snake_case card transport preserves frontend review and media contracts', () => {
  const schedule = {
    due: '2026-10-20T12:00:00Z',
    last_review: '2026-10-16T12:00:00Z',
    stability: 3,
    difficulty: 4,
    scheduled_days: 4,
    reps: 2,
    lapses: 1,
    remaining_steps: 0,
    state: 2,
  };
  const log = {
    due: schedule.due,
    review: schedule.last_review,
    scheduled_days: 4,
    stability: 3,
    difficulty: 4,
    remaining_steps: 0,
    rating: 3,
    state: 2,
  };
  const result = cardFromTransport(
    {
      id: 'card',
      folder_id: 'folder',
      created_at: 10,
      due_at: 20,
      last_reviewed: 15,
      items: [
        {
          id: 'item',
          folder_id: 'folder',
          card_id: 'card',
          kind: 'image',
          hint: true,
          position: 0,
          original_name: 'image.png',
          relative_path: 'media/item.png',
          mime_type: 'image/png',
          byte_size: 8,
          last_notified_due: 20,
          schedule,
          reviews: [{ log, automatic: false }],
        },
      ],
    },
    'text',
  );
  assert.equal(result.folderId, 'folder');
  assert.equal(result.createdAt, 10);
  assert.equal(result.dueAt, 20);
  assert.equal(result.lastReviewed, 15);
  assert.deepEqual(result.items[0].schedule, {
    Due: schedule.due,
    LastReview: schedule.last_review,
    Stability: 3,
    Difficulty: 4,
    ScheduledDays: 4,
    Reps: 2,
    Lapses: 1,
    RemainingSteps: 0,
    State: 2,
  });
  assert.deepEqual(result.items[0].reviews, [
    {
      automatic: false,
      log: {
        Due: log.due,
        Review: log.review,
        ScheduledDays: 4,
        Stability: 3,
        Difficulty: 4,
        RemainingSteps: 0,
        Rating: 3,
        State: 2,
      },
    },
  ]);
  assert.equal(result.items[0].relativePath, 'media/item.png');
  assert.equal(result.items[0].hint, true);
  assert.equal(result.items[0].lastNotifiedDue, 20);
  assert.equal('folder_id' in result, false);
});

test('drafts and file selections translate across the transport boundary', () => {
  assert.equal(draftToTransport({ kind: 'text', text: 'A clue', hint: true }, 'text').hint, true);
  assert.deepEqual(draftToTransport({ id: 'item', kind: 'text', text: 'Remember', sourcePath: undefined }, 'text'), {
    id: 'item',
    kind: 'text',
    text: 'Remember',
    source_path: undefined,
  });
  assert.deepEqual(
    mediaFromTransport({
      source_path: '/image.png',
      original_name: 'image.png',
      preview_url: '/preview/id',
      byte_size: 8,
    }),
    {
      sourcePath: '/image.png',
      originalName: 'image.png',
      previewURL: '/preview/id',
      byteSize: 8,
    },
  );
  assert.deepEqual(
    cardFromTransport({ id: 'card', folder_id: 'folder', created_at: 1, due_at: 2, last_reviewed: 0 }, 'text').items,
    [],
  );
});

test('configuration mapping owns frontend names and text identifiers', () => {
  const settings = configurationFromTransport({
    localization_changed: 'custom:language',
    preferences_storage_key: 'custom.preferences',
    media_route_prefix: '/saved/',
    text_kind: 'plain',
    schedules_changed: 'custom:changed',
    show_reviews: 'custom:show',
    open_floating_review: 'custom:open',
    close_floating_review: 'custom:close',
  });
  assert.deepEqual(settings, {
    localizationChanged: 'custom:language',
    preferencesStorageKey: 'custom.preferences',
    mediaRoutePrefix: '/saved/',
    textKind: 'plain',
    schedulesChanged: 'custom:changed',
    showReviews: 'custom:show',
    openFloatingReview: 'custom:open',
    closeFloatingReview: 'custom:close',
  });
  const draft = draftToTransport({ kind: 'text', text: 'Remember' }, settings.textKind);
  assert.equal(draft.kind, 'plain');
  const card = cardFromTransport(
    {
      id: 'card',
      folder_id: 'folder',
      items: [
        {
          id: 'item',
          kind: draft.kind,
          schedule: { due: '2026-10-20T12:00:00Z', last_review: '2026-10-16T12:00:00Z' },
          reviews: [],
        },
      ],
    },
    settings.textKind,
  );
  assert.equal(card.items[0].kind, 'text');
});
