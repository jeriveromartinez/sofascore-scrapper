# Ubuntu `.deb` Package Guide

How to build, install, upgrade, and troubleshoot the Ubuntu `.deb`
package (`iptv_<version>_amd64.deb`). This is the production packaging
path: a single package containing the Go binary, the compiled Vue
dashboard, and a systemd unit — install it and the panel is up.

## Build

### Prerequisites

- Docker (the build runs entirely inside a container; no Go/Node toolchain on the host).
- `bash` (Linux/macOS/Git Bash on Windows) **or** PowerShell 5.1+ / PowerShell Core on Windows.

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

- `version` defaults to the latest git tag (`git describe --tags --abbrev=0`), with a leading `v` stripped. Falls back to `0.1.0` when there are no tags.
- The version becomes the `.deb` version (`Version:` in the control file) and the output filename.
- Output: `dist/iptv_<version>_amd64.deb`.

Examples:

```bash
./deployments/package/build-deb.sh 0.1.0
./deployments/package/build-deb.sh 1.2.3
```

```powershell
.\deployments\package\build-deb.ps1 -Version 1.2.3
```

### How the build works

`deployments/package/Dockerfile.builder` is a three-stage build:

1. **`build-vue`** — `node:22-alpine`, runs `npm ci` + `npm run build` on `web/`.
2. **`build-go`** — `golang:1.25-alpine`, runs `go build` with `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 -trimpath -ldflags="-s -w"` plus `internal/buildinfo` overrides for `Version` and `Commit`.
3. **`package`** — `ubuntu:22.04` with `dpkg-dev`; lays out the filesystem tree, runs `dpkg-deb --build --root-owner-group`, and leaves the `.deb` at `/iptv.deb`.

The image keeps `/iptv.deb`; `build-deb.sh` / `build-deb.ps1` extract it with `docker create` + `docker cp` (avoids MSYS / PowerShell path mangling of `/bin/sh` on Windows hosts).

### What goes inside

| Path | Contents |
|---|---|
| `/opt/iptv/iptv` | Go backend binary (static, amd64) |
| `/opt/iptv/web/dist` | Compiled Vue dashboard (served by the binary) |
| `/lib/systemd/system/iptv.service` | systemd unit |
| `/etc/iptv/env` | Generated on first install by `postinst` |

The control file declares `Depends: mariadb-server (>= 10.5), redis-server (>= 5.0)`.

## Install

On Ubuntu 22.04/24.04 (amd64), with root or sudo:

```bash
sudo apt install ./dist/iptv_<version>_amd64.deb
```

Using `apt` (not `dpkg -i`) resolves the `mariadb-server` and `redis-server` dependencies automatically. If you must use `dpkg -i`, install dependencies first or pass `--force-depends`.

### What `postinst` does automatically

On first install and on every upgrade:

1. Creates the `iptv` system user (`/usr/sbin/nologin` shell) if missing.
2. Creates `/opt/iptv/apk_storage` and `/opt/iptv/image_storage`, chowned to `iptv`.
3. If `/etc/iptv/env` does not exist yet, generates it with:
   - random `DB_PASSWORD` (24 chars) and `JWT_SECRET` (48 chars) from `/dev/urandom`;
   - defaults: `API_ADDR=0.0.0.0:8080`, `DB_HOST=127.0.0.1`, `DB_USER=iptv`, `DB_NAME=iptv`, `REDIS_URL=redis://127.0.0.1:6379/0`, storage paths under `/opt/iptv`.
   - Permissions: `0640`, owner `root:iptv`.
4. Runs `systemctl daemon-reload`.
5. If MariaDB is running, creates the database and user: `CREATE DATABASE iptv`, `CREATE USER 'iptv'@'localhost'` + `'iptv'@'127.0.0.1'` with the generated password, grants all on `iptv.*`.
6. Enables and starts `iptv.service`.

> The config file is **never overwritten** on upgrade — existing `/etc/iptv/env` is kept as-is. To rotate credentials, edit the file and restart the service.

### First login (bootstrap)

The first account registered is promoted to `admin` automatically. To get a bootstrap invitation token (only valid while the `users` table is empty):

```bash
sudo -u iptv bash -c 'set -a; . /etc/iptv/env; set +a; /opt/iptv/iptv bootstrap-invitation'
```

In Docker, run the equivalent against the running container (the
service is the same one — the bootstrap subcommand uses the env file
the postinst generated, so it picks up the same MariaDB/Redis/JWT
credentials as the live `iptv` binary):

```bash
docker exec -u root <container> bash -c 'sudo -u iptv bash -c "set -a; . /etc/iptv/env; set +a; /opt/iptv/iptv bootstrap-invitation"'
```

The token is printed to stdout, valid for 24h (Redis TTL), and
single-use (consumed atomically by the registration handler).
The subcommand refuses to issue a token when the `users` table is
non-empty, so use it on a fresh install or after a full wipe.

Then register via `POST /api/web/v1/users/register` (email + password + `invitation_token`).

## Service management

```bash
sudo systemctl status iptv.service
sudo systemctl restart iptv.service
sudo journalctl -u iptv.service -f
```

The unit runs `Type=simple`, user/group `iptv`, `WorkingDirectory=/opt/iptv`,
`EnvironmentFile=/etc/iptv/env`, with `NoNewPrivileges`,
`ProtectSystem=full`, `ReadWritePaths=/opt/iptv`, `ProtectHome=true`.

## Upgrade

```bash
./deployments/package/build-deb.sh <new-version>
sudo apt install ./dist/iptv_<new-version>_amd64.deb
```

Or on Windows:

```powershell
.\deployments\package\build-deb.ps1 -Version <new-version>
# Copy dist\iptv_<new-version>_amd64.deb to the target Ubuntu host, then:
sudo apt install ./iptv_<new-version>_amd64.deb
```

`postinst` runs again; `/etc/iptv/env` and stored data (DB, `apk_storage`, `image_storage`) are preserved. The service is restarted.

## Uninstall

```bash
sudo apt remove iptv        # stops + disables the service, keeps /etc/iptv/env and data
sudo apt purge iptv         # also removes /etc/iptv/env and storage dirs
```

`prerm` stops and disables `iptv.service` on remove/purge.

## Troubleshooting

| Symptom | Cause / fix |
|---|---|
| `JWT_SECRET is required` when running the binary by hand | The env file is not loaded. Source it first: `set -a; . /etc/iptv/env; set +a` — the systemd unit does this automatically. |
| `connect database: dial tcp ... connection refused` | MariaDB not running or `DB_HOST`/`DB_PORT` wrong in `/etc/iptv/env`. |
| Service starts then exits | Check `journalctl -u iptv.service -e`; verify DB credentials in `/etc/iptv/env` match what `postinst` created (`sudo mariadb -uroot -e "SELECT User,Host FROM mysql.user"`). |
| Port 8080 already in use | Change `API_ADDR` in `/etc/iptv/env` and restart. |
| Dashboard 404 / API-only | The binary serves `web/dist` relative to `WorkingDirectory`; verify `/opt/iptv/web/dist/index.html` exists (it is packaged). |
| Scraper returns 403 from SofaScore | External anti-bot block on the host IP; unrelated to the package. |
