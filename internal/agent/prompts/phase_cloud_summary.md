## Phase: Cloud Summary

Generate a final report documenting the cloud setup.

### Report Content

1. **Cloud Setup Status**: ✅ Complete

2. **Configuration Summary**:
   - Repository: {owner}/{repo}
   - Service ID: {service_id}
   - Organization: {client_name}
   - Sampling Rate: {rate}%
   - Export Spans: {enabled/disabled}
   - Env Var Recording: {enabled/disabled}

3. **API Key Status**:
   - Created: Yes/No/Already existed
   - If created, remind to save it securely

4. **Next Steps For User**:
   - Push changes to deploy with Tusk Drift
   - Run `tusk list --cloud` to see cloud traces
   - Run `tusk run --cloud` to execute cloud tests
   - Set up CI/CD workflow (if not done)

5. **Useful Links**:
   - Dashboard: <https://app.usetusk.ai>
   - Documentation: <https://docs.usetusk.ai/api-tests/tusk-drift-cloud>
   - Support: <support@usetusk.ai>

### Steps For Agent (You)

1. **Generate report**: Create a comprehensive markdown report with all the above sections.

2. **Display report**: Output the full report content to the terminal so the user can read it.

3. **Save report**: Use `write_file` to save the report to `.tusk/CLOUD_SETUP_REPORT.md`

4. **Transition**: Call `transition_phase` to complete the setup.

### Important Notes

- Include all relevant state information in the report
- Make the report actionable with clear next steps
- Emphasize the importance of committing and pushing changes
