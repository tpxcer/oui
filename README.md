# OUI

OUI 是基于最新 3x-ui 改造的 Xray 面板版本，在保留 3x-ui 协议管理、用户管理、流量统计、节点管理等核心能力的基础上，加入了来自 OUI Panel 的 Telegram 机器人和信息预览能力，让面板更适合快速开节点、观察节点状态和追踪实际使用位置。

> [!IMPORTANT]
> 本项目仅用于个人学习、服务器管理和合法通信场景。请勿用于任何违法用途，也请在正式环境中自行完成安全审计、备份和访问控制配置。

## 功能亮点

- **Telegram 一键节点创建**：在 TG 机器人中快速创建常用节点配置，支持 Hysteria2、VLESS Reality Vision、VLESS XHTTP TLS、VLESS XHTTP Reality 等预设。
- **节点上线提醒开关**：每个入站节点都可以单独设置 TG 上下线提醒，方便按节点粒度管理通知噪音。
- **信息预览页**：将原 3x-ui API 文档入口调整为信息预览，集中展示 VPN 溯源、服务器信息、流量使用情况和资源状态。
- **VPN 溯源查询**：支持通过 IP 查询节点实际使用位置，辅助判断连接来源和使用环境。
- **3x-ui 核心能力保留**：继续支持 Xray-core 管理、入站/客户端管理、订阅、证书、节点、流量、日志和面板设置等能力。

## 快速安装

```bash
bash <(curl -Ls https://raw.githubusercontent.com/tpxcer/oui/main/install.sh)
```

安装完成后按终端提示进入面板。建议首次部署后立即修改默认登录信息、面板路径和安全相关设置。

## 数据库

项目沿用 3x-ui 的数据库能力，支持 SQLite 和 PostgreSQL。

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

- [MHSanaei/3x-ui](https://github.com/MHSanaei/3x-ui)：本项目的基础面板。
- [alireza0](https://github.com/alireza0/)：3x-ui 上游致谢作者。
- OUI Panel：Telegram 一键配置、节点通知和信息预览思路来源。

## 许可

本项目基于 3x-ui 继续遵循 GPL-3.0 许可证。使用、修改和分发时请遵守上游项目与本项目的许可证要求。
