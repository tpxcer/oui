## OUI 2026.7.9-3

- 修复 Hysteria2 端口跳跃在 Clash/Mihomo 订阅中缺少 `ports` 字段的问题。
- 使用 Clash/Mihomo 类客户端导入一键创建的 Hysteria2 节点时，会正确携带端口跳跃范围，避免客户端只使用基础端口导致连接失败。
- 增加回归测试，覆盖启用和未启用端口跳跃两种 Hysteria2 YAML 输出。
