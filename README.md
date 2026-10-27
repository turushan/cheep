<div align="center">
  <a href="https://cheep.sh"><img src="https://cheep.sh/bird.svg" alt="Cheep's bird" width="112"></a>
  <h1>Cheep</h1>
  <p><strong>Namecheap at command speed.</strong></p>
  <p>An unofficial, open-source Namecheap CLI for people, scripts and AI agents.<br>
  Check domains, read your account's prices and change DNS through reviewed plans, without the dashboard.</p>
  <p>
    <a href="#install"><strong>Install</strong></a>
    &nbsp; · &nbsp;
    <a href="https://cheep.sh"><strong>cheep.sh</strong></a>
    &nbsp; · &nbsp;
    <a href="https://cheep.sh/docs"><strong>Guides</strong></a>
  </p>
  <p>
    <a href="https://github.com/turushan/cheep/releases/latest"><img src="https://img.shields.io/github/v/release/turushan/cheep?label=release&color=e7622e" alt="Latest release"></a>
    <a href="https://github.com/turushan/cheep/actions/workflows/ci.yml"><img src="https://github.com/turushan/cheep/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
    <img src="https://img.shields.io/badge/macOS%20%C2%B7%20Linux%20%C2%B7%20Windows-171713" alt="macOS, Linux and Windows">
    <img src="https://img.shields.io/github/go-mod/go-version/turushan/cheep?color=171713" alt="Go version">
    <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-171713" alt="MIT license"></a>
  </p>
</div>

<p align="center">
  <a href="https://cheep.sh/media/cheep-film.74fff4a9.mp4">
    <picture>
      <source media="(prefers-reduced-motion: reduce)" srcset="https://cheep.sh/media/cheep-demo-poster.c1ea9af4.png">
      <img src="https://cheep.sh/media/cheep-demo.98fe3d29.gif" alt="Animated demo: one command checks four names, a split-flap board flips the answers, then Cheep's bird runs in with the words Namecheap at command speed" width="800">
    </picture>
  </a>
</p>
<p align="center"><sub>Illustrated demo · Real Cheep output with sample data · <a href="https://cheep.sh/media/cheep-film.74fff4a9.mp4"><strong>Watch the full film with sound</strong></a></sub></p>

## One command, every name

Ask about as many names as you like. Namecheap answers up to 50 of them in one request.

```text
$ cheep domains check paperkite.dev paperkite.com paperkite.app paperkite.co
DOMAIN         AVAILABLE  PREMIUM  PREMIUM REGISTRATION
paperkite.dev  yes        no       0.00
paperkite.com  no         no       0.00
paperkite.app  yes        no       0.00
paperkite.co   yes        yes      1288.00
```

The names and results here are illustrative. Cheep always asks Namecheap for the live answer, and
`cheep account pricing dev --action register` reads your own account's price before you buy.

## Real money needs a real yes

In production, any call that can charge your account stops until you add three flags. Cheep names the
missing one each time, so a slip costs you a retry, never money.

```text
$ cheep api domains create --params-file register.json
Error: API mutations require --yes after reviewing a --dry-run
$ cheep api domains create --params-file register.json --yes
Error: production API mutations require --production
$ cheep api domains create --params-file register.json --yes --production
Error: this method can charge the account; pass --accept-charge after checking current pricing
```

| Flag | What it confirms |
|---|---|
| `--yes` | You reviewed the change. Run it with `--dry-run` first to see the parameters Cheep will send. |
| `--production` | The change is for your real account, not the sandbox. |
| `--accept-charge` | You checked the current price before Cheep spends anything. |

New profiles start in Namecheap's sandbox, so nothing real changes while you learn.

## Read the plan, then change DNS

Namecheap replaces a whole zone in one write, so Cheep treats every change as a whole-zone change.
Export the zone, edit the file, read the plan, then apply it.

```text
$ cheep dns plan paperkite.dev --file zone.yaml
Domain: paperkite.dev
Email type: NONE -> NONE
Changes: +1 -1 =1
   HOST  TYPE  VALUE         TTL   MX
+  @     A     203.0.113.20  1799
   HOST  TYPE  VALUE       TTL   MX
-  @     A     192.0.2.10  1799
$ cheep dns apply paperkite.dev --file zone.yaml --confirm-domain paperkite.dev --production
PRODUCTION DNS apply for paperkite.dev. Snapshot: /Users/you/Library/Application Support/cheep/snapshots/paperkite.dev/20261027T143000.000000000Z.yaml
Applied DNS zone for paperkite.dev: +1 -1 =1.
```

