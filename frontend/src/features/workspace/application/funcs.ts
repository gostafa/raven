import type { Card, ItemDraft } from '../../cards';
import { hasContent } from '../../cards/domain';
import type { Workspace } from '../../folders';
import { reconcileFolders } from '../../folders';
import type { AppPorts } from '../domain/types';
export async function loadWorkspace(ports: AppPorts, workspace: Workspace) {
  const [folders, cards] = await Promise.all([ports.ListFolders(), ports.ListCards()]);
  return { workspace: reconcileFolders(workspace, folders), cards, now: ports.now() };
}
export async function saveCard(
  ports: AppPorts,
  input: { folderId: string; newFolderTitle: string; editing: Card | null; items: ItemDraft[] },
  onFolderCreated: (folder: Awaited<ReturnType<AppPorts['CreateFolder']>>) => void,
) {
  if (!hasContent(input.items) || (input.folderId === '__new' && !input.newFolderTitle.trim()))
    throw new Error('Add something you want to remember and choose a folder for this card.');
  let folderId = input.folderId;
  if (folderId === '__new') {
    const folder = await ports.CreateFolder({ title: input.newFolderTitle, description: '', tags: [] });
    folderId = folder.id;
    onFolderCreated(folder);
  }
  return input.editing
    ? ports.UpdateCard({ folderId: input.editing.folderId, cardId: input.editing.id, items: input.items })
    : ports.CreateCard({ folderId, items: input.items });
}
export async function deleteSelection(ports: AppPorts, target: { folderId: string; cardId?: string }) {
  if (target.cardId !== undefined && target.cardId.length > 0) await ports.DeleteCard(target.folderId, target.cardId);
  else await ports.DeleteFolder(target.folderId);
}
