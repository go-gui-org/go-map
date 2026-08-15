# Contributing

## Prerequisites

- Go 1.26+
- SDL2 + glyph dependencies (see go-gui README)

## Build and Test

Run the full local validation gate before pushing a branch:

```
make prepush
```

`make prepush` approximates the CI matrix from one host: race-enabled tests,
`go vet`, lint, and a build. It aborts on the first failing target. Individual
targets (`make test`, `test-race`, `vet`, `lint`, `build`) are available for a
tighter loop while iterating.

Gate targets run with `GOWORK=off` so they resolve the versions in `go.mod`,
which is what CI does — a local `go.work` pointing at a sibling checkout would
otherwise validate something CI never sees.

### CI-only validation

`make prepush` covers one host. CI additionally runs the whole suite on both
`ubuntu-latest` and `macos-latest`, so a platform-specific failure on the OS you
are not using can only be caught there.

## Coding Conventions

- No variable shadowing. Use `=` to reassign, not `:=`.
- Widgets follow the `*Cfg` struct pattern from go-gui.
- Event callbacks: `func(gui.EventCtx)` — `ctx.Layout`, `ctx.Event`,
  `ctx.Window`. `OnClick` and the other consume-class events are marked handled
  by dispatch before the callback runs; call `ctx.Bubble()` to pass one on.
  Everything else calls `ctx.Consume()` to stop propagation.
- Wrap comments at 90 columns when practical.
- Favor reducing heap allocations.
- Use glyph (via go-gui) for text; consult it before writing new text routines.

## License

Contributions accepted under [MIT](LICENSE).
