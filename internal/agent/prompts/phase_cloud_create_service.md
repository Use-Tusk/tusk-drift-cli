## Phase: Create Observable Service

Create an observable service in Tusk Cloud for this repository.

### Steps

1. **Ask for service name**: Use `ask_user` to ask: "What would you like to name this observable service? (default: Backend service)"
   - If the user provides an empty or blank response, use "Backend service" as the name.

2. **Create the service**: Use `cloud_create_service` with:
   - `owner`: from state.git_repo_owner
   - `repo`: from state.git_repo_name
   - `client_id`: from state.selected_client_id
   - `project_type`: from state.project_type (e.g., "nodejs" or "python")
   - `service_name`: the name from step 1
   - `app_dir`: (optional) relative path if not at repo root

3. **On success**: The response will include the `service_id`

4. **Transition**: Move to the next phase with:
   - `cloud_service_id`: the created service ID
   - `service_name`: the name chosen in step 1

### Error Handling

- If the service already exists, that's okay - use the existing service ID
- If there are permission errors, suggest the user check their organization settings
