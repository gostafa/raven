# Raven

A Wails 3 + Svelte desktop app for remembering what you learn, on macOS, Windows, and Linux. Save information as memory cards, organize cards in folders, and revisit them when they are due. The interface uses the original black-and-white content palette, dark sidebar, and a daily review dashboard. An independent 48-pixel floating logo shows a badge when content items are ready to review.

## Run

Install Go **1.27.2**, Bun **1.4.2**, and the platform prerequisites below. Then, from this folder:

```sh
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.29
bun run start
```

Ensure your Go bin directory is on `PATH`. The launcher uses the project-local Wails installation in `.tools/` when present; it is already prepared on this Mac and excluded from version control. Development installs frontend dependencies with Bun and generates the Go bindings automatically.

## Use

- The home view shows your folders; select a folder to see its cards. The sidebar can be collapsed.
- Choose **New card** and fill **What you want to remember** with any number of text blocks, images, audio files, and videos. Use the arrows to order mixed content and **Remove** to discard an item. Each card is one information collection and needs at least one nonempty item; a single text block or media file is enough. Select or create a folder. Each block starts with an automatic Again rating using FSRS v4.0.0; its first review follows the returned due time.
- Choose **Edit card** below a library card to change its content, add files, or reorder and remove items. Editing preserves the review schedule. Empty text blocks are skipped. File selection uses a native multi-file picker; files are copied when saving, so canceling an editor does not import anything.
- Select a library card to review in Raven, or click the floating logo to review due items in a separate, always-on-top card beside it. The floating card leaves the main window as it is. Read or play one content block at a time and rate it **Again**, **Hard**, **Good**, or **Easy**. Each block has its own FSRS schedule using the library defaults. Library cards can also be reviewed early.
- The backend wakes at the next FSRS due time and refreshes review counts. The floating logo displays the number of due items (up to **9+**). Its review card shows **All caught up** when nothing is due. Close the card with its close button, **Escape**, or another logo click; switching apps leaves it visible. Dismissal waits until a rating save finishes. Reopening loads the remaining due items, and leaving an item or closing the card pauses playback. Desktop alerts group newly due items and still open reviews in the main window, with authorization requested once. Alerts require OS permission; macOS requires a bundled, signed app. Unavailable notifications leave reviews and the widget working. Reminders run while Raven is running, reconcile on wake/restart, and do not repeat already notified due occurrences.
- Search all card text, media filenames, or folder names, and folder titles, descriptions, or tags. Press **Cmd+K** on macOS or **Ctrl+K** elsewhere to focus search. Press **Escape** in search to clear it.
- Choose **New folder** to add a name, optional description, and comma-separated tags. Empty and duplicate tags are ignored.
- Folders, cards, and review schedules are saved in a local bbolt database and restored when Raven reopens. Selection and sidebar visibility are saved separately in localStorage.
- Legacy folders saved in localStorage are not imported and are left untouched. Existing bbolt folders are preserved.
- Closing the main window hides it; the logo widget stays running.
- Click the floating logo to toggle its review card. Drag anywhere on the widget, including the logo, to move it; an open review card follows and stays within the screen’s work area.
- Right-click the widget for **Open Raven**, **Reset widget position**, and **Quit Raven**. These actions are also in the system tray/menu bar.
- **Quit Raven** (or macOS Cmd+Q) exits the process and all windows. Stopping the development server also exits the app.

The logo and review card are independent windows in the same process, not separately launched background processes. They remain available when the main window closes, until you explicitly quit Raven.

### Start at Login

On its first production launch, Raven enables **Start at login** for the current user on macOS, Windows, and Linux. Launch a production build once from its permanent installation location (for example, `/Applications/raven.app` on macOS). macOS may require approval in **System Settings → General → Login Items**. Development/debug builds never register startup or record the first-launch attempt; their startup menu items are disabled.

Login launches show only the floating widget and keep reminders running. Opening Raven normally still opens the main window. **Start at login** is a checkbox in both the widget's right-click menu and the system tray/menu bar menu. Turning it off takes effect at the next login; quitting Raven only ends the current session. Raven attempts default registration once and does not restore it on later launches if you disable it or remove its OS registration. If registration fails, Raven keeps working and you can retry using the menu checkbox.

Registration uses Wails' native per-user startup support: SMAppService for bundled macOS apps, a LaunchAgent for unbundled macOS builds, the Windows current-user Run registry key, and an XDG autostart desktop entry on Linux. Windows/Linux and unbundled macOS startup entries include `--autostart`; bundled macOS launches are detected through the native login Apple event. Install portable binaries at a stable path; disable startup before moving a binary, then re-enable it from the new location. A normal second launch opens the running Raven instance instead of opening another database connection.

