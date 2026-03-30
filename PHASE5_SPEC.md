# Phase 5 — Nginx + Dashboard UI (Vanilla HTML/CSS/JS)
> Spec viết bởi senior, giao cho fresher implement.
> Đọc kỹ từ đầu đến cuối trước khi code.

---

## 1. Tổng quan

Phase 5 gồm 2 việc:
1. **Nginx** làm reverse proxy — đứng trước Go API, serve static files
2. **Dashboard UI** — giao diện web thuần HTML/CSS/JS để quản lý agents

Sau phase này, user chỉ cần mở `http://localhost:8080` để dùng toàn bộ hệ thống.

**Thay đổi kiến trúc:**
```
Trước:  Browser → :5000 (Go API trực tiếp)
Sau:    Browser → :8080 (Nginx) → portal:5000 (Go API, internal)
```

Port `5000` sẽ không còn expose ra host nữa.

---

## 2. Cấu trúc thư mục mới

Tạo thêm 2 thư mục ở root của project:

```
network-isolation-docker-portal/
├── nginx/
│   └── nginx.conf          ← cấu hình nginx
├── dashboard/
│   ├── index.html          ← toàn bộ UI trong 1 file HTML
│   ├── style.css           ← CSS riêng
│   └── app.js              ← JS riêng
├── api/                    (không đổi)
├── agent/                  (không đổi)
├── db/                     (không đổi)
└── docker-compose.yml      ← có thay đổi
```

---

## 3. Nginx — `nginx/nginx.conf`

```nginx
server {
    listen 80;
    server_name _;

    # Serve dashboard static files
    location / {
        root /usr/share/nginx/html;
        index index.html;
        try_files $uri $uri/ /index.html;
    }

    # Proxy API requests to Go backend
    location /api/ {
        proxy_pass http://portal:5000;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    }

    # Proxy health check
    location /health {
        proxy_pass http://portal:5000;
    }
}
```

**Lưu ý:**
- `proxy_pass http://portal:5000` — dùng service name trong Docker network
- Không cần Dockerfile riêng cho nginx — dùng `image: nginx:alpine` với volume mount

---

## 4. Sửa `docker-compose.yml`

### 4a. Thêm service `nginx` (thêm trước service `agent`):

```yaml
  nginx:
    image: nginx:alpine
    container_name: portal-nginx
    ports:
      - "8080:80"
    volumes:
      - ./nginx/nginx.conf:/etc/nginx/conf.d/default.conf:ro
      - ./dashboard:/usr/share/nginx/html:ro
    depends_on:
      portal:
        condition: service_healthy
    restart: unless-stopped
    networks:
      - isolation-net
```

### 4b. Sửa service `portal` — xóa port 5000:

```yaml
  portal:
    build: ./api
    container_name: portal
    ports:
      - "9999:9999"     # Chỉ giữ TCP port, xóa "5000:5000"
    ...
```

Port `5000` không expose ra ngoài nữa — chỉ Nginx mới gọi được qua internal network.

---

## 5. Dashboard UI

### Luồng hoạt động:

```
Mở localhost:8080
    ↓
Có JWT trong localStorage?
    ├── Không → hiện Login Form
    └── Có    → hiện Main Dashboard
                    ├── Agents Panel (auto-refresh 10s)
                    └── Events Panel
```

### 5a. `dashboard/index.html`

Một file HTML duy nhất với 2 "màn hình" — toggle bằng CSS `display: none/block`:

**Màn hình 1: Login**
```
┌─────────────────────────────┐
│   Network Isolation Portal  │
│                             │
│   Username: [_____________] │
│   Password: [_____________] │
│                             │
│          [ Login ]          │
│                             │
│   <div id="login-error">    │
└─────────────────────────────┘
```

