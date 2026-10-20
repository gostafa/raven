const { createHash } = require('node:crypto');
const { createReadStream } = require('node:fs');
const { lstat, mkdtemp, readFile, realpath } = require('node:fs/promises');
const path = require('node:path');

function assetPaths(env) {
  const files = (env.RELEASE_FILES || '')
    .split(/\r?\n/)
    .map((file) => file.trim())
    .filter(Boolean);
  if (files.length === 0) throw new Error('RELEASE_FILES must select at least one asset');
  for (const file of files) {
    if (
      path.posix.isAbsolute(file) ||
      /[\\:*?\[\]{}!\x00-\x1f]/.test(file) ||
      file.split('/').some((part) => !part || part === '.' || part === '..')
    ) {
      throw new Error(`Expected an exact relative asset path: ${file}`);
    }
  }
  if (new Set(files).size !== files.length) throw new Error('Duplicate release asset paths');
  const names = files.map((file) => path.posix.basename(file));
  if (new Set(names).size !== names.length) throw new Error('Release asset filenames must be unique');
  return files;
}

function checksumsEnabled(env) {
  const value = env.VERIFY_CHECKSUMS ?? 'true';
  if (value !== 'true' && value !== 'false') throw new Error('VERIFY_CHECKSUMS must be true or false');
  return value === 'true';
}

async function prepare({ github, context, core, env }) {
  assetPaths(env);
  checksumsEnabled(env);
  if (!env.RELEASE_TAG || !env.ARTIFACT_PATTERN || !env.RUNNER_TEMP) {
    throw new Error('RELEASE_TAG, ARTIFACT_PATTERN, and RUNNER_TEMP are required');
  }
  try {
    await github.rest.repos.getReleaseByTag({ ...context.repo, tag: env.RELEASE_TAG });
  } catch (error) {
    if (error.status !== 404) throw error;
    const directory = await mkdtemp(path.join(env.RUNNER_TEMP, 'raven-release-'));
    core.setOutput('download-path', directory);
    return;
  }
  throw new Error(`Release ${env.RELEASE_TAG} already exists`);
}

async function regularAsset(root, relative) {
  const file = path.join(root, relative);
  const resolved = await realpath(file);
  const within = path.relative(root, resolved);
  if (
    within.startsWith(`..${path.sep}`) ||
    path.isAbsolute(within) ||
    within === '..' ||
    !(await lstat(file)).isFile()
  ) {
    throw new Error(`Expected a regular asset inside the download directory: ${relative}`);
  }
  return file;
}

async function verifyAssets(env) {
  const selected = assetPaths(env);
  const verify = checksumsEnabled(env);
  if (!env.DOWNLOAD_PATH) throw new Error('DOWNLOAD_PATH is required');
  const root = await realpath(env.DOWNLOAD_PATH);
  const files = [];
  for (const relative of selected) {
    const file = await regularAsset(root, relative);
    files.push(file);
    if (!verify) continue;
    const sidecar = await regularAsset(root, `${relative}.sha256`);
    const checksum = (await readFile(sidecar, 'utf8')).trim();
    const match = /^([a-fA-F0-9]{64}) [ *]([^\r\n]+)$/.exec(checksum);
    if (!match || match[2] !== path.basename(file)) {
      throw new Error(`Invalid SHA-256 sidecar for ${relative}`);
    }
    const hash = createHash('sha256');
    for await (const chunk of createReadStream(file)) hash.update(chunk);
    if (hash.digest('hex') !== match[1].toLowerCase()) {
      throw new Error(`SHA-256 checksum mismatch for ${relative}`);
    }
  }
  return files;
}

module.exports = { prepare, verifyAssets };
