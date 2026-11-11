// SwanPanel Documentation & Marketing i18n Engine
// High conversion, punchy copy in both English & Vietnamese

const translations = {
  en: {
    meta: {
      title_landing: "SwanPanel - Modern Zero-Trust Control Plane for strongSwan VPN",
      desc_landing: "Enterprise-grade, zero-trust web control plane for strongSwan IKEv2/IPsec VPNs with 100% native client provisioning, automated PKI, Split-DNS, and zero-downtime RAM reloads."
    },
    nav: {
      why: "Why SwanPanel",
      features: "Capabilities",
      comparison: "Comparison",
      docs: "Docs & Guide",
      star: "Star on GitHub",
      get_started: "Get Started",
      toggle_theme: "Toggle theme",
      switch_lang: "Tiếng Việt",
      switch_lang_title: "Chuyển sang Tiếng Việt",
      menu: "Menu"
    },
    hero: {
      badge: "Open Source & Non-Profit • Free under AGPL-3.0",
      title_start: "Enterprise Zero-Trust Remote Access.",
      title_accent: "100% Native Clients. Zero 3rd-Party Apps.",
      desc: "Stop forcing employees to install bulky, battery-draining VPN apps. SwanPanel brings zero-trust control, automated PKI, Split-DNS, and dynamic RAM reloads to strongSwan IPsec in a single self-contained binary.",
      install_title: "1-Line Production Quickstart (Ubuntu, Debian, RHEL, Rocky, Fedora)",
      install_hint_1: "Auto-detects OS & tunes kernel forwarding",
      install_hint_2: "Zero config required • Ready in under 60s",
      install_hint_3: "Single pure Go binary + embedded React SPA",
      copied: "Copied!",
      copy: "Copy",
      btn_get_started: "Get Started & Docs",
      btn_star: "Star on GitHub"
    },
    pains: {
      tag: "Why SwanPanel",
      title: "Built to Fix Everything Broken About Traditional VPNs",
      subtitle: "Say goodbye to arcane config files, battery-draining client apps, and expensive SaaS subscriptions.",
      p1_title: "The Old Way: Heavy Client Apps",
      p1_pain: "Employees hate installing clunky 3rd-party apps (OpenVPN, WireGuard client, Cisco AnyConnect) that drain laptop batteries, consume memory, and conflict with OS network settings.",
      p1_sol_badge: "SwanPanel Solution",
      p1_sol: "100% Native Connectivity: Connect directly from macOS, iOS, Windows, Linux, and Android system settings with native hardware-accelerated IPsec encryption. Zero apps to install or maintain.",
      p2_title: "The Old Way: Fragile SSH Config Files",
      p2_pain: "Editing raw swanctl.conf or ipsec.conf over SSH is dangerous. A single typo crashes the daemon, and reloading static files drops active tunnels for all connected remote engineers.",
      p2_sol_badge: "SwanPanel Solution",
      p2_sol: "Zero-SSH Dynamic VICI Control: Reload connections dynamically in RAM via UNIX socket /var/run/charon.vici without dropping live tunnels. Terminate unauthorized sessions immediately.",
      p3_title: "The Old Way: Expensive SaaS Lock-In",
      p3_pain: "Commercial Zero-Trust platforms (Tailscale, Cloudflare Zero Trust) charge $5-$20/user/month, lock your infrastructure into proprietary closed clouds, and route sensitive network metadata outside.",
      p3_sol_badge: "SwanPanel Solution",
      p3_sol: "100% Free & Self-Hosted: Deploy on your own servers under GNU AGPL-3.0. Your keys, traffic, and access policies stay entirely under your sovereignty with zero recurring subscription fees."
    },
    features: {
      tag: "Capabilities",
      title: "Enterprise Zero-Trust Capabilities Built-In",
      subtitle: "Everything you need for enterprise remote access without bloated clusters or external dependencies.",
      f1_title: "1-Click Native Client Provisioning",
      f1_desc: "Auto-generate signed Apple .mobileconfig, Windows PowerShell Add-VpnConnection scripts, Linux NetworkManager profiles, and Android IKEv2 cert bundles.",
      f2_title: "Autonomous Internal PKI (X.509)",
      f2_desc: "Built-in Certificate Authority automatically provisions 4096-bit RSA Root CAs, signs client certs, and packages encrypted .p12 bundles without manual OpenSSL scripts.",
      f3_title: "Split-DNS & MagicDNS Engine",
      f3_desc: "Embedded high-performance DNS server (miekg/dns) auto-resolves internal devices (.strongnet) and custom overrides while keeping personal web traffic private.",
      f4_title: "Micro-Segmented Access Policies",
      f4_desc: "Granular zero-trust access control mapping User Groups to Resource Groups, compiled dynamically into kernel traffic selectors and DROP filters.",
      f5_title: "Dynamic RAM Control via VICI",
      f5_desc: "Direct strongSwan charon daemon management over UNIX socket /var/run/charon.vici. Dynamic RAM reloads without dropping live engineer tunnels.",
      f6_title: "Single Self-Contained Binary",
      f6_desc: "Go backend, SQLite database with auto-migration, embedded React 19 SPA, and DNS engine compile into one pure Go executable with zero CGO dependencies."
    },
    comparison: {
      tag: "Comparison Matrix",
      title: "How SwanPanel Compares",
      subtitle: "Native OS enterprise connectivity versus traditional alternatives.",
      col_feature: "Feature",
      col_swan: "SwanPanel + strongSwan",
      col_raw: "Raw strongSwan CLI",
      col_wire: "WireGuard / OpenVPN",
      col_saas: "Commercial SaaS (Tailscale)",
      r1_title: "3rd-Party Client App Required",
      r1_swan: "No (100% Native OS)",
      r1_raw: "No (Native OS)",
      r1_wire: "Yes (Always Required)",
      r1_saas: "Yes (Proprietary Agent)",
      r2_title: "Web Management Interface",
      r2_swan: "Yes (Single Binary)",
      r2_raw: "No (CLI / SSH only)",
      r2_wire: "Requires 3rd party tool",
      r2_saas: "Yes (Closed cloud)",
      r3_title: "Dynamic RAM Reload (Zero Downtime)",
      r3_swan: "Yes (VICI IPC)",
      r3_raw: "Manual swanctl commands",
      r3_wire: "Varies by daemon",
      r3_saas: "Proprietary protocol",
      r4_title: "Automated PKI & .mobileconfig",
      r4_swan: "Yes (Built-in)",
      r4_raw: "Manual OpenSSL / CFSSL",
      r4_wire: "Manual key generation",
      r4_saas: "Managed in vendor cloud",
      r5_title: "Split-DNS & MagicDNS Server",
      r5_swan: "Yes (Embedded miekg/dns)",
      r5_raw: "External BIND/dnsmasq",
      r5_wire: "Requires external setup",
      r5_saas: "Yes (Vendor hosted)",
      r6_title: "Cost / Seat",
      r6_swan: "100% Free (AGPL-3.0)",
      r6_raw: "100% Free",
      r6_wire: "Free / Community",
      r6_saas: "$5 - $20 / user / month"
    },
    deploy: {
      tag: "Get Started Today",
      title: "Ready to Modernize Your Remote Access?",
      desc: "Deploy SwanPanel in under 60 seconds with our automated production script, or read our comprehensive guide with visual walkthroughs for administrators and end-users.",
      tab_script: "1-Line Production Script",
      tab_docker: "Docker Compose",
      btn_guide: "Explore Documentation & Visual Setup Guides"
    },
    user_guide: {
      adv1_title: "Hardware-Accelerated IPsec",
      adv1_desc: "Native AES-256-GCM encryption runs directly in the OS kernel with zero CPU virtualization overhead.",
      adv2_title: "Zero Battery & RAM Bloat",
      adv2_desc: "No background tray agent or daemon draining laptop battery. Seamless sleep, wake, and standby.",
      adv3_title: "MOBIKE Seamless Roaming",
      adv3_desc: "Roam seamlessly between Wi-Fi and 4G/5G cellular connections without dropping active sessions (RFC 4555).",
      adv4_title: "Automatic Split-DNS",
      adv4_desc: "Only internal corporate domains (.strongnet) route through VPN DNS, keeping personal browsing completely untouched."
    },
    guide_sidebar: {
      group_install: "1. Installation & Setup",
      prereq: "Prerequisites & Ports",
      install_quick: "1-Line Production Installer",
      install_docker: "Docker Compose Setup",
      install_config: "Configuration & Env",
      install_proxy: "Reverse Proxy & TLS",

      group_admin: "2. Administrator Guide",
      admin_login: "Sign In & Password",
      admin_dashboard: "Dashboard Overview",
      admin_subnets: "Subnets & IP Pools",
      admin_devices: "Devices & Profiles",
      admin_users: "Users & User Groups",
      admin_rules: "Zero-Trust Access Rules",
      admin_dns: "Split-DNS & MagicDNS",
      admin_oidc: "OIDC SSO Providers",
      admin_settings: "Server Settings & VICI",
      admin_logs: "System Audit Logs",

      group_user: "3. End-User VPN Guide",
      user_overview: "Why Native Clients?",
      user_apple: "Apple macOS & iOS",
      user_windows: "Microsoft Windows",
      user_linux: "Linux NetworkManager",
      user_android: "Android IKEv2 RSA",
      user_samples: "Sample Config Files"
    },
    guide: {
      prereq_title: "System Requirements & Port Matrix",
      prereq_desc: "SwanPanel is a lightweight, single-binary control plane written in pure Go. It coordinates strongSwan's high-performance IPsec kernel module without requiring Java, Node.js runtimes, or external database servers.",
      ports_title: "Network Firewall & Port Mapping",
      ports_desc: "Open the following inbound ports on your cloud firewall (AWS Security Group, GCP Firewall, Cloudflare, or local iptables):",

      quick_title: "1-Line Automated Production Installer",
      quick_desc: "The recommended deployment method for Ubuntu, Debian, RHEL, Rocky Linux, AlmaLinux, and Fedora. The installer automatically detects your distribution, installs strongSwan with the VICI plugin, tunes Linux kernel packet forwarding, generates an initial PKI CA, and registers a systemd service:",
      quick_post: "Once complete, access your control plane at http://<SERVER_IP>:8080.",

      docker_title: "Production Docker Compose Deployment",
      docker_desc: "If you prefer containerized workloads, run SwanPanel via Docker Compose. Ensure the host has the strongSwan VICI UNIX socket mapped at /var/run/charon.vici:",

      config_title: "Configuration & Environment Variables",
      config_desc: "Configure SwanPanel via environment variables or /etc/swan-panel/config.env:",

      proxy_title: "TLS Termination & Reverse Proxy",
      proxy_desc: "For production security, terminate HTTPS via Caddy or Nginx:",

      admin_login_title: "Initial Administrator Sign In",
      admin_login_desc: "Navigate to http://<SERVER_IP>:8080. Log in using the default administrative credentials:",
      admin_login_warn: "You will be prompted to replace this default secret immediately upon your first sign in to ensure zero-trust compliance.",
      caption_login: "Figure 2.1: SwanPanel administrative authentication interface with brute-force rate-limiting.",

      admin_dash_title: "Dashboard & Gateway Live Status",
      admin_dash_desc: "The Dashboard provides instantaneous visibility into your strongSwan daemon: VICI UNIX socket connection health, active Security Associations (IKE_SA / CHILD_SA), connected client devices, and realtime RX/TX throughput metrics.",
      caption_dash: "Figure 2.2: Live Gateway Dashboard showing connected native clients and VICI memory socket status.",

      admin_subnets_title: "Subnets & IP Pool Allocation",
      admin_subnets_desc: "In Resources -> Subnets, configure virtual VPN client address pools (e.g. 10.50.0.0/16) and register protected private subnets (e.g. 10.60.10.0/24 for Production, 10.60.20.0/24 for Staging).",
      caption_subnets: "Figure 2.3: Subnet allocation table managing CIDR blocks and IP utilization.",
      caption_add_subnet: "Figure 2.4: Modal dialog for allocating dedicated subnets and assigning resource groups.",

      admin_devices_title: "Device Inventory & Client Profile Provisioning",
      admin_devices_desc: "Every endpoint connecting to SwanPanel is tracked under Resources -> Devices. When you register a new device, SwanPanel's internal PKI automatically generates an X.509 client certificate and prepares platform-specific configuration profiles (.mobileconfig for Apple, PowerShell for Windows, NetworkManager for Linux).",
      caption_devices: "Figure 2.5: Device inventory listing hostnames, assigned virtual IPs, and 1-click download actions.",
      caption_add_device: "Figure 2.6: Device provisioning modal supporting Apple, Windows, Linux, and Android presets.",

      admin_users_title: "User Directory & Role-Based User Groups",
      admin_users_desc: "Manage local users and organize them into functional groups (e.g. Engineering, DevOps, Contractors, Sales). Access rules are enforced at the group level to maintain zero-trust segmentation.",
      caption_users: "Figure 2.7: User directory tracking assigned devices, authentication roles, and group memberships.",
      caption_groups: "Figure 2.8: User Group management for policy mapping and access scoping.",

      admin_rules_title: "Zero-Trust Access Rules Engine",
      admin_rules_desc: "Under Access Rules, define explicit ALLOW and DENY policies mapping User Groups to Resource Groups. When you save a rule, SwanPanel dynamically recompiles Linux kernel traffic selectors and pushes the changes into RAM via the VICI socket without dropping any live tunnels.",
      caption_rules: "Figure 2.9: Micro-segmentation matrix mapping User Groups to Resource Subnets with instant RAM reload.",
      caption_add_rule: "Figure 2.10: Rule modal dialog for selecting source user group, destination target, and action.",

      admin_dns_title: "Split-DNS Engine & Hostname Overrides",
      admin_dns_desc: "SwanPanel includes an embedded DNS server powered by miekg/dns. It automatically registers connected devices under <hostname>.strongnet and allows administrators to define custom A and CNAME record overrides for internal services (e.g. k8s.internal -> 10.60.10.100).",
      caption_dns: "Figure 2.11: Split-DNS engine interface managing internal zone records and recursive upstreams.",
      caption_add_dns: "Figure 2.12: Modal for configuring custom split-tunneling DNS records with instant live caching.",

      admin_oidc_title: "Enterprise Single Sign-On (OIDC SSO)",
      admin_oidc_desc: "Connect your organization's identity provider (Google Workspace, Microsoft Entra ID, Okta, Keycloak, or Auth0) using standard OpenID Connect (OIDC).",
      caption_oidc: "Figure 2.13: OIDC configuration portal managing corporate Single Sign-On integrations.",
      caption_add_oidc: "Figure 2.14: Connecting an enterprise identity provider via Issuer URL, Client ID, and Secret.",

      admin_settings_title: "Server Settings & VICI Socket Tuning",
      admin_settings_desc: "Under Server Settings, set your gateway's public FQDN (e.g. vpn.company.com), configure default DNS search domains, and verify UNIX socket connectivity to /var/run/charon.vici.",
      caption_settings: "Figure 2.15: Server settings managing gateway public address, PKI certificate regeneration, and socket health.",

      admin_logs_title: "Real-Time System Audit & Kernel Logs",
      admin_logs_desc: "Stream structured system and charon daemon logs directly in the browser over Server-Sent Events (SSE). Filter by severity level (DEBUG, INFO, WARN, ERROR) or search by client IP address.",
      caption_logs: "Figure 2.16: Realtime streaming audit logs with scope filtering, search, and live pause controls.",

      user_why_title: "Why SwanPanel Uses 100% Native OS Clients",
      user_why_desc: "Unlike legacy VPN solutions that force employees to install bulky, battery-draining client applications, SwanPanel leverages the native IKEv2/IPsec engine built directly into every modern operating system.",

      user_apple_title: "Apple Ecosystem: macOS & iOS (.mobileconfig)",
      user_apple_desc: "SwanPanel packages your client certificate, private key, and Root CA trust chain into a single signed Apple configuration profile (.mobileconfig).",

      user_win_title: "Microsoft Windows 10 & 11 (PowerShell Script)",
      user_win_desc: "SwanPanel generates an automated, idempotent PowerShell script that imports the Root CA, installs your machine certificate, and creates the native Windows VPN connection:",

      user_linux_title: "Linux Desktop (NetworkManager & nmcli)",
      user_linux_desc: "On Ubuntu, Fedora, Debian, and Arch desktops, SwanPanel integrates with native NetworkManager:",

      user_android_title: "Google Android (Native IKEv2 RSA)",
      user_android_desc: "Android connects via native IKEv2/IPSec RSA using standard system settings:",

      user_samples_title: "Reference Configuration Profiles",
      user_samples_desc: "Explore reference configuration samples bundled with SwanPanel:"
    },
    footer: {
      github: "GitHub Repository",
      wiki: "Wiki",
      releases: "Release v0.0.2",
      guide: "Docs & Guide",
      home: "Home",
      admin_guide: "Admin Guide",
      enduser_guide: "End-User Guide",
      license: "AGPL-3.0 License",
      credit: "SwanPanel is an open-source project created by Thái Thiên Vũ. Licensed under GNU AGPL-3.0."
    }
  },

  vi: {
    meta: {
      title_landing: "SwanPanel - Bảng Điều Khiển Zero-Trust Hiện Đại cho strongSwan VPN",
      desc_landing: "Hệ thống Web Control Plane Zero-Trust chuẩn doanh nghiệp cho strongSwan IKEv2/IPsec VPN. Cấu hình 100% native client, tích hợp PKI, Split-DNS và nạp cấu hình động qua RAM không downtime."
    },
    nav: {
      why: "Vì sao chọn SwanPanel",
      features: "Tính năng",
      comparison: "So sánh",
      docs: "Tài liệu & Hướng dẫn",
      star: "Tặng 1 sao GitHub",
      get_started: "Bắt đầu ngay",
      toggle_theme: "Chuyển giao diện",
      switch_lang: "English",
      switch_lang_title: "Switch to English",
      menu: "Menu"
    },
    hero: {
      badge: "Mã Nguồn Mở & Phi Lợi Nhuận • Giấy Phép AGPL-3.0",
      title_start: "VPN Doanh Nghiệp Zero-Trust.",
      title_accent: "100% Native OS. Không Cài Thêm App.",
      desc: "Chấm dứt việc ép nhân viên phải cài đặt các app VPN cồng kềnh, tốn pin. SwanPanel mang đến nền tảng Zero-Trust hiện đại, tự động hóa PKI, tích hợp Split-DNS và nạp cấu hình động qua RAM cho strongSwan IPsec trong một file nhị phân duy nhất.",
      install_title: "Lệnh Cài Đặt Nhanh 1 Dòng (Ubuntu, Debian, RHEL, Rocky, Fedora)",
      install_hint_1: "Tự nhận diện OS & cấu hình chuyển tiếp kernel",
      install_hint_2: "Không cần cấu hình phức tạp • Sẵn sàng dưới 60s",
      install_hint_3: "Một file nhị phân Go duy nhất nhúng sẵn React SPA",
      copied: "Đã chép!",
      copy: "Sao chép",
      btn_get_started: "Bắt Đầu & Xem Tài Liệu",
      btn_star: "Tặng 1 sao trên GitHub"
    },
    pains: {
      tag: "Vì sao chọn SwanPanel",
      title: "Giải Quyết Triệt Để Mọi Bất Tiện Của VPN Truyền Thống",
      subtitle: "Nói lời tạm biệt với file cấu hình phức tạp, app VPN tốn pin và chi phí thuê bao SaaS đắt đỏ.",
      p1_title: "Cách Cũ: Cài Đặt App Nặng Nề",
      p1_pain: "Nhân viên rất ngại cài đặt các ứng dụng VPN của bên thứ ba (OpenVPN, WireGuard client, Cisco AnyConnect). Những ứng dụng này ngốn RAM, làm nóng máy, hao pin và dễ xung đột mạng.",
      p1_sol_badge: "Giải pháp SwanPanel",
      p1_sol: "Kết nối 100% Native: Kết nối thẳng từ Cài đặt hệ thống trên macOS, iOS, Windows, Linux và Android với tốc độ tăng tốc phần cứng của IPsec. Hoàn toàn không cần cài app phụ.",
      p2_title: "Cách Cũ: Sửa File Qua SSH Dễ Sập Hệ Thống",
      p2_pain: "Sửa file swanctl.conf hay ipsec.conf qua SSH rất dễ lỗi cú pháp. Mỗi lần khởi động lại daemon, toàn bộ kết nối VPN của các kỹ sư trong công ty đều bị ngắt quãng đột ngột.",
      p2_sol_badge: "Giải pháp SwanPanel",
      p2_sol: "Điều khiển VICI Zero-SSH: Nạp cấu hình động trực tiếp vào RAM qua UNIX socket /var/run/charon.vici mà không làm gián đoạn đường hầm đang chạy. Ngắt phiên kết nối ngay lập tức.",
      p3_title: "Cách Cũ: Khóa Chặt Vào SaaS Đắt Đỏ",
      p3_pain: "Các nền tảng Zero-Trust thương mại (Tailscale, Cloudflare Zero Trust) thu 5$ - 20$/user/tháng, giam lỏng hạ tầng của bạn vào hệ thống đóng và lưu trữ metadata trên cloud ngoài.",
      p3_sol_badge: "Giải pháp SwanPanel",
      p3_sol: "Tự Chủ & Miễn Phí 100%: Triển khai trên máy chủ riêng của bạn theo giấy phép GNU AGPL-3.0. Khóa bí mật, lưu lượng và luật phân quyền thuộc toàn quyền quản lý của bạn, không phí định kỳ."
    },
    features: {
      tag: "Tính Năng Cốt Lõi",
      title: "Mọi Thứ Bạn Cần Cho Hệ Thống Remote Access Doanh Nghiệp",
      subtitle: "Hiệu năng mạnh mẽ chuẩn doanh nghiệp, không cồng kềnh, không chi phí ẩn, không phụ thuộc bên ngoài.",
      f1_title: "Cấp Phát Cấu Hình Native 1 Click",
      f1_desc: "Tự động tạo profile Apple .mobileconfig, script PowerShell cho Windows, Linux NetworkManager và Android IKEv2 kèm chứng chỉ PKCS#12 nhúng sẵn.",
      f2_title: "Tự Động Hóa PKI Nội Bộ (X.509)",
      f2_desc: "Hệ thống Certificate Authority tích hợp tự động cấp phát Root CA 4096-bit RSA, ký chứng chỉ client và đóng gói file .p12 không cần lệnh OpenSSL.",
      f3_title: "Máy Chủ Split-DNS & MagicDNS",
      f3_desc: "DNS engine nhúng hiệu năng cao (miekg/dns) tự phân giải tên miền nội bộ (.strongnet) và ghi đè tùy chỉnh mà không ảnh hưởng duyệt web cá nhân.",
      f4_title: "Chính Sách Phân Quyền Micro-Segmented",
      f4_desc: "Phân quyền Zero-Trust chi tiết giữa Nhóm người dùng và Nhóm tài nguyên, tự động biên dịch thành bộ lọc DROP/ACCEPT ở tầng kernel Linux.",
      f5_title: "Điều Khiển Động Qua Socket VICI",
      f5_desc: "Quản trị trực tiếp daemon strongSwan qua UNIX socket /var/run/charon.vici. Nạp cấu hình tức thì vào RAM không làm gián đoạn đường hầm đang chạy.",
      f6_title: "Một File Nhị Phân Go Tự Chứa Duy Nhất",
      f6_desc: "Backend Go, cơ sở dữ liệu SQLite tự động migrate, giao diện React 19 SPA nhúng sẵn và DNS engine đóng gói thành 1 file duy nhất không phụ thuộc CGO."
    },
    comparison: {
      tag: "Bảng So Sánh",
      title: "Ưu Thế Vượt Trội Của SwanPanel",
      subtitle: "So sánh khả năng kết nối native doanh nghiệp của SwanPanel với các phương án truyền thống.",
      col_feature: "Tiêu chí",
      col_swan: "SwanPanel + strongSwan",
      col_raw: "strongSwan CLI Thô",
      col_wire: "WireGuard / OpenVPN",
      col_saas: "Commercial SaaS (Tailscale)",
      r1_title: "Yêu Cầu Cài App Bên Thứ Ba",
      r1_swan: "Không (100% Native OS)",
      r1_raw: "Không (Native OS)",
      r1_wire: "Có (Bắt buộc phải cài)",
      r1_saas: "Có (Cài Agent độc quyền)",
      r2_title: "Giao Diện Web Quản Trị",
      r2_swan: "Có (Tích hợp Single Binary)",
      r2_raw: "Không (Chỉ có CLI / SSH)",
      r2_wire: "Cần cài thêm công cụ ngoài",
      r2_saas: "Có (Cloud đóng của hãng)",
      r3_title: "Nạp Động Qua RAM (Không Rớt Mạng)",
      r3_swan: "Có (VICI IPC)",
      r3_raw: "Lệnh swanctl thủ công",
      r3_wire: "Tùy thuộc daemon",
      r3_saas: "Giao thức riêng của hãng",
      r4_title: "Tự Động PKI & Xuất .mobileconfig",
      r4_swan: "Có (Tích hợp sẵn)",
      r4_raw: "Tự tạo qua OpenSSL / CFSSL",
      r4_wire: "Tự sinh cặp khóa thủ công",
      r4_saas: "Quản lý trên cloud nhà cung cấp",
      r5_title: "Máy Chủ Split-DNS & MagicDNS",
      r5_swan: "Có (Nhúng sẵn miekg/dns)",
      r5_raw: "Cần dựng BIND/dnsmasq ngoài",
      r5_wire: "Cần cài đặt bên ngoài",
      r5_saas: "Có (Lưu trên cloud của hãng)",
      r6_title: "Chi Phí Bản Quyền / Tài Khoản",
      r6_swan: "100% Miễn phí (AGPL-3.0)",
      r6_raw: "100% Miễn phí",
      r6_wire: "Miễn phí / Cộng đồng",
      r6_saas: "5$ - 20$ / người dùng / tháng"
    },
    deploy: {
      tag: "Bắt Đầu Ngay",
      title: "Sẵn Sàng Hiện Đại Hóa Hệ Thống VPN?",
      desc: "Triển khai SwanPanel trong chưa đầy 60 giây bằng lệnh tự động, hoặc khám phá tài liệu hướng dẫn trực quan đầy đủ cho quản trị viên và người dùng.",
      tab_script: "Lệnh Cài Đặt Tự Động 1 Dòng",
      tab_docker: "Cấu Hình Docker Compose",
      btn_guide: "Khám Phá Tài Liệu & Hướng Dẫn Trực Quan"
    },
    user_guide: {
      adv1_title: "Tăng Tốc Phần Cứng IPsec",
      adv1_desc: "Mã hóa AES-256-GCM chạy trực tiếp trong kernel hệ điều hành với chi phí CPU gần như bằng 0.",
      adv2_title: "Không Tốn Pin & Bộ Nhớ RAM",
      adv2_desc: "Không chạy ngầm agent tiêu hao pin. Chế độ sleep, wake và chờ hoạt động mượt mà không độ trễ kết nối lại.",
      adv3_title: "Chuyển Mạng Không Ngắt (MOBIKE)",
      adv3_desc: "Chuyển đổi liền mạch giữa Wi-Fi và mạng di động 4G/5G mà không làm rớt phiên SSH hoặc VPN (RFC 4555).",
      adv4_title: "Tự Động Split-DNS",
      adv4_desc: "Chỉ các tên miền nội bộ công ty (.strongnet) được định tuyến qua VPN DNS, truy cập internet cá nhân được giữ nguyên."
    },
    guide_sidebar: {
      group_install: "1. Cài đặt & Khởi tạo",
      prereq: "Yêu cầu hệ thống & Mở port",
      install_quick: "Cài đặt tự động 1 dòng",
      install_docker: "Triển khai Docker Compose",
      install_config: "Cấu hình biến môi trường",
      install_proxy: "Reverse Proxy & TLS",

      group_admin: "2. Hướng dẫn Quản trị viên",
      admin_login: "Đăng nhập & Mật khẩu",
      admin_dashboard: "Tổng quan Dashboard",
      admin_subnets: "Dải mạng & Cấp phát IP",
      admin_devices: "Thiết bị & Tải Profile",
      admin_users: "Người dùng & Nhóm",
      admin_rules: "Chính sách Zero-Trust",
      admin_dns: "Split-DNS & MagicDNS",
      admin_oidc: "Tích hợp SSO OIDC",
      admin_settings: "Cài đặt máy chủ & VICI",
      admin_logs: "Nhật ký hệ thống",

      group_user: "3. Hướng dẫn Người dùng",
      user_overview: "Vì sao dùng Native VPN?",
      user_apple: "Apple macOS & iOS",
      user_windows: "Microsoft Windows",
      user_linux: "Linux NetworkManager",
      user_android: "Google Android RSA",
      user_samples: "File cấu hình mẫu"
    },
    guide: {
      prereq_title: "Yêu Cầu Hệ Thống & Ma Trận Cổng Mạng",
      prereq_desc: "SwanPanel là giải pháp điều khiển gọn nhẹ, đóng gói thành 1 file nhị phân tĩnh thuần Go. Hệ thống phối hợp module kernel IPsec hiệu năng cao của strongSwan mà không cần Java, Node.js hay cơ sở dữ liệu bên ngoài.",
      ports_title: "Mở Cổng Tường Lửa Mạng (Port Mapping)",
      ports_desc: "Mở các cổng mạng sau trên tường lửa đám mây của bạn (AWS Security Group, GCP Firewall, Cloudflare hoặc iptables):",

      quick_title: "Cài Đặt Tự Động 1 Dòng Lệnh",
      quick_desc: "Phương pháp triển khai khuyến nghị cho Ubuntu, Debian, RHEL, Rocky Linux, AlmaLinux và Fedora. Script tự động cài strongSwan kèm plugin VICI, tối ưu kernel forwarding, khởi tạo Root CA và đăng ký service systemd:",
      quick_post: "Sau khi hoàn tất, mở trình duyệt truy cập http://<SERVER_IP>:8080.",

      docker_title: "Triển Khai Bằng Docker Compose",
      docker_desc: "Nếu bạn ưu tiên chạy container, triển khai SwanPanel qua Docker Compose. Đảm bảo ánh xạ UNIX socket /var/run/charon.vici từ máy chủ:",

      config_title: "Cấu Hình & Biến Môi Trường",
      config_desc: "Quản lý tham số cấu hình SwanPanel qua file /etc/swan-panel/config.env hoặc biến môi trường:",

      proxy_title: "Cấu Hình Reverse Proxy & Chứng Chỉ HTTPS",
      proxy_desc: "Để đảm bảo an toàn môi trường production, đặt SwanPanel phía sau reverse proxy hỗ trợ TLS như Caddy hoặc Nginx:",

      admin_login_title: "Đăng Nhập Quản Trị Viên Ban Đầu",
      admin_login_desc: "Mở http://<SERVER_IP>:8080. Đăng nhập bằng tài khoản quản trị mặc định ban đầu:",
      admin_login_warn: "Hệ thống sẽ nhắc bạn đổi mật khẩu này ngay trong lần đầu đăng nhập để tuân thủ tiêu chuẩn Zero-Trust.",
      caption_login: "Hình 2.1: Giao diện xác thực quản trị viên SwanPanel với cơ chế Rate-limit chống Brute-force.",

      admin_dash_title: "Bảng Điều Khiển & Trạng Thái Gateway",
      admin_dash_desc: "Dashboard cung cấp cái nhìn toàn diện tức thì: Trạng thái socket UNIX VICI, các phiên kết nối IKE_SA đang hoạt động, danh sách thiết bị online và băng thông RX/TX thời gian thực.",
      caption_dash: "Hình 2.2: Dashboard trực tiếp hiển thị danh sách thiết bị native đang kết nối và socket RAM VICI.",

      admin_subnets_title: "Quản Lý Dải Mạng & Cấp Phát IP",
      admin_subnets_desc: "Tại Tài nguyên -> Subnets, cấu hình dải IP ảo cấp cho client VPN (ví dụ 10.50.0.0/16) và khai báo các dải mạng nội bộ cần bảo vệ (10.60.10.0/24 Production, 10.60.20.0/24 Staging).",
      caption_subnets: "Hình 2.3: Bảng phân bổ subnet quản lý các dải CIDR và tỷ lệ sử dụng IP.",
      caption_add_subnet: "Hình 2.4: Hộp thoại cấp phát dải mạng mới và gắn vào nhóm tài nguyên.",

      admin_devices_title: "Quản Lý Thiết Bị & Cấp Phát Profile",
      admin_devices_desc: "Mỗi thiết bị kết nối được định danh tại Tài nguyên -> Thiết bị. Khi tạo mới, PKI nội bộ tự động ký chứng chỉ X.509 và tạo profile tương ứng (.mobileconfig cho Apple, PowerShell cho Windows, NetworkManager cho Linux).",
      caption_devices: "Hình 2.5: Danh bạ thiết bị hiển thị tên máy, IP ảo được cấp và nút tải cấu hình 1 click.",
      caption_add_device: "Hình 2.6: Hộp thoại tạo thiết bị hỗ trợ cấu hình nhanh cho Apple, Windows, Linux và Android.",

      admin_users_title: "Danh Bạ Người Dùng & Nhóm Phân Quyền",
      admin_users_desc: "Quản lý tài khoản người dùng và phân loại thành các nhóm chức năng (Engineering, DevOps, Contractors). Các chính sách Zero-Trust được gán ở cấp độ nhóm để quản lý dễ dàng.",
      caption_users: "Hình 2.7: Danh bạ người dùng theo dõi thiết bị được cấp, quyền hạn và thành viên nhóm.",
      caption_groups: "Hình 2.8: Quản lý nhóm người dùng phục vụ cho việc gán chính sách truy cập.",

      admin_rules_title: "Hệ Thống Luật Truy Cập Zero-Trust",
      admin_rules_desc: "Tại Quy tắc truy cập, thiết lập các luật ALLOW và DENY giữa Nhóm người dùng và Nhóm tài nguyên. SwanPanel tự động biên dịch thành luật kernel và nạp thẳng vào RAM qua VICI không làm rớt kết nối.",
      caption_rules: "Hình 2.9: Ma trận phân quyền Zero-Trust vi phân đoạn giữa Nhóm người dùng và Subnet tài nguyên.",
      caption_add_rule: "Hình 2.10: Hộp thoại tạo luật mới chọn nhóm nguồn, tài nguyên đích và hành động.",

      admin_dns_title: "Máy Chủ Split-DNS & Ghi Đè Tên Miền",
      admin_dns_desc: "SwanPanel tích hợp máy chủ DNS miekg/dns, tự động gán tên miền <hostname>.strongnet cho mỗi thiết bị và cho phép quản trị viên tạo bản ghi A/CNAME nội bộ (ví dụ k8s.internal -> 10.60.10.100).",
      caption_dns: "Hình 2.11: Giao diện Split-DNS quản lý bản ghi tên miền nội bộ và DNS upstream.",
      caption_add_dns: "Hình 2.12: Hộp thoại thêm bản ghi DNS ghi đè với cơ chế nạp bộ nhớ cache tức thì.",

      admin_oidc_title: "Tích Hợp Đăng Nhập Doanh Nghiệp (OIDC SSO)",
      admin_oidc_desc: "Kết nối hệ thống định danh của doanh nghiệp (Google Workspace, Microsoft Entra ID, Okta, Keycloak hoặc Auth0) qua giao thức chuẩn OpenID Connect.",
      caption_oidc: "Hình 2.13: Cổng kết nối SSO quản lý đăng nhập một lần cho toàn bộ nhân sự công ty.",
      caption_add_oidc: "Hình 2.14: Kết nối nhà cung cấp định danh qua Issuer URL, Client ID và Client Secret.",

      admin_settings_title: "Cài Đặt Máy Chủ & Quản Trị Socket VICI",
      admin_settings_desc: "Tại Cài đặt máy chủ, thiết lập FQDN công khai của gateway (vpn.company.com), cấu hình tên miền tìm kiếm và kiểm tra kết nối socket UNIX /var/run/charon.vici.",
      caption_settings: "Hình 2.15: Cài đặt máy chủ quản lý địa chỉ public, tạo lại chứng chỉ PKI và kiểm tra socket.",

      admin_logs_title: "Nhật Ký Hệ Thống & Audit Stream Thời Gian Thực",
      admin_logs_desc: "Truyền phát trực tiếp log kernel và daemon charon qua Server-Sent Events (SSE). Lọc theo mức độ nghiêm trọng (DEBUG, INFO, WARN, ERROR) hoặc tìm kiếm theo IP.",
      caption_logs: "Hình 2.16: Bảng dòng lệnh realtime hiển thị log có bộ lọc scope, tìm kiếm và nút tạm dừng.",

      user_why_title: "Vì Sao SwanPanel Sử Dụng 100% Native OS Client?",
      user_why_desc: "Khác với các giải pháp VPN truyền thống bắt buộc cài app ngoài gây hao pin và chậm máy, SwanPanel tận dụng trực tiếp engine IKEv2/IPsec có sẵn trong mọi hệ điều hành hiện đại.",

      user_apple_title: "Hệ Sinh Thái Apple: macOS & iOS (.mobileconfig)",
      user_apple_desc: "SwanPanel đóng gói chứng chỉ client, khóa riêng tư và chuỗi Root CA vào 1 profile Apple (.mobileconfig) duy nhất.",

      user_win_title: "Microsoft Windows 10 & 11 (PowerShell Script)",
      user_win_desc: "SwanPanel tạo file PowerShell tự động import Root CA, cài đặt chứng chỉ máy tính và thiết lập kết nối VPN native trên Windows:",

      user_linux_title: "Linux Desktop (NetworkManager & nmcli)",
      user_linux_desc: "Trên các bản phân phối Ubuntu, Fedora, Debian và Arch, SwanPanel tích hợp trực tiếp với NetworkManager:",

      user_android_title: "Google Android (IKEv2 RSA Native)",
      user_android_desc: "Android kết nối qua giao thức chuẩn IKEv2/IPSec RSA trong Cài đặt hệ thống:",

      user_samples_title: "Tham Khảo Các File Cấu Hình Mẫu",
      user_samples_desc: "Tải và kiểm tra các file cấu hình mẫu đi kèm SwanPanel:"
    },
    footer: {
      github: "GitHub Repository",
      wiki: "Tài liệu Wiki",
      releases: "Bản phát hành v0.0.2",
      guide: "Tài liệu & Hướng dẫn",
      home: "Trang chủ",
      admin_guide: "Cẩm Nang Quản Trị",
      enduser_guide: "Cẩm Nang Người Dùng",
      license: "Giấy phép AGPL-3.0",
      credit: "SwanPanel là phần mềm mã nguồn mở được phát triển bởi Thái Thiên Vũ. Phát hành theo giấy phép GNU AGPL-3.0."
    }
  }
};

