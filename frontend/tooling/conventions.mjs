/* eslint-disable security/detect-non-literal-fs-filename -- Paths are fixed repository assets or local module-resolution candidates. */
import ts from 'typescript';
import { parse } from 'svelte/compiler';
import { existsSync, statSync } from 'node:fs';
import path from 'node:path';

function declarationSpecifier(node) {
  if (!(ts.isImportDeclaration(node) || ts.isExportDeclaration(node))) return null;
  return node.moduleSpecifier && ts.isStringLiteral(node.moduleSpecifier) ? node.moduleSpecifier.text : null;
}
function callSpecifier(node, source) {
  if (!ts.isCallExpression(node)) return null;
  const dynamic = node.expression.kind === ts.SyntaxKind.ImportKeyword || node.expression.getText(source) === 'require';
  return dynamic && ts.isStringLiteral(node.arguments[0]) ? node.arguments[0].text : null;
}
function typeSpecifier(node) {
  if (!ts.isImportTypeNode(node)) return null;
  return ts.isLiteralTypeNode(node.argument) && ts.isStringLiteral(node.argument.literal)
    ? node.argument.literal.text
    : null;
}
function moduleSpecifiers(source) {
  const results = [];
  function visit(node) {
    results.push(
      ...[declarationSpecifier(node), callSpecifier(node, source), typeSpecifier(node)].filter(
        (value) => value !== null,
      ),
    );
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
function featureOf(file) {
  return /\/features\/([^/]+)\//.exec(file)?.[1];
}
function sourceContext(filename) {
  const file = path.resolve(filename).replaceAll('\\', '/');
  return {
    file,
    feature: featureOf(file),
    adapter: file.includes('/adapters/'),
    core: /\/(domain|application)\//.test(file),
    basename: path.basename(file),
  };
}
function filenameErrors(context) {
  const valid = /^(?:funcs|vars|consts|types|index)\.ts$|^(?:funcs|vars)\.svelte\.ts$|^[A-Z][A-Za-z0-9]*\.svelte$/;
  return context.feature && !valid.test(context.basename)
    ? ['Use a role-based TypeScript filename or PascalCase Svelte component']
    : [];
}
function scripts(file, text) {
  if (!file.endsWith('.svelte')) return [text];
  const ast = parse(text, { modern: true });
  return [ast.instance, ast.module]
    .filter(Boolean)
    .map((script) => text.slice(script.content.start, script.content.end));
}
function frameworkImport(specifier, resolved) {
  if (specifier === 'svelte' || specifier.startsWith('svelte/')) return true;
  return resolved ? implementationImport(resolved) : false;
}
function implementationImport(resolved) {
  return resolved.includes('/adapters/') || resolved.endsWith('.svelte') || resolved.endsWith('.svelte.ts');
}
function permittedCrossFeature(context, resolved, target) {
  const composition =
    context.file.endsWith('/src/main.ts') && resolved.endsWith(`/features/${target}/adapters/index.ts`);
  const domain = context.core && resolved.endsWith(`/features/${target}/domain/index.ts`);
  return composition || domain;
}
function crossFeatureImport(context, resolved) {
  if (!resolved) return false;
  const target = featureOf(resolved);
  if (!target || target === context.feature || resolved.endsWith(`/features/${target}/index.ts`)) return false;
  return !permittedCrossFeature(context, resolved, target);
}
function importErrors(context, specifier) {
  const resolved = resolveImport(context.file, specifier)?.replaceAll('\\', '/');
  return [
    ...transportErrors(context, specifier, resolved),
    ...(context.core && frameworkImport(specifier, resolved)
      ? [`Core imports framework or adapter code: ${specifier}`]
      : []),
    ...(crossFeatureImport(context, resolved) ? [`Import another feature through its index.ts: ${specifier}`] : []),
  ];
}
function transportErrors(context, specifier, resolved) {
  return !context.adapter && (specifier.startsWith('@wailsio/') || resolved?.includes('/bindings/'))
    ? [`Transport import belongs in an adapter: ${specifier}`]
    : [];
}
function browserErrors(node) {
  const browser =
    /^(window|document|navigator|localStorage|crypto|Storage|ParentNode|HTMLElement|HTMLMediaElement|setTimeout|setInterval|\$state|\$derived|\$effect)$/;
  return ts.isIdentifier(node) && browser.test(node.text)
    ? [`Core depends on browser or Svelte API: ${node.text}`]
    : [];
}
function clockErrors(node, source) {
  return ts.isPropertyAccessExpression(node) && node.expression.getText(source) === 'Date' && node.name.text === 'now'
    ? ['Inject the clock into core workflows']
    : [];
}
function coreErrors(source) {
  const errors = [];
  function visit(node) {
    errors.push(...browserErrors(node), ...clockErrors(node, source));
    ts.forEachChild(node, visit);
  }
  visit(source);
  return errors;
}
function declarationErrors(node) {
  if (ts.isInterfaceDeclaration(node) || ts.isTypeAliasDeclaration(node))
    return ['Type declarations belong in types.ts'];
  return ts.isVariableStatement(node) ? ['Module constants/state belong in consts.ts or vars.ts'] : [];
}
function checkScript(context, script) {
  const source = ts.createSourceFile(context.file, script, ts.ScriptTarget.Latest, true, ts.ScriptKind.TS);
  return [
    ...moduleSpecifiers(source).flatMap((specifier) => importErrors(context, specifier)),
    ...(context.core ? coreErrors(source) : []),
    ...(context.feature && context.basename === 'funcs.ts' ? source.statements.flatMap(declarationErrors) : []),
  ];
}
export function checkSource(filename, text) {
  const context = sourceContext(filename);
  return [...filenameErrors(context), ...scripts(context.file, text).flatMap((script) => checkScript(context, script))];
}
