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
Then save the same report using write_file to the exact path: `.tusk/setup/SETUP_REPORT.md`
⚠️ IMPORTANT: The file MUST be written to `.tusk/setup/SETUP_REPORT.md` — do NOT use any other filename or directory (e.g. do NOT write to `.tusk/SETUP_NOTES.md` or any other path).

After saving the report, call transition_phase to complete.
