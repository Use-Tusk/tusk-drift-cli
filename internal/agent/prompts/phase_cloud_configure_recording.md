## Phase: Configure Recording

Configure the recording parameters for Tusk Drift Cloud.

### Configuration Options

1. **Sampling Rate** (0.0 to 1.0):
   - Percentage of requests to record
   - Recommended: 0.1 (10%) for dev/staging, 0.01 (1%) for production
   - Default: 0.1

2. **Export Spans** (boolean):
   - Whether to upload trace data to Tusk Cloud
   - Required for cloud features
   - Default: true

3. **Record Environment Variables** (boolean):
   - Whether to record and replay environment variables
   - Recommended if app behavior depends on env vars
   - Default: false

### Steps

1. **Present defaults**: Tell the user the default configuration:
   - Sampling rate: 0.1 (10%)
   - Export spans: true
   - Record env vars: false

2. **Ask for customization**: Use `ask_user` to ask if they want to customize:
   "The default recording configuration is:
   - Sampling rate: 10% (0.1)
   - Export spans: enabled
   - Record environment variables: disabled

   Press Enter to accept defaults, or type 'custom' to customize:"

3. **If customizing**: Ask for each value:
   - Sampling rate (number between 0.0 and 1.0)
   - Export spans (yes/no)
   - Record env vars (yes/no)

4. **Save configuration**: Use `cloud_save_config` with:
   - `service_id`: from state.cloud_service_id
   - `sampling_rate`: the chosen rate
   - `export_spans`: the chosen value
   - `enable_env_var_recording`: the chosen value

5. **Transition**: Move to the next phase with:
   - `sampling_rate`: the chosen rate
   - `export_spans`: the chosen value
   - `enable_env_var_recording`: the chosen value

### Update .gitignore for Cloud

After saving the cloud configuration, update `.gitignore` to also exclude `.tusk/traces`:

1. Read `.gitignore` to check if `.tusk/traces` is already present
2. If not present, append `.tusk/traces` to the Tusk section

Since cloud users fetch traces from Tusk Cloud rather than storing them locally, the traces directory should be gitignored.

### Important Notes

- Lower sampling rates reduce performance overhead
- Export spans must be true for cloud features to work
- Environment variable recording is useful for apps that depend on env vars for business logic
