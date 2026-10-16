import { defineConfig, globalIgnores } from 'eslint/config';
import nextVitals from 'eslint-config-next/core-web-vitals';
import nextTs from 'eslint-config-next/typescript';
import tseslint from 'typescript-eslint';
import security from 'eslint-plugin-security';
import tailwind from 'eslint-plugin-better-tailwindcss';
import { featureBoundaries } from './tooling/eslint/feature-boundaries.mjs';

const eslintConfig = defineConfig([
  ...nextVitals,
  ...nextTs,
  {
    name: 'strict/type-checked',
    files: ['**/*.{ts,tsx,mts,cts}'],
    extends: [tseslint.configs.strictTypeChecked, tseslint.configs.stylisticTypeChecked],
    languageOptions: {
      parserOptions: { projectService: true, tsconfigRootDir: import.meta.dirname },
    },
    rules: {
      '@typescript-eslint/strict-boolean-expressions': 'error',
      '@typescript-eslint/no-floating-promises': ['error', { ignoreVoid: false }],
      '@typescript-eslint/no-misused-promises': 'error',
      '@typescript-eslint/switch-exhaustiveness-check': 'error',
      '@typescript-eslint/consistent-type-imports': ['error', { prefer: 'type-imports' }],
      '@typescript-eslint/no-unused-vars': ['error', { args: 'all', caughtErrors: 'all' }],
    },
  },
  {
    name: 'security/source',
    files: ['**/*.{js,jsx,mjs,cjs,ts,tsx,mts,cts}'],
    plugins: { security },
    linterOptions: {
      reportUnusedDisableDirectives: 'error',
      reportUnusedInlineConfigs: 'error',
    },
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
      'react/no-danger': 'error',
      'react/jsx-no-target-blank': ['error', { enforceDynamicLinks: 'always' }],
    },
  },
  {
    name: 'tailwind/v4-shadcn',
    files: ['src/**/*.{js,jsx,ts,tsx}'],
    plugins: { 'better-tailwindcss': tailwind },
    settings: {
      'better-tailwindcss': {
        cwd: import.meta.dirname,
        entryPoint: 'src/app/globals.css',
        tsconfig: 'tsconfig.json',
        // Recognize the project's CSS components as well as shadcn's cn/cva helpers.
        detectComponentClasses: true,
      },
    },
    rules: {
      ...tailwind.configs['correctness'].rules,
      'better-tailwindcss/no-duplicate-classes': 'error',
      'better-tailwindcss/no-deprecated-classes': 'error',
      'better-tailwindcss/enforce-consistent-class-order': 'error',
    },
  },
  {
    files: ['src/**/*.{ts,tsx}'],
    plugins: { architecture: { rules: { 'feature-boundaries': featureBoundaries } } },
    rules: { 'architecture/feature-boundaries': 'error' },
  },
  {
    files: ['src/shared/**/*.{ts,tsx}'],
    rules: {
      'no-restricted-imports': [
        'error',
        {
          patterns: [
            {
              group: ['@/features/**', '@/app/**', '**/features/**', '**/app/**'],
              message: 'Shared foundations must not depend on features or routes.',
            },
          ],
        },
      ],
    },
  },
  {
    files: ['src/features/**/*.{ts,tsx}'],
    rules: {
      'no-restricted-imports': [
        'error',
        {
          patterns: [
            {
              group: [
                '@/app/**',
                '**/app/**',
                '@/features/*/components/**',
                '@/features/*/hooks/**',
                '@/features/*/lib/**',
              ],
              message: 'Use feature public entry points and keep route dependencies in app.',
            },
          ],
        },
      ],
    },
  },
  // Override default ignores of eslint-config-next.
  globalIgnores([
    // Default ignores of eslint-config-next:
    '.next/**',
    'out/**',
    'build/**',
    'next-env.d.ts',
    'coverage/**',
    'dist/**',
  ]),
]);

export default eslintConfig;
