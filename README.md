# E5SubBot

[简体中文](README_CN.md)

A Telegram bot for scheduled Microsoft Graph requests, with SQLite/MySQL storage and account export. Commands are available in private chats. Graph activity **does not guarantee** Microsoft 365 E5 subscription renewal.

## Setup

1. Create a Telegram bot and a Microsoft application with delegated `Mail.Read` and `User.Read` permissions. Set the redirect URI to `http://localhost/e5sub`.
2. Copy [config.yml.example](config.yml.example) to `config.yml` and fill in the bot token, administrator IDs, schedule, and database settings.
3. Start the bot and send `/bind`. Follow the prompts to provide the application credentials, authorization callback URL, and alias.

## Docker

Image: `microcharon/e5subbot` (`linux/amd64`, `linux/arm64`). For SQLite, set `sqlite.db: /data/data.db`:

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

Keep existing database paths when upgrading and back up the database first.

## Binary

Download from [Releases](https://github.com/Debcharon/E5SubBot/releases) or build with `go build -o E5SubBot .`. Supported platforms: Linux amd64/arm64, Windows amd64, and macOS amd64/arm64. Run from the directory containing `config.yml`; use `--version` to check the build.

<details>
<summary>Run with systemd</summary>

Install the binary at `/usr/local/bin/E5SubBot` and the configuration at `/usr/local/etc/E5SubBot/config.yml`. Create an `e5subbot` user with permission to read the configuration and write the database and `log/` directory. Save the following as `/etc/systemd/system/e5subbot.service`:

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

```sh
sudo systemctl daemon-reload
sudo systemctl enable --now e5subbot
journalctl -u e5subbot -f
```

</details>

## Configuration and commands

See [config.yml.example](config.yml.example) for settings. `bindmax`, `errlimit`, `goroutine`, `admin`, and `notice` reload automatically; other changes require a restart. Set `log_stdout_only: true` to disable file logging and `/log` downloads.

| Command | Action |
| --- | --- |
| `/start`, `/help` | Help |
| `/bind`, `/cancel` | Start or cancel binding; sessions expire after 15 minutes |
| `/my`, `/unbind` | View or remove your accounts |
| `/export` | Export account data, including credentials |
| `/task`, `/status`, `/log` | Run a task, check status, or download logs (administrators only) |

Accounts are retained after failures. More than `errlimit` consecutive authorization errors trigger a reauthorization notice: back up with `/export`, then use `/unbind` and `/bind`. Keep configuration and exported credentials private.

## Releases

After merging into `master`, push a tag in `vMAJOR.YYYYMMDD.REVISION` format, e.g. `v1.20260926.0` (UTC date; daily revision starts at `0`). Publishing requires CI to pass and produces GitHub Release archives/checksums and Docker `<tag>`/`latest` images.

Set `DOCKER_USERNAME` and `DOCKER_TOKEN` secrets in the GitHub `Docker Hub` environment. Check both the [Docker](.github/workflows/docker.yaml) and [binary](.github/workflows/release.yaml) workflows after tagging.

[GPLv3](LICENSE)
