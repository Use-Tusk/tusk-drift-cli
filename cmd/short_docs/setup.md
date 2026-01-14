# AI-Powered Setup

`tusk setup` uses an AI agent to automatically analyze your codebase and set up Tusk Drift for you.

## What the agent does

1. Discover your project structure, language, and dependencies
2. Verify your service starts correctly
3. Instrument the Tusk Drift SDK into your application
4. Create configuration files (`.tusk/config.yaml`)
5. Test the setup with recording and replay

## Requirements

- Anthropic API key: Set via `ANTHROPIC_API_KEY` environment variable or `--api-key` flag
- Supported languages: Python (FastAPI, Flask, Django, Starlette) and Node.js (Express, Fastify, Koa, Hapi)

## Modes

- Interactive (default): Shows a TUI with real-time progress and allows you to approve/reject agent actions
- Headless (`--print`): Streams output to stdout, useful for automation or CI environments
- Eligibility check (`--eligibility-only`): Scans your directory for services and outputs a JSON report of which are eligible for SDK setup

## Progress & Resumption

The agent saves progress to `PROGRESS.md` by default. If setup is interrupted, running `tusk setup` again will resume from where it left off. Use `--disable-progress-state` to start fresh.

## Cloud Setup

After local setup completes, the agent can optionally configure Tusk Drift Cloud integration:

- Authenticate with Tusk Cloud
- Create/select a service in Tusk Cloud
- Generate API keys for CI/CD integration
