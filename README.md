![Tusk CLI Banner](assets/tusk-banner.png)

<div align="center">

![GitHub Release](https://img.shields.io/github/v/release/Use-Tusk/tusk-drift-cli)
[![Build and test](https://github.com/Use-Tusk/tusk-drift-cli/actions/workflows/main.yml/badge.svg?branch=main)](https://github.com/Use-Tusk/tusk-drift-cli/actions/workflows/main.yml)
[![OpenSSF Best Practices](https://www.bestpractices.dev/projects/11340/badge)](https://www.bestpractices.dev/projects/11340)
[![OpenSSF Scorecard](https://api.securityscorecards.dev/projects/github.com/Use-Tusk/tusk-drift-cli/badge)](https://securityscorecards.dev/viewer/?uri=github.com/Use-Tusk/tusk-drift-cli)
<br>
[![Go Report Card](https://goreportcard.com/badge/github.com/Use-Tusk/tusk-drift-cli)](https://goreportcard.com/report/github.com/Use-Tusk/tusk-drift-cli)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)
[![X URL](https://img.shields.io/twitter/url?url=https%3A%2F%2Fx.com%2Fusetusk&style=flat&logo=x&label=Tusk&color=BF40BF)](https://x.com/usetusk)
[![Slack URL](https://img.shields.io/badge/slack-badge?style=flat&logo=slack&label=Tusk&color=BF40BF)](https://join.slack.com/t/tusk-community/shared_invite/zt-3fve1s7ie-NAAUn~UpHsf1m_2tdoGjsQ)

</div>

The Tusk CLI provides tools for automated testing workflows. It supports two products:

- **[Tusk Drift](docs/drift/)** - Live traffic record/replay as API tests
- **[Tusk Unit](docs/unit/)** - Automated unit tests

## Tusk Drift — API Record/Replay Testing

Record real API traffic and replay it as deterministic tests. Works locally and in CI/CD with Tusk Drift Cloud.

```bash
tusk drift setup          # AI setup agent
tusk drift run            # Replay recorded traces
tusk drift run --cloud    # Run against Tusk Drift Cloud
```

[Get started with Tusk Drift →](docs/drift/)

## Tusk Unit — Automated Unit Tests

View, review, and apply unit tests from Tusk directly from your terminal or automation pipeline.

```bash
tusk unit latest-run                    # Latest run for current branch
tusk unit get-run <run-id>              # Full run details
tusk unit get-diffs <run-id>            # Get diffs to apply
```

[Get started with Tusk Unit →](docs/unit/)

## Install

### Quick install (recommended)

**Linux/macOS:**

Install the latest version:

```bash
curl -fsSL https://cli.usetusk.ai/install.sh | sh
```

To install a specific version:

```bash
curl -fsSL https://cli.usetusk.ai/install.sh | sh -s -- v1.2.3
```

Linux additional dependencies (for replay sandboxing):

- Debian/Ubuntu: `sudo apt install bubblewrap socat`
- Fedora/RHEL: `sudo dnf install bubblewrap socat`
- Arch: `sudo pacman -S bubblewrap socat`

Without these, sandboxing is disabled and replays run without network isolation. See [Architecture - Sandboxing](docs/drift/architecture.md#sandboxing-with-fence).

**Homebrew:**

```bash
brew tap use-tusk/tap
brew install use-tusk/tap/tusk
```

To update:

```bash
brew upgrade use-tusk/tap/tusk
```

**Windows:**

We recommend using [WSL](https://learn.microsoft.com/en-us/windows/wsl/install) for the best experience on Windows. With WSL, you can use the Linux/macOS installation steps above and avoid Windows-specific configuration. For native Windows installation without WSL, expand below to see the steps.

<details>
<summary><b>Installation steps</b></summary>
Download the latest release from [GitHub Releases](https://github.com/Use-Tusk/tusk-drift-cli/releases/latest):

1. Download `tusk-drift-cli_*_Windows_x86_64.zip` (or `arm64` for ARM-based Windows)
2. Extract the ZIP file
3. Move `tusk.exe` to a directory in your PATH (e.g., `C:\tools\`), or add the extracted directory to your PATH:

   **Option A: Add to PATH via PowerShell (run as Administrator):**

   ```powershell
   [Environment]::SetEnvironmentVariable("Path", $env:Path + ";C:\path\to\tusk", "User")
   ```

   **Option B: Add to PATH via System Settings:**
   - Press `Win + R`, type `sysdm.cpl`, press Enter
   - Click "Environment Variables"
   - Under "User variables", select `Path` and click "Edit"
   - Click "New" and add the folder containing `tusk.exe`
   - Click OK to save

4. Restart your terminal and verify:

   ```powershell
   tusk --version
   ```

Note: Windows requires additional configuration for running tests. See [Windows Support](docs/drift/configuration.md#windows-support) for details on TCP communication mode setup.
</details>

### Manual Download

Download pre-built binaries from [GitHub Releases](https://github.com/Use-Tusk/tusk-drift-cli/releases/latest).

### Build from source

```bash
# Go 1.25+
git clone https://github.com/Use-Tusk/tusk-drift-cli.git
cd tusk-drift-cli
make deps
make build

tusk --help
```

## Community

Join our open source community on [Slack](https://join.slack.com/t/tusk-community/shared_invite/zt-3fve1s7ie-NAAUn~UpHsf1m_2tdoGjsQ).

## Development

See [`CONTRIBUTING.md`](./CONTRIBUTING.md).

## License

See [`LICENSE`](./LICENSE).
