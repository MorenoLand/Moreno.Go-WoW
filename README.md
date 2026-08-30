# A data-driven 3D game client in Go

This repository contains a Go client project focused on accurate, data-driven world, model, animation, and gameplay behavior.

## Scope and provenance

The source code in this repository is original work for this project. It does not include or redistribute Blizzard Entertainment source code, proprietary client binaries, game assets, extracted data, or other Blizzard-owned content. This is an independent project and is not affiliated with, endorsed by, or sponsored by Blizzard Entertainment.

Any separately supplied game data remains outside this repository and must be used only where lawful. Third-party dependencies remain subject to their own licenses.

## Status

Initial G3N renderer bootstrap. Interfaces and behavior may change as the client develops.

## Running

```sh
go run .
```

Set `WOW_ACCOUNT` and `WOW_PASSWORD` before running. `WOW_AUTH` may override the default `127.0.0.1:3724` auth address, and `WOW_REALM` may select a realm by name, address, or id.

## Development

Use the repository’s documented toolchain and commands as they are added. For a standard Go layout, the usual checks are:

```sh
gofmt -w .
go test ./...
go build ./...
```

Build and test results confirm the checked code only; runtime behavior and visual parity require testing the actual client with the intended game data.
