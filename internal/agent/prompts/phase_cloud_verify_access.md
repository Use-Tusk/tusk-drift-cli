## Phase: Verify Repository Access

Verify that Tusk has access to the GitHub/GitLab repository.

### Steps

1. **Check access**: Use `cloud_verify_repo_access` with:
   - `owner`: from state.git_repo_owner
   - `repo`: from state.git_repo_name
   - `client_id`: from state.selected_client_id

2. **If access verified**: Transition to the next phase with success.

3. **If NO_CODE_HOSTING_RESOURCE error**:
   - The user needs to install the Tusk GitHub/GitLab app
   - Use `cloud_get_auth_url` to get the installation URL with:
     - `hosting_type`: from state.code_hosting_type
     - `client_id`: from state.selected_client_id
     - `user_id`: from state.user_id
   - Use `cloud_open_browser` to open the installation URL
   - Inform the user to:
     1. Install the Tusk app in the browser
     2. Grant access to their repository
     3. Return here and press Enter to retry
   - Use `ask_user` to wait for the user to complete installation
   - Retry `cloud_verify_repo_access`

4. **If REPO_NOT_FOUND error**:
   - The Tusk app is installed but doesn't have access to this specific repo
   - Instruct the user to update their Tusk app permissions
   - Use the same browser flow as above

5. **Transition**: Once access is verified, transition with:
   - `repo_access_verified`: true

### Important Notes

- The user will need to leave the terminal to complete browser-based authorization
- Be patient and provide clear instructions
- Allow multiple retry attempts
- If repeated failures, suggest contacting <support@usetusk.ai>
