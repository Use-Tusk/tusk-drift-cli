## Phase: Cloud Auth

Authenticate the user with Tusk Cloud. This is required to access cloud features.

### Steps

1. **Check existing authentication**: Use `cloud_check_auth` to see if the user is already logged in.

2. **If already authenticated**:
   - Get the list of organizations with `cloud_get_clients`
   - If there's only one organization, use it automatically
   - If there are multiple organizations, use `ask_user_select` to let the user choose:
     - Provide options with `id` (the client ID) and `label` (the organization name)
     - The tool returns the selected ID
   - Select the organization with `cloud_select_client`

3. **If not authenticated** (login flow):
   a. Tell the user you need to authenticate them with Tusk Cloud
   b. Call `cloud_login` to initiate the device code flow
      - This returns a `verification_url` and `user_code`
      - It also opens the browser automatically
   c. Use `ask_user` to display the authentication URL and code to the user:
      - Tell them to complete authentication in their browser
      - Show them the URL they need to visit
      - Show them the user code they need to enter
      - Ask them to press Enter when done
   d. Call `cloud_wait_for_login` to wait for authentication to complete
      - This will poll until the user completes authentication
      - Returns success with user info when done
   e. After successful login, proceed with organization selection as above

4. **Transition**: Once authenticated and organization selected, transition to the next phase with:
   - `is_authenticated`: true
   - `user_email`: the user's email
   - `user_id`: the user's ID  
   - `selected_client_id`: the selected organization ID
   - `selected_client_name`: the selected organization name

### Important Notes

- The login flow is two-step: `cloud_login` starts it and returns the URL/code, `cloud_wait_for_login` waits for completion
- Use `ask_user` for free-text input or displaying information that needs confirmation
- Use `ask_user_select` when presenting the user with specific options to choose from (e.g., organizations)
- If `cloud_login` already succeeds (returns `success: true`), the user was authenticated via existing session - skip to organization selection
- If authentication fails or times out, provide clear instructions to try again
