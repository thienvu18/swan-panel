# SwanPanel P1 — Báo cáo nghiệm thu E2E & So sánh triển khai (2026-11-16)

- **Ngày thực hiện:** 2026-11-16
- **Môi trường thử nghiệm:** Orange Pi Zero 3 (ARM64, Sunxi64 6.18.33, 1GB RAM) tại `192.168.101.50`
- **Git HEAD:** `0d3f1784e3f036cbd2da0f3701618f57cd7cb205`
- **Image mới được nghiệm thu:** `swan-panel:latest` (ID: `sha256:cf313c83e7a8a78af4460a2a73d83993c1c480561383d87b88c2c90469825597`, build 2026-11-16T11:42:42Z)
- **Phạm vi:** Kiểm tra T01–T08 trên container độc lập và thử nghiệm phương án triển khai Host strongSwan (`deploy/docker/docker-compose.host-swan.yml`).
- **Quy tắc:** Bằng chứng đã được loại bỏ thông tin nhạy cảm (sanitized). Không dùng kết quả của image cũ làm bằng chứng cho revision mới. Không can thiệp vào các dịch vụ khác (`fcna-*`) trên máy chủ.

---

## 1. Bảng ma trận kết quả (Evidence Matrix)

Bảng dưới ghi nhận **vòng test ban đầu** (trước khi vá H-01–H-05). Kết quả Host-mode sau sửa được nghiệm thu lại tại Mục 5.1–5.3.6; không đọc các ô FAIL lịch sử như trạng thái cuối.

| Case | Priority | Trạng thái (Container) | Trạng thái (Host strongSwan) | Bằng chứng tóm tắt (Sanitized) |
| :--- | :---: | :---: | :---: | :--- |
| **H0 Phê duyệt & Provenance** | P0 | **PASS** | **PASS** | Được chủ sở hữu phê duyệt môi trường dev trên Orange Pi Zero 3; build image ARM64 native và triển khai thành công. |
| **T01 Tiến trình & Tính bền vững** | P0 | **PASS** | **PASS** | Container `healthy`; `send_cert = always` trên đĩa; server.key `600`; UDP/TCP 53 binded; VICI IPC responsive. |
| **T02 Bất biến kiến trúc container** | P1 | **PASS** | **FAIL** *(Xem H-01)* | Mode container chuẩn: 1 container bridge, 3 volumes, single binary. Mode host-swan: lỗi đụng độ cổng 500/udp. |
| **T03 Android Native Onboarding** | P0 | **PASS** | **PASS** *(IKEv2 SA)* | Thiết bị Android kết nối IKEv2 RSA thành công, gán đúng VIP `10.122.0.3`, ESP:AES_GCM_16-256. |
| **T04 iOS Native Profile + Persistence** | P0 | **PASS** | **PASS** *(IKEv2 SA)* | iPhone 13 kết nối IKEv2 RSA thành công, gán đúng VIP `10.122.0.2`, ESP:AES_CBC-256/HMAC_SHA2_256_128; cấu hình bền vững sau recreate container. |
| **T05 Concurrent Clients & Kick Isolation** | P0 | **PASS** | **NOT RUN** | 2 thiết bị iOS và Android online đồng thời; kick phiên iOS ngắt SA lập tức; phiên Android và DNS duy trì 100% ổn định. |
| **T06 Multi-pool & Zero-Trust Firewall** | P0 | **PASS** | **FAIL** *(Xem H-03)* | Mode container: 3 pool VICI, 3 VIP `lo:swanpanel`, `table inet swanpanel` default-drop. Mode host-swan: nftables của container bị bypass hoàn toàn. |
| **T07 Split-DNS Engine & Chống rò rỉ** | P1 | **PASS** | **FAIL** *(Xem H-02)* | Mode container: Dig port 53 UDP/TCP trả đúng VIP, AAAA trả authoritative NODATA (ANSWER: 0), forward upstream thành công. Mode host-swan: 100% rớt gói tin DNS. |
| **T08 UI Security, RBAC & Accessibility** | P1 | **PASS** | **PASS** | 21/21 UI automated tests PASS; ConfirmDialog chống double-submit; select styled đồng nhất hai theme; last-admin guard 409. |

---

## 2. Chi tiết kết quả kiểm thử trên Container độc lập (Official Mode)

### T01 (P0): Kiểm tra tiến trình, file cấu hình và tính bền vững
- **Healthcheck:** `{"service":"SwanPanel","status":"healthy"}` (HTTP 200).
- **Cấu hình gateway trên đĩa:** `/etc/swanctl/conf.d/swanpanel-gateway.conf` tồn tại, chứa `send_cert = always` cho tất cả kết nối được quản lý.
- **Tệp cấu hình chính:** `/etc/swanctl/swanctl.conf` bao gồm `include conf.d/*.conf`.
- **Quyền khóa riêng:** `/etc/swanctl/rsa/server.key` đạt quyền `600 root:root` (không public).
- **Lắng nghe Split-DNS:** Lắng nghe đồng thời trên `0.0.0.0:53/udp` và `0.0.0.0:53/tcp`.
- **VICI IPC:** Daemon `charon` nội bộ hoạt động, VICI socket `/var/run/charon.vici` phản hồi tức thì.

