## Phase: Detect Language

Identify the project's language/runtime by checking for common project markers.

### Detection Rules

| Language | Markers | SDK Available |
|----------|---------|---------------|
| Node.js | package.json | ✅ Yes |
| Python | requirements.txt, pyproject.toml, setup.py, Pipfile | ❌ Coming soon |
| Go | go.mod | ❌ Coming soon |
| Java | pom.xml, build.gradle | ❌ Coming soon |
| Ruby | Gemfile | ❌ Coming soon |
| Rust | Cargo.toml | ❌ Coming soon |

### Instructions

1. Use `list_directory` to check the project root for these markers
2. If multiple markers exist (e.g., package.json AND pyproject.toml), ask the user which is the primary project
3. If no markers found, ask the user what type of project this is

### Decision

**If Node.js detected:**
Call `transition_phase` with:

```json
{
  "results": {
    "project_type": "nodejs"
  }
}
```

**If Python detected:**
Call `transition_phase` with:

```json
{
  "results": {
    "project_type": "python"
  }
}
```

**If unsupported language detected:**
You must call `abort_setup` with a clear explanation and the detected project type:

```json
{
  "reason": "Detected Go project (found go.mod). Tusk Drift currently only supports Node.js and Python services. Support for Go is coming soon.",
  "project_type": "go"
}
```

Do NOT continue with setup for unsupported projects.

**If no project detected:**
Ask the user what type of project this is before proceeding.
