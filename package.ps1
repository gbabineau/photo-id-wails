# Install NSIS (winget install NSIS.NSIS) and make sure `makensis` is on the PATH.
run wails build -clean -nsis
# This produces build/bin/photo-id-amd64-installer.exe. It installs the app, adds Start Menu and desktop shortcuts, and registers an uninstaller. It also installs the WebView2 runtime if the machine lacks it, which is rare on current Windows.