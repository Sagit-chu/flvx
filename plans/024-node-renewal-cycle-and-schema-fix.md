# Node Renewal Cycle And Schema Fix Plan

- [x] Review the node schema migration path and current expiry implementation
- [x] Backfill legacy node tables with the new metadata columns so old SQLite installs do not fail
- [x] Replace one-off node expiry UX with recurring renewal cycle fields (month/quarter/year)
- [x] Update node reminders and dashboard cards to use recurring renewal calculations
- [x] Verify backend and frontend changes, then complete the plan
