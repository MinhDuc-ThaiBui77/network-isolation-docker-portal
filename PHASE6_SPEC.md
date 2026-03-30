# Phase 6 — Real eBPF Agent Deployment
> Spec viết bởi senior, giao cho fresher implement.
> Repo agent: `/root/jspinkman/Network_isolate/`
> Repo portal: `/root/jspinkman/network-isolation-docker-portal/` (KHÔNG đụng vào)

---

## 1. Tổng quan

Phase này kết nối **real eBPF agent** (C + libbpf) vào portal thay cho mock agent.
Agent đã có sẵn code TCP client với đúng protocol — không cần viết lại từ đầu.

**1 việc cần code:**
- Tạo `isolation/agent.service` — systemd unit file để agent chạy như service

**Không đụng vào:** `agent.c`, `isolation.bpf.c`, `isolation.h`, `Makefile`, `setup_env.sh`, toàn bộ portal repo.

---

## 2. Sơ đồ mạng

```
Mac (192.168.49.139)
  ├── QEMU VM1 — Portal   hostfwd: 2222→22, 9999→9999
  │     └── Docker: portal container TCP :9999
  │
  └── QEMU VM2 — Agent    hostfwd: 2223→22
        └── ./agent eth0 --portal 192.168.49.139 --port 9999
```

**Luồng kết nối của agent:**
```
VM2 → 192.168.49.139:9999 (Mac) → VM1:9999 (QEMU hostfwd) → Docker portal:9999
```

---

## 3. Chuẩn bị VM1 (Portal VM) — cần restart

VM1 hiện khởi động bằng lệnh:
```bash
qemu-system-x86_64 \
  -m 4096 -cpu Westmere -machine pc -smp 2 \
  -hda oracle8.qcow2 \
  -nic user,hostfwd=tcp::2222-:22 \
  -nographic -serial mon:stdio
```

**Cần thêm** `hostfwd=tcp::9999-:9999` vào `-nic`:
```bash
qemu-system-x86_64 \
  -m 4096 -cpu Westmere -machine pc -smp 2 \
  -hda oracle8.qcow2 \
  -nic user,hostfwd=tcp::2222-:22,hostfwd=tcp::9999-:9999 \
  -nographic -serial mon:stdio
```

Sau khi restart VM1 với lệnh mới, kiểm tra từ Mac:
```bash
curl -s http://localhost:9999 2>&1 || echo "port reachable"
# Hoặc đơn giản hơn:
nc -zv localhost 9999
# Expect: Connection succeeded (portal đang lắng nghe)
```

---

## 4. Tạo `isolation/agent.service` (systemd unit file)

Tạo file mới `isolation/agent.service`:

```ini
[Unit]
Description=eBPF Network Isolation Agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/opt/isolation/agent INTERFACE --portal PORTAL_IP --port 9999
Restart=on-failure
RestartSec=5s
StandardOutput=journal
StandardError=journal
SyslogIdentifier=isolation-agent

[Install]
WantedBy=multi-user.target
```

`INTERFACE` và `PORTAL_IP` là placeholders — người deploy thay bằng giá trị thật.
Không hardcode vào file này.

---

## 5. Tóm tắt Files thay đổi

| File | Thay đổi |
|------|----------|
| `isolation/agent.service` | **Tạo mới** — systemd unit file |

**Chỉ 1 file mới. Không sửa file nào khác.**

---

## 6. Hướng dẫn Deploy trên VM2 (Agent VM)

### Bước 1 — Setup environment (1 lần)

SSH vào VM2:
```bash
ssh -p 2223 user@localhost
```

Chạy setup:
```bash
cd /root/jspinkman/Network_isolate
sudo bash setup_env.sh --yes
```

### Bước 2 — Build

```bash
cd /root/jspinkman/Network_isolate/isolation
make clean && make
```

Build thành công khi thấy:
```
[OK] Build complete: isolation.bpf.o agent
```

### Bước 3 — Test chạy tay

```bash
# Xem tên interface mạng của VM2
ip -br link | grep -v lo
# Thường là: eth0, ens3, enp0s3, ...

# Chạy thử
sudo ./agent <tên_interface> --portal 192.168.49.139 --port 9999
```

