## Phase: Summary

Generate a final report documenting:

1. **Setup Status**: ✅ Complete / 🟠 Partial / ❌ Failed
2. **Summary**:
   - SDK installed and initialized: Yes/No
   - Config file created: Yes/No
   - Simple test passed: Yes/No
   - Complex test passed: Yes/No/Skipped
3. **Configuration Created**:
   - List all files created/modified
4. **Test Results**:
   - What was tested
   - Any issues encountered
5. **Next Steps**:
   - Recommendations for the user
6. **Notes**:
   - Any important observations

IMPORTANT: First output the FULL report content as text in your response so the user can read it in the terminal.
Then save the same report to .tusk/setup/SETUP_REPORT.md using write_file.

After saving the report, call transition_phase to complete.
