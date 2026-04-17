开发版：v1.3.20-dev.4

说明
- 本版聚焦修复 MOD 删除接口和模组列表展示名不一致的问题
- 重点覆盖“手动上传后文件名带时间戳 / 版本后缀时，列表能看到但删除失败”的场景

修复
- 修复 `DELETE /api/mods/:name` 无法按前端显示完整名称删除带后缀 `.tmod` 文件的问题
- 删除逻辑现在兼容完整显示名和旧短名两种请求参数
- 删除成功后会同步清理 `enabled.json` 与 `workshop_mapping.json` 中的对应条目

测试
- GOOS=windows GOARCH=amd64 JWT_SECRET=0123456789abcdef0123456789abcdef go test ./...
- npm run build
- GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o dist/release/v1.3.20-dev.4/terraria-panel .

备注
- 本版本为开发版，仅供测试机验证
- 建议重点回归：手动上传后立即删除、版本后缀文件删除、创意工坊下载模组删除
