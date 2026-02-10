## Phase: Setup CI Workflow

Create (or update) a GitHub Actions workflow that runs Tusk Drift replay on every push/PR using `Use-Tusk/drift-action@v1`.

### Goal

Add a workflow file that:

1. Triggers on pull requests and pushes to default branch.
2. Reuses any project-specific CI preparation needed for replay (for example env file copy, image build, dependency install, app setup).
3. Uses `Use-Tusk/drift-action@v1` instead of manual CLI install/cache/run steps.
4. Uses `TUSK_API_KEY` from secrets.
5. Follows the current action input contract from the action definition (`action.yml`), not assumptions.

### Steps

1. **Inspect existing workflows**
   - Look in `.github/workflows/` (usually in the repo root, if the current service directory doesn't have it) for relevant CI workflow patterns.
   - Prefer matching existing conventions in this repo (naming, triggers, setup steps, style).

2. **Detect replay prerequisites**
   - If existing workflows include setup needed before tests (for example Docker buildx/bake, env copy, dependency install), include those before the Drift step.
   - Keep only what is needed for replay to start and run the app/tests. If it's not a Docker setup, it is likely that we DO NOT need to include any external services like Postgres, Redis, etc -- only what is needed to start the service itself.

3. **Fetch action docs/contract (authoritative first)**
   - Use `http_request` to fetch:
     - `https://raw.githubusercontent.com/Use-Tusk/drift-action/v1/action.yml` (authoritative input contract)
     - optionally: `https://raw.githubusercontent.com/Use-Tusk/drift-action/v1/README.md` (usage examples)
   - If remote fetch fails, continue with the minimal known-good snippet below.
   - Prefer `action.yml` over README when there is any mismatch.

4. **Create or update workflow**
   - If a Tusk Drift workflow already exists, update it to use `Use-Tusk/drift-action@v1`.
   - Otherwise create `.github/workflows/tusk-drift.yml`.
   - Use this shape for the Drift step:

   ```yaml
   - name: Run Tusk Drift trace tests
     uses: Use-Tusk/drift-action@v1
     with:
       cache-key: ${{ runner.os }}-tusk-drift-${{ hashFiles('.tusk/config.yaml') }}
       api-key: ${{ secrets.TUSK_API_KEY }}
   ```

5. **Important behavior requirements**
   - Do not add manual `curl ... install.sh`, manual `actions/cache`, or direct `tusk run` commands if using `drift-action`.
   - Keep `actions/checkout` in the workflow.
   - If the service runs from a subdirectory, set `working-directory` input on the Drift action.
   - Do not hardcode API keys; always use `${{ secrets.TUSK_API_KEY }}`.
   - Keep action configuration minimal by default:
     - Do not set `cli-source` unless there is a clear reason.
     - If `cli-source` is explicitly needed, default to `release` (do not use `source` in normal customer workflows).
     - Avoid tweaking advanced inputs unless required by the repo.

6. **Show the result to the user**
   - Print the final workflow path and key snippet so the user can review quickly.
   - Mention any assumptions you made.

7. **Transition**
   - Call `transition_phase` with:
     - `ci_workflow_configured`: true/false
     - `ci_workflow_path`: created/updated workflow path (if any)

### Important Notes

- Replay often works without external dependencies because outbound calls are mocked from traces, but still preserve any setup that the app needs to boot in CI.
- Prefer minimal, reliable setup over copying unrelated heavy steps.
