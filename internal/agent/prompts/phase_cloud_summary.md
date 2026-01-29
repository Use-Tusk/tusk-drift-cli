## Phase: Cloud Summary

Generate a final report documenting the cloud setup and list of next steps.

### Report Content

1. **Cloud Setup Status**: ✅ Complete

2. **Configuration Summary**:
   - Repository: {owner}/{repo}
   - Service ID: {service_id}
   - Organization: {client_name}
   - Sampling Rate: {rate}%
   - Export Spans: {enabled/disabled}
   - Env Var Recording: {enabled/disabled}

3. **Trace Upload Status**:
   - Attempted: Yes/No
   - Success: Yes/No
   - Traces Uploaded: {count}
   - If failed, explain why and how to upload manually later

4. **Suite Validation Status**:
   - Attempted: Yes/No
   - Success: Yes/No
   - Tests in Suite: {count}
   - If no tests in suite, explain how to validate manually with `tusk run --cloud --validate-suite`

5. **API Key Status**:
   - Created: Yes/No/Already existed
   - If created, remind to save it securely

6. **Next Steps** (display this as a numbered checklist in the terminal output):

   **Step 1: Update your TuskDrift initialization**
   Add the API key to your `TuskDrift.initialize()` call so the SDK can upload traces to Tusk Cloud.
   - Understand/find their `TuskDrift.initialize` call in their codebase
   - Once you find their existing initialization, show them how to modify their code to add the API key
   - Your example should match their existing code style, file location, and any other options they already have configured
   - If the API key was not created (api_key_created == false), remind the user they'll need to create one at https://app.usetusk.ai/app/settings/api-keys before this will work.

   **Step 2: Store your environment variables**
   Set these environment variables wherever you want to record traces:
   - `TUSK_API_KEY=<your key>`
   - `TUSK_DRIFT_MODE=RECORD`

   **Step 3: Deploy to a recording environment**
   Deploy with these env vars to a **non-production** environment:
   - Choose a **development** or **staging** environment that typically has the default branch (main/master) deployed to it
   - It should receive some regular traffic — e.g. developers using it or QA teams testing against it
   - Avoid production: recording adds minor overhead, and a stable lower environment gives a cleaner baseline for trace recording

   **Step 4: Verify traces are recording**
   After deploying, confirm traces are being uploaded:
   - Run `tusk list --cloud` to see cloud traces
   - Or check the Tusk dashboard: https://app.usetusk.ai

   **Step 5: Set up a CI workflow**
   Add Tusk Drift to your CI pipeline to automatically run tests on every PR. Note to the user that they can use same API key as above or create a new one:
   - Docs:
     - https://docs.usetusk.ai/api-tests/ci-cd-setup
     - https://docs.usetusk.ai/api-tests/tusk-drift-cloud#deviation-analysis
   

   **Step 6: Commit, merge & deploy**
   - Commit the SDK instrumentation changes and `.tusk/` config files
   - Open a PR/MR, merge, and deploy to your recording environment

7. **Useful Links**:
   - Dashboard: https://app.usetusk.ai
   - Documentation: https://docs.usetusk.ai/api-tests/tusk-drift-cloud
   - CI/CD Setup Guide: https://docs.usetusk.ai/api-tests/ci-cd-setup
   - Support: support@usetusk.ai

Mention that they can view this checklist at any time in `.tusk/CLOUD_SETUP_REPORT.md`.

### Steps For Agent (You)

1. **Generate report**: Create a comprehensive markdown report with all the above sections.

2. **Display report**: Output the full report content to the terminal so the user can read it. The Next Steps checklist (section 6) is especially important — make sure it is clearly displayed.

3. **Save report**: Use `write_file` to save the report to `.tusk/CLOUD_SETUP_REPORT.md`

4. **Transition**: Call `transition_phase` to complete the setup.

### Important Notes

- Include all relevant state information in the report
- Make the report actionable with clear next steps
- Emphasize the importance of committing and pushing changes
