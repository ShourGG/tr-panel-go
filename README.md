# TR Panel Go

轻量且高性能的泰拉瑞亚服务器管理面板后端，使用 Go 语言构建。

[![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go)](https://golang.org)
[![License](https://img.shields.io/badge/license-CC%20BY--NC%204.0-lightgrey.svg)](LICENSE)
[![GitHub Stars](https://img.shields.io/github/stars/ShourGG/tr-panel-go?style=flat&logo=github)](https://github.com/ShourGG/tr-panel-go/stargazers)
[![GitHub Forks](https://img.shields.io/github/forks/ShourGG/tr-panel-go?style=flat&logo=github)](https://github.com/ShourGG/tr-panel-go/network)
[![GitHub Issues](https://img.shields.io/github/issues/ShourGG/tr-panel-go?style=flat&logo=github)](https://github.com/ShourGG/tr-panel-go/issues)
[![GitHub Last Commit](https://img.shields.io/github/last-commit/ShourGG/tr-panel-go?style=flat&logo=github)](https://github.com/ShourGG/tr-panel-go/commits)
[![Code Size](https://img.shields.io/github/languages/code-size/ShourGG/tr-panel-go?style=flat)](https://github.com/ShourGG/tr-panel-go)
[![Top Language](https://img.shields.io/github/languages/top/ShourGG/tr-panel-go?style=flat)](https://github.com/ShourGG/tr-panel-go)

---

## 项目简介

泰拉瑞亚游戏服务器管理后端，提供 RESTful API 和 WebSocket 实时通信，支持服务器监控、玩家管理、插件管理和自动化任务调度。

---

## 核心特性

**性能**
- Go 原生高性能，API 响应 ~100ms
- 数据库索引优化，查询性能提升 50-80%
- 页面加载 ~78ms，LCP ~105ms

**架构**
- RESTful API + WebSocket 实时推送
- 分层设计，业务与数据解耦
- SQLite 轻量存储，单文件部署

**功能**
- TShock 插件服务器支持
- 玩家统计和会话追踪
- 自动备份和定时任务
- JWT 认证和权限控制

---

## 主要功能

**服务器管理**：启停控制、实时监控、日志查看、多房间支持

**玩家管理**：会话记录、统计分析、活动追踪、行为审计

**插件管理**：安装卸载、配置编辑、版本管理、依赖处理

**系统功能**：文件管理、备份恢复、定时任务、资源监控

---

## 技术栈

| 技术 | 说明 |
|------|------|
| 语言 | Go 1.21+ |
| 框架 | Gin + gorilla/websocket |
| 数据库 | SQLite 3 |
| 认证 | JWT Token |
| 调度 | robfig/cron |

---

## 性能优化

**数据库**：多表索引优化，查询性能提升 50-80%

**前端**：代码分包（Vue/Ant Design/Monaco/ECharts）

**代码**：移除 console.log，Terser 压缩

---

## 开源协议

本项目采用 CC BY-NC 4.0 协议，详见 [LICENSE](LICENSE) 文件。

**禁止商业使用** - 不得用于盈利项目或商业产品

---

## 作者

由 [ShourGG](https://github.com/ShourGG) 开发维护

---

## 问题反馈

https://github.com/ShourGG/tr-panel-go/issues
