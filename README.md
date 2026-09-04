# Cheep

[![CI](https://github.com/turushan/cheep/actions/workflows/ci.yml/badge.svg)](https://github.com/turushan/cheep/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/license-MIT-171713.svg)](LICENSE)

**Namecheap at command speed.**

Cheep is a safe, unofficial Namecheap CLI with friendly output for people and stable JSON for AI
agents, scripts, and CI. Check domains, inspect account prices, manage DNS through reviewed plans,
or call any method in Namecheap's official API catalog without opening the dashboard.

[Website](https://cheep.sh) · [Authentication](docs/AUTHENTICATION.md) ·
[Complete API access](docs/API.md) · [Automation contract](docs/AUTOMATION.md) ·
[Safety model](docs/SAFETY.md)

Cheep is independent. It is not affiliated with or endorsed by Namecheap.

## Install

Go 1.26.6 or newer is currently required. Prebuilt binaries and Homebrew follow the first tagged
release.

```bash
go install github.com/turushan/cheep/cmd/cheep@latest
cheep version
```

## Check your first domain

Namecheap requires an API user, API key, and whitelisted public IPv4. Create those in the
[Namecheap sandbox](https://www.sandbox.namecheap.com/) if you want to try Cheep without touching
production.

Save the non-secret profile fields:

```bash
cheep --environment sandbox auth configure sandbox \
  --api-user YOUR_SANDBOX_USER \
  --client-ip YOUR_PUBLIC_IPV4 \
  --default
```

Store the API key through a hidden prompt, verify access, and check as many names as you want:

```bash
cheep auth set-key
cheep doctor
cheep domains check paperkite.dev paperkite.com
```

Human output stays compact:

```text
DOMAIN          AVAILABLE  PREMIUM  PREMIUM REGISTRATION
paperkite.dev   yes        no
paperkite.com   no         no
```

The names and results above are illustrative. Cheep always asks Namecheap for the live answer.

## Use Cheep from an AI agent

Add `--json` for a versioned envelope, `--no-input` to forbid prompts, and `--readonly` when the
task must not change remote state:

```bash
cheep --json --no-input --readonly domains check paperkite.dev
```

```json
{
  "schema_version": "v1",
  "command": "domains.check",
  "ok": true,
  "data": [
    {
      "domain": "paperkite.dev",
      "available": true,
      "premium": false
    }
  ],
  "meta": {
    "profile": "sandbox",
    "environment": "sandbox"
  }
}
```

For headless environments, set `CHEEP_API_USER`, `CHEEP_API_KEY`, `CHEEP_CLIENT_IP`, and
`CHEEP_ENVIRONMENT`. Never put an API key in a command argument.

Useful agent discovery commands:

```bash
cheep schema --json
cheep api methods --json
cheep api describe domains/renew --json
cheep --help
cheep domains check --help
```

Successful data goes to stdout. Diagnostics and errors go to stderr. See the
[automation contract](docs/AUTOMATION.md) for stable fields, exit codes, timeouts, and stream
behavior.

## Everyday commands

| Goal | Command |
|---|---|
| Check one or more names | `cheep domains check example.com example.dev` |
| List account domains | `cheep domains list` |
| Inspect one domain | `cheep domains info example.com` |
| Read an exact account price | `cheep account pricing com --action register` |
| List TLD capabilities | `cheep tlds list` |
| Export a DNS zone | `cheep dns export example.com --file zone.yaml` |
| Preview a desired DNS zone | `cheep dns plan example.com --file zone.yaml` |
| List every supported Namecheap method | `cheep api methods` |
| Call any official API method | `cheep api domains get-contacts -p DomainName=example.com` |
| Inspect the command inventory | `cheep schema --json` |

Run `cheep <command> --help` for the complete flags and arguments.

## Safety defaults

- Sandbox is the default environment.
- API keys go to the operating system credential manager, not the config file.
- `--readonly` refuses every remote mutation.
- `--dry-run` calculates mutations without applying them.
- `--no-input` prevents hidden interactive prompts in automation.
- Each command has a bounded timeout.
- DNS uses export, plan, apply, and restore instead of individual blind edits.
- Generic mutations require `--yes`; production mutations also require `--production`.
- Charge-bearing API methods additionally require `--accept-charge` after a current price check.

Read [complete API access](docs/API.md), [DNS workflows](docs/DNS.md), and the
[safety contract](docs/SAFETY.md) before applying a mutation.

## Current status

Cheep is in active development. Its purpose-built commands cover authentication, account and domain
inspection, exact pricing, TLD inspection, and guarded whole-zone DNS workflows. The `cheep api`
surface provides named access to every method in Namecheap's official API catalog, including domain
registration, renewal, transfers, SSL, users, addresses, and domain privacy.

The website is deployed separately. Its code and assets are never included in CLI release archives
or Homebrew installations.

## Development

```bash
make check
make build
./bin/cheep version
```

## License

[MIT](LICENSE)
