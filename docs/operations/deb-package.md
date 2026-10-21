# Ubuntu `.deb` Package Guide

**Audience:** operators, sysadmins, and release engineers shipping the
platform to Ubuntu.

`iptv_<version>_amd64.deb` is the production packaging path. One
package carries the Go backend, the compiled Vue dashboard, and a
systemd unit. Install it and the panel is up.

## Build

### Prerequisites

- Docker. The build runs entirely inside a container — no Go and no
  Node toolchain on the host.
- Either `bash` (Linux, macOS, or Git Bash on Windows) or
  PowerShell 5.1+ / PowerShell Core on Windows.

### Build the package

Linux, macOS, or Git Bash on Windows:

```bash
./deployments/package/build-deb.sh [-h|--help] [version]
```

Windows (PowerShell):

```powershell
.\deployments\package\build-deb.ps1 [-Version <version>]
Get-Help .\deployments\package\build-deb.ps1 -Full
```

Both scripts behave the same:

- `version` defaults to the most recent git tag (`git describe --tags --abbrev=0`)
  with a leading `v` stripped, and falls back to `0.1.0` in a repo
  with no tags.
- The version must match `^[0-9][0-9a-zA-Z.+~-]*$`.
- The version becomes both the `Version:` field of the control file and
  the output filename.
- Output: `dist/iptv_<version>_amd64.deb`.

```bash
./deployments/package/build-deb.sh 0.1.0
./deployments/package/build-deb.sh 1.2.3
```

```powershell
.\deployments\package\build-deb.ps1 -Version 1.2.3
```

### How the build works

`deployments/package/Dockerfile.builder` is a three-stage build:

| Stage | Base image | What it does |
|---|---|---|
| `build-vue` | `node:22-alpine` | `npm ci` + `npm run build` over `web/` |
| `build-go` | `golang:1.25-alpine` | `go build` with `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 -trimpath -ldflags="-s -w"`, plus `-X` overrides for `internal/buildinfo.Version` and `.Commit` |
| `package` | `ubuntu:22.04` + `dpkg-dev` | lays out the filesystem tree, stamps the version into the control file, runs `dpkg-deb --build --root-owner-group` |

`CGO_ENABLED=0` yields a statically linked binary, which is why the
package declares no Go runtime dependency. The short commit hash is
baked in so the dashboard's build panel can show version and commit.

The build image is also the artifact container: it keeps the finished
package at `/iptv.deb`, and both scripts extract it with
`docker create` + `docker cp` rather than running a shell in the
container (this avoids MSYS path mangling on Git Bash and quoting
problems in PowerShell).

### Package contents

| Path in the package | Source |
|---|---|
| `/opt/iptv/iptv` | Go backend binary (static, linux/amd64, mode 0755) |
| `/opt/iptv/web/dist` | Compiled Vue dashboard, served by the binary |
| `/lib/systemd/system/iptv.service` | `deployments/package/iptv.service` |
| `./DEBIAN/postinst`, `./DEBIAN/prerm` | Maintainer scripts, mode 0755 |

`/etc/iptv/env` is **not** shipped. It is generated on the target host
by `postinst` on first install, which is how the random database
password and JWT secret stay out of version control.

Control file dependencies:

```
Depends: mariadb-server (>= 10.5), redis-server (>= 5.0)
```

## Install

Ubuntu 22.04 or 24.04, amd64, with root or sudo:

```bash
sudo apt install ./dist/iptv_<version>_amd64.deb
```

Use `apt`, not `dpkg -i`: `apt` pulls in `mariadb-server` and
`redis-server` from the configured sources. With `dpkg -i` the install
aborts on unmet dependencies until you install them by hand.

### What `postinst` does

`postinst` runs on first install and again on every upgrade:

1. Creates the `iptv` system user if missing — home `/opt/iptv`, shell
   `/usr/sbin/nologin`.
2. Creates `/opt/iptv/apk_storage` and `/opt/iptv/image_storage`, then
   `chown -R iptv:iptv /opt/iptv`.
3. If `/etc/iptv/env` does **not** exist, generates it:
   - `DB_PASSWORD` — 24 random alphanumeric characters from
     `/dev/urandom`.
   - `JWT_SECRET` — 48 random alphanumeric characters.
   - Defaults: `API_ADDR=0.0.0.0:8080`, `DB_HOST=127.0.0.1`,
     `DB_PORT=3306`, `DB_USER=iptv`, `DB_NAME=iptv`,
     `REDIS_URL=redis://127.0.0.1:6379/0`,
     `APK_STORAGE_PATH=/opt/iptv/apk_storage`,
     `IMAGE_STORAGE_PATH=/opt/iptv/image_storage`,
     `SCRAPE_BATCH_SIZE=500`, `SCRAPE_CONCURRENCY=8`.
   - Mode `0640`, owner `root:iptv`.
4. Runs `systemctl daemon-reload`.
5. If MariaDB is running locally, creates the database and grants:
   `CREATE DATABASE iptv`, and `CREATE USER 'iptv'@'localhost'` plus
   `'iptv'@'127.0.0.1'` with the password from the env file, granted
   all privileges on `iptv.*`.
6. Enables and starts `iptv.service`.

