## OUI 2026.7.9-2

- 修复 VLESS Reality / XHTTP Reality 链接偶发无法连接的问题：订阅和单独链接现在固定使用第一个主 SNI，不再随机选到不可用备用 SNI。
- TG 一键 Reality 目标只写入已验证的主 SNI，避免服务端配置包含会导致 Reality 握手失败的备用域名。
- 增加回归测试，覆盖普通订阅链接和 JSON 订阅的 Reality SNI 选择规则。
