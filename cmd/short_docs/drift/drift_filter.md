## Filtering tests

Filter tests with `-f`/`--filter`.

Fields: `path=...,name=...,type=...,method=...,status=...,id=...,suite_status=...`.
Comma-separated, values are regex.

Use `suite_status` to filter cloud tests by suite status (`draft` or `in_suite`).
When `suite_status=draft` is set, draft tests are fetched directly from the backend.

Examples:

```bash
tusk drift <list/run> -f 'type=GRAPHQL,op=^GetUser$'
tusk drift <list/run> -f 'method=POST,path=/checkout'
tusk drift <list/run> -f 'file=2025-09-24.*trace.*\\.jsonl'
tusk drift run --cloud -f 'suite_status=draft'
```

See <https://github.com/Use-Tusk/tusk-cli/blob/main/docs/drift/filter.md> for more details.
