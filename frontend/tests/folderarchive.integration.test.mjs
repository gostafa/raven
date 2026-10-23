import assert from 'node:assert/strict';
import { test } from 'node:test';
import { exportFolders, importFolders } from '../src/features/folderarchive/index.ts';
import { archiveFromTransport } from '../src/features/workspace/adapters/transport/funcs.ts';

test('export sends the complete selection once and rejects an empty selection', async () => {
  const selections = [];
  const result = { canceled: false, folderCount: 2 };
  const ports = {
    ExportFolders: async (ids) => {
      selections.push(ids);
      return result;
    },
  };
  assert.equal(await exportFolders(ports, ['first', 'second', 'first']), result);
  assert.deepEqual(selections, [['first', 'second']]);
  await assert.rejects(exportFolders(ports, []), /Select at least one folder/);
  await assert.rejects(exportFolders(ports, ['']), /Select at least one folder/);
  assert.equal(selections.length, 1);
});

test('both import modes refresh the library only after a completed import', async () => {
  for (const mode of ['fresh', 'resume']) {
    const calls = [];
    const result = { canceled: false, folderIds: ['new-folder'] };
    const ports = {
      ImportFolders: async (value) => {
        calls.push(value);
        return result;
      },
    };
    assert.equal(
      await importFolders(ports, mode, async () => {
        calls.push('refresh');
      }),
      result,
    );
    assert.deepEqual(calls, [mode, 'refresh']);
  }
});

test('canceled and failed imports preserve the current workspace snapshot', async () => {
  let refreshed = false;
  const refresh = async () => {
    refreshed = true;
  };
  const canceled = { canceled: true };
  assert.equal(await importFolders({ ImportFolders: async () => canceled }, 'fresh', refresh), canceled);
  const failure = new Error('Invalid archive');
  await assert.rejects(
    importFolders(
      {
        ImportFolders: async () => {
          throw failure;
        },
      },
      'resume',
      refresh,
    ),
    failure,
  );
  await assert.rejects(importFolders({}, '', refresh), /Choose how to start/);
  assert.equal(refreshed, false);
});

test('archive transport exposes frontend-owned completion and cancellation contracts', () => {
  assert.deepEqual(
    archiveFromTransport({
      canceled: true,
      filename: '',
      folder_ids: null,
      folder_count: 0,
      card_count: 0,
      media_count: 0,
    }),
    {
      canceled: true,
      filename: '',
      folderIds: [],
      folderCount: 0,
      cardCount: 0,
      mediaCount: 0,
    },
  );
});
