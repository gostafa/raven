import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { spawnSync } from 'node:child_process';
import { chmod, mkdir, mkdtemp, readFile, readdir, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { test } from 'node:test';

const root = fileURLToPath(new URL('../../../', import.meta.url));
const helper = path.join(root, '.github/actions/build-macos/funcs.sh');

test('the real packaging task uses the requested bundle directory throughout the build', () => {
  for (const output of [undefined, path.join(tmpdir(), 'raven macos output', 'build')]) {
    const args = ['--dry', '--force', 'darwin:package', 'ARCH=arm64', 'DEV=false'];
    if (output) args.push(`BIN_DIR=${output}`);
    const result = spawnSync('task', args, { cwd: root, encoding: 'utf8' });
    assert.equal(result.status, 0, result.stderr);
    const commands = result.stdout + result.stderr;
    const directory = output ?? 'bin';
    assert.ok(commands.includes(`prepare-macos-output --dir "${directory}"`), commands);
    assert.ok(commands.includes(`-o "${directory}/raven"`), commands);
    assert.ok(commands.includes(`mkdir -p "${directory}/raven.app/Contents/MacOS"`), commands);
    assert.ok(commands.includes(`codesign --verify --deep --strict "${directory}/raven.app"`), commands);
    if (output) assert.ok(!commands.includes('"bin/'), commands);
  }
});

async function fixture(t) {
  const directory = await mkdtemp(path.join(tmpdir(), 'raven macos test '));
  t.after(() => rm(directory, { recursive: true, force: true }));
  const tools = path.join(directory, 'mock-tools');
  const temporary = path.join(directory, 'temp');
  await mkdir(tools);
  await mkdir(temporary);
  const scripts = {
    uname: 'if [ "$1" = -s ]; then echo "${TEST_OS:-Darwin}"; else echo "${TEST_MACHINE:-arm64}"; fi',
    go: `if [ "$1" = list ]; then
      echo v3.0.0-beta.29
    else
      test "$GOBIN" = "$(dirname "$GOBIN")/tools"
      test "$2" = github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.29
    fi`,
    bun: 'exit 0',
    task: `test "$1" = darwin:package
      test "$2" = "ARCH=$PACKAGE_ARCH"
      test "$3" = DEV=false
      output="\${4#BIN_DIR=}"
      mkdir -p "$output/raven.app/Contents/MacOS" "$output/raven.app/Contents/Resources"
      printf 'CFBundleExecutable=raven\nCFBundleVersion=0.1.0\nCFBundleShortVersionString=0.1.0\n' > "$output/raven.app/Contents/Info.plist"
      printf executable > "$output/raven.app/Contents/MacOS/raven"
      chmod +x "$output/raven.app/Contents/MacOS/raven"
      for resource in icons.icns raven.wav Assets.car; do
        printf resource > "$output/raven.app/Contents/Resources/$resource"
      done`,
    ditto: 'cp -R "$1" "$2"',
    plutil: `case "$1" in
      -replace) printf '%s=%s\n' "$2" "$4" >> "$5" ;;
      -extract) awk -F= -v key="$2" '$1 == key { value=$2 } END { print value }' "$6" ;;
      -lint) test -f "$2" ;;
      *) exit 44 ;;
    esac`,
    xattr: 'exit 0',
    codesign: 'exit 0',
    lipo: 'echo "${TEST_BINARY_ARCH:-arm64}"',
    git: 'exit 0',
    mount: `if [ -f "$TEST_ROOT/mounted-path" ]; then
      printf '/dev/fixture on %s (hfs, local, read-only)\n' "$(cat "$TEST_ROOT/mounted-path")"
    fi`,
    hdiutil: `case "$1" in
      create)
        while [ "$#" -gt 1 ]; do
          if [ "$1" = -srcfolder ]; then shift; printf '%s' "$1" > "$TEST_ROOT/image-dir"; fi
          shift
        done
        printf 'disk image' > "$1"
        ;;
      verify) test -s "$2" ;;
      attach)
        test "$2" = -readonly
        test "$3" = -nobrowse
        test "$4" = -mountpoint
        cp -R "$(cat "$TEST_ROOT/image-dir")/." "$5/"
        printf '%s' "$5" > "$TEST_ROOT/mounted-path"
        if [ -n "\${TEST_MISSING_RESOURCE:-}" ]; then
          rm "$5/raven.app/Contents/Resources/$TEST_MISSING_RESOURCE"
        fi
        if [ "\${TEST_BAD_VERSION:-false}" = true ]; then
          printf 'CFBundleVersion=0.0.0\n' >> "$5/raven.app/Contents/Info.plist"
        fi
        if [ "\${TEST_BAD_SHORTCUT:-false}" = true ]; then
          rm "$5/Applications"
          ln -s /tmp "$5/Applications"
        fi
        if [ "\${TEST_PARTIAL_ATTACH:-false}" = true ]; then exit 42; fi
        ;;
      detach)
        test -f "$TEST_ROOT/mounted-path" || exit 1
        if [ "\${TEST_BUSY_MOUNT:-false}" = true ] && [ "$2" != -force ]; then exit 42; fi
        rm "$TEST_ROOT/mounted-path"
        ;;
      *) exit 44 ;;
    esac`,
  };
  for (const [name, script] of Object.entries(scripts)) {
    const file = path.join(tools, name);
    await writeFile(
      file,
      `#!/bin/bash
set -eu
printf '%s:%s\n' '${name}' "$*" >> "$TEST_ROOT/tool-log"
action=\${1:-}
if [ '${name}' = codesign ] && [ "$action" = --verify ] && [[ "\${*: -1}" = */mounted/* ]]; then
  action=mounted-verify
fi
if [ "\${TEST_FAIL:-}" = "${name}:$action" ]; then exit 42; fi
${script}
`,
    );
    await chmod(file, 0o755);
  }
  const run = (extra = {}) =>
    spawnSync('/bin/bash', [helper], {
      cwd: directory,
      env: {
        ...process.env,
        PATH: `${tools}:${process.env.PATH}`,
        TEST_ROOT: directory,
        GITHUB_WORKSPACE: directory,
        GITHUB_OUTPUT: path.join(directory, 'github-output'),
        RUNNER_TEMP: temporary,
        RELEASE_TAG: 'v1.2.3',
        PACKAGE_ARCH: 'arm64',
        TEST_MACHINE: 'arm64',
        TEST_BINARY_ARCH: 'arm64',
        ...extra,
      },
      encoding: 'utf8',
    });
  return { directory, temporary, run };
}

async function noOutputs(directory, temporary) {
  await assert.rejects(readFile(path.join(directory, 'github-output')), /ENOENT/);
  await assert.rejects(readdir(path.join(directory, 'bin')), /ENOENT/);
  await assert.rejects(readFile(path.join(directory, 'mounted-path')), /ENOENT/);
  assert.deepEqual(await readdir(temporary), []);
}

test('both native architectures produce a verified, versioned DMG and checksum', async (t) => {
  for (const [arch, machine] of [
    ['amd64', 'x86_64'],
    ['arm64', 'arm64'],
  ]) {
    await t.test(arch, async (t) => {
      const { directory, temporary, run } = await fixture(t);
      const result = run({ PACKAGE_ARCH: arch, TEST_MACHINE: machine, TEST_BINARY_ARCH: machine });
      assert.equal(result.status, 0, result.stderr);
      const artifact = `raven-v1.2.3-macos-${arch}`;
      const bytes = await readFile(path.join(directory, `bin/${artifact}.dmg`));
      const hash = createHash('sha256').update(bytes).digest('hex');
      assert.equal(
        await readFile(path.join(directory, `bin/${artifact}.dmg.sha256`), 'utf8'),
        `${hash}  ${artifact}.dmg\n`,
      );
      assert.equal(
        await readFile(path.join(directory, 'github-output'), 'utf8'),
        `artifact-name=${artifact}\ndmg-path=bin/${artifact}.dmg\nchecksum-path=bin/${artifact}.dmg.sha256\n`,
      );
      const log = await readFile(path.join(directory, 'tool-log'), 'utf8');
      assert.match(log, /go:install github.com\/wailsapp\/wails\/v3\/cmd\/wails3@v3\.0\.0-beta\.29/);
      assert.ok(log.includes(`task:darwin:package ARCH=${arch} DEV=false BIN_DIR=`));
      for (const key of ['CFBundleShortVersionString', 'CFBundleVersion']) {
        assert.ok(log.includes(`plutil:-replace ${key} -string 1.2.3 `));
        assert.ok(log.includes(`plutil:-extract ${key} raw -o - `));
      }
      assert.ok(log.indexOf('plutil:-replace CFBundleVersion') < log.indexOf('codesign:--force'));
      assert.ok(log.indexOf('hdiutil:verify') < log.indexOf('hdiutil:attach'));
      assert.ok(log.lastIndexOf('codesign:--verify') < log.indexOf('hdiutil:detach'));
      assert.deepEqual(await readdir(temporary), []);
      await assert.rejects(readFile(path.join(directory, 'mounted-path')), /ENOENT/);
    });
  }
});

test('invalid tags and mismatching or unsupported hosts fail before building', async (t) => {
  const { directory, temporary, run } = await fixture(t);
  for (const tag of ['', '1.2.3', 'v01.2.3', 'v1.2.3-beta', 'v256.0.0', 'v0.0.65536', 'v1.2.3\n']) {
    assert.notEqual(run({ RELEASE_TAG: tag }).status, 0, tag);
  }
  for (const extra of [{ TEST_OS: 'Linux' }, { PACKAGE_ARCH: '../../other' }, { PACKAGE_ARCH: 'amd64' }]) {
    assert.notEqual(run(extra).status, 0);
  }
  await noOutputs(directory, temporary);
  const log = await readFile(path.join(directory, 'tool-log'), 'utf8');
  assert.ok(!log.includes('go:') && !log.includes('task:'));
});

test('tool failures never promote or expose a release artifact', async (t) => {
  for (const failure of [
    'go:install',
    'task:darwin:package',
    'codesign:--force',
    'hdiutil:create',
    'hdiutil:verify',
    'hdiutil:attach',
    'codesign:mounted-verify',
    'git:diff',
  ]) {
    await t.test(failure, async (t) => {
      const { directory, temporary, run } = await fixture(t);
      const result = run({ TEST_FAIL: failure });
      assert.equal(result.status, 42, result.stderr);
      await noOutputs(directory, temporary);
    });
  }
});

test('mounted images must contain the requested architecture, versions, resources, and shortcut', async (t) => {
  for (const extra of [
    { TEST_BINARY_ARCH: 'x86_64' },
    { TEST_BAD_VERSION: 'true' },
    { TEST_BAD_SHORTCUT: 'true' },
    ...['icons.icns', 'raven.wav', 'Assets.car'].map((resource) => ({ TEST_MISSING_RESOURCE: resource })),
  ]) {
    await t.test(JSON.stringify(extra), async (t) => {
      const { directory, temporary, run } = await fixture(t);
      assert.notEqual(run(extra).status, 0);
      await noOutputs(directory, temporary);
      const log = await readFile(path.join(directory, 'tool-log'), 'utf8');
      assert.match(log, /hdiutil:detach/);
    });
  }
});

test('partial mounts and failed detaches are cleaned up without releasing the DMG', async (t) => {
  for (const extra of [{ TEST_PARTIAL_ATTACH: 'true' }, { TEST_BUSY_MOUNT: 'true' }]) {
    await t.test(JSON.stringify(extra), async (t) => {
      const { directory, temporary, run } = await fixture(t);
      assert.equal(run(extra).status, 42);
      await noOutputs(directory, temporary);
      if (extra.TEST_BUSY_MOUNT) {
        assert.match(await readFile(path.join(directory, 'tool-log'), 'utf8'), /hdiutil:detach -force/);
      }
    });
  }
});

test('release publication and cleanup require both macOS builds', async () => {
  const workflow = Bun.YAML.parse(await readFile(path.join(root, '.github/workflows/release.yml'), 'utf8'));
  assert.deepEqual(workflow.jobs.macos.strategy.matrix.include, [
    { arch: 'amd64', runner: 'macos-15-intel' },
    { arch: 'arm64', runner: 'macos-15' },
  ]);
  const publish = workflow.jobs.publish;
  assert.ok(publish.needs.includes('macos'));
  assert.ok(workflow.jobs.cleanup.needs.includes('macos'));
  assert.equal(publish.if, "github.event_name == 'push'");
  const inputs = publish.steps.find((step) => step.uses === '$/.github/actions/publish-release').with;
  assert.equal(inputs['include-checksums'], 'false');
  for (const arch of ['amd64', 'arm64']) {
    const artifact = `raven-\${{ github.ref_name }}-macos-${arch}`;
    assert.ok(inputs.files.split('\n').includes(`${artifact}/${artifact}.dmg`));
  }
  const action = Bun.YAML.parse(await readFile(path.join(root, '.github/actions/build-macos/action.yml'), 'utf8'));
  assert.ok(
    action.runs.steps.findIndex((step) => step.id === 'build') <
      action.runs.steps.findIndex((step) => step.id === 'upload'),
  );
  assert.equal(action.runs.steps.find((step) => step.id === 'upload').with['if-no-files-found'], 'error');
  assert.ok(action.runs.steps.every((step) => !step['continue-on-error']));
});