**Màn hình 2: Dashboard** (sau khi login)
```
┌─────────────────────────────────────────────┐
│ Network Isolation Portal      [Logout]       │
├───────────────────┬─────────────────────────┤
│  AGENTS           │  EVENTS                 │
│  ─────────────    │  ────────────────────   │
│  Agent #1         │  [table: id/cmd/time]   │
│  IP: 10.0.0.1     │                         │
│  State: NORMAL    │                         │
│  [Isolate][Rel.]  │                         │
│                   │                         │
│  Agent #2         │                         │
│  ...              │                         │
└───────────────────┴─────────────────────────┘
```

**Cấu trúc HTML cần có:**
```html
<!-- Login screen -->
<div id="screen-login">
  <form id="form-login">
    <input id="input-username" type="text" />
    <input id="input-password" type="password" />
    <button type="submit">Login</button>
  </form>
  <p id="login-error"></p>
</div>

<!-- Dashboard screen -->
<div id="screen-dashboard" style="display:none">
  <header>
    <span>Network Isolation Portal</span>
    <button id="btn-logout">Logout</button>
  </header>
  <main>
    <section id="agents-list"><!-- JS render vào đây --></section>
    <section id="events-list"><!-- JS render vào đây --></section>
  </main>
</div>
```

Load CSS và JS ở cuối `<body>`:
```html
<link rel="stylesheet" href="style.css">
<script src="app.js"></script>
```

---

### 5b. `dashboard/app.js`

Implement đầy đủ các hàm sau (không dùng framework, không import gì):

#### Hằng số:
```js
const API_BASE = '/api';
const TOKEN_KEY = 'jwt_token';
```

#### Hàm tiện ích:
```js
function getToken() { return localStorage.getItem(TOKEN_KEY); }
function setToken(t) { localStorage.setItem(TOKEN_KEY, t); }
function clearToken() { localStorage.removeItem(TOKEN_KEY); }

// Fetch wrapper tự động gắn Authorization header
// Nếu server trả 401 → gọi logout()
async function apiFetch(path, options = {}) { ... }
```

#### Auth:
```js
function showLogin() { /* hiện #screen-login, ẩn #screen-dashboard */ }
function showDashboard() { /* hiện #screen-dashboard, ẩn #screen-login */ }
function logout() { clearToken(); showLogin(); }

async function handleLogin(e) {
  e.preventDefault();
  // Lấy username, password từ form
  // POST /api/auth/login
  // Nếu OK → setToken(data.token) → showDashboard() → loadAll()
  // Nếu lỗi → hiện message trong #login-error
}
```

#### Load data:
```js
async function loadAgents() {
  // GET /api/agents
  // Render danh sách vào #agents-list
  // Mỗi agent hiện: ID, IP, trạng thái (cần gọi thêm status API)
  // Mỗi agent có nút Isolate và Release
}

async function loadAgentStatus(agentId) {
  // GET /api/agents/{id}/status
  // Trả về object status để render
}

async function loadEvents() {
  // GET /api/events?limit=20
  // Render vào #events-list dạng table
  // Cột: ID | Agent | Command | Payload | Time
}

function loadAll() {
  loadAgents();
  loadEvents();
}
```

#### Actions:
```js
async function isolateAgent(agentId) {
  // Hiện prompt() để user nhập IPs (cách nhau bằng dấu phẩy)
  // POST /api/agents/{id}/isolate với body {"ips": [...]}
  // Sau khi xong → loadAgents() để refresh
}

async function releaseAgent(agentId) {
  // POST /api/agents/{id}/release
  // Sau khi xong → loadAgents()
}
```

#### Init:
```js
document.addEventListener('DOMContentLoaded', () => {
  document.getElementById('form-login').addEventListener('submit', handleLogin);
  document.getElementById('btn-logout').addEventListener('click', logout);

  if (getToken()) {
    showDashboard();
    loadAll();
    setInterval(loadAll, 10000);  // auto-refresh 10s
  } else {
    showLogin();
  }
});
```

---

### 5c. `dashboard/style.css`

