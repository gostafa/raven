import assert from 'node:assert/strict';
import { test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { ESLint } from 'eslint';

const eslint = new ESLint({ cwd: fileURLToPath(new URL('../../', import.meta.url)) });

test('production lint rejects large, complex functions and excessive parameters', async () => {
  const source = `export function oversized(first, second, third, fourth) {
${Array.from({ length: 11 }, (_, index) => `  if (first === ${String(index)}) second++;`).join('\n')}
${'\n'.repeat(10)}
  return [first, second, third, fourth];
}`;
  const [result] = await eslint.lintText(source, { filePath: 'scripts/raven.mjs' });
  const rules = new Set(result.messages.map((message) => message.ruleId));
  for (const rule of ['complexity', 'max-params', 'max-statements', 'max-lines-per-function']) {
    assert.ok(rules.has(rule), rule);
  }
});

test('type-aware Svelte lint catches unhandled promises and preserves reactive props', async () => {
  const [result] = await eslint.lintText(
    `<script lang="ts">
      let { value = 0 }: { value?: number } = $props();
      async function save(): Promise<void> { await Promise.resolve(); }
      save();
    </script><p>{value}</p>`,
    { filePath: 'frontend/src/App.svelte' },
  );
  assert.ok(result.messages.some((message) => message.ruleId === '@typescript-eslint/no-floating-promises'));
  assert.ok(!result.messages.some((message) => message.ruleId === 'prefer-const'));
  assert.ok(!result.messages.some((message) => message.fatal));
});

test('unchecked transport assertions, console logging and dynamic code remain errors', async () => {
  const [typed] = await eslint.lintText('export function unsafe(value: unknown): string { return value as string; }', {
    filePath: 'frontend/src/features/cards/domain/funcs.ts',
  });
  assert.ok(typed.messages.some((message) => message.ruleId === '@typescript-eslint/no-unsafe-type-assertion'));
  const [script] = await eslint.lintText('console.error("failed"); eval("untrusted");', {
    filePath: 'scripts/raven.mjs',
  });
  for (const rule of ['no-console', 'no-eval']) {
    assert.ok(
      script.messages.some((message) => message.ruleId === rule && message.severity === 2),
      rule,
    );
  }
});

test('lint scope isolates browser globals, checks test correctness and excludes generated assets', async () => {
  const browser = await eslint.calculateConfigForFile('frontend/src/App.svelte');
  const tests = await eslint.calculateConfigForFile('frontend/tests/cards.test.mjs');
  assert.equal(browser.languageOptions.globals.process, undefined);
  assert.equal(tests.rules.complexity, undefined);
  assert.equal(tests.rules['no-eval'][0], 2);
  assert.equal(browser.linterOptions.reportUnusedDisableDirectives, 2);
  for (const path of ['taskotter/fixture.js', 'frontend/bindings/fixture.ts', 'frontend/dist/fixture.js']) {
    assert.equal(await eslint.isPathIgnored(path), true, path);
  }
});
