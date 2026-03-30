# Network Isolation Docker Portal

Web Dashboard quản lý eBPF Network Isolation agents, thay thế Portal CLI bằng REST API + giao diện web container hóa.

## Mục tiêu

- Xây dựng control plane cho project eBPF network isolation
- Học Docker nâng cao: multi-service, networking, best practices
- Project CV-worthy liên quan security

---

## Kiến trúc

```
┌─────────────────────────────────────────────────────────────────────┐
│                        macOS 192.168.49.139                         │
│                                                                     │
│   Browser ──────────────────────────────────────────────────────┐  │
│                                                                  │  │
│   QEMU hostfwd          QEMU hostfwd                            │  │
│   :2222 → VM1:22        :9999 → VM1:9999                        │  │
│   :8080 → VM1:8080      :2223 → VM2:22                          │  │
└──────────────┬───────────────────────┬──────────────────────────┼──┘
               │                       │                          │
    ┌──────────▼──────────────────┐    │           :8080          │
    │  VM1 — Portal (Docker)      │    │      ┌───────────────────┘
    │                             │    │      │
    │  ┌─────────────────────┐    │    │      ▼
    │  │ nginx :8080         │◄───────────────┘  (Dashboard UI)
    │  │  serve dashboard    │    │    │
    │  │  proxy /api/*       │    │    │
    │  └──────┬──────────────┘    │    │
    │         │ /api/*            │    │
    │  ┌──────▼──────────────┐    │    │
    │  │ portal :5000        │    │    │
    │  │  REST API           │    │    │
    │  │  TCP server :9999 ◄─────────────────── VM2 agent kết nối
    │  └──────┬──────────────┘    │    │         (TCP persistent)
    │         │                   │    │
    │  ┌──────▼──────┐ ┌───────┐  │    │
    │  │ PostgreSQL  │ │ Redis │  │    │
    │  │ event log   │ │ cache │  │    │
    │  └─────────────┘ └───────┘  │    │
    └─────────────────────────────┘    │
                                       │
    ┌──────────────────────────────────▼──┐
    │  VM2 — Agent (eBPF)                 │
    │                                     │
    │  ens3 (192.168.49.139 whitelisted)  │
    │  ┌───────────────────────────────┐  │
    │  │ XDP hook (ingress)            │  │
    │  │  src IP in whitelist? → PASS  │  │
    │  │  src IP not in whitelist?     │  │
    │  │    ISOLATED → DROP            │  │
    │  │    NORMAL   → PASS            │  │
    │  └───────────────────────────────┘  │
    │  ┌───────────────────────────────┐  │
    │  │ TC hook (egress)              │  │
    │  │  dst IP in whitelist? → PASS  │  │
    │  │  dst IP not in whitelist?     │  │
    │  │    ISOLATED → DROP            │  │
    │  │    NORMAL   → PASS            │  │
    │  └───────────────────────────────┘  │
    │                                     │
    │  isolation-agent (userspace)        │
    │   ├─ kết nối TCP → 192.168.49.139:9999
    │   ├─ nhận lệnh: isolate/release     │
    │   └─ cập nhật BPF maps             │
    └─────────────────────────────────────┘
```

### Luồng traffic chính

| Luồng | Path |
|---|---|
| **Xem Dashboard** | Mac browser → Mac:8080 → VM1 nginx → portal API |
| **Agent kết nối** | VM2 → Mac:9999 → VM1 portal TCP :9999 |
| **Lệnh Isolate** | Dashboard → nginx → portal API → TCP → VM2 agent → BPF maps |
| **Packet bình thường** | Bất kỳ IP → VM2 ens3 → XDP check → PASS |
| **Packet bị chặn** | IP lạ → VM2 ens3 → XDP check → ISOLATED → DROP |
| **VM2 gửi ra ngoài** | VM2 → TC check → IP lạ + ISOLATED → DROP |
| **VM2 → portal** | VM2 → 192.168.49.139 → TC check → whitelisted → PASS |

### Services (Docker trên VM1)

| Service | Image              | Vai trò                        |
|---------|--------------------|--------------------------------|
| nginx   | nginx:alpine       | Reverse proxy + serve dashboard |
| portal  | build ./api        | Go REST API + TCP server :9999 |
| db      | postgres:16-alpine | Lưu event log                  |
| redis   | redis:7-alpine     | Cache agent status (TTL 10s)   |

---

## Tech Stack

| Layer      | Công nghệ                                              |
|------------|--------------------------------------------------------|
| API        | Go 1.22 — stdlib `net/http` + `net` TCP               |
| Auth       | JWT (golang-jwt/jwt/v5) + bcrypt (golang.org/x/crypto) |
| Database   | PostgreSQL 16 — event log                             |
| Cache      | Redis 7 — status TTL 10s                              |
| Frontend   | Vanilla HTML/CSS/JS — không framework                 |
| Container  | Docker multi-stage build, Alpine images               |

---

## Cấu trúc thư mục

```
network-isolation-docker-portal/
├── docker-compose.yml
├── .env                      # credentials (không commit lên git)
├── api/                      # Go API server
│   ├── Dockerfile            # multi-stage: golang:1.22 → alpine:3.19
│   ├── go.mod, go.sum
│   ├── main.go               # entry point: khởi động HTTP + TCP + DB + Redis
│   ├── config.go             # đọc env vars → Config struct
│   ├── auth.go               # JWT: InitAuth, GenerateToken, ValidateToken, loginHandler, SeedAdminUser
│   ├── middleware.go         # authMiddleware (Bearer token check)
│   ├── routes.go             # 9 HTTP handlers, đăng ký routes
│   ├── portal.go             # TCP server, agent registry (sync.RWMutex)
│   ├── agent.go              # Agent struct, wraps net.Conn (sync.Mutex)
│   ├── protocol.go           # parseStatusResponse → StatusInfo JSON
│   ├── db.go                 # PostgreSQL: ConnectDB, LogEvent, QueryEvents, User CRUD
│   └── cache.go              # Redis: CacheGet, CacheSet, CacheInvalidate
├── agent/                    # Mock agent (Go)
│   ├── Dockerfile
│   └── main.go               # TCP client, command handler, auto-reconnect
├── db/
│   └── init.sql              # CREATE TABLE events, users
├── nginx/
│   └── nginx.conf            # reverse proxy /api/ → portal:5000, serve /
└── dashboard/                # Frontend (vanilla JS)
    ├── index.html
    ├── app.js
    └── style.css
```

