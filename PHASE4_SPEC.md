# Phase 4 — API Authentication (JWT)
> Spec viết bởi senior, giao cho fresher implement.
> Đọc kỹ từ đầu đến cuối trước khi code. Không tự ý thêm feature ngoài scope.

---

## 1. Tổng quan

Hiện tại toàn bộ `/api/*` endpoints không có bảo vệ gì — bất kỳ ai cũng gọi được.
Phase 4 thêm **JWT Bearer Token authentication**:

- Một endpoint login: `POST /api/auth/login` → trả về JWT token
- Mọi endpoint `/api/*` khác → **bắt buộc** có `Authorization: Bearer <token>` header
- Endpoint `/health` → **không** cần auth (healthcheck của Docker dùng nó)

---

## 2. Công nghệ & Dependencies mới

Thêm vào `api/go.mod` (chạy `go get`):

```
github.com/golang-jwt/jwt/v5 v5.2.1
golang.org/x/crypto v0.22.0
```

`golang.org/x/crypto` dùng cho **bcrypt** (hash password).
`golang-jwt/jwt/v5` là thư viện JWT chuẩn nhất cho Go hiện tại.

---

## 3. Database — Thêm bảng `users`

Sửa file `db/init.sql`, **thêm vào cuối file** (không xóa gì cũ):

```sql
CREATE TABLE IF NOT EXISTS users (
    id         SERIAL PRIMARY KEY,
    username   VARCHAR(50) UNIQUE NOT NULL,
    password   TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

**Lưu ý:** PostgreSQL đã chạy rồi, bảng này chỉ được tạo khi chạy fresh (volume mới).
Trong môi trường dev, `docker compose down -v && docker compose up` để reset.

---

## 4. Config — Thêm biến môi trường

Sửa `api/config.go`:

**Thêm 3 field vào struct `Config`:**
```go
JWTSecret      string
AdminUsername  string
AdminPassword  string
```

**Thêm vào hàm `LoadConfig()`:**
```go
JWTSecret:     os.Getenv("JWT_SECRET"),
AdminUsername: os.Getenv("ADMIN_USERNAME"),
AdminPassword: os.Getenv("ADMIN_PASSWORD"),
```

---

## 5. Database Layer — Thêm User functions

Sửa `api/db.go`, thêm vào cuối file:

**Struct:**
```go
type User struct {
    ID       int
    Username string
    Password string // bcrypt hash
}
```

**Hai hàm cần implement:**

```go
// CreateUser tạo user mới với password đã được bcrypt hash từ trước.
// Trả về error nếu username đã tồn tại.
func CreateUser(username, hashedPassword string) error

