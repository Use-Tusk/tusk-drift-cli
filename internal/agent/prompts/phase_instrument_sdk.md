## Phase: Instrument SDK

Install the Tusk Drift SDK and instrument the application.

### Step 1: Install SDK (if not already installed)

First check if @use-tusk/drift-node-sdk is already in package.json dependencies.
If NOT installed, run the install command based on the package manager:

- npm: npm install @use-tusk/drift-node-sdk
- yarn: yarn add @use-tusk/drift-node-sdk
- pnpm: pnpm add @use-tusk/drift-node-sdk

Skip installation if the SDK is already in dependencies.

### Step 2: Create SDK Initialization File

Create a file called tuskDriftInit.ts (or .js) in the same directory as your entry point.
IMPORTANT: All code files must end with a trailing newline.

NOTE: This is LOCAL setup - do NOT use any API keys. Leave apiKey undefined for local mode.

**For CommonJS (module_system = "cjs"):**

```typescript
import { TuskDrift } from "@use-tusk/drift-node-sdk";

TuskDrift.initialize({
  env: process.env.NODE_ENV,
});

export { TuskDrift };
```

**For ESM (module_system = "esm"):**

```typescript
import { register } from "node:module";
import { pathToFileURL } from "node:url";

// Register the ESM loader - MUST be before importing the SDK
register("@use-tusk/drift-node-sdk/hook.mjs", pathToFileURL("./"));

import { TuskDrift } from "@use-tusk/drift-node-sdk";

TuskDrift.initialize({
  env: process.env.NODE_ENV,
});

export { TuskDrift };
```

### Step 3: Import SDK at Entry Point

**For CommonJS:** Add as the FIRST import in the entry file:

```typescript
import { TuskDrift } from "./tuskDriftInit";
// ... rest of imports
```

**For ESM:** Modify the start script in package.json to use --import flag:

```json
"start": "node --import ./dist/tuskDriftInit.js dist/server.js"
```

(Adjust paths based on the actual compiled output location)

### Step 4: Mark App as Ready

Find where the app finishes initialization (usually .listen() callback) and add:

```typescript
TuskDrift.markAppAsReady();
```

When done, call transition_phase with:
{
  "results": {
    "sdk_installed": true,
    "sdk_instrumented": true
  }
}
