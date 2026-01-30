## Phase: Validate Suite

Validate uploaded traces and add them to the test suite.

### Prerequisites

Check the state from the previous phase:

- `trace_upload_success`: whether traces were uploaded successfully
- `traces_uploaded`: number of traces uploaded

### Step 1: Check if Validation is Needed

If no traces were uploaded (`trace_upload_success` is false or `traces_uploaded` is 0):

- Skip validation
- Inform user that validation was skipped because no traces were uploaded
- Transition with `suite_validation_attempted: false`

### Step 2: Run Validation

If traces were uploaded, run the validation:

1. Use `cloud_run_validation` tool (no parameters needed)
   - This executes `tusk run --cloud --validate-suite --print`
   - It validates all uploaded traces against the live service
   - Passing tests are automatically added to the test suite

> **Tip:** If validation fails because the server is already running on the required port, prompt the user to stop the existing server process before retrying. The validation tool needs to start its own instance of the service.


2. Parse the result:
   - `success`: whether validation ran successfully
   - `tests_passed`: number of tests that passed validation
   - `tests_failed`: number of tests that failed validation
   - `tests_in_suite`: number of tests added to suite (same as tests_passed)
   - `output`: raw output from the command

### Step 3: Handle Results

**If validation succeeds** (tests_in_suite > 0):

- Report success to user
- Mention how many tests are now in the suite

**If validation fails** (tests_in_suite == 0):

- Warn user that no tests passed validation
- Suggest they can:
  - Check the service is running correctly
  - Review trace recordings
  - Run `tusk run --cloud --validate-suite` manually later

**If validation errors**:

- Report the error
- Don't block the setup - continue to summary

### Step 4: Transition

Call `transition_phase` with:

```json
{
  "results": {
    "suite_validation_attempted": true,
    "suite_validation_success": true/false,
    "tests_in_suite": <number of tests added to suite>
  }
}
```

### Important Notes

- Validation requires the service to be running (it replays traces against live service)
- The `cloud_run_validation` tool handles starting/stopping the service internally
- Success is defined as having at least 1 test in the suite
- Failed validation should not block cloud setup completion
- Users can always run validation manually later with `tusk run --cloud --validate-suite`
