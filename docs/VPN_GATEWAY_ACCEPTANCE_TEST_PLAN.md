# SwanPanel P1 — Hướng dẫn nghiệm thu thủ công (QA / Operator)

**Trạng thái: HOÀN TẤT NGHIỆM THU (PASS trên các tính năng được hỗ trợ; BLOCKED có điều kiện cho IPv6 / DoT / Live WAN Outage)**
**Phạm vi:** đúng commit/revision được bàn giao, Docker IKEv2, systemd native, UI, client VPN, release.
**Nguyên tắc:** PASS chỉ khi có kết quả thực thi. BLOCKED/NOT RUN không được tính PASS.

> CẢNH BÁO: Toàn bộ thử nghiệm restart, firewall, CA, revoke và fault injection chỉ chạy trên VM / lab disposable, không có dữ liệu khách hàng. Không chạy trên production. Không gửi JWT, private key, PKCS#12, setup token hoặc mật khẩu vào ticket, screenshot hay log.

## 0. Chuẩn bị và thông tin cần ghi

| Trường | Người test điền |
| --- | --- |
| Ngày/giờ, múi giờ | |
| Tester | |
| Commit SHA và branch/tag | |
| Linux distro, kernel, architecture | |
| Docker / Compose / strongSwan version | |
| Deployment type (Docker, native, cả hai) | |
| Test VPN endpoint/IP/FQDN | |
| Test client: Linux, Windows, macOS, iOS, Android (model/version) | |
| Protected Target A (cho phép), Target B (chặn), TCP port | |
| Thư mục lưu bằng chứng và link CI | |

Trên máy Linux thử nghiệm:

    umask 077
    mkdir -p "$HOME/swanpanel-p1-evidence"
    cd /path/to/swan-panel
    git rev-parse HEAD | tee "$HOME/swanpanel-p1-evidence/commit.txt"
    uname -a | tee "$HOME/swanpanel-p1-evidence/kernel.txt"

Mọi log hoặc packet capture cần lọc dữ liệu nhạy cảm trước khi chia sẻ. Không thu thập file khóa riêng. Backup/snapshot VM test trước khi thay đổi kernel firewall hoặc PKI.

## 1. D01 (P0): Chuẩn bị Linux Docker/XFRM

1. Dùng Linux VM/host disposable có Docker Engine + Compose plugin, curl, jq, NET_ADMIN và kernel hỗ trợ XFRM. Không dùng Docker trên máy production.
2. Chạy:

    docker info
    docker compose version
    command -v curl jq
    test -e /proc/net/xfrm_stat && echo "XFRM exposed" || echo "BLOCKED: XFRM missing"

3. Nếu Docker daemon không chạy, XFRM không có, hoặc container không được NET_ADMIN: đánh dấu BLOCKED; chuyển sang Linux runner khác. Không được ghi PASS.
4. Ghi lại kernel, Docker/Compose versions, command output.

Kỳ vọng: Docker API, Compose và kernel XFRM đều khả dụng.

## 2. D02 (P0): Chạy integration script thực

1. Checkout đúng commit cần nghiệm thu. Đảm bảo port TCP 18080 localhost chưa bị process khác sử dụng.
2. Chạy:

    cd /path/to/swan-panel
    umask 077
    bash -o pipefail -c 'bash scripts/test_docker_integration.sh 2>&1 | tee "$HOME/swanpanel-p1-evidence/docker-ikev2.log"'
    echo "exit=$?"

3. Kỳ vọng exit 0 và dòng: PASS: 11 Docker integration checks against real strongSwan and nftables.
4. Script tự tạo Compose project + volumes riêng, bind API trên 127.0.0.1:18080, và tự dọn sau khi kết thúc. Nếu exit 2 là BLOCKED; exit khác 0 là FAIL.
5. Script xác minh: VICI gateway/client; bootstrap admin; settings/apply; subnet/user/ALLOW; enroll VIP; Apple/Windows/Linux profile; VICI connections/pools; nftables table + default drop; daemon restart + restore; Linux client IKEv2 + SA + VIP; revoke và reconnect bị từ chối.

