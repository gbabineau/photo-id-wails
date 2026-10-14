# Photo ID

This is the Go/Wails workspace for a standalone desktop rewrite of the Python/Tkinter Photo ID quiz. The app opens quiz JSON, resolves common names against the eBird taxonomy, and runs a photo-identification quiz. Data-processing helpers have not yet been migrated.

## Prerequisites

- Go
- Node.js and npm
- The Wails v2 CLI: `go install github.com/wailsapp/wails/v2/cmd/wails@latest`
- The platform-specific Wails build requirements: https://wails.io/docs/gettingstarted/installation

## Development

Run `wails dev` from the project root. Wails starts the Vite development server and provides live reload.

Choose **Open quiz** in the desktop app to load a quiz JSON file. Taxonomy is downloaded using `EBIRDAPIKEY` and cached in the user's cache directory. To use an existing taxonomy JSON file instead, set `EBIRD_TAXONOMY_PATH`. The parser fixture is `testdata/test_data.json`.

Photo lookup uses Playwright for Go to load eBird's media catalog in Chromium and complete the Anubis browser challenge. Catalog results and downloaded photo bytes are cached for eight months under the operating system's user cache directory (`photo-id/media` and `photo-id/photos`). Cached catalog results skip Chromium; cached photo bytes are reused when images are displayed. The first uncached search downloads the Playwright driver and Chromium headless shell, so it needs an internet connection. No Python interpreter is needed. eBird may change its catalog markup, which could require updating the result extraction logic.

## Build

Run `wails build` from the project root to build the desktop application for the current platform. Build each release on its target platform with that platform's Wails prerequisites installed.
