开发版：v1.3.20-dev.2

说明
- 本版继续完善备份与文件下载链路，目标是兼顾安全、速度和部署成本
- 下载方案升级为 A+B：短时下载票据 + 代理层直出，同时保留无代理回退

新增
- 新增备份下载票据接口
- 新增文件管理下载票据接口
- 新增统一下载入口，下载链接只携带短时票据

优化
- 支持 Nginx `X-Accel-Redirect` 直出本地现成文件
- 未配置代理直出时，自动回退到 Go 直接下载
- 文件管理打包下载继续保持 Go 动态 zip，不强依赖代理层
- 修正压缩下载文件名，避免 `.zip.zip`

测试
- GOOS=windows GOARCH=amd64 CGO_ENABLED=0 JWT_SECRET=0123456789abcdef0123456789abcdef go test ./...
- npm run build
- build-linux.bat

备注
- 本版本为开发版，仅供测试机验证
- Nginx 直出为可选能力，不配置也能正常下载
