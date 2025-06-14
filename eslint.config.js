import { createConfigForNuxt } from '@nuxt/eslint-config/flat';

export default createConfigForNuxt({
  features: {
    typescript: true,
    vue: true,
  },
}).append({
  rules: {
    // Custom rules for your project
    'no-console': 'warn',
    'prefer-const': 'error',
    'no-unused-vars': 'warn',
    '@typescript-eslint/no-unused-vars': 'warn',
    'vue/multi-word-component-names': 'off',
    'vue/no-multiple-template-root': 'off',
  },
});
