import { Events } from '@wailsio/runtime';
import { RavenService } from '../../../../bindings/raven';
import { createMediaURL } from '../../cards';
import type { AppPorts, RuntimeConfiguration } from '../domain/types';
import {
  configurationFromTransport,
  folderFromTransport,
  cardFromTransport,
  draftToTransport,
  mediaFromTransport,
} from './transport/funcs';
async function call<T>(action: () => Promise<T>): Promise<T> {
  try {
    return await action();
  } catch (error) {
    // eslint-disable-next-line preserve-caught-error -- The facade deliberately exposes an owned message without transport error chains.
    throw new Error(error instanceof Error ? error.message : 'Raven could not complete that action. Please try again.');
  }
}
export function createRuntime(configuration: RuntimeConfiguration): AppPorts {
  return {
    configuration,
    mediaURL: createMediaURL(configuration.mediaRoutePrefix),
    ListFolders: () => call(async () => ((await RavenService.ListFolders()) ?? []).map(folderFromTransport)),
    CreateFolder: (input) => call(async () => folderFromTransport(await RavenService.CreateFolder(input))),
    DeleteFolder: (id) => call(() => RavenService.DeleteFolder(id)),
    ListCards: () =>
      call(async () =>
        ((await RavenService.ListCards()) ?? []).map((card) => cardFromTransport(card, configuration.textKind)),
      ),
    CreateCard: (input) =>
      call(async () =>
        cardFromTransport(
          await RavenService.CreateCard({
            folder_id: input.folderId,
            items: input.items.map((draft) => draftToTransport(draft, configuration.textKind)),
          }),
          configuration.textKind,
        ),
      ),
    UpdateCard: (input) =>
      call(async () =>
        cardFromTransport(
          await RavenService.UpdateCard({
            folder_id: input.folderId,
            card_id: input.cardId,
            items: input.items.map((draft) => draftToTransport(draft, configuration.textKind)),
          }),
          configuration.textKind,
        ),
      ),
    DeleteCard: (folder, card) => call(() => RavenService.DeleteCard(folder, card)),
    ReviewItem: (folder, card, item, rating) =>
      call(async () =>
        cardFromTransport(
          await RavenService.ReviewItem({
            folder_id: folder,
            card_id: card,
            item_id: item,
            rating,
          }),
          configuration.textKind,
        ),
      ),
    SelectMediaFiles: (kind) =>
      call(async () => ((await RavenService.SelectMediaFiles(kind)) ?? []).map(mediaFromTransport)),
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
      onSchedulesChanged: (callback) => Events.On(configuration.schedulesChanged, callback),
      onShowReviews: (callback) => Events.On(configuration.showReviews, callback),
      onOpenFloatingReview: (callback) => Events.On(configuration.openFloatingReview, callback),
      onCloseFloatingReview: (callback) => Events.On(configuration.closeFloatingReview, callback),
    },
  };
}

export async function loadConfiguration(): Promise<RuntimeConfiguration> {
  return call(async () => configurationFromTransport(await RavenService.GetConfiguration()));
}
