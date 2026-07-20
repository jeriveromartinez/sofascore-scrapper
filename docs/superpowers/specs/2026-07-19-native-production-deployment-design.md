# Native Production Deployment Design

## Goal

Deploy every validated `main` revision automatically to the existing native IPTV installation on the self-hosted GitHub Actions runner labeled `iptv`.

The deployment must not use Docker or a container registry. It must preserve the existing production configuration, storage, and user-level systemd service.

## Existing Production Layout

- Application root: `/opt/iptv`
- Executable: `/opt/iptv/iptv`
- Dashboard assets: `/opt/iptv/web/dist`
- APK storage: `/opt/iptv/apk_storage`
- Image storage: `/opt/iptv/image_storage`
- Service: `iptv.service`, managed with `systemctl --user`
- Runner: self-hosted Linux runner with labels `self-hosted` and `iptv`
- Service account: `iptv`
- Host: TrueNAS system container running systemd as PID 1

The workflow must not replace the service unit or its environment configuration.

The `iptv` user manager exposes its bus at `/run/user/<uid>/bus`. GitHub Actions
jobs do not inherit `XDG_RUNTIME_DIR`, so the workflow must derive
`XDG_RUNTIME_DIR=/run/user/$(id -u)` instead of hardcoding the production UID.

## Trigger And Revision Selection

`.github/workflows/deploy.yml` will deploy automatically after the `CI` workflow completes successfully for `main`. A failed, cancelled, or skipped CI run must not start a deployment.

Manual `workflow_dispatch` remains available for an intentional redeployment from `main`.

For an automatic deployment, the workflow must check out `github.event.workflow_run.head_sha`, not the default-branch tip or the deploy workflow's own SHA. This guarantees that the deployed revision is the revision validated by CI. A manual run uses `github.sha` and rejects non-`main` refs.

Deployments use one concurrency group with `cancel-in-progress: false`, so a newer revision waits instead of interrupting a publication or rollback already in progress.

## Workflow Architecture

The production job uses:

- `runs-on: [self-hosted, iptv]`
- read-only repository contents permission
- the GitHub `production` environment
- a bounded timeout
- `actions/checkout` pinned to the selected deployment SHA

The workflow has four phases:

1. Select and audit the validated SHA.
2. provision and verify build and runtime dependencies.
3. build the frontend and native Go executable in the Actions workspace.
4. publish through a native deployment script and verify production health.

No repository checkout or source build occurs inside `/opt/iptv`.

## Dependency Provisioning And Preflight

The runner is Ubuntu. The workflow reports commands for an operator to install
missing base packages with `apt-get`; the deployment itself does not require
non-interactive sudo access.

Exact build toolchains are provisioned automatically with maintained GitHub Actions:

- Go from `go.mod` (`1.25.x`)
- Node.js 22 with npm cache keyed by `web/package-lock.json`

Before modifying production, preflight checks must:

- verify the runner is Linux and executes as `iptv`;
- derive and export `XDG_RUNTIME_DIR` from the effective UID;
- verify `$XDG_RUNTIME_DIR/bus` is a Unix socket;
- propagate `XDG_RUNTIME_DIR` through `$GITHUB_ENV` for later workflow steps;
- verify `systemctl --user` can find `iptv.service`;
- `/opt/iptv` exists and is writable by the runner account;
- `/opt/iptv/apk_storage` and `/opt/iptv/image_storage` exist and are writable;
- the current service configuration is left in place;
- `curl`, Go, Node.js, and npm are available after provisioning.

`deployments/native/deploy.sh` derives the same runtime-directory default when
it is invoked outside the workflow. It preserves an explicitly supplied value
for tests and alternate systemd user-manager layouts.

Database and Redis connectivity are verified through the application's readiness endpoint after restart. The current scraper uses HTTP with uTLS and has no Chromium runtime dependency.

## Build

The frontend build runs from `web/`:

```sh
npm ci
npm run build
```

The backend is built as a static Linux executable from the repository root:

```sh
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o build/iptv ./cmd/server
```

The deployment script receives only the completed executable and `web/dist` directory. A failed build cannot alter production.

## Native Publication

`deployments/native/deploy.sh` owns publication and rollback. It defaults to the known production paths but accepts environment overrides so its behavior can be tested without writing to `/opt/iptv`.

The script must:

1. validate its input artifacts and production paths;
2. stage the new executable and dashboard assets on the `/opt/iptv` filesystem;
3. preserve the current executable and dashboard assets as the previous release;
4. atomically replace `/opt/iptv/iptv` and `/opt/iptv/web/dist` without touching the service environment, `apk_storage`, or `image_storage`;
5. preserve executable mode on the installed binary;
6. restart with `systemctl --user restart iptv.service`;
7. derive the local readiness URL from `API_ADDR` in `/etc/iptv/iptv.env`, then require both an active service and a successful `/health/ready` response.

The script must clean temporary staging data on exit. The previous successful artifacts remain available for rollback and are replaced on the next deployment.

## Failure Handling And Rollback

Any failed preflight or build stops before production changes.

After publication begins, failure to install files, restart the service, reach the active state, or receive HTTP 200 from readiness triggers artifact rollback. Rollback restores the previous executable and dashboard assets and restarts `iptv.service`. The job then fails even if restoration succeeds, so GitHub records the deployment as unsuccessful.

If rollback also fails, the script reports both the original deployment failure and rollback failure and leaves diagnostics in the Actions log.

Application startup runs forward database migrations. Artifact rollback must never execute down migrations or modify the database. A migration that is incompatible with the previous executable still requires the manual procedure in `docs/operations/rollback.md`.

## Security And Operational Constraints

- The workflow runs only trusted `main` code on the production runner.
- Pull request code never executes on the `iptv` runner.
- No production secrets are copied into the repository or Actions workspace by this workflow.
- Existing environment values remain owned by `iptv.service`.
- Base dependency installation is an explicit operator action; application publication and service management run as `iptv`.
- Workflow logs include the deployed commit SHA but no environment values or credentials.

## Verification

Implementation verification must include:

- YAML parsing and inspection of workflow triggers, permissions, runner labels, concurrency, and SHA selection;
- `bash -n deployments/native/deploy.sh`;
- script tests with temporary deployment roots and fake `systemctl`/health commands covering successful publication and failed-health rollback;
- workflow contract checks for dynamic user runtime-directory derivation, user-bus socket validation, `$GITHUB_ENV` propagation, and absence of a hardcoded UID;
- `npm ci && npm run build`;
- `CGO_ENABLED=0 go build ./cmd/server`;
- existing Go tests and vet checks;
- review that no Docker or registry step remains in the production deployment;
- after merge, confirmation in GitHub Actions that the `iptv` runner executes the validated SHA and `/health/ready` succeeds.
