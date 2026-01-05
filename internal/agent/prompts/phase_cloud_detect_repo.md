## Phase: Detect Repository

Detect the Git repository information from the current directory.

### Steps

1. **Detect Git repository**: Use `cloud_detect_git_repo` to detect:
   - Repository owner (username or organization)
   - Repository name
   - Hosting type (github or gitlab)
   - Remote name and URL

2. **Handle multiple remotes**: If there are multiple git remotes:
   - Ask the user which remote to use
   - The response will include the list of available remotes

3. **Validate detection**: Ensure we have:
   - A valid repository owner
   - A valid repository name
   - A recognized hosting type (github or gitlab)

4. **Transition**: Once repository is detected, transition with:
   - `git_repo_owner`: the repository owner
   - `git_repo_name`: the repository name
   - `code_hosting_type`: "github" or "gitlab"

### Error Handling

- If not a git repository, inform the user they need to run from a git repository
- If no remotes configured, instruct the user to add a remote
- If hosting type is unsupported, inform the user that only GitHub and GitLab are supported
