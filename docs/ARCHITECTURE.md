# Architecture

NC CLI has four boundaries.

1. `cmd/nccli` starts the process and returns a stable exit status.
2. `internal/cli` defines commands, flags, validation, and presentation.
3. Internal application services express provider-independent operations.
4. The Namecheap adapter uses the official Go SDK and converts provider types into NC CLI models.

The command layer does not call the SDK directly. This boundary keeps safety policy testable and
allows a narrow direct API implementation if an official SDK method is missing.

## Configuration

Non-secret profile data belongs in the platform configuration directory. API keys belong in the
operating-system keychain. Environment variables support CI and other headless environments. NC CLI
never accepts an API key as a command argument.

The default file uses the platform configuration directory returned by Go. This normally means
`~/Library/Application Support/nccli/config.toml` on macOS and `~/.config/nccli/config.toml` on
Linux. The file is written atomically with mode `600` on systems that support Unix file modes.

The precedence order is:

1. Explicit non-secret command flags
2. `NCCLI_*` environment variables
3. Compatible `NAMECHEAP_*` environment variables
4. The selected profile
5. Safe defaults

The environment must resolve to `sandbox` or `production`. An absent environment resolves to
`sandbox`. The application never relies on the SDK's production zero value.

## Output

Human-readable output is the default. `--json` emits one versioned JSON document to stdout. Data
uses stdout. Diagnostics and errors use stderr. See [AUTOMATION.md](AUTOMATION.md).

## Release

GitHub Releases are the canonical artifact source. GoReleaser builds static executables for macOS,
Linux, and Windows. Homebrew is the primary installation channel after public launch. Source builds
remain available through `go install`.