For native startup verification on each OS, launch the installed production app, confirm both menu checkboxes agree, and log out and back in. Expect one widget, working reminders, and a hidden main window; opening Raven normally should reveal that same instance. Disable startup and repeat the login check, then explicitly re-enable it. On macOS, test the packaged app through a real login as well as the `--autostart` argument: the argument alone does not exercise SMAppService's native Apple event. OS startup controls and permissions can override a registration.

### Local Storage

The database is stored at `os.UserConfigDir()/raven/raven.db`: `~/Library/Application Support/raven/raven.db` on macOS, `%AppData%\raven\raven.db` on Windows, and `$XDG_CONFIG_HOME/raven/raven.db` (or `~/.config/raven/raven.db`) on Linux. New database directories use permissions `0700` and files use `0600` on Unix. Raven owns one database connection until explicit quit; hiding the window keeps it open. Single-instance handling runs before storage opens: normal second launches open the running Raven window, while automatic second launches leave its visibility unchanged. If another process holds the database lock, opening it still times out after one second. Startup errors include the database path, and an unreadable database is never silently replaced. A separate `settings` bucket records the one-time login startup initialization attempt without changing the schema version.

Each `folders/<folderID>` bucket contains JSON `metadata` (`id`, `title`, `description`, `tags`), a `cards` bucket for collection metadata, and `card_items/<cardID>/<itemID>` for separate content records. Each item records its owning folder/card, position in the single content collection, and kind. Text items store text; media items store the original filename, MIME type, byte size, and managed relative path. The frontend receives hydrated `items` arrays, but these arrays are never embedded in stored card metadata. Each item stores its complete FSRS card state and review logs, including an automatic-initialization flag. FSRS times use RFC3339 timestamps; UI summary times use Unix milliseconds. Card IDs are sequential within each folder; the UI identifies cards by folder ID plus card ID. Item IDs are random, and items belong to exactly one card. Content edits and review updates are transactional. The UI shows loading/retry states and retains form entries if a save fails.

Schema version 4 is recorded in `meta/schemaVersion`. On opening an older database, Raven first saves a private `raven.db.v1-*.bak`, `raven.db.v2-*.bak`, or `raven.db.v3-*.bak` snapshot. Original prompt and answer strings become separate text items in one collection. Version 2 cards merge their prompt items followed by their answer items, retaining each side's previous order, all item IDs, and all media paths. The migration preserves card IDs and sequences, removes the old side field, and initializes every item with FSRS Again at migration time. The old stage-based schedule is replaced; original records remain in the backup. A failed migration leaves the original records unchanged. Successfully migrated databases are not migrated again.

Media is stored beside the database as `media/<itemID>.<extension>`, with independent copies for each attachment. New media directories use Unix permissions `0700`, and imported files use `0600`. Imports are streamed into temporary files, synced, and renamed before the database commit. Failed saves remove new copies; successful edits remove discarded copies. Startup removes abandoned import files and unreferenced managed media after validating database records. Moving or deleting original source files does not affect saved cards. **Back up both `raven.db` and the entire `media` directory while Raven is quit.**

Images support JPEG, PNG, GIF, WebP, and AVIF; audio supports MP3, WAV, M4A, Ogg, and FLAC; video supports MP4/M4V, WebM, Ogg, and MOV. Actual playback depends on the platform WebView's codecs. Files are checked against their declared media type; Raven does not record or transcode media. Saved media is streamed through a card/item-scoped endpoint with byte-range seeking. Reviews show one content item at a time, playback stops when leaving an item, and unavailable or unplayable files show an item-level message.

## Build

```sh
bun run build
bun run package
```

Native output is in `bin/`. macOS packaging creates `bin/raven.app`; Windows and Linux have their own packaging tasks in `build/`. Build on each target OS for the simplest setup. A GitHub Actions workflow builds on macOS, Windows, and Ubuntu when this folder is pushed to GitHub.

macOS builds enable Wails' `private_mac_apis` tag for transparent WKWebView backgrounds. This lets the floating widget appear as a circle with its notification badge outside the edge; without the tag, WebKit paints an opaque square behind it. The tag applies to development and packaged builds.

Windows can also be cross-compiled from macOS or Linux without Docker:

```sh
bun run build -- GOOS=windows GOARCH=amd64
```

Platform prerequisites:

| Platform | Requirements                                                   |
| -------- | -------------------------------------------------------------- |
| macOS    | macOS 13+, Xcode Command Line Tools (`xcode-select --install`) |
| Windows  | Windows 10/11 with Microsoft Edge WebView2 Runtime             |
| Linux    | GTK4 and WebKitGTK 6.0; Ubuntu 24.04 is the baseline           |

Ubuntu build dependencies:

```sh
sudo apt-get install build-essential pkg-config libgtk-4-dev libwebkitgtk-6.0-dev
```

