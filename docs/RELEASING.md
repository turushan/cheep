# Releasing

GitHub Releases are the canonical binary source. GoReleaser builds macOS, Linux, and Windows
archives for amd64 and arm64.

1. Run `make check`, `make lint`, `make vuln`, and `make release-check` on a clean tree.
2. Create and push an annotated `v*` tag.
3. Let the release workflow create a draft release.
4. Confirm that checksum verification and the Linux archive smoke test pass.
5. Inspect the draft notes and artifacts before publishing the release.

The release workflow never publishes a draft automatically.