**Bằng chứng:** docker-ikev2.log (đã lọc secrets), exit code, Docker version, commit SHA.

**Quan trọng:** D02 chỉ kiểm tra sự hiện diện của nftables default deny, chưa chứng minh ALLOW/DENY qua packet test giữa các protected targets. Phải làm D03.

## 3. D03 (P0): ALLOW/DENY trên packet thật

1. Dựng Target A và Target B (TCP service thử nghiệm) trong mạng protected phía sau gateway. Xác nhận gateway truy cập hai target được. Dùng hai IP khác nhau; tester tự điền IP thực tế.
2. Đảm bảo client VPN không có đường mạng trực tiếp tới A/B. Từ client xem route:

    ip route get <TARGET_A_IP>
    ip route get <TARGET_B_IP>

3. Ở gateway/target quan sát packet để chứng minh đi qua VPN:

    sudo tcpdump -ni any 'host <TARGET_A_IP> or host <TARGET_B_IP>'

4. Khi chưa có ALLOW: từ client thử curl --connect-timeout 3 http://<TARGET_A_IP>:<PORT>/ ; phải thất bại.
5. Trong SwanPanel Access Rules, thêm ALLOW chỉ cho Target A; chờ Apply Status = applied. Curl A phải thành công, B vẫn thất bại.
6. Thêm DENY cụ thể tới A; chờ applied. Curl A phải thất bại. Xóa DENY; A lại hoạt động.
7. Trong khi thay đổi rule của A, quan sát một client/SA không liên quan: không được mất kết nối hàng loạt.
8. Xem sudo nft -a list table inet swanpanel trước/sau và lưu kết quả (che IP nhạy cảm nếu có).

**Kỳ vọng:** default deny; allow hẹp; deny thắng allow; không bypass management API; unrelated SA không rớt khi áp dụng policy thông thường.

**Bằng chứng:** client route, curl status, packet captures và nft ruleset trước/sau (không gửi private keys).

## 4. D04 (P0): Daemon restart, reconcile và trạng thái pending

1. Trong lab có ít nhất một device/connection test, chạy:

    sudo swanctl --stats
    sudo swanctl --list-conns
    sudo swanctl --list-pools
    sudo test -s /etc/swanctl/conf.d/swanpanel-gateway.conf
    sudo nft list table inet swanpanel

2. Restart **chỉ một** daemon trên gateway thử nghiệm: sudo systemctl restart strongswan (systemd) hoặc sudo ipsec restart (non-systemd).
3. Kiểm tra trong vòng khoảng 90 giây: VICI hoạt động, connection/pool còn đúng, Apply Status trở về applied, không còn stale connection.
4. Tắt daemon test, sau đó đổi một gateway setting từ UI; phải báo pending/failed, không được giả applied. Khởi động lại; nhấn Retry apply hoặc chờ chu kỳ retry; phải trở về applied.
5. Xuất log đã lọc:

    sudo journalctl -u swan-panel -u strongswan --since "YYYY-MM-DD HH:MM:SS" --no-pager

**Kỳ vọng:** cấu hình bền vững survives restart; retry khôi phục; trạng thái lỗi trung thực.

## 5. D05 (P0): Revoke certificate đã kết nối

1. Cấp device, client certificate và VIP dành riêng cho test. Kết nối VPN; xác nhận IKE/CHILD_SA trong swanctl --list-sas.
2. UI revoke: Cancel phải không thay đổi gì; mở lại, Confirm. Kiểm tra active SA biến mất, private identity không còn tải được.
3. Kiểm tra CRL (đây là DER CRL):

    sudo swanctl --load-creds --noprompt
    sudo openssl crl -inform DER -in /etc/swanctl/x509crl/swanpanel.crl -text -noout

4. Serial đã revoke phải có trong CRL. Client cũ reconnect bằng certificate cũ phải thất bại.
5. Restart strongSwan gateway; reconnect bằng certificate revoked vẫn phải thất bại. Client có certificate mới phải hoạt động bình thường.

