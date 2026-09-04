# Complete Namecheap API access

Cheep exposes every method listed in Namecheap's official API catalog below `cheep api`. Existing
purpose-built commands remain the simplest option for common domain, account, and DNS work. The
complete API surface exists for advanced workflows and for methods that do not yet have a richer
Cheep model.

## Discover methods

```bash
cheep api methods
cheep api methods --json
cheep api describe domains/create
cheep api describe namecheap.ssl.activate --json
```

`api methods` is generated from the same catalog that builds the command tree. Each entry includes
the CLI path, exact Namecheap command name, documentation link, mutation status, and charge status.
The catalog covers these official families:

- `domains`
- `domains dns`
- `domains ns`
- `domains transfer`
- `ssl`
- `users`
- `users address`
- `domainprivacy`, sent to Namecheap using its legacy `whoisguard` command names

## Pass parameters

Namecheap's method documentation defines the required parameter names. Pass non-sensitive values
with repeatable `--param` flags:

```bash
cheep api domains get-contacts --param DomainName=example.com
cheep api domains check --param DomainList=example.com,example.net --json
```

Use a JSON object when a method needs many fields:

```bash
cheep api domains create --params-file registration.json --dry-run
```

JSON strings, numbers, booleans, and null values become Namecheap string parameters. Use `-` to read
the object from stdin. Files containing contact or payment data should have mode `0600` and should
never be committed.

Never place a password, transfer code, CSR, or payment credential directly in `--param`. Read it
from an environment variable without putting the value in shell history:

```bash
export CHEEP_TRANSFER_CODE='replace-me'
cheep api domains transfer create \
  --param DomainName=example.com \
  --secret-param EPPCode=CHEEP_TRANSFER_CODE \
  --dry-run
```

Cheep rejects attempts to override `ApiUser`, `ApiKey`, `UserName`, `ClientIp`, or `Command`. The
selected profile always owns those fields.

## Apply mutations

Start with sandbox and a dry run:

```bash
cheep --environment sandbox --dry-run \
  api domains create --params-file registration.json
```

A dry run makes no API request and redacts recognized secret parameters. Apply a non-charge-bearing
mutation with `--yes`. Add `--accept-charge` for a method that can spend account funds:

```bash
cheep --environment sandbox \
  api domains create --params-file registration.json \
  --yes --accept-charge
```

Production mutations need one additional gate:

```bash
cheep --environment production \
  api domains renew --param DomainName=example.com --param Years=1 \
  --yes --production --accept-charge
```

Before `--accept-charge`, query live account pricing with `cheep account pricing` or
`cheep api users get-pricing`. Some products and premium domains require extra price parameters
defined by Namecheap. The generic API surface sends the exact parameters you provide and does not
invent or silently change them.

`--readonly` refuses every method classified as a mutation, including when all confirmation flags
are present.

## Generic response format

Namecheap returns a different XML structure for each method. Cheep preserves that structure as a
stable JSON element tree instead of guessing field types:

```json
{
  "method": "namecheap.domains.getContacts",
  "status": "OK",
  "response": {
    "name": "CommandResponse",
    "attributes": {
      "Type": "namecheap.domains.getContacts"
    },
    "children": []
  }
}
```

Each element has a name and may have attributes, text, or ordered children. Cheep converts API
errors into its normal stable error envelope. If a mutation loses its response after transmission,
Cheep returns `api_outcome_unknown` and does not retry the request.

## Direct method lookup

Every catalog method has a named command, and `api call` accepts either its exact Namecheap name or
slash-separated CLI path:

```bash
cheep api call namecheap.domains.getInfo --param DomainName=example.com
cheep api call domains/get-info --param DomainName=example.com
```

`api call` is still catalog-bound. A misspelled or unknown method fails before any network request.
