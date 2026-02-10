# AI-Powered Setup

`tusk setup` uses an AI agent to automatically analyze your codebase and set up Tusk Drift for you.

## What the agent does

1. Discover your project structure, language, and dependencies
2. Verify your service starts correctly
3. Instrument the Tusk Drift SDK into your application
4. Create configuration files (`.tusk/config.yaml`)
5. Test the setup with recording and replay

Full docs: <https://docs.usetusk.ai/api-tests/setup-agent>

## Requirements

- Supported languages: Python (FastAPI, Flask, Django, Starlette) and Node.js (Express, Fastify, Koa, Hapi)

## API Key Options

For convenience, by default the setup agent uses Tusk's servers as a proxy to the Anthropic API (no API key needed). Your data is never used for training, see privacy policy: usetusk.ai/privacy.

To use your own Anthropic API key instead:

- Set via `ANTHROPIC_API_KEY` environment variable, or
- Pass `--api-key` flag (requests go directly to Anthropic)

## Modes

- Interactive (default): Shows a TUI with real-time progress and allows you to approve/reject agent actions
- Headless (`--print`): Streams output to stdout, useful for automation or CI environments
- Eligibility check (`--eligibility-only`): Scans your directory for services and outputs a JSON report of which are eligible for SDK setup

## Progress & Resumption

The agent saves progress to `.tusk/setup/PROGRESS.md` by default. If setup is interrupted, running `tusk setup` again will resume from where it left off. Use `--disable-progress-state` to start fresh.

## Cloud Setup

After local setup completes, the agent can optionally configure Tusk Drift Cloud integration:

- Authenticate with Tusk Cloud
- Create/select a service in Tusk Cloud
- Generate API keys for CI/CD integration
