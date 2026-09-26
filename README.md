# E5SubBot

[简体中文](README_CN.md) · [Docker workflow](.github/workflows/docker.yaml)

E5SubBot is a Telegram bot that calls Microsoft Graph on a schedule for accounts you bind. Calling Graph does **not** guarantee that a Microsoft 365 E5 subscription will renew.

It supports SQLite or MySQL, account export, per-user task feedback, and administrator commands. The bot accepts commands in private chats.

## Set up

1. Create a Telegram bot and a Microsoft application. Add delegated `Mail.Read` and `User.Read` permissions and the `http://localhost/e5sub` redirect URI. The authorization request also asks for `openid` and `offline_access` scopes.
2. Copy [config.yml.example](config.yml.example) to `config.yml`. Set `bot_token`, `admin`, `cron`, and database settings. Keep this file private because it contains credentials.
3. Start the bot with Docker or a Go binary. Send `/bind` to the bot, then follow its prompts to provide the application ID and secret, followed by the redirected URL and an account alias.

## Docker

The published image is `microcharon/e5subbot` for `linux/amd64` and `linux/arm64`. For a new SQLite installation, set `sqlite.db: /data/data.db` in `config.yml`, then run:

```sh
mkdir -p data log
docker run -d --name e5sub --restart=always \
  -e TZ=Asia/Shanghai \
  -v "$PWD/config.yml:/config.yml:ro" \
  -v "$PWD/data:/data" \
  -v "$PWD/log:/log" \
  microcharon/e5subbot:latest
docker logs -f e5sub
```

For an existing installation, keep its current `config.yml` and database path. Back up `data.db` before replacing a container or changing mounts.

## Binary and service

Use a binary from this repository's [Releases](https://github.com/Debcharon/E5SubBot/releases), or build locally with `go build -o E5SubBot .`. Keep `config.yml` in the working directory.

For systemd, install the binary at `/usr/local/bin/E5SubBot` and put `config.yml` in `/usr/local/etc/E5SubBot`. Create an `e5subbot` service account that can read the configuration and write the SQLite database and `log/` directory in that working directory. Save this unit as `/etc/systemd/system/e5subbot.service`:

```ini
[Unit]
Description=E5SubBot
Wants=network-online.target
After=network-online.target

[Service]
Type=simple
User=e5subbot
Group=e5subbot
WorkingDirectory=/usr/local/etc/E5SubBot
ExecStart=/usr/local/bin/E5SubBot
Restart=on-failure
RestartSec=5s
UMask=0077
NoNewPrivileges=true

[Install]
WantedBy=multi-user.target
```

Run `sudo systemctl daemon-reload` and `sudo systemctl enable --now e5subbot` after saving the unit.

`make test` runs the Go tests. `make snapshot` creates local release archives with GoReleaser.

## Publishing a version

Release tags use `vMAJOR.YYYYMMDD.REVISION`, following [tego](https://github.com/Debcharon/tego/releases): `v1.20260926.0` is the first release on 2026-09-26 (UTC), and `v1.20260926.1` is another release on the same day. Reset `REVISION` to `0` on a new day; change `MAJOR` for a new incompatible release line.

After merging changes into `master`, tag the merged commit and push the tag. The [Docker workflow](.github/workflows/docker.yaml) publishes `microcharon/e5subbot:<tag>` and `:latest` for amd64 and arm64. The [binary release workflow](.github/workflows/release.yaml) runs tests, builds the platforms listed in `.goreleaser.yml`, and creates a GitHub Release with archives and checksums.

The Docker job uses the `Docker Hub` GitHub environment. Add `DOCKER_USERNAME` and `DOCKER_TOKEN` as secrets in that environment. GitHub provides the release workflow's `GITHUB_TOKEN` automatically. A tag push starts both workflows independently; check both runs before announcing a version.

## Configuration

| Key | Purpose |
| --- | --- |
| `bot_token` | Telegram bot token |
| `admin` | Comma-separated Telegram user IDs for `/task` and `/log` |
| `cron` | Five-field schedule for Graph requests |
| `db`, `table` | `sqlite` or `mysql`; existing account table name (usually `users`) |
| `sqlite.db` | SQLite database path |
| `mysql.*` | MySQL connection settings when `db: mysql` |
| `bindmax`, `errlimit`, `goroutine` | Per-user account limit, repeated-error limit, and task worker count |
| `notice`, `socks5` | Optional help text and Telegram SOCKS5 proxy |

`bindmax`, `errlimit`, `goroutine`, `admin`, and `notice` can be changed while the bot runs. Restart it after changing the token, database, proxy, cron schedule, or table name.

## Commands

| Command | Action |
| --- | --- |
| `/start`, `/help` | Show help and configured notice |
| `/bind` | Bind a Microsoft account |
| `/my`, `/unbind` | View or remove your accounts |
| `/export` | Send your account data as JSON; the file contains secrets |
| `/task`, `/log` | Run a task or download the log (administrators only) |

The task stores rotated refresh tokens. Accounts exceeding `errlimit` consecutive Graph errors are automatically unbound; back up or export account data before relying on that behavior.

Licensed under [GPLv3](LICENSE).
