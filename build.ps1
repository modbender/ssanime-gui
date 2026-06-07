# build.ps1 — ssanime-gui build script for Windows PowerShell
#
# Usage:
#   .\build.ps1                        # Windows GUI binary (default)
#   .\build.ps1 -Target windows-gui   # same as default
#   .\build.ps1 -Target windows-console # Windows binary with console (debug)
#   .\build.ps1 -Target linux          # Linux amd64 binary
#   .\build.ps1 -Target darwin         # macOS amd64 binary
#   .\build.ps1 -Target all            # all three platforms

param(
    [ValidateSet("windows-gui", "windows-console", "linux", "darwin", "all")]
    [string]$Target = "windows-gui"
)

$CMD    = ".\cmd\ssanime"
$LDWIN  = "-H=windowsgui -s -w"
$LDUNIX = "-s -w"

function Build-WindowsGUI {
    Write-Host "Building Windows GUI binary (no console)..."
    $env:GOOS = "windows"; $env:GOARCH = "amd64"
    go build -ldflags $LDWIN -o ssanime.exe $CMD
    if ($LASTEXITCODE -eq 0) {
        $size = [math]::Round((Get-Item ssanime.exe).Length / 1MB, 1)
        Write-Host "OK  ssanime.exe  ($size MB)"
    }
    Remove-Item Env:\GOOS, Env:\GOARCH -ErrorAction SilentlyContinue
}

function Build-WindowsConsole {
    Write-Host "Building Windows console binary..."
    go build -ldflags "-s -w" -o ssanime.exe $CMD
    if ($LASTEXITCODE -eq 0) {
        $size = [math]::Round((Get-Item ssanime.exe).Length / 1MB, 1)
        Write-Host "OK  ssanime.exe  ($size MB)"
    }
}

function Build-Linux {
    Write-Host "Building Linux amd64 binary..."
    $env:GOOS = "linux"; $env:GOARCH = "amd64"; $env:CGO_ENABLED = "0"
    go build -ldflags $LDUNIX -o ssanime-linux-amd64 $CMD
    if ($LASTEXITCODE -eq 0) {
        $size = [math]::Round((Get-Item ssanime-linux-amd64).Length / 1MB, 1)
        Write-Host "OK  ssanime-linux-amd64  ($size MB)"
    }
    Remove-Item Env:\GOOS, Env:\GOARCH, Env:\CGO_ENABLED -ErrorAction SilentlyContinue
}

function Build-Darwin {
    Write-Host "Building macOS amd64 binary..."
    $env:GOOS = "darwin"; $env:GOARCH = "amd64"; $env:CGO_ENABLED = "0"
    go build -ldflags $LDUNIX -o ssanime-darwin-amd64 $CMD
    if ($LASTEXITCODE -eq 0) {
        $size = [math]::Round((Get-Item ssanime-darwin-amd64).Length / 1MB, 1)
        Write-Host "OK  ssanime-darwin-amd64  ($size MB)"
    }
    Remove-Item Env:\GOOS, Env:\GOARCH, Env:\CGO_ENABLED -ErrorAction SilentlyContinue
}

switch ($Target) {
    "windows-gui"     { Build-WindowsGUI }
    "windows-console" { Build-WindowsConsole }
    "linux"           { Build-Linux }
    "darwin"          { Build-Darwin }
    "all"             { Build-WindowsGUI; Build-Linux; Build-Darwin }
}
