# TR Panel Go

A lightweight and high-performance Terraria server management panel backend built with Go.

[![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go)](https://golang.org)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Build Status](https://img.shields.io/badge/build-passing-brightgreen.svg)](https://github.com/ShourGG/tr-panel-go)

## Overview

TR Panel Go is a modern, high-performance backend service for managing Terraria game servers. Built with Go, it provides RESTful APIs and WebSocket support for real-time server monitoring, player management, plugin administration, and automated task scheduling.

**Key Highlights:**
- Written in Go for superior performance and concurrency
- RESTful API design with WebSocket for real-time updates
- Support for TShock plugin servers
- Comprehensive player statistics and session tracking
- Automated backup and scheduled task execution
- Database-driven configuration management

---

## Features

### Core Functionality
- **Server Management**: Start, stop, restart Terraria servers with real-time status monitoring
- **Player Management**: Track player sessions, statistics, and activity history
- **Plugin Administration**: Install, configure, and manage TShock plugins
- **File Management**: Browse, edit, and manage server configuration files
- **Backup System**: Automated and manual backup with restore capabilities
- **Scheduled Tasks**: Cron-based task scheduler for automated operations

### Real-time Monitoring
- WebSocket-based live server logs
- Real-time player connection events
- System resource monitoring (CPU, memory, disk)
- Activity logging and audit trails

### Database Features
- SQLite-based storage for lightweight deployment
- Optimized indexes for high-performance queries
- Player statistics aggregation
- Session history tracking

---

## Tech Stack

| Component | Technology |
|-----------|-----------|
| Language | Go 1.21+ |
| Web Framework | Gin (HTTP routing) |
| WebSocket | gorilla/websocket |
| Database | SQLite 3 |
| ORM | Native SQL with prepared statements |
| Authentication | JWT-based token authentication |
| Process Management | os/exec with lifecycle control |

**Dependencies:**
```
github.com/gin-gonic/gin
github.com/gorilla/websocket
github.com/mattn/go-sqlite3
github.com/robfig/cron/v3
golang.org/x/crypto
```

---

## Project Structure

```
tr-panel-go/
├── api/                  # API route handlers
│   ├── auth.go          # Authentication endpoints
│   ├── room.go          # Server room management
│   ├── player.go        # Player data APIs
│   ├── plugin.go        # Plugin management
│   └── ...
├── config/              # Configuration management
│   ├── config.go        # Config loader
│   └── paths.go         # Path resolution
├── db/                  # Database layer
│   ├── db.go           # Database initialization
│   ├── schema.sql      # Database schema
│   └── migrations/     # Migration scripts
├── models/              # Data models
│   ├── room.go         # Room model
│   ├── player_stats.go # Player statistics
│   └── ...
├── services/            # Business logic layer
│   ├── config_service.go
│   ├── plugin_server_service.go
│   └── log_monitor.go
├── storage/             # Data access layer
│   ├── interface.go    # Storage interfaces
│   └── sqlite_*.go     # SQLite implementations
├── websocket/           # WebSocket handlers
│   └── handler.go
├── utils/               # Utility functions
│   ├── logger.go       # Logging utilities
│   ├── process.go      # Process management
│   └── file.go         # File operations
├── scheduler/           # Task scheduling
│   ├── scheduler.go    # Cron scheduler
│   └── handlers.go     # Task handlers
├── middleware/          # HTTP middlewares
│   ├── auth.go         # JWT authentication
│   └── ratelimit.go    # Rate limiting
└── main.go             # Application entry point
```

---

## Quick Start

### Prerequisites

- Go 1.21 or higher
- GCC compiler (for SQLite CGO)
- Linux/Windows server
- Terraria Dedicated Server (optional for testing)

### Installation

1. Clone the repository:
```bash
git clone https://github.com/ShourGG/tr-panel-go.git
cd tr-panel-go
```

2. Install dependencies:
```bash
go mod download
```

3. Build the binary:
```bash
# For Linux
CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o tr-panel .

# For Windows
go build -ldflags="-s -w" -o tr-panel.exe .
```

4. Run the application:
```bash
./tr-panel
```

The server will start on `http://localhost:8800` by default.

### Configuration

Create a `.env` file in the project root:

```env
# Server Configuration
PORT=8800
HOST=0.0.0.0

# JWT Secret
JWT_SECRET=your-secret-key-here

# Database
DB_PATH=./data/panel.db

# Terraria Server Paths
TERRARIA_SERVER_PATH=/path/to/TerrariaServer
TSHOCK_PATH=/path/to/tshock
```

---

## API Documentation

### Authentication

**POST** `/api/auth/login`
```json
{
  "username": "admin",
  "password": "password"
}
```

**POST** `/api/auth/register`
```json
{
  "username": "admin",
  "password": "password",
  "email": "admin@example.com"
}
```

### Server Management

**GET** `/api/rooms` - List all server rooms

**POST** `/api/rooms` - Create a new server room

**GET** `/api/rooms/:id` - Get room details

**POST** `/api/rooms/:id/start` - Start server

**POST** `/api/rooms/:id/stop` - Stop server

### Player Management

**GET** `/api/players` - List all players

**GET** `/api/players/:id/stats` - Get player statistics

**GET** `/api/players/:id/sessions` - Get player session history

### Plugin Management

**GET** `/api/plugins` - List installed plugins

**POST** `/api/plugins/install` - Install a plugin

**DELETE** `/api/plugins/:id` - Uninstall plugin

### WebSocket

**WS** `/ws/logs/:roomId` - Real-time server logs

**WS** `/ws/system` - System monitoring updates

---

## Deployment

### Linux Production Deployment

1. Upload the binary to your server:
```bash
scp tr-panel user@server:/opt/tr-panel/
```

2. Create a systemd service:
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

3. Enable and start the service:
```bash
sudo systemctl daemon-reload
sudo systemctl enable tr-panel
sudo systemctl start tr-panel
```

### Docker Deployment (Optional)

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

## Performance Optimizations

This backend implements several performance optimizations:

1. **Database Indexing**: Optimized indexes on frequently queried tables
   - Player sessions indexed by player ID and timestamp
   - Player stats indexed by last update time
   - Activity logs indexed by type and timestamp

2. **Code Splitting**: Frontend assets are split into multiple chunks for faster loading
   - Vue vendor bundle
   - Ant Design components bundle
   - Monaco Editor bundle
   - ECharts visualization bundle

3. **Console Removal**: Production builds have console.log statements removed

4. **Minification**: JavaScript bundles are minified with Terser

**Performance Metrics:**
- API Response Time: ~100ms average
- Page Load Time: ~78ms (Loading)
- LCP (Largest Contentful Paint): ~105ms

---

## Development

### Running in Development Mode

```bash
# Install Air for hot reload (optional)
go install github.com/cosmtrek/air@latest

# Run with hot reload
air

# Or run directly
go run main.go
```

### Running Tests

```bash
go test ./...
```

### Code Style

This project follows standard Go conventions:
- Use `gofmt` for code formatting
- Follow effective Go guidelines
- Write clear, self-documenting code
- Minimize comments (code should be self-explanatory)

---

## Contributing

Contributions are welcome! Please follow these guidelines:

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

---

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

---

## Author

Developed by [ShourGG](https://github.com/ShourGG)

---

## Support

For issues, questions, or feature requests, please open an issue on GitHub:
https://github.com/ShourGG/tr-panel-go/issues