The widget uses native clipping on macOS and Windows, and transparent GTK styling on Linux. On macOS it joins all Spaces and can appear above fullscreen apps. Linux always-on-top and transparent rendering depend on the compositor; Wails cannot enforce topmost stacking on Wayland. See the [Wails platform prerequisites](https://v3.wails.io/getting-started/installation/).

## Versions

Verified against live registries on **15 October 2026**. Direct dependencies are pinned; Bun lockfiles and `go.sum` are included.

| Tool / package       | Version       |
| -------------------- | ------------- |
| Go                   | 1.27.2        |
| Bun                  | 1.4.2         |
| Wails / JS runtime   | 3.0.0-beta.29 |
| bbolt                | 1.5.0         |
| go-fsrs              | 4.0.0         |
| Svelte               | 5.57.2        |
| Vite / Svelte plugin | 8.3.4 / 7.3.1 |
| svelte-check         | 4.7.6         |
| TypeScript           | 6.0.3         |

Wails 3 is currently beta. TypeScript 7.0.2 is the latest release, but svelte-check 4.7.6 supports only TypeScript 5 and 6, so Raven uses the latest supported TypeScript 6 release.

## Project

- `funcs.go`: thin Wails facade, constructor injection, and executable Fx graph startup.
- `types.go`: facade-owned transport contracts and private application state.
- `internal/features/`: library, reminders, autostart, and companion features with owned ports and concrete adapters.
- Feature `composition/` packages expose Fx modules; domain, application, and ports packages remain independent of Fx. Desktop adapters own menus, events, and native window configuration.
- `internal/shared/`: reusable infrastructure, including selected-file access shared by media adapters.
- `frontend/src/main.ts` and `App.svelte`: adapter composition and selection of the workspace, widget, or floating review.
- `frontend/src/features/`: folders, cards, reviews, workspace, companion, and shared UI, with explicit public entry points and frontend-owned contracts.
- `internal/tooling/conventions/` and `frontend/tests/conventions.test.mjs`: filename and dependency-boundary checks, including public Go API types.
- `frontend/public/style.css`: shared workspace theme tokens.
- `raven.svg`: original supplied logo; copied to `frontend/public/raven.svg`.
- `build/appicon.png`, `build/darwin/icons.icns`, `build/windows/icon.ico`: icons rendered from the supplied SVG.

Backend dependencies are constructed through one Fx graph using `go.uber.org/fx` v1.24.0. The Wails shell establishes single-instance ownership before its lifecycle bridge starts storage and optional notifications. Reminder reconciliation starts after the native application readiness event. Wails keeps the native event loop on the main goroutine; Fx releases workers, notifications, and persistence in dependency order. Background work uses a runtime context independent of startup deadlines. Notification initialization failures leave the widget and review reconciliation available.

Install development tooling with `bun install` and UI dependencies with `bun install --cwd frontend`. After editing Go services, regenerate bindings with the pinned Wails CLI. Build frontend assets once before loading Go packages on a fresh checkout.

After changes, run the required checks in order:

```sh
wails3 generate bindings -ts -i -clean
task taskotter:ci:fix
task taskotter:ci
bun run --cwd frontend check
bun run --cwd frontend test
bun run --cwd frontend build
go test -race ./...
bun run lint
bun run unused
go vet ./...
```

## Windows Releases

Push a stable version tag such as `v0.1.0` to run **Release**. It runs the full
CI matrix before building a Windows x64 Inno Setup installer, checking silent
installation and uninstallation, and publishing the installer and SHA-256 checksum
on a GitHub Release with generated release notes. Version components must fit
Windows metadata (0–65535). Prerelease tags are rejected. Rerunning publication
replaces assets for the same tag.

The unsigned installer installs Raven for the current user, adds a Start Menu
shortcut, and offers an optional desktop shortcut. It bundles the WebView2
bootstrapper; installing the runtime requires internet access when it is missing.
Uninstalling preserves Raven's user data.

Release builds use the Inno Setup 6 compiler preinstalled on GitHub's Windows
runner, without downloading or downgrading it through Chocolatey. The workflow
checks that the compiler exists and logs its version before building.

To package locally on Windows, install the repository's Go, Bun, Wails, and Task
versions plus Inno Setup 6, then run from the repository root:

```sh
task windows:create:inno:installer ARCH=amd64 VERSION=0.1.0
```

The installer is written to `bin/raven-0.1.0-windows-amd64-setup.exe`. Override
`ISCC` if the compiler is outside `C:/Program Files (x86)/Inno Setup 6/ISCC.exe`.
For local packaging, use the version already in `build/windows/info.json`; the
release workflow applies its tag version to executable metadata automatically.
Existing NSIS and MSIX packaging commands remain available.
