# OUI

OUI 是面向 Linux VPS 的 Xray 管理面板，保留协议管理、用户管理、流量统计、节点管理、订阅、证书和日志等核心能力，并加入 Telegram 机器人、一键节点创建、IP 限制自动对接、公网归属信息、信息预览、网页一键更新和中文版本说明能力，让面板更适合快速开节点、观察节点状态和处理异常连接。

> [!IMPORTANT]
> 本项目仅用于个人学习、服务器管理和合法通信场景。请勿用于任何违法用途，也请在正式环境中自行完成安全审计、备份和访问控制配置。

## 功能亮点

- **面板/TG 一键节点创建**：在入站列表或 TG 机器人中快速创建常用节点配置，支持 Hysteria2、VLESS Reality Vision、VLESS XHTTP TLS、VLESS XHTTP Reality，并自动选择 `50000-65000` 高位端口、尝试放行防火墙。
- **节点上线/下线提醒**：每个入站节点都可以单独设置 TG 上下线提醒；上线可显示 IP 与归属地，同一节点多个 IP 会按上线时间展示为 `节点名称(ip2)`、`节点名称(ip3)`，离线通知会显示离线 IP、在线时长和本次流量。
- **IP 限制自动对接**：安装和更新时会自动尝试启用 Fail2Ban/IP Limit 监控和防火墙链。客户端超过 IP 上限时，只封“超限 IP + 当前节点端口”，不删除 Xray 客户端，不封整台 VPS。
- **TG 封禁操作反馈**：IP 超限通知中可临时解封，或进入 `设置IP数量` 调整该客户端允许的 IP 数量；临时解封成功、失败和到期重新封禁都会发送 TG 反馈。
- **信息预览页**：将 API 文档入口调整为信息预览，集中展示公网归属信息、服务器信息、流量使用情况和资源状态。
- **公网归属查询**：支持通过 IP 查询归属地、运营商和经纬度，辅助排查节点出口和异常连接。
- **服务器商信息**：设置中可配置自定义拉取链接、VEID 与 API KEY，在服务器信息中展示套餐、节点位置、流量用量、重置时间、IP 和 PTR。
- **中文安装与一键更新**：安装/更新主流程中文化，面板左下角使用日期版本号，并提供版本检测、后台一键更新和本版本中文更新内容展示。
- **中文发布说明**：每个正式版本都应更新 `CHANGELOG.md` 和 `config/release-notes/latest.md`；GitHub Release、面板检测更新、网页一键更新弹窗和 Telegram 更新菜单会同步展示本版本更新内容。
- **核心能力保留**：继续支持 Xray-core 管理、入站/客户端管理、订阅、证书、节点、流量、日志和面板设置等能力。

## 快速安装

```bash
bash <(curl -Ls https://raw.githubusercontent.com/tpxcer/oui/main/install.sh)
```

安装完成后按终端提示进入面板。建议首次部署后立即修改默认登录信息、面板路径和安全相关设置。

## IP 限制与 Fail2Ban

客户端的 `IP 限制` 设置为大于 `0` 后，OUI 会读取 Xray access log，按客户端记录最近出现过的 IP。超过上限时，系统会优先保留较早上线的 IP，并对后续超限 IP 执行以下动作：

- 写入 `/var/log/x-ui/3xipl.log`。
- 只针对“超限 IP + 当前入站端口”写入防火墙规则。
- 已配置 Telegram 管理员通知时，发送“超出 IP 上限，已掐断”。
- 不删除 Xray 客户端，不封整台 VPS，也不影响同一 IP 访问面板、SSH 或其它节点端口。

同步规则：

- `IP 限制 = 0` 时，会立即解除该客户端已有的 IP 限制封禁。
- 节点离线或关闭时，会解除该节点端口上的对应封禁。
- IP 限制数增加时，会按首次上线时间解封现在应允许的 IP。
- 限制为 `1` 时，第 2 个不同 IP 属于超限；限制为 `3` 时，第 4 个不同 IP 属于超限。

Telegram 快捷按钮：

- `临时解封 1/6/24 小时`：只在指定时长内放行该 IP，不修改面板 IP 限制；到期后会重新封禁。
- `设置IP数量`：进入 Telegram 的 IP 限制数量设置菜单，由管理员选择新的允许 IP 数量。
- 临时解封成功、失败和到期重新封禁都会发送 Telegram 反馈。

安装脚本和更新脚本会自动尝试配置 IP Limit 所需的 Fail2Ban 监控和 `f2b-3x-ipl` 防火墙链，涉及文件如下：

```text
/etc/fail2ban/jail.d/3x-ipl.conf
/etc/fail2ban/filter.d/3x-ipl.conf
/etc/fail2ban/action.d/3x-ipl.conf
/var/log/x-ui/3xipl.log
/var/log/x-ui/3xipl-banned.log
```

如果自动配置失败，不会中断 OUI 安装或更新。可以稍后手动执行：

```bash
x-ui install-iplimit
```

检查状态：

```bash
fail2ban-client status 3x-ipl
tail -f /var/log/x-ui/3xipl-banned.log
```

不希望安装脚本自动配置 Fail2Ban 时，可在执行前设置：

```bash
export XUI_SKIP_IPLIMIT_SETUP=true
```

说明：OUI 负责识别超限 IP 并写入端口级防火墙规则；Fail2Ban 作为日志监控和辅助状态组件，不执行全端口封禁。

## 数据库

项目支持 SQLite 和 PostgreSQL。

- **SQLite**：默认方案，数据库文件通常位于 `/etc/x-ui/x-ui.db`，适合轻量部署。
- **PostgreSQL**：适合更多客户端、更高并发或需要独立数据库管理的场景。

PostgreSQL 环境变量示例：

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

## 鸣谢

- alireza0：上游致谢作者。
- OUI Panel：Telegram 一键配置、节点通知和信息预览思路来源。

## 许可

本项目继续遵循 GPL-3.0 许可证。使用、修改和分发时请遵守相关开源许可证要求。
