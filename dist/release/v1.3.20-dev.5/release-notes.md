开发版：v1.3.20-dev.5

说明
- 本版主要清理前端国际化文件中的重复键，避免构建输出继续出现 duplicate key 警告
- 同时同步更新当前进度文档，记录已验证通过和暂定项

修复
- 修复 `pluginServer` 文案对象中的重复 `passwordPlaceholder` 键
- 插件服快速配置弹窗改用独立的 `serverPasswordPlaceholder` 文案键

优化
- 前端构建输出已不再出现 locale duplicate key 警告
- 同步整理修复进度文档中的实际验证状态

测试
- npm run build
- GOOS=windows GOARCH=amd64 JWT_SECRET=0123456789abcdef0123456789abcdef go test ./...
- GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o dist/release/v1.3.20-dev.5/terraria-panel .

备注
- 本版本为开发版，仅供测试机验证
- 当前剩余构建警告为 Browserslist 数据过旧和 Tailwind ambiguous class，不影响本次功能