Apply saves a snapshot first, reads the live zone again right before it writes, and stops if the zone
changed in the meantime. `cheep dns restore` puts any snapshot back with one command.

## Built for AI agents

Your agent runs the same commands you do and gets a stable JSON envelope. Data goes to stdout, errors
go to stderr, and the exit codes are documented.

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
      "premium": false,
      "premium_registration_price": 0,
      "premium_renewal_price": 0,
      "premium_restore_price": 0,
      "premium_transfer_price": 0,
      "icann_fee": 0,
      "eap_fee": 0
    }
  ],
  "meta": {
    "profile": "sandbox",
    "environment": "sandbox"
  }
}
```

Cheep prints the envelope on one line; it is indented here for reading. Under `--readonly`, every
change to the account is refused and the command exits with code 7:

```text
$ cheep --json --no-input --readonly dns apply paperkite.dev --file zone.yaml --confirm-domain paperkite.dev --production
{"schema_version":"v1","command":"dns.apply","ok":false,"data":null,"error":{"code":"readonly","message":"--readonly refuses DNS mutations"}}
```

| Flag | For agents |
|---|---|
| `--json` | A versioned envelope with the command, the result and the profile it used. |
| `--no-input` | Never waits on a prompt, so an unattended run cannot hang. |
| `--readonly` | Refuses every change to your Namecheap account. |

Agents can discover the whole surface with `cheep schema --json`, `cheep api methods --json` and
`cheep api describe domains/renew --json`. The [automation contract](docs/AUTOMATION.md) lists every
exit code, stable field and timeout.

## The whole API, too

Purpose-built commands cover the daily work. For everything else, `cheep api` reaches all 59 methods in
Namecheap's official catalog, from SSL to transfers, and every write or charge waits for its flags.

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

Run `cheep <command> --help` for every flag and argument.

## Install

With Homebrew on macOS or Linux:

```bash
brew install turushan/tap/cheep
```

With Go 1.26.10 or newer:

```bash
go install github.com/turushan/cheep/cmd/cheep@latest
```

Prebuilt archives for macOS, Linux and Windows, with checksums, are on the
[releases page](https://github.com/turushan/cheep/releases/latest).

### Your first check, in the sandbox

Namecheap needs an API user, an API key and a whitelisted public IPv4 before any command can run.
Create them in the [Namecheap sandbox](https://www.sandbox.namecheap.com/) to try Cheep without
touching production. Save the profile, store the key through a hidden prompt, and let `doctor` check
everything:

```bash
cheep --environment sandbox auth configure sandbox \
  --api-user YOUR_SANDBOX_USER \
  --client-ip YOUR_PUBLIC_IPV4 \
  --default
cheep auth set-key
cheep doctor
cheep domains check paperkite.dev paperkite.com
```

The [authentication guide](docs/AUTHENTICATION.md) covers production profiles and CI.

## Private by design

Cheep runs on your computer and talks only to Namecheap's API. Nothing passes through a server of
ours, and there is no telemetry, no analytics and no update check. Your API key stays in the operating
system's credential manager: Keychain on macOS, Credential Manager on Windows and Secret Service on
Linux. On servers Cheep reads `CHEEP_API_KEY` instead. Never put a key in a command argument.

<details>
<summary><strong>Build it yourself</strong></summary>

Requires Go 1.26.10 or newer.

```bash
git clone https://github.com/turushan/cheep.git
cd cheep
make check
make build
./bin/cheep version
```

The website at cheep.sh is built separately; its code and media never ship in the CLI's release
archives or Homebrew installs.

</details>

[DNS workflows](docs/DNS.md) · [Complete API access](docs/API.md) · [Automation contract](docs/AUTOMATION.md) ·
[Safety contract](docs/SAFETY.md) · [Authentication](docs/AUTHENTICATION.md) · [MIT license](LICENSE)

<sub>Cheep is independent and unofficial. It is not affiliated with or endorsed by Namecheap. Prices and
availability always come live from Namecheap.</sub>