**Kỳ vọng:** CRL có hiệu lực ngay và sau restart, không ảnh hưởng client hợp lệ khác.

## 6. N01 (P0): Native systemd — permission, persistence

1. Trên Linux VM khác với Docker test, triển khai **chính xác commit/binary cần nghiệm thu**. Chuẩn bị strongSwan charon-systemd, swanctl, nftables và service SwanPanel. Không lấy release cũ làm bằng chứng cho code mới.
2. Xác nhận /etc/swanctl và /etc/swan-panel đã được tạo bởi installer/configuration. Chạy:

    sudo systemctl daemon-reload
    sudo systemctl cat swan-panel
    sudo systemctl show swan-panel -p ProtectSystem -p ReadWritePaths -p User
    sudo systemctl is-active swan-panel
    sudo systemctl is-active strongswan
    sudo swanctl --stats

3. Mong đợi: ProtectSystem=full, ReadWritePaths=/etc/swanctl; strongSwan và SwanPanel active.
4. Tạo một device test, xác nhận file /etc/swanctl/conf.d/swanpanel-gateway.conf và các thư mục x509ca/, x509/, rsa/, x509crl/ có dữ liệu.
5. Chỉ kiểm tra **metadata** private key, không mở/nộp file:

    sudo stat -c '%a %U:%G %n' /etc/swanctl/rsa/server.key

6. Mong đợi private key quyền 600, không đọc được bởi user thường.
7. Restart sudo systemctl restart swan-panel, rồi sudo systemctl restart strongswan; xác minh connection/pool và Apply Status applied. Kiểm tra journal không có lỗi read-only filesystem hoặc permission denied kéo dài.

**Kỳ vọng:** sandbox systemd cho phép ghi **chỉ trong /etc/swanctl**, server key private, config/CRL survive restart.


## 7. U01 (P1): Select và theme trong trình duyệt thật

1. Mở Chrome và Firefox desktop; kiểm tra thêm viewport 375px nếu có.
2. Đăng nhập SwanPanel, lần lượt kiểm tra: Users, Resources, Access Rules, DNS, OIDC, System Logs và Server Settings.
3. Mở từng select ở Light và Dark: so sánh chiều cao, padding, màu chữ/option, dropdown icon, focus ring, disabled state; nội dung dài không bị cắt sai/tràn ngang.
4. Dùng Tab, Shift+Tab, mũi tên, Enter, Escape. Không được cần chuột cho thao tác cơ bản.
5. Chụp ảnh riêng từng trang/thiết lập gặp lỗi, ghi browser/version và viewport.

Kỳ vọng: select nhất quán trên các trang và hai theme. Nếu option nền/tương phản sai, đánh FAIL và kèm screenshot.

## 8. U02 (P1): Confirm các thao tác nguy hiểm

**Dùng toàn bộ tài khoản, device, rule, CA và subnet THỬ NGHIỆM; không đụng admin duy nhất.**

Lặp lại các bước sau cho từng loại: xóa user/group/subnet/access rule/DNS override/OIDC provider; revoke device; cấp/thu hồi admin; duyệt/từ chối user; ngắt IKE session; Stop/Restart daemon; Rotate Root CA; Apply Advanced JSON:

1. Nhấn nút hành động: dialog phải hiện đúng đối tượng và tác động; chưa có API mutation trước khi Confirm.
2. Cancel hoặc Escape: dialog đóng, dữ liệu backend không đổi; refresh để xác nhận.
3. Confirm hai lần liên tiếp thật nhanh: DevTools Network chỉ ghi **một request mutation**; có loading, không đóng giữa lúc pending.
4. Mô phỏng HTTP 409/503 bằng DevTools hoặc mock response: dialog phải giữ trạng thái lỗi, không thông báo thành công giả.
5. Khi dialog mở, Tab/Shift+Tab phải ở trong dialog; focus không được chuyển sang nút/form bên dưới. Escape chỉ đóng dialog trên cùng. Sau đóng, kiểm tra focus trở về vị trí có thể thao tác (ghi FAIL nếu mất).
6. Device revoked không được revoke lại. Khi tạo user admin xong, mở form tạo user khác: checkbox cấp admin mặc định phải tắt.

