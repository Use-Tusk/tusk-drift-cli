## Phase: Upload Traces

Upload local traces from `.tusk/traces/` to Tusk Cloud.

### Step 1: Check for Local Traces

Use `tusk_list` to check if there are any local traces in `.tusk/traces/`.

### Step 2: If No Traces Exist

If no traces are found:

1. **Explain the situation** to the user:
   - No local traces were found to upload in `.tusk/traces/`.
   - Traces are recordings of API requests that become your test suite
   - Without traces, there's nothing to upload or validate

2. **Suggest how to fix**: Suggest running `tusk setup` (without the flag) to go through the full setup flow which includes recording traces.

3. **Ask user to confirm** using `ask_user`:
   ```
   "No local traces found in .tusk/traces/. Traces are needed to create your initial test suite.

   To record traces, run `tusk setup` (without --skip-to-cloud) which will guide you through recording.

   Would you like to continue without uploading traces? You can always upload traces later. (yes/no)"
   ```

4. **If user says yes**: Transition with:
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

5. **If user says no**: Use `abort_setup` with:
   ```json
   {
     "reason": "User chose to stop and record traces first. Run `tusk setup` to record traces."
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