---

## Yêu cầu

- Docker + Docker Compose v2
- File `.env` tại root (xem mục **Cấu hình**)

---

## Cấu hình — file `.env`

Tạo file `.env` tại root project:

```env
# PostgreSQL
POSTGRES_USER=portal
POSTGRES_PASSWORD=portalpass
POSTGRES_DB=portal_db
DATABASE_URL=postgres://portal:portalpass@db:5432/portal_db?sslmode=disable

# Redis
REDIS_URL=redis://redis:6379

# JWT (đổi thành chuỗi random dài ≥ 32 ký tự)
JWT_SECRET=change-this-to-a-long-random-secret-string

# Admin user (tự động tạo khi khởi động lần đầu)
ADMIN_USERNAME=admin
ADMIN_PASSWORD=changeme123
```

---

## Chạy project

```bash
# Lần đầu hoặc sau khi thay đổi code
docker compose up --build -d

# Xem logs
docker compose logs -f portal

# Dừng
docker compose down

# Reset hoàn toàn (xóa cả database)
docker compose down -v
```

Kiểm tra tất cả services đã chạy:
```bash
docker compose ps
```

---

## Truy cập Dashboard

### Từ máy chạy Docker trực tiếp
Mở browser: `http://localhost:8080`

### Từ máy Mac (VM chạy QEMU qua port 2222)

Mở SSH tunnel trên Mac:
```bash
ssh -L 8080:localhost:8080 -p 2222 user@localhost
```
Giữ terminal này mở, sau đó mở browser: `http://localhost:8080`

Đăng nhập bằng `ADMIN_USERNAME` / `ADMIN_PASSWORD` đã cấu hình trong `.env`.

---

## API Endpoints

**Base URL:** `http://localhost:8080` (qua Nginx)

| Method | Endpoint                   | Mô tả                          | Auth |
|--------|----------------------------|--------------------------------|------|
| POST   | `/api/auth/login`          | Đăng nhập, nhận JWT token      | No   |
| GET    | `/health`                  | Health check                   | No   |
| GET    | `/api/agents`              | Danh sách agents đang kết nối  | Yes  |
| GET    | `/api/agents/{id}/status`  | Trạng thái agent (Redis cache) | Yes  |
| POST   | `/api/agents/{id}/isolate` | Cô lập mạng                    | Yes  |
| POST   | `/api/agents/{id}/release` | Tắt cô lập                     | Yes  |
| POST   | `/api/agents/{id}/whitelist` | Thêm/xóa IP whitelist        | Yes  |
| POST   | `/api/agents/broadcast`    | Gửi lệnh đến tất cả agents    | Yes  |
| GET    | `/api/events`              | Lịch sử sự kiện                | Yes  |

**Auth header:** `Authorization: Bearer <token>`

---

## TCP Protocol (agent ↔ portal)

Port 9999, text-based, newline-delimited:

| Command                    | Response                            |
|----------------------------|-------------------------------------|
| `isolate 10.0.0.1 10.0.0.2` | `OK:ISOLATED (2 IPs whitelisted)` |
| `release`                  | `OK:RELEASED`                       |
| `status`                   | `STATE:ISOLATED,WL:10.0.0.1;10.0.0.2` |
| `whitelist add 10.0.0.1`   | `OK:WL_ADD 10.0.0.1`               |
| `quit`                     | `OK:SHUTDOWN`                       |

---

## Lưu ý kỹ thuật

- **Dual protocol**: Go binary chạy HTTP :5000 + TCP :9999 trong cùng process
- **Concurrency**: `Portal` dùng `sync.RWMutex`, mỗi `Agent` có `sync.Mutex` riêng
- **JWT**: HS256, expiry 24h, secret từ env — panic khi khởi động nếu thiếu
- **bcrypt**: `DefaultCost` (10) cho password hashing
- **Port exposure**: `:9999` expose ra host (cho real agent kết nối vào), `:5000` chỉ internal
- **Docker**: multi-stage build (~12MB image), non-root user, healthcheck, restart policy

---

## Phases & Tiến độ

| Phase | Nội dung                 | Status    |
|-------|--------------------------|-----------|
| 1     | Go API + TCP Server      | **DONE**  |
| 2     | PostgreSQL Event Log     | **DONE**  |
| 3     | Redis Caching            | **DONE**  |
| 4     | JWT Authentication       | **DONE**  |
| 5     | Nginx + Dashboard UI     | **DONE**  |
| 6     | Real eBPF Agent trên VM  | **DONE**  |

---

## Lịch sử

- **2026-03-29**: Hoàn thành Phase 1+2+3 — Go API, PostgreSQL, Redis, mock agent, test OK
- **2026-03-30**: Hoàn thành Phase 4 — JWT auth, bcrypt, middleware, admin seed
- **2026-03-30**: Hoàn thành Phase 5 — Nginx reverse proxy, vanilla JS dashboard, dark theme
- **2026-03-30**: Hoàn thành Phase 6 — Real eBPF agent (C+libbpf) kết nối portal qua systemd service, XDP+TC hooks hoạt động
