# Raven Agent Instructions

## JavaScript and TypeScript Tooling

Always use Bun for JavaScript/TypeScript dependency management and package scripts. Use Bun's install, add, remove, run, and package execution commands. Run the existing test script with `bun run test`.

## TaskOtter Files

Treat `taskotter/**` as read-only generated tooling. Never create, edit, delete, rename, format, regenerate, or update files under it, including through automated commands. Put project-specific tasks in `taskfiles/NAME/Taskfile.yml`, and call TaskOtter tasks without changing their implementation. Keep the root `Taskfile.yml` as a symlink to `taskfiles/root/Taskfile.yml` so existing Task and Wails commands work from the repository root.

## Preserve the Colors

Keep Raven's monochrome palette and dark sidebar during future design, content, refactoring, or feature work. New components must reuse this palette. Neutral grayscale shades are allowed for borders, dividers, muted text, focus indicators, and interaction states; do not introduce colored accents or themes.

- Main content and dialogs: black `#000` backgrounds and white `#fff` primary text; primary buttons use white backgrounds with black text. Use subtle gray borders and dividers with a stronger gray for hover states, rather than pure white outlines. Reuse the `--line` and `--line-strong` tokens.
- Sidebar: `#181818b8` background, `#f2f2f2` foreground, `#a3a3a3` muted text, `#303030` dividers, `#ffffff0d` hover, and `#ffffff14` selected backgrounds.
- Floating widget: white circle with the original black logo; badges remain black and white.
- Preserve the native black window background, dark appearance, and monochrome theme metadata.

## Architecture

- Use ports and adapters architecture, organized by feature. Keep each backend feature's domain rules, application use cases, ports, and adapters together under `internal/`; reuse existing layers when extending a feature.
- Each `internal/features/<feature>/` root contains only layer directories: `domain/`, `application/`, `ports/`, `adapters/`, and `composition/`. Place all files, including tests and documentation, inside the appropriate layer; create only layers with content.
- Keep domain, application, and ports packages independent of adapters, composition packages, and third-party libraries. Adapters implement consumer-owned ports; composition packages wire concrete implementations.
- Every outbound adapter must explicitly implement a specific, named port owned by its consumer. Declare Go conformance with compile-time interface assertions in `vars.go`; use `implements`, `satisfies`, or a port-typed factory/function in TypeScript. Keep ports independent of concrete adapters and third-party types. Helpers inside an adapter share its port; Svelte UI adapters use typed props and callbacks as their contracts.
- Keep Raven's Wails services as the public facade. Put implementation details in `internal/` and expose only the contracts frontend consumers need.

## Backend Dependency Injection and Lifecycle

- Use `go.uber.org/fx` at the executable and feature `composition/` boundaries. Each feature composition package exposes a `Module`; the executable assembles one graph. Keep Fx imports, parameter/result objects, and registration in these boundaries.
- Inject runtime dependencies through constructors and consumer-owned ports. Constructors allocate stable objects; use Fx lifecycle hooks to acquire resources and start workers. Keep construction out of facade methods and avoid mutable service dependency installation or service locators.
- Composition packages may import Fx, concrete adapters, and their internal contracts. Keep ordinary constructors callable without Fx. Use typed providers or `fx.As` to bind adapters to ports, retaining compile-time conformance assertions.
- Preserve role-based filenames in composition packages: module variables in `vars.go`, providers and hook registration in `funcs.go`, and parameter/result objects in `types.go`.
- Fx parameter objects are passed by value and parameter/result fields are populated by reflection. Keep lint exceptions limited to these framework requirements; retain ordinary adapter and core checks.
- Wails owns the native event loop on the main goroutine. Its private lifecycle bridge starts and stops Fx once. Establish single-instance ownership before opening persistence, and wait for application readiness before starting reminders.
- Use a cancellable runtime context for background work and separate bounded contexts for startup and shutdown. Stop workers before closing their dependencies; release partial startup resources. Keep notification initialization optional and route Fx events through the injected structured logger.

## Go Package Files

Every Go package may contain only the following Go filenames and their platform/build variants. Replace `PACKAGENAME` with the actual package name; create only files that have content.

| Filename                          | Contents                                                                            |
| --------------------------------- | ----------------------------------------------------------------------------------- |
| `funcs.go`                        | Functions and methods                                                               |
| `vars.go`                         | Package variables, including sentinel errors                                        |
| `consts.go`                       | Constants                                                                           |
| `types.go`                        | Type declarations and package documentation                                         |
| `PACKAGENAME_test.go`             | Tests of unexported implementation, using `package PACKAGENAME`                     |
| `PACKAGENAME_integration_test.go` | Tests of exported behavior, using `package PACKAGENAME_test` and only exported APIs |
| `PACKAGENAME_bench_test.go`       | Benchmarks                                                                          |