window.SwanI18n = {
  translations,
  currentLang: 'en',

  init() {
    const saved = localStorage.getItem('swanpanel-docs-lang');
    const browserLang = navigator.language && navigator.language.startsWith('vi') ? 'vi' : 'en';
    this.currentLang = saved || browserLang;
    this.applyLanguage(this.currentLang);
  },

  setLanguage(lang) {
    if (!translations[lang]) return;
    this.currentLang = lang;
    localStorage.setItem('swanpanel-docs-lang', lang);
    this.applyLanguage(lang);
  },

  toggleLanguage() {
    const nextLang = this.currentLang === 'vi' ? 'en' : 'vi';
    this.setLanguage(nextLang);
  },

  applyLanguage(lang) {
    const dict = translations[lang] || translations.en;
    document.documentElement.lang = lang;

    if (dict.meta && dict.meta.title_landing) {
      document.title = dict.meta.title_landing;
      const metaDesc = document.querySelector('meta[name="description"]');
      if (metaDesc) metaDesc.setAttribute('content', dict.meta.desc_landing);
    }

    document.querySelectorAll('[data-i18n]').forEach((el) => {
      const key = el.getAttribute('data-i18n');
      const val = this.resolveKey(dict, key);
      if (val !== undefined) {
        el.textContent = val;
      }
    });

    document.querySelectorAll('[data-i18n-html]').forEach((el) => {
      const key = el.getAttribute('data-i18n-html');
      const val = this.resolveKey(dict, key);
      if (val !== undefined) {
        el.innerHTML = val;
      }
    });

    const langBtn = document.getElementById('lang-toggle');
    if (langBtn) {
      const textSpan = langBtn.querySelector('.lang-text');
      if (textSpan) {
        textSpan.textContent = lang === 'vi' ? 'VI' : 'EN';
      }
      langBtn.setAttribute('title', dict.nav.switch_lang_title || 'Switch Language');
      langBtn.setAttribute('aria-label', dict.nav.switch_lang_title || 'Switch Language');
    }
  },

  resolveKey(obj, path) {
    return path.split('.').reduce((prev, curr) => (prev ? prev[curr] : undefined), obj);
  }
};
