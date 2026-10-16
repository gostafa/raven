import { Events } from '@wailsio/runtime';
import { RavenService } from '../../../../bindings/raven';
import type { Card as TransportCard, Folder as TransportFolder } from '../../../../bindings/raven/models';
import { mediaURL, type Card } from '../../cards';
import type { Folder } from '../../folders';
import type { AppPorts } from '../domain/types';
function folderFromTransport(value: TransportFolder): Folder {
  return { ...value, tags: value.tags ?? [] };
}
function cardFromTransport(value: TransportCard): Card {
  return { ...value, items: (value.items ?? []).map((item) => ({ ...item, reviews: item.reviews ?? [] })) };
}
async function call<T>(action: () => Promise<T>): Promise<T> {
  try {
    return await action();
  } catch (error) {
    // eslint-disable-next-line preserve-caught-error -- The facade deliberately exposes an owned message without transport error chains.
    throw new Error(error instanceof Error ? error.message : 'Raven could not complete that action. Please try again.');
  }
}
export function createRuntime(): AppPorts {
  return {
    mediaURL,
    ListFolders: () => call(async () => ((await RavenService.ListFolders()) ?? []).map(folderFromTransport)),
    CreateFolder: (input) => call(async () => folderFromTransport(await RavenService.CreateFolder(input))),
    DeleteFolder: (id) => call(() => RavenService.DeleteFolder(id)),
    ListCards: () => call(async () => ((await RavenService.ListCards()) ?? []).map(cardFromTransport)),
    CreateCard: (input) => call(async () => cardFromTransport(await RavenService.CreateCard(input))),
    UpdateCard: (input) => call(async () => cardFromTransport(await RavenService.UpdateCard(input))),
    DeleteCard: (folder, card) => call(() => RavenService.DeleteCard(folder, card)),
    ReviewItem: (folder, card, item, rating) =>
      call(async () => cardFromTransport(await RavenService.ReviewItem(folder, card, item, rating))),
    SelectMediaFiles: (kind) => call(async () => (await RavenService.SelectMediaFiles(kind)) ?? []),
    ShowMain: () => call(() => RavenService.ShowMain()),
    HideMain: () => call(() => RavenService.HideMain()),
    ResetWidget: () => call(() => RavenService.ResetWidget()),
    Quit: () => call(() => RavenService.Quit()),
    ShowReviews: () => call(() => RavenService.ShowReviews()),
    RefreshReminders: () => call(() => RavenService.RefreshReminders()),
    ToggleFloatingReview: () => call(() => RavenService.ToggleFloatingReview()),
    HideFloatingReview: () => call(() => RavenService.HideFloatingReview()),
    preferences: {
      getItem: (key) => window.localStorage.getItem(key),
      setItem: (key, value) => {
        window.localStorage.setItem(key, value);
      },
    },
    now: () => Date.now(),
    newKey: () => crypto.randomUUID(),
    events: {
      onSchedulesChanged: (callback) => Events.On('raven:schedules-changed', callback),
      onShowReviews: (callback) => Events.On('raven:show-reviews', callback),
      onOpenFloatingReview: (callback) => Events.On('raven:open-floating-review', callback),
      onCloseFloatingReview: (callback) => Events.On('raven:close-floating-review', callback),
    },
  };
}
