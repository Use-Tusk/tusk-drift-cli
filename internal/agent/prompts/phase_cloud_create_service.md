## Phase: Create Observable Service

Create an observable service in Tusk Cloud for this repository.

### Steps

1. **Generate a default service name**: Based on what you've learned about this project (framework, entry point, repo name, project type), come up with a concise, descriptive name for this service. For example: "Users API", "Payment Gateway", "Auth Service", etc. Keep it short (2-4 words) and meaningful.

2. **Ask for service name**: Use `ask_user` to ask: "What would you like to name this observable service? (default: {your generated name})"
   - If the user provides an empty or blank response, use your generated default name.

3. **Create the service**: Use `cloud_create_service` with:
   - `owner`: from state.git_repo_owner
   - `repo`: from state.git_repo_name
   - `client_id`: from state.selected_client_id
   - `project_type`: from state.project_type (e.g., "nodejs" or "python")
   - `service_name`: the name from step 2
   - `app_dir`: (optional) relative path if not at repo root

4. **On success**: The response will include the `service_id`

5. **Transition**: Move to the next phase with:
   - `cloud_service_id`: the created service ID
   - `service_name`: the name chosen in step 2

### Error Handling

- If the service already exists, that's okay - use the existing service ID
- If there are permission errors, suggest the user check their organization settings
