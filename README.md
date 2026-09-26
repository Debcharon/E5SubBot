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

Use a binary from this repository's [Releases](https://github.com/Debcharon/E5SubBot/releases), or build locally with `go build -o E5SubBot .`. Keep `config.yml` in the working directory. The [systemd unit](e5subbot.service) expects the binary at `/usr/local/bin/E5SubBot` and the working directory at `/usr/local/etc/E5SubBot`.

`make test` runs the Go tests. `make snapshot` creates local release archives with GoReleaser.

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