CSS tối thiểu, **không dùng external CSS framework** (không Bootstrap, không Tailwind).

Yêu cầu:
- Font: `system-ui, sans-serif`
- Layout dashboard: 2 cột dùng CSS Flexbox hoặc Grid
- Màu nền tối (dark theme) hoặc sáng — tùy chọn, miễn nhìn được
- Mỗi agent card có border, padding rõ ràng
- Nút Isolate màu đỏ/cam, nút Release màu xanh
- Bảng Events có `border-collapse: collapse`, `th` có màu nền

---

## 6. Tóm tắt Files cần tạo/thay đổi

| File | Thay đổi |
|------|----------|
| `nginx/nginx.conf` | **Tạo mới** |
| `dashboard/index.html` | **Tạo mới** |
| `dashboard/app.js` | **Tạo mới** |
| `dashboard/style.css` | **Tạo mới** |
| `docker-compose.yml` | Thêm service nginx, xóa port 5000 của portal |

**Tổng cộng: 4 file mới, 1 file sửa.**

---

## 7. Test Cases

```bash
# 1. Nginx và portal chạy bình thường
curl -s http://localhost:8080/health
# Expect: {"status":"ok","tcp_port":9999}

# 2. API vẫn accessible qua Nginx
curl -s -X POST http://localhost:8080/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"changeme123"}'
# Expect: {"token":"eyJ...","expires_in":86400}

# 3. Port 5000 không còn accessible từ host
curl -s http://localhost:5000/health
# Expect: Connection refused

# 4. Mở browser http://localhost:8080 → thấy login form
# 5. Login với admin/changeme123 → thấy dashboard với danh sách agents
# 6. Click Isolate → nhập IP → agent bị isolate → trạng thái cập nhật
# 7. Click Release → agent trở về NORMAL
# 8. Click Logout → về lại login form
# 9. Refresh trang khi đang login → vẫn ở dashboard (token còn trong localStorage)
```

---

## 8. Những thứ KHÔNG làm

- **Không** cần WebSocket (polling 10s là đủ)
- **Không** dùng framework JS (React, Vue, jQuery...)
- **Không** dùng CSS framework (Bootstrap, Tailwind...)
- **Không** cần HTTPS (đó là infra concern)
- **Không** thêm endpoint mới vào Go API
- **Không** sửa bất kỳ file nào trong `api/` hoặc `agent/`
- **Không** thêm Nginx Dockerfile — dùng `image: nginx:alpine` + volume mount là đủ

---

## 9. Lưu ý kỹ thuật

1. **`try_files $uri $uri/ /index.html`** trong nginx.conf là bắt buộc — để reload trang không bị 404.

2. **`apiFetch`** phải tự động gắn header:
   ```js
   headers: {
     'Authorization': `Bearer ${getToken()}`,
     'Content-Type': 'application/json',
     ...options.headers
   }
   ```
   Và check `response.status === 401` → gọi `logout()`.

3. **Isolate dùng `prompt()`** để nhập IPs là đủ cho Phase 5 — không cần modal phức tạp.

4. **Không dùng `innerHTML` với data từ API** để tránh XSS — dùng `textContent` hoặc `createElement`.

5. **`docker compose down`** (không cần `-v`) là đủ để restart với nginx mới — không cần xóa volume vì không đổi DB schema.

---

## 10. Checklist trước khi submit

- [ ] `docker compose up --build -d` thành công, không có container nào exit
- [ ] `docker compose ps` → 5 services đều `running` (db, redis, portal, nginx, agent)
- [ ] `curl http://localhost:5000` → **Connection refused** (port đã đóng)
- [ ] `curl http://localhost:8080/health` → `{"status":"ok",...}`
- [ ] Tất cả 9 test cases ở mục 7 pass
- [ ] Không có `console.error` nào trong browser DevTools khi dùng dashboard
- [ ] Không sửa file nào trong `api/` hoặc `agent/`
