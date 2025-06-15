@echo off
setlocal enabledelayedexpansion

REM Retry Release Script for Windows
REM Usage: scripts\retry-release.bat <version> [force]
REM Example: scripts\retry-release.bat v1.2.3
REM Example: scripts\retry-release.bat v1.2.3 force

set VERSION=%1
set FORCE=%2

if "%VERSION%"=="" (
    echo [ERROR] Version is required
    echo Usage: %0 ^<version^> [force]
    echo Example: %0 v1.2.3
    exit /b 1
)

echo [INFO] Retrying release for version: %VERSION%

REM Check if tag exists
git rev-parse %VERSION% >nul 2>&1
if %errorlevel% equ 0 (
    echo [WARN] Tag %VERSION% already exists
    
    if "%FORCE%"=="force" (
        echo [INFO] Force flag detected, deleting existing tag...
        git tag -d %VERSION% 2>nul
        git push origin --delete %VERSION% 2>nul
        echo [SUCCESS] Existing tag deleted
    ) else (
        echo [ERROR] Tag already exists. Use 'force' argument to recreate it.
        echo Command: %0 %VERSION% force
        exit /b 1
    )
)

REM Get current commit
for /f "tokens=*" %%i in ('git rev-parse HEAD') do set CURRENT_COMMIT=%%i
echo [INFO] Current commit: %CURRENT_COMMIT%

REM Create new tag on current commit
echo [INFO] Creating tag %VERSION% on latest commit...
git tag -a %VERSION% -m "Release %VERSION%"

REM Push the tag
echo [INFO] Pushing tag to remote...
git push origin %VERSION%

if %errorlevel% equ 0 (
    echo [SUCCESS] Tag %VERSION% created and pushed successfully!
    echo [INFO] GitHub Actions will now automatically start the release build.
    for /f "tokens=*" %%i in ('git config --get remote.origin.url') do set REPO_URL=%%i
    echo [INFO] Monitor progress in your GitHub repository's Actions tab
) else (
    echo [ERROR] Failed to push tag
    exit /b 1
)

endlocal
