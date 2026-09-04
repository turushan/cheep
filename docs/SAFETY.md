# Safety contract

## Environments

Sandbox is the default. A production profile becomes the default only through `auth configure
--default`. Every production DNS mutation requires the separate `--production` gate, even when the
selected profile or environment override already resolves to production.

Human-mode mutation diagnostics print `SANDBOX` or `PRODUCTION` to stderr. JSON mode writes one
result document to stdout on success and one error document to stderr on failure.

The presence of production credentials does not authorize production use.

## Money

Registration, renewal, transfer, reactivation, privacy, and certificate purchases require:

- A fresh exact price from Namecheap
- A matching `--max-price`
- Exact target confirmation
- A visible environment label

Cheep never retries an ambiguous charge-bearing request.

## DNS

Namecheap's host update endpoint replaces the complete zone. Cheep therefore reads the current
zone, writes a local snapshot, calculates a record-level plan, checks for concurrent changes,
applies the complete intended zone once, and reads it back for verification.

Cheep never retries a DNS write whose outcome may be unknown. It names the pre-change snapshot
and requires the operator to read and reconcile the live zone before trying another change.

An empty selector never means every record. A plan that would empty a zone requires a separate,
specific confirmation.

## Secrets

API keys use the operating-system keychain by default. Environment variables support headless
automation. Secrets never appear in process arguments, URLs, output, debug logs, errors, snapshots,
or test fixtures.