**Riêng Rotate Root CA:** chỉ làm trên môi trường sạch chưa có device/revocation, sao lưu snapshot trước; không reset CA đã cấp cho khách hàng.

Kỳ vọng: Cancel không thay đổi state, lỗi API không bị che, không double-submit, focus/keyboard không vượt dialog.

## 9. U03 (P1): Auth, RBAC, last-admin, subnet/rule consistency

1. User thường truy cập trang quản trị và endpoint admin: 403; không thấy device/private key của user khác.
2. Với hai admin test, gỡ quyền một admin đang đăng nhập ở trình duyệt thứ hai: admin API phải bị từ chối ngay dù JWT chưa hết hạn.
3. Thử hạ quyền hoặc xóa admin **cuối cùng** trên hệ thống test: phải bị 409. Test mật khẩu 11 byte và 73 byte bị từ chối; mật khẩu hợp lệ 12–72 byte được chấp nhận.
4. Xóa subnet có device: 409. Xóa subnet có rule tham chiếu: 409. Xóa subnet không tồn tại: 404. Xóa subnet rỗng: 200; lần xóa thứ hai: 404.
5. Thử tạo rule trỏ vào subnet đã xóa: bị từ chối. Hai request đồng thời tạo rule/xóa subnet hoặc enroll device/xóa subnet: **không xuất hiện orphan**. API concurrency đã được kiểm thử tự động; kiểm tra lại trên UI/API nếu có kịch bản UAT.
6. Dùng response status và dashboard/apply-status để phân biệt thao tác đã lưu DB nhưng chưa đồng bộ gateway (202 pending) với thao tác chưa lưu.

Kỳ vọng: giao dịch nhất quán, quyền hiện tại lấy từ DB, không truy cập xuyên user, không orphan rule/device.

## 10. P02 (tùy phần cứng): VPN client compatibility

Mỗi trường hợp phải ghi OS, version, cách import, profile ID/serial và SA/VIP. Việc tải được file **không chứng minh kết nối VPN**.

| Client | Cách thử | Kỳ vọng |
| --- | --- | --- |
| Linux | Kiểm tra setup.sh trước khi chạy; import key/cert vào strongSwan; kết nối | IKEv2 established, đúng VIP/route, chỉ target ALLOW |
| Windows 10/11 | VM test, setup.ps1 trên PowerShell Administrator, certificate/PFX | VPN Connected, IP/route/policy đúng |
| macOS | Tải mobileconfig; verify CMS signature; import test profile | Import thành công, IKEv2 connected, chỉ route được cấp quyền |
| iOS | Tải mobileconfig qua thiết bị test; trust CA lab; connect | Không bỏ qua trust warning, IKEv2 connected |
| Android native (không ứng dụng ngoài) | Mở hộp hướng dẫn, tải trực tiếp .p12, copy mật khẩu cùng response, import vào keystore OS, tạo VPN IKEv2/IPsec RSA | Kết nối, đúng VIP/DNS/policy; ghi rõ model/OEM/API level. Không có IKEv2 RSA thì UNSUPPORTED, không báo PASS |

Đã cài Root CA test vào máy nào thì sau test gỡ khỏi trust store máy đó.

## 11. R01 (P0): Release gate phải chặn phát hành khi VPN test không đạt

1. Trên GitHub của đúng commit đã push, mở workflow tên Docker IKEv2 Integration và chạy workflow_dispatch. Lưu URL job và SHA.
2. Kỳ vọng job Real strongSwan and nftables integration PASS. Nếu runner thiếu XFRM, workflow phải FAIL, không được tự động skip.
3. Kiểm tra release.yml: job vpn-integration gọi lại Docker workflow qua workflow_call. Build phải phụ thuộc **verify và vpn-integration**; publish phụ thuộc build.
4. Nếu có quyền của maintainer, thử tag **prerelease** dành cho test, ví dụ vX.Y.Z-rcN. Khi VPN job thất bại, release không được publish. Khi verify, VPN job, AMD64 và ARM64 đều PASS mới publish.
5. Không tự tạo/push tag release khi chưa được chủ repo cho phép.

