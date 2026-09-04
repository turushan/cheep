# Automation contract

## Streams

Successful command data goes to stdout. Errors, warnings, prompts, and progress go to stderr. A
successful JSON command writes no explanatory prose to stdout.

## JSON

`--json` returns an envelope with these stable top-level fields:

```json
{
  "schema_version": "v1",
  "command": "version",
  "ok": true,
  "data": {}
}
```

Errors use the same envelope on stderr and add an `error` object. Removing or changing the meaning
of a versioned field requires a new schema version.

## Exit codes

| Code | Meaning |
|---:|---|
| 0 | Success |
| 1 | Unexpected failure |
| 2 | Invalid command or arguments |
| 3 | Missing or invalid authentication |
| 4 | Network failure or timeout |
| 5 | Provider API failure |
| 6 | Concurrent change or state conflict |
| 7 | Safety policy refused the operation |
| 8 | Price or spending limit refused the operation |
| 130 | Interrupted by the operator |

## Non-interactive execution

`--no-input` forbids prompts. A command that needs confirmation must fail unless all explicit
confirmation flags are present. `--readonly` blocks every remote mutation. `--dry-run` calculates
and prints a plan without applying it.

## Experimental command inventory

`cheep schema --json` reports the current command and local-flag inventory as
`experimental-v0`. It is not the stable automation contract yet. Positional arguments, inherited
flags, output types, and safety requirements will be added before its first stable version.
