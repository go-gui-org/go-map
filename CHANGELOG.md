# Changelog

All notable changes are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## Unreleased

## v0.7.0 - 2026-09-05

- Bump go-gui v0.68.0 → v0.69.0 (go-glyph v1.25.0 indirect). No migration
  needed: no use of the removed `Scrollable` fields.

## v0.6.0 - 2026-09-05

- Bump go-gui v0.66.1 → v0.68.0 and go-glyph v1.24.0 → v1.25.0 (indirect).

## v0.5.0 - 2026-09-02

### Changed

- **The module path is now `github.com/go-gui-org/go-map`.** This is the first
  tag under the new path. Tag v0.4.1 and earlier declare
  `github.com/mike-ward/go-map`, so a consumer that wants a tagged build must
  move to v0.5.0. Change the import paths, then run `go mod tidy`.
- **Bump go-gui v0.53.0 -> v0.66.1 and go-glyph -> v1.24.0.** This covers the
  per-scope effective IDs, the `ColorSet` per-state colors, the `Padding` and
  `Sizing` self-flag change, the single-method `View` interface, and the
  window-owned theme. The notes below describe the earlier steps of the same
  migration.
- `mapview` calls `gui.GenerateViewLayout` again. The local `layoutRecursive`
  fork is gone.
- `make prepush` runs the full local gate: race tests, lint, cross-compile, and
  the export audit.
- Structural lines use `DrawContext.Scale`, so a hairline is one physical pixel
  on a HiDPI display.
- **Bump go-gui v0.52.0 → v0.53.0.** go-gui's nine input factories now panic on
  an empty `Cfg.ID`: focus traversal and per-widget state are keyed by it, so a
  control without one renders and clicks but is unreachable by keyboard. No
  library code is affected — the two forced edits are both toolbar buttons in
  `examples/full-map`. The per-city buttons are keyed `"city:" + c.Name` rather
  than a constant, because they are built in a loop and a shared ID would
  collapse them onto one tab stop and one state slot.
- **BREAKING: event callbacks take a single `gui.EventCtx`.** Bump go-gui
  v0.51.1 → v0.52.0. `(*gui.Layout, *gui.Event, *gui.Window)` becomes
  `func(gui.EventCtx)`.
- **Consume-class callbacks are handled by default.** `OnClick` is marked
  handled by dispatch before the callback runs. One behaviour change follows: a
  click landing on the InfoWindow body used to keep bubbling to ancestors after
  `handlePopupClick` absorbed it, and is now consumed — which is what the
  surrounding comment always said should happen.
- Migration guide upstream: `docs/migration-eventctx.md` in go-gui.

## v0.4.1 — 2026-05-17

### Changed

- Bump `go-gui` to v0.19.1 (scroll phase bridge, context menu focus fix,
  animation heartbeat, Metal autorelease fix)
- Bump `go-glyph` to v1.7.1 (indirect)

## v0.4.0 — 2026-04-30

### Changed

- Bump `go-gui` to v0.17.0
- Bump `go-glyph` to v1.7.0 (indirect)

## v0.3.2 — 2026-04-26

### Changed

- `mapview`: split `input.go` into `keyboard.go`, `pan.go`, `scroll.go`; harden
  non-finite coordinates
- Bump `go-gui` to v0.12.7

### Added

- Docs: `tile-mapview` deep-dive; ignore antivibe deep-dive dir

## v0.3.1 — 2026-04-19

### Added

- Architecture document (`docs/architecture.md`)
- Benchmark test suites for `tile`, `tile/wms`, and `mapview`

### Changed

- `mapview`: state-version counter (`bumpVersion`) replaces per-frame
  `FrameCount` version — no-op frames replay cached tessellation without calling
  `OnDraw`
- `mapview`: fractional-zoom scale-bar spacing tolerance relaxed to 0.01 px

### Fixed

- Various lint and code-review fixes across `tile` and `mapview`

## v0.1.0 — initial

### Added

- `projection`: `LatLng`, `Point`, `Bounds`, Web Mercator
  `Project`/`ProjectF`/`Unproject`/`UnprojectF`; `TileSize = 256`
- `tile`: `Coord{Z,X,Y}`, `Source` interface, `OSM()` / `OSMWithUserAgent()`
  adapters, LRU `Cache`
- `mapview`: interactive tile rendering, pan/zoom state registry, input
  handlers, scale bar, attribution, home button, `OnMove`/`OnZoomChange`
  callbacks
- Phase 1 tests: viewport math, zoom-to-cursor, dateline wrap, OSM UA
- `examples/basic`: minimal runnable OSM demo
