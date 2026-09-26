# E5SubBot

[English](README.md) · [Docker 工作流](.github/workflows/docker.yaml)

E5SubBot 是通过 Telegram 管理账号、定时调用 Microsoft Graph 的机器人。调用 Graph **不保证** Microsoft 365 E5 订阅一定续期。

项目支持 SQLite、MySQL、账号导出、任务反馈和管理员命令。机器人只处理私聊命令。

## 开始使用

1. 创建 Telegram 机器人和 Microsoft 应用。配置 `Mail.Read`、`User.Read` 委托权限，并将 `http://localhost/e5sub` 设为重定向 URI。授权请求还会使用 `openid` 和 `offline_access` scope。
2. 将 [config.yml.example](config.yml.example) 复制为 `config.yml`，填写 `bot_token`、`admin`、`cron` 和数据库配置。文件中含有凭据，请妥善保管。
3. 通过 Docker 或 Go 二进制启动机器人。在私聊中发送 `/bind`，按提示依次提供应用 ID 与密钥、跳转后的 URL 与账号别名。

## Docker 部署

镜像为 `microcharon/e5subbot`，支持 `linux/amd64` 和 `linux/arm64`。新建 SQLite 部署时，先将 `config.yml` 中的 `sqlite.db` 设为 `/data/data.db`，然后执行：

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

现有部署请保留原来的 `config.yml` 和数据库路径。替换容器或调整挂载前，先备份 `data.db`。

## 二进制与系统服务

可从本仓库的 [Releases](https://github.com/Debcharon/E5SubBot/releases) 下载，或用 `go build -o E5SubBot .` 自行构建。`config.yml` 应放在工作目录。

使用 systemd 时，将二进制放在 `/usr/local/bin/E5SubBot`，将 `config.yml` 放在 `/usr/local/etc/E5SubBot`。创建 `e5subbot` 服务账号，使其能读取配置，并能在工作目录中写入 SQLite 数据库和 `log/` 目录。将以下内容保存为 `/etc/systemd/system/e5subbot.service`：

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

保存后运行 `sudo systemctl daemon-reload` 和 `sudo systemctl enable --now e5subbot`。

`make test` 运行 Go 测试；`make snapshot` 用 GoReleaser 生成本地发布包。

## 发布新版本

版本标签沿用 [tego](https://github.com/Debcharon/tego/releases) 的 `v主版本.年月日.当日序号` 格式。例如 `v1.20260926.0` 表示 2026 年 9 月 26 日（UTC）的首次发布，同日再次发布用 `v1.20260926.1`。换一天后序号从 `0` 开始；出现不兼容的新版本线时再增加主版本号。

改动合并到 `master` 后，给合并后的提交打标签并推送。[Docker 工作流](.github/workflows/docker.yaml)会发布 amd64、arm64 镜像，标签为 `microcharon/e5subbot:<版本标签>` 和 `:latest`。[二进制发布工作流](.github/workflows/release.yaml)会先运行测试，再按 `.goreleaser.yml` 构建归档和校验文件，并创建 GitHub Release。

Docker 任务使用 GitHub 的 `Docker Hub` environment，需要在其中设置 `DOCKER_USERNAME` 和 `DOCKER_TOKEN` 两个 secret。二进制发布使用 GitHub 自动提供的 `GITHUB_TOKEN`。推送标签会独立触发两个工作流，发布前请确认两边均成功。

## 配置项

| 配置 | 用途 |
| --- | --- |
| `bot_token` | Telegram 机器人令牌 |
| `admin` | 可使用 `/task`、`/log` 的 Telegram 用户 ID，逗号分隔 |
| `cron` | 调用 Graph 的五段式定时表达式 |
| `db`、`table` | `sqlite` 或 `mysql`，以及原有账号表名（通常为 `users`） |
| `sqlite.db` | SQLite 数据库路径 |
| `mysql.*` | 使用 MySQL 时的连接配置 |
| `bindmax`、`errlimit`、`goroutine` | 每人绑定上限、连续错误上限、任务并发数 |
| `notice`、`socks5` | 可选的帮助提示与 Telegram SOCKS5 代理 |

运行时可修改 `bindmax`、`errlimit`、`goroutine`、`admin`、`notice`。修改令牌、数据库、代理、定时表达式或表名后需要重启。

## 命令

| 命令 | 功能 |
| --- | --- |
| `/start`、`/help` | 查看帮助与提示 |
| `/bind` | 绑定 Microsoft 账号 |
| `/my`、`/unbind` | 查看或解绑自己的账号 |
| `/export` | 导出含密钥的账号 JSON 文件 |
| `/task`、`/log` | 手动执行任务或下载日志，仅管理员可用 |

任务会保存更新后的 refresh token。账号连续 Graph 调用失败次数超过 `errlimit` 后会自动解绑；请提前备份或导出账号数据。

项目采用 [GPLv3](LICENSE) 许可证。