Output mong đợi:
```
Interface: eth0 (ifindex=2)
BPF object loaded successfully.
XDP attached to ifindex 2 (generic/SKB mode).
TC egress attached to ifindex 2.
Connecting to portal 192.168.49.139:9999 ...
Connected to portal 192.168.49.139:9999
Portal IP 192.168.49.139 auto-whitelisted.
```

Kiểm tra Dashboard (`http://localhost:8080` từ Mac): agent mới phải xuất hiện.

Dừng: `Ctrl+C`

### Bước 4 — Cài systemd service

```bash
# Copy binary + BPF object
sudo mkdir -p /opt/isolation
sudo cp agent isolation.bpf.o /opt/isolation/

# Copy service file, thay INTERFACE và PORTAL_IP bằng giá trị thật
sudo cp agent.service /etc/systemd/system/isolation-agent.service
sudo sed -i 's/INTERFACE/<tên_interface>/' /etc/systemd/system/isolation-agent.service
sudo sed -i 's/PORTAL_IP/192.168.49.139/' /etc/systemd/system/isolation-agent.service

# Enable và start
sudo systemctl daemon-reload
sudo systemctl enable --now isolation-agent

# Kiểm tra
sudo systemctl status isolation-agent
sudo journalctl -u isolation-agent -f
```

---

## 7. Test Cases

### Trên VM2:
```bash
# Service đang chạy
sudo systemctl status isolation-agent
# Expect: active (running)

# Log kết nối thành công
sudo journalctl -u isolation-agent --no-pager | tail -5
# Expect: "Connected to portal 192.168.49.139:9999"
```

### Trên Dashboard (Mac → `http://localhost:8080`):

| # | Hành động | Kết quả mong đợi |
|---|-----------|------------------|
| 1 | Xem Agents panel | Agent thật xuất hiện, state `NORMAL` |
| 2 | Click Isolate → nhập `10.0.0.1` | State chuyển `ISOLATED` |
| 3 | Ping từ IP không trong whitelist vào VM2 | Bị drop |
| 4 | Ping từ `192.168.49.139` (Mac) vào VM2 | Pass (auto-whitelisted) |
| 5 | Click Release | State về `NORMAL` |
| 6 | Xem Events panel | Log các lệnh vừa thực hiện |
| 7 | Reboot VM2 | Service tự start, agent tự reconnect portal |

---

## 8. Những thứ KHÔNG làm

- **Không** sửa `agent.c`, `isolation.bpf.c`, `isolation.h`, `Makefile`, `setup_env.sh`
- **Không** sửa bất cứ thứ gì trong portal repo
- **Không** hardcode IP hay interface trong `agent.service`

---

## 9. Lưu ý kỹ thuật

1. **`isolation.bpf.o` phải cùng thư mục với `agent` binary** — `agent.c` dùng relative path `"isolation.bpf.o"`. Copy cả hai vào `/opt/isolation/`.

2. **Agent tự whitelist `192.168.49.139`** (Mac/portal IP) ngay khi kết nối — đảm bảo kết nối TCP không bị cắt khi isolate. Đây là behavior đúng, không cần thay đổi.

3. **`ExecStart` phải dùng absolute path** — systemd không có PATH của user shell.

4. **Mock agent (Docker) vẫn chạy song song** — sẽ có 2 agent trong dashboard. Nếu muốn test agent thật riêng, stop mock agent bằng `docker compose stop agent`.

---

## 10. Checklist trước khi submit

- [ ] VM1 đã restart với `hostfwd=tcp::9999-:9999`, `nc -zv localhost 9999` từ Mac thành công
- [ ] `make clean && make` trên VM2 không có error
- [ ] Chạy tay `sudo ./agent <iface> --portal 192.168.49.139 --port 9999` → kết nối được, hiện trên dashboard
- [ ] `agent.service` có `INTERFACE` và `PORTAL_IP` là placeholder (không hardcode)
- [ ] `sudo systemctl enable --now isolation-agent` → service `active`
- [ ] Reboot VM2 → service tự start lại
- [ ] Isolate/Release từ dashboard hoạt động với agent thật
- [ ] Không sửa bất kỳ file nào ngoài file mới `agent.service`
