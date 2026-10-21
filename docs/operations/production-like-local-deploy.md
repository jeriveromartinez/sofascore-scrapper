# Production-Like Local Deploy

**Audience:** operators and release engineers validating a `.deb` build
before it reaches a real Ubuntu host.

> **Status: historical.** Earlier revisions of this page documented a
> one-shot "production-like" deploy driven by the container image
> `iptv-prod-like:latest` and the entrypoint script
> `/usr/local/bin/deploy-prod-like.sh`. **Neither exists in this
> repository.** There is no `Dockerfile` for that image and no such
> script anywhere on disk; the only references to either name were in
> this page itself. Every command in the old revision — `docker run
> iptv-prod-like:latest`, `docker exec iptv-prod …`, the smoke-battery
> table, `/var/log/iptv.log` — was therefore unrunnable.
>
> The page is kept because the *goal* is still valid: exercise a real
> `.deb` against real MariaDB and Redis before shipping it. The rest
> of this document describes the workflows that actually exist.

## Validate a `.deb` today

| Goal | Use |
|---|---|
| Prove a `.deb` installs and the binary boots against real MariaDB and Redis | [Chain test](#chain-test) — `deployments/docker/Dockerfile.test` |
| Prove a **sequence** of `.deb` upgrades does not break the schema | [Chain test](#chain-test) — mount every `.deb` in `dist/` |
| Iterate on Go or Vue code against MariaDB + Redis without a `.deb` | [`compose.dev.yml`](README.md#docker-compose-for-testing) |
| Prove the install behaves on a real Ubuntu 24.04 host, with systemd | [Dedicated VM](#dedicated-ubuntu-vm) |

The compose files under `deployments/docker/` build from source; they
do **not** exercise `postinst`, the systemd unit, or the packaged
binary. Only the chain test and a real VM install do.

## Chain test

`deployments/docker/Dockerfile.test` builds `iptv-chain-test`: Ubuntu
24.04 with `mariadb-server`, `redis-server`, `sudo`, and `wget`. Its
entrypoint is `deployments/docker/chain-test.sh`. Nothing pre-baked —
the whole point is that `dpkg -i` runs the real `postinst` from the
real package.

```bash
# 1. Produce one or more .deb files.
./deployments/package/build-deb.sh <version>

# 2. Build the harness.
docker build -f deployments/docker/Dockerfile.test -t iptv-chain-test .

# 3. Run it against everything in dist/.
docker run --rm -p 8080:8080 -v "$PWD/dist:/tmp/dist:ro" iptv-chain-test
```

```powershell
docker build -f deployments/docker/Dockerfile.test -t iptv-chain-test .
docker run --rm -p 8080:8080 -v "${PWD}/dist:/tmp/dist:ro" iptv-chain-test
```

Docker Desktop on Windows: use the resolved Windows path for `-v`, not
a relative one.

### What the chain test does

1. Starts MariaDB and waits for `mariadb-admin ping` (up to 30 s).
2. Drops any leftover `iptv` database and `iptv` database user so each
   container run starts from the same baseline.
3. Starts Redis and waits for `redis-cli ping`.
4. Globs `/tmp/dist/iptv_*.deb` and installs them **in shell glob
   order** — oldest filename first — with `dpkg -i`. Each install
   therefore runs `postinst` against the state left by the previous
   version.
5. After each install, starts the binary directly as the `iptv` user
   (`sudo -u iptv -E /opt/iptv/iptv`) and polls `/health/live` for up
   to 60 seconds. The container has no systemd, so `iptv.service` is
   never started; the binary is launched the same way the unit would,
   with the same `WorkingDirectory` and env file.
6. On failure, prints the last 15 lines of the binary's log.

A version that fails to become healthy is logged and skipped — the
run only fails if the **last** `.deb` in the chain does not come up.
That models reality: an old published release may be broken, but the
release you are about to tag must not be.

### Schema smoke test

After the final `.deb` is healthy, the script asserts the schema that
GORM `AutoMigrate` produced, guarding the class of bug from PR #101
(`Unknown column 'deleted_at'` on insert):

- `deleted_at` column present on `push_messages`, `scheduled_pushes`,
  and `delivery_attempts`.
- Indexes `idx_push_messages_deleted_at`,
  `idx_scheduled_pushes_deleted_at`,
  `idx_delivery_attempts_deleted_at` present.
- An `INSERT` into each of the three tables accepts `deleted_at IS NULL`
  and a real timestamp, with `FOREIGN_KEY_CHECKS=0` (the harness has no
  user, device, or tournament rows to reference).
- The smoke rows are deleted again.

### Gotchas

- **Filename order is upgrade order.** The script globs with shell
  ordering, not a semver sort. `0.0.11` sorts before `0.0.5`
  lexicographically. Mount a directory containing only the packages you
  want, or rename copies so the glob order matches the upgrade order.
- **Do not pre-create the `iptv` database user.** `postinst` generates a
  random password and runs `CREATE USER IF NOT EXISTS`, which does not
  update an existing user's password. A pre-created user makes the env
  file and the actual grants diverge, and the binary then fails to
  authenticate. The script therefore drops the user and lets `postinst`
  create it.
- **The container has no systemd.** A unit-file regression that only
  shows up under `systemctl` — a bad `EnvironmentFile` mode, a missing
  `ReadWritePaths` — will not be caught here. Use the VM flow below for
  that.

## Dedicated Ubuntu VM

The only flow that exercises `apt install`, `postinst`, and
`iptv.service` exactly as production does:

```bash
# On the VM (Ubuntu 24.04, amd64)
scp dist/iptv_<version>_amd64.deb user@vm:/tmp/
ssh user@vm
sudo apt install /tmp/iptv_<version>_amd64.deb
systemctl status iptv.service
curl -s http://127.0.0.1:8080/health/ready
sudo journalctl -u iptv.service -n 100 --no-pager
```

To exercise an upgrade path rather than a fresh install, install the
older package first, create some state (a registered admin, a scraped
event), then install the newer one and confirm the state survived and
`AutoMigrate` applied cleanly.

See [`deb-package.md`](deb-package.md) for the full postinst contract
and the upgrade procedure.

## Iterating on code: compose.dev.yml

For the edit-restart loop, skip the packaging entirely:

```bash
# compose.dev.yml fails fast unless DB_PASSWORD and JWT_SECRET are set.
printf 'DB_PASSWORD=%s\nJWT_SECRET=%s\n' \
    "$(openssl rand -hex 16)" "$(openssl rand -hex 32)" > .env

docker compose -f deployments/docker/compose.dev.yml up --build
curl http://localhost:8080/health/live
```

MariaDB 10.11, Redis 7-alpine, and one backend built from the repo
`Dockerfile`, with persistent volumes. It reuses the same `/health/live`
and `/health/ready` probes as the packaged service, so a green
`compose.dev.yml` plus a green chain test is a good signal before the
VM step.

`compose.multi.yml` runs three backends behind nginx with Prometheus;
`compose.test.yml` is the integration-test harness. Both are documented
in [`README.md`](README.md#docker-compose-for-testing).

## Registering the first admin: the wire format is protobuf

Every `/api/web/v1/users/*` endpoint speaks **Protocol Buffers**, not
JSON. The request body is the binary wire encoding of the message
declared in `proto/api.proto`, the response is protobuf, and the
content type is `application/x-protobuf`.

`server.ParseProtoBody` calls `proto.Unmarshal` on the raw body without
looking at `Content-Type`. A `curl -d '{"email":…,"password":…}'`
therefore either fails protobuf decoding or decodes to a zero-valued
message; either way the handler short-circuits and answers:

```
400 {"error":"email and password are required"}
```

This looks like a validation bug but is a wire-format mismatch. Do not
`curl` these endpoints with JSON.

Use the bundled round-trip client instead, which speaks the real format.
It needs a base URL and a bootstrap token, so point it at a host you
control:

```bash
# Mint the invitation token on the machine holding the install.
sudo -u iptv bash -c 'set -a; . /etc/iptv/env; set +a; /opt/iptv/iptv bootstrap-invitation'

# Paste the token and base URL into scripts/auth_smoke/main.go, then:
go run ./scripts/auth_smoke/
```

> Do not try to `docker exec` into the chain-test container for this.
> It runs with `--rm`, so its writable layer — `/etc/iptv/env` and the
> generated database password — is discarded the moment the chain
> finishes. Mint the token on a `.deb` install host or a container you
> started yourself.

`scripts/auth_smoke/main.go` exercises register → list users → mint an
invitation → refresh → logout, printing each status and the decoded
proto fields. For ad-hoc calls, marshal the request with
`google.golang.org/protobuf/proto` and the generated bindings in
`internal/gen/api` — see [`runbook.md`](runbook.md) → Bootstrap via
invitation.

The Vue dashboard at `http://localhost:8080` speaks protobuf correctly
through its generated `web/src/api` bindings.

## Known limitation: push metrics on MariaDB

`GET /api/web/v1/pushes/metrics/aggregate` (registered in
`internal/push/handler.go`, under the `pushes` group) answers `200`,
but two of its sub-queries use `strftime('%H', sent_at)` and
`date('now')` — SQLite functions that MariaDB does not implement. See
`internal/push/metrics_aggregator.go`.

The aggregator ignores those two errors (`if err == nil`), so on
MariaDB:

- `audiencePeakToday` comes back `0`.
- `hourlyHistogram30D` comes back empty.
- `topPlatforms` and `topAppVersions` are unaffected — that query is
  plain SQL and works.

The endpoint never surfaces an error, so this shows up as "the chart is
blank", not as a 500. Fixing it means swapping the date functions for
their MariaDB equivalents (`HOUR(sent_at)`, `DATE(sent_at)`); that
change is not in this repository yet.

## Troubleshooting

| Symptom | Cause and fix |
|---|---|
| `FAIL: no .deb files found in /tmp/dist` | Empty or wrong bind mount. `docker run --rm -v "$PWD/dist:/tmp/dist:ro" alpine ls /tmp/dist` to check the mount independently. |
| `FAIL: final version ... did not start cleanly` | The last `.deb` in the chain. The script prints the last 15 log lines before the summary — read those first. |
| An older version logs `FAIL: <deb> did not become healthy` | Expected and tolerated when the historical release was broken. Only the final version gates the run. |
| `mariadbd: Can't open and lock privilege tables` | `/var/lib/mysql` bind-mounted from an empty host directory. The chain-test image uses the image's own `datadir`; do not mount over it. |
| Register returns `email and password are required` | JSON sent to a protobuf endpoint. See above. |
| `users table is not empty, refusing to bootstrap invitation` | Someone already registered. Recreate the container for a clean slate, or use the existing credentials. |
| Chain installs versions out of order | Filename glob order, not semver. See gotchas above. |
| Metrics chart is empty on MariaDB | Known limitation, documented above. |

## Where to escalate

- Packaging, install, and upgrade — [`deb-package.md`](deb-package.md).
- Backups and downgrades — [`rollback.md`](rollback.md).
- Day-to-day operations and alerts — [`runbook.md`](runbook.md).
- Compose stacks and the iteration loop —
  [`README.md`](README.md#docker-compose-for-testing) and
  [`../development/README.md`](../development/README.md).