## Phase: Verify Restore

Restore the original recording configuration and report verification results.

### Step 1: Restore Original Config

Use `cloud_save_config` to restore the original recording values from state:
- `sampling_rate`: use the `original_sampling_rate` value from state
- `export_spans`: use the `original_export_spans` value from state
- `enable_env_var_recording`: use the `original_enable_env_var_recording` value from state
- `service_id`: pass empty string (to avoid overwriting the existing service ID)

### Step 2: Report Results

Output a verification summary based on the current state:

If `verify_simple_passed` is true:
- Report that the setup is verified and working correctly
- If `verify_complex_passed` is also true, note that both simple and complex tests passed
- If `verify_complex_passed` is false, note that the simple test passed but the complex test failed or was skipped

If `verify_simple_passed` is false:
- Report that verification failed
- Recommend the user run `tusk setup` to reconfigure

### Transition

Call transition_phase with:
```json
{
  "results": {},
  "notes": "Verification complete. Simple: <pass/fail>. Complex: <pass/fail/skipped>. Config restored."
}
```
