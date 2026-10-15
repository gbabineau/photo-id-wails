$ErrorActionPreference = 'Stop'

$preferredWailsPath = Join-Path $env:USERPROFILE 'go\bin\wails.exe'
if (Test-Path $preferredWailsPath) {
    $wailsPath = $preferredWailsPath
} else {
    $wailsCommand = Get-Command wails -ErrorAction SilentlyContinue
    if (-not $wailsCommand) {
        throw 'Wails CLI was not found. Install it with: go install github.com/wailsapp/wails/v2/cmd/wails@latest'
    }
    $wailsPath = $wailsCommand.Source
}

Push-Location $PSScriptRoot
try {
    Write-Host 'Building the Wails desktop application...'
    & $wailsPath build
    if ($LASTEXITCODE -ne 0) {
        exit $LASTEXITCODE
    }

    Write-Host 'Running Go unit tests...'
    go test ./...
    if ($LASTEXITCODE -ne 0) {
        exit $LASTEXITCODE
    }

    Write-Host 'Build and tests passed. Output: build/bin/photo-id.exe'
} finally {
    Pop-Location
}
