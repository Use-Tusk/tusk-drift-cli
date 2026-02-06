## Phase: Create Observable Service

Create an observable service in Tusk Cloud for this repository.

**Important**: The `repo_id` from the previous phase (Verify Access) is NOT a service ID. It's an internal repository identifier. You must create a new observable service using `cloud_create_service` to get a `service_id` (which will be a UUID like `710bf707-1ba1-47f1-81ae-a3fb3d9a7424`).

### Steps

1. **Detect project type** (if not already known from state.project_type):
   - Use `list_directory` to check for project markers:
     - `package.json` → Node.js project ("nodejs")
     - `requirements.txt`, `pyproject.toml`, `setup.py`, or `Pipfile` → Python project ("python")
   - Do NOT ask the user - detect automatically from files

2. **Generate a default service name**: Based on what you've learned about this project (framework, entry point, repo name, project type), come up with a concise, descriptive name for this service. For example: "Users API", "Payment Gateway", "Auth Service", etc. Keep it short (2-4 words) and meaningful.

3. **Ask for service name**: Use `ask_user` to ask: "What would you like to name this observable service? (Press enter to use '{your generated name}')"
   - If the user provides an empty or blank response, use your generated default name.

4. **Create the service**: Use `cloud_create_service` with:
   - `owner`: from state.git_repo_owner
   - `repo`: from state.git_repo_name
   - `client_id`: from state.selected_client_id
   - `project_type`: detected in step 1, or from state.project_type
   - `service_name`: the name from step 3
   - `app_dir`: (optional) relative path if not at repo root

5. **On success**: The response will include the `service_id`

6. **Transition**: Move to the next phase with:
   - `cloud_service_id`: the created service ID
   - `service_name`: the name chosen in step 3
   - `project_type`: the detected project type (so subsequent phases can use it)

### Error Handling

- If the service already exists, that's okay - use the existing service ID
- If there are permission errors, suggest the user check their organization settings
