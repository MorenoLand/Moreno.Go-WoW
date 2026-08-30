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

The sign-in form is the primary configuration path. `WOW_ACCOUNT`, `WOW_PASSWORD`, `WOW_AUTH`, `WOW_REALM`, and `WOW_DATA` are optional launch-time overrides.

Build the attached-console executable with `scripts\build.ps1` or `scripts/build.sh`; the output is `bin/MorenoWoW.exe` on Windows. Run `bin\MorenoWoW.exe --debug` from a terminal to keep text diagnostics attached to the G3N window. Ctrl+C requests shutdown of both.

The client creates `MorenoWoW/config.json` under the platform user configuration directory and remembers the auth address, account, locale, realm selector, and MPQ data path. It never writes the password to that file. Use the form, `--data`, or `WOW_DATA` to set the data path.

## Development

Use the repository’s documented toolchain and commands as they are added. For a standard Go layout, the usual checks are:

```sh
gofmt -w .
go test ./...
go build ./...
```

Build and test results confirm the checked code only; runtime behavior and visual parity require testing the actual client with the intended game data.
