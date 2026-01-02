## Phase: Check Compatibility (Python)

Verify the project's dependencies are compatible with the Tusk Drift Python SDK.

### Context

The SDK manifest has been loaded and is available in the current state. It contains:

- `sdkVersion`: The SDK version
- `instrumentations`: Array of `{ packageName, supportedVersions }`

### Understanding Instrumentation

The SDK captures outbound requests during recording and mocks them during replay. The key question: **which packages make outbound calls using protocols the SDK can't intercept?**

#### Low risk: HTTP-based packages

These typically use Python's urllib or requests under the hood, which the SDK instruments:

- HTTP clients: requests, httpx, aiohttp, urllib3
- API SDKs: boto3, google-cloud-*, stripe, twilio
- REST clients: Most PyPI packages that call external APIs

These are generally safe even if not explicitly in the manifest.

#### High risk: Custom protocol packages

These use their own wire protocols and MUST be explicitly instrumented:

| Category | Examples |
|----------|----------|
| SQL Databases | psycopg2, psycopg, asyncpg, sqlalchemy (with db driver), pymysql, mysql-connector-python |
| NoSQL Databases | pymongo, motor, elasticsearch, cassandra-driver |
| Cache | redis, aioredis |
| Message Queues | pika (RabbitMQ), kafka-python, aiokafka, celery |
| gRPC | grpcio |

### Instructions

1. Read `requirements.txt`, `pyproject.toml`, or `Pipfile` to get the project's dependencies
2. For each dependency in a "high risk" category above:
   - Check if it's in the manifest's `instrumentations` array
   - If present, verify the version matches `supportedVersions` (use semver-like matching)
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
  "question": "Found compatibility issues:\n\n- pymongo@4.5.0: Not instrumented. Database queries won't be recorded/replayed.\n- kafka-python@2.0.2: Not instrumented. Kafka messages won't be captured.\n\nThe SDK will still capture HTTP traffic, but these calls will hit real services during replay.\n\nWould you like to proceed anyway? (yes/no)"
}
```

3. If user responds "yes", call `transition_phase` with warnings:

```json
{
  "results": {
    "compatibility_warnings": ["pymongo@4.5.0 not instrumented", "kafka-python@2.0.2 not instrumented"]
  }
}
```

4. If user responds "no", call `abort_setup`:

```json
{
  "reason": "User chose not to proceed due to unsupported packages: pymongo, kafka-python"
}
```
