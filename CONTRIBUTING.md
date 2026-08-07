# Contributing

## Prerequisites

- Go 1.26+
- SDL2 + glyph dependencies (see go-gui README)

## Build and Test

```
go build ./...
go vet ./...
go test ./...
golangci-lint run ./...
gofmt -l .
```

All must pass before committing.

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
