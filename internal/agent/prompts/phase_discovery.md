## Phase: Discovery

Your first goal is to detect the project's **language/runtime**. Check for:

- **Node.js**: package.json
- **Python**: requirements.txt, pyproject.toml, setup.py, Pipfile
- **Go**: go.mod
- **Java**: pom.xml, build.gradle
- **Ruby**: Gemfile
- **Rust**: Cargo.toml
- **Other**: Look for common project markers

### If NOT Node.js

Currently, only Node.js is supported. If you detect a different language/runtime:

1. Call `abort_setup` with a clear message explaining:
   - What language/runtime was detected
   - That Tusk Drift currently only supports Node.js
   - That support for more languages is coming soon

Example:

```json
{
  "reason": "Detected Python project (found requirements.txt). Tusk Drift currently only supports Node.js services. Support for Python is coming soon."
}
```

### If Node.js

#### Step 1: Fetch SDK Manifest

Use `fetch_sdk_manifest` to get the list of instrumented packages:

```json
{
  "url": "https://unpkg.com/@use-tusk/drift-node-sdk@latest/dist/instrumentation-manifest.json"
}
```

The manifest contains:

- `sdkVersion`: The SDK version
- `instrumentations`: Array of `{ packageName, supportedVersions }`

#### Step 2: Check Project Dependencies

Read `package.json` and compare the project's dependencies against the manifest's `instrumentations` array. Note which instrumented packages the project uses (e.g., pg, ioredis, mysql2, graphql).

If the project has **no instrumented packages** (no database clients, no HTTP libraries from the list), warn the user but continue - they may still want to record HTTP traffic.

#### Step 3: Gather Project Information

1. **Package manager**: Check for package-lock.json (npm), yarn.lock (yarn), or pnpm-lock.yaml (pnpm)
2. **Module system**: Check package.json for "type": "module" (ESM) vs CommonJS (no type field or "type": "commonjs")
3. **Entry point**: Find the main server file (server.ts, index.ts, app.ts, main.ts) - look at package.json "main" or "scripts.start"
4. **Start command**: How to start the service (e.g., npm run start, npm run dev)
5. **Port**: Look for PORT env var usage or hardcoded port in the entry file
6. **Health endpoint**: Check for /health, /healthz, /liveness routes
7. **Docker**: Check for Dockerfile or docker-compose.yml
8. **Service name**: Infer from package.json name or directory name

If this appears to be a monorepo or you're not sure you're at the service root, use ask_user to confirm.

If this looks like a simple demo/toy project, confirm the user wants to proceed.

If there are multiple plausible start commands (e.g., "npm run start", "npm run dev", "npm run dev:watch"), ask the user to choose the correct one.

Note: "docker_type" must be "none" if the service can be started without Docker (e.g., "npm run start"), even if the service has a Dockerfile or docker-compose.yml.

When you have gathered this information, call transition_phase with all the results:

```json
{
  "results": {
    "project_type": "nodejs",
    "package_manager": "npm|yarn|pnpm",
    "module_system": "esm|cjs",
    "entry_point": "src/server.ts",
    "start_command": "npm run start",
    "port": "3000",
    "health_endpoint": "/health",
    "docker_type": "none|dockerfile|compose",
    "service_name": "my-service"
  }
}
```
