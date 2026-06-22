## OUI 2026.6.23

- 修复前端依赖安全扫描红灯：升级 `vite` 到 `8.0.16`，修复 Vite 相关高危审计告警。
- 升级 `swagger-ui-react` 到 `5.32.7`，同步带入 `dompurify 3.4.11` 和 `js-yaml 4.2.0`，清理 Swagger/API 文档相关中危审计告警。
- 更新锁定依赖 `form-data` 到 `4.0.6`，修复 CRLF 注入高危审计告警；当前 `npm audit` 已恢复为 0 漏洞。
