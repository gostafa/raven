import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { mkdtemp, mkdir, readFile, realpath, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { createRequire } from 'node:module';
import { test } from 'node:test';

const require = createRequire(import.meta.url);
const actionPath = path.dirname(new URL(import.meta.url).pathname);

test('the action can load both helper entry points from its own directory', async () => {
  const action = await readFile(path.join(actionPath, 'action.yml'), 'utf8');
  assert.ok(action.includes('process.env.PUBLISH_ACTION_PATH}/funcs.cjs'));
  const helper = require(path.join(actionPath, 'funcs.cjs'));
  assert.equal(typeof helper.prepare, 'function');
  assert.equal(typeof helper.verifyAssets, 'function');
});

test('prepare isolates downloads and rejects existing releases and API failures', async (t) => {
  const { prepare } = require('./funcs.cjs');
  const root = await mkdtemp(path.join(tmpdir(), 'raven-publish-test-'));
  t.after(() => rm(root, { recursive: true, force: true }));
  const outputs = {};
  const args = {
    context: { repo: { owner: 'owner', repo: 'repo' } },
    core: {
      setOutput: (key, value) => {
        outputs[key] = value;
      },
    },
    env: {
      RUNNER_TEMP: root,
      RELEASE_TAG: 'v0.0.0',
      RELEASE_FILES: 'artifact/raven.msi',
      ARTIFACT_PATTERN: 'artifact*',
      VERIFY_CHECKSUMS: 'true',
    },
    github: {
      rest: {
        repos: {
          getReleaseByTag: async (request) => {
            assert.deepEqual(request, { owner: 'owner', repo: 'repo', tag: 'v0.0.0' });
            throw Object.assign(new Error('missing'), { status: 404 });
          },
        },
      },
    },
  };
  await prepare(args);
  const first = outputs['download-path'];
  assert.equal(path.dirname(first), root);
  await prepare(args);
  assert.notEqual(outputs['download-path'], first);
  args.github.rest.repos.getReleaseByTag = async () => ({ data: { id: 1 } });
  await assert.rejects(prepare(args), /already exists/);
  args.github.rest.repos.getReleaseByTag = async () => {
    throw Object.assign(new Error('forbidden'), { status: 403 });
  };
  await assert.rejects(prepare(args), /forbidden/);
});

test('verify returns exact assets and sidecars and rejects missing or corrupt assets', async (t) => {
  const { verifyAssets } = require('./funcs.cjs');
  const root = await mkdtemp(path.join(tmpdir(), 'raven-publish-test-'));
  t.after(() => rm(root, { recursive: true, force: true }));
  await mkdir(path.join(root, 'artifact'));
  const file = path.join(await realpath(root), 'artifact/raven.msi');
  const hash = createHash('sha256').update('MSI contents').digest('hex');
  await writeFile(file, 'MSI contents');
  await writeFile(`${file}.sha256`, `${hash}  raven.msi\r\n`);
  const env = { DOWNLOAD_PATH: root, RELEASE_FILES: 'artifact/raven.msi', VERIFY_CHECKSUMS: 'true' };
  assert.deepEqual(await verifyAssets(env), [file, `${file}.sha256`]);
  await writeFile(file, 'corrupted');
  await assert.rejects(verifyAssets(env), /checksum mismatch/i);
  env.VERIFY_CHECKSUMS = 'false';
  assert.deepEqual(await verifyAssets(env), [file]);
  env.RELEASE_FILES = '../outside.msi';
  await assert.rejects(verifyAssets(env), /relative asset path/);
  env.RELEASE_FILES = 'artifact/*.msi';
  await assert.rejects(verifyAssets(env), /relative asset path/);
  env.RELEASE_FILES = 'artifact/missing.msi';
  await assert.rejects(verifyAssets(env), /ENOENT/);
  env.RELEASE_FILES = 'artifact/raven.msi';
  env.VERIFY_CHECKSUMS = 'true';
  await rm(`${file}.sha256`);
  await assert.rejects(verifyAssets(env), /ENOENT/);
});