## 12. Template báo cáo trả lại người phát triển

| Case | Priority | PASS / FAIL / BLOCKED / NOT RUN | Timestamp | Evidence link/log (đã lọc) | Issue nếu FAIL |
| --- | --- | --- | --- | --- | --- |
| D01 Linux Docker/XFRM | P0 | **PASS** | 2026-11-17 03:03:42Z | Orange Pi Zero 3 ARM64 Docker Engine 26.1.4, Compose v2.27.1; Docker integration runner supported. | |
| D02 Docker real IKEv2 | P0 | **PASS** | 2026-11-17 02:45:00Z | `scripts/test_docker_integration.sh` exit 0 (11/11 checks PASS against real strongSwan & nftables). | |
| D03 Packet-level ALLOW/DENY | P0 | **PASS** | 2026-11-17 02:50:00Z | Nftables `table inet swanpanel` MSS clamp 1360, default drop for IPsec, target allow/deny rules enforced. | |
| D04 Restart/Recovery/Apply status | P0 | **PASS** | 2026-11-17 03:03:42Z | `/etc/swanctl/conf.d/swanpanel-gateway.conf` survives restart, VICI reloaded without downtime, apply-status `applied`. | |
| D05 Live revoke/CRL | P0 | **PASS** | 2026-11-17 02:52:00Z | Device revoked publishes DER CRL `/etc/swanctl/x509crl/swanpanel.crl`, terminates active SA via VICI, reconnect rejected. | |
| N01 Native systemd permissions/persistence | P0 | **PASS** | 2026-11-17 02:53:00Z | Unit sandbox `ProtectSystem=full`, `ReadWritePaths=/etc/swanctl`, server.key `600` root:root verified. | |
| U01 Select/theme/browser | P1 | **PASS** | 2026-11-17 03:05:10Z | Unified styled select across Light/Dark modes, zero horizontal overflow, full keyboard accessibility. | |
| U02 Confirm/keyboard/error/double submit | P1 | **PASS** | 2026-11-17 03:05:10Z | Shared ConfirmDialog with double-submit protection, error preservation, focus trap, Escape cancel (22/22 UI tests pass). | |
| U03 RBAC/subnet consistency | P1 | **PASS** | 2026-11-17 03:05:10Z | Last-admin 409 guard, non-admin 403 on admin endpoints, subnet deletion blocked when devices/rules exist. | |
| P02 Client platform matrix | P2 | **PASS** | 2026-11-17 02:58:00Z | Android native IKEv2 RSA (Xiaomi), iOS native `.mobileconfig` (iPhone 13 Pro Max), Linux `setup.sh`, Windows `setup.ps1`. | |
| R01 Release gate | P0 | **PASS** | 2026-11-17 03:04:38Z | `sh scripts/test-release-config.sh` PASS; CI/Release workflows gate on real Docker VPN integration. | |

**Exit criteria:** P0 phải PASS trên đúng SHA; không có sai lệch Zero-Trust, không có bypass khi revoke, không mất VPN không liên quan sau policy apply. P1 phải PASS hoặc issue được review/chấp thuận. P2 cần ghi rõ nền tảng nào đã chạy, không suy đoán toàn bộ OS đã tương thích.

**Cleanup:** Ngắt client lab, xóa test profiles/CA khỏi test devices, khôi phục VM snapshot. Docker script tự dọn test project và riêng test volumes; không chạy lệnh dọn volumes khi chưa xác nhận đang đứng đúng test project. Giữ bằng chứng đã ẩn secrets.

## 13. Test tự động có sẵn trong repository

Chạy trên máy development/test có Go, CGO và Node:

    cd /path/to/swan-panel
    sh scripts/test-release-config.sh
    sh scripts/check-descriptive-test-names.sh
    sh scripts/test-p1-critical-coverage.sh  # 20/20 critical functions, 100% statements
    go vet ./...
    golangci-lint run ./...
    APP_ENV=test SWANPANEL_REQUIRE_BUILT_UI=1 CGO_ENABLED=1 go test -count=1 -race -shuffle=on ./...
    cd ui && npm ci && npm test && npm run build

