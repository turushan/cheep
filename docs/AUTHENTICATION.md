# Authentication

Namecheap's legacy API requires an API user, username, API key, and one whitelisted public IPv4.
Sandbox and production are separate accounts with separate keys and IP allowlists.

## Configure sandbox

Enable API access in the Namecheap sandbox and whitelist the public IPv4 from which Cheep will
run. Then save the non-secret profile fields:

```bash
cheep --environment sandbox auth configure sandbox \
  --api-user YOUR_SANDBOX_USER \
  --client-ip YOUR_PUBLIC_IPV4 \
  --default
```

The username defaults to the API user. Supply `--username` only for a reseller setup that acts as a
different Namecheap username.

Store the API key through the hidden terminal prompt:

```bash
cheep --profile sandbox auth set-key
```

For CI or another headless environment, use `CHEEP_API_KEY`. To store a piped key in the local
keychain, use `auth set-key --stdin`. Never place an API key directly in command arguments.

## Verify access

```bash
cheep --profile sandbox auth status
cheep --profile sandbox doctor
```

`auth status` checks local resolution and never prints the key. `doctor` performs one authenticated,
read-only domain-list request. A `1011150` result means the request IPv4 is not in the selected
Namecheap environment's allowlist.

## Environment variables

Cheep supports these variables for headless use:

```text
CHEEP_PROFILE
CHEEP_ENVIRONMENT
CHEEP_API_USER
CHEEP_USERNAME
CHEEP_API_KEY
CHEEP_CLIENT_IP
CHEEP_CONFIG
```

Compatible `NAMECHEAP_*` variables are also recognized. `CHEEP_*` takes precedence.

## Sources

- [Namecheap API introduction](https://www.namecheap.com/support/api/intro/)
- [Namecheap API error codes](https://www.namecheap.com/support/api/error-codes/)
- [Namecheap API FAQ](https://www.namecheap.com/support/knowledgebase/article.aspx/9739/63/api-faq/)
