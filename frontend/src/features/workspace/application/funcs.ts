import type { Card, ItemDraft } from '../../cards';
import { hasContent } from '../../cards/domain';
import type { PreferencesStorage, Workspace } from '../../folders';
import { reconcileFolders } from '../../folders';
import type { AppPorts, ViewPreferences } from '../domain/types';
import { defaultViewPreferences, parseViewPreferences } from '../domain/funcs';

export function loadViewPreferences(storage: Pick<PreferencesStorage, 'getItem'>, key: string): ViewPreferences {
  const saved = storage.getItem(`${key}.view`);
  return saved === null ? defaultViewPreferences() : parseViewPreferences(JSON.parse(saved));
}

export function saveViewPreferences(
  storage: Pick<PreferencesStorage, 'setItem'>,
  key: string,
  preferences: ViewPreferences,
): void {
  storage.setItem(`${key}.view`, JSON.stringify(preferences));
}
export async function loadWorkspace(
  ports: AppPorts,
  workspace: Workspace,
): Promise<{
  workspace: Workspace;
  cards: Card[];
  now: number;
}> {
  const [folders, cards] = await Promise.all([ports.ListFolders(), ports.ListCards()]);
  return { workspace: reconcileFolders(workspace, folders), cards, now: ports.now() };
}
export async function saveCard(
  ports: AppPorts,
  input: { folderId: string; newFolderTitle: string; editing: Card | null; items: ItemDraft[] },
  onFolderCreated: (folder: Awaited<ReturnType<AppPorts['CreateFolder']>>) => void,
): Promise<Card> {
  if (!validCardInput(input)) throw new Error('Add something you want to remember and choose a folder for this card.');
  let folderId = input.folderId;
  if (folderId === '__new') {
    const folder = await ports.CreateFolder({ title: input.newFolderTitle, description: '', tags: [] });
    folderId = folder.id;
    onFolderCreated(folder);
  }
  return input.editing
    ? await ports.UpdateCard({ folderId: input.editing.folderId, cardId: input.editing.id, items: input.items })
    : await ports.CreateCard({ folderId, items: input.items });
}
function validCardInput(input: { items: ItemDraft[]; folderId: string; newFolderTitle: string }): boolean {
  return hasContent(input.items) && (input.folderId !== '__new' || input.newFolderTitle.trim().length > 0);
}
export async function deleteSelection(ports: AppPorts, target: { folderId: string; cardId?: string }): Promise<void> {
  if (target.cardId !== undefined && target.cardId.length > 0) await ports.DeleteCard(target.folderId, target.cardId);
  else await ports.DeleteFolder(target.folderId);
}
