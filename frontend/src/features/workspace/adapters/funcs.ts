import { Events } from '@wailsio/runtime';
import { localizationFromTransport } from './transport/funcs';
import { RavenService } from '../../../../bindings/raven';
import { createMediaURL, createNativePreviewStarter } from '../../cards';
import type { AppPorts, RuntimeConfiguration } from '../domain/types';
import {
  configurationFromTransport,
  parseTransport,
  folderFromTransport,
  cardFromTransport,
  draftToTransport,
  mediaFromTransport,
  archiveFromTransport,
} from './transport/funcs';
import type {
  TransportFolder,
  TransportCard,
  TransportMedia,
  TransportConfiguration,
  TransportArchiveResult,
} from './transport/types';
async function call<T>(action: () => Promise<T>): Promise<T> {
  try {
    return await action();
  } catch (error) {
    const cause: unknown = error instanceof Error ? error.cause : null;
    const code = cause !== null && typeof cause === 'object' && 'code' in cause ? String(cause.code) : 'action_failed';
    // eslint-disable-next-line preserve-caught-error -- Keep implementation and transport details outside the frontend-owned error contract.
    throw new Error(code);
  }
}
export function createRuntime(configuration: RuntimeConfiguration): AppPorts {
  return {
    configuration,
    GetLocalization: () =>
      call(async () => localizationFromTransport(parseTransport(await RavenService.GetLocalization()))),
    SetLanguagePreference: (preference) =>
      call(async () => localizationFromTransport(parseTransport(await RavenService.SetLanguagePreference(preference)))),
    onLocalizationChanged: (callback) =>
      Events.On(configuration.localizationChanged, (event) => {
        callback(localizationFromTransport(event.data));
      }),
    mediaURL: createMediaURL(configuration.mediaRoutePrefix),
    previewStart: createNativePreviewStarter({
      once: (name, callback) => Events.Once(name, callback),
      emit: (name) => Events.Emit(name),
      newKey: () => crypto.randomUUID(),
    }),
    ListFolders: () =>
      call(async () =>
        ((parseTransport(await RavenService.ListFolders()) as TransportFolder[] | null) ?? []).map(folderFromTransport),
      ),
    CreateFolder: (input) =>
      call(async () =>
        folderFromTransport(parseTransport(await RavenService.CreateFolder(JSON.stringify(input))) as TransportFolder),
      ),
    UpdateFolder: (input) =>
      call(async () =>
        folderFromTransport(parseTransport(await RavenService.UpdateFolder(JSON.stringify(input))) as TransportFolder),
      ),
    DeleteFolder: (id) => call(() => RavenService.DeleteFolder(id)),
    ExportFolders: (folderIds) =>
      call(async () =>
        archiveFromTransport(
          parseTransport(
            await RavenService.ExportFolders(JSON.stringify({ folder_ids: folderIds })),
          ) as TransportArchiveResult,
        ),
      ),
    ImportFolders: (mode) =>
      call(async () =>
        archiveFromTransport(parseTransport(await RavenService.ImportFolders(mode)) as TransportArchiveResult),
      ),
    ListCards: () =>
      call(async () =>
        ((parseTransport(await RavenService.ListCards()) as TransportCard[] | null) ?? []).map((card) =>
          cardFromTransport(card, configuration.textKind),
        ),
      ),
    CreateCard: (input) =>
      call(async () =>
        cardFromTransport(
          parseTransport(
            await RavenService.CreateCard(
              JSON.stringify({
                folder_id: input.folderId,
                items: input.items.map((draft) => draftToTransport(draft, configuration.textKind)),
              }),
            ),
          ) as TransportCard,
          configuration.textKind,
        ),
      ),
    UpdateCard: (input) =>
      call(async () =>
        cardFromTransport(
          parseTransport(
            await RavenService.UpdateCard(
              JSON.stringify({
                folder_id: input.folderId,
                card_id: input.cardId,
                items: input.items.map((draft) => draftToTransport(draft, configuration.textKind)),
              }),
            ),
          ) as TransportCard,
          configuration.textKind,
        ),
      ),
    DeleteCard: (folder, card) => call(() => RavenService.DeleteCard(folder, card)),
    ReviewItem: (folder, card, item, rating) =>
      call(async () =>
        cardFromTransport(
          parseTransport(
            await RavenService.ReviewItem(
              JSON.stringify({
                folder_id: folder,
                card_id: card,
                item_id: item,
                rating,
              }),
            ),
          ) as TransportCard,
          configuration.textKind,
        ),
      ),
    SelectMediaFiles: (kind) =>
      call(async () =>
        ((parseTransport(await RavenService.SelectMediaFiles(kind)) as TransportMedia[] | null) ?? []).map(
          mediaFromTransport,
        ),
      ),
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
  return call(async () =>
    configurationFromTransport(parseTransport(await RavenService.GetConfiguration()) as TransportConfiguration),
  );
}