Platform/build variants must retain the role-based name, such as `funcs_darwin.go`, `funcs_other.go`, `vars_windows.go`, or `PACKAGENAME_integration_darwin_test.go`. Use the appropriate filename suffixes and build constraints for the target platform or build mode.

Place package documentation on the package declaration in `types.go`. Consolidate Go code into the allowed filenames when intentionally changing a package; names such as `doc.go`, `errors.go`, and additional test filenames are outside this convention. Non-Go repository files are unaffected by the Go filename rules.

## Public API Boundaries

- Keep implementation types, fields, helpers, and state unexported wherever possible. Export symbols inside `internal/` only when another internal package needs them.
- Frontend-facing facade signatures, exported fields, type aliases, and interface methods must use public facade types or standard-library types. Keep internal types, Fx types, and third-party adapter types out of this public API.
- Translate implementation errors, events, and options at the facade boundary. Expose public error contracts without leaking concrete internal or third-party errors through error chains.
- Keep frontend-facing consumers independent of internal packages and concrete adapters. Exported APIs inside `internal/` may use internal contracts; composition APIs may additionally use Fx and adapter types. These are implementation boundaries, not public Wails contracts.

## Svelte Architecture

- Organize frontend features under `frontend/src/features/<feature>/`, keeping each feature's domain logic, application use cases, ports, adapters, and components together. Reuse existing feature layers when extending behavior.
- Keep domain and application modules independent of Svelte, DOM APIs, generated Wails bindings, and concrete adapters. Define ports using frontend-owned contracts and wire concrete dependencies at application entry points.
- Treat Svelte components as UI adapters. Keep domain rules and application workflows in their feature modules; keep component-owned interaction state and rendering in components.
- Use typed props and callbacks, `$state` for reactive UI state, and `$derived` for computed values. Use effects for side effects and clean up subscriptions, event listeners, timers, and other resources when their owner is disposed.

## Svelte and TypeScript Files

Use the following filenames within each feature layer or adapter directory; create only files that have content. Replace `Component` and `feature` with descriptive names.

| Filename                       | Contents                                                        |
| ------------------------------ | --------------------------------------------------------------- |
| `Component.svelte`             | PascalCase UI components with typed props and callbacks         |
| `funcs.ts`                     | Functions and application use cases                             |
| `vars.ts`                      | Module variables and state without Svelte runes                 |
| `consts.ts`                    | Constants                                                       |
| `types.ts`                     | Type declarations, domain contracts, and port interfaces        |
| `index.ts`                     | Explicit exports forming the feature's public interface         |
| `funcs.svelte.ts`              | Shared functions that require Svelte runes                      |
| `vars.svelte.ts`               | Shared reactive state that requires Svelte runes                |
| `feature.test.mjs`             | Implementation tests using the existing Node test runner        |
| `feature.integration.test.mjs` | Public behavior tests using only the feature's public interface |

Keep tests under `frontend/tests/` so the existing Node test runner discovers both test forms. Application entry points, configuration files, ambient declarations, and generated files are exempt from helper filename rules. Component-local functions, types, and state may remain in the component. Consolidate feature helpers into the allowed filenames when intentionally modifying a feature.

## Svelte Public Boundaries

- Export only the components, functions, and frontend-owned types consumers need through the feature's `index.ts`. Import other features through their public interfaces.
- Keep generated Wails binding imports and transport types inside adapters. Translate backend data and errors into frontend-owned contracts before exposing them to application modules or components.
- Keep adapter instances and implementation state private. Supply dependencies through ports and typed props or callbacks rather than coupling components to concrete backend services.

These conventions apply when intentionally changing a package or feature; an instruction-only update does not require reorganizing existing implementation code.

## Required Verification After Every Change

After each change, including documentation or configuration edits, run these commands from the repository root in this order:

```sh
task taskotter:ci:fix
task taskotter:ci
```

Both commands must finish successfully before reporting completion. Inspect changes produced by the fix task. Resolve failures and rerun both commands after further edits until the final tree passes. If an external blocker prevents a successful run, report the exact blocker and failed command; keep the task incomplete rather than claiming success or skipping checks.

When frontend code or configuration changes, also run these commands from the repository root after the TaskOtter pair:

```sh
bun run --cwd frontend check
bun run --cwd frontend test
bun run --cwd frontend build
```

All applicable frontend commands must finish successfully before reporting completion. After further edits, rerun the TaskOtter pair followed by the frontend commands.

## Repository Inspection

Prefer lean-ctx for repository work: `ctx_read` for file reads, `ctx_search` for code searches, and `ctx_shell` or `/opt/homebrew/bin/lean-ctx -c "<command>"` for shell commands. Read `/Users/mostafakhairy/.codex/LEAN-CTX.md` when you need the full routing rules.
