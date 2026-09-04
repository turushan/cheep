# Architecture

Cheep has four boundaries.

1. `cmd/cheep` starts the process and returns a stable exit status.
2. `internal/cli` defines commands, flags, validation, and presentation.
3. Internal application services express provider-independent operations.
4. The Namecheap adapter uses the official Go SDK and converts provider types into Cheep models.

The command layer does not call the SDK directly. This boundary keeps safety policy testable and
allows a narrow direct API implementation if an official SDK method is missing.

## Configuration

Non-secret profile data belongs in the platform configuration directory. API keys belong in the
operating-system keychain. Environment variables support CI and other headless environments. Cheep
never accepts an API key as a command argument.

The default file uses the platform configuration directory returned by Go. This normally means
`~/Library/Application Support/cheep/config.toml` on macOS and `~/.config/cheep/config.toml` on
Linux. The file is written atomically with mode `600` on systems that support Unix file modes.

The precedence order is:

1. Explicit non-secret command flags
2. `CHEEP_*` environment variables
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

## Website

The `cheep.sh` website belongs in `website/` in the same public repository as the CLI. The Go module
stays at the repository root. The website keeps its own Node package, tests, Wrangler configuration,
and deployment workflow inside that directory.

Repository membership does not imply artifact membership. GoReleaser and Homebrew distribute only
the compiled `cheep` binary, `LICENSE`, and `README.md`. The website source is deployed only by its
Cloudflare workflow and is never included in CLI archives or Homebrew installations.

The first website should be a static documentation and landing site deployed through a Cloudflare
Worker with static assets. It should explain installation, authentication, safety guarantees,
commands, and the independent-project disclaimer. It must never receive or process Namecheap
credentials.

One repository gives Cheep one issue tracker, one contributor community, one star count, and docs
that can change in the same pull request as the commands they describe. A separate website
repository becomes justified only if the site develops an independent product lifecycle or a
stateful backend.
