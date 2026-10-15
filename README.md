# Photo ID

This is the Go/Wails workspace for a standalone desktop rewrite of the Python/Tkinter Photo ID quiz. The app opens quiz JSON, resolves common names against the eBird taxonomy, and runs a photo-identification quiz. Data-processing helpers have not yet been migrated.

## Prerequisites

- Go
- Node.js and npm
- The Wails v2 CLI: `go install github.com/wailsapp/wails/v2/cmd/wails@latest`
- The platform-specific Wails build requirements: [https://wails.io/docs/gettingstarted/installation]

## Development

Run `wails dev` from the project root. Wails starts the Vite development server and provides live reload.

Choose **Open quiz** in the desktop app to load a quiz JSON file. Taxonomy is downloaded using `EBIRDAPIKEY` and cached in the user's cache directory. To use an existing taxonomy JSON file instead, set `EBIRD_TAXONOMY_PATH`. The parser fixture is `testdata/test_data.json`.

At startup, the app reads CSV life lists from the user's `photo-id/have_lists` cache directory. Species found there are marked with a blue dot in quiz details and beside the photo prompt.

## Icon Assets

`build/grackle-avatar.jpg` is the original GitHub Pages avatar. `build/appicon.png` is the square Wails icon source; Wails uses it to generate the Windows icon when `build/windows/icon.ico` is missing.

Photo lookup uses Playwright for Go to load eBird's media catalog in Chromium and complete the Anubis browser challenge. Catalog results and downloaded photo bytes are cached for eight months under the operating system's user cache directory (`photo-id/media` and `photo-id/photos`). Cached catalog results skip Chromium; cached photo bytes are reused when images are displayed. The first uncached search downloads the Playwright driver and Chromium headless shell, so it needs an internet connection. No Python interpreter is needed. eBird may change its catalog markup, which could require updating the result extraction logic.

## Build

Run `./build.ps1` in PowerShell to build the desktop application and run the Go unit tests. In VS Code, select **Terminal > Run Build Task** or press `Ctrl+Shift+B`, then choose **Build and test Photo ID**. The executable is written to `build/bin/photo-id.exe` on Windows. Build each release on its target platform with that platform's Wails prerequisites installed.

## GitHub Actions

The `Quality checks` workflow runs on pushes to every branch and pull requests targeting `main`. It builds the frontend, checks Go formatting, runs `go vet` and unit tests with coverage, and runs the gated eBird live test as an integration-test stub (it is skipped unless `PHOTO_ID_LIVE_MEDIA_TEST=1`).

SonarQube Cloud analysis is configured in `sonar-project.properties`. Create or bind this repository as a project in SonarQube Cloud, then add its token as the GitHub Actions secret `SONAR_TOKEN` to enable scans. The scan waits for the quality gate. Until the secret is configured, the SonarQube job succeeds with a notice that the scan was skipped. For a self-hosted SonarQube Server, set the `SONAR_HOST_URL` repository variable and update `sonar-project.properties` for that server.

For a local `act` experiment, put a token you already have in the ignored root file `sonar.local.env` as `SONAR_TOKEN=...`, then pass it with `act --secret-file sonar.local.env`. Do not paste the token into chat or commit that file. `act` needs Docker and cannot faithfully reproduce this workflow's `windows-latest` job; use GitHub Actions for the end-to-end test. GitHub does not reveal a secret's saved value, so generate a new SonarQube token if you no longer have it.

To block merges when checks fail, add a branch protection rule or ruleset for `main` in GitHub and require the `Build and test` and `SonarQube` checks. Configure the Sonar token before making the SonarQube check a required status.