Steps 1, 2, 4, 5, and 6 are safe to repeat. Step 3 is guarded by
`if [ ! -f /etc/iptv/env ]`.

> **Config survives upgrades.** An existing `/etc/iptv/env` is never
> overwritten, and `CREATE USER IF NOT EXISTS` does not reset a
> password. Rotating the database password or the JWT secret means
> editing the env file yourself and, for the database password,
> running the matching `ALTER USER` — then restarting the service.

> MariaDB often starts a few seconds after `apt` finishes. If step 5
> skipped because `mariadbd` was not up yet, the service will fail to
> connect. Fix with `sudo mariadb -uroot` and run the `CREATE DATABASE`
> / `CREATE USER` / `GRANT` statements by hand, or reinstall the
> package once MariaDB is running.

### First login: bootstrap invitation

The seeder creates a default `admin@local` / `admin1234` on an empty
`users` table. To skip that hardcoded credential and register the first
account through the invitation flow instead, mint a token **before**
anyone logs in:

```bash
sudo -u iptv bash -c 'set -a; . /etc/iptv/env; set +a; /opt/iptv/iptv bootstrap-invitation'
```

The token is printed to stdout. It carries a 24-hour Redis TTL, is
single-use (consumed atomically by the registration handler), and the
command refuses to issue one while `users` is non-empty.

The first account to register becomes the admin.

## Service management

```bash
sudo systemctl status iptv.service
sudo systemctl restart iptv.service
sudo journalctl -u iptv.service -f
```

The unit is `Type=simple`, runs as user and group `iptv`, with
`WorkingDirectory=/opt/iptv` and `EnvironmentFile=/etc/iptv/env`.
Hardening: `NoNewPrivileges`, `PrivateTmp`, `ProtectSystem=full`,
`ReadWritePaths=/opt/iptv`, `ProtectHome=true`, `LimitNOFILE=65536`,
`Restart=on-failure` with a 5-second backoff. It
`Requires` `mariadb.service` and `redis-server.service`.

Storage paths default to `./apk_storage` and `./image_storage`
relative to `WorkingDirectory`; `/etc/iptv/env` sets both explicitly.

## Upgrade

```bash
./deployments/package/build-deb.sh <new-version>
sudo apt install ./dist/iptv_<new-version>_amd64.deb
```

```powershell
.\deployments\package\build-deb.ps1 -Version <new-version>
# Copy dist\iptv_<new-version>_amd64.deb to the target host, then:
sudo apt install ./iptv_<new-version>_amd64.deb
```

`postinst` runs again. `/etc/iptv/env`, the database, and both storage
directories are preserved; the service is restarted. Schema changes
are applied by the new binary's `AutoMigrate` on boot — see the schema
policy in [`rollback.md`](rollback.md) → Schema policy, especially for
destructive changes that must be applied to the database by hand.

## Uninstall

```bash
sudo apt remove iptv
sudo apt purge iptv
```

The package ships **only** `postinst` and `prerm`. There is no
`postrm`. Both `remove` and `purge` therefore do exactly the same
thing — `prerm` stops and disables `iptv.service`, and nothing else.

Consequences, for both commands:

- `/etc/iptv/env` **survives**, including `DB_PASSWORD` and
  `JWT_SECRET`.
- `/opt/iptv/apk_storage`, `/opt/iptv/image_storage`, and
  `/opt/iptv/iptv` survive.
- The MariaDB database `iptv` and the `iptv` database user survive.
- The `iptv` system user survives.

`apt purge` does **not** clear the configuration. To wipe a host for
real, delete the paths by hand:

```bash
sudo systemctl stop iptv.service 2>/dev/null || true
sudo rm -rf /etc/iptv /opt/iptv
sudo mariadb -uroot -e "DROP DATABASE IF EXISTS iptv;
  DROP USER IF EXISTS 'iptv'@'localhost';
  DROP USER IF EXISTS 'iptv'@'127.0.0.1';"
sudo userdel iptv
```

## Troubleshooting

| Symptom | Cause and fix |
|---|---|
| `JWT_SECRET is required` when running the binary by hand | The env file is not loaded. `set -a; . /etc/iptv/env; set +a` first — the systemd unit does this for you. |
| `connect database: dial tcp ... connection refused` | MariaDB is not running, or `DB_HOST` / `DB_PORT` in `/etc/iptv/env` is wrong. |
| Service starts, then exits immediately | Check `sudo journalctl -u iptv.service -n 100`. Usually the DB user was never created because MariaDB was still booting during `postinst`. |
| Dashboard loads but every API call 401s | The JWT secret in `/etc/iptv/env` does not match what the running sessions were signed with — expected after a manual secret rotation; users must log in again. |
| Port 8080 already in use | Change `API_ADDR` in `/etc/iptv/env` and restart. |
| Dashboard 404 or API-only responses | Verify `/opt/iptv/web/dist/index.html` exists. The binary serves `web/dist` relative to `WorkingDirectory`. |
| `apt install` fails on dependencies | `dpkg -i` was used instead of `apt install`, or the apt sources are missing the Ubuntu archive. |
| Scraper returns 403 from an upstream | Anti-bot block on the host IP. Unrelated to packaging. |