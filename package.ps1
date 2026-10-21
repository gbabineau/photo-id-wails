#delete previous versions of the installer
$installerPath = "build\bin\photo-id-amd64-installer.exe"
Remove-Item -Path $installerPath -Force -ErrorAction SilentlyContinue
# Install NSIS (winget install NSIS.NSIS) and make sure `makensis` is on the PATH.
run wails build -clean -nsis
# This produces build/bin/photo-id-amd64-installer.exe. It installs the app, adds Start Menu and desktop shortcuts, and registers an uninstaller. It also installs the WebView2 runtime if the machine lacks it, which is rare on current Windows.
#Because it runs asynchronously, though we want to wait.

Write-Host "Waiting for makensis to finish..." -NoNewline
while (-not (Test-Path $installerPath)) {
    Write-Host "." -NoNewline
    Start-Sleep -Seconds 1
}
Write-Host "`nFinished! Installer is ready."