# DNS workflow

Namecheap's `setHosts` endpoint replaces the complete DNS zone. Cheep treats every write as a
whole-zone change even when only one record differs.

## Inspect and export

```bash
cheep --profile sandbox dns list example.com
cheep --profile sandbox dns export example.com --file example.com.yaml
```

The exported YAML contains only fields that Cheep can reason about safely. `namecheap_dns` records
the read-only provider state. Cheep reads the provider again before any write and does not trust
that saved value as authorization.

```yaml
version: 1
domain: example.com
email_type: NONE
namecheap_dns: true
records:
  - host: "@"
    type: A
    value: 192.0.2.10
    ttl: 300
  - host: www
    type: CNAME
    value: example.com
    ttl: 300
```

## Plan

```bash
cheep --profile sandbox dns plan example.com --file example.com.yaml
```

Planning performs one or more read-only requests. It prints records to add, remove, and keep. It
does not write to Namecheap.

The same safety path is available through apply dry-run mode:

```bash
cheep --profile sandbox --dry-run dns apply example.com --file example.com.yaml
```

## Apply

```bash
cheep --profile sandbox dns apply example.com \
  --file example.com.yaml \
  --confirm-domain example.com
```

Before the write, Cheep:

1. Validates the complete file and exact domain.
2. Reads the current zone, calculates a deterministic diff, and records its fingerprint.
3. Refuses an incompatible mail configuration.
4. Confirms that the domain currently uses Namecheap DNS.
5. Confirms that the snapshotted zone still has the planned fingerprint.
6. Writes a private, timestamped snapshot.
7. Confirms the fingerprint again inside the provider adapter.
8. Replaces the zone once through the official SDK without automatic retries.
9. Reads the zone again and verifies the complete result.

Namecheap does not offer a compare-and-swap operation, so a small race remains between the final
read and `setHosts`. If the response or verification is inconclusive, Cheep returns the stable
`dns_outcome_unknown` error. Do not retry it blindly. Run `cheep dns list`, inspect the named
snapshot, and reconcile the actual zone first.

An empty desired zone also requires `--allow-empty-zone`. `--readonly` blocks every apply. CAA
writes and OX or GMAIL mail zones remain disabled until their round-trip behavior is modeled and
verified.

Changing `email_type` requires a second exact gate such as `--confirm-email-type MX`. Every
production DNS mutation also requires `--production`, even when a production profile or an
environment override is already selected.

## Restore

A snapshot uses the same versioned YAML format as an export. Restoring follows the exact apply
safety path:

```bash
cheep --profile sandbox dns restore example.com \
  --file /path/from/the/previous-apply-output.yaml \
  --confirm-domain example.com
```

Cheep takes another snapshot before restoring, so the state being replaced remains recoverable.

## Source

[Namecheap's setHosts documentation](https://www.namecheap.com/support/api/methods/domains-dns/set-hosts/)
warns that omitted host records are deleted. Cheep's plan, snapshot, and verification steps exist
because of that behavior.
