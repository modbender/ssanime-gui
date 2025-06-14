@echo off
setlocal enabledelayedexpansion

echo 🚀 SSAnime GUI Release Script
echo.

:: Check if we're in a git repository
git rev-parse --git-dir >nul 2>&1
if errorlevel 1 (
    echo ❌ Not in a git repository
    exit /b 1
)

:: Check if working directory is clean
for /f %%i in ('git status --porcelain') do (
    echo ❌ Working directory is not clean. Please commit or stash changes first.
    git status --short
    exit /b 1
)

:: Get current version
for /f "tokens=*" %%i in ('node -p "require('./package.json').version"') do set CURRENT_VERSION=%%i
echo 📦 Current version: %CURRENT_VERSION%

:: Ask for version type
echo.
echo Select version bump type:
echo 1) Patch (bug fixes) - e.g., 1.0.0 → 1.0.1
echo 2) Minor (new features) - e.g., 1.0.0 → 1.1.0
echo 3) Major (breaking changes) - e.g., 1.0.0 → 2.0.0
echo 4) Custom version
echo 5) Cancel

set /p choice="Enter your choice (1-5): "

if "%choice%"=="1" (
    set VERSION_TYPE=patch
) else if "%choice%"=="2" (
    set VERSION_TYPE=minor
) else if "%choice%"=="3" (
    set VERSION_TYPE=major
) else if "%choice%"=="4" (
    set /p CUSTOM_VERSION="Enter custom version (e.g., 1.2.3): "
    set VERSION_TYPE=!CUSTOM_VERSION!
) else if "%choice%"=="5" (
    echo Release cancelled
    exit /b 0
) else (
    echo ❌ Invalid choice
    exit /b 1
)

:: Confirm the action
echo.
echo ⚠️  This will:
echo    • Run tests
echo    • Bump version in package.json
echo    • Generate/update CHANGELOG.md
echo    • Create a git commit
echo    • Create a git tag
echo    • Push to remote repository
echo.
set /p confirm="Continue? (y/N): "

if /i not "%confirm%"=="y" (
    echo Release cancelled
    exit /b 0
)

:: Run tests first
echo.
echo 🧪 Running tests...
call pnpm test
if errorlevel 1 (
    echo ❌ Tests failed. Release cancelled.
    exit /b 1
)

:: Run commit-and-tag-version
echo.
echo 📝 Generating changelog and bumping version...
call pnpm commit-and-tag-version --release-as %VERSION_TYPE%
if errorlevel 1 (
    echo ❌ Version bump failed
    exit /b 1
)

:: Get new version
for /f "tokens=*" %%i in ('node -p "require('./package.json').version"') do set NEW_VERSION=%%i

echo.
echo ✅ Version bumped: %CURRENT_VERSION% → %NEW_VERSION%

:: Push to remote
echo.
echo 🚀 Pushing to remote repository...
git push --follow-tags origin main

if errorlevel 0 (
    echo.
    echo 🎉 Release completed successfully!
    echo 🏷️  Tag: v%NEW_VERSION%
    echo 🚀 Pushed to: origin/main
    echo.
    echo ℹ️  The GitHub Actions workflow will automatically build and attach executables to the release.
) else (
    echo ❌ Failed to push to remote repository
    exit /b 1
)

pause
