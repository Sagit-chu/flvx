# Panel upgrade safety and recovery

The helper is a subcommand in the **currently running image**, dispatched before
application/database initialization. This fix must first be installed through the
normal administrator deployment procedure on older releases lacking the helper.
It cannot retroactively repair an already-running legacy upgrade helper.

Supported deployments have one `docker-compose.yml`, the original Compose project
labels, and a writable host bind mount containing that file and `.env`. The
helper mounts that directory at its **host absolute path** and passes explicit
project, project-directory, env-file and compose-file options. Multiple Compose
files, mismatched mounts/resources, or changed database settings fail closed.

Jobs are under `<deployment>/.panel-upgrades/<jobId>/` (directory 0700, files
0600). `*.original` contains immutable per-job configuration snapshots, not a live
database backup. `events.log` contains safe phase messages only. Legacy
`.upgrade.bak` files and `upgrade.log` are never changed. Helper containers remain
for diagnosis; they are intentionally not automatically removed. Operators may
remove completed helpers after retaining the job's files.

The `latest` pointer plus filesystem lock prevents simultaneous jobs across API
processes and backend replacement. Failed validation/pull leaves live files
untouched. A failed configuration application attempts to restore both files
before any recreate. Once `restarting` is recorded, failures require recovery:
no automatic database rollback, image rollback, or container rollback is safe.
The helper has a 30 minute overall deadline, including a maximum five minute
post-recreate verification window. Docker client timeout during recreation can
leave daemon work in flight; it is always treated as uncertain, not retried.

## Operator recovery after an interrupted/uncertain job

There is intentionally no remotely callable “clear lock” endpoint. Do not delete
job directories or rewrite status JSON to force another attempt.

1. Work on the Docker host and identify the job from `latest`, `status.json` and
   `plan.json`. Preserve the entire job directory, live Compose/env files, and
   container inspection information in a private location. These files can
   contain credentials. Never paste `.env` or raw Compose config into public logs.
2. Restore Docker daemon access if necessary. Inspect the exact helper container
   named by the job. If it is running, let it finish or explicitly stop it as an
   administrator. Verify it has stopped. If launch was uncertain, establish that
   no matching helper exists and no Docker create/up operation remains in flight.
   Do not clear the job while any of these facts are uncertain.
3. Inspect the original project's backend and frontend container IDs, images,
   mounts and health. Check the actual database and application logs privately.
   After any attempted recreation, assume migrations may already have occurred.
   Preserve a current database snapshot before further manual deployment work.
   Use the normal documented deployment procedure to bring both services to a
   consistent, healthy state. Do not copy an old database over the current one or
   blindly redeploy an older image. Configuration snapshots alone cannot undo a
   schema migration.
4. Only after verifying no helper/daemon work remains and both services and their
   database are consistent, archive the `latest` pointer under a unique filename
   in `.panel-upgrades/` (for example `resolved-<jobId>.pointer`). Do this while
   the API is stopped, or hold an exclusive OS flock on `.panel-upgrades/lock`,
   so no other start can race this manual recovery. Keep the job directory and
   archived pointer. This permits a new job without changing recovery evidence.
5. Restart the API if stopped and reload the browser settings page so it discovers
   the current `latest` task instead of following the resolved task ID. No current
   job is expected. A fresh start creates a new ID and new backups, never
   overwriting old evidence.

A lost helper, unavailable daemon, or stale status is displayed as
`recovery_required` while retaining the active lock. This is intentionally a
conservative derived status, so a helper that is still completing work can report
its actual durable result without racing a status request.
