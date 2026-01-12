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
4. **Categorize each package** using the decision tree below

#### Package Classification Decision Tree

For EACH dependency, follow these steps IN ORDER:

```text
1. Is the package in the SDK manifest?
   ├─ YES → Is the version compatible with the manifest's version range?
   │        ├─ YES → SUPPORTED (the SDK instruments this package)
   │        └─ NO  → UNSUPPORTED (version mismatch - SDK can't instrument this version)
   │
   └─ NO  → Is this a HIGH-RISK package? (see table below)
            ├─ YES → UNSUPPORTED (requires instrumentation but SDK doesn't support it)
            │        This includes: databases, caches, message queues, gRPC
            │        Also includes ODMs/ORMs that wrap these (e.g., mongoengine wraps pymongo)
            │
            └─ NO  → UNKNOWN (low-risk, doesn't affect compatibility)
                     These are safe because either:
                     - They use HTTP under the hood (auto-captured)
                     - They're utility libraries with no I/O
```

**IMPORTANT**: Web frameworks (flask, fastapi, express, etc.) ARE in the manifest and should be classified as SUPPORTED, not UNKNOWN.

#### High-Risk Packages (require explicit SDK instrumentation)

These packages use custom wire protocols that the SDK must explicitly instrument. If a package in this category is NOT in the manifest (or version doesn't match), it is **UNSUPPORTED**.

| Category | Node.js Packages | Python Packages |
|----------|------------------|-----------------|
| **SQL Databases** | pg, mysql2, better-sqlite3, tedious, oracledb | psycopg2, psycopg, pymysql, sqlite3, cx_Oracle |
| **NoSQL Databases** | mongodb, mongoose | pymongo, motor, mongoengine, odmantic |
| **Cache/Key-Value** | ioredis, redis | redis, aioredis |
| **Message Queues** | kafkajs, amqplib, bullmq, rhea | kafka-python, pika, celery, kombu |
| **gRPC** | @grpc/grpc-js, grpc | grpcio, grpclib |
| **Elasticsearch** | @elastic/elasticsearch | elasticsearch, opensearch-py |

#### Low-Risk Packages (safe as UNKNOWN)

These packages are safe even if not in the manifest:

- **HTTP-based API SDKs**: stripe, twilio, boto3, sendgrid, etc. (use HTTP under the hood → auto-captured)
- **Utility libraries**: lodash, numpy, pandas, etc. (no network I/O)
- **Dev dependencies**: pytest, eslint, typescript, etc. (not runtime dependencies)

### Response Schema

The eligibility report must conform to this structure:

```typescript
interface PackageInfo {
  packages: string[];    // e.g., ["pg@8.11.0", "axios@1.6.0"]
  reasoning: string;     // REQUIRED - must explain WHY packages are in this category
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
- **partially_compatible**: Runtime is nodejs/python AND has some unsupported packages, but they are used by only a subset of endpoints
- **not_compatible**: One of the following:
  - Runtime is "other" (Go, Java, Ruby, Rust, etc. are not yet supported)
  - Runtime is nodejs/python BUT has **critical** unsupported packages that the majority of endpoints depend on (e.g., the primary database driver for a service that stores all data in MongoDB)

#### Critical vs Non-Critical Unsupported Packages

When determining status, consider whether unsupported packages are **critical** to the service:

- **Critical**: The service's core functionality depends on this package. Most/all endpoints would fail without it.
  - Example: A service using MongoDB as its only database → `pymongo` is critical
  - Example: A service where every request writes to Redis for session management → `redis` is critical

- **Non-Critical**: Only some endpoints use this package; others work fine without it.
  - Example: A service with 10 endpoints where only 2 use Redis for caching → `redis` is non-critical
  - Example: A service that uses Kafka for async events but has synchronous endpoints that work independently

If ALL unsupported packages are non-critical → **partially_compatible**
If ANY unsupported package is critical → **not_compatible**

### Output Format

Call `transition_phase` with:

```json
{
  "results": {
    "eligibility_report": {
      "services": {
        "./backend": {
          "status": "compatible",
          "status_reasoning": "Node.js service with Express framework. All high-risk dependencies (pg) are in the SDK manifest.",
          "runtime": "nodejs",
          "framework": "express",
          "supported_packages": {
            "packages": ["express@4.18.0", "pg@8.11.0"],
            "reasoning": "express (web framework) and pg (SQL database driver) are both in the Node.js SDK manifest with compatible versions"
          },
          "unknown_packages": {
            "packages": ["axios@1.6.0", "lodash@4.17.21"],
            "reasoning": "axios uses HTTP under the hood (auto-captured); lodash is a utility library with no I/O"
          }
        },
        "./services/auth": {
          "status": "partially_compatible",
          "status_reasoning": "Python service with FastAPI. Redis is unsupported but only used for optional caching in minority of endpoints",
          "runtime": "python",
          "framework": "fastapi",
          "supported_packages": {
            "packages": ["fastapi==0.109.0", "requests==2.31.0", "psycopg2==2.9.9"],
            "reasoning": "fastapi (web framework), requests (HTTP client), and psycopg2 (PostgreSQL driver) are in the Python SDK manifest"
          },
          "unsupported_packages": {
            "packages": ["redis==5.0.0"],
            "reasoning": "redis is a high-risk cache driver not in the SDK manifest, but only used for optional caching (non-critical)"
          },
          "unknown_packages": {
            "packages": ["boto3==1.34.0", "pydantic==2.5.0"],
            "reasoning": "boto3 uses HTTP under the hood (auto-captured); pydantic is a validation library with no I/O"
          }
        },
        "./services/users": {
          "status": "not_compatible",
          "status_reasoning": "Python service with Flask. MongoDB (pymongo) is the primary database for all user data - every endpoint depends on it, making it critical. The SDK cannot capture these database calls.",
          "runtime": "python",
          "framework": "flask",
          "supported_packages": {
            "packages": ["flask==2.3.0", "requests==2.31.0"],
            "reasoning": "flask (web framework) and requests (HTTP client) are in the Python SDK manifest"
          },
          "unsupported_packages": {
            "packages": ["pymongo==4.6.0", "mongoengine==0.27.0"],
            "reasoning": "pymongo is the primary database driver (CRITICAL - all endpoints depend on it); mongoengine is an ODM that wraps pymongo"
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
        "total_services": 4,
        "compatible": 1,
        "partially_compatible": 1,
        "not_compatible": 2
      }
    }
  }
}
```

### Important

- Service paths should be relative to the working directory (e.g., `./backend`, `./services/api`)
- Summary counts MUST match the actual services in the report
- If no services are found, return an empty services map
