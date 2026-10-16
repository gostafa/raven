/* eslint-disable security/detect-non-literal-fs-filename -- Paths are fixed repository assets or local module-resolution candidates. */
import assert from 'node:assert/strict';
import { test } from 'node:test';
import { readFile, readdir } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import { checkSource } from '../tooling/conventions.mjs';

test('frontend source follows filename and architecture conventions', async () => {
  const root = new URL('../src/', import.meta.url);
  for (const file of await readdir(root, { recursive: true })) {
    if (!/\.(ts|svelte)$/.test(file) || file.endsWith('.d.ts')) continue;
    const url = new URL(file, root);
    const errors = checkSource(fileURLToPath(url), await readFile(url, 'utf8'));
    assert.deepEqual(errors, [], file);
  }
});

test('convention fixtures catch imports, re-exports, transport aliases, DOM and filenames', () => {
  for (const [filename, source] of [
    ['cards/domain/helpers.ts', 'export function value(){}'],
    ['cards/domain/funcs.ts', 'import {Events} from "@wailsio/runtime";'],
    ['cards/domain/funcs.ts', 'export * from "@wailsio/runtime";'],
    ['cards/domain/types.ts', 'type Card = import("../../../../bindings/raven/models").Card;'],
    ['cards/domain/funcs.ts', 'export function value(){return window.localStorage}'],
    ['cards/application/funcs.ts', 'export function value(){return Date.now()}'],
    ['cards/components/Card.svelte', '<script>import {value} from "../../folders/domain/funcs";</script>'],
    ['cards/domain/funcs.ts', 'export {createRuntime} from "../../workspace/adapters/funcs";'],
    ['cards/domain/funcs.ts', 'export type Value=string;'],
  ]) {
    assert.ok(checkSource(`frontend/src/features/${filename}`, source).length, filename);
  }
  assert.deepEqual(
    checkSource(
      'frontend/src/features/cards/domain/funcs.ts',
      'export function value(input: string){return input.trim()}',
    ),
    [],
  );
  assert.deepEqual(
    checkSource(
      'frontend/src/features/cards/adapters/funcs.ts',
      'import {Events} from "@wailsio/runtime"; export function subscribe(){return Events}',
    ),
    [],
  );
});
