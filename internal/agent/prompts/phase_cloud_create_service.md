## Phase: Create Observable Service

Create an observable service in Tusk Cloud for this repository.

### Steps

1. **Create the service**: Use `cloud_create_service` with:
   - `owner`: from state.git_repo_owner
   - `repo`: from state.git_repo_name
   - `client_id`: from state.selected_client_id
   - `project_type`: from state.project_type (e.g., "nodejs" or "python")
   - `app_dir`: (optional) relative path if not at repo root

2. **On success**: The response will include the `service_id`

3. **Transition**: Move to the next phase with:
   - `cloud_service_id`: the created service ID

### Error Handling

- If the service already exists, that's okay - use the existing service ID
- If there are permission errors, suggest the user check their organization settings
