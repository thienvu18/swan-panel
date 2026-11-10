// SwanPanel Documentation & Landing i18n Engine
// Easily extensible: Add new languages by contributing a locale object below!

const translations = {
  en: {
    meta: {
      title_landing: "SwanPanel - Modern Zero-Trust Control Plane for strongSwan VPN",
      desc_landing: "Enterprise-grade, zero-trust web control plane for strongSwan IKEv2/IPsec VPNs with 100% native client provisioning, automated PKI, Split-DNS, and zero-downtime RAM reloads.",
      title_guide: "SwanPanel User Guide & Technical Documentation",
      desc_guide: "Complete user guide and operational reference for SwanPanel: 1-line installation, native client configuration, Split-DNS, PKI, and zero-trust policies."
    },
    nav: {
      features: "Features",
      why: "Why SwanPanel",
      architecture: "Architecture",
      comparison: "Comparison",
      guide: "User Guide",
      wiki: "Wiki",
      star: "Star on GitHub",
      read_docs: "Read Docs",
      toggle_theme: "Toggle theme",
      switch_lang: "Tiếng Việt",
      switch_lang_title: "Chuyển sang Tiếng Việt",
      menu: "Menu"
    },
    hero: {
      badge: "Open Source & Non-Profit • Free under AGPL-3.0",
      title_start: "Enterprise Zero-Trust Remote Access.",
      title_accent: "100% Native Clients. Zero 3rd-Party Apps.",
      desc: "Stop forcing employees to install bulky, battery-draining VPN apps. SwanPanel brings modern zero-trust control, automated PKI, Split-DNS, and zero-downtime dynamic reloads to strongSwan IPsec in a single self-contained binary.",
      install_title: "Quickstart 1-Line Installer (Ubuntu, Debian, RHEL, Rocky, Fedora)",
      copied: "Copied!",
      btn_deploy: "Explore User Guide",
      btn_download: "Download Binary (v0.0.1)",
      btn_star: "Star on GitHub"
    },
    pains: {
      tag: "The Problems We Solve",
      title: "Say Goodbye to VPN Friction & SaaS Extortion",
      subtitle: "Why traditional VPN administration and commercial Zero-Trust SaaS fail modern engineering teams.",
      pain1_title: "Arcane Config Files & Risky Downtime",
      pain1_desc: "Editing raw swanctl.conf or ipsec.conf text files over SSH is error-prone. A single syntax mistake crashes the daemon, and reloading static files drops active tunnels for all connected engineers.",
      pain1_sol: "SwanPanel Solution: Zero-SSH management via /var/run/charon.vici. Dynamic in-memory reloads execute in RAM with zero tunnel interruptions, live session termination, and real-time telemetry.",
      pain2_title: "The 3rd-Party VPN Client Nightmare",
      pain2_desc: "Users hate installing clunky 3rd-party apps (OpenVPN, WireGuard client, Cisco AnyConnect) that consume RAM, drain laptop batteries, and conflict with native OS networking.",
      pain2_sol: "SwanPanel Solution: 100% Native OS profiles. Apple .mobileconfig (1-click install in macOS/iOS Settings), Windows PowerShell Add-VpnConnection, Linux NetworkManager, and Android IKEv2. Zero software to install.",
      pain3_title: "Manual PKI & Certificate Mismatches",
      pain3_desc: "Generating X.509 Root CAs, signing client certificates, and bundling password-protected PKCS#12 (.p12) archives with OpenSSL command lines requires deep crypto expertise and hours of manual labor.",
      pain3_sol: "SwanPanel Solution: Autonomous internal PKI. SwanPanel automatically provisions 4096-bit RSA Root CAs, issues client certificates, and packages encrypted .p12 bundles with one click.",
      pain4_title: "Expensive SaaS Lock-In & Privacy Leaks",
      pain4_desc: "Commercial platforms (Tailscale, Cloudflare Zero Trust, Twingate) charge high per-seat monthly subscriptions, lock you into proprietary closed-source controllers, and route metadata through external clouds.",
      pain4_sol: "SwanPanel Solution: 100% Self-Hosted, Sovereign, and Non-Profit. Free forever under AGPL-3.0. Your keys, access policies, and traffic stay entirely on your servers."
    },
    features: {
      tag: "Core Capabilities",
      title: "Engineered for Sovereign Zero-Trust Infrastructure",
      subtitle: "Enterprise-grade performance without bloat, subscriptions, or external dependencies.",
      f1_title: "100% Native Client Provisioning",
      f1_desc: "Deliver signed Apple .mobileconfig profiles, Windows PowerShell scripts, Linux NetworkManager files, and Android IKEv2 profiles with embedded PKCS#12 certificates. Users connect directly from OS system settings.",
      f2_title: "Zero-SSH strongSwan Control",
      f2_desc: "Communicate directly with strongSwan's charon daemon via UNIX socket /var/run/charon.vici using the binary govici protocol. Reload connections in RAM without dropping active tunnels and terminate sessions instantly.",
      f3_title: "Automated Internal PKI",
      f3_desc: "Autonomous X.509 Certificate Authority. Automatically provisions 4096-bit RSA Root CAs, signs client certificates, and packages password-protected PKCS#12 bundles without OpenSSL CLI steps.",
      f4_title: "Configurable Split-DNS & MagicDNS",
      f4_desc: "Integrated high-performance DNS server (miekg/dns). Dynamically resolves internal device hostnames (.strongnet, .internal), supports custom DNS overrides, and recursively forwards queries upstream.",
      f5_title: "Dynamic Zero-Trust Policies",
      f5_desc: "Define granular access control rules mapping User Groups to Resource Groups. SwanPanel compiles policies dynamically into kernel traffic selectors and explicit DROP filters, eliminating lateral movement.",
      f6_title: "Single Self-Contained Binary",
      f6_desc: "The entire control plane—Go backend, SQLite database with auto-migration, embedded React 19 SPA, PKI, and DNS engine—compiles into a pure Go static executable with zero CGO dependencies."
    },
    arch: {
      tag: "Architecture",
      title: "Zero-Downtime Decoupled Control Plane",
      subtitle: "SwanPanel completely decouples high-level web administration from the kernel-level strongSwan data plane."
    },
    comparison: {
      tag: "Comparison Matrix",
      title: "How SwanPanel Compares",
      subtitle: "Comparing native OS enterprise connectivity versus traditional alternatives.",
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
      r2_saas: "Yes (Closed-source cloud)",
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
    steps: {
      tag: "Get Started in 60 Seconds",
      title: "Deploying SwanPanel is Ridiculously Simple",
      subtitle: "From a bare Linux VPS to a production-ready zero-trust VPN in three quick steps.",
      s1_num: "01",
      s1_title: "Run 1-Line Installer",
      s1_desc: "Run curl -sSL https://raw.githubusercontent.com/thienvu18/swan-panel/main/install.sh | sudo bash. The script installs strongSwan, tunes kernel forwarding, and starts the systemd service.",
      s2_num: "02",
      s2_title: "Open Web Control Plane",
      s2_desc: "Navigate to http://<SERVER_IP>:8080. Log in with the default administrator credentials, configure your external hostname, and invite your team.",
      s3_num: "03",
      s3_title: "Download Native Profiles & Connect",
      s3_desc: "Add personal or company devices with one click. Download Apple .mobileconfig, Windows PowerShell, or Linux profiles and connect directly from OS settings."
    },
    community: {
      tag: "Open Source & Non-Profit",
      title: "Built for the Community. Free Forever.",
      desc1: "SwanPanel was created by Thái Thiên Vũ as an open-source, non-profit community project. We believe secure network access should be sovereign, fast, and free from recurring corporate SaaS subscriptions.",
      desc2: "There are no artificial limits, no paid enterprise editions, and no user tracking. SwanPanel is 100% free under the GNU AGPL-3.0 license.",
      cta_star_title: "Show Your Support: Star on GitHub",
      cta_star_desc: "If SwanPanel helps you save time, reduce infrastructure costs, or streamline VPN management, please give us a Star on GitHub! Your star helps more developers discover the project and motivates ongoing open-source maintenance.",
      btn_star_github: "Star SwanPanel on GitHub",
      btn_contribute: "Contribute Code",
      btn_discussions: "Join Discussions",
      btn_translate: "Submit a Translation"
    },
    footer: {
      home: "Home",
      guide: "User Guide",
      wiki: "Wiki",
      github: "GitHub",
      releases: "Releases",
      license: "AGPL-3.0 License",
      credit: "SwanPanel is open-source software created by Thái Thiên Vũ. Licensed under GNU AGPL-3.0."
    },
    guide_sidebar: {
      getting_started: "Getting Started",
      intro: "Introduction",
      prerequisites: "Prerequisites",
      installation: "1-Line Installation",
      configuration: "Configuration",
      client_setup: "Native Client Setup",
      client_apple: "macOS & iOS",
      client_windows: "Windows 10 & 11",
      client_linux: "Linux (NetworkManager)",
      client_android: "Android (Native IKEv2)",
      subsystems: "Subsystems & Core",
      vici: "Zero-SSH VICI Control",
      dns: "Split-DNS & MagicDNS",
      policy: "Zero-Trust Policies",
      pki: "Automated Internal PKI",
      operations: "Operations",
      production: "Production Hardening",
      troubleshooting: "Troubleshooting & FAQ"
    }
  },

  vi: {
    meta: {
      title_landing: "SwanPanel - Bảng Điều Khiển Zero-Trust Hiện Đại cho strongSwan VPN",
      desc_landing: "Hệ thống Web Control Plane Zero-Trust chuẩn doanh nghiệp cho strongSwan IKEv2/IPsec VPN. Cấu hình 100% native client, tích hợp PKI, Split-DNS và nạp cấu hình động qua RAM không downtime.",
      title_guide: "Tài Liệu Hướng Dẫn Kỹ Thuật & Vận Hành SwanPanel",
      desc_guide: "Tài liệu kỹ thuật toàn diện cho SwanPanel: cài đặt 1 dòng lệnh, cấu hình native client, Split-DNS, PKI và chính sách Zero-Trust."
    },
    nav: {
      features: "Tính năng",
      why: "Tại sao chọn SwanPanel",
      architecture: "Kiến trúc",
      comparison: "So sánh",
      guide: "Tài liệu",
      wiki: "Wiki",
      star: "Tặng 1 sao GitHub",
      read_docs: "Xem tài liệu",
      toggle_theme: "Chuyển giao diện",
      switch_lang: "English",
      switch_lang_title: "Switch to English",
      menu: "Menu"
    },
    hero: {
      badge: "Mã Nguồn Mở & Phi Lợi Nhuận • Giấy Phép AGPL-3.0",
      title_start: "VPN Zero-Trust Chuẩn Doanh Nghiệp.",
      title_accent: "Tận Dụng 100% Native Client. Không Cần Cài App.",
      desc: "Chấm dứt việc ép nhân viên phải cài đặt các ứng dụng VPN của bên thứ ba cồng kềnh, tốn pin. SwanPanel mang đến nền tảng quản trị Zero-Trust hiện đại, tự động hóa PKI, tích hợp Split-DNS và nạp cấu hình động không downtime cho strongSwan IPsec trong một file nhị phân duy nhất.",
      install_title: "Lệnh Cài Đặt Tự Động 1 Dòng (Ubuntu, Debian, RHEL, Rocky, Fedora)",
      copied: "Đã sao chép!",
      btn_deploy: "Khám Phá Tài Liệu",
      btn_download: "Tải Bản Nhị Phân (v0.0.1)",
      btn_star: "Tặng 1 sao trên GitHub"
    },
    pains: {
      tag: "Vấn Đề Thực Tế & Giải Pháp",
      title: "Giải Phóng Bạn Khỏi Sự Phiền Toái của VPN Truyền Thống",
      subtitle: "Tại sao quản trị VPN thủ công và các dịch vụ Zero-Trust SaaS đắt đỏ đang làm đau đầu các đội ngũ kỹ thuật.",
      pain1_title: "Cơn Ác Mộng Cấu Hình CLI & Rủi Ro Rớt Mạng",
      pain1_desc: "Chỉnh sửa file cấu hình thô swanctl.conf hay ipsec.conf qua SSH rất dễ nhầm lẫn cú pháp. Mỗi lần reload file tĩnh, toàn bộ đường hầm VPN đang hoạt động của cả công ty đều bị ngắt quãng.",
      pain1_sol: "Giải pháp SwanPanel: Quản trị Zero-SSH qua socket /var/run/charon.vici. Nạp cấu hình trực tiếp vào RAM không gián đoạn đường hầm, ngắt phiên làm việc tức thời và theo dõi lưu lượng thời gian thực.",
      pain2_title: "Gánh Nặng Ứng Dụng Bên Thứ Ba & Hao Pin",
      pain2_desc: "Nhân viên thường gặp khó khăn khi cài đặt các app VPN của bên thứ ba (OpenVPN, WireGuard client, Cisco AnyConnect). Các app này chạy ngầm tốn RAM, hao pin laptop/điện thoại và dễ xung đột mạng.",
      pain2_sol: "Giải pháp SwanPanel: 100% Native OS Profile. Xuất file Apple .mobileconfig (1 cú click cài vào Cài đặt macOS/iOS), script Windows PowerShell Add-VpnConnection, Linux NetworkManager và Android IKEv2. Không cần cài thêm bất kỳ app nào.",
      pain3_title: "Khó Khăn Quản Trị PKI & Đóng Gói Chứng Chỉ",
      pain3_desc: "Tạo Root CA, ký chứng chỉ client và đóng gói file PKCS#12 (.p12) bằng dòng lệnh OpenSSL đòi hỏi chuyên môn mật mã phức tạp, dễ sai sót chuỗi CA trust chain và mất nhiều thời gian.",
      pain3_sol: "Giải pháp SwanPanel: Tích hợp PKI tự trị. SwanPanel tự động khởi tạo Root CA 4096-bit RSA, ký chứng chỉ và đóng gói file .p12 có mật khẩu bảo vệ chỉ với một cú nhấp chuột.",
      pain4_title: "Chi Phí SaaS Đắt Đỏ & Rủi Ro Dữ Liệu",
      pain4_desc: "Các nền tảng thương mại (Tailscale, Cloudflare Zero Trust, Twingate) thu phí định kỳ đắt đỏ theo từng người dùng mỗi tháng, giam cầm hạ tầng của bạn vào hệ thống đóng và lưu trữ metadata trên cloud bên ngoài.",
      pain4_sol: "Giải pháp SwanPanel: 100% Self-Hosted, Tự Chủ & Phi Lợi Nhuận. Miễn phí hoàn toàn theo giấy phép AGPL-3.0. Khóa bí mật, phân quyền và lưu lượng mạng hoàn toàn thuộc quyền kiểm soát của bạn."
    },
    features: {
      tag: "Tính Năng Cốt Lõi",
      title: "Thiết Kế Cho Hạ Tầng Zero-Trust Tự Chủ",
      subtitle: "Hiệu năng vượt trội chuẩn doanh nghiệp, không rườm rà, không chi phí ẩn, không phụ thuộc bên ngoài.",
      f1_title: "Cấu Hình 100% Native Client",
      f1_desc: "Cung cấp profile Apple .mobileconfig đã ký, script Windows PowerShell, file cấu hình Linux NetworkManager và Android IKEv2 kèm chứng chỉ PKCS#12. Người dùng kết nối trực tiếp từ cài đặt hệ điều hành.",
      f2_title: "Điều Khiển strongSwan Zero-SSH",
      f2_desc: "Giao tiếp trực tiếp với daemon charon qua UNIX socket /var/run/charon.vici theo giao thức govici. Nạp cấu hình vào RAM không ngắt đường hầm đang chạy, chấm dứt phiên ngay tức thì.",
      f3_title: "Tự Động Hóa PKI Nội Bộ",
      f3_desc: "Hệ thống Certificate Authority X.509 độc lập. Tự sinh Root CA 4096-bit RSA, cấp phát chứng chỉ client và đóng gói archive .p12 có mã hóa mật khẩu mà không cần gõ lệnh OpenSSL.",
      f4_title: "Máy Chủ Split-DNS & MagicDNS Tùy Biến",
      f4_desc: "Tích hợp DNS engine hiệu năng cao (miekg/dns). Tự động phân giải hostname nội bộ (.strongnet, .internal) về IP ảo, hỗ trợ Custom Overrides thủ công và chuyển tiếp truy vấn ra internet.",
      f5_title: "Chính Sách Phân Quyền Zero-Trust Động",
      f5_desc: "Thiết lập các luật phân quyền linh hoạt giữa Nhóm người dùng (User Groups) và Nhóm tài nguyên (Resource Groups). SwanPanel tự biên dịch luật thành bộ lọc DROP/ACCEPT ở tầng kernel, chặn đứng di chuyển ngang.",
      f6_title: "Một File Nhị Phân Duy Nhất (Single-Binary)",
      f6_desc: "Toàn bộ hệ thống—Backend Go, cơ sở dữ liệu SQLite tự động migrate, giao diện React 19 SPA nhúng sẵn, PKI và DNS server—được biên dịch thành 1 file nhị phân tĩnh duy nhất không phụ thuộc CGO."
    },
    arch: {
      tag: "Kiến Trúc",
      title: "Control Plane Phân Tách Không Downtime",
      subtitle: "SwanPanel tách biệt hoàn toàn tầng quản trị giao diện web khỏi tầng dữ liệu kernel IPsec của strongSwan."
    },
    comparison: {
      tag: "Bảng So Sánh",
      title: "Ưu Thế Của SwanPanel",
      subtitle: "So sánh khả năng kết nối native doanh nghiệp của SwanPanel với các giải pháp khác.",
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
      r3_title: "Nạp Động Qua RAM (Không Downtime)",
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
    steps: {
      tag: "Bắt Đầu Trong 60 Giây",
      title: "Triển Khai Cực Kỳ Nhanh Chóng & Dễ Dàng",
      subtitle: "Từ máy chủ VPS Linux trắng đến hệ thống VPN Zero-Trust sẵn sàng vận hành chỉ với 3 bước.",
      s1_num: "01",
      s1_title: "Chạy Lệnh Cài Đặt 1 Dòng",
      s1_desc: "Chạy curl -sSL https://raw.githubusercontent.com/thienvu18/swan-panel/main/install.sh | sudo bash. Script tự cài strongSwan, cấu hình kernel forwarding và khởi chạy systemd service.",
      s2_num: "02",
      s2_title: "Mở Giao Diện Web Quản Trị",
      s2_desc: "Truy cập http://<IP_SERVER>:8080 trên trình duyệt. Đăng nhập với tài khoản root admin mặc định, cấu hình IP/tên miền máy chủ và mời các thành viên tham gia.",
      s3_num: "03",
      s3_title: "Tải Profile Native & Kết Nối",
      s3_desc: "Khởi tạo thiết bị cá nhân hoặc máy chủ chỉ bằng 1 cú nhấp chuột. Tải file Apple .mobileconfig, script PowerShell hoặc Linux về máy và kết nối trực tiếp từ Cài đặt hệ điều hành."
    },
    community: {
      tag: "Mã Nguồn Mở & Phi Lợi Nhuận",
      title: "Vì Lợi Ích Cộng Đồng. Miễn Phí Mãi Mãi.",
      desc1: "SwanPanel được phát triển bởi Thái Thiên Vũ với tinh thần cống hiến cho cộng đồng mã nguồn mở và hoàn toàn phi lợi nhuận. Chúng tôi tin rằng quyền quản trị mạng riêng của doanh nghiệp phải hoàn toàn tự chủ, tốc độ cao và không bị ràng buộc bởi chi phí thuê bao SaaS đắt đỏ.",
      desc2: "Dự án không có giới hạn tính năng nhân tạo, không có bản 'Enterprise' trả phí, và không thu thập bất kỳ dữ liệu telemetry nào. SwanPanel hoàn toàn tự do theo giấy phép GNU AGPL-3.0.",
      cta_star_title: "Ủng Hộ Tác Giả: Tặng 1 Sao Trên GitHub",
      cta_star_desc: "Nếu SwanPanel giúp bạn tiết kiệm thời gian, tối ưu chi phí hạ tầng hay đơn giản hóa việc quản trị VPN, hãy ủng hộ tinh thần tác giả bằng 1 nút Star (Sao) trên GitHub! Ngôi sao của bạn sẽ giúp dự án tiếp cận được nhiều kỹ sư hơn và là nguồn động lực lớn để duy trì phát triển mã nguồn mở.",
      btn_star_github: "Tặng Sao SwanPanel trên GitHub",
      btn_contribute: "Đóng Góp Mã Nguồn",
      btn_discussions: "Diễn Đàn Thảo Luận",
      btn_translate: "Đóng Góp Bản Dịch Mới"
    },
    footer: {
      home: "Trang chủ",
      guide: "Tài liệu",
      wiki: "Wiki",
      github: "GitHub",
      releases: "Bản phát hành",
      license: "Giấy phép AGPL-3.0",
      credit: "SwanPanel là phần mềm mã nguồn mở được phát triển bởi Thái Thiên Vũ. Phát hành theo giấy phép GNU AGPL-3.0."
    },
    guide_sidebar: {
      getting_started: "Bắt đầu nhanh",
      intro: "Giới thiệu",
      prerequisites: "Yêu cầu hệ thống",
      installation: "Cài đặt 1 dòng lệnh",
      configuration: "Biến môi trường",
      client_setup: "Cấu hình Native Client",
      client_apple: "macOS & iOS",
      client_windows: "Windows 10 & 11",
      client_linux: "Linux (NetworkManager)",
      client_android: "Android (IKEv2)",
      subsystems: "Phân hệ cốt lõi",
      vici: "Điều khiển VICI Zero-SSH",
      dns: "Split-DNS & MagicDNS",
      policy: "Chính sách Zero-Trust",
      pki: "Tự động hóa PKI",
      operations: "Vận hành hệ thống",
      production: "Triển khai Production",
      troubleshooting: "Khắc phục sự cố & FAQ"
    }
  }
};

// Expose globally for browser usage
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

    // Update document title & meta description if defined
    const isGuide = window.location.pathname.includes('guide');
    if (isGuide && dict.meta.title_guide) {
      document.title = dict.meta.title_guide;
      const metaDesc = document.querySelector('meta[name="description"]');
      if (metaDesc) metaDesc.setAttribute('content', dict.meta.desc_guide);
    } else if (!isGuide && dict.meta.title_landing) {
      document.title = dict.meta.title_landing;
      const metaDesc = document.querySelector('meta[name="description"]');
      if (metaDesc) metaDesc.setAttribute('content', dict.meta.desc_landing);
    }

    // Translate all elements with data-i18n attribute
    document.querySelectorAll('[data-i18n]').forEach((el) => {
      const key = el.getAttribute('data-i18n');
      const val = this.resolveKey(dict, key);
      if (val !== undefined) {
        el.textContent = val;
      }
    });

    // Translate elements with data-i18n-html (allowing nested spans/tags)
    document.querySelectorAll('[data-i18n-html]').forEach((el) => {
      const key = el.getAttribute('data-i18n-html');
      const val = this.resolveKey(dict, key);
      if (val !== undefined) {
        el.innerHTML = val;
      }
    });

    // Translate attributes: data-i18n-attr="title:nav.switch_lang_title,aria-label:nav.menu"
    document.querySelectorAll('[data-i18n-attr]').forEach((el) => {
      const attrDefs = el.getAttribute('data-i18n-attr').split(',');
      attrDefs.forEach((def) => {
        const [attr, key] = def.split(':');
        const val = this.resolveKey(dict, key);
        if (val !== undefined) {
          el.setAttribute(attr.trim(), val);
        }
      });
    });

    // Update language switch button text and label
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