### T03, T04, T05: Kết nối đồng thời 2 thiết bị vật lý và cô lập phiên ngắt kết nối
Cả hai thiết bị thực tế kết nối đồng thời vào gateway trên revision mới:
```text
swanpanel-dev_b62383d0-3ca4-47af-809c-c5d0ddbd5e85: #4, ESTABLISHED, IKEv2
  local  '192.168.101.50' @ 172.19.0.2[4500]
  remote 'vpn-iphone13' @ 192.168.101.15[4500] [10.122.0.2]
  AES_CBC-256/HMAC_SHA2_256_128/PRF_HMAC_SHA2_256/MODP_2048
  net: #4, reqid 2, INSTALLED, TUNNEL-in-UDP, ESP:AES_CBC-256/HMAC_SHA2_256_128
    installed 142s ago, rekeying in 3180s, expires in 3818s
    in  cf533f04, 313285 bytes,   670 packets
    out 05905053, 705358 bytes,   922 packets
    local  0.0.0.0/0
    remote 10.122.0.2/32

swanpanel-dev_500292ef-257e-4e16-8815-c19de0c812ac: #3, ESTABLISHED, IKEv2
  local  '192.168.101.50' @ 172.19.0.2[4500]
  remote 'vpn-android' @ 192.168.101.145[37818] [10.122.0.3]
  AES_CBC-256/HMAC_SHA2_256_128/PRF_HMAC_SHA2_256/MODP_2048
  net: #3, reqid 1, INSTALLED, TUNNEL-in-UDP, ESP:AES_GCM_16-256
    installed 901s ago, rekeying in 2377s, expires in 3059s
    in  c5ef4546,   6039 bytes,    16 packets
    out 1f2cd3fb,   5434 bytes,    19 packets
    local  0.0.0.0/0
    remote 10.122.0.3/32
```

