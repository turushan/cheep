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
| 6 | Concurrent change, state conflict, or a write whose outcome is unknown |
| 7 | Safety policy refused the operation |
| 8 | Charge-bearing method refused without `--accept-charge` |
| 130 | Interrupted by the operator |

## Non-interactive execution

`--no-input` forbids prompts. A command that needs confirmation must fail unless all explicit
confirmation flags are present. `--readonly` blocks every remote mutation. `--dry-run` calculates
and prints a plan without applying it. `--readonly` also refuses a `--dry-run` of a `cheep api`
mutation, but allows a `--dry-run` of `dns apply` or `dns restore`, which reads the live zone and
writes nothing.

Use `cheep api methods --json` to discover the complete Namecheap method catalog. Generic API
responses preserve provider XML as ordered element trees so automation never loses repeated child
elements or attributes. Use `--secret-param NAME=ENV_VAR` for sensitive method parameters.

## Experimental command inventory

`cheep schema --json` reports the current command and local-flag inventory as
`experimental-v0`. It is not the stable automation contract yet. Positional arguments, inherited
flags, output types, and safety requirements will be added before its first stable version.
