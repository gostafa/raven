/* eslint-disable security/detect-non-literal-fs-filename -- Paths are fixed repository assets or local module-resolution candidates. */
import { existsSync } from 'node:fs';
import { delimiter, dirname, join } from 'node:path';
import { spawn } from 'node:child_process';
import { fileURLToPath } from 'node:url';

const root = fileURLToPath(new URL('../', import.meta.url));
const tools = join(root, '.tools');
const binary = process.platform === 'win32' ? 'wails3.exe' : 'wails3';
const local = join(tools, binary);
const localNodeDir = process.platform === 'win32' ? join(tools, 'node') : join(tools, 'node', 'bin');
const nodeDir = existsSync(join(localNodeDir, process.platform === 'win32' ? 'node.exe' : 'node'))
  ? localNodeDir
  : dirname(process.execPath);
const child = spawn(existsSync(local) ? local : binary, process.argv.slice(2), {
  cwd: root,
  stdio: 'inherit',
  env: {
    ...process.env,
    PATH: [tools, nodeDir, process.env.PATH].join(delimiter),
  },
});
child.on('error', (error) => {
  console.error(
    error.code === 'ENOENT'
      ? 'Install Wails first: go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.29'
      : error.message,
  );
  process.exitCode = 1;
});
child.on('exit', (code, signal) => {
  if (signal) process.kill(process.pid, signal);
  else process.exitCode = code ?? 1;
});
for (const signal of ['SIGINT', 'SIGTERM']) {
  process.on(signal, () => child.kill(signal));
}