// FindUserByUsername tìm user theo username.
// Trả về (nil, nil) nếu không tìm thấy (không phải lỗi).
func FindUserByUsername(username string) (*User, error)
```

---

## 6. File mới: `api/auth.go`

Tạo file `api/auth.go` với **4 phần**:

### 6a. Biến global

```go
var jwtSecret []byte
```

Biến này được set từ `main.go` khi startup.

### 6b. Hàm `InitAuth(secret string)`

```go
func InitAuth(secret string)
```

Set `jwtSecret = []byte(secret)`.
Nếu secret rỗng, log warning và panic (không cho chạy thiếu secret).

### 6c. Hàm `GenerateToken(userID int, username string) (string, error)`

Tạo JWT token với claims:
- `sub`: userID (dạng string)
- `username`: username
- `exp`: NOW + 24 giờ
- `iat`: NOW

Dùng `jwt.NewWithClaims(jwt.SigningMethodHS256, claims)`.
Ký bằng `jwtSecret`.

### 6d. Hàm `ValidateToken(tokenStr string) (jwt.MapClaims, error)`

Parse và validate token.
- Nếu token hết hạn → trả error
- Nếu chữ ký sai → trả error
- Nếu OK → trả `jwt.MapClaims`

**Bắt buộc** kiểm tra `alg == HS256` khi parse (dùng `jwt.WithValidMethods([]string{"HS256"})`).

### 6e. Handler `loginHandler(w http.ResponseWriter, r *http.Request)`

Request body (JSON):
```json
{"username": "admin", "password": "secret123"}
```

Logic:
1. Decode body → lấy username, password
2. Validate: username hoặc password rỗng → 400 `{"error": "username and password are required"}`
3. Gọi `FindUserByUsername(username)`
4. Nếu user không tồn tại → 401 `{"error": "invalid credentials"}`
5. `bcrypt.CompareHashAndPassword(user.Password, password)` → nếu sai → 401 `{"error": "invalid credentials"}`
6. Gọi `GenerateToken(user.ID, user.Username)` → nếu lỗi → 500
7. Trả 200:
```json
{
  "token": "eyJhbGci...",
  "expires_in": 86400
}
```

**Quan trọng:** Bước 4 và 5 phải trả về **cùng một error message** `"invalid credentials"` để tránh username enumeration attack.

### 6f. Hàm `SeedAdminUser(username, password string)`

Gọi trong `main.go` sau `ConnectDB`.

Logic:
1. Nếu `username == ""` hoặc `password == ""` → log warning, return (không panic)
2. Gọi `FindUserByUsername(username)`
3. Nếu user đã tồn tại → log "admin user already exists", return
4. Hash password bằng `bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)`
5. Gọi `CreateUser(username, hashedPassword)`
6. Log "admin user created: {username}"

---

## 7. File mới: `api/middleware.go`

Tạo file `api/middleware.go`:

### Hàm `authMiddleware(next http.HandlerFunc) http.HandlerFunc`

```go
func authMiddleware(next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        // 1. Lấy header Authorization
        // 2. Kiểm tra format "Bearer <token>"
        // 3. Gọi ValidateToken
        // 4. Nếu lỗi → 401 {"error": "unauthorized"}
        // 5. Nếu OK → gọi next(w, r)
    }
}
```

**Lưu ý:**
- Nếu header không có hoặc không đúng format → 401
- Nếu token invalid/expired → 401
- **Không** trả 403 — chỉ dùng 401 cho auth failures

---

## 8. Sửa `api/routes.go`

### Thêm route login (KHÔNG bảo vệ bằng auth):
```go
mux.HandleFunc("POST /api/auth/login", loginHandler)
```

### Wrap tất cả các route `/api/*` còn lại bằng `authMiddleware`:
```go
mux.HandleFunc("GET /api/agents", authMiddleware(listAgentsHandler))
mux.HandleFunc("GET /api/agents/{id}/status", authMiddleware(agentStatusHandler))
mux.HandleFunc("POST /api/agents/{id}/isolate", authMiddleware(isolateHandler))
mux.HandleFunc("POST /api/agents/{id}/release", authMiddleware(releaseHandler))
mux.HandleFunc("POST /api/agents/{id}/whitelist", authMiddleware(whitelistHandler))
mux.HandleFunc("POST /api/agents/broadcast", authMiddleware(broadcastHandler))
mux.HandleFunc("GET /api/events", authMiddleware(eventsHandler))
```

`/health` giữ nguyên, không wrap.

---

## 9. Sửa `api/main.go`

Thêm **2 dòng** sau khi `ConnectDB` thành công:

```go
InitAuth(cfg.JWTSecret)
SeedAdminUser(cfg.AdminUsername, cfg.AdminPassword)
```

Thứ tự:
1. `ConnectDB` ✓ (đã có)
2. `InitAuth(cfg.JWTSecret)` ← thêm
3. `SeedAdminUser(...)` ← thêm
4. `ConnectRedis` ✓ (đã có)
5. ...

---

## 10. Sửa `docker-compose.yml`

Trong service `portal`, thêm vào block `environment`:
```yaml
- JWT_SECRET=${JWT_SECRET}
- ADMIN_USERNAME=${ADMIN_USERNAME}
- ADMIN_PASSWORD=${ADMIN_PASSWORD}
```

---

## 11. Tóm tắt Files cần thay đổi

| File | Thay đổi |
|------|----------|
| `db/init.sql` | Thêm bảng `users` |
| `api/go.mod` | Thêm 2 deps mới |
| `api/config.go` | Thêm 3 field: JWTSecret, AdminUsername, AdminPassword |
| `api/db.go` | Thêm struct User + CreateUser + FindUserByUsername |
| `api/auth.go` | **Tạo mới**: InitAuth, GenerateToken, ValidateToken, loginHandler, SeedAdminUser |
| `api/middleware.go` | **Tạo mới**: authMiddleware |
| `api/routes.go` | Thêm login route, wrap các route với authMiddleware |
| `api/main.go` | Thêm gọi InitAuth + SeedAdminUser |
| `docker-compose.yml` | Thêm 3 env vars cho portal service |

**Tổng cộng: 2 file mới, 7 file sửa.**

---

## 12. API Contract hoàn chỉnh

### Login
```
POST /api/auth/login
Content-Type: application/json

{"username": "admin", "password": "yourpassword"}

→ 200 OK
{"token": "eyJhbGci...", "expires_in": 86400}

→ 401 Unauthorized (sai credentials)
{"error": "invalid credentials"}

→ 400 Bad Request (thiếu field)
{"error": "username and password are required"}
```

### Dùng token cho protected endpoints
```
GET /api/agents
Authorization: Bearer eyJhbGci...

→ 200 OK (nếu token hợp lệ)
→ 401 Unauthorized nếu không có token hoặc token sai
{"error": "unauthorized"}
```

---

## 13. Test Cases (dùng curl để verify)

Sau khi code xong, chạy các test này theo thứ tự:

```bash
# 1. Health check vẫn hoạt động không cần auth
curl -s http://localhost:5000/health
# Expect: {"status":"ok","tcp_port":9999}

# 2. Gọi API không có token → phải bị từ chối
curl -s http://localhost:5000/api/agents
# Expect: {"error":"unauthorized"} với HTTP 401

# 3. Login sai credentials
curl -s -X POST http://localhost:5000/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"wrong"}'
# Expect: {"error":"invalid credentials"} với HTTP 401

# 4. Login đúng credentials
curl -s -X POST http://localhost:5000/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"yourpassword"}'
# Expect: {"token":"eyJ...","expires_in":86400} với HTTP 200

# 5. Gọi API với token hợp lệ
TOKEN="eyJ..."   # lấy từ bước 4
curl -s http://localhost:5000/api/agents \
  -H "Authorization: Bearer $TOKEN"
# Expect: {"agents":[...]} với HTTP 200

# 6. Login thiếu field
curl -s -X POST http://localhost:5000/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin"}'
# Expect: {"error":"username and password are required"} với HTTP 400
```

---

## 14. Những thứ KHÔNG làm (out of scope)

- **Không** thêm refresh token (chỉ access token là đủ)
- **Không** thêm endpoint đổi password, tạo user mới qua API
- **Không** thêm role/permission (chỉ có 1 loại user)
- **Không** thêm rate limiting cho login
- **Không** lưu token vào Redis hay blacklist
- **Không** thêm HTTPS/TLS (đó là việc của Nginx ở Phase 5)
- **Không** sửa bất cứ logic nào của agents, portal, TCP server

---

## 15. Lưu ý kỹ thuật quan trọng

1. **JWT_SECRET phải đủ dài:** Ít nhất 32 ký tự random. Trong `.env` file:
   ```
   JWT_SECRET=change-this-to-a-very-long-random-secret-string
   ADMIN_USERNAME=admin
   ADMIN_PASSWORD=changeme123
   ```

2. **Không hardcode** secret, username, password trong code. Phải đọc từ env.

3. **bcrypt cost:** Dùng `bcrypt.DefaultCost` (10). Không giảm xuống để "cho nhanh".

4. **`SeedAdminUser` phải idempotent:** Chạy 100 lần vẫn không tạo duplicate, không lỗi.

5. **`FindUserByUsername` trả `(nil, nil)` khi not found**, không phải `sql.ErrNoRows` — caller phải check `user == nil`.

6. **Error messages của 401 phải giống nhau** giữa "user không tồn tại" và "password sai". Đây là security requirement.

7. **Đừng** import package không cần thiết.

---

## 16. Checklist trước khi submit

- [ ] `docker compose build --no-cache && docker compose up -d` thành công
- [ ] Tất cả 6 test cases curl ở mục 13 pass
- [ ] `docker compose logs portal` không có error/panic khi startup
- [ ] Không có hardcoded credentials trong code
- [ ] Không thay đổi bất kỳ logic nào ngoài scope của Phase 4
