## OUI 2026.6.15

- 修复命令行证书设置不一致：`x-ui setting -webCert/-webCertKey` 现在会真正写入面板证书路径，仍兼容 `x-ui cert -webCert/-webCertKey`。
- 修复 Web 访问路径带尾斜杠时可能访问异常的问题：内部统一按不带尾斜杠保存，路由兼容 `/abc`、`/abc/` 和 `/abc/panel/`。
- 优化更新脚本的 SSL 提示：证书申请失败或跳过 SSL 时不再误显示 HTTPS 成功信息。
- 优化更新脚本 SSL 配置选项，更新时也可以明确选择跳过 SSL 并绑定到本机地址。
- 优化防火墙管理：按 UFW 规则序号批量删除时改为倒序删除，避免序号变化导致误删。
- 优化 IP Limit 卸载流程：完整卸载 Fail2Ban 前增加明确二次确认。
