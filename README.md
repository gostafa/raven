# Raven

A desktop app for remembering what you learn. Save text, images, audio, and video in cards, organize them into folders, and review each item with spaced repetition.

Built with Go, Wails 3, and Svelte for macOS, Windows, and Linux.

## Features

- Mixed-media cards, folders, tags, and search.
- Scheduled reviews with **Again**, **Hard**, **Good**, and **Easy** ratings.
- A floating widget with due counts and quick reviews.
- Local storage, desktop reminders, and optional start at login.

Closing the main window keeps Raven running. Use **Quit Raven** in the widget or tray menu to exit.

## Development

Install Go **1.27.2**, Bun **1.4.2**, Node.js **26.12.0+**, Task, and the native dependencies for your platform:

| Platform | Requirements |
| --- | --- |
| macOS | macOS 13+ and Xcode Command Line Tools |
| Windows | Windows 10/11 and Microsoft Edge WebView2 Runtime |
| Linux | GTK4 and WebKitGTK 6.0 development libraries; Ubuntu 24.04 is the baseline |

From the repository root:

```sh
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.29
bun install
bun install --cwd frontend
bun run start
```

Ensure your Go bin directory is on `PATH`. Development generates frontend bindings automatically. Wails 3 is a beta dependency.

## Build

Run on the target operating system:

```sh
bun run build
bun run package
```

Output is written to `bin/`. Windows installer packaging also requires Inno Setup 6.

## Checks

Run from the repository root, in order:

```sh
task taskotter:ci:fix
task taskotter:ci
```

For frontend changes, also run:

```sh
bun run --cwd frontend check
bun run --cwd frontend test
bun run --cwd frontend build
```

## Your Data

Raven stores its database and managed media in the user configuration directory:

| Platform | Location |
| --- | --- |
| macOS | `~/Library/Application Support/raven/` |
| Windows | `%AppData%\raven\` |
| Linux | `$XDG_CONFIG_HOME/raven/` or `~/.config/raven/` |

Quit Raven before backing up **both `raven.db` and the entire `media/` directory**. Imported media is copied into Raven; moving the original files does not affect saved cards.
