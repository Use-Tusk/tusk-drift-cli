## Filtering tests

Filter tests with `-f`/`--filter`.

Fields: `path=...,name=...,type=...,method=...,status=...,id=...`.
Comma-separated, values are regex.

Examples:

```bash
tusk drift <list/run> -f 'type=GRAPHQL,op=^GetUser$'
tusk drift <list/run> -f 'method=POST,path=/checkout'
tusk drift <list/run> -f 'file=2025-09-24.*trace.*\\.jsonl'
```

See <https://github.com/Use-Tusk/tusk-cli/blob/main/docs/drift/filter.md> for more details.
