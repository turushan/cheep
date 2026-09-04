# Cheep

The safe, unofficial Namecheap CLI for humans, scripts, and agents.

Cheep makes Namecheap account, domain, and DNS automation predictable from a terminal. It is
designed around explicit environments, stable machine output, declarative DNS changes, and strict
spending controls.

Cheep is an independent open-source project. It is not affiliated with or endorsed by Namecheap.

The project website will live at [cheep.sh](https://cheep.sh). Its source belongs in `website/` in
this repository so command changes and their documentation can ship together.

## Status

The project is under active development. The current command surface supports sandbox-first
authentication, read-only account and domain inspection, exact pricing, and guarded whole-zone DNS
workflows. Domain purchases and other charge-bearing mutations do not exist yet.

## Build from source

Go 1.26.6 or newer is required because the official Namecheap SDK requires it.

```bash
go install github.com/turushan/cheep/cmd/cheep@latest
```

For development:

```bash
make check
make build
./bin/cheep version
```

## Start with the sandbox

```bash
cheep --environment sandbox auth configure sandbox \
  --api-user USER \
  --client-ip IPV4 \
  --default
cheep --profile sandbox auth set-key
cheep --profile sandbox doctor
cheep --profile sandbox domains check example.com
```

Read [authentication](docs/AUTHENTICATION.md), [DNS safety](docs/DNS.md),
[automation](docs/AUTOMATION.md), and [the safety contract](docs/SAFETY.md) before using mutation
commands.

## License

MIT
