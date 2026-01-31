## Phase: Verify Setup

Validate the existing Tusk Drift setup and prepare for verification testing.

### Step 1: Validate Config

Run `tusk_validate_config` to verify .tusk/config.yaml is valid.
If validation fails, report the error and call transition_phase with failure notes.

### Step 2: Read Current Recording Config

Read `.tusk/config.yaml` and extract the current values:
- Under `recording`: `sampling_rate`, `export_spans`, `enable_env_var_recording`
- Under `service`: `id` (if present)
- Under `service`: `port`
- Under `service.readiness_check`: extract the health endpoint URL if available
- Under `service.start`: `command`

Store the recording values as the originals to restore later.

### Step 3: Override Recording Config

Use `cloud_save_config` to set:
- `sampling_rate`: 1 (record everything for verification)
- `export_spans`: false (keep traces local during verification)
- `enable_env_var_recording`: keep whatever it currently is
- `service_id`: pass the existing service ID if one was found, or empty string if none

### Step 4: Delete Existing Traces

Run command: `rm -rf .tusk/traces/*`

This ensures we start with a clean slate for verification.

### Step 5: Check for Verify Cache

Read `.tusk/verify-cache.json` if it exists. If found, this contains cached endpoint
information from a previous successful verify or setup run.

Call transition_phase with:
```json
{
  "results": {
    "original_sampling_rate": <original value>,
    "original_export_spans": <original value>,
    "original_enable_env_var_recording": <original value>,
    "start_command": "<from config.yaml service.start.command>",
    "port": "<from config.yaml service.port>",
    "health_endpoint": "<from config.yaml or readiness check>"
  },
  "notes": "Setup validated. Config overridden for verification. Cache: <found|not found>"
}
```