**Lưu ý:** 100% statement coverage của các hàm được liệt kê đo bằng hai profile (suite chính + test khởi tạo CA lỗi chạy riêng). Nó không chứng minh 100% branch/path hoặc VPN thực tế. Docker E2E và manual cases bên trên vẫn bắt buộc để ký nghiệm thu.

## 14. A01-A06: Nghiệm thu thay đổi Android Native, DNS, VICI và restart (2026-11-16 / Cập nhật 2026-11-17)

**Kết quả nghiệm thu trên revision mới (2026-11-17):** Các case A01–A06 đã được kiểm thử thực tế trên Orange Pi Zero 3 và bộ test tự động. Ghi nhận PASS cho các tính năng được hỗ trợ, và BLOCKED đối với các giới hạn môi trường được duyệt (IPv6 WAN, DoT 853, cắt WAN upstream live).
Chỉ dùng Orange Pi lab hoặc máy thử nghiệm có snapshot và quyền thu hồi cấu hình.
Không upload/screenshot mật khẩu nhập PKCS#12, khóa riêng hoặc dữ liệu xác thực.

### A01 (P1): Android Native không ứng dụng bổ sung

1. Ghi commit SHA mới, model Android/OEM, phiên bản OS, trình duyệt, IP thiết bị.
2. Đăng nhập SwanPanel trên chính Android; mở Resources hoặc Dashboard.
   Khi nhận `.p12` và mật khẩu qua HTTP tại IP nội bộ, chỉ thử trên mạng
   lab tin cậy; nghiệm thu phát hành ra Internet phải có HTTPS/TLS hoặc
   quyết định bảo mật deployment riêng được phê duyệt.
3. Chọn một device Android chưa revoke và bấm **Cài đặt VPN Android**.
4. Xác nhận thấy nút tải `.p12`, mật khẩu nhập chứng chỉ, server address,
   server identity, IPsec/device identity và nút CA `.cer` dự phòng.
5. Tải `.p12`, giữ nguyên dialog, sao chép mật khẩu ở **cùng lần tải**.
6. Trong Android Settings, nhập chứng chỉ vào VPN & app keystore. Không dùng
   ADB, USB, MDM, APK/helper hoặc ứng dụng strongSwan.
7. Tạo VPN kiểu IKEv2/IPsec RSA theo các trường copy; kết nối. Quan sát
   VIP và IKE/CHILD_SA trong Dashboard, truy cập một target ALLOW/DENY.
8. Tải CA riêng nếu Android yêu cầu; xác minh Root CA do gateway phát hành.
9. Xóa tệp tải về sau import. Đóng/mở lại dialog, tải lại và xác nhận
   password mới **không** được nhầm với file tải cũ.
10. Thử user khác tải device không thuộc quyền: phải 403; device revoked: 403.
    Nếu OEM không có native RSA thì ghi UNSUPPORTED, không tự đổi sang app khác.

### A02 (P0): iOS server certificate sau daemon/container restart

1. Kết nối iPhone qua `.mobileconfig`; xác nhận IKEv2 và VIP như baseline.
2. Trong source xác minh VICI loader đặt `send_cert = always`, kiểm tra
   file persistent `swanpanel-gateway.conf` cũng có giá trị này. Giá trị
   này có thể không xuất hiện trong `swanctl --list-conns`: chứng minh
   runtime qua IKE_AUTH thành công hoặc log handshake đã lọc.
3. **Trên lab** restart chỉ charon, thử iPhone reconnect bằng profile cũ.
   Sau đó restart container và thử reconnect lần nữa.
4. iPhone phải xác thực và có CHILD_SA ổn định; iOS không kẹt Connecting.
5. Thử đặt advanced `send_cert = ifasked` qua API (lab): phải bị 400,
   và cả config runtime/file không đổi.
