## Filtering tests

Filter tests with `-f`/`--filter`.

Fields: `path=...,name=...,type=...,method=...,status=...,id=...`.
Comma-separated, values are regex.

Examples:

```bash
tusk <list/run> -f 'type=GRAPHQL,op=^GetUser$'
tusk <list/run> -f 'method=POST,path=/checkout'
tusk <list/run> -f 'file=2025-09-24.*trace.*\\.jsonl'
```

See <https://github.com/Use-Tusk/tusk-cli/blob/main/docs/drift/filter.md> for more details.
