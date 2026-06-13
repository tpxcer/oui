# OUI

OUI 是一个面向 Linux VPS 的 Xray 管理面板，重点放在“快速创建可用节点、集中查看服务器状态、通过 Telegram 远程管理”和“中文化的日常运维体验”上。项目保留入站、客户端、订阅、证书、流量、日志和 Xray-core 管理等基础能力，并加入一键节点创建、Telegram 上下线/IP 限制通知、IP 限制自动对接、公网归属信息、服务器信息预览、网页一键更新和中文版本更新说明等功能。

> [!IMPORTANT]
> 本项目仅用于个人学习、服务器管理和合法通信场景。请勿用于任何违法用途。正式部署前请自行完成安全审计、数据备份、访问控制、服务器防火墙和账号权限配置。

## 项目定位

OUI 适合这些使用场景：

- 在自己的 VPS 上快速部署 Xray 面板。
- 需要通过网页或 Telegram 快速创建常用节点。
- 需要查看服务器流量、套餐、IP、到期、重置和公网归属等信息。
- 需要通过 IP 归属信息辅助排查节点出口、异常连接或服务商信息。
- 需要限制单个客户端可同时使用的 IP 数量，并只掐断超限 IP。
- 需要中文安装脚本、中文面板、更直接的一键更新流程和每版中文更新说明。

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
- 入站节点可单独设置上线/下线通知开关。
- 上线通知可显示 IP 地址和归属地；同一节点多个 IP 会按上线时间展示为 `节点名称(ip2)`、`节点名称(ip3)`。
- 下线通知会显示离线 IP、在线时长、下线时间和本次流量。
- IP 超限时会发送“超出 IP 上限，已掐断”通知，并附带保留 IP、掐断 IP 和快捷按钮。
- TG 中可直接临时解封，或进入 `设置IP数量` 调整该客户端允许的 IP 数量。
- TG 更新菜单会显示最新版号和本版本中文更新内容；从 TG 发起更新后，开始更新和更新成功反馈只显示更新状态，不重复展示更新内容。
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

- 公网归属信息：通过 IP 查询归属地、运营商和经纬度。
- 服务器信息：展示服务器商返回的套餐、节点位置、流量、重置时间、IP、PTR 等。
- 流量用量：以条形对比图展示月流量和已使用流量。
- 资源概览：辅助查看服务器运行状态和面板状态。

### 公网归属信息

公网归属信息用于通过 IP 查询归属地和运营商，方便排查：

- 节点出口是否符合预期。
- 客户端实际连接位置。
- IP 查询结果是否异常。

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

### 网页一键更新与中文更新说明

面板支持检测 GitHub Release 最新版本，并在发现新版本时从页面发起后台更新。每个正式版本都应维护中文更新说明，面板检测更新、网页一键更新确认弹窗和 Telegram 更新菜单会同步展示本版本更新内容；Telegram 后台更新开始和更新成功反馈只显示更新状态。

更新流程：

1. 面板检测当前版本、最新版本和 GitHub Release 中文说明。
2. 点击更新前确认本版本更新内容。
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

发布前请同步更新：

```text
CHANGELOG.md
config/release-notes/latest.md
```

GitHub Actions 会在 tag 发布后自动把 `config/release-notes/latest.md` 写入对应 Release 正文，供面板和 Telegram 读取展示。

### IP 限制与 Fail2Ban 自动对接

OUI 支持按客户端限制可使用的 IP 数量。客户端配置中的 `IP Limit` 大于 `0` 时，后台任务会持续读取 Xray access log，记录该客户端最近出现过的 IP，并按首次上线时间保留较早进入的 IP。后续不同 IP 超出上限时，会触发以下处理：

- 将超限 IP 写入 `/var/log/x-ui/3xipl.log`。
- 只针对“超限 IP + 当前入站端口”写入防火墙规则。
- 如果 Telegram 机器人已配置管理员通知，会发送“超出 IP 上限，已掐断”的提醒。
- 不删除 Xray 客户端，不封整台 VPS，也不影响同一 IP 访问面板、SSH 或其它节点端口。

IP 限制的同步规则：

- `IP Limit = 0` 时，会立即解除该客户端已有的 IP 限制封禁。
- 节点离线或关闭时，会解除该节点端口上的对应封禁。
- IP 限制数增加时，会按首次上线时间解封现在应允许的 IP。
- 限制为 `1` 时，第 2 个不同 IP 属于超限；限制为 `3` 时，第 4 个不同 IP 属于超限。

Telegram 快捷按钮规则：

- `临时解封 1/6/24 小时`：只在指定时长内放行该 IP，不修改面板 IP 限制；到期后会重新封禁。
- `设置IP数量`：进入 Telegram 的 IP 限制数量设置菜单，由管理员选择新的允许 IP 数量。
- 临时解封成功、失败和到期重新封禁都会发送 Telegram 反馈。

安装脚本和更新脚本会自动尝试配置 IP Limit 所需的 Fail2Ban 监控和 `f2b-3x-ipl` 防火墙链。自动配置会创建或更新这些文件：

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

注意：OUI 程序本身负责识别超限 IP 并写入端口级防火墙规则；Fail2Ban 作为日志监控和辅助状态组件，不执行全端口封禁。若服务器使用云厂商安全组，还需要确认业务端口允许正常访问，面板端口按需限制来源。

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
