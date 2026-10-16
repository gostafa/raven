# Raven agent instructions

## Preserve the colors

Keep Raven's monochrome palette and dark sidebar during future design, content, refactoring, or feature work. New components must reuse this palette. Neutral grayscale shades are allowed for borders, dividers, muted text, focus indicators, and interaction states; do not introduce colored accents or themes.

- Main content and dialogs: black `#000` backgrounds and white `#fff` primary text; primary buttons use white backgrounds with black text. Use subtle gray borders and dividers with a stronger gray for hover states, rather than pure white outlines. Reuse the `--line` and `--line-strong` tokens.
- Sidebar: `#181818b8` background, `#f2f2f2` foreground, `#a3a3a3` muted text, `#303030` dividers, `#ffffff0d` hover, and `#ffffff14` selected backgrounds.
- Floating widget: white circle with the original black logo; badges remain black and white.
- Preserve the native black window background, dark appearance, and monochrome theme metadata.

## lean-ctx

Prefer lean-ctx MCP tools: `ctx_shell` for commands, `ctx_read` for file reads, and `ctx_search` for code search. Full rules: `/Users/mostafakhairy/.codex/LEAN-CTX.md`.
