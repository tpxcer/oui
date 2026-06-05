# OUI

OUI 是一个面向 Linux VPS 的 Xray 管理面板，重点放在“快速创建可用节点、集中查看服务器状态、通过 Telegram 远程管理”和“中文化的日常运维体验”上。项目保留入站、客户端、订阅、证书、流量、日志和 Xray-core 管理等面板基础能力，并加入一键节点创建、Telegram 机器人、VPN 溯源、服务器商信息预览和网页一键更新等功能。

> [!IMPORTANT]
> 本项目仅用于个人学习、服务器管理和合法通信场景。请勿用于任何违法用途。正式部署前请自行完成安全审计、数据备份、访问控制、服务器防火墙和账号权限配置。

## 项目定位

OUI 适合这些使用场景：

- 在自己的 VPS 上快速部署 Xray 面板。
- 需要通过网页或 Telegram 快速创建常用节点。
- 需要查看服务器流量、套餐、IP、到期或重置等信息。
- 需要通过 IP 查询节点实际使用位置，辅助排查异常连接。
- 需要中文安装脚本、中文面板和更直接的一键更新流程。

OUI 当前以 Linux 环境为主，不提供 Windows 版安装包或 Windows 专用脚本。

## 功能亮点

### 一键节点创建

入站列表支持一键创建常用节点，目标是尽量减少手动填写复杂 Xray 配置的步骤。

已支持的预设：

- Hysteria2
- VLESS Reality Vision
- VLESS XHTTP TLS
- VLESS XHTTP Reality

一键创建时会自动选择 `50000-65000` 高位端口，并尝试自动放行防火墙端口。若服务器没有检测到 `ufw` 或 `firewalld`，面板会提示需要手动放行端口。

### Telegram 机器人

OUI 集成 Telegram 机器人能力，用于远程管理和通知。

主要能力：

- Telegram 一键创建节点。
- Telegram 返回订阅信息。
- 入站节点可单独设置上线通知开关。
- 支持登录通知、CPU 通知、备份通知等提醒。
- 已保存过机器人 API 时，可以直接沿用旧配置启用机器人。
- 首次启用机器人时，机器人 API 和聊天 ID 必填。

设置位置：

```text
设置 -> Telegram
```

### 信息预览

OUI 将原来的 API 文档入口调整为信息预览，更适合日常查看状态。

信息预览主要包括：

- VPN 溯源：通过 IP 查询节点实际使用位置。
- 服务器信息：展示服务器商返回的套餐、节点位置、流量、重置时间、IP、PTR 等。
- 流量用量：以条形对比图展示月流量和已使用流量。
- 资源概览：辅助查看服务器运行状态和面板状态。

### VPN 溯源

VPN 溯源用于通过 IP 查询节点实际使用位置，方便排查：

- 节点出口是否符合预期。
- 客户端实际连接位置。
- IP 查询结果是否异常。

地址显示会尽量拼接为完整中文地址，例如：

```text
地址：中国台湾省彰化县埔盐乡
```

### 自定义服务器商信息

服务器信息不限定某一家服务商。你可以在设置中自定义服务器商名称、接口地址、VEID 和 API KEY，让面板按配置拉取服务器信息。

适合这类接口：

- 返回 JSON 的服务器商 API。
- 兼容 64Clouds / KiwiVM 风格字段的 API。
- 自己搭建的服务器信息中转接口。

常见展示内容：

- 服务器商名称
- 主机名
- 节点位置
- 套餐名称
- 月流量
- 已用流量
- 流量重置时间
- 磁盘、内存、交换分区
- IP 地址
- PTR 信息

### 订阅服务

OUI 保留订阅服务能力，支持按客户端生成订阅信息，并可配合一键节点创建快速交付配置。

常见能力：

- 客户端订阅链接。
- Clash 订阅。
- JSON 订阅。
- 订阅路径、端口、证书、域名配置。
- 客户端流量与有效期信息展示。

### 网页一键更新

面板支持检测 GitHub Release 最新版本，并在发现新版本时从页面发起后台更新。

更新流程：

1. 面板检测当前版本和最新版本。
2. 点击更新后进入后台更新状态。
3. 更新脚本下载对应版本安装包。
4. 面板服务重启。
5. 页面自动等待服务恢复并刷新当前页面。

版本号使用发布日期格式，例如：

```text
2026.5.29
2026.5.29-1
2026.5.29-2
```

同一天多次发布时，后缀会递增。

### IP 限制与 Fail2Ban 自动对接

