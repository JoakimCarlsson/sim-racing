// ESLint configuration for sim-racing web client
// Uses eslint-config-prettier to disable style rules that Prettier handles.
'use strict';

/** @type {import('eslint').Linter.Config} */
module.exports = {
  root: true,
  env: {
    browser: true,
    es2022: true,
  },
  parser: '@typescript-eslint/parser',
  parserOptions: {
    ecmaVersion: 'latest',
    sourceType: 'module',
    project: './tsconfig.json',
  },
  plugins: ['@typescript-eslint'],
  extends: [
    'eslint:recommended',
    'plugin:@typescript-eslint/recommended',
    'prettier',
  ],
  rules: {
    // Allow console.log for dev tooling / debug output in this early stage.
    'no-console': 'off',
    // Permit explicit `any` narrowly where needed; tighten later.
    '@typescript-eslint/no-explicit-any': 'warn',
    // Existing source declares functions inside try/catch blocks; narrow
    // rather than refactoring source (see non_goals in the plan).
    'no-inner-declarations': 'off',
  },
};
