## Phase: Create Config

Create the .tusk/config.yaml file based on gathered information.

### For Non-Docker Services

```yaml
service:
  name: {service_name}
  port: {port}
  start:
    command: {start_command}
  readiness_check:
    command: curl -fsS http://localhost:{port}{health_endpoint}
    timeout: 30s
    interval: 1s

traces:
  dir: .tusk/traces

tusk_api:
  url: https://api.usetusk.ai

test_execution:
  timeout: 30s

recording:
  sampling_rate: 1.0
  export_spans: false
  enable_env_var_recording: true
```

### For Docker Compose Services

```yaml
service:
  name: {service_name}
  port: {port}
  start:
    command: docker compose -f docker-compose.yml -f docker-compose.tusk-override.yml up
  stop:
    command: docker compose down
  communication:
    type: tcp
    tcp_port: 9001
  readiness_check:
    command: curl -fsS http://localhost:{port}{health_endpoint}
    timeout: 30s
    interval: 1s

traces:
  dir: .tusk/traces

tusk_api:
  url: https://api.usetusk.ai

test_execution:
  timeout: 30s

recording:
  sampling_rate: 1.0
  export_spans: false
  enable_env_var_recording: true
```

Also create docker-compose.tusk-override.yml if using Docker Compose.

IMPORTANT:

- Always ensure config files end with a trailing newline.
- After creating the config file, ALWAYS call `tusk_validate_config` to verify the config is valid.
- If validation fails, check the error messages for unknown keys or missing required fields and fix them.

Common config mistakes to avoid:

- `start_command: "..."` should be `start: { command: "..." }` (nested structure)
- `readiness_command: "..."` should be `readiness_check: { command: "..." }` (nested structure)
- `port: 3000` at root level should be `service: { port: 3000 }` (under service section)

When done, call transition_phase with:
{
  "results": {
    "config_created": true
  }
}
