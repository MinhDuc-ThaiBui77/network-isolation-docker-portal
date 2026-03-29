# Security Monitoring Dashboard

## Mục tiêu

Xây dựng Web Dashboard quản lý eBPF Network Isolation agents, thay thế Portal CLI.
- Học Docker nâng cao (multi-service, networking, best practices)
- Project CV-worthy liên quan security

## Liên kết với project eBPF

Project gốc: `/root/net_isolate/` (xem `Guideline.md` để hiểu bối cảnh)
- **Agent thật** (C + libbpf): chạy trên VM endpoint, dùng XDP + TC hooks để cô lập network
- **Portal API** (Go): quản lý agents qua HTTP REST + TCP, thay thế Portal CLI (Python)
- **Protocol**: text-based TCP, newline-delimited, port 9999
  - Commands: `isolate <ip>`, `release`, `status`, `whitelist add/del <ip>`, `quit`
  - Responses: `OK:ISOLATED (N IPs whitelisted)`, `OK:RELEASED`, `STATE:ISOLATED,WL:ip1;ip2`

Project này **thay thế Portal CLI bằng Web Dashboard** trong Docker, giữ nguyên protocol TCP với agent.

## Tech Stack

- **API server**: Go (stdlib `net/http` + `net` TCP) — zero external dependencies
- **Agent mock**: Go — giả lập agent cho Docker testing
- **Agent thật**: C + libbpf — chạy trên VM (không nằm trong repo này)
- **Docker**: multi-stage build, alpine images

## Kiến trúc

```
Browser ──► Nginx (:8080) ──► Go API (:5000) ──► PostgreSQL
                                    │                  Redis
                                    │
                                TCP :9999
                                    │
                        VM: Agent eBPF thật (kết nối từ ngoài)
                        Docker: Agent mock (test nội bộ)
```

| Service    | Vai trò                                      |
|------------|----------------------------------------------|
| Nginx      | Reverse proxy + serve dashboard HTML         |
| Go API     | REST endpoints + TCP server cho agent (:9999)|
| PostgreSQL | Lưu event log (isolate/release history)      |
| Redis      | Cache agent status (TTL 10s)                 |
| Agent mock | Test nội bộ không cần VM                     |

## Cấu trúc thư mục

```
docker/
├── README.md
├── docker-compose.yml
├── .env                          # DB credentials (không commit lên git)
├── api/                          # Go API server
│   ├── Dockerfile                # multi-stage: golang → alpine
│   ├── .dockerignore
│   ├── go.mod, go.sum
│   ├── main.go                   # entry point: TCP + HTTP + DB, graceful shutdown
│   ├── config.go                 # Config từ env vars
│   ├── agent.go                  # Agent struct (wraps TCP connection)
│   ├── portal.go                 # Portal struct (TCP accept loop, agent management)
│   ├── protocol.go               # parseStatusResponse(), JSON structs
│   ├── db.go                     # ConnectDB, LogEvent, QueryEvents
│   ├── cache.go                  # ConnectRedis, CacheGet/Set/Invalidate
│   └── routes.go                 # HTTP handlers (net/http.ServeMux)
├── agent/                        # Go mock agent
│   ├── Dockerfile                # multi-stage: golang → alpine
│   ├── .dockerignore
│   ├── go.mod
│   └── main.go                   # TCP client, command handler, reconnect loop
├── db/                           # PostgreSQL init
│   └── init.sql                  # CREATE TABLE events
├── nginx/                        # (Phase 5)
│   ├── Dockerfile
│   ├── nginx.conf
│   └── static/
│       ├── index.html
│       ├── app.js
│       └── style.css
└── (volumes: pgdata)             # PostgreSQL data persisted
```

## API Endpoints

| Method | Endpoint                       | Mô tả                  | Auth |
|--------|--------------------------------|-------------------------|------|
| POST   | /api/auth/login                | Login, nhận token       | No   |
| GET    | /api/agents                    | List agents             | Yes  |
| GET    | /api/agents/{id}/status        | Agent status            | Yes  |
| POST   | /api/agents/{id}/isolate       | Isolate (body: ips)     | Yes  |
| POST   | /api/agents/{id}/release       | Release isolation       | Yes  |
| POST   | /api/agents/{id}/whitelist     | Add/del whitelist IP    | Yes  |
| POST   | /api/agents/broadcast          | Broadcast command       | Yes  |
| GET    | /api/events                    | Event history           | Yes  |
| GET    | /health                        | Health check            | No   |

## Phases & Tiến độ

| Phase | Nội dung                    | Status      |
|-------|-----------------------------|-------------|
| 1     | Go API + TCP Server         | DONE        |
| 2     | PostgreSQL Event Logging    | DONE        |
| 3     | Redis Caching               | DONE        |
| 4     | API Authentication          | NOT STARTED |
| 5     | Nginx + Dashboard           | NOT STARTED |
| 6     | Real Agent trên VM          | NOT STARTED |

## Lưu ý kỹ thuật

- **Dual protocol**: Go binary chạy HTTP :5000 + TCP :9999 cùng process (goroutines)
- **Concurrency**: Portal/Agent dùng sync.RWMutex, mỗi Agent có sync.Mutex riêng
- **Port exposure**: 9999 expose ra host (cho real agent), 5000 chỉ internal (qua nginx)
- **Docker best practices**: multi-stage build, non-root user, .dockerignore, healthcheck, restart policy
- **External dependencies**: `github.com/lib/pq` (PostgreSQL driver), `github.com/redis/go-redis/v9` (Redis client)

## Lịch sử

- **2026-03-29**: Hoàn thành Docker basics. Lên plan project mới. Code Phase 1 (Python) xong → rewrite sang Go. Phase 1 + 2 + 3 hoàn thành và test OK.
