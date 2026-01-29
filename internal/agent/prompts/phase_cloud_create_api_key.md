## Phase: Create API Key

Create an API key for recording traces to Tusk Cloud. This key authenticates your service with Tusk Cloud so recorded traces can be uploaded. This phase is optional - the user can skip if they already have an API key configured.

### Steps

1. **Check if API key exists**: Use `cloud_check_api_key_exists` to see if TUSK_API_KEY is already set.

2. **If API key exists**: Inform the user and skip to transition.

3. **If no API key**:
   - Ask the user if they want to create an API key now using `ask_user`:
     "This API key allows Tusk Drift to record and upload traces to Tusk Cloud.

     Would you like to create one now? Enter a name for the key (or press Enter to skip):"
   - If user provides a name, create the key with `cloud_create_api_key`:
     - `name`: the user-provided name
     - `client_id`: from state.selected_client_id
   - If user skips (empty input), that's fine - proceed to transition

4. **If API key created**:
   - Display the key to the user with clear instructions:
     - Save the key securely - it won't be shown again
     - Add to shell config: `export TUSK_API_KEY=<key>`
     - Or set as CI/CD secret
   - Wait for user acknowledgment

5. **Transition**: Move to the next phase with:
   - `api_key_created`: true/false

### Important Notes

- API keys are only shown once - emphasize saving the key
- This phase is optional - users can create keys later in the web UI
- Provide the web UI link: <https://app.usetusk.ai/app/settings/api-keys>