OUI 支持按客户端限制可使用的 IP 数量。客户端配置中的 `IP Limit` 大于 `0` 时，后台任务会持续读取 Xray access log，记录该客户端最近出现过的 IP，并按时间顺序保留最早进入的 IP。后续不同 IP 超出上限时，会触发以下处理：

- 将超限 IP 写入 `/var/log/x-ui/3xipl.log`。
- 临时移除并重新添加该客户端，迫使现有连接断开。
- 如果 Telegram 机器人已配置管理员通知，会发送“超出 IP 上限，已掐断”的提醒。
- 如果系统已启用 Fail2Ban 的 `3x-ipl` jail，会立即把超限 IP 加入防火墙封禁。

安装脚本和更新脚本会自动尝试配置 Fail2Ban/IP Limit jail。自动配置会创建或更新这些文件：

```text
/etc/fail2ban/jail.d/3x-ipl.conf
/etc/fail2ban/filter.d/3x-ipl.conf
/etc/fail2ban/action.d/3x-ipl.conf
/var/log/x-ui/3xipl.log
/var/log/x-ui/3xipl-banned.log
```

自动配置失败不会中断 OUI 安装或更新。失败后可以手动运行：

```bash
x-ui install-iplimit
```

也可以进入管理菜单：

```text
x-ui -> 21. IP 限制管理 -> 1. Install Fail2ban and configure IP Limit
```

检查状态：

```bash
fail2ban-client status 3x-ipl
tail -f /var/log/x-ui/3xipl-banned.log
```

如果服务器环境不希望安装或改动 Fail2Ban，可在安装或更新前设置：

```bash
export XUI_SKIP_IPLIMIT_SETUP=true
```

注意：OUI 程序本身可以识别超限 IP 并断开客户端连接；防火墙级封禁依赖 Fail2Ban 和 `3x-ipl` jail 正常运行。若服务器使用云厂商安全组，还需要确认业务端口允许正常访问，面板端口按需限制来源。

## 快速安装

在 Linux VPS 上执行：

```bash
bash <(curl -Ls https://raw.githubusercontent.com/tpxcer/oui/main/install.sh)
```

安装完成后，终端会显示面板访问地址。首次进入面板后建议立即完成：

- 修改默认账号和密码。
- 修改面板访问路径。
- 配置 HTTPS 证书。
- 配置防火墙和安全组。
- 备份数据库。

## 更新

命令行更新：

```bash
x-ui update
```

或直接运行：

```bash
bash <(curl -Ls https://raw.githubusercontent.com/tpxcer/oui/main/update.sh)
```

网页更新：

```text
面板左下角版本区域 -> 检测更新 / 一键更新
```

## 数据库

OUI 支持 SQLite 和 PostgreSQL。

### SQLite

默认方案，适合轻量部署。

常见数据库位置：

```text
/etc/x-ui/x-ui.db
```

### PostgreSQL

适合更多客户端、更高并发或需要独立数据库管理的场景。

环境变量示例：

```bash
XUI_DB_TYPE=postgres
XUI_DB_DSN=postgres://xui:password@127.0.0.1:5432/xui?sslmode=disable
```

## Docker

默认 Docker Compose 使用 SQLite：

```bash
docker compose up -d
```

如果需要 PostgreSQL，可参考 `docker-compose.yml` 中的注释启用 `postgres` profile。

## 常用目录

常见安装路径：

```text
/usr/local/x-ui
/etc/x-ui
/etc/systemd/system/x-ui.service
```

常用命令：

```bash
x-ui
x-ui status
x-ui restart
x-ui update
x-ui log
x-ui install-iplimit
```

## 开发与验证

前端：

```bash
cd frontend
npm ci
npm run typecheck
npm run lint
npm test
npm run build
```

后端：

```bash
go test ./...
```

生成前端 OpenAPI 数据：

```bash
cd frontend
npm run gen:api
```

## 安全建议

部署后建议至少完成这些配置：

- 使用强密码。
- 修改默认面板路径。
- 开启 HTTPS。
- 限制面板端口访问来源。
- 定期备份 `/etc/x-ui/x-ui.db`。
- 不要把 Telegram token、服务器商 API KEY、VEID、订阅链接提交到公开仓库。
- 更新前先确认当前数据库已经备份。

## 鸣谢

- OUI Panel：Telegram 一键配置、节点通知和信息预览思路来源。
- Xray-core：核心代理能力来源。
- 相关开源社区：协议、订阅、前端组件和运维脚本生态支持。

## 许可

本项目继续遵循 GPL-3.0 许可证。使用、修改和分发时请遵守相关开源许可证要求。
