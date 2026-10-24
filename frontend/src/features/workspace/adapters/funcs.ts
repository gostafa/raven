import { Events } from '@wailsio/runtime';
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
  localizationFromTransport,
} from './transport/funcs';
import {
  decode,
  decodeList,
  isFolder,
  isCard,
  isMedia,
  isConfiguration,
  isArchive,
} from './transport/validation/funcs';

async function call<T>(action: () => Promise<T>): Promise<T> {
  try {
    return await action();
  } catch (error) {
    const cause: unknown = error instanceof Error ? error.cause : null;
    const code = transportErrorCode(cause);
    // eslint-disable-next-line preserve-caught-error -- Keep transport details outside the frontend-owned error contract.
    throw new Error(code);
  }
}
function transportErrorCode(cause: unknown): string {
  return cause !== null && typeof cause === 'object' && 'code' in cause ? String(cause.code) : 'action_failed';
}

class Runtime implements AppPorts {
  public readonly mediaURL;
  public readonly previewStart;
  public readonly events;
  public readonly preferences = {
    getItem: (key: string) => window.localStorage.getItem(key),
    setItem: (key: string, value: string) => {
      window.localStorage.setItem(key, value);
    },
  };
  public constructor(public readonly configuration: RuntimeConfiguration) {
    this.mediaURL = createMediaURL(configuration.mediaRoutePrefix);
    this.previewStart = createNativePreviewStarter({
      once: (name, callback) => Events.Once(name, callback),
      emit: (name) => Events.Emit(name),
      newKey: () => crypto.randomUUID(),
    });
    this.events = runtimeEvents(configuration);
  }
  public GetLocalization: AppPorts['GetLocalization'] = () =>
    call(async () => localizationFromTransport(parseTransport(await RavenService.GetLocalization())));
  public SetLanguagePreference: AppPorts['SetLanguagePreference'] = (preference) =>
    call(async () => localizationFromTransport(parseTransport(await RavenService.SetLanguagePreference(preference))));
  public onLocalizationChanged: AppPorts['onLocalizationChanged'] = (callback) =>
    Events.On(this.configuration.localizationChanged, (event) => {
      callback(localizationFromTransport(event.data));
    });
  public ListFolders: AppPorts['ListFolders'] = () =>
    call(async () => decodeList(parseTransport(await RavenService.ListFolders()), isFolder).map(folderFromTransport));
  public CreateFolder: AppPorts['CreateFolder'] = (input) =>
    call(async () =>
      folderFromTransport(decode(parseTransport(await RavenService.CreateFolder(JSON.stringify(input))), isFolder)),
    );
  public UpdateFolder: AppPorts['UpdateFolder'] = (input) =>
    call(async () =>
      folderFromTransport(decode(parseTransport(await RavenService.UpdateFolder(JSON.stringify(input))), isFolder)),
    );
  public DeleteFolder: AppPorts['DeleteFolder'] = (id) => call(() => RavenService.DeleteFolder(id));
  public ExportFolders: AppPorts['ExportFolders'] = (folderIds) =>
    call(async () =>
      archiveFromTransport(
        decode(parseTransport(await RavenService.ExportFolders(JSON.stringify({ folder_ids: folderIds }))), isArchive),
      ),
    );
  public ImportFolders: AppPorts['ImportFolders'] = (mode) =>
    call(async () => archiveFromTransport(decode(parseTransport(await RavenService.ImportFolders(mode)), isArchive)));
  public ListCards: AppPorts['ListCards'] = () =>
    call(async () =>
      decodeList(parseTransport(await RavenService.ListCards()), isCard).map((card) =>
        cardFromTransport(card, this.configuration.textKind),
      ),
    );
  public CreateCard: AppPorts['CreateCard'] = (input) =>
    call(async () =>
      cardFromTransport(
        decode(
          parseTransport(
            await RavenService.CreateCard(
              JSON.stringify({
                folder_id: input.folderId,
                items: input.items.map((draft) => draftToTransport(draft, this.configuration.textKind)),
              }),
            ),
          ),
          isCard,
        ),
        this.configuration.textKind,
      ),
    );
  public UpdateCard: AppPorts['UpdateCard'] = (input) =>
    call(async () =>
      cardFromTransport(
        decode(
          parseTransport(
            await RavenService.UpdateCard(
              JSON.stringify({
                folder_id: input.folderId,
                card_id: input.cardId,
                items: input.items.map((draft) => draftToTransport(draft, this.configuration.textKind)),
              }),
            ),
          ),
          isCard,
        ),
        this.configuration.textKind,
      ),
    );
  public DeleteCard: AppPorts['DeleteCard'] = (folder, card) => call(() => RavenService.DeleteCard(folder, card));
  public ReviewItem: AppPorts['ReviewItem'] = (input) =>
    call(async () =>
      cardFromTransport(
        decode(
          parseTransport(
            await RavenService.ReviewItem(
              JSON.stringify({
                folder_id: input.folderId,
                card_id: input.cardId,
                item_id: input.itemId,
                rating: input.rating,
              }),
            ),
          ),
          isCard,
        ),
        this.configuration.textKind,
      ),
    );
  public SelectMediaFiles: AppPorts['SelectMediaFiles'] = (kind) =>
    call(async () =>
      decodeList(parseTransport(await RavenService.SelectMediaFiles(kind)), isMedia).map(mediaFromTransport),
    );
  public ShowMain: AppPorts['ShowMain'] = () => call(() => RavenService.ShowMain());
  public HideMain: AppPorts['HideMain'] = () => call(() => RavenService.HideMain());
  public ResetWidget: AppPorts['ResetWidget'] = () => call(() => RavenService.ResetWidget());
  public Quit: AppPorts['Quit'] = () => call(() => RavenService.Quit());
  public ShowReviews: AppPorts['ShowReviews'] = () => call(() => RavenService.ShowReviews());
  public RefreshReminders: AppPorts['RefreshReminders'] = () => call(() => RavenService.RefreshReminders());
  public ToggleFloatingReview: AppPorts['ToggleFloatingReview'] = () => call(() => RavenService.ToggleFloatingReview());
  public HideFloatingReview: AppPorts['HideFloatingReview'] = () => call(() => RavenService.HideFloatingReview());
  public now = (): number => Date.now();
  public newKey = (): string => crypto.randomUUID();
}

function runtimeEvents(configuration: RuntimeConfiguration): AppPorts['events'] {
  return {
    onSchedulesChanged: (callback) => Events.On(configuration.schedulesChanged, callback),
    onShowReviews: (callback) => Events.On(configuration.showReviews, callback),
    onOpenFloatingReview: (callback) => Events.On(configuration.openFloatingReview, callback),
    onCloseFloatingReview: (callback) => Events.On(configuration.closeFloatingReview, callback),
  };
}

export function createRuntime(configuration: RuntimeConfiguration): AppPorts {
  return new Runtime(configuration);
}

export async function loadConfiguration(): Promise<RuntimeConfiguration> {
  return await call(async () =>
    configurationFromTransport(decode(parseTransport(await RavenService.GetConfiguration()), isConfiguration)),
  );
}
