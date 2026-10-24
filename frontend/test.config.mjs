/* eslint-disable security/detect-non-literal-fs-filename -- Paths are fixed repository assets or local module-resolution candidates. */
import { registerHooks } from 'node:module';
import { existsSync, readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { compile } from 'svelte/compiler';

// Keep Node's test runner while supporting the same TS/Svelte imports as Vite.
registerHooks({
  resolve(specifier, context, next) {
    if (specifier.startsWith('.') && context.parentURL) {
      const url = new URL(specifier, context.parentURL);
      for (const suffix of ['', '.ts', '/index.ts', '.svelte']) {
        const candidate = new URL(url.href + suffix);
        if (existsSync(fileURLToPath(candidate)) && !candidate.pathname.endsWith('/')) {
          try {
            return next(candidate.href, context);
          } catch (error) {
            if (suffix) throw error;
          }
        }
      }
    }
    return next(specifier, context);
  },
  load(url, context, next) {
    // Vite handles styles in the app; Node tests exercise the component modules.
    if (url.endsWith('.css')) return { format: 'module', shortCircuit: true, source: 'export {};' };
    if (url.endsWith('.svelte')) {
      return {
        format: 'module',
        shortCircuit: true,
        source: compile(readFileSync(new URL(url), 'utf8'), { filename: fileURLToPath(url), generate: 'client' }).js
          .code,
      };
    }
    return next(url, context);
  },
});
