/* eslint-disable security/detect-non-literal-fs-filename -- Test paths are isolated temporary directories and fixed repository fixtures. */
/* eslint-disable no-template-curly-in-string -- These strings are shell fixtures with shell parameter expansion. */
import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { createRequire } from 'node:module';
import { spawnSync } from 'node:child_process';
import { mkdtemp, mkdir, readFile, writeFile, chmod, cp, rm, realpath } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { test } from 'node:test';

const root = fileURLToPath(new URL('../../', import.meta.url));
const helper = path.join(root, 'taskfiles/linux/funcs.sh');

async function fixture(t, overrides = {}) {
  const directory = await mkdtemp(path.join(tmpdir(), 'raven linux test '));
  t.after(() => rm(directory, { recursive: true, force: true }));
  const tools = path.join(directory, 'mock-tools');
  await mkdir(tools);
  await writeFile(path.join(directory, 'go.mod'), 'module fixture\n\ngo 1.27.2\n');
  await cp(path.join(root, 'build'), path.join(directory, 'build'), { recursive: true });
  const scripts = {
    uname: 'if [ "$1" = -s ]; then echo "${TEST_OS:-Linux}"; else echo "${TEST_MACHINE:-x86_64}"; fi',
    go: 'echo go1.27.2',
    bun: 'echo 1.4.2',
    wails3: 'exit 42',
    cc: 'exit 0',
    'pkg-config': 'exit 0',
    ...overrides,
  };
  for (const [name, script] of Object.entries(scripts)) {
    if (script === null) continue;
    const file = path.join(tools, name);
    await writeFile(file, `#!/bin/sh\n${script}\n`);
    await chmod(file, 0o755);
  }
  const env = {
    ...process.env,
    PATH: `${tools}:${process.env.PATH}`,
    RAVEN_LINUX_ROOT: directory,
    RAVEN_LINUX_ARCH: 'amd64',
    RAVEN_LINUX_VERSION: '1.2.3',
    RAVEN_LINUX_TAG: '',
    RAVEN_LINUX_DEV: 'false',
    RAVEN_LINUX_FORMAT: 'all',
    RAVEN_LINUX_OBFUSCATED: 'false',
    GITHUB_OUTPUT: path.join(directory, 'github-output'),
  };
  const run = (command, extra = {}) =>
    spawnSync('/bin/bash', [helper, command], {
      cwd: directory,
      env: { ...env, ...extra },
      encoding: 'utf8',
    });
  return { directory, tools, run, env };
}

function ok(result) {
  assert.equal(result.status, 0, result.stderr || result.stdout);
}
function fails(result, message) {
  assert.notEqual(result.status, 0, 'command unexpectedly succeeded');
  assert.match(result.stderr, message);
}

test('only matching native AMD64 and ARM64 Linux hosts are accepted', async (t) => {
  const { run } = await fixture(t);
  ok(run('validate-host'));
  ok(run('validate-host', { RAVEN_LINUX_ARCH: 'arm64', TEST_MACHINE: 'aarch64' }));
  fails(run('validate-host', { TEST_OS: 'Darwin' }), /native Linux/);
  fails(run('validate-host', { TEST_MACHINE: 'aarch64' }), /does not match/);
  for (const arch of ['', 'arm', '386', '../../other']) {
    fails(run('validate-host', { RAVEN_LINUX_ARCH: arch }), /ARCH must be/);
  }
});

test('packaging validates stable versions, limits, development mode, tags, and formats', async (t) => {
  const { run } = await fixture(t);
  for (const version of ['0.0.0', '255.255.65535']) {
    ok(run('validate-package', { RAVEN_LINUX_VERSION: version }));
  }
  for (const version of ['', 'v1.2.3', '1.2', '01.2.3', '1.2.3-beta', '256.1.1', '1.1.65536', '1.2.3\n']) {
    fails(run('validate-package', { RAVEN_LINUX_VERSION: version }), /VERSION|limits/);
  }
  fails(run('validate-package', { RAVEN_LINUX_DEV: 'true' }), /DEV=false/);
  fails(run('validate-package', { RAVEN_LINUX_TAG: 'v2.0.0' }), /TAG must equal/);
  for (const format of ['aur', 'snap', 'appimage']) {
    fails(run('validate-package', { RAVEN_LINUX_FORMAT: format }), /FORMAT must be/);
  }
  fails(run('validate-release'), /TAG is required/);
  fails(run('validate-release', { RAVEN_LINUX_TAG: 'v1.2.3', RAVEN_LINUX_FORMAT: 'deb' }), /all three formats/);
});

test('missing build dependencies fail before frontend generation or package writes', async (t) => {
  const { run, tools, directory } = await fixture(t, { cc: null });
  fails(run('validate-build', { PATH: tools }), /Missing tool: cc/);
  await assert.rejects(readFile(path.join(directory, 'github-output')), /ENOENT/);
  await assert.rejects(readFile(path.join(directory, 'bin/.linux/amd64/prepared')), /ENOENT/);
});

