## OUI 2026.7.8

- 修复面板生成 VLESS Reality 分享链接时缺少 `sni` 参数的问题。
- 一键创建的 VLESS Reality Vision / XHTTP Reality 节点，从面板二维码或复制链接导入客户端时，会正确携带 Reality SNI，避免握手失败导致无法连接。
- 同步更新链接生成快照测试，确保后续 Reality 分享链接持续包含 `sni`。
