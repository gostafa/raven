/* eslint-disable security/detect-non-literal-fs-filename -- Paths are fixed repository assets or local module-resolution candidates. */
import ts from 'typescript';
import { parse } from 'svelte/compiler';
import { existsSync, statSync } from 'node:fs';
import path from 'node:path';

function moduleSpecifiers(source) {
  const results = [];
  function visit(node) {
    if (
      (ts.isImportDeclaration(node) || ts.isExportDeclaration(node)) &&
      node.moduleSpecifier &&
      ts.isStringLiteral(node.moduleSpecifier)
    )
      results.push(node.moduleSpecifier.text);
    if (
      ts.isCallExpression(node) &&
      (node.expression.kind === ts.SyntaxKind.ImportKeyword || node.expression.getText(source) === 'require') &&
      ts.isStringLiteral(node.arguments[0])
    )
      results.push(node.arguments[0].text);
    if (ts.isImportTypeNode(node) && ts.isLiteralTypeNode(node.argument) && ts.isStringLiteral(node.argument.literal))
      results.push(node.argument.literal.text);
    ts.forEachChild(node, visit);
  }
  visit(source);
  return results;
}

function resolveImport(file, specifier) {
  const candidate = specifier.startsWith('@/')
    ? path.resolve('frontend/src', specifier.slice(2))
    : specifier.startsWith('.')
      ? path.resolve(path.dirname(file), specifier)
      : null;
  if (!candidate) return null;
  return (
    ['', '.ts', '.svelte.ts', '.svelte', '/index.ts']
      .map((suffix) => candidate + suffix)
      .find((value) => existsSync(value) && statSync(value).isFile()) ?? candidate
  );
}

export function checkSource(filename, text) {
  const file = path.resolve(filename).replaceAll('\\', '/');
  const feature = /\/features\/([^/]+)\//.exec(file)?.[1];
  const adapter = file.includes('/adapters/');
  const core = /\/(domain|application)\//.test(file);
  const errors = [];
  const basename = path.basename(file);
  if (
    feature &&
    !/^(?:funcs|vars|consts|types|index)\.ts$|^(?:funcs|vars)\.svelte\.ts$|^[A-Z][A-Za-z0-9]*\.svelte$/.test(basename)
  )
    errors.push('Use a role-based TypeScript filename or PascalCase Svelte component');
  let scripts = [text];
  if (file.endsWith('.svelte')) {
    const ast = parse(text, { modern: true });
    scripts = [ast.instance, ast.module]
      .filter(Boolean)
      .map((script) => text.slice(script.content.start, script.content.end));
  }
  for (const script of scripts) {
    const source = ts.createSourceFile(file, script, ts.ScriptTarget.Latest, true, ts.ScriptKind.TS);
    for (const specifier of moduleSpecifiers(source)) {
      const resolved = resolveImport(file, specifier)?.replaceAll('\\', '/');
      if (!adapter && (specifier.startsWith('@wailsio/') || resolved?.includes('/bindings/')))
        errors.push(`Transport import belongs in an adapter: ${specifier}`);
      if (
        core &&
        (specifier === 'svelte' ||
          specifier.startsWith('svelte/') ||
          resolved?.includes('/adapters/') ||
          resolved?.endsWith('.svelte') ||
          resolved?.endsWith('.svelte.ts'))
      )
        errors.push(`Core imports framework or adapter code: ${specifier}`);
      const targetFeature = resolved && /\/features\/([^/]+)\//.exec(resolved)?.[1];
      if (targetFeature && targetFeature !== feature && !resolved.endsWith(`/features/${targetFeature}/index.ts`)) {
        const composition =
          file.endsWith('/src/main.ts') && resolved.endsWith(`/features/${targetFeature}/adapters/index.ts`);
        const domainApi = core && resolved.endsWith(`/features/${targetFeature}/domain/index.ts`);
        if (!composition && !domainApi) errors.push(`Import another feature through its index.ts: ${specifier}`);
      }
    }
    if (core) {
      function visit(node) {
        if (
          ts.isIdentifier(node) &&
          /^(window|document|navigator|localStorage|crypto|Storage|ParentNode|HTMLElement|HTMLMediaElement|setTimeout|setInterval|\$state|\$derived|\$effect)$/.test(
            node.text,
          )
        )
          errors.push(`Core depends on browser or Svelte API: ${node.text}`);
        if (
          ts.isPropertyAccessExpression(node) &&
          node.expression.getText(source) === 'Date' &&
          node.name.text === 'now'
        )
          errors.push('Inject the clock into core workflows');
        ts.forEachChild(node, visit);
      }
      visit(source);
    }
    if (feature && basename === 'funcs.ts') {
      for (const statement of source.statements) {
        if (ts.isInterfaceDeclaration(statement) || ts.isTypeAliasDeclaration(statement))
          errors.push('Type declarations belong in types.ts');
        if (ts.isVariableStatement(statement)) errors.push('Module constants/state belong in consts.ts or vars.ts');
      }
    }
  }
  return errors;
}
