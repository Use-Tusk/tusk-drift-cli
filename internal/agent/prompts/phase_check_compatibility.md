## Phase: Check Compatibility

Verify the project's dependencies are compatible with the Tusk Drift SDK.

### Context

The SDK manifest has been loaded and is available in the current state. It contains:

- `sdkVersion`: The SDK version
- `instrumentations`: Array of `{ packageName, supportedVersions }`

### Understanding Instrumentation

The SDK captures outbound requests during recording and mocks them during replay. The key question: **which packages make outbound calls using protocols the SDK can't intercept?**

#### Low risk: HTTP-based packages

These typically use Node's http/https under the hood, which the SDK instruments:

- HTTP clients: axios, got, node-fetch, superagent, request
- API SDKs: aws-sdk, @aws-sdk/*, stripe, twilio, sendgrid
- REST clients: Most npm packages that call external APIs

These are generally safe even if not explicitly in the manifest.

#### High risk: Custom protocol packages

These use their own wire protocols and MUST be explicitly instrumented:

| Category | Examples |
|----------|----------|
| SQL Databases | pg, mysql2, postgres, mysql, mariadb, better-sqlite3 |
| NoSQL Databases | mongodb, mongoose, cassandra-driver, couchbase, @elastic/elasticsearch |
| Cache | ioredis, redis, memcached |
| Message Queues | kafkajs, amqplib, bullmq, @aws-sdk/client-sqs |
| gRPC | @grpc/grpc-js |

### Instructions

1. Read `package.json` to get the project's dependencies
2. For each dependency in a "high risk" category above:
   - Check if it's in the manifest's `instrumentations` array
   - If present, verify the version matches `supportedVersions` (use semver: `8.*` means `8.0.0` - `8.x.x`)
   - If NOT in manifest OR version doesn't match → flag as compatibility issue

### Decision

**All compatible:**
Call `transition_phase` with:

```json
{
  "results": {
    "compatibility_warnings": []
  }
}
```

**Issues found:**

1. Explain which packages are unsupported and why it matters
2. Use `ask_user` tool to confirm if they want to proceed:

```json
{
  "question": "Found compatibility issues:\n\n- mongodb@6.3.0: Not instrumented. Database queries won't be recorded/replayed.\n- kafkajs@2.2.0: Not instrumented. Kafka messages won't be captured.\n\nThe SDK will still capture HTTP traffic, but these calls will hit real services during replay.\n\nWould you like to proceed anyway? (yes/no)"
}
```

3. If user responds "yes", call `transition_phase` with warnings:

```json
{
  "results": {
    "compatibility_warnings": ["mongodb@6.3.0 not instrumented", "kafkajs@2.2.0 not instrumented"]
  }
}
```

4. If user responds "no", call `abort_setup`:

```json
{
  "reason": "User chose not to proceed due to unsupported packages: mongodb, kafkajs"
}
```
