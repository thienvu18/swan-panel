// SwanPanel Marketing & Landing i18n Engine
// Streamlined, punchy marketing copy with natural phrasing and preserved technical terms

const translations = {
  en: {
    meta: {
      title_landing: "SwanPanel - Modern Zero-Trust Control Plane for strongSwan VPN",
      desc_landing: "Enterprise-grade, zero-trust web control plane for strongSwan IKEv2/IPsec VPNs with 100% native client provisioning, automated PKI, Split-DNS, and zero-downtime RAM reloads."
    },
    nav: {
      why: "Why SwanPanel",
      features: "Features",
      comparison: "Comparison",
      quickstart: "Quickstart",
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
      install_title: "1-Line Production Installer (Ubuntu, Debian, RHEL, Rocky, Fedora)",
      install_hint_1: "Auto-detects OS & tunes kernel forwarding",
      install_hint_2: "Zero config required • Ready in under 60 seconds",
      install_hint_3: "Single pure Go binary with embedded React SPA & SQLite",
      copied: "Copied!",
      copy: "Copy",
      btn_get_started: "Get Started (60s)",
      btn_star: "Star on GitHub",
      btn_download: "Download v0.0.1 Binary"
    },
    pains: {
      tag: "Why SwanPanel",
      title: "Built to Fix Everything Broken About Traditional VPNs",
      subtitle: "Say goodbye to arcane config files, battery-draining client apps, and expensive SaaS subscriptions.",
      p1_title: "100% Native OS Client Profiles",
      p1_pain: "Employees hate installing clunky 3rd-party apps (OpenVPN, WireGuard client, Cisco AnyConnect) that drain laptop batteries, consume memory, and conflict with OS network settings.",
      p1_sol: "Native Connectivity: Connect directly from macOS, iOS, Windows, Linux, and Android system settings with native hardware-accelerated IPsec encryption. Zero apps to install.",
      p2_title: "Zero-Downtime Dynamic RAM Reloads",
      p2_pain: "Editing raw swanctl.conf or ipsec.conf over SSH is dangerous. A single typo crashes the daemon, and reloading static files drops active tunnels for all connected engineers.",
      p2_sol: "Zero-SSH VICI Control: Reload connections dynamically in RAM via UNIX socket /var/run/charon.vici without dropping live tunnels. Terminate active sessions immediately.",
      p3_title: "Autonomous Internal PKI (X.509)",
      p3_pain: "Generating Root CAs, signing client certs, and bundling password-protected PKCS#12 (.p12) archives with OpenSSL command lines takes hours of tedious manual crypto labor.",
      p3_sol: "1-Click Certificate Issuance: SwanPanel automatically provisions 4096-bit RSA Root CAs, signs client certificates, and packages encrypted .p12 bundles with one click.",
      p4_title: "Sovereign, Open Source & Non-Profit",
      p4_pain: "Commercial Zero-Trust platforms (Tailscale, Cloudflare Zero Trust) charge $5-$20/user/month, lock you into proprietary closed clouds, and route sensitive metadata outside.",
      p4_sol: "100% Free Forever: Fully self-hosted on your own hardware under GNU AGPL-3.0. Your keys, traffic, and access policies stay entirely under your sovereignty with zero SaaS fees."
    },
    features: {
      tag: "Capabilities",
      title: "Everything You Need for Enterprise Remote Access",
      subtitle: "Enterprise-grade performance without bloat, complex clusters, or external dependencies.",
      f1_title: "Native Client Profiles",
      f1_desc: "1-click signed Apple .mobileconfig, Windows PowerShell Add-VpnConnection, Linux NetworkManager, and Android IKEv2 profiles with embedded PKCS#12 certs.",
      f2_title: "Zero-SSH strongSwan Control",
      f2_desc: "Direct charon daemon management via UNIX socket /var/run/charon.vici using govici IPC. Dynamic in-memory reloads without dropping active tunnels.",
      f3_title: "Automated Internal PKI",
      f3_desc: "Self-contained X.509 Certificate Authority. Automatically provisions 4096-bit Root CA, issues client certs, and generates password-protected .p12 archives.",
      f4_title: "Split-DNS & MagicDNS",
      f4_desc: "High-performance embedded DNS server (miekg/dns). Auto-resolves internal devices (.strongnet), supports custom DNS overrides, and forwards recursive queries.",
      f5_title: "Zero-Trust Access Policies",
      f5_desc: "Granular access control mapping User Groups to Resource Groups. Compiles dynamically into kernel traffic selectors and explicit DROP filters.",
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
    steps: {
      tag: "Quickstart",
      title: "Up and Running in 60 Seconds",
      subtitle: "From a fresh Linux server to an enterprise zero-trust VPN in three quick steps.",
      s1_num: "01",
      s1_title: "Run 1-Line Installer",
      s1_desc: "Run curl -sSL https://raw.githubusercontent.com/thienvu18/swan-panel/main/install.sh | sudo bash. The script installs strongSwan, tunes kernel forwarding, and starts the systemd service.",
      s2_num: "02",
      s2_title: "Open Web Control Plane",
      s2_desc: "Navigate to http://<SERVER_IP>:8080. Log in with the default administrator credentials, configure your external hostname, and invite your team.",
      s3_num: "03",
      s3_title: "Download Native Profiles & Connect",
      s3_desc: "Provision devices with one click. Download Apple .mobileconfig, Windows PowerShell, or Linux profiles and connect directly from OS system settings."
    },
    community: {
      tag: "Open Source & Non-Profit",
      title: "Built for the Community. Free Forever.",
      desc1: "SwanPanel was created by Thái Thiên Vũ as an open-source, non-profit community project. We believe secure network access should be sovereign, fast, and free from recurring corporate SaaS subscriptions.",
      desc2: "There are no artificial limits, no paid enterprise editions, and no user tracking. SwanPanel is 100% free under the GNU AGPL-3.0 license.",
      cta_star_title: "Support the Project with a GitHub Star",
      cta_star_desc: "If SwanPanel helps your team save time, cut infrastructure costs, or streamline VPN operations, please give us a Star on GitHub! Your star helps more organizations discover SwanPanel and motivates ongoing open-source development.",
      btn_star_github: "Star SwanPanel on GitHub",
      btn_contribute: "Contribute on GitHub",
      btn_discussions: "Join Discussions"
    },
    footer: {
      github: "GitHub Repository",
      releases: "Release v0.0.1",
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
      quickstart: "Cài đặt nhanh",
      star: "Tặng 1 sao GitHub",
      get_started: "Bắt đầu ngay",
      toggle_theme: "Chuyển giao diện",
      switch_lang: "English",
      switch_lang_title: "Switch to English",
      menu: "Menu"
    },
    hero: {
      badge: "Mã Nguồn Mở & Phi Lợi Nhuận • Giấy Phép AGPL-3.0",
      title_start: "VPN Zero-Trust Chuẩn Doanh Nghiệp.",
      title_accent: "100% Native Client. Không Cần Cài Thêm App.",
      desc: "Chấm dứt việc ép nhân viên phải cài đặt các app VPN cồng kềnh, tốn pin. SwanPanel mang đến nền tảng Zero-Trust hiện đại, tự động hóa PKI, tích hợp Split-DNS và nạp cấu hình động qua RAM cho strongSwan IPsec trong một file nhị phân duy nhất.",
      install_title: "Lệnh Cài Đặt Tự Động 1 Dòng (Ubuntu, Debian, RHEL, Rocky, Fedora)",
      install_hint_1: "Tự nhận diện hệ điều hành & cấu hình chuyển tiếp gói tin kernel",
      install_hint_2: "Không cần cấu hình phức tạp • Sẵn sàng hoạt động dưới 60 giây",
      install_hint_3: "Một file nhị phân Go duy nhất nhúng sẵn React SPA & SQLite",
      copied: "Đã chép!",
      copy: "Sao chép",
      btn_get_started: "Cài đặt ngay (60s)",
      btn_star: "Tặng 1 sao trên GitHub",
      btn_download: "Tải Bản Nhị Phân v0.0.1"
    },
    pains: {
      tag: "Vì sao chọn SwanPanel",
      title: "Giải Quyết Triệt Để Mọi Bất Tiện Của VPN Truyền Thống",
      subtitle: "Nói lời tạm biệt với file cấu hình phức tạp, app VPN tốn pin và chi phí thuê bao SaaS đắt đỏ.",
      p1_title: "Cấu Hình 100% Native OS Client",
      p1_pain: "Nhân viên rất ngại cài đặt các ứng dụng VPN của bên thứ ba (OpenVPN, WireGuard client, Cisco AnyConnect). Những ứng dụng này ngốn RAM, làm nóng máy, hao pin và dễ xung đột mạng.",
      p1_sol: "Kết nối Native trực tiếp: Kết nối thẳng từ Cài đặt hệ thống trên macOS, iOS, Windows, Linux và Android với tốc độ tăng tốc phần cứng của IPsec. Hoàn toàn không cần cài app phụ.",
      p2_title: "Nạp Cấu Hình Động Qua RAM (Không Rớt Mạng)",
      p2_pain: "Sửa file swanctl.conf hay ipsec.conf qua SSH rất dễ lỗi cú pháp. Mỗi lần khởi động lại daemon, toàn bộ kết nối VPN của các kỹ sư trong công ty đều bị ngắt quãng đột ngột.",
      p2_sol: "Điều khiển VICI Zero-SSH: Nạp cấu hình động trực tiếp vào RAM qua UNIX socket /var/run/charon.vici mà không làm gián đoạn đường hầm đang chạy. Ngắt phiên kết nối ngay lập tức.",
      p3_title: "Tự Động Hóa PKI Nội Bộ (X.509)",
      p3_pain: "Tự tạo Root CA, ký chứng chỉ client và đóng gói file PKCS#12 (.p12) bằng dòng lệnh OpenSSL đòi hỏi kỹ năng chuyên sâu và tốn hàng giờ thao tác thủ công dễ sai lệch.",
      p3_sol: "Xuất Chứng Chỉ 1 Cú Click: SwanPanel tự động khởi tạo Root CA 4096-bit RSA, cấp phát chứng chỉ client và đóng gói file .p12 có mã hóa mật khẩu hoàn toàn tự động.",
      p4_title: "Tự Chủ, Mã Nguồn Mở & Phi Lợi Nhuận",
      p4_pain: "Các nền tảng Zero-Trust thương mại (Tailscale, Cloudflare Zero Trust) thu 5$ - 20$/user/tháng, giam lỏng hạ tầng của bạn vào hệ thống đóng và lưu trữ metadata trên cloud ngoài.",
      p4_sol: "Miễn Phí Mãi Mãi (AGPL-3.0): Tự lưu trữ (Self-Hosted) 100% trên hạ tầng riêng. Khóa bí mật, lưu lượng mạng và luật phân quyền hoàn toàn nằm trong tay bạn, không phí định kỳ."
    },
    features: {
      tag: "Tính Năng",
      title: "Mọi Thứ Bạn Cần Cho Hệ Thống Remote Access Doanh Nghiệp",
      subtitle: "Hiệu năng mạnh mẽ chuẩn doanh nghiệp, không cồng kềnh, không chi phí ẩn, không phụ thuộc bên ngoài.",
      f1_title: "Cấu Hình Native Client",
      f1_desc: "Tải 1 cú nhấp profile Apple .mobileconfig, script PowerShell cho Windows, Linux NetworkManager và Android IKEv2 kèm chứng chỉ PKCS#12 nhúng sẵn.",
      f2_title: "Quản Trị strongSwan Zero-SSH",
      f2_desc: "Giao tiếp trực tiếp với daemon charon qua UNIX socket /var/run/charon.vici theo giao thức govici. Nạp cấu hình vào RAM không ngắt đường hầm đang chạy.",
      f3_title: "Tự Động Hóa PKI",
      f3_desc: "Hệ thống Certificate Authority X.509 tích hợp. Tự tạo Root CA 4096-bit, cấp phát chứng chỉ thiết bị và đóng gói archive .p12 bảo mật cao.",
      f4_title: "Split-DNS & MagicDNS",
      f4_desc: "Máy chủ DNS nội bộ hiệu năng cao (miekg/dns). Tự phân giải tên miền nội bộ (.strongnet), hỗ trợ Custom Overrides và chuyển tiếp truy vấn ra internet.",
      f5_title: "Chính Sách Zero-Trust Động",
      f5_desc: "Phân quyền chi tiết giữa Nhóm người dùng (User Groups) và Nhóm tài nguyên (Resource Groups). Tự biên dịch thành bộ lọc DROP/ACCEPT ở tầng kernel.",
      f6_title: "Một File Nhị Phân Duy Nhất",
      f6_desc: "Backend Go, cơ sở dữ liệu SQLite tự động migrate, giao diện React 19 SPA nhúng sẵn và DNS engine compile thành 1 file nhị phân tĩnh duy nhất."
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
    steps: {
      tag: "Cài Đặt Nhanh",
      title: "Sẵn Sàng Hoạt Động Trong 60 Giây",
      subtitle: "Từ máy chủ VPS Linux trắng đến hệ thống VPN Zero-Trust sẵn sàng phục vụ chỉ qua 3 bước.",
      s1_num: "01",
      s1_title: "Chạy Lệnh Cài Đặt 1 Dòng",
      s1_desc: "Chạy lệnh curl -sSL https://raw.githubusercontent.com/thienvu18/swan-panel/main/install.sh | sudo bash. Script tự cài strongSwan, cấu hình kernel forwarding và khởi động dịch vụ systemd.",
      s2_num: "02",
      s2_title: "Mở Giao Diện Web Quản Trị",
      s2_desc: "Truy cập http://<IP_SERVER>:8080 trên trình duyệt. Đăng nhập bằng tài khoản root admin mặc định, cấu hình IP/tên miền máy chủ và mời thành viên.",
      s3_num: "03",
      s3_title: "Tải Profile Native & Kết Nối",
      s3_desc: "Tạo thiết bị chỉ với 1 cú click. Tải file Apple .mobileconfig, script PowerShell cho Windows hoặc Linux về máy và kết nối trực tiếp từ Cài đặt hệ điều hành."
    },
    community: {
      tag: "Mã Nguồn Mở & Phi Lợi Nhuận",
      title: "Vì Lợi Ích Cộng Đồng. Miễn Phí Mãi Mãi.",
      desc1: "SwanPanel được phát triển bởi Thái Thiên Vũ với tinh thần cống hiến cho cộng đồng mã nguồn mở và hoàn toàn phi lợi nhuận. Chúng tôi tin rằng quyền quản trị mạng riêng của doanh nghiệp phải hoàn toàn tự chủ, tốc độ cao và không bị ràng buộc bởi chi phí thuê bao SaaS đắt đỏ.",
      desc2: "Dự án không có giới hạn tính năng nhân tạo, không có bản 'Enterprise' trả phí, và không thu thập bất kỳ dữ liệu telemetry nào. SwanPanel hoàn toàn tự do theo giấy phép GNU AGPL-3.0.",
      cta_star_title: "Ủng Hộ Tác Giả: Tặng 1 Sao Trên GitHub",
      cta_star_desc: "Nếu SwanPanel giúp bạn tiết kiệm thời gian, tối ưu chi phí hạ tầng hay đơn giản hóa việc quản trị VPN, hãy ủng hộ tinh thần tác giả bằng 1 nút Star (Sao) trên GitHub! Ngôi sao của bạn sẽ giúp dự án tiếp cận được nhiều kỹ sư hơn và là nguồn động lực lớn để duy trì phát triển mã nguồn mở.",
      btn_star_github: "Tặng Sao SwanPanel trên GitHub",
      btn_contribute: "Đóng Góp Trên GitHub",
      btn_discussions: "Diễn Đàn Thảo Luận"
    },
    footer: {
      github: "GitHub Repository",
      releases: "Bản phát hành v0.0.1",
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
