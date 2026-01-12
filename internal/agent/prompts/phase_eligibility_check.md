## Phase: Eligibility Check

Discover all backend services in the directory tree and check their SDK compatibility with Drift SDK

### Objectives

1. Discover all backend services (any depth)
2. For each service: detect runtime, check compatibility against SDK manifest
3. Output structured eligibility report

### Service Discovery

Scan the repository to discover all **backend API services**. A backend service is a deployable application that exposes HTTP/gRPC endpoints that external clients or other services consume.

**Your goal**: Find directories containing runnable server applications, NOT libraries/SDKs/packages meant to be imported by others.

#### What IS a Backend Service

- An application with an entry point that starts an HTTP/gRPC server
- Something you would deploy to production and send requests to
- Has route definitions, endpoint handlers, or controller classes
- May have a `Dockerfile`, deployment configs, or `main` entry point

#### What is NOT a Backend Service (EXCLUDE these)

- **SDKs/Client Libraries**: Packages meant to be `npm install`ed or `pip install`ed by consumers
- **Utility Libraries**: Shared code packages with no server/API functionality
- **CLI Tools**: Command-line applications that don't expose network APIs
- **Frontend Applications**: React, Vue, Angular apps (even if they have package.json)
- **Lambda/Serverless Functions**: Unless they clearly define HTTP endpoints
- **Test Directories**: Folders that only contain test code
- **Example/Demo Folders**: Sample code or documentation examples

#### Detection Strategy

1. **Start from the root** and explore the directory tree
2. **Look for server framework indicators** in dependency files
3. **Verify server behavior** by checking for:
   - Route/endpoint definitions (e.g., `app.get()`, `@app.route()`, `@Get()`)
   - Server startup code (e.g., `app.listen()`, `uvicorn.run()`, `http.ListenAndServe()`)
   - Controller/handler files
4. **Exclude if** the primary purpose is to be imported (look for `"main"` field pointing to lib entry, `exports` in package.json, or library-style structure)

#### Runtime-Specific Indicators

| Runtime | Service Indicator | Exclude If |
|---------|-------------------|------------|
| **Node.js** | `package.json` with express, fastify, hono, koa, nest, hapi + route handlers | `"main": "lib/index.js"`, has `"exports"` field, or name suggests SDK (`*-sdk`, `*-client`) |
| **Python** | `pyproject.toml`/`requirements.txt` with fastapi, flask, django, starlette, sanic + app entry | `setup.py` with `packages=`, or structured as importable module without server entry |
| **Other** | Go (`go.mod`), Java (`pom.xml`/`build.gradle`), Ruby (`Gemfile`), Rust (`Cargo.toml`) with web frameworks | N/A - these are not yet supported |

### Compatibility Check

For each discovered service:

1. **Determine runtime** from markers (nodejs, python, or other)
2. **Get SDK manifest** for that runtime (provided in context)
3. **Read dependencies** (package.json, requirements.txt, etc.)
4. **Categorize packages**:
   - **Supported**: In SDK manifest with matching version range
   - **Unsupported**: In high-risk category (see below) but NOT in manifest or version mismatch
   - **Unknown**: Not in manifest and not in high-risk category

#### Low-Risk Packages (HTTP-based)

The SDKs instrument all major HTTP client libraries (Node.js: `http`/`https` modules, `axios`, `fetch`; Python: `requests`, `httpx`, `urllib3`, `aiohttp`).

Any third-party packages that make HTTP calls under the hood—such as API SDKs (Stripe, Twilio, AWS SDK, etc.)—are generally safe because their HTTP traffic will be captured automatically.

**These should be categorized as `unknown_packages` (not `unsupported`)**, even if not explicitly in the manifest. The SDKs will capture their network calls.

Only packages using **custom wire protocols** (databases, caches, message queues, gRPC) require explicit instrumentation and should be checked against the SDK manifests.

#### High-Risk Categories (require explicit instrumentation)

| Category | Node.js | Python |
|----------|---------|--------|
| SQL DB | pg, mysql2, better-sqlite3 | psycopg2, pymysql, sqlite3 |
| NoSQL DB | mongodb, mongoose | pymongo, motor |
| Cache | ioredis, redis | redis, aioredis |
| Queue | kafkajs, amqplib, bullmq | kafka-python, pika, celery |
| gRPC | @grpc/grpc-js | grpcio |

### Response Schema

The eligibility report must conform to this structure:

```typescript
interface PackageInfo {
  packages: string[];    // e.g., ["pg@8.11.0", "axios@1.6.0"]
  reasoning: string;     // REQUIRED - explanation for categorization
}

interface ServiceEligibility {
  status: "compatible" | "partially_compatible" | "not_compatible";
  status_reasoning: string;  // REQUIRED - why this status was assigned
  runtime: "nodejs" | "python" | "other";
  framework?: string;        // e.g., "express", "fastapi", "gin"
  supported_packages?: PackageInfo;
  unsupported_packages?: PackageInfo;
  unknown_packages?: PackageInfo;
}

interface EligibilitySummary {
  total_services: number;        // Must equal Object.keys(services).length
  compatible: number;            // Must equal count where status === "compatible"
  partially_compatible: number;  // Must equal count where status === "partially_compatible"
  not_compatible: number;        // Must equal count where status === "not_compatible"
}

interface EligibilityReport {
  services: {
    [path: string]: ServiceEligibility;  // path is relative, e.g., "./backend"
  };
  summary: EligibilitySummary;
}
```

### Status Determination

- **compatible**: Runtime is nodejs/python AND no unsupported packages
- **partially_compatible**: Runtime is nodejs/python AND has some unsupported packages
- **not_compatible**: Runtime is "other" (Go, Java, Ruby, Rust, etc. are not yet supported)

### Output Format

Call `transition_phase` with:

```json
{
  "results": {
    "eligibility_report": {
      "services": {
        "./backend": {
          "status": "compatible",
          "status_reasoning": "Node.js service with Express framework. All dependencies are supported by the SDK.",
          "runtime": "nodejs",
          "framework": "express",
          "supported_packages": {
            "packages": ["pg@8.11.0"],
            "reasoning": "pg is in manifest with version 8.*"
          },
          "unknown_packages": {
            "packages": ["axios@1.6.0", "lodash@4.17.21"],
            "reasoning": "axios uses HTTP under the hood (auto-captured), lodash is a utility library"
          }
        },
        "./services/auth": {
          "status": "partially_compatible",
          "status_reasoning": "Python service with FastAPI. Some dependencies are not instrumented by the SDK.",
          "runtime": "python",
          "framework": "fastapi",
          "supported_packages": {
            "packages": ["httpx==0.27.0"],
            "reasoning": "In manifest with version 0.27.*"
          },
          "unsupported_packages": {
            "packages": ["redis==5.0.0"],
            "reasoning": "Redis 5.x not in Python SDK manifest, only 4.x supported"
          }
        },
        "./services/billing": {
          "status": "not_compatible",
          "status_reasoning": "Go is not currently supported by the Tusk Drift SDK.",
          "runtime": "other",
          "framework": "gin"
        }
      },
      "summary": {
        "total_services": 3,
        "compatible": 1,
        "partially_compatible": 1,
        "not_compatible": 1
      }
    }
  }
}
```

### Important

- Service paths should be relative to the working directory (e.g., `./backend`, `./services/api`)
- Summary counts MUST match the actual services in the report
- If no services are found, return an empty services map
