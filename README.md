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

Release archives support Linux amd64/arm64, Windows amd64, and macOS amd64/arm64. Run `E5SubBot --version` to see the version, commit, and build date.

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

Pull requests and pushes to `master` run formatting checks, race tests, `go vet`, workflow linting, and release configuration checks. Both publishing workflows also validate the target tag before publishing.

## Configuration

| Key | Purpose |
| --- | --- |
| `bot_token` | Telegram bot token |
| `admin` | Comma-separated Telegram user IDs for `/task`, `/status`, and `/log` |
| `cron` | Five-field schedule for Graph requests |
| `db`, `table` | `sqlite` or `mysql`; existing account table name (usually `users`) |
| `sqlite.db` | SQLite database path |
| `mysql.*` | MySQL connection settings when `db: mysql` |
| `bindmax`, `errlimit`, `goroutine` | Per-user account limit, consecutive authorization-error notification threshold, and task worker count |
| `log_stdout_only` | Default `false`; set to `true` to write only to standard output |
| `notice`, `socks5` | Optional help text and Telegram SOCKS5 proxy |

`bindmax`, `errlimit`, `goroutine`, `admin`, and `notice` can be changed while the bot runs. Restart it after changing the token, database, proxy, cron schedule, table name, or logging mode. With stdout-only logging, use `journalctl -u e5subbot` or `docker logs e5sub`; `/log` does not provide a file.

## Commands

| Command | Action |
| --- | --- |
| `/start`, `/help` | Show help and configured notice |
| `/bind` | Bind a Microsoft account |
| `/cancel` | Cancel binding; unfinished sessions expire after 15 minutes |
| `/my`, `/unbind` | View or remove your accounts |
| `/export` | Send your account data as JSON; the file contains secrets |
| `/task`, `/status`, `/log` | Run a task, view the version and latest task status, or download the log (administrators only) |

The task stores rotated refresh tokens even if the subsequent mail request fails. Network failures, throttling, and server errors never automatically unbind accounts; throttling and server errors receive bounded retries. After more than `errlimit` consecutive explicit authorization errors, the bot asks for reauthorization and retains the account data. Error counters and the latest task status reset after a restart.

To update authorization or application credentials, back up with `/export`, then use `/unbind` and `/bind`. Account details hide secrets; exported files still contain complete credentials and must be kept private.

Licensed under [GPLv3](LICENSE).
