import { createConfigForNuxt } from '@nuxt/eslint-config/flat';

export default createConfigForNuxt({
  features: {
    typescript: true,
    vue: true,
  },
})
  .append({
    rules: {
      // Custom rules for your project - more lenient for development
      'no-console': 'off', // Allow console statements during development
      'prefer-const': 'error',
      'no-unused-vars': 'off', // Disable for development
      '@typescript-eslint/no-unused-vars': 'off', // Disable for development
      'vue/multi-word-component-names': 'off',
      'vue/no-multiple-template-root': 'off',
      'vue/html-self-closing': 'off', // Disable to avoid conflict with Prettier
      '@typescript-eslint/no-explicit-any': 'off', // Allow any during development
      '@typescript-eslint/no-dynamic-delete': 'off', // Allow dynamic delete
      '@typescript-eslint/no-require-imports': 'off', // Allow require imports
      'vue/require-default-prop': 'off',
    },
  })
  .append({
    // Ignore JSON files completely
    ignores: ['**/*.json'],
  });
