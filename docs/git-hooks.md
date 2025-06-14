# Git Hooks Setup

This project uses [Husky](https://typicode.github.io/husky/) for Git hooks to ensure code quality and consistency.

## What's Configured

### Pre-commit Hook

- Runs `lint-staged` on staged files
- Lints and formats only changed files for better performance
- Configured in `.husky/pre-commit`

### Pre-push Hook

- Runs full test suite including:
  - ESLint check on all files
  - Prettier format check on all files
  - TypeScript type checking
- Configured in `.husky/pre-push`

### Commit Message Hook

- Enforces conventional commit format
- Examples: `feat: add new feature`, `fix(ui): resolve button styling`
- Configured in `.husky/commit-msg`

## Lint-staged Configuration

Located in `package.json`:

```json
{
  "lint-staged": {
    "*.{js,ts,vue}": ["eslint --fix", "prettier --write"],
    "*.{json,md,yml,yaml}": ["prettier --write"],
    "*.{css,scss}": ["prettier --write"]
  }
}
```

## Available Scripts

- `pnpm lint` - Run ESLint on all files
- `pnpm lint:fix` - Fix ESLint errors automatically
- `pnpm format` - Format all files with Prettier
- `pnpm format:check` - Check if files are formatted
- `pnpm type-check` - Run TypeScript type checking
- `pnpm test` - Run all checks (lint + format + type-check)

## How It Works

1. **On commit**: Only staged files are linted and formatted
2. **On push**: Full test suite runs to catch any issues
3. **Commit messages**: Must follow conventional commit format

## Bypassing Hooks (Use Sparingly)

```bash
# Skip pre-commit hook
git commit --no-verify -m "message"

# Skip pre-push hook
git push --no-verify
```

## Setup for New Contributors

Hooks are automatically installed when running `pnpm install` thanks to the `prepare` script in `package.json`.

### Manual Setup (if needed)

If hooks aren't working, you can manually initialize Husky:

```bash
# Install dependencies (includes Husky)
pnpm install

# Initialize Husky (creates .husky directory structure)
pnpm dlx husky init

# Run prepare script to ensure hooks are installed
pnpm run prepare
```

The `prepare` script in `package.json` contains:

```json
{
  "scripts": {
    "prepare": "husky"
  }
}
```

This ensures Husky is properly initialized after each `pnpm install`.

## Modern Husky Format

This project uses Husky v9+ which has a cleaner format without the deprecated shebang lines (`#!/usr/bin/env sh`) and husky.sh sourcing. The hook files contain only the essential commands for better maintainability and future compatibility.
