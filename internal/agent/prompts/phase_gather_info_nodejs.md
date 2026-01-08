## Phase: Gather Project Information (Node.js)

Collect the details needed to set up Tusk Drift for this Node.js project.

### Required Information

| Field | How to find | Example |
|-------|-------------|---------|
| package_manager | Lockfile: package-lock.json → npm, yarn.lock → yarn, pnpm-lock.yaml → pnpm | "npm" |
| module_system | package.json "type": "module" → ESM, otherwise → CJS | "esm" |
| entry_point | package.json "main" or parse "scripts.start" command | "src/server.ts" |
| start_command | package.json "scripts" - prefer "start" or "dev" | "npm run start" |
| port | Look for PORT env var or hardcoded port in entry file | "3000" |
| health_endpoint | Grep for /health, /healthz, /liveness routes | "/health" |
| docker_type | Check for Dockerfile or docker-compose.yml | "none" |
| service_name | package.json "name" or directory name | "my-service" |
| framework | Check dependencies: next, express, fastify, etc. | "next" |

### Instructions

1. **Package Manager**: Check which lockfile exists
2. **Module System**: Read package.json and check "type" field
3. **Entry Point**:
   - Check package.json "main" field
   - Or parse the start script to find the entry file
4. **Start Command**:
   - If multiple options (start, dev, dev:watch), ask user to choose
   - Prefer "start" for production-like testing
5. **Port**:
   - Grep for `PORT` or `process.env.PORT`
   - Check entry file for hardcoded port
6. **Health Endpoint**:
   - Grep for common patterns: `/health`, `/healthz`, `/liveness`, `/ready`
   - If not found, note as empty string
7. **Docker**:
   - Set to "dockerfile" if Dockerfile exists
   - Set to "compose" if docker-compose.yml exists  
   - Set to "none" if service can run without Docker (even if Dockerfile exists)
8. **Service Name**: Use package.json "name" or fall back to directory name
9. **Framework**: Check package.json dependencies to detect:
   - Next.js: `next` in dependencies
   - Express: `express` in dependencies
   - Fastify: `fastify` in dependencies
   - Other/Generic: if none of the above match

### Edge Cases

- **Monorepo**: If you see a workspaces field or multiple package.json files, ask user to confirm the service root
- **Simple/Demo project**: If package.json has few dependencies, confirm user wants to proceed
- **TypeScript**: Note if tsconfig.json exists (affects entry point paths)

### Transition

Call `transition_phase` with all gathered information:

```json
{
  "results": {
    "package_manager": "npm",
    "module_system": "esm",
    "entry_point": "src/server.ts",
    "start_command": "npm run start",
    "port": "3000",
    "health_endpoint": "/health",
    "docker_type": "none",
    "service_name": "my-service",
    "framework": "express"
  }
}
```
