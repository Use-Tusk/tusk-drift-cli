# Filtering Tests

Both `tusk drift run` and `tusk drift list` accept `--filter` to narrow which tests are shown or executed.

## Syntax

You can use fielded filters: `key=regex[,key=regex...]` (AND semantics across keys). Values are Go regexes; wrap in quotes if needed.

Keys (case-insensitive; aliases in parentheses):

- `path` (`p`)
- `name` (`display`, `display_name`, `n`) – human-friendly display (e.g., `query GetUser`)
- `op` (`operation`, `operation_name`, `graphql_op`) – GraphQL operation name only (e.g., `GetUser`)
- `type` (`t`) – display type like `HTTP`, `GRAPHQL`, `GRPC`, etc.
- `method` (`m`) – HTTP method
- `status` (`s`) – test status label for display (e.g., `success`, `error`)
- `id` (`trace`, `trace_id`) – trace ID
- `file` (`filename`, `f`) – source file name
- `suite_status` (`suite`) – cloud suite status: `draft` or `in_suite` (exact values only, not regex)

Notes:

- Commas separate conditions; all conditions must match (logical AND).
- Use quotes around regex values if they contain spaces or shell metacharacters.

## Examples

GraphQL:

- By operation name (recommended):  
  `tusk drift list -f 'type=GRAPHQL,op=^GetUser$'`  
  `tusk drift run  -f 'type=GRAPHQL,op=Get(User|Resources)'`
- By display text (includes the verb):  
  `tusk drift run -f 'name="^query\\s+GetUser$"'`

HTTP:

- By route: `tusk drift run -f 'path=^/api/orders(/|$)'`
- By method + route: `tusk drift run -f 'method=POST,path=/checkout'`
- By type: `tusk drift list -f 'type=HTTP'`

Suite status (cloud only):

- Draft tests only: `tusk drift run --cloud -f 'suite_status=draft'`
- In-suite tests only: `tusk drift run --cloud -f 'suite_status=in_suite'`

Trace/file:

- Specific trace: `tusk drift run -f 'id=84d0de6b4e4498e996c7f8b8c0f35230'`
- By file: `tusk drift list -f 'file=2025-09-24.*trace.*\\.jsonl'`

Regex tips:

- Anchor with `^` and `$` for exact matches.
- Use `|` for OR (e.g., `name="Get(User|Resources)"`).
- Prefer single quotes around the entire filter to avoid shell escaping issues.

Behavior:

- If the filter yields 0 tests, `tusk drift run` exits gracefully without starting your service.