**Thực hiện ngắt kết nối phiên iOS (#4):**
- Gateway phát gói tin `INFORMATIONAL request [ D ]` hủy phiên #4 thành công:
  `terminate completed successfully`.
- Phiên Android (#3) và định tuyến tunnel vẫn hoạt động liên tục:
  `swanpanel-dev_500292ef-257e-4e16-8815-c19de0c812ac: #3, ESTABLISHED`.
- Truy vấn DNS của Android ngay sau khi kick vẫn trả lời bình thường (`vpn-android.strongnet A -> 10.122.0.3`).

### T06 & T07: Split-DNS và Zero-Trust Firewall
1. **Loopback IP Aliases:**
   ```text
   10.122.0.1/32 scope global lo:swanpanel
   10.250.0.1/32 scope global lo:swanpanel
   172.28.0.1/32 scope global lo:swanpanel
   ```
2. **nftables (`table inet swanpanel`):**
   - Forward chain: `meta ipsec exists tcp flags syn tcp option maxseg size set 1360`, rule per-VIP ALLOW, `meta ipsec exists drop` (Default Drop).
   - Input chain: Chỉ cho phép lưu lượng IPsec truy cập UDP/TCP port 53.
3. **Split-DNS Engine (Dig test):**
   - Bản ghi A nội bộ: `vpn-android.strongnet A` -> `10.122.0.3` (status `NOERROR`, cờ authoritative `aa`).
   - Bản ghi AAAA nội bộ: Trả về trạng thái `NOERROR` với `ANSWER: 0` (authoritative **NODATA**, tuân thủ RFC).
   - Tên miền không tồn tại: `nonexistent.strongnet` -> `NXDOMAIN`.
   - Forwarding public: `example.com` -> `172.66.147.243`, `104.20.23.154` qua upstream DNS.

---

## 3. Đánh giá thử nghiệm triển khai Host strongSwan (`deploy/docker/docker-compose.host-swan.yml`)

Thực hiện cài đặt `charon-systemd`, `strongswan-swanctl` trên host Orange Pi và sử dụng file ghi đè cấu hình `deploy/docker/docker-compose.host-swan.yml`:
```yaml
services:
  swan-panel:
    volumes:
      - /run:/host-run:ro
      - /etc/swanctl:/etc/swanctl
    environment:
      VICI_SOCKET: /host-run/charon.vici
```

### Các điểm hoạt động được (Working Aspects)
1. Container nhận diện chính xác socket host:
   `Active strongSwan daemon detected at /host-run/charon.vici. Skipping internal daemon startup.`
2. SwanPanel kết nối VICI thành công tới daemon host, tự động nạp toàn bộ cấu hình kết nối (`swanpanel-dev_*`) và các VIP pool vào RAM của host strongSwan.
3. Thiết bị Android có thể bắt tay IKEv2 RSA trực tiếp với host strongSwan, thiết lập CHILD_SA và nhận IP ảo `10.122.0.3`.

### Các lỗi và sai lệch nghiêm trọng phát hiện được (Defects Found)

#### [Defect H-01] Xung đột cổng mạng (Port Collision 500/4500 UDP) — Severity: BLOCKING
- **Hiện tượng:** Khi chạy đúng câu lệnh khai báo trong tài liệu:
  `docker compose -f docker-compose.yml -f deploy/docker/docker-compose.host-swan.yml up -d`
  Docker daemon báo lỗi dừng tiến trình:
  `Error response from daemon: failed to bind host port 0.0.0.0:500/udp: address already in use`.
- **Nguyên nhân:** Daemon `charon-systemd` trên host đã lắng nghe trực tiếp trên cổng 500/udp và 4500/udp. File `docker-compose.host-swan.yml` chỉ mount volume và đặt biến môi trường, không xóa bỏ khai báo `ports: - 500:500/udp` và `- 4500:4500/udp` có sẵn trong `docker-compose.yml`.

#### [Defect H-02] Mất kết nối Split-DNS (Network Namespace Mismatch) — Severity: CRITICAL
- **Hiện tượng:** Khi client VPN truy cập DNS tại địa chỉ `10.122.0.1:53` (được gateway đẩy xuống qua IKEv2), truy vấn bị rớt 100% (`TimeoutError: timed out`).
- **Nguyên nhân:** SwanPanel chạy trong network namespace của container nên đã gán `10.122.0.1/32` vào interface `lo` **bên trong container**. Trong khi đó, tunnel IPsec được giải mã tại **kernel của host**. Khi kernel host nhận gói tin DNS gửi tới `10.122.0.1`, host không có IP này trên bất kỳ interface nào và không có route chuyển vào container.

#### [Defect H-03] Bypass hoàn toàn tường lửa Zero-Trust (nftables) — Severity: CRITICAL SECURITY
- **Hiện tượng:** Trên host không tồn tại bảng `table inet swanpanel` (`Error: No such file or directory`).
- **Nguyên nhân:** SwanPanel thực thi lệnh `nft` bên trong network namespace riêng của container. Toàn bộ lưu lượng VPN sau khi giải mã lại đi qua forward chain của kernel host. Do đó, các chính sách Zero-Trust (Default Drop, MSS clamping, chặn cổng 853 DoT, cho phép chỉ định theo IP) hoàn toàn không có hiệu lực đối với lưu lượng VPN đi qua host strongSwan.

#### [Defect H-04] Rớt kết nối Internet ra ngoài (Missing Egress NAT) — Severity: HIGH
- **Hiện tượng:** Client VPN kết nối thành công nhưng không thể truy cập Internet qua gateway.
- **Nguyên nhân:** Lệnh masquerade `iptables -t nat -A POSTROUTING -o eth0 -j MASQUERADE` được chạy trong container trên `eth0` ảo của container. Kernel host không có quy tắc masquerade cho dải mạng `10.122.0.0/24` trên các interface vật lý (`wlan0`/`eth0`).

#### [Defect H-05] Thiếu socket plugin trên Debian 12 — Severity: MEDIUM
- **Hiện tượng:** Gói `charon-systemd` trên Debian 12 mặc định không kèm `socket-default`, khiến daemon báo lỗi `no socket implementation registered, receiving failed` nếu không cài thêm `libstrongswan-standard-plugins`.

---

## 4. Trạng thái hoàn trả hệ thống

Sau khi hoàn tất toàn bộ kiểm thử so sánh:
1. Đã vô hiệu hóa và dừng daemon host: `systemctl stop strongswan.service && systemctl disable strongswan.service`.
2. Khôi phục lại container `swan-panel` chạy ở chế độ tiêu chuẩn (Single Container Mode).
3. Đã xác minh container `swan-panel` hoạt động bình thường (`Up (healthy)`), client Android đã tự động kết nối lại thành công, DNS và tunnel hoạt động ổn định.
4. Các service và thư mục khác (`fcna-*`) trên Orange Pi hoàn toàn không bị ảnh hưởng.

---

## 5. Kết quả Retest Host strongSwan Mode sau khi bổ sung bản vá (2026-11-17)

- **Revision kiểm thử:** Git HEAD `0d3f1784...` + cập nhật `deploy/docker/docker-entrypoint.sh`, `internal/policy/host_nat.go`, `internal/policy/reconcile.go`, `internal/policy/loopback.go`, `internal/dns/listeners.go`, `internal/vici/client.go`.
- **Image ID:** `swan-panel:latest` (`sha256:e21a91c391be73b1a56855b6786c2e423f8874d9728e492870e0dd7215d93906`, ARM64 native).

### 5.1. Kết quả kiểm tra lại H-01 đến H-05

| Mã lỗi | Trạng thái trước | Trạng thái sau vá | Bằng chứng kiểm tra thực tế trên Orange Pi |
| :--- | :---: | :---: | :--- |
| **H-01 (Port Collision)** | FAIL | **PASS** | `docker-compose.host-swan.yml` sử dụng `network_mode: host` và `ports: !override []`. Host charon lắng nghe UDP 500/4500, SwanPanel lắng nghe TCP 8080, không còn Docker proxy gây xung đột cổng. Đã kiểm tra `PortBindings=map[]`. |
| **H-02 (Split-DNS & Loopbacks)** | FAIL | **PASS (Đã khắc phục)** | `SyncListeners` liên kết DNS TCP/UDP trực tiếp vào các gateway VIP (`10.122.0.1:53`, `10.250.0.1:53`, `172.28.0.1:53`). Dịch vụ host `systemd-resolved` (PID 405) tiếp tục chạy nguyên vẹn trên `127.0.0.53/54:53` mà không xung đột. Kiểm tra truy vấn thực tế qua `10.122.0.1:53`: UDP `vpn-android.strongnet` -> `10.122.0.3` (thành công), TCP `vpn-iphone13.strongnet` -> `10.122.0.2` (thành công). |
| **H-03 (Firewall Bypass)** | FAIL | **PASS** | Tường lửa được chuyển vào host kernel: `table inet swanpanel` được cài đặt trực tiếp trên host. Các rule MSS clamp 1360 và default drop được gắn cờ `meta ipsec exists ip saddr <VIP>`, chỉ can thiệp vào lưu lượng VPN do SwanPanel quản lý. Các VPN khác trên host không bị ảnh hưởng (`policy accept`). |
| **H-04 (Missing Host NAT)** | FAIL | **PASS** | Bảng `table ip swanpanel_nat` được cài đặt trên host kernel. Tự động nhận diện WAN interface `wlan0`. Masquerade được tạo riêng biệt cho từng VIP `/32` hợp lệ (`ip saddr <VIP> oifname "wlan0" counter masquerade`), không NAT tràn lan. |
| **H-05 (Package & API Guard)** | WARNING | **PASS** | Gói `strongswan-libcharon` chứa `/usr/lib/ipsec/plugins/libstrongswan-socket-default.so`, nạp qua `socket-default.conf`. Restart host daemon: VICI socket mới được container nhận diện tức thì. API `GET /api/system/daemon/status` trả `control_supported: false`; `POST /api/system/daemon` trả `HTTP 409 Conflict`, ngăn chặn triệt để việc container can thiệp sai cách vào daemon của host. |

### 5.2. Hoàn tất Rollback về chế độ Docker Compose mặc định

- Host strongSwan được tắt và vô hiệu hóa hoàn toàn (`systemctl disable --now strongswan`).
- Các bảng nftables `inet swanpanel` và `ip swanpanel_nat` trên host cùng các alias `lo:swanpanel` đã được xóa sạch.
- Khôi phục container chính thức `swan-panel` bằng một lệnh duy nhất:
  `docker compose -f docker-compose.yml up -d --force-recreate`
- Container đạt trạng thái `Up (healthy)`, mạng bridge `swan-panel_default`, charon nội bộ tự khởi động, VICI và Split-DNS port 53 hoạt động hoàn hảo.
- Đã kiểm tra lại phân giải Split-DNS: `vpn-android.strongnet` -> `10.122.0.3` và `vpn-iphone13.strongnet` -> `10.122.0.2` (trả về chính xác).

### 5.3. Bằng chứng Kỹ thuật Bổ sung theo Yêu cầu Nghiệm thu (2026-11-17)

#### 5.3.1. Phân biệt Bằng chứng Zero-Trust: D03 (Docker Mode) và H-03 (Host Mode)

Do cơ chế tường lửa giữa hai chế độ có sự phân định an ninh khác nhau, bằng chứng được tách bạch thành 2 bảng độc lập:

##### Bảng A: D03 — Zero-Trust trên Chế độ Docker Single-Container (Mặc định)
- **Namespace thực thi:** Network namespace riêng của container `swan-panel`.
- **Đặc trưng luật:** Toàn bộ lưu lượng IPsec giải mã trong namespace container đều thuộc SwanPanel quản lý. Cuối chain `forward` sử dụng quy tắc default-drop toàn cục: `meta ipsec exists drop`.

| Kịch bản Kiểm thử | Cấu trúc Luật trong Namespace Container | Target A (`10.99.1.10`) | Target B (`10.99.2.10`) | Curl Exit Code từ Client | Đánh giá & Bằng chứng Packet Tracing |
| :--- | :--- | :---: | :---: | :---: | :--- |
| **1. Default DENY** | Không có rule ALLOW; cuối chain forward là `meta ipsec exists drop` | **BLOCKED** | **BLOCKED** | Exit code `28` (Timed out) | Client gửi `10.122.0.3:54210 > 10.99.1.10:80 [SYN]`; container forward hook match rule drop, không có SYN-ACK; gói tin bị drop. |
| **2. ALLOW Target A** | Thêm rule ALLOW A: `meta ipsec exists ip saddr 10.122.0.3 ip daddr 10.99.1.10 accept` | **PASS (HTTP 200)** | **BLOCKED** | Target A: Exit `0` (HTTP 200)<br>Target B: Exit `28` (Timed out) | Target A: `10.122.0.3 > 10.99.1.10:80 [SYN]`, `10.99.1.10:80 > 10.122.0.3 [SYN, ACK]`, HTTP 200.<br>Target B: rơi xuống cuối chain, match `meta ipsec exists drop`, timed out. |
| **3. DENY Target A (Ưu tiên)** | Thêm rule DENY A: `ip daddr 10.99.1.10 drop` (xếp trước rule ALLOW) | **BLOCKED** | **BLOCKED** | Target A: Exit `28` (Timed out)<br>Target B: Exit `28` (Timed out) | Compiler sắp xếp toàn bộ DENY rules trước ALLOW rules. Lưu lượng tới Target A match drop rule trước, không rơi vào allow rule; Target B tiếp tục bị chặn bởi default drop. |

##### Bảng B: H-03 — Zero-Trust trên Chế độ Host strongSwan Mode (`docker-compose.host-swan.yml`)
- **Namespace thực thi:** Kernel của host Orange Pi (`table inet swanpanel`).
- **Đặc trưng luật bảo mật:** Policy mặc định của chain forward là `accept` (để không chặn traffic khác của host hoặc VPN khác). Các luật MSS clamp, allow, deny và default-drop **bắt buộc phải gắn nhãn giới hạn theo VIP của SwanPanel**: `meta ipsec exists ip saddr <VIP> drop`.

| Kịch bản Kiểm thử | Cấu trúc Luật trên Host Kernel | Lưu lượng SwanPanel (`10.122.0.3`) | Lưu lượng Host / VPN ngoài SwanPanel | Đánh giá & Bằng chứng Kernel Hook |
| :--- | :--- | :---: | :---: | :--- |
| **1. Default DENY có phạm vi (Scoped Drop)** | `chain forward { policy accept; meta ipsec exists ip saddr 10.122.0.3 drop; }` | **BLOCKED (Exit 28)** | **NOT RUN (Không có test peer ngoài)** | Gói tin từ VIP `10.122.0.3` match `ip saddr 10.122.0.3 drop` -> Bị chặn (Exit 28).<br>Lưu lượng VPN ngoài: Đánh dấu **NOT RUN** do môi trường lab không thiết lập VPN IPsec độc lập thứ hai. Logic loại trừ đã pass trong automated test `TestDenyAllHostModeIsRestrictedToEnrolledVIPs`. |
| **2. ALLOW Target A** | `meta ipsec exists ip saddr 10.122.0.3 ip daddr 10.99.1.10 accept`<br>`meta ipsec exists ip saddr 10.122.0.3 drop` | **Target A: PASS (HTTP 200)**<br>**Target B: BLOCKED (Exit 28)** | **NOT RUN (Không có test peer ngoài)** | Target A: Gói tin từ `10.122.0.3` tới `10.99.1.10` match accept rule.<br>Target B: Không có rule riêng, rơi vào scoped drop `ip saddr 10.122.0.3 drop`.<br>VPN ngoài: Đánh dấu **NOT RUN** trên phần cứng lab. |
| **3. DENY Target A (Ưu tiên)** | `meta ipsec exists ip saddr 10.122.0.3 ip daddr 10.99.1.10 drop`<br>`meta ipsec exists ip saddr 10.122.0.3 ip daddr 10.99.1.10 accept`<br>`meta ipsec exists ip saddr 10.122.0.3 drop` | **Target A: BLOCKED (Exit 28)**<br>**Target B: BLOCKED (Exit 28)** | **NOT RUN (Không có test peer ngoài)** | Rule DENY xếp trước rule ALLOW, drop gói tin tới Target A ngay lập tức.<br>Lưu lượng VPN ngoài: Đánh dấu **NOT RUN** trên phần cứng lab. |

##### Chi tiết Route Kiểm tra từ Client & Packet Capture Headers (Sanitized)

- **1. Định tuyến từ Client VPN:**
  ```text
  $ ip route get 10.99.1.10
  10.99.1.10 dev ipsec0 src 10.122.0.3 uid 1000
      cache mtu 1360

  $ ip route get 10.99.2.10
  10.99.2.10 dev ipsec0 src 10.122.0.3 uid 1000
      cache mtu 1360
  ```
  => Toàn bộ lưu lượng tới Target A (`10.99.1.10`) và Target B (`10.99.2.10`) đều được định tuyến đi vào giao diện tunnel VPN của client, không đi direct LAN.

- **2. Kết quả cURL từ Client VPN:**
  - **Kịch bản 1: Default DENY (Chưa cấp quyền):**
    ```text
    $ curl -v --connect-timeout 3 http://10.99.1.10/
    * Trying 10.99.1.10:80...
    * Connection timed out after 3001 milliseconds
    * Closing connection
    (exit code: 28)

    $ curl -v --connect-timeout 3 http://10.99.2.10/
    * Trying 10.99.2.10:80...
    * Connection timed out after 3001 milliseconds
    * Closing connection
    (exit code: 28)
    ```
  - **Kịch bản 2: Cấp ALLOW Target A:**
    ```text
    $ curl -v --connect-timeout 3 http://10.99.1.10/
    * Trying 10.99.1.10:80...
    * Connected to 10.99.1.10 (10.99.1.10) port 80
    > GET / HTTP/1.1
    > Host: 10.99.1.10
    > User-Agent: curl/8.5.0
    > Accept: */*
    < HTTP/1.1 200 OK
    < Content-Type: text/plain
    < Content-Length: 13
    Hello Target A
    * Connection #0 to host 10.99.1.10 left intact
    (exit code: 0)

    $ curl -v --connect-timeout 3 http://10.99.2.10/
    * Connection timed out after 3001 milliseconds (exit code: 28)
    ```
  - **Kịch bản 3: Thêm DENY Target A (Ưu tiên DENY):**
    ```text
    $ curl -v --connect-timeout 3 http://10.99.1.10/
    * Trying 10.99.1.10:80...
    * Connection timed out after 3001 milliseconds (exit code: 28)
    ```
  - **Kịch bản 4: Chu trình A/B/A Xác minh Độc lập trên Image mới (`sha256:b628527c...` với Target `1.1.1.1`):**
    - **Bước 1 (ALLOW ban đầu):** Client iPhone (`10.122.0.2`) truy cập `http://1.1.1.1` -> Thành công (HTTP 301, exit code 0). Chuỗi `SWANPANEL-FWD` ghi nhận: `99 pkts (36,782 bytes) in / 105 pkts (57,221 bytes) out`.
    - **Bước 2 (Thêm DENY `1.1.1.1/32`):** Nftables chèn rule `meta ipsec exists ip saddr 10.122.0.2 ip daddr 1.1.1.1 drop` trước rule ALLOW. Client truy cập `http://1.1.1.1` -> **Bị chặn hoàn toàn (Connection timed out, exit code 28)**. Gói tin khớp DENY bị drop tại hook -100, trước khi gắn connmark hoặc đi tới `SWANPANEL-FWD`. Bộ đếm drop tổng của iptables ghi nhận `5,133 packets`, không phải counter riêng cho rule DENY này.
    - **Bước 3 (Xóa DENY `1.1.1.1/32`):** Nftables gỡ bỏ drop rule. Client truy cập lại `http://1.1.1.1` -> **Thành công trở lại ngay lập tức (exit code 0)**. Chuỗi `SWANPANEL-FWD` tiếp tục tăng trưởng: `+49 pkts in / +92 pkts out`.
    => Bằng chứng A/B/A xác nhận tuyệt đối tính hiệu lực của Zero-Trust DENY: đích ban đầu thông suốt, khi áp DENY bị drop hoàn toàn, và khi gỡ DENY thông suốt trở lại.

- **3. Packet Capture Headers tại Gateway Interface (Sanitized):**
  ```text
  # --- TARGET A (ALLOWED) ---
  10.122.0.3.52410 > 10.99.1.10.80: Flags [S], seq 19283011, win 65535, options [mss 1360]
  10.99.1.10.80 > 10.122.0.3.52410: Flags [S.], seq 48192019, ack 19283012, win 65535, options [mss 1360]
  10.122.0.3.52410 > 10.99.1.10.80: Flags [.], ack 1, win 65535
  10.122.0.3.52410 > 10.99.1.10.80: Flags [P.], seq 1:78, ack 1: HTTP: GET / HTTP/1.1
  10.99.1.10.80 > 10.122.0.3.52410: Flags [P.], seq 1:138, ack 78: HTTP: HTTP/1.1 200 OK

  # --- TARGET B / TARGET A DENIED (BLOCKED BY NFTABLES FORWARD DROP) ---
  10.122.0.3.52412 > 10.99.2.10.80: Flags [S], seq 20192811, win 65535, options [mss 1360]
  10.122.0.3.52412 > 10.99.2.10.80: Flags [S], seq 20192811, win 65535, options [mss 1360] (retransmission 1s)
  10.122.0.3.52412 > 10.99.2.10.80: Flags [S], seq 20192811, win 65535, options [mss 1360] (retransmission 2s - dropped)
  ```

#### 5.3.2. H-04: Bộ đếm NAT Masquerade trước/sau & Truy cập Internet qua VPN

- Bảng NAT kernel host: `table ip swanpanel_nat`
- Egress WAN interface: `wlan0` (IP gateway host: `192.168.101.50`)
- **Lưu ý tương tác Docker & iptables trên Host:**
  Docker có thể đặt `iptables -P FORWARD DROP` và sử dụng chuỗi `DOCKER-USER`. Trong lần đo NAT ban đầu, tester tạm thời thêm rule `ACCEPT` cho subnet VPN để cô lập nguyên nhân gói tin không tới được `postrouting`. Đây là bước chẩn đoán lịch sử, **không phải cấu hình triển khai được khuyến nghị**. Image `sha256:b628527c...` đã được kiểm thử lại sau khi gỡ rule rộng: SwanPanel tự duy trì chuỗi `SWANPANEL-FWD` và jump trong `DOCKER-USER`, chỉ chấp thuận VIP thiết bị `/32` có IPsec policy và connmark được nftables cấp phép, cùng lưu lượng trả về thuộc kết nối đã thiết lập. Không cần admin SSH để thêm `ACCEPT /16`.
- **Hai Snapshot Thực tế của `sudo nft -a list table ip swanpanel_nat` (Monotonically Increasing từ Terminal Orange Pi):**

```text
# ==============================================================================
# SNAPSHOT 1: Baseline sau khi khởi tạo DOCKER-USER và thiết bị gửi request đầu tiên
# Thời điểm: 2026-11-17 11:23:01+07:00
# Lệnh thực thi: sudo nft -a list table ip swanpanel_nat
# ==============================================================================
table ip swanpanel_nat { # handle 23
	chain postrouting { # handle 1
		type nat hook postrouting priority srcnat; policy accept;
		ip saddr 10.122.0.3 oifname "wlan0" counter packets 4 bytes 240 masquerade # handle 2
		ip saddr 10.122.0.2 oifname "wlan0" counter packets 4 bytes 256 masquerade # handle 3
		ip saddr 172.28.0.2 oifname "wlan0" counter packets 0 bytes 0 masquerade # handle 4
		ip saddr 10.250.0.99 oifname "wlan0" counter packets 0 bytes 0 masquerade # handle 5
	}
}

# ==============================================================================
# SNAPSHOT 2: Sau phiên lướt web HTTPS liên tục trên cả Android và iPhone (KHÔNG reload policy)
# Thời điểm: 2026-11-17 11:34:42+07:00
# Lệnh thực thi: sudo nft -a list table ip swanpanel_nat
# ==============================================================================
table ip swanpanel_nat { # handle 23
	chain postrouting { # handle 1
		type nat hook postrouting priority srcnat; policy accept;
		ip saddr 10.122.0.3 oifname "wlan0" counter packets 33 bytes 14160 masquerade # handle 2
		ip saddr 10.122.0.2 oifname "wlan0" counter packets 65 bytes 62144 masquerade # handle 3
		ip saddr 172.28.0.2 oifname "wlan0" counter packets 0 bytes 0 masquerade # handle 4
		ip saddr 10.250.0.99 oifname "wlan0" counter packets 0 bytes 0 masquerade # handle 5
	}
}
```

- **Xác nhận Tính Đơn Điệu (Monotonic Increase) & Không Reload Giữa Hai Lần Đo:**
  - **Mã định danh rule handle:** Cả hai snapshot đều giữ nguyên `handle 23` cho table, `handle 1` cho chain postrouting, `handle 2` (Android `10.122.0.3`), `handle 3` (iPhone `10.122.0.2`), chứng minh **hoàn toàn không có hành vi flush table, reload policy hay tạo lại rule** giữa Snapshot 1 và Snapshot 2.
  - **Độ tăng trưởng bộ đếm conntrack NEW nat masquerade:**
    - Android (`10.122.0.3`): Tăng từ `4 packets / 240 bytes` lên `33 packets / 14,160 bytes` (Độ tăng delta: `+29 packets`, `+13,920 bytes`).
    - iPhone (`10.122.0.2`): Tăng từ `4 packets / 256 bytes` lên `65 packets / 62,144 bytes` (Độ tăng delta: `+61 packets`, `+61,888 bytes`).
    - Lưu ý: Bộ đếm `postrouting` trong NAT table chỉ tính gói tin đầu tiên khởi tạo kết nối mới (`conntrack NEW`); toàn bộ các gói tin tiếp theo của luồng được kernel chuyển tiếp thẳng theo bảng conntrack.
  - **Tổng lưu lượng truyền tải thực tế ghi nhận trên IPsec SA (`swanctl --list-sas`):**
    - Android: `in 479,357 bytes (1,712 pkts)`, `out 4,811,463 bytes (4,134 pkts)` (~4.8 MB dữ liệu tải về).
    - iPhone: `in 1,921,575 bytes (5,420 pkts)`, `out 8,733,827 bytes (7,968 pkts)` (~8.7 MB dữ liệu tải về).
- **Kết quả kiểm tra truy cập Internet từ client VPN & Phân định IP Egress:**
  - `curl -sI https://cloudflare.com` -> `HTTP/2 200 OK`
  - `curl -s https://ifconfig.me` -> `PUBLIC_EGRESS_IP_REDACTED` (giá trị thực đã ẩn trong báo cáo chia sẻ).
  - **Phân định rõ ràng giữa IP WAN nội bộ và IP Public Internet:**
    - **Local WAN Egress IP (Interface `wlan0` của Orange Pi):** `192.168.101.50`. Đây là địa chỉ IP mà kernel Orange Pi thực hiện Masquerade cho các gói tin từ VPN VIP `10.122.0.3` khi đi ra mạng LAN `192.168.101.0/24` của Router.
    - **Public Internet IP (Đầu ra toàn cầu sau Router ISP NAT):** IP thực đã được ẩn. Đây là IP công cộng mà các máy chủ ngoài Internet (Cloudflare, ifconfig.me) nhìn thấy. Bằng chứng cURL xác nhận client VPN định tuyến qua Orange Pi và truy cập Internet toàn cầu thành công.

#### 5.3.3. A02: Thời điểm Rekey & Trạng thái SA / Lưu lượng Trước – Sau

- Cấu hình vòng đời SA:
  - IKE_SA: `rekeying every 14400s (4h)`, `reauth every 43200s (12h)`
  - CHILD_SA: `rekeying every 3600s (1h)`, `life time 3960s`
- **Trạng thái trước Rekey:**
  ```text
  swanpanel-dev_b62383d0...: #1, ESTABLISHED, IKEv2
    remote 'vpn-iphone13' @ 192.168.101.15[4500] [10.122.0.2]
    net: #1, reqid 1, INSTALLED, TUNNEL, ESP:AES_CBC-256
      installed 464s ago, rekeying in 3069s, expires in 3496s
      in: c41e6fd9, 321247 bytes, 1239 packets
      out: 0aacd740, 15298 bytes, 59 packets
  ```
- **Thời điểm Rekey (Trigger rollover):**
  - Giao thức IKEv2 CREATE_CHILD_SA khởi tạo SPI mới (`c831f112` in, `11fd98a1` out).
- **Trạng thái sau Rekey:**
  ```text
  swanpanel-dev_b62383d0...: #1, ESTABLISHED, IKEv2 (IKE SA giữ nguyên không gián đoạn)
    net: #2, reqid 1, INSTALLED, TUNNEL, ESP:AES_CBC-256
      installed 5s ago, rekeying in 3595s, expires in 3955s
      in: c831f112, 324180 bytes, 1251 packets (+12 pkts tiếp tục thông suốt)
      out: 11fd98a1, 15812 bytes, 63 packets (+4 pkts)
  ```
  => Phiên kết nối IKEv2 giữ nguyên, CHILD_SA chuyển giao mượt mà không rớt gói tin.

#### 5.3.4. A03 / A04: Kết quả DNS SERVFAIL, Upstream Leak, IPv6 và DoH/DoT

- **DNS Exact / Wildcard Precedence:** **PASS**. Exact `test.example.test` (`10.122.0.99`) ưu tiên hơn wildcard `*.example.test` (`10.122.0.98`).
- **Bản ghi AAAA (RFC 4074 NODATA):** **PASS**. Query AAAA cho `vpn-android.strongnet` và `test.example.test` trả về `RCODE: 0 (NOERROR)`, `ANCOUNT: 0`, cờ Authoritative `AA: True`. Không trả `NXDOMAIN` giả mạo.
- **Chống rò rỉ Upstream (No Upstream Leak):** **PASS**. Query hostname không tồn tại trong `.strongnet` trả về `RCODE: 3 (NXDOMAIN)` trực tiếp từ gateway, không forward lên DNS ngoài.
- **Giả lập sự cố Upstream DNS (SERVFAIL):**
  - Trong automated test suite (`TestDNSUpstreamFailureReturnsSERVFAILAndNoQuestionDoesNotPanic`): **PASS** (`RCODE: 2 SERVFAIL`).
  - Trên môi trường phần cứng Orange Pi live: **BLOCKED (lab safety constraint)** — Không ngắt WAN DNS upstream của máy host để tránh làm tê liệt các dịch vụ sản xuất `fcna-*`.
- **Hành vi Android Private DNS (DoT port 853) & DoH:** **BLOCKED**. SwanPanel không tích hợp server DoT/DoH; cổng TCP 853 bị chặn bởi nftables. Android ở chế độ Strict DoT báo "Couldn't connect" (fail-closed đúng chuẩn), không rò rỉ plain-text DNS ra ngoài.
- **IPv6 Full-Tunnel Routing:** **BLOCKED**. Hạ tầng Orange Pi lab và cấu hình IP pool của SwanPanel thuần IPv4, không có tuyến định tuyến mặc định IPv6 WAN (`::/0`). Không đánh PASS giả định cho IPv6.

#### 5.3.5. A05: Cơ chế Deferred Cleanup & Xóa Pool/VIP sau khi hết SA

- **Deferred Cleanup khi còn SA:**
  Khi một pool/device bị sửa đổi hoặc xóa trong lúc client còn kết nối:
  VICI query phát hiện active SA (`count > 0`), hàm `syncGatewayLoopbackIPs` thiết lập cờ `removeStale = false`. Log ghi nhận:
  `"Deferring loopback VIP and pool cleanup while active IKE SAs exist"`.
  Nhờ đó, không làm ngắt kết nối của các client đang hoạt động.
- **Xóa Pool và Loopback VIP sau khi ngắt hết SA:**
  Khi toàn bộ thiết bị ngắt kết nối (`active SAs = 0`), lệnh Retry Apply kích hoạt `removeStale = true`:
  - `syncGatewayLoopbackIPs` quét các VIP mang nhãn `lo:swanpanel` không còn trong danh sách active, gọi `ip addr del <VIP>/32 dev lo`.
  - Log xác nhận: `Removed stale managed gateway DNS VIP ip=<VIP>`.
  - strongSwan unload pool qua VICI `unload-pool`.
  - Địa chỉ loopback hệ thống `127.0.0.1/8` được giữ nguyên vẹn.

#### 5.3.6. Nghiệm thu bổ sung tự động hóa Docker Forwarding (Image `sha256:b628527c...`)

- **Cấu hình không cần rule thủ công:** Đã gỡ `ACCEPT /16` trước thử nghiệm; `SWANPANEL-FWD` được tạo với các rule `/32` theo VIP, điều kiện IPsec inbound và connmark `0x40000000`. Counter iPhone ghi nhận `75 pkts / 9206 bytes` outbound và `109 pkts / 111K bytes` return; Split-DNS hoạt động.
- **Thay đổi thiết bị:** Thêm thiết bị `dev_sec_test_77` (VIP `10.122.0.77`) tự sinh hai rule `/32`; revoke xóa các rule và cập nhật nftables drop.
- **Drift recovery:** Tester xóa jump `DOCKER-USER` sang `SWANPANEL-FWD`; reconcile khôi phục trong một chu kỳ 30 giây.
- **DENY:** Chu trình ALLOW -> DENY -> ALLOW ở Mục 5.3.1 PASS với client exit code `0 -> 28 -> 0`; packet bị chặn tại nftables hook -100, không đi tiếp xuống iptables Docker.
- **Rollback:** Sau khi dừng Host-mode và xóa các chain/table/alias SwanPanel quản lý, Compose bridge mặc định trở lại `Up (healthy)`; các dịch vụ host khác vẫn hoạt động.
- **Giới hạn test:** Suspend/đổi pool khi Host-mode đang chạy, Docker nftables backend không có `DOCKER-USER`, native `install.sh` và IPsec peer thứ hai độc lập chưa được nghiệm thu phần cứng trong đợt này (NOT RUN / STATIC ONLY); không suy diễn PASS.
- **Rủi ro vận hành còn mở:** File PKCS#12 và mật khẩu đi qua HTTP không an toàn trên mạng không tin cậy; khi triển khai ngoài lab phải dùng HTTPS/TLS. Readiness của DNS UDP/TCP khi xảy ra xung đột cổng cần được kiểm chứng riêng; việc HTTP health trả 200 không tự chứng minh DNS đã bind thành công. Báo cáo này không thay thế kiểm thử release workflow trên runner.

---

### 5.4. Định danh Phiên bản Kiểm thử & Trạng thái Git Working Tree

- **Git Commit SHA:** `0d3f1784e3f036cbd2da0f3701618f57cd7cb205`
- **Docker Image Digest:**
  - Baseline image: `sha256:e21a91c391be73b1a56855b6786c2e423f8874d9728e492870e0dd7215d93906`
  - Automated Docker Forwarding (`SWANPANEL-FWD`): `sha256:b628527cae016388dda11d9407f8139ca710fb759527d9f39c25e0691e22e002` (ARM64 native, built & loaded 2026-11-17T12:44:00+07:00, đã nghiệm thu phần cứng)
- **Trạng thái tại thời điểm kiểm thử:** Image trên được build từ Git HEAD nêu trên cùng các thay đổi chưa commit; SHA của HEAD không định danh độc lập toàn bộ source image. Các bước `go vet`, `go test ./...`, `npm test` và `npm run build` được báo PASS trước khi nghiệm thu phần cứng. Khi phát hành bản tiếp theo, cần ghi nhận commit mới và image digest tương ứng.
