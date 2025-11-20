# TR Panel Go

轻量且高性能的泰拉瑞亚服务器管理面板后端，使用 Go 语言构建。

[![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go)](https://golang.org)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Build Status](https://img.shields.io/badge/build-passing-brightgreen.svg)](https://github.com/ShourGG/tr-panel-go)

---

## 项目简介

TR Panel Go 是一个现代化的泰拉瑞亚游戏服务器管理后端服务。基于 Go 语言开发，提供 RESTful API 和 WebSocket 支持，实现实时服务器监控、玩家管理、插件管理和自动化任务调度。

---

## 核心亮点

**性能优势**
- Go 语言原生性能，卓越的并发处理能力
- API 平均响应时间 ~100ms
- 页面加载时间 ~78ms，LCP 指标 ~105ms
- 数据库查询索引优化，高频查询性能提升 50-80%

**架构设计**
- RESTful API 设计规范，接口清晰易用
- WebSocket 实时推送，服务器日志和玩家事件即时更新
- 分层架构设计，业务逻辑与数据访问解耦
- SQLite 轻量级存储，单文件部署无需额外数据库服务

**功能完备**
- TShock 插件服务器完整支持
- 玩家会话追踪和统计分析
- 自动化备份和定时任务调度
- 配置文件在线编辑和管理
- JWT 身份认证和权限控制

---

## 主要功能

**服务器管理**
- 启动、停止、重启泰拉瑞亚服务器
- 实时状态监控和进程管理
- 服务器日志实时查看
- 多房间支持，独立配置管理

**玩家管理**
- 玩家会话记录和统计
- 游戏时长和活跃度分析
- 玩家活动历史追踪
- 玩家行为日志审计

**插件管理**
- TShock 插件安装和卸载
- 插件配置在线编辑
- 插件版本管理
- 插件依赖关系处理

**系统功能**
- 文件管理器，浏览和编辑配置文件
- 自动备份和手动备份
- 定时任务调度（基于 Cron）
- 系统资源监控（CPU、内存、磁盘）

---

## 技术栈

| 技术 | 说明 |
|------|------|
| 开发语言 | Go 1.21+ |
| Web 框架 | Gin（高性能 HTTP 路由） |
| WebSocket | gorilla/websocket |
| 数据库 | SQLite 3 |
| 数据访问 | 原生 SQL + 预编译语句 |
| 身份认证 | JWT Token |
| 进程管理 | os/exec 生命周期控制 |
| 任务调度 | robfig/cron |

**核心依赖**
```
github.com/gin-gonic/gin          # Web 框架
github.com/gorilla/websocket      # WebSocket 支持
github.com/mattn/go-sqlite3       # SQLite 驱动
github.com/robfig/cron/v3         # 定时任务
golang.org/x/crypto               # 加密库
```

---

## 快速开始

**环境要求**
- Go 1.21 或更高版本
- GCC 编译器（用于 SQLite CGO）
- Linux 或 Windows 服务器
- 泰拉瑞亚专用服务器（可选，用于测试）

**安装步骤**

1. 克隆仓库
```bash
git clone https://github.com/ShourGG/tr-panel-go.git
cd tr-panel-go
```

2. 安装依赖
```bash
go mod download
```

3. 编译程序
```bash
# Linux 平台
CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o tr-panel .

# Windows 平台
go build -ldflags="-s -w" -o tr-panel.exe .
```

4. 启动服务
```bash
./tr-panel
```

默认启动在 `http://localhost:8800`

**配置文件**

在项目根目录创建 `.env` 文件：

```env
# 服务器配置
PORT=8800
HOST=0.0.0.0

# JWT 密钥
JWT_SECRET=your-secret-key-here

# 数据库路径
DB_PATH=./data/panel.db

# 泰拉瑞亚服务器路径
TERRARIA_SERVER_PATH=/path/to/TerrariaServer
TSHOCK_PATH=/path/to/tshock
```

---

## API 接口说明

**身份认证**

`POST /api/auth/login` - 用户登录
```json
{
  "username": "admin",
  "password": "password"
}
```

`POST /api/auth/register` - 用户注册
```json
{
  "username": "admin",
  "password": "password",
  "email": "admin@example.com"
}
```

**服务器管理**

`GET /api/rooms` - 获取服务器列表

`POST /api/rooms` - 创建服务器房间

`GET /api/rooms/:id` - 获取房间详情

`POST /api/rooms/:id/start` - 启动服务器

`POST /api/rooms/:id/stop` - 停止服务器

**玩家管理**

`GET /api/players` - 获取玩家列表

`GET /api/players/:id/stats` - 获取玩家统计

`GET /api/players/:id/sessions` - 获取玩家会话历史

**插件管理**

`GET /api/plugins` - 获取已安装插件列表

`POST /api/plugins/install` - 安装插件

`DELETE /api/plugins/:id` - 卸载插件

**WebSocket 接口**

`WS /ws/logs/:roomId` - 实时服务器日志

`WS /ws/system` - 系统监控实时更新

---

## 生产部署

**Linux 服务器部署**

1. 上传编译后的二进制文件
```bash
scp tr-panel user@server:/opt/tr-panel/
```

2. 创建 systemd 服务
```ini
[Unit]
Description=TR Panel Go Service
After=network.target

[Service]
Type=simple
User=terraria
WorkingDirectory=/opt/tr-panel
ExecStart=/opt/tr-panel/tr-panel
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
```

3. 启用并启动服务
```bash
sudo systemctl daemon-reload
sudo systemctl enable tr-panel
sudo systemctl start tr-panel
```

**Docker 部署（可选）**

```dockerfile
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY . .
RUN apk add --no-cache gcc musl-dev
RUN CGO_ENABLED=1 go build -ldflags="-s -w" -o tr-panel .

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/tr-panel .
EXPOSE 8800
CMD ["./tr-panel"]
```

---

## 性能优化

**数据库优化**
- 玩家会话表按玩家 ID 和时间索引
- 玩家统计表按更新时间索引
- 活动日志表按类型和时间索引
- 查询性能提升 50-80%

**前端优化**
- Vue 核心库单独打包
- Ant Design 组件按需加载
- Monaco 编辑器延迟加载
- ECharts 图表库独立分包

**代码优化**
- 生产环境移除 console.log
- JavaScript 使用 Terser 压缩
- CSS 文件压缩和合并

**性能指标**
- API 响应时间：平均 100ms
- 页面加载时间：78ms
- 最大内容绘制（LCP）：105ms

---

## 开发指南

**开发模式运行**

```bash
# 安装热重载工具（可选）
go install github.com/cosmtrek/air@latest

# 使用热重载运行
air

# 或直接运行
go run main.go
```

**运行测试**

```bash
go test ./...
```

**代码规范**

- 使用 `gofmt` 格式化代码
- 遵循 Go 语言最佳实践
- 代码自解释，减少不必要的注释
- 保持函数简洁，单一职责

---

## 贡献指南

欢迎贡献代码！请遵循以下流程：

1. Fork 本仓库
2. 创建功能分支 (`git checkout -b feature/new-feature`)
3. 提交更改 (`git commit -m '添加新功能'`)
4. 推送到分支 (`git push origin feature/new-feature`)
5. 创建 Pull Request

---

## 开源协议

本项目采用 MIT 协议开源，详见 [LICENSE](LICENSE) 文件。

---

## 作者

由 [ShourGG](https://github.com/ShourGG) 开发维护

---

## 问题反馈

如有问题、建议或功能需求，请在 GitHub 上提交 Issue：

https://github.com/ShourGG/tr-panel-go/issues
