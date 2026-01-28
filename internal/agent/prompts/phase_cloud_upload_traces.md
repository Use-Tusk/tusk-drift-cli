## Phase: Upload Traces

Upload local traces from `.tusk/traces/` to Tusk Cloud.

### Step 1: Check for Local Traces

Use `tusk_list` to check if there are any local traces in `.tusk/traces/`.

### Step 2: If No Traces Exist

If no traces are found:

1. **Check if setup was completed**: Use `tusk_validate_config` or check if `.tusk/config.yaml` exists to determine if the user has already completed local setup.

2. **If config exists** (setup was done before):
   - Explain that their config is already set up for cloud recording (`export_spans: true`)
   - They just need to start their app with `TUSK_DRIFT_MODE=RECORD` environment variable set
   - Make some API requests to their service
   - Traces will be saved to `.tusk/traces/`
   - After recording, they can re-run `tusk setup --skip-to-cloud` to continue
   
   Example message using `ask_user`:
   ```
   "No local traces found in .tusk/traces/, but your .tusk/config.yaml is already configured for cloud recording.

   Without traces, there's nothing to upload or validate against your service.
 
   To record traces:
   1. Start your app with TUSK_DRIFT_MODE=RECORD (e.g., `TUSK_DRIFT_MODE=RECORD <your start command>`)
   2. Make some API requests to your service
   3. Stop your app after a few seconds (traces are saved to .tusk/traces/)
   4. Re-run `tusk setup --skip-to-cloud`
 
   Would you like to stop here and record traces first? (recommended) (yes/no)
   ```

3. **If config does not exist** (setup was never done):
   - Suggest running `tusk setup` (without `--skip-to-cloud`) to go through the full setup flow which includes SDK instrumentation and recording traces
   
   Example message using `ask_user`:
   ```
   "No local traces found and no .tusk/config.yaml exists.

   Without traces, there's nothing to upload or validate against your service.
   
   Please run `tusk setup` (without --skip-to-cloud) first to:
   - Instrument the Tusk Drift SDK
   - Create configuration files
   - Record initial traces
   
   Would you like to stop here and record traces first? (recommended) (yes/no)
   ```

4. **If user says yes**: Before aborting, reset the config for local recording mode:
   
   a. First, call `cloud_save_config` to set recording config for local mode:
   ```json
   {
     "service_id": "<from state.cloud_service_id>",
     "sampling_rate": 1.0,
     "export_spans": false,
     "enable_env_var_recording": false
   }
   ```
   
   b. Then, call `reset_cloud_progress` to remove the Configure Recording phase from progress so it will run again:
   ```json
   {
     "phase_name": "Configure Recording"
   }
   ```
   
   c. Finally, call `abort_setup` with:
   ```json
   {
     "reason": "User chose to stop and record traces first. Config has been reset for local recording (sampling_rate: 1.0, export_spans: false). Run `tusk setup` to record traces, then `tusk setup --skip-to-cloud` to continue cloud setup."
   }
   ```

5. **If user says no**: Transition with:
   ```json
   {
     "results": {
       "trace_upload_attempted": false,
       "trace_upload_success": false,
       "traces_uploaded": 0
     },
     "notes": "No local traces found. User chose to continue."
   }
   ```

### Step 3: If Traces Exist

If traces are found, upload them to Tusk Cloud:

1. Use `cloud_upload_traces` with:
   - `service_id`: from state.cloud_service_id

2. Check the result for success and count of uploaded traces/spans

3. **Transition** with:
   ```json
   {
     "results": {
       "trace_upload_attempted": true,
       "trace_upload_success": true/false,
       "traces_uploaded": <number of trace files uploaded>
     }
   }
   ```

### Important Notes

- This phase gives users control over whether to continue without traces
- Upload failures should warn but allow continuing
- Users can always upload traces later by running `tusk setup` again