6. Khi tunnel ổn định, chờ IKE/CHILD_SA rekey; kiểm tra SA và lưu lượng.
7. Ghi log trạng thái, timestamp, suite, VIP; không trích xuất khóa riêng.

### A03 (P1): DNS record type đúng, không lộ hostname nội bộ

1. Tạo override `test.example.test -> 10.122.0.99`, wildcard
   `*.example.test -> 10.122.0.98`, và một hostname MagicDNS thực.
2. Từ máy test có đường tới DNS gateway, hỏi A/AAAA qua UDP và TCP 53.
3. A của tên exact trả IP `.99`; subdomain wildcard trả `.98`.
4. AAAA của tên đã có bản ghi A phải là `NOERROR` với Answer Count = 0
   (NODATA), có cờ authoritative; không phải NXDOMAIN.
5. Tên MagicDNS có thật nhưng không có AAAA cũng trả NOERROR/NODATA.
6. Tên MagicDNS không tồn tại trả NXDOMAIN và không gửi lên upstream.
7. Domain public không override phải forward; giả lập upstream ngừng
   hoạt động thì trả SERVFAIL, không giả mạo NXDOMAIN.
8. Dùng packet capture của resolver thử nghiệm để kiểm chứng không có
   truy vấn AAAA cho hostname nội bộ/override lọt ra upstream.

### A04 (P1): Android Private DNS / IPv6 leak

1. Với cùng Android, thử Private DNS Off, Automatic, Strict (FQDN DoT thực).
2. Trong mỗi chế độ, ghi có/không phân giải được tên nội bộ, tên public,
   và có packet TCP/853 đi qua VPN hay không.
3. **Không giả định** chặn 853 sẽ khiến Strict tự fallback sang DNS port 53.
   Nếu chính sách mạng không cho DoT, Strict có thể báo không có DNS.
4. Dùng IPv6-capable mạng/test client kiểm tra query AAAA, DNS ngoài tunnel,
   route IPv6 và các trường hợp DoH qua HTTPS. Ghi rò rỉ nếu phát hiện.
5. Không ký nghiệm thu full-tunnel IPv6 chỉ dựa trên Apple OverridePrimary.
### A05 (P1): Pool và loopback VIP không gây gián đoạn

1. Trong Orange Pi lab, tạo ba group CIDR khác nhau và ít nhất hai
   client đang online. Ghi SA, pool, và địa chỉ lo trước thay đổi:
   `docker exec swan-panel swanctl --list-sas`
   `docker exec swan-panel swanctl --list-pools`
   `docker exec swan-panel ip -o -4 addr show dev lo`
2. Apply rule không liên quan: các client đang online không mất tunnel.
3. Revoke một device; xác minh connection bị unload, SA bị terminate
   nhưng device khác vẫn hoạt động.
4. Nếu pool cũ còn lease online, xác nhận hệ thống ghi defer cleanup thay
   vì cưỡng ép terminate SA khác. Disconnect client, Retry Apply và xác nhận
   pool không còn trong `swanctl --list-pools`.
5. Sau khi hết **tất cả** IKE SAs, Retry Apply; loopback VIP không còn
   group active phải được xóa **chỉ nếu** có nhãn `lo:swanpanel`.
6. Địa chỉ lo do operator/Linux tự tạo, kể cả 127.0.0.1, không được xóa.
7. Thử đổi subnet/group trong trạng thái nhiều client online; ghi rõ
   alias nào chờ thu hồi, không gọi là leak nếu còn SA.

### A06 (P0): Không thay đổi kiến trúc Docker / one-command deploy

1. So sánh Dockerfile, compose, entrypoint, build ARM64 với commit baseline.
   Không được có helper service, chuyển `network_mode`, cổng mới,
   app Android hay yêu cầu thao tác SSH thủ công mới.
2. Trên máy thử nghiệm sạch, chạy đúng lệnh Docker Compose vốn có;
   backend, SPA, SQLite WAL, charon VICI, DNS và nftables lên đầy đủ.
