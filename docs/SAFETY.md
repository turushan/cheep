# Safety contract

## Environments

Sandbox is the default. Production must be explicit in the selected profile or environment. Every
mutation prints `SANDBOX` or `PRODUCTION` to stderr before confirmation or application.

The presence of production credentials does not authorize production use.

## Money

Registration, renewal, transfer, reactivation, privacy, and certificate purchases require:

- A fresh exact price from Namecheap
- A matching `--max-price`
- Exact target confirmation
- A visible environment label

NC CLI never retries an ambiguous charge-bearing request.

## DNS

Namecheap's host update endpoint replaces the complete zone. NC CLI therefore reads the current
zone, writes a local snapshot, calculates a record-level plan, checks for concurrent changes,
applies the complete intended zone once, and reads it back for verification.

An empty selector never means every record. A plan that would empty a zone requires a separate,
specific confirmation.

## Secrets

API keys use the operating-system keychain by default. Environment variables support headless
automation. Secrets never appear in process arguments, URLs, output, debug logs, errors, snapshots,
or test fixtures.

