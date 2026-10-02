# E5SubBot

[English](README.md)

通过 Telegram 管理账号、定时调用 Microsoft Graph 的机器人，支持 SQLite/MySQL 和账号导出。命令仅限私聊；Graph 调用**不保证** Microsoft 365 E5 订阅续期。

## 开始使用

1. 创建 Telegram 机器人和 Microsoft 应用，配置 `Mail.Read`、`User.Read` 委托权限，重定向 URI 设为 `http://localhost/e5sub`。
2. 将 [config.yml.example](config.yml.example) 复制为 `config.yml`，填写机器人令牌、管理员 ID、定时任务和数据库配置。
3. 启动后发送 `/bind`，按提示提供应用凭据、授权回调 URL 和账号别名。

## Docker

镜像：`microcharon/e5subbot`，支持 `linux/amd64`、`linux/arm64`。使用 SQLite 时，设置 `sqlite.db: /data/data.db`：

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

升级时保留现有数据库路径，并提前备份数据库。

## 二进制

从 [Releases](https://github.com/Debcharon/E5SubBot/releases) 下载，或用 `go build -o E5SubBot .` 构建。支持 Linux amd64/arm64、Windows amd64、macOS amd64/arm64。在 `config.yml` 所在目录运行，使用 `--version` 查看构建版本。

<details>
<summary>使用 systemd</summary>

二进制放在 `/usr/local/bin/E5SubBot`，配置放在 `/usr/local/etc/E5SubBot/config.yml`。创建 `e5subbot` 用户，确保能读取配置、写入数据库和 `log/` 目录。将以下内容保存为 `/etc/systemd/system/e5subbot.service`：

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

## 配置与命令

配置见 [config.yml.example](config.yml.example)。`bindmax`、`errlimit`、`goroutine`、`admin`、`notice` 支持热更新，其余修改需要重启。设置 `log_stdout_only: true` 后只输出到标准输出，`/log` 不再提供文件。

| 命令 | 功能 |
| --- | --- |
| `/start`、`/help` | 查看帮助 |
| `/bind`、`/cancel` | 开始或取消绑定；会话 15 分钟后过期 |
| `/my`、`/unbind` | 查看或解绑自己的账号 |
| `/export` | 导出账号数据，包含完整凭据 |
| `/task`、`/status`、`/log` | 执行任务、查看状态、下载日志，仅管理员可用 |

任务失败不会自动删除账号。连续授权错误超过 `errlimit` 后提示重新授权：先用 `/export` 备份，再通过 `/unbind`、`/bind` 重新绑定。配置和导出文件含有凭据，请妥善保管。

## 发布

合并到 `master` 后推送 `v主版本.年月日.当日序号` 标签，例如 `v1.20260926.0`（UTC 日期，当日序号从 `0` 开始）。CI 通过后，自动发布 GitHub Release 归档及校验文件、Docker 版本镜像和 `latest`。

在 GitHub 的 `Docker Hub` environment 中设置 `DOCKER_USERNAME`、`DOCKER_TOKEN` 两个 secret。打标签后检查 [Docker](.github/workflows/docker.yaml) 和 [二进制](.github/workflows/release.yaml) 工作流。

[GPLv3](LICENSE)