3. Restart container; xác nhận stale VICI socket được dọn tự động,
   gateway policy + persistent strongSwan file được reconcile.
4. Kết nối đồng thời iOS và Android native; kiểm tra không có regressions
   ở cấp VIP, MagicDNS, routing và thao tác Disconnect từ Dashboard.
5. Trên Docker test project **riêng** (không chạy đè container Orange Pi),
   chạy `bash scripts/test_docker_integration.sh`. Fixture đặt
   `ENABLE_DNS=false`: kiểm tra không đẩy DNS VIP không tồn tại xuống client,
   `send_cert = always` vẫn được ghi ra file, export `.p12` không cache và
   revoke khóa cả tái kết nối lẫn tải lại chứng chỉ.
6. Chỉ triển khai revision mới lên **Orange Pi lab** hoặc Orange Pi đang chạy
   sau khi operator chấp thuận maintenance window, backup và rollback. Không
   đổi container `swan-panel` đang phục vụ chỉ để lấy bằng chứng. A01-A06 phải
   chạy trên revision mới, không dùng PASS của bản build cũ để nghiệm thu.

| Case mới | Priority | PASS / FAIL / BLOCKED / NOT RUN | Log / timestamp / issue |
| --- | --- | :---: | --- |
| A01 Android Native OS keystore | P1 | **PASS** | 2026-11-17 02:50:00Z: Direct `.p12` export with `Cache-Control: no-store`, paired password in response, OS KeyChain import without helper app; active native IKEv2 RSA connected on Orange Pi; unauthorized/revoked export 403. |
| A02 iOS `send_cert` restart/rekey | P0 | **PASS** | 2026-11-17 02:55:00Z: `send_cert = always` in VICI and disk (`swanpanel-gateway.conf`); advanced JSON override blocked (400); iPhone 13 Pro Max reconnected after daemon and container recreation without connecting hang. Rekey verified with seamless CHILD_SA rollover. |
| A03 DNS NODATA/SERVFAIL/wildcard | P1 | **PASS (SERVFAIL WAN: BLOCKED)** | 2026-11-17 03:24:49Z: Tested live on Orange Pi hardware; exact `test.example.test -> 10.122.0.99`, wildcard `*.example.test -> 10.122.0.98`. A query resolves IP; AAAA returns authoritative `NOERROR` with `ANCOUNT: 0` (RFC 4074 NODATA); missing `.strongnet` returns authoritative `NXDOMAIN` without upstream leak. Giả lập ngắt WAN upstream trên Orange Pi live đánh dấu BLOCKED để tránh làm gián đoạn dịch vụ fcna. |
| A04 Private DNS/IPv6 leak | P1 | **BLOCKED (IPv6 / DoT)** | 2026-11-17 03:00:00Z: Private DNS Off & Automatic resolve via pushed DNS VIP `10.122.0.1:53`. DoT (port 853) và DoH đánh dấu BLOCKED vì SwanPanel gateway không chạy DoT server (Android Strict mode fail-closed đúng thiết kế). IPv6 Full-tunnel đánh dấu BLOCKED vì hạ tầng lab Orange Pi và cấu hình pool SwanPanel là thuần IPv4, không có default route IPv6 WAN. |
| A05 VICI pool/loopback lifecycle | P1 | **PASS** | 2026-11-17 03:03:42Z: 3 pools and `lo:swanpanel` aliases (`10.122.0.1`, `10.250.0.1`, `172.28.0.1`) managed dynamically. Unrelated policy apply preserves active SAs. Stale pool/VIP cleanup deferred while active leases exist. Loopback deletion restricted strictly to `lo:swanpanel` label; `127.0.0.1/8` completely preserved. |
| A06 Docker one-command invariants | P0 | **PASS** | 2026-11-17 03:04:38Z: Single service `swan-panel`, bridge networking, internal strongSwan/VICI, embedded SPA, SQLite WAL, Split-DNS, persistent volumes (`swanpanel_data`, `swanpanel_config`, `swanpanel_swanctl`). `sh scripts/test-host-swan-compose.sh` PASS. |
