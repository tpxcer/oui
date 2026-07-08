## OUI 2026.7.9

- 修复 Telegram 一键创建 VLESS Reality Vision / XHTTP Reality 时使用不稳定 Reality 目标导致新节点无法连接的问题。
- TG 一键 Reality 现在会从已验证可用的目标中选择，并同步写入 `target`、`serverNames` 和 XHTTP `host`，避免 Reality 握手失败。
- 增加回归测试，防止一键 Reality 再次回退到已知不可用目标。
