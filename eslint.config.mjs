import js from '@eslint/js';
import tseslint from 'typescript-eslint';
import svelte from 'eslint-plugin-svelte';
import security from 'eslint-plugin-security';
import globals from 'globals';

export default tseslint.config(
  {
    ignores: [
      'taskotter/**',
      'frontend/bindings/**',
      'frontend/dist/**',
      '**/node_modules/**',
      'build/**',
      '.cache/**',
    ],
  },
  js.configs.recommended,
  {
    files: ['**/*.{js,mjs,cjs,ts,svelte}'],
    languageOptions: { globals: { ...globals.browser, ...globals.node } },
    plugins: { security },
    linterOptions: { reportUnusedDisableDirectives: 'error', reportUnusedInlineConfigs: 'error' },
    rules: {
      ...Object.fromEntries(Object.keys(security.configs.recommended.rules).map((rule) => [rule, 'error'])),
      'no-eval': 'error',
      'no-implied-eval': 'error',
      'no-new-func': 'error',
      'no-script-url': 'error',
      'no-proto': 'error',
      'no-extend-native': 'error',
      'no-debugger': 'error',
      'no-console': ['error', { allow: ['warn', 'error'] }],
      eqeqeq: ['error', 'always'],
    },
  },
  {
    files: ['frontend/src/**/*.ts'],
    extends: [...tseslint.configs.strictTypeChecked, ...tseslint.configs.stylisticTypeChecked],
    languageOptions: { parserOptions: { projectService: true, tsconfigRootDir: import.meta.dirname } },
    rules: {
      '@typescript-eslint/strict-boolean-expressions': 'error',
      '@typescript-eslint/no-floating-promises': ['error', { ignoreVoid: false }],
      '@typescript-eslint/no-misused-promises': 'error',
      '@typescript-eslint/switch-exhaustiveness-check': 'error',
      '@typescript-eslint/consistent-type-imports': ['error', { prefer: 'type-imports' }],
      '@typescript-eslint/no-unused-vars': ['error', { args: 'all', caughtErrors: 'all' }],
    },
  },
  ...svelte.configs['flat/recommended'],
  {
    files: ['frontend/src/**/*.svelte'],
    languageOptions: { parserOptions: { parser: tseslint.parser, extraFileExtensions: ['.svelte'] } },
    rules: {
      'no-unused-vars': 'off',
      '@typescript-eslint/no-unused-vars': ['error', { args: 'all', caughtErrors: 'all' }],
    },
    plugins: { '@typescript-eslint': tseslint.plugin },
  },
);