test('a failed package command cannot be promoted or expose action outputs', async (t) => {
  const { directory, run } = await fixture(t);
  const stage = path.join(directory, 'bin/.linux/amd64');
  await mkdir(path.join(stage, 'payload/usr/bin'), { recursive: true });
  await mkdir(path.join(stage, 'packages'));
  await writeFile(path.join(stage, 'prepared'), '1.2.3|deb|\n');
  await writeFile(path.join(stage, 'payload/usr/bin/raven'), 'fixture binary');
  await chmod(path.join(stage, 'payload/usr/bin/raven'), 0o755);
  const env = { RAVEN_LINUX_FORMAT: 'deb' };
  assert.equal(run('bundle', env).status, 42);
  fails(run('promote', env), /must pass GUI/);
  await assert.rejects(readFile(path.join(directory, 'bin/raven-v1.2.3-linux-amd64.deb')), /ENOENT/);
  await assert.rejects(readFile(path.join(directory, 'github-output')), /ENOENT/);
  fails(run('bundle', { RAVEN_LINUX_FORMAT: 'rpm' }), /does not match/);
});

test('the release workflow requires both native Linux builds and keeps checksums private', async () => {
  const workflow = await readFile(path.join(root, '.github/workflows/release.yml'), 'utf8');
  const publish = workflow.slice(workflow.indexOf('  publish:'));
  assert.match(workflow, /runner: ubuntu-26\.04\n/);
  assert.match(workflow, /runner: ubuntu-26\.04-arm\n/);
  assert.match(publish, /if: github.event_name == 'push'/);
  assert.match(publish, /- windows\s+- linux/);
  assert.match(publish, /include-checksums: 'false'/);
  for (const arch of ['amd64', 'arm64']) {
    for (const extension of ['deb', 'rpm', 'pkg.tar.zst']) {
      assert.ok(publish.includes(`-linux-${arch}.${extension}\n`));
    }
  }
  const action = await readFile(path.join(root, '.github/actions/build-linux/action.yml'), 'utf8');
  assert.ok(action.indexOf('task linux:release') < action.indexOf('uses: actions/upload-artifact'));
  assert.ok(!action.includes('continue-on-error'));
});

test('Linux release permits sandbox namespaces and restores host policy after success or failure', async (t) => {
  const { directory, env } = await fixture(t, {
    sysctl: 'if [ "$1" = -n ]; then echo 1; else printf "%s\\n" "$2" >> "$RAVEN_LINUX_ROOT/policy-log"; fi',
    sudo: 'exec "$@"',
    task: 'grep -qx "kernel.apparmor_restrict_unprivileged_userns=0" "$RAVEN_LINUX_ROOT/policy-log" || exit 43; exit "$TEST_RELEASE_STATUS"',
  });
  const action = Bun.YAML.parse(await readFile(path.join(root, '.github/actions/build-linux/action.yml'), 'utf8'));
  const script = action.runs.steps.find((step) => step.id === 'build').run;
  for (const status of [0, 42]) {
    await rm(path.join(directory, 'policy-log'), { force: true });
    const result = spawnSync('/bin/bash', ['-e', '-c', script], {
      env: { ...env, RELEASE_TAG: 'v1.2.3', PACKAGE_ARCH: 'amd64', TEST_RELEASE_STATUS: String(status) },
      encoding: 'utf8',
    });
    assert.equal(result.status, status, result.stderr);
    assert.deepEqual((await readFile(path.join(directory, 'policy-log'), 'utf8')).trim().split('\n'), [
      'kernel.apparmor_restrict_unprivileged_userns=0',
      'kernel.apparmor_restrict_unprivileged_userns=1',
    ]);
  }
});

test('checksum publication is opt-in and requires valid checksums', async (t) => {
  const directory = await realpath(await mkdtemp(path.join(tmpdir(), 'raven-sidecars-')));
  t.after(() => rm(directory, { recursive: true, force: true }));
  await mkdir(path.join(directory, 'artifact'));
  const file = path.join(directory, 'artifact/raven.deb');
  await writeFile(file, 'package contents');
  const hash = createHash('sha256').update('package contents').digest('hex');
  await writeFile(`${file}.sha256`, `${hash}  raven.deb\n`);
  const action = Bun.YAML.parse(await readFile(path.join(root, '.github/actions/publish-release/action.yml'), 'utf8'));
  const script = action.runs.steps.find((step) => step.id === 'verify').with.script;
  const AsyncFunction = Object.getPrototypeOf(async () => {}).constructor;
  const verify = new AsyncFunction('require', 'process', 'core', script);
  const require = createRequire(import.meta.url);
  const env = {
    PUBLISH_ACTION_PATH: path.join(root, '.github/actions/publish-release'),
    DOWNLOAD_PATH: directory,
    RELEASE_FILES: 'artifact/raven.deb',
    VERIFY_CHECKSUMS: 'true',
    INCLUDE_CHECKSUMS: 'false',
  };
  let output;
  const core = {
    setOutput: (_name, value) => {
      output = value.split('\n');
    },
  };
  await verify(require, { env }, core);
  assert.deepEqual(output, [file]);
  env.INCLUDE_CHECKSUMS = 'true';
  await verify(require, { env }, core);
  assert.deepEqual(output, [file, `${file}.sha256`]);
  env.VERIFY_CHECKSUMS = 'false';
  await assert.rejects(verify(require, { env }, core), /requires verification/);
  env.VERIFY_CHECKSUMS = 'true';
  await writeFile(file, 'corrupted');
  await assert.rejects(verify(require, { env }, core), /checksum mismatch/i);
});
